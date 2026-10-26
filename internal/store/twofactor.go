package store

import (
	"database/sql"
	"errors"
)

// Two-step sign-in (TOTP) for admins. The secrets come in already sealed (internal/auth.Sealer); the store only
// keeps them. Recovery codes come in as hashes.

// TOTP is an admin's two-step state as stored.
type TOTP struct {
	Secret   string // sealed; "" while off
	Pending  string // sealed; set between start and confirm
	Enabled  bool
	LastStep int64
}

func (s *Store) AdminTOTP(adminID string) (*TOTP, error) {
	t := &TOTP{}
	var enabled int
	err := s.row(s.db, `SELECT totp_secret, totp_pending, totp_enabled, totp_last_step FROM admins WHERE id = ?`, adminID).Scan(&t.Secret, &t.Pending, &enabled, &t.LastStep)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	t.Enabled = enabled == 1
	return t, err
}

// SetTOTPPending keeps a new secret until the admin confirms it with a code.
func (s *Store) SetTOTPPending(adminID, sealed string) error {
	_, err := s.exec(s.db, `UPDATE admins SET totp_pending = ? WHERE id = ?`, sealed, adminID)
	return err
}

// EnableTOTP turns two-step sign-in on with the confirmed secret and a fresh set of recovery codes (hashes).
func (s *Store) EnableTOTP(adminID, sealed string, step int64, recoveryHashes []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.exec(tx, `UPDATE admins SET totp_secret = ?, totp_pending = '', totp_enabled = 1, totp_last_step = ? WHERE id = ?`, sealed, step, adminID); err != nil {
		return err
	}
	if _, err := s.exec(tx, `DELETE FROM admin_recovery_codes WHERE admin_id = ?`, adminID); err != nil {
		return err
	}
	for _, h := range recoveryHashes {
		if _, err := s.exec(tx, `INSERT INTO admin_recovery_codes (admin_id, code_hash) VALUES (?, ?)`, adminID, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DisableTOTP(adminID string) error {
	if _, err := s.exec(s.db, `UPDATE admins SET totp_secret = '', totp_pending = '', totp_enabled = 0, totp_last_step = 0 WHERE id = ?`, adminID); err != nil {
		return err
	}
	_, err := s.exec(s.db, `DELETE FROM admin_recovery_codes WHERE admin_id = ?`, adminID)
	return err
}

// UseTOTPStep records a code's time step as used; false when that step (or a later one) was used already.
func (s *Store) UseTOTPStep(adminID string, step int64) (bool, error) {
	r, err := s.exec(s.db, `UPDATE admins SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, adminID, step)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n == 1, nil
}

// UseRecoveryCode spends a recovery code; false when it is unknown or was used.
func (s *Store) UseRecoveryCode(adminID, hash string) (bool, error) {
	r, err := s.exec(s.db, `UPDATE admin_recovery_codes SET used_at = ? WHERE admin_id = ? AND code_hash = ? AND used_at IS NULL`, now(), adminID, hash)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n == 1, nil
}

func (s *Store) RecoveryCodesLeft(adminID string) (int, error) {
	var n int
	err := s.row(s.db, `SELECT COUNT(*) FROM admin_recovery_codes WHERE admin_id = ? AND used_at IS NULL`, adminID).Scan(&n)
	return n, err
}
