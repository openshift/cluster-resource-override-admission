package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertExternalConfig(t *testing.T) {
	external := &ClusterResourceOverride{
		Spec: ClusterResourceOverrideSpec{
			LimitCPUToMemoryPercent:     400,
			CPURequestToLimitPercent:    25,
			MemoryRequestToLimitPercent: 50,
		},
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
		assert func(t *testing.T, objGot *ClusterResourceOverride, errGot error)
	}{
		{
			name: "WithValidObject",
			file: "testdata/external.yaml",
			assert: func(t *testing.T, objGot *ClusterResourceOverride, errGot error) {
				assert.NoError(t, errGot)
				assert.NotNil(t, objGot)

				assert.Equal(t, int64(25), objGot.Spec.MemoryRequestToLimitPercent)
				assert.Equal(t, int64(50), objGot.Spec.CPURequestToLimitPercent)
				assert.Equal(t, int64(200), objGot.Spec.LimitCPUToMemoryPercent)
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
