package tool

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/approval"
	"openagent/internal/browser"
	"openagent/internal/domain"
	"openagent/internal/integration"
	"openagent/internal/metrics"
	"openagent/internal/repository"
)

// Executor handles tool calls with permission, schema, approval, audit
type Executor struct {
	repo        *repository.DB
	registry    *Registry
	integration *integration.Registry
	approvalSvc *approval.Service
	jwtSecret   string // for credential decrypt
	browserMgr  *browser.Manager
	browserExec *browser.ToolExecutor
}

func NewExecutor(repo *repository.DB, registry *Registry, integ *integration.Registry, approvalSvc *approval.Service, jwtSecret string) *Executor {
	return &Executor{repo: repo, registry: registry, integration: integ, approvalSvc: approvalSvc, jwtSecret: jwtSecret}
}

func (e *Executor) SetBrowserManager(m *browser.Manager) {
	e.browserMgr = m
	if m != nil {
		e.browserExec = browser.NewToolExecutor(m)
	}
}

type ExecuteParams struct {
	OrganizationID uuid.UUID
	AgentID        uuid.UUID
	TaskID         *uuid.UUID
	RunID          *uuid.UUID
	ToolName       string
	Input          map[string]any
	IdempotencyKey string // client-provided or generated
}

func (e *Executor) ExecuteApproved(ctx context.Context, execID uuid.UUID) (*domain.ToolExecution, error) {
	// Execute a tool that was previously approval_required, bypassing approval check and actually running the provider
	var orgID, agentID uuid.UUID
	var taskID, runID sql.NullString
	var toolID uuid.UUID
	var toolName string
	var inputJSON []byte
	var idemKey string
	err := e.repo.QueryRow(`SELECT organization_id, agent_id, task_id, run_id, tool_id, tool_name, input, idempotency_key FROM tool_executions WHERE id=$1`, execID).Scan(&orgID, &agentID, &taskID, &runID, &toolID, &toolName, &inputJSON, &idemKey)
	if err != nil {
		return nil, fmt.Errorf("load execution: %w", err)
	}
	var input map[string]any
	_ = json.Unmarshal(inputJSON, &input)
	if input == nil {
		input = map[string]any{}
	}
	var tid *uuid.UUID
	if taskID.Valid {
		if uid, err := uuid.Parse(taskID.String); err == nil { tid = &uid }
	}
	if runID.Valid {
		if _, err := uuid.Parse(runID.String); err != nil { _ = err }
	}
	// load tool
	tool, err := e.registry.Get(&orgID, toolName)
	if err != nil {
		tool, err = e.registry.Get(nil, toolName)
		if err != nil {
			return nil, fmt.Errorf("tool not found: %s", toolName)
		}
	}
	if !tool.Enabled {
		return nil, fmt.Errorf("tool disabled: %s", toolName)
	}
	if err := e.registry.ValidateInput(tool, input); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}
	// Re-validate permissions (agent still authorized, still in project if task exists)
	allowed := false
	if tools, err := e.repo.ListAgentTools(agentID); err == nil {
		for _, at := range tools {
			if at.Tool != nil && at.Tool.Name == toolName && at.Enabled { allowed = true; break }
			if at.ToolID == toolID { allowed = true; break }
		}
	}
	if !allowed {
		if agent, err := e.repo.GetAgent(orgID, agentID); err == nil && agent.Version != nil {
			for _, cap := range agent.Version.Capabilities {
				for _, t := range capabilityToTools(cap.Name) {
					if t == toolName { allowed = true; break }
				}
				if allowed { break }
			}
		}
		if !allowed {
			if agent, err := e.repo.GetAgent(orgID, agentID); err == nil {
				for _, t := range agent.Tools {
					if t == toolName { allowed = true; break }
				}
			}
		}
	}
	if !allowed {
		msg := fmt.Sprintf("agent not authorized for tool %s (re-check after approval)", toolName)
		_ = e.repo.UpdateToolExecutionStatus(execID, "failed", nil, &msg)
		return nil, fmt.Errorf("%s", msg)
	}
	// Also check project assignment if task exists
	if tid != nil {
		if task, err := e.repo.GetTask(orgID, *tid); err == nil {
			var inProject bool
			_ = e.repo.QueryRow(`SELECT EXISTS(SELECT 1 FROM project_agents WHERE project_id=$1 AND agent_id=$2)`, task.ProjectID, agentID).Scan(&inProject)
			if !inProject {
				_ = e.repo.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_team_members atm JOIN project_teams pt ON pt.team_id=atm.team_id WHERE pt.project_id=$1 AND atm.agent_id=$2)`, task.ProjectID, agentID).Scan(&inProject)
			}
			if !inProject {
				msg := fmt.Sprintf("agent %s not assigned to project %s", agentID.String()[:8], task.ProjectID.String()[:8])
				_ = e.repo.UpdateToolExecutionStatus(execID, "failed", nil, &msg)
				return nil, fmt.Errorf("%s", msg)
			}
		}
	}
	// Check idempotency guard: if already succeeded, don't re-execute
	var currentStatus string
	_ = e.repo.QueryRow(`SELECT status FROM tool_executions WHERE id=$1`, execID).Scan(&currentStatus)
	if currentStatus == "succeeded" {
		if existing, err := e.repo.GetToolExecutionByIdempotency(orgID, idemKey); err == nil {
			return existing, nil
		}
	}
	// Mark running
	_ = e.repo.UpdateToolExecutionStatus(execID, "running", nil, nil)
	metrics.IncToolExecutions()
	// Resolve credentials
	var creds map[string]any
	providerName := mapToolToProvider(tool.Name)
	if providerName != "" && e.integration != nil {
		if _, ok := e.integration.Get(providerName); ok {
			var credEnc sql.NullString
			_ = e.repo.QueryRow(`SELECT credentials_encrypted FROM integrations WHERE organization_id=$1 AND provider=$2 LIMIT 1`, orgID, providerName).Scan(&credEnc)
			if credEnc.Valid && credEnc.String != "" {
				if dec, err := integration.Decrypt(credEnc.String, e.jwtSecret); err == nil {
					creds = dec
				}
			}
		}
	}
	// Real web_search before mock
	if tool.Name == "web_search" {
		if realOut, realErr := tryRealWebSearch(ctx, input); realErr == nil && realOut != nil {
			_ = e.repo.UpdateToolExecutionStatus(execID, "succeeded", realOut, nil)
			_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`, orgID, agentID, "tool_execution", execID)
			updated, _ := e.repo.GetToolExecutionByIdempotency(orgID, idemKey)
			if updated == nil {
				updated = &domain.ToolExecution{ID: execID, Status: "succeeded", Output: realOut}
			}
			return updated, nil
		}
	}
	providerName = mapToolToProvider(tool.Name)
	var provider integration.Provider
	if e.integration != nil {
		if pvd, ok := e.integration.Get(providerName); ok {
			provider = pvd
		}
	}
	if provider == nil {
		provider = integration.NewMock(providerName)
		if provider.Name() == "" {
			provider = integration.NewMock(tool.Name)
		}
	}
	out, err := provider.Execute(ctx, tool.Name, input, creds)
	if err != nil {
		msg := err.Error()
		metrics.IncToolFailed()
		_ = e.repo.UpdateToolExecutionStatus(execID, "failed", nil, &msg)
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_failed',$3,$4)`, orgID, agentID, "tool_execution", execID)
		return nil, err
	}
	_ = e.repo.UpdateToolExecutionStatus(execID, "succeeded", out, nil)
	_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`, orgID, agentID, "tool_execution", execID)
	updated, _ := e.repo.GetToolExecutionByIdempotency(orgID, idemKey)
	if updated != nil {
		return updated, nil
	}
	return &domain.ToolExecution{ID: execID, Status: "succeeded", Output: out}, nil
}

func (e *Executor) Execute(ctx context.Context, p ExecuteParams) (*domain.ToolExecution, error) {
	// idempotency: check existing
	if p.IdempotencyKey == "" {
		// generate from org+agent+tool+input hash
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%v", p.OrganizationID, p.AgentID, p.ToolName, p.Input)))
		p.IdempotencyKey = fmt.Sprintf("%x", h[:8])
	}
	if existing, err := e.repo.GetToolExecutionByIdempotency(p.OrganizationID, p.IdempotencyKey); err == nil && existing != nil {
		// duplicate prevention: return existing without re-executing
		return existing, nil
	}

	// load tool
	tool, err := e.registry.Get(&p.OrganizationID, p.ToolName)
	if err != nil {
		// try global
		tool, err = e.registry.Get(nil, p.ToolName)
		if err != nil { return nil, fmt.Errorf("tool not found: %s", p.ToolName) }
	}
	if !tool.Enabled {
		return nil, fmt.Errorf("tool disabled: %s", p.ToolName)
	}

	// schema validation (no arbitrary code execution)
	if err := e.registry.ValidateInput(tool, p.Input); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}

	// permission check: agent must have tool assigned or required_permissions satisfied
	// For Phase 5, check agent_tools; if not assigned, check if agent has capability
	// Simplified: check if agent has tool in agent_tools or has any capability that maps
	allowed := false
	if tools, err := e.repo.ListAgentTools(p.AgentID); err == nil {
		for _, at := range tools {
			if at.Tool != nil && at.Tool.Name == p.ToolName && at.Enabled { allowed = true; break }
			if at.ToolID == tool.ID { allowed = true; break }
		}
	}
	// Enforce explicit assignment or capability-granted tool — no automatic low-risk fallback
	if !allowed {
		// Check if agent has a capability that grants this tool (e.g., research → web_search)
		if agent, err := e.repo.GetAgent(p.OrganizationID, p.AgentID); err == nil && agent.Version != nil {
			for _, cap := range agent.Version.Capabilities {
				for _, t := range capabilityToTools(cap.Name) {
					if t == p.ToolName {
						allowed = true
						break
					}
				}
				if allowed {
					break
				}
			}
		}
		// Also check legacy Capabilities field
		if !allowed {
			if agent, err := e.repo.GetAgent(p.OrganizationID, p.AgentID); err == nil {
				for _, t := range agent.Tools {
					if t == p.ToolName {
						allowed = true
						break
					}
				}
			}
		}
	}
	if !allowed {
		// create audit log for denied
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_denied',$3,$4)`,
			p.OrganizationID, p.AgentID, "tool", tool.ID)
		return nil, fmt.Errorf("agent not authorized for tool %s", p.ToolName)
	}

	// authorization hardening: project assignment check
	if p.TaskID != nil {
		if task, err := e.repo.GetTask(p.OrganizationID, *p.TaskID); err == nil {
			if task.ProjectID != (uuid.UUID{}) {
				// Agent must be assigned to project (via project_agents or team)
				assigned := false
				if rows, err2 := e.repo.Query(`SELECT 1 FROM project_agents WHERE project_id=$1 AND agent_id=$2`, task.ProjectID, p.AgentID); err2 == nil {
					if rows.Next() { assigned = true }
					rows.Close()
				}
				if !assigned {
					if task.TeamID != nil {
						if rows, err2 := e.repo.Query(`SELECT 1 FROM agent_team_members WHERE team_id=$1 AND agent_id=$2`, *task.TeamID, p.AgentID); err2 == nil {
							if rows.Next() { assigned = true }
							rows.Close()
						}
					}
				}
				// Also allow if agent is sole assignee of this task (already implies project trust)
				if !assigned && task.AssignedToAgent != nil && *task.AssignedToAgent == p.AgentID {
					assigned = true
				}
				if !assigned {
					_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_denied_project',$3,$4)`,
						p.OrganizationID, p.AgentID, "tool", tool.ID)
					return nil, fmt.Errorf("agent not assigned to project %s for tool %s", task.ProjectID.String()[:8], p.ToolName)
				}
			}
		}
	}
	// rate limiting via execution_budgets / tool_executions
	if err := e.checkRateLimits(p.OrganizationID, p.AgentID, tool.Name); err != nil {
		return nil, fmt.Errorf("rate limited: %w", err)
	}
	// approval check: high/critical or policy requires
	needsApproval := RequiresApproval(tool)
	var approvalID *uuid.UUID
	if needsApproval {
		// Persist correlation & run context for durable resume
		approvalContext := map[string]any{"tool": tool.Name, "risk": tool.RiskLevel, "idempotency_key": p.IdempotencyKey}
		if p.RunID != nil { approvalContext["run_id"] = p.RunID.String() }
		if p.TaskID != nil { approvalContext["task_id"] = p.TaskID.String() }
		if p.RunID != nil && p.TaskID != nil { approvalContext["resume_state"] = map[string]any{"run_id": p.RunID.String(), "task_id": p.TaskID.String(), "tool": tool.Name} }
		approval, err := e.approvalSvc.Create(
			p.OrganizationID, "agent", &p.AgentID, nil,
			"tool_execution", uuid.New(),
			fmt.Sprintf("Agent %s wants to %s", p.AgentID.String()[:8], tool.Name),
			fmt.Sprintf("Tool %s with input %v", tool.Name, RedactedInput(p.Input)),
			p.Input, tool.RiskLevel, tool.Name, p.ToolName, approvalContext, 24*time.Hour,
		)
		if err != nil { return nil, err }
		approvalID = &approval.ID
		// create tool execution in pending/approval_required with correlation
		exec := &domain.ToolExecution{
			OrganizationID: p.OrganizationID,
			AgentID:        p.AgentID,
			TaskID:         p.TaskID,
			RunID:          p.RunID,
			ToolID:         tool.ID,
			ToolName:       tool.Name,
			Input:          p.Input,
			Status:         "approval_required",
			ApprovalID:     approvalID,
			IdempotencyKey: p.IdempotencyKey,
		}
		created, err := e.repo.CreateToolExecution(exec)
		if err != nil { return nil, err }
		// update approval entity_id to point to execution and store execution id in context for recovery
		_, _ = e.repo.Exec(`UPDATE approvals SET entity_id=$2, context=$3 WHERE id=$1`, approval.ID, created.ID, fmt.Sprintf(`{"tool": "%s", "run_id": "%s", "execution_id": "%s", "task_id": "%s"}`, tool.Name, safeUUID(p.RunID), created.ID.String(), safeUUID(p.TaskID)))
		// audit
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_approval_requested',$3,$4)`,
			p.OrganizationID, p.AgentID, "tool_execution", created.ID)
		return created, fmt.Errorf("approval_required: %s", approval.ID.String())
	}

	// Browser tools: delegate to browser manager (must be before generic provider)
	if strings.HasPrefix(tool.Name, "browser.") {
		if e.browserMgr == nil || e.browserExec == nil {
			return nil, fmt.Errorf("browser capability unavailable: manager not configured (set PLAYWRIGHT_ENABLED or use mock)")
		}
		// Browser tools use existing approval risk (low/medium) — no extra approval here
		metrics.IncToolExecutions()
		metrics.IncBrowserToolCalls()
		// Persist execution as running
		exec := &domain.ToolExecution{
			OrganizationID: p.OrganizationID,
			AgentID:        p.AgentID,
			TaskID:         p.TaskID,
			RunID:          p.RunID,
			ToolID:         tool.ID,
			ToolName:       tool.Name,
			Input:          p.Input,
			Status:         "running",
			IdempotencyKey: p.IdempotencyKey,
		}
		created, err := e.repo.CreateToolExecution(exec)
		if err != nil {
			if existing, err2 := e.repo.GetToolExecutionByIdempotency(p.OrganizationID, p.IdempotencyKey); err2 == nil {
				return existing, nil
			}
			return nil, err
		}
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_started',$3,$4)`,
			p.OrganizationID, p.AgentID, "tool_execution", created.ID)
		// Extract profileId if present
		var profileID *uuid.UUID
		if pidStr, ok := p.Input["profileId"].(string); ok && pidStr != "" {
			if uid, err := uuid.Parse(pidStr); err == nil { profileID = &uid }
		}
		if pid, ok := p.Input["profile_id"].(string); ok && profileID == nil && pid != "" {
			if uid, err := uuid.Parse(pid); err == nil { profileID = &uid }
		}
		// Enforce profile access if profileId provided
		if profileID != nil {
			if err := e.browserMgr.EnforceAgentAccess(ctx, p.OrganizationID, p.AgentID, *profileID); err != nil {
				msg := err.Error()
				metrics.IncBrowserToolFailures()
				_ = e.repo.UpdateToolExecutionStatus(created.ID, "failed", nil, &msg)
				return created, fmt.Errorf("%s", msg)
			}
		}
		// Domain for audit
		domainStr := ""
		if u, ok := p.Input["url"].(string); ok { domainStr = browser.DomainOf(u) }
		out, execErr := e.browserExec.Execute(ctx, p.AgentID, p.OrganizationID, p.TaskID, p.RunID, profileID, tool.Name, p.Input)
		// Audit
		statusStr := "success"
		if execErr != nil { statusStr = "failed" }
		_ = e.browserMgr.RecordAudit(ctx, &domain.BrowserAuditLog{
			OrganizationID: p.OrganizationID, AgentID: &p.AgentID, TaskID: p.TaskID, RunID: p.RunID,
			BrowserProfileID: profileID, ToolName: tool.Name, Action: strings.TrimPrefix(tool.Name, "browser."),
			Target: strPtrIf(p.Input["url"]), Domain: strPtrIf(domainStr), ResultStatus: &statusStr,
			Metadata: map[string]any{"input": RedactedInput(p.Input), "output_success": out != nil && out["success"] == true},
		})
		if execErr != nil {
			msg := execErr.Error()
			metrics.IncBrowserToolFailures()
			_ = e.repo.UpdateToolExecutionStatus(created.ID, "failed", nil, &msg)
			_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_failed',$3,$4)`,
				p.OrganizationID, p.AgentID, "tool_execution", created.ID)
			created.Status = "failed"
			created.Error = &msg
			return created, execErr
		}
		// Success
		if out != nil {
			if s, ok := out["success"].(bool); ok && !s {
				metrics.IncBrowserToolFailures()
				// Still mark succeeded but with success=false payload (tool-level failure)
			} else {
				metrics.IncBrowserPagesVisited()
				if tool.Name == "browser.download" { metrics.IncBrowserDownloads() }
			}
		}
		_ = e.repo.UpdateToolExecutionStatus(created.ID, "succeeded", out, nil)
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
			p.OrganizationID, p.AgentID, "tool_execution", created.ID)
		created.Status = "succeeded"
		created.Output = out
		return created, nil
	}

	// No approval needed → execute
	metrics.IncToolExecutions()
	exec := &domain.ToolExecution{
		OrganizationID: p.OrganizationID,
		AgentID:        p.AgentID,
		TaskID:         p.TaskID,
		RunID:          p.RunID,
		ToolID:         tool.ID,
		ToolName:       tool.Name,
		Input:          p.Input,
		Status:         "running",
		IdempotencyKey: p.IdempotencyKey,
	}
	created, err := e.repo.CreateToolExecution(exec)
	if err != nil {
		// duplicate idempotency race: return existing
		if existing, err2 := e.repo.GetToolExecutionByIdempotency(p.OrganizationID, p.IdempotencyKey); err2 == nil {
			return existing, nil
		}
		return nil, err
	}
	// audit start
	_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_started',$3,$4)`,
		p.OrganizationID, p.AgentID, "tool_execution", created.ID)

	// Resolve credentials: find integration for this tool's provider if needed
	var creds map[string]any
	// For generic http_api, look for integration with provider http_api
	// For now, we try to find integration by tool name mapping
	providerName := mapToolToProvider(tool.Name)
	if providerName != "" && e.integration != nil {
		if prov, ok := e.integration.Get(providerName); ok {
			_ = prov
			// Find integration record for org + provider
			var credEnc sql.NullString
			err := e.repo.QueryRow(`SELECT credentials_encrypted FROM integrations WHERE organization_id=$1 AND provider=$2 LIMIT 1`, p.OrganizationID, providerName).Scan(&credEnc)
			if err == nil && credEnc.Valid && credEnc.String != "" {
				if dec, err := integration.Decrypt(credEnc.String, e.jwtSecret); err == nil {
					creds = dec
				}
			}
		}
	}

	// Real web_search via external API (Serper → DuckDuckGo fallback) before mock
	if tool.Name == "web_search" {
		if realOut, realErr := tryRealWebSearch(ctx, p.Input); realErr == nil && realOut != nil {
			_ = e.repo.UpdateToolExecutionStatus(created.ID, "succeeded", realOut, nil)
			_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
				p.OrganizationID, p.AgentID, "tool_execution", created.ID)
			created.Status = "succeeded"
			created.Output = realOut
			metrics.IncToolExecutions()
			return created, nil
		}
	}
	// Execute via provider
	var output map[string]any
	var execErr *string
	providerName = mapToolToProvider(tool.Name)
	var provider integration.Provider
	if e.integration != nil {
		if pvd, ok := e.integration.Get(providerName); ok {
			provider = pvd
		}
	}
	if provider == nil {
		// fallback to mock
		provider = integration.NewMock(providerName)
		if provider.Name() == "" { provider = integration.NewMock(tool.Name) }
	}
	out, err := provider.Execute(ctx, tool.Name, p.Input, creds)
	if err != nil {
		msg := err.Error()
		execErr = &msg
		metrics.IncToolFailed()
		_ = e.repo.UpdateToolExecutionStatus(created.ID, "failed", nil, execErr)
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_failed',$3,$4)`,
			p.OrganizationID, p.AgentID, "tool_execution", created.ID)
		created.Status = "failed"
		created.Error = execErr
		return created, err
	}
	output = out
	_ = e.repo.UpdateToolExecutionStatus(created.ID, "succeeded", output, nil)
	_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
		p.OrganizationID, p.AgentID, "tool_execution", created.ID)
	created.Status = "succeeded"
	created.Output = output
	return created, nil
}

