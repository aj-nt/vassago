// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/cron"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

// newCronFireCallback creates a FireCallback that publishes a CronFireEvent
// on the pub/sub hub when a cron job fires. Any agents subscribed to the
// "memory" target with appropriate filters will receive this event.
func newCronFireCallback(s *store.Store, hub *pubsub.Hub) cron.FireCallback {
	return func(job *cron.CronJob) error {
		slog.Info("Cron job firing",
			"job_id", job.ID,
			"job_name", job.Name,
			"agent_id", job.AgentID,
			"schedule", job.Schedule,
		)

		// Publish a CronFireEvent on the hub so subscribed agents receive it.
		// Note: AgentID is set to "cron" (not the job's owner) so the owning
		// agent doesn't filter out its own scheduled tasks via the Hub's
		// self-exclusion logic (sub.AgentID == event.AgentID → skip).
		hub.Publish(pubsub.Event{
			Type:     pb.UpdateEvent_CRON_FIRE,
			Target:   "memory",
			AgentID:  "cron", // System origin, not the owning agent
			CronFire: &pb.CronFireEvent{
				JobId:       job.ID,
				JobName:     job.Name,
				Schedule:   job.Schedule,
				AgentId:    job.AgentID,
				Message:    job.Message,
				FiredAt:    time.Now().Unix(),
				NextFireAt: job.NextFireAt,
			},
		})

		slog.Info("Cron fire event published",
			"job_id", job.ID,
			"job_name", job.Name,
		)

		return nil
	}
}