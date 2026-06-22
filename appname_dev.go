//go:build !production

package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
)

// Dev builds derive a per-directory application name so the GTK4 backend's
// single-instance machinery (app id "org.wails.<name>") gives each dev build
// its own window instead of forwarding activation to an already-running
// production Polaris or another worktree's dev build. The absolute path is
// hashed in so two checkouts sharing a leaf folder name still get distinct ids.
func appName() string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		return "Polaris (dev)"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(wd))
	return fmt.Sprintf("Polaris (dev %s-%08x)", filepath.Base(wd), h.Sum32())
}
