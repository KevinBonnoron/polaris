package polaris

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestLockedWriterNoInterleave drives the same writer from many goroutines the
// way the stdout parser and stderr drain do, and asserts every line survives
// intact. Run with -race to also catch the unsynchronised write.
func TestLockedWriterNoInterleave(t *testing.T) {
	var buf bytes.Buffer
	w := newLockedWriter(&buf)

	const writers = 8
	const perWriter = 200
	var wg sync.WaitGroup
	wg.Add(writers)
	for g := 0; g < writers; g++ {
		go func(g int) {
			defer wg.Done()
			line := strings.Repeat(fmt.Sprintf("%d", g), 512)
			for i := 0; i < perWriter; i++ {
				fmt.Fprintln(w, line)
			}
		}(g)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != writers*perWriter {
		t.Fatalf("got %d lines, want %d", len(lines), writers*perWriter)
	}
	for _, l := range lines {
		if len(l) != 512 {
			t.Fatalf("interleaved line of length %d: %q", len(l), l)
		}
		first := l[0]
		for i := 0; i < len(l); i++ {
			if l[i] != first {
				t.Fatalf("interleaved line mixing writers: %q", l)
			}
		}
	}
}
