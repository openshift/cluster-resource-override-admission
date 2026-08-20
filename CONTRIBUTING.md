# Contributing to Cluster Resource Override Admission

This document covers contribution guidelines for the OpenShift Cluster Resource Override admission webhook operand.

## Related Resources

Operator repo [openshift/cluster-resource-override-admission-operator](https://github.com/openshift/cluster-resource-override-admission-operator)
CI configuration [openshift/release/.../cluster-resource-override-admission/](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/cluster-resource-override-admission)
AI guidance [AGENTS.md](AGENTS.md)
OpenShift docs [Cluster Resource Override Operator](https://docs.openshift.com/container-platform/latest/nodes/clusters/nodes-cluster-overcommit.html#nodes-cluster-resource-override_nodes-cluster-overcommit)

## Review and Approval Policy

Every change in every pull request must be understood and approved by two humans. This can be the PR author and a reviewer, or — if the author used an AI tool and does not fully understand the contents of the PR — two human reviewers.

**Exception:** PRs authored by deterministic automation tools that are part of our CI and related systems (whose code has been reviewed by the OpenShift engineering org) can be merged with a single human review.

Every change should be closely scrutinized for bugs. Our software is complex with many interdependencies. Review changes from multiple angles:

- **Product architecture**: Does this fit the intended design of the admission webhook and OpenShift?
- **Security**: Are there new attack surfaces, credential handling issues, or privilege escalations? Admission webhooks have cluster-wide authority and can modify any pod.
- **Mutation order**: The override sequence in `pkg/clusterresourceoverride/mutator.go` matters. Changes must preserve the order: AnnotateOriginalRequest → OverrideMemory → OverrideCPULimit → OverrideCPUWithLimit → OverrideCPUWithRequest.
- **Idempotency**: CPU request overrides use per-container annotations to ensure idempotent reinvocations. Any mutation logic must be idempotent.
- **Thread safety**: Are shared resources properly synchronized?
- **Regressions**: Could this break existing override behavior or exempt namespace handling?
- **Effects on other components**: How does this impact the operator, pod creation latency, or cluster stability?

## PR Title Convention

PR titles should be prefixed with a Jira ticket reference. For example:

```
AUTOSCALE-123: Fix the whatsit in the thingamajig
OCPBUGS-456: Correct nil pointer in admission handler
NO-JIRA: Update Go dependencies
```

## PR Workflow

This repo uses [OpenShift CI (Prow)](https://docs.ci.openshift.org/) for continuous integration. PRs are automatically merged once all required tests pass and the correct labels are present.

### Required labels for merge

- `lgtm` — Added by a reviewer via the `/lgtm` command. Any developer from the OpenShift org can add this after reviewing the PR.
- `approved` — Added by an approver listed in the [OWNERS](OWNERS) file via the `/approve` command.
- `verified` — Added via the `/verified` command to indicate changes have been verified to work correctly (see Verified Label section below).

### Useful commands

Comment these on the PR:

| Command | Effect |
|---------|--------|
| `/lgtm` | Add the `lgtm` label after reviewing |
| `/lgtm cancel` | Remove the `lgtm` label |
| `/approve` | Add the `approved` label (OWNERS approvers only) |
| `/retest` | Re-run all failed required tests |
| `/retest-required` | Re-run only the failed required tests |
| `/test <test-name>` | Run a specific test |
| `/hold` | Prevent the PR from being merged |
| `/hold cancel` | Remove the hold and allow merging |
| `/verified` | Mark the PR as verified |
| `/cherry-pick release-4.18` | Create a cherry-pick PR to a release branch |

### Preventing premature merges

- Add the `WIP:` prefix to the PR title (e.g., `WIP: AUTOSCALE-123: Work in progress`). Prow adds the `do-not-merge/work-in-progress` label automatically.
- Use `/hold` to temporarily block merging while awaiting additional review or testing.

## Test Expectations

PRs should include tests to verify correctness and prevent future regressions:

- **Unit tests**: Required for new logic, bug fixes, and behavior changes. Run with `make test` or `make test-unit`.
- **Integration tests**: Expected for changes affecting the admission webhook flow, resource override calculations, or exemption logic.

## Verified Label

The `/verified` command marks that changes have been verified to work correctly. Examples:

```
/verified
/verified by unit tests
/verified by e2e-aws-ovn
/verified deferred to QE
```

## Development Quick Reference

| Task | Command |
|------|---------|
| Build binary | `make build` |
| Run unit tests | `make test` or `make test-unit` |
| Build container image | `make local-image IMAGE_TAG_BASE=<registry>/<repo> IMAGE_VERSION=<tag>` |
| Push container image | `make local-push IMAGE_TAG_BASE=<registry>/<repo> IMAGE_VERSION=<tag>` |
| Generate manifests | `make manifests` |
| Verify code | `make verify` |
| Update dependencies | `go mod tidy && go mod vendor` |

## Pre-Submit Checklist

Before requesting review:

1. `make build` — Verify the code compiles
2. `make test` — Run unit tests
3. `make verify` — Run verification checks
4. Review your diff for secrets, credentials, or debug code
5. Address any [CodeRabbit](https://coderabbit.ai/) review feedback — as a courtesy to the human reviewer who follows. Responding with an explanation of why you're not acting on a suggestion is fine; the goal is to resolve straightforward issues so human reviewers can focus on the substantive aspects.

## Code Style

- Run `go fmt ./...` before committing
- Follow Go conventions for error strings: lowercase, no trailing punctuation, wrap with `fmt.Errorf("context: %w", err)`
- Use structured logging with klog: constant messages, key-value pairs in lowerCamelCase
- Import ordering: stdlib, external packages, internal packages (separated by blank lines)

## Dependency Management

This repo uses Go modules with vendoring:

```bash
# Update dependencies
go mod tidy
go mod vendor

# Commit vendor changes separately from logic changes
git add go.mod go.sum
git commit -m "AUTOSCALE-XXX: Update dependencies"
git add vendor/
git commit -m "AUTOSCALE-XXX: Run go mod vendor"
```

## AI Code Review

Our repos use [CodeRabbit](https://coderabbit.ai/) for automated AI code review. CodeRabbit will post review comments on your PR automatically.

As a courtesy to the human reviewer who follows, please address CodeRabbit's feedback before requesting human review. You do not need to accept every suggestion — responding with an explanation of why you are not taking action on a comment is perfectly acceptable. The goal is to resolve straightforward issues so that human reviewers can focus on the substantive aspects of the change.
