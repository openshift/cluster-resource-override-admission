package clusterresourceoverride

import (
	"math"
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

func TestConfigValidate(t *testing.T) {
	valid := func() *Config {
		return &Config{
			CpuRequestToLimitRatio:    0.5,
			CpuRequestToRequestRatio:  0.5,
			MemoryRequestToLimitRatio: 0.5,
			LimitCPUToMemoryRatio:     2.0,
		}
	}

	// Accepted. The three request ratios are valid across the whole [0,1] range.
	// LimitCPUToMemoryRatio has no upper bound: it mirrors the operator CRD, which only
	// requires >= 0, so an arbitrarily large finite value is accepted here (and saturated
	// by the CPU arithmetic downstream) rather than rejected at a bound the operator would
	// not enforce.
	accepted := []struct {
		name   string
		mutate func(*Config)
	}{
		{"midrange", func(c *Config) {}},
		{"request ratios at zero", func(c *Config) {
			c.CpuRequestToLimitRatio, c.CpuRequestToRequestRatio, c.MemoryRequestToLimitRatio = 0, 0, 0
		}},
		{"request ratios at one", func(c *Config) {
			c.CpuRequestToLimitRatio, c.CpuRequestToRequestRatio, c.MemoryRequestToLimitRatio = 1, 1, 1
		}},
		{"limit-to-memory ratio zero", func(c *Config) { c.LimitCPUToMemoryRatio = 0 }},
		{"limit-to-memory ratio very large finite", func(c *Config) { c.LimitCPUToMemoryRatio = math.MaxFloat64 }},
	}
	for _, tc := range accepted {
		t.Run("accepted/"+tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(c)
			assert.NoError(t, c.Validate())
		})
	}

	// Rejected. Each error names the offending field so an operator can locate it in the
	// configuration file.
	rejected := []struct {
		name   string
		field  string
		mutate func(*Config)
	}{
		{"request ratio just above one", "cpuRequestToLimitRatio", func(c *Config) { c.CpuRequestToLimitRatio = math.Nextafter(1, 2) }},
		{"request ratio just below zero", "cpuRequestToRequestRatio", func(c *Config) { c.CpuRequestToRequestRatio = -math.SmallestNonzeroFloat64 }},
		{"request ratio NaN", "memoryRequestToLimitRatio", func(c *Config) { c.MemoryRequestToLimitRatio = math.NaN() }},
		{"request ratio +Inf", "cpuRequestToLimitRatio", func(c *Config) { c.CpuRequestToLimitRatio = math.Inf(1) }},
		{"request ratio -Inf", "cpuRequestToRequestRatio", func(c *Config) { c.CpuRequestToRequestRatio = math.Inf(-1) }},
		{"limit-to-memory ratio negative", "limitCPUToMemoryRatio", func(c *Config) { c.LimitCPUToMemoryRatio = -math.SmallestNonzeroFloat64 }},
		{"limit-to-memory ratio NaN", "limitCPUToMemoryRatio", func(c *Config) { c.LimitCPUToMemoryRatio = math.NaN() }},
		{"limit-to-memory ratio +Inf", "limitCPUToMemoryRatio", func(c *Config) { c.LimitCPUToMemoryRatio = math.Inf(1) }},
	}
	for _, tc := range rejected {
		t.Run("rejected/"+tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(c)
			err := c.Validate()
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.field, "error should name the offending field")
			}
		})
	}
}
