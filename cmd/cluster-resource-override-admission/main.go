package main

import (
	"errors"
	"sync"

	"k8s.io/klog"

	"github.com/openshift/generic-admission-server/pkg/cmd"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"

	admissionresponse "github.com/openshift/cluster-resource-override-admission/pkg/response"
)

func main() {
	cmd.RunAdmissionServer(&mutatingHook{})
}

type mutatingHook struct {
	lock        sync.RWMutex
	initialized bool
}

// Initialize is called as a post-start hook
func (m *mutatingHook) Initialize(kubeClientConfig *restclient.Config, stopCh <-chan struct{}) error {
	m.lock.Lock()
	defer func() {
		m.initialized = true
		m.lock.Unlock()
	}()

	if m.initialized {
		return nil
	}

	klog.V(5).Info("loaded successfully")

	return nil
}

// MutatingResource is the resource to use for hosting your admission webhook. If the hook implements
// ValidatingAdmissionHook as well, the two resources for validating and mutating admission must be different.
// Note: this is (usually) not the same as the payload resource!
func (m *mutatingHook) MutatingResource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "admission.autoscaling.openshift.io",
			Version:  "v1",
			Resource: "clusterresourceoverrides",
		},
		"clusterresourceoverride"
}

// Admit is called to decide whether to accept the admission request. The returned AdmissionResponse may
// use the Patch field to mutate the object from the passed AdmissionRequest.
func (m *mutatingHook) Admit(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if !m.initialized {
		return admissionresponse.WithInternalServerError(request, errors.New("not initialized"))
	}

	return admissionresponse.WithAllowed(request)
}