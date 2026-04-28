package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/perftriage"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// PerfTriageSettings carries the runtime tuning for the perf_triager
// agent. Wired from config.RuntimeSettings in cmd/root.go, mirroring
// the LogTriageSettings shape so an operator who already understands
// the log_triage knobs can configure perf without re-learning. Zero
// values yield the defaults documented on each field.
type PerfTriageSettings struct {
	// Enabled gates the whole feature. When false, the perf_triage
	// pre-stage Guard still fires but Execute short-circuits with a
	// "skipped: feature disabled" stage output. Default: true.
	Enabled bool

	// MinBytes is the lower bound for stage invocation. Traces below
	// this size rarely benefit from full perf extraction (they're
	// usually a single-frame snapshot or banner-only). Default: 200.
	MinBytes int

	// MaxRetries bounds the fail-loud retry budget for a single-shot
	// emit_perf_trace that rejects on schema / cross-field validation.
	// 0 = never retry (just degrade). Default: 1.
	MaxRetries int

	// TwoStepEnabled turns the two-step fallback on or off. When
	// false, low-coverage or oversized traces take the single-shot
	// result (or nil bundle on failure) without attempting
	// segmentation. Default: true.
	TwoStepEnabled bool

	// TwoStepBytes is the trace-size threshold above which the
	// controller goes straight to two-step (bypassing single-shot).
	// HiTrace / atrace bodies past 64 KB usually contain multiple
	// PIDs / startups / jank windows that benefit from per-segment
	// extraction. Default: 64 * 1024 (64 KB) — slightly higher than
	// log_triage's 32 KB because perf events are bigger per-line.
	TwoStepBytes int

	// TwoStepCoverage is the coverage threshold below which a
	// single-shot bundle triggers two-step escalation. Coverage is
	// 1 - bytes(residue)/bytes(rawTrace). Default: 0.3.
	TwoStepCoverage float64

	// MaxLLMCalls caps the total LLM invocations for one perf_triage
	// stage run across both steps (1 single-shot + 1 segmentation +
	// up to 10 per-segment, matching the segmenter's 10-segment cap).
	// Default: 12.
	MaxLLMCalls int
}

// DefaultPerfTriageSettings returns the conservative defaults.
func DefaultPerfTriageSettings() PerfTriageSettings {
	return PerfTriageSettings{
		Enabled:         true,
		MinBytes:        200,
		MaxRetries:      1,
		TwoStepEnabled:  true,
		TwoStepBytes:    64 * 1024,
		TwoStepCoverage: 0.3,
		MaxLLMCalls:     12,
	}
}

// perfTriagerEvaluator is the Evaluator implementation for the
// perf_triager agent. Mirrors logTriagerEvaluator's shape including
// a two-step fallback (Step A: emit_perf_segmentation; Step B: per-
// segment emit_perf_trace + MergePerfBundles). Single-shot still
// works for small traces; the controller in perfTriager.Execute
// dispatches the right path based on size + coverage.
//
// The ReAct loop terminates as soon as emit_perf_trace OR
// emit_perf_segmentation succeeds; a failed emit lets the loop
// continue within MaxIterations so the LLM can correct on the next
// turn.
type perfTriagerEvaluator struct {
	settings PerfTriageSettings
	emitSeen bool
	llmCalls int
}

// BuildInitialInstruction: skill owns the full structural prompt;
// no per-dispatch supplement beyond the attached trace (which the
// builder renders in the user section).
func (e *perfTriagerEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	e.emitSeen = false
	return ""
}

// ShouldStop — primary gate is the emit observation; this hook is the
// secondary check BaseAgent consults between iterations.
func (e *perfTriagerEvaluator) ShouldStop(_ llm.Response, _ int) bool { return e.emitSeen }

// Observe watches mid-loop tool results; a successful
// emit_perf_trace OR emit_perf_segmentation flips emitSeen and
// requests loop termination. Two-step Step A produces the latter;
// the controller restarts a fresh BaseAgent.Execute per segment for
// Step B, each of which terminates on its own emit_perf_trace.
func (e *perfTriagerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || obs.LastToolResult == nil {
		return LoopSignal{}
	}
	if !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	switch obs.LastToolResult.ToolName {
	case "emit_perf_trace", "emit_perf_segmentation":
		e.emitSeen = true
		return LoopSignal{StopRequested: true, StopReason: obs.LastToolResult.ToolName + " emitted"}
	}
	return LoopSignal{}
}

