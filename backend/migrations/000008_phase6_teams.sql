-- 000008_phase6_teams.sql — AI Team Builder + Orchestration

-- Agent Teams
CREATE TABLE IF NOT EXISTS agent_teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    objective TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','archived')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_teams_org ON agent_teams(organization_id);

-- Team Versions (immutable)
CREATE TABLE IF NOT EXISTS agent_team_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    version INT NOT NULL CHECK (version >0),
    objective TEXT NOT NULL DEFAULT '',
    workflow JSONB NOT NULL DEFAULT '[]'::jsonb,
    communication_rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    delegation_rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (team_id, version)
);

-- Team Members (agents with roles)
CREATE TABLE IF NOT EXISTS agent_team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    responsibilities TEXT NOT NULL DEFAULT '',
    dependencies TEXT NOT NULL DEFAULT '[]', -- JSON array of agent IDs this member depends on
    tools JSONB NOT NULL DEFAULT '[]'::jsonb,
    knowledge_requirements TEXT NOT NULL DEFAULT '[]',
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (team_id, agent_id)
);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON agent_team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_agent ON agent_team_members(agent_id);

-- Team Workflow steps (optional structured workflow)
CREATE TABLE IF NOT EXISTS team_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    steps JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{from, to, condition}]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Project ↔ Team assignment
CREATE TABLE IF NOT EXISTS project_teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, team_id)
);

-- Task dependencies (blocking)
CREATE TABLE IF NOT EXISTS task_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, depends_on_task_id),
    CHECK (task_id != depends_on_task_id)
);
CREATE INDEX IF NOT EXISTS idx_task_deps_task ON task_dependencies(task_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_depends ON task_dependencies(depends_on_task_id);

-- Extend tasks with execution limits
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS max_retries INT NOT NULL DEFAULT 3;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS delegation_depth INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS max_delegation_depth INT NOT NULL DEFAULT 5;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS timeout_seconds INT NOT NULL DEFAULT 300;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_team ON tasks(team_id);

-- Team activity log for dashboard
CREATE TABLE IF NOT EXISTS team_activity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user','agent','system')),
    actor_id UUID,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_team_activity_team ON team_activity(team_id, created_at DESC);

-- Triggers
DROP TRIGGER IF EXISTS trg_teams_updated_at ON agent_teams;
CREATE TRIGGER trg_teams_updated_at BEFORE UPDATE ON agent_teams FOR EACH ROW EXECUTE FUNCTION update_updated_at();
