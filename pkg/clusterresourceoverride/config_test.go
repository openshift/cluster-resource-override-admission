package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertExternalConfig(t *testing.T) {
	external := &ClusterResourceOverrideConfig{
		LimitCPUToMemoryPercent:     400,
		CPURequestToLimitPercent:    25,
		MemoryRequestToLimitPercent: 50,
	}

	configGot := ConvertExternalConfig(external)
	assert.NotNil(t, configGot)
	assert.Equal(t, 4.0, configGot.LimitCPUToMemoryRatio)
	assert.Equal(t, 0.25, configGot.CpuRequestToLimitRatio)
	assert.Equal(t, 0.50, configGot.MemoryRequestToLimitRatio)
}

func TestDecodeWithFile(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		assert func(t *testing.T, objGot *ClusterResourceOverrideConfig, errGot error)
	}{
		{
			name: "WithValidObject",
			file: "testdata/external.yaml",
			assert: func(t *testing.T, objGot *ClusterResourceOverrideConfig, errGot error) {
				assert.NoError(t, errGot)
				assert.NotNil(t, objGot)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objGot, errGot := DecodeWithFile(tt.file)

			tt.assert(t, objGot, errGot)
		})
	}
}
