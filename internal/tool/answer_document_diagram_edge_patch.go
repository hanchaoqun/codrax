package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the resource bound explicit in both the projected JSON schema and the
// executor. A normal diagram repair may legitimately need one operation per
// visible edge (including body-only edges that were rejected for missing
// anchors), so the old limit of 16 made some fully validated transactions
// impossible to express. The surrounding answer/block/body size limits remain
// the primary payload bounds; this cap is a final defence against pathological
// edit arrays.
const maxModelAuthoredDiagramEdgeEdits = 128
const maxModelAuthoredDiagramParticipantEdits = 64

type resolvedAtomicDiagramEdgeEdit struct {
	edit          emitAnswerDiagramEdgeEdit
	originalIndex int
	sharedRemove  []emitAnswerDiagramEdgeEdit
	skip          bool
}

type atomicDiagramParticipantDispositionRosterRow struct {
	BlockID       string `json:"block_id"`
	ParticipantID string `json:"participant_id"`
	Issue         string `json:"issue"`
	Detail        string `json:"detail,omitempty"`
}

type atomicDiagramParticipantDispositionRosterError struct {
	Version    int                                            `json:"version"`
	Missing    []atomicDiagramParticipantDispositionRosterRow `json:"missing,omitempty"`
	Unexpected []atomicDiagramParticipantDispositionRosterRow `json:"unexpected,omitempty"`
}

func (e *atomicDiagramParticipantDispositionRosterError) Error() string {
	if e == nil {
		return "diagram participant disposition roster mismatch"
	}
	formatRows := func(rows []atomicDiagramParticipantDispositionRosterRow) string {
		parts := make([]string, 0, len(rows))
		for _, row := range rows {
			item := fmt.Sprintf("%s/%s", row.BlockID, row.ParticipantID)
			if strings.TrimSpace(row.Detail) != "" {
				item += ": " + row.Detail
			}
			parts = append(parts, item)
		}
		return strings.Join(parts, "; ")
	}
	parts := make([]string, 0, 2)
	if len(e.Missing) > 0 {
		parts = append(parts, "missing isolated-participant decisions=["+formatRows(e.Missing)+"]")
	}
	if len(e.Unexpected) > 0 {
		parts = append(parts, "unexpected/ineligible decisions=["+formatRows(e.Unexpected)+"]")
	}
	return "diagram participant disposition roster mismatch: " + strings.Join(parts, "; ") +
		"; submit exactly one model-owned remove_if_isolated or retain_as_context decision for every missing row, and omit every unexpected row"
}

func (e *atomicDiagramParticipantDispositionRosterError) canonicalJSON() string {
	if e == nil {
		return ""
	}
	copyErr := *e
	copyErr.Version = 1
	sortRows := func(rows []atomicDiagramParticipantDispositionRosterRow) []atomicDiagramParticipantDispositionRosterRow {
		out := append([]atomicDiagramParticipantDispositionRosterRow(nil), rows...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].BlockID != out[j].BlockID {
				return out[i].BlockID < out[j].BlockID
			}
			if out[i].ParticipantID != out[j].ParticipantID {
				return out[i].ParticipantID < out[j].ParticipantID
			}
			if out[i].Issue != out[j].Issue {
				return out[i].Issue < out[j].Issue
			}
			return out[i].Detail < out[j].Detail
		})
		return out
	}
	copyErr.Missing = sortRows(copyErr.Missing)
	copyErr.Unexpected = sortRows(copyErr.Unexpected)
	raw, err := json.Marshal(copyErr)
	if err != nil {
		return ""
	}
	return string(raw)
}

func atomicDiagramParticipantDispositionRosterMetadata(err error) (rosterJSON, progressSignature string) {
	roster, ok := err.(*atomicDiagramParticipantDispositionRosterError)
	if !ok || roster == nil {
		return "", ""
	}
	rosterJSON = roster.canonicalJSON()
	if rosterJSON == "" {
		return "", ""
	}
	sum := sha256.Sum256([]byte(rosterJSON))
	return rosterJSON, fmt.Sprintf("v1:%x", sum[:])
}

// applyModelAuthoredDiagramAtomicEdits turns model-declared edge and boundary
// operations into full block replacements over the previous model-authored
// carrier. This is a structural patch compiler, not an answer normalizer: the
// model supplies every target tuple, operation, replacement tuple, visible
// label, and uncertainty boundary. The compiler preserves all unmentioned
// block fields and Mermaid lines, then the ordinary relation and participant
// evidence gates validate the merged document.
func applyModelAuthoredDiagramAtomicEdits(
	prev *types.AnswerDocumentV2,
	patch *types.AnswerDocumentV2Patch,
	edits []emitAnswerDiagramEdgeEdit,
	boundaries []emitAnswerDiagramBoundaryReplacement,
	lease *types.AnswerDiagramRelationRepairLease,
	stagePrecedenceOpt ...[]stageauthority.PrecedenceRelation,
) error {
	return applyModelAuthoredDiagramAtomicEditsWithParticipants(
		prev, patch, edits, boundaries, nil, nil, lease, stagePrecedenceOpt...,
	)
}

func applyModelAuthoredDiagramAtomicEditsWithParticipants(
	prev *types.AnswerDocumentV2,
	patch *types.AnswerDocumentV2Patch,
	edits []emitAnswerDiagramEdgeEdit,
	boundaries []emitAnswerDiagramBoundaryReplacement,
	participantEdits []emitAnswerDiagramParticipantEdit,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
	stagePrecedenceOpt ...[]stageauthority.PrecedenceRelation,
) error {
	return applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
		prev, patch, edits, boundaries, nil, participantEdits, protectedParticipants, lease, stagePrecedenceOpt...,
	)
}

func applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
	prev *types.AnswerDocumentV2,
	patch *types.AnswerDocumentV2Patch,
	edits []emitAnswerDiagramEdgeEdit,
	boundaries []emitAnswerDiagramBoundaryReplacement,
	boundaryEdits []emitAnswerDiagramBoundaryEdit,
	participantEdits []emitAnswerDiagramParticipantEdit,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
	stagePrecedenceOpt ...[]stageauthority.PrecedenceRelation,
) error {
	if prev == nil || patch == nil {
		return fmt.Errorf("previous answer and patch are required")
	}
	if len(edits) == 0 && len(boundaries) == 0 && len(boundaryEdits) == 0 && len(participantEdits) == 0 {
		return nil
	}
	if len(edits) > maxModelAuthoredDiagramEdgeEdits {
		return fmt.Errorf("too many edits: got %d, max %d", len(edits), maxModelAuthoredDiagramEdgeEdits)
	}
	if len(participantEdits) > maxModelAuthoredDiagramParticipantEdits {
		return fmt.Errorf("too many participant edits: got %d, max %d", len(participantEdits), maxModelAuthoredDiagramParticipantEdits)
	}
	var stagePrecedence []stageauthority.PrecedenceRelation
	if len(stagePrecedenceOpt) > 0 {
		stagePrecedence = stagePrecedenceOpt[0]
	}
	previous := make(map[string]types.AnswerBlock, len(prev.Blocks))
	ambiguous := make(map[string]bool)
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previous[id]; exists {
			ambiguous[id] = true
			continue
		}
		previous[id] = block
	}
	claimed := make(map[string]string)
	for _, block := range patch.ReplaceBlocks {
		claimed[strings.TrimSpace(block.ID)] = "replace_blocks"
	}
	for _, block := range patch.AddBlocks {
		claimed[strings.TrimSpace(block.ID)] = "add_blocks"
	}
	for _, id := range patch.RemoveBlockIDs {
		claimed[strings.TrimSpace(id)] = "remove_block_ids"
	}

	working := make(map[string]types.AnswerBlock)
	order := make([]string, 0, len(edits)+len(boundaries)+len(boundaryEdits)+len(participantEdits))
	usedFailureRefs := make(map[string]bool, len(edits))
	usedAdditionRefs := make(map[string]bool, len(edits))
	loadBlock := func(blockID string, index int, field string, requireDiagram bool) (types.AnswerBlock, error) {
		if block, exists := working[blockID]; exists {
			if requireDiagram && (block.Kind != types.BlockDiagram || block.Diagram == nil || strings.TrimSpace(block.Diagram.Body) == "") {
				return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q is not an existing diagram carrier", field, index, blockID)
			}
			return block, nil
		}
		if op, exists := claimed[blockID]; exists {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q conflicts with %s", field, index, blockID, op)
		}
		base, ok := previous[blockID]
		if !ok || ambiguous[blockID] {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q does not uniquely select a previous block", field, index, blockID)
		}
		if requireDiagram && (base.Kind != types.BlockDiagram || base.Diagram == nil || strings.TrimSpace(base.Diagram.Body) == "") {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q is not an existing diagram carrier", field, index, blockID)
		}
		block := cloneAtomicDiagramPatchBlock(base)
		working[blockID] = block
		order = append(order, blockID)
		return block, nil
	}
	resolvedEdits := make([]resolvedAtomicDiagramEdgeEdit, 0, len(edits))
	for i, edit := range edits {
		failureRef := strings.TrimSpace(edit.FailureRef)
		if failureRef != "" {
			if usedFailureRefs[failureRef] {
				return fmt.Errorf("diagram_edge_edits[%d] reuses failure_ref=%q; each live failure may be consumed at most once", i, failureRef)
			}
			usedFailureRefs[failureRef] = true
		}
		additionRef := strings.TrimSpace(edit.AdditionRef)
		if additionRef != "" {
			if usedAdditionRefs[additionRef] {
				return fmt.Errorf("diagram_edge_edits[%d] reuses addition_ref=%q; each live allowed addition may be selected at most once", i, additionRef)
			}
			usedAdditionRefs[additionRef] = true
		}
		var err error
		edit, err = resolveAtomicDiagramFailureRef(edit, lease)
		if err != nil {
			return fmt.Errorf("diagram_edge_edits[%d]: %w", i, err)
		}
		blockID := strings.TrimSpace(edit.BlockID)
		if blockID == "" {
			return fmt.Errorf("edit[%d] has empty block_id", i)
		}
		resolvedEdits = append(resolvedEdits, resolvedAtomicDiagramEdgeEdit{edit: edit, originalIndex: i})
	}
	// Every body_occurrence is minted against the immutable rejected draft.
	// Removing or replacing occurrence 1 first would renumber occurrence 2 in
	// the working Mermaid body and make an otherwise complete atomic repair
	// impossible. Execute only exact same-pair body selections from the highest
	// base occurrence downward. This changes no model-authored action, edge,
	// label, or relation; it merely preserves the stable meaning of the typed
	// selectors while applying the transaction.
	groups := make(map[string][]int)
	for i, item := range resolvedEdits {
		if item.edit.Match == nil || item.edit.BodyOccurrence <= 0 ||
			strings.EqualFold(strings.TrimSpace(item.edit.Action), "add") {
			continue
		}
		key := strings.Join([]string{
			strings.TrimSpace(item.edit.BlockID),
			strings.TrimSpace(item.edit.Match.FromNode),
			strings.TrimSpace(item.edit.Match.ToNode),
		}, "\x00")
		groups[key] = append(groups[key], i)
	}
	for key, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		members := make([]resolvedAtomicDiagramEdgeEdit, len(indexes))
		for i, index := range indexes {
			members[i] = resolvedEdits[index]
		}
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].edit.BodyOccurrence > members[j].edit.BodyOccurrence
		})
		for start := 0; start < len(members); {
			end := start + 1
			for end < len(members) && members[end].edit.BodyOccurrence == members[start].edit.BodyOccurrence {
				end++
			}
			if end-start > 1 {
				if err := validateAtomicSharedBodyRemove(members[start:end]); err != nil {
					return fmt.Errorf(
						"diagram_edge_edits[%d..%d] select the same base body_occurrence=%d for carrier=%q: %w",
						members[start].originalIndex, members[end-1].originalIndex,
						members[start].edit.BodyOccurrence, key, err,
					)
				}
				members[start].sharedRemove = make([]emitAnswerDiagramEdgeEdit, 0, end-start)
				for i := start; i < end; i++ {
					members[start].sharedRemove = append(members[start].sharedRemove, members[i].edit)
					if i > start {
						members[i].skip = true
					}
				}
			}
			start = end
		}
		for i, index := range indexes {
			resolvedEdits[index] = members[i]
		}
	}
	for _, resolved := range resolvedEdits {
		if resolved.skip {
			continue
		}
		edit, i := resolved.edit, resolved.originalIndex
		blockID := strings.TrimSpace(edit.BlockID)
		block, err := loadBlock(blockID, i, "diagram_edge_edits", false)
		if err != nil {
			return err
		}
		if len(resolved.sharedRemove) > 0 {
			if err := applyAtomicSharedBodyRemove(&block, resolved.sharedRemove); err != nil {
				return fmt.Errorf("edit[%d] block_id=%q: %w", i, blockID, err)
			}
			working[blockID] = block
			continue
		}
		if err := applyOneModelAuthoredDiagramEdgeEdit(&block, edit, lease, stagePrecedence); err != nil {
			return fmt.Errorf("edit[%d] block_id=%q: %w", i, blockID, err)
		}
		working[blockID] = block
	}
	boundarySeen := make(map[string]bool, len(boundaries))
	for i, replacement := range boundaries {
		blockID := strings.TrimSpace(replacement.BlockID)
		if blockID == "" {
			return fmt.Errorf("diagram_boundary_replacements[%d] has empty block_id", i)
		}
		if boundarySeen[blockID] {
			return fmt.Errorf("diagram_boundary_replacements[%d] duplicates block_id=%q", i, blockID)
		}
		boundarySeen[blockID] = true
		block, err := loadBlock(blockID, i, "diagram_boundary_replacements", true)
		if err != nil {
			return err
		}
		seenParticipants := make(map[string]bool, len(replacement.ParticipantBoundaries))
		for j, boundary := range replacement.ParticipantBoundaries {
			participant := strings.TrimSpace(boundary.Participant)
			if participant == "" || boundary.Status != types.DiagramParticipantBoundaryUnproven {
				return fmt.Errorf("diagram_boundary_replacements[%d].participant_boundaries[%d] requires participant and status=unproven", i, j)
			}
			if seenParticipants[participant] {
				return fmt.Errorf("diagram_boundary_replacements[%d] duplicates participant=%q", i, participant)
			}
			seenParticipants[participant] = true
		}
		block.ParticipantBoundaries = append([]types.DiagramParticipantBoundary(nil), replacement.ParticipantBoundaries...)
		working[blockID] = block
	}
	usedBoundaryRefs := make(map[string]bool, len(boundaryEdits))
	for i, edit := range boundaryEdits {
		ref := strings.TrimSpace(edit.BoundaryRef)
		action := strings.TrimSpace(edit.Action)
		if ref == "" || action == "" {
			return fmt.Errorf("diagram_boundary_edits[%d] requires boundary_ref and action", i)
		}
		if usedBoundaryRefs[ref] {
			return fmt.Errorf("diagram_boundary_edits[%d] reuses boundary_ref=%q", i, ref)
		}
		usedBoundaryRefs[ref] = true
		var failure *types.AnswerDiagramParticipantBoundaryRepairFailure
		if lease != nil {
			for j := range lease.ParticipantBoundaryFailures {
				if strings.TrimSpace(lease.ParticipantBoundaryFailures[j].BoundaryRef) == ref {
					failure = &lease.ParticipantBoundaryFailures[j]
					break
				}
			}
		}
		if failure == nil || !failure.AllowsBoundaryAction(action) {
			return fmt.Errorf("diagram_boundary_edits[%d] boundary_ref=%q is stale, unknown, or does not allow action=%q", i, ref, action)
		}
		blockID := strings.TrimSpace(failure.BlockID)
		if boundarySeen[blockID] {
			return fmt.Errorf("diagram_boundary_edits[%d] block_id=%q conflicts with diagram_boundary_replacements", i, blockID)
		}
		block, err := loadBlock(blockID, i, "diagram_boundary_edits", true)
		if err != nil {
			return err
		}
		immutableBase, baseOK := previous[blockID]
		if !baseOK || ambiguous[blockID] {
			return fmt.Errorf("diagram_boundary_edits[%d] boundary_ref=%q has no unique immutable base", i, ref)
		}
		if got := types.AnswerDiagramParticipantBoundaryFingerprint(immutableBase.ParticipantBoundaries); got == "" || got != failure.BaseBoundaryFingerprint {
			return fmt.Errorf("diagram_boundary_edits[%d] boundary_ref=%q does not match the current boundary generation", i, ref)
		}
		participant := strings.TrimSpace(failure.Participant)
		matches := make([]int, 0, 2)
		for j, boundary := range block.ParticipantBoundaries {
			if strings.EqualFold(strings.TrimSpace(boundary.Participant), participant) {
				matches = append(matches, j)
			}
		}
		switch types.AnswerDiagramParticipantBoundaryRepairAction(action) {
		case types.AnswerDiagramParticipantBoundaryRepairAddUnproven:
			if len(matches) != 0 {
				return fmt.Errorf("diagram_boundary_edits[%d] add_unproven requires participant=%q to be absent", i, participant)
			}
			block.ParticipantBoundaries = append(block.ParticipantBoundaries, types.DiagramParticipantBoundary{
				Participant: participant, Status: types.DiagramParticipantBoundaryUnproven,
			})
		case types.AnswerDiagramParticipantBoundaryRepairRemove:
			if len(matches) != 1 {
				return fmt.Errorf("diagram_boundary_edits[%d] remove_boundary requires exactly one participant=%q row", i, participant)
			}
			at := matches[0]
			block.ParticipantBoundaries = append(block.ParticipantBoundaries[:at], block.ParticipantBoundaries[at+1:]...)
		case types.AnswerDiagramParticipantBoundaryRepairDeduplicate:
			if len(matches) < 2 {
				return fmt.Errorf("diagram_boundary_edits[%d] deduplicate_boundary requires duplicate participant=%q rows", i, participant)
			}
			kept := false
			out := make([]types.DiagramParticipantBoundary, 0, len(block.ParticipantBoundaries)-len(matches)+1)
			for _, boundary := range block.ParticipantBoundaries {
				if !strings.EqualFold(strings.TrimSpace(boundary.Participant), participant) {
					out = append(out, boundary)
					continue
				}
				if !kept {
					out = append(out, boundary)
					kept = true
				}
			}
			block.ParticipantBoundaries = out
		default:
			return fmt.Errorf("diagram_boundary_edits[%d] action=%q is invalid", i, action)
		}
		working[blockID] = block
	}
	orphanParticipantEdits := make([]emitAnswerDiagramParticipantEdit, 0, len(participantEdits))
	visibilityParticipantEdits := make([]emitAnswerDiagramParticipantEdit, 0, len(participantEdits))
	for i, edit := range participantEdits {
		switch strings.TrimSpace(edit.Action) {
		case string(types.AnswerDiagramParticipantVisibilityRepairEnsureVisible):
			if strings.TrimSpace(edit.ParticipantRef) == "" || strings.TrimSpace(edit.NodeID) == "" ||
				strings.TrimSpace(edit.VisibleLabel) == "" || strings.TrimSpace(edit.BlockID) != "" || strings.TrimSpace(edit.ParticipantID) != "" {
				return fmt.Errorf("diagram_participant_edits[%d] ensure_visible requires participant_ref, node_id, and visible_label; omit block_id and participant_id", i)
			}
			visibilityParticipantEdits = append(visibilityParticipantEdits, edit)
		case string(types.AnswerDiagramOrphanDispositionRemove), string(types.AnswerDiagramOrphanDispositionRetain):
			if strings.TrimSpace(edit.ParticipantRef) != "" || strings.TrimSpace(edit.NodeID) != "" {
				return fmt.Errorf("diagram_participant_edits[%d] orphan disposition must omit participant_ref and node_id", i)
			}
			orphanParticipantEdits = append(orphanParticipantEdits, edit)
		default:
			return fmt.Errorf("diagram_participant_edits[%d].action=%q is invalid", i, edit.Action)
		}
	}
	participantSeen := make(map[string]bool, len(orphanParticipantEdits))
	participantCandidates := make(map[string]types.AnswerDiagramOrphanCleanupCandidate, len(orphanParticipantEdits))
	for i, edit := range orphanParticipantEdits {
		blockID := strings.TrimSpace(edit.BlockID)
		participantID := strings.TrimSpace(edit.ParticipantID)
		if blockID == "" || participantID == "" {
			return fmt.Errorf("diagram_participant_edits[%d] requires block_id and participant_id", i)
		}
		action := strings.TrimSpace(edit.Action)
		if action != string(types.AnswerDiagramOrphanDispositionRemove) &&
			action != string(types.AnswerDiagramOrphanDispositionRetain) {
			return fmt.Errorf("diagram_participant_edits[%d].action=%q is invalid; choose remove_if_isolated or retain_as_context", i, edit.Action)
		}
		key := blockID + "\x00" + participantID
		if participantSeen[key] {
			return fmt.Errorf("diagram_participant_edits[%d] duplicates block_id=%q participant_id=%q", i, blockID, participantID)
		}
		participantSeen[key] = true
		_, err := loadBlock(blockID, i, "diagram_participant_edits", true)
		if err != nil {
			return err
		}
		candidate, ok := atomicDiagramLeaseOrphanCandidate(lease, blockID, participantID)
		if ok {
			participantCandidates[key] = candidate
		}
	}
	if err := validateAtomicDiagramParticipantDispositionRoster(
		previous, working, orphanParticipantEdits, participantSeen, participantCandidates,
		protectedParticipants, lease,
	); err != nil {
		return err
	}
	for i, edit := range orphanParticipantEdits {
		blockID := strings.TrimSpace(edit.BlockID)
		participantID := strings.TrimSpace(edit.ParticipantID)
		key := blockID + "\x00" + participantID
		block := working[blockID]
		base := previous[blockID]
		// Orphan dispositions are conditional decisions against the
		// producer-published pre-edit roster. A model-selected typed addition
		// may keep this participant connected. In that case the disposition
		// has no work to do; accepting it as a no-op preserves the model's
		// relation and wording without a second topology-guessing round.
		if !atomicDiagramParticipantDispositionIsRequired(
			base, block, participantID, protectedParticipants, lease,
		) {
			continue
		}
		if err := applyOneModelAuthoredDiagramParticipantEdit(
			&block, base, edit, participantCandidates[key], protectedParticipants, lease,
		); err != nil {
			return fmt.Errorf("diagram_participant_edits[%d] block_id=%q participant_id=%q: %w", i, blockID, participantID, err)
		}
		working[blockID] = block
	}
	usedParticipantRefs := make(map[string]bool, len(visibilityParticipantEdits))
	usedVisibilityNodes := make(map[string]bool, len(visibilityParticipantEdits))
	for i, edit := range visibilityParticipantEdits {
		ref := strings.TrimSpace(edit.ParticipantRef)
		if usedParticipantRefs[ref] {
			return fmt.Errorf("diagram_participant_edits[%d] reuses participant_ref=%q", i, ref)
		}
		usedParticipantRefs[ref] = true
		failure, ok := atomicDiagramLeaseParticipantVisibilityFailure(lease, ref)
		if !ok || !failure.AllowsParticipantAction(strings.TrimSpace(edit.Action)) {
			return fmt.Errorf("diagram_participant_edits[%d] participant_ref=%q is stale, unknown, or does not allow action=%q", i, ref, edit.Action)
		}
		blockID := strings.TrimSpace(failure.BlockID)
		nodeKey := blockID + "\x00" + strings.TrimSpace(edit.NodeID)
		if usedVisibilityNodes[nodeKey] {
			return fmt.Errorf("diagram_participant_edits[%d] duplicates block_id=%q node_id=%q", i, blockID, edit.NodeID)
		}
		usedVisibilityNodes[nodeKey] = true
		block, err := loadBlock(blockID, i, "diagram_participant_edits", true)
		if err != nil {
			return err
		}
		base := previous[blockID]
		if err := applyOneModelAuthoredDiagramParticipantVisibilityEdit(&block, base, edit, failure); err != nil {
			return fmt.Errorf("diagram_participant_edits[%d] participant_ref=%q: %w", i, ref, err)
		}
		working[blockID] = block
	}
	for _, blockID := range order {
		patch.ReplaceBlocks = append(patch.ReplaceBlocks, working[blockID])
	}
	// An atomic diagram operation is itself the model's edit declaration for
	// the block. Listing that same block in unchanged_block_ids is therefore a
	// redundant preservation assertion, not a competing whole-block mutation:
	// the compiler starts from the immutable base and preserves every unlisted
	// carrier byte. Absorb the redundant id after all target blocks have been
	// resolved, while leaving unknown/unrelated unchanged ids for the ordinary
	// patch validator to check. Replace/add/remove remain true conflicts above.
	if len(working) > 0 && len(patch.UnchangedBlockIDs) > 0 {
		kept := patch.UnchangedBlockIDs[:0]
		for _, rawID := range patch.UnchangedBlockIDs {
			if _, edited := working[strings.TrimSpace(rawID)]; edited {
				continue
			}
			kept = append(kept, rawID)
		}
		patch.UnchangedBlockIDs = kept
	}
	return nil
}

func atomicDiagramLeaseParticipantVisibilityFailure(
	lease *types.AnswerDiagramRelationRepairLease,
	ref string,
) (types.AnswerDiagramParticipantVisibilityRepairFailure, bool) {
	ref = strings.TrimSpace(ref)
	var found types.AnswerDiagramParticipantVisibilityRepairFailure
	count := 0
	if lease != nil {
		for _, failure := range lease.ParticipantVisibilityFailures {
			if strings.TrimSpace(failure.ParticipantRef) != ref {
				continue
			}
			found = failure
			count++
		}
	}
	return found, ref != "" && count == 1
}

