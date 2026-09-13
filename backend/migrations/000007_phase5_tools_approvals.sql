-- 000007_phase5_tools_approvals.sql — Tool system + Approval engine + Integrations

-- Tools catalog (generic, org-scoped or global)
CREATE TABLE IF NOT EXISTS tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE, -- null = global built-in
    name TEXT NOT NULL CHECK (name ~ '^[a-z0-9_]+$'),
    description TEXT NOT NULL DEFAULT '',
    input_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    risk_level TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low','medium','high','critical')),
    required_permissions JSONB NOT NULL DEFAULT '[]'::jsonb, -- e.g., ["tool:publish_social_post"]
    approval_policy JSONB NOT NULL DEFAULT '{"require_approval": false}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
CREATE INDEX IF NOT EXISTS idx_tools_org ON tools(organization_id);
CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);

-- Seed built-in tools (global, organization_id NULL)
INSERT INTO tools (organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled) VALUES
(NULL, 'web_search', 'Search the web for information', '{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}', '{"type":"object","properties":{"results":{"type":"array"}}}', 'low', '[]', '{"require_approval": false}', true),
(NULL, 'http_request', 'Make an HTTP request to an external API', '{"type":"object","required":["url"],"properties":{"url":{"type":"string"},"method":{"type":"string"},"body":{"type":"object"}}}', '{"type":"object","properties":{"status":{"type":"integer"},"body":{"type":"object"}}}', 'high', '["tool:http_request"]', '{"require_approval": true}', true),
(NULL, 'send_email', 'Send an email via configured provider', '{"type":"object","required":["to","subject","body"],"properties":{"to":{"type":"string"},"subject":{"type":"string"},"body":{"type":"string"}}}', '{"type":"object","properties":{"messageId":{"type":"string"}}}', 'critical', '["tool:send_email"]', '{"require_approval": true}', true),
(NULL, 'create_calendar_event', 'Create a calendar event', '{"type":"object","required":["title","start"],"properties":{"title":{"type":"string"},"start":{"type":"string"},"end":{"type":"string"},"attendees":{"type":"array"}}}', '{"type":"object","properties":{"eventId":{"type":"string"}}}', 'medium', '["tool:create_calendar_event"]', '{"require_approval": false}', true),
(NULL, 'publish_social_post', 'Publish a post to social media', '{"type":"object","required":["platform","content"],"properties":{"platform":{"type":"string","enum":["linkedin","x","instagram","facebook"]},"content":{"type":"string"},"media":{"type":"array"}}}', '{"type":"object","properties":{"postId":{"type":"string"},"url":{"type":"string"}}}', 'high', '["tool:publish_social_post"]', '{"require_approval": true}', true),
(NULL, 'read_social_analytics', 'Read analytics for social posts', '{"type":"object","required":["platform"],"properties":{"platform":{"type":"string"},"period":{"type":"string"}}}', '{"type":"object","properties":{"metrics":{"type":"object"}}}', 'low', '["tool:read_social_analytics"]', '{"require_approval": false}', true),
(NULL, 'upload_file', 'Upload a file to storage', '{"type":"object","required":["filename","content"],"properties":{"filename":{"type":"string"},"content":{"type":"string"}}}', '{"type":"object","properties":{"fileId":{"type":"string"}}}', 'medium', '["tool:upload_file"]', '{"require_approval": false}', true),
(NULL, 'create_task', 'Create a task in a project', '{"type":"object","required":["projectId","title"],"properties":{"projectId":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"assignedToAgent":{"type":"string"}}}', '{"type":"object","properties":{"taskId":{"type":"string"}}}', 'low', '["tool:create_task"]', '{"require_approval": false}', true),
(NULL, 'send_channel_message', 'Send a message to a channel', '{"type":"object","required":["channelId","body"],"properties":{"channelId":{"type":"string"},"body":{"type":"string"}}}', '{"type":"object","properties":{"messageId":{"type":"string"}}}', 'low', '["tool:send_channel_message"]', '{"require_approval": false}', true)
ON CONFLICT DO NOTHING;

-- Agent ↔ Tool assignment (which tools an agent may use)
CREATE TABLE IF NOT EXISTS agent_tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_id UUID NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, tool_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_tools_agent ON agent_tools(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_tools_tool ON agent_tools(tool_id);

-- Tool executions (audit trail, idempotent)
CREATE TABLE IF NOT EXISTS tool_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    tool_id UUID NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tool_name TEXT NOT NULL,
    input JSONB NOT NULL DEFAULT '{}'::jsonb,
    output JSONB,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed','approval_required','cancelled')),
    approval_id UUID REFERENCES approvals(id) ON DELETE SET NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tool_exec_org ON tool_executions(organization_id);
CREATE INDEX IF NOT EXISTS idx_tool_exec_agent ON tool_executions(agent_id);
CREATE INDEX IF NOT EXISTS idx_tool_exec_task ON tool_executions(task_id);
CREATE INDEX IF NOT EXISTS idx_tool_exec_run ON tool_executions(run_id);
CREATE INDEX IF NOT EXISTS idx_tool_exec_idem ON tool_executions(idempotency_key);

-- Extend approvals to support tool executions and new statuses
ALTER TABLE approvals DROP CONSTRAINT IF EXISTS approvals_status_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_status_check CHECK (status IN ('pending','approved','rejected','expired','cancelled'));

ALTER TABLE approvals ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low','medium','high','critical'));
ALTER TABLE approvals ADD COLUMN IF NOT EXISTS action TEXT NOT NULL DEFAULT '';
ALTER TABLE approvals ADD COLUMN IF NOT EXISTS target TEXT NOT NULL DEFAULT '';
ALTER TABLE approvals ADD COLUMN IF NOT EXISTS context JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE approvals ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE approvals DROP CONSTRAINT IF EXISTS approvals_entity_type_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_entity_type_check CHECK (entity_type IN ('agent_action','task','document','integration','tool_execution'));

-- Expand integrations provider list to support Phase 5 targets
ALTER TABLE integrations DROP CONSTRAINT IF EXISTS integrations_provider_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_provider_check CHECK (provider IN ('slack','github','notion','google_drive','custom','linkedin','x','instagram','facebook','email','calendar','http_api'));

-- Add encrypted credential marker
ALTER TABLE integrations ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT;
ALTER TABLE integrations ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ;

-- Trigger for tools updated_at
DROP TRIGGER IF EXISTS trg_tools_updated_at ON tools;
CREATE TRIGGER trg_tools_updated_at BEFORE UPDATE ON tools FOR EACH ROW EXECUTE FUNCTION update_updated_at();
DROP TRIGGER IF EXISTS trg_tool_exec_updated_at ON tool_executions;
CREATE TRIGGER trg_tool_exec_updated_at BEFORE UPDATE ON tool_executions FOR EACH ROW EXECUTE FUNCTION update_updated_at();
