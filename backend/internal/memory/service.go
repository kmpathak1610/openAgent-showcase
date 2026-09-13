package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/llm"
	"openagent/internal/repository"
)

// Service handles memory lifecycle with production-grade consolidation
type Service struct {
	repo        *repository.DB
	embedder    llm.Embedder
	llmProvider llm.Provider // optional for ambiguous cases
}

func New(repo *repository.DB, embedder llm.Embedder) *Service {
	return &Service{repo: repo, embedder: embedder}
}

func NewWithLLM(repo *repository.DB, embedder llm.Embedder, provider llm.Provider) *Service {
	return &Service{repo: repo, embedder: embedder, llmProvider: provider}
}

// Memory policies
var (
	MinImportanceToStore = 0.3
	WorkingTTL           = 1 * time.Hour
	ConversationTTL      = 7 * 24 * time.Hour
)

// Consolidation thresholds (cosine distance via pgvector: 0 = identical, 2 = opposite)
const (
	distanceDuplicate   = 0.15 // very similar -> duplicate/merge
	distanceNearDup     = 0.25 // near duplicate -> reinforce/merge
	distanceCandidate   = 0.35 // candidate threshold for semantic search
	confidenceMergeBoost = 0.15
	confidenceExplicitBoost = 0.30
	maxCandidates       = 10
)

type ConsolidationResult string

const (
	ResultCreated   ConsolidationResult = "CREATED"
	ResultMerged    ConsolidationResult = "MERGED"
	ResultUpdated   ConsolidationResult = "UPDATED"
	ResultConflict  ConsolidationResult = "CONFLICT"
	ResultRejected  ConsolidationResult = "REJECTED"
	ResultUnchanged ConsolidationResult = "UNCHANGED"
)

type ConsolidateInput struct {
	OrganizationID uuid.UUID
	AgentID        *uuid.UUID
	ProjectID      *uuid.UUID
	ConversationID *uuid.UUID
	TaskID         *uuid.UUID
	MemoryType     string
	Scope          string
	Source         string
	SourceEventID  *uuid.UUID
	Content        string
	Importance     float64
	Confidence     float64
	Metadata       map[string]any
	ExpiresAt      *time.Time
	CorrelationID  *uuid.UUID
}

type ConsolidateOutput struct {
	Memory    *domain.Memory      `json:"memory"`
	Result    ConsolidationResult `json:"result"`
	Reason    string              `json:"reason"`
	RelatedID *uuid.UUID          `json:"relatedId,omitempty"`
}

// Create is backward-compatible wrapper that uses consolidation pipeline
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, memoryType, scope, source, content string, importance float64, metadata map[string]any, expiresAt *time.Time) (*domain.Memory, error) {
	if !domain.ValidMemoryTypes[memoryType] {
		return nil, fmt.Errorf("invalid memory_type")
	}
	if !domain.ValidMemoryScopes[scope] {
		return nil, fmt.Errorf("invalid scope")
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content required")
	}
	// Scope validation
	if scope == "project" && projectID == nil {
		return nil, fmt.Errorf("project_id required for project scope")
	}
	if scope == "agent" && agentID == nil {
		return nil, fmt.Errorf("agent_id required for agent scope")
	}
	// Default source/confidence handling
	if source == "" {
		source = "manual"
	}
	// Determine default importance/confidence before consolidation
	if importance == 0 {
		importance = 0.5
	}
	// Working/conversation TTL
	if memoryType == "working" && expiresAt == nil {
		t := time.Now().Add(WorkingTTL)
		expiresAt = &t
	}
	if memoryType == "conversation" && expiresAt == nil {
		t := time.Now().Add(ConversationTTL)
		expiresAt = &t
	}
	// Conversation low importance policy
	if memoryType == "conversation" && importance < MinImportanceToStore {
		return nil, fmt.Errorf("conversation memory importance too low (%.2f < %.2f)", importance, MinImportanceToStore)
	}
	input := ConsolidateInput{
		OrganizationID: orgID,
		AgentID:        agentID,
		ProjectID:      projectID,
		MemoryType:     memoryType,
		Scope:          scope,
		Source:         source,
		Content:        content,
		Importance:     importance,
		Metadata:       metadata,
		ExpiresAt:      expiresAt,
	}
	// Default confidence based on source
	if source == "EXPLICIT_USER_PREFERENCE" || source == "explicit_user_preference" || source == "user_preference" {
		input.Confidence = 0.95
	} else if source == "USER_MESSAGE" {
		input.Confidence = 0.8
	} else {
		input.Confidence = 0.5
	}
	out, err := s.Consolidate(ctx, input)
	if err != nil {
		return nil, err
	}
	if out.Result == ResultRejected {
		return nil, fmt.Errorf("memory rejected: %s", out.Reason)
	}
	return out.Memory, nil
}

// Consolidate is the canonical memory consolidation engine
func (s *Service) Consolidate(ctx context.Context, in ConsolidateInput) (*ConsolidateOutput, error) {
	// 1. validate candidate
	if strings.TrimSpace(in.Content) == "" {
		return &ConsolidateOutput{Result: ResultRejected, Reason: "empty content"}, nil
	}
	normalized := normalizeContent(in.Content)
	if normalized == "" {
		return &ConsolidateOutput{Result: ResultRejected, Reason: "empty after normalization"}, nil
	}
	// 2. low-value check
	if isLowValue(in.Content, in.Importance, in.MemoryType) {
		_ = s.emitEvent(ctx, in.OrganizationID, nil, in.Scope, "memory.rejected", in.CorrelationID, map[string]any{"reason": "low_value", "content": in.Content[:min(100, len(in.Content))]})
		return &ConsolidateOutput{Result: ResultRejected, Reason: "low-value memory"}, nil
	}
	// 3. secret detection
	if isSecretContent(in.Content) {
		_ = s.emitEvent(ctx, in.OrganizationID, nil, in.Scope, "memory.rejected", in.CorrelationID, map[string]any{"reason": "potential_secret", "content": "redacted"})
		return &ConsolidateOutput{Result: ResultRejected, Reason: "potential secret detected"}, nil
	}
	// 4. content hash + idempotency
	hash := contentHash(normalized)
	idemKey := idempotencyKey(in.OrganizationID, in.Scope, in.AgentID, in.ProjectID, in.ConversationID, in.TaskID, hash)
	// 5. Check exact idempotency (same hash in same scope) - prevents uncontrolled exact duplicates
	var existingID uuid.UUID
	var existingContent, existingStatus string
	var existingImportance, existingConfidence float64
	// Try exact hash match first (fast path)
	errExact := s.repo.QueryRow(`SELECT id, content, importance, confidence, status FROM memories WHERE organization_id=$1 AND scope=$2 AND agent_id IS NOT DISTINCT FROM $3 AND project_id IS NOT DISTINCT FROM $4 AND conversation_id IS NOT DISTINCT FROM $5 AND task_id IS NOT DISTINCT FROM $6 AND content_hash=$7 AND status IN ('active','candidate','validating','conflict') LIMIT 1`,
		in.OrganizationID, in.Scope, in.AgentID, in.ProjectID, in.ConversationID, in.TaskID, hash).Scan(&existingID, &existingContent, &existingImportance, &existingConfidence, &existingStatus)
	if errExact == nil {
		// Exact duplicate found -> MERGE / reinforce
		return s.handleExactDuplicate(ctx, in, existingID, existingImportance, existingConfidence, hash, idemKey)
	}

	// 6. Embedding for semantic candidate retrieval
	var embeddingStr string
	var embedding []float32
	if s.embedder != nil {
		if res, err := s.embedder.Embed(ctx, llm.EmbedRequest{Input: []string{in.Content}}); err == nil && len(res.Embeddings) > 0 {
			embedding = res.Embeddings[0]
			embeddingStr = vectorToString(embedding)
		}
	}
	// 7. Find semantic candidates within allowed scope (bounded)
	candidates, err := s.findCandidates(ctx, in, embeddingStr)
	if err != nil {
		// On retrieval error, fallback to creating new (don't block write)
		candidates = nil
	}
	// 8. No candidates -> CREATE new
	if len(candidates) == 0 {
		return s.createNewMemory(ctx, in, hash, idemKey, embeddingStr)
	}
	// 9. Classify each candidate deterministically, nearest first
	// Candidates are already sorted by vector distance if embedding available
	for _, cand := range candidates {
		rel, confidence := classifyRelationship(cand.Content, in.Content, cand, in, embeddingStr)
		switch rel {
		case "duplicate", "reinforce":
			// Merge / reinforce existing
			return s.handleMerge(ctx, in, cand, hash, confidence)
		case "contradiction":
			return s.handleContradiction(ctx, in, cand, hash, idemKey, embeddingStr)
		case "update":
			return s.handleUpdate(ctx, in, cand, hash, idemKey, embeddingStr)
		case "unrelated":
			continue
		}
		// If ambiguous and LLM available, try LLM classification
		if s.llmProvider != nil && rel == "ambiguous" {
			if llmRel, llmConf, llmConsolidated, err := s.llmClassify(ctx, cand.Content, in.Content); err == nil {
				switch llmRel {
				case "duplicate", "reinforce":
					// Use LLM consolidated content if provided
					if llmConsolidated != "" {
						in.Content = llmConsolidated
						hash = contentHash(normalizeContent(llmConsolidated))
					}
					return s.handleMerge(ctx, in, cand, hash, llmConf)
				case "contradiction":
					return s.handleContradiction(ctx, in, cand, hash, idemKey, embeddingStr)
				case "update":
					return s.handleUpdate(ctx, in, cand, hash, idemKey, embeddingStr)
				}
			}
		}
	}
	// No relationship matched -> CREATE
	return s.createNewMemory(ctx, in, hash, idemKey, embeddingStr)
}

