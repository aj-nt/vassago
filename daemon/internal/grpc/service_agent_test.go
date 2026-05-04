// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)



func TestRegisterAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		agentID     string
		expectError bool
		wantCode    codes.Code
	}{
		{"valid agent", "test-agent", false, codes.OK},
		{"empty agent ID", "", true, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStoreSvc(t)
			svc := NewService(s, nil, nil)
			ctx := context.Background()

			req := &pb.RegisterAgentRequest{
				AgentId: tt.agentID,
				Name:    "Test Agent",
				Role:    "developer",
			}

			resp, err := svc.RegisterAgent(ctx, req)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if status.Code(err) != tt.wantCode {
					t.Errorf("Expected error code %v, got %v", tt.wantCode, status.Code(err))
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if resp.AgentId != tt.agentID {
					t.Errorf("Expected agent ID %q, got %q", tt.agentID, resp.AgentId)
				}
				if !resp.IsNew {
					t.Error("Expected IsNew to be true for new agent")
				}
			}
		})
	}
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		agentID     string
		expectError bool
		wantCode    codes.Code
	}{
		{"valid agent", "test-agent", false, codes.OK},
		{"empty agent ID", "", true, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStoreSvc(t)
			svc := NewService(s, nil, nil)
			ctx := context.Background()

			_, err := svc.RegisterAgent(ctx, &pb.RegisterAgentRequest{
				AgentId: tt.agentID,
				Name:    "Test Agent",
				Role:    "developer",
			})
			if err != nil && !tt.expectError {
				t.Fatalf("Failed to register agent: %v", err)
			}

			req := &pb.AgentId{AgentId: tt.agentID}
			resp, err := svc.Heartbeat(ctx, req)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if status.Code(err) != tt.wantCode {
					t.Errorf("Expected error code %v, got %v", tt.wantCode, status.Code(err))
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if resp == nil {
					t.Error("Expected non-nil response")
				}
			}
		})
	}
}

func TestRegisterAgent_Duplicate(t *testing.T) {
	t.Parallel()

	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// First registration
	req1 := &pb.RegisterAgentRequest{
		AgentId: "test-agent",
		Name:    "Test Agent",
		Role:    "developer",
	}
	resp1, err := svc.RegisterAgent(ctx, req1)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}
	if !resp1.IsNew {
		t.Error("Expected first registration to be new")
	}

	// Second registration with same ID
	req2 := &pb.RegisterAgentRequest{
		AgentId: "test-agent",
		Name:    "Test Agent 2",
		Role:    "developer",
	}
	resp2, err := svc.RegisterAgent(ctx, req2)
	if err != nil {
		t.Fatalf("Second registration failed: %v", err)
	}
	if resp2.IsNew {
		t.Error("Expected second registration to not be new")
	}
	if resp2.AgentId != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %q", resp2.AgentId)
	}
}