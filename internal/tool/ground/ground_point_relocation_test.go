package ground

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestGroundPointRelocationActualJavaDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		annotation, declaration int
	}{
		{"EchoHandler", 7, 8},
		{"UpperHandler", 9, 10},
		{"StatsHandler", 13, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "src/main/java/app/handlers/" + tc.name + ".java"
			data, err := os.ReadFile(filepath.Join("../../..", "eval/fixtures/java-annotation-router", source))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			if !strings.Contains(lines[tc.annotation-1], "@Route(") || !strings.Contains(lines[tc.declaration-1], "class "+tc.name) {
				t.Fatal("fixture no longer has distinct annotation and declaration lines")
			}
			gc := &Context{LineIndex: buildLineIndex([]types.ToolResult{buildGutterReadResult(source, 1, lines, len(lines))}, "")}
			item := types.EvidenceItem{ID: tc.name, Kind: types.EvidenceDirect, Scope: types.ScopeLine,
				Source: source, LineStart: tc.annotation, LineEnd: tc.annotation,
				AnchorKind: types.AnchorDefinition, AnchorSymbol: tc.name, Subject: tc.name, Predicate: "defined_in", Object: source}
			report := GroundItemScoped(&item, gc)
			if report.Status != types.GroundingGrounded || report.Tier != types.TierLineText || report.OriginalLine != tc.annotation || report.AdjustedLine != tc.declaration {
				t.Fatalf("declaration lookup premise failed: %+v / %+v", report, item)
			}
			if item.LineStart != tc.declaration || item.LineEnd != tc.declaration {
				t.Fatalf("relocated point = %d-%d, want exact declaration %d-%d", item.LineStart, item.LineEnd, tc.declaration, tc.declaration)
			}
			if item.AnchorKind != types.AnchorDefinition || types.ClaimFormOf(item) != types.ClaimDefinitionFact || item.Snippet != strings.TrimSpace(lines[tc.declaration-1]) {
				t.Fatalf("relocation must retain declaration-only authority: %+v", item)
			}
		})
	}
}

func TestGroundPointRelocationAcrossTiers(t *testing.T) {
	for _, tier := range []types.GroundingTier{types.TierLineText, types.TierFQNameSameFile, types.TierSnippetFuzzy, types.TierPackageSymbol, types.TierNearestCall, types.TierNearestCondition} {
		for _, delta := range []int{-5, 5} {
			for _, explicitEnd := range []bool{false, true} {
				name := string(tier) + "/delta=" + itoa(delta) + "/end=" + itoa(boolInt(explicitEnd))
				t.Run(name, func(t *testing.T) {
					original, target := 20, 20+delta
					if tier == types.TierLineText {
						target = 20 + delta/5
					}
					item := types.EvidenceItem{ID: "point", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
						Source: "pkg/a.go", LineStart: original, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Target"}
					if explicitEnd {
						item.LineEnd = original
					}
					gc := &Context{LineIndex: map[string]map[int]string{}, Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{}}}
					wantSource := item.Source
					switch tier {
					case types.TierLineText:
						gc.LineIndex[item.Source] = map[int]string{target: "func Target() {}"}
					case types.TierFQNameSameFile:
						gc.Graph.FileIndex[item.Source] = &repomap.FileInfo{RelPath: item.Source, Symbols: []repomap.Symbol{{Name: "Target", Kind: "function", Line: target}}}
					case types.TierSnippetFuzzy:
						item.AnchorKind, item.AnchorSymbol, item.Snippet = types.AnchorAssignment, "result", "result := computeDistinctValue()"
						gc.LineIndex[item.Source] = map[int]string{target: item.Snippet}
					case types.TierPackageSymbol:
						wantSource = "pkg/b.go"
						gc.Graph.FileIndex[item.Source] = &repomap.FileInfo{RelPath: item.Source, Package: "pkg"}
						gc.Graph.FileIndex[wantSource] = &repomap.FileInfo{RelPath: wantSource, Package: "pkg"}
						gc.Graph.SymbolDefs = map[string][]*repomap.Symbol{"Target": {{Name: "Target", Kind: "function", File: wantSource, Line: target}}}
					case types.TierNearestCall:
						item.AnchorKind, item.Subject, item.Object = types.AnchorCall, "owner", "Target"
						gc.Graph.FileIndex[item.Source] = &repomap.FileInfo{RelPath: item.Source, Relations: []repomap.Relation{{Kind: "call", File: item.Source, Line: target, ToEP: repomap.RelationEndpoint{Name: "Target"}}}}
					case types.TierNearestCondition:
						item.AnchorKind, item.AnchorSymbol = types.AnchorCondition, "ready"
						gc.LineIndex[item.Source] = map[int]string{target: "if ready {"}
					}
					report := GroundItemScoped(&item, gc)
					if report.Tier != tier || report.Status == types.GroundingUngrounded || report.OriginalLine != original || report.AdjustedLine != target {
						t.Fatalf("tier/relocation premise failed: %+v / %+v", report, item)
					}
					wantEnd := 0
					if explicitEnd {
						wantEnd = target
					}
					if item.Source != wantSource || item.LineStart != target || item.LineEnd != wantEnd {
						t.Fatalf("relocated interval = %s:%d-%d, want %s:%d-%d", item.Source, item.LineStart, item.LineEnd, wantSource, target, wantEnd)
					}
				})
			}
		}
	}
}

