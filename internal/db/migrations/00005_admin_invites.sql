-- +goose Up
-- More than one admin: an admin invites another by name + email; the one-time link lets them set their own password.

CREATE TABLE admin_invites (
  id          TEXT PRIMARY KEY,
  email       TEXT NOT NULL,
  name        TEXT NOT NULL,
  code_hash   TEXT NOT NULL UNIQUE,
  invited_by  TEXT NOT NULL,
  created_at  BIGINT NOT NULL,
  expires_at  BIGINT NOT NULL,
  used_at     BIGINT
);

-- +goose Down
DROP TABLE admin_invites;
