-- +goose Up
-- Portable SQL: runs on SQLite and PostgreSQL alike. Times are Unix milliseconds, booleans 0 / 1, ids text.

CREATE TABLE admins (
  id            TEXT PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at    BIGINT NOT NULL
);

CREATE TABLE admin_sessions (
  id_hash    TEXT PRIMARY KEY,
  admin_id   TEXT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL
);

CREATE TABLE members (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  email      TEXT NOT NULL UNIQUE,
  avatar_url TEXT NOT NULL DEFAULT '',
  disabled   INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL
);

CREATE TABLE workspaces (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  revision   BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL
);

CREATE TABLE memberships (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  member_id    TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  role         TEXT NOT NULL,
  PRIMARY KEY (workspace_id, member_id)
);

CREATE TABLE tokens (
  id           TEXT PRIMARY KEY,
  member_id    TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  device       TEXT NOT NULL,
  prefix       TEXT NOT NULL,
  hash         TEXT NOT NULL UNIQUE,
  created_at   BIGINT NOT NULL,
  expires_at   BIGINT,
  last_used_at BIGINT,
  revoked_at   BIGINT
);
CREATE INDEX tokens_member ON tokens(member_id);

CREATE TABLE invites (
  id                 TEXT PRIMARY KEY,
  member_id          TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  code_hash          TEXT NOT NULL UNIQUE,
  token_expiry_days  INTEGER NOT NULL DEFAULT 0,
  created_at         BIGINT NOT NULL,
  expires_at         BIGINT NOT NULL,
  used_at            BIGINT,
  token_id           TEXT
);

CREATE TABLE items (
  id           TEXT NOT NULL,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  kind         TEXT NOT NULL,
  parent_id    TEXT,
  name         TEXT NOT NULL,
  sort         DOUBLE PRECISION NOT NULL DEFAULT 0,
  data         TEXT NOT NULL DEFAULT '{}',
  revision     BIGINT NOT NULL,
  deleted      INTEGER NOT NULL DEFAULT 0,
  updated_by   TEXT,
  updated_at   BIGINT NOT NULL,
  PRIMARY KEY (workspace_id, id)
);
CREATE INDEX items_revision ON items(workspace_id, revision);

CREATE TABLE audit_log (
  id         TEXT PRIMARY KEY,
  at         BIGINT NOT NULL,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_at ON audit_log(at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE items;
DROP TABLE invites;
DROP TABLE tokens;
DROP TABLE memberships;
DROP TABLE workspaces;
DROP TABLE members;
DROP TABLE admin_sessions;
DROP TABLE admins;
