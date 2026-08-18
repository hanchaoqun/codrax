package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	answerDocMultiTopicEvidencePerUnitLimit = 16
	answerDocMultiTopicSharedEvidenceLimit  = 12
)

type answerDocInvestigationEvidenceOwner struct {
	unitIDs map[string]bool
	item    types.EvidenceItem
}

// renderAnswerDocMultiTopicEvidenceOwnership preserves the typed producer
// lineage that already exists between topic evidence nodes and EvidenceItems.
// It is prompt-only guidance: ownership says where a fact was investigated,
// not that two facts form one mechanism or that either fact is a conclusion.
func renderAnswerDocMultiTopicEvidenceOwnership(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return ""
	}
	plan := types.CompileInvestigationPlan(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract)
	if len(plan.Units) < 2 {
		return ""
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return ""
	}
	ledger := closure.NodeArtifactLedger()
	if ledger.Empty() {
		return ""
	}

	evidence := answerDocTypedEnrichmentEvidencePool(ctx, answerDocMaxEnrichmentCandidateFacts)
	byID := make(map[string]*answerDocInvestigationEvidenceOwner, len(evidence))
	for _, item := range evidence {
		if item.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		id := types.RuntimeArtifactIDForEvidenceItem(item)
		if id == "" {
			continue
		}
		byID[id] = &answerDocInvestigationEvidenceOwner{
			unitIDs: map[string]bool{},
			item:    item,
		}
	}
	for _, record := range ledger.RecordsByKind(types.RuntimeArtifactEvidenceItem) {
		id := strings.TrimSpace(record.EvidenceID)
		if id == "" {
			id = strings.TrimSpace(record.Artifact.ID)
		}
		owned := byID[id]
		if owned == nil {
			continue
		}
		unit, ok := plan.InvestigationUnitForEvidenceNode(record.ProducerNodeID)
		if !ok {
			continue
		}
		owned.unitIDs[unit.ID] = true
	}

	unitRows := make(map[string][]answerDocInvestigationEvidenceOwner, len(plan.Units))
	var shared []answerDocInvestigationEvidenceOwner
	for _, owned := range byID {
		switch len(owned.unitIDs) {
		case 0:
			shared = append(shared, *owned)
		case 1:
			for unitID := range owned.unitIDs {
				unitRows[unitID] = append(unitRows[unitID], *owned)
			}
		default:
			shared = append(shared, *owned)
		}
	}
	hasOwned := false
	for _, unit := range plan.Units {
		if len(unitRows[unit.ID]) > 0 {
			hasOwned = true
			break
		}
	}
	if !hasOwned && len(shared) == 0 {
		return ""
	}

	sortEvidenceOwners := func(rows []answerDocInvestigationEvidenceOwner) {
		sort.SliceStable(rows, func(i, j int) bool {
			return types.RuntimeArtifactIDForEvidenceItem(rows[i].item) < types.RuntimeArtifactIDForEvidenceItem(rows[j].item)
		})
	}

	var b strings.Builder
	b.WriteString("## Evidence ownership by investigation unit (prompt-only)\n\n")
	b.WriteString("- The grouping below comes from typed task-node artifact lineage. Use it to keep facts with the investigation unit that produced them; do not echo this heading, unit IDs, evidence IDs, or connectivity labels in the user-facing answer.\n")
	b.WriteString("- Unit ownership is not a call, causal, precedence, fallback, or failure-path edge. Join facts into one mechanism only when an explicit typed relation row supplies the exact endpoints and direction.\n")
	b.WriteString("- `connectivity=standalone_fact` rows may explain a definition, state, option, or local behavior, but they cannot by themselves become a transition in a mechanism chain. `connectivity=explicit_typed_relation` authorizes only the displayed subject-to-object relation, not a broader category or neighboring step.\n")
	b.WriteString("- Evidence assigned to another unit, shared probe evidence, and disconnected standalone facts remain supporting context unless a typed relation independently connects them to the current section.\n\n")

	for _, unit := range plan.Units {
		rows := unitRows[unit.ID]
		if len(rows) == 0 {
			continue
		}
		sortEvidenceOwners(rows)
		fmt.Fprintf(&b, "### Unit %d: %s\n\n", unit.Index, answerDocInlineClip(firstNonEmptyAnswerDocUnitLabel(unit), 180))
		limit := len(rows)
		if limit > answerDocMultiTopicEvidencePerUnitLimit {
			limit = answerDocMultiTopicEvidencePerUnitLimit
		}
		for _, owned := range rows[:limit] {
			b.WriteString(renderAnswerDocInvestigationEvidenceOwnerRow(owned.item))
		}
		if len(rows) > limit {
			fmt.Fprintf(&b, "- ... %d additional owned evidence rows remain available in the main Evidence Items section.\n", len(rows)-limit)
		}
		b.WriteString("\n")
	}

	if len(shared) > 0 {
		sortEvidenceOwners(shared)
		b.WriteString("### Shared or not uniquely assigned evidence\n\n")
		limit := len(shared)
		if limit > answerDocMultiTopicSharedEvidenceLimit {
			limit = answerDocMultiTopicSharedEvidenceLimit
		}
		for _, owned := range shared[:limit] {
			b.WriteString(renderAnswerDocInvestigationEvidenceOwnerRow(owned.item))
		}
		if len(shared) > limit {
			fmt.Fprintf(&b, "- ... %d additional shared evidence rows remain available in the main Evidence Items section.\n", len(shared)-limit)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func firstNonEmptyAnswerDocUnitLabel(unit types.InvestigationUnit) string {
	if label := strings.TrimSpace(unit.Label); label != "" {
		return label
	}
	if summary := strings.TrimSpace(unit.Summary); summary != "" {
		return summary
	}
	return fmt.Sprintf("unit-%d", unit.Index)
}

func renderAnswerDocInvestigationEvidenceOwnerRow(item types.EvidenceItem) string {
	id := types.RuntimeArtifactIDForEvidenceItem(item)
	connectivity := "standalone_fact"
	if answerDocEvidenceHasExplicitTypedRelation(item) {
		connectivity = "explicit_typed_relation"
	}
	surface := strings.TrimSpace(types.EvidenceAuthoritativeSurfaceText(item, false))
	if surface == "" {
		surface = firstNonEmptyAnswerDocEvidenceLabel(item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object)
	}
	surface = answerDocInlineClip(surface, 240)
	loc := item.DisplayLocation(true)
	if loc != "" {
		return fmt.Sprintf("- evidence_id=`%s` connectivity=`%s` @ %s: %s\n", id, connectivity, loc, surface)
	}
	return fmt.Sprintf("- evidence_id=`%s` connectivity=`%s`: %s\n", id, connectivity, surface)
}

func firstNonEmptyAnswerDocEvidenceLabel(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "grounded evidence"
}

func answerDocEvidenceHasExplicitTypedRelation(item types.EvidenceItem) bool {
	if strings.TrimSpace(item.Subject) == "" || strings.TrimSpace(item.Object) == "" {
		return false
	}
	switch item.AnchorKind {
	case types.AnchorCall, types.AnchorCallback, types.AnchorPrecedence:
		return true
	}
	switch item.Kind {
	case types.EvidenceRelationship, types.EvidenceRegistration, types.EvidenceDataflowPath, types.EvidenceControlFlow:
		return true
	default:
		return false
	}
}
