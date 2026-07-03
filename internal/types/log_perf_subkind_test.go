package types

import "testing"

func TestLogPerfSubKind_IsValid(t *testing.T) {
	for _, k := range AllLogPerfSubKinds() {
		if !k.IsValid() {
			t.Errorf("declared kind %q must report IsValid=true", k)
		}
	}
	if !LogPerfSubKindUnknown.IsValid() {
		t.Error("empty / unknown is valid as the default zero value")
	}
	for _, bad := range []LogPerfSubKind{"bogus", "panic", "log_panic"} {
		if bad.IsValid() {
			t.Errorf("undeclared %q must report IsValid=false", bad)
		}
	}
}

func TestLogPerfSubKind_LogVsPerfClassification(t *testing.T) {
	logKinds := []LogPerfSubKind{
		LogPanicFrame, LogOOMFrame, LogTimeoutFrame,
		LogErrorFrame, LogNoiseFrame,
	}
	perfKinds := []LogPerfSubKind{
		PerfJankFrame, PerfStallFrame, PerfStartupFrame,
	}
	for _, k := range logKinds {
		if !k.IsLogSubKind() {
			t.Errorf("%q should report IsLogSubKind=true", k)
		}
		if k.IsPerfSubKind() {
			t.Errorf("%q should report IsPerfSubKind=false", k)
		}
	}
	for _, k := range perfKinds {
		if !k.IsPerfSubKind() {
			t.Errorf("%q should report IsPerfSubKind=true", k)
		}
		if k.IsLogSubKind() {
			t.Errorf("%q should report IsLogSubKind=false", k)
		}
	}
}

