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

func TestTraceCausalProjectionPreservesMultiLayerChainRelevance(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-on-chain", "binder-100", "runnable_delay", "9.000", 9.0, 0.93, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
		}),
		traceProjectionTestRootWithNotes("root-adjacent", "io-200", "io_wait", "7.000", 7.0, 0.90, 2, []string{
			"chain_relevance=adjacent",
			"causality=adjacent_to_wakeup_chain",
		}),
		{
			ID:              "root-background",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "root_cause_background",
			ClaimKey:        "root_cause_background",
			Subject:         "disk-300",
			Object:          "background_io_pressure",
			Value:           "18.000",
			Unit:            "ms",
			RichNotes: []string{
				"rank=3",
				"tier=supporting",
				"chain_relevance=background",
				"causality=background",
				"impact_ms=18.000",
				"cumulative_impact_ms=18.000",
			},
			Confidence: 0.88,
		},
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "binder-100 -> target-400",
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if got.PrimaryRootCause == nil || got.PrimaryRootCause.Subject != "binder-100" {
		t.Fatalf("expected on-chain primary to remain first, got %+v", got.PrimaryRootCause)
	}
	if len(got.PrimaryRootCauses) != 2 {
		t.Fatalf("expected both primary root-cause layers to be retained, got %+v", got.PrimaryRootCauses)
	}
	if got.PrimaryRootCauses[0].ChainRelevance != "on_chain" ||
		got.PrimaryRootCauses[1].ChainRelevance != "adjacent" {
		t.Fatalf("primary layers should preserve typed chain relevance: %+v", got.PrimaryRootCauses)
	}
	if len(got.OnChainCauses) != 1 || got.OnChainCauses[0].Subject != "binder-100" {
		t.Fatalf("on-chain cause projection missing: %+v", got.OnChainCauses)
	}
	if len(got.AdjacentCauses) != 1 || got.AdjacentCauses[0].Subject != "io-200" {
		t.Fatalf("adjacent cause projection missing: %+v", got.AdjacentCauses)
	}
	if len(got.BackgroundCauses) != 1 || got.BackgroundCauses[0].Subject != "disk-300" {
		t.Fatalf("background cause projection missing: %+v", got.BackgroundCauses)
	}
}

func TestTraceCausalProjectionPreservesMultiHopPathWithRunningWaker(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-io", "threadpool-400", "io_burst_episode", "119.000", 119.0, 0.88, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}),
		traceProjectionTestRootWithNotes("root-running", "worker-200", "running", "118.000", 118.0, 0.86, 2, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=2",
		}),
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "threadpool-400 -> worker-200 -> app-100",
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if len(got.PrimaryRootCauses) != 2 {
		t.Fatalf("projection should preserve co-primary on-chain layers, got %+v", got.PrimaryRootCauses)
	}
	if got.PrimaryRootCauses[0].Subject != "threadpool-400" || got.PrimaryRootCauses[1].Subject != "worker-200" {
		t.Fatalf("projection should keep ordered on-chain root layers, got %+v", got.PrimaryRootCauses)
	}
	if len(got.WakeupPath) != 3 || got.WakeupPath[0] != "threadpool-400" || got.WakeupPath[1] != "worker-200" || got.WakeupPath[2] != "app-100" {
		t.Fatalf("multi-hop wakeup path must be preserved for answer rendering, got %+v", got.WakeupPath)
	}
}

func traceProjectionTestRoot(id, subject, object, value string, cumulative, confidence float64, rank int) ObservationRecord {
	return traceProjectionTestRootWithNotes(id, subject, object, value, cumulative, confidence, rank, []string{
		"causality=on_wakeup_chain",
	})
}

func traceProjectionTestRootWithNotes(id, subject, object, value string, cumulative, confidence float64, rank int, extraNotes []string) ObservationRecord {
	notes := []string{
		"rank=" + strconv.Itoa(rank),
		"tier=primary",
		"impact_ms=" + value,
		"cumulative_impact_ms=" + fmt.Sprintf("%.3f", cumulative),
	}
	notes = append(notes, extraNotes...)
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
		RichNotes:       notes,
		Confidence:      confidence,
	}
}
