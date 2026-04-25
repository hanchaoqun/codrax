package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// coderEvaluator drives the B1 apply-stage agent. The coder LLM
// acts as a "dumb marshaller": it reads the installed ChangePlan
// and emits one apply_patch call per ChangeUnit, copying the
// plan's path / kind / new_content / patch verbatim into each
// tool call's params. The skill's Workflow reinforces this; the
// apply_patch tool's W1 / W1b invariants reject any drift.
//
// Stop condition: WriteClosure.AppliedSet must be a superset of
// plan.TargetPaths. The evaluator does NOT try to be clever about
// partial success — if the LLM skipped a path the coder reports
// incompleteness and the orchestrator surfaces a fail-loud error.
// The surrounding verify→plan retry loop (orchestrator B2.3) treats
// that as a plan-level failure and may dispatch a fresh plan-apply-
// verify cycle when the user has set pipeline_max_verify_retries > 0.
//
// ReAct iteration budget: N = len(plan.Changes) + 3. One turn per
// apply_patch call (the LLM needs a turn to see each tool result)
// plus slack for a correction retry. BaseAgent's MaxIterations cap
// provides the outer safety net.
type coderEvaluator struct {
	// mu is captured in BuildInitialInstruction so ShouldStop can
	// check the applied-set without needing ctx passed through.
	// Mirror of plannerEvaluator.mu pattern.
	mu *types.MutableState

	// Iteration cap knobs populated from AgentSettings at construction.
	// softIterSlack is added to len(plan.TargetPaths) to form the soft
	// cap; hardIterRecovery is the post-soft-cap recovery window.
	softIterSlack    int
	hardIterRecovery int
}

// BuildInitialInstruction captures the Mutable pointer and returns
// the per-dispatch dynamic supplement that lists the remaining
// unapplied paths. Listing them in the prompt reinforces the
// skill's "one apply_patch per ChangeUnit, in DependsOn order"
// discipline — a reminder stronger than skill.Workflow alone,
// because this supplement is re-rendered each turn if mid-loop
// observations fire.
//
// When baseline capture is enabled (Mutable.BaselineReport non-nil),
// a "## Pre-existing failures" or "## Baseline status" section is
// injected so the coder LLM has explicit context on what failures
// are out of scope. Goal: keep the coder focused on plan.TargetPaths
// and stop it from drifting into "let me also fix this unrelated
// failing test" territory.
func (e *coderEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	_ = sk
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	e.mu = ctx.Mutable
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return "No ChangePlan is installed on this Run — apply phase cannot proceed. " +
			"Return without calling any tool; the orchestrator will surface the error."
	}
	applied := ctx.Mutable.WriteClosure().AppliedSet()
	pending := pendingPaths(plan, applied)
	if len(pending) == 0 {
		return "All plan changes are already applied. Return without additional tool calls."
	}

	var b strings.Builder
	b.WriteString("## Apply plan\n\n")
	fmt.Fprintf(&b, "Plan ID: `%s`\n", plan.ID)
	fmt.Fprintf(&b, "Target paths (W1-enforced): %d total, %d remaining.\n\n",
		len(plan.TargetPaths), len(pending))
	b.WriteString("For each remaining change, call `apply_patch` EXACTLY ONCE with `{path, kind}` — nothing else. " +
		"The tool hydrates new_content / patch / delete from the plan on Mutable; the schema rejects content " +
		"parameters so you cannot (and must not) re-emit them. Do not invent paths — only those listed below are " +
		"authorized (W1 rejects drift). Respect depends_on order: apply a unit only after every path in its " +
		"depends_on list has already been applied.\n\n")
	b.WriteString("Remaining changes (ordered by plan declaration; respect depends_on):\n\n")
	for _, c := range orderedUnappliedChanges(plan, applied) {
		fmt.Fprintf(&b, "- `%s` (kind=%s)", c.Path, c.Kind)
		if len(c.DependsOn) > 0 {
			fmt.Fprintf(&b, ", depends_on=%v", c.DependsOn)
		}
		b.WriteString("\n")
	}
	// Baseline guidance — three states:
	//   1. baseline absent (capture disabled or no test runner): no
	//      section. Coder behaves as before.
	//   2. baseline present + has failing tests: list pre-existing
	//      failures, instruct coder NOT to fix them.
	//   3. baseline present + all green: positive signal — any new
	//      failure post-apply IS a regression.
	baseline := ctx.Mutable.BaselineReport()
	if baseline != nil {
		failures := types.CollectBaselineFailures(baseline)
		b.WriteString("\n")
		if len(failures) > 0 {
			b.WriteString("## Pre-existing failures (out of scope)\n\n")
			b.WriteString("These tests were ALREADY failing on the unmodified worktree — they predate your changes:\n\n")
			b.WriteString(types.RenderBaselineFailureList(failures, 15))
			b.WriteString("\n")
			if len(plan.AcceptanceTests) > 0 {
				b.WriteString("Plan acceptance tests (the ones you DO need to make pass):\n\n")
				for _, a := range plan.AcceptanceTests {
					fmt.Fprintf(&b, "- %s\n", a)
				}
				b.WriteString("\n")
			}
			b.WriteString("Stay within plan.TargetPaths. Failures outside the acceptance set are NOT your responsibility — " +
				"do not invent extra apply_patch calls trying to fix them.")
		} else {
			b.WriteString("## Baseline status\n\n")
			b.WriteString("The baseline test suite is fully green — every test passed on the unmodified worktree. " +
				"Any new test failure after your changes IS a regression caused by this plan, so apply only what " +
				"the plan declares and stop.")
		}
	}
	return b.String()
}

