package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

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
	return fixed
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
