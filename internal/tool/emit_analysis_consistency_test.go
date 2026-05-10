package tool

import (
	"encoding/json"
	"reflect"
	"strings"
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
		"question_kind": true,
		// v4 fail-loud additions: every classification carries its own
		// confidence; the predicates object is the cross-language
		// replacement for the deleted prose-cue tables and must be
		// fully populated.
		"intent_confidence": true, "complexity_confidence": true,
		"kind_confidence": true,
		"predicates":      true,
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

func TestEmitAnalysisSchemaIncludesDiagramHintEnum(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["diagram_hint"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"diagram_hint\"")
	}
	var prop struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("diagram_hint property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	if prop.Type != "object" {
		t.Fatalf("diagram_hint type = %q, want object", prop.Type)
	}
	kindProp, ok := prop.Properties["kind"]
	if !ok {
		t.Fatal("diagram_hint.kind missing from schema")
	}
	if kindProp.Type != "string" {
		t.Fatalf("diagram_hint.kind type = %q, want string", kindProp.Type)
	}
	if !reflect.DeepEqual(kindProp.Enum, skill.AnalysisDiagramKindValues()) {
		t.Fatalf("diagram_hint.kind enum drift:\n  schema:   %v\n  contract: %v", kindProp.Enum, skill.AnalysisDiagramKindValues())
	}
	if len(prop.Required) != 1 || prop.Required[0] != "kind" {
		t.Fatalf("diagram_hint required = %v, want [kind]", prop.Required)
	}
}

func TestEmitAnalysisSchemaRequiresSemanticPredicates(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["predicates"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"predicates\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("predicates property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_role_locate_lookup", "is_diagnostic_question"} {
		if _, ok := prop.Properties[want]; !ok {
			t.Fatalf("predicates.%s missing from schema", want)
		}
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("predicates.required = %v, want %s included", prop.Required, want)
		}
	}
	desc := prop.Properties["is_diagnostic_question"].Description
	for _, want := range []string{"similar problem still exists", "current-risk", "with or without an attached runtime artifact"} {
		if !strings.Contains(desc, want) {
			t.Errorf("predicates.is_diagnostic_question schema description missing %q; got: %q", want, desc)
		}
	}
}
