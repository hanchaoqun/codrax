package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// hvigor + alternative layouts — verify the walk fallback finds JUnit
// XML in non-Maven/Gradle locations.

func TestLocateJUnitReportDir_HvigorEntryLayout(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, "entry", "build", "default", "intermediates", "test", "test-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST-foo.xml"), []byte("<testsuite/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := locateJUnitReportDir(repo)
	if got != dir {
		t.Errorf("locateJUnitReportDir = %q; want %q", got, dir)
	}
}

func TestLocateJUnitReportDir_FallbackWalk(t *testing.T) {
	// Drop a non-conventional path that the walk should still find.
	repo := t.TempDir()
	dir := filepath.Join(repo, "module-a", "build", "reports", "test-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST-bar.xml"), []byte("<testsuite/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := locateJUnitReportDir(repo)
	if got != dir {
		t.Errorf("locateJUnitReportDir = %q; want %q", got, dir)
	}
}

func TestLocateJUnitReportDir_NestedUnderTestResults(t *testing.T) {
	// Gradle-style: build/test-results/test/<suite>/TEST-*.xml.
	repo := t.TempDir()
	resultsRoot := filepath.Join(repo, "build", "test-results")
	leafDir := filepath.Join(resultsRoot, "test")
	if err := os.MkdirAll(leafDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leafDir, "TEST-x.xml"), []byte("<testsuite/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := locateJUnitReportDir(repo)
	// The known-candidate `build/test-results/test` should match
	// directly (it has top-level .xml).
	if got != leafDir {
		t.Errorf("locateJUnitReportDir = %q; want %q", got, leafDir)
	}
}

func TestLocateJUnitReportDir_SkipsLargeIrrelevantDirs(t *testing.T) {
	// Plant XML inside node_modules — walk should NOT find it.
	repo := t.TempDir()
	dir := filepath.Join(repo, "node_modules", "junk", "test-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST.xml"), []byte("<testsuite/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := locateJUnitReportDir(repo)
	if got != "" {
		t.Errorf("walk should skip node_modules; got %q", got)
	}
}

func TestLocateJUnitReportDir_NoMatch(t *testing.T) {
	repo := t.TempDir()
	if got := locateJUnitReportDir(repo); got != "" {
		t.Errorf("empty repo should yield empty; got %q", got)
	}
}
