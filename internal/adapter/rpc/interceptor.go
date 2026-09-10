package rpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const authMetadataKey = "x-media-rpc-token"

// UnaryClientAuthInterceptor attaches the shared media RPC token to every
// outgoing unary call as metadata.
func UnaryClientAuthInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, authMetadataKey, token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerAuthInterceptor rejects any unary call whose metadata is missing
// the shared media RPC token, carries the wrong one, or when the server has no
// token configured.
func UnaryServerAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		got := md.Get(authMetadataKey)
		if len(got) == 0 || got[0] != token || token == "" {
			return nil, status.Error(codes.Unauthenticated, "bad or missing media rpc token")
		}
		return handler(ctx, req)
	}
}
