package types

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// TraceInputPreparer prepares a complete physical trace at the product/tool
// boundary. One handle belongs to one Run and is shared by all projections.
// It never denotes a sticky attachment or a model-created completeness claim.
// Implementations validate cached file generations on every call and honor the
// caller's cancellation without poisoning later requests in the same Run.
type TraceInputPreparer interface {
	Prepare(context.Context, string) (*attachment.TraceMaterial, error)
	// PreparedMaterials exposes successful receipts for exact capture aliasing.
	// Consumers still validate each receipt; this never triggers preparation.
	PreparedMaterials() []*attachment.TraceMaterial
}
