// internal/e2ee/adapter/in/web/controller_test.go
package web

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/e2ee/application/command"
	"pet-messenger/internal/e2ee/application/port/in"
	"pet-messenger/internal/e2ee/application/query"
	domaine2ee "pet-messenger/internal/e2ee/domain"
)

type fakeRegister struct {
	fn func(cmd in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error)
}

func (f *fakeRegister) Handle(_ context.Context, cmd in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
	return f.fn(cmd)
}

type fakeApprove struct {
	fn func(cmd in.ApproveDeviceCommand) error
}

func (f *fakeApprove) Handle(_ context.Context, cmd in.ApproveDeviceCommand) error { return f.fn(cmd) }

type fakePutCallKeyEnvelopes struct {
	fn func(cmd in.PutCallKeyEnvelopesCommand) error
}

func (f *fakePutCallKeyEnvelopes) Handle(_ context.Context, cmd in.PutCallKeyEnvelopesCommand) error {
	return f.fn(cmd)
}

type fakeListDevices struct {
	fn func(q in.ListDevicesQuery) ([]*domaine2ee.Device, error)
}

func (f *fakeListDevices) Handle(_ context.Context, q in.ListDevicesQuery) ([]*domaine2ee.Device, error) {
	return f.fn(q)
}

type fakeListConversationDevices struct {
	fn func(q in.ListConversationDevicesQuery) ([]*domaine2ee.Device, error)
}

func (f *fakeListConversationDevices) Handle(_ context.Context, q in.ListConversationDevicesQuery) ([]*domaine2ee.Device, error) {
	return f.fn(q)
}

type fakeGetCallKeyEnvelope struct {
	fn func(q in.GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error)
}

func (f *fakeGetCallKeyEnvelope) Handle(_ context.Context, q in.GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error) {
	return f.fn(q)
}

func e2eeRouter(h *E2EEDeviceHandler, actor *uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	users := r.Group("/users")
	conversations := r.Group("/conversations")
	calls := r.Group("/calls")
	for _, grp := range []*gin.RouterGroup{users, conversations, calls} {
		if actor != nil {
			grp.Use(func(c *gin.Context) { c.Set(httpadapter.ContextUserIDKey, *actor); c.Next() })
		}
	}
	h.RegisterRoutes(users)
	h.RegisterConversationRoutes(conversations)
	h.RegisterCallRoutes(calls)
	return r
}

