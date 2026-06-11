# crossplane-infra

**Tier 5 of 5 — Crossplane: Kubernetes-native control-plane IaC with GitOps reconciliation**

## Purpose

GCP infrastructure managed as Kubernetes custom resources via Crossplane. Covers the full Crossplane feature set: XRDs/Compositions (pipeline mode with function-patch-and-transform + custom Go function), EnvironmentConfigs, Claims, provider family providers, and OCI configuration packages — demonstrating the control-plane model vs imperative `apply`.

## Position in the IaC spectrum

| Tier | Tool | Trade-off vs this tier |
|------|------|----------------------|
| 1 | Terraform + Terragrunt | CLI-driven, state file in GCS. No reconciliation loop — drift is not auto-corrected. |
| 2 | OpenTofu | Same model as Terraform, open-source license. |
| 3 | Pulumi | Real language, Automation API. Still CLI/process-driven, not a controller. |
| 4 | AWS CDK | Synthesizes to CloudFormation stacks. AWS-only. No continuous reconciliation. |
| **5 (this)** | Crossplane | Kubernetes controller model: desired state in etcd, reconciliation loop auto-corrects drift. GitOps-native (ArgoCD/Flux applies manifests). Trade-off: requires running k8s cluster; highest operational complexity. |

## Key concepts (interview talking points)

- **MR → XRD/Composition → Claim**: Managed Resources are low-level (1:1 cloud resource). XRD + Composition = abstract API that operators expose to app teams. Claims are what app teams create — they don't touch cloud details.
- **Pipeline mode Composition**: steps chained via `functionRef` — more composable than patch-and-transform alone. `function-auto-labeler` (custom Go function) + `function-patch-and-transform` + `function-environment-configs` show multi-step pipeline.
- **EnvironmentConfigs**: cluster-scoped key-value store injected into compositions — avoids hardcoding per-env values in the composition itself.
- **Provider family split**: `upbound/provider-gcp` monolithic (v0.41) installs _all_ GCP CRDs (~800). Family providers (`provider-gcp-storage`, `provider-gcp-compute`) install only what you need — lighter cluster, faster reconcile.
- **DeploymentRuntimeConfig** (v1beta1): replaces deprecated `ControllerConfig` (v1alpha1, removed in Crossplane 1.14+) for provider pod resource/config tuning.

## Structure

```
providers/              # Provider + DeploymentRuntimeConfig (replaces ControllerConfig)
managed-resources/      # Low-level MRs: Bucket, Instance, Network/Subnetwork
compositions/           # XRD, Composition (pipeline mode), Claim example
environment-configs/    # Per-env config store (dev/staging/prod)
functions/              # function-auto-labeler (custom Go fn)
custom-function/        # Go source for the auto-labeler function (function-sdk-go)
packages/               # crossplane.yaml (meta v1) + Configuration install manifest
install/                # Helm values for crossplane core install
```

## Running

```bash
# Prerequisites: Kubernetes cluster, Helm, crossplane CLI

# Install Crossplane core
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system --create-namespace \
  -f install/crossplane-helm-values.yaml

# Install providers (family split)
kubectl apply -f providers/provider-gcp.yaml

# Create GCP credentials secret
kubectl create secret generic gcp-credentials \
  --namespace crossplane-system \
  --from-file=credentials.json=/path/to/sa.json

# Apply ProviderConfig
kubectl apply -f providers/provider-config-gcp.yaml

# Apply XRD + Composition
kubectl apply -f compositions/xrd-web-application.yaml
kubectl apply -f compositions/composition-web-app-gcp.yaml

# Apply EnvironmentConfigs
kubectl apply -f environment-configs/

# Create a Claim (triggers composition → MR creation)
kubectl apply -f compositions/claim-web-app-dev.yaml
```

## Notes

- `ControllerConfig` (v1alpha1) removed — this repo uses `DeploymentRuntimeConfig` (v1beta1) per Crossplane 1.14+ requirements.
- Provider credentials stay in a k8s Secret referenced by `secretRef` — no credentials in git.
- Composition `enforced = false` equivalent: `revisionActivationPolicy: Automatic` in the Configuration package.
