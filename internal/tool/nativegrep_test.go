package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedCorpus creates a tiny tree the native grep can walk. Mixes a
// source file, a skip-worthy binary (NUL byte), an excluded dir and
// a file that would match only via the basename glob.
func seedCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	write("a.go", "package a\n\nfunc NeedleFunc() {}\n// NEEDLE in comment\n")
	write("b.go", "package a\n\nfunc Other() {}\n")
	write("sub/c.go", "package sub\n\n// NeedleFunc usage\n")
	write("vendor/d.go", "// NeedleFunc but vendored\n")
	write("binary.bin", "\x00NeedleFunc\x00")
	return root
}

func TestNativeGrep_ContentAndFiles(t *testing.T) {
	root := seedCorpus(t)

	// Files-only mode: expect a.go and sub/c.go; vendor skipped; binary skipped.
	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "Needle",
		Root:        root,
		FilesOnly:   true,
		ExcludeDirs: []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("NativeGrep: %v", err)
	}
	got := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(got) != 2 {
		t.Fatalf("files-only: want 2 paths, got %d: %q", len(got), res.Output)
	}
	sawA := false
	sawC := false
	for _, line := range got {
		switch {
		case strings.HasSuffix(line, "a.go"):
			sawA = true
		case strings.HasSuffix(line, filepath.Join("sub", "c.go")):
			sawC = true
		case strings.Contains(line, "vendor"):
			t.Fatalf("vendor/ should be skipped; saw %q", line)
		case strings.HasSuffix(line, "binary.bin"):
			t.Fatalf("binary.bin should be skipped; saw %q", line)
		}
	}
	if !sawA || !sawC {
		t.Fatalf("missing matches: a=%v c=%v output=%q", sawA, sawC, res.Output)
	}

	// Content mode: same pattern, expect multiple lines with file:lineno prefix.
	res, err = NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "Needle",
		Root:        root,
		ExcludeDirs: []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("NativeGrep content: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(res.Output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("content line not path:lineno:text: %q", line)
		}
	}
	if res.Matches < 2 {
		t.Fatalf("want >=2 matches, got %d (output=%q)", res.Matches, res.Output)
	}
}

func TestNativeGrep_IgnoreCase(t *testing.T) {
	root := seedCorpus(t)
	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "needle",
		Root:        root,
		IgnoreCase:  true,
		FilesOnly:   true,
		ExcludeDirs: []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("NativeGrep ci: %v", err)
	}
	if res.Matches == 0 {
		t.Fatal("ignore_case should still match mixed-case source")
	}
}

func TestNativeGrep_Timeout(t *testing.T) {
	root := seedCorpus(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	_, err := NativeGrep(ctx, NativeGrepOpts{
		Pattern:   "Needle",
		Root:      root,
		FilesOnly: true,
	})
	if err == nil {
		t.Fatal("expected context cancel to surface")
	}
}

func TestNativeGrep_Include(t *testing.T) {
	root := seedCorpus(t)
	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "Needle",
		Root:        root,
		FilesOnly:   true,
		Include:     "a.go",
		ExcludeDirs: []string{"vendor"},
	})
	if err != nil {
		t.Fatalf("NativeGrep include: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "a.go") {
		t.Fatalf("include glob should keep only a.go, got %q", res.Output)
	}
}
