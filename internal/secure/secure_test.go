package secure

import (
	"strings"
	"testing"
)

func vault(t *testing.T) *Vault {
	t.Helper()
	v, err := NewVault([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"98765 43210":       "+919876543210",
		"098765-43210":      "+919876543210",
		"919876543210":      "+919876543210",
		"+91 98765 43210":   "+919876543210",
		"0091 9876543210":   "+919876543210",
		"+1 (415) 555-0100": "+14155550100",
	}
	for in, want := range cases {
		got, err := NormalizePhone(in)
		if err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "12345", "+12", "98765x43210", "1234567890123456789"} {
		if _, err := NormalizePhone(bad); err == nil {
			t.Errorf("NormalizePhone(%q) accepted", bad)
		}
	}
}

func TestEncryptRoundTripAndTamper(t *testing.T) {
	v := vault(t)
	sealed, err := v.Encrypt("+919876543210")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "9876543210") {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := v.Decrypt(sealed)
	if err != nil || got != "+919876543210" {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := v.Decrypt(sealed); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	a, _ := v.Encrypt("x")
	b, _ := v.Encrypt("x")
	if string(a) == string(b) {
		t.Fatal("encryption is deterministic; nonce reuse")
	}
}

func TestHashStableAndKeyed(t *testing.T) {
	v := vault(t)
	a, b := v.HashPhone("+91"), v.HashPhone("+91")
	if a != b {
		t.Fatal("hash not stable")
	}
	other, _ := NewVault([]byte(strings.Repeat("z", 32)))
	if v.HashPhone("+91") == other.HashPhone("+91") {
		t.Fatal("hash not keyed")
	}
	ref := PublicRef("R", v.HashPhone("+919876543210"))
	if !strings.HasPrefix(ref, "R-") || len(ref) != 10 {
		t.Fatalf("bad ref %q", ref)
	}
}

func TestSignVerify(t *testing.T) {
	v := vault(t)
	tag := v.Sign("hello")
	if !v.Verify("hello", tag) || v.Verify("hellO", tag) {
		t.Fatal("sign/verify mismatch")
	}
	if t1, t2 := NewToken(), NewToken(); t1 == t2 {
		t.Fatal("tokens repeat")
	}
	if MaskPhone("+919876543210") != "+91•••••••210" {
		t.Fatalf("mask = %q", MaskPhone("+919876543210"))
	}
}

func TestShortSecretRejected(t *testing.T) {
	if _, err := NewVault([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}
