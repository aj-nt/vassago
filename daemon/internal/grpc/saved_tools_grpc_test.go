// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/vassago/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errCode extracts the gRPC status code from an error, or OK if no error.
func errCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	s, ok := status.FromError(err)
	if !ok {
		return codes.Unknown
	}
	return s.Code()
}

func ptrInt32(v int32) *int32  { return &v }
func ptrStr(v string) *string { return &v }
func ptrBool(v bool) *bool    { return &v }

// ── AddSavedTool ──

func TestAddSavedTool_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "my-tool",
		Description: "A test tool",
		Type:        "script",
		Parameters:  `{"arg": "value"}`,
		Script:      "echo hello",
		Author:      "test-user",
		Category:    "utilities",
		Priority:    ptrInt32(3),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "my-tool", resp.Name)
	assert.Equal(t, "script", resp.Type)
	assert.Equal(t, int32(3), resp.Priority)
	assert.Equal(t, "test-user", resp.Author)
}

func TestAddSavedTool_DefaultsPriorityTo3(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "default-prio",
		Description: "test",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
		// Priority nil = zero = default to 3
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), resp.Priority, "nil/zero priority should default to 3")
}

func TestAddSavedTool_MissingFields(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name string
		req  *pb.AddSavedToolRequest
	}{
		{"empty name", &pb.AddSavedToolRequest{Description: "d", Type: "script", Parameters: "{}", Script: "echo", Author: "x"}},
		{"empty description", &pb.AddSavedToolRequest{Name: "n", Type: "script", Parameters: "{}", Script: "echo", Author: "x"}},
		{"empty type", &pb.AddSavedToolRequest{Name: "n", Description: "d", Parameters: "{}", Script: "echo", Author: "x"}},
		{"empty parameters", &pb.AddSavedToolRequest{Name: "n", Description: "d", Type: "script", Script: "echo", Author: "x"}},
		{"empty script", &pb.AddSavedToolRequest{Name: "n", Description: "d", Type: "script", Parameters: "{}", Author: "x"}},
		{"empty author", &pb.AddSavedToolRequest{Name: "n", Description: "d", Type: "script", Parameters: "{}", Script: "echo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddSavedTool(ctx, tt.req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, errCode(err))
		})
	}
}

func TestAddSavedTool_InvalidType(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "bad-type",
		Description: "test",
		Type:        "binary",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
	assert.Contains(t, err.Error(), "type must be 'script'")
}

func TestAddSavedTool_InvalidParametersJSON(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "bad-json",
		Description: "test",
		Type:        "script",
		Parameters:  `not json`,
		Script:      "echo",
		Author:      "user",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
	assert.Contains(t, err.Error(), "parameters must be valid JSON")
}

func TestAddSavedTool_PriorityOutOfRange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name     string
		priority *int32
		wantErr  bool
	}{
		{"negative", ptrInt32(-1), true},
		{"too high", ptrInt32(99), true},
		{"valid low", ptrInt32(1), false},
		{"valid high", ptrInt32(5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
				Name:        "range-" + tt.name,
				Description: "test",
				Type:        "script",
				Parameters:  `{}`,
				Script:      "echo",
				Author:      "user",
				Priority:    tt.priority,
			})
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, codes.InvalidArgument, errCode(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ── GetSavedTool ──

func TestGetSavedTool_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "get-me",
		Description: "test",
		Type:        "script",
		Parameters:  `{"x": 1}`,
		Script:      "echo test",
		Author:      "user",
		Priority:    ptrInt32(4),
	})
	require.NoError(t, err)

	getResp, err := svc.GetSavedTool(ctx, &pb.GetSavedToolRequest{Id: addResp.Id})
	require.NoError(t, err)
	assert.Equal(t, addResp.Id, getResp.Id)
	assert.Equal(t, "get-me", getResp.Name)
	assert.Equal(t, int32(4), getResp.Priority)
}

func TestGetSavedTool_MissingID(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.GetSavedTool(ctx, &pb.GetSavedToolRequest{Id: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
}

func TestGetSavedTool_NotFound(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.GetSavedTool(ctx, &pb.GetSavedToolRequest{Id: "nonexistent-id"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, errCode(err))
}

// ── ListSavedTools ──

func TestListSavedTools_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
			Name:        "tool-" + string(rune('a'+i)),
			Description: "list test",
			Type:        "script",
			Parameters:  `{}`,
			Script:      "echo",
			Author:      "user",
			Category:    "test-cat",
		})
		require.NoError(t, err)
	}

	resp, err := svc.ListSavedTools(ctx, &pb.ListSavedToolsRequest{
		Category: "test-cat",
		Limit:    ptrInt32(10),
	})
	require.NoError(t, err)
	assert.Len(t, resp.Tools, 2)
}

func TestListSavedTools_EmptyResult(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.ListSavedTools(ctx, &pb.ListSavedToolsRequest{
		Category: "nonexistent-category",
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Tools)
}

// ── SearchSavedTools ──

func TestSearchSavedTools_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "deploy-script",
		Description: "Production deployment tool",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "kubectl apply",
		Author:      "ops",
		Category:    "devops",
	})
	require.NoError(t, err)

	resp, err := svc.SearchSavedTools(ctx, &pb.SearchSavedToolsRequest{
		Query:    "deploy",
		Category: "devops",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Tools, 1)
	assert.Equal(t, "deploy-script", resp.Tools[0].Name)
}

