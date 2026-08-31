package tool

import (
	"encoding/json"
	"reflect"
	"slices"
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
		"question_kind": true, "predicate_axis": true,
		// v4 fail-loud additions: every classification carries its own
		// confidence; the predicates object is the cross-language
		// replacement for the deleted prose-cue tables and must be
		// fully populated.
		"intent_confidence": true, "complexity_confidence": true,
		"kind_confidence":                true,
		"predicates":                     true,
		"diagnostic_profile":             true,
		"answer_role_profile":            true,
		"error_granularity_profile":      true,
		"requested_answer_dimensions":    true,
		"runtime_selection_profile":      true,
		"runtime_artifact_scope_profile": true,
		"runtime_target_profile":         true,
		"runtime_question_profile":       true,
		"history_selection_profile":      true,
		"completeness_obligation":        true,
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

func TestEmitAnalysisSchemaSeparatesRuntimeDimensionDecisionFromScopeConsequence(t *testing.T) {
	var parsed struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	runtimeProfile := parsed.Properties["runtime_question_profile"].(map[string]any)
	if got := runtimeProfile["description"].(string); strings.Count(got, skill.AnalysisRuntimeScopeSchemaTeaching) != 1 || strings.Contains(got, skill.AnalysisRuntimeScopeFromDimensionTeaching) {
		t.Fatalf("runtime profile schema must carry one compact scope tuple, not duplicate the workflow: %q", got)
	}
	dimensions := parsed.Properties["requested_answer_dimensions"].(map[string]any)
	dimensionItems := dimensions["properties"].(map[string]any)["dimensions"].(map[string]any)["items"].(map[string]any)
	roleDescription := dimensionItems["properties"].(map[string]any)["role"].(map[string]any)["description"].(string)
	if strings.Count(roleDescription, skill.AnalysisRuntimeDimensionSchemaTeaching) != 1 || strings.Contains(roleDescription, skill.AnalysisRuntimeCausalAttributionTeaching) {
		t.Fatalf("dimension-role schema must carry one compact role reminder, not duplicate the workflow: %q", roleDescription)
	}
	for _, want := range []string{"Passive wording", "was limited, constrained, or affected", "condition class is not yet known"} {
		if !strings.Contains(roleDescription, want) {
			t.Fatalf("dimension-role schema lost finite passive-effect guidance %q: %q", want, roleDescription)
		}
	}
	for _, want := range []string{
		"runtime_work_relation",
		"measured or to-be-discovered runtime work items/spans/operations from a requested work class",
		"work identity may be an investigation output",
		"exact relation credential and causal boundary",
		"relation_path owns only a separately requested topology, endpoint, or hop sequence",
		"Emit both when both visible surfaces are requested",
	} {
		if !strings.Contains(roleDescription, want) {
			t.Fatalf("dimension-role schema lost runtime-work relation guidance %q: %q", want, roleDescription)
		}
	}
	if strings.Contains(roleDescription, skill.AnalysisRuntimeScopeFromDimensionTeaching) {
		t.Fatalf("dimension-role schema must not repeat the runtime scope consequence: %q", roleDescription)
	}
	predicates := parsed.Properties["predicates"].(map[string]any)
	predicateProperties := predicates["properties"].(map[string]any)
	for field, want := range map[string]string{
		"is_scalar_answer":  "ENTIRE principal answer",
		"is_count_question": "mixed/dimensioned answer merely includes one duration, count, percentage, or total",
	} {
		description := predicateProperties[field].(map[string]any)["description"].(string)
		if !strings.Contains(description, want) {
			t.Fatalf("%s schema description lost mixed-answer boundary %q: %q", field, want, description)
		}
	}
}

func TestEmitAnalysisExecutorRejectsProviderSchemaRequiredFieldOmission(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	complete := map[string]any{
		"intent": "explain", "scenario": "architecture_explain", "complexity": "complex",
		"keywords": []string{"pipeline", "data flow"}, "entities": []string{"Analyzer", "Explorer"},
		"question_kind": "mechanism", "predicate_axis": "flow",
		"intent_confidence": 0.9, "complexity_confidence": 0.9, "kind_confidence": 0.9,
		"predicates": map[string]any{
			"is_scalar_answer": false, "is_role_locate_lookup": false, "is_count_question": false,
			"is_cross_component": true, "is_relational_lookup": true, "is_category_enumeration": false,
			"is_history_lookup": false, "is_diagnostic_question": false, "has_per_member_table": false,
		},
		"diagnostic_profile": map[string]any{
			"is_diagnostic": false, "current_risk": false, "historical_regression": false,
			"current_version_check": false, "confidence": 0.9,
		},
		"answer_role_profile":         map[string]any{"is_role_binding_requested": false, "confidence": 0.9},
		"error_granularity_profile":   map[string]any{"is_granularity_question": false, "confidence": 0.9},
		"requested_answer_dimensions": map[string]any{"is_dimensioned_answer": false, "confidence": 0.9},
		"runtime_artifact_scope_profile": map[string]any{
			"requested_scope": "not_applicable", "confidence": 0.9,
		},
		"runtime_target_profile":   map[string]any{"declaration": "not_applicable", "confidence": 0.9},
		"runtime_question_profile": map[string]any{"scope": "not_applicable", "confidence": 0.9},
		"history_selection_profile": map[string]any{
			"mode": "not_applicable", "item_kind": "not_applicable", "confidence": 0.9,
		},
		"completeness_obligation": map[string]any{"required": false, "source_quote": ""},
		"diagram_hint": map[string]any{
			"kind": "flow", "required": true,
			"relation_scope_quote": "Analyzer 到 Explorer 的数据流",
			"participants": []map[string]any{
				{"identity": "Analyzer", "role": "incident_required", "source_quote": "Analyzer"},
				{"identity": "Explorer", "role": "incident_required", "source_quote": "Explorer"},
			},
		},
		"call_chain_endpoints": map[string]any{
			"source": "", "sink": "", "sink_mode": "exact",
			"runtime_selection_required": false, "runtime_selection_source_quote": "",
		},
		"runtime_selection_profile": map[string]any{
			"is_selection_question": false, "source_quote": "", "confidence": 0.9,
		},
	}
	for _, missing := range []string{"question_kind", "predicate_axis"} {
		t.Run(missing, func(t *testing.T) {
			payload := make(map[string]any, len(complete)-1)
			for key, value := range complete {
				if key != missing {
					payload[key] = value
				}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			ctx := &types.BusContext{
				Mutable:                     types.NewMutableState("Analyzer 到 Explorer 的数据流"),
				PresentationDiagramRequired: true,
			}
			res, err := (&EmitAnalysis{}).Execute(ctx, raw)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if res.Success || !strings.Contains(res.Summary, "missing required top-level field(s): "+missing) {
				t.Fatalf("missing %s must fail before zero-value normalization: %+v", missing, res)
			}
			if ctx.Mutable.RequestModel() != nil {
				t.Fatalf("missing %s must not persist a degraded RequestModel", missing)
			}
		})
	}

	t.Run("unauthorized_required_diagram_does_not_mint_axis_gate", func(t *testing.T) {
		payload := make(map[string]any, len(complete)-1)
		for key, value := range complete {
			if key != "predicate_axis" {
				payload[key] = value
			}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState("Analyzer 到 Explorer 的数据流")}
		res, err := (&EmitAnalysis{}).Execute(ctx, raw)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if !res.Success {
			t.Fatalf("unauthorized model required=true must soften before predicate-axis presence gate: %+v", res)
		}
		if rm := ctx.Mutable.RequestModel(); rm == nil || rm.DiagramHint == nil || rm.DiagramHint.Required {
			t.Fatalf("unauthorized required diagram did not soften as expected: %+v", rm)
		}
	})
}

func TestEmitAnalysisSchemaDeclaresCallChainEndpointDirectionAsSingleSource(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v", err)
	}
	raw, ok := parsed.Properties["call_chain_endpoints"]
	if !ok {
		t.Fatal("emit_analysis schema is missing call_chain_endpoints")
	}
	var prop struct {
		Description string                     `json:"description"`
		Properties  map[string]json.RawMessage `json:"properties"`
		Required    []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		t.Fatalf("call_chain_endpoints schema is invalid: %v", err)
	}
	if !slices.Equal(prop.Required, []string{"source", "sink", "sink_mode"}) {
		t.Fatalf("call_chain_endpoints.required=%v", prop.Required)
	}
	if _, ok := prop.Properties["source"]; !ok {
		t.Fatal("call_chain_endpoints is missing source")
	}
	if _, ok := prop.Properties["sink"]; !ok {
		t.Fatal("call_chain_endpoints is missing sink")
	}
	if _, ok := prop.Properties["sink_mode"]; !ok {
		t.Fatal("call_chain_endpoints is missing sink_mode")
	}
	for _, removed := range []string{"runtime_selection_required", "runtime_selection_source_quote"} {
		if _, ok := prop.Properties[removed]; ok {
			t.Fatalf("call_chain_endpoints must not retain independent selection field %q", removed)
		}
	}
	for _, want := range []string{"ONLY field", "entities", "exact_targets", "unordered", "discover_path"} {
		if !strings.Contains(prop.Description, want) {
			t.Fatalf("call_chain_endpoints description must pin %q: %s", want, prop.Description)
		}
	}
	if !strings.Contains(prop.Description, types.CallChainEndpointProfileTeaching) {
		t.Fatalf("call_chain_endpoints schema must consume the single teaching source: %s", prop.Description)
	}
	if !strings.Contains(prop.Description, types.CallChainEndpointLowMindRule) {
		t.Fatalf("call_chain_endpoints schema must front-load the inert non-call-chain shape: %s", prop.Description)
	}
}

func TestEmitAnalysisSchemaDeclaresDedicatedRuntimeSelectionProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v", err)
	}
	if !slices.Contains(parsed.Required, "runtime_selection_profile") {
		t.Fatalf("runtime_selection_profile is not provider-required: %v", parsed.Required)
	}
	raw, ok := parsed.Properties["runtime_selection_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing runtime_selection_profile")
	}
	var prop struct {
		Description string                     `json:"description"`
		Properties  map[string]json.RawMessage `json:"properties"`
		Required    []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		t.Fatalf("runtime_selection_profile schema is invalid: %v", err)
	}
	if !slices.Equal(prop.Required, []string{"is_selection_question", "source_quote", "confidence"}) {
		t.Fatalf("runtime_selection_profile.required=%v", prop.Required)
	}
	for _, field := range prop.Required {
		if _, ok := prop.Properties[field]; !ok {
			t.Fatalf("runtime_selection_profile missing property %q", field)
		}
	}
	for _, want := range []string{
		"one stated discriminator value",
		"initial/full output",
		"retry/error/patch",
		"sink=\"\", sink_mode=discover",
		"never fill sink from repository pre-scan/search/evidence",
		"independent of call-chain",
	} {
		if !strings.Contains(prop.Description, want) {
			t.Fatalf("runtime_selection_profile teaching missing %q: %s", want, prop.Description)
		}
	}
}

