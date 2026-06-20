CREATE TABLE IF NOT EXISTS call_recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    format TEXT DEFAULT 'wav',
    sample_rate INT DEFAULT 16000,
    channels INT DEFAULT 2,
    duration_sec INT,
    file_size_bytes BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recording_call_id ON call_recordings(call_id);
