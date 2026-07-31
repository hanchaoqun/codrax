package types

import (
	"sort"
	"strings"
)

// RuntimeArtifactScopeCoveragePredicate is the typed observation predicate
// emitted when a deterministic runtime query proves that its physical
// time/line scan covered the complete attached artifact. It says nothing about
// whether a filtered/capped result enumerated every matching relation; callers
// must consult the independent EnumerationAuthority for all/only/count claims.
const RuntimeArtifactScopeCoveragePredicate = "runtime_artifact_scope_coverage"

// RuntimeArtifactScopeCoverageSource names a producer lane that can prove
// physical full-artifact coverage. The model-query lane is minted only by the
// trace_query execution wrapper after checking its typed params and result
// metadata; the supplement lane is minted from SystemTraceSupplementMeta.
type RuntimeArtifactScopeCoverageSource string

const (
	RuntimeArtifactScopeCoverageModelQuery       RuntimeArtifactScopeCoverageSource = "model_unbounded_query"
	RuntimeArtifactScopeCoverageSystemSupplement RuntimeArtifactScopeCoverageSource = "system_supplement"
)

// RuntimeArtifactScopeCoverage is the unified consumer view for model-issued
// unbounded canonical queries and the deterministic system supplement.
// CoveredScope is a physical artifact boundary, not value/enumeration
// authority.
type RuntimeArtifactScopeCoverage struct {
	CoveredScope RuntimeArtifactRequestedScope
	Sources      []RuntimeArtifactScopeCoverageSource
	Views        []string
}

func (c RuntimeArtifactScopeCoverage) FullArtifact() bool {
	return c.CoveredScope == RuntimeArtifactScopeFullArtifact && len(c.Sources) > 0
}

// CompileRuntimeArtifactScopeCoverage consumes only precise typed carriers.
// Free-form summaries, model prose, query timestamps, and inferred min/max
// event bounds cannot mint full-artifact coverage.
func CompileRuntimeArtifactScopeCoverage(ledger ObservationLedger, supplement *SystemTraceSupplementMeta) RuntimeArtifactScopeCoverage {
	sourceSet := make(map[RuntimeArtifactScopeCoverageSource]bool)
	viewSet := make(map[string]bool)

	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
			record.GroundingPolicy != ClaimGroundingHard ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			strings.TrimSpace(record.Predicate) != RuntimeArtifactScopeCoveragePredicate ||
			strings.TrimSpace(record.Object) != string(RuntimeArtifactScopeFullArtifact) ||
			strings.TrimSpace(record.Scope) != string(RuntimeArtifactScopeFullArtifact) {
			continue
		}
		source := RuntimeArtifactScopeCoverageModelQuery
		if record.SystemSupplement {
			source = RuntimeArtifactScopeCoverageSystemSupplement
		}
		sourceSet[source] = true
		if view := strings.TrimSpace(record.Value); view != "" {
			viewSet[view] = true
		}
	}

	if supplement != nil &&
		supplement.RequestedArtifactScope == RuntimeArtifactScopeFullArtifact &&
		len(supplement.Views) > 0 {
		sourceSet[RuntimeArtifactScopeCoverageSystemSupplement] = true
		for _, raw := range supplement.Views {
			if view := strings.TrimSpace(raw); view != "" {
				viewSet[view] = true
			}
		}
	}

	if len(sourceSet) == 0 {
		return RuntimeArtifactScopeCoverage{}
	}
	sources := make([]RuntimeArtifactScopeCoverageSource, 0, len(sourceSet))
	for _, source := range []RuntimeArtifactScopeCoverageSource{
		RuntimeArtifactScopeCoverageModelQuery,
		RuntimeArtifactScopeCoverageSystemSupplement,
	} {
		if sourceSet[source] {
			sources = append(sources, source)
		}
	}
	views := make([]string, 0, len(viewSet))
	for view := range viewSet {
		views = append(views, view)
	}
	sort.Strings(views)
	return RuntimeArtifactScopeCoverage{
		CoveredScope: RuntimeArtifactScopeFullArtifact,
		Sources:      sources,
		Views:        views,
	}
}
