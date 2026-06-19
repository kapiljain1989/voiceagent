-- 005: SIP Trunk Security — IP whitelist, auth, TLS, SRTP

-- IP whitelist per trunk (only allow calls from configured IPs)
CREATE TABLE IF NOT EXISTS sip_trunk_acl (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trunk_id UUID REFERENCES sip_trunks(id) ON DELETE CASCADE,
    ip_address TEXT NOT NULL,
    cidr_bits INT DEFAULT 32,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_trunk_acl_trunk ON sip_trunk_acl(trunk_id);
CREATE INDEX IF NOT EXISTS idx_trunk_acl_ip ON sip_trunk_acl(ip_address);

-- Extend sip_trunks with security fields
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS auth_realm TEXT DEFAULT '';
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS auth_user TEXT DEFAULT '';
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS auth_password_hash TEXT DEFAULT '';
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS tls_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS srtp_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE sip_trunks ADD COLUMN IF NOT EXISTS security_policy TEXT DEFAULT 'strict';

-- Security audit log
CREATE TABLE IF NOT EXISTS sip_security_log (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    trunk_id UUID REFERENCES sip_trunks(id) ON DELETE SET NULL,
    trunk_name TEXT,
    source_ip TEXT,
    call_id TEXT,
    details TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_security_log_time ON sip_security_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_security_log_type ON sip_security_log(event_type);
