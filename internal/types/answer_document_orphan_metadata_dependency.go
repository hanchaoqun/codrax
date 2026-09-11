package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// AnswerDiagramOrphanMetadataDependency is a producer-owned, current-generation
// cleanup receipt. It is not an evidence claim or a mandatory disposition.
// Only a caller that has established the original selected body-removal source
// may bind one; readers can recheck but never reconstruct it from model JSON.
type AnswerDiagramOrphanMetadataDependency struct {
	baseFingerprint string
	failureRefs     []string
}

func cloneAnswerDiagramOrphanMetadataDependency(in *AnswerDiagramOrphanMetadataDependency) *AnswerDiagramOrphanMetadataDependency {
	if in == nil {
		return nil
	}
	out := *in
	out.failureRefs = append([]string(nil), in.failureRefs...)
	return &out
}

// MarshalJSON adds a read-only omission hint, derived from private provenance.
// Unmarshal has no matching authority field: decision_optional cannot mint a
// receipt or relax the executor's original candidate checks.
func (c AnswerDiagramOrphanCleanupCandidate) MarshalJSON() ([]byte, error) {
	type plain AnswerDiagramOrphanCleanupCandidate
	return json.Marshal(struct {
		plain
		DecisionOptional bool `json:"decision_optional,omitempty"`
	}{plain(c), c.MetadataDependency != nil})
}

// BindAnswerDiagramOrphanMetadataDependency binds every current incident
// metadata occurrence, not a pair-level guess or a subset. The original causal
// source is checked by the tool producer before this function is called.
func BindAnswerDiagramOrphanMetadataDependency(base *AnswerDocumentV2, candidate AnswerDiagramOrphanCleanupCandidate, lease *AnswerDiagramRelationRepairLease) (AnswerDiagramOrphanCleanupCandidate, bool) {
	if base == nil || lease == nil || candidate.BlockID == "" || candidate.ParticipantID == "" || len(candidate.AllowedActions) == 0 {
		return AnswerDiagramOrphanCleanupCandidate{}, false
	}
	var block *AnswerBlock
	for i := range base.Blocks {
		if base.Blocks[i].ID == candidate.BlockID {
			if block != nil {
				return AnswerDiagramOrphanCleanupCandidate{}, false
			}
			block = &base.Blocks[i]
		}
	}
	if block == nil || block.Kind != BlockDiagram || block.Diagram == nil {
		return AnswerDiagramOrphanCleanupCandidate{}, false
	}
	declarations := 0
	for _, decl := range mermaidcompat.RemovableNodeDeclarations(block.Diagram.Body) {
		if decl.Ident == candidate.ParticipantID {
			declarations++
		}
	}
	if declarations != 1 || mermaidcompat.SequenceParticipantReferenced(block.Diagram.Body, candidate.ParticipantID) {
		return AnswerDiagramOrphanCleanupCandidate{}, false
	}
	for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
		if edge.From == candidate.ParticipantID || edge.To == candidate.ParticipantID {
			return AnswerDiagramOrphanCleanupCandidate{}, false
		}
	}
	var refs []string
	for index, anchor := range block.EdgeAnchors {
		if strings.TrimSpace(anchor.FromNode) != candidate.ParticipantID && strings.TrimSpace(anchor.ToNode) != candidate.ParticipantID {
			continue
		}
		ref := ""
		for _, failure := range lease.Failures {
			if failure.BlockID != block.ID || failure.AnchorOccurrence != index+1 ||
				failure.TargetCarrier != AnswerDiagramRelationRepairCarrierStaleAnchor ||
				!failure.AllowsAction(string(AnswerDiagramRelationRepairActionRemove)) ||
				!AnswerDiagramRelationRepairOccurrenceMatchesBase(base, failure) {
				continue
			}
			if ref != "" && ref != failure.FailureRef {
				return AnswerDiagramOrphanCleanupCandidate{}, false
			}
			ref = failure.FailureRef
		}
		if ref == "" {
			return AnswerDiagramOrphanCleanupCandidate{}, false
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return AnswerDiagramOrphanCleanupCandidate{}, false
	}
	raw, err := json.Marshal(struct {
		ID          string
		Participant string
		Kind        AnswerBlockKind
		Diagram     *AnswerDiagramBlock
		Anchors     []DiagramEdgeAnchor
	}{block.ID, candidate.ParticipantID, block.Kind, block.Diagram, block.EdgeAnchors})
	if err != nil {
		return AnswerDiagramOrphanCleanupCandidate{}, false
	}
	sum := sha256.Sum256(raw)
	candidate.MetadataDependency = &AnswerDiagramOrphanMetadataDependency{baseFingerprint: hex.EncodeToString(sum[:]), failureRefs: refs}
	candidate.DispositionBaseFingerprint = ""
	candidate.AllowedActions = append([]AnswerDiagramOrphanDispositionAction(nil), candidate.AllowedActions...)
	return candidate, true
}

func AnswerDiagramOrphanMetadataDependencyMatchesBase(base *AnswerDocumentV2, candidate AnswerDiagramOrphanCleanupCandidate, lease *AnswerDiagramRelationRepairLease) bool {
	if candidate.MetadataDependency == nil {
		return false
	}
	bound, ok := BindAnswerDiagramOrphanMetadataDependency(base, candidate, lease)
	return ok && reflect.DeepEqual(bound.MetadataDependency, candidate.MetadataDependency)
}

// CarryAnswerDiagramOrphanMetadataDependencies reads only the installed private
// receipts. A fresh delta cannot supply or upgrade this provenance. Rebinding
// must preserve the entire current base and every incident selector exactly.
func CarryAnswerDiagramOrphanMetadataDependencies(base *AnswerDocumentV2, source, target *AnswerDiagramRelationRepairLease) []AnswerDiagramOrphanCleanupCandidate {
	if source == nil || target == nil {
		return nil
	}
	var out []AnswerDiagramOrphanCleanupCandidate
	for _, candidate := range source.OptionalOrphanCleanups {
		if !AnswerDiagramOrphanMetadataDependencyMatchesBase(base, candidate, source) ||
			!AnswerDiagramOrphanMetadataDependencyMatchesBase(base, candidate, target) {
			continue
		}
		candidate.MetadataDependency = cloneAnswerDiagramOrphanMetadataDependency(candidate.MetadataDependency)
		candidate.AllowedActions = append([]AnswerDiagramOrphanDispositionAction(nil), candidate.AllowedActions...)
		out = append(out, candidate)
	}
	return out
}
