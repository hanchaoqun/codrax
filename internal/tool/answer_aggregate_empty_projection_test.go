package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Empty is a completed projection, not a request to revive its inputs. Exercise
// non-trace projectors too, so the fix cannot depend on one runtime profile.
func TestPreEmitAggregateProjectionRespectsNonTraceEmptyResult(t *testing.T) {
	for _, boundary := range []string{"no_directed_path", "peer_errors"} {
		for _, mixed := range []bool{false, true} {
			t.Run(boundary+map[bool]string{false: "/empty", true: "/mixed"}[mixed], func(t *testing.T) {
				mu := types.NewMutableState("typed aggregate projection")
				ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}
				var facts []types.AnswerAggregateFact
				if boundary == "no_directed_path" {
					ir.RequestModel = types.RequestModel{
						Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
						CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "Source.run", Sink: "Sink.run"},
						AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"Source.run", "Sink.run"}},
					}
					mu.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{
						Reason: types.PrincipalSpanWaiverNoDirectedPath, Rationale: "only a shared callee was observed",
					})
					facts = []types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet,
						Label: "unsupported ordered path", Value: "2", Role: types.AnswerAggregateRolePrincipalAnswer,
						Members: []string{"Source.run", "Sink.run"},
					}}
				} else {
					mu.SetLogTriage(&types.LogBundle{Errors: []types.LogError{
						{Type: "native_error", Message: "first error"}, {Type: "bridge_error", Message: "second error"},
					}})
					facts = []types.AnswerAggregateFact{
						{Kind: types.AnswerAggregateBehaviorOutcome, Label: "unproved propagation", Value: "first caused second", Role: types.AnswerAggregateRoleSupportingCoverage},
						{Kind: types.AnswerAggregateErrorGranularity, Label: "unproved grouping", Value: "one propagated failure", Role: types.AnswerAggregateRoleSupportingCoverage},
					}
				}
				if mixed {
					facts = append(facts, types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar,
						Label: "inspected source edge count", Value: "5", Role: types.AnswerAggregateRoleSupportingCoverage,
						SupportRefs: []string{"internal/worker.go:12"},
					})
				}
				mu.SetInvestigationAggregateFacts(facts)
				mu.SetInvestigationComplete("model handoff retained for audit")
				mu.RetainInvestigationAggregateFacts()
				ctx := &types.BusContext{AnalysisIR: ir, Mutable: mu}
				raw := mu.StableInvestigationAggregateFacts()
				plan := answerSurfacePlan(ctx)
				wantCount := 0
				if mixed {
					wantCount = 1
				}
				if plan == nil || len(plan.StableAggregateFacts) != wantCount {
					t.Fatalf("test must exercise a real completed projection: %+v", plan)
				}
				if got := preEmitStableAggregateFacts(ctx); !reflect.DeepEqual(got, plan.StableAggregateFacts) {
					t.Errorf("direct reader revived excluded input: got=%+v want=%+v", got, plan.StableAggregateFacts)
				}
				cached := newPreEmitCheckContext(ctx)
				for i := 0; i < 8; i++ {
					if got := cached.stableAggregateFactsForCheck(); !reflect.DeepEqual(got, plan.StableAggregateFacts) {
						t.Errorf("cached reader revived excluded input: %+v", got)
					}
					if got := cached.principalAggregateMemberSetFactRefsForCheck(); len(got) != 0 {
						t.Errorf("excluded path became a required roster: %+v", got)
					}
				}
				if !cached.stableFactsExcluded || cached.derivedBuilds != (preEmitDerivedBuildCounts{1, 1, 1}) {
					t.Errorf("empty projection must be normalized and cached once: excluded=%v builds=%+v", cached.stableFactsExcluded, cached.derivedBuilds)
				}
				if !reflect.DeepEqual(raw, mu.StableInvestigationAggregateFacts()) {
					t.Fatal("answer view mutated durable model audit facts")
				}
			})
		}
	}
}

func TestPreEmitAggregateProjectionNilAndEmptySlicesAreAuthoritative(t *testing.T) {
	mu := types.NewMutableState("legacy input")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar, Label: "raw scalar", Value: "97"}})
	mu.SetInvestigationComplete("retained")
	for _, facts := range [][]types.AnswerAggregateFact{nil, {}} {
		cached := newPreEmitCheckContext(&types.BusContext{Mutable: mu})
		cached.surfacePlanBuilt = true
		cached.surfacePlan = &types.AnswerSurfacePlan{StableAggregateFacts: facts}
		for i := 0; i < 3; i++ {
			if got := cached.stableAggregateFactsForCheck(); !reflect.DeepEqual(got, facts) {
				t.Errorf("present plan must preserve nil/empty result, got %+v", got)
			}
		}
		if !cached.stableFactsExcluded || cached.derivedBuilds.stableAggregateFacts != 1 {
			t.Errorf("cached empty authority was not retained: %+v", cached.derivedBuilds)
		}
	}
}

func TestPreEmitAggregateProjectionNoPlanKeepsLegacyHandoff(t *testing.T) {
	mu := types.NewMutableState("legacy input")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar, Label: "raw scalar", Value: "97"}})
	mu.SetInvestigationComplete("retained")
	ctx := &types.BusContext{Mutable: mu}
	if answerSurfacePlan(ctx) != nil {
		t.Fatal("test must use the compatibility lane without a plan")
	}
	want := mu.StableInvestigationAggregateFacts()
	if len(want) != 1 || !reflect.DeepEqual(preEmitStableAggregateFacts(ctx), want) {
		t.Fatal("no-plan legacy input must remain available")
	}
	cached := newPreEmitCheckContext(ctx)
	if !reflect.DeepEqual(cached.stableAggregateFactsForCheck(), want) || cached.stableFactsExcluded {
		t.Fatal("legacy cache must retain input without claiming it was projected")
	}
	if preEmitStableAggregateFacts(nil) != nil || newPreEmitCheckContext().stableAggregateFactsForCheck() != nil {
		t.Fatal("missing context must remain nil-safe")
	}
}

func TestPreEmitAggregateProjectionCacheDoesNotLeakAcrossGenerations(t *testing.T) {
	mu := types.NewMutableState("generation test")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateBehaviorOutcome, Label: "unsupported", Value: "one caused two"}})
	mu.SetInvestigationComplete("retained")
	mu.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "first"}, {Type: "second"}}})
	ctx := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	first := newPreEmitCheckContext(ctx)
	if got := first.stableAggregateFactsForCheck(); len(got) != 0 {
		t.Fatalf("first generation must preserve explicit empty projection: %+v", got)
	}
	// One remaining error still has direct-runtime authority. Clear the
	// producer, as between partial dispatches, to exercise a changed plan.
	mu.SetLogTriage(nil)
	second := newPreEmitCheckContext(ctx)
	if len(second.stableAggregateFactsForCheck()) != 1 {
		t.Fatal("new emit/patch generation must recompile changed evidence authority")
	}
	if len(first.stableAggregateFactsForCheck()) != 0 || first.derivedBuilds.surfacePlan != 1 || second.derivedBuilds.surfacePlan != 1 {
		t.Fatal("request-local immutable cache was changed or rebuilt")
	}
}
