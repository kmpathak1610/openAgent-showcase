package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ——— Organizations & Users

type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	AvatarURL *string   `json:"avatarUrl"`
	Settings  map[string]any `json:"settings"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	AvatarURL    *string   `json:"avatarUrl"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	LastSeenAt   *time.Time `json:"lastSeenAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type OrganizationMember struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	UserID         uuid.UUID `json:"userId"`
	Role           string    `json:"role"` // owner, admin, member, guest
	JoinedAt       time.Time `json:"joinedAt"`
	User           *User     `json:"user,omitempty"`
}

// ——— Agents — Phase 2 structured model

var ValidAutonomyLevels = map[string]bool{
	"assistant":      true,
	"task_executor":  true,
	"collaborative":  true,
	"autonomous":     true,
}

var ValidAgentStatuses = map[string]bool{
	"draft": true, "active": true, "paused": true, "archived": true,
}

type Agent struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description"`
	AvatarURL      *string    `json:"avatarUrl"`
	Avatar         *string    `json:"avatar"` // alias
	Purpose        string     `json:"purpose"`
	Intent         string     `json:"intent"`        // original NL description (legacy)
	SystemPrompt   string     `json:"systemPrompt"` // derived (legacy, now in version)
	Model          string     `json:"model"`
	Provider       string     `json:"provider"`
	Capabilities   []string   `json:"capabilities"` // legacy flat, now structured via agent_capabilities
	Tools          []string   `json:"tools"`
	Status         string     `json:"status"`
	AutonomyLevel  string     `json:"autonomyLevel"`
	CurrentVersion int        `json:"currentVersion"`
	LastStatus     string     `json:"lastStatus"` // idle/working/waiting/blocked/approval_required/failed/offline
	LastStatusAt   *time.Time `json:"lastStatusAt"`
	OwnerID        *uuid.UUID `json:"ownerId"`
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	// hydrated
	Version        *AgentVersion     `json:"version,omitempty"`
	CapabilityList []AgentCapability `json:"capabilityList,omitempty"`
	Permissions    []AgentPermission `json:"permissions,omitempty"`
}

type AgentVersion struct {
	ID                uuid.UUID      `json:"id"`
	AgentID           uuid.UUID      `json:"agentId"`
	Version           int            `json:"version"`
	Config            map[string]any `json:"config"` // legacy
	ChangeSummary     string         `json:"changeSummary"`
	Role              string         `json:"role"`
	Objective         string         `json:"objective"`
	Instructions      string         `json:"instructions"`
	Capabilities      []AgentCapability `json:"capabilities"`
	BehavioralRules   []string       `json:"behavioralRules"`
	ToolPolicy        map[string]any `json:"toolPolicy"`
	MemoryPolicy      map[string]any `json:"memoryPolicy"`
	ApprovalPolicy    map[string]any `json:"approvalPolicy"`
	ModelConfiguration map[string]any `json:"modelConfiguration"`
	CreatedBy         *uuid.UUID     `json:"createdBy"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type AgentCapability struct {
	ID          uuid.UUID `json:"id"`
	AgentID     uuid.UUID `json:"agentId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AgentPermission struct {
	ID           uuid.UUID  `json:"id"`
	AgentID      uuid.UUID  `json:"agentId"`
	ResourceType string     `json:"resourceType"` // organization/project/channel/tool/knowledge/document
	ResourceID   *uuid.UUID `json:"resourceId"`
	Permission   string     `json:"permission"` // read/write/execute/admin
	GrantedBy    *uuid.UUID `json:"grantedBy"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// Builder preview (not persisted, returned to UI)
type AgentBuilderPreview struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Purpose           string            `json:"purpose"`
	Role              string            `json:"role"`
	Objective         string            `json:"objective"`
	Responsibilities  []string          `json:"responsibilities"`
	Instructions      string            `json:"instructions"`
	Capabilities      []AgentCapability `json:"capabilities"`
	BehavioralRules   []string          `json:"behavioralRules"`
	ToolPolicy        map[string]any    `json:"toolPolicy"`
	MemoryPolicy      map[string]any    `json:"memoryPolicy"`
	ApprovalPolicy    map[string]any    `json:"approvalPolicy"`
	ModelConfiguration map[string]any   `json:"modelConfiguration"`
	AutonomyLevel     string            `json:"autonomyLevel"`
	Avatar            string            `json:"avatar"`
	KnowledgeNeeds    []string          `json:"knowledgeNeeds"`
	RecommendedTools  []string          `json:"recommendedTools"`
	Permissions       []AgentPermission `json:"permissions"`
	Warnings          []string          `json:"warnings,omitempty"`
}

// ——— Projects

type Project struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description"`
	Objective      string    `json:"objective"`
	Icon           string    `json:"icon"`
	Status         string    `json:"status"`
	Settings       map[string]any `json:"settings"`
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ProjectMember struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"projectId"`
	UserID    uuid.UUID `json:"userId"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joinedAt"`
	User      *User     `json:"user,omitempty"`
}

