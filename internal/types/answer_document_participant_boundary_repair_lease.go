package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// AnswerDiagramParticipantBoundaryRepairAction is one model-selected local
// mutation over participant_boundaries. It never creates, removes, or labels a
// Mermaid edge.
type AnswerDiagramParticipantBoundaryRepairAction string

const (
	AnswerDiagramParticipantBoundaryRepairAddUnproven AnswerDiagramParticipantBoundaryRepairAction = "add_unproven"
	AnswerDiagramParticipantBoundaryRepairRemove      AnswerDiagramParticipantBoundaryRepairAction = "remove_boundary"
	AnswerDiagramParticipantBoundaryRepairDeduplicate AnswerDiagramParticipantBoundaryRepairAction = "deduplicate_boundary"
)

// AnswerDiagramParticipantBoundaryRepairFailure binds one precise validator
// mismatch to the boundary-only actions executable against the same rejected
// carrier generation. BaseBoundaryFingerprint is internal execution authority;
// it is intentionally absent from the model-visible retry delta.
type AnswerDiagramParticipantBoundaryRepairFailure struct {
	BoundaryRef             string                                         `json:"boundary_ref,omitempty"`
	BlockID                 string                                         `json:"block_id"`
	Participant             string                                         `json:"participant"`
	Issue                   string                                         `json:"issue"`
	AllowedBoundaryActions  []AnswerDiagramParticipantBoundaryRepairAction `json:"allowed_boundary_actions,omitempty"`
	BaseBoundaryFingerprint string                                         `json:"-"`
}

func (f AnswerDiagramParticipantBoundaryRepairFailure) AllowsBoundaryAction(action string) bool {
	action = strings.TrimSpace(action)
	for _, allowed := range f.AllowedBoundaryActions {
		if action == string(allowed) {
			return true
		}
	}
	return false
}

// WithAnswerDiagramParticipantBoundaryRepairFailures installs boundary-only
// capabilities onto an optional relation lease. The relation and boundary
// lanes share one immutable patch generation but remain separate authorities:
// boundary failures cannot mint a relation, and relation failures cannot edit
// participant_boundaries.
func WithAnswerDiagramParticipantBoundaryRepairFailures(
	base *AnswerDocumentV2,
	lease *AnswerDiagramRelationRepairLease,
	failures []AnswerDiagramParticipantBoundaryRepairFailure,
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
	// Every producer generation replaces, rather than accumulates, the local
	// participant mismatch roster. Relation refs on the same immutable base are
	// preserved independently.
	out.ParticipantBoundaryFailures = nil

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
		for _, prior := range lease.ParticipantBoundaryFailures {
			block, ok := blocks[strings.TrimSpace(prior.BlockID)]
			if !ok || blockCounts[strings.TrimSpace(prior.BlockID)] != 1 ||
				AnswerDiagramParticipantBoundaryFingerprint(block.ParticipantBoundaries) != prior.BaseBoundaryFingerprint {
				return nil
			}
		}
	}
	clean := make([]AnswerDiagramParticipantBoundaryRepairFailure, 0, len(failures))
	seen := make(map[string]bool, len(failures))
	for _, failure := range failures {
		failure.BoundaryRef = ""
		failure.BlockID = strings.TrimSpace(failure.BlockID)
		failure.Participant = strings.TrimSpace(failure.Participant)
		failure.Issue = strings.TrimSpace(failure.Issue)
		block, ok := blocks[failure.BlockID]
		if !ok || blockCounts[failure.BlockID] != 1 || block.Kind != BlockDiagram ||
			failure.Participant == "" || failure.Issue == "" {
			return lease
		}
		count := answerDiagramParticipantBoundaryCount(block.ParticipantBoundaries, failure.Participant)
		switch failure.Issue {
		case "stale_boundary_for_connected_participant", "unknown_or_context_only_boundary":
			if count != 1 {
				continue
			}
			failure.AllowedBoundaryActions = []AnswerDiagramParticipantBoundaryRepairAction{AnswerDiagramParticipantBoundaryRepairRemove}
		case "missing_unproven_boundary":
			if count != 0 {
				continue
			}
			failure.AllowedBoundaryActions = []AnswerDiagramParticipantBoundaryRepairAction{AnswerDiagramParticipantBoundaryRepairAddUnproven}
		case "duplicate_unproven_boundary":
			if count < 2 {
				continue
			}
			failure.AllowedBoundaryActions = []AnswerDiagramParticipantBoundaryRepairAction{AnswerDiagramParticipantBoundaryRepairDeduplicate}
		default:
			// Identity, endpoint, component, and typed-edge defects require a
			// model-authored visible graph change before the boundary can be
			// recomputed. Publishing a boundary-only action for them would be an
			// executable but semantically incomplete capability.
			continue
		}
		failure.BaseBoundaryFingerprint = AnswerDiagramParticipantBoundaryFingerprint(block.ParticipantBoundaries)
		key := strings.ToLower(failure.BlockID) + "\x00" + strings.ToLower(failure.Participant) + "\x00" + failure.Issue
		if seen[key] {
			continue
		}
		seen[key] = true
		failure.BoundaryRef = answerDiagramParticipantBoundaryRepairRef(failure)
		if failure.BoundaryRef == "" {
			return lease
		}
		clean = append(clean, failure)
	}
	if len(clean) == 0 {
		return out
	}
	sort.SliceStable(clean, func(i, j int) bool { return clean[i].BoundaryRef < clean[j].BoundaryRef })
	out.ParticipantBoundaryFailures = clean
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

func answerDiagramParticipantBoundaryRepairRef(failure AnswerDiagramParticipantBoundaryRepairFailure) string {
	payload := struct {
		BlockID     string                                         `json:"block_id"`
		Participant string                                         `json:"participant"`
		Issue       string                                         `json:"issue"`
		Actions     []AnswerDiagramParticipantBoundaryRepairAction `json:"actions"`
		Fingerprint string                                         `json:"fingerprint"`
	}{failure.BlockID, failure.Participant, failure.Issue, failure.AllowedBoundaryActions, failure.BaseBoundaryFingerprint}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "rb1-" + hex.EncodeToString(sum[:12])
}

// AnswerDiagramParticipantBoundaryFingerprint lets the executor reject a ref
// rebound to a different boundary generation without inspecting diagram body,
// labels, request text, reasoning, or answer prose.
func AnswerDiagramParticipantBoundaryFingerprint(boundaries []DiagramParticipantBoundary) string {
	raw, err := json.Marshal(boundaries)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func answerDiagramParticipantBoundaryCount(boundaries []DiagramParticipantBoundary, participant string) int {
	participant = strings.TrimSpace(participant)
	count := 0
	for _, boundary := range boundaries {
		if strings.EqualFold(strings.TrimSpace(boundary.Participant), participant) {
			count++
		}
	}
	return count
}
