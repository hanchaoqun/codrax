package orchestrator

import (
	"fmt"
	"strings"

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

// runTypedAnswerRoleProfileCheck enforces positive answer-role bindings from
// typed carriers only: RequestModel.AnswerRoleProfile and AnswerDocumentV2
// item candidate_role annotations. It deliberately does not inspect RawRequest
// or rendered answer prose.
func runTypedAnswerRoleProfileCheck(doc *types.AnswerDocumentV2, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil || rm.AnswerRoleProfile == nil || !rm.AnswerRoleProfile.Active() {
		return nil
	}
	missing := types.MissingRequiredCandidateRoles(doc, rm.AnswerRoleProfile.RequiredCandidateRoles)
	if len(missing) == 0 {
		return nil
	}
	roles := make([]string, 0, len(missing))
	for _, role := range missing {
		roles = append(roles, string(role))
	}
	return []types.Violation{{
		Kind:   types.ViolMustInclude,
		Detail: fmt.Sprintf("principal answer rows are missing required candidate_role value(s): %s", strings.Join(roles, ", ")),
		Repair: "Add principal scalar/list/table item(s) with items[].candidate_role set to the required answer role enum(s), or correct the row roles if the principal answer was mislabeled. Prose-only wording does not satisfy this typed role-binding contract.",
		Stage:  string(types.StageFinalize),
		ClusterKey: fmt.Sprintf(
			"typed_answer_role:%s",
			strings.Join(roles, ","),
		),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks[].items[].candidate_role",
			Reason:     "principal answer rows do not satisfy the typed request role profile",
			Confidence: rm.AnswerRoleProfile.Confidence,
		},
	}}
}