func applyOneModelAuthoredDiagramParticipantVisibilityEdit(
	block *types.AnswerBlock,
	base types.AnswerBlock,
	edit emitAnswerDiagramParticipantEdit,
	failure types.AnswerDiagramParticipantVisibilityRepairFailure,
) error {
	if block == nil || block.Kind != types.BlockDiagram || block.Diagram == nil ||
		base.Kind != types.BlockDiagram || base.Diagram == nil {
		return fmt.Errorf("ensure_visible requires one existing diagram block")
	}
	if got := types.AnswerDiagramParticipantVisibilityFingerprint(base); got == "" || got != failure.BaseDiagramFingerprint {
		return fmt.Errorf("participant_ref does not match the current diagram generation")
	}
	visibleLabel, ok := normalizeAtomicMermaidParticipantVisibleLabel(edit.VisibleLabel)
	if !ok {
		return fmt.Errorf("ensure_visible requires a non-empty model-authored visible_label of at most 512 bytes")
	}
	nodeID := strings.TrimSpace(edit.NodeID)
	body, ok := mermaidcompat.AddRemovableNodeDeclaration(block.Diagram.Body, nodeID, visibleLabel)
	if !ok {
		return fmt.Errorf("node_id is unsafe, already used, or the diagram family has no lossless standalone declaration form")
	}
	labels := diagramEvidenceNodeLabels(body, block.Diagram.Kind)
	if !diagramParticipantEndpointExplicitlyDisplaysIdentity([]string{failure.Participant}, nodeID, labels) {
		return fmt.Errorf("the model-authored node_id/visible_label does not visibly carry the exact required participant identity")
	}
	block.Diagram.Body = body
	return nil
}

func cloneAtomicDiagramPatchBlock(in types.AnswerBlock) types.AnswerBlock {
	out := in
	out.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), in.EdgeAnchors...)
	out.ParticipantBoundaries = append([]types.DiagramParticipantBoundary(nil), in.ParticipantBoundaries...)
	if in.Diagram != nil {
		diagram := *in.Diagram
		out.Diagram = &diagram
	}
	return out
}

func applyOneModelAuthoredDiagramParticipantEdit(
	block *types.AnswerBlock,
	base types.AnswerBlock,
	edit emitAnswerDiagramParticipantEdit,
	candidate types.AnswerDiagramOrphanCleanupCandidate,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
) error {
	if block == nil || block.Kind != types.BlockDiagram || block.Diagram == nil ||
		base.Kind != types.BlockDiagram || base.Diagram == nil || lease == nil {
		return fmt.Errorf("orphan disposition requires one live relation-repair lease and an existing diagram")
	}
	participantID := strings.TrimSpace(edit.ParticipantID)
	baseDecl, count := atomicDiagramUniqueRemovableDeclaration(base.Diagram.Body, participantID)
	if count != 1 {
		return fmt.Errorf("base declaration is not one unique standalone participant/node line (matches=%d)", count)
	}
	protected := make(map[string]bool, len(protectedParticipants)+len(block.ParticipantBoundaries)+len(base.ParticipantBoundaries))
	for _, raw := range protectedParticipants {
		if key := atomicDiagramParticipantSurfaceKey(raw); key != "" {
			protected[key] = true
		}
	}
	for _, boundaries := range [][]types.DiagramParticipantBoundary{base.ParticipantBoundaries, block.ParticipantBoundaries} {
		for _, boundary := range boundaries {
			if key := atomicDiagramParticipantSurfaceKey(boundary.Participant); key != "" {
				protected[key] = true
			}
		}
	}
	if atomicDiagramParticipantProtected(protected, participantID, baseDecl.Label) {
		return fmt.Errorf("participant is protected by the typed requested-participant slate or an unproven boundary")
	}
	incident, allFailed := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(base, participantID, lease)
	if incident == 0 || !allFailed {
		return fmt.Errorf("base participant must have incident edges and every incident edge must be covered by a remove-capable live failure")
	}
	for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
		if strings.TrimSpace(edge.From) == participantID || strings.TrimSpace(edge.To) == participantID {
			return fmt.Errorf("participant remains incident to a visible edge after the selected relation edits")
		}
	}
	for _, anchor := range block.EdgeAnchors {
		if strings.TrimSpace(anchor.FromNode) == participantID || strings.TrimSpace(anchor.ToNode) == participantID {
			return fmt.Errorf("participant remains incident to typed edge metadata after the selected relation edits")
		}
	}
	action := strings.TrimSpace(edit.Action)
	if !candidate.AllowsAction(action) {
		return fmt.Errorf("action=%q is not allowed by the live orphan decision", action)
	}
	var body string
	switch action {
	case string(types.AnswerDiagramOrphanDispositionRemove):
		if strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("remove_if_isolated does not accept visible_label")
		}
		if mermaidcompat.SequenceParticipantReferenced(block.Diagram.Body, participantID) {
			return fmt.Errorf("participant remains referenced by a visible sequence directive after the selected relation edits")
		}
		body, count = mermaidcompat.RemoveRemovableNodeDeclaration(block.Diagram.Body, participantID)
	case string(types.AnswerDiagramOrphanDispositionRetain):
		visibleLabel, ok := normalizeAtomicMermaidParticipantVisibleLabel(edit.VisibleLabel)
		if !ok {
			return fmt.Errorf("retain_as_context requires a non-empty model-authored visible_label of at most 512 bytes")
		}
		body, count = mermaidcompat.RewriteRemovableNodeDeclarationLabel(block.Diagram.Body, participantID, visibleLabel)
	default:
		return fmt.Errorf("unsupported orphan disposition action=%q", action)
	}
	if count != 1 {
		return fmt.Errorf("current declaration is not one unique safely editable participant/node line (matches=%d)", count)
	}
	block.Diagram.Body = body
	return nil
}

func normalizeAtomicMermaidParticipantVisibleLabel(raw string) (string, bool) {
	if strings.ContainsRune(raw, '\x00') {
		return "", false
	}
	label := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	if label == "" || len(label) > 512 {
		return "", false
	}
	// Mermaid declaration labels are single-line carriers. Preserve every
	// model-authored word while encoding line boundaries in Mermaid's portable
	// display form; this is syntax normalization, not label authorship.
	label = strings.ReplaceAll(label, "\n", "<br/>")
	if len(label) > 512 {
		return "", false
	}
	return label, true
}

func atomicDiagramLeaseOrphanCandidate(
	lease *types.AnswerDiagramRelationRepairLease,
	blockID, participantID string,
) (types.AnswerDiagramOrphanCleanupCandidate, bool) {
	blockID = strings.TrimSpace(blockID)
	participantID = strings.TrimSpace(participantID)
	var found types.AnswerDiagramOrphanCleanupCandidate
	count := 0
	if lease != nil {
		for _, candidate := range lease.OptionalOrphanCleanups {
			if strings.TrimSpace(candidate.BlockID) != blockID ||
				strings.TrimSpace(candidate.ParticipantID) != participantID {
				continue
			}
			found = candidate
			count++
		}
	}
	return found, count == 1
}

// atomicDiagramParticipantCleanupIneligibility explains only syntax/typed
// liveness from the exact post-edit block. It does not infer a relationship
// from a message label or choose whether the model should keep another edge.
// The bounded endpoint list gives the next retry enough information to avoid
// repeatedly treating a still-connected participant as an orphan.
func atomicDiagramParticipantCleanupIneligibility(
	current, base types.AnswerBlock,
	participantID string,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
) string {
	participantID = strings.TrimSpace(participantID)
	decl, count := atomicDiagramUniqueRemovableDeclaration(current.Diagram.Body, participantID)
	if count != 1 {
		return fmt.Sprintf("the current graph has %d uniquely removable declaration rows for this participant; the local cleanup contract requires exactly one", count)
	}
	if atomicDiagramParticipantEditProtected(base, current, participantID, decl.Label, protectedParticipants) {
		return "the participant is protected by the requested-participant slate or an unproven boundary"
	}
	const maxIncident = 4
	incident := make([]string, 0, maxIncident)
	totalIncident := 0
	if current.Diagram != nil {
		for _, edge := range mermaidcompat.ParseEdges(current.Diagram.Body) {
			from, to := strings.TrimSpace(edge.From), strings.TrimSpace(edge.To)
			if from != participantID && to != participantID {
				continue
			}
			totalIncident++
			if len(incident) < maxIncident {
				incident = append(incident, from+strings.TrimSpace(edge.Operator)+to)
			}
		}
		if mermaidcompat.SequenceParticipantReferenced(current.Diagram.Body, participantID) {
			return "the participant remains referenced by a visible sequence directive after the selected relation edits"
		}
	}
	for _, anchor := range current.EdgeAnchors {
		if strings.TrimSpace(anchor.FromNode) == participantID || strings.TrimSpace(anchor.ToNode) == participantID {
			totalIncident++
		}
	}
	if totalIncident > 0 {
		suffix := ""
		if totalIncident > len(incident) {
			suffix = fmt.Sprintf(" (+%d additional visible/typed incident carriers)", totalIncident-len(incident))
		}
		if len(incident) == 0 {
			return fmt.Sprintf("the participant remains connected by %d typed incident carrier(s) after the selected relation edits", totalIncident)
		}
		return fmt.Sprintf(
			"the participant remains connected after the selected relation edits by %s%s; it is not isolated",
			strings.Join(incident, ", "), suffix,
		)
	}
	incidentBase, allFailed := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(base, participantID, lease)
	if incidentBase == 0 {
		return "the participant was already disconnected in the immutable base; this local relation-repair lease cannot authorize an unrelated declaration deletion"
	}
	if !allFailed {
		return "at least one immutable-base incident relation is outside the current remove-capable failure set"
	}
	return "the participant is not published by the current lease; the executor cannot widen the model's cleanup capability"
}

// validateAtomicDiagramParticipantDispositionRoster computes the complete
// post-edge-edit decision surface before applying any participant mutation.
// It does not choose a graph outcome. Every row comes from the live typed
// orphan roster plus structural Mermaid endpoints/typed anchors; message text,
// visible labels, request prose, and answer prose are not evidence inputs.
func validateAtomicDiagramParticipantDispositionRoster(
	previous map[string]types.AnswerBlock,
	working map[string]types.AnswerBlock,
	participantEdits []emitAnswerDiagramParticipantEdit,
	participantDecisions map[string]bool,
	participantCandidates map[string]types.AnswerDiagramOrphanCleanupCandidate,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
) error {
	if lease == nil {
		if len(participantEdits) == 0 {
			return nil
		}
	}
	roster := &atomicDiagramParticipantDispositionRosterError{Version: 1}
	for _, edit := range participantEdits {
		blockID := strings.TrimSpace(edit.BlockID)
		participantID := strings.TrimSpace(edit.ParticipantID)
		key := blockID + "\x00" + participantID
		base, baseOK := previous[blockID]
		current, currentOK := working[blockID]
		candidate, candidateOK := participantCandidates[key]
		if !baseOK || !currentOK || current.Diagram == nil || base.Diagram == nil || !candidateOK {
			detail := "the participant is not one unique live optional_orphan_cleanups row"
			if baseOK && currentOK && current.Diagram != nil && base.Diagram != nil {
				detail = atomicDiagramParticipantCleanupIneligibility(current, base, participantID, protectedParticipants, lease)
			}
			roster.Unexpected = append(roster.Unexpected, atomicDiagramParticipantDispositionRosterRow{
				BlockID: blockID, ParticipantID: participantID, Issue: "not_live_candidate", Detail: detail,
			})
			continue
		}
		if !candidate.AllowsAction(strings.TrimSpace(edit.Action)) {
			roster.Unexpected = append(roster.Unexpected, atomicDiagramParticipantDispositionRosterRow{
				BlockID: blockID, ParticipantID: participantID, Issue: "action_not_allowed",
				Detail: fmt.Sprintf("action=%q is not allowed by the live optional_orphan_cleanups row", strings.TrimSpace(edit.Action)),
			})
			continue
		}
		// This is a conditional disposition over a live, signed candidate. If
		// other selected edits leave it connected or protected, the operation
		// is an intentional no-op rather than an invalid topology prediction.
		// Unknown candidates and actions still fail above, and an actually
		// isolated row remains mandatory below.
	}
	if lease == nil {
		return roster
	}
	for _, candidate := range lease.OptionalOrphanCleanups {
		blockID := strings.TrimSpace(candidate.BlockID)
		participantID := strings.TrimSpace(candidate.ParticipantID)
		base, baseOK := previous[blockID]
		current, changed := working[blockID]
		if !baseOK || !changed || current.Diagram == nil || base.Diagram == nil {
			continue
		}
		if !atomicDiagramParticipantDispositionIsRequired(base, current, participantID, protectedParticipants, lease) {
			continue
		}
		if !participantDecisions[blockID+"\x00"+participantID] {
			roster.Missing = append(roster.Missing, atomicDiagramParticipantDispositionRosterRow{
				BlockID: blockID, ParticipantID: participantID, Issue: "isolated_decision_required",
				Detail: "became isolated after the selected edge edits",
			})
		}
	}
	if len(roster.Missing) > 0 || len(roster.Unexpected) > 0 {
		return roster
	}
	return nil
}

