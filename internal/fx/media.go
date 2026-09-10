package fxapp

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"
	"google.golang.org/grpc"

	rpcadapter "pet-messenger/internal/adapter/rpc"
	"pet-messenger/internal/adapter/rpc/mediav1"
	"pet-messenger/internal/adapter/sfu"
	"pet-messenger/internal/config"
	infraauth "pet-messenger/internal/infrastructure/auth"
	medianodeinfra "pet-messenger/internal/infrastructure/medianode"
	natsinfra "pet-messenger/internal/infrastructure/nats"
	usecasecall "pet-messenger/internal/usecase/call"
)

// --- media-side NATS: publisher-only core connection ---

func newMediaNATSConn(lc fx.Lifecycle, cfg *config.Config) (*natsgo.Conn, error) {
	nc, err := natsgo.Connect(cfg.NATS.URL,
		natsgo.Name("pet-messenger-media"),
		natsgo.RetryOnFailedConnect(true),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("media nats connect %s: %w", cfg.NATS.URL, err)
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return nc.Drain() }})
	return nc, nil
}

var MediaNATSModule = fx.Module("media-nats",
	fx.Provide(
		newMediaNATSConn,
		natsinfra.NewCallEventPublisher,
		func(p *natsinfra.CallEventPublisher) sfu.CallEventPublisher { return p },
	),
)

var JoinTicketModule = fx.Module("join-ticket",
	fx.Provide(
		infraauth.NewJoinTicketVerifier,
	),
)

var SFUModule = fx.Module("sfu",
	fx.Provide(sfu.NewManager),
)

// --- gRPC server ---

func newGRPCServer(lc fx.Lifecycle, cfg *config.Config, roomSrv *rpcadapter.RoomControlServer) *grpc.Server {
	srv := grpc.NewServer(grpc.UnaryInterceptor(rpcadapter.UnaryServerAuthInterceptor(cfg.Media.RPCToken)))
	mediav1.RegisterRoomControlServer(srv, roomSrv)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			lis, err := net.Listen("tcp", grpcListenAddr(cfg))
			if err != nil {
				return fmt.Errorf("grpc listen: %w", err)
			}
			go func() {
				if err := srv.Serve(lis); err != nil {
					log.Printf("grpc: serve: %v", err)
				}
			}()
			log.Printf("grpc: RoomControl on %s", lis.Addr())
			return nil
		},
		OnStop: func(context.Context) error { srv.GracefulStop(); return nil },
	})
	return srv
}

// grpcListenAddr: bind to the port of MEDIA_GRPC_ADDR (host part is the
// advertised address, which may be a k8s service name that is not locally
// bindable), or ":9090" if unset.
func grpcListenAddr(cfg *config.Config) string {
	if cfg.Media.GRPCAddr == "" {
		return ":9090"
	}
	_, port, err := net.SplitHostPort(cfg.Media.GRPCAddr)
	if err != nil {
		return ":9090"
	}
	return ":" + port
}

var RPCServerModule = fx.Module("rpc-server",
	fx.Provide(rpcadapter.NewRoomControlServer, newGRPCServer),
	fx.Invoke(func(*grpc.Server) {}),
)

// --- media-node registry heartbeat ---

func mediaNodeIdentity(cfg *config.Config) usecasecall.Node {
	id := cfg.Media.NodeID
	if id == "" {
		if h, err := os.Hostname(); err == nil {
			id = h
		} else {
			id = "media-unknown"
		}
	}
	return usecasecall.Node{ID: id, GRPCAddr: cfg.Media.GRPCAddr, WSAddr: cfg.Media.WSAddr}
}

func startMediaNodeHeartbeat(lc fx.Lifecycle, cfg *config.Config, reg *medianodeinfra.RedisRegistry, mgr sfu.Manager) {
	self := mediaNodeIdentity(cfg)
	cap := cfg.Media.RoomCapacity
	stop := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := reg.Register(ctx, self, cap); err != nil {
				return fmt.Errorf("media node register: %w", err)
			}
			go func() {
				t := time.NewTicker(5 * time.Second)
				defer t.Stop()
				for {
					select {
					case <-stop:
						return
					case <-t.C:
						tot := mgr.Totals()
						_ = reg.Heartbeat(context.Background(), self, cap,
							medianodeinfra.Load{Rooms: tot.Rooms, Participants: tot.Participants})
					}
				}
			}()
			log.Printf("media: registered node %s (grpc %s, ws %s)", self.ID, self.GRPCAddr, self.WSAddr)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(stop)
			return reg.Deregister(ctx, self.ID)
		},
	})
}

var MediaNodeModule = fx.Module("media-node",
	fx.Provide(medianodeinfra.NewRedisRegistry),
	fx.Invoke(startMediaNodeHeartbeat),
)
