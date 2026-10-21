package rpc

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"pet-messenger/internal/platform/adapter/rpc/mediav1"
	"pet-messenger/internal/platform/adapter/sfu"
	"pet-messenger/internal/platform/config"
)

// nopEvents satisfies sfu.CallEventPublisher without recording anything.
type nopEvents struct{}

func (nopEvents) ParticipantJoined(_, _ uuid.UUID, _ string) {}
func (nopEvents) ParticipantLeft(_, _ uuid.UUID)             {}
func (nopEvents) Ended(_ uuid.UUID)                          {}

const testRPCToken = "sekret"

func startRoomControl(t *testing.T, cfg *config.Config) mediav1.RoomControlClient {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mgr, err := sfu.NewManager(cfg, nopEvents{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(UnaryServerAuthInterceptor(cfg.Media.RPCToken)))
	mediav1.RegisterRoomControlServer(srv, NewRoomControlServer(mgr))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientAuthInterceptor(cfg.Media.RPCToken)),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return mediav1.NewRoomControlClient(conn)
}

func TestRoomControlServer_Lifecycle(t *testing.T) {
	cfg := &config.Config{Media: config.MediaConfig{RPCToken: testRPCToken}}
	c := startRoomControl(t, cfg)
	ctx := context.Background()

	callID := uuid.Must(uuid.NewV7())
	if _, err := c.CreateRoom(ctx, &mediav1.CreateRoomRequest{CallId: callID.String()}); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	st, err := c.RoomStats(ctx, &mediav1.RoomStatsRequest{CallId: callID.String()})
	if err != nil {
		t.Fatalf("RoomStats: %v", err)
	}
	if !st.Exists || st.Participants != 0 {
		t.Fatalf("RoomStats after create = %+v, want {Exists:true Participants:0}", st)
	}

	// Mute / Remove against a room with no participants → NotFound.
	userID := uuid.Must(uuid.NewV7())
	if _, err := c.MuteParticipant(ctx, &mediav1.MuteParticipantRequest{CallId: callID.String(), UserId: userID.String()}); status.Code(err) != codes.NotFound {
		t.Fatalf("MuteParticipant = %v, want NotFound", err)
	}
	if _, err := c.UnmuteParticipant(ctx, &mediav1.UnmuteParticipantRequest{CallId: callID.String(), UserId: userID.String()}); status.Code(err) != codes.NotFound {
		t.Fatalf("UnmuteParticipant = %v, want NotFound", err)
	}
	if _, err := c.RemoveParticipant(ctx, &mediav1.RemoveParticipantRequest{CallId: callID.String(), UserId: userID.String()}); status.Code(err) != codes.NotFound {
		t.Fatalf("RemoveParticipant = %v, want NotFound", err)
	}

	// Mute against a room that does not exist → also NotFound.
	if _, err := c.MuteParticipant(ctx, &mediav1.MuteParticipantRequest{CallId: uuid.Must(uuid.NewV7()).String(), UserId: userID.String()}); status.Code(err) != codes.NotFound {
		t.Fatalf("MuteParticipant missing room = %v, want NotFound", err)
	}

	if _, err := c.CloseRoom(ctx, &mediav1.CloseRoomRequest{CallId: callID.String()}); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}
	st, err = c.RoomStats(ctx, &mediav1.RoomStatsRequest{CallId: callID.String()})
	if err != nil {
		t.Fatalf("RoomStats after close: %v", err)
	}
	if st.Exists {
		t.Fatalf("RoomStats after close = %+v, want Exists:false", st)
	}
}

func TestRoomControlServer_ListParticipants(t *testing.T) {
	cfg := &config.Config{Media: config.MediaConfig{RPCToken: testRPCToken}}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mgr, err := sfu.NewManager(cfg, nopEvents{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(UnaryServerAuthInterceptor(cfg.Media.RPCToken)))
	mediav1.RegisterRoomControlServer(srv, NewRoomControlServer(mgr))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientAuthInterceptor(cfg.Media.RPCToken)),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := mediav1.NewRoomControlClient(conn)
	ctx := context.Background()

	// A room that doesn't exist -> NotFound.
	callID := uuid.Must(uuid.NewV7())
	if _, err := c.ListParticipants(ctx, &mediav1.ListParticipantsRequest{CallId: callID.String()}); status.Code(err) != codes.NotFound {
		t.Fatalf("ListParticipants missing room = %v, want NotFound", err)
	}

	if err := mgr.CreateRoom(callID); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	userID := uuid.Must(uuid.NewV7())
	if err := mgr.Join(callID, userID, "publisher", nopSignaller{}); err != nil {
		t.Fatalf("join: %v", err)
	}
	resp, err := c.ListParticipants(ctx, &mediav1.ListParticipantsRequest{CallId: callID.String()})
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(resp.Participants) != 1 {
		t.Fatalf("Participants = %+v, want 1 entry", resp.Participants)
	}
	got := resp.Participants[0]
	if got.UserId != userID.String() || got.Role != "publisher" || got.Muted || got.SelfMuted {
		t.Fatalf("unexpected participant: %+v", got)
	}
}

// nopSignaller is a sfu.Signaller that drops everything on the floor.
type nopSignaller struct{}

func (nopSignaller) Send([]byte) error { return nil }
func (nopSignaller) Close()            {}

func TestRoomControlServer_BadID(t *testing.T) {
	cfg := &config.Config{Media: config.MediaConfig{RPCToken: testRPCToken}}
	c := startRoomControl(t, cfg)
	if _, err := c.CreateRoom(context.Background(), &mediav1.CreateRoomRequest{CallId: "not-a-uuid"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateRoom bad id = %v, want InvalidArgument", err)
	}
}

func TestRoomControlServer_Capacity(t *testing.T) {
	cfg := &config.Config{Media: config.MediaConfig{RPCToken: testRPCToken, RoomCapacity: 1}}
	c := startRoomControl(t, cfg)
	ctx := context.Background()

	if _, err := c.CreateRoom(ctx, &mediav1.CreateRoomRequest{CallId: uuid.Must(uuid.NewV7()).String()}); err != nil {
		t.Fatalf("first CreateRoom: %v", err)
	}
	_, err := c.CreateRoom(ctx, &mediav1.CreateRoomRequest{CallId: uuid.Must(uuid.NewV7()).String()})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("CreateRoom past capacity = %v, want ResourceExhausted", err)
	}
}

func TestRoomControlServer_BadToken(t *testing.T) {
	cfg := &config.Config{Media: config.MediaConfig{RPCToken: testRPCToken}}
	// Server trusts testRPCToken; client presents the wrong one.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mgr, err := sfu.NewManager(cfg, nopEvents{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(UnaryServerAuthInterceptor(cfg.Media.RPCToken)))
	mediav1.RegisterRoomControlServer(srv, NewRoomControlServer(mgr))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientAuthInterceptor("wrong")),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	_, err = mediav1.NewRoomControlClient(conn).CreateRoom(context.Background(), &mediav1.CreateRoomRequest{CallId: uuid.Must(uuid.NewV7()).String()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateRoom bad token = %v, want Unauthenticated", err)
	}
}
