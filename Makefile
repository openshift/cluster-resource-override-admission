all: build
.PHONY: all

GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_BINDIR := bin

ARTIFACTS := "./artifacts/manifests"
OUTPUT_DIR := "./_output"
MANIFEST_DIR := "$(OUTPUT_DIR)/manifests"

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

# build image for ci
CI_IMAGE_REGISTRY ?=registry.ci.openshift.org
$(call build-image,cluster-resource-override-admission,$(CI_IMAGE_REGISTRY)/autoscaling/cluster-resource-override,./images/ci/Dockerfile,.)

# build image for dev use.
local-image:
	docker build -t $(LOCAL_IMAGE_REGISTRY):$(IMAGE_TAG) -f ./images/dev/Dockerfile.dev .

local-push:
	docker push $(LOCAL_IMAGE_REGISTRY):$(IMAGE_TAG)

# generate manifests for installing on a dev cluster.
manifests:
	rm -rf $(MANIFEST_DIR)
	mkdir -p $(MANIFEST_DIR)
	cp -r $(ARTIFACTS)/* $(MANIFEST_DIR)/

