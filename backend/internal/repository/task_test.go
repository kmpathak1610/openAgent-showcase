package repository

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestTask_OrgIsolation_CreateAndList(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgA := uuid.New()
	orgB := uuid.New()
	projectA := uuid.New()
	// ListTasks for orgA should filter by org
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(orgA, 20).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","required_role","required_capabilities","preferred_agent_id","created_at","updated_at"}).
			AddRow(uuid.New(), orgA, projectA, nil, nil, nil, "Task A", "desc", nil, nil, nil, "pending", "medium", nil, nil, nil, nil, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(orgB, 20).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","required_role","required_capabilities","preferred_agent_id","created_at","updated_at"}))
	tasksA, _ := db.ListTasks(orgA, nil, nil, "", nil, 20)
	tasksB, _ := db.ListTasks(orgB, nil, nil, "", nil, 20)
	if len(tasksA)!=1 || tasksA[0].Title!="Task A" { t.Fatalf("orgA isolation failed") }
	if len(tasksB)!=0 { t.Fatalf("orgB should be empty") }
	if err:=mock.ExpectationsWereMet(); err!=nil { t.Fatal(err) }
}

func TestTask_Delegation_Hierarchy(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	parentID := uuid.New()
	childID := uuid.New()
	projectID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(orgID, 100).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","required_role","required_capabilities","preferred_agent_id","created_at","updated_at"}).
			AddRow(parentID, orgID, projectID, nil, nil, nil, "Parent", "desc", nil, nil, nil, "pending", "medium", nil, nil, nil, nil, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")).
			AddRow(childID, orgID, projectID, nil, parentID, nil, "Child", "desc", nil, nil, nil, "pending", "medium", nil, nil, nil, nil, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")))
	tasks, _ := db.ListTasks(orgID, nil, nil, "", nil, 100)
	if len(tasks)!=2 { t.Fatalf("expected 2 tasks") }
	var foundParent, foundChild bool
	for _, tsk := range tasks {
		if tsk.ID==parentID && tsk.ParentTaskID==nil { foundParent=true }
		if tsk.ID==childID && tsk.ParentTaskID!=nil && *tsk.ParentTaskID==parentID { foundChild=true }
	}
	if !foundParent || !foundChild { t.Fatal("hierarchy not preserved") }
	if err:=mock.ExpectationsWereMet(); err!=nil { t.Fatal(err) }
}

func TestTask_Permission_AssignedAgent(t *testing.T) {
	db, mock, close := newMockDB(t)
	defer close()
	orgID := uuid.New()
	agentID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, project_id, channel_id, parent_task_id, team_id, title, description, created_by, assigned_to_agent, assigned_to_user, status, priority, deadline, correlation_id, required_role, required_capabilities, preferred_agent_id, created_at, updated_at FROM tasks WHERE organization_id=$1 AND assigned_to_agent=$2 ORDER BY created_at DESC LIMIT $3`)).
		WithArgs(orgID, agentID, 20).WillReturnRows(sqlmock.NewRows([]string{"id","organization_id","project_id","channel_id","parent_task_id","team_id","title","description","created_by","assigned_to_agent","assigned_to_user","status","priority","deadline","correlation_id","required_role","required_capabilities","preferred_agent_id","created_at","updated_at"}).
			AddRow(uuid.New(), orgID, uuid.New(), nil, nil, nil, "Agent Task", "desc", nil, agentID, nil, "assigned", "high", nil, nil, nil, nil, nil, mustParseTime("2024-01-01T00:00:00Z"), mustParseTime("2024-01-01T00:00:00Z")))
	tasks, _ := db.ListTasks(orgID, nil, nil, "", &agentID, 20)
	if len(tasks)!=1 || tasks[0].AssignedToAgent==nil || *tasks[0].AssignedToAgent!=agentID { t.Fatal("assigned agent filter failed") }
	if err:=mock.ExpectationsWereMet(); err!=nil { t.Fatal(err) }
}
