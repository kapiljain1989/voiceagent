-- VoiceAgent Production Schema v1
-- All tables for the AI call center platform

-- Users (authentication)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'agent',
    api_key TEXT UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Agents (call center agents)
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    extension TEXT,
    department TEXT DEFAULT 'Support',
    expertise TEXT[] DEFAULT '{}',
    status TEXT DEFAULT 'Available',
    max_calls INT DEFAULT 3,
    active_calls INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Calls (complete call history)
CREATE TABLE IF NOT EXISTS calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    caller_number TEXT,
    called_number TEXT,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    mode TEXT NOT NULL DEFAULT 'interactive',
    status TEXT DEFAULT 'active',
    start_time TIMESTAMPTZ DEFAULT NOW(),
    end_time TIMESTAMPTZ,
    duration INT DEFAULT 0,
    summary TEXT,
    sentiment TEXT DEFAULT 'neutral',
    action_items TEXT[] DEFAULT '{}',
    commitments TEXT[] DEFAULT '{}',
    transcript JSONB DEFAULT '[]',
    suggestions JSONB DEFAULT '[]',
    voice_sentiment JSONB,
    robocall_score FLOAT DEFAULT 0,
    robocall_category TEXT DEFAULT 'human',
    pii_detected BOOLEAN DEFAULT FALSE,
    llm_model TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_calls_agent ON calls(agent_id);
CREATE INDEX IF NOT EXISTS idx_calls_start ON calls(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_calls_status ON calls(status);

-- Call transcripts (per-utterance for search and history)
CREATE TABLE IF NOT EXISTS call_transcripts (
    id BIGSERIAL PRIMARY KEY,
    call_id UUID REFERENCES calls(id) ON DELETE CASCADE,
    speaker TEXT NOT NULL,
    text TEXT NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_transcript_call ON call_transcripts(call_id);

-- Documents (RAG knowledge base metadata)
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT,
    content TEXT,
    chroma_id TEXT,
    chunks INT DEFAULT 0,
    status TEXT DEFAULT 'indexed',
    uploaded_at TIMESTAMPTZ DEFAULT NOW()
);

-- SIP Trunks
CREATE TABLE IF NOT EXISTS sip_trunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    provider TEXT DEFAULT 'custom',
    address TEXT NOT NULL,
    port INT DEFAULT 5060,
    transport TEXT DEFAULT 'udp',
    register BOOLEAN DEFAULT FALSE,
    username TEXT,
    password TEXT,
    caller_id TEXT,
    codecs TEXT DEFAULT 'PCMU,PCMA,G722',
    status TEXT DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Voice Prints (biometrics)
CREATE TABLE IF NOT EXISTS voice_prints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label TEXT NOT NULL,
    type TEXT NOT NULL,
    features JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Blocklist (robocall)
CREATE TABLE IF NOT EXISTS blocklist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number TEXT UNIQUE NOT NULL,
    reason TEXT,
    source TEXT DEFAULT 'manual',
    call_count INT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- LLM Configs
CREATE TABLE IF NOT EXISTS llm_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    region TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    system_prompt TEXT,
    max_tokens INT DEFAULT 512,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Queue entries (persisted across restarts)
CREATE TABLE IF NOT EXISTS queue_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id TEXT NOT NULL,
    caller_number TEXT,
    queue_name TEXT NOT NULL DEFAULT 'Support',
    reason TEXT,
    priority TEXT DEFAULT 'normal',
    wait_start TIMESTAMPTZ DEFAULT NOW(),
    assigned_agent UUID REFERENCES agents(id) ON DELETE SET NULL,
    status TEXT DEFAULT 'waiting',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_queue_status ON queue_entries(status, queue_name);