func atomicDiagramParticipantDispositionIsRequired(
	base, current types.AnswerBlock,
	participantID string,
	protectedParticipants []string,
	lease *types.AnswerDiagramRelationRepairLease,
) bool {
	if current.Diagram == nil || base.Diagram == nil {
		return false
	}
	decl, count := atomicDiagramUniqueRemovableDeclaration(current.Diagram.Body, participantID)
	if count != 1 || atomicDiagramParticipantEditProtected(base, current, participantID, decl.Label, protectedParticipants) {
		return false
	}
	return !atomicDiagramParticipantHasIncidentCarrier(current, participantID) &&
		atomicDiagramBaseCandidateStillAuthorized(base, participantID, lease)
}

func atomicDiagramParticipantEditProtected(
	base, current types.AnswerBlock,
	participantID, visibleLabel string,
	protectedParticipants []string,
) bool {
	protected := make(map[string]bool, len(protectedParticipants)+len(base.ParticipantBoundaries)+len(current.ParticipantBoundaries))
	for _, raw := range protectedParticipants {
		if key := atomicDiagramParticipantSurfaceKey(raw); key != "" {
			protected[key] = true
		}
	}
	for _, boundaries := range [][]types.DiagramParticipantBoundary{base.ParticipantBoundaries, current.ParticipantBoundaries} {
		for _, boundary := range boundaries {
			if key := atomicDiagramParticipantSurfaceKey(boundary.Participant); key != "" {
				protected[key] = true
			}
		}
	}
	return atomicDiagramParticipantProtected(protected, participantID, visibleLabel)
}

func atomicDiagramParticipantHasIncidentCarrier(block types.AnswerBlock, participantID string) bool {
	if block.Diagram != nil {
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if strings.TrimSpace(edge.From) == participantID || strings.TrimSpace(edge.To) == participantID {
				return true
			}
		}
		if mermaidcompat.SequenceParticipantReferenced(block.Diagram.Body, participantID) {
			return true
		}
	}
	for _, anchor := range block.EdgeAnchors {
		if strings.TrimSpace(anchor.FromNode) == participantID || strings.TrimSpace(anchor.ToNode) == participantID {
			return true
		}
	}
	return false
}

func atomicDiagramBaseCandidateStillAuthorized(
	base types.AnswerBlock,
	participantID string,
	lease *types.AnswerDiagramRelationRepairLease,
) bool {
	incident, allFailed := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(base, participantID, lease)
	return incident > 0 && allFailed
}

func atomicDiagramUniqueRemovableDeclaration(body, participantID string) (mermaidcompat.NodeDecl, int) {
	participantID = strings.TrimSpace(participantID)
	var found mermaidcompat.NodeDecl
	count := 0
	for _, decl := range mermaidcompat.RemovableNodeDeclarations(body) {
		if strings.TrimSpace(decl.Ident) != participantID {
			continue
		}
		found = decl
		count++
	}
	return found, count
}

func atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(
	base types.AnswerBlock,
	participantID string,
	lease *types.AnswerDiagramRelationRepairLease,
) (int, bool) {
	if base.Diagram == nil || lease == nil {
		return 0, false
	}
	edges := mermaidcompat.ParseEdges(base.Diagram.Body)
	pairTotals := make(map[string]int)
	for _, edge := range edges {
		key := strings.TrimSpace(edge.From) + "\x00" + strings.TrimSpace(edge.To)
		pairTotals[key]++
	}
	pairOccurrences := make(map[string]int)
	usedFailureRefs := make(map[string]bool)
	incident := 0
	for _, edge := range edges {
		from, to := strings.TrimSpace(edge.From), strings.TrimSpace(edge.To)
		pairKey := from + "\x00" + to
		pairOccurrences[pairKey]++
		if from != participantID && to != participantID {
			continue
		}
		incident++
		matched := false
		for _, failure := range lease.Failures {
			ref := strings.TrimSpace(failure.FailureRef)
			if usedFailureRefs[ref] || !failure.CanRemoveVisibleBodyOccurrence(
				base.ID, from, to, pairOccurrences[pairKey], pairTotals[pairKey],
			) {
				continue
			}
			usedFailureRefs[ref] = true
			matched = true
			break
		}
		if !matched {
			return incident, false
		}
	}
	return incident, true
}

func atomicDiagramParticipantSurfaceKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(raw, "`\\\"'")))
}

func atomicDiagramParticipantProtected(protected map[string]bool, surfaces ...string) bool {
	for _, surface := range surfaces {
		for _, candidate := range []string{surface, types.DiagramPrimaryVisibleIdentity(surface)} {
			key := atomicDiagramParticipantSurfaceKey(candidate)
			if key != "" && protected[key] {
				return true
			}
		}
	}
	return false
}

// validateAtomicSharedBodyRemove admits the only safe overlap between atomic
// edit selectors: the model selected every live failure ref attached to one
// exact visible Mermaid statement and chose remove for each of them. Distinct
// typed anchors can legitimately share one body occurrence (for example a
// rejected call and a rejected precedence claim rendered on the same arrow).
// The statement can be removed only once, while every selected anchor must be
// removed in the same transaction. No action, relation, endpoint, or visible
// wording is inferred here.
func validateAtomicSharedBodyRemove(members []resolvedAtomicDiagramEdgeEdit) error {
	if len(members) < 2 {
		return fmt.Errorf("shared body removal requires at least two selected failures")
	}
	first := members[0].edit
	if first.Match == nil || first.BodyOccurrence <= 0 {
		return fmt.Errorf("shared body removal requires one exact positive body occurrence")
	}
	blockID := strings.TrimSpace(first.BlockID)
	fromNode := strings.TrimSpace(first.Match.FromNode)
	toNode := strings.TrimSpace(first.Match.ToNode)
	bodyOccurrence := first.BodyOccurrence
	seenTargets := make(map[string]bool, len(members))
	for _, member := range members {
		edit := member.edit
		if !edit.failureRefResolved || strings.TrimSpace(edit.FailureRef) == "" {
			return fmt.Errorf("overlapping selectors require distinct live failure_ref values")
		}
		if !strings.EqualFold(strings.TrimSpace(edit.Action), "remove") || edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("overlapping selectors permit only model-selected action=remove without edge or visible_label")
		}
		if edit.Match == nil || strings.TrimSpace(edit.BlockID) != blockID ||
			strings.TrimSpace(edit.Match.FromNode) != fromNode || strings.TrimSpace(edit.Match.ToNode) != toNode ||
			edit.BodyOccurrence != bodyOccurrence {
			return fmt.Errorf("overlapping selectors do not identify one exact visible body carrier")
		}
		if edit.failureRefCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor &&
			edit.failureRefCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
			return fmt.Errorf("target_carrier=%s cannot share a visible body removal", edit.failureRefCarrier)
		}
		targetKey := strings.Join([]string{
			string(edit.failureRefCarrier), string(edit.Match.RelationKind),
			strings.TrimSpace(edit.Match.FromIdentity), strings.TrimSpace(edit.Match.ToIdentity),
		}, "\x00")
		if seenTargets[targetKey] {
			return fmt.Errorf("overlapping failure refs select the same typed target")
		}
		seenTargets[targetKey] = true
	}
	return nil
}

func applyAtomicSharedBodyRemove(block *types.AnswerBlock, edits []emitAnswerDiagramEdgeEdit) error {
	if block == nil || block.Diagram == nil || len(edits) < 2 || edits[0].Match == nil {
		return fmt.Errorf("shared body removal requires an existing diagram carrier")
	}
	first := edits[0]
	lineIndex, err := findAtomicMermaidEdgeLine(
		block.Diagram.Body, first.Match.FromNode, first.Match.ToNode, first.BodyOccurrence,
	)
	if err != nil {
		return err
	}
	anchorIndexes := make(map[int]bool, len(edits))
	for _, edit := range edits {
		anchorIndex, _, anchorErr := findAtomicDiagramAnchor(block.EdgeAnchors, *edit.Match, 1)
		if anchorErr != nil {
			if edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
				continue
			}
			return anchorErr
		}
		if anchorIndexes[anchorIndex] {
			return fmt.Errorf("overlapping failure refs resolve to the same exact prior anchor")
		}
		anchorIndexes[anchorIndex] = true
	}
	indexes := make([]int, 0, len(anchorIndexes))
	for index := range anchorIndexes {
		indexes = append(indexes, index)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(indexes)))
	for _, index := range indexes {
		block.EdgeAnchors = append(block.EdgeAnchors[:index], block.EdgeAnchors[index+1:]...)
	}
	lines := strings.Split(block.Diagram.Body, "\n")
	lines = append(lines[:lineIndex], lines[lineIndex+1:]...)
	block.Diagram.Body = strings.Join(lines, "\n")
	return nil
}

