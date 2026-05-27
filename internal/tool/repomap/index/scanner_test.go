package index

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsExcludedPath_WindowsReservedDeviceName(t *testing.T) {
	wantReserved := runtime.GOOS == "windows"
	for _, rel := range []string{
		"nul",
		"nul.txt",
		filepath.Join("sub", "con"),
		filepath.Join("nested", "LPT1.log"),
	} {
		if got := isExcludedPath(rel); got != wantReserved {
			t.Fatalf("isExcludedPath(%q) = %v, want %v on %s", rel, got, wantReserved, runtime.GOOS)
		}
	}
	for _, rel := range []string{
		"null.go",
		filepath.Join("sub", "console.txt"),
		filepath.Join("pkg", "component.go"),
	} {
		if isExcludedPath(rel) {
			t.Fatalf("isExcludedPath(%q) should be false", rel)
		}
	}
}

func TestIsExcludedPath_TransientToolCaches(t *testing.T) {
	for _, rel := range []string{
		filepath.Join(".gotmp-tool", "go-build", "x.go"),
		filepath.Join("sub", ".gocache-worktree", "obj", "y.go"),
		filepath.Join(".vs", "state", "solution.suo"),
	} {
		if !isExcludedPath(rel) {
			t.Fatalf("isExcludedPath(%q)=false, want true", rel)
		}
	}
	for _, rel := range []string{
		filepath.Join("pkg", "gotmp_helper", "main.go"),
		filepath.Join("internal", "memory", "cache.go"),
	} {
		if isExcludedPath(rel) {
			t.Fatalf("isExcludedPath(%q) should be false", rel)
		}
	}
}

func TestScanFilesWithProgressReportsWalkFallback(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var lastFiles, lastParseable int
	var sawCurrent bool
	entries, err := ScanFilesWithProgress(repo, func(files, parseable int, current string) {
		lastFiles = files
		lastParseable = parseable
		if strings.TrimSpace(current) != "" {
			sawCurrent = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2: %+v", len(entries), entries)
	}
	if lastFiles != 2 || lastParseable != 1 || !sawCurrent {
		t.Fatalf("progress=(files=%d parseable=%d current=%v), want files=2 parseable=1 current=true", lastFiles, lastParseable, sawCurrent)
	}
}

func TestEntriesFromGitLinesReportsAcceptedFileProgress(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var events []string
	entries := entriesFromGitLines(repo, []string{"src/main.go", "missing.go", "notes.txt"}, func(files, parseable int, current string) {
		events = append(events, current)
	})
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2: %+v", len(entries), entries)
	}
	if len(events) != 2 || events[0] != "src/main.go" || events[1] != "notes.txt" {
		t.Fatalf("progress should report accepted files only, got %#v", events)
	}
}
