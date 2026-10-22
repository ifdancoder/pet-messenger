# Production deploy guide

Runbook for taking pet-messenger from "code on `main`" to "running at
`https://messenger.ifdancoder.ru`" on the shared k3s node, and for every
deploy after the first one. The design this follows is
`superpowers/specs/2026-10-12-k8s-shared-server-deploy-cicd-design.md`;
this document is the operator-facing checklist version of it, plus the
gaps found while reviewing it for completeness.

The node is shared with one other, unrelated project. Nothing here may
make assumptions about that project beyond what `cluster/README.md`
already states.

## Before the first deploy: one-time manual setup

These are infrastructure actions, not CI. None of them are automated by
`deploy.yml` on purpose, per `cluster/README.md`: a project's own pipeline
must never be able to edit the ceiling that constrains it. Do them in
this order.

### 1. DNS

Point `messenger.ifdancoder.ru`'s A record at the server's public IP.
cert-manager's HTTP-01 challenge (step 3) needs this resolvable from the
public internet before it will succeed.

### 2. Cluster-wide quotas

```bash
kubectl apply -f cluster/quotas/pet-messenger.yaml
```

Read `cluster/README.md` first if the node's total RAM is not 4096Mi, or
if the other project's footprint has changed: the numbers in that file
are arithmetic against a specific node size, not a universal default.

**`cluster/quotas/other-project.yaml` is still a placeholder.** Its
namespace name and numbers are TODOs for the other project's own
administrator, not something this repo can fill in. Don't block
pet-messenger's own deploy on it, but don't forget it either: until it's
applied with real numbers, that namespace has no ceiling, and the 1792Mi
"left over" in the node arithmetic is unenforced. Follow up with whoever
runs that project.

### 3. cert-manager + ClusterIssuer

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector
```

Check https://cert-manager.io/docs/releases/ for a newer patch release
first: the pinned version above may be stale by the time you run this.

Edit `cluster/cert-manager/cluster-issuer.yaml`: replace
`REPLACE_ME@example.com` with a real address willing to receive
Let's Encrypt expiry notices. Then:

```bash
kubectl apply -f cluster/cert-manager/cluster-issuer.yaml
kubectl get clusterissuer letsencrypt-prod -o jsonpath='{.status.conditions[0].message}'
```

Expect something like "The ACME account was registered with the ACME
server". If it errors, the usual cause is DNS from step 1 not resolving
yet, or the email field still saying `REPLACE_ME@example.com`.

### 4. GitHub Actions self-hosted runner

Register a runner on the target server, scoped to this repository
(GitHub → repo → Settings → Actions → Runners → New self-hosted runner).
Give its service user read access to the k3s kubeconfig, typically
`/etc/rancher/k3s/k3s.yaml`, root-owned by default: either run the
runner as a user in the right group, or copy a scoped kubeconfig into
the runner's home and set `KUBECONFIG` in its environment.

`deploy.yml`'s `build-and-push` and `deploy` jobs both run on this
runner specifically, so neither registry credentials nor the cluster API
ever need to leave the server.

### 5. Registry

Set the repository variable `REGISTRY` (Settings → Secrets and
variables → Actions → Variables → New repository variable) to the real
registry host:port. `130.17.20.23:5000`, hardcoded in old manifests
before this overlay existed, is explicitly unconfirmed: do not reuse it
without checking it still points somewhere real and reachable from the
runner.

### 6. Runtime secrets

```bash
JWT_ED25519_SEED_BASE64="$(go run ./cmd/genkey)"
kubectl create namespace pet-messenger --dry-run=client -o yaml | kubectl apply -f -
kubectl -n pet-messenger create secret generic pet-messenger-secrets \
  --from-literal=JWT_ED25519_SEED_BASE64="$JWT_ED25519_SEED_BASE64" \
  --from-literal=BLUEPRINT_DB_PASSWORD=<a real password, not "app">
kubectl -n pet-messenger create secret generic registry-credentials \
  --type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson="$HOME/.docker/config.json"
