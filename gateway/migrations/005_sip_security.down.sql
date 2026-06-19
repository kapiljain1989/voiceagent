DROP TABLE IF EXISTS sip_security_log;
DROP TABLE IF EXISTS sip_trunk_acl;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS auth_realm;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS auth_user;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS auth_password_hash;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS tls_enabled;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS srtp_enabled;
ALTER TABLE sip_trunks DROP COLUMN IF EXISTS security_policy;
