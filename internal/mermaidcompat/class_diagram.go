package mermaidcompat

import (
	"strings"
)

// NormalizeClassDiagramToFlowchart converts the portable, directed subset of
// Mermaid classDiagram into the flowchart carrier understood by every Codrax
// renderer. The pass is deliberately syntax-only: it preserves authored node
// identities, class-member text, relation direction, relation labels, and the
// relation meaning encoded by the UML operator. It does not inspect answer
// prose, infer source facts, add nodes or edges, or decide whether a relation
// is grounded.
//
// Unsupported classDiagram constructs fail open by returning the original
// body with converted=false. That keeps richer Mermaid source available to a
// browser renderer instead of partially rewriting a diagram and silently
// losing information.
func NormalizeClassDiagramToFlowchart(body string) (normalized string, converted bool) {
	lines := strings.Split(body, "\n")
	if !classDiagramDirectivePresent(lines) {
		return body, false
	}

	type classNode struct {
		ident   string
		members []string
	}
	var (
		comments  []string
		nodes     []classNode
		edges     []Edge
		inside    *classNode
		nodeIndex = make(map[string]int)
	)

	appendNode := func(node classNode) bool {
		if !portableClassDiagramIdentifier(node.ident) {
			return false
		}
		if index, exists := nodeIndex[node.ident]; exists {
			// Mermaid permits separate declarations and repeated class bodies.
			// Its class database appends later members to the existing class;
			// preserve that order and every authored member, including repeats.
			nodes[index].members = append(nodes[index].members, node.members...)
			return true
		}
		nodeIndex[node.ident] = len(nodes)
		nodes = append(nodes, node)
		return true
	}

	directiveSeen := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "%%") {
			comments = append(comments, trimmed)
			continue
		}
		if !directiveSeen {
			if !strings.EqualFold(trimmed, "classDiagram") {
				return body, false
			}
			directiveSeen = true
			continue
		}
		if inside != nil {
			if trimmed == "}" {
				if !appendNode(*inside) {
					return body, false
				}
				inside = nil
				continue
			}
			// Nested or one-line class bodies are outside the lossless subset.
			if strings.Contains(trimmed, "{") || strings.Contains(trimmed, "}") {
				return body, false
			}
			inside.members = append(inside.members, trimmed)
			continue
		}

		if strings.HasPrefix(trimmed, "class ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "class "))
			if strings.HasSuffix(rest, "{") {
				ident := strings.TrimSpace(strings.TrimSuffix(rest, "{"))
				if !portableClassDiagramIdentifier(ident) {
					return body, false
				}
				inside = &classNode{ident: ident}
				continue
			}
			if !appendNode(classNode{ident: rest}) {
				return body, false
			}
			continue
		}

		from, to, label, operator, ok := splitClassDiagramEdgeLine(trimmed)
		// Cardinalities are meaningful relationship data. The flowchart subset
		// has no equivalent endpoint-cardinality grammar, so leave such a
		// diagram untouched instead of deleting that information.
		if strings.ContainsAny(trimmed, `"'`) || !ok ||
			!portableClassDiagramIdentifier(from) || !portableClassDiagramIdentifier(to) {
			return body, false
		}
		// UML operator semantics and an authored label carry independent
		// information. The portable arrow has no equivalent operator, and its
		// label slot is already occupied. Keep the native source rather than
		// dropping either fact or rewriting the author's label to combine them.
		if strings.TrimSpace(label) != "" && operator != "-->" && operator != "<--" {
			return body, false
		}
		edges = append(edges, Edge{From: from, To: to, Label: label, Operator: operator})
	}
	if !directiveSeen || inside != nil || (len(nodes) == 0 && len(edges) == 0) {
		return body, false
	}

	// Relation endpoints need declarations in the flowchart carrier, but this
	// does not invent entities: each endpoint is already authored in a visible
	// classDiagram edge.
	for _, edge := range edges {
		appendNode(classNode{ident: edge.From})
		appendNode(classNode{ident: edge.To})
	}

	out := make([]string, 0, 1+len(comments)+len(nodes)+len(edges))
	out = append(out, "flowchart TD")
	for _, comment := range comments {
		out = append(out, "    "+comment)
	}
	for _, node := range nodes {
		labelParts := append([]string{escapeFlowchartClassLabel(node.ident)}, escapedFlowchartClassMembers(node.members)...)
		label := strings.Join(labelParts, "<br/>")
		out = append(out, "    "+node.ident+`["`+label+`"]`)
	}
	for _, edge := range edges {
		label := strings.TrimSpace(edge.Label)
		if label == "" {
			label = classDiagramOperatorLabel(edge.Operator)
		}
		label = escapeFlowchartClassLabel(label)
		out = append(out, "    "+edge.From+` -->|"`+label+`"| `+edge.To)
	}
	return strings.Join(out, "\n"), true
}

func escapedFlowchartClassMembers(members []string) []string {
	if len(members) == 0 {
		return nil
	}
	out := make([]string, len(members))
	for i, member := range members {
		out[i] = escapeFlowchartClassLabel(member)
	}
	return out
}

func escapeFlowchartClassLabel(label string) string {
	label = strings.ReplaceAll(label, "&", "&amp;")
	label = strings.ReplaceAll(label, "<", "&lt;")
	label = strings.ReplaceAll(label, ">", "&gt;")
	label = strings.ReplaceAll(label, `"`, "&quot;")
	return label
}

func classDiagramDirectivePresent(lines []string) bool {
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		return strings.EqualFold(trimmed, "classDiagram")
	}
	return false
}

func portableClassDiagramIdentifier(ident string) bool {
	if ident == "" {
		return false
	}
	for i, r := range ident {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func classDiagramOperatorLabel(operator string) string {
	switch operator {
	case "<|..", "..|>":
		return "implements"
	case "<|--", "--|>":
		return "extends"
	case "*--", "--*":
		return "composition"
	case "o--", "--o":
		return "aggregation"
	case "<..", "..>":
		return "dependency"
	default:
		return "association"
	}
}
