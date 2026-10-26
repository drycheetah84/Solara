//go:build windows

package host

import (
	"os"

	"golang.org/x/sys/windows"
)

// IsElevated mirrors ModuleInitialization.IsElevated on Windows
// (member of the built-in Administrators role, i.e. an elevated token).
func IsElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// restartElevated mirrors TryRestartWithElevatedPrivileges on Windows:
// ShellExecute the same executable with the "runas" verb, then exit(0).
func restartElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_NORMAL); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