// ParseOutput produces the StageOutput. When emit_perf_trace succeeded,
// Mutable.PerfTrace() is already populated and we render a compact
// report. Otherwise fail-loud so the retry path applies.
func (e *perfTriagerEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	if ctx == nil || ctx.Mutable == nil {
		return &StageOutput{
			Error: "perf_triager requires a writable context; the caller did not provide one",
		}, nil
	}
	// Step A success path: emit_perf_segmentation populated
	// Mutable.PerfSegments. Return cleanly so the controller can
	// drive Step B.
	if segs := ctx.Mutable.PerfSegments(); len(segs) > 0 {
		return &StageOutput{StageReport: "perf segmentation recorded"}, nil
	}
	// Single-shot / Step-B success path: a PerfBundle was stored.
	bundle := ctx.Mutable.PerfTrace()
	if bundle != nil {
		return &StageOutput{
			StageReport: renderPerfTriageStageReport(bundle),
		}, nil
	}

	// No bundle and no segments. Check whether either emit was
	// attempted at all.
	hasEmitAttempt := false
	for _, r := range toolResults {
		if r.ToolName == "emit_perf_trace" || r.ToolName == "emit_perf_segmentation" {
			hasEmitAttempt = true
			break
		}
	}
	if !hasEmitAttempt {
		return &StageOutput{
			Error: "perf_triager did not call emit_perf_trace or emit_perf_segmentation within the ReAct loop",
		}, nil
	}
	var rejections []string
	for _, r := range toolResults {
		if r.ToolName != "emit_perf_trace" && r.ToolName != "emit_perf_segmentation" {
			continue
		}
		if r.Success {
			continue
		}
		rejections = append(rejections, r.Summary)
	}
	return &StageOutput{
		Error: "perf_triager emit rejected: " + strings.Join(rejections, "; "),
	}, nil
}

// DetermineMissingPiece — perf triage is a fact-producing pre-stage.
func (e *perfTriagerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// perfTriager is the agent wrapper around BaseAgent that plugs the
// evaluator + gates into the orchestrator pipeline.
type perfTriager struct {
	base     *BaseAgent
	settings PerfTriageSettings
	deps     *Dependencies
}

// Name satisfies Agent.
func (a *perfTriager) Name() types.AgentName { return types.AgentPerfTriager }

// Execute runs the perf_triage stage. Mirrors logTriager.Execute:
// gate → straight-to-two-step on size → single-shot → conditional
// two-step escalation on coverage. Skipped + advisory degrade paths
// match logTriager so the orchestrator's pre-stage handling is
// symmetric.
func (a *perfTriager) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	if !a.settings.Enabled {
		return a.skipped(ctx, "perf_triage_enabled=false"), nil
	}
	if ctx == nil || ctx.AttachedHitrace == "" {
		return a.skipped(ctx, "AttachedHitrace empty"), nil
	}
	if len(ctx.AttachedHitrace) < a.settings.MinBytes {
		return a.skipped(ctx, fmt.Sprintf("trace size %d below min %d",
			len(ctx.AttachedHitrace), a.settings.MinBytes)), nil
	}

	// Straight-to-two-step threshold.
	if a.settings.TwoStepEnabled && len(ctx.AttachedHitrace) >= a.settings.TwoStepBytes {
		logging.Info("[perf_triage] dispatching two-step: bytes=%d ≥ threshold=%d",
			len(ctx.AttachedHitrace), a.settings.TwoStepBytes)
		return a.runTwoStep(ctx, sk, "oversized")
	}

	// Single-shot attempt.
	out, err := a.base.Execute(ctx, sk)
	if err != nil {
		return out, err
	}
	bundle := perfBundleFromCtx(ctx)
	if bundle != nil && !a.shouldEscalate(bundle) {
		return out, nil
	}
	if !a.settings.TwoStepEnabled {
		return out, nil
	}
	reason := "bundle_nil"
	if bundle != nil {
		reason = fmt.Sprintf("coverage=%.2f < threshold=%.2f",
			bundle.Coverage, a.settings.TwoStepCoverage)
		ctx.Mutable.SetPerfTrace(nil)
	}
	logging.Info("[perf_triage] escalating to two-step: reason=%s", reason)
	return a.runTwoStep(ctx, sk, reason)
}

// skipped returns a no-op StageOutput.
func (a *perfTriager) skipped(_ *types.AgentContext, reason string) *StageOutput {
	logging.Info("[perf_triage] skipped: %s", reason)
	return &StageOutput{StageReport: "perf_triage skipped: " + reason}
}

