package api

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
)

// The admin panel's API. Members never log in; the admin does (email + password made at first setup), and
// issues members' device tokens and one-time invite links.

const (
	sessionCookie = "dialcatz_admin"
	adminKey      = "admin"
)

// csrf: admin calls must carry X-DialCatz (a custom header a cross-site form cannot send) on top of the
// SameSite=Strict cookie.
func (s *Server) csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.GetHeader("X-DialCatz") != "1" {
			fail(c, http.StatusForbidden, "Missing X-DialCatz header")
			return
		}
		c.Next()
	}
}

func (s *Server) setSession(c *gin.Context, sid string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: sid, Path: "/api/admin", MaxAge: maxAge, HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
}

func (s *Server) sessionAdmin(c *gin.Context) *store.Admin {
	sid, err := c.Cookie(sessionCookie)
	if err != nil || sid == "" {
		return nil
	}
	a, err := s.store.SessionAdmin(sid)
	if err != nil {
		return nil
	}
	return a
}

func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		a := s.sessionAdmin(c)
		if a == nil {
			fail(c, http.StatusUnauthorized, "Please sign in")
			return
		}
		c.Set(adminKey, a)
		c.Next()
	}
}

func admin(c *gin.Context) *store.Admin { return c.MustGet(adminKey).(*store.Admin) }
func (s *Server) audit(c *gin.Context, action, detail string) {
	s.store.Audit("admin:"+admin(c).Email, action, detail)
}

func (s *Server) adminState(c *gin.Context) {
	n, err := s.store.AdminCount()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"needsSetup": n == 0, "admin": s.sessionAdmin(c), "publicUrl": s.cfg.PublicURL, "oauth": s.oauthEnabled(), "passkeys": s.webauthn != nil})
}

// startSession signs the admin in on this browser.
func (s *Server) startSession(c *gin.Context, a *store.Admin, how string) bool {
	sid, err := s.store.CreateSession(a.ID)
	if err != nil {
		internal(c, err)
		return false
	}
	s.setSession(c, sid, int((7 * 24 * time.Hour).Seconds()))
	s.store.Audit("admin:"+a.Email, "admin.login", how+" "+c.ClientIP())
	return true
}

type credentials struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// setup creates the first admin; it only works while there is none.
func (s *Server) setup(c *gin.Context) {
	var in credentials
	if !bind(c, &in) {
		return
	}
	if n, err := s.store.AdminCount(); err != nil || n > 0 {
		fail(c, http.StatusConflict, "This server is already set up. Sign in instead.")
		return
	}
	if !validEmail(in.Email) || clean(in.Name, 120) == "" {
		fail(c, http.StatusBadRequest, "Enter your name and a valid email")
		return
	}
	if err := auth.CheckStrength(in.Password); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a, err := s.store.CreateAdmin(in.Email, clean(in.Name, 120), in.Password)
	if err != nil {
		storeErr(c, err, "")
		return
	}
	sid, err := s.store.CreateSession(a.ID)
	if err != nil {
		internal(c, err)
		return
	}
	s.setSession(c, sid, int((7 * 24 * time.Hour).Seconds()))
	s.store.Audit("admin:"+a.Email, "server.setup", "")
	c.JSON(http.StatusOK, a)
}

func (s *Server) login(c *gin.Context) {
	var in credentials
	if !bind(c, &in) {
		return
	}
	a, err := s.store.CheckAdmin(in.Email, in.Password)
	if err != nil {
		fail(c, http.StatusUnauthorized, "Wrong email or password")
		return
	}
	// Two-step sign-in on: the password alone only earns a ticket for the code step (POST /login/code).
	if a.TwoFactor {
		c.JSON(http.StatusOK, gin.H{"needsCode": true, "ticket": s.tickets.issue(a.ID, "password")})
		return
	}
	if s.startSession(c, a, "password") {
		c.JSON(http.StatusOK, a)
	}
}

func (s *Server) logout(c *gin.Context) {
	if sid, err := c.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(sid)
	}
	s.setSession(c, "", -1)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) changePassword(c *gin.Context) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !bind(c, &in) {
		return
	}
	a := admin(c)
	if _, err := s.store.CheckAdmin(a.Email, in.Current); err != nil {
		fail(c, http.StatusUnauthorized, "The current password is wrong")
		return
	}
	if err := auth.CheckStrength(in.New); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetAdminPassword(a.ID, in.New); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "admin.password", "")
	// Every session ended with the change; start a fresh one here.
	sid, err := s.store.CreateSession(a.ID)
	if err == nil {
		s.setSession(c, sid, int((7 * 24 * time.Hour).Seconds()))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ------------------------------------------------------------------ members

type memberIn struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
	Disabled  bool   `json:"disabled"`
}

