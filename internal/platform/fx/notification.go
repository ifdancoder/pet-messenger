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

	wsadapter "pet-messenger/internal/platform/adapter/ws"
	callout "pet-messenger/internal/call/application/port/out"
	"pet-messenger/internal/platform/config"
	convpersistence "pet-messenger/internal/conversation/adapter/out/persistence"
	presenceinfra "pet-messenger/internal/platform/infrastructure/presence"
	"pet-messenger/internal/platform/infrastructure/push"
	messageout "pet-messenger/internal/message/application/port/out"
	notificationworker "pet-messenger/internal/notification/adapter/in/messaging"
	notificationweb "pet-messenger/internal/notification/adapter/in/web"
	notificationmessaging "pet-messenger/internal/notification/adapter/out/messaging"
	notificationpersistence "pet-messenger/internal/notification/adapter/out/persistence"
	notificationcommand "pet-messenger/internal/notification/application/command"
	notificationout "pet-messenger/internal/notification/application/port/out"
	notificationquery "pet-messenger/internal/notification/application/query"
	pushtokenweb "pet-messenger/internal/pushtoken/adapter/in/web"
	pushtokenpersistence "pet-messenger/internal/pushtoken/adapter/out/persistence"
	pushtokencommand "pet-messenger/internal/pushtoken/application/command"
	pushtokenout "pet-messenger/internal/pushtoken/application/port/out"
)

var NotificationModule = fx.Module("notification",
	fx.Provide(
		// Secondary adapters.
		notificationpersistence.NewNotificationRepository,
		notificationmessaging.NewProducer,

		// Bridges from the concrete persistence adapter to this context's
		// own out ports.
		func(r *notificationpersistence.NotificationRepository) notificationout.NotificationWriter { return r },
		func(r *notificationpersistence.NotificationRepository) notificationout.NotificationReader { return r },
		func(r *notificationpersistence.NotificationRepository) notificationout.NotificationMarker { return r },

		// Bridges from the concrete producer to this context's own
		// WSNotifier port and to the OTHER bounded contexts' own
		// consumer-owned ports (message's NotificationProducer, call's
		// CallNotifier); relocated here from internal/fx/call.go, because
		// this context is the producer of the concrete type.
		func(p *notificationmessaging.Producer) notificationout.WSNotifier { return p },
		func(p *notificationmessaging.Producer) messageout.NotificationProducer { return p },
		func(p *notificationmessaging.Producer) callout.CallNotifier { return p },

		// Bridges to this context's own remaining consumer-owned ports.
		func(r *convpersistence.ConversationRepository) notificationout.MemberLister { return r },
		func(r *pushtokenpersistence.PushTokenRepository) notificationout.PushTokenLister { return r },
		func(p *presenceinfra.RedisPresence) notificationout.Presence { return p },
		provideNotificationPusher,

		// CQRS write side.
		notificationcommand.NewMarkReadThroughHandler,
		notificationcommand.NewProcessNotificationIntentHandler,

		// CQRS read side.
		notificationquery.NewListNotificationsHandler,
		notificationquery.NewUnreadCountHandler,

		// Primary adapters.
		notificationweb.NewNotificationHandler,
		notificationworker.NewWorker,
	),
	fx.Invoke(startNotifWorker),
	fx.Invoke(startNotifWSSubscription),
)

var PushModule = fx.Module("push",
	fx.Provide(
		pushtokenpersistence.NewPushTokenRepository,

		// Bridges from the concrete persistence adapter to this context's
		// own out ports.
		func(r *pushtokenpersistence.PushTokenRepository) pushtokenout.PushTokenWriter { return r },
		func(r *pushtokenpersistence.PushTokenRepository) pushtokenout.PushTokenEraser { return r },

		pushtokencommand.NewRegisterPushTokenHandler,
		pushtokencommand.NewUnregisterPushTokenHandler,

		pushtokenweb.NewPushTokenHandler,
	),
)

// provideNotificationPusher builds the per-platform device pusher from
// config, as notificationout.Pusher. A misconfigured provider (e.g. bad FCM
// credentials) is a hard boot failure; returning the error lets fx fail
// fast rather than start with a silently degraded push subsystem.
func provideNotificationPusher(cfg *config.Config) (notificationout.Pusher, error) {
	return push.NewMultiPusher(context.Background(), cfg.Push)
}

// startNotifWorker opens the durable queue-group subscription that drives the
// notification delivery worker. One instance runs per api pod; JetStream
// load-balances "notif.>" across the "notif-workers" group, redelivering an
// unacked message up to MaxDeliver times.
func startNotifWorker(lc fx.Lifecycle, js natsgo.JetStreamContext, w *notificationworker.Worker) {
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
