// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"encoding/json"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) AddSavedTool(ctx context.Context, req *pb.AddSavedToolRequest) (*pb.SavedToolEntry, error) {
	if req.Name == "" || req.Description == "" || req.Type == "" || req.Parameters == "" || req.Script == "" || req.Author == "" {
		return nil, status.Error(codes.InvalidArgument, "name, description, type, parameters, script, and author are required")
	}
	if req.Type != "script" {
		return nil, status.Error(codes.InvalidArgument, "type must be 'script'")
	}
	if !json.Valid([]byte(req.Parameters)) {
		return nil, status.Error(codes.InvalidArgument, "parameters must be valid JSON")
	}

	priority := int(req.GetPriority())
	if priority == 0 {
		priority = 3
	}
	if priority < 1 || priority > 5 {
		return nil, status.Error(codes.InvalidArgument, "priority must be between 1 and 5")
	}

	row, err := s.store.AddSavedTool(
		ownerFromCtx(ctx),
		req.Name,
		req.Description,
		req.Type,
		req.Parameters,
		req.Script,
		req.Category,
		req.Triggers,
		req.Author,
		priority,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "add saved tool: %v", err)
	}

	return toProtoSavedTool(row), nil
}

func (s *Service) GetSavedTool(ctx context.Context, req *pb.GetSavedToolRequest) (*pb.SavedToolEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	row, err := s.store.GetSavedTool(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get saved tool: %v", err)
	}

	return toProtoSavedTool(row), nil
}

func (s *Service) ListSavedTools(ctx context.Context, req *pb.ListSavedToolsRequest) (*pb.SavedToolList, error) {
	limit := store.ClampLimit(int(req.GetLimit()), 50, 0)

	rows, err := s.store.ListSavedTools(ownerFromCtx(ctx), req.Category, req.Author, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list saved tools: %v", err)
	}

	tools := make([]*pb.SavedToolEntry, len(rows))
	for i, row := range rows {
		tools[i] = toProtoSavedTool(&row)
	}

	return &pb.SavedToolList{Tools: tools}, nil
}

func (s *Service) SearchSavedTools(ctx context.Context, req *pb.SearchSavedToolsRequest) (*pb.SavedToolList, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	limit := store.ClampLimit(int(req.GetLimit()), 20, 0)

	rows, err := s.store.SearchSavedTools(ownerFromCtx(ctx), req.Query, req.Category, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search saved tools: %v", err)
	}

	tools := make([]*pb.SavedToolEntry, len(rows))
	for i, row := range rows {
		tools[i] = toProtoSavedTool(&row)
	}

	return &pb.SavedToolList{Tools: tools}, nil
}

func (s *Service) UpdateSavedTool(ctx context.Context, req *pb.UpdateSavedToolRequest) (*pb.SavedToolEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = req.GetName()
	}
	if req.Description != nil {
		updates["description"] = req.GetDescription()
	}
	if req.Parameters != nil {
		params := req.GetParameters()
		if !json.Valid([]byte(params)) {
			return nil, status.Error(codes.InvalidArgument, "parameters must be valid JSON")
		}
		updates["parameters"] = params
	}
	if req.Script != nil {
		updates["script"] = req.GetScript()
	}
	if req.Category != nil {
		updates["category"] = req.GetCategory()
	}
	if len(req.Triggers) > 0 {
		updates["triggers"] = req.Triggers
	}
	if req.Priority != nil {
		p := int(req.GetPriority())
		if p < 1 || p > 5 {
			return nil, status.Error(codes.InvalidArgument, "priority must be between 1 and 5")
		}
		updates["priority"] = p
	}
	if req.RequiresApprovalEveryCall != nil {
		updates["requires_approval_every_call"] = req.GetRequiresApprovalEveryCall()
	}

	row, err := s.store.UpdateSavedTool(ownerFromCtx(ctx), req.Id, updates)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update saved tool: %v", err)
	}

	return toProtoSavedTool(row), nil
}

func (s *Service) RemoveSavedTool(ctx context.Context, req *pb.RemoveSavedToolRequest) (*pb.RemoveSavedToolResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	removed, err := s.store.RemoveSavedTool(ownerFromCtx(ctx), req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "remove saved tool: %v", err)
	}

	return &pb.RemoveSavedToolResponse{Removed: removed}, nil
}

func toProtoSavedTool(row *store.SavedToolRow) *pb.SavedToolEntry {
	var triggers []string
	json.Unmarshal([]byte(row.Triggers), &triggers)

	return &pb.SavedToolEntry{
		Id:                        row.ID,
		Name:                      row.Name,
		Description:               row.Description,
		Type:                      row.Type,
		Parameters:                row.Parameters,
		Script:                    row.Script,
		Category:                  row.Category,
		Triggers:                  triggers,
		Author:                    row.Author,
		Priority:                  int32(row.Priority),
		Version:                   int32(row.Version),
		RequiresApprovalEveryCall: row.RequiresApprovalEveryCall,
		CreatedAt:                 row.CreatedAt,
		UpdatedAt:                 row.UpdatedAt,
	}
}
