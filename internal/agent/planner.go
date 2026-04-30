package agent

import (
	"fmt"
	"strings"

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
	// The header is intentionally generic ("Planning context"). The
	// hint body itself carries the situation tag:
	//   - verify→plan retry — body opens with "Previous attempt N
	//     verify failed ..." (buildRetryHint).
	//   - empty-repo scaffold — body opens with "SCAFFOLD DIRECTIVE
	//     —" (planPreHook proactive seed).
	//   - streaming-stall recovery — body opens with "RETRY
	//     DIRECTIVE —" (write_scheduler transient retry).
	// Keeping the wrapper neutral avoids hardcoding "verify failed"
	// under branches where it isn't true.
	return "\n\n## Planning context\n\n" + hint +
		"\n\nFollow the directive in the planning context above. It was added because the system observed a condition that affects how this dispatch must emit."
}

// ShouldStop terminates the ReAct loop as soon as a ChangePlan has
// been installed on Mutable (any of the three emit paths' happy
// path) or when the loop has burned through its iteration cap.
//
// Three emit paths land a finished plan on Mutable.ChangePlan:
//
//  1. emit_change_plan single-shot — LLM emits the whole plan in one
//     tool call. Smallest LLM round count when the plan fits.
//
//  2. emit_plan_skeleton + emit_plan_change multi-round — skeleton
//     first (metadata only, payload-bounded), then per-file body
//     emits. The LAST emit_plan_change call promotes
//     PartialChangePlan → ChangePlan once every body slot is filled.
//
// All three names belong to the recovery list because every one of
// them is a structured emit whose function.arguments string can be
// truncated by a streaming gateway; the LLM's clean retry of any of
// them at the soft cap must be allowed to execute before the hard
// cap forces termination. The cap is the per-dispatch value captured
// in BuildInitialInstruction; when no per-dispatch override is
// present (zero value), the agent-settings default takes over.
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
	// any of the planner's structured-emit tools. Streaming gateways
	// occasionally truncate the function.arguments of a large emit
	// payload, the tool rejects with `unexpected EOF`, and the LLM's
	// next iteration is a clean retry. ShouldStop runs BEFORE tool
	// execution, so a flat soft-cap break would discard that recovery
	// before it ever runs.
	return iterationCapShouldStop(resp, iteration,
		soft, hard,
		emitChangePlanToolName,
		emitPlanSkeletonToolName,
		emitPlanChangeToolName)
}

const (
	emitChangePlanToolName   = "emit_change_plan"
	emitPlanSkeletonToolName = "emit_plan_skeleton"
	emitPlanChangeToolName   = "emit_plan_change"
)

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
		// Three failure shapes to distinguish so the caller's retry
		// hint and the user-visible LastError both carry actionable
		// information instead of "did not emit":
		//
		//  (a) Partial plan installed but file contents still missing
		//      — the outline landed, the model was midway through
		//      per-file content emits when the loop bound hit. List
		//      the missing paths so the next round knows what to do.
		//  (b) Most recent structured emit was rejected — surface the
		//      validator's reason verbatim.
		//  (c) Nothing emitted at all — generic message.
		//
		// User-facing wording deliberately avoids tool names
		// (emit_change_plan / emit_plan_skeleton / emit_plan_change)
		// and structural slot names (PartialChangePlan / ChangePlan)
		// because this string ends up on TaskState.LastError and is
		// surfaced verbatim to the user; internal tool names confuse
		// without adding actionable signal. Operator log lines below
		// keep enough breadcrumbs for ops debugging.
		var partial *types.ChangePlan
		if ctx != nil && ctx.Mutable != nil {
			partial = ctx.Mutable.PartialChangePlan()
		}
		var reason string
		if partial != nil {
			missing := plannerMissingBodies(partial)
			total := plannerNonDeleteSlotCount(partial)
			if len(missing) == 0 {
				// Outline + every body present yet ChangePlan slot
				// stayed empty. Internal mismatch — log details, give
				// the user a short non-jargon line.
				logging.Warning("[planner] partial plan id=%s has every body filled but promotion to ChangePlan never fired (internal state mismatch)", partial.ID)
				reason = "the change plan was outlined and every file body provided, but the plan was never finalized (please retry)"
			} else {
				preview := missing
				if len(preview) > 5 {
					preview = preview[:5]
				}
				logging.Warning("[planner] partial plan id=%s has %d/%d file bodies missing: %v", partial.ID, len(missing), total, missing)
				reason = fmt.Sprintf(
					"the change plan listed %d files but %d still need their contents written. Pending files: %s",
					total, len(missing),
					strings.Join(preview, ", "))
				if len(missing) > len(preview) {
					reason += fmt.Sprintf(" (and %d more)", len(missing)-len(preview))
				}
			}
		} else {
			// Default message stays neutral; operator log captures
			// the failed-tool diagnostics for ops investigation.
			reason = "no change plan was produced this round"
			for i := len(toolResults) - 1; i >= 0; i-- {
				tr := toolResults[i]
				if !tr.Success && (tr.ToolName == emitChangePlanToolName ||
					tr.ToolName == emitPlanSkeletonToolName ||
					tr.ToolName == emitPlanChangeToolName) {
					logging.Warning("[planner] structured emit rejected: tool=%s summary=%s", tr.ToolName, tr.Summary)
					reason = "the proposed change plan was rejected: " + tr.Summary
					break
				}
			}
		}
		logging.Warning("[planner] no plan installed: %s", reason)
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

// plannerMissingBodies returns the repo-relative paths of partial
// plan changes that still need their content written (kind in
// {create, modify} with empty NewContent, or kind=patch with empty
// Patch). delete kinds are body-trivial and excluded.
//
// Used by ParseOutput to surface the precise gap when the loop
// terminates with a partial plan installed but no completed plan.
// Pure read of partial.Changes — no mutation.
func plannerMissingBodies(partial *types.ChangePlan) []string {
	if partial == nil {
		return nil
	}
	var missing []string
	for _, c := range partial.Changes {
		switch strings.TrimSpace(c.Kind) {
		case "create", "modify":
			if strings.TrimSpace(c.NewContent) == "" {
				missing = append(missing, c.Path)
			}
		case "patch":
			if strings.TrimSpace(c.Patch) == "" {
				missing = append(missing, c.Path)
			}
		}
	}
	return missing
}

// plannerNonDeleteSlotCount returns the number of changes that
// require a body fill (kinds: create / modify / patch). Used as the
// denominator in ParseOutput's "X/Y files still need contents"
// message so the user sees a concrete progress fraction.
func plannerNonDeleteSlotCount(partial *types.ChangePlan) int {
	if partial == nil {
		return 0
	}
	n := 0
	for _, c := range partial.Changes {
		if strings.TrimSpace(c.Kind) != "delete" {
			n++
		}
	}
	return n
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
