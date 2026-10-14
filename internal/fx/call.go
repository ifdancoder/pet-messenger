package fxapp

import (
	"context"
	"fmt"
	"log"

	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"

	callevents "pet-messenger/internal/adapter/callevents"
	httpadapter "pet-messenger/internal/adapter/http"
	rpcadapter "pet-messenger/internal/adapter/rpc"
	"pet-messenger/internal/config"
	infraauth "pet-messenger/internal/infrastructure/auth"
	iceinfra "pet-messenger/internal/infrastructure/ice"
	medianodeinfra "pet-messenger/internal/infrastructure/medianode"
	notifinfra "pet-messenger/internal/infrastructure/notification"
	"pet-messenger/internal/infrastructure/postgres"
	usecasecall "pet-messenger/internal/usecase/call"
)

var CallModule = fx.Module("call",
	fx.Provide(
		postgres.NewCallRepository,
		usecasecall.NewService,
		httpadapter.NewCallHandler,
		callevents.NewConsumer,
		// RoomControl (gRPC client)
		func(cfg *config.Config) usecasecall.RoomControl { return rpcadapter.NewRoomControlClient(cfg) },
		// MediaNodeRegistry (Redis, read side)
		func(r *medianodeinfra.RedisRegistry) usecasecall.MediaNodeRegistry { return r },
		medianodeinfra.NewRedisRegistry,
		// JoinTicketIssuer
		func(i *infraauth.JoinTicketIssuer) usecasecall.JoinTicketIssuer { return i },
		infraauth.NewJoinTicketIssuer,
		// ICE
		func(p *iceinfra.Provider) usecasecall.ICEConfigProvider { return p },
		iceinfra.NewProvider,
		// CallNotifier
		func(n *notifinfra.CallNotifier) usecasecall.CallNotifier { return n },
		notifinfra.NewCallNotifier,
	),
	fx.Invoke(startCallEventSubscription, callevents.StartReaper),
)

func startCallEventSubscription(lc fx.Lifecycle, nc *natsgo.Conn, cons *callevents.Consumer) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := nc.Subscribe(callevents.Wildcard, cons.HandleMsg)
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", callevents.Wildcard, err)
			}
			sub = s
			log.Printf("nats: subscribed to %s", callevents.Wildcard)
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
