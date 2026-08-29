package clusterresourceoverride

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	factor = 1000.0 / (1024.0 * 1024.0 * 1024.0) // 1000 milliCores per 1GiB
)

func TestMutator_Mutate(t *testing.T) {

	cpu := resource.MustParse("1m")
	memory := resource.MustParse("1Mi")

	// Tests the mutator using CPURequestToLimitRatio as the CPU request override
	t.Run("WithCpuRequestToLimitRatio", func(t *testing.T) {
		floor := &CPUMemory{
			CPU:    &cpu,
			Memory: &memory,
		}
		config := &Config{
			LimitCPUToMemoryRatio:     2.0,
			CpuRequestToLimitRatio:    0.25,
			MemoryRequestToLimitRatio: 0.5,
			CpuRequestToRequestRatio:  0,
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
	})

	// Tests mutator using CPURequestToRequestRatio as the CPU resource override
	// Ensures CPURequestToRequestRatio overwrites CPURequestToLimitRatio
	// Ensures CPURequestToRequestRatio doesn't compute with request from CPURequestToLimitRatio
	// Tests the per container annotations to ensure the mutator applies the override
	//   according to each container's original request
	t.Run("WithCpuRequestToRequestRatio", func(t *testing.T) {
		floor := &CPUMemory{
			CPU:    &cpu,
			Memory: &memory,
		}
		config := &Config{
			LimitCPUToMemoryRatio:     2.0,
			CpuRequestToLimitRatio:    0.25,
			MemoryRequestToLimitRatio: 0.5,
			CpuRequestToRequestRatio:  0.25,
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
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("4Gi"),
								corev1.ResourceCPU:    resource.MustParse("2000m"),
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
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1Gi"),
								corev1.ResourceCPU:    resource.MustParse("1000m"),
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
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("500m"),
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
		validate(t, podGot.Spec.InitContainers[0].Resources.Requests, corev1.ResourceCPU, resource.MustParse("125m"))

		// verify db container
		validate(t, podGot.Spec.Containers[0].Resources.Requests, corev1.ResourceMemory, resource.MustParse("8Gi"))
		validate(t, podGot.Spec.Containers[0].Resources.Limits, corev1.ResourceCPU, resource.MustParse("32000m"))
		validate(t, podGot.Spec.Containers[0].Resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))

		// verify app container
		validate(t, podGot.Spec.Containers[1].Resources.Requests, corev1.ResourceMemory, resource.MustParse("1Gi"))
		validate(t, podGot.Spec.Containers[1].Resources.Limits, corev1.ResourceCPU, resource.MustParse("4000m"))
		validate(t, podGot.Spec.Containers[1].Resources.Requests, corev1.ResourceCPU, resource.MustParse("250m"))
	})
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

func TestMutator_OverrideCPUWithRequest(t *testing.T) {
	testContainerName := "test"
	testAnnotation := fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, testContainerName)
	tests := []struct {
		name    string
		pod     *corev1.Pod
		mutator func() *podMutator
		input   *corev1.ResourceRequirements
		assert  func(t *testing.T, resources *corev1.ResourceRequirements)
	}{
		{
			// happy path
			name: "WithCpuRequest",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "2000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// request annotation not present
			name: "WithoutCpuRequest",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				_, found := resources.Requests[corev1.ResourceCPU]
				require.False(t, found, "expected no CPU request to be set")
			},
		},
		{
			// Ratio override value not set
			name: "WithRatioValueZero",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "2000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0,
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				_, found := resources.Requests[corev1.ResourceCPU]
				require.False(t, found, "expected no CPU request to be set")
			},
		},
		{
			// CPU request modified from previous override
			// Regression test to ensure annotation is used to get request
			name: "WithModifiedRequest",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "2000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// Override is below the set floor
			name: "WithOverrideBelowFloor",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "1000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
					floor: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("500m")
							return &q
						}(),
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// Override is above the set ceiling
			name: "WithOverrideAboveCeiling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "2000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.5,
					},
					ceiling: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("500m")
							return &q
						}(),
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// Override inbetween the set ceiling and floor
			name: "WithOverrideBetweenFloorCeiling",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "1000m",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.5,
					},
					ceiling: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("1000m")
							return &q
						}(),
					},
					floor: &CPUMemory{
						CPU: func() *resource.Quantity {
							q := resource.MustParse("250m")
							return &q
						}(),
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				validate(t, resources.Requests, corev1.ResourceCPU, resource.MustParse("500m"))
			},
		},
		{
			// Annotation value is "0", should be a no-op
			name: "WithZeroAnnotationValue",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
					Annotations: map[string]string{
						testAnnotation: "0",
					},
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				_, found := resources.Requests[corev1.ResourceCPU]
				require.False(t, found, "expected no CPU request to be set")
			},
		},
		{
			// Container has limits but no request at all, no annotation set.
			// CpuRequestToRequestRatio is configured but should be a no-op.
			name: "WithNoRequestAndNoAnnotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
					corev1.ResourceCPU:    resource.MustParse("2000m"),
				},
			},
			assert: func(t *testing.T, resources *corev1.ResourceRequirements) {
				// verify limits are unchanged
				validate(t, resources.Limits, corev1.ResourceMemory, resource.MustParse("2Gi"))
				validate(t, resources.Limits, corev1.ResourceCPU, resource.MustParse("2000m"))
				// verify no requests were created
				_, found := resources.Requests[corev1.ResourceCPU]
				require.False(t, found, "expected no CPU request to be set")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()

			target.OverrideCPUWithRequest(test.input, testContainerName, test.pod)

			test.assert(t, test.input)
		})
	}
}

