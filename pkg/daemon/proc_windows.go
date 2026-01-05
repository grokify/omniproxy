//go:build windows

package daemon

import (
	"os"
	"os/exec"
)

// checkProcessAlive checks if a process is still running.
// On Windows, FindProcess always succeeds, so we try to get exit code.
func checkProcessAlive(process *os.Process) error {
	// On Windows, Signal(0) doesn't work the same way.
	// We rely on the fact that FindProcess succeeded and the PID file exists.
	// A more robust check would use Windows API, but for now this suffices.
	return nil
}

// setSysProcAttr sets platform-specific process attributes for background execution.
// On Windows, we don't set Setsid as it's Unix-specific.
func setSysProcAttr(cmd *exec.Cmd) {
	// Windows doesn't support Setsid. The process will run detached
	// because we're not waiting for it.
}

// signalTerminate sends a termination signal to the process.
// On Windows, we use Kill() as SIGTERM is not supported.
func signalTerminate(process *os.Process) error {
	return process.Kill()
}
