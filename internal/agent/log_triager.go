package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// LogTriageSettings carries the runtime tuning for the log_triager
// agent. Wired from config.RuntimeSettings in cmd/root.go. Zero
// values yield the conservative defaults documented on each field.
type LogTriageSettings struct {
	// Enabled gates the whole feature. When false, the log_triage
	// stage Guard still fires (for observability) but Execute
	// short-circuits with a "skipped: feature disabled" stage output.
	// Default: true.
	Enabled bool

	// MinBytes is the lower bound for stage invocation. When
	// len(AttachedLog) < MinBytes, the stage Guard in topology.go
	// returns false and the pre-stage is skipped. Very short logs
	// rarely contain extractable frames; the LLM call cost outweighs
	// the benefit. Default: 50.
	MinBytes int

	// MaxRetries bounds the fail-loud retry budget for a single-shot
	// emit_log_triage that rejects on schema / cross-field validation.
	// 0 = never retry (just degrade). Default: 1.
	MaxRetries int

	// TwoStepEnabled turns the two-step fallback on or off. When
	// false, low-coverage or oversized logs take the single-shot
	// result (or nil bundle on failure) without attempting
	// segmentation. Default: true.
	TwoStepEnabled bool

	// TwoStepBytes is the log-size threshold above which the
	// controller goes straight to two-step (bypassing single-shot).
	// Chosen conservatively: single-shot handles short panics; only
	// genuinely long polyglot / multi-cycle logs pay the segmentation
	// cost. Default: 32 * 1024 (32 KB).
	TwoStepBytes int

	// TwoStepCoverage is the coverage threshold below which a
	// single-shot bundle triggers two-step escalation. Coverage is
	// 1 - bytes(unknown_chunks)/bytes(rawLog). Default: 0.3.
	TwoStepCoverage float64

	// MaxLLMCalls caps the total LLM invocations for one log_triage
	// stage run across both steps (1 single-shot + 1 segment + N
	// per-segment). Defence against a runaway segmentation that
	// produces many tiny segments. Default: 12.
	//
	// Default raised from 8 → 12 in 2026-04 alongside the
	// log_attach_max_bytes default bump (1 MB → 50 MB). Rationale:
	// the segmenter skill caps emit_log_segmentation at 10 segments,
	// so a budget of 12 = 1 single-shot + 1 segmentation + 10
	// per-segment calls covers every segment the LLM is allowed to
	// emit. The previous 8 capped per-segment to 6, silently dropping
	// the last 4 segments on big captures.
	MaxLLMCalls int
}

// DefaultLogTriageSettings returns the conservative defaults.
// cmd/root.go fills missing fields from this function before agent
// construction so both the CLI and tests see the same baseline.
func DefaultLogTriageSettings() LogTriageSettings {
	return LogTriageSettings{
		Enabled:         true,
		MinBytes:        50,
		MaxRetries:      1,
		TwoStepEnabled:  true,
		TwoStepBytes:    32 * 1024,
		TwoStepCoverage: 0.3,
		MaxLLMCalls:     12,
	}
}

// logTriagerEvaluator is the Evaluator implementation for the
// log_triager agent. The ReAct loop is short and single-purpose:
// the agent reads the attached log, optionally paginates via
// read_file on the blob, and calls emit_log_triage once. ShouldStop
// terminates the loop when the emit succeeds.
//
// The two-step controller lives on the evaluator as well: after the
// first ParseOutput, if the resulting bundle is nil or its Coverage
// is below the configured floor AND the two-step path is enabled,
// the caller (BaseAgent.Execute) does NOT automatically re-dispatch —
// two-step is handled by log_triager.go's ExecuteWithRetry wrapper,
// which layers around the standard Execute. See dispatchTwoStep for
// the segmentation + per-segment extraction + merge path.
type logTriagerEvaluator struct {
	settings  LogTriageSettings
	emitSeen  bool // flipped true after a successful emit_log_triage
	llmCalls  int  // counter toward settings.MaxLLMCalls
}

