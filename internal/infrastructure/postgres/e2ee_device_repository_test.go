package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"pet-messenger/internal/domain/conversation"
	"pet-messenger/internal/domain/e2ee"
)

func testE2EEDevice(userID uuid.UUID, marker byte) *e2ee.Device {
	publicKey := make([]byte, 32)
	publicKey[0] = marker
	return &e2ee.Device{
		ID: uuid.New(), UserID: userID, PublicKey: publicKey,
		Fingerprint: uuid.NewString(), Label: "test", CreatedAt: time.Now().UTC(),
	}
}

func TestE2EEDeviceRepositoryTrustLifecycleAndConversationDiscovery(t *testing.T) {
	ctx := context.Background()
	devices := NewE2EEDeviceRepository(testDB)
	conversations := NewConversationRepository(testDB)
	firstUser := createTestUser(t)
	secondUser := createTestUser(t)

	first := testE2EEDevice(firstUser.ID, 1)
	if err := devices.Register(ctx, first); err != nil {
		t.Fatalf("register first device: %v", err)
	}
	if first.Status != e2ee.StatusTrusted {
		t.Fatalf("first device status = %q, want trusted", first.Status)
	}

	pending := testE2EEDevice(firstUser.ID, 2)
	if err := devices.Register(ctx, pending); err != nil {
		t.Fatalf("register second device: %v", err)
	}
	if pending.Status != e2ee.StatusPending {
		t.Fatalf("second device status = %q, want pending", pending.Status)
	}
	if err := devices.Approve(ctx, first.ID, pending.ID); err != nil {
		t.Fatalf("approve second device: %v", err)
	}

	other := testE2EEDevice(secondUser.ID, 3)
	if err := devices.Register(ctx, other); err != nil {
		t.Fatalf("register other user's device: %v", err)
	}

	chat, members, err := conversation.NewGroupChat(firstUser.ID, "encrypted room")
	if err != nil {
		t.Fatalf("new group: %v", err)
	}
	if err := conversations.Create(ctx, chat, members); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := conversations.AddMember(ctx, &conversation.Member{
		ConversationID: chat.ID, UserID: secondUser.ID, Role: conversation.RoleMember, JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	got, err := devices.ListTrustedForConversation(ctx, chat.ID)
	if err != nil {
		t.Fatalf("list conversation devices: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("trusted device count = %d, want 3", len(got))
	}
	seen := map[uuid.UUID]bool{}
	for _, device := range got {
		seen[device.ID] = true
	}
	if !seen[first.ID] || !seen[pending.ID] || !seen[other.ID] {
		t.Fatalf("conversation device set = %v, missing a trusted device", seen)
	}

	scope := "call:" + uuid.NewString()
	wantCiphertext := []byte("opaque call key")
	if err := devices.PutKeyEnvelopes(ctx, []*e2ee.KeyEnvelope{{
		OwnerDeviceID: first.ID, RecipientDeviceID: other.ID, Scope: scope,
		Ciphertext: wantCiphertext, CreatedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("put call key envelope: %v", err)
	}
	envelope, err := devices.GetKeyEnvelope(ctx, other.ID, scope)
	if err != nil {
		t.Fatalf("get call key envelope: %v", err)
	}
	if envelope.OwnerDeviceID != first.ID || string(envelope.Ciphertext) != string(wantCiphertext) {
		t.Fatalf("key envelope = %#v, want owner %s ciphertext %q", envelope, first.ID, wantCiphertext)
	}
}
