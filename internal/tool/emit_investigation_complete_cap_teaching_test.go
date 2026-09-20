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

// The public retry and optional-compaction responses must not undo the
// kind-preserving advice in the initial schema. This exercises both responses,
// not only a helper or a source-text match.
func TestCompletionCapRuntimeTeachingAgreesWithPublishedSchema(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	description := schema.Properties["aggregate_facts"].Description
	start := strings.Index(description, "When approaching the cap,")
	end := strings.Index(description, " A named-mechanism comparison is different:")
	if start < 0 || end <= start {
		t.Fatal("published schema lost the cap guidance paragraph")
	}
	guidance := description[start:end]
	for _, optional := range []bool{false, true} {
		name := "rejected_principal_overflow"
		if optional {
			name = "optional_compaction_disclosure"
		}
		t.Run(name, func(t *testing.T) {
			mut := types.NewMutableState("test aggregate capacity without changing value kinds")
			ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{}}
			if optional {
				ctx.AnalysisIR.RequestModel.Intent = types.IntentRootCause
				ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioRootCause
				ctx.AnalysisIR.RequestModel.LogTriage = &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}}
			}
			count := types.MaxAnswerAggregateFacts + 1
			facts := emit2AggregateFacts(count, count)
			// All are scalars, including integral and decimal values. The cap
			// must never be escaped by changing them to grouped integer counts.
			facts[0]["value"] = "12.5"
			params, err := json.Marshal(map[string]any{"result_kind": "resolved", "confidence": "high", "reason": "verified measurements", "aggregate_facts": facts})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
			if err != nil || result.Success != optional {
				t.Fatalf("existing admission boundary changed: optional=%v result=%+v err=%v", optional, result, err)
			}
			if !strings.Contains(result.Summary, guidance) {
				t.Errorf("runtime response contradicts/omits the published kind-preserving guidance:\n%s", result.Summary)
			}
			for _, unsafe := range []string{"merge same-family per-group scalars into ONE grouped_count", "merge same-family facts into one grouped_count"} {
				if strings.Contains(result.Summary, unsafe) {
					t.Errorf("runtime response reintroduces invalid conversion %q", unsafe)
				}
			}
			retained := mut.StableInvestigationAggregateFacts()
			if !optional && len(retained) != 0 {
				t.Fatalf("hard rejection published an aggregate handoff: %+v", retained)
			}
			if optional {
				if len(retained) != types.MaxAnswerAggregateFacts || retained[0].Value != "12.5" || retained[0].Unit != "ms" {
					t.Fatalf("existing disclosed compaction lost value or ruler: %+v", retained)
				}
				for _, fact := range retained {
					if fact.Kind != types.AnswerAggregateScalar {
						t.Fatalf("cap changed measurement kind: %+v", fact)
					}
				}
			}
		})
	}
}
