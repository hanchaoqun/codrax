package impact

import (
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func rootAdapterGraph(root string) *rmtypes.Graph {
	return &rmtypes.Graph{
		Root: root,
		FileIndex: map[string]*rmtypes.FileInfo{
			"src/widget.py": {
				RelPath: "src/widget.py",
				Symbols: []rmtypes.Symbol{{ID: "py::src/widget.py::Widget::7", Name: "Widget", File: "src/widget.py", Line: 7, EndLine: 10}},
				LineFeatures: map[int][]rmtypes.LineFeature{
					7: {rmtypes.LineFeatureGuard},
					8: {rmtypes.LineFeatureGuard, rmtypes.LineFeatureCallExpression},
					9: {rmtypes.LineFeatureReturnStmt},
				},
			},
			"src/base.py":           {RelPath: "src/base.py"},
			"src/caller.py":         {RelPath: "src/caller.py"},
			"tests/test_widget.py":  {RelPath: "tests/test_widget.py"},
			"tests/test_foreign.py": {RelPath: "tests/test_foreign.py"},
		},
		ImportGraph:    map[string][]string{"src/widget.py": {"src/base.py"}},
		ReverseImports: map[string][]string{"src/widget.py": {"src/caller.py"}},
		Scores:         map[string]float64{"src/widget.py": 0.75},
	}
}

func rootAdapterDirectory(t *testing.T, root string, parts ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func assertRootAdapterMethods(t *testing.T, p GraphProvider, prefix string) {
	t.Helper()
	if p == nil {
		t.Fatal("authorized graph has no provider")
	}
	file := path.Join(prefix, "src/widget.py")
	for _, tc := range []struct {
		name string
		got  []string
		want []string
	}{
		{"imports", p.Imports(file), []string{path.Join(prefix, "src/base.py")}},
		{"reverse imports", p.ReverseImports(file), []string{path.Join(prefix, "src/caller.py")}},
		{"related tests", p.RelatedTests(file), []string{path.Join(prefix, "tests/test_widget.py")}},
	} {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	want := []SymbolRef{{ID: "py::src/widget.py::Widget::7", Name: "Widget", File: file, Line: 7, EndLine: 10}}
	if got := p.SymbolsInFile(file); !reflect.DeepEqual(got, want) {
		t.Errorf("symbol identity/range or file mapping changed: got %+v, want %+v", got, want)
	}
	features, ok := p.(LineFeatureProvider)
	if !ok {
		t.Fatal("rooted graph lost line-feature provider")
	}
	if got := features.LineFeaturesInRange(file, 7, 8); !reflect.DeepEqual(got, []string{"call_expression", "guard"}) {
		t.Errorf("line features were broadened or lost: %v", got)
	}
}

func assertRootAdapterEmpty(t *testing.T, p GraphProvider, file string) {
	t.Helper()
	if p == nil {
		return
	}
	if got := p.Imports(file); len(got) != 0 {
		t.Errorf("foreign/unindexed imports(%q) = %v", file, got)
	}
	if got := p.ReverseImports(file); len(got) != 0 {
		t.Errorf("foreign/unindexed reverse imports(%q) = %v", file, got)
	}
	if got := p.RelatedTests(file); len(got) != 0 {
		t.Errorf("foreign/unindexed related tests(%q) = %v", file, got)
	}
	if got := p.SymbolsInFile(file); len(got) != 0 {
		t.Errorf("foreign/unindexed symbols(%q) = %+v", file, got)
	}
	if features, ok := p.(LineFeatureProvider); ok {
		if got := features.LineFeaturesInRange(file, 7, 8); len(got) != 0 {
			t.Errorf("foreign/unindexed line features(%q) = %v", file, got)
		}
	}
}

func TestRootedRepomapAdapterFiveMethodsRespectMainAndWorktreeRoots(t *testing.T) {
	base := t.TempDir()
	main := rootAdapterDirectory(t, base, "main")
	worktree := rootAdapterDirectory(t, base, "worktree")
	for _, tc := range []struct {
		name, root, prefix string
	}{
		{"whole main", main, ""},
		{"whole worktree", worktree, ""},
		{"child main", rootAdapterDirectory(t, main, "packages/widget"), "packages/widget"},
		{"child worktree", rootAdapterDirectory(t, worktree, "packages/widget"), "packages/widget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, before := rootAdapterGraph(tc.root), rootAdapterGraph(tc.root)
			p := GraphProviderFromSearchGraphForRoots(g, main, worktree)
			assertRootAdapterMethods(t, p, tc.prefix)
			assertRootAdapterMethods(t, GraphProviderFromSearchGraphForRoots(g, worktree, main), tc.prefix)
			if tc.prefix != "" {
				for _, bad := range []string{"src/widget.py", "packages/sibling/src/widget.py", "packages/widget-extra/src/widget.py", "packages/widget"} {
					assertRootAdapterEmpty(t, p, bad)
				}
			}
			for _, bad := range []string{"../src/widget.py", "/src/widget.py", " src/widget.py", "packages/widget/../../../src/widget.py", "packages\\widget\\src\\widget.py"} {
				assertRootAdapterEmpty(t, p, bad)
			}
			if !reflect.DeepEqual(g, before) {
				t.Fatal("adapter modified the shared graph or its graph-local identities")
			}
			// A graph-local caller remains compatible after the rooted view is used.
			assertRootAdapterMethods(t, GraphProviderFromSearchGraph(g), "")
		})
	}
}

func TestRootedRepomapAdapterDoesNotInferRootEquivalence(t *testing.T) {
	base := t.TempDir()
	main := rootAdapterDirectory(t, base, "main")
	worktree := rootAdapterDirectory(t, base, "worktree")
	foreign := rootAdapterDirectory(t, base, "foreign", "main")
	file := filepath.Join(main, "not-a-directory")
	if err := os.WriteFile(file, []byte("source file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		handle any
		roots  []string
	}{
		{"nil handle", nil, []string{main}},
		{"typed nil", (*rmtypes.Graph)(nil), []string{main}},
		{"wrong type", struct{}{}, []string{main}},
		{"no authorized roots", rootAdapterGraph(main), nil},
		{"empty graph root", rootAdapterGraph(""), []string{main}},
		{"relative graph root", rootAdapterGraph("main"), []string{main}},
		{"missing graph root", rootAdapterGraph(filepath.Join(main, "missing")), []string{main}},
		{"foreign same basename", rootAdapterGraph(foreign), []string{main}},
		{"worktree not explicitly authorized", rootAdapterGraph(worktree), []string{main}},
		{"parent graph not child scope", rootAdapterGraph(base), []string{main}},
		{"relative authorized root", rootAdapterGraph(main), []string{"main"}},
		{"missing authorized root", rootAdapterGraph(main), []string{filepath.Join(base, "missing")}},
		{"graph root is file", rootAdapterGraph(file), []string{main}},
		{"authorized root is file", rootAdapterGraph(file), []string{file}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if p := GraphProviderFromSearchGraphForRoots(tc.handle, tc.roots...); p != nil {
				t.Fatal("unknown, foreign, or non-directory root was granted a rooted graph")
			}
		})
	}
}

func TestRootedRepomapAdapterResolvesRootSymlinksWithoutEscape(t *testing.T) {
	base := t.TempDir()
	main := rootAdapterDirectory(t, base, "main")
	child := rootAdapterDirectory(t, main, "packages/widget")
	foreign := rootAdapterDirectory(t, base, "foreign")
	escape := filepath.Join(main, "external")
	alias := filepath.Join(base, "main-alias")
	if err := os.Symlink(foreign, escape); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(main, alias); err != nil {
		t.Fatal(err)
	}
	if p := GraphProviderFromSearchGraphForRoots(rootAdapterGraph(escape), main); p != nil {
		t.Fatal("lexically nested graph root escaped via a symlink")
	}
	assertRootAdapterMethods(t, GraphProviderFromSearchGraphForRoots(rootAdapterGraph(child), alias), "packages/widget")
	assertRootAdapterMethods(t, GraphProviderFromSearchGraphForRoots(rootAdapterGraph(filepath.Join(alias, "packages/widget")), main), "packages/widget")
}

func TestRootedRepomapAdapterPartialGraphDoesNotInventCoverage(t *testing.T) {
	main := t.TempDir()
	child := rootAdapterDirectory(t, main, "packages/widget")
	for _, noIndex := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "no FileIndex"}[noIndex], func(t *testing.T) {
			g := rootAdapterGraph(child)
			// Old graph edges and files[] alone do not establish indexed membership.
			g.Files = []*rmtypes.FileInfo{{RelPath: "tests/test_widget_hidden.py"}}
			g.ImportGraph["src/widget.py"] = append(g.ImportGraph["src/widget.py"], "src/hidden.py", "../sibling/base.py", "/foreign.py")
			g.ReverseImports["src/widget.py"] = append(g.ReverseImports["src/widget.py"], "src/unindexed.py")
			g.ImportGraph["src/unindexed.py"] = []string{"src/base.py"}
			g.ReverseImports["src/unindexed.py"] = []string{"src/caller.py"}
			g.FileIndex["src/nil.py"] = nil
			g.ImportGraph["src/nil.py"] = []string{"src/base.py"}
			if noIndex {
				g.FileIndex = nil
			}
			p := GraphProviderFromSearchGraphForRoots(g, main)
			if noIndex {
				assertRootAdapterEmpty(t, p, "packages/widget/src/widget.py")
			} else {
				assertRootAdapterMethods(t, p, "packages/widget")
			}
			for _, missing := range []string{"src/unindexed.py", "src/nil.py", "tests/test_widget_hidden.py", "../sibling/src/widget.py"} {
				assertRootAdapterEmpty(t, p, "packages/widget/"+missing)
			}
		})
	}
}
