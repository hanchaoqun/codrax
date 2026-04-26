package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildWriteTaskGraph emits the linear plan→apply→verify TaskGraph
// the scheduler walks for write-mode Runs. A separate function from
// the analyzer's read-mode TaskGraph because the shape is fixed —
// no scenario inference, no hypothesis binding, no risk matrix. The
// graph is a deterministic 3-node chain with EdgeValidationFeedback
// from verify back to plan so a verify SuccessCriteria failure can
// trigger requeueValidationTargets and re-plan with a fresh
// PlanningHint.
//
// Mode shapes the returned graph:
//
//   - ModePlan: plan node only. Run terminates after plan emission;
//     the plan IS the answer. No apply, no verify.
//   - ModeApply: full plan→apply→verify chain. PlanPath != "" causes
//     the plan node to be skipped (R8a: user has a reviewed plan).
//   - ModeVerify: verify node only. Re-runs tests against an existing
//     plan without re-applying.
//
// retryBudget caps the verify→plan cycle count via
// TaskGraph.ExecutionPolicy.RetryBudget — the same machinery
// scheduler.go uses for read-mode SuccessCriteria retries. A zero
// budget disables retries (verify failure terminates the Run).
//
// EntryConditions reuse the existing write-mode Criterion kinds:
//   - apply gates on CritPlanReady (PendingApplies non-empty)
//   - verify gates on CritPatchApplies (AppliedSet ⊇ TargetPaths)
//
// SuccessCriteria mirror the per-node verdict:
//   - plan SC: CritPlanReady (plan emitted with non-empty queue)
//   - apply SC: CritPatchApplies (every TargetPath landed)
//   - verify SC: CritTestsPass + CritNoRegression (both must pass)
//
// OneShot semantics: plan is OneShot=true (each plan emission is a
// fresh node-done event); apply / verify are OneShot=false so the
// EdgeValidationFeedback retry cycle can re-queue them. This mirrors
// the read-mode finalize-once-pattern via a structural flag instead
// of a NodeType-special-case in the scheduler.
func BuildWriteTaskGraph(mode types.PipelineMode, planPath string, retryBudget int) types.TaskGraph {
	const (
		idPlan   = "plan"
		idApply  = "apply"
		idVerify = "verify"
	)

	plan := types.TaskNode{
		ID:        idPlan,
		Type:      types.NodePlan,
		Objective: "Emit a ChangePlan describing the requested code change",
		OneShot:   true,
		// EntryConditions empty — plan runs first.
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritPlanReady},
		},
	}
	apply := types.TaskNode{
		ID:        idApply,
		Type:      types.NodeApply,
		Objective: "Apply the ChangePlan inside the worktree",
		OneShot:   false,
		EntryConditions: []types.Criterion{
			{Kind: types.CritPlanReady},
		},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritPatchApplies},
		},
	}
	verify := types.TaskNode{
		ID:        idVerify,
		Type:      types.NodeVerify,
		Objective: "Run the test suite and confirm no regression",
		OneShot:   false,
		EntryConditions: []types.Criterion{
			{Kind: types.CritPatchApplies},
		},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritTestsPass},
			{Kind: types.CritNoRegression},
		},
	}

	// Sub-graph selection by mode. The shared shape:
	//   plan → apply → verify (linear hard-deps)
	//   verify → plan (validation_feedback for retry cycle)
	// is sliced by mode so ModePlan stops at plan, ModeVerify starts
	// at verify, ModeApply runs the full chain.
	var (
		nodes []types.TaskNode
		edges []types.TaskEdge
	)
	switch mode {
	case types.ModePlan:
		nodes = []types.TaskNode{plan}
	case types.ModeVerify:
		// Verify-only: drop the apply EntryCondition (no apply ran
		// in this Run) and the test gate uses the plan loaded from
		// disk for AcceptanceTests context.
		verify.EntryConditions = nil
		nodes = []types.TaskNode{verify}
	case types.ModeApply:
		// ModeApply graph is now uniform regardless of whether the
		// user supplied --plan-file: always plan + apply + verify with
		// the verify→plan retry feedback edge. The previous design
		// skipped the plan node entirely when planPath != "" and
		// removed the retry edge, calling verify failure "terminal"
		// for that path. This was wrong on three counts:
		//   1. Empirically — every plan/apply pipeline in the
		//      literature (SWE-agent, Aider, AutoCodeRover, Reflexion)
		//      gains +5-12pp pass rate from retry on benchmarks. Our
		//      eval surfaced robot-name (3/4 → could be 4/4) and
		//      trinary (compile error → could be fixed) failing for
		//      this exact reason.
		//   2. Architecturally — re-running the plan node on retry
		//      is NOT "re-applying the same plan." clearForReplan
		//      wipes ChangePlan/AppliedSet AND planPath, so the next
		//      plan dispatch regenerates the plan informed by
		//      Mutable.PlanningHint. The first iteration still uses
		//      the disk plan via the planNode.OneShot+SkipFirst
		//      shortcut (R8a preserved: don't regenerate the user-
		//      reviewed plan unless retry forces it).
		//   3. Operationally — the production workflow is plan +
		//      review + approve (separate Run with --plan-file), so
		//      the old code made retry effectively dead code in the
		//      one path users actually take.
		//
		// R8a-preserving short-circuit: when planPath is non-empty,
		// the FIRST plan-node visit is a no-op (applyPreHook will
		// LoadChangePlanFromFile). Subsequent visits (after retry)
		// re-emit. Implemented by the SkipOnFirstVisit flag on the
		// plan node — scheduler honours it during graph walk.
		plan.SkipOnFirstVisit = (planPath != "")
		if planPath != "" {
			// Apply's EntryConditions normally gate on CritPlanReady
			// (WriteClosure.PendingApplies non-empty), populated by
			// emit_change_plan during the plan stage. With the plan
			// node skipped on first visit, PendingApplies is still
			// empty when apply tries to enter — chicken-and-egg.
			// applyPreHook seeds PendingApplies from the disk plan,
			// so dropping the gate is safe (the load happens before
			// the agent dispatches). On retry, plan node DOES run
			// and re-populates PendingApplies normally, so this
			// override only affects the first apply.
			apply.EntryConditions = nil
		}
		nodes = []types.TaskNode{plan, apply, verify}
		edges = []types.TaskEdge{
			{From: idPlan, To: idApply, EdgeType: types.EdgeHardDependency},
			{From: idApply, To: idVerify, EdgeType: types.EdgeHardDependency},
			// Verify SuccessCriteria failure requeues the plan
			// node so the next iteration emits a fresh plan
			// informed by Mutable.PlanningHint, AND requeues the
			// apply node so the freshly-emitted plan actually
			// gets applied to the worktree.
			{From: idVerify, To: idPlan, EdgeType: types.EdgeValidationFeedback},
			{From: idVerify, To: idApply, EdgeType: types.EdgeValidationFeedback},
		}
	default:
		// Read mode never reaches this builder; defensive empty.
		return types.TaskGraph{}
	}

	return types.TaskGraph{
		Nodes: nodes,
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, // write stages are strictly sequential
			RetryBudget:    retryBudget,
		},
	}
}

// IsWriteGraph reports whether g contains any write-mode node type.
// Used by runTaskGraph to dispatch to the write scheduler loop
// instead of the read scheduler loop. Read-mode TaskGraphs from the
// analyzer never carry NodePlan / NodeApply / NodeVerify, so this
// scan is robust against shape drift.
func IsWriteGraph(g types.TaskGraph) bool {
	for _, n := range g.Nodes {
		switch n.Type {
		case types.NodePlan, types.NodeApply, types.NodeVerify:
			return true
		}
	}
	return false
}
