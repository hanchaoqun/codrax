package types

import "testing"

func TestBuildHistoricalCurrentSourceAuthority_SeparatesTransitionFromExactCurrentDefinitions(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		{
			ID: "vcs:paths", Origin: AnswerEvidenceOriginVCSDiff,
			Predicate:    "changed_paths",
			SurfaceTerms: []string{"internal/agent/agent_test.go", "internal/tool/test_surface_test.go"},
		},
		{
			ID: "current:helper", Origin: AnswerEvidenceOriginCurrentSource,
			SourceRef:     ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/agent/agent_test.go"},
			Span:          ObservationSpan{LineStart: 1808, LineEnd: 1820},
			EvidenceScope: ScopeLineRange, GroundingStatus: GroundingGrounded,
			ClaimKey: "explicitRuntimeArtifactLog",
		},
		{
			ID: "current:unrelated", Origin: AnswerEvidenceOriginCurrentSource,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/types/context.go"},
			Span:      ObservationSpan{LineStart: 10}, EvidenceScope: ScopeLine,
			GroundingStatus: GroundingGrounded, ClaimKey: "Unrelated",
		},
	}}

	got := BuildHistoricalCurrentSourceAuthority(ledger)
	if !got.Active || got.TransitionStatus != HistoricalTransitionUnproven {
		t.Fatalf("authority=%+v, want active unproven transition", got)
	}
	if got.Reason != "no_typed_revision_mapping_or_behavioral_transition_witness" {
		t.Fatalf("reason=%q", got.Reason)
	}
	if got.CurrentDefinitionTotal != 1 || len(got.CurrentDefinitions) != 1 {
		t.Fatalf("definitions=%+v total=%d, want only changed-path definition", got.CurrentDefinitions, got.CurrentDefinitionTotal)
	}
	def := got.CurrentDefinitions[0]
	if def.Symbol != "explicitRuntimeArtifactLog" || def.Path != "internal/agent/agent_test.go" || !def.HistoricalPathMatch {
		t.Fatalf("definition=%+v", def)
	}
}

func TestBuildHistoricalCurrentSourceAuthority_RuntimeArtifactNeverMintsTransitionFromMatchingSymbol(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		{ID: "runtime", Origin: AnswerEvidenceOriginRuntimeArtifact, Subject: "RankGraph", Summary: "old build stack"},
		{
			ID: "current", Origin: AnswerEvidenceOriginCurrentSource,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/graph/rank.go"},
			Span:      ObservationSpan{LineStart: 42}, EvidenceScope: ScopeLine,
			GroundingStatus: GroundingRecovered, ClaimKey: "RankGraph",
		},
	}}
	got := BuildHistoricalCurrentSourceAuthority(ledger)
	if !got.Active || got.TransitionStatus != HistoricalTransitionUnproven {
		t.Fatalf("matching symbol/path must not prove artifact revision transition: %+v", got)
	}
}

func TestBuildHistoricalCurrentSourceAuthority_InactiveWithoutBothLanes(t *testing.T) {
	got := BuildHistoricalCurrentSourceAuthority(ObservationLedger{Records: []ObservationRecord{{
		ID: "runtime", Origin: AnswerEvidenceOriginRuntimeArtifact,
	}}})
	if got.Active || got.TransitionStatus != HistoricalTransitionNotApplicable {
		t.Fatalf("one-lane authority=%+v", got)
	}
}
