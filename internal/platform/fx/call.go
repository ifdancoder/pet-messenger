package fxapp

import (
	"context"
	"fmt"
	"log"

	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"

	rpcadapter "pet-messenger/internal/platform/adapter/rpc"
	callmessaging "pet-messenger/internal/call/adapter/in/messaging"
	callweb "pet-messenger/internal/call/adapter/in/web"
	callpersistence "pet-messenger/internal/call/adapter/out/persistence"
	callcommand "pet-messenger/internal/call/application/command"
	callout "pet-messenger/internal/call/application/port/out"
	callquery "pet-messenger/internal/call/application/query"
	"pet-messenger/internal/platform/config"
	e2eeout "pet-messenger/internal/e2ee/application/port/out"
	infraauth "pet-messenger/internal/platform/infrastructure/auth"
	iceinfra "pet-messenger/internal/platform/infrastructure/ice"
	medianodeinfra "pet-messenger/internal/platform/infrastructure/medianode"
)

var CallModule = fx.Module("call",
	fx.Provide(
		// Secondary adapter -- provided as its concrete type, because it
		// satisfies four out ports at once (the call/participant read and
		// write ports split in this slice's design spec), plus e2ee's own
		// CallLoader port below.
		callpersistence.NewCallRepository,

		func(r *callpersistence.CallRepository) callout.CallWriter { return r },
		func(r *callpersistence.CallRepository) callout.CallLoader { return r },
		func(r *callpersistence.CallRepository) callout.ParticipantWriter { return r },
		func(r *callpersistence.CallRepository) callout.ParticipantLister { return r },

		// CQRS write side -- one provider per command.
		callcommand.NewStartCallHandler,
		callcommand.NewJoinCallHandler,
		callcommand.NewEndCallHandler,
		callcommand.NewMuteParticipantHandler,
		callcommand.NewUnmuteParticipantHandler,
		callcommand.NewRemoveParticipantHandler,
		callcommand.NewHandleParticipantLeftHandler,
		callcommand.NewHandleCallEndedHandler,
		callcommand.NewReapStaleCallsHandler,

		// CQRS read side.
		callquery.NewGetLiveForConversationHandler,

		// Primary adapters.
		callweb.NewCallHandler,
		callmessaging.NewConsumer,

		// RoomControl (gRPC client)
		func(cfg *config.Config) callout.RoomControl { return rpcadapter.NewRoomControlClient(cfg) },
		// MediaNodeRegistry (Redis, read side)
		func(r *medianodeinfra.RedisRegistry) callout.MediaNodeRegistry { return r },
		medianodeinfra.NewRedisRegistry,
		// JoinTicketIssuer
		func(i *infraauth.JoinTicketIssuer) callout.JoinTicketIssuer { return i },
		infraauth.NewJoinTicketIssuer,
		// ICE
		func(p *iceinfra.Provider) callout.ICEConfigProvider { return p },
		iceinfra.NewProvider,

		// e2ee's own CallLoader bridge: this context IS the producer for
		// it, so the bridge lives here, now retargeted from the old wide
		// domaincall.Repository interface to this slice's own new
		// concrete adapter, closing out the forward-note left after
		// e2ee's own retrofit.
		func(r *callpersistence.CallRepository) e2eeout.CallLoader { return r },

		// ConversationReader is provided by ConversationModule: that
		// context owns the concrete persistence adapter that satisfies
		// this port, so its bridge lives in that producer's own fx file.
		// SystemEventPoster is provided by MessageModule, CallNotifier by
		// NotificationModule, for the same reason.
	),
	fx.Invoke(startCallEventSubscription, callmessaging.StartReaper),
)

func startCallEventSubscription(lc fx.Lifecycle, nc *natsgo.Conn, cons *callmessaging.Consumer) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := nc.Subscribe(callmessaging.Wildcard, cons.HandleMsg)
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", callmessaging.Wildcard, err)
			}
			sub = s
			log.Printf("nats: subscribed to %s", callmessaging.Wildcard)
			return nil
		},
		OnStop: func(context.Context) error {
			if sub != nil {
				return sub.Unsubscribe()
			}
			return nil
		},
	})
}
