// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestMetricsCounterNew(t *testing.T) {
	c := NewMetrics()
	s := c.Snapshot()

	if s.TotalTurns != 0 {
		t.Errorf("TotalTurns = %d, want 0", s.TotalTurns)
	}
	if s.TotalToolsCalled != 0 {
		t.Errorf("TotalToolsCalled = %d, want 0", s.TotalToolsCalled)
	}
	if s.LoopDetections != 0 {
		t.Errorf("LoopDetections = %d, want 0", s.LoopDetections)
	}
	if s.RoutingFallbacks != 0 {
		t.Errorf("RoutingFallbacks = %d, want 0", s.RoutingFallbacks)
	}
	if s.AccumulatedCostUSD != 0 {
		t.Errorf("AccumulatedCostUSD = %f, want 0", s.AccumulatedCostUSD)
	}
}

func TestMetricsIncrementTurns(t *testing.T) {
	c := NewMetrics()
	c.IncTurns()
	c.IncTurns()
	c.IncTurns()

	s := c.Snapshot()
	if s.TotalTurns != 3 {
		t.Errorf("TotalTurns = %d, want 3", s.TotalTurns)
	}
}

func TestMetricsIncrementToolsCalled(t *testing.T) {
	c := NewMetrics()
	c.AddToolsCalled(5)
	c.AddToolsCalled(2)

	s := c.Snapshot()
	if s.TotalToolsCalled != 7 {
		t.Errorf("TotalToolsCalled = %d, want 7", s.TotalToolsCalled)
	}
}

func TestMetricsIncrementLoopDetections(t *testing.T) {
	c := NewMetrics()
	c.IncLoopDetections()
	c.IncLoopDetections()

	s := c.Snapshot()
	if s.LoopDetections != 2 {
		t.Errorf("LoopDetections = %d, want 2", s.LoopDetections)
	}
}

func TestMetricsIncrementRoutingFallbacks(t *testing.T) {
	c := NewMetrics()
	c.IncRoutingFallbacks()
	c.IncRoutingFallbacks()
	c.IncRoutingFallbacks()

	s := c.Snapshot()
	if s.RoutingFallbacks != 3 {
		t.Errorf("RoutingFallbacks = %d, want 3", s.RoutingFallbacks)
	}
}

func TestMetricsAddCost(t *testing.T) {
	c := NewMetrics()
	c.AddCost(0.0042)
	c.AddCost(0.0018)

	s := c.Snapshot()
	if s.AccumulatedCostUSD < 0.0059 || s.AccumulatedCostUSD > 0.0061 {
		t.Errorf("AccumulatedCostUSD = %f, want ~0.0060", s.AccumulatedCostUSD)
	}
}

func TestMetricsSnapshot(t *testing.T) {
	c := NewMetrics()
	c.IncTurns()
	c.AddToolsCalled(3)
	c.IncLoopDetections()
	c.AddCost(0.003)

	snap := c.Snapshot()
	if snap.TotalTurns != 1 {
		t.Errorf("snapshot TotalTurns = %d", snap.TotalTurns)
	}
	if snap.TotalToolsCalled != 3 {
		t.Errorf("snapshot TotalToolsCalled = %d", snap.TotalToolsCalled)
	}
	if snap.LoopDetections != 1 {
		t.Errorf("snapshot LoopDetections = %d", snap.LoopDetections)
	}
	if snap.AccumulatedCostUSD != 0.003 {
		t.Errorf("snapshot AccumulatedCostUSD = %f", snap.AccumulatedCostUSD)
	}
}

func TestMetricsSnapshotIsIndependent(t *testing.T) {
	c := NewMetrics()
	c.IncTurns()
	snap := c.Snapshot()

	// Mutate original — snapshot should be unchanged
	c.IncTurns()
	c.IncTurns()

	if snap.TotalTurns != 1 {
		t.Errorf("snapshot should be immutable: got %d, want 1", snap.TotalTurns)
	}
	s := c.Snapshot()
	if s.TotalTurns != 3 {
		t.Errorf("original should be 3, got %d", s.TotalTurns)
	}
}

func TestMetricsLogDump(t *testing.T) {
	l, buf := testLoggerJSON()
	Init(l)

	c := NewMetrics()
	c.IncTurns()
	c.IncTurns()
	c.AddToolsCalled(12)
	c.IncLoopDetections()
	c.AddCost(0.025)

	c.LogDump()

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if entry["msg"] != "metrics.snapshot" {
		t.Errorf("msg = %q", entry["msg"])
	}
	if entry["total_turns"] != float64(2) {
		t.Errorf("total_turns = %v", entry["total_turns"])
	}
	if entry["total_tools_called"] != float64(12) {
		t.Errorf("total_tools_called = %v", entry["total_tools_called"])
	}
	if entry["loop_detections"] != float64(1) {
		t.Errorf("loop_detections = %v", entry["loop_detections"])
	}
	cost, ok := entry["accumulated_cost_usd"].(float64)
	if !ok || cost < 0.024 || cost > 0.026 {
		t.Errorf("accumulated_cost_usd = %v, want ~0.025", entry["accumulated_cost_usd"])
	}
}

func TestGlobalMetrics(t *testing.T) {
	// Reset global metrics by creating fresh Metrics
	m := NewMetrics()
	oldMetrics := globalMetrics
	globalMetrics = m
	defer func() { globalMetrics = oldMetrics }()

	MetricsIncTurns()
	MetricsAddToolsCalled(5)
	MetricsIncLoopDetections()
	MetricsIncRoutingFallbacks()
	MetricsAddCost(0.01)

	snap := m.Snapshot()
	if snap.TotalTurns != 1 {
		t.Errorf("TotalTurns = %d", snap.TotalTurns)
	}
	if snap.TotalToolsCalled != 5 {
		t.Errorf("TotalToolsCalled = %d", snap.TotalToolsCalled)
	}
	if snap.AccumulatedCostUSD != 0.01 {
		t.Errorf("AccumulatedCostUSD = %f", snap.AccumulatedCostUSD)
	}
}

func TestMetricsConcurrentUpdates(t *testing.T) {
	c := NewMetrics()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.IncTurns()
				c.AddToolsCalled(1)
				c.IncLoopDetections()
				c.IncRoutingFallbacks()
				c.AddCost(0.001)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	snap := c.Snapshot()
	if snap.TotalTurns != 1000 {
		t.Errorf("TotalTurns = %d, want 1000", snap.TotalTurns)
	}
	if snap.TotalToolsCalled != 1000 {
		t.Errorf("TotalToolsCalled = %d, want 1000", snap.TotalToolsCalled)
	}
}

// testLoggerJSON returns a logger writing JSON to buf.
func testLoggerJSON() (*slog.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), buf
}
