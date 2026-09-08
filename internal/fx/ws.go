// internal/fx/ws.go
package fxapp

import (
	"go.uber.org/fx"

	wsadapter "pet-messenger/internal/adapter/ws"
)

var WSModule = fx.Module("ws",
	fx.Provide(
		wsadapter.NewHub,
		wsadapter.NewWSHandler,
	),
)
