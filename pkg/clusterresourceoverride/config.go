package clusterresourceoverride

import (
	"fmt"
	"io"
	"math"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// ClusterResourceOverride is the configuration for the ClusterResourceOverride
// admission controller which overrides user-provided container request/limit values.
type ClusterResourceOverride struct {
	metav1.TypeMeta `json:",inline"`
	Spec            ClusterResourceOverrideSpec `json:"spec,omitempty"`
}

type ClusterResourceOverrideSpec struct {
	// For each of the following, if a non-zero ratio is specified then the initial
	// value (if any) in the pod spec is overwritten according to the ratio.
	// LimitRange defaults are merged prior to the override.
	//

	// ForceSelinuxRelabel (if true) label pods with spc_t if they have a PVC
	ForceSelinuxRelabel bool `json:"forceSelinuxRelabel"`

	// LimitCPUToMemoryPercent (if > 0) overrides the CPU limit to a ratio of the memory limit;
	// 100% overrides CPU to 1 core per 1GiB of RAM. This is done before overriding the CPU request.
	LimitCPUToMemoryPercent int64 `json:"limitCPUToMemoryPercent"`

	// CPURequestToLimitPercent (if > 0) overrides CPU request to a percentage of CPU limit
	CPURequestToLimitPercent int64 `json:"cpuRequestToLimitPercent"`

	// MemoryRequestToLimitPercent (if > 0) overrides memory request to a percentage of memory limit
	MemoryRequestToLimitPercent int64 `json:"memoryRequestToLimitPercent"`

	// CPURequestToRequestPercent (if > 0) overrides CPU request to a percentage of the
	// existing CPU request.
	CPURequestToRequestPercent int64 `json:"cpuRequestToRequestPercent"`
}

type Config struct {
	ForceSelinuxRelabel       bool
	LimitCPUToMemoryRatio     float64
	CpuRequestToLimitRatio    float64
	MemoryRequestToLimitRatio float64
	CpuRequestToRequestRatio  float64
}

func (c *Config) String() string {
	return fmt.Sprintf("LimitCPUToMemoryRatio=%f CpuRequestToLimitRatio=%f MemoryRequestToLimitRatio=%f CpuRequestToRequestRatio=%f ForceSelinuxRelabel=%v",
		c.LimitCPUToMemoryRatio, c.CpuRequestToLimitRatio, c.MemoryRequestToLimitRatio, c.CpuRequestToRequestRatio, c.ForceSelinuxRelabel)
}

// Validate rejects a Config whose ratios are not finite and in range so an
// invalid configuration (for example a hand-edited configuration file the
// operator did not produce) fails fast at load time instead of silently
// producing wrong requests at admission. The three request ratios are fractions
// of another value and must be in [0,1]; LimitCPUToMemoryRatio scales memory to
// CPU (100% is 1 core per GiB) and only has to be finite and non-negative.
func (c *Config) Validate() error {
	for _, r := range []struct {
		name  string
		ratio float64
	}{
		{"cpuRequestToLimitRatio", c.CpuRequestToLimitRatio},
		{"cpuRequestToRequestRatio", c.CpuRequestToRequestRatio},
		{"memoryRequestToLimitRatio", c.MemoryRequestToLimitRatio},
	} {
		if math.IsNaN(r.ratio) || math.IsInf(r.ratio, 0) || r.ratio < 0 || r.ratio > 1 {
			return fmt.Errorf("%s must be a finite value in [0,1], got %v", r.name, r.ratio)
		}
	}
	if r := c.LimitCPUToMemoryRatio; math.IsNaN(r) || math.IsInf(r, 0) || r < 0 {
		return fmt.Errorf("limitCPUToMemoryRatio must be a finite non-negative value, got %v", r)
	}
	return nil
}

func ConvertExternalConfig(object *ClusterResourceOverride) *Config {
	return &Config{
		ForceSelinuxRelabel:       object.Spec.ForceSelinuxRelabel,
		LimitCPUToMemoryRatio:     float64(object.Spec.LimitCPUToMemoryPercent) / 100,
		CpuRequestToLimitRatio:    float64(object.Spec.CPURequestToLimitPercent) / 100,
		MemoryRequestToLimitRatio: float64(object.Spec.MemoryRequestToLimitPercent) / 100,
		CpuRequestToRequestRatio:  float64(object.Spec.CPURequestToRequestPercent) / 100,
	}
}

// DecodeUnstructured decodes a raw stream into a an
// unstructured.Unstructured instance.
func Decode(reader io.Reader) (object *ClusterResourceOverride, err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(reader, 30)

	c := &ClusterResourceOverride{}
	if err = decoder.Decode(c); err != nil {
		return
	}

	object = c
	return
}

func DecodeWithFile(path string) (object *ClusterResourceOverride, err error) {
	reader, openErr := os.Open(path)
	if openErr != nil {
		err = fmt.Errorf("unable to load file %s: %s", path, openErr)
		return
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("unable to close file %s: %w", path, closeErr)
		}
	}()

	object, err = Decode(reader)
	return
}
