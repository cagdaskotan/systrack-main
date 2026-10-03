package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

// CredentialCrypto handles encryption/decryption of sensitive credentials
type CredentialCrypto struct {
	key []byte
}

// NewCredentialCrypto creates a new crypto instance
// Uses AES_ENCRYPTION_KEY environment variable or generates a default key
func NewCredentialCrypto() (*CredentialCrypto, error) {
	// Get encryption key from environment variable
	keyStr := os.Getenv("AES_ENCRYPTION_KEY")

	// If no key in env, use a default key (PRODUCTION: must be in env!)
	if keyStr == "" {
		// Default 32-byte key for AES-256
		// IMPORTANT: This should be changed in production via environment variable
		keyStr = "systrack-default-key-change-me-32byte-aes256-key"
	}

	// Ensure key is 32 bytes for AES-256
	key := []byte(keyStr)
	if len(key) < 32 {
		// Pad the key to 32 bytes
		paddedKey := make([]byte, 32)
		copy(paddedKey, key)
		key = paddedKey
	} else if len(key) > 32 {
		// Truncate to 32 bytes
		key = key[:32]
	}

	return &CredentialCrypto{
		key: key,
	}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
func (cc *CredentialCrypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(cc.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create a nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func (cc *CredentialCrypto) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(cc.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Global crypto instance
var globalCrypto *CredentialCrypto

// InitCrypto initializes the global crypto instance
func InitCrypto() error {
	crypto, err := NewCredentialCrypto()
	if err != nil {
		return err
	}
	globalCrypto = crypto
	return nil
}

// EncryptPassword encrypts a password using the global crypto instance
func EncryptPassword(password string) (string, error) {
	if globalCrypto == nil {
		if err := InitCrypto(); err != nil {
			return "", err
		}
	}
	return globalCrypto.Encrypt(password)
}

// DecryptPassword decrypts a password using the global crypto instance
func DecryptPassword(encrypted string) (string, error) {
	if globalCrypto == nil {
		if err := InitCrypto(); err != nil {
			return "", err
		}
	}
	return globalCrypto.Decrypt(encrypted)
}
