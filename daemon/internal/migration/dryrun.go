// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package migration

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// DryRunImport reads a Hermes state.db and reports what would be imported,
// without writing anything to Vassago.
func DryRunImport(hermesDBPath string, agentID string) (*ImportResult, error) {
	if agentID == "" {
		agentID = "hermes-import"
	}
	result := &ImportResult{}

	hermesDB, err := sql.Open("sqlite", hermesDBPath)
	if err != nil {
		return nil, fmt.Errorf("open hermes db: %w", err)
	}
	defer hermesDB.Close()

	if _, err := hermesDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set hermes WAL mode: %w", err)
	}

	// Count memories
	memRows, err := hermesDB.Query("SELECT id, target, category, key FROM memories ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("query hermes memories: %w", err)
	}
	defer memRows.Close()

	for memRows.Next() {
		var id int64
		var target, category, key string
		if err := memRows.Scan(&id, &target, &category, &key); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan memory %d: %v", id, err))
			continue
		}
		result.MemoriesRead++
		if target != "" && key != "" && category != "" {
			result.MemoriesImported++
		} else {
			result.MemoriesSkipped++
		}
	}
	if err := memRows.Err(); err != nil {
		return nil, fmt.Errorf("after scanning memories: %w", err)
	}

	// Count sessions and messages
	sessRows, err := hermesDB.Query("SELECT id FROM sessions ORDER BY id")
	if err != nil {
		log.Printf("WARNING: could not query sessions: %v", err)
		return result, nil
	}
	defer sessRows.Close()

	var sessionIDs []string
	for sessRows.Next() {
		var id string
		if err := sessRows.Scan(&id); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan session: %v", err))
			continue
		}
		result.SessionsRead++
		sessionIDs = append(sessionIDs, id)
		result.SessionsImported++
	}
	if err := sessRows.Err(); err != nil {
		return nil, fmt.Errorf("after scanning sessions: %w", err)
	}

	// Count messages for all sessions
	for _, sid := range sessionIDs {
		var count int
		err := hermesDB.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sid).Scan(&count)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("count messages for session %s: %v", sid, err))
			continue
		}
		result.MessagesRead += count
		result.MessagesImported += count
	}

	log.Printf("Dry run complete: %d memories, %d sessions, %d messages would be imported",
		result.MemoriesImported, result.SessionsImported, result.MessagesImported)

	return result, nil
}