func mapToolToProvider(toolName string) string {
	switch toolName {
	case "publish_social_post", "read_social_analytics":
		return "linkedin" // default, could be x/instagram etc. For abstraction we pick generic
	case "send_email":
		return "email"
	case "create_calendar_event":
		return "calendar"
	case "web_search":
		return "http_api"
	case "http_request":
		return "http_api"
	case "upload_file":
		return "google_drive"
	case "send_channel_message", "create_task":
		return "" // internal, no external provider
	default:
		return "http_api"
	}
}

func capabilityToTools(capName string) []string {
	switch capName {
	case "content_creation":
		return []string{"editor", "content_gen", "create_task"}
	case "publish_content":
		return []string{"publish_social_post", "social_publish", "cms_publish"}
	case "performance_analysis":
		return []string{"read_social_analytics", "analytics"}
	case "scheduling":
		return []string{"create_calendar_event", "scheduler"}
	case "research":
		return []string{"web_search", "search", "rag", "http_request"}
	case "communication":
		return []string{"send_channel_message", "messaging"}
	case "data_management":
		return []string{"upload_file", "database"}
	case "send_email":
		return []string{"send_email"}
	case "external_api":
		return []string{"http_request", "http_tool"}
	case "automation":
		return []string{"create_task", "workflow"}
	default:
		return nil
	}
}

