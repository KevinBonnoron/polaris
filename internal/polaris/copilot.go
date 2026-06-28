package polaris

func copilotSpawnCommand(binary, task string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "copilot"
	}
	return bin, []string{task}, nil
}
