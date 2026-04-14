// Package counterfactual expands a task graph with alternative
// explanation paths when the request is ambiguous enough to warrant
// them. The output is always a superset of the input graph: probe
// and finalize stages are untouched, a pair of counterfactual
// evidence nodes is inserted after the main evidence node, and a
// reconcile node collects both branches before finalize.
//
// Counterfactual expansion is opt-in and defaults to off for
// Analyzer v3's first release (batch b9 flips the default). The
// analyzer agent turns it on when:
//
//   - RequestModel.Ambiguities contains ≥1 entry with ≥2 options, AND
//   - RequestModel.Complexity == "complex", AND
//   - RequestModel.Intent is one of {root_cause, security_audit, explain}.
//
// Expand() is safe to call with ShouldExpand=false as a no-op.
package counterfactual

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Options controls a single expansion call. The zero value is a
// no-op: Enabled=false leaves the graph untouched.
type Options struct {
	Enabled bool
	// MaxBranches caps the number of counterfactual pairs added per
	// ambiguity. Zero or negative means "1".
	MaxBranches int
}

// ShouldExpand returns true when the default policy says the
// request merits a counterfactual branch. The analyzer agent may
// override this with its own Options.
func ShouldExpand(rm types.RequestModel) bool {
	if rm.Complexity != types.ComplexityComplex {
		return false
	}
	switch rm.Intent {
	case types.IntentRootCause, types.IntentExplain:
	default:
		return false
	}
	for _, a := range rm.Ambiguities {
		if len(a.Options) >= 2 && strings.TrimSpace(a.Clause) != "" {
			return true
		}
	}
	return false
}

// Expand inserts counterfactual evidence nodes and a reconcile node
// into the task graph. It returns the modified graph (the input is
// copied, not mutated) and a slice of newly created node IDs so the
// caller can bind hypotheses to them.
func Expand(tg types.TaskGraph, rm types.RequestModel, opts Options) (types.TaskGraph, []string) {
	if !opts.Enabled {
		return tg, nil
	}
	maxBranches := opts.MaxBranches
	if maxBranches <= 0 {
		maxBranches = 1
	}

	out := cloneGraph(tg)
	mainEvidence := findFirstNode(out, types.NodeEvidence)
	finalize := findFirstNode(out, types.NodeFinalize)
	if mainEvidence == "" || finalize == "" {
		return tg, nil
	}

	// Build a reconcile node that collapses all counterfactual
	// branches. If the graph already has a reconcile we reuse it so
	// the architecture_explain template's reconcile node is honored.
	reconcileID := findFirstNode(out, types.NodeReconcile)
	if reconcileID == "" {
		reconcileID = "cf_reconcile"
		out.Nodes = append(out.Nodes, types.TaskNode{
			ID: reconcileID, Type: types.NodeReconcile,
			Objective:       "Reconcile counterfactual branches by evidence weight.",
			EntryConditions: []string{"all_counterfactuals_evaluated"},
		})
		// Rewire finalize: remove any existing direct edge from
		// mainEvidence → finalize and redirect through reconcile.
		out.Edges = replaceEdgeTarget(out.Edges, mainEvidence, finalize, reconcileID)
		out.Edges = append(out.Edges, types.TaskEdge{
			From: reconcileID, To: finalize, EdgeType: types.EdgeHardDependency,
		})
	}

	var newIDs []string
	branchIdx := 0
	for _, a := range rm.Ambiguities {
		if branchIdx >= maxBranches {
			break
		}
		if len(a.Options) < 2 || strings.TrimSpace(a.Clause) == "" {
			continue
		}
		for optIdx, opt := range a.Options[:2] { // exactly two branches per ambiguity
			id := fmt.Sprintf("cf_%d_%d", branchIdx, optIdx)
			out.Nodes = append(out.Nodes, types.TaskNode{
				ID: id, Type: types.NodeEvidence,
				Objective: fmt.Sprintf("Counterfactual branch: assume %q resolves to %q and collect supporting evidence.",
					a.Clause, opt),
				IsCounterfactual: true,
				MaxRetries:       1,
			})
			out.Edges = append(out.Edges,
				types.TaskEdge{From: mainEvidence, To: id, EdgeType: types.EdgeSoftDependency},
				types.TaskEdge{From: id, To: reconcileID, EdgeType: types.EdgeHardDependency},
			)
			newIDs = append(newIDs, id)
		}
		branchIdx++
	}
	return out, newIDs
}

// findFirstNode returns the ID of the first node with the requested
// type, or "" if none exists.
func findFirstNode(tg types.TaskGraph, t types.TaskNodeType) string {
	for _, n := range tg.Nodes {
		if n.Type == t {
			return n.ID
		}
	}
	return ""
}

// replaceEdgeTarget rewrites every edge that goes from→oldTo so that
// it goes from→newTo. Used to splice a reconcile node between the
// main evidence node and the finalize node.
func replaceEdgeTarget(edges []types.TaskEdge, from, oldTo, newTo string) []types.TaskEdge {
	out := make([]types.TaskEdge, len(edges))
	for i, e := range edges {
		if e.From == from && e.To == oldTo {
			e.To = newTo
		}
		out[i] = e
	}
	return out
}

// cloneGraph returns a deep copy of a TaskGraph so Expand never
// mutates its input. The node and edge slices are copied; string
// slices inside nodes are shared because nodes are not modified
// after cloning.
func cloneGraph(tg types.TaskGraph) types.TaskGraph {
	out := tg
	out.Nodes = append([]types.TaskNode(nil), tg.Nodes...)
	out.Edges = append([]types.TaskEdge(nil), tg.Edges...)
	return out
}