func (e *Executor) checkRateLimits(orgID, agentID uuid.UUID, toolName string) error {
	var count int
	_ = e.repo.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE organization_id=$1 AND agent_id=$2 AND created_at > now() - interval '1 minute'`, orgID, agentID).Scan(&count)
	var limit int = 60
	_ = e.repo.QueryRow(`SELECT rate_limit_per_minute FROM execution_budgets WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID).Scan(&limit)
	if limit == 0 {
		limit = 60
	}
	if count >= limit {
		return fmt.Errorf("rate limit %d/min exceeded", limit)
	}
	var toolCount int
	_ = e.repo.QueryRow(`SELECT COUNT(*) FROM tool_executions WHERE organization_id=$1 AND agent_id=$2 AND tool_name=$3 AND created_at > now() - interval '1 day'`, orgID, agentID, toolName).Scan(&toolCount)
	var maxTools int = 500
	_ = e.repo.QueryRow(`SELECT max_tool_calls_per_day FROM execution_budgets WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID).Scan(&maxTools)
	if maxTools == 0 {
		maxTools = 500
	}
	if toolCount >= maxTools {
		return fmt.Errorf("daily tool limit %d exceeded for %s", maxTools, toolName)
	}
	return nil
}

func safeUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func strPtrIf(v any) *string {
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	if v == nil { return nil }
	s := fmt.Sprintf("%v", v)
	if s == "" || s == "<nil>" { return nil }
	return &s
}

func strPtr(s string) *string { return &s }

// ExecuteAfterApproval executes a tool execution that was previously approval-gated, bypassing the approval check
func (e *Executor) ExecuteAfterApproval(ctx context.Context, execID uuid.UUID) (*domain.ToolExecution, error) {
	// Load execution
	exec, err := e.repo.GetToolExecution(execID)
	if err != nil {
		return nil, err
	}
	if exec.Status != "approval_required" {
		return exec, nil // idempotent: already executed
	}
	// Re-validate authorization (agent still allowed?)
	tool, err := e.registry.Get(&exec.OrganizationID, exec.ToolName)
	if err != nil {
		tool, err = e.registry.Get(nil, exec.ToolName)
		if err != nil {
			return nil, fmt.Errorf("tool not found: %s", exec.ToolName)
		}
	}
	if err := e.registry.ValidateInput(tool, exec.Input); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}
	// Check rate limits again
	if err := e.checkRateLimits(exec.OrganizationID, exec.AgentID, exec.ToolName); err != nil {
		return nil, err
	}
	// Mark running
	_ = e.repo.UpdateToolExecutionStatus(exec.ID, "running", nil, nil)
	// Browser tools: delegate directly (no provider)
	if strings.HasPrefix(tool.Name, "browser.") {
		if e.browserMgr == nil || e.browserExec == nil {
			msg := "browser capability unavailable: manager not configured"
			_ = e.repo.UpdateToolExecutionStatus(exec.ID, "failed", nil, &msg)
			return nil, fmt.Errorf("%s", msg)
		}
		var profileID *uuid.UUID
		if pidStr, ok := exec.Input["profileId"].(string); ok && pidStr != "" {
			if uid, err := uuid.Parse(pidStr); err == nil { profileID = &uid }
		}
		if pid, ok := exec.Input["profile_id"].(string); ok && profileID == nil && pid != "" {
			if uid, err := uuid.Parse(pid); err == nil { profileID = &uid }
		}
		metrics.IncBrowserToolCalls()
		out, execErr := e.browserExec.Execute(ctx, exec.AgentID, exec.OrganizationID, &exec.ID, nil, profileID, tool.Name, exec.Input)
		statusStr := "success"
		if execErr != nil { statusStr = "failed" }
		domainStr := ""
		if u, ok := exec.Input["url"].(string); ok { domainStr = browser.DomainOf(u) }
		_ = e.browserMgr.RecordAudit(ctx, &domain.BrowserAuditLog{
			OrganizationID: exec.OrganizationID, AgentID: &exec.AgentID, TaskID: exec.TaskID, RunID: exec.RunID,
			BrowserProfileID: profileID, ToolName: tool.Name, Action: strings.TrimPrefix(tool.Name, "browser."),
			Target: strPtrIf(exec.Input["url"]), Domain: strPtrIf(domainStr), ResultStatus: &statusStr,
		})
		if execErr != nil {
			msg := execErr.Error()
			metrics.IncBrowserToolFailures()
			_ = e.repo.UpdateToolExecutionStatus(exec.ID, "failed", nil, &msg)
			_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_failed',$3,$4)`,
				exec.OrganizationID, exec.AgentID, "tool_execution", exec.ID)
			return nil, execErr
		}
		_ = e.repo.UpdateToolExecutionStatus(exec.ID, "succeeded", out, nil)
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
			exec.OrganizationID, exec.AgentID, "tool_execution", exec.ID)
		if updated, err := e.repo.GetToolExecution(exec.ID); err == nil {
			return updated, nil
		}
		exec.Output = out
		exec.Status = "succeeded"
		return exec, nil
	}
	// Resolve credentials
	var creds map[string]any
	providerName := mapToolToProvider(tool.Name)
	if providerName != "" && e.integration != nil {
		var credEnc sql.NullString
		_ = e.repo.QueryRow(`SELECT credentials_encrypted FROM integrations WHERE organization_id=$1 AND provider=$2 LIMIT 1`, exec.OrganizationID, providerName).Scan(&credEnc)
		if credEnc.Valid && credEnc.String != "" {
			if dec, err := integration.Decrypt(credEnc.String, e.jwtSecret); err == nil {
				creds = dec
			}
		}
	}
	// Real web_search path
	if tool.Name == "web_search" {
		if realOut, realErr := tryRealWebSearch(ctx, exec.Input); realErr == nil && realOut != nil {
			_ = e.repo.UpdateToolExecutionStatus(exec.ID, "succeeded", realOut, nil)
			_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
				exec.OrganizationID, exec.AgentID, "tool_execution", exec.ID)
			exec.Status = "succeeded"
			exec.Output = realOut
			return exec, nil
		}
	}
	var provider integration.Provider
	if e.integration != nil {
		if pvd, ok := e.integration.Get(providerName); ok {
			provider = pvd
		}
	}
	if provider == nil {
		provider = integration.NewMock(providerName)
		if provider.Name() == "" {
			provider = integration.NewMock(tool.Name)
		}
	}
	out, err := provider.Execute(ctx, tool.Name, exec.Input, creds)
	if err != nil {
		msg := err.Error()
		_ = e.repo.UpdateToolExecutionStatus(exec.ID, "failed", nil, &msg)
		_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_failed',$3,$4)`,
			exec.OrganizationID, exec.AgentID, "tool_execution", exec.ID)
		return nil, err
	}
	_ = e.repo.UpdateToolExecutionStatus(exec.ID, "succeeded", out, nil)
	_, _ = e.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, actor_agent_id, action, entity_type, entity_id) VALUES ($1,'agent',$2,'tool_succeeded',$3,$4)`,
		exec.OrganizationID, exec.AgentID, "tool_execution", exec.ID)
	// Reload to get output
	if updated, err := e.repo.GetToolExecution(execID); err == nil {
		return updated, nil
	}
	exec.Output = out
	exec.Status = "succeeded"
	return exec, nil
}

func RedactedInput(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		// never log secrets
		if isSecretKey(k) {
			out[k] = "***REDACTED***"
		} else {
			out[k] = v
		}
	}
	return out
}

func isSecretKey(k string) bool {
	lower := strings.ToLower(k)
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") || strings.Contains(lower, "password")
}

func tryRealWebSearch(ctx context.Context, input map[string]any) (map[string]any, error) {
	query, _ := input["query"].(string)
	if query == "" {
		query, _ = input["q"].(string)
	}
	if query == "" {
		return nil, fmt.Errorf("query required")
	}
	limit := 5
	if l, ok := input["limit"].(float64); ok && l > 0 && l < 10 {
		limit = int(l)
	}
	if l, ok := input["limit"].(int); ok && l > 0 {
		limit = l
	}
	// Try Serper if key available
	if apiKey := os.Getenv("SERPER_API_KEY"); apiKey != "" {
		body, _ := json.Marshal(map[string]any{"q": query, "num": limit})
		req, err := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search", strings.NewReader(string(body)))
		if err == nil {
			req.Header.Set("X-API-KEY", apiKey)
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				if resp.StatusCode == 200 {
					var parsed map[string]any
					if err := json.Unmarshal(respBody, &parsed); err == nil {
						// Normalize to {results: [{title, link, snippet}]}
						if organic, ok := parsed["organic"].([]any); ok {
							var results []map[string]any
							for i, item := range organic {
								if i >= limit { break }
								if m, ok := item.(map[string]any); ok {
									results = append(results, map[string]any{
										"title":   m["title"],
										"link":    m["link"],
										"snippet": m["snippet"],
									})
								}
							}
							return map[string]any{"results": results, "source": "serper"}, nil
						}
					}
				}
			}
		}
	}
	// Fallback: DuckDuckGo
	// Use query param encoding
	encoded := url.QueryEscape(query)
	duckURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", encoded)
	req, err := http.NewRequestWithContext(ctx, "GET", duckURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("duckduckgo %d", resp.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var results []map[string]any
	if abs, ok := parsed["AbstractText"].(string); ok && abs != "" {
		results = append(results, map[string]any{"title": parsed["Heading"], "link": parsed["AbstractURL"], "snippet": abs})
	}
	if related, ok := parsed["RelatedTopics"].([]any); ok {
		for i, item := range related {
			if i >= limit- len(results) { break }
			if m, ok := item.(map[string]any); ok {
				if txt, ok := m["Text"].(string); ok && txt != "" {
					results = append(results, map[string]any{"title": m["Text"], "link": m["FirstURL"], "snippet": txt})
				}
			}
		}
	}
	if len(results) == 0 {
		// Still return empty but not error, so mock isn't used unnecessarily
		return map[string]any{"results": []any{}, "query": query, "note": "no results from duckduckgo, try serper"}, nil
	}
	return map[string]any{"results": results, "source": "duckduckgo"}, nil
}
