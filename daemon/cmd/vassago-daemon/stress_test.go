// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

// -- Test 1: Rapid sequential writes (100 memories back-to-back) --

func TestStress_RapidMemoryWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const count = 100
	var errors int64

	for i := range count {
		_, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
			Target:      "memory",
			Category:    "stress",
			Key:         fmt.Sprintf("rapid_%d", i),
			Content:     fmt.Sprintf("Rapid write #%d at %v", i, time.Now().UnixNano()),
			Priority:    3,
			SourceAgent: "stress-tester",
		})
		if err != nil {
			t.Logf("write %d failed: %v", i, err)
			atomic.AddInt64(&errors, 1)
		}
	}

	if got := atomic.LoadInt64(&errors); got > 0 {
		t.Errorf("%d/%d writes failed", got, count)
	}

	// Verify count
	listResp, err := client.ListMemories(ctx, &pb.ListMemoriesRequest{Target: "memory"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	stressCount := 0
	for _, e := range listResp.Entries {
		if e.Category == "stress" {
			stressCount++
		}
	}
	if stressCount != count {
		t.Errorf("expected %d stress entries, got %d", count, stressCount)
	}
}

// -- Test 2: Large content (10KB single memory) --

func TestStress_LargeContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 10KB content
	largeContent := strings.Repeat("abcdefghij ", 1000) // ~11KB

	resp, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "stress",
		Key:         "large_content",
		Content:     largeContent,
		Priority:    3,
		SourceAgent: "stress-tester",
	})
	if err != nil {
		t.Fatalf("AddMemory with large content: %v", err)
	}

	// Verify we can read it back
	getResp, err := client.GetMemory(ctx, &pb.GetMemoryRequest{Id: resp.Entry.Id})
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if len(getResp.Content) < 10000 {
		t.Errorf("large content truncated: got %d bytes, want >= 10000", len(getResp.Content))
	}
}

// -- Test 3: Rapid session creation + message flooding --

func TestStress_SessionMessageFlood(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 10 sessions
	const sessionCount = 10
	sessions := make([]string, sessionCount)
	for i := range sessionCount {
		sess, err := client.CreateSession(ctx, &pb.CreateSessionRequest{
			AgentId: "stress-agent",
			Source:  "stress",
			Title:   fmt.Sprintf("Stress session %d", i),
		})
		if err != nil {
			t.Fatalf("CreateSession %d: %v", i, err)
		}
		sessions[i] = sess.Id
	}

	// Flood each session with 50 messages
	const messagesPerSession = 50
	var totalErrors int64

	for i, sid := range sessions {
		msgs := make([]*pb.Message, messagesPerSession)
		for j := range messagesPerSession {
			msgs[j] = &pb.Message{
				Role:    "user",
				Content: fmt.Sprintf("Message %d in session %d: %s", j, i, strings.Repeat("x", 100)),
			}
		}
		_, err := client.AddMessages(ctx, &pb.AddMessagesRequest{
			SessionId: sid,
			Messages:  msgs,
		})
		if err != nil {
			t.Logf("AddMessages session %d failed: %v", i, err)
			atomic.AddInt64(&totalErrors, 1)
		}
	}

	if got := atomic.LoadInt64(&totalErrors); got > 0 {
		t.Errorf("%d sessions failed to add messages", got)
	}

	// Verify session 0 has all messages
	includeMsgs := true
	sess, err := client.GetSession(ctx, &pb.GetSessionRequest{
		SessionId:       sessions[0],
		IncludeMessages: &includeMsgs,
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(sess.Messages) != messagesPerSession {
		t.Errorf("session 0: expected %d messages, got %d", messagesPerSession, len(sess.Messages))
	}
}

// -- Test 4: Concurrent reads and writes --

func TestStress_ConcurrentReadWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Pre-populate
	resp, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
		Target: "memory", Category: "stress", Key: "concurrent_rw",
		Content: "initial", Priority: 3, SourceAgent: "stress-tester",
	})
	if err != nil {
		t.Fatalf("pre-populate: %v", err)
	}
	memID := resp.Entry.Id

	// 20 concurrent readers + 10 concurrent writers
	var readErrors, writeErrors int64
	done := make(chan struct{})

	// Writers
	for i := range 10 {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			newContent := fmt.Sprintf("update_%d", idx)
			_, err := client.UpdateMemory(ctx, &pb.UpdateMemoryRequest{
				Id:      memID,
				Content: &newContent,
			})
			if err != nil {
				atomic.AddInt64(&writeErrors, 1)
			}
		}(i)
	}

	// Readers
	for i := range 20 {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			_, err := client.Search(ctx, &pb.SearchRequest{
				Query:  "concurrent",
				Target: "memory",
			})
			if err != nil {
				t.Logf("reader %d error: %v", idx, err)
				atomic.AddInt64(&readErrors, 1)
			}
		}(i)
	}

	// Wait for all 30 goroutines
	for range 30 {
		<-done
	}

	if got := atomic.LoadInt64(&writeErrors); got > 8 {
		t.Errorf("%d/10 writes failed (some contention expected under stress)", got)
	}
	if got := atomic.LoadInt64(&readErrors); got > 0 {
		t.Errorf("%d/20 reads failed", got)
	}
}

