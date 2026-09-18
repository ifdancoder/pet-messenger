#!/usr/bin/env bash
# Builds the four service images straight into minikube's own docker daemon
# and rolls the minikube overlay out to match. Run this after every change
# you want to actually see in the cluster: there is no CI/registry pipeline
# for this stack (see ../kubernetes/README.md), so nothing else keeps the
# running pods in sync with the checked-out code.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

NAMESPACE=pet-messenger
OVERLAY=deploy/kubernetes/overlays/minikube

# An array, not a plain string: KUBECTL is documented (README.md) as
# overridable to the multi-word "minikube kubectl --" when no standalone
# kubectl binary is on PATH. Keeping it a string and quoting it below (as
# "$KUBECTL") would pass that whole phrase as a single argv[0], never split
# back into separate words - every call would fail with "minikube kubectl
# --: command not found".
read -ra KUBECTL <<<"${KUBECTL:-kubectl}"

# Unique on every run, not just on a new commit: a stable tag would leave
# `kubectl apply -k` with nothing to react to, since a Deployment whose
# spec text hasn't changed doesn't get a new rollout. The short SHA keeps
# the tag traceable back to the commit it was built from; the timestamp is
# what actually forces the rollout, dirty tree or not.
TAG="$(git rev-parse --short HEAD)-$(date +%s)"

echo "==> building :$TAG into minikube's docker daemon"
eval "$(minikube docker-env)"
docker build -q -t "pet-messenger-api:$TAG" -f deploy/media/Dockerfile.api . >/dev/null
docker build -q -t "pet-messenger-media:$TAG" -f deploy/media/Dockerfile.media . >/dev/null
docker build -q -t "pet-messenger-migrate:$TAG" -f deploy/kubernetes/Dockerfile.migrate . >/dev/null
docker build -q -t "pet-messenger-frontend:$TAG" -f frontend/Dockerfile frontend >/dev/null

echo "==> pointing $OVERLAY at :$TAG"
# Quoted: an unquoted tag is a YAML scalar, and a short SHA ending in a
# digit-letter-digit run that reads as scientific notation (a commit
# hash ending "e" followed by "-<all-digit timestamp>", e.g.
# "724920e-1789545873") parses as a number, not a string, and kustomize
# rejects it outright. The pattern matches either quoted or unquoted
# form so this stays idempotent across repeated runs.
sed -i -E "s/newTag: \"?[A-Za-z0-9._-]+\"?/newTag: \"${TAG}\"/g" "$OVERLAY/kustomization.yaml"

# Job pod templates are immutable after creation, so a plain apply on top of
# an already-run migrate Job fails with "field is immutable" instead of
# picking up the new image — always start it fresh.
echo "==> re-running migrations"
"${KUBECTL[@]}" delete job -n "$NAMESPACE" migrate --ignore-not-found

echo "==> applying $OVERLAY"
"${KUBECTL[@]}" apply -k "$OVERLAY"

"${KUBECTL[@]}" -n "$NAMESPACE" wait --for=condition=complete job/migrate --timeout=120s
"${KUBECTL[@]}" -n "$NAMESPACE" rollout status deployment/api
"${KUBECTL[@]}" -n "$NAMESPACE" rollout status deployment/media
"${KUBECTL[@]}" -n "$NAMESPACE" rollout status deployment/frontend

echo "==> done: cluster now runs $TAG"
