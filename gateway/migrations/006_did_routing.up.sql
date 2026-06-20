-- 006: DID Routing + ACD enhancements

-- DID routing table
CREATE TABLE IF NOT EXISTS did_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    did_pattern TEXT NOT NULL,
    match_type TEXT DEFAULT 'exact',
    trunk_id UUID REFERENCES sip_trunks(id) ON DELETE SET NULL,
    destination_type TEXT NOT NULL,
    destination_value TEXT NOT NULL,
    priority INT DEFAULT 0,
    time_condition JSONB,
    overflow_destination TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_did_routes_pattern ON did_routes(did_pattern);
CREATE INDEX IF NOT EXISTS idx_did_routes_priority ON did_routes(priority DESC);

-- Default route
INSERT INTO did_routes (did_pattern, match_type, destination_type, destination_value, priority)
VALUES ('*', 'prefix', 'queue', 'Support', -1)
ON CONFLICT DO NOTHING;

-- ACD: queue configuration
ALTER TABLE queues ADD COLUMN IF NOT EXISTS routing_strategy TEXT DEFAULT 'skills';
ALTER TABLE queues ADD COLUMN IF NOT EXISTS wrap_up_sec INT DEFAULT 15;

-- ACD: agent idle tracking
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_call_end TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS active_calls INT DEFAULT 0;
