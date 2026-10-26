# crossplane-infra

**Tier 5 of 5: Crossplane v2, Kubernetes-native control-plane IaC with GitOps reconciliation**

GCP infrastructure as Kubernetes resources. An app team creates one namespaced `WebApplication`; Crossplane renders a
VPC, subnet, firewall, VM and bucket from it, and keeps them reconciled. CI proves the whole path on every push and
every Monday, including a kind cluster where a `WebApplication` reconciles to Ready with no cloud account.

Versions (all verified current when this repo was last updated): Crossplane 2.4.3, `crossplane` CLI 2.4.1,
provider-upjet-gcp 3.1.0 (`provider-gcp-compute`, `provider-gcp-storage`), function-patch-and-transform 0.11.2,
function-environment-configs 0.9.2, function-go-templating 0.13.2, function-auto-ready 0.7.3, provider-nop 0.6.0,
function-sdk-go 0.7.1, Go 1.27, Kubernetes (kind) 1.36.

## Position in the IaC spectrum

| Tier | Tool | Trade-off vs this tier |
|------|------|----------------------|
| 1 | Terraform + Terragrunt | CLI-driven, state file in GCS. No reconciliation loop: drift is not auto-corrected. |
| 2 | OpenTofu | Same model as Terraform, open-source license. |
| 3 | Pulumi | Real language, Automation API. Still CLI/process-driven, not a controller. |
| 4 | AWS CDK | Synthesizes to CloudFormation stacks. AWS-only. No continuous reconciliation. |
| **5 (this)** | Crossplane | Kubernetes controller model: desired state in etcd, reconciliation loop auto-corrects drift. GitOps-native (Argo CD/Flux applies manifests). Trade-off: needs a running cluster; highest operational complexity. |

## Level map

| Level | Folder | What you learn | Checked in CI by |
|-------|--------|----------------|------------------|
| L1 | `install/`, `providers/` | Helm install of Crossplane v2, family providers, `DeploymentRuntimeConfig`, `ManagedResourceActivationPolicy`, `ClusterProviderConfig` | `validate`, `kind` (Helm values are used for the install) |
| L2 | `managed-resources/` | One YAML = one cloud resource (namespaced managed resources, `*.gcp.m.upbound.io`) | `validate` against the provider CRDs |
| L3 | `compositions/` | XRD v2 (`apiextensions.crossplane.io/v2`, namespaced XR, no Claim), Pipeline-mode Composition, patch-and-transform | `validate`, `render` |
| L4 | `environment-configs/`, `functions/` | `EnvironmentConfig` (`v1beta1`) for per-environment values, function-environment-configs, XR overrides | `render` (dev vs prod), `kind` |
| L5 | `custom-function/` | Your own Composition function in Go on function-sdk-go: input type, unit tests, container, xpkg | `function` (tidy, vet, golangci-lint, `go test -race`), `render`, `kind` |
| MAX | `packages/`, `local-lab/` | Configuration package (`crossplane xpkg build`); a second implementation of the same API on provider-nop, picked by label | `package`, `kind` |

### Crossplane v2 in one paragraph

Claims are gone: the XRD has `scope: Namespaced` and the XR (`WebApplication`) lives in the app team's namespace.
Composed resources of a namespaced XR are namespaced too, so the Composition uses the `*.gcp.m.upbound.io` API groups
of provider-upjet-gcp v3 and a `ClusterProviderConfig`. Pipeline mode is the only mode; native patch-and-transform
and native environments are gone, replaced by `function-patch-and-transform` and `function-environment-configs`.
Managed-resource CRDs stay inactive until a `ManagedResourceActivationPolicy` activates them. The XR's Crossplane
fields (`compositionRef`, `compositionSelector`) sit under `spec.crossplane`.

## Structure

```
install/                 Helm values for Crossplane v2 (no --enable-* args: those flags no longer exist)
providers/               Providers (GCP family) + DeploymentRuntimeConfig + MRAP + ClusterProviderConfig
managed-resources/       Low-level MRs: Network, Subnetwork, Instance, Bucket
compositions/            XRD v2, GCP Composition (pipeline), example XR
environment-configs/     EnvironmentConfig per environment (dev/staging/prod)
functions/               Function manifests: the three public ones + go-templating, auto-ready, and the custom one
custom-function/         Go source of function-resource-labeler (function-sdk-go), Dockerfile, xpkg metadata, tests
packages/                Configuration package metadata + install manifest
local-lab/               provider-nop + a Composition backed by it (runs anywhere, no cloud)
tests/                   Sample XRs, expected render output, manifests that must be rejected, render function set
scripts/                 What CI runs; every script also runs on your machine
```

## Run it