// -- Test 5: Hot block under many memories --

func TestStress_HotBlockWithManyMemories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add 200 memories with varying priorities
	const count = 200
	for i := range count {
		priority := int32(i%5 + 1) // priorities 1-5
		_, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
			Target:      "memory",
			Category:    "stress",
			Key:         fmt.Sprintf("hotblock_%d", i),
			Content:     fmt.Sprintf("Memory %d with priority %d", i, priority),
			Priority:    priority,
			SourceAgent: "stress-tester",
		})
		if err != nil {
			t.Fatalf("AddMemory %d: %v", i, err)
		}
	}

	// Get hot block — should fit within char limit
	hb, err := client.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		CharLimit: 4000,
	})
	if err != nil {
		t.Fatalf("GetHotBlock: %v", err)
	}
	if hb.EntryCount == 0 {
		t.Error("hot block should have entries")
	}
	if hb.EntryCount > 200 {
		t.Errorf("hot block entries should be limited, got %d", hb.EntryCount)
	}
	// Content should fit within the char limit (with some overhead per entry)
	if len(hb.Content) > 6000 {
		t.Errorf("hot block content too large: %d bytes for 4000 char limit", len(hb.Content))
	}
}

// -- Test 6: Search under load --

func TestStress_SearchUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add 100 memories
	for i := range 100 {
		_, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
			Target:      "memory",
			Category:    "stress",
			Key:         fmt.Sprintf("search_%d", i),
			Content:     fmt.Sprintf("This is memory entry number %d about golang testing", i),
			Priority:    3,
			SourceAgent: "stress-tester",
		})
		if err != nil {
			t.Fatalf("AddMemory %d: %v", i, err)
		}
	}

	// 20 concurrent searches
	var searchErrors int64
	done := make(chan struct{})

	for i := range 20 {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			queries := []string{"golang", "testing", "memory", "number 50", "entry"}
			resp, err := client.Search(ctx, &pb.SearchRequest{
				Query:  queries[idx%len(queries)],
				Target: "memory",
			})
			if err != nil {
				t.Logf("search %d error: %v", idx, err)
				atomic.AddInt64(&searchErrors, 1)
			} else if len(resp.Entries) == 0 {
				t.Logf("search %d returned 0 results for query %q", idx, queries[idx%len(queries)])
			}
		}(i)
	}

	for range 20 {
		<-done
	}

	if got := atomic.LoadInt64(&searchErrors); got > 0 {
		t.Errorf("%d/20 searches failed", got)
	}
}
