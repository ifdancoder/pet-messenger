// internal/fx/message.go
package fxapp

import (
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/infrastructure/postgres"
	usecasemsg "pet-messenger/internal/usecase/message"
)

var MessageModule = fx.Module("message",
	fx.Provide(
		postgres.NewMessageRepository,
		usecasemsg.NewService,
		httpadapter.NewMessageHandler,
	),
)
