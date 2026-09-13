package integration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Simple AES-GCM encryption derived from secret (JWT_SECRET or separate key)
// Never store raw provider secrets in ordinary agent configuration

func deriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func Encrypt(plain map[string]any, secret string) (string, error) {
	plaintext, err := json.Marshal(plain)
	if err != nil { return "", err }
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(ciphertext string, secret string) (map[string]any, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil { return nil, err }
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return nil, err }
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize { return nil, fmt.Errorf("ciphertext too short") }
	nonce, ct := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil { return nil, err }
	var out map[string]any
	if err := json.Unmarshal(plaintext, &out); err != nil { return nil, err }
	return out, nil
}

// Redacted returns a copy with secrets masked for audit/logging
func Redacted(creds map[string]any) map[string]any {
	out := make(map[string]any, len(creds))
	for k, v := range creds {
		// mask values that look like secrets
		if isSecretKey(k) {
			out[k] = "***REDACTED***"
		} else {
			out[k] = v
		}
	}
	return out
}

func isSecretKey(k string) bool {
	lower := strings.ToLower(k)
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") || strings.Contains(lower, "password") || strings.Contains(lower, "credential")
}
