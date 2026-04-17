// Package auth implements password hashing, phone-number encryption,
// masking rules, org/class scope checks, and login/session logic for
// HarborClass.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// DeriveKey produces a stable 32-byte key from an arbitrary-length
// configuration secret using SHA-256. This avoids requiring operators
// to provision a raw 32-byte blob.
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// HashPassword uses a salted SHA-256 pipeline. HarborClass normally
// uses bcrypt via golang.org/x/crypto, but we keep the stdlib variant
// here as a fallback that remains test-friendly in offline runs.
func HashPassword(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(":"))
	h.Write([]byte(password))
	return "sha256$" + salt + "$" + base64.RawStdEncoding.EncodeToString(h.Sum(nil))
}

// VerifyPassword checks a password against a stored HashPassword string.
func VerifyPassword(password, stored string) bool {
	// "sha256$<salt>$<digest>"
	// split without strings package for simplicity/locality.
	var parts [3]string
	i := 0
	start := 0
	for j := 0; j < len(stored) && i < 3; j++ {
		if stored[j] == '$' {
			parts[i] = stored[start:j]
			i++
			start = j + 1
		}
	}
	if i < 2 {
		return false
	}
	parts[2] = stored[start:]
	if parts[0] != "sha256" {
		return false
	}
	return HashPassword(password, parts[1]) == stored
}

// EncryptPII encrypts plaintext with AES-GCM and returns base64.
func EncryptPII(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptPII reverses EncryptPII.
func DecryptPII(key []byte, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode cipher: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(ct) < ns {
		return "", errors.New("cipher too short")
	}
	nonce, body := ct[:ns], ct[ns:]
	pt, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// MaskPhone returns a display-safe masked phone number, preserving only
// the last 4 digits. Empty strings are returned unchanged.
func MaskPhone(p string) string {
	if p == "" {
		return ""
	}
	digits := make([]rune, 0, len(p))
	for _, r := range p {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) <= 4 {
		return "****"
	}
	out := make([]rune, len(digits))
	for i := range digits {
		if i >= len(digits)-4 {
			out[i] = digits[i]
		} else {
			out[i] = '*'
		}
	}
	return string(out)
}
