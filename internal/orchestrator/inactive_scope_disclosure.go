package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validateInactiveScopeDisclosure is the post-emit hard gate for
// multi-repo answers bounded by an active sub-repo subset. It
// consumes typed BusContext fields (PendingSubRepos, AnalysisIR.
// RequestModel) and the rendered AnswerDocumentV2; no prose scan,
// no LLM dispatch.
//
// Activation conditions and the disclosure obligation itself live
// in internal/types/inactive_scope_disclosure.go so the same predicate
// can be reused by future pre-emit / repair surfaces without
// duplicating activation logic.
func validateInactiveScopeDisclosure(doc *types.AnswerDocumentV2, busCtx *types.BusContext) []types.Violation {
	obligation := types.BuildInactiveScopeDisclosureObligationFromBus(busCtx, doc)
	if !obligation.Active() {
		return nil
	}
	if types.AnswerDocumentDisclosesInactiveScope(doc, obligation) {
		return nil
	}
	subject := obligation.RequestedSubject
	if subject == "" {
		subject = "the requested target"
	}
	pendingHint := strings.Join(obligation.PendingRootRels, ", ")
	if pendingHint == "" {
		pendingHint = "an inactive sub-repo"
	}
	return []types.Violation{{
		Kind: types.ViolInactiveScopeDisclosureMissing,
		Detail: fmt.Sprintf(
			"answer is bounded by the active sub-repo set (reason=%s) but does not disclose the inactive scope. The active set excluded: %s. The user-mentioned subject %q has no resolution in the active scope and may live in an inactive sub-repo.",
			obligation.Reason, pendingHint, subject,
		),
		Repair: fmt.Sprintf(
			"re-emit the answer so it either (a) names an inactive sub-repo by its RootRel (one of: %s) directly in a principal block / caveat, or (b) sets `scope_disclosure` on a block to inactive_scope_named / out_of_active_scope / requires_workspace_adjust. Without this disclosure the user cannot tell whether %q is truly absent or simply outside the current active scope.",
			pendingHint, subject,
		),
		Stage: string(types.StageFinalize),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.scope_disclosure",
			Reason:     "multi-repo bounded answer omitted inactive-scope disclosure",
			Confidence: 0.9,
		},
		ClusterKey: types.IdentityClusterKey(
			fmt.Sprintf("inactive_scope_disclosure:%s", strings.Join(obligation.PendingRootRels, ",")),
			"missing",
		),
	}}
}
