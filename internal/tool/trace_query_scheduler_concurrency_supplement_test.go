package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC-08.4 adds two closed-state populations and one coverage receipt to this
// real fixture. Remove ONLY these new predicates to preserve the old 63-row
// census/wording pins; HMC-04.5 additionally contributes one S and one D
// summary. Pin that delta separately, not an unrelated root/chain/state change.
func schedulerConcurrencySupplementLegacyMeta(t *testing.T, ctx *types.BusContext) *types.SystemTraceSupplementMeta {
	t.Helper()
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	results := ctx.Mutable.SystemTraceSupplementResults()
	if meta == nil || len(results) != 1 || len(meta.Views) != 1 || len(meta.ViewValueObservations) != 1 || len(meta.ViewObservationFamilies) != 1 {
		t.Fatal("fixture no longer has one supplemental result receipt")
	}
	old := results[0]
	old.Observations = nil
	states := map[string]int{}
	sleepSummaries := map[string]int{}
	coverage := 0
	for _, r := range results[0].Observations {
		switch r.Predicate {
		case "scheduler_concurrency":
			states[r.Subject]++
		case "scheduler_concurrency_coverage":
			coverage++
		case "target_sleep_state_summary":
			sleepSummaries[r.Object]++
			if r.ResultCount == nil || *r.ResultCount != 2 {
				t.Fatalf("full-population sleep summary count changed: %+v", r)
			}
		default:
			old.Observations = append(old.Observations, r)
			continue
		}
		if r.Role != types.AnswerAggregateRoleSupportingCoverage || r.Producer != "trace_query" || r.SourceRef.QueryScopeID == "" {
			t.Fatalf("new disclosure delta acquired wrong authority: %+v", r)
		}
	}
	if !reflect.DeepEqual(states, map[string]int{"running": 1, "runnable": 1}) || coverage != 1 {
		t.Fatalf("not the reviewed three-row additive delta: states=%v coverage=%d", states, coverage)
	}
	if !reflect.DeepEqual(sleepSummaries, map[string]int{"s_sleep": 1, "d_sleep": 1}) {
		t.Fatalf("not the reviewed two-row sleep summary delta: %v", sleepSummaries)
	}
	oldFamilies := traceSupplementViewFamilyCensus(old)
	wantFamilies := types.TraceSupplementViewFamilyCensus{RootCauseRows: 12, WakeupChainRows: 9, TargetStateRows: 12, OtherRows: 30}
	if oldFamilies != wantFamilies || traceSupplementValueObservationCount(old) != 63 {
		t.Fatalf("old supplemental population changed: %+v", oldFamilies)
	}
	newFamilies := oldFamilies
	newFamilies.OtherRows += 3
	newFamilies.TargetStateRows += 2
	if meta.ViewValueObservations[0] != 68 || meta.ViewObservationFamilies[0] != newFamilies {
		t.Fatalf("new population is not additive background context: %+v", meta)
	}
	meta.ViewValueObservations[0] = 63
	meta.ViewObservationFamilies[0] = oldFamilies
	return meta
}
