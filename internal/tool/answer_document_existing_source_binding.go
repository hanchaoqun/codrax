package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Identity support is separate from relation authority: full-pool definitions
// and owner stamps may bind a displayed qualified name, but the proved call
// itself must still be an exact current request-scoped operation. This value
// is validator-local and never changes evidence, model anchors, or the body.
type diagramExistingSourceBinding struct {
	evidence []types.EvidenceItem
	required []types.AnswerRequiredAnchor
}

// A node-only anchor may use the strict visible-name proof; a partially
// supplied pair is not node-only and must never have its asserted half
// silently replaced by labels. Keep that malformed metadata for repair.
func diagramExistingSourceBindingHasPartialIdentity(anchors []types.DiagramEdgeAnchor, edgeKey string) bool {
	for _, anchor := range anchors {
		if diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode) == edgeKey &&
			(strings.TrimSpace(anchor.FromIdentity) == "") != (strings.TrimSpace(anchor.ToIdentity) == "") {
			return true
		}
	}
	return false
}

func (binding diagramExistingSourceBinding) provesRequestedCall(from, to string, requested []types.EvidenceItem) bool {
	for _, proved := range diagramCallEdgeRequiredQualifiedCallerEvidence(binding.evidence, binding.required, from, to) {
		for _, selected := range requested {
			if proved.ID == selected.ID && proved.Source == selected.Source &&
				proved.LineStart == selected.LineStart && proved.LineEnd == selected.LineEnd &&
				proved.Subject == selected.Subject && proved.Object == selected.Object &&
				proved.AnchorKind == selected.AnchorKind && proved.AnchorSymbol == selected.AnchorSymbol &&
				proved.OwnerSymbol == selected.OwnerSymbol && proved.OwnerIdentity == selected.OwnerIdentity &&
				proved.Origin == selected.Origin && proved.GroundingStatus == selected.GroundingStatus &&
				proved.Producer == selected.Producer && types.ClaimFormOf(selected) == types.ClaimCallEdge {
				return true
			}
		}
	}
	return false
}
