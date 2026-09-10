package tracequery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Start at actual Run publication without referring to proposed Go fields:
// the first RED must be a missing provenance handoff, not a compile error.
func b1638b2bPublishedSources(t *testing.T, row any) map[string]any {
	t.Helper()
	body, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	sources, ok := object["measurement_sources"].(map[string]any)
	if !ok {
		t.Fatalf("derived scheduler row lost native source references: %s", body)
	}
	return sources
}

func b1638b2bAssertSources(t *testing.T, got, want *types.TraceSchedulerMeasurementSources) {
	t.Helper()
	if want == nil || len(want.Domains) == 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("derived row must retain exactly its native input sources: got=%+v want=%+v", got, want)
	}
}

func TestB1638B2BActualRunDrilldownSourceHandoff(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	for _, tc := range []struct {
		name, runningSource string
		minimum             float64
	}{{"churn_winner", "state_churn", .000001}, {"duration_winner", "top_running", 100}} {
		t.Run(tc.name, func(t *testing.T) {
			result := Run(idx, Query{View: "window_stats", PID: 300, TimeStart: 5, TimeEnd: 5.07,
				TimeStartSet: true, TimeEndSet: true, MinDurationMs: tc.minimum})
			if result.WindowStats == nil {
				t.Fatal("real Run lost its window statistics")
			}
			foundRunning, foundRunnable := false, false
			for _, step := range result.WindowStats.StateDrilldownPlan {
				if step.Thread.PID != 300 {
					continue
				}
				if step.State == string(StateRunning) {
					foundRunning = true
					if step.Source != tc.runningSource || step.ImpactMs <= 0 {
						t.Fatalf("existing drilldown winner/value changed: %+v", step)
					}
					b1638b2bPublishedSources(t, step)
					var want *types.TraceSchedulerMeasurementSources
					if tc.runningSource == "state_churn" {
						want = types.TraceSchedulerMeasurementSourcesFromDomain(result.WindowStats.StateChurn[0].MeasurementDomain)
						if !near(step.ImpactMs, 30, .000001) || !near(step.TotalMs, 70, .000001) {
							t.Fatalf("churn winner lost its separate dominant/total account: %+v", step)
						}
					} else {
						want = threadDurationMeasurementSources(result.WindowStats.TopRunning[0])
						if !near(step.ImpactMs, 20, .000001) {
							t.Fatalf("running winner no longer preserves its single CPU bucket: %+v", step)
						}
					}
					b1638b2bAssertSources(t, step.MeasurementSources, want)
				}
				if step.State == string(StateRunnable) {
					foundRunnable = true
					// The drilldown chooses one CPU bucket (10ms); unlike the
					// ordinary rank family below, it does not sum both CPUs.
					if step.Source != "top_runnable" || !near(step.ImpactMs, 10, .000001) {
						t.Fatalf("native runnable account changed: %+v", step)
					}
					b1638b2bPublishedSources(t, step)
					b1638b2bAssertSources(t, step.MeasurementSources, threadDurationMeasurementSources(result.WindowStats.RunnableTop[0]))
				}
			}
			if !foundRunning || !foundRunnable {
				t.Fatalf("fixture must exercise both drilldown constructors: %+v", result.WindowStats.StateDrilldownPlan)
			}
		})
	}
}

func TestB1638B2BActualRunOrdinaryRankSourceHandoff(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	result := Run(idx, Query{View: "root_cause_rank", TimeStart: 5, TimeEnd: 5.07,
		TimeStartSet: true, TimeEndSet: true, MinDurationMs: .000001, Limit: 100})
	if result.RootCauseRank == nil {
		t.Fatal("real Run lost its root-cause result")
	}
	seen := map[string]bool{}
	stats := ComputeWindowStats(idx, Query{TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true, MinDurationMs: .000001})
	rows := append([]RootCauseRankItem(nil), result.RootCauseRank.Items...)
	rows = append(rows, result.RootCauseRank.AbsorbedItems...)
	for _, row := range rows {
		if row.Thread.PID != 300 || !strings.HasPrefix(row.Source, "window_stats") {
			continue
		}
		if row.Type == "runnable_wait" || row.Type == "sleep_wait" || row.Source == "window_stats.state_churn" {
			seen[row.Source+":"+row.Type] = true
			b1638b2bPublishedSources(t, row)
			var want []*types.TraceSchedulerMeasurementSources
			switch {
			case row.Source == "window_stats.state_churn":
				want = append(want, types.TraceSchedulerMeasurementSourcesFromDomain(stats.StateChurn[0].MeasurementDomain))
			case row.Type == "runnable_wait":
				for _, td := range stats.RunnableTop {
					if td.Thread.PID == 300 {
						want = append(want, threadDurationMeasurementSources(td))
					}
				}
				if !near(row.CumulativeImpactMs, 20, .000001) || row.MemberCount != 2 {
					t.Fatalf("ordinary family lost its original two CPU members/20ms: %+v", row)
				}
			case row.Type == "sleep_wait":
				for _, td := range stats.SleepTop {
					if td.Thread.PID == 300 {
						want = append(want, threadDurationMeasurementSources(td))
					}
				}
			}
			b1638b2bAssertSources(t, row.MeasurementSources, types.MergeTraceSchedulerMeasurementSources(want...))
		}
	}
	if len(seen) < 2 {
		t.Fatalf("fixture must publish ordinary duration/churn source rows, got %v", seen)
	}
}

