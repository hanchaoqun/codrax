package skill

import "crypto/sha256"

// SharedGuidanceID names a contract supplied by more than one prompt producer.
// Ownership is internal metadata, not a skill name or a YAML-controlled flag.
type SharedGuidanceID string

const TraceQueryViewMatrixGuidance SharedGuidanceID = "trace_query_view_matrix"

type sharedGuidanceOwner struct {
	id         SharedGuidanceID
	bodyDigest [sha256.Size]byte
}

// withSharedGuidance attaches ownership where the default body is constructed.
// A custom copy that changes the body must fall back to dynamic guidance, even
// if it retains the original rule's private metadata.
func (item TierBItem) withSharedGuidance(id SharedGuidanceID) TierBItem {
	item.sharedGuidance = &sharedGuidanceOwner{id: id, bodyDigest: sha256.Sum256([]byte(item.Body))}
	return item
}

// ProvidesSharedGuidance is true only for an unchanged, internally owned rule.
// The caller must additionally establish that this rule really renders.
func (item TierBItem) ProvidesSharedGuidance(id SharedGuidanceID) bool {
	return item.sharedGuidance != nil && item.sharedGuidance.id == id &&
		item.sharedGuidance.bodyDigest == sha256.Sum256([]byte(item.Body))
}
