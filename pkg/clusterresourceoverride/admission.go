package clusterresourceoverride

import (
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	restclient "k8s.io/client-go/rest"
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