// ——— Channels

type Channel struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	ProjectID      *uuid.UUID `json:"projectId"`
	Name           string     `json:"name"`
	DisplayName    string     `json:"displayName"`
	Description    string     `json:"description"`
	Topic          string     `json:"topic"`
	ChannelType    string     `json:"channelType"`
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ChannelMember struct {
	ID         uuid.UUID `json:"id"`
	ChannelID  uuid.UUID `json:"channelId"`
	MemberType string    `json:"memberType"` // user | agent
	UserID     *uuid.UUID `json:"userId"`
	AgentID    *uuid.UUID `json:"agentId"`
	Role       string    `json:"role"`
	JoinedAt   time.Time `json:"joinedAt"`
}

// ——— Messages

type Message struct {
	ID            uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	ChannelID     uuid.UUID  `json:"channelId"`
	ThreadID      *uuid.UUID `json:"threadId"`
	SenderType    string     `json:"senderType"` // user | agent | system
	SenderUserID  *uuid.UUID `json:"senderUserId"`
	SenderAgentID *uuid.UUID `json:"senderAgentId"`
	Body          string     `json:"body"`
	BodyFormat    string     `json:"bodyFormat"`
	Metadata      map[string]any `json:"metadata"`
	EditedAt      *time.Time `json:"editedAt"`
	DeletedAt     *time.Time `json:"deletedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	Sender        *User              `json:"sender,omitempty"`
	Reactions     []Reaction         `json:"reactions,omitempty"`
	ThreadCount   int                `json:"threadCount,omitempty"`
}

type Reaction struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	MessageID      uuid.UUID `json:"messageId"`
	UserID         uuid.UUID `json:"userId"`
	Emoji          string    `json:"emoji"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ——— Tasks — Phase 4

var ValidTaskStatuses = map[string]bool{
	"pending": true, "assigned": true, "running": true, "waiting": true, "blocked": true, "approval_required": true, "completed": true, "failed": true, "cancelled": true,
	// legacy
	"todo": true, "in_progress": true, "in_review": true, "done": true,
}

type Task struct {
	ID                 uuid.UUID  `json:"id"`
	OrganizationID     uuid.UUID  `json:"organizationId"`
	ProjectID          uuid.UUID  `json:"projectId"`
	ChannelID          *uuid.UUID `json:"channelId"`
	ParentTaskID       *uuid.UUID `json:"parentTaskId"`
	TeamID             *uuid.UUID `json:"teamId"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	CreatedBy          *uuid.UUID `json:"createdBy"`
	AssignedToAgent    *uuid.UUID `json:"assignedToAgent"`
	AssignedToUser     *uuid.UUID `json:"assignedToUser"`
	Status             string     `json:"status"`
	Priority           string     `json:"priority"`
	Deadline           *time.Time `json:"deadline"`
	DueAt              *time.Time `json:"dueAt"` // legacy alias
	CorrelationID      *uuid.UUID `json:"correlationId"`
	RetryCount         int        `json:"retryCount"`
	MaxRetries         int        `json:"maxRetries"`
	DelegationDepth    int        `json:"delegationDepth"`
	MaxDelegationDepth int        `json:"maxDelegationDepth"`
	TimeoutSeconds     int        `json:"timeoutSeconds"`
	RequiredRole       *string    `json:"requiredRole,omitempty"`
	RequiredCapabilities []string `json:"requiredCapabilities,omitempty"`
	PreferredAgentID   *uuid.UUID `json:"preferredAgentId,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	// hydrated
	AssigneeAgent *Agent `json:"assigneeAgent,omitempty"`
	AssigneeUser  *User  `json:"assigneeUser,omitempty"`
}

type TaskAssignment struct {
	ID              uuid.UUID `json:"id"`
	TaskID          uuid.UUID `json:"taskId"`
	AssigneeType    string    `json:"assigneeType"`
	AssigneeUserID  *uuid.UUID `json:"assigneeUserId"`
	AssigneeAgentID *uuid.UUID `json:"assigneeAgentId"`
	AssignedBy      *uuid.UUID `json:"assignedBy"`
	AssignedAt      time.Time `json:"assignedAt"`
}

type TaskEvent struct {
	ID          uuid.UUID `json:"id"`
	TaskID      uuid.UUID `json:"taskId"`
	ActorType   string    `json:"actorType"` // user/agent/system
	ActorUserID *uuid.UUID `json:"actorUserId"`
	ActorAgentID *uuid.UUID `json:"actorAgentId"`
	EventType   string    `json:"eventType"` // created, assigned, started, delegated, tool_called, tool_completed, result_created, approval_requested, completed, failed
	CorrelationID *uuid.UUID `json:"correlationId"`
	Payload     map[string]any `json:"payload"`
	CreatedAt   time.Time  `json:"createdAt"`
}

var ValidTaskEventTypes = map[string]bool{
	"created": true, "assigned": true, "started": true, "delegated": true, "tool_called": true, "tool_completed": true, "result_created": true, "approval_requested": true, "completed": true, "failed": true, "status_changed": true, "commented": true, "cancelled": true,
}

// ——— Knowledge + RAG Phase 3

type KnowledgeSource struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	Name           string    `json:"name"`
	SourceType     string    `json:"sourceType"` // upload/website/text/integration
	Config         map[string]any `json:"config"`
	Status         string    `json:"status"`
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type KnowledgeCollection struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description"`
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Document struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	ProjectID      *uuid.UUID `json:"projectId"`
	CollectionID   *uuid.UUID `json:"collectionId"`
	SourceID       *uuid.UUID `json:"sourceId"`
	Title          string     `json:"title"`
	SourceType     string     `json:"sourceType"`
	SourceURL      *string    `json:"sourceUrl"`
	MimeType       string     `json:"mimeType"`
	SizeBytes      int64      `json:"sizeBytes"`
	Status         string     `json:"status"` // pending/processing/ready/failed
	Scope          string     `json:"scope"`  // organization/project/agent
	Version        int        `json:"version"`
	Metadata       map[string]any `json:"metadata"`
	Content        string     `json:"content,omitempty"` // for text source, not always returned
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	ChunkCount     int        `json:"chunkCount,omitempty"`
}

