//go:build windows

package sysexec

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from briefly flashing when a
// console subprocess is spawned from a GUI app.
const createNoWindow = 0x08000000

func Hide(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