func TestLogPerfSubKindOf_NoFrameMatchReturnsUnknown(t *testing.T) {
	item := EvidenceItem{Source: "src/x.go", LineStart: 10}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{SignalPanic}},
		Errors: []LogError{{Type: "PanicError", Frames: []LogFrame{{File: "other.go", Line: 5}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogPerfSubKindUnknown {
		t.Errorf("non-matching frame should return unknown; got %q", got)
	}
}

func TestLogPerfSubKindOf_PanicFrameWins(t *testing.T) {
	item := EvidenceItem{Source: "src/foo.go", LineStart: 42}
	bundle := &LogBundle{
		Meta: LogMeta{Signals: []LogSignal{SignalPanic, SignalCrash}},
		Errors: []LogError{{
			Type:   "PanicError",
			Frames: []LogFrame{{File: "src/foo.go", Line: 42}},
		}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogPanicFrame {
		t.Errorf("expected LogPanicFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_OOMFrame(t *testing.T) {
	item := EvidenceItem{Source: "alloc.go", LineStart: 100}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{SignalOOM}},
		Errors: []LogError{{Type: "OutOfMemory", Frames: []LogFrame{{File: "alloc.go", Line: 100}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogOOMFrame {
		t.Errorf("expected LogOOMFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_TimeoutFrame(t *testing.T) {
	item := EvidenceItem{Source: "ctx.go", LineStart: 7}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{SignalTimeout}},
		Errors: []LogError{{Type: "DeadlineExceeded", Frames: []LogFrame{{File: "ctx.go", Line: 7}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogTimeoutFrame {
		t.Errorf("expected LogTimeoutFrame; got %q", got)
	}
}

// TestLogPerfSubKindOf_PerformanceFrame locks the 2026-05-08
// expansion: SignalPerformance log frames now project to
// LogPerformanceFrame instead of falling through to the generic
// LogErrorFrame.
func TestLogPerfSubKindOf_PerformanceFrame(t *testing.T) {
	item := EvidenceItem{Source: "api.go", LineStart: 42}
	bundle := &LogBundle{
		Meta: LogMeta{Signals: []LogSignal{SignalPerformance}},
		// LogError carries a typed entry so frame-match succeeds —
		// real perf-mention logs typically have a structured
		// "slow API took 5s" record. logBundleFrameMatch walks
		// the Errors tree.
		Errors: []LogError{{Type: "SlowAPI", Frames: []LogFrame{{File: "api.go", Line: 42}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogPerformanceFrame {
		t.Errorf("expected LogPerformanceFrame; got %q", got)
	}
}

// TestLogPerfSubKindOf_PerformanceLowerThanTimeout pins the severity
// ladder ranking: when both timeout and performance signals are
// present (e.g. a request that timed out AND was slow), the
// timeout-class wins because it carries stronger semantics
// (operation was cancelled) than the perf observation.
func TestLogPerfSubKindOf_PerformanceLowerThanTimeout(t *testing.T) {
	item := EvidenceItem{Source: "api.go", LineStart: 42}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{SignalPerformance, SignalTimeout}},
		Errors: []LogError{{Type: "DeadlineExceeded", Frames: []LogFrame{{File: "api.go", Line: 42}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogTimeoutFrame {
		t.Errorf("expected LogTimeoutFrame (timeout outranks performance); got %q", got)
	}
}

func TestLogPerfSubKindOf_ErrorFrameWithoutSeveritySignal(t *testing.T) {
	item := EvidenceItem{Source: "handler.go", LineStart: 30}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{}}, // no severity
		Errors: []LogError{{Type: "ValidationFailure", Frames: []LogFrame{{File: "handler.go", Line: 30}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogErrorFrame {
		t.Errorf("expected LogErrorFrame (typed error, no severity signal); got %q", got)
	}
}

func TestLogPerfSubKindOf_NoiseFrame(t *testing.T) {
	// Frame matched (file equality) but it's NOT in any LogError
	// (no Errors array entry, just Residue / unstructured chunk
	// reference). Should fall through to LogNoiseFrame.
	item := EvidenceItem{Source: "noise.go", LineStart: 1}
	bundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{}},
		Errors: []LogError{{Type: "", Frames: []LogFrame{{File: "noise.go", Line: 1}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogNoiseFrame {
		t.Errorf("expected LogNoiseFrame (frame matched, no typed error); got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfStall(t *testing.T) {
	item := EvidenceItem{Source: "render.go", LineStart: 5}
	bundle := &PerfBundle{
		Stalls: []PerfStall{{File: "render.go", Symbol: "doRender"}},
	}
	if got := LogPerfSubKindOf(item, nil, bundle); got != PerfStallFrame {
		t.Errorf("expected PerfStallFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfJank(t *testing.T) {
	item := EvidenceItem{Source: "view.go", Subject: "main_thread"}
	bundle := &PerfBundle{
		Janks: []PerfJank{{TriggerSpan: "main_thread", DurationMs: 200}},
	}
	if got := LogPerfSubKindOf(item, nil, bundle); got != PerfJankFrame {
		t.Errorf("expected PerfJankFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfStartup_SlowAppLaunch(t *testing.T) {
	item := EvidenceItem{Source: "startup.go", LineStart: 1}
	// Slow startup: AppLaunchMs > PerfStartupSlowColdMs (1200ms)
	bundle := &PerfBundle{
		Startup: &PerfStartup{AppLaunchMs: 2000},
	}
	if got := LogPerfSubKindOf(item, nil, bundle); got != PerfStartupFrame {
		t.Errorf("expected PerfStartupFrame for slow cold-start; got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfStartup_FastStartupReturnsUnknown(t *testing.T) {
	item := EvidenceItem{Source: "startup.go", LineStart: 1}
	bundle := &PerfBundle{
		Startup: &PerfStartup{AppLaunchMs: 800}, // below threshold
	}
	if got := LogPerfSubKindOf(item, nil, bundle); got == PerfStartupFrame {
		t.Errorf("fast startup should NOT trigger PerfStartupFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfStartup_NilStartupReturnsUnknown(t *testing.T) {
	item := EvidenceItem{Source: "startup.go", LineStart: 1}
	bundle := &PerfBundle{Startup: nil}
	if got := LogPerfSubKindOf(item, nil, bundle); got == PerfStartupFrame {
		t.Errorf("nil Startup should NOT trigger PerfStartupFrame; got %q", got)
	}
}

func TestLogPerfSubKindOf_PerfWinsOverLog(t *testing.T) {
	// Both bundles match. Perf-side classification takes priority
	// (perf signals are more specific than generic log presence).
	item := EvidenceItem{Source: "render.go", LineStart: 5}
	logBundle := &LogBundle{
		Meta:   LogMeta{Signals: []LogSignal{SignalPanic}},
		Errors: []LogError{{Type: "Panic", Frames: []LogFrame{{File: "render.go", Line: 5}}}},
	}
	perfBundle := &PerfBundle{
		Stalls: []PerfStall{{File: "render.go", Symbol: "render"}},
	}
	if got := LogPerfSubKindOf(item, logBundle, perfBundle); got != PerfStallFrame {
		t.Errorf("perf-stall should win over log-panic; got %q", got)
	}
}

func TestLogPerfSubKindOf_NilBundlesReturnsUnknown(t *testing.T) {
	item := EvidenceItem{Source: "x.go", LineStart: 1}
	if got := LogPerfSubKindOf(item, nil, nil); got != LogPerfSubKindUnknown {
		t.Errorf("nil bundles → unknown; got %q", got)
	}
}

func TestLogPerfSubKindOf_EmptySourceReturnsUnknown(t *testing.T) {
	item := EvidenceItem{LineStart: 5}
	bundle := &LogBundle{
		Errors: []LogError{{Type: "X", Frames: []LogFrame{{File: "x.go", Line: 5}}}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogPerfSubKindUnknown {
		t.Errorf("empty Source → unknown; got %q", got)
	}
}

func TestLogPerfSubKindOf_CauseChainTraversed(t *testing.T) {
	// Frame is in a Cause chain (depth-2 LogError), not at top.
	// The walker must descend into Cause.
	item := EvidenceItem{Source: "deep.go", LineStart: 99}
	bundle := &LogBundle{
		Meta: LogMeta{Signals: []LogSignal{SignalPanic}},
		Errors: []LogError{{
			Type:   "TopLevelPanic",
			Frames: []LogFrame{{File: "top.go", Line: 1}},
			Cause: &LogError{
				Type:   "DeeperCause",
				Frames: []LogFrame{{File: "deep.go", Line: 99}},
			},
		}},
	}
	if got := LogPerfSubKindOf(item, bundle, nil); got != LogPanicFrame {
		t.Errorf("Cause-chain frame should still classify; got %q", got)
	}
}

func TestAllLogPerfSubKinds_DefensiveCopy(t *testing.T) {
	a := AllLogPerfSubKinds()
	b := AllLogPerfSubKinds()
	// 2026-05-08: 9 sub-kinds — added LogPerformanceFrame between
	// LogTimeoutFrame and LogErrorFrame so SignalPerformance log
	// frames stop collapsing to the generic LogErrorFrame.
	if len(a) != 9 {
		t.Fatalf("expected 9 sub-kinds, got %d", len(a))
	}
	a[0] = "MUTATED"
	if b[0] == "MUTATED" {
		t.Error("AllLogPerfSubKinds must return defensive copies")
	}
}
