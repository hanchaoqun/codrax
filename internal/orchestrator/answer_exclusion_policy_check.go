package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runTypedAnswerExclusionPolicyCheck enforces user-stated candidate exclusions
// from typed carriers only: RequestModel.AnswerExclusionPolicy and
// AnswerDocumentV2 item candidate_role annotations. It deliberately does not
// inspect the raw request or rendered answer prose.
func runTypedAnswerExclusionPolicyCheck(doc *types.AnswerDocumentV2, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil || rm.AnswerExclusionPolicy == nil || !rm.AnswerExclusionPolicy.Active() {
		return nil
	}
	policy := rm.AnswerExclusionPolicy
	var out []types.Violation
	for bi, block := range doc.Blocks {
		if block.SurfaceRole != types.SurfacePrincipal || len(block.Items) == 0 {
			continue
		}
		for ii, item := range block.Items {
			if !policy.ExcludesRole(item.CandidateRole) {
				continue
			}
			itemID := item.ID
			if itemID == "" {
				itemID = fmt.Sprintf("%d", ii)
			}
			role := string(item.CandidateRole)
			fieldPath := fmt.Sprintf("answer_document.blocks[%d].items[%d].candidate_role", bi, ii)
			out = append(out, types.Violation{
				Kind:       types.ViolMustExclude,
				Detail:     fmt.Sprintf("principal answer item %q in block %q has excluded candidate_role=%q", itemID, block.ID, role),
				Repair:     fmt.Sprintf("Remove this %s candidate from the principal answer rows, or correct %s only if the row was mislabeled. Scope-boundary caveats may mention the excluded category without listing concrete excluded candidates.", role, fieldPath),
				Stage:      string(types.StageFinalize),
				ClusterKey: fmt.Sprintf("typed_answer_exclusion:%s:%s:%s", role, block.ID, itemID),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    fieldPath,
					Reason:     "principal answer row conflicts with analyzer-emitted answer_exclusion_policy",
					Confidence: policy.Confidence,
				},
			})
		}
	}
	return out
}