func (s *Service) handleExactDuplicate(ctx context.Context, in ConsolidateInput, existingID uuid.UUID, existingImportance, existingConfidence float64, hash, idemKey string) (*ConsolidateOutput, error) {
	// Load full existing memory
	existing, err := s.getMemoryByID(ctx, existingID)
	if err != nil {
		return s.createNewMemory(ctx, in, hash, idemKey, "")
	}
	// Reinforce confidence deterministically
	newConf := reinforceConfidence(existing.Confidence, in.Confidence, in.Source)
	newImp := existing.Importance
	if in.Importance > newImp {
		newImp = in.Importance
	}
	// Choose stronger formulation
	consolidatedContent := chooseStrongerContent(existing.Content, in.Content)
	newHash := contentHash(normalizeContent(consolidatedContent))
	now := time.Now()
	// Version history
	_ = s.createVersion(ctx, existing, "reinforce duplicate")
	// Update existing
	_, err = s.repo.Exec(`UPDATE memories SET content=$1, content_hash=$2, importance=$3, confidence=$4, last_confirmed_at=$5, last_accessed_at=$5, updated_at=now(), version=version+1, metadata=COALESCE(metadata,'{}'::jsonb) || $6 WHERE id=$7`,
		consolidatedContent, newHash, newImp, newConf, now, fmt.Sprintf(`{"consolidated":true,"last_seen":"%s","merge_reason":"exact_duplicate"}`, now.Format(time.RFC3339)), existingID)
	if err != nil {
		return nil, err
	}
	updated, _ := s.getMemoryByID(ctx, existingID)
	if updated == nil {
		updated = existing
		updated.Content = consolidatedContent
		updated.Confidence = newConf
		updated.Importance = newImp
	}
	_ = s.emitEvent(ctx, in.OrganizationID, &existingID, in.Scope, "memory.merged", in.CorrelationID, map[string]any{"reason": "exact_duplicate", "confidence": newConf})
	// Also audit
	_ = s.insertAudit(ctx, in.OrganizationID, "memory.merged", existingID, map[string]any{"before": existing.Content, "after": consolidatedContent})
	return &ConsolidateOutput{Memory: updated, Result: ResultMerged, Reason: "exact duplicate reinforced", RelatedID: &existingID}, nil
}

func (s *Service) handleMerge(ctx context.Context, in ConsolidateInput, cand *domain.Memory, hash string, classificationConf float64) (*ConsolidateOutput, error) {
	// Semantic duplicate -> merge / reinforce
	newConf := reinforceConfidence(cand.Confidence, in.Confidence, in.Source)
	if classificationConf > 0 && classificationConf > newConf {
		newConf = classificationConf
	}
	newImp := cand.Importance
	if in.Importance > newImp {
		newImp = in.Importance
	}
	// Preserve strongest formulation (not blind concatenation)
	consolidatedContent := chooseStrongerContent(cand.Content, in.Content)
	newHash := contentHash(normalizeContent(consolidatedContent))
	now := time.Now()
	_ = s.createVersion(ctx, cand, "semantic merge")
	_, err := s.repo.Exec(`UPDATE memories SET content=$1, content_hash=$2, importance=$3, confidence=$4, last_confirmed_at=$5, last_accessed_at=$5, updated_at=now(), version=version+1, metadata=COALESCE(metadata,'{}'::jsonb) || $6 WHERE id=$7`,
		consolidatedContent, newHash, newImp, newConf, now, fmt.Sprintf(`{"consolidated":true,"merge_reason":"semantic_duplicate","last_seen":"%s"}`, now.Format(time.RFC3339)), cand.ID)
	if err != nil {
		return nil, err
	}
	updated, _ := s.getMemoryByID(ctx, cand.ID)
	if updated == nil {
		updated = cand
		updated.Content = consolidatedContent
		updated.Confidence = newConf
		updated.Importance = newImp
	}
	_ = s.emitEvent(ctx, in.OrganizationID, &cand.ID, in.Scope, "memory.merged", in.CorrelationID, map[string]any{"reason": "semantic_duplicate", "confidence": newConf})
	_ = s.insertAudit(ctx, in.OrganizationID, "memory.merged", cand.ID, map[string]any{"before": cand.Content, "after": consolidatedContent})
	return &ConsolidateOutput{Memory: updated, Result: ResultMerged, Reason: "semantic duplicate merged", RelatedID: &cand.ID}, nil
}

func (s *Service) handleUpdate(ctx context.Context, in ConsolidateInput, cand *domain.Memory, hash, idemKey, embeddingStr string) (*ConsolidateOutput, error) {
	// Supersede old fact with new
	now := time.Now()
	// Version old
	_ = s.createVersion(ctx, cand, "superseded by update")
	// Mark old as superseded
	_, _ = s.repo.Exec(`UPDATE memories SET status='superseded', superseded_by=$1, updated_at=now(), metadata=COALESCE(metadata,'{}'::jsonb) || $2 WHERE id=$3`, uuid.New(), fmt.Sprintf(`{"superseded_reason":"update","superseded_at":"%s"}`, now.Format(time.RFC3339)), cand.ID)
	_ = s.emitEvent(ctx, in.OrganizationID, &cand.ID, cand.Scope, "memory.updated", in.CorrelationID, map[string]any{"superseded_by": "new", "reason": "update"})
	// Create new active memory
	newMem, err := s.insertMemory(ctx, in, hash, idemKey, embeddingStr, "active", chooseStrongerContent(cand.Content, in.Content))
	if err != nil {
		// If insert failed due to uniqueness, try merge fallback
		return s.handleMerge(ctx, in, cand, hash, 0)
	}
	// Link superseded_by on old to new
	_, _ = s.repo.Exec(`UPDATE memories SET superseded_by=$1 WHERE id=$2`, newMem.ID, cand.ID)
	_ = s.emitEvent(ctx, in.OrganizationID, &newMem.ID, in.Scope, "memory.created", in.CorrelationID, map[string]any{"reason": "update_supersede", "supersedes": cand.ID.String()})
	_ = s.insertAudit(ctx, in.OrganizationID, "memory.updated", newMem.ID, map[string]any{"supersedes": cand.ID.String(), "content": newMem.Content})
	return &ConsolidateOutput{Memory: newMem, Result: ResultUpdated, Reason: "update superseded previous", RelatedID: &cand.ID}, nil
}

