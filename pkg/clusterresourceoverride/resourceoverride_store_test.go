package clusterresourceoverride

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func makeUnstructuredROWithUID(namespace, name, uid string, podSelector *metav1.LabelSelector, spec ClusterResourceOverrideSpec) *unstructured.Unstructured {
	u := makeUnstructuredRO(namespace, name, podSelector, spec)
	u.Object["metadata"].(map[string]interface{})["uid"] = uid
	return u
}

func makeUnstructuredRO(namespace, name string, podSelector *metav1.LabelSelector, spec ClusterResourceOverrideSpec) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "autoscaling.openshift.io/v1",
		"kind":       "ResourceOverride",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]interface{}{
			"podResourceOverride": map[string]interface{}{
				"limitCPUToMemoryPercent":     spec.LimitCPUToMemoryPercent,
				"cpuRequestToLimitPercent":    spec.CPURequestToLimitPercent,
				"memoryRequestToLimitPercent": spec.MemoryRequestToLimitPercent,
				"cpuRequestToRequestPercent":  spec.CPURequestToRequestPercent,
				"forceSelinuxRelabel":         spec.ForceSelinuxRelabel,
			},
		},
	}
	if podSelector != nil {
		selectorBytes, _ := json.Marshal(podSelector)
		var selectorMap interface{}
		_ = json.Unmarshal(selectorBytes, &selectorMap)
		obj["spec"].(map[string]interface{})["podSelector"] = selectorMap
	}
	return &unstructured.Unstructured{Object: obj}
}

func newTestLister(objects ...*unstructured.Unstructured) *resourceOverrideLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for _, obj := range objects {
		_ = indexer.Add(obj)
	}
	return &resourceOverrideLister{indexer: indexer}
}

func TestListByNamespace_SingleMatch(t *testing.T) {
	ro := makeUnstructuredRO("test-ns", "override-a", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "override-a", results[0].Name)
	assert.Equal(t, "test-ns", results[0].Namespace)
	assert.Equal(t, int64(50), results[0].Spec.CPURequestToLimitPercent)
}

func TestListByNamespace_NoMatch(t *testing.T) {
	ro := makeUnstructuredRO("other-ns", "override-a", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestListByNamespace_ExemptNamespaceSkipped(t *testing.T) {
	ro := makeUnstructuredRO("openshift-monitoring", "override-a", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("openshift-monitoring")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestListByNamespace_WithPodSelector(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "web"},
	}
	ro := makeUnstructuredRO("test-ns", "override-a", selector, ClusterResourceOverrideSpec{
		MemoryRequestToLimitPercent: 75,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Selector)
	assert.Equal(t, map[string]string{"app": "web"}, results[0].Selector.MatchLabels)
	assert.Equal(t, int64(75), results[0].Spec.MemoryRequestToLimitPercent)
}

func TestListByNamespace_MultipleInSameNamespace(t *testing.T) {
	roA := makeUnstructuredRO("test-ns", "override-a", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 25,
	})
	roB := makeUnstructuredRO("test-ns", "override-b", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(roA, roB)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestListByNamespace_NilStore(t *testing.T) {
	store := &resourceOverrideLister{indexer: nil}

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestListByNamespace_NilReceiver(t *testing.T) {
	var store *resourceOverrideLister

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestListByNamespace_MalformedObjectSkipped(t *testing.T) {
	malformed := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "bad-ro",
		},
		"spec": map[string]interface{}{},
	}}
	valid := makeUnstructuredRO("test-ns", "good-ro", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(malformed, valid)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "good-ro", results[0].Name)
}

func TestListByNamespace_NonUnstructuredItemSkipped(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	valid := makeUnstructuredRO("test-ns", "good-ro", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 30,
	})
	_ = indexer.Add(valid)
	_ = indexer.Add("not-an-unstructured-object")
	store := &resourceOverrideLister{indexer: indexer}

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "good-ro", results[0].Name)
}

func TestUnstructuredToROView_MissingSpec(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "bad",
		},
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing spec")
}

func TestUnstructuredToROView_SpecNotAMap(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "bad",
		},
		"spec": "not-a-map",
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing spec")
}

func TestUnstructuredToROView_NoPodSelector(t *testing.T) {
	u := makeUnstructuredRO("test-ns", "no-sel", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Nil(t, view.Selector)
	assert.Equal(t, int64(50), view.Spec.CPURequestToLimitPercent)
}

func TestUnstructuredToROView_ExplicitNullPodSelector(t *testing.T) {
	u := makeUnstructuredRO("test-ns", "null-sel", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 40,
	})
	u.Object["spec"].(map[string]interface{})["podSelector"] = nil

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Nil(t, view.Selector)
	assert.Equal(t, int64(40), view.Spec.CPURequestToLimitPercent)
}

