package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// writeAnalyzerEvaluator drives the write_analyzer agent. The
// agent's job is to characterise the user's code-change request as
// a structured WriteAnalysisIR via emit_write_analysis. One ReAct
// loop, terminate as soon as a successful emit lands.
//
// Mirror of perfTriagerEvaluator in shape but simpler: no two-step
// fallback (write tasks don't have the size/coverage problem traces
// do). The skill workflow tells the LLM to do 1-2 light pre-scan
// rounds (read_file / list_files / repo_map) to ground its
// classification, then emit.
type writeAnalyzerEvaluator struct {
	emitSeen bool
}

// BuildInitialInstruction: skill owns the structural prompt; the
// only per-dispatch supplement is the prior-attempt-failed hint
// from Mutable.AnalyzerRetryHint, set by runWriteAnalyzePhase
// when an earlier attempt was rejected. Mirrors the read
// analyzer's hint-consumption pattern (analyzer.go:184): consume-
// once via Reset so a stale hint can't leak into a future Run.
//
// Without this, runWriteAnalyzePhase's SetAnalyzerRetryHint call
// in the retry path would be dead code — the hint would be set
// but never surfaced to the LLM, and the second attempt would
// repeat the first's mistake. (Commit 10's #11 plumbing was
// half-wired; commit 13 closes the loop.)
func (e *writeAnalyzerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	e.emitSeen = false
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	hint := ctx.Mutable.AnalyzerRetryHint()
	if hint == "" {
		return ""
	}
	ctx.Mutable.ResetAnalyzerRetryHint()
	return "## Previous attempt rejected\n\n" + hint + "\n\n" +
		"Re-emit emit_write_analysis with the issue above resolved. The skill prompt below describes the schema; this section names what went wrong on the previous attempt so you can correct it directly.\n\n"
}

// ShouldStop — primary gate is the emit observation; this hook is
// the secondary check BaseAgent consults between iterations.
func (e *writeAnalyzerEvaluator) ShouldStop(_ llm.Response, _ int) bool { return e.emitSeen }

// Observe watches mid-loop tool results; a successful
// emit_write_analysis flips emitSeen and requests termination.
func (e *writeAnalyzerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || obs.LastToolResult == nil {
		return LoopSignal{}
	}
	if !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	if obs.LastToolResult.ToolName == "emit_write_analysis" {
		e.emitSeen = true
		return LoopSignal{StopRequested: true, StopReason: "emit_write_analysis emitted"}
	}
	return LoopSignal{}
}

// ParseOutput produces the StageOutput. emit_write_analysis success
// path: WriteAnalysisIR is on Mutable, render a compact stage
// report. Failure paths fail loud so the retry mechanism can apply.
func (e *writeAnalyzerEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	if ctx == nil || ctx.Mutable == nil {
		return &StageOutput{
			Error: "write_analyzer requires a writable context; the caller did not provide one",
		}, nil
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir != nil {
		return &StageOutput{
			StageReport: renderWriteAnalysisStageReport(ir),
		}, nil
	}

	hasEmitAttempt := false
	var rejections []string
	for _, r := range toolResults {
		if r.ToolName != "emit_write_analysis" {
			continue
		}
		hasEmitAttempt = true
		if !r.Success {
			rejections = append(rejections, r.Summary)
		}
	}
	if !hasEmitAttempt {
		return &StageOutput{
			Error: "write_analyzer did not call emit_write_analysis within the ReAct loop",
		}, nil
	}
	return &StageOutput{
		Error: "write_analyzer emit rejected: " + strings.Join(rejections, "; "),
	}, nil
}

// DetermineMissingPiece — the write analyzer is a fact-producing
// pre-stage; downstream missing piece is always "facts".
func (e *writeAnalyzerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

// writeAnalyzer wraps BaseAgent + evaluator into the orchestrator
// Agent contract. No two-step controller, so Execute is just
// BaseAgent.Execute.
type writeAnalyzer struct {
	base *BaseAgent
}

func (a *writeAnalyzer) Name() types.AgentName { return types.AgentWriteAnalyzer }

func (a *writeAnalyzer) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	return a.base.Execute(ctx, sk)
}

const writeAnalyzerIterCap = 6

// NewWriteAnalyzerAgent constructs the write_analyzer with a
// BaseAgent ReAct loop. The write analyzer is a bounded classifier,
// not the heavy code exploration lane; deeper investigation belongs
// to the controller's explore_code action after a durable workflow
// batch exists. Cap the inherited global ReAct budget so the first
// attempt cannot spend the full analyzer budget on planner-level root
// cause work before emit_write_analysis has even produced the typed
// seed.
func NewWriteAnalyzerAgent(deps *Dependencies) Agent {
	d := *deps
	if d.MaxIterations == 0 || d.MaxIterations > writeAnalyzerIterCap {
		d.MaxIterations = writeAnalyzerIterCap
	}
	eval := &writeAnalyzerEvaluator{}
	base := NewBaseAgent(types.AgentWriteAnalyzer, &d, eval)
	return &writeAnalyzer{base: base}
}

// renderWriteAnalysisStageReport produces a short observability
// digest of the IR.
func renderWriteAnalysisStageReport(ir *types.WriteAnalysisIR) string {
	if ir == nil {
		return "no write analysis produced"
	}
	var sb strings.Builder
	sb.WriteString("Task: ")
	sb.WriteString(string(ir.Request.Task.Kind))
	sb.WriteString(" / ")
	sb.WriteString(string(ir.Request.Task.Scope))
	sb.WriteString("\n")
	sb.WriteString("Summary: ")
	sb.WriteString(ir.Request.Task.Summary)
	sb.WriteString("\n")
	sb.WriteString("Risk: ")
	sb.WriteString(string(ir.Request.Risk.Overall))
	if ir.Request.Risk.AffectsPublicAPI {
		sb.WriteString(" (affects public API)")
	}
	if ir.Request.Risk.ChangesPersistence {
		sb.WriteString(" (persistence)")
	}
	if ir.Request.Risk.ChangesBuildSystem {
		sb.WriteString(" (build)")
	}
	sb.WriteString("\n")
	if len(ir.Request.ScopeAnchors) > 0 {
		sb.WriteString("Anchors: ")
		sb.WriteString(strings.Join(ir.Request.ScopeAnchors, ", "))
		sb.WriteString("\n")
	}
	if len(ir.Request.Constraints) > 0 {
		sb.WriteString("Constraints: ")
		labels := make([]string, 0, len(ir.Request.Constraints))
		for _, c := range ir.Request.Constraints {
			labels = append(labels, c.Kind)
		}
		sb.WriteString(strings.Join(labels, ", "))
		sb.WriteString("\n")
	}
	if len(ir.Request.ExpectedOutcomes) > 0 {
		sb.WriteString("Outcomes: ")
		sb.WriteString(strings.Join(ir.Request.ExpectedOutcomes, " | "))
		sb.WriteString("\n")
	}
	if ir.PhaseProposal.Split != "" && ir.PhaseProposal.Split != "single" {
		sb.WriteString("Phases: ")
		sb.WriteString(ir.PhaseProposal.Split)
		sb.WriteString(" (")
		labels := make([]string, 0, len(ir.PhaseProposal.Phases))
		for _, p := range ir.PhaseProposal.Phases {
			labels = append(labels, p.Goal)
		}
		sb.WriteString(strings.Join(labels, " → "))
		sb.WriteString(")\n")
	}
	return sb.String()
}
