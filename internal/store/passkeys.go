package store

import (
	"database/sql"
	"errors"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
)

// Passkeys (WebAuthn) for admins. The store keeps each credential record as the JSON the api package gives it;
// it never needs to read inside.

// Passkey is one admin passkey as listed in the panel; Credential is the stored record (JSON).
type Passkey struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt *int64 `json:"lastUsedAt"`
	Credential string `json:"-"`
}

// AdminHandle is the admin's WebAuthn user handle (random, stable), made the first time it is asked for.
func (s *Store) AdminHandle(adminID string) (string, error) {
	var h string
	if err := s.row(s.db, `SELECT webauthn_handle FROM admins WHERE id = ?`, adminID).Scan(&h); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if h != "" {
		return h, nil
	}
	h = auth.Random(32)
	_, err := s.exec(s.db, `UPDATE admins SET webauthn_handle = ? WHERE id = ? AND webauthn_handle = ''`, h, adminID)
	if err != nil {
		return "", err
	}
	// Another request may have set one first: read back what stuck.
	err = s.row(s.db, `SELECT webauthn_handle FROM admins WHERE id = ?`, adminID).Scan(&h)
	return h, err
}

// AdminByHandle finds the admin a passkey's user handle belongs to.
func (s *Store) AdminByHandle(handle string) (*Admin, error) {
	if handle == "" {
		return nil, ErrNotFound
	}
	return s.scanAdmin(s.row(s.db, `SELECT `+adminCols+` FROM admins a WHERE a.webauthn_handle = ?`, handle))
}

func (s *Store) Passkeys(adminID, rpID string) ([]Passkey, error) {
	rs, err := s.rows(s.db, `SELECT id, name, created_at, last_used_at, credential FROM admin_passkeys WHERE admin_id = ? AND rp_id = ? ORDER BY created_at`, adminID, rpID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []Passkey{}
	for rs.Next() {
		var p Passkey
		var used sql.NullInt64
		if err := rs.Scan(&p.ID, &p.Name, &p.CreatedAt, &used, &p.Credential); err != nil {
			return nil, err
		}
		p.LastUsedAt = nullable(used)
		out = append(out, p)
	}
	return out, rs.Err()
}

func (s *Store) AddPasskey(adminID, rpID, credentialID, credential, name string) (*Passkey, error) {
	p := &Passkey{ID: auth.ID("pk_"), Name: name, CreatedAt: now()}
	_, err := s.exec(s.db, `INSERT INTO admin_passkeys (id, admin_id, rp_id, credential_id, credential, name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, p.ID, adminID, rpID, credentialID, credential, name, p.CreatedAt)
	return p, unique(err)
}

// PasskeyUsed stores the updated record (sign count, flags) after a sign-in.
func (s *Store) PasskeyUsed(adminID, rpID, credentialID, credential string) error {
	_, err := s.exec(s.db, `UPDATE admin_passkeys SET credential = ?, last_used_at = ? WHERE admin_id = ? AND rp_id = ? AND credential_id = ?`, credential, now(), adminID, rpID, credentialID)
	return err
}

func (s *Store) DeletePasskey(adminID, id string) error {
	r, err := s.exec(s.db, `DELETE FROM admin_passkeys WHERE id = ? AND admin_id = ?`, id, adminID)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
