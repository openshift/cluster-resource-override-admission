package clusterresourceoverride

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/cache"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"

	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
)

const (
	clusterResourceOverrideAnnotation = "autoscaling.openshift.io/cluster-resource-override-enabled"
	defaultResyncPeriod               = 5 * time.Hour
	inClusterConfigFilePath           = "/var/cluster-resource-override.yaml"
)

const (
	cpuBaseScaleFactor = 1000.0 / (1024.0 * 1024.0 * 1024.0) // 1000 milliCores per 1GiB
)

var (
	defaultCPUFloor    = resource.MustParse("1m")
	defaultMemoryFloor = resource.MustParse("1Mi")
)

// ConfigLoaderFunc loads a Config object from appropriate source and returns it.
type ConfigLoaderFunc func() (config *Config, err error)

// Admission interface encapsulates the admission logic for ClusterResourceOverride plugin.
type Admission interface {
	// GetConfiguration returns the configuration in use by the admission logic.
	GetConfiguration() *Config

	// IsApplicable returns true if the given resource inside the request is
	// applicable to this admission controller. Otherwise it returns false.
	IsApplicable(request *admissionv1beta1.AdmissionRequest) bool

	// IsExempt returns true if the given resource is exempt from being admitted.
	// Otherwise it returns false. On any error, response is set with appropriate
	// status and error message.
	// If response is not nil, the caller should not proceed with the admission.
	IsExempt(request *admissionv1beta1.AdmissionRequest) (exempt bool, response *admissionv1beta1.AdmissionResponse)

	// Admit makes an attempt to admit the specified resource in the request.
	// It returns an AdmissionResponse that is set appropriately. On success,
	// the response should contain the patch for update.
	Admit(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse
}

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewInClusterAdmission(kubeClientConfig *restclient.Config, stopCh <-chan struct{}) (admission Admission, err error) {
	configLoader := func() (config *Config, err error) {
		configPath := os.Getenv("CONFIGURATION_PATH")
		if configPath == "" {
			configPath = inClusterConfigFilePath
		}

		externalConfig, err := DecodeWithFile(inClusterConfigFilePath)
		if err != nil {
			return
		}

		config = ConvertExternalConfig(externalConfig)
		return
	}

	return NewAdmission(kubeClientConfig, stopCh, configLoader)
}

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewAdmission(kubeClientConfig *restclient.Config, stopCh <-chan struct{}, configLoaderFunc ConfigLoaderFunc) (admission Admission, err error) {
	config, err := configLoaderFunc()
	if err != nil {
		return
	}

	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return
	}

	factory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)
	informer := factory.Core().V1().Namespaces()
	nsLister := informer.Lister()

	nsInformer := informer.Informer()
	go nsInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, nsInformer.HasSynced) {
		err = errors.New("failed to wait for cache to sync")

		klog.V(1).Info(err.Error())
		return
	}

	limitRangesLister := factory.Core().V1().LimitRanges().Lister()

	admission = &clusterResourceOverrideAdmission{
		config:   config,
		nsLister: nsLister,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: limitRangesLister,
		},
	}

	return
}

func setNamespaceFloor(nsMinimum *Floor) *Floor {
	target := &Floor{
		Memory: &defaultMemoryFloor,
		CPU:    &defaultCPUFloor,
	}

	// floor associated with a namespace has higher precedence.
	if nsMinimum != nil {
		if nsMinimum.Memory != nil {
			target.Memory = nsMinimum.Memory
		}

		if nsMinimum.CPU != nil {
			target.CPU = nsMinimum.CPU
		}
	}

	return target
}

var (
	BadRequestErr = errors.New("unexpected object")
)

type clusterResourceOverrideAdmission struct {
	config       *Config
	nsLister     corev1listers.NamespaceLister
	limitQuerier *namespaceLimitQuerier
}

func (p *clusterResourceOverrideAdmission) GetConfiguration() *Config {
	return p.config
}

func (p *clusterResourceOverrideAdmission) IsApplicable(request *admissionv1beta1.AdmissionRequest) bool {
	if request.Resource.Resource == string(corev1.ResourcePods) &&
		request.SubResource == "" &&
		(request.Operation == admissionv1beta1.Create || request.Operation == admissionv1beta1.Update) {

		return true
	}

	return false
}

func (p *clusterResourceOverrideAdmission) IsExempt(request *admissionv1beta1.AdmissionRequest) (exempt bool, response *admissionv1beta1.AdmissionResponse) {
	klog.V(5).Infof("%s - checking if the resource is exempt", request.Namespace)

	pod, err := getPod(request)
	if err != nil {
		response = admissionresponse.WithBadRequest(request, err)
		return
	}

	klog.V(5).Infof("looking at pod %s in project %s", pod.Name, request.Namespace)

	// allow annotations on project to override
	ns, err := p.nsLister.Get(request.Namespace)
	if err != nil {
		klog.Warningf("got an error retrieving namespace: %v", err)
		response = admissionresponse.WithForbidden(request, err)
		return
	}

	projectEnabledPlugin, exists := ns.Annotations[clusterResourceOverrideAnnotation]
	if exists && projectEnabledPlugin != "true" {
		klog.V(5).Infof("namespace=%s skipping, namespace is not enabled", request.Namespace)

		exempt = true
		return
	}

	if IsNamespaceExempt(ns.Name) {
		klog.V(5).Infof("namespace=%s skipping exempt namespace", request.Namespace)

		exempt = true // project is exempted, do nothing
		return
	}

	return
}

func (p *clusterResourceOverrideAdmission) Admit(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	klog.V(5).Infof("%s - admitting resource", request.Namespace)

	pod, err := getPod(request)
	if err != nil {
		return admissionresponse.WithBadRequest(request, err)
	}

	// Don't mutate resource requirements below the namespace
	// limit minimums.
	nsMinimum, err := p.limitQuerier.QueryMinimum(request.Namespace)
	if err != nil {
		return admissionresponse.WithForbidden(request, err)
	}

	klog.V(5).Infof("initial pod limits are: %#v", pod.Spec)

	mutator, err := NewMutator(p.config, setNamespaceFloor(nsMinimum), cpuBaseScaleFactor)
	if err != nil {
		return admissionresponse.WithInternalServerError(request, err)
	}

	current, err := mutator.Mutate(pod)
	if err != nil {
		return admissionresponse.WithInternalServerError(request, err)
	}

	klog.V(5).Infof("pod limits after overrides are: %#v", current.Spec)

	patch, patchErr := Patch(request.Object, current)
	if patchErr != nil {
		return admissionresponse.WithInternalServerError(request, patchErr)
	}

	return admissionresponse.WithPatch(request, patch)
}

func getPod(request *admissionv1beta1.AdmissionRequest) (pod *corev1.Pod, err error) {
	pod = &corev1.Pod{}
	err = json.Unmarshal(request.Object.Raw, pod)
	return
}
