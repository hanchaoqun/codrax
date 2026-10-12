package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeDiagramEndpointLabels is display data keyed only by the current
// producer's exact node ID. Equal names never merge nodes or authorize edges;
// conflicting names for one ID remain unresolved. The relation provider owns
// source, query, window and event qualification, not the Mermaid text.
func runtimeDiagramEndpointLabels(rows []RuntimeDiagramRelation) map[string]string {
	labels, ambiguous := map[string]string{}, map[string]bool{}
	for _, row := range rows {
		for _, endpoint := range [][2]string{{row.FromNode, row.FromLabel}, {row.ToNode, row.ToLabel}} {
			id := strings.TrimSpace(endpoint[0])
			label, err := mermaidcompat.NormalizeExplicitNodeLabel(endpoint[1])
			if id == "" || label == "" || err != nil {
				continue
			}
			if prior, exists := labels[id]; exists && prior != label {
				ambiguous[id] = true
			}
			labels[id] = label
		}
	}
	for id := range ambiguous {
		delete(labels, id)
	}
	return labels
}

// normalizeRuntimeDiagramDefaultEndpointLabels fills only the default display
// of an exact producer node selected by the model: an implicit endpoint, or a
// single declaration whose label is still the same node ID. This also absorbs
// atomic edge edits' system-created label=nodeID fallback. Authored aliases,
// identities, edges, notes and source-less nodes are never rewritten.
func normalizeRuntimeDiagramDefaultEndpointLabels(doc *types.AnswerDocumentV2, rows []RuntimeDiagramRelation) int {
	if doc == nil {
		return 0
	}
	labels := runtimeDiagramEndpointLabels(rows)
	ids := make([]string, 0, len(labels))
	for id := range labels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	fixed := 0
	for i := range doc.Blocks {
		diagram := doc.Blocks[i].Diagram
		if diagram == nil {
			continue
		}
		decls := map[string][]string{}
		sequence := diagramEvidenceUsesSequenceSyntax(diagram.Body, diagram.Kind)
		for _, line := range strings.Split(diagram.Body, "\n") {
			parsed := mermaidcompat.SequenceParticipantDeclarations(line)
			if !sequence {
				parsed = mermaidcompat.NodeDeclarationsAll(line)
			}
			for _, decl := range parsed {
				decls[decl.Ident] = append(decls[decl.Ident], decl.Label)
			}
		}
		used := map[string]bool{}
		for _, edge := range mermaidcompat.ParseEdges(diagram.Body) {
			used[edge.From], used[edge.To] = true, true
		}
		for _, id := range ids {
			if !used[id] {
				continue
			}
			var updated string
			switch {
			case len(decls[id]) == 0:
				updated, _ = mermaidcompat.AddExplicitNodeDeclarationChecked(diagram.Body, id, labels[id])
			case len(decls[id]) == 1 && decls[id][0] == id:
				updated, _ = mermaidcompat.RewriteRemovableNodeDeclarationLabel(diagram.Body, id, labels[id])
				if updated == diagram.Body && !sequence {
					updated = rewriteRuntimeInlineDefaultLabel(diagram.Body, id, labels[id])
				}
			default:
				continue
			}
			if updated != "" && updated != diagram.Body {
				diagram.Body = updated
				fixed++
			}
		}
	}
	return fixed
}

// Reuse the syntax editor on one exact parsed inline token, retaining the
// surrounding statement, shape, classes and arrow bytes. No label substring
// replacement or semantic matching is involved.
func rewriteRuntimeInlineDefaultLabel(body, id, label string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		for _, decl := range mermaidcompat.InlineNodeDeclarations(line) {
			if decl.Ident != id || decl.Label != id || strings.Count(line, decl.Token) != 1 {
				continue
			}
			carrier, n := mermaidcompat.RewriteRemovableNodeDeclarationLabel("flowchart TD\n"+decl.Token, id, label)
			if n != 1 {
				return body
			}
			lines[i] = strings.Replace(line, decl.Token, strings.TrimPrefix(carrier, "flowchart TD\n"), 1)
			return strings.Join(lines, "\n")
		}
	}
	return body
}
