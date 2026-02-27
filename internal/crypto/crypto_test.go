package crypto

import (
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := []byte("test-master-password")
	plaintext := "my-secret-password"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if !IsEncrypted(encrypted) {
		t.Fatalf("expected encrypted string to have ENC: prefix, got %q", encrypted)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestWrongKey(t *testing.T) {
	key := []byte("correct-password")
	wrongKey := []byte("wrong-password")

	encrypted, err := Encrypt("secret", key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected Decrypt with wrong key to fail")
	}
}

func TestEmptyString(t *testing.T) {
	key := []byte("password")

	encrypted, err := Encrypt("", key)
	if err != nil {
		t.Fatalf("Encrypt empty string failed: %v", err)
	}

	if encrypted != "" {
		t.Fatalf("expected empty string for empty input, got %q", encrypted)
	}

	decrypted, err := Decrypt("", key)
	if err != nil {
		t.Fatalf("Decrypt empty string failed: %v", err)
	}

	if decrypted != "" {
		t.Fatalf("expected empty string, got %q", decrypted)
	}
}

func TestIsEncrypted(t *testing.T) {
	if IsEncrypted("plaintext") {
		t.Fatal("plaintext should not be detected as encrypted")
	}
	if !IsEncrypted("ENC:abc123") {
		t.Fatal("ENC:abc123 should be detected as encrypted")
	}
	if IsEncrypted("") {
		t.Fatal("empty string should not be detected as encrypted")
	}
}

func TestPlaintextPassthrough(t *testing.T) {
	key := []byte("password")

	plaintext := "not-encrypted-password"
	result, err := Decrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Decrypt plaintext passthrough failed: %v", err)
	}

	if result != plaintext {
		t.Fatalf("expected passthrough %q, got %q", plaintext, result)
	}
}

func TestEncryptWithDerivedKeyRoundTrip(t *testing.T) {
	key := []byte("test-master-password")
	plaintext := "my-secret-password"

	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}
	derivedKey, err := DeriveKey(string(key), salt)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	encrypted, err := EncryptWithDerivedKey(plaintext, derivedKey, salt)
	if err != nil {
		t.Fatalf("EncryptWithDerivedKey failed: %v", err)
	}

	if !IsEncrypted(encrypted) {
		t.Fatalf("expected ENC: prefix, got %q", encrypted)
	}

	// Decrypt uses the original key bytes (re-derives internally via scrypt)
	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptWithDerivedKeyEmpty(t *testing.T) {
	derivedKey := make([]byte, 32)
	salt := make([]byte, 16)

	encrypted, err := EncryptWithDerivedKey("", derivedKey, salt)
	if err != nil {
		t.Fatalf("EncryptWithDerivedKey empty failed: %v", err)
	}
	if encrypted != "" {
		t.Fatalf("expected empty string for empty input, got %q", encrypted)
	}
}

func TestUniqueEncryptions(t *testing.T) {
	key := []byte("password")
	plaintext := "same-password"

	enc1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	enc2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if enc1 == enc2 {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}

	// Both should decrypt to the same value
	dec1, _ := Decrypt(enc1, key)
	dec2, _ := Decrypt(enc2, key)
	if dec1 != dec2 {
		t.Fatalf("decrypted values differ: %q vs %q", dec1, dec2)
	}
}
