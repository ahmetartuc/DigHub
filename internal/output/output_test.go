package output

import (
	"strings"
	"testing"
)

func TestSnippetLines(t *testing.T) {
	long := strings.Repeat("a", maxSnippetWidth+20)
	got := snippetLines([]string{"\n  KEY=1  \n\n", long + "\nthird\nfourth"})

	if len(got) != maxSnippetLines {
		t.Fatalf("got %d lines, want %d: %q", len(got), maxSnippetLines, got)
	}
	if got[0] != "KEY=1" {
		t.Errorf("first line = %q, want trimmed KEY=1", got[0])
	}
	if !strings.HasSuffix(got[1], "…") || len([]rune(got[1])) != maxSnippetWidth+1 {
		t.Errorf("long line not cut to width: %d runes", len([]rune(got[1])))
	}
	if got[2] != "third" {
		t.Errorf("third line = %q", got[2])
	}
}
