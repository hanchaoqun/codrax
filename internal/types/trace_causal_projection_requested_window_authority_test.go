package types

import "testing"

func requestedWindowAuthorityRecord(id, predicate, subject, object, value string, notes ...string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact,
			Path: "/tmp/customer.systrace",
		},
		ClaimKey:  predicate + ":" + subject,
		Predicate: predicate,
		Subject:   subject,
		Object:    object,
		Value:     value,
		Unit:      "ms",
		RichNotes: notes,
	}
}

func requestedWindowAuthorityFixture(includeFullBoard bool) []ObservationRecord {
	const target = "ui-100"
	records := []ObservationRecord{
		requestedWindowAuthorityRecord(
			"frame-sub", "frame_target_resolution", target, "explicit_query_target", "",
			"window_source=query_window", "window=10.020000..10.070000",
		),
		requestedWindowAuthorityRecord(
			"path-sub", "wakeup_chain", target, "worker-sub-200 -> "+target, "",
			"branch=1", "selected_window=10.020000..10.070000",
		),
		requestedWindowAuthorityRecord(
			"rank-sub", "root_cause_primary", "worker-sub-200", "runnable", "2.000",
			"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=2.000",
			"selected_window=10.020000..10.070000",
		),
		requestedWindowAuthorityRecord(
			"state-sub", "target_window_states", target, "state_partition", "50.000",
			"selected_window=10.020000..10.070000", "running=10.000", "runnable=5.000",
			"sleep=35.000", "d_state=0.000", "io_wait=0.000", "total=50.000",
		),
		requestedWindowAuthorityRecord(
			"state-full", "target_window_states", target, "state_partition", "100.000",
			"selected_window=10.000000..10.100000", "running=20.000", "runnable=10.000",
			"sleep=70.000", "d_state=0.000", "io_wait=0.000", "total=100.000",
		),
	}
	if includeFullBoard {
		records = append(records,
			requestedWindowAuthorityRecord(
				"path-full", "wakeup_chain", target, "worker-full-300 -> "+target, "",
				"branch=1", "selected_window=10.000000..10.100000",
			),
			requestedWindowAuthorityRecord(
				"rank-full", "root_cause_primary", "worker-full-300", "runnable", "8.000",
				"rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=8.000",
				"selected_window=10.000000..10.100000",
			),
		)
	}
	return records
}

func requestedWindowAuthorityProfile() *RuntimeArtifactScopeProfile {
	start, end := 10.0, 10.1
	return &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.0s 到 10.1s",
	}
}

func TestTraceProjectionExplicitUserWindowOutranksInteriorFrameProbe(t *testing.T) {
	projection := CompileTraceCausalProjection(ObservationLedger{
		Records:                     requestedWindowAuthorityFixture(true),
		RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile(),
		AnchorUserEntities:          []AnchorUserEntity{{Value: "100", TypedLane: true}},
	})
	if projection.WindowStartTs != 10 || projection.WindowEndTs != 10.1 {
		t.Fatalf("explicit requested window lost to interior probe: %.6f..%.6f", projection.WindowStartTs, projection.WindowEndTs)
	}
	if projection.TargetStateAccount == nil || projection.TargetStateAccount.TotalMS != 100 {
		t.Fatalf("full requested-window target state did not attach: %+v", projection.TargetStateAccount)
	}
	if len(projection.WakeupPath) != 2 || projection.WakeupPath[0] != "worker-full-300" ||
		projection.WakeupPathQueryWindowStartTs != 10 || projection.WakeupPathQueryWindowEndTs != 10.1 {
		t.Fatalf("requested-window wakeup path was not elected: path=%v window=%.6f..%.6f",
			projection.WakeupPath, projection.WakeupPathQueryWindowStartTs, projection.WakeupPathQueryWindowEndTs)
	}
}

func TestTraceProjectionExplicitUserWindowRequiresExactTraceCoverage(t *testing.T) {
	projection := CompileTraceCausalProjection(ObservationLedger{
		Records:                     requestedWindowAuthorityFixture(false),
		RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile(),
		AnchorUserEntities:          []AnchorUserEntity{{Value: "100", TypedLane: true}},
	})
	if projection.WindowStartTs != 10.02 || projection.WindowEndTs != 10.07 {
		t.Fatalf("request intent without an exact causal carrier must not mint coverage: %.6f..%.6f", projection.WindowStartTs, projection.WindowEndTs)
	}
	if projection.TargetStateAccount == nil || projection.TargetStateAccount.TotalMS != 50 {
		t.Fatalf("legacy interior account should remain when requested window lacks causal coverage: %+v", projection.TargetStateAccount)
	}
}

func TestCompileObservationLedgerCarriesClonedRuntimeArtifactScope(t *testing.T) {
	profile := requestedWindowAuthorityProfile()
	ledger := CompileObservationLedger(ObservationLedgerInput{RequestModel: &RequestModel{RuntimeArtifactScopeProfile: profile}})
	if ledger.RuntimeArtifactScopeProfile == nil || ledger.RuntimeArtifactScopeProfile == profile {
		t.Fatalf("runtime artifact scope must be carried as an independent typed value: ledger=%p input=%p", ledger.RuntimeArtifactScopeProfile, profile)
	}
	*profile.TimeStart = 99
	if start, _, ok := ledger.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); !ok || start != 10 {
		t.Fatalf("ledger scope alias-mutated with request model: start=%v ok=%t", start, ok)
	}
}
