-- 000010_distributed.sql — Durable queue, distributed locks, evaluation

-- Durable job queue for worker (alternative to in-memory channel) — for multi-instance
CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('agent_run','ingest_document','scheduled_task','evaluation')),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','claimed','running','succeeded','failed','dead')),
    attempts INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    idempotency_key TEXT UNIQUE,
    claimed_by TEXT,
    claimed_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, next_retry_at) WHERE status IN ('pending','claimed');
CREATE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_jobs_org ON jobs(organization_id);

-- Evaluation results cache (optional, for quick dashboard)
CREATE TABLE IF NOT EXISTS evaluations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    success BOOLEAN NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    checks JSONB NOT NULL DEFAULT '[]'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id)
);
CREATE INDEX IF NOT EXISTS idx_evaluations_task ON evaluations(task_id);
CREATE INDEX IF NOT EXISTS idx_evaluations_org ON evaluations(organization_id);

-- Distributed lock helper via pg_advisory_lock is available natively; no table needed
-- Add index for triggers next_run_at for SKIP LOCKED claim performance
CREATE INDEX IF NOT EXISTS idx_triggers_claim ON triggers(next_run_at, enabled) WHERE trigger_type='schedule' AND enabled=true;

-- Ensure tool_executions idempotency_key is unique per org (already, but ensure)
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_exec_org_idem ON tool_executions(organization_id, idempotency_key) WHERE idempotency_key <> '';

-- Validate agent_run status transitions via check constraint supplement (application-level enforced, but add DB index for status lookup)
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_task_id) WHERE parent_task_id IS NOT NULL;

DROP TRIGGER IF EXISTS trg_jobs_updated_at ON jobs;
CREATE TRIGGER trg_jobs_updated_at BEFORE UPDATE ON jobs FOR EACH ROW EXECUTE FUNCTION update_updated_at();
DROP TRIGGER IF EXISTS trg_evaluations_updated_at ON evaluations;
-- evaluations has no updated_at, but ensure if needed
