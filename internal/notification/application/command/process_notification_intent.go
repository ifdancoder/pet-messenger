// internal/notification/application/command/process_notification_intent.go
package command

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/google/uuid"

	"pet-messenger/internal/platform/infrastructure/push"
	"pet-messenger/internal/notification/application/port/in"
	"pet-messenger/internal/notification/application/port/out"
	domainnotif "pet-messenger/internal/notification/domain"
)

type processNotificationIntentHandler struct {
	writer   out.NotificationWriter
	members  out.MemberLister
	presence out.Presence
	ws       out.WSNotifier
	tokens   out.PushTokenLister
	pusher   out.Pusher
}

// NewProcessNotificationIntentHandler wires the ProcessNotificationIntent
// use case.
func NewProcessNotificationIntentHandler(writer out.NotificationWriter, members out.MemberLister, presence out.Presence, ws out.WSNotifier, tokens out.PushTokenLister, pusher out.Pusher) in.ProcessNotificationIntentUseCase {
	return &processNotificationIntentHandler{
		writer: writer, members: members, presence: presence, ws: ws, tokens: tokens, pusher: pusher,
	}
}

// Handle resolves cmd's recipients, records a Notification for each
// (idempotent via dedup), and delivers it over WebSocket if the recipient
// is online, falling back to device push otherwise. An unrecognized Kind is
// silently skipped (not an error): JetStream redelivery would never make it
// recognized.
func (h *processNotificationIntentHandler) Handle(ctx context.Context, cmd in.ProcessNotificationIntentCommand) error {
	switch cmd.Kind {
	case "message", "member_removed", "member_tier_changed", "call_started":
		// known; proceed
	default:
		log.Printf("notification: unknown intent kind %q; skipping", cmd.Kind)
		return nil
	}

	recipients, err := h.recipients(ctx, cmd)
	if err != nil {
		return err
	}
	for _, uid := range recipients {
		if err := h.deliverOne(ctx, uid, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (h *processNotificationIntentHandler) recipients(ctx context.Context, cmd in.ProcessNotificationIntentCommand) ([]uuid.UUID, error) {
	if cmd.RecipientID != nil {
		id, err := uuid.Parse(*cmd.RecipientID)
		if err != nil {
			return nil, nil // unparseable target; nothing to do, let the caller Ack (empty slice)
		}
		return []uuid.UUID{id}, nil
	}
	convID, err := uuid.Parse(cmd.ConversationID)
	if err != nil {
		return nil, nil
	}
	ids, err := h.members.ListMemberIDs(ctx, convID)
	if err != nil {
		return nil, err
	}
	var sender uuid.UUID
	if cmd.SenderID != nil {
		sender, _ = uuid.Parse(*cmd.SenderID)
	}
	kept := ids[:0:0]
	for _, id := range ids {
		if id != sender {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

func (h *processNotificationIntentHandler) deliverOne(ctx context.Context, uid uuid.UUID, cmd in.ProcessNotificationIntentCommand) error {
	kind, payload := kindAndPayload(uid, cmd)
	dedup := dedupKey(uid, cmd)
	n, err := domainnotif.New(uid, kind, payload, dedup)
	if err != nil {
		log.Printf("notification: build notification for %s: %v; skipping", uid, err)
		return nil // malformed intent for this recipient; do not retry forever
	}
	switch err := h.writer.Create(ctx, n); {
	case errors.Is(err, domainnotif.ErrDuplicate):
		return nil // already delivered on an earlier attempt
	case err != nil:
		return err // transient; retry the whole message
	}

	online, perr := h.presence.Online(ctx, uid)
	if perr != nil {
		log.Printf("notification: presence for %s: %v; treating as offline", uid, perr)
		online = false
	}
	if online {
		if err := h.ws.Notify(uid, wireEnvelope(n)); err != nil {
			log.Printf("notification: ws notify %s: %v", uid, err)
		}
		return nil
	}
	toks, err := h.tokens.ListForUser(ctx, uid)
	if err != nil {
		return err // transient; retry
	}
	for _, tok := range toks {
		switch err := h.pusher.Send(ctx, tok, n); {
		case err == nil:
		case errors.Is(err, push.ErrTokenGone):
			if derr := h.tokens.DeleteToken(ctx, tok.Token); derr != nil {
				log.Printf("notification: delete gone token: %v", derr)
			}
		default:
			log.Printf("notification: push to %s (%s): %v", uid, tok.Platform, err)
		}
	}
	return nil
}

// wireEnvelope is the JSON a client receives in a device push / the WS
// notification envelope's "notification" field. Pure serialization, no
// infra dependency; moved here from internal/infrastructure/push.Body,
// which had this logic misplaced in the adapter layer.
func wireEnvelope(n *domainnotif.Notification) []byte {
	b, _ := json.Marshal(struct {
		ID        string          `json:"id"`
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt string          `json:"created_at"`
	}{n.ID.String(), string(n.Kind), n.Payload, n.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")})
	return b
}

func dedupKey(uid uuid.UUID, cmd in.ProcessNotificationIntentCommand) string {
	switch cmd.Kind {
	case "message":
		if cmd.MessageID != nil {
			return "msg:" + *cmd.MessageID
		}
	case "member_removed":
		return "kick:" + cmd.ConversationID + ":" + uid.String()
	case "member_tier_changed":
		return "tier:" + cmd.ConversationID + ":" + uid.String() + ":" + cmd.NewTier
	case "call_started":
		return "call:" + cmd.CallID + ":" + uid.String()
	}
	return ""
}

func kindAndPayload(uid uuid.UUID, cmd in.ProcessNotificationIntentCommand) (domainnotif.Kind, json.RawMessage) {
	switch cmd.Kind {
	case "message":
		b, _ := json.Marshal(map[string]any{
			"conversation_id": cmd.ConversationID, "message_id": deref(cmd.MessageID),
			"message_seq": cmd.MessageSeq, "sender_id": deref(cmd.SenderID), "preview": cmd.Preview,
		})
		return domainnotif.KindMessage, b
	case "member_removed":
		b, _ := json.Marshal(map[string]any{"conversation_id": cmd.ConversationID})
		return domainnotif.KindMemberRemoved, b
	case "member_tier_changed":
		b, _ := json.Marshal(map[string]any{"conversation_id": cmd.ConversationID, "new_tier": cmd.NewTier})
		return domainnotif.KindMemberTierChanged, b
	case "call_started":
		b, _ := json.Marshal(map[string]any{
			"conversation_id": cmd.ConversationID, "call_id": cmd.CallID,
		})
		return domainnotif.KindCallStarted, b
	}
	return domainnotif.KindMessage, json.RawMessage(`{}`) // unreachable; Handle guards on a known Kind
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
