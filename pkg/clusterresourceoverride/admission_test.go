package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
