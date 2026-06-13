DELETE FROM llm_configs WHERE name IN ('Claude Haiku', 'Gemini Flash');
DELETE FROM agents WHERE email LIKE '%@voiceagent.ai';
DELETE FROM users WHERE username = 'admin';
