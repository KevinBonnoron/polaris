package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Terminal struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Binary    string `json:"binary,omitempty"`
	Path      string `json:"path,omitempty"`
	Installed bool   `json:"installed"`
	Default   bool   `json:"default,omitempty"`
}

type candidate struct {
	id       string
	name     string
	binaries []string
	platform string
}

var candidates = []candidate{
	{id: "ghostty", name: "Ghostty", binaries: []string{"ghostty"}},
	{id: "kitty", name: "Kitty", binaries: []string{"kitty"}},
	{id: "alacritty", name: "Alacritty", binaries: []string{"alacritty"}},
	{id: "wezterm", name: "WezTerm", binaries: []string{"wezterm"}},
	{id: "foot", name: "Foot", binaries: []string{"foot"}, platform: "linux"},
	{id: "gnome-terminal", name: "GNOME Terminal", binaries: []string{"gnome-terminal"}, platform: "linux"},
	{id: "konsole", name: "Konsole", binaries: []string{"konsole"}, platform: "linux"},
	{id: "xfce4-terminal", name: "Xfce Terminal", binaries: []string{"xfce4-terminal"}, platform: "linux"},
	{id: "tilix", name: "Tilix", binaries: []string{"tilix"}, platform: "linux"},
	{id: "terminator", name: "Terminator", binaries: []string{"terminator"}, platform: "linux"},
	{id: "xterm", name: "xterm", binaries: []string{"xterm"}, platform: "linux"},
	{id: "iterm2", name: "iTerm2", binaries: []string{"iterm2"}, platform: "darwin"},
	{id: "terminal-app", name: "Terminal", binaries: []string{}, platform: "darwin"},
	{id: "wt", name: "Windows Terminal", binaries: []string{"wt"}, platform: "windows"},
	{id: "powershell", name: "PowerShell", binaries: []string{"pwsh", "powershell"}, platform: "windows"},
	{id: "cmd", name: "Command Prompt", binaries: []string{"cmd"}, platform: "windows"},
}

func Detect() []Terminal {
	out := make([]Terminal, 0, len(candidates))
	for _, c := range candidates {
		if c.platform != "" && c.platform != runtime.GOOS {
			continue
		}
		t := Terminal{ID: c.id, Name: c.name}
		if c.id == "terminal-app" {
			if _, err := os.Stat("/System/Applications/Utilities/Terminal.app"); err == nil {
				t.Installed = true
				t.Path = "/System/Applications/Utilities/Terminal.app"
			}
		} else {
			for _, bin := range c.binaries {
				if p, err := exec.LookPath(bin); err == nil {
					t.Installed = true
					t.Binary = bin
					t.Path = p
					break
				}
			}
		}
		out = append(out, t)
	}

	defaultID := detectDefault(out)
	if defaultID != "" {
		for i := range out {
			if out[i].ID == defaultID {
				out[i].Default = true
				break
			}
		}
	} else {
		for i := range out {
			if out[i].Installed {
				out[i].Default = true
				break
			}
		}
	}

	return out
}

func detectDefault(terms []Terminal) string {
	switch runtime.GOOS {
	case "linux":
		return detectDefaultLinux(terms)
	case "darwin":
		return "terminal-app"
	case "windows":
		for _, t := range terms {
			if t.ID == "wt" && t.Installed {
				return "wt"
			}
		}
		return "cmd"
	}
	return ""
}

func detectDefaultLinux(terms []Terminal) string {
	installed := map[string]string{}
	for _, t := range terms {
		if t.Installed {
			installed[t.Binary] = t.ID
			installed[t.ID] = t.ID
		}
	}

	if env := strings.TrimSpace(os.Getenv("TERMINAL")); env != "" {
		base := baseName(env)
		if id, ok := installed[base]; ok {
			return id
		}
	}

	if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/terminal").Output(); err == nil {
		desktop := strings.TrimSpace(string(out))
		if id := matchDesktop(desktop, installed); id != "" {
			return id
		}
	}

	if out, err := exec.Command("gsettings", "get", "org.gnome.desktop.default-applications.terminal", "exec").Output(); err == nil {
		execLine := strings.Trim(strings.TrimSpace(string(out)), "'\"")
		if execLine != "" {
			if id, ok := installed[baseName(execLine)]; ok {
				return id
			}
		}
	}

	return ""
}

