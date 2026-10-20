// internal/e2ee/application/command/approve_device.go
package command

import (
	"context"
	"crypto/ed25519"
	"errors"

	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/port/out"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

// Sentinel errors.
var (
	// ErrApproverNotTrusted covers every reason the claimed approver cannot
	// vouch for anything: it does not exist, belongs to someone else, is
	// not itself trusted, or has a malformed signing key.
	ErrApproverNotTrusted = errors.New("e2ee: approver device is not trusted")
	// ErrPendingDeviceNotFound covers every reason the claimed target is
	// not an approvable pending device: it does not exist, belongs to
	// someone else, or is not pending.
	ErrPendingDeviceNotFound = errors.New("e2ee: pending device not found")
	// ErrInvalidApprovalSignature: the approver's signature does not verify
	// over the target's fingerprint.
	ErrInvalidApprovalSignature = errors.New("e2ee: invalid approval signature")
	// ErrCannotApprove: the persistence layer rejected the approval for a
	// reason other than the target no longer being pending (a narrow,
	// unexpected failure mode preserved from the original handler).
	ErrCannotApprove = errors.New("e2ee: device cannot approve this request")
)

type approveDeviceHandler struct {
	loader out.DeviceLoader
	writer out.DeviceWriter
}

// NewApproveDeviceHandler wires the ApproveDevice use case.
func NewApproveDeviceHandler(loader out.DeviceLoader, writer out.DeviceWriter) in.ApproveDeviceUseCase {
	return &approveDeviceHandler{loader: loader, writer: writer}
}

// Handle trusts cmd.TargetID on cmd.ApproverID's vouching. Both devices
// must belong to cmd.Actor; the approver must already be trusted and the
// target must still be pending; the signature must verify over the
// target's fingerprint.
func (h *approveDeviceHandler) Handle(ctx context.Context, cmd in.ApproveDeviceCommand) error {
	approver, err := h.loader.Get(ctx, cmd.ApproverID)
	if err != nil || approver.UserID != cmd.Actor || approver.Status != domaine2ee.StatusTrusted || len(approver.SigningPublicKey) != ed25519.PublicKeySize {
		return ErrApproverNotTrusted
	}
	target, err := h.loader.Get(ctx, cmd.TargetID)
	if err != nil || target.UserID != cmd.Actor || target.Status != domaine2ee.StatusPending {
		return ErrPendingDeviceNotFound
	}
	if !approver.VerifyApprovalSignature(cmd.TargetID, target.Fingerprint, cmd.Signature) {
		return ErrInvalidApprovalSignature
	}
	if err := h.writer.Approve(ctx, cmd.ApproverID, cmd.TargetID); err != nil {
		if errors.Is(err, domaine2ee.ErrNotFound) {
			return ErrPendingDeviceNotFound
		}
		return ErrCannotApprove
	}
	return nil
}