func TestUnstructuredToROView_PodSelectorWithMatchExpressions(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "env",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"staging", "dev"},
			},
		},
	}
	u := makeUnstructuredRO("test-ns", "expr-sel", selector, ClusterResourceOverrideSpec{
		MemoryRequestToLimitPercent: 60,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	require.NotNil(t, view.Selector)
	require.Len(t, view.Selector.MatchExpressions, 1)
	assert.Equal(t, "env", view.Selector.MatchExpressions[0].Key)
	assert.Equal(t, metav1.LabelSelectorOpIn, view.Selector.MatchExpressions[0].Operator)
	assert.Equal(t, []string{"staging", "dev"}, view.Selector.MatchExpressions[0].Values)
}

func TestUnstructuredToROView_MissingPodResourceOverrideSpec(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "no-override",
		},
		"spec": map[string]interface{}{},
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing podResourceOverride")
}

func TestUnstructuredToROView_EmptyPodResourceOverride(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "empty-override",
		},
		"spec": map[string]interface{}{
			"podResourceOverride": map[string]interface{}{},
		},
	}}

	view, err := unstructuredToROView(u)
	require.NoError(t, err, "empty podResourceOverride is valid, all fields default to zero")
	assert.Equal(t, int64(0), view.Spec.CPURequestToLimitPercent)
	assert.Equal(t, int64(0), view.Spec.MemoryRequestToLimitPercent)
}

func TestUnstructuredToROView_MissingPodResourceOverrideKey(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "no-override-key",
		},
		"spec": map[string]interface{}{},
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing podResourceOverride")
}

func TestUnstructuredToROView_PartialSpecFields(t *testing.T) {
	u := makeUnstructuredRO("test-ns", "partial", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 75,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Equal(t, int64(75), view.Spec.CPURequestToLimitPercent)
	assert.Equal(t, int64(0), view.Spec.LimitCPUToMemoryPercent)
	assert.Equal(t, int64(0), view.Spec.MemoryRequestToLimitPercent)
	assert.Equal(t, int64(0), view.Spec.CPURequestToRequestPercent)
	assert.False(t, view.Spec.ForceSelinuxRelabel)
}

func TestUnstructuredToROView_ZeroValueSpec(t *testing.T) {
	u := makeUnstructuredRO("test-ns", "zero", nil, ClusterResourceOverrideSpec{})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Equal(t, int64(0), view.Spec.LimitCPUToMemoryPercent)
	assert.Equal(t, int64(0), view.Spec.CPURequestToLimitPercent)
	assert.Equal(t, int64(0), view.Spec.MemoryRequestToLimitPercent)
	assert.Equal(t, int64(0), view.Spec.CPURequestToRequestPercent)
	assert.False(t, view.Spec.ForceSelinuxRelabel)
}

func TestUnstructuredToROView_PodSelectorInvalidType(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling.openshift.io/v1",
		"kind":       "ResourceOverride",
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "bad-selector-type",
		},
		"spec": map[string]interface{}{
			"podSelector": "not-a-map",
			"podResourceOverride": map[string]interface{}{
				"spec": map[string]interface{}{
					"cpuRequestToLimitPercent": int64(50),
				},
			},
		},
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "podSelector")
}

func TestUnstructuredToROView_SpecFieldWrongType(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling.openshift.io/v1",
		"kind":       "ResourceOverride",
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "wrong-type",
		},
		"spec": map[string]interface{}{
			"podResourceOverride": map[string]interface{}{
				"cpuRequestToLimitPercent": "fifty",
			},
		},
	}}

	view, err := unstructuredToROView(u)
	// json.Unmarshal into int64 from string "fifty" returns an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
	_ = view
}

func TestUnstructuredToROView_PodResourceOverrideNotMap(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "test-ns",
			"name":      "bad-type",
		},
		"spec": map[string]interface{}{
			"podResourceOverride": "not-a-map",
		},
	}}

	_, err := unstructuredToROView(u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing podResourceOverride")
}

