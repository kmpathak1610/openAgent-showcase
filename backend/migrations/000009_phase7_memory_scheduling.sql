-- 000009_phase7_memory_scheduling.sql — Memory + Autonomous + Triggers + Scheduling

-- Extend memories to Phase 7 spec (add missing columns, keep backward compat with kind/source_type)
ALTER TABLE memories ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT 'agent' CHECK (memory_type IN ('working','project','agent','conversation'));
ALTER TABLE memories ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'agent' CHECK (scope IN ('task','session','project','agent','conversation','organization'));
ALTER TABLE memories ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'interaction';
ALTER TABLE memories ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE CASCADE;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
-- Backfill
UPDATE memories SET memory_type = CASE WHEN kind='fact' THEN 'agent' WHEN kind='preference' THEN 'agent' WHEN kind='summary' THEN 'conversation' WHEN kind='episode' THEN 'working' ELSE 'agent' END WHERE memory_type='agent' AND kind IS NOT NULL;
UPDATE memories SET scope = memory_type WHERE scope='agent' AND memory_type IS NOT NULL;
UPDATE memories SET source = source_type WHERE source='interaction' AND source_type IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project_id);
CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(memory_type);
CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at) WHERE expires_at IS NOT NULL;

-- Triggers abstraction (schedule, message_received, task_completed, task_failed, new_document, approval_received, project_event)
CREATE TABLE IF NOT EXISTS triggers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    trigger_type TEXT NOT NULL CHECK (trigger_type IN ('schedule','message_received','task_completed','task_failed','new_document','approval_received','project_event')),
    config JSONB NOT NULL DEFAULT '{}'::jsonb, -- for schedule: {cron, timezone, oneTimeAt, recurring}
    enabled BOOLEAN NOT NULL DEFAULT true,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL, -- null = team/org trigger
    team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    last_triggered_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_triggers_org ON triggers(organization_id);
CREATE INDEX IF NOT EXISTS idx_triggers_next_run ON triggers(next_run_at) WHERE enabled=true AND trigger_type='schedule';
CREATE INDEX IF NOT EXISTS idx_triggers_agent ON triggers(agent_id);
DROP TRIGGER IF EXISTS trg_triggers_updated_at ON triggers;
CREATE TRIGGER trg_triggers_updated_at BEFORE UPDATE ON triggers FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Scheduled task executions (one-time or recurring instances)
CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    trigger_id UUID NOT NULL REFERENCES triggers(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','running','completed','failed','cancelled')),
    scheduled_at TIMESTAMPTZ NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    recurrence TEXT, -- cron or 'once' or 'weekly:mon@09:00'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_trigger ON scheduled_tasks(trigger_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_scheduled ON scheduled_tasks(scheduled_at) WHERE status='scheduled';
DROP TRIGGER IF EXISTS trg_scheduled_tasks_updated_at ON scheduled_tasks;
CREATE TRIGGER trg_scheduled_tasks_updated_at BEFORE UPDATE ON scheduled_tasks FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Execution budgets / guardrails (per agent or team per day)
CREATE TABLE IF NOT EXISTS execution_budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    team_id UUID REFERENCES agent_teams(id) ON DELETE CASCADE,
    max_tasks_per_day INT NOT NULL DEFAULT 100,
    max_tool_calls_per_day INT NOT NULL DEFAULT 500,
    max_delegation_depth INT NOT NULL DEFAULT 5,
    max_retries INT NOT NULL DEFAULT 3,
    timeout_seconds INT NOT NULL DEFAULT 300,
    rate_limit_per_minute INT NOT NULL DEFAULT 60,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, agent_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_budgets_org ON execution_budgets(organization_id);

-- Agent status view helper (materialized via queries, but we store last status for quick lookup)
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT 'offline' CHECK (last_status IN ('idle','working','waiting','blocked','approval_required','failed','offline'));
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_status_at TIMESTAMPTZ;

-- Ensure memories updated_at trigger
DROP TRIGGER IF EXISTS trg_memories_updated_at ON memories;
CREATE TRIGGER trg_memories_updated_at BEFORE UPDATE ON memories FOR EACH ROW EXECUTE FUNCTION update_updated_at();
