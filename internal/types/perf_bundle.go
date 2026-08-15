package types

import "strings"

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
	Meta         PerfMeta          `json:"meta"`
	Frames       []PerfFrame       `json:"frames,omitempty"`
	Janks        []PerfJank        `json:"janks,omitempty"`
	Stalls       []PerfStall       `json:"stalls,omitempty"`
	Startup      *PerfStartup      `json:"startup,omitempty"`
	Observations []PerfObservation `json:"observations,omitempty"`
	Residue      []string          `json:"residue,omitempty"`
	Coverage     float64           `json:"coverage,omitempty"`

	// Layer-4 derivation. Validator-written.
	ResolvedFiles []string `json:"resolved_files,omitempty"`
	Entities      []string `json:"entities,omitempty"`
	IntentHint    string   `json:"intent_hint,omitempty"` // "performance" when any jank present
}

// HasStructuredObservations reports whether the perf bundle carries at
// least one typed runtime observation. Residue-only bundles are excluded:
// they are useful context, but not precise enough to drive hard
// grounding-disposition decisions.
func (b *PerfBundle) HasStructuredObservations() bool {
	if b == nil {
		return false
	}
	return len(b.Frames) > 0 || len(b.Janks) > 0 || len(b.Stalls) > 0 ||
		b.Startup != nil || len(b.Observations) > 0
}

// IsExternalSource reports whether the trace has structured perf
// observations but none of them resolve to current-repo files. This is
// the trace analogue of LogBundle.IsExternalSource: runtime facts remain
// answer-grade, but repo file:line citations are not the source of those
// facts.
func (b *PerfBundle) IsExternalSource() bool {
	if b == nil {
		return false
	}
	return len(b.ResolvedFiles) == 0 && b.HasStructuredObservations()
}

// LogFrames returns the perf bundle's frame-like observations in the
// shared LogFrame shape used by drift and answer-surface projection.
// PerfStall.Symbol maps to LogFrame.Func and keeps File/Line when
// present; jank trigger spans and startup mode are symbol-only frames.
func (b *PerfBundle) LogFrames() []LogFrame {
	if b == nil {
		return nil
	}
	var out []LogFrame
	for i := range b.Stalls {
		s := &b.Stalls[i]
		if strings.TrimSpace(s.Symbol) == "" {
			continue
		}
		out = append(out, LogFrame{
			File:               s.File,
			Line:               s.Line,
			Func:               s.Symbol,
			ArtifactStartTsMs:  s.StartTsMs,
			ArtifactDurationMs: s.DurationMs,
		})
	}
	for i := range b.Janks {
		j := &b.Janks[i]
		span := strings.TrimSpace(j.TriggerSpan)
		if span == "" {
			continue
		}
		out = append(out, LogFrame{Func: span})
	}
	if b.Startup != nil {
		mode := strings.TrimSpace(b.Startup.Mode)
		if mode != "" {
			out = append(out, LogFrame{Func: mode + "-startup"})
		}
	}
	return out
}

// PerfMeta carries trace-level descriptive fields.
type PerfMeta struct {
	// BugClasses mirrors LogMeta.BugClasses for the perf channel
	// (deterministic cross-platform bug-pattern detection populated
	// by the perf_triage entry hook before the LLM dispatch). Empty
	// when no registered pattern fired. See log_bundle.go for the
	// architectural rationale.
	BugClasses []DetectedBugClass `json:"bug_classes,omitempty"`

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
	// provided by the LLM (≤200 chars). It is retained for audit and
	// compatibility, but is not deterministic trace-query authority.
	Summary string `json:"summary,omitempty"`
}

// PerfObservationAuthority identifies who minted the semantic content of a
// PerfObservation. The zero value means unspecified authority. Consumers must
// fail closed: measured values remain usable, but unspecified authority cannot
// mint a jank verdict or a causal claim.
//
// This is intentionally validator-owned and is not exposed in the
// emit_perf_trace tool schema. Downstream prompt projection can therefore
// distinguish model-extracted navigation hypotheses from deterministic
// system semantics without inspecting user wording or observation prose.
type PerfObservationAuthority string

