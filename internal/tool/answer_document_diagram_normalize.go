package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeOrphanDiagramEdgeAnchors removes diagram-only metadata after the
// model intentionally removes every typed diagram block. Edge anchors may live
// on a sibling prose/list block while they own endpoints in another diagram,
// but with no diagram anywhere in the document there is no body alias or
// visible relation left to own. Keeping those orphan anchors would turn a
// successful optional-diagram removal into a second, invisible relation claim
// and an avoidable hard retry.
//
// This is a structural JSON repair only: it never edits answer prose, diagram
// source, citations, conclusions, or relation metadata while any typed diagram
// remains present.
func normalizeOrphanDiagramEdgeAnchors(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockDiagram && doc.Blocks[i].Diagram != nil {
			return 0
		}
	}
	removed := 0
	for i := range doc.Blocks {
		removed += len(doc.Blocks[i].EdgeAnchors)
		doc.Blocks[i].EdgeAnchors = nil
	}
	return removed
}

func normalizeDiagramEdgeAnchorMetadata(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fixed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		aliases := diagramNodeAliasIndex(block.Diagram.Body)
		for j := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[j]
			if resolved := aliases[diagramSurfaceKey(anchor.FromNode)]; resolved != "" && anchor.FromNode != resolved {
				anchor.FromNode = resolved
				fixed++
			}
			if resolved := aliases[diagramSurfaceKey(anchor.ToNode)]; resolved != "" && anchor.ToNode != resolved {
				anchor.ToNode = resolved
				fixed++
			}
			rel := anchor.RelationKind
			if !rel.IsValid() {
				rel = types.RelationForClaimForm(anchor.ClaimForm)
				if rel.IsValid() {
					anchor.RelationKind = rel
					fixed++
				}
			}
			if rel.IsValid() {
				wantClaim := types.ClaimFormForRelation(rel)
				if wantClaim != types.ClaimUnknown && anchor.ClaimForm != wantClaim {
					anchor.ClaimForm = wantClaim
					fixed++
				}
			}
		}
	}
	// Edge anchors may intentionally live on a sibling prose/list block. The
	// Mermaid syntax normalizer can replace a nonportable code identity in the
	// diagram body with codraxNodeN while preserving that identity as the
	// visible label. Resolve such sibling metadata only when the exact label
	// pair maps to one visible directed edge in exactly one diagram block.
	// Ambiguous/reused labels remain untouched and fail through the ordinary
	// ownership gate; no edge, direction, relation kind, or evidence is minted.
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		for j := range block.EdgeAnchors {
			anchor := &block.EdgeAnchors[j]
			from, to, ok := diagramUniqueVisibleAliasPair(doc, anchor.FromNode, anchor.ToNode)
			if !ok {
				continue
			}
			if anchor.FromNode != from {
				anchor.FromNode = from
				fixed++
			}
			if anchor.ToNode != to {
				anchor.ToNode = to
				fixed++
			}
		}
	}
	return fixed
}

func diagramUniqueVisibleAliasPair(doc *types.AnswerDocumentV2, rawFrom, rawTo string) (string, string, bool) {
	if doc == nil || strings.TrimSpace(rawFrom) == "" || strings.TrimSpace(rawTo) == "" {
		return "", "", false
	}
	var resolvedFrom, resolvedTo string
	matches := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		aliases := diagramNodeAliasIndex(block.Diagram.Body)
		from := aliases[diagramSurfaceKey(rawFrom)]
		to := aliases[diagramSurfaceKey(rawTo)]
		if from == "" || to == "" {
			continue
		}
		visible := false
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if diagramEvidenceEdgeKey(edge.From, edge.To) == diagramEvidenceEdgeKey(from, to) {
				visible = true
				break
			}
		}
		if !visible {
			continue
		}
		matches++
		if matches > 1 {
			return "", "", false
		}
		resolvedFrom, resolvedTo = from, to
	}
	return resolvedFrom, resolvedTo, matches == 1
}

func diagramNodeAliasIndex(body string) map[string]string {
	values := map[string]string{}
	ambiguous := map[string]bool{}
	add := func(surface, ident string) {
		key := diagramSurfaceKey(surface)
		ident = strings.TrimSpace(ident)
		if key == "" || ident == "" || ambiguous[key] {
			return
		}
		if existing := values[key]; existing != "" && existing != ident {
			delete(values, key)
			ambiguous[key] = true
			return
		}
		values[key] = ident
	}
	for _, line := range strings.Split(body, "\n") {
		for _, decl := range mermaidcompat.SequenceParticipantDeclarations(line) {
			add(decl.Ident, decl.Ident)
			add(decl.Label, decl.Ident)
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			add(decl.Ident, decl.Ident)
			add(decl.Label, decl.Ident)
		}
	}
	return values
}

func diagramSurfaceKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
