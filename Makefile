OUTPUT_DIR := "./_output"
MANIFESTS_FILE_PATH := "$(OUTPUT_DIR)/manifests.yaml"
KUSTOMIZE_COPY_DIR := "$(OUTPUT_DIR)/tmp/"
ARTIFACTS := "./artifacts"
KUSTOMIZE_BASE := "$(ARTIFACTS)/base"
CERT_FILE_PATH := "$(KUSTOMIZE_COPY_DIR)/certs.configmap.yaml"
KUSTOMIZATION_YAML := "$(ARTIFACTS)/kustomization.yaml"
KUSTOMIZE_CONFIG_YAML := "$(ARTIFACTS)/kustomizeconfig.yaml"

build:
	go build -o bin/cluster-resource-override-admission-server github.com/openshift/cluster-resource-override-admission/cmd/clusterresourceoverrideadmissionserver

manifests:
	rm -rf $(KUSTOMIZE_COPY_DIR)
	mkdir -p $(KUSTOMIZE_COPY_DIR)
	cp -r $(KUSTOMIZE_BASE)/* $(KUSTOMIZE_COPY_DIR)/
	./hack/generate-cert.sh "$(CERT_FILE_PATH)"
	cp $(KUSTOMIZATION_YAML) $(KUSTOMIZE_COPY_DIR)/
	cp $(KUSTOMIZE_CONFIG_YAML) $(KUSTOMIZE_COPY_DIR)/
	kustomize build $(KUSTOMIZE_COPY_DIR) -o $(MANIFESTS_FILE_PATH)

clean:
	rm -rf $(OUTPUT_DIR)