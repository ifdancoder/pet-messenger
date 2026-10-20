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
	domaincall "pet-messenger/internal/domain/call"
	e2eeout "pet-messenger/internal/e2ee/application/port/out"
	infraauth "pet-messenger/internal/infrastructure/auth"
	iceinfra "pet-messenger/internal/infrastructure/ice"
	medianodeinfra "pet-messenger/internal/infrastructure/medianode"
	"pet-messenger/internal/infrastructure/postgres"
	usecasecall "pet-messenger/internal/usecase/call"
)

var CallModule = fx.Module("call",
	fx.Provide(
		postgres.NewCallRepository,
		usecasecall.NewService,
		httpadapter.NewCallHandler,
		callevents.NewConsumer,
		// CallLoader is e2ee's own narrow, consumer-defined read port onto
		// this context's call repository.
		func(r domaincall.Repository) e2eeout.CallLoader { return r },
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
		// CallNotifier is provided by NotificationModule: the notification
		// context owns the concrete producer that satisfies this port, so
		// its bridge lives in that producer's own fx file.
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
