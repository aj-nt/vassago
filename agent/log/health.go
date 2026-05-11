// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthConfig configures the /health endpoint.
type HealthConfig struct {
	Version   string
	StartTime time.Time
	// Providers maps provider name to status: "healthy", "degraded", "unreachable".
	// May be nil if provider status is not being tracked separately.
	Providers map[string]string
}

// HealthResponse is the JSON response from /health.
type HealthResponse struct {
	Status       string            `json:"status"`
	Version      string            `json:"version"`
	UptimeSeconds int64            `json:"uptime_seconds"`
	Metrics      MetricsSnapshot   `json:"metrics"`
	RecentErrors []ErrorSummary    `json:"recent_errors"`
	Providers    map[string]string `json:"providers,omitempty"`
}

// ErrorSummary holds a recent error for the health response.
type ErrorSummary struct {
	Source    string `json:"source"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// HealthHandler serves the /health JSON endpoint.
type HealthHandler struct {
	cfg          HealthConfig
	errors       []ErrorSummary
	mu           sync.Mutex
	maxErrors    int
}

var globalHealth *HealthHandler

// SetGlobalHealthHandler sets the global health handler for error recording.
func SetGlobalHealthHandler(h *HealthHandler) { globalHealth = h }

// RecordHealthError records an error to the global health handler, if set.
func RecordHealthError(source, message string) {
	if globalHealth != nil {
		globalHealth.RecordError(source, message)
	}
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(cfg HealthConfig) *HealthHandler {
	return &HealthHandler{
		cfg:       cfg,
		maxErrors: 10,
	}
}

// RecordError adds an error to the recent errors buffer.
// Thread-safe; trims to maxErrors on overflow.
func (h *HealthHandler) RecordError(source, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.errors = append(h.errors, ErrorSummary{
		Source:    source,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
	if len(h.errors) > h.maxErrors {
		h.errors = h.errors[len(h.errors)-h.maxErrors:]
	}
}

// ServeHTTP implements http.Handler.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status := "healthy"

	h.mu.Lock()
	// If there are recent errors, status is degraded
	if len(h.errors) > 0 {
		status = "degraded"
	}

	// Copy errors snapshot
	errorsCopy := make([]ErrorSummary, len(h.errors))
	copy(errorsCopy, h.errors)
	h.mu.Unlock()

	// Check provider health for degradation
	if h.cfg.Providers != nil {
		for _, s := range h.cfg.Providers {
			if s == "unreachable" {
				status = "unreachable"
				break
			}
			if s == "degraded" && status != "unreachable" {
				status = "degraded"
			}
		}
	}

	providers := h.cfg.Providers
	if providers == nil {
		providers = map[string]string{}
	}

	resp := HealthResponse{
		Status:        status,
		Version:       h.cfg.Version,
		UptimeSeconds: int64(time.Since(h.cfg.StartTime).Seconds()),
		Metrics:       GlobalMetricsSnapshot(),
		RecentErrors:  errorsCopy,
		Providers:     providers,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
