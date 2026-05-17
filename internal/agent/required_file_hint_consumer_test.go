// Package agent — required_file_hint_consumer_test.go (2026-05-10).
//
// L3-T3 of the forced-read remediation: explorer-side threshold-band
// consumption of analyzer-emitted RequiredFileHints. The two hooks:
//
//  1. effectivePrimaryFiles — high-confidence (≥0.8) hints join
//     the primary-files set alongside analyzer-derived primary +
//     emit-cited files.
//  2. preReadEligibleHintFiles / mergeHintFilesIntoPreRead — soft
//     (≥0.5) hints are eligible for the pre-read pool, with
//     high-confidence hints prioritised in the limited slot budget.
package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func makeEvaluatorWithHints(hints []types.RequiredFileHint) *explorerEvaluator {
	return &explorerEvaluator{requiredFileHints: hints}
}

type requiredFileHintAliasGater struct {
	aliases map[string]string
}

func (g requiredFileHintAliasGater) ResolveActiveSetPath(_ *types.BusContext, _, llmPath string, _ func(string) bool) types.ActiveSetGateResult {
	if dst, ok := g.aliases[llmPath]; ok {
		return types.ActiveSetGateResult{Allowed: true, ResolvedPath: dst, AutoPrefixed: true}
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (g requiredFileHintAliasGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

// === preReadEligibleHintFiles ===

func TestPreReadEligibleHintFiles_NilHints_NilOutput(t *testing.T) {
	e := makeEvaluatorWithHints(nil)
	if got := e.preReadEligibleHintFiles(); got != nil {
		t.Errorf("nil hints: expected nil, got %v", got)
	}
}

func TestPreReadEligibleHintFiles_AllBelowThreshold_NilOutput(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "a.go", Confidence: 0.4},
		{Path: "b.go", Confidence: 0.49},
	})
	if got := e.preReadEligibleHintFiles(); got != nil {
		t.Errorf("all below threshold: expected nil, got %v", got)
	}
}

func TestPreReadEligibleHintFiles_HighConfidenceFirst(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "soft.go", Confidence: 0.6},
		{Path: "high.go", Confidence: 0.95},
		{Path: "medium.go", Confidence: 0.7},
		{Path: "high2.go", Confidence: 0.85},
	})
	got := e.preReadEligibleHintFiles()
	want := []string{"high.go", "high2.go", "soft.go", "medium.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (high-confidence first, then soft)", got, want)
	}
}

func TestPreReadEligibleHintFiles_DropsBelowSoftThreshold(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "high.go", Confidence: 0.9},
		{Path: "drop.go", Confidence: 0.3},
		{Path: "soft.go", Confidence: 0.6},
	})
	got := e.preReadEligibleHintFiles()
	want := []string{"high.go", "soft.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPreReadEligibleHintFiles_CanonicalisesPaths(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "./internal/foo.go", Confidence: 0.9},
	})
	got := e.preReadEligibleHintFiles()
	if len(got) != 1 || got[0] != "internal/foo.go" {
		t.Errorf("./prefix should canonicalize; got %v", got)
	}
}

// === mergeHintFilesIntoPreRead ===

func TestMergeHintFilesIntoPreRead_NoHints_PassesThrough(t *testing.T) {
	e := makeEvaluatorWithHints(nil)
	caller := []string{"a.go", "b.go"}
	got := e.mergeHintFilesIntoPreRead(caller)
	if !reflect.DeepEqual(got, caller) {
		t.Errorf("got %v, want %v (no hints → passthrough)", got, caller)
	}
}

func TestMergeHintFilesIntoPreRead_HintsPrependedDeduped(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "high.go", Confidence: 0.9},
		{Path: "shared.go", Confidence: 0.6}, // also in caller
	})
	caller := []string{"shared.go", "caller-only.go"}
	got := e.mergeHintFilesIntoPreRead(caller)
	want := []string{"high.go", "shared.go", "caller-only.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (hints first, dedup against caller)", got, want)
	}
}

func TestMergeHintFilesIntoPreRead_PreservesCallerOrderAfterHints(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "h.go", Confidence: 0.9},
	})
	caller := []string{"c1.go", "c2.go", "c3.go"}
	got := e.mergeHintFilesIntoPreRead(caller)
	want := []string{"h.go", "c1.go", "c2.go", "c3.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// === effectivePrimaryFiles with high-confidence hints ===

func TestEffectivePrimaryFiles_HighConfidenceHintAddedToPrimary(t *testing.T) {
	// Set up evaluator with no analyzer-derived primary, no
	// emit-cited evidence, only a high-confidence hint.
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "internal/critical.go", Confidence: 0.95},
		{Path: "internal/soft.go", Confidence: 0.6},
	})
	// e.searchResult is nil → primaryEntityFiles returns nil.
	got := e.effectivePrimaryFiles()
	if len(got) != 1 || got[0] != "internal/critical.go" {
		t.Errorf("got %v, want [internal/critical.go] (only ≥0.8 hint joins)", got)
	}
}

func TestEffectivePrimaryFiles_LowConfidenceHintIgnored(t *testing.T) {
	e := makeEvaluatorWithHints([]types.RequiredFileHint{
		{Path: "internal/maybe.go", Confidence: 0.79}, // just below 0.8
	})
	got := e.effectivePrimaryFiles()
	if len(got) != 0 {
		t.Errorf("got %v, want empty (0.79 < 0.8 not eligible for primary)", got)
	}
}

