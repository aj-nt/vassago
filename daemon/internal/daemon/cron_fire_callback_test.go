// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"testing"
	"time"

	"github.com/aj-nt/mnemosyne/daemon/internal/cron"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
)

func TestCronFireCallbackPublishesEvent(t *testing.T) {
	s := testDaemonStore(t)
	defer s.Close()

	// Create an agent for foreign key
	_, _, err := s.EnsureAgent("test-agent", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	hub := pubsub.NewHub()

	// Subscribe to receive events
	sub := &pubsub.Subscription{
		ID:      "test-sub-1",
		AgentID: "other-agent", // Different agent — should NOT be filtered out (self-filter is in matches())
		Targets: map[string]bool{"memory": true},
		Ch:      make(chan pubsub.Event, 10),
	}
	hub.Subscribe(sub)

	// Create the fire callback
	callback := newCronFireCallback(s, hub)

	// Fire a cron job
	job := &cron.CronJob{
		ID:         "job-1",
		Name:       "Morning report",
		Schedule:   "0 9 * * *",
		AgentID:    "test-agent",
		Message:    "Generate the daily report",
		Enabled:    true,
		NextFireAt: 1704067200,
	}

	err = callback(job)
	if err != nil {
		t.Fatalf("FireCallback: %v", err)
	}

	// Verify event was published (hub.Publish drops self-events, so "other-agent" should get it)
	select {
	case event := <-sub.Ch:
		if event.Type != pb.UpdateEvent_CRON_FIRE {
			t.Errorf("expected CRON_FIRE event type, got %v", event.Type)
		}
		if event.CronFire == nil {
			t.Fatal("expected CronFire event data")
		}
		if event.CronFire.JobId != "job-1" {
			t.Errorf("expected job_id=job-1, got %s", event.CronFire.JobId)
		}
		if event.CronFire.JobName != "Morning report" {
			t.Errorf("expected job_name=Morning report, got %s", event.CronFire.JobName)
		}
		if event.CronFire.AgentId != "test-agent" {
			t.Errorf("expected agent_id=test-agent, got %s", event.CronFire.AgentId)
		}
		if event.CronFire.Message != "Generate the daily report" {
			t.Errorf("expected message=Generate the daily report, got %s", event.CronFire.Message)
		}
		if event.CronFire.Schedule != "0 9 * * *" {
			t.Errorf("expected schedule=0 9 * * *, got %s", event.CronFire.Schedule)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestCronFireCallbackNoSubscription(t *testing.T) {
	s := testDaemonStore(t)
	defer s.Close()

	hub := pubsub.NewHub()

	// No subscribers — callback should still succeed (just no one receives it)
	callback := newCronFireCallback(s, hub)

	job := &cron.CronJob{
		ID:       "job-2",
		Name:     "No subscribers",
		Schedule: "0 9 * * *",
		AgentID:  "test-agent",
		Message:  "Nobody listens",
		Enabled:  true,
	}

	err := callback(job)
	if err != nil {
		t.Fatalf("FireCallback with no subscribers: %v", err)
	}
}

func testDaemonStore(t *testing.T) *store.Store {
	t.Helper()
	path := t.TempDir() + "/test.db"
	s, err := store.New(path)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return s
}