func TestMutator_AnnotateOriginalRequest(t *testing.T) {
	containerName := "mycontainer"
	annotationKey := fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, containerName)

	tests := []struct {
		name    string
		mutator func() *podMutator
		input   *corev1.ResourceRequirements
		pod     *corev1.Pod
		assert  func(t *testing.T, pod *corev1.Pod)
	}{
		{
			// Annotation is written with the correct per-container key
			name: "PerContainerAnnotation",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				},
			},
			pod: &corev1.Pod{},
			assert: func(t *testing.T, pod *corev1.Pod) {
				val, found := pod.Annotations[annotationKey]
				require.True(t, found, "expected per-container annotation to be set")
				require.Equal(t, "500m", val)
			},
		},
		{
			// Second container does not overwrite first container's annotation
			name: "MultipleContainersGetSeparateAnnotations",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						CpuRequestToRequestRatio: 0.25,
					},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, "other"): "1000m",
					},
				},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				otherKey := fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, "other")
				otherVal, otherFound := pod.Annotations[otherKey]
				require.True(t, otherFound, "expected other container annotation to remain")
				require.Equal(t, "1000m", otherVal)

				val, found := pod.Annotations[annotationKey]
				require.True(t, found, "expected this container annotation to be set")
				require.Equal(t, "200m", val)
			},
		},
		{
			// No CPU request means no annotation is written
			name: "ZeroRequestSkipsAnnotation",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{},
				}
			},
			input: &corev1.ResourceRequirements{},
			pod:   &corev1.Pod{},
			assert: func(t *testing.T, pod *corev1.Pod) {
				_, found := pod.Annotations[annotationKey]
				require.False(t, found, "expected no annotation for zero request")
			},
		},
		{
			// Existing annotation is not overwritten on reinvocation
			name: "ExistingAnnotationNotOverwritten",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{},
				}
			},
			input: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotationKey: "500m",
					},
				},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				val, found := pod.Annotations[annotationKey]
				require.True(t, found)
				require.Equal(t, "500m", val, "expected original annotation to be preserved")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()
			target.AnnotateOriginalRequest(test.input, containerName, test.pod)
			test.assert(t, test.pod)
		})
	}
}

