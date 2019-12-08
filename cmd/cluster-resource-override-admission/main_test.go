package main

import (
	"testing"

	"github.com/openshift/generic-admission-server/pkg/apiserver"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"
)

func TestMutatingHook_MutatingResource(t *testing.T) {
	pluralWant := schema.GroupVersionResource{
		Group:    "admission.autoscaling.openshift.io",
		Version:  "v1",
		Resource: "clusterresourceoverrides",
	}
	singularWant := "clusterresourceoverride"

	var hook apiserver.MutatingAdmissionHook = &clusterResourceOverrideHook{}

	pluralGot, singularGot := hook.MutatingResource()

	assert.Equal(t, pluralWant, pluralGot)
	assert.Equal(t, singularWant, singularGot)
}
