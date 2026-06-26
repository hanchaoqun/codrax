package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFallbackWarnLogFormatPinned pins the fallback WARN log format at
// the source level: AGENTS.md and CLAUDE.md quote this format
// verbatim ("repomap: <lang> <file> tier N→M: <reason>"), and the two
// drifted apart once already (the doc carried a stale variant for
// weeks with nothing pinning either side). Static source pin — precise
// and independent of logging plumbing.
func TestFallbackWarnLogFormatPinned(t *testing.T) {
	raw, err := os.ReadFile("parse_fallback.go")
	if err != nil {
		t.Fatal(err)
	}
	const want = `"repomap: %s %s tier %d→%d: %s"`
	if !strings.Contains(string(raw), want) {
		t.Fatalf("parse_fallback.go no longer carries the documented WARN format %s — update AGENTS.md and CLAUDE.md repomap red lines in the same change", want)
	}
	root := repoRootForRepomapTest(t)
	const docWant = "repomap: <lang> <file> tier N→M: <reason>"
	const stale = "repomap: <file> X→Y"
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md"} {
		docRaw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		doc := string(docRaw)
		if !strings.Contains(doc, docWant) {
			t.Fatalf("%s must quote the pinned repomap WARN format %q", rel, docWant)
		}
		if strings.Contains(doc, stale) {
			t.Fatalf("%s still contains stale repomap WARN format %q", rel, stale)
		}
	}
}

func repoRootForRepomapTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		next := filepath.Dir(wd)
		if next == wd {
			t.Fatal("could not locate repo root from test working directory")
		}
		wd = next
	}
}
