// internal/e2ee/application/command/put_call_key_envelopes_test.go
package command

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

type fakeTrustedDeviceLister struct {
	byConv map[uuid.UUID][]*domaine2ee.Device
	err    error
}

func (f *fakeTrustedDeviceLister) ListTrustedForConversation(_ context.Context, convID uuid.UUID) ([]*domaine2ee.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byConv[convID], nil
}

type fakeKeyEnvelopeWriter struct {
	put []*domaine2ee.KeyEnvelope
	err error
}

func (f *fakeKeyEnvelopeWriter) PutKeyEnvelopes(_ context.Context, envelopes []*domaine2ee.KeyEnvelope) error {
	if f.err != nil {
		return f.err
	}
	f.put = append(f.put, envelopes...)
	return nil
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

type fakeConversationMemberChecker struct {
	member *domainconv.Member
	err    error
}

func (f *fakeConversationMemberChecker) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return f.member, f.err
}

func TestPutCallKeyEnvelopes_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, ConversationID: convID, InitiatorID: actor, Status: domaincall.StatusActive}
	owner := &domaine2ee.Device{ID: ownerID, UserID: actor, Status: domaine2ee.StatusTrusted}

	loader := &fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{ownerID: owner}}
	trustedLister := &fakeTrustedDeviceLister{byConv: map[uuid.UUID][]*domaine2ee.Device{convID: {{ID: recipientID}}}}
	envelopeWriter := &fakeKeyEnvelopeWriter{}
	calls := &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}
	conversations := &fakeConversationMemberChecker{member: &domainconv.Member{}}
	h := NewPutCallKeyEnvelopesHandler(loader, trustedLister, envelopeWriter, calls, conversations)

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{
		Actor: actor, CallID: callID, OwnerDeviceID: ownerID,
		Envelopes: []in.KeyEnvelopeInput{{RecipientDeviceID: recipientID, Ciphertext: []byte("ct")}},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(envelopeWriter.put) != 1 || envelopeWriter.put[0].RecipientDeviceID != recipientID {
		t.Fatalf("put = %+v", envelopeWriter.put)
	}
}

func TestPutCallKeyEnvelopes_CallNotFound(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{}, &fakeTrustedDeviceLister{}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{}}, &fakeConversationMemberChecker{})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, ErrCallNotFound) {
		t.Fatalf("err = %v, want ErrCallNotFound", err)
	}
}

func TestPutCallKeyEnvelopes_NotConversationMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, Status: domaincall.StatusActive, InitiatorID: actor}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{}, &fakeTrustedDeviceLister{}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{err: domainconv.ErrNotMember})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: callID})
	if !errors.Is(err, ErrNotConversationMember) {
		t.Fatalf("err = %v, want ErrNotConversationMember", err)
	}
}

func TestPutCallKeyEnvelopes_NotCallInitiator(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	initiator := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, Status: domaincall.StatusActive, InitiatorID: initiator}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{}, &fakeTrustedDeviceLister{}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: callID})
	if !errors.Is(err, ErrNotCallInitiator) {
		t.Fatalf("err = %v, want ErrNotCallInitiator", err)
	}
}

func TestPutCallKeyEnvelopes_OwnerNotTrusted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, Status: domaincall.StatusActive, InitiatorID: actor}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{}}, &fakeTrustedDeviceLister{}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: callID, OwnerDeviceID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, ErrOwnerNotTrusted) {
		t.Fatalf("err = %v, want ErrOwnerNotTrusted", err)
	}
}

func TestPutCallKeyEnvelopes_InvalidEnvelope_NotTrusted(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, ConversationID: convID, InitiatorID: actor, Status: domaincall.StatusActive}
	owner := &domaine2ee.Device{ID: ownerID, UserID: actor, Status: domaine2ee.StatusTrusted}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{ownerID: owner}}, &fakeTrustedDeviceLister{byConv: map[uuid.UUID][]*domaine2ee.Device{convID: {}}}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{
		Actor: actor, CallID: callID, OwnerDeviceID: ownerID,
		Envelopes: []in.KeyEnvelopeInput{{RecipientDeviceID: uuid.Must(uuid.NewV7()), Ciphertext: []byte("ct")}},
	})
	if !errors.Is(err, ErrInvalidKeyEnvelope) {
		t.Fatalf("err = %v, want ErrInvalidKeyEnvelope", err)
	}
}

func TestPutCallKeyEnvelopes_InvalidEnvelope_Duplicate(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, ConversationID: convID, InitiatorID: actor, Status: domaincall.StatusActive}
	owner := &domaine2ee.Device{ID: ownerID, UserID: actor, Status: domaine2ee.StatusTrusted}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{ownerID: owner}}, &fakeTrustedDeviceLister{byConv: map[uuid.UUID][]*domaine2ee.Device{convID: {{ID: recipientID}}}}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{
		Actor: actor, CallID: callID, OwnerDeviceID: ownerID,
		Envelopes: []in.KeyEnvelopeInput{
			{RecipientDeviceID: recipientID, Ciphertext: []byte("ct1")},
			{RecipientDeviceID: recipientID, Ciphertext: []byte("ct2")},
		},
	})
	if !errors.Is(err, ErrInvalidKeyEnvelope) {
		t.Fatalf("err = %v, want ErrInvalidKeyEnvelope", err)
	}
}

func TestPutCallKeyEnvelopes_MissingEnvelope(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	call := &domaincall.Call{ID: callID, ConversationID: convID, InitiatorID: actor, Status: domaincall.StatusActive}
	owner := &domaine2ee.Device{ID: ownerID, UserID: actor, Status: domaine2ee.StatusTrusted}
	h := NewPutCallKeyEnvelopesHandler(&fakeDeviceLoader{byID: map[uuid.UUID]*domaine2ee.Device{ownerID: owner}}, &fakeTrustedDeviceLister{byConv: map[uuid.UUID][]*domaine2ee.Device{convID: {{ID: uuid.Must(uuid.NewV7())}, {ID: uuid.Must(uuid.NewV7())}}}}, &fakeKeyEnvelopeWriter{}, &fakeCallLoader{byID: map[uuid.UUID]*domaincall.Call{callID: call}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	err := h.Handle(context.Background(), in.PutCallKeyEnvelopesCommand{Actor: actor, CallID: callID, OwnerDeviceID: ownerID, Envelopes: nil})
	if !errors.Is(err, ErrMissingKeyEnvelope) {
		t.Fatalf("err = %v, want ErrMissingKeyEnvelope", err)
	}
}
