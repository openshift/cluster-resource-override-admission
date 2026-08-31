# Cluster Resource Override Admission

**ClusterResourceOverride Admission** is the operand workload deployed and managed by the [cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator). When pods are created in an OpenShift cluster, this component automatically adjusts their container resource requests and limits based on configured ratios. This provides cluster admins with controls to oversubscribe nodes and maximize resource utilization. By setting ratios appropriately, admins can run more pods on nodes than what the pod resource requests suggest. If ratios are set too low, workload quality of service may suffer.

Instead of manually setting resource limits for every pod, cluster administrators configure override ratios once, and this component applies them automatically.

## What It Does

The ClusterResourceOverride Admission component modifies the ratio between requests and limits that are set on containers. When used together with namespace LimitRanges that specify limits and defaults, you can achieve the desired level of resource overcommit for your cluster.

The component supports four override ratios:

- **memoryRequestToLimitPercent**: Sets memory request as a percentage of memory limit (e.g., 50% means a 2Gi limit gets a 1Gi request)
- **cpuRequestToLimitPercent**: Sets CPU request as a percentage of CPU limit (e.g., 25% means a 1000m limit gets a 250m request)
- **limitCPUToMemoryPercent**: Derives CPU limit from memory limit (e.g., 200% means 1Gi memory gets 2 CPU cores)
- **cpuRequestToRequestPercent**: Scales down CPU request from existing request value (e.g., 75% means a 1000m request becomes 750m)

## How It Works

### Opt-In Model

First namespaces must explicitly opt-in by setting the label:
```
clusterresourceoverrides.admission.autoscaling.openshift.io/enabled: "true"
```

Without this label, pods in the namespace are **not** processed by the webhook.

### Two-Tier Configuration System

The component supports two levels of configuration, allowing flexibility from cluster-wide defaults to namespace-specific customization:

#### Tier 1: ClusterResourceOverride (Cluster-Wide Default)

- **Scope**: Applies to all opted-in namespaces across the cluster
- **Location**: Configuration file at `/etc/clusterresourceoverride/config/override.yaml`
- **Managed by**: When deployed on OpenShift, this file is automatically created and updated by the Cluster Resource Override Operator from the ClusterResourceOverride object.
- **Use case**: Set sensible defaults for the entire cluster

#### Tier 2: ResourceOverride CRs (Namespace-Specific) (Optional)

- **Scope**: Applies only within a specific namespace

#### Precedence Rules

When a pod is created:

1. The component checks if any **ResourceOverride CRs** in that namespace match the pod's labels
2. **If a ResourceOverride CR matches**, it takes precedence and its ratios are used
3. **If no ResourceOverride CRs match** (or none exist), the cluster-wide **ClusterResourceOverride** ratios are used as fallback
4. If multiple ResourceOverride CRs match, they are resolved alphabetically by name (first one wins)

### Exempted Namespaces

OpenShift default namespaces are always exempt from overrides (hardcoded):
- `openshift`, `openshift-*`
- `kubernetes`, `kubernetes-*`
- `kube`, `kube-*`

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

## Links

- **OpenShift Documentation**: [Cluster Resource Override Operator](https://docs.openshift.com/container-platform/latest/nodes/clusters/nodes-cluster-overcommit.html#nodes-cluster-resource-override_nodes-cluster-overcommit)
- **Operator Repository**: [cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator)
- **CI Configuration**: [openshift/release/.../cluster-resource-override-admission/](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/cluster-resource-override-admission)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines, PR workflow, and development practices. For AI-specific guidance, see [AGENTS.md](AGENTS.md).

#### ClusterResourceOverride Parameters
The file `artifacts/configuration.yaml` is copied to `/etc/clusterresourceoverride/config/override.yaml` inside the docker image. If you want to change the parameters then edit the file and rebuild the image.
```yaml
apiVersion: v1
kind: ClusterResourceOverrideConfig
spec:
  memoryRequestToLimitPercent: 50
  cpuRequestToLimitPercent: 25
  limitCPUToMemoryPercent: 200
  cpuRequestToRequestPercent: 25
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
