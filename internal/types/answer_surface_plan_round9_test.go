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
		LogSourceDriftAnchors: []LogSourceDriftAnchor{{File: "x.go", AnchoredLine: 100}},
	}
	// Drift surface mode requires a long-form-prose family in addition
	// to the diagnostic intent — supply Scenario=ArchitectureExplain so
	// ResolveQuestionFamily routes to QFArchitecture (long-form). The
	// gate's intent dimension is what the cases vary.
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
		rm := RequestModel{Intent: tc.intent, Scenario: ScenarioArchitectureExplain}
		got := preferredAnswerSummarySurfaceMode(plan, rm)
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

// TestIntentFrameCap_TraceAndRootCauseLargerThanDefault pins the
// B.7 audit followup contract (2026-05-02): trace and root-cause
// questions raise the frame cap above the historical 16 because
// deeper stack walks materially help drift detection on those
// shapes. Other intents stay at 16.
func TestIntentFrameCap_TraceAndRootCauseLargerThanDefault(t *testing.T) {
	cases := []struct {
		intent Intent
		want   int
	}{
		{IntentTrace, 32},
		{IntentRootCause, 48},
		{IntentExplain, 16},
		{IntentEnumerate, 16},
		{IntentConfigQuery, 16},
		{IntentUnknown, 16}, // empty / unset Intent
		{Intent(""), 16},    // unrecognised string
	}
	for _, c := range cases {
		if got := intentFrameCap(c.intent); got != c.want {
			t.Errorf("intentFrameCap(%q) = %d, want %d", c.intent, got, c.want)
		}
	}
}

// TestCollectArtifactFramesWithOrigin_RootCauseRaisesCap pins the
// end-to-end B.7 contract: when intent=RootCause, the cap allows
// 48 perf stalls instead of 16. We synthesise 50 stalls and verify
// the cap honours the intent.
func TestCollectArtifactFramesWithOrigin_RootCauseRaisesCap(t *testing.T) {
	stalls := make([]PerfStall, 50)
	for i := range stalls {
		stalls[i] = PerfStall{Symbol: "Sym" + fmtInt(i), File: "x.go", Line: 100 + i}
	}
	perf := &PerfBundle{Stalls: stalls}

	defaultFrames, _ := collectArtifactFramesWithOrigin(nil, perf, IntentExplain)
	if len(defaultFrames) != 16 {
		t.Errorf("default cap: got %d frames, want 16", len(defaultFrames))
	}
	traceFrames, _ := collectArtifactFramesWithOrigin(nil, perf, IntentTrace)
	if len(traceFrames) != 32 {
		t.Errorf("trace cap: got %d frames, want 32", len(traceFrames))
	}
	rcFrames, _ := collectArtifactFramesWithOrigin(nil, perf, IntentRootCause)
	if len(rcFrames) != 48 {
		t.Errorf("root-cause cap: got %d frames, want 48", len(rcFrames))
	}
}

// fmtInt is a tiny helper so the test doesn't need strconv import.
func fmtInt(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
