package godot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ensureDevcontainerUp checks whether the devcontainer for workDir is running,
// starts a stopped container directly via docker, or runs `devcontainer up`
// when the devcontainer CLI is available. Returns the container ID, whether
// this call started it (so the caller can stop it when done), and any error.
func ensureDevcontainerUp(workDir string) (containerID string, weStarted bool, err error) {
	absDir, absErr := filepath.Abs(workDir)
	if absErr != nil {
		absDir = workDir
	}
	filter := "label=devcontainer.local_folder=" + absDir
	out, err := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
	if err != nil {
		return "", false, fmt.Errorf("docker ps failed: %w", err)
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		return ids[0], false, nil
	}
	out, err = exec.Command("docker", "ps", "-a", "--filter", filter, "-q").Output()
	if err != nil {
		return "", false, fmt.Errorf("docker ps -a failed: %w", err)
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		if err := exec.Command("docker", "start", ids[0]).Run(); err != nil {
			return "", false, err
		}
		return ids[0], true, nil
	}
	if _, err := exec.LookPath("devcontainer"); err == nil {
		cmd := exec.Command("devcontainer", "up", "--workspace-folder", workDir)
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", false, err
		}
		out2, err := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
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

// devcontainerInfo returns the container ID and workspace folder path inside the
// container for the devcontainer associated with workDir. It returns ("", "",
// nil) when no running container is found, and an error when a docker command
// fails. An empty inspect result falls back to /workspaces/<dir>.
func devcontainerInfo(workDir string) (containerID, wsFolder string, err error) {
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
