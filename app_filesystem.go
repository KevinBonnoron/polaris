package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type CodeEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "dist": true,
	"build": true, "__pycache__": true, ".cache": true, ".turbo": true,
	"coverage": true, ".nyc_output": true, "vendor": true, "target": true,
	".gradle": true, ".idea": true, ".vscode": true, "out": true,
	".venv": true, ".svn": true, ".hg": true, ".tox": true, ".yarn": true,
	".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true,
	".terraform": true, ".parcel-cache": true, ".gradle-cache": true,
}

func (app *App) PickFiles(defaultDir string) ([]string, error) {
	if appRef == nil {
		return nil, fmt.Errorf("application not ready")
	}
	return appRef.Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		SetDirectory(defaultDir).
		PromptForMultipleSelection()
}

func (app *App) SaveTempImage(base64Data, ext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("polaris_screenshot_%d%s", time.Now().UnixNano(), ext)
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (app *App) PasteClipboardImage() (string, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("polaris_paste_%d.png", time.Now().UnixNano()))

	switch runtime.GOOS {
	case "linux":
		toolFound := false
		if bin, err := exec.LookPath("wl-paste"); err == nil {
			toolFound = true
			cmd := exec.Command(bin, "--type", "image/png")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
		}
		if bin, err := exec.LookPath("xclip"); err == nil {
			toolFound = true
			cmd := exec.Command(bin, "-selection", "clipboard", "-t", "image/png", "-o")
			if data, err := cmd.Output(); err == nil && len(data) > 0 {
				return path, os.WriteFile(path, data, 0o644)
			}
		}
		if !toolFound {
			return "", fmt.Errorf("clipboard image paste requires wl-clipboard (Wayland) or xclip (X11) — install one of them to paste images")
		}
	case "darwin":
		script := fmt.Sprintf(`try
set img to the clipboard as «class PNGf»
set f to open for access POSIX file %q with write permission
write img to f
close access f
end try`, path)
		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Run(); err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no image in clipboard")
}

func (app *App) ReadFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// resolveInProject joins rel onto the project root and guarantees the result
// stays inside it. filepath.Join already cleans "..", but a plain HasPrefix
// check would also accept a sibling sharing the prefix (e.g. base "/a/proj"
// vs "/a/proj-secrets"), so the boundary is enforced with a trailing separator.
func resolveInProject(projectPath, rel string) (base, full string, err error) {
	base, err = filepath.Abs(projectPath)
	if err != nil {
		return "", "", err
	}
	full = filepath.Join(base, filepath.FromSlash(rel))
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("path outside project")
	}
	// Re-check after resolving symlinks: a symlinked entry inside the project
	// could otherwise point os.ReadDir/ReadFile/WriteFile outside the boundary.
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", "", err
	}
	realFull, err := evalSymlinksAllowingMissing(full)
	if err != nil {
		return "", "", err
	}
	if realFull != realBase && !strings.HasPrefix(realFull, realBase+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("path outside project")
	}
	return base, full, nil
}

// evalSymlinksAllowingMissing resolves symlinks in p, tolerating a path whose
// final components do not exist yet (e.g. a file WriteProjectFile is about to
// create): it resolves the deepest existing ancestor and re-appends the rest.
func evalSymlinksAllowingMissing(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	var missing []string
	cur := p
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve %q", p)
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		cur = parent
	}
}

func (app *App) ListProjectDir(projectPath, relDir string) ([]CodeEntry, error) {
	_, dir, err := resolveInProject(projectPath, relDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []CodeEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && skipDirs[name] {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(relDir, name))
		var size int64
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
		}
		out = append(out, CodeEntry{Name: name, Path: rel, IsDir: e.IsDir(), Size: size})
	}
	return out, nil
}

func (app *App) ReadProjectFile(projectPath, relPath string) (string, error) {
	_, full, err := resolveInProject(projectPath, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	const maxSize = 512 * 1024
	if info.Size() > maxSize {
		return "", fmt.Errorf("file too large")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (app *App) WriteProjectFile(projectPath, relPath, content string) error {
	_, full, err := resolveInProject(projectPath, relPath)
	if err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0644)
}

func (app *App) OpenDirectory(title string) (string, error) {
	if title == "" {
		title = "Choose project folder"
	}
	if appRef == nil {
		return "", fmt.Errorf("application not ready")
	}
	return appRef.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle(title).
		PromptForSingleSelection()
}
