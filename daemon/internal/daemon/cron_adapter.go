// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"github.com/aj-nt/vassago/daemon/internal/cron"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// cronStoreAdapter wraps a *store.Store to implement cron.CronStore.
type cronStoreAdapter struct {
	store *store.Store
}

func newCronStoreAdapter(s *store.Store) *cronStoreAdapter {
	return &cronStoreAdapter{store: s}
}

func (a *cronStoreAdapter) GetDueCronJobs(now int64) ([]*cron.CronJob, error) {
	pbJobs, err := a.store.GetDueCronJobs(now)
	if err != nil {
		return nil, err
	}
	result := make([]*cron.CronJob, len(pbJobs))
	for i, j := range pbJobs {
		result[i] = pbCronJobToCron(j)
	}
	return result, nil
}

func (a *cronStoreAdapter) UpdateCronJobFireTimestamps(id string, lastFireAt, nextFireAt int64) error {
	return a.store.UpdateCronJobFireTimestamps(id, lastFireAt, nextFireAt)
}

func (a *cronStoreAdapter) ListCronJobs(agentID string, includeDisabled bool) ([]*cron.CronJob, error) {
	pbJobs, err := a.store.ListCronJobs("", agentID, includeDisabled)
	if err != nil {
		return nil, err
	}
	result := make([]*cron.CronJob, len(pbJobs))
	for i, j := range pbJobs {
		result[i] = pbCronJobToCron(j)
	}
	return result, nil
}

func pbCronJobToCron(j *pb.CronJob) *cron.CronJob {
	return &cron.CronJob{
		ID:         j.Id,
		Name:       j.Name,
		Schedule:   j.Schedule,
		AgentID:    j.AgentId,
		Message:    j.Message,
		Enabled:    j.Enabled,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
		LastFireAt: j.LastFireAt,
		NextFireAt: j.NextFireAt,
	}
}
