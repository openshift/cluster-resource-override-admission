package clusterresourceoverride

import (
	"fmt"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// ClusterResourceOverrideConfig is the configuration for the ClusterResourceOverride
// admission controller which overrides user-provided container request/limit values.
type ClusterResourceOverrideConfig struct {
	metav1.TypeMeta `json:",inline"`

	// For each of the following, if a non-zero ratio is specified then the initial
	// value (if any) in the pod spec is overwritten according to the ratio.
	// LimitRange defaults are merged prior to the override.
	//

	// LimitCPUToMemoryPercent (if > 0) overrides the CPU limit to a ratio of the memory limit;
	// 100% overrides CPU to 1 core per 1GiB of RAM. This is done before overriding the CPU request.
	LimitCPUToMemoryPercent int64 `json:"limitCPUToMemoryPercent"`

	// CPURequestToLimitPercent (if > 0) overrides CPU request to a percentage of CPU limit
	CPURequestToLimitPercent int64 `json:"cpuRequestToLimitPercent"`

	// MemoryRequestToLimitPercent (if > 0) overrides memory request to a percentage of memory limit
	MemoryRequestToLimitPercent int64 `json:"memoryRequestToLimitPercent"`
}

type Config struct {
	LimitCPUToMemoryRatio     float64
	CpuRequestToLimitRatio    float64
	MemoryRequestToLimitRatio float64
}

func Convert(config *ClusterResourceOverrideConfig) *Config {
	return &Config{
		LimitCPUToMemoryRatio:     float64(config.LimitCPUToMemoryPercent) / 100,
		CpuRequestToLimitRatio:    float64(config.CPURequestToLimitPercent) / 100,
		MemoryRequestToLimitRatio: float64(config.MemoryRequestToLimitPercent) / 100,
	}
}

// DecodeUnstructured decodes a raw stream into a an
// unstructured.Unstructured instance.
func Decode(reader io.Reader) (config *ClusterResourceOverrideConfig, err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(reader, 30)

	c := &ClusterResourceOverrideConfig{}
	if err = decoder.Decode(c); err != nil {
		return
	}

	config = c
	return
}

func DecodeWithFile(path string) (config *ClusterResourceOverrideConfig, err error) {
	reader, openErr := os.Open(path)
	if err != nil {
		err = fmt.Errorf("unable to load file %s: %s", path, openErr)
		return
	}

	config, err = Decode(reader)
	return
}
