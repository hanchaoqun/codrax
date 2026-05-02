package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── runIntentCoverageOracle ──────────────────────────────────────────

func TestIntentOracle_TraceShallow_FailsOnSingleStepNoDiagram(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentTrace}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: "X is called from Y.",
		Steps:   []types.AnswerStep{{Index: 1, Description: "step 1"}},
	}
	vs := runIntentCoverageOracle(doc, rm)
	if len(vs) != 1 || vs[0].Kind != types.ViolIntentTraceShallow {
		t.Fatalf("expected ViolIntentTraceShallow; got %+v", vs)
	}
}

func TestIntentOracle_TraceShallow_PassesOnTwoSteps(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentTrace}
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "step 1"},
			{Index: 2, Description: "step 2"},
		},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_TraceShallow_PassesOnSequenceDiagram(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentTrace}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: "trace below:\n```mermaid\nsequenceDiagram\n    A->>B: call\n    B->>C: call\n```\n",
		Steps:   []types.AnswerStep{{Index: 1, Description: "step 1"}},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_EnumerateNotList_FailsOnExplanation(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "There are several things.",
	}
	vs := runIntentCoverageOracle(doc, rm)
	if len(vs) != 1 || vs[0].Kind != types.ViolIntentEnumerateNotList {
		t.Fatalf("expected ViolIntentEnumerateNotList; got %+v", vs)
	}
}

func TestIntentOracle_EnumerateNotList_PassesOnListOfSymbols(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{{Name: "A"}, {Name: "B"}},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_EnumerateNotList_PassesOnStepListWithItems(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentEnumerate}
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "X"},
			{Index: 2, Description: "Y"},
		},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_RootCause_FailsOnNoCitations(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentRootCause}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "the bug is real",
	}
	vs := runIntentCoverageOracle(doc, rm)
	if len(vs) != 1 || vs[0].Kind != types.ViolIntentRootCauseNoCause {
		t.Fatalf("expected ViolIntentRootCauseNoCause; got %+v", vs)
	}
}

func TestIntentOracle_RootCause_PassesWithOneCitation(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentRootCause}
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   "cause anchored",
		Citations: []types.Citation{{File: "a.go", Line: 10}},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_ConfigQuery_FailsOnExplanationNoConfigCitation(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Summary:   "MaxConn is 5",
		Citations: []types.Citation{{File: "internal/foo.go", Line: 10}},
	}
	vs := runIntentCoverageOracle(doc, rm)
	if len(vs) != 1 || vs[0].Kind != types.ViolIntentConfigNoTrail {
		t.Fatalf("expected ViolIntentConfigNoTrail; got %+v", vs)
	}
}

func TestIntentOracle_ConfigQuery_PassesOnValueShape(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}
	doc := &types.AnswerDocument{
		Shape: types.ShapeValue,
		Value: &types.AnswerValue{Literal: "5"},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_ConfigQuery_PassesOnConfigCitation(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentConfigQuery}
	doc := &types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "codrax.yaml", Line: 5}},
	}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestIntentOracle_NonTargetIntent_NoOp(t *testing.T) {
	rm := &types.RequestModel{Intent: types.IntentExplain}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	if vs := runIntentCoverageOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected no-op for non-target Intent; got %+v", vs)
	}
}

// ── runSubjectAnchorOracle ────────────────────────────────────────────

func TestSubjectOracle_FailsWhenSubjectMissingFromAnswer(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			EntityAxes: []string{"DispatchHandler"},
			Confidence: 0.8,
		},
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "the system uses an unrelated function called Process.",
		Symbols: []types.AnswerSymbol{{Name: "Process"}},
	}
	vs := runSubjectAnchorOracle(doc, rm)
	if len(vs) != 1 || vs[0].Kind != types.ViolSubjectAnchorMissing {
		t.Fatalf("expected ViolSubjectAnchorMissing; got %+v", vs)
	}
}

func TestSubjectOracle_PassesWhenSubjectInSymbolName(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			EntityAxes: []string{"DispatchHandler"},
			Confidence: 0.8,
		},
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeListOfSymbols,
		Symbols: []types.AnswerSymbol{{Name: "DispatchHandler"}},
	}
	if vs := runSubjectAnchorOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestSubjectOracle_PassesWhenSubjectInSummaryInlineCode(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			EntityAxes: []string{"DispatchHandler"},
			Confidence: 0.8,
		},
	}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "the function `DispatchHandler` is invoked first.",
	}
	if vs := runSubjectAnchorOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestSubjectOracle_BelowFloor_NoOp(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			EntityAxes: []string{"X"},
			Confidence: 0.4, // below 0.6 floor
		},
	}
	doc := &types.AnswerDocument{}
	if vs := runSubjectAnchorOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected no-op below floor; got %+v", vs)
	}
}

