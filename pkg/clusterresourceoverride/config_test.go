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
	// requires >= 0, so a large value (up to what the int64 external percentage can express)
	// is accepted here rather than rejected at a bound the operator would not enforce. This
	// PR validates the configured ranges only; overflow-safe arithmetic for a large
	// CPU-to-memory percentage is handled in #113.
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
		{"limit-to-memory ratio at the external maximum", func(c *Config) { c.LimitCPUToMemoryRatio = float64(math.MaxInt64) / 100 }},
	}
	for _, tc := range accepted {
		t.Run("accepted/"+tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(c)
			assert.NoError(t, c.Validate())
		})
	}

	// Rejected. Each error names the offending internal ratio field (not the YAML percentage
	// key it was derived from).
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
