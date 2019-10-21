package response

import (
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WithInternalServerError(request *admissionv1beta1.AdmissionRequest, err error) *admissionv1beta1.AdmissionResponse {
	return &admissionv1beta1.AdmissionResponse{
		UID:     request.UID,
		Allowed: false,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusInternalServerError,
			Reason:  metav1.StatusReasonInternalError,
			Message: err.Error(),
		},
	}
}

func WithAllowed(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	return &admissionv1beta1.AdmissionResponse{
		UID:     request.UID,
		Allowed: true,
	}
}
