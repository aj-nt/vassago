// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

// ============================================================================
// Phase 28: Namespace — gRPC interceptor tests
// ============================================================================

func TestOwnerFromCtx_Default(t *testing.T) {
	ctx := context.Background()
	owner := ownerFromCtx(ctx)
	if owner != "default" {
		t.Errorf("ownerFromCtx() = %q, want %q", owner, "default")
	}
}

func TestOwnerFromCtx_CustomNamespace(t *testing.T) {
	md := metadata.Pairs("x-vassago-namespace", "tenant-42")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	owner := ownerFromCtx(ctx)
	if owner != "tenant-42" {
		t.Errorf("ownerFromCtx() = %q, want %q", owner, "tenant-42")
	}
}

func TestOwnerFromCtx_EmptyValue(t *testing.T) {
	md := metadata.Pairs("x-vassago-namespace", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	owner := ownerFromCtx(ctx)
	if owner != "default" {
		t.Errorf("ownerFromCtx() with empty value = %q, want %q", owner, "default")
	}
}

func TestOwnerFromCtx_NoMetadata(t *testing.T) {
	ctx := context.Background()
	owner := ownerFromCtx(ctx)
	if owner != "default" {
		t.Errorf("ownerFromCtx() without metadata = %q, want %q", owner, "default")
	}
}

func TestOwnerFromCtx_MultipleValues(t *testing.T) {
	md := metadata.Pairs("x-vassago-namespace", "first", "x-vassago-namespace", "second")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	owner := ownerFromCtx(ctx)
	// Should take the first value
	if owner != "first" {
		t.Errorf("ownerFromCtx() with multiple values = %q, want %q", owner, "first")
	}
}

func TestOwnerFromCtx_WithAuthHeader(t *testing.T) {
	// Namespace should still work even with other metadata present
	md := metadata.Pairs(
		"authorization", "Bearer sk-test-123",
		"x-vassago-namespace", "alice",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	owner := ownerFromCtx(ctx)
	if owner != "alice" {
		t.Errorf("ownerFromCtx() = %q, want %q", owner, "alice")
	}
}
