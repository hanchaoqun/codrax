package types

import (
	"reflect"
	"testing"
)

func fullArtifactCoverageRecord() ObservationRecord {
	return ObservationRecord{
		ID:              "trace_query:artifact#runtime_artifact_scope_coverage",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query:run2",
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind:         ObservationSourceRuntimeArtifact,
			ArtifactID:   "runtime_artifact:abc",
			ArtifactKind: "trace",
		},
		Predicate: RuntimeArtifactScopeCoveragePredicate,
		Object:    string(RuntimeArtifactScopeFullArtifact),
		Value:     "thread_timeline",
		Scope:     string(RuntimeArtifactScopeFullArtifact),
	}
}

func TestCompileRuntimeArtifactScopeCoverageUnifiesQueryAndSupplement(t *testing.T) {
	got := CompileRuntimeArtifactScopeCoverage(
		ObservationLedger{Records: []ObservationRecord{fullArtifactCoverageRecord()}},
		&SystemTraceSupplementMeta{
			RequestedArtifactScope: RuntimeArtifactScopeFullArtifact,
			Views:                  []string{"window_stats"},
		},
	)
	if !got.FullArtifact() {
		t.Fatalf("expected full artifact coverage: %+v", got)
	}
	if want := []RuntimeArtifactScopeCoverageSource{
		RuntimeArtifactScopeCoverageModelQuery,
		RuntimeArtifactScopeCoverageSystemSupplement,
	}; !reflect.DeepEqual(got.Sources, want) {
		t.Fatalf("sources=%v, want %v", got.Sources, want)
	}
	if want := []string{"thread_timeline", "window_stats"}; !reflect.DeepEqual(got.Views, want) {
		t.Fatalf("views=%v, want %v", got.Views, want)
	}
}

func TestCompileRuntimeArtifactScopeCoverageKeepsSupplementObservationOffModelLane(t *testing.T) {
	record := fullArtifactCoverageRecord()
	record.SystemSupplement = true
	got := CompileRuntimeArtifactScopeCoverage(ObservationLedger{Records: []ObservationRecord{record}}, nil)
	if want := []RuntimeArtifactScopeCoverageSource{RuntimeArtifactScopeCoverageSystemSupplement}; !reflect.DeepEqual(got.Sources, want) {
		t.Fatalf("sources=%v, want %v", got.Sources, want)
	}
}

func TestCompileRuntimeArtifactScopeCoverageRejectsUntypedOrBoundedClaims(t *testing.T) {
	base := fullArtifactCoverageRecord()
	tests := []struct {
		name   string
		mutate func(*ObservationRecord)
	}{
		{"model prose producer", func(r *ObservationRecord) { r.Producer = "explorer" }},
		{"soft grounding", func(r *ObservationRecord) { r.GroundingPolicy = ClaimGroundingSoft }},
		{"wrong source", func(r *ObservationRecord) { r.SourceRef.Kind = ObservationSourceModelClaim }},
		{"bounded object", func(r *ObservationRecord) { r.Object = string(RuntimeArtifactScopeExplicitWindow) }},
		{"bounded scope", func(r *ObservationRecord) { r.Scope = string(RuntimeArtifactScopeExplicitWindow) }},
		{"wrong predicate", func(r *ObservationRecord) { r.Predicate = "window_stats" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := base
			tc.mutate(&record)
			if got := CompileRuntimeArtifactScopeCoverage(ObservationLedger{Records: []ObservationRecord{record}}, nil); got.FullArtifact() {
				t.Fatalf("invalid record minted full coverage: %+v", got)
			}
		})
	}

	if got := CompileRuntimeArtifactScopeCoverage(nilLedger(), &SystemTraceSupplementMeta{
		RequestedArtifactScope: RuntimeArtifactScopeFullArtifact,
	}); got.FullArtifact() {
		t.Fatalf("supplement without executed views minted full coverage: %+v", got)
	}
}

func nilLedger() ObservationLedger {
	return ObservationLedger{}
}
