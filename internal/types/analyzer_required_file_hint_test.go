// Package types — analyzer_required_file_hint_test.go (2026-05-10).
//
// L3-T1 of the forced-read remediation: the new RequiredFileHint
// struct + AnalyzerHints.RequiredFileHints field. Pin the
// JSON-roundtrip shape and the threshold-band semantics the
// downstream explorer relies on.
package types

import (
	"encoding/json"
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
