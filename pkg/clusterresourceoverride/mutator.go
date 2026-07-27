package clusterresourceoverride

import (
	"errors"
	"fmt"
	"math"
	"math/big"

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
	if math.IsNaN(cpuBaseScaleFactor) || math.IsInf(cpuBaseScaleFactor, 0) || cpuBaseScaleFactor < 0 {
		err = fmt.Errorf("NewMutator: cpuBaseScaleFactor must be finite and non-negative, got %v", cpuBaseScaleFactor)
		return
	}
	// Reject a non-finite ratio here rather than letting it reach the arithmetic, where a
	// NaN would otherwise be quietly treated as 0%. A Config from ConvertExternalConfig
	// cannot be non-finite (the external percentages are int64); this guards a Config built
	// directly. Range validation of the ratios lives in the configuration loader.
	for _, r := range []struct {
		name  string
		ratio float64
	}{
		{"LimitCPUToMemoryRatio", config.LimitCPUToMemoryRatio},
		{"CpuRequestToLimitRatio", config.CpuRequestToLimitRatio},
		{"MemoryRequestToLimitRatio", config.MemoryRequestToLimitRatio},
		{"CpuRequestToRequestRatio", config.CpuRequestToRequestRatio},
	} {
		if math.IsNaN(r.ratio) || math.IsInf(r.ratio, 0) {
			err = fmt.Errorf("NewMutator: %s must be finite, got %v", r.name, r.ratio)
			return
		}
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

	if m.config.ForceSelinuxRelabel {
		m.OverrideForceSelinuxRelabel(current)
	}

	for i := range current.Spec.InitContainers {
		m.Override(&current.Spec.InitContainers[i], current)
	}

	for i := range current.Spec.Containers {
		m.Override(&current.Spec.Containers[i], current)
	}

	out = current
	return
}

const (
	SpcType                      = "spc_t"
	SelinuxRelabelResource       = "forceselinuxrelabel"
	SelinuxRelabelGroup          = "admission.node.openshift.io"
	OriginalCPURequestAnnotation = "clusterresourceoverrides.admission.autoscaling.openshift.io/original-cpu-request"
)

var (
	// SelinuxFixEnabledLabelName is the name of the label applied to the pod
	SelinuxFixEnabledLabelName = fmt.Sprintf("%s.%s/enabled", SelinuxRelabelResource, SelinuxRelabelGroup)
)

func (m *podMutator) OverrideForceSelinuxRelabel(pod *corev1.Pod) {
	enabled, exists := pod.Labels[SelinuxFixEnabledLabelName]
	if !exists || (exists && enabled == "false") {
		return
	}
	// Find if the persistent volume claim exists
	foundPersistentVolumeClaim := false
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			foundPersistentVolumeClaim = true
			break
		}
	}
	// Return without modification if the PVC does not exist
	if !foundPersistentVolumeClaim {
		return
	}
	// PVC exists so modify the pod with the spc_t label
	if pod.Spec.SecurityContext == nil {
		pod.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	pod.Spec.SecurityContext.SELinuxOptions = &corev1.SELinuxOptions{Type: SpcType}
}

func (m *podMutator) Override(container *corev1.Container, current *corev1.Pod) {
	// Needs to run before an override modifies the request
	m.AnnotateOriginalRequest(&container.Resources, container.Name, current)

	m.OverrideMemory(&container.Resources)

	// The order is important here, this is processed prior to overriding CPU request.
	m.OverrideCPULimit(&container.Resources)

	m.OverrideCPUWithLimit(&container.Resources)

	// Should run after OverrideCPUWithLimit
	m.OverrideCPUWithRequest(&container.Resources, container.Name, current)
}

