package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The completion primer must not override the native tool schema with a
// blanket numeric-value rule: categorical outcomes are intentional values.
// This checks actual prompt assembly and accepted typed facts, not model prose.
func TestCompletionHandoffTeachingAgreesWithAggregateValueKinds(t *testing.T) {
	facts := []types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateTotalCount, Label: "count", Value: "3", Unit: "items"},
		{Kind: types.AnswerAggregateScalar, Label: "latency", Value: "12.5", Unit: "ms"},
		{Kind: types.AnswerAggregateBehaviorOutcome, Label: "behavior", Value: "fail_fast"},
		{Kind: types.AnswerAggregateErrorGranularity, Label: "failure scope", Value: "per_item_rejection"},
	}
	normalized, err := types.NormalizeAnswerAggregateFacts(facts)
	if err != nil || len(normalized) != len(facts) {
		t.Fatalf("typed aggregate examples must be valid before judging teaching: facts=%+v err=%v", normalized, err)
	}
	for i, fact := range normalized {
		if fact.Kind != facts[i].Kind || fact.Value != facts[i].Value || fact.Unit != facts[i].Unit {
			t.Fatalf("typed example changed: got %+v, want %+v", fact, facts[i])
		}
	}
	var schema struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&tool.EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	aggregates := schema.Properties["aggregate_facts"]
	if aggregates.Type != "array" {
		t.Fatalf("native aggregate carrier changed: %q", aggregates.Type)
	}
	for _, fact := range facts {
		found := false
		for _, kind := range aggregates.Items.Properties["kind"].Enum {
			found = found || kind == string(fact.Kind)
		}
		if !found {
			t.Fatalf("schema does not advertise accepted kind %q", fact.Kind)
		}
	}
	prompt := (&explorerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Objective: "explain observed behavior"}, nil)
	for _, forbidden := range []string{"`value` is numeric only", "prose-only summaries use `scalar_value`"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("completion teaching contradicts accepted kinds or narrative ownership: %s", forbidden)
		}
	}
	for _, want := range []string{
		"native JSON array of objects",
		"`behavior_outcome` and `error_granularity_verdict` keep category strings",
		"narrative conclusions belong in `reason`",
		"the count kinds accept integer values only",
		"`kind=\"scalar_value\"`",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("completion teaching missing scoped schema guidance: %s", want)
		}
	}
}
