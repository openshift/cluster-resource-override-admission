module github.com/openshift/cluster-resource-override-admission

go 1.12

require (
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/openshift/generic-admission-server v1.14.1-0.20210121183534-8cbb259223ad
	github.com/openshift/library-go v0.0.0-20191112181215-0597a29991ca
	github.com/stretchr/testify v1.4.0
	gomodules.xyz/jsonpatch/v2 v2.0.1
	k8s.io/api v0.19.5
	k8s.io/apimachinery v0.19.5
	k8s.io/client-go v0.19.5
	k8s.io/klog v1.0.0
)
