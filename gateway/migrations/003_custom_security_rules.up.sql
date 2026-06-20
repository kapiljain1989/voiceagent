-- Custom security rules (user-defined, merged with built-in defaults)

CREATE TABLE IF NOT EXISTS custom_pii_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    regex TEXT NOT NULL,
    mask TEXT NOT NULL DEFAULT '[REDACTED]',
    level TEXT NOT NULL DEFAULT 'high',
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS custom_robocall_keywords (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phrase TEXT UNIQUE NOT NULL,
    weight FLOAT DEFAULT 1.0,
    category TEXT DEFAULT 'spam',
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS voice_biometric_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT UNIQUE NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Default biometric config
INSERT INTO voice_biometric_config (key, value) VALUES
    ('match_threshold', '0.85'),
    ('fraud_alert_threshold', '0.90'),
    ('auto_enroll', 'false'),
    ('min_audio_seconds', '3')
ON CONFLICT (key) DO NOTHING;
