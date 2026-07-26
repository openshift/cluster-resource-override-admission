package clusterresourceoverride

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/api/resource"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func TestSetNamespaceFloor(t *testing.T) {
	cpu := resource.MustParse("1000m")
	require.False(t, cpu.Equal(defaultCPUFloor), "bad test setup, default and namespace cpu floor must not be equal")

	memory := resource.MustParse("1Gi")
	require.False(t, memory.Equal(defaultMemoryFloor), "bad test setup, default and namespace memory floor must not be equal")

	namespaceFloor := &CPUMemory{
		CPU:    &cpu,
		Memory: &memory,
	}

	floorGot := setNamespaceFloor(namespaceFloor)
	require.NotNil(t, floorGot)

	assert.True(t, cpu.Equal(*floorGot.CPU))
	assert.True(t, memory.Equal(*floorGot.Memory))
}

// TestOverrideHookUpdatesNotApplicable tests to make sure that admission
// regards UPDATE requests to a pod as not applicable, as currently kubernetes
// doesn't allow the resource fields to be updated in-place
func TestAdmissionUpdateRequestsNotApplicable(t *testing.T) {
	admission := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation:   "UPDATE",
		Resource:    metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
		SubResource: "",
	}
	applicable := admission.IsApplicable(req)
	assert.False(t, applicable)
}

func TestSetNamespaceFloor_NilInput(t *testing.T) {
	floorGot := setNamespaceFloor(nil)
	require.NotNil(t, floorGot)
	assert.True(t, defaultCPUFloor.Equal(*floorGot.CPU), "nil input should use default CPU floor")
	assert.True(t, defaultMemoryFloor.Equal(*floorGot.Memory), "nil input should use default memory floor")
}

func TestSetNamespaceFloor_PartialNilCPU(t *testing.T) {
	memory := resource.MustParse("2Gi")
	floorGot := setNamespaceFloor(&CPUMemory{CPU: nil, Memory: &memory})
	require.NotNil(t, floorGot)
	assert.True(t, defaultCPUFloor.Equal(*floorGot.CPU), "nil CPU should use default")
	assert.True(t, memory.Equal(*floorGot.Memory), "non-nil memory should override default")
}

func TestSetNamespaceFloor_PartialNilMemory(t *testing.T) {
	cpu := resource.MustParse("500m")
	floorGot := setNamespaceFloor(&CPUMemory{CPU: &cpu, Memory: nil})
	require.NotNil(t, floorGot)
	assert.True(t, cpu.Equal(*floorGot.CPU), "non-nil CPU should override default")
	assert.True(t, defaultMemoryFloor.Equal(*floorGot.Memory), "nil memory should use default")
}

func TestGetConfiguration(t *testing.T) {
	config := &Config{CpuRequestToLimitRatio: 0.5}
	a := &clusterResourceOverrideAdmission{config: config}
	assert.Equal(t, config, a.GetConfiguration())
}

func TestIsApplicable_CreatePod(t *testing.T) {
	a := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Resource:  metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
	}
	assert.True(t, a.IsApplicable(req))
}

func TestIsApplicable_DeleteNotApplicable(t *testing.T) {
	a := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation: admissionv1.Delete,
		Resource:  metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
	}
	assert.False(t, a.IsApplicable(req))
}

func TestIsApplicable_SubResourceNotApplicable(t *testing.T) {
	a := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation:   admissionv1.Create,
		Resource:    metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
		SubResource: "status",
	}
	assert.False(t, a.IsApplicable(req))
}

func TestIsApplicable_NonPodResourceNotApplicable(t *testing.T) {
	a := clusterResourceOverrideAdmission{}
	req := &admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Resource:  metav1.GroupVersionResource{Resource: "deployments"},
	}
	assert.False(t, a.IsApplicable(req))
}

func newFakeNamespaceLister(namespaces ...*corev1.Namespace) corev1listers.NamespaceLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, ns := range namespaces {
		_ = indexer.Add(ns)
	}
	return corev1listers.NewNamespaceLister(indexer)
}

func TestIsExempt_OptedInNamespace(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-app",
			Labels: map[string]string{EnabledLabelName: "true"},
		},
	}
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister(ns)}

	req := &admissionv1.AdmissionRequest{Namespace: "my-app"}
	exempt, selinuxExempt, resp := a.IsExempt(req)

	assert.False(t, exempt, "opted-in namespace should not be exempt")
	assert.True(t, selinuxExempt, "selinux should still be exempt without its label")
	assert.Nil(t, resp)
}

func TestIsExempt_NoLabel(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
		},
	}
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister(ns)}

	req := &admissionv1.AdmissionRequest{Namespace: "my-app"}
	exempt, selinuxExempt, resp := a.IsExempt(req)

	assert.True(t, exempt, "namespace without label should be exempt")
	assert.True(t, selinuxExempt)
	assert.Nil(t, resp)
}

func TestIsExempt_LabelSetToFalse(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-app",
			Labels: map[string]string{EnabledLabelName: "false"},
		},
	}
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister(ns)}

	req := &admissionv1.AdmissionRequest{Namespace: "my-app"}
	exempt, _, resp := a.IsExempt(req)

	assert.True(t, exempt, "label=false should be exempt")
	assert.Nil(t, resp)
}

func TestIsExempt_NamespaceNotFound(t *testing.T) {
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister()}

	req := &admissionv1.AdmissionRequest{Namespace: "does-not-exist"}
	exempt, selinuxExempt, resp := a.IsExempt(req)

	assert.True(t, exempt)
	assert.True(t, selinuxExempt)
	assert.NotNil(t, resp, "namespace not found should set forbidden response")
}

func TestIsExempt_SelinuxLabelEnabled(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
			Labels: map[string]string{
				EnabledLabelName:           "true",
				SelinuxFixEnabledLabelName: "true",
			},
		},
	}
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister(ns)}

	req := &admissionv1.AdmissionRequest{Namespace: "my-app"}
	exempt, selinuxExempt, resp := a.IsExempt(req)

	assert.False(t, exempt)
	assert.False(t, selinuxExempt)
	assert.Nil(t, resp)
}

func TestIsExempt_OnlySelinuxLabel(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
			Labels: map[string]string{
				SelinuxFixEnabledLabelName: "true",
			},
		},
	}
	a := &clusterResourceOverrideAdmission{nsLister: newFakeNamespaceLister(ns)}

	req := &admissionv1.AdmissionRequest{Namespace: "my-app"}
	exempt, selinuxExempt, resp := a.IsExempt(req)

	assert.True(t, exempt, "CRO exempt without CRO label")
	assert.False(t, selinuxExempt, "selinux not exempt with selinux label")
	assert.Nil(t, resp)
}

func newFakeLimitRangeLister() corev1listers.LimitRangeLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	return corev1listers.NewLimitRangeLister(indexer)
}

func makeAdmissionRequest(namespace string, pod *corev1.Pod) *admissionv1.AdmissionRequest {
	podBytes, _ := json.Marshal(pod)
	return &admissionv1.AdmissionRequest{
		Namespace: namespace,
		Resource:  metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: podBytes},
	}
}

func TestAdmit_ROMatchOverridesClusterConfig(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio:    0.25,
		MemoryRequestToLimitRatio: 0.50,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-a",
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent:    80,
					MemoryRequestToLimitPercent: 90,
				},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed, "admission should be allowed")
	require.NotNil(t, resp.Patch, "should produce a patch")

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "800m", cpuReq.String(), "CPU request should be 80%% of 1000m limit (RO config, not CRO 25%%)")

	memReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory]
	oneGi := int64(1024 * 1024 * 1024)
	expectedMem := oneGi * 90 / 100
	expectedMem = expectedMem - (expectedMem % (1024 * 1024))
	assert.Equal(t, resource.NewQuantity(expectedMem, resource.BinarySI).String(), memReq.String(),
		"Memory request should be 90%% of 1Gi (RO config, not CRO 50%%)")
}

func TestAdmit_NoROMatch_FallsBackToClusterConfig(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio:    0.25,
		MemoryRequestToLimitRatio: 0.50,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-a",
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 80,
				},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "250m", cpuReq.String(), "CPU request should be 25%% of limit (CRO fallback)")
}

func TestAdmit_NilResolver_UsesClusterConfig(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	a := &clusterResourceOverrideAdmission{
		config:   croConfig,
		resolver: nil,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "250m", cpuReq.String(), "nil resolver should use CRO config")
}

func TestAdmit_MultipleROMatches_LexicographicWinner(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-beta",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 90},
			},
			{
				Namespace: "test-ns",
				Name:      "override-alpha",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", cpuReq.String(), "override-alpha (50%%) should win over override-beta (90%%)")
}

func TestAdmit_EmptySelector_MatchesAllPods(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "wildcard",
				Selector:  &metav1.LabelSelector{},
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 60},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: map[string]string{"tier": "anything"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "600m", cpuReq.String(), "empty selector should match, using RO 60%%")
}

