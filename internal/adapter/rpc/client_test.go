package rpc

import (
	"context"
	"net"
	"slices"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"pet-messenger/internal/adapter/rpc/mediav1"
	callout "pet-messenger/internal/call/application/port/out"
	"pet-messenger/internal/config"
)

type fakeRoomServer struct {
	mediav1.UnimplementedRoomControlServer
	created, closed, muted, unmuted, removed []string
	participants                             []*mediav1.Participant
}

func (f *fakeRoomServer) CreateRoom(_ context.Context, r *mediav1.CreateRoomRequest) (*mediav1.CreateRoomResponse, error) {
	f.created = append(f.created, r.CallId)
	return &mediav1.CreateRoomResponse{}, nil
}

func (f *fakeRoomServer) CloseRoom(_ context.Context, r *mediav1.CloseRoomRequest) (*mediav1.CloseRoomResponse, error) {
	f.closed = append(f.closed, r.CallId)
	return &mediav1.CloseRoomResponse{}, nil
}

func (f *fakeRoomServer) MuteParticipant(_ context.Context, r *mediav1.MuteParticipantRequest) (*mediav1.MuteParticipantResponse, error) {
	f.muted = append(f.muted, r.CallId+"/"+r.UserId)
	return &mediav1.MuteParticipantResponse{}, nil
}

func (f *fakeRoomServer) UnmuteParticipant(_ context.Context, r *mediav1.UnmuteParticipantRequest) (*mediav1.UnmuteParticipantResponse, error) {
	f.unmuted = append(f.unmuted, r.CallId+"/"+r.UserId)
	return &mediav1.UnmuteParticipantResponse{}, nil
}

func (f *fakeRoomServer) RemoveParticipant(_ context.Context, r *mediav1.RemoveParticipantRequest) (*mediav1.RemoveParticipantResponse, error) {
	f.removed = append(f.removed, r.CallId+"/"+r.UserId)
	return &mediav1.RemoveParticipantResponse{}, nil
}

func (f *fakeRoomServer) ListParticipants(_ context.Context, _ *mediav1.ListParticipantsRequest) (*mediav1.ListParticipantsResponse, error) {
	return &mediav1.ListParticipantsResponse{Participants: f.participants}, nil
}

func startServer(t *testing.T, token string, impl mediav1.RoomControlServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(UnaryServerAuthInterceptor(token)))
	mediav1.RegisterRoomControlServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestRoomControlClient_CreateRoom(t *testing.T) {
	fake := &fakeRoomServer{}
	addr := startServer(t, "sekret", fake)
	c := NewRoomControlClient(&config.Config{Media: config.MediaConfig{RPCToken: "sekret"}})
	t.Cleanup(c.Close)

	id := uuid.Must(uuid.NewV7())
	if err := c.CreateRoom(context.Background(), callout.Node{ID: "n", GRPCAddr: addr}, id); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if len(fake.created) != 1 || fake.created[0] != id.String() {
		t.Fatalf("server saw %v", fake.created)
	}
}

func TestRoomControlClient_BadToken(t *testing.T) {
	addr := startServer(t, "right", &fakeRoomServer{})
	c := NewRoomControlClient(&config.Config{Media: config.MediaConfig{RPCToken: "wrong"}})
	t.Cleanup(c.Close)
	err := c.CreateRoom(context.Background(), callout.Node{GRPCAddr: addr}, uuid.Must(uuid.NewV7()))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestRoomControlClient_ListParticipants(t *testing.T) {
	fake := &fakeRoomServer{participants: []*mediav1.Participant{
		{UserId: "u1", Role: "publisher", Muted: true, SelfMuted: false, TrackKinds: []string{"mic", "camera"}},
	}}
	addr := startServer(t, "sekret", fake)
	c := NewRoomControlClient(&config.Config{Media: config.MediaConfig{RPCToken: "sekret"}})
	t.Cleanup(c.Close)

	callID := uuid.Must(uuid.NewV7())
	got, err := c.ListParticipants(context.Background(), callout.Node{GRPCAddr: addr}, callID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListParticipants = %+v, want 1 entry", got)
	}
	want := callout.ParticipantInfo{UserID: "u1", Role: "publisher", Muted: true, SelfMuted: false, TrackKinds: []string{"mic", "camera"}}
	if got[0].UserID != want.UserID || got[0].Role != want.Role || got[0].Muted != want.Muted || got[0].SelfMuted != want.SelfMuted || !slices.Equal(got[0].TrackKinds, want.TrackKinds) {
		t.Fatalf("ListParticipants[0] = %+v, want %+v", got[0], want)
	}
}

// TestRoomControlClient_ListParticipants_NilTrackKinds proves the client
// normalizes a nil TrackKinds — the shape protobuf always produces for a
// zero-track participant after a real marshal/unmarshal round-trip, since
// the wire format cannot distinguish an empty repeated field from an absent
// one — into a non-nil, empty slice. Without the fix, this nil would flow
// straight through to the HTTP layer and serialize as JSON `null`, which is
// exactly what crashed the frontend's CallPreviewBar.
func TestRoomControlClient_ListParticipants_NilTrackKinds(t *testing.T) {
	participant := &mediav1.Participant{UserId: "u2", Role: "publisher", Muted: false, SelfMuted: true}
	// Force a real protobuf wire round-trip rather than trusting the Go
	// struct literal's zero value: marshal then unmarshal, so TrackKinds
	// reflects exactly what protoc-gen-go produces for a zero-length
	// repeated field on the decode side (nil), not just an unset field the
	// test happened to construct that way.
	wire, err := proto.Marshal(participant)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	roundTripped := &mediav1.Participant{}
	if err := proto.Unmarshal(wire, roundTripped); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if roundTripped.TrackKinds != nil {
		t.Fatalf("test fixture invalid: want TrackKinds nil after wire round-trip, got %#v", roundTripped.TrackKinds)
	}

	fake := &fakeRoomServer{participants: []*mediav1.Participant{roundTripped}}
	addr := startServer(t, "sekret", fake)
	c := NewRoomControlClient(&config.Config{Media: config.MediaConfig{RPCToken: "sekret"}})
	t.Cleanup(c.Close)

	got, err := c.ListParticipants(context.Background(), callout.Node{GRPCAddr: addr}, uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListParticipants = %+v, want 1 entry", got)
	}
	if got[0].TrackKinds == nil {
		t.Fatalf("ListParticipants[0].TrackKinds = nil, want non-nil empty slice")
	}
	if len(got[0].TrackKinds) != 0 {
		t.Fatalf("ListParticipants[0].TrackKinds = %#v, want empty", got[0].TrackKinds)
	}
}

func TestRoomControlClient_UnmuteParticipant(t *testing.T) {
	fake := &fakeRoomServer{}
	addr := startServer(t, "secret", fake)
	c := NewRoomControlClient(&config.Config{Media: config.MediaConfig{RPCToken: "secret"}})
	t.Cleanup(c.Close)
	callID, userID := uuid.New(), uuid.New()
	if err := c.UnmuteParticipant(context.Background(), callout.Node{GRPCAddr: addr}, callID, userID); err != nil {
		t.Fatal(err)
	}
	if len(fake.unmuted) != 1 || fake.unmuted[0] != callID.String()+"/"+userID.String() {
		t.Fatalf("unmuted = %v", fake.unmuted)
	}
}
