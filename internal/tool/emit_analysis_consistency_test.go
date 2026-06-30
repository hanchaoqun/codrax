package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
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
		"kind_confidence":           true,
		"predicates":                true,
		"diagnostic_profile":        true,
		"answer_role_profile":       true,
		"error_granularity_profile": true,
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
	relationDesc := prop.Properties["is_relational_lookup"].Description
	for _, want := range []string{
		"filtering/selecting/counting members of source set X",
		"relationship to target Y",
		"role/category members that can invoke capability Y",
		"is_category_enumeration=true",
		"is_count_question=true",
	} {
		if !strings.Contains(relationDesc, want) {
			t.Errorf("predicates.is_relational_lookup schema description missing %q; got: %q", want, relationDesc)
		}
	}
	if strings.Contains(relationDesc, "propose_sub_agents") || strings.Contains(relationDesc, "SubExplorer") {
		t.Fatalf("relation schema description must remain generic, got: %q", relationDesc)
	}
}

func TestEmitAnalysisSchemaRequiresDiagnosticProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["diagnostic_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"diagnostic_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("diagnostic_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_diagnostic", "current_risk", "historical_regression", "current_version_check", "confidence"} {
		if _, ok := prop.Properties[want]; !ok {
			t.Fatalf("diagnostic_profile.%s missing from schema", want)
		}
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("diagnostic_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	if _, ok := prop.Properties["observation_summary"]; !ok {
		t.Fatal("diagnostic_profile.observation_summary missing from schema")
	}
	desc := prop.Properties["current_version_check"].Description
	if !strings.Contains(desc, "verify current code") {
		t.Errorf("diagnostic_profile.current_version_check schema description should mention current-code verification; got: %q", desc)
	}
}

func TestEmitAnalysisSchemaIncludesConversationReferenceProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["conversation_reference_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"conversation_reference_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("conversation_reference_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"requires_prior_context", "needs_repo_verification", "ambiguity"} {
		if _, ok := prop.Properties[want]; !ok {
			t.Fatalf("conversation_reference_profile.%s missing from schema", want)
		}
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("conversation_reference_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	if !reflect.DeepEqual(prop.Properties["ambiguity"].Enum, []string{"none", "ambiguous", "missing"}) {
		t.Fatalf("conversation_reference_profile.ambiguity enum = %v", prop.Properties["ambiguity"].Enum)
	}
}

func TestEmitAnalysisSchemaIncludesSourceScopeProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["source_scope_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"source_scope_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("source_scope_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"requested_scope", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("source_scope_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, scope := range types.AllSourceScopes() {
		wantEnum = append(wantEnum, string(scope))
	}
	if !reflect.DeepEqual(prop.Properties["requested_scope"].Enum, wantEnum) {
		t.Fatalf("source_scope_profile.requested_scope enum = %v, want %v", prop.Properties["requested_scope"].Enum, wantEnum)
	}
	if prop.Properties["source_quotes"].Type != "array" {
		t.Fatalf("source_scope_profile.source_quotes type = %q, want array", prop.Properties["source_quotes"].Type)
	}
}

func TestEmitAnalysisSchemaIncludesSourceInventoryProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["source_inventory_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"source_inventory_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type  string   `json:"type"`
			Enum  []string `json:"enum"`
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("source_inventory_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_source_inventory", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("source_inventory_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantRoles []string
	for _, role := range types.AllAnswerCandidateRoles() {
		wantRoles = append(wantRoles, string(role))
	}
	if !reflect.DeepEqual(prop.Properties["target_roles"].Items.Enum, wantRoles) {
		t.Fatalf("source_inventory_profile.target_roles enum = %v, want %v",
			prop.Properties["target_roles"].Items.Enum, wantRoles)
	}
	var wantUnderlying []string
	for _, underlying := range types.AllSourceInventoryTypeUnderlyings() {
		wantUnderlying = append(wantUnderlying, string(underlying))
	}
	if !reflect.DeepEqual(prop.Properties["type_underlying"].Enum, wantUnderlying) {
		t.Fatalf("source_inventory_profile.type_underlying enum = %v, want %v",
			prop.Properties["type_underlying"].Enum, wantUnderlying)
	}
}

func TestEmitAnalysisSchemaIncludesFieldValueProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["field_value_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"field_value_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("field_value_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_field_value_lookup", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("field_value_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, kind := range types.AllFieldValueLiteralKinds() {
		wantEnum = append(wantEnum, string(kind))
	}
	if !reflect.DeepEqual(prop.Properties["literal_kind"].Enum, wantEnum) {
		t.Fatalf("field_value_profile.literal_kind enum = %v, want %v", prop.Properties["literal_kind"].Enum, wantEnum)
	}
}

func TestEmitAnalysisSchemaIncludesRuntimeArtifactValueProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["artifact_value_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"artifact_value_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("artifact_value_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_artifact_value_lookup", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("artifact_value_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, kind := range types.AllFieldValueLiteralKinds() {
		wantEnum = append(wantEnum, string(kind))
	}
	if !reflect.DeepEqual(prop.Properties["literal_kind"].Enum, wantEnum) {
		t.Fatalf("artifact_value_profile.literal_kind enum = %v, want %v", prop.Properties["literal_kind"].Enum, wantEnum)
	}
}

func TestEmitAnalysisSchemaIncludesRuntimeTargets(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["runtime_targets"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"runtime_targets\"")
	}
	var prop struct {
		Type  string `json:"type"`
		Items struct {
			Properties map[string]struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"properties"`
			Required []string `json:"required"`
		} `json:"items"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("runtime_targets property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	if prop.Type != "array" {
		t.Fatalf("runtime_targets.type = %q, want array", prop.Type)
	}
	if !reflect.DeepEqual(prop.Items.Properties["kind"].Enum, []string{"process", "thread"}) {
		t.Fatalf("runtime_targets.kind enum = %v, want [process thread]", prop.Items.Properties["kind"].Enum)
	}
	for _, want := range []string{"kind", "confidence"} {
		found := false
		for _, field := range prop.Items.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("runtime_targets.items.required = %v, want %s included", prop.Items.Required, want)
		}
	}
}

func TestEmitAnalysisSchemaIncludesAnswerExclusionPolicy(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["answer_exclusion_policy"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"answer_exclusion_policy\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("answer_exclusion_policy property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_exclusion_requested", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("answer_exclusion_policy.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, role := range types.AllAnswerCandidateRoles() {
		wantEnum = append(wantEnum, string(role))
	}
	if !reflect.DeepEqual(prop.Properties["excluded_candidate_roles"].Items.Enum, wantEnum) {
		t.Fatalf("answer_exclusion_policy.excluded_candidate_roles enum = %v, want %v",
			prop.Properties["excluded_candidate_roles"].Items.Enum, wantEnum)
	}
}

func TestEmitAnalysisSchemaIncludesAnswerRoleProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["answer_role_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"answer_role_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("answer_role_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_role_binding_requested", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("answer_role_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, role := range types.AllAnswerCandidateRoles() {
		wantEnum = append(wantEnum, string(role))
	}
	if !reflect.DeepEqual(prop.Properties["required_candidate_roles"].Items.Enum, wantEnum) {
		t.Fatalf("answer_role_profile.required_candidate_roles enum = %v, want %v",
			prop.Properties["required_candidate_roles"].Items.Enum, wantEnum)
	}
}

func TestEmitAnalysisSchemaIncludesCurrentSourceExplanationProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["current_source_explanation_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"current_source_explanation_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("current_source_explanation_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_current_source_explanation_requested", "confidence"} {
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("current_source_explanation_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	var wantEnum []string
	for _, mode := range types.AllCurrentSourceExplanationModes() {
		wantEnum = append(wantEnum, string(mode))
	}
	if !reflect.DeepEqual(prop.Properties["modes"].Items.Enum, wantEnum) {
		t.Fatalf("current_source_explanation_profile.modes enum = %v, want %v",
			prop.Properties["modes"].Items.Enum, wantEnum)
	}
}

func TestEmitAnalysisSchemaIncludesErrorGranularityProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["error_granularity_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"error_granularity_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("error_granularity_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	for _, want := range []string{"is_granularity_question", "confidence"} {
		if _, ok := prop.Properties[want]; !ok {
			t.Fatalf("error_granularity_profile.%s missing from schema", want)
		}
		found := false
		for _, field := range prop.Required {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("error_granularity_profile.required = %v, want %s included", prop.Required, want)
		}
	}
	if _, ok := prop.Properties["source_quotes"]; !ok {
		t.Fatal("error_granularity_profile.source_quotes missing from schema")
	}
	var wantEnum []string
	for _, verdict := range types.AllErrorGranularityVerdicts() {
		if verdict == types.ErrorGranularityNotEnoughEvidence {
			continue
		}
		wantEnum = append(wantEnum, string(verdict))
	}
	if !reflect.DeepEqual(prop.Properties["requested_verdict_options"].Items.Enum, wantEnum) {
		t.Fatalf("error_granularity_profile.requested_verdict_options enum = %v, want %v",
			prop.Properties["requested_verdict_options"].Items.Enum, wantEnum)
	}
}