type DocumentVersion struct {
	ID         uuid.UUID `json:"id"`
	DocumentID uuid.UUID `json:"documentId"`
	Version    int       `json:"version"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Metadata   map[string]any `json:"metadata"`
	CreatedBy  *uuid.UUID `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

type DocumentChunk struct {
	ID             uuid.UUID  `json:"id"`
	DocumentID     uuid.UUID  `json:"documentId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	ChunkIndex     int        `json:"chunkIndex"`
	Content        string     `json:"content"`
	TokenCount     int        `json:"tokenCount"`
	Embedding      []float32  `json:"embedding,omitempty"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time  `json:"createdAt"`
	Score          float64    `json:"score,omitempty"` // for retrieval results
	DocumentTitle  string     `json:"documentTitle,omitempty"`
}

type Embedding struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organizationId"`
	DocumentChunkID uuid.UUID `json:"documentChunkId"`
	Embedding       []float32 `json:"embedding"`
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"createdAt"`
}

type AgentKnowledge struct {
	ID           uuid.UUID  `json:"id"`
	AgentID      uuid.UUID  `json:"agentId"`
	DocumentID   *uuid.UUID `json:"documentId"`
	CollectionID *uuid.UUID `json:"collectionId"`
	GrantedBy    *uuid.UUID `json:"grantedBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	// hydrated
	Document   *Document            `json:"document,omitempty"`
	Collection *KnowledgeCollection `json:"collection,omitempty"`
}

