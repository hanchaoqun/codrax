package types

import (
	"fmt"
	"testing"
)

func requestedWindowRankSurvivorRecord(id string, start, end float64) ObservationRecord {
	record := requestedWindowAuthorityRecord(
		id, "root_cause_primary", "worker-200", "priority_inversion_candidate", "8.300",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"effective_impact_ms=8.300", "cumulative_impact_ms=9.000",
		"rank_board_target=app-100", "rank_board_params_fingerprint=rank-board",
	)
	record.RichNotes = append(record.RichNotes,
		"selected_window="+formatTraceCausalProjectionTestWindow(start, end))
	record.Span = ObservationSpan{LineStart: 4, LineEnd: 8, StartTs: 1.000, EndTs: 1.010}
	return record
}

func formatTraceCausalProjectionTestWindow(start, end float64) string {
	return fmt.Sprintf("%.6f..%.6f", start, end)
}

func TestTraceProjectionExplicitWindowRankFactSurvivesEarlierExpandedTwin(t *testing.T) {
	start, end := 1.000, 1.010
	profile := &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "1.000s 到 1.010s",
	}
	records := []ObservationRecord{
		// This model-exploration row is intentionally first and only 20us wider.
		// It is inside the broad F-2 grouping tolerance but outside the principal
		// value tolerance, matching the r651 production witness.
		requestedWindowRankSurvivorRecord("model-expanded", 1.000, 1.010020),
		requestedWindowAuthorityRecord(
			"path-exact", "wakeup_chain", "app-100", "worker-200 -> app-100", "",
			"branch=1", "selected_window=1.000000..1.010000",
		),
		requestedWindowAuthorityRecord(
			"state-exact", "target_window_states", "app-100", "state_partition", "10.000",
			"selected_window=1.000000..1.010000", "running=0.000", "runnable=0.000",
			"sleep=10.000", "d_state=0.000", "io_wait=0.000", "total=10.000",
		),
		requestedWindowRankSurvivorRecord("supplement-exact", 1.000, 1.010000),
	}
	projection := CompileTraceCausalProjection(ObservationLedger{
		Records:                     records,
		RuntimeArtifactScopeProfile: profile,
		AnchorUserEntities:          []AnchorUserEntity{{Value: "app-100", TypedLane: true}},
	})
	if projection.WindowStartTs != start || projection.WindowEndTs != end {
		t.Fatalf("explicit requested window was not elected: %.6f..%.6f", projection.WindowStartTs, projection.WindowEndTs)
	}
	seats := TraceAnswerDecisionEliminableSeats(projection, 8)
	if len(seats) != 1 {
		t.Fatalf("exact-window ranked fact was lost or duplicated after same-fact folding: seats=%+v projection=%+v", seats, projection)
	}
	seat := seats[0]
	if seat.EvidenceID != "supplement-exact" ||
		!TraceCausalProjectionNodeMatchesPrincipalWindow(seat, start, end) ||
		seat.Rank != 1 || seat.Subject != "worker-200" || seat.EffectiveImpactMS != 8.300 {
		t.Fatalf("principal seat did not retain the exact-window typed donor: %+v", seat)
	}
}
