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

Point three A records at the server's public IP: `messenger.ifdancoder.ru`,
`turn.messenger.ifdancoder.ru`, and `stun.messenger.ifdancoder.ru`.
cert-manager's HTTP-01 challenge (step 3) needs the first one resolvable
from the public internet before it will succeed; the other two are
coturn's own address (step 7) and carry no TLS cert of their own, since
coturn runs with `no-tls`/`no-dtls` and is reached over raw UDP/TCP on
port 3478, not through Traefik.

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

`cluster/cert-manager/cluster-issuer.yaml`'s `email` is already set to a
real address (not the `REPLACE_ME@example.com` placeholder); change it
first if a different address should receive Let's Encrypt expiry
notices. Then:

```bash
kubectl apply -f cluster/cert-manager/cluster-issuer.yaml
kubectl get clusterissuer letsencrypt-prod -o jsonpath='{.status.conditions[0].message}'
```

Expect something like "The ACME account was registered with the ACME
server". If it errors, the usual cause is DNS from step 1 not resolving
yet.

### 4. GitHub Actions self-hosted runner

Register a runner on the target server, scoped to this repository
(GitHub → repo → Settings → Actions → Runners → New self-hosted runner).
Give its service user read access to the k3s kubeconfig, typically
`/etc/rancher/k3s/k3s.yaml`, root-owned by default: either run the
runner as a user in the right group, or copy a scoped kubeconfig into
the runner's home and set `KUBECONFIG` in its environment.

`deploy.yml`'s `build-and-import` and `deploy` jobs both run on this
runner specifically, so the cluster API never needs to leave the server.
There is no registry in this design at all: `build-and-import` builds
each image with the host's Docker daemon, then hands it straight to
k3s's own containerd (`k3s ctr images import`), since k3s runs its own
embedded containerd (`/run/k3s/containerd/containerd.sock`), entirely
separate from the one the host's `dockerd` uses
(`/run/containerd/containerd.sock`): an image `docker build` produces
is invisible to k3s until that import step hands it over.

### 5. Passwordless sudo for the k3s image import

The runner's own user needs to run `k3s ctr images import` without a
password prompt (CI has no terminal to type one into). Add a sudoers
rule scoped to exactly that command, as root:

```bash
echo '<runner-user> ALL=(root) NOPASSWD: /usr/local/bin/k3s ctr images import -' \
  | sudo tee /etc/sudoers.d/pet-messenger-runner-k3s-ctr
sudo visudo -c   # validates every file under /etc/sudoers.d/, catches a typo before it locks sudo out
```

