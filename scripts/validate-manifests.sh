#!/usr/bin/env bash
# Offline schema validation with `crossplane resource validate` (no cluster):
#   1. every manifest in the repo must validate against the Crossplane core schemas, the XRD and the provider CRDs,
#      and no kind may be skipped for lack of a schema (--error-on-missing-schemas);
#   2. every file in tests/invalid/ must be REJECTED, and the rejection must mention the text on its `# expect:` line.
#      That proves the validator can fail, and pins down the pre-v3 shapes this repo migrated away from.
# Needs the `crossplane` CLI on PATH. CROSSPLANE_VERSION picks the core schemas (default: the Helm chart's).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

crossplane_version="${CROSSPLANE_VERSION:-v2.4.3}"
schemas="$root/build/schemas"
cache="$root/build/xp-cache"
validate=(crossplane resource validate --cache-dir "$cache" --error-on-missing-schemas
  --crossplane-image "xpkg.crossplane.io/crossplane/crossplane:$crossplane_version")

"$root/scripts/fetch-schemas.sh" "$schemas"

echo "== all manifests validate"
"${validate[@]}" "$schemas" \
  compositions/,environment-configs/,functions/,providers/,local-lab/,managed-resources/,packages/composition-package.yaml,tests/xrs/

echo "== invalid manifests are rejected"
status=0
for f in tests/invalid/*.yaml; do
  expect="$(sed -n 's/^# expect: //p' "$f" | head -n 1)"
  [ -n "$expect" ] || { echo "FAIL $f has no '# expect:' line"; status=1; continue; }
  if out="$("${validate[@]}" "$schemas" "$f" 2>&1)"; then
    echo "FAIL $f was accepted"; status=1
  elif grep -qF -- "$expect" <<<"$out"; then
    echo "ok   $f rejected: $expect"
  else
    echo "FAIL $f was rejected, but not for '$expect':"; echo "$out"; status=1
  fi
done
exit "$status"
