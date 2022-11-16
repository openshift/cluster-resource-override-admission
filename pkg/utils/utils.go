package utils

import (
	"encoding/json"

	jsonpatch "gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Patch takes 2 byte arrays and returns a new response with json patch.
// The original object should be passed in as raw bytes to avoid the roundtripping problem
// described in https://github.com/kubernetes-sigs/kubebuilder/issues/510.
func Patch(original runtime.RawExtension, mutated *corev1.Pod) (patches []byte, err error) {
	current, marshalErr := json.Marshal(mutated)
	if marshalErr != nil {
		err = marshalErr
		return
	}

	operations, patchErr := jsonpatch.CreatePatch(original.Raw, current)
	if patchErr != nil {
		err = patchErr
		return
	}

	patchBytes, marshalErr := json.Marshal(operations)
	if marshalErr != nil {
		err = marshalErr
		return
	}

	patches = patchBytes
	return
}

func GetPod(request *admissionv1.AdmissionRequest) (pod *corev1.Pod, err error) {
	pod = &corev1.Pod{}
	err = json.Unmarshal(request.Object.Raw, pod)
	return
}
