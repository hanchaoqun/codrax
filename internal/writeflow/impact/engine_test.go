package impact

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildAnalysisResultProjectsImpactTargets(t *testing.T) {
	prevNow := nowImpactAnalysis
	nowImpactAnalysis = func() time.Time { return time.Unix(20, 0) }
	defer func() { nowImpactAnalysis = prevNow }()
	graph := fakeGraphProvider{
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
	result := BuildAnalysisResult(Input{
		Plan: &types.ChangePlan{
			ID: "plan-impact",
			VerificationProbes: []types.VerificationProbe{{
				ID:           "probe-contract",
				ContractRefs: []string{"contract-1"},
			}},
		},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-impact:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:     "pkg/a.py",
				Status:   "modified",
				PathRole: types.SourcePathRoleProduction,
				Hunks:    []types.PatchEffectHunk{{NewStart: 12, NewLines: 2}},
			}},
		},
		Graph: graph,
	})
	if result.ResultID == "" || result.ObligationSet == nil || result.CreatedAt != time.Unix(20, 0) {
		t.Fatalf("impact analysis identity missing: %+v", result)
	}
	if !impactAnalysisResultHasSurface(result, "hunk", "pkg/a.py", "") ||
		!impactAnalysisResultHasSurface(result, "symbol", "pkg/a.py", "py::pkg::::target::1") {
		t.Fatalf("changed surfaces missing: %+v", result.ChangedSurfaces)
	}
	if !impactAnalysisResultHasTarget(result, "behavior_contract", "", "contract-1") ||
		!impactAnalysisResultHasTarget(result, "dependent", "pkg/caller.py", "") ||
		!impactAnalysisResultHasTarget(result, "test_surface", "tests/test_a.py", "") {
		t.Fatalf("verification targets missing: %+v", result.VerificationTargets)
	}
	if result.VerificationTargets[0].Kind != "behavior_contract" {
		t.Fatalf("behavior contract should rank before graph targets: %+v", result.VerificationTargets)
	}
}

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

func TestAnnotatePatchEffectLineFeatureEventsAddsSoftEvents(t *testing.T) {
	graph := fakeGraphProvider{
		features: map[string]map[int][]string{
			"pkg/a.py": {
				10: []string{"guard"},
				11: []string{"call_expression"},
				12: []string{"assignment"},
			},
		},
	}
	record := types.PatchEffectRecord{
		Files: []types.PatchEffectFile{{
			Path:  "pkg/a.py",
			Hunks: []types.PatchEffectHunk{{NewStart: 10, NewLines: 3, AddedLines: 1, RemovedLines: 1}},
		}},
	}

	AnnotatePatchEffectLineFeatureEvents(&record, graph)
	for _, code := range []string{"control_flow_guard_touched", "call_site_touched", "state_assignment_touched"} {
		if !patchEffectHasEvent(record, code) {
			t.Fatalf("missing %s in events: %+v", code, record.Files[0].Events)
		}
	}
}

func impactAnalysisResultHasSurface(result types.ImpactAnalysisResult, kind, path, symbol string) bool {
	for _, surface := range result.ChangedSurfaces {
		if surface.Kind == kind && surface.Path == path && surface.Symbol == symbol {
			return true
		}
	}
	return false
}

func impactAnalysisResultHasTarget(result types.ImpactAnalysisResult, kind, relatedPath, contractRef string) bool {
	for _, target := range result.VerificationTargets {
		if target.Kind != kind {
			continue
		}
		if relatedPath != "" && target.RelatedPath != relatedPath {
			continue
		}
		if contractRef != "" && target.ContractRef != contractRef {
			continue
		}
		return true
	}
	return false
}

type fakeGraphProvider struct {
	imports        map[string][]string
	reverseImports map[string][]string
	tests          map[string][]string
	symbols        map[string][]SymbolRef
	features       map[string]map[int][]string
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

func (f fakeGraphProvider) LineFeaturesInRange(path string, startLine, endLine int) []string {
	var out []string
	for line := startLine; line <= endLine; line++ {
		out = append(out, f.features[path][line]...)
	}
	return out
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

func patchEffectHasEvent(record types.PatchEffectRecord, code string) bool {
	for _, file := range record.Files {
		for _, event := range file.Events {
			if event.Code == code && event.Severity == "warning" {
				return true
			}
		}
	}
	return false
}