// Annotates pod with original CPU request value. Annotation provides idempotency for
// OverrideCPUWithRequest in case of reinvocation. Also preserves original value in case
// of OverrideCPUWithLimit modifying the request prior to OverrideCPUWithRequest.
func (m *podMutator) AnnotateOriginalRequest(resources *corev1.ResourceRequirements, name string, pod *corev1.Pod) {
	if m.config.CpuRequestToRequestRatio == 0 {
		return
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	key := fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, name)
	_, found := pod.Annotations[key]
	if !found {
		request := resources.Requests[corev1.ResourceCPU]
		if request.IsZero() {
			return
		}
		pod.Annotations[key] = request.String()
	}
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

// saturatingRoundedPercent rounds ratio*100 to the nearest integer percent and returns it
// as an int64, clamping at the int64 bounds. An out-of-range float64-to-int64 conversion is
// implementation-dependent (the Go spec does not guarantee it wraps) and can yield a
// negative value, which would then produce a negative CPU quantity instead of saturating; a
// NaN ratio (only reachable from a directly built Config) becomes 0 rather than an arbitrary
// int64.
func saturatingRoundedPercent(ratio float64) int64 {
	p := math.Round(ratio * 100)
	switch {
	case math.IsNaN(p):
		return 0
	case p >= float64(math.MaxInt64):
		return math.MaxInt64
	case p <= float64(math.MinInt64):
		return math.MinInt64
	default:
		return int64(p)
	}
}

// saturatingInt64 returns n as an int64, clamping to the int64 range when n does not fit.
// math/big's Int64 is undefined for an out-of-range value, so the magnitude must be gated
// first; the CPU amounts computed here are non-negative in normal operation, and an
// out-of-range magnitude saturates to the nearest int64 bound.
func saturatingInt64(n *big.Int) int64 {
	if n.IsInt64() {
		return n.Int64()
	}
	if n.Sign() < 0 {
		return math.MinInt64
	}
	return math.MaxInt64
}

// recoverWholePercent recovers the whole-number percent that a ratio (stored as percent/100)
// represents, and reports whether the ratio is exactly that whole percent. ConvertExternalConfig
// builds float64(percent)/100 from an int64 percentage, so every request percentage in [0,100]
// and every practical CPU-to-memory percentage takes the exact integer-percent path; a Config
// built directly with a fractional ratio such as 0.005 does not, and is scaled linearly instead
// of being rounded to a percent. (An astronomically large CPU-to-memory percentage can recover a
// slightly different whole percent through the float64 round-trip and still report whole; exact
// handling of the full unbounded int64 percentage is tracked in #115.)
func recoverWholePercent(ratio float64) (pct int64, whole bool) {
	pct = saturatingRoundedPercent(ratio)
	return pct, ratio == float64(pct)/100
}

// saturatingTruncFloat64 converts v to an int64, truncating toward zero (the historical
// behavior) and clamping to the int64 range. An out-of-range float64-to-int64 conversion is
// implementation-dependent in Go, so the magnitude is gated first; a NaN becomes 0.
func saturatingTruncFloat64(v float64) int64 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= float64(math.MaxInt64):
		return math.MaxInt64
	case v <= float64(math.MinInt64):
		return math.MinInt64
	default:
		return int64(v)
	}
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

	// A whole-number percent (every practical CPU-to-memory percentage ConvertExternalConfig
	// produces) is applied exactly:
	// scale the memory bytes to millicores by cpuBaseScaleFactor's exact rational form and do
	// the multiply/divide in big.Int, so the result is the exact floor (the historical
	// truncating behavior) rather than a float64 approximation (a 1536Mi limit at 29% is 435m,
	// not the 434m a float cast produces), and a result past int64 saturates instead of
	// wrapping. The default cpuBaseScaleFactor 1000/2^30 is exactly representable as a float64,
	// so this path is exact for the production configuration; the unbounded-percentage exact
	// case is tracked in #115. A ratio that is not a whole percent is only reachable from a
	// directly built Config and is scaled linearly (historical behavior) rather than rounded to
	// a percent.
	var amount int64
	if pct, whole := recoverWholePercent(m.config.LimitCPUToMemoryRatio); whole {
		scale := new(big.Rat).SetFloat64(m.cpuBaseScaleFactor) // finite and non-negative: validated in NewMutator
		amountBig := new(big.Int).Mul(big.NewInt(limit.Value()), big.NewInt(pct))
		amountBig.Mul(amountBig, scale.Num())
		amountBig.Quo(amountBig, new(big.Int).Mul(big.NewInt(100), scale.Denom()))
		amount = saturatingInt64(amountBig)
	} else {
		amount = saturatingTruncFloat64(float64(limit.Value()) * m.config.LimitCPUToMemoryRatio * m.cpuBaseScaleFactor)
	}
	overridden := resource.NewMilliQuantity(amount, resource.DecimalSI)
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
func (m *podMutator) OverrideCPUWithLimit(resources *corev1.ResourceRequirements) {
	limit, found := resources.Limits[corev1.ResourceCPU]
	if !found {
		return
	}

	if m.config.CpuRequestToLimitRatio == 0 {
		return
	}

	// CpuRequestToLimitRatio is stored as a float64 (percent/100). A whole-number percent is
	// applied in big.Int so an exact-integer result is not lost to a truncating float cast
	// (100m at 29% is float64(100)*0.29 = 28.999... -> 28m with a plain cast; big.Int gives
	// 29m); a ratio that is not a whole percent (only reachable from a directly built Config)
	// is scaled linearly, matching the historical behavior for such a value. Both paths
	// saturate the int64 conversion. This assumes limit.MilliValue() has not itself overflowed
	// int64, a pre-existing edge for an astronomically large Pod CPU quantity tracked in #115.
	var amount int64
	if pct, whole := recoverWholePercent(m.config.CpuRequestToLimitRatio); whole {
		amountBig := new(big.Int).Mul(big.NewInt(limit.MilliValue()), big.NewInt(pct))
		amountBig.Quo(amountBig, big.NewInt(100))
		amount = saturatingInt64(amountBig)
	} else {
		amount = saturatingTruncFloat64(float64(limit.MilliValue()) * m.config.CpuRequestToLimitRatio)
	}
	overridden := resource.NewMilliQuantity(amount, limit.Format)

	if m.IsCpuFloorSpecified() && overridden.Cmp(*m.floor.CPU) < 0 {
		klog.V(5).Infof("%s pod request %q below namespace minimum; setting request to %q", corev1.ResourceCPU, overridden.String(), m.floor.CPU.String())
		clone := m.floor.CPU.DeepCopy()
		overridden = &clone
	}

	if m.IsCpuCeilingSpecified() && overridden.Cmp(*m.ceiling.CPU) > 0 {
		klog.V(5).Infof("%s pod request %q above namespace maximum; setting request to %q", corev1.ResourceCPU, overridden.String(), m.ceiling.CPU.String())
		clone := m.ceiling.CPU.DeepCopy()
		overridden = &clone
	}

	ensureRequests(resources)
	resources.Requests[corev1.ResourceCPU] = *overridden
}

