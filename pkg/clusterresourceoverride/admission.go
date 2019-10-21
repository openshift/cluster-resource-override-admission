package clusterresourceoverride

import (
	"errors"

	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"
	coreapi "k8s.io/kubernetes/pkg/apis/core"
)

const (
	PluginName                        = "autoscaling.openshift.io/ClusterResourceOverride"
	clusterResourceOverrideAnnotation = "autoscaling.openshift.io/cluster-resource-override-enabled"
)

// NewInClusterAdmission returns a new instance of Admission that is appropriate
// to be consumed in cluster.
func NewInClusterAdmission(kubeClientConfig *restclient.Config) (admission Admission, err error) {
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
	nsLister corev1listers.NamespaceLister
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
	return nil
}