func TestMutator_OverrideCpuWithLimit(t *testing.T) {
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

			target.OverrideCPUWithLimit(test.input)

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

func TestMutator_OverrideSelinuxFix(t *testing.T) {
	tests := []struct {
		name    string
		mutator func() *podMutator
		input   *corev1.Pod
		assert  func(t *testing.T, resources *corev1.Pod)
	}{
		{
			name: "Selinux Enabled with PVC and label",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						ForceSelinuxRelabel:       true,
						LimitCPUToMemoryRatio:     100,
						CpuRequestToLimitRatio:    100,
						MemoryRequestToLimitRatio: 100,
					},
				}
			},
			input: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"forceselinuxrelabel.admission.node.openshift.io/enabled": "true",
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "test-pv-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "container_1",
							Image: "busybox",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "test-pvc",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				require.Equal(t, pod.Spec.SecurityContext.SELinuxOptions.Type, SpcType)
			},
		},
		{
			name: "Selinux Enabled with PVC without label",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						ForceSelinuxRelabel:       true,
						LimitCPUToMemoryRatio:     100,
						CpuRequestToLimitRatio:    100,
						MemoryRequestToLimitRatio: 100,
					},
				}
			},
			input: &corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "test-pv-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "container_1",
							Image: "busybox",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "test-pvc",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				require.Nil(t, pod.Spec.SecurityContext)
			},
		},
		{
			name: "Selinux Enabled without PVC with label",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						ForceSelinuxRelabel:       true,
						LimitCPUToMemoryRatio:     100,
						CpuRequestToLimitRatio:    100,
						MemoryRequestToLimitRatio: 100,
					},
				}
			},
			input: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"forceselinuxrelabel.admission.node.openshift.io/enabled": "true",
					},
				},
				Spec: corev1.PodSpec{},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				require.Nil(t, pod.Spec.SecurityContext)
			},
		},
		{
			name: "Selinux Enabled without PVC with label and existing SecurityContext",
			mutator: func() *podMutator {
				return &podMutator{
					config: &Config{
						ForceSelinuxRelabel:       true,
						LimitCPUToMemoryRatio:     100,
						CpuRequestToLimitRatio:    100,
						MemoryRequestToLimitRatio: 100,
					},
				}
			},
			input: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"forceselinuxrelabel.admission.node.openshift.io/enabled": "true",
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "test-pv-storage",
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "container_1",
							Image: "busybox",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "test-pvc",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, pod *corev1.Pod) {
				require.Nil(t, pod.Spec.SecurityContext)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := test.mutator()

			target.OverrideForceSelinuxRelabel(test.input)

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

// TestMutator_CPUOverridesDoNotUndercountFractionalPercent covers the float64
// truncation across the three CPU override paths. 29% is the canonical case:
// float64(x)*0.29 is 28.999999999999996, so a plain int64 cast dropped the result
// by 1m. Each path is exercised through the real config conversion.
func TestMutator_CPUOverridesDoNotUndercountFractionalPercent(t *testing.T) {
	t.Run("OverrideCPUWithLimit", func(t *testing.T) {
		m := &podMutator{config: ConvertExternalConfig(&ClusterResourceOverride{
			Spec: ClusterResourceOverrideSpec{CPURequestToLimitPercent: 29},
		})}
		res := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
		}
		m.OverrideCPUWithLimit(res)
		validate(t, res.Requests, corev1.ResourceCPU, resource.MustParse("29m"))
	})

	t.Run("OverrideCPUWithRequest", func(t *testing.T) {
		m := &podMutator{config: ConvertExternalConfig(&ClusterResourceOverride{
			Spec: ClusterResourceOverrideSpec{CPURequestToRequestPercent: 29},
		})}
		name := "c"
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, name): "100m",
		}}}
		res := &corev1.ResourceRequirements{}
		m.OverrideCPUWithRequest(res, name, pod)
		validate(t, res.Requests, corev1.ResourceCPU, resource.MustParse("29m"))
	})

	t.Run("OverrideCPULimit", func(t *testing.T) {
		m := &podMutator{
			config: ConvertExternalConfig(&ClusterResourceOverride{
				Spec: ClusterResourceOverrideSpec{LimitCPUToMemoryPercent: 29},
			}),
			cpuBaseScaleFactor: factor,
		}
		res := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1536Mi")},
		}
		m.OverrideCPULimit(res)
		validate(t, res.Limits, corev1.ResourceCPU, resource.MustParse("435m"))
	})
}

// TestMutator_CPUOverridesPreserveProgrammaticFractionalRatio covers a Config built directly
// with a ratio that is not a whole-number percent. This is only reachable programmatically
// (ConvertExternalConfig always yields a multiple of 0.01 from its int64 percentage), and
// #118's Config.Validate accepts the full [0,1] range, so the mutator scales such a ratio
// linearly instead of rounding it to the nearest percent: 0.5% of 1000m is 5m, not 10m.
func TestMutator_CPUOverridesPreserveProgrammaticFractionalRatio(t *testing.T) {
	t.Run("OverrideCPUWithLimit is linear, not rounded", func(t *testing.T) {
		m := &podMutator{config: &Config{CpuRequestToLimitRatio: 0.005}}
		res := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1000m")},
		}
		m.OverrideCPUWithLimit(res)
		validate(t, res.Requests, corev1.ResourceCPU, resource.MustParse("5m"))
	})

	t.Run("OverrideCPUWithLimit floors a fractional product", func(t *testing.T) {
		// 33.33% of 1000m is 333.3m, floored to 333m (a round-to-33% would give 330m).
		m := &podMutator{config: &Config{CpuRequestToLimitRatio: 0.3333}}
		res := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1000m")},
		}
		m.OverrideCPUWithLimit(res)
		validate(t, res.Requests, corev1.ResourceCPU, resource.MustParse("333m"))
	})

	t.Run("OverrideCPUWithRequest is linear, not rounded", func(t *testing.T) {
		m := &podMutator{config: &Config{CpuRequestToRequestRatio: 0.005}}
		name := "c"
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, name): "1000m",
		}}}
		res := &corev1.ResourceRequirements{}
		m.OverrideCPUWithRequest(res, name, pod)
		validate(t, res.Requests, corev1.ResourceCPU, resource.MustParse("5m"))
	})

	t.Run("OverrideCPULimit is linear, not rounded", func(t *testing.T) {
		// 1Gi is exactly 2^30 bytes and the default factor is 1000/2^30, so the size cancels
		// and 0.5% yields 5m (a round-to-1% would give 10m).
		m := &podMutator{config: &Config{LimitCPUToMemoryRatio: 0.005}, cpuBaseScaleFactor: factor}
		res := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
		}
		m.OverrideCPULimit(res)
		validate(t, res.Limits, corev1.ResourceCPU, resource.MustParse("5m"))
	})
}

