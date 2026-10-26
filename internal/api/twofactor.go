package api

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/gin-gonic/gin"
)

// Two-step sign-in for admins: TOTP codes from an authenticator app, plus ten one-time recovery codes.
// Signing in (password, or Google / GitHub) with it on gives a ticket; POST /login/code with the ticket and a
// code finishes the sign-in.

const (
	ticketTTL    = 5 * time.Minute
	ticketTries  = 5
	ticketCookie = "dialcatz_ticket"
)

// tickets are half-done sign-ins, in memory: short-lived, single-use, bound to one admin, a few tries each.
type tickets struct {
	mu sync.Mutex
	m  map[string]*ticket // hash of the ticket → ticket
}
type ticket struct {
	adminID string
	via     string
	expires time.Time
	misses  int
}

func newTickets() *tickets { return &tickets{m: map[string]*ticket{}} }

func (t *tickets) issue(adminID, via string) string {
	plain := auth.Random(24)
	t.mu.Lock()
	defer t.mu.Unlock()
	n := time.Now()
	for k, v := range t.m {
		if n.After(v.expires) {
			delete(t.m, k)
		}
	}
	t.m[auth.Hash(plain)] = &ticket{adminID: adminID, via: via, expires: n.Add(ticketTTL)}
	return plain
}

// get returns the live ticket's admin and how they started signing in ("" when unknown or expired).
func (t *tickets) get(plain string) (adminID, via string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tk := t.m[auth.Hash(plain)]
	if tk == nil || time.Now().After(tk.expires) {
		return "", ""
	}
	return tk.adminID, tk.via
}

// miss counts a wrong code; the ticket is gone after a few.
func (t *tickets) miss(plain string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tk := t.m[auth.Hash(plain)]; tk != nil {
		if tk.misses++; tk.misses >= ticketTries {
			delete(t.m, auth.Hash(plain))
		}
	}
}

// take uses the ticket up; false when another request took it first.
func (t *tickets) take(plain string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	h := auth.Hash(plain)
	if t.m[h] == nil {
		return false
	}
	delete(t.m, h)
	return true
}

// secondFactor checks an authenticator code (each works once) or a recovery code (spent).
func (s *Server) secondFactor(adminID, code string) (bool, string, error) {
	st, err := s.store.AdminTOTP(adminID)
	if err != nil || !st.Enabled {
		return false, "", err
	}
	secret, err := s.sealer.Open(st.Secret)
	if err != nil {
		return false, "", err
	}
	if step := auth.CheckTOTP(secret, code, time.Now()); step > 0 {
		ok, err := s.store.UseTOTPStep(adminID, step)
		return ok, "code", err
	}
	if len(strings.TrimSpace(code)) >= 10 {
		ok, err := s.store.UseRecoveryCode(adminID, auth.RecoveryHash(code))
		return ok, "recovery code", err
	}
	return false, "", nil
}

func (s *Server) loginCode(c *gin.Context) {
	var in struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if !bind(c, &in) {
		return
	}
	// Google / GitHub sign-in hands the ticket over in a cookie instead of a JSON answer.
	if in.Ticket == "" {
		in.Ticket, _ = c.Cookie(ticketCookie)
	}
	adminID, via := s.tickets.get(in.Ticket)
	if adminID == "" {
		fail(c, http.StatusUnauthorized, "This sign-in has expired. Start again.")
		return
	}
	ok, kind, err := s.secondFactor(adminID, in.Code)
	if err != nil {
		internal(c, err)
		return
	}
	if !ok {
		s.tickets.miss(in.Ticket)
		fail(c, http.StatusUnauthorized, "That code is not right (or was used already)")
		return
	}
	if !s.tickets.take(in.Ticket) {
		fail(c, http.StatusUnauthorized, "This sign-in has expired. Start again.")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: ticketCookie, Value: "", Path: "/api/admin", MaxAge: -1, HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
	a, err := s.store.GetAdmin(adminID)
	if err != nil {
		storeErr(c, err, "No such admin")
		return
	}
	if s.startSession(c, a, via+" + "+kind) {
		c.JSON(http.StatusOK, a)
	}
}

func (s *Server) twoFactorState(c *gin.Context) {
	a := admin(c)
	left, err := s.store.RecoveryCodesLeft(a.ID)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": a.TwoFactor, "recoveryCodesLeft": left})
}

// twoFactorStart makes a new secret (kept pending until confirmed) and returns it with its otpauth:// URI.
func (s *Server) twoFactorStart(c *gin.Context) {
	a := admin(c)
	if a.TwoFactor {
		fail(c, http.StatusConflict, "Two-step sign-in is already on")
		return
	}
	secret := auth.NewTOTPSecret()
	if err := s.store.SetTOTPPending(a.ID, s.sealer.Seal(secret)); err != nil {
		internal(c, err)
		return
	}
	issuer := "DialCatz Den"
	if u, err := url.Parse(s.cfg.PublicURL); err == nil && u.Host != "" {
		issuer += " (" + u.Host + ")"
	}
	c.JSON(http.StatusOK, gin.H{"secret": secret, "uri": auth.TOTPURI(secret, issuer, a.Email)})
}

// twoFactorConfirm turns it on once a code from the app matches, and returns the recovery codes (shown once).
func (s *Server) twoFactorConfirm(c *gin.Context) {
	var in struct {
		Code string `json:"code"`
	}
	if !bind(c, &in) {
		return
	}
	a := admin(c)
	st, err := s.store.AdminTOTP(a.ID)
	if err != nil {
		internal(c, err)
		return
	}
	if st.Enabled || st.Pending == "" {
		fail(c, http.StatusConflict, "Start setting up two-step sign-in first")
		return
	}
	secret, err := s.sealer.Open(st.Pending)
	if err != nil {
		internal(c, err)
		return
	}
	step := auth.CheckTOTP(secret, in.Code, time.Now())
	if step == 0 {
		fail(c, http.StatusBadRequest, "That code does not match. Check the clock of the phone and try the newest code.")
		return
	}
	codes := auth.NewRecoveryCodes(10)
	hashes := make([]string, len(codes))
	for i, rc := range codes {
		hashes[i] = auth.RecoveryHash(rc)
	}
	if err := s.store.EnableTOTP(a.ID, st.Pending, step, hashes); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "admin.2fa.on", "")
	c.JSON(http.StatusOK, gin.H{"recoveryCodes": codes})
}

// twoFactorDisable needs the password and a code (or a recovery code).
func (s *Server) twoFactorDisable(c *gin.Context) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !bind(c, &in) {
		return
	}
	a := admin(c)
	if _, err := s.store.CheckAdmin(a.Email, in.Password); err != nil {
		fail(c, http.StatusUnauthorized, "The password is wrong")
		return
	}
	ok, _, err := s.secondFactor(a.ID, in.Code)
	if err != nil {
		internal(c, err)
		return
	}
	if !ok {
		fail(c, http.StatusUnauthorized, "That code is not right (or was used already)")
		return
	}
	if err := s.store.DisableTOTP(a.ID); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "admin.2fa.off", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
