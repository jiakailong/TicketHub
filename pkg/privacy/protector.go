package privacy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Protector encrypts reversible private fields and builds deterministic blind indexes.
// Encryption keys and the lookup key must be independent.
type Protector struct {
	activeVersion string
	keys          map[string][]byte
	lookupKey     []byte
}

func NewProtector(activeVersion string, encodedKeys map[string]string, encodedLookupKey string) (*Protector, error) {
	activeVersion = strings.TrimSpace(activeVersion)
	if activeVersion == "" {
		return nil, fmt.Errorf("privacy active key version is required")
	}
	keys := make(map[string][]byte, len(encodedKeys))
	for version, encoded := range encodedKeys {
		version = strings.TrimSpace(version)
		key, err := decodeKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode privacy encryption key %s: %w", version, err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("privacy encryption key %s must be 32 bytes", version)
		}
		keys[version] = key
	}
	if _, ok := keys[activeVersion]; !ok {
		return nil, fmt.Errorf("privacy active encryption key %s is not configured", activeVersion)
	}
	lookupKey, err := decodeKey(encodedLookupKey)
	if err != nil {
		return nil, fmt.Errorf("decode privacy lookup key: %w", err)
	}
	if len(lookupKey) < 32 {
		return nil, fmt.Errorf("privacy lookup key must be at least 32 bytes")
	}
	return &Protector{activeVersion: activeVersion, keys: keys, lookupKey: lookupKey}, nil
}

func (p *Protector) ActiveVersion() string {
	return p.activeVersion
}

func (p *Protector) Encrypt(plaintext string, additionalData []byte) ([]byte, string, error) {
	if plaintext == "" {
		return nil, p.activeVersion, nil
	}
	aead, err := p.aead(p.activeVersion)
	if err != nil {
		return nil, "", err
	}
	// NewGCMWithRandomNonce prepends a fresh nonce and appends the authentication tag.
	ciphertext := aead.Seal(nil, nil, []byte(plaintext), additionalData)
	return ciphertext, p.activeVersion, nil
}

func (p *Protector) Decrypt(ciphertext []byte, version string, additionalData []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	aead, err := p.aead(version)
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, additionalData)
	if err != nil {
		return "", fmt.Errorf("decrypt private field: %w", err)
	}
	return string(plaintext), nil
}

func (p *Protector) Lookup(value string) []byte {
	mac := hmac.New(sha256.New, p.lookupKey)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (p *Protector) aead(version string) (cipher.AEAD, error) {
	key, ok := p.keys[strings.TrimSpace(version)]
	if !ok {
		return nil, fmt.Errorf("privacy encryption key %s is not configured", version)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func decodeKey(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
}