Replace `<runner-user>` with whichever user actually runs the self-hosted
runner service (not necessarily the one from step 4's kubeconfig access).

### 6. Runtime secrets

```bash
JWT_ED25519_SEED_BASE64="$(go run ./cmd/genkey)"
MEDIA_RPC_TOKEN="$(openssl rand -hex 32)"
TURN_SECRET="$(openssl rand -hex 32)"
kubectl create namespace pet-messenger --dry-run=client -o yaml | kubectl apply -f -
kubectl -n pet-messenger create secret generic pet-messenger-secrets \
  --from-literal=JWT_ED25519_SEED_BASE64="$JWT_ED25519_SEED_BASE64" \
  --from-literal=BLUEPRINT_DB_PASSWORD=<a real password, not "app"> \
  --from-literal=MEDIA_RPC_TOKEN="$MEDIA_RPC_TOKEN" \
  --from-literal=TURN_SECRET="$TURN_SECRET"
```

`registry-credentials` (`imagePullSecrets` in `base/{api,media,migrate,frontend}.yaml`)
is not needed with this design: there's no registry to pull from, so
kubelet never attempts a pull that secret would authenticate. The
reference to it stays in the base manifests anyway, so switching to a
real registry later costs nothing. Only create it if that switch
actually happens:

```bash
kubectl -n pet-messenger create secret generic registry-credentials \
  --type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson="$HOME/.docker/config.json"
```

`MEDIA_RPC_TOKEN` (gRPC control plane between `api` and `media`) and
`TURN_SECRET` (TURN credential minting) used to ship as plain-text
`devtoken`/`devsecret` literals in the committed `config.yaml` ConfigMap,
with no production override. They're now sourced from this Secret
instead, the same way `JWT_ED25519_SEED_BASE64` already was; `api` and
`media`'s existing `envFrom: [..., {secretRef: {name: pet-messenger-secrets}}]`
picks them up with no manifest change. Minikube still defaults both to
`devtoken`/`devsecret` (see `make minikube-secrets`), which is fine for
a local stack nothing else can reach.

**`TURN_SECRET` has a second copy that must match.** coturn validates
TURN credentials against its own `static-auth-secret` in
`deploy/kubernetes/base/coturn.yaml`, which still reads the literal
`devsecret`. Edit that line to the same value as `$TURN_SECRET` above
before applying to production, or TURN auth will fail: the api/media
pods and coturn will be signing and checking against two different
secrets.

#### Optional: back the same values up as GitHub Actions secrets

Nothing in `ci.yml`/`deploy.yml` reads these today, so this step is not
required for the pipeline to work. It exists only so the values above
aren't the one and only copy, living solely in the cluster's etcd and
whoever's terminal history generated them. Run it right after the
`kubectl create secret` step above, reusing the same shell variables:

```bash
gh secret set JWT_ED25519_SEED_BASE64 --body "$JWT_ED25519_SEED_BASE64"
gh secret set MEDIA_RPC_TOKEN --body "$MEDIA_RPC_TOKEN"
gh secret set TURN_SECRET --body "$TURN_SECRET"
gh secret set BLUEPRINT_DB_PASSWORD --body "<the same real password used above>"
```

If a workflow is later changed to apply the cluster Secret itself
(instead of a human running `kubectl create secret` by hand), it would
read these as `${{ secrets.JWT_ED25519_SEED_BASE64 }}` etc. That change
is not part of this design; these commands alone only store the values
on GitHub, they do not change anything in the running cluster.

Keep all of these somewhere safe outside the cluster too: losing
`JWT_ED25519_SEED_BASE64` invalidates every issued session and refresh
token with no way back.

### 7. TURN relay address

Already done: `overlays/production/kustomization.yaml` carries a patch
pointing `TURN_URL`/`STUN_URLS` at `turn.messenger.ifdancoder.ru:3478`
and `stun.messenger.ifdancoder.ru:3478` (step 1's DNS records), while
`base/config.yaml` itself stays at `127.0.0.1`, which is what Minikube's
overlay still needs (coturn on the same node there). If that domain ever
changes, edit the patch in the production overlay, not the base
ConfigMap, or Minikube's own TURN testing breaks.

coturn's own `static-auth-secret` still needs to match `TURN_SECRET`
from step 6, by hand, every time `TURN_SECRET` is regenerated: see that
step's note.

## First deploy

Everything past this point is what `deploy.yml` does automatically on
every push to `main`: the first deploy is not special, it is just the
first time this sequence runs.

1. `test-go` and `test-frontend` re-run (not just trusting the PR's
   earlier CI run, since `main` can move between approval and merge).
2. `build-and-import` (self-hosted runner): builds all four images
   (`api`, `media`, `migrate`, `frontend`) tagged
   `pet-messenger-<name>:${GITHUB_SHA}`, then imports each one straight
   into k3s's own containerd (`k3s ctr images import`); there is no
   registry to push to.
3. `deploy` (self-hosted runner): rewrites
   `deploy/kubernetes/overlays/production/kustomization.yaml`'s `images:` block to the new
   tag, deletes and reapplies the `migrate` Job (Job pod
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
kubectl -n pet-messenger set image deployment/api api=pet-messenger-api:<previous-sha>
kubectl -n pet-messenger set image deployment/media media=pet-messenger-media:<previous-sha>
kubectl -n pet-messenger set image deployment/frontend frontend=pet-messenger-frontend:<previous-sha>
```

This only works if `<previous-sha>` is still sitting in k3s's own
containerd image store (`sudo k3s ctr images ls`): with no registry,
there is nowhere else for `IfNotPresent` to pull a missing one from. If
it's gone, `git revert` and letting CI rebuild it is the only path back.

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