func TestListByNamespace_EmptyNamespace(t *testing.T) {
	ro := makeUnstructuredRO("test-ns", "ro-a", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("")
	require.NoError(t, err)
	assert.Empty(t, results, "empty namespace should return no results")
}

func TestListByNamespace_MultipleExemptNamespacePrefixes(t *testing.T) {
	ros := []*unstructured.Unstructured{
		makeUnstructuredRO("openshift-operators", "ro1", nil, ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50}),
		makeUnstructuredRO("kube-public", "ro2", nil, ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50}),
		makeUnstructuredRO("kubernetes-dashboard", "ro3", nil, ClusterResourceOverrideSpec{CPURequestToLimitPercent: 50}),
	}
	store := newTestLister(ros...)

	for _, ns := range []string{"openshift-operators", "kube-public", "kubernetes-dashboard"} {
		results, err := store.ListByNamespace(ns)
		require.NoError(t, err)
		assert.Empty(t, results, "exempt namespace %s should be filtered", ns)
	}
}

func TestUnstructuredToROView_AllFields(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"tier": "batch"},
	}
	u := makeUnstructuredRO("my-ns", "my-ro", selector, ClusterResourceOverrideSpec{
		LimitCPUToMemoryPercent:     200,
		CPURequestToLimitPercent:    25,
		MemoryRequestToLimitPercent: 50,
		CPURequestToRequestPercent:  30,
		ForceSelinuxRelabel:         true,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Equal(t, "my-ns", view.Namespace)
	assert.Equal(t, "my-ro", view.Name)
	assert.Equal(t, map[string]string{"tier": "batch"}, view.Selector.MatchLabels)
	assert.Equal(t, int64(200), view.Spec.LimitCPUToMemoryPercent)
	assert.Equal(t, int64(25), view.Spec.CPURequestToLimitPercent)
	assert.Equal(t, int64(50), view.Spec.MemoryRequestToLimitPercent)
	assert.Equal(t, int64(30), view.Spec.CPURequestToRequestPercent)
	assert.True(t, view.Spec.ForceSelinuxRelabel)
}

func TestUnstructuredToROView_UIDExtracted(t *testing.T) {
	u := makeUnstructuredROWithUID("test-ns", "my-ro", "abc-123-uid", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Equal(t, types.UID("abc-123-uid"), view.UID)
}

func TestUnstructuredToROView_MissingUID(t *testing.T) {
	u := makeUnstructuredRO("test-ns", "no-uid-ro", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})

	view, err := unstructuredToROView(u)
	require.NoError(t, err)
	assert.Empty(t, view.UID, "missing UID should result in empty UID, not a panic")
}

func TestNewResourceOverrideLister_MissingCRD_DoesNotBlockStartup(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		resourceOverrideGVR: "ResourceOverrideList",
	})

	notFoundErr := apierrors.NewNotFound(schema.GroupResource{Group: resourceOverrideGVR.Group, Resource: resourceOverrideGVR.Resource}, "")
	dynClient.PrependReactor("list", "resourceoverrides", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, notFoundErr
	})
	dynClient.PrependWatchReactor("resourceoverrides", func(action clienttesting.Action) (bool, watch.Interface, error) {
		return true, nil, notFoundErr
	})

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, time.Minute, metav1.NamespaceAll, nil)
	stopCh := make(chan struct{})
	defer close(stopCh)

	start := time.Now()
	store, err := newResourceOverrideListerWithTimeout(factory, stopCh, 200*time.Millisecond)
	elapsed := time.Since(start)

	assert.Nil(t, store, "missing CRD should soft-fail to a nil store, not a usable one")
	assert.Error(t, err)
	assert.Less(t, elapsed, 2*time.Second,
		"sync should time out quickly when the CRD is missing, not block webhook startup indefinitely")
}

func TestSafeUnstructuredToROView_RecoversFromPanic(t *testing.T) {
	// A nil *unstructured.Unstructured panics inside u.GetNamespace() (nil pointer
	// dereference). safeUnstructuredToROView must convert that into an error so a
	// single malformed/unexpected cache entry can't crash the whole webhook process.
	assert.NotPanics(t, func() {
		_, err := safeUnstructuredToROView(nil)
		assert.Error(t, err, "a nil unstructured object should produce an error, not a panic")
	})
}

func TestListByNamespace_UIDPreserved(t *testing.T) {
	ro := makeUnstructuredROWithUID("test-ns", "override-a", "uid-456", nil, ClusterResourceOverrideSpec{
		CPURequestToLimitPercent: 50,
	})
	store := newTestLister(ro)

	results, err := store.ListByNamespace("test-ns")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, types.UID("uid-456"), results[0].UID)
}
