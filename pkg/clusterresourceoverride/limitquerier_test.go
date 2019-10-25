package clusterresourceoverride

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"testing"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestQueryMinimum(t *testing.T) {
	tests := []struct{
		name string
		objects []runtime.Object
	}{
		{

		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := k8sfake.NewSimpleClientset(test.objects...)

			factory := informers.NewSharedInformerFactory(client, defaultResyncPeriod)
			factory.Core().V1().Namespaces()

		})
	}
}
