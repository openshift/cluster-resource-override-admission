package clusterresourceoverride

import (
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/openshift/cluster-resource-override-admission/pkg/api"
	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
)

const (
	Resource = "clusterresourceoverrides"
	Singular = "clusterresourceoverride"
	Name     = "clusterresourceoverride"
)

const (
	defaultResyncPeriod  = 5 * time.Hour
	configurationEnvName = "CONFIGURATION_PATH"
)

const (
	cpuBaseScaleFactor = 1000.0 / (1024.0 * 1024.0 * 1024.0) // 1000 milliCores per 1GiB
)

var (
	EnabledLabelName = fmt.Sprintf("%s.%s/enabled", Resource, api.Group)
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
		configPath := os.Getenv(configurationEnvName)
		if configPath == "" {
			err = fmt.Errorf("name=%s no configuration file specified, env var %s is not set", Name, configurationEnvName)
			return
		}

		externalConfig, decodeErr := DecodeWithFile(configPath)
		if decodeErr != nil {
			err = fmt.Errorf("name=%s file=%s failed to decode configuration - %s", Name, configPath, decodeErr.Error())
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
	config, configLoadErr := configLoaderFunc()
	if configLoadErr != nil {
		err = fmt.Errorf("name=%s failed to load configuration - %s", Name, configLoadErr.Error())
		return
	}

	client, clientErr := kubernetes.NewForConfig(kubeClientConfig)
	if clientErr != nil {
		err = fmt.Errorf("name=%s failed to load configuration - %s", Name, clientErr.Error())
		return
	}

	factory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)
	informer := factory.Core().V1().Namespaces()
	nsLister := informer.Lister()

	nsInformer := informer.Informer()
	go nsInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, nsInformer.HasSynced) {
		err = fmt.Errorf("name=%s failed to wait for cache to sync", Name)

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
	// we enforce an opt-in model.
	// all resource(s) are by default exempt unless the containing namespace has the right label.
	exempt = true

	ns, err := p.nsLister.Get(request.Namespace)
	if err != nil {
		klog.Warningf("namespace=%s error retrieving namespace: %v", request.Namespace, err)
		response = admissionresponse.WithForbidden(request, err)
		return
	}

	enabled, exists := ns.Labels[EnabledLabelName]
	if exists && enabled == "true" {
		klog.V(5).Infof("namespace=%s namespace is not exempt", request.Namespace)

		exempt = false
		return
	}

	klog.V(5).Infof("namespace=%s - namespace is exempt", request.Namespace)
	return
}

func (p *clusterResourceOverrideAdmission) Admit(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	klog.V(5).Infof("namespace=%s - admitting resource", request.Namespace)

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

	klog.V(5).Infof("namespace=%s initial pod limits are: %#v", request.Namespace, pod.Spec)

	mutator, err := NewMutator(p.config, setNamespaceFloor(nsMinimum), cpuBaseScaleFactor)
	if err != nil {
		return admissionresponse.WithInternalServerError(request, err)
	}

	current, err := mutator.Mutate(pod)
	if err != nil {
		return admissionresponse.WithInternalServerError(request, err)
	}

	klog.V(5).Infof("namespace=%s pod limits after overrides are: %#v", request.Namespace, current.Spec)

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
