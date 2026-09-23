//go:build !windows

package proc

import "os/exec"

// SetHideWindow is a no-op on non-Windows platforms.
func SetHideWindow(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}
