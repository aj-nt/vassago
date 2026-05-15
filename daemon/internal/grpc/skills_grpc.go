// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
)

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

func (s *Service) AddSkill(ctx context.Context, req *pb.AddSkillRequest) (*pb.SkillEntry, error) {
	priority := int32(3)
	if req.Priority != nil {
		priority = *req.Priority
	}

	params := store.AddSkillParams{
		Owner:       ownerFromCtx(ctx),
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

func (s *Service) GetSkill(ctx context.Context, req *pb.GetSkillRequest) (*pb.SkillEntry, error) {
	entry, err := s.store.GetSkillByID(req.Id)
	if err != nil {
		return nil, err
	}
	return skillEntryToProto(entry), nil
}

func (s *Service) ListSkills(ctx context.Context, req *pb.ListSkillsRequest) (*pb.SkillList, error) {
	limit := 0
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.ListSkills(ownerFromCtx(ctx), req.Category, req.SourceAgent, limit)
	if err != nil {
		return nil, err
	}

	skills := make([]*pb.SkillEntry, len(entries))
	for i, e := range entries {
		skills[i] = skillEntryToProto(e)
	}
	return &pb.SkillList{Skills: skills}, nil
}

func (s *Service) RemoveSkill(ctx context.Context, req *pb.RemoveSkillRequest) (*pb.RemoveSkillResponse, error) {
	success, err := s.store.RemoveSkill(ownerFromCtx(ctx), req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.RemoveSkillResponse{Success: success}, nil
}

func (s *Service) SearchSkills(ctx context.Context, req *pb.SearchSkillsRequest) (*pb.SkillList, error) {
	limit := 0
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.SearchSkills(ownerFromCtx(ctx), req.Query, req.Category, limit)
	if err != nil {
		return nil, err
	}

	skills := make([]*pb.SkillEntry, len(entries))
	for i, e := range entries {
		skills[i] = skillEntryToProto(e)
	}
	return &pb.SkillList{Skills: skills}, nil
}