func newFakeLimitRangeListerWithRange(namespace string, min, max resource.Quantity, resourceName corev1.ResourceName) corev1listers.LimitRangeLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "limits",
			Namespace: namespace,
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Min:  corev1.ResourceList{resourceName: min},
					Max:  corev1.ResourceList{resourceName: max},
				},
			},
		},
	}
	_ = indexer.Add(lr)
	return corev1listers.NewLimitRangeLister(indexer)
}

func TestAdmit_ROWithLimitRangeFloor(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "low-ratio",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 5},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeListerWithRange(
				"test-ns",
				resource.MustParse("200m"),
				resource.MustParse("4000m"),
				corev1.ResourceCPU,
			),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "200m", cpuReq.String(),
		"RO config 5%% of 1000m = 50m, but LimitRange floor is 200m, so floor should win")
}

func TestAdmit_ROWithLimitRangeCeiling(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "high-ratio",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 100},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeListerWithRange(
				"test-ns",
				resource.MustParse("10m"),
				resource.MustParse("500m"),
				corev1.ResourceCPU,
			),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", cpuReq.String(),
		"RO config 100%% of 1000m = 1000m, but LimitRange ceiling is 500m, so ceiling should win")
}

func TestAdmit_ROWithCPURequestToRequestPercent(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "req-to-req",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToRequestPercent: 50},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", cpuReq.String(),
		"CPURequestToRequestPercent=50 should halve the original 1000m request")
}

func TestAdmit_MalformedPodJSON(t *testing.T) {
	a := &clusterResourceOverrideAdmission{
		config: &Config{CpuRequestToLimitRatio: 0.25},
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	req := &admissionv1.AdmissionRequest{
		Namespace: "test-ns",
		Resource:  metav1.GroupVersionResource{Resource: string(corev1.ResourcePods)},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{invalid json}`)},
	}
	resp := a.Admit(req)

	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Result)
	assert.Equal(t, int32(400), resp.Result.Code, "malformed JSON should return 400 Bad Request")
}

func TestAdmit_InitContainers(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.5,
	}

	a := &clusterResourceOverrideAdmission{
		config:   croConfig,
		resolver: nil,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "init",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2000m"),
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	initCpuReq := patched.Spec.InitContainers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "1", initCpuReq.String(), "init container CPU request should be 50%% of 2000m")

	mainCpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", mainCpuReq.String(), "main container CPU request should be 50%% of 1000m")
}

func TestAdmit_MultipleContainers(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	a := &clusterResourceOverrideAdmission{
		config:   croConfig,
		resolver: nil,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
					},
				},
				{
					Name: "sidecar",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	appCpu := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "250m", appCpu.String(), "app container: 25%% of 1000m")

	sidecarCpu := patched.Spec.Containers[1].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "125m", sidecarCpu.String(), "sidecar container: 25%% of 500m")
}

func TestAdmit_PodWithExistingRequests(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.5,
	}

	a := &clusterResourceOverrideAdmission{
		config:   croConfig,
		resolver: nil,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1000m"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("200m"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuReq := patched.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	assert.Equal(t, "500m", cpuReq.String(),
		"existing request should be overridden to 50%% of limit, not preserved")
}

func TestAdmit_PodWithNoLimits_NoMutation(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio:    0.5,
		MemoryRequestToLimitRatio: 0.5,
	}

	a := &clusterResourceOverrideAdmission{
		config:   croConfig,
		resolver: nil,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      "main",
					Resources: corev1.ResourceRequirements{},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
}

func TestAdmit_ROWithLimitCPUToMemory(t *testing.T) {
	croConfig := &Config{
		CpuRequestToLimitRatio: 0.25,
	}

	roLister := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "mem-to-cpu",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					LimitCPUToMemoryPercent: 100,
				},
			},
		},
	}}

	a := &clusterResourceOverrideAdmission{
		config: croConfig,
		limitQuerier: &namespaceLimitQuerier{
			limitRangesLister: newFakeLimitRangeLister(),
		},
		resolver: NewOverrideResolver(roLister, croConfig, nil),
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
	req := makeAdmissionRequest("test-ns", pod)
	resp := a.Admit(req)

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Patch)

	var patched corev1.Pod
	podBytes, _ := json.Marshal(pod)
	applyJSONPatch(t, podBytes, resp.Patch, &patched)

	cpuLimit := patched.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
	assert.Equal(t, "1", cpuLimit.String(),
		"LimitCPUToMemory at 100%% with 1Gi should set CPU limit to 1 core")
}

func applyJSONPatch(t *testing.T, original, patchBytes []byte, target interface{}) {
	t.Helper()
	patch, err := jsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)
	result, err := patch.Apply(original)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(result, target))
}