func matchDesktop(desktop string, installed map[string]string) string {
	if desktop == "" {
		return ""
	}
	name := strings.TrimSuffix(desktop, ".desktop")
	name = strings.ToLower(name)
	knownPrefixes := map[string]string{
		"com.mitchellh.ghostty":  "ghostty",
		"org.gnome.terminal":     "gnome-terminal",
		"org.kde.konsole":        "konsole",
		"kitty":                  "kitty",
		"alacritty":              "alacritty",
		"org.wezfurlong.wezterm": "wezterm",
		"foot":                   "foot",
		"xfce4-terminal":         "xfce4-terminal",
		"tilix":                  "tilix",
		"terminator":             "terminator",
		"xterm":                  "xterm",
	}
	for prefix, id := range knownPrefixes {
		if strings.HasPrefix(name, prefix) {
			if _, ok := installed[id]; ok {
				return id
			}
		}
	}
	return ""
}

func baseName(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if idx := strings.IndexAny(cmd, " \t"); idx >= 0 {
		cmd = cmd[:idx]
	}
	if idx := strings.LastIndexAny(cmd, "/\\"); idx >= 0 {
		cmd = cmd[idx+1:]
	}
	return cmd
}

func OpenInDir(id, dir string) error {
	if dir == "" {
		dir = os.Getenv("HOME")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("directory not accessible: %s", dir)
	}

	var c candidate
	for _, cand := range candidates {
		if cand.id == id {
			c = cand
			break
		}
	}
	if c.id == "" {
		return errors.New("unknown terminal")
	}

	cmd, err := buildLaunchCommand(c, dir)
	if err != nil {
		return err
	}
	cmd.Dir = dir
	return cmd.Start()
}

func buildLaunchCommand(c candidate, dir string) (*exec.Cmd, error) {
	switch c.id {
	case "terminal-app":
		return exec.Command("open", "-a", "Terminal", dir), nil
	case "iterm2":
		return exec.Command("open", "-a", "iTerm", dir), nil
	case "ghostty":
		return exec.Command("ghostty", "--working-directory="+dir), nil
	case "kitty":
		return exec.Command("kitty", "--directory", dir), nil
	case "alacritty":
		return exec.Command("alacritty", "--working-directory", dir), nil
	case "wezterm":
		return exec.Command("wezterm", "start", "--cwd", dir), nil
	case "gnome-terminal":
		return exec.Command("gnome-terminal", "--working-directory="+dir), nil
	case "konsole":
		return exec.Command("konsole", "--workdir", dir), nil
	case "xfce4-terminal":
		return exec.Command("xfce4-terminal", "--working-directory="+dir), nil
	case "tilix":
		return exec.Command("tilix", "--working-directory="+dir), nil
	case "terminator":
		return exec.Command("terminator", "--working-directory="+dir), nil
	case "foot":
		return exec.Command("foot"), nil
	case "xterm":
		return exec.Command("xterm"), nil
	case "wt":
		return exec.Command("wt", "-d", dir), nil
	case "powershell":
		bin := "pwsh"
		if _, err := exec.LookPath(bin); err != nil {
			bin = "powershell"
		}
		return exec.Command("cmd", "/C", "start", "", bin, "-NoExit", "-Command", "Set-Location -LiteralPath '"+strings.ReplaceAll(dir, "'", "''")+"'"), nil
	case "cmd":
		return exec.Command("cmd", "/C", "start", "", "cmd", "/K", "cd /d "+dir), nil
	}
	return nil, fmt.Errorf("unsupported terminal: %s", c.id)
}
