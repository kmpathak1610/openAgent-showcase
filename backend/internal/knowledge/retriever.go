package knowledge

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/llm"
	"openagent/internal/repository"
)

// Retriever implements domain.KnowledgeRetriever using pgvector + permission filtering
type Retriever struct {
	db       *repository.DB
	embedder llm.Embedder
	reranker domain.Reranker // optional
}

func NewRetriever(db *repository.DB, embedder llm.Embedder, reranker domain.Reranker) *Retriever {
	return &Retriever{db: db, embedder: embedder, reranker: reranker}
}

func (r *Retriever) SemanticSearch(ctx context.Context, query string, filter domain.RetrievalFilter) ([]domain.RetrievalResult, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("embedding provider not configured: set OPENROUTER_API_KEY or EMBEDDING_PROVIDER")
	}
	// embed query
	embReq := llm.EmbedRequest{Input: []string{query}, Model: "text-embedding-3-small"}
	res, err := r.embedder.Embed(ctx, embReq)
	if err != nil { return nil, err }
	if len(res.Embeddings) == 0 { return nil, nil }
	embedding := res.Embeddings[0]
	limit := filter.Limit
	if limit == 0 { limit = 5 }
	// Enforce permission filtering via GetAllowedDocIDs inside VectorSearch
	// Also enforce metadata filtering if filter.Metadata provided (post-filter)
	results, err := r.db.VectorSearch(filter.OrganizationID, embedding, filter, limit*2) // over-fetch for filtering
	if err != nil { return nil, err }
	// metadata filtering
	if len(filter.Metadata) > 0 {
		filtered := []domain.RetrievalResult{}
		for _, res := range results {
			match := true
			for k, v := range filter.Metadata {
				if mv, ok := res.Chunk.Metadata[k]; !ok || stringify(mv) != v {
					match = false
					break
				}
			}
			if match { filtered = append(filtered, res) }
		}
		results = filtered
	}
	if len(results) > limit { results = results[:limit] }
	// rerank if enabled
	if r.reranker != nil {
		return r.reranker.Rerank(query, results)
	}
	return results, nil
}

func (r *Retriever) HybridSearch(ctx context.Context, query string, filter domain.RetrievalFilter) ([]domain.RetrievalResult, error) {
	sem, err := r.SemanticSearch(ctx, query, filter)
	if err != nil { return nil, err }
	kw, err := r.db.KeywordSearch(filter.OrganizationID, query, filter, filter.Limit)
	if err != nil { return nil, err }
	// merge and dedup by chunk ID, keep highest score
	seen := map[uuid.UUID]domain.RetrievalResult{}
	for _, res := range sem { seen[res.Chunk.ID] = res }
	for _, res := range kw {
		if existing, ok := seen[res.Chunk.ID]; ok {
			// boost score if appears in both
			if res.Score > existing.Score { seen[res.Chunk.ID] = res }
			// else keep sem
		} else {
			seen[res.Chunk.ID] = res
		}
	}
	merged := make([]domain.RetrievalResult, 0, len(seen))
	for _, v := range seen { merged = append(merged, v) }
	sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	limit := filter.Limit
	if limit == 0 { limit = 5 }
	if len(merged) > limit { merged = merged[:limit] }
	if r.reranker != nil {
		return r.reranker.Rerank(query, merged)
	}
	return merged, nil
}

func (r *Retriever) ContextualRetrieval(ctx context.Context, query string, agentID, projectID, channelID *uuid.UUID, limit int) ([]domain.RetrievalResult, error) {
	// Determine org from context? For Phase 3, we need org via agent's org or project's org.
	// Caller should provide filter with org; here we try to infer via agent/project lookups
	// For now, we require that caller passes agentID that belongs to org, and we fetch org via DB
	// Simplify: search with agent+project filter and rely on GetAllowedDocIDs to enforce
	var orgID uuid.UUID
	if agentID != nil {
		// fetch agent org
		agent, err := r.db.GetAgentByID(*agentID) // need method that doesn't require org; we'll add fallback
		if err == nil && agent != nil {
			orgID = agent.OrganizationID
		}
	}
	if orgID == uuid.Nil && projectID != nil {
		proj, err := r.db.GetProjectByID(*projectID)
		if err == nil && proj != nil {
			orgID = proj.OrganizationID
		}
	}
	// If still nil, we cannot filter — caller should have provided org via other path
	// For Phase 3, we expose a more explicit method: SemanticSearch with full filter
	filter := domain.RetrievalFilter{
		OrganizationID: orgID,
		AgentID:        agentID,
		ProjectID:      projectID,
		ChannelID:      channelID,
		Limit:          limit,
	}
	return r.SemanticSearch(ctx, query, filter)
}

func stringify(v any) string {
	if s, ok := v.(string); ok { return s }
	return ""
}