// resolveAtomicDiagramFailureRef converts one model-selected, lease-owned
// failure reference into the exact structural locator already present in the
// rejected draft. It does not choose an action, replacement edge, visible
// label, or relation. References are useful only inside the live lease; an
// unknown, cross-block, ambiguous, or stale reference fails closed. Once the
// live ref and action pass those checks, obsolete selector coordinates carry
// no authority: the ref already owns the exact carrier, so match/occurrence
// fields are quarantined instead of creating a second contradictory selector.
func resolveAtomicDiagramFailureRef(
	edit emitAnswerDiagramEdgeEdit,
	lease *types.AnswerDiagramRelationRepairLease,
) (emitAnswerDiagramEdgeEdit, error) {
	ref := strings.TrimSpace(edit.FailureRef)
	additionRef := strings.TrimSpace(edit.AdditionRef)
	if ref != "" && additionRef != "" {
		action := strings.ToLower(strings.TrimSpace(edit.Action))
		// A model that selected both live opaque refs has already expressed the
		// exact bind-existing-carrier intent. Accept the schema's older "add"
		// spelling as the transport alias for "attach"; the recursive failure
		// resolver and addition resolver below still require a live failure that
		// allows attach and an exact compatible candidate. No ref, action target,
		// relation, endpoint, or wording is selected by this normalization.
		if action == "add" {
			edit.Action = string(types.AnswerDiagramRelationRepairActionAttach)
			action = string(types.AnswerDiagramRelationRepairActionAttach)
		}
		if action != string(types.AnswerDiagramRelationRepairActionAttach) {
			return edit, fmt.Errorf("failure_ref and addition_ref may be paired only with action=attach")
		}
		failureOnly := edit
		failureOnly.AdditionRef = ""
		failureOnly.attachPairResolving = true
		resolved, err := resolveAtomicDiagramFailureRef(failureOnly, lease)
		if err != nil {
			return edit, err
		}
		resolved.AdditionRef = additionRef
		resolved.attachPairResolving = false
		return resolveAtomicDiagramAdditionRef(resolved, lease)
	}
	if additionRef != "" {
		return resolveAtomicDiagramAdditionRef(edit, lease)
	}
	if strings.ToLower(strings.TrimSpace(edit.Action)) == string(types.AnswerDiagramRelationRepairActionAttach) &&
		!edit.attachPairResolving {
		return edit, fmt.Errorf("action=attach requires both failure_ref and addition_ref")
	}
	if ref == "" {
		return edit, nil
	}
	action := strings.ToLower(strings.TrimSpace(edit.Action))
	if action == "add" {
		return edit, fmt.Errorf("action=add does not accept failure_ref; select an allowed addition with a complete edge")
	}
	if lease == nil || lease.Version != 1 {
		return edit, fmt.Errorf("failure_ref=%q is not present in a live relation-repair lease", ref)
	}
	var failure *types.AnswerDiagramRelationRepairFailure
	for i := range lease.Failures {
		if strings.TrimSpace(lease.Failures[i].FailureRef) != ref {
			continue
		}
		if failure != nil {
			return edit, fmt.Errorf("failure_ref=%q is ambiguous in the live relation-repair lease", ref)
		}
		row := lease.Failures[i]
		failure = &row
	}
	if failure == nil {
		return edit, fmt.Errorf("failure_ref=%q is unknown or stale for the live relation-repair lease", ref)
	}
	edit.failureRefCarrier = failure.TargetCarrier
	// These fields are legacy selector mirrors. The opaque ref is the only
	// current-generation carrier identity, so keeping them would create two
	// authorities for one operation (and visible-body failures may not even
	// have a schema-expressible relation_kind). Quarantine them before the
	// lease-owned locator is installed. This changes no model-authored action,
	// replacement edge, or visible label.
	edit.Match = nil
	edit.Occurrence = 0
	edit.BodyOccurrence = 0
	// Only carriers that own a visible Mermaid statement inherit the
	// producer's body occurrence.  stale_anchor and prior_anchor_metadata
	// select metadata that has no visible body edge by definition; copying a
	// producer-side diagnostic occurrence into those carriers makes the
	// advertised remove/replace operation impossible because their executor
	// correctly requires body_occurrence=0.
	switch failure.TargetCarrier {
	case types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge,
		types.AnswerDiagramRelationRepairCarrierPriorAnchor,
		types.AnswerDiagramRelationRepairCarrierLabelPair:
		edit.BodyOccurrence = failure.BodyOccurrence
	}
	// A visible-body failure's occurrence is minted against the same immutable
	// rejected draft as its anchor snapshot.  Reuse that position for the
	// matching prior anchor as well.  This matters when two failed visible rows
	// carry identical technical tuples: selecting occurrence 2 must remove
	// anchor 2, not whichever identical anchor happens to be found first.
	if failure.TargetCarrier == types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge &&
		failure.BodyOccurrence > 0 {
		edit.Occurrence = failure.BodyOccurrence
	}
	if !failure.AllowsAction(action) {
		return edit, fmt.Errorf(
			"failure_ref=%q targets carrier=%s and does not allow action=%s; allowed_actions=%v",
			ref, failure.TargetCarrier, action, failure.AllowedActions,
		)
	}
	blockID := strings.TrimSpace(failure.BlockID)
	if declared := strings.TrimSpace(edit.BlockID); declared != "" && declared != blockID {
		return edit, fmt.Errorf("failure_ref=%q belongs to block_id=%q, not %q", ref, blockID, declared)
	}
	edit.BlockID = blockID

	var baseAnchors []types.DiagramEdgeAnchor
	for _, block := range lease.Blocks {
		if strings.TrimSpace(block.BlockID) == blockID {
			baseAnchors = append(baseAnchors, block.BaseAnchors...)
		}
	}
	// failure.FromIdentity/ToIdentity are the validator's resolved endpoint
	// identities. They may be more precise than (and intentionally disagree
	// with) the rejected draft's model-authored anchor identities. The opaque
	// ref is bound to the exact base snapshot, so resolve it first by its stable
	// visible carrier and typed relation. Use identity only to disambiguate two
	// otherwise identical carrier rows; never require a rejected identity to
	// equal the corrected validator projection.
	if failure.TargetCarrier == types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
		match := types.DiagramEdgeAnchor{
			FromNode: strings.TrimSpace(failure.FromNode), ToNode: strings.TrimSpace(failure.ToNode),
			FromIdentity: strings.TrimSpace(failure.FromIdentity), ToIdentity: strings.TrimSpace(failure.ToIdentity),
			RelationKind: types.AnswerDiagramRelationRepairFailureEffectiveRelation(*failure),
		}
		if strings.TrimSpace(match.FromNode) == "" || strings.TrimSpace(match.ToNode) == "" ||
			strings.ContainsAny(match.FromNode+match.ToNode, "\r\n") {
			return edit, fmt.Errorf("failure_ref=%q cannot identify its visible body edge: from_node and to_node must be complete single-line values", ref)
		}
		if action == string(types.AnswerDiagramRelationRepairActionReplace) && edit.Edge != nil {
			relation := types.AnswerDiagramRelationRepairFailureEffectiveRelation(*failure)
			if !relation.IsValid() || strings.TrimSpace(failure.FromIdentity) == "" ||
				strings.TrimSpace(failure.ToIdentity) == "" {
				return edit, fmt.Errorf("failure_ref=%q does not own a complete typed replacement tuple", ref)
			}
			edit.Edge.RelationKind = relation
			edit.Edge.FromIdentity = strings.TrimSpace(failure.FromIdentity)
			edit.Edge.ToIdentity = strings.TrimSpace(failure.ToIdentity)
		}
		edit.Match = &match
		edit.failureRefResolved = true
		edit.failureIssue = strings.TrimSpace(failure.Issue)
		return edit, nil
	}
	if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor &&
		failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata &&
		failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierStaleAnchor &&
		failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierLabelPair {
		return edit, fmt.Errorf(
			"failure_ref=%q has no unambiguous atomic target in block_id=%q; target_carrier=%s allowed_actions=%v",
			ref, blockID, failure.TargetCarrier, failure.AllowedActions,
		)
	}
	matches := types.AnswerDiagramRelationRepairFailureAnchorCandidates(*failure, baseAnchors)
	switch len(matches) {
	case 1:
		match := matches[0]
		if action == string(types.AnswerDiagramRelationRepairActionReplace) && edit.Edge != nil {
			relation := types.AnswerDiagramRelationRepairFailureEffectiveRelation(*failure)
			fromIdentity := strings.TrimSpace(failure.FromIdentity)
			toIdentity := strings.TrimSpace(failure.ToIdentity)
			if fromIdentity == "" || toIdentity == "" {
				fromIdentity = strings.TrimSpace(match.FromIdentity)
				toIdentity = strings.TrimSpace(match.ToIdentity)
			}
			if !relation.IsValid() || fromIdentity == "" || toIdentity == "" {
				return edit, fmt.Errorf("failure_ref=%q does not own a complete typed replacement tuple", ref)
			}
			edit.Edge.RelationKind = relation
			edit.Edge.FromIdentity = fromIdentity
			edit.Edge.ToIdentity = toIdentity
		}
		edit.Match = &match
		edit.failureRefResolved = true
		edit.failureIssue = strings.TrimSpace(failure.Issue)
		return edit, nil
	case 0:
		return edit, fmt.Errorf("failure_ref=%q no longer selects its %s in block_id=%q", ref, failure.TargetCarrier, blockID)
	default:
		return edit, fmt.Errorf("failure_ref=%q matches %d candidate %s rows; the live failure is structurally ambiguous", ref, len(matches), failure.TargetCarrier)
	}
}

// resolveAtomicDiagramAdditionRef turns one model-selected live candidate into
// a complete hidden anchor tuple. The opaque ref is the choice: the compiler
// does not infer a relation from node names, labels, request prose, or Mermaid
// messages. The model still authors both visible endpoints and the visible
// label. Repeated hidden technical fields are quarantined because the selected
// candidate already owns them. For standalone metadata attach, the visible
// endpoints and wording are preserved from the exact model-authored base row;
// for diagram add/attach they remain authored in the current model call.
func resolveAtomicDiagramAdditionRef(
	edit emitAnswerDiagramEdgeEdit,
	lease *types.AnswerDiagramRelationRepairLease,
) (emitAnswerDiagramEdgeEdit, error) {
	ref := strings.TrimSpace(edit.AdditionRef)
	action := strings.ToLower(strings.TrimSpace(edit.Action))
	if action != "add" && action != string(types.AnswerDiagramRelationRepairActionAttach) {
		return edit, fmt.Errorf("addition_ref=%q is valid only with action=add or action=attach", ref)
	}
	if lease == nil || lease.Version != 1 {
		return edit, fmt.Errorf("addition_ref=%q is not present in a live relation-repair lease", ref)
	}
	var selected *types.AnswerDiagramRelationRepairCandidate
	for i := range lease.AllowedAdditions {
		if strings.TrimSpace(lease.AllowedAdditions[i].AdditionRef) != ref {
			continue
		}
		if selected != nil {
			return edit, fmt.Errorf("addition_ref=%q is ambiguous in the live relation-repair lease", ref)
		}
		candidate := lease.AllowedAdditions[i]
		selected = &candidate
	}
	if selected == nil {
		return edit, fmt.Errorf("addition_ref=%q is unknown or stale for the live relation-repair lease", ref)
	}
	blockID := strings.TrimSpace(selected.BlockID)
	if declared := strings.TrimSpace(edit.BlockID); declared != "" && declared != blockID {
		return edit, fmt.Errorf("addition_ref=%q belongs to block_id=%q, not %q", ref, blockID, declared)
	}
	if action == string(types.AnswerDiagramRelationRepairActionAttach) {
		if !edit.failureRefResolved || edit.Match == nil {
			return edit, fmt.Errorf("addition_ref=%q action=attach requires one exact live failure", ref)
		}
		failure := types.AnswerDiagramRelationRepairFailure{
			BlockID: strings.TrimSpace(edit.BlockID), TargetCarrier: edit.failureRefCarrier,
			Issue:    strings.TrimSpace(edit.failureIssue),
			FromNode: strings.TrimSpace(edit.Match.FromNode), ToNode: strings.TrimSpace(edit.Match.ToNode),
			FromIdentity: strings.TrimSpace(edit.Match.FromIdentity), ToIdentity: strings.TrimSpace(edit.Match.ToIdentity),
			RelationKind: edit.Match.RelationKind, BodyOccurrence: edit.BodyOccurrence,
		}
		if !types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, *selected) {
			return edit, fmt.Errorf("addition_ref=%q is not compatible with the selected failure carrier", ref)
		}
		if edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
			if edit.Edge != nil {
				return edit, fmt.Errorf("addition_ref=%q metadata attach preserves the existing local nodes and visible label; omit edge", ref)
			}
			preserved := *edit.Match
			edit.Edge = &preserved
			edit.metadataAttach = true
		}
	}
	if edit.Edge == nil {
		return edit, fmt.Errorf("addition_ref=%q requires a model-authored edge with from_node, to_node, and visible_label", ref)
	}
	edit.BlockID = blockID
	edit.Edge.RelationKind = selected.RelationKind
	edit.Edge.FromIdentity = strings.TrimSpace(selected.FromIdentity)
	edit.Edge.ToIdentity = strings.TrimSpace(selected.ToIdentity)
	selectedCopy := *selected
	selectedCopy.FromNodeIDs = append([]string(nil), selected.FromNodeIDs...)
	selectedCopy.ToNodeIDs = append([]string(nil), selected.ToNodeIDs...)
	edit.additionCandidate = &selectedCopy
	return edit, nil
}

func atomicDiagramFailureMatchesAnchorExactly(
	failure types.AnswerDiagramRelationRepairFailure,
	anchor types.DiagramEdgeAnchor,
) bool {
	if failure.RelationKind.IsValid() && failure.RelationKind != anchor.RelationKind {
		return false
	}
	if strings.TrimSpace(failure.FromNode) != "" || strings.TrimSpace(failure.ToNode) != "" {
		if strings.TrimSpace(failure.FromNode) != strings.TrimSpace(anchor.FromNode) ||
			strings.TrimSpace(failure.ToNode) != strings.TrimSpace(anchor.ToNode) {
			return false
		}
	}
	if strings.TrimSpace(failure.FromIdentity) != "" || strings.TrimSpace(failure.ToIdentity) != "" {
		if strings.TrimSpace(failure.FromIdentity) != strings.TrimSpace(anchor.FromIdentity) ||
			strings.TrimSpace(failure.ToIdentity) != strings.TrimSpace(anchor.ToIdentity) {
			return false
		}
	}
	return true
}

