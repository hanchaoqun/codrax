package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchDirFilter_RootOnlyKeepsNestedProjectDirs(t *testing.T) {
	repo := t.TempDir()
	filter := NewSearchDirFilter(repo, repo)

	if !filter.ExcludesRepoRelativePath("logs/runtime.log") {
		t.Fatal("root-level logs/ should be excluded")
	}
	if filter.ExcludesRepoRelativePath(filepath.ToSlash(filepath.Join("internal", "logs", "logger.go"))) {
		t.Fatal("nested internal/logs/ should remain searchable")
	}
	if !filter.ExcludesRepoRelativePath("memory/session.md") {
		t.Fatal("root-level memory/ should be excluded")
	}
	if filter.ExcludesRepoRelativePath(filepath.ToSlash(filepath.Join("pkg", "memory", "cache.go"))) {
		t.Fatal("nested pkg/memory/ should remain searchable")
	}
}

func TestSearchDirFilter_ExplicitTargetBypassesStructuralNoise(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "dist", "bundle"), 0o755); err != nil {
		t.Fatalf("seed dist: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "eval", "cases"), 0o755); err != nil {
		t.Fatalf("seed eval: %v", err)
	}

	distFilter := NewSearchDirFilter(repo, filepath.Join(repo, "dist"))
	if distFilter.ExcludesRepoRelativePath(filepath.ToSlash(filepath.Join("dist", "bundle", "app.js"))) {
		t.Fatal("explicit dist/ target should not be excluded")
	}
	for _, pattern := range distFilter.AnyLevelPatterns() {
		if pattern == "dist" {
			t.Fatal("explicit dist/ target should exempt the dist any-level pattern")
		}
	}

	evalFilter := NewSearchDirFilter(repo, filepath.Join(repo, "eval"))
	if evalFilter.ExcludesRepoRelativePath(filepath.ToSlash(filepath.Join("eval", "cases", "u11a.case"))) {
		t.Fatal("explicit eval/ target should not be excluded")
	}
	if evalFilter.ExcludesRepoRelativePath(filepath.ToSlash(filepath.Join("internal", "eval", "helper.go"))) {
		t.Fatal("explicit root eval/ target should not turn nested internal/eval into noise")
	}
}
