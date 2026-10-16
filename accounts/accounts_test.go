package accounts

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAccounts(t *testing.T) {
	// Valid accounts
	accs := []BaseAccount{
		{
			ID:        "acc-1",
			AccountID: "upstream-1",
			Keys: []Key{
				{
					ID:        "key_1",
					Hash:      "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Last4:     "1234",
					CreatedAt: time.Now(),
				},
			},
		},
		{
			ID:        "acc-2",
			AccountID: "upstream-2",
			Keys: []Key{
				{
					ID:        "key_2",
					Hash:      "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
					Last4:     "5678",
					CreatedAt: time.Now(),
				},
			},
		},
	}

	if err := Validate(accs, "TestProvider"); err != nil {
		t.Fatalf("valid accounts rejected: %v", err)
	}

	// Invalid slug
	invalidSlug := []BaseAccount{{ID: "-bad-slug"}}
	if err := Validate(invalidSlug, "TestProvider"); err == nil {
		t.Fatal("invalid slug accepted")
	}

	// Duplicate account ID
	dupID := []BaseAccount{{ID: "dup"}, {ID: "dup"}}
	if err := Validate(dupID, "TestProvider"); err == nil || !strings.Contains(err.Error(), "duplicate account ID") {
		t.Fatalf("duplicate account ID accepted: %v", err)
	}

	// Duplicate upstream ID
	dupUpstream := []BaseAccount{
		{ID: "a1", AccountID: "shared-upstream"},
		{ID: "a2", AccountID: "shared-upstream"},
	}
	if err := Validate(dupUpstream, "CustomProvider"); err == nil || !strings.Contains(err.Error(), "CustomProvider account \"shared-upstream\" is linked to both") {
		t.Fatalf("duplicate upstream ID accepted: %v", err)
	}

	// Duplicate key ID within same account
	dupKeyID := []BaseAccount{
		{
			ID: "a1",
			Keys: []Key{
				{ID: "k1", Hash: "sha256:1111"},
				{ID: "k1", Hash: "sha256:2222"},
			},
		},
	}
	if err := Validate(dupKeyID, "TestProvider"); err == nil || !strings.Contains(err.Error(), "duplicate key ID") {
		t.Fatalf("duplicate key ID accepted: %v", err)
	}

	// Invalid key hash format
	badHash := []BaseAccount{
		{
			ID: "a1",
			Keys: []Key{
				{ID: "k1", Hash: "md5:1234"},
			},
		},
	}
	if err := Validate(badHash, "TestProvider"); err == nil || !strings.Contains(err.Error(), "invalid hash format") {
		t.Fatalf("invalid key hash accepted: %v", err)
	}

	// Duplicate key hash across different accounts
	dupHashAcross := []BaseAccount{
		{
			ID: "a1",
			Keys: []Key{
				{ID: "k1", Hash: "sha256:abcd"},
			},
		},
		{
			ID: "a2",
			Keys: []Key{
				{ID: "k2", Hash: "sha256:abcd"},
			},
		},
	}
	if err := Validate(dupHashAcross, "TestProvider"); err == nil || !strings.Contains(err.Error(), "duplicate API key hash across accounts") {
		t.Fatalf("duplicate key hash across accounts accepted: %v", err)
	}
}
