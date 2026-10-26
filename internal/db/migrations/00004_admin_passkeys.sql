-- +goose Up
-- Passkeys (WebAuthn) for admin sign-in. A passkey belongs to one admin and one RP ID (the public URL's host):
-- moving the server to another domain means registering passkeys again. `credential` is the WebAuthn credential
-- record as JSON (public key, sign count, flags) — nothing secret.

ALTER TABLE admins ADD COLUMN webauthn_handle TEXT NOT NULL DEFAULT '';

CREATE TABLE admin_passkeys (
  id            TEXT PRIMARY KEY,
  admin_id      TEXT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  rp_id         TEXT NOT NULL,
  credential_id TEXT NOT NULL,
  credential    TEXT NOT NULL,
  name          TEXT NOT NULL,
  created_at    BIGINT NOT NULL,
  last_used_at  BIGINT
);
CREATE UNIQUE INDEX admin_passkeys_credential ON admin_passkeys(rp_id, credential_id);
CREATE INDEX admin_passkeys_admin ON admin_passkeys(admin_id);

-- +goose Down
DROP TABLE admin_passkeys;
ALTER TABLE admins DROP COLUMN webauthn_handle;
