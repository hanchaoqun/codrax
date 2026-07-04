package tool

// trace_query_periodic_source_vs1_test.go — VS-1 (§7.8) observation-note pins:
// periodic-source rows publish typed periodic_source / detected_period_ms /
// lateness_ms / effective_impact_ms notes on the wakeup_causal_aggregate,
// wakeup_causal_impact and root_cause_rank records; non-periodic rows stay
// byte-identical. Numbers are the berlin vsync_cust live values.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func periodicVS1Result() tracequery.Result {
	return tracequery.Result{
		View:       "wakeup_chain",
		SourcePath: "/traces/berlin.systrace",
		TimeStart:  4520.100,
		TimeEnd:    4520.150,
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "main", PID: 4144},
			Window: tracequery.TimeWindow{StartTs: 4520.100, EndTs: 4520.150},
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Thread:                    tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 610},
				Window:                    tracequery.TimeWindow{StartTs: 4520.100, EndTs: 4520.106},
				ActualWindow:              tracequery.TimeWindow{StartTs: 4520.100, EndTs: 4520.106},
				ChainDepth:                1,
				OnChain:                   true,
				DominantState:             string(tracequery.StateSSleep),
				DominantImpactMs:          5.800,
				TotalMs:                   5.818,
				SleepMs:                   5.800,
				RunnableMs:                0.018,
				TargetBlockedMs:           5.818,
				LineStart:                 100,
				LineEnd:                   105,
				PeriodicSource:            true,
				DetectedPeriodMs:          8.302,
				LatenessMs:                0.000,
				EffectivePeriodicImpactMs: 0.018,
				Summary:                   "VSyncGenerator sleep occurrence",
			}},
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{
				{
					Thread:                    tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 610},
					ChainDepth:                1,
					OccurrenceCount:           6,
					DominantState:             string(tracequery.StateSSleep),
					DominantImpactMs:          36.256,
					ProjectedImpactMs:         36.256,
					TotalMs:                   36.361,
					SleepMs:                   36.256,
					RunnableMs:                0.105,
					LineStart:                 100,
					LineEnd:                   160,
					PeriodicSource:            true,
					DetectedPeriodMs:          8.302,
					LatenessMs:                0.071,
					EffectivePeriodicImpactMs: 0.176,
					Summary:                   "VSyncGenerator aggregated occurrences",
				},
				{
					Thread:            tracequery.ThreadRef{Comm: "worker", PID: 21},
					ChainDepth:        2,
					OccurrenceCount:   2,
					DominantState:     string(tracequery.StateRunnable),
					DominantImpactMs:  12.0,
					ProjectedImpactMs: 12.0,
					TotalMs:           13.0,
					RunnableMs:        12.0,
					LineStart:         300,
					LineEnd:           340,
					Summary:           "worker aggregated occurrences",
				},
			},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 4520.100, EndTs: 4520.150},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "primary", Type: "running",
					Thread:             tracequery.ThreadRef{Comm: "RSUniRenderThre", PID: 1963},
					ImpactMs:           4.115,
					CumulativeImpactMs: 4.115,
					Score:              7.25, Confidence: 0.86,
					LineStart: 300, LineEnd: 320, Source: "wakeup_chain.causal_impacts",
					Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
					DominantState: string(tracequery.StateRunning),
					Summary:       "RSUniRender running",
				},
				{
					Rank: 2, Tier: "secondary", Type: "sleep_wait",
					Thread:             tracequery.ThreadRef{Comm: "VSyncGenerator", PID: 610},
					ImpactMs:           36.256,
					CumulativeImpactMs: 36.361,
					EffectiveImpactMs:  0.176,
					PeriodicSource:     true,
					DetectedPeriodMs:   8.302,
					LatenessMs:         0.071,
					Score:              0.30, Confidence: 0.82,
					LineStart: 100, LineEnd: 160, Source: "wakeup_chain.aggregated_impacts",
					Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
					DominantState: string(tracequery.StateSSleep),
					Summary:       "VSyncGenerator periodic aggregate",
				},
			},
		},
	}
}

