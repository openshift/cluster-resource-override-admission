package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	factor = 1000.0 / (1024.0 * 1024.0 * 1024.0) // 1000 milliCores per 1GiB
)

func TestMutator_Mutate(t *testing.T) {
	cpu := resource.MustParse("1m")
	memory := resource.MustParse("1Mi")
	floor := &CPUMemory{
		CPU:    &cpu,
		Memory: &memory,
	}
	config := &Config{
		LimitCPUToMemoryRatio:     2.0,
		CpuRequestToLimitRatio:    0.25,
		MemoryRequestToLimitRatio: 0.5,
	}
	mutator, err := NewMutator(config, floor, &CPUMemory{}, factor)
	require.NoError(t, err)
	require.NotNil(t, mutator)

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "db",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("16Gi"),
							corev1.ResourceCPU:    resource.MustParse("8000m"),
						},
					},
				},
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("2Gi"),
							corev1.ResourceCPU:    resource.MustParse("2000m"),
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "init",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
							corev1.ResourceCPU:    resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}

	podGot, errGot := mutator.Mutate(pod)

	assert.NoError(t, errGot)
	assert.NotNil(t, podGot)

	// verify init container
	validate(t, podGot.Spec.InitContainers[0].Resources.Requests, corev1.ResourceMemory, resource.MustParse("512Mi"))
	validate(t, podGot.Spec.InitContainers[0].Resources.Limits, corev1.ResourceCPU, resource.MustParse("2000m"))
	validate(t, podGot.Spec.InitContainers[0].Resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))

	// verify db container
	validate(t, podGot.Spec.Containers[0].Resources.Requests, corev1.ResourceMemory, resource.MustParse("8Gi"))
	validate(t, podGot.Spec.Containers[0].Resources.Limits, corev1.ResourceCPU, resource.MustParse("32000m"))
	validate(t, podGot.Spec.Containers[0].Resources.Requests, corev1.ResourceCPU, resource.MustParse("8000m"))

	// verify app container
	validate(t, podGot.Spec.Containers[1].Resources.Requests, corev1.ResourceMemory, resource.MustParse("1Gi"))
	validate(t, podGot.Spec.Containers[1].Resources.Limits, corev1.ResourceCPU, resource.MustParse("4000m"))
	validate(t, podGot.Spec.Containers[1].Resources.Requests, corev1.ResourceCPU, resource.MustParse("1000m"))
}

func TestMutator_OverrideMemory(t *testing.T) {
	tests := []struct {
		name    string
		mutator func() *podMutator
		input   *corev1.ResourceRequirements
		assert  func(t *testing.T, resources *corev1.ResourceRequirements)
	}{
		{
			// memory floor is not specified.
			// MemoryRequestToLimitRatio is specified in config.
			name: "WithNoMemoryRequest",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						MemoryRequestToLimitRatio: 0.5,
					},
				}
			},
			// memory request is not specified in resources.
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceMemory, resource.MustParse("1Gi"))
			},
		},
		{
			// rounding to the floor value expected.
			name: "WithRounding",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						MemoryRequestToLimitRatio: 0.50,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("3Mi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceMemory, resource.MustParse("1Mi"))
			},
		},
		{
			// memory floor is not specified.
			// MemoryRequestToLimitRatio is specified in config.
			name: "WithMemoryRequest",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						MemoryRequestToLimitRatio: 0.5,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				// memory request is specified in resources, it will get overridden.
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceMemory, resource.MustParse("1Gi"))
			},
		},
		{
			// memory floor is specified.
			// MemoryRequestToLimitRatio is specified in config.
			// resources.limit.memory=4Gi, floor.memory=4Gi,
			// resources.request.memory is expected to be above the floor threshold.
			name: "WithMemoryRequestBelowTheFloor",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						MemoryRequestToLimitRatio: 0.5,
					},
					floor: &CPUMemory{
						Memory: func() *resource.Quantity {
							q := resource.MustParse("4Gi")
							return &q
						}(),
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("6Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceMemory, resource.MustParse("4Gi"))
			},
		},
		{
			// resources.limit.memory is not specified, no changes expected.
			name: "WithResourceLimitNotSpecified",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						MemoryRequestToLimitRatio: 0.5,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceMemory, resource.MustParse("2Gi"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()

			target.OverrideMemory(test.input)

			test.assert(t, test.input)
		})
	}
}

func TestMutator_OverrideCpu(t *testing.T) {
	tests := []struct {
		name    string
		mutator func() *podMutator
		input   *corev1.ResourceRequirements
		assert  func(t *testing.T, resources *corev1.ResourceRequirements)
	}{
		{
			// cpu floor is not specified.
			// CpuRequestToLimitRatio is specified in config.
			name: "WithNoCpuRequest",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToLimitRatio: 0.5,
					},
				}
			},
			// cpu request is not specified in resources.
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("1000m"))
			},
		},
		{
			// cpu floor is not specified.
			// CpuRequestToLimitRatio is specified in config.
			name: "WithCpuRequest",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToLimitRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2000m"),
				},
				// cpu request is specified in resources, it will get overridden.
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// cpu floor is specified.
			// CpuRequestToLimitRatio: 0.10, is specified in config.
			// resources.limit.cpu=1000m, floor.cpu=250m,
			// resources.request.memory is expected to be above the floor threshold.
			name: "WithCpuRequestBelowTheFloor",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToLimitRatio: 0.10,
					},
					floor: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("250m")
							return &q
						}(),
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("250m"))
			},
		},
		{
			// resources.limit.cpu is not specified, no changes expected.
			name: "WithResourceLimitNotSpecified",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToLimitRatio: 0.10,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("1000m"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()

			target.OverrideCPU(test.input)

			test.assert(t, test.input)
		})
	}
}

func TestMutator_OverrideCPULimit(t *testing.T) {
	tests := []struct {
		name    string
		mutator func() *podMutator
		input   *corev1.ResourceRequirements
		assert  func(t *testing.T, resources *corev1.ResourceRequirements)
	}{
		{
			name: "WithMemoryLimitSpecified",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						LimitCPUToMemoryRatio: 2.0,
					},
					cpuBaseScaleFactor: factor,
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Limits, corev1.ResourceCPU, resource.MustParse("8000m"))
			},
		},
		{
			name: "WithNoMemoryLimitSpecified",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						LimitCPUToMemoryRatio: 2.0,
					},
					cpuBaseScaleFactor: factor,
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Limits, corev1.ResourceCPU, resource.MustParse("1000m"))
			},
		},
		{
			name: "WithFloor",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						LimitCPUToMemoryRatio: 0.5,
					},
					floor: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("1000m")
							return &q
						}(),
					},
					cpuBaseScaleFactor: factor,
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Limits, corev1.ResourceCPU, resource.MustParse("1000m"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()

			target.OverrideCPULimit(test.input)

			test.assert(t, test.input)
		})
	}
}

func validate(t *testing.T, list corev1.ResourceList, name corev1.ResourceName, want resource.Quantity) {
	got, ok := list[corev1.ResourceName(name)]
	require.Truef(t, ok, "expected: %s, now absent", name)

	result := got.Equal(want)
	require.True(t, result, "mutated, expected: %v, got %v", want, got)
}
