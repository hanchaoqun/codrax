package tool

// trace_query_gated_note_p0e_test.go — §20 E-Gap⑤ pin (P0-E engine half,
// 2026-07-07): the root_cause_rank observation row publishes the R5d gated
// TOTAL under the registered priority_inversion_gated note key (single
// source: the full-precision sum of the two typed components, formatted once
// — no round3(a)+round3(b) re-add), and the wakeup_causal_aggregate face
// publishes its cross-occurrence gated caliber (§20 E-Gap②) under the same
// keys as the per-occurrence face. Projection-side parsing is deferred to the
// P0-A batch — this pin covers the engine-side publication only.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestRankRowPublishesGatedTotalNote(t *testing.T) {
	result := tracequery.Result{
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 10.0, EndTs: 10.0821},
			Items: []tracequery.RootCauseRankItem{{
				Rank:                  1,
				Tier:                  "primary",
				Type:                  "priority_inversion_candidate",
				Thread:                tracequery.ThreadRef{Comm: "dep", PID: 200},
				ImpactMs:              37.410,
				EffectiveImpactMs:     37.410,
				CumulativeImpactMs:    58.919,
				GatedRunnableMs:       20.713,
				GatedRunningDeficitMs: 16.697,
				Confidence:            0.91,
				LineStart:             100,
				LineEnd:               180,
				Summary:               "dep inversion candidate",
			}},
		},
	}
	records := traceQueryTypedObservations(result, "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	var rankNotes []string
	for _, record := range records {
		if strings.Contains(record.ID, "#root_cause_rank:1") {
			rankNotes = record.RichNotes
		}
	}
	if rankNotes == nil {
		t.Fatalf("rank observation record missing: %+v", records)
	}
	joined := strings.Join(rankNotes, "\n")
	if !strings.Contains(joined, "priority_inversion_gated=37.410") {
		t.Fatalf("rank row must publish the gated TOTAL note (E-Gap⑤), got notes:\n%s", joined)
	}
	if !strings.Contains(joined, "gated_runnable=20.713") || !strings.Contains(joined, "gated_running_deficit=16.697") {
		t.Fatalf("gated composition notes must ride alongside the total, got notes:\n%s", joined)
	}
}

func TestCausalAggregateFacePublishesGatedNotes(t *testing.T) {
	notes := traceQueryTypedCausalAggregateRichNotes(tracequery.WakeupCausalAggregate{
		Thread:                   tracequery.ThreadRef{Comm: "dep", PID: 300},
		ChainDepth:               1,
		OccurrenceCount:          2,
		DominantState:            "runnable",
		DominantImpactMs:         5,
		TotalMs:                  8,
		PriorityInversion:        true,
		PriorityInversionGatedMs: 6.0,
		GatedRunnableMs:          5.0,
		GatedRunningDeficitMs:    1.0,
		GatedAggregationCaliber:  tracequery.GatedCaliberSumDisjointOccurrences,
	})
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "priority_inversion_candidate=true") {
		t.Fatalf("inversion-TYPED aggregate must carry the candidate note, got:\n%s", joined)
	}
	if !strings.Contains(joined, "priority_inversion_gated=6.000") {
		t.Fatalf("aggregate face must publish the gated total (E-Gap②), got:\n%s", joined)
	}
	if !strings.Contains(joined, "gated_runnable=5.000") || !strings.Contains(joined, "gated_running_deficit=1.000") {
		t.Fatalf("aggregate face must publish the gated composition, got:\n%s", joined)
	}
	// F3 (§20.2 absorption): the aggregation caliber is disclosed typed so
	// P0-A can parse which ruler produced the total.
	if !strings.Contains(joined, "gated_aggregation_caliber=sum_disjoint_occurrences") {
		t.Fatalf("F3: the aggregation caliber note must be published, got:\n%s", joined)
	}
}

// F2 pin (§20.2 absorption, 2026-07-07): a sleep-dominant aggregate with one
// inversion member (raw invCount>0 flag set) is NOT inversion-typed on the
// rank face — the note face must not light the inversion label nor the gated
// composition prose while its bar shows the raw dominant value (the
// contradiction row). Gate = the SAME typed determination the rank face uses
// (WakeupCausalAggregateInversionTyped), never the raw flag.
func TestSleepDominantAggregateWithInversionMemberStaysUnlabelled(t *testing.T) {
	notes := traceQueryTypedCausalAggregateRichNotes(tracequery.WakeupCausalAggregate{
		Thread:                   tracequery.ThreadRef{Comm: "dep", PID: 300},
		ChainDepth:               1,
		OccurrenceCount:          2,
		DominantState:            "s_sleep",
		DominantImpactMs:         40,
		SleepMs:                  40,
		RunnableMs:               2,
		TotalMs:                  42,
		PriorityInversion:        true, // raw flag: one member was a candidate
		PriorityInversionGatedMs: 2.0,
		GatedRunnableMs:          2.0,
		GatedAggregationCaliber:  tracequery.GatedCaliberSumDisjointOccurrences,
	})
	joined := strings.Join(notes, "\n")
	if strings.Contains(joined, "priority_inversion_candidate=true") {
		t.Fatalf("F2: sleep-dominant aggregate must not light the inversion label off the raw flag, got:\n%s", joined)
	}
	if strings.Contains(joined, "priority_inversion_gated=") || strings.Contains(joined, "gated_runnable=") || strings.Contains(joined, "gated_aggregation_caliber=") {
		t.Fatalf("F2: gated composition notes must stay gated to the rank-face typed determination, got:\n%s", joined)
	}
}