func applyOneModelAuthoredDiagramEdgeEdit(
	block *types.AnswerBlock,
	edit emitAnswerDiagramEdgeEdit,
	lease *types.AnswerDiagramRelationRepairLease,
	stagePrecedence []stageauthority.PrecedenceRelation,
) error {
	if block == nil {
		return fmt.Errorf("target carrier is unavailable")
	}
	action := strings.ToLower(strings.TrimSpace(edit.Action))
	if action != "relabel" && action != "remove" && action != "replace" && action != "add" &&
		action != string(types.AnswerDiagramRelationRepairActionAttach) {
		return fmt.Errorf("unsupported action %q", edit.Action)
	}
	if action != "add" && action != "replace" &&
		(strings.TrimSpace(edit.FromNodeVisibleLabel) != "" || strings.TrimSpace(edit.ToNodeVisibleLabel) != "") {
		return fmt.Errorf("from_node_visible_label/to_node_visible_label are valid only for action=add or action=replace")
	}
	occurrence := edit.Occurrence
	if occurrence == 0 {
		occurrence = 1
	}
	if occurrence < 1 {
		return fmt.Errorf("occurrence must be at least 1")
	}
	if edit.BodyOccurrence < 0 {
		return fmt.Errorf("body_occurrence must be at least 1")
	}
	// Relation validators may find missing or stale edge_anchors on a
	// non-diagram structured relation block. Such a block has no Mermaid body to
	// rewrite. A live addition_ref may append only hidden anchor metadata after
	// the lease proves that the model already selected the same evidence in its
	// claim and visible item; existing-row refs retain remove/attach. Every
	// reader-visible block field remains byte-identical.
	if block.Diagram == nil {
		if action == "add" {
			if edit.failureRefResolved || strings.TrimSpace(edit.FailureRef) != "" || edit.Match != nil ||
				edit.BodyOccurrence != 0 || edit.Occurrence > 1 || edit.additionCandidate == nil ||
				strings.TrimSpace(edit.AdditionRef) == "" {
				return fmt.Errorf("non-diagram metadata add requires one exact addition_ref")
			}
			if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
				return err
			}
			if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
				return fmt.Errorf("non-diagram metadata add requires edge.visible_label authored by the model")
			}
			for _, anchor := range block.EdgeAnchors {
				if atomicDiagramAnchorSameTuple(anchor, *edit.Edge) {
					return fmt.Errorf("non-diagram metadata add duplicates an existing exact anchor")
				}
			}
			block.EdgeAnchors = append(block.EdgeAnchors, *edit.Edge)
			return nil
		}
		if !edit.failureRefResolved ||
			edit.failureRefCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata ||
			edit.BodyOccurrence != 0 || edit.Match == nil || strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("non-diagram carrier requires one live prior_anchor_metadata operation")
		}
		anchorIndex, _, err := findAtomicDiagramAnchor(block.EdgeAnchors, *edit.Match, 1)
		if err != nil {
			return err
		}
		switch action {
		case "remove":
			if edit.Edge != nil || edit.metadataAttach {
				return fmt.Errorf("non-diagram metadata remove does not accept an edge")
			}
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
			return nil
		case string(types.AnswerDiagramRelationRepairActionAttach):
			if !edit.metadataAttach || edit.Edge == nil || strings.TrimSpace(edit.FailureRef) == "" ||
				strings.TrimSpace(edit.AdditionRef) == "" || !edit.Edge.HasEndpointIdentityPair() ||
				!edit.Edge.RelationKind.IsValid() {
				return fmt.Errorf("non-diagram metadata attach requires one exact failure_ref+addition_ref pair")
			}
			prior := block.EdgeAnchors[anchorIndex]
			if strings.TrimSpace(edit.Edge.FromNode) != strings.TrimSpace(prior.FromNode) ||
				strings.TrimSpace(edit.Edge.ToNode) != strings.TrimSpace(prior.ToNode) ||
				strings.TrimSpace(edit.Edge.VisibleLabel) != strings.TrimSpace(prior.VisibleLabel) {
				return fmt.Errorf("non-diagram metadata attach must preserve existing local nodes and visible label")
			}
			block.EdgeAnchors[anchorIndex] = *edit.Edge
			return nil
		default:
			return fmt.Errorf("non-diagram prior_anchor_metadata permits only action=remove or paired action=attach")
		}
	}
	if action == "add" {
		if strings.TrimSpace(edit.FailureRef) != "" {
			return fmt.Errorf("action=add must omit failure_ref")
		}
		if edit.Match != nil {
			return fmt.Errorf("action=add must omit match")
		}
		// Validate the model-authored endpoint choice before the sequence
		// deduplicator rewrites an exact technical node to the one uniquely
		// declared alias for that typed endpoint. Validating after that trusted
		// rewrite made the executor reject its own alias whenever the producer
		// lease listed only the technical node. The canonicalizer has its own
		// exact/unique typed binding checks, so this order preserves both gates:
		// unlisted model choices still fail, while a system-selected existing
		// declaration cannot invalidate an already-authorized choice.
		if err := validateAtomicDiagramAdditionEndpointBindings(block, edit.Edge, edit.additionCandidate, stagePrecedence); err != nil {
			return err
		}
		if err := canonicalizeAtomicSequenceAdditionNodeRefs(block, edit.Edge, stagePrecedence); err != nil {
			return err
		}
		if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
			return err
		}
		if occurrence != 1 || edit.BodyOccurrence != 0 {
			return fmt.Errorf("action=add does not accept occurrence or body_occurrence")
		}
		if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
			return fmt.Errorf("action=add requires edge.visible_label authored by the model")
		}
		for _, anchor := range block.EdgeAnchors {
			if atomicDiagramAnchorSameTuple(anchor, *edit.Edge) {
				return fmt.Errorf("action=add duplicates an existing exact anchor")
			}
		}
		if err := ensureAtomicDiagramEndpointDeclarations(block, edit); err != nil {
			return err
		}
		line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
		if err != nil {
			return err
		}
		body := strings.TrimRight(block.Diagram.Body, "\n")
		block.Diagram.Body = body + "\n" + line + "\n"
		block.EdgeAnchors = append(block.EdgeAnchors, *edit.Edge)
		return nil
	}
	if action == "replace" && edit.failureIssue == diagramRequestedStageSpineIncomplete {
		if err := canonicalizeAtomicSequenceAdditionNodeRefs(block, edit.Edge, stagePrecedence); err != nil {
			return err
		}
		if edit.Edge == nil || block.Diagram == nil ||
			!diagramStagePrecedenceAnchorBindsVisibleNodes(
				stagePrecedence, *edit.Edge, diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind),
			) {
			return fmt.Errorf("requested stage-spine replacement must keep one checkout-verified stage pair on its matching visible endpoints")
		}
	}
	if action == string(types.AnswerDiagramRelationRepairActionAttach) &&
		(strings.TrimSpace(edit.FailureRef) == "" || strings.TrimSpace(edit.AdditionRef) == "") {
		return fmt.Errorf("action=attach requires failure_ref and addition_ref")
	}
	if edit.Match == nil {
		return fmt.Errorf("action=%s requires match", action)
	}
	if err := validateAtomicDiagramAnchor(edit.Match, "match"); err != nil && !edit.failureRefResolved {
		return err
	}
	anchorIndex, anchorPairOccurrence, anchorErr := findAtomicDiagramAnchor(block.EdgeAnchors, *edit.Match, occurrence)
	bodyOnly := false
	if anchorErr != nil {
		if action == "relabel" {
			return fmt.Errorf("%w%s", anchorErr, atomicDiagramPriorAnchorRoster(lease, block.ID))
		}
		if !atomicDiagramBodyOnlyFailureAuthorized(lease, block.ID, *edit.Match) {
			return fmt.Errorf("%w%s", anchorErr, atomicDiagramPriorAnchorRoster(lease, block.ID))
		}
		// The producer-owned retry delta names this exact failed visible
		// relation, while the failure itself is that no matching prior
		// anchor exists. Another relation may legitimately share the same
		// visible endpoint pair (for example a grounded return beside an
		// unanchored call), so pair-level anchor presence must not veto this
		// body-only lane. body_occurrence remains mandatory whenever the
		// visible pair is ambiguous, and unrelated anchors stay untouched.
		// The model still chooses remove/replace and every replacement
		// field; the compiler merely makes that declared operation
		// executable against the previous model-authored Mermaid AST.
		bodyOnly = true
	}
	if action == string(types.AnswerDiagramRelationRepairActionAttach) && !bodyOnly {
		return fmt.Errorf("action=attach requires an existing visible edge without a typed anchor")
	}
	// A metadata-only prior-anchor ref deliberately has no unique Mermaid body
	// occurrence. Removing it therefore deletes exactly the selected structured
	// anchor and leaves every visible edge byte-identical. Any failed visible
	// edge has its own producer-owned visible_body_edge ref with a non-zero base
	// occurrence. This prevents the contradictory contract where one ref both
	// requires body_occurrence to disambiguate repeated messages and rejects it
	// because the same ref published occurrence zero.
	if anchorErr == nil && edit.failureRefResolved &&
		edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
		if action != "remove" {
			return fmt.Errorf("prior_anchor_metadata permits only action=remove")
		}
		if edit.BodyOccurrence != 0 {
			return fmt.Errorf("body_occurrence must be omitted for prior_anchor_metadata")
		}
		if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("action=remove must omit edge and visible_label")
		}
		block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		return nil
	}
	// A live stale_anchor ref has already selected one exact base anchor whose
	// visible Mermaid edge is absent. Do not re-authorize that selection by
	// comparing the validator-resolved identities with the rejected model
	// anchor a second time: those identities may legitimately be more precise
	// than the model-authored metadata even though the unique node/relation
	// carrier is the same. Requiring that second equality made the advertised
	// remove/replace capability impossible to execute and sent the finalizer
	// back through a body-edge lookup that must fail by definition.
	if anchorErr == nil && edit.failureRefResolved &&
		edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierStaleAnchor {
		if edit.BodyOccurrence != 0 {
			return fmt.Errorf("body_occurrence must be omitted for stale_anchor")
		}
		switch action {
		case "remove":
			if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
				return fmt.Errorf("action=remove must omit edge and visible_label")
			}
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		case "replace":
			if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
				return err
			}
			if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
				return fmt.Errorf("action=replace requires edge.visible_label authored by the model")
			}
			if err := ensureAtomicDiagramEndpointDeclarations(block, edit); err != nil {
				return err
			}
			line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
			if err != nil {
				return err
			}
			body := strings.TrimRight(block.Diagram.Body, "\n")
			block.Diagram.Body = body + "\n" + line + "\n"
			block.EdgeAnchors[anchorIndex] = *edit.Edge
		default:
			return fmt.Errorf("stale_anchor permits only action=remove or action=replace")
		}
		return nil
	}
	anchorWithoutBody := anchorErr == nil &&
		(action == "remove" || action == "replace") &&
		atomicDiagramAnchorWithoutBodyFailureAuthorized(lease, block.ID, block.EdgeAnchors[anchorIndex])
	if anchorWithoutBody {
		if edit.BodyOccurrence != 0 {
			return fmt.Errorf("body_occurrence must be omitted for an anchor with no visible Mermaid edge")
		}
		switch action {
		case "remove":
			if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
				return fmt.Errorf("action=remove must omit edge and visible_label")
			}
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		case "replace":
			if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
				return err
			}
			if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
				return fmt.Errorf("action=replace requires edge.visible_label authored by the model")
			}
			if err := ensureAtomicDiagramEndpointDeclarations(block, edit); err != nil {
				return err
			}
			line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
			if err != nil {
				return err
			}
			body := strings.TrimRight(block.Diagram.Body, "\n")
			block.Diagram.Body = body + "\n" + line + "\n"
			block.EdgeAnchors[anchorIndex] = *edit.Edge
		}
		return nil
	}
	if action == "replace" {
		if err := ensureAtomicDiagramEndpointDeclarations(block, edit); err != nil {
			return err
		}
	}
	bodyOccurrence, err := atomicDiagramBodyOccurrence(
		block.Diagram.Body, block.EdgeAnchors, edit.Match.FromNode, edit.Match.ToNode,
		anchorPairOccurrence, edit.BodyOccurrence, bodyOnly,
	)
	if err != nil {
		return err
	}
	lineIndex, err := findAtomicMermaidEdgeLine(block.Diagram.Body, edit.Match.FromNode, edit.Match.ToNode, bodyOccurrence)
	if err != nil {
		return err
	}
	lines := strings.Split(block.Diagram.Body, "\n")
	preservedNodeDeclarations := atomicDiagramInlineNodeDeclarationsToLift(
		block.Diagram.Body, lines, lineIndex,
	)
	switch action {
	case "remove":
		if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("action=remove must omit edge and visible_label")
		}
		lines = replaceAtomicMermaidStatementLine(lines, lineIndex, preservedNodeDeclarations)
		if !bodyOnly {
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		}
	case "relabel":
		if edit.Edge != nil {
			return fmt.Errorf("action=relabel must omit edge")
		}
		label := strings.TrimSpace(edit.VisibleLabel)
		if label == "" || strings.ContainsAny(label, "\r\n") {
			return fmt.Errorf("action=relabel requires one non-empty single-line visible_label")
		}
		updated, err := relabelAtomicMermaidEdgeLine(block.Diagram.Body, lines[lineIndex], label)
		if err != nil {
			return err
		}
		lines[lineIndex] = updated
		block.EdgeAnchors[anchorIndex].VisibleLabel = label
	case "replace", string(types.AnswerDiagramRelationRepairActionAttach):
		if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
			return err
		}
		if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
			return fmt.Errorf("action=replace requires edge.visible_label authored by the model")
		}
		line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
		if err != nil {
			return err
		}
		indent := lines[lineIndex][:len(lines[lineIndex])-len(strings.TrimLeft(lines[lineIndex], " \t"))]
		replacement := append([]string(nil), preservedNodeDeclarations...)
		replacement = append(replacement, indent+strings.TrimLeft(line, " \t"))
		lines = replaceAtomicMermaidStatementLine(lines, lineIndex, replacement)
		if bodyOnly {
			block.EdgeAnchors = append(block.EdgeAnchors, *edit.Edge)
		} else {
			block.EdgeAnchors[anchorIndex] = *edit.Edge
		}
	}
	block.Diagram.Body = strings.Join(lines, "\n")
	return nil
}

