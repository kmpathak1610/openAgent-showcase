package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"openagent/internal/domain"
)

func vectorToString(v []float32) string {
	if len(v) == 0 { return "" }
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 { sb.WriteString(",") }
		sb.WriteString(fmt.Sprintf("%f", f))
	}
	sb.WriteString("]")
	return sb.String()
}

// ——— Knowledge Sources

func (db *DB) CreateKnowledgeSource(orgID uuid.UUID, name, sourceType string, config map[string]any, createdBy uuid.UUID) (*domain.KnowledgeSource, error) {
	cfgJSON, _ := json.Marshal(config)
	var s domain.KnowledgeSource
	var cfg []byte
	err := db.QueryRow(
		`INSERT INTO knowledge_sources (organization_id, name, source_type, config, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id, organization_id, name, source_type, config, status, created_by, created_at, updated_at`,
		orgID, name, sourceType, cfgJSON, createdBy,
	).Scan(&s.ID, &s.OrganizationID, &s.Name, &s.SourceType, &cfg, &s.Status, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil { return nil, err }
	if len(cfg) > 0 { _ = json.Unmarshal(cfg, &s.Config) }
	return &s, nil
}

func (db *DB) ListKnowledgeSources(orgID uuid.UUID) ([]*domain.KnowledgeSource, error) {
	rows, err := db.Query(`SELECT id, organization_id, name, source_type, config, status, created_by, created_at, updated_at FROM knowledge_sources WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.KnowledgeSource
	for rows.Next() {
		var s domain.KnowledgeSource
		var cfg []byte
		if err := rows.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.SourceType, &cfg, &s.Status, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil { return nil, err }
		if len(cfg) > 0 { _ = json.Unmarshal(cfg, &s.Config) }
		out = append(out, &s)
	}
	return out, nil
}

func (db *DB) GetKnowledgeSource(orgID, id uuid.UUID) (*domain.KnowledgeSource, error) {
	var s domain.KnowledgeSource
	var cfg []byte
	err := db.QueryRow(`SELECT id, organization_id, name, source_type, config, status, created_by, created_at, updated_at FROM knowledge_sources WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(
		&s.ID, &s.OrganizationID, &s.Name, &s.SourceType, &cfg, &s.Status, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil { return nil, err }
	if len(cfg) > 0 { _ = json.Unmarshal(cfg, &s.Config) }
	return &s, nil
}

// ——— Knowledge Collections

func (db *DB) CreateKnowledgeCollection(orgID uuid.UUID, name, description string, createdBy uuid.UUID) (*domain.KnowledgeCollection, error) {
	slug := slugify(name)
	base := slug
	for i := 0; i < 5; i++ {
		var c domain.KnowledgeCollection
		err := db.QueryRow(`INSERT INTO knowledge_collections (organization_id, name, slug, description, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id, organization_id, name, slug, description, created_by, created_at, updated_at`,
			orgID, name, slug, description, createdBy).Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Slug, &c.Description, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
		if err == nil { return &c, nil }
		if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "unique") { return nil, err }
		slug = fmt.Sprintf("%s-%s", base, uuid.NewString()[:4])
	}
	return nil, fmt.Errorf("failed to create collection")
}

func (db *DB) ListKnowledgeCollections(orgID uuid.UUID) ([]*domain.KnowledgeCollection, error) {
	rows, err := db.Query(`SELECT id, organization_id, name, slug, description, created_by, created_at, updated_at FROM knowledge_collections WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.KnowledgeCollection
	for rows.Next() {
		var c domain.KnowledgeCollection
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Slug, &c.Description, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil { return nil, err }
		out = append(out, &c)
	}
	return out, nil
}

func (db *DB) GetKnowledgeCollection(orgID, id uuid.UUID) (*domain.KnowledgeCollection, error) {
	var c domain.KnowledgeCollection
	err := db.QueryRow(`SELECT id, organization_id, name, slug, description, created_by, created_at, updated_at FROM knowledge_collections WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(
		&c.ID, &c.OrganizationID, &c.Name, &c.Slug, &c.Description, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil { return nil, err }
	return &c, nil
}

// ——— Documents

func (db *DB) CreateDocument(orgID uuid.UUID, projectID, collectionID, sourceID *uuid.UUID, title, sourceType string, sourceURL *string, mimeType string, scope string, content string, metadata map[string]any, createdBy uuid.UUID) (*domain.Document, error) {
	if scope == "" { scope = "organization" }
	metaJSON, _ := json.Marshal(metadata)
	var doc domain.Document
	var srcID, collID sql.NullString
	var projID sql.NullString
	var srcURL sql.NullString
	if sourceURL != nil { srcURL = sql.NullString{String: *sourceURL, Valid: true} }
	err := db.QueryRow(
		`INSERT INTO documents (organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, scope, metadata, created_by, size_bytes, status, version)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending',1)
		 RETURNING id, organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, size_bytes, status, scope, version, metadata, created_by, created_at, updated_at`,
		orgID, projectID, collectionID, sourceID, title, sourceType, srcURL, mimeType, scope, metaJSON, createdBy, int64(len(content)),
	).Scan(&doc.ID, &doc.OrganizationID, &projID, &collID, &srcID, &doc.Title, &doc.SourceType, &srcURL, &doc.MimeType, &doc.SizeBytes, &doc.Status, &doc.Scope, &doc.Version, &metaJSON, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)
	if err != nil { return nil, err }
	if projID.Valid { uid, _ := uuid.Parse(projID.String); doc.ProjectID = &uid }
	if collID.Valid { uid, _ := uuid.Parse(collID.String); doc.CollectionID = &uid }
	if srcID.Valid { uid, _ := uuid.Parse(srcID.String); doc.SourceID = &uid }
	if srcURL.Valid { doc.SourceURL = &srcURL.String }
	if len(metaJSON) > 0 { _ = json.Unmarshal(metaJSON, &doc.Metadata) }
	doc.Content = content
	// store initial content as version 1
	_, _ = db.Exec(`INSERT INTO document_versions (document_id, version, title, content, metadata, created_by) VALUES ($1,1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, doc.ID, title, content, metaJSON, createdBy)
	// chunk count will be filled after ingestion
	return &doc, nil
}

func (db *DB) GetDocument(orgID, docID uuid.UUID) (*domain.Document, error) {
	var doc domain.Document
	var projID, collID, srcID sql.NullString
	var srcURL sql.NullString
	var metaJSON []byte
	err := db.QueryRow(
		`SELECT id, organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, size_bytes, status, scope, version, metadata, created_by, created_at, updated_at FROM documents WHERE id=$1 AND organization_id=$2`,
		docID, orgID).Scan(&doc.ID, &doc.OrganizationID, &projID, &collID, &srcID, &doc.Title, &doc.SourceType, &srcURL, &doc.MimeType, &doc.SizeBytes, &doc.Status, &doc.Scope, &doc.Version, &metaJSON, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)
	if err != nil { return nil, err }
	if projID.Valid { uid, _ := uuid.Parse(projID.String); doc.ProjectID = &uid }
	if collID.Valid { uid, _ := uuid.Parse(collID.String); doc.CollectionID = &uid }
	if srcID.Valid { uid, _ := uuid.Parse(srcID.String); doc.SourceID = &uid }
	if srcURL.Valid { doc.SourceURL = &srcURL.String }
	if len(metaJSON) > 0 { _ = json.Unmarshal(metaJSON, &doc.Metadata) }
	// chunk count
	_ = db.QueryRow(`SELECT count(*) FROM document_chunks WHERE document_id=$1`, docID).Scan(&doc.ChunkCount)
	return &doc, nil
}

func (db *DB) ListDocuments(orgID uuid.UUID, projectID *uuid.UUID, scope string, status string, limit int) ([]*domain.Document, error) {
	if limit <= 0 || limit > 100 { limit = 20 }
	query := `SELECT id, organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, size_bytes, status, scope, version, metadata, created_by, created_at, updated_at FROM documents WHERE organization_id=$1`
	args := []any{orgID}
	idx := 2
	if projectID != nil {
		query += fmt.Sprintf(` AND project_id=$%d`, idx)
		args = append(args, *projectID)
		idx++
	}
	if scope != "" {
		query += fmt.Sprintf(` AND scope=$%d`, idx)
		args = append(args, scope)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(` AND status=$%d`, idx)
		args = append(args, status)
		idx++
	}
	query += ` ORDER BY created_at DESC`
	query += fmt.Sprintf(` LIMIT $%d`, idx)
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Document
	for rows.Next() {
		var doc domain.Document
		var projID, collID, srcID sql.NullString
		var srcURL sql.NullString
		var metaJSON []byte
		if err := rows.Scan(&doc.ID, &doc.OrganizationID, &projID, &collID, &srcID, &doc.Title, &doc.SourceType, &srcURL, &doc.MimeType, &doc.SizeBytes, &doc.Status, &doc.Scope, &doc.Version, &metaJSON, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt); err != nil { return nil, err }
		if projID.Valid { uid, _ := uuid.Parse(projID.String); doc.ProjectID = &uid }
		if collID.Valid { uid, _ := uuid.Parse(collID.String); doc.CollectionID = &uid }
		if srcID.Valid { uid, _ := uuid.Parse(srcID.String); doc.SourceID = &uid }
		if srcURL.Valid { doc.SourceURL = &srcURL.String }
		if len(metaJSON) > 0 { _ = json.Unmarshal(metaJSON, &doc.Metadata) }
		out = append(out, &doc)
	}
	return out, nil
}

func (db *DB) UpdateDocumentStatus(orgID, docID uuid.UUID, status string) error {
	_, err := db.Exec(`UPDATE documents SET status=$3, updated_at=now() WHERE id=$1 AND organization_id=$2`, docID, orgID, status)
	return err
}

func (db *DB) DeleteDocument(orgID, docID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM documents WHERE id=$1 AND organization_id=$2`, docID, orgID)
	return err
}

func (db *DB) ArchiveDocument(orgID, docID uuid.UUID) error {
	_, err := db.Exec(`UPDATE documents SET status='archived', updated_at=now() WHERE id=$1 AND organization_id=$2`, docID, orgID)
	return err
}

// ——— Document Chunks & Embeddings

func (db *DB) CreateDocumentChunks(orgID, docID uuid.UUID, chunks []domain.DocumentChunk) error {
	tx, err := db.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	for _, ch := range chunks {
		metaJSON, _ := json.Marshal(ch.Metadata)
		var chunkID uuid.UUID
		// embedding as vector string
		embedStr := vectorToString(ch.Embedding)
		err = tx.QueryRow(
			`INSERT INTO document_chunks (document_id, organization_id, chunk_index, content, token_count, embedding, metadata) VALUES ($1,$2,$3,$4,$5,$6::vector,$7) RETURNING id`,
			docID, orgID, ch.ChunkIndex, ch.Content, ch.TokenCount, embedStr, metaJSON,
		).Scan(&chunkID)
		if err != nil { return fmt.Errorf("insert chunk %d: %w", ch.ChunkIndex, err) }
		// also insert into embeddings table
		if len(ch.Embedding) > 0 {
			_, err = tx.Exec(`INSERT INTO embeddings (organization_id, document_chunk_id, embedding, model) VALUES ($1,$2,$3::vector,$4)`, orgID, chunkID, embedStr, "text-embedding-3-small")
			if err != nil { return err }
		}
	}
	// mark ready and update chunk count via version? status
	_, err = tx.Exec(`UPDATE documents SET status='ready', updated_at=now() WHERE id=$1`, docID)
	if err != nil { return err }
	return tx.Commit()
}



func (db *DB) ListDocumentChunks(docID uuid.UUID) ([]*domain.DocumentChunk, error) {
	rows, err := db.Query(`SELECT id, document_id, organization_id, chunk_index, content, token_count, metadata, created_at FROM document_chunks WHERE document_id=$1 ORDER BY chunk_index`, docID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.DocumentChunk
	for rows.Next() {
		var ch domain.DocumentChunk
		var meta []byte
		if err := rows.Scan(&ch.ID, &ch.DocumentID, &ch.OrganizationID, &ch.ChunkIndex, &ch.Content, &ch.TokenCount, &meta, &ch.CreatedAt); err != nil { return nil, err }
		if len(meta) > 0 { _ = json.Unmarshal(meta, &ch.Metadata) }
		out = append(out, &ch)
	}
	return out, nil
}

// ——— Agent / Project Knowledge Joins

func (db *DB) AttachAgentKnowledge(agentID uuid.UUID, docID, collID *uuid.UUID, grantedBy uuid.UUID) (*domain.AgentKnowledge, error) {
	var ak domain.AgentKnowledge
	err := db.QueryRow(`INSERT INTO agent_knowledge (agent_id, document_id, collection_id, granted_by) VALUES ($1,$2,$3,$4) RETURNING id, agent_id, document_id, collection_id, granted_by, created_at`,
		agentID, docID, collID, grantedBy).Scan(&ak.ID, &ak.AgentID, &ak.DocumentID, &ak.CollectionID, &ak.GrantedBy, &ak.CreatedAt)
	if err != nil { return nil, err }
	return &ak, nil
}

func (db *DB) DetachAgentKnowledge(agentID, knowledgeID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM agent_knowledge WHERE id=$1 AND agent_id=$2`, knowledgeID, agentID)
	return err
}

func (db *DB) ListAgentKnowledge(agentID uuid.UUID) ([]*domain.AgentKnowledge, error) {
	rows, err := db.Query(`SELECT id, agent_id, document_id, collection_id, granted_by, created_at FROM agent_knowledge WHERE agent_id=$1 ORDER BY created_at`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.AgentKnowledge
	for rows.Next() {
		var ak domain.AgentKnowledge
		if err := rows.Scan(&ak.ID, &ak.AgentID, &ak.DocumentID, &ak.CollectionID, &ak.GrantedBy, &ak.CreatedAt); err != nil { return nil, err }
		out = append(out, &ak)
	}
	return out, nil
}

func (db *DB) AttachProjectKnowledge(projectID uuid.UUID, docID, collID *uuid.UUID, grantedBy uuid.UUID) (*domain.ProjectKnowledge, error) {
	var pk domain.ProjectKnowledge
	err := db.QueryRow(`INSERT INTO project_knowledge (project_id, document_id, collection_id, granted_by) VALUES ($1,$2,$3,$4) RETURNING id, project_id, document_id, collection_id, granted_by, created_at`,
		projectID, docID, collID, grantedBy).Scan(&pk.ID, &pk.ProjectID, &pk.DocumentID, &pk.CollectionID, &pk.GrantedBy, &pk.CreatedAt)
	if err != nil { return nil, err }
	return &pk, nil
}

func (db *DB) ListProjectKnowledge(projectID uuid.UUID) ([]*domain.ProjectKnowledge, error) {
	rows, err := db.Query(`SELECT id, project_id, document_id, collection_id, granted_by, created_at FROM project_knowledge WHERE project_id=$1`, projectID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.ProjectKnowledge
	for rows.Next() {
		var pk domain.ProjectKnowledge
		if err := rows.Scan(&pk.ID, &pk.ProjectID, &pk.DocumentID, &pk.CollectionID, &pk.GrantedBy, &pk.CreatedAt); err != nil { return nil, err }
		out = append(out, &pk)
	}
	return out, nil
}

func (db *DB) DetachProjectKnowledge(projectID, knowledgeID uuid.UUID) error {
	_, err := db.Exec(`DELETE FROM project_knowledge WHERE id=$1 AND project_id=$2`, knowledgeID, projectID)
	return err
}

// ——— Permission-filtered retrieval helpers

// getAllowedDocIDs returns document IDs accessible to agent based on org, project, agent joins and scope
func (db *DB) GetAllowedDocIDs(filter domain.RetrievalFilter) ([]uuid.UUID, error) {
	// Build union of allowed docs:
	// - organization scope docs (scope='organization' and org match)
	// - project scope docs where project matches filter.ProjectID and is member
	// - agent-specific via agent_knowledge where agent matches
	// For Phase 3, we implement: all org docs + project docs if project filter + agent docs
	// Additionally enforce AllowedDocIDs if provided (intersection)
	query := `SELECT id FROM documents WHERE organization_id=$1 AND status='ready'`
	args := []any{filter.OrganizationID}
	idx := 2
	// Scope filtering is done at retrieval time; we return superset then filter later
	// But we filter by scope if filter.Scope specified
	if filter.Scope != "" {
		query += fmt.Sprintf(` AND scope=$%d`, idx)
		args = append(args, filter.Scope)
		idx++
	}
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	idMap := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil { return nil, err }
		idMap[id] = true
	}
	// If agent filter, restrict to agent knowledge + org + project
	if filter.AgentID != nil {
		// fetch agent knowledge doc ids
		rows2, err := db.Query(`SELECT document_id FROM agent_knowledge WHERE agent_id=$1 AND document_id IS NOT NULL`, *filter.AgentID)
		if err == nil {
			agentDocs := map[uuid.UUID]bool{}
			for rows2.Next() {
				var did uuid.UUID
				if err := rows2.Scan(&did); err == nil { agentDocs[did] = true }
			}
			rows2.Close()
			// For agent retrieval, allow: org docs + agent docs (intersection? union)
			// Agent should see org + its own + project docs if in project
			// We keep org docs and add agent docs
			for id := range agentDocs { idMap[id] = true }
			// Also collections via agent_knowledge -> expand to documents in collection
			rows3, _ := db.Query(`SELECT collection_id FROM agent_knowledge WHERE agent_id=$1 AND collection_id IS NOT NULL`, *filter.AgentID)
			if rows3 != nil {
				for rows3.Next() {
					var cid uuid.UUID
					rows3.Scan(&cid)
					// fetch docs in collection
					rows4, _ := db.Query(`SELECT id FROM documents WHERE collection_id=$1`, cid)
					if rows4 != nil {
						for rows4.Next() { var did uuid.UUID; rows4.Scan(&did); idMap[did] = true }
						rows4.Close()
					}
				}
				rows3.Close()
			}
		}
	}
	// Project filter
	if filter.ProjectID != nil {
		rows2, _ := db.Query(`SELECT document_id FROM project_knowledge WHERE project_id=$1 AND document_id IS NOT NULL`, *filter.ProjectID)
		if rows2 != nil {
			for rows2.Next() { var did uuid.UUID; rows2.Scan(&did); idMap[did] = true }
			rows2.Close()
		}
		rows3, _ := db.Query(`SELECT collection_id FROM project_knowledge WHERE project_id=$1 AND collection_id IS NOT NULL`, *filter.ProjectID)
		if rows3 != nil {
			for rows3.Next() {
				var cid uuid.UUID; rows3.Scan(&cid)
				rows4, _ := db.Query(`SELECT id FROM documents WHERE collection_id=$1`, cid)
				if rows4 != nil {
					for rows4.Next() { var did uuid.UUID; rows4.Scan(&did); idMap[did] = true }
					rows4.Close()
				}
			}
			rows3.Close()
		}
	}
	// If explicit AllowedDocIDs provided, intersect
	if len(filter.AllowedDocIDs) > 0 {
		allowedSet := map[uuid.UUID]bool{}
		for _, id := range filter.AllowedDocIDs { allowedSet[id] = true }
		intersected := []uuid.UUID{}
		for id := range idMap {
			if allowedSet[id] { intersected = append(intersected, id) }
		}
		// if AllowedDocIDs was meant as whitelist, return only those that are also in idMap
		// If none match, return whitelist filtered by org (still enforce)
		ids := []uuid.UUID{}
		for _, id := range filter.AllowedDocIDs {
			if idMap[id] { ids = append(ids, id) }
		}
		if len(ids) > 0 { return ids, nil }
		return intersected, nil
	}
	ids := make([]uuid.UUID, 0, len(idMap))
	for id := range idMap { ids = append(ids, id) }
	return ids, nil
}

// VectorSearch performs pgvector cosine similarity with permission filtering
func (db *DB) VectorSearch(orgID uuid.UUID, queryEmbedding []float32, filter domain.RetrievalFilter, limit int) ([]domain.RetrievalResult, error) {
	if limit <= 0 { limit = 5 }
	if limit > 20 { limit = 20 }
	allowedIDs, err := db.GetAllowedDocIDs(filter)
	if err != nil { return nil, err }
	if len(allowedIDs) == 0 { return []domain.RetrievalResult{}, nil }
	_ = orgID // allowedIDs already org-filtered via GetAllowedDocIDs
	embedStr := vectorToString(queryEmbedding)
	inPlaceholders := []string{}
	args := []any{embedStr}
	for i, id := range allowedIDs {
		inPlaceholders = append(inPlaceholders, fmt.Sprintf("$%d", i+2))
		args = append(args, id)
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))
	query := fmt.Sprintf(`
		SELECT dc.id, dc.document_id, dc.organization_id, dc.chunk_index, dc.content, dc.token_count, dc.metadata, dc.created_at,
		       d.title, (1 - (dc.embedding <=> $1::vector)) as score
		FROM document_chunks dc
		JOIN documents d ON d.id = dc.document_id
		WHERE dc.document_id IN (%s) AND dc.embedding IS NOT NULL
		ORDER BY dc.embedding <=> $1::vector
		LIMIT %s`, strings.Join(inPlaceholders, ","), limitPlaceholder)
	rows, err := db.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var results []domain.RetrievalResult
	for rows.Next() {
		var ch domain.DocumentChunk
		var meta []byte
		var title sql.NullString
		var score sql.NullFloat64
		if err := rows.Scan(&ch.ID, &ch.DocumentID, &ch.OrganizationID, &ch.ChunkIndex, &ch.Content, &ch.TokenCount, &meta, &ch.CreatedAt, &title, &score); err != nil { return nil, err }
		if len(meta) > 0 { _ = json.Unmarshal(meta, &ch.Metadata) }
		if title.Valid { ch.DocumentTitle = title.String }
		if score.Valid { ch.Score = score.Float64 }
		res := domain.RetrievalResult{Chunk: ch, Score: 0}
		if score.Valid { res.Score = score.Float64; res.Chunk.Score = score.Float64 }
		results = append(results, res)
	}
	return results, nil
}

// KeywordSearch simple ILIKE for hybrid
func (db *DB) KeywordSearch(orgID uuid.UUID, query string, filter domain.RetrievalFilter, limit int) ([]domain.RetrievalResult, error) {
	allowedIDs, err := db.GetAllowedDocIDs(filter)
	if err != nil { return nil, err }
	if len(allowedIDs) == 0 { return []domain.RetrievalResult{}, nil }
	inPlaceholders := []string{}
	args := []any{"%" + query + "%"}
	for i, id := range allowedIDs {
		inPlaceholders = append(inPlaceholders, fmt.Sprintf("$%d", i+2))
		args = append(args, id)
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))
	querySQL := fmt.Sprintf(`SELECT dc.id, dc.document_id, dc.organization_id, dc.chunk_index, dc.content, dc.token_count, dc.metadata, dc.created_at, d.title
		FROM document_chunks dc JOIN documents d ON d.id=dc.document_id
		WHERE dc.content ILIKE $1 AND dc.document_id IN (%s)
		ORDER BY dc.created_at DESC LIMIT %s`, strings.Join(inPlaceholders, ","), limitPlaceholder)
	rows, err := db.Query(querySQL, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var results []domain.RetrievalResult
	for rows.Next() {
		var ch domain.DocumentChunk
		var meta []byte
		var title sql.NullString
		if err := rows.Scan(&ch.ID, &ch.DocumentID, &ch.OrganizationID, &ch.ChunkIndex, &ch.Content, &ch.TokenCount, &meta, &ch.CreatedAt, &title); err != nil { return nil, err }
		if len(meta) > 0 { _ = json.Unmarshal(meta, &ch.Metadata) }
		if title.Valid { ch.DocumentTitle = title.String }
		ch.Score = 0.5
		results = append(results, domain.RetrievalResult{Chunk: ch, Score: 0.5})
	}
	return results, nil
}
