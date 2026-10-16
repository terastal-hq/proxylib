// Package apikey provides key generation, hashing, verification, and registry lookup
// for account and administration API keys across proxy services.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	// HashPrefix identifies canonical SHA-256 hashed keys.
	HashPrefix = "sha256:"
	// RandomKeyBytes is the default entropy size used for generating keys.
	RandomKeyBytes = 32
	// DefaultEntropyChars is the default number of base62 characters appended to the prefix.
	DefaultEntropyChars = 43
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Generate creates a cryptographically secure API key with the given prefix
// followed by 43 base62 characters (representing approximately 256 bits of entropy).
func Generate(prefix string) (string, error) {
	return GenerateWithEntropy(prefix, DefaultEntropyChars)
}

// GenerateWithEntropy creates a cryptographically secure API key with prefix
// followed by entropyChars base62 characters.
func GenerateWithEntropy(prefix string, entropyChars int) (string, error) {
	if entropyChars < 1 {
		return "", errors.New("entropy characters must be positive")
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	charsLen := big.NewInt(int64(len(base62Chars)))
	for i := 0; i < entropyChars; i++ {
		n, err := rand.Int(rand.Reader, charsLen)
		if err != nil {
			return "", fmt.Errorf("generate api key: %w", err)
		}
		sb.WriteByte(base62Chars[n.Int64()])
	}
	return sb.String(), nil
}

// GenerateID creates a random short identifier for an API key (e.g., "key_3f9a1c2e").
func GenerateID() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate key id: %w", err)
	}
	return "key_" + hex.EncodeToString(buf[:]), nil
}

// Hash returns the canonical SHA-256 hash representation of a secret: "sha256:<hex>".
func Hash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return HashPrefix + hex.EncodeToString(h[:])
}

// HashBytes computes the raw [32]byte SHA-256 digest of a secret.
func HashBytes(secret string) [32]byte {
	return sha256.Sum256([]byte(secret))
}

// Last4 returns the trailing 4 characters of a secret for display purposes.
// If the secret is shorter than 4 characters, it returns the full secret.
func Last4(secret string) string {
	if len(secret) <= 4 {
		return secret
	}
	return secret[len(secret)-4:]
}

// Match compares a raw secret against an expected hash (or plaintext key) in constant time.
func Match(secret, expected string) bool {
	if strings.HasPrefix(expected, HashPrefix) {
		h := Hash(secret)
		return subtle.ConstantTimeCompare([]byte(h), []byte(expected)) == 1
	}
	// Fallback when expected is stored as plaintext (e.g. from environment variable)
	return subtle.ConstantTimeCompare([]byte(secret), []byte(expected)) == 1
}

// Binding associates an API key with its owning account and key metadata.
type Binding struct {
	AccountID string
	KeyID     string
}

// Registry provides constant-time lookup from raw API keys to their account bindings.
// It is immutable once created.
type Registry struct {
	entries map[[32]byte]Binding
}

// NewRegistry constructs an immutable Registry from a list of key hash bindings.
func NewRegistry(bindings map[string]Binding) (*Registry, error) {
	entries := make(map[[32]byte]Binding, len(bindings))
	for hashStr, b := range bindings {
		digest, err := parseHash(hashStr)
		if err != nil {
			return nil, err
		}
		if _, exists := entries[digest]; exists {
			return nil, fmt.Errorf("duplicate api key hash in registry: %s", hashStr)
		}
		entries[digest] = b
	}
	return &Registry{entries: entries}, nil
}

func parseHash(hashStr string) ([32]byte, error) {
	var digest [32]byte
	if !strings.HasPrefix(hashStr, HashPrefix) {
		return digest, errors.New("key hash must start with 'sha256:'")
	}
	hexPart := strings.TrimPrefix(hashStr, HashPrefix)
	decoded, err := hex.DecodeString(hexPart)
	if err != nil || len(decoded) != 32 {
		return digest, fmt.Errorf("invalid sha256 hex encoding: %s", hexPart)
	}
	copy(digest[:], decoded)
	return digest, nil
}

// Lookup finds the account binding for a given raw secret.
// Returns false if the key is not in the registry.
func (reg *Registry) Lookup(secret string) (Binding, bool) {
	if reg == nil || len(reg.entries) == 0 || secret == "" {
		return Binding{}, false
	}
	digest := HashBytes(secret)
	binding, found := reg.entries[digest]
	return binding, found
}

// Size returns the number of keys registered.
func (reg *Registry) Size() int {
	if reg == nil {
		return 0
	}
	return len(reg.entries)
}

// HasKeyID returns true if any key in the registry has the specified key ID.
func (reg *Registry) HasKeyID(keyID string) bool {
	if reg == nil {
		return false
	}
	for _, b := range reg.entries {
		if b.KeyID == keyID {
			return true
		}
	}
	return false
}

// Bindings returns a map of hash strings to their bindings for registry copying.
func (reg *Registry) Bindings() map[string]Binding {
	if reg == nil {
		return nil
	}
	result := make(map[string]Binding, len(reg.entries))
	for digest, b := range reg.entries {
		result[HashPrefix+hex.EncodeToString(digest[:])] = b
	}
	return result
}

// KeyMetadata describes a key without exposing its hash or secret.
type KeyMetadata struct {
	ID        string     `json:"id"`
	Label     string     `json:"label,omitempty"`
	Last4     string     `json:"last4"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used_at,omitempty"`
}
