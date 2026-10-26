package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
)

// More than one admin. Admins are all equal; one invites another with a one-time link (the new admin chooses
// their own password), and any admin can remove another — never the last one.

// AdminRow is an admin as the Admins page lists them.
type AdminRow struct {
	Admin
	Passkeys int `json:"passkeys"`
}

func (s *Store) ListAdmins() ([]AdminRow, error) {
	rs, err := s.rows(s.db, `SELECT a.id, a.email, a.name, a.created_at, a.totp_enabled, (SELECT COUNT(*) FROM admin_passkeys p WHERE p.admin_id = a.id) FROM admins a ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []AdminRow{}
	for rs.Next() {
		var r AdminRow
		var tf int
		if err := rs.Scan(&r.ID, &r.Email, &r.Name, &r.CreatedAt, &tf, &r.Passkeys); err != nil {
			return nil, err
		}
		r.TwoFactor = tf == 1
		out = append(out, r)
	}
	return out, rs.Err()
}

var ErrLastAdmin = errors.New("the last admin")

// DeleteAdmin removes an admin (their sessions, passkeys and two-step codes go with them); never the last one.
func (s *Store) DeleteAdmin(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := s.row(tx, `SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	r, err := s.exec(tx, `DELETE FROM admins WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if c, _ := r.RowsAffected(); c == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// AdminInvite is a pending invite as listed (the code itself is never stored).
type AdminInvite struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	InvitedBy string `json:"invitedBy"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

const adminInviteTTL = 48 * time.Hour

// CreateAdminInvite returns the one-time code (shown once, in the link) and the invite.
func (s *Store) CreateAdminInvite(email, name, invitedBy string) (string, *AdminInvite, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var taken int
	if err := s.row(s.db, `SELECT COUNT(*) FROM admins WHERE email = ?`, email).Scan(&taken); err != nil {
		return "", nil, err
	}
	if taken > 0 {
		return "", nil, ErrConflict
	}
	code, hash := auth.NewInviteCode()
	t := now()
	inv := &AdminInvite{ID: auth.ID("ai_"), Email: email, Name: strings.TrimSpace(name), InvitedBy: invitedBy, CreatedAt: t, ExpiresAt: t + adminInviteTTL.Milliseconds()}
	// A new invite for the same email replaces an older open one.
	if _, err := s.exec(s.db, `DELETE FROM admin_invites WHERE email = ? AND used_at IS NULL`, email); err != nil {
		return "", nil, err
	}
	_, err := s.exec(s.db, `INSERT INTO admin_invites (id, email, name, code_hash, invited_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, inv.ID, inv.Email, inv.Name, hash, invitedBy, inv.CreatedAt, inv.ExpiresAt)
	return code, inv, err
}

func (s *Store) ListAdminInvites() ([]AdminInvite, error) {
	rs, err := s.rows(s.db, `SELECT id, email, name, invited_by, created_at, expires_at FROM admin_invites WHERE used_at IS NULL AND expires_at > ? ORDER BY created_at DESC`, now())
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []AdminInvite{}
	for rs.Next() {
		var i AdminInvite
		if err := rs.Scan(&i.ID, &i.Email, &i.Name, &i.InvitedBy, &i.CreatedAt, &i.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rs.Err()
}

func (s *Store) DeleteAdminInvite(id string) error {
	_, err := s.exec(s.db, `DELETE FROM admin_invites WHERE id = ? AND used_at IS NULL`, id)
	return err
}

// AdminInviteByCode finds a live invite (for the accept page).
func (s *Store) AdminInviteByCode(code string) (*AdminInvite, error) {
	var i AdminInvite
	err := s.row(s.db, `SELECT id, email, name, invited_by, created_at, expires_at FROM admin_invites WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, auth.Hash(code), now()).Scan(&i.ID, &i.Email, &i.Name, &i.InvitedBy, &i.CreatedAt, &i.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &i, err
}

// AcceptAdminInvite makes the admin from a live invite with the password they chose, in one transaction.
func (s *Store) AcceptAdminInvite(code, name, password string) (*Admin, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id, email, invName string
	err = s.row(tx, `SELECT id, email, name FROM admin_invites WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, auth.Hash(code), now()).Scan(&id, &email, &invName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = invName
	}
	a := &Admin{ID: auth.ID("a_"), Email: email, Name: name, CreatedAt: now()}
	if _, err := s.exec(tx, `INSERT INTO admins (id, email, name, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`, a.ID, a.Email, a.Name, hash, a.CreatedAt); err != nil {
		return nil, unique(err)
	}
	r, err := s.exec(tx, `UPDATE admin_invites SET used_at = ? WHERE id = ? AND used_at IS NULL`, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return a, tx.Commit()
}
