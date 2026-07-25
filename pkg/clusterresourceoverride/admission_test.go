package clusterresourceoverride

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewAdmissionValidatesConfiguration(t *testing.T) {
	// NewAdmission is the single boundary every ConfigLoaderFunc passes through, so an
	// invalid config must be rejected there, before any API client is built, regardless of
	// which loader produced it.
	t.Run("rejects an out-of-range ratio", func(t *testing.T) {
		loader := func() (*Config, error) {
			return &Config{CpuRequestToLimitRatio: 1.5}, nil
		}
		admission, err := NewAdmission(nil, nil, loader)
		require.Error(t, err)
		require.Nil(t, admission)
		assert.Contains(t, err.Error(), "invalid configuration")
		assert.Contains(t, err.Error(), "cpuRequestToLimitRatio")
	})

	t.Run("rejects a nil loader", func(t *testing.T) {
		admission, err := NewAdmission(nil, nil, nil)
		require.Error(t, err)
		require.Nil(t, admission)
		assert.Contains(t, err.Error(), "loader is nil")
	})

	t.Run("rejects a nil config from the loader", func(t *testing.T) {
		loader := func() (*Config, error) { return nil, nil }
		admission, err := NewAdmission(nil, nil, loader)
		require.Error(t, err)
		require.Nil(t, admission)
		assert.Contains(t, err.Error(), "nil config")
	})
}

func TestNewInClusterAdmissionRejectsInvalidConfigFile(t *testing.T) {
	// A hand-edited configuration file with an out-of-range percentage must fail startup
	// before the API client is created, so no cluster is needed here.
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "apiVersion: v1\nkind: ClusterResourceOverrideConfig\nspec:\n  cpuRequestToLimitPercent: 101\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	t.Setenv(configurationEnvName, path)

	admission, err := NewInClusterAdmission(nil, nil)
	require.Error(t, err)
	require.Nil(t, admission)
	assert.Contains(t, err.Error(), "invalid configuration")
	assert.Contains(t, err.Error(), "cpuRequestToLimitRatio")
}

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
