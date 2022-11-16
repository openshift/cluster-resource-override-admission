package main

import (
	"testing"

	"github.com/openshift/generic-admission-server/pkg/apiserver"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"
)

func TestMutatingHook_MutatingResource(t *testing.T) {
	pluralWant := schema.GroupVersionResource{
		Group:    "admission.node.openshift.io",
		Version:  "v1",
		Resource: "podsvtoverride",
	}
	singularWant := "podsvtoverride"

	var hook apiserver.MutatingAdmissionHook = &podSvtRelabel{}

	pluralGot, singularGot := hook.MutatingResource()

	assert.Equal(t, pluralWant, pluralGot)
	assert.Equal(t, singularWant, singularGot)
}