// ensureAtomicDiagramEndpointDeclarations prevents an atomic edge edit from
// making Mermaid invent an unlabeled implicit node. The model owns both the
// endpoint id and the optional reader-facing node labels. Existing explicit
// declarations remain byte-identical. For a newly introduced endpoint, an
// omitted display label falls back only to the exact node id the model already
// authored in this edit; an explicit model label still wins. The adapter never
// derives identity or wording from the relation lease, technical identity,
// source text, or prompt prose.
func ensureAtomicDiagramEndpointDeclarations(block *types.AnswerBlock, edit emitAnswerDiagramEdgeEdit) error {
	if block == nil || block.Diagram == nil || edit.Edge == nil {
		return fmt.Errorf("edge endpoint declarations require an existing diagram and edge")
	}
	type endpoint struct {
		field string
		node  string
		label string
	}
	endpoints := []endpoint{
		{field: "from_node", node: strings.TrimSpace(edit.Edge.FromNode), label: strings.TrimSpace(edit.FromNodeVisibleLabel)},
		{field: "to_node", node: strings.TrimSpace(edit.Edge.ToNode), label: strings.TrimSpace(edit.ToNodeVisibleLabel)},
	}
	labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
	pending := make(map[string]string, 2)
	order := make([]string, 0, 2)
	for _, endpoint := range endpoints {
		if endpoint.node == "" {
			return fmt.Errorf("edge.%s must be non-empty", endpoint.field)
		}
		if declaredLabel, declared := labels[strings.ToLower(endpoint.node)]; declared {
			if endpoint.label != "" && endpoint.label != strings.TrimSpace(declaredLabel) {
				return fmt.Errorf("%s_visible_label must be omitted or exactly match the current explicit label %q because edge.%s=%q already has an explicit declaration", endpoint.field, strings.TrimSpace(declaredLabel), endpoint.field, endpoint.node)
			}
			continue
		}
		if endpoint.label == "" {
			endpoint.label = endpoint.node
		}
		if prior, exists := pending[endpoint.node]; exists {
			if prior != endpoint.label {
				return fmt.Errorf("self-edge endpoint %q has conflicting model-authored visible labels", endpoint.node)
			}
			continue
		}
		pending[endpoint.node] = endpoint.label
		order = append(order, endpoint.node)
	}
	for _, node := range order {
		body, ok := mermaidcompat.AddExplicitNodeDeclaration(block.Diagram.Body, node, pending[node])
		if !ok {
			return fmt.Errorf("cannot add an explicit model-authored declaration for endpoint %q in this Mermaid family; use an existing declared node id or a supported flow/sequence/class carrier", node)
		}
		block.Diagram.Body = body
	}
	return nil
}

// validateAtomicDiagramAdditionEndpointBindings closes the generation gap
// between an addition_ref's hidden typed tuple and the model-authored visible
// endpoints. Exact technical endpoint spellings remain valid. A broader
// business/component carrier is valid only when the typed producer listed its
// node ID on that exact side. Verified read-stage aliases retain their existing
// pair-scoped authority. Labels, messages, request prose, and answer prose are
// never inspected here.
func validateAtomicDiagramAdditionEndpointBindings(
	block *types.AnswerBlock,
	edge *types.DiagramEdgeAnchor,
	candidate *types.AnswerDiagramRelationRepairCandidate,
	stagePrecedence []stageauthority.PrecedenceRelation,
) error {
	if block == nil || edge == nil || candidate == nil {
		return nil
	}
	if edge.RelationKind == types.DiagramRelPrecedence && block.Diagram != nil &&
		diagramStagePrecedenceAnchorBindsVisibleNodes(
			stagePrecedence, *edge, diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind),
		) {
		return nil
	}
	type endpoint struct {
		name     string
		node     string
		identity string
		allowed  []string
	}
	for _, endpoint := range []endpoint{
		{name: "from_node", node: edge.FromNode, identity: edge.FromIdentity, allowed: candidate.FromNodeIDs},
		{name: "to_node", node: edge.ToNode, identity: edge.ToIdentity, allowed: candidate.ToNodeIDs},
	} {
		node := strings.TrimSpace(endpoint.node)
		identity := strings.TrimSpace(endpoint.identity)
		if diagramEvidenceNodeIdentityBindsEndpoint(node, identity) ||
			atomicDiagramNodeIDListed(node, endpoint.allowed) {
			continue
		}
		return fmt.Errorf(
			"addition_ref=%q edge.%s=%q is not a typed carrier for %s=%q; use the exact technical endpoint or one producer-listed node id on this side",
			strings.TrimSpace(candidate.AdditionRef), endpoint.name, node, strings.TrimSuffix(endpoint.name, "_node")+"_identity", identity,
		)
	}
	return nil
}

func atomicDiagramNodeIDListed(node string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(node), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

// atomicDiagramInlineNodeDeclarationsToLift preserves only model-authored
// shaped node carriers that live on the exact flowchart/graph edge statement
// being removed or replaced. The edge relation itself is never retained. A
// declaration already present on another line wins, preventing duplicates;
// otherwise the exact token is lifted at the old statement's indentation so
// subgraph membership and reader-facing business labels survive. Sequence and
// class diagrams use independent participant/class declarations and remain
// byte-identical on this lane.
func atomicDiagramInlineNodeDeclarationsToLift(body string, lines []string, lineIndex int) []string {
	header := atomicDiagramHeader(body)
	if (header != "flowchart" && header != "graph") || lineIndex < 0 || lineIndex >= len(lines) {
		return nil
	}
	candidates := mermaidcompat.InlineNodeDeclarations(lines[lineIndex])
	if len(candidates) == 0 {
		return nil
	}
	existing := make(map[string]bool)
	for i, line := range lines {
		if i == lineIndex {
			continue
		}
		for _, decl := range mermaidcompat.InlineNodeDeclarations(line) {
			existing[strings.TrimSpace(decl.Ident)] = true
		}
	}
	indent := lines[lineIndex][:len(lines[lineIndex])-len(strings.TrimLeft(lines[lineIndex], " \t"))]
	seen := make(map[string]bool)
	out := make([]string, 0, len(candidates))
	for _, decl := range candidates {
		ident := strings.TrimSpace(decl.Ident)
		token := strings.TrimSpace(decl.Token)
		if ident == "" || token == "" || existing[ident] || seen[ident] {
			continue
		}
		seen[ident] = true
		out = append(out, indent+token)
	}
	return out
}

func replaceAtomicMermaidStatementLine(lines []string, index int, replacement []string) []string {
	out := make([]string, 0, len(lines)-1+len(replacement))
	out = append(out, lines[:index]...)
	out = append(out, replacement...)
	out = append(out, lines[index+1:]...)
	return out
}

// canonicalizeAtomicSequenceAdditionNodeRefs prevents one typed endpoint from
// being rendered twice under two Mermaid participant ids. Mermaid accepts an
// undeclared message endpoint and silently creates an implicit participant;
// after a local relation repair that can leave the original declared
// participant disconnected beside a second implicit copy of the same stage or
// code identity.
//
// This is a carrier-only normalization. It runs only when the sequence body
// already contains explicit participant declarations and the model-authored
// endpoint plus exactly one declaration both bind to the same hidden typed
// endpoint. Read-stage alias families are admitted only for checkout-verified
// precedence rows. Ambiguity fails closed; labels, direction, relation kind,
// technical identities, participant membership, and answer prose are never
// inferred or changed.
func canonicalizeAtomicSequenceAdditionNodeRefs(
	block *types.AnswerBlock,
	edge *types.DiagramEdgeAnchor,
	stagePrecedence []stageauthority.PrecedenceRelation,
) error {
	if block == nil || block.Diagram == nil || edge == nil ||
		types.MermaidBodySyntaxFamily(block.Diagram.Body) != types.MermaidSyntaxSequence {
		return nil
	}
	var declarations []mermaidcompat.NodeDecl
	for _, line := range strings.Split(block.Diagram.Body, "\n") {
		declarations = append(declarations, mermaidcompat.SequenceParticipantDeclarations(line)...)
	}
	if len(declarations) == 0 {
		return nil
	}
	type endpoint struct {
		name     string
		node     *string
		identity string
	}
	endpoints := []endpoint{
		{name: "from_node", node: &edge.FromNode, identity: edge.FromIdentity},
		{name: "to_node", node: &edge.ToNode, identity: edge.ToIdentity},
	}
	for _, item := range endpoints {
		current := strings.TrimSpace(*item.node)
		identity := strings.TrimSpace(item.identity)
		if current == "" || identity == "" || atomicSequenceNodeIsDeclared(current, declarations) {
			continue
		}
		canonical, matched, ambiguous := atomicSequenceUniqueDeclaredTypedNode(
			current, identity, edge.RelationKind, declarations, stagePrecedence,
		)
		if ambiguous {
			return fmt.Errorf(
				"action=add edge.%s=%q resolves to the same typed endpoint %q as multiple declared sequence participants; use one exact declared participant id",
				item.name, current, identity,
			)
		}
		if matched {
			*item.node = canonical
		}
	}
	return nil
}

func atomicSequenceNodeIsDeclared(node string, declarations []mermaidcompat.NodeDecl) bool {
	for _, declaration := range declarations {
		// Mermaid participant IDs are case-sensitive. EqualFold here made
		// `participant Analyze` appear to declare an `analyze` message
		// endpoint, so Mermaid silently created a second implicit actor while
		// the typed anchor gate believed the original participant was reused.
		if strings.TrimSpace(declaration.Ident) == strings.TrimSpace(node) {
			return true
		}
	}
	return false
}

func atomicSequenceUniqueDeclaredTypedNode(
	modelNode string,
	endpointIdentity string,
	relation types.DiagramRelationKind,
	declarations []mermaidcompat.NodeDecl,
	stagePrecedence []stageauthority.PrecedenceRelation,
) (string, bool, bool) {
	modelBindsGeneric := atomicSequenceSurfaceBindsEndpoint(modelNode, endpointIdentity)
	targetStage, targetStageOK := "", false
	modelStage, modelStageOK := "", false
	if relation == types.DiagramRelPrecedence && len(stagePrecedence) > 0 {
		targetStage, targetStageOK = atomicSequenceUniqueStageIdentity(stagePrecedence, endpointIdentity)
		modelStage, modelStageOK = atomicSequenceUniqueStageIdentity(stagePrecedence, modelNode)
	}
	modelBindsStage := targetStageOK && modelStageOK && targetStage == modelStage
	if !modelBindsGeneric && !modelBindsStage {
		return "", false, false
	}

	matches := make(map[string]string)
	for _, declaration := range declarations {
		ident := strings.TrimSpace(declaration.Ident)
		if ident == "" {
			continue
		}
		declBinds := atomicSequenceSurfaceBindsEndpoint(ident, endpointIdentity) ||
			atomicSequenceSurfaceBindsEndpoint(declaration.Label, endpointIdentity)
		if !declBinds && targetStageOK {
			if stage, ok := atomicSequenceUniqueStageIdentity(stagePrecedence, ident); ok && stage == targetStage {
				declBinds = true
			}
			if !declBinds {
				if stage, ok := atomicSequenceUniqueStageIdentity(stagePrecedence, declaration.Label); ok && stage == targetStage {
					declBinds = true
				}
			}
		}
		if declBinds {
			matches[strings.ToLower(ident)] = ident
		}
	}
	if len(matches) == 0 {
		return "", false, false
	}
	if len(matches) != 1 {
		return "", false, true
	}
	for _, ident := range matches {
		return ident, true, false
	}
	return "", false, false
}

func atomicSequenceSurfaceBindsEndpoint(surface, endpointIdentity string) bool {
	surface = strings.TrimSpace(surface)
	endpointIdentity = strings.TrimSpace(endpointIdentity)
	return surface != "" && endpointIdentity != "" &&
		(types.AnswerCodeIdentitySurfacesEquivalent(surface, endpointIdentity) ||
			types.AnswerCodeIdentitySurfacesCompatible(surface, endpointIdentity) ||
			types.AnswerCodeIdentityOwnsEndpoint(surface, endpointIdentity))
}

func atomicSequenceUniqueStageIdentity(relations []stageauthority.PrecedenceRelation, surface string) (string, bool) {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return "", false
	}
	matches := make(map[string]bool)
	visit := func(row stageauthority.StageRow) {
		if diagramStageRowIdentityMatches(row, surface) {
			matches[row.StageIdent] = true
		}
	}
	for _, relation := range relations {
		visit(relation.From)
		visit(relation.To)
	}
	if len(matches) != 1 {
		return "", false
	}
	for stage := range matches {
		return stage, true
	}
	return "", false
}

