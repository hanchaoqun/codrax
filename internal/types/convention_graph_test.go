package types

import "testing"

func TestConventionGraphFromExplorationHandoffDerivesTypedConventions(t *testing.T) {
	handoff := WriteExplorationHandoff{
		BatchID:          "batch-1",
		Goal:             "repair planner",
		ExistingPatterns: []string{"planner sections are pure data", "planner sections are pure data"},
		EvidenceRefs: []WriteExplorationEvidenceRef{{
			ID:           "ev-mechanism",
			Kind:         string(EvidenceMechanism),
			Source:       "internal/agent/planner.go",
			LineStart:    105,
			LineEnd:      120,
			Subject:      "BuildInitialInstruction",
			AnchorSymbol: "BuildInitialInstruction",
			Summary:      "planner composes sections from typed inputs",
		}, {
			ID:        "ev-direct",
			Kind:      string(EvidenceDirect),
			Source:    "internal/agent/planner.go",
			LineStart: 210,
			Summary:   "direct facts are evidence, not conventions",
		}},
	}

	graph := ConventionGraphFromExplorationHandoff(handoff)
	if graph.BatchID != "batch-1" || graph.Source != "write_exploration_handoff" {
		t.Fatalf("graph metadata not preserved: %+v", graph)
	}
	if got := conventionGraphCategoryCount(graph, ConventionCategoryLocalPattern); got != 1 {
		t.Fatalf("local pattern should dedupe to one node, got %d: %+v", got, graph.Nodes)
	}
	if got := conventionGraphCategoryCount(graph, ConventionCategoryMechanism); got != 1 {
		t.Fatalf("mechanism evidence should produce one convention, got %d: %+v", got, graph.Nodes)
	}
	if got := conventionGraphCategoryCount(graph, ConventionCategoryRelationship); got != 0 {
		t.Fatalf("unsupported evidence kind should not invent relationship conventions, got %d: %+v", got, graph.Nodes)
	}
}

func TestNormalizeWriteExplorationHandoffDerivesConventionGraphAndCopies(t *testing.T) {
	handoff := WriteExplorationHandoff{
		BatchID:          "batch-1",
		ExistingPatterns: []string{"test helpers use factory fixtures"},
	}

	normalized := NormalizeWriteExplorationHandoff(handoff)
	if normalized.ConventionGraph == nil || len(normalized.ConventionGraph.Nodes) != 1 {
		t.Fatalf("expected derived convention graph: %+v", normalized.ConventionGraph)
	}
	cloned := cloneWriteExplorationHandoff(normalized)
	cloned.ConventionGraph.Nodes[0].Summary = "mutated"
	if normalized.ConventionGraph.Nodes[0].Summary == "mutated" {
		t.Fatalf("convention graph clone should be defensive")
	}
}

func conventionGraphCategoryCount(graph ConventionGraph, category ConventionCategory) int {
	count := 0
	for _, node := range graph.Nodes {
		if node.Category == category {
			count++
		}
	}
	return count
}