type ProjectKnowledge struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    uuid.UUID  `json:"projectId"`
	DocumentID   *uuid.UUID `json:"documentId"`
	CollectionID *uuid.UUID `json:"collectionId"`
	GrantedBy    *uuid.UUID `json:"grantedBy"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// Retrieval abstraction types

type RetrievalFilter struct {
	OrganizationID uuid.UUID
	AgentID        *uuid.UUID
	ProjectID      *uuid.UUID
	ChannelID      *uuid.UUID
	AllowedDocIDs  []uuid.UUID // explicit allow list from permissions
	Metadata       map[string]string
	Scope          string // organization/project/agent
	Limit          int
}

type RetrievalResult struct {
	Chunk     DocumentChunk `json:"chunk"`
	Score     float64       `json:"score"`
	Document  *Document     `json:"document,omitempty"`
}

type Reranker interface {
	Rerank(query string, results []RetrievalResult) ([]RetrievalResult, error)
}

// KnowledgeRetriever is the clean domain interface LLM providers use (no direct PG access)
type KnowledgeRetriever interface {
	SemanticSearch(ctx context.Context, query string, filter RetrievalFilter) ([]RetrievalResult, error)
	HybridSearch(ctx context.Context, query string, filter RetrievalFilter) ([]RetrievalResult, error)
	ContextualRetrieval(ctx context.Context, query string, agentID, projectID, channelID *uuid.UUID, limit int) ([]RetrievalResult, error)
}

// ——— Agent runs & actions — Phase 4

type AgentRun struct {
	ID                uuid.UUID  `json:"id"`
	OrganizationID    uuid.UUID  `json:"organizationId"`
	AgentID           uuid.UUID  `json:"agentId"`
	TaskID            *uuid.UUID `json:"taskId"`
	ChannelID         *uuid.UUID `json:"channelId"`
	AgentVersion      *int       `json:"agentVersion"`
	TriggerType       string     `json:"triggerType"`
	Status            string     `json:"status"`
	CurrentIteration  int        `json:"currentIteration"`
	WaitingReason     *string    `json:"waitingReason,omitempty"`
	PendingActionID   *uuid.UUID `json:"pendingActionId,omitempty"`
	Input             map[string]any `json:"input"`
	Context           map[string]any `json:"context"`
	RetrievedKnowledge []RetrievalResult `json:"retrievedKnowledge"`
	Output            map[string]any `json:"output"`
	TokenMetadata     map[string]any `json:"tokenMetadata"`
	CorrelationID     *uuid.UUID `json:"correlationId"`
	ParentRunID       *uuid.UUID `json:"parentRunId"`
	Error             *string    `json:"error"`
	StartedAt         *time.Time `json:"startedAt"`
	FinishedAt        *time.Time `json:"finishedAt"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	// hydrated actions
	Actions           []AgentAction `json:"actions,omitempty"`
}

type AgentAction struct {
	ID            uuid.UUID `json:"id"`
	RunID         uuid.UUID `json:"runId"`
	Seq           int       `json:"seq"`
	ActionType    string    `json:"actionType"`
	ToolName      *string   `json:"toolName"`
	Input         map[string]any `json:"input"`
	Output        map[string]any `json:"output"`
	Status        string    `json:"status"`
	CorrelationID *uuid.UUID `json:"correlationId"`
	CreatedAt     time.Time `json:"createdAt"`
}

var ValidAgentRunStatuses = map[string]bool{
	"queued": true, "running": true, "awaiting_approval": true, "succeeded": true, "failed": true, "cancelled": true,
	"pending": true, "assigned": true, "waiting": true, "blocked": true, "approval_required": true, "completed": true,
}

// ——— Tools + Approvals + Integrations — Phase 5

var ValidRiskLevels = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
var ValidApprovalStatuses = map[string]bool{"pending": true, "approved": true, "rejected": true, "expired": true, "cancelled": true}

type Tool struct {
	ID                 uuid.UUID `json:"id"`
	OrganizationID     *uuid.UUID `json:"organizationId"` // null = global
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	InputSchema        map[string]any `json:"inputSchema"`
	OutputSchema       map[string]any `json:"outputSchema"`
	RiskLevel          string    `json:"riskLevel"`
	RequiredPermissions []string `json:"requiredPermissions"`
	ApprovalPolicy     map[string]any `json:"approvalPolicy"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type AgentTool struct {
	ID        uuid.UUID `json:"id"`
	AgentID   uuid.UUID `json:"agentId"`
	ToolID    uuid.UUID `json:"toolId"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	Tool      *Tool     `json:"tool,omitempty"`
}

