// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestChainUnaryInterceptor(t *testing.T) {
	t.Parallel()

	// Mock interceptors
	interceptor1 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		return resp, err
	}

	interceptor2 := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		return resp, err
	}

	// Handler that returns a response
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	chained := ChainUnaryInterceptor(interceptor1, interceptor2)
	resp, err := chained(context.Background(), "request", &grpc.UnaryServerInfo{FullMethod: "TestMethod"}, handler)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp != "response" {
		t.Errorf("Expected 'response', got %v", resp)
	}
}

func TestChainStreamInterceptor(t *testing.T) {
	t.Parallel()

	// Mock interceptors
	interceptor1 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	interceptor2 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	// Handler that returns no error
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	chained := ChainStreamInterceptor(interceptor1, interceptor2)

	// Mock server stream
	mockStream := &mockServerStream{}
	err := chained(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "TestStreamMethod"}, handler)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestUnaryInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handlerErr error
		wantError  bool
	}{
		{"success", nil, false},
		{"error", status.Error(codes.Internal, "test error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return "response", tt.handlerErr
			}

			resp, err := UnaryInterceptor(context.Background(), "request", &grpc.UnaryServerInfo{FullMethod: "TestMethod"}, handler)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if resp != "response" {
					t.Errorf("Expected 'response', got %v", resp)
				}
			}
		})
	}
}

func TestStreamInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handlerErr error
		wantError  bool
	}{
		{"success", nil, false},
		{"error", status.Error(codes.Internal, "test error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(srv interface{}, stream grpc.ServerStream) error {
				return tt.handlerErr
			}

			mockStream := &mockServerStream{}
			err := StreamInterceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "TestStreamMethod"}, handler)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestRecoveryInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   grpc.UnaryHandler
		wantError bool
	}{
		{
			name: "success",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				return "response", nil
			},
			wantError: false,
		},
		{
			name: "panic",
			handler: func(ctx context.Context, req interface{}) (interface{}, error) {
				panic("test panic")
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := RecoveryInterceptor(context.Background(), "request", &grpc.UnaryServerInfo{FullMethod: "TestMethod"}, tt.handler)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error from panic recovery, got nil")
				}
				if status.Code(err) != codes.Internal {
					t.Errorf("Expected Internal error code, got %v", status.Code(err))
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if resp != "response" {
					t.Errorf("Expected 'response', got %v", resp)
				}
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0sms"},
		{time.Millisecond, "1nsms"},
		{time.Second, "1s"},
		{time.Minute, "1m0s"},
		{time.Hour, "1h0m0s"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.duration), func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}
