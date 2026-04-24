package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// plannerEvaluator drives the planner agent's single-emit ReAct loop
// for B0 write-mode plan stage. The agent's one structured
// contribution is a call to emit_change_plan, which installs the
// ChangePlan on BusContext.Mutable. ParseOutput reads the installed
// plan and returns a success StageOutput; when the LLM fails to emit
// the plan within the loop cap, a clean error surfaces upstream so
// runPlanPhase writes a diagnostic to TaskState.LastError instead of
// a silent no-op.
//
// Shape is closest to the finalizer's answerDocumentEvaluator: one
// tool is expected, success is signaled by a populated slot on
// Mutable, and the loop should terminate as soon as the slot fills.
// Unlike the finalizer, planner does not support correction retries
// in B0 — if the first emit_change_plan call is rejected by the
// tool's schema validator, ParseOutput reports the failure instead
// of nudging for a retry. B1 will add retry-hint semantics once the
// plan content can be richer (open-question #1).
//
// Red-line discipline (L3): plannerEvaluator MUST NOT reference the
// ground.* package. Plan content is a structured proposal, not
// citation-bearing evidence; grounding has no meaning here.
type plannerEvaluator struct {
	// mu is captured in BuildInitialInstruction so ShouldStop can
	// check whether the emit_change_plan tool has installed a plan
	// without needing ctx at every Evaluator entry. Mirror of
	// answerDocumentEvaluator.mu.
	mu *types.MutableState
}

// BuildInitialInstruction captures the Mutable pointer for later
// ShouldStop inspection and returns no dynamic supplement — the
// skill's Workflow + Goal + Prohibitions are self-contained for
// the plan stage (the agent knows its job from the skill).
func (e *plannerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	_ = sk
	if ctx != nil {
		e.mu = ctx.Mutable
	}
	return ""
}

// ShouldStop terminates the ReAct loop as soon as a ChangePlan has
// been installed on Mutable (emit_change_plan's happy path) or when
// the loop has burned through its defensive iteration cap. The cap
// is kept intentionally small (3) because plan-stage LLM calls are
// expected to be single-shot; a runaway loop is a sign of a broken
// skill prompt that should surface as a fail-loud error upstream.
func (e *plannerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	_ = resp
	if e.mu != nil && e.mu.ChangePlan() != nil {
		return true
	}
	// Defensive cap: without a plan after 3 iterations the skill /
	// LLM is deadlocked. ParseOutput will then return a clean error.
	return iteration >= 3
}

// ParseOutput reads the installed ChangePlan, or reports a clean
// failure if emit_change_plan was never called successfully. The
// returned StageOutput carries MissingPiece (required by
// orchestrator's decision path) and an Error field when the
// emission failed.
func (e *plannerEvaluator) ParseOutput(
	ctx *types.AgentContext,
	messages []llm.Message,
	toolResults []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	_ = messages
	out := &StageOutput{}

	var plan *types.ChangePlan
	if ctx != nil && ctx.Mutable != nil {
		plan = ctx.Mutable.ChangePlan()
	}

	if plan == nil {
		// Scan the tool results for the most recent failed
		// emit_change_plan to surface a useful error to upstream.
		reason := "planner did not call emit_change_plan"
		for i := len(toolResults) - 1; i >= 0; i-- {
			tr := toolResults[i]
			if tr.ToolName == "emit_change_plan" && !tr.Success {
				reason = "planner's emit_change_plan call was rejected: " + tr.Summary
				break
			}
		}
		logging.Warning("[planner] %s", reason)
		out.MissingPiece = types.MissingFacts
		out.Error = reason
		return out, fmt.Errorf("%s", reason)
	}

	logging.Info("[planner] plan installed: id=%s changes=%d target_paths=%d",
		plan.ID, len(plan.Changes), len(plan.TargetPaths))
	out.MissingPiece = types.MissingNone
	return out, nil
}

// DetermineMissingPiece returns MissingNone on success (plan emitted)
// and MissingFacts when the emit failed — matching ParseOutput's
// decision so upstream code can observe a consistent signal.
func (e *plannerEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewPlannerAgent constructs the B0 plan-stage agent. Wiring
// mirrors NewFinalizerAgent: a single Evaluator impl on top of
// BaseAgent's ReAct loop. The change-plan-skill provides the
// prompt and ToolSuggestions (read_file / grep / list_files /
// repo_map / exec_command / emit_change_plan).
func NewPlannerAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentPlanner, deps, &plannerEvaluator{})
}
