package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1700AutomaticCitationContextPreservesRawFactsAndPatchGeneration(t *testing.T) {
	ctx, citation, _ := b1700AggregateCitationReviewContext(t, "Ghost")
	pctx := newPreEmitCheckContext(ctx)
	rawRefs := pctx.principalAggregateMemberSetFactRefsForCheck()
	rawFacts := pctx.stableAggregateFactsForCheck()
	before, _ := json.Marshal(rawFacts)
	auto := pctx.forAutomaticSourceCitation()
	if auto == pctx || len(auto.principalAggregateMemberSetFactRefsForCheck()) != 0 || len(auto.stableAggregateFactsForCheck()) != 0 {
		t.Fatal("unobserved member was not isolated from automatic citation candidates")
	}
	for i := 0; i < 128; i++ {
		if pctx.forAutomaticSourceCitation() != auto || auto.forAutomaticSourceCitation() != auto {
			t.Fatal("automatic candidate view must be cached once per emit")
		}
		if !reflect.DeepEqual(rawRefs, pctx.principalAggregateMemberSetFactRefsForCheck()) ||
			!preEmitCitationSupportsAggregateItemWithContext(pctx, "Ghost", "", citation) {
			t.Fatal("filtering automatic candidates changed existing model-selection compatibility")
		}
	}
	after, _ := json.Marshal(pctx.stableAggregateFactsForCheck())
	if string(before) != string(after) || pctx.derivedBuilds.principalAggregateRefs != 1 {
		t.Fatal("automatic view mutated or repeatedly rebuilt the original fact cache")
	}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet,
		Role: types.AnswerAggregateRolePrincipalAnswer, Label: "proposed member", Value: "1",
		Members: []string{"Other"}, SupportRefs: []string{"Other @ other.go:2"}}})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	next := newPreEmitCheckContext(ctx).forAutomaticSourceCitation()
	if next == auto || len(next.principalAggregateMemberSetFactRefsForCheck()) != 1 || len(auto.principalAggregateMemberSetFactRefsForCheck()) != 0 {
		t.Fatal("automatic candidates leaked across immutable emit/patch generations")
	}
}
