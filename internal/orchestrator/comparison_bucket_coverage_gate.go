package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// comparisonBucketSectionCoverageViolation reports whether v is a typed
// finalizer-surface defect that loses the user's explicit comparison
// partition. The signal is intentionally narrow:
//   - RequestModel must resolve to QFComparison from typed bucket structure.
//   - The violation must be the required section block carrier, not a generic
//     facet/metadata advisory.
//
// This keeps broad block/facet telemetry soft by default while making the
// user-visible "one principal section per bucket" contract complete for
// comparison answers.
func comparisonBucketSectionCoverageViolation(v types.Violation, bus *types.BusContext) bool {
	if v.Kind != types.ViolBlockCoverageMissing || !comparisonQuestionRequiresBucketSections(bus) {
		return false
	}
	if v.MissingBlockKind == types.BlockSection {
		return true
	}
	return v.ClusterKey == types.BlockKindClusterKey(types.BlockSection, "answer_block_coverage")
}

func comparisonQuestionRequiresBucketSections(bus *types.BusContext) bool {
	if bus == nil || bus.AnalysisIR == nil {
		return false
	}
	rm := bus.AnalysisIR.RequestModel
	if types.ResolveQuestionFamily(rm) != types.QFComparison {
		return false
	}
	return len(rm.QuestionStructure().Buckets) >= 2
}

func isStrictViolationForBus(v types.Violation, bus *types.BusContext) bool {
	if !isSoftViolationKind(v.Kind) {
		return true
	}
	return comparisonBucketSectionCoverageViolation(v, bus)
}

func hasAnyStrictViolationForBus(vs []types.Violation, bus *types.BusContext) bool {
	for _, v := range vs {
		if isStrictViolationForBus(v, bus) {
			return true
		}
	}
	return false
}
