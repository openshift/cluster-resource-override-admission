package clusterresourceoverride

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

var clusterConfig = &Config{
	LimitCPUToMemoryRatio:     0.5,
	CpuRequestToLimitRatio:    0.25,
	MemoryRequestToLimitRatio: 0.5,
}

func makePod(name string, podLabels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: podLabels,
		},
	}
}

type fakeROLister struct {
	views map[string][]ResourceOverrideView
	err   error
}

func (f *fakeROLister) ListByNamespace(namespace string) ([]ResourceOverrideView, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.views[namespace], nil
}

func TestResolveConfig_NilStore(t *testing.T) {
	resolver := NewOverrideResolver(nil, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "nil store should return cluster config")
}

func TestResolveConfig_StoreError(t *testing.T) {
	store := &fakeROLister{err: fmt.Errorf("connection refused")}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "store error should fall back to cluster config")
}

func TestResolveConfig_NoROsInNamespace(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "no ROs should fall back to cluster config")
}

func TestResolveConfig_SingleMatchOverridesCluster(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
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
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	require.NotEqual(t, clusterConfig, got, "matching RO should override cluster config")
	assert.InDelta(t, 0.8, got.CpuRequestToLimitRatio, 0.001)
	assert.InDelta(t, 0.9, got.MemoryRequestToLimitRatio, 0.001)
	assert.InDelta(t, 0.0, got.LimitCPUToMemoryRatio, 0.001)
}

func TestResolveConfig_NoMatchFallsBackToCluster(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
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
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "non-matching RO should fall back to cluster config")
}

func TestResolveConfig_EmptySelectorMatchesAllPods(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-all",
				Selector:  &metav1.LabelSelector{},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 60,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.6, got.CpuRequestToLimitRatio, 0.001, "empty selector should match all pods")
}

func TestResolveConfig_NilSelectorMatchesAllPods(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-nil-sel",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 70,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.7, got.CpuRequestToLimitRatio, 0.001, "nil selector should match all pods")
}

func TestResolveConfig_MultipleMatches_FirstLexicographicallyWins(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-beta",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 90,
				},
			},
			{
				Namespace: "test-ns",
				Name:      "override-alpha",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 50,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.5, got.CpuRequestToLimitRatio, 0.001,
		"override-alpha (lexicographically first) should win with 50%%")
}

func TestResolveConfig_InvalidSelector_SkippedAndLogged(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "bad-selector",
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: metav1.LabelSelectorOperator("InvalidOp"),
							Values:   []string{"web"},
						},
					},
				},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 99,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "invalid selector should be skipped, falling back to cluster config")
}

func TestResolveConfig_InvalidSelectorSkipped_ValidSelectorUsed(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "bad-selector",
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: metav1.LabelSelectorOperator("InvalidOp"),
							Values:   []string{"web"},
						},
					},
				},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 99,
				},
			},
			{
				Namespace: "test-ns",
				Name:      "valid-selector",
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 40,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.4, got.CpuRequestToLimitRatio, 0.001,
		"valid-selector should be used even when bad-selector is skipped")
}

func TestResolveConfig_PodWithNilLabels(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "specific-selector",
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 80,
				},
			},
			{
				Namespace: "test-ns",
				Name:      "wildcard",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 60,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.6, got.CpuRequestToLimitRatio, 0.001,
		"pod with nil labels should only match nil/empty selectors")
}

func TestResolveConfig_MatchExpressionsSelector(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "expr-override",
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "env",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"staging", "dev"},
						},
					},
				},
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 35,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{"env": "staging"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.35, got.CpuRequestToLimitRatio, 0.001,
		"matchExpressions In should match")
}

func TestResolveConfig_AllSpecFields(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "full-override",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					LimitCPUToMemoryPercent:     200,
					CPURequestToLimitPercent:    25,
					MemoryRequestToLimitPercent: 50,
					CPURequestToRequestPercent:  30,
					ForceSelinuxRelabel:         true,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 2.0, got.LimitCPUToMemoryRatio, 0.001)
	assert.InDelta(t, 0.25, got.CpuRequestToLimitRatio, 0.001)
	assert.InDelta(t, 0.5, got.MemoryRequestToLimitRatio, 0.001)
	assert.InDelta(t, 0.3, got.CpuRequestToRequestRatio, 0.001)
	assert.True(t, got.ForceSelinuxRelabel)
}