```

Keep `JWT_ED25519_SEED_BASE64` somewhere safe outside the cluster too:
losing it invalidates every issued session and refresh token with no way
back.

### 7. Replace the dev placeholders baked into the ConfigMap

`deploy/kubernetes/base/config.yaml` ships `MEDIA_RPC_TOKEN: devtoken`
and `TURN_SECRET: devsecret` as literal values, committed in plain text.
They're fine for minikube; shipped unchanged to production they mean the
gRPC control plane between `api` and `media`, and TURN credential
minting, both run on a password anyone who reads this repo already
knows. The production overlay does not currently override them.

Fix before going live, not after: move both into
`pet-messenger-secrets` instead of the ConfigMap, drop them from
`config.yaml`, and add a small patch to
`deploy/kubernetes/overlays/production/kustomization.yaml` (or just
extend step 6's `kubectl create secret` command) so `api`/`media`'s
existing `envFrom: [..., {secretRef: {name: pet-messenger-secrets}}]`
picks up the real values the same way `JWT_ED25519_SEED_BASE64` already
does. `TURN_URL`/`STUN_URLS` in the same ConfigMap also still point at
`127.0.0.1:3478`: update them to the real coturn address reachable from
outside the cluster once that's known, or calls will fail to establish a
TURN relay path from a real client network.

## First deploy

Everything past this point is what `deploy.yml` does automatically on
every push to `main`: the first deploy is not special, it is just the
first time this sequence runs.

1. `test-go` and `test-frontend` re-run (not just trusting the PR's
   earlier CI run, since `main` can move between approval and merge).
2. `build-and-push` (self-hosted runner): builds and pushes all four
   images (`api`, `media`, `migrate`, `frontend`) tagged
   `${REGISTRY}/pet-messenger-<name>:${GITHUB_SHA}`.
3. `deploy` (self-hosted runner): rewrites
   `deploy/kubernetes/overlays/production/kustomization.yaml`'s `images:` block to the new
   registry+tag, deletes and reapplies the `migrate` Job (Job pod
   templates are immutable, so it can't just be patched), applies
   `kubectl apply -k overlays/production`, then waits on the migrate Job
   and all three Deployments' rollout status.

Trigger it by merging to `main`. Watch the run in the Actions tab; a
failure in `deploy` most often means one of the one-time setup steps
above was skipped (missing secret, missing quota so pods hang `Pending`,
runner not reachable).

## After the first deploy: verify

- `kubectl -n pet-messenger get pods`: every pod `Running`, `migrate`
  `Completed`.
- `kubectl get certificate -n pet-messenger`: `READY: True` for
  `pet-messenger-tls`. If stuck, check `kubectl describe certificate` and
  the `ClusterIssuer`'s status from step 3.
- `curl https://messenger.ifdancoder.ru/api/health` returns the DB health
  payload.
- Open `https://messenger.ifdancoder.ru/` in two browser tabs (or two
  devices), start a call between them, and **leave it idle for 4-5
  minutes** without talking. This is the one known, not-pre-solved risk
  in the design: Traefik's own entrypoint `idleTimeout` (commonly 180s
  in k3s) sits in front of `frontend/nginx.conf`'s deliberate 1-hour
  timeout on `/rtc`. If signaling drops around the 3-minute mark here
  but never did through the minikube port-forward path (which never
  passes through Traefik), that confirms it: fix by raising or
  disabling Traefik's entrypoint idle timeout (`--entrypoints.web.transport.respondingTimeouts.idleTimeout`
  or the equivalent Helm/k3s server-arg, depending on how Traefik is
  configured on this node). Don't change it pre-emptively without this
  test: the k3s default may already be long enough, and this is
  cluster-wide Traefik config, not something pet-messenger's own
  manifests control.
- Confirm with the other project's administrator whether
  `cluster/quotas/other-project.yaml` has been filled in with real
  numbers and applied yet (step 2's follow-up). Not blocking, but easy
  to forget once pet-messenger itself is up and working.

## Every deploy after the first

Just merge to `main`. No manual steps repeat unless something changes
them: a new secret value, a new one-time dependency (another
cert-manager-style shared service), or a resource-sizing change big
enough to need `cluster/quotas/pet-messenger.yaml` revisited (and then
hand-applied again: CI still cannot touch that file).

## Rollback

`deploy.yml` always deploys the exact image built from the `main` commit
it ran on. To roll back:

```bash
git revert <bad commit>   # or reset main to a known-good commit and force-push, if the team is fine with that
git push origin main
```

A new push re-runs the whole pipeline and redeploys the reverted code.
There is no separate "rollback" job: reverting the source and letting
CI redeploy is the rollback.

If the cluster itself needs an immediate rollback faster than a new CI
run (an active incident), redeploy a known-good tag directly without
waiting for the pipeline:

```bash
kubectl -n pet-messenger set image deployment/api api=${REGISTRY}/pet-messenger-api:<previous-sha>
kubectl -n pet-messenger set image deployment/media media=${REGISTRY}/pet-messenger-media:<previous-sha>
kubectl -n pet-messenger set image deployment/frontend frontend=${REGISTRY}/pet-messenger-frontend:<previous-sha>
```

Follow up by reverting in git too, so the next ordinary merge to `main`
doesn't undo the emergency rollback.

## Troubleshooting

- **Pods stuck `Pending`**: almost always the namespace quota (step 2).
  `kubectl -n pet-messenger describe resourcequota pet-messenger-quota`
  shows current usage against the hard limits.
- **`migrate` Job stuck or erroring on reapply**: Job pod templates are
  immutable; `deploy.yml` already deletes and recreates it each run. If
  running by hand, do the same: `kubectl delete job -n pet-messenger migrate --ignore-not-found`
  before reapplying.
- **Certificate never becomes `Ready`**: check DNS (step 1) actually
  resolves to this node, and that nothing else is intercepting port 80
  for the HTTP-01 challenge.
- **Calls drop signaling after ~3 minutes in production only**: see the
  Traefik `idleTimeout` note under "After the first deploy: verify".
