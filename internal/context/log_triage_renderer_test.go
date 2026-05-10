// Package context — log_triage_renderer_test.go (2026-05-10).
//
// Fix-C of the analyzer-failure remediation: the new "## Primary
// Error Signal" section renders BEFORE Detected Patterns + Errors
// tree so the LLM anchors on the verbatim runtime error message
// rather than on potentially-unresolved frame file:line refs.
//
// Includes parallel-frame detector that surfaces "N goroutines /
// threads at same coordinate" as a corroborating concurrency
// signal independent of the BugClass first-match-only behaviour.
package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === renderPrimaryErrorSignal ===

func TestRenderPrimaryErrorSignal_NilBundle_Empty(t *testing.T) {
	if got := renderPrimaryErrorSignal(nil); got != "" {
		t.Errorf("nil bundle: expected empty; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_NoErrors_Empty(t *testing.T) {
	bundle := &types.LogBundle{}
	if got := renderPrimaryErrorSignal(bundle); got != "" {
		t.Errorf("empty errors: expected empty section; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_ConcurrentMapWrites_VerbatimMessage(t *testing.T) {
	// Mimic the logtri_goroutine_dump fixture.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{
				Type:    "fatal error",
				Message: "concurrent map writes",
			},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "## Primary Error Signal") {
		t.Errorf("missing section heading; got %q", got)
	}
	if !strings.Contains(got, "fatal error") {
		t.Errorf("missing error type; got %q", got)
	}
	if !strings.Contains(got, "concurrent map writes") {
		t.Errorf("missing verbatim message; got %q", got)
	}
}

func TestRenderPrimaryErrorSignal_PythonRuntimeError_Verbatim(t *testing.T) {
	// logtri_python style — Python traceback chain.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{
				Type:    "RuntimeError",
				Message: "config unavailable",
				Cause: &types.LogError{
					Type:    "KeyError",
					Message: "'database'",
				},
			},
		},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "RuntimeError") {
		t.Error("expected RuntimeError type")
	}
	if !strings.Contains(got, "config unavailable") {
		t.Error("expected verbatim RuntimeError message")
	}
	if !strings.Contains(got, "caused by") {
		t.Error("expected cause chain rendering")
	}
	if !strings.Contains(got, "KeyError") {
		t.Error("expected KeyError type from cause chain")
	}
}

func TestRenderPrimaryErrorSignal_OnlyMessage_NoType_StillRendered(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Message: "free-form runtime panic"}},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "free-form runtime panic") {
		t.Error("message-only error should still render")
	}
}

func TestRenderPrimaryErrorSignal_OnlyType_NoMessage_StillRendered(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "Segmentation fault"}},
	}
	got := renderPrimaryErrorSignal(bundle)
	if !strings.Contains(got, "Segmentation fault") {
		t.Error("type-only error should still render")
	}
}

// === detectParallelFrameCoordinates ===

func TestDetectParallelFrameCoordinates_None(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Frames: []types.LogFrame{{File: "a.go", Line: 10}, {File: "b.go", Line: 20}}},
		},
	}
	if got := detectParallelFrameCoordinates(bundle); got != nil {
		t.Errorf("no parallel coordinates: expected nil; got %v", got)
	}
}

func TestDetectParallelFrameCoordinates_GoroutineDumpStyle(t *testing.T) {
	// 3 goroutines at same line (concurrent map writes pattern).
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Frames: []types.LogFrame{{File: "internal/agent/analyzer.go", Line: 100}}},
			{Frames: []types.LogFrame{{File: "internal/agent/analyzer.go", Line: 100}}},
			{Frames: []types.LogFrame{{File: "internal/agent/analyzer.go", Line: 100}}},
		},
	}
	got := detectParallelFrameCoordinates(bundle)
	if len(got) != 1 {
		t.Fatalf("expected 1 parallel coordinate; got %v", got)
	}
	if got[0].file != "internal/agent/analyzer.go" || got[0].line != 100 || got[0].count != 3 {
		t.Errorf("unexpected hit: %+v", got[0])
	}
}

