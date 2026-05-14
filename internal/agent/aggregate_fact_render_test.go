package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderStructuredAggregateFactsPrioritizesPrincipalMemberSet(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 18; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("candidate bucket %02d", i),
			Value: fmt.Sprintf("%d", i),
		})
	}
	facts = append(facts, types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "verified qualifying members",
		Value:   "1",
		Members: []string{"explorer"},
	})

	got := renderStructuredAggregateFacts(facts, 16)
	if !strings.Contains(got, "label=verified qualifying members") ||
		!strings.Contains(got, "principal_member_set=true") ||
		!strings.Contains(got, "members=[`explorer`]") {
		t.Fatalf("principal member_set should survive prompt projection:\n%s", got)
	}
	if strings.Index(got, "label=verified qualifying members") > strings.Index(got, "label=candidate bucket 00") {
		t.Fatalf("principal member_set should render before lower-priority count facts:\n%s", got)
	}
}

func TestStructuredAggregatePromptFactLimitExpandsForComplexTypedQuestions(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 24; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("bucket %02d", i),
			Value: fmt.Sprintf("%d", i),
		})
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Complexity: types.ComplexityComplex,
		Predicates: types.SemanticPredicates{
			IsCrossComponent:      true,
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		SubTopics: []types.SubTopic{{Summary: "left"}, {Summary: "right"}},
	}}}

	if got := structuredAggregatePromptFactLimit(ctx, facts); got <= structuredAggregateDefaultPromptFacts {
		t.Fatalf("complex typed questions should expand aggregate prompt budget, got %d", got)
	}
}

func TestStructuredAggregatePromptFactLimitDoesNotOmitPrincipalMemberSetRows(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < structuredAggregateMaxPromptFacts+3; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   fmt.Sprintf("principal set %02d", i),
			Value:   "1",
			Members: []string{fmt.Sprintf("member-%02d", i)},
		})
	}

	got := structuredAggregatePromptFactLimit(nil, facts)
	if got != len(facts) {
		t.Fatalf("principal member_set fact rows should not be capped below principal count, got %d want %d", got, len(facts))
	}
}
