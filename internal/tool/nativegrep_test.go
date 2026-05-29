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

func TestNativeGrep_FixedStringTreatsRegexPunctuationLiterally(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "trace.log")
	body := "[GT]codraxNode1>prio=20\nGTcodraxNode1 prio=20\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "[GT]codraxNode1>prio=20",
		Root:        path,
		FixedString: true,
		DisplayRoot: root,
	})
	if err != nil {
		t.Fatalf("NativeGrep fixed string: %v", err)
	}
	if res.Matches != 1 {
		t.Fatalf("fixed string should match exactly once, got %d output=%q", res.Matches, res.Output)
	}
	if !strings.Contains(res.Output, "trace.log:1:[GT]codraxNode1>prio=20") {
		t.Fatalf("fixed string output should be repo-relative and literal:\n%s", res.Output)
	}
}

func TestNativeGrep_LineWindow(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.log")
	body := "needle before\nnoise\nneedle in window\nneedle after\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}

	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:     "needle",
		Root:        path,
		LineStart:   2,
		LineEnd:     3,
		DisplayRoot: root,
	})
	if err != nil {
		t.Fatalf("NativeGrep line window: %v", err)
	}
	if res.Matches != 1 || !strings.Contains(res.Output, "events.log:3:needle in window") {
		t.Fatalf("line window should keep only line 3, matches=%d output=%q", res.Matches, res.Output)
	}
	if strings.Contains(res.Output, "before") || strings.Contains(res.Output, "after") {
		t.Fatalf("line window leaked outside range:\n%s", res.Output)
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

func TestNativeGrep_ExplicitLargeTraceFileScansPastDefaultCap(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "record_trace_20260526170756@929-448250782.systrace")
	var b strings.Builder
	b.WriteString(strings.Repeat("x", nativeGrepMaxFileBytes+1024))
	b.WriteString("\n")
	b.WriteString("com.tencent.mm-48517 (48517) [010] .... 935.305162: print: B|14973|Choreographer#doFrame 59185\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write large systrace: %v", err)
	}

	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:    "Choreographer.*doFrame",
		Root:       path,
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatalf("NativeGrep explicit large file: %v", err)
	}
	if res.Matches != 1 {
		t.Fatalf("explicit large file should be scanned fully; matches=%d skipped=%d output=%q", res.Matches, res.SkippedLargeFiles, res.Output)
	}
	if res.SkippedLargeFiles != 0 {
		t.Fatalf("explicit large file must not count as skipped, got %d", res.SkippedLargeFiles)
	}
	if !strings.Contains(res.Output, "Choreographer#doFrame 59185") {
		t.Fatalf("missing systrace event in output:\n%s", res.Output)
	}
}

func TestNativeGrep_DirectorySearchStillSkipsLargeFilesByDefault(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.log")
	body := strings.Repeat("x", nativeGrepMaxFileBytes+1024) + "\nNeedleBeyondDefaultCap\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write large log: %v", err)
	}

	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern: "NeedleBeyondDefaultCap",
		Root:    root,
	})
	if err != nil {
		t.Fatalf("NativeGrep directory large file: %v", err)
	}
	if res.Matches != 0 {
		t.Fatalf("directory search should keep default large-file safety cap; output=%q", res.Output)
	}
	if res.SkippedLargeFiles != 1 {
		t.Fatalf("SkippedLargeFiles = %d, want 1", res.SkippedLargeFiles)
	}
	if len(res.SkippedLargeFilePaths) != 1 || !strings.HasSuffix(res.SkippedLargeFilePaths[0], "large.log") {
		t.Fatalf("SkippedLargeFilePaths = %#v, want large.log", res.SkippedLargeFilePaths)
	}
}
