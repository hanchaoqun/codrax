package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// withV4Required injects the schema-v4 required fields (predicates +
// per-classification confidences) into a partial JSON payload that
// only sets the v3 fields. Tests that need to exercise different v4
// values inline them directly instead. Implemented as a string append
// so the fixture syntax stays raw-JSON for readability.
//
// The defaults express a conservative, "no special predicates fired,
// LLM is moderately confident" baseline so the call passes validation
// without forcing every test to think about v4 semantics it does not
// care about.
const v4DefaultsJSON = `,
	"intent_confidence": 0.7,
	"complexity_confidence": 0.7,
	"kind_confidence": 0.7,
	"predicates": {
		"is_scalar_answer": false,
		"is_role_locate_lookup": false,
		"is_count_question": false,
		"is_cross_component": false,
		"is_relational_lookup": false,
		"is_category_enumeration": false,
		"is_history_lookup": false,
		"is_diagnostic_question": false, "has_per_member_table": false
	},
	"diagnostic_profile": {
		"is_diagnostic": false,
		"current_risk": false,
		"historical_regression": false,
		"current_version_check": false,
		"confidence": 0.7
	},
	"answer_role_profile": {
		"is_role_binding_requested": false,
		"confidence": 0.7
	},
	"error_granularity_profile": {
		"is_granularity_question": false,
		"confidence": 0.7
	},
	"runtime_artifact_scope_profile": {
		"requested_scope": "not_applicable",
		"confidence": 0.7
	},
	"history_selection_profile": {
		"mode": "not_applicable",
		"item_kind": "not_applicable",
		"confidence": 0.7
	},
	"completeness_obligation": {
		"required": false,
		"source_quote": ""
	}
`

func withV4Required(partial string) string {
	trimmed := strings.TrimSpace(partial)
	if !strings.HasSuffix(trimmed, "}") {
		// Pre-condition: partial must be a JSON object literal. Tests
		// that violate this should be rewritten, not silently patched.
		panic("withV4Required: payload is not a JSON object literal: " + partial)
	}
	// Strip the closing brace + any trailing comma/whitespace from the
	// last field so the v4 defaults can append cleanly with their own
	// leading comma.
	body := strings.TrimRightFunc(trimmed[:len(trimmed)-1], func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	defaults := v4DefaultsJSON
	if !strings.Contains(trimmed, `"predicate_axis"`) {
		defaults += `,
	"predicate_axis": ""`
	}
	if !strings.Contains(trimmed, `"call_chain_endpoints"`) {
		defaults += `,
	"call_chain_endpoints": {
		"source": "",
		"sink": "",
		"sink_mode": "exact",
		"runtime_selection_required": false,
		"runtime_selection_source_quote": ""
	}`
	}
	if !strings.Contains(trimmed, `"runtime_selection_profile"`) &&
		!strings.Contains(trimmed, `"runtime_selection_required":true`) {
		defaults += `,
	"runtime_selection_profile": {
		"is_selection_question": false,
		"source_quote": "",
		"confidence": 0.7
	}`
	}
	if !strings.Contains(trimmed, `"requested_answer_dimensions"`) {
		defaults += `,
	"requested_answer_dimensions": {
		"is_dimensioned_answer": false,
		"confidence": 0.7
	}`
	}
	if !strings.Contains(trimmed, `"runtime_target_profile"`) && !strings.Contains(trimmed, `"runtime_targets"`) {
		defaults += `,
	"runtime_target_profile": {
		"declaration": "unspecified",
		"confidence": 0.7
	}`
	}
	if !strings.Contains(trimmed, `"runtime_question_profile"`) {
		defaults += `,
	"runtime_question_profile": {
		"scope": "unspecified",
		"confidence": 0.7
	}`
	}
	return body + defaults + "}"
}

func withRequiredAnswerRoleProfile(payload string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return payload
	}
	if _, ok := obj["answer_role_profile"]; !ok {
		obj["answer_role_profile"] = json.RawMessage(`{"is_role_binding_requested":false,"confidence":0.7}`)
	}
	if _, ok := obj["error_granularity_profile"]; !ok {
		obj["error_granularity_profile"] = json.RawMessage(`{"is_granularity_question":false,"confidence":0.7}`)
	}
	if _, ok := obj["runtime_artifact_scope_profile"]; !ok {
		obj["runtime_artifact_scope_profile"] = json.RawMessage(`{"requested_scope":"not_applicable","confidence":0.7}`)
	}
	if _, hasProfile := obj["runtime_target_profile"]; !hasProfile {
		if _, hasTargets := obj["runtime_targets"]; !hasTargets {
			obj["runtime_target_profile"] = json.RawMessage(`{"declaration":"unspecified","confidence":0.7}`)
		}
	}
	if _, ok := obj["runtime_question_profile"]; !ok {
		obj["runtime_question_profile"] = json.RawMessage(`{"scope":"unspecified","confidence":0.7}`)
	}
	if _, ok := obj["history_selection_profile"]; !ok {
		isHistoryLookup := false
		if rawPredicates, ok := obj["predicates"]; ok {
			var predicates struct {
				IsHistoryLookup bool `json:"is_history_lookup"`
			}
			_ = json.Unmarshal(rawPredicates, &predicates)
			isHistoryLookup = predicates.IsHistoryLookup
		}
		if isHistoryLookup {
			obj["history_selection_profile"] = json.RawMessage(`{"mode":"unspecified","item_kind":"unspecified","confidence":0.7}`)
		} else {
			obj["history_selection_profile"] = json.RawMessage(`{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.7}`)
		}
	}
	if _, ok := obj["completeness_obligation"]; !ok {
		obj["completeness_obligation"] = json.RawMessage(`{"required":false,"source_quote":""}`)
	}
	if _, ok := obj["predicate_axis"]; !ok {
		obj["predicate_axis"] = json.RawMessage(`""`)
	}
	if _, ok := obj["call_chain_endpoints"]; !ok {
		obj["call_chain_endpoints"] = json.RawMessage(`{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""}`)
	}
	if _, ok := obj["runtime_selection_profile"]; !ok {
		profile := `{"is_selection_question":false,"source_quote":"","confidence":0.7}`
		if rawEndpoints, exists := obj["call_chain_endpoints"]; exists {
			var endpoints types.CallChainEndpointProfile
			if json.Unmarshal(rawEndpoints, &endpoints) == nil && endpoints.RuntimeSelectionRequired {
				encodedQuote, _ := json.Marshal(endpoints.RuntimeSelectionSourceQuote)
				profile = `{"is_selection_question":true,"source_quote":` + string(encodedQuote) + `,"confidence":0.7}`
			}
		}
		obj["runtime_selection_profile"] = json.RawMessage(profile)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return string(out)
}

func TestEmitAnalysis_RuntimeProfileValidationReportsIndependentErrorsTogether(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["runtime", "profile"],
		"entities": ["trace"],
		"question_kind": "mechanism"
	}`)), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	// All three objects are valid JSON wire shapes, so strict decode succeeds;
	// each independently lacks its own required typed declaration.
	payload["runtime_artifact_scope_profile"] = map[string]any{}
	payload["runtime_target_profile"] = map[string]any{}
	payload["runtime_question_profile"] = map[string]any{}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: types.NewMutableState("explain the runtime profile"),
	}, raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid profiles should be rejected: %q", res.Summary)
	}
	wants := []string{
		"runtime_artifact_scope_profile missing required field(s): requested_scope, confidence",
		"runtime_target_profile missing required field(s): declaration, confidence",
		"runtime_question_profile missing required field(s): scope, confidence",
	}
	last := -1
	for _, want := range wants {
		at := strings.Index(res.Summary, want)
		if at < 0 {
			t.Fatalf("summary should report %q in the same retry, got %q", want, res.Summary)
		}
		if at <= last {
			t.Fatalf("profile errors should follow schema order, got %q", res.Summary)
		}
		last = at
	}
}

func TestEmitAnalysis_RuntimeProfileValidationDoesNotReportDependentTargetCascade(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["runtime", "target"],
		"entities": ["worker"],
		"question_kind": "mechanism"
	}`)), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	payload["runtime_targets"] = []any{map[string]any{
		"kind":       "process",
		"source":     "user_explicit",
		"confidence": 0.9,
	}}
	payload["runtime_target_profile"] = map[string]any{
		"declaration":  "named_target",
		"source_quote": "worker",
		"confidence":   0.9,
	}
	payload["runtime_question_profile"] = map[string]any{}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: types.NewMutableState("explain worker"),
	}, raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{
		"runtime_targets[0] is structurally invalid",
		"runtime_question_profile missing required field(s): scope, confidence",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain independent error %q, got %q", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "named_target requires at least one structurally valid runtime_targets entry") {
		t.Fatalf("malformed target roster must not create dependent missing-target noise: %q", res.Summary)
	}
}

func testBoolPtr(v bool) *bool {
	return &v
}

func testFloatPtr(v float64) *float64 {
	return &v
}

func TestParseHistorySelectionProfileSeparatesOrdinalFromHistoryEvidenceOrigin(t *testing.T) {
	confidence := 0.9
	profile, errText, warnings := parseHistorySelectionProfile(
		"解释最近一次合入",
		true,
		&emitHistorySelectionProfileParam{
			Mode:        "latest_one",
			ItemKind:    "merge",
			SourceQuote: "最近一次合入",
			Confidence:  &confidence,
		},
	)
	if errText != "" || len(warnings) != 0 || profile == nil ||
		profile.Mode != types.HistorySelectionLatestOne ||
		profile.ItemKind != types.HistorySelectionItemMerge || profile.Count != 1 {
		t.Fatalf("profile=%+v err=%q warnings=%v", profile, errText, warnings)
	}
	if _, errText, _ := parseHistorySelectionProfile("普通源码问题", false, nil); errText != "" {
		t.Fatalf("non-history direct callers should default to not_applicable: %q", errText)
	}
	if _, errText, _ := parseHistorySelectionProfile("历史问题", true, nil); errText == "" {
		t.Fatal("history lookup must fail loud when the selection profile is missing")
	}
	badQuote := &emitHistorySelectionProfileParam{
		Mode:        "latest_one",
		ItemKind:    "commit",
		SourceQuote: "earliest commit",
		Confidence:  &confidence,
	}
	if _, errText, _ := parseHistorySelectionProfile("latest commit", true, badQuote); !strings.Contains(errText, "copied verbatim") {
		t.Fatalf("unanchored selection quote should fail, got %q", errText)
	}
	badCount := &emitHistorySelectionProfileParam{
		Mode:        "recent_n",
		ItemKind:    "commit",
		Count:       0,
		SourceQuote: "recent commits",
		Confidence:  &confidence,
	}
	if _, errText, _ := parseHistorySelectionProfile("recent commits", true, badCount); !strings.Contains(errText, "count in [1,100]") {
		t.Fatalf("recent_n without a bounded count should fail, got %q", errText)
	}
}

func TestParseRuntimeArtifactScopeProfileAnchorsUserScopeAndSoftensModelScope(t *testing.T) {
	confidence := 0.9
	start, end := 34579.45, 34579.48
	full, errText, warnings := parseRuntimeArtifactScopeProfile(
		"只分析这份 trace，不分析代码",
		true,
		&emitRuntimeArtifactScopeProfileParam{
			RequestedScope: string(types.RuntimeArtifactScopeFullArtifact),
			SourceQuote:    "这份 trace",
			Confidence:     &confidence,
		},
	)
	if errText != "" || len(warnings) != 0 || !full.FullArtifact() {
		t.Fatalf("anchored full artifact scope should survive: profile=%+v err=%q warnings=%v", full, errText, warnings)
	}
	explicit, errText, warnings := parseRuntimeArtifactScopeProfile(
		"分析 34579.45..34579.48 秒",
		true,
		&emitRuntimeArtifactScopeProfileParam{
			RequestedScope: string(types.RuntimeArtifactScopeExplicitWindow),
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "34579.45..34579.48",
			Confidence:     &confidence,
		},
	)
	gotStart, gotEnd, ok := explicit.ExplicitTimeWindow()
	if errText != "" || len(warnings) != 0 || !ok || gotStart != start || gotEnd != end {
		t.Fatalf("anchored explicit scope should survive: profile=%+v err=%q warnings=%v", explicit, errText, warnings)
	}
	canonical, errText, warnings := parseRuntimeArtifactScopeProfile(
		"分析目标在 34579.45..34579.48 秒窗口内的状态",
		true,
		&emitRuntimeArtifactScopeProfileParam{
			RequestedScope: string(types.RuntimeArtifactScopeBoundedSelector),
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "目标在 34579.45..34579.48 秒窗口内",
			Confidence:     &confidence,
		},
	)
	gotStart, gotEnd, ok = canonical.ExplicitTimeWindow()
	if errText != "" || len(warnings) != 1 || !ok || gotStart != start || gotEnd != end {
		t.Fatalf("anchored bounded selector with a valid typed window should canonicalize: profile=%+v err=%q warnings=%v", canonical, errText, warnings)
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&types.RequestModel{
		Intent:                      types.IntentExplain,
		RuntimeArtifactScopeProfile: canonical,
	}); !decided || !allowed {
		t.Fatalf("canonicalized exact window must retain deterministic trace report authority: decided=%t allowed=%t profile=%+v", decided, allowed, canonical)
	}
	invalidEnd := start
	bounded, errText, warnings := parseRuntimeArtifactScopeProfile(
		"分析目标 span-A",
		true,
		&emitRuntimeArtifactScopeProfileParam{
			RequestedScope: string(types.RuntimeArtifactScopeBoundedSelector),
			TimeStart:      &start,
			TimeEnd:        &invalidEnd,
			SourceQuote:    "span-A",
			Confidence:     &confidence,
		},
	)
	if errText != "" || len(warnings) != 0 ||
		bounded.RequestedScope != types.RuntimeArtifactScopeBoundedSelector ||
		bounded.TimeStart != nil || bounded.TimeEnd != nil {
		t.Fatalf("invalid typed times must not promote a bounded selector: profile=%+v err=%q warnings=%v", bounded, errText, warnings)
	}
	softened, errText, warnings := parseRuntimeArtifactScopeProfile(
		"只分析这份 trace",
		true,
		&emitRuntimeArtifactScopeProfileParam{
			RequestedScope: string(types.RuntimeArtifactScopeFullArtifact),
			SourceQuote:    "model selected 1..2",
			Confidence:     &confidence,
		},
	)
	if errText != "" || softened.RequestedScope != types.RuntimeArtifactScopeUnspecified || len(warnings) == 0 {
		t.Fatalf("unanchored model scope must soften, not become authority: profile=%+v err=%q warnings=%v", softened, errText, warnings)
	}
}

func TestParseRuntimeTargetProfileRequiresDeclaredNamedTarget(t *testing.T) {
	confidence := 0.95
	targets := []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 59566, Thread: "com.baidu.tieba",
		Source: "user_explicit", Confidence: confidence,
	}}
	if _, errText, _ := parseRuntimeTargetProfile("分析 com.baidu.tieba-59566 的状态", true, nil, nil); errText == "" {
		t.Fatal("runtime artifact analysis must fail loud when runtime_target_profile is missing")
	}
	named := &emitRuntimeTargetProfileParam{
		Declaration: string(types.RuntimeTargetDeclarationNamedTarget),
		SourceQuote: "com.baidu.tieba-59566",
		Confidence:  &confidence,
	}
	if _, errText, _ := parseRuntimeTargetProfile("分析 com.baidu.tieba-59566 的状态", true, named, nil); !strings.Contains(errText, "requires at least one") {
		t.Fatalf("named declaration without runtime_targets must fail loud, got %q", errText)
	}
	profile, errText, warnings := parseRuntimeTargetProfile("分析 com.baidu.tieba-59566 的状态", true, named, targets)
	if errText != "" || len(warnings) != 0 || !profile.NamedTarget() {
		t.Fatalf("anchored named target should survive: profile=%+v err=%q warnings=%v", profile, errText, warnings)
	}
	noNamed := &emitRuntimeTargetProfileParam{
		Declaration: string(types.RuntimeTargetDeclarationNoNamedTarget),
		Confidence:  &confidence,
	}
	if _, errText, _ := parseRuntimeTargetProfile("分析整份 trace", true, noNamed, targets); !strings.Contains(errText, "conflicts") {
		t.Fatalf("no_named_target must reject contradictory target rows, got %q", errText)
	}
}

func TestParseRuntimeQuestionProfileSeparatesFactBreadthFromLegacyLabels(t *testing.T) {
	confidence := 0.95
	if _, errText, _ := parseRuntimeQuestionProfile("分析这份 trace", true, nil, false); errText == "" {
		t.Fatal("runtime artifact analysis must fail loud when runtime_question_profile is missing")
	}
	bounded := &emitRuntimeQuestionProfileParam{
		Scope:        "bounded_fact_set",
		FactFamilies: []string{"target_scheduler_state", "target_wait_occurrences", "recorded_reason", "occurrence_time", "count_or_duration"},
		SourceQuote:  "有没有进入过不可中断等待",
		Confidence:   &confidence,
	}
	profile, errText, warnings := parseRuntimeQuestionProfile(
		"这份 trace 里有没有进入过不可中断等待，时间、记录原因和总量是什么",
		true,
		bounded,
		false,
	)
	if errText != "" || len(warnings) != 0 || profile == nil || !profile.BoundedFactSet() {
		t.Fatalf("bounded runtime fact profile rejected: profile=%+v err=%q warnings=%v", profile, errText, warnings)
	}
	if len(profile.FactFamilies) != 5 || profile.FactFamilies[0] != types.RuntimeQuestionFactTargetSchedulerState {
		t.Fatalf("bounded state/wait fact families not preserved: %+v", profile)
	}
	emptyFamilies := *bounded
	emptyFamilies.FactFamilies = nil
	if _, errText, _ := parseRuntimeQuestionProfile("有没有进入过不可中断等待", true, &emptyFamilies, false); !strings.Contains(errText, "requires one or more fact_families") {
		t.Fatalf("bounded fact set without fact families must fail loud, got %q", errText)
	}
	relationProfile, errText, relationWarnings := parseRuntimeQuestionProfile(
		"有没有进入过不可中断等待",
		true,
		bounded,
		true,
	)
	if errText != "" || len(relationWarnings) != 0 || relationProfile == nil || !relationProfile.BoundedFactSet() {
		t.Fatalf("finite typed relation facts must retain bounded breadth: profile=%+v err=%q warnings=%v", relationProfile, errText, relationWarnings)
	}
	badQuote := *bounded
	badQuote.SourceQuote = "paraphrased quote"
	gotProfile, errText, gotWarnings := parseRuntimeQuestionProfile("analyze this trace", true, &badQuote, false)
	if errText != "" || gotProfile == nil || !gotProfile.BoundedFactSet() ||
		len(gotWarnings) != 1 || !strings.Contains(gotWarnings[0], "ignored unanchored source_quote") {
		t.Fatalf("unanchored audit quote must warn without dropping typed scope: profile=%+v err=%q warnings=%v", gotProfile, errText, gotWarnings)
	}
	notApplicable := &emitRuntimeQuestionProfileParam{
		Scope: "not_applicable", Confidence: &confidence,
	}
	if _, errText, _ := parseRuntimeQuestionProfile("analyze this trace", true, notApplicable, false); !strings.Contains(errText, "conflicts") {
		t.Fatalf("runtime not_applicable must conflict with an attached artifact, got %q", errText)
	}
}

func TestRuntimeQuestionCausalDiagnosisUsesScopeAndRequiredCausalDimensionAsBreadthAuthority(t *testing.T) {
	profile := &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	causalDimension := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "root cause", Role: types.RequestedAnswerDimensionCausalAttribution, Required: true,
		}},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		profile,
		nil,
		types.IntentTrace,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); !strings.Contains(issue, "causal_diagnosis requires") {
		t.Fatalf("causal diagnosis without a typed diagnosis carrier must fail loud: %q", issue)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		nil,
		types.IntentTrace,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("bounded facts must stay orthogonal to diagnosis lanes: %q", issue)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		profile,
		causalDimension,
		types.IntentTrace,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("causal scope plus required causal dimension must be sufficient without duplicate legacy flags: %q", issue)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		profile,
		causalDimension,
		types.IntentTrace,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); issue != "" {
		t.Fatalf("typed diagnostic carrier plus causal dimension must retain causal breadth: %q", issue)
	}
}

func TestRuntimeQuestionBoundedFactsAllowDiagnosticFrameLocation(t *testing.T) {
	profile := &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue},
	}
	dimensions := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{
			{Label: "ArkTS frame", Role: types.RequestedAnswerDimensionObservedValue, Required: true},
			{Label: "Cangjie frame", Role: types.RequestedAnswerDimensionObservedValue, Required: true},
		},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		profile,
		dimensions,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); issue != "" {
		t.Fatalf("finite observed crash-frame dimensions were incorrectly widened or rejected: %q", issue)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		profile,
		dimensions,
		types.IntentRootCause,
		types.ScenarioRootCause,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); !strings.Contains(issue, "must not suppress the chain/ranking evidence") ||
		!strings.Contains(issue, "requested_answer_dimensions causal role") ||
		!strings.Contains(issue, "causal_contributor_set for a roster/ranking") {
		t.Fatalf("explicit root-cause classifiers must not be accepted with bounded-fact breadth: %q", issue)
	}
}

func TestParseRequestedAnswerDimensionsRejectsActiveEmptyProfile(t *testing.T) {
	active := true
	confidence := 0.9
	profile, signals, errText, warnings := parseRequestedAnswerDimensions("find the root cause", &emitRequestedAnswerDimensionsParam{
		IsDimensionedAnswer: &active,
		Confidence:          &confidence,
	})
	if profile != nil || len(signals) != 0 || len(warnings) != 0 ||
		!strings.Contains(errText, "requires at least one dimensions row") ||
		!strings.Contains(errText, "role=target_effect_verdict") ||
		!strings.Contains(errText, "role=causal_attribution") {
		t.Fatalf("active empty answer-dimension profile must fail as one actionable typed contradiction: profile=%+v signals=%v err=%q warnings=%v", profile, signals, errText, warnings)
	}
}

func TestRuntimeQuestionBoundedFactsConflictWithRequiredTargetEffectVerdict(t *testing.T) {
	verdictDimension := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "target effect", Role: types.RequestedAnswerDimensionTargetEffectVerdict, Required: true,
		}},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		verdictDimension,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); !strings.Contains(issue, "bounded_fact_set conflicts") || !strings.Contains(issue, "role=target_effect_verdict") {
		t.Fatalf("bounded target-effect verdict must fail loud for a complete analyzer retry: %q", issue)
	} else {
		for _, want := range []string{
			"Preserve that required user-facing dimension",
			"not a pre-decided conclusion",
			"yes, no, mixed, or unproven",
			"do not delete or relabel",
			"bounded_effect_verdict",
		} {
			if !strings.Contains(issue, want) {
				t.Fatalf("bounded causal retry guidance missing %q: %q", want, issue)
			}
		}
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		verdictDimension,
		types.IntentRootCause,
		types.ScenarioRootCause,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); !strings.Contains(issue, "target_effect_verdict") || !strings.Contains(issue, "cannot authorize cause discovery") {
		t.Fatalf("finite target-effect role must not authorize causal diagnosis: %q", issue)
	}

	optional := *verdictDimension
	optional.Dimensions = append([]types.RequestedAnswerDimension(nil), verdictDimension.Dimensions...)
	optional.Dimensions[0].Required = false
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		&optional,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("an optional causal display hint must not widen a bounded fact request: %q", issue)
	}

	boundedValues := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "duration", Role: types.RequestedAnswerDimensionCount, Required: true,
		}},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		boundedValues,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("finite observed values must remain bounded: %q", issue)
	}
}

func TestRuntimeQuestionCausalAttributionDoesNotAliasTargetEffectVerdict(t *testing.T) {
	causalAttribution := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "root cause", Role: types.RequestedAnswerDimensionCausalAttribution, Required: true,
		}},
	}
	finite := &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedEffectVerdict,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		finite,
		causalAttribution,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); !strings.Contains(issue, "causal_attribution owns a full discovered root-cause") ||
		!strings.Contains(issue, "target_effect_verdict") ||
		!strings.Contains(issue, "will not reinterpret") {
		t.Fatalf("finite scope silently aliased causal attribution into a target verdict: %q", issue)
	}

	full := &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	if issue := validateRuntimeQuestionProfileConsistency(
		full,
		causalAttribution,
		types.IntentRootCause,
		types.ScenarioRootCause,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); issue != "" {
		t.Fatalf("full causal attribution tuple rejected: %q", issue)
	}
}

func TestRuntimeQuestionExplainTargetEffectVerdictUsesDistinctTypedDimension(t *testing.T) {
	targetVerdict := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label:       "是否受算力限制",
			Role:        types.RequestedAnswerDimensionTargetEffectVerdict,
			SourceQuote: "是否受算力限制",
			Required:    true,
		}},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		targetVerdict,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); !strings.Contains(issue, "target_effect_verdict") || !strings.Contains(issue, "cannot authorize cause discovery") {
		t.Fatalf("one finite target-effect verdict must not widen into causal diagnosis: %q", issue)
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		targetVerdict,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); !strings.Contains(issue, "finite target-effect verdict") || !strings.Contains(issue, "may remain an unresolved mechanism") {
		t.Fatalf("bounded retry must teach the causal-verdict shape, got %q", issue)
	}
	boundedEffect := &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedEffectVerdict,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		boundedEffect,
		targetVerdict,
		types.IntentExplain,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("finite condition-to-target verdict must not widen into full causal diagnosis: %q", issue)
	}
	if boundedEffect.RequiresFullReport() {
		t.Fatal("bounded effect verdict must not authorize full root-cause projection")
	}

	measuredFrequency := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "CPU 频率值", Role: types.RequestedAnswerDimensionEvidenceSource, Required: true,
		}},
	}
	if issue := validateRuntimeQuestionProfileConsistency(
		&types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		measuredFrequency,
		types.IntentExplain,
		types.ScenarioGeneric,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); issue != "" {
		t.Fatalf("ordinary measured frequency/evidence lookup must remain bounded: %q", issue)
	}
}

func TestRuntimeQuestionCausalContributorSetCannotEnterFiniteScope(t *testing.T) {
	roster := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label:    "ranked contributors",
			Role:     types.RequestedAnswerDimensionCausalContributorSet,
			Required: true,
		}},
	}
	profile := &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration},
	}
	for _, scope := range []types.RuntimeQuestionScope{
		types.RuntimeQuestionScopeBoundedFactSet,
		types.RuntimeQuestionScopeBoundedEffectVerdict,
	} {
		profile.Scope = scope
		issue := validateRuntimeQuestionProfileConsistency(
			profile,
			roster,
			types.IntentExplain,
			types.ScenarioGeneric,
			types.SemanticPredicates{},
			types.DiagnosticIntentProfile{},
		)
		for _, want := range []string{
			"causal_contributor_set",
			"causal_diagnosis",
			"causal_diagnosis_canonical_field_target=",
			`"scope":"causal_diagnosis"`,
		} {
			if !strings.Contains(issue, want) {
				t.Fatalf("scope %s causal-roster repair missing %q: %s", scope, want, issue)
			}
		}
	}

	causal := &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	if issue := validateRuntimeQuestionProfileConsistency(
		causal,
		roster,
		types.IntentRootCause,
		types.ScenarioRootCause,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); issue != "" {
		t.Fatalf("full causal roster tuple rejected: %q", issue)
	}
}

func TestRuntimeQuestionFiniteVerdictRepairTargetConvergesWholeTypedTuple(t *testing.T) {
	targetVerdict := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Label: "target effect", Role: types.RequestedAnswerDimensionTargetEffectVerdict, Required: true,
		}},
	}
	profile := &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{
			types.RuntimeQuestionFactCountOrDuration,
			types.RuntimeQuestionFactTargetSchedulerState,
			types.RuntimeQuestionFactFrequencyResidency,
		},
	}
	issue := validateRuntimeQuestionProfileConsistency(
		profile,
		targetVerdict,
		types.IntentExplain,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	)
	for _, want := range []string{
		"canonical_field_target=",
		`"scope":"bounded_effect_verdict"`,
		`"fact_families":["count_or_duration","target_scheduler_state","frequency_residency"]`,
		`"intent":"explain"`,
		`"scenario":"performance_bottleneck"`,
		`"is_diagnostic_question":false`,
		`"is_diagnostic":false`,
		"next COMPLETE object",
		"preserve the required target_effect_verdict dimension",
	} {
		if !strings.Contains(issue, want) {
			t.Fatalf("finite-verdict repair target missing %q: %s", want, issue)
		}
	}

	// Applying the typed field target to a complete model-owned object must
	// converge immediately. The validator does not mutate the model payload.
	profile.Scope = types.RuntimeQuestionScopeBoundedEffectVerdict
	if got := validateRuntimeQuestionProfileConsistency(
		profile,
		targetVerdict,
		types.IntentExplain,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
	); got != "" {
		t.Fatalf("canonical finite-verdict tuple did not converge: %q", got)
	}

	// A stale diagnostic flag on an otherwise coherent bounded-effect tuple
	// points back to the same finite target instead of bouncing to causal.
	if got := validateRuntimeQuestionProfileConsistency(
		profile,
		targetVerdict,
		types.IntentExplain,
		types.ScenarioPerformanceBottleneck,
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
	); !strings.Contains(got, "canonical_field_target=") || strings.Contains(got, "use causal_diagnosis and omit fact_families") {
		t.Fatalf("stale legacy flags caused another breadth bounce: %q", got)
	}
}

func TestEmitAnalysisRejectsCausalBreadthWithoutTypedDiagnosisCarrier(t *testing.T) {
	raw := "请说明目标进程、transaction 编号和直接唤醒者"
	payload := `{
		"intent":"trace",
		"scenario":"generic",
		"complexity":"simple",
		"intent_confidence":0.95,
		"complexity_confidence":0.95,
		"kind_confidence":0.95,
		"keywords":["binder_transaction","sched_wakeup"],
		"entities":["client-20","binder:100_1-101","transaction=42"],
		"question_kind":"call_chain",
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.95},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.95},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.95},
		"runtime_artifact_scope_profile":{"requested_scope":"full_artifact","source_quote":"请说明目标进程、transaction 编号和直接唤醒者","confidence":0.95},
		"runtime_target_profile":{"declaration":"no_named_target","confidence":0.95},
		"runtime_question_profile":{"scope":"causal_diagnosis","source_quote":"请说明目标进程、transaction 编号和直接唤醒者","confidence":0.95},
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.95},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	ctx := &types.BusContext{Mutable: types.NewMutableState(raw)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"binder_transaction"}}})
	res, err := (&EmitAnalysis{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "causal_diagnosis requires") {
		t.Fatalf("incoherent causal breadth must reject for analyzer retry: success=%t summary=%q", res.Success, res.Summary)
	}

	bounded := strings.Replace(payload, `"scope":"causal_diagnosis"`, `"scope":"bounded_fact_set","fact_families":["relation_peer","transaction_id","direct_waker"]`, 1)
	ctx = &types.BusContext{Mutable: types.NewMutableState(raw)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"binder_transaction"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(bounded))
	if err != nil {
		t.Fatalf("bounded Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("finite bounded relation facts must remain admissible: %q", res.Summary)
	}

	effectObjective := "CPU4 的 policy 上限是否限制了目标线程，证据是什么"
	boundedEffect := strings.Replace(
		bounded,
		`"scope":"bounded_fact_set","fact_families":["relation_peer","transaction_id","direct_waker"]`,
		`"scope":"bounded_effect_verdict","fact_families":["frequency_residency"]`,
		1,
	)
	boundedEffect = strings.Replace(
		boundedEffect,
		`"completeness_obligation":`,
		`"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[{"index":1,"label":"是否限制了目标线程","role":"target_effect_verdict","source_quote":"是否限制了目标线程","required":true},{"index":2,"label":"证据是什么","role":"evidence_source","source_quote":"证据是什么","required":true}]},"completeness_obligation":`,
		1,
	)
	legacyAliasedEffect := strings.Replace(boundedEffect, `"role":"target_effect_verdict"`, `"role":"causal_attribution"`, 1)
	ctx = &types.BusContext{Mutable: types.NewMutableState(effectObjective)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"cpu_frequency_limits"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(legacyAliasedEffect))
	if err != nil {
		t.Fatalf("legacy-aliased bounded-effect Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "causal_attribution owns a full discovered root-cause") ||
		!strings.Contains(res.Summary, "target_effect_verdict") {
		t.Fatalf("production emit silently aliased causal attribution into finite target effect: success=%t summary=%q", res.Success, res.Summary)
	}
	ctx = &types.BusContext{Mutable: types.NewMutableState(effectObjective)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"cpu_frequency_limits"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(boundedEffect))
	if err != nil {
		t.Fatalf("bounded-effect Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("finite condition-to-target verdict must be production-reachable without full diagnosis: %q", res.Summary)
	}
	effectProfile := ctx.Mutable.RequestModel().RuntimeQuestionProfile
	if effectProfile == nil || !effectProfile.BoundedEffectVerdict() || effectProfile.RequiresFullReport() || len(effectProfile.FactFamilies) != 1 || effectProfile.FactFamilies[0] != types.RuntimeQuestionFactFrequencyResidency {
		t.Fatalf("bounded effect profile not persisted at finite breadth: %+v", effectProfile)
	}

	causalObjective := "请按重要程度给出根因排序，并说明目标进程、transaction 编号和直接唤醒者"
	boundedWithCausalDimension := strings.Replace(
		bounded,
		`"completeness_obligation":`,
		`"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[{"index":1,"label":"根因排序","role":"causal_contributor_set","source_quote":"根因排序","required":true}]},"completeness_obligation":`,
		1,
	)
	ctx = &types.BusContext{Mutable: types.NewMutableState(causalObjective)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"binder_transaction"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(boundedWithCausalDimension))
	if err != nil {
		t.Fatalf("bounded causal-dimension Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "bounded_fact_set conflicts") ||
		!strings.Contains(res.Summary, "causal_contributor_set") ||
		!strings.Contains(res.Summary, "causal_diagnosis_canonical_field_target=") {
		t.Fatalf("production entry must reject a bounded scope that cannot satisfy a required causal roster: success=%t summary=%q", res.Success, res.Summary)
	}

	causalRoster := strings.Replace(
		boundedWithCausalDimension,
		`"scope":"bounded_fact_set","fact_families":["relation_peer","transaction_id","direct_waker"]`,
		`"scope":"causal_diagnosis"`,
		1,
	)
	ctx = &types.BusContext{Mutable: types.NewMutableState(causalObjective)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"binder_transaction"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(causalRoster))
	if err != nil {
		t.Fatalf("causal roster Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed causal scope/roster must be accepted without redundant legacy mirrors: %q", res.Summary)
	}
	gotRM := ctx.Mutable.RequestModel()
	if gotRM == nil || gotRM.RuntimeQuestionProfile == nil ||
		gotRM.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeCausalDiagnosis ||
		gotRM.RequestedAnswerDimensions == nil ||
		len(gotRM.RequestedAnswerDimensions.Dimensions) != 1 ||
		gotRM.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionCausalContributorSet {
		t.Fatalf("causal contributor role/scope not persisted: %+v", gotRM)
	}

	causalWithRedundantFamilies := strings.Replace(payload, `"scenario":"generic"`, `"scenario":"performance_bottleneck"`, 1)
	causalWithRedundantFamilies = strings.Replace(
		causalWithRedundantFamilies,
		`"scope":"causal_diagnosis"`,
		`"scope":"causal_diagnosis","fact_families":["target_scheduler_state","recorded_reason"]`,
		1,
	)
	ctx = &types.BusContext{Mutable: types.NewMutableState(raw)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"binder_transaction"}}})
	res, err = (&EmitAnalysis{}).Execute(ctx, json.RawMessage(causalWithRedundantFamilies))
	if err != nil {
		t.Fatalf("causal compat Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "fact_families conflicts") || !strings.Contains(res.Summary, "will not be silently discarded") {
		t.Fatalf("contradictory non-bounded fact families must fail loud for a coherent model retry: success=%t summary=%q", res.Success, res.Summary)
	}
}

func TestParseRuntimeQuestionProfileUnanchoredQuoteIsAuditWarningNotRetry(t *testing.T) {
	confidence := 0.95
	profile, errText, warnings := parseRuntimeQuestionProfile(
		"请分析 com.demo 主线程这一帧窗口内的卡顿原因",
		true,
		&emitRuntimeQuestionProfileParam{
			Scope:       string(types.RuntimeQuestionScopeCausalDiagnosis),
			SourceQuote: "分析卡顿原因", // non-contiguous paraphrase
			Confidence:  &confidence,
		},
		false,
	)
	if errText != "" {
		t.Fatalf("audit-only quote must not reject typed scope: %q", errText)
	}
	if profile == nil || profile.Scope != types.RuntimeQuestionScopeCausalDiagnosis {
		t.Fatalf("typed scope lost: %+v", profile)
	}
	if profile.SourceQuote != "" {
		t.Fatalf("unanchored audit quote must be dropped, got %q", profile.SourceQuote)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "ignored unanchored source_quote") {
		t.Fatalf("warnings = %v, want one precise audit warning", warnings)
	}
}

func TestParseRuntimeQuestionProfileExactQuoteRemainsForAudit(t *testing.T) {
	confidence := 0.95
	profile, errText, warnings := parseRuntimeQuestionProfile(
		"请分析 com.demo 主线程这一帧窗口内的卡顿原因",
		true,
		&emitRuntimeQuestionProfileParam{
			Scope:       string(types.RuntimeQuestionScopeCausalDiagnosis),
			SourceQuote: "卡顿原因",
			Confidence:  &confidence,
		},
		false,
	)
	if errText != "" || len(warnings) != 0 {
		t.Fatalf("exact audit quote rejected or warned: err=%q warnings=%v", errText, warnings)
	}
	if profile == nil || profile.SourceQuote != "卡顿原因" {
		t.Fatalf("exact audit quote not retained: %+v", profile)
	}
}

func TestEmitAnalysisRuntimeQuestionSchemaPinsFactFamilyConditional(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &root); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	properties := root["properties"].(map[string]any)
	profile := properties["runtime_question_profile"].(map[string]any)
	profileProperties := profile["properties"].(map[string]any)
	families := profileProperties["fact_families"].(map[string]any)
	if got := families["minItems"]; got != float64(1) {
		t.Fatalf("fact_families minItems=%v, want 1", got)
	}
	allOf, ok := profile["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		t.Fatalf("runtime_question_profile conditional missing: %#v", profile["allOf"])
	}
	conditional := allOf[0].(map[string]any)
	ifBranch := conditional["if"].(map[string]any)
	scope := ifBranch["properties"].(map[string]any)["scope"].(map[string]any)
	if !reflect.DeepEqual(scope["enum"], []any{string(types.RuntimeQuestionScopeBoundedFactSet), string(types.RuntimeQuestionScopeBoundedEffectVerdict)}) {
		t.Fatalf("conditional scope enum=%#v", scope["enum"])
	}
	thenRequired := conditional["then"].(map[string]any)["required"].([]any)
	if !reflect.DeepEqual(thenRequired, []any{"fact_families"}) {
		t.Fatalf("finite runtime scopes must require fact_families: %#v", thenRequired)
	}
	elseNot := conditional["else"].(map[string]any)["not"].(map[string]any)
	elseRequired := elseNot["required"].([]any)
	if !reflect.DeepEqual(elseRequired, []any{"fact_families"}) {
		t.Fatalf("non-finite scopes must forbid fact_families: %#v", elseRequired)
	}
}

func TestParseRequestedAnswerDimensionRoleSeparatesDisplayRoleFromRuntimeFactFamily(t *testing.T) {
	role, warning, errText := parseRequestedAnswerDimensionRole("frequency_residency")
	if errText != "" || role != types.RequestedAnswerDimensionObservedValue || !strings.Contains(warning, "specific runtime semantics remain") {
		t.Fatalf("known runtime fact-family drift must normalize losslessly: role=%q warning=%q err=%q", role, warning, errText)
	}
	role, warning, errText = parseRequestedAnswerDimensionRole("causal_attribution")
	if errText != "" || warning != "" || role != types.RequestedAnswerDimensionCausalAttribution {
		t.Fatalf("valid display role changed: role=%q warning=%q err=%q", role, warning, errText)
	}
	role, warning, errText = parseRequestedAnswerDimensionRole("target_effect_verdict")
	if errText != "" || warning != "" || role != types.RequestedAnswerDimensionTargetEffectVerdict {
		t.Fatalf("valid finite-verdict role changed: role=%q warning=%q err=%q", role, warning, errText)
	}
	role, warning, errText = parseRequestedAnswerDimensionRole("branch_behavior")
	if errText != "" || warning != "" || role != types.RequestedAnswerDimensionBranchBehavior {
		t.Fatalf("valid branch-behavior role changed: role=%q warning=%q err=%q", role, warning, errText)
	}
	if _, _, errText = parseRequestedAnswerDimensionRole("made_up_runtime_role"); !strings.Contains(errText, "is invalid") {
		t.Fatalf("unknown role must fail loud instead of silently becoming other: %q", errText)
	}
}

func TestEmitAnalysisSchemaRequiresExplicitCompletenessDecision(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &root); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	required, _ := root["required"].([]any)
	if !slices.Contains(required, any("completeness_obligation")) {
		t.Fatalf("root required fields omit completeness_obligation: %#v", required)
	}
	properties := root["properties"].(map[string]any)
	profile := properties["completeness_obligation"].(map[string]any)
	description, _ := profile["description"].(string)
	for _, want := range []string{
		"Required typed decision",
		"required=false",
		"source_quote=\"\"",
		"whole requested mechanism path",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("completeness schema description missing %q: %q", want, description)
		}
	}
}

func TestEmitAnalysisSchemaRequiresExplicitAnswerDimensionDecision(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &root); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	required, _ := root["required"].([]any)
	if !slices.Contains(required, any("requested_answer_dimensions")) {
		t.Fatalf("root required fields omit requested_answer_dimensions: %#v", required)
	}
	properties := root["properties"].(map[string]any)
	profile := properties["requested_answer_dimensions"].(map[string]any)
	description, _ := profile["description"].(string)
	for _, want := range []string{"Required typed declaration", "is_dimensioned_answer=false", "not an evidence origin"} {
		if !strings.Contains(description, want) {
			t.Fatalf("requested-answer-dimension schema description missing %q: %q", want, description)
		}
	}
}

func TestEmitAnalysisRejectsMissingCompletenessDecisionWithoutScanningRequest(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(withV4Required(`{
		"intent":"explain",
		"scenario":"generic",
		"complexity":"simple",
		"keywords":["status"],
		"entities":["service"],
		"question_kind":"mechanism"
	}`)), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	delete(payload, "completeness_obligation")
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	mut := types.NewMutableState("ordinary status question with no completeness words")
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mut}, raw)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "completeness_obligation is required") {
		t.Fatalf("missing typed decision must fail by field presence, got success=%t summary=%q", res.Success, res.Summary)
	}
}

func TestEmitAnalysisRuntimeArtifactCarrierIncludesRunEntryPreflight(t *testing.T) {
	preflight := &types.BusContext{RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:    "trace",
			Source:  "eval/fixtures/customer.systrace",
			Carrier: "request_path",
		}},
	}}
	if !emitAnalysisHasRuntimeArtifactCarrier(preflight) {
		t.Fatal("a normalized run-entry request-path artifact must preserve runtime scope authority")
	}
	if !emitAnalysisHasRuntimeArtifactCarrier(&types.BusContext{AttachedHitrace: "attached"}) {
		t.Fatal("the existing attached-trace carrier must remain supported")
	}
	if emitAnalysisHasRuntimeArtifactCarrier(&types.BusContext{}) {
		t.Fatal("an empty context must not manufacture a runtime artifact carrier")
	}
}

// -----------------------------------------------------------------------------
// Validator tests (pure, no tool wiring)
// -----------------------------------------------------------------------------

func TestValidateAnalysisInput_HappyPath(t *testing.T) {
	limits := DefaultAnalysisLimits()
	kw := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	ents := []string{"OrchestratorAgent", "StageAnalyze"}

	res := validateAnalysisInput(kw, ents, limits, "", 0)

	if res.RejectReason != "" {
		t.Errorf("clean input must not reject, got %q", res.RejectReason)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("clean input must not warn, got %v", res.Warnings)
	}
	if len(res.DroppedEntities) != 0 {
		t.Errorf("clean input must not drop entities, got %v", res.DroppedEntities)
	}
	if len(res.FilteredEntities) != 2 {
		t.Errorf("FilteredEntities should pass through clean entities, got %v", res.FilteredEntities)
	}
}

func TestValidateSelfConsistency_RoleLocateRequiresScalarValueAndSubject(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:     false,
		IsRoleLocateLookup: true,
	}
	reason := validateSelfConsistency(
		types.IntentExplain,
		types.ScenarioArchitectureExplain,
		"mechanism",
		preds,
		types.DiagnosticIntentProfile{},
		types.AxisDefine,
		[]string{"AnalysisIR"},
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "is_role_locate_lookup=true requires is_scalar_answer=true") {
		t.Fatalf("expected scalar role-locate contradiction, got %q", reason)
	}
}

func TestValidateSelfConsistency_RoleLocateAllowsNumericLineAnswer(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:     true,
		IsRoleLocateLookup: true,
	}
	reason := validateSelfConsistency(
		types.IntentReturnValue,
		types.ScenarioGeneric,
		"return_value",
		preds,
		types.DiagnosticIntentProfile{},
		types.AxisReturn,
		[]string{"WARN"},
		nil,
		types.AnswerSubject{Kind: types.SubjectNumeric, Confidence: 0.9},
	)
	if reason != "" {
		t.Fatalf("numeric line/event-row answers are legitimate scalar role-locate values, got %q", reason)
	}
}

func TestValidateSelfConsistency_CallChainRequiresPreciseRelationshipSignal(t *testing.T) {
	base := types.SemanticPredicates{}
	reason := validateSelfConsistency(
		types.IntentTrace,
		types.ScenarioGeneric,
		string(types.ReqCallChain),
		base,
		types.DiagnosticIntentProfile{},
		types.AxisUnknown,
		[]string{"com.baidu.tieba-59566"},
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "requires a precise relationship signal") {
		t.Fatalf("single-target state query mislabeled as call_chain must reject, got %q", reason)
	}

	cases := []struct {
		name     string
		preds    types.SemanticPredicates
		axis     types.PredicateAxis
		entities []string
	}{
		{
			name:     "explicit call axis permits a general caller lookup without typed runtime target",
			axis:     types.AxisCall,
			entities: []string{"RenderThread"},
		},
		{
			name:     "relational predicate permits one typed relationship target",
			preds:    types.SemanticPredicates{IsRelationalLookup: true},
			entities: []string{"Dispatch"},
		},
		{
			name:     "two endpoints permit source-to-sink trace",
			entities: []string{"Dispatch", "Handler"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateSelfConsistency(
				types.IntentTrace,
				types.ScenarioGeneric,
				string(types.ReqCallChain),
				tc.preds,
				types.DiagnosticIntentProfile{},
				tc.axis,
				tc.entities,
				nil,
				types.AnswerSubject{},
			); got != "" {
				t.Fatalf("valid relational call-chain shape rejected: %q", got)
			}
		})
	}

	if got := validateSelfConsistency(
		types.IntentTrace,
		types.ScenarioGeneric,
		string(types.ReqConditional),
		base,
		types.DiagnosticIntentProfile{},
		types.AxisCondition,
		[]string{"RenderThread"},
		nil,
		types.AnswerSubject{},
	); got != "" {
		t.Fatalf("single-target conditional runtime fact is not a call-chain contradiction: %q", got)
	}
}

func TestEmitAnalysis_Execute_RejectsSingleTargetRuntimeStateAsCallChain(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("分析 trace 中 com.baidu.tieba-59566 是否进入 D 状态、IO 等待时间、原因和总量")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "tieba.ftrace", Signals: []string{"sched-switch", "blocked-reason"}},
	})
	payload := withV4Required(`{
		"intent": "trace",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["trace", "thread", "state", "duration", "reason"],
		"entities": ["com.baidu.tieba", "59566"],
		"question_kind": "call_chain",
		"predicate_axis": "call",
		"runtime_targets": [{
			"kind": "thread",
			"thread": "com.baidu.tieba-59566",
			"source": "user_explicit",
			"confidence": 0.95
		}],
		"runtime_target_profile": {
			"declaration": "named_target",
			"source_quote": "com.baidu.tieba-59566",
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "requires predicates.is_relational_lookup=true") {
		t.Fatalf("call-axis-only single-target runtime state classification must retry instead of persisting call_chain, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("rejected call_chain classification must not persist RequestModel: %+v", mu.RequestModel())
	}
}

func TestValidateRuntimeArtifactCallChainConsistencyRequiresTypedRelationForOneTarget(t *testing.T) {
	target := []types.RuntimeTarget{{
		Kind:   types.RuntimeTargetKindThread,
		PID:    59566,
		Thread: "com.baidu.tieba-59566",
	}}
	if got := validateRuntimeArtifactCallChainConsistency(
		string(types.ReqCallChain),
		types.SemanticPredicates{},
		target,
	); !strings.Contains(got, "predicate_axis=call only names the relationship axis") {
		t.Fatalf("one-target call chain without relational authority must reject, got %q", got)
	}
	if got := validateRuntimeArtifactCallChainConsistency(
		string(types.ReqCallChain),
		types.SemanticPredicates{IsRelationalLookup: true},
		target,
	); got != "" {
		t.Fatalf("one-target typed waker/caller relation must remain valid, got %q", got)
	}
	if got := validateRuntimeArtifactCallChainConsistency(
		string(types.ReqCallChain),
		types.SemanticPredicates{},
		append(target, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 59843, Thread: "CookieMonsterCl-59843"}),
	); got != "" {
		t.Fatalf("two runtime endpoints must remain valid without a redundant relational predicate, got %q", got)
	}
	if got := validateRuntimeArtifactCallChainConsistency(
		string(types.ReqCallChain),
		types.SemanticPredicates{},
		append(target, types.RuntimeTarget{Kind: types.RuntimeTargetKindProcess, PID: 59566, Thread: "com.baidu.tieba"}),
	); got == "" {
		t.Fatal("two rows for the same positive pid must remain one focus identity, not source + sink")
	}
}

func TestEmitAnalysis_SourceCallChainAmbiguousEntitiesRequireExactEndpointPair(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请用 Mermaid 时序图展示 analyzer.go 里 buildAnalysisIR 到 gate.Run 的调用顺序"
	base := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["buildAnalysisIR", "gate.Run", "AnalysisIR", "call", "sequence"],
		"entities": ["analyzer.go", "buildAnalysisIR", "gate.Run", "AnalysisIR"],
		"question_kind": "call_chain",
		"predicate_axis": "call"
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, base)
	if res.Success || !strings.Contains(res.Summary, "requires call_chain_endpoints.source") {
		t.Fatalf("unordered entities must not become directional authority, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("missing ordered endpoint analysis must not persist: %+v", mu.RequestModel())
	}

	withEndpoints := strings.Replace(base,
		`"question_kind": "call_chain",`,
		`"question_kind": "call_chain", "call_chain_endpoints": {"source":"buildAnalysisIR", "sink":"gate.Run"},`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, withEndpoints)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("ordered endpoints should admit the two-endpoint lane, got success=%t summary=%q", res.Success, res.Summary)
	}
	if got := mu.RequestModel().AnalyzerHints.MentionedEntities; slices.Contains(got, "AnalysisIR") {
		t.Fatalf("nested AnalysisIR must not gain user-mentioned authority: %v", got)
	}

	objective = "请展示 buildAnalysisIR 到 gate.Run 的调用链；AnalysisIR 只是需要解释的中间类型"
	res, mu = runEmitAnalysisWithObjective(t, objective, withEndpoints)
	if res.Success || !strings.Contains(res.Summary, "entity ordering is not endpoint authority") {
		t.Fatalf("ambiguous call-chain entities must request exact endpoint identities, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("ambiguous endpoint analysis must not persist: %+v", mu.RequestModel())
	}

	withExact := strings.Replace(base,
		`"question_kind": "call_chain",`,
		`"question_kind": "call_chain", "exact_targets": ["buildAnalysisIR", "gate.Run"], "call_chain_endpoints": {"source":"buildAnalysisIR", "sink":"gate.Run"},`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, withExact)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("explicit source/sink pair should resolve contextual entity ambiguity, got success=%t summary=%q", res.Success, res.Summary)
	}
	if got := mu.RequestModel().AnalyzerHints.ExactTargets; !reflect.DeepEqual(got, []string{"buildAnalysisIR", "gate.Run"}) {
		t.Fatalf("persisted exact endpoints=%v", got)
	}
	if source, sink, ok := types.CallChainOrderedEndpointHints(*mu.RequestModel()); !ok || source != "buildAnalysisIR" || sink != "gate.Run" {
		t.Fatalf("persisted ordered endpoints=%q,%q,%t", source, sink, ok)
	}

	reversed := strings.Replace(withExact,
		`{"source":"buildAnalysisIR", "sink":"gate.Run"}`,
		`{"source":"gate.Run", "sink":"buildAnalysisIR"}`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, reversed)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("a reverse model-authored direction over the same request identities is valid, got success=%t summary=%q", res.Success, res.Summary)
	}
	if source, sink, ok := types.CallChainOrderedEndpointHints(*mu.RequestModel()); !ok || source != "gate.Run" || sink != "buildAnalysisIR" {
		t.Fatalf("entity/exact-target order must not rewrite typed direction: %q,%q,%t", source, sink, ok)
	}

	discovered := strings.Replace(withExact,
		`{"source":"buildAnalysisIR", "sink":"gate.Run"}`,
		`{"source":"buildAnalysisIR", "sink":"Resolved.run"}`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, discovered)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("ordered source-discovered endpoint should remain an evidence-gated investigation target, got success=%t summary=%q", res.Success, res.Summary)
	}
	if profile := mu.RequestModel().CallChainEndpointProfile; profile == nil || !profile.DiscoverTerminalActive() || profile.Source != "buildAnalysisIR" || profile.Sink != "" {
		t.Fatalf("analyzer-discovered conceptual sink was not demoted to discover_terminal mode: %+v", profile)
	}

	withOneExact := strings.Replace(withEndpoints,
		`"question_kind": "call_chain",`,
		`"question_kind": "call_chain", "exact_targets": ["buildAnalysisIR"],`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, withOneExact)
	if res.Success || !strings.Contains(res.Summary, "contains only one symbol endpoint") {
		t.Fatalf("one exact endpoint must not suppress the typed endpoint pair, got success=%t summary=%q", res.Success, res.Summary)
	}

	twoEntity := strings.Replace(withEndpoints,
		`["analyzer.go", "buildAnalysisIR", "gate.Run", "AnalysisIR"]`,
		`["analyzer.go", "buildAnalysisIR", "gate.Run"]`,
		1,
	)
	res, mu = runEmitAnalysisWithObjective(t, objective, twoEntity)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("exactly two typed symbol entities with ordered endpoints should succeed, got success=%t summary=%q", res.Success, res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainRepairsMissingAxisWithoutDroppingOrderedEndpoints(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := withV4Required(`{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["buildAnalysisIR", "gate.Run", "call", "sequence"],
		"entities": ["buildAnalysisIR", "gate.Run"],
		"question_kind": "call_chain",
		"call_chain_endpoints": {"source":"buildAnalysisIR", "sink":"gate.Run", "sink_mode":"exact"}
	}`)
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("missing redundant axis should be repaired, got success=%t summary=%q", res.Success, res.Summary)
	}
	rm := mu.RequestModel()
	if rm.PredicateAxis != types.AxisCall {
		t.Fatalf("predicate axis=%q, want call", rm.PredicateAxis)
	}
	if source, sink, ok := types.CallChainOrderedEndpointHints(*rm); !ok || source != "buildAnalysisIR" || sink != "gate.Run" {
		t.Fatalf("ordered endpoints were lost after repair: %q,%q,%t", source, sink, ok)
	}
	if !strings.Contains(res.Summary, "normalized missing predicate_axis to call") {
		t.Fatalf("summary must disclose deterministic repair: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainRejectsExplicitNonCallAxis(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := withV4Required(`{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["buildAnalysisIR", "gate.Run", "call", "sequence"],
		"entities": ["buildAnalysisIR", "gate.Run"],
		"question_kind": "call_chain",
		"predicate_axis": "configure",
		"call_chain_endpoints": {"source":"buildAnalysisIR", "sink":"gate.Run", "sink_mode":"exact"}
	}`)
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if res.Success || !strings.Contains(res.Summary, "contradicts predicate_axis=configure") {
		t.Fatalf("explicit axis contradiction must retry, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("contradictory analysis must not persist: %+v", mu.RequestModel())
	}
}

func TestEmitAnalysis_SourceCallChainMayDiscoverRequestedRuntimeDestination(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "处理 kind=json 的输入时，最终由哪个类处理？从 run_pipeline 到这个类的解析链路是什么？"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["run_pipeline","kind=json","resolve","registry","handler"],
		"entities":["run_pipeline","kind=json"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"run_pipeline","sink":"","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("discover destination must remain a valid directional investigation: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if !profile.DiscoverSinkActive() || profile.Sink != "" {
		t.Fatalf("persisted discovery profile wrong: %+v", profile)
	}
	if source, sink, ok := types.CallChainOrderedEndpointHints(*mu.RequestModel()); ok || source != "" || sink != "" {
		t.Fatalf("discover profile must not become an exact hard-gate pair: %q %q %t", source, sink, ok)
	}
}

func TestEmitAnalysis_SourceCallChainRoleBoundEndpointsNormalizeToDiscoverPath(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "从 CLI 入口到底层 HTTP 重试发送的完整调用链是怎样的？"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["CLI","HTTP","retry","call chain"],
		"entities":["main","HttpTransport"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"main","sink":"HttpTransport","sink_mode":"exact"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("role-bound call-chain endpoints should normalize without retry: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() || profile.Source != "" || profile.Sink != "" {
		t.Fatalf("pre-scan guesses must not survive as endpoint authority: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized to discover_path") {
		t.Fatalf("authority removal must remain auditable: %q", res.Summary)
	}
	anchors := types.CompileRequiredMechanismAnchors(*mu.RequestModel(), types.AnswerContract{}, types.QFCallChain, nil)
	if len(anchors) != 0 {
		t.Fatalf("role-bound path must not hard-require guessed answer anchors: %+v", anchors)
	}
}

func TestEmitAnalysis_SourceCallChainAcceptsExplicitDiscoverPathWithoutRuntimeSelectionAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释从命令入口到最终网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":[],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil || !mu.RequestModel().CallChainEndpointProfile.DiscoverPathActive() {
		t.Fatalf("explicit discover_path should persist as an authority-free call-chain profile: success=%t summary=%q model=%+v", res.Success, res.Summary, mu.RequestModel())
	}
	if callChainDiscoverySelectionRequired(&types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: *mu.RequestModel()}}) {
		t.Fatal("discover_path must not inherit runtime implementation selection proof")
	}
}

func TestEmitAnalysis_SourceCallChainDiscoverPathCarriesExplicitRuntimeSelectionAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "从日志入口到写出端的完整路径是什么？运行时具体的 sink 是如何被选择出来的？"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["日志入口","写出端","sink"],
		"entities":[],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{
			"source":"","sink":"","sink_mode":"discover_path",
			"runtime_selection_required":true,
			"runtime_selection_source_quote":"运行时具体的 sink 是如何被选择出来的"
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("anchored runtime-selection declaration should persist: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() || !profile.RequiresRuntimeSelectionEvidence() {
		t.Fatalf("endpoint identity demotion must retain selection authority: %+v", profile)
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: *mu.RequestModel()}}
	if !callChainDiscoverySelectionRequired(ctx) {
		t.Fatal("explicit runtime-selection question must require typed selection evidence in discover_path mode")
	}
}

func TestEmitAnalysis_MechanismFlowCarriesRuntimeSelectionWithoutCallChainEndpoints(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释首次完整输出为什么使用 emit_answer_document，而重试补丁什么时候改用 emit_answer_document_patch"
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["emit_answer_document","emit_answer_document_patch","retry"],
		"entities":["emit_answer_document","emit_answer_document_patch"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.95,
			"dimensions":[{
				"index":1,
				"label":"首次完整输出 vs retry patch",
				"role":"function_or_purpose",
				"source_quote":"首次完整输出为什么使用 emit_answer_document，而重试补丁什么时候改用 emit_answer_document_patch",
				"required":true
			}]
		},
		"call_chain_endpoints":{
			"source":"","sink":"","sink_mode":"exact",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":""
		},
		"runtime_selection_profile":{
			"is_selection_question":true,
			"source_quote":"首次完整输出为什么使用 emit_answer_document，而重试补丁什么时候改用 emit_answer_document_patch",
			"confidence":0.95
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("runtime selection must remain reachable outside call_chain: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || profile.Active() || !profile.RequiresRuntimeSelectionEvidence() {
		t.Fatalf("mechanism/flow runtime-only carrier was dropped or promoted to endpoint authority: %+v", profile)
	}
}

func TestEmitAnalysis_RuntimeSelectionProfileRejectsContradictoryFalseQuoteWithoutRequestScan(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "first full versus patch retry"
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["tools"],
		"entities":["tool_a","tool_b"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"call_chain_endpoints":{
			"source":"","sink":"","sink_mode":"exact",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":""
		},
		"runtime_selection_profile":{
			"is_selection_question":false,
			"source_quote":"first full versus patch retry",
			"confidence":0.95
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if res.Success || !strings.Contains(res.Summary, `is_selection_question=false requires source_quote=""`) {
		t.Fatalf("typed selection contradiction must fail loud: success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("contradictory typed selection carrier must not persist: %+v", mu.RequestModel())
	}
}

func TestEmitAnalysis_RuntimeArtifactDropsStaleDiscoverWhenDedicatedSelectionIsFalse(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "分析 trace 中 app-100 的线程状态和 CPU 频率"
	payload := `{
		"intent":"trace",
		"scenario":"performance_bottleneck",
		"complexity":"moderate",
		"keywords":["trace","app-100","state","frequency"],
		"entities":["app-100"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"call_chain_endpoints":{
			"source":"app-100","sink":"","sink_mode":"discover",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":""
		},
		"runtime_selection_profile":{
			"is_selection_question":false,
			"source_quote":"",
			"confidence":1.0
		},
		"runtime_artifact_scope_profile":{
			"requested_scope":"unspecified",
			"confidence":0.9
		},
		"runtime_target_profile":{
			"declaration":"named_target",
			"source_quote":"app-100",
			"confidence":1.0
		},
		"runtime_targets":[{
			"kind":"thread","thread":"app-100",
			"source":"user_explicit","confidence":1.0
		}],
		"runtime_question_profile":{
			"scope":"bounded_fact_set",
			"fact_families":["target_scheduler_state","frequency_residency"],
			"confidence":0.95
		}
	}`
	mu := types.NewMutableState(objective)
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, AttachedHitrace: "inline trace"},
		json.RawMessage(withV4Required(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("runtime artifact analysis should converge: success=%t summary=%q", res.Success, res.Summary)
	}
	if got := mu.RequestModel().CallChainEndpointProfile; got != nil {
		t.Fatalf("stale source-code endpoint carrier survived explicit false selection profile: %+v", got)
	}
	if !strings.Contains(res.Summary, "dropped stale source-code call_chain_endpoints for runtime artifact") {
		t.Fatalf("authority removal must remain auditable: %q", res.Summary)
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: *mu.RequestModel()}}
	if callChainDiscoverySelectionRequired(ctx) {
		t.Fatal("finite runtime facts must not inherit source-code selection evidence completion gates")
	}
}

func TestMissingEmitAnalysisRequiredTopLevelFieldsRequiresRelationCarrierOnlyForCurrentSource(t *testing.T) {
	sourceFlow := json.RawMessage(`{
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"runtime_artifact_scope_profile":{"requested_scope":"not_applicable"},
		"current_source_explanation_profile":{"modes":["explain_current_mechanism"]}
	}`)
	if missing := missingEmitAnalysisRequiredTopLevelFields(sourceFlow, false); !slices.Contains(missing, "call_chain_endpoints") || !slices.Contains(missing, "runtime_selection_profile") {
		t.Fatalf("current-source flow must fail loud instead of silently losing runtime selection: %v", missing)
	}

	runtimeFlow := json.RawMessage(`{
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window"},
		"external_observation_policy":{"current_source_mode":"exclude"}
	}`)
	if missing := missingEmitAnalysisRequiredTopLevelFields(runtimeFlow, false); slices.Contains(missing, "call_chain_endpoints") {
		t.Fatalf("runtime-artifact-only flow must retain the inert legacy-provider default: %v", missing)
	}

	definitionDiagram := json.RawMessage(`{
		"question_kind":"mechanism",
		"predicate_axis":"define",
		"runtime_artifact_scope_profile":{"requested_scope":"not_applicable"}
	}`)
	if missing := missingEmitAnalysisRequiredTopLevelFields(definitionDiagram, true); !slices.Contains(missing, "call_chain_endpoints") || !slices.Contains(missing, "runtime_selection_profile") {
		t.Fatalf("required current-source diagram must carry the explicit relation/selection discriminator: %v", missing)
	}
}

func TestEmitAnalysisMissingRelationCarrierRetryReTeachesSelectionDecision(t *testing.T) {
	got := emitAnalysisMissingTopLevelFieldsSummary([]string{"call_chain_endpoints", "runtime_selection_profile"})
	for _, want := range []string{
		"emit only ordered endpoint identity",
		"runtime_selection_profile",
		"do not default false merely to satisfy presence",
		"source_quote",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("selection-carrier retry lost %q: %s", want, got)
		}
	}
	ordinary := emitAnalysisMissingTopLevelFieldsSummary([]string{"runtime_question_profile"})
	if strings.Contains(ordinary, "runtime_selection_profile") {
		t.Fatalf("unrelated missing-field retry gained selection teaching: %s", ordinary)
	}
}

func TestEmitAnalysis_SourceCallChainRejectsUnanchoredRuntimeSelectionDeclaration(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释从命令入口到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":[],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{
			"source":"","sink":"","sink_mode":"discover_path",
			"runtime_selection_required":true,
			"runtime_selection_source_quote":"runtime provider selection"
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if res.Success || !strings.Contains(res.Summary, "contiguous verbatim CURRENT-request") {
		t.Fatalf("invented selection authority must fail loud: success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("rejected selection authority must not persist: %+v", mu.RequestModel())
	}
}

func TestEmitAnalysis_SourceCallChainRejectsQuoteWhenRuntimeSelectionIsFalse(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释从命令入口到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":[],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{
			"source":"","sink":"","sink_mode":"discover_path",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":"调用路径"
		}
	}`
	res, _ := runEmitAnalysisWithObjective(t, objective, payload)
	if res.Success || !strings.Contains(res.Summary, "runtime_selection_profile=false conflicts with the legacy internal runtime-selection carrier") {
		t.Fatalf("false selection declaration with a quote must fail loud: success=%t summary=%q", res.Success, res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainDiscoverFilePathDemotesWithoutRetry(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "从 CLI 入口到底层 HTTP 重试发送的完整调用链是怎样的？"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["CLI","HTTP","retry","call chain"],
		"entities":["packages/cli/src/main.ts","HttpTransport"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"packages/cli/src/main.ts","sink":"","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("file-path discover candidate must demote without an analyzer retry: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() {
		t.Fatalf("file path must not survive as endpoint authority: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized to discover_path") {
		t.Fatalf("authority demotion must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainNormalizesDiscoverModeWithTwoNamedEndpoints(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["buildAnalysisIR","gate.Run","call","sequence"],
		"entities":["buildAnalysisIR","gate.Run"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"buildAnalysisIR","sink":"gate.Run","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("two ordered endpoint fields must survive a locally contradictory mode enum: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.ExactActive() || profile.Source != "buildAnalysisIR" || profile.Sink != "gate.Run" {
		t.Fatalf("enum repair must preserve the analyzer-authored ordered endpoint pair as exact: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized call_chain_endpoints sink_mode from discover to exact") {
		t.Fatalf("enum repair must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainNormalizesDiscoverPathModeWithTwoNamedEndpoints(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["buildAnalysisIR","gate.Run","call","sequence"],
		"entities":["buildAnalysisIR","gate.Run"],
		"exact_targets":["buildAnalysisIR","gate.Run"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"buildAnalysisIR","sink":"gate.Run","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("two ordered endpoint fields must survive a discover_path enum mismatch: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.ExactActive() || profile.Source != "buildAnalysisIR" || profile.Sink != "gate.Run" {
		t.Fatalf("enum repair must preserve the current-request ordered endpoint pair as exact: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized call_chain_endpoints sink_mode from discover_path to exact") {
		t.Fatalf("discover_path enum repair must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainDiscoverPathEnumRepairDoesNotMintUnprovenEndpointAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释命令入口到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":["Invented.entry","Invented.exit"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"Invented.entry","sink":"Invented.exit","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("unproven endpoint candidates should normalize without becoming authority: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() || profile.Source != "" || profile.Sink != "" {
		t.Fatalf("provenance authority must still demote unproven endpoints after enum repair: %+v", profile)
	}
	for _, want := range []string{
		"normalized call_chain_endpoints sink_mode from discover_path to exact",
		"normalized to discover_path",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("enum repair and authority demotion must both remain auditable; missing %q in %q", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_SourceCallChainEnumRepairDoesNotMintUnprovenEndpointAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释命令入口到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":["Invented.entry","Invented.exit"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"Invented.entry","sink":"Invented.exit","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("unproven endpoint candidates should be normalized without becoming authority: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() || profile.Source != "" || profile.Sink != "" {
		t.Fatalf("enum repair must still defer unproven endpoint selection to grounded exploration: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized to discover_path") {
		t.Fatalf("provenance demotion after enum repair must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainNormalizesDiscoverPathWithNamedSourceToDiscoverTerminal(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释从 main 到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":["main"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"main","sink":"","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("one named request source must survive a discover_path enum mismatch: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverTerminalActive() || profile.Source != "main" || profile.Sink != "" {
		t.Fatalf("named-source conceptual-terminal request must use discover_terminal: %+v", profile)
	}
	if !strings.Contains(res.Summary, "normalized call_chain_endpoints sink_mode from discover_path to discover_terminal") {
		t.Fatalf("one-source enum repair must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainNamedSourceProvenanceSurvivesSplitEntityRoster(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "FastTokenizer.tokenize 最终如何调用 Rust 实现"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["FastTokenizer","tokenize","Rust","call path"],
		"entities":["FastTokenizer","tokenize","Rust"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"FastTokenizer.tokenize","sink":"","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("verbatim method-qualified source must survive a split entity roster: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverTerminalActive() || profile.Source != "FastTokenizer.tokenize" {
		t.Fatalf("verbatim typed endpoint candidate did not retain source authority: %+v", profile)
	}
	if strings.Contains(res.Summary, "discover source was not an exact current-request code identity") {
		t.Fatalf("verbatim endpoint was falsely demoted after entity splitting: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainDiscoverPathNamedSourceRepairDoesNotMintUnprovenAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释命令入口到网络发送的调用路径"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["entry","network","call path"],
		"entities":["Invented.entry"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"Invented.entry","sink":"","sink_mode":"discover_path"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("unproven one-source candidate should normalize without becoming authority: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverPathActive() || profile.Source != "" || profile.Sink != "" {
		t.Fatalf("provenance authority must demote the repaired unproven source: %+v", profile)
	}
	for _, want := range []string{
		"normalized call_chain_endpoints sink_mode from discover_path to discover_terminal",
		"normalized to discover_path",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("enum repair and authority demotion must both remain auditable; missing %q in %q", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_SourceCallChainRejectsDiscoverEmptyWithUniqueTypedExactDestination(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["buildAnalysisIR","gate.Run","call","sequence"],
		"entities":["buildAnalysisIR","gate.Run"],
		"exact_targets":["buildAnalysisIR","gate.Run"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"buildAnalysisIR","sink":"","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if res.Success || !strings.Contains(res.Summary, "sink_mode=discover contradicts typed exact_targets") ||
		!strings.Contains(res.Summary, `unique named destination is "gate.Run"`) ||
		!strings.Contains(res.Summary, "typed no_directed_path") {
		t.Fatalf("unique typed destination must not be silently discarded by discover-empty: success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("rejected named-destination contradiction must not persist: %+v", mu.RequestModel())
	}
}

func TestEmitAnalysis_SourceCallChainRequiredSequencePairRepairsDiscardedSink(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请用 Mermaid 时序图展示 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["buildAnalysisIR","gate.Run","call","sequence"],
		"entities":["buildAnalysisIR","gate.Run","unrelated.Helper"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.9,
			"rationale":"用户要求时序图",
			"dimensions":[{"index":1,"label":"Mermaid 时序图","role":"diagram","required":true,"source_quote":"用 Mermaid 时序图"}]
		},
		"diagram_hint":{
			"kind":"sequence",
			"required":true,
			"relation_scope_quote":"buildAnalysisIR 到 gate.Run 的调用顺序",
			"participants":[
				{"identity":"buildAnalysisIR","role":"incident_required","source_quote":"buildAnalysisIR 到 gate.Run 的调用顺序"},
				{"identity":"gate.Run","role":"incident_required","source_quote":"buildAnalysisIR 到 gate.Run 的调用顺序"}
			]
		},
		"call_chain_endpoints":{
			"source":"buildAnalysisIR",
			"sink":"",
			"sink_mode":"discover",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":""
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("typed diagram pair should repair the discarded sink: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.ExactActive() || profile.Source != "buildAnalysisIR" || profile.Sink != "gate.Run" {
		t.Fatalf("required sequence pair was not preserved as ordered exact endpoints: %+v", profile)
	}
	if !strings.Contains(res.Summary, "required typed call diagram") {
		t.Fatalf("diagram participant repair must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceCallChainRequiredSequenceScopeRepairsDiscardedSinkWithEmptyParticipants(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "请用 Mermaid 时序图展示 analyzer.go 里 buildAnalysisIR 到 gate.Run 的调用顺序"
	payload := `{
		"intent":"trace",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["buildAnalysisIR","gate.Run","call","sequence"],
		"entities":["internal/agent/analyzer.go","buildAnalysisIR","gate.Run"],
		"question_kind":"call_chain",
		"predicate_axis":"call",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.9,
			"rationale":"用户要求时序图",
			"dimensions":[{"index":1,"label":"Mermaid 时序图","role":"diagram","required":true,"source_quote":"用 Mermaid 时序图"}]
		},
		"diagram_hint":{
			"kind":"sequence",
			"required":true,
			"relation_scope_quote":"buildAnalysisIR 到 gate.Run 的调用顺序",
			"participants":[]
		},
		"call_chain_endpoints":{
			"source":"buildAnalysisIR",
			"sink":"",
			"sink_mode":"discover",
			"runtime_selection_required":false,
			"runtime_selection_source_quote":""
		}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("typed diagram relation scope should repair the discarded sink: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.ExactActive() || profile.Source != "buildAnalysisIR" || profile.Sink != "gate.Run" {
		t.Fatalf("required sequence relation scope was not preserved as ordered exact endpoints: %+v", profile)
	}
	if !strings.Contains(res.Summary, "relation scope contains one unique other typed code identity") {
		t.Fatalf("diagram relation-scope repair must remain auditable: %q", res.Summary)
	}
}

func TestCallChainUniqueExactDestinationUsesSetIdentityWithoutGuessing(t *testing.T) {
	discover := &types.CallChainEndpointProfile{Source: "pkg::Start", SinkMode: types.CallChainSinkResolutionDiscover}
	for _, tc := range []struct {
		name    string
		profile *types.CallChainEndpointProfile
		targets []string
		want    string
		ok      bool
	}{
		{name: "unique", profile: discover, targets: []string{"pkg::Start", "svc::Finish"}, want: "svc::Finish", ok: true},
		{name: "path does not add endpoint ambiguity", profile: discover, targets: []string{"pkg::Start", "svc::Finish", "src/pipeline.rs"}, want: "svc::Finish", ok: true},
		{name: "source absent", profile: discover, targets: []string{"svc::Finish"}},
		{name: "no destination", profile: discover, targets: []string{"pkg::Start"}},
		{name: "two destinations", profile: discover, targets: []string{"pkg::Start", "svc::Finish", "audit::Finish"}},
		{name: "exact profile", profile: &types.CallChainEndpointProfile{Source: "pkg::Start", Sink: "svc::Finish", SinkMode: types.CallChainSinkResolutionExact}, targets: []string{"pkg::Start", "svc::Finish"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := callChainUniqueExactDestination(tc.profile, tc.targets)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("destination=%q ok=%t, want %q/%t", got, ok, tc.want, tc.ok)
			}
		})
	}
	if issue := validateCallChainEndpointWireShape(string(types.ReqCallChain), types.AxisCall, true, discover, []string{"pkg::Start", "svc::Finish"}); issue != "" {
		t.Fatalf("runtime artifact carrier must stay outside source-code endpoint admission, got %q", issue)
	}
}

func TestEmitAnalysis_MechanismWithProvenancedOrderedCallProfilePromotesToCallChain(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "处理 kind=json 的输入时，从 run_pipeline 到最终处理类的解析链路是什么？"
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["run_pipeline","resolve","registry","handler"],
		"entities":["run_pipeline","kind=json"],
		"question_kind":"mechanism",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"run_pipeline","sink":"","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("typed mechanism/call-chain contradiction should normalize, success=%t summary=%q", res.Success, res.Summary)
	}
	rm := mu.RequestModel()
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqCallChain ||
		rm.CallChainEndpointProfile == nil || !rm.CallChainEndpointProfile.DiscoverSinkActive() {
		t.Fatalf("ordered discover profile must survive under normalized call-chain family: %+v", rm)
	}
	if !strings.Contains(res.Summary, "normalized question_kind=mechanism to call_chain") {
		t.Fatalf("normalization must remain auditable: %q", res.Summary)
	}
}

func TestEmitAnalysis_MechanismProfileWithoutExactSourceProvenanceDoesNotPromote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "解释插件装载机制"
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["plugin","registry","handler"],
		"entities":["Invented.entry","registry"],
		"question_kind":"mechanism",
		"predicate_axis":"call",
		"call_chain_endpoints":{"source":"Invented.entry","sink":"","sink_mode":"discover"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("unproven optional profile should be dropped without changing the mechanism question: success=%t summary=%q", res.Success, res.Summary)
	}
	rm := mu.RequestModel()
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqMechanism || rm.CallChainEndpointProfile != nil {
		t.Fatalf("unproven source must not promote or retain ordered authority: %+v", rm)
	}
}

func TestEmitAnalysis_SourceCallChainSemanticSinkCandidateCannotBecomeExactAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "FastTokenizer.tokenize 最终怎么调到 Rust 实现？请给出完整跨语言调用链"
	payload := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["FastTokenizer.tokenize", "Rust", "tokenize_bytes", "call"],
		"entities": ["FastTokenizer.tokenize", "tokenize_bytes"],
		"question_kind": "call_chain",
		"predicate_axis": "call",
		"call_chain_endpoints": {"source":"FastTokenizer.tokenize", "sink":"tokenize_bytes"}
	}`
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("semantic sink resolved by current-checkout pre-scan must stay a call chain: success=%t summary=%q", res.Success, res.Summary)
	}
	profile := mu.RequestModel().CallChainEndpointProfile
	if profile == nil || !profile.DiscoverTerminalActive() || profile.Source != "FastTokenizer.tokenize" || profile.Sink != "" {
		t.Fatalf("resolved semantic endpoint must remain a grounded terminal-discovery target, got %+v", profile)
	}
	if !strings.Contains(res.Summary, "demoted to discover_terminal") {
		t.Fatalf("resolved endpoint demotion must remain auditable: %q", res.Summary)
	}
}

func TestReconcileSetValuedRoleLocatePredicates(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:        false,
		IsRoleLocateLookup:    true,
		IsCategoryEnumeration: true,
	}
	got, reason := reconcileSetValuedRoleLocatePredicates(types.IntentEnumerate, preds, nil)
	if reason == "" {
		t.Fatal("expected set-valued role-locate normalization reason")
	}
	if got.IsRoleLocateLookup {
		t.Fatalf("set-valued role-locate must be normalized out of scalar lane: %+v", got)
	}
	if !got.IsCategoryEnumeration {
		t.Fatalf("category enumeration signal must be preserved: %+v", got)
	}

	scalar := types.SemanticPredicates{
		IsScalarAnswer:     true,
		IsRoleLocateLookup: true,
	}
	got, reason = reconcileSetValuedRoleLocatePredicates(types.IntentReturnValue, scalar, nil)
	if reason != "" || !got.IsRoleLocateLookup {
		t.Fatalf("scalar role-locate must remain intact, got %+v reason=%q", got, reason)
	}
}

func TestNormalizeRoleBindingScalarShape_StabilizesMechanismRoleLookup(t *testing.T) {
	preds := types.SemanticPredicates{
		IsRelationalLookup: true,
	}
	profile := &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleAgent},
		SourceQuotes:           []string{"agent"},
		Confidence:             0.9,
	}

	gotPreds, gotSubject, gotTopics, reason := normalizeRoleBindingScalarShape(
		types.IntentExplain,
		"mechanism",
		types.AxisCall,
		preds,
		profile,
		types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.4},
		[]types.SubTopic{{Summary: "follow the role binding"}},
	)

	if reason == "" {
		t.Fatal("expected normalization reason")
	}
	if !gotPreds.IsScalarAnswer || !gotPreds.IsRoleLocateLookup {
		t.Fatalf("role-binding lookup must be scalar role-locate: %+v", gotPreds)
	}
	if gotPreds.IsRelationalLookup || gotPreds.IsCategoryEnumeration {
		t.Fatalf("scalar role binding must not keep relation/category lane: %+v", gotPreds)
	}
	if gotSubject.Kind != types.SubjectTypeName {
		t.Fatalf("answer subject = %q, want type_name", gotSubject.Kind)
	}
	if len(gotTopics) != 0 {
		t.Fatalf("exploratory subtopics should be dropped for scalar role binding: %+v", gotTopics)
	}
}

func TestNormalizeRoleBindingScalarShape_PreservesSetValuedEnumeration(t *testing.T) {
	preds := types.SemanticPredicates{
		IsCategoryEnumeration: true,
	}
	profile := &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleAgent},
		SourceQuotes:           []string{"agents"},
		Confidence:             0.9,
	}

	gotPreds, gotSubject, _, reason := normalizeRoleBindingScalarShape(
		types.IntentEnumerate,
		"enumeration",
		types.AxisCall,
		preds,
		profile,
		types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.7},
		nil,
	)

	if reason != "" {
		t.Fatalf("enumeration must not be scalar-normalized, got reason=%q", reason)
	}
	if gotPreds.IsScalarAnswer || gotPreds.IsRoleLocateLookup || !gotPreds.IsCategoryEnumeration {
		t.Fatalf("enumeration predicates should be preserved: %+v", gotPreds)
	}
	if gotSubject.Kind != types.SubjectGeneric {
		t.Fatalf("enumeration subject should be preserved, got %+v", gotSubject)
	}
}

func TestEmitAnalysis_Execute_NormalizesRoleBindingMechanismToScalarRoleLookup(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("哪个 agent 可以调用 SubAgent?")
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["agent", "subagent", "call"],
		"entities": ["SubAgent"],
		"question_kind": "mechanism",
		"predicate_axis": "call",
		"answer_subject": {"kind": "generic", "confidence": 0.4},
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": true,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.8
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["agent"],
			"source_quotes": ["agent"],
			"confidence": 0.9
		}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("role-binding mechanism should normalize instead of rejecting, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "role_locate=true") || !strings.Contains(res.Summary, "required_roles=agent") {
		t.Fatalf("summary should disclose normalized role lookup and role profile, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !rm.Predicates.IsScalarAnswer || !rm.Predicates.IsRoleLocateLookup || rm.Predicates.IsRelationalLookup {
		t.Fatalf("predicates not normalized to scalar role lookup: %+v", rm.Predicates)
	}
	if rm.AnswerSubject.Kind != types.SubjectTypeName {
		t.Fatalf("answer_subject.kind = %q, want type_name", rm.AnswerSubject.Kind)
	}
	if rm.AnswerRoleProfile == nil || !rm.AnswerRoleProfile.Active() ||
		rm.AnswerRoleProfile.RequiredCandidateRoles[0] != types.AnswerCandidateRoleAgent {
		t.Fatalf("answer_role_profile not persisted as agent: %+v", rm.AnswerRoleProfile)
	}
	if got := types.ResolveQuestionFamily(*rm); got != types.QFRoleLookup {
		t.Fatalf("family = %q, want role_lookup", got)
	}
}

func TestEmitAnalysis_SetValuedRoleLocateNormalizesToEnumeration(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数。")
	payload := `{
		"intent": "enumerate",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["internal/analysis", "subpackage", "entry", "function"],
		"entities": ["internal/analysis"],
		"question_kind": "enumeration",
		"answer_subject": {"kind": "function_name"},
		"completeness_obligation": {"required": true, "source_quote": "所有子包"},
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": true,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.8
		}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("set-valued role-locate should normalize instead of rejecting, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "set-valued role-locate normalized") {
		t.Fatalf("summary should disclose predicate normalization, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.Predicates.IsRoleLocateLookup {
		t.Fatalf("role-locate scalar lane should be cleared: %+v", rm.Predicates)
	}
	if !rm.Predicates.IsCategoryEnumeration || !rm.Predicates.IsRelationalLookup {
		t.Fatalf("set-valued enumeration signals should be preserved: %+v", rm.Predicates)
	}
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) {
		t.Fatalf("normalized exhaustive enumeration should still require member_set handoff: %+v", rm.Predicates)
	}
}

func TestEmitAnalysis_RequestedAnswerDimensionsSoftProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	objective := "对比最近两次提交的代码 diff，再结合当前源码分析它们分别影响了哪些当前实现链路；请说明每次提交的 diff 线索、当前关键代码、作用和影响，不要只给 commit id。"
	mu := types.NewMutableState(objective)
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["commit", "diff", "current source", "impact"],
		"entities": ["commit", "diff"],
		"question_kind": "history",
		"requested_answer_dimensions": {
			"is_dimensioned_answer": true,
			"confidence": 0.9,
			"dimensions": [
				{"label": "diff 线索", "role": "diff_clue", "source_quote": "diff 线索", "required": true, "index": 1},
				{"label": "当前关键代码", "role": "current_key_code", "source_quote": "当前关键代码", "required": true, "index": 2},
				{"label": "not in request", "role": "impact", "source_quote": "not in request", "required": true, "index": 3},
				{"label": "影响", "role": "impact", "source_quote": "影响", "required": true, "index": 4}
			]
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("profile should be soft-normalized, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		t.Fatalf("requested dimensions not persisted: rm=%+v", rm)
	}
	if len(rm.RequestedAnswerDimensions.Dimensions) != 3 {
		t.Fatalf("dimensions=%d want 3: %+v", len(rm.RequestedAnswerDimensions.Dimensions), rm.RequestedAnswerDimensions.Dimensions)
	}
	if !strings.Contains(res.Summary, `answer_dimensions=["diff 线索","当前关键代码","影响"]`) {
		t.Fatalf("summary should report normalized dimension labels, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "requested_answer_dimensions ignored unanchored dimension") {
		t.Fatalf("summary should warn for unanchored optional dimension, got %q", res.Summary)
	}
}

func TestEmitAnalysis_RequiredDiagramDimensionProvidesPreciseVisualAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	const objective = "请枚举 LoopController 的生产实现并给出 Mermaid 类图。"
	mu := types.NewMutableState(objective)
	payload := withV4Required(`{
		"intent":"enumerate",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["LoopController","implementations","class diagram"],
		"entities":["LoopController"],
		"question_kind":"type_relation",
		"predicate_axis":"implement",
		"diagram_hint":{"kind":"call_dag","required":true,"relation_scope_quote":"LoopController 的生产实现并给出 Mermaid 类图","participants":[
			{"identity":"LoopController","source_quote":"LoopController","role":"incident_required"}
		]},
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.97,
			"dimensions":[
				{"label":"Mermaid 类图","role":"diagram","source_quote":"Mermaid 类图","required":true,"index":1}
			]
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed required diagram dimension should authorize the visual: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		t.Fatalf("required diagram authority was lost: %+v", rm)
	}
	if rm.DiagramHint.Kind != types.DiagramArchitecture {
		t.Fatalf("implement axis must use architecture semantics, got %+v", rm.DiagramHint)
	}
	if !strings.Contains(res.Summary, "normalized diagram_hint.kind from call_dag to architecture") {
		t.Fatalf("kind reconciliation should remain auditable: %s", res.Summary)
	}
}

func TestReconcileRequiredDiagramRelationAxis_UsesOnlyTypedCurrentTurnContract(t *testing.T) {
	tests := []struct {
		name   string
		intent types.Intent
		hint   *types.DiagramHint
		axis   types.PredicateAxis
		want   types.PredicateAxis
		warn   bool
	}{
		{name: "required flow closes define escape", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true}, axis: types.AxisDefine, want: types.AxisFlow, warn: true},
		{name: "required sequence closes unknown escape", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true}, axis: types.AxisUnknown, want: types.AxisFlow, warn: true},
		{name: "required call dag owns call edges", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramCallDAG, Required: true}, axis: types.AxisDefine, want: types.AxisCall, warn: true},
		{name: "optional visual remains guidance", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramFlow, Required: false}, axis: types.AxisDefine, want: types.AxisDefine},
		{name: "architecture grouping remains presentation", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true}, axis: types.AxisDefine, want: types.AxisDefine},
		{name: "trace keeps independent causal authority", intent: types.IntentTrace, hint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true}, axis: types.AxisDefine, want: types.AxisDefine},
		{name: "root cause keeps independent causal authority", intent: types.IntentRootCause, hint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true}, axis: types.AxisDefine, want: types.AxisDefine},
		{name: "precise register axis is preserved", intent: types.IntentExplain, hint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true}, axis: types.AxisRegister, want: types.AxisRegister},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHint, gotAxis, warning := reconcileRequiredDiagramRelationAxis(tt.intent, tt.hint, tt.axis)
			if gotHint != tt.hint {
				t.Fatalf("hint identity changed: got=%p want=%p", gotHint, tt.hint)
			}
			if gotAxis != tt.want {
				t.Fatalf("axis=%q want %q", gotAxis, tt.want)
			}
			if (warning != "") != tt.warn {
				t.Fatalf("warning=%q wantPresent=%t", warning, tt.warn)
			}
		})
	}
}

func TestEmitAnalysis_RequiredFlowRelationCannotLoseEdgeOwnershipAfterPatch(t *testing.T) {
	const objective = "对比 emit_answer_document 和 emit_answer_document_patch，并用 Mermaid 小流程图说明它们在 finalizer 里的关系。"
	payload := withV4Required(`{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["emit_answer_document","emit_answer_document_patch","finalizer"],
		"entities":["emit_answer_document","emit_answer_document_patch","finalizer"],
		"question_kind":"mechanism",
		"predicate_axis":"define",
		"diagram_hint":{"kind":"flow","required":true,"relation_scope_quote":"emit_answer_document 和 emit_answer_document_patch，并用 Mermaid 小流程图说明它们在 finalizer 里的关系","participants":[
			{"identity":"emit_answer_document","role":"incident_required","source_quote":"emit_answer_document"},
			{"identity":"emit_answer_document_patch","role":"incident_required","source_quote":"emit_answer_document_patch"},
			{"identity":"finalizer","role":"incident_required","source_quote":"finalizer"}
		]}
	}`)
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)
	if !res.Success {
		t.Fatalf("typed relation contract rejected: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.PredicateAxis != types.AxisFlow {
		t.Fatalf("required flow relation axis was not preserved: %+v", rm)
	}
	if !strings.Contains(res.Summary, `normalized predicate_axis from "define" to flow`) {
		t.Fatalf("typed reconciliation must remain auditable: %s", res.Summary)
	}

	view := types.BuildAnswerSemanticView(&types.AnalysisIR{RequestModel: *rm}, nil)
	if view == nil || view.RelationAxis != types.AxisFlow {
		t.Fatalf("semantic view lost reconciled relation axis: %+v", view)
	}
	patched := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "relation", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid",
			Body: "flowchart LR\n  Full[emit_answer_document] --> Patch[emit_answer_document_patch]\n",
		},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(patched, view, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingRelationAnchor {
		t.Fatalf("metadata-free patched arrow escaped typed relation ownership: %+v", got)
	}
}

func TestRequiredDiagramRequestedDimension_DoesNotAuthorizeOtherOrOptionalRoles(t *testing.T) {
	for _, profile := range []*types.RequestedAnswerDimensionProfile{
		nil,
		{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionDiagram, Required: false}}},
		{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionOther, Required: true}}},
	} {
		if requiredDiagramRequestedDimension(profile) {
			t.Fatalf("profile must not authorize a hard visual: %+v", profile)
		}
	}
}

func TestParseDiagramHint_PreservesMoreThanTwelveExplicitlyNamedParticipants(t *testing.T) {
	const raw = "show P01 P02 P03 P04 P05 P06 P07 P08 P09 P10 P11 P12 P13 in one diagram"
	required := true
	participants := make([]emitDiagramParticipantParam, 0, 13)
	for i := 1; i <= 13; i++ {
		identity := fmt.Sprintf("P%02d", i)
		participants = append(participants, emitDiagramParticipantParam{
			Identity: identity, Role: string(types.DiagramParticipantIncidentRequired), SourceQuote: identity,
		})
	}
	got, reason, warnings := parseDiagramHint(raw, &emitDiagramHintParam{
		Kind: "architecture", Required: &required, RelationScopeQuote: raw, Participants: &participants,
	}, true)
	if reason != "" || len(warnings) != 0 {
		t.Fatalf("explicit roster should survive without arbitrary cap: reason=%q warnings=%v", reason, warnings)
	}
	if got == nil || len(got.Participants) != 13 {
		t.Fatalf("participants=%+v, want all 13 exact current-request actors", got)
	}
}

// §29.166 OBLSWEEP-1: the dropped-dimension obligation-signal mint is
// precise-anchor gated at the producer. Arm 1 pins the demotion (prose
// paraphrase quotes mint nothing and no longer force the source lane on a
// runtime-artifact run — the typed carrier for a "结合当前源码" demand is
// external_observation_policy.current_source_mode="allow"). Arm 2 pins the
// preserved lane: precise file:line / path quotes on the identical shape keep
// minting signals, requiring the lane, and surfacing the summary count.
func TestEmitAnalysis_DroppedRuntimeCurrentSourceDimensionsBecomeObligationSignals(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	const objective = "请结合当前源码解释系统如何解析 trace span，并说明证据边界。"
	makePayload := func(mechanismQuote, keyCodeQuote string) string {
		return withV4Required(fmt.Sprintf(`{
		"intent": "explain",
		"scenario": "performance_bottleneck",
		"complexity": "moderate",
		"keywords": ["trace", "span", "source", "boundary"],
		"entities": ["RenderService:DoFrame"],
		"question_kind": "mechanism",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.9
		},
		"external_observation_policy": {
			"current_source_mode": "default",
			"artifact_citation_mode": "external_only",
			"confidence": 0.95
		},
		"requested_answer_dimensions": {
			"is_dimensioned_answer": true,
			"confidence": 0.9,
			"dimensions": [
				{"label": "trace 解析规则", "role": "function_or_purpose", "source_quote": %q, "required": true, "index": 1},
				{"label": "耗时判定逻辑", "role": "current_key_code", "source_quote": %q, "required": true, "index": 2},
				{"label": "证据边界", "role": "boundary", "source_quote": "证据边界", "required": true, "index": 3}
			]
		}
	}`, mechanismQuote, keyCodeQuote))
	}
	run := func(t *testing.T, mechanismQuote, keyCodeQuote string) (*types.RequestModel, string) {
		t.Helper()
		mu := types.NewMutableState(objective)
		mu.SetPerfTrace(&types.PerfBundle{
			Observations: []types.PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "H:RenderService:DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}},
		})
		res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(makePayload(mechanismQuote, keyCodeQuote)))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !res.Success {
			t.Fatalf("runtime current-source dimensions should soft-normalize, got %q", res.Summary)
		}
		rm := mu.RequestModel()
		if rm == nil {
			t.Fatal("RequestModel not persisted")
		}
		if rm.RequestedAnswerDimensions == nil || len(rm.RequestedAnswerDimensions.Dimensions) != 1 ||
			rm.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionBoundary {
			t.Fatalf("only boundary display dimension should survive, got %+v", rm.RequestedAnswerDimensions)
		}
		return rm, res.Summary
	}

	// Arm 1 (§29.166 demotion): prose paraphrase quotes — dropped by request
	// provenance, no precise anchor — must not mint obligation signals.
	rm, summary := run(t, "span 解析规则", "帧预算阈值")
	if len(rm.CurrentSourceObligationSignals) != 0 {
		t.Fatalf("prose-only dropped dimensions must not mint obligation signals: %+v", rm.CurrentSourceObligationSignals)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneAllowedOptional {
		t.Fatalf("prose-only dropped dimensions must not force the source lane, got %s", got)
	}
	if strings.Contains(summary, "current_source_obligation_signals") {
		t.Fatalf("summary should not report obligation signals when none minted, got %q", summary)
	}

	// Arm 2 (preserved lane): precise anchors on the identical shape keep the
	// dropped obligations alive.
	rm, summary = run(t, "internal/trace/span_parser.go:88", "internal/render/frame_budget.go")
	if len(rm.CurrentSourceObligationSignals) != 2 {
		t.Fatalf("current-source obligation signals=%d want 2: %+v", len(rm.CurrentSourceObligationSignals), rm.CurrentSourceObligationSignals)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("dropped runtime current-source obligations should require source lane, got %s", got)
	}
	if !strings.Contains(summary, "current_source_obligation_signals=2") {
		t.Fatalf("summary should expose typed obligation signal count, got %q", summary)
	}
}

func TestEmitAnalysis_CurrentSourceExplanationProfileSoftProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	objective := "结合这段 trace 和当前源码解释调度链路，并说明现在实现是否还会触发"
	mu := types.NewMutableState(objective)
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["trace", "current source", "scheduler", "flow"],
		"entities": ["trace", "scheduler"],
		"question_kind": "mechanism",
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"confidence": 0.9,
			"modes": ["trace_current_flow", "bogus_future_mode"],
			"source_quotes": ["当前源码解释", "现在实现是否还会触发", "not in request"],
			"target_terms": ["调度链路", "trace"],
			"rationale": "current request asks to connect trace with current source"
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("profile should be soft-normalized, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active() {
		t.Fatalf("current source explanation profile not persisted: rm=%+v", rm)
	}
	if got := rm.CurrentSourceExplanationProfile.Modes; len(got) != 2 ||
		got[0] != types.CurrentSourceExplanationTraceCurrentFlow ||
		got[1] != types.CurrentSourceExplanationOther {
		t.Fatalf("modes=%v", got)
	}
	if !strings.Contains(res.Summary, "current_source_explanation=2") {
		t.Fatalf("summary should report current source explanation modes, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "current_source_explanation_profile ignored unanchored source_quote") {
		t.Fatalf("summary should warn for unanchored optional source quote, got %q", res.Summary)
	}
}

func TestEmitAnalysis_RouteBackedHistoryExplanationPreservesMixedLane(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("latest integration feature with current implementation impact")
	ctx := &types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "repo",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired,
		},
	}
	payload := json.RawMessage(`{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["git","merge","implementation","impact"],
		"entities":[],
		"question_kind":"history",
		"intent_confidence":0.9,
		"complexity_confidence":0.9,
		"kind_confidence":0.9,
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":false,
			"is_history_lookup":true,
			"is_diagnostic_question":false,
			"has_per_member_table":false
		},
		"diagnostic_profile":{
			"is_diagnostic":false,
			"current_risk":false,
			"historical_regression":false,
			"current_version_check":false,
			"confidence":0.9
		},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.9},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.9},
		"runtime_artifact_scope_profile":{"requested_scope":"not_applicable","confidence":0.9},
		"runtime_target_profile":{"declaration":"not_applicable","confidence":0.9},
		"runtime_question_profile":{"scope":"not_applicable","confidence":0.9},
		"history_selection_profile":{"mode":"latest_one","item_kind":"merge","source_quote":"latest integration","confidence":0.9},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`)
	res, err := (&EmitAnalysis{}).Execute(ctx, payload)
	if err != nil || !res.Success {
		t.Fatalf("Execute err=%v result=%+v", err, res)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.CurrentSourceObligationSignals) != 1 ||
		rm.CurrentSourceObligationSignals[0].Kind != types.CurrentSourceObligationSignalRouteBackedHistoryExplanation {
		t.Fatalf("route-backed history obligation not minted: %+v", rm)
	}
	if !types.IsHistoryBackedCurrentCodeExplanation(*rm) {
		t.Fatal("production emit path must recover mixed history/current-code authority")
	}
	if !strings.Contains(res.Summary, "current_source_obligation_signals=1") {
		t.Fatalf("typed signal count missing from result summary: %q", res.Summary)
	}
}

func TestEmitAnalysis_CurrentSourceExplanationProfileMissingFieldsSoftDrops(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只总结日志里发生了什么")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "summary"],
		"entities": ["log"],
		"question_kind": "mechanism",
		"current_source_explanation_profile": {
			"confidence": 0.8,
			"source_quotes": ["current source"]
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing optional profile field should not reject analysis, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.CurrentSourceExplanationProfile != nil {
		t.Fatalf("invalid optional profile must be dropped, got %+v", rm.CurrentSourceExplanationProfile)
	}
	if !strings.Contains(res.Summary, "current_source_explanation_profile ignored") {
		t.Fatalf("summary should disclose soft drop, got %q", res.Summary)
	}
}

func TestValidateSelfConsistency_DefineAxisSingleTargetRequiresSubjectDisambiguation(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:        false,
		IsRoleLocateLookup:    false,
		IsCountQuestion:       false,
		IsCrossComponent:      false,
		IsRelationalLookup:    false,
		IsCategoryEnumeration: false,
		IsHistoryLookup:       false,
		IsDiagnosticQuestion:  false,
	}
	reason := validateSelfConsistency(
		types.IntentExplain,
		types.ScenarioArchitectureExplain,
		"mechanism",
		preds,
		types.DiagnosticIntentProfile{},
		types.AxisDefine,
		[]string{"AnalysisIR"},
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "single-target define-axis lookup is under-specified") {
		t.Fatalf("expected define-axis disambiguation reject, got %q", reason)
	}
}

func TestValidateSelfConsistency_DefineAxisSingleTargetAcceptsExplicitRoleLocate(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:     true,
		IsRoleLocateLookup: true,
	}
	reason := validateSelfConsistency(
		types.IntentExplain,
		types.ScenarioArchitectureExplain,
		"mechanism",
		preds,
		types.DiagnosticIntentProfile{},
		types.AxisDefine,
		[]string{"AnalysisIR"},
		nil,
		types.AnswerSubject{Kind: types.SubjectFunctionName},
	)
	if reason != "" {
		t.Fatalf("explicit role-locate classification should pass, got %q", reason)
	}
}

func TestNormalizeMissingAnswerSubjectForNonScalarExplain(t *testing.T) {
	preds := types.SemanticPredicates{
		IsScalarAnswer:     false,
		IsRoleLocateLookup: false,
	}
	got, warning := normalizeMissingAnswerSubjectForNonScalarExplain(
		types.AxisDefine,
		types.IntentExplain,
		preds,
		[]string{"codrax"},
		nil,
		types.AnswerSubject{},
	)
	if warning == "" {
		t.Fatal("expected normalization warning")
	}
	if got.Kind != types.SubjectGeneric {
		t.Fatalf("answer_subject.kind = %q, want generic", got.Kind)
	}

	kept, warning := normalizeMissingAnswerSubjectForNonScalarExplain(
		types.AxisDefine,
		types.IntentReturnValue,
		preds,
		[]string{"codrax"},
		nil,
		types.AnswerSubject{},
	)
	if warning != "" || kept.Kind != types.SubjectUnknown {
		t.Fatalf("scalar-prone return_value classification must still require explicit subject, got %+v warning=%q", kept, warning)
	}
}

func TestValidateSelfConsistency_DiagnosticPredicateAlignsIntentAndScenario(t *testing.T) {
	base := types.SemanticPredicates{IsDiagnosticQuestion: true}
	reason := validateSelfConsistency(
		types.IntentExplain,
		types.ScenarioArchitectureExplain,
		"mechanism",
		base,
		types.DiagnosticIntentProfile{IsDiagnostic: true},
		types.AxisUnknown,
		nil,
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "intent=root_cause") ||
		!strings.Contains(reason, "set predicates.is_diagnostic_question") ||
		!strings.Contains(reason, "without system replacement") {
		t.Fatalf("expected diagnostic intent alignment reject, got %q", reason)
	}

	reason = validateSelfConsistency(
		types.IntentRootCause,
		types.ScenarioArchitectureExplain,
		"mechanism",
		base,
		types.DiagnosticIntentProfile{IsDiagnostic: true},
		types.AxisUnknown,
		nil,
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "scenario=root_cause") ||
		!strings.Contains(reason, "clear the diagnostic predicate/profile flags") {
		t.Fatalf("expected diagnostic scenario alignment reject, got %q", reason)
	}

	reason = validateSelfConsistency(
		types.IntentRootCause,
		types.ScenarioRootCause,
		"mechanism",
		base,
		types.DiagnosticIntentProfile{IsDiagnostic: true},
		types.AxisUnknown,
		nil,
		nil,
		types.AnswerSubject{},
	)
	if reason != "" {
		t.Fatalf("aligned diagnostic classification should pass, got %q", reason)
	}
}

func TestEmitAnalysis_RejectsContradictoryDiagnosticExplainRoute(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["panic", "runtime", "frame"],
		"entities": ["RuntimeError"],
		"question_kind": "mechanism",
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false, "has_per_member_table": false`, `"is_diagnostic_question": true, "has_per_member_table": false`, 1)

	res, mu := runEmitAnalysisPayload(t, "这是什么错误？", payload)
	if res.Success {
		t.Fatalf("Execute must reject contradictory diagnostic route instead of replacing model intent: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "is_diagnostic_question=true conflicts with intent=explain") ||
		!strings.Contains(res.Summary, "attached-log conclusion-boundary explanation") {
		t.Fatalf("unexpected rejection: %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Fatalf("rejected contradictory analysis must not persist a rewritten RequestModel: %+v", rm)
	}
}

func TestValidateSelfConsistency_AcceptsAlignedPerformanceDiagnosticRoute(t *testing.T) {
	reason := validateSelfConsistency(
		types.IntentRootCause,
		types.ScenarioPerformanceBottleneck,
		"mechanism",
		types.SemanticPredicates{IsDiagnosticQuestion: true},
		types.DiagnosticIntentProfile{IsDiagnostic: true},
		types.AxisUnknown,
		nil,
		nil,
		types.AnswerSubject{},
	)
	if reason != "" {
		t.Fatalf("aligned performance diagnostic route should pass, got %q", reason)
	}
}

func TestValidateSelfConsistency_CurrentVersionCheckRequiresDiagnosticCompanion(t *testing.T) {
	reason := validateSelfConsistency(
		types.IntentConfigQuery,
		types.ScenarioGeneric,
		"config_mapping",
		types.SemanticPredicates{IsDiagnosticQuestion: false},
		types.DiagnosticIntentProfile{
			CurrentVersionCheck: true,
			Confidence:          0.9,
		},
		types.AxisConfigure,
		[]string{"feature_x_timeout"},
		nil,
		types.AnswerSubject{Kind: types.SubjectConfigKey},
	)
	if reason == "" || !strings.Contains(reason, "current_version_check=true is only valid") {
		t.Fatalf("expected isolated current_version_check reject, got %q", reason)
	}

	reason = validateSelfConsistency(
		types.IntentRootCause,
		types.ScenarioRootCause,
		"mechanism",
		types.SemanticPredicates{IsDiagnosticQuestion: false},
		types.DiagnosticIntentProfile{
			CurrentRisk:         true,
			CurrentVersionCheck: true,
			ObservationSummary:  "previous run rewrote the wrong topic",
			Confidence:          0.9,
		},
		types.AxisUnknown,
		[]string{"Finalizer"},
		nil,
		types.AnswerSubject{},
	)
	if reason != "" {
		t.Fatalf("diagnostic current-status profile should pass, got %q", reason)
	}
}

func TestValidateSelfConsistency_RootCauseRequiresDiagnosticPredicate(t *testing.T) {
	reason := validateSelfConsistency(
		types.IntentRootCause,
		types.ScenarioRootCause,
		"mechanism",
		types.SemanticPredicates{},
		types.DiagnosticIntentProfile{},
		types.AxisUnknown,
		nil,
		nil,
		types.AnswerSubject{},
	)
	if reason == "" || !strings.Contains(reason, "is_diagnostic_question=true") {
		t.Fatalf("expected root_cause/predicate alignment reject, got %q", reason)
	}
}

func TestEmitAnalysis_WriteModeToleratesAdvisoryRootCauseClassifierDrift(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["main.cpp", "return", "typo"],
		"entities": ["main.cpp", "greet"],
		"question_kind": "mechanism"
	}`)

	res, mu := runEmitAnalysisPayloadWithMode(t, "fix the typo in main.cpp", payload, types.ModePlan)
	if !res.Success {
		t.Fatalf("write-mode advisory read analyzer drift should not reject, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "write-mode tolerated read-analyzer root_cause") {
		t.Fatalf("summary should disclose the tolerance path, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.Intent != types.IntentRootCause {
		t.Fatalf("intent should preserve the LLM-emitted read classifier for observability, got %q", rm.Intent)
	}

	readRes, _ := runEmitAnalysisPayloadWithMode(t, "fix the typo in main.cpp", payload, types.ModeRead)
	if readRes.Success {
		t.Fatalf("read mode must keep rejecting root_cause without diagnostic typed signal, got %q", readRes.Summary)
	}
	if !strings.Contains(readRes.Summary, "intent=root_cause requires a diagnostic typed signal") {
		t.Fatalf("read-mode rejection reason drifted: %q", readRes.Summary)
	}
}

func TestNormalizeDiagnosticMirrorSignals_PredicateWinsWithoutStrongSignal(t *testing.T) {
	preds := types.SemanticPredicates{IsDiagnosticQuestion: false}
	diagnostic := types.DiagnosticIntentProfile{
		IsDiagnostic:        true,
		CurrentVersionCheck: true,
		Confidence:          0.9,
	}

	gotPreds, gotDiagnostic, warnings := normalizeDiagnosticMirrorSignals(preds, diagnostic)

	if gotPreds.IsDiagnosticQuestion {
		t.Fatalf("predicate should stay false without strong diagnostic signals: %+v", gotPreds)
	}
	if gotDiagnostic.IsDiagnostic {
		t.Fatalf("diagnostic mirror should align false: %+v", gotDiagnostic)
	}
	if gotDiagnostic.CurrentVersionCheck {
		t.Fatalf("isolated current_version_check should be softened false: %+v", gotDiagnostic)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected mirror softening warnings, got %v", warnings)
	}
}

func TestNormalizeDiagnosticMirrorSignals_StrongSignalPromotesPredicate(t *testing.T) {
	preds := types.SemanticPredicates{IsDiagnosticQuestion: false}
	diagnostic := types.DiagnosticIntentProfile{
		CurrentRisk: true,
		Confidence:  0.9,
	}

	gotPreds, gotDiagnostic, warnings := normalizeDiagnosticMirrorSignals(preds, diagnostic)

	if !gotPreds.IsDiagnosticQuestion {
		t.Fatalf("current_risk should promote diagnostic predicate: %+v", gotPreds)
	}
	if !gotDiagnostic.IsDiagnostic {
		t.Fatalf("strong diagnostic signal should align profile mirror true: %+v", gotDiagnostic)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected promotion warnings, got %v", warnings)
	}
}

func TestNormalizeDiagnosticMirrorSignals_PredicatePromotesMirror(t *testing.T) {
	preds := types.SemanticPredicates{IsDiagnosticQuestion: true}
	diagnostic := types.DiagnosticIntentProfile{Confidence: 0.9}

	gotPreds, gotDiagnostic, warnings := normalizeDiagnosticMirrorSignals(preds, diagnostic)

	if !gotPreds.IsDiagnosticQuestion {
		t.Fatalf("predicate should remain true: %+v", gotPreds)
	}
	if !gotDiagnostic.IsDiagnostic {
		t.Fatalf("predicate should align diagnostic profile mirror true: %+v", gotDiagnostic)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one mirror warning, got %v", warnings)
	}
}

func TestNormalizeDiagnosticProfileForExternalObservationPolicyClearsCurrentStatus(t *testing.T) {
	diagnostic := types.DiagnosticIntentProfile{
		IsDiagnostic:         true,
		CurrentRisk:          true,
		HistoricalRegression: true,
		CurrentVersionCheck:  true,
		Confidence:           0.92,
	}
	policy := &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"只分析 trace，不分析代码"},
		Confidence:        0.95,
	}

	got, warnings := normalizeDiagnosticProfileForExternalObservationPolicy(diagnostic, policy)

	if !got.IsDiagnostic {
		t.Fatalf("trace-only diagnostic should remain diagnostic: %+v", got)
	}
	if got.CurrentRisk || got.CurrentVersionCheck || got.HistoricalRegression {
		t.Fatalf("current-status fields should be cleared by source-exclude policy: %+v", got)
	}
	if len(warnings) != 3 {
		t.Fatalf("expected three repair warnings, got %v", warnings)
	}
}

func TestParseErrorGranularityProfile_SoftensUnanchoredQuotes(t *testing.T) {
	raw := "全面排查finalyzer阶段的各种重试是否真的必要，哪些其实是可以靠系统进行修复的"
	profile, err, warnings := parseErrorGranularityProfile(raw, &emitErrorGranularityProfileParam{
		IsGranularityQuestion:   testBoolPtr(true),
		RequestedVerdictOptions: []string{string(types.ErrorGranularityPerItemRejection)},
		SourceQuotes:            []string{"按是否可系统修复原则评估"},
		Confidence:              testFloatPtr(0.8),
	})

	if err != "" {
		t.Fatalf("unanchored optional quotes should not reject, got %q", err)
	}
	if profile != nil {
		t.Fatalf("unanchored quote should soften the optional profile to nil, got %+v", profile)
	}
	if len(warnings) == 0 {
		t.Fatal("expected softening warning")
	}
}

func TestParseErrorGranularityProfile_NormalizedQuoteMatch(t *testing.T) {
	raw := "哪些其实是可以靠系统进行修复的？"
	quote := "哪些 其实 是 可以 靠 系统 进行 修复 的"
	profile, err, warnings := parseErrorGranularityProfile(raw, &emitErrorGranularityProfileParam{
		IsGranularityQuestion:   testBoolPtr(true),
		RequestedVerdictOptions: []string{string(types.ErrorGranularityPerItemRejection)},
		SourceQuotes:            []string{quote},
		Confidence:              testFloatPtr(0.8),
	})

	if err != "" {
		t.Fatalf("normalized quote should not reject, got %q", err)
	}
	if profile == nil || len(profile.SourceQuotes) != 1 || profile.SourceQuotes[0] != quote {
		t.Fatalf("normalized anchored quote should be preserved, got %+v", profile)
	}
	if len(warnings) != 0 {
		t.Fatalf("normalized match should not warn, got %v", warnings)
	}
}

func TestParseErrorGranularityProfile_DoesNotAcceptShortSubstring(t *testing.T) {
	raw := "finalizer 重试"
	profile, err, warnings := parseErrorGranularityProfile(raw, &emitErrorGranularityProfileParam{
		IsGranularityQuestion:   testBoolPtr(true),
		RequestedVerdictOptions: []string{string(types.ErrorGranularityPerItemRejection)},
		SourceQuotes:            []string{"finalizer 重试要保留还是移除"},
		Confidence:              testFloatPtr(0.8),
	})

	if err != "" {
		t.Fatalf("unanchored optional quote should not reject, got %q", err)
	}
	if profile != nil {
		t.Fatalf("short-substring overlap must not keep the profile active, got %+v", profile)
	}
	if len(warnings) == 0 {
		t.Fatal("expected softening warning")
	}
}

func TestValidateAnalysisInput_WarnBelowKeywords(t *testing.T) {
	limits := AnalysisLimits{WarnBelowKeywords: 8, RejectBelowKeywords: 0}
	res := validateAnalysisInput([]string{"a", "b", "c"}, nil, limits, "", 0)

	if res.RejectReason != "" {
		t.Errorf("soft floor must not reject, got %q", res.RejectReason)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "got=3") || !strings.Contains(res.Warnings[0], "want≥8") {
		t.Errorf("warning missing count details, got %q", res.Warnings[0])
	}
	if !strings.Contains(res.Warnings[0], "recommended") {
		t.Errorf("warning should say 'recommended floor', got %q", res.Warnings[0])
	}
}

func TestValidateAnalysisInput_DeprecatedRejectBelowKeywordsWarnsOnly(t *testing.T) {
	limits := AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 5}
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits, "", 0)

	if res.RejectReason != "" {
		t.Fatalf("deprecated keyword reject floor must not reject, got %q", res.RejectReason)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected advisory warning, got %+v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "got=2") || !strings.Contains(res.Warnings[0], "want≥5") {
		t.Errorf("warning missing count details, got %q", res.Warnings[0])
	}
	if !strings.Contains(res.Warnings[0], "advisory") {
		t.Errorf("warning should say advisory, got %q", res.Warnings[0])
	}
}

func TestValidateAnalysisInput_DeprecatedRejectMergesWithWarn(t *testing.T) {
	limits := AnalysisLimits{WarnBelowKeywords: 8, RejectBelowKeywords: 6}
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits, "", 0)

	if res.RejectReason != "" {
		t.Fatalf("keyword floors are advisory only, got reject %q", res.RejectReason)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected one merged advisory warning, got %+v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "want≥8") {
		t.Fatalf("higher soft floor should remain the visible advisory floor, got %q", res.Warnings[0])
	}
}

func TestValidateAnalysisInput_DropsGenericEntities(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: []string{"agent", "handler"},
	}
	ents := []string{"OrchestratorAgent", "Agent", "Handler", "StageAnalyze", "HANDLER"}
	// Empty seenBlob → whitelist is inactive → historical strict
	// dropping behavior (Agent / Handler removed unconditionally).
	res := validateAnalysisInput(nil, ents, limits, "", 0)

	if res.RejectReason != "" {
		t.Errorf("filter-only run must not reject, got %q", res.RejectReason)
	}

	// Surviving entities must keep their original casing.
	wantKept := []string{"OrchestratorAgent", "StageAnalyze"}
	if len(res.FilteredEntities) != len(wantKept) {
		t.Fatalf("FilteredEntities = %v, want %v", res.FilteredEntities, wantKept)
	}
	for i, w := range wantKept {
		if res.FilteredEntities[i] != w {
			t.Errorf("FilteredEntities[%d] = %q, want %q", i, res.FilteredEntities[i], w)
		}
	}

	// Dropped entries carry their original casing and the warning
	// lists them sorted lexicographically for determinism.
	if len(res.DroppedEntities) != 3 {
		t.Errorf("DroppedEntities count = %d, want 3 (Agent, Handler, HANDLER)", len(res.DroppedEntities))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "dropped_generic_entities") {
		t.Errorf("warning text missing label, got %q", res.Warnings[0])
	}
	if strings.Contains(res.Warnings[0], "blocklist_shadow") {
		t.Fatalf("shadow telemetry must stay log-only, got user-facing warning %q", res.Warnings[0])
	}
	if len(res.BlocklistShadow) != 3 {
		t.Fatalf("BlocklistShadow count = %d, want 3; got %v", len(res.BlocklistShadow), res.BlocklistShadow)
	}
	for _, item := range res.BlocklistShadow {
		if item.Surface == "" {
			t.Fatalf("BlocklistShadow contains empty surface: %v", res.BlocklistShadow)
		}
		if item.Resolution != "inferred_concept" {
			t.Fatalf("BlocklistShadow resolution = %q, want inferred_concept", item.Resolution)
		}
		if item.UseForSearch || item.UseForShape {
			t.Fatalf("dropped generic shadow must not be search/shape eligible: %+v", item)
		}
		if item.Reason != "blocked_generic_entity_not_prescan_verified" {
			t.Fatalf("unexpected BlocklistShadow reason: %+v", item)
		}
	}
	if !strings.Contains(res.BlocklistShadowSummary, "blocklist_shadow: dropped=3") ||
		!strings.Contains(res.BlocklistShadowSummary, "would_search=0") ||
		!strings.Contains(res.BlocklistShadowSummary, "would_shape=0") ||
		!strings.Contains(res.BlocklistShadowSummary, "inferred_concept=3") {
		t.Fatalf("BlocklistShadowSummary missing counters: %q", res.BlocklistShadowSummary)
	}
}

func TestValidateAnalysisInput_EmptyBlocklistSkipsFilter(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: nil,
	}
	ents := []string{"agent", "handler", "Explorer"}
	res := validateAnalysisInput(nil, ents, limits, "", 0)

	if len(res.DroppedEntities) != 0 {
		t.Errorf("nil blocklist must drop nothing, got %v", res.DroppedEntities)
	}
	if len(res.FilteredEntities) != 3 {
		t.Errorf("nil blocklist must pass all entities through, got %v", res.FilteredEntities)
	}
	if len(res.BlocklistShadow) != 0 || res.BlocklistShadowSummary != "" {
		t.Errorf("nil blocklist must not emit shadow telemetry, got shadow=%v summary=%q",
			res.BlocklistShadow, res.BlocklistShadowSummary)
	}
}

// TestValidateAnalysisInput_WhitelistKeepsVerifiedGenericEntity pins
// the 2026-04-15 fix: when a generic-blocklist entity ALSO appears
// in the typed pre-scan corpus as a standalone code/prose token, it
// must be kept instead of dropped so real symbols named `Agent` or
// `Handler` survive. A distinct `kept_generic_verified_entities`
// warning fires so the operator can audit the rescue.
func TestValidateAnalysisInput_WhitelistKeepsVerifiedGenericEntity(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: []string{"agent", "handler", "count"},
	}
	// seenBlob is what `AnalyzerEvaluator.Observe` would have appended:
	// lowercased concatenation of successful pre-scan tool Summaries.
	// Here we simulate code/prose snippets that contain both `agent`
	// and `handler` as standalone tokens, but NOT the word `count`.
	seenBlob := "type agent struct {}\nfunc handler() {}\n"
	ents := []string{"Orchestrator", "Agent", "Handler", "Count"}

	res := validateAnalysisInput(nil, ents, limits, seenBlob, 1)

	// Count is still dropped (not in the seenBlob).
	if len(res.DroppedEntities) != 1 || res.DroppedEntities[0] != "Count" {
		t.Errorf("DroppedEntities = %v, want [Count]", res.DroppedEntities)
	}
	// Agent and Handler should be rescued by the whitelist.
	if len(res.KeptVerifiedEntities) != 2 {
		t.Fatalf("KeptVerifiedEntities count = %d, want 2; got %v",
			len(res.KeptVerifiedEntities), res.KeptVerifiedEntities)
	}
	// Both rescued entries should appear in the final FilteredEntities
	// so downstream consumers see the full list in one place.
	names := map[string]bool{}
	for _, e := range res.FilteredEntities {
		names[e] = true
	}
	for _, want := range []string{"Orchestrator", "Agent", "Handler"} {
		if !names[want] {
			t.Errorf("FilteredEntities missing %q, got %v", want, res.FilteredEntities)
		}
	}
	if names["Count"] {
		t.Errorf("Count should have been dropped, but is in FilteredEntities")
	}

	// Two distinct warnings: one for dropped (Count), one for
	// kept-verified (Agent, Handler).
	var haveDropped, haveKept bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "dropped_generic_entities") {
			haveDropped = true
		}
		if strings.Contains(w, "kept_generic_verified_entities") {
			haveKept = true
		}
	}
	if !haveDropped {
		t.Errorf("Warnings missing dropped_generic_entities line, got %v", res.Warnings)
	}
	if !haveKept {
		t.Errorf("Warnings missing kept_generic_verified_entities line, got %v", res.Warnings)
	}
	if len(res.BlocklistShadow) != 1 || res.BlocklistShadow[0].Surface != "Count" {
		t.Errorf("shadow telemetry should cover only dropped Count, got %v", res.BlocklistShadow)
	}
}

func TestValidateAnalysisInput_WhitelistRejectsPathOnlyGenericEntity(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: []string{"agent", "handler"},
	}
	seenBlob := "internal/agent/analyzer.go\ninternal/tool/handler.go\n"
	ents := []string{"Orchestrator", "Agent", "Handler"}

	res := validateAnalysisInput(nil, ents, limits, seenBlob, 1)

	if len(res.KeptVerifiedEntities) != 0 {
		t.Fatalf("path-only hits must not be whitelisted, got kept=%v", res.KeptVerifiedEntities)
	}
	if len(res.DroppedEntities) != 2 {
		t.Fatalf("DroppedEntities = %v, want Agent and Handler", res.DroppedEntities)
	}
	names := map[string]bool{}
	for _, e := range res.FilteredEntities {
		names[e] = true
	}
	if !names["Orchestrator"] {
		t.Fatalf("non-generic entity should survive, got %v", res.FilteredEntities)
	}
	if names["Agent"] || names["Handler"] {
		t.Fatalf("path-only generic entities should be dropped, got %v", res.FilteredEntities)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "kept_generic_verified_entities") {
			t.Fatalf("path-only hits should not emit kept warning, got %v", res.Warnings)
		}
	}
	if len(res.BlocklistShadow) != 2 {
		t.Fatalf("path-only generic entities should be shadowed as dropped, got %v", res.BlocklistShadow)
	}
}

// TestValidateAnalysisInput_WhitelistInactiveWithEmptyBlob pins the
// backwards-compat invariant: when the seenBlob is empty (tests,
// fallback paths, analyzer ran with no pre-scans), the whitelist
// exception is INACTIVE and the historical strict drop behavior
// applies byte-for-byte. This is the regression guard against
// accidentally changing the no-pre-scan default.
func TestValidateAnalysisInput_WhitelistInactiveWithEmptyBlob(t *testing.T) {
	limits := AnalysisLimits{
		GenericEntityBlocklist: []string{"agent", "handler"},
	}
	ents := []string{"Agent", "Handler", "Explorer"}

	res := validateAnalysisInput(nil, ents, limits, "" /* empty blob */, 0)

	if len(res.KeptVerifiedEntities) != 0 {
		t.Errorf("empty blob must not rescue anything, got %v", res.KeptVerifiedEntities)
	}
	if len(res.DroppedEntities) != 2 {
		t.Errorf("empty blob + strict blocklist should drop 2, got %v", res.DroppedEntities)
	}
	if len(res.FilteredEntities) != 1 || res.FilteredEntities[0] != "Explorer" {
		t.Errorf("FilteredEntities = %v, want [Explorer]", res.FilteredEntities)
	}
}

// TestValidateAnalysisInput_KeywordHitRatioWarning fires the
// keyword_hit_ratio soft floor and asserts the warning shape.
func TestValidateAnalysisInput_KeywordHitRatioWarning(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywordHitRatio: 0.75,
	}
	// 1 of 4 keywords appears in the seenBlob → ratio 0.25 → below
	// 0.75 floor → warning fires.
	seenBlob := "internal/orchestrator/orchestrator.go\n"
	keywords := []string{"orchestrator", "pipeline", "stage", "dispatch"}

	res := validateAnalysisInput(keywords, nil, limits, seenBlob, 1)

	if res.RejectReason != "" {
		t.Errorf("soft floor must not reject, got %q", res.RejectReason)
	}
	if res.Probe.KeywordHits != 1 || res.Probe.KeywordTotal != 4 {
		t.Errorf("Probe = %+v, want Hits=1 Total=4", res.Probe)
	}
	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "keyword_hit_ratio") && strings.Contains(w, "below floor") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings missing keyword_hit_ratio warning, got %v", res.Warnings)
	}
}

// TestValidateAnalysisInput_HitRatioWarningsDisabledWhenFloorZero
// pins the disabled-by-default behavior: when both floors are 0,
// the probe is still computed and surfaced via res.Probe but no
// warnings fire regardless of hit ratio.
func TestValidateAnalysisInput_HitRatioWarningsDisabledWhenFloorZero(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywordHitRatio: 0,
		WarnBelowEntityHitRatio:  0,
	}
	// Pathological hit ratios: 0 of 5 keywords, 0 of 3 entities.
	seenBlob := "internal/unrelated.go\n"
	keywords := []string{"foo", "bar", "baz", "qux", "quux"}
	entities := []string{"Missing1", "Missing2", "Missing3"}

	res := validateAnalysisInput(keywords, entities, limits, seenBlob, 1)

	if res.Probe.KeywordHits != 0 || res.Probe.EntityHits != 0 {
		t.Errorf("Probe should show zero hits, got %+v", res.Probe)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "hit_ratio") {
			t.Errorf("threshold=0 must disable hit-ratio warnings, got %q", w)
		}
	}
}

// TestMutableState_PrescanSummaryBlob exercises the new accessor
// trio (Append / Read / Reset). The lowercase-at-write invariant
// is critical for the validator's whitelist + probe checks, so
// the test asserts it explicitly.
func TestMutableState_PrescanSummaryBlob(t *testing.T) {
	mu := types.NewMutableState("trace the pipeline")

	// Initial state: empty blob.
	if got := mu.PrescanSummaryBlob(); got != "" {
		t.Errorf("fresh Mutable should have empty blob, got %q", got)
	}

	// Append two summaries; they should be lowercased and
	// newline-separated.
	mu.AppendPrescanSummary("Internal/Agent/Analyzer.go matched")
	mu.AppendPrescanSummary("File: internal/tool/emit_analysis.go")

	blob := mu.PrescanSummaryBlob()
	if blob == "" {
		t.Fatal("blob should be non-empty after appends")
	}
	if !strings.Contains(blob, "internal/agent/analyzer.go") {
		t.Errorf("append should lowercase at write; blob=%q", blob)
	}
	if strings.Contains(blob, "Internal/Agent/Analyzer.go") {
		t.Errorf("blob must NOT contain the original-case summary; blob=%q", blob)
	}
	// Two appends → two newlines.
	if count := strings.Count(blob, "\n"); count != 2 {
		t.Errorf("newline count = %d, want 2", count)
	}

	// Reset wipes the blob.
	mu.ResetPrescanSummary()
	if got := mu.PrescanSummaryBlob(); got != "" {
		t.Errorf("reset should clear blob, got %q", got)
	}
	// Post-reset append starts fresh.
	mu.AppendPrescanSummary("Fresh")
	if got := mu.PrescanSummaryBlob(); got != "fresh\n" {
		t.Errorf("post-reset append = %q, want \"fresh\\n\"", got)
	}

	// Nil receiver is a no-op (defensive path from the signature
	// comment). Not testable through the normal constructor, but
	// we exercise the nil-check branch by shadowing.
	var nilMu *types.MutableState
	nilMu.AppendPrescanSummary("ignored")
	if got := nilMu.PrescanSummaryBlob(); got != "" {
		t.Errorf("nil Mutable should return empty blob, got %q", got)
	}
	nilMu.ResetPrescanSummary() // must not panic
}

// TestEmitAnalysis_Execute_ReadsPrescanBlobFromMutable is the
// end-to-end tool contract: when the analyzer has appended pre-scan
// summaries to Mutable, emit_analysis must thread them into
// validateAnalysisInput so the whitelist and hit-ratio probe
// activate. Exercises the whole pipeline from MutableState to
// ToolResult.Summary.
func TestEmitAnalysis_Execute_ReadsPrescanBlobFromMutable(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: []string{"agent", "handler"},
	})

	mu := types.NewMutableState("explore the agents")
	// Simulate the analyzer's Observe having recorded a pre-scan
	// snippet where `agent` appears as a standalone symbol/prose token.
	// Path-only hits such as internal/agent/... are intentionally not
	// enough to rescue a generic entity anymore.
	mu.AppendPrescanSummary("type Agent struct {}\ninternal/agent/explorer.go")

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["explore"],
		"entities": ["Agent", "Handler", "Orchestrator"],
		"question_kind": "mechanism",
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}

	// `Agent` should be rescued by the whitelist (it appears in the
	// seen-blob); `Handler` should be dropped (not in the blob).
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	names := map[string]bool{}
	for _, e := range rm.AnalyzerHints.Entities {
		names[e] = true
	}
	if !names["Agent"] {
		t.Errorf("Agent should have been rescued by whitelist, got %v", rm.AnalyzerHints.Entities)
	}
	if !names["Orchestrator"] {
		t.Errorf("Orchestrator should be kept (non-blocklist), got %v", rm.AnalyzerHints.Entities)
	}
	if names["Handler"] {
		t.Errorf("Handler should be dropped (not in seenBlob), got %v", rm.AnalyzerHints.Entities)
	}
	// Summary should mention both the kept-verified warning.
	if !strings.Contains(res.Summary, "kept_generic_verified_entities") {
		t.Errorf("Summary should surface kept_generic_verified_entities, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ConfigTraceRecordsExactNoMatchAsUnverified(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	missingKey := "zz_absent_config_" + "knob"
	mu := types.NewMutableState("how is " + missingKey + " resolved?")
	mu.AppendPrescanSummary("[grep params: pattern=" + missingKey + " case_insensitive=true files_only=true]\nno matches found")
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["zz", "absent", "config", "knob"],
		"entities": ["` + missingKey + `"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"}
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	finds := mu.EvidenceClosure().UnverifiedFindings()
	if len(finds) != 1 {
		t.Fatalf("unverified findings = %d, want 1: %+v", len(finds), finds)
	}
	if finds[0].Token != missingKey || finds[0].Kind != "symbol" || finds[0].Reason != "exact target has no current production-defining prescan hit" {
		t.Fatalf("unexpected unverified finding: %+v", finds[0])
	}
}

func TestEmitAnalysis_ConfigTraceNoMatchRequiresExactPatternToken(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	key := "short_config_" + "knob"
	mu := types.NewMutableState("how is " + key + " resolved?")
	mu.AppendPrescanSummary("[grep params: pattern=short_config_knob_extra case_insensitive=true files_only=true]\nno matches found")
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["short", "config", "knob"],
		"entities": ["` + key + `"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"}
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if finds := mu.EvidenceClosure().UnverifiedFindings(); len(finds) != 0 {
		t.Fatalf("substring pattern should not mark exact key absent: %+v", finds)
	}
}

func TestEmitAnalysis_ConfigTraceRecordsAuxiliaryOnlyExactMatchesAsUnverified(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	key := "explore_mid_loop_hint_budget"
	mu := types.NewMutableState("how is " + key + " resolved?")
	mu.AppendPrescanSummary(
		"[grep params: pattern=" + key + " case_insensitive=true files_only=true]\n" +
			"internal/skill/analysis_contract.go\n" +
			"internal/agent/explorer_test.go\n",
	)
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["explore", "mid", "loop", "hint", "budget"],
		"entities": ["` + key + `"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"},
		"exact_targets": ["` + key + `"]
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	finds := mu.EvidenceClosure().UnverifiedFindings()
	if len(finds) != 1 {
		t.Fatalf("unverified findings = %d, want 1: %+v", len(finds), finds)
	}
	if finds[0].Token != key || finds[0].Reason != "exact target matched only auxiliary prescan files" {
		t.Fatalf("unexpected auxiliary-only finding: %+v", finds[0])
	}
}

func TestEmitAnalysis_ConfigTraceProductionTextHitDoesNotMarkTargetUnverified(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	key := "explore_mid_loop_hint_budget"
	mu := types.NewMutableState("how is " + key + " resolved?")
	mu.AppendPrescanSummary(
		"[grep params: pattern=" + key + " case_insensitive=true files_only=true]\n" +
			"configs/runtime.json:14\n",
	)
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["explore", "mid", "loop", "hint", "budget"],
		"entities": ["` + key + `"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"},
		"exact_targets": ["` + key + `"]
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if finds := mu.EvidenceClosure().UnverifiedFindings(); len(finds) != 0 {
		t.Fatalf("production config text hit should clear unverified target, got %+v", finds)
	}
}

func TestEmitAnalysis_ConfigMappingPreservesContextRolesBeforeScenarioInference(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	key := "pipeline_max_retries_per_stage"
	mu := types.NewMutableState("比较 " + key + " 的默认配置和覆盖层")
	payload := `{
		"intent": "config_query",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["pipeline", "retries", "config"],
		"entities": ["` + key + `"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"},
		"exact_targets": ["` + key + `"],
		"exact_context_roles": ["default", "config", "override"]
	}`

	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	want := []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleDefault,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleOverride,
	}
	if !reflect.DeepEqual(rm.AnalyzerHints.ExactContextRoles, want) {
		t.Fatalf("ExactContextRoles = %v, want %v", rm.AnalyzerHints.ExactContextRoles, want)
	}
}

func TestEmitAnalysis_Execute_PersistsDiagramHint(t *testing.T) {
	mu := types.NewMutableState("trace Dispatch to Handler path")
	payload := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["dispatch", "handler"],
		"entities": ["Dispatch", "Handler"],
		"question_kind": "call_chain",
		"exact_targets": ["Dispatch", "Handler"],
		"predicate_axis": "call",
		"call_chain_endpoints": {"source":"Dispatch", "sink":"Handler"},
		"diagram_hint": {"kind": "call_dag", "required": false, "relation_scope_quote":"", "participants": [
			{"identity":"Dispatch", "role":"incident_required", "source_quote":"Dispatch"},
			{"identity":"Handler", "role":"context_only", "source_quote":"Handler"}
		]}
	}`

	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.DiagramHint == nil {
		t.Fatal("DiagramHint not persisted")
	}
	if rm.DiagramHint.Kind != types.DiagramCallDAG {
		t.Fatalf("DiagramHint.Kind = %q, want %q", rm.DiagramHint.Kind, types.DiagramCallDAG)
	}
	if len(rm.DiagramHint.Participants) != 2 ||
		rm.DiagramHint.Participants[0] != (types.DiagramParticipantHint{Identity: "Dispatch", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Dispatch"}) ||
		rm.DiagramHint.Participants[1] != (types.DiagramParticipantHint{Identity: "Handler", Role: types.DiagramParticipantContextOnly, SourceQuote: "Handler"}) {
		t.Fatalf("DiagramHint.Participants = %+v, want typed roles preserved", rm.DiagramHint.Participants)
	}
	if !strings.Contains(res.Summary, "diagram_hint=call_dag") {
		t.Fatalf("summary missing diagram hint echo: %q", res.Summary)
	}
}

func TestEmitAnalysis_DiagramParticipantSchemaUsesPlanningSSOT(t *testing.T) {
	raw, err := json.Marshal((&EmitAnalysis{}).Parameters())
	if err != nil {
		t.Fatalf("marshal Parameters: %v", err)
	}
	if count := strings.Count(string(raw), skill.AnalysisDiagramParticipantPlanningContract); count != 2 {
		t.Fatalf("diagram participant SSOT must describe diagram_hint and participants exactly once each, got %d in %s", count, raw)
	}
	if !strings.Contains(string(raw), `"required":["identity","role","source_quote"]`) {
		t.Fatalf("diagram participant schema must require exact current-request provenance: %s", raw)
	}
	if !strings.Contains(string(raw), `"required":["kind","required","relation_scope_quote","participants"]`) {
		t.Fatalf("diagram schema must require relation-surface provenance: %s", raw)
	}
}

func TestParseDiagramHintRejectsAmbiguousParticipantContract(t *testing.T) {
	required := true
	unknownRole := []emitDiagramParticipantParam{{Identity: "StageA", Role: "required", SourceQuote: "StageA"}}
	duplicateIdentity := []emitDiagramParticipantParam{
		{Identity: "ArkRunner", Role: "incident_required", SourceQuote: "ArkRunner"},
		{Identity: " arkrunner ", Role: "context_only", SourceQuote: "ArkRunner"},
	}
	tests := []struct {
		name string
		hint *emitDiagramHintParam
		want string
	}{
		{
			name: "unknown role",
			hint: &emitDiagramHintParam{Kind: "flow", Required: &required, RelationScopeQuote: "show StageA and ArkRunner", Participants: &unknownRole},
			want: "is not recognised",
		},
		{
			name: "duplicate identity",
			hint: &emitDiagramHintParam{Kind: "flow", Required: &required, RelationScopeQuote: "show StageA and ArkRunner", Participants: &duplicateIdentity},
			want: "duplicates",
		},
		{
			name: "missing participant provenance",
			hint: &emitDiagramHintParam{Kind: "flow", Required: &required, RelationScopeQuote: "show StageA and ArkRunner", Participants: &[]emitDiagramParticipantParam{{Identity: "StageA", Role: "incident_required"}}},
			want: "source_quote is empty",
		},
		{
			name: "quote does not own identity",
			hint: &emitDiagramHintParam{Kind: "flow", Required: &required, RelationScopeQuote: "show StageA and ArkRunner", Participants: &[]emitDiagramParticipantParam{{Identity: "StageA", Role: "incident_required", SourceQuote: "show"}}},
			want: "does not contain identity",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, reason, _ := parseDiagramHint("show StageA and ArkRunner", tc.hint, true); got != nil || !strings.Contains(reason, tc.want) {
				t.Fatalf("parseDiagramHint()=(%+v,%q), want nil reason containing %q", got, reason, tc.want)
			}
		})
	}
}

func TestParseDiagramHintDropsInvalidOptionalProvenanceAfterHardRequirementDemotion(t *testing.T) {
	required := true
	participants := []emitDiagramParticipantParam{
		{Identity: "app-20", Role: "incident_required", SourceQuote: "app-20 (20)"},
		{Identity: "CPU 001", Role: "context_only", SourceQuote: "同 CPU 竞争"},
	}
	raw := "app-20 和 rival-30 频繁短片段切换；下一步应看同 CPU 竞争"

	got, reason, warnings := parseDiagramHint(raw, &emitDiagramHintParam{
		Kind:               "sequence",
		Required:           &required,
		RelationScopeQuote: "app-20 和 rival-30 频繁短片段切换",
		Participants:       &participants,
	}, false)
	if reason != "" || got == nil {
		t.Fatalf("invalid optional participant guidance should be dropped without retry: got=%+v reason=%q warnings=%v", got, reason, warnings)
	}
	if got.Required || len(got.Participants) != 0 {
		t.Fatalf("demoted optional diagram retained hard/invalid participants: %+v", got)
	}
	joined := strings.Join(warnings, " | ")
	for _, want := range []string{
		"normalized diagram_hint.required from true to false",
		`dropped optional diagram participant "app-20"`,
		`dropped optional diagram participant "CPU 001"`,
		"cannot create a retry",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("optional provenance repair warning missing %q: %s", want, joined)
		}
	}
}

func TestParseDiagramHintClearsUnanchoredOptionalRelationScopeWithoutRetry(t *testing.T) {
	required := false
	participants := []emitDiagramParticipantParam{}
	got, reason, warnings := parseDiagramHint("explain the scheduler", &emitDiagramHintParam{
		Kind:               "flow",
		Required:           &required,
		RelationScopeQuote: "invented pipeline scope",
		Participants:       &participants,
	}, false)
	if reason != "" || got == nil || got.Required || got.RelationScopeQuote != "" {
		t.Fatalf("unanchored optional scope should clear without retry: got=%+v reason=%q warnings=%v", got, reason, warnings)
	}
	if joined := strings.Join(warnings, " | "); !strings.Contains(joined, "cleared optional diagram_hint.relation_scope_quote") {
		t.Fatalf("optional scope repair must remain auditable: %s", joined)
	}
}

func TestEmitAnalysis_Execute_RejectsDiagramHintWithoutRequiredAuthority(t *testing.T) {
	mu := types.NewMutableState("draw the current pipeline")
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["pipeline", "diagram"],
		"entities": ["pipeline"],
		"question_kind": "mechanism",
		"diagram_hint": {"kind": "flow"}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "diagram_hint.required is missing") {
		t.Fatalf("missing authority bit must fail loudly instead of silently making the diagram optional: %+v", res)
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Fatalf("rejected analysis must not persist a request model: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_FreeFormPresentationDoesNotAuthorizeHardDiagram(t *testing.T) {
	mu := types.NewMutableState("解释 run 到 match 的调用关系")
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["run","match","call"],
		"entities":["run","match"],
		"question_kind":"mechanism",
		"predicate_axis":"call",
		"diagram_hint":{"kind":"call_dag","required":true,"relation_scope_quote":"run 到 match 的调用关系","participants":[
			{"identity":"run","role":"incident_required","source_quote":"run"},
			{"identity":"match","role":"incident_required","source_quote":"match"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable:               mu,
		PresentationDirective: "markdown table",
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("optional diagram guidance should survive: %+v", res)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || hint.Required {
		t.Fatalf("free-form non-diagram directive minted hard visual authority: %+v", hint)
	}
	if !strings.Contains(res.Summary, "normalized diagram_hint.required from true to false") {
		t.Fatalf("normalization must be disclosed: %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RequiresExplicitDiagramParticipantSlate(t *testing.T) {
	base := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["pipeline", "diagram"],
		"entities": ["pipeline"],
		"question_kind": "mechanism",
		"diagram_hint": %s
	}`

	t.Run("missing relation scope fails loud", func(t *testing.T) {
		mu := types.NewMutableState("draw the current pipeline")
		payload := fmt.Sprintf(base, `{"kind":"flow","required":true,"participants":[]}`)
		res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "flow diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if res.Success || !strings.Contains(res.Summary, "diagram_hint.relation_scope_quote is empty") {
			t.Fatalf("missing relation-surface authority must fail loudly: %+v", res)
		}
		if mu.RequestModel() != nil {
			t.Fatal("rejected analysis persisted a request model")
		}
	})

	t.Run("missing participant slate fails loud", func(t *testing.T) {
		mu := types.NewMutableState("draw the current pipeline")
		payload := fmt.Sprintf(base, `{"kind":"flow","required":true,"relation_scope_quote":"draw the current pipeline"}`)
		res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "flow diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if res.Success || !strings.Contains(res.Summary, "diagram_hint.participants is missing") {
			t.Fatalf("missing participant slate must fail loudly: %+v", res)
		}
		if mu.RequestModel() != nil {
			t.Fatal("rejected analysis persisted a request model")
		}
	})

	t.Run("explicit empty participant slate is accepted", func(t *testing.T) {
		mu := types.NewMutableState("draw a generic pipeline")
		payload := fmt.Sprintf(base, `{"kind":"flow","required":true,"relation_scope_quote":"draw a generic pipeline","participants":[]}`)
		res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "flow diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !res.Success {
			t.Fatalf("explicit empty participant slate rejected: %s", res.Summary)
		}
		if hint := mu.RequestModel().DiagramHint; hint == nil || hint.Participants == nil || len(hint.Participants) != 0 {
			t.Fatalf("DiagramHint=%+v, want present empty participant slate", hint)
		}
	})
}

func TestEmitAnalysis_Execute_RejectsEmptyParticipantSlateWhenRequiredDiagramDimensionCoListsTypedEntities(t *testing.T) {
	raw := "请给出 codrax read-mode pipeline 的逻辑视图：用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流，然后简要说明各组件责任。"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","diagram"],
		"entities":["BusContext","Mutable","AgentAnalyzer","AgentExplorer","AgentExtractor","AgentFinalizer","Orchestrator"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"codrax read-mode pipeline 的逻辑视图","participants":[]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"Mermaid 架构图","role":"diagram","source_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","required":true},
			{"index":2,"label":"组件责任","role":"stage_or_workflow","source_quote":"简要说明各组件责任","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "participants is explicitly empty") ||
		!strings.Contains(res.Summary, "[BusContext Mutable]") ||
		!strings.Contains(res.Summary, "Do not leave the slate empty") {
		t.Fatalf("empty participant slate must fail with typed cross-field guidance: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected empty participant slate persisted a request model")
	}
}

func TestEmitAnalysis_Execute_RequiredDiagramDimensionCannotLoseDiagramHintOnRepair(t *testing.T) {
	raw := "请用 Mermaid 架构图画出 Analyzer、Explorer 和 Mutable 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["Analyzer","Explorer","Mutable","数据流"],
		"entities":["Analyzer","Explorer","Mutable"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"Mermaid 架构图","role":"diagram","source_quote":"Mermaid 架构图","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid 架构图", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "missing required top-level field(s): diagram_hint") ||
		!strings.Contains(res.Summary, "presentation authority and does not prove any edge") {
		t.Fatalf("required diagram carrier omission must fail with bounded repair guidance: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("analysis missing its required diagram carrier must not persist a RequestModel")
	}
}

func TestEmitAnalysis_Execute_RejectsEmptyParticipantSlateWhenRelationScopeCoListsTypedEntities(t *testing.T) {
	raw := "请给出 codrax read-mode pipeline 的逻辑视图：用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流，然后简要说明各组件责任。"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","diagram"],
		"entities":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"架构图","role":"diagram","source_quote":"用 Mermaid 架构图画出","required":true},
			{"index":2,"label":"组件责任","role":"function_or_purpose","source_quote":"简要说明各组件责任","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "participants is explicitly empty") ||
		!strings.Contains(res.Summary, "[analyzer explorer extractor finalizer Mutable BusContext]") ||
		!strings.Contains(res.Summary, "Do not leave the slate empty") {
		t.Fatalf("typed relation scope with an empty participant slate must fail loudly: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected empty participant slate persisted a request model")
	}
}

func TestValidateRequiredDiagramEmptyParticipantSlateDoesNotGuessSingleScopeOrEnterTrace(t *testing.T) {
	base := types.RequestModel{
		Intent:        types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"pipeline"}},
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Role: types.RequestedAnswerDimensionDiagram, Required: true, SourceQuote: "draw the pipeline diagram",
			}},
		},
	}
	if got := validateRequiredDiagramEmptyParticipantSlate(base, true); got != "" {
		t.Fatalf("one possible enclosing scope must not become a participant hard gate: %q", got)
	}
	trace := base
	trace.Intent = types.IntentTrace
	trace.AnalyzerHints.Entities = []string{"threadA", "threadB"}
	trace.RequestedAnswerDimensions.Dimensions[0].SourceQuote = "draw threadA and threadB"
	if got := validateRequiredDiagramEmptyParticipantSlate(trace, true); got != "" {
		t.Fatalf("Trace must stay on causal-projection contracts: %q", got)
	}
}

func TestEmitAnalysis_Execute_RejectsRequiredDiagramWhenInferredParticipantLacksCurrentRequestAuthority(t *testing.T) {
	mu := types.NewMutableState("explain analyze to finalizer; require a Mermaid sequenceDiagram; show BusContext")
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["analyze", "finalizer", "sequenceDiagram", "BusContext"],
		"entities": ["analyze", "finalizer", "BusContext"],
		"question_kind": "mechanism",
		"diagram_hint": {"kind":"sequence","required":true,"relation_scope_quote":"require a Mermaid sequenceDiagram; show BusContext","participants":[
			{"identity":"Orchestrator","role":"incident_required","source_quote":"Orchestrator"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || mu.RequestModel() != nil {
		t.Fatalf("required diagram must fail loud instead of silently dropping an unauthorized participant: success=%t model=%+v summary=%q", res.Success, mu.RequestModel(), res.Summary)
	}
	for _, want := range []string{"diagram_hint.participants[0]", "exact user-authored visible identity", "do not substitute a repository symbol"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("required participant repair missing %q: %s", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_Execute_RequiredDiagramPreservesRequestedDisplayIdentityAcrossSourceSymbolDiscovery(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","MutableState","BusContext"],
		"entities":["analyzer","MutableState","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"analyzer、Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"MutableState","role":"incident_required","source_quote":"MutableState"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"}
		]}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid architecture", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || mu.RequestModel() != nil {
		t.Fatalf("repository symbol substitution must not silently erase the requested display participant: success=%t model=%+v summary=%q", res.Success, mu.RequestModel(), res.Summary)
	}
	if !strings.Contains(res.Summary, "exact user-authored visible identity") {
		t.Fatalf("repair must direct the analyzer back to the requested visible identity: %s", res.Summary)
	}

	corrected := strings.ReplaceAll(payload, `"identity":"MutableState","role":"incident_required","source_quote":"MutableState"`, `"identity":"Mutable","role":"incident_required","source_quote":"Mutable"`)
	res, err = (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid architecture", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(corrected)))
	if err != nil {
		t.Fatalf("corrected Execute: %v", err)
	}
	if !res.Success || mu.RequestModel() == nil || mu.RequestModel().DiagramHint == nil {
		t.Fatalf("corrected user-visible participant should be accepted: success=%t summary=%q", res.Success, res.Summary)
	}
	want := []types.DiagramParticipantHint{
		{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "analyzer"},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Mutable"},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "BusContext"},
	}
	if got := mu.RequestModel().DiagramHint.Participants; !reflect.DeepEqual(got, want) {
		t.Fatalf("corrected participants=%+v, want exact requested display identities %+v", got, want)
	}
}

func TestEmitAnalysis_Execute_RequiredDiagramDimensionPreventsSiblingHintDowngrade(t *testing.T) {
	raw := "解释 read mode 从 analyze 到 finalizer 的时序：必须给 Mermaid sequenceDiagram，并给 stage 表"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["read mode","analyze","finalizer","sequenceDiagram","stage"],
		"entities":["analyze","finalizer"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"sequence","required":false,"relation_scope_quote":"read mode 从 analyze 到 finalizer 的时序","participants":[]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"sequenceDiagram","role":"diagram","source_quote":"Mermaid sequenceDiagram","required":true},
			{"index":2,"label":"stage","role":"stage_or_workflow","source_quote":"stage 表","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute rejected: %s", res.Summary)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || !hint.Required || hint.Kind != types.DiagramSequence {
		t.Fatalf("DiagramHint=%+v, want required sequence contract reconciled from the typed dimension", hint)
	}
	if len(hint.Participants) != 0 {
		t.Fatalf("reconciliation must not synthesize participants: %+v", hint.Participants)
	}
	if !strings.Contains(res.Summary, "normalized diagram_hint.required from false to true") {
		t.Fatalf("summary must disclose the structured reconciliation: %q", res.Summary)
	}
}

func TestReconcileDiagramHintRequiredWithRequestedDimensionsKeepsOptionalAndMissingShapes(t *testing.T) {
	optional := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Role: types.RequestedAnswerDimensionDiagram, Required: false,
		}},
	}
	hint := &types.DiagramHint{Kind: types.DiagramFlow, Required: false}
	if got, warning := reconcileDiagramHintRequiredWithRequestedDimensions(hint, optional); got != hint || warning != "" {
		t.Fatalf("optional dimension must remain optional: got=%+v warning=%q", got, warning)
	}
	required := &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Role: types.RequestedAnswerDimensionDiagram, Required: true,
		}},
	}
	if got, warning := reconcileDiagramHintRequiredWithRequestedDimensions(nil, required); got != nil || warning != "" {
		t.Fatalf("a dimension cannot synthesize a missing diagram kind: got=%+v warning=%q", got, warning)
	}
}

func TestEmitAnalysis_Execute_DropsContextOnlyParticipantOutsideTypedDiagramRelationScope(t *testing.T) {
	raw := "解释 read mode 从 analyze 到 finalizer 的时序，必须给 sequenceDiagram，并再给表格列出输入、输出和状态载体，例如 BusContext"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["read mode","analyze","finalizer","sequenceDiagram","BusContext"],
		"entities":["analyze","finalizer","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"sequence","required":true,"relation_scope_quote":"从 analyze 到 finalizer 的时序，必须给 sequenceDiagram","participants":[
			{"identity":"analyze","role":"incident_required","source_quote":"analyze"},
			{"identity":"finalizer","role":"incident_required","source_quote":"finalizer"},
			{"identity":"BusContext","role":"context_only","source_quote":"BusContext"}
		]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"时序","role":"stage_or_workflow","source_quote":"从 analyze 到 finalizer 的时序","required":true},
			{"index":2,"label":"输入","role":"other","source_quote":"输入","required":true},
			{"index":3,"label":"输出","role":"other","source_quote":"输出","required":true},
			{"index":4,"label":"状态载体","role":"other","source_quote":"状态载体","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute rejected: %s", res.Summary)
	}
	hint := mu.RequestModel().DiagramHint
	want := []types.DiagramParticipantHint{
		{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "analyze"},
		{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "finalizer"},
	}
	if hint == nil || !reflect.DeepEqual(hint.Participants, want) {
		t.Fatalf("participants=%+v, want only relation-surface-authorized rows %+v", hint, want)
	}
	if !strings.Contains(res.Summary, "outside diagram_hint.relation_scope_quote") {
		t.Fatalf("row-local cross-surface rejection must be disclosed: %q", res.Summary)
	}
	if got := mu.RequestModel().AnalyzerHints.Entities; !slices.Contains(got, "BusContext") {
		t.Fatalf("named state carrier must remain available in typed request carriers: %v", got)
	}
}

func TestEmitAnalysis_Execute_DropsIncidentParticipantsFromSiblingPresentationSurface(t *testing.T) {
	raw := "解释 codrax read mode 一次请求从 analyze 到 finalizer 的时序：必须给 Mermaid sequenceDiagram，并再给一张表列出每个 stage 的输入、输出和主要状态载体（例如 AnalysisIR、EvidenceItems、AnswerDocument、Mutable/BusContext）。"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["read mode","analyze","finalizer","sequenceDiagram","AnalysisIR","BusContext"],
		"entities":["analyze","finalizer","AnalysisIR","EvidenceItems","AnswerDocument","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"sequence","required":true,"relation_scope_quote":"codrax read mode 一次请求从 analyze 到 finalizer 的时序","participants":[
			{"identity":"analyze","role":"incident_required","source_quote":"从 analyze 到 finalizer 的时序"},
			{"identity":"finalizer","role":"incident_required","source_quote":"到 finalizer 的时序"},
			{"identity":"AnalysisIR","role":"incident_required","source_quote":"AnalysisIR、EvidenceItems、AnswerDocument"},
			{"identity":"EvidenceItems","role":"incident_required","source_quote":"EvidenceItems、AnswerDocument"},
			{"identity":"AnswerDocument","role":"incident_required","source_quote":"AnswerDocument"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"}
		]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"Mermaid sequenceDiagram","role":"diagram","source_quote":"Mermaid sequenceDiagram","required":true},
			{"index":2,"label":"输入、输出和主要状态载体","role":"stage_or_workflow","source_quote":"输入、输出和主要状态载体","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute rejected: %s", res.Summary)
	}
	hint := mu.RequestModel().DiagramHint
	want := []types.DiagramParticipantHint{
		{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "从 analyze 到 finalizer 的时序"},
		{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "到 finalizer 的时序"},
	}
	if hint == nil || !reflect.DeepEqual(hint.Participants, want) {
		t.Fatalf("participants=%+v, want closed relation surface %+v", hint, want)
	}
	joined := res.Summary
	for _, identity := range []string{"AnalysisIR", "EvidenceItems", "AnswerDocument", "BusContext"} {
		if !strings.Contains(joined, `dropped incident_required diagram participant "`+identity+`"`) {
			t.Fatalf("missing auditable sibling-surface drop for %q: %s", identity, joined)
		}
		if !slices.Contains(mu.RequestModel().AnalyzerHints.Entities, identity) {
			t.Fatalf("sibling table identity %q must remain available outside the diagram: %v", identity, mu.RequestModel().AnalyzerHints.Entities)
		}
	}
}

func TestEmitAnalysis_Execute_PreservesSplitClauseIncidentParticipants(t *testing.T) {
	raw := "对比 emit_answer_document 和 emit_answer_document_patch 的失败恢复能力、输入结构和适用时机，并用 Mermaid 小流程图说明它们在 finalizer 里的关系"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["emit_answer_document","emit_answer_document_patch","finalizer"],
		"entities":["emit_answer_document","emit_answer_document_patch","finalizer"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"flow","required":true,"relation_scope_quote":"并用 Mermaid 小流程图说明它们在 finalizer 里的关系","participants":[
			{"identity":"emit_answer_document","role":"incident_required","source_quote":"emit_answer_document"},
			{"identity":"emit_answer_document_patch","role":"incident_required","source_quote":"emit_answer_document_patch"},
			{"identity":"finalizer","role":"context_only","source_quote":"finalizer 里的关系"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid flow", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("split-clause relation contract rejected: %s", res.Summary)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || len(hint.Participants) != 3 {
		t.Fatalf("split-clause incident participants were lost: %+v", hint)
	}
	for _, want := range []string{"emit_answer_document", "emit_answer_document_patch"} {
		if !slices.ContainsFunc(hint.Participants, func(got types.DiagramParticipantHint) bool {
			return got.Identity == want && got.Role == types.DiagramParticipantIncidentRequired
		}) {
			t.Fatalf("missing independently grounded incident participant %q: %+v", want, hint.Participants)
		}
	}
	if !slices.ContainsFunc(hint.Participants, func(got types.DiagramParticipantHint) bool {
		return got.Identity == "finalizer" && got.Role == types.DiagramParticipantContextOnly
	}) {
		t.Fatalf("relation-local surrounding context was not preserved: %+v", hint.Participants)
	}
}

func TestEmitAnalysis_Execute_KeepsStateCarrierNamedInsideSequenceRelationSurface(t *testing.T) {
	raw := "必须给 analyze 通过 BusContext 到 finalizer 的 sequenceDiagram 时序"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyze","BusContext","finalizer","sequenceDiagram"],
		"entities":["analyze","BusContext","finalizer"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"sequence","required":true,"relation_scope_quote":"必须给 analyze 通过 BusContext 到 finalizer 的 sequenceDiagram 时序","participants":[
			{"identity":"analyze","role":"incident_required","source_quote":"analyze"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"},
			{"identity":"finalizer","role":"incident_required","source_quote":"finalizer"}
		]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"时序","role":"stage_or_workflow","source_quote":"analyze 通过 BusContext 到 finalizer 的 sequenceDiagram 时序","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute rejected: %s", res.Summary)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || len(hint.Participants) != 3 {
		t.Fatalf("an explicitly related carrier must survive diagram scoping: %+v", hint)
	}
}

func TestEmitAnalysis_Execute_PreservesExplicitFlowCarriersWithSiblingResponsibilityDimension(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流，然后简要说明各组件责任"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"complex",
		"keywords":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"entities":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"explorer","role":"incident_required","source_quote":"explorer"},
			{"identity":"extractor","role":"incident_required","source_quote":"extractor"},
			{"identity":"finalizer","role":"incident_required","source_quote":"finalizer"},
			{"identity":"Mutable","role":"incident_required","source_quote":"Mutable"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"}
		]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"组件","role":"stage_or_workflow","source_quote":"analyzer、explorer、extractor、finalizer","required":true},
			{"index":2,"label":"数据流","role":"stage_or_workflow","source_quote":"数据流","required":true},
			{"index":3,"label":"责任","role":"function_or_purpose","source_quote":"说明各组件责任","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil || !res.Success {
		t.Fatalf("explicit flow-carrier slate should pass, err=%v result=%+v", err, res)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || len(hint.Participants) != 6 {
		t.Fatalf("sibling answer dimensions must not erase explicit relation participants: %+v", hint)
	}
	for _, name := range []string{"Mutable", "BusContext"} {
		found := false
		for _, participant := range hint.Participants {
			if participant.Identity == name && participant.Role == types.DiagramParticipantIncidentRequired {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("explicit flow carrier %q was not preserved: %+v", name, hint.Participants)
		}
	}
}

func TestEmitAnalysis_Execute_RejectsCompositeAndBareContextOnlyRequiredFlowParticipants(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"entities":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"Mutable/BusContext","role":"context_only","source_quote":"Mutable/BusContext"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "collapses distinct typed entities [Mutable BusContext]") ||
		!strings.Contains(res.Summary, "source_quote is only the typed participant roster [Mutable BusContext]") {
		t.Fatalf("composite/bare context escape must fail with copy-ready typed guidance: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected participant slate must not publish a RequestModel")
	}
}

func TestEmitAnalysis_Execute_RejectsParticipantRosterAsContextOnlyProvenance(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"entities":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"Mutable","role":"context_only","source_quote":"Mutable/BusContext"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "typed participant roster [Mutable BusContext]") ||
		!strings.Contains(res.Summary, "does not prove surrounding context") {
		t.Fatalf("a delimiter-only participant roster must not demote one relation member to context-only: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected participant-role provenance must not publish a RequestModel")
	}
}

func TestEmitAnalysis_Execute_RejectsRelationClauseAsContextOnlyProvenance(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"entities":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"explorer","role":"incident_required","source_quote":"explorer"},
			{"identity":"extractor","role":"incident_required","source_quote":"extractor"},
			{"identity":"finalizer","role":"incident_required","source_quote":"finalizer"},
			{"identity":"Mutable","role":"context_only","source_quote":"Mutable/BusContext 之间的数据流"},
			{"identity":"BusContext","role":"context_only","source_quote":"Mutable/BusContext 之间的数据流"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "typed participant roster [Mutable BusContext]") ||
		!strings.Contains(res.Summary, "Do not change the semantic role merely to repair provenance") ||
		!strings.Contains(res.Summary, "only when the CURRENT request explicitly requires a relation") {
		t.Fatalf("a co-listed relation clause must not demote explicit relation members to context-only: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected participant-role provenance must not publish a RequestModel")
	}
}

func TestEmitAnalysis_Execute_AllowsWiderExplicitContextForCoListedParticipants(t *testing.T) {
	raw := "用 Mermaid 架构图画 analyzer 到 finalizer 的数据流，并把 Mutable/BusContext 仅作为外围背景"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","finalizer","Mutable","BusContext"],
		"entities":["analyzer","finalizer","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"analyzer 到 finalizer 的数据流，并把 Mutable/BusContext 仅作为外围背景","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer 到 finalizer 的数据流"},
			{"identity":"finalizer","role":"incident_required","source_quote":"analyzer 到 finalizer 的数据流"},
			{"identity":"Mutable","role":"context_only","source_quote":"把 Mutable/BusContext 仅作为外围背景"},
			{"identity":"BusContext","role":"context_only","source_quote":"把 Mutable/BusContext 仅作为外围背景"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("a wider explicit surrounding-context quote must remain a valid escape: %+v", res)
	}
}

func TestEmitAnalysis_Execute_RejectsOmittedEntityCoListedByParticipantSourceQuote(t *testing.T) {
	raw := "用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","explorer","extractor","finalizer","Mutable","BusContext"],
		"entities":["Analyzer","Explorer","Extractor","Finalizer","Mutable","BusContext","Orchestrator"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"用 Mermaid 架构图画出 analyzer、explorer、extractor、finalizer 以及 Mutable/BusContext 之间的数据流","participants":[
			{"identity":"Analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"Explorer","role":"incident_required","source_quote":"explorer"},
			{"identity":"Extractor","role":"incident_required","source_quote":"extractor"},
			{"identity":"Finalizer","role":"incident_required","source_quote":"finalizer"},
			{"identity":"BusContext","role":"incident_required","source_quote":"Mutable/BusContext"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, `source_quote for "BusContext" also names typed relation entity/entities [Mutable]`) ||
		!strings.Contains(res.Summary, "emit one row per co-listed actor") {
		t.Fatalf("one shared provenance quote must not authorize BusContext while silently omitting Mutable: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("an incomplete co-listed participant slate must not publish a RequestModel")
	}
}

func TestEmitAnalysis_RequiredFlowParticipantsTreatSnakeAndCamelAsSameWholeIdentity(t *testing.T) {
	raw := "解释 emit_answer_document 和 emit_answer_document_patch 在首次完整输出与重试补丁时如何选择，并画出两者的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["emit_answer_document","emit_answer_document_patch"],
		"entities":["EmitAnswerDocument","EmitAnswerDocumentPatch"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"emit_answer_document 和 emit_answer_document_patch 在首次完整输出与重试补丁时如何选择","participants":[
			{"identity":"emit_answer_document","role":"incident_required","source_quote":"emit_answer_document"},
			{"identity":"emit_answer_document_patch","role":"incident_required","source_quote":"emit_answer_document_patch"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid flowchart", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil || !res.Success || mu.RequestModel() == nil {
		t.Fatalf("whole-identity snake/camel aliases must not collide by prefix: err=%v result=%+v", err, res)
	}
	if got := mu.RequestModel().DiagramHint; got == nil || len(got.Participants) != 2 {
		t.Fatalf("both exact participants must survive: %+v", got)
	}
}

func TestDiagramParticipantIdentityAliasKeyPreservesQualifiedIdentitySemantics(t *testing.T) {
	if diagramParticipantIdentityAliasKey("EmitAnswerDocument") != diagramParticipantIdentityAliasKey("emit_answer_document") {
		t.Fatal("snake_case and CamelCase spellings of one flat identifier must share one alias key")
	}
	for _, pair := range [][2]string{{"A.B", "AB"}, {"A::B", "AB"}, {"A/B", "AB"}} {
		if diagramParticipantIdentityAliasKey(pair[0]) == diagramParticipantIdentityAliasKey(pair[1]) {
			t.Fatalf("qualified identity %q must not collapse into flat identity %q", pair[0], pair[1])
		}
	}
}

func TestEmitAnalysis_Execute_AllowsExplicitContextBoundaryProvenanceAndSeparateIncidentCarriers(t *testing.T) {
	raw := "画出 Analyzer 到 Finalizer 的数据流，并把 SurroundingSystem 仅作为周边边界；数据经过 Mutable 和 BusContext"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["Analyzer","Finalizer","SurroundingSystem","Mutable","BusContext"],
		"entities":["Analyzer","Finalizer","SurroundingSystem","Mutable","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"画出 Analyzer 到 Finalizer 的数据流，并把 SurroundingSystem 仅作为周边边界；数据经过 Mutable 和 BusContext","participants":[
			{"identity":"Analyzer","role":"incident_required","source_quote":"Analyzer"},
			{"identity":"Finalizer","role":"incident_required","source_quote":"Finalizer"},
			{"identity":"Mutable","role":"incident_required","source_quote":"Mutable"},
			{"identity":"BusContext","role":"incident_required","source_quote":"BusContext"},
			{"identity":"SurroundingSystem","role":"context_only","source_quote":"把 SurroundingSystem 仅作为周边边界"}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid architecture diagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil || !res.Success {
		t.Fatalf("explicit context-only escape and separate incident rows should pass, err=%v result=%+v", err, res)
	}
	hint := mu.RequestModel().DiagramHint
	if hint == nil || len(hint.Participants) != 5 || hint.Participants[4].Role != types.DiagramParticipantContextOnly {
		t.Fatalf("typed participant roles were not preserved: %+v", hint)
	}
}

func TestValidateRequiredFlowDiagramParticipantProvenanceDoesNotEnterTraceOrNonFlow(t *testing.T) {
	base := types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"Mutable", "BusContext"}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "Mutable/BusContext", Role: types.DiagramParticipantContextOnly, SourceQuote: "Mutable/BusContext",
		}}},
	}
	trace := base
	trace.Intent = types.IntentTrace
	if got := validateRequiredFlowDiagramParticipantProvenance(trace); got != "" {
		t.Fatalf("Trace must stay on its causal contracts: %q", got)
	}
	nonFlow := base
	nonFlow.PredicateAxis = types.AxisCall
	if got := validateRequiredFlowDiagramParticipantProvenance(nonFlow); got != "" {
		t.Fatalf("non-flow diagram must not inherit source-flow role provenance: %q", got)
	}
}

func TestEmitAnalysis_Execute_RejectsAllParticipantsOutsideRelationScope(t *testing.T) {
	raw := "解释从 analyze 到 finalizer 的时序，必须给 sequenceDiagram，并再给表格列出阶段、输入、输出和状态载体，例如 BusContext"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyze","finalizer","sequenceDiagram","BusContext"],
		"entities":["analyze","finalizer","BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"sequence","required":true,"relation_scope_quote":"从 analyze 到 finalizer 的时序，必须给 sequenceDiagram","participants":[
			{"identity":"BusContext","role":"context_only","source_quote":"BusContext"}
		]},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[
			{"index":1,"label":"阶段","role":"stage_or_workflow","source_quote":"阶段","required":true},
			{"index":2,"label":"输入","role":"other","source_quote":"输入","required":true},
			{"index":3,"label":"输出","role":"other","source_quote":"输出","required":true},
			{"index":4,"label":"状态载体","role":"other","source_quote":"状态载体","required":true}
		]}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, PresentationDirective: "Mermaid sequenceDiagram", PresentationDiagramRequired: true}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "contains none of the proposed context_only") {
		t.Fatalf("all-outside scope must force one analyzer correction instead of silently emptying the requested slate: %+v", res)
	}
	if mu.RequestModel() != nil {
		t.Fatal("rejected all-outside scope must not publish a RequestModel")
	}
}

// TestComputeAnalysisQualityProbe is a direct unit test of the
// probe computation helper: case-insensitive substring match,
// ratio handling, empty-input edge cases.
func TestComputeAnalysisQualityProbe(t *testing.T) {
	seenBlob := "internal/agent/analyzer.go\ninternal/orchestrator/topology.go\n"

	t.Run("counts substring hits case-insensitively", func(t *testing.T) {
		p := ComputeAnalysisQualityProbe(seenBlob,
			[]string{"Analyzer", "Orchestrator", "Missing"}, // 2 of 3 match
			[]string{"Topology", "Absent"},                  // 1 of 2 match
			2)
		if p.KeywordHits != 2 || p.KeywordTotal != 3 {
			t.Errorf("KeywordHits/Total = %d/%d, want 2/3", p.KeywordHits, p.KeywordTotal)
		}
		if p.EntityHits != 1 || p.EntityTotal != 2 {
			t.Errorf("EntityHits/Total = %d/%d, want 1/2", p.EntityHits, p.EntityTotal)
		}
		if p.PrescanRounds != 2 {
			t.Errorf("PrescanRounds = %d, want 2", p.PrescanRounds)
		}
	})

	t.Run("ratios are zero when total is zero", func(t *testing.T) {
		p := ComputeAnalysisQualityProbe(seenBlob, nil, nil, 0)
		if p.KeywordHitRatio() != 0 || p.EntityHitRatio() != 0 {
			t.Errorf("zero-total ratios should be 0, got kw=%v ent=%v",
				p.KeywordHitRatio(), p.EntityHitRatio())
		}
	})

	t.Run("empty blob produces zero probe", func(t *testing.T) {
		p := ComputeAnalysisQualityProbe("", []string{"anything"}, []string{"anything"}, 0)
		if p.KeywordHits != 0 || p.EntityHits != 0 {
			t.Errorf("empty blob should produce zero hits, got %+v", p)
		}
	})

	t.Run("ratio math matches hits/total", func(t *testing.T) {
		p := ComputeAnalysisQualityProbe(seenBlob,
			[]string{"Analyzer", "Topology", "Missing", "Absent"}, // 2 of 4
			nil, 1)
		if got := p.KeywordHitRatio(); got != 0.5 {
			t.Errorf("KeywordHitRatio = %v, want 0.5", got)
		}
	})
}

func TestDefaultAnalysisLimits_SensibleDefaults(t *testing.T) {
	limits := DefaultAnalysisLimits()
	if limits.WarnBelowKeywords != 8 {
		t.Errorf("WarnBelowKeywords = %d, want 8", limits.WarnBelowKeywords)
	}
	if limits.RejectBelowKeywords != 0 {
		t.Errorf("RejectBelowKeywords = %d, want 0 (soft-only by default)", limits.RejectBelowKeywords)
	}
	// Historical generic nouns must all be in the default blocklist.
	deny := make(map[string]bool, len(limits.GenericEntityBlocklist))
	for _, w := range limits.GenericEntityBlocklist {
		deny[strings.ToLower(w)] = true
	}
	for _, w := range []string{"count", "function", "thing", "agent", "handler", "module"} {
		if !deny[w] {
			t.Errorf("default blocklist missing historical generic noun %q", w)
		}
	}
}

func TestSetAnalysisLimits_RoundTrip(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })

	custom := AnalysisLimits{
		WarnBelowKeywords:         5,
		RejectBelowKeywords:       2,
		EmitOnlyCorrectionRetries: 4,
		GenericEntityBlocklist:    []string{"foo", "bar"},
	}
	SetAnalysisLimits(custom)

	got := CurrentAnalysisLimits()
	if got.WarnBelowKeywords != 5 || got.RejectBelowKeywords != 0 || got.EmitOnlyCorrectionRetries != 4 {
		t.Errorf("limits not installed: %+v", got)
	}
	if len(got.GenericEntityBlocklist) != 2 {
		t.Errorf("blocklist not installed: %v", got.GenericEntityBlocklist)
	}
	// CurrentAnalysisLimits returns a copy — mutating it must not
	// affect subsequent reads.
	got.GenericEntityBlocklist[0] = "tampered"
	again := CurrentAnalysisLimits()
	if again.GenericEntityBlocklist[0] != "foo" {
		t.Error("CurrentAnalysisLimits must return a defensive copy")
	}
}

// -----------------------------------------------------------------------------
// Execute + Summary tests (exercise the full tool contract)
// -----------------------------------------------------------------------------

func runEmitAnalysisWithObjective(t *testing.T, objective, payload string) (types.ToolResult, *types.MutableState) {
	t.Helper()
	mu := types.NewMutableState(objective)
	// Most legacy helper callers exercise emit_analysis semantics below the
	// turn-policy preflight. Install a typed presentation carrier so their
	// explicitly required diagram fixtures retain the production shape. Tests
	// for missing presentation authority call Execute directly.
	busCtx := &types.BusContext{Mutable: mu, PresentationDirective: "test fixture current-turn presentation authority", PresentationDiagramRequired: true}
	tool := &EmitAnalysis{}
	// Auto-inject the schema-v4 required fields (predicates +
	// per-classification confidences) so the v3-style payloads in
	// existing tests stay readable. Tests that exercise v4-specific
	// failure modes call Execute directly with a raw payload.
	res, err := tool.Execute(busCtx, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res, mu
}

func runEmitAnalysisPayload(t *testing.T, objective, payload string) (types.ToolResult, *types.MutableState) {
	t.Helper()
	mu := types.NewMutableState(objective)
	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu, PresentationDirective: "test fixture current-turn presentation authority", PresentationDiagramRequired: true}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res, mu
}

func runEmitAnalysisPayloadWithMode(t *testing.T, objective, payload string, mode types.PipelineMode) (types.ToolResult, *types.MutableState) {
	t.Helper()
	mu := types.NewMutableState(objective)
	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu, Mode: mode}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res, mu
}

func runEmitAnalysis(t *testing.T, payload string) (types.ToolResult, *types.MutableState) {
	t.Helper()
	return runEmitAnalysisWithObjective(t, "trace the pipeline through analyze", payload)
}

func TestEmitAnalysis_Execute_PersistsNormalizedRequestModel(t *testing.T) {
	// Use limits that don't warn on 3 keywords so we isolate the
	// happy-path persistence behavior.
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "root-cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["orchestrator", "pipeline", "analyze"],
		"entities": ["Orchestrator", "StageAnalyze"],
		"question_kind": "mechanism",
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false, "has_per_member_table": false`, `"is_diagnostic_question": true, "has_per_member_table": false`, 1)

	res, mu := runEmitAnalysisPayload(t, "trace the pipeline through analyze", payload)
	if !res.Success {
		t.Fatalf("Execute should succeed, got summary=%q", res.Summary)
	}

	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted on Mutable")
	}
	// Normalized: "root-cause" → IntentRootCause.
	if rm.Intent != types.IntentRootCause {
		t.Errorf("Intent = %q, want root_cause", rm.Intent)
	}
	if rm.Scenario != types.ScenarioRootCause {
		t.Errorf("Scenario = %q, want root_cause", rm.Scenario)
	}
	if len(rm.AnalyzerHints.Keywords) != 3 {
		t.Errorf("Keywords count = %d, want 3", len(rm.AnalyzerHints.Keywords))
	}
	if len(rm.AnalyzerHints.Entities) != 2 {
		t.Errorf("Entities count = %d, want 2", len(rm.AnalyzerHints.Entities))
	}
}

func TestEmitAnalysis_EquivalentTopLevelDimensionDuplicateIsLosslesslyRemoved(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"is_dimensioned_answer": "false",
		"intent": "root-cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["orchestrator", "pipeline", "analyze"],
		"entities": ["Orchestrator", "StageAnalyze"],
		"question_kind": "mechanism"
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false, "has_per_member_table": false`, `"is_diagnostic_question": true, "has_per_member_table": false`, 1)

	res, mu := runEmitAnalysisPayload(t, "trace the pipeline through analyze", payload)
	if !res.Success {
		t.Fatalf("equivalent legacy duplicate should not burn an analyzer retry: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("request model was not persisted after lossless duplicate repair")
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.IsDimensionedAnswer {
		t.Fatalf("canonical nested false decision changed to true: %+v", rm.RequestedAnswerDimensions)
	}
}

func TestEmitAnalysis_Execute_StringWrappedSubTopics(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["explorer", "diagram"],
		"entities": ["explorerEvaluator"],
		"question_kind": "mechanism",
		"sub_topics": "[{\"summary\":\"核心流程\",\"entities\":[\"explorerEvaluator\"]}]"
	}`)

	res, mu := runEmitAnalysisPayload(t, "explorer具体是怎么工作的？请用图表的方式展示", payload)
	if !res.Success {
		t.Fatalf("string-wrapped sub_topics should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.SubTopics) != 1 {
		t.Fatalf("sub_topics not persisted after repair: %+v", rm)
	}
	if rm.SubTopics[0].Summary != "核心流程" || len(rm.SubTopics[0].Entities) != 1 || rm.SubTopics[0].Entities[0] != "explorerEvaluator" {
		t.Fatalf("sub_topic decoded incorrectly: %+v", rm.SubTopics[0])
	}
	if !strings.Contains(res.Summary, `sub_topics=["核心流程"]`) {
		t.Fatalf("summary should report normalized sub-topic labels, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_StringWrappedRuntimeTargets(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "root_cause",
		"scenario": "performance_bottleneck",
		"complexity": "complex",
		"keywords": ["trace", "pid", "42591", "jank"],
		"entities": ["42591"],
		"question_kind": "root_cause",
		"runtime_target_profile": {"declaration":"named_target","source_quote":"42591进程","confidence":0.96},
		"runtime_targets": "[{\"kind\":\"process\",\"pid\":42591,\"source\":\"user_explicit\",\"confidence\":0.96}]"
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false, "has_per_member_table": false`, `"is_diagnostic_question": true, "has_per_member_table": false`, 1)

	res, mu := runEmitAnalysisPayload(t, "分析42591进程滑动卡顿的深层次根因，不要分析代码", payload)
	if !res.Success {
		t.Fatalf("string-wrapped runtime_targets should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.RuntimeTargets) != 1 {
		t.Fatalf("runtime_targets not persisted after repair: %+v", rm)
	}
	if got := rm.RuntimeTargets[0]; got.PID != 42591 || got.Kind != types.RuntimeTargetKindProcess {
		t.Fatalf("runtime target decoded incorrectly: %+v", got)
	}
}

func TestEmitAnalysis_Execute_StringWrappedComplexProfilesWithTransportCloseTag(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := withV4Required(`{
		"intent": "enumerate",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["EvalAll", "UnknownKind", "criterion", "公开函数"],
		"entities": ["EvalAll", "UnknownKind", "internal/analysis/criterion"],
		"question_kind": "enumeration",
		"language": "zh",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"sub_topics": "[{\"summary\":\"枚举公开函数\",\"entities\":[\"EvalAll\",\"UnknownKind\"]}]</parameter>",
		"source_inventory_profile": "{\"is_source_inventory\":true,\"target_roles\":[\"function\"],\"requested_fields\":[\"name\",\"location\",\"summary\"],\"source_quotes\":[\"只列公开函数\"],\"confidence\":1.0}</parameter>"
	}`)

	res, mu := runEmitAnalysisPayload(t, "internal/analysis/criterion 只列公开函数，每个函数给出文件:行和中文职责说明", payload)
	if !res.Success {
		t.Fatalf("transport-suffixed complex profiles should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist after repaired emit_analysis")
	}
	if len(rm.SubTopics) != 1 || rm.SubTopics[0].Summary != "枚举公开函数" {
		t.Fatalf("sub_topics not persisted after repair: %+v", rm.SubTopics)
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.IsSourceInventory {
		t.Fatalf("source_inventory_profile not persisted after repair: %+v", rm.SourceInventoryProfile)
	}
	if len(rm.SourceInventoryProfile.TargetRoles) != 1 ||
		rm.SourceInventoryProfile.TargetRoles[0] != types.AnswerCandidateRoleFunction {
		t.Fatalf("target_roles not decoded after repair: %+v", rm.SourceInventoryProfile.TargetRoles)
	}
	if !rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldSummary) {
		t.Fatalf("requested_fields lost summary after repair: %+v", rm.SourceInventoryProfile.RequestedFields)
	}
}

func TestEmitAnalysis_Execute_PersistsEnumerationBoundary(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["gate", "run", "checks"],
		"entities": ["gate.Run", "checkCoverage"],
		"question_kind": "mechanism",
		"enumeration_boundary": {
			"declared_count": 7,
			"source_quote": "7 checks"
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "What order do gate.Run's 7 checks execute in?", payload)
	if !res.Success {
		t.Fatalf("Execute should succeed, got summary=%q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.EnumerationBoundary == nil {
		t.Fatal("EnumerationBoundary not persisted on RequestModel")
	}
	if rm.EnumerationBoundary.DeclaredCount != 7 {
		t.Fatalf("DeclaredCount = %d, want 7", rm.EnumerationBoundary.DeclaredCount)
	}
	if rm.EnumerationBoundary.SourceQuote != "7 checks" {
		t.Fatalf("SourceQuote = %q, want %q", rm.EnumerationBoundary.SourceQuote, "7 checks")
	}
	if !strings.Contains(res.Summary, "boundary=7") {
		t.Fatalf("summary missing boundary count: %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_StripsEnumerationBoundaryDuplicatingExplicitRuntimeWindow(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	var payload map[string]any
	if err := json.Unmarshal([]byte(withV4Required(`{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["trace", "window", "app-100"],
		"entities": ["app-100"],
		"question_kind": "mechanism"
	}`)), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	payload["runtime_artifact_scope_profile"] = map[string]any{
		"requested_scope": "explicit_time_window",
		"time_start":      5.000,
		"time_end":        5.007,
		"source_quote":    "5.000s 到 5.007s",
		"confidence":      1.0,
	}
	payload["enumeration_boundary"] = map[string]any{
		"declared_count": 7,
		"source_quote":   "5.000s 到 5.007s",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	mu := types.NewMutableState("请分析 app-100 在 5.000s 到 5.007s 的 trace 状态")
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, AttachedHitrace: "inline trace"}, raw)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("time window conflict should be normalized without an analyzer retry: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.RuntimeArtifactScopeProfile == nil {
		t.Fatalf("runtime scope missing: %+v", rm)
	}
	if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); !ok {
		t.Fatalf("typed time window changed: %+v", rm.RuntimeArtifactScopeProfile)
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("time interval became a principal member count: %+v", rm.EnumerationBoundary)
	}
	if !strings.Contains(res.Summary, "already the typed explicit runtime time window") {
		t.Fatalf("normalization audit warning missing: %q", res.Summary)
	}
}

func TestEnumerationBoundarySeparateFromExplicitRuntimeWindowRemainsActive(t *testing.T) {
	start, end := 5.0, 5.007
	profile := &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "5.000s 到 5.007s",
	}
	boundary := &emitEnumerationBoundaryParam{DeclaredCount: 7, SourceQuote: "7 checks"}
	if enumerationBoundaryDuplicatesExplicitRuntimeWindow(boundary, profile) {
		t.Fatal("a separately quoted principal set inside a runtime window must remain active")
	}
}

func TestEmitAnalysis_Execute_ScalarCountStripsScopeEnumerationBoundary(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "simple",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.95,
		"kind_confidence": 0.95,
		"keywords": ["runTaskGraph", "git log", "commit"],
		"entities": ["runTaskGraph"],
		"question_kind": "history",
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": true,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"enumeration_boundary": {
			"declared_count": 20,
			"source_quote": "最近 20 次修改 internal/orchestrator/ 目录的 commit"
		}
	}`

	mu := types.NewMutableState("最近 20 次修改 internal/orchestrator/ 目录的 commit 中，有多少个直接涉及 runTaskGraph 函数？给出数字。")
	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got summary=%q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("scalar count scope window must not persist as a principal enumeration boundary: %+v", rm.EnumerationBoundary)
	}
	if strings.Contains(res.Summary, "boundary=20") {
		t.Fatalf("summary must not advertise a principal boundary for scalar count scope windows: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "scope windows") {
		t.Fatalf("summary should explain the soft strip, got %q", res.Summary)
	}
}

// TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryQuoteOutsideRequest
// pins the soft-strip behaviour: enumeration_boundary is schema-OPTIONAL
// and downstream consumers tolerate nil, so a quote that does not appear
// verbatim in the request is stripped + warned instead of rejected. The
// LLM still gets the corrective message in the tool summary, but the
// emit succeeds and the RequestModel persists without spending a heavy
// LLM retry round on an optional handoff. Mirrors the pattern used by
// ErrorGranularityCountsAreContextual (line ~912) and
// sanitizeExactContextTerms (line ~1960).
func TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryQuoteOutsideRequest(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["gate", "run", "checks"],
		"entities": ["gate.Run"],
		"question_kind": "mechanism",
		"enumeration_boundary": {
			"declared_count": 7,
			"source_quote": "9 checks"
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "What order do gate.Run's 7 checks execute in?", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored enumeration_boundary because source_quote") {
		t.Fatalf("summary missing strip-warn message: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "must appear verbatim in the request and contain the declared count") {
		t.Fatalf("summary missing strip-warn rationale: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of optional enumeration_boundary")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("EnumerationBoundary must be stripped to nil, got %+v", rm.EnumerationBoundary)
	}
}

func TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryEmptyQuote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["design", "architecture", "module"],
		"entities": ["ResourceSchedule", "resched"],
		"question_kind": "mechanism",
		"enumeration_boundary": {
			"declared_count": 1,
			"source_quote": ""
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "帮我解析当前目录下的代码，做一个详细的设计文档", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored enumeration_boundary because source_quote is empty") {
		t.Fatalf("summary missing empty-quote strip warning: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of optional enumeration_boundary")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("EnumerationBoundary must be stripped to nil, got %+v", rm.EnumerationBoundary)
	}
}

func TestEmitAnalysis_Execute_WarnAndStripsInactiveEnumerationBoundary(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["explorer", "subagent", "call"],
		"entities": ["explorer", "subagent"],
		"question_kind": "mechanism",
		"enumeration_boundary": {
			"declared_count": 0,
			"source_quote": ""
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "explorer 如何调用 subagent？", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored inactive enumeration_boundary") {
		t.Fatalf("summary missing inactive-boundary warning: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of inactive optional enumeration_boundary")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("EnumerationBoundary must be stripped to nil, got %+v", rm.EnumerationBoundary)
	}
}

func TestEmitAnalysis_Execute_WarnAndStripsUnknownEnumerationBoundaryWithQuote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["subagent", "registry", "default"],
		"entities": ["subagent"],
		"question_kind": "enumeration",
		"enumeration_boundary": {
			"declared_count": 0,
			"source_quote": "默认 subagent 有几个"
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "默认 subagent 有几个？列出来。", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored inactive enumeration_boundary with declared_count<=0") {
		t.Fatalf("summary missing inactive-boundary warning: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of inactive optional enumeration_boundary")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("EnumerationBoundary must be stripped to nil, got %+v", rm.EnumerationBoundary)
	}
	if got := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind); got != types.ReqEnumeration {
		t.Fatalf("stripping unknown count boundary must not erase the emitted question kind: got %q in %+v", got, rm)
	}
}

// TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryCountNotInQuote
// pins the strip+warn behaviour when the source_quote is present in the
// request but does not contain the declared count literal. Same rationale
// as TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryQuoteOutsideRequest:
// optional field, downstream tolerates nil, hard reject would waste a
// full analyzer round on a non-load-bearing field.
func TestEmitAnalysis_Execute_WarnAndStripsEnumerationBoundaryCountNotInQuote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["analysis", "packages", "entrypoints"],
		"entities": ["internal/analysis"],
		"question_kind": "enumeration",
		"enumeration_boundary": {
			"declared_count": 26,
			"source_quote": "所有子包"
		}
	}`

	res, mu := runEmitAnalysisWithObjective(t, "列出 internal/analysis 下所有子包及各自入口点", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored enumeration_boundary because source_quote") {
		t.Fatalf("summary missing strip-warn message: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of optional enumeration_boundary")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("EnumerationBoundary must be stripped to nil, got %+v", rm.EnumerationBoundary)
	}
}

func TestEmitAnalysis_Execute_WarnAndStripsUngroundedBuckets(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["codrax", "opencode", "comparison"],
		"entities": ["codrax", "opencode"],
		"question_kind": "mechanism",
		"buckets": [
			{"label": "codrax side", "anchors": ["codrax"]},
			{"label": "opencode side", "anchors": ["opencode"]}
		]
	}`

	res, mu := runEmitAnalysisWithObjective(t, "对比 codrax 和 opencode 的读模式防幻觉机制", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored buckets because no label survived current-request provenance validation") {
		t.Fatalf("summary missing bucket strip warning: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of optional buckets")
	}
	if len(rm.Buckets) != 0 {
		t.Fatalf("invalid buckets must be stripped to nil, got %+v", rm.Buckets)
	}
}

func TestEmitAnalysis_Execute_WarnAndStripsSingleBucket(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["explorer", "subagent"],
		"entities": ["explorer"],
		"question_kind": "mechanism",
		"buckets": [
			{"label": "explorer", "anchors": ["explorer"]}
		]
	}`

	res, mu := runEmitAnalysisWithObjective(t, "解释 explorer 如何调用 subagent", payload)
	if !res.Success {
		t.Fatalf("Execute should accept + warn, got reject summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored buckets because a comparison partition needs at least 2 current-request labels") {
		t.Fatalf("summary missing single-bucket strip warning: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel must persist after soft-strip of optional single bucket")
	}
	if len(rm.Buckets) != 0 {
		t.Fatalf("single bucket must be stripped to nil, got %+v", rm.Buckets)
	}
}

func TestEmitAnalysis_Execute_PreservesValidBuckets(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["codrax", "opencode", "comparison"],
		"entities": ["codrax", "opencode"],
		"question_kind": "mechanism",
		"buckets": [
			{"label": "codrax", "anchors": ["codrax"]},
			{"label": "opencode", "anchors": ["opencode"]}
		]
	}`

	res, mu := runEmitAnalysisWithObjective(t, "对比 codrax 和 opencode 的读模式防幻觉机制", payload)
	if !res.Success {
		t.Fatalf("Execute should succeed, got summary=%q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist")
	}
	if len(rm.Buckets) != 2 || rm.Buckets[0].Label != "codrax" || rm.Buckets[1].Label != "opencode" {
		t.Fatalf("valid buckets not preserved: %+v", rm.Buckets)
	}
}

func TestEmitAnalysis_Execute_ErrorGranularitySuppressesContextualEnumerationBoundary(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	objective := "When emit_evidence receives a batch where exactly one item is missing the anchor_kind field, does the whole call fail, or does the rest of the batch succeed and only the bad item get rejected?"
	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["emit_evidence", "anchor_kind", "batch"],
		"entities": ["emit_evidence", "anchor_kind"],
		"question_kind": "unknown",
		"intent_confidence": 0.85,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.85,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": 0.8
		},
		"error_granularity_profile": {
			"is_granularity_question": true,
			"confidence": 0.9,
			"requested_verdict_options": ["per_item_rejection", "whole_batch_failure"],
			"source_quotes": ["does the whole call fail, or does the rest of the batch succeed and only the bad item get rejected"]
		},
		"enumeration_boundary": {
			"declared_count": 1,
			"source_quote": "exactly one item is missing the anchor_kind field"
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`

	res, mu := runEmitAnalysisPayload(t, objective, payload)
	if !res.Success {
		t.Fatalf("Execute should suppress contextual enumeration boundary, got summary=%q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("contextual enumeration boundary should be suppressed, got %+v", rm.EnumerationBoundary)
	}
	if rm.ErrorGranularityProfile == nil || !rm.ErrorGranularityProfile.Active() {
		t.Fatal("error granularity profile should remain active")
	}
	if !strings.Contains(res.Summary, "ignored enumeration_boundary") {
		t.Fatalf("summary should report suppression warning, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Summary_ReportsNormalizedDelta(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	// F3-1 (2026-07-03): "root-cause" is a case/style enum alias and is
	// now repaired at the toolparam chokepoint BEFORE the tool's own
	// semantic normalizer (audited via the structured_payload_compat
	// repair record), so it no longer appears as a tool-level delta.
	// "register" -> "registration" is a SEMANTIC coercion the tool owns,
	// and stays in the "normalized:" clause.
	payload := withV4Required(`{
		"intent": "root-cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "register",
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false, "has_per_member_table": false`, `"is_diagnostic_question": true, "has_per_member_table": false`, 1)

	res, _ := runEmitAnalysisPayload(t, "trace the pipeline through analyze", payload)
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "intent=root_cause") {
		t.Errorf("Summary missing canonical intent, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "normalized:") {
		t.Errorf("Summary missing normalized clause, got %q", res.Summary)
	}
	if want := `question_kind "register"→"registration"`; !strings.Contains(res.Summary, want) {
		t.Errorf("Summary missing semantic delta %q, got %q", want, res.Summary)
	}
	if strings.Contains(res.Summary, `intent "root-cause"`) {
		t.Errorf("case-style alias must be repaired at the toolparam chokepoint, not reported as a tool-level delta: %q", res.Summary)
	}
}

func TestEmitAnalysis_Summary_CleanInputNoNormalizedClause(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a", "b", "c", "d", "e", "f", "g", "h"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
	}`
	res, _ := runEmitAnalysis(t, payload)
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "normalized:") {
		t.Errorf("clean input should not emit 'normalized:' clause, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "warn:") {
		t.Errorf("clean input should not emit 'warn:' clause, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_WarnPathStillPersists(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 8, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a", "b"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
	}`
	res, mu := runEmitAnalysis(t, payload)

	if !res.Success {
		t.Fatalf("warn path must still succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "warn:") {
		t.Errorf("warn path should surface a 'warn:' clause, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "recommended") {
		t.Errorf("warning text should mention recommended floor, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm == nil {
		t.Error("warn path must still persist the RequestModel")
	}
}

func TestEmitAnalysis_Execute_DeprecatedRejectPathPersistsWithWarning(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 5})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a", "b"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
	}`
	res, mu := runEmitAnalysis(t, payload)

	if !res.Success {
		t.Fatalf("deprecated reject path must fail-open, got summary=%q", res.Summary)
	}
	if strings.Contains(res.Summary, "rejected") {
		t.Errorf("deprecated keyword floor must not reject, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "warn:") || !strings.Contains(res.Summary, "recommended") {
		t.Errorf("deprecated reject path should persist with warning, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm == nil {
		t.Error("deprecated reject path must persist RequestModel")
	}
}

func TestEmitAnalysis_Execute_RejectsDegenerateClassification(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "unknown",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": [],
		"entities": [],
		"question_kind": "unknown",
	}`
	res, mu := runEmitAnalysis(t, payload)

	if res.Success {
		t.Fatalf("degenerate classification must fail, got summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "degenerate classification") {
		t.Errorf("reject Summary should name the degenerate classification, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "User Request section only") {
		t.Errorf("reject Summary should point the model back to the User Request, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Errorf("degenerate reject must not persist RequestModel, got %+v", rm)
	}
}

func TestEmitAnalysis_Execute_GenericEntitiesDropped(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	// Use the default blocklist but disable keyword floors so we
	// isolate the entity-filter signal.
	limits := DefaultAnalysisLimits()
	limits.WarnBelowKeywords = 0
	limits.RejectBelowKeywords = 0
	SetAnalysisLimits(limits)

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["foo"],
		"entities": ["Orchestrator", "agent", "handler"],
		"question_kind": "mechanism",
	}`

	res, mu := runEmitAnalysis(t, payload)
	if !res.Success {
		t.Fatalf("entity filter must not fail the call, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped_generic_entities") {
		t.Errorf("Summary should mention dropped entities, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if len(rm.AnalyzerHints.Entities) != 1 || rm.AnalyzerHints.Entities[0] != "Orchestrator" {
		t.Errorf("Entities should only retain Orchestrator, got %v", rm.AnalyzerHints.Entities)
	}
}

func TestEmitAnalysis_Execute_RejectsControlInput(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["build", "load", "graph"],
		"entities": ["buildOrLoadGraph"],
		"question_kind": "mechanism",
	}`
	objective := "## Prior conversation\nold topic\n\n## Current request\n\\q"
	res, mu := runEmitAnalysisWithObjective(t, objective, payload)

	if res.Success {
		t.Fatalf("control input must be rejected, got summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "control command") {
		t.Errorf("reject summary should mention control command, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "Prior Conversation") {
		t.Errorf("reject summary should mention Prior Conversation bleed, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Errorf("control-input reject must not persist RequestModel, got %+v", rm)
	}
}

func TestEmitAnalysis_Execute_InvalidJSONFails(t *testing.T) {
	mu := types.NewMutableState("anything")
	tool := &EmitAnalysis{}
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(`{not json`))
	if err == nil {
		t.Error("invalid JSON should return an error")
	}
	if res.Success {
		t.Errorf("invalid JSON must fail, got summary=%q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_MissingMutableFails(t *testing.T) {
	tool := &EmitAnalysis{}
	res, _ := tool.Execute(&types.BusContext{}, json.RawMessage(`{"intent":"explain"}`))
	if res.Success {
		t.Error("nil Mutable should fail")
	}
	if !strings.Contains(res.Summary, "writable context") {
		t.Errorf("failure Summary should mention missing writable context, got %q", res.Summary)
	}
}

// -----------------------------------------------------------------------------
// Schema v4 fail-loud tests — the LLM MUST emit predicates + confidences.
// A missing field rejects the call so retry exhausts cleanly rather than
// silently defaulting to "no predicates fired" (which would mask the
// classification miss the prose-cue tables used to catch).
// -----------------------------------------------------------------------------

func TestEmitAnalysis_Execute_RejectsMissingPredicatesObject(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	// Confidences present but the whole `predicates` object missing.
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"intent_confidence": 0.7,
		"complexity_confidence": 0.7,
		"kind_confidence": 0.7
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("missing predicates object must reject")
	}
	if !strings.Contains(res.Summary, "predicates object missing") {
		t.Errorf("reject summary should name the missing predicates object, got %q", res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Error("missing-predicates reject must not persist RequestModel")
	}
}

func TestEmitAnalysis_Execute_RejectsMissingPredicateField(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	// is_count_question intentionally omitted from the otherwise-complete
	// predicates object. Pointer fields catch this; a value type would
	// silently default to false here.
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"intent_confidence": 0.7,
		"complexity_confidence": 0.7,
		"kind_confidence": 0.7,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("missing predicate field must reject")
	}
	if !strings.Contains(res.Summary, "is_count_question") {
		t.Errorf("reject summary should name the missing field, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DefaultsMissingDiagnosticProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("diagnose whether the current failure is still present")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"intent_confidence": 0.7,
		"complexity_confidence": 0.7,
		"kind_confidence": 0.7,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true, "has_per_member_table": false
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("missing diagnostic_profile mirror should default instead of retrying, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "diagnostic_profile auto-defaulted") {
		t.Errorf("summary should disclose diagnostic defaulting, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !rm.Predicates.IsDiagnosticQuestion || !rm.DiagnosticProfile.IsDiagnostic {
		t.Fatalf("diagnostic predicate should mirror into defaulted profile, got preds=%+v profile=%+v",
			rm.Predicates, rm.DiagnosticProfile)
	}
	if rm.DiagnosticProfile.CurrentRisk || rm.DiagnosticProfile.HistoricalRegression || rm.DiagnosticProfile.CurrentVersionCheck {
		t.Fatalf("missing profile must not invent diagnostic sub-flags, got %+v", rm.DiagnosticProfile)
	}
	if rm.DiagnosticProfile.Confidence != 0.5 {
		t.Fatalf("default confidence = %.2f, want 0.5", rm.DiagnosticProfile.Confidence)
	}
}

func TestEmitAnalysis_Execute_RejectsMissingAnswerRoleProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"intent_confidence": 0.7,
		"complexity_confidence": 0.7,
		"kind_confidence": 0.7,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if res.Success {
		t.Fatal("missing answer_role_profile object must reject")
	}
	if !strings.Contains(res.Summary, "answer_role_profile object missing") {
		t.Errorf("reject summary should name the missing answer_role_profile object, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsMissingErrorGranularityProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"intent_confidence": 0.7,
		"complexity_confidence": 0.7,
		"kind_confidence": 0.7,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": 0.7
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if res.Success {
		t.Fatal("missing error_granularity_profile object must reject")
	}
	if !strings.Contains(res.Summary, "error_granularity_profile object missing") {
		t.Errorf("reject summary should name the missing error_granularity_profile object, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsConfidenceOutOfRange(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
	}`)
	// Tamper: replace one valid confidence with an out-of-range value.
	payload = strings.Replace(payload, `"intent_confidence": 0.7`, `"intent_confidence": 1.5`, 1)
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("out-of-range confidence must reject")
	}
	if !strings.Contains(res.Summary, "intent_confidence") || !strings.Contains(res.Summary, "1.50") {
		t.Errorf("reject summary should name the bad field + value, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsInvalidPredicateAxis(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explain the analyzer")
	tool := &EmitAnalysis{}
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "mechanism",
		"predicate_axis": "ponder"
	}`)
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("invalid axis must reject")
	}
	if !strings.Contains(res.Summary, "predicate_axis") {
		t.Errorf("reject summary should name predicate_axis, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_SchemaNormalizesLocalModelScalarArtifacts(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("读取代码，对比 codrax 和 opencode 的读模式防幻觉机制")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "\"explain\"",
		"scenario": "\"architecture_explain\"",
		"complexity": "\"complex\"",
		"keywords": ["codrax", "opencode", "hallucination"],
		"entities": ["codrax", "opencode"],
		"question_kind": "\"mechanism\"",
		"language": "\"zh\"",
		"predicate_axis": "\"\"",
		"intent_confidence": "0.95",
		"complexity_confidence": ": 0.90",
		"kind_confidence": "0.85",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": true,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": "0.7"
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": "0.7"
		},
		"error_granularity_profile": {
			"is_granularity_question": false,
			"confidence": "0.7"
		},
		"runtime_artifact_scope_profile": {
			"requested_scope": "\"not_applicable\"",
			"confidence": "0.7"
		},
		"enumeration_boundary": {
			"declared_count": "0",
			"source_quote": ""
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute should schema-normalize scalar artifacts before unmarshal: %v", err)
	}
	if !res.Success {
		t.Fatalf("schema-normalized emit_analysis should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist after schema-normalized emit_analysis")
	}
	if rm.Intent != types.IntentExplain || rm.Scenario != types.ScenarioArchitectureExplain ||
		rm.Complexity != types.ComplexityComplex || rm.AnalyzerHints.Kind != "mechanism" {
		t.Fatalf("classification enums not normalized: %+v", rm)
	}
	if rm.PredicateAxis != types.AxisUnknown {
		t.Fatalf("empty JSON-string predicate_axis should normalize to AxisUnknown, got %q", rm.PredicateAxis)
	}
	if rm.IntentConfidence != 0.95 || rm.ComplexityConfidence != 0.90 || rm.KindConfidence != 0.85 {
		t.Fatalf("numeric strings not normalized: intent=%.2f complexity=%.2f kind=%.2f",
			rm.IntentConfidence, rm.ComplexityConfidence, rm.KindConfidence)
	}
	if rm.EnumerationBoundary != nil {
		t.Fatalf("inactive enumeration_boundary should be soft-stripped, got %+v", rm.EnumerationBoundary)
	}
}

func TestEmitAnalysis_Execute_NormalizesEnumerateWithCountPredicate(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("list agents and include the count")
	tool := &EmitAnalysis{}
	// The user wants a list plus its count. Count is an attribute of the
	// enumeration; forcing the LLM to switch intent=return_value loses the
	// member-list obligation downstream.
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["agent"],
		"entities": ["Agent"],
		"question_kind": "enumeration",
		"intent_confidence": 0.7, "complexity_confidence": 0.7,
		"kind_confidence": 0.7, 
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("enumerate + count predicate should be normalized, not rejected: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm.Intent != types.IntentEnumerate {
		t.Fatalf("intent = %q, want enumerate", rm.Intent)
	}
	if rm.Predicates.IsCountQuestion || rm.Predicates.IsScalarAnswer {
		t.Fatalf("enumeration should clear count/scalar predicates, got %+v", rm.Predicates)
	}
	if !strings.Contains(res.Summary, "per-list attributes") {
		t.Fatalf("normalization warning should explain per-list counts, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsCountWithoutScalar(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("how many agents are there")
	tool := &EmitAnalysis{}
	// is_count_question=true must imply is_scalar_answer=true.
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["agent"],
		"entities": ["Agent"],
		"question_kind": "return_value",
		"intent_confidence": 0.7, "complexity_confidence": 0.7,
		"kind_confidence": 0.7, 
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("count without scalar must reject")
	}
	if !strings.Contains(res.Summary, "is_scalar_answer") {
		t.Errorf("reject summary should name is_scalar_answer, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsCategoryEnumerationWithScalar(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("what kinds of agents are there")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["agent", "kind"],
		"entities": ["Agent"],
		"question_kind": "enumeration",
		"intent_confidence": 0.7, "complexity_confidence": 0.7,
		"kind_confidence": 0.7, 
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("category enumeration + scalar must reject")
	}
	if !strings.Contains(res.Summary, "is_category_enumeration") {
		t.Errorf("reject summary should name is_category_enumeration, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsV4FieldsOntoRequestModel(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("count the agents")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["agent", "count"],
		"entities": ["Agent"],
		"question_kind": "return_value",
		"intent_confidence": 0.92,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.78,
		"predicate_axis": "register",
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"observation_summary": "count request is not diagnostic",
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.IntentConfidence != 0.92 {
		t.Errorf("IntentConfidence = %f, want 0.92", rm.IntentConfidence)
	}
	if !rm.Predicates.IsScalarAnswer || !rm.Predicates.IsCountQuestion {
		t.Errorf("Predicates not plumbed, got %+v", rm.Predicates)
	}
	if rm.Predicates.IsCrossComponent {
		t.Errorf("IsCrossComponent should be false, got %+v", rm.Predicates)
	}
	if rm.PredicateAxis != types.AxisRegister {
		t.Errorf("PredicateAxis = %q, want register", rm.PredicateAxis)
	}
	if rm.DiagnosticProfile.CurrentRisk || rm.DiagnosticProfile.CurrentVersionCheck || rm.DiagnosticProfile.Confidence != 0.7 {
		t.Errorf("DiagnosticProfile not plumbed, got %+v", rm.DiagnosticProfile)
	}
	if rm.DiagnosticProfile.ObservationSummary != "count request is not diagnostic" {
		t.Errorf("DiagnosticProfile.ObservationSummary = %q", rm.DiagnosticProfile.ObservationSummary)
	}
}

func TestEmitAnalysis_Execute_PersistsConversationReferenceProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("那个配置项默认值是什么")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["config", "default", "explore"],
		"entities": ["explore_mid_loop_hint_budget"],
		"question_kind": "config_mapping",
		"answer_subject": {
			"kind": "config_key",
			"entity_axes": ["config key → value"],
			"confidence": 0.88
		},
		"intent_confidence": 0.91,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.82,
		"predicate_axis": "configure",
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"conversation_reference_profile": {
			"requires_prior_context": true,
			"needs_repo_verification": true,
			"ambiguity": "none",
			"resolved_subjects": [{
				"surface": "explore_mid_loop_hint_budget",
				"kind": "config_key",
				"source": "prior_context",
				"role": "primary_subject",
				"use_as_exact_target": true,
				"confidence": 0.91
			}]
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ConversationReferenceProfile == nil {
		t.Fatalf("ConversationReferenceProfile not persisted: %+v", rm)
	}
	profile := rm.ConversationReferenceProfile
	if !profile.RequiresPriorContext || !profile.NeedsRepoVerification || profile.Ambiguity != types.ConversationReferenceAmbiguityNone {
		t.Fatalf("ConversationReferenceProfile flags wrong: %+v", profile)
	}
	if got := profile.ResolvedSubjects; len(got) != 1 ||
		got[0].Surface != "explore_mid_loop_hint_budget" ||
		got[0].Kind != types.SubjectConfigKey ||
		got[0].Source != types.ConversationReferenceSourcePriorContext ||
		!got[0].UseAsExactTarget {
		t.Fatalf("ResolvedSubjects not plumbed: %+v", got)
	}
	if len(rm.AnalyzerHints.ExactTargets) != 0 {
		t.Fatalf("prior-only resolved subject must not be copied into exact_targets: %+v", rm.AnalyzerHints.ExactTargets)
	}
}

func TestEmitAnalysis_Execute_DropsCurrentRequestOnlyConversationSubjects(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explore_mid_loop_hint_budget 默认值是什么")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["config", "default", "explore"],
		"entities": ["explore_mid_loop_hint_budget"],
		"question_kind": "config_mapping",
		"intent_confidence": 0.91,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.82,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"conversation_reference_profile": {
			"requires_prior_context": false,
			"needs_repo_verification": true,
			"ambiguity": "none",
			"resolved_subjects": [{
				"surface": "explore_mid_loop_hint_budget",
				"kind": "config_key",
				"source": "current_request",
				"role": "primary_subject",
				"use_as_exact_target": false,
				"confidence": 0.91
			}]
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("current-request-only conversation subjects should be dropped, not rejected: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.ConversationReferenceProfile != nil {
		t.Fatalf("current-request-only subjects should not pollute prior-conversation profile: %+v", rm.ConversationReferenceProfile)
	}
}

func TestEmitAnalysis_Execute_NormalizesPriorSubjectRequiresPriorContext(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("那个配置项默认值是什么")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["config", "default", "explore"],
		"entities": ["explore_mid_loop_hint_budget"],
		"question_kind": "config_mapping",
		"intent_confidence": 0.91,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.82,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"conversation_reference_profile": {
			"requires_prior_context": false,
			"needs_repo_verification": true,
			"ambiguity": "none",
			"resolved_subjects": [{
				"surface": "explore_mid_loop_hint_budget",
				"kind": "config_key",
				"source": "prior_context",
				"role": "primary_subject",
				"use_as_exact_target": true,
				"confidence": 0.91
			}]
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("prior-context subject should normalize requires_prior_context, not reject: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ConversationReferenceProfile == nil {
		t.Fatalf("ConversationReferenceProfile not persisted: %+v", rm)
	}
	if !rm.ConversationReferenceProfile.RequiresPriorContext {
		t.Fatalf("requires_prior_context should be normalized true: %+v", rm.ConversationReferenceProfile)
	}
	if got := rm.ConversationReferenceProfile.ResolvedSubjects; len(got) != 1 ||
		got[0].Source != types.ConversationReferenceSourcePriorContext ||
		!got[0].UseAsExactTarget {
		t.Fatalf("resolved prior subject not preserved: %+v", got)
	}
}

func TestEmitAnalysis_Execute_PersistsSourceScopeProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("测试文件里哪些 handler 会走 SubAgent?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["handler", "SubAgent", "test"],
		"entities": ["SubAgent"],
		"question_kind": "enumeration",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": true,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_scope_profile": {
			"requested_scope": "test",
			"include_auxiliary_as_principal": true,
			"confidence": 0.91,
			"rationale": "current request asks about test files as principal scope"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "source_scope=test") {
		t.Fatalf("summary should surface source scope lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceScopeProfile == nil {
		t.Fatalf("SourceScopeProfile not persisted: %+v", rm)
	}
	if rm.SourceScopeProfile.RequestedScope != types.SourceScopeTest ||
		!rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal ||
		!rm.SourceScopeProfile.AllowsAuxiliaryPrincipal() {
		t.Fatalf("SourceScopeProfile fields wrong: %+v", rm.SourceScopeProfile)
	}
}

func TestEmitAnalysis_SourceInventorySoftensEchoedProductionScope(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign func", "public class", "package"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "production",
			"source_quotes": ["仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class"],
			"confidence": 0.95,
			"rationale": "model inferred production from broad grep"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type"],
			"requested_fields": ["name", "location", "summary"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source_inventory_profile should remain active: %+v", rm)
	}
	if rm.SourceScopeProfile != nil {
		t.Fatalf("echoed production source scope should be softened before persistence: %+v", rm.SourceScopeProfile)
	}
	if !types.SourceInventoryRequiresRepoWideLens(*rm) {
		t.Fatalf("softened source-inventory request should require repo-wide lens: %+v", rm)
	}
	if !strings.Contains(res.Summary, "auto-softened") {
		t.Fatalf("summary should disclose source-scope softening, got %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceInventorySoftensConstructOnlyAuxiliaryScope(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign func", "public class", "package"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "auxiliary",
			"include_auxiliary_as_principal": true,
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.95,
			"rationale": "model inferred auxiliary from repository layout"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type"],
			"requested_fields": ["name", "location", "package"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source_inventory_profile should remain active: %+v", rm)
	}
	if rm.SourceScopeProfile != nil {
		t.Fatalf("construct-only auxiliary source scope should be softened before persistence: %+v", rm.SourceScopeProfile)
	}
	if !types.SourceInventoryRequiresRepoWideLens(*rm) {
		t.Fatalf("softened source-inventory request should require repo-wide lens: %+v", rm)
	}
	if !strings.Contains(res.Summary, "construct quote") && !strings.Contains(res.Summary, "construct quote(s)") {
		t.Fatalf("summary should disclose construct-only scope softening, got %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceInventorySoftensScopeWhenEveryQuoteIsRejected(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些 extend 块和 public class？")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["extend", "public class"],
		"entities": ["extend", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.75,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "auxiliary",
			"include_auxiliary_as_principal": true,
			"source_quotes": ["thirdparty Cangjie corpus"],
			"confidence": 0.95,
			"rationale": "model inferred a repository layout"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["extend 块", "public class"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source inventory profile should remain active: %+v", rm)
	}
	if rm.SourceScopeProfile != nil {
		t.Fatalf("scope with zero validated quotes must not retain hard authority: %+v", rm.SourceScopeProfile)
	}
	for _, want := range []string{"entry ignored", "no validated current-request source quote"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q: %s", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_SourceInventoryKeepsIndependentSourceScopeQuote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("只列出 Cangjie 测试语料中的 extend 块和 public class。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["Cangjie", "测试语料", "extend", "public class"],
		"entities": ["Cangjie 测试语料", "extend", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.75,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "auxiliary",
			"include_auxiliary_as_principal": true,
			"source_quotes": ["Cangjie 测试语料"],
			"confidence": 0.95,
			"rationale": "current request names a source material boundary"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["extend 块", "public class"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceScopeProfile == nil {
		t.Fatalf("independent source scope quote should be preserved: %+v", rm)
	}
	if rm.SourceScopeProfile.RequestedScope != types.SourceScopeAuxiliary ||
		!rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal {
		t.Fatalf("source scope fields wrong: %+v", rm.SourceScopeProfile)
	}
}

func TestEmitAnalysis_SourceInventoryKeepsPackageModuleRequestedFields(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出仓库里的 package 声明及位置。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["package", "声明", "位置"],
		"entities": ["package 声明"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["package"],
			"requested_fields": ["name", "package", "module", "location"],
			"source_quotes": ["package 声明"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed with package/module display fields, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	profile := rm.SourceInventoryProfile
	if profile == nil || !profile.Active() {
		t.Fatalf("source_inventory_profile should remain active: %+v", profile)
	}
	if len(profile.TargetRoles) != 1 || profile.TargetRoles[0] != types.AnswerCandidateRolePackage {
		t.Fatalf("target role should remain package, got %+v", profile.TargetRoles)
	}
	gotFields := map[types.SourceInventoryRequestedField]bool{}
	for _, field := range profile.RequestedFields {
		gotFields[field] = true
	}
	if !gotFields[types.SourceInventoryFieldName] || !gotFields[types.SourceInventoryFieldLocation] {
		t.Fatalf("valid display fields should survive, got %+v", profile.RequestedFields)
	}
	if !gotFields[types.SourceInventoryFieldPackage] || !gotFields[types.SourceInventoryFieldModule] {
		t.Fatalf("package/module display attributes should survive, got %+v", profile.RequestedFields)
	}
	if strings.Contains(res.Summary, "requested_fields") && strings.Contains(res.Summary, "ignored") {
		t.Fatalf("package/module display fields should not be treated as invalid: %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceInventoryDemotesPackageDisplayRoleWithConstructPrincipals(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	payload := `{
		"intent": "enumerate",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["extend", "foreign func", "public class", "package"],
		"entities": ["extend", "foreign func", "public class", "package"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type", "package"],
			"requested_fields": ["name", "location", "package", "module", "namespace"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil {
		t.Fatalf("RequestModel/source_inventory_profile not persisted: %+v", rm)
	}
	roles := rm.SourceInventoryProfile.PrincipalTargetRoles()
	if len(roles) != 2 || roles[0] != types.AnswerCandidateRoleFunction || roles[1] != types.AnswerCandidateRoleType {
		t.Fatalf("package display role should not remain principal with construct roles, got %+v profile=%+v", roles, rm.SourceInventoryProfile)
	}
	if rm.SourceInventoryProfile.RequiresPrincipalRole(types.AnswerCandidateRolePackage) {
		t.Fatalf("package display field leaked into principal source-inventory roles: %+v", rm.SourceInventoryProfile)
	}
	if !strings.Contains(res.Summary, "display attribute role") {
		t.Fatalf("normalization should be disclosed in summary warnings, got %q", res.Summary)
	}
}

func TestEmitAnalysis_SourceInventoryKeepsProductionWithTypedAuxiliaryExclusion(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出 production files 里的 public functions，不包含 fixtures。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["production files", "public functions", "fixtures"],
		"entities": ["public functions"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "production",
			"source_quotes": ["production files"],
			"confidence": 0.95
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["public functions"],
			"confidence": 0.95
		},
		"answer_exclusion_policy": {
			"is_exclusion_requested": true,
			"excluded_candidate_roles": ["fixture"],
			"source_quotes": ["不包含 fixtures"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceScopeProfile == nil {
		t.Fatalf("production source scope should persist with typed auxiliary exclusion: %+v", rm)
	}
	if rm.SourceScopeProfile.RequestedScope != types.SourceScopeProduction {
		t.Fatalf("source scope = %+v, want production", rm.SourceScopeProfile)
	}
	if types.SourceInventoryRequiresRepoWideLens(*rm) {
		t.Fatalf("typed auxiliary exclusion should allow bounded production lens: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_PersistsChangeImpactProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Which production files would need changes if CitationReq.Required changed type?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["CitationReq", "Required", "production"],
		"entities": ["CitationReq.Required"],
		"question_kind": "enumeration",
		"answer_subject": {
			"kind": "file_path",
			"entity_axes": ["changed target → affected files"],
			"confidence": 0.91
		},
		"intent_confidence": 0.93,
		"complexity_confidence": 0.88,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": true,
			"is_relational_lookup": true,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"change_impact_profile": {
			"is_change_impact": true,
			"target": "CitationReq.Required",
			"target_kind": "struct_field",
			"scope": "production",
			"requested_output": "files",
			"affected_site_kinds": ["definition", "assignment", "read", "guard", "validation"],
			"confidence": 0.92,
			"rationale": "answer asks affected production files for a target type change"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "change_impact=CitationReq.Required") {
		t.Fatalf("summary should surface typed impact lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ChangeImpactProfile == nil || !rm.ChangeImpactProfile.Active() {
		t.Fatalf("ChangeImpactProfile not persisted: %+v", rm)
	}
	profile := rm.ChangeImpactProfile
	if profile.Target != "CitationReq.Required" ||
		profile.TargetKind != types.SubjectStructField ||
		profile.Scope != types.ImpactScopeProduction ||
		profile.RequestedOutput != types.ImpactOutputFiles {
		t.Fatalf("ChangeImpactProfile fields wrong: %+v", profile)
	}
	if got := profile.AffectedSiteKinds; len(got) != 5 || got[2] != types.ImpactSiteRead {
		t.Fatalf("AffectedSiteKinds not preserved: %+v", got)
	}
}

func TestEmitAnalysis_Execute_PersistsFieldValueProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["CitationReq", "Required"],
		"entities": ["CitationReq.Required"],
		"question_kind": "return_value",
		"answer_subject": {
			"kind": "numeric",
			"entity_axes": ["field literal count"],
			"confidence": 0.91
		},
		"intent_confidence": 0.93,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"field_value_profile": {
			"is_field_value_lookup": true,
			"target": "CitationReq.Required",
			"literal": "false",
			"literal_kind": "bool",
			"source_quote": "CitationReq.Required 设置为 false",
			"confidence": 0.96,
			"rationale": "current request asks for production-site count under this field literal"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "field_value=CitationReq.Required=false") {
		t.Fatalf("summary should surface typed field/value lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.FieldValueProfile == nil || !rm.FieldValueProfile.Active() {
		t.Fatalf("FieldValueProfile not persisted: %+v", rm)
	}
	profile := rm.FieldValueProfile
	if profile.Owner != "CitationReq" ||
		profile.Field != "Required" ||
		profile.Literal != "false" ||
		profile.LiteralKind != types.FieldValueLiteralBool {
		t.Fatalf("FieldValueProfile fields wrong: %+v", profile)
	}
}

func TestEmitAnalysis_Execute_RejectsUngroundedFieldValueProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Foo.timeout = 30 的生产代码位点有几处？")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["Foo", "timeout"],
		"entities": ["Foo.timeout"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"field_value_profile": {
			"is_field_value_lookup": true,
			"target": "Foo.timeout",
			"literal": "30",
			"literal_kind": "number",
			"source_quote": "Foo.timeout = 60",
			"confidence": 0.96
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatalf("Execute should reject ungrounded field_value_profile, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "field_value_profile.source_quote") {
		t.Fatalf("rejection should name field_value_profile.source_quote, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_ConvertsFieldValueProfileForRuntimeArtifact(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("这段 HiTrace 里 GC span 的开始事件在附件 trace 的第几行？有没有超过 50ms 的 GC span？只分析 trace")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"gc-pause"}},
		Observations: []types.PerfObservation{{
			Kind:       "span",
			Subject:    "H:GC:Collect",
			Summary:    "GC span starts on trace line 5 and lasts 8ms",
			LineStart:  5,
			LineEnd:    6,
			DurationMs: 8,
			Confidence: 0.95,
		}},
	})
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["GC", "span", "trace", "50ms"],
		"entities": ["H:GC:Collect"],
		"question_kind": "return_value",
		"answer_subject": {"kind": "numeric", "confidence": 0.9},
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "只分析 trace",
			"confidence": 0.9
		},
		"field_value_profile": {
			"is_field_value_lookup": true,
			"literal": "8ms",
			"literal_kind": "number",
			"source_quote": "有没有超过 50ms 的 GC span",
			"confidence": 0.96
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("invalid optional field_value_profile on runtime artifact should be dropped, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.FieldValueProfile != nil {
		t.Fatalf("invalid runtime-artifact field_value_profile should be dropped, got %+v", rm.FieldValueProfile)
	}
	if rm.RuntimeArtifactValueProfile == nil || !rm.RuntimeArtifactValueProfile.Active() {
		t.Fatalf("runtime artifact field value should be preserved as artifact_value_profile: %+v", rm.RuntimeArtifactValueProfile)
	}
	if got := rm.RuntimeArtifactValueProfile.Value; got != "8ms" {
		t.Fatalf("artifact value = %q, want 8ms", got)
	}
	if got := rm.RuntimeArtifactValueProfile.Target; !strings.Contains(got, "GC span") {
		t.Fatalf("artifact target should preserve the runtime value surface, got %q", got)
	}
	if !strings.Contains(res.Summary, "artifact_value=") {
		t.Fatalf("summary should surface artifact_value profile, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsRuntimeArtifactValueProfileInMixedCurrentSourceTurn(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("结合当前源码解释 trace 里 H:Frame duration 86.1ms 为什么发生")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "frame.tracebundle.json", Signals: []string{"frame-jank"}},
		Frames: []types.PerfFrame{{
			FrameNo:    42,
			DurationMs: 86.1,
			Janky:      true,
		}},
	})
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "root_cause",
		"scenario": "performance_bottleneck",
		"complexity": "medium",
		"keywords": ["H:Frame", "duration", "86.1ms", "源码"],
		"entities": ["H:Frame"],
		"question_kind": "explanation",
		"answer_subject": {"kind": "numeric", "confidence": 0.9},
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": true,
			"historical_regression": false,
			"current_version_check": true,
			"confidence": 0.8
		},
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["explain_current_mechanism"],
			"source_quotes": ["结合当前源码解释"],
			"target_terms": ["H:Frame"],
			"confidence": 0.85
		},
		"artifact_value_profile": {
			"is_artifact_value_lookup": true,
			"target": "H:Frame duration",
			"value": "86.1",
			"unit": "ms",
			"literal_kind": "number",
			"artifact_refs": ["frame.tracebundle.json"],
			"observation_refs": ["frame:42"],
			"confidence": 0.94,
			"rationale": "trace frame observation supplies the exact duration"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("artifact_value_profile should persist in mixed trace/current-source turn, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.RuntimeArtifactValueProfile == nil || !rm.RuntimeArtifactValueProfile.Active() {
		t.Fatalf("RuntimeArtifactValueProfile not persisted: %+v", rm)
	}
	if rm.FieldValueProfile != nil {
		t.Fatalf("artifact_value_profile must not become current-source field_value_profile: %+v", rm.FieldValueProfile)
	}
	contract := types.CompileAnswerIntentContract(*rm, nil)
	if !contract.HasOrigin(types.AnswerEvidenceOriginRuntimeArtifact) ||
		!contract.HasOutput(types.AnswerRequestedOutputKeyValue) {
		t.Fatalf("artifact value should request runtime key-value support, got %+v", contract)
	}
}

func TestEmitAnalysis_RuntimeDiagnosticDropsAnalyzerArtifactValueAndSummary(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	objective := "只分析这份 trace，不分析代码。目标线程是 app-100，请分析它在 5.000s 到 5.007s 这一帧窗口内的丢帧根因。"
	mu := types.NewMutableState(objective)
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Authority: types.PerfObservationAuthorityPreTriageModelExtraction,
			LineStart: 4,
			LineEnd:   7,
		}},
	})
	payload := `{
		"intent":"root_cause",
		"scenario":"performance_bottleneck",
		"complexity":"moderate",
		"keywords":["trace","app-100","丢帧","根因"],
		"entities":["app-100"],
		"question_kind":"mechanism",
		"predicate_axis":"condition",
		"intent_confidence":0.95,
		"complexity_confidence":0.9,
		"kind_confidence":0.9,
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":false,
			"is_history_lookup":false,
			"is_diagnostic_question":true,
			"has_per_member_table":false
		},
		"diagnostic_profile":{
			"is_diagnostic":true,
			"current_risk":false,
			"historical_regression":false,
			"current_version_check":false,
			"observation_summary":"VerifyClass sync-rpc directly blocked app-100 for 5800ms",
			"confidence":0.95
		},
		"artifact_value_profile":{
			"is_artifact_value_lookup":true,
			"target":"VerifyClass stall duration",
			"value":"5800",
			"unit":"ms",
			"literal_kind":"number",
			"observation_refs":["observation:1"],
			"confidence":0.95
		},
		"external_observation_policy":{
			"current_source_mode":"exclude",
			"exclusion_kind":"explicit_user_exclusion",
			"current_source_exclusion_quote":"只分析这份 trace，不分析代码",
			"confidence":1.0
		},
		"answer_exclusion_policy":{
			"is_exclusion_requested":true,
			"excluded_candidate_roles":["function","method","type","file"],
			"source_quotes":["只分析这份 trace，不分析代码"],
			"confidence":0.9
		},
		"runtime_artifact_scope_profile":{
			"requested_scope":"explicit_time_window",
			"time_start":5.0,
			"time_end":5.007,
			"source_quote":"5.000s 到 5.007s 这一帧窗口",
			"confidence":1.0
		},
		"runtime_target_profile":{
			"declaration":"named_target",
			"source_quote":"目标线程是 app-100",
			"confidence":1.0
		},
		"runtime_targets":[{"kind":"thread","thread":"app-100","source":"user_explicit","confidence":1.0}],
		"runtime_question_profile":{
			"scope":"causal_diagnosis",
			"source_quote":"丢帧根因",
			"confidence":0.95
		},
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.95,
			"dimensions":[{"index":1,"label":"丢帧根因","role":"causal_attribution","source_quote":"丢帧根因","required":true}]
		},
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":1.0},
		"completeness_obligation":{"required":false,"source_quote":""},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.8},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.8}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime diagnostic should normalize optional analyzer guesses: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.RuntimeArtifactValueProfile != nil {
		t.Fatalf("non-scalar diagnostic retained analyzer artifact value: %+v", rm.RuntimeArtifactValueProfile)
	}
	if rm.DiagnosticProfile.ObservationSummary != "" {
		t.Fatalf("attached-artifact analyzer summary retained as authority: %q", rm.DiagnosticProfile.ObservationSummary)
	}
	if rm.AnswerExclusionPolicy != nil {
		t.Fatalf("current-source boundary must not become answer member-role exclusion: %+v", rm.AnswerExclusionPolicy)
	}
	for _, want := range []string{"cleared diagnostic_profile.observation_summary", "dropped artifact_value_profile outside predicates.is_scalar_answer=true", "evidence-source exclusion does not exclude answer member roles"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("normalization warning missing %q: %s", want, res.Summary)
		}
	}
}

func TestReconcileAnswerExclusionWithCurrentSourceBoundaryPreservesIndependentQuote(t *testing.T) {
	answer := &types.AnswerExclusionPolicy{
		IsExclusionRequested:   true,
		ExcludedCandidateRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleTest},
		SourceQuotes:           []string{"不包含测试"},
		Confidence:             0.9,
	}
	external := &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"不分析代码"},
		Confidence:        1,
	}
	got, warning := reconcileAnswerExclusionWithCurrentSourceBoundary(answer, external)
	if got != answer || warning != "" {
		t.Fatalf("independently quoted answer exclusion must survive unchanged: got=%+v warning=%q", got, warning)
	}
}

func TestEmitAnalysis_Execute_PersistsTypedRuntimeTargets(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "root_cause",
		"scenario": "performance_bottleneck",
		"complexity": "complex",
		"keywords": ["trace", "pid", "42591", "jank"],
		"entities": ["42591", "berlin.systrace"],
		"question_kind": "root_cause",
		"intent_confidence": 0.92,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.85,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.88
		},
		"runtime_target_profile": {"declaration":"named_target","source_quote":"42591进程","confidence":0.96},
		"runtime_targets": [{
			"kind": "process",
			"pid": 42591,
			"source": "user_explicit",
			"confidence": 0.96,
			"description": "user explicitly named process id 42591"
		}]
	}`
	res, mu := runEmitAnalysisPayload(t, "分析42591进程在6793222s 到 6793225s 期间滑动卡顿的深层次根因，不要分析代码", payload)
	if !res.Success {
		t.Fatalf("runtime_targets should persist, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.RuntimeTargets) != 1 {
		t.Fatalf("RuntimeTargets not persisted: %+v", rm)
	}
	got := rm.RuntimeTargets[0]
	if got.Kind != types.RuntimeTargetKindProcess || got.PID != 42591 || got.Source != "user_explicit" {
		t.Fatalf("RuntimeTargets[0] = %+v, want typed process pid=42591", got)
	}
	if !strings.Contains(res.Summary, "runtime_targets=1") {
		t.Fatalf("summary should surface runtime target count, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsMalformedRuntimeTargetInsteadOfWarningAndDropping(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["trace", "pid", "59566", "D-state"],
		"entities": ["com.baidu.tieba-59566", "59566"],
		"question_kind": "mechanism",
		"intent_confidence": 0.92,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.85,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.88
		},
		"runtime_targets": [{
			"kind": "process",
			"thread": "com.baidu.tieba-59566",
			"source": "user_explicit",
			"confidence": 0.96
		}]
	}`
	res, mu := runEmitAnalysisPayload(t, "分析 com.baidu.tieba 59566 进程的 D-state", payload)
	if res.Success ||
		!strings.Contains(res.Summary, "runtime_targets[0] is structurally invalid") ||
		!strings.Contains(res.Summary, "process target requires pid") {
		t.Fatalf("malformed process target must fail loud, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("rejected malformed target must not persist a partial RequestModel: %+v", mu.RequestModel())
	}
}

func TestParseRuntimeTargetsRejectsWholeSetWhenAnyIdentityIsMalformed(t *testing.T) {
	confidence := 0.9
	pid := 42
	targets, warnings, errText := parseRuntimeTargets([]emitRuntimeTargetParam{
		{Kind: "process", PID: &pid, Source: "user_explicit", Confidence: &confidence},
		{Kind: "process", Thread: "missing-pid-43", Source: "user_explicit", Confidence: &confidence},
	})
	if errText == "" || len(targets) != 0 {
		t.Fatalf("partial target set must not survive malformed sibling: targets=%+v warnings=%v err=%q", targets, warnings, errText)
	}
}

func TestEmitAnalysis_Execute_RuntimeTargetInvalidSourceCannotCarryNamedAuthority(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "root_cause",
		"scenario": "performance_bottleneck",
		"complexity": "complex",
		"keywords": ["trace", "pid", "42591", "jank"],
		"entities": ["42591"],
		"question_kind": "root_cause",
		"intent_confidence": 0.92,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.85,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.88
		},
		"runtime_target_profile": {"declaration":"named_target","source_quote":"42591进程","confidence":0.96},
		"runtime_targets": [{
			"kind": "process",
			"pid": 42591,
			"source": "model guessed from prose",
			"confidence": 0.96
		}]
	}`
	res, mu := runEmitAnalysisPayload(t, "分析42591进程在6793222s 到 6793225s 期间滑动卡顿的深层次根因", payload)
	if res.Success || !strings.Contains(res.Summary, "source must be user_explicit") {
		t.Fatalf("named target with invalid/cleared provenance must fail loud, got success=%t summary=%q", res.Success, res.Summary)
	}
	if mu.RequestModel() != nil {
		t.Fatalf("rejected named target must not persist request authority: %+v", mu.RequestModel())
	}
}

func TestEmitAnalysis_Execute_DropsInvalidFieldValueProfileForGenericCountCurrentSource(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("统计 internal/tool 下非测试 Go 文件数量，并解释当前实现如何计算这个值。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["internal/tool", "Go", "文件数量", "当前实现"],
		"entities": ["internal/tool"],
		"question_kind": "return_value",
		"answer_subject": {"kind": "numeric", "confidence": 0.9},
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": true,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["explain_current_mechanism"],
			"source_quotes": ["解释当前实现"],
			"confidence": 0.82
		},
		"field_value_profile": {
			"is_field_value_lookup": true,
			"target": "internal/tool non-test Go files",
			"literal": "count",
			"literal_kind": "",
			"source_quote": "统计 internal/tool 下非测试 Go 文件数量",
			"confidence": 0.72
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("generic count/current-source field_value_profile should be dropped, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.FieldValueProfile != nil {
		t.Fatalf("invalid generic count field_value_profile should be dropped, got %+v", rm.FieldValueProfile)
	}
	if rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active() {
		t.Fatalf("current_source_explanation_profile should survive: %+v", rm.CurrentSourceExplanationProfile)
	}
	if !strings.Contains(res.Summary, "generic scalar/current-source request") {
		t.Fatalf("expected summary warning to record dropped optional profile, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ExternalObservationPolicyExcludeRequiresAnchoredQuote(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只分析日志，不要读取源码")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "runtime"],
		"entities": ["panic"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "不要读取源码",
			"artifact_citation_quotes": ["not in request"],
			"confidence": 0.91
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("policy should be soft-normalized, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("anchored exclude policy should survive: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.SourceQuotes; len(got) != 1 || got[0] != "不要读取源码" {
		t.Fatalf("source quotes should keep only anchored entries, got %+v", got)
	}
	if !strings.Contains(res.Summary, "external_observation_policy=exclude") {
		t.Fatalf("summary should report policy mode, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "external_observation_policy.artifact_citation_quotes entry ignored") {
		t.Fatalf("summary should warn for unanchored quote, got %q", res.Summary)
	}
}

func TestEmitAnalysis_RedundantToolNameTypeFieldPreservesExternalObservationPolicy(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只分析这份 trace，不分析代码。请分析 com.baidu.tieba 59566 主线程在 34579.472865s 到 34579.587805s 这一帧窗口内的卡顿原因。")
	payload := `{
		"type": "emit_analysis",
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "complex",
		"keywords": ["trace", "jank", "main_thread"],
		"entities": ["com.baidu.tieba", "59566"],
		"question_kind": "mechanism",
		"language": "zh",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.9,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": true,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": true,
			"historical_regression": true,
			"current_version_check": true,
			"confidence": 0.9
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": 0.8
		},
		"error_granularity_profile": {
			"is_granularity_question": false,
			"confidence": 0.8
		},
		"runtime_artifact_scope_profile": {
			"requested_scope": "full_artifact",
			"source_quote": "这份 trace",
			"confidence": 0.95
		},
		"external_observation_policy": {
			"artifact_citation_mode": "external_only",
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "只分析这份 trace，不分析代码",
			"confidence": 0.95
		},
		"completeness_obligation":{"required":false,"source_quote":""}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("redundant top-level type should be repaired without losing policy, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("anchored runtime-only policy should survive redundant type repair: %+v", rm)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneExcluded {
		t.Fatalf("CurrentSourceLaneDecision=%s, want excluded", got)
	}
	if rm.DiagnosticProfile.CurrentRisk || rm.DiagnosticProfile.CurrentVersionCheck || rm.DiagnosticProfile.HistoricalRegression {
		t.Fatalf("diagnostic current-source flags should be repaired under exclude policy: %+v", rm.DiagnosticProfile)
	}
	if !strings.Contains(res.Summary, "external_observation_policy=exclude") {
		t.Fatalf("summary should expose preserved exclude policy, got %q", res.Summary)
	}
}

func TestEmitAnalysis_SynthesizesCurrentSourceAllowFromRouteBackedRuntimeArtifact(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("explain attached log against the current checkout")
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Subject:   "finalizer timeout",
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 2,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["finalizer", "timeout", "validation"],
		"entities": ["finalizer", "LLM stream timeout", "validation failure"],
		"question_kind": "mechanism"
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			Operation:                 "investigate",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("route-backed runtime artifact should synthesize allow policy, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil ||
		rm.ExternalObservationPolicy.CurrentSourceMode != types.ExternalObservationCurrentSourceAllow {
		t.Fatalf("route-backed runtime artifact should synthesize current-source allow policy: %+v", rm)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("route-backed synthesized allow should require current source, got %s", got)
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatalf("route-backed synthesized allow must not be source-optional: %+v", rm)
	}
	for _, want := range []string{"external_observation_policy=allow", "synthesized as allow from typed route metadata"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should expose synthesized allow policy %q, got %q", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_SynthesizesCurrentSourceAllowFromMixedRuntimeRoute(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("explain attached log together with current source")
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Subject:   "finalizer timeout",
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 2,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["finalizer", "timeout", "validation"],
		"entities": ["finalizer", "LLM stream timeout", "validation failure"],
		"question_kind": "mechanism"
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "hybrid",
			Source:                    "mixed",
			Operation:                 "investigate",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("mixed runtime/source route should synthesize allow policy, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil ||
		rm.ExternalObservationPolicy.CurrentSourceMode != types.ExternalObservationCurrentSourceAllow {
		t.Fatalf("mixed runtime/source route should synthesize current-source allow policy: %+v", rm)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("mixed route synthesized allow should require current source, got %s", got)
	}
	if !strings.Contains(res.Summary, "synthesized as allow from typed route metadata") {
		t.Fatalf("summary should expose synthesized mixed-route allow policy, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ArtifactPipelineAccessDoesNotSynthesizeCurrentSourceAllow(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("explain attached runtime log")
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Subject:   "finalizer timeout",
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 2,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["finalizer", "timeout"],
		"entities": ["finalizer", "LLM stream timeout"],
		"question_kind": "mechanism"
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			Operation:                 "investigate",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("artifact-only route should remain analyzable, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("request model missing")
	}
	if rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.CurrentSourceMode == types.ExternalObservationCurrentSourceAllow {
		t.Fatalf("optional current-source route must not synthesize current-source allow: %+v", rm.ExternalObservationPolicy)
	}
	if strings.Contains(res.Summary, "synthesized as allow") {
		t.Fatalf("summary must not claim synthesized current-source allow: %q", res.Summary)
	}
}

func TestEmitAnalysis_RouteBackedRuntimeArtifactDoesNotOverrideExplicitSourceExclude(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只分析日志，不要读取源码")
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Subject:   "finalizer timeout",
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 2,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "timeout"],
		"entities": ["finalizer"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "不要读取源码",
			"confidence": 0.9
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			Operation:                 "investigate",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("explicit source exclusion should remain valid, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("explicit source exclusion should not be overridden by route hint: %+v", rm)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneExcluded {
		t.Fatalf("explicit source exclusion should win over route hint, got %s", got)
	}
	if strings.Contains(res.Summary, "synthesized as allow") {
		t.Fatalf("summary should not claim synthesized allow when explicit exclude wins: %q", res.Summary)
	}
}

func TestEmitAnalysis_CurrentSourceExplanationSoftensConflictingExternalExclude(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("这份 trace 里 RenderService span 怎么被系统解析？请结合当前源码解释，不要把 trace 行号当成源码引用。")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"RenderService"}},
		Observations: []types.PerfObservation{{
			Kind:       "span",
			Subject:    "RenderService",
			Summary:    "RenderService span observed in trace",
			LineStart:  7,
			Confidence: 0.95,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["trace", "RenderService", "current source"],
		"entities": ["RenderService", "trace parser"],
		"question_kind": "mechanism",
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["trace_current_flow", "explain_current_mechanism"],
			"source_quotes": ["请结合当前源码解释"],
			"confidence": 0.95
		},
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"artifact_citation_mode": "external_only",
			"current_source_exclusion_quote": "不要把 trace 行号当成源码引用",
			"artifact_citation_quotes": ["不要把 trace 行号当成源码引用"],
			"confidence": 0.9
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route: "repo", Source: "artifact", Operation: "investigate", NeedsRepoAccess: true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("conflicting current-source/exclude policy should be normalized, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.CurrentSourceExplanationProfile == nil || !rm.CurrentSourceExplanationProfile.Active() {
		t.Fatalf("current-source explanation profile should survive: %+v", rm)
	}
	if rm.ExternalObservationPolicy == nil {
		t.Fatalf("external observation policy should survive for artifact citation mode: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceAllow {
		t.Fatalf("CurrentSourceMode=%q, want allow after conflicting exclude normalization", got)
	}
	if rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("conflicting exclude must not suppress current source: %+v", rm.ExternalObservationPolicy)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("artifact citation mode should survive, got %q", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("current-source profile should require source lane, got %s", got)
	}
	if !strings.Contains(res.Summary, "auto-softened to allow") {
		t.Fatalf("summary should expose policy normalization warning, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ExternalObservationPolicyUnanchoredExcludeDefaults(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("分析日志")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "runtime"],
		"entities": ["panic"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "不要读取源码",
			"confidence": 0.91
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("unanchored exclude should become a warning, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || (rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()) {
		t.Fatalf("unanchored exclude must not suppress current source: %+v", rm)
	}
	if !strings.Contains(res.Summary, "exclude ignored") {
		t.Fatalf("summary should explain ignored exclusion, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ExternalObservationPolicyExcludeRequiresTypedExclusionKind(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("这段日志显示 finalizer 因 LLM timeout 触发重试；请结合当前源码解释系统如何区分模型响应超时和成文校验失败，并说明这个日志结论的边界。")
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRetryCycle,
			Summary:    "first_byte_timeout exceeded after 40s",
			LineStart:  2,
			Confidence: 0.95,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["timeout", "finalizer", "validation"],
		"entities": ["finalizer", "LLM timeout"],
		"question_kind": "conditional",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"artifact_citation_mode": "external_only",
			"current_source_exclusion_quote": "请结合当前源码解释系统如何区分模型响应超时和成文校验失败，并说明这个日志结论的边界",
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing exclusion_kind should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("policy should survive for artifact citation mode: %+v", rm)
	}
	if rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("exclude without typed exclusion_kind must not suppress current source: %+v", rm.ExternalObservationPolicy)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceDefault {
		t.Fatalf("CurrentSourceMode=%q, want neutral default after runtime-artifact invalid exclude repair", got)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("artifact citation mode should survive, got %q", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneAllowedOptional {
		t.Fatalf("invalid runtime-artifact exclude should keep source allowed but optional without an independent typed obligation, got %s", got)
	}
	if !strings.Contains(res.Summary, "exclusion_kind") {
		t.Fatalf("summary should explain ignored exclusion_kind, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "invalid current_source_mode=exclude fell back to default") {
		t.Fatalf("summary should expose invalid-exclude neutral fallback, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ArtifactCitationCompatibilityQuotesCannotMintSourceExclusion(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	raw := "这段日志为什么触发 finalizer 重写？请结合当前源码说明这是模型响应异常还是系统校验失败；不要把日志行当成当前源码引用。"
	mu := types.NewMutableState(raw)
	mu.SetLogTriage(&types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRetryCycle,
			Summary:    "first_byte_timeout exceeded after 40s",
			LineStart:  2,
			Confidence: 0.95,
		}},
	})
	// B3's older analyzer shape put a citation-identity boundary and a
	// positive current-source request in one role-less quote array. The
	// strings remain available for audit, but they are not typed proof that
	// current source itself is banned.
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["finalizer", "timeout", "validation"],
		"entities": ["finalizer", "LLM timeout"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"artifact_citation_mode": "external_only",
			"source_quotes": [
				"不要把日志行当成当前源码引用",
				"请结合当前源码说明这是模型响应异常还是系统校验失败"
			],
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "artifact",
			Operation:       "investigate",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("role-less compatibility quotes should fail open without losing artifact citation policy, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("policy should survive for external citation identity: %+v", rm)
	}
	if rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("legacy combined quotes must not mint current-source exclusion: %+v", rm.ExternalObservationPolicy)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceAllow {
		t.Fatalf("route-backed mixed artifact should recover allow mode, got %q", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneRequired {
		t.Fatalf("mixed artifact/current-source route should require source after invalid exclusion is dropped, got %s", got)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("artifact citation identity should survive, got %q", got)
	}
	for _, want := range []string{
		"no current_source_exclusion_quote",
		"invalid current_source_mode=exclude fell back to default",
		"current_source_mode synthesized as allow from typed route metadata",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should expose role-split normalization %q, got %q", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_RouteBackedRuntimeOnlyRepairsMissingExclusionKind(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只分析这份 trace，不分析代码。请分析 app 主线程卡顿原因。")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "capture.systrace", Signals: []string{"sched_switch"}},
	})
	payload := `{
			"intent": "root_cause",
			"scenario": "root_cause",
			"complexity": "complex",
			"intent_confidence": 0.9,
			"complexity_confidence": 0.8,
			"kind_confidence": 0.9,
			"keywords": ["trace", "sched_switch", "卡顿"],
			"entities": ["app", "main thread"],
			"question_kind": "mechanism",
			"predicates": {
				"is_scalar_answer": false,
				"is_role_locate_lookup": false,
				"is_count_question": false,
				"is_cross_component": false,
				"is_relational_lookup": false,
				"is_category_enumeration": false,
				"is_history_lookup": false,
				"is_diagnostic_question": true,
				"has_per_member_table": false
			},
			"diagnostic_profile": {
				"is_diagnostic": true,
				"current_risk": true,
				"historical_regression": true,
				"current_version_check": true,
				"confidence": 0.9
			},
			"answer_role_profile": {
				"is_role_binding_requested": false,
				"confidence": 0.8
			},
			"error_granularity_profile": {
				"is_granularity_question": false,
				"confidence": 0.8
			},
			"runtime_artifact_scope_profile": {
				"requested_scope": "full_artifact",
				"source_quote": "这份 trace",
				"confidence": 0.95
			},
			"runtime_target_profile": {
				"declaration": "no_named_target",
				"confidence": 0.95
			},
			"runtime_question_profile": {
				"scope": "causal_diagnosis",
				"source_quote": "卡顿原因",
				"confidence": 0.95
			},
			"requested_answer_dimensions": {
				"is_dimensioned_answer": true,
				"confidence": 0.95,
				"dimensions": [{"index":1,"label":"卡顿原因","role":"causal_attribution","source_quote":"卡顿原因","required":true}]
			},
			"external_observation_policy": {
				"current_source_mode": "exclude",
				"artifact_citation_mode": "external_only",
				"current_source_exclusion_quote": "只分析这份 trace，不分析代码",
				"confidence": 0.95
			},
			"completeness_obligation":{"required":false,"source_quote":""}
		}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu,
		TurnRouteHint: types.TurnRouteHint{
			Route:                     "repo",
			Source:                    "artifact",
			Operation:                 "investigate",
			NeedsRepoAccess:           true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
			Confidence:                0.95,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("route-backed trace-only missing exclusion_kind should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("route-backed missing exclusion_kind should become source-excluded: %+v", rm)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneExcluded {
		t.Fatalf("CurrentSourceLaneDecision=%s, want excluded", got)
	}
	if rm.DiagnosticProfile.CurrentRisk || rm.DiagnosticProfile.CurrentVersionCheck || rm.DiagnosticProfile.HistoricalRegression {
		t.Fatalf("exclude policy should clear current-source diagnostic flags: %+v", rm.DiagnosticProfile)
	}
	if !strings.Contains(res.Summary, "missing exclusion_kind repaired from typed route metadata") {
		t.Fatalf("summary should expose typed route-backed enum repair, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "invalid current_source_mode=exclude fell back to default") {
		t.Fatalf("route-backed repair must happen before invalid-exclude neutral fallback, got %q", res.Summary)
	}
}

func TestEmitAnalysis_RuntimeArtifactInvalidExcludeFallsBackToOptionalDefault(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("这段 HiTrace 需要解释 trace span 解析机制和证据边界")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"RenderService"}},
		Observations: []types.PerfObservation{{
			Kind:       "span",
			Subject:    "RenderService DoFrame",
			Summary:    "RenderService DoFrame lasted 86.111ms",
			LineStart:  5,
			LineEnd:    6,
			Confidence: 0.95,
		}},
	})
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "performance_bottleneck",
		"complexity": "moderate",
		"keywords": ["trace", "span", "RenderService"],
		"entities": ["RenderService", "DoFrame"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"artifact_citation_mode": "external_only",
			"current_source_exclusion_quote": "resolved_files=0",
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("invalid runtime-artifact exclude should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("policy should survive as default/artifact-only citation: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceDefault {
		t.Fatalf("CurrentSourceMode=%q, want default", got)
	}
	if rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("invalid exclude must not suppress current source: %+v", rm.ExternalObservationPolicy)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("ArtifactCitationMode=%q, want external_only", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneAllowedOptional {
		t.Fatalf("CurrentSourceLaneDecision=%s, want allowed_optional", got)
	}
	if !rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatalf("invalid exclude fallback must not force a runtime-only turn into broad source scanning: %+v", rm)
	}
	for _, want := range []string{
		"current_source_exclusion_quote ignored",
		"invalid current_source_mode=exclude fell back to default",
		"external_observation_policy=default",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain %q, got %q", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_TraceOnlyHallucinatedExcludeDoesNotForceSourceLane(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	raw := "请使用 trace_query 分析 1.000s 到 1.010s 的窗口，目标线程是 app-100，并判断主要阻塞原因。"
	mu := types.NewMutableState(raw)
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}},
		Observations: []types.PerfObservation{{
			Kind: "sched_switch", Subject: "app-100", Summary: "app-100 switched out", Confidence: 0.95,
		}},
	})
	payload := withV4Required(`{
		"intent": "trace",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["trace_query", "app-100", "blocking"],
		"entities": ["app-100", "trace_query"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "只分析 trace，不分析代码",
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable:         mu,
		AttachedHitrace: "sched_switch: prev_comm=app prev_pid=100",
		TurnRouteHint: types.TurnRouteHint{
			Route: "repo", Source: "artifact", Operation: "investigate", NeedsRepoAccess: true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("trace-only analysis should accept neutral source fallback, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("normalized policy missing: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceDefault {
		t.Fatalf("hallucinated source exclusion forced mode=%q, want default", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneAllowedOptional {
		t.Fatalf("hallucinated source exclusion forced current-source lane=%s: %+v", got, rm)
	}
	if !types.RuntimeArtifactRequestSourceNavigationNotRequired(*rm, true) {
		t.Fatalf("trace-only source navigation should remain optional: %+v", rm)
	}
	if !strings.Contains(res.Summary, "invalid current_source_mode=exclude fell back to default") {
		t.Fatalf("neutral fallback not disclosed: %q", res.Summary)
	}
}

func TestEmitAnalysis_TraceOnlyUnbackedAllowDoesNotForceSourceLane(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	raw := "请使用 trace_query 分析 1.000s 到 1.010s 的窗口，目标线程是 app-100，并判断主要阻塞原因。"
	mu := types.NewMutableState(raw)
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "attached.systrace", Signals: []string{"sched_switch"}},
		Observations: []types.PerfObservation{{
			Kind: "sched_switch", Subject: "app-100", Summary: "app-100 switched out", Confidence: 0.95,
		}},
	})
	payload := withV4Required(`{
		"intent": "trace",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["trace_query", "app-100", "blocking"],
		"entities": ["app-100", "trace_query"],
		"question_kind": "mechanism",
		"external_observation_policy": {
			"current_source_mode": "allow",
			"artifact_citation_mode": "external_only",
			"confidence": 0.95
		},
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": false,
			"confidence": 0.95
		}
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable:         mu,
		AttachedHitrace: "sched_switch: prev_comm=app prev_pid=100",
		TurnRouteHint: types.TurnRouteHint{
			Route: "repo", Source: "artifact", Operation: "investigate", NeedsRepoAccess: true,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
			Confidence:                0.9,
		},
	}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("trace-only unbacked allow should normalize without a retry, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("normalized policy missing: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceDefault {
		t.Fatalf("unbacked allow forced mode=%q, want default", got)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneAllowedOptional {
		t.Fatalf("unbacked allow forced current-source lane=%s: %+v", got, rm)
	}
	if !types.RuntimeArtifactRequestSourceNavigationNotRequired(*rm, true) {
		t.Fatalf("trace-only source navigation should remain optional: %+v", rm)
	}
	if !strings.Contains(res.Summary, "unbacked current_source_mode=allow fell back to default") {
		t.Fatalf("neutral fallback not disclosed: %q", res.Summary)
	}
}

func TestEmitAnalysis_ExternalObservationPolicyRepairsStringWrappedObject(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("只分析日志，不要读取源码")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "runtime"],
		"entities": ["panic"],
		"question_kind": "mechanism",
		"external_observation_policy": "{\"current_source_mode\":\"exclude\",\"exclusion_kind\":\"explicit_user_exclusion\",\"current_source_exclusion_quote\":\"不要读取源码\",\"confidence\":0.91}"
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-wrapped external_observation_policy should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("repaired exclude policy should survive: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.SourceQuotes; len(got) != 1 || got[0] != "不要读取源码" {
		t.Fatalf("source quotes should survive repair, got %+v", got)
	}
	if !strings.Contains(res.Summary, "external_observation_policy=exclude") {
		t.Fatalf("summary should report repaired policy mode, got %q", res.Summary)
	}
}

func TestEmitAnalysis_ExternalObservationPolicyArtifactCitationMode(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{
		WarnBelowKeywords:   0,
		RejectBelowKeywords: 0,
	})

	mu := types.NewMutableState("结合当前关键代码分析日志，但不要把日志行当成当前源码引用")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "runtime", "source"],
		"entities": ["panic"],
		"question_kind": "mechanism",
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["explain_current_mechanism"],
			"source_quotes": ["当前关键代码"],
			"confidence": 0.91
		},
		"external_observation_policy": "{\"current_source_mode\":\"default\",\"artifact_citation_mode\":\"external_only\",\"artifact_citation_quotes\":[\"不要把日志行当成当前源码引用\"],\"confidence\":\"0.88\"}"
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("artifact citation mode should be accepted through string-wrapped policy, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("policy should survive: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("ArtifactCitationMode=%q, want external_only", got)
	}
	if rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		t.Fatalf("artifact citation mode must not exclude current source: %+v", rm.ExternalObservationPolicy)
	}
	if !strings.Contains(res.Summary, "artifact_citation=external_only") {
		t.Fatalf("summary should report artifact citation mode, got %q", res.Summary)
	}

	mu = types.NewMutableState("结合当前代码分析外部日志，日志行号不要作为当前源码引用")
	payload = withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["log", "source"],
		"entities": ["panic"],
		"question_kind": "mechanism",
		"external_observation_policy": "{\"artifact_citation_mode\":\"external_only\",\"confidence\":\"0.77\"}"
	}`)
	res, err = (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute artifact-only policy: %v", err)
	}
	rm = mu.RequestModel()
	if rm == nil || rm.ExternalObservationPolicy == nil {
		t.Fatalf("artifact-only policy should survive: %+v", rm)
	}
	if got := rm.ExternalObservationPolicy.CurrentSourceMode; got != types.ExternalObservationCurrentSourceDefault {
		t.Fatalf("CurrentSourceMode=%q, want default", got)
	}
	if got := rm.ExternalObservationPolicy.ArtifactCitationMode; got != types.ExternalObservationArtifactCitationExternalOnly {
		t.Fatalf("ArtifactCitationMode=%q, want external_only", got)
	}
}

func TestEmitAnalysis_Execute_PersistsAnswerExclusionPolicy(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出公开 API，但不要列变量。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["api", "public"],
		"entities": ["API"],
		"question_kind": "enumeration",
		"intent_confidence": 0.93,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_exclusion_policy": {
			"is_exclusion_requested": true,
			"excluded_candidate_roles": ["variable"],
			"source_quotes": ["不要列变量"],
			"confidence": 0.94,
			"rationale": "current request excludes variable rows from the public API list"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "excluded_roles=variable") {
		t.Fatalf("summary should surface typed exclusion lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.AnswerExclusionPolicy == nil || !rm.AnswerExclusionPolicy.Active() {
		t.Fatalf("AnswerExclusionPolicy not persisted: %+v", rm)
	}
	if got := rm.AnswerExclusionPolicy.ExcludedCandidateRoles; len(got) != 1 || got[0] != types.AnswerCandidateRoleVariable {
		t.Fatalf("excluded roles wrong: %+v", got)
	}
}

func TestEmitAnalysis_Execute_RejectsAuxiliaryPrincipalExclusionConflict(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign func", "public class", "ArkTS", "iota"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"include_auxiliary_as_principal": true,
			"source_quotes": [],
			"confidence": 0.9
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "type"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.9
		},
		"answer_exclusion_policy": {
			"is_exclusion_requested": true,
			"excluded_candidate_roles": ["fixture", "example", "test", "generated"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.94,
			"rationale": "incorrectly treating positive inventory buckets as exclusions"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatalf("Execute should reject mutually exclusive auxiliary principal/exclusion policies, got %q", res.Summary)
	}
	for _, want := range []string{"source_scope_profile allows repo-owned auxiliary", "answer_exclusion_policy excludes auxiliary"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("rejection missing %q:\n%s", want, res.Summary)
		}
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Fatalf("rejected analysis must not persist contradictory request model: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_SoftensRepoWideInventoryAuxiliaryExclusion(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	repo := t.TempDir()
	for _, rel := range []string{
		"internal/tool/source_inventory_language_census.go",
		"internal/tool/repomap/index/extract_cangjie.go",
		"internal/tool/repomap/index/cangjie_parser.go",
	} {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign func", "public class"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": true,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"required_files": [
			{"path":"internal/tool/source_inventory_language_census.go","confidence":0.8,"rationale":"support implementation, not a user-named source scope"},
			{"path":"internal/tool/repomap/index/extract_cangjie.go","confidence":0.75,"rationale":"support implementation"},
			{"path":"internal/tool/repomap/index/cangjie_parser.go","confidence":0.75,"rationale":"support implementation"}
		],
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type", "function", "constant"],
			"requested_fields": ["name", "location", "package", "summary"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.9
		},
		"answer_exclusion_policy": {
			"is_exclusion_requested": true,
			"excluded_candidate_roles": ["fixture", "example", "test", "generated"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.94,
			"rationale": "incorrectly treating positive inventory buckets as exclusions"
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, RepoRoot: repo}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("repo-wide inventory should soften imprecise exclusion/support hints, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.AnswerExclusionPolicy != nil && rm.AnswerExclusionPolicy.Active() {
		t.Fatalf("target-category exclusion should be softened, got %+v", rm.AnswerExclusionPolicy)
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source_inventory_profile should remain active: %+v", rm.SourceInventoryProfile)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 0 {
		t.Fatalf("model-authored source-inventory support files must stay soft, got required_file hints: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if !strings.Contains(res.Summary, "required_files=[]") {
		t.Fatalf("summary should expose normalized empty required_files lane, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "model-authored source-inventory path hint") {
		t.Fatalf("summary should disclose source-inventory required_files softening, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RepairsInvalidInventoryRolesWithoutNarrowingToGuessedFile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	rel := "internal/tool/repomap/index/parser.go"
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := "仓库里有哪些扩展块、外部声明、公开类型？分别列出文件路径、符号名和包路径。"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["扩展块", "外部声明", "公开类型"],
		"entities": ["扩展块", "外部声明", "公开类型"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"required_files": [
			{"path":"internal/tool/repomap/index/parser.go","confidence":0.9,"rationale":"model guessed parser implementation"}
		],
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["extension_surface", "external_surface", "visibility_surface"],
			"requested_fields": ["name", "location", "package"],
			"source_quotes": ["扩展块", "外部声明", "公开类型", "not copied from request"],
			"confidence": 0.95
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, RepoRoot: repo},
		json.RawMessage(withRequiredAnswerRoleProfile(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("invalid optional roles should degrade locally, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("typed enumeration should repair an active inventory profile: %+v", rm)
	}
	for _, role := range []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
	} {
		if !rm.SourceInventoryProfile.RequiresPrincipalRole(role) {
			t.Fatalf("repaired profile missing default structural role %s: %+v", role, rm.SourceInventoryProfile)
		}
	}
	if got, want := rm.SourceInventoryProfile.RequestedFields, []types.SourceInventoryRequestedField{
		types.SourceInventoryFieldName,
		types.SourceInventoryFieldLocation,
		types.SourceInventoryFieldPackage,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("independently valid requested fields were lost: got %+v want %+v", got, want)
	}
	if got, want := rm.SourceInventoryProfile.SourceQuotes, []string{"扩展块", "外部声明", "公开类型"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source quote repair = %+v, want %+v", got, want)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 0 {
		t.Fatalf("invalid role list let a guessed parser file narrow inventory scope: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	for _, want := range []string{
		"target_roles omitted or empty",
		"synthesized source_inventory_profile",
		"model-authored source-inventory path hint",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing local-degradation disclosure %q: %s", want, res.Summary)
		}
	}
}

func TestEmitAnalysis_Execute_ExplicitNonInventoryDoesNotSoftenRequiredFile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	rel := "internal/tool/repomap/index/parser.go"
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package index\nfunc Parse() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("解释解析流程。")
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["解析", "流程"],
		"entities": ["解析流程"],
		"question_kind": "mechanism",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"required_files": [
			{"path":"internal/tool/repomap/index/parser.go","confidence":0.9,"rationale":"implementation entry"}
		],
		"source_inventory_profile": {
			"is_source_inventory": false,
			"target_roles": ["not_a_role"],
			"confidence": 0.9
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, RepoRoot: repo},
		json.RawMessage(withRequiredAnswerRoleProfile(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.AnalyzerHints.RequiredFileHints) != 1 ||
		rm.AnalyzerHints.RequiredFileHints[0].Path != rel {
		t.Fatalf("explicit non-inventory request lost ordinary required file: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_KeepsExplicitSourceInventoryRequiredFile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	repo := t.TempDir()
	rel := "internal/tool/source_inventory_language_census.go"
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package tool\nfunc Census() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("只列 internal/tool/source_inventory_language_census.go 里的函数。")
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["source_inventory_language_census.go", "function"],
		"entities": ["internal/tool/source_inventory_language_census.go", "function"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.75,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"required_files": [
			{"path":"internal/tool/source_inventory_language_census.go","confidence":0.91,"rationale":"user named exact source file"}
		],
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location", "summary"],
			"source_quotes": ["函数"],
			"confidence": 0.9
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, RepoRoot: repo}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("explicit source-inventory file should be preserved, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.AnalyzerHints.RequiredFileHints) != 1 {
		t.Fatalf("explicit required file hint not persisted: rm=%+v summary=%q", rm, res.Summary)
	}
	if got := rm.AnalyzerHints.RequiredFileHints[0].Path; got != rel {
		t.Fatalf("required_file path = %q, want %q", got, rel)
	}
	if strings.Contains(res.Summary, "model-authored source-inventory path hint") {
		t.Fatalf("explicit path must not be softened: %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsAnswerVisibilityProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出公开符号。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["symbol", "public"],
		"entities": ["API"],
		"question_kind": "enumeration",
		"intent_confidence": 0.93,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_visibility_profile": {
			"symbol_visibility": "public_exported",
			"source_quotes": ["公开符号"],
			"confidence": 0.94,
			"rationale": "current request asks for public symbols"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "symbol_visibility=public_exported") {
		t.Fatalf("summary should surface symbol visibility lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.AnswerVisibilityProfile == nil || !rm.AnswerVisibilityProfile.ExcludesPrivateSymbols() {
		t.Fatalf("AnswerVisibilityProfile not persisted: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_PersistsSourceInventoryProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出 internal/types 包里所有公开的 type X string 加 const 集合的枚举类型名。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["symbol", "enum", "type", "string"],
		"entities": ["internal/types"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type"],
			"type_underlying": "string",
			"requires_const_set": true,
			"requested_fields": ["name", "location", "count"],
			"source_quotes": ["公开的 type X string"],
			"confidence": 0.95,
			"rationale": "current request asks for public string enum type names"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "source_inventory=type") ||
		!strings.Contains(res.Summary, "inventory_underlying=string") ||
		!strings.Contains(res.Summary, "inventory_const_set=true") {
		t.Fatalf("summary should surface source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("SourceInventoryProfile not persisted: %+v", rm)
	}
	if !rm.SourceInventoryProfile.RequiresRole(types.AnswerCandidateRoleType) ||
		rm.SourceInventoryProfile.TypeUnderlying != types.SourceInventoryTypeUnderlyingString ||
		!rm.SourceInventoryProfile.RequiresConstSet {
		t.Fatalf("SourceInventoryProfile wrong: %+v", rm.SourceInventoryProfile)
	}
}

func TestEmitAnalysis_Execute_SynthesizesInventoryFromSourceScopeEnumeration(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	for _, rel := range []string{
		"internal/tool/builtin.go",
		"internal/skill/defaults.go",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte("package sample\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mu := types.NewMutableState("仓库里有哪些 @Entry 标记的 ArkTS 页面入口，以及哪些 @Builder 复用片段？分别列出文件路径和函数名。")
	mu.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 2 matching files]",
			"[grep params: pattern=.ets include=* files_only=true]",
			"./internal/tool/builtin.go",
			"./internal/skill/defaults.go",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			Include:        "*",
			FilesOnly:      true,
			ResultCount:    2,
			CandidateFiles: []string{"./internal/tool/builtin.go", "./internal/skill/defaults.go"},
		},
	})
	tool := &EmitAnalysis{}
	payload := withRequiredAnswerRoleProfile(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["ArkTS", "@Entry", "@Builder", ".ets"],
		"entities": [],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.95,
		"kind_confidence": 0.95,
		"predicate_axis": "define",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"include_auxiliary_as_principal": false,
			"confidence": 0.95,
			"rationale": "source scoped enumeration"
		}
	}`)

	res, err := tool.Execute(&types.BusContext{RepoRoot: repo, Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("summary should expose synthesized source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source-scope enumeration should synthesize source inventory profile: %+v", rm)
	}
	if got := len(rm.AnalyzerHints.RequiredFileHints); got != 0 {
		t.Fatalf("low-confidence synthesized inventory must not project noisy prescan required files, got %d %+v", got, rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestEmitAnalysis_SynthesizedInventoryPreservesValidatedSourceQuotesFromInvalidProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？")
	tool := &EmitAnalysis{}
	payload := withRequiredAnswerRoleProfile(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["extend", "foreign func", "public class"],
		"entities": [],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.95,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"requested_fields": ["name", "location"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class", "not in request"],
			"confidence": 0.95
		}
	}`)

	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("typed enumeration should synthesize source inventory profile: %+v", rm)
	}
	got := strings.Join(rm.SourceInventoryProfile.SourceQuotes, "|")
	for _, want := range []string{"extend 块", "foreign func 声明", "public class"} {
		if !strings.Contains(got, want) {
			t.Fatalf("synthesized source inventory lost validated source quote %q: %+v", want, rm.SourceInventoryProfile.SourceQuotes)
		}
	}
	if strings.Contains(got, "not in request") {
		t.Fatalf("synthesized source inventory kept unvalidated source quote: %+v", rm.SourceInventoryProfile.SourceQuotes)
	}
}

func TestEmitAnalysis_EnrichesSynthesizedInventoryFromAnalyzerPrescan(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。")
	prescan := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"."},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
			"repo_lens:candidate_budget_truncated",
		},
		Execution: &types.SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []types.SourceInventoryObservationSet{
			{
				Role: types.AnswerCandidateRoleType,
				Members: []types.SourceInventoryObservationMember{
					{Name: "Cart", Role: types.AnswerCandidateRoleType, File: "demo/cart/Cart.cj", Line: 14, SurfaceTerms: []string{"public class", "public class Cart"}},
					{Name: "extend Cart", Role: types.AnswerCandidateRoleType, File: "demo/cart/Cart.cj", Line: 30, SurfaceTerms: []string{"extend", "extend Cart"}},
				},
			},
			{
				Role: types.AnswerCandidateRoleFunction,
				Members: []types.SourceInventoryObservationMember{
					{Name: "native_add", Role: types.AnswerCandidateRoleFunction, File: "demo/bridge/Bridge.cj", Line: 6, SurfaceTerms: []string{"foreign func", "foreign func native_add"}},
				},
			},
			{
				Role: types.AnswerCandidateRolePackage,
				Members: []types.SourceInventoryObservationMember{
					{Name: "demo.cart", Role: types.AnswerCandidateRolePackage, File: "demo/cart/Cart.cj", Line: 4, SurfaceTerms: []string{"package demo.cart"}},
				},
			},
			{
				Role: types.AnswerCandidateRoleField,
				Members: []types.SourceInventoryObservationMember{
					{Name: "count", Role: types.AnswerCandidateRoleField, File: "demo/cart/Cart.cj", Line: 16, SurfaceTerms: []string{"field count"}},
				},
			},
			{
				Role: types.AnswerCandidateRoleVariable,
				Members: []types.SourceInventoryObservationMember{
					{Name: "temp", Role: types.AnswerCandidateRoleVariable, File: "demo/cart/Cart.cj", Line: 21, SurfaceTerms: []string{"variable temp"}},
				},
			},
			{
				Role: types.AnswerCandidateRoleConstant,
				Members: []types.SourceInventoryObservationMember{
					{Name: "VERSION", Role: types.AnswerCandidateRoleConstant, File: "demo/cart/Cart.cj", Line: 8, SurfaceTerms: []string{"constant VERSION"}},
				},
			},
		},
	}
	mu.AppendDispatchToolResult(types.ToolResult{
		ToolName:        "repo_map",
		Success:         true,
		SourceInventory: &prescan,
		Refinement: &types.ToolRefinementHint{
			ReasonCode:        "source_inventory_candidate_budget_truncated",
			PreferredNextTool: "repo_map",
			PreferredParams: map[string]string{
				"view":  "source_inventory",
				"query": "extend foreign func public class",
			},
		},
	})
	payload := withRequiredAnswerRoleProfile(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign", "foreign func", "public", "class", "package"],
		"entities": ["extend", "foreign func", "public class", "package 声明"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.80,
		"kind_confidence": 0.95,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		}
	}`)

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("prescan should synthesize source_inventory_profile: %+v", rm)
	}
	for _, want := range []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction} {
		if !rm.SourceInventoryProfile.RequiresRole(want) {
			t.Fatalf("source_inventory_profile roles = %+v, want %s", rm.SourceInventoryProfile.TargetRoles, want)
		}
	}
	for _, notWant := range []types.AnswerCandidateRole{
		types.AnswerCandidateRolePackage,
		types.AnswerCandidateRoleField,
		types.AnswerCandidateRoleVariable,
		types.AnswerCandidateRoleConstant,
	} {
		if rm.SourceInventoryProfile.RequiresRole(notWant) {
			t.Fatalf("advisory prescan observation role %s must not widen synthesized principal roles: %+v", notWant, rm.SourceInventoryProfile.TargetRoles)
		}
	}
	gotQuotes := strings.Join(rm.SourceInventoryProfile.SourceQuotes, "|")
	for _, want := range []string{"extend", "foreign func", "public class"} {
		if !strings.Contains(gotQuotes, want) {
			t.Fatalf("source_inventory_profile quotes = %+v, want %q", rm.SourceInventoryProfile.SourceQuotes, want)
		}
	}
	if strings.Contains(gotQuotes, "public class Cart") || strings.Contains(gotQuotes, "foreign func native_add") {
		t.Fatalf("row-specific surface terms not present in the current request must not become hard profile quotes: %+v", rm.SourceInventoryProfile.SourceQuotes)
	}
	if !strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("summary should expose enriched inventory lane, got %q", res.Summary)
	}
}

func TestEnrichSourceInventoryProfileFromAnalyzerPrescan_PreservesNonEmptyRationale(t *testing.T) {
	raw := "列出 extend 和 foreign func 声明"
	mu := types.NewMutableState(raw)
	prescan := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Scopes:       []string{"."},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
		Sets: []types.SourceInventoryObservationSet{
			{
				Role: types.AnswerCandidateRoleFunction,
				Members: []types.SourceInventoryObservationMember{
					{Name: "native_add", Role: types.AnswerCandidateRoleFunction, File: "demo/bridge/Bridge.cj", Line: 6, SurfaceTerms: []string{"foreign func"}},
				},
			},
			{
				Role: types.AnswerCandidateRoleType,
				Members: []types.SourceInventoryObservationMember{
					{Name: "extend Cart", Role: types.AnswerCandidateRoleType, File: "demo/cart/Cart.cj", Line: 30, SurfaceTerms: []string{"extend"}},
				},
			},
		},
	}
	mu.AppendDispatchToolResult(types.ToolResult{
		ToolName:        "repo_map",
		Success:         true,
		SourceInventory: &prescan,
	})
	const rationale = "model-authored rationale stays audit-only"
	rm := types.RequestModel{
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
			RequestedFields: []types.SourceInventoryRequestedField{
				types.SourceInventoryFieldName,
				types.SourceInventoryFieldLocation,
			},
			Confidence: 0.10,
			Rationale:  rationale,
		},
	}

	warning := enrichSourceInventoryProfileFromAnalyzerPrescan(&types.BusContext{Mutable: mu}, &rm, raw)
	if warning == "" {
		t.Fatal("prescan enrichment should report a typed profile update")
	}
	if rm.SourceInventoryProfile.Rationale != rationale {
		t.Fatalf("non-empty rationale must stay audit-only, got %q want %q", rm.SourceInventoryProfile.Rationale, rationale)
	}
	if rm.SourceInventoryProfile.Confidence < 0.55 {
		t.Fatalf("confidence was not upgraded from typed prescan observation: %+v", rm.SourceInventoryProfile)
	}
	gotQuotes := strings.Join(rm.SourceInventoryProfile.SourceQuotes, "|")
	for _, want := range []string{"extend", "foreign func"} {
		if !strings.Contains(gotQuotes, want) {
			t.Fatalf("source_inventory_profile quotes = %+v, want %q", rm.SourceInventoryProfile.SourceQuotes, want)
		}
	}
}

func TestMergeSourceInventoryAnalyzerPrescanRequestedPathScopes_RequiresMentionedAnalyzerScope(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{MentionedEntities: []string{"internal/analysis/criterion"}},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		},
	}
	observation := types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"internal/analysis/criterion"},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:    types.AnswerCandidateRoleType,
			Members: []types.SourceInventoryObservationMember{{Name: "Kind", File: "internal/analysis/criterion/grammar.go"}},
		}},
	}
	if !mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&rm, observation, "internal/analysis/criterion 请列出公开类型") {
		t.Fatal("matching current-request entity plus analyzer-stage lens should mint requested path scope")
	}
	if got := rm.AnalyzerHints.SourceInventoryRequestedPathScopes; len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("requested path scopes = %#v", got)
	}

	unmatched := rm
	unmatched.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
	unmatched.AnalyzerHints.MentionedEntities = []string{"internal/types"}
	if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&unmatched, observation, "internal/types 请列出公开类型") {
		t.Fatalf("unmatched exploration scope must not become request authority: %#v", unmatched.AnalyzerHints.SourceInventoryRequestedPathScopes)
	}

	wrongStage := rm
	wrongStage.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
	observation.Provenance = []string{
		types.SourceInventoryProvenanceRepoLensToolQuery,
		types.SourceInventoryProvenanceStageExplore,
	}
	if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&wrongStage, observation, "internal/analysis/criterion 请列出公开类型") {
		t.Fatalf("exploration cursor must not become request authority: %#v", wrongStage.AnalyzerHints.SourceInventoryRequestedPathScopes)
	}

	root := rm
	root.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
	root.AnalyzerHints.MentionedEntities = []string{"."}
	observation.Scopes = []string{"."}
	observation.Provenance = []string{
		types.SourceInventoryProvenanceRepoLensToolQuery,
		types.SourceInventoryProvenanceStageAnalyze,
	}
	if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&root, observation, ". 请列出公开类型") {
		t.Fatalf("root navigation must remain repo-wide, got %#v", root.AnalyzerHints.SourceInventoryRequestedPathScopes)
	}
}

func TestMergeSourceInventoryAnalyzerPrescanRequestedPathScopes_AcceptsVerifiedSourceScopeQuote(t *testing.T) {
	rm := types.RequestModel{
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		},
		SourceScopeProfile: &types.SourceScopeProfile{
			RequestedScope: types.SourceScopeProduction,
			SourceQuotes:   []string{"internal/analysis/criterion"},
		},
	}
	observation := types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"internal/analysis/criterion"},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
	}
	if !mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&rm, observation, "列出 internal/analysis/criterion 的公开类型") {
		t.Fatal("verified source-scope quote plus matching analyzer lens should mint requested path scope")
	}
	if got := rm.AnalyzerHints.SourceInventoryRequestedPathScopes; len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("requested path scopes = %#v", got)
	}

	rm.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
	rm.SourceScopeProfile.SourceQuotes = []string{"internal/types"}
	if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&rm, observation, "列出 internal/types 的公开类型") {
		t.Fatalf("unmatched source-scope quote must not authorize analyzer lens: %#v", rm.AnalyzerHints.SourceInventoryRequestedPathScopes)
	}
}

func TestMergeSourceInventoryAnalyzerPrescanRequestedPathScopes_AcceptsExactRequestPathWithoutOptionalAnalyzerCarrier(t *testing.T) {
	rm := types.RequestModel{
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		},
	}
	observation := types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"internal/analysis/criterion"},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
	}
	if !mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(
		&rm,
		observation,
		"internal/analysis/criterion 请列出公开符号",
	) {
		t.Fatal("exact request path plus matching analyzer-stage lens should mint requested path scope")
	}
	if got := rm.AnalyzerHints.SourceInventoryRequestedPathScopes; len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("requested path scopes = %#v", got)
	}

	for _, raw := range []string{
		"internal/analysis/criterion_extra 请列出公开符号",
		"prefixinternal/analysis/criterion 请列出公开符号",
		"internal/analysis/criterion.go 请列出公开符号",
	} {
		clone := rm
		clone.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
		if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&clone, observation, raw) {
			t.Fatalf("path collision %q must not mint requested path scope: %#v", raw, clone.AnalyzerHints.SourceInventoryRequestedPathScopes)
		}
	}
}

func TestMergeSourceInventoryAnalyzerPrescanRequestedPathScopes_UsesRepoRootQueryCoordinate(t *testing.T) {
	rm := types.RequestModel{SourceInventoryProfile: &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
	}}
	observation := types.SourceInventoryObservation{
		Active:          true,
		Scopes:          []string{"."},
		QueryPathScopes: []string{"internal/analysis/criterion"},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:    types.AnswerCandidateRoleType,
			Members: []types.SourceInventoryObservationMember{{Name: "Kind"}},
		}},
	}
	if !mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(
		&rm,
		observation,
		"internal/analysis/criterion 请列出公开符号",
	) {
		t.Fatal("repo-root query coordinate should survive selected-sub-repo operational scope normalization")
	}
	if got := rm.AnalyzerHints.SourceInventoryRequestedPathScopes; len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("requested path scopes = %#v", got)
	}

	wrongRequest := rm
	wrongRequest.AnalyzerHints.SourceInventoryRequestedPathScopes = nil
	if mergeSourceInventoryAnalyzerPrescanRequestedPathScopes(&wrongRequest, observation, "internal/types 请列出公开符号") {
		t.Fatalf("query provenance without exact current-request identity must remain unauthorized: %#v", wrongRequest.AnalyzerHints.SourceInventoryRequestedPathScopes)
	}
}

func TestEmitAnalysis_PersistsRequestBoundAnalyzerPrescanPathScope(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	raw := "internal/analysis/criterion 请列出公开类型和函数"
	mu := types.NewMutableState(raw)
	prescan := types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     false,
		Scopes:       []string{"internal/analysis/criterion"},
		Provenance: []string{
			types.SourceInventoryProvenanceRepoLensToolQuery,
			types.SourceInventoryProvenanceStageAnalyze,
		},
		Execution: &types.SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []types.SourceInventoryObservationSet{{
			Role: types.AnswerCandidateRoleFunction,
			Members: []types.SourceInventoryObservationMember{{
				Name: "Eval", Role: types.AnswerCandidateRoleFunction, File: "internal/analysis/criterion/eval.go",
			}},
		}, {
			Role: types.AnswerCandidateRoleType,
			Members: []types.SourceInventoryObservationMember{{
				Name: "Kind", Role: types.AnswerCandidateRoleType, File: "internal/analysis/criterion/grammar.go",
			}},
		}},
	}
	mu.AppendDispatchToolResult(types.ToolResult{ToolName: "repo_map", Success: true, SourceInventory: &prescan})
	payload := withRequiredAnswerRoleProfile(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["criterion", "type", "function"],
		"entities": ["Kind", "Eval"],
		"question_kind": "enumeration",
		"intent_confidence": 0.95,
		"complexity_confidence": 0.90,
		"kind_confidence": 0.95,
		"predicate_axis": "define",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type", "function"],
			"requested_fields": ["name", "location", "count"],
			"source_quotes": ["公开类型和函数"],
			"confidence": 0.95
		}
	}`)
	result, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil || !result.Success {
		t.Fatalf("emit_analysis failed: err=%v result=%+v", err, result)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("request model missing")
	}
	if got := types.SourceInventoryRequestedPathScopes(*rm); len(got) != 1 || got[0] != "internal/analysis/criterion" {
		t.Fatalf("persisted requested path scopes = %#v", got)
	}
	if types.SourceInventoryRequiresRepoWideLens(*rm) {
		t.Fatal("persisted analyzer-prescan path boundary must prevent repo-wide expansion")
	}
}

func TestEmitAnalysis_ProjectsPrescanFilesForSourceInventoryCoverage(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	for _, rel := range []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_functions.ets",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte("@Entry\n@Component\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mu := types.NewMutableState("列出 ArkTS corpus 里的页面入口和 builder。")
	mu.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: "[grep: 5 matching files]\n[grep params: pattern=@Entry|@Builder path=. include=*.ets files_only=true]\n" +
			"[prescan production matches]\n" +
			"no non-auxiliary matches found\n" +
			"[prescan auxiliary matches - not production proof]\n" +
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets\n" +
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets\n" +
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets\n" +
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets\n" +
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_functions.ets\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Path:        ".",
			Include:     "*.ets",
			FilesOnly:   true,
			ResultCount: 5,
			CandidateFiles: []string{
				"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				"internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets",
				"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
				"internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets",
				"internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_functions.ets",
			},
		},
	})
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["ArkTS", "Entry", "Builder", "corpus"],
		"entities": ["ArkTS corpus", "@Entry", "@Builder"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"source_quotes": ["corpus"],
			"confidence": 0.9,
			"rationale": "the request asks about corpus files"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["页面入口和 builder"],
			"confidence": 0.95,
			"rationale": "current request asks for a source inventory"
		}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		RepoRoot: repo,
		Mutable:  mu,
	}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel missing")
	}
	if got := len(rm.AnalyzerHints.RequiredFileHints); got != 5 {
		t.Fatalf("projected required file hints=%d, want 5: %+v", got, rm.AnalyzerHints.RequiredFileHints)
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			t.Fatalf("projected hint should be hard-coverage confidence, got %+v", hint)
		}
		if !strings.HasSuffix(hint.Path, ".ets") || !strings.Contains(hint.Path, "tree-sitter-arkts/corpus/sources/") {
			t.Fatalf("unexpected projected path: %+v", hint)
		}
	}
	if !types.SourceInventoryRequiredFileCoverageShape(*rm) {
		t.Fatalf("projected source inventory hints should preserve source-inventory shape: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if !types.SourceInventoryRequiresRepoWideLens(*rm) {
		t.Fatalf("scope=all source inventory should run the root lens before hard prescan coverage")
	}
	if types.RequiredFileHintCurrentSourceCoverageApplies(*rm) {
		t.Fatalf("repo-wide projected source inventory hints are soft navigation hints, not hard required-file coverage: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestEmitAnalysis_DoesNotProjectDocumentationPrescanFilesForCodeConstructInventory(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	for _, rel := range []string{"AGENTS.md", "README.md", "docs/architecture.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte("source inventory docs\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mu := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？")
	mu.AppendDispatchToolResult(types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindListFiles,
			Path:        ".",
			Recursive:   true,
			ResultCount: 3,
			CandidateFiles: []string{
				"AGENTS.md",
				"README.md",
				"docs/architecture.md",
			},
		},
	})
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["extend", "foreign", "public", "class"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"confidence": 0.9,
			"rationale": "the request asks about source declarations"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type", "function"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.95,
			"rationale": "current request asks for a source declaration inventory"
		}
	}`

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		RepoRoot: repo,
		Mutable:  mu,
	}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel missing")
	}
	if got := rm.AnalyzerHints.RequiredFileHints; len(got) != 0 {
		t.Fatalf("documentation prescan files must remain navigation context, not source-inventory coverage hints: %+v", got)
	}
	if types.RequiredFileHintCurrentSourceCoverageApplies(*rm) {
		t.Fatalf("documentation-only prescan must not activate source-inventory required-file coverage: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if strings.Contains(res.Summary, "required_files: projected") {
		t.Fatalf("summary should not claim projected required files for documentation-only prescan: %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForObservationOnlyRuntime(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("请只基于这段 systrace 文本分析 inode/dev/entry_name，不要分析当前仓库代码。")
	mu.SetPerfTrace(&types.PerfBundle{
		Observations: []types.PerfObservation{{
			Kind:      "file_io",
			Subject:   "inode=0xb9b8e",
			Summary:   "entry_name=foo.db dev=260:136 bytes=4096",
			LineStart: 4,
			LineEnd:   5,
		}},
	})
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "performance_bottleneck",
		"complexity": "moderate",
		"keywords": ["inode", "dev", "entry_name", "file_io"],
		"entities": ["inode 0xb9b8e", "dev 260:136", "foo.db"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"artifact_citation_mode": "external_only",
			"current_source_exclusion_quote": "不要分析当前仓库代码",
			"artifact_citation_quotes": ["请只基于这段 systrace 文本"],
			"confidence": 0.95
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["inode", "device", "file"],
			"requested_fields": ["name", "location", "summary", "values"],
			"source_quotes": ["inode/dev/entry_name"],
			"confidence": 0.9,
			"rationale": "runtime artifact IO identifiers, not current source"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("observation-only runtime summary must not advertise source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		t.Fatalf("observation-only runtime must drop source inventory profile: %+v", rm.SourceInventoryProfile)
	}
	if got := rm.CurrentSourceLaneDecision(); got != types.CurrentSourceLaneExcluded {
		t.Fatalf("CurrentSourceLaneDecision=%s, want excluded", got)
	}
	if contract := types.BuildExactResolutionContract(*rm); contract != nil {
		t.Fatalf("observation-only runtime must not build exact source contract: %+v", contract)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForTypedRelation(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("请用类型关系图表示 LoopController 接口和主要实现类型，并列出所在文件。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["LoopController", "interface", "implementer"],
		"entities": ["LoopController"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicate_axis": "implement",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": true,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["LoopController 接口和主要实现类型"],
			"confidence": 0.95,
			"rationale": "relation implementer inventory"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("typed relation summary must not advertise source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		t.Fatalf("typed relation request must drop source inventory profile: %+v", rm.SourceInventoryProfile)
	}
	if !types.HasTypedRelationMemberSetShape(*rm) {
		t.Fatalf("typed relation shape should remain active: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_SoftensAnswerRoleForPerMemberRelation(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Handler 接口有哪些实现类？每个实现各自负责哪个路径？")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["Handler", "implement", "route", "path"],
		"entities": ["EchoHandler", "StatsHandler", "UpperHandler"],
		"question_kind": "enumeration",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"predicate_axis": "implement",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": true,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.9
		},
		"answer_subject": {
			"kind": "type_name",
			"entity_axes": ["handler → route"],
			"confidence": 0.9
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["function", "route"],
			"source_quotes": ["每个实现各自负责哪个路径"],
			"confidence": 0.9
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "answer_role_profile auto-softened: typed per-member relation rows") {
		t.Fatalf("summary should disclose typed role-profile softening, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.AnswerRoleProfile != nil {
		t.Fatalf("per-member relation attribute roles must not become candidate_role obligations: %+v", rm.AnswerRoleProfile)
	}
	if !types.HasTypedRelationMemberSetShape(*rm) || !rm.Predicates.HasPerMemberTable {
		t.Fatalf("relation/member-table shape must survive role-profile softening: %+v", rm)
	}
}

func TestSoftenAnswerRoleProfileForPerMemberRelation_PreservesRoleLocate(t *testing.T) {
	rm := &types.RequestModel{
		PredicateAxis: types.AxisImplement,
		Predicates: types.SemanticPredicates{
			IsRoleLocateLookup: true,
			IsRelationalLookup: true,
			HasPerMemberTable:  true,
		},
		AnswerRoleProfile: &types.AnswerRoleProfile{
			IsRoleBindingRequested: true,
			RequiredCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleRoute,
			},
		},
	}
	if softened, warning := softenAnswerRoleProfileForPerMemberRelation(rm); softened || warning != "" {
		t.Fatalf("explicit typed role-locate must remain active: softened=%t warning=%q", softened, warning)
	}
	if rm.AnswerRoleProfile == nil || !rm.AnswerRoleProfile.Active() {
		t.Fatalf("explicit typed role-locate profile was lost: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForRegistryBindingMemberSet(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("codrax 默认注册到 SubAgentRegistry 的 subagent 名称有哪些？请给出总数、完整成员名，以及注册和 Name() 返回值的证据。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["SubAgentRegistry", "Register", "subagent", "Name"],
		"entities": ["SubAgentRegistry", "codrax"],
		"question_kind": "enumeration",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.75,
		"kind_confidence": 0.9,
		"predicate_axis": "register",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.9
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["codrax 默认注册到 SubAgentRegistry 的 subagent 名称有哪些？"],
			"confidence": 0.85,
			"rationale": "registry member set, not source-code inventory"
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"required_candidate_roles": [],
			"source_quotes": ["codrax 默认注册到 SubAgentRegistry 的 subagent 名称有哪些？"],
			"confidence": 0.9
		},
		"completeness_obligation": {
			"required": true,
			"source_quote": "完整成员名"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("registry/binding member-set summary must not advertise source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		t.Fatalf("registry/binding member set must not persist source_inventory_profile: %+v", rm.SourceInventoryProfile)
	}
	if !types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) {
		t.Fatalf("registry/binding answer still needs structured member_set handoff: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_KeepsPreciseSourceInventoryWhenAnswerRoleProfileDrifts(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("仓库里有哪些声明？分别列出文件路径和符号名。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["declaration", "source inventory", "path", "symbol"],
		"entities": ["declaration"],
		"question_kind": "enumeration",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.75,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"confidence": 0.9
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type", "function"],
			"requested_fields": ["name", "location", "package"],
			"source_quotes": ["declarations"],
			"confidence": 0.9,
			"rationale": "bounded declaration inventory"
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["type", "function"],
			"source_quotes": ["declarations"],
			"confidence": 0.85
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("precise source inventory declaration lane should be retained despite noisy answer_role_profile: %+v", rm)
	}
	if !types.SourceInventoryPrincipalNavigationActive(*rm) {
		t.Fatalf("retained source inventory should own principal navigation: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForRelationFlow(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("分析 io_uring send recv opcode 到 socket 层的调用链，并给出关键函数和逻辑图。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "complex",
		"keywords": ["io_uring", "send", "recv", "call chain"],
		"entities": ["IORING_OP_SEND", "IORING_OP_RECV", "io_uring", "socket"],
		"question_kind": "call_chain",
		"exact_targets": ["io_uring", "socket"],
		"intent_confidence": 0.94,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.92,
		"predicate_axis": "call",
		"call_chain_endpoints": {"source":"io_uring", "sink":"socket"},
		"diagram_hint": {"kind": "call_dag", "required": true, "relation_scope_quote":"给出关键函数和逻辑图", "participants": []},
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": true,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "struct_field", "not_a_role"],
			"requested_fields": ["name", "location", "summary"],
			"source_quotes": ["关键函数"],
			"confidence": 0.83,
			"rationale": "flow answer will mention key functions"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("relation-flow summary must not advertise source inventory lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() {
		t.Fatalf("relation-flow request must drop source inventory profile: %+v", rm.SourceInventoryProfile)
	}
	if !types.SourceInventoryProfileConflictsWithRelationFlow(types.RequestModel{
		Intent:                 types.IntentTrace,
		PredicateAxis:          types.AxisCall,
		AnalyzerHints:          types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}},
	}) {
		t.Fatal("relation-flow typed helper should remain active for the same shape")
	}
}

func TestDropSourceInventoryProfileForArchitectureNarrative(t *testing.T) {
	rm := types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioArchitectureExplain,
		PredicateAxis: types.AxisDefine,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			HasPerMemberTable:     true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind:     string(types.ReqMechanism),
			Entities: []string{"PipelineStage", "StageBinding", "Orchestrator"},
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
		SubTopics: []types.SubTopic{
			{Summary: "stage membership"},
			{Summary: "stage responsibilities"},
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleConstant},
			RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldSummary},
			Confidence:        0.88,
		},
	}
	dropped, warning := dropSourceInventoryProfileForTypedRelation(&rm)
	if !dropped || rm.SourceInventoryProfile != nil {
		t.Fatalf("architecture narrative source inventory was not dropped: dropped=%v profile=%+v", dropped, rm.SourceInventoryProfile)
	}
	if !strings.Contains(warning, "architecture/mechanism narrative") {
		t.Fatalf("warning=%q, want typed architecture rationale", warning)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForArchitectureNarrative(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","stage","architecture"],
		"entities":["PipelineStage","StageBinding","Orchestrator"],
		"question_kind":"mechanism",
		"intent_confidence":0.95,
		"complexity_confidence":0.85,
		"kind_confidence":0.9,
		"predicate_axis":"define",
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":true,
			"is_history_lookup":false,
			"is_diagnostic_question":false,
			"has_per_member_table":true
		},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"diagram_hint":{"kind":"architecture","required":false,"participants":[]},
		"sub_topics":[{"summary":"stage membership"},{"summary":"stage responsibilities"}],
		"source_inventory_profile":{
			"is_source_inventory":true,
			"target_roles":["type","constant"],
			"requested_fields":["name","summary","location"],
			"source_quotes":["pipeline stages"],
			"confidence":0.88
		}
	}`
	mu := types.NewMutableState("describe the pipeline stages and architecture")
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("architecture analysis should succeed after deterministic demotion: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("incidental architecture source inventory survived emit path: %+v", rm)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("normalized summary must not advertise a principal source inventory lane: %s", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForRequiredWorkflowDimension(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent":"explain",
		"scenario":"generic",
		"complexity":"moderate",
		"keywords":["pipeline","stage","responsibility"],
		"entities":["PipelineStage","StageBinding"],
		"question_kind":"mechanism",
		"intent_confidence":0.95,
		"complexity_confidence":0.85,
		"kind_confidence":0.9,
		"predicate_axis":"define",
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":true,
			"is_history_lookup":false,
			"is_diagnostic_question":false,
			"has_per_member_table":true
		},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"dimensions":[
				{"label":"名称","source_quote":"名称","role":"other","required":true,"index":1},
				{"label":"职责","source_quote":"职责","role":"function_or_purpose","required":true,"index":2},
				{"label":"阶段和流转","source_quote":"阶段和流转","role":"stage_or_workflow","required":true,"index":3}
			],
			"confidence":0.92
		},
		"completeness_obligation":{"required":true,"source_quote":"哪几个 stage"},
		"source_inventory_profile":{
			"is_source_inventory":true,
			"target_roles":["type","constant"],
			"requires_const_set":true,
			"requested_fields":["name","summary","location"],
			"source_quotes":["哪几个 stage"],
			"confidence":0.9
		}
	}`
	rawRequest := "请说明 pipeline 哪几个 stage，并列出名称、职责、阶段和流转。"
	mu := types.NewMutableState(rawRequest)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("workflow analysis should succeed after deterministic demotion: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("incidental workflow source inventory survived emit path: %+v", rm)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("normalized summary must not advertise a principal source inventory lane: %s", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForArchitectureMemberExplanation(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	repo := t.TempDir()
	path := filepath.Join(repo, "internal/types/enums.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package types\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := `{
		"intent":"enumerate",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","stage","responsibility"],
		"entities":["PipelineStage","StageBinding"],
		"question_kind":"enumeration",
		"intent_confidence":0.95,
		"complexity_confidence":0.85,
		"kind_confidence":0.9,
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":true,
			"is_history_lookup":false,
			"is_diagnostic_question":false,
			"has_per_member_table":true
		},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_subject":{"kind":"enum_value","entity_axes":["PipelineStage → stage_member"],"confidence":0.8},
		"required_files":[{"path":"internal/types/enums.go","confidence":0.9,"rationale":"stage definitions"}],
		"source_inventory_profile":{
			"is_source_inventory":true,
			"target_roles":["constant"],
			"requested_fields":["name","location","summary"],
			"source_quotes":["read-mode pipeline 由哪几个 stage 组成","每个 stage 大致负责什么"],
			"confidence":0.88
		}
	}`
	rawRequest := "codrax 的 read-mode pipeline 由哪几个 stage 组成？每个 stage 大致负责什么？描述整体架构。"
	mu := types.NewMutableState(rawRequest)
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, RepoRoot: repo},
		json.RawMessage(withRequiredAnswerRoleProfile(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("architecture member analysis should succeed after deterministic demotion: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("incidental architecture-member source inventory survived emit path: %+v", rm)
	}
	hints := rm.AnalyzerHints.RequiredFileHints
	if len(hints) != 1 || hints[0].Path != "internal/types/enums.go" {
		t.Fatalf("supporting required file should survive after principal source inventory is dropped: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("normalized summary must not advertise a principal source inventory lane: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, `required_files=["internal/types/enums.go"]`) {
		t.Fatalf("summary should retain the grounded supporting file: %s", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsSourceInventoryForBoundedArchitectureDiagramMembers(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","members","responsibility","diagram"],
		"entities":["PipelineMember","MemberBinding"],
		"question_kind":"mechanism",
		"intent_confidence":0.95,
		"complexity_confidence":0.85,
		"kind_confidence":0.9,
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":false,
			"is_history_lookup":false,
			"is_diagnostic_question":false,
			"has_per_member_table":false
		},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"answer_subject":{"kind":"type_name","confidence":0.85},
		"diagram_hint":{"kind":"flow","required":true,"relation_scope_quote":"Draw the 4 principal members as a flow diagram","participants":[]},
		"enumeration_boundary":{"declared_count":4,"source_quote":"4 principal members"},
		"completeness_obligation":{"required":false,"source_quote":""},
		"source_inventory_profile":{
			"is_source_inventory":true,
			"target_roles":["constant","type"],
			"requested_fields":["name","summary"],
			"source_quotes":["4 principal members"],
			"confidence":0.8
		}
	}`
	rawRequest := "Draw the 4 principal members as a flow diagram and explain each responsibility."
	mu := types.NewMutableState(rawRequest)
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, RepoRoot: t.TempDir(), PresentationDiagramRequired: true},
		json.RawMessage(withRequiredAnswerRoleProfile(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("bounded architecture diagram analysis should succeed after deterministic demotion: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("incidental bounded-diagram source inventory survived emit path: %+v", rm)
	}
	if rm.EnumerationBoundary == nil || rm.EnumerationBoundary.DeclaredCount != 4 ||
		rm.DiagramHint == nil || !rm.DiagramHint.Required {
		t.Fatalf("model-owned bounded diagram contract must survive source-inventory demotion: %+v", rm)
	}
	if strings.Contains(res.Summary, "source_inventory=") {
		t.Fatalf("normalized summary must not advertise a principal source inventory lane: %s", res.Summary)
	}
}

func TestEmitAnalysis_Execute_ConceptualArchitectureDiagramOutranksModelAddedConstSet(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["pipeline","members","responsibility","diagram"],
		"entities":["PipelineMember","MemberBinding"],
		"question_kind":"mechanism",
		"intent_confidence":0.95,
		"complexity_confidence":0.85,
		"kind_confidence":0.9,
		"predicates":{
			"is_scalar_answer":false,
			"is_role_locate_lookup":false,
			"is_count_question":false,
			"is_cross_component":false,
			"is_relational_lookup":false,
			"is_category_enumeration":false,
			"is_history_lookup":false,
			"is_diagnostic_question":false,
			"has_per_member_table":false
		},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"diagram_hint":{"kind":"flow","required":true,"relation_scope_quote":"Draw all principal members as a flow diagram","participants":[]},
		"completeness_obligation":{"required":true,"source_quote":"all principal members"},
		"source_inventory_profile":{
			"is_source_inventory":true,
			"target_roles":["constant","type"],
			"requires_const_set":true,
			"requested_fields":["name","location","summary","values"],
			"source_quotes":["all principal members"],
			"confidence":0.82
		}
	}`
	rawRequest := "Draw all principal members as a flow diagram and explain each responsibility."
	mu := types.NewMutableState(rawRequest)
	res, err := (&EmitAnalysis{}).Execute(
		&types.BusContext{Mutable: mu, RepoRoot: t.TempDir(), PresentationDiagramRequired: true},
		json.RawMessage(withRequiredAnswerRoleProfile(payload)),
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("conceptual architecture diagram should succeed after deterministic demotion: %s", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile != nil {
		t.Fatalf("model-added const-set source inventory survived conceptual diagram boundary: %+v", rm)
	}
	if !rm.CompletenessObligation.IsActive() || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		t.Fatalf("model-owned completeness and diagram contracts must survive source-inventory demotion: %+v", rm)
	}
}

func TestEmitAnalysisSchema_SourceInventoryExcludesConceptualArchitectureMembers(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatalf("decode emit_analysis schema: %v", err)
	}
	properties, _ := schema["properties"].(map[string]any)
	profile, _ := properties["source_inventory_profile"].(map[string]any)
	description, _ := profile["description"].(string)
	for _, want := range []string{"conceptual stages", "architecture/mechanism", "role=stage_or_workflow", "supporting evidence"} {
		if !strings.Contains(description, want) {
			t.Fatalf("source_inventory_profile schema description missing %q: %q", want, description)
		}
	}
	dimensions, _ := properties["requested_answer_dimensions"].(map[string]any)
	dimensionProperties, _ := dimensions["properties"].(map[string]any)
	dimensionList, _ := dimensionProperties["dimensions"].(map[string]any)
	items, _ := dimensionList["items"].(map[string]any)
	itemProperties, _ := items["properties"].(map[string]any)
	role, _ := itemProperties["role"].(map[string]any)
	roleDescription, _ := role["description"].(string)
	for _, want := range []string{"source_location", "actual user-visible source file/path", "never means a package name, module name, namespace name", "member_set", "observed_value", "source_attribute", "principal structured item", "stage_or_workflow", "does not imply a source declaration inventory"} {
		if !strings.Contains(roleDescription, want) {
			t.Fatalf("requested_answer_dimensions role teaching missing %q: %q", want, roleDescription)
		}
	}
}

func TestEmitAnalysis_Execute_NormalizesSourceInventoryConstructRoleAliases(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	rawRequest := "仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？分别列出文件路径和符号名，并指出包路径（package 声明）。"
	mu := types.NewMutableState(rawRequest)
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "complex",
		"keywords": ["extend", "foreign func", "public class"],
		"entities": ["extend", "foreign func", "public class"],
		"question_kind": "enumeration",
		"sub_topics": [
			{"summary": "列出仓库中所有 ArkTS extend 块", "entities": ["ArkTS", "extend"]},
			{"summary": "列出仓库中所有 ArkTS foreign func 声明", "entities": ["ArkTS", "foreign func"]},
			{"summary": "列出仓库中所有 ArkTS public class", "entities": ["ArkTS", "public class"]}
		],
		"intent_confidence": 0.94,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.92,
		"predicate_axis": "define",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false,
			"has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["extend_block", "foreign_func", "public_class"],
			"requested_fields": ["name", "location", "package"],
			"source_quotes": ["extend 块", "foreign func 声明", "public class"],
			"confidence": 0.9
		},
		"requested_answer_dimensions": {
			"is_dimensioned_answer": true,
			"confidence": 0.9,
			"dimensions": [
				{"index": 1, "label": "文件路径", "role": "source_location", "source_quote": "列出文件路径", "required": true},
				{"index": 2, "label": "符号名", "role": "member_set", "source_quote": "符号名", "required": true},
				{"index": 3, "label": "包路径（package 声明）", "role": "source_location", "source_quote": "并指出包路径（package 声明）", "required": true}
			]
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source_inventory_profile should survive construct-family target_roles, rm=%+v", rm)
	}
	got := rm.SourceInventoryProfile.PrincipalTargetRoles()
	want := []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("principal target roles = %+v, want %+v", got, want)
	}
	if strings.Contains(res.Summary, "target_roles omitted or empty") {
		t.Fatalf("summary should not drop source inventory profile: %s", res.Summary)
	}
	if rm.RequestedAnswerDimensions == nil || len(rm.RequestedAnswerDimensions.Dimensions) != 3 ||
		rm.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionSourceLocation ||
		rm.RequestedAnswerDimensions.Dimensions[2].Role != types.RequestedAnswerDimensionSourceAttribute {
		t.Fatalf("legacy package/source_location drift should reconcile from typed requested_fields: %+v", rm.RequestedAnswerDimensions)
	}
	if !strings.Contains(res.Summary, "source_attribute") {
		t.Fatalf("normalization warning should remain auditable: %s", res.Summary)
	}
	if len(rm.SubTopics) != 3 {
		t.Fatalf("source inventory subtopics should be reconstructed from source_quotes, got %+v", rm.SubTopics)
	}
	for _, topic := range rm.SubTopics {
		if strings.Contains(topic.Summary, "ArkTS") {
			t.Fatalf("prescan-only language guesses must not survive source_inventory subtopic normalization: %+v", rm.SubTopics)
		}
	}
	joinedKeywords := strings.Join(rm.AnalyzerHints.Keywords, "\n")
	for _, guessed := range []string{"ArkTS", "iota"} {
		if strings.Contains(joinedKeywords, guessed) {
			t.Fatalf("prescan-only syntax/language guess %q must not survive source_inventory keyword normalization: %+v", guessed, rm.AnalyzerHints.Keywords)
		}
	}
	for _, want := range []string{"extend 块", "foreign func 声明", "public class", "type", "function", "name", "location", "package"} {
		if !strings.Contains(joinedKeywords, want) {
			t.Fatalf("source_inventory keyword normalization lost typed/validated vocabulary %q: %+v", want, rm.AnalyzerHints.Keywords)
		}
	}
}

func TestEmitAnalysis_Execute_SourceInventoryInfersSubjectBeforeValuesNormalization(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("列出 ArkTS 里 @Entry 标记的页面入口和 @Builder 复用片段，给出文件路径和函数名。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["ArkTS", "@Entry", "@Builder", "function"],
		"entities": ["@Entry", "@Builder"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.86,
		"kind_confidence": 0.92,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": true
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"source_scope_profile": {
			"requested_scope": "all",
			"include_auxiliary_as_principal": true,
			"confidence": 0.9,
			"source_quotes": ["ArkTS"]
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location", "values"],
			"source_quotes": ["@Entry", "@Builder"],
			"confidence": 0.9
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		t.Fatalf("source_inventory_profile should persist: %+v", rm)
	}
	if rm.AnswerSubject.Kind != types.SubjectFunctionName {
		t.Fatalf("answer_subject should be inferred from source_inventory target role, got %+v", rm.AnswerSubject)
	}
	if rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldValues) {
		t.Fatalf("values display drift should be removed for function-name inventory: %+v", rm.SourceInventoryProfile.RequestedFields)
	}
	if !types.SourceInventoryPrincipalNavigationActive(*rm) {
		t.Fatalf("normalized marker/decorator inventory should own source_inventory navigation: %+v", rm.SourceInventoryProfile)
	}
}

func TestEmitAnalysis_Execute_SourceInventoryConstSetDoesNotImplyValuesField(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出 internal/types 包里所有公开的字符串枚举类型名（type X string 加 const 集合）。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["symbol", "enum", "type", "string"],
		"entities": ["internal/types"],
		"question_kind": "enumeration",
		"intent_confidence": 0.94,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_subject": {
			"kind": "type_name",
			"entity_axes": ["type → const_set"],
			"confidence": 0.9
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["type", "constant"],
			"type_underlying": "string",
			"requires_const_set": true,
			"requested_fields": ["name", "location", "values"],
			"source_quotes": ["字符串枚举类型名"],
			"confidence": 0.95,
			"rationale": "current request asks for public string enum type names"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.SourceInventoryProfile == nil {
		t.Fatalf("SourceInventoryProfile not persisted: %+v", rm)
	}
	if rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldValues) {
		t.Fatalf("const-set qualifier should not request enum values for a type-name inventory: %+v", rm.SourceInventoryProfile.RequestedFields)
	}
	if !rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldName) ||
		!rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldLocation) {
		t.Fatalf("normalizer should preserve requested name/location fields: %+v", rm.SourceInventoryProfile.RequestedFields)
	}
}

func TestEmitAnalysis_Execute_RejectsUngroundedAnswerExclusionPolicy(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("列出公开 API。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["api"],
		"entities": ["API"],
		"question_kind": "enumeration",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_exclusion_policy": {
			"is_exclusion_requested": true,
			"excluded_candidate_roles": ["variable"],
			"source_quotes": ["不要列变量"],
			"confidence": 0.94
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatalf("Execute should reject ungrounded answer_exclusion_policy, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "answer_exclusion_policy.source_quotes") {
		t.Fatalf("rejection should name answer_exclusion_policy.source_quotes, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsAnswerRoleProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Is analyze retry wired, and what is the retry budget parameter called?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["analyze", "retry"],
		"entities": ["analyze"],
		"question_kind": "return_value",
		"intent_confidence": 0.93,
		"complexity_confidence": 0.76,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["budget_cap"],
			"source_quotes": ["retry budget parameter"],
			"confidence": 0.94,
			"rationale": "the current request asks for the budget cap parameter, not the retry attempt counter"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "required_roles=budget_cap") {
		t.Fatalf("summary should surface typed answer-role lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.AnswerRoleProfile == nil || !rm.AnswerRoleProfile.Active() {
		t.Fatalf("AnswerRoleProfile not persisted: %+v", rm)
	}
	if got := rm.AnswerRoleProfile.RequiredCandidateRoles; len(got) != 1 || got[0] != types.AnswerCandidateRoleBudgetCap {
		t.Fatalf("required roles wrong: %+v", got)
	}
}

func TestEmitAnalysis_Execute_SoftensUngroundedAnswerRoleProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Is analyze retry wired?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "simple",
		"keywords": ["analyze", "retry"],
		"entities": ["analyze"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["budget_cap"],
			"source_quotes": ["retry budget parameter"],
			"confidence": 0.94
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("ungrounded optional answer_role_profile should be ignored, not reject: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "answer_role_profile auto-softened") {
		t.Fatalf("summary should surface answer_role_profile softening, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm == nil || rm.AnswerRoleProfile != nil {
		t.Fatalf("unanchored optional answer_role_profile must not become a hard role gate: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_SoftensAnswerRoleProfileMissingSourceQuotes(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Which retry budget parameter controls analyze retries?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "simple",
		"keywords": ["analyze", "retry", "budget"],
		"entities": ["analyze"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"answer_role_profile": {
			"is_role_binding_requested": true,
			"required_candidate_roles": ["budget_cap"],
			"confidence": 0.94
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("missing source_quotes on optional answer_role_profile should not reject: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "answer_role_profile auto-softened") {
		t.Fatalf("summary should surface answer_role_profile softening, got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm == nil || rm.AnswerRoleProfile != nil {
		t.Fatalf("source-quote-free answer_role_profile must not become a hard role gate: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_PersistsErrorGranularityProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("If one record fails validation in the batch, does the system reject just that record or fail the whole batch?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["record", "validation", "batch"],
		"entities": ["batch"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"error_granularity_profile": {
			"is_granularity_question": true,
			"requested_verdict_options": ["per_item_rejection", "whole_batch_failure"],
			"source_quotes": ["reject just that record or fail the whole batch"],
			"confidence": 0.95,
			"rationale": "current request asks for failure scope across record and batch"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "error_granularity=true") {
		t.Fatalf("summary should surface typed error granularity lane, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || rm.ErrorGranularityProfile == nil || !rm.ErrorGranularityProfile.Active() {
		t.Fatalf("ErrorGranularityProfile not persisted: %+v", rm)
	}
	if rm.ErrorGranularityProfile.Confidence != 0.95 ||
		len(rm.ErrorGranularityProfile.SourceQuotes) != 1 {
		t.Fatalf("ErrorGranularityProfile fields wrong: %+v", rm.ErrorGranularityProfile)
	}
	if got := rm.ErrorGranularityProfile.RequestedVerdictOptions; len(got) != 2 ||
		got[0] != types.ErrorGranularityPerItemRejection ||
		got[1] != types.ErrorGranularityWholeBatch {
		t.Fatalf("ErrorGranularityProfile requested options wrong: %+v", got)
	}
}

func TestEmitAnalysis_Execute_SoftensDiagnosticScalarMechanismErrorGranularityProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("请结合当前源码区分模型响应超时和成文校验失败的机制")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["model", "timeout", "finalizer"],
		"entities": ["model timeout", "finalizer validation"],
		"question_kind": "mechanism",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": true,
			"confidence": 0.8
		},
		"current_source_explanation_profile": {
			"is_current_source_explanation_requested": true,
			"modes": ["explain_current_mechanism"],
			"source_quotes": ["结合当前源码"],
			"confidence": 0.8
		},
		"error_granularity_profile": {
			"is_granularity_question": true,
			"requested_verdict_options": ["per_item_rejection", "whole_batch_failure"],
			"source_quotes": ["区分模型响应超时和成文校验失败"],
			"confidence": 0.85,
			"rationale": "diagnostic class distinction is not a failure-scope verdict"
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should soften optional diagnostic error granularity, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.ErrorGranularityProfile != nil {
		t.Fatalf("diagnostic scalar mechanism profile should not force error granularity contract: %+v", rm.ErrorGranularityProfile)
	}
	if !strings.Contains(res.Summary, "error_granularity_profile auto-softened") {
		t.Fatalf("summary should report softening warning, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_SoftensUngroundedErrorGranularityProfile(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("If one record fails validation in the batch, what happens?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "simple",
		"keywords": ["record", "validation", "batch"],
		"entities": ["batch"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"error_granularity_profile": {
			"is_granularity_question": true,
			"source_quotes": ["reject just that record or fail the whole batch"],
			"confidence": 0.95
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should soften ungrounded error_granularity_profile, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.ErrorGranularityProfile != nil {
		t.Fatalf("ungrounded optional profile should be ignored, got %+v", rm.ErrorGranularityProfile)
	}
	if !strings.Contains(res.Summary, "error_granularity_profile auto-softened") {
		t.Fatalf("summary should report softening warning, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsInvalidErrorGranularityOption(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Does the bad record get rejected or does the whole batch fail?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "architecture_explain",
		"complexity": "simple",
		"keywords": ["record", "batch"],
		"entities": ["batch"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"error_granularity_profile": {
			"is_granularity_question": true,
			"requested_verdict_options": ["mostly_ok"],
			"source_quotes": ["bad record get rejected or does the whole batch fail"],
			"confidence": 0.95
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatalf("Execute should reject invalid error granularity option, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "requested_verdict_options") {
		t.Fatalf("rejection should name requested_verdict_options, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsInvalidOptionalExactTargets(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["explore", "hint", "budget"],
		"entities": ["explore_mid_loop_hint_budget", "codrax.yaml"],
		"question_kind": "config_mapping",
		"exact_targets": ["codrax.yaml"],
		"intent_confidence": 0.8,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("invalid optional exact_targets should be dropped, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped invalid optional exact_targets") {
		t.Fatalf("summary should disclose exact_targets cleanup, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist")
	}
	if len(rm.AnalyzerHints.ExactTargets) != 0 {
		t.Fatalf("invalid exact_targets should be dropped, got %+v", rm.AnalyzerHints.ExactTargets)
	}
}

func TestEmitAnalysis_Execute_DropsRequiredFileExactTargetsForSourceInventory(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("internal/analysis/criterion 只列公开函数，每个函数给出文件:行和中文职责说明")
	tool := &EmitAnalysis{}
	payload := withV4Required(`{
		"intent": "enumerate",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["criterion", "公开函数", "EvalAll"],
		"entities": ["internal/analysis/criterion", "EvalAll"],
		"question_kind": "enumeration",
		"language": "zh",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": true,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"exact_targets": ["internal/analysis/criterion/eval.go", "internal/analysis/criterion/grammar.go"],
		"required_files": [
			{"path":"internal/analysis/criterion/eval.go","confidence":0.95},
			{"path":"internal/analysis/criterion/grammar.go","confidence":0.9}
		],
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location", "summary"],
			"source_quotes": ["只列公开函数"],
			"confidence": 0.95
		}
	}`)

	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("required-file exact_targets should be dropped, not rejected: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped exact_targets") {
		t.Fatalf("summary should disclose dropped exact_targets, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist")
	}
	if len(rm.AnalyzerHints.ExactTargets) != 0 {
		t.Fatalf("non-verbatim file exact_targets should be dropped: %+v", rm.AnalyzerHints.ExactTargets)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 0 {
		t.Fatalf("non-verbatim source-inventory required file hints should stay soft: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if !strings.Contains(res.Summary, "required_files=[]") {
		t.Fatalf("summary should expose normalized empty required_files lane, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "model-authored source-inventory path hint") {
		t.Fatalf("summary should disclose source-inventory required_files softening, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DemotesRequiredFileExactTargetWhenNonFileSubjectExists(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("根据 CLAUDE.md，OAuthCredentials 的格式是怎样的？")
	tool := &EmitAnalysis{}
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["CLAUDE.md", "OAuthCredentials", "格式"],
		"entities": ["CLAUDE.md", "OAuthCredentials"],
		"question_kind": "mechanism",
		"language": "zh",
		"answer_subject": {"kind":"type_name","confidence":0.9},
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"exact_targets": ["CLAUDE.md"],
		"required_files": [
			{"path":"CLAUDE.md","confidence":0.95,"rationale":"上下文说明文件"}
		]
	}`)
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("context file exact target should demote, not reject: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "demoted exact_targets") {
		t.Fatalf("summary should disclose exact-target demotion, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist")
	}
	if len(rm.AnalyzerHints.ExactTargets) != 0 {
		t.Fatalf("context file exact target should be demoted: %+v", rm.AnalyzerHints.ExactTargets)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 1 || rm.AnalyzerHints.RequiredFileHints[0].Path != "CLAUDE.md" {
		t.Fatalf("required file context should remain preserved: %+v", rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestEmitAnalysis_Execute_KeepsRequiredFileExactTargetForFileSubject(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("CLAUDE.md 这个文件是否存在？")
	tool := &EmitAnalysis{}
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "simple",
		"keywords": ["CLAUDE.md", "文件", "存在"],
		"entities": ["CLAUDE.md"],
		"question_kind": "direct_lookup",
		"language": "zh",
		"answer_subject": {"kind":"file_path","confidence":0.9},
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"exact_targets": ["CLAUDE.md"],
		"required_files": [
			{"path":"CLAUDE.md","confidence":0.95,"rationale":"用户询问的文件"}
		]
	}`)
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("file-subject exact target should remain valid: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel should persist")
	}
	if got := rm.AnalyzerHints.ExactTargets; len(got) != 1 || got[0] != "CLAUDE.md" {
		t.Fatalf("file-subject exact target should be preserved, got %+v", got)
	}
}

func TestEmitAnalysis_Execute_DropsInvalidExactTargetsForRuntimeArtifact(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("这段 HiTrace 里 GC span 的开始事件在附件 trace 的第几行？有没有超过 50ms 的 GC span？只分析 trace")
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"gc-pause"}},
		Observations: []types.PerfObservation{{
			Kind:       "span",
			Subject:    "H:GC:Collect",
			Summary:    "GC span starts on trace line 5 and lasts 8ms",
			LineStart:  5,
			LineEnd:    6,
			DurationMs: 8,
			Confidence: 0.95,
		}},
	})
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["GC", "span", "trace", "50ms"],
		"entities": ["H:GC:Collect"],
		"question_kind": "return_value",
		"answer_subject": {"kind": "numeric", "confidence": 0.9},
		"exact_targets": ["H:GC:Collect", "50ms"],
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "只分析 trace",
			"confidence": 0.9
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("invalid optional exact_targets on runtime artifact should be filtered, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if got := rm.AnalyzerHints.ExactTargets; len(got) != 1 || got[0] != "50ms" {
		t.Fatalf("should keep only request-verbatim exact target, got %#v", got)
	}
}

func TestEmitAnalysis_Execute_DefaultsRuntimeArtifactRoleLocateSubject(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("这段日志里 WARN 在附件日志的第几行？只分析日志")
	mu.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Lang: "text", Signals: []types.LogSignal{types.SignalValidation}},
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRuntimeEvent,
			Summary:    "WARN appears on line 3",
			LineStart:  3,
			Diagnostic: true,
			Confidence: 0.95,
		}},
	})
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["WARN", "日志", "行号"],
		"entities": ["WARN"],
		"question_kind": "return_value",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		},
		"external_observation_policy": {
			"current_source_mode": "exclude",
			"exclusion_kind": "explicit_user_exclusion",
			"current_source_exclusion_quote": "只分析日志",
			"confidence": 0.9
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("runtime artifact line lookup should default answer_subject.kind, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.AnswerSubject.Kind != types.SubjectNumeric {
		t.Fatalf("answer_subject.kind should default to numeric, got %+v", rm.AnswerSubject)
	}
	if rm.Intent != types.IntentReturnValue {
		t.Fatalf("runtime artifact scalar question should normalize to return_value, got %s", rm.Intent)
	}
}

func TestEmitAnalysis_Execute_DefaultsTypedRoleLocateSubjectFromQuestionKind(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Which function is the entry point for this behavior?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["entry", "function", "behavior"],
		"entities": ["behavior"],
		"question_kind": "call_chain",
		"predicate_axis": "call",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("typed role-locate subject should default from question_kind, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if rm.AnswerSubject.Kind != types.SubjectFunctionName {
		t.Fatalf("answer_subject.kind = %q, want function_name", rm.AnswerSubject.Kind)
	}
}

func TestEmitAnalysis_Execute_DoesNotDefaultAmbiguousRoleLocateSubject(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Locate the thing related to this mechanism.")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["thing", "mechanism"],
		"entities": ["mechanism"],
		"question_kind": "mechanism",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": true,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.1
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("ambiguous role-locate subject should remain fail-loud")
	}
	if !strings.Contains(res.Summary, "answer_subject.kind") {
		t.Fatalf("rejection should point at missing answer_subject.kind, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_DropsInvalidExactContextTermsWithWarning(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["explore", "hint", "budget"],
		"entities": ["explore_mid_loop_hint_budget"],
		"question_kind": "config_mapping",
		"exact_targets": ["explore_mid_loop_hint_budget"],
		"exact_context_terms": ["runtime"],
		"intent_confidence": 0.8,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("invalid exact_context_terms should be dropped, not reject: %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if len(rm.AnalyzerHints.ExactContextTerms) != 0 {
		t.Fatalf("ExactContextTerms = %v, want empty after drop", rm.AnalyzerHints.ExactContextTerms)
	}
	if !strings.Contains(res.Summary, "ignored exact_context_terms") {
		t.Fatalf("summary should surface drop warning, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsExactTargetsAndHistoryPredicate(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("Who introduced EvidenceClosure in git history?")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "return_value",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["EvidenceClosure", "history", "commit"],
		"entities": ["EvidenceClosure"],
		"question_kind": "history",
		"exact_targets": ["EvidenceClosure"],
		"intent_confidence": 0.91,
		"complexity_confidence": 0.80,
		"kind_confidence": 0.93,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": true,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !rm.Predicates.IsHistoryLookup {
		t.Fatalf("IsHistoryLookup = false, want true")
	}
	if len(rm.AnalyzerHints.ExactTargets) != 1 || rm.AnalyzerHints.ExactTargets[0] != "EvidenceClosure" {
		t.Fatalf("ExactTargets = %v, want [EvidenceClosure]", rm.AnalyzerHints.ExactTargets)
	}
}

func TestEmitAnalysis_Execute_AllowsNonScalarHistoryLookup(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("最近一次合入的是什么特性？请说明特性内容。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "explain",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["merge", "合入", "特性", "git log"],
		"entities": [],
		"question_kind": "history",
		"intent_confidence": 0.91,
		"complexity_confidence": 0.80,
		"kind_confidence": 0.93,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": true,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("non-scalar history lookup should be accepted, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !rm.Predicates.IsHistoryLookup || rm.Predicates.IsScalarAnswer {
		t.Fatalf("predicates = %+v, want history=true scalar=false", rm.Predicates)
	}
	if rm.Intent != types.IntentExplain || types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) != types.ReqHistory {
		t.Fatalf("history summary shape drifted: intent=%s kind=%s", rm.Intent, rm.AnalyzerHints.Kind)
	}
}

func TestEmitAnalysis_Execute_RejectsScalarHistoryWithNarrativeIntent(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("最近一次合入的是什么特性？请说明特性内容。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "trace",
		"scenario": "generic",
		"complexity": "simple",
		"keywords": ["merge", "合入", "特性", "git log"],
		"entities": [],
		"question_kind": "history",
		"intent_confidence": 0.91,
		"complexity_confidence": 0.80,
		"kind_confidence": 0.93,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": true,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatalf("scalar history with trace intent should be rejected")
	}
	if !strings.Contains(res.Summary, "principal history scalar answers must use intent=return_value") {
		t.Fatalf("summary should guide scalar vs narrative history repair, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_AllowsHistoryTraceDiagramWhenNonScalar(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("根据最近一次合入找到对应代码，详细解释并画出逻辑图。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["git log", "合入", "对应代码", "逻辑图", "diagram"],
		"entities": [],
		"question_kind": "history",
		"intent_confidence": 0.91,
		"complexity_confidence": 0.80,
		"kind_confidence": 0.93,
		"diagram_hint": {"kind": "flow", "required": true, "relation_scope_quote":"画出逻辑图", "participants": []},
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": true,
			"is_diagnostic_question": false, "has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("history-backed code trace/diagram should be accepted when non-scalar, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !rm.Predicates.IsHistoryLookup || rm.Predicates.IsScalarAnswer {
		t.Fatalf("predicates = %+v, want history=true scalar=false", rm.Predicates)
	}
	if rm.Intent != types.IntentTrace || rm.DiagramHint == nil || rm.DiagramHint.Kind != types.DiagramFlow {
		t.Fatalf("history trace/diagram shape drifted: intent=%s diagram=%+v", rm.Intent, rm.DiagramHint)
	}
}

func TestEmitAnalysis_Execute_PersistsExactContextTerms(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "simple",
		"keywords": ["explore", "hint", "budget"],
		"entities": ["explore_mid_loop_hint_budget"],
		"question_kind": "config_mapping",
		"exact_targets": ["explore_mid_loop_hint_budget"],
		"exact_context_terms": ["explore"],
		"intent_confidence": 0.8,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.8,
		"predicates": {
			"is_scalar_answer": true,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if !reflect.DeepEqual(rm.AnalyzerHints.ExactContextTerms, []string{"explore"}) {
		t.Fatalf("ExactContextTerms = %v, want [explore]", rm.AnalyzerHints.ExactContextTerms)
	}
	if !strings.Contains(res.Summary, "exact_ctx=1") {
		t.Fatalf("summary should mention exact_ctx count, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_PersistsExactContextRoles(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["explore_mid_loop_hint_budget", "code default", "codrax.yaml", "CLI"],
		"entities": ["explore_mid_loop_hint_budget", "codrax.yaml"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "config_key"},
		"exact_targets": ["explore_mid_loop_hint_budget"],
		"exact_context_roles": ["default", "config", "override"],
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	want := []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleDefault,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleOverride,
	}
	if !reflect.DeepEqual(rm.AnalyzerHints.ExactContextRoles, want) {
		t.Fatalf("ExactContextRoles = %v, want %v", rm.AnalyzerHints.ExactContextRoles, want)
	}
	if !strings.Contains(res.Summary, "exact_roles=3") {
		t.Fatalf("summary should mention exact_roles count, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RecoversMultiKeyConfigTargetsBeforeRoleValidation(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	request := "比较 pipeline_max_steps 和 pipeline_max_retries_per_stage，并逐项列出默认值、配置文件和 CLI 覆盖层。"
	tool := &EmitAnalysis{}
	base := `{
		"intent":"config_query","scenario":"config_trace","complexity":"moderate",
		"keywords":["pipeline_max_steps","pipeline_max_retries_per_stage"],
		"entities":["pipeline_max_steps","pipeline_max_retries_per_stage"],
		"question_kind":"config_mapping","predicate_axis":"configure",
		"intent_confidence":0.9,"complexity_confidence":0.8,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":true,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":true},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9}`

	mu := types.NewMutableState(request)
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(base+`}`)))
	if res.Success || !strings.Contains(res.Summary, "must emit exact_context_roles") {
		t.Fatalf("missing roles should be rejected after exact-target recovery, got success=%t summary=%q", res.Success, res.Summary)
	}

	mu = types.NewMutableState(request)
	res, _ = tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(base+`,"exact_context_roles":["default","config","override"]}`)))
	if !res.Success {
		t.Fatalf("typed roles should make recovered multi-key mapping valid, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	wantTargets := []string{"pipeline_max_steps", "pipeline_max_retries_per_stage"}
	if !reflect.DeepEqual(rm.AnalyzerHints.ExactTargets, wantTargets) {
		t.Fatalf("ExactTargets = %v, want %v", rm.AnalyzerHints.ExactTargets, wantTargets)
	}
}

func TestEmitAnalysis_Execute_NormalizesFiniteMultiKeyConfigValueInventoryToPerMemberMatrix(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	request := "比较 pipeline_max_steps 和 pipeline_max_retries_per_stage，分别列出默认值、配置文件和 CLI 覆盖层。"
	tool := &EmitAnalysis{}
	base := `{
		"intent":"config_query","scenario":"config_trace","complexity":"moderate",
		"keywords":["pipeline_max_steps","pipeline_max_retries_per_stage"],
		"entities":["pipeline_max_steps","pipeline_max_retries_per_stage"],
		"question_kind":"config_mapping","predicate_axis":"configure",
		"intent_confidence":0.9,"complexity_confidence":0.8,"kind_confidence":0.9,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":false,"is_category_enumeration":true,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":false},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.9},
		"source_inventory_profile":{"is_source_inventory":true,"target_roles":["config_key"],"requested_fields":["name","values"],"source_quotes":["pipeline_max_steps","pipeline_max_retries_per_stage"],"confidence":0.9}`

	mu := types.NewMutableState(request)
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(base+`}`)))
	if res.Success || !strings.Contains(res.Summary, "must emit exact_context_roles") {
		t.Fatalf("typed multi-key value inventory should normalize to a matrix and require roles, success=%t summary=%q", res.Success, res.Summary)
	}

	mu = types.NewMutableState(request)
	res, _ = tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(base+`,"exact_context_roles":["default","config","override"]}`)))
	if !res.Success {
		t.Fatalf("typed precedence roles should close normalized matrix analysis, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || !rm.Predicates.HasPerMemberTable {
		t.Fatalf("finite config value inventory did not persist per-member matrix shape: %+v", rm)
	}
}

func TestEmitAnalysis_Execute_PreservesConfigTraceRolesWhenAnswerSubjectDriftsNumeric(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。")
	tool := &EmitAnalysis{}
	payload := `{
		"intent": "config_query",
		"scenario": "config_trace",
		"complexity": "moderate",
		"keywords": ["explore_mid_loop_hint_budget", "code default", "codrax.yaml", "CLI"],
		"entities": ["explore_mid_loop_hint_budget", "codrax.yaml"],
		"question_kind": "config_mapping",
		"answer_subject": {"kind": "numeric"},
		"exact_targets": ["explore_mid_loop_hint_budget"],
		"exact_context_roles": ["default", "config", "override"],
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": false, "has_per_member_table": false
		}
		,
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if !res.Success {
		t.Fatalf("Execute should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	want := []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleDefault,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleOverride,
	}
	if !reflect.DeepEqual(rm.AnalyzerHints.ExactContextRoles, want) {
		t.Fatalf("ExactContextRoles = %v, want %v", rm.AnalyzerHints.ExactContextRoles, want)
	}
	if !strings.Contains(res.Summary, "exact_roles=3") {
		t.Fatalf("summary should mention exact_roles count, got %q", res.Summary)
	}
}
