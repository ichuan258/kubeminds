// Package crypto provides AES-256-GCM encryption/decryption for sensitive config values.
//
// Usage pattern:
//  1. Generate a 32-byte master key (e.g. `openssl rand -hex 32`) and store it in
//     the KUBEMINDS_MASTER_KEY environment variable.
//  2. Encrypt your API key with `make encrypt-key KEY=sk-xxx` → outputs an "enc:aes256:..." string.
//  3. Paste the encrypted string into config.yaml. The config loader decrypts it automatically at startup.
//
// The "enc:aes256:" prefix acts as a sentinel so plain-text values pass through unchanged.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// encPrefix is the sentinel prefix that identifies an encrypted config value.
// Format: enc:aes256:<base64(nonce+ciphertext)>
const encPrefix = "enc:aes256:"

// Encrypt encrypts plaintext with AES-256-GCM using the provided 32-byte key.
// The returned string includes the "enc:aes256:" prefix and can be stored directly in config.
// A fresh random 12-byte nonce is generated for every call, so repeated encryption of the
// same plaintext produces different ciphertext (semantically secure).
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("crypto: key must be exactly 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create GCM: %w", err)
	}

	// Generate a cryptographically random nonce (12 bytes for GCM).
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and GCM authentication tag to the nonce.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value previously encrypted with Encrypt.
// If value does not start with the "enc:aes256:" prefix, it is returned unchanged —
// this allows mixing encrypted and plain-text values in the same config file during migration.
func Decrypt(key []byte, value string) (string, error) {
	if !IsEncrypted(value) {
		// Not an encrypted value, return as-is.
		return value, nil
	}

	if len(key) != 32 {
		return "", fmt.Errorf("crypto: key must be exactly 32 bytes, got %d", len(key))
	}

	// Strip prefix and base64-decode.
	encoded := strings.TrimPrefix(value, encPrefix)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to base64-decode encrypted value: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Likely wrong key or tampered data.
		return "", fmt.Errorf("crypto: decryption failed (wrong key or corrupted data): %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted reports whether value is an encrypted config value (has the enc:aes256: prefix).
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encPrefix)
}

// MasterKeyFromEnv reads the 32-byte master key from the KUBEMINDS_MASTER_KEY environment variable.
// The env var must be a 64-character lowercase hex string (e.g. from `openssl rand -hex 32`).
// Returns an error if the variable is missing or malformed.
func MasterKeyFromEnv() ([]byte, error) {
	hexKey := os.Getenv("KUBEMINDS_MASTER_KEY")
	if hexKey == "" {
		return nil, fmt.Errorf("crypto: KUBEMINDS_MASTER_KEY environment variable is not set; " +
			"generate one with: openssl rand -hex 32")
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: KUBEMINDS_MASTER_KEY is not valid hex: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: KUBEMINDS_MASTER_KEY must be 64 hex chars (32 bytes), got %d bytes", len(key))
	}

	return key, nil
}

// DecryptValue decrypts a single config value using the master key from KUBEMINDS_MASTER_KEY.
// If the value does not have the "enc:aes256:" prefix, it is returned unchanged.
// If the value is encrypted but the env var is missing or the key is wrong, an error is returned.
func DecryptValue(value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}

	key, err := MasterKeyFromEnv()
	if err != nil {
		return "", fmt.Errorf("crypto: cannot decrypt config value: %w", err)
	}

	return Decrypt(key, value)
}
