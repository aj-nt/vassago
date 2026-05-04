// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"
	"sync"
)

// MutationRateLimit tracks AddMemory calls per agent session to prevent
// loop-driven memory pollution. Thread-safe for concurrent tool handlers.
type MutationRateLimit struct {
	mu      sync.Mutex
	count   int
	limit   int
	enabled bool
}

// NewMutationRateLimit creates a rate limiter.
// A limit of 0 or less uses the default (10).
// A limit of -1 disables rate limiting entirely.
func NewMutationRateLimit(limit int) *MutationRateLimit {
	if limit == -1 {
		return &MutationRateLimit{limit: 0, enabled: false}
	}
	if limit <= 0 {
		limit = 10 // default
	}
	return &MutationRateLimit{
		limit:   limit,
		enabled: true,
	}
}

// Allow returns true if an AddMemory call is permitted.
// Returns false if the session has exceeded its mutation limit.
func (r *MutationRateLimit) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.enabled {
		return true
	}

	if r.count >= r.limit {
		return false
	}
	r.count++
	return true
}

// Count returns the number of mutations consumed so far.
func (r *MutationRateLimit) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Reset clears the mutation counter (e.g., at session start).
func (r *MutationRateLimit) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count = 0
}

// String returns a human-readable description of the rate limit state.
func (r *MutationRateLimit) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return "mutation rate limit: disabled"
	}
	return fmt.Sprintf("mutation rate limit: %d/%d", r.count, r.limit)
}