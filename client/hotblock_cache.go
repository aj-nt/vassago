// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"sync"
	"time"
)

// HotBlockCache provides a local cache of the hot block content.
// It holds the latest hot block text and can be updated when
// telepathy events arrive from the daemon.
type HotBlockCache struct {
	mu         sync.RWMutex
	content    string
	charCount  int
	entryCount int
	computedAt int64
	target     string
	agentID    string
}

// NewHotBlockCache creates a new cache with the initial hot block content.
func NewHotBlockCache(target, agentID, content string, charCount, entryCount int) *HotBlockCache {
	return &HotBlockCache{
		content:    content,
		charCount:  charCount,
		entryCount: entryCount,
		computedAt: time.Now().Unix(),
		target:     target,
		agentID:    agentID,
	}
}

// Content returns the current hot block text.
func (c *HotBlockCache) Content() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.content
}

// Update replaces the cached hot block with new content.
// Called when a telepathy event arrives from the daemon.
func (c *HotBlockCache) Update(content string, charCount, entryCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.content = content
	c.charCount = charCount
	c.entryCount = entryCount
	c.computedAt = time.Now().Unix()
}

// Stat returns metadata about the cached hot block.
func (c *HotBlockCache) Stat() (content string, charCount, entryCount int, computedAt int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.content, c.charCount, c.entryCount, c.computedAt
}

// Age returns how long since the hot block was last computed, in seconds.
func (c *HotBlockCache) Age() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(time.Unix(c.computedAt, 0))
}
