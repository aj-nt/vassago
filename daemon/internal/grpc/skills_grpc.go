// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

// skillEntryToProto converts a store.SkillEntry to a pb.SkillEntry.
func skillEntryToProto(e *store.SkillEntry) *pb.SkillEntry {
	if e == nil {
		return nil
	}
	return &pb.SkillEntry{
		Id:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Triggers:    e.Triggers,
		Category:    e.Category,
		Content:     e.Content,
		SourceAgent: e.SourceAgent,
		Priority:    e.Priority,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// AddSkill creates a new skill entry.
func (s *Service) AddSkill(ctx context.Context, req *pb.AddSkillRequest) (*pb.SkillEntry, error) {
	priority := int32(3) // default
	if req.Priority != nil {
		priority = *req.Priority
	}

	params := store.AddSkillParams{
		Name:        req.Name,
		Description: req.Description,
		Triggers:    req.Triggers,
		Category:    req.Category,
		Content:     req.Content,
		SourceAgent: req.SourceAgent,
		Priority:    priority,
	}

	entry, err := s.store.AddSkill(params)
	if err != nil {
		return nil, err
	}

	return skillEntryToProto(entry), nil
}

// GetSkill retrieves a skill by ID.
func (s *Service) GetSkill(ctx context.Context, req *pb.GetSkillRequest) (*pb.SkillEntry, error) {
	entry, err := s.store.GetSkillByID(req.Id)
	if err != nil {
		return nil, err
	}
	return skillEntryToProto(entry), nil
}

// ListSkills retrieves skills with optional filtering.
func (s *Service) ListSkills(ctx context.Context, req *pb.ListSkillsRequest) (*pb.SkillList, error) {
	limit := 0
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.ListSkills(req.Category, req.SourceAgent, limit)
	if err != nil {
		return nil, err
	}

	skills := make([]*pb.SkillEntry, len(entries))
	for i, e := range entries {
		skills[i] = skillEntryToProto(e)
	}
	return &pb.SkillList{Skills: skills}, nil
}

// RemoveSkill deletes a skill by ID.
func (s *Service) RemoveSkill(ctx context.Context, req *pb.RemoveSkillRequest) (*pb.RemoveSkillResponse, error) {
	success, err := s.store.RemoveSkill(req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.RemoveSkillResponse{Success: success}, nil
}

// SearchSkills searches skills using FTS5.
func (s *Service) SearchSkills(ctx context.Context, req *pb.SearchSkillsRequest) (*pb.SkillList, error) {
	limit := 0
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.SearchSkills(req.Query, req.Category, limit)
	if err != nil {
		return nil, err
	}

	skills := make([]*pb.SkillEntry, len(entries))
	for i, e := range entries {
		skills[i] = skillEntryToProto(e)
	}
	return &pb.SkillList{Skills: skills}, nil
}