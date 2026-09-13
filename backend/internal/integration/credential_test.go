package integration

import (
	"testing"
)

func TestCredential_EncryptDecrypt_Isolation(t *testing.T) {
	secret := "test-jwt-secret-32-chars-long-123"
	creds := map[string]any{"apiKey": "sk-12345", "token": "tok-abc", "other": "value"}
	enc, err := Encrypt(creds, secret)
	if err != nil { t.Fatalf("encrypt: %v", err) }
	if enc == "" { t.Fatal("empty ciphertext") }
	// ensure not storing raw
	if enc == "sk-12345" { t.Fatal("should be encrypted") }
	dec, err := Decrypt(enc, secret)
	if err != nil { t.Fatalf("decrypt: %v", err) }
	if dec["apiKey"] != "sk-12345" || dec["token"] != "tok-abc" { t.Fatalf("decrypt mismatch: %v", dec) }
	// wrong secret should fail
	if _, err := Decrypt(enc, "wrong-secret"); err == nil { t.Fatal("should fail with wrong secret") }
}

func TestCredential_Redacted(t *testing.T) {
	creds := map[string]any{"apiKey": "secret123", "username": "alice", "password": "pwd"}
	red := Redacted(creds)
	if red["apiKey"] == "secret123" { t.Fatal("should redact apiKey") }
	if red["password"] == "pwd" { t.Fatal("should redact password") }
	if red["username"] != "alice" { t.Fatal("username should not be redacted") }
}

func TestCredential_DifferentOrgs_Isolation(t *testing.T) {
	secret1 := "secret-org1"
	secret2 := "secret-org2"
	creds := map[string]any{"token": "abc"}
	enc1, _ := Encrypt(creds, secret1)
	enc2, _ := Encrypt(creds, secret2)
	if enc1 == enc2 { t.Fatal("different secrets should produce different ciphertexts") }
	if _, err := Decrypt(enc1, secret2); err == nil { t.Fatal("cross-org decrypt should fail") }
}
