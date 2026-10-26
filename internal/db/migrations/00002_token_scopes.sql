-- +goose Up
-- Token scopes: a token (or an invite, whose token inherits it) may be limited to some of its member's workspaces.
-- scoped = 0 is every workspace of the member (as before); scoped = 1 is only the rows listed below, so a scope
-- whose workspaces were all deleted stays empty instead of quietly widening to "all".

ALTER TABLE tokens ADD COLUMN scoped INTEGER NOT NULL DEFAULT 0;
ALTER TABLE invites ADD COLUMN scoped INTEGER NOT NULL DEFAULT 0;

CREATE TABLE token_workspaces (
  token_id     TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  PRIMARY KEY (token_id, workspace_id)
);

CREATE TABLE invite_workspaces (
  invite_id    TEXT NOT NULL REFERENCES invites(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  PRIMARY KEY (invite_id, workspace_id)
);

-- +goose Down
DROP TABLE invite_workspaces;
DROP TABLE token_workspaces;
ALTER TABLE invites DROP COLUMN scoped;
ALTER TABLE tokens DROP COLUMN scoped;
