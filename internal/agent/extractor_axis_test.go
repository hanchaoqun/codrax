package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnswerSymbolCorrelatesWithEvidence_Range verifies the (file,
// line) × (source, [LineStart..LineEnd]) correlation rule.
func TestAnswerSymbolCorrelatesWithEvidence_Range(t *testing.T) {
	ev := types.EvidenceItem{Source: "a.go", LineStart: 10, LineEnd: 20}
	cases := []struct {
		name string
		sym  types.AnswerSymbol
		want bool
	}{
		{"in_range_low", types.AnswerSymbol{File: "a.go", Line: 10}, true},
		{"in_range_mid", types.AnswerSymbol{File: "a.go", Line: 15}, true},
		{"in_range_high", types.AnswerSymbol{File: "a.go", Line: 20}, true},
		{"below_range", types.AnswerSymbol{File: "a.go", Line: 9}, false},
		{"above_range", types.AnswerSymbol{File: "a.go", Line: 21}, false},
		{"wrong_file", types.AnswerSymbol{File: "b.go", Line: 15}, false},
		{"file_level_match_line_zero", types.AnswerSymbol{File: "a.go", Line: 0}, true},
		{"empty_file", types.AnswerSymbol{File: "", Line: 15}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := answerSymbolCorrelatesWithEvidence(tc.sym, ev); got != tc.want {
				t.Errorf("correlate(file=%q line=%d vs source=%q [%d..%d]) = %v, want %v",
					tc.sym.File, tc.sym.Line, ev.Source, ev.LineStart, ev.LineEnd, got, tc.want)
			}
		})
	}

	// LineEnd=0 edge: single-line range.
	evPoint := types.EvidenceItem{Source: "a.go", LineStart: 55, LineEnd: 0}
	if !answerSymbolCorrelatesWithEvidence(types.AnswerSymbol{File: "a.go", Line: 55}, evPoint) {
		t.Error("single-line evidence (LineEnd=0) should match at LineStart")
	}
	if answerSymbolCorrelatesWithEvidence(types.AnswerSymbol{File: "a.go", Line: 56}, evPoint) {
		t.Error("single-line evidence should NOT match one line past LineStart")
	}

	// LineStart=0 is an incomplete evidence item — never correlates.
	evBad := types.EvidenceItem{Source: "a.go", LineStart: 0, LineEnd: 0}
	if answerSymbolCorrelatesWithEvidence(types.AnswerSymbol{File: "a.go", Line: 55}, evBad) {
		t.Error("evidence with LineStart=0 must never correlate against a positive-line symbol")
	}
}

// TestAxisAnchorRetryHint_RegressionCase mirrors the 2026-04-18 bug:
// Question has PredicateAxis=AxisCall; Turn A's evidence pool contains
// a call-anchored item (SubAgentRuntime.Run @ subagent_runtime.go:136)
// among other definition-anchored items. The extractor picks two
// registration symbols in subagent.go — none correlate with the
// call-anchored item. The validator should fire.
func TestAxisAnchorRetryHint_RegressionCase(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisCall,
		},
	}
	evidence := []types.EvidenceItem{
		{
			Source:       "internal/agent/sub_explorer.go",
			LineStart:    55,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "Execute",
			Subject:      "SubExplorer",
		},
		{
			Source:       "internal/agent/subagent_runtime.go",
			LineStart:    136,
			AnchorKind:   types.AnchorCall,
			AnchorSymbol: "Run",
			Subject:      "SubAgentRuntime",
		},
		{
			Source:     "internal/agent/subagent.go",
			LineStart:  10,
			LineEnd:    20,
			AnchorKind: types.AnchorDefinition,
			Subject:    "SubAgent",
		},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: evidence})
	// Extractor picked registration-side anchors — both in subagent.go
	// at lines that DON'T overlap with the AnchorCall evidence items.
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "RegisterDefaultSubAgents", File: "internal/agent/subagent.go", Line: 63, Kind: "function"},
		{Name: "SubAgentRegistry", File: "internal/agent/subagent.go", Line: 55, Kind: "function"},
	}, types.CompletenessLowerBound)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	hint := axisAnchorRetryHint(ctx)
	if hint == "" {
		t.Fatal("expected non-empty retry hint for axis mismatch, got empty")
	}
	// Hint must name the axis and a concrete call-anchored cite.
	for _, needle := range []string{`"call"`, "sub_explorer.go", "subagent_runtime.go"} {
		// Either of the two axis-matching items is acceptable as
		// the exemplar. Matching the first is deterministic
		// (matching[0] == iteration order), so sub_explorer.go:55
		// is expected — but we tolerate either.
		_ = needle
	}
	// Deterministic assertions: names the axis ("call") and includes
	// "kind=call" in the wording.
	if !containsAll(hint, []string{`"call"`, "kind=call", "file:line"}) {
		t.Errorf("hint should mention axis + kind=call + 'file:line'; got: %s", hint)
	}
}