const (
	PerfObservationAuthorityPreTriageModelExtraction PerfObservationAuthority = "pretriage_model_extraction"
	PerfObservationAuthorityDeterministicValidator   PerfObservationAuthority = "deterministic_validator"
)

// PerfObservation preserves trace-local facts that are important to the user
// but are not necessarily janky frames, stalls, or startup envelopes. Examples:
// "GC:Collect begins on artifact line 5", "the GC span lasts 8ms", or
// "no GC span exceeds 50ms". These are runtime-artifact facts, not current
// repository source citations.
type PerfObservation struct {
	Authority  PerfObservationAuthority `json:"authority,omitempty"`
	Kind       string                   `json:"kind,omitempty"`
	Subject    string                   `json:"subject,omitempty"`
	Summary    string                   `json:"summary,omitempty"`
	Evidence   string                   `json:"evidence,omitempty"`
	LineStart  int                      `json:"line_start,omitempty"`
	LineEnd    int                      `json:"line_end,omitempty"`
	StartTsMs  float64                  `json:"start_ts_ms,omitempty"`
	EndTsMs    float64                  `json:"end_ts_ms,omitempty"`
	DurationMs float64                  `json:"duration_ms,omitempty"`
	Tags       []string                 `json:"tags,omitempty"`
	Confidence float64                  `json:"confidence,omitempty"`
}

// IsNavigationOnly reports whether this observation can be used only to
// locate a trace region for deterministic follow-up. The zero authority is
// intentionally fail-closed for bundles produced before Authority existed:
// model-authored subject/summary text must not silently become semantic or
// causal authority merely because the producer omitted the discriminator.
func (o PerfObservation) IsNavigationOnly() bool {
	return o.Authority != PerfObservationAuthorityDeterministicValidator
}

// PerfFrame represents a single UI frame the trace observed.
// FrameNo + TsMs make each frame uniquely addressable so the
// validator can detect duplicates.
type PerfFrame struct {
	FrameNo       int                      `json:"frame_no,omitempty"`
	TsMs          float64                  `json:"ts_ms,omitempty"`
	DurationMs    float64                  `json:"duration_ms"`
	Phase         string                   `json:"phase,omitempty"` // "measure" / "layout" / "draw" / "composite" / ""
	Janky         bool                     `json:"janky,omitempty"`
	JankAuthority PerfObservationAuthority `json:"jank_authority,omitempty"`
}

// JankIsPreTriageModelExtraction reports whether Janky is a navigation
// classification emitted by perf pre-triage rather than a deadline verdict
// backed by an explicit device refresh/deadline authority. DurationMs remains
// a separately measured value in either case.
func (f PerfFrame) JankIsPreTriageModelExtraction() bool {
	return f.Janky && f.JankAuthority != PerfObservationAuthorityDeterministicValidator
}

// PerfJank describes one slow interval with the chain of
// `tracing_mark_write: B|...` tags that opened inside it. Duration is the
// value lane; TriggerSpan / Reason / Tags are a separate causal-candidate
// lane governed by CausalAuthority. Pre-triage extraction must not turn the
// innermost tag or a best-guess reason into a proven cause.
type PerfJank struct {
	StartTsMs        float64                  `json:"start_ts_ms"`
	DurationMs       float64                  `json:"duration_ms"`
	TriggerSpan      string                   `json:"trigger_span,omitempty"`
	Reason           string                   `json:"reason,omitempty"` // "io" / "lock" / "sync-call" / "heavy-compute" / ""
	Tags             []string                 `json:"tags,omitempty"`
	VerdictAuthority PerfObservationAuthority `json:"verdict_authority,omitempty"`
	CausalAuthority  PerfObservationAuthority `json:"causal_authority,omitempty"`
}

