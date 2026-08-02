package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateModelAuthoredAnswerRelationClaims keeps the model in charge of the
// conclusion while ensuring its structured value relations do not drift from
// the accepted investigation handoff. Only typed block metadata and typed
// trace authorities are read; visible block text is never inspected or
// rewritten.
func validateModelAuthoredAnswerRelationClaims(ctx *types.BusContext, doc *types.AnswerDocumentV2) error {
	if ctx == nil || ctx.Mutable == nil || doc == nil {
		return nil
	}
	var submitted []types.AnswerRelationClaim
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind != "" {
			continue
		}
		submitted = append(submitted, block.RelationClaims...)
	}
	accepted := ctx.Mutable.StableInvestigationRelationClaims()
	if len(submitted) == 0 && len(accepted) == 0 {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	if err := types.ValidateAnswerRelationClaims(submitted, authorities, false); err != nil {
		return fmt.Errorf("model-authored answer relation_claims do not match typed trace authority: %w", err)
	}
	if len(accepted) > 0 && !types.AnswerRelationClaimsEqual(submitted, accepted) {
		return fmt.Errorf("model-authored answer relation_claims must preserve every accepted investigation relation claim exactly; copy the accepted_model_relation_claims from Trace Decision Inputs onto the block(s) using those values, then revise your own prose if needed")
	}
	return nil
}
