package index

import (
	"path/filepath"
	"runtime"
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
