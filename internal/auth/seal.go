package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Sealer encrypts small secrets at rest (the admins' TOTP secrets) with AES-256-GCM, under a key derived from
// DIALCATZ_SECRET_KEY. Changing that key makes sealed values unreadable: two-step sign-in then has to be set up again.
type Sealer struct{ aead cipher.AEAD }

func NewSealer(secretKey, purpose string) (*Sealer, error) {
	key, err := hkdf.Key(sha256.New, []byte(secretKey), nil, "dialcatz-den "+purpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal returns base64(nonce | ciphertext).
func (s *Sealer) Seal(plain string) string {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return base64.RawStdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(plain), nil))
}

var ErrUnseal = errors.New("the stored secret cannot be decrypted (was DIALCATZ_SECRET_KEY changed?)")

func (s *Sealer) Open(sealed string) (string, error) {
	b, err := base64.RawStdEncoding.DecodeString(sealed)
	n := s.aead.NonceSize()
	if err != nil || len(b) < n {
		return "", ErrUnseal
	}
	plain, err := s.aead.Open(nil, b[:n], b[n:], nil)
	if err != nil {
		return "", ErrUnseal
	}
	return string(plain), nil
}
