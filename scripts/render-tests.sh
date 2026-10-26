#!/usr/bin/env bash
# Render every Composition with its sample XRs using `crossplane composition render` (the real Crossplane render
# engine and the real functions, run as containers) and assert on the output:
#   * tests/expected/<xr>.tsv holds `yq expression <TAB> expected value` lines, evaluated on the rendered stream;
#   * the rendered output must also validate against the XRD and provider schemas.
# The custom Go function is built from source and run on localhost:9443 (render's "Development" runtime).
# Needs: crossplane CLI, docker, yq (mikefarah v4), go. CROSSPLANE_VERSION picks the render engine.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

crossplane_version="${CROSSPLANE_VERSION:-v2.4.3}"
work="$root/build/render"
schemas="$root/build/schemas"
rm -rf "$work"
mkdir -p "$work"

"$root/scripts/fetch-schemas.sh" "$schemas"

echo "== build and start function-resource-labeler"
(cd custom-function && go build -o "$work/function-resource-labeler" .)
"$work/function-resource-labeler" --insecure --address 127.0.0.1:9443 >"$work/function.log" 2>&1 &
fn_pid=$!
trap 'kill "$fn_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do
  (exec 3<>/dev/tcp/127.0.0.1/9443) 2>/dev/null && break
  sleep 1
done
(exec 3<>/dev/tcp/127.0.0.1/9443) 2>/dev/null || { echo "function did not start:"; cat "$work/function.log"; exit 1; }

# xr file -> composition file. Each XR names its Composition by compositionRef or compositionSelector.
declare -A composition=(
  [xr-dev-gcp]=compositions/composition-web-app-gcp.yaml
  [xr-prod-gcp]=compositions/composition-web-app-gcp.yaml
  [xr-dev-nop]=local-lab/composition-web-app-nop.yaml
)

status=0
for xr in "${!composition[@]}"; do
  echo "== render $xr with ${composition[$xr]}"
  out="$work/$xr.rendered.yaml"
  crossplane composition render "tests/xrs/$xr.yaml" "${composition[$xr]}" tests/functions-render.yaml \
    --xrd compositions/xrd-web-application.yaml \
    --required-resources environment-configs/ \
    --crossplane-version "$crossplane_version" \
    --include-full-xr >"$out"

  while IFS=$'\t' read -r expr want; do
    [ -n "$expr" ] || continue
    got="$(yq ea "$expr" "$out" | paste -sd' ' -)"
    if [ "$got" = "$want" ]; then
      echo "ok   $expr = $want"
    else
      echo "FAIL $expr: want '$want', got '$got'"
      status=1
    fi
  done <"tests/expected/$xr.tsv"

  echo "== validate rendered $xr against the XRD and provider schemas"
  crossplane resource validate "$schemas" "$out" --error-on-missing-schemas \
    --cache-dir "$root/build/xp-cache" \
    --crossplane-image "xpkg.crossplane.io/crossplane/crossplane:$crossplane_version" || status=1
done
exit "$status"
