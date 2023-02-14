package clusterresourceoverride

import (
	"fmt"
	"io"
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
}

type Config struct {
	ForceSelinuxRelabel       bool
	LimitCPUToMemoryRatio     float64
	CpuRequestToLimitRatio    float64
	MemoryRequestToLimitRatio float64
}

func (c *Config) String() string {
	return fmt.Sprintf("LimitCPUToMemoryRatio=%f CpuRequestToLimitRatio=%f MemoryRequestToLimitRatio=%f ForceSelinuxRelabel=%v",
		c.LimitCPUToMemoryRatio, c.CpuRequestToLimitRatio, c.MemoryRequestToLimitRatio, c.ForceSelinuxRelabel)
}

func ConvertExternalConfig(object *ClusterResourceOverride) *Config {
	return &Config{
		ForceSelinuxRelabel:       object.Spec.ForceSelinuxRelabel,
		LimitCPUToMemoryRatio:     float64(object.Spec.LimitCPUToMemoryPercent) / 100,
		CpuRequestToLimitRatio:    float64(object.Spec.CPURequestToLimitPercent) / 100,
		MemoryRequestToLimitRatio: float64(object.Spec.MemoryRequestToLimitPercent) / 100,
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
	if err != nil {
		err = fmt.Errorf("unable to load file %s: %s", path, openErr)
		return
	}

	object, err = Decode(reader)
	return
}
