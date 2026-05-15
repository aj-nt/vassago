// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ChainUnaryInterceptor chains multiple unary interceptors.
func ChainUnaryInterceptor(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var chainedHandler grpc.UnaryHandler = func(ctx context.Context, req interface{}) (interface{}, error) {
			return handler(ctx, req)
		}

		for i := len(interceptors) - 1; i >= 0; i-- {
			current := interceptors[i]
			prevHandler := chainedHandler
			chainedHandler = func(ctx context.Context, req interface{}) (interface{}, error) {
				return current(ctx, req, info, prevHandler)
			}
		}

		return chainedHandler(ctx, req)
	}
}

// ChainStreamInterceptor chains multiple stream interceptors.
func ChainStreamInterceptor(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var chainedHandler grpc.StreamHandler = func(srv interface{}, stream grpc.ServerStream) error {
			return handler(srv, stream)
		}

		for i := len(interceptors) - 1; i >= 0; i-- {
			current := interceptors[i]
			prevHandler := chainedHandler
			chainedHandler = func(srv interface{}, stream grpc.ServerStream) error {
				return current(srv, stream, info, prevHandler)
			}
		}

		return chainedHandler(srv, ss)
	}
}

// Interceptors returns gRPC unary and stream interceptors for logging, recovery, and authentication.
func Interceptors(apiKey string) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	return ChainUnaryInterceptor(UnaryInterceptor, APIKeyInterceptor(apiKey)),
		ChainStreamInterceptor(StreamInterceptor, StreamAPIKeyInterceptor(apiKey))
}

// UnaryInterceptor logs requests and recovers from panics.
func UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	if err != nil {
		st, _ := status.FromError(err)
		slog.Info("gRPC call",
			"method", info.FullMethod,
			"duration", formatDuration(duration),
			"code", st.Code(),
			"error", st.Message(),
		)
	} else {
		slog.Debug("gRPC call",
			"method", info.FullMethod,
			"duration", formatDuration(duration),
		)
	}

	return resp, err
}

// StreamInterceptor logs streaming requests and recovers from panics.
func StreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	duration := time.Since(start)

	if err != nil {
		st, _ := status.FromError(err)
		slog.Info("gRPC stream",
			"method", info.FullMethod,
			"duration", formatDuration(duration),
			"code", st.Code(),
			"error", st.Message(),
		)
	} else {
		slog.Debug("gRPC stream",
			"method", info.FullMethod,
			"duration", formatDuration(duration),
		)
	}

	return err
}

// RecoveryInterceptor recovers from panics in unary handlers.
func RecoveryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in gRPC handler", "method", info.FullMethod, "panic", r)
			err = status.Errorf(codes.Internal, "internal error: %v", r)
		}
	}()
	return handler(ctx, req)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return time.Duration(d.Milliseconds()).String() + "ms"
	}
	return d.Round(time.Millisecond).String()
}
