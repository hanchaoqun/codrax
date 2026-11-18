package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPublishesStateAccountKeyOnBothTypedRows(t *testing.T) {
	const key = "state_account:v1:typed-wire-pin"
	result := tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "/traces/app.systrace",
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			StateChurn: []tracequery.ThreadStateChurnSummary{{
				Thread: tracequery.ThreadRef{Comm: "app", PID: 20}, StateAccountKey: key,
				DominantState: string(tracequery.StateRunnable), DominantImpactMs: 5,
				TotalMs: 8, RunningMs: 3, RunnableMs: 5, FragmentCount: 20, StateSwitches: 19,
				MaxSegmentMs: 0.5, P95SegmentMs: 0.5,
				Summary: "dominant_state=runnable impact=5.000ms total=8.000ms fragments=20 switches=19 max_segment=0.500ms p95_segment=0.500ms totals running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms",
			}},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			Items: []tracequery.RootCauseRankItem{{
				Rank:              1,
				Tier:              "primary",
				Type:              "runnable_wait",
				Thread:            tracequery.ThreadRef{Comm: "app", PID: 20},
				StateAccountKey:   key,
				DominantState:     string(tracequery.StateRunnable),
				RunnableMs:        5,
				ImpactMs:          5,
				EffectiveImpactMs: 5,
				Causality:         "on_wakeup_chain",
				ChainRelevance:    "on_chain",
				LineStart:         4,
				LineEnd:           23,
			}},
		},
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "app", PID: 20},
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Thread:           tracequery.ThreadRef{Comm: "app", PID: 20},
				Window:           tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
				StateAccountKey:  key,
				OnChain:          true,
				DominantState:    string(tracequery.StateRunnable),
				DominantImpactMs: 5,
				TotalMs:          8,
				RunningMs:        3,
				RunnableMs:       5,
				FragmentCount:    21,
				StateSwitches:    20,
				MaxSegmentMs:     0.5,
				P95SegmentMs:     0.5,
				Summary:          "dominant_state=runnable impact=5.000ms total=8.000ms fragments=21 switches=20 max_segment=0.500ms p95_segment=0.500ms totals running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms",
				LineStart:        3,
				LineEnd:          23,
			}},
		},
	}

	rows := traceQueryTypedObservations(result, "trace", "/blobs/trace.json", "", "", time.Now())
	found := map[string]bool{}
	for _, row := range rows {
		if row.Predicate != "root_cause_primary" && row.Predicate != "wakeup_causal_impact" && row.Predicate != "state_churn" {
			continue
		}
		want := types.TraceNoteKeyStateAccountKey + "=" + key
		for _, note := range row.RichNotes {
			if strings.TrimSpace(note) == want {
				found[row.Predicate] = true
			}
		}
	}
	for _, predicate := range []string{"root_cause_primary", "wakeup_causal_impact", "state_churn"} {
		if !found[predicate] {
			t.Fatalf("%s must publish the exact state-account wire; rows=%+v", predicate, rows)
		}
	}

	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: rows,
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 1 || !strings.Contains(items[0].Text, "19 次切换/20 段") {
		t.Fatalf("typed result must publish the canonical whole-window account once: %+v", items)
	}

	// The system supplement executes the same exact bounded query into a
	// distinct payload carrier. The state-account key and source path, not the
	// payload filename, must keep the publication single-seated.
	supplementRows := traceQueryTypedObservations(result, "trace", "/blobs/supplement.json", "", "", time.Now())
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{}, []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: supplementRows,
	}})
	items = runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 1 || !strings.Contains(items[0].Text, "19 次切换/20 段") {
		t.Fatalf("model query plus exact system supplement must retain one canonical account: %+v", items)
	}
}

