// Package types — analyzer_required_file_hint_test.go (2026-05-10).
//
// L3-T1 of the forced-read remediation: the new RequiredFileHint
// struct + AnalyzerHints.RequiredFileHints field. Pin the
// JSON-roundtrip shape and the threshold-band semantics the
// downstream explorer relies on.
package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRequiredFileHint_JSONRoundtrip(t *testing.T) {
	in := RequiredFileHint{
		Path:       "internal/analysis/gate/gate.go",
		Confidence: 0.95,
		Rationale:  "directly implements the gate.Run with 9 sequential check calls",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out RequiredFileHint
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("roundtrip: got %+v, want %+v", out, in)
	}
}

func TestQualifyForcedReadSeedPath_SingleRepoRequiresExistingRegularFile(t *testing.T) {
	root := t.TempDir()
	real := "internal/agent/explorer.go"
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(real)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, real), []byte("package agent\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/agent/dir_only"), 0o755); err != nil {
		t.Fatalf("mkdir dir_only: %v", err)
	}

	ctx := &BusContext{RepoRoot: root}
	got, ok := QualifyForcedReadSeedPath(ctx, "./"+real)
	if !ok || got != real {
		t.Fatalf("existing forced-read seed = (%q,%t), want (%q,true)", got, ok, real)
	}
	if got, ok := QualifyForcedReadSeedPath(ctx, "history/old/removed_file.go"); ok || got != "" {
		t.Fatalf("missing forced-read seed should be rejected, got (%q,%t)", got, ok)
	}
	if got, ok := QualifyForcedReadSeedPath(ctx, "internal/agent/dir_only"); ok || got != "" {
		t.Fatalf("directory forced-read seed should be rejected, got (%q,%t)", got, ok)
	}
	if got, ok := QualifyForcedReadSeedPath(ctx, "../outside.go"); ok || got != "" {
		t.Fatalf("parent-traversal forced-read seed should be rejected, got (%q,%t)", got, ok)
	}
	external := filepath.Join(t.TempDir(), "external.go")
	if err := os.WriteFile(external, []byte("package external\n"), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}
	if got, ok := QualifyForcedReadSeedPath(ctx, external); ok || got != "" {
		t.Fatalf("repo-external absolute forced-read seed should be rejected, got (%q,%t)", got, ok)
	}
	absoluteInside := filepath.Join(root, filepath.FromSlash(real))
	if got, ok := QualifyForcedReadSeedPath(ctx, absoluteInside); !ok || got != real {
		t.Fatalf("repo-internal absolute forced-read seed = (%q,%t), want (%q,true)", got, ok, real)
	}
}

func TestRequiredFileHint_OmitemptyRationale(t *testing.T) {
	// Rationale has omitempty; empty string should not appear in JSON.
	in := RequiredFileHint{Path: "a.go", Confidence: 0.4}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(b); got != `{"path":"a.go","confidence":0.4}` {
		t.Errorf("omitempty rationale: got %s", got)
	}
}

func TestAnalyzerHints_RequiredFileHints_OptionalEmpty(t *testing.T) {
	// Empty slice → omitted from JSON.
	in := AnalyzerHints{Keywords: []string{"foo"}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(b); got != `{"keywords":["foo"]}` {
		t.Errorf("empty RequiredFileHints should omit; got %s", got)
	}
}

func TestAnalyzerHints_RequiredFileHints_Populated(t *testing.T) {
	in := AnalyzerHints{
		Keywords: []string{"foo"},
		RequiredFileHints: []RequiredFileHint{
			{Path: "a.go", Confidence: 0.9, Rationale: "primary"},
			{Path: "b.go", Confidence: 0.6, Rationale: "secondary"},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out AnalyzerHints
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.RequiredFileHints) != 2 {
		t.Fatalf("RequiredFileHints len = %d, want 2", len(out.RequiredFileHints))
	}
	if out.RequiredFileHints[0].Path != "a.go" || out.RequiredFileHints[0].Confidence != 0.9 {
		t.Errorf("hint 0 mismatch: %+v", out.RequiredFileHints[0])
	}
}

func TestRequiredFileHintCoverage_SourceInventoryUsesInventoryCap(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqEnumeration),
			RequiredFileHints: []RequiredFileHint{{
				Path:       "src/a.ets",
				Confidence: 0.9,
			}},
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
	}

	if !SourceInventoryRequiredFileCoverageShape(rm) {
		t.Fatal("source inventory request should have required-file coverage shape")
	}
	if !RequiredFileHintSourceInventoryCoverageApplies(rm) {
		t.Fatal("source inventory required-file hints should be hard coverage obligations")
	}
	if got := RequiredFileHintCoverageMaxForRequest(rm); got != SourceInventoryRequiredFileHintCoverageMax {
		t.Fatalf("coverage cap=%d, want source inventory cap %d", got, SourceInventoryRequiredFileHintCoverageMax)
	}
}

func TestRequiredFileHintCoverage_MixedRuntimeCurrentSourceUsesTightCap(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		PerfTrace: &PerfBundle{
			Frames: []PerfFrame{{FrameNo: 1, DurationMs: 86.1, Janky: true}},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码解释"},
			Confidence:                          0.9,
		},
		AnalyzerHints: AnalyzerHints{RequiredFileHints: []RequiredFileHint{
			{Path: "internal/tracequery/parse.go", Confidence: 0.9},
			{Path: "internal/hitraceconv/types.go", Confidence: 0.9},
			{Path: "internal/agent/perf_triager.go", Confidence: 0.9},
		}},
	}

	if !RequiredFileHintCurrentSourceCoverageApplies(rm) {
		t.Fatal("mixed runtime/current-source required-file hints should remain source obligations")
	}
	if !MixedRuntimeCurrentSourceRequiredFileCoverageShape(rm) {
		t.Fatal("mixed runtime/current-source request should use the mixed coverage shape")
	}
	if got := RequiredFileHintCoverageMaxForRequest(rm); got != MixedRuntimeCurrentSourceRequiredFileHintCoverageMax {
		t.Fatalf("coverage cap=%d, want mixed cap %d", got, MixedRuntimeCurrentSourceRequiredFileHintCoverageMax)
	}
}

func TestRequiredFileHintCoverage_ObservationOnlyRuntimeSkipsInventoryShape(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqEnumeration),
			RequiredFileHints: []RequiredFileHint{{
				Path:       "src/a.go",
				Confidence: 0.9,
			}},
		},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
		LogTriage: &LogBundle{
			Observations: []LogObservation{{Kind: "timeout", Subject: "external runtime"}},
		},
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode: ExternalObservationCurrentSourceExclude,
			ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"do not inspect source"},
		},
	}

	if SourceInventoryRequiredFileCoverageShape(rm) {
		t.Fatal("observation-only runtime artifact must not activate source inventory required-file coverage")
	}
	if RequiredFileHintCurrentSourceCoverageApplies(rm) {
		t.Fatal("observation-only runtime artifact must not force current-source required reads")
	}
}
