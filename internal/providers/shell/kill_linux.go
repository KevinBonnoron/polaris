//go:build linux

package shell

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// killTerminalProcesses kills all processes belonging to the terminal session
// identified by the shell's PID (which is also the session ID, since pty.Start
// sets Setsid=true making the shell a session leader). Bash's job control puts
// each foreground command in its own process group, so killing only the shell
// process leaves children (e.g. npm, node) alive and holding ports.
func killTerminalProcesses(shellPID int) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		// /proc/pid/stat: pid (comm) state ppid pgrp session ...
		// comm may contain spaces and parentheses, find last ")" to skip it.
		s := string(data)
		rp := strings.LastIndex(s, ")")
		if rp < 0 {
			continue
		}
		fields := strings.Fields(s[rp+1:])
		// fields: [0]=state [1]=ppid [2]=pgrp [3]=session
		if len(fields) < 4 {
			continue
		}
		sid, err := strconv.Atoi(fields[3])
		if err != nil || sid != shellPID {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
