package clusterresourceoverride

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

type namespaceLimitQuerier struct {
	limitRangesLister corev1listers.LimitRangeLister
}

func (l *namespaceLimitQuerier) QueryFloorAndCeiling(namespace string) (floor *CPUMemory, ceiling *CPUMemory, err error) {
	limitRanges, listErr := l.limitRangesLister.LimitRanges(namespace).List(labels.Everything())
	if listErr != nil {
		err = fmt.Errorf("failed to query limitrange - %v", listErr)
		return
	}

	nsCPUMinimum, nsCPUMaximum := GetMinMax(limitRanges, corev1.ResourceCPU)
	nsMemMinimum, nsMemMaximum := GetMinMax(limitRanges, corev1.ResourceMemory)

	floor = &CPUMemory{
		CPU:    nsCPUMinimum,
		Memory: nsMemMinimum,
	}
	ceiling = &CPUMemory{
		CPU:    nsCPUMaximum,
		Memory: nsMemMaximum,
	}
	return
}

// GetMinMax finds the Minimum and Maximum limit for respectively for the specified resource.
// Nil is returned if limitRanges is empty or limits contains no resourceName limits.
func GetMinMax(limitRanges []*corev1.LimitRange, resourceName corev1.ResourceName) (minimum *resource.Quantity, maximum *resource.Quantity) {
	minList, maxList := findMinMaxLimits(limitRanges, resourceName)

	minimum = minQuantity(minList)
	maximum = maxQuantity(maxList)

	return
}

func findMinMaxLimits(limitRanges []*corev1.LimitRange, resourceName corev1.ResourceName) (minimum []*resource.Quantity, maximum []*resource.Quantity) {
	minimum = []*resource.Quantity{}
	maximum = []*resource.Quantity{}

	for _, limitRange := range limitRanges {
		for _, limits := range limitRange.Spec.Limits {
			if limits.Type == corev1.LimitTypeContainer {
				if min, found := limits.Min[resourceName]; found {
					clone := min.DeepCopy()
					minimum = append(minimum, &clone)
				}

				if max, found := limits.Max[resourceName]; found {
					clone := max.DeepCopy()
					maximum = append(maximum, &clone)
				}
			}
		}
	}

	return
}

func minQuantity(quantities []*resource.Quantity) *resource.Quantity {
	if len(quantities) == 0 {
		return nil
	}

	min := *quantities[0]

	for i := range quantities {
		if quantities[i].Cmp(min) < 0 {
			min = *quantities[i]
		}
	}

	copy := min.DeepCopy()
	return &copy
}

func maxQuantity(quantities []*resource.Quantity) *resource.Quantity {
	if len(quantities) == 0 {
		return nil
	}

	max := *quantities[0]

	for i := range quantities {
		if quantities[i].Cmp(max) > 0 {
			max = *quantities[i]
		}
	}

	copy := max.DeepCopy()
	return &copy
}
