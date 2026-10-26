// Package store holds every query of the server. Written once with ? placeholders (db.Q rewrites them for
// PostgreSQL) in SQL both SQLite and PostgreSQL understand.
package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/db"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

type Store struct{ db *db.DB }

func New(d *db.DB) *Store { return &Store{db: d} }

// DB is the plain connection, for calls that take a db.Querier outside a transaction.
func (s *Store) DB() db.Querier { return s.db }

func now() int64 { return time.Now().UnixMilli() }

func (s *Store) exec(q db.Querier, query string, args ...any) (sql.Result, error) {
	return q.Exec(s.db.Q(query), args...)
}
func (s *Store) row(q db.Querier, query string, args ...any) *sql.Row {
	return q.QueryRow(s.db.Q(query), args...)
}
func (s *Store) rows(q db.Querier, query string, args ...any) (*sql.Rows, error) {
	return q.Query(s.db.Q(query), args...)
}

func unique(err error) error {
	if err != nil && (strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate key")) {
		return ErrConflict
	}
	return err
}

func nullable(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

var Roles = map[string]bool{"owner": true, "editor": true, "viewer": true}

// ------------------------------------------------------------------ admins

type Admin struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	// TwoFactor: two-step sign-in (TOTP) is on; signing in then also asks for a code.
	TwoFactor bool `json:"twoFactor"`
	hash      string
}

const adminCols = `a.id, a.email, a.name, a.created_at, a.password_hash, a.totp_enabled`

