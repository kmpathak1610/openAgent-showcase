-- 000003_phase2_agents.sql — Agent Builder structured model

-- Extend agents with Phase 2 fields (keep existing columns for backward compat)
ALTER TABLE agents ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS avatar TEXT; -- alias for avatar_url generic
ALTER TABLE agents ADD COLUMN IF NOT EXISTS autonomy_level TEXT NOT NULL DEFAULT 'assistant' CHECK (autonomy_level IN ('assistant','task_executor','collaborative','autonomous'));
ALTER TABLE agents ADD COLUMN IF NOT EXISTS current_version INT NOT NULL DEFAULT 1 CHECK (current_version > 0);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- Backfill owner_id from created_by where null
UPDATE agents SET owner_id = created_by WHERE owner_id IS NULL AND created_by IS NOT NULL;

-- Extend agent_versions with structured fields
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS objective TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS instructions TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS behavioral_rules JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS tool_policy JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS memory_policy JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS model_configuration JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Agent capabilities (normalized, current state per agent)
CREATE TABLE IF NOT EXISTS agent_capabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (name ~ '^[a-z0-9_]+$'),
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agent_capabilities_agent ON agent_capabilities(agent_id);

-- Agent permissions (resource-scoped)
CREATE TABLE IF NOT EXISTS agent_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL CHECK (resource_type IN ('organization','project','channel','tool','knowledge','document')),
    resource_id UUID, -- null means wildcard for type
    permission TEXT NOT NULL CHECK (permission IN ('read','write','execute','admin')),
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, resource_type, resource_id, permission)
);
CREATE INDEX IF NOT EXISTS idx_agent_permissions_agent ON agent_permissions(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_permissions_resource ON agent_permissions(resource_type, resource_id);

-- Ensure updated_at trigger for new column still works (already exists)
