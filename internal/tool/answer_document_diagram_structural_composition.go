package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func preEmitRequestedNoArrowOwnershipGroups(pctx *preEmitCheckContext) []types.DiagramNoArrowOwnershipGroup {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.AnalysisIR == nil {
		return nil
	}
	declarations := types.DiagramOwnershipDeclarationsFromEvidence(pctx.evidenceItems())
	if carriers, ok := stageauthority.LoadReadModeStateCarriers(pctx.ctx.RepoRoot); ok {
		for _, carrier := range carriers {
			declarations = append(declarations, types.DiagramOwnershipDeclaration{
				Owner: carrier.Owner, Member: carrier.Field, Type: carrier.Type,
				Source: carrier.File, Line: carrier.Line,
			})
		}
	}
	return types.RequestedDiagramNoArrowOwnershipGroups(pctx.ctx.AnalysisIR.RequestModel, declarations)
}

// preCheckDiagramNoArrowOwnershipDirection checks only an ownership relation
// the model has chosen to express through subgraph nesting. Ordinary visual
// groups and absent/peer ownership presentations remain outside the gate. A
// reverse nesting of the exact typed owner/member pair is a precise structural
// contradiction, so the repair asks the model to fix or remove that grouping;
// the system never moves nodes or authors a replacement diagram.
func preCheckDiagramNoArrowOwnershipDirection(doc *types.AnswerDocumentV2, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil {
		return nil
	}
	rows := preEmitRequestedNoArrowOwnershipGroups(pctx)
	if len(rows) == 0 {
		return nil
	}
	type mismatch struct {
		blockID string
		owner   string
		member  string
		typ     string
		source  string
		line    int
	}
	var mismatches []mismatch
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		subgraphs := mermaidcompat.ParseSubgraphs(block.Diagram.Body)
		if len(subgraphs) == 0 {
			continue
		}
		for _, row := range rows {
			ownerSurfaces := []string{row.Owner}
			memberSurfaces := []string{row.Member, row.Type}
			if diagramSubgraphHierarchyContains(subgraphs, memberSurfaces, ownerSurfaces) {
				mismatches = append(mismatches, mismatch{
					blockID: block.ID, owner: row.Owner, member: row.Member, typ: row.Type,
					source: row.Source, line: row.Line,
				})
			}
		}
	}
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, item := range mismatches {
		parts = append(parts, fmt.Sprintf(
			"block=%q currently nests owner=%q under member/type=%q/%q; typed declaration requires owner=%q to contain member=%q (source=%s:%d)",
			item.blockID, item.owner, item.member, item.typ, item.owner, item.member, item.source, item.line,
		))
	}
	return []emitFixHint{{
		Field:      "blocks[kind=diagram].diagram.body subgraph nesting",
		HardSignal: preEmitHardSignalTypedDiagramParticipantCoverage,
		ExpectedShape: "for each listed exact declaration-backed ownership pair that the diagram chooses to group, make the exact owner the visible parent subgraph and the exact member/type a visible nested node or nested subgroup; use no arrow and no edge_anchors row for containment. Correct or remove only the reversed grouping and preserve unrelated model-authored edges, groups, prose, citations, and boundaries: " +
			strings.Join(parts, "; "),
		Reason: "the model-authored subgraph hierarchy visibly asserts containment direction. The same parser/checkout-owned declaration that produced the no-arrow recipe proves the opposite owner/member order; this check reads typed participants, declaration fields, and Mermaid hierarchy only, and never creates, moves, or rewrites a diagram node.",
	}}
}

func diagramSubgraphHierarchyContains(subgraphs []mermaidcompat.Subgraph, containerSurfaces, nestedSurfaces []string) bool {
	for containerIndex, group := range subgraphs {
		if !diagramStructuralIdentityMatches(group.Ident, group.Label, containerSurfaces) {
			continue
		}
		for nestedIndex, nested := range subgraphs {
			if nestedIndex != containerIndex && diagramSubgraphDescendsFrom(subgraphs, nestedIndex, containerIndex) &&
				diagramStructuralIdentityMatches(nested.Ident, nested.Label, nestedSurfaces) {
				return true
			}
		}
		for candidateIndex, candidate := range subgraphs {
			if candidateIndex != containerIndex && !diagramSubgraphDescendsFrom(subgraphs, candidateIndex, containerIndex) {
				continue
			}
			for _, node := range candidate.Nodes {
				if diagramStructuralIdentityMatches(node.Ident, node.Label, nestedSurfaces) {
					return true
				}
			}
		}
	}
	return false
}

func diagramSubgraphDescendsFrom(subgraphs []mermaidcompat.Subgraph, child, ancestor int) bool {
	for child >= 0 && child < len(subgraphs) {
		parent := subgraphs[child].ParentIndex
		if parent == ancestor {
			return true
		}
		if parent < 0 || parent == child {
			return false
		}
		child = parent
	}
	return false
}

func diagramStructuralIdentityMatches(ident, label string, surfaces []string) bool {
	for _, candidate := range []string{strings.TrimSpace(ident), strings.TrimSpace(label)} {
		if candidate == "" {
			continue
		}
		for _, surface := range surfaces {
			if strings.TrimSpace(surface) != "" && types.AnswerCodeIdentitySurfacesCompatible(surface, candidate) {
				return true
			}
		}
	}
	return false
}
