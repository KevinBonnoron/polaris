//go:build !windows

package sysexec

import "os/exec"

func Hide(_ *exec.Cmd) {}