func TestB1638B2BActualRunAnchoredRemainderKeepsNativeOrigin(t *testing.T) {
	idx := runsplitF2ProbeTrace(t)
	q := Query{View: "root_cause_rank", PID: 100, TimeStart: 1, TimeEnd: 1.2, MaxDepth: 4,
		MinDurationMs: .05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	result := Run(idx, q)
	if result.RootCauseRank == nil {
		t.Fatal("actual Run did not publish the existing anchored/remainder pair")
	}
	found := false
	for _, row := range result.RootCauseRank.Items {
		if row.Thread.PID != 200 || row.Type != "runnable_wait" || row.Source != "window_stats" || !row.ChainAnchorRemainderSeat {
			continue
		}
		found = true
		if !near(row.RunnableMs, 35, .000001) || !near(row.ChainAnchorFullMs, 37, .000001) ||
			row.ChainRelevance != "adjacent" || rootCauseItemDirectionPopulationEligible(&row) {
			t.Fatalf("source handoff changed the established remainder's numeric/causal boundary: %+v", row)
		}
		b1638b2bPublishedSources(t, row)
	}
	if !found {
		t.Fatal("fixture did not exercise the real window-stat remainder transformation")
	}
}

func TestB1638B2BActualRunDIOCauseSubsetsKeepOriginOnly(t *testing.T) {
	// One CPU bucket contains both a proven cause and an unproven remainder;
	// this is a constructed counterpart of the existing three-CPU fixture,
	// not a changed customer capture. The legacy slice branch must stay live.
	body := strings.NewReplacer("[003]", "[002]", "[004]", "[002]", "target_cpu=003", "target_cpu=002", "target_cpu=004", "target_cpu=002").Replace(dioPartitionPartialTrace)
	idx := buildTraceIndex(t, "measurement_cause_subsets.systrace", body)
	q := Query{View: "root_cause_rank", PID: 100, TimeStart: 3, TimeEnd: 3.12, TimeStartSet: true, TimeEndSet: true,
		MaxDepth: 4, MinDurationMs: .05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 32}
	stats := ComputeWindowStats(idx, ensureQueryFlavor(idx, q))
	splitSeen := false
	var parentSources []*types.TraceSchedulerMeasurementSources
	for _, td := range stats.dstateCensus {
		if td.Thread.PID == 200 && len(td.causeSlices) > 1 && td.MeasurementDomain != nil {
			splitSeen = true
			parentSources = append(parentSources, threadDurationMeasurementSources(td))
		}
	}
	if !splitSeen {
		t.Fatal("fixture must exercise a genuine native bucket with multiple cause slices")
	}
	result := Run(idx, q)
	if result.RootCauseRank == nil {
		t.Fatal("real Run lost its D/IO rank result")
	}
	seats := dioPartitionSeats(*result.RootCauseRank)
	if len(seats) != 2 {
		t.Fatalf("existing cause/remainder partition must survive: %+v", seats)
	}
	for _, row := range seats {
		want := 10.0
		if row.BlockedReasonCaller == "dma_fence_default_wait" {
			want = 50
		} else if !row.DStateCauseUnprovenRemainder {
			t.Fatalf("unproven member lost its original remainder qualification: %+v", row)
		}
		if !near(row.CumulativeImpactMs, want, .000001) {
			t.Fatalf("subset acquired its parent bucket's complete value: %+v", row)
		}
		b1638b2bPublishedSources(t, row)
		b1638b2bAssertSources(t, row.MeasurementSources, types.MergeTraceSchedulerMeasurementSources(parentSources...))
	}
}

func TestB1638B2BFamilySourcesKeepAllMembersAndUnknown(t *testing.T) {
	idx := b1638b2aRunningChurnIndex(t)
	q := Query{TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true}
	var rows []RootCauseRankItem
	var inputs []*types.TraceSchedulerMeasurementSources
	for i := 0; i < 10; i++ {
		query := q
		query.LineEnd = 20 + i // Same values, genuinely distinct native line scopes.
		stats := ComputeWindowStats(idx, query)
		if len(stats.RunnableTop) == 0 || stats.RunnableTop[0].MeasurementDomain == nil {
			t.Fatal("actual native producer must establish each source")
		}
		row := rcmRankItem("runnable_wait", ThreadRef{Comm: "churny", PID: 300}, 1,
			5+float64(i)*.002, 5+float64(i)*.002+.001, 30+i*2, 31+i*2)
		row.MeasurementSources = threadDurationMeasurementSources(stats.RunnableTop[0])
		rows = append(rows, row)
		inputs = append(inputs, row.MeasurementSources)
	}
	for _, unknown := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			members := append([]RootCauseRankItem(nil), rows...)
			wants := append([]*types.TraceSchedulerMeasurementSources(nil), inputs...)
			if unknown {
				members[0].MeasurementSources = nil
				wants[0] = nil
			}
			if reverse {
				for i, j := 0, len(members)-1; i < j; i, j = i+1, j-1 {
					members[i], members[j] = members[j], members[i]
				}
			}
			legacy := append([]RootCauseRankItem(nil), members...)
			indices := make([]int, len(members))
			for i := range members {
				indices[i] = i
				legacy[i].MeasurementSources = nil
			}
			got := mergeSameThreadTypeRankFamily(q, false, members, indices)
			b1638b2bAssertSources(t, got.MeasurementSources, types.MergeTraceSchedulerMeasurementSources(wants...))
			if got.MemberCount != 10 || len(got.MemberRoster) >= got.MemberCount || got.MeasurementSources.HasUnknown != unknown {
				t.Fatalf("all-member source union must outlive display cap and keep unknown: %+v", got)
			}
			want := mergeSameThreadTypeRankFamily(q, false, legacy, indices)
			withoutSource := got
			withoutSource.MeasurementSources = nil
			if !reflect.DeepEqual(withoutSource, want) {
				t.Fatal("source references changed family values, eligibility, fold caliber or display roster")
			}
			// A subsequent fold cannot silently repair unknown or borrow the
			// representative's known domain; inputs remain independently owned.
			again := rootCauseMemberMeasurementSources([]RootCauseRankItem{got, members[len(members)-1]})
			if again.HasUnknown != unknown {
				t.Fatal("refolding repaired an unknown member")
			}
			before := inputs[1].Domains[0]
			for i := range got.MeasurementSources.Domains {
				got.MeasurementSources.Domains[i].PartitionID = "mutated family"
			}
			if inputs[1].Domains[0] != before {
				t.Fatal("family source mutation reached a member")
			}
		}
	}
}

