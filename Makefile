ARTIFACTS := "./artifacts"
KUSTOMIZE_SOURCE := "$(ARTIFACTS)/base"
KUSTOMIZE_CONFIG_YAML := "$(ARTIFACTS)/kustomizeconfig.yaml"

OUTPUT_DIR := "./_output"
KUSTOMIZE_DEST := "$(OUTPUT_DIR)/tmp/"
KUSTOMIZE_DEST_DEPLOY := "$(KUSTOMIZE_DEST)/deploy"
KUSTOMIZE_DEST_WEBHOOK := "$(KUSTOMIZE_DEST)/webhook"

MANIFESTS_OUTPUT_DIR := "$(OUTPUT_DIR)/manifests"
CERT_FILE_PATH := "$(OUTPUT_DIR)/certs.configmap.yaml"

IMAGE_REPO ?= "docker.io/tohinkashem/clusterresourceoverride"
IMAGE_TAG ?= "dev"

export GO111MODULE=on

vendor:
	go mod vendor
	go mod tidy

build:
	go build -mod=vendor -o bin/cluster-resource-override-admission-server github.com/openshift/cluster-resource-override-admission/cmd/clusterresourceoverrideadmissionserver

image:
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) -f Dockerfile.dev .

push:
	docker push $(IMAGE_REPO):$(IMAGE_TAG)

unit:
	go test -v -mod=vendor ./pkg/...

manifests:
	rm -rf $(KUSTOMIZE_DEST)
	mkdir -p $(KUSTOMIZE_DEST)
	cp -r $(KUSTOMIZE_SOURCE)/* $(KUSTOMIZE_DEST)/

	./hack/generate-cert.sh "$(CERT_FILE_PATH)"
	cp $(CERT_FILE_PATH) $(KUSTOMIZE_DEST_DEPLOY)
	cp $(CERT_FILE_PATH) $(KUSTOMIZE_DEST_WEBHOOK)

	cp $(KUSTOMIZE_CONFIG_YAML) $(KUSTOMIZE_DEST_DEPLOY)
	cp $(KUSTOMIZE_CONFIG_YAML) $(KUSTOMIZE_DEST_WEBHOOK)

	mkdir -p $(MANIFESTS_OUTPUT_DIR)
	kustomize build $(KUSTOMIZE_DEST_DEPLOY) -o $(MANIFESTS_OUTPUT_DIR)/deploy.yaml
	kustomize build $(KUSTOMIZE_DEST_WEBHOOK) -o $(MANIFESTS_OUTPUT_DIR)/webhook.yaml

clean:
	rm -rf $(OUTPUT_DIR)
