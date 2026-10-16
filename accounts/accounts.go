// Package accounts provides shared account models, API key definitions, and generic account validation.
package accounts

import (
	"fmt"
	"strings"
	"time"

	"github.com/terastal-hq/proxylib/accountid"
	"github.com/terastal-hq/proxylib/apikey"
)

// Key represents an API key owned by an account.
type Key struct {
	ID        string    `json:"id"`
	Label     string    `json:"label,omitempty"`
	Hash      string    `json:"hash"`
	Last4     string    `json:"last4"`
	CreatedAt time.Time `json:"createdAt"`
}

// BaseAccount provides common fields shared across all proxy account representations.
type BaseAccount struct {
	ID           string `json:"id"`
	Label        string `json:"label,omitempty"`
	Enabled      bool   `json:"enabled"`
	AccountID    string `json:"accountId,omitempty"`
	AccountEmail string `json:"accountEmail,omitempty"`
	AccountName  string `json:"accountName,omitempty"`
	Keys         []Key  `json:"keys,omitempty"`
}

func (b BaseAccount) GetID() string         { return b.ID }
func (b BaseAccount) GetUpstreamID() string { return b.AccountID }
func (b BaseAccount) GetKeys() []Key        { return b.Keys }
func (b BaseAccount) GetLabel() string      { return b.Label }
func (b BaseAccount) GetEmail() string      { return b.AccountEmail }
func (b BaseAccount) IsEnabled() bool       { return b.Enabled }
func (b *BaseAccount) SetLabel(l string)    { b.Label = l }
func (b *BaseAccount) SetEnabled(e bool)    { b.Enabled = e }
func (b *BaseAccount) AddKey(k Key)         { b.Keys = append(b.Keys, k) }
func (b *BaseAccount) RemoveKey(keyID string) bool {
	filtered := make([]Key, 0, len(b.Keys))
	found := false
	for _, k := range b.Keys {
		if k.ID == keyID {
			found = true
			continue
		}
		filtered = append(filtered, k)
	}
	if found {
		b.Keys = filtered
	}
	return found
}

// Account represents any proxy account type that provides account ID, upstream ID, and keys.
type Account interface {
	GetID() string
	GetUpstreamID() string
	GetKeys() []Key
}

// MutableAccount extends Account with mutation methods for CLI and Admin operations.
type MutableAccount interface {
	Account
	GetLabel() string
	GetEmail() string
	IsEnabled() bool
	SetLabel(string)
	SetEnabled(bool)
	AddKey(Key)
	RemoveKey(keyID string) bool
}

// Validate validates a collection of accounts for slug constraints, ID uniqueness,
// 1:1 upstream identity mapping, and API key hash format and uniqueness.
func Validate[A Account](accounts []A, providerDisplayName string) error {
	seenIDs := make(map[string]bool, len(accounts))
	seenUpstreamIDs := make(map[string]string, len(accounts))
	seenKeyHashes := make(map[string]string)

	for _, acc := range accounts {
		id := acc.GetID()
		if err := accountid.Validate(id); err != nil {
			return err
		}
		if seenIDs[id] {
			return fmt.Errorf("duplicate account ID %q", id)
		}
		seenIDs[id] = true

		if upstream := acc.GetUpstreamID(); upstream != "" {
			if existingAcc, exists := seenUpstreamIDs[upstream]; exists {
				return fmt.Errorf("%s account %q is linked to both %q and %q", providerDisplayName, upstream, existingAcc, id)
			}
			seenUpstreamIDs[upstream] = id
		}

		keys := acc.GetKeys()
		keyIDs := make(map[string]bool, len(keys))
		for _, key := range keys {
			if key.ID == "" {
				return fmt.Errorf("account %q has key with empty ID", id)
			}
			if keyIDs[key.ID] {
				return fmt.Errorf("account %q has duplicate key ID %q", id, key.ID)
			}
			keyIDs[key.ID] = true

			if !strings.HasPrefix(key.Hash, apikey.HashPrefix) {
				return fmt.Errorf("account %q key %q has invalid hash format", id, key.ID)
			}
			if existingAcc, exists := seenKeyHashes[key.Hash]; exists {
				return fmt.Errorf("duplicate API key hash across accounts %q and %q", existingAcc, id)
			}
			seenKeyHashes[key.Hash] = id
		}
	}
	return nil
}
