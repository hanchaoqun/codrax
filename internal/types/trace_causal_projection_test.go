package types

import (
	"fmt"
	"strconv"
	"testing"
)

func TestTraceCausalProjectionPrefersAttributableChainRootCause(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRoot("root-app", "app-100", "compute_supply", "0.020", 0.02, 0.90, 1),
		traceProjectionTestRoot("root-pressure", "unknown-thread", "io_pressure", "16.000", 16.0, 0.95, 1),
		traceProjectionTestRoot("root-threadpool", "threadpool-400", "io_wait", "11.000", 11.0, 0.86, 4),
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "threadpool-400 -> network-300 -> cookie-200 -> app-100",
		},
		{
			ID:              "hop",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         "threadpool-400",
			Object:          "io_wait",
			Value:           "11.000",
			Unit:            "ms",
			RichNotes:       []string{"causality=on_wakeup_chain", "chain_depth=1", "impact=11.000ms"},
		},
		{
			ID:              "hop-duplicate",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         "threadpool-400",
			Object:          "io_wait",
			Value:           "11.000",
			Unit:            "ms",
			RichNotes:       []string{"causality=on_wakeup_chain", "chain_depth=1", "impact=11.000ms"},
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if got.PrimaryRootCause == nil {
		t.Fatal("expected a primary root cause")
	}
	if got.PrimaryRootCause.Subject != "threadpool-400" ||
		got.PrimaryRootCause.Object != "io_wait" ||
		got.PrimaryRootCause.CumulativeImpactMS != 11.0 {
		t.Fatalf("primary root cause should prefer attributable wakeup-chain nodes over aggregate sentinels, got %+v", got.PrimaryRootCause)
	}
	if len(got.WakeupPath) != 4 || got.WakeupPath[0] != "threadpool-400" || got.WakeupPath[3] != "app-100" {
		t.Fatalf("wakeup path not projected: %+v", got.WakeupPath)
	}
	if len(got.SupportingHops) != 1 || got.SupportingHops[0].Subject != "threadpool-400" {
		t.Fatalf("supporting causal hops should be projected once: %+v", got.SupportingHops)
	}
}

func traceProjectionTestRoot(id, subject, object, value string, cumulative, confidence float64, rank int) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Subject:         subject,
		Object:          object,
		Value:           value,
		Unit:            "ms",
		RichNotes: []string{
			"rank=" + strconv.Itoa(rank),
			"tier=primary",
			"impact_ms=" + value,
			"cumulative_impact_ms=" + fmt.Sprintf("%.3f", cumulative),
			"causality=on_wakeup_chain",
		},
		Confidence: confidence,
	}
}
