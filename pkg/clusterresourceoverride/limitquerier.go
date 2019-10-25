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

func (l *namespaceLimitQuerier) QueryMinimum(namespace string) (minimum *Floor, err error) {
	limits, listErr := l.limitRangesLister.LimitRanges(namespace).List(labels.Everything())
	if listErr != nil {
		err = fmt.Errorf("failed to query limitrange - %v", listErr)
		return
	}

	nsCPUFloor := minResourceLimits(limits, corev1.ResourceCPU)
	nsMemFloor := minResourceLimits(limits, corev1.ResourceMemory)

	minimum = &Floor{
		CPU:    nsCPUFloor,
		Memory: nsMemFloor,
	}
	return
}

// minResourceLimits finds the Min limit for resourceName. Nil is
// returned if limitRanges is empty or limits contains no resourceName
// limits.
func minResourceLimits(limitRanges []*corev1.LimitRange, resourceName corev1.ResourceName) *resource.Quantity {
	limits := []*resource.Quantity{}

	for _, limitRange := range limitRanges {
		for _, limit := range limitRange.Spec.Limits {
			if limit.Type == corev1.LimitTypeContainer {
				if limit, found := limit.Min[resourceName]; found {
					clone := limit.DeepCopy()
					limits = append(limits, &clone)
				}
			}
		}
	}

	if len(limits) == 0 {
		return nil
	}

	return minQuantity(limits)
}

func minQuantity(quantities []*resource.Quantity) *resource.Quantity {
	min := quantities[0].DeepCopy()

	for i := range quantities {
		if quantities[i].Cmp(min) < 0 {
			min = quantities[i].DeepCopy()
		}
	}

	return &min
}