func TestEmitAnalysisSchemaIncludesRuntimeArtifactScopeProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	propRaw, ok := parsed.Properties["runtime_artifact_scope_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"runtime_artifact_scope_profile\"")
	}
	var prop struct {
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("runtime_artifact_scope_profile property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	var gotRequired = map[string]bool{}
	for _, field := range prop.Required {
		gotRequired[field] = true
	}
	for _, field := range []string{"requested_scope", "confidence"} {
		if !gotRequired[field] {
			t.Fatalf("runtime_artifact_scope_profile.required=%v, missing %s", prop.Required, field)
		}
	}
	var want []string
	for _, scope := range types.AllRuntimeArtifactRequestedScopes() {
		want = append(want, string(scope))
	}
	if !reflect.DeepEqual(prop.Properties["requested_scope"].Enum, want) {
		t.Fatalf("requested_scope enum=%v, want %v", prop.Properties["requested_scope"].Enum, want)
	}
	if prop.Properties["time_start"].Type != "number" || prop.Properties["time_end"].Type != "number" {
		t.Fatalf("explicit window bounds must be numeric: %+v", prop.Properties)
	}
}

func TestEmitAnalysisSchemaIncludesHistorySelectionProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v", err)
	}
	propRaw, ok := parsed.Properties["history_selection_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing history_selection_profile")
	}
	var prop struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("history_selection_profile schema: %v", err)
	}
	var wantModes []string
	for _, value := range types.AllHistorySelectionModes() {
		wantModes = append(wantModes, string(value))
	}
	if !reflect.DeepEqual(prop.Properties["mode"].Enum, wantModes) {
		t.Fatalf("history selection mode enum=%v, want %v", prop.Properties["mode"].Enum, wantModes)
	}
	var wantKinds []string
	for _, value := range types.AllHistorySelectionItemKinds() {
		wantKinds = append(wantKinds, string(value))
	}
	if !reflect.DeepEqual(prop.Properties["item_kind"].Enum, wantKinds) {
		t.Fatalf("history selection item kind enum=%v, want %v", prop.Properties["item_kind"].Enum, wantKinds)
	}
	for _, field := range []string{"mode", "item_kind", "confidence"} {
		if !slices.Contains(prop.Required, field) {
			t.Fatalf("history_selection_profile.required=%v missing %s", prop.Required, field)
		}
	}
}

