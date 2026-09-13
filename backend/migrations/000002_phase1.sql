-- 000002_phase1.sql — Phase 1 workspace enhancements

-- Projects: objective column (Phase 1 spec)
ALTER TABLE projects ADD COLUMN IF NOT EXISTS objective TEXT NOT NULL DEFAULT '';

-- Reactions (cleanly supported via JSONB if full table not needed, but explicit table is cleaner)
CREATE TABLE IF NOT EXISTS reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji TEXT NOT NULL CHECK (emoji ~ '^.{1,16}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (message_id, user_id, emoji)
);
CREATE INDEX IF NOT EXISTS idx_reactions_message ON reactions (message_id);
CREATE INDEX IF NOT EXISTS idx_reactions_org ON reactions (organization_id);

-- Message mentions (derived at app layer but index helps)
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_messages_body_trgm ON messages USING gin (body gin_trgm_ops);

-- Ensure updated_at trigger for reactions not needed (immutable)

-- For presence: we keep in-memory via WS hub; no DB table needed for Phase 1.
