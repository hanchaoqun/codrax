package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestKotlinImportResolver covers the JVM package-declaration match,
// wildcard expansion, the precise top-level-decl hit, and the
// member-import fallback to "any file in the package".
func TestKotlinImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "src/main/com/app/User.kt",
			Language: types.LangKotlin,
			Package:  "com.app",
			Symbols:  []types.Symbol{{Name: "User", Kind: "class"}},
		},
		{
			RelPath:  "src/main/com/app/Utils.kt",
			Language: types.LangKotlin,
			Package:  "com.app",
			Symbols:  []types.Symbol{{Name: "helper", Kind: "function"}},
		},
		{
			RelPath:  "src/main/com/other/Thing.kt",
			Language: types.LangKotlin,
			Package:  "com.other",
			Symbols:  []types.Symbol{{Name: "Thing", Kind: "class"}},
		},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &kotlinImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label string
		path  string
		want  []string
	}{
		{
			label: "precise class decl import",
			path:  "com.app.User",
			want:  []string{"src/main/com/app/User.kt"},
		},
		{
			label: "top-level function decl import",
			path:  "com.app.helper",
			want:  []string{"src/main/com/app/Utils.kt"},
		},
		{
			label: "wildcard import resolves whole package",
			path:  "com.app.*",
			want:  []string{"src/main/com/app/User.kt", "src/main/com/app/Utils.kt"},
		},
		{
			label: "member import with no decl hit falls back to package",
			path:  "com.app.Unknown",
			want:  []string{"src/main/com/app/User.kt", "src/main/com/app/Utils.kt"},
		},
		{
			label: "top-level symbol with no package is unresolvable",
			path:  "Standalone",
			want:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			got := r.Resolve(g, files[0], types.Import{Path: c.path}, ctx)
			if !sameStringSet(got, c.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

// TestArkTSImportResolver covers the HarmonyOS builtin black-hole
// rules (which must return nil rather than fall through to basename
// glob) and the unmapped-bundle path.
func TestArkTSImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "entry/src/main/ets/pages/Index.ets", Language: types.LangArkTS},
	}
	g := BuildGraph(t.TempDir(), files)
	js := &jsImportResolver{}
	r := newArkTSImportResolver(js)
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	builtins := []string{
		"@ohos.window",
		"@kit.ArkUI",
		"@hms.security.cryptoFramework",
		"@arkui.component",
		"@system.router",
	}
	for _, path := range builtins {
		t.Run("blackhole "+path, func(t *testing.T) {
			if got := r.Resolve(g, files[0], types.Import{Path: path}, ctx); got != nil {
				t.Fatalf("Resolve(%q) = %v, want nil (builtin must black-hole)", path, got)
			}
		})
	}

	t.Run("unmapped bundle is unresolvable", func(t *testing.T) {
		got := r.Resolve(g, files[0], types.Import{Path: "@bundle:com.unknown.feature/utils"}, ctx)
		if got != nil {
			t.Fatalf("Resolve(@bundle:com.unknown.feature/utils) = %v, want nil", got)
		}
	})
}

// TestCangjieImportResolver covers the stdlib/core black-hole, the
// exact package lookup, the shrinking-prefix fallback, and the
// last-component basename salvage.
func TestCangjieImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "src/cart/cart.cj", Language: types.LangCangjie, Package: "demo.cart"},
		{RelPath: "src/widget/Widget.cj", Language: types.LangCangjie},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &cangjieImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label string
		path  string
		want  []string
	}{
		{label: "stdlib import black-holed", path: "std.collection.ArrayList", want: nil},
		{label: "core import black-holed", path: "core.Option", want: nil},
		{label: "runtime import black-holed", path: "runtime.gc", want: nil},
		{label: "harmony import black-holed", path: "ohos.hilog", want: nil},
		{label: "exact package lookup", path: "demo.cart", want: []string{"src/cart/cart.cj"}},
		{label: "shrinking-prefix fallback", path: "demo.cart.Item", want: []string{"src/cart/cart.cj"}},
		{label: "last-component basename salvage", path: "pkg.Widget", want: []string{"src/widget/Widget.cj"}},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			got := r.Resolve(g, files[0], types.Import{Path: c.path}, ctx)
			if !sameStringSet(got, c.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}
