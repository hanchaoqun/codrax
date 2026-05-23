// Package tool — emit_analysis_irrelevant_files_test.go (2026-05-10).
//
// L4 of the forced-read remediation: validateAndBuildIrrelevantFiles
// canonicalises and caps the LLM-emitted irrelevant_files array.
package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidateAndBuildIrrelevantFiles_NilInput_NilOutput(t *testing.T) {
	if got := validateAndBuildIrrelevantFiles(nil, nil); got != nil {
		t.Errorf("nil: expected nil; got %v", got)
	}
}

func TestValidateAndBuildIrrelevantFiles_HappyPath(t *testing.T) {
	in := []string{"a.go", "b.go", "c.go"}
	got := validateAndBuildIrrelevantFiles(in, nil)
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

func TestValidateAndBuildIrrelevantFiles_DropsEmpty(t *testing.T) {
	val := &analysisValidationResult{}
	in := []string{"", "  ", "a.go"}
	got := validateAndBuildIrrelevantFiles(in, val)
	if len(got) != 1 || got[0] != "a.go" {
		t.Errorf("empty/whitespace should drop; got %v", got)
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildIrrelevantFiles_DropsDuplicates(t *testing.T) {
	val := &analysisValidationResult{}
	in := []string{"a.go", "./a.go", `a.go`, "b.go"}
	got := validateAndBuildIrrelevantFiles(in, val)
	// "a.go" / "./a.go" canonicalise to the same string → dedup.
	if len(got) != 2 {
		t.Errorf("dedup should leave 2 entries; got %v", got)
	}
}

func TestValidateAndBuildIrrelevantFiles_HardCapEnforced(t *testing.T) {
	val := &analysisValidationResult{}
	in := make([]string, irrelevantFilesMax+5)
	for i := range in {
		in[i] = "f" + string(rune('a'+i)) + ".go"
	}
	got := validateAndBuildIrrelevantFiles(in, val)
	if len(got) != irrelevantFilesMax {
		t.Errorf("hard cap = %d, got %d", irrelevantFilesMax, len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning for over-cap; got %v", val.Warnings)
	}
}

func TestValidateAndBuildIrrelevantFiles_CanonicalisesPaths(t *testing.T) {
	in := []string{`internal\foo.go`, "./internal/bar.go"}
	got := validateAndBuildIrrelevantFiles(in, nil)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0] != "internal/foo.go" {
		t.Errorf("backslash → slash; got %q", got[0])
	}
	if got[1] != "internal/bar.go" {
		t.Errorf("./prefix stripped; got %q", got[1])
	}
}

func TestEmitAnalysisExecute_RepairsIrrelevantFilesObjectEntries(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("explain the analyzer")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["analyzer", "irrelevant", "files"],
		"entities": ["analyzer"],
		"question_kind": "mechanism",
		"irrelevant_files": [
			{"path": "./internal/noise.go", "confidence": 0.2, "rationale": "off-topic"},
			{"file": "internal/no-path.go", "confidence": 0.1},
			"internal/other.go"
		]
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("object-shaped irrelevant_files should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	got := rm.AnalyzerHints.IrrelevantFiles
	if len(got) != 2 || got[0] != "internal/noise.go" || got[1] != "internal/other.go" {
		t.Fatalf("irrelevant files = %+v, want repaired paths", got)
	}
	if !strings.Contains(res.Summary, "repaired 1 object entries") {
		t.Fatalf("summary should disclose object-entry repair, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped 1 malformed optional entries") {
		t.Fatalf("summary should disclose dropped malformed entry, got %q", res.Summary)
	}
}

// === schema smoke ===

func TestEmitAnalysisSchema_IrrelevantFilesPresent(t *testing.T) {
	emitAnalysisSchemaOnce.Do(buildEmitAnalysisSchema)
	if !strings.Contains(string(emitAnalysisSchemaCache), `"irrelevant_files"`) {
		t.Error("emit_analysis schema should declare irrelevant_files property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `OFF-TOPIC`) {
		t.Error("irrelevant_files description should mention OFF-TOPIC")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `plain strings only`) {
		t.Error("irrelevant_files description should explicitly require string entries")
	}
}
