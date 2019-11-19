all: build
.PHONY: all

GO=GO111MODULE=on GOFLAGS=-mod=vendor go
GO_BUILD_BINDIR := bin

ARTIFACTS := "./artifacts/manifests"
OUTPUT_DIR := "./_output"
MANIFEST_DIR := "$(OUTPUT_DIR)/manifests"
CERT_FILE_PATH := "$(OUTPUT_DIR)/certs.yaml"
MANIFEST_SECRET_YAML := "$(MANIFEST_DIR)/004_secret.yaml"
MANIFEST_APISERVICE_YAML := "$(MANIFEST_DIR)/007_apiservice.yaml"
MANIFEST_MUTATING_WEBHOOK_YAML := "$(MANIFEST_DIR)/008_mutating.yaml"

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/library-go/alpha-build-machinery/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

# build image for ci
CI_IMAGE_REGISTRY ?=registry.svc.ci.openshift.org
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

	# generate certs
	./hack/generate-cert.sh "$(CERT_FILE_PATH)"

	# load the certs into the manifest yaml.
	./hack/load-cert-into-manifest.sh "$(CERT_FILE_PATH)" "$(MANIFEST_SECRET_YAML)" "$(MANIFEST_APISERVICE_YAML)" "$(MANIFEST_MUTATING_WEBHOOK_YAML)"

