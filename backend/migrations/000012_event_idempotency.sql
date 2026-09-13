-- 000012_event_idempotency.sql — Durable event idempotency (P1-5)
CREATE TABLE IF NOT EXISTS processed_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    source TEXT,
    correlation_id UUID,
    payload_hash TEXT,
    consumer TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processed' CHECK (status IN ('processing','processed','failed')),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, consumer)
);
CREATE INDEX IF NOT EXISTS idx_processed_events_type ON processed_events(event_type);
CREATE INDEX IF NOT EXISTS idx_processed_events_correlation ON processed_events(correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_processed_events_consumer ON processed_events(consumer, event_type);

-- Helper for idempotency check
CREATE OR REPLACE FUNCTION check_event_idempotency(p_event_id TEXT, p_consumer TEXT) RETURNS BOOLEAN AS $$
DECLARE
    exists BOOLEAN;
BEGIN
    SELECT TRUE INTO exists FROM processed_events WHERE event_id = p_event_id AND consumer = p_consumer AND status = 'processed' LIMIT 1;
    RETURN COALESCE(exists, FALSE);
END;
$$ LANGUAGE plpgsql;
