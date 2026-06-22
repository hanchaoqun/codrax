package tool

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSearchDirFilter_IncludeAuxiliarySourceKeepsRuntimeNoiseExcluded(t *testing.T) {
	repo := t.TempDir()
	defaultFilter := NewSearchDirFilter(repo, repo)
	if !defaultFilter.ExcludesRepoRelativePath("internal/thirdparty/parser/corpus/source.cj") {
		t.Fatal("default broad search should keep thirdparty auxiliary source filtered")
	}
	if !defaultFilter.ExcludesRepoRelativePath("eval/fixtures/testdata/source.cj") {
		t.Fatal("default broad search should keep root eval filtered")
	}

	sourceFilter := NewSearchDirFilterWithOptions(repo, repo, SearchDirFilterOptions{IncludeAuxiliarySource: true})
	for _, rel := range []string{
		"internal/thirdparty/parser/corpus/source.cj",
		"vendor/runtime/source.cpp",
		"eval/fixtures/testdata/source.cj",
		"tests/data/source.ts",
	} {
		if sourceFilter.ExcludesRepoRelativePath(rel) {
			t.Fatalf("typed auxiliary source search should include %s", rel)
		}
	}
	for _, rel := range []string{
		".codrax/output/source.cj",
		"node_modules/pkg/source.ts",
		"eval/results/run-1.repo/source.cj",
	} {
		if !sourceFilter.ExcludesRepoRelativePath(rel) {
			t.Fatalf("typed auxiliary source search must still exclude runtime/dependency noise %s", rel)
		}
	}

	globs := strings.Join(sourceFilter.RipgrepGlobs(), "\n")
	if strings.Contains(globs, "!thirdparty/") || strings.Contains(globs, "!vendor/") {
		t.Fatalf("ripgrep globs should not exclude reopened auxiliary source dirs:\n%s", globs)
	}
	if strings.Contains(globs, "!/eval/results/**") {
		t.Fatalf("runtime artifact roots must not be hard-coded without typed configuration:\n%s", globs)
	}

	runtimeFilter := NewSearchDirFilterWithOptions(repo, repo, SearchDirFilterOptions{
		IncludeAuxiliarySource: true,
		RuntimeArtifactRoots:   []string{"eval/results"},
	})
	if !runtimeFilter.ExcludesRepoRelativePath("eval/results/run-1.repo/source.cj") {
		t.Fatal("configured runtime artifact root should stay excluded")
	}
	runtimeGlobs := strings.Join(runtimeFilter.RipgrepGlobs(), "\n")
	if !strings.Contains(runtimeGlobs, "!/eval/results/**") {
		t.Fatalf("configured runtime artifact root should be rendered for shell backends:\n%s", runtimeGlobs)
	}
}
