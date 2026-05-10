// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthHandlerReturns200(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version: "1.0.0",
		StartTime: time.Now().Add(-5 * time.Minute),
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHealthHandlerJSONContentType(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version: "1.0.0",
		StartTime: time.Now(),
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHealthHandlerResponseFields(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	start := time.Now().Add(-10 * time.Minute)
	h := NewHealthHandler(HealthConfig{
		Version:   "2.3.1",
		StartTime: start,
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("status = %q, want healthy", resp.Status)
	}
	if resp.Version != "2.3.1" {
		t.Errorf("version = %q, want 2.3.1", resp.Version)
	}
	if resp.UptimeSeconds < 590 || resp.UptimeSeconds > 610 {
		t.Errorf("uptime_seconds = %d, want ~600", resp.UptimeSeconds)
	}
}

func TestHealthHandlerIncludesMetrics(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	// Populate some metrics
	MetricsIncTurns()
	MetricsIncTurns()
	MetricsIncLoopDetections()
	MetricsAddCost(0.01)

	h := NewHealthHandler(HealthConfig{
		Version:   "1.0.0",
		StartTime: time.Now(),
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Metrics.TotalTurns < 2 {
		t.Errorf("metrics.total_turns = %d, want >= 2", resp.Metrics.TotalTurns)
	}
	if resp.Metrics.LoopDetections < 1 {
		t.Errorf("metrics.loop_detections = %d, want >= 1", resp.Metrics.LoopDetections)
	}
	if resp.Metrics.AccumulatedCostUSD < 0.009 {
		t.Errorf("metrics.accumulated_cost_usd = %f, want >= 0.01", resp.Metrics.AccumulatedCostUSD)
	}
}

func TestHealthHandlerDegradedWhenErrorsPresent(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version:   "1.0.0",
		StartTime: time.Now(),
	})

	// Record a recent error
	h.RecordError("provider", "openai timeout on iteration 3")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded", resp.Status)
	}
	if len(resp.RecentErrors) == 0 {
		t.Fatal("expected recent errors, got none")
	}
	if resp.RecentErrors[0].Source != "provider" {
		t.Errorf("recent_errors[0].source = %q, want provider", resp.RecentErrors[0].Source)
	}
}

func TestHealthHandlerRecentErrorsAreCapped(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version:   "1.0.0",
		StartTime: time.Now(),
	})

	// Record more than the cap (default 10)
	for i := 0; i < 15; i++ {
		h.RecordError("test", "error message")
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.RecentErrors) > 10 {
		t.Errorf("recent_errors length = %d, want <= 10", len(resp.RecentErrors))
	}
}

func TestHealthHandlerProviderStatus(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version:   "1.0.0",
		StartTime: time.Now(),
		Providers: map[string]string{
			"openai":    "healthy",
			"anthropic": "degraded",
			"ollama":    "unreachable",
		},
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Providers["openai"] != "healthy" {
		t.Errorf("providers.openai = %q", resp.Providers["openai"])
	}
	if resp.Providers["anthropic"] != "degraded" {
		t.Errorf("providers.anthropic = %q", resp.Providers["anthropic"])
	}
}

func TestHealthHandlerWithoutProviderStatusDefaultsOK(t *testing.T) {
	l, _ := testLoggerJSON()
	Init(l)

	h := NewHealthHandler(HealthConfig{
		Version:   "1.0.0",
		StartTime: time.Now(),
		// Providers nil — should work fine
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp HealthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Status != "healthy" {
		t.Errorf("status = %q", resp.Status)
	}
}
