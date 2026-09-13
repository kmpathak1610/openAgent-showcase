package repository

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/domain"
)

func TestKnowledge_TenantIsolation_ListDocuments(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgA := uuid.New()
	orgB := uuid.New()
	// orgA docs
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, size_bytes, status, scope, version, metadata, created_by, created_at, updated_at FROM documents WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(orgA, 20).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","collection_id","source_id","title","source_type","source_url","mime_type","size_bytes","status","scope","version","metadata","created_by","created_at","updated_at"}).
			AddRow(uuid.New(), orgA, nil, nil, nil, "Brand Guidelines", "text", nil, "text/plain", 100, "ready", "organization", 1, []byte(`{}`), nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, collection_id, source_id, title, source_type, source_url, mime_type, size_bytes, status, scope, version, metadata, created_by, created_at, updated_at FROM documents WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(orgB, 20).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","collection_id","source_id","title","source_type","source_url","mime_type","size_bytes","status","scope","version","metadata","created_by","created_at","updated_at"}))
	docsA, _ := db.ListDocuments(orgA, nil, "", "", 20)
	docsB, _ := db.ListDocuments(orgB, nil, "", "", 20)
	if len(docsA)!=1 || docsA[0].Title!="Brand Guidelines" { t.Fatalf("orgA isolation failed") }
	if len(docsB)!=0 { t.Fatalf("orgB should have 0, got %d", len(docsB)) }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestKnowledge_AgentSpecific_Access(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	agentID := uuid.New()
	otherAgent := uuid.New()
	docID := uuid.New()
	// Setup: doc belongs to org, status ready
	// GetAllowedDocIDs for agentID should include agent knowledge
	// Mock base org docs query
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM documents WHERE organization_id=$1 AND status='ready'`)).
		WithArgs(orgID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(docID))
	// Agent knowledge doc ids
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT document_id FROM agent_knowledge WHERE agent_id=$1 AND document_id IS NOT NULL`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"document_id"}).AddRow(docID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT collection_id FROM agent_knowledge WHERE agent_id=$1 AND collection_id IS NOT NULL`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"collection_id"}))
	allowed, _ := db.GetAllowedDocIDs(domain.RetrievalFilter{OrganizationID: orgID, AgentID: &agentID})
	if len(allowed)!=1 || allowed[0]!=docID { t.Fatalf("agent should have access, got %v", allowed) }

	// other agent without knowledge should still have org doc (since org docs are visible to all)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM documents WHERE organization_id=$1 AND status='ready'`)).
		WithArgs(orgID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(docID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT document_id FROM agent_knowledge WHERE agent_id=$1 AND document_id IS NOT NULL`)).
		WithArgs(otherAgent).WillReturnRows(sqlmock.NewRows([]string{"document_id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT collection_id FROM agent_knowledge WHERE agent_id=$1 AND collection_id IS NOT NULL`)).
		WithArgs(otherAgent).WillReturnRows(sqlmock.NewRows([]string{"collection_id"}))
	allowed2, _ := db.GetAllowedDocIDs(domain.RetrievalFilter{OrganizationID: orgID, AgentID: &otherAgent})
	// For Phase 3, org docs are visible to all agents (union), so other agent still sees org doc
	if len(allowed2)!=1 { t.Fatalf("other agent should see org doc, got %v", allowed2) }

	// Now test explicit scope filtering: only agent scope
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM documents WHERE organization_id=$1 AND status='ready' AND scope=$2`)).
		WithArgs(orgID, "agent").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT document_id FROM agent_knowledge WHERE agent_id=$1 AND document_id IS NOT NULL`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"document_id"}).AddRow(docID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT collection_id FROM agent_knowledge WHERE agent_id=$1 AND collection_id IS NOT NULL`)).
		WithArgs(agentID).WillReturnRows(sqlmock.NewRows([]string{"collection_id"}))
	allowed3, _ := db.GetAllowedDocIDs(domain.RetrievalFilter{OrganizationID: orgID, AgentID: &agentID, Scope: "agent"})
	if len(allowed3)!=1 { t.Fatalf("agent scope should return doc via knowledge, got %v", allowed3) }

	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestKnowledge_PermissionFiltering_AllowedDocIDs(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	doc1 := uuid.New()
	doc2 := uuid.New()
	// base org docs includes both docs
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM documents WHERE organization_id=$1 AND status='ready'`)).
		WithArgs(orgID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(doc1).AddRow(doc2))
	// filter with AllowedDocIDs whitelist = only doc1
	allowed, _ := db.GetAllowedDocIDs(domain.RetrievalFilter{OrganizationID: orgID, AllowedDocIDs: []uuid.UUID{doc1}})
	if len(allowed)!=1 || allowed[0]!=doc1 { t.Fatalf("should filter to doc1, got %v", allowed) }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestKnowledge_VectorRetrieval_RequiresPermission(t *testing.T) {
	// VectorSearch should respect allowedIDs and not leak cross-org
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	_ = orgID
	// Mock GetAllowedDocIDs will be called inside VectorSearch; we mock its internal queries
	// For this test, we mock GetAllowedDocIDs indirectly via its DB queries
	// Instead we test that VectorSearch returns empty if no allowed docs
	// Setup GetAllowedDocIDs to return empty
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM documents WHERE organization_id=$1 AND status='ready'`)).
		WithArgs(orgID).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// No agent/project, so allowed = 0, VectorSearch should return empty without querying chunks
	filter := domain.RetrievalFilter{OrganizationID: orgID, Limit: 5}
	results, err := db.VectorSearch(orgID, []float32{0.1, 0.2, 0.3}, filter, 5)
	if err != nil { t.Fatalf("vector search err: %v", err) }
	if len(results)!=0 { t.Fatalf("expected 0 results for no docs, got %d", len(results)) }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}