func TestDetectParallelFrameCoordinates_NestedCauseChain(t *testing.T) {
	// 2 frames at same coordinate distributed across nested cause chain.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{
				Frames: []types.LogFrame{{File: "x.go", Line: 5}},
				Cause: &types.LogError{
					Frames: []types.LogFrame{{File: "x.go", Line: 5}},
				},
			},
		},
	}
	got := detectParallelFrameCoordinates(bundle)
	if len(got) != 1 || got[0].count != 2 {
		t.Errorf("nested cause chain: expected 1 hit count=2; got %v", got)
	}
}

func TestDetectParallelFrameCoordinates_EmptyFileSkipped(t *testing.T) {
	// Frames with File="" (validator-cleared synthetic refs)
	// shouldn't co-locate by Raw text — skipped.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Frames: []types.LogFrame{{File: "", Line: 100}, {File: "", Line: 100}}},
		},
	}
	if got := detectParallelFrameCoordinates(bundle); got != nil {
		t.Errorf("empty File should be skipped; got %v", got)
	}
}

func TestDetectParallelFrameCoordinates_MultipleHitsSorted(t *testing.T) {
	// Two distinct coordinates with different counts. Higher count
	// should sort first.
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Frames: []types.LogFrame{
				{File: "low.go", Line: 1}, {File: "low.go", Line: 1},
				{File: "high.go", Line: 1}, {File: "high.go", Line: 1}, {File: "high.go", Line: 1},
			}},
		},
	}
	got := detectParallelFrameCoordinates(bundle)
	if len(got) != 2 {
		t.Fatalf("expected 2 hits; got %v", got)
	}
	if got[0].file != "high.go" || got[0].count != 3 {
		t.Errorf("highest count first; got [0]=%+v", got[0])
	}
	if got[1].file != "low.go" || got[1].count != 2 {
		t.Errorf("got [1]=%+v", got[1])
	}
}

// === sanitizeForInlineCode ===

func TestSanitizeForInlineCode_BackticksReplaced(t *testing.T) {
	got := sanitizeForInlineCode("error with `backtick` in message")
	if strings.Contains(got, "`") {
		t.Errorf("backticks not stripped: %q", got)
	}
}

func TestSanitizeForInlineCode_ControlCharsStripped(t *testing.T) {
	got := sanitizeForInlineCode("line1\nline2\tcol")
	if strings.Contains(got, "\n") || strings.Contains(got, "\t") {
		t.Errorf("control chars not flattened: %q", got)
	}
}

func TestSanitizeForInlineCode_LengthCapped(t *testing.T) {
	got := sanitizeForInlineCode(strings.Repeat("x", 250))
	if len(got) > 210 {
		t.Errorf("length not capped; got len=%d", len(got))
	}
}

// === Integration: formatLogTriageStructured wires the new section ===

func TestFormatLogTriageStructured_PrimarySignalSectionPresent(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{
			{Type: "fatal error", Message: "concurrent map writes"},
		},
	}
	got := formatLogTriageStructured(bundle)
	if !strings.Contains(got, "## Primary Error Signal") {
		t.Error("Primary Error Signal section missing from formatted output")
	}
	// Section must come BEFORE the Errors tree.
	primaryIdx := strings.Index(got, "## Primary Error Signal")
	errorsIdx := strings.Index(got, "### Error")
	if primaryIdx < 0 {
		t.Fatal("Primary Error Signal section index not found")
	}
	if errorsIdx >= 0 && primaryIdx > errorsIdx {
		t.Errorf("Primary Error Signal must appear BEFORE Error tree (primary=%d errors=%d)", primaryIdx, errorsIdx)
	}
}

func TestFormatLogTriageStructured_NoErrors_NoPrimarySection(t *testing.T) {
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Residue: types.LogResidue{UnknownChunks: []string{"ambiguous"}},
	}
	got := formatLogTriageStructured(bundle)
	if strings.Contains(got, "## Primary Error Signal") {
		t.Error("no errors but Primary Error Signal section emitted; should be skipped")
	}
}
