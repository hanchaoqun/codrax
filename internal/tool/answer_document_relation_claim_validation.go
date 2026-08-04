package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateModelAuthoredAnswerRelationClaims keeps the model in charge of the
// conclusion while ensuring any structured value relations it chooses to
// publish do not drift from typed trace authority. The investigation model has
// already acknowledged closure-critical relations before Finalize; requiring
// the finalizer to copy the same hidden metadata again adds no visible-answer
// proof and creates a format-only retry. Only submitted typed block metadata
// is checked here; visible block text is never inspected or rewritten.
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
	if len(submitted) == 0 {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	if err := types.ValidateAnswerRelationClaims(submitted, authorities, false); err != nil {
		return fmt.Errorf("model-authored answer relation_claims do not match typed trace authority: %w; carrier path: keep each submitted claim on blocks[i].relation_claims (not top-level $.relation_claims), or omit optional relation_claims metadata", err)
	}
	return nil
}