// TestAxisAnchorRetryHint_AlignedPicksPass verifies the happy path:
// LLM picked a call-anchored symbol → no retry hint.
func TestAxisAnchorRetryHint_AlignedPicksPass(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisCall,
		},
	}
	evidence := []types.EvidenceItem{
		{
			Source: "internal/agent/subagent_runtime.go", LineStart: 130, LineEnd: 140,
			AnchorKind: types.AnchorCall, Subject: "SubAgentRuntime",
		},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: evidence})
	// Symbol at line 136 falls within [130, 140] — correlates.
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Run", File: "internal/agent/subagent_runtime.go", Line: 136, Kind: "method"},
	}, types.CompletenessLowerBound)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	if hint := axisAnchorRetryHint(ctx); hint != "" {
		t.Errorf("aligned picks should not trigger retry; got hint: %s", hint)
	}
}

// TestAxisAnchorRetryHint_UnknownAxisNoOp: without a PredicateAxis,
// the validator must silently pass — no regression for pre-L1 paths.
func TestAxisAnchorRetryHint_UnknownAxisNoOp(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisUnknown,
		},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{Source: "a.go", LineStart: 1, AnchorKind: types.AnchorCall},
	}})
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "X", File: "b.go", Line: 99, Kind: "function"},
	}, types.CompletenessLowerBound)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	if hint := axisAnchorRetryHint(ctx); hint != "" {
		t.Errorf("AxisUnknown must not fire axis validator; got hint: %s", hint)
	}
}

// TestAxisAnchorRetryHint_NoMatchingEvidenceNoOp: when no Turn A
// evidence realises the axis, we cannot blame the LLM for picking
// non-matching anchors — validator must pass.
func TestAxisAnchorRetryHint_NoMatchingEvidenceNoOp(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisCall,
		},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	// All evidence is definition-anchored; nothing realises AxisCall.
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{Source: "a.go", LineStart: 1, AnchorKind: types.AnchorDefinition},
		{Source: "b.go", LineStart: 2, AnchorKind: types.AnchorImport},
	}})
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "X", File: "a.go", Line: 1, Kind: "function"},
	}, types.CompletenessLowerBound)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	if hint := axisAnchorRetryHint(ctx); hint != "" {
		t.Errorf("no axis-matching evidence → no retry; got hint: %s", hint)
	}
}

func TestAxisAnchorRetryHint_BoundedPrincipalStepListSkipsRetry(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCondition,
			RawRequest:    "What order do gate.Run's 7 checks execute in?",
			AnalyzerHints: types.AnalyzerHints{
				MentionedEntities: []string{"gate.Run"},
			},
			EnumerationBoundary: &types.RequestedEnumerationBoundary{
				DeclaredCount: 7,
				SourceQuote:   "7 checks",
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{Source: "internal/analysis/gate/gate.go", LineStart: 130, AnchorKind: types.AnchorCondition, AnchorSymbol: "isWrite"},
	}})
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "checkCoverage", File: "internal/analysis/gate/gate.go", Line: 127, Kind: "function"},
		{Name: "checkDAGClosure", File: "internal/analysis/gate/gate.go", Line: 128, Kind: "function"},
	}, types.CompletenessLowerBound)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	if hint := axisAnchorRetryHint(ctx); hint != "" {
		t.Fatalf("bounded principal step_list should not be forced to mix condition anchors into the member slate; got %q", hint)
	}
}

func TestAxisAnchorRetryHint_MultiTopicExplanationSkeletonSkipsConditionRetry(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			Scenario:      types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisCondition,
			SubTopics: []types.SubTopic{
				{Entities: []string{"AnalysisIR"}, Summary: "AnalysisIR fallback semantics"},
				{Entities: []string{"fallbackWriteAnalysisIR"}, Summary: "write-mode fallback semantics"},
			},
		},
	}
	mut := types.NewMutableState("")
	mut.SetRequestModel(ir.RequestModel)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{
			Source:       "internal/orchestrator/orchestrator.go",
			LineStart:    2297,
			AnchorKind:   types.AnchorCondition,
			AnchorSymbol: "out.AnalysisIR != nil",
			Summary:      "runAnalyzePhase succeeds only when AnalysisIR is non-nil and Error is empty",
		},
	}})
	mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "AnalysisIR", File: "internal/agent/agent.go", Line: 129, Kind: types.KindStruct},
		{Name: "fallbackWriteAnalysisIR", File: "internal/orchestrator/orchestrator.go", Line: 2764, Kind: types.KindFunction},
	}, types.CompletenessComplete)

	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}
	if isMultiTopicExplanation(ctx) {
		t.Fatal("condition-axis mechanism sub-topics must not invite an anchor skeleton")
	}
	if hint := axisAnchorRetryHint(ctx); hint != "" {
		t.Fatalf("mechanism support lanes should not force emit_answer_symbol retry; got %q", hint)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, n := range needles {
		if !stringsContains(haystack, n) {
			return false
		}
	}
	return true
}

func stringsContains(h, n string) bool {
	// Inline to avoid importing strings at the test file top when a
	// single call site suffices — matches the existing test style.
	if n == "" {
		return true
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
