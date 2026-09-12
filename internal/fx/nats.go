// internal/fx/nats.go
package fxapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"

	wsadapter "pet-messenger/internal/adapter/ws"
	"pet-messenger/internal/config"
	natsinfra "pet-messenger/internal/infrastructure/nats"
)

func newNATSConn(lc fx.Lifecycle, cfg *config.Config) (*natsgo.Conn, error) {
	log.Printf("nats: connecting to %s", cfg.NATS.URL)
	nc, err := natsgo.Connect(cfg.NATS.URL,
		natsgo.Name("pet-messenger-api"),
		natsgo.RetryOnFailedConnect(true),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(2*time.Second),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			log.Printf("nats: disconnected: %v", err)
		}),
		natsgo.ReconnectHandler(func(nc *natsgo.Conn) {
			log.Printf("nats: reconnected to %s", nc.ConnectedUrl())
		}),
		natsgo.ClosedHandler(func(_ *natsgo.Conn) {
			log.Printf("nats: connection closed")
		}),
		natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			log.Printf("nats: async error: %v", err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect %s: %w", cfg.NATS.URL, err)
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error { return nc.Drain() },
	})
	return nc, nil
}

func newJetStream(nc *natsgo.Conn) (natsgo.JetStreamContext, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("nats jetstream context: %w", err)
	}
	return js, nil
}

const notifStreamName = "NOTIF"

// ensureNotifStream creates (or updates, idempotently) the NOTIF work-queue
// stream. It runs immediately before the worker subscription so Fx module
// ordering cannot leave the consumer racing a first-time stream creation.
func ensureNotifStream(ctx context.Context, js natsgo.JetStreamContext) error {
	cfg := &natsgo.StreamConfig{
		Name:      notifStreamName,
		Subjects:  []string{"notif.msg.*", "notif.member.*", "notif.call.*"},
		Retention: natsgo.WorkQueuePolicy,
		Storage:   natsgo.FileStorage,
		MaxAge:    24 * time.Hour,
		Discard:   natsgo.DiscardOld,
	}
	if _, err := js.AddStream(cfg); err != nil {
		if _, uerr := js.UpdateStream(cfg); uerr != nil {
			return fmt.Errorf("ensure NOTIF stream: add=%v update=%v", err, uerr)
		}
		log.Printf("nats: NOTIF stream updated")
		return nil
	}
	log.Printf("nats: NOTIF stream ready")
	return nil
}

// startMessageSubscription opens the single per-pod msg.> subscription and feeds
// every event's raw bytes to the Hub, keyed by the conversation id in the
// subject.
func startMessageSubscription(lc fx.Lifecycle, nc *natsgo.Conn, hub *wsadapter.Hub) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := nc.Subscribe(natsinfra.WildcardSubject, func(m *natsgo.Msg) {
				if cid, ok := natsinfra.ConversationIDFromSubject(m.Subject); ok {
					hub.Broadcast(cid, m.Data)
				}
			})
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", natsinfra.WildcardSubject, err)
			}
			sub = s
			log.Printf("nats: subscribed to %s", natsinfra.WildcardSubject)
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

// startMemberRevocationSubscription opens the per-pod member.revoked.>
// subscription and drops the named user's local subscription to the named
// conversation from the Hub.
func startMemberRevocationSubscription(lc fx.Lifecycle, nc *natsgo.Conn, hub *wsadapter.Hub) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := nc.Subscribe(natsinfra.RevokeWildcardSubject, func(m *natsgo.Msg) {
				cid, ok := natsinfra.ConversationIDFromRevokeSubject(m.Subject)
				if !ok {
					log.Printf("nats: member-revoked on unparseable subject %q", m.Subject)
					return
				}
				var ev natsinfra.MemberRevokedEvent
				if err := json.Unmarshal(m.Data, &ev); err != nil {
					log.Printf("nats: bad member-revoked payload on %s: %v", m.Subject, err)
					return
				}
				uid, err := uuid.Parse(ev.UserID)
				if err != nil {
					log.Printf("nats: member-revoked with bad user id %q", ev.UserID)
					return
				}
				hub.DropSubscription(uid, cid)
			})
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", natsinfra.RevokeWildcardSubject, err)
			}
			sub = s
			log.Printf("nats: subscribed to %s", natsinfra.RevokeWildcardSubject)
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

var NATSModule = fx.Module("nats",
	fx.Provide(
		newNATSConn,
		newJetStream,
		natsinfra.NewPublisher,
		natsinfra.NewMemberEventPublisher,
	),
	fx.Invoke(startMessageSubscription),
	fx.Invoke(startMemberRevocationSubscription),
)
