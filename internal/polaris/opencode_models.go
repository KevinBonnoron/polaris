package polaris

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

// ansiEscape matches ANSI/VT100 escape sequences produced by TUI-based CLIs.
var ansiEscape = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-9;?]*[ -/]*[@-~])`)

// ParseModelsOutput parses CLI output to extract model IDs.
// It strips ANSI escape sequences and filters out non-model lines.
func ParseModelsOutput(raw string) []string {
	clean := ansiEscape.ReplaceAllString(raw, "")
	var models []string
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Discard lines with spaces or non-printable characters — those are
		// UI chrome, not model IDs.
		valid := true
		for _, r := range line {
			if r == ' ' || !unicode.IsPrint(r) {
				valid = false
				break
			}
		}
		if valid {
			models = append(models, line)
		}
	}
	return models
}

// ListOpencodeModels returns the list of models from the opencode CLI.
// binary is the already-resolved path from detection (may carry a "wsl:" prefix
// on Windows); pass empty to fall back to exec.LookPath.
func ListOpencodeModels(binary string) ([]string, error) {
	if binary == "" {
		p, err := exec.LookPath("opencode")
		if err != nil {
			return nil, err
		}
		binary = p
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if linuxPath, ok := strings.CutPrefix(binary, "wsl:"); ok {
		cmd = exec.CommandContext(ctx, "wsl.exe", "--", resolveWslShell(), "-lc", "'"+strings.ReplaceAll(linuxPath, "'", "'\\''")+"' models")
		// cmd.Env = wslFilterEnv(os.Environ()) // TODO: fix env filtering — snap/opencode doesn't work without full env
	} else {
		cmd = exec.CommandContext(ctx, binary, "models")
	}
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	return ParseModelsOutput(string(out)), nil
}
