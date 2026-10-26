#!/usr/bin/env bash
# Download the CRDs the offline validation needs into a directory (default build/schemas):
# the namespaced GCP managed resources + ClusterProviderConfig, provider-nop's NopResource, the XRD and the
# function input schemas.
# Versions are read from providers/provider-gcp.yaml and local-lab/provider-nop.yaml, so bumping a provider there
# bumps the schemas CI validates against. (Pulling the provider packages would give the same CRDs but costs
# hundreds of MB; the CRDs are published in the provider repositories under package/crds.)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$root/build/schemas}"
mkdir -p "$out"

gcp_version="$(grep -oE 'provider-gcp-compute:v[0-9]+\.[0-9]+\.[0-9]+' "$root/providers/provider-gcp.yaml" | cut -d: -f2)"
nop_version="$(grep -oE 'provider-nop:v[0-9]+\.[0-9]+\.[0-9]+' "$root/local-lab/provider-nop.yaml" | cut -d: -f2)"
if [ -z "$gcp_version" ] || [ -z "$nop_version" ]; then
  echo "cannot read provider versions" >&2
  exit 1
fi

fetch() { # repo version crd-file-name
  curl --fail --silent --show-error --location --retry 3 \
    "https://raw.githubusercontent.com/$1/$2/package/crds/$3" --output "$out/$3"
}

gcp=crossplane-contrib/provider-upjet-gcp
for crd in \
  compute.gcp.m.upbound.io_networks.yaml \
  compute.gcp.m.upbound.io_subnetworks.yaml \
  compute.gcp.m.upbound.io_firewalls.yaml \
  compute.gcp.m.upbound.io_instances.yaml \
  storage.gcp.m.upbound.io_buckets.yaml \
  gcp.m.upbound.io_clusterproviderconfigs.yaml; do
  fetch "$gcp" "$gcp_version" "$crd"
done
fetch crossplane-contrib/provider-nop "$nop_version" nop.crossplane.io_nopresources.yaml

# The XRD (schema of WebApplication), the public Functions (validate pulls their packages to learn the schema
# of each pipeline step's input) and the custom function's input CRD (it has no public package to pull).
cp "$root/compositions/xrd-web-application.yaml" "$out/xrd-web-application.yaml"
for f in "$root"/functions/function-*.yaml; do
  [ "$(basename "$f")" = function-resource-labeler.yaml ] || cp "$f" "$out/"
done
cp "$root"/custom-function/package/input/*.yaml "$out/"
echo "schemas for provider-upjet-gcp $gcp_version and provider-nop $nop_version in $out"
