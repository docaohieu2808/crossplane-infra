#!/usr/bin/env bash
# Print what is needed to debug a failed kind-e2e.sh run. Never fails: it is for humans reading a CI log.
set +e

kubectl get pods -A
kubectl get providers.pkg.crossplane.io,providerrevisions.pkg.crossplane.io,functions.pkg.crossplane.io,functionrevisions.pkg.crossplane.io -o wide
kubectl describe functions.pkg.crossplane.io
kubectl get xrd,compositions.apiextensions.crossplane.io
kubectl get webapplications.infra.example.io -A -o yaml
kubectl get nopresources.nop.crossplane.io -A -o yaml
kubectl get events -A --sort-by=.lastTimestamp | tail -n 60
kubectl -n crossplane-system logs deploy/crossplane --tail=200
for d in $(kubectl -n crossplane-system get deploy -o name | grep -E 'function|provider'); do
  echo "--- $d"
  kubectl -n crossplane-system logs "$d" --tail=100
done
exit 0
