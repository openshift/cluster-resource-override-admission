# Overview
`ClusterResourceOverride` Mutating Webhook Server.

## Developer Workflow
### Deploy
#### Prerequisites:
* `go`: `1.22` or above
* `jq`: Install [jq](https://stedolan.github.io/jq)
* `cfssl`: Install [cfssl](https://github.com/cloudflare/cfssl)
* `cfssljson`: Install [cfssl](https://github.com/cloudflare/cfssl)
* `podman`: Install [podman](https://podman.io/docs/installation)
  - Alternatively [docker](https://docs.docker.com/engine/install/) or [buildah](https://github.com/containers/buildah/blob/main/install.md)+`
* `kubectl` or `oc` Install from either
  * [OpenShift](https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html)
  * [Kubernetes](https://kubernetes.io/docs/reference/kubectl/)


`ClusterResourceOverride` Admission Webhook Operator is located at [cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator).

#### ClusterResourceOverride Parameters
The file `artifacts/configuration.yaml` is copied to `/etc/clusterresourceoverride/config/override.yaml` inside the docker image. If you want to change the parameters then edit the file and rebuild the image.
```yaml
apiVersion: v1
kind: ClusterResourceOverrideConfig
spec:
  memoryRequestToLimitPercent: 50
  cpuRequestToLimitPercent: 25
  limitCPUToMemoryPercent: 200
```

`ClusterResourceOverride` admission webhook server loads the configuration file when it starts. 

#### Build:
```bash
make build
```

Build and push image:
```bash
# make local-image DEV_IMAGE_REGISTRY={url to repository} IMAGE_TAG={tag}
# Specify your image builder with IMAGE_BUILDER=podman|docker|buildah. Defaults to podman.
make local-image IMAGE_TAG_BASE=docker.io/redhat/clusterresourceoverride IMAGE_VERSION=dev

make local-push IMAGE_TAG_BASE=docker.io/redhat/clusterresourceoverride IMAGE_VERSION=dev
```

#### Deploy
If you build your own image then edit the `deployment.yaml` file inside `artifacts/manifests` and point to the right `image`.
```
    spec:
      serviceAccountName: clusterresourceoverride
      containers:
        - name: clusterresourceoverride
          image: docker.io/redhat/clusterresourceoverride:dev
          imagePullPolicy: Always

```  

```bash
# generate manifests
make manifests

kubectl apply -f _output/manifests
```
