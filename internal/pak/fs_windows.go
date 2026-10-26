//go:build windows

package pak

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// checkNotInUse fails with ErrFileInUse when another process holds the pak
// open in a way that would prevent replacing it (no FILE_SHARE_DELETE).
func checkNotInUse(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.DELETE|windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if isInUse(err) {
			return ErrFileInUse
		}
		if isAccessDenied(err) {
			return fmt.Errorf("%w (access denied opening the pak for replacement; if the game is closed, the folder may need write permission)", ErrFileInUse)
		}
		return err
	}
	windows.CloseHandle(h)
	return nil
}

func isInUse(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_USER_MAPPED_FILE)
}

// replaceFile atomically replaces dst with src (MoveFileEx REPLACE_EXISTING
// | WRITE_THROUGH).
func replaceFile(src, dst string) error {
	s, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	d, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(s, d, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}

func syncDir(string) {}

func freeSpace(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}

func isAccessDenied(err error) bool { return errors.Is(err, windows.ERROR_ACCESS_DENIED) }
