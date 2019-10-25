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
The file `artifacts/configuration.yaml` is copied to `/var/cluster-resource-override.yaml` inside the docker image. If you want to change the parameters then edit the file and rebuild the image.
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
# make image IMAGE_REPO={url to repository} IMAGE_TAG={tag}
make image IMAGE_REPO=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev

make push IMAGE_REPO=docker.io/redhat/clusterresourceoverride IMAGE_TAG=dev
```

#### Deploy
Prerequisites:
* kustomize: Install [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md) 

If you build your own image then edit the `deployment.yaml` file inside `artifacts/base/deploy` and point to the right `image`.
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

kubectl apply -f _output/manifests/deploy.yaml
kubectl apply -f _output/manifests/webhook.yaml
```