func TestEmitAnalysisSchemaRequiresPredicateAxisAndTeachesFlow(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	raw := (&EmitAnalysis{}).Parameters()
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("emit_analysis schema is not valid JSON: %v\nraw=%s", err, string(raw))
	}
	if !slices.Contains(parsed.Required, "predicate_axis") {
		t.Fatalf("predicate_axis must be explicit in every analyzer JSON object; required=%v", parsed.Required)
	}
	propRaw, ok := parsed.Properties["predicate_axis"]
	if !ok {
		t.Fatal("emit_analysis schema is missing property \"predicate_axis\"")
	}
	var prop struct {
		Enum        []string `json:"enum"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("predicate_axis property is not valid JSON schema: %v", err)
	}
	if !reflect.DeepEqual(prop.Enum, skill.AnalysisPredicateAxisValues()) {
		t.Fatalf("predicate_axis enum drift: schema=%v contract=%v", prop.Enum, skill.AnalysisPredicateAxisValues())
	}
	for _, want := range []string{"flow", "value", "state", "control", "Empty only when"} {
		if !strings.Contains(prop.Description, want) {
			t.Fatalf("predicate_axis schema teaching missing %q in %q", want, prop.Description)
		}
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
		Type        string `json:"type"`
		Description string `json:"description"`
		Properties  map[string]struct {
			Type        string   `json:"type"`
			Enum        []string `json:"enum"`
			Description string   `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatalf("diagram_hint property is not valid JSON schema: %v\nraw=%s", err, string(propRaw))
	}
	if prop.Type != "object" {
		t.Fatalf("diagram_hint type = %q, want object", prop.Type)
	}
	for _, want := range []string{"explicitly requested visual modality is authoritative", "sequence even when the topic is a call chain", "Do not replace an explicit sequence request with call_dag"} {
		if !strings.Contains(prop.Description, want) {
			t.Fatalf("diagram_hint schema description must preserve explicit modality; missing %q in %q", want, prop.Description)
		}
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
	requiredProp, ok := prop.Properties["required"]
	if !ok || requiredProp.Type != "boolean" {
		t.Fatalf("diagram_hint.required missing boolean property: %+v", prop.Properties)
	}
	if !strings.Contains(requiredProp.Description, "explicit current-turn visual request") {
		t.Fatalf("diagram_hint.required description lacks authority boundary: %q", requiredProp.Description)
	}
	if !reflect.DeepEqual(prop.Required, []string{"kind", "required", "relation_scope_quote", "participants"}) {
		t.Fatalf("diagram_hint required = %v, want [kind required relation_scope_quote participants]", prop.Required)
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

func TestEmitAnalysisSchemaIncludesRuntimeTargetProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatal(err)
	}
	propRaw, ok := parsed.Properties["runtime_target_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing runtime_target_profile")
	}
	var prop struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatal(err)
	}
	want := []string{"not_applicable", "no_named_target", "named_target", "unspecified"}
	if !reflect.DeepEqual(prop.Properties["declaration"].Enum, want) {
		t.Fatalf("runtime_target_profile declaration enum=%v want=%v", prop.Properties["declaration"].Enum, want)
	}
	for _, required := range []string{"declaration", "confidence"} {
		if !slices.Contains(prop.Required, required) {
			t.Fatalf("runtime_target_profile.required=%v missing %s", prop.Required, required)
		}
	}
}

