CREATE TABLE identities (
 id VARCHAR(36) PRIMARY KEY,
 issuer VARCHAR(256) NOT NULL,
 subject VARCHAR(128) NOT NULL,
 email VARCHAR(320) NOT NULL,
 client_code VARCHAR(64) NOT NULL UNIQUE,
 code_version INTEGER NOT NULL DEFAULT 1,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 fixture BOOLEAN NOT NULL DEFAULT FALSE,
 UNIQUE(issuer,subject)
);
CREATE TABLE login_transactions (
 id VARCHAR(128) PRIMARY KEY,
 device_id VARCHAR(128) NOT NULL,
 state_hash VARCHAR(64) NOT NULL UNIQUE,
 nonce VARCHAR(128) NOT NULL,
 verifier_cipher TEXT NOT NULL,
 expires_at BIGINT NOT NULL,
 consumed BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE account_sessions (
 id VARCHAR(128) PRIMARY KEY,
 identity_id VARCHAR(36) NOT NULL REFERENCES identities(id),
 device_id VARCHAR(128) NOT NULL,
 access_hash VARCHAR(64) NOT NULL UNIQUE,
 refresh_hash VARCHAR(64) NOT NULL UNIQUE,
 cognito_cipher TEXT NOT NULL,
 access_expires BIGINT NOT NULL,
 expires_at BIGINT NOT NULL,
 revoked BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE consumed_refresh_tokens (
 token_hash VARCHAR(64) PRIMARY KEY,
 session_id VARCHAR(128) NOT NULL REFERENCES account_sessions(id)
);
CREATE TABLE fixture_bindings (
 fixture_key VARCHAR(64) PRIMARY KEY,
 identity_id VARCHAR(36) NOT NULL REFERENCES identities(id)
);