Prerequisites: a Kubernetes cluster (`kind create cluster` is enough), `helm`, `kubectl`, and the
[`crossplane` CLI](https://docs.crossplane.io/latest/cli/).

### With no cloud account (this is what CI does)

```bash
scripts/kind-e2e.sh      # needs a running kind cluster named "kind", helm, docker, the crossplane CLI
```

It installs Crossplane from the Helm chart, builds the custom function and pushes it to a throwaway local registry,
installs the functions and provider-nop, applies the XRD, EnvironmentConfigs and Compositions, creates
`tests/xrs/xr-dev-nop.yaml`, waits for `Ready`, checks the three composed resources (labelled by the Go function,
values from the EnvironmentConfig and the XR), then deletes the XR and checks nothing is left.

### Against GCP

```bash
helm repo add crossplane-stable https://charts.crossplane.io/stable && helm repo update
helm install crossplane crossplane-stable/crossplane --version 2.4.3 \
  --namespace crossplane-system --create-namespace -f install/crossplane-helm-values.yaml

kubectl apply -f providers/provider-gcp.yaml
kubectl apply -f providers/managed-resource-activation-policy.yaml

kubectl create secret generic gcp-credentials --namespace crossplane-system \
  --from-file=credentials.json=/path/to/sa.json                 # never commit this file
# edit projectID in providers/provider-config-gcp.yaml, then
kubectl apply -f providers/provider-config-gcp.yaml

kubectl apply -f functions/function-patch-and-transform.yaml -f functions/function-environment-configs.yaml
# The custom function has no public registry: build and push it to yours and edit `package`
# (steps are in functions/function-resource-labeler.yaml), then
kubectl apply -f functions/function-resource-labeler.yaml

kubectl apply -f environment-configs/
kubectl apply -f compositions/xrd-web-application.yaml -f compositions/composition-web-app-gcp.yaml

kubectl create namespace team-shop
kubectl apply -f tests/xrs/xr-dev-gcp.yaml      # creates VPC, subnet, firewall, VM, bucket in your project
```

The GCP path has not been reconciled by this repo's CI (no GCP project). It is rendered and validated against the
provider's schemas, which catches wrong fields and API versions, not wrong quotas, IAM or org policies.

### Render and validate locally (no cluster)

```bash
scripts/validate-manifests.sh   # crossplane resource validate: all manifests + must-fail manifests
scripts/render-tests.sh         # crossplane composition render + assertions (needs docker, go, yq)
scripts/build-configuration-package.sh
```

## What the Composition does

`compositions/composition-web-app-gcp.yaml`, three steps:

1. `function-environment-configs` selects the `EnvironmentConfig` whose `environment` label equals
   `spec.parameters.environment` and puts it in the pipeline context.
2. `function-patch-and-transform` renders Network, Subnetwork, Firewall (SSH from Google's IAP range only), Instance
   (Debian 12, **no external IP**, OS Login, narrow API scopes) and Bucket (public access prevented). Each value is
   patched from the environment first, then from the XR if the XR sets it (`tests/xrs/xr-prod-gcp.yaml` overrides
   region, machine type, disk and CIDR; `xr-dev-gcp.yaml` sets nothing).
3. `function-resource-labeler` (the Go function) stamps labels on every composed resource.

`local-lab/composition-web-app-nop.yaml` implements the same API with `function-go-templating`,
`function-resource-labeler` and `function-auto-ready` over provider-nop; an XR picks it with
`spec.crossplane.compositionSelector.matchLabels: {provider: nop}`.

## What CI checks (`.github/workflows/ci.yml`: push, pull request, Mondays 03:23 UTC, manual)

| Job | Runs |
|-----|------|
| `lint` | actionlint, yamllint (strict), shellcheck, every action pinned to an existing commit SHA |
| `function` | `go mod tidy` is a no-op, `go vet`, golangci-lint, `go test -race`, `go build` |
| `validate` | `crossplane resource validate` of every manifest against Crossplane core + XRD + provider CRDs + function input schemas, with missing schemas treated as errors; then the files in `tests/invalid/` (pre-v3 field shapes, a bad enum, a CEL rule, a missing required field) must each be **rejected** for the stated reason |
| `render` | `crossplane composition render` of both Compositions with three sample XRs (functions run as containers, the Go function from source); `yq` assertions in `tests/expected/`; rendered output validated against the schemas |
| `package` | `crossplane xpkg build` of the Configuration package |
| `kind` | the end-to-end run described above on a kind cluster |

Dependabot (`.github/dependabot.yml`) bumps actions, the Go module and the function's Docker base image weekly.
Provider, function and Crossplane versions in the manifests are bumped by hand; CI reads the provider versions from
the manifests, so a bump there is validated against that version's CRDs automatically.

## Honest limits

- No GCP reconcile in CI; see above.
- The kind job uses provider-nop, so it proves the Crossplane machinery (XRD, Compositions, functions, readiness,
  garbage collection), not GCP behaviour.
- `custom-function/input/v1beta1/zz_generated.deepcopy.go` and `package/input/*.yaml` are written to match controller-gen
  output rather than generated in CI; the function's Go tests and the render/kind jobs exercise them.
- The Configuration package does not list the custom function as a dependency (no public registry for it).
- Bucket names are global in GCS; the Composition uses `<appName>-<environment>-bucket`, which can collide.

## Notes

- `ControllerConfig` is gone; provider pod tuning uses `DeploymentRuntimeConfig` (`pkg.crossplane.io/v1beta1`).
- The monolithic `provider-gcp` is not used: the family providers install only the CRDs you activate.
- Provider credentials stay in a Secret referenced by `secretRef`; nothing secret is in git.
