package tool

import "github.com/hanchaoqun/codrax/internal/types"

// Reuse only a completed, unique current query selection, and only when the
// typed named-input carriers independently identify that original. The bare
// ZIP query contract stays unchanged; this is the system supplement's lane.
func traceSupplementSelectedZIPParams(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, bool) {
	if ctx == nil || ctx.Mutable == nil || ctx.TraceInputPreparer == nil || p.Path != "" || p.Source != "" ||
		p.ArchiveMember != "" || ctx.AttachedTraceMaterial != nil || ctx.AttachedHitrace != "" {
		return p, true
	}
	material, state := ctx.Mutable.SuccessfulPreparedArchiveSelection(contextFromBus(ctx))
	if state == types.TracePreparedSelectionAbsent {
		return p, true
	}
	if state != types.TracePreparedSelectionUnique || material == nil {
		return p, false
	}
	if scope := traceSupplementRequestedArtifactScope(ctx); scope != nil && scope.Comparison != nil {
		return p, false // An outer archive does not identify a comparison side/member.
	}
	candidates := traceQueryReferencedArtifactCandidates(ctx, material)
	if len(candidates) != 1 || !material.MatchesSelection(candidates[0].resolved, material.ArchiveMember()) ||
		traceQueryValidateReadyMaterial(ctx, material) != nil {
		// Do not drop the selected ZIP and let the legacy suffix-based
		// resolver silently promote a different named original instead.
		return p, false
	}
	p.Source, p.Path = "path", material.QueryPath()
	return p, true
}
