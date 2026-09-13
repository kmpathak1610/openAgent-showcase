package approval

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
)

type Service struct {
	repo *repository.DB
}

func New(repo *repository.DB) *Service { return &Service{repo: repo} }

func (s *Service) Create(orgID uuid.UUID, requesterType string, requesterAgentID, requesterUserID *uuid.UUID, entityType string, entityID uuid.UUID, title, description string, payload map[string]any, riskLevel, action, target string, context map[string]any, expiresIn time.Duration) (*domain.Approval, error) {
	if riskLevel == "" { riskLevel = "medium" }
	if !domain.ValidRiskLevels[riskLevel] { riskLevel = "medium" }
	if action == "" { action = title }
	expiresAt := time.Now().Add(expiresIn)
	if expiresIn == 0 { expiresAt = time.Now().Add(24 * time.Hour) }
	payloadJSON, _ := json.Marshal(payload)
	contextJSON, _ := json.Marshal(context)
	var approval domain.Approval
	var risk, act, tgt sql.NullString
	var ctxJSON []byte
	var expires sql.NullTime
	err := s.repo.QueryRow(
		`INSERT INTO approvals (organization_id, requester_type, requester_agent_id, requester_user_id, entity_type, entity_id, title, description, payload, risk_level, action, target, context, status, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'pending',$14)
		 RETURNING id, organization_id, requester_type, requester_agent_id, requester_user_id, entity_type, entity_id, title, description, payload, risk_level, action, target, context, status, decided_by, decided_at, expires_at, created_at, updated_at`,
		orgID, requesterType, requesterAgentID, requesterUserID, entityType, entityID, title, description, payloadJSON, riskLevel, action, target, contextJSON, expiresAt,
	).Scan(&approval.ID, &approval.OrganizationID, &approval.RequesterType, &approval.RequesterAgentID, &approval.RequesterUserID, &approval.EntityType, &approval.EntityID, &approval.Title, &approval.Description, &payloadJSON, &risk, &act, &tgt, &ctxJSON, &approval.Status, &approval.DecidedBy, &approval.DecidedAt, &expires, &approval.CreatedAt, &approval.UpdatedAt)
	if err != nil { return nil, err }
	if risk.Valid { approval.RiskLevel = risk.String }
	if act.Valid { approval.Action = act.String }
	if tgt.Valid { approval.Target = tgt.String }
	if len(ctxJSON)>0 { _ = json.Unmarshal(ctxJSON, &approval.Context) }
	if len(payloadJSON)>0 { _ = json.Unmarshal(payloadJSON, &approval.Payload) }
	if expires.Valid { approval.ExpiresAt = &expires.Time }
	return &approval, nil
}

func (s *Service) Decide(orgID, approvalID uuid.UUID, decidedBy uuid.UUID, decision string) (*domain.Approval, error) {
	if !domain.ValidApprovalStatuses[decision] { return nil, fmt.Errorf("invalid status %s", decision) }
	if decision == "pending" { return nil, fmt.Errorf("cannot decide to pending") }
	// ensure exists and org isolation
	var currentStatus string
	var expiresAt sql.NullTime
	err := s.repo.QueryRow(`SELECT status, expires_at FROM approvals WHERE id=$1 AND organization_id=$2`, approvalID, orgID).Scan(&currentStatus, &expiresAt)
	if err != nil { return nil, err }
	if currentStatus != "pending" { return nil, fmt.Errorf("approval already decided: %s", currentStatus) }
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		// auto-expire
		_, _ = s.repo.Exec(`UPDATE approvals SET status='expired', updated_at=now() WHERE id=$1`, approvalID)
		return nil, fmt.Errorf("approval expired")
	}
	var approval domain.Approval
	var payloadJSON, ctxJSON []byte
	var risk, act, tgt sql.NullString
	var expires sql.NullTime
	var decidedAt sql.NullTime
	var decidedByID sql.NullString
	err = s.repo.QueryRow(
		`UPDATE approvals SET status=$3, decided_by=$4, decided_at=now(), updated_at=now() WHERE id=$1 AND organization_id=$2 RETURNING id, organization_id, requester_type, requester_agent_id, requester_user_id, entity_type, entity_id, title, description, payload, risk_level, action, target, context, status, decided_by, decided_at, expires_at, created_at, updated_at`,
		approvalID, orgID, decision, decidedBy,
	).Scan(&approval.ID, &approval.OrganizationID, &approval.RequesterType, &approval.RequesterAgentID, &approval.RequesterUserID, &approval.EntityType, &approval.EntityID, &approval.Title, &approval.Description, &payloadJSON, &risk, &act, &tgt, &ctxJSON, &approval.Status, &decidedByID, &decidedAt, &expires, &approval.CreatedAt, &approval.UpdatedAt)
	if err != nil { return nil, err }
	if risk.Valid { approval.RiskLevel = risk.String }
	if act.Valid { approval.Action = act.String }
	if tgt.Valid { approval.Target = tgt.String }
	if len(payloadJSON)>0 { _ = json.Unmarshal(payloadJSON, &approval.Payload) }
	if len(ctxJSON)>0 { _ = json.Unmarshal(ctxJSON, &approval.Context) }
	if decidedByID.Valid { uid, _ := uuid.Parse(decidedByID.String); approval.DecidedBy = &uid }
	if decidedAt.Valid { approval.DecidedAt = &decidedAt.Time }
	if expires.Valid { approval.ExpiresAt = &expires.Time }
	return &approval, nil
}

