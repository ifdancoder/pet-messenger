package main

import (
	"go.uber.org/fx"

	fxapp "pet-messenger/internal/platform/fx"
)

func main() {
	fx.New(
		fxapp.ConfigModule,
		fxapp.RedisModule,
		fxapp.MediaNATSModule,
		fxapp.JoinTicketModule,
		fxapp.SFUModule,
		fxapp.RPCServerModule,
		fxapp.MediaNodeModule,
		fxapp.SignalingModule,
	).Run()
}
