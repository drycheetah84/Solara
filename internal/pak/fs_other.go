//go:build !windows

package pak

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// checkNotInUse is a no-op on Unix: renaming over an open file is allowed.
func checkNotInUse(string) error { return nil }

func isInUse(err error) bool { return errors.Is(err, syscall.ETXTBSY) }

func replaceFile(src, dst string) error { return os.Rename(src, dst) }

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

func freeSpace(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

func isAccessDenied(err error) bool { return errors.Is(err, os.ErrPermission) }
