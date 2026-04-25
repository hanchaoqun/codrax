package types

// PerfBundle is the validated output of the perf_triage pre-stage,
// mirroring LogBundle for the HiTrace / Android-systrace channel.
// Written once by the perf_triager agent via SetPerfTrace; read by
// the analyzer (entity hint + intent elevation to IntentPerformance)
// and by the finalizer for rendering.
//
// The schema is deliberately narrower than LogBundle. Performance
// traces answer "where did the frame / startup time go?" — the
// emission is jank-span + main-thread-stall centric, not error-tree
// centric. LogBundle's recursive cause chain does not apply.
//
// Layer structure (matches LogBundle doc for consistency):
//
//	Layer 1 — Meta: source tool, approximate trace duration,
//	         app PID if detectable, list of one-word signals.
//	Layer 2 — Frames / spans: the actual timing data.
//	Layer 3 — Residue: unstructured chunks (emitter could not
//	         parse). Zero information loss.
//	Layer 4 — Derivation: Coverage, IntentHint, ResolvedFiles,
//	         Entities. Written by the validator, never by the LLM.
//
// JSON tags are stable — this struct is part of the write-once
// handoff contract and gets persisted in the Run's debug log.
type PerfBundle struct {
	Meta     PerfMeta      `json:"meta"`
	Frames   []PerfFrame   `json:"frames,omitempty"`
	Janks    []PerfJank    `json:"janks,omitempty"`
	Stalls   []PerfStall   `json:"stalls,omitempty"`
	Startup  *PerfStartup  `json:"startup,omitempty"`
	Residue  []string      `json:"residue,omitempty"`
	Coverage float64       `json:"coverage,omitempty"`

	// Layer-4 derivation. Validator-written.
	ResolvedFiles []string `json:"resolved_files,omitempty"`
	Entities      []string `json:"entities,omitempty"`
	IntentHint    string   `json:"intent_hint,omitempty"` // "performance" when any jank present
}

// PerfMeta carries trace-level descriptive fields.
type PerfMeta struct {
	// Source is the capture tool's canonical name: "hitrace"
	// (HarmonyOS) / "atrace" (Android) / "systrace" (legacy) /
	// "perfetto" (when the LLM decoded a perfetto text dump) /
	// "unknown".
	Source string `json:"source"`

	// DurationMs is the approximate wall-clock span of the trace
	// excerpt (last-timestamp − first-timestamp). 0 means unknown.
	DurationMs float64 `json:"duration_ms,omitempty"`

	// AppPID is the foreground process id observed most often in
	// the trace. 0 when not detectable.
	AppPID int `json:"app_pid,omitempty"`

	// Signals is a small set of one-word labels describing what
	// went wrong. Canonical values: "jank", "cold-start-slow",
	// "main-thread-stall", "io-block", "gc-pause", "render-miss".
	// Kept short so the analyzer can use them as entity tokens.
	Signals []string `json:"signals,omitempty"`

	// Summary is an optional one-line natural-language synopsis
	// provided by the LLM (≤200 chars). Rendered verbatim by the
	// finalizer.
	Summary string `json:"summary,omitempty"`
}

// PerfFrame represents a single UI frame the trace observed.
// FrameNo + TsMs make each frame uniquely addressable so the
// validator can detect duplicates.
type PerfFrame struct {
	FrameNo    int     `json:"frame_no,omitempty"`
	TsMs       float64 `json:"ts_ms,omitempty"`
	DurationMs float64 `json:"duration_ms"`
	Phase      string  `json:"phase,omitempty"` // "measure" / "layout" / "draw" / "composite" / ""
	Janky      bool    `json:"janky,omitempty"` // DurationMs > frame budget (16.6 ms @ 60Hz)
}

// PerfJank describes one span of observed jank with the chain of
// `tracing_mark_write: B|...` tags that opened inside it. The LLM
// collects the innermost (deepest) tag as the most likely cause.
type PerfJank struct {
	StartTsMs   float64  `json:"start_ts_ms"`
	DurationMs  float64  `json:"duration_ms"`
	TriggerSpan string   `json:"trigger_span,omitempty"`
	Reason      string   `json:"reason,omitempty"` // "io" / "lock" / "sync-call" / "heavy-compute" / ""
	Tags        []string `json:"tags,omitempty"`
}

// PerfStall is a sub-event inside a PerfJank — specifically a
// main-thread blocking call whose duration is noteworthy. Kept as
// a separate slice so consumers can list stalls without walking
// Jank.Tags.
type PerfStall struct {
	StartTsMs   float64 `json:"start_ts_ms"`
	DurationMs  float64 `json:"duration_ms"`
	Kind        string  `json:"kind,omitempty"` // "io" / "lock" / "sync-rpc" / "native-call" / ""
	Symbol      string  `json:"symbol,omitempty"`
	File        string  `json:"file,omitempty"`
	Line        int     `json:"line,omitempty"`
}

// PerfStartup carries cold-start / warm-start timing when the trace
// covers a process-spawn event (detected by `ActivityTaskManager`
// or `AppInit` tags). Single-occurrence per bundle.
type PerfStartup struct {
	Mode           string  `json:"mode"` // "cold" / "warm" / "hot"
	AppLaunchMs    float64 `json:"app_launch_ms,omitempty"`
	AbilityInitMs  float64 `json:"ability_init_ms,omitempty"`
	FirstFrameMs   float64 `json:"first_frame_ms,omitempty"`
}

// Jank threshold constants the validator uses to populate Meta.Signals
// and the Janky bit on PerfFrame. Kept public so callers (and the
// finalizer prose renderer) can explain the thresholds they applied.
const (
	// PerfFrameBudget60HzMs is the 60fps frame budget in
	// milliseconds. Frames exceeding this are marked janky unless
	// the device's refresh rate is explicitly reported.
	PerfFrameBudget60HzMs = 16.67

	// PerfStartupSlowColdMs is the cold-start slow threshold. Cold
	// starts above this mark add "cold-start-slow" to Meta.Signals.
	PerfStartupSlowColdMs = 1200.0

	// PerfMainThreadStallMs is the main-thread stall threshold. A
	// blocking call longer than this adds "main-thread-stall" to
	// Meta.Signals.
	PerfMainThreadStallMs = 100.0
)
