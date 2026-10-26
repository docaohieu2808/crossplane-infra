#!/usr/bin/env bash
# Build the Configuration package (XRD + GCP Composition + dependencies) into build/crossplane-web-app-gcp.xpkg.
# Usage: scripts/build-configuration-package.sh   (needs the `crossplane` CLI on PATH)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stage="$root/build/package-root"

rm -rf "$stage"
mkdir -p "$stage"
cp "$root/packages/crossplane.yaml" "$stage/crossplane.yaml"
cp "$root/compositions/xrd-web-application.yaml" "$stage/xrd.yaml"
cp "$root/compositions/composition-web-app-gcp.yaml" "$stage/composition.yaml"

crossplane xpkg build --package-root="$stage" --package-file="$root/build/crossplane-web-app-gcp.xpkg"
echo "built $root/build/crossplane-web-app-gcp.xpkg"