func TestEffectivePrimaryFiles_HintDeduplicatedAgainstCited(t *testing.T) {
	e := &explorerEvaluator{
		requiredFileHints: []types.RequiredFileHint{
			{Path: "internal/foo.go", Confidence: 0.9},
		},
		structuredEvidence: []types.EvidenceItem{
			{Producer: "explorer.emit_evidence", Source: "internal/foo.go"},
		},
	}
	got := e.effectivePrimaryFiles()
	if len(got) != 1 {
		t.Errorf("got %v, want 1 entry (hint+cited dedup)", got)
	}
}

// === filterValidRequiredFileHints — graph-membership validation ===

func TestFilterValidRequiredFileHints_NilHints_NilOut(t *testing.T) {
	if got := filterValidRequiredFileHints(nil, nil); got != nil {
		t.Errorf("nil hints: expected nil; got %v", got)
	}
}

func TestFilterValidRequiredFileHints_NilGraph_FailOpen(t *testing.T) {
	hints := []types.RequiredFileHint{{Path: "a.go", Confidence: 0.9}}
	got := filterValidRequiredFileHints(hints, nil)
	if !reflect.DeepEqual(got, hints) {
		t.Errorf("nil graph should fail-open; got %v", got)
	}
}

func TestFilterValidRequiredFileHints_EmptyFileIndex_FailOpen(t *testing.T) {
	hints := []types.RequiredFileHint{{Path: "a.go", Confidence: 0.9}}
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{}}
	got := filterValidRequiredFileHints(hints, graph)
	if !reflect.DeepEqual(got, hints) {
		t.Errorf("empty FileIndex should fail-open; got %v", got)
	}
}

func TestFilterValidRequiredFileHints_DropsHallucinatedPath(t *testing.T) {
	hints := []types.RequiredFileHint{
		{Path: "internal/real.go", Confidence: 0.9, Rationale: "real"},
		{Path: "internal/fake.go", Confidence: 0.9, Rationale: "hallucinated"},
		{Path: "real.go", Confidence: 0.8, Rationale: "also real"},
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/real.go": {RelPath: "internal/real.go"},
			"real.go":          {RelPath: "real.go"},
		},
	}
	got := filterValidRequiredFileHints(hints, graph)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (fake.go dropped)", len(got))
	}
	for _, h := range got {
		if h.Path == "internal/fake.go" {
			t.Errorf("hallucinated path should be dropped; got %v", h)
		}
	}
}

func TestFilterValidRequiredFileHints_AllInvalid_ReturnsNil(t *testing.T) {
	hints := []types.RequiredFileHint{
		{Path: "fake1.go", Confidence: 0.9},
		{Path: "fake2.go", Confidence: 0.9},
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"real.go": {RelPath: "real.go"},
		},
	}
	got := filterValidRequiredFileHints(hints, graph)
	if got != nil {
		t.Errorf("all hallucinated → expected nil; got %v", got)
	}
}

func TestFilterValidRequiredFileHints_CanonicalisesBeforeLookup(t *testing.T) {
	// LLM emits "./internal/foo.go" but FileIndex stores
	// "internal/foo.go". Canonicalisation should match.
	hints := []types.RequiredFileHint{
		{Path: "./internal/foo.go", Confidence: 0.9},
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/foo.go": {RelPath: "internal/foo.go"},
		},
	}
	got := filterValidRequiredFileHints(hints, graph)
	if len(got) != 1 {
		t.Errorf("canonical path should match; got %v", got)
	}
}

func TestFilterValidRequiredFileHints_MultiRepoAliasSurvivesPrimaryGraphMiss(t *testing.T) {
	root := t.TempDir()
	prefixed := filepath.Join(root, "opencode", "packages", "opencode", "src", "tool")
	if err := os.MkdirAll(prefixed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefixed, "read.ts"), []byte("export const ReadTool = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hints := []types.RequiredFileHint{
		{Path: "packages/opencode/src/tool/read.ts", Confidence: 0.9},
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"internal/primary.go": {RelPath: "internal/primary.go"},
		},
	}
	ctx := &types.AgentContext{
		RepoRoot: root,
		MultiGraph: requiredFileHintAliasGater{aliases: map[string]string{
			"packages/opencode/src/tool/read.ts": "opencode/packages/opencode/src/tool/read.ts",
		}},
	}

	got := filterValidRequiredFileHints(hints, graph, ctx)
	if len(got) != 1 {
		t.Fatalf("active-set alias should survive primary graph miss, got %+v", got)
	}
	if got[0].Path != "opencode/packages/opencode/src/tool/read.ts" {
		t.Fatalf("hint path should be normalized into active-set namespace, got %q", got[0].Path)
	}
}

func TestFilterValidRequiredFileHints_EmptyPathDropped(t *testing.T) {
	hints := []types.RequiredFileHint{
		{Path: "", Confidence: 0.9},
		{Path: "real.go", Confidence: 0.9},
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomaptypes.FileInfo{
			"real.go": {RelPath: "real.go"},
		},
	}
	got := filterValidRequiredFileHints(hints, graph)
	if len(got) != 1 || got[0].Path != "real.go" {
		t.Errorf("empty path should be dropped; got %v", got)
	}
}
