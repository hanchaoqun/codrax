package convention

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLearnFromGraphBuildsRepositoryConventionNodes(t *testing.T) {
	graph := fakeConventionGraphProvider{
		imports: map[string][]string{
			"pkg/a.py": []string{"pkg/base.py"},
		},
		reverseImports: map[string][]string{
			"pkg/a.py": []string{"pkg/caller.py"},
		},
		tests: map[string][]string{
			"pkg/a.py": []string{"tests/test_a.py"},
		},
	}
	got := LearnFromGraph(LearnInput{
		BatchID:     "batch-1",
		Goal:        "repair axis",
		ActivePaths: []string{"pkg/a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef",
			Files: []types.PatchEffectFile{{
				Path:     "pkg/a.py",
				Language: "py",
				PathRole: types.SourcePathRoleProduction,
			}},
		},
		Graph: graph,
	})
	for _, want := range []string{
		"actual patch touched production path pkg/a.py language=py",
		"related test surface tests/test_a.py is associated with pkg/a.py",
		"dependent surface pkg/caller.py imports pkg/a.py",
		"changed surface pkg/a.py imports dependency pkg/base.py",
	} {
		if !conventionGraphHasSummary(got, want) {
			t.Fatalf("missing convention %q in %+v", want, got.Nodes)
		}
	}
}

func TestLearnFromGraphIsSoftOnlyTypedConvention(t *testing.T) {
	got := LearnFromGraph(LearnInput{
		ActivePaths: []string{"pkg/a.py"},
		PatchEffect: &types.PatchEffectRecord{
			Files: []types.PatchEffectFile{{
				Path:     "pkg/a.py",
				PathRole: types.SourcePathRoleProduction,
			}},
		},
	})
	if len(got.Nodes) == 0 {
		t.Fatal("expected patch-surface convention node")
	}
	for _, node := range got.Nodes {
		if node.SourceStage != "convention_learner" || node.Strength == "" {
			t.Fatalf("learner node must carry typed soft provenance: %+v", node)
		}
	}
}

type fakeConventionGraphProvider struct {
	imports        map[string][]string
	reverseImports map[string][]string
	tests          map[string][]string
}

func (f fakeConventionGraphProvider) Imports(path string) []string {
	return append([]string(nil), f.imports[path]...)
}

func (f fakeConventionGraphProvider) ReverseImports(path string) []string {
	return append([]string(nil), f.reverseImports[path]...)
}

func (f fakeConventionGraphProvider) RelatedTests(path string) []string {
	return append([]string(nil), f.tests[path]...)
}

func conventionGraphHasSummary(graph types.ConventionGraph, want string) bool {
	for _, node := range graph.Nodes {
		if node.Summary == want {
			return true
		}
	}
	return false
}
