package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// -----------------------------------------------------------------------------
// Validator tests (pure, no tool wiring)
// -----------------------------------------------------------------------------

func TestValidateAnalysisInput_HappyPath(t *testing.T) {
	limits := DefaultAnalysisLimits()
	kw := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	ents := []string{"OrchestratorAgent", "StageAnalyze"}

	res := validateAnalysisInput(kw, ents, limits)

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

func TestValidateAnalysisInput_WarnBelowKeywords(t *testing.T) {
	limits := AnalysisLimits{WarnBelowKeywords: 8, RejectBelowKeywords: 0}
	res := validateAnalysisInput([]string{"a", "b", "c"}, nil, limits)

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
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits)

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
	res := validateAnalysisInput([]string{"a", "b"}, nil, limits)

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
	res := validateAnalysisInput(nil, ents, limits)

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
	if !strings.Contains(res.Warnings[0], "dropped generic entities") {
		t.Errorf("warning text missing label, got %q", res.Warnings[0])
	}
}

func TestValidateAnalysisInput_EmptyBlocklistSkipsFilter(t *testing.T) {
	limits := AnalysisLimits{
		WarnBelowKeywords:      0,
		RejectBelowKeywords:    0,
		GenericEntityBlocklist: nil,
	}
	ents := []string{"agent", "handler", "Explorer"}
	res := validateAnalysisInput(nil, ents, limits)

	if len(res.DroppedEntities) != 0 {
		t.Errorf("nil blocklist must drop nothing, got %v", res.DroppedEntities)
	}
	if len(res.FilteredEntities) != 3 {
		t.Errorf("nil blocklist must pass all entities through, got %v", res.FilteredEntities)
	}
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

func runEmitAnalysis(t *testing.T, payload string) (types.ToolResult, *types.MutableState) {
	t.Helper()
	mu := types.NewMutableState("trace the pipeline through analyze")
	busCtx := &types.BusContext{Mutable: mu}
	tool := &EmitAnalysis{}
	res, err := tool.Execute(busCtx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res, mu
}

func TestEmitAnalysis_Execute_PersistsNormalizedRequestModel(t *testing.T) {
	// Use limits that don't warn on 3 keywords so we isolate the
	// happy-path persistence behavior.
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	payload := `{
		"intent": "root-cause",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["orchestrator", "pipeline", "analyze"],
		"entities": ["Orchestrator", "StageAnalyze"],
		"question_kind": "mechanism",
		"answer_shape": "explanation"
	}`

	res, mu := runEmitAnalysis(t, payload)
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
	if rm.Scenario != types.ScenarioArchitectureExplain {
		t.Errorf("Scenario = %q, want architecture_explain", rm.Scenario)
	}
	if len(rm.AnalyzerHints.Keywords) != 3 {
		t.Errorf("Keywords count = %d, want 3", len(rm.AnalyzerHints.Keywords))
	}
	if len(rm.AnalyzerHints.Entities) != 2 {
		t.Errorf("Entities count = %d, want 2", len(rm.AnalyzerHints.Entities))
	}
}

func TestEmitAnalysis_Summary_ReportsNormalizedDelta(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	// "root-cause" and "symbol_list" both coerce to canonical values
	// and should appear in the "normalized:" clause of the Summary.
	payload := `{
		"intent": "root-cause",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["a"],
		"entities": ["Foo"],
		"question_kind": "register",
		"answer_shape": "symbol_list"
	}`

	res, _ := runEmitAnalysis(t, payload)
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
		`answer_shape "symbol_list"→"list_of_symbols"`,
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
		"answer_shape": "explanation"
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
		"answer_shape": "explanation"
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
		"answer_shape": "explanation"
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
		"answer_shape": "explanation"
	}`

	res, mu := runEmitAnalysis(t, payload)
	if !res.Success {
		t.Fatalf("entity filter must not fail the call, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped generic entities") {
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
	if !strings.Contains(res.Summary, "Mutable") {
		t.Errorf("failure Summary should mention Mutable, got %q", res.Summary)
	}
}
