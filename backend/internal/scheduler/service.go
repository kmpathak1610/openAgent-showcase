package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
	"openagent/internal/worker"
)

type Service struct {
	repo   *repository.DB
	worker *worker.Pool
}

func New(repo *repository.DB, worker *worker.Pool) *Service {
	return &Service{repo: repo, worker: worker}
}

func (s *Service) CreateTrigger(orgID uuid.UUID, name, triggerType string, config map[string]any, agentID, teamID, projectID *uuid.UUID, createdBy uuid.UUID) (*domain.Trigger, error) {
	if !domain.ValidTriggerTypes[triggerType] {
		return nil, fmt.Errorf("invalid trigger_type")
	}
	cfgJSON, _ := json.Marshal(config)
	var nextRun *time.Time
	if triggerType == "schedule" {
		if scheduledAt, ok := config["scheduledAt"].(string); ok && scheduledAt != "" {
			// one-time
			if t, err := time.Parse(time.RFC3339, scheduledAt); err == nil {
				nextRun = &t
			}
		} else if cron, ok := config["cron"].(string); ok && cron != "" {
			// recurring: compute next run in timezone
			tzStr, _ := config["timezone"].(string)
			if tzStr == "" { tzStr = "UTC" }
			loc, _ := time.LoadLocation(tzStr)
			if loc == nil { loc = time.UTC }
			// For Phase 7 demo, support simple "0 9 * * 1" for Monday 9am
			// Compute next Monday 9am in timezone
			now := time.Now().In(loc)
			next := nextWeekly(cron, now)
			if next != nil {
				nextRun = next
			} else {
				t := time.Now().Add(1 * time.Minute)
				nextRun = &t
			}
		}
	}
	var t domain.Trigger
	var cfg []byte
	var lastTriggered sql.NullTime
	var nextRunSQL sql.NullTime
	if nextRun != nil { nextRunSQL = sql.NullTime{Time: *nextRun, Valid: true} }
	err := s.repo.QueryRow(
		`INSERT INTO triggers (organization_id, name, trigger_type, config, agent_id, team_id, project_id, created_by, next_run_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, organization_id, name, trigger_type, config, enabled, agent_id, team_id, project_id, created_by, last_triggered_at, next_run_at, created_at, updated_at`,
		orgID, name, triggerType, cfgJSON, agentID, teamID, projectID, createdBy, nextRunSQL,
	).Scan(&t.ID, &t.OrganizationID, &t.Name, &t.TriggerType, &cfg, &t.Enabled, &t.AgentID, &t.TeamID, &t.ProjectID, &t.CreatedBy, &lastTriggered, &nextRunSQL, &t.CreatedAt, &t.UpdatedAt)
	if err != nil { return nil, err }
	if len(cfg)>0 { _ = json.Unmarshal(cfg, &t.Config) }
	if lastTriggered.Valid { t.LastTriggeredAt = &lastTriggered.Time }
	if nextRunSQL.Valid { t.NextRunAt = &nextRunSQL.Time }
	return &t, nil
}

func nextWeekly(cron string, now time.Time) *time.Time {
	// Support "0 9 * * 1" => Monday 9am, and "weekly:mon@09:00"
	if cron == "0 9 * * 1" || cron == "weekly:mon@09:00" {
		// Find next Monday
		daysUntilMonday := (8 - int(now.Weekday())) % 7
		if daysUntilMonday == 0 && now.Hour() >= 9 {
			daysUntilMonday = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 9, 0, 0, 0, now.Location())
		return &next
	}
	// Default: next minute
	t := now.Add(time.Minute)
	return &t
}

