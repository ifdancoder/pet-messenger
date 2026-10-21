# internal/platform

Everything that is not a bounded context: the Composition Root and the
cross-cutting, outer-ring concerns every context's own `domain`/
`application`/`adapter` layers depend on but none of them owns.

- `fx/` — the Composition Root (`package fxapp`). Wires concrete adapters
  to the ports each bounded context declares.
- `config/` — process configuration (environment variables, flags) loaded
  once at startup and handed to the fx modules that need it.
- `database/` — the `*sql.DB` bootstrap and migration runner shared by
  every context's own persistence adapters.
- `adapter/` — transport-protocol adapters shared across contexts or
  generic enough to satisfy one context's port from outside that context's
  own package. See `adapter/README.md`.
- `infrastructure/` — integrations with external systems (Postgres, Redis,
  NATS, object storage, push providers) shared across contexts or generic
  enough to satisfy one context's port from outside that context's own
  package. See `infrastructure/README.md`.

The bounded contexts themselves (`internal/auth`, `internal/call`,
`internal/conversation`, `internal/e2ee`, `internal/message`,
`internal/notification`, `internal/pushtoken`, `internal/user`) sit one
level up, as siblings of this directory, not inside it: each is a
self-contained package tree with its own `domain/application/adapter`
layering, and none of them depends on another context's internals, only on
narrow ports and on what lives here.
