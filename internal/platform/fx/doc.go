// Package fxapp is the application's Composition Root: it wires concrete
// adapters to the ports each bounded context declares, and nothing else.
// It is the only package allowed to import concrete types from more than
// one bounded context at once (every context itself depends only inward,
// on its own domain/application/port layers and other contexts' narrow,
// consumer-owned ports).
//
// One file per fx.Module: one file per bounded context (auth.go, call.go,
// conversation.go, e2ee.go, message.go, notification.go, user.go) plus one
// file per cross-cutting infrastructure concern shared by several contexts
// (config.go, database.go, http.go, media.go, nats.go, objectstore.go,
// redis.go, ws.go). A cross-context fx bridge (a function satisfying one
// context's out port with another context's concrete adapter) always
// lives in the PRODUCER's own file, never the consumer's.
package fxapp
