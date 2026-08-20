# AGENTS.md — cluster-resource-override-admission

This file provides AI-specific guidance for working in the cluster-resource-override-admission repository. For contribution guidelines, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Project Overview

This repo contains the **Cluster Resource Override** admission webhook operand for OpenShift — a component that automatically adjusts container resource requests and limits to enforce fair resource distribution and prevent resource exhaustion.

The admission webhook is deployed and managed by the [cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator).

### What It Does

The webhook intercepts pod creation requests and modifies container resource specifications according to configured ratios:
- **memoryRequestToLimitPercent**: Sets memory request as a percentage of memory limit (e.g., 50% means a 2Gi limit gets a 1Gi request)
- **cpuRequestToLimitPercent**: Sets CPU request as a percentage of CPU limit (e.g., 25% means a 1000m limit gets a 250m request)
- **limitCPUToMemoryPercent**: Derives CPU limit from memory limit (e.g., 200% means 1Gi memory gets 2 CPU cores)

### Opt-In Model

Namespaces must explicitly opt-in by setting label:
```
clusterresourceoverrides.admission.autoscaling.openshift.io/enabled: "true"
```

Hardcoded namespaces exemptions: `openshift`, `openshift-*`, `kubernetes`, `kubernetes-*`, `kube`, `kube-*`

### Two-Tier Configuration

1. **ClusterResourceOverride** (cluster-wide file) — Default ratios loaded from `/etc/clusterresourceoverride/config/override.yaml`
2. **ResourceOverride CRs** (namespace-scoped, optional) — Per-namespace or per-pod overrides with label selectors

When multiple ResourceOverrides match a pod, conflicts are resolved lexicographically (alphabetically by name). Namespace-specific ResourceOverride CRs take precedence over the cluster-wide ClusterResourceOverride configuration.

## Repository Structure

```
cmd/
  cluster-resource-override-admission/
    main.go                  # Entry point (9 lines - delegates to generic-admission-server)
    clusterresourceoverride.go  # Admission hook implementation
pkg/
  clusterresourceoverride/
    admission.go             # Core admission controller setup and IsApplicable/IsExempt/Admit logic
    mutator.go               # Pod mutation logic - CRITICAL ORDER DEPENDENCY
    config.go                # Configuration loading and conversion
    resolver.go              # ResourceOverride CR matching and conflict resolution
    resourceoverride_store.go  # Lister for ResourceOverride CRs
    exempt.go                # Hardcoded namespace exemption logic
    limitquerier.go          # LimitRange floor/ceiling lookup
    patcher.go               # JSON patch generation
  response/
    response.go              # AdmissionResponse helpers
  api/
    api.go                   # API group constants
  version/
    version.go               # Version info
artifacts/
  configuration.yaml         # Default ClusterResourceOverride config (copied to image)
  manifests/                 # Deployment manifests (100-600 numbered files)
  example/                   # Example request/deployment manifests
images/
  ci/Dockerfile              # CI build image
  dev/Dockerfile.dev         # Dev build image
Dockerfile.rhel7             # Production RHEL 7 image
```

## Architecture: What Is Not Obvious

### Critical Mutation Order

The `Override()` method in `pkg/clusterresourceoverride/mutator.go` MUST execute steps in this exact order:

1. **AnnotateOriginalRequest** — Store original CPU request in pod annotation (for idempotency)
2. **OverrideMemory** — Set memory request from memory limit
3. **OverrideCPULimit** — Set CPU limit from memory limit (if configured)
4. **OverrideCPUWithLimit** — Set CPU request from CPU limit
5. **OverrideCPUWithRequest** — Set CPU request from original CPU request (uses annotation)

Changing this order breaks override semantics and idempotency guarantees.

### Idempotency via Annotations

To handle reinvocations (e.g., if the webhook is called multiple times for the same pod), CPU request overrides use per-container annotations:
```
clusterresourceoverrides.admission.autoscaling.openshift.io/original-cpu-request-<container-name>
```

This ensures the override is based on the *original* user request, not a previously overridden value.

### LimitRange Integration

After override calculation, the webhook queries the namespace's LimitRange objects and applies:
- **Floor (minimum)**: If overridden value < minimum, set to minimum
- **Ceiling (maximum)**: If overridden value > maximum, set to maximum

This happens in `limitquerier.go` via informer-backed listers (no API calls during admission).

### ResourceOverride Conflict Resolution

When multiple ResourceOverride CRs match a pod:
1. All matching CRs are collected
2. Sorted lexicographically by name
3. First one (alphabetically) wins
4. Warning event posted to the losing ResourceOverride objects

This is handled in `resolver.go:selectWinner()`.

### Informer-Backed Admission

The webhook uses client-go informers for:
- Namespaces (to check labels and exemptions)
- LimitRanges (to query floor/ceiling)
- ResourceOverride CRs (optional, only if CRD is installed)

All data is served from in-memory caches — no API calls during admission path for performance.

## Common Pitfalls

