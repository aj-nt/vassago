// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"fmt"
	"io"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/metadata"
)

// nullSubscribeStream is a no-op implementation of pb.Mnemosyne_SubscribeClient
// (grpc.ServerStreamingClient[UpdateEvent]). Recv() returns io.EOF immediately,
// signaling that the stream has ended — which is the correct behavior when
// there is no daemon to subscribe to.
type nullSubscribeStream struct{}

func (n *nullSubscribeStream) Recv() (*pb.UpdateEvent, error) {
	return nil, io.EOF
}

func (n *nullSubscribeStream) Header() (metadata.MD, error)    { return nil, nil }
func (n *nullSubscribeStream) Trailer() metadata.MD            { return nil }
func (n *nullSubscribeStream) CloseSend() error                { return nil }
func (n *nullSubscribeStream) Context() context.Context        { return context.Background() }
func (n *nullSubscribeStream) SendMsg(m interface{}) error    { return nil }
func (n *nullSubscribeStream) RecvMsg(m interface{}) error     { return io.EOF }

// NullMnemo is a no-op implementation of MnemoClient. It is used when the
// Mnemosyne daemon is unavailable, eliminating the need for nil checks
// throughout the codebase. Methods that are "fire-and-forget" (session
// lifecycle, heartbeat, close) silently succeed. Methods that return data
// return empty results. Methods that perform writes against the daemon
// return "mnemo not connected" errors so callers can distinguish success
// from unavailable-daemon.
type NullMnemo struct{}

// Ensure NullMnemo implements MnemoClient at compile time.
var _ MnemoClient = (*NullMnemo)(nil)

func (n *NullMnemo) RegisterAgent(ctx context.Context, agentID, name, role string) (string, bool, error) {
	return "", false, nil
}

func (n *NullMnemo) Heartbeat(ctx context.Context, agentID string) error {
	return nil
}

func (n *NullMnemo) Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
	return &nullSubscribeStream{}, nil
}

func (n *NullMnemo) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) RemoveMemory(ctx context.Context, id string) error {
	return fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error) {
	return nil, nil
}

func (n *NullMnemo) ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	return nil, nil
}

func (n *NullMnemo) GetHotBlock(ctx context.Context, target, agentID string, charLimit int32, boostCategories, suppressCategories []string, boostPriority int32) (string, error) {
	return "", nil
}

func (n *NullMnemo) CreateSession(ctx context.Context, agentID, source, title string) (string, error) {
	return "", nil
}

func (n *NullMnemo) AddMessages(ctx context.Context, sessionID string, messages []Message) error {
	return nil
}

func (n *NullMnemo) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (n *NullMnemo) Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error) {
	return &pb.ConsolidateResponse{}, nil
}

func (n *NullMnemo) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) SearchSessions(ctx context.Context, query string, limit int32) ([]SessionInfo, error) {
	return nil, nil
}

func (n *NullMnemo) ListRecentSessions(ctx context.Context, limit int32) ([]SessionInfo, error) {
	return nil, nil
}

func (n *NullMnemo) GetSession(ctx context.Context, sessionID string) (*SessionDetail, error) {
	return nil, nil
}

func (n *NullMnemo) SendMessage(ctx context.Context, platform, target, message string) (string, error) {
	return "", fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error) {
	return nil, nil
}

func (n *NullMnemo) AddTodo(ctx context.Context, content string, priority int32, sourceAgent string) (*TodoItem, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) ListTodos(ctx context.Context, includeCompleted bool, sourceAgent string) ([]TodoItem, error) {
	return nil, nil
}

func (n *NullMnemo) CompleteTodo(ctx context.Context, id string) (*TodoItem, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) RemoveTodo(ctx context.Context, id string) error {
	return fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) CreateCronJob(ctx context.Context, name, schedule, agentID, message string, enabled *bool) (*CronJobInfo, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) ListCronJobs(ctx context.Context, agentID string, includeDisabled bool) ([]CronJobInfo, error) {
	return nil, nil
}

func (n *NullMnemo) UpdateCronJob(ctx context.Context, id string, name, schedule, message *string, enabled *bool) (*CronJobInfo, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) DeleteCronJob(ctx context.Context, id string) error {
	return fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) AddSkill(ctx context.Context, name, description string, triggers []string, category, content, sourceAgent string, priority int32) (*pb.SkillEntry, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) GetSkill(ctx context.Context, id string) (*pb.SkillEntry, error) {
	return nil, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) ListSkills(ctx context.Context, category, sourceAgent string, limit int32) ([]*pb.SkillEntry, error) {
	return nil, nil
}

func (n *NullMnemo) RemoveSkill(ctx context.Context, id string) (bool, error) {
	return false, fmt.Errorf("mnemo not connected")
}

func (n *NullMnemo) SearchSkills(ctx context.Context, query, category string, limit int32) ([]*pb.SkillEntry, error) {
	return nil, nil
}

func (n *NullMnemo) Close() error {
	return nil
}

// IsConnected returns false for NullMnemo, indicating the daemon is not available.
func (n *NullMnemo) IsConnected() bool { return false }

// IsConnected returns true for the real Client, indicating the daemon is connected.
func (c *Client) IsConnected() bool { return true }