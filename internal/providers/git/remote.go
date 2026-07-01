package git

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KevinBonnoron/polaris/internal/sysexec"
)

type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
	ProviderUnknown   Provider = "unknown"
)

type Remote struct {
	Provider Provider `json:"provider"`
	Host     string   `json:"host"`
	BaseURL  string   `json:"baseUrl"`
	Owner    string   `json:"owner"`
	Repo     string   `json:"repo"`
	URL      string   `json:"url"`
}

// DetectRemote walks up from projectPath to find a `.git` directory, reads
// its config, and returns the parsed `origin` remote. The fallback when the
// project doesn't have one yet is a nil Remote with no error.
func DetectRemote(projectPath string) (*Remote, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("empty project path")
	}

	gitDir, err := findGitDir(projectPath)
	if err != nil {
		return nil, err
	}

	if gitDir == "" {
		return nil, nil
	}

	originURL, err := readOriginURL(filepath.Join(gitDir, "config"))
	if err != nil {
		return nil, err
	}

	if originURL == "" {
		return nil, nil
	}

	return parseRemote(originURL), nil
}

func findGitDir(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		// Worktrees and submodules use a `.git` file pointing at the real dir.
		if err == nil && !info.IsDir() {
			real, rerr := resolveGitFile(candidate)
			if rerr != nil {
				return "", rerr
			}
			if real != "" {
				return real, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func resolveGitFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return target, nil
}

func readOriginURL(configPath string) (string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}

		if !inOrigin {
			continue
		}

		if eq := strings.IndexByte(line, '='); eq > 0 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if strings.EqualFold(key, "url") {
				return val, nil
			}
		}
	}

	return "", scanner.Err()
}

// parseRemote accepts the common forms produced by git:
//
//	https://host/owner/repo[.git]
//	http://host/owner/repo[.git]
//	ssh://git@host[:port]/owner/repo[.git]
//	git@host:owner/repo[.git]
//
// It populates Provider via a simple host suffix match.
func parseRemote(raw string) *Remote {
	r := &Remote{URL: raw, Provider: ProviderUnknown}
	host, path := splitRemote(raw)
	r.Host = host
	owner, repo := splitPath(path)
	r.Owner = owner
	r.Repo = repo
	r.Provider = providerFor(host)
	if host != "" {
		scheme := "https"
		if strings.HasPrefix(strings.ToLower(raw), "http://") {
			scheme = "http"
		}
		r.BaseURL = scheme + "://" + host
	}
	return r
}

func splitRemote(raw string) (host, path string) {
	raw = strings.TrimSpace(raw)
	// scp-like: git@host:owner/repo
	if !strings.Contains(raw, "://") && strings.Contains(raw, ":") && !strings.HasPrefix(raw, "/") {
		at := strings.IndexByte(raw, '@')
		colon := strings.IndexByte(raw, ':')
		if colon > at {
			return raw[at+1 : colon], raw[colon+1:]
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	return u.Hostname(), strings.TrimPrefix(u.Path, "/")
}

func splitPath(path string) (owner, repo string) {
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return "", ""
	}
	idx := strings.LastIndexByte(path, '/')
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

// ProviderToken describes a token discovered from a third-party CLI.
type ProviderToken struct {
	Provider Provider `json:"provider"`
	Token    string   `json:"token"`
	Source   string   `json:"source"`
}

// DetectProviderToken returns a credential the user has already configured for
// a given provider. Today it shells out to `gh` and `glab`; both CLIs print
// the active token on stdout when asked. Returns (nil, nil) if no token is
// found — callers treat that as "user has to type it themselves".
func DetectProviderToken(provider string) (*ProviderToken, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(provider))) {
	case ProviderGitHub:
		if tok, err := runForToken("gh", "auth", "token"); err == nil && tok != "" {
			return &ProviderToken{Provider: ProviderGitHub, Token: tok, Source: "gh"}, nil
		}
		if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
			return &ProviderToken{Provider: ProviderGitHub, Token: tok, Source: "env:GITHUB_TOKEN"}, nil
		}
	case ProviderGitLab:
		if tok, err := runForToken("glab", "auth", "status", "--show-token"); err == nil {
			if t := extractGlabToken(tok); t != "" {
				return &ProviderToken{Provider: ProviderGitLab, Token: t, Source: "glab"}, nil
			}
		}
		if tok := strings.TrimSpace(os.Getenv("GITLAB_TOKEN")); tok != "" {
			return &ProviderToken{Provider: ProviderGitLab, Token: tok, Source: "env:GITLAB_TOKEN"}, nil
		}
	}
	return nil, nil
}

func runForToken(name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// glab prints lines like `✓ Token: glpat-xxxx`. Pick the first one we see.
func extractGlabToken(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(strings.ToLower(line), "token:")
		if idx < 0 {
			continue
		}
		tok := strings.TrimSpace(line[idx+len("token:"):])
		tok = strings.Trim(tok, "\"'")
		if tok != "" && tok != "<no token>" {
			return tok
		}
	}
	return ""
}
