// Package context — log_triage_frame_drift_test.go (2026-05-10).
//
// Step 2 of the model-confidence-respected architecture: when an
// attached log carries frames whose paths resolve to current-repo
// files but the function name does not match what current code has
// there, the renderer surfaces a "Frame ↔ current-code drift
// warning" section so the model knows to treat those frames as
// observation-only rather than reading current content to explain
// the log.
//
// The canonical scenario: a synthetic Go panic dumps frames like
// `internal/agent/analyzer.go:100` for a function
// `main.writeSession` that does not exist in current code. The
// self-referential trap previously led the model to read the real
// file at that line and confabulate a story unrelated to the actual
// panic class ("concurrent map writes" → MultiGraph nil deref).
package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubLocator is a hand-rolled SymbolLocator for unit tests. Returns
// whatever pre-loaded SymbolLocations are mapped to a query name
// (LocateSymbol) or file (SymbolsInFile).
type stubLocator struct {
	byName map[string][]types.SymbolLocation
	byFile map[string][]types.SymbolLocation
}

func (l *stubLocator) LocateSymbol(name string) []types.SymbolLocation {
	return l.byName[name]
}

func (l *stubLocator) SymbolsInFile(file string) []types.SymbolLocation {
	return l.byFile[file]
}

// stubBundle returns a LogBundle with a single error whose frame
// uses the supplied file/line/func. Sufficient for drift testing.
func stubBundle(file string, line int, fn string) *types.LogBundle {
	return &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{
			{
				Type:    "runtime.error",
				Message: "synthetic panic",
				Frames: []types.LogFrame{
					{File: file, Line: line, Func: fn, Raw: "synthetic", Confidence: 1.0},
				},
			},
		},
	}
}

func TestRenderFrameDrift_NilLocatorReturnsEmpty(t *testing.T) {
	// Single-shot CLI flows pass nil locator. Drift section must
	// degrade gracefully — pre-Step-2 behaviour preserved.
	got := renderLogTriageFrameDrift(stubBundle("a.go", 10, "Foo"), nil)
	if got != "" {
		t.Errorf("nil locator must skip drift section; got %q", got)
	}
}

func TestRenderFrameDrift_NoFramesReturnsEmpty(t *testing.T) {
	loc := &stubLocator{}
	bundle := &types.LogBundle{Meta: types.LogMeta{Lang: "go"}}
	if got := renderLogTriageFrameDrift(bundle, loc); got != "" {
		t.Errorf("empty bundle must skip drift section; got %q", got)
	}
}

func TestRenderFrameDrift_NoneStatusSuppressed(t *testing.T) {
	// Frame agrees with current code (file + function match, line
	// inside def range). No drift → no section, no prompt waste.
	loc := &stubLocator{
		byName: map[string][]types.SymbolLocation{
			"Foo": {{File: "a.go", Line: 5, EndLine: 50}},
		},
		byFile: map[string][]types.SymbolLocation{
			"a.go": {{File: "a.go", Line: 5, EndLine: 50}},
		},
	}
	got := renderLogTriageFrameDrift(stubBundle("a.go", 30, "Foo"), loc)
	if got != "" {
		t.Errorf("DriftStatusNone must suppress section; got %q", got)
	}
}

func TestRenderFrameDrift_UnmappableFires(t *testing.T) {
	// The canonical self-referential trap: log frame's path is in
	// the repo but the symbol does not exist there at all.
	// internal/agent/analyzer.go is real; main.writeSession is not
	// — drift_detect will return Unmappable (or close), and the
	// section must warn the model.
	loc := &stubLocator{
		byName: map[string][]types.SymbolLocation{}, // writeSession nowhere
		byFile: map[string][]types.SymbolLocation{}, // analyzer.go un-indexed (Unmappable path)
	}
	got := renderLogTriageFrameDrift(stubBundle("internal/agent/analyzer.go", 100, "writeSession"), loc)
	if got == "" {
		t.Fatal("unmappable frame must trigger drift warning")
	}
	if !strings.Contains(got, "Frame ↔ current-code drift warning") {
		t.Errorf("section heading missing: %q", got)
	}
	if !strings.Contains(got, "Unmappable") {
		t.Errorf("Unmappable status not surfaced: %q", got)
	}
	if !strings.Contains(got, "internal/agent/analyzer.go:100") {
		t.Errorf("frame location not cited: %q", got)
	}
	if !strings.Contains(got, "OPAQUE OBSERVATION") {
		t.Errorf("opaque-observation directive missing: %q", got)
	}
}

