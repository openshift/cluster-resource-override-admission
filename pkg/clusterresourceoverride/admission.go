package clusterresourceoverride

import (
	"errors"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"
	coreapi "k8s.io/kubernetes/pkg/apis/core"
)

const (
	PluginName                        = "autoscaling.openshift.io/ClusterResourceOverride"
	clusterResourceOverrideAnnotation = "autoscaling.openshift.io/cluster-resource-override-enabled"
	defaultResyncPeriod               = 5 * time.Hour
	inClusterConfigFilePath           = "/var/cluster-resource-override.yaml"
)

// ConfigLoaderFunc loads a Config object from appropriate source and returns it.
type ConfigLoaderFunc func() (config *Config, err error)

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewInClusterAdmission(kubeClientConfig *restclient.Config) (admission Admission, err error) {
	configLoader := func() (config *Config, err error) {
		configPath := os.Getenv("CONFIGURATION_PATH")
		if configPath == "" {
			configPath = inClusterConfigFilePath
		}

		externalConfig, err := DecodeWithFile(inClusterConfigFilePath)
		if err != nil {
			return
		}

		config = Convert(externalConfig)
		return
	}

	return NewAdmission(kubeClientConfig, configLoader)
}

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewAdmission(kubeClientConfig *restclient.Config, configLoaderFunc ConfigLoaderFunc) (admission Admission, err error) {
	config, err := configLoaderFunc()
	if err != nil {
		return
	}

	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return
	}

	factory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)
	limitRangesLister := factory.Core().V1().LimitRanges().Lister()
	nsLister := factory.Core().V1().Namespaces().Lister()

	admission = &clusterResourceOverrideAdmission{
		config:            config,
		nsLister:          nsLister,
		limitRangesLister: limitRangesLister,
	}

	return
}

// Admission interface encapsulates the admission logic for ClusterResourceOverride plugin.
type Admission interface {
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

var (
	BadRequestErr = errors.New("unexpected object")
)

type clusterResourceOverrideAdmission struct {
	config            *Config
	nsLister          corev1listers.NamespaceLister
	limitRangesLister corev1listers.LimitRangeLister
}

func (p *clusterResourceOverrideAdmission) IsApplicable(request *admissionv1beta1.AdmissionRequest) bool {
	if request.Resource.Resource == string(coreapi.ResourcePods) &&
		request.SubResource == "" &&
		(request.Operation == admissionv1beta1.Create || request.Operation == admissionv1beta1.Update) {

		return true
	}

	return false
}

func (p *clusterResourceOverrideAdmission) IsExempt(request *admissionv1beta1.AdmissionRequest) (exempt bool, response *admissionv1beta1.AdmissionResponse) {
	pod, ok := request.Object.Object.(*coreapi.Pod)
	if !ok {
		response = admissionresponse.WithBadRequest(request, BadRequestErr)
		return
	}

	klog.V(5).Infof("%s is looking at creating pod %s in project %s", PluginName, pod.Name, request.Namespace)

	// allow annotations on project to override
	ns, err := p.nsLister.Get(request.Namespace)
	if err != nil {
		klog.Warningf("%s got an error retrieving namespace: %v", PluginName, err)
		response = admissionresponse.WithForbidden(request, err)
		return
	}

	projectEnabledPlugin, exists := ns.Annotations[clusterResourceOverrideAnnotation]
	if exists && projectEnabledPlugin != "true" {
		klog.V(5).Infof("%s is disabled for project %s", PluginName, request.Namespace)
		exempt = true
		return
	}

	if isExemptedNamespace(ns.Name) {
		klog.V(5).Infof("%s is skipping exempted project %s", PluginName, request.Namespace)
		exempt = true // project is exempted, do nothing
		return
	}

	return
}

func (p *clusterResourceOverrideAdmission) Admit(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	pod, ok := request.Object.Object.(*coreapi.Pod)
	if !ok {
		return admissionresponse.WithBadRequest(request, BadRequestErr)
	}

	namespaceLimits := []*corev1.LimitRange{}

	if p.limitRangesLister != nil {
		limits, err := p.limitRangesLister.LimitRanges(request.Namespace).List(labels.Everything())
		if err != nil {
			return admissionresponse.WithForbidden(request, err)
		}
		namespaceLimits = limits
	}

	// Don't mutate resource requirements below the namespace
	// limit minimums.
	nsCPUFloor := minResourceLimits(namespaceLimits, corev1.ResourceCPU)
	nsMemFloor := minResourceLimits(namespaceLimits, corev1.ResourceMemory)

	klog.V(5).Infof("%s: initial pod limits are: %#v", PluginName, pod.Spec)

	mutator := newMutator(p.config, nsCPUFloor, nsMemFloor)
	current, err := mutator.Mutate(pod)
	if err != nil {
		return admissionresponse.WithInternalServerError(request, err)
	}

	klog.V(5).Infof("%s: pod limits after overrides are: %#v", PluginName, current.Spec)

	patch, patchErr := Patch(request.Object, current)
	if patchErr != nil {
		return admissionresponse.WithInternalServerError(request, patchErr)
	}

	return admissionresponse.WithPatch(request, patch)
}

// minResourceLimits finds the Min limit for resourceName. Nil is
// returned if limitRanges is empty or limits contains no resourceName
// limits.
func minResourceLimits(limitRanges []*corev1.LimitRange, resourceName corev1.ResourceName) *resource.Quantity {
	limits := []*resource.Quantity{}

	for _, limitRange := range limitRanges {
		for _, limit := range limitRange.Spec.Limits {
			if limit.Type == corev1.LimitTypeContainer {
				if limit, found := limit.Min[resourceName]; found {
					clone := limit.DeepCopy()
					limits = append(limits, &clone)
				}
			}
		}
	}

	if len(limits) == 0 {
		return nil
	}

	return minQuantity(limits)
}

func minQuantity(quantities []*resource.Quantity) *resource.Quantity {
	min := quantities[0].DeepCopy()

	for i := range quantities {
		if quantities[i].Cmp(min) < 0 {
			min = quantities[i].DeepCopy()
		}
	}

	return &min
}
