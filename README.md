# Overview

`ClusterResourceOverride` Mutating Webhook Server.

## Developer Workflow

### Deploy

#### Prerequisites

* `go`: `1.12` or above
* `jq`: Install [jq](https://stedolan.github.io/jq)
* `cfssl`: Install [cfssl](https://github.com/cloudflare/cfssl)
* `cfssljson`: Install [cfssl](https://github.com/cloudflare/cfssl)

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

#### Build

```bash
make build
```

Build and push image:

```bash
# make local-image LOCAL_IMAGE_REGISTRY={url to repository} IMAGE_TAG={tag}
make local-image LOCAL_IMAGE_REGISTRY=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev

make local-push LOCAL_IMAGE_REGISTRY=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev
```

#### Deploying

If you build your own image then edit the `deployment.yaml` file inside `artifacts/manifests` and point to the right `image`.

```yaml
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

#### Testing SVT Relabel

```bash
kubectl create ns svt-test
kubectl edit ns svt-test
```

Add to the labels:

```yaml
podsvtoverride.admission.node.openshift.io/enabled: "true"
```
