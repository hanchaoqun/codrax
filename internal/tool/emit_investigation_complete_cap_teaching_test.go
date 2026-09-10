package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Cap guidance must preserve the value-kind contract advertised by the real
// completion schema. A smaller payload does not authorize a different metric.
func TestB1640BCompletionCapTeachingPreservesValueKinds(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			MaxItems    int    `json:"maxItems"`
			Description string `json:"description"`
			Items       struct {
				Type       string `json:"type"`
				Properties map[string]struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	aggregates := schema.Properties["aggregate_facts"]
	if aggregates.Type != "array" || aggregates.Items.Type != "object" || aggregates.MaxItems != types.MaxAnswerAggregateFacts {
		t.Fatalf("native aggregate shape/cap changed: %+v", aggregates)
	}
	if aggregates.Items.Properties["value"].Type != "string" {
		t.Fatal("value must remain a string, with validity determined by its kind")
	}
	wantKinds := []string{"total_count", "unique_count", "grouped_count", "bucket_count", "excluded_count", "scalar_value", "member_set", "negative_search", "negative_observation", "behavior_outcome", "error_granularity_verdict"}
	if got := aggregates.Items.Properties["kind"].Enum; !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("cap teaching must not change kind enum: got %v, want %v", got, wantKinds)
	}
	if strings.Contains(aggregates.Description, "merge same-family per-group scalars into ONE grouped_count") {
		t.Error("cap guidance still coerces scalar measurements into integer counts")
	}
	for _, want := range []string{
		"grouped_count with members only for already-verified non-negative integer counts",
		"never for scalar measurements or categorical outcomes",
		"Keep measurements in scalar_value and preserve categorical kinds",
		"do not change a fact's kind merely to fit the cap",
		"Prioritize currently verified facts within the cap",
	} {
		if !strings.Contains(aggregates.Description, want) {
			t.Errorf("cap guidance missing value-preserving boundary: %q", want)
		}
	}
}

func TestB1640BCompletionCapExamplesUseExistingNormalizer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fact    types.AnswerAggregateFact
		wantErr bool
	}{
		{"integer_group", types.AnswerAggregateFact{Kind: types.AnswerAggregateGroupedCount, Label: "group", Value: "3", Unit: "items", Members: []string{"alpha", "beta", "gamma"}}, false},
		{"integer_total", types.AnswerAggregateFact{Kind: types.AnswerAggregateTotalCount, Label: "total", Value: "3", Unit: "items"}, false},
		{"measurement", types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "latency", Value: "12.5", Unit: "ms"}, false},
		{"behavior_category", types.AnswerAggregateFact{Kind: types.AnswerAggregateBehaviorOutcome, Label: "behavior", Value: "fail_fast"}, false},
		{"failure_category", types.AnswerAggregateFact{Kind: types.AnswerAggregateErrorGranularity, Label: "failure scope", Value: "per_item_rejection"}, false},
		{"measurement_is_not_group_count", types.AnswerAggregateFact{Kind: types.AnswerAggregateGroupedCount, Label: "latency", Value: "12.5", Unit: "ms"}, true},
		{"measurement_is_not_bucket_count", types.AnswerAggregateFact{Kind: types.AnswerAggregateBucketCount, Label: "latency", Value: "12.5", Unit: "ms"}, true},
		{"category_is_not_count", types.AnswerAggregateFact{Kind: types.AnswerAggregateTotalCount, Label: "behavior", Value: "fail_fast"}, true},
		{"negative_is_not_count", types.AnswerAggregateFact{Kind: types.AnswerAggregateGroupedCount, Label: "group", Value: "-1"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []types.AnswerAggregateFact{tc.fact}
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := types.NormalizeAnswerAggregateFacts(input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("existing normalization boundary changed: got=%+v err=%v wantErr=%v", got, err, tc.wantErr)
			}
			if !tc.wantErr && (len(got) != 1 || got[0].Kind != tc.fact.Kind || got[0].Value != tc.fact.Value || got[0].Unit != tc.fact.Unit || !reflect.DeepEqual(got[0].Members, tc.fact.Members)) {
				t.Fatalf("valid fact changed kind/value/unit/members: got %+v, want %+v", got, tc.fact)
			}
			after, err := json.Marshal(input)
			if err != nil || string(before) != string(after) {
				t.Fatalf("normalization mutated submitted facts: before=%s after=%s err=%v", before, after, err)
			}
		})
	}
}
