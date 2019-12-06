# Overview
`ClusterResourceOverride` Mutating Webhook Server.

## Developer Workflow
### Deploy
#### Prerequisites:
* `go`: `1.12` or above
* `jq`: Install [jq](https://stedolan.github.io/jq)
* `cfssl`: Install [cfssl](https://github.com/cloudflare/cfssl)
* `cfssljson`: Install [cfssl](https://github.com/cloudflare/cfssl)

#### ClusterResourceOverride Parameters
The file `artifacts/configuration.yaml` is copied to `/etc/clusterresourceoverride/config/override.yaml` inside the docker image. If you want to change the parameters then edit the file and rebuild the image.
```
apiVersion: v1
kind: ClusterResourceOverrideConfig
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
# make local-image IMAGE_REGISTRY={url to repository} IMAGE_TAG={tag}
make local-image IMAGE_REGISTRY=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev

make local-push IMAGE_REGISTRY=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev
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
