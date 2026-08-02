package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateCompletionRelationClaims binds model-authored relation metadata to
// the same typed trace projection consumed by the final answer pipeline. A
// two-ruler accounting is closure-critical: every exact same-ruler subtotal
// and its cross-ruler prohibition must be acknowledged before the model may
// close. No request/reason prose participates.
func validateCompletionRelationClaims(ctx *types.BusContext, claims []types.AnswerRelationClaim) ([]types.AnswerRelationClaim, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	if err := types.ValidateAnswerRelationClaims(claims, authorities, true); err != nil {
		return nil, fmt.Errorf("model-authored relation claims do not match typed trace authority: %w; copy the required relation_authority objects exactly and revise your own reason to obey them", err)
	}
	out := types.CloneAnswerRelationClaims(claims)
	for i := range out {
		out[i] = types.NormalizeAnswerRelationClaim(out[i])
	}
	return out, nil
}
