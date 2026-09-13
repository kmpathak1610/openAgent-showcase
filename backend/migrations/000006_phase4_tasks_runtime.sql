-- 000006_phase4_tasks_runtime.sql — Task engine + Agent runtime

-- Tasks: add hierarchy + direct assignee fields + deadline + new statuses
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS parent_task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS assigned_to_agent UUID REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS assigned_to_user UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deadline TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS correlation_id UUID;
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_task_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_agent ON tasks(assigned_to_agent);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_user ON tasks(assigned_to_user);
CREATE INDEX IF NOT EXISTS idx_tasks_correlation ON tasks(correlation_id);

-- Update status check: drop old, add new
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_status_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_status_check CHECK (status IN ('pending','assigned','running','waiting','blocked','approval_required','completed','failed','cancelled'));

-- Task events: expand event types + correlation
ALTER TABLE task_events DROP CONSTRAINT IF EXISTS task_events_event_type_check;
ALTER TABLE task_events ADD CONSTRAINT task_events_event_type_check CHECK (event_type IN ('created','assigned','started','delegated','tool_called','tool_completed','result_created','approval_requested','completed','failed','status_changed','commented','cancelled'));
ALTER TABLE task_events ADD COLUMN IF NOT EXISTS correlation_id UUID;
CREATE INDEX IF NOT EXISTS idx_task_events_correlation ON task_events(correlation_id);

-- Agent runs: add version, context, retrieved knowledge, token metadata, correlation, parent run
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS agent_version INT;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS context JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS retrieved_knowledge JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS token_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS correlation_id UUID;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS parent_run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_agent_runs_correlation ON agent_runs(correlation_id);
CREATE INDEX IF NOT EXISTS idx_agent_runs_parent ON agent_runs(parent_run_id);

-- Update agent_runs status to include Phase 4 statuses (keep existing but add pending etc for uniformity)
ALTER TABLE agent_runs DROP CONSTRAINT IF EXISTS agent_runs_status_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_status_check CHECK (status IN ('queued','running','awaiting_approval','succeeded','failed','cancelled','pending','assigned','waiting','blocked','approval_required','completed'));

-- Agent actions: add correlation
ALTER TABLE agent_actions ADD COLUMN IF NOT EXISTS correlation_id UUID;
CREATE INDEX IF NOT EXISTS idx_agent_actions_correlation ON agent_actions(correlation_id);
