// Package api is the HTTP surface: the sync API the DialCatz app uses (token = identity), the admin API behind
// the admin panel (email + password session), and the embedded admin panel itself.
package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/config"
	"github.com/catzoidhub/DialCatz-Den/internal/live"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
)

type Server struct {
	cfg     *config.Config
	store   *store.Store
	limiter *limiter
	// hub: live events and presence for the app's event streams.
	hub *live.Hub
	// sealer encrypts the admins' TOTP secrets at rest.
	sealer *auth.Sealer
	// tickets: the half-done sign-ins waiting for a two-step code.
	tickets *tickets
	// webauthn: passkeys for admins (nil when the public URL cannot carry them); ceremonies: begun, not finished.
	webauthn   *webauthn.WebAuthn
	ceremonies *ceremonies
}

func New(cfg *config.Config, st *store.Store) (*Server, error) {
	sealer, err := auth.NewSealer(cfg.SecretKey, "totp secret")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, store: st, limiter: newLimiter(), hub: live.New(), sealer: sealer, tickets: newTickets(), webauthn: newWebAuthn(cfg.PublicURL), ceremonies: newCeremonies()}, nil
}

// Shutdown ends the open event streams, so the HTTP server's graceful shutdown does not wait on them.
func (s *Server) Shutdown() { s.hub.Close() }

const maxBody = 8 << 20

// Router wires every route. `panel` is the built admin panel (index.html + assets).
func (s *Server) Router(panel fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), s.logRequests(), securityHeaders())
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		c.Next()
	})

	r.GET("/api/health", s.health)

	v1 := r.Group("/api/v1")
	v1.POST("/connect", s.rateLimited("connect", 10), s.connect)
	authed := v1.Group("", s.requireToken())
	authed.GET("/me", s.me)
	authed.GET("/workspaces/:id/changes", s.changes)
	authed.POST("/workspaces/:id/push", s.push)
	authed.GET("/workspaces/:id/events", s.events)
	authed.POST("/workspaces/:id/presence", s.presence)

	adm := r.Group("/api/admin", s.csrf())
	adm.GET("/state", s.adminState)
	adm.POST("/setup", s.rateLimited("setup", 10), s.setup)
	adm.POST("/login", s.rateLimited("login", 8), s.login)
	adm.POST("/login/code", s.rateLimited("login-code", 10), s.loginCode)
	adm.GET("/admin-invite", s.rateLimited("admin-invite", 20), s.adminInviteInfo)
	adm.POST("/admin-invite/accept", s.rateLimited("admin-invite", 20), s.acceptAdminInvite)
	adm.POST("/login/passkey/begin", s.rateLimited("passkey", 20), s.passkeyLoginBegin)
	adm.POST("/login/passkey/finish", s.rateLimited("passkey", 20), s.passkeyLoginFinish)
	adm.POST("/logout", s.logout)
	adm.GET("/oauth/:provider/start", s.rateLimited("oauth", 20), s.oauthStart)
	adm.GET("/oauth/:provider/callback", s.rateLimited("oauth", 20), s.oauthCallback)
	in := adm.Group("", s.requireAdmin())
	in.POST("/password", s.changePassword)
	in.GET("/admins", s.listAdmins)
	in.POST("/admins/invites", s.inviteAdmin)
	in.DELETE("/admins/invites/:id", s.cancelAdminInvite)
	in.DELETE("/admins/:id", s.removeAdmin)
	in.GET("/passkeys", s.listPasskeys)
	in.POST("/passkeys/register/begin", s.passkeyRegisterBegin)
	in.POST("/passkeys/register/finish", s.rateLimited("passkey", 20), s.passkeyRegisterFinish)
	in.DELETE("/passkeys/:id", s.deletePasskey)
	in.GET("/2fa", s.twoFactorState)
	in.POST("/2fa/start", s.twoFactorStart)
	in.POST("/2fa/confirm", s.rateLimited("2fa", 10), s.twoFactorConfirm)
	in.POST("/2fa/disable", s.rateLimited("2fa", 10), s.twoFactorDisable)
	in.GET("/members", s.listMembers)
	in.POST("/members", s.createMember)
	in.PATCH("/members/:id", s.updateMember)
	in.DELETE("/members/:id", s.deleteMember)
	in.PUT("/members/:id/workspaces/:ws", s.setMembership)
	in.GET("/workspaces", s.listWorkspaces)
	in.POST("/workspaces", s.createWorkspace)
	in.PATCH("/workspaces/:id", s.renameWorkspace)
	in.DELETE("/workspaces/:id", s.deleteWorkspace)
	in.GET("/tokens", s.listTokens)
	in.POST("/tokens", s.createToken)
	in.DELETE("/tokens/:id", s.revokeToken)
	in.GET("/invites", s.listInvites)
	in.POST("/invites", s.createInvite)
	in.DELETE("/invites/:id", s.deleteInvite)
	in.GET("/audit", s.listAudit)
	in.GET("/audit/stats", s.auditStats)
	in.POST("/backup", s.rateLimited("backup", 10), s.backup)
	in.POST("/restore", s.rateLimited("restore", 20), s.restore)

	r.NoRoute(s.panel(panel))
	return r
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": config.Product, "version": config.Version})
}