func (s *Service) ListTriggers(orgID uuid.UUID) ([]*domain.Trigger, error) {
	rows, err := s.repo.Query(`SELECT id, organization_id, name, trigger_type, config, enabled, agent_id, team_id, project_id, created_by, last_triggered_at, next_run_at, created_at, updated_at FROM triggers WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Trigger
	for rows.Next() {
		var t domain.Trigger
		var cfg []byte
		var lastTriggered, nextRun sql.NullTime
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.TriggerType, &cfg, &t.Enabled, &t.AgentID, &t.TeamID, &t.ProjectID, &t.CreatedBy, &lastTriggered, &nextRun, &t.CreatedAt, &t.UpdatedAt); err != nil { return nil, err }
		if len(cfg)>0 { _ = json.Unmarshal(cfg, &t.Config) }
		if lastTriggered.Valid { t.LastTriggeredAt = &lastTriggered.Time }
		if nextRun.Valid { t.NextRunAt = &nextRun.Time }
		out = append(out, &t)
	}
	return out, nil
}

func (s *Service) GetTrigger(orgID, id uuid.UUID) (*domain.Trigger, error) {
	var t domain.Trigger
	var cfg []byte
	var lastTriggered, nextRun sql.NullTime
	err := s.repo.QueryRow(`SELECT id, organization_id, name, trigger_type, config, enabled, agent_id, team_id, project_id, created_by, last_triggered_at, next_run_at, created_at, updated_at FROM triggers WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(
		&t.ID, &t.OrganizationID, &t.Name, &t.TriggerType, &cfg, &t.Enabled, &t.AgentID, &t.TeamID, &t.ProjectID, &t.CreatedBy, &lastTriggered, &nextRun, &t.CreatedAt, &t.UpdatedAt)
	if err != nil { return nil, err }
	if len(cfg)>0 { _ = json.Unmarshal(cfg, &t.Config) }
	if lastTriggered.Valid { t.LastTriggeredAt = &lastTriggered.Time }
	if nextRun.Valid { t.NextRunAt = &nextRun.Time }
	return &t, nil
}

func (s *Service) DeleteTrigger(orgID, id uuid.UUID) error {
	_, err := s.repo.Exec(`DELETE FROM triggers WHERE id=$1 AND organization_id=$2`, id, orgID)
	return err
}

// ProcessDueTriggers claims due schedule triggers atomically (exactly-once even with multiple workers)
// Uses UPDATE ... WHERE next_run_at <= now() RETURNING to claim, so only one worker succeeds per trigger.
func (s *Service) ProcessDueTriggers(ctx context.Context) (int, error) {
	// Atomically claim due triggers: UPDATE with condition next_run_at <= now() and return claimed rows
	// We use a transaction per trigger with SELECT FOR UPDATE SKIP LOCKED for durability, but simpler: UPDATE ... WHERE ... RETURNING
	// For recurring we compute nextRun before claiming, then update to new nextRun or null/disable in same statement.
	rows, err := s.repo.Query(`SELECT id, organization_id, name, config, agent_id, team_id, project_id, next_run_at FROM triggers WHERE enabled=true AND trigger_type='schedule' AND next_run_at IS NOT NULL AND next_run_at <= now() FOR UPDATE SKIP LOCKED`)
	var dueIDs []struct {
		id uuid.UUID
		orgID uuid.UUID
		name string
		config []byte
		agentID sql.NullString
		teamID sql.NullString
		projectID sql.NullString
		nextRun sql.NullTime
	}
	if err == nil && rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, orgID uuid.UUID
			var name string
			var cfg []byte
			var aID, tID, pID sql.NullString
			var nr sql.NullTime
			if err := rows.Scan(&id, &orgID, &name, &cfg, &aID, &tID, &pID, &nr); err != nil { continue }
			dueIDs = append(dueIDs, struct {
				id uuid.UUID
				orgID uuid.UUID
				name string
				config []byte
				agentID sql.NullString
				teamID sql.NullString
				projectID sql.NullString
				nextRun sql.NullTime
			}{id, orgID, name, cfg, aID, tID, pID, nr})
		}
	} else if err != nil {
		// Fallback to non-locking query if SKIP LOCKED not supported (e.g., mock)
		rows2, err2 := s.repo.Query(`SELECT id, organization_id, name, config, agent_id, team_id, project_id, next_run_at FROM triggers WHERE enabled=true AND trigger_type='schedule' AND next_run_at IS NOT NULL AND next_run_at <= now()`)
		if err2 != nil { return 0, err2 }
		defer rows2.Close()
		for rows2.Next() {
			var id, orgID uuid.UUID
			var name string
			var cfg []byte
			var aID, tID, pID sql.NullString
			var nr sql.NullTime
			if err := rows2.Scan(&id, &orgID, &name, &cfg, &aID, &tID, &pID, &nr); err != nil { continue }
			dueIDs = append(dueIDs, struct {
				id uuid.UUID
				orgID uuid.UUID
				name string
				config []byte
				agentID sql.NullString
				teamID sql.NullString
				projectID sql.NullString
				nextRun sql.NullTime
			}{id, orgID, name, cfg, aID, tID, pID, nr})
		}
	}
	count := 0
	for _, rec := range dueIDs {
		var cfg map[string]any
		_ = json.Unmarshal(rec.config, &cfg)
		title, _ := cfg["title"].(string)
		if title == "" { title = rec.name }
		desc, _ := cfg["description"].(string)
		var projID *uuid.UUID
		if rec.projectID.Valid { uid, _ := uuid.Parse(rec.projectID.String); projID = &uid }
		var agID *uuid.UUID
		if rec.agentID.Valid { uid, _ := uuid.Parse(rec.agentID.String); agID = &uid }
		var tmID *uuid.UUID
		if rec.teamID.Valid { uid, _ := uuid.Parse(rec.teamID.String); tmID = &uid }
		scheduledAt := time.Now()
		if rec.nextRun.Valid { scheduledAt = rec.nextRun.Time }
		timezone, _ := cfg["timezone"].(string)
		if timezone == "" { timezone = "UTC" }
		recurrence, _ := cfg["cron"].(string)

		// Atomically claim: UPDATE only if still due (next_run_at <= now()), return rows affected
		var claimedID uuid.UUID
		var nextRunVal sql.NullTime
		var lastTriggered sql.NullTime
		// Compute nextRun for recurring
		var newNextRun *time.Time
		if cron, ok := cfg["cron"].(string); ok && cron != "" {
			tzStr, _ := cfg["timezone"].(string)
			if tzStr == "" { tzStr = "UTC" }
			loc, _ := time.LoadLocation(tzStr)
			if loc == nil { loc = time.UTC }
			newNextRun = nextWeekly(cron, time.Now().In(loc))
		}
		if newNextRun != nil {
			nextRunVal = sql.NullTime{Time: *newNextRun, Valid: true}
		}
		// Use transaction to claim and create scheduled_task atomically
		tx, err := s.repo.Begin()
		if err != nil { continue }
		// Try to claim by updating trigger — only succeeds if still due
		var resClaimed uuid.UUID
		claimQuery := `UPDATE triggers SET last_triggered_at=now(), updated_at=now() WHERE id=$1 AND next_run_at <= now() AND enabled=true RETURNING id`
		if newNextRun != nil {
			claimQuery = `UPDATE triggers SET last_triggered_at=now(), next_run_at=$2, updated_at=now() WHERE id=$1 AND next_run_at <= now() AND enabled=true RETURNING id`
			err = tx.QueryRow(claimQuery, rec.id, *newNextRun).Scan(&resClaimed)
		} else if cfg["cron"] != nil && cfg["cron"] != "" {
			claimQuery = `UPDATE triggers SET last_triggered_at=now(), next_run_at=NULL, updated_at=now() WHERE id=$1 AND next_run_at <= now() AND enabled=true RETURNING id`
			err = tx.QueryRow(claimQuery, rec.id).Scan(&resClaimed)
		} else {
			// one-time: disable
			claimQuery = `UPDATE triggers SET last_triggered_at=now(), next_run_at=NULL, enabled=false, updated_at=now() WHERE id=$1 AND next_run_at <= now() AND enabled=true RETURNING id`
			err = tx.QueryRow(claimQuery, rec.id).Scan(&resClaimed)
		}
		if err != nil {
			_ = tx.Rollback()
			continue // another worker claimed first
		}
		// Insert into scheduled_tasks for audit
		var scheduledID uuid.UUID
		err = tx.QueryRow(
			`INSERT INTO scheduled_tasks (organization_id, trigger_id, project_id, team_id, agent_id, title, description, scheduled_at, timezone, recurrence) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			rec.orgID, rec.id, projID, tmID, agID, title, desc, scheduledAt, timezone, recurrence,
		).Scan(&scheduledID)
		if err != nil {
			_ = tx.Rollback()
			continue
		}
		// Create real task if project specified
		if projID != nil {
			var createdBy uuid.UUID
			_ = tx.QueryRow(`SELECT created_by FROM triggers WHERE id=$1`, rec.id).Scan(&createdBy)
			corrID := uuid.New()
			_, err = tx.Exec(`INSERT INTO tasks (organization_id, project_id, team_id, title, description, status, priority, correlation_id, created_by) VALUES ($1,$2,$3,$4,$5,'pending','medium',$6,$7)`,
				rec.orgID, *projID, tmID, title, desc, corrID, createdBy)
			if err == nil && agID != nil && s.worker != nil {
				var taskID uuid.UUID
				_ = tx.QueryRow(`SELECT id FROM tasks WHERE correlation_id=$1 ORDER BY created_at DESC LIMIT 1`, corrID).Scan(&taskID)
				if taskID != uuid.Nil {
					// Commit before enqueue to ensure task visible
					_ = tx.Commit()
					s.worker.Enqueue(worker.Job{
						ID:   taskID.String(),
						Type: "agent_run",
						Payload: map[string]any{
							"organization_id": rec.orgID.String(),
							"agent_id": agID.String(),
							"task_id": taskID.String(),
							"correlation_id": corrID.String(),
							"trigger": "schedule",
						},
					})
					count++
					continue
				}
			}
		}
		_ = tx.Commit()
		_ = claimedID
		_ = lastTriggered
		_ = nextRunVal
		count++
	}
	return count, nil
}