func TestMutator_OverrideCPULimitFloorsExactly(t *testing.T) {
	// OverrideCPULimit does an exact integer division and floors it. These lock both
	// halves: the truncation fix (1536Mi at 29% is exactly 435m, previously lost to
	// float64 error as 434m) and the floor policy (768Mi at 29% is exactly 217.5m,
	// which floors to 217m rather than rounding up to 218m; 1538Mi at 29% floors to
	// 435m, not 436m).
	cases := []struct {
		name   string
		memory string
		pct    int64
		want   string
	}{
		{"exact integer result is not lost to float64 error", "1536Mi", 29, "435m"},
		{"fractional result floors down", "1538Mi", 29, "435m"},
		{"exact half floors down", "768Mi", 29, "217m"},
		{"high percent floors down", "88Mi", 96, "82m"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &podMutator{
				config: ConvertExternalConfig(&ClusterResourceOverride{
					Spec: ClusterResourceOverrideSpec{LimitCPUToMemoryPercent: c.pct},
				}),
				cpuBaseScaleFactor: factor,
			}
			res := &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(c.memory)},
			}
			m.OverrideCPULimit(res)
			validate(t, res.Limits, corev1.ResourceCPU, resource.MustParse(c.want))
		})
	}
}

func TestMutator_OverrideCPULimitSaturates(t *testing.T) {
	// A percent near the int64 ceiling produces an exact large result rather than a wrapped
	// negative one, and a result past int64 saturates to MaxInt64. Asserting the exact value
	// (not just non-negative) also catches a recovered percent that is merely off by a little.
	cases := []struct {
		name   string
		memory string
		pct    int64
		want   int64
	}{
		{"max percent within int64 floors exactly", "1Mi", math.MaxInt64, 90071992547409919},
		{"max percent past int64 saturates", "1Gi", math.MaxInt64, math.MaxInt64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &podMutator{
				config: ConvertExternalConfig(&ClusterResourceOverride{
					Spec: ClusterResourceOverrideSpec{LimitCPUToMemoryPercent: c.pct},
				}),
				cpuBaseScaleFactor: factor,
			}
			res := &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(c.memory)},
			}
			m.OverrideCPULimit(res)
			got := res.Limits[corev1.ResourceCPU]
			assert.GreaterOrEqual(t, got.MilliValue(), int64(0), "must never be negative")
			assert.Equal(t, c.want, got.MilliValue(), "exact saturated or floored value")
		})
	}
}

func TestSaturatingRoundedPercent(t *testing.T) {
	// The helper recovers an integer percent from the float64 ratio for the legacy path and
	// must never emit an arbitrary int64 for a non-finite input.
	assert.Equal(t, int64(0), saturatingRoundedPercent(0))
	assert.Equal(t, int64(29), saturatingRoundedPercent(0.29))
	assert.Equal(t, int64(200), saturatingRoundedPercent(2.0))
	assert.Equal(t, int64(math.MaxInt64), saturatingRoundedPercent(math.Inf(1)))
	assert.Equal(t, int64(math.MinInt64), saturatingRoundedPercent(math.Inf(-1)))
	assert.Equal(t, int64(0), saturatingRoundedPercent(math.NaN()), "NaN must not become an arbitrary int64")
}

func TestConvertExternalConfigPercentRoundTrip(t *testing.T) {
	// Every supported request percentage (0..100) must survive int64 -> float64/100 ->
	// Round(*100) exactly, so the CPU request paths are not off by 1m.
	for p := int64(0); p <= 100; p++ {
		c := ConvertExternalConfig(&ClusterResourceOverride{
			Spec: ClusterResourceOverrideSpec{
				CPURequestToLimitPercent:   p,
				CPURequestToRequestPercent: p,
			},
		})
		assert.Equal(t, p, saturatingRoundedPercent(c.CpuRequestToLimitRatio), "cpuRequestToLimit percent %d", p)
		assert.Equal(t, p, saturatingRoundedPercent(c.CpuRequestToRequestRatio), "cpuRequestToRequest percent %d", p)
	}
}

