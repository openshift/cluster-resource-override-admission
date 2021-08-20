module github.com/openshift/cluster-resource-override-admission

go 1.16

require (
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/openshift/build-machinery-go v0.0.0-20210806203541-4ea9b6da3a37
	github.com/openshift/generic-admission-server v1.14.1-0.20210422140326-da96454c926d
	github.com/openshift/library-go v0.0.0-20210819104210-e14e06ba8d47 // indirect
	github.com/stretchr/testify v1.7.0
	gomodules.xyz/jsonpatch/v2 v2.0.1
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/klog v1.0.0
)
