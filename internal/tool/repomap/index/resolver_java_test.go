package index

import (
	"testing"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestJavaImportResolver covers the single-class, wildcard,
// package-fallback, external, and degenerate branches of the Java
// resolver using a handcrafted types.FileInfo graph — no tree-sitter parse
// in the loop, just the Prepare / Resolve logic.
func TestJavaImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "src/com/acme/app/App.java",
			Language: types.LangJava,
			Package:  "com.acme.app",
			Symbols: []types.Symbol{
				{Name: "App", Kind: "class"},
				{Name: "Inner", Kind: "class"},
			},
		},
		{
			RelPath:  "src/com/acme/app/Helper.java",
			Language: types.LangJava,
			Package:  "com.acme.app",
			Symbols: []types.Symbol{
				{Name: "Helper", Kind: "class"},
				{Name: "Flag", Kind: "enum"},
			},
		},
		{
			RelPath:  "src/com/acme/util/StringOps.java",
			Language: types.LangJava,
			Package:  "com.acme.util",
			Symbols: []types.Symbol{
				{Name: "StringOps", Kind: "class"},
				{Name: "Joinable", Kind: "interface"},
			},
		},
	}
	g := BuildGraph(t.TempDir(), files)

	var r javaImportResolver
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label   string
		imp     string
		want    []string
		wantRsn string
	}{
		{
			label: "class in known package",
			imp:   "com.acme.app.App",
			want:  []string{"src/com/acme/app/App.java"},
		},
		{
			label: "enum in known package",
			imp:   "com.acme.app.Flag",
			want:  []string{"src/com/acme/app/Helper.java"},
		},
		{
			label: "interface in other package",
			imp:   "com.acme.util.Joinable",
			want:  []string{"src/com/acme/util/StringOps.java"},
		},
		{
			label: "wildcard import",
			imp:   "com.acme.app.*",
			want: []string{
				"src/com/acme/app/App.java",
				"src/com/acme/app/Helper.java",
			},
		},
		{
			label: "package match but unknown class name (fallback to all files in package)",
			imp:   "com.acme.app.MissingClass",
			want: []string{
				"src/com/acme/app/App.java",
				"src/com/acme/app/Helper.java",
			},
		},
		{
			label:   "stdlib external",
			imp:     "java.util.List",
			wantRsn: "java_external",
		},
		{
			label:   "third-party external",
			imp:     "org.junit.Test",
			wantRsn: "java_external",
		},
		{
			label:   "degenerate single-segment",
			imp:     "Oops",
			wantRsn: "java_external",
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, files[0], types.Import{Path: c.imp}, ctx)
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
