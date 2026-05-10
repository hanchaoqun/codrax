// Package tool — emit_analysis_required_files_test.go (2026-05-10).
//
// L3-T2 of the forced-read remediation: validateAndBuildRequiredFileHints
// is the per-entry validator + canonicaliser for the LLM-emitted
// `required_files` array. Pin the exact behaviour the IR consumer
// (explorer) relies on.
package tool

import (
	"math"
	"strings"
	"testing"
)

func TestValidateAndBuildRequiredFileHints_NilInput_NilOutput(t *testing.T) {
	got := validateAndBuildRequiredFileHints(nil, nil)
	if got != nil {
		t.Errorf("nil input should return nil; got %v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_EmptyInput_NilOutput(t *testing.T) {
	got := validateAndBuildRequiredFileHints([]emitRequiredFileParam{}, nil)
	if got != nil {
		t.Errorf("empty input should return nil; got %v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_HappyPath(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: "internal/foo.go", Confidence: 0.9, Rationale: "primary"},
		{Path: "internal/bar.go", Confidence: 0.6, Rationale: "secondary"},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 2 {
		t.Fatalf("got %d hints, want 2", len(got))
	}
	if got[0].Path != "internal/foo.go" || got[0].Confidence != 0.9 || got[0].Rationale != "primary" {
		t.Errorf("hint[0] = %+v", got[0])
	}
}

func TestValidateAndBuildRequiredFileHints_WindowsBackslashCanonicalised(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: `internal\foo.go`, Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 || got[0].Path != "internal/foo.go" {
		t.Errorf("backslash should be canonicalised to slash; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_DotSlashPrefixStripped(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: "./internal/foo.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 || got[0].Path != "internal/foo.go" {
		t.Errorf("./prefix should be stripped; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_EmptyPathDropped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "", Confidence: 0.9},
		{Path: "  ", Confidence: 0.9},
		{Path: "internal/foo.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 1 {
		t.Errorf("empty/whitespace paths should be dropped; got %d hints", len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected a dropped-warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_NaNConfidenceDropped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: math.NaN()},
		{Path: "b.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 1 || got[0].Path != "b.go" {
		t.Errorf("NaN confidence should drop entry; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_OutOfRangeClamped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: -0.5},
		{Path: "b.go", Confidence: 1.5},
		{Path: "c.go", Confidence: 0.5},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 3 {
		t.Fatalf("clamping should keep all 3 entries; got %d", len(got))
	}
	if got[0].Confidence != 0 {
		t.Errorf("negative confidence should clamp to 0; got %v", got[0].Confidence)
	}
	if got[1].Confidence != 1 {
		t.Errorf("over-1 confidence should clamp to 1; got %v", got[1].Confidence)
	}
	if !containsAny(val.Warnings, "clamped") {
		t.Errorf("expected a clamping warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_RationaleTruncated(t *testing.T) {
	long := strings.Repeat("x", requiredFileHintRationaleMaxChars+50)
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: 0.9, Rationale: long},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 {
		t.Fatalf("got %d hints", len(got))
	}
	if !strings.HasSuffix(got[0].Rationale, "…") {
		t.Errorf("over-cap rationale should end in …; got %q", got[0].Rationale)
	}
	if len([]rune(got[0].Rationale)) > requiredFileHintRationaleMaxChars {
		t.Errorf("rationale runes = %d, want ≤ %d", len([]rune(got[0].Rationale)), requiredFileHintRationaleMaxChars)
	}
}

func TestValidateAndBuildRequiredFileHints_HardCapEnforced(t *testing.T) {
	val := &analysisValidationResult{}
	in := make([]emitRequiredFileParam, requiredFileHintsMax+5)
	for i := range in {
		in[i] = emitRequiredFileParam{Path: "f" + string(rune('a'+i)) + ".go", Confidence: 0.9}
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != requiredFileHintsMax {
		t.Errorf("hard cap = %d, got %d", requiredFileHintsMax, len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning for over-cap entries; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_LowConfidenceKept(t *testing.T) {
	// Threshold gating happens at consumer (explorer), not here.
	// Even confidence=0 entries pass through to preserve "I'm unsure"
	// signal. The consumer drops them at <0.5.
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: 0.1, Rationale: "low"},
		{Path: "b.go", Confidence: 0.0},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 2 {
		t.Errorf("low-confidence entries should pass through; got %d", len(got))
	}
}

// helper
func containsAny(s []string, sub string) bool {
	for _, x := range s {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// === skill prompt smoke test ===

func TestEmitAnalysisSchema_RequiredFilesPresent(t *testing.T) {
	emitAnalysisSchemaOnce.Do(buildEmitAnalysisSchema)
	if !strings.Contains(string(emitAnalysisSchemaCache), `"required_files"`) {
		t.Error("emit_analysis schema should declare required_files property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"confidence"`) {
		t.Error("required_files items should declare confidence property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"rationale"`) {
		t.Error("required_files items should declare rationale property")
	}
	// Threshold bands documented in description (LLM-facing).
	if !strings.Contains(string(emitAnalysisSchemaCache), `0.8`) {
		t.Error("required_files description should mention 0.8 threshold band")
	}
}
