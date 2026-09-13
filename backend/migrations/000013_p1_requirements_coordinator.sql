-- 000013_p1_requirements_coordinator.sql — P1-6 Task requirements + P1-7 Team coordinator
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS required_role TEXT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS required_capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS preferred_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_required_role ON tasks(required_role) WHERE required_role IS NOT NULL;

-- Team coordinator (P1-7)
ALTER TABLE agent_teams ADD COLUMN IF NOT EXISTS coordinator_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_teams_coordinator ON agent_teams(coordinator_agent_id) WHERE coordinator_agent_id IS NOT NULL;

-- Ensure coordinator belongs to team (enforced at application layer, not DB constraint to avoid complexity)
