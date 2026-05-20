// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// APIKeyInterceptor checks for a valid API key in the request metadata.
// Accepts both x-api-key and Authorization: Bearer <key> headers.
func APIKeyInterceptor(apiKey string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip authentication for health check endpoints
		if info.FullMethod == "/vassago.Vassago/Health" {
			return handler(ctx, req)
		}

		// If no API key is configured, skip authentication
		if apiKey == "" {
			return handler(ctx, req)
		}

		// Extract API key from metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "metadata not provided")
		}

		key := extractKey(md)
		if key == "" {
			return nil, status.Errorf(codes.Unauthenticated, "API key not provided")
		}
		if key != apiKey {
			return nil, status.Errorf(codes.Unauthenticated, "invalid API key")
		}

		return handler(ctx, req)
	}
}

// StreamAPIKeyInterceptor checks for a valid API key in stream request metadata.
// Accepts both x-api-key and Authorization: Bearer <key> headers.
func StreamAPIKeyInterceptor(apiKey string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip authentication for health check endpoints
		if info.FullMethod == "/vassago.Vassago/Health" {
			return handler(srv, ss)
		}

		// If no API key is configured, skip authentication
		if apiKey == "" {
			return handler(srv, ss)
		}

		// Extract API key from metadata
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Errorf(codes.Unauthenticated, "metadata not provided")
		}

		key := extractKey(md)
		if key == "" {
			return status.Errorf(codes.Unauthenticated, "API key not provided")
		}
		if key != apiKey {
			return status.Errorf(codes.Unauthenticated, "invalid API key")
		}

		return handler(srv, ss)
	}
}

// extractKey extracts the API key from gRPC metadata.
// Tries x-api-key first, then Authorization: Bearer <key>.
func extractKey(md metadata.MD) string {
	if values := md.Get("x-api-key"); len(values) > 0 {
		return values[0]
	}
	if values := md.Get("authorization"); len(values) > 0 {
		v := values[0]
		if after, ok := strings.CutPrefix(v, "Bearer "); ok {
			return after
		}
		return v
	}
	return ""
}
