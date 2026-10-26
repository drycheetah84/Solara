//go:build !windows

package host

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// IsElevated mirrors ModuleInitialization.IsElevated: geteuid()==0 on Linux,
// false on other non-Windows platforms.
func IsElevated() bool {
	if runtime.GOOS == "linux" {
		return os.Geteuid() == 0
	}
	return false
}

// restartElevated mirrors TryRestartWithElevatedPrivileges on Linux:
// `sudo <exe>` (no arguments forwarded, as in the original), then exit(0).
func restartElevated() error {
	if runtime.GOOS != "linux" {
		return errors.New("Operation is not supported on this platform.")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command("sudo", exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
