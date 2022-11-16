package podsvtrelabel

import (
	"fmt"
	"time"

	"k8s.io/client-go/tools/cache"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/openshift/cluster-resource-override-admission/pkg/api"
	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
	"github.com/openshift/cluster-resource-override-admission/pkg/utils"
)

const (
	Resource = "podsvtoverride"
	Singular = "podsvtoverride"
	Name     = "podsvtoverride"
)

const (
	defaultResyncPeriod = 5 * time.Hour
	SpcType             = "spc_t"
)

var (
	EnabledLabelName = fmt.Sprintf("%s.%s/enabled", Resource, api.SVTRelabelGroup)
)

// Admission interface encapsulates the admission logic for ClusterResourceOverride plugin.
type Admission interface {
	// IsApplicable returns true if the given resource inside the request is
	// applicable to this admission controller. Otherwise it returns false.
	IsApplicable(request *admissionv1.AdmissionRequest) bool

	// IsExempt returns true if the given resource is exempt from being admitted.
	// Otherwise it returns false. On any error, response is set with appropriate
	// status and error message.
	// If response is not nil, the caller should not proceed with the admission.
	IsExempt(request *admissionv1.AdmissionRequest) (exempt bool, response *admissionv1.AdmissionResponse)

	// Admit makes an attempt to admit the specified resource in the request.
	// It returns an AdmissionResponse that is set appropriately. On success,
	// the response should contain the patch for update.
	Admit(admissionSpec *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse
}

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewInClusterAdmission(kubeClientConfig *restclient.Config, stopCh <-chan struct{}) (admission Admission, err error) {
	return NewAdmission(kubeClientConfig, stopCh)
}

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewAdmission(kubeClientConfig *restclient.Config, stopCh <-chan struct{}) (admission Admission, err error) {
	client, clientErr := kubernetes.NewForConfig(kubeClientConfig)
	if clientErr != nil {
		err = fmt.Errorf("name=%s failed to load configuration - %s", Name, clientErr.Error())
		return
	}

	factory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)

	namespaces := factory.Core().V1().Namespaces()
	nsInformer := namespaces.Informer()
	go nsInformer.Run(stopCh)

	limitRanges := factory.Core().V1().LimitRanges()
	limitRangeInformer := limitRanges.Informer()
	go limitRangeInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, nsInformer.HasSynced) {
		err = fmt.Errorf("name=%s failed to wait for Namespace informer cache to sync", Name)
		return
	}

	if !cache.WaitForCacheSync(stopCh, limitRangeInformer.HasSynced) {
		err = fmt.Errorf("name=%s failed to wait for LimitRange informer cache to sync", Name)
		return
	}

	admission = &podSVTOverride{
		nsLister: namespaces.Lister(),
	}

	return
}

type podSVTOverride struct {
	nsLister corev1listers.NamespaceLister
}

func (p *podSVTOverride) IsApplicable(request *admissionv1.AdmissionRequest) bool {
	if request.Resource.Resource == string(corev1.ResourcePods) &&
		request.SubResource == "" &&
		(request.Operation == admissionv1.Create || request.Operation == admissionv1.Update) {

		return true
	}

	return false
}

func (p *podSVTOverride) IsExempt(request *admissionv1.AdmissionRequest) (exempt bool, response *admissionv1.AdmissionResponse) {
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

func (p *podSVTOverride) Admit(request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	namespace := request.Namespace
	klog.V(5).Infof("namespace=%s - admitting resource", namespace)

	currentPod, err := utils.GetPod(request)
	if err != nil {
		return admissionresponse.WithBadRequest(request, err)
	}

	// Find if the persistent volume claim exists
	foundPersistentVolumeClaim := false
	for _, volume := range currentPod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			foundPersistentVolumeClaim = true
			break
		}
	}

	// Return without modification if the PVC does not exist
	if !foundPersistentVolumeClaim {
		klog.V(5).Infof("namespace=%s - pod=%s admitted without modification ", namespace, currentPod.Name)
		return admissionresponse.WithAllowed(request)
	}

	// PVC exists so modify the pod with the spc_t label
	if currentPod.Spec.SecurityContext == nil {
		currentPod.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	currentPod.Spec.SecurityContext.SELinuxOptions = &corev1.SELinuxOptions{Type: SpcType}

	klog.V(5).Infof("namespace=%s - pod=%s admitted with modification ", namespace, currentPod.Name)

	patch, patchErr := utils.Patch(request.Object, currentPod)
	if patchErr != nil {
		return admissionresponse.WithInternalServerError(request, patchErr)
	}

	return admissionresponse.WithPatch(request, patch)
}
