package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	prefix   = "ENC:"
	saltLen  = 16
	nonceLen = 12
	keyLen   = 32
	scryptN  = 32768
	scryptR  = 8
	scryptP  = 1
)

// DeriveKey derives a 32-byte AES key from a password and salt using scrypt.
func DeriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}
	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns an "ENC:base64(...)" string.
// Returns empty string for empty input.
func Encrypt(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	derivedKey, err := DeriveKey(string(key), salt)
	if err != nil {
		return "", fmt.Errorf("deriving key: %w", err)
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Encode salt + nonce + ciphertext
	combined := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	combined = append(combined, salt...)
	combined = append(combined, nonce...)
	combined = append(combined, ciphertext...)

	return prefix + base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts an "ENC:base64(...)" string back to plaintext.
// Non-encrypted strings are returned unchanged (migration support).
func Decrypt(encoded string, key []byte) (string, error) {
	if !IsEncrypted(encoded) {
		return encoded, nil
	}

	b64 := strings.TrimPrefix(encoded, prefix)
	combined, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	if len(combined) < saltLen+nonceLen {
		return "", fmt.Errorf("ciphertext too short")
	}

	salt := combined[:saltLen]
	nonce := combined[saltLen : saltLen+nonceLen]
	ciphertext := combined[saltLen+nonceLen:]

	derivedKey, err := DeriveKey(string(key), salt)
	if err != nil {
		return "", fmt.Errorf("deriving key: %w", err)
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	return string(plaintext), nil
}

// EncryptWithDerivedKey encrypts plaintext using a pre-derived AES-256 key,
// skipping scrypt derivation. The provided salt is stored in the output so
// Decrypt can re-derive the same key. Output format is identical to Encrypt.
func EncryptWithDerivedKey(plaintext string, derivedKey, salt []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	combined := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	combined = append(combined, salt...)
	combined = append(combined, nonce...)
	combined = append(combined, ciphertext...)

	return prefix + base64.StdEncoding.EncodeToString(combined), nil
}

// GenerateSalt returns a random salt for key derivation.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return salt, nil
}

// IsEncrypted returns true if the string has the "ENC:" prefix.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, prefix)
}
