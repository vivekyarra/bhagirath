// Package secure implements PII protection: phone numbers are stored only as
// an HMAC (for de-duplication and distinct-reporter counting) and as AES-256-GCM
// ciphertext (for consented callbacks). Plain numbers never reach the database,
// the dashboard or the logs.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInvalidPhone is returned for strings that are not plausible E.164 numbers.
var ErrInvalidPhone = errors.New("invalid phone number")

// Vault holds the keys used to protect personal data.
type Vault struct {
	hashKey []byte
	aead    cipher.AEAD
	signKey []byte
}

// NewVault derives purpose-specific keys from a single master secret
// (at least 32 bytes, typically from BHAGIRATH_SECRET_KEY).
func NewVault(master []byte) (*Vault, error) {
	if len(master) < 32 {
		return nil, errors.New("secret key must be at least 32 bytes")
	}
	derive := func(label string) []byte {
		m := hmac.New(sha256.New, master)
		m.Write([]byte("bhagirath/v1/" + label))
		return m.Sum(nil)
	}
	block, err := aes.NewCipher(derive("encrypt"))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{hashKey: derive("hash"), aead: aead, signKey: derive("sign")}, nil
}

// NormalizePhone converts common Indian formats to E.164 (+91XXXXXXXXXX).
// Numbers with an explicit country code are kept as-is.
func NormalizePhone(raw string) (string, error) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", ErrInvalidPhone
		}
	}
	s := b.String()
	switch {
	case strings.HasPrefix(s, "+"):
	case strings.HasPrefix(s, "00"):
		s = "+" + s[2:]
	case len(s) == 11 && s[0] == '0':
		s = "+91" + s[1:]
	case len(s) == 10:
		s = "+91" + s
	case len(s) == 12 && strings.HasPrefix(s, "91"):
		s = "+" + s
	default:
		return "", ErrInvalidPhone
	}
	digits := len(s) - 1
	if digits < 8 || digits > 15 {
		return "", ErrInvalidPhone
	}
	return s, nil
}

// HashPhone returns a stable keyed hash of a normalized phone number.
func (v *Vault) HashPhone(e164 string) string {
	m := hmac.New(sha256.New, v.hashKey)
	m.Write([]byte(e164))
	return hex.EncodeToString(m.Sum(nil))
}

// PublicRef derives a short, non-reversible identifier for display
// (e.g. "R-7F3A2C9Q"). It never reveals the phone number.
func PublicRef(prefix, hash string) string {
	raw, _ := hex.DecodeString(hash)
	if len(raw) < 5 {
		raw = append(raw, make([]byte, 5-len(raw))...)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:5])
	return prefix + "-" + enc
}

// Encrypt seals plaintext with AES-256-GCM. The nonce is prepended.
func (v *Vault) Encrypt(plain string) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, []byte(plain), []byte("phone")), nil
}

// Decrypt opens a value produced by Encrypt.
func (v *Vault) Decrypt(sealed []byte) (string, error) {
	ns := v.aead.NonceSize()
	if len(sealed) < ns {
		return "", errors.New("ciphertext too short")
	}
	out, err := v.aead.Open(nil, sealed[:ns], sealed[ns:], []byte("phone"))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(out), nil
}

// Sign returns an HMAC-SHA256 tag over msg (base64url, no padding).
func (v *Vault) Sign(msg string) string {
	m := hmac.New(sha256.New, v.signKey)
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Verify checks a tag produced by Sign in constant time.
func (v *Vault) Verify(msg, tag string) bool {
	return hmac.Equal([]byte(v.Sign(msg)), []byte(tag))
}

// NewToken returns a URL-safe random token with 256 bits of entropy.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the SHA-256 hex digest of a bearer token; only the digest
// is stored, so a database leak does not leak working links.
func HashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// MaskPhone renders a number as +91•••••••123 for operator confirmation screens.
func MaskPhone(e164 string) string {
	if len(e164) <= 6 {
		return "•••"
	}
	return e164[:3] + strings.Repeat("•", len(e164)-6) + e164[len(e164)-3:]
}