func doE2EEJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestE2EEDeviceHandler_Register_Created(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey := make([]byte, 32)
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	want := &domaine2ee.Device{ID: deviceID, UserID: actor, Fingerprint: "fp", Status: domaine2ee.StatusTrusted}
	h := NewE2EEDeviceHandler(&fakeRegister{fn: func(cmd in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
		if cmd.Actor != actor || cmd.DeviceID != deviceID {
			t.Errorf("cmd = %+v", cmd)
		}
		return want, true, nil
	}}, nil, nil, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	body := `{"id":"` + deviceID.String() + `","public_key":"` + base64.RawURLEncoding.EncodeToString(publicKey) + `","signing_public_key":"` + base64.RawURLEncoding.EncodeToString(signingPublicKey) + `"}`
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
}

func TestE2EEDeviceHandler_Register_IdempotentReplay(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	deviceID := uuid.Must(uuid.NewV7())
	publicKey := make([]byte, 32)
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	want := &domaine2ee.Device{ID: deviceID, UserID: actor, Fingerprint: "fp"}
	h := NewE2EEDeviceHandler(&fakeRegister{fn: func(in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
		return want, false, nil
	}}, nil, nil, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	body := `{"id":"` + deviceID.String() + `","public_key":"` + base64.RawURLEncoding.EncodeToString(publicKey) + `","signing_public_key":"` + base64.RawURLEncoding.EncodeToString(signingPublicKey) + `"}`
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an idempotent replay, body %s", w.Code, w.Body.String())
	}
}

func TestE2EEDeviceHandler_Register_InvalidPublicKeyLength(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := NewE2EEDeviceHandler(&fakeRegister{fn: func(in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
		t.Fatal("use case must not be called")
		return nil, false, nil
	}}, nil, nil, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	body := `{"id":"` + uuid.Must(uuid.NewV7()).String() + `","public_key":"` + base64.RawURLEncoding.EncodeToString(make([]byte, 31)) + `","signing_public_key":"` + base64.RawURLEncoding.EncodeToString(make([]byte, 32)) + `"}`
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestE2EEDeviceHandler_Register_IDConflict(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	publicKey := make([]byte, 32)
	signingPublicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	h := NewE2EEDeviceHandler(&fakeRegister{fn: func(in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
		return nil, false, command.ErrDeviceIDConflict
	}}, nil, nil, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	body := `{"id":"` + uuid.Must(uuid.NewV7()).String() + `","public_key":"` + base64.RawURLEncoding.EncodeToString(publicKey) + `","signing_public_key":"` + base64.RawURLEncoding.EncodeToString(signingPublicKey) + `"}`
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestE2EEDeviceHandler_List(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := NewE2EEDeviceHandler(nil, nil, nil, &fakeListDevices{fn: func(q in.ListDevicesQuery) ([]*domaine2ee.Device, error) {
		return []*domaine2ee.Device{{ID: uuid.Must(uuid.NewV7()), UserID: actor, PublicKey: []byte{1, 2, 3}}}, nil
	}}, nil, nil)
	r := e2eeRouter(h, &actor)

	w := doE2EEJSON(t, r, http.MethodGet, "/users/me/e2ee/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Devices []map[string]any `json:"devices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Devices) != 1 {
		t.Fatalf("len = %d, want 1", len(body.Devices))
	}
	if _, has := body.Devices[0]["public_key"]; has {
		t.Error("List must omit public_key (unlike ListConversationDevices)")
	}
}

func TestE2EEDeviceHandler_ListConversationDevices_IncludesPublicKey(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	pubKey := []byte{9, 9, 9}
	h := NewE2EEDeviceHandler(nil, nil, nil, nil, &fakeListConversationDevices{fn: func(q in.ListConversationDevicesQuery) ([]*domaine2ee.Device, error) {
		if q.ConversationID != convID {
			t.Errorf("ConversationID = %v, want %v", q.ConversationID, convID)
		}
		return []*domaine2ee.Device{{ID: uuid.Must(uuid.NewV7()), PublicKey: pubKey}}, nil
	}}, nil)
	r := e2eeRouter(h, &actor)

	w := doE2EEJSON(t, r, http.MethodGet, "/conversations/"+convID.String()+"/e2ee/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Devices []struct {
			PublicKey string `json:"public_key"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Devices) != 1 || body.Devices[0].PublicKey != base64.RawURLEncoding.EncodeToString(pubKey) {
		t.Fatalf("devices = %+v", body.Devices)
	}
}

func TestE2EEDeviceHandler_ListConversationDevices_NotMember(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	h := NewE2EEDeviceHandler(nil, nil, nil, nil, &fakeListConversationDevices{fn: func(in.ListConversationDevicesQuery) ([]*domaine2ee.Device, error) {
		return nil, query.ErrNotConversationMember
	}}, nil)
	r := e2eeRouter(h, &actor)

	w := doE2EEJSON(t, r, http.MethodGet, "/conversations/"+uuid.Must(uuid.NewV7()).String()+"/e2ee/devices", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestE2EEDeviceHandler_Approve_NoContent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	targetID := uuid.Must(uuid.NewV7())
	approverID := uuid.Must(uuid.NewV7())
	h := NewE2EEDeviceHandler(nil, &fakeApprove{fn: func(cmd in.ApproveDeviceCommand) error {
		if cmd.TargetID != targetID || cmd.ApproverID != approverID {
			t.Errorf("cmd = %+v", cmd)
		}
		return nil
	}}, nil, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	sig := base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	body := `{"approver_id":"` + approverID.String() + `","signature":"` + sig + `"}`
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices/"+targetID.String()+"/approve", body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body %s", w.Code, w.Body.String())
	}
}

func TestE2EEDeviceHandler_Approve_ErrorMapping(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	targetID := uuid.Must(uuid.NewV7())
	approverID := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not trusted", command.ErrApproverNotTrusted, http.StatusForbidden},
		{"pending not found", command.ErrPendingDeviceNotFound, http.StatusNotFound},
		{"invalid signature", command.ErrInvalidApprovalSignature, http.StatusForbidden},
		{"cannot approve", command.ErrCannotApprove, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewE2EEDeviceHandler(nil, &fakeApprove{fn: func(in.ApproveDeviceCommand) error { return tc.err }}, nil, nil, nil, nil)
			r := e2eeRouter(h, &actor)
			sig := base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			body := `{"approver_id":"` + approverID.String() + `","signature":"` + sig + `"}`
			w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices/"+targetID.String()+"/approve", body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestE2EEDeviceHandler_PutCallKeyEnvelopes_NoContent(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	h := NewE2EEDeviceHandler(nil, nil, &fakePutCallKeyEnvelopes{fn: func(cmd in.PutCallKeyEnvelopesCommand) error {
		if cmd.CallID != callID || cmd.OwnerDeviceID != ownerID || len(cmd.Envelopes) != 1 || cmd.Envelopes[0].RecipientDeviceID != recipientID {
			t.Errorf("cmd = %+v", cmd)
		}
		return nil
	}}, nil, nil, nil)
	r := e2eeRouter(h, &actor)

	ct := base64.RawURLEncoding.EncodeToString([]byte("ciphertext"))
	body := `{"owner_device_id":"` + ownerID.String() + `","envelopes":[{"recipient_device_id":"` + recipientID.String() + `","ciphertext":"` + ct + `"}]}`
	w := doE2EEJSON(t, r, http.MethodPut, "/calls/"+callID.String()+"/e2ee/key-envelopes", body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body %s", w.Code, w.Body.String())
	}
}

func TestE2EEDeviceHandler_PutCallKeyEnvelopes_ErrorMapping(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"call not found", command.ErrCallNotFound, http.StatusNotFound},
		{"not member", command.ErrNotConversationMember, http.StatusForbidden},
		{"not initiator", command.ErrNotCallInitiator, http.StatusForbidden},
		{"owner not trusted", command.ErrOwnerNotTrusted, http.StatusForbidden},
		{"invalid envelope", command.ErrInvalidKeyEnvelope, http.StatusBadRequest},
		{"missing envelope", command.ErrMissingKeyEnvelope, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewE2EEDeviceHandler(nil, nil, &fakePutCallKeyEnvelopes{fn: func(in.PutCallKeyEnvelopesCommand) error { return tc.err }}, nil, nil, nil)
			r := e2eeRouter(h, &actor)
			body := `{"owner_device_id":"` + uuid.Must(uuid.NewV7()).String() + `","envelopes":[]}`
			w := doE2EEJSON(t, r, http.MethodPut, "/calls/"+callID.String()+"/e2ee/key-envelopes", body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestE2EEDeviceHandler_GetCallKeyEnvelope_OK(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	recipientID := uuid.Must(uuid.NewV7())
	ownerID := uuid.Must(uuid.NewV7())
	ciphertext := []byte("ct")
	ownerPubKey := []byte{7, 7, 7}
	h := NewE2EEDeviceHandler(nil, nil, nil, nil, nil, &fakeGetCallKeyEnvelope{fn: func(q in.GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error) {
		if q.CallID != callID || q.RecipientDeviceID != recipientID {
			t.Errorf("q = %+v", q)
		}
		return &domaine2ee.KeyEnvelope{OwnerDeviceID: ownerID, Ciphertext: ciphertext}, &domaine2ee.Device{PublicKey: ownerPubKey}, nil
	}})
	r := e2eeRouter(h, &actor)

	w := doE2EEJSON(t, r, http.MethodGet, "/calls/"+callID.String()+"/e2ee/key-envelope?device_id="+recipientID.String(), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["owner_device_id"] != ownerID.String() || body["owner_public_key"] != base64.RawURLEncoding.EncodeToString(ownerPubKey) || body["ciphertext"] != base64.RawURLEncoding.EncodeToString(ciphertext) {
		t.Fatalf("body = %v", body)
	}
}

func TestE2EEDeviceHandler_GetCallKeyEnvelope_ErrorMapping(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	callID := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"call not found", query.ErrCallNotFound, http.StatusNotFound},
		{"not member", query.ErrNotConversationMember, http.StatusForbidden},
		{"recipient not trusted", query.ErrRecipientNotTrusted, http.StatusForbidden},
		{"key not ready", query.ErrKeyNotReady, http.StatusNotFound},
		{"owner no longer trusted", query.ErrOwnerNoLongerTrusted, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewE2EEDeviceHandler(nil, nil, nil, nil, nil, &fakeGetCallKeyEnvelope{fn: func(in.GetCallKeyEnvelopeQuery) (*domaine2ee.KeyEnvelope, *domaine2ee.Device, error) {
				return nil, nil, tc.err
			}})
			r := e2eeRouter(h, &actor)
			w := doE2EEJSON(t, r, http.MethodGet, "/calls/"+callID.String()+"/e2ee/key-envelope?device_id="+uuid.Must(uuid.NewV7()).String(), "")
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestE2EEDeviceHandler_Register_NoAuthContext(t *testing.T) {
	h := NewE2EEDeviceHandler(&fakeRegister{fn: func(in.RegisterDeviceCommand) (*domaine2ee.Device, bool, error) {
		t.Fatal("use case must not be called")
		return nil, false, nil
	}}, nil, nil, nil, nil, nil)
	r := e2eeRouter(h, nil)
	w := doE2EEJSON(t, r, http.MethodPost, "/users/me/e2ee/devices", `{"id":"x","public_key":"x","signing_public_key":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