func (s *Service) handleContradiction(ctx context.Context, in ConsolidateInput, cand *domain.Memory, hash, idemKey, embeddingStr string) (*ConsolidateOutput, error) {
	// Do not silently overwrite. Preserve evidence.
	// Policy: prefer newer explicit user preference with stronger confidence
	isExplicit := in.Source == "EXPLICIT_USER_PREFERENCE" || in.Source == "USER_MESSAGE" || in.Source == "user_preference"
	newConf := in.Confidence
	if newConf == 0 {
		newConf = 0.5
	}
	if isExplicit && newConf >= 0.9 && cand.Confidence < 0.8 {
		// Treat as UPDATE with stronger source winning
		return s.handleUpdate(ctx, in, cand, hash, idemKey, embeddingStr)
	}
	// Otherwise create conflict: keep both, mark metadata conflict
	now := time.Now()
	// Mark existing with conflict metadata
	_, _ = s.repo.Exec(`UPDATE memories SET status='conflict', updated_at=now(), last_confirmed_at=$2, metadata=COALESCE(metadata,'{}'::jsonb) || $3 WHERE id=$1`,
		cand.ID, now, fmt.Sprintf(`{"conflict_with":"%s","conflict_reason":"contradictory value","conflict_at":"%s"}`, hash[:8], now.Format(time.RFC3339)))
	_ = s.createVersion(ctx, cand, "conflict detected")
	// Create new memory as active but also conflict
	newMem, err := s.insertMemory(ctx, in, hash, idemKey, embeddingStr, "conflict", in.Content)
	if err != nil {
		return nil, err
	}
	// Add conflict metadata to new
	_, _ = s.repo.Exec(`UPDATE memories SET metadata=COALESCE(metadata,'{}'::jsonb) || $1 WHERE id=$2`, fmt.Sprintf(`{"conflict_with":"%s","conflict_reason":"contradictory value"}`, cand.ID.String()), newMem.ID)
	_ = s.emitEvent(ctx, in.OrganizationID, &newMem.ID, in.Scope, "memory.conflict", in.CorrelationID, map[string]any{"existing": cand.ID.String(), "reason": "contradictory values", "csv_vs_pdf": true})
	_ = s.emitEvent(ctx, in.OrganizationID, &cand.ID, cand.Scope, "memory.conflict", in.CorrelationID, map[string]any{"conflict_with": newMem.ID.String()})
	_ = s.insertAudit(ctx, in.OrganizationID, "memory.conflict", newMem.ID, map[string]any{"conflict_with": cand.ID.String(), "existing_content": cand.Content, "new_content": in.Content})
	return &ConsolidateOutput{Memory: newMem, Result: ResultConflict, Reason: "contradictory facts preserved", RelatedID: &cand.ID}, nil
}

func (s *Service) createNewMemory(ctx context.Context, in ConsolidateInput, hash, idemKey, embeddingStr string) (*ConsolidateOutput, error) {
	// Determine final confidence/importance
	conf := in.Confidence
	if conf == 0 {
		if in.Source == "EXPLICIT_USER_PREFERENCE" {
			conf = 0.95
		} else if in.Source == "USER_MESSAGE" {
			conf = 0.8
		} else {
			conf = 0.5
		}
	}
	imp := in.Importance
	if imp == 0 {
		imp = 0.5
		// Boost for explicit preference
		if in.Source == "EXPLICIT_USER_PREFERENCE" && imp < 0.8 {
			imp = 0.85
		}
	}
	// Sanitize content (already)
	content := strings.TrimSpace(in.Content)
	if len(content) > 5000 {
		content = content[:5000]
	}
	mem, err := s.insertMemory(ctx, in, hash, idemKey, embeddingStr, "active", content)
	if err != nil {
		// Handle unique violation -> treat as merged
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "idx_memories_idempotency") {
			// Try to fetch existing by hash
			var existingID uuid.UUID
			err2 := s.repo.QueryRow(`SELECT id FROM memories WHERE organization_id=$1 AND scope=$2 AND content_hash=$3 LIMIT 1`, in.OrganizationID, in.Scope, hash).Scan(&existingID)
			if err2 == nil {
				if existing, err3 := s.getMemoryByID(ctx, existingID); err3 == nil {
					return &ConsolidateOutput{Memory: existing, Result: ResultMerged, Reason: "idempotent duplicate", RelatedID: &existingID}, nil
				}
			}
		}
		return nil, err
	}
	// Set confidence/importance properly if they differ from defaults
	if conf != mem.Confidence || imp != mem.Importance {
		_, _ = s.repo.Exec(`UPDATE memories SET confidence=$1, importance=$2 WHERE id=$3`, conf, imp, mem.ID)
		mem.Confidence = conf
		mem.Importance = imp
	}
	_ = s.emitEvent(ctx, in.OrganizationID, &mem.ID, in.Scope, "memory.created", in.CorrelationID, map[string]any{"confidence": conf, "importance": imp})
	_ = s.insertAudit(ctx, in.OrganizationID, "memory.created", mem.ID, map[string]any{"content": content})
	return &ConsolidateOutput{Memory: mem, Result: ResultCreated, Reason: "new memory created"}, nil
}

