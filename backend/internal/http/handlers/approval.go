package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/approval"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/tool"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

type ApprovalHandler struct {
	repo       *repository.DB
	approval   *approval.Service
	toolExec   *tool.Executor
	hub        *ws.Hub
	workerPool *worker.Pool
}

func NewApprovalHandler(repo *repository.DB, svc *approval.Service, exec *tool.Executor, hub *ws.Hub) *ApprovalHandler {
	return &ApprovalHandler{repo: repo, approval: svc, toolExec: exec, hub: hub}
}

func NewApprovalHandlerWithWorker(repo *repository.DB, svc *approval.Service, exec *tool.Executor, hub *ws.Hub, pool *worker.Pool) *ApprovalHandler {
	return &ApprovalHandler{repo: repo, approval: svc, toolExec: exec, hub: hub, workerPool: pool}
}

func (h *ApprovalHandler) SetWorkerPool(pool *worker.Pool) { h.workerPool = pool }

func (h *ApprovalHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	status := r.URL.Query().Get("status")
	approvals, err := h.approval.List(claims.OrganizationID, status, 20)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if approvals == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, approvals, nil)
}

func (h *ApprovalHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	a, err := h.approval.Get(claims.OrganizationID, id)
	if err != nil { writeError(w, 404, "NOT_FOUND", "approval not found"); return }
	writeData(w, 200, a, nil)
}

func (h *ApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	approval, err := h.approval.Decide(claims.OrganizationID, id, claims.UserID, "approved")
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	if approval.EntityType == "tool_execution" {
		rows, _ := h.repo.Query(`SELECT id, organization_id, agent_id, task_id, run_id, tool_name FROM tool_executions WHERE approval_id=$1`, approval.ID)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var execID, orgID, agentID uuid.UUID
				var taskIDNull, runIDNull sql.NullString
				var toolName string
				if err := rows.Scan(&execID, &orgID, &agentID, &taskIDNull, &runIDNull, &toolName); err != nil {
					continue
				}
				// Actually execute the tool now (persist real result, idempotent)
				execResult, execErr := h.toolExec.ExecuteApproved(r.Context(), execID)
				if execErr != nil {
					if h.hub != nil {
						h.hub.BroadcastToOrg(claims.OrganizationID, "tool.failed", map[string]any{"approvalId": approval.ID.String(), "tool": toolName, "error": execErr.Error()})
					}
					// still resume agent so LLM sees actual error
				} else {
					if h.hub != nil {
						h.hub.BroadcastToOrg(claims.OrganizationID, "tool.approved", map[string]any{"approvalId": approval.ID.String(), "tool": toolName, "output": execResult.Output})
						h.hub.BroadcastToOrg(claims.OrganizationID, "agent.tool_completed", map[string]any{"tool": toolName, "approvalId": approval.ID.String(), "output": execResult.Output})
					}
				}
				// Resume the SAME logical AgentRun: update run + task, enqueue resume_run with run_id
				if taskIDNull.Valid {
					if tid, err := uuid.Parse(taskIDNull.String); err == nil {
						// Mark task back to running so agent can continue
						_, _ = h.repo.UpdateTaskStatus(orgID, tid, "running", "agent", agentID, map[string]any{"resumed_after_approval": approval.ID.String(), "tool_result": execResult})
						var resumeRunID *uuid.UUID
						if runIDNull.Valid {
							if rid, err := uuid.Parse(runIDNull.String); err == nil {
								resumeRunID = &rid
								// Persist resume state in same run
								_ = h.repo.UpdateAgentRunWaiting(orgID, rid, "running", 0, nil, nil, map[string]any{"approval_resumed": approval.ID.String(), "tool_result": execResult})
								// Also ensure standard status
								_ = h.repo.UpdateAgentRunStatus(orgID, rid, "running", map[string]any{"approval_resumed": approval.ID.String()}, nil)
							}
						}
						if h.workerPool != nil {
							payload := map[string]any{
								"organization_id": orgID.String(),
								"agent_id":        agentID.String(),
								"task_id":         tid.String(),
								"correlation_id":  approval.ID.String(),
								"trigger":         "approval_received",
							}
							if resumeRunID != nil {
								payload["run_id"] = resumeRunID.String()
								h.workerPool.Enqueue(worker.Job{
									ID:   resumeRunID.String() + "-resume-" + approval.ID.String()[:8],
									Type: "resume_run",
									Payload: payload,
								})
							} else {
								h.workerPool.Enqueue(worker.Job{
									ID:   tid.String() + "-resume-" + approval.ID.String()[:8],
									Type: "agent_run",
									Payload: payload,
								})
							}
						}
						if h.hub != nil {
							h.hub.BroadcastToOrg(orgID, "agent.resumed", map[string]any{"taskId": tid.String(), "approvalId": approval.ID.String()})
						}
					}
				}
			}
		}
	}
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "approval.decided", map[string]any{"approval": approval, "decision": "approved"})
	}
	writeData(w, 200, approval, nil)
}

