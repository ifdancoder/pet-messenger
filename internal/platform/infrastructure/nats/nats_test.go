package nats

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmsg "pet-messenger/internal/message/domain"
)

func TestSubjectRoundTrip(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	subj := SubjectForConversation(id.String())
	if subj != "msg."+id.String() {
		t.Fatalf("subject = %q", subj)
	}
	got, ok := ConversationIDFromSubject(subj)
	if !ok || got != id {
		t.Fatalf("ConversationIDFromSubject(%q) = %v, %v", subj, got, ok)
	}
}

func TestConversationIDFromSubject_Rejects(t *testing.T) {
	for _, bad := range []string{"msg.", "msg.not-a-uuid", "other." + uuid.Must(uuid.NewV7()).String(), "", "msg"} {
		if _, ok := ConversationIDFromSubject(bad); ok {
			t.Errorf("ConversationIDFromSubject(%q) accepted", bad)
		}
	}
}

func TestToPayload(t *testing.T) {
	sender := uuid.Must(uuid.NewV7())
	edited := time.Now().UTC()
	m := &domainmsg.Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: uuid.Must(uuid.NewV7()),
		Seq:            7,
		SenderID:       &sender,
		Type:           domainmsg.TypeText,
		Content:        "hi",
		EditedAt:       &edited,
		CreatedAt:      time.Now().UTC(),
	}
	p := ToPayload(m)
	if p.ID != m.ID.String() || p.ConversationID != m.ConversationID.String() || p.Seq != 7 {
		t.Fatalf("payload ids/seq wrong: %+v", p)
	}
	if p.SenderID == nil || *p.SenderID != sender.String() {
		t.Fatalf("SenderID = %v", p.SenderID)
	}
	if p.Type != "text" || p.Content != "hi" || p.EditedAt == nil || p.DeletedAt != nil {
		t.Fatalf("payload = %+v", p)
	}
}

func TestMessageEvent_JSONShape(t *testing.T) {
	m := &domainmsg.Message{
		ID:             uuid.Must(uuid.NewV7()),
		ConversationID: uuid.Must(uuid.NewV7()),
		Seq:            1,
		Type:           domainmsg.TypeText,
		Content:        "x",
		CreatedAt:      time.Now().UTC(),
	}
	data, err := json.Marshal(MessageEvent{Type: MessageCreated, ConversationID: m.ConversationID.String(), Message: ToPayload(m)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back struct {
		Type           string `json:"type"`
		ConversationID string `json:"conversation_id"`
		Message        struct {
			ID       string  `json:"id"`
			Seq      int64   `json:"seq"`
			SenderID *string `json:"sender_id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Type != "message.created" || back.ConversationID != m.ConversationID.String() {
		t.Fatalf("envelope = %s", data)
	}
	if back.Message.ID != m.ID.String() || back.Message.Seq != 1 || back.Message.SenderID != nil {
		t.Fatalf("message = %s", data)
	}
}
