// internal/fx/conversation.go
package fxapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"

	wsadapter "pet-messenger/internal/platform/adapter/ws"
	callout "pet-messenger/internal/call/application/port/out"
	"pet-messenger/internal/conversation/adapter/in/web"
	"pet-messenger/internal/conversation/adapter/out/messaging"
	"pet-messenger/internal/conversation/adapter/out/persistence"
	"pet-messenger/internal/conversation/application/command"
	"pet-messenger/internal/conversation/application/port/out"
	"pet-messenger/internal/conversation/application/query"
	"pet-messenger/internal/conversation/domain"
	e2eeout "pet-messenger/internal/e2ee/application/port/out"
	natsinfra "pet-messenger/internal/platform/infrastructure/nats"
	messageout "pet-messenger/internal/message/application/port/out"
)

var ConversationModule = fx.Module("conversation",
	fx.Provide(
		// Secondary adapters — provided as their concrete types, because
		// each satisfies several out ports plus several other bounded
		// contexts' own narrow, consumer-defined interfaces at once.
		persistence.NewConversationRepository,
		persistence.NewInboxRepository,
		messaging.NewEventPublisher,
		messaging.NewInboxProjectorFromRepository,

		// Bridges from the concrete write-side adapter to this context's
		// own out ports.
		func(r *persistence.ConversationRepository) out.LoadConversationPort { return r },
		func(r *persistence.ConversationRepository) out.UpdateConversationPort { return r },
		func(r *persistence.ConversationRepository) out.PublicChannelViewPort { return r },
		func(r *persistence.ConversationRepository) out.MemberViewPort { return r },
		func(r *persistence.InboxRepository) out.InboxViewPort { return r },

		// Bridges from the concrete write-side adapter to the OTHER
		// bounded contexts' own narrow, consumer-defined read ports (see
		// the design spec's "Cross-context seams").
		func(r *persistence.ConversationRepository) messageout.ConversationMembershipPort { return r },
		func(r *persistence.ConversationRepository) messageout.ConversationRosterPort { return r },
		func(r *persistence.ConversationRepository) messageout.ReadCursorPort { return r },
		func(r *persistence.ConversationRepository) callout.ConversationReader { return r },
		// The MemberLister bridge for the notification context now lives
		// in NotificationModule (internal/fx/notification.go) as
		// notificationout.MemberLister, not here.
		func(r *persistence.ConversationRepository) wsadapter.MemberChecker { return r },
		func(r *persistence.ConversationRepository) e2eeout.ConversationMemberChecker { return r },

		// CQRS write side — one provider per command.
		command.NewOpenDirectHandler,
		command.NewCreateGroupChatHandler,
		command.NewCreateChannelHandler,
		command.NewJoinChannelHandler,
		command.NewLeaveConversationHandler,
		command.NewAddMemberHandler,
		command.NewRemoveMemberHandler,
		command.NewSetMemberTierHandler,
		command.NewClaimCreatorHandler,
		command.NewSetArchivedHandler,

		// CQRS read side — one provider per query.
		query.NewListMineHandler,
		query.NewListPublicChannelsHandler,
		query.NewListMembersHandler,

		// Primary adapter.
		web.NewConversationHandler,
	),
	fx.Invoke(startInboxProjector, startMemberRevocationFanout),
)

// startInboxProjector ensures the CONVERSATION_EVENTS stream exists, then
// opens two subscriptions feeding the one InboxProjector: a JetStream
// queue-group durable consumer on conversation.events.> (this context's
// own lifecycle events — durable, because losing one causes permanent
// read-model drift) and a plain core-NATS subscription on msg.> (the
// message domain's existing best-effort fan-out, reused read-only to keep
// last_message_at current).
func startInboxProjector(lc fx.Lifecycle, nc *natsgo.Conn, js natsgo.JetStreamContext, projector *messaging.InboxProjector) {
	var lifecycleSub, messageSub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if err := messaging.EnsureStream(js); err != nil {
				return fmt.Errorf("ensure CONVERSATION_EVENTS stream: %w", err)
			}
			ls, err := js.QueueSubscribe(messaging.WildcardSubject, "conversation-inbox-projectors", projector.HandleLifecycleEvent,
				natsgo.Durable("conversation-inbox-projector"),
				natsgo.ManualAck(),
				natsgo.AckWait(30*time.Second),
				natsgo.MaxDeliver(5),
				natsgo.AckExplicit(),
			)
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", messaging.WildcardSubject, err)
			}
			lifecycleSub = ls

			ms, err := nc.Subscribe(natsinfra.WildcardSubject, projector.HandleMessageEvent)
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", natsinfra.WildcardSubject, err)
			}
			messageSub = ms

			log.Printf("nats: inbox projector subscribed to %s (queue group) and %s", messaging.WildcardSubject, natsinfra.WildcardSubject)
			return nil
		},
		OnStop: func(context.Context) error {
			if lifecycleSub != nil {
				_ = lifecycleSub.Drain()
			}
			if messageSub != nil {
				return messageSub.Unsubscribe()
			}
			return nil
		},
	})
}

// startMemberRevocationFanout opens a per-pod, non-durable JetStream
// subscription to conversation.events.> and drops a removed member's live
// WebSocket subscription to that conversation on this pod. Deliberately
// NOT a queue-group consumer (unlike startInboxProjector's lifecycle
// subscription): every pod must see every member.removed event to drop
// its own local socket, so this uses an ephemeral per-connection consumer
// instead — the direct successor to the pre-retrofit
// member.revoked.> core-NATS fan-out (see the design spec's "Migration of
// existing wiring").
func startMemberRevocationFanout(lc fx.Lifecycle, js natsgo.JetStreamContext, hub *wsadapter.Hub) {
	var sub *natsgo.Subscription
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			s, err := js.Subscribe(messaging.WildcardSubject, func(m *natsgo.Msg) {
				var env struct {
					Type    string          `json:"type"`
					Payload json.RawMessage `json:"payload"`
				}
				if err := json.Unmarshal(m.Data, &env); err != nil {
					log.Printf("nats: ws revocation fanout: bad envelope: %v", err)
					_ = m.Ack()
					return
				}
				if env.Type != "member.removed" {
					_ = m.Ack()
					return
				}
				var ev domain.MemberRemoved
				if err := json.Unmarshal(env.Payload, &ev); err != nil {
					log.Printf("nats: ws revocation fanout: bad member.removed payload: %v", err)
					_ = m.Ack()
					return
				}
				hub.DropSubscription(ev.UserID, ev.ConversationID)
				_ = m.Ack()
			}, natsgo.ManualAck())
			if err != nil {
				return fmt.Errorf("subscribe %s (revocation fanout): %w", messaging.WildcardSubject, err)
			}
			sub = s
			log.Printf("nats: ws revocation fanout subscribed to %s (ephemeral, per-pod)", messaging.WildcardSubject)
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
