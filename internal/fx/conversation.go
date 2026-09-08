// internal/fx/conversation.go
package fxapp

import (
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/infrastructure/postgres"
	usecaseconv "pet-messenger/internal/usecase/conversation"
)

var ConversationModule = fx.Module("conversation",
	fx.Provide(
		postgres.NewConversationRepository,
		usecaseconv.NewService,
		httpadapter.NewConversationHandler,
	),
)