func TestResolveConfig_ExemptNamespace_SkippedDuringSelection(t *testing.T) {
	exemptNamespaces := []string{
		"openshift-monitoring",
		"openshift-ingress",
		"kubernetes-system",
		"kube-system",
		"openshift",
		"kubernetes",
		"kube",
	}

	for _, ns := range exemptNamespaces {
		t.Run(ns, func(t *testing.T) {
			store := &fakeROLister{views: map[string][]ResourceOverrideView{
				ns: {
					{
						Namespace: ns,
						Name:      "override-in-exempt-ns",
						Selector:  nil,
						Spec: ClusterResourceOverrideSpec{
							CPURequestToLimitPercent: 99,
						},
					},
				},
			}}
			resolver := NewOverrideResolver(store, clusterConfig, nil)

			pod := makePod("test-pod", map[string]string{"app": "web"})
			got := resolver.ResolveConfig(ns, pod)

			assert.Equal(t, clusterConfig, got,
				"ResourceOverride in exempt namespace %q should be skipped, falling back to cluster config", ns)
		})
	}
}

func TestResolveConfig_ExemptNamespaceRO_NonExemptROUsed(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"my-app": {
			{
				Namespace: "my-app",
				Name:      "valid-override",
				Selector:  nil,
				Spec: ClusterResourceOverrideSpec{
					CPURequestToLimitPercent: 60,
				},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("my-app", pod)

	assert.InDelta(t, 0.6, got.CpuRequestToLimitRatio, 0.001,
		"non-exempt namespace RO should be used normally")
}

func TestFilterMatchingROs_ExemptNamespaceFiltered(t *testing.T) {
	candidates := []ResourceOverrideView{
		{Namespace: "openshift-monitoring", Name: "exempt-ro", Selector: nil},
		{Namespace: "my-app", Name: "valid-ro", Selector: nil},
		{Namespace: "kube-system", Name: "another-exempt", Selector: nil},
	}
	matched := filterMatchingROs(candidates, "my-app", map[string]string{})
	require.Len(t, matched, 1)
	assert.Equal(t, "valid-ro", matched[0].Name)
}

func TestResolveConfig_NilPod(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-a",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 80},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	got := resolver.ResolveConfig("test-ns", nil)
	assert.Equal(t, clusterConfig, got, "nil pod should fall back to cluster config without panicking")
}

func TestResolveConfig_NilClusterConfig(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{}}
	resolver := NewOverrideResolver(store, nil, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Nil(t, got, "nil clusterConfig should be returned as-is when no ROs match")
}

func TestResolveConfig_EmptyNamespace(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("", pod)

	assert.Equal(t, clusterConfig, got, "empty namespace should fall back to cluster config")
}

func TestResolveConfig_PodWithEmptyLabelsMap(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "specific",
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 80},
			},
			{
				Namespace: "test-ns",
				Name:      "wildcard",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 60},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", map[string]string{})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.6, got.CpuRequestToLimitRatio, 0.001,
		"empty labels map should behave same as nil labels")
}

func TestResolveConfig_CombinedMatchLabelsAndMatchExpressions(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "combined-selector",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"prod", "staging"}},
					},
				},
				Spec: ClusterResourceOverrideSpec{CPURequestToLimitPercent: 45},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	t.Run("both match", func(t *testing.T) {
		pod := makePod("pod1", map[string]string{"app": "web", "env": "prod"})
		got := resolver.ResolveConfig("test-ns", pod)
		assert.InDelta(t, 0.45, got.CpuRequestToLimitRatio, 0.001)
	})

	t.Run("labels match but expression does not", func(t *testing.T) {
		pod := makePod("pod2", map[string]string{"app": "web", "env": "dev"})
		got := resolver.ResolveConfig("test-ns", pod)
		assert.Equal(t, clusterConfig, got)
	})

	t.Run("expression matches but label does not", func(t *testing.T) {
		pod := makePod("pod3", map[string]string{"app": "api", "env": "prod"})
		got := resolver.ResolveConfig("test-ns", pod)
		assert.Equal(t, clusterConfig, got)
	})
}

func TestResolveConfig_DoesNotExistExpression(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "no-gpu",
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "gpu", Operator: metav1.LabelSelectorOpDoesNotExist},
					},
				},
				Spec: ClusterResourceOverrideSpec{CPURequestToLimitPercent: 55},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	t.Run("label absent - matches", func(t *testing.T) {
		pod := makePod("pod1", map[string]string{"app": "web"})
		got := resolver.ResolveConfig("test-ns", pod)
		assert.InDelta(t, 0.55, got.CpuRequestToLimitRatio, 0.001)
	})

	t.Run("label present - no match", func(t *testing.T) {
		pod := makePod("pod2", map[string]string{"gpu": "true"})
		got := resolver.ResolveConfig("test-ns", pod)
		assert.Equal(t, clusterConfig, got)
	})
}

