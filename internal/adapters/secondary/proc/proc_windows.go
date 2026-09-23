//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// SetHideWindow suppresses the console window creation on Windows when launching subprocesses.
func SetHideWindow(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags = 0x08000000 // CREATE_NO_WINDOW
	return cmd
}
