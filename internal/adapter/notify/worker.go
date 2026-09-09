package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"

	domainconv "pet-messenger/internal/domain/conversation"
	domainnotif "pet-messenger/internal/domain/notification"
	"pet-messenger/internal/domain/pushtoken"
	notifinfra "pet-messenger/internal/infrastructure/notification"
	"pet-messenger/internal/infrastructure/push"
)

type Worker struct {
	notifs   domainnotif.Repository
	tokens   pushtoken.Repository
	members  domainconv.Repository
	presence Presence
	pusher   push.Pusher
	ws       WSNotifier
}

func NewWorker(notifs domainnotif.Repository, tokens pushtoken.Repository, members domainconv.Repository, presence Presence, pusher push.Pusher, ws WSNotifier) *Worker {
	return &Worker{notifs: notifs, tokens: tokens, members: members, presence: presence, pusher: pusher, ws: ws}
}

// HandleMsg is the JetStream queue-subscribe callback. It never returns an
// error — it Acks / Naks / Terms the message itself.
func (w *Worker) HandleMsg(m *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var intent notifinfra.NotifIntent
	if err := json.Unmarshal(m.Data, &intent); err != nil {
		log.Printf("notify: undecodable intent on %s: %v", m.Subject, err)
		_ = m.Term()
		return
	}

	recipients, err := w.recipients(ctx, intent)
	if err != nil {
		log.Printf("notify: recipients for %s: %v — will retry", m.Subject, err)
		_ = m.Nak()
		return
	}

	for _, uid := range recipients {
		if err := w.deliverOne(ctx, uid, intent); err != nil {
			log.Printf("notify: deliver to %s: %v — will retry", uid, err)
			_ = m.Nak()
			return
		}
	}
	_ = m.Ack()
}

func (w *Worker) recipients(ctx context.Context, in notifinfra.NotifIntent) ([]uuid.UUID, error) {
	if in.RecipientID != nil {
		id, err := uuid.Parse(*in.RecipientID)
		if err != nil {
			return nil, nil // unparseable target — nothing to do, let the caller Ack (empty slice)
		}
		return []uuid.UUID{id}, nil
	}
	convID, err := uuid.Parse(in.ConversationID)
	if err != nil {
		return nil, nil
	}
	ids, err := w.members.ListMemberIDs(ctx, convID)
	if err != nil {
		return nil, err
	}
	var sender uuid.UUID
	if in.SenderID != nil {
		sender, _ = uuid.Parse(*in.SenderID)
	}
	out := ids[:0:0]
	for _, id := range ids {
		if id != sender {
			out = append(out, id)
		}
	}
	return out, nil
}

func (w *Worker) deliverOne(ctx context.Context, uid uuid.UUID, in notifinfra.NotifIntent) error {
	switch in.Kind {
	case "message", "member_removed", "member_tier_changed":
		// known — proceed
	default:
		log.Printf("notify: unknown intent kind %q for %s — skipping", in.Kind, uid)
		return nil
	}

	kind, payload := kindAndPayload(uid, in)
	dedup := dedupKey(uid, in)
	n, err := domainnotif.New(uid, kind, payload, dedup)
	if err != nil {
		log.Printf("notify: build notification for %s: %v — skipping", uid, err)
		return nil // malformed intent for this recipient — do not retry forever
	}
	switch err := w.notifs.Create(ctx, n); {
	case errors.Is(err, domainnotif.ErrDuplicate):
		return nil // already delivered on an earlier attempt
	case err != nil:
		return err // transient — retry the whole message
	}

	online, perr := w.presence.Online(ctx, uid)
	if perr != nil {
		log.Printf("notify: presence for %s: %v — treating as offline", uid, perr)
		online = false
	}
	if online {
		if err := w.ws.Notify(uid, push.Body(n)); err != nil {
			log.Printf("notify: ws notify %s: %v", uid, err)
		}
		return nil
	}
	toks, err := w.tokens.ListForUser(ctx, uid)
	if err != nil {
		return err // transient — retry
	}
	for _, tok := range toks {
		switch err := w.pusher.Send(ctx, tok, n); {
		case err == nil:
		case errors.Is(err, push.ErrTokenGone):
			if derr := w.tokens.DeleteToken(ctx, tok.Token); derr != nil {
				log.Printf("notify: delete gone token: %v", derr)
			}
		default:
			log.Printf("notify: push to %s (%s): %v", uid, tok.Platform, err)
		}
	}
	return nil
}

func dedupKey(uid uuid.UUID, in notifinfra.NotifIntent) string {
	switch in.Kind {
	case "message":
		if in.MessageID != nil {
			return "msg:" + *in.MessageID
		}
	case "member_removed":
		return "kick:" + in.ConversationID + ":" + uid.String()
	case "member_tier_changed":
		return "tier:" + in.ConversationID + ":" + uid.String() + ":" + in.NewTier
	}
	return ""
}

func kindAndPayload(uid uuid.UUID, in notifinfra.NotifIntent) (domainnotif.Kind, json.RawMessage) {
	switch in.Kind {
	case "message":
		b, _ := json.Marshal(map[string]any{
			"conversation_id": in.ConversationID, "message_id": deref(in.MessageID),
			"message_seq": in.MessageSeq, "sender_id": deref(in.SenderID), "preview": in.Preview,
		})
		return domainnotif.KindMessage, b
	case "member_removed":
		b, _ := json.Marshal(map[string]any{"conversation_id": in.ConversationID})
		return domainnotif.KindMemberRemoved, b
	case "member_tier_changed":
		b, _ := json.Marshal(map[string]any{"conversation_id": in.ConversationID, "new_tier": in.NewTier})
		return domainnotif.KindMemberTierChanged, b
	}
	return domainnotif.KindMessage, json.RawMessage(`{}`) // unreachable — deliverOne guards on a known Kind
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