func TestResolveConfig_CrossNamespaceROIgnored(t *testing.T) {
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "other-ns",
				Name:      "cross-ns-ro",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 99},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, nil)

	pod := makePod("test-pod", nil)
	got := resolver.ResolveConfig("test-ns", pod)

	assert.Equal(t, clusterConfig, got, "RO from a different namespace should be ignored, falling back to cluster config")
}

func TestFilterMatchingROs_EmptyInput(t *testing.T) {
	matched := filterMatchingROs(nil, "test-ns", map[string]string{"app": "web"})
	assert.Empty(t, matched)
}

func TestFilterMatchingROs_EmptySliceInput(t *testing.T) {
	matched := filterMatchingROs([]ResourceOverrideView{}, "test-ns", map[string]string{"app": "web"})
	assert.Empty(t, matched)
}

func TestFilterMatchingROs_AllExempt(t *testing.T) {
	candidates := []ResourceOverrideView{
		{Namespace: "kube-system", Name: "ro1", Selector: nil},
		{Namespace: "kube-system", Name: "ro2", Selector: nil},
	}
	matched := filterMatchingROs(candidates, "kube-system", map[string]string{})
	assert.Empty(t, matched)
}

func TestFilterMatchingROs_NilPodLabels(t *testing.T) {
	candidates := []ResourceOverrideView{
		{Namespace: "test-ns", Name: "wildcard", Selector: nil},
		{Namespace: "test-ns", Name: "specific", Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "web"},
		}},
	}
	matched := filterMatchingROs(candidates, "test-ns", nil)
	require.Len(t, matched, 1)
	assert.Equal(t, "wildcard", matched[0].Name)
}

func newTestResolver(store ResourceOverrideLister, config *Config, recorder record.EventRecorder) *OverrideResolver {
	return NewOverrideResolver(store, config, recorder)
}

func TestSelectWinner_SingleElement(t *testing.T) {
	resolver := newTestResolver(nil, clusterConfig, nil)
	input := []ResourceOverrideView{
		{Name: "only-one", Spec: ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50}},
	}
	winner := resolver.selectWinner(input, "ns", "pod")
	assert.Equal(t, "only-one", winner.Name)
}

func TestSelectWinner_LexicographicOrder(t *testing.T) {
	resolver := newTestResolver(nil, clusterConfig, nil)
	input := []ResourceOverrideView{
		{Name: "z-last"},
		{Name: "a-first"},
		{Name: "m-middle"},
	}
	winner := resolver.selectWinner(input, "ns", "pod")
	assert.Equal(t, "a-first", winner.Name)
}

func TestSelectWinner_SamePrefix(t *testing.T) {
	resolver := newTestResolver(nil, clusterConfig, nil)
	input := []ResourceOverrideView{
		{Name: "override-2"},
		{Name: "override-10"},
		{Name: "override-1"},
	}
	winner := resolver.selectWinner(input, "ns", "pod")
	assert.Equal(t, "override-1", winner.Name, "lexicographic: '1' < '10' < '2'")
}

func TestResolveConfig_MultipleMatches_EmitsWarningEvent(t *testing.T) {
	fakeRecorder := record.NewFakeRecorder(10)
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-beta",
				UID:       "uid-beta",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 90},
			},
			{
				Namespace: "test-ns",
				Name:      "override-alpha",
				UID:       "uid-alpha",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, fakeRecorder)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.5, got.CpuRequestToLimitRatio, 0.001,
		"override-alpha should win")

	select {
	case event := <-fakeRecorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "OverrideConflict")
		assert.Contains(t, event, "override-beta")
		assert.Contains(t, event, "test-pod")
	default:
		t.Fatal("expected a Warning event for multiple ResourceOverride matches, but none was emitted")
	}
}

