package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestJsImportResolver exercises tsconfig path-alias rewriting plus
// the extension + /index.* candidate matrix. Fixtures are laid out
// on disk under t.TempDir() because the resolver reads tsconfig
// itself; the file index is populated through BuildGraph's
// FileIndex, which does not require the files to actually exist but
// we create them so the test feels realistic and the fallback code
// paths can stat.
func TestJsImportResolver(t *testing.T) {
	dir := t.TempDir()
	tsconfig := `{
  // JSONC inline comments must be tolerated
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "@components/*": ["src/components/*"],
      "@utils": ["src/util/index.ts"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatal(err)
	}

	files := []*types.FileInfo{
		{RelPath: "tsconfig.json", IsSpecial: true},
		{RelPath: "src/app.ts", Language: types.LangTypeScript},
		{RelPath: "src/components/Button.tsx", Language: types.LangTypeScript},
		{RelPath: "src/components/Modal/index.ts", Language: types.LangTypeScript},
		{RelPath: "src/util/index.ts", Language: types.LangTypeScript},
		{RelPath: "src/util/strings.ts", Language: types.LangTypeScript},
	}
	g := BuildGraph(dir, files)

	r := &jsImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(r.aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %d: %+v", len(r.aliases), r.aliases)
	}

	cases := []struct {
		label   string
		imp     string
		want    []string
		wantRsn string
	}{
		{
			label: "glob alias to .tsx file",
			imp:   "@/components/Button",
			want:  []string{"src/components/Button.tsx"},
		},
		{
			label: "glob alias to directory index",
			imp:   "@/components/Modal",
			want:  []string{"src/components/Modal/index.ts"},
		},
		{
			label: "longer-prefix alias wins over shorter",
			imp:   "@components/Button",
			want:  []string{"src/components/Button.tsx"},
		},
		{
			label: "glob alias to .ts file in subdir",
			imp:   "@/util/strings",
			want:  []string{"src/util/strings.ts"},
		},
		{
			label: "non-glob exact alias to explicit file",
			imp:   "@utils",
			want:  []string{"src/util/index.ts"},
		},
		{
			label:   "bare module external",
			imp:     "react",
			wantRsn: "js_external",
		},
		{
			label:   "scoped bare module external",
			imp:     "@scope/pkg",
			wantRsn: "js_external",
		},
		{
			label:   "alias prefix match but target missing",
			imp:     "@/ghost/nowhere",
			wantRsn: "js_external",
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, files[1], types.Import{Path: c.imp}, ctx)
			if !sameStringSet(got, c.want) {
				t.Errorf("Resolve(%q) = %v, want %v", c.imp, got, c.want)
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
				t.Errorf("unexpected Unresolved append: %+v",
					ctx.Unresolved[before:])
			}
		})
	}
}

// TestJsImportResolverNoTsconfig verifies that the resolver degrades
// cleanly when no tsconfig exists: every non-relative import is
// classified js_external.
func TestJsImportResolverNoTsconfig(t *testing.T) {
	dir := t.TempDir()
	files := []*types.FileInfo{
		{RelPath: "a.ts", Language: types.LangTypeScript},
	}
	g := BuildGraph(dir, files)

	r := &jsImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(r.aliases) != 0 {
		t.Errorf("expected no aliases, got %+v", r.aliases)
	}
	got := r.Resolve(g, files[0], types.Import{Path: "lodash"}, ctx)
	if got != nil {
		t.Errorf("Resolve(lodash) = %v, want nil", got)
	}
	if n := len(ctx.Unresolved); n != 1 || ctx.Unresolved[0].Reason != "js_external" {
		t.Errorf("unresolved = %+v", ctx.Unresolved)
	}
}

// TestJsImportResolverSharedInstance verifies that running Prepare
// twice on the same struct (as happens when JS and TS both point at
// the same resolver in defaultResolvers) doesn't double-populate.
func TestJsImportResolverSharedInstance(t *testing.T) {
	dir := t.TempDir()
	tsconfig := `{"compilerOptions":{"paths":{"@/*":["src/*"]}}}`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []*types.FileInfo{
		{RelPath: "tsconfig.json", IsSpecial: true},
		{RelPath: "src/a.ts", Language: types.LangTypeScript},
	}
	g := BuildGraph(dir, files)

	r := &jsImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatal(err)
	}
	firstLen := len(r.aliases)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.aliases) != firstLen {
		t.Errorf("second Prepare duplicated aliases: was %d now %d",
			firstLen, len(r.aliases))
	}
}