func (h *ApprovalHandler) Reject(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ Reason string `json:"reason"` }
	_ = json.NewDecoder(r.Body).Decode(&req)
	approval, err := h.approval.Decide(claims.OrganizationID, id, claims.UserID, "rejected")
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	// If tool execution pending, mark it failed and resume agent with rejection
	rows, _ := h.repo.Query(`SELECT id, organization_id, agent_id, task_id, run_id FROM tool_executions WHERE approval_id=$1`, approval.ID)
	var resumeTasks []struct {
		execID uuid.UUID
		orgID  uuid.UUID
		agentID uuid.UUID
		taskID *uuid.UUID
		runID  *uuid.UUID
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var execID, orgID, agentID uuid.UUID
			var taskNull, runNull sql.NullString
			rows.Scan(&execID, &orgID, &agentID, &taskNull, &runNull)
			errMsg := "rejected by " + claims.UserID.String()
			if req.Reason != "" { errMsg = req.Reason }
			_ = h.repo.UpdateToolExecutionStatus(execID, "failed", nil, &errMsg)
			var tid, rid *uuid.UUID
			if taskNull.Valid { if uid, err := uuid.Parse(taskNull.String); err == nil { tid = &uid } }
			if runNull.Valid { if uid, err := uuid.Parse(runNull.String); err == nil { rid = &uid } }
			resumeTasks = append(resumeTasks, struct {
				execID uuid.UUID
				orgID  uuid.UUID
				agentID uuid.UUID
				taskID *uuid.UUID
				runID  *uuid.UUID
			}{execID, orgID, agentID, tid, rid})
		}
	}
	// Resume agents so they can choose alternative
	for _, rt := range resumeTasks {
		if rt.taskID != nil {
			_, _ = h.repo.UpdateTaskStatus(rt.orgID, *rt.taskID, "running", "agent", rt.agentID, map[string]any{"rejected_approval": approval.ID.String(), "reason": req.Reason})
			if rt.runID != nil {
				_ = h.repo.UpdateAgentRunStatus(rt.orgID, *rt.runID, "running", map[string]any{"rejection": req.Reason, "approval": approval.ID.String()}, nil)
				// Log rejection as action for debugger visibility
				_, _ = h.repo.CreateAgentAction(*rt.runID, 999, "approval_rejected", nil, map[string]any{"approval_id": approval.ID.String(), "reason": req.Reason}, map[string]any{"rejected": true}, "failed", &approval.ID)
			}
			if h.workerPool != nil {
				payload := map[string]any{
					"organization_id": rt.orgID.String(),
					"agent_id":        rt.agentID.String(),
					"task_id":         rt.taskID.String(),
					"correlation_id":  approval.ID.String(),
					"trigger":         "approval_rejected",
				}
				if rt.runID != nil {
					payload["run_id"] = rt.runID.String()
					h.workerPool.Enqueue(worker.Job{
						ID:   rt.runID.String() + "-rejected-" + approval.ID.String()[:8],
						Type: "resume_run",
						Payload: payload,
					})
				} else {
					h.workerPool.Enqueue(worker.Job{
						ID:   rt.taskID.String() + "-rejected-" + approval.ID.String()[:8],
						Type: "agent_run",
						Payload: payload,
					})
				}
			}
			if h.hub != nil {
				h.hub.BroadcastToOrg(rt.orgID, "agent.resumed", map[string]any{"taskId": rt.taskID.String(), "approvalId": approval.ID.String(), "decision": "rejected"})
			}
		}
	}
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "approval.decided", map[string]any{"approval": approval, "decision": "rejected"})
		h.hub.BroadcastToOrg(claims.OrganizationID, "tool.rejected", map[string]any{"approvalId": approval.ID.String()})
	}
	writeData(w, 200, approval, nil)
}

func (h *ApprovalHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	approval, err := h.approval.Decide(claims.OrganizationID, id, claims.UserID, "cancelled")
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", err.Error()); return }
	writeData(w, 200, approval, nil)
}
