package clusterresourceoverride

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNamespaceExempt(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{
			name: "openshift",
			want: true,
		},
		{
			name: "kubernetes",
			want: true,
		},
		{
			name: "kube",
			want: true,
		},
		{
			name: "foo",
			want: false,
		},
		{
			name: "openshift-marketplace",
			want: true,
		},
		{
			name: "kubernetes-system",
			want: true,
		},
		{
			name: "kube-dns",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNamespaceExempt(tt.name)
			assert.Equal(t, tt.want, got)
		})
	}
}