func (s *Service) Get(orgID, id uuid.UUID) (*domain.Approval, error) {
	var a domain.Approval
	var payloadJSON, ctxJSON []byte
	var risk, act, tgt sql.NullString
	var expires, decidedAt sql.NullTime
	var decidedBy sql.NullString
	err := s.repo.QueryRow(`SELECT id, organization_id, requester_type, requester_agent_id, requester_user_id, entity_type, entity_id, title, description, payload, risk_level, action, target, context, status, decided_by, decided_at, expires_at, created_at, updated_at FROM approvals WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(
		&a.ID, &a.OrganizationID, &a.RequesterType, &a.RequesterAgentID, &a.RequesterUserID, &a.EntityType, &a.EntityID, &a.Title, &a.Description, &payloadJSON, &risk, &act, &tgt, &ctxJSON, &a.Status, &decidedBy, &decidedAt, &expires, &a.CreatedAt, &a.UpdatedAt)
	if err != nil { return nil, err }
	if risk.Valid { a.RiskLevel = risk.String }
	if act.Valid { a.Action = act.String }
	if tgt.Valid { a.Target = tgt.String }
	if len(payloadJSON)>0 { _ = json.Unmarshal(payloadJSON, &a.Payload) }
	if len(ctxJSON)>0 { _ = json.Unmarshal(ctxJSON, &a.Context) }
	if decidedBy.Valid { uid,_:=uuid.Parse(decidedBy.String); a.DecidedBy=&uid }
	if decidedAt.Valid { a.DecidedAt=&decidedAt.Time }
	if expires.Valid { a.ExpiresAt=&expires.Time }
	return &a, nil
}

func (s *Service) List(orgID uuid.UUID, status string, limit int) ([]*domain.Approval, error) {
	if limit<=0 || limit>50 { limit=20 }
	query := `SELECT id, organization_id, requester_type, requester_agent_id, requester_user_id, entity_type, entity_id, title, description, payload, risk_level, action, target, context, status, decided_by, decided_at, expires_at, created_at, updated_at FROM approvals WHERE organization_id=$1`
	args := []any{orgID}
	idx:=2
	if status!="" {
		query += ` AND status=$`+itoa(idx)
		args=append(args, status)
		idx++
	}
	query += ` ORDER BY created_at DESC LIMIT $`+itoa(idx)
	args=append(args, limit)
	rows, err := s.repo.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Approval
	for rows.Next() {
		var a domain.Approval
		var payloadJSON, ctxJSON []byte
		var risk, act, tgt sql.NullString
		var expires, decidedAt sql.NullTime
		var decidedBy sql.NullString
		if err:=rows.Scan(&a.ID,&a.OrganizationID,&a.RequesterType,&a.RequesterAgentID,&a.RequesterUserID,&a.EntityType,&a.EntityID,&a.Title,&a.Description,&payloadJSON,&risk,&act,&tgt,&ctxJSON,&a.Status,&decidedBy,&decidedAt,&expires,&a.CreatedAt,&a.UpdatedAt); err!=nil { return nil, err }
		if risk.Valid { a.RiskLevel=risk.String }
		if act.Valid { a.Action=act.String }
		if tgt.Valid { a.Target=tgt.String }
		if len(payloadJSON)>0 { _ = json.Unmarshal(payloadJSON, &a.Payload) }
		if len(ctxJSON)>0 { _ = json.Unmarshal(ctxJSON, &a.Context) }
		if decidedBy.Valid { uid,_:=uuid.Parse(decidedBy.String); a.DecidedBy=&uid }
		if decidedAt.Valid { a.DecidedAt=&decidedAt.Time }
		if expires.Valid { a.ExpiresAt=&expires.Time }
		out=append(out, &a)
	}
	return out, nil
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

// ExpirePending checks and expires approvals past expires_at (called periodically)
func (s *Service) ExpirePending() (int64, error) {
	res, err := s.repo.Exec(`UPDATE approvals SET status='expired', updated_at=now() WHERE status='pending' AND expires_at IS NOT NULL AND expires_at < now()`)
	if err != nil { return 0, err }
	return res.RowsAffected()
}
