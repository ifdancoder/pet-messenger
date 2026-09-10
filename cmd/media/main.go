package main

import (
	"go.uber.org/fx"

	fxapp "pet-messenger/internal/fx"
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
	).Run()
}