type ToolExecution struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	AgentID        uuid.UUID `json:"agentId"`
	TaskID         *uuid.UUID `json:"taskId"`
	RunID          *uuid.UUID `json:"runId"`
	ToolID         uuid.UUID `json:"toolId"`
	ToolName       string    `json:"toolName"`
	Input          map[string]any `json:"input"`
	Output         map[string]any `json:"output"`
	Status         string    `json:"status"` // pending/running/succeeded/failed/approval_required/cancelled
	ApprovalID     *uuid.UUID `json:"approvalId"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Error          *string   `json:"error"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Approval struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organizationId"`
	RequesterType    string     `json:"requesterType"` // agent/user/system
	RequesterAgentID *uuid.UUID `json:"requesterAgentId"`
	RequesterUserID  *uuid.UUID `json:"requesterUserId"`
	EntityType       string     `json:"entityType"` // tool_execution, task, document, integration
	EntityID         uuid.UUID  `json:"entityId"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Payload          map[string]any `json:"payload"`
	RiskLevel        string     `json:"riskLevel"`
	Action           string     `json:"action"`
	Target           string     `json:"target"`
	Context          map[string]any `json:"context"`
	Status           string     `json:"status"` // pending/approved/rejected/expired/cancelled
	DecidedBy        *uuid.UUID `json:"decidedBy"`
	DecidedAt        *time.Time `json:"decidedAt"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type Integration struct {
	ID                 uuid.UUID `json:"id"`
	OrganizationID     uuid.UUID `json:"organizationId"`
	Name               string    `json:"name"`
	Provider           string    `json:"provider"`
	Config             map[string]any `json:"config"`
	Credentials        map[string]any `json:"credentials"` // decrypted for response (never raw secrets)
	CredentialsEncrypted *string `json:"-"`
	Status             string    `json:"status"`
	CreatedBy          *uuid.UUID `json:"createdBy"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt"`
}

// ——— Teams — Phase 6

