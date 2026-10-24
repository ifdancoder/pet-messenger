# pet-messenger

*[Русская версия](README.ru.md)*

Backend (plus a small React client) for a real-time messenger: direct
messages, Slack-style group chats and Telegram-style broadcast channels,
group audio/video calls with screen sharing over a self-hosted WebRTC
SFU, web push notifications, and end-to-end-encrypted call key exchange.

The backend is split into two services sharing one Go module:
`api-service` (stateless: auth, chats, channels, notifications, WebSocket
fan-out) and `media-service` (stateful: the Pion SFU, one call lives on
one pod for its whole duration). They're built as a Clean Architecture +
Hexagonal + CQRS retrofit, organized into 8 independent bounded contexts.

## Tech stack

- Go 1.26, Gin (HTTP), `uber-go/fx` (composition root / dependency injection)
- PostgreSQL via `database/sql` + `pgx`, `golang-migrate` for schema migrations
- Redis (presence tracking, caching), NATS JetStream (durable, at-least-once cross-pod event fan-out)
- Pion WebRTC (the SFU itself) + coturn (TURN/STUN relay) for calls
- MinIO (S3-compatible) for avatar storage
- gRPC (`proto/media/v1/media.proto`) as the control plane between `api-service` and `media-service`
- Swagger/OpenAPI via `swaggo/swag`, served at `/swagger` when `SWAGGER_ENABLED=true`
- React 19 + Vite + TypeScript frontend
- Docker Compose for local dev; Kubernetes (Kustomize: `base` + `minikube`/`production` overlays) for deployment
- GitHub Actions: `ci.yml` tests every PR, `deploy.yml` builds, pushes, and deploys to a shared k3s node on every push to `main`
- `testcontainers-go` for integration tests (spins up real Postgres/NATS/Redis containers; needs Docker)

## Key engineering decisions

- **Clean + Hexagonal + CQRS, package-by-bounded-context.** Each of the
  8 contexts (`auth`, `user`, `conversation`, `message`, `pushtoken`,
  `notification`, `e2ee`, `call`) is its own
  `internal/<context>/{domain,application/{command,query,port/{in,out}},adapter/{in,out}}`
  tree. Cross-cutting infrastructure (the fx composition root, config,
  generic transport and infra adapters) lives outside all of them, in
  `internal/platform/` (see that directory's own `README.md`).
- **Consumer-owned ports.** An interface lives in the package that calls
  it, not the package that implements it. A context that needs another
  context's data goes through that other context's own narrow port, not
  its internals.
- **E2EE call keys via device vouching, not a trusted key server.** A
  device becomes trusted when an already-trusted device of the same
  account signs its registration; call session keys are wrapped once per
  trusted recipient device.
- **Idempotent message sends.** `SendMessageCommand` takes a caller-
  supplied idempotency key, so a retried request from a flaky client
  connection can't double-post.
- **Keyset (cursor) pagination everywhere a list exists** (conversations,
  messages, members, notifications, public channels), not offset
  pagination, so a page stays correct under concurrent writes.
- **Shared-node resource governance split by who may change it.**
  Per-container `resources.requests/limits` live in
  `deploy/kubernetes/base/` and deploy through this project's own CI.
  The `ResourceQuota`/`LimitRange` that bounds the whole namespace from
  outside lives in `cluster/quotas/`, applied by hand under the cluster
  admin's kubeconfig, specifically so this project's own pipeline can
  never edit the ceiling that constrains it.

## Project structure

- `cmd/`: binaries `api`, `media`, `migrate`, `genkey`, `rebuildconversationinbox`.
- `internal/<context>/`: the 8 bounded contexts, each with its own domain/application/adapter layers.
- `internal/platform/`: composition root (`fx/`), `config/`, `database/` bootstrap, and shared/generic `adapter/` and `infrastructure/` code.
- `db/migrations/`: `golang-migrate` SQL migrations.
- `proto/`: the `RoomControl` gRPC contract between `api-service` and `media-service`.
- `frontend/`: the React + Vite + TypeScript client.
- `deploy/`: Docker Compose stacks (root, and `deploy/media/` for the full call stack) and `deploy/kubernetes/` (Kustomize manifests).
- `cluster/`: cluster-wide k8s config (quotas, cert-manager), applied by hand, never by any single project's CI.
- `docs/`: design specs and plans (`docs/superpowers/`) and `docs/deploy-guide.md`, the production deploy runbook.

## How to run

`cmd/api` connects to Postgres, Redis, NATS, and MinIO at startup and
fails to boot if any one of them is unreachable (each is checked eagerly,
not lazily on first use). There is no reduced "Postgres only" mode.
Bring up all four first:

```bash
cp .env.example .env
# fill in at least JWT_ED25519_SEED_BASE64 (go run ./cmd/genkey)
docker compose up -d psql_bp                    # Postgres only; see note below
docker run -d --name pm-redis --network host redis:7 --port 6379
docker run -d --name pm-nats  --network host nats:2.10 -js
docker run -d --name pm-minio --network host -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin minio/minio server /data
go run ./cmd/migrate -direction up
make run   # go run cmd/api/main.go, plus the frontend dev server on :5173
```

`make docker-run` (the root `docker-compose.yml`'s full `app` + `frontend`
+ `psql_bp` stack) predates NATS/Redis/MinIO/calls being added to this
project and does not pass those through to the `app` container; use it
only for `psql_bp`, not for a working containerized `app`. See
Limitations below.

### Full stack, including calls

```bash
export JWT_ED25519_SEED_BASE64="$(go run ./cmd/genkey)"
docker compose -f deploy/media/docker-compose.yml up --build
cd frontend && npm install && npm run dev
```

`deploy/media/docker-compose.yml` does not run MinIO either, so avatar
upload/serve will fail there the same way; everything else (chat,
channels, calls) works. See `deploy/media/README.md` for the manual
two-tab call verification procedure (including forcing the TURN relay
path), and `deploy/kubernetes/README.md` for the Minikube path, which
runs the same manifests the production overlay does and does include
MinIO.

### Other Makefile targets

`make build` (binary only), `make test` (full suite), `make itest`
(just the Postgres integration suite), `make watch` (live reload via
`air`), `make clean`.

## Environment variables and secrets

See `.env.example` for the full list with inline comments on which are
required, which have defaults, and where to get a real value. Never put
real values in that file or commit them; `.env` itself is gitignored.

Highlights:

- `JWT_ED25519_SEED_BASE64` (required): `go run ./cmd/genkey`.
- `BLUEPRINT_DB_*`: your Postgres connection.
- `PUSH_VAPID_*` / `PUSH_FCM_*` (optional): leave blank to disable web push / FCM push.
- `MEDIA_RPC_TOKEN`, `TURN_SECRET`: shared secrets between `api`, `media`, and coturn. See `docs/deploy-guide.md` for how these are generated and stored for a real deployment, not left as the repo's dev defaults.

## API documentation

The REST API is documented with Swagger (OpenAPI 2.0), generated from
`@`-annotations on each bounded context's HTTP controller
(`internal/<context>/adapter/in/web/controller.go`). Regenerate the spec
after changing a route with:

```bash
make swagger
```

The Swagger UI is gated behind `SWAGGER_ENABLED=true` (off by default)
and served at `/swagger/index.html` when enabled.

Example requests:

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "a-strong-password", "display_name": "Alice"}'

