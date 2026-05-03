package tools

import (
	"fmt"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
)

// RegisterCronTools registers the ScheduleCron tool.
func RegisterCronTools(reg *ToolRegistry, mnemo *memoclient.Client, agentID string) {
	reg.Register("ScheduleCron", handleScheduleCron(mnemo, agentID), ScheduleCronSchema)
}

// ScheduleCronSchema is the JSON schema for the ScheduleCron tool.
var ScheduleCronSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"action": map[string]interface{}{
			"type":        "string",
			"enum":         []string{"create", "list", "update", "delete"},
			"description":  "Action to perform: create a new cron job, list existing jobs, update a job, or delete a job",
		},
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable name for the cron job (required for create, optional for update)",
		},
		"schedule": map[string]interface{}{
			"type":        "string",
			"description": "Cron expression in 5-field format (minute hour day month weekday). Examples: '0 9 * * *' (daily at 9am), '*/15 * * * *' (every 15 min), '0 9 * * 1-5' (weekdays at 9am)",
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "The message/prompt to send to the agent when the job fires (required for create)",
		},
		"job_id": map[string]interface{}{
			"type":        "string",
			"description": "The ID of the cron job (required for update and delete)",
		},
		"enabled": map[string]interface{}{
			"type":        "boolean",
			"description": "Whether the cron job is active (default: true for create)",
		},
		"include_disabled": map[string]interface{}{
			"type":        "boolean",
			"description": "Whether to include disabled jobs when listing (default: false)",
		},
	},
	"required":             []string{"action"},
	"additionalProperties": false,
}

func handleScheduleCron(mnemo *memoclient.Client, agentID string) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		action := strArg(args, "action", "")
		if action == "" {
			return "action is required (create, list, update, or delete)", true
		}

		switch action {
		case "create":
			return cronCreate(mnemo, agentID, args)
		case "list":
			return cronList(mnemo, agentID, args)
		case "update":
			return cronUpdate(mnemo, args)
		case "delete":
			return cronDelete(mnemo, args)
		default:
			return fmt.Sprintf("unknown action %q: must be create, list, update, or delete", action), true
		}
	}
}

func cronCreate(mnemo *memoclient.Client, agentID string, args map[string]interface{}) (string, bool) {
	name := strArg(args, "name", "")
	schedule := strArg(args, "schedule", "")
	message := strArg(args, "message", "")

	if name == "" {
		return "name is required for create action", true
	}
	if schedule == "" {
		return "schedule is required for create action", true
	}
	if message == "" {
		return "message is required for create action", true
	}

	var enabled *bool
	if e, ok := args["enabled"]; ok {
		if b, ok := e.(bool); ok {
			enabled = &b
		}
	}

	ctx, cancel := withTimeout(DefaultTimeout)
	defer cancel()

	job, err := mnemo.CreateCronJob(ctx, name, schedule, agentID, message, enabled)
	if err != nil {
		return fmt.Sprintf("Error creating cron job: %v", err), true
	}

	status := "enabled"
	if !job.Enabled {
		status = "disabled"
	}
	return fmt.Sprintf("Created cron job %q (ID: %s)\nSchedule: %s\nMessage: %s\nStatus: %s",
		job.Name, job.ID, job.Schedule, job.Message, status), false
}

func cronList(mnemo *memoclient.Client, agentID string, args map[string]interface{}) (string, bool) {
	includeDisabled := boolArg(args, "include_disabled", false)

	ctx, cancel := withTimeout(DefaultTimeout)
	defer cancel()

	jobs, err := mnemo.ListCronJobs(ctx, agentID, includeDisabled)
	if err != nil {
		return fmt.Sprintf("Error listing cron jobs: %v", err), true
	}

	if len(jobs) == 0 {
		return "No cron jobs found.", false
	}

	result := fmt.Sprintf("Found %d cron job(s):\n", len(jobs))
	for _, job := range jobs {
		status := "✓"
		if !job.Enabled {
			status = "✗"
		}
		nextFire := "not scheduled"
		if job.NextFireAt > 0 {
			nextFire = time.Unix(job.NextFireAt, 0).Format(time.RFC3339)
		}
		lastFire := "never"
		if job.LastFireAt > 0 {
			lastFire = time.Unix(job.LastFireAt, 0).Format(time.RFC3339)
		}
		result += fmt.Sprintf("\n%s %s (ID: %s)\n  Schedule: %s\n  Message: %s\n  Next fire: %s\n  Last fire: %s\n",
			status, job.Name, job.ID, job.Schedule, job.Message, nextFire, lastFire)
	}
	return result, false
}

func cronUpdate(mnemo *memoclient.Client, args map[string]interface{}) (string, bool) {
	jobID := strArg(args, "job_id", "")
	if jobID == "" {
		return "job_id is required for update action", true
	}

	var name, schedule, message *string
	var enabled *bool

	if n := strArg(args, "name", ""); n != "" {
		name = &n
	}
	if s := strArg(args, "schedule", ""); s != "" {
		schedule = &s
	}
	if m := strArg(args, "message", ""); m != "" {
		message = &m
	}
	if e, ok := args["enabled"].(bool); ok {
		enabled = &e
	}

	if name == nil && schedule == nil && message == nil && enabled == nil {
		return "at least one field must be provided for update (name, schedule, message, or enabled)", true
	}

	ctx, cancel := withTimeout(DefaultTimeout)
	defer cancel()

	job, err := mnemo.UpdateCronJob(ctx, jobID, name, schedule, message, enabled)
	if err != nil {
		return fmt.Sprintf("Error updating cron job: %v", err), true
	}

	status := "enabled"
	if !job.Enabled {
		status = "disabled"
	}
	return fmt.Sprintf("Updated cron job %q (ID: %s)\nSchedule: %s\nMessage: %s\nStatus: %s",
		job.Name, job.ID, job.Schedule, job.Message, status), false
}

func cronDelete(mnemo *memoclient.Client, args map[string]interface{}) (string, bool) {
	jobID := strArg(args, "job_id", "")
	if jobID == "" {
		return "job_id is required for delete action", true
	}

	ctx, cancel := withTimeout(DefaultTimeout)
	defer cancel()

	err := mnemo.DeleteCronJob(ctx, jobID)
	if err != nil {
		return fmt.Sprintf("Error deleting cron job: %v", err), true
	}

	return fmt.Sprintf("Deleted cron job %s", jobID), false
}