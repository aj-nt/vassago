// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"fmt"
	"time"
)

// ConsolidationScope defines what category of memories to consolidate.
type ConsolidationScope int

const (
	ConsolidateScopeAll          ConsolidationScope = iota // All low-priority entries
	ConsolidateScopeObservations                           // observation category with priority <= 2
	ConsolidateScopeCategory                               // Specific category with priority <= 2
	ConsolidateScopeExpired                                // Entries past their expires_at
	ConsolidateScopeIdle                                   // Not accessed in 30+ days, priority <= 2
)

// ConsolidateResult holds the result of a consolidation operation.
type ConsolidateResult struct {
	Removed int
	Merged  int
	Summary string
}

// ConsolidationCandidate is a memory entry earmarked for consolidation,
// with a reason explaining why it's a candidate.
type ConsolidationCandidate struct {
	Entry  *MemoryEntry
	Reason string
}

// GetConsolidationCandidates returns memory entries that match the given consolidation scope.
// batchSize limits the number of candidates returned (0 or negative = no limit).
func (s *Store) GetConsolidationCandidates(owner string, scope ConsolidationScope, category string, batchSize int) ([]*ConsolidationCandidate, error) {
	batchSize = ClampLimit(batchSize, 100, 0)

	query := `
		SELECT id, owner, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE owner = ?`
	var args = []any{owner}

	now := time.Now().Unix()

	switch scope {
	case ConsolidateScopeExpired:
		query += " AND expires_at > 0 AND expires_at < ?"
		args = append(args, now)
	case ConsolidateScopeIdle:
		cutoff := now - int64(30*24*60*60) // 30 days ago
		query += " AND accessed_at > 0 AND accessed_at < ? AND priority <= 2"
		args = append(args, cutoff)
	case ConsolidateScopeObservations:
		query += " AND category = 'observation' AND priority <= 2"
	case ConsolidateScopeCategory:
		if category == "" {
			return nil, fmt.Errorf("category is required for CATEGORY scope")
		}
		query += " AND category = ? AND priority <= 2"
		args = append(args, category)
	case ConsolidateScopeAll:
		// All entries with priority 1 (lowest)
		query += " AND priority = 1"
	default:
		return nil, fmt.Errorf("unknown consolidation scope: %d", scope)
	}

	query += " ORDER BY priority ASC, updated_at ASC LIMIT ?"
	args = append(args, batchSize)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get consolidation candidates: %w", err)
	}
	defer rows.Close()

	entries, err := scanMemories(rows)
	if err != nil {
		return nil, fmt.Errorf("scan candidates: %w", err)
	}

	var candidates []*ConsolidationCandidate
	for _, e := range entries {
		reason := consolidationReason(scope, e, category)
		candidates = append(candidates, &ConsolidationCandidate{
			Entry:  e,
			Reason: reason,
		})
	}
	return candidates, nil
}

