package polaris

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func runInteractive(input string) (lines []string, waitingCalls int) {
	reader := strings.NewReader(input)
	var sink bytes.Buffer
	streamInteractive(reader, &sink, func(line string) {
		lines = append(lines, line)
	}, func() {
		waitingCalls++
	})
	return lines, waitingCalls
}

func TestStreamInteractive_FlushesOnNewline(t *testing.T) {
	lines, waiting := runInteractive("hello\nworld\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(lines), lines)
	}
	if waiting != 0 {
		t.Fatalf("expected 0 waiting calls, got %d", waiting)
	}
	if !strings.Contains(lines[0], "hello") || !strings.Contains(lines[1], "world") {
		t.Fatalf("unexpected line contents: %#v", lines)
	}
}

func TestStreamInteractive_FlushesPartialOnEOF(t *testing.T) {
	lines, _ := runInteractive("trailing no newline")
	if len(lines) != 1 || !strings.Contains(lines[0], "trailing no newline") {
		t.Fatalf("expected trailing line emitted at EOF, got %#v", lines)
	}
}

func TestStreamInteractive_DetectsYNPrompt(t *testing.T) {
	// Real-world Copilot case: prompt without a trailing newline. The bracket
	// pattern must fire onWaiting and the prompt text must reach onLine.
	lines, waiting := runInteractive("Install GitHub Copilot CLI? ['y/N']")
	if waiting != 1 {
		t.Fatalf("expected 1 waiting call, got %d (lines=%#v)", waiting, lines)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "[y/N]") && !strings.Contains(lines[0], "'y/N'") {
		t.Fatalf("expected prompt text in line, got %#v", lines)
	}
}

func TestStreamInteractive_DetectsParenYNPrompt(t *testing.T) {
	lines, waiting := runInteractive("Continue (y/n)")
	if waiting != 1 {
		t.Fatalf("expected 1 waiting call, got %d (lines=%#v)", waiting, lines)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %#v", len(lines), lines)
	}
}

func TestStreamInteractive_NoFalsePositiveOnPlainQuestion(t *testing.T) {
	// A regular sentence ending with "?" should NOT trigger waiting — only the
	// curated prompt shapes do.
	_, waiting := runInteractive("did this work?\n")
	if waiting != 0 {
		t.Fatalf("plain question must not trigger waiting, got %d calls", waiting)
	}
}

func TestStreamInteractive_NoFalsePositiveOnBracketsWithoutYN(t *testing.T) {
	_, waiting := runInteractive("[info] starting\n")
	if waiting != 0 {
		t.Fatalf("bracketed log line must not trigger waiting, got %d calls", waiting)
	}
}

func TestStreamInteractive_PromptInMiddleOfStream(t *testing.T) {
	// First a normal log line, then a prompt with no newline.
	input := "starting up\nReady. Proceed? [y/N]"
	lines, waiting := runInteractive(input)
	if waiting != 1 {
		t.Fatalf("expected 1 waiting call, got %d (lines=%#v)", waiting, lines)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[1], "[y/N]") {
		t.Fatalf("prompt line missing y/N: %#v", lines[1])
	}
}

// Ensure streamInteractive doesn't hang when the reader returns 0 bytes then
// EOF (the bufio path handles this transparently, but we pin it down).
func TestStreamInteractive_HandlesEmptyReader(t *testing.T) {
	lines, waiting := runInteractive("")
	if len(lines) != 0 || waiting != 0 {
		t.Fatalf("empty input must produce no output, got lines=%#v waiting=%d", lines, waiting)
	}
}

// Sanity check: io.Reader interface stays decoupled from our usage.
var _ io.Reader = (*strings.Reader)(nil)
