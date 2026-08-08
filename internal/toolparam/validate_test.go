package toolparam

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRejectsNestedEnumAfterStringArrayNormalization(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "actions":{"type":"array","items":{"type":"object","properties":{
	      "kind":{"type":"string","enum":["derive_rules"]}
	    },"required":["kind"],"additionalProperties":false}}
	  },
	  "required":["actions"],
	  "additionalProperties":false
	}`)
	raw := json.RawMessage(`{"actions":"[{\"kind\":\"assemble_answer\"}]"}`)
	normalized, report := Normalize(raw, schema, repairPolicy)
	if !report.Changed() {
		t.Fatal("fixture must exercise string-to-array compatibility normalization")
	}
	err := Validate(normalized, schema)
	if err == nil || !strings.Contains(err.Error(), "$.actions[0].kind") || !strings.Contains(err.Error(), "enum") {
		t.Fatalf("normalized nested enum violation must fail at the exact schema path: err=%v raw=%s", err, normalized)
	}
}

func TestValidateAcceptsSchemaValidNormalizedNestedArray(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "actions":{"type":"array","maxItems":1,"items":{"type":"object","properties":{
	      "kind":{"type":"string","enum":["derive_rules"]}
	    },"required":["kind"],"additionalProperties":false}}
	  },
	  "required":["actions"],
	  "additionalProperties":false
	}`)
	raw := json.RawMessage(`{"actions":"[{\"kind\":\"derive_rules\"}]"}`)
	normalized, report := Normalize(raw, schema, repairPolicy)
	if !report.Changed() {
		t.Fatal("fixture must exercise string-to-array compatibility normalization")
	}
	if err := Validate(normalized, schema); err != nil {
		t.Fatalf("schema-valid normalized payload was rejected: %v raw=%s", err, normalized)
	}
}

func TestValidateRepairsChecksChangedSubtreeWithoutUpgradingUnrelatedRequiredFields(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "actions":{"type":"array","items":{"type":"object","properties":{
	      "kind":{"type":"string","enum":["derive_rules"]}
	    },"required":["kind"]}},
	    "new_required_field":{"type":"string"}
	  },
	  "required":["actions","new_required_field"]
	}`)
	for _, tc := range []struct {
		name    string
		kind    string
		wantErr bool
	}{
		{name: "valid changed subtree", kind: "derive_rules"},
		{name: "invalid nested enum", kind: "assemble_answer", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(`{"actions":"[{\"kind\":\"` + tc.kind + `\"}]"}`)
			normalized, report := Normalize(raw, schema, repairPolicy)
			err := ValidateRepairs(normalized, schema, report)
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "$.actions[0].kind")) {
				t.Fatalf("changed subtree violation not detected: %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unrelated missing required field must not be upgraded by one subtree repair: %v", err)
			}
		})
	}
}

func TestValidateAppliesRequiredAdditionalPropertiesAndConditionalBounds(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{"kind":{"type":"string"},"script":{"type":"string"}},
	  "required":["kind"],
	  "additionalProperties":false,
	  "allOf":[{"if":{"properties":{"kind":{"const":"custom"}},"required":["kind"]},
	    "then":{"required":["script"],"properties":{"script":{"minLength":1}}},
	    "else":{"properties":{"script":{"maxLength":0}}}}]
	}`)
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "missing", raw: `{}`, want: "$.kind"},
		{name: "extra", raw: `{"kind":"plain","extra":1}`, want: "$.extra"},
		{name: "then", raw: `{"kind":"custom"}`, want: "$.script"},
		{name: "else", raw: `{"kind":"plain","script":"x"}`, want: "$.script"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(json.RawMessage(tc.raw), schema)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected precise violation at %s, got %v", tc.want, err)
			}
		})
	}
	if err := Validate(json.RawMessage(`{"kind":"custom","script":"x"}`), schema); err != nil {
		t.Fatalf("valid conditional payload rejected: %v", err)
	}
}
