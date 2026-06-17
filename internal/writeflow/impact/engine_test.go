package impact

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildObligationSetMergesPatchEffectAndGraph(t *testing.T) {
	graph := fakeGraphProvider{
		imports: map[string][]string{
			"pkg/a.py": []string{"pkg/base.py"},
		},
		reverseImports: map[string][]string{
			"pkg/a.py": []string{"pkg/caller.py"},
		},
		tests: map[string][]string{
			"pkg/a.py": []string{"tests/test_a.py"},
		},
		symbols: map[string][]SymbolRef{
			"pkg/a.py": {{
				ID:      "py::pkg::::target::1",
				Name:    "target",
				File:    "pkg/a.py",
				Line:    10,
				EndLine: 20,
			}},
		},
	}
	set := BuildObligationSet(Input{
		Plan: &types.ChangePlan{
			ID: "plan-impact",
			Changes: []types.FileChange{{
				Path: "pkg/a.py",
				Kind: "modify",
			}},
		},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-impact:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:     "pkg/a.py",
				Status:   "modified",
				PathRole: types.SourcePathRoleProduction,
				Hunks:    []types.PatchEffectHunk{{NewStart: 12, NewLines: 2}},
				Events: []types.PatchEffectEvent{{
					Code:     "structured_file_parse_error",
					Severity: "error",
					Path:     "pkg/a.py",
				}},
			}},
		},
		Graph: graph,
	})

	for _, want := range []struct {
		kind     string
		relation string
		path     string
		related  string
		symbol   string
	}{
		{kind: "changed_file", relation: "actual_diff", path: "pkg/a.py"},
		{kind: "effect_followup", relation: "structured_file_parse_error", path: "pkg/a.py"},
		{kind: "dependency", relation: "imports", path: "pkg/a.py", related: "pkg/base.py"},
		{kind: "dependent", relation: "reverse_import", path: "pkg/a.py", related: "pkg/caller.py"},
		{kind: "test_surface", relation: "related_test", path: "pkg/a.py", related: "tests/test_a.py"},
		{kind: "changed_symbol", relation: "patch_hunk", path: "pkg/a.py", symbol: "py::pkg::::target::1"},
	} {
		if !impactSetHasObligation(set, want.kind, want.relation, want.path, want.related, want.symbol) {
			t.Fatalf("missing obligation %+v in %+v", want, set.Obligations)
		}
	}
}

func TestBuildObligationSetSkipsGraphForAuxiliaryPatchEffectPath(t *testing.T) {
	graph := fakeGraphProvider{
		reverseImports: map[string][]string{
			"tests/test_a.py": []string{"pkg/a.py"},
		},
	}
	set := BuildObligationSet(Input{
		PatchEffect: &types.PatchEffectRecord{
			Files: []types.PatchEffectFile{{
				Path:     "tests/test_a.py",
				Status:   "modified",
				PathRole: types.SourcePathRoleTest,
			}},
		},
		Graph: graph,
	})
	if impactSetHasObligation(set, "dependent", "reverse_import", "tests/test_a.py", "pkg/a.py", "") {
		t.Fatalf("test-only patch paths should not spawn graph impact obligations: %+v", set.Obligations)
	}
	if !impactSetHasObligation(set, "changed_file", "actual_diff", "tests/test_a.py", "", "") {
		t.Fatalf("test path should still be recorded as an actual changed file: %+v", set.Obligations)
	}
}

type fakeGraphProvider struct {
	imports        map[string][]string
	reverseImports map[string][]string
	tests          map[string][]string
	symbols        map[string][]SymbolRef
}

func (f fakeGraphProvider) Imports(path string) []string {
	return append([]string(nil), f.imports[path]...)
}

func (f fakeGraphProvider) ReverseImports(path string) []string {
	return append([]string(nil), f.reverseImports[path]...)
}

func (f fakeGraphProvider) RelatedTests(path string) []string {
	return append([]string(nil), f.tests[path]...)
}

func (f fakeGraphProvider) SymbolsInFile(path string) []SymbolRef {
	return append([]SymbolRef(nil), f.symbols[path]...)
}

func impactSetHasObligation(set types.ImpactObligationSet, kind, relation, path, related, symbol string) bool {
	for _, ob := range set.Obligations {
		if ob.Kind != kind || ob.Relation != relation || ob.SubjectPath != path {
			continue
		}
		if related != "" && ob.RelatedPath != related {
			continue
		}
		if symbol != "" && ob.SubjectSymbol != symbol {
			continue
		}
		return true
	}
	return false
}
