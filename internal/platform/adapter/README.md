# internal/platform/adapter

Transport-protocol adapters that sit outside any single bounded context's
own `internal/<context>/adapter/` tree. A package belongs here, not inside
a context, when it either speaks a protocol shared by several contexts, or
implements one context's own narrow port from outside that context's
package (because the protocol client is generic enough to be reused).

- `http/` — shared HTTP middleware (`AuthMiddleware`, `ContextUserIDKey`)
  that every bounded context's own HTTP controller depends on to read the
  authenticated user id off the request context.
- `ws/` — the WebSocket chat fan-out hub (`Hub`), shared by the
  `message`, `notification`, and `conversation` contexts to push live
  events to connected clients.
- `rpc/` — the gRPC client to the media-service's control plane
  (`RoomControlClient`). Satisfies `call`'s own `RoomControl` port.
- `sfu/` — the media-service's embedded Pion WebRTC selective forwarding
  unit. This is the core of the `cmd/media` binary itself, not a port
  implementation for an `api`-service bounded context.
- `signaling/` — the media-service's `/rtc` WebSocket signaling server
  that negotiates a client's join and pairs it with `sfu/`.

Context-specific adapters (an HTTP controller, a Postgres repository, a
NATS consumer that only that context's own commands/queries use) live
inside that context's own `internal/<context>/adapter/{in,out}/` tree
instead, following the Clean + Hexagonal retrofit's package-by-context
structure.
