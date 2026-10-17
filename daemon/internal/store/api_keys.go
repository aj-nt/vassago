// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"database/sql"
	"fmt"
	"time"
)

// SetAPIKeyOwner maps an API key to a namespace owner.
// This is the Phase 28 foundation for API-key-based multi-tenancy:
// when an agent connects, its API key determines which owner namespace
// it operates in.
func (s *Store) SetAPIKeyOwner(apiKey, owner string) error {
	if apiKey == "" {
		return fmt.Errorf("api_key must not be empty")
	}
	if owner == "" {
		owner = "default"
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO api_keys (api_key, owner, created_at)
		 VALUES (?, ?, ?)`,
		apiKey, owner, now,
	)
	if err != nil {
		return fmt.Errorf("set api key owner: %w", err)
	}
	return nil
}

// GetOwnerForAPIKey returns the owner namespace for an API key.
// Returns "default" if the key is unknown or empty.
func (s *Store) GetOwnerForAPIKey(apiKey string) (string, error) {
	if apiKey == "" {
		return "default", nil
	}

	var owner string
	err := s.db.QueryRow(
		`SELECT owner FROM api_keys WHERE api_key = ?`, apiKey,
	).Scan(&owner)

	if err == sql.ErrNoRows {
		return "default", nil
	}
	if err != nil {
		return "default", fmt.Errorf("get owner for api key: %w", err)
	}
	return owner, nil
}
