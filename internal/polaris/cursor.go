package polaris

import "io"

// streamCursorJSON parses cursor's stream-json stdout. Cursor uses the same
// schema as claude-code but runs as a one-shot process (no interactive stdin),
// so AskUserQuestion and stdin-close callbacks are not needed.
func streamCursorJSON(reader io.Reader, sink io.Writer, onEvent func(StreamEvent), onTok onTokens) streamTurnStats {
	return streamClaudeJSON(reader, sink, onEvent, nil, onTok, nil)
}

func cursorSpawnCommand(binary, model, task string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "agent"
	}
	args := []string{"--print", "--output-format", "stream-json", "--stream-partial-output", "--trust"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, task)
	return bin, args, nil
}

func cursorResumeCommand(binary, model, message string) (string, []string, error) {
	bin := binary
	if bin == "" {
		bin = "agent"
	}
	args := []string{"--print", "--output-format", "stream-json", "--stream-partial-output", "--trust", "--continue"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, message)
	return bin, args, nil
}
