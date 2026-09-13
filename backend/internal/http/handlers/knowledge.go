package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/http/middleware"
	"openagent/internal/knowledge"
	"openagent/internal/repository"
	"openagent/internal/worker"
)

type KnowledgeHandler struct {
	db       *repository.DB
	ingest   *knowledge.Service
	retriever *knowledge.Retriever
	worker   *worker.Pool
}

func NewKnowledgeHandler(db *repository.DB, ingest *knowledge.Service, retriever *knowledge.Retriever, worker *worker.Pool) *KnowledgeHandler {
	return &KnowledgeHandler{db: db, ingest: ingest, retriever: retriever, worker: worker}
}

// Sources

func (h *KnowledgeHandler) CreateSource(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Name       string         `json:"name"`
		SourceType string         `json:"sourceType"`
		Config     map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Name == "" || req.SourceType == "" { writeError(w, 400, "VALIDATION_ERROR", "name and sourceType required"); return }
	src, err := h.db.CreateKnowledgeSource(claims.OrganizationID, req.Name, req.SourceType, req.Config, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, src, nil)
}

func (h *KnowledgeHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	srcs, err := h.db.ListKnowledgeSources(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if srcs == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, srcs, nil)
}

// Collections

func (h *KnowledgeHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct{ Name string `json:"name"`; Description string `json:"description"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Name == "" { writeError(w, 400, "VALIDATION_ERROR", "name required"); return }
	col, err := h.db.CreateKnowledgeCollection(claims.OrganizationID, req.Name, req.Description, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, col, nil)
}

func (h *KnowledgeHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	colls, err := h.db.ListKnowledgeCollections(claims.OrganizationID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if colls == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, colls, nil)
}

// Documents

func (h *KnowledgeHandler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	// support multipart and json
	var title, sourceType, scope, content, sourceURL, collectionIDStr, projectIDStr string
	var mimeType = "text/plain"
	var fileBytes []byte
	var metadata map[string]any

	ctype := r.Header.Get("Content-Type")
	if strings.Contains(ctype, "multipart/form-data") {
		if err := r.ParseMultipartForm(20 << 20); err != nil { writeError(w, 400, "VALIDATION_ERROR", "multipart parse failed"); return }
		title = r.FormValue("title")
		sourceType = r.FormValue("source_type")
		if sourceType == "" { sourceType = r.FormValue("sourceType") }
		scope = r.FormValue("scope")
		collectionIDStr = r.FormValue("collection_id")
		projectIDStr = r.FormValue("projectId")
		sourceURL = r.FormValue("source_url")
		mimeType = r.FormValue("mime_type")
		if mimeType == "" { mimeType = "text/plain" }
		content = r.FormValue("content")
		// file
		if file, header, err := r.FormFile("file"); err == nil {
			defer file.Close()
			fileBytes, _ = io.ReadAll(io.LimitReader(file, 10<<20))
			if title == "" && header.Filename != "" { title = header.Filename }
			if mimeType == "text/plain" && header.Header.Get("Content-Type") != "" { mimeType = header.Header.Get("Content-Type") }
			extracted := knowledge.ExtractFileContentWithFilename(fileBytes, mimeType, header.Filename)
			// If parser failed, ExtractFileContent returns marker starting with "[extraction via"
			if strings.HasPrefix(extracted, "[extraction via") {
				// Persist failure reason in metadata for ingest to mark FAILED
				if metadata == nil {
					metadata = map[string]any{}
				}
				metadata["extraction_error"] = extracted
				metadata["parser"] = knowledge.GetParser(mimeType, header.Filename).Name()
				metadata["failed_at"] = true
				content = "" // will trigger FAILED status after CreateDocument
				// Store marker as content for debugging but mark for failure
				content = extracted
			} else {
				content = extracted
			}
			if sourceType == "" { sourceType = "upload" }
		}
		if sourceType == "" { sourceType = "upload" }
	} else {
		var req struct {
			Title        string         `json:"title"`
			SourceType   string         `json:"sourceType"`
			SourceURL    string         `json:"sourceUrl"`
			MimeType     string         `json:"mimeType"`
			Scope        string         `json:"scope"`
			Content      string         `json:"content"`
			Text         string         `json:"text"`
			CollectionID string         `json:"collectionId"`
			ProjectID    string         `json:"projectId"`
			Metadata     map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
		title = req.Title
		sourceType = req.SourceType
		scope = req.Scope
		content = req.Content
		if content == "" { content = req.Text }
		sourceURL = req.SourceURL
		collectionIDStr = req.CollectionID
		projectIDStr = req.ProjectID
		metadata = req.Metadata
		mimeType = req.MimeType
		if mimeType == "" { mimeType = "text/plain" }
		if sourceType == "" {
			if sourceURL != "" { sourceType = "website" } else if content != "" { sourceType = "text" } else { sourceType = "upload" }
		}
	}

	if title == "" { writeError(w, 400, "VALIDATION_ERROR", "title required"); return }
	if sourceType == "" { sourceType = "text" }
	if scope == "" { scope = "organization" }
	if scope == "project" && projectIDStr == "" {
		writeError(w, 400, "VALIDATION_ERROR", "projectId required for project scope")
		return
	}

	// handle website fetch if sourceType website and content empty but url provided
	if sourceType == "website" && content == "" && sourceURL != "" {
		fetched, err := knowledge.ExtractWebsiteContent(sourceURL)
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", "failed to fetch website: "+err.Error())
			return
		}
		content = fetched
	}
	if content == "" {
		writeError(w, 400, "VALIDATION_ERROR", "content required (upload file, website URL, or text)")
		return
	}

	var projectID *uuid.UUID
	if projectIDStr != "" {
		pid, err := uuid.Parse(projectIDStr)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid projectId"); return }
		// verify project belongs to org
		if _, err := h.db.GetProject(claims.OrganizationID, pid); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
		projectID = &pid
		if scope == "organization" { scope = "project" }
	}
	var collectionID *uuid.UUID
	if collectionIDStr != "" {
		cid, err := uuid.Parse(collectionIDStr)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid collectionId"); return }
		if _, err := h.db.GetKnowledgeCollection(claims.OrganizationID, cid); err != nil { writeError(w, 404, "NOT_FOUND", "collection not found"); return }
		collectionID = &cid
	}
	var sourceID *uuid.UUID
	// optionally create knowledge source for tracking? For file/website we could auto-create source
	// For Phase 3, we store source_id as nil unless integration

	var srcURLPtr *string
	if sourceURL != "" { srcURLPtr = &sourceURL }

	doc, err := h.db.CreateDocument(claims.OrganizationID, projectID, collectionID, sourceID, title, sourceType, srcURLPtr, mimeType, scope, content, metadata, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }

	// If parser failed (marker), mark document FAILED immediately and don't enqueue
	if strings.HasPrefix(content, "[extraction via") {
		_ = h.db.UpdateDocumentStatus(claims.OrganizationID, doc.ID, "failed")
		// Update metadata with failure details
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["status"] = "failed"
		metadata["failed_at"] = true
		writeData(w, 201, doc, nil)
		return
	}

	// enqueue async ingestion
	if h.worker != nil {
		h.worker.Enqueue(worker.Job{
			ID:   doc.ID.String(),
			Type: "ingest_document",
			Payload: map[string]any{
				"document_id": doc.ID.String(),
				"organization_id": claims.OrganizationID.String(),
				"content": content,
			},
		})
	} else {
		// fallback sync for tests without worker
		go func() {
			_ = h.ingest.IngestDocument(nil, claims.OrganizationID, doc.ID, content)
		}()
	}

	writeData(w, 201, doc, nil)
}

func (h *KnowledgeHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	q := r.URL.Query()
	var projectID *uuid.UUID
	if pidStr := q.Get("projectId"); pidStr != "" {
		pid, _ := uuid.Parse(pidStr)
		projectID = &pid
	}
	scope := q.Get("scope")
	status := q.Get("status")
	limit, _ := strconv.Atoi(q.Get("limit"))
	docs, err := h.db.ListDocuments(claims.OrganizationID, projectID, scope, status, limit)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if docs == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, docs, nil)
}

func (h *KnowledgeHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	docID, _ := uuid.Parse(chi.URLParam(r, "id"))
	doc, err := h.db.GetDocument(claims.OrganizationID, docID)
	if err != nil { writeError(w, 404, "NOT_FOUND", "document not found"); return }
	writeData(w, 200, doc, nil)
}

func (h *KnowledgeHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	docID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if err := h.db.DeleteDocument(claims.OrganizationID, docID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"deleted": true}, nil)
}

func (h *KnowledgeHandler) ListDocumentChunks(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	docID, _ := uuid.Parse(chi.URLParam(r, "id"))
	// verify org
	if _, err := h.db.GetDocument(claims.OrganizationID, docID); err != nil { writeError(w, 404, "NOT_FOUND", "document not found"); return }
	chunks, err := h.db.ListDocumentChunks(docID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, chunks, nil)
}

func (h *KnowledgeHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		Query     string            `json:"query"`
		Limit     int               `json:"limit"`
		ProjectID string            `json:"projectId"`
		AgentID   string            `json:"agentId"`
		Scope     string            `json:"scope"`
		Metadata  map[string]string `json:"metadata"`
		Hybrid    bool              `json:"hybrid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Query == "" { writeError(w, 400, "VALIDATION_ERROR", "query required"); return }
	filter := domain.RetrievalFilter{
		OrganizationID: claims.OrganizationID,
		Scope:          req.Scope,
		Metadata:       req.Metadata,
		Limit:          req.Limit,
	}
	if req.ProjectID != "" {
		pid, err := uuid.Parse(req.ProjectID)
		if err == nil { filter.ProjectID = &pid }
	}
	if req.AgentID != "" {
		aid, err := uuid.Parse(req.AgentID)
		if err == nil { filter.AgentID = &aid }
	}
	if filter.Limit == 0 { filter.Limit = 5 }
	var results []domain.RetrievalResult
	var err error
	if req.Hybrid {
		results, err = h.retriever.HybridSearch(r.Context(), req.Query, filter)
	} else {
		results, err = h.retriever.SemanticSearch(r.Context(), req.Query, filter)
	}
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if results == nil { results = []domain.RetrievalResult{} }
	writeData(w, 200, results, nil)
}

// Agent knowledge attach/detach

func (h *KnowledgeHandler) AttachAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	agentID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, agentID); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	var req struct {
		DocumentID   *string `json:"documentId"`
		CollectionID *string `json:"collectionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	var docID, collID *uuid.UUID
	if req.DocumentID != nil && *req.DocumentID != "" {
		uid, _ := uuid.Parse(*req.DocumentID)
		// verify doc belongs to org
		if _, err := h.db.GetDocument(claims.OrganizationID, uid); err != nil { writeError(w, 404, "NOT_FOUND", "document not found"); return }
		docID = &uid
	}
	if req.CollectionID != nil && *req.CollectionID != "" {
		uid, _ := uuid.Parse(*req.CollectionID)
		if _, err := h.db.GetKnowledgeCollection(claims.OrganizationID, uid); err != nil { writeError(w, 404, "NOT_FOUND", "collection not found"); return }
		collID = &uid
	}
	if docID == nil && collID == nil { writeError(w, 400, "VALIDATION_ERROR", "documentId or collectionId required"); return }
	ak, err := h.db.AttachAgentKnowledge(agentID, docID, collID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, ak, nil)
}

func (h *KnowledgeHandler) ListAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	agentID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetAgent(claims.OrganizationID, agentID); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	aks, err := h.db.ListAgentKnowledge(agentID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if aks == nil { writeData(w, 200, []any{}, nil); return }
	// hydrate documents/collections
	for _, ak := range aks {
		if ak.DocumentID != nil {
			if doc, err := h.db.GetDocument(claims.OrganizationID, *ak.DocumentID); err == nil { ak.Document = doc }
		}
		if ak.CollectionID != nil {
			if col, err := h.db.GetKnowledgeCollection(claims.OrganizationID, *ak.CollectionID); err == nil { ak.Collection = col }
		}
	}
	writeData(w, 200, aks, nil)
}

func (h *KnowledgeHandler) DetachAgentKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	agentID, _ := uuid.Parse(chi.URLParam(r, "id"))
	knowledgeID, _ := uuid.Parse(chi.URLParam(r, "knowledgeId"))
	if _, err := h.db.GetAgent(claims.OrganizationID, agentID); err != nil { writeError(w, 404, "NOT_FOUND", "agent not found"); return }
	if err := h.db.DetachAgentKnowledge(agentID, knowledgeID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"detached": true}, nil)
}

// Project knowledge

func (h *KnowledgeHandler) AttachProjectKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	projectID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, projectID); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	var req struct {
		DocumentID   *string `json:"documentId"`
		CollectionID *string `json:"collectionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	var docID, collID *uuid.UUID
	if req.DocumentID != nil && *req.DocumentID != "" {
		uid, _ := uuid.Parse(*req.DocumentID)
		if _, err := h.db.GetDocument(claims.OrganizationID, uid); err != nil { writeError(w, 404, "NOT_FOUND", "document not found"); return }
		docID = &uid
	}
	if req.CollectionID != nil && *req.CollectionID != "" {
		uid, _ := uuid.Parse(*req.CollectionID)
		if _, err := h.db.GetKnowledgeCollection(claims.OrganizationID, uid); err != nil { writeError(w, 404, "NOT_FOUND", "collection not found"); return }
		collID = &uid
	}
	if docID == nil && collID == nil { writeError(w, 400, "VALIDATION_ERROR", "documentId or collectionId required"); return }
	pk, err := h.db.AttachProjectKnowledge(projectID, docID, collID, claims.UserID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 201, pk, nil)
}

func (h *KnowledgeHandler) ListProjectKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	projectID, _ := uuid.Parse(chi.URLParam(r, "id"))
	if _, err := h.db.GetProject(claims.OrganizationID, projectID); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	pks, err := h.db.ListProjectKnowledge(projectID)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if pks == nil { writeData(w, 200, []any{}, nil); return }
	writeData(w, 200, pks, nil)
}

func (h *KnowledgeHandler) DetachProjectKnowledge(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	projectID, _ := uuid.Parse(chi.URLParam(r, "id"))
	knowledgeID, _ := uuid.Parse(chi.URLParam(r, "knowledgeId"))
	if _, err := h.db.GetProject(claims.OrganizationID, projectID); err != nil { writeError(w, 404, "NOT_FOUND", "project not found"); return }
	if err := h.db.DetachProjectKnowledge(projectID, knowledgeID); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	writeData(w, 200, map[string]any{"detached": true}, nil)
}
