// Package adminapi provides reusable state-transition mutations for Admin API account and key management.
package adminapi

import (
	"strings"

	"github.com/terastal-hq/proxylib/accounts"
)

// PatchAccountPayload defines the JSON body for patching account metadata.
type PatchAccountPayload struct {
	Label   *string `json:"label"`
	Enabled *bool   `json:"enabled"`
}

// FindAccount searches for an account by its ID.
func FindAccount[A accounts.Account](accList []A, id string) (A, bool) {
	for _, acc := range accList {
		if acc.GetID() == id {
			return acc, true
		}
	}
	var zero A
	return zero, false
}

// ApplyPatchAccount updates an account's Label and Enabled state if found.
func ApplyPatchAccount[A accounts.MutableAccount](accList *[]A, id string, payload PatchAccountPayload) (updated A, found bool) {
	for i := range *accList {
		if (*accList)[i].GetID() == id {
			if payload.Label != nil {
				(*accList)[i].SetLabel(strings.TrimSpace(*payload.Label))
			}
			if payload.Enabled != nil {
				(*accList)[i].SetEnabled(*payload.Enabled)
			}
			return (*accList)[i], true
		}
	}
	return updated, false
}

// ApplyDeleteAccount removes an account by its ID. Returns true if the account was found and removed.
func ApplyDeleteAccount[A accounts.MutableAccount](accList *[]A, id string) bool {
	filtered := make([]A, 0, len(*accList))
	found := false
	for _, acc := range *accList {
		if acc.GetID() == id {
			found = true
			continue
		}
		filtered = append(filtered, acc)
	}
	if found {
		*accList = filtered
	}
	return found
}

// ApplyCreateKey appends a key to the target account. Returns true if the account was found.
func ApplyCreateKey[A accounts.MutableAccount](accList *[]A, id string, key accounts.Key) bool {
	for i := range *accList {
		if (*accList)[i].GetID() == id {
			(*accList)[i].AddKey(key)
			return true
		}
	}
	return false
}

// ApplyDeleteKey removes a key from the target account.
func ApplyDeleteKey[A accounts.MutableAccount](accList *[]A, id, keyID string) (foundAccount, foundKey bool) {
	for i := range *accList {
		if (*accList)[i].GetID() == id {
			foundAccount = true
			if (*accList)[i].RemoveKey(keyID) {
				foundKey = true
				return true, true
			}
			return true, false
		}
	}
	return false, false
}