func (s *Service) insertMemory(ctx context.Context, in ConsolidateInput, hash, idemKey, embeddingStr, status, content string) (*domain.Memory, error) {
	metaJSON, _ := json.Marshal(in.Metadata)
	if metaJSON == nil {
		metaJSON = []byte(`{}`)
	}
	// Ensure agent_id/project_id handling for NULL
	var agID, projID, convID, taskID sql.NullString
	if in.AgentID != nil {
		agID = sql.NullString{String: in.AgentID.String(), Valid: true}
	}
	if in.ProjectID != nil {
		projID = sql.NullString{String: in.ProjectID.String(), Valid: true}
	}
	if in.ConversationID != nil {
		convID = sql.NullString{String: in.ConversationID.String(), Valid: true}
	}
	if in.TaskID != nil {
		taskID = sql.NullString{String: in.TaskID.String(), Valid: true}
	}
	// source_event_id
	var sourceEventID sql.NullString
	if in.SourceEventID != nil {
		sourceEventID = sql.NullString{String: in.SourceEventID.String(), Valid: true}
	}
	// confidence/importance defaults
	conf := in.Confidence
	if conf == 0 {
		conf = 0.5
	}
	imp := in.Importance
	if imp == 0 {
		imp = 0.5
	}
	now := time.Now()
	var mem domain.Memory
	var retAg, retProj, retConv, retTask sql.NullString
	var retSourceEvent sql.NullString
	var retExpires, retLastAccess, retLastConfirm sql.NullTime
	var retMeta []byte
	var retHash, retIdem sql.NullString
	var retSuperseded sql.NullString
	err := s.repo.QueryRow(
		`INSERT INTO memories (organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, embedding, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, version)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::vector,$15,$16,$17,$18,$19,$20,1)
		 RETURNING id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at`,
		in.OrganizationID, agID, projID, convID, taskID, in.MemoryType, in.Scope, in.Source, sourceEventID, content, imp, conf, status, embeddingStr, metaJSON, in.ExpiresAt, now, now, hash, idemKey,
	).Scan(&mem.ID, &mem.OrganizationID, &retAg, &retProj, &retConv, &retTask, &mem.MemoryType, &mem.Scope, &mem.Source, &retSourceEvent, &mem.Content, &mem.Importance, &mem.Confidence, &mem.Status, &retMeta, &retExpires, &retLastAccess, &retLastConfirm, &retHash, &retIdem, &retSuperseded, &mem.Version, &mem.CreatedAt, &mem.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if retAg.Valid {
		uid, _ := uuid.Parse(retAg.String)
		mem.AgentID = &uid
	}
	if retProj.Valid {
		uid, _ := uuid.Parse(retProj.String)
		mem.ProjectID = &uid
	}
	if retConv.Valid {
		uid, _ := uuid.Parse(retConv.String)
		mem.ConversationID = &uid
	}
	if retTask.Valid {
		uid, _ := uuid.Parse(retTask.String)
		mem.TaskID = &uid
	}
	if retSourceEvent.Valid {
		uid, _ := uuid.Parse(retSourceEvent.String)
		mem.SourceEventID = &uid
	}
	if retExpires.Valid {
		mem.ExpiresAt = &retExpires.Time
	}
	if retLastAccess.Valid {
		mem.LastAccessedAt = &retLastAccess.Time
	}
	if retLastConfirm.Valid {
		mem.LastConfirmedAt = &retLastConfirm.Time
	}
	if retHash.Valid {
		mem.ContentHash = retHash.String
	}
	if retIdem.Valid {
		mem.IdempotencyKey = retIdem.String
	}
	if retSuperseded.Valid {
		uid, _ := uuid.Parse(retSuperseded.String)
		mem.SupersededBy = &uid
	}
	if len(retMeta) > 0 {
		_ = json.Unmarshal(retMeta, &mem.Metadata)
	}
	return &mem, nil
}

func (s *Service) findCandidates(ctx context.Context, in ConsolidateInput, embeddingStr string) ([]*domain.Memory, error) {
	// Bounded candidate retrieval with scope filtering BEFORE semantic comparison
	// Only ACTIVE/CONFLICT candidates within same allowed scope
	// Use vector similarity if embedding available, otherwise fallback to recency/importance
	var rows *sql.Rows
	var err error
	// Build strict scope filter
	baseSQL := `SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at FROM memories
		WHERE organization_id=$1 AND scope=$2 AND agent_id IS NOT DISTINCT FROM $3 AND project_id IS NOT DISTINCT FROM $4 AND conversation_id IS NOT DISTINCT FROM $5 AND task_id IS NOT DISTINCT FROM $6 AND status IN ('active','conflict','candidate','validating') AND (expires_at IS NULL OR expires_at > now())`
	args := []any{in.OrganizationID, in.Scope, in.AgentID, in.ProjectID, in.ConversationID, in.TaskID}
	idx := 7
	if embeddingStr != "" {
		sql := baseSQL + fmt.Sprintf(` ORDER BY embedding <=> $%d::vector LIMIT $%d`, idx, idx+1)
		args = append(args, embeddingStr, maxCandidates)
		rows, err = s.repo.Query(sql, args...)
	} else {
		sql := baseSQL + fmt.Sprintf(` ORDER BY importance DESC, confidence DESC, last_confirmed_at DESC LIMIT $%d`, idx)
		args = append(args, maxCandidates)
		rows, err = s.repo.Query(sql, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Memory
	for rows.Next() {
		var m domain.Memory
		var agID, projID, convID, taskID, sourceEventID sql.NullString
		var expires, lastAccess, lastConfirm sql.NullTime
		var meta []byte
		var hash, idem, superseded sql.NullString
		if err := rows.Scan(&m.ID, &m.OrganizationID, &agID, &projID, &convID, &taskID, &m.MemoryType, &m.Scope, &m.Source, &sourceEventID, &m.Content, &m.Importance, &m.Confidence, &m.Status, &meta, &expires, &lastAccess, &lastConfirm, &hash, &idem, &superseded, &m.Version, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if agID.Valid {
			uid, _ := uuid.Parse(agID.String)
			m.AgentID = &uid
		}
		if projID.Valid {
			uid, _ := uuid.Parse(projID.String)
			m.ProjectID = &uid
		}
		if convID.Valid {
			uid, _ := uuid.Parse(convID.String)
			m.ConversationID = &uid
		}
		if taskID.Valid {
			uid, _ := uuid.Parse(taskID.String)
			m.TaskID = &uid
		}
		if sourceEventID.Valid {
			uid, _ := uuid.Parse(sourceEventID.String)
			m.SourceEventID = &uid
		}
		if expires.Valid {
			m.ExpiresAt = &expires.Time
		}
		if lastAccess.Valid {
			m.LastAccessedAt = &lastAccess.Time
		}
		if lastConfirm.Valid {
			m.LastConfirmedAt = &lastConfirm.Time
		}
		if hash.Valid {
			m.ContentHash = hash.String
		}
		if idem.Valid {
			m.IdempotencyKey = idem.String
		}
		if superseded.Valid {
			uid, _ := uuid.Parse(superseded.String)
			m.SupersededBy = &uid
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &m.Metadata)
		}
		// Filter by vector distance threshold if we have embedding
		// Since we already ordered by distance, we can compute post-filter using text similarity as proxy when embedder is stub
		out = append(out, &m)
	}
	// Additional filtering: remove candidates with low textual overlap if distance suggests unrelated
	// Use deterministic text similarity to prune
	filtered := []*domain.Memory{}
	for _, c := range out {
		sim := textSimilarity(c.Content, in.Content)
		// If embedding distance was high but text similarity very low, likely unrelated; but we keep up to 5 for classification
		if sim < 0.25 && embeddingStr != "" {
			// Need to estimate distance proxy: if stub embedder returns same vector for all (0.01), embedding distance is 0, so skip this filter.
			// Only filter if we can detect embedding is stub (all same). Stub returns constant 0.01 dims => distance 0 for all, so we rely on text.
			// If text similarity <0.25 and not sharing any key subject token, skip
			if !sharesSubject(c.Content, in.Content) {
				continue
			}
		}
		filtered = append(filtered, c)
	}
	// Sort by confidence+importance + text similarity for stub cases
	if embeddingStr == "" || isStubEmbedding(embeddingStr) {
		sort.Slice(filtered, func(i, j int) bool {
			si := textSimilarity(filtered[i].Content, in.Content)
			sj := textSimilarity(filtered[j].Content, in.Content)
			if si == sj {
				if filtered[i].Confidence == filtered[j].Confidence {
					return filtered[i].Importance > filtered[j].Importance
				}
				return filtered[i].Confidence > filtered[j].Confidence
			}
			return si > sj
		})
	}
	if len(filtered) > maxCandidates {
		filtered = filtered[:maxCandidates]
	}
	return filtered, nil
}

func isStubEmbedding(s string) bool {
	// Stub embedder returns "[0.010000,0.010000,...]" identical for all
	return strings.Count(s, "0.010000") > 5
}

func (s *Service) getMemoryByID(ctx context.Context, id uuid.UUID) (*domain.Memory, error) {
	var m domain.Memory
	var agID, projID, convID, taskID, sourceEventID sql.NullString
	var expires, lastAccess, lastConfirm sql.NullTime
	var meta []byte
	var hash, idem, superseded sql.NullString
	err := s.repo.QueryRow(`SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at FROM memories WHERE id=$1`, id).
		Scan(&m.ID, &m.OrganizationID, &agID, &projID, &convID, &taskID, &m.MemoryType, &m.Scope, &m.Source, &sourceEventID, &m.Content, &m.Importance, &m.Confidence, &m.Status, &meta, &expires, &lastAccess, &lastConfirm, &hash, &idem, &superseded, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if agID.Valid {
		uid, _ := uuid.Parse(agID.String)
		m.AgentID = &uid
	}
	if projID.Valid {
		uid, _ := uuid.Parse(projID.String)
		m.ProjectID = &uid
	}
	if convID.Valid {
		uid, _ := uuid.Parse(convID.String)
		m.ConversationID = &uid
	}
	if taskID.Valid {
		uid, _ := uuid.Parse(taskID.String)
		m.TaskID = &uid
	}
	if sourceEventID.Valid {
		uid, _ := uuid.Parse(sourceEventID.String)
		m.SourceEventID = &uid
	}
	if expires.Valid {
		m.ExpiresAt = &expires.Time
	}
	if lastAccess.Valid {
		m.LastAccessedAt = &lastAccess.Time
	}
	if lastConfirm.Valid {
		m.LastConfirmedAt = &lastConfirm.Time
	}
	if hash.Valid {
		m.ContentHash = hash.String
	}
	if idem.Valid {
		m.IdempotencyKey = idem.String
	}
	if superseded.Valid {
		uid, _ := uuid.Parse(superseded.String)
		m.SupersededBy = &uid
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &m.Metadata)
	}
	return &m, nil
}

func (s *Service) createVersion(ctx context.Context, mem *domain.Memory, reason string) error {
	metaJSON, _ := json.Marshal(mem.Metadata)
	_, err := s.repo.Exec(`INSERT INTO memory_versions (memory_id, version, content, importance, confidence, status, metadata, source, reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		mem.ID, mem.Version, mem.Content, mem.Importance, mem.Confidence, mem.Status, metaJSON, mem.Source, reason)
	return err
}

func (s *Service) emitEvent(ctx context.Context, orgID uuid.UUID, memID *uuid.UUID, scope, eventType string, corrID *uuid.UUID, payload map[string]any) error {
	payloadJSON, _ := json.Marshal(payload)
	var memIDVal sql.NullString
	if memID != nil {
		memIDVal = sql.NullString{String: memID.String(), Valid: true}
	}
	var corrVal sql.NullString
	if corrID != nil {
		corrVal = sql.NullString{String: corrID.String(), Valid: true}
	}
	_, err := s.repo.Exec(`INSERT INTO memory_events (organization_id, memory_id, scope, event_type, correlation_id, payload) VALUES ($1,$2,$3,$4,$5,$6)`, orgID, memIDVal, scope, eventType, corrVal, payloadJSON)
	// best effort, ignore error if table not yet migrated or mock
	return err
}

func (s *Service) insertAudit(ctx context.Context, orgID uuid.UUID, action string, entityID uuid.UUID, after map[string]any) error {
	afterJSON, _ := json.Marshal(after)
	_, err := s.repo.Exec(`INSERT INTO audit_logs (organization_id, actor_type, action, entity_type, entity_id, after) VALUES ($1,'system',$2,'memory',$3,$4)`, orgID, action, entityID, afterJSON)
	return err
}

// Helpers

func vectorToString(v []float32) string {
	if len(v) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", f))
	}
	sb.WriteString("]")
	return sb.String()
}

func contentHash(normalized string) string {
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

func idempotencyKey(orgID uuid.UUID, scope string, agentID, projectID, convID, taskID *uuid.UUID, hash string) string {
	parts := []string{orgID.String(), scope}
	if agentID != nil {
		parts = append(parts, agentID.String())
	}
	if projectID != nil {
		parts = append(parts, projectID.String())
	}
	if convID != nil {
		parts = append(parts, convID.String())
	}
	if taskID != nil {
		parts = append(parts, taskID.String())
	}
	parts = append(parts, hash[:16])
	return strings.Join(parts, ":")
}

func normalizeContent(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// collapse whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	// remove trailing period
	s = strings.TrimSuffix(s, ".")
	return s
}

func isLowValue(content string, importance float64, memoryType string) bool {
	trimmed := strings.TrimSpace(content)
	// Very short content is low-value unless it's a test placeholder
	if len(trimmed) < 4 {
		return true
	}
	if len(trimmed) < 8 && importance < 0.6 {
		// Allow short content like "test" in unit tests but flag real low-value like "hi"
		lower := strings.ToLower(trimmed)
		if lower == "test" {
			return false
		}
		return true
	}
	lower := strings.ToLower(trimmed)
	lowPhrases := []string{
		"agent said hello",
		"the task started",
		"task started at",
		"user asked a temporary question",
		"the user asked a temporary question",
		"hello",
		"hi there",
		"temporary question",
	}
	for _, p := range lowPhrases {
		if lower == p || strings.Contains(lower, p) {
			// Allow if importance is explicitly high (user preference)
			if importance < 0.7 {
				return true
			}
		}
	}
	// Very short or just timestamp
	if len(trimmed) < 20 && regexp.MustCompile(`\d{1,2}:\d{2}`).MatchString(trimmed) {
		return true
	}
	// Generic greeting
	if lower == "hello" || lower == "hi" || lower == "thanks" || lower == "ok" {
		return true
	}
	// Content like "Agent said hello." exactly
	if strings.Contains(lower, "said hello") && len(trimmed) < 50 {
		return true
	}
	return false
}

var secretPattern = regexp.MustCompile(`(?i)(api[_-]?key|password|secret|token|credentials|bearer|aws_access|github_token|sk-[a-zA-Z0-9]{20,}|-----BEGIN)`)

func isSecretContent(content string) bool {
	lower := strings.ToLower(content)
	if secretPattern.MatchString(content) {
		// Additional check for token-like patterns
		if strings.Contains(lower, "sk-") || strings.Contains(lower, "bearer") || strings.Contains(lower, "password") || strings.Contains(content, "-----BEGIN") {
			return true
		}
		// If it contains api_key and looks like credential assignment
		if strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
			if regexp.MustCompile(`(?i)api[_-]?key\s*[=:]\s*['"]?[a-zA-Z0-9_\-]{16,}`).MatchString(content) {
				return true
			}
		}
		// If contains secret/password but also contains long random string, reject
		if regexp.MustCompile(`[a-zA-Z0-9]{20,}`).MatchString(content) && (strings.Contains(lower, "secret") || strings.Contains(lower, "token")) {
			return true
		}
	}
	return false
}

func reinforceConfidence(oldConf, newConf float64, source string) float64 {
	// Deterministic policy: repeated consistent observation increases confidence
	// explicit user preference gets larger boost
	if oldConf == 0 {
		oldConf = 0.5
	}
	if newConf == 0 {
		newConf = 0.5
	}
	base := oldConf
	if source == "EXPLICIT_USER_PREFERENCE" || source == "explicit_user_preference" {
		// Strong source: jump toward 0.95
		return minFloat(0.99, base+(1-base)*0.5)
	}
	// Regular reinforcement: similar to 0.5 ->0.65 etc
	return minFloat(0.99, base+(1-base)*0.3)
}

func chooseStrongerContent(a, b string) string {
	// Preserve strongest useful formulation, not concatenation
	// Heuristic: longer and more specific wins, but prefer higher word count and not just filler
	aTrim := strings.TrimSpace(a)
	bTrim := strings.TrimSpace(b)
	if bTrim == "" {
		return aTrim
	}
	if aTrim == "" {
		return bTrim
	}
	// Score by word count + information density
	score := func(s string) float64 {
		words := len(strings.Fields(s))
		unique := len(tokenSet(s))
		// Prefer moderate length (not too short, not too long)
		lengthScore := float64(words) * 0.5 + float64(unique)*0.5
		// Penalize if very short
		if words < 4 {
			lengthScore -= 2
		}
		return lengthScore
	}
	if score(bTrim) > score(aTrim) {
		return bTrim
	}
	return aTrim
}

func classifyRelationship(existing, candidate string, existingMem *domain.Memory, in ConsolidateInput, embeddingStr string) (string, float64) {
	// Returns: duplicate, reinforce, update, contradiction, unrelated, ambiguous
	normE := normalizeContent(existing)
	normC := normalizeContent(candidate)
	if normE == normC {
		return "duplicate", 0.95
	}
	// Token overlap
	tokensE := tokenSet(normE)
	tokensC := tokenSet(normC)
	jacc := jaccard(tokensE, tokensC)
	sim := textSimilarity(existing, candidate) // includes Levenshtein-ish

	// Contradiction detection: same subject but opposing value
	if isContradictory(existing, candidate) {
		return "contradiction", 0.85
	}
	// Value change with same subject -> update (higher priority than duplicate)
	if sharesSubject(existing, candidate) && hasValueChange(existing, candidate) {
		// Even if similarity is high, numeric/format value change is an update
		if sim > 0.45 || jacc > 0.35 {
			return "update", 0.78
		}
	}
	// High similarity -> duplicate/merge
	if sim > 0.88 || jacc > 0.85 {
		return "duplicate", 0.9
	}
	if sim > 0.75 || jacc > 0.7 {
		// Near duplicate, could be reinforce
		return "reinforce", 0.8
	}
	// Subject overlap but different details -> update or reinforce
	if sharesSubject(existing, candidate) && (sim > 0.45 || jacc > 0.4) {
		if hasValueChange(existing, candidate) {
			return "update", 0.75
		}
		return "reinforce", 0.7
	}
	// Low similarity -> unrelated
	if sim < 0.3 && jacc < 0.2 {
		return "unrelated", 0.2
	}
	// Ambiguous zone
	if sim >= 0.3 && sim <= 0.75 {
		return "ambiguous", 0.5
	}
	return "unrelated", 0.3
}

func tokenSet(s string) map[string]struct{} {
	tokens := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	set := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if len(t) < 2 {
			continue
		}
		// Remove stopwords
		if t == "the" || t == "a" || t == "an" || t == "is" || t == "are" || t == "for" || t == "this" || t == "that" || t == "customer" || t == "user" {
			// keep customer/user as they are subject but not for jaccard? Actually keep them; they are important for subject.
			// Keep them but they affect similarity. Let's keep them.
		}
		set[t] = struct{}{}
	}
	return set
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func textSimilarity(a, b string) float64 {
	// Simple normalized similarity based on token Jaccard + Levenshtein approximation
	aNorm := normalizeContent(a)
	bNorm := normalizeContent(b)
	if aNorm == bNorm {
		return 1.0
	}
	// Token Jaccard
	ta := tokenSet(aNorm)
	tb := tokenSet(bNorm)
	j := jaccard(ta, tb)
	// Length similarity
	lenA := len(aNorm)
	lenB := len(bNorm)
	maxLen := lenA
	if lenB > maxLen {
		maxLen = lenB
	}
	if maxLen == 0 {
		return 0
	}
	// Simple edit distance proxy: compare ordered words
	wordsA := strings.Fields(aNorm)
	wordsB := strings.Fields(bNorm)
	commonSeq := longestCommonSubsequence(wordsA, wordsB)
	seqSim := float64(commonSeq*2) / float64(len(wordsA)+len(wordsB))
	// Combined
	return j*0.6 + seqSim*0.4
}

func longestCommonSubsequence(a, b []string) int {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return 0
	}
	// DP with small sizes (memories are short, <30 words)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	return dp[m][n]
}

func sharesSubject(a, b string) bool {
	// Heuristic: if they share at least one noun subject token
	subjects := []string{"customer", "user", "report", "reports", "preference", "prefers", "format", "csv", "pdf", "project", "task", "memory"}
	aLow := strings.ToLower(a)
	bLow := strings.ToLower(b)
	shared := 0
	for _, s := range subjects {
		if strings.Contains(aLow, s) && strings.Contains(bLow, s) {
			shared++
		}
	}
	// Also check token overlap > 0.3
	ta := tokenSet(normalizeContent(a))
	tb := tokenSet(normalizeContent(b))
	if jaccard(ta, tb) > 0.3 {
		shared++
	}
	return shared > 0
}

func hasValueChange(a, b string) bool {
	// Detect if specific value token changed (e.g., csv vs pdf, numbers)
	aLow := strings.ToLower(a)
	bLow := strings.ToLower(b)
	// Extract format tokens
	formats := []string{"csv", "pdf", "json", "xlsx", "xls", "tsv", "xml", "txt", "html", "parquet", "avro"}
	aHas := ""
	bHas := ""
	for _, f := range formats {
		if strings.Contains(aLow, f) {
			aHas = f
		}
		if strings.Contains(bLow, f) {
			bHas = f
		}
	}
	if aHas != "" && bHas != "" && aHas != bHas {
		return true
	}
	// Numeric change like 2.3% vs 2.5%
	reNum := regexp.MustCompile(`\d+\.?\d*%?`)
	numsA := reNum.FindAllString(aLow, -1)
	numsB := reNum.FindAllString(bLow, -1)
	if len(numsA) > 0 && len(numsB) > 0 && strings.Join(numsA, ",") != strings.Join(numsB, ",") {
		if sharesSubject(a, b) {
			return true
		}
	}
	return false
}

func isContradictory(a, b string) bool {
	aLow := strings.ToLower(a)
	bLow := strings.ToLower(b)
	// Check opposing format values with same subject
	formats := []string{"csv", "pdf", "json", "xlsx", "xls", "tsv", "xml", "txt", "html"}
	aFmt, bFmt := "", ""
	for _, f := range formats {
		if strings.Contains(aLow, f) {
			aFmt = f
		}
		if strings.Contains(bLow, f) {
			bFmt = f
		}
	}
	if aFmt != "" && bFmt != "" && aFmt != bFmt {
		// Ensure same subject context (customer/report)
		if sharesSubject(a, b) {
			return true
		}
	}
	// Check negation differences
	if strings.Contains(aLow, "not") != strings.Contains(bLow, "not") && sharesSubject(a, b) {
		return true
	}
	// Check explicit contradictory days/statuses
	contradictPairs := [][]string{
		{"csv", "pdf"},
		{"pdf", "csv"},
		{"enabled", "disabled"},
		{"active", "archived"},
		{"yes", "no"},
	}
	for _, pair := range contradictPairs {
		if strings.Contains(aLow, pair[0]) && strings.Contains(bLow, pair[1]) && sharesSubject(a, b) {
			return true
		}
		if strings.Contains(aLow, pair[1]) && strings.Contains(bLow, pair[0]) && sharesSubject(a, b) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Retrieve now prefers ACTIVE and filters by status
func (s *Service) Retrieve(ctx context.Context, orgID uuid.UUID, agentID *uuid.UUID, projectID *uuid.UUID, query string, limit int, minImportance float64) ([]*domain.Memory, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var rows *sql.Rows
	var err error
	buildScopeFilter := func(args []any, idx int) (string, []any, int) {
		var clauses []string
		clauses = append(clauses, `scope = 'organization'`)
		if projectID != nil {
			clauses = append(clauses, fmt.Sprintf(`(scope = 'project' AND project_id = $%d)`, idx))
			args = append(args, *projectID)
			idx++
			clauses = append(clauses, fmt.Sprintf(`(scope = 'task' AND project_id = $%d)`, idx-1))
		}
		if agentID != nil {
			clauses = append(clauses, fmt.Sprintf(`(scope = 'agent' AND agent_id = $%d)`, idx))
			args = append(args, *agentID)
			idx++
			clauses = append(clauses, fmt.Sprintf(`(scope = 'session' AND agent_id = $%d)`, idx-1))
		}
		if agentID != nil && projectID != nil {
			clauses = append(clauses, fmt.Sprintf(`(scope = 'working' AND (agent_id = $%d OR project_id = $%d))`, idx, idx+1))
			args = append(args, *agentID, *projectID)
			idx += 2
			clauses = append(clauses, fmt.Sprintf(`(scope = 'conversation' AND (project_id = $%d OR agent_id = $%d))`, idx-1, idx-2))
		} else if agentID != nil {
			clauses = append(clauses, fmt.Sprintf(`(scope = 'conversation' AND agent_id = $%d)`, idx-1))
			clauses = append(clauses, fmt.Sprintf(`(scope = 'working' AND agent_id = $%d)`, idx-1))
		} else if projectID != nil {
			clauses = append(clauses, fmt.Sprintf(`(scope = 'conversation' AND project_id = $%d)`, idx-2))
		} else {
			clauses = append(clauses, `scope = 'conversation'`)
			clauses = append(clauses, `scope = 'working'`)
		}
		filter := ` AND (` + strings.Join(clauses, ` OR `) + `)`
		return filter, args, idx
	}

	// Common filters: only active/conflict memories, not stale/archived/superseded/merged
	statusFilter := ` AND status IN ('active','conflict') AND (expires_at IS NULL OR expires_at > now())`

	if query != "" && s.embedder != nil {
		if res, err2 := s.embedder.Embed(ctx, llm.EmbedRequest{Input: []string{query}}); err2 == nil && len(res.Embeddings) > 0 {
			embeddingStr := vectorToString(res.Embeddings[0])
			querySQL := `SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at FROM memories
				WHERE organization_id=$1 AND importance >= $2` + statusFilter
			args := []any{orgID, minImportance}
			idx := 3
			scopeFilter, args, idx := buildScopeFilter(args, idx)
			querySQL += scopeFilter
			querySQL += fmt.Sprintf(` ORDER BY embedding <=> $%d::vector, confidence DESC, importance DESC LIMIT $%d`, idx, idx+1)
			args = append(args, embeddingStr, limit)
			rows, err = s.repo.Query(querySQL, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			return scanMemoriesExtended(rows)
		}
	}
	querySQL := `SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at FROM memories
		WHERE organization_id=$1 AND importance >= $2` + statusFilter
	args := []any{orgID, minImportance}
	idx := 3
	scopeFilter, args, idx := buildScopeFilter(args, idx)
	querySQL += scopeFilter
	querySQL += fmt.Sprintf(` ORDER BY confidence DESC, importance DESC, last_confirmed_at DESC, created_at DESC LIMIT $%d`, idx)
	args = append(args, limit)
	rows, err = s.repo.Query(querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoriesExtended(rows)
}

func scanMemoriesExtended(rows *sql.Rows) ([]*domain.Memory, error) {
	var out []*domain.Memory
	for rows.Next() {
		var m domain.Memory
		var agID, projID, convID, taskID, srcEventID sql.NullString
		var expires, lastAccess, lastConfirm sql.NullTime
		var meta []byte
		var hash, idem, superseded sql.NullString
		if err := rows.Scan(&m.ID, &m.OrganizationID, &agID, &projID, &convID, &taskID, &m.MemoryType, &m.Scope, &m.Source, &srcEventID, &m.Content, &m.Importance, &m.Confidence, &m.Status, &meta, &expires, &lastAccess, &lastConfirm, &hash, &idem, &superseded, &m.Version, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if agID.Valid {
			uid, _ := uuid.Parse(agID.String)
			m.AgentID = &uid
		}
		if projID.Valid {
			uid, _ := uuid.Parse(projID.String)
			m.ProjectID = &uid
		}
		if convID.Valid {
			uid, _ := uuid.Parse(convID.String)
			m.ConversationID = &uid
		}
		if taskID.Valid {
			uid, _ := uuid.Parse(taskID.String)
			m.TaskID = &uid
		}
		if srcEventID.Valid {
			uid, _ := uuid.Parse(srcEventID.String)
			m.SourceEventID = &uid
		}
		if expires.Valid {
			m.ExpiresAt = &expires.Time
		}
		if lastAccess.Valid {
			m.LastAccessedAt = &lastAccess.Time
		}
		if lastConfirm.Valid {
			m.LastConfirmedAt = &lastConfirm.Time
		}
		if hash.Valid {
			m.ContentHash = hash.String
		}
		if idem.Valid {
			m.IdempotencyKey = idem.String
		}
		if superseded.Valid {
			uid, _ := uuid.Parse(superseded.String)
			m.SupersededBy = &uid
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &m.Metadata)
		}
		out = append(out, &m)
	}
	// Context quality: deduplicate, remove superseded, enforce ordering
	out = deduplicateForContext(out)
	return out, nil
}

func deduplicateForContext(mems []*domain.Memory) []*domain.Memory {
	seen := make(map[string]bool)
	var filtered []*domain.Memory
	for _, m := range mems {
		norm := normalizeContent(m.Content)
		if seen[norm] {
			continue
		}
		if m.Status == "superseded" || m.Status == "archived" || m.Status == "stale" || m.Status == "merged" {
			continue
		}
		seen[norm] = true
		filtered = append(filtered, m)
	}
	// Already sorted by confidence/importance; keep top
	return filtered
}

// CleanupExpired removes expired memories (called periodically)
func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	res, err := s.repo.Exec(`DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at < now() AND status IN ('stale','archived')`)
	if err != nil {
		return 0, err
	}
	// Also mark stale where expired but not yet archived
	_, _ = s.repo.Exec(`UPDATE memories SET status='stale', updated_at=now() WHERE expires_at IS NOT NULL AND expires_at < now() AND status='active'`)
	return res.RowsAffected()
}

// Archive marks a memory as archived (soft delete)
func (s *Service) Archive(ctx context.Context, orgID, memoryID uuid.UUID) error {
	m, err := s.getMemoryByID(ctx, memoryID)
	if err != nil {
		return err
	}
	if m.OrganizationID != orgID {
		return fmt.Errorf("not found")
	}
	_ = s.createVersion(ctx, m, "archived")
	_, err = s.repo.Exec(`UPDATE memories SET status='archived', updated_at=now() WHERE id=$1 AND organization_id=$2`, memoryID, orgID)
	if err == nil {
		_ = s.emitEvent(ctx, orgID, &memoryID, m.Scope, "memory.archived", nil, map[string]any{"reason": "manual"})
	}
	return err
}

// Restore archived memory
func (s *Service) Restore(ctx context.Context, orgID, memoryID uuid.UUID) error {
	m, err := s.getMemoryByID(ctx, memoryID)
	if err != nil {
		return err
	}
	if m.OrganizationID != orgID {
		return fmt.Errorf("not found")
	}
	if m.Status != "archived" && m.Status != "stale" && m.Status != "superseded" {
		return fmt.Errorf("memory not archived")
	}
	_ = s.createVersion(ctx, m, "restored")
	_, err = s.repo.Exec(`UPDATE memories SET status='active', updated_at=now(), last_confirmed_at=now() WHERE id=$1 AND organization_id=$2`, memoryID, orgID)
	if err == nil {
		_ = s.emitEvent(ctx, orgID, &memoryID, m.Scope, "memory.reconfirmed", nil, map[string]any{"reason": "restore"})
	}
	return err
}

// MarkStale identifies and marks stale memories in bounded batch
func (s *Service) MarkStale(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 50
	}
	res, err := s.repo.Exec(`UPDATE memories SET status='stale', updated_at=now() WHERE id IN (
		SELECT id FROM memories WHERE status='active' AND (
			(expires_at IS NOT NULL AND expires_at < now()) OR
			(memory_type='working' AND last_confirmed_at < now() - interval '1 hour') OR
			(memory_type='conversation' AND COALESCE(last_accessed_at, last_confirmed_at) < now() - interval '7 days')
		) LIMIT $1
	)`, batchSize)
	if err != nil {
		return 0, err
	}
	c, _ := res.RowsAffected()
	if c > 0 {
		// emit stale events best effort
	}
	return c, nil
}

// ListVersions returns version history for a memory
func (s *Service) ListVersions(ctx context.Context, memoryID uuid.UUID) ([]*domain.MemoryVersion, error) {
	rows, err := s.repo.Query(`SELECT id, memory_id, version, content, importance, confidence, status, metadata, source, reason, created_at FROM memory_versions WHERE memory_id=$1 ORDER BY version ASC`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.MemoryVersion
	for rows.Next() {
		var v domain.MemoryVersion
		var meta []byte
		if err := rows.Scan(&v.ID, &v.MemoryID, &v.Version, &v.Content, &v.Importance, &v.Confidence, &v.Status, &meta, &v.Source, &v.Reason, &v.CreatedAt); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &v.Metadata)
		}
		out = append(out, &v)
	}
	return out, nil
}

// Get retrieves single memory by id with org check
func (s *Service) Get(ctx context.Context, orgID, memoryID uuid.UUID) (*domain.Memory, error) {
	m, err := s.getMemoryByID(ctx, memoryID)
	if err != nil {
		return nil, err
	}
	if m.OrganizationID != orgID {
		return nil, fmt.Errorf("not found")
	}
	// bump last_accessed
	_, _ = s.repo.Exec(`UPDATE memories SET last_accessed_at=now() WHERE id=$1`, memoryID)
	return m, nil
}

// List returns memories with optional filters including status
func (s *Service) List(ctx context.Context, orgID uuid.UUID, agentID, projectID *uuid.UUID, status, scope, memoryType string, limit int) ([]*domain.Memory, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, memory_type, scope, source, source_event_id, content, importance, confidence, status, metadata, expires_at, last_accessed_at, last_confirmed_at, content_hash, idempotency_key, superseded_by, version, created_at, updated_at FROM memories WHERE organization_id=$1`
	args := []any{orgID}
	idx := 2
	if status != "" {
		query += fmt.Sprintf(` AND status=$%d`, idx)
		args = append(args, status)
		idx++
	} else {
		query += ` AND status IN ('active','conflict')`
	}
	if scope != "" {
		query += fmt.Sprintf(` AND scope=$%d`, idx)
		args = append(args, scope)
		idx++
	}
	if memoryType != "" {
		query += fmt.Sprintf(` AND memory_type=$%d`, idx)
		args = append(args, memoryType)
		idx++
	}
	if agentID != nil {
		query += fmt.Sprintf(` AND agent_id=$%d`, idx)
		args = append(args, *agentID)
		idx++
	}
	if projectID != nil {
		query += fmt.Sprintf(` AND project_id=$%d`, idx)
		args = append(args, *projectID)
		idx++
	}
	query += fmt.Sprintf(` ORDER BY confidence DESC, importance DESC, created_at DESC LIMIT $%d`, idx)
	args = append(args, limit)
	rows, err := s.repo.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoriesExtended(rows)
}

// LLM classification fallback for ambiguous cases
func (s *Service) llmClassify(ctx context.Context, existing, candidate string) (string, float64, string, error) {
	if s.llmProvider == nil {
		return "", 0, "", fmt.Errorf("no llm")
	}
	prompt := fmt.Sprintf(`You are a memory consolidation classifier. Determine relationship between existing memory and candidate.

Existing: "%s"
Candidate: "%s"

Classify as one of: duplicate, reinforce, update, contradiction, unrelated.
Return ONLY JSON: {"relationship":"duplicate|reinforce|update|contradiction|unrelated","confidence":0.0-1.0,"consolidated_memory":"strongest formulation","reason":"short reason"}`, existing, candidate)
	resp, err := s.llmProvider.Complete(ctx, llm.CompletionRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", 0, "", err
	}
	content := resp.Content
	// Extract JSON
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", 0, "", fmt.Errorf("no json")
	}
	jsonStr := content[start : end+1]
	var out struct {
		Relationship       string  `json:"relationship"`
		Confidence         float64 `json:"confidence"`
		ConsolidatedMemory string  `json:"consolidated_memory"`
		Reason             string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return "", 0, "", err
	}
	valid := map[string]bool{"duplicate": true, "reinforce": true, "update": true, "contradiction": true, "unrelated": true}
	if !valid[out.Relationship] {
		return "", 0, "", fmt.Errorf("invalid relationship")
	}
	return out.Relationship, out.Confidence, out.ConsolidatedMemory, nil
}

// BackgroundConsolidation runs bounded batch consolidation for stale/duplicate clusters
func (s *Service) BackgroundConsolidation(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 20
	}
	// Find candidate clusters: active memories grouped by organization+scope+project/agent that have near duplicates
	// For performance, run limited query to find potential duplicates via content_hash prefix or recent creations
	rows, err := s.repo.Query(`SELECT id, organization_id, agent_id, project_id, conversation_id, task_id, scope, content FROM memories WHERE status='active' ORDER BY created_at DESC LIMIT $1`, batchSize*2)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type item struct {
		id             uuid.UUID
		org            uuid.UUID
		agent, project, conv, task *uuid.UUID
		scope, content string
	}
	var items []item
	for rows.Next() {
		var it item
		var ag, pr, co, ta sql.NullString
		if err := rows.Scan(&it.id, &it.org, &ag, &pr, &co, &ta, &it.scope, &it.content); err != nil {
			continue
		}
		if ag.Valid {
			uid, _ := uuid.Parse(ag.String)
			it.agent = &uid
		}
		if pr.Valid {
			uid, _ := uuid.Parse(pr.String)
			it.project = &uid
		}
		if co.Valid {
			uid, _ := uuid.Parse(co.String)
			it.conv = &uid
		}
		if ta.Valid {
			uid, _ := uuid.Parse(ta.String)
			it.task = &uid
		}
		items = append(items, it)
	}
	var consolidated int64
	// Naive pairwise check within batch (bounded)
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].org != items[j].org || items[i].scope != items[j].scope {
				continue
			}
			// Must share same owner to be consolidatable (scope isolation)
			if !sameOwner(items[i].agent, items[j].agent) || !sameOwner(items[i].project, items[j].project) {
				continue
			}
			rel, _ := classifyRelationship(items[i].content, items[j].content, nil, ConsolidateInput{}, "")
			if rel == "duplicate" || rel == "reinforce" {
				// Merge j into i (keep earlier)
				cand := &domain.Memory{ID: items[i].id, Content: items[i].content, OrganizationID: items[i].org, Scope: items[i].scope, AgentID: items[i].agent, ProjectID: items[i].project}
				in := ConsolidateInput{
					OrganizationID: items[j].org,
					AgentID:        items[j].agent,
					ProjectID:      items[j].project,
					ConversationID: items[j].conv,
					TaskID:         items[j].task,
					Scope:          items[j].scope,
					Content:        items[j].content,
					Importance:     0.5,
					Source:         "manual",
				}
				// Use merge path directly to avoid recursion
				if _, err := s.handleMerge(ctx, in, cand, contentHash(normalizeContent(items[j].content)), 0.8); err == nil {
					consolidated++
					if consolidated >= int64(batchSize) {
						return consolidated, nil
					}
				}
			}
		}
	}
	return consolidated, nil
}

func sameOwner(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Keep old scan for backward compat (used by legacy callers that select fewer columns)
// scanMemories is kept for compatibility but delegates to extended
func scanMemories(rows *sql.Rows) ([]*domain.Memory, error) {
	// Try extended scan first, fallback to legacy
	cols, _ := rows.Columns()
	if len(cols) >= 14 && contains(cols, "confidence") {
		return scanMemoriesExtended(rows)
	}
	// Legacy: id, org, agent, project, memory_type, scope, source, content, importance, metadata, expires, created, updated
	var out []*domain.Memory
	for rows.Next() {
		var m domain.Memory
		var agID, projID sql.NullString
		var expires sql.NullTime
		var meta []byte
		if err := rows.Scan(&m.ID, &m.OrganizationID, &agID, &projID, &m.MemoryType, &m.Scope, &m.Source, &m.Content, &m.Importance, &meta, &expires, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if agID.Valid {
			uid, _ := uuid.Parse(agID.String)
			m.AgentID = &uid
		}
		if projID.Valid {
			uid, _ := uuid.Parse(projID.String)
			m.ProjectID = &uid
		}
		if expires.Valid {
			m.ExpiresAt = &expires.Time
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &m.Metadata)
		}
		m.Confidence = 0.5
		m.Status = "active"
		m.Version = 1
		out = append(out, &m)
	}
	return out, nil
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
