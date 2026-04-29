package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// APIKeyInterceptor checks for a valid API key in the request metadata.
func APIKeyInterceptor(apiKey string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip authentication for health check endpoints
		if info.FullMethod == "/mnemosyne.Mnemosyne/Health" {
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

		// Check for API key in headers
		values := md.Get("x-api-key")
		if len(values) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "API key not provided")
		}

		// Validate API key
		if values[0] != apiKey {
			return nil, status.Errorf(codes.Unauthenticated, "invalid API key")
		}

		// Proceed with the handler
		return handler(ctx, req)
	}
}

// StreamAPIKeyInterceptor checks for a valid API key in stream request metadata.
func StreamAPIKeyInterceptor(apiKey string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip authentication for health check endpoints
		if info.FullMethod == "/mnemosyne.Mnemosyne/Health" {
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

		// Check for API key in headers
		values := md.Get("x-api-key")
		if len(values) == 0 {
			return status.Errorf(codes.Unauthenticated, "API key not provided")
		}

		// Validate API key
		if values[0] != apiKey {
			return status.Errorf(codes.Unauthenticated, "invalid API key")
		}

		// Proceed with the handler
		return handler(srv, ss)
	}
}