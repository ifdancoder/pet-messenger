// internal/e2ee/application/command/put_call_key_envelopes.go
package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domaincall "pet-messenger/internal/call/domain"
	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// Sentinel errors.
var (
	// ErrCallNotFound: the call does not exist, or exists but is not live.
	ErrCallNotFound = errors.New("e2ee: active call not found")
	// ErrNotConversationMember: the actor is not a member of the call's
	// conversation.
	ErrNotConversationMember = errors.New("e2ee: actor is not a conversation member")
	// ErrNotCallInitiator: only the call's initiator may publish its key.
	ErrNotCallInitiator = errors.New("e2ee: only the call initiator may publish its key")
	// ErrOwnerNotTrusted covers every reason the claimed owner device
	// cannot publish a key: it does not exist, belongs to someone else, or
	// is not trusted.
	ErrOwnerNotTrusted = errors.New("e2ee: owner device is not trusted")
	// ErrInvalidKeyEnvelope: a submitted envelope's recipient is not a
	// trusted device in this conversation, or the same recipient appears
	// twice.
	ErrInvalidKeyEnvelope = errors.New("e2ee: invalid call key envelope")
	// ErrMissingKeyEnvelope: the submitted envelope set does not cover
	// every trusted device in the conversation exactly once.
	ErrMissingKeyEnvelope = errors.New("e2ee: a call key envelope is required for every trusted device")
)

type putCallKeyEnvelopesHandler struct {
	loader        out.DeviceLoader
	trustedLister out.TrustedDeviceLister
	envelopes     out.KeyEnvelopeWriter
	calls         out.CallLoader
	conversations out.ConversationMemberChecker
}

// NewPutCallKeyEnvelopesHandler wires the PutCallKeyEnvelopes use case.
func NewPutCallKeyEnvelopesHandler(loader out.DeviceLoader, trustedLister out.TrustedDeviceLister, envelopes out.KeyEnvelopeWriter, calls out.CallLoader, conversations out.ConversationMemberChecker) in.PutCallKeyEnvelopesUseCase {
	return &putCallKeyEnvelopesHandler{loader: loader, trustedLister: trustedLister, envelopes: envelopes, calls: calls, conversations: conversations}
}

// callForMember loads callID, confirms it is live, and confirms actor is a
// member of its conversation.
func (h *putCallKeyEnvelopesHandler) callForMember(ctx context.Context, callID, actor uuid.UUID) (*domaincall.Call, error) {
	call, err := h.calls.GetByID(ctx, callID)
	if err != nil || call.Status != domaincall.StatusActive {
		return nil, ErrCallNotFound
	}
	if _, err := h.conversations.GetMember(ctx, call.ConversationID, actor); err != nil {
		return nil, ErrNotConversationMember
	}
	return call, nil
}

// Handle publishes cmd.CallID's encrypted session key, wrapped once for
// every trusted device in the call's conversation. cmd.Actor must be the
// call's initiator; cmd.OwnerDeviceID must be one of cmd.Actor's own
// trusted devices; cmd.Envelopes must cover every trusted device in the
// conversation exactly once.
func (h *putCallKeyEnvelopesHandler) Handle(ctx context.Context, cmd in.PutCallKeyEnvelopesCommand) error {
	call, err := h.callForMember(ctx, cmd.CallID, cmd.Actor)
	if err != nil {
		return err
	}
	if call.InitiatorID != cmd.Actor {
		return ErrNotCallInitiator
	}
	owner, err := h.loader.Get(ctx, cmd.OwnerDeviceID)
	if err != nil || owner.UserID != cmd.Actor || owner.Status != domaine2ee.StatusTrusted {
		return ErrOwnerNotTrusted
	}
	trusted, err := h.trustedLister.ListTrustedForConversation(ctx, call.ConversationID)
	if err != nil {
		return fmt.Errorf("list trusted devices: %w", err)
	}
	allowed := make(map[uuid.UUID]bool, len(trusted))
	for _, device := range trusted {
		allowed[device.ID] = true
	}

	now := time.Now().UTC()
	envelopes := make([]*domaine2ee.KeyEnvelope, 0, len(cmd.Envelopes))
	seen := make(map[uuid.UUID]bool, len(cmd.Envelopes))
	for _, item := range cmd.Envelopes {
		if !allowed[item.RecipientDeviceID] || seen[item.RecipientDeviceID] {
			return ErrInvalidKeyEnvelope
		}
		seen[item.RecipientDeviceID] = true
		envelopes = append(envelopes, &domaine2ee.KeyEnvelope{OwnerDeviceID: cmd.OwnerDeviceID, RecipientDeviceID: item.RecipientDeviceID, Scope: callKeyScope(call.ID), Ciphertext: item.Ciphertext, CreatedAt: now})
	}
	if len(envelopes) != len(allowed) {
		return ErrMissingKeyEnvelope
	}
	if err := h.envelopes.PutKeyEnvelopes(ctx, envelopes); err != nil {
		return fmt.Errorf("put key envelopes: %w", err)
	}
	return nil
}

// callKeyScope is the key-envelope scope string for one call's session
// key, shared with GetCallKeyEnvelope's own matching scope computation.
func callKeyScope(callID uuid.UUID) string { return "call:" + callID.String() }
