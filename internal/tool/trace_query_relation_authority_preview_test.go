package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryCarriesTypedRelationAuthorityBeforeExploreClosure(t *testing.T) {
	target := tracequery.ThreadRef{Comm: "target", PID: 17267}
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{
				{Rank: 4, Type: "runnable_wait", Thread: target, EffectiveImpactMs: 3.956, FixDirection: "scheduling_supply", Causality: "self_wall_clock", ChainRelevance: "on_chain"},
				{Rank: 10, Type: "runnable_wait", Thread: target, EffectiveImpactMs: 1.648, FixDirection: "scheduling_supply", Causality: "on_wakeup_chain", ChainRelevance: "on_chain"},
				{Rank: 13, Type: "runnable_wait", Thread: target, EffectiveImpactMs: 1.193, FixDirection: "scheduling_supply", Causality: "self_wall_clock", ChainRelevance: "on_chain"},
			},
			SelfRunnableTwoRuler: &tracequery.SelfRunnableTwoRulerAccounting{
				Thread:         target,
				WallSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 4, EffMs: 3.956}, {Rank: 13, EffMs: 1.193}},
				EdgeSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 10, EffMs: 1.648}},
				WallSubtotalMs: 5.149,
				EdgeSubtotalMs: 1.648,
			},
		},
		TargetWindowStates: &tracequery.TargetWindowStateAccount{Thread: target, TotalMs: 233.190},
		WindowStats: &tracequery.WindowStats{BlockedReasonCensus: []tracequery.BlockedReasonPIDCensus{{
			Thread: target,
			Count:  50,
			Callers: []tracequery.BlockedReasonCensusCaller{{
				Caller: "fscache_page_wait", Count: 50, DelayTotalMs: 16.358,
			}},
		}}},
	}

	got := traceQuerySummary(result, traceQueryParams{View: result.View}, "attached_trace", "/tmp/result.json")
	for _, want := range []string{
		"relation_authority scope=root_cause_rank policy=typed_pair_only",
		"rank_row_state_breakdown scope=this_row_only cross_row_containment=unproven_without_exact_pair_carrier",
		"fix_direction role=repair_classification_only same_direction_addition=not_authorized_without_exact_typed_subtotal",
		"self_wall_clock_seats=#4:3.956ms,#13:1.193ms self_wall_clock_subtotal=5.149ms",
		"wakeup_edge_seats=#10:1.648ms wakeup_edge_subtotal=1.648ms",
		"cross_ruler_addition=forbidden cross_ruler_physical_relation=unresolved",
		"relation_claim_required authority_id=trace:self_runnable_two_ruler:",
		"addition=authorized_to_published_subtotal subtotal_value=5.149 subtotal_unit=ms model_must_copy_to=emit_investigation_complete.relation_claims",
		"physical_relation=unresolved addition=forbidden model_must_copy_to=emit_investigation_complete.relation_claims",
		"blocked_reason_census_relation subject=target-17267 records=50 value_caliber=kernel_record_count caller_delay_caliber=vendor_reported_delay_sum state_relation_authority=census_alone_not_sufficient typed_interval_join_required=true add_or_subtract_from_state_total=not_authorized_by_census_alone",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing relation authority %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "cross_ruler_total") || strings.Contains(got, "5.604ms") {
		t.Fatalf("relation preview must never mint a cross-ruler total:\n%s", got)
	}
	if relationAt, bodyAt := strings.Index(got, "relation_authority scope="), strings.Index(got, "## Root cause rank"); relationAt < 0 || bodyAt < 0 || relationAt > bodyAt {
		t.Fatalf("relation authority must remain in the model-visible head, relation=%d body=%d:\n%s", relationAt, bodyAt, got)
	}
}

func TestTraceQuerySummarySilencesInvalidTwoRulerCarrier(t *testing.T) {
	target := tracequery.ThreadRef{Comm: "target", PID: 7}
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{{Rank: 1, Type: "runnable_wait", Thread: target, EffectiveImpactMs: 2}},
			SelfRunnableTwoRuler: &tracequery.SelfRunnableTwoRulerAccounting{
				Thread:    target,
				WallSeats: []tracequery.SelfRunnableTwoRulerSeat{{Rank: 1, EffMs: 2}},
				EdgeSeats: []tracequery.SelfRunnableTwoRulerSeat{{Rank: 2, EffMs: 1}},
				// Deliberately invalid: the wall subtotal does not reproduce.
				WallSubtotalMs: 9,
				EdgeSubtotalMs: 1,
			},
		},
	}
	got := traceQuerySummary(result, traceQueryParams{View: result.View}, "attached_trace", "/tmp/result.json")
	if strings.Contains(got, "self_runnable_two_ruler subject=") {
		t.Fatalf("invalid typed carrier must fail closed:\n%s", got)
	}
	if !strings.Contains(got, "relation_authority scope=root_cause_rank policy=typed_pair_only") {
		t.Fatalf("generic relation boundary must remain available when an invalid positive carrier is suppressed:\n%s", got)
	}
}

func TestTraceQuerySummaryDoesNotInventBlockedReasonStateRelationAcrossThreads(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank: 1, Type: "running", Thread: tracequery.ThreadRef{Comm: "target", PID: 10}, EffectiveImpactMs: 1,
		}}},
		TargetWindowStates: &tracequery.TargetWindowStateAccount{Thread: tracequery.ThreadRef{Comm: "target", PID: 10}, TotalMs: 4},
		WindowStats: &tracequery.WindowStats{BlockedReasonCensus: []tracequery.BlockedReasonPIDCensus{{
			Thread: tracequery.ThreadRef{Comm: "other", PID: 11}, Count: 8,
		}}},
	}
	got := traceQuerySummary(result, traceQueryParams{View: result.View}, "attached_trace", "/tmp/result.json")
	if strings.Contains(got, "blocked_reason_census_relation") {
		t.Fatalf("a different thread's census must not be related to the target state account:\n%s", got)
	}
}
