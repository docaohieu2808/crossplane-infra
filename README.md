# crossplane-infra

Crossplane K8s-native IaC for GCP: composite resources, compositions, managed resources, a custom Go function, and an OCI package.

## What's Inside

| Path | Description |
|------|-------------|
| `compositions/xrd-web-application.yaml` | XRD: defines `WebApplication` claim API schema |
| `compositions/composition-web-app-gcp.yaml` | Composition: maps `WebApplication` → GCP managed resources |
| `compositions/claim-web-app-dev.yaml` | Example claim: deploy a web app in dev |
| `managed-resources/gcp-compute-instance.yaml` | Direct MR: GCE instance |
| `managed-resources/gcp-network-vpc.yaml` | Direct MR: VPC network |
| `managed-resources/gcp-storage-bucket.yaml` | Direct MR: GCS bucket |
| `providers/provider-gcp.yaml` | GCP provider v0.41.0 install manifest |
| `providers/provider-config-gcp.yaml` | GCP provider auth config |
| `functions/function-auto-labeler.yaml` | Composition function: auto-label all resources |
| `custom-function/` | Go composition function skeleton (`fn.go`, `main.go`) |
| `packages/composition-package.yaml` | OCI package config for distribution |
| `packages/crossplane.yaml` | Package metadata |
| `environment-configs/env-{dev,staging,prod}.yaml` | EnvironmentConfig per environment |
| `install/crossplane-helm-values.yaml` | Helm values for Crossplane on K3s |

## Levels

| Level | Skill | What You Do |
|-------|-------|-------------|
| L1 | Crossplane basics | Install Crossplane; understand managed resources vs claims |
| L2 | Managed resources | Apply MRs directly (`kubectl apply -f managed-resources/`) |
| L3 | Compositions | Read/modify XRDs and Compositions; understand patching |
| L4 | Claims | Use `WebApplication` claim to provision full GCP stack with one manifest |
| L5 | EnvironmentConfig | Inject per-env values into compositions without changing claims |
| L6 | Composition functions | Use pipeline-mode functions; understand `function-auto-labeler` |
| L7 | Custom functions | Build and push a custom Go composition function via `custom-function/` |
| MAX | OCI packages | Package compositions + XRDs as OCI artifact; distribute via `crossplane.yaml` |

## Prerequisites

- Kubernetes cluster (K3s, kind, or managed) with `kubectl` configured
- Helm >= 3
- Crossplane >= 1.15
- GCP service account with permissions for compute, storage, networking

```bash
# Install Crossplane via Helm
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm install crossplane crossplane-stable/crossplane \
  --namespace crossplane-system --create-namespace \
  --values install/crossplane-helm-values.yaml

# Wait for Crossplane to be ready
kubectl -n crossplane-system wait --for=condition=ready pod -l app=crossplane --timeout=120s
```

## Quick Start

```bash
# 1. Install GCP provider
kubectl apply -f providers/provider-gcp.yaml
kubectl wait provider/provider-gcp --for=condition=Healthy --timeout=120s

# 2. Create GCP credentials secret
kubectl create secret generic gcp-creds \
  --from-file=creds=/home/hieudc/iac-devops-terraform.json \
  -n crossplane-system

# 3. Configure provider auth
kubectl apply -f providers/provider-config-gcp.yaml

# 4. Install XRD and Composition
kubectl apply -f compositions/xrd-web-application.yaml
kubectl apply -f compositions/composition-web-app-gcp.yaml

# 5. Submit a claim
kubectl apply -f compositions/claim-web-app-dev.yaml
kubectl get webapplication my-web-app-dev
```

## Usage Examples

### Provision GCP resources directly (no composition)

```bash
kubectl apply -f managed-resources/gcp-network-vpc.yaml
kubectl apply -f managed-resources/gcp-storage-bucket.yaml
kubectl get managed  # watch all managed resources
```

### Watch claim status

```bash
kubectl describe webapplication my-web-app-dev
kubectl get composite  # see the XR created by the claim
```

### Use EnvironmentConfig for per-env patching

```bash
kubectl apply -f environment-configs/env-dev.yaml
# Composition patches read from EnvironmentConfig automatically
```

### Build and push a custom composition function

```bash
cd custom-function
go build ./...
docker build -t your-registry/fn-auto-labeler:latest .
docker push your-registry/fn-auto-labeler:latest
# Update functions/function-auto-labeler.yaml with your image
kubectl apply -f functions/function-auto-labeler.yaml
```

### Package and distribute compositions as OCI

```bash
kubectl crossplane build configuration --name composition-package
kubectl crossplane push configuration your-registry/crossplane-gcp-config:v1.0.0
```

## Claim Schema (WebApplication)

```yaml
spec:
  parameters:
    appName: my-web-app          # required
    environment: dev             # required: dev | staging | prod
    region: us-central1          # required
    machineType: e2-micro        # GCE machine type
    diskSizeGb: 20
    bucketLocation: US-CENTRAL1
    enableVersioning: false
    networkCidr: "10.0.1.0/24"
```

## K3s Deployment Note

This repo targets K3s on an Oracle server (`10.10.0.5`) managed via Rancher. The Helm values in `install/crossplane-helm-values.yaml` are tuned for single-node resource limits (500m CPU, limited memory).
