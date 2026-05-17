package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// E' (2026-05-17 architecture §1.5/§1.6) — DAG node-level dispatch.
//
// Forensic anchor: prior to this change, when the analyzer's
// expandEvidenceNodes (compiler/templates.go) produced N independent
// evidence sibling nodes (`{prefix}_t0` / `_t1` / `_tN`, one per
// SubTopic), the scheduler's readyExplorerWindowContext correctly
// returned all N nodes in a single slice — but the explore dispatch
// loop then COALESCED the entire window into one explorer LLM
// dispatch (renderWindowHintContext emits "Cover every node objective
// below in this dispatch:\n\n" listing all N objectives). One explorer
// instance had to serially chew through every sub-topic, producing
// the 5-minute Ctrl+C scenario in the 08:14 forensic run.
//
// Architecture red line §1.5: the DAG produced by the analyzer is
// typed authoritative data. The scheduler is code. Code must respect
// what data says — including the *freedom* (no edge between siblings
// = no ordering constraint = independent dispatch). Coalescing
// independent sibling nodes into one dispatch is the scheduler
// IGNORING that typed freedom. This file restores the alignment.
//
// Design (intentionally minimal):
//
//   - When the ready window contains ≥2 NodeEvidence siblings that
//     share a common ID prefix (the `{prefix}_t{i}` pattern emitted
//     by expandEvidenceNodes), trim the window down to its FIRST
//     element. The outer scheduler loop will pick up the remaining
//     siblings on subsequent iterations: each iteration re-runs
//     readyExplorerWindowContext, which filters out nodes whose
//     status is already nodeRunning / nodeDone, so the next loop
//     naturally sees only the un-dispatched siblings.
//
//   - This produces N sequential dispatches, each focused on ONE
//     sub-topic. Per-node EventTaskNodeStart / EventTaskNodeEnd
//     already fire correctly (orchestrator.go:4064-4066, 4172-4174).
//     The /dag UI shows N rows, each transitioning
//     pending → running → done independently.
//
//   - Multi-topic step budget is already scaled at
//     orchestrator.go:3833 (`nSub * agentCfg.SubTopicPipelineStepsExtra`)
//     so N sequential dispatches fit the budget the analyzer already
//     reserved for multi-topic questions.
//
//   - PARALLELISM is a Phase-2 concern. Serial-per-node alone is the
//     architecturally-correct foundation: it respects the DAG's
//     independence relations, surfaces them to the user, and gives
//     each sub-topic its own focused explorer dispatch (instead of
//     one explorer juggling N objectives). True wall-clock
//     parallelism via errgroup needs busCtx.TaskState.RetryHint
//     to move from shared state to per-dispatch parameter — out of
//     scope for this commit.
//
// Single-sub_topic / non-evidence-window paths are byte-identical to
// the prior behavior: the helper returns the same window slice with
// no mutation.

// shouldDispatchExploreNodesIndividually reports whether the ready
// explore window represents multiple INDEPENDENT evidence sibling
// nodes that should each receive a focused explorer dispatch instead
// of being merged into one explorer LLM call.
//
// Returns true iff the window contains AT LEAST TWO types.NodeEvidence
// entries. Other node kinds in the same window (probe, validate,
// reconcile) are allowed — they are handled by the companion trim
// helper, which strips the sibling evidence nodes but preserves
// non-evidence companions so the explorer dispatch still receives
// probe-scope + validate-context.
//
// Real-world DAG shape (from internal/analysis/compiler/templates.go
// expandEvidenceNodes when len(SubTopics) > 1): one probe node, N
// evidence sibling nodes (`{prefix}_t0` / `_t1` / `_tN`), one
// validate node. readyExplorerWindowContext currently has no
// EntryConditions to gate them apart, so all 1+N+1 nodes appear
// simultaneously on the first ready window. Before E', the explorer
// got a single dispatch covering all of them. After E', this
// predicate fires on (N≥2 evidence) and the trim helper retains
// probe + first evidence + validate; the outer scheduler loop
// dispatches the remaining (N-1) evidence siblings one by one on
// subsequent iterations.
//
// Sibling-edge check is implicit: expandEvidenceNodes does NOT emit
// hard/soft edges between sibling evidence nodes (templates.go builds
// edges from each evidence node to validate, never between
// siblings), so any window with ≥2 evidence siblings has no
// inter-sibling edges by construction. If a future analyzer template
// adds sibling-sibling edges, this predicate will need an explicit
// edge scan; today's expansion makes that scan vacuous.
func shouldDispatchExploreNodesIndividually(window []*types.TaskNode) bool {
	if len(window) < 2 {
		return false
	}
	evidenceCount := 0
	for _, n := range window {
		if n == nil {
			continue
		}
		if n.Type == types.NodeEvidence {
			evidenceCount++
			if evidenceCount >= 2 {
				return true
			}
		}
	}
	return false
}

// trimExploreWindowToFirstEvidence drops all but the first
// NodeEvidence entry while preserving every non-evidence companion
// (probe / validate / reconcile / counterfactual). The first
// evidence node — by readyExplorerWindowContext's declaration-order
// sort — is the lowest-index sibling, so successive scheduler
// iterations pick up t1, t2, ... in order.
//
// Example transformation for the forensic 08:14 case:
//
//	IN:  [probe, evidence_t0, evidence_t1, validate]
//	OUT: [probe, evidence_t0, validate]
//
// Next scheduler iteration after t0 markDone:
//
//	IN:  [evidence_t1, validate]   (probe + t0 already done)
//	OUT: same (only 1 evidence node, predicate does not fire)
//
// The returned slice is a freshly-allocated copy — it does NOT
// alias the input. The scheduler treats window slices as read-only
// after readyExplorerWindow returns, so the allocation matters only
// for downstream sequential modification (none today).
func trimExploreWindowToFirstEvidence(window []*types.TaskNode) []*types.TaskNode {
	if len(window) == 0 {
		return window
	}
	out := make([]*types.TaskNode, 0, len(window))
	keptFirstEvidence := false
	for _, n := range window {
		if n == nil {
			out = append(out, n)
			continue
		}
		if n.Type == types.NodeEvidence {
			if keptFirstEvidence {
				continue
			}
			keptFirstEvidence = true
		}
		out = append(out, n)
	}
	return out
}
