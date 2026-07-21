---
name: Release Chores
description: "Perform release chores for a new OpenShift version: bump OCP release branch, Go version, K8s dependencies, builder/base images, UBI minimal image, and run vendor update."
---

# CRO Release Chores

Perform all mechanical file updates required for a new OpenShift release. This follows the established pattern from PRs #97 (4.21), #100 (4.22), and #108 (5.0).

## Step 1 — Gather Inputs

Ask the user for the following values:

1. **New OCP version** (e.g. `5.1`)
2. **New Go version** — major.minor only (e.g. `1.27`)
3. **New Go patch version** — for `go.mod` (e.g. `1.27.3`). This must match the Go version available in the `rhel-9-release-golang-{GO}-openshift-{OCP}` builder image.
4. **New Kubernetes dependency version** (e.g. `v0.37.0`). User can check available versions via: `go list -mod=readonly -m -versions k8s.io/api | sed 's/ /\n/g'`
5. **OS** — macOS or Linux (determines `sed -i` syntax)
6. **Update generic-admission-server?** — whether to also run `go get -u github.com/openshift/generic-admission-server@master` (optional, not always needed)

## Step 2 — Read Current Values

Read the following files to extract the current values that will be replaced:

- `hack/update-vendor.sh` → current `release_branch` (e.g. `release-5.0`) and `kube_release` (e.g. `v0.36.0`)
- `.ci-operator.yaml` → current builder image tag (e.g. `rhel-9-release-golang-1.26-openshift-5.0`)
- `images/ci/Dockerfile` → current builder image tag and base image (e.g. `ocp/5.0:base-rhel9`)
- `images/dev/Dockerfile.dev` → current UBI minimal version (e.g. `9.8`)
- `go.mod` → current Go version (e.g. `1.26.3`)

## Step 3 — Auto-Detect Latest UBI Minimal Version

Run the following command to find the latest available UBI 9 minimal version:

```bash
skopeo list-tags docker://registry.access.redhat.com/ubi9/ubi-minimal 2>&1 \
  | jq -r '.Tags[]' | grep -E '^9\.[0-9]+$' | sort -V | tail -1
```

Compare the result with the current version in `images/dev/Dockerfile.dev`. Only update if a newer version is available. Inform the user of the current and latest versions.

## Step 4 — Generate and Run sed Commands

Based on the user's OS choice, use the correct `sed -i` syntax:
- **macOS**: `sed -i ''`
- **Linux**: `sed -i`

Generate and run the following commands, substituting the actual old and new values:

```bash
# Update release branch in vendor script (e.g. release-5.0 → release-5.1)
sed -i{SED} 's/release-{OLD_OCP}/release-{NEW_OCP}/' hack/update-vendor.sh

# Update K8s version in vendor script (e.g. v0.36.0 → v0.37.0)
sed -i{SED} 's/{OLD_KUBE}/{NEW_KUBE}/' hack/update-vendor.sh

# Update builder image in CI Dockerfile (e.g. golang-1.26-openshift-5.0 → golang-1.27-openshift-5.1)
sed -i{SED} 's/golang-{OLD_GO}-openshift-{OLD_OCP}/golang-{NEW_GO}-openshift-{NEW_OCP}/' images/ci/Dockerfile

# Update base image in CI Dockerfile (e.g. ocp/5.0 → ocp/5.1)
sed -i{SED} 's|ocp/{OLD_OCP}:base-rhel9|ocp/{NEW_OCP}:base-rhel9|' images/ci/Dockerfile

# Update builder image in CI operator config
sed -i{SED} 's/golang-{OLD_GO}-openshift-{OLD_OCP}/golang-{NEW_GO}-openshift-{NEW_OCP}/' .ci-operator.yaml

# Update UBI minimal version in dev Dockerfile (only if newer available)
sed -i{SED} 's/ubi9-minimal:{OLD_UBI}/ubi9-minimal:{NEW_UBI}/' images/dev/Dockerfile.dev

# Update Go version in go.mod
go mod edit -go={NEW_GO_PATCH}
```

After running the sed commands, display the changes to the user for confirmation before proceeding.

## Step 5 — Run Vendor Update

Execute the following commands in sequence:

```bash
hack/update-vendor.sh
```

If the user opted to update generic-admission-server:
```bash
go get -u github.com/openshift/generic-admission-server@master
```

Then finalize dependencies:
```bash
go mod tidy
go mod vendor
```

## Step 6 — Verify Build

Run `make build` to confirm everything compiles correctly.

## Step 7 — Summarize

Display a summary table of all version transitions:

| Component | Old | New |
|---|---|---|
| OCP version | {OLD_OCP} | {NEW_OCP} |
| Go version | {OLD_GO_PATCH} | {NEW_GO_PATCH} |
| K8s deps | {OLD_KUBE} | {NEW_KUBE} |
| UBI minimal | {OLD_UBI} | {NEW_UBI} |
| Builder image | golang-{OLD_GO}-openshift-{OLD_OCP} | golang-{NEW_GO}-openshift-{NEW_OCP} |

List all files that were modified.
