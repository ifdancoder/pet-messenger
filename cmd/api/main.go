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
		fxapp.UserModule,
		fxapp.ConversationModule,
		fxapp.MessageModule,
		fxapp.NotificationModule,
		fxapp.PushModule,
		fxapp.CallModule,
		fxapp.NATSModule,
		fxapp.RedisModule,
		fxapp.ObjectStoreModule,
		fxapp.WSModule,
		fxapp.HTTPModule,
	).Run()
}