// If a container CPU limit has not been specified or defaulted, the CPU request is
// overridden to this percentage of the original request (stored in a pod annotation).
func (m *podMutator) OverrideCPUWithRequest(resources *corev1.ResourceRequirements, name string, pod *corev1.Pod) {
	if m.config.CpuRequestToRequestRatio == 0 {
		return
	}

	key := fmt.Sprintf("%s-%s", OriginalCPURequestAnnotation, name)
	strValue, found := pod.Annotations[key]
	if !found {
		klog.Warningf("failed to find %q annotation for pod %s/%s; skipping CPU request override", key, pod.Namespace, pod.Name)
		return
	}

	request, err := resource.ParseQuantity(strValue)
	if err != nil {
		klog.Warningf("failed to parse %q annotation for pod %s/%s: %v; skipping CPU request override", key, pod.Namespace, pod.Name, err)
		return
	}

	if request.IsZero() {
		return
	}

	// Same whole-percent-vs-fractional split as OverrideCPUWithLimit: a whole percent is applied
	// exactly in big.Int, a fractional ratio (only from a directly built Config) is scaled
	// linearly, and the conversion saturates. As above, this assumes request.MilliValue() has
	// not itself overflowed int64.
	var amount int64
	if pct, whole := recoverWholePercent(m.config.CpuRequestToRequestRatio); whole {
		amountBig := new(big.Int).Mul(big.NewInt(request.MilliValue()), big.NewInt(pct))
		amountBig.Quo(amountBig, big.NewInt(100))
		amount = saturatingInt64(amountBig)
	} else {
		amount = saturatingTruncFloat64(float64(request.MilliValue()) * m.config.CpuRequestToRequestRatio)
	}
	overridden := resource.NewMilliQuantity(amount, request.Format)

	if m.IsCpuFloorSpecified() && overridden.Cmp(*m.floor.CPU) < 0 {
		klog.V(5).Infof("%s pod request %q below namespace minimum; setting request to %q", corev1.ResourceCPU, overridden.String(), m.floor.CPU.String())
		clone := m.floor.CPU.DeepCopy()
		overridden = &clone
	}

	if m.IsCpuCeilingSpecified() && overridden.Cmp(*m.ceiling.CPU) > 0 {
		klog.V(5).Infof("%s pod request %q above namespace maximum; setting request to %q", corev1.ResourceCPU, overridden.String(), m.ceiling.CPU.String())
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
