package clusterresourceoverride

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/stretchr/testify/assert"
)

func TestGetMinMax(t *testing.T) {
	tests := []struct {
		name         string
		limitRanges  []*corev1.LimitRange
		resourceName corev1.ResourceName
		minimumWant  resource.Quantity
		maximumWant  resource.Quantity
	}{
		{
			name: "WithMaximumCPULimit",
			limitRanges: []*corev1.LimitRange{
				{
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypeContainer,
								Max: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1024Mi"),
									corev1.ResourceCPU:    resource.MustParse("1000m"),
								},
								Min: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
						},
					},
				},
			},
			resourceName: corev1.ResourceCPU,
			maximumWant:  resource.MustParse("1000m"),
			minimumWant:  resource.MustParse("100m"),
		},

		{
			name: "WithMaximumMemoryLimit",
			limitRanges: []*corev1.LimitRange{
				{
					Spec: corev1.LimitRangeSpec{
						Limits: []corev1.LimitRangeItem{
							{
								Type: corev1.LimitTypeContainer,
								Max: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1024Mi"),
									corev1.ResourceCPU:    resource.MustParse("1000m"),
								},
								Min: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
						},
					},
				},
			},
			resourceName: corev1.ResourceMemory,
			maximumWant:  resource.MustParse("1024Mi"),
			minimumWant:  resource.MustParse("128Mi"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minimumGot, maximumGot := GetMinMax(test.limitRanges, test.resourceName)

			assert.True(t, test.minimumWant.Equal(*minimumGot))
			assert.True(t, test.maximumWant.Equal(*maximumGot))
		})
	}
}
