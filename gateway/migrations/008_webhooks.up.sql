CREATE TABLE IF NOT EXISTS webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    method TEXT DEFAULT 'POST',
    headers JSONB DEFAULT '{}',
    auth_type TEXT DEFAULT 'none',
    auth_value TEXT,
    events TEXT[] DEFAULT '{}',
    enabled BOOLEAN DEFAULT TRUE,
    retry_count INT DEFAULT 3,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    call_id TEXT,
    status_code INT,
    response_body TEXT,
    error TEXT,
    sent_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_logs_webhook ON webhook_logs(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_sent ON webhook_logs(sent_at);