func TestB1638B2BThreadDurationSourceMergeAndCopies(t *testing.T) {
	stats := ComputeWindowStats(b1638b2aRunningChurnIndex(t), Query{TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true})
	if len(stats.TopRunning) != 2 || stats.TopRunning[0].MeasurementDomain == nil || stats.TopRunning[1].MeasurementDomain == nil {
		t.Fatal("actual per-CPU producer must yield two independent native sources")
	}
	for _, unknown := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			members := []ThreadDuration{cloneThreadDurationMeasurement(stats.TopRunning[0]), cloneThreadDurationMeasurement(stats.TopRunning[1])}
			// An existing uncertain set is authoritative about uncertainty;
			// the retained legacy singular pointer cannot fill it in.
			if unknown {
				members[0].MeasurementSources = &types.TraceSchedulerMeasurementSources{HasUnknown: true}
			}
			if reverse {
				members[0], members[1] = members[1], members[0]
			}
			want := types.MergeTraceSchedulerMeasurementSources(threadDurationMeasurementSources(members[0]), threadDurationMeasurementSources(members[1]))
			census := map[string]ThreadDuration{"a": members[0], "b": members[1]}
			got := aggregateChainRunnableCensusByThread(census, map[int]bool{300: true}, 0)
			if len(got) != 1 || !near(got[0].DurationMs, 30, .000001) || got[0].CPU != -1 || got[0].MeasurementDomain != nil {
				t.Fatalf("existing aggregation and singular disagreement boundary changed: %+v", got)
			}
			b1638b2bAssertSources(t, got[0].MeasurementSources, want)
			if got[0].MeasurementSources.HasUnknown != unknown {
				t.Fatal("known legacy field repaired an existing unknown collection")
			}
			for _, copied := range []ThreadDuration{
				cloneThreadDurationMeasurement(got[0]), copyThreadDurationDisplayRoster(got, 1)[0],
				topThreadDurations(map[string]ThreadDuration{"one": got[0]}, 1)[0],
			} {
				b1638b2bAssertSources(t, copied.MeasurementSources, want)
				copied.MeasurementSources.Domains[0].PartitionID = "mutated copy"
				b1638b2bAssertSources(t, got[0].MeasurementSources, want)
			}
		}
	}
}

