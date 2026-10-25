# cert-manager (cluster-wide shared infrastructure)

cert-manager and the `ClusterIssuer` below are not specific to
pet-messenger — any namespace on this node can request a certificate
from the same issuer later. Install and apply both by hand, under the
admin kubeconfig; this is cluster infra, not any single project's CI
concern (see `../README.md`).

## Install cert-manager

Check https://cert-manager.io/docs/releases/ for the latest patch release
before running this — the version below was current when this was
written and may be outdated by the time you apply it:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector
```

This creates its own `cert-manager` namespace, separate from
`pet-messenger` and the other project's namespace — it is covered by the
node's 512Mi system-overhead reserve in `../README.md`'s arithmetic, not
by either project's own quota.

## Apply the ClusterIssuer

Edit `cluster-issuer.yaml` first: replace `REPLACE_ME@example.com` with a
real email you're willing to register with Let's Encrypt (it only
receives certificate-expiry warnings; it is not committed with a real
address on purpose — that's a one-time choice for whoever applies this,
not something to bake into the repo). Then:

```bash
kubectl apply -f cluster-issuer.yaml
kubectl get clusterissuer letsencrypt-prod -o jsonpath='{.status.conditions[0].message}'
```

Expected final status message: something like "The ACME account was
registered with the ACME server" — if it instead errors, the most common
cause is the email not being filled in, or the HTTP-01 challenge path
not being reachable from the public internet yet (confirm DNS is
pointed at this node before applying — see the production overlay's
Ingress, Task 5 of the implementation plan).

`cluster-issuer.yaml`'s solver uses `ingress: {class: nginx}`: this node
runs `ingress-nginx` as its real ingress controller, not k3s's built-in
Traefik (there is no `traefik` IngressClass registered on it at all). Any
project applying its own Ingress on this node needs
`ingressClassName: nginx` to match, or its traffic never reaches that
Ingress's rules, same as pet-messenger's own
`deploy/kubernetes/overlays/production/ingress.yaml`.
