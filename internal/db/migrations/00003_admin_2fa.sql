-- +goose Up
-- Two-step sign-in for admins (TOTP). Secrets are stored encrypted (AES-GCM, key from DIALCATZ_SECRET_KEY);
-- totp_pending holds a secret between "start" and "confirm"; totp_last_step stops a code being used twice.
-- Recovery codes are stored as SHA-256 hashes and work once each.

ALTER TABLE admins ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN totp_pending TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admins ADD COLUMN totp_last_step BIGINT NOT NULL DEFAULT 0;

CREATE TABLE admin_recovery_codes (
  admin_id   TEXT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  code_hash  TEXT NOT NULL,
  used_at    BIGINT,
  PRIMARY KEY (admin_id, code_hash)
);

-- +goose Down
DROP TABLE admin_recovery_codes;
ALTER TABLE admins DROP COLUMN totp_last_step;
ALTER TABLE admins DROP COLUMN totp_enabled;
ALTER TABLE admins DROP COLUMN totp_pending;
ALTER TABLE admins DROP COLUMN totp_secret;
