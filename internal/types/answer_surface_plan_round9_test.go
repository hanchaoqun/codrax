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

func TestBuildAnswerSurfacePlan_ExternalTraceExactTargetsDoNotForceCurrentStatus(t *testing.T) {
	perf := &PerfBundle{
		Observations: []PerfObservation{{
			Kind:      "state_churn",
			Subject:   "app-20",
			Summary:   "state_churn app-20 dominant_state=runnable",
			LineStart: 3,
			LineEnd:   23,
		}},
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent:    IntentRootCause,
			Scenario:  ScenarioPerformanceBottleneck,
			PerfTrace: perf,
			AnalyzerHints: AnalyzerHints{
				ExactTargets: []string{"app-20", "11.0s-11.008s"},
			},
			Predicates: SemanticPredicates{IsDiagnosticQuestion: true},
			DiagnosticProfile: DiagnosticIntentProfile{
				IsDiagnostic: true,
				CurrentRisk:  true,
				Confidence:   0.95,
			},
			ExternalObservationPolicy: &ExternalObservationPolicy{
				ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    ExternalObservationCurrentSourceDefault,
				Confidence:           0.95,
			},
		},
		AnswerContract: AnswerContract{
			CurrentStatusDiagnostic: &CurrentStatusDiagnosticContract{Required: true},
		},
	}
	mut := NewMutableState("trace window stats")
	mut.SetPerfTrace(perf)

	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	if plan.CurrentStatusDiagnosticRequired {
		t.Fatal("external trace exact targets alone must not force current-status diagnostic")
	}
	if plan.CurrentSourceEvidenceOrigin {
		t.Fatal("external trace exact targets alone must not force current-source evidence origin")
	}
	if plan.RuntimeGroundingDisposition == nil || !plan.RuntimeGroundingDisposition.IsActive() {
		t.Fatalf("runtime grounding disposition should remain active: %+v", plan.RuntimeGroundingDisposition)
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

func TestCollectAllLogFrames_WalksCauseChain(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{
			Type: "outer",
			Frames: []LogFrame{
				{File: "outer.go", Line: 10, Func: "Outer", Raw: "outer.go:10"},
			},
			Cause: &LogError{
				Type: "inner",
				Frames: []LogFrame{
					{File: "inner.go", Line: 20, Func: "Inner", Raw: "inner.go:20"},
				},
			},
		}},
	}
	frames := collectAllLogFrames(bundle)
	got := make(map[string]bool)
	for _, frame := range frames {
		got[frame.Func] = true
	}
	for _, want := range []string{"Outer", "Inner"} {
		if !got[want] {
			t.Fatalf("cause-chain frame %q was not collected: %+v", want, frames)
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

func TestCollectExternalObservationSeeds_SurfacesStructuredObservations(t *testing.T) {
	bundle := &LogBundle{
		Observations: []LogObservation{{
			Kind:       LogObservationTopicMismatch,
			Subject:    "line-number handling",
			Summary:    "answer body discussed emit_evidence instead of the requested line-number bug",
			Diagnostic: true,
			Confidence: 0.9,
		}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	for _, seed := range seeds {
		if seed.Kind == "log_observation" &&
			strings.Contains(seed.Raw, "emit_evidence") &&
			seed.Func == "line-number handling" {
			return
		}
	}
	t.Fatalf("structured observation seed missing: %+v", seeds)
}

func TestCollectArtifactExternalObservationSeeds_SurfacesPerfTrace(t *testing.T) {
	perf := &PerfBundle{
		Janks: []PerfJank{{
			DurationMs:  52,
			TriggerSpan: "RenderList",
			Reason:      "heavy-compute",
		}},
		Stalls: []PerfStall{{
			DurationMs: 140,
			Kind:       "io",
			Symbol:     "LoadAvatar",
			File:       "ui/profile.go",
			Line:       88,
		}},
	}
	seeds := CollectArtifactExternalObservationSeeds(nil, perf, nil)
	var sawJank, sawStall bool
	for _, seed := range seeds {
		switch seed.Kind {
		case "perf_jank":
			sawJank = seed.Func == "RenderList" && strings.Contains(seed.Raw, "heavy-compute")
		case "perf_stall":
			sawStall = seed.Func == "LoadAvatar" && seed.File == "ui/profile.go" && seed.Line == 88
		}
	}
	if !sawJank || !sawStall {
		t.Fatalf("perf external observation seeds missing jank/stall: %+v", seeds)
	}
}

func TestCollectExternalObservationSeeds_WalksCauseChainFrames(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{
			Type: "outer",
			Cause: &LogError{
				Type: "inner",
				Frames: []LogFrame{{
					File: "inner.go",
					Line: 22,
					Func: "Inner",
					Raw:  "inner.go:22 Inner",
				}},
			},
		}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	for _, seed := range seeds {
		if seed.Kind == "log_frame" && seed.File == "inner.go" && seed.Line == 22 {
			return
		}
	}
	t.Fatalf("nested cause frame did not surface as external observation seed: %+v", seeds)
}

func TestCollectExternalObservationSeeds_TagsFirstFramePerErrorAsHead(t *testing.T) {
	bundle := &LogBundle{
		Errors: []LogError{{
			Type: "Error",
			Frames: []LogFrame{
				{Lang: "arkts", File: "NativeBridge.ets", Line: 33, Func: "NativeBridge.invokeOhSum"},
				{Lang: "arkts", File: "Home.ets", Line: 54, Func: "HomePage.computeTotal"},
			},
			Cause: &LogError{
				Type: "panic",
				Frames: []LogFrame{
					{Lang: "cangjie", File: "Bridge.cj", Line: 18, Func: "demo.bridge.ohSum"},
					{Lang: "cangjie", File: "Bridge.cj", Line: 42, Func: "demo.bridge.checkout"},
				},
			},
		}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	roles := make(map[string]string)
	for _, seed := range seeds {
		if seed.Kind == "log_frame" {
			roles[seed.Func] = seed.Role
		}
	}
	want := map[string]string{
		"NativeBridge.invokeOhSum": "error_head_frame",
		"HomePage.computeTotal":    "caller_frame",
		"demo.bridge.ohSum":        "error_head_frame",
		"demo.bridge.checkout":     "caller_frame",
	}
	for fn, role := range want {
		if roles[fn] != role {
			t.Fatalf("frame %s role=%q, want %q (all=%+v)", fn, roles[fn], role, seeds)
		}
	}
}

func TestCollectExternalObservationSeeds_DefaultsFrameLanguageFromBundle(t *testing.T) {
	bundle := &LogBundle{
		Meta: LogMeta{Lang: "python"},
		Errors: []LogError{{
			Type: "RuntimeError",
			Frames: []LogFrame{{
				File: "/app/src/loader.py",
				Line: 25,
				Func: "load_config",
				Raw:  "File \"/app/src/loader.py\", line 25, in load_config",
			}},
		}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	for _, seed := range seeds {
		if seed.Kind != "log_frame" {
			continue
		}
		if seed.Lang != "python" {
			t.Fatalf("frame seed lang=%q, want bundle language python (seed=%+v)", seed.Lang, seed)
		}
		return
	}
	t.Fatalf("expected a log_frame seed, got %+v", seeds)
}

func TestSelectExternalObservationSeedsForPrompt_BalancesMetaAndCrossLanguageFrames(t *testing.T) {
	bundle := &LogBundle{
		Meta: LogMeta{Signals: []LogSignal{SignalPanic, SignalCrash}},
		Errors: []LogError{{
			Type:    "Error",
			Message: "Cangjie native call failed: index out of bounds",
			Frames: []LogFrame{
				{Lang: "arkts", File: "entry/src/main/ets/bridges/NativeBridge.ets", Line: 33, Func: "NativeBridge.invokeOhSum", Raw: "at NativeBridge.invokeOhSum (NativeBridge.ets:33:11)"},
				{Lang: "arkts", File: "entry/src/main/ets/pages/Home.ets", Line: 54, Func: "HomePage.computeTotal", Raw: "at HomePage.computeTotal (Home.ets:54:7)"},
			},
			Cause: &LogError{
				Type:    "panic",
				Message: "index out of bounds: index=5, size=3",
				Frames: []LogFrame{
					{Lang: "cangjie", File: "src/bridge/Bridge.cj", Line: 18, Func: "demo.bridge.ohSum", Raw: "at demo.bridge.ohSum(src/bridge/Bridge.cj:18)"},
					{Lang: "cangjie", File: "src/bridge/Bridge.cj", Line: 42, Func: "demo.bridge.checkout", Raw: "at demo.bridge.checkout(src/bridge/Bridge.cj:42)"},
				},
			},
		}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	if len(seeds) < 8 {
		t.Fatalf("collector should retain a typed pool beyond the prompt cap; got %d: %+v", len(seeds), seeds)
	}
	selected := SelectExternalObservationSeedsForPrompt(seeds, 6)
	got := make(map[string]bool)
	gotMessages := make(map[string]bool)
	gotChains := make(map[string]bool)
	for _, seed := range selected {
		got[seed.Func] = true
		if seed.Kind == "error_message" {
			gotMessages[seed.Raw] = true
		}
		if seed.Kind == "error_chain" {
			gotChains[seed.Raw] = true
		}
	}
	if !gotMessages["Cangjie native call failed: index out of bounds"] {
		t.Fatalf("balanced prompt selection should preserve the top-level runtime message: %+v", selected)
	}
	if !gotChains[`runtime cause chain: upstream panic("index out of bounds: index=5, size=3") is wrapped / re-raised as top-level Error("Cangjie native call failed: index out of bounds")`] {
		t.Fatalf("balanced prompt selection should preserve the typed cause-chain direction: %+v", selected)
	}
	for _, want := range []string{
		"NativeBridge.invokeOhSum",
		"HomePage.computeTotal",
		"demo.bridge.ohSum",
		"demo.bridge.checkout",
	} {
		if !got[want] {
			t.Fatalf("balanced prompt selection dropped cross-language frame %q: %+v", want, selected)
		}
	}
}

func TestCollectExternalObservationSeeds_UsesDiagnosticFramePoolBeyondSixteen(t *testing.T) {
	var frames []LogFrame
	for i := 1; i <= 20; i++ {
		frames = append(frames, LogFrame{
			Lang: "cpp",
			File: "src/engine/chain.cpp",
			Line: 100 + i,
			Func: "Engine::Step",
			Raw:  "at Engine::Step(src/engine/chain.cpp)",
		})
	}
	bundle := &LogBundle{
		Errors: []LogError{{Type: "panic", Frames: frames}},
	}
	seeds := CollectExternalObservationSeeds(bundle, nil)
	gotFrames := 0
	for _, seed := range seeds {
		if seed.Kind == "log_frame" {
			gotFrames++
		}
	}
	if gotFrames != len(frames) {
		t.Fatalf("collector should retain the diagnostic frame pool, got %d frames; want %d (seeds=%+v)", gotFrames, len(frames), seeds)
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
