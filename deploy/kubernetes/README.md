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
minikube kubectl -- apply -k deploy/kubernetes/overlays/minikube
minikube kubectl -- -n pet-messenger wait --for=condition=complete job/migrate --timeout=120s
minikube kubectl -- -n pet-messenger rollout status deployment/api
minikube kubectl -- -n pet-messenger rollout status deployment/media
minikube kubectl -- -n pet-messenger rollout status deployment/frontend
```

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
