// Package devenv resolves how a project's commands should run: directly, inside
// a nix develop shell, or inside its devcontainer. The language providers
// (nodejs/python/csharp/taskfile) share this logic verbatim.
package devenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Detect inspects projectPath for environment markers and returns the run
// environment: "devcontainer", "nix", or "" (run directly).
func Detect(projectPath string) string {
	for _, name := range []string{".devcontainer", "devcontainer.json"} {
		if _, err := os.Stat(filepath.Join(projectPath, name)); err == nil {
			return "devcontainer"
		}
	}
	for _, name := range []string{"devenv.nix", "flake.nix", ".devenv"} {
		if _, err := os.Stat(filepath.Join(projectPath, name)); err == nil {
			return "nix"
		}
	}
	return ""
}

// BuildCommand assembles the *exec.Cmd to run exe with args in workDir under the
// given run environment ("nix", "devcontainer", or anything else for direct).
func BuildCommand(ctx context.Context, workDir, runEnv, exe string, args []string) (*exec.Cmd, error) {
	switch runEnv {
	case "nix":
		// The flake ref is "." because cmd.Dir is already workDir; passing
		// workDir again would resolve it relative to workDir (workDir/workDir)
		// when workDir is a relative path.
		nixArgs := append([]string{"develop", ".", "--command", exe}, args...)
		cmd := exec.CommandContext(ctx, "nix", nixArgs...)
		cmd.Dir = workDir
		return cmd, nil
	case "devcontainer":
		containerID, wsFolder, err := Info(workDir)
		if err != nil {
			return nil, err
		}
		if containerID != "" {
			dockerArgs := append([]string{"exec", "-i", "-w", wsFolder, containerID, exe}, args...)
			cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
			cmd.Dir = workDir
			return cmd, nil
		}
		dcArgs := append([]string{"exec", "--workspace-folder", workDir, "--", exe}, args...)
		cmd := exec.CommandContext(ctx, "devcontainer", dcArgs...)
		cmd.Dir = workDir
		return cmd, nil
	default:
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Dir = workDir
		return cmd, nil
	}
}

// EnsureUp checks whether the devcontainer for workDir is running, starts a
// stopped container directly via docker, or runs `devcontainer up` when the
// devcontainer CLI is available. It returns the container ID, whether this call
// started it (so the caller can stop it when done), and any error.
func EnsureUp(ctx context.Context, workDir string) (containerID string, weStarted bool, err error) {
	absDir, absErr := filepath.Abs(workDir)
	if absErr != nil {
		absDir = workDir
	}
	filter := "label=devcontainer.local_folder=" + absDir
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", filter, "-q").Output()
	if err != nil {
		return "", false, fmt.Errorf("docker ps failed: %w", err)
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		return ids[0], false, nil
	}
	out, err = exec.CommandContext(ctx, "docker", "ps", "-a", "--filter", filter, "-q").Output()
	if err != nil {
		return "", false, fmt.Errorf("docker ps -a failed: %w", err)
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		if err := exec.CommandContext(ctx, "docker", "start", ids[0]).Run(); err != nil {
			return "", false, err
		}
		return ids[0], true, nil
	}
	if _, err := exec.LookPath("devcontainer"); err == nil {
		cmd := exec.CommandContext(ctx, "devcontainer", "up", "--workspace-folder", workDir)
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", false, err
		}
		out2, err := exec.CommandContext(ctx, "docker", "ps", "--filter", filter, "-q").Output()
		if err != nil {
			return "", false, err
		}
		ids := strings.Fields(string(out2))
		if len(ids) == 0 {
			return "", false, fmt.Errorf("devcontainer started but no running container matched %q", filter)
		}
		return ids[0], true, nil
	}
	return "", false, fmt.Errorf("no devcontainer found for this project; open it in VS Code first or install the devcontainer CLI")
}

// Info returns the container ID and the workspace folder path inside the
// container for the devcontainer associated with workDir. It returns
// ("", "", nil) when no running container is found, and an error when a docker
// command fails. An empty inspect result falls back to /workspaces/<dir>.
func Info(workDir string) (containerID, wsFolder string, err error) {
	absDir, absErr := filepath.Abs(workDir)
	if absErr != nil {
		absDir = workDir
	}
	idOut, err := exec.Command("docker", "ps",
		"--filter", "label=devcontainer.local_folder="+absDir,
		"-q",
	).Output()
	if err != nil {
		return "", "", fmt.Errorf("docker ps failed: %w", err)
	}
	ids := strings.Fields(string(idOut))
	if len(ids) == 0 {
		return "", "", nil
	}
	containerID = ids[0]
	wsfOut, err := exec.Command("docker", "inspect", containerID,
		"--format", `{{index .Config.Labels "devcontainer.workspace_folder"}}`).Output()
	if err != nil {
		return "", "", fmt.Errorf("docker inspect failed: %w", err)
	}
	wsFolder = strings.TrimSpace(string(wsfOut))
	if wsFolder == "" {
		wsFolder = "/workspaces/" + filepath.Base(workDir)
	}
	return containerID, wsFolder, nil
}
