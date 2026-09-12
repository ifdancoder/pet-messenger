// internal/fx/notification.go
package fxapp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	notify "pet-messenger/internal/adapter/notify"
	wsadapter "pet-messenger/internal/adapter/ws"
	"pet-messenger/internal/config"
	notifinfra "pet-messenger/internal/infrastructure/notification"
	"pet-messenger/internal/infrastructure/postgres"
	presenceinfra "pet-messenger/internal/infrastructure/presence"
	"pet-messenger/internal/infrastructure/push"
	usecasenotif "pet-messenger/internal/usecase/notification"
)

var NotificationModule = fx.Module("notification",
	fx.Provide(
		postgres.NewNotificationRepository,
		usecasenotif.NewService,
		httpadapter.NewNotificationHandler,
		notifinfra.NewProducer,
		notifinfra.NewWSNotifier,
		notify.NewWorker,
		func(w *notifinfra.WSNotifier) notify.WSNotifier { return w },
		func(p *presenceinfra.RedisPresence) notify.Presence { return p },
	),
	fx.Invoke(startNotifWorker),
	fx.Invoke(startNotifWSSubscription),
)

var PushModule = fx.Module("push",
	fx.Provide(
		postgres.NewPushTokenRepository,
		httpadapter.NewPushTokenHandler,
		provideMultiPusher,
	),
)

// provideMultiPusher builds the per-platform device pusher from config. A
// misconfigured provider (e.g. bad FCM credentials) is a hard boot failure —
// returning the error lets fx fail fast rather than start with a silently
// degraded push subsystem.
func provideMultiPusher(cfg *config.Config) (push.Pusher, error) {
	return push.NewMultiPusher(context.Background(), cfg.Push)
}

// startNotifWorker opens the durable queue-group subscription that drives the
// notification delivery worker. One instance runs per api pod; JetStream
// load-balances "notif.>" across the "notif-workers" group, redelivering an
// unacked message up to MaxDeliver times.
func startNotifWorker(lc fx.Lifecycle, js natsgo.JetStreamContext, w *notify.Worker) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := ensureNotifStream(ctx, js); err != nil {
				return err
			}
			s, err := js.QueueSubscribe("notif.>", "notif-workers", w.HandleMsg,
				natsgo.Durable("notif-worker"),
				natsgo.ManualAck(),
				natsgo.AckWait(30*time.Second),
				natsgo.MaxDeliver(5),
				natsgo.AckExplicit(),
			)
			if err != nil {
				return fmt.Errorf("subscribe notif.>: %w", err)
			}
			sub = s
			log.Printf("nats: notification worker subscribed to notif.> (group notif-workers)")
			return nil
		},
		OnStop: func(context.Context) error {
			if sub != nil {
				return sub.Drain()
			}
			return nil
		},
	})
}

// startNotifWSSubscription opens the per-pod core-NATS subscription that turns a
// notif.ws.<uid> publish (from the worker's WSNotifier, anywhere in the fleet)
// into a Hub.SendToUser on this pod.
func startNotifWSSubscription(lc fx.Lifecycle, nc *natsgo.Conn, hub *wsadapter.Hub) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := nc.Subscribe("notif.ws.*", func(m *natsgo.Msg) {
				uid, err := uuid.Parse(strings.TrimPrefix(m.Subject, "notif.ws."))
				if err != nil {
					log.Printf("nats: notif.ws on unparseable subject %q", m.Subject)
					return
				}
				hub.SendToUser(uid, m.Data)
			})
			if err != nil {
				return fmt.Errorf("subscribe notif.ws.*: %w", err)
			}
			sub = s
			log.Printf("nats: subscribed to notif.ws.*")
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
