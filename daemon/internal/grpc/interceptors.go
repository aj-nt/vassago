package grpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Interceptors returns gRPC unary and stream interceptors for logging and recovery.
func Interceptors() (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	return UnaryInterceptor, StreamInterceptor
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