//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package privatefile

import (
	"errors"
	"os"
)

func lockFile(*os.File) error {
	return errors.New("platform locking unsupported")
}

func renameFile(from, to string) error {
	return os.Rename(from, to)
}

func syncDirectory(string) error {
	return nil
}