// BuildInitialInstruction — the skill carries everything structural;
// there is no per-dispatch dynamic supplement beyond the attached log
// itself, which the prompt builder renders as a first-class user
// section (see internal/context/builder.go::formatAttachedLog).
// Returning "" keeps the Skill / Evaluator boundary clean (see
// docs/architecture.md §3.3).
func (e *logTriagerEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	e.emitSeen = false
	return ""
}

// ShouldStop terminates the ReAct loop as soon as the LLM calls
// emit_log_triage (or emit_log_segmentation in two-step mode).
// The Observe path flips emitSeen on successful emit; this hook is
// the secondary check that BaseAgent consults before starting the
// next iteration.
func (e *logTriagerEvaluator) ShouldStop(_ llm.Response, _ int) bool {
	return e.emitSeen
}

// Observe watches mid-loop tool results to detect the emit. A
// successful emit_log_triage (or emit_log_segmentation) flips
// emitSeen; the next iteration's ShouldStop terminates the loop.
// A failed emit lets the loop continue — the LLM sees the
// rejection reason in the tool result and can correct on the next
// iteration (within MaxRetries).
func (e *logTriagerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || obs.LastToolResult == nil {
		return LoopSignal{}
	}
	if !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	switch obs.LastToolResult.ToolName {
	case "emit_log_triage", "emit_log_segmentation":
		e.emitSeen = true
		return LoopSignal{StopRequested: true, StopReason: obs.LastToolResult.ToolName + " emitted"}
	}
	return LoopSignal{}
}

// ParseOutput produces the StageOutput from the ReAct loop's accumulated
// state. When the LLM called emit_log_triage, Mutable.LogTriage() is
// already populated (the tool's Execute did the write) and this
// function just reports success. When no emit happened, the stage
// fails loud — runAnalyzePhase-style retry applies.
func (e *logTriagerEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	if ctx == nil || ctx.Mutable == nil {
		return &StageOutput{
			Error: "log_triager requires a writable context; the caller did not provide one",
		}, nil
	}

	// Detect a successful emit via Mutable.LogTriage() rather than
	// walking tool results — the emit tool is the sole writer and
	// presence of a bundle is the clean success signal.
	bundle := ctx.Mutable.LogTriage()
	if bundle != nil {
		return &StageOutput{
			StageReport: renderLogTriageStageReport(bundle),
		}, nil
	}

	// No bundle. Check whether any emit tool was even attempted.
	hasEmitAttempt := false
	for _, r := range toolResults {
		if r.ToolName == "emit_log_triage" || r.ToolName == "emit_log_segmentation" {
			hasEmitAttempt = true
			break
		}
	}
	if !hasEmitAttempt {
		return &StageOutput{
			Error: "log_triager did not call emit_log_triage within the ReAct loop",
		}, nil
	}
	// Emit attempted but all calls rejected. The rejection reason is
	// already in tool results; StageOutput.Error summarises so the
	// retry-hint builder can surface it.
	var rejections []string
	for _, r := range toolResults {
		if r.ToolName != "emit_log_triage" && r.ToolName != "emit_log_segmentation" {
			continue
		}
		if r.Success {
			continue
		}
		rejections = append(rejections, r.Summary)
	}
	return &StageOutput{
		Error: "log_triager emit_log_triage rejected: " + strings.Join(rejections, "; "),
	}, nil
}

