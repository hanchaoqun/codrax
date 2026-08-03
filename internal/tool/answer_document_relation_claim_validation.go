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
	if err := types.ValidateAnswerRelationClaims(submitted, authorities, true); err != nil {
		return fmt.Errorf("model-authored answer relation_claims do not match typed trace authority: %w; carrier path: copy each exact Trace Decision Inputs object to blocks[i].relation_claims on a model-authored block (not top-level $.relation_claims)", err)
	}
	if len(accepted) > 0 && !answerRelationClaimsContainAccepted(submitted, accepted) {
		return fmt.Errorf("model-authored answer relation_claims must preserve every accepted investigation relation claim exactly and also include any closure-critical typed_relation_authority added by deterministic supplement; copy those Trace Decision Inputs onto the block(s), then revise your own prose if needed")
	}
	return nil
}

func answerRelationClaimsContainAccepted(submitted, accepted []types.AnswerRelationClaim) bool {
	byID := make(map[string]types.AnswerRelationClaim, len(submitted))
	for _, claim := range submitted {
		normalized := types.NormalizeAnswerRelationClaim(claim)
		byID[normalized.AuthorityID] = normalized
	}
	selected := make([]types.AnswerRelationClaim, 0, len(accepted))
	for _, claim := range accepted {
		normalized := types.NormalizeAnswerRelationClaim(claim)
		matched, ok := byID[normalized.AuthorityID]
		if !ok {
			return false
		}
		selected = append(selected, matched)
	}
	return types.AnswerRelationClaimsEqual(selected, accepted)
}
