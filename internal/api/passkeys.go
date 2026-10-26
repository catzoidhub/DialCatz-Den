package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Passkeys (WebAuthn) for admins: register one or more on the Account page, then sign in with one instead of an
// email and password. A passkey sign-in is complete on its own — it is phishing-resistant and the device checks
// the person (user verification required) — so two-step codes are not asked for after it. Passkeys are bound to
// the RP ID, the host of DIALCATZ_PUBLIC_URL.

const ceremonyTTL = 5 * time.Minute

// ceremonies are the begun-but-not-finished registrations and sign-ins, in memory, single use.
type ceremonies struct {
	mu sync.Mutex
	m  map[string]*ceremony
}
type ceremony struct {
	adminID string // registration only
	data    webauthn.SessionData
	expires time.Time
}

func newCeremonies() *ceremonies { return &ceremonies{m: map[string]*ceremony{}} }

func (c *ceremonies) put(adminID string, data *webauthn.SessionData) string {
	plain := auth.Random(24)
	c.mu.Lock()
	defer c.mu.Unlock()
	n := time.Now()
	for k, v := range c.m {
		if n.After(v.expires) {
			delete(c.m, k)
		}
	}
	c.m[auth.Hash(plain)] = &ceremony{adminID: adminID, data: *data, expires: n.Add(ceremonyTTL)}
	return plain
}

// take returns the ceremony and forgets it (a challenge answers once).
func (c *ceremonies) take(plain string) *ceremony {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := auth.Hash(plain)
	cer := c.m[h]
	delete(c.m, h)
	if cer == nil || time.Now().After(cer.expires) {
		return nil
	}
	return cer
}

// newWebAuthn sets up WebAuthn for the public URL; nil (passkeys off) when the URL cannot carry them.
func newWebAuthn(publicURL string) *webauthn.WebAuthn {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	host := u.Hostname()
	// WebAuthn needs a domain (or localhost) in a secure context: no bare IP addresses, no plain http elsewhere.
	if net.ParseIP(host) != nil || (u.Scheme != "https" && host != "localhost" && !strings.HasSuffix(host, ".localhost")) {
		slog.Info("passkeys are off: they need an https:// public URL on a domain name", "public", publicURL)
		return nil
	}
	w, err := webauthn.New(&webauthn.Config{
		RPID:          host,
		RPDisplayName: "DialCatz Den",
		RPOrigins:     []string{u.Scheme + "://" + u.Host},
	})
	if err != nil {
		slog.Warn("passkeys are off", "err", err)
		return nil
	}
	return w
}

// passkeyUser is an admin as go-webauthn sees them.
type passkeyUser struct {
	admin  *store.Admin
	handle []byte
	creds  []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.handle }
func (u *passkeyUser) WebAuthnName() string                       { return u.admin.Email }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.admin.Name }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Server) passkeyUser(a *store.Admin) (*passkeyUser, error) {
	h, err := s.store.AdminHandle(a.ID)
	if err != nil {
		return nil, err
	}
	keys, err := s.store.Passkeys(a.ID, s.webauthn.Config.RPID)
	if err != nil {
		return nil, err
	}
	u := &passkeyUser{admin: a, handle: []byte(h)}
	for _, k := range keys {
		var c webauthn.Credential
		if json.Unmarshal([]byte(k.Credential), &c) == nil {
			u.creds = append(u.creds, c)
		}
	}
	return u, nil
}

func credentialKey(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

func (s *Server) passkeysOn(c *gin.Context) bool {
	if s.webauthn == nil {
		fail(c, http.StatusNotFound, "Passkeys need the server on an https:// address with a domain name (DIALCATZ_PUBLIC_URL)")
		return false
	}
	return true
}

// ------------------------------------------------------------------ the Account page

func (s *Server) listPasskeys(c *gin.Context) {
	if s.webauthn == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "passkeys": []store.Passkey{}})
		return
	}
	keys, err := s.store.Passkeys(admin(c).ID, s.webauthn.Config.RPID)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "rpId": s.webauthn.Config.RPID, "passkeys": keys})
}

