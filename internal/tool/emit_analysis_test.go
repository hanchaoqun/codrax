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
}

// TestValidateAnalysisInput_WhitelistKeepsVerifiedGenericEntity pins
// the 2026-04-15 fix: when a generic-blocklist entity ALSO appears
// in the pre-scan summary blob (lowercase substring match), it must
// be kept instead of dropped so real symbols named `Agent` or
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
	// Here we simulate a grep files_only=true hit on files that
	// contain both `agent` and `handler` as real symbols, but NOT
	// the word `count`.
	seenBlob := "internal/agent/analyzer.go\ninternal/tool/handler.go\n"
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
	// grep files_only hit on files containing `agent`.
	mu.AppendPrescanSummary("internal/agent/analyzer.go\ninternal/agent/explorer.go")

	payload := `{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["explore"],
		"entities": ["Agent", "Handler", "Orchestrator"],
		"question_kind": "mechanism",
		"answer_shape": "explanation"
	}`

	tl := &EmitAnalysis{}
	res, err := tl.Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
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
	res, err := tool.Execute(busCtx, json.RawMessage(payload))
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
		"answer_shape": "none"
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
		"answer_shape": "explanation"
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
		"answer_shape": "step_list"
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
	if !strings.Contains(res.Summary, "Mutable") {
		t.Errorf("failure Summary should mention Mutable, got %q", res.Summary)
	}
}
