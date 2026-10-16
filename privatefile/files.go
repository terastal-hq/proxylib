// Package privatefile provides secure, owner-only atomic file persistence and file locks.
package privatefile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CommitError means rename succeeded but directory durability could not be confirmed.
// Callers must reflect the replacement in memory, not pretend the old file survived.
type CommitError struct{ Err error }

func (e *CommitError) Error() string { return "file replaced but durability is uncertain" }
func (e *CommitError) Unwrap() error { return e.Err }

// InspectFile checks that the path is a regular file and not a symbolic link.
// If the file does not exist, it returns nil.
func InspectFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("storage path must not be a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return errors.New("storage path must be a regular file, not a directory or device")
	}
	return nil
}

// EnsurePrivateDir creates a directory with owner-only permissions (0700) and
// verifies it is not a symlink and has appropriate private permissions on non-Windows platforms.
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("storage directory must be a directory, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("storage directory must be private (chmod 700)")
	}
	return nil
}

// AtomicWrite safely and durably replaces the target file using a temporary file in the same directory.
func AtomicWrite(path string, data []byte) error {
	if err := InspectFile(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := file.Name()
	defer func() { _ = os.Remove(tmpName) }()
	defer func() { _ = file.Close() }()

	if err := file.Chmod(0o600); err != nil {
		return err
	}
	n, err := file.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := InspectFile(path); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return &CommitError{Err: err}
	}
	return nil
}

// SyncDir flushes the directory entries to disk on supported platforms.
func SyncDir(path string) error {
	return syncDirectory(path)
}

// ReadStrictJSON reads a JSON configuration file, requiring owner-only permissions (on non-Windows),
// a regular file, a non-empty JSON object, and rejecting duplicate case-insensitive top-level keys.
func ReadStrictJSON(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file %s must be a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("file %s must have owner-only permissions (chmod 600)", path)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("file %s must contain a JSON object", path)
	}
	if err := RejectDuplicateFields(data); err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return data, nil
}

// RejectDuplicateFields requires exactly one complete JSON object and ensures
// its top-level keys are case-insensitively unique.
func RejectDuplicateFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	if opening != json.Delim('{') {
		return errors.New("config must contain exactly one JSON object")
	}
	var keys []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			continue
		}
		for _, previous := range keys {
			if strings.EqualFold(key, previous) {
				return fmt.Errorf("duplicate case-insensitive top-level config fields: %q", key)
			}
		}
		keys = append(keys, key)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return errors.New("config must contain a complete JSON object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("config must contain exactly one JSON object")
	}
	return nil
}

// Lock represents an advisory exclusive file lock.
type Lock struct {
	file *os.File
}

// AcquireLock opens and locks the target path exclusively with non-blocking mode.
func AcquireLock(path string) (*Lock, error) {
	if err := InspectFile(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, errors.New("file is already locked by another process or locking is unavailable")
	}
	return &Lock{file: file}, nil
}

// Close releases the lock and closes the underlying descriptor.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
