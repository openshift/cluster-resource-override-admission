all: build
.PHONY: all

GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_BINDIR := bin

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/library-go/alpha-build-machinery/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

# generate image targets
IMAGE_REGISTRY ?=registry.svc.ci.openshift.org
$(call build-image,cluster-resource-override-admission,$(IMAGE_REGISTRY)/autoscaling/cluster-resource-override,./images/ci/Dockerfile,.)