// Consolidate removes memory entries according to the given scope.
// EXPIRED: removes all entries past their expires_at timestamp.
// IDLE: removes entries not accessed in 30+ days with priority <= 2.
// OBSERVATIONS: removes observations older than 7 days with priority <= 2.
// CATEGORY: removes entries in the specified category with priority <= 2.
// ALL: delegates to EXPIRED + IDLE.
func (s *Store) Consolidate(owner string, scope ConsolidationScope, category string, batchSize int) (*ConsolidateResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	switch scope {
	case ConsolidateScopeExpired:
		result, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND expires_at > 0 AND expires_at < ?", owner, now)
		if err != nil {
			return nil, fmt.Errorf("consolidate expired: %w", err)
		}
		removed, _ := result.RowsAffected()
		return &ConsolidateResult{
			Removed: int(removed),
			Summary: fmt.Sprintf("Removed %d expired memories", removed),
		}, nil

	case ConsolidateScopeIdle:
		cutoff := now - int64(30*24*60*60)
		result, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND accessed_at > 0 AND accessed_at < ? AND priority <= 2", owner, cutoff)
		if err != nil {
			return nil, fmt.Errorf("consolidate idle: %w", err)
		}
		removed, _ := result.RowsAffected()
		return &ConsolidateResult{
			Removed: int(removed),
			Summary: fmt.Sprintf("Removed %d idle memories (not accessed in 30 days)", removed),
		}, nil

	case ConsolidateScopeObservations:
		weekAgo := now - int64(7*24*60*60)
		result, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND category = 'observation' AND created_at < ? AND priority <= 2", owner, weekAgo)
		if err != nil {
			return nil, fmt.Errorf("consolidate observations: %w", err)
		}
		removed, _ := result.RowsAffected()
		return &ConsolidateResult{
			Removed: int(removed),
			Summary: fmt.Sprintf("Removed %d old observations (7+ days, priority <= 2)", removed),
		}, nil

	case ConsolidateScopeCategory:
		if category == "" {
			return nil, fmt.Errorf("category is required for CATEGORY scope")
		}
		result, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND category = ? AND priority <= 2", owner, category)
		if err != nil {
			return nil, fmt.Errorf("consolidate category %s: %w", category, err)
		}
		removed, _ := result.RowsAffected()
		return &ConsolidateResult{
			Removed: int(removed),
			Summary: fmt.Sprintf("Removed %d memories in category %s (priority <= 2)", removed, category),
		}, nil

	case ConsolidateScopeAll:
		// ALL deletes expired + idle entries in one transaction
		expiredResult, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND expires_at > 0 AND expires_at < ?", owner, now)
		if err != nil {
			return nil, fmt.Errorf("consolidate all (expired): %w", err)
		}
		expiredRemoved, _ := expiredResult.RowsAffected()

		idleCutoff := now - int64(30*24*60*60)
		idleResult, err := s.db.Exec(
			"DELETE FROM memories WHERE owner = ? AND accessed_at > 0 AND accessed_at < ? AND priority <= 2", owner, idleCutoff)
		if err != nil {
			return nil, fmt.Errorf("consolidate all (idle): %w", err)
		}
		idleRemoved, _ := idleResult.RowsAffected()

		return &ConsolidateResult{
			Removed: int(expiredRemoved) + int(idleRemoved),
			Merged:  0,
			Summary: fmt.Sprintf("Consolidated all: %d expired + %d idle", expiredRemoved, idleRemoved),
		}, nil

	default:
		return nil, fmt.Errorf("unknown consolidation scope: %d", scope)
	}
}

// ExpireStaleMemories removes all memories that have passed their expires_at timestamp.
// This is the startup sweep — run once when the daemon starts.
func (s *Store) ExpireStaleMemories(owner string) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec(
		"DELETE FROM memories WHERE owner = ? AND expires_at > 0 AND expires_at < ?", owner, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("expire stale memories: %w", err)
	}
	removed, _ := result.RowsAffected()
	return int(removed), nil
}

// consolidationReason returns a human-readable reason why an entry is a consolidation candidate.
func consolidationReason(scope ConsolidationScope, entry *MemoryEntry, category string) string {
	now := time.Now().Unix()
	switch scope {
	case ConsolidateScopeExpired:
		return fmt.Sprintf("expired %d seconds ago", now-entry.ExpiresAt)
	case ConsolidateScopeIdle:
		days := (now - entry.AccessedAt) / 86400
		return fmt.Sprintf("not accessed in %d days, priority %d", days, entry.Priority)
	case ConsolidateScopeObservations:
		days := (now - entry.CreatedAt) / 86400
		return fmt.Sprintf("observation %d days old, priority %d", days, entry.Priority)
	case ConsolidateScopeCategory:
		return fmt.Sprintf("category %s, priority %d", entry.Category, entry.Priority)
	case ConsolidateScopeAll:
		return fmt.Sprintf("priority %d (lowest)", entry.Priority)
	default:
		return "unknown scope"
	}
}
