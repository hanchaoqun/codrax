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
