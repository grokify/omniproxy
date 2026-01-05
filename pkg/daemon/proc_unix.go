//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// checkProcessAlive checks if a process is still running.
func checkProcessAlive(process *os.Process) error {
	return process.Signal(syscall.Signal(0))
}

// setSysProcAttr sets platform-specific process attributes for background execution.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}
}

// signalTerminate sends a termination signal to the process.
func signalTerminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
