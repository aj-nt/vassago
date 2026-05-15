// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// benchStore creates a store for benchmark use (accepts testing.TB).
func benchStore(tb testing.TB) *Store {
	tb.Helper()
	dir := tb.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		tb.Fatalf("create bench store: %v", err)
	}
	tb.Cleanup(func() { s.Close() })
	return s
}

// BenchmarkHotBlock_Compute measures hot block computation at increasing scales.
// Run with: go test -bench=BenchmarkHotBlock -benchmem ./daemon/internal/store/
func BenchmarkHotBlock_Compute(b *testing.B) {
	scales := []int{10, 100, 500, 1000, 2000, 5000}
	for _, n := range scales {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			s := benchStore(b)
			defer s.Close()
			seedMemories(b, s, n)
			cfg := HotBlockConfig{
				CharLimit:     4000,
				BoostPriority: 1,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := s.ComputeHotBlock("default", "memory", cfg)
				if err != nil {
					b.Fatalf("ComputeHotBlock: %v", err)
				}
			}
		})
	}
}

// BenchmarkHotBlock_ComputeWithSession measures hot block computation
// with last-session context prepended.
func BenchmarkHotBlock_ComputeWithSession(b *testing.B) {
	scales := []int{100, 1000, 5000}
	for _, n := range scales {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			s := benchStore(b)
			defer s.Close()
			seedMemories(b, s, n)
			cfg := HotBlockConfig{
				CharLimit:     4000,
				BoostPriority: 1,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := s.ComputeHotBlockWithSession("default", "memory", cfg,
					"session-abc123", "User asked about project structure and deployment")
				if err != nil {
					b.Fatalf("ComputeHotBlockWithSession: %v", err)
				}
			}
		})
	}
}

// seedMemories populates the store with n memory entries for benchmarking.
// Uses varied categories and priorities to exercise sort/comparison paths.
func seedMemories(tb testing.TB, s *Store, n int) {
	tb.Helper()
	cats := []string{"project", "environment", "quirk", "observation", "user", "preference"}
	for i := 0; i < n; i++ {
		params := AddMemoryParams{
			Owner:       "default",
			Target:      "memory",
			Category:    cats[i%len(cats)],
			Key:         fmt.Sprintf("bench-key-%d", i),
			Content:     fmt.Sprintf("Benchmark memory entry number %d with extra words to make content longer so we exercise real-world hot block accumulation behavior and char limit enforcement.", i),
			Priority:    int32(1 + (i % 5)),
			SourceAgent: "bench",
		}
		if _, err := s.AddMemory(params); err != nil {
			tb.Fatalf("seed memory %d: %v", i, err)
		}
	}
}
