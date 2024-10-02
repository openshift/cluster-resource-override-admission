package main

import (
	"github.com/openshift/generic-admission-server/pkg/cmd"
	"k8s.io/klog/v2"
)

func main() {
	// jkyros: This is present only in 4.13 because the generic-admission-server switched to klogv2, and
	// the only version of generic-admission-server we have that's compatible with our kube 1.26.1 deps
	// doesn't initialize the flags right, so we initialize them here first so they aren't "unknown" and
	// won't cause the pod to crash.
	klog.InitFlags(nil)
	cmd.RunAdmissionServer(&clusterResourceOverrideHook{})
}
