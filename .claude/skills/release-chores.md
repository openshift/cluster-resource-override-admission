---
name: Release Chores
description: "Automate CRO release version bumps and dependency updates"
argument-hint: "[new_ocp_version] [new_go_version] [new_k8s_version] [dry_run]"
allowed-tools: Read, Edit, Glob, Grep, Bash(curl:*), Bash(skopeo list-tags:*), Bash(skopeo inspect:*), Bash(hack/update-vendor.sh:*), Bash(go get:*), Bash(go list:*), Bash(go mod tidy:*), Bash(go mod vendor:*), Bash(make build:*), Bash(git:*), Bash(sed:*), Bash(jq:*)
---

# CRO Release Chores

Automate version bumps and release chores for cluster-resource-override-admission.

## Parameters

All parameters are optional. If not provided, the skill will ask the user or auto-detect values.

Arguments are positional (use empty strings `""` to skip):

1. `$1` - **new_ocp_version** (optional): New OpenShift version (e.g., `5.1`). If not provided, ask the user.
2. `$2` - **new_go_version** (optional): New Go version (e.g., `1.27` or `1.27.3`) — auto-detected from ocp-build-data `streams.yml` if not provided.
3. `$3` - **new_k8s_version** (optional): New Kubernetes dependency version (e.g., `v0.37.0`) — auto-detected if not provided.
4. `$4` - **dry_run** (optional): Set to `true` to preview changes without applying (default: `false`).

## Important Notes

- **Go version**: The Go version in `go.mod` MUST match the exact version (including patch) used in CI, determined by the ocp-build-data `streams.yml` file
- **CI Operator updates**: `.ci-operator.yaml` is updated by automatic PRs from the ART team — do NOT update it manually
- **Go version source**: Auto-fetched from `https://raw.githubusercontent.com/openshift-eng/ocp-build-data/openshift-{VERSION}/streams.yml`

## Your Task

### Step 1: Pre-flight Checks

1. Verify environment:
   - Check that `hack/update-vendor.sh` exists (confirms correct repository)
   - Check for uncommitted changes with `git status` and warn user if any exist
   - If uncommitted changes exist, **STOP** and ask user to commit or stash first, or explicitly approve continuing

