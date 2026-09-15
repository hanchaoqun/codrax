package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1700SourceLocationToolShapeDoesNotCertifyMembers(t *testing.T) {
	for _, test := range []struct {
		name     string
		edit     func(*types.RequestModel)
		wantNoop bool
	}{
		{"files", nil, true},
		{"sites", func(rm *types.RequestModel) { rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputSites }, true},
		{"inactive", func(rm *types.RequestModel) { rm.ChangeImpactProfile.IsChangeImpact = false }, false},
		{"symbols", func(rm *types.RequestModel) { rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputSymbols }, false},
		{"unknown", func(rm *types.RequestModel) { rm.ChangeImpactProfile.RequestedOutput = types.ImpactOutputUnknown }, false},
		{"bounded", func(rm *types.RequestModel) {
			rm.EnumerationBoundary = &types.RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "one member"}
			rm.AnalyzerHints.ExactTargets = []string{"Worker"}
		}, false},
		{"multi-topic", func(rm *types.RequestModel) {
			rm.Intent, rm.AnalyzerHints, rm.Predicates = "", types.AnalyzerHints{}, types.SemanticPredicates{}
			rm.SubTopics = []types.SubTopic{{Summary: "first", Entities: []string{"First"}}, {Summary: "second", Entities: []string{"Second"}}}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rm := types.RequestModel{Intent: types.IntentEnumerate, AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				Predicates:          types.SemanticPredicates{IsCategoryEnumeration: true},
				ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true, RequestedOutput: types.ImpactOutputFiles}}
			if test.edit != nil {
				test.edit(&rm)
			}
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
				Label: "proposed files", Value: "1", Members: []string{"src/worker.go:12"}, SupportRefs: []string{"src/worker.go:12"}}
			mu := types.NewMutableState("file-output shape")
			mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
			mu.SetInvestigationComplete("model-proposed file set")
			ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: mu}
			before, _ := json.Marshal(mu.StableInvestigationAggregateFacts())
			res, err := (&EmitAnswerSymbol{}).Execute(ctx, json.RawMessage(`{"items":[],"completeness":"unknown"}`))
			if err != nil {
				t.Fatal(err)
			}
			if !res.Success || strings.Contains(res.Summary, "ignored") != test.wantNoop {
				t.Errorf("empty unknown slate no-op=%v want=%v: %s", strings.Contains(res.Summary, "ignored"), test.wantNoop, res.Summary)
			}
			if test.wantNoop && (!strings.Contains(res.Summary, "ignored") || !strings.Contains(res.Summary, "source eligibility is checked separately")) {
				t.Errorf("file/site shape no-op claimed evidence authority: %s", res.Summary)
			}
			if types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &rm, types.AnswerAggregateSourceContextFromBusContext(ctx)) {
				t.Fatal("tool shape certified an unobserved source member")
			}
			after, _ := json.Marshal(mu.StableInvestigationAggregateFacts())
			symbols, _ := mu.EmittedAnswerSymbols()
			if string(before) != string(after) || len(symbols) != 0 {
				t.Fatal("tool shape changed the model proposal or minted a symbol slate")
			}
		})
	}
}
