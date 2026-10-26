#!/usr/bin/env bash
# End-to-end test on a kind cluster, no cloud account needed. Proves that the XRD, the Compositions, the three
# public functions AND the custom Go function really reconcile a WebApplication to Ready:
#   1. install Crossplane from the Helm chart with install/crossplane-helm-values.yaml;
#   2. start a throwaway local registry, build the custom function image + xpkg and push it there;
#   3. install the functions, provider-nop (a provider that manages nothing), the EnvironmentConfigs, XRD, Compositions;
#   4. create a WebApplication that selects the provider-nop Composition, wait for Ready, check what it produced;
#   5. delete it and check everything composed from it is gone.
# Needs: a running kind cluster (kubectl context set to it), helm, docker, the crossplane CLI, curl.
# The GCP Composition is applied too (proves it is accepted) but not reconciled: that needs a GCP project.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

chart_version="${CROSSPLANE_CHART_VERSION:-2.4.3}"
registry_name=xp-ci-registry
kind_network=kind
xr=shop-dev-nop
ns=team-shop

echo "== install Crossplane $chart_version"
helm repo add crossplane-stable https://charts.crossplane.io/stable >/dev/null
helm repo update >/dev/null
helm upgrade --install crossplane crossplane-stable/crossplane --version "$chart_version" \
  --namespace crossplane-system --create-namespace -f install/crossplane-helm-values.yaml --wait --timeout 5m

echo "== local registry for the custom function"
docker rm -f "$registry_name" >/dev/null 2>&1 || true
docker run -d --name "$registry_name" --network "$kind_network" -p 127.0.0.1:5001:5000 registry:3.1.2 >/dev/null
registry_ip="$(docker inspect -f "{{(index .NetworkSettings.Networks \"$kind_network\").IPAddress}}" "$registry_name")"
[ -n "$registry_ip" ] || { echo "registry has no IP on the $kind_network network" >&2; exit 1; }
# A private IP is spoken to over plain HTTP by Crossplane's package manager (it follows the go-containerregistry
# rules); the node's containerd needs the same hint to pull the function image.
for node in $(kind get nodes); do
  docker exec "$node" mkdir -p "/etc/containerd/certs.d/$registry_ip:5000"
  docker exec -i "$node" tee "/etc/containerd/certs.d/$registry_ip:5000/hosts.toml" >/dev/null <<TOML
server = "http://$registry_ip:5000"

[host."http://$registry_ip:5000"]
  capabilities = ["pull", "resolve"]
TOML
done

echo "== build and push function-resource-labeler"
mkdir -p build
docker build --build-arg GO_VERSION="${GO_VERSION:-1.27}" --tag function-resource-labeler:ci custom-function
crossplane xpkg build --package-root custom-function/package \
  --embed-runtime-image function-resource-labeler:ci --package-file build/function-resource-labeler.xpkg
crossplane xpkg push --package-files build/function-resource-labeler.xpkg \
  "localhost:5001/function-resource-labeler:v0.1.0"

echo "== install functions"
kubectl apply -f functions/function-patch-and-transform.yaml -f functions/function-environment-configs.yaml \
  -f functions/function-go-templating.yaml -f functions/function-auto-ready.yaml
sed "s#registry.example.com/function-resource-labeler:v0.1.0#$registry_ip:5000/function-resource-labeler:v0.1.0#" \
  functions/function-resource-labeler.yaml | kubectl apply -f -
kubectl wait functions.pkg.crossplane.io --all --for=condition=Healthy --timeout=300s

echo "== install provider-nop"
kubectl apply -f local-lab/provider-nop.yaml
kubectl wait providers.pkg.crossplane.io/provider-nop --for=condition=Healthy --timeout=300s
kubectl wait crd/nopresources.nop.crossplane.io --for=condition=Established --timeout=120s

echo "== install EnvironmentConfigs, XRD and Compositions"
kubectl apply -f environment-configs/
kubectl apply -f compositions/xrd-web-application.yaml
kubectl wait xrd/webapplications.infra.example.io --for=condition=Established --timeout=120s
kubectl apply -f compositions/composition-web-app-gcp.yaml -f local-lab/composition-web-app-nop.yaml
kubectl get compositions.apiextensions.crossplane.io web-app-gcp web-app-nop

echo "== create $xr and wait for Ready"
kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f tests/xrs/xr-dev-nop.yaml
kubectl -n "$ns" wait "webapplications.infra.example.io/$xr" --for=condition=Ready --timeout=300s

echo "== check what was composed"
status=0
check() { # description want got
  if [ "$2" = "$3" ]; then echo "ok   $1 = $2"; else echo "FAIL $1: want '$2', got '$3'"; status=1; fi
}
nops() { kubectl -n "$ns" get nopresources.nop.crossplane.io "$@"; }
check "composed NopResources" 3 "$(nops -l "crossplane.io/composite=$xr" --no-headers | wc -l | tr -d ' ')"
check "NopResources labelled by the custom function" 3 \
  "$(nops -l "crossplane.io/composite=$xr,infra.example.io/composition=web-app-nop,app.kubernetes.io/managed-by=crossplane" --no-headers | wc -l | tr -d ' ')"
check "all NopResources Ready" "True True True" \
  "$(nops -l "crossplane.io/composite=$xr" -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{" "}{end}' | xargs)"
check "status.vpcNetworkName" "$xr-vpc" "$(kubectl -n "$ns" get webapplications.infra.example.io "$xr" -o jsonpath='{.status.vpcNetworkName}')"
check "vm machineType from the XR" "e2-small" "$(nops "$xr-vm" -o jsonpath='{.spec.forProvider.fields.machineType}')"
check "vm diskSizeGb from the EnvironmentConfig" "20" "$(nops "$xr-vm" -o jsonpath='{.spec.forProvider.fields.diskSizeGb}')"

echo "== delete $xr and wait for cleanup"
kubectl -n "$ns" delete "webapplications.infra.example.io/$xr" --wait --timeout=300s
check "NopResources left after delete" 0 "$(nops --no-headers 2>/dev/null | wc -l | tr -d ' ')"

exit "$status"