// shouldEscalate reports whether the single-shot bundle's Coverage
// is below the threshold.
func (a *perfTriager) shouldEscalate(b *types.PerfBundle) bool {
	if b == nil {
		return true
	}
	return b.Coverage < a.settings.TwoStepCoverage
}

// runTwoStep implements Step A (segmentation) + Step B (per-segment
// extraction) + MergePerfBundles. Returns a StageOutput; on any
// internal failure falls back to whatever bus.PerfTrace holds.
//
// The two-step controller is structurally identical to
// log_triager.runTwoStep — same shape, different tool names + skill
// + bundle type — kept inline (instead of factored) so each channel
// can evolve independently when the LLM ergonomics diverge.
func (a *perfTriager) runTwoStep(ctx *types.AgentContext, _ *skill.Config, reason string) (*StageOutput, error) {
	// Look up the segmentation skill from the registry.
	segSkill, err := a.skillByName("perf-segmentation-skill")
	if err != nil {
		logging.Warning("[perf_triage] two-step: segmentation skill missing: %v", err)
		return &StageOutput{
			StageReport: "two-step skipped (segmentation skill missing) — degraded",
		}, nil
	}

	// Step A: emit_perf_segmentation. Reset bus state so the
	// segmenter writes onto a fresh slot.
	ctx.Mutable.SetPerfTrace(nil)
	ctx.Mutable.SetPerfSegments(nil)
	a.base.eval.(*perfTriagerEvaluator).emitSeen = false
	if _, segErr := a.base.Execute(ctx, segSkill); segErr != nil {
		logging.Warning("[perf_triage] two-step: segmentation dispatch failed: %v", segErr)
		return &StageOutput{StageReport: fmt.Sprintf("two-step segmentation failed: %v", segErr)}, nil
	}
	segBytes := ctx.Mutable.PerfSegments()
	if len(segBytes) == 0 {
		logging.Info("[perf_triage] two-step: segmenter produced no usable segments (reason=%s)", reason)
		return &StageOutput{StageReport: "two-step produced no segments — degraded"}, nil
	}
	var segments []tool.PerfSegment
	if err := json.Unmarshal(segBytes, &segments); err != nil {
		logging.Warning("[perf_triage] two-step: malformed segments payload: %v", err)
		return &StageOutput{StageReport: "two-step malformed segmentation — degraded"}, nil
	}

	// Step B: dispatch emit_perf_trace per actionable segment.
	perSegBudget := a.settings.MaxLLMCalls - 2
	if perSegBudget <= 0 {
		perSegBudget = 1
	}
	partials := make([]*types.PerfBundle, 0, len(segments))
	calls := 0
	origTrace := ctx.AttachedHitrace
	rawBytes := len(origTrace)

	// Look up the extraction skill (the same perf-triage-skill used
	// in single-shot).
	extractSkill, err := a.skillByName("perf-triage-skill")
	if err != nil {
		logging.Warning("[perf_triage] two-step: extraction skill missing: %v", err)
		return &StageOutput{StageReport: "two-step skipped (extraction skill missing) — degraded"}, nil
	}

	for _, s := range segments {
		if calls >= perSegBudget {
			logging.Info("[perf_triage] two-step budget reached at %d calls; stopping fan-out", calls)
			break
		}
		if !tool.IsExtractablePerfSegment(s.Kind) {
			continue
		}
		// Narrow the trace window for this sub-dispatch. Like log
		// triage, the AgentContext.AttachedHitrace field is per-
		// view; the prompt builder formats it directly so slicing
		// here directs the LLM to one segment at a time.
		ctx.AttachedHitrace = origTrace[s.ByteStart:s.ByteEnd]
		ctx.Mutable.SetPerfTrace(nil)
		a.base.eval.(*perfTriagerEvaluator).emitSeen = false

		subOut, subErr := a.base.Execute(ctx, extractSkill)
		calls++
		if subErr != nil || subOut == nil || subOut.Error != "" {
			logging.Warning("[perf_triage] two-step: segment %s [%d:%d] emit failed: %v",
				s.Kind, s.ByteStart, s.ByteEnd, subErr)
			continue
		}
		if partial := ctx.Mutable.PerfTrace(); partial != nil {
			partials = append(partials, partial)
		}
	}

	// Restore full trace + clear segments so the analyzer's prompt
	// downstream sees the canonical attachment.
	ctx.AttachedHitrace = origTrace
	ctx.Mutable.SetPerfSegments(nil)

	if len(partials) == 0 {
		logging.Info("[perf_triage] two-step: no segments produced bundles")
		return &StageOutput{StageReport: "two-step produced no per-segment bundles — degraded"}, nil
	}

	merged := perftriage.MergePerfBundles(partials, rawBytes)
	if merged == nil {
		return &StageOutput{StageReport: "two-step merge produced nil bundle — degraded"}, nil
	}
	ctx.Mutable.SetPerfTrace(merged)
	return &StageOutput{StageReport: renderPerfTriageStageReport(merged)}, nil
}

