package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runTypedAnswerExclusionPolicyCheck enforces user-stated candidate exclusions
// from typed carriers only: RequestModel.AnswerExclusionPolicy /
// AnswerVisibilityProfile and AnswerDocumentV2 item candidate_role
// annotations. It deliberately does not inspect the raw request or rendered
// answer prose.
func runTypedAnswerExclusionPolicyCheck(doc *types.AnswerDocumentV2, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	excludedRoles := typedAnswerExcludedRoles(rm)
	if len(excludedRoles) == 0 {
		return nil
	}
	confidence := typedAnswerExclusionConfidence(rm)
	var out []types.Violation
	for bi, block := range doc.Blocks {
		if block.SurfaceRole != types.SurfacePrincipal || len(block.Items) == 0 {
			continue
		}
		for ii, item := range block.Items {
			if !excludedRoles[item.CandidateRole] {
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
					Reason:     "principal answer row conflicts with analyzer-emitted typed exclusion / visibility scope",
					Confidence: confidence,
				},
			})
		}
	}
	return out
}

func typedAnswerExcludedRoles(rm *types.RequestModel) map[types.AnswerCandidateRole]bool {
	out := map[types.AnswerCandidateRole]bool{}
	if rm == nil {
		return out
	}
	if policy := rm.AnswerExclusionPolicy; policy != nil && policy.Active() {
		for _, role := range policy.ExcludedCandidateRoles {
			if role != types.AnswerCandidateRoleUnknown {
				out[role] = true
			}
		}
	}
	if rm.AnswerVisibilityProfile != nil && rm.AnswerVisibilityProfile.ExcludesPrivateSymbols() {
		out[types.AnswerCandidateRolePrivate] = true
	}
	return out
}

func typedAnswerExclusionConfidence(rm *types.RequestModel) float64 {
	if rm == nil {
		return 0
	}
	confidence := 0.0
	if policy := rm.AnswerExclusionPolicy; policy != nil && policy.Active() && policy.Confidence > confidence {
		confidence = policy.Confidence
	}
	if profile := rm.AnswerVisibilityProfile; profile != nil && profile.Active() && profile.Confidence > confidence {
		confidence = profile.Confidence
	}
	return confidence
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

// runTypedErrorGranularityProfileCheck enforces failure-scope verdict
// obligations from typed carriers only: RequestModel.ErrorGranularityProfile
// and AnswerDocumentV2 decision-block error_granularity_verdict annotations.
// It deliberately does not inspect RawRequest or rendered answer prose.
func runTypedErrorGranularityProfileCheck(doc *types.AnswerDocumentV2, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil || rm.ErrorGranularityProfile == nil || !rm.ErrorGranularityProfile.Active() {
		return nil
	}
	if !types.ShouldCarryErrorGranularityHardContract(*rm) {
		return nil
	}
	if !types.MissingErrorGranularityVerdict(doc, rm.ErrorGranularityProfile) {
		if verdict, mismatch := types.ErrorGranularityVerdictOptionMismatch(doc, rm.ErrorGranularityProfile); mismatch {
			options := make([]string, 0, len(rm.ErrorGranularityProfile.RequestedVerdictOptions))
			for _, option := range rm.ErrorGranularityProfile.RequestedVerdictOptions {
				options = append(options, string(option))
			}
			options = append(options, string(types.ErrorGranularityNotEnoughEvidence))
			return []types.Violation{{
				Kind:       types.ViolMustInclude,
				Detail:     fmt.Sprintf("principal decision block has error_granularity_verdict=%q outside requested verdict option(s): %s", verdict, strings.Join(options, ", ")),
				Repair:     "Replace error_granularity_verdict with the most specific requested verdict enum supported by the cited evidence, or use not_enough_evidence when the evidence cannot decide between the requested alternatives.",
				Stage:      string(types.StageFinalize),
				ClusterKey: fmt.Sprintf("typed_error_granularity:option_mismatch:%s", verdict),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_document.blocks[].error_granularity_verdict",
					Reason:     "principal decision block verdict does not match the typed request verdict options",
					Confidence: rm.ErrorGranularityProfile.Confidence,
				},
			}}
		}
		return nil
	}
	return []types.Violation{{
		Kind:       types.ViolMustInclude,
		Detail:     "principal decision block is missing error_granularity_verdict",
		Repair:     "Add a principal decision block with error_granularity_verdict set to the canonical failure-scope enum supported by the cited evidence. Prose-only wording does not satisfy this typed verdict contract.",
		Stage:      string(types.StageFinalize),
		ClusterKey: "typed_error_granularity:missing_verdict",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks[].error_granularity_verdict",
			Reason:     "principal decision block does not satisfy the typed failure-scope profile",
			Confidence: rm.ErrorGranularityProfile.Confidence,
		},
	}}
}
