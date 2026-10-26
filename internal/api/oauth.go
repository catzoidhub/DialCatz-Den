package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
)

// Sign in with Google / GitHub, through the admin's own OAuth apps (DIALCATZ_GOOGLE_CLIENT_ID/SECRET,
// DIALCATZ_GITHUB_CLIENT_ID/SECRET). It only signs in an existing admin whose verified email matches; it never
// creates one. Authorization code flow with state and PKCE; the state travels in a SameSite=Lax cookie, since
// the provider's redirect back is a cross-site navigation.

const oauthCookie = "dialcatz_oauth"

type oauthProvider struct {
	name, authURL, tokenURL, clientID, clientSecret, scope string
	extra                                                  url.Values
	// email returns the account's verified email from the access token.
	email func(ctx context.Context, accessToken string) (string, error)
}

var oauthHTTP = &http.Client{Timeout: 10 * time.Second}

func (s *Server) provider(name string) *oauthProvider {
	switch {
	case name == "google" && s.cfg.GoogleClientID != "" && s.cfg.GoogleClientSecret != "":
		return &oauthProvider{name: name, authURL: "https://accounts.google.com/o/oauth2/v2/auth", tokenURL: "https://oauth2.googleapis.com/token",
			clientID: s.cfg.GoogleClientID, clientSecret: s.cfg.GoogleClientSecret, scope: "openid email",
			extra: url.Values{"prompt": {"select_account"}}, email: googleEmail}
	case name == "github" && s.cfg.GitHubClientID != "" && s.cfg.GitHubClientSecret != "":
		return &oauthProvider{name: name, authURL: "https://github.com/login/oauth/authorize", tokenURL: "https://github.com/login/oauth/access_token",
			clientID: s.cfg.GitHubClientID, clientSecret: s.cfg.GitHubClientSecret, scope: "user:email",
			extra: url.Values{"allow_signup": {"false"}}, email: githubEmail}
	}
	return nil
}

// oauthEnabled lists the providers set up on this server (for the sign-in page).
func (s *Server) oauthEnabled() []string {
	out := []string{}
	for _, n := range []string{"google", "github"} {
		if s.provider(n) != nil {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) redirectURL(p *oauthProvider) string {
	return s.cfg.PublicURL + "/api/admin/oauth/" + p.name + "/callback"
}

func (s *Server) setOAuthCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: oauthCookie, Value: value, Path: "/api/admin/oauth", MaxAge: maxAge, HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) oauthStart(c *gin.Context) {
	p := s.provider(c.Param("provider"))
	if p == nil {
		fail(c, http.StatusNotFound, "This sign-in is not set up on this server")
		return
	}
	state, verifier := auth.Random(24), auth.Random(48)
	sum := sha256.Sum256([]byte(verifier))
	s.setOAuthCookie(c, p.name+"."+state+"."+verifier, 600)
	q := url.Values{
		"client_id":             {p.clientID},
		"redirect_uri":          {s.redirectURL(p)},
		"response_type":         {"code"},
		"scope":                 {p.scope},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	for k, v := range p.extra {
		q[k] = v
	}
	c.Redirect(http.StatusFound, p.authURL+"?"+q.Encode())
}

// oauthCallback ends on the panel: "/" signed in, or "/?signin=<reason>" (code, nomatch, denied, expired, failed, off).
func (s *Server) oauthCallback(c *gin.Context) {
	back := func(reason string) {
		target := "/"
		if reason != "" {
			target += "?signin=" + reason
		}
		c.Redirect(http.StatusFound, target)
	}
	p := s.provider(c.Param("provider"))
	if p == nil {
		back("off")
		return
	}
	saved, _ := c.Cookie(oauthCookie)
	s.setOAuthCookie(c, "", -1)
	parts := strings.Split(saved, ".")
	if len(parts) != 3 || parts[0] != p.name || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(c.Query("state"))) != 1 {
		back("expired")
		return
	}
	if c.Query("error") != "" || c.Query("code") == "" {
		back("denied")
		return
	}
	email, err := s.oauthEmail(c.Request.Context(), p, c.Query("code"), parts[2])
	if err != nil {
		slog.Warn("oauth sign-in failed", "provider", p.name, "err", err)
		back("failed")
		return
	}
	a, err := s.store.AdminByEmail(email)
	if errors.Is(err, store.ErrNotFound) {
		s.store.Audit(p.name+":"+email, "admin.oauth.rejected", "no admin with this email "+c.ClientIP())
		back("nomatch")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	if a.TwoFactor {
		http.SetCookie(c.Writer, &http.Cookie{Name: ticketCookie, Value: s.tickets.issue(a.ID, p.name), Path: "/api/admin", MaxAge: int(ticketTTL.Seconds()), HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
		back("code")
		return
	}
	if s.startSession(c, a, p.name) {
		back("")
	}
}

// oauthEmail exchanges the code (with the PKCE verifier) for an access token and asks for the verified email.
func (s *Server) oauthEmail(ctx context.Context, p *oauthProvider, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.redirectURL(p)},
		"client_id":     {p.clientID},
		"client_secret": {p.clientSecret},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := doJSON(req, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("no access token (%s)", tok.Error)
	}
	email, err := p.email(ctx, tok.AccessToken)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(email)), nil
}

func doJSON(req *http.Request, v any) error {
	res, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 && res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("%s: HTTP %d", req.URL.Host, res.StatusCode)
	}
	return json.Unmarshal(body, v)
}

func bearerGet(ctx context.Context, u, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func googleEmail(ctx context.Context, token string) (string, error) {
	req, err := bearerGet(ctx, "https://openidconnect.googleapis.com/v1/userinfo", token)
	if err != nil {
		return "", err
	}
	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := doJSON(req, &info); err != nil {
		return "", err
	}
	if info.Email == "" || !info.EmailVerified {
		return "", errors.New("the Google account has no verified email")
	}
	return info.Email, nil
}

// githubEmail: GitHub's profile email can be anything; /user/emails says which one is primary and verified.
func githubEmail(ctx context.Context, token string) (string, error) {
	req, err := bearerGet(ctx, "https://api.github.com/user/emails", token)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := doJSON(req, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", errors.New("the GitHub account has no primary verified email")
}