func TestEmitAnalysisSchemaIncludesRuntimeQuestionProfile(t *testing.T) {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &parsed); err != nil {
		t.Fatal(err)
	}
	propRaw, ok := parsed.Properties["runtime_question_profile"]
	if !ok {
		t.Fatal("emit_analysis schema is missing runtime_question_profile")
	}
	var prop struct {
		Properties map[string]struct {
			Enum  []string `json:"enum"`
			Items *struct {
				Enum []string `json:"enum"`
			} `json:"items,omitempty"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(propRaw, &prop); err != nil {
		t.Fatal(err)
	}
	want := []string{"not_applicable", "bounded_fact_set", "bounded_effect_verdict", "causal_diagnosis", "relation_analysis", "system_overview", "unspecified"}
	if !reflect.DeepEqual(prop.Properties["scope"].Enum, want) {
		t.Fatalf("runtime_question_profile scope enum=%v want=%v", prop.Properties["scope"].Enum, want)
	}
	wantFamilies := runtimeQuestionFactFamilyValues()
	families := prop.Properties["fact_families"]
	if families.Items == nil || !reflect.DeepEqual(families.Items.Enum, wantFamilies) {
		t.Fatalf("runtime_question_profile fact_families enum=%v want=%v", families.Items, wantFamilies)
	}
	for _, want := range []string{
		"Choose the most specific semantic fact family before a generic unit family",
		"`count_or_duration` is only the generic count/duration family when no named family above owns that measurement",
		"both `target_wait_occurrences` and `io_latency`",
	} {
		if !strings.Contains(string(propRaw), want) {
			t.Fatalf("runtime_question_profile fact-family schema teaching missing %q", want)
		}
	}
	for _, required := range []string{"scope", "confidence"} {
		if !slices.Contains(prop.Required, required) {
			t.Fatalf("runtime_question_profile.required=%v missing %s", prop.Required, required)
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