// DetermineMissingPiece — log_triage is a fact-producing pre-stage,
// so missing work is "facts" in every case. The orchestrator does
// not branch on this value for pre-stages; returning the most
// semantic value keeps future consumers consistent.
func (e *logTriagerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// renderLogTriageStageReport produces a compact human-readable
// digest of the bundle for StageReports. The analyzer's prompt does
// NOT render this (it reads the structured bundle directly); this
// is purely for operator observability when grepping a trace.
func renderLogTriageStageReport(b *types.LogBundle) string {
	if b == nil {
		return "no log bundle produced"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Lang: %s\n", b.Meta.Lang)
	if len(b.Meta.Signals) > 0 {
		sigs := make([]string, len(b.Meta.Signals))
		for i, s := range b.Meta.Signals {
			sigs[i] = string(s)
		}
		fmt.Fprintf(&sb, "Signals: %s\n", strings.Join(sigs, ", "))
	}
	if b.Meta.Summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n", b.Meta.Summary)
	}
	fmt.Fprintf(&sb, "Errors: %d\n", len(b.Errors))
	for i, e := range b.Errors {
		fmt.Fprintf(&sb, "  [%d] %s — %s (frames=%d)\n",
			i, e.Type, truncateStr(e.Message, 80), len(e.Frames))
		c := e.Cause
		depth := 1
		for c != nil {
			fmt.Fprintf(&sb, "      ↳ caused by %s (frames=%d)\n",
				c.Type, len(c.Frames))
			c = c.Cause
			depth++
			if depth > types.LogBundleCaps.MaxCauseDepth {
				break
			}
		}
	}
	if len(b.ResolvedFiles) > 0 {
		fmt.Fprintf(&sb, "ResolvedFiles: %s\n", strings.Join(b.ResolvedFiles, ", "))
	}
	if len(b.Entities) > 0 {
		fmt.Fprintf(&sb, "Entities: %s\n", strings.Join(b.Entities, ", "))
	}
	fmt.Fprintf(&sb, "IntentHint: %s\n", b.IntentHint)
	fmt.Fprintf(&sb, "Coverage: %.2f\n", b.Coverage)
	return sb.String()
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── Two-step controller ─────────────────────────────────────────

// logTriager is the public agent type. It wraps BaseAgent so the
// orchestrator's dispatch table can look it up, and it layers the
// two-step controller around BaseAgent.Execute. The controller
// runs in three modes:
//
//   - len(AttachedLog) >= TwoStepBytes: go straight to two-step.
//   - Otherwise: single-shot. If the resulting bundle's Coverage
//     is below TwoStepCoverage and two-step is enabled, escalate.
//   - If the stage is disabled or the log is below MinBytes, skip.
//
// All three paths return a StageOutput that the orchestrator applies
// via applyStageOutput. Failure (bundle nil after both paths) is
// non-fatal — the orchestrator logs a warning and proceeds with the
// main 4-stage pipeline; bus.LogTriage() returns nil and every
// downstream consumer's nil-check degrades cleanly.
type logTriager struct {
	base     *BaseAgent
	settings LogTriageSettings
}

// Name satisfies Agent.
func (a *logTriager) Name() types.AgentName { return types.AgentLogTriager }

// Execute runs the full log-triage flow: gate → single-shot → optional
// two-step escalation. The staged dispatch returns the final StageOutput.
func (a *logTriager) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	if !a.settings.Enabled {
		return a.skipped(ctx, "log_triage_enabled=false"), nil
	}
	if ctx == nil || ctx.AttachedLog == "" {
		return a.skipped(ctx, "AttachedLog empty"), nil
	}
	if len(ctx.AttachedLog) < a.settings.MinBytes {
		return a.skipped(ctx, fmt.Sprintf("log size %d below min %d",
			len(ctx.AttachedLog), a.settings.MinBytes)), nil
	}

	// Straight-to-two-step threshold.
	if a.settings.TwoStepEnabled && len(ctx.AttachedLog) >= a.settings.TwoStepBytes {
		logging.Info("[log_triage] dispatching two-step: bytes=%d ≥ threshold=%d",
			len(ctx.AttachedLog), a.settings.TwoStepBytes)
		return a.runTwoStep(ctx, sk, "oversized")
	}

	// Single-shot attempt.
	out, err := a.base.Execute(ctx, sk)
	if err != nil {
		return out, err
	}
	bundle := triageBundle(ctx)

	if bundle != nil && !a.shouldEscalate(bundle) {
		return out, nil
	}
	if !a.settings.TwoStepEnabled {
		// Two-step off. If single-shot produced a bundle (any coverage),
		// keep it. If it produced nothing, the stage degrades.
		return out, nil
	}
	reason := "bundle_nil"
	if bundle != nil {
		reason = fmt.Sprintf("coverage=%.2f < threshold=%.2f",
			bundle.Coverage, a.settings.TwoStepCoverage)
		// Clear the single-shot bundle so two-step's merge starts fresh.
		ctx.Mutable.SetLogTriage(nil)
	}
	logging.Info("[log_triage] escalating to two-step: reason=%s", reason)
	return a.runTwoStep(ctx, sk, reason)
}

// skipped returns a no-op StageOutput that marks the stage as
// skipped. The orchestrator's applyStageOutput treats an empty
// Findings + empty Error as "nothing to do".
func (a *logTriager) skipped(_ *types.AgentContext, reason string) *StageOutput {
	logging.Info("[log_triage] skipped: %s", reason)
	return &StageOutput{
		StageReport: "log_triage skipped: " + reason,
	}
}

// shouldEscalate reports whether the single-shot bundle's Coverage
// is below the threshold.
func (a *logTriager) shouldEscalate(b *types.LogBundle) bool {
	if b == nil {
		return true
	}
	return b.Coverage < a.settings.TwoStepCoverage
}

// runTwoStep implements Step A (segmentation) + Step B (per-segment
// extraction) + Merge. Returns a StageOutput; on any internal failure
// falls back to whatever bus.LogTriage holds (may be nil → degrade).
func (a *logTriager) runTwoStep(ctx *types.AgentContext, sk *skill.Config, reason string) (*StageOutput, error) {
	// Count the single-shot call if one already ran.
	e := a.base.eval.(*logTriagerEvaluator)
	_ = e // budget tracking is managed by MaxLLMCalls check below

	// Step A: segmentation. Swap the skill to log-segmentation-skill
	// for this dispatch. We don't mutate the input sk — we look up
	// from the registry in the caller's normal dispatch path, so
	// the two-step controller constructs a local skill resolver.
	segSkill, err := lookupSkillSafe(a.base, "log-segmentation-skill")
	if err != nil {
		logging.Warning("[log_triage] two-step: log-segmentation-skill missing: %v (falling back to single-shot result)", err)
		return &StageOutput{
			StageReport: fmt.Sprintf("two-step abandoned (skill missing) — degraded via reason=%s", reason),
		}, nil
	}

	// Reset the emit flag on the evaluator so Step A's emit triggers
	// the loop stop independently of any prior state.
	e.emitSeen = false

	// Dispatch Step A. This uses BaseAgent.Execute with the
	// segmentation skill. The tool call in Step A is
	// emit_log_segmentation which writes to Mutable.LogSegments().
	segOut, err := a.base.Execute(ctx, segSkill)
	if err != nil || segOut == nil {
		// Commit 58 Batch D fix (audit #1): include the underlying
		// emit_log_segmentation rejection reason in StageReport so
		// downstream operators / diagnostic logs see *why* the
		// segmentation step refused. Pre-commit-58 the err was logged
		// once at WARN and then swallowed.
		errMsg := "no error returned"
		if err != nil {
			errMsg = err.Error()
		}
		logging.Warning("[log_triage] two-step: segmentation failed: %v", err)
		return &StageOutput{
			StageReport: fmt.Sprintf("two-step segmentation failed — degraded via reason=%s; underlying: %s",
				reason, errMsg),
		}, nil
	}
	segBytes := ctx.Mutable.LogSegments()
	if len(segBytes) == 0 {
		logging.Warning("[log_triage] two-step: no segments emitted; degrading")
		return &StageOutput{
			StageReport: fmt.Sprintf("two-step segmentation produced no segments — degraded via reason=%s (segmenter ran but its emit_log_segmentation tool yielded zero structured segments; check if the input is unstructured noise that the segmenter cannot decompose)",
				reason),
		}, nil
	}
	var segments []tool.LogSegment
	if err := json.Unmarshal(segBytes, &segments); err != nil {
		logging.Warning("[log_triage] two-step: malformed segments payload: %v", err)
		return &StageOutput{
			StageReport: "two-step malformed segmentation — degraded",
		}, nil
	}

	// Step B: run emit_log_triage per stack-shaped segment.
	// Budget: 1 single-shot + 1 segment + up to (MaxLLMCalls - 2)
	// per-segment dispatches.
	perSegBudget := a.settings.MaxLLMCalls - 2
	if perSegBudget <= 0 {
		perSegBudget = 1
	}
	partials := make([]*types.LogBundle, 0, len(segments))
	calls := 0
	origLog := ctx.AttachedLog
	for _, s := range segments {
		if calls >= perSegBudget {
			logging.Info("[log_triage] two-step budget reached at %d calls; stopping fan-out", calls)
			break
		}
		if !isExtractableSegment(s.Kind) {
			continue
		}
		// Narrow the log window for this sub-dispatch. The AttachedLog
		// field is per-AgentContext-view; the BaseAgent's prompt
		// builder uses ctx.AttachedLog, so slicing here directs the
		// LLM's attention to one segment at a time.
		ctx.AttachedLog = origLog[s.ByteStart:s.ByteEnd]
		// Clear any prior bundle so the emit lands on a fresh slot.
		ctx.Mutable.SetLogTriage(nil)
		e.emitSeen = false

		subOut, subErr := a.base.Execute(ctx, sk)
		calls++
		if subErr != nil || subOut == nil || subOut.Error != "" {
			logging.Warning("[log_triage] two-step: segment %s [%d:%d] emit failed: %v",
				s.Kind, s.ByteStart, s.ByteEnd, subErr)
			continue
		}
		if partial := ctx.Mutable.LogTriage(); partial != nil {
			partials = append(partials, partial)
		}
	}
	// Restore the full log so the analyzer downstream sees the
	// complete text in its own prompt.
	ctx.AttachedLog = origLog

	if len(partials) == 0 {
		return &StageOutput{
			StageReport: fmt.Sprintf("two-step produced zero partial bundles (segments=%d) — degraded", len(segments)),
		}, nil
	}

	merged := logtriage.MergeBundles(partials, len(origLog))
	ctx.Mutable.SetLogTriage(merged)
	logging.Info("[log_triage] two-step merged: parts=%d final_errors=%d resolved=%d coverage=%.2f",
		len(partials), len(merged.Errors), len(merged.ResolvedFiles), merged.Coverage)
	return &StageOutput{
		StageReport: renderLogTriageStageReport(merged),
	}, nil
}

// isExtractableSegment reports whether Step B should dispatch
// emit_log_triage on this segment kind. Stack / caused_by / trace
// carry structured stack-frame data; header / context / noise do
// not contain frames and would waste a call.
func isExtractableSegment(kind string) bool {
	switch kind {
	case "stack", "caused_by", "trace":
		return true
	}
	return false
}

// triageBundle is a nil-safe accessor used by the controller to
// read the current bundle state without needing to go through
// BusContext indirection.
func triageBundle(ctx *types.AgentContext) *types.LogBundle {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.LogTriage()
}

// lookupSkillSafe fetches a named skill from the agent's dependency
// graph. Returns an error describing the miss when the skill registry
// was not wired (unit tests that bypass orchestrator construction)
// or the name is unknown. The caller's two-step controller degrades
// via early return on error.
func lookupSkillSafe(base *BaseAgent, name string) (*skill.Config, error) {
	if base == nil || base.deps == nil || base.deps.Skills == nil {
		return nil, fmt.Errorf("skill registry not wired into Dependencies")
	}
	return base.deps.Skills.Get(name)
}

// NewLogTriagerAgent constructs the log_triager agent with a
// BaseAgent ReAct loop and the two-step controller. settings carries
// the runtime tuning; a zero-value settings struct inherits the
// defaults from DefaultLogTriageSettings.
func NewLogTriagerAgent(deps *Dependencies, settings LogTriageSettings) Agent {
	if (settings == LogTriageSettings{}) {
		settings = DefaultLogTriageSettings()
	}
	eval := &logTriagerEvaluator{settings: settings}

	// The log_triager's ReAct loop is short: single emit terminates
	// it. Cap iterations via AgentSettings.LogTriagerIterCap (yaml:
	// agent_log_triager_iter_cap, default 6) so a misbehaving LLM
	// that never calls emit_log_triage cannot burn iterations
	// indefinitely.
	d := *deps
	cap := d.AgentSettings.LogTriagerIterCap
	if cap <= 0 {
		cap = 6
	}
	if d.MaxIterations == 0 || d.MaxIterations > cap {
		d.MaxIterations = cap
	}
	base := NewBaseAgent(types.AgentLogTriager, &d, eval)
	return &logTriager{
		base:     base,
		settings: settings,
	}
}
