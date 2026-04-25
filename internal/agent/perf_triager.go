package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// perfTriagerEvaluator is the Evaluator implementation for the
// perf_triager agent. Mirrors logTriagerEvaluator's shape but
// single-shot — the perf_triage stage does not implement a two-step
// segmentation fallback (HiTrace excerpts are structurally simpler
// than multi-goroutine panic dumps: a single emit covers the whole
// trace or fails loud).
//
// The ReAct loop terminates as soon as emit_perf_trace succeeds; a
// failed emit lets the loop continue within MaxIterations so the LLM
// can correct on the next turn.
type perfTriagerEvaluator struct {
	emitSeen bool
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

// Observe watches mid-loop tool results; a successful emit_perf_trace
// flips emitSeen and requests loop termination.
func (e *perfTriagerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || obs.LastToolResult == nil {
		return LoopSignal{}
	}
	if !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	if obs.LastToolResult.ToolName == "emit_perf_trace" {
		e.emitSeen = true
		return LoopSignal{StopRequested: true, StopReason: "emit_perf_trace emitted"}
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
	bundle := ctx.Mutable.PerfTrace()
	if bundle != nil {
		return &StageOutput{
			StageReport: renderPerfTriageStageReport(bundle),
		}, nil
	}

	// Check if any emit was attempted at all.
	hasEmitAttempt := false
	for _, r := range toolResults {
		if r.ToolName == "emit_perf_trace" {
			hasEmitAttempt = true
			break
		}
	}
	if !hasEmitAttempt {
		return &StageOutput{
			Error: "perf_triager did not call emit_perf_trace within the ReAct loop",
		}, nil
	}
	var rejections []string
	for _, r := range toolResults {
		if r.ToolName != "emit_perf_trace" {
			continue
		}
		if r.Success {
			continue
		}
		rejections = append(rejections, r.Summary)
	}
	return &StageOutput{
		Error: "perf_triager emit_perf_trace rejected: " + strings.Join(rejections, "; "),
	}, nil
}

// DetermineMissingPiece — perf triage is a fact-producing pre-stage.
func (e *perfTriagerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// perfTriager is the agent wrapper around BaseAgent that plugs the
// evaluator + gates into the orchestrator pipeline.
type perfTriager struct {
	base *BaseAgent
}

// Name satisfies Agent.
func (a *perfTriager) Name() types.AgentName { return types.AgentPerfTriager }

// Execute runs the perf_triage stage. Short-circuits to skipped when
// AttachedHitrace is empty — identical pre-stage contract as
// log_triager, so the orchestrator's advisory-failure handling treats
// it symmetrically.
func (a *perfTriager) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	if ctx == nil || ctx.AttachedHitrace == "" {
		logging.Info("[perf_triage] skipped: AttachedHitrace empty")
		return &StageOutput{StageReport: "perf_triage skipped: no trace attached"}, nil
	}
	return a.base.Execute(ctx, sk)
}

// NewPerfTriagerAgent constructs the perf_triager with a short ReAct
// loop. Cap at 4 iterations: the emit is either valid on first try,
// corrected on the second, or we give up and the stage degrades
// advisorily (pipeline continues with bus.PerfTrace()==nil).
func NewPerfTriagerAgent(deps *Dependencies) Agent {
	eval := &perfTriagerEvaluator{}
	d := *deps
	if d.MaxIterations == 0 || d.MaxIterations > 4 {
		d.MaxIterations = 4
	}
	base := NewBaseAgent(types.AgentPerfTriager, &d, eval)
	return &perfTriager{base: base}
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