// VerdictIsPreTriageModelExtraction reports whether membership in Janks is a
// model-extracted slow-frame candidate rather than a typed frame-deadline
// verdict. It is independent from the trigger/reason causal authority.
func (j PerfJank) VerdictIsPreTriageModelExtraction() bool {
	return j.VerdictAuthority != PerfObservationAuthorityDeterministicValidator
}

// CauseIsPreTriageModelExtraction reports whether TriggerSpan / Reason / Tags
// describe a model-extracted cause candidate rather than a deterministic
// trace-query causal result. The time interval and validator-derived Janky bit
// remain separate measured/value lanes.
func (j PerfJank) CauseIsPreTriageModelExtraction() bool {
	return j.CausalAuthority != PerfObservationAuthorityDeterministicValidator
}

// HasAuthoritativeJankVerdict reports whether the bundle contains at least one
// jank verdict minted by a deterministic validator with scenario authority.
// Slow-frame candidates and legacy entries with unspecified authority do not
// satisfy hard jank facets.
func (b *PerfBundle) HasAuthoritativeJankVerdict() bool {
	if b == nil {
		return false
	}
	for _, frame := range b.Frames {
		if frame.Janky && !frame.JankIsPreTriageModelExtraction() {
			return true
		}
	}
	for _, jank := range b.Janks {
		if !jank.VerdictIsPreTriageModelExtraction() {
			return true
		}
	}
	return false
}

// PerfStall is a sub-event inside a PerfJank — specifically a
// main-thread blocking call whose duration is noteworthy. Kept as
// a separate slice so consumers can list stalls without walking
// Jank.Tags.
type PerfStall struct {
	Authority  PerfObservationAuthority `json:"authority,omitempty"`
	StartTsMs  float64                  `json:"start_ts_ms"`
	DurationMs float64                  `json:"duration_ms"`
	Kind       string                   `json:"kind,omitempty"` // "io" / "lock" / "sync-rpc" / "native-call" / ""
	Symbol     string                   `json:"symbol,omitempty"`
	File       string                   `json:"file,omitempty"`
	Line       int                      `json:"line,omitempty"`
}

// IsNavigationOnly reports whether the stall tuple is a pre-triage model
// extraction rather than a validator-owned runtime fact. The zero value is
// fail-closed for persisted bundles produced before this discriminator was
// introduced: a model-selected span, kind, symbol, or duration must not gain
// causal authority merely because an older producer omitted the field.
func (s PerfStall) IsNavigationOnly() bool {
	return s.Authority != PerfObservationAuthorityDeterministicValidator
}

// PerfStartup carries cold-start / warm-start timing when the trace
// covers a process-spawn event (detected by `ActivityTaskManager`
// or `AppInit` tags). Single-occurrence per bundle.
type PerfStartup struct {
	Mode          string  `json:"mode"` // "cold" / "warm" / "hot"
	AppLaunchMs   float64 `json:"app_launch_ms,omitempty"`
	AbilityInitMs float64 `json:"ability_init_ms,omitempty"`
	FirstFrameMs  float64 `json:"first_frame_ms,omitempty"`
}

// Performance comparison constants. These values are useful only after their
// corresponding scenario authority is explicit. In particular, a 60Hz
// comparison constant must not become a default device refresh/deadline or
// jank verdict when the trace did not provide one.
const (
	// PerfFrameBudget60HzMs is the mathematical period of a 60fps signal in
	// milliseconds. It is not a validator-owned default jank threshold.
	PerfFrameBudget60HzMs = 16.67

	// PerfStartupSlowColdMs is the cold-start slow threshold. Cold
	// starts above this mark add "cold-start-slow" to Meta.Signals.
	PerfStartupSlowColdMs = 1200.0

	// PerfMainThreadStallMs is the main-thread stall threshold. A
	// blocking call longer than this adds "main-thread-stall" to
	// Meta.Signals.
	PerfMainThreadStallMs = 100.0
)
