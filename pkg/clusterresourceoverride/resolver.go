package clusterresourceoverride

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
)

var resourceOverrideGVK = schema.GroupVersionKind{
	Group:   "autoscaling.openshift.io",
	Version: "v1",
	Kind:    "ResourceOverride",
}

type OverrideResolver struct {
	lister        ResourceOverrideLister
	clusterConfig *Config
	recorder      record.EventRecorder
}

func NewOverrideResolver(lister ResourceOverrideLister, clusterConfig *Config, recorder record.EventRecorder) *OverrideResolver {
	return &OverrideResolver{
		lister:        lister,
		clusterConfig: clusterConfig,
		recorder:      recorder,
	}
}

func (r *OverrideResolver) ResolveConfig(namespace string, pod *corev1.Pod) *Config {
	if r.lister == nil {
		klog.V(5).Infof("namespace=%s ResourceOverride lister is nil, using ClusterResourceOverride config", namespace)
		return r.clusterConfig
	}

	if pod == nil {
		klog.Warningf("namespace=%s pod is nil, using ClusterResourceOverride config", namespace)
		return r.clusterConfig
	}

	candidates, err := r.lister.ListByNamespace(namespace)
	if err != nil {
		klog.Warningf("namespace=%s failed to list ResourceOverrides: %v; falling back to ClusterResourceOverride config", namespace, err)
		return r.clusterConfig
	}

	if len(candidates) == 0 {
		klog.V(5).Infof("namespace=%s no ResourceOverrides found, using ClusterResourceOverride config", namespace)
		return r.clusterConfig
	}

	podLabels := pod.Labels
	if podLabels == nil {
		podLabels = map[string]string{}
	}

	matched := filterMatchingROs(candidates, podLabels)
	if len(matched) == 0 {
		klog.V(5).Infof("namespace=%s no ResourceOverrides match pod labels, using ClusterResourceOverride config", namespace)
		return r.clusterConfig
	}

	winner := r.selectWinner(matched, namespace, pod.Name)

	config := ConvertExternalConfig(&ClusterResourceOverride{Spec: winner.Spec})
	klog.V(5).Infof("namespace=%s using ResourceOverride %q config: %s", namespace, winner.Name, config)
	return config
}

func filterMatchingROs(candidates []ResourceOverrideView, podLabels map[string]string) []ResourceOverrideView {
	var matched []ResourceOverrideView
	for _, ro := range candidates {
		if IsNamespaceExempt(ro.Namespace) {
			klog.V(5).Infof("namespace=%s ResourceOverride %q skipped: exempt namespace", ro.Namespace, ro.Name)
			continue
		}
		if ro.Selector == nil || isEmptySelector(ro.Selector) {
			matched = append(matched, ro)
			continue
		}
		sel, err := metav1.LabelSelectorAsSelector(ro.Selector)
		if err != nil {
			klog.Warningf("namespace=%s ResourceOverride %q has invalid podSelector: %v; skipping", ro.Namespace, ro.Name, err)
			continue
		}
		if sel.Matches(labels.Set(podLabels)) {
			matched = append(matched, ro)
		}
	}
	return matched
}

func isEmptySelector(sel *metav1.LabelSelector) bool {
	return len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0
}

func (r *OverrideResolver) selectWinner(matched []ResourceOverrideView, namespace string, podName string) ResourceOverrideView {
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})

	winner := matched[0]

	if len(matched) > 1 {
		for _, loser := range matched[1:] {
			klog.Warningf("namespace=%s pod=%s matched multiple ResourceOverrides; %q selected, %q ignored",
				namespace, podName, winner.Name, loser.Name)

			if r.recorder != nil {
				eventObj := roViewToEventObject(loser)
				r.recorder.Eventf(eventObj, corev1.EventTypeWarning, "OverrideConflict",
					"Pod %s/%s matched multiple ResourceOverrides; %q selected, %q ignored",
					namespace, podName, winner.Name, loser.Name)
			}
		}
	}

	return winner
}

func roViewToEventObject(view ResourceOverrideView) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(resourceOverrideGVK)
	u.SetNamespace(view.Namespace)
	u.SetName(view.Name)
	u.SetUID(view.UID)
	return u
}
