package nats

import (
	"time"

	domainmsg "pet-messenger/internal/message/domain"
)

// MessageEventType is the kind of change a MessageEvent carries.
type MessageEventType string

// Message event types. The values double as the "type" field a WS client sees.
const (
	MessageCreated MessageEventType = "message.created"
	MessageUpdated MessageEventType = "message.updated"
	MessageDeleted MessageEventType = "message.deleted"
)

// MessageEvent is the JSON payload published on SubjectForConversation after a
// message write commits. The publisher marshals it once; the subscriber, Hub,
// and WS write-pump treat the bytes as opaque — so this struct is also exactly
// the envelope a WS client receives.
//
// Consumers must order by the message's Seq; NATS/WS arrival order is not
// guaranteed (concurrent sends can interleave between the DB write and the
// publish).
type MessageEvent struct {
	Type           MessageEventType `json:"type"`
	ConversationID string           `json:"conversation_id"`
	Message        MessagePayload   `json:"message"`
}

// MessagePayload mirrors the fields the HTTP layer exposes (messageResponse). A
// soft-deleted message arrives with Content cleared and DeletedAt set.
type MessagePayload struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	Seq            int64      `json:"seq"`
	SenderID       *string    `json:"sender_id"`
	Type           string     `json:"type"`
	Content        string     `json:"content"`
	ReplyToID      *string    `json:"reply_to_id"`
	SystemEvent    *string    `json:"system_event,omitempty"`
	SystemTargetID *string    `json:"system_target_id,omitempty"`
	EditedAt       *time.Time `json:"edited_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ToPayload maps a message aggregate onto the wire payload published on
// SubjectForConversation.
func ToPayload(m *domainmsg.Message) MessagePayload {
	p := MessagePayload{
		ID:             m.ID.String(),
		ConversationID: m.ConversationID.String(),
		Seq:            m.Seq,
		Type:           string(m.Type),
		Content:        m.Content,
		EditedAt:       m.EditedAt,
		DeletedAt:      m.DeletedAt,
		CreatedAt:      m.CreatedAt,
	}
	if m.SenderID != nil {
		s := m.SenderID.String()
		p.SenderID = &s
	}
	if m.ReplyToID != nil {
		s := m.ReplyToID.String()
		p.ReplyToID = &s
	}
	if m.SystemEvent != "" {
		s := string(m.SystemEvent)
		p.SystemEvent = &s
	}
	if m.SystemTargetID != nil {
		s := m.SystemTargetID.String()
		p.SystemTargetID = &s
	}
	return p
}