func atomicDiagramPriorAnchorRoster(lease *types.AnswerDiagramRelationRepairLease, blockID string) string {
	if lease == nil || lease.Version != 1 {
		return ""
	}
	rows := make([]string, 0, 8)
	for _, block := range lease.Blocks {
		if strings.TrimSpace(block.BlockID) != strings.TrimSpace(blockID) {
			continue
		}
		for _, anchor := range block.BaseAnchors {
			ref := ""
			for _, failure := range lease.Failures {
				if strings.TrimSpace(failure.BlockID) == strings.TrimSpace(blockID) &&
					atomicDiagramFailureMatchesAnchorExactly(failure, anchor) {
					ref = strings.TrimSpace(failure.FailureRef)
					break
				}
			}
			rows = append(rows, fmt.Sprintf(
				"prior_anchor={relation:%s node:%s->%s identity:%s->%s failure_ref:%q}",
				anchor.RelationKind, anchor.FromNode, anchor.ToNode,
				anchor.FromIdentity, anchor.ToIdentity, ref,
			))
			if len(rows) == 8 {
				break
			}
		}
		break
	}
	// A body-only missing-anchor failure has no prior anchor by definition;
	// still expose its live selector and exact structural locator.
	if len(rows) == 0 {
		for _, failure := range lease.Failures {
			if strings.TrimSpace(failure.BlockID) != strings.TrimSpace(blockID) ||
				strings.TrimSpace(failure.FailureRef) == "" {
				continue
			}
			rows = append(rows, fmt.Sprintf(
				"body_failure={failure_ref:%q issue:%s relation:%s node:%s->%s identity:%s->%s body_occurrence:%d}",
				failure.FailureRef, failure.Issue, failure.RelationKind,
				failure.FromNode, failure.ToNode, failure.FromIdentity, failure.ToIdentity, failure.BodyOccurrence,
			))
			if len(rows) == 8 {
				break
			}
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return "; live exact prior-anchor roster: " + strings.Join(rows, "; ")
}

// atomicDiagramAnchorWithoutBodyFailureAuthorized admits only the exact
// producer-owned stale-anchor failure from the immediately preceding repair
// lease. The model still chooses whether to remove that anchor or replace it
// with a complete visible edge. No Mermaid label, request text, reasoning, or
// answer prose participates in this permission.
func atomicDiagramAnchorWithoutBodyFailureAuthorized(
	lease *types.AnswerDiagramRelationRepairLease,
	blockID string,
	anchor types.DiagramEdgeAnchor,
) bool {
	if lease == nil || lease.Version != 1 || strings.TrimSpace(blockID) == "" {
		return false
	}
	for _, failure := range lease.Failures {
		if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierStaleAnchor ||
			strings.TrimSpace(failure.BlockID) != strings.TrimSpace(blockID) {
			continue
		}
		if failure.RelationKind.IsValid() && failure.RelationKind != anchor.RelationKind {
			continue
		}
		if strings.TrimSpace(failure.FromNode) != "" || strings.TrimSpace(failure.ToNode) != "" {
			if strings.TrimSpace(failure.FromNode) != strings.TrimSpace(anchor.FromNode) ||
				strings.TrimSpace(failure.ToNode) != strings.TrimSpace(anchor.ToNode) {
				continue
			}
		}
		if strings.TrimSpace(failure.FromIdentity) != "" || strings.TrimSpace(failure.ToIdentity) != "" {
			if strings.TrimSpace(failure.FromIdentity) != strings.TrimSpace(anchor.FromIdentity) ||
				strings.TrimSpace(failure.ToIdentity) != strings.TrimSpace(anchor.ToIdentity) {
				continue
			}
		}
		return true
	}
	return false
}

func atomicDiagramBodyOccurrence(
	body string,
	anchors []types.DiagramEdgeAnchor,
	fromNode, toNode string,
	anchorPairOccurrence, declaredBodyOccurrence int,
	bodyOnly bool,
) (int, error) {
	bodyCount := atomicDiagramBodyPairCount(body, fromNode, toNode)
	if bodyCount == 0 {
		return 0, fmt.Errorf("Mermaid body has no matching edge for %s->%s", fromNode, toNode)
	}
	if declaredBodyOccurrence > 0 {
		if declaredBodyOccurrence > bodyCount {
			return 0, fmt.Errorf("body_occurrence=%d exceeds %d visible edge(s) for %s->%s", declaredBodyOccurrence, bodyCount, fromNode, toNode)
		}
		return declaredBodyOccurrence, nil
	}
	if bodyCount == 1 {
		return 1, nil
	}
	if !bodyOnly {
		anchorCount := atomicDiagramAnchorPairCount(anchors, fromNode, toNode)
		if anchorCount == bodyCount && anchorPairOccurrence > 0 && anchorPairOccurrence <= bodyCount {
			return anchorPairOccurrence, nil
		}
	}
	return 0, fmt.Errorf(
		"Mermaid body has %d edges for %s->%s that do not map one-to-one to prior anchors; set body_occurrence explicitly",
		bodyCount, fromNode, toNode,
	)
}

func atomicDiagramBodyPairCount(body, fromNode, toNode string) int {
	fromNode, toNode = strings.TrimSpace(fromNode), strings.TrimSpace(toNode)
	count := 0
	for _, edge := range mermaidcompat.ParseEdges(body) {
		if strings.TrimSpace(edge.From) == fromNode && strings.TrimSpace(edge.To) == toNode {
			count++
		}
	}
	return count
}

func atomicDiagramAnchorPairCount(anchors []types.DiagramEdgeAnchor, fromNode, toNode string) int {
	fromNode, toNode = strings.TrimSpace(fromNode), strings.TrimSpace(toNode)
	count := 0
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.FromNode) == fromNode && strings.TrimSpace(anchor.ToNode) == toNode {
			count++
		}
	}
	return count
}

func atomicDiagramBodyOnlyFailureAuthorized(
	lease *types.AnswerDiagramRelationRepairLease,
	blockID string,
	match types.DiagramEdgeAnchor,
) bool {
	if lease == nil || lease.Version != 1 || strings.TrimSpace(blockID) == "" {
		return false
	}
	for _, failure := range lease.Failures {
		if failure.TargetCarrier != types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
			continue
		}
		if strings.TrimSpace(failure.BlockID) != strings.TrimSpace(blockID) ||
			strings.TrimSpace(failure.FromNode) != strings.TrimSpace(match.FromNode) ||
			strings.TrimSpace(failure.ToNode) != strings.TrimSpace(match.ToNode) {
			continue
		}
		if failure.RelationKind.IsValid() && failure.RelationKind != match.RelationKind {
			continue
		}
		return true
	}
	return false
}

func validateAtomicDiagramAnchor(anchor *types.DiagramEdgeAnchor, field string) error {
	if anchor == nil {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(anchor.FromNode) == "" || strings.TrimSpace(anchor.ToNode) == "" || !anchor.RelationKind.IsValid() {
		return fmt.Errorf("%s requires from_node, to_node, and a valid relation_kind", field)
	}
	if strings.ContainsAny(anchor.FromNode+anchor.ToNode+anchor.VisibleLabel, "\r\n") {
		return fmt.Errorf("%s fields must be single-line", field)
	}
	return nil
}

func atomicDiagramAnchorSameTuple(left, right types.DiagramEdgeAnchor) bool {
	return strings.TrimSpace(left.FromNode) == strings.TrimSpace(right.FromNode) &&
		strings.TrimSpace(left.ToNode) == strings.TrimSpace(right.ToNode) &&
		strings.TrimSpace(left.FromIdentity) == strings.TrimSpace(right.FromIdentity) &&
		strings.TrimSpace(left.ToIdentity) == strings.TrimSpace(right.ToIdentity) &&
		left.RelationKind == right.RelationKind
}

func findAtomicDiagramAnchor(anchors []types.DiagramEdgeAnchor, match types.DiagramEdgeAnchor, occurrence int) (int, int, error) {
	exactSeen := 0
	pairSeen := 0
	for i, anchor := range anchors {
		if strings.TrimSpace(anchor.FromNode) == strings.TrimSpace(match.FromNode) &&
			strings.TrimSpace(anchor.ToNode) == strings.TrimSpace(match.ToNode) {
			pairSeen++
		}
		if !atomicDiagramAnchorSameTuple(anchor, match) {
			continue
		}
		exactSeen++
		if exactSeen == occurrence {
			return i, pairSeen, nil
		}
	}
	return -1, 0, fmt.Errorf("match did not select occurrence %d of an exact prior anchor", occurrence)
}

func atomicDiagramHeader(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		return strings.ToLower(strings.Fields(line)[0])
	}
	return ""
}

func findAtomicMermaidEdgeLine(body, fromNode, toNode string, occurrence int) (int, error) {
	header := atomicDiagramHeader(body)
	if header == "" {
		return -1, fmt.Errorf("diagram body has no syntax header")
	}
	seen := 0
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		parsed := mermaidcompat.ParseEdges(header + "\n" + strings.TrimSpace(raw))
		for _, edge := range parsed {
			if strings.TrimSpace(edge.From) == strings.TrimSpace(fromNode) && strings.TrimSpace(edge.To) == strings.TrimSpace(toNode) {
				seen++
				if seen == occurrence {
					if len(parsed) != 1 {
						return -1, fmt.Errorf("matched Mermaid statement contains multiple edges; split it before an atomic edit")
					}
					return i, nil
				}
			}
		}
	}
	return -1, fmt.Errorf("Mermaid body has no matching edge occurrence %d for %s->%s", occurrence, fromNode, toNode)
}

func renderAtomicMermaidEdgeLine(body string, edge types.DiagramEdgeAnchor) (string, error) {
	label := strings.TrimSpace(edge.VisibleLabel)
	if label == "" || strings.ContainsAny(label, "\r\n") {
		return "", fmt.Errorf("edge.visible_label must be one non-empty line")
	}
	fromNode, toNode := strings.TrimSpace(edge.FromNode), strings.TrimSpace(edge.ToNode)
	switch atomicDiagramHeader(body) {
	case "sequencediagram":
		op := "->>"
		if edge.RelationKind == types.DiagramRelReturn {
			op = "-->>"
		}
		return "    " + fromNode + op + toNode + ": " + label, nil
	case "classdiagram":
		return "    " + fromNode + " --> " + toNode + " : " + label, nil
	case "flowchart", "graph":
		if strings.Contains(label, "|") {
			return "", fmt.Errorf("flowchart visible_label may not contain an unescaped pipe")
		}
		return "    " + fromNode + " -->|" + label + "| " + toNode, nil
	default:
		return "", fmt.Errorf("unsupported Mermaid body family for atomic edit")
	}
}

func relabelAtomicMermaidEdgeLine(body, rawLine, label string) (string, error) {
	header := atomicDiagramHeader(body)
	indent := rawLine[:len(rawLine)-len(strings.TrimLeft(rawLine, " \t"))]
	line := strings.TrimSpace(rawLine)
	switch header {
	case "sequencediagram":
		arrowAt, operator := mermaidcompat.FindSequenceArrow(line)
		if arrowAt < 0 {
			return "", fmt.Errorf("matched sequence statement has no arrow")
		}
		colon := atomicSequenceMessageColon(line, arrowAt+len(operator))
		if colon < 0 {
			return "", fmt.Errorf("matched sequence statement has no message label")
		}
		return indent + strings.TrimSpace(line[:colon]) + ": " + label, nil
	case "classdiagram":
		if at := strings.LastIndex(line, " : "); at >= 0 {
			return indent + strings.TrimSpace(line[:at]) + " : " + label, nil
		}
		return indent + line + " : " + label, nil
	case "flowchart", "graph":
		if strings.Contains(label, "|") {
			return "", fmt.Errorf("flowchart visible_label may not contain an unescaped pipe")
		}
		arrowAt, operator := mermaidcompat.FindFlowchartArrow(line)
		if arrowAt < 0 {
			return "", fmt.Errorf("matched flowchart statement has no arrow")
		}
		tailAt := arrowAt + len(operator)
		tail := line[tailAt:]
		trimmedTail := strings.TrimLeft(tail, " \t")
		spaceLen := len(tail) - len(trimmedTail)
		if strings.HasPrefix(trimmedTail, "|") {
			if end := strings.Index(trimmedTail[1:], "|"); end >= 0 {
				end++
				return indent + line[:tailAt] + tail[:spaceLen] + "|" + label + "|" + trimmedTail[end+1:], nil
			}
			return "", fmt.Errorf("matched flowchart statement has an unterminated label")
		}
		return indent + line[:tailAt] + "|" + label + "| " + strings.TrimSpace(tail), nil
	default:
		return "", fmt.Errorf("unsupported Mermaid body family for atomic relabel")
	}
}

func atomicSequenceMessageColon(line string, start int) int {
	for i := start; i < len(line); i++ {
		if line[i] != ':' {
			continue
		}
		if (i > 0 && line[i-1] == ':') || (i+1 < len(line) && line[i+1] == ':') {
			continue
		}
		return i
	}
	return -1
}
