// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/vassago/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// ── Ping ──

func TestPing_Healthy(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.Ping(ctx, &pb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Greater(t, resp.Timestamp, int64(0))
}

func TestPing_StoreDown(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	svc.store.Close()
	ctx := context.Background()

	_, err := svc.Ping(ctx, &pb.HealthCheckRequest{})
	require.Error(t, err)
}

// ── SetSummaryClient ──

func TestSetSummaryClient(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	assert.Nil(t, svc.summaryClient)

	mock := &mockLLMClient{result: "summary"}
	svc.SetSummaryClient(mock)
	assert.NotNil(t, svc.summaryClient)
	assert.Equal(t, mock, svc.summaryClient)
}

type mockLLMClient struct {
	result string
	err    error
}

func (m *mockLLMClient) Complete(prompt string) (string, error) {
	return m.result, m.err
}

// ── Interceptors function ──

func TestInterceptors_WithAPIKey(t *testing.T) {
	unary, stream := Interceptors("secret-key")
	assert.NotNil(t, unary)
	assert.NotNil(t, stream)
}

func TestInterceptors_EmptyAPIKey(t *testing.T) {
	unary, stream := Interceptors("")
	assert.NotNil(t, unary)
	assert.NotNil(t, stream)
}

// ── NamespaceUnaryInterceptor ──

func TestNamespaceUnaryInterceptor_PassThrough(t *testing.T) {
	handler := func(ctx context.Context, req any) (any, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test/Method"}
	resp, err := NamespaceUnaryInterceptor(context.Background(), "req", info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "response", resp)
}