func (s *Store) AdminCount() (int, error) {
	var n int
	err := s.row(s.db, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(email, name, password string) (*Admin, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	a := &Admin{ID: auth.ID("a_"), Email: strings.ToLower(strings.TrimSpace(email)), Name: strings.TrimSpace(name), CreatedAt: now()}
	_, err = s.exec(s.db, `INSERT INTO admins (id, email, name, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`, a.ID, a.Email, a.Name, hash, a.CreatedAt)
	return a, unique(err)
}

func (s *Store) scanAdmin(r *sql.Row) (*Admin, error) {
	a := &Admin{}
	var twoFactor int
	err := r.Scan(&a.ID, &a.Email, &a.Name, &a.CreatedAt, &a.hash, &twoFactor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	a.TwoFactor = twoFactor == 1
	return a, err
}

func (s *Store) GetAdmin(id string) (*Admin, error) {
	return s.scanAdmin(s.row(s.db, `SELECT `+adminCols+` FROM admins a WHERE a.id = ?`, id))
}

// AdminByEmail is for sign-in through Google / GitHub: it only ever finds an existing admin.
func (s *Store) AdminByEmail(email string) (*Admin, error) {
	return s.scanAdmin(s.row(s.db, `SELECT `+adminCols+` FROM admins a WHERE a.email = ?`, strings.ToLower(strings.TrimSpace(email))))
}

// CheckAdmin returns the admin when the email and password match.
func (s *Store) CheckAdmin(email, password string) (*Admin, error) {
	a, err := s.AdminByEmail(email)
	if err != nil {
		// Same work either way, so the answer time does not tell which emails exist.
		auth.CheckPassword(password, "$argon2id$v=19$m=65536,t=2,p=2$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g")
		return nil, ErrNotFound
	}
	if !auth.CheckPassword(password, a.hash) {
		return nil, ErrNotFound
	}
	return a, nil
}

func (s *Store) SetAdminPassword(id, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.exec(s.db, `UPDATE admins SET password_hash = ? WHERE id = ?`, hash, id)
	if err == nil {
		_, err = s.exec(s.db, `DELETE FROM admin_sessions WHERE admin_id = ?`, id)
	}
	return err
}

const sessionTTL = 7 * 24 * time.Hour

// CreateSession returns the cookie value; only its hash is stored.
func (s *Store) CreateSession(adminID string) (string, error) {
	sid := auth.Random(32)
	t := now()
	_, err := s.exec(s.db, `INSERT INTO admin_sessions (id_hash, admin_id, created_at, expires_at) VALUES (?, ?, ?, ?)`, auth.Hash(sid), adminID, t, t+sessionTTL.Milliseconds())
	_, _ = s.exec(s.db, `DELETE FROM admin_sessions WHERE expires_at < ?`, t)
	return sid, err
}

func (s *Store) SessionAdmin(sid string) (*Admin, error) {
	return s.scanAdmin(s.row(s.db, `SELECT `+adminCols+` FROM admin_sessions s JOIN admins a ON a.id = s.admin_id WHERE s.id_hash = ? AND s.expires_at > ?`, auth.Hash(sid), now()))
}

func (s *Store) DeleteSession(sid string) error {
	_, err := s.exec(s.db, `DELETE FROM admin_sessions WHERE id_hash = ?`, auth.Hash(sid))
	return err
}

// ------------------------------------------------------------------ members

type Membership struct {
	WorkspaceID string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
}

type Member struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// AvatarURL is the admin's picture URL, or a Gravatar URL for the email when none was set (Gravatar is then true).
	AvatarURL  string       `json:"avatarUrl"`
	Gravatar   bool         `json:"gravatar"`
	Disabled   bool         `json:"disabled"`
	CreatedAt  int64        `json:"createdAt"`
	Workspaces []Membership `json:"workspaces"`
	Devices    int          `json:"devices"`
}

// GravatarURL for an email; d=404 makes Gravatar answer 404 when there is no picture, so clients show initials.
func GravatarURL(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "https://gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?s=160&d=404"
}

func (m *Member) fillAvatar() {
	if m.AvatarURL == "" && m.Email != "" {
		m.AvatarURL, m.Gravatar = GravatarURL(m.Email), true
	}
}

func (s *Store) memberships(memberID string) ([]Membership, error) {
	rs, err := s.rows(s.db, `SELECT w.id, w.name, m.role FROM memberships m JOIN workspaces w ON w.id = m.workspace_id WHERE m.member_id = ? ORDER BY w.name`, memberID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []Membership{}
	for rs.Next() {
		var m Membership
		if err := rs.Scan(&m.WorkspaceID, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rs.Err()
}

func (s *Store) ListMembers() ([]Member, error) {
	rs, err := s.rows(s.db, `SELECT m.id, m.name, m.email, m.avatar_url, m.disabled, m.created_at,
		(SELECT COUNT(*) FROM tokens t WHERE t.member_id = m.id AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?))
		FROM members m ORDER BY m.name`, now())
	if err != nil {
		return nil, err
	}
	out := []Member{}
	for rs.Next() {
		var m Member
		var disabled int
		if err := rs.Scan(&m.ID, &m.Name, &m.Email, &m.AvatarURL, &disabled, &m.CreatedAt, &m.Devices); err != nil {
			rs.Close()
			return nil, err
		}
		m.Disabled = disabled == 1
		m.fillAvatar()
		out = append(out, m)
	}
	rs.Close()
	for i := range out {
		if out[i].Workspaces, err = s.memberships(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) GetMember(id string) (*Member, error) {
	m := &Member{}
	var disabled int
	err := s.row(s.db, `SELECT id, name, email, avatar_url, disabled, created_at FROM members WHERE id = ?`, id).Scan(&m.ID, &m.Name, &m.Email, &m.AvatarURL, &disabled, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Disabled = disabled == 1
	m.fillAvatar()
	m.Workspaces, err = s.memberships(id)
	return m, err
}

func (s *Store) CreateMember(name, email, avatar string) (*Member, error) {
	m := &Member{ID: auth.ID("m_"), Name: strings.TrimSpace(name), Email: strings.ToLower(strings.TrimSpace(email)), AvatarURL: strings.TrimSpace(avatar), CreatedAt: now(), Workspaces: []Membership{}}
	_, err := s.exec(s.db, `INSERT INTO members (id, name, email, avatar_url, created_at) VALUES (?, ?, ?, ?, ?)`, m.ID, m.Name, m.Email, m.AvatarURL, m.CreatedAt)
	m.fillAvatar()
	return m, unique(err)
}

func (s *Store) UpdateMember(id, name, email, avatar string, disabled bool) error {
	d := 0
	if disabled {
		d = 1
	}
	r, err := s.exec(s.db, `UPDATE members SET name = ?, email = ?, avatar_url = ?, disabled = ? WHERE id = ?`, strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(avatar), d, id)
	if err != nil {
		return unique(err)
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteMember(id string) error {
	r, err := s.exec(s.db, `DELETE FROM members WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMembership adds the member to the workspace with the role, or changes the role; role "" removes them.
func (s *Store) SetMembership(workspaceID, memberID, role string) error {
	if role == "" {
		_, err := s.exec(s.db, `DELETE FROM memberships WHERE workspace_id = ? AND member_id = ?`, workspaceID, memberID)
		return err
	}
	_, err := s.exec(s.db, `INSERT INTO memberships (workspace_id, member_id, role) VALUES (?, ?, ?) ON CONFLICT (workspace_id, member_id) DO UPDATE SET role = excluded.role`, workspaceID, memberID, role)
	return err
}

// Role of a member on a workspace ("" when not a member).
func (s *Store) Role(workspaceID, memberID string) (string, error) {
	var role string
	err := s.row(s.db, `SELECT role FROM memberships WHERE workspace_id = ? AND member_id = ?`, workspaceID, memberID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// ------------------------------------------------------------------ workspaces

type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Revision  int64  `json:"revision"`
	CreatedAt int64  `json:"createdAt"`
	Members   int    `json:"members"`
	Items     int    `json:"items"`
}

func (s *Store) ListWorkspaces() ([]Workspace, error) {
	rs, err := s.rows(s.db, `SELECT w.id, w.name, w.revision, w.created_at,
		(SELECT COUNT(*) FROM memberships m WHERE m.workspace_id = w.id),
		(SELECT COUNT(*) FROM items i WHERE i.workspace_id = w.id AND i.deleted = 0)
		FROM workspaces w ORDER BY w.name`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []Workspace{}
	for rs.Next() {
		var w Workspace
		if err := rs.Scan(&w.ID, &w.Name, &w.Revision, &w.CreatedAt, &w.Members, &w.Items); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rs.Err()
}

func (s *Store) CreateWorkspace(name string) (*Workspace, error) {
	w := &Workspace{ID: auth.ID("w_"), Name: strings.TrimSpace(name), CreatedAt: now()}
	_, err := s.exec(s.db, `INSERT INTO workspaces (id, name, revision, created_at) VALUES (?, ?, 0, ?)`, w.ID, w.Name, w.CreatedAt)
	return w, err
}

func (s *Store) RenameWorkspace(id, name string) error {
	r, err := s.exec(s.db, `UPDATE workspaces SET name = ? WHERE id = ?`, strings.TrimSpace(name), id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteWorkspace(id string) error {
	r, err := s.exec(s.db, `DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ tokens

type Token struct {
	ID         string `json:"id"`
	MemberID   string `json:"memberId"`
	MemberName string `json:"memberName"`
	Device     string `json:"device"`
	Prefix     string `json:"prefix"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiresAt  *int64 `json:"expiresAt"`
	LastUsedAt *int64 `json:"lastUsedAt"`
	RevokedAt  *int64 `json:"revokedAt"`
	// Scope: the workspaces this token may use; null = every workspace of its member.
	Scope []ScopeWorkspace `json:"scope"`
}

// ScopeWorkspace is one workspace of a token's or an invite's scope.
type ScopeWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// scopes reads token_workspaces or invite_workspaces (owner column token_id / invite_id) into owner id → workspaces.
func (s *Store) scopes(q db.Querier, table, owner string) (map[string][]ScopeWorkspace, error) {
	rs, err := s.rows(q, `SELECT x.`+owner+`, w.id, w.name FROM `+table+` x JOIN workspaces w ON w.id = x.workspace_id ORDER BY w.name`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string][]ScopeWorkspace{}
	for rs.Next() {
		var id string
		var w ScopeWorkspace
		if err := rs.Scan(&id, &w.ID, &w.Name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], w)
	}
	return out, rs.Err()
}

// scopeOf is the scope of a scoped token / invite: never nil, so an emptied scope stays "no workspace".
func scopeOf(all map[string][]ScopeWorkspace, id string) []ScopeWorkspace {
	if ws := all[id]; ws != nil {
		return ws
	}
	return []ScopeWorkspace{}
}

func (s *Store) ListTokens() ([]Token, error) {
	rs, err := s.rows(s.db, `SELECT t.id, t.member_id, m.name, t.device, t.prefix, t.created_at, t.expires_at, t.last_used_at, t.revoked_at, t.scoped
		FROM tokens t JOIN members m ON m.id = t.member_id ORDER BY t.revoked_at IS NOT NULL, m.name, t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	out := []Token{}
	scoped := map[string]bool{}
	for rs.Next() {
		var t Token
		var exp, used, rev sql.NullInt64
		var sc int
		if err := rs.Scan(&t.ID, &t.MemberID, &t.MemberName, &t.Device, &t.Prefix, &t.CreatedAt, &exp, &used, &rev, &sc); err != nil {
			rs.Close()
			return nil, err
		}
		t.ExpiresAt, t.LastUsedAt, t.RevokedAt = nullable(exp), nullable(used), nullable(rev)
		scoped[t.ID] = sc == 1
		out = append(out, t)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	all, err := s.scopes(s.db, "token_workspaces", "token_id")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if scoped[out[i].ID] {
			out[i].Scope = scopeOf(all, out[i].ID)
		}
	}
	return out, nil
}

func expiry(days int) any {
	if days <= 0 {
		return nil
	}
	return now() + int64(days)*24*time.Hour.Milliseconds()
}

// CreateToken issues a token for a member's device; the plain token is returned once and never stored. scope nil
// = every workspace of the member; otherwise only those (an empty, non-nil scope allows none).
func (s *Store) CreateToken(q db.Querier, memberID, device string, expiryDays int, scope []string) (string, *Token, error) {
	plain, hash, prefix := auth.NewToken()
	t := &Token{ID: auth.ID("t_"), MemberID: memberID, Device: strings.TrimSpace(device), Prefix: prefix, CreatedAt: now()}
	if t.Device == "" {
		t.Device = "Device"
	}
	exp := expiry(expiryDays)
	if v, ok := exp.(int64); ok {
		t.ExpiresAt = &v
	}
	scoped := 0
	if scope != nil {
		scoped = 1
	}
	if _, err := s.exec(q, `INSERT INTO tokens (id, member_id, device, prefix, hash, created_at, expires_at, scoped) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, t.ID, memberID, t.Device, prefix, hash, t.CreatedAt, exp, scoped); err != nil {
		return "", nil, err
	}
	for _, ws := range scope {
		if _, err := s.exec(q, `INSERT INTO token_workspaces (token_id, workspace_id) VALUES (?, ?)`, t.ID, ws); err != nil {
			return "", nil, err
		}
	}
	return plain, t, nil
}

func (s *Store) RevokeToken(id string) error {
	r, err := s.exec(s.db, `UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now(), id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Caller is who a request's token belongs to.
type Caller struct {
	TokenID string
	Device  string
	Member  Member
	// Scoped: the token may only use the workspaces in Scope (else every workspace of the member).
	Scoped bool
	Scope  map[string]bool
}

// May tells whether the token's scope covers the workspace (membership is checked separately).
func (c *Caller) May(workspaceID string) bool { return !c.Scoped || c.Scope[workspaceID] }

func (s *Store) loadScope(q db.Querier, table, owner, id string) (map[string]bool, error) {
	rs, err := s.rows(q, `SELECT workspace_id FROM `+table+` WHERE `+owner+` = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]bool{}
	for rs.Next() {
		var ws string
		if err := rs.Scan(&ws); err != nil {
			return nil, err
		}
		out[ws] = true
	}
	return out, rs.Err()
}

// TokenCaller finds a live token (not revoked, not expired, member not disabled) and notes its use.
func (s *Store) TokenCaller(plain string) (*Caller, error) {
	if !strings.HasPrefix(plain, auth.TokenPrefix) {
		return nil, ErrNotFound
	}
	c := &Caller{}
	var disabled, scoped int
	var last sql.NullInt64
	t := now()
	err := s.row(s.db, `SELECT t.id, t.device, t.last_used_at, t.scoped, m.id, m.name, m.email, m.avatar_url, m.disabled
		FROM tokens t JOIN members m ON m.id = t.member_id
		WHERE t.hash = ? AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?)`, auth.Hash(plain), t).
		Scan(&c.TokenID, &c.Device, &last, &scoped, &c.Member.ID, &c.Member.Name, &c.Member.Email, &c.Member.AvatarURL, &disabled)
	if errors.Is(err, sql.ErrNoRows) || disabled == 1 {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Member.fillAvatar()
	if c.Scoped = scoped == 1; c.Scoped {
		if c.Scope, err = s.loadScope(s.db, "token_workspaces", "token_id", c.TokenID); err != nil {
			return nil, err
		}
	}
	// "Last used" to the minute is enough; it saves a write per request.
	if !last.Valid || t-last.Int64 > 60_000 {
		_, _ = s.exec(s.db, `UPDATE tokens SET last_used_at = ? WHERE id = ?`, t, c.TokenID)
	}
	return c, nil
}

// ------------------------------------------------------------------ invites

type Invite struct {
	ID         string `json:"id"`
	MemberID   string `json:"memberId"`
	MemberName string `json:"memberName"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	UsedAt     *int64 `json:"usedAt"`
	// Scope the device token will get; null = every workspace of the member.
	Scope []ScopeWorkspace `json:"scope"`
}

// CreateInvite makes a one-time code; scope as for CreateToken (the token made from the invite inherits it).
func (s *Store) CreateInvite(memberID string, validHours, tokenExpiryDays int, scope []string) (string, *Invite, error) {
	code, hash := auth.NewInviteCode()
	t := now()
	inv := &Invite{ID: auth.ID("i_"), MemberID: memberID, CreatedAt: t, ExpiresAt: t + int64(validHours)*time.Hour.Milliseconds()}
	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	scoped := 0
	if scope != nil {
		scoped = 1
	}
	if _, err := s.exec(tx, `INSERT INTO invites (id, member_id, code_hash, token_expiry_days, created_at, expires_at, scoped) VALUES (?, ?, ?, ?, ?, ?, ?)`, inv.ID, memberID, hash, tokenExpiryDays, inv.CreatedAt, inv.ExpiresAt, scoped); err != nil {
		return "", nil, err
	}
	for _, ws := range scope {
		if _, err := s.exec(tx, `INSERT INTO invite_workspaces (invite_id, workspace_id) VALUES (?, ?)`, inv.ID, ws); err != nil {
			return "", nil, err
		}
	}
	return code, inv, tx.Commit()
}

func (s *Store) ListInvites() ([]Invite, error) {
	rs, err := s.rows(s.db, `SELECT i.id, i.member_id, m.name, i.created_at, i.expires_at, i.used_at, i.scoped FROM invites i JOIN members m ON m.id = i.member_id
		WHERE i.used_at IS NULL AND i.expires_at > ? ORDER BY i.created_at DESC`, now())
	if err != nil {
		return nil, err
	}
	out := []Invite{}
	scoped := map[string]bool{}
	for rs.Next() {
		var i Invite
		var used sql.NullInt64
		var sc int
		if err := rs.Scan(&i.ID, &i.MemberID, &i.MemberName, &i.CreatedAt, &i.ExpiresAt, &used, &sc); err != nil {
			rs.Close()
			return nil, err
		}
		i.UsedAt = nullable(used)
		scoped[i.ID] = sc == 1
		out = append(out, i)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	all, err := s.scopes(s.db, "invite_workspaces", "invite_id")
	if err != nil {
		return nil, err
	}
	for i := range out {
		if scoped[out[i].ID] {
			out[i].Scope = scopeOf(all, out[i].ID)
		}
	}
	return out, nil
}

func (s *Store) DeleteInvite(id string) error {
	_, err := s.exec(s.db, `DELETE FROM invites WHERE id = ? AND used_at IS NULL`, id)
	return err
}

// RedeemInvite exchanges a one-time code for a new device token, in one transaction (a code works once).
func (s *Store) RedeemInvite(code, device string) (string, *Caller, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	var id, memberID string
	var days, scoped int
	err = s.row(tx, `SELECT id, member_id, token_expiry_days, scoped FROM invites WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, auth.Hash(code), now()).Scan(&id, &memberID, &days, &scoped)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	// The token inherits the invite's scope.
	var scope []string
	var scopeSet map[string]bool
	if scoped == 1 {
		if scopeSet, err = s.loadScope(tx, "invite_workspaces", "invite_id", id); err != nil {
			return "", nil, err
		}
		scope = []string{}
		for ws := range scopeSet {
			scope = append(scope, ws)
		}
	}
	plain, tok, err := s.CreateToken(tx, memberID, device, days, scope)
	if err != nil {
		return "", nil, err
	}
	r, err := s.exec(tx, `UPDATE invites SET used_at = ?, token_id = ? WHERE id = ? AND used_at IS NULL`, now(), tok.ID, id)
	if err != nil {
		return "", nil, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return "", nil, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return "", nil, err
	}
	m, err := s.GetMember(memberID)
	if err != nil {
		return "", nil, err
	}
	if m.Disabled {
		return "", nil, ErrNotFound
	}
	return plain, &Caller{TokenID: tok.ID, Device: tok.Device, Member: *m, Scoped: scoped == 1, Scope: scopeSet}, nil
}

// ------------------------------------------------------------------ audit

type AuditEntry struct {
	ID     string `json:"id"`
	At     int64  `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

func (s *Store) Audit(actor, action, detail string) {
	_, _ = s.exec(s.db, `INSERT INTO audit_log (id, at, actor, action, detail) VALUES (?, ?, ?, ?, ?)`, auth.ID("l_"), now(), actor, action, detail)
}

// AuditKinds groups the activity log's actions for the panel's filter (by action prefix).
var AuditKinds = map[string][]string{
	"members":    {"member."},
	"workspaces": {"workspace."},
	"devices":    {"token.", "invite.", "device."},
	"admins":     {"admin.", "passkey."},
	"server":     {"server."},
}

// AuditFilter narrows the activity log: a kind from AuditKinds and words to find in who / what / detail.
type AuditFilter struct {
	Kind   string
	Search string
}

func (f AuditFilter) where() (string, []any) {
	conds := []string{}
	args := []any{}
	if prefixes, ok := AuditKinds[f.Kind]; ok {
		or := []string{}
		for _, p := range prefixes {
			or = append(or, `action LIKE ?`)
			args = append(args, p+"%")
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+")")
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(q)) + "%"
		conds = append(conds, `(LOWER(actor) LIKE ? ESCAPE '\' OR LOWER(action) LIKE ? ESCAPE '\' OR LOWER(detail) LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return strings.Join(conds, " AND "), args
}

// ListAudit pages through the activity log, newest first: `before` is the last entry of the previous page
// (empty for the first), so a page stays fast however long the log gets.
func (s *Store) ListAudit(f AuditFilter, limit int, beforeAt int64, beforeID string) ([]AuditEntry, error) {
	where, args := f.where()
	if beforeID != "" {
		if where != "" {
			where += " AND "
		}
		where += `(at < ? OR (at = ? AND id < ?))`
		args = append(args, beforeAt, beforeAt, beforeID)
	}
	q := `SELECT id, at, actor, action, detail FROM audit_log`
	if where != "" {
		q += ` WHERE ` + where
	}
	rs, err := s.rows(s.db, q+` ORDER BY at DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []AuditEntry{}
	for rs.Next() {
		var a AuditEntry
		if err := rs.Scan(&a.ID, &a.At, &a.Actor, &a.Action, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rs.Err()
}

// AuditCount counts the entries the filter matches.
func (s *Store) AuditCount(f AuditFilter) (int, error) {
	where, args := f.where()
	q := `SELECT COUNT(*) FROM audit_log`
	if where != "" {
		q += ` WHERE ` + where
	}
	var n int
	err := s.row(s.db, q, args...).Scan(&n)
	return n, err
}

// AuditKindOf is the AuditKinds group an action belongs to ("other" for none).
func AuditKindOf(action string) string {
	for k, prefixes := range AuditKinds {
		for _, p := range prefixes {
			if strings.HasPrefix(action, p) {
				return k
			}
		}
	}
	return "other"
}

// EachAudit calls fn for every entry with from <= at < to, oldest first (who and what only: for the activity charts).
func (s *Store) EachAudit(from, to int64, fn func(at int64, actor, action string)) error {
	rs, err := s.rows(s.db, `SELECT at, actor, action FROM audit_log WHERE at >= ? AND at < ? ORDER BY at`, from, to)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		var at int64
		var actor, action string
		if err := rs.Scan(&at, &actor, &action); err != nil {
			return err
		}
		fn(at, actor, action)
	}
	return rs.Err()
}

// AuditCountBetween counts the entries with from <= at < to.
func (s *Store) AuditCountBetween(from, to int64) (int, error) {
	var n int
	err := s.row(s.db, `SELECT COUNT(*) FROM audit_log WHERE at >= ? AND at < ?`, from, to).Scan(&n)
	return n, err
}