func TestB1638B2BAnchoredCloneAndCausalConstructorsPreserveSourceOwnership(t *testing.T) {
	stats := ComputeWindowStats(b1638b2aRunningChurnIndex(t), Query{TimeStart: 5, TimeEnd: 5.07, TimeStartSet: true, TimeEndSet: true})
	sources := threadDurationMeasurementSources(stats.RunnableTop[0])
	seat := rcmRankItem("runnable_wait", ThreadRef{Comm: "churny", PID: 300}, 10, 5.02, 5.03, 4, 5)
	seat.MeasurementSources = sources
	seat.ChainRelevance = "on_chain"
	clone := rspaCloneAsRemainderSeat(seat, 3, 10, 7, 0, 0, "original remainder")
	rspaClipSeatToAnchored(&seat, 3, 10, 3, 0, 0, "original anchored")
	if clone.RunnableMs != 7 || seat.RunnableMs != 3 || !clone.ChainAnchorRemainderSeat || seat.ChainAnchorRemainderSeat {
		t.Fatal("existing anchored/remainder values or roles changed")
	}
	b1638b2bAssertSources(t, clone.MeasurementSources, sources)
	clone.MeasurementSources.Domains[0].PartitionID = "mutated remainder"
	b1638b2bAssertSources(t, seat.MeasurementSources, sources)

	for _, priority := range []bool{false, true} {
		impact := WakeupCausalImpact{Thread: seat.Thread, DominantState: string(StateRunnable), RunnableMs: 10, TotalMs: 10,
			PriorityInversionGatedMs: 3, MeasurementSources: sources}
		aggregate := WakeupCausalAggregate{Thread: seat.Thread, DominantState: string(StateRunnable), RunnableMs: 10, TotalMs: 10,
			PriorityInversionGatedMs: 3, MeasurementSources: sources}
		got := []RootCauseRankItem{rootCauseItemFromCausalImpactRole(impact, priority), rootCauseItemFromCausalAggregateRole(aggregate, priority)}
		impact.MeasurementSources, aggregate.MeasurementSources = nil, nil
		legacy := []RootCauseRankItem{rootCauseItemFromCausalImpactRole(impact, priority), rootCauseItemFromCausalAggregateRole(aggregate, priority)}
		for i, row := range got {
			b1638b2bAssertSources(t, row.MeasurementSources, sources)
			row.MeasurementSources.Domains[0].PartitionID = "mutated rank"
			if sources.Domains[0].PartitionID == "mutated rank" {
				t.Fatal("rank constructor aliases its causal input")
			}
			row.MeasurementSources = nil
			if !reflect.DeepEqual(row, legacy[i]) {
				t.Fatal("causal source handoff changed numeric, priority or chain permission fields")
			}
		}
	}
}

func TestB1638B2BUnknownWinnerCannotBorrowAnotherNativeMethod(t *testing.T) {
	stats := ComputeWindowStats(b1638b2aRunningChurnIndex(t), Query{TimeStart: 5, TimeEnd: 5.07,
		TimeStartSet: true, TimeEndSet: true, MinDurationMs: .000001})
	if len(stats.StateChurn) != 1 || len(stats.TopRunning) != 2 || stats.TopRunning[0].MeasurementDomain == nil {
		t.Fatal("fixture must retain separate churn and running sources")
	}
	// Simulate a legacy churn carrier while the other method is still known.
	// The selected state_churn winner must stay unknown, not copy TopRunning.
	stats.StateChurn[0].MeasurementDomain = nil
	steps, _ := buildStateDrilldownPlanForTarget(stats, 100, 300, "")
	found := false
	for _, step := range steps {
		if step.Thread.PID == 300 && step.State == string(StateRunning) {
			found = true
			if step.Source != "state_churn" || step.MeasurementSources != nil || !near(step.ImpactMs, 30, .000001) {
				t.Fatalf("unknown winner borrowed a distinct method or changed selection: %+v", step)
			}
		}
	}
	if !found {
		t.Fatal("unknown winner was dropped")
	}
	legacy := []RootCauseRankItem{
		rcmRankItem("runnable_wait", ThreadRef{PID: 300}, 1, 5.01, 5.011, 1, 2),
		rcmRankItem("runnable_wait", ThreadRef{PID: 300}, 1, 5.02, 5.021, 3, 4),
	}
	fold := mergeSameThreadTypeRankFamily(Query{TimeStart: 5, TimeEnd: 5.07}, false, legacy, []int{0, 1})
	if fold.MeasurementSources != nil || fold.MemberCount != 2 {
		t.Fatal("legacy family must remain usable without fabricated sources")
	}
}