type AgentTeam struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Objective      string    `json:"objective"`
	Description    string    `json:"description"`
	Status         string    `json:"status"` // draft/active/archived
	CreatedBy      *uuid.UUID `json:"createdBy"`
	CoordinatorAgentID *uuid.UUID `json:"coordinatorAgentId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	// hydrated
	Members      []AgentTeamMember `json:"members,omitempty"`
	CurrentVersion *AgentTeamVersion `json:"currentVersion,omitempty"`
	Workflow     *TeamWorkflow `json:"workflow,omitempty"`
}

type AgentTeamVersion struct {
	ID                uuid.UUID `json:"id"`
	TeamID            uuid.UUID `json:"teamId"`
	Version           int       `json:"version"`
	Objective         string    `json:"objective"`
	Workflow          []WorkflowStep `json:"workflow"`
	CommunicationRules []string `json:"communicationRules"`
	DelegationRules   []string `json:"delegationRules"`
	Permissions       map[string]any `json:"permissions"`
	ApprovalPolicy    map[string]any `json:"approvalPolicy"`
	Config            map[string]any `json:"config"`
	CreatedBy         *uuid.UUID `json:"createdBy"`
	CreatedAt         time.Time `json:"createdAt"`
}

type AgentTeamMember struct {
	ID                   uuid.UUID `json:"id"`
	TeamID               uuid.UUID `json:"teamId"`
	AgentID              uuid.UUID `json:"agentId"`
	Role                 string    `json:"role"`
	Responsibilities     string    `json:"responsibilities"`
	Dependencies         []string  `json:"dependencies"` // agent IDs
	Tools                []string  `json:"tools"`
	KnowledgeRequirements []string `json:"knowledgeRequirements"`
	AddedAt              time.Time `json:"addedAt"`
	// hydrated
	Agent *Agent `json:"agent,omitempty"`
}

type TeamWorkflow struct {
	ID        uuid.UUID `json:"id"`
	TeamID    uuid.UUID `json:"teamId"`
	Name      string    `json:"name"`
	Steps     []WorkflowStep `json:"steps"`
	CreatedAt time.Time `json:"createdAt"`
}

type WorkflowStep struct {
	From      string `json:"from"` // agent role or "human"
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
	Action    string `json:"action,omitempty"`
}

type TeamActivity struct {
	ID             uuid.UUID `json:"id"`
	TeamID         uuid.UUID `json:"teamId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	ActorType      string    `json:"actorType"`
	ActorID        *uuid.UUID `json:"actorId"`
	EventType      string    `json:"eventType"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time `json:"createdAt"`
}

type TaskDependency struct {
	ID               uuid.UUID `json:"id"`
	TaskID           uuid.UUID `json:"taskId"`
	DependsOnTaskID  uuid.UUID `json:"dependsOnTaskId"`
	CreatedAt        time.Time `json:"createdAt"`
}

// Team builder preview (not persisted)
type TeamBuilderPreview struct {
	Name           string             `json:"name"`
	Objective      string             `json:"objective"`
	Description    string             `json:"description"`
	Agents         []TeamAgentProposal `json:"agents"`
	Workflow       []WorkflowStep     `json:"workflow"`
	CommunicationRules []string       `json:"communicationRules"`
	DelegationRules []string         `json:"delegationRules"`
	Permissions    map[string]any     `json:"permissions"`
	ApprovalPolicy map[string]any     `json:"approvalPolicy"`
	Warnings       []string           `json:"warnings,omitempty"`
}

type TeamAgentProposal struct {
	Name           string   `json:"name"`
	Role           string   `json:"role"`
	Responsibilities string `json:"responsibilities"`
	Dependencies   []string `json:"dependencies"`
	Tools          []string `json:"tools"`
	Knowledge      []string `json:"knowledge"`
	Autonomy       string   `json:"autonomy"`
	ApprovalPolicy string   `json:"approvalPolicy"`
}

// ——— Memory + Scheduling + Autonomy — Phase 7

var ValidMemoryTypes = map[string]bool{"working": true, "project": true, "agent": true, "conversation": true, "task": true}
var ValidMemoryScopes = map[string]bool{"task": true, "session": true, "project": true, "agent": true, "conversation": true, "organization": true}
var ValidMemoryStatuses = map[string]bool{"candidate": true, "validating": true, "active": true, "merged": true, "superseded": true, "stale": true, "archived": true, "conflict": true}
var ValidMemorySources = map[string]bool{"interaction": true, "document": true, "manual": true, "USER_MESSAGE": true, "AGENT_OBSERVATION": true, "TASK_RESULT": true, "DOCUMENT": true, "TOOL_RESULT": true, "EXPLICIT_USER_PREFERENCE": true, "agent": true, "user_preference": true}
var ValidTriggerTypes = map[string]bool{"schedule": true, "message_received": true, "task_completed": true, "task_failed": true, "new_document": true, "approval_received": true, "project_event": true}
var ValidAgentStatusesPhase7 = map[string]bool{"idle": true, "working": true, "waiting": true, "blocked": true, "approval_required": true, "failed": true, "offline": true}

type Memory struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	AgentID        *uuid.UUID `json:"agentId"`
	ProjectID      *uuid.UUID `json:"projectId"`
	ConversationID *uuid.UUID `json:"conversationId,omitempty"`
	TaskID         *uuid.UUID `json:"taskId,omitempty"`
	MemoryType     string    `json:"memoryType"` // working/project/agent/conversation/task
	Scope          string    `json:"scope"`
	Source         string    `json:"source"` // interaction/document/manual or explicit sources
	SourceEventID  *uuid.UUID `json:"sourceEventId,omitempty"`
	Content        string    `json:"content"`
	Importance     float64   `json:"importance"`
	Confidence     float64   `json:"confidence"`
	Status         string    `json:"status"` // candidate/validating/active/merged/superseded/stale/archived/conflict
	Metadata       map[string]any `json:"metadata"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	LastAccessedAt *time.Time `json:"lastAccessedAt,omitempty"`
	LastConfirmedAt *time.Time `json:"lastConfirmedAt,omitempty"`
	ContentHash    string `json:"contentHash,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	SupersededBy   *uuid.UUID `json:"supersededBy,omitempty"`
	Version        int `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	// legacy
	Kind           string    `json:"kind,omitempty"`
}

type MemoryVersion struct {
	ID         uuid.UUID `json:"id"`
	MemoryID   uuid.UUID `json:"memoryId"`
	Version    int       `json:"version"`
	Content    string    `json:"content"`
	Importance float64   `json:"importance"`
	Confidence float64   `json:"confidence"`
	Status     string    `json:"status"`
	Metadata   map[string]any `json:"metadata"`
	Source     string    `json:"source"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"createdAt"`
}

