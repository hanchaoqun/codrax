package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// diagramParticipantRuntimeCompactRepair is the per-participant repair text
// for a requested participant that the current runtime rows bind to a
// recorded endpoint. It names one published row and one operation, in the
// row's own published wording. It returns "" unless every mismatch is of
// that kind, so every other participant failure keeps its full guidance.
//
// Only typed data is read: the mismatch issue, the request binding on the
// published rows, the draft's anchor identities and the published additions.
// No relation, arrow, label or wording is chosen for the model.
func diagramParticipantRuntimeCompactRepair(
	doc *types.AnswerDocumentV2,
	mismatches []DiagramParticipantCoverageMismatch,
	rows []RuntimeDiagramRelation,
	relationDelta string,
) string {
	if doc == nil || len(mismatches) == 0 || len(rows) == 0 {
		return ""
	}
	var published types.AnswerDiagramRelationRepairDelta
	if strings.TrimSpace(relationDelta) != "" && json.Unmarshal([]byte(relationDelta), &published) != nil {
		return ""
	}
	presented := runtimeDiagramPresentationRows(rows)
	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		if mismatch.Issue != DiagramParticipantCoverageIdentityMissing && mismatch.Issue != DiagramParticipantCoverageTypedEdgeMissing {
			return ""
		}
		var row *RuntimeDiagramRelation
		field, endpoint, printed := "", "", false
		for i := range presented {
			if diagramParticipantRuntimeRequested(presented[i].requestedTo, mismatch.Participant) {
				row, field, endpoint = &presented[i], "to_node", presented[i].ToLabel
			} else if diagramParticipantRuntimeRequested(presented[i].requestedFrom, mismatch.Participant) {
				row, field, endpoint = &presented[i], "from_node", presented[i].FromLabel
			}
			if row != nil {
				// Only the first rows are printed with their edge_anchor.
				printed = i < runtimeDiagramRecipeRowLimit
				break
			}
		}
		if row == nil {
			return ""
		}
		event := runtimeDiagramRecipeRowText(*row)
		// Another lookup's copy of the same physical row is the same event.
		copies := []RuntimeDiagramRelation{*row}
		if key, ok := runtimeDiagramWakeEventKey(*row); ok {
			copies = copies[:0]
			for _, other := range rows {
				if otherKey, ok := runtimeDiagramWakeEventKey(other); ok && otherKey == key {
					copies = append(copies, other)
				}
			}
		}
		drawn := false
		for _, block := range doc.Blocks {
			for _, anchor := range block.EdgeAnchors {
				for _, copy := range copies {
					drawn = drawn || block.Diagram != nil && anchor.RelationKind == copy.Kind && anchor.FromIdentity == copy.FromIdentity && anchor.ToIdentity == copy.ToIdentity
				}
			}
		}
		boundary := diagramParticipantRuntimeStaleBoundaryClause(doc, mismatch)
		if drawn {
			parts = append(parts, fmt.Sprintf("Participant %q is the recorded endpoint %q. Its arrow (%s) is already drawn but is not accepted as drawn: repair that arrow instead of adding another one%s", mismatch.Participant, endpoint, event, boundary))
			continue
		}
		// Name only an operation that exists: the addition published for
		// this row's source, and the printed row with its edge_anchor.
		var operations []string
		source := strings.Join(row.SupportRefs, "; ")
		for _, candidate := range published.AllowedAdditions {
			// The published addition may carry another lookup's credentials
			// for this same recorded source; its source names the row.
			if candidate.RelationKind == row.Kind && source != "" && candidate.Source == source {
				operations = append(operations, "in a patch use diagram_edge_edits action \"add\" with the addition_ref listed for this row's source")
				break
			}
		}
		if printed {
			operations = append(operations, "in a full re-emit copy this row's edge_anchor from the runtime diagram anchors")
		}
		if len(operations) == 0 {
			return ""
		}
		parts = append(parts, fmt.Sprintf("Participant %q is the recorded endpoint %q, and no accepted arrow of that endpoint is in this diagram. Published row to draw: %s. Add it as one arrow whose %s is that endpoint's lifeline: %s%s", mismatch.Participant, endpoint, event, field, strings.Join(operations, "; "), boundary))
	}
	return strings.Join(parts, ". ") + "."
}

// Once the participant's recorded arrow is accepted, an unproven boundary row
// for that participant contradicts it, so one repair must change both.
func diagramParticipantRuntimeStaleBoundaryClause(doc *types.AnswerDocumentV2, mismatch DiagramParticipantCoverageMismatch) string {
	for _, block := range doc.Blocks {
		if block.ID != mismatch.BlockID {
			continue
		}
		for _, boundary := range block.ParticipantBoundaries {
			if strings.EqualFold(strings.TrimSpace(boundary.Participant), strings.TrimSpace(mismatch.Participant)) {
				return fmt.Sprintf(". The same repair must also drop this participant's unproven boundary row: in that patch send diagram_boundary_replacements for block %q with the complete participant_boundaries array minus that row; in a full re-emit omit that row", block.ID)
			}
		}
	}
	return ""
}

func diagramParticipantRuntimeRequested(participants []string, identity string) bool {
	for _, participant := range participants {
		if participant == identity {
			return true
		}
	}
	return false
}
