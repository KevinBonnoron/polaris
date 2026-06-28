package polaris

func codexSpawnCommand(binary, model, task string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "codex"
	}
	args := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if task != "" {
		args = append(args, "--")
	}
	args = append(args, task)
	return bin, args, nil
}

func codexResumeCommand(binary, sessionID, model, message string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "codex"
	}
	args := []string{"exec", "resume", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, sessionID)
	if message != "" {
		args = append(args, "--", message)
	}
	return bin, args, nil
}
