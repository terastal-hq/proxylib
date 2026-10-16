package configfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripKeys(t *testing.T) {
	fields := map[string]json.RawMessage{
		"apiKey":       json.RawMessage(`"secret"`),
		"AccessToken":  json.RawMessage(`"token"`),
		"RefreshToken": json.RawMessage(`"refresh"`),
		"accounts":     json.RawMessage(`[]`),
	}

	StripKeys(fields, "apikey", "ACCESSTOKEN")
	if _, ok := fields["apiKey"]; ok {
		t.Fatal("apiKey was not stripped")
	}
	if _, ok := fields["AccessToken"]; ok {
		t.Fatal("AccessToken was not stripped")
	}
	if _, ok := fields["RefreshToken"]; !ok {
		t.Fatal("RefreshToken was unexpectedly stripped")
	}
	if _, ok := fields["accounts"]; !ok {
		t.Fatal("accounts was unexpectedly stripped")
	}

	// nil safety
	StripKeys(nil, "foo")
}

func TestPatchAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Patch initial missing file
	err := Patch(path, func(fields map[string]json.RawMessage) error {
		fields["name"] = json.RawMessage(`"test"`)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("Patch on missing file failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"name": "test"`) {
		t.Fatalf("unexpected content after initial patch: %s, %v", data, err)
	}

	// Patch creates missing parent directories
	nestedPath := filepath.Join(dir, "sub", "sub2", "config.json")
	err = Patch(nestedPath, func(fields map[string]json.RawMessage) error {
		fields["nested"] = json.RawMessage(`true`)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("Patch with missing parent failed: %v", err)
	}
	nestedData, err := os.ReadFile(nestedPath)
	if err != nil || !strings.Contains(string(nestedData), `"nested": true`) {
		t.Fatalf("nested patch content invalid: %s, %v", nestedData, err)
	}

	// Patch with validation failure
	validationErr := errors.New("invalid port")
	err = Patch(path, func(fields map[string]json.RawMessage) error {
		fields["port"] = json.RawMessage(`-1`)
		return nil
	}, func(candidate []byte) error {
		return validationErr
	})
	if err == nil || !errors.Is(err, ErrUnloadableConfig) {
		t.Fatalf("expected ErrUnloadableConfig, got %v", err)
	}

	// Verify file was NOT modified after validation failure
	dataAfterFail, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(dataAfterFail), "port") {
		t.Fatalf("file was modified after validation failure: %s", dataAfterFail)
	}

	// Successful update and key strip
	err = Patch(path, func(fields map[string]json.RawMessage) error {
		fields["version"] = json.RawMessage(`2`)
		StripKeys(fields, "name")
		return nil
	}, func(candidate []byte) error {
		return nil
	})
	if err != nil {
		t.Fatalf("valid patch failed: %v", err)
	}

	finalData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(finalData), "name") {
		t.Fatalf("name was not stripped: %s", finalData)
	}
	if !strings.Contains(string(finalData), `"version": 2`) {
		t.Fatalf("version was not added: %s", finalData)
	}
}
