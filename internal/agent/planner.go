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
// the plan stage hook writes a diagnostic to TaskState.LastError instead of
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

	// Default iteration caps from AgentSettings — used as the floor
	// when no per-dispatch override has been computed by the
	// orchestrator (e.g. unit tests that bypass orchestrator wiring,
	// or runs where the analyzer's IR is unavailable).
	defaultSoftIterCap int
	defaultHardIterCap int

	// Per-dispatch caps captured in BuildInitialInstruction from
	// ctx.MaxIterOverride. Zero means "no override seen — fall back
	// to the defaults above." This mirrors the per-dispatch
	// MaxIterOverride mechanism the explorer already consumes from
	// AgentContext (see internal/agent/agent.go::maxIter resolution
	// and orchestrator.go's per-dispatch scaling block). The single
	// MaxIterOverride channel carries the orchestrator's complexity-
	// aware budget decision; the planner interprets it as its inner-
	// cap soft floor (recovery slack added on top).
	dispatchSoftIterCap int
	dispatchHardIterCap int
}

// BuildInitialInstruction captures the Mutable pointer + per-dispatch
// iteration cap override for later ShouldStop inspection, and returns
// the PlanningHint supplement when the orchestrator's verify→plan
// retry loop (B2.3) has seeded one. The hint carries failure context
// from the previous ChangeReport so the planner knows what to avoid
// on this retry. When no hint is set (first attempt, or retry
// disabled), returns empty string and the skill's Workflow/Goal/
// Prohibitions drive the dispatch unchanged.
//
// PlannerSoftIterCapOverride consumption: the orchestrator's
// per-dispatch scaling block (orchestrator.go's StagePlan branch)
// writes the analyzer-complexity-scaled soft cap to
// ctx.PlannerSoftIterCapOverride. The hard cap is derived as soft +
// the agent-settings recovery slack (default hard - default soft).
// Zero override = no analyzer signal available → fall back to
// agent-settings defaults so unit tests and degraded paths still
// work. Distinct from MaxIterOverride (outer ReAct loop ceiling) so
// the outer cap remains a strict superset of the inner soft cap and
// the soft→hard recovery window can actually run.
func (e *plannerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	_ = sk
	if ctx != nil {
		e.mu = ctx.Mutable
		if ctx.PlannerSoftIterCapOverride > 0 {
			e.dispatchSoftIterCap = ctx.PlannerSoftIterCapOverride
			recoverySlack := e.defaultHardIterCap - e.defaultSoftIterCap
			if recoverySlack < 1 {
				recoverySlack = 1
			}
			e.dispatchHardIterCap = e.dispatchSoftIterCap + recoverySlack
		} else {
			e.dispatchSoftIterCap = 0
			e.dispatchHardIterCap = 0
		}
	}
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	hint := ctx.Mutable.PlanningHint()
	if hint == "" {
		return ""
	}
	// Consume-once: clear the hint so a subsequent sub-dispatch
	// within the same retry iteration does not double-apply it.
	ctx.Mutable.ResetPlanningHint()
	return "\n\n## Retry feedback\n\n" + hint +
		"\n\nThe previous ChangePlan's verify stage failed. Read the feedback above, diagnose what was wrong with the plan (not the test runner, which is deterministic), and emit a revised ChangePlan that fixes the root cause."
}

// ShouldStop terminates the ReAct loop as soon as a ChangePlan has
// been installed on Mutable (emit_change_plan's happy path) or when
// the loop has burned through its iteration cap. The cap is the
// per-dispatch value captured in BuildInitialInstruction; when no
// per-dispatch override is present (zero value), the agent-settings
// default takes over.
func (e *plannerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	if e.mu != nil && e.mu.ChangePlan() != nil {
		return true
	}
	soft, hard := e.dispatchSoftIterCap, e.dispatchHardIterCap
	if soft <= 0 || hard <= soft {
		soft, hard = e.defaultSoftIterCap, e.defaultHardIterCap
	}
	// Two-stage cap: soft cap is the normal stop; the soft→hard window
	// spares one extra iteration whenever the LLM is actively calling
	// emit_change_plan. Streaming gateways occasionally truncate the
	// function.arguments of a large emit_change_plan payload (~5 KB
	// JSON), the tool rejects with `unexpected EOF`, and the LLM's
	// next iteration is a clean retry. ShouldStop runs BEFORE tool
	// execution, so a flat soft-cap break would discard that recovery
	// before it ever runs.
	return iterationCapShouldStop(resp, iteration,
		soft, hard,
		emitChangePlanToolName)
}

const emitChangePlanToolName = "emit_change_plan"

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
			if tr.ToolName == emitChangePlanToolName && !tr.Success {
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
	return NewBaseAgent(types.AgentPlanner, deps, &plannerEvaluator{
		defaultSoftIterCap: deps.AgentSettings.PlannerSoftIterCap,
		defaultHardIterCap: deps.AgentSettings.PlannerHardIterCap,
	})
}