// A rank seat publishes the identity of the measured account it shows exactly
// once: as state_account_key where it is priced at that account, and as
// measured_state_account_key where its price is something else. A row never
// carries the hash twice, and one-seat convergence reads only the first key.
func TestTraceQueryPublishesMeasuredStateAccountKeyOnlyWherePricingKeyIsWithheld(t *testing.T) {
	const key = "state_account:v2:0123456789abcdef0123456789abcdef"
	seat := func(pricing, measured string, effective float64) []types.ObservationRecord {
		result := tracequery.Result{
			View:       "root_cause_rank",
			SourcePath: "/traces/app.systrace",
			RootCauseRank: &tracequery.RootCauseRankResult{
				Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
				Items: []tracequery.RootCauseRankItem{{
					Rank: 1, Tier: "primary", Type: "runnable_wait",
					Thread:          tracequery.ThreadRef{Comm: "dep", PID: 21},
					StateAccountKey: pricing, MeasuredStateAccountKey: measured,
					DominantState: string(tracequery.StateRunnable), RunnableMs: 5,
					ImpactMs: effective, EffectiveImpactMs: effective,
					Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
					LineStart: 4, LineEnd: 23,
				}},
			},
		}
		return traceQueryTypedObservations(result, "trace", "/blobs/trace.json", "", "", time.Now())
	}
	notes := func(rows []types.ObservationRecord) (pricing, measured []string) {
		seats := 0
		for _, row := range rows {
			if row.Predicate != "root_cause_primary" || row.Subject != "dep-21" {
				continue
			}
			seats++
			for _, note := range row.RichNotes {
				note = strings.TrimSpace(note)
				switch {
				case strings.HasPrefix(note, "state_account_key="):
					pricing = append(pricing, note)
				case strings.HasPrefix(note, "measured_state_account_key="):
					measured = append(measured, note)
				}
			}
		}
		if seats != 1 {
			t.Fatalf("fixture must publish exactly one primary seat row, got %d", seats)
		}
		return pricing, measured
	}

	// Priced at 3 ms while the measured runnable account is 5 ms.
	rows := seat("", key, 3)
	pricing, measured := notes(rows)
	if len(pricing) != 0 || len(measured) != 1 || measured[0] != "measured_state_account_key="+key {
		t.Fatalf("a seat priced differently from its measurement publishes only the measured identity: pricing=%q measured=%q", pricing, measured)
	}
	// Priced exactly at the account: the same single note as before this
	// identity existed, and no second carriage of the hash.
	pricing, measured = notes(seat(key, key, 5))
	if len(pricing) != 1 || pricing[0] != "state_account_key="+key || len(measured) != 0 {
		t.Fatalf("a seat priced at its account publishes only state_account_key: pricing=%q measured=%q", pricing, measured)
	}
	// No identity proved: neither note.
	if pricing, measured = notes(seat("", "", 3)); len(pricing) != 0 || len(measured) != 0 {
		t.Fatalf("no native identity, no note: pricing=%q measured=%q", pricing, measured)
	}

	// The compiled node carries the measured identity on its own field and
	// answers through the accessor; the pricing field stays empty.
	projection := types.TraceCausalProjectionFromObservationRecords(rows)
	compiled := 0
	for _, group := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.RankedSeats, projection.OnChainCauses} {
		for _, node := range group {
			if node.Subject != "dep-21" {
				continue
			}
			compiled++
			if node.StateAccountKey != "" || node.MeasuredStateAccountKey != key || node.MeasuredStateFactKey() != key {
				t.Fatalf("compiled seat: pricing=%q measured=%q accessor=%q", node.StateAccountKey, node.MeasuredStateAccountKey, node.MeasuredStateFactKey())
			}
		}
	}
	if compiled == 0 {
		t.Fatal("the seat did not compile into the projection")
	}
}

// A multi-occurrence account publishes one identity on the aggregate row and
// names it on each occurrence row beside that occurrence's own account.
func TestTraceQueryPublishesMultiOccurrenceAccountIdentity(t *testing.T) {
	const account = "state_account:v2:00112233445566778899aabbccddeeff"
	const own = "state_account:v2:ffeeddccbbaa99887766554433221100"
	thread := tracequery.ThreadRef{Comm: "dep", PID: 21}
	rows := func(group, aggregateKey string) []types.ObservationRecord {
		return traceQueryTypedObservations(tracequery.Result{
			View:       "wakeup_chain",
			SourcePath: "/traces/app.systrace",
			WakeupChain: &tracequery.ChainResult{
				Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
				CausalImpacts: []tracequery.WakeupCausalImpact{{
					Thread: thread, Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.004}, ChainDepth: 1, OnChain: true,
					DominantState: string(tracequery.StateRunnable), DominantImpactMs: 2, TotalMs: 2, RunnableMs: 2,
					StateAccountKey: own, MeasuredOccurrenceGroupKey: group,
				}},
				AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
					Thread: thread, OccurrenceCount: 2, ChainDepth: 1, DominantState: string(tracequery.StateRunnable),
					DominantImpactMs: 5, TotalMs: 5, RunnableMs: 5, FirstTs: 11, LastTs: 11.008,
					MeasuredStateAccountKey: aggregateKey,
				}},
			},
		}, "trace", "/blobs/trace.json", "", "", time.Now())
	}
	notesOf := func(records []types.ObservationRecord, predicate string) []string {
		for _, record := range records {
			if record.Predicate == predicate {
				return record.RichNotes
			}
		}
		t.Fatalf("no %s row", predicate)
		return nil
	}
	has := func(notes []string, want string) bool {
		for _, note := range notes {
			if strings.TrimSpace(note) == want {
				return true
			}
		}
		return false
	}
	records := rows(account, account)
	impact, aggregate := notesOf(records, "wakeup_causal_impact"), notesOf(records, "wakeup_causal_aggregate")
	if !has(impact, "state_account_key="+own) || !has(impact, "measured_occurrence_group_key="+account) {
		t.Fatalf("occurrence must keep its own account and name its group: %q", impact)
	}
	if !has(aggregate, "measured_state_account_key="+account) {
		t.Fatalf("aggregate must publish the account: %q", aggregate)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	compiled := 0
	for _, node := range measuredFactProjectionNodesHMC376(projection) {
		if node.Predicate == "wakeup_causal_impact" {
			compiled++
			if node.MeasuredOccurrenceGroupKey != account || node.MeasuredStateFactKey() != own {
				t.Fatalf("compiled occurrence: group=%q accessor=%q", node.MeasuredOccurrenceGroupKey, node.MeasuredStateFactKey())
			}
		}
	}
	if compiled == 0 {
		t.Fatal("the occurrence did not compile into the projection")
	}
	// No identity proved: neither note.
	records = rows("", "")
	for _, notes := range [][]string{notesOf(records, "wakeup_causal_impact"), notesOf(records, "wakeup_causal_aggregate")} {
		for _, note := range notes {
			if strings.HasPrefix(note, "measured_occurrence_group_key=") || strings.HasPrefix(note, "measured_state_account_key=") {
				t.Fatalf("no native identity, no note: %q", note)
			}
		}
	}
}
