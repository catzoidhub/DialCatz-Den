package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Item is one synced thing of a workspace: a collection, folder, request or environment (names only — variable
// values never reach the server). Every write takes the workspace's next revision number.
type Item struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	ParentID  *string         `json:"parentId"`
	Name      string          `json:"name"`
	Sort      float64         `json:"sort"`
	Data      json.RawMessage `json:"data"`
	Revision  int64           `json:"revision"`
	Deleted   bool            `json:"deleted"`
	UpdatedBy *Person         `json:"updatedBy,omitempty"`
	UpdatedAt int64           `json:"updatedAt"`
}

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Change is what a client pushes: the item as it has it, and the revision its copy was based on (0 = new).
type Change struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	ParentID     *string         `json:"parentId"`
	Name         string          `json:"name"`
	Sort         float64         `json:"sort"`
	Data         json.RawMessage `json:"data"`
	Deleted      bool            `json:"deleted"`
	BaseRevision int64           `json:"baseRevision"`
}

type Applied struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type Conflict struct {
	ID     string `json:"id"`
	Server *Item  `json:"server"`
}

var Kinds = map[string]bool{"collection": true, "folder": true, "request": true, "environment": true}

const itemCols = `i.id, i.kind, i.parent_id, i.name, i.sort, i.data, i.revision, i.deleted, i.updated_at, i.updated_by, COALESCE(m.name, '')`

func scanItem(sc interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var parent, by sql.NullString
	var data string
	var deleted int
	var byName string
	if err := sc.Scan(&it.ID, &it.Kind, &parent, &it.Name, &it.Sort, &data, &it.Revision, &deleted, &it.UpdatedAt, &by, &byName); err != nil {
		return nil, err
	}
	if parent.Valid {
		it.ParentID = &parent.String
	}
	if by.Valid {
		it.UpdatedBy = &Person{ID: by.String, Name: byName}
	}
	it.Data = json.RawMessage(data)
	it.Deleted = deleted == 1
	return &it, nil
}

// Changes returns the items changed after revision `since` (deleted ones too) and the workspace's revision.
func (s *Store) Changes(workspaceID string, since int64) (int64, []Item, error) {
	var rev int64
	err := s.row(s.db, `SELECT revision FROM workspaces WHERE id = ?`, workspaceID).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, ErrNotFound
	}
	if err != nil {
		return 0, nil, err
	}
	rs, err := s.rows(s.db, `SELECT `+itemCols+` FROM items i LEFT JOIN members m ON m.id = i.updated_by
		WHERE i.workspace_id = ? AND i.revision > ? AND i.revision <= ? ORDER BY i.revision`, workspaceID, since, rev)
	if err != nil {
		return 0, nil, err
	}
	defer rs.Close()
	out := []Item{}
	for rs.Next() {
		it, err := scanItem(rs)
		if err != nil {
			return 0, nil, err
		}
		// A client starting from zero does not need items that were created and deleted before it came.
		if since == 0 && it.Deleted {
			continue
		}
		out = append(out, *it)
	}
	return rev, out, rs.Err()
}

// Push applies changes in one transaction. A change based on an older revision than the server's is a
// conflict and is not applied (the client keeps both); deletes always win.
func (s *Store) Push(workspaceID, memberID string, changes []Change) (int64, []Applied, []Conflict, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, nil, err
	}
	defer tx.Rollback()
	lock := `SELECT revision FROM workspaces WHERE id = ?`
	if s.db.Postgres {
		lock += ` FOR UPDATE`
	}
	var rev int64
	if err := s.row(tx, lock, workspaceID).Scan(&rev); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, nil, ErrNotFound
		}
		return 0, nil, nil, err
	}
	applied := []Applied{}
	conflicts := []Conflict{}
	t := now()
	for _, c := range changes {
		cur, err := scanItem(s.row(tx, `SELECT `+itemCols+` FROM items i LEFT JOIN members m ON m.id = i.updated_by WHERE i.workspace_id = ? AND i.id = ?`, workspaceID, c.ID))
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, nil, nil, err
		}
		if c.Deleted {
			if !exists || cur.Deleted {
				continue
			}
			rev++
			if _, err := s.exec(tx, `UPDATE items SET deleted = 1, revision = ?, updated_by = ?, updated_at = ? WHERE workspace_id = ? AND id = ?`, rev, memberID, t, workspaceID, c.ID); err != nil {
				return 0, nil, nil, err
			}
			applied = append(applied, Applied{ID: c.ID, Revision: rev})
			continue
		}
		if exists && cur.Revision > c.BaseRevision {
			conflicts = append(conflicts, Conflict{ID: c.ID, Server: cur})
			continue
		}
		data := string(c.Data)
		if data == "" || data == "null" {
			data = "{}"
		}
		rev++
		_, err = s.exec(tx, `INSERT INTO items (id, workspace_id, kind, parent_id, name, sort, data, revision, deleted, updated_by, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
			ON CONFLICT (workspace_id, id) DO UPDATE SET kind = excluded.kind, parent_id = excluded.parent_id, name = excluded.name,
			sort = excluded.sort, data = excluded.data, revision = excluded.revision, deleted = 0, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
			c.ID, workspaceID, c.Kind, c.ParentID, c.Name, c.Sort, data, rev, memberID, t)
		if err != nil {
			return 0, nil, nil, err
		}
		applied = append(applied, Applied{ID: c.ID, Revision: rev})
	}
	if _, err := s.exec(tx, `UPDATE workspaces SET revision = ? WHERE id = ?`, rev, workspaceID); err != nil {
		return 0, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, nil, err
	}
	return rev, applied, conflicts, nil
}

// Revision is a workspace's current revision (for the live event stream's hello).
func (s *Store) Revision(workspaceID string) (int64, error) {
	var rev int64
	err := s.row(s.db, `SELECT revision FROM workspaces WHERE id = ?`, workspaceID).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return rev, err
}
