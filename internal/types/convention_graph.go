package types

import (
	"fmt"
	"sort"
	"strings"
)

const conventionGraphMaxNodes = 32

type ConventionCategory string

const (
	ConventionCategoryLocalPattern ConventionCategory = "local_pattern"
	ConventionCategoryMechanism    ConventionCategory = "mechanism"
	ConventionCategoryRelationship ConventionCategory = "relationship"
	ConventionCategoryRegistration ConventionCategory = "registration"
)

// ConventionNode is a typed, evidence-backed local convention hint. Convention
// nodes are soft planning/critic context only; hard gates must not make
// decisions from style or pattern text.
type ConventionNode struct {
	ID            string             `json:"id,omitempty"`
	Category      ConventionCategory `json:"category"`
	Summary       string             `json:"summary"`
	Source        string             `json:"source,omitempty"`
	LineStart     int                `json:"line_start,omitempty"`
	LineEnd       int                `json:"line_end,omitempty"`
	Subject       string             `json:"subject,omitempty"`
	AnchorSymbol  string             `json:"anchor_symbol,omitempty"`
	EvidenceRefID string             `json:"evidence_ref_id,omitempty"`
	SourceStage   string             `json:"source_stage,omitempty"`
	Strength      string             `json:"strength,omitempty"`
}

// ConventionGraph is an inspectable project-local convention artifact. The
// first MVP derives it from read-only exploration handoff signals and persists
// it through WriteContextPack as P3 advisory context.
type ConventionGraph struct {
	GraphID string           `json:"graph_id,omitempty"`
	BatchID string           `json:"batch_id,omitempty"`
	Goal    string           `json:"goal,omitempty"`
	Source  string           `json:"source,omitempty"`
	Nodes   []ConventionNode `json:"nodes,omitempty"`
}

func ConventionGraphFromExplorationHandoff(h WriteExplorationHandoff) ConventionGraph {
	h = normalizeWriteExplorationHandoffCore(h)
	return conventionGraphFromNormalizedExplorationHandoff(h)
}

func NormalizeConventionGraph(in ConventionGraph) ConventionGraph {
	in.GraphID = trimWriteExplorationText(in.GraphID)
	in.BatchID = trimWriteExplorationText(in.BatchID)
	in.Goal = trimWriteExplorationText(in.Goal)
	in.Source = trimWriteExplorationText(in.Source)
	if in.Source == "" && len(in.Nodes) > 0 {
		in.Source = "write_exploration_handoff"
	}
	nodes := make([]ConventionNode, 0, len(in.Nodes))
	seen := map[string]bool{}
	for _, node := range in.Nodes {
		node = normalizeConventionNode(node)
		if node.Category == "" || node.Summary == "" {
			continue
		}
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		nodes = append(nodes, node)
		if len(nodes) >= conventionGraphMaxNodes {
			break
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	in.Nodes = nodes
	if in.GraphID == "" && len(in.Nodes) > 0 {
		in.GraphID = "convention-graph:" + in.BatchID
	}
	return in
}

func cloneConventionGraph(in *ConventionGraph) *ConventionGraph {
	if in == nil {
		return nil
	}
	cp := *in
	cp.Nodes = append([]ConventionNode(nil), in.Nodes...)
	return &cp
}

func conventionGraphFromNormalizedExplorationHandoff(h WriteExplorationHandoff) ConventionGraph {
	graph := ConventionGraph{
		BatchID: h.BatchID,
		Goal:    h.Goal,
		Source:  "write_exploration_handoff",
	}
	for _, pattern := range h.ExistingPatterns {
		pattern = trimWriteExplorationText(pattern)
		if pattern == "" {
			continue
		}
		graph.Nodes = append(graph.Nodes, normalizeConventionNode(ConventionNode{
			Category:    ConventionCategoryLocalPattern,
			Summary:     pattern,
			SourceStage: "explore",
			Strength:    "observed",
		}))
	}
	for _, ref := range h.EvidenceRefs {
		category := conventionCategoryFromEvidenceKind(ref.Kind)
		if category == "" {
			continue
		}
		summary := trimWriteExplorationText(ref.Summary)
		if summary == "" {
			summary = renderWriteEvidenceRefText(ref)
		}
		if summary == "" {
			continue
		}
		graph.Nodes = append(graph.Nodes, normalizeConventionNode(ConventionNode{
			Category:      category,
			Summary:       summary,
			Source:        ref.Source,
			LineStart:     ref.LineStart,
			LineEnd:       ref.LineEnd,
			Subject:       ref.Subject,
			AnchorSymbol:  ref.AnchorSymbol,
			EvidenceRefID: ref.ID,
			SourceStage:   "explore",
			Strength:      "evidence_ref",
		}))
	}
	return NormalizeConventionGraph(graph)
}

func normalizeConventionNode(in ConventionNode) ConventionNode {
	in.Category = conventionCategoryFromString(string(in.Category))
	in.Summary = trimWriteExplorationText(in.Summary)
	in.Source = normalizeConventionPath(in.Source)
	in.Subject = trimWriteExplorationText(in.Subject)
	in.AnchorSymbol = trimWriteExplorationText(in.AnchorSymbol)
	in.EvidenceRefID = trimWriteExplorationText(in.EvidenceRefID)
	in.SourceStage = trimWriteExplorationText(in.SourceStage)
	in.Strength = trimWriteExplorationText(in.Strength)
	if in.LineStart < 0 {
		in.LineStart = 0
	}
	if in.LineEnd < 0 {
		in.LineEnd = 0
	}
	if in.LineEnd > 0 && in.LineStart > 0 && in.LineEnd < in.LineStart {
		in.LineEnd = in.LineStart
	}
	in.ID = conventionNodeID(in)
	return in
}

func conventionCategoryFromEvidenceKind(kind string) ConventionCategory {
	switch EvidenceKind(strings.TrimSpace(kind)) {
	case EvidenceMechanism:
		return ConventionCategoryMechanism
	case EvidenceRelationship:
		return ConventionCategoryRelationship
	case EvidenceRegistration:
		return ConventionCategoryRegistration
	default:
		return ""
	}
}

func conventionCategoryFromString(raw string) ConventionCategory {
	switch ConventionCategory(strings.TrimSpace(raw)) {
	case ConventionCategoryLocalPattern,
		ConventionCategoryMechanism,
		ConventionCategoryRelationship,
		ConventionCategoryRegistration:
		return ConventionCategory(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func conventionNodeID(node ConventionNode) string {
	key := strings.Join([]string{
		string(node.Category),
		node.Summary,
		node.Source,
		fmt.Sprintf("%d", node.LineStart),
		node.Subject,
		node.AnchorSymbol,
		node.EvidenceRefID,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		return ""
	}
	return "convention:" + key
}

func normalizeConventionPath(raw string) string {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
