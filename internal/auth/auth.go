// Package auth: admin passwords (argon2id), member tokens and invite codes (long random, stored as SHA-256).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// TokenPrefix makes tokens recognisable to people and to secret scanners.
const TokenPrefix = "dct_"

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

// HashPassword returns an encoded argon2id hash ($argon2id$v=19$m=…,t=…,p=…$salt$hash).
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func CheckPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[4])
	want, err2 := enc.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Random returns n random bytes as URL-safe text.
func Random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ID is a short random id with a readable prefix (m_, w_, t_ …).
func ID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

// NewToken makes a member token: dct_ + 32 random bytes. Only its hash is stored.
func NewToken() (token, hash, prefix string) {
	token = TokenPrefix + Random(32)
	return token, Hash(token), token[:len(TokenPrefix)+6]
}

// NewInviteCode makes a one-time invite code (shorter: it travels in a link, lives a day, works once).
func NewInviteCode() (code, hash string) {
	code = Random(18)
	return code, Hash(code)
}

func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

var ErrWeakPassword = errors.New("use at least 10 characters for the admin password")

func CheckStrength(password string) error {
	if len([]rune(password)) < 10 {
		return ErrWeakPassword
	}
	return nil
}
