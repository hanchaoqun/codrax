package types

// TaskArtifactContract is the immutable analyzer-authored artifact declaration
// for one TaskNode. It is derived from TaskNode.Inputs/Outputs and intentionally
// carries no runtime-produced artifact IDs; runtime artifact ledgers must live
// outside AnalysisIR.
type TaskArtifactContract struct {
	NodeID  string   `json:"node_id"`
	Inputs  []string `json:"inputs,omitempty"`
	Outputs []string `json:"outputs,omitempty"`
}

// TaskArtifactContracts projects TaskGraph node artifact declarations into a
// compact typed view. Nodes without declared inputs or outputs are omitted.
func TaskArtifactContracts(g TaskGraph) []TaskArtifactContract {
	out := make([]TaskArtifactContract, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if len(n.Inputs) == 0 && len(n.Outputs) == 0 {
			continue
		}
		out = append(out, TaskArtifactContract{
			NodeID:  n.ID,
			Inputs:  append([]string(nil), n.Inputs...),
			Outputs: append([]string(nil), n.Outputs...),
		})
	}
	return out
}

// BranchTarget is one typed target of a TaskGraph branch/backtrack edge.
type BranchTarget struct {
	NodeID string `json:"node_id"`
	Guard  string `json:"guard,omitempty"`
}

// BranchPoint groups outgoing branch/backtrack edges from one node. The first
// consumer is validation_feedback, which represents lightweight execution-tree
// backtracking without requiring a separate graph framework.
type BranchPoint struct {
	FromNodeID string         `json:"from_node_id"`
	EdgeType   EdgeType       `json:"edge_type"`
	Targets    []BranchTarget `json:"targets,omitempty"`
}

// ValidationFeedbackBranchPoints groups every validation_feedback edge in the
// graph by source node, preserving graph declaration order.
func ValidationFeedbackBranchPoints(g TaskGraph) []BranchPoint {
	positions := make(map[string]int)
	out := make([]BranchPoint, 0)
	seenTargets := make(map[string]map[string]bool)
	for _, e := range g.Edges {
		if e.EdgeType != EdgeValidationFeedback || e.From == "" || e.To == "" {
			continue
		}
		idx, ok := positions[e.From]
		if !ok {
			idx = len(out)
			positions[e.From] = idx
			out = append(out, BranchPoint{FromNodeID: e.From, EdgeType: EdgeValidationFeedback})
			seenTargets[e.From] = make(map[string]bool)
		}
		if seenTargets[e.From][e.To] {
			continue
		}
		seenTargets[e.From][e.To] = true
		out[idx].Targets = append(out[idx].Targets, BranchTarget{NodeID: e.To, Guard: e.Guard})
	}
	return out
}

// ValidationFeedbackTargets returns the typed backtrack target IDs for one
// validation node, preserving edge declaration order and removing duplicates.
func ValidationFeedbackTargets(g TaskGraph, fromNodeID string) []string {
	if fromNodeID == "" {
		return nil
	}
	for _, bp := range ValidationFeedbackBranchPoints(g) {
		if bp.FromNodeID != fromNodeID {
			continue
		}
		out := make([]string, 0, len(bp.Targets))
		for _, target := range bp.Targets {
			out = append(out, target.NodeID)
		}
		return out
	}
	return nil
}
