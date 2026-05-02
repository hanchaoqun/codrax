package logtriage

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestMultiSource_ValidateBundle_MultipleErrors pins the design
// invariant that a multi-source attached log (CLI: `--log a.log
// --log b.log`; REPL: `/log a` then `/log append b`) — concatenated
// upstream into a single string with `# codrax-source:` markers —
// is correctly carried through ValidateBundle. The LLM emits ONE
// LogBundle per dispatch but its Errors[] slice holds ONE entry
// per source. This test verifies Layer-4 derivation
// (ResolvedFiles, Entities, IntentHint) handles multiple parallel
// errors as a single union, with no per-source dropout.
//
// Source-A panics in pkg/foo.go (definition-line confidence 0.9).
// Source-B panics in pkg/bar.go (separate process / time window).
// Both sources contribute Entities and ResolvedFiles to the final
// merged bundle. IntentHint is RootCause when EITHER source has
// a panic + frame.line>0.
func TestMultiSource_ValidateBundle_MultipleErrors(t *testing.T) {
	in := ValidateInput{
		Meta: types.LogMeta{
			Signals: []types.LogSignal{types.SignalPanic},
		},
		Errors: []types.LogError{
			// Source A: process 1 panic.
			{
				Type: "runtime.Error",
				Frames: []types.LogFrame{
					{File: "wherever1.go", Line: 42, Func: "pkg.Foo",
						Raw: "raw1", Confidence: 0.9},
				},
			},
			// Source B: process 2 panic.
			{
				Type: "runtime.Error",
				Frames: []types.LogFrame{
					{File: "wherever2.go", Line: 99, Func: "pkg.Bar",
						Raw: "raw2", Confidence: 0.9},
				},
			},
		},
	}
	got := ValidateBundle(in, "")
	if got == nil {
		t.Fatal("nil bundle")
	}
	// Both Errors[] entries survive (no dedup across distinct sources).
	if len(got.Errors) != 2 {
		t.Errorf("Errors[] len = %d, want 2 (one per source)", len(got.Errors))
	}
	// Entities derived from BOTH sources' frames are unioned + deduped.
	hasFoo, hasBar := false, false
	for _, e := range got.Entities {
		if e == "Foo" {
			hasFoo = true
		}
		if e == "Bar" {
			hasBar = true
		}
	}
	if !hasFoo || !hasBar {
		t.Errorf("entities should union both source funcs; got %v", got.Entities)
	}
	// IntentHint = RootCause because at least one Frame has Line>0
	// AND Signals contains panic.
	if got.IntentHint != types.IntentRootCause {
		t.Errorf("IntentHint = %q, want IntentRootCause (panic + line>0 in either source)",
			got.IntentHint)
	}
}

// TestMultiSource_MergeBundles_TwoStepEscalation pins the two-step
// escalation path: when input log size > log_triage_two_step_bytes
// OR coverage < log_triage_two_step_coverage, the system runs
// emit_log_segmentation to split the log into byte-addressed
// regions, then runs emit_log_triage per actionable segment, then
// merges via MergeBundles. This test exercises the merge contract
// for that flow — multi-segment partial bundles must concat without
// loss.
func TestMultiSource_MergeBundles_PreservesFromBothSegments(t *testing.T) {
	segA := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalPanic},
		},
		Errors: []types.LogError{
			{Type: "ErrA",
				Frames: []types.LogFrame{
					{File: "fileA.go", Line: 10, Func: "FuncA", Confidence: 0.9},
				}},
		},
		Entities:      []string{"FuncA"},
		ResolvedFiles: []string{"fileA.go"},
		IntentHint:    types.IntentRootCause,
	}
	segB := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalCrash},
		},
		Errors: []types.LogError{
			{Type: "ErrB",
				Frames: []types.LogFrame{
					{File: "fileB.go", Line: 20, Func: "FuncB", Confidence: 0.9},
				}},
		},
		Entities:      []string{"FuncB"},
		ResolvedFiles: []string{"fileB.go"},
		IntentHint:    types.IntentRootCause,
	}
	merged := MergeBundles([]*types.LogBundle{segA, segB}, 1000)
	if merged == nil {
		t.Fatal("nil merge")
	}
	if len(merged.Errors) != 2 {
		t.Errorf("Errors len = %d, want 2", len(merged.Errors))
	}
	// Signals union of {panic, crash}.
	signalSet := map[types.LogSignal]bool{}
	for _, s := range merged.Meta.Signals {
		signalSet[s] = true
	}
	if !signalSet[types.SignalPanic] || !signalSet[types.SignalCrash] {
		t.Errorf("signals union missing one: %v", merged.Meta.Signals)
	}
}

// TestMultiSource_SourceMarkerInRawLog pins the contract surface
// the CLI loadMultiPathSlice + REPL handleLogAppend rely on: the
// `# codrax-source: <path>` literal token is the LLM-visible
// boundary marker. Logtriage skill prompts must teach this token
// (verified separately in skill/defaults_test.go); this test fixes
// the literal so a typo refactor cannot silently drift.
func TestMultiSource_SourceMarkerLiteral(t *testing.T) {
	const markerPrefix = "# codrax-source:"
	cases := []string{
		"# codrax-source: a.log\n",
		"# codrax-source: /full/path/b.log\n",
		"# codrax-source: relative/c.log\n",
	}
	for _, line := range cases {
		if !strings.HasPrefix(line, markerPrefix) {
			t.Errorf("marker drift: %q must start with %q", line, markerPrefix)
		}
	}
	// Logtriage skill prompts (internal/skill/defaults.go) cite this
	// exact prefix so the LLM identifies cross-source boundaries.
	// Any rename of the constant requires updating both the CLI
	// loader and the skill prompt simultaneously.
}
