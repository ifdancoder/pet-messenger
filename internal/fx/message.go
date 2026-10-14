// internal/fx/message.go
package fxapp

import (
	"go.uber.org/fx"

	convout "pet-messenger/internal/conversation/application/port/out"
	messageweb "pet-messenger/internal/message/adapter/in/web"
	messagemessaging "pet-messenger/internal/message/adapter/out/messaging"
	messagepersistence "pet-messenger/internal/message/adapter/out/persistence"
	messagecommand "pet-messenger/internal/message/application/command"
	messageout "pet-messenger/internal/message/application/port/out"
	messagequery "pet-messenger/internal/message/application/query"
	usecasecall "pet-messenger/internal/usecase/call"
)

var MessageModule = fx.Module("message",
	fx.Provide(
		// Secondary adapters — the persistence adapter is provided as its
		// concrete type because it satisfies three out ports at once.
		messagepersistence.NewMessageRepository,
		messagemessaging.NewPublisher,

		// Bridges from the concrete persistence adapter to this context's
		// own out ports.
		func(r *messagepersistence.MessageRepository) messageout.MessageLoader { return r },
		func(r *messagepersistence.MessageRepository) messageout.MessageWriter { return r },
		func(r *messagepersistence.MessageRepository) messageout.MessageHistoryPort { return r },

		// CQRS write side — one provider per command.
		messagecommand.NewSendMessageHandler,
		messagecommand.NewEditMessageHandler,
		messagecommand.NewDeleteMessageHandler,
		messagecommand.NewMarkReadHandler,
		messagecommand.NewPostSystemEventHandler,

		// Bridges from the concrete PostSystemEventHandler to the OTHER
		// bounded contexts' own narrow, consumer-defined ports (see the
		// design spec's "Why message needs real cross-context seam work").
		// Relocated here from internal/fx/conversation.go and
		// internal/fx/call.go — this context is the producer of the
		// concrete type, so the bridge belongs in its own fx file, not the
		// consumers'.
		func(h *messagecommand.PostSystemEventHandler) convout.SystemEventPoster { return h },
		func(h *messagecommand.PostSystemEventHandler) usecasecall.SystemEventPoster { return h },

		// CQRS read side.
		messagequery.NewListMessagesHandler,

		// Primary adapter.
		messageweb.NewMessageHandler,
	),
)
