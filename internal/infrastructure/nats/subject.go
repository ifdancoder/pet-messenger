// Package nats holds the NATS event schema and the core-NATS publisher used to
// fan message changes out across api pods. It is package `nats`; the upstream
// client github.com/nats-io/nats.go is also package `nats`, so files here import
// it as `natsgo`.
package nats

import "github.com/google/uuid"

// WildcardSubject is the per-pod subscription: one subscription receives every
// conversation's message events and the pod filters locally.
const WildcardSubject = "msg.>"

// SubjectForConversation is where a conversation's message events are published.
func SubjectForConversation(conversationID string) string {
	return "msg." + conversationID
}

// ConversationIDFromSubject parses "msg.<uuid>" back to the conversation id.
func ConversationIDFromSubject(subject string) (uuid.UUID, bool) {
	const prefix = "msg."
	if len(subject) <= len(prefix) || subject[:len(prefix)] != prefix {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(subject[len(prefix):])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