func TestSubjectOracle_GenericSubjectKind_NoOp(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectGeneric,
			EntityAxes: []string{"X"},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocument{}
	if vs := runSubjectAnchorOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected no-op for generic kind; got %+v", vs)
	}
}

func TestSubjectOracle_NoEntityAxes_NoOp(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocument{}
	if vs := runSubjectAnchorOracle(doc, rm); len(vs) != 0 {
		t.Errorf("expected no-op without entity axes; got %+v", vs)
	}
}

// ── runPredicateAxisOracle ────────────────────────────────────────────

func TestPredicateAxisOracle_FailsWhenNoMatchingAnchor(t *testing.T) {
	rm := &types.RequestModel{PredicateAxis: types.AxisCall}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	mut := types.NewMutableState("any")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{Source: "a.go", LineStart: 1, AnchorKind: types.AnchorDefinition},
		},
	})
	vs := runPredicateAxisOracle(doc, rm, mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolPredicateAxisMissing {
		t.Fatalf("expected ViolPredicateAxisMissing; got %+v", vs)
	}
}

func TestPredicateAxisOracle_PassesWhenMatchingAnchorExists(t *testing.T) {
	rm := &types.RequestModel{PredicateAxis: types.AxisCall}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	mut := types.NewMutableState("any")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{Source: "a.go", LineStart: 5, AnchorKind: types.AnchorCall},
		},
	})
	if vs := runPredicateAxisOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestPredicateAxisOracle_AxisRegisterAcceptsAssignment(t *testing.T) {
	rm := &types.RequestModel{PredicateAxis: types.AxisRegister}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation}
	mut := types.NewMutableState("any")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{Source: "a.go", LineStart: 5, AnchorKind: types.AnchorAssignment},
		},
	})
	if vs := runPredicateAxisOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestPredicateAxisOracle_UnknownAxis_NoOp(t *testing.T) {
	rm := &types.RequestModel{PredicateAxis: types.AxisUnknown}
	doc := &types.AnswerDocument{}
	mut := types.NewMutableState("any")
	if vs := runPredicateAxisOracle(doc, rm, mut); len(vs) != 0 {
		t.Errorf("expected no-op for unknown axis; got %+v", vs)
	}
}

// ── helper: hasSequenceDiagram ────────────────────────────────────

func TestHasSequenceDiagram_DetectsFenced(t *testing.T) {
	cases := map[string]bool{
		"```mermaid\nsequenceDiagram\nA->>B\n```":             true,
		"text\n```mermaid\nsequenceDiagram\nA->>B\n```\nmore": true,
		"```mermaid\nflowchart TD\nA-->B\n```":                false,
		"":                  false,
		"sequenceDiagram":   false, // no fence
		"```mermaid\nfoo``": false, // no closing fence
	}
	for in, want := range cases {
		if got := hasSequenceDiagram(in); got != want {
			t.Errorf("hasSequenceDiagram(%q) = %v, want %v", in, got, want)
		}
	}
}

// ── helper: isConfigShapedPath ────────────────────────────────────

func TestIsConfigShapedPath_KnownExtensions(t *testing.T) {
	cases := map[string]bool{
		"codrax.yaml":     true,
		"x/y.yml":         true,
		"foo.json":        true,
		"foo.json5":       true,
		"a/b.toml":        true,
		"settings.ini":    true,
		"app.conf":        true,
		"app.cfg":         true,
		".env":            true,
		"path/to/.envrc":  true,
		"a/.bashrc":       true,
		"internal/foo.go": false,
		"main.py":         false,
		"":                false,
	}
	for in, want := range cases {
		if got := isConfigShapedPath(in); got != want {
			t.Errorf("isConfigShapedPath(%q) = %v, want %v", in, got, want)
		}
	}
}

// ── helper: extractInlineCodeTokens ───────────────────────────────

func TestExtractInlineCodeTokens_HandlesFencesAndInline(t *testing.T) {
	src := "this `Foo` and `Bar` then ```\nfenced\n``` and `Baz`."
	got := extractInlineCodeTokens(src)
	want := []string{"Foo", "Bar", "Baz"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