// skillByName looks up a skill from deps.Skills, returning the
// configured skill or an error. Mirror of log_triager's helper —
// kept package-local so each agent has a contained surface.
func (a *perfTriager) skillByName(name string) (*skill.Config, error) {
	if a.deps == nil || a.deps.Skills == nil {
		return nil, fmt.Errorf("perf_triager: no skill registry on deps")
	}
	return a.deps.Skills.Get(name)
}

// perfBundleFromCtx is a shorthand for ctx.Mutable.PerfTrace() with
// nil-checks. Pulled out for symmetry with the log_triager pattern.
func perfBundleFromCtx(ctx *types.AgentContext) *types.PerfBundle {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.PerfTrace()
}

// NewPerfTriagerAgent constructs the perf_triager with a BaseAgent
// ReAct loop and the two-step controller. settings carries the
// runtime tuning; a zero-value settings struct inherits defaults.
func NewPerfTriagerAgent(deps *Dependencies, settings PerfTriageSettings) Agent {
	if (settings == PerfTriageSettings{}) {
		settings = DefaultPerfTriageSettings()
	}
	eval := &perfTriagerEvaluator{settings: settings}
	d := *deps
	cap := d.AgentSettings.PerfTriagerIterCap
	if cap <= 0 {
		cap = 6
	}
	if d.MaxIterations == 0 || d.MaxIterations > cap {
		d.MaxIterations = cap
	}
	base := NewBaseAgent(types.AgentPerfTriager, &d, eval)
	return &perfTriager{base: base, settings: settings, deps: &d}
}

// renderPerfTriageStageReport renders a compact digest of a
// PerfBundle for StageReports. Pure-prose observability only; the
// analyzer consumes the structured bundle directly.
func renderPerfTriageStageReport(b *types.PerfBundle) string {
	if b == nil {
		return "no perf bundle produced"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Source: %s\n", b.Meta.Source)
	if b.Meta.DurationMs > 0 {
		fmt.Fprintf(&sb, "Duration: %.1fms\n", b.Meta.DurationMs)
	}
	if len(b.Meta.Signals) > 0 {
		fmt.Fprintf(&sb, "Signals: %s\n", strings.Join(b.Meta.Signals, ", "))
	}
	if b.Meta.Summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n", b.Meta.Summary)
	}
	fmt.Fprintf(&sb, "Frames: %d  Janks: %d  Stalls: %d\n",
		len(b.Frames), len(b.Janks), len(b.Stalls))
	for i, j := range b.Janks {
		if i >= 3 {
			fmt.Fprintf(&sb, "  … +%d more janks\n", len(b.Janks)-3)
			break
		}
		fmt.Fprintf(&sb, "  jank[%d] %.1fms trigger=%s reason=%s\n",
			i, j.DurationMs, j.TriggerSpan, j.Reason)
	}
	for i, s := range b.Stalls {
		if i >= 3 {
			fmt.Fprintf(&sb, "  … +%d more stalls\n", len(b.Stalls)-3)
			break
		}
		fmt.Fprintf(&sb, "  stall[%d] %.1fms kind=%s sym=%s\n",
			i, s.DurationMs, s.Kind, s.Symbol)
	}
	if b.Startup != nil {
		fmt.Fprintf(&sb, "Startup: mode=%s launch=%.1fms firstFrame=%.1fms\n",
			b.Startup.Mode, b.Startup.AppLaunchMs, b.Startup.FirstFrameMs)
	}
	if len(b.ResolvedFiles) > 0 {
		fmt.Fprintf(&sb, "ResolvedFiles: %s\n", strings.Join(b.ResolvedFiles, ", "))
	}
	if len(b.Entities) > 0 {
		fmt.Fprintf(&sb, "Entities: %s\n", strings.Join(b.Entities, ", "))
	}
	fmt.Fprintf(&sb, "IntentHint: %s\n", b.IntentHint)
	return sb.String()
}