func TestSearchSavedTools_EmptyQuery(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.SearchSavedTools(ctx, &pb.SearchSavedToolsRequest{Query: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
}

// ── UpdateSavedTool ──

func TestUpdateSavedTool_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "original",
		Description: "before update",
		Type:        "script",
		Parameters:  `{"v": 1}`,
		Script:      "echo old",
		Author:      "user",
	})
	require.NoError(t, err)

	resp, err := svc.UpdateSavedTool(ctx, &pb.UpdateSavedToolRequest{
		Id:          addResp.Id,
		Name:        ptrStr("updated"),
		Description: ptrStr("after update"),
		Script:      ptrStr("echo new"),
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", resp.Name)
	assert.Equal(t, "after update", resp.Description)
	assert.Equal(t, "echo new", resp.Script)
	assert.Equal(t, int32(3), resp.Priority)
}

func TestUpdateSavedTool_MissingID(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.UpdateSavedTool(ctx, &pb.UpdateSavedToolRequest{
		Id:   "",
		Name: ptrStr("x"),
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
}

func TestUpdateSavedTool_InvalidParametersJSON(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "json-test",
		Description: "test",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
	})
	require.NoError(t, err)

	_, err = svc.UpdateSavedTool(ctx, &pb.UpdateSavedToolRequest{
		Id:         addResp.Id,
		Parameters: ptrStr("not json"),
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
	assert.Contains(t, err.Error(), "parameters must be valid JSON")
}

func TestUpdateSavedTool_PriorityOutOfRange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "prio-test",
		Description: "test",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
	})
	require.NoError(t, err)

	_, err = svc.UpdateSavedTool(ctx, &pb.UpdateSavedToolRequest{
		Id:       addResp.Id,
		Priority: ptrInt32(99),
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
}

func TestUpdateSavedTool_AllUpdatableFields(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "all-fields",
		Description: "original",
		Type:        "script",
		Parameters:  `{"orig": true}`,
		Script:      "echo orig",
		Author:      "user",
		Category:    "cat-a",
	})
	require.NoError(t, err)

	resp, err := svc.UpdateSavedTool(ctx, &pb.UpdateSavedToolRequest{
		Id:                        addResp.Id,
		Name:                      ptrStr("renamed"),
		Description:               ptrStr("updated desc"),
		Parameters:                ptrStr(`{"new": true}`),
		Script:                    ptrStr("echo updated"),
		Category:                  ptrStr("cat-b"),
		Triggers:                  []string{"on_create", "on_update"},
		Priority:                  ptrInt32(5),
		RequiresApprovalEveryCall: ptrBool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, "renamed", resp.Name)
	assert.Equal(t, "updated desc", resp.Description)
	assert.Equal(t, `{"new": true}`, resp.Parameters)
	assert.Equal(t, "echo updated", resp.Script)
	assert.Equal(t, "cat-b", resp.Category)
	assert.Equal(t, []string{"on_create", "on_update"}, resp.Triggers)
	assert.Equal(t, int32(5), resp.Priority)
	assert.True(t, resp.RequiresApprovalEveryCall)
}

// ── RemoveSavedTool ──

func TestRemoveSavedTool_Success(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	addResp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "remove-me",
		Description: "test",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
	})
	require.NoError(t, err)

	removeResp, err := svc.RemoveSavedTool(ctx, &pb.RemoveSavedToolRequest{Id: addResp.Id})
	require.NoError(t, err)
	assert.True(t, removeResp.Removed)

	// Verify it's gone
	_, err = svc.GetSavedTool(ctx, &pb.GetSavedToolRequest{Id: addResp.Id})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, errCode(err))
}

func TestRemoveSavedTool_MissingID(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.RemoveSavedTool(ctx, &pb.RemoveSavedToolRequest{Id: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errCode(err))
}

func TestRemoveSavedTool_NotFound(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.RemoveSavedTool(ctx, &pb.RemoveSavedToolRequest{Id: "nonexistent"})
	require.NoError(t, err)
	assert.False(t, resp.Removed, "removing nonexistent tool should return Removed=false")
}

// ── toProtoSavedTool ──

func TestToProtoSavedTool_AllFields(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "proto-test",
		Description: "check conversion",
		Type:        "script",
		Parameters:  `{"a": 1}`,
		Script:      "echo test",
		Author:      "author",
		Category:    "testing",
		Triggers:    []string{"daily"},
		Priority:    ptrInt32(2),
	})
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "proto-test", resp.Name)
	assert.Equal(t, "check conversion", resp.Description)
	assert.Equal(t, "script", resp.Type)
	assert.Equal(t, `{"a": 1}`, resp.Parameters)
	assert.Equal(t, "echo test", resp.Script)
	assert.Equal(t, "testing", resp.Category)
	assert.Equal(t, []string{"daily"}, resp.Triggers)
	assert.Equal(t, "author", resp.Author)
	assert.Equal(t, int32(2), resp.Priority)
	assert.Greater(t, resp.CreatedAt, int64(0))
	assert.Greater(t, resp.UpdatedAt, int64(0))
}

func TestToProtoSavedTool_EmptyTriggers(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := svc.AddSavedTool(ctx, &pb.AddSavedToolRequest{
		Name:        "no-triggers",
		Description: "test",
		Type:        "script",
		Parameters:  `{}`,
		Script:      "echo",
		Author:      "user",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Triggers, 0)
}
