// cmd/api/main.go
package main

import (
	"go.uber.org/fx"

	fxapp "pet-messenger/internal/platform/fx"
)

// @title						pet-messenger API
// @version					1.0
// @description				REST API for the pet-messenger backend: auth, conversations, messages, calls, push notifications, and end-to-end encryption key exchange.
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Access token issued by POST /auth/login or /auth/register, sent as "Bearer <token>".
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
		fxapp.E2EEModule,
		fxapp.NATSModule,
		fxapp.RedisModule,
		fxapp.ObjectStoreModule,
		fxapp.WSModule,
		fxapp.HTTPModule,
	).Run()
}
