package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

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
		"is_diagnostic_question": false
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
	return body + v4DefaultsJSON + "}"
}

func withRequiredAnswerRoleProfile(payload string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return payload
	}
	if _, ok := obj["answer_role_profile"]; ok {
		if _, ok := obj["error_granularity_profile"]; ok {
			return payload
		}
	}
	if _, ok := obj["answer_role_profile"]; !ok {
		obj["answer_role_profile"] = json.RawMessage(`{"is_role_binding_requested":false,"confidence":0.7}`)
	}
	if _, ok := obj["error_granularity_profile"]; !ok {
		obj["error_granularity_profile"] = json.RawMessage(`{"is_granularity_question":false,"confidence":0.7}`)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return string(out)
}

func testBoolPtr(v bool) *bool {
	return &v
}

func testFloatPtr(v float64) *float64 {
	return &v
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
			"is_diagnostic_question": false
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
	if reason == "" || !strings.Contains(reason, "intent=root_cause") {
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
	if reason == "" || !strings.Contains(reason, "scenario=root_cause") {
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

func TestValidateAnalysisInput_RejectBelowKeywords(t *testing.T) {
	limits := AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 5}
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits, "", 0)

	if res.RejectReason == "" {
		t.Fatal("hard floor must reject")
	}
	if !strings.Contains(res.RejectReason, "got=2") || !strings.Contains(res.RejectReason, "want≥5") {
		t.Errorf("reject reason missing count details, got %q", res.RejectReason)
	}
	if !strings.Contains(res.RejectReason, "hard floor") {
		t.Errorf("reject reason should say 'hard floor', got %q", res.RejectReason)
	}
}

func TestValidateAnalysisInput_RejectWinsOverWarn(t *testing.T) {
	// When both thresholds trip, reject must win so the caller gets
	// the machine-readable failure signal instead of only a soft warning.
	limits := AnalysisLimits{WarnBelowKeywords: 8, RejectBelowKeywords: 6}
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits, "", 0)

	if res.RejectReason == "" {
		t.Fatal("hard floor must reject when both thresholds trip")
	}
	// Warnings may or may not also fire; the reject reason is the
	// load-bearing signal so we do not assert on Warnings content.
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
// in the pre-scan summary blob as a standalone code/prose token, it
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

func TestEmitAnalysis_Execute_PersistsDiagramHint(t *testing.T) {
	mu := types.NewMutableState("trace the dispatch path")
	payload := `{
		"intent": "trace",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["dispatch", "handler"],
		"entities": ["Dispatch", "Handler"],
		"question_kind": "call_chain",
		"predicate_axis": "call",
		"diagram_hint": {"kind": "call_dag"}
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
	if !strings.Contains(res.Summary, "diagram_hint=call_dag") {
		t.Fatalf("summary missing diagram hint echo: %q", res.Summary)
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
		WarnBelowKeywords:      5,
		RejectBelowKeywords:    2,
		GenericEntityBlocklist: []string{"foo", "bar"},
	}
	SetAnalysisLimits(custom)

	got := CurrentAnalysisLimits()
	if got.WarnBelowKeywords != 5 || got.RejectBelowKeywords != 2 {
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
	busCtx := &types.BusContext{Mutable: mu}
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
	res, err := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
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
	payload = strings.Replace(payload, `"is_diagnostic_question": false`, `"is_diagnostic_question": true`, 1)

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
			"is_diagnostic_question": false
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
		}
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

	// "root-cause" and "register" both coerce to canonical values
	// and should appear in the "normalized:" clause of the Summary.
	payload := withV4Required(`{
		"intent": "root-cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "register",
	}`)
	payload = strings.Replace(payload, `"is_diagnostic_question": false`, `"is_diagnostic_question": true`, 1)

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
	for _, want := range []string{
		`intent "root-cause"→"root_cause"`,
		`question_kind "register"→"registration"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("Summary missing delta %q, got %q", want, res.Summary)
		}
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

func TestEmitAnalysis_Execute_RejectPathDoesNotPersist(t *testing.T) {
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

	if res.Success {
		t.Fatalf("reject path must fail, got summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "rejected") {
		t.Errorf("reject Summary should say 'rejected', got %q", res.Summary)
	}
	if rm := mu.RequestModel(); rm != nil {
		t.Errorf("reject path must not persist RequestModel, got %+v", rm)
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
			"is_diagnostic_question": false
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
		t.Fatal("missing predicate field must reject")
	}
	if !strings.Contains(res.Summary, "is_count_question") {
		t.Errorf("reject summary should name the missing field, got %q", res.Summary)
	}
}

func TestEmitAnalysis_Execute_RejectsMissingDiagnosticProfile(t *testing.T) {
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
			"is_diagnostic_question": false
		}
	}`
	res, _ := tool.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(withRequiredAnswerRoleProfile(payload)))
	if res.Success {
		t.Fatal("missing diagnostic_profile object must reject")
	}
	if !strings.Contains(res.Summary, "diagnostic_profile object missing") {
		t.Errorf("reject summary should name the missing diagnostic_profile object, got %q", res.Summary)
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
			"is_diagnostic_question": false
		},
		"diagnostic_profile": {
			"is_diagnostic": false,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.7
		}
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
			"is_diagnostic_question": false
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
		}
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

func TestEmitAnalysis_Execute_RejectsInconsistentCountIntent(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})
	mu := types.NewMutableState("how many agents are there")
	tool := &EmitAnalysis{}
	// LLM marked is_count_question=true but still picked intent=enumerate.
	// This is the session-6 failure mode the deleted prose-cue table
	// used to silently downgrade. Now the LLM must fix it itself.
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
			"is_diagnostic_question": false
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
		t.Fatal("count question + intent=enumerate must reject")
	}
	if !strings.Contains(res.Summary, "is_count_question") || !strings.Contains(res.Summary, "intent=enumerate") {
		t.Errorf("reject summary should name both contradicting fields, got %q", res.Summary)
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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

func TestEmitAnalysis_Execute_RejectsUngroundedAnswerRoleProfile(t *testing.T) {
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
			"is_diagnostic_question": false
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
	if res.Success {
		t.Fatalf("Execute should reject ungrounded answer_role_profile, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "answer_role_profile.source_quotes") {
		t.Fatalf("rejection should name answer_role_profile.source_quotes, got %q", res.Summary)
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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

func TestEmitAnalysis_Execute_RejectsInvalidExactTargets(t *testing.T) {
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
			"is_diagnostic_question": false
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
		t.Fatal("invalid exact_targets must reject")
	}
	if !strings.Contains(res.Summary, "exact_targets") {
		t.Fatalf("reject summary should mention exact_targets, got %q", res.Summary)
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
			"is_diagnostic_question": false
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
