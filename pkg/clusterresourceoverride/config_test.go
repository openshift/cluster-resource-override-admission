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
			CPURequestToRequestPercent:  25,
		},
	}

	configGot := ConvertExternalConfig(external)
	assert.NotNil(t, configGot)
	assert.Equal(t, 4.0, configGot.LimitCPUToMemoryRatio)
	assert.Equal(t, 0.25, configGot.CpuRequestToLimitRatio)
	assert.Equal(t, 0.50, configGot.MemoryRequestToLimitRatio)
	assert.Equal(t, 0.25, configGot.CpuRequestToRequestRatio)
}

func TestConvertExternalConfig_ZeroValues(t *testing.T) {
	external := &ClusterResourceOverride{
		Spec: ClusterResourceOverrideSpec{},
	}

	configGot := ConvertExternalConfig(external)
	assert.NotNil(t, configGot)
	assert.Equal(t, 0.0, configGot.LimitCPUToMemoryRatio)
	assert.Equal(t, 0.0, configGot.CpuRequestToLimitRatio)
	assert.Equal(t, 0.0, configGot.MemoryRequestToLimitRatio)
	assert.Equal(t, 0.0, configGot.CpuRequestToRequestRatio)
	assert.False(t, configGot.ForceSelinuxRelabel)
}

func TestConvertExternalConfig_ForceSelinuxRelabel(t *testing.T) {
	external := &ClusterResourceOverride{
		Spec: ClusterResourceOverrideSpec{
			ForceSelinuxRelabel: true,
		},
	}

	configGot := ConvertExternalConfig(external)
	assert.True(t, configGot.ForceSelinuxRelabel)
}

func TestConvertExternalConfig_NegativeValues(t *testing.T) {
	external := &ClusterResourceOverride{
		Spec: ClusterResourceOverrideSpec{
			CPURequestToLimitPercent:    -50,
			MemoryRequestToLimitPercent: -25,
		},
	}

	configGot := ConvertExternalConfig(external)
	assert.Equal(t, -0.5, configGot.CpuRequestToLimitRatio)
	assert.Equal(t, -0.25, configGot.MemoryRequestToLimitRatio)
}

func TestConvertExternalConfig_LargeValues(t *testing.T) {
	external := &ClusterResourceOverride{
		Spec: ClusterResourceOverrideSpec{
			LimitCPUToMemoryPercent: 100000,
		},
	}

	configGot := ConvertExternalConfig(external)
	assert.Equal(t, 1000.0, configGot.LimitCPUToMemoryRatio)
}

func TestConfigString(t *testing.T) {
	config := &Config{
		LimitCPUToMemoryRatio:     0.5,
		CpuRequestToLimitRatio:    0.25,
		MemoryRequestToLimitRatio: 0.5,
		CpuRequestToRequestRatio:  0.3,
		ForceSelinuxRelabel:       true,
	}

	s := config.String()
	assert.Contains(t, s, "LimitCPUToMemoryRatio=0.5")
	assert.Contains(t, s, "CpuRequestToLimitRatio=0.25")
	assert.Contains(t, s, "MemoryRequestToLimitRatio=0.5")
	assert.Contains(t, s, "CpuRequestToRequestRatio=0.3")
	assert.Contains(t, s, "ForceSelinuxRelabel=true")
}

func TestDecodeWithFile_NonExistentFile(t *testing.T) {
	_, err := DecodeWithFile("testdata/does-not-exist.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to load file")
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
				assert.Equal(t, int64(25), objGot.Spec.CPURequestToRequestPercent)
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
