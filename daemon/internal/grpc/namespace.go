// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const namespaceKey = "x-vassago-namespace"

// ownerFromCtx extracts the namespace (owner) from gRPC metadata.
// Falls back to "default" if no namespace header is present.
//
// Phase 28 (Namespace Isolation): foundation for multi-tenancy.
func ownerFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "default"
	}
	values := md.Get(namespaceKey)
	if len(values) == 0 {
		return "default"
	}
	owner := values[0]
	if owner == "" {
		return "default"
	}
	return owner
}

// NamespaceUnaryInterceptor is a pass-through interceptor hook for future
// namespace validation (Phase 28+).
func NamespaceUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return handler(ctx, req)
}
