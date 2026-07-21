package clusterresourceoverride

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// defaultResourceOverrideSyncTimeout bounds how long NewResourceOverrideLister waits for
// the informer's initial sync. Without a bound, a missing ResourceOverride CRD causes the
// informer's List/Watch calls to fail and retry forever, so cache.WaitForCacheSync (which
// only returns early when stopCh fires) would block NewAdmission()/webhook startup indefinitely.
const defaultResourceOverrideSyncTimeout = 30 * time.Second

var resourceOverrideGVR = schema.GroupVersionResource{
	Group:    "autoscaling.openshift.io",
	Version:  "v1",
	Resource: "resourceoverrides",
}

type ResourceOverrideView struct {
	Namespace string
	Name      string
	UID       types.UID
	Selector  *metav1.LabelSelector
	Spec      ClusterResourceOverrideSpec
}

type ResourceOverrideLister interface {
	ListByNamespace(namespace string) ([]ResourceOverrideView, error)
}

type resourceOverrideLister struct {
	indexer cache.Indexer
}

func NewResourceOverrideLister(
	dynFactory dynamicinformer.DynamicSharedInformerFactory,
	stopCh <-chan struct{},
) (ResourceOverrideLister, error) {
	return newResourceOverrideListerWithTimeout(dynFactory, stopCh, defaultResourceOverrideSyncTimeout)
}

func newResourceOverrideListerWithTimeout(
	dynFactory dynamicinformer.DynamicSharedInformerFactory,
	stopCh <-chan struct{},
	timeout time.Duration,
) (ResourceOverrideLister, error) {
	informer := dynFactory.ForResource(resourceOverrideGVR).Informer()
	go informer.Run(stopCh)

	if !waitForCacheSyncOrTimeout(stopCh, timeout, informer.HasSynced) {
		return nil, fmt.Errorf("ResourceOverride informer sync failed or timed out after %s", timeout)
	}

	return &resourceOverrideLister{indexer: informer.GetIndexer()}, nil
}

// waitForCacheSyncOrTimeout bounds cache.WaitForCacheSync to timeout so a missing
// ResourceOverride CRD (whose informer never syncs) cannot block startup indefinitely.
// It still respects stopCh for a clean early exit on real shutdown.
func waitForCacheSyncOrTimeout(stopCh <-chan struct{}, timeout time.Duration, hasSynced cache.InformerSynced) bool {
	bounded := make(chan struct{})
	timer := time.NewTimer(timeout)
	go func() {
		defer timer.Stop()
		select {
		case <-stopCh:
		case <-timer.C:
		}
		close(bounded)
	}()
	return cache.WaitForCacheSync(bounded, hasSynced)
}

func NewResourceOverrideListerOrNil(kubeClientConfig *restclient.Config, stopCh <-chan struct{}) ResourceOverrideLister {
	dynClient, err := dynamic.NewForConfig(kubeClientConfig)
	if err != nil {
		klog.Warningf("ResourceOverride informer disabled: %v", err)
		return nil
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, defaultResyncPeriod, metav1.NamespaceAll, nil)
	store, err := NewResourceOverrideLister(factory, stopCh)
	if err != nil {
		klog.Warningf("ResourceOverride informer disabled: %v", err)
		return nil
	}

	return store
}

func (s *resourceOverrideLister) ListByNamespace(namespace string) ([]ResourceOverrideView, error) {
	if s == nil || s.indexer == nil {
		return nil, nil
	}

	items, err := s.indexer.ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		return nil, err
	}

	out := make([]ResourceOverrideView, 0, len(items))
	for _, item := range items {
		u, ok := item.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		view, err := safeUnstructuredToROView(u)
		if err != nil {
			klog.Warningf("skip malformed ResourceOverride %s/%s: %v", namespace, u.GetName(), err)
			continue
		}
		if IsNamespaceExempt(view.Namespace) {
			continue
		}
		out = append(out, view)
	}
	return out, nil
}

// safeUnstructuredToROView wraps unstructuredToROView and converts any panic from the
// unstructured helpers into an error, so ListByNamespace can skip the malformed entry
// rather than crashing the webhook process (which would take down ClusterResourceOverride
// admission too, not just ResourceOverride).
func safeUnstructuredToROView(u *unstructured.Unstructured) (view ResourceOverrideView, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic decoding ResourceOverride: %v", r)
		}
	}()
	return unstructuredToROView(u)
}

func unstructuredToROView(u *unstructured.Unstructured) (ResourceOverrideView, error) {
	view := ResourceOverrideView{
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
		UID:       u.GetUID(),
	}

	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil || !found {
		return view, fmt.Errorf("missing spec")
	}

	if selectorObj, ok := spec["podSelector"]; ok && selectorObj != nil {
		selectorBytes, err := json.Marshal(selectorObj)
		if err != nil {
			return view, fmt.Errorf("failed to marshal podSelector: %v", err)
		}
		sel := &metav1.LabelSelector{}
		if err := json.Unmarshal(selectorBytes, sel); err != nil {
			return view, fmt.Errorf("failed to unmarshal podSelector: %v", err)
		}
		view.Selector = sel
	}

	override, found, err := unstructured.NestedMap(spec, "podResourceOverride")
	if err != nil || !found {
		return view, fmt.Errorf("missing podResourceOverride")
	}

	overrideBytes, err := json.Marshal(override)
	if err != nil {
		return view, fmt.Errorf("failed to marshal podResourceOverride: %v", err)
	}
	if err := json.Unmarshal(overrideBytes, &view.Spec); err != nil {
		return view, fmt.Errorf("failed to unmarshal podResourceOverride: %v", err)
	}

	return view, nil
}
