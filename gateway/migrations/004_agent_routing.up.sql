-- Agent routing: full profile + queue-based skill routing

-- Extend agents table with routing fields
ALTER TABLE agents ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id);
ALTER TABLE agents ADD COLUMN IF NOT EXISTS languages TEXT[] DEFAULT '{English}';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS priority INT DEFAULT 1;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS schedule JSONB;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS customer_tiers TEXT[] DEFAULT '{standard}';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS current_calls INT DEFAULT 0;

-- Queue definitions
CREATE TABLE IF NOT EXISTS queues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    skills_required TEXT[] DEFAULT '{}',
    max_wait_seconds INT DEFAULT 300,
    overflow_queue TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed queues
INSERT INTO queues (name, description, skills_required) VALUES
    ('Support', 'General customer support', '{general,technical,billing}'),
    ('Sales', 'Sales and upsell inquiries', '{sales,upsell,enterprise}'),
    ('Billing', 'Billing, payments, refunds', '{billing,payments,refunds}'),
    ('Escalation', 'Complex issues, complaints', '{escalation,compliance,retention}')
ON CONFLICT (name) DO NOTHING;

-- Agent-queue membership (many-to-many)
CREATE TABLE IF NOT EXISTS agent_queues (
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    queue_id UUID REFERENCES queues(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, queue_id)
);

-- Link admin user to first agent and assign to queues
DO $$
DECLARE
    admin_uid UUID;
    agent_uid UUID;
    support_qid UUID;
BEGIN
    SELECT id INTO admin_uid FROM users WHERE username='admin' LIMIT 1;
    SELECT id INTO agent_uid FROM agents WHERE name='Sarah Chen' LIMIT 1;
    SELECT id INTO support_qid FROM queues WHERE name='Support' LIMIT 1;

    IF admin_uid IS NOT NULL AND agent_uid IS NOT NULL THEN
        UPDATE agents SET user_id = admin_uid WHERE id = agent_uid AND user_id IS NULL;
    END IF;

    IF agent_uid IS NOT NULL AND support_qid IS NOT NULL THEN
        INSERT INTO agent_queues (agent_id, queue_id) VALUES (agent_uid, support_qid) ON CONFLICT DO NOTHING;
    END IF;
END $$;
