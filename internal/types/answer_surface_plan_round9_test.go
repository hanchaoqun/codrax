package types

import (
	"strings"
	"testing"
)

// TestPreferredAnswerSummarySurfaceMode_GatesOnUserIntent: round-9
// user red line — drift surface mode fires when the LLM-classified
// user intent (rm.Intent) is RootCause OR Trace. Other intents
// stay out of drift surface mode even when drift anchors exist.
// Reuses the existing Intent enum (single source of truth).
func TestPreferredAnswerSummarySurfaceMode_GatesOnUserIntent(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RequiredShape:         ShapeExplanation,
		LogSourceDriftAnchors: []LogSourceDriftAnchor{{File: "x.go", AnchoredLine: 100}},
	}
	cases := []struct {
		intent Intent
		want   AnswerSummarySurfaceMode
	}{
		{IntentRootCause, AnswerSummarySurfaceDriftBoundedRootCause},
		{IntentTrace, AnswerSummarySurfaceDriftBoundedRootCause},
		{IntentExplain, AnswerSummarySurfaceDefault},
		{IntentEnumerate, AnswerSummarySurfaceDefault},
		{IntentConfigQuery, AnswerSummarySurfaceDefault},
		{IntentReturnValue, AnswerSummarySurfaceDefault},
		{IntentUnknown, AnswerSummarySurfaceDefault},
	}
	for _, tc := range cases {
		got := preferredAnswerSummarySurfaceMode(plan, RequestModel{Intent: tc.intent})
		if got != tc.want {
			t.Errorf("Intent=%q: got %q; want %q", tc.intent, got, tc.want)
		}
	}
}

// TestPreferredAnswerSummarySurfaceMode_ScenarioNoLongerHardCap:
// pre-round-9 the gate read rm.Scenario; now it reads rm.Intent.
// A non-RootCause Scenario should NOT block drift mode when the
// LLM explicitly classified user intent as RootCause.
func TestPreferredAnswerSummarySurfaceMode_ScenarioNoLongerHardCap(t *testing.T) {
	plan := &AnswerSurfacePlan{
		RequiredShape:         ShapeExplanation,
		LogSourceDriftAnchors: []LogSourceDriftAnchor{{File: "x.go", AnchoredLine: 100}},
	}
	rm := RequestModel{
		Intent:   IntentRootCause,
		Scenario: ScenarioArchitectureExplain, // anything OTHER than ScenarioRootCause
	}
	got := preferredAnswerSummarySurfaceMode(plan, rm)
	if got != AnswerSummarySurfaceDriftBoundedRootCause {
		t.Errorf("user intent RootCause should drive drift mode regardless of Scenario; got %q", got)
	}
}

// TestCollectAllLogFrames_PreservesSymbolOnlyFrames: round-9 user
// red line — frames without file/line MUST NOT be silently dropped.
// They still have Func and may carry the symbol identity the user's
// question is asking about.
func TestCollectAllLogFrames_PreservesSymbolOnlyFrames(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{Frames: []LogFrame{
			{File: "x.go", Line: 50, Func: "Resolved", Raw: "x.go:50"},
			{File: "", Line: 0, Func: "Unresolved", Raw: "Unresolved (basename failed)"},
			{File: "", Line: 0, Func: "<init>", Raw: "constructor frame"},
		}}},
	}
	frames := collectAllLogFrames(bundle)
	if len(frames) != 3 {
		t.Fatalf("got %d frames; want 3 (1 resolved + 2 symbol-only)", len(frames))
	}
	// Resolved frame must come first (priority pass).
	if frames[0].Func != "Resolved" {
		t.Errorf("first frame should be resolved; got %+v", frames[0])
	}
	// Symbol-only frames preserved.
	gotSymbols := make(map[string]bool)
	for _, f := range frames {
		gotSymbols[f.Func] = true
	}
	for _, want := range []string{"Resolved", "Unresolved", "<init>"} {
		if !gotSymbols[want] {
			t.Errorf("symbol-only frame %q dropped", want)
		}
	}
}

// TestCollectExternalObservationSeeds_SurfacesResidueChunks:
// round-9 user red line — UnknownChunks (unstructured log/print
// output) must surface as observation seeds. Pre-fix they were
// invisible to all downstream consumers.
func TestCollectExternalObservationSeeds_SurfacesResidueChunks(t *testing.T) {
	bundle := &LogBundle{
		Residue: LogResidue{
			UnknownChunks: []string{
				"DEBUG: cache size = 4096",
				"WARN: deprecation: LegacyAPI used in MyHandler",
			},
		},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	gotResidue := 0
	for _, s := range seeds {
		if s.Kind == "log_residue" {
			gotResidue++
			if !strings.Contains(s.Raw, "cache") && !strings.Contains(s.Raw, "deprecation") {
				t.Errorf("residue seed Raw missing content: %q", s.Raw)
			}
		}
	}
	if gotResidue == 0 {
		t.Errorf("no residue seeds surfaced; got %d total seeds: %+v", len(seeds), seeds)
	}
}
