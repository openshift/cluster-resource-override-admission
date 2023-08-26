package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestSetNamespaceFloor(t *testing.T) {
	cpu := resource.MustParse("1000m")
	require.False(t, cpu.Equal(defaultCPUFloor), "bad test setup, default and namespace cpu floor must not be equal")

	memory := resource.MustParse("1Gi")
	require.False(t, memory.Equal(defaultMemoryFloor), "bad test setup, default and namespace memory floor must not be equal")

	namespaceFloor := &CPUMemory{
		CPU:    &cpu,
		Memory: &memory,
	}

	floorGot := setNamespaceFloor(namespaceFloor)
	require.NotNil(t, floorGot)

	assert.True(t, cpu.Equal(*floorGot.CPU))
	assert.True(t, memory.Equal(*floorGot.Memory))
}

// TestOverrideHookUpdatesNotApplicable tests to make sure that admission
// regards UPDATE requests to a pod as not applicable, as currently kubernetes
// doesn't allow the resource fields to be updated in-place
func TestAdmissionUpdateRequestsNotApplicable(t *testing.T) {
	admission := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation:   "UPDATE",
		Resource:    metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
		SubResource: "",
	}
	applicable := admission.IsApplicable(req)
	assert.False(t, applicable)
}
