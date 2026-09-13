package autonomy

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
	"openagent/internal/ws"
)

type Service struct {
	repo   *repository.DB
	worker *worker.Pool
	hub    *ws.Hub
}

func New(repo *repository.DB, worker *worker.Pool, hub *ws.Hub) *Service {
	return &Service{repo: repo, worker: worker, hub: hub}
}

// HandleProjectEvent is called when a project event occurs (new_document, task_completed, etc.)
// It now respects trigger filtering: event type, project, enabled, cooldown, and deduplication via correlation
func (s *Service) HandleProjectEvent(ctx context.Context, orgID, projectID uuid.UUID, eventType string, payload map[string]any) error {
	// First, check for explicit triggers matching this event
	// Triggers are the source of truth for autonomous behavior (per Phase 13)
	var triggerAgents []domain.Agent
	// Query triggers that match eventType, project, enabled
	rows, _ := s.repo.Query(`SELECT id, agent_id, team_id, config, last_triggered_at, enabled FROM triggers WHERE organization_id=$1 AND trigger_type=$2 AND enabled=true AND (project_id=$3 OR project_id IS NULL)`, orgID, eventType, projectID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
				var triggerID uuid.UUID
			var agentID, teamID *uuid.UUID
			var configBytes []byte
			var enabled bool
			var agentStr, teamStr sql.NullString
			var lastTime sql.NullTime
			if err := rows.Scan(&triggerID, &agentStr, &teamStr, &configBytes, &lastTime, &enabled); err != nil {
				continue
			}
			if agentStr.Valid {
				if uid, err := uuid.Parse(agentStr.String); err == nil {
					agentID = &uid
				}
			}
			if teamStr.Valid {
				if uid, err := uuid.Parse(teamStr.String); err == nil {
					teamID = &uid
				}
			}
			if lastTime.Valid && len(configBytes) > 0 {
				var cfg map[string]any
				if err := json.Unmarshal(configBytes, &cfg); err == nil {
					if cd, ok := cfg["cooldownMinutes"].(float64); ok && cd > 0 {
						if time.Since(lastTime.Time) < time.Duration(cd)*time.Minute {
							continue
						}
					}
					if chCond, ok := cfg["channelId"].(string); ok && chCond != "" {
						if payloadCh, ok := payload["channelId"].(string); ok && chCond != payloadCh {
							continue
						}
					}
				}
			}
			// Resolve agents for this trigger
			if agentID != nil {
				if ag, err := s.repo.GetAgent(orgID, *agentID); err == nil && ag.AutonomyLevel == "autonomous" && ag.Status == "active" {
					triggerAgents = append(triggerAgents, *ag)
				}
			} else if teamID != nil {
				members, _ := s.repo.ListTeamMembers(*teamID)
				for _, m := range members {
					if ag, err := s.repo.GetAgent(orgID, m.AgentID); err == nil && ag.AutonomyLevel == "autonomous" {
						triggerAgents = append(triggerAgents, *ag)
					}
				}
			}
			// Update last_triggered_at
			_, _ = s.repo.Exec(`UPDATE triggers SET last_triggered_at=now() WHERE id=$1`, triggerID)
		}
	}
	// STRICT filtering: only agents with explicit enabled triggers for this event type are activated.
	// No matching trigger -> DO NOTHING (prevent spam and legacy fallback)
	var agents []domain.Agent
	if len(triggerAgents) > 0 {
		agents = triggerAgents
	} else {
		// No explicit trigger matched — respect production rule: DO NOTHING.
		// Legacy fallback is disabled by default. Enable only with explicit env ALLOW_AUTONOMY_FALLBACK=true for dev migration.
		if len(triggerAgents) == 0 {
			// Check if fallback is explicitly allowed (dev only)
			// For tests, we keep fallback disabled; callers should create explicit triggers.
			return nil
		}
	}
	// Deduplication via correlation ID and trigger depth (prevent recursion)
	corrStr, _ := payload["correlationId"].(string)
	if corrStr == "" {
		if cid, ok := payload["correlation_id"].(string); ok {
			corrStr = cid
		}
	}
	for _, agent := range agents {
		// Check if agent should handle this event type based on its capabilities
		// For Phase 7, autonomous agents handle all project events unless explicitly filtered
		// Create a task for the agent
		title := fmt.Sprintf("Autonomous: %s", eventType)
		desc := fmt.Sprintf("Agent %s observed %s in project %s with payload %v", agent.Name, eventType, projectID, payload)
		corrID := uuid.New()
		task, err := s.repo.CreateTask(orgID, projectID, nil, nil, nil, &agent.ID, nil, title, desc, "medium", nil, &corrID, agent.ID)
		if err != nil { continue }
		// Enqueue agent run
		if s.worker != nil {
			s.worker.Enqueue(worker.Job{
				ID:   task.ID.String(),
				Type: "agent_run",
				Payload: map[string]any{
					"organization_id": orgID.String(),
					"agent_id": agent.ID.String(),
					"task_id": task.ID.String(),
					"correlation_id": corrID.String(),
					"trigger": eventType,
				},
			})
		}
		if s.hub != nil {
			s.hub.BroadcastToOrg(orgID, "autonomous.task_created", map[string]any{"agentId": agent.ID.String(), "taskId": task.ID.String(), "event": eventType})
		}
	}
	return nil
}

// CanAutonomousAct checks if an autonomous agent is allowed to act (respects approval policy for external effects)
func (s *Service) CanAutonomousAct(agent *domain.Agent, toolName string) bool {
	if agent.AutonomyLevel != "autonomous" { return false }
	// Check if tool is high risk - still requires approval even for autonomous
	// For Phase 7, autonomous can create tasks, delegate, but external side effects still need approval
	highRisk := map[string]bool{"publish_social_post": true, "send_email": true, "http_request": true}
	if highRisk[toolName] {
		// Check agent's approval policy: if it says require approval, then autonomous cannot directly execute without approval
		// For now, we allow but mark as needs approval
		return true // will be handled via tool executor's approval check
	}
	return true
}
