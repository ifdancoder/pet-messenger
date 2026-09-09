// internal/fx/ws.go
package fxapp

import (
	"go.uber.org/fx"

	wsadapter "pet-messenger/internal/adapter/ws"
	"pet-messenger/internal/config"
	"pet-messenger/internal/domain/conversation"
	infraauth "pet-messenger/internal/infrastructure/auth"
	presenceinfra "pet-messenger/internal/infrastructure/presence"
)

// newWSHandler threads the configured CORS origins into the WS handler so the
// /ws upgrade accepts the same browser origins the gin CORS block does.
func newWSHandler(hub *wsadapter.Hub, verifier *infraauth.JWTVerifier, members conversation.Repository, cfg *config.Config, presence wsadapter.PresenceRecorder) *wsadapter.WSHandler {
	return wsadapter.NewWSHandler(hub, verifier, members, cfg.CORS.OriginHostPatterns(), presence)
}

var WSModule = fx.Module("ws",
	fx.Provide(
		wsadapter.NewHub,
		newWSHandler,
		func(p *presenceinfra.RedisPresence) wsadapter.PresenceRecorder { return p },
	),
)
