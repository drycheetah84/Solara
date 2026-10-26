package host

import (
	"os"

	"solara/internal/logging"
)

// ModuleInitialize mirrors ModuleInitialization.Initialize: self-elevation is
// opt-in via MOTHERSHIP_SELF_ELEVATE=1|true (the low ports are normally
// granted with urlacl/capabilities instead), then the "Main" logger.
func ModuleInitialize() {
	if v := os.Getenv("MOTHERSHIP_SELF_ELEVATE"); v == "1" || v == "true" {
		TryRestartWithElevatedPrivileges()
	}
	logging.Main = logging.New("Main")
}

// TryRestartWithElevatedPrivileges mirrors the original: no-op when already
// elevated; on failure prints the same message and continues.
func TryRestartWithElevatedPrivileges() {
	if IsElevated() {
		return
	}
	if err := restartElevated(); err != nil {
		logging.Console("Failed to restart with elevated privileges: %s", err.Error())
	}
}
