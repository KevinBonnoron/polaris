package nodejs

import (
	"fmt"
	"path/filepath"

	"github.com/KevinBonnoron/polaris/internal/providers/procrunner"
)

const (
	EventLine = "nodejs:run:line"
	EventExit = "nodejs:run:exit"
)

type Emitter = procrunner.Emitter

type Runner struct {
	pr *procrunner.Runner
}

func NewRunner(emit Emitter) *Runner {
	return &Runner{pr: procrunner.New(emit, EventLine, EventExit)}
}

// Start launches `<packageManager> run <script>` from the directory containing
// the manifest. The returned runID is the handle for streaming and cancel.
func (runner *Runner) Start(manifestPath, packageManager, script, runEnv string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("empty script name")
	}
	pm := defaultPM(packageManager)
	args := []string{"run", script}
	if pm == "deno" {
		args = []string{"task", script}
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, pm, [][]string{args})
}

// RunCommand runs an arbitrary package-manager command (e.g. "install",
// "add <pkg>") from the directory containing the manifest.
func (runner *Runner) RunCommand(manifestPath, packageManager, runEnv string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty args")
	}
	workDir, err := workDirOf(manifestPath)
	if err != nil {
		return "", err
	}
	return runner.pr.Run(workDir, runEnv, defaultPM(packageManager), [][]string{args})
}

func (runner *Runner) Stop(runID string) error { return runner.pr.Stop(runID) }

func defaultPM(pm string) string {
	if pm == "" {
		return "npm"
	}
	return pm
}

func workDirOf(manifestPath string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	return filepath.Dir(manifestPath), nil
}
