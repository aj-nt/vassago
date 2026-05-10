// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import "sync/atomic"

// Metrics holds counters and gauges for agent observability.
// All fields are atomic for concurrent access.
type Metrics struct {
	TotalTurns        int64   // atomic counter for pipeline iterations
	TotalToolsCalled  int64   // atomic counter for tool dispatches
	LoopDetections    int64   // atomic counter for YIELD/TERMINATE from loop detection
	RoutingFallbacks  int64   // atomic counter for provider fallback events
	AccumulatedCostUSD int64  // atomic: cost in micro-dollars (µ$); divide by 1e6 for USD
}

// NewMetrics creates a new zero-initialized Metrics.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// MetricsSnapshot is a point-in-time read of all counters.
type MetricsSnapshot struct {
	TotalTurns         int
	TotalToolsCalled   int
	LoopDetections     int
	RoutingFallbacks   int
	AccumulatedCostUSD float64
}

// Snapshot returns an independent copy of current metric values.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalTurns:         int(atomic.LoadInt64(&m.TotalTurns)),
		TotalToolsCalled:   int(atomic.LoadInt64(&m.TotalToolsCalled)),
		LoopDetections:     int(atomic.LoadInt64(&m.LoopDetections)),
		RoutingFallbacks:   int(atomic.LoadInt64(&m.RoutingFallbacks)),
		AccumulatedCostUSD: float64(atomic.LoadInt64(&m.AccumulatedCostUSD)) / 1_000_000.0,
	}
}

// IncTurns increments the iteration counter.
func (m *Metrics) IncTurns() {
	atomic.AddInt64(&m.TotalTurns, 1)
}

// AddToolsCalled adds n to the tools-called counter.
func (m *Metrics) AddToolsCalled(n int) {
	atomic.AddInt64(&m.TotalToolsCalled, int64(n))
}

// IncLoopDetections increments the loop detection counter.
func (m *Metrics) IncLoopDetections() {
	atomic.AddInt64(&m.LoopDetections, 1)
}

// IncRoutingFallbacks increments the routing fallback counter.
func (m *Metrics) IncRoutingFallbacks() {
	atomic.AddInt64(&m.RoutingFallbacks, 1)
}

// AddCost adds cost in USD to the accumulated cost tracker.
func (m *Metrics) AddCost(usd float64) {
	// Convert to micro-dollars for atomic int64 storage
	micros := int64(usd * 1_000_000.0)
	atomic.AddInt64(&m.AccumulatedCostUSD, micros)
}

// LogDump emits the current metrics snapshot as a structured log entry.
func (m *Metrics) LogDump() {
	snap := m.Snapshot()
	Info("metrics.snapshot",
		"total_turns", snap.TotalTurns,
		"total_tools_called", snap.TotalToolsCalled,
		"loop_detections", snap.LoopDetections,
		"routing_fallbacks", snap.RoutingFallbacks,
		"accumulated_cost_usd", snap.AccumulatedCostUSD,
	)
}

// --- Global convenience functions ---

var globalMetrics = NewMetrics()

// MetricsIncTurns is the global convenience for IncTurns.
func MetricsIncTurns() { globalMetrics.IncTurns() }

// MetricsAddToolsCalled is the global convenience for AddToolsCalled.
func MetricsAddToolsCalled(n int) { globalMetrics.AddToolsCalled(n) }

// MetricsIncLoopDetections is the global convenience for IncLoopDetections.
func MetricsIncLoopDetections() { globalMetrics.IncLoopDetections() }

// MetricsIncRoutingFallbacks is the global convenience for IncRoutingFallbacks.
func MetricsIncRoutingFallbacks() { globalMetrics.IncRoutingFallbacks() }

// MetricsAddCost is the global convenience for AddCost.
func MetricsAddCost(usd float64) { globalMetrics.AddCost(usd) }

// GlobalMetricsSnapshot returns a snapshot of the global metrics.
func GlobalMetricsSnapshot() MetricsSnapshot { return globalMetrics.Snapshot() }
