// cmd/api/main.go
package main

import (
	"go.uber.org/fx"

	fxapp "pet-messenger/internal/fx"
)

func main() {
	fx.New(
		fxapp.ConfigModule,
		fxapp.DatabaseModule,
		fxapp.AuthModule,
		fxapp.ConversationModule,
		fxapp.MessageModule,
		fxapp.NATSModule,
		fxapp.RedisModule,
		fxapp.WSModule,
		fxapp.HTTPModule,
	).Run()
}
