package http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"pet-messenger/internal/domain/e2ee"
)

type fakeE2EERepository struct {
	devices  map[uuid.UUID]*e2ee.Device
	approved bool
}

func (f *fakeE2EERepository) Register(context.Context, *e2ee.Device) error { return nil }
func (f *fakeE2EERepository) Get(_ context.Context, id uuid.UUID) (*e2ee.Device, error) {
	d, ok := f.devices[id]
	if !ok {
		return nil, e2ee.ErrNotFound
	}
	return d, nil
}
func (f *fakeE2EERepository) ListForUser(context.Context, uuid.UUID) ([]*e2ee.Device, error) {
	return nil, nil
}
func (f *fakeE2EERepository) ListTrusted(context.Context, uuid.UUID) ([]*e2ee.Device, error) {
	return nil, nil
}
func (f *fakeE2EERepository) ListTrustedForConversation(context.Context, uuid.UUID) ([]*e2ee.Device, error) {
	return nil, nil
}
func (f *fakeE2EERepository) Approve(_ context.Context, approver, target uuid.UUID) error {
	f.approved = true
	return nil
}
func (f *fakeE2EERepository) PutKeyEnvelopes(context.Context, []*e2ee.KeyEnvelope) error { return nil }
func (f *fakeE2EERepository) GetKeyEnvelope(context.Context, uuid.UUID, string) (*e2ee.KeyEnvelope, error) {
	return nil, e2ee.ErrNotFound
}

func e2eeRouter(handler *E2EEDeviceHandler, actor uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/users")
	group.Use(func(c *gin.Context) { c.Set(ContextUserIDKey, actor); c.Next() })
	handler.RegisterRoutes(group)
	return router
}

func TestE2EEDeviceHandlerApproveRequiresValidDeviceSignature(t *testing.T) {
	actor := uuid.New()
	approverID := uuid.New()
	targetID := uuid.New()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	target := &e2ee.Device{ID: targetID, UserID: actor, Status: e2ee.StatusPending, Fingerprint: "target-fingerprint"}
	repo := &fakeE2EERepository{devices: map[uuid.UUID]*e2ee.Device{
		approverID: {ID: approverID, UserID: actor, Status: e2ee.StatusTrusted, SigningPublicKey: publicKey},
		targetID:   target,
	}}
	router := e2eeRouter(NewE2EEDeviceHandler(repo, nil, nil), actor)

	invalid := doConvJSON(t, router, http.MethodPost, "/users/me/e2ee/devices/"+targetID.String()+"/approve",
		`{"approver_id":"`+approverID.String()+`","signature":"`+base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))+`"}`)
	if invalid.Code != http.StatusForbidden || repo.approved {
		t.Fatalf("invalid signature: status=%d approved=%v", invalid.Code, repo.approved)
	}

	signature := ed25519.Sign(privateKey, approvalStatement(approverID, targetID, target.Fingerprint))
	valid := doConvJSON(t, router, http.MethodPost, "/users/me/e2ee/devices/"+targetID.String()+"/approve",
		`{"approver_id":"`+approverID.String()+`","signature":"`+base64.RawURLEncoding.EncodeToString(signature)+`"}`)
	if valid.Code != http.StatusNoContent || !repo.approved {
		t.Fatalf("valid signature: status=%d approved=%v body=%s", valid.Code, repo.approved, valid.Body.String())
	}
}
