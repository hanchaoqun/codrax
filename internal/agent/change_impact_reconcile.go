package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const changeImpactHardReconcileConfidenceFloor = 0.75

// reconcileChangeImpactProfile aligns the analyzer's broad intent fields with
// an active typed change-impact lane. The profile is authored by the analyzer
// LLM; this helper only resolves contradictions between that typed lane and the
// coarser intent/question_kind/predicate fields when the analyzer marked that
// lane with enough confidence for a hard routing repair. Low-confidence
// profiles remain available to downstream prompt/support planners, but do not
// rewrite the user's answer shape.
func reconcileChangeImpactProfile(rm types.RequestModel) (types.RequestModel, string) {
	profile := rm.ChangeImpactProfile
	if profile == nil || !profile.Active() {
		return rm, ""
	}
	if profile.Confidence < changeImpactHardReconcileConfidenceFloor {
		return rm, ""
	}
	if !changeImpactOutputIsPrincipalSet(profile.RequestedOutput) {
		return rm, ""
	}
	before := changeImpactClassificationSummary(rm)
	rm.Intent = types.IntentEnumerate
	rm.AnalyzerHints.Kind = string(types.ReqEnumeration)
	rm.Predicates.IsCategoryEnumeration = true
	rm.Predicates.IsRelationalLookup = true
	rm.Predicates.IsCrossComponent = false
	rm.Predicates.IsScalarAnswer = false
	rm.Predicates.IsCountQuestion = false
	rm.Predicates.IsRoleLocateLookup = false
	after := changeImpactClassificationSummary(rm)
	if before == after {
		return rm, ""
	}
	return rm, fmt.Sprintf(
		"active change_impact_profile requested_output=%s requires set-valued affected-member routing (%s → %s)",
		profile.RequestedOutput,
		before,
		after,
	)
}

func changeImpactOutputIsPrincipalSet(out types.ImpactRequestedOutput) bool {
	switch out {
	case types.ImpactOutputFiles, types.ImpactOutputSites, types.ImpactOutputSymbols:
		return true
	default:
		return false
	}
}

func changeImpactClassificationSummary(rm types.RequestModel) string {
	return strings.Join([]string{
		"intent=" + string(rm.Intent),
		"kind=" + strings.TrimSpace(rm.AnalyzerHints.Kind),
		fmt.Sprintf("cat=%t", rm.Predicates.IsCategoryEnumeration),
		fmt.Sprintf("rel=%t", rm.Predicates.IsRelationalLookup),
		fmt.Sprintf("cross=%t", rm.Predicates.IsCrossComponent),
		fmt.Sprintf("scalar=%t", rm.Predicates.IsScalarAnswer),
	}, " ")
}
