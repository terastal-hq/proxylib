package apikey

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	for _, prefix := range []string{"cxp_", "agp_", "test_"} {
		key, err := Generate(prefix)
		if err != nil {
			t.Fatalf("Generate(%q) error = %v", prefix, err)
		}
		if !strings.HasPrefix(key, prefix) {
			t.Fatalf("Generate(%q) key = %q, want prefix %q", prefix, key, prefix)
		}
		if len(key) != len(prefix)+DefaultEntropyChars {
			t.Fatalf("Generate(%q) key length = %d, want %d", prefix, len(key), len(prefix)+DefaultEntropyChars)
		}
		chars := key[len(prefix):]
		for i, c := range chars {
			if !strings.ContainsRune(base62Chars, c) {
				t.Fatalf("Generate(%q) character at %d = %c is not in base62", prefix, i, c)
			}
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		key, err := Generate("test_")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if seen[key] {
			t.Fatalf("duplicate key generated: %q", key)
		}
		seen[key] = true
	}
}

func TestGenerateWithEntropy(t *testing.T) {
	_, err := GenerateWithEntropy("cxp_", 0)
	if err == nil {
		t.Fatalf("expected error for 0 entropy chars")
	}
	key, err := GenerateWithEntropy("cxp_", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != len("cxp_")+10 {
		t.Fatalf("expected length %d, got %d", len("cxp_")+10, len(key))
	}
}

func TestGenerateID(t *testing.T) {
	id, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID failed: %v", err)
	}
	if !strings.HasPrefix(id, "key_") {
		t.Fatalf("expected id to start with 'key_', got %q", id)
	}
	if len(id) != 12 { // "key_" (4) + 8 hex chars = 12
		t.Fatalf("expected id length 12, got %d", len(id))
	}
}

func TestHashAndMatch(t *testing.T) {
	secret := "test_secret_12345"
	h := Hash(secret)

	if !strings.HasPrefix(h, HashPrefix) {
		t.Fatalf("Hash(%q) = %q, want prefix %q", secret, h, HashPrefix)
	}
	if !Match(secret, h) {
		t.Fatalf("Match failed for secret and its hash")
	}
	if Match("wrong_secret", h) {
		t.Fatalf("Match succeeded for incorrect secret")
	}

	// Plaintext matching fallback
	if !Match("plain_secret", "plain_secret") {
		t.Fatalf("Match failed for identical plaintext")
	}
	if Match("plain_secret", "other") {
		t.Fatalf("Match succeeded for mismatched plaintext")
	}
}

func TestLast4(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"cxp_12345678", "5678"},
		{"agp_abcdef12", "ef12"},
		{"abcd", "abcd"},
		{"ab", "ab"},
		{"", ""},
	}
	for _, tc := range cases {
		got := Last4(tc.input)
		if got != tc.want {
			t.Errorf("Last4(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRegistry(t *testing.T) {
	key1, _ := Generate("cxp_")
	key2, _ := Generate("agp_")

	bindings := map[string]Binding{
		Hash(key1): {AccountID: "acc1", KeyID: "k1"},
		Hash(key2): {AccountID: "acc2", KeyID: "k2"},
	}

	reg, err := NewRegistry(bindings)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	if reg.Size() != 2 {
		t.Fatalf("expected size 2, got %d", reg.Size())
	}
	if !reg.HasKeyID("k1") || !reg.HasKeyID("k2") {
		t.Fatalf("HasKeyID failed")
	}
	if reg.HasKeyID("k3") {
		t.Fatalf("HasKeyID succeeded for nonexistent key")
	}

	b, ok := reg.Lookup(key1)
	if !ok || b.AccountID != "acc1" || b.KeyID != "k1" {
		t.Fatalf("Lookup key1 failed: got %+v, %v", b, ok)
	}

	b, ok = reg.Lookup(key2)
	if !ok || b.AccountID != "acc2" || b.KeyID != "k2" {
		t.Fatalf("Lookup key2 failed: got %+v, %v", b, ok)
	}

	_, ok = reg.Lookup("invalid_key")
	if ok {
		t.Fatalf("Lookup invalid_key should fail")
	}

	allBindings := reg.Bindings()
	if len(allBindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(allBindings))
	}

	// Invalid hash prefix in NewRegistry
	_, err = NewRegistry(map[string]Binding{"invalid_hash": {AccountID: "acc"}})
	if err == nil {
		t.Fatalf("expected error for invalid hash prefix")
	}

	// Invalid hex in NewRegistry
	_, err = NewRegistry(map[string]Binding{"sha256:nothex": {AccountID: "acc"}})
	if err == nil {
		t.Fatalf("expected error for invalid hex in hash")
	}

	// Duplicate hash error
	duplicateEntries := map[string]Binding{
		Hash(key1): {AccountID: "acc1"},
	}
	reg1, err := NewRegistry(duplicateEntries)
	if err != nil {
		t.Fatalf("failed creating valid registry: %v", err)
	}
	if reg1.Size() != 1 {
		t.Fatalf("expected size 1, got %d", reg1.Size())
	}
}
