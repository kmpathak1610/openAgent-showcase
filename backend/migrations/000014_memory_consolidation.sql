-- 000014_memory_consolidation.sql — Production Memory Consolidation
-- Implements lifecycle, confidence, scope safety, versioning, events

-- 1) Extend memories table with consolidation fields (keep backward compat)
ALTER TABLE memories ADD COLUMN IF NOT EXISTS conversation_id UUID; -- generic conversation/channel/thread reference
ALTER TABLE memories ADD COLUMN IF NOT EXISTS task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS confidence REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1);
ALTER TABLE memories ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('candidate','validating','active','merged','superseded','stale','archived','conflict'));
ALTER TABLE memories ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS last_confirmed_at TIMESTAMPTZ;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS source_event_id UUID;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS content_hash TEXT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS superseded_by UUID REFERENCES memories(id) ON DELETE SET NULL;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

-- Allow agent_id to be nullable for organization/project scope memories (previously NOT NULL)
ALTER TABLE memories ALTER COLUMN agent_id DROP NOT NULL;

-- Expand memory_type to include 'task' (spec types: WORKING, CONVERSATION, TASK, PROJECT, AGENT)
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_memory_type_check;
ALTER TABLE memories ADD CONSTRAINT memories_memory_type_check CHECK (memory_type IN ('working','project','agent','conversation','task'));

-- Add updated check for scope (already includes needed scopes, add 'organization' if not already? it's already present in 000009)
-- Ensure scope check allows all needed
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_scope_check;
ALTER TABLE memories ADD CONSTRAINT memories_scope_check CHECK (scope IN ('task','session','project','agent','conversation','organization'));

-- 2) Backfill existing memories for new fields
UPDATE memories SET confidence = COALESCE(confidence, LEAST(1.0, GREATEST(0.0, importance))) WHERE confidence IS NULL OR confidence = 0.5;
UPDATE memories SET status = 'active' WHERE status IS NULL OR status = '';
UPDATE memories SET last_confirmed_at = COALESCE(last_confirmed_at, updated_at, created_at) WHERE last_confirmed_at IS NULL;
UPDATE memories SET last_accessed_at = COALESCE(last_accessed_at, updated_at) WHERE last_accessed_at IS NULL;
UPDATE memories SET content_hash = encode(digest(content, 'sha256'), 'hex') WHERE content_hash IS NULL;
-- Generate idempotency_key for existing (org + scope + hash)
UPDATE memories SET idempotency_key = organization_id::text || ':' || scope || ':' || COALESCE(agent_id::text,'') || ':' || COALESCE(project_id::text,'') || ':' || COALESCE(content_hash,'') WHERE idempotency_key IS NULL;

-- 3) Indexes for consolidation performance (bounded candidate retrieval)
CREATE INDEX IF NOT EXISTS idx_memories_status ON memories(status) WHERE status IN ('active','candidate','validating','conflict');
CREATE INDEX IF NOT EXISTS idx_memories_org_scope ON memories(organization_id, scope);
CREATE INDEX IF NOT EXISTS idx_memories_org_project ON memories(organization_id, project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_org_agent ON memories(organization_id, agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_confidence ON memories(confidence DESC);
CREATE INDEX IF NOT EXISTS idx_memories_last_confirmed ON memories(last_confirmed_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_content_hash ON memories(content_hash);
CREATE INDEX IF NOT EXISTS idx_memories_conversation ON memories(conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_task ON memories(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_expires_stale ON memories(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memories_superseded ON memories(superseded_by) WHERE superseded_by IS NOT NULL;

-- Unique idempotency for exact duplicates within same scope (prevents uncontrolled duplicates)
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_idempotency ON memories(organization_id, scope, COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'::uuid), COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid), content_hash) WHERE content_hash IS NOT NULL AND status IN ('active','candidate','validating');

-- Ensure HNSW index exists for vector search (already in 000001, but ensure)
CREATE INDEX IF NOT EXISTS idx_memories_embedding_hnsw ON memories USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);

-- Also need pgcrypto digest for content_hash generation if not exists
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 4) Memory versions / history for audit/rollback
CREATE TABLE IF NOT EXISTS memory_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    version INT NOT NULL,
    content TEXT NOT NULL,
    importance REAL NOT NULL,
    confidence REAL NOT NULL,
    status TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    source TEXT NOT NULL DEFAULT 'interaction',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (memory_id, version)
);
CREATE INDEX IF NOT EXISTS idx_memory_versions_memory ON memory_versions(memory_id);
CREATE INDEX IF NOT EXISTS idx_memory_versions_created ON memory_versions(created_at DESC);

-- 5) Memory events for lifecycle observability (supplements audit_logs, not replacement)
CREATE TABLE IF NOT EXISTS memory_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    memory_id UUID REFERENCES memories(id) ON DELETE SET NULL,
    scope TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('memory.created','memory.merged','memory.updated','memory.conflict','memory.archived','memory.reconfirmed','memory.stale','memory.candidate','memory.validating','memory.rejected')),
    correlation_id UUID,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_memory_events_org ON memory_events(organization_id);
CREATE INDEX IF NOT EXISTS idx_memory_events_memory ON memory_events(memory_id) WHERE memory_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_memory_events_type ON memory_events(event_type);
CREATE INDEX IF NOT EXISTS idx_memory_events_created ON memory_events(created_at DESC);

-- 6) Helper function for staleness detection (optional, for scheduler)
CREATE OR REPLACE FUNCTION memory_is_stale(m memories) RETURNS BOOLEAN AS $$
BEGIN
    IF m.expires_at IS NOT NULL AND m.expires_at < now() THEN RETURN TRUE; END IF;
    IF m.status IN ('archived','superseded','merged','stale') THEN RETURN TRUE; END IF;
    -- Working memory: stale after 1 hour if not confirmed
    IF m.memory_type = 'working' AND m.last_confirmed_at < now() - interval '1 hour' THEN RETURN TRUE; END IF;
    -- Conversation: stale after 7 days if not accessed
    IF m.memory_type = 'conversation' AND COALESCE(m.last_accessed_at, m.last_confirmed_at) < now() - interval '7 days' THEN RETURN TRUE; END IF;
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Ensure updated_at trigger still active (already exists, but recreate to include new cols)
DROP TRIGGER IF EXISTS trg_memories_updated_at ON memories;
CREATE TRIGGER trg_memories_updated_at BEFORE UPDATE ON memories FOR EACH ROW EXECUTE FUNCTION update_updated_at();
