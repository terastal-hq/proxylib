package privatefile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsurePrivateDirAndInspect(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "private")
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatalf("EnsurePrivateDir failed: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("perm = %o, want 700", info.Mode().Perm())
	}

	target := filepath.Join(dir, "file.json")
	if err := InspectFile(target); err != nil {
		t.Fatalf("InspectFile on non-existent failed: %v", err)
	}
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InspectFile(target); err != nil {
		t.Fatalf("InspectFile on regular file failed: %v", err)
	}

	// Symlink rejection
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.json")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := InspectFile(link); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("expected symlink error, got %v", err)
		}
	}
}

func TestAtomicWriteAndReadStrictJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{"version": 1, "name": "proxy"}`)

	if err := AtomicWrite(path, data); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	read, err := ReadStrictJSON(path)
	if err != nil {
		t.Fatalf("ReadStrictJSON failed: %v", err)
	}
	if !strings.Contains(string(read), `"proxy"`) {
		t.Fatalf("ReadStrictJSON got %s", read)
	}

	// Non-existent file returns nil, nil
	missing, err := ReadStrictJSON(filepath.Join(dir, "missing.json"))
	if err != nil || missing != nil {
		t.Fatalf("missing file expected nil, nil; got %v, %v", missing, err)
	}

	// Invalid non-object JSON
	invalidPath := filepath.Join(dir, "invalid.json")
	if err := AtomicWrite(invalidPath, []byte(`"not an object"`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStrictJSON(invalidPath); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("expected JSON object error, got %v", err)
	}

	// Trailing garbage or second object
	for _, bad := range []string{`{"a":1} garbage`, `{"a":1} {"b":2}`} {
		badPath := filepath.Join(dir, "bad.json")
		if err := AtomicWrite(badPath, []byte(bad)); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadStrictJSON(badPath); err == nil {
			t.Fatalf("accepted trailing payload %q", bad)
		}
	}
}

func TestRejectDuplicateFields(t *testing.T) {
	valid := []byte(`{"foo": 1, "bar": 2}`)
	if err := RejectDuplicateFields(valid); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}

	duplicate := []byte(`{"foo": 1, "FOO": 2}`)
	if err := RejectDuplicateFields(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate keys accepted: %v", err)
	}
}

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	lock1, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	defer lock1.Close()

	// Second lock should fail immediately
	lock2, err := AcquireLock(lockPath)
	if err == nil {
		_ = lock2.Close()
		t.Fatal("second acquire succeeded unexpectedly")
	}

	// Release first lock
	if err := lock1.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Now second lock should succeed
	lock3, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("failed to acquire lock after release: %v", err)
	}
	_ = lock3.Close()
}
