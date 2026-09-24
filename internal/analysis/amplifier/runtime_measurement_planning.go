package amplifier

import "github.com/hanchaoqun/codrax/internal/types"

// PlanningFacts contains only in-process material facts, not model judgments.
// Zero values retain legacy planning. No field proves an answer or grants an
// early completion: the ordinary evidence and completion checks still apply.
type PlanningFacts struct {
	SinglePhysicalRuntimeArtifact bool
}

// Runtime event names often share a transport prefix, not independent user
// questions. In one validated finite measurement scope they remain search
// hints. Explicit topics, partitions and answer dimensions are never removed.
func runtimeMeasurementsShareInvestigation(rm types.RequestModel, facts PlanningFacts) bool {
	if !facts.SinglePhysicalRuntimeArtifact || len(rm.SubTopics) != 0 || len(rm.Buckets) != 0 ||
		rm.CurrentSourceLaneDecision().RequiresCurrentSource() ||
		rm.Predicates.IsDiagnosticQuestion || rm.Predicates.IsRelationalLookup || rm.Predicates.IsHistoryLookup ||
		rm.DiagnosticProfile.IsDiagnostic || rm.DiagnosticProfile.CurrentRisk ||
		rm.DiagnosticProfile.HistoricalRegression || rm.DiagnosticProfile.CurrentVersionCheck {
		return false
	}
	profile := rm.RuntimeQuestionProfile
	if !profile.BoundedFactSet() || profile.RequestsRuntimeWorkRelation() || profile.RequestsFrameCausality() || len(profile.FactFamilies) == 0 {
		return false
	}
	for _, family := range profile.FactFamilies {
		switch family {
		case types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences,
			types.RuntimeQuestionFactOccurrenceTime, types.RuntimeQuestionFactCountOrDuration,
			types.RuntimeQuestionFactIOLatency, types.RuntimeQuestionFactResourcePressure,
			types.RuntimeQuestionFactFrequencyResidency, types.RuntimeQuestionFactOtherObservedValue:
		default:
			return false
		}
	}
	if dimensions := rm.RequestedAnswerDimensions; dimensions != nil {
		for _, dimension := range dimensions.Dimensions {
			if !dimension.Required {
				continue
			}
			switch dimension.Role {
			case types.RequestedAnswerDimensionComparisonAxis, types.RequestedAnswerDimensionRelationPath,
				types.RequestedAnswerDimensionRuntimeWorkRelation, types.RequestedAnswerDimensionTargetEffectVerdict,
				types.RequestedAnswerDimensionCausalAttribution, types.RequestedAnswerDimensionCausalContributorSet:
				return false
			}
		}
	}
	// Missing, selector-only, invalid or multiple windows do not establish one
	// measurement population. Full-artifact scope is valid for the single source.
	scope := rm.RuntimeArtifactScopeProfile
	_, _, singleWindow := scope.ExplicitTimeWindow()
	fullArtifact := scope.FullArtifact() && scope.TimeStart == nil && scope.TimeEnd == nil && len(scope.TimeWindows) == 0
	return fullArtifact || singleWindow
}
