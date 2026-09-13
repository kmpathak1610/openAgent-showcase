-- 000015_browser_profiles_sessions.sql — Browser capability foundation

-- Browser profiles: persistent identity per org/user/provider
CREATE TABLE IF NOT EXISTS browser_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT 'generic' CHECK (provider IN ('generic','linkedin','x','instagram','facebook','slack','github','notion','google','custom')),
    name TEXT NOT NULL,
    storage_key TEXT NOT NULL, -- internal key, never filesystem path; e.g. hash(org/user/provider/name)
    status TEXT NOT NULL DEFAULT 'disconnected' CHECK (status IN ('connected','disconnected','expired','needs_reauth','error')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    UNIQUE (organization_id, owner_user_id, provider, name)
);
CREATE INDEX IF NOT EXISTS idx_browser_profiles_org ON browser_profiles(organization_id);
CREATE INDEX IF NOT EXISTS idx_browser_profiles_owner ON browser_profiles(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_browser_profiles_provider ON browser_profiles(provider);
DROP TRIGGER IF EXISTS trg_browser_profiles_updated_at ON browser_profiles;
CREATE TRIGGER trg_browser_profiles_updated_at BEFORE UPDATE ON browser_profiles FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Browser sessions: ephemeral, scoped to org+user+profile
CREATE TABLE IF NOT EXISTS browser_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    browser_profile_id UUID NOT NULL REFERENCES browser_profiles(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'connected' CHECK (status IN ('connected','disconnected','expired','needs_reauth','error')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '30 minutes'),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_profile ON browser_sessions(browser_profile_id);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_org ON browser_sessions(organization_id);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_owner ON browser_sessions(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_expires ON browser_sessions(expires_at) WHERE status='connected';
DROP TRIGGER IF EXISTS trg_browser_sessions_updated_at ON browser_sessions;
CREATE TRIGGER trg_browser_sessions_updated_at BEFORE UPDATE ON browser_sessions FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Browser audit trail (extends audit_logs but browser-specific)
CREATE TABLE IF NOT EXISTS browser_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    browser_profile_id UUID REFERENCES browser_profiles(id) ON DELETE SET NULL,
    browser_session_id UUID REFERENCES browser_sessions(id) ON DELETE SET NULL,
    tool_name TEXT NOT NULL,
    action TEXT NOT NULL,
    target TEXT,
    domain TEXT,
    result_status TEXT,
    approval_id UUID REFERENCES approvals(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_browser_audit_org ON browser_audit_logs(organization_id);
CREATE INDEX IF NOT EXISTS idx_browser_audit_profile ON browser_audit_logs(browser_profile_id);
CREATE INDEX IF NOT EXISTS idx_browser_audit_session ON browser_audit_logs(browser_session_id);
CREATE INDEX IF NOT EXISTS idx_browser_audit_created ON browser_audit_logs(created_at DESC);

-- Extend tools for browser if not already seeded (seeded in migration 000007, now add browser.*)
-- 000007 constrained tools.name to '^[a-z0-9_]+$' which rejects dotted names like 'browser.search'.
-- Codebase uses dotted browser.* names consistently (browser/tools.go, tool/executor.go), so widen constraint.
ALTER TABLE tools DROP CONSTRAINT IF EXISTS tools_name_check;
ALTER TABLE tools ADD CONSTRAINT tools_name_check CHECK (name ~ '^[a-z0-9_.]+$');
INSERT INTO tools (organization_id, name, description, input_schema, output_schema, risk_level, required_permissions, approval_policy, enabled) VALUES
(NULL, 'browser.search', 'Search the web using browser (legitimate web research)', '{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":10}}}', '{"type":"object","properties":{"results":{"type":"array"},"query":{"type":"string"}}}', 'low', '["tool:browser.search"]', '{"require_approval": false}', true),
(NULL, 'browser.navigate', 'Navigate browser to a URL (allowlisted http/https only)', '{"type":"object","required":["url"],"properties":{"url":{"type":"string"},"profileId":{"type":"string"}}}', '{"type":"object","properties":{"url":{"type":"string"},"title":{"type":"string"}}}', 'low', '["tool:browser.navigate"]', '{"require_approval": false}', true),
(NULL, 'browser.extract', 'Extract normalized content from current page', '{"type":"object","required":[],"properties":{"selector":{"type":"string"},"maxChars":{"type":"integer"}}}', '{"type":"object","properties":{"content":{"type":"string"},"url":{"type":"string"}}}', 'low', '["tool:browser.extract"]', '{"require_approval": false}', true),
(NULL, 'browser.click', 'Click an element by selector', '{"type":"object","required":["selector"],"properties":{"selector":{"type":"string"}}}', '{"type":"object","properties":{"clicked":{"type":"boolean"}}}', 'medium', '["tool:browser.click"]', '{"require_approval": false}', true),
(NULL, 'browser.type', 'Type text into an element', '{"type":"object","required":["selector","text"],"properties":{"selector":{"type":"string"},"text":{"type":"string"}}}', '{"type":"object","properties":{"typed":{"type":"boolean"}}}', 'medium', '["tool:browser.type"]', '{"require_approval": false}', true),
(NULL, 'browser.select', 'Select an option', '{"type":"object","required":["selector","value"],"properties":{"selector":{"type":"string"},"value":{"type":"string"}}}', '{"type":"object","properties":{"selected":{"type":"boolean"}}}', 'medium', '["tool:browser.select"]', '{"require_approval": false}', true),
(NULL, 'browser.wait', 'Wait for selector or timeout', '{"type":"object","required":[],"properties":{"selector":{"type":"string"},"timeoutMs":{"type":"integer"}}}', '{"type":"object","properties":{"waited":{"type":"boolean"}}}', 'low', '["tool:browser.wait"]', '{"require_approval": false}', true),
(NULL, 'browser.back', 'Navigate back', '{"type":"object","required":[],"properties":{}}', '{"type":"object","properties":{"url":{"type":"string"}}}', 'low', '["tool:browser.back"]', '{"require_approval": false}', true),
(NULL, 'browser.screenshot', 'Capture screenshot (base64, truncated for LLM)', '{"type":"object","required":[],"properties":{"fullPage":{"type":"boolean"}}}', '{"type":"object","properties":{"screenshot":{"type":"string"},"url":{"type":"string"}}}', 'low', '["tool:browser.screenshot"]', '{"require_approval": false}', true),
(NULL, 'browser.download', 'Download a file (size-limited)', '{"type":"object","required":["url"],"properties":{"url":{"type":"string"}}}', '{"type":"object","properties":{"fileId":{"type":"string"},"size":{"type":"integer"}}}', 'medium', '["tool:browser.download"]', '{"require_approval": false}', true),
(NULL, 'browser.new_tab', 'Open a new tab', '{"type":"object","required":[],"properties":{"url":{"type":"string"}}}', '{"type":"object","properties":{"tabId":{"type":"string"}}}', 'low', '["tool:browser.new_tab"]', '{"require_approval": false}', true)
ON CONFLICT (organization_id, name) DO NOTHING;
