package clusterresourceoverride

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

type CPUMemory struct {
	CPU    *resource.Quantity
	Memory *resource.Quantity
}

func NewMutator(config *Config, minimum *CPUMemory, maximum *CPUMemory, cpuBaseScaleFactor float64) (mutator *podMutator, err error) {
	if config == nil || minimum == nil || maximum == nil {
		err = errors.New("NewMutator: invalid input")
		return
	}

	mutator = &podMutator{
		config:             config,
		floor:              minimum,
		ceiling:            maximum,
		cpuBaseScaleFactor: cpuBaseScaleFactor,
	}
	return
}

type podMutator struct {
	config             *Config
	floor              *CPUMemory
	ceiling            *CPUMemory
	cpuBaseScaleFactor float64
}

func (m *podMutator) Mutate(in *corev1.Pod) (out *corev1.Pod, err error) {
	current := in.DeepCopy()

	for i := range current.Spec.InitContainers {
		m.Override(&current.Spec.InitContainers[i])
	}

	for i := range current.Spec.Containers {
		m.Override(&current.Spec.Containers[i])
	}

	out = current
	return
}

func (m *podMutator) Override(container *corev1.Container) {
	m.OverrideMemory(&container.Resources)

	// The order is important here, this is processed prior to overriding CPU request.
	m.OverrideCPULimit(&container.Resources)

	m.OverrideCPU(&container.Resources)
}

// If a container memory limit has been specified or defaulted, the memory request
// is overridden to this percentage of the limit.
func (m *podMutator) OverrideMemory(resources *corev1.ResourceRequirements) {
	limit, found := resources.Limits[corev1.ResourceMemory]
	if !found {
		return
	}

	if m.config.MemoryRequestToLimitRatio == 0 {
		return
	}

	modFunc := func(q resource.Quantity) int64 {
		switch q.Format {
		case resource.BinarySI:
			return 1024 * 1024
		default:
			return 1000 * 1000
		}
	}

	// memory is measured in whole bytes.
	// the plugin rounds down to the nearest MiB rather than bytes to improve ease of use for end-users.
	amount := limit.Value() * int64(m.config.MemoryRequestToLimitRatio*100) / 100
	mod := modFunc(limit)

	if rem := amount % mod; rem != 0 {
		amount = amount - rem
	}

	overridden := resource.NewQuantity(int64(amount), limit.Format)
	if m.IsMemoryFloorSpecified() && overridden.Cmp(*m.floor.Memory) < 0 {
		klog.V(5).Infof("%s pod limit %q below namespace limit; setting limit to %q", corev1.ResourceMemory, overridden.String(), m.floor.Memory.String())
		copy := m.floor.Memory.DeepCopy()
		overridden = &copy
	}

	if m.IsMemoryCeilingSpecified() && overridden.Cmp(*m.ceiling.Memory) > 0 {
		klog.V(5).Infof("%s pod limit %q above namespace limit; setting limit to %q", corev1.ResourceMemory, overridden.String(), m.ceiling.Memory.String())
		copy := m.ceiling.Memory.DeepCopy()
		overridden = &copy
	}

	ensureRequests(resources)
	resources.Requests[corev1.ResourceMemory] = *overridden
}

// If a container memory limit has been specified or defaulted, the CPU limit is
// overridden to a percentage of the memory limit, with a 100 percentage scaling
// 1Gi of RAM to equal 1 CPU core. This is processed prior to overriding CPU
// request (if configured).
func (m *podMutator) OverrideCPULimit(resources *corev1.ResourceRequirements) {
	limit, found := resources.Limits[corev1.ResourceMemory]
	if !found {
		return
	}

	if m.config.LimitCPUToMemoryRatio == 0 {
		return
	}

	amount := float64(limit.Value()) * m.config.LimitCPUToMemoryRatio * m.cpuBaseScaleFactor
	overridden := resource.NewMilliQuantity(int64(amount), resource.DecimalSI)
	if m.IsCpuFloorSpecified() && overridden.Cmp(*m.floor.CPU) < 0 {
		klog.V(5).Infof("%s pod limit %q below namespace limit; setting limit to %q", corev1.ResourceCPU, overridden.String(), m.floor.CPU.String())

		clone := m.floor.CPU.DeepCopy()
		overridden = &clone
	}

	if m.IsCpuCeilingSpecified() && overridden.Cmp(*m.ceiling.CPU) > 0 {
		klog.V(5).Infof("%s pod limit %q above namespace limit; setting limit to %q", corev1.ResourceCPU, overridden.String(), m.ceiling.CPU.String())

		clone := m.ceiling.CPU.DeepCopy()
		overridden = &clone
	}

	ensureLimits(resources)
	resources.Limits[corev1.ResourceCPU] = *overridden
}

// If a container CPU limit has been specified or defaulted, the CPU request is
// overridden to this percentage of the limit.
func (m *podMutator) OverrideCPU(resources *corev1.ResourceRequirements) {
	limit, found := resources.Limits[corev1.ResourceCPU]
	if !found {
		return
	}

	if m.config.CpuRequestToLimitRatio == 0 {
		return
	}

	amount := float64(limit.MilliValue()) * m.config.CpuRequestToLimitRatio
	overridden := resource.NewMilliQuantity(int64(amount), limit.Format)

	if m.IsCpuFloorSpecified() && overridden.Cmp(*m.floor.CPU) < 0 {
		klog.V(5).Infof("%s pod limit %q below namespace limit; setting limit to %q", corev1.ResourceCPU, overridden.String(), m.floor.CPU.String())
		clone := m.floor.CPU.DeepCopy()
		overridden = &clone
	}

	if m.IsCpuCeilingSpecified() && overridden.Cmp(*m.ceiling.CPU) > 0 {
		klog.V(5).Infof("%s pod limit %q above namespace limit; setting limit to %q", corev1.ResourceCPU, overridden.String(), m.ceiling.CPU.String())
		clone := m.ceiling.CPU.DeepCopy()
		overridden = &clone
	}

	ensureRequests(resources)
	resources.Requests[corev1.ResourceCPU] = *overridden
}

func (m *podMutator) IsCpuFloorSpecified() bool {
	return m.floor != nil && m.floor.CPU != nil
}

func (m *podMutator) IsMemoryFloorSpecified() bool {
	return m.floor != nil && m.floor.Memory != nil
}

func (m *podMutator) IsCpuCeilingSpecified() bool {
	return m.ceiling != nil && m.ceiling.CPU != nil
}

func (m *podMutator) IsMemoryCeilingSpecified() bool {
	return m.ceiling != nil && m.ceiling.Memory != nil
}

func ensureRequests(resources *corev1.ResourceRequirements) {
	if len(resources.Requests) == 0 {
		resources.Requests = corev1.ResourceList{}
	}
}

func ensureLimits(resources *corev1.ResourceRequirements) {
	if len(resources.Limits) == 0 {
		resources.Limits = corev1.ResourceList{}
	}
}
