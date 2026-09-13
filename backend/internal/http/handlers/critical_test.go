package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/auth"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/worker"
	"openagent/internal/ws"
	"log/slog"
)

func mustParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func TestCriticalPath_HumanTaskAgentApproval(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil { t.Fatal(err) }
	defer db.Close()
	mock.MatchExpectationsInOrder(false)
	repo := repository.New(db)
	authSvc := auth.New("test-secret-32-chars-long-enough-12345")
	hub := ws.NewHub(slog.Default())
	workerPool := worker.New(10, slog.Default())

	orgID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	agentID := uuid.New()
	channelID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM organization_members WHERE organization_id=$1 AND user_id=$2`)).WillReturnRows(
		sqlmock.NewRows([]string{"role"}).AddRow("owner"),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,name,slug,description,objective,icon,status,settings,created_by,created_at,updated_at FROM projects WHERE id=$1 AND organization_id=$2`)).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","name","slug","description","objective","icon","status","settings","created_by","created_at","updated_at"}).
			AddRow(projectID, orgID, "Test Project", "test-project", "desc", "obj", "📁", "active", []byte(`{}`), userID, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,project_id,name,display_name,description,topic,channel_type,created_by,created_at,updated_at FROM channels WHERE id=$1 AND organization_id=$2`)).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","project_id","name","display_name","description","topic","channel_type","created_by","created_at","updated_at"}).
			AddRow(channelID, orgID, projectID, "general", "General", "desc", "topic", "standard", userID, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, name, slug, description, avatar_url, avatar, purpose, intent, system_prompt, model, provider, capabilities, tools, status, autonomy_level, current_version, owner_id, created_by, created_at, updated_at FROM agents WHERE id=$1 AND organization_id=$2`)).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","name","slug","description","avatar_url","avatar","purpose","intent","system_prompt","model","provider","capabilities","tools","status","autonomy_level","current_version","owner_id","created_by","created_at","updated_at"}).
			AddRow(agentID, orgID, "Test Agent", "test-agent", "desc", nil, nil, "purpose", "intent", "prompt", "model", "stub", []byte(`[]`), []byte(`[]`), "active", "assistant", 1, userID, userID, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")),
	)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO tasks`).WillReturnRows(
		sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","created_at","updated_at"}).
			AddRow(uuid.New(), orgID, projectID, channelID, nil, nil, "Test Task", "desc", userID, agentID, nil, "assigned", "high", nil, uuid.New(), mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")),
	)
	mock.ExpectExec(`INSERT INTO task_assignments`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectExec(`INSERT INTO task_events`).WillReturnResult(sqlmock.NewResult(1,1))
	mock.ExpectCommit()

	taskH := NewTaskHandler(repo, workerPool, hub)
	token, _ := authSvc.CreateToken(userID, orgID, "owner", "test@example.com", 3600000000000)
	reqBody, _ := json.Marshal(map[string]any{
		"projectId": projectID.String(),
		"channelId": channelID.String(),
		"title": "Test Task",
		"description": "desc",
		"priority": "high",
		"assignedToAgent": agentID.String(),
	})
	req := httptest.NewRequest("POST", "/tasks", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	claims, _ := authSvc.VerifyToken(token)
	ctx := context.WithValue(req.Context(), middleware.CtxClaims, claims)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/tasks", taskH.Create)
	r.ServeHTTP(rr, req)

	if rr.Code != 201 {
		t.Fatalf("expected 201, got %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["data"] == nil {
		t.Fatal("expected data")
	}
}
