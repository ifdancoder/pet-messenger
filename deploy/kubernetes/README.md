# Kubernetes / Minikube

This is stage 5 of the backend architecture: the same base manifests run the
local Minikube stack; production supplies its own overlay and managed Postgres,
Redis, and object storage.

## Minikube

Start Minikube, create the two required secrets, then apply the overlay. The
base image tags point at the configured private registry; change them in a
production overlay when publishing a new release.

```bash
minikube start
JWT_ED25519_SEED_BASE64="$(go run ./cmd/genkey)"
minikube kubectl -- create namespace pet-messenger --dry-run=client -o yaml | minikube kubectl -- apply -f -
minikube kubectl -- -n pet-messenger create secret generic pet-messenger-secrets \
  --from-literal=JWT_ED25519_SEED_BASE64="$JWT_ED25519_SEED_BASE64" \
  --from-literal=BLUEPRINT_DB_PASSWORD=app --dry-run=client -o yaml | minikube kubectl -- apply -f -
minikube kubectl -- -n pet-messenger create secret generic registry-credentials \
  --type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson="$HOME/.docker/config.json" \
  --dry-run=client -o yaml | minikube kubectl -- apply -f -
KUBECTL="minikube kubectl --" deploy/kubernetes/deploy-minikube.sh
```

There is no CI/registry pipeline for this stack — the base manifests' image
tags exist only for a real release (see below). `deploy-minikube.sh` is what
actually gets local changes running: it builds all four images straight into
minikube's own docker daemon, retags the `overlays/minikube` kustomization to
match (every build gets a fresh, unique tag so `kubectl apply -k` always sees
a changed Deployment and actually rolls out — a stable tag would make repeat
applies a no-op), re-runs the migration Job, and waits for every rollout.
Run it after any change you want to see in the cluster; `kubectl apply -k`
alone will not pick up new code. `KUBECTL` defaults to plain `kubectl`;
override it as above if only `minikube kubectl --` is on your PATH.

For the manual call page, forward the API and media WebSocket ports:

```bash
minikube kubectl -- -n pet-messenger port-forward svc/frontend 8082:80
minikube kubectl -- -n pet-messenger port-forward svc/media 8081:8081
```

Open `http://localhost:8082/`. The frontend proxies `/api` and the WebSocket to
the API service. The Minikube overlay advertises `localhost:8081` to the
browser, matching the media port-forward. Coturn uses
the Minikube node network; TURN relay verification needs a host-reachable TURN
address and is therefore a production-overlay concern.

## Production

Production runs on an existing k3s node shared with one other project.
Unlike Minikube, there is a real CI/CD pipeline for this path — see
`.github/workflows/ci.yml` (tests every PR) and `.github/workflows/deploy.yml`
(builds, pushes, and deploys on every push to `main`, via a self-hosted
GitHub Actions runner that lives on the target server). The full design
and the per-workload memory/CPU numbers behind it are in
`../../docs/superpowers/specs/2026-10-12-k8s-shared-server-deploy-cicd-design.md`.

### One-time manual setup (not automated by CI)

1. Register a self-hosted GitHub Actions runner on the target server
   (GitHub repo → Settings → Actions → Runners → New self-hosted runner),
   scoped to this repository. Give its service user read access to the
   k3s kubeconfig (typically `/etc/rancher/k3s/k3s.yaml`, root-owned by
   default) — either run the runner as a user in the right group, or
   copy a scoped kubeconfig into the runner's home directory and set
   `KUBECONFIG` in its environment.
2. Set the repository variable `REGISTRY` (Settings → Secrets and
   variables → Actions → Variables) to the real registry host:port. The
   `130.17.20.23:5000` that used to be hardcoded in the base manifests is
   explicitly unconfirmed — do not reuse it without checking.
3. Point `messenger.ifdancoder.ru`'s DNS A record at the server's public IP.
4. Apply the cluster-wide pieces by hand, under the admin kubeconfig (see
   `../../cluster/README.md` and `../../cluster/cert-manager/README.md`):
   `cluster/quotas/pet-messenger.yaml` and cert-manager + its
   `ClusterIssuer`.
5. Create the runtime secrets in the real cluster's `pet-messenger`
   namespace — the same `kubectl create secret` commands the Minikube
   section above already documents, run against the real cluster's
   context instead of `minikube kubectl --`.
6. Once the other project's administrator knows its real memory/CPU
   need, fill in and apply `cluster/quotas/other-project.yaml`.

### Known limitation to watch for

Traefik's own entrypoint idle timeout (commonly 180s by default in k3s)
sits in front of `frontend/nginx.conf`'s deliberate 1-hour timeout on
`/rtc`. If call signaling drops after roughly 3 minutes of quiet in
production specifically (not reproducible via the Minikube port-forward
path, which never goes through Traefik), raise or disable that
entrypoint timeout — it was not pre-emptively changed here because the
k3s default may already be long enough; confirm against real call
behavior rather than a guessed value.
