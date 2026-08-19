package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answerDocRequestedNoArrowOwnershipGroups collects only parser-stamped or
// checkout-verified ownership declarations whose owner and member/type both
// resolve to distinct typed incident_required participants. This makes the
// projection language-neutral: any supported extractor that publishes the
// same declaration metadata gets the same representation semantics.
//
// The rows are prompt authority only. They never create an edge, satisfy a
// directed-relation boundary, or inspect user/model prose.
func answerDocRequestedNoArrowOwnershipGroups(ctx *types.AgentContext) []types.DiagramNoArrowOwnershipGroup {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	declarations := types.DiagramOwnershipDeclarationsFromEvidence(answerDocumentAuthorityEvidencePool(ctx))
	for i := range declarations {
		declarations[i].Source = normalizedStageBindingSourcePath(declarations[i].Source)
	}

	// The read-mode provider is another exact parser-backed declaration
	// source. It covers shared carrier fields even when the Explorer did not
	// redundantly emit their definition rows.
	if carriers, ok := stageauthority.LoadReadModeStateCarriers(ctx.RepoRoot); ok {
		for _, carrier := range carriers {
			declarations = append(declarations, types.DiagramOwnershipDeclaration{
				Owner: carrier.Owner, Member: carrier.Field, Type: carrier.Type,
				Source: normalizedStageBindingSourcePath(carrier.File), Line: carrier.Line,
			})
		}
	}
	return types.RequestedDiagramNoArrowOwnershipGroups(ctx.AnalysisIR.RequestModel, declarations)
}

// renderAnswerDocDiagramStructuralCompositionHandoff keeps two independent
// relation classes visible at the end of the long finalizer prompt: directed
// operation/order edges and declaration-backed no-arrow containment. A
// missing directed bridge must not erase a proved structural relationship.
func renderAnswerDocDiagramStructuralCompositionHandoff(ctx *types.AgentContext) string {
	rows := answerDocRequestedNoArrowOwnershipGroups(ctx)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Final Diagram Structural Composition Handoff\n\n")
	b.WriteString("- Keep directed relation recipes and no-arrow ownership/grouping facts as separate typed layers in the same requested diagram when both help the reader. An unproved call/data-flow bridge forbids only that arrow; it does not erase an independently proved containment relation or require the owner and member to be flattened into unrelated peer nodes.\n")
	b.WriteString("- For each row below, use one visible group/subgraph whose primary label is the exact owner participant and place the exact member participant as a visible nested node or nested group. Add no arrow and no `edge_anchors` row for this containment. If the requested directed relation is still unproved for either participant, retain its `participant_boundaries` row: that boundary and the no-arrow grouping are compatible facts.\n")
	for i, row := range rows {
		fmt.Fprintf(&b, "- no_arrow_ownership_group[%d]: owner=`%s`; member=`%s`; type=`%s`; source=`%s:%d`; directed_relation_authority=`none`; representation=`visible_owner_group_with_visible_nested_member`.\n",
			i+1, row.Owner, row.Member, row.Type, row.Source, row.Line)
		fmt.Fprintf(&b, "  copy_ready_no_arrow_mermaid[%d]:\n    subgraph ownership_group_%d[\"%s\"]\n      ownership_member_%d[\"%s<br/>%s\"]\n    end\n",
			i+1, i+1, answerDocNoArrowMermaidLabel(row.Owner), i+1,
			answerDocNoArrowMermaidLabel(row.Member), answerDocNoArrowMermaidLabel(row.Type))
	}
	b.WriteString("- A separately proved local operation may be drawn inside the matching participant group with its own exact technical endpoints and anchor. It remains a local fact and cannot close the missing inter-participant transfer. Keep any proved request-scoped spine unchanged in a sibling visual region; do not add an arrow between the spine and the ownership group without its own typed operation.\n")
	b.WriteString("- Reader-facing diagram copy: source locations, recipe/validator terms, raw relation kinds such as `precedence`/`call`, and `@ file:line` suffixes belong in citations or typed anchors, not in visible node/message labels. Write visible labels as concise repository/domain responsibilities and actions; this display guidance cannot add, remove, reverse, or reconnect any relation. The model still authors all visible wording, selection, layout, and conclusions.\n\n")
	return b.String()
}

func answerDocNoArrowMermaidLabel(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
