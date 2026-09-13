-- 000011_p0_approval_continuation.sql — Proper approval → same-run continuation (P0-1)
-- Add durable continuation state to agent_runs

ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS current_iteration INT NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS waiting_reason TEXT;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS pending_action_id UUID REFERENCES agent_actions(id) ON DELETE SET NULL;
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS correlation_id_orig UUID; -- keep original correlation
-- Ensure agent_runs has correlation_id column already (from 000006), no need to add

-- Expand status to include explicit P0 states (keep existing for backwards compat, add new)
ALTER TABLE agent_runs DROP CONSTRAINT IF EXISTS agent_runs_status_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_status_check CHECK (status IN (
  'queued','running','awaiting_approval','succeeded','failed','cancelled',
  'pending','assigned','waiting','blocked','approval_required','completed',
  'WAITING_FOR_APPROVAL','RESUMING','RUNNING','COMPLETED','FAILED'
));

-- Approvals already has PENDING/APPROVED/REJECTED/EXPIRED/CANCELLED via 000007, ensure
ALTER TABLE approvals DROP CONSTRAINT IF EXISTS approvals_status_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_status_check CHECK (status IN ('pending','approved','rejected','expired','cancelled'));

-- Ensure agent_actions can store approval correlation
ALTER TABLE agent_actions ADD COLUMN IF NOT EXISTS correlation_id UUID;
CREATE INDEX IF NOT EXISTS idx_agent_actions_correlation ON agent_actions(correlation_id);

-- Index for resumption lookup
CREATE INDEX IF NOT EXISTS idx_agent_runs_waiting ON agent_runs(status) WHERE status IN ('awaiting_approval','WAITING_FOR_APPROVAL');
CREATE INDEX IF NOT EXISTS idx_agent_runs_correlation ON agent_runs(correlation_id) WHERE correlation_id IS NOT NULL;
