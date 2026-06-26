package polaris

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
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
func ListOpencodeModels() ([]string, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "models").Output()
	if err != nil {
		return nil, err
	}

	return ParseModelsOutput(string(out)), nil
}
