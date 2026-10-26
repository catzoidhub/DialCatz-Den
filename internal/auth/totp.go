package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238) as every authenticator app expects it: SHA-1, 6 digits, 30 second steps.

const (
	totpPeriod = 30
	totpDigits = 6
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret makes a 160-bit secret, base32 without padding (what otpauth:// URIs carry).
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b)
}

// TOTPURI is the otpauth:// link an authenticator app scans as a QR code.
func TOTPURI(secret, issuer, account string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	esc := func(s string) string { return strings.ReplaceAll(url.PathEscape(s), ":", "%3A") }
	label := esc(issuer) + ":" + esc(account)
	return "otpauth://totp/" + label + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

func totpAt(key []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000)
}

// CheckTOTP returns the time step the code belongs to (the current one, or one either side for clock drift), or
// 0 when it matches none. Callers keep the last step used so a code cannot be replayed.
func CheckTOTP(secret, code string, t time.Time) int64 {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0
	}
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return 0
	}
	now := t.Unix() / totpPeriod
	var found int64
	for _, step := range []int64{now - 1, now, now + 1} {
		if subtle.ConstantTimeCompare([]byte(totpAt(key, step)), []byte(code)) == 1 {
			found = step
		}
	}
	return found
}

// TOTPCode is the code for a moment (tests and tools).
func TOTPCode(secret string, t time.Time) string {
	key, _ := b32.DecodeString(strings.ToUpper(secret))
	return totpAt(key, t.Unix()/totpPeriod)
}

// NewRecoveryCodes makes n one-time codes like "k7m2q-x9p4t" (no 0/1/o/l to misread).
func NewRecoveryCodes(n int) []string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	out := make([]string, n)
	for i := range out {
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		for j := range b {
			b[j] = alphabet[int(b[j])%len(alphabet)]
		}
		out[i] = string(b[:5]) + "-" + string(b[5:])
	}
	return out
}

// RecoveryHash normalises a typed recovery code (case, spaces, dashes) and hashes it.
func RecoveryHash(code string) string {
	c := strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	return Hash("recovery:" + c)
}
