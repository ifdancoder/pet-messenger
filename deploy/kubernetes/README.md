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