// ------------------------------------------------------------------ helpers

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

func internal(c *gin.Context, err error) {
	slog.Error("request failed", "path", c.FullPath(), "err", err)
	fail(c, http.StatusInternalServerError, "Something went wrong on the server")
}

func storeErr(c *gin.Context, err error, notFound string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(c, http.StatusNotFound, notFound)
	case errors.Is(err, store.ErrConflict):
		fail(c, http.StatusConflict, "That email is already used")
	default:
		internal(c, err)
	}
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, http.StatusBadRequest, "The request body is not valid JSON for this call")
		return false
	}
	return true
}

func clean(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

func validEmail(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " \t\n<>")
}

func validURL(u string) bool {
	if u == "" {
		return true
	}
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}

func (s *Server) logRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if strings.HasPrefix(c.Request.URL.Path, "/api/") && c.Request.URL.Path != "/api/health" {
			slog.Info("http", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "ms", time.Since(start).Milliseconds())
		}
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		}
		c.Next()
	}
}

// ------------------------------------------------------------------ rate limiting

// limiter: a fixed window per (bucket, client address); enough to slow password and code guessing.
type limiter struct {
	mu   sync.Mutex
	hits map[string]*window
}
type window struct {
	start time.Time
	n     int
}

func newLimiter() *limiter { return &limiter{hits: map[string]*window{}} }

func (l *limiter) allow(key string, perMinute int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := time.Now()
	if len(l.hits) > 10_000 {
		for k, w := range l.hits {
			if t.Sub(w.start) > time.Minute {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[key]
	if w == nil || t.Sub(w.start) > time.Minute {
		l.hits[key] = &window{start: t, n: 1}
		return true
	}
	w.n++
	return w.n <= perMinute
}

func (s *Server) rateLimited(bucket string, perMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.limiter.allow(bucket+"|"+c.ClientIP(), perMinute) {
			fail(c, http.StatusTooManyRequests, "Too many attempts. Wait a minute and try again.")
			return
		}
		c.Next()
	}
}

// ------------------------------------------------------------------ admin panel (static)

func (s *Server) panel(panel fs.FS) gin.HandlerFunc {
	files := http.FileServer(http.FS(panel))
	return func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if strings.HasPrefix(p, "api/") {
			fail(c, http.StatusNotFound, "No such API route")
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			fail(c, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if p != "" {
			if f, err := panel.Open(p); err == nil {
				f.Close()
				if strings.HasPrefix(p, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		// The panel is a single page: every other path gets index.html.
		index, err := fs.ReadFile(panel, "index.html")
		if err != nil {
			c.String(http.StatusOK, "DialCatz Den is running. The admin panel was not built into this binary (run `bun run build` in admin/).")
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	}
}
