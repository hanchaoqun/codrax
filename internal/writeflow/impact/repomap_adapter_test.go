package impact

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestGraphProviderFromSearchGraphAdaptsRepomapGraph(t *testing.T) {
	graph := &rmtypes.Graph{
		FileIndex: map[string]*rmtypes.FileInfo{
			"pkg/a.py": {
				RelPath: "pkg/a.py",
				LineFeatures: map[int][]rmtypes.LineFeature{
					3: []rmtypes.LineFeature{rmtypes.LineFeatureGuard},
				},
				Symbols: []rmtypes.Symbol{{
					ID:      rmtypes.SymbolID("py::pkg::::target::1"),
					Name:    "target",
					File:    "pkg/a.py",
					Line:    3,
					EndLine: 5,
				}},
			},
			"pkg/base.py":        {RelPath: "pkg/base.py"},
			"pkg/caller.py":      {RelPath: "pkg/caller.py"},
			"pkg/test_a.py":      {RelPath: "pkg/test_a.py"},
			"other/test_b.py":    {RelPath: "other/test_b.py"},
			"pkg/not_related.md": {RelPath: "pkg/not_related.md"},
		},
		ImportGraph: map[string][]string{
			"pkg/a.py": []string{"pkg/base.py"},
		},
		ReverseImports: map[string][]string{
			"pkg/a.py": []string{"pkg/caller.py"},
		},
	}

	provider := GraphProviderFromSearchGraph(graph)
	if provider == nil {
		t.Fatal("expected repomap graph provider")
	}
	if got := provider.Imports("pkg/a.py"); len(got) != 1 || got[0] != "pkg/base.py" {
		t.Fatalf("imports not adapted: %+v", got)
	}
	if got := provider.ReverseImports("pkg/a.py"); len(got) != 1 || got[0] != "pkg/caller.py" {
		t.Fatalf("reverse imports not adapted: %+v", got)
	}
	if got := provider.RelatedTests("pkg/a.py"); len(got) != 1 || got[0] != "pkg/test_a.py" {
		t.Fatalf("related tests not adapted/scoped: %+v", got)
	}
	if got := provider.SymbolsInFile("pkg/a.py"); len(got) != 1 || got[0].ID != "py::pkg::::target::1" {
		t.Fatalf("symbols not adapted: %+v", got)
	}
	featureProvider, ok := provider.(LineFeatureProvider)
	if !ok {
		t.Fatal("repomap provider should expose line features")
	}
	if got := featureProvider.LineFeaturesInRange("pkg/a.py", 3, 3); len(got) != 1 || got[0] != "guard" {
		t.Fatalf("line features not adapted: %+v", got)
	}
}
