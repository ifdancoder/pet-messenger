package rpc

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pet-messenger/internal/adapter/rpc/mediav1"
	"pet-messenger/internal/config"
	usecasecall "pet-messenger/internal/usecase/call"
)

type fakeRoomServer struct {
	mediav1.UnimplementedRoomControlServer
	created, closed, muted, removed []string
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

func (f *fakeRoomServer) RemoveParticipant(_ context.Context, r *mediav1.RemoveParticipantRequest) (*mediav1.RemoveParticipantResponse, error) {
	f.removed = append(f.removed, r.CallId+"/"+r.UserId)
	return &mediav1.RemoveParticipantResponse{}, nil
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
	if err := c.CreateRoom(context.Background(), usecasecall.Node{ID: "n", GRPCAddr: addr}, id); err != nil {
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
	err := c.CreateRoom(context.Background(), usecasecall.Node{GRPCAddr: addr}, uuid.Must(uuid.NewV7()))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}
