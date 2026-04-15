package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// TestEmitAnalysisSchemaMatchesContract is the single consistency
// guardrail between the SSOT in internal/skill/analysis_contract.go
// and the JSON schema emit_analysis hands the LLM.
//
// It parses the live schema via EmitAnalysis.Parameters(), extracts
// each classification field's enum array, and asserts it deep-equals
// the canonical slice returned by skill.Analysis*Values(). Any future
// regression that re-introduces hardcoded enum literals on either
// side of the boundary must fail this test loudly.
func TestEmitAnalysisSchemaMatchesContract(t *testing.T) {
	var parsed struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}

	cases := []struct {
		field  string
		values []string
	}{
		{"intent", skill.AnalysisIntentValues()},
		{"scenario", skill.AnalysisScenarioValues()},
		{"complexity", skill.AnalysisComplexityValues()},
		{"question_kind", skill.AnalysisQuestionKindValues()},
		{"answer_shape", skill.AnalysisAnswerShapeValues()},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			prop, ok := parsed.Properties[tc.field]
			if !ok {
				t.Fatalf("emit_analysis schema is missing property %q", tc.field)
			}
			if prop.Type != "string" {
				t.Errorf("%s type = %q, want string", tc.field, prop.Type)
			}
			if !reflect.DeepEqual(prop.Enum, tc.values) {
				t.Errorf("%s enum drift:\n  schema:   %v\n  contract: %v",
					tc.field, prop.Enum, tc.values)
			}
		})
	}

	// Required-field set sanity check — if a required field is ever
	// removed from the schema without also pruning the agent-side
	// pipeline, this fails and forces a deliberate fix.
	wantRequired := map[string]bool{
		"intent": true, "scenario": true, "complexity": true,
		"keywords": true, "entities": true,
		"question_kind": true, "answer_shape": true,
	}
	gotRequired := make(map[string]bool, len(parsed.Required))
	for _, r := range parsed.Required {
		gotRequired[r] = true
	}
	if !reflect.DeepEqual(gotRequired, wantRequired) {
		t.Errorf("emit_analysis required-field set drift:\n  schema:   %v\n  expected: %v",
			parsed.Required, wantRequired)
	}
}
