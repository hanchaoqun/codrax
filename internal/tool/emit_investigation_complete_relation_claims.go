package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateCompletionRelationClaims binds model-authored relation metadata to
// the same typed trace projection consumed by the final answer pipeline.
// Relation metadata is optional at investigation closure because the finalizer
// already receives the typed authority slate; requiring a format-only copy
// cannot validate model prose and caused provider-shape retry storms. Any claim
// the model does submit remains exact-validated. No request/reason prose
// participates.
func validateCompletionRelationClaims(ctx *types.BusContext, claims []types.AnswerRelationClaim) ([]types.AnswerRelationClaim, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	if err := types.ValidateAnswerRelationClaims(claims, authorities, false); err != nil {
		return nil, fmt.Errorf("model-authored relation claims do not match typed trace authority: %w; copy submitted relation_authority objects exactly or omit this optional metadata, and keep your own reason consistent", err)
	}
	out := types.CloneAnswerRelationClaims(claims)
	for i := range out {
		out[i] = types.NormalizeAnswerRelationClaim(out[i])
	}
	return out, nil
}
