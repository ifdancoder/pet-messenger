// internal/e2ee/application/query/get_call_key_envelope.go
package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/domain/call"
	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// Sentinel errors.
var (
	// ErrCallNotFound: the call does not exist, or exists but is not live.
	ErrCallNotFound = errors.New("e2ee: active call not found")
	// ErrRecipientNotTrusted covers every reason the claimed recipient
	// device cannot receive a key: it does not exist, belongs to someone
	// else, or is not trusted.
	ErrRecipientNotTrusted = errors.New("e2ee: device is not trusted")
	// ErrKeyNotReady: the owning device has not yet published this call's
	// key for this recipient.
	ErrKeyNotReady = errors.New("e2ee: call key is not ready")
	// ErrOwnerNoLongerTrusted: the device that published this key is no
	// longer trusted, or is not the call's initiator.
	ErrOwnerNoLongerTrusted = errors.New("e2ee: call key owner is no longer trusted")
)

type getCallKeyEnvelopeHandler struct {
	loader        out.DeviceLoader
	envelopes     out.KeyEnvelopeReader
	calls         out.CallLoader
	conversations out.ConversationMemberChecker
}

// NewGetCallKeyEnvelopeHandler wires the GetCallKeyEnvelope use case.
func NewGetCallKeyEnvelopeHandler(loader out.DeviceLoader, envelopes out.KeyEnvelopeReader, calls out.CallLoader, conversations out.ConversationMemberChecker) in.GetCallKeyEnvelopeUseCase {
	return &getCallKeyEnvelopeHandler{loader: loader, envelopes: envelopes, calls: calls, conversations: conversations}
}

// callForMember loads callID, confirms it is live, and confirms actor is a
// member of its conversation. Mirrors the command package's own copy of
// this logic exactly; duplicated rather than shared, per the design spec.
func (h *getCallKeyEnvelopeHandler) callForMember(ctx context.Context, callID, actor uuid.UUID) (*domaincall.Call, error) {
	call, err := h.calls.GetByID(ctx, callID)
	if err != nil || call.Status != domaincall.StatusActive {
		return nil, ErrCallNotFound
	}
	if _, err := h.conversations.GetMember(ctx, call.ConversationID, actor); err != nil {
		return nil, ErrNotConversationMember
	}
	return call, nil
}

// callKeyScope is the key-envelope scope string for one call's session
// key. Mirrors the command package's own copy exactly; both must produce
// the identical string, since they address the same envelope rows from two
// different packages.
func callKeyScope(callID uuid.UUID) string { return "call:" + callID.String() }

// Handle returns q.CallID's encrypted session key, wrapped for
// q.RecipientDeviceID, once the owning device has published it and is
// still trusted.
func (h *getCallKeyEnvelopeHandler) Handle(ctx context.Context, q in.GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error) {
	call, err := h.callForMember(ctx, q.CallID, q.Actor)
	if err != nil {
		return nil, nil, err
	}
	recipient, err := h.loader.Get(ctx, q.RecipientDeviceID)
	if err != nil || recipient.UserID != q.Actor || recipient.Status != domaine2ee.StatusTrusted {
		return nil, nil, ErrRecipientNotTrusted
	}
	envelope, err := h.envelopes.GetKeyEnvelope(ctx, q.RecipientDeviceID, callKeyScope(call.ID))
	if errors.Is(err, domaine2ee.ErrNotFound) {
		return nil, nil, ErrKeyNotReady
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get key envelope: %w", err)
	}
	owner, err := h.loader.Get(ctx, envelope.OwnerDeviceID)
	if err != nil || owner.Status != domaine2ee.StatusTrusted || owner.UserID != call.InitiatorID {
		return nil, nil, ErrOwnerNoLongerTrusted
	}
	return envelope, owner, nil
}
