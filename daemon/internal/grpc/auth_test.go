// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAPIKeyInterceptor(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		metadata    metadata.MD
		expectError bool
		expectCode  codes.Code
	}{
		{
			name:        "no API key configured",
			apiKey:      "",
			metadata:    metadata.MD{},
			expectError: false,
		},
		{
			name:        "valid API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{"x-api-key": []string{"test-key"}},
			expectError: false,
		},
		{
			name:        "missing API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{},
			expectError: true,
			expectCode:  codes.Unauthenticated,
		},
		{
			name:        "invalid API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{"x-api-key": []string{"wrong-key"}},
			expectError: true,
			expectCode:  codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := APIKeyInterceptor(tt.apiKey)

			ctx := context.Background()
			if len(tt.metadata) > 0 {
				ctx = metadata.NewIncomingContext(ctx, tt.metadata)
			}

			// Mock handler
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return "success", nil
			}

			_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Method"}, handler)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				st, _ := status.FromError(err)
				if st.Code() != tt.expectCode {
					t.Errorf("expected code %v but got %v", tt.expectCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestStreamAPIKeyInterceptor(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		metadata    metadata.MD
		expectError bool
		expectCode  codes.Code
	}{
		{
			name:        "no API key configured",
			apiKey:      "",
			metadata:    metadata.MD{},
			expectError: false,
		},
		{
			name:        "valid API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{"x-api-key": []string{"test-key"}},
			expectError: false,
		},
		{
			name:        "missing API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{},
			expectError: true,
			expectCode:  codes.Unauthenticated,
		},
		{
			name:        "invalid API key",
			apiKey:      "test-key",
			metadata:    metadata.MD{"x-api-key": []string{"wrong-key"}},
			expectError: true,
			expectCode:  codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := StreamAPIKeyInterceptor(tt.apiKey)

			ctx := context.Background()
			if len(tt.metadata) > 0 {
				ctx = metadata.NewIncomingContext(ctx, tt.metadata)
			}

			// Mock stream
			mockStream := &mockServerStream{ctx: ctx}

			// Mock handler
			handler := func(srv interface{}, stream grpc.ServerStream) error {
				return nil
			}

			err := interceptor(nil, mockStream, &grpc.StreamServerInfo{FullMethod: "/test.Method"}, handler)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				st, _ := status.FromError(err)
				if st.Code() != tt.expectCode {
					t.Errorf("expected code %v but got %v", tt.expectCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// mockServerStream is a minimal implementation of grpc.ServerStream for testing
type mockServerStream struct {
	ctx context.Context
	grpc.ServerStream
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}