func (in *memberIn) check(c *gin.Context) bool {
	in.Name, in.Email, in.AvatarURL = clean(in.Name, 120), clean(in.Email, 200), clean(in.AvatarURL, 500)
	if in.Name == "" || !validEmail(in.Email) {
		fail(c, http.StatusBadRequest, "Enter a name and a valid email")
		return false
	}
	if !validURL(in.AvatarURL) {
		fail(c, http.StatusBadRequest, "The avatar must be an http(s) URL")
		return false
	}
	return true
}

func (s *Server) listMembers(c *gin.Context) {
	ms, err := s.store.ListMembers()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, ms)
}

func (s *Server) createMember(c *gin.Context) {
	var in memberIn
	if !bind(c, &in) || !in.check(c) {
		return
	}
	m, err := s.store.CreateMember(in.Name, in.Email, in.AvatarURL)
	if err != nil {
		storeErr(c, err, "")
		return
	}
	s.audit(c, "member.created", m.Name+" <"+m.Email+">")
	c.JSON(http.StatusOK, m)
}

func (s *Server) updateMember(c *gin.Context) {
	var in memberIn
	if !bind(c, &in) || !in.check(c) {
		return
	}
	if err := s.store.UpdateMember(c.Param("id"), in.Name, in.Email, in.AvatarURL, in.Disabled); err != nil {
		storeErr(c, err, "No such member")
		return
	}
	s.audit(c, "member.updated", in.Name)
	m, _ := s.store.GetMember(c.Param("id"))
	c.JSON(http.StatusOK, m)
}

func (s *Server) deleteMember(c *gin.Context) {
	m, err := s.store.GetMember(c.Param("id"))
	if err != nil {
		storeErr(c, err, "No such member")
		return
	}
	if err := s.store.DeleteMember(m.ID); err != nil {
		storeErr(c, err, "No such member")
		return
	}
	s.audit(c, "member.deleted", m.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) setMembership(c *gin.Context) {
	var in struct {
		Role string `json:"role"`
	}
	if !bind(c, &in) {
		return
	}
	if in.Role != "" && !store.Roles[in.Role] {
		fail(c, http.StatusBadRequest, "Role must be owner, editor or viewer")
		return
	}
	if err := s.store.SetMembership(c.Param("ws"), c.Param("id"), in.Role); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "member.role", c.Param("id")+" @ "+c.Param("ws")+" = "+in.Role)
	m, _ := s.store.GetMember(c.Param("id"))
	c.JSON(http.StatusOK, m)
}

// ------------------------------------------------------------------ workspaces

func (s *Server) listWorkspaces(c *gin.Context) {
	ws, err := s.store.ListWorkspaces()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (s *Server) createWorkspace(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if !bind(c, &in) {
		return
	}
	if in.Name = clean(in.Name, 120); in.Name == "" {
		fail(c, http.StatusBadRequest, "Give the workspace a name")
		return
	}
	w, err := s.store.CreateWorkspace(in.Name)
	if err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "workspace.created", w.Name)
	c.JSON(http.StatusOK, w)
}

func (s *Server) renameWorkspace(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if !bind(c, &in) {
		return
	}
	if in.Name = clean(in.Name, 120); in.Name == "" {
		fail(c, http.StatusBadRequest, "Give the workspace a name")
		return
	}
	if err := s.store.RenameWorkspace(c.Param("id"), in.Name); err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	s.audit(c, "workspace.renamed", in.Name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) deleteWorkspace(c *gin.Context) {
	if err := s.store.DeleteWorkspace(c.Param("id")); err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	s.audit(c, "workspace.deleted", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ------------------------------------------------------------------ tokens and invites

func (s *Server) listTokens(c *gin.Context) {
	ts, err := s.store.ListTokens()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, ts)
}

func (s *Server) createToken(c *gin.Context) {
	var in struct {
		MemberID     string   `json:"memberId"`
		Device       string   `json:"device"`
		ExpiryDays   int      `json:"expiryDays"`
		WorkspaceIDs []string `json:"workspaceIds"`
	}
	if !bind(c, &in) {
		return
	}
	m, err := s.store.GetMember(in.MemberID)
	if err != nil {
		storeErr(c, err, "No such member")
		return
	}
	if in.ExpiryDays < 0 || in.ExpiryDays > 3650 {
		fail(c, http.StatusBadRequest, "Expiry must be 0 (never) to 3650 days")
		return
	}
	scope, named, ok := scopeFor(c, m, in.WorkspaceIDs)
	if !ok {
		return
	}
	plain, t, err := s.store.CreateToken(s.store.DB(), m.ID, clean(in.Device, 80), in.ExpiryDays, scope)
	if err != nil {
		internal(c, err)
		return
	}
	t.MemberName, t.Scope = m.Name, named
	s.audit(c, "token.created", m.Name+" – "+t.Device)
	// The only time the token is shown.
	c.JSON(http.StatusOK, gin.H{"token": plain, "info": t})
}

func (s *Server) revokeToken(c *gin.Context) {
	if err := s.store.RevokeToken(c.Param("id")); err != nil {
		storeErr(c, err, "No such active token")
		return
	}
	s.audit(c, "token.revoked", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) listInvites(c *gin.Context) {
	is, err := s.store.ListInvites()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, is)
}

func (s *Server) createInvite(c *gin.Context) {
	var in struct {
		MemberID        string   `json:"memberId"`
		ValidHours      int      `json:"validHours"`
		TokenExpiryDays int      `json:"tokenExpiryDays"`
		WorkspaceIDs    []string `json:"workspaceIds"`
	}
	if !bind(c, &in) {
		return
	}
	m, err := s.store.GetMember(in.MemberID)
	if err != nil {
		storeErr(c, err, "No such member")
		return
	}
	if in.ValidHours <= 0 || in.ValidHours > 24*14 {
		in.ValidHours = 24
	}
	if in.TokenExpiryDays < 0 || in.TokenExpiryDays > 3650 {
		in.TokenExpiryDays = 0
	}
	scope, named, ok := scopeFor(c, m, in.WorkspaceIDs)
	if !ok {
		return
	}
	code, inv, err := s.store.CreateInvite(m.ID, in.ValidHours, in.TokenExpiryDays, scope)
	if err != nil {
		internal(c, err)
		return
	}
	inv.MemberName, inv.Scope = m.Name, named
	link := "dialcatz://connect?server=" + url.QueryEscape(s.cfg.PublicURL) + "&code=" + url.QueryEscape(code)
	s.audit(c, "invite.created", m.Name+" ("+strconv.Itoa(in.ValidHours)+" h)")
	c.JSON(http.StatusOK, gin.H{"link": link, "code": code, "server": s.cfg.PublicURL, "invite": inv})
}

// scopeFor checks a token's / invite's workspaceIds: none = every workspace of the member (nil scope), otherwise
// each must be one the member belongs to. Returns the ids and the same with names, for the answer.
func scopeFor(c *gin.Context, m *store.Member, ids []string) ([]string, []store.ScopeWorkspace, bool) {
	if len(ids) == 0 {
		return nil, nil, true
	}
	names := map[string]string{}
	for _, w := range m.Workspaces {
		names[w.WorkspaceID] = w.Name
	}
	seen := map[string]bool{}
	scope := []string{}
	named := []store.ScopeWorkspace{}
	for _, id := range ids {
		name, ok := names[id]
		if !ok {
			fail(c, http.StatusBadRequest, "Pick only workspaces "+m.Name+" belongs to")
			return nil, nil, false
		}
		if !seen[id] {
			seen[id] = true
			scope = append(scope, id)
			named = append(named, store.ScopeWorkspace{ID: id, Name: name})
		}
	}
	return scope, named, true
}

func (s *Server) deleteInvite(c *gin.Context) {
	if err := s.store.DeleteInvite(c.Param("id")); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "invite.deleted", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// listAudit (GET /audit?kind=&q=&limit=&beforeAt=&beforeId=) is one page of the activity log, newest first,
// filtered by kind (members, workspaces, devices, admins, server) and words, with the matching total and whether
// older entries follow.
func (s *Server) listAudit(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	f := store.AuditFilter{Kind: c.Query("kind"), Search: clean(c.Query("q"), 200)}
	beforeAt, _ := strconv.ParseInt(c.Query("beforeAt"), 10, 64)
	entries, err := s.store.ListAudit(f, limit+1, beforeAt, c.Query("beforeId"))
	if err != nil {
		internal(c, err)
		return
	}
	total, err := s.store.AuditCount(f)
	if err != nil {
		internal(c, err)
		return
	}
	more := len(entries) > limit
	if more {
		entries = entries[:limit]
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "more": more, "total": total})
}
