package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// AnswerDiagramParticipantVisibilityRepairAction is a model-selected local
// declaration edit. It cannot create an edge, anchor, relation, conclusion, or
// reader-facing wording by itself; the patch call must still provide the node
// id and visible label.
type AnswerDiagramParticipantVisibilityRepairAction string

const AnswerDiagramParticipantVisibilityRepairEnsureVisible AnswerDiagramParticipantVisibilityRepairAction = "ensure_visible"

// AnswerDiagramParticipantVisibilityRepairFailure binds one exact typed
// participant-visibility mismatch to the immutable rejected diagram carrier.
// BaseDiagramFingerprint remains executor-only generation authority.
type AnswerDiagramParticipantVisibilityRepairFailure struct {
	ParticipantRef            string                                           `json:"participant_ref,omitempty"`
	BlockID                   string                                           `json:"block_id"`
	Participant               string                                           `json:"participant"`
	Issue                     string                                           `json:"issue"`
	AllowedParticipantActions []AnswerDiagramParticipantVisibilityRepairAction `json:"allowed_participant_actions,omitempty"`
	BaseDiagramFingerprint    string                                           `json:"-"`
}

func (f AnswerDiagramParticipantVisibilityRepairFailure) AllowsParticipantAction(action string) bool {
	action = strings.TrimSpace(action)
	for _, allowed := range f.AllowedParticipantActions {
		if action == string(allowed) {
			return true
		}
	}
	return false
}

// WithAnswerDiagramParticipantVisibilityRepairFailures installs only the
// exact boundary_participant_not_visible rows. Other participant mismatch
// families keep their existing boundary/relation capabilities or broad
// model-authored repair path.
func WithAnswerDiagramParticipantVisibilityRepairFailures(
	base *AnswerDocumentV2,
	lease *AnswerDiagramRelationRepairLease,
	failures []AnswerDiagramParticipantVisibilityRepairFailure,
) *AnswerDiagramRelationRepairLease {
	if base == nil || len(failures) == 0 || len(failures) > AnswerDiagramRelationRepairDeltaMaxEntries {
		return lease
	}
	out := cloneAnswerDiagramRelationRepairLease(lease)
	if out == nil {
		out = &AnswerDiagramRelationRepairLease{Version: 1}
	} else if out.Version != 1 || len(ValidateAnswerDiagramRelationRepairLease(out, base)) != 0 {
		return nil
	}
	out.ParticipantVisibilityFailures = nil

	blockCounts := make(map[string]int, len(base.Blocks))
	blocks := make(map[string]AnswerBlock, len(base.Blocks))
	for _, block := range base.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		blockCounts[id]++
		blocks[id] = block
	}
	if lease != nil {
		for _, prior := range lease.ParticipantVisibilityFailures {
			block, ok := blocks[strings.TrimSpace(prior.BlockID)]
			if !ok || blockCounts[strings.TrimSpace(prior.BlockID)] != 1 ||
				AnswerDiagramParticipantVisibilityFingerprint(block) != prior.BaseDiagramFingerprint {
				return nil
			}
		}
	}

	clean := make([]AnswerDiagramParticipantVisibilityRepairFailure, 0, len(failures))
	seen := make(map[string]bool, len(failures))
	for _, failure := range failures {
		failure.ParticipantRef = ""
		failure.BlockID = strings.TrimSpace(failure.BlockID)
		failure.Participant = strings.TrimSpace(failure.Participant)
		failure.Issue = strings.TrimSpace(failure.Issue)
		block, ok := blocks[failure.BlockID]
		if !ok || blockCounts[failure.BlockID] != 1 || block.Kind != BlockDiagram || block.Diagram == nil ||
			failure.Participant == "" || failure.Issue != "boundary_participant_not_visible" {
			continue
		}
		failure.AllowedParticipantActions = []AnswerDiagramParticipantVisibilityRepairAction{
			AnswerDiagramParticipantVisibilityRepairEnsureVisible,
		}
		failure.BaseDiagramFingerprint = AnswerDiagramParticipantVisibilityFingerprint(block)
		key := strings.ToLower(failure.BlockID) + "\x00" + strings.ToLower(failure.Participant) + "\x00" + failure.Issue
		if seen[key] {
			continue
		}
		seen[key] = true
		failure.ParticipantRef = answerDiagramParticipantVisibilityRepairRef(failure)
		if failure.ParticipantRef == "" {
			return lease
		}
		clean = append(clean, failure)
	}
	if len(clean) == 0 {
		return out
	}
	sort.SliceStable(clean, func(i, j int) bool { return clean[i].ParticipantRef < clean[j].ParticipantRef })
	out.ParticipantVisibilityFailures = clean
	if len(out.Blocks) == 0 {
		for _, block := range base.Blocks {
			id := strings.TrimSpace(block.ID)
			if id == "" || block.Kind != BlockDiagram {
				continue
			}
			out.Blocks = append(out.Blocks, AnswerDiagramRelationRepairLeaseBlock{
				BlockID: id, Kind: block.Kind,
				BaseAnchors: append([]DiagramEdgeAnchor(nil), block.EdgeAnchors...),
			})
		}
	}
	return out
}

func answerDiagramParticipantVisibilityRepairRef(failure AnswerDiagramParticipantVisibilityRepairFailure) string {
	payload := struct {
		BlockID     string                                           `json:"block_id"`
		Participant string                                           `json:"participant"`
		Issue       string                                           `json:"issue"`
		Actions     []AnswerDiagramParticipantVisibilityRepairAction `json:"actions"`
		Fingerprint string                                           `json:"fingerprint"`
	}{failure.BlockID, failure.Participant, failure.Issue, failure.AllowedParticipantActions, failure.BaseDiagramFingerprint}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "rp1-" + hex.EncodeToString(sum[:12])
}

// AnswerDiagramParticipantVisibilityFingerprint binds the local capability to
// the exact diagram generation without interpreting its prose or labels.
func AnswerDiagramParticipantVisibilityFingerprint(block AnswerBlock) string {
	if block.Kind != BlockDiagram || block.Diagram == nil {
		return ""
	}
	payload := struct {
		ID      string              `json:"id"`
		Kind    AnswerBlockKind     `json:"kind"`
		Diagram *AnswerDiagramBlock `json:"diagram"`
	}{strings.TrimSpace(block.ID), block.Kind, block.Diagram}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
