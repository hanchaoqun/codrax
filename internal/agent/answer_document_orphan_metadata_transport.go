package agent

import (
	"encoding/json"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This DTO is display-only. In particular, its optional hint never constructs
// the private receipt that the executor requires for a cleanup operation.
type answerDocOrphanCleanupVisibleCandidate struct {
	BlockID          string                                       `json:"block_id"`
	ParticipantID    string                                       `json:"participant_id"`
	VisibleLabel     string                                       `json:"visible_label,omitempty"`
	AllowedActions   []types.AnswerDiagramOrphanDispositionAction `json:"allowed_actions"`
	DecisionOptional bool                                         `json:"decision_optional,omitempty"`
}

func answerDocOrphanCleanupVisibleRows(raw []byte) []answerDocOrphanCleanupVisibleCandidate {
	var display struct {
		Rows []answerDocOrphanCleanupVisibleCandidate `json:"optional_orphan_cleanups"`
	}
	if json.Unmarshal(raw, &display) != nil {
		return nil
	}
	return display.Rows
}

func answerDocCarryMetadataOrphanSources(base *types.AnswerDocumentV2, lease *types.AnswerDiagramRelationRepairLease, ctx *types.AgentContext, primary *types.MutableState) []types.AnswerDiagramOrphanCleanupCandidate {
	var mutables []*types.MutableState
	if ctx != nil && ctx.Mutable != nil {
		mutables = append(mutables, ctx.Mutable)
	}
	if primary != nil && (len(mutables) == 0 || primary != mutables[0]) {
		mutables = append(mutables, primary)
	}
	var out []types.AnswerDiagramOrphanCleanupCandidate
	indexes := make(map[string]int)
	conflicts := make(map[string]bool)
	for _, mu := range mutables {
		for _, candidate := range types.CarryAnswerDiagramOrphanMetadataDependencies(base, mu.AnswerDiagramRelationRepairLease(), lease) {
			key := candidate.BlockID + "\x00" + candidate.ParticipantID
			if index, exists := indexes[key]; exists {
				// Both receipts already rechecked the entire identical base/ref
				// population. Different action/label payloads are not first-wins.
				if candidate.VisibleLabel != out[index].VisibleLabel || !reflect.DeepEqual(candidate.AllowedActions, out[index].AllowedActions) {
					conflicts[key] = true
				}
				continue
			}
			indexes[key] = len(out)
			out = append(out, candidate)
		}
	}
	kept := out[:0]
	for _, candidate := range out {
		if !conflicts[candidate.BlockID+"\x00"+candidate.ParticipantID] {
			kept = append(kept, candidate)
		}
	}
	return kept
}
