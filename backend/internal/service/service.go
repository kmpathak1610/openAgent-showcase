package service

import "openagent/internal/repository"

// Services orchestrate business logic; depend on repository.DB + llm + worker.
// For Phase 1 the handlers call repository directly; services will be expanded for agent orchestration in next phase.

type OrganizationService struct {
	repo *repository.DB
}

func NewOrganizationService(r *repository.DB) *OrganizationService {
	return &OrganizationService{repo: r}
}

type AgentService struct {
	repo *repository.DB
}

func NewAgentService(r *repository.DB) *AgentService {
	return &AgentService{repo: r}
}

type ProjectService struct{ repo *repository.DB }
func NewProjectService(r *repository.DB) *ProjectService { return &ProjectService{repo:r} }

type ChannelService struct{ repo *repository.DB }
func NewChannelService(r *repository.DB) *ChannelService { return &ChannelService{repo:r} }

type MessageService struct{ repo *repository.DB }
func NewMessageService(r *repository.DB) *MessageService { return &MessageService{repo:r} }
