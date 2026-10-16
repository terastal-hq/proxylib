// Package configfile provides atomic JSON configuration file reading, patching, and legacy field cleanup.
package configfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/terastal-hq/proxylib/privatefile"
)

// ErrUnloadableConfig is returned when a patched configuration fails candidate validation.
var ErrUnloadableConfig = errors.New("change rejected: the resulting configuration would not load at startup")

// StripKeys deletes keys case-insensitively from a JSON field map.
func StripKeys(fields map[string]json.RawMessage, keys ...string) {
	if fields == nil || len(keys) == 0 {
		return
	}
	for _, target := range keys {
		for key := range fields {
			if strings.EqualFold(key, target) {
				delete(fields, key)
			}
		}
	}
}

// ReadFields loads a JSON file and parses top-level fields into a map.
// If the file does not exist, an empty map is returned.
func ReadFields(path string) (map[string]json.RawMessage, error) {
	data, err := privatefile.ReadStrictJSON(path)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	if len(data) == 0 {
		return fields, nil
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse config fields: %w", err)
	}
	return fields, nil
}

// Patch reads the JSON configuration at path, invokes edit to modify fields,
// formats the JSON, invokes validate on the candidate data, and durably replaces the file.
func Patch(path string, edit func(fields map[string]json.RawMessage) error, validate func([]byte) error) error {
	if edit == nil {
		return nil
	}
	fields, err := ReadFields(path)
	if err != nil {
		return err
	}

	if err := edit(fields); err != nil {
		return err
	}

	outData, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if validate != nil {
		if err := validate(outData); err != nil {
			return fmt.Errorf("%w: %v", ErrUnloadableConfig, err)
		}
	}

	return privatefile.AtomicWrite(path, outData)
}
