package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/catzoidhub/DialCatz-Den/internal/db"
)

// Backups for moving a server (export here, restore there — SQLite and PostgreSQL alike): members, workspaces with
// their items, memberships, devices (token hashes, so every device keeps working when the new server answers on the
// same address) and the activity log. Admins stay with each server (the new one has its own, made at its first
// start); open invite links are left out (they carry the old address).

// BackupTables, parents first.
var BackupTables = []string{"members", "workspaces", "memberships", "tokens", "token_workspaces", "items", "audit_log"}

// wipeTables, children first: everything a restore replaces (invites too: they point at the old members).
var wipeTables = []string{"token_workspaces", "invite_workspaces", "invites", "tokens", "memberships", "items", "workspaces", "members", "audit_log"}

type Row = map[string]any

// SchemaVersion is the newest migration applied (goose), so a backup knows what it was made with.
func (s *Store) SchemaVersion() (int64, error) {
	var v int64
	err := s.row(s.db, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = ?`, true).Scan(&v)
	return v, err
}

// ExportTable reads every row of a table as column → value.
func (s *Store) ExportTable(table string) ([]Row, error) {
	if !allowed(table) {
		return nil, fmt.Errorf("not a backup table: %s", table)
	}
	rs, err := s.rows(s.db, `SELECT * FROM `+table)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := Row{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				r[strings.ToLower(c)] = string(b)
			} else {
				r[strings.ToLower(c)] = vals[i]
			}
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

func allowed(table string) bool {
	for _, t := range wipeTables {
		if t == table {
			return true
		}
	}
	return false
}

// Restore replaces everything with the backup's tables, in one transaction. Only the columns this server's tables
// have are written (a backup from an older server still restores; missing columns take their defaults).
func (s *Store) Restore(tables map[string][]Row) (map[string]int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, t := range wipeTables {
		if _, err := s.exec(tx, `DELETE FROM `+t); err != nil {
			return nil, fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	counts := map[string]int{}
	for _, t := range BackupTables {
		rows := tables[t]
		if len(rows) == 0 {
			continue
		}
		cols, err := s.columns(tx, t)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			names := []string{}
			args := []any{}
			for _, c := range cols {
				if v, ok := r[c]; ok {
					names = append(names, c)
					args = append(args, plain(v))
				}
			}
			if len(names) == 0 {
				continue
			}
			q := `INSERT INTO ` + t + ` (` + strings.Join(names, ", ") + `) VALUES (` + strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ") + `)`
			if _, err := s.exec(tx, q, args...); err != nil {
				return nil, fmt.Errorf("restoring %s: %w", t, err)
			}
			counts[t]++
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return counts, nil
}

// columns lists a table's columns on this server (lower case).
func (s *Store) columns(q db.Querier, table string) ([]string, error) {
	rs, err := s.rows(q, `SELECT * FROM `+table+` WHERE 1 = 0`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	for i := range cols {
		cols[i] = strings.ToLower(cols[i])
	}
	return cols, err
}

// plain turns a JSON value into what the drivers take: whole numbers as int64, others as float64.
func plain(v any) any {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, _ := n.Float64()
		return f
	}
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	if b, ok := v.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	return v
}