2. Parse arguments. For any value not provided:
   - **OCP version**: Ask the user: "What is the new OpenShift version? (e.g., `5.1`)"
   - **Go version**: Will be auto-detected in Step 3.5 (don't ask yet)
   - **K8s version**: Will be auto-detected in Step 4 (don't ask yet)
   - **dry_run**: Default to `false`

3. Validate the OCP version matches format `X.Y` (e.g., `5.1`)

### Step 2: Detect Current Versions

Use `grep` to extract current versions from files:

1. **Current OpenShift version**: Extract from `hack/update-vendor.sh` using the `release_branch` variable (e.g., `release-5.0` → `5.0`)
2. **Current Go version (CI)**: Extract from `.ci-operator.yaml` using pattern `golang-\K[0-9]+\.[0-9]+`
3. **Current Go version (Dockerfile)**: Extract from `images/ci/Dockerfile` using pattern `golang-\K[0-9]+\.[0-9]+`
4. **Current Kubernetes version**: Extract from `hack/update-vendor.sh` using the `kube_release` variable
5. **Current UBI version**: Extract from `images/dev/Dockerfile.dev` using pattern `ubi9-minimal:\K[0-9.]+`
6. **Current Go version (go.mod)**: Extract from `go.mod`

Display all detected versions to the user.

### Step 3: Check .ci-operator.yaml Status

1. Extract OpenShift version from `.ci-operator.yaml` using pattern `openshift-\K[0-9]+\.[0-9]+`
2. If the version doesn't match the new OCP version:
   - Check if the builder image for the new version exists:
     ```bash
     skopeo inspect docker://registry.ci.openshift.org/openshift/release:rhel-9-release-golang-{GO_MINOR}-openshift-{NEW_OCP} --no-tags 2>&1
     ```
   - If the image **exists**: Suggest the user update `.ci-operator.yaml` to use the correct OCP version and Go version matching the available image, then continue
   - If the image **does not exist**: **WARN** user that the builder image for the new version is not available yet — suggest waiting for the ART team to publish it, or provide a Go version override manually
3. Extract the Go minor version from `.ci-operator.yaml` for reference

### Step 3.5: Determine Full Go Version from ocp-build-data

If Go version was not provided as argument, auto-detect the exact patch version:

1. **Fetch streams.yml**:
   ```bash
   curl -sSL https://raw.githubusercontent.com/openshift-eng/ocp-build-data/openshift-$NEW_VERSION/streams.yml
   ```

2. **Parse the Go version**: Look for `rhel-9-golang:` entry and extract version from `golang-builder-v\K[0-9]+\.[0-9]+\.[0-9]+` in the `image:` field

3. **Handle failures**:
   - If curl fails and no override provided, ask the user for the Go version as fallback
   - If argument was provided with patch version (X.Y.Z), use it directly
   - If argument was minor only (X.Y), still fetch streams.yml for the patch

4. **Store results**:
   - `$GO_MINOR_VERSION` — for Dockerfile updates (e.g., `1.27`)
   - `$GO_FULL_VERSION` — for `go.mod` (e.g., `1.27.3`)

### Step 4: Auto-detect Remaining Versions

1. **Kubernetes version** (if not provided as argument):
   - Get available versions: `go list -mod=readonly -m -versions k8s.io/api | sed 's/ /\n/g'`
   - Find the latest patch for the next minor version after the current
   - Fall back to asking the user if command fails

2. **UBI version**:
   ```bash
   skopeo list-tags docker://registry.access.redhat.com/ubi9/ubi-minimal 2>&1 \
     | jq -r '.Tags[]' | grep -E '^9\.[0-9]+$' | sort -V | tail -1
   ```
   - If skopeo/jq fails, keep current UBI version and warn user

### Step 4.5: Verify Container Images Exist

Verify new container images exist using `skopeo inspect`. Track verification status:
- ✓ = Image verified and exists
- \* = Could not verify (auth/network/skopeo not installed)
- ✗ = Image does NOT exist

**Images to verify**:

1. **UBI Minimal**: `registry.access.redhat.com/ubi9/ubi-minimal:$NEW_UBI_VERSION`
2. **Golang Builder**: `registry.ci.openshift.org/ocp/builder:rhel-9-golang-$GO_MINOR_VERSION-openshift-$NEW_VERSION`

Do not prompt or stop on verification issues — continue and document results.

### Step 5: Display Change Summary

```
OpenShift:        [CURRENT] → [NEW]
Go (Dockerfiles): [CURRENT] → [GO_MINOR_VERSION] (from .ci-operator.yaml)
Go (go.mod):      [CURRENT] → [GO_FULL_VERSION] (from ocp-build-data)
Kubernetes:       [CURRENT] → [NEW]
UBI Minimal:      [CURRENT] → [NEW] (or "unchanged")
CI base image:    ocp/[CURRENT]:base-rhel9 → ocp/[NEW]:base-rhel9
```

Include verification status markers (✓/\*/✗) next to each image.

Files to be updated:
- `hack/update-vendor.sh`
- `images/ci/Dockerfile`
- `images/dev/Dockerfile.dev`
- `go.mod`, `go.sum`, `vendor/`

**If `DRY_RUN` is `true`**: Display summary and exit without making changes.

**Otherwise**: Ask user for confirmation before proceeding.

### Step 6: Apply Changes

Execute file updates using `sed` via Bash (auto-detect macOS vs Linux for `sed -i` syntax — do NOT ask the user):

1. **Update release branch** in `hack/update-vendor.sh`:
   ```bash
   sed -i 's/release-{OLD_OCP}/release-{NEW_OCP}/' hack/update-vendor.sh
   ```

2. **Update K8s version** in `hack/update-vendor.sh`:
   ```bash
   sed -i 's/{OLD_KUBE}/{NEW_KUBE}/' hack/update-vendor.sh
   ```

3. **Update builder image** in `images/ci/Dockerfile`:
   ```bash
   sed -i 's/golang-{OLD_GO}-openshift-{OLD_OCP}/golang-{NEW_GO}-openshift-{NEW_OCP}/' images/ci/Dockerfile
   ```

4. **Update base image** in `images/ci/Dockerfile`:
   ```bash
   sed -i 's|ocp/{OLD_OCP}:base-rhel9|ocp/{NEW_OCP}:base-rhel9|' images/ci/Dockerfile
   ```

5. **Update UBI version** in `images/dev/Dockerfile.dev` (only if newer):
   ```bash
   sed -i 's/ubi9-minimal:{OLD_UBI}/ubi9-minimal:{NEW_UBI}/' images/dev/Dockerfile.dev
   ```

6. **Update Go version** in `go.mod`:
   ```bash
   go mod edit -go={GO_FULL_VERSION}
   ```

**Do NOT update `.ci-operator.yaml`** — this is handled by the ART team.

### Step 7: Update Go Dependencies

Run the following commands in sequence:

1. **Update build-machinery-go**:
   ```bash
   go get -u github.com/openshift/build-machinery-go@master
   ```
   - If this fails, warn but continue

2. **Run update-vendor.sh**:
   ```bash
   hack/update-vendor.sh
   ```
   - **If this fails**: Inform user to restore with `git checkout -- .` and investigate

3. **Update generic-admission-server** (ask user first — optional, not always needed):
   ```bash
   go get -u github.com/openshift/generic-admission-server@master
   ```

4. **Tidy and vendor**:
   ```bash
   go mod tidy
   go mod vendor
   ```
   - **If either fails**: Inform user to restore with `git checkout -- .` and check dependency availability

### Step 8: Verify Build

```bash
make build
```

If this fails, inform user and suggest checking dependency versions.

### Step 9: Create Git Commit

1. Display changes:
   ```bash
   git status --short
   git diff --name-only
   ```

2. Create feature branch:
   ```bash
   git checkout -b release-chores-$NEW_VERSION
   ```

3. Stage and commit:
   ```bash
   git add -A
   git commit -m "Updates for $NEW_VERSION"
   ```

4. Show the commit:
   ```bash
   git show HEAD --stat
   ```

5. Provide next steps:
   - Review: `git show HEAD`
   - Push and create PR: `git push origin release-chores-$NEW_VERSION`

## Examples

```bash
# No arguments — skill asks for OCP version, auto-detects everything else
/release-chores

# Just OCP version — auto-detect Go and K8s versions
/release-chores 5.1

# Dry run — preview changes without applying
/release-chores 5.1 "" "" true

# Specify custom K8s version
/release-chores 5.1 "" v0.37.0

# Specify all parameters explicitly
/release-chores 5.1 1.27.3 v0.37.0 false
```

## Troubleshooting

### Error: ".ci-operator.yaml is still on old version"
- The ART team hasn't created their PR yet
- Wait for ART PR, merge it, then run this command

### Error: "go mod tidy failed"
- Check if K8s version exists: `go list -m -versions k8s.io/api`
- Try with different version parameters
- Reset changes: `git checkout -- .`

### Image verification issues
- **skopeo not found**: Install with `sudo dnf install skopeo` (Fedora/RHEL) or `sudo apt install skopeo` (Ubuntu)
- **unauthorized**: Login with `podman login <registry>`
- **not found**: Image may not be published yet — wait and retry
