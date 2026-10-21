# internal/platform/infrastructure

Integrations with external systems and resources (databases, caches,
object storage, message brokers, third-party push providers) that sit
outside any single bounded context's own `internal/<context>/adapter/`
tree. A package belongs here, not inside a context, when it wraps a raw
client/connection that's genuinely shared across contexts, or satisfies
one context's own narrow port from outside that context's package because
the integration is generic enough to be reused.

- `auth/` — JWT issuing/verification (`JWTVerifier`, used by the shared
  `adapter/http.AuthMiddleware`) and signed join-ticket minting. Satisfies
  `call`'s own `JoinTicketIssuer` port.
- `ice/` — builds the ICE server list handed to a client joining a call.
  Satisfies `call`'s own `ICEConfigProvider` port.
- `medianode/` — the Redis-backed registry of live media-service pods and
  their load. Satisfies `call`'s own `MediaNodeRegistry` port.
- `nats/` — the NATS event schema and the core-NATS publisher, shared by
  several contexts' own messaging adapters.
- `objectstore/` — MinIO (S3-compatible) client construction for file
  attachments.
- `postgres/` — shared `*sql.DB` construction and the migration-runner
  bootstrap used by every context's own persistence-adapter test files.
- `presence/` — Redis-backed tracking of which users currently hold a
  live WebSocket connection, used by `notification`.
- `push/` — delivers one notification to a single registered device
  across APNs, FCM, and WebPush.
- `redis/` — shared go-redis client construction.

Context-owned persistence adapters (the Postgres repository implementing
one context's own `CallWriter`/`MessageRepository`/etc. ports) live inside
that context's own `internal/<context>/adapter/out/persistence/` tree
instead, following the Clean + Hexagonal retrofit's package-by-context
structure. This directory holds the generic client construction those
adapters are built on top of, not the adapters themselves.
