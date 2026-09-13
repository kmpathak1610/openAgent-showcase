package runtime

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"openagent/internal/domain"
)

func msgRow(orgID, channelID, msgID uuid.UUID) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "organization_id", "channel_id", "thread_id", "sender_type", "sender_user_id", "sender_agent_id", "body", "body_format", "metadata", "edited_at", "deleted_at", "created_at", "updated_at"}).
		AddRow(msgID, orgID, channelID, nil, "user", uuid.New(), nil, "hello", "text", nil, nil, nil, now, now)
}

func TestResolveThreadID_DelegatedThreadsUnderUserMessage(t *testing.T) {
	rt, mock, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	channelID := uuid.New()
	userMsgID := uuid.New()
	parentID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at FROM messages WHERE id=$1 AND organization_id=$2`)).
		WithArgs(userMsgID, orgID).
		WillReturnRows(msgRow(orgID, channelID, userMsgID))

	task := &domain.Task{
		ID:            uuid.New(),
		ChannelID:     &channelID,
		ParentTaskID:  &parentID,
		CorrelationID: &userMsgID,
	}
	got := rt.resolveThreadID(orgID, task)
	if got == nil {
		t.Fatal("expected threadID for delegated task, got nil")
	}
	if *got != userMsgID {
		t.Fatalf("expected thread %s, got %s", userMsgID, *got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestResolveThreadID_FirstResponderInline(t *testing.T) {
	rt, _, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	channelID := uuid.New()
	userMsgID := uuid.New()
	task := &domain.Task{
		ID:            uuid.New(),
		ChannelID:     &channelID,
		ParentTaskID:  nil,
		CorrelationID: &userMsgID,
	}
	got := rt.resolveThreadID(orgID, task)
	if got != nil {
		t.Fatalf("expected nil thread for first responder, got %s", got.String())
	}
}

func TestResolveThreadID_ChannelMismatchNil(t *testing.T) {
	rt, mock, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	taskChannel := uuid.New()
	otherChannel := uuid.New()
	userMsgID := uuid.New()
	parentID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,organization_id,channel_id,thread_id,sender_type,sender_user_id,sender_agent_id,body,body_format,metadata,edited_at,deleted_at,created_at,updated_at FROM messages WHERE id=$1 AND organization_id=$2`)).
		WithArgs(userMsgID, orgID).
		WillReturnRows(msgRow(orgID, otherChannel, userMsgID))

	task := &domain.Task{
		ID:            uuid.New(),
		ChannelID:     &taskChannel,
		ParentTaskID:  &parentID,
		CorrelationID: &userMsgID,
	}
	if got := rt.resolveThreadID(orgID, task); got != nil {
		t.Fatalf("expected nil for channel mismatch, got %s", got.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestResolveThreadID_NilCorrelationNil(t *testing.T) {
	rt, _, close := newTestRuntime(t)
	defer close()
	orgID := uuid.New()
	channelID := uuid.New()
	parentID := uuid.New()
	task := &domain.Task{
		ID:            uuid.New(),
		ChannelID:     &channelID,
		ParentTaskID:  &parentID,
		CorrelationID: nil,
	}
	if got := rt.resolveThreadID(orgID, task); got != nil {
		t.Fatalf("expected nil for nil correlation, got %s", got.String())
	}
}
