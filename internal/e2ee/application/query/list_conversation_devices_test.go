// internal/e2ee/application/query/list_conversation_devices_test.go
package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

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

type fakeConversationMemberChecker struct {
	member *domainconv.Member
	err    error
}

func (f *fakeConversationMemberChecker) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domainconv.Member, error) {
	return f.member, f.err
}

func TestListConversationDevices_HappyPath(t *testing.T) {
	actor := uuid.Must(uuid.NewV7())
	convID := uuid.Must(uuid.NewV7())
	want := []*domaine2ee.Device{{ID: uuid.Must(uuid.NewV7())}}
	h := NewListConversationDevicesHandler(&fakeTrustedDeviceLister{byConv: map[uuid.UUID][]*domaine2ee.Device{convID: want}}, &fakeConversationMemberChecker{member: &domainconv.Member{}})

	got, err := h.Handle(context.Background(), in.ListConversationDevicesQuery{Actor: actor, ConversationID: convID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListConversationDevices_NotMember(t *testing.T) {
	h := NewListConversationDevicesHandler(&fakeTrustedDeviceLister{}, &fakeConversationMemberChecker{err: domainconv.ErrNotMember})

	_, err := h.Handle(context.Background(), in.ListConversationDevicesQuery{Actor: uuid.Must(uuid.NewV7()), ConversationID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, ErrNotConversationMember) {
		t.Fatalf("err = %v, want ErrNotConversationMember", err)
	}
}