1. **Do not modify the mutation order(in pkg/clusterresourceoverride/mutator.go).** The sequence in `mutator.go:Override()` is critical. Changing the order breaks override semantics and can cause unexpected behavior (e.g., CPU request being set before the annotation is written).

2. **Do not break idempotency.** Any mutation logic must handle reinvocations gracefully. Use annotations to store original values if needed. The webhook may be called multiple times for the same pod due to API server retry logic.

3. **Watch for LimitRange precedence.** LimitRange acts as a floor/ceiling AFTER override calculation. A perfectly valid override calculation may be clamped by LimitRange minimums/maximums.

4. **ResourceOverride CRs are optional.** The webhook works without the ResourceOverride CRD installed. Code in `resourceoverride_store.go` must handle nil listers gracefully.

5. **Do not modify vendor/ directly.** Run `go mod tidy && go mod vendor` and commit vendor changes in a separate commit from logic changes.

## Human-in-the-Loop Triggers

Stop and consult a human before:

- **Changing mutation order** (`mutator.go:Override()`) — This affects core override semantics
- **Modifying namespace exemptions** (`exempt.go`) — Security and product impact
- **Changing the opt-in label name** (`admission.go:EnabledLabelName`) — Breaking change for existing clusters
- **Altering idempotency logic** (annotation handling in `mutator.go`) — Can cause double-overrides or incorrect calculations
- **Modifying config schema** (`config.go:ClusterResourceOverrideSpec`) — May require operator changes
- **Changing ResourceOverride conflict resolution** (`resolver.go`) — Affects multi-CR behavior
- **Adding new override ratios** — Product feature requiring operator coordination
- **Modifying deployment manifests** (`artifacts/manifests/`) — Affects production deployments

## Paired Changes

These files must be updated together:

| If you change... | Also update... |
|-----------------|----------------|
| `config.go:ClusterResourceOverrideSpec` | Update `artifacts/configuration.yaml` to match new schema |
| Mutation logic in `mutator.go` | Add corresponding tests in `mutator_test.go` |
| Namespace exemption in `exempt.go` | Add tests in `exempt_test.go` |
| ResourceOverride matching in `resolver.go` | Add tests in `resolver_test.go` |
| `go.mod` dependencies | Run `go mod vendor` and commit vendor changes separately |
| Deployment manifests in `artifacts/manifests/` | Run `make manifests` to copy to `_output/manifests/` |

## Testing Guidance

### Unit Tests

Run with `make test` or `make test-unit`. Key test files:
- `mutator_test.go` — Override calculation tests with various configurations
- `admission_test.go` — End-to-end admission flow tests
- `resolver_test.go` — ResourceOverride matching and conflict resolution
- `exempt_test.go` — Namespace exemption logic
- `limitquerier_test.go` — LimitRange floor/ceiling logic

### Test Data

The `pkg/clusterresourceoverride/testdata/` directory contains test fixtures. When adding new test cases, follow the existing pattern.

### Integration Testing

Integration tests are expected for:
- Changes to admission flow (`admission.go`)
- New override ratio types
- ResourceOverride matching changes
- LimitRange integration changes

## Configuration

### Default Configuration

The file `artifacts/configuration.yaml` is baked into the container image at `/etc/clusterresourceoverride/config/override.yaml`. Default ratios:

```yaml
memoryRequestToLimitPercent: 50    # Memory request = 50% of memory limit
cpuRequestToLimitPercent: 25       # CPU request = 25% of CPU limit
limitCPUToMemoryPercent: 200       # 1 GiB memory → 2 CPU cores
```

### Configuration Loading

The webhook loads config at startup from the path specified in `CONFIGURATION_PATH` env var. No hot-reloading — config changes require pod restart.

## Build and Deploy

| Task | Command |
|------|---------|
| Build binary | `make build` |
| Build container image | `make local-image IMAGE_TAG_BASE=<registry>/<repo> IMAGE_VERSION=<tag>` |
| Push container image | `make local-push IMAGE_TAG_BASE=<registry>/<repo> IMAGE_VERSION=<tag>` |
| Generate manifests | `make manifests` (copies `artifacts/manifests/*` to `_output/manifests/`) |
| Run unit tests | `make test` or `make test-unit` |
| Run verification | `make verify` |

The image builder defaults to `podman` but can be overridden with `IMAGE_BUILDER=docker` or `IMAGE_BUILDER=buildah`.

## Further Reading

- [CONTRIBUTING.md](CONTRIBUTING.md) — Contribution workflow, PR commands, test expectations
- [README.md](README.md) — Product overview and developer quickstart
- [OpenShift Documentation](https://docs.openshift.com/container-platform/latest/nodes/clusters/nodes-cluster-overcommit.html#nodes-cluster-resource-override_nodes-cluster-overcommit) — End-user documentation
- [cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator) — Operator repository
- [generic-admission-server](https://github.com/openshift/generic-admission-server) — Framework used by this webhook
