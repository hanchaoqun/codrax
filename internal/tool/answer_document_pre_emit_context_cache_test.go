package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreEmitCheckContextCachesAggregateDerivationsAcrossRosterItems(t *testing.T) {
	ctx := qceSpecimenCtx()
	wantFacts := preEmitStableAggregateFacts(ctx)
	wantRefs := preEmitPrincipalAggregateMemberSetFactRefs(ctx, wantFacts)
	pctx := newPreEmitCheckContext(ctx)
	label := "StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)"
	cit := types.Citation{File: "internal/types/enums.go", Line: 33}
	for i := 0; i < 128; i++ {
		if !preEmitCitationSupportsAggregateItemWithContext(pctx, label, "", cit) {
			t.Fatalf("iteration %d lost the existing typed aggregate citation authority", i)
		}
		if !preEmitItemMatchesPrincipalAggregateMemberWithContext(pctx, label, "") {
			t.Fatalf("iteration %d lost the existing principal member authority", i)
		}
		if got := pctx.stableAggregateFactsForCheck(); !reflect.DeepEqual(got, wantFacts) {
			t.Fatalf("iteration %d cached stable facts changed semantics:\n got=%+v\nwant=%+v", i, got, wantFacts)
		}
		if got := pctx.principalAggregateMemberSetFactRefsForCheck(); !reflect.DeepEqual(got, wantRefs) {
			t.Fatalf("iteration %d cached principal refs changed semantics:\n got=%+v\nwant=%+v", i, got, wantRefs)
		}
	}
	if got := pctx.derivedBuilds; got.surfacePlan != 1 || got.stableAggregateFacts != 1 || got.principalAggregateRefs != 1 {
		t.Fatalf("request-local immutable derivations must build once, got %+v", got)
	}
}

func TestPreEmitCheckContextCacheIsNotSharedAcrossPatches(t *testing.T) {
	ctx := qceSpecimenCtx()
	first := newPreEmitCheckContext(ctx)
	second := newPreEmitCheckContext(ctx)
	first.principalAggregateMemberSetFactRefsForCheck()
	second.principalAggregateMemberSetFactRefsForCheck()
	if first.derivedBuilds.principalAggregateRefs != 1 || second.derivedBuilds.principalAggregateRefs != 1 {
		t.Fatalf("each emit/patch context needs an independent derivation generation: first=%+v second=%+v", first.derivedBuilds, second.derivedBuilds)
	}
}

func BenchmarkPreEmitAggregateCitationRosterCache(b *testing.B) {
	ctx := qceSpecimenCtx()
	label := "StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)"
	cit := types.Citation{File: "internal/types/enums.go", Line: 33}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pctx := newPreEmitCheckContext(ctx)
		for item := 0; item < 128; item++ {
			preEmitCitationSupportsAggregateItemWithContext(pctx, label, "", cit)
		}
	}
}

func BenchmarkPreEmitAggregateCitationRosterUncached(b *testing.B) {
	ctx := qceSpecimenCtx()
	label := "StageAnalyze (analyzer, classifier + repo_map + AnalysisIR)"
	cit := types.Citation{File: "internal/types/enums.go", Line: 33}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for item := 0; item < 128; item++ {
			preEmitCitationSupportsAggregateItemWithContext(newPreEmitCheckContext(ctx), label, "", cit)
		}
	}
}
