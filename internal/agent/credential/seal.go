package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
)

// Seal encrypts plaintext with AES-256-GCM so per-agent model API keys can be
// stored at rest without appearing in cleartext. The key must come from the
// deployment environment (AGENT_SECRET_KEY) and never from a request.
func Seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("seal: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal. A wrong key or tampered ciphertext
// returns an error, never partial plaintext.
func Open(key, ciphertext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("open: ciphertext too short")
	}
	nonce, body := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, body, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) < 32 {
		return nil, errors.New("seal key must contain at least 32 bytes")
	}
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealKeyFromEnv returns the deployment key used for Seal/Open. Custom agent
// API keys are rejected when it is unset so they are never stored raw.
func SealKeyFromEnv() ([]byte, error) {
	key := os.Getenv("AGENT_SECRET_KEY")
	if key == "" {
		return nil, errors.New("AGENT_SECRET_KEY is required to manage agent API keys")
	}
	return []byte(key), nil
}