// ShouldStop terminates the ReAct loop as soon as every target
// path has been applied OR the defensive iteration cap is hit.
// The cap is len(plan.TargetPaths) + 3 so a small plan (1 change)
// is tolerant of one or two correction retries, and a large plan
// (say 10 changes) gets 13 total turns — plenty for the LLM to
// make 10 apply_patch calls plus slack.
//
// Two-stage cap: at the soft cap, spare 3 extra iters when the LLM
// is calling apply_patch — the per-call payload is just `{path, kind}`
// (lightweight after Q2's "dumb marshaller" refactor) but a multi-file
// plan can still see one streaming hiccup near the boundary; flat cap
// would discard a clean retry. Hard cap = soft cap + 3.
func (e *coderEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	if e.mu == nil {
		return true // no mutable to check against — stop to avoid a loop
	}
	plan := e.mu.ChangePlan()
	if plan == nil {
		return true // no plan — ParseOutput will report the error
	}
	applied := e.mu.WriteClosure().AppliedSet()
	if isAppliedComplete(plan, applied) {
		return true
	}
	softCap := len(plan.TargetPaths) + e.softIterSlack
	hardCap := softCap + e.hardIterRecovery
	return iterationCapShouldStop(resp, iteration,
		softCap, hardCap,
		applyPatchToolName)
}

const applyPatchToolName = "apply_patch"

// ParseOutput inspects Mutable's AppliedSet vs plan.TargetPaths
// and returns success when the set is complete, or a structured
// error listing the missing paths when the LLM failed to cover
// every target.
func (e *coderEvaluator) ParseOutput(
	ctx *types.AgentContext,
	_ []llm.Message,
	toolResults []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	out := &StageOutput{}

	if ctx == nil || ctx.Mutable == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "coder: Mutable is nil"
		return out, fmt.Errorf("%s", out.Error)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "coder: no ChangePlan installed on Mutable — cannot apply"
		return out, fmt.Errorf("%s", out.Error)
	}

	applied := ctx.Mutable.WriteClosure().AppliedSet()
	if isAppliedComplete(plan, applied) {
		logging.Info("[coder] apply complete: %d/%d paths applied",
			len(applied), len(plan.TargetPaths))
		out.MissingPiece = types.MissingNone
		return out, nil
	}

	// Incomplete. Report every missing path + the reason derived
	// from the latest failed tool result (if any) so the coder's
	// caller sees actionable detail.
	missing := pendingPaths(plan, applied)
	var lastToolErr string
	for i := len(toolResults) - 1; i >= 0; i-- {
		tr := toolResults[i]
		if tr.ToolName == "apply_patch" && !tr.Success {
			lastToolErr = tr.Summary
			break
		}
	}
	reason := fmt.Sprintf("coder: apply incomplete — %d of %d paths missing: %s",
		len(missing), len(plan.TargetPaths), strings.Join(missing, ", "))
	if lastToolErr != "" {
		reason += "\n\nlast apply_patch rejection: " + lastToolErr
	}
	out.MissingPiece = types.MissingFacts
	out.Error = reason
	return out, fmt.Errorf("coder: apply incomplete (%d missing)", len(missing))
}

// DetermineMissingPiece reflects ParseOutput's determination.
func (e *coderEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewCoderAgent constructs the B1 apply-stage agent.
func NewCoderAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentCoder, deps, &coderEvaluator{
		softIterSlack:    deps.AgentSettings.CoderSoftIterSlack,
		hardIterRecovery: deps.AgentSettings.CoderHardIterRecovery,
	})
}

// ── helpers ────────────────────────────────────────────────────

// isAppliedComplete is true when every plan.TargetPaths entry is
// also in applied. Uses TargetPaths as the authoritative truth
// (mirrors the emit_change_plan dedup discipline).
func isAppliedComplete(plan *types.ChangePlan, applied map[string]bool) bool {
	if plan == nil {
		return false
	}
	for _, p := range plan.TargetPaths {
		if !applied[p] {
			return false
		}
	}
	return true
}

// pendingPaths returns plan.TargetPaths minus the applied set,
// preserving declaration order so the resulting slice is stable
// across repeated calls (same Mutable, same plan).
func pendingPaths(plan *types.ChangePlan, applied map[string]bool) []string {
	if plan == nil {
		return nil
	}
	out := make([]string, 0, len(plan.TargetPaths))
	for _, p := range plan.TargetPaths {
		if !applied[p] {
			out = append(out, p)
		}
	}
	return out
}

// orderedUnappliedChanges returns the Changes[] entries whose Path
// is not yet applied, in declaration order. Consumed by
// BuildInitialInstruction to render the remaining-work summary for
// the LLM.
func orderedUnappliedChanges(plan *types.ChangePlan, applied map[string]bool) []types.FileChange {
	if plan == nil {
		return nil
	}
	out := make([]types.FileChange, 0, len(plan.Changes))
	for _, c := range plan.Changes {
		if !applied[c.Path] {
			out = append(out, c)
		}
	}
	return out
}