func TestGroundPointRelocationSymbolTableCanonicalization(t *testing.T) {
	const source = "service.go"
	gc := &Context{LineIndex: map[string]map[int]string{source: {
		10: "// Target owns the request.", 11: "// Details.", 12: "// More details.", 13: "//", 14: "// End of docs.", 15: "func Target() {}",
	}}, Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: {RelPath: source, Symbols: []repomap.Symbol{{Name: "Target", Kind: "function", Line: 10}}}}}}
	for _, end := range []int{0, 10} {
		item := types.EvidenceItem{Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: source,
			LineStart: 10, LineEnd: end, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Target"}
		report := GroundItemScoped(&item, gc)
		if report.Tier != types.TierSymbolTable || item.LineStart != 15 {
			t.Fatalf("symbol canonicalization premise: %+v / %+v", report, item)
		}
		wantEnd := 0
		if end != 0 {
			wantEnd = 15
		}
		if item.LineEnd != wantEnd {
			t.Fatalf("symbol point end = %d, want %d", item.LineEnd, wantEnd)
		}
	}
}

func TestGroundPointRelocationPreservesUnmovedAndUnknownCoordinates(t *testing.T) {
	for _, tc := range []struct {
		start, end    int
		hasDefinition bool
	}{
		{10, 10, true}, {10, 0, true}, {0, 0, true}, {10, 10, false}, {0, 0, false},
	} {
		item := types.EvidenceItem{Kind: types.EvidenceDirect, Source: "a.go", LineStart: tc.start, LineEnd: tc.end, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Target"}
		gc := &Context{Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{}}}
		if tc.hasDefinition {
			gc.Graph.FileIndex[item.Source] = &repomap.FileInfo{RelPath: item.Source, Symbols: []repomap.Symbol{{Name: "Target", Kind: "function", Line: 10}}}
		}
		GroundItemScoped(&item, gc)
		if item.LineEnd != tc.end {
			t.Fatalf("unmoved/unknown end changed: %+v -> %+v", tc, item)
		}
		if tc.hasDefinition && (item.GroundingStatus == types.GroundingUngrounded || item.LineStart != 10) {
			t.Fatalf("existing exact symbol location recovery was lost: %+v", item)
		}
		if !tc.hasDefinition && (item.GroundingStatus != types.GroundingUngrounded || item.LineStart != tc.start) {
			t.Fatalf("failed location was rewritten: %+v", item)
		}
	}
}

func TestGroundPointRelocationDoesNotMoveDeclaredRange(t *testing.T) {
	for _, target := range []int{19, 21, 24} {
		item := types.EvidenceItem{Kind: types.EvidenceRelationship, Scope: types.ScopeLineRange, Source: "a.go", LineStart: 20, LineEnd: 23,
			AnchorKind: types.AnchorCall, AnchorSymbol: "Target", Subject: "owner", Object: "Target"}
		gc := &Context{LineIndex: map[string]map[int]string{"a.go": {19: "", 20: "", 21: "", 22: "", 23: "", 24: ""}}}
		gc.LineIndex["a.go"][target] = "Target()"
		GroundItemScoped(&item, gc)
		if item.LineStart != 20 || item.LineEnd != 23 || item.Scope != types.ScopeLineRange {
			t.Fatalf("range moved to its witness: %+v", item)
		}
		if got := item.GroundingStatus != types.GroundingUngrounded; got != (target >= 20 && target <= 23) {
			t.Fatalf("outside witness changed range authority: target=%d item=%+v", target, item)
		}
	}
	gc := &Context{LineIndex: map[string]map[int]string{"stages.go": {10: "var order = []Stage{", 11: "Analyze,", 12: "Explore,", 13: "Finalize,", 14: "}"}}}
	for _, scope := range []types.EvidenceScope{"", types.ScopeLine, types.ScopeLineRange} {
		item := types.EvidenceItem{Kind: types.EvidenceRelationship, Scope: scope, Source: "stages.go", LineStart: 10, LineEnd: 14,
			AnchorKind: types.AnchorPrecedence, AnchorSymbol: "Analyze", Subject: "Analyze", Object: "Explore"}
		GroundItemScoped(&item, gc)
		wantStart := 11
		if scope == types.ScopeLineRange {
			wantStart = 10
		}
		if item.GroundingStatus != types.GroundingGrounded || item.LineStart != wantStart || item.LineEnd != 14 {
			t.Fatalf("bounded precedence changed: %+v", item)
		}
	}
	item := types.EvidenceItem{Scope: types.ScopeLineRange, Source: "a.go", LineStart: 10, LineEnd: 0}
	before := item
	GroundItemScoped(&item, &Context{})
	if item.GroundingStatus != types.GroundingUngrounded || !reflect.DeepEqual([]int{item.LineStart, item.LineEnd}, []int{before.LineStart, before.LineEnd}) {
		t.Fatalf("invalid range was repaired by guessing: %+v", item)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
