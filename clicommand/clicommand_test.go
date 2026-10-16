package clicommand

import (
	"bytes"
	"strings"
	"testing"

	"github.com/terastal-hq/proxylib/accounts"
)

type testAccount struct {
	accounts.BaseAccount
}

func newTestConfig(initial []testAccount) (*bytes.Buffer, Config[testAccount]) {
	buf := new(bytes.Buffer)
	accs := make([]testAccount, len(initial))
	copy(accs, initial)

	cfg := Config[testAccount]{
		AppName: "test-proxy",
		Output:  buf,
		GetAccounts: func() ([]testAccount, error) {
			res := make([]testAccount, len(accs))
			copy(res, accs)
			return res, nil
		},
		UpdateAccounts: func(edit func(*[]testAccount) error) error {
			return edit(&accs)
		},
		GetID:      func(a testAccount) string { return a.ID },
		GetEmail:   func(a testAccount) string { return a.AccountEmail },
		IsEnabled:  func(a testAccount) bool { return a.Enabled },
		SetEnabled: func(a *testAccount, e bool) { a.Enabled = e },
		GetKeys:    func(a testAccount) []accounts.Key { return a.Keys },
		AddKey:     func(a *testAccount, k accounts.Key) { a.Keys = append(a.Keys, k) },
		RemoveKey:  func(a *testAccount, id string) bool { return a.RemoveKey(id) },
		KeyPrefix:  "test_",
		GenerateKey: func() (string, error) {
			return "test_secret123", nil
		},
		GenerateKeyID: func() (string, error) {
			return "key_mock1", nil
		},
	}
	return buf, cfg
}

func TestRunAccounts(t *testing.T) {
	initial := []testAccount{
		{
			BaseAccount: accounts.BaseAccount{
				ID:           "acc-1",
				Label:        "Account 1",
				Enabled:      true,
				AccountEmail: "user@example.com",
			},
		},
	}

	buf, cfg := newTestConfig(initial)

	// List
	if err := RunAccounts([]string{"list"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "acc-1") || !strings.Contains(buf.String(), "user@example.com") {
		t.Fatalf("list output unexpected: %s", buf.String())
	}

	// Disable
	buf.Reset()
	if err := RunAccounts([]string{"disable", "acc-1"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "disabled") {
		t.Fatalf("disable output unexpected: %s", buf.String())
	}
	accs, _ := cfg.GetAccounts()
	if accs[0].IsEnabled() {
		t.Fatal("account was not disabled")
	}

	// Enable
	buf.Reset()
	if err := RunAccounts([]string{"enable", "acc-1"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "enabled") {
		t.Fatalf("enable output unexpected: %s", buf.String())
	}

	// Remove
	buf.Reset()
	if err := RunAccounts([]string{"remove", "acc-1"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "removed") {
		t.Fatalf("remove output unexpected: %s", buf.String())
	}
	accs, _ = cfg.GetAccounts()
	if len(accs) != 0 {
		t.Fatal("account was not removed")
	}

	// Remove not found
	if err := RunAccounts([]string{"remove", "acc-missing"}, cfg); err == nil {
		t.Fatal("expected error on missing account")
	}
}

func TestRunKeys(t *testing.T) {
	initial := []testAccount{
		{
			BaseAccount: accounts.BaseAccount{
				ID:      "acc-1",
				Enabled: true,
			},
		},
	}

	buf, cfg := newTestConfig(initial)

	// Create key
	if err := RunKeys([]string{"create", "acc-1", "--label", "test-label"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "test_secret123") || !strings.Contains(buf.String(), "key_mock1") {
		t.Fatalf("create output unexpected: %s", buf.String())
	}

	// List keys
	buf.Reset()
	if err := RunKeys([]string{"list", "acc-1"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "key_mock1") || !strings.Contains(buf.String(), "test-label") {
		t.Fatalf("list keys output unexpected: %s", buf.String())
	}

	// Revoke key
	buf.Reset()
	if err := RunKeys([]string{"revoke", "acc-1", "key_mock1"}, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "revoked") {
		t.Fatalf("revoke output unexpected: %s", buf.String())
	}
	accs, _ := cfg.GetAccounts()
	if len(accs[0].GetKeys()) != 0 {
		t.Fatal("key was not revoked")
	}
}

func TestRunAdminKey(t *testing.T) {
	buf := new(bytes.Buffer)
	var savedHash string
	update := func(hash string) error {
		savedHash = hash
		return nil
	}

	err := RunAdminKey([]string{"rotate"}, "test-proxy", buf, update, func() (string, error) {
		return "admin_secret_999", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "admin_secret_999") {
		t.Fatalf("admin key output unexpected: %s", buf.String())
	}
	if savedHash == "" {
		t.Fatal("hash was not saved")
	}

	// Invalid args
	if err := RunAdminKey([]string{"bad"}, "test-proxy", buf, update, nil); err == nil {
		t.Fatal("expected error on invalid args")
	}
}