func TestMutator_OverrideCPULimitHonorsScaleFactor(t *testing.T) {
	// cpuBaseScaleFactor is an exported NewMutator parameter; a non-default factor must be
	// honored, not silently ignored. Doubling the default scale doubles the result.
	build := func(f float64) *podMutator {
		m, err := NewMutator(
			&Config{LimitCPUToMemoryRatio: 1.0},
			&CPUMemory{}, &CPUMemory{}, f,
		)
		require.NoError(t, err)
		return m
	}
	res := func() *corev1.ResourceRequirements {
		return &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
		}
	}

	def := res()
	build(factor).OverrideCPULimit(def)
	validate(t, def.Limits, corev1.ResourceCPU, resource.MustParse("1000m"))

	doubled := res()
	build(2 * factor).OverrideCPULimit(doubled)
	validate(t, doubled.Limits, corev1.ResourceCPU, resource.MustParse("2000m"))
}

func TestNewMutator_RejectsInvalidScaleFactor(t *testing.T) {
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		_, err := NewMutator(&Config{}, &CPUMemory{}, &CPUMemory{}, f)
		assert.Error(t, err, "factor %v should be rejected", f)
	}
	_, err := NewMutator(&Config{}, &CPUMemory{}, &CPUMemory{}, factor)
	assert.NoError(t, err)
}

func TestNewMutator_RejectsNonFiniteRatio(t *testing.T) {
	// A NaN or infinite ratio (only reachable from a directly built Config) must fail
	// construction rather than be silently treated as 0% at admission time.
	for _, c := range []*Config{
		{LimitCPUToMemoryRatio: math.NaN()},
		{CpuRequestToLimitRatio: math.Inf(1)},
		{MemoryRequestToLimitRatio: math.Inf(-1)},
		{CpuRequestToRequestRatio: math.NaN()},
	} {
		_, err := NewMutator(c, &CPUMemory{}, &CPUMemory{}, factor)
		assert.Error(t, err, "non-finite ratio should be rejected")
	}
	_, err := NewMutator(&Config{LimitCPUToMemoryRatio: 2.0}, &CPUMemory{}, &CPUMemory{}, factor)
	assert.NoError(t, err)
}

func TestOverridePreservesExactPercentThroughConversion(t *testing.T) {
	// End to end: a whole-number percent from the operator CRD keeps its exact integer
	// arithmetic through ConvertExternalConfig -> NewMutator -> Mutate, rather than losing the
	// last millicore to a truncating float cast. Config.Validate is added in #118 and joins
	// this chain once that merges; the conversion and mutation steps are what this PR changes.
	podWithLimits := func(limits corev1.ResourceList) *corev1.Pod {
		return &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:      "app",
					Resources: corev1.ResourceRequirements{Limits: limits},
				}},
			},
		}
	}

	t.Run("CPU-to-memory scales a memory limit to an exact CPU limit", func(t *testing.T) {
		cro := &ClusterResourceOverride{Spec: ClusterResourceOverrideSpec{LimitCPUToMemoryPercent: 29}}
		mutator, err := NewMutator(ConvertExternalConfig(cro), &CPUMemory{}, &CPUMemory{}, factor)
		require.NoError(t, err)

		out, err := mutator.Mutate(podWithLimits(corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1536Mi")}))
		require.NoError(t, err)

		// 1536Mi is 1.5GiB, so 1.5 * 1000 mCPU * 29% is exactly 435m; a float cast yields 434m.
		got := out.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
		assert.Equal(t, "435m", got.String())
	})

	t.Run("CPU-request-to-limit sets an exact CPU request", func(t *testing.T) {
		cro := &ClusterResourceOverride{Spec: ClusterResourceOverrideSpec{CPURequestToLimitPercent: 29}}
		mutator, err := NewMutator(ConvertExternalConfig(cro), &CPUMemory{}, &CPUMemory{}, factor)
		require.NoError(t, err)

		out, err := mutator.Mutate(podWithLimits(corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}))
		require.NoError(t, err)

		// 100m * 29% is exactly 29m; float64(100)*0.29 is 28.999... and truncates to 28m.
		got := out.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
		assert.Equal(t, "29m", got.String())
	})
}