func TestResolveConfig_ThreeMatches_LexicographicWinner_EventListsAllIgnored(t *testing.T) {
	fakeRecorder := record.NewFakeRecorder(10)
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-gamma",
				UID:       "uid-gamma",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 30},
			},
			{
				Namespace: "test-ns",
				Name:      "override-alpha",
				UID:       "uid-alpha",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50},
			},
			{
				Namespace: "test-ns",
				Name:      "override-beta",
				UID:       "uid-beta",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 70},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, fakeRecorder)

	pod := makePod("my-pod", map[string]string{"app": "web"})
	got := resolver.ResolveConfig("test-ns", pod)

	assert.InDelta(t, 0.5, got.CpuRequestToLimitRatio, 0.001,
		"override-alpha (lexicographically first) should win with 50%%")

	// One event is fired per losing ResourceOverride (override-beta, override-gamma),
	// not a single combined event, so drain both off the channel.
	var events []string
	for i := 0; i < 2; i++ {
		select {
		case event := <-fakeRecorder.Events:
			events = append(events, event)
		default:
			t.Fatalf("expected 2 Warning events for 3-way ResourceOverride conflict, got %d", i)
		}
	}
	combined := strings.Join(events, " | ")
	assert.Contains(t, combined, "Warning")
	assert.Contains(t, combined, "OverrideConflict")
	assert.Contains(t, combined, "override-beta", "an event should reference ignored RO override-beta")
	assert.Contains(t, combined, "override-gamma", "an event should reference ignored RO override-gamma")
	assert.Contains(t, combined, "my-pod", "events should reference the pod name")
}

func TestSelectWinner_LexicographicOrder_EmitsEventWithAllIgnored(t *testing.T) {
	fakeRecorder := record.NewFakeRecorder(10)
	resolver := newTestResolver(nil, clusterConfig, fakeRecorder)
	input := []ResourceOverrideView{
		{Namespace: "ns", Name: "z-last", UID: "uid-z"},
		{Namespace: "ns", Name: "a-first", UID: "uid-a"},
		{Namespace: "ns", Name: "m-middle", UID: "uid-m"},
	}
	winner := resolver.selectWinner(input, "ns", "pod")
	assert.Equal(t, "a-first", winner.Name, "lexicographically first should win")

	// One event per loser (m-middle, z-last); the winner (a-first) is named as
	// "selected" in each message but must never appear as the "ignored" party.
	var events []string
	for i := 0; i < 2; i++ {
		select {
		case event := <-fakeRecorder.Events:
			events = append(events, event)
		default:
			t.Fatalf("expected 2 Warning events, got %d", i)
		}
	}
	combined := strings.Join(events, " | ")
	assert.Contains(t, combined, "m-middle", "an event should list ignored m-middle")
	assert.Contains(t, combined, "z-last", "an event should list ignored z-last")
	assert.NotContains(t, combined, `"a-first" ignored`, "winner should never appear as the ignored party")
}

func TestSelectWinner_SingleElement_NoEvent(t *testing.T) {
	fakeRecorder := record.NewFakeRecorder(10)
	resolver := newTestResolver(nil, clusterConfig, fakeRecorder)
	input := []ResourceOverrideView{
		{Namespace: "ns", Name: "only-one", UID: "uid-1"},
	}
	resolver.selectWinner(input, "ns", "pod")

	select {
	case event := <-fakeRecorder.Events:
		t.Fatalf("expected no event for single element, but got: %s", event)
	default:
	}
}

func TestResolveConfig_SingleMatch_NoEvent(t *testing.T) {
	fakeRecorder := record.NewFakeRecorder(10)
	store := &fakeROLister{views: map[string][]ResourceOverrideView{
		"test-ns": {
			{
				Namespace: "test-ns",
				Name:      "override-only",
				UID:       "uid-only",
				Selector:  nil,
				Spec:      ClusterResourceOverrideSpec{CPURequestToLimitPercent: 70},
			},
		},
	}}
	resolver := NewOverrideResolver(store, clusterConfig, fakeRecorder)

	pod := makePod("test-pod", map[string]string{"app": "web"})
	resolver.ResolveConfig("test-ns", pod)

	select {
	case event := <-fakeRecorder.Events:
		t.Fatalf("expected no event for single match, but got: %s", event)
	default:
	}
}

func TestIsEmptySelector(t *testing.T) {
	assert.True(t, isEmptySelector(&metav1.LabelSelector{}))
	assert.True(t, isEmptySelector(&metav1.LabelSelector{
		MatchLabels:      map[string]string{},
		MatchExpressions: []metav1.LabelSelectorRequirement{},
	}), "explicitly empty maps should also be considered empty")
	assert.False(t, isEmptySelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"a": "b"},
	}))
	assert.False(t, isEmptySelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "x", Operator: metav1.LabelSelectorOpExists},
		},
	}))
}