curl http://localhost:8080/conversations \
  -H "Authorization: Bearer <access_token>"
```

## Database migrations

```bash
go run ./cmd/migrate -direction up    # or -direction down
```

Reads the same `BLUEPRINT_DB_*` configuration as `cmd/api`, against
`db/migrations/`. In Kubernetes, the same binary (built from
`deploy/kubernetes/Dockerfile.migrate`) runs as a one-shot `migrate` Job
on every deploy; its pod template is immutable, so a rerun is a delete
and reapply, not a patch (`deploy.yml` already does this).

## Tests

```bash
make test    # go test ./... -v
make itest   # just internal/platform/database and internal/platform/infrastructure/postgres
```

Most of the suite is unit tests; the persistence and messaging packages
use `testcontainers-go`, so Docker needs to be running for the full
`make test`. `internal/platform/adapter/sfu` has a handful of real
WebRTC end-to-end tests (`TestSFUEndToEnd`, `TestSFUViewerCannotPublish`,
`TestSFUMuteRequiresModerator`, `TestSessionReconnectPreservesRoomAndOwnership`)
that currently fail reliably, not just occasionally: see Limitations.

## Limitations

- **The SFU's real-WebRTC end-to-end tests fail reliably** (`TestSFUEndToEnd`,
  `TestSFUViewerCannotPublish`, `TestSFUMuteRequiresModerator`,
  `TestSessionReconnectPreservesRoomAndOwnership`): a subscriber never
  receives the RTP it's supposed to be forwarded. Two real bugs in
  `internal/platform/adapter/sfu/{room,forward}.go`'s subscribe-to-an-
  already-publishing-peer path are fixed (confirmed via direct SDP
  inspection: `subscribe()` was reusing a subscriber's own upload
  transceiver for a forwarded track via `AddTrack`'s transceiver-reuse
  behavior, and subscribing too early, before the subscriber's own first
  negotiation completed, hit the same collision a different way). After
  both fixes the SDP negotiation is clean and `OnTrack` fires correctly
  on the subscriber, but the RTP payload still never reaches `ReadRTP`,
  for a third, deeper reason not yet root-caused. Pion's own ICE/DTLS/PC
  trace logging shows no error at all during this. The lower-level
  `TestRoomForwardsRTPBetweenPionPeers` (forwarding without the full
  signaling/renegotiation path) still passes, so the forwarding
  mechanism itself works; whatever's wrong is specific to subscribing a
  peer that is simultaneously publishing its own track, through the
  actor's full offer/answer path.
- `make docker-run` / the root `docker-compose.yml`'s `app` service does
  not boot as-is: it's missing `JWT_ED25519_SEED_BASE64` and has no
  NATS/Redis/MinIO services for it to reach even if that were fixed.
  Use it only to bring up `psql_bp`; use `deploy/media/docker-compose.yml`
  for an actually working containerized full stack.
- No staging environment; a merge to `main` deploys straight to
  production (see `docs/deploy-guide.md`).
- No autoscaling.
- `cmd/rebuildconversationinbox` is an operator-triggered one-off
  (`kubectl run`/ad hoc `Job`), not a standing scheduled workload.
- The production k3s node is shared with one other project; that
  project's own `ResourceQuota` (`cluster/quotas/other-project.yaml`) is
  still a placeholder pending its administrator's real numbers.
- Traefik's entrypoint idle timeout in front of the production Ingress
  has not been confirmed long enough for a quiet call's signaling
  WebSocket; see `docs/deploy-guide.md`'s post-deploy verification step.
- No frontend test suite; CI only lints (`oxlint`) and builds it.

## License

No `LICENSE` file or package-manifest `license` field exists in this
repository yet. Until one is added, all rights are reserved by default.
