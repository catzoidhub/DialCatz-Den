// Package config reads the server's settings from the environment.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Product is the name the server answers with on /api/health; the DialCatz app checks it.
const Product = "DialCatz Den"

// Version is set at build time (-ldflags "-X …/config.Version=v0.1.0").
var Version = "dev"

type Config struct {
	// Addr to listen on, e.g. ":8080".
	Addr string
	// PublicURL is the address members use (https://dialcatz.example.com); it goes into invite links.
	PublicURL string
	// DatabaseURL: postgres://… for PostgreSQL, else a SQLite file path (default /data/dialcatz.db).
	DatabaseURL string
	// SecretKey: the admins' two-step (TOTP) secrets are encrypted with a key derived from it; generated into the
	// data folder when not given. Changing it means setting up two-step sign-in again.
	SecretKey string
	DataDir   string
	// SecureCookies: the admin session cookie only over HTTPS (on unless the public URL is http://).
	SecureCookies bool
	// OAuth apps of the admin's own for "Sign in with Google / GitHub" (each on only when both values are set).
	GoogleClientID, GoogleClientSecret string
	GitHubClientID, GitHubClientSecret string
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func Load() (*Config, error) {
	c := &Config{
		Addr:        env("DIALCATZ_ADDR", ":8080"),
		PublicURL:   strings.TrimRight(env("DIALCATZ_PUBLIC_URL", "http://localhost:8080"), "/"),
		DataDir:     env("DIALCATZ_DATA_DIR", "/data"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		SecretKey:   os.Getenv("DIALCATZ_SECRET_KEY"),

		GoogleClientID:     env("DIALCATZ_GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("DIALCATZ_GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID:     env("DIALCATZ_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("DIALCATZ_GITHUB_CLIENT_SECRET", ""),
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("data folder %s: %w", c.DataDir, err)
	}
	if c.DatabaseURL == "" {
		c.DatabaseURL = filepath.Join(c.DataDir, "dialcatz.db")
	}
	if c.SecretKey == "" {
		key, err := persistedKey(filepath.Join(c.DataDir, "secret.key"))
		if err != nil {
			return nil, err
		}
		c.SecretKey = key
	}
	c.SecureCookies = strings.HasPrefix(c.PublicURL, "https://")
	return c, nil
}

// persistedKey reads the generated secret key, or makes one the first time.
func persistedKey(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		return strings.TrimSpace(string(b)), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	key := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return key, nil
}

// IsPostgres tells which database DATABASE_URL points to.
func (c *Config) IsPostgres() bool {
	return strings.HasPrefix(c.DatabaseURL, "postgres://") || strings.HasPrefix(c.DatabaseURL, "postgresql://")
}
