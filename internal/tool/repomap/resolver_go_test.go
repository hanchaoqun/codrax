package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGoImportResolver builds a throwaway repo with a handcrafted
// go.mod and exercises every branch of goImportResolver: internal
// exact match, internal package with multiple files, internal miss,
// external (stdlib + third-party), v2 sub-module stripping, and the
// no-go.mod fallback.
func TestGoImportResolver(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/sample\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []*FileInfo{
		{RelPath: "main.go", Language: LangGo, Package: "sample", IsSpecial: false},
		{RelPath: "internal/a/a.go", Language: LangGo, Package: "a"},
		{RelPath: "internal/a/a_helpers.go", Language: LangGo, Package: "a"},
		{RelPath: "internal/b/b.go", Language: LangGo, Package: "b"},
		{RelPath: "go.mod", Language: "", IsSpecial: true},
	}
	g := BuildGraph(dir, files)

	var r goImportResolver
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if r.modulePath != "example.com/sample" {
		t.Fatalf("modulePath = %q, want example.com/sample", r.modulePath)
	}

	cases := []struct {
		label   string
		fi      *FileInfo
		imp     Import
		want    []string
		wantRsn string // non-empty when we expect an Unresolved entry with this reason
	}{
		{
			label: "internal exact match single file",
			fi:    files[3], // internal/b/b.go
			imp:   Import{Path: "example.com/sample/internal/b"},
			want:  []string{"internal/b/b.go"},
		},
		{
			label: "internal package with multiple files",
			fi:    files[0],
			imp:   Import{Path: "example.com/sample/internal/a"},
			want:  []string{"internal/a/a.go", "internal/a/a_helpers.go"},
		},
		{
			label:   "internal miss (directory does not exist)",
			fi:      files[0],
			imp:     Import{Path: "example.com/sample/internal/c"},
			want:    nil,
			wantRsn: "go_internal_miss",
		},
		{
			label:   "stdlib external",
			fi:      files[0],
			imp:     Import{Path: "fmt"},
			want:    nil,
			wantRsn: "go_external",
		},
		{
			label:   "third-party external",
			fi:      files[0],
			imp:     Import{Path: "github.com/some/lib/pkg"},
			want:    nil,
			wantRsn: "go_external",
		},
		{
			label: "root module self-import",
			fi:    files[1],
			imp:   Import{Path: "example.com/sample"},
			want:  []string{"main.go"},
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, c.fi, c.imp, ctx)
			if !sameStringSet(got, c.want) {
				t.Errorf("Resolve(%q) = %v, want %v", c.imp.Path, got, c.want)
			}
			if c.wantRsn != "" {
				if len(ctx.Unresolved) != before+1 {
					t.Fatalf("expected one Unresolved append, got delta %d",
						len(ctx.Unresolved)-before)
				}
				if ctx.Unresolved[before].Reason != c.wantRsn {
					t.Errorf("Reason = %q, want %q",
						ctx.Unresolved[before].Reason, c.wantRsn)
				}
			} else if len(ctx.Unresolved) != before {
				t.Errorf("unexpected Unresolved append: %+v", ctx.Unresolved[before:])
			}
		})
	}
}

// TestGoImportResolverNoModule covers the fallback path when no
// go.mod is present: Resolve must delegate to the legacy resolver
// without panicking.
func TestGoImportResolverNoModule(t *testing.T) {
	dir := t.TempDir()
	files := []*FileInfo{
		{RelPath: "a.go", Language: LangGo, Package: "main"},
	}
	g := BuildGraph(dir, files)

	var r goImportResolver
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if r.modulePath != "" {
		t.Errorf("expected empty modulePath, got %q", r.modulePath)
	}
	// Should not panic. Legacy resolver will return nil for an
	// unknown external path; that is acceptable.
	_ = r.Resolve(g, files[0], Import{Path: "fmt"}, ctx)
}

// TestStripGoVersionSegment locks the v2+ stripping behavior used
// when an import path embeds a major-version segment the file layout
// does not include.
func TestStripGoVersionSegment(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"example.com/foo/v2/bar", "example.com/foo/bar"},
		{"example.com/foo/v9/bar", "example.com/foo/bar"},
		{"example.com/foo/v10/bar", "example.com/foo/bar"},
		{"example.com/foo/v1/bar", "example.com/foo/v1/bar"},   // v1 not a module version segment
		{"example.com/foo/vN/bar", "example.com/foo/vN/bar"},   // non-numeric
		{"example.com/foo/valid/bar", "example.com/foo/valid/bar"},
		{"example.com/foo", "example.com/foo"},
	}
	for _, c := range cases {
		if got := stripGoVersionSegment(c.in); got != c.out {
			t.Errorf("stripGoVersionSegment(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}
