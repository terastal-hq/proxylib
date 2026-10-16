package adminapi

import (
	"testing"
	"time"

	"github.com/terastal-hq/proxylib/accounts"
)

type testAccount struct {
	accounts.BaseAccount
}

func TestAdminAPIMutations(t *testing.T) {
	accs := []*testAccount{
		{
			BaseAccount: accounts.BaseAccount{
				ID:      "acc-1",
				Label:   "Original",
				Enabled: false,
				Keys: []accounts.Key{
					{ID: "k1", Hash: "sha256:1111", CreatedAt: time.Now()},
				},
			},
		},
	}

	// FindAccount
	foundAcc, ok := FindAccount(accs, "acc-1")
	if !ok || foundAcc.GetID() != "acc-1" {
		t.Fatal("FindAccount failed")
	}
	_, ok = FindAccount(accs, "missing")
	if ok {
		t.Fatal("FindAccount succeeded on missing")
	}

	// ApplyPatchAccount
	newLabel := "Updated"
	newEnabled := true
	updated, ok := ApplyPatchAccount(&accs, "acc-1", PatchAccountPayload{
		Label:   &newLabel,
		Enabled: &newEnabled,
	})
	if !ok || updated.GetLabel() != "Updated" || !updated.IsEnabled() {
		t.Fatalf("ApplyPatchAccount failed: %v, %+v", ok, updated)
	}

	// ApplyCreateKey
	newKey := accounts.Key{ID: "k2", Hash: "sha256:2222", CreatedAt: time.Now()}
	if !ApplyCreateKey(&accs, "acc-1", newKey) {
		t.Fatal("ApplyCreateKey failed")
	}
	if len(accs[0].GetKeys()) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(accs[0].GetKeys()))
	}
	if ApplyCreateKey(&accs, "missing", newKey) {
		t.Fatal("ApplyCreateKey succeeded on missing")
	}

	// ApplyDeleteKey
	foundAccount, foundKey := ApplyDeleteKey(&accs, "acc-1", "k1")
	if !foundAccount || !foundKey || len(accs[0].GetKeys()) != 1 {
		t.Fatalf("ApplyDeleteKey failed: %v, %v", foundAccount, foundKey)
	}
	foundAccount, foundKey = ApplyDeleteKey(&accs, "acc-1", "k_missing")
	if !foundAccount || foundKey {
		t.Fatalf("expected foundAccount true and foundKey false, got %v, %v", foundAccount, foundKey)
	}

	// ApplyDeleteAccount
	if !ApplyDeleteAccount(&accs, "acc-1") || len(accs) != 0 {
		t.Fatal("ApplyDeleteAccount failed")
	}
	if ApplyDeleteAccount(&accs, "acc-1") {
		t.Fatal("ApplyDeleteAccount succeeded again on deleted account")
	}
}
