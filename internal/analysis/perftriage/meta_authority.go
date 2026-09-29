package perftriage

import "github.com/hanchaoqun/codrax/internal/types"

// perfMetaMergePartition prevents unrelated estimates from being laundered
// through a validator-owned part. Individual source bundles remain in the run
// audit. BugClasses and observation records are merged separately from all parts.
func perfMetaMergePartition(parts []*types.PerfBundle) ([]*types.PerfBundle, types.PerfObservationAuthority) {
	var verified []*types.PerfBundle
	for _, p := range parts {
		if p.HasAuthoritativeMeta() {
			verified = append(verified, p)
		}
	}
	if len(verified) > 0 {
		return verified, types.PerfObservationAuthorityDeterministicValidator
	}
	return parts, types.PerfObservationAuthorityPreTriageModelExtraction
}
