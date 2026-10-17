// This file is part of Vassago.
// See LICENSE for license information.

package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/store"
)

// Stats holds aggregated daemon statistics.
type Stats struct {
	MemoryCount    int            `json:"memory_count"`
	UserCount      int            `json:"user_count"`
	TotalMemories  int            `json:"total_memories"`
	AgentCount     int            `json:"agent_count"`
	SessionCount   int            `json:"session_count"`
	ByCategory     map[string]int `json:"by_category"`
	DBPath         string         `json:"db_path"`
	SchemaVersion  int            `json:"schema_version"`
	ServerUptime   string         `json:"server_uptime"`
}

// Handler serves the dashboard JSON API and UI.
type Handler struct {
	store      *store.Store
	dbPath     string
	startTime  time.Time
}

// New creates a new dashboard handler.
func New(st *store.Store, dbPath string) *Handler {
	return &Handler{
		store:     st,
		dbPath:    dbPath,
		startTime: time.Now(),
	}
}

// RegisterRoutes registers dashboard routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", h.handleStats)
	mux.HandleFunc("/api/memories", h.handleListMemories)
	mux.HandleFunc("/api/search", h.handleSearch)
	mux.HandleFunc("/api/hot-block", h.handleHotBlock)
	mux.HandleFunc("/dashboard", h.handleUI)
	mux.HandleFunc("/", h.handleUI) // catch-all for SPA routing
}

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	// Count memories by target
	memories, _ := h.store.ListMemories("default", "memory", "", 0)
	users, _ := h.store.ListMemories("default", "user", "", 0)

	// Build category breakdown
	byCategory := make(map[string]int)
	for _, m := range memories {
		byCategory[m.Category]++
	}

	stats := Stats{
		MemoryCount:   len(memories),
		UserCount:     len(users),
		TotalMemories: len(memories) + len(users),
		ByCategory:    byCategory,
		DBPath:        h.dbPath,
		SchemaVersion: store.SchemaVersion,
		ServerUptime:  time.Since(h.startTime).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (h *Handler) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = "memory"
	}
	category := r.URL.Query().Get("category")

	entries, err := h.store.ListMemories("default", target, category, 0)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type memJSON struct {
		ID         string `json:"id"`
		Target     string `json:"target"`
		Category   string `json:"category"`
		Key        string `json:"key"`
		Content    string `json:"content"`
		Priority   int32  `json:"priority"`
		Source     string `json:"source"`
		CreatedAt  int64  `json:"created_at"`
		UpdatedAt  int64  `json:"updated_at"`
	}

	result := make([]memJSON, len(entries))
	for i, e := range entries {
		result[i] = memJSON{
			ID:        e.ID,
			Target:    e.Target,
			Category:  e.Category,
			Key:       e.Key,
			Content:   e.Content,
			Priority:  e.Priority,
			Source:    e.SourceAgent,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing query parameter 'q'", 400)
		return
	}

	entries, err := h.store.Search("default", query, "", 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type searchResult struct {
		ID        string `json:"id"`
		Target    string `json:"target"`
		Category  string `json:"category"`
		Key       string `json:"key"`
		Content   string `json:"content"`
		Priority  int32  `json:"priority"`
		Source    string `json:"source"`
	}

	result := make([]searchResult, len(entries))
	for i, e := range entries {
		result[i] = searchResult{
			ID:       e.ID,
			Target:   e.Target,
			Category: e.Category,
			Key:      e.Key,
			Content:  e.Content,
			Priority: e.Priority,
			Source:   e.SourceAgent,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *Handler) handleHotBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = "memory"
	}

	result, err := h.store.ComputeHotBlock("default", target, store.HotBlockConfig{
		CharLimit:    4000,
		BoostPriority: 2,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type hbJSON struct {
		Content    string `json:"content"`
		CharCount  int    `json:"char_count"`
		EntryCount int    `json:"entry_count"`
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hbJSON{
		Content:    result.Content,
		CharCount:  int(result.CharCount),
		EntryCount: int(result.EntryCount),
	})
}

func (h *Handler) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}
