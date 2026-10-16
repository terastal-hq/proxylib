// Package accountid provides validation and slug derivation for multi-account identifiers.
package accountid

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var accountIDRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}[a-z0-9]$|^[a-z0-9]$`)

// Validate checks whether id satisfies the account slug requirements:
// 1-32 characters, lowercase alphanumeric and hyphens, starting and ending with alphanumeric.
func Validate(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("account ID must not be empty")
	}
	if len(id) > 32 {
		return fmt.Errorf("account ID %q exceeds maximum length of 32 characters", id)
	}
	if !accountIDRegex.MatchString(id) {
		return fmt.Errorf("account ID %q must contain only lowercase letters, numbers, and internal hyphens", id)
	}
	return nil
}

// Derive generates a unique, valid account slug from an email address.
func Derive(email string, existingIDs []string) string {
	parts := strings.Split(email, "@")
	raw := strings.ToLower(strings.TrimSpace(parts[0]))

	var sb strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	slug := strings.Trim(sb.String(), "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		slug = "account"
	}
	if len(slug) > 28 {
		slug = strings.TrimRight(slug[:28], "-")
	}

	candidate := slug
	counter := 2
	for slices.Contains(existingIDs, candidate) {
		suffix := "-" + strconv.Itoa(counter)
		if len(slug)+len(suffix) > 32 {
			candidate = strings.TrimRight(slug[:32-len(suffix)], "-") + suffix
		} else {
			candidate = slug + suffix
		}
		counter++
	}
	return candidate
}