type MemoryEvent struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	MemoryID       *uuid.UUID `json:"memoryId,omitempty"`
	Scope          string    `json:"scope"`
	EventType      string    `json:"eventType"`
	CorrelationID  *uuid.UUID `json:"correlationId,omitempty"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Trigger struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organizationId"`
	Name            string    `json:"name"`
	TriggerType     string    `json:"triggerType"`
	Config          map[string]any `json:"config"` // cron, timezone, oneTimeAt, recurring
	Enabled         bool      `json:"enabled"`
	AgentID         *uuid.UUID `json:"agentId"`
	TeamID          *uuid.UUID `json:"teamId"`
	ProjectID       *uuid.UUID `json:"projectId"`
	CreatedBy       *uuid.UUID `json:"createdBy"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt"`
	NextRunAt       *time.Time `json:"nextRunAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ScheduledTask struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	TriggerID      uuid.UUID `json:"triggerId"`
	ProjectID      *uuid.UUID `json:"projectId"`
	TeamID         *uuid.UUID `json:"teamId"`
	AgentID        *uuid.UUID `json:"agentId"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	ScheduledAt    time.Time `json:"scheduledAt"`
	Timezone       string    `json:"timezone"`
	Recurrence     *string   `json:"recurrence"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ExecutionBudget struct {
	ID                 uuid.UUID `json:"id"`
	OrganizationID     uuid.UUID `json:"organizationId"`
	AgentID            *uuid.UUID `json:"agentId"`
	TeamID             *uuid.UUID `json:"teamId"`
	MaxTasksPerDay     int       `json:"maxTasksPerDay"`
	MaxToolCallsPerDay int       `json:"maxToolCallsPerDay"`
	MaxDelegationDepth int       `json:"maxDelegationDepth"`
	MaxRetries         int       `json:"maxRetries"`
	TimeoutSeconds     int       `json:"timeoutSeconds"`
	RateLimitPerMinute int       `json:"rateLimitPerMinute"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ——— Browser Profiles & Sessions — Phase: Browser Capability

var ValidBrowserProfileStatuses = map[string]bool{"connected": true, "disconnected": true, "expired": true, "needs_reauth": true, "error": true}
var ValidBrowserSessionStatuses = map[string]bool{"connected": true, "disconnected": true, "expired": true, "needs_reauth": true, "error": true}
var ValidBrowserProviders = map[string]bool{"generic": true, "linkedin": true, "x": true, "instagram": true, "facebook": true, "slack": true, "github": true, "notion": true, "google": true, "custom": true}

type BrowserProfile struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organizationId"`
	OwnerUserID    uuid.UUID `json:"ownerUserId"`
	Provider       string    `json:"provider"`
	Name           string    `json:"name"`
	StorageKey     string    `json:"-"` // never expose filesystem path
	Status         string    `json:"status"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastUsedAt     *time.Time `json:"lastUsedAt,omitempty"`
}

type BrowserSession struct {
	ID               uuid.UUID `json:"id"`
	BrowserProfileID uuid.UUID `json:"browserProfileId"`
	OrganizationID   uuid.UUID `json:"organizationId"`
	OwnerUserID      uuid.UUID `json:"ownerUserId"`
	Status           string    `json:"status"`
	StartedAt        time.Time `json:"startedAt"`
	LastActivityAt   time.Time `json:"lastActivityAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type BrowserAuditLog struct {
	ID               uuid.UUID `json:"id"`
	OrganizationID   uuid.UUID `json:"organizationId"`
	OwnerUserID      *uuid.UUID `json:"ownerUserId"`
	AgentID          *uuid.UUID `json:"agentId"`
	TaskID           *uuid.UUID `json:"taskId"`
	RunID            *uuid.UUID `json:"runId"`
	BrowserProfileID *uuid.UUID `json:"browserProfileId"`
	BrowserSessionID *uuid.UUID `json:"browserSessionId"`
	ToolName         string    `json:"toolName"`
	Action           string    `json:"action"`
	Target           *string   `json:"target"`
	Domain           *string   `json:"domain"`
	ResultStatus     *string   `json:"resultStatus"`
	ApprovalID       *uuid.UUID `json:"approvalId"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time `json:"createdAt"`
}
