package index

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanExcludesVendoredThirdPartyTrees pins the vendored-tree
// exclusion: grammar test corpora and forked libs under thirdparty/
// (any spelling, any depth) must never enter the graph — they are
// dependency code, not the user's navigation target, and the
// 2026-06-12 repomap_v3 refresh showed corpus sources outranking real
// extractors on path-token matches. Real sources alongside them keep
// scanning.
func TestScanExcludesVendoredThirdPartyTrees(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"internal/app/main.go": "package app\nfunc Run() {}\n",
		"internal/thirdparty/tree-sitter-x/corpus/sources/sample.cj": "package demo\nclass Demo {}\n",
		"third_party/lib/util.py":                                    "def util():\n    pass\n",
		"third-party/widget/widget.js":                               "function widget() {}\n",
	}
	for rel, body := range files {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ScanFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[filepath.ToSlash(e.RelPath)] = true
	}
	if !got["internal/app/main.go"] {
		t.Errorf("real source missing from scan: %v", got)
	}
	for _, excluded := range []string{
		"internal/thirdparty/tree-sitter-x/corpus/sources/sample.cj",
		"third_party/lib/util.py",
		"third-party/widget/widget.js",
	} {
		if got[excluded] {
			t.Errorf("vendored path %s entered the scan — thirdparty trees must be excluded at any depth", excluded)
		}
	}
}
