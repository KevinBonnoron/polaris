package csharp

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
	out, _ := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
	if id := strings.TrimSpace(string(out)); id != "" {
		return id, false, nil
	}
	out, _ = exec.Command("docker", "ps", "-a", "--filter", filter, "-q").Output()
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
		out2, _ := exec.Command("docker", "ps", "--filter", filter, "-q").Output()
		return strings.TrimSpace(string(out2)), true, nil
	}
	return "", false, fmt.Errorf("no devcontainer found for this project; open it in VS Code first or install the devcontainer CLI")
}

// devcontainerInfo returns the container ID and workspace folder path inside the
// container for the devcontainer associated with workDir. Returns ("", "") when
// no running container is found.
func devcontainerInfo(workDir string) (containerID, wsFolder string) {
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		absDir = workDir
	}
	idOut, _ := exec.Command("docker", "ps",
		"--filter", "label=devcontainer.local_folder="+absDir,
		"-q",
	).Output()
	ids := strings.Fields(string(idOut))
	if len(ids) == 0 {
		return "", ""
	}
	containerID = ids[0]
	wsfOut, _ := exec.Command("docker", "inspect", containerID,
		"--format", `{{index .Config.Labels "devcontainer.workspace_folder"}}`).Output()
	wsFolder = strings.TrimSpace(string(wsfOut))
	if wsFolder == "" {
		wsFolder = "/workspaces/" + filepath.Base(workDir)
	}
	return containerID, wsFolder
}
