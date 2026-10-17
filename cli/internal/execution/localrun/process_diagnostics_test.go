package localrun

import (
	"strings"
	"testing"
)

func TestProcessReportsOversizedFrameInsteadOfSilentEOF(t *testing.T) {
	p := &execProcess{lines: make(chan []byte, 1)}
	p.read(strings.NewReader(strings.Repeat("x", maxLineBytes+1)))
	if p.ReadError() == nil {
		t.Fatal("oversized frame looked like a clean EOF")
	}
	if _, open := <-p.Lines(); open {
		t.Fatal("reader did not terminate")
	}
}
