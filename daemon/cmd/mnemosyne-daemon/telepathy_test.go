package main

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestTelepathy_agentReceivesUpdate(t *testing.T) {
	// Set up daemon with store + pub/sub hub
	s := testStoreTelepathy(t)
	hub := pubsub.NewHub()
	svc := daemongrpc.NewService(s, hub, nil)

	// Start gRPC server on random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	pb.RegisterMnemosyneServer(server, svc)

	go func() { _ = server.Serve(lis) }() //nolint:errcheck
	defer server.GracefulStop()

	// Connect subscriber client (Agent A)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewMnemosyneClient(conn)
	ctx := context.Background()

	// Agent A subscribes to "memory" target
	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
		AgentId: "agent-a",
		Targets: []string{"memory"},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Agent B writes a memory
	_, err = client.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "telepathy-test",
		Content:     "Agent B discovered something",
		Priority:    3,
		SourceAgent:  "agent-b",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Agent A should receive the update within 2 seconds
	recvCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, err := recvWithCtx(stream, recvCtx)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}

	if event.EventType != pb.UpdateEvent_ADDED {
		t.Errorf("expected ADDED event, got %v", event.EventType)
	}
	if event.Key != "telepathy-test" {
		t.Errorf("expected key=telepathy-test, got %s", event.Key)
	}
	if event.AgentId != "agent-b" {
		t.Errorf("expected agent_id=agent-b, got %s", event.AgentId)
	}
}

func TestTelepathy_noSelfNotification(t *testing.T) {
	s := testStoreTelepathy(t)
	hub := pubsub.NewHub()
	svc := daemongrpc.NewService(s, hub, nil)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	pb.RegisterMnemosyneServer(server, svc)

	go func() { _ = server.Serve(lis) }() //nolint:errcheck
	defer server.GracefulStop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewMnemosyneClient(conn)
	ctx := context.Background()

	// Agent A subscribes
	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
		AgentId: "agent-a",
		Targets: []string{"memory"},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Agent A writes a memory — should NOT receive its own event
	_, err = client.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "self-write",
		Content:     "My own write",
		Priority:    3,
		SourceAgent:  "agent-a",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Try to receive — should timeout (no self-notification)
	recvCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	_, err = recvWithCtx(stream, recvCtx)
	if err == nil {
		t.Error("should not receive own events")
	}
	// context.DeadlineExceeded is expected — it means no event arrived
}

func recvWithCtx(stream pb.Mnemosyne_SubscribeClient, ctx context.Context) (*pb.UpdateEvent, error) {
	ch := make(chan struct {
		evt *pb.UpdateEvent
		err error
	}, 1)
	go func() {
		evt, err := stream.Recv()
		ch <- struct {
			evt *pb.UpdateEvent
			err error
		}{evt, err}
	}()
	select {
	case result := <-ch:
		return result.evt, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func testStoreTelepathy(t *testing.T) *store.Store {
	t.Helper()
	tmpFile := t.TempDir() + "/telepathy-test.db"
	s, err := store.New(tmpFile)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