func (s *Server) passkeyRegisterBegin(c *gin.Context) {
	if !s.passkeysOn(c) {
		return
	}
	u, err := s.passkeyUser(admin(c))
	if err != nil {
		internal(c, err)
		return
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for i := range u.creds {
		exclude = append(exclude, u.creds[i].Descriptor())
	}
	creation, data, err := s.webauthn.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude),
	)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": s.ceremonies.put(u.admin.ID, data), "options": creation})
}

func (s *Server) passkeyRegisterFinish(c *gin.Context) {
	if !s.passkeysOn(c) {
		return
	}
	var in struct {
		Token      string          `json:"token"`
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if !bind(c, &in) {
		return
	}
	a := admin(c)
	cer := s.ceremonies.take(in.Token)
	if cer == nil || cer.adminID != a.ID {
		fail(c, http.StatusBadRequest, "This passkey setup has expired. Start again.")
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Credential)
	if err != nil {
		fail(c, http.StatusBadRequest, "The passkey answer could not be read")
		return
	}
	u, err := s.passkeyUser(a)
	if err != nil {
		internal(c, err)
		return
	}
	cred, err := s.webauthn.CreateCredential(u, cer.data, parsed)
	if err != nil {
		slog.Info("passkey registration refused", "admin", a.Email, "err", err)
		fail(c, http.StatusBadRequest, "The passkey could not be checked. Try again.")
		return
	}
	record, _ := json.Marshal(cred)
	name := clean(in.Name, 60)
	if name == "" {
		name = "Passkey"
	}
	p, err := s.store.AddPasskey(a.ID, s.webauthn.Config.RPID, credentialKey(cred.ID), string(record), name)
	if errors.Is(err, store.ErrConflict) {
		fail(c, http.StatusConflict, "This passkey is already registered")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "passkey.added", name)
	c.JSON(http.StatusOK, p)
}

func (s *Server) deletePasskey(c *gin.Context) {
	if err := s.store.DeletePasskey(admin(c).ID, c.Param("id")); err != nil {
		storeErr(c, err, "No such passkey")
		return
	}
	s.audit(c, "passkey.removed", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ------------------------------------------------------------------ sign in

func (s *Server) passkeyLoginBegin(c *gin.Context) {
	if !s.passkeysOn(c) {
		return
	}
	// Discoverable: the device offers its passkeys for this site, no email typed first.
	assertion, data, err := s.webauthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": s.ceremonies.put("", data), "options": assertion})
}

func (s *Server) passkeyLoginFinish(c *gin.Context) {
	if !s.passkeysOn(c) {
		return
	}
	var in struct {
		Token      string          `json:"token"`
		Credential json.RawMessage `json:"credential"`
	}
	if !bind(c, &in) {
		return
	}
	cer := s.ceremonies.take(in.Token)
	if cer == nil {
		fail(c, http.StatusUnauthorized, "This sign-in has expired. Start again.")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
	if err != nil {
		fail(c, http.StatusBadRequest, "The passkey answer could not be read")
		return
	}
	var who *passkeyUser
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		a, err := s.store.AdminByHandle(string(userHandle))
		if err != nil {
			return nil, err
		}
		u, err := s.passkeyUser(a)
		if err != nil {
			return nil, err
		}
		who = u
		return u, nil
	}
	_, cred, err := s.webauthn.ValidatePasskeyLogin(handler, cer.data, parsed)
	if err != nil || who == nil {
		slog.Info("passkey sign-in refused", "err", err)
		fail(c, http.StatusUnauthorized, "That passkey is not registered here (or was removed). Sign in with your password and add it on the Account page.")
		return
	}
	// A sign counter that went backwards means the passkey may have been copied: refuse.
	if cred.Authenticator.CloneWarning {
		s.store.Audit("admin:"+who.admin.Email, "passkey.clone-warning", credentialKey(cred.ID))
		fail(c, http.StatusUnauthorized, "This passkey looks copied (its counter went backwards). Remove it on the Account page and add it again.")
		return
	}
	record, _ := json.Marshal(cred)
	if err := s.store.PasskeyUsed(who.admin.ID, s.webauthn.Config.RPID, credentialKey(cred.ID), string(record)); err != nil {
		internal(c, err)
		return
	}
	if s.startSession(c, who.admin, "passkey") {
		c.JSON(http.StatusOK, who.admin)
	}
}
