package clusterresourceoverride

import (
	"strings"
)

// this a real shame to be special cased.
var (
	forbiddenNames    = []string{"openshift", "kubernetes", "kube"}
	forbiddenPrefixes = []string{"openshift-", "kubernetes-", "kube-"}
)

func IsNamespaceExempt(name string) bool {
	for _, s := range forbiddenNames {
		if name == s {
			return true
		}
	}

	for _, s := range forbiddenPrefixes {
		if strings.HasPrefix(name, s) {
			return true
		}
	}

	return false
}
