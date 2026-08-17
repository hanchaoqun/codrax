package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answerDocNoArrowOwnershipGroup is an exact, source-backed structural
// relation between two requested diagram participants. It deliberately has
// no direction: a declaration can prove containment/ownership, but cannot
// prove a call, transfer, execution order, or runtime causality.
type answerDocNoArrowOwnershipGroup struct {
	owner  string
	member string
	typ    string
	source string
	line   int
}

// answerDocRequestedNoArrowOwnershipGroups collects only parser-stamped or
// checkout-verified ownership declarations whose owner and member/type both
// resolve to distinct typed incident_required participants. This makes the
// projection language-neutral: any supported extractor that publishes the
// same declaration metadata gets the same representation semantics.
//
// The rows are prompt authority only. They never create an edge, satisfy a
// directed-relation boundary, or inspect user/model prose.
func answerDocRequestedNoArrowOwnershipGroups(ctx *types.AgentContext) []answerDocNoArrowOwnershipGroup {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return nil
	}
	participants := make([][]string, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		surfaces := types.DiagramParticipantIdentitySurfaces(rm, participant)
		if identity := strings.TrimSpace(participant.Identity); identity != "" {
			surfaces = append([]string{identity}, surfaces...)
		}
		participants = append(participants, surfaces)
	}
	if len(participants) < 2 {
		return nil
	}
	matchesParticipant := func(identity string) []int {
		matches := make([]int, 0, 1)
		for i, surfaces := range participants {
			matched := false
			for _, surface := range surfaces {
				if types.AnswerCodeIdentitySurfacesCompatible(surface, identity) {
					matched = true
					break
				}
			}
			if matched {
				matches = append(matches, i)
			}
		}
		return matches
	}
	rows := make([]answerDocNoArrowOwnershipGroup, 0, 4)
	seen := make(map[string]bool)
	appendRow := func(row answerDocNoArrowOwnershipGroup) {
		row.owner = strings.TrimSpace(row.owner)
		row.member = strings.TrimSpace(row.member)
		row.typ = strings.TrimSpace(row.typ)
		row.source = normalizedStageBindingSourcePath(row.source)
		ownerMatches := matchesParticipant(row.owner)
		memberMatches := matchesParticipant(row.member)
		if len(memberMatches) != 1 && row.typ != "" {
			memberMatches = matchesParticipant(row.typ)
		}
		if row.owner == "" || row.member == "" || row.source == "" || row.line <= 0 ||
			len(ownerMatches) != 1 || len(memberMatches) != 1 || ownerMatches[0] == memberMatches[0] {
			return
		}
		key := strings.ToLower(row.owner + "\x00" + row.member + "\x00" + row.typ)
		if seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, row)
	}

	for _, item := range answerDocumentAuthorityEvidencePool(ctx) {
		if !item.IsCitable() || item.AnchorKind != types.AnchorDefinition ||
			strings.TrimSpace(item.DeclaredOwner) == "" || strings.TrimSpace(item.DeclaredBinding) == "" ||
			strings.TrimSpace(item.DeclaredType) == "" || types.RuntimeArtifactPathKind(item.Source) != "" {
			continue
		}
		binding := strings.TrimSpace(item.DeclaredBinding)
		member := binding
		if dot := strings.LastIndex(binding, "."); dot >= 0 && dot+1 < len(binding) {
			member = binding[dot+1:]
		}
		appendRow(answerDocNoArrowOwnershipGroup{
			owner: strings.TrimSpace(item.DeclaredOwner), member: member,
			typ: strings.TrimSpace(item.DeclaredType), source: item.Source, line: item.LineStart,
		})
	}

	// The read-mode provider is another exact parser-backed declaration
	// source. It covers shared carrier fields even when the Explorer did not
	// redundantly emit their definition rows.
	if carriers, ok := stageauthority.LoadReadModeStateCarriers(ctx.RepoRoot); ok {
		for _, carrier := range carriers {
			appendRow(answerDocNoArrowOwnershipGroup{
				owner: carrier.Owner, member: carrier.Field, typ: carrier.Type,
				source: carrier.File, line: carrier.Line,
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].source != rows[j].source {
			return rows[i].source < rows[j].source
		}
		if rows[i].line != rows[j].line {
			return rows[i].line < rows[j].line
		}
		if rows[i].owner != rows[j].owner {
			return rows[i].owner < rows[j].owner
		}
		return rows[i].member < rows[j].member
	})
	return rows
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
			i+1, row.owner, row.member, row.typ, row.source, row.line)
		fmt.Fprintf(&b, "  copy_ready_no_arrow_mermaid[%d]:\n    subgraph ownership_group_%d[\"%s\"]\n      ownership_member_%d[\"%s<br/>%s\"]\n    end\n",
			i+1, i+1, answerDocNoArrowMermaidLabel(row.owner), i+1,
			answerDocNoArrowMermaidLabel(row.member), answerDocNoArrowMermaidLabel(row.typ))
	}
	b.WriteString("- A separately proved local operation may be drawn inside the matching participant group with its own exact technical endpoints and anchor. It remains a local fact and cannot close the missing inter-participant transfer. Keep any proved request-scoped spine unchanged in a sibling visual region; do not add an arrow between the spine and the ownership group without its own typed operation.\n")
	b.WriteString("- Reader-facing diagram copy: source locations, recipe/validator terms, raw relation kinds such as `precedence`/`call`, and `@ file:line` suffixes belong in citations or typed anchors, not in visible node/message labels. Write visible labels as concise repository/domain responsibilities and actions; this display guidance cannot add, remove, reverse, or reconnect any relation. The model still authors all visible wording, selection, layout, and conclusions.\n\n")
	return b.String()
}

func answerDocNoArrowMermaidLabel(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
