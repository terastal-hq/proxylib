package accountid

import "testing"

func TestValidate(t *testing.T) {
	valid := []string{"default", "work", "personal", "acc-1", "a", "1", "account-123"}
	for _, id := range valid {
		if err := Validate(id); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",
		"-start",
		"end-",
		"UPPER",
		"has space",
		"has_underscore",
		"a-very-long-account-id-exceeding-thirty-two-chars",
	}
	for _, id := range invalid {
		if err := Validate(id); err == nil {
			t.Errorf("Validate(%q) = nil, want error", id)
		}
	}
}

func TestDerive(t *testing.T) {
	cases := []struct {
		email    string
		existing []string
		want     string
	}{
		{"developer@example.com", nil, "developer"},
		{"DEV.ELOPER@example.com", nil, "dev-eloper"},
		{"first.last+tag@work.co", nil, "first-last-tag"},
		{"user@example.com", []string{"user"}, "user-2"},
		{"user@example.com", []string{"user", "user-2"}, "user-3"},
		{"@empty.com", nil, "account"},
		{"---@empty.com", nil, "account"},
	}
	for _, tc := range cases {
		got := Derive(tc.email, tc.existing)
		if got != tc.want {
			t.Errorf("Derive(%q, %v) = %q, want %q", tc.email, tc.existing, got, tc.want)
		}
		if err := Validate(got); err != nil {
			t.Errorf("Derive produced invalid slug %q: %v", got, err)
		}
	}
}
