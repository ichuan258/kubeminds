package crypto

import (
	"strings"
	"testing"
)

// generateTestKey returns a deterministic 32-byte key for use in tests only.
func generateTestKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := generateTestKey()
	plaintext := "sk-test-api-key-12345"

	encrypted, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Encrypted value must carry the sentinel prefix.
	if !strings.HasPrefix(encrypted, encPrefix) {
		t.Errorf("Encrypt() output missing prefix %q, got %q", encPrefix, encrypted)
	}

	decrypted, err := Decrypt(key, encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_ProducesUniqueNonce(t *testing.T) {
	// Encrypting the same plaintext twice must produce different ciphertext
	// because a fresh random nonce is used each time.
	key := generateTestKey()
	plaintext := "same-api-key"

	enc1, _ := Encrypt(key, plaintext)
	enc2, _ := Encrypt(key, plaintext)

	if enc1 == enc2 {
		t.Error("Encrypt() produced identical ciphertexts for the same input — nonce reuse detected")
	}
}

func TestDecrypt_PlaintextPassThrough(t *testing.T) {
	// Values without the enc: prefix are returned unchanged — no key needed.
	key := generateTestKey()
	plain := "not-encrypted-value"

	out, err := Decrypt(key, plain)
	if err != nil {
		t.Fatalf("Decrypt() unexpected error for plain value: %v", err)
	}
	if out != plain {
		t.Errorf("Decrypt() = %q, want %q", out, plain)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key := generateTestKey()
	wrongKey := make([]byte, 32) // all zeros

	encrypted, _ := Encrypt(key, "secret")

	_, err := Decrypt(wrongKey, encrypted)
	if err == nil {
		t.Error("Decrypt() with wrong key should return an error")
	}
}

func TestDecrypt_TamperedData(t *testing.T) {
	key := generateTestKey()
	encrypted, _ := Encrypt(key, "secret")

	// Tamper with the last character of the base64-encoded payload.
	tampered := encrypted[:len(encrypted)-1] + "X"
	_, err := Decrypt(key, tampered)
	if err == nil {
		t.Error("Decrypt() with tampered ciphertext should return an error")
	}
}

func TestIsEncrypted(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"enc:aes256:abc123", true},
		{"sk-plain-api-key", false},
		{"", false},
		{"enc:other:abc", false}, // wrong prefix variant
	}

	for _, c := range cases {
		if got := IsEncrypted(c.value); got != c.want {
			t.Errorf("IsEncrypted(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	shortKey := []byte("tooshort")
	_, err := Encrypt(shortKey, "plaintext")
	if err == nil {
		t.Error("Encrypt() with short key should return an error")
	}
}