func periodicVS1FindRecord(t *testing.T, rows []types.ObservationRecord, predicate, subjectPart string) types.ObservationRecord {
	t.Helper()
	for _, row := range rows {
		if row.Predicate == predicate && strings.Contains(row.Subject, subjectPart) {
			return row
		}
	}
	t.Fatalf("no %s record for subject %q in %+v", predicate, subjectPart, rows)
	return types.ObservationRecord{}
}

func periodicVS1CountNote(notes []string, note string) int {
	count := 0
	for _, n := range notes {
		if n == note {
			count++
		}
	}
	return count
}

func TestTraceQueryPeriodicSourceTypedNotes(t *testing.T) {
	rows := traceQueryTypedObservations(periodicVS1Result(), "attached_trace", "/blobs/r.json", "", "", time.Now())

	agg := periodicVS1FindRecord(t, rows, "wakeup_causal_aggregate", "VSyncGenerator")
	for _, want := range []string{
		"periodic_source=true", "detected_period_ms=8.302", "lateness_ms=0.071", "effective_impact_ms=0.176",
	} {
		if periodicVS1CountNote(agg.RichNotes, want) != 1 {
			t.Fatalf("aggregate record must carry %q exactly once: %v", want, agg.RichNotes)
		}
	}

	impact := periodicVS1FindRecord(t, rows, "wakeup_causal_impact", "VSyncGenerator")
	for _, want := range []string{
		"periodic_source=true", "detected_period_ms=8.302",
		// the first occurrence's lateness/effective are legitimately ~0 and
		// MUST still publish — the zero is the load-bearing cadence fact.
		"lateness_ms=0.000", "effective_impact_ms=0.018",
	} {
		if periodicVS1CountNote(impact.RichNotes, want) != 1 {
			t.Fatalf("impact record must carry %q exactly once: %v", want, impact.RichNotes)
		}
	}

	rank := periodicVS1FindRecord(t, rows, "root_cause_secondary", "VSyncGenerator")
	for _, want := range []string{
		"periodic_source=true", "detected_period_ms=8.302", "lateness_ms=0.071",
	} {
		if periodicVS1CountNote(rank.RichNotes, want) != 1 {
			t.Fatalf("rank record must carry %q exactly once: %v", want, rank.RichNotes)
		}
	}
	// The discounted value rides the existing effective note — exactly once
	// (no duplicate from the periodic lane), and never the raw 36ms sleep.
	if periodicVS1CountNote(rank.RichNotes, "effective_impact_ms=0.176") != 1 {
		t.Fatalf("rank record must carry the discounted effective exactly once: %v", rank.RichNotes)
	}

	// Non-periodic rows carry NO periodic note (byte-stable lane).
	worker := periodicVS1FindRecord(t, rows, "wakeup_causal_aggregate", "worker")
	running := periodicVS1FindRecord(t, rows, "root_cause_primary", "RSUniRenderThre")
	for _, rec := range []types.ObservationRecord{worker, running} {
		for _, note := range rec.RichNotes {
			if strings.HasPrefix(note, "periodic_source=") || strings.HasPrefix(note, "detected_period_ms=") || strings.HasPrefix(note, "lateness_ms=") {
				t.Fatalf("non-periodic record must not carry periodic notes: %v", rec.RichNotes)
			}
		}
	}
}

// TestTraceQueryPeriodicSourceZeroEffectiveNoteSurvives pins the pure-cadence
// edge: a periodic rank row whose discounted attribution is exactly 0 still
// publishes effective_impact_ms=0.000 (the positive-value filter would have
// dropped it, silently reverting the display to the raw sleep).
func TestTraceQueryPeriodicSourceZeroEffectiveNoteSurvives(t *testing.T) {
	result := periodicVS1Result()
	result.RootCauseRank.Items[1].EffectiveImpactMs = 0
	result.RootCauseRank.Items[1].LatenessMs = 0
	rows := traceQueryTypedObservations(result, "attached_trace", "/blobs/r.json", "", "", time.Now())
	rank := periodicVS1FindRecord(t, rows, "root_cause_secondary", "VSyncGenerator")
	if periodicVS1CountNote(rank.RichNotes, "effective_impact_ms=0.000") != 1 {
		t.Fatalf("zero discounted attribution must publish explicitly: %v", rank.RichNotes)
	}
}
