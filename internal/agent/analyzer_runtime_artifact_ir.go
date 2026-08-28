package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeArtifactTraceDiscoveryInputsComplete reports whether the analyzer has
// already delivered both pieces of typed discovery state owned by the primary
// runtime-artifact probe: one validated user target and one validated explicit
// time window. It deliberately does not infer either value from request prose,
// sub-topic summaries, trace rows, or an explorer-authored query cursor.
func runtimeArtifactTraceDiscoveryInputsComplete(rm types.RequestModel) bool {
	if rm.RuntimeTargetProfile == nil || !rm.RuntimeTargetProfile.NamedTarget() ||
		rm.RuntimeArtifactScopeProfile == nil {
		return false
	}
	if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); !ok {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if target.Active() && !types.RuntimeTargetIsExplorationCursorSource(target.Source) {
			return true
		}
	}
	return false
}

// runtimeArtifactPrimaryDiscoveryProbeIDs identifies only required probe nodes
// that are hard predecessors of evidence nodes. Optional source-inventory /
// refinement probes and counterfactual probes retain their own objectives,
// entry conditions, and lifecycle.
func runtimeArtifactPrimaryDiscoveryProbeIDs(graph types.TaskGraph) map[string]bool {
	evidenceIDs := make(map[string]bool)
	probeEligible := make(map[string]bool)
	for i := range graph.Nodes {
		node := graph.Nodes[i]
		switch {
		case node.Type == types.NodeEvidence:
			evidenceIDs[node.ID] = true
		case node.Type == types.NodeProbe && !node.Optional && !node.IsCounterfactual:
			probeEligible[node.ID] = true
		}
	}
	out := make(map[string]bool)
	for _, edge := range graph.Edges {
		if edge.EdgeType == types.EdgeHardDependency &&
			probeEligible[edge.From] && evidenceIDs[edge.To] {
			out[edge.From] = true
		}
	}
	return out
}

// removeRuntimeArtifactTaskGraphNodes removes a discovery-only node before the
// scheduler sees the IR. Evidence nodes become graph roots; every downstream
// evidence/validate/extract/finalize dependency remains byte-for-byte intact.
func removeRuntimeArtifactTaskGraphNodes(graph *types.TaskGraph, remove map[string]bool) {
	if graph == nil || len(remove) == 0 {
		return
	}
	nodes := graph.Nodes[:0]
	for _, node := range graph.Nodes {
		if remove[node.ID] {
			continue
		}
		nodes = append(nodes, node)
	}
	graph.Nodes = nodes

	edges := graph.Edges[:0]
	for _, edge := range graph.Edges {
		if remove[edge.From] || remove[edge.To] {
			continue
		}
		edges = append(edges, edge)
	}
	graph.Edges = edges

	criticalPath := graph.ExecutionPolicy.CriticalPath[:0]
	for _, nodeID := range graph.ExecutionPolicy.CriticalPath {
		if remove[nodeID] {
			continue
		}
		criticalPath = append(criticalPath, nodeID)
	}
	graph.ExecutionPolicy.CriticalPath = criticalPath
}

func appendRuntimeArtifactEvidenceObjective(unitObjective, sharedObjective string) string {
	unitObjective = strings.TrimSpace(unitObjective)
	sharedObjective = strings.TrimSpace(sharedObjective)
	switch {
	case unitObjective == "":
		return sharedObjective
	case sharedObjective == "":
		return unitObjective
	case unitObjective == sharedObjective:
		return unitObjective
	default:
		return unitObjective + " " + sharedObjective
	}
}
