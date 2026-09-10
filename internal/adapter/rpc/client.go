// Package rpc is the gRPC transport for the media-service control plane. Its
// RoomControlClient dials media pods on demand and satisfies
// usecase/call.RoomControl; the auth interceptors guard both ends with a
// shared secret.
package rpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"pet-messenger/internal/adapter/rpc/mediav1"
	"pet-messenger/internal/config"
	usecasecall "pet-messenger/internal/usecase/call"
)

// RoomControlClient dials media pods on demand and caches one ClientConn per
// address. It satisfies usecase/call.RoomControl.
type RoomControlClient struct {
	token string
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// NewRoomControlClient builds a RoomControlClient that authenticates every call
// with cfg.Media.RPCToken.
func NewRoomControlClient(cfg *config.Config) *RoomControlClient {
	return &RoomControlClient{token: cfg.Media.RPCToken, conns: map[string]*grpc.ClientConn{}}
}

func (c *RoomControlClient) stub(addr string) (mediav1.RoomControlClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, ok := c.conns[addr]
	if !ok {
		var err error
		conn, err = grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(UnaryClientAuthInterceptor(c.token)),
		)
		if err != nil {
			return nil, fmt.Errorf("dial media %s: %w", addr, err)
		}
		c.conns[addr] = conn
	}
	return mediav1.NewRoomControlClient(conn), nil
}

// Close tears down every cached ClientConn.
func (c *RoomControlClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		_ = conn.Close()
	}
	c.conns = map[string]*grpc.ClientConn{}
}

func (c *RoomControlClient) call(ctx context.Context, node usecasecall.Node, fn func(mediav1.RoomControlClient, context.Context) error) error {
	stub, err := c.stub(node.GRPCAddr)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return fn(stub, cctx)
}

// CreateRoom asks the media pod to open the room for callID.
func (c *RoomControlClient) CreateRoom(ctx context.Context, node usecasecall.Node, callID uuid.UUID) error {
	return c.call(ctx, node, func(s mediav1.RoomControlClient, ctx context.Context) error {
		_, err := s.CreateRoom(ctx, &mediav1.CreateRoomRequest{CallId: callID.String()})
		return err
	})
}

// CloseRoom asks the media pod to tear down the room for callID.
func (c *RoomControlClient) CloseRoom(ctx context.Context, node usecasecall.Node, callID uuid.UUID) error {
	return c.call(ctx, node, func(s mediav1.RoomControlClient, ctx context.Context) error {
		_, err := s.CloseRoom(ctx, &mediav1.CloseRoomRequest{CallId: callID.String()})
		return err
	})
}

// MuteParticipant asks the media pod to mute userID in callID.
func (c *RoomControlClient) MuteParticipant(ctx context.Context, node usecasecall.Node, callID, userID uuid.UUID) error {
	return c.call(ctx, node, func(s mediav1.RoomControlClient, ctx context.Context) error {
		_, err := s.MuteParticipant(ctx, &mediav1.MuteParticipantRequest{CallId: callID.String(), UserId: userID.String()})
		return err
	})
}

// RemoveParticipant asks the media pod to drop userID from callID.
func (c *RoomControlClient) RemoveParticipant(ctx context.Context, node usecasecall.Node, callID, userID uuid.UUID) error {
	return c.call(ctx, node, func(s mediav1.RoomControlClient, ctx context.Context) error {
		_, err := s.RemoveParticipant(ctx, &mediav1.RemoveParticipantRequest{CallId: callID.String(), UserId: userID.String()})
		return err
	})
}

var _ usecasecall.RoomControl = (*RoomControlClient)(nil)
