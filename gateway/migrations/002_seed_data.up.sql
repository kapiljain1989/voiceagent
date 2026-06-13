-- Seed data for VoiceAgent
-- Default admin user (password: admin)
INSERT INTO users (username, password_hash, role) VALUES
    ('admin', 'admin', 'admin')
ON CONFLICT (username) DO NOTHING;

-- Demo agents
INSERT INTO agents (name, email, extension, department, expertise, status) VALUES
    ('Sarah Chen', 'sarah@voiceagent.ai', '2001', 'Support', ARRAY['billing','retention'], 'Available'),
    ('Alex Rivera', 'alex@voiceagent.ai', '2002', 'Sales', ARRAY['sales','upsell'], 'Available'),
    ('Priya Sharma', 'priya@voiceagent.ai', '2003', 'Support', ARRAY['technical','claims'], 'Available'),
    ('Marcus Johnson', 'marcus@voiceagent.ai', '2004', 'Billing', ARRAY['billing','payments'], 'Available'),
    ('Yuki Tanaka', 'yuki@voiceagent.ai', '2005', 'Support', ARRAY['general','retention'], 'Available'),
    ('Raj Patel', 'raj@voiceagent.ai', '2006', 'Sales', ARRAY['sales','enterprise'], 'Available'),
    ('Emma Wilson', 'emma@voiceagent.ai', '2007', 'Escalation', ARRAY['escalation','compliance'], 'Available')
ON CONFLICT DO NOTHING;

-- Default LLM config
INSERT INTO llm_configs (name, provider, model, region, is_default, max_tokens) VALUES
    ('Claude Haiku', 'anthropic-vertex', 'claude-3-5-haiku@20241022', 'us-east5', true, 512),
    ('Gemini Flash', 'google-vertex', 'gemini-2.0-flash', 'us-east5', false, 512)
ON CONFLICT DO NOTHING;
