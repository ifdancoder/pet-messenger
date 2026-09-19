# Simple Makefile for a Go project

# Build the application
all: build test

build:
	@echo "Building..."
	
	
	@go build -o main cmd/api/main.go

# Run the application
run:
	@go run cmd/api/main.go &
	@npm install --prefer-offline --no-fund --prefix ./frontend
	@npm run dev --prefix ./frontend
# Create DB container
docker-run:
	@if docker compose up --build 2>/dev/null; then \
		: ; \
	else \
		echo "Falling back to Docker Compose V1"; \
		docker-compose up --build; \
	fi

# Shutdown DB container
docker-down:
	@if docker compose down 2>/dev/null; then \
		: ; \
	else \
		echo "Falling back to Docker Compose V1"; \
		docker-compose down; \
	fi

# Test the application
test:
	@echo "Testing..."
	@go test ./... -v
# Integrations Tests for the application
itest:
	@echo "Running integration tests..."
	@go test ./internal/database ./internal/infrastructure/postgres -v

# Clean the binary
clean:
	@echo "Cleaning..."
	@rm -f main

# Live Reload
watch:
	@if command -v air > /dev/null; then \
            air; \
            echo "Watching...";\
        else \
            read -p "Go's 'air' is not installed on your machine. Do you want to install it? [Y/n] " choice; \
            if [ "$$choice" != "n" ] && [ "$$choice" != "N" ]; then \
                go install github.com/air-verse/air@latest; \
                air; \
                echo "Watching...";\
            else \
                echo "You chose not to install air. Exiting..."; \
                exit 1; \
            fi; \
        fi

# --- Minikube (see deploy/kubernetes/README.md) ---
MINIKUBE_NS := pet-messenger
# No standalone kubectl binary is assumed to be on PATH - only minikube
# itself ships one, via "minikube kubectl --" (see deploy-minikube.sh's own
# KUBECTL handling, which is what makes that multi-word form work at all).
MINIKUBE_KUBECTL := minikube kubectl --

# Starts the cluster itself; a no-op if it's already running.
minikube-start:
	@minikube status >/dev/null 2>&1 && echo "minikube already running" || minikube start

# Creates the namespace and the two secrets deploy-minikube.sh's workloads
# need (see README.md), but only the first time: re-running this would
# otherwise mint a fresh JWT_ED25519_SEED_BASE64 on every "make minikube-up"
# and silently invalidate every issued session.
minikube-secrets:
	@$(MINIKUBE_KUBECTL) create namespace $(MINIKUBE_NS) --dry-run=client -o yaml | $(MINIKUBE_KUBECTL) apply -f -
	@if $(MINIKUBE_KUBECTL) -n $(MINIKUBE_NS) get secret pet-messenger-secrets >/dev/null 2>&1; then \
		echo "secret/pet-messenger-secrets already exists, leaving it alone"; \
	else \
		echo "creating secret/pet-messenger-secrets"; \
		seed="$$(go run ./cmd/genkey)"; \
		$(MINIKUBE_KUBECTL) -n $(MINIKUBE_NS) create secret generic pet-messenger-secrets \
			--from-literal=JWT_ED25519_SEED_BASE64="$$seed" \
			--from-literal=BLUEPRINT_DB_PASSWORD=app; \
	fi
	@if $(MINIKUBE_KUBECTL) -n $(MINIKUBE_NS) get secret registry-credentials >/dev/null 2>&1; then \
		echo "secret/registry-credentials already exists, leaving it alone"; \
	elif [ -f "$$HOME/.docker/config.json" ]; then \
		echo "creating secret/registry-credentials"; \
		$(MINIKUBE_KUBECTL) -n $(MINIKUBE_NS) create secret generic registry-credentials \
			--type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson="$$HOME/.docker/config.json"; \
	else \
		echo "no ~/.docker/config.json found - skipping registry-credentials (only needed to pull from a real registry, not for deploy-minikube.sh's local-daemon images)"; \
	fi

# Builds the four images into minikube's own docker daemon and rolls the
# overlay out to match - see deploy-minikube.sh for what this actually does.
minikube-deploy:
	@KUBECTL="$(MINIKUBE_KUBECTL)" deploy/kubernetes/deploy-minikube.sh

# (Re)starts the frontend port-forward. Idempotent: safe to call again after
# minikube-deploy replaces the frontend pod and drops the previous tunnel.
minikube-port-forward:
	@fuser -k 8082/tcp >/dev/null 2>&1 || true
	@sleep 1
	@setsid nohup $(MINIKUBE_KUBECTL) -n $(MINIKUBE_NS) port-forward svc/frontend 8082:80 >/tmp/pet-messenger-port-forward.log 2>&1 </dev/null &
	@sleep 2
	@curl -sf -o /dev/null http://localhost:8082/ && echo "==> pet-messenger is up at http://localhost:8082/" || echo "==> port-forward started but http://localhost:8082/ isn't answering yet - check /tmp/pet-messenger-port-forward.log"

# One-shot: start the cluster, provision secrets, deploy, forward the port.
minikube-up: minikube-start minikube-secrets minikube-deploy minikube-port-forward

# Stops the port-forward and the cluster (state preserved - not a delete).
minikube-stop:
	@pkill -f "port-forward svc/frontend 8082:80" 2>/dev/null || true
	@minikube stop

.PHONY: all build run test clean watch docker-run docker-down itest \
	minikube-start minikube-secrets minikube-deploy minikube-port-forward minikube-up minikube-stop
