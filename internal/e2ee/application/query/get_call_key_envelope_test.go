// internal/e2ee/application/query/get_call_key_envelope_test.go
package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/call/domain"
	domainconv "pet-messenger/internal/conversation/domain"
	"pet-messenger/internal/e2ee/application/port/in"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

type fakeDeviceLoader struct {
	byID map[uuid.UUID]*domaine2ee.Device
}

func (f *fakeDeviceLoader) Get(_ context.Context, id uuid.UUID) (*domaine2ee.Device, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return nil, domaine2ee.ErrNotFound
}

type fakeKeyEnvelopeReader struct {
	byRecipientScope map[string]*domaine2ee.KeyEnvelope
}

func (f *fakeKeyEnvelopeReader) GetKeyEnvelope(_ context.Context, recipientID uuid.UUID, scope string) (*domaine2ee.KeyEnvelope, error) {
	if e, ok := f.byRecipientScope[recipientID.String()+"|"+scope]; ok {
		return e, nil
	}
	return nil, domaine2ee.ErrNotFound
}

type fakeCallLoader struct {
	byID map[uuid.UUID]*domaincall.Call
}

func (f *fakeCallLoader) GetByID(_ context.Context, id uuid.UUID) (*domaincall.Call, error) {
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, domaincall.ErrNotFound
}

func TestGetCallKeyEnvelope_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, ConversationID: convID, InitiatorID: ownerID, Status: domaincall.StatusActive}
	recipient := &domaine2ee.Device{ID: recipientID, UserID: actor, Status: domaine2ee.StatusTrusted}
	owner := &domaine2ee.Device{ID: ownerID, UserID: ownerID, Status: domaine2ee.StatusTrusted}
	envelope := &domaine2ee.KeyEnvelope{OwnerDeviceID: ownerID, RecipientDeviceID: recipientID, Ciphertext: []byte("ct")}

	h := NewGetCallKeyEnvelopeHandler(
		&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{recipientID: recipient, ownerID: owner}},
		&fakeKeyEnvelopeReader{byRecipientScope: map[string]*domaine2ee.KeyEnvelope{recipientID.String() + "|" + callKeyScope(callID): envelope}},
		&fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}},
		&fakeConversationMemberChecker{member: &domainconv.Member{}},
	)

	gotEnvelope, gotOwner, err := h.Handle(context.Background(), in.GetCallKeyEnvelopeQuery{Actor: actor, CallID: callID, RecipientDeviceID: recipientID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if gotEnvelope != envelope || gotOwner != owner {
		t.Errorf("envelope=%+v owner=%+v", gotEnvelope, gotOwner)
	}
}

func TestGetCallKeyEnvelope_CallNotFound(t *testing.T) {
	h := NewGetCallKeyEnvelopeHandler(&fakeDeviceLoader{}, &fakeKeyEnvelopeReader{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{}}, &fakeConversationMemberChecker{})
	_, _, err := h.Handle(context.Background(), in.GetCallKeyEnvelopeQuery{CallID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, ErrCallNotFound) {
		t.Fatalf("err = %v, want ErrCallNotFound", err)
	}
}

func TestGetCallKeyEnvelope_RecipientNotTrusted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, Status: domaincall.StatusActive}
	h := NewGetCallKeyEnvelopeHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{}}, &fakeKeyEnvelopeReader{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	_, _, err := h.Handle(context.Background(), in.GetCallKeyEnvelopeQuery{Actor: actor, CallID: callID, RecipientDeviceID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, ErrRecipientNotTrusted) {
		t.Fatalf("err = %v, want ErrRecipientNotTrusted", err)
	}
}

func TestGetCallKeyEnvelope_KeyNotReady(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, Status: domaincall.StatusActive}
	recipient := &domaine2ee.Device{ID: recipientID, UserID: actor, Status: domaine2ee.StatusTrusted}
	h := NewGetCallKeyEnvelopeHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{recipientID: recipient}}, &fakeKeyEnvelopeReader{byRecipientScope: map[string]*domaine2ee.KeyEnvelope{}}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	_, _, err := h.Handle(context.Background(), in.GetCallKeyEnvelopeQuery{Actor: actor, CallID: callID, RecipientDeviceID: recipientID})
	if !errors.Is(err, ErrKeyNotReady) {
		t.Fatalf("err = %v, want ErrKeyNotReady", err)
	}
}

func TestGetCallKeyEnvelope_OwnerNoLongerTrusted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, InitiatorID: uuid.Must(uuid.NewV7()), Status: domaincall.StatusActive} // owner != initiator
	recipient := &domaine2ee.Device{ID: recipientID, UserID: actor, Status: domaine2ee.StatusTrusted}
	owner := &domaine2ee.Device{ID: ownerID, Status: domaine2ee.StatusTrusted}
	envelope := &domaine2ee.KeyEnvelope{OwnerDeviceID: ownerID, RecipientDeviceID: recipientID}
	h := NewGetCallKeyEnvelopeHandler(
		&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{recipientID: recipient, ownerID: owner}},
		&fakeKeyEnvelopeReader{byRecipientScope: map[string]*domaine2ee.KeyEnvelope{recipientID.String() + "|" + callKeyScope(callID): envelope}},
		&fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}},
		&fakeConversationMemberChecker{member: &domainconv.Member{}},
	)

	_, _, err := h.Handle(context.Background(), in.GetCallKeyEnvelopeQuery{Actor: actor, CallID: callID, RecipientDeviceID: recipientID})
	if !errors.Is(err, ErrOwnerNoLongerTrusted) {
		t.Fatalf("err = %v, want ErrOwnerNoLongerTrusted", err)
	}
}