func TestRenderFrameDrift_TailRenameFires(t *testing.T) {
	// File exists, has SOME symbols, but query function isn't one
	// of them — function was renamed. drift_detect_test.go's case
	// 3 covers this; here we verify the renderer surfaces it.
	loc := &stubLocator{
		byName: map[string][]types.SymbolLocation{},
		byFile: map[string][]types.SymbolLocation{
			"a.go": {{File: "a.go", Line: 1, EndLine: 50}}, // some symbol
		},
	}
	got := renderLogTriageFrameDrift(stubBundle("a.go", 10, "OldName"), loc)
	if got == "" {
		t.Fatal("tail-rename must trigger drift warning")
	}
	if !strings.Contains(got, "renamed") {
		t.Errorf("tail-rename guidance missing: %q", got)
	}
}

func TestRenderFrameDrift_BucketingByStatus(t *testing.T) {
	// Multiple frames hit different drift statuses — the section
	// must aggregate by status (one bucket per class) rather than
	// listing each frame independently.
	loc := &stubLocator{
		byName: map[string][]types.SymbolLocation{},
		byFile: map[string][]types.SymbolLocation{},
	}
	bundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go"},
		Errors: []types.LogError{
			{
				Type: "panic",
				Frames: []types.LogFrame{
					{File: "a.go", Line: 10, Func: "FunA", Raw: "x", Confidence: 1.0},
					{File: "b.go", Line: 20, Func: "FunB", Raw: "y", Confidence: 1.0},
					{File: "c.go", Line: 30, Func: "FunC", Raw: "z", Confidence: 1.0},
				},
			},
		},
	}
	got := renderLogTriageFrameDrift(bundle, loc)
	if got == "" {
		t.Fatal("expected drift section with 3 unmappable frames")
	}
	for _, frame := range []string{"a.go:10", "b.go:20", "c.go:30"} {
		if !strings.Contains(got, frame) {
			t.Errorf("frame %s missing from section: %q", frame, got)
		}
	}
}

func TestRenderFrameDrift_R6Audit(t *testing.T) {
	// The section is LLM-facing. Must not leak internal pipeline
	// jargon.
	loc := &stubLocator{byFile: map[string][]types.SymbolLocation{}}
	got := renderLogTriageFrameDrift(stubBundle("a.go", 10, "Foo"), loc)
	if got == "" {
		t.Skip("drift section did not render in this scenario")
	}
	for _, banned := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"BusContext", "MutableState", "AgentContext",
		"SymbolLocator", "FrameDrift", "MultiGraph",
		"L1", "L2", "L3", "L4",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("internal term %q leaked into LLM-facing drift section: %q", banned, got)
		}
	}
}

func TestDriftStatusHumanLabel_R6Audit(t *testing.T) {
	for _, s := range []types.FrameDriftStatus{
		types.DriftStatusLineDrift,
		types.DriftStatusTailRename,
		types.DriftStatusFileMoved,
		types.DriftStatusUnmappable,
	} {
		label := driftStatusHumanLabel(s)
		if label == "" {
			t.Errorf("status %q missing label", s)
		}
		for _, banned := range []string{
			"DriftStatus", "explorer", "extractor", "finalizer",
		} {
			if strings.Contains(label, banned) {
				t.Errorf("label for %q leaks internal term %q: %q", s, banned, label)
			}
		}
	}
}
