package tool

// P2.1 Session 1 Phase 2 — emit_answer_symbol structural tests.
//
// Mirrors emit_evidence_test.go in shape but adds three P2.1-specific
// invariants:
//
//   - line == 0 is REJECTED (Pattern 2 line-hallucination guard;
//     emit_evidence allows line == 0 because [DIRECT] tags can describe
//     file-wide facts, but answer symbols must always cite a concrete
//     definition site).
//
//   - file inside ctx.WorkDir is REJECTED (blob-file path leak gate,
//     UNRESOLVED bug N=1; see memory/project_blob_file_leak_unresolved.md).
//
//   - kind enum is closed; aliases (func/function, struct/type/class)
//     normalize to canonical forms.

import (
	"encoding/json"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newAnswerSymbolCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func TestEmitAnswerSymbol_AcceptsValidBatch(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "ExplorerAgent", "file": "internal/agent/explorer.go", "line": 23, "kind": "type"},
          {"name": "NewExplorerAgent", "file": "internal/agent/explorer.go", "line": 50, "kind": "func", "chain": "Register binds NewExplorerAgent → Name() returns explorer", "rationale": "named explorer"}
        ],
        "completeness": "complete"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	if claim != types.CompletenessComplete {
		t.Errorf("completeness claim not stored: got %q, want complete", claim)
	}
	if got[0].Name != "ExplorerAgent" || got[0].Line != 23 || got[0].Kind != "type" {
		t.Errorf("first item not preserved: %+v", got[0])
	}
	if got[1].Kind != "function" {
		t.Errorf("kind alias 'func' should normalize to 'function', got %q", got[1].Kind)
	}
	if got[1].Chain == "" || got[1].Rationale == "" {
		t.Errorf("optional fields dropped: chain=%q rationale=%q", got[1].Chain, got[1].Rationale)
	}
}

func TestEmitAnswerSymbol_StructuredPayloadCompatRepairsStringItemsAndCount(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
		"items": "[{\"name\":\"ExplorerAgent\",\"file\":\"internal/agent/explorer.go\",\"line\":\"23\",\"kind\":\"type\"}]",
		"completeness": "\"complete\"",
		"count": "1"
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 || got[0].Line != 23 || got[0].Name != "ExplorerAgent" {
		t.Fatalf("answer symbol item was not preserved after compat repair: %+v", got)
	}
	if claim != types.CompletenessComplete {
		t.Fatalf("completeness claim = %q, want complete", claim)
	}
}

func TestEmitAnswerSymbol_MaterializesSourceInventoryAggregateSlate(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.Language = "zh"
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &types.CompletenessObligation{
			Required:    true,
			SourceQuote: "all public string enum types",
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
			RequiresConstSet:  true,
			Confidence:        0.95,
		},
	}}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "internal/types public string enum types",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Provenance:  "system:source_inventory",
		Members:     []string{"Intent", "Scenario"},
		SupportRefs: []string{"Intent: internal/types/analysis_ir.go:847", "Scenario: internal/types/analysis_ir.go:867"},
	}})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	params := json.RawMessage(`{
        "items": [],
        "completeness": "unknown"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected deterministic materialization success, got: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if claim != types.CompletenessComplete {
		t.Fatalf("claim = %q, want complete", claim)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 materialized symbols, got %+v", got)
	}
	if got[0].Name != "Intent" || got[0].File != "internal/types/analysis_ir.go" || got[0].Line != 847 || got[0].Kind != types.KindType {
		t.Fatalf("first materialized symbol mismatch: %+v", got[0])
	}
	if !strings.Contains(res.Summary, "materialized 2 answer-symbol") {
		t.Fatalf("summary should disclose deterministic materialization, got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_CanonicalizesDocCommentLineToDefinition(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	seedReadFileHistory(ctx, "internal/tool/tool.go", 11,
		"// Tool defines the interface for all local tools.",
		"//",
		"// IsWrite reports whether the tool's primary purpose is to mutate the",
		"// filesystem. It backs the requires_write permission boundary: read-only",
		"// agents must not be granted access to tools that return true here.",
		"// Implementations should embed ReadOnly or WriteCapable to satisfy this",
		"// method without boilerplate.",
		"type Tool interface {",
		"    Name() string",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"name": "Tool", "file": "internal/tool/tool.go", "line": 11, "kind": "interface"}
        ],
        "completeness": "complete"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Line != 18 {
		t.Fatalf("line = %d, want canonical definition line 18", got[0].Line)
	}
}

// TestEmitAnswerSymbol_CountFieldMatch pins commit 49 read-
// mode Gap 2: when LLM emits a self-declared count, the
// validator rejects on len(items) mismatch with hint to
// either trim or recount. Closes the gap where prose
// summary said "found 47" while items[] held 12.
func TestEmitAnswerSymbol_CountFieldMatch(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// Match: count=2, items=2 → success.
	params := json.RawMessage(`{
        "items": [
          {"name": "A", "file": "a.go", "line": 1, "kind": "function"},
          {"name": "B", "file": "b.go", "line": 1, "kind": "function"}
        ],
        "completeness": "complete",
        "count": 2
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("count=2 items=2 should succeed; got: %s", res.Summary)
	}
}

// TestEmitAnswerSymbol_CountFieldMismatch rejects mismatch
// loudly with an actionable hint.
func TestEmitAnswerSymbol_CountFieldMismatch(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// Mismatch: count=47, items=2 → reject.
	params := json.RawMessage(`{
        "items": [
          {"name": "A", "file": "a.go", "line": 1, "kind": "function"},
          {"name": "B", "file": "b.go", "line": 1, "kind": "function"}
        ],
        "completeness": "lower_bound",
        "count": 47
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("count=47 vs items=2 should reject")
	}
	if !strings.Contains(res.Summary, "47") || !strings.Contains(res.Summary, "2") {
		t.Errorf("rejection should name both numbers; got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "revise") {
		t.Errorf("rejection should hint at fix; got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_IgnoresUnexpectedSlateWhenSupportLaneIsNonSymbol(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all internal imports",
			},
		},
	}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal import packages",
		Value:   "2",
		Members: []string{"@acme/internal/foo", "@acme/internal/bar"},
	}})
	ctx.Mutable.SetInvestigationComplete("import member set complete")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "import-foo",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "src/main.ts",
			LineStart:       3,
			AnchorKind:      types.AnchorImport,
			AnchorSymbol:    "@acme/internal/foo",
			SurfaceTerms:    []string{"@acme/internal/foo"},
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "import-bar",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "src/main.ts",
			LineStart:       4,
			AnchorKind:      types.AnchorImport,
			AnchorSymbol:    "@acme/internal/bar",
			SurfaceTerms:    []string{"@acme/internal/bar"},
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	params := json.RawMessage(`{
		"items": [],
		"completeness": "unknown",
		"count": 2
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unexpected non-symbol support-lane slate should no-op, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored") {
		t.Fatalf("summary should make the no-op explicit, got: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 0 || claim != types.CompletenessUnknown {
		t.Fatalf("unexpected answer-symbol call must not populate the slate, got symbols=%+v claim=%q", got, claim)
	}
}

// TestEmitAnswerSymbol_NoCountIsBackcompat pins back-compat:
// pre-commit-49 calls without count field still work.
func TestEmitAnswerSymbol_NoCountIsBackcompat(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "A", "file": "a.go", "line": 1, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Errorf("no count = no validation = should succeed; got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsLineZero(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":0,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success")
	}
	if !strings.Contains(res.Summary, "line must be > 0") {
		t.Errorf("expected line guard diagnosis, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "Pattern 2") {
		t.Errorf("expected Pattern 2 attribution in error, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsLineNegative(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":-5,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure on negative line")
	}
}

// Session-22 follow-up: when an external-source log is attached, a
// line=0 item must reject with a redirect that names
// symbols_completeness=unknown as the proper escape — not the bland
// "line must be > 0" message that historically sent the LLM into a
// retry loop (2026-04-21 logtri_custom trace).
func TestEmitAnswerSymbol_RejectsLineZero_ExternalLogRedirectsToUnknown(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// Publish an external-source bundle: errors present, zero
	// resolved files → IsExternalSource() returns true.
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalDB}},
		Errors: []types.LogError{
			{Type: "pool_exhausted"},
		},
	})
	params := json.RawMessage(`{"items":[{"name":"pool_exhausted","file":"a.go","line":0,"kind":"literal"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("line=0 on external-source log must be rejected; got Success=true Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "symbols_completeness=\"unknown\"") {
		t.Errorf("reject message must redirect to completeness=unknown; got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "external-source") {
		t.Errorf("reject message must name the external-source cause; got %q", res.Summary)
	}
}

// Companion: line=0 with NO log bundle attached falls through to the
// historical bland line-hallucination message — the redirect is
// scoped to the external-source case where unknown completeness is
// semantically correct.
func TestEmitAnswerSymbol_RejectsLineZero_NoBundleUsesHistoricalMessage(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx() // no SetLogTriage call
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":0,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("line=0 must reject; got Success=true")
	}
	if strings.Contains(res.Summary, "external-source") {
		t.Errorf("no-bundle path must not invoke external-source redirect; got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "line must be > 0") {
		t.Errorf("historical line-guard message missing; got %q", res.Summary)
	}
}

// The happy path: when the log is external-source, the LLM sets
// completeness=unknown and omits items[] — the tool accepts (the
// partial-drop path's empty-items semantics for unknown
// completeness pre-dated this fix).
func TestEmitAnswerSymbol_ExternalSourceLog_UnknownEmptyItemsAccepted(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta:   types.LogMeta{Signals: []types.LogSignal{types.SignalDB}},
		Errors: []types.LogError{{Type: "pool_exhausted"}},
	})
	params := json.RawMessage(`{"items":[],"completeness":"unknown"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("external-source + completeness=unknown + empty items must be accepted; got Success=false Summary=%q", res.Summary)
	}
}

// IsExternalSource helper pin — used by multiple tool paths, must
// stay honest about the (no resolved files AND has errors) predicate.
func TestLogBundle_IsExternalSource(t *testing.T) {
	cases := []struct {
		name   string
		bundle *types.LogBundle
		want   bool
	}{
		{"nil bundle", nil, false},
		{"empty bundle", &types.LogBundle{}, false},
		{"errors present, no resolved", &types.LogBundle{
			Errors: []types.LogError{{Type: "X"}},
		}, true},
		{"errors + resolved → not external", &types.LogBundle{
			Errors:        []types.LogError{{Type: "X"}},
			ResolvedFiles: []string{"a.go"},
		}, false},
		{"resolved but no errors (degraded) → not external", &types.LogBundle{
			ResolvedFiles: []string{"a.go"},
		}, false},
	}
	for _, c := range cases {
		if got := c.bundle.IsExternalSource(); got != c.want {
			t.Errorf("%s: IsExternalSource()=%v want %v", c.name, got, c.want)
		}
	}
}

func TestEmitAnswerSymbol_PartialDropKeepsValidItems(t *testing.T) {
	// 2026-04-19 regression: one line=0 hallucination used to kill the
	// whole batch, leaving the finalizer with no symbols. Now the
	// invalid item is dropped and the valid one survives.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "Bad",  "file": "a.go", "line": 0,  "kind": "function"},
          {"name": "Good", "file": "a.go", "line": 42, "kind": "function"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected partial success, got failure: %s", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 || got[0].Name != "Good" {
		t.Fatalf("expected only valid item kept, got %d items: %+v", len(got), got)
	}
	if !strings.Contains(res.Summary, "dropped 1 invalid item") {
		t.Errorf("expected drop note in summary, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "line must be > 0") {
		t.Errorf("expected dropped-item reason in summary, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_PartialDropDowngradesCompleteClaim(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "Bad",  "file": "a.go", "line": 0,  "kind": "function"},
          {"name": "Good", "file": "a.go", "line": 42, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected partial success, got failure: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 || got[0].Name != "Good" {
		t.Fatalf("expected only valid item kept, got %d items: %+v", len(got), got)
	}
	if claim != types.CompletenessUnknown {
		t.Fatalf("partial structural drop must not retain completeness=complete, got %q", claim)
	}
	if !strings.Contains(res.Summary, "completeness downgraded from complete to unknown") {
		t.Fatalf("expected downgrade note in summary, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsBeyondRequestedEnumerationBoundary(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			EnumerationBoundary: &types.RequestedEnumerationBoundary{
				DeclaredCount: 2,
				SourceQuote:   "2 checks",
			},
		},
	}
	params := json.RawMessage(`{
        "items": [
          {"name": "checkCoverage", "file": "internal/analysis/gate/gate.go", "line": 127, "kind": "function"},
          {"name": "checkDAGClosure", "file": "internal/analysis/gate/gate.go", "line": 128, "kind": "function"},
          {"name": "checkBudgetSanity", "file": "internal/analysis/gate/gate.go", "line": 129, "kind": "function"}
        ],
        "completeness": "lower_bound"
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected boundary rejection for oversized answer-symbol slate")
	}
	if !strings.Contains(res.Summary, "bounded principal set") {
		t.Fatalf("reject summary must mention bounded principal set, got %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_AttributeBearingEnumerationRejectsLowerBoundWithTwoAxisHint(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			PredicateAxis: types.AxisDefine,
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{"aggregator", "compiler"},
			},
			EnumerationBoundary: &types.RequestedEnumerationBoundary{
				DeclaredCount: 2,
				SourceQuote:   "all packages",
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all packages",
			},
		},
	}
	params := json.RawMessage(`{
        "items": [
          {"name": "aggregator", "file": "internal/analysis/aggregator/aggregator.go", "line": 1, "kind": "package", "rationale": "entry function New at internal/analysis/aggregator/aggregator.go:112"}
        ],
        "completeness": "lower_bound"
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("lower_bound must reject for exhaustive attribute-bearing enumeration")
	}
	for _, want := range []string{
		"attribute-bearing enumeration",
		"principal members",
		"per-member attributes",
		"rationale",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("reject summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestEmitAnswerSymbol_ReusesCompiledStepCandidateNameAtSameLine(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "What order do gate.Run's 7 checks execute in?",
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
	ctx.EvidenceItems = []types.EvidenceItem{
		{Kind: types.EvidenceDirect, Source: "internal/analysis/gate/gate.go", LineStart: 135, AnchorKind: types.AnchorCall, AnchorSymbol: "checkContractComplete", Subject: "Run", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: "internal/analysis/gate/gate.go", LineStart: 136, AnchorKind: types.AnchorCall, AnchorSymbol: "checkHypothesisCoverage", Subject: "Run", GroundingStatus: types.GroundingGrounded},
		{Kind: types.EvidenceDirect, Source: "internal/analysis/gate/gate.go", LineStart: 144, AnchorKind: types.AnchorCall, AnchorSymbol: "checkSubtopicCoherence", Subject: "Run", GroundingStatus: types.GroundingGrounded},
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/analysis/gate/gate.go: showing lines 135-145 of 471 total]\n   135│ \t\tchecks = append(checks, checkContractComplete(ir, th))\n   136│ \t\tchecks = append(checks, checkHypothesisCoverage(ir, th))\n   137│ \t\t// Cross-signal coherence gates.\n   138│ \t\t// for the multi-topic / shape-vs-subject mis-classification\n   139│ \t\t// patterns the downstream explorer / extractor / finalizer\n   140│ \t\t// layers historically had to clean up after the fact.\n   141│ \t\t// purely structural (no keyword tables)\n   142│ \t\t// emitted IR fields against each other and against the\n   143│ \t\t// repomap-verified TermGraph domains.\n   144│ \t\tchecks = append(checks, checkSubtopicCoherence(ir))\n   145│ \t\tchecks = append(checks, checkShapeSubjectCoherence(ir))\n",
	})
	params := json.RawMessage(`{
        "items": [
          {"name": "checkResourceCount", "file": "internal/analysis/gate/gate.go", "line": 135, "kind": "method"},
          {"name": "checkOutputValueCount", "file": "internal/analysis/gate/gate.go", "line": 136, "kind": "method"},
          {"name": "checkResourceAddressing", "file": "internal/analysis/gate/gate.go", "line": 144, "kind": "method"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success via compiled candidate reuse, got %q", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 3 {
		t.Fatalf("want 3 accepted symbols, got %d", len(got))
	}
	want := []string{"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("accepted symbol[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestEmitAnswerSymbol_RejectsBlobPath(t *testing.T) {
	// Blob-file path leak gate. ctx.WorkDir is the per-trace temp
	// directory where StoreBlob persists large tool outputs. If the
	// LLM mistakes a blob path for a repo path the answer cite will
	// resolve to a self-erasing tempdir entry.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = "/tmp/codrax-trace-abc123"
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "/tmp/codrax-trace-abc123/grep-xyz.txt", "line": 42, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on blob-file path")
	}
	if !strings.Contains(res.Summary, "lives inside the per-trace WorkDir") {
		t.Errorf("expected blob-leak diagnosis, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_AcceptsSiblingDirectory(t *testing.T) {
	// The blob check uses normalized prefix matching with a trailing
	// slash. A sibling directory whose name shares a prefix with the
	// WorkDir must NOT be rejected — this pins the prefix-match
	// boundary.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = "/tmp/codrax-trace-abc"
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "/tmp/codrax-trace-abc-sibling/repo/file.go", "line": 5, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sibling directory should be accepted, got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_AcceptsRepoPathWhenWorkDirEmpty(t *testing.T) {
	// Empty WorkDir = no blob staging configured (unit tests, CI).
	// The check must skip; otherwise every test would reject every
	// cite.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = ""
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "internal/agent/foo.go", "line": 10, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("expected success when WorkDir empty, got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsUnknownKind(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// `xyz` is not in types.AllAnswerSymbolKinds and not an alias —
	// must reject. (`trait`, `protocol`, `module`, etc. ARE in the
	// expanded cross-language taxonomy; a previous version of this
	// test used `trait` as the "unknown" example, which now passes
	// validation — the rename here mirrors the enum expansion.)
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"xyz"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown kind")
	}
	if !strings.Contains(res.Summary, "unknown kind") {
		t.Errorf("expected kind enum diagnosis, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsMissingName(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing name")
	}
}

func TestEmitAnswerSymbol_RejectsBareFilename(t *testing.T) {
	// looksLikePath requires '/' or '.'. A bare token without either
	// would also be rejected by emit_evidence; pin the same shape.
	// 'README' has neither.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"README","line":1,"kind":"const"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on non-path-shaped file")
	}
}

func TestEmitAnswerSymbol_RejectsUnknownFields(t *testing.T) {
	// DisallowUnknownFields parity with emit_evidence. A hallucinated
	// 'comment' field must fail loud rather than be silently ignored.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function","comment":"extra"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown field")
	}
}

// TestEmitAnswerSymbol_EmptySetSemantics pins the 2026-04-17 fix: an
// empty symbols[] is now a legitimate answer shape for "how many X"
// questions that turn out to be zero. Rules:
//
//	items=[] + completeness=complete     → ACCEPT (confirmed empty set)
//	items=[] + completeness=unknown      → ACCEPT (could not determine)
//	items=[] + completeness=lower_bound  → REJECT (nonsensical — claims
//	                                       MORE items exist beyond zero)
func TestEmitAnswerSymbol_EmptySetSemantics(t *testing.T) {
	cases := []struct {
		name         string
		completeness string
		wantSuccess  bool
	}{
		{"empty + complete accepts (confirmed zero)", "complete", true},
		{"empty + unknown accepts (undetermined)", "unknown", true},
		{"empty + lower_bound rejects (contradiction)", "lower_bound", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &EmitAnswerSymbol{}
			ctx := newAnswerSymbolCtx()
			params := json.RawMessage(`{"items":[],"completeness":"` + c.completeness + `"}`)
			res, _ := tool.Execute(ctx, params)
			if res.Success != c.wantSuccess {
				t.Errorf("success=%v want=%v (summary=%q)", res.Success, c.wantSuccess, res.Summary)
			}
		})
	}
}

func TestEmitAnswerSymbol_RequiresMutable(t *testing.T) {
	// Sub-agent dispatch path passes a BusContext with Mutable nil.
	// The tool must refuse rather than silently drop the writes.
	tool := &EmitAnswerSymbol{}
	ctx := &types.BusContext{}
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 9 — completeness claim schema tests
// -----------------------------------------------------------------------------

func TestEmitAnswerSymbol_MissingCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// items are fine, but no completeness — schema requires it
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("missing completeness must be rejected (required field)")
	}
	if !strings.Contains(res.Summary, "completeness") {
		t.Errorf("error must mention completeness, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_EmptyCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":""}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("empty completeness string must be rejected — silent coercion is the exact bug we are closing")
	}
}

func TestEmitAnswerSymbol_UnknownCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"definitely_complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown completeness enum value must be rejected")
	}
	if !strings.Contains(res.Summary, "unknown completeness") {
		t.Errorf("error must diagnose unknown value, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_CompletenessValues_AllAccepted(t *testing.T) {
	cases := []struct {
		raw    string
		stored types.CompletenessClaim
	}{
		{"complete", types.CompletenessComplete},
		{"lower_bound", types.CompletenessLowerBound},
		{"unknown", types.CompletenessUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			tool := &EmitAnswerSymbol{}
			ctx := newAnswerSymbolCtx()
			params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"` + tc.raw + `"}`)
			res, _ := tool.Execute(ctx, params)
			if !res.Success {
				t.Fatalf("completeness=%q should succeed, got: %s", tc.raw, res.Summary)
			}
			_, claim := ctx.Mutable.EmittedAnswerSymbols()
			if claim != tc.stored {
				t.Errorf("completeness=%q stored as %q, want %q", tc.raw, claim, tc.stored)
			}
			if !strings.Contains(res.Summary, "completeness="+string(tc.stored)) && !strings.Contains(res.Summary, "completeness=unknown") {
				// summary always includes the claim for audit
				t.Errorf("summary missing completeness tag: %q", res.Summary)
			}
		})
	}
}

func TestEmitAnswerSymbol_SecondCallReplaces(t *testing.T) {
	// Set semantics: a second Execute call REPLACES the first slate
	// rather than appending. This is the retry contract — on a
	// downgrade retry the LLM calls again with a different set, and
	// that second call must fully overwrite.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()

	first := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	if res, _ := tool.Execute(ctx, first); !res.Success {
		t.Fatalf("first call failed: %s", res.Summary)
	}

	second := json.RawMessage(`{"items":[{"name":"Bar","file":"b.go","line":2,"kind":"method"},{"name":"Baz","file":"c.go","line":3,"kind":"type"}],"completeness":"lower_bound"}`)
	if res, _ := tool.Execute(ctx, second); !res.Success {
		t.Fatalf("second call failed: %s", res.Summary)
	}

	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(syms) != 2 {
		t.Errorf("second call should replace with 2 items, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Bar" {
		t.Errorf("first item should be Bar (from second call), got %q", syms[0].Name)
	}
	if claim != types.CompletenessLowerBound {
		t.Errorf("claim should be lower_bound after second call, got %q", claim)
	}
}

// Sister test: emit_evidence now also enforces the blob-leak gate on
// its source field. Pin the parallel behavior so future refactors of
// the shared isInsideWorkDir helper cannot accidentally diverge the
// two channels.
func TestEmitEvidence_RejectsBlobSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.WorkDir = "/tmp/codrax-trace-xyz"
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "/tmp/codrax-trace-xyz/grep-out.txt", "line_start": 100}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("emit_evidence must also reject blob-staged source paths")
	}
	if !strings.Contains(res.Summary, "lives inside the per-trace WorkDir") {
		t.Errorf("expected blob-leak diagnosis, got: %q", res.Summary)
	}
}

// TestEmitAnswerSymbol_FloorGroundingDropsCallSiteLine pins the
// Fix D contract: when the cited file was read_file'd in this
// pipeline AND the claimed file:line (±2 window) does not contain
// the symbol name as a code identifier, the item is dropped before
// reaching the deterministic floor. Regression target was the u3a
// run-1 case where the extractor LLM read evidence bullets of the
// shape "[relationship] X calls Y — file:line" as "X is at line"
// and emitted symbols whose file:line pointed at the call-site
// line (Y's call inside X) rather than X's own definition; all 10
// downstream finalizer citations failed grounding because the line
// text contained Y's identifier, not X's.
func TestEmitAnswerSymbol_FloorGroundingDropsCallSiteLine(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// Simulate a prior read_file on explorer.go: line 992 is the call
	// site of canonicalExplorerPath inside focusedAnchorNeighborhood
	// (which is defined at line 985 in real source). The line text
	// contains "canonicalExplorerPath", NOT "focusedAnchorNeighborhood".
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/explorer.go: showing lines 990-994 of 8000 total]\n   990│ \n   991│ func add(path string) {\n   992│ \tpath = canonicalExplorerPath(path)\n   993│ \tif path == \"\" {\n   994│ \t\treturn\n",
	})
	params := json.RawMessage(`{
        "items": [
          {"name": "explorerEvaluator.focusedAnchorNeighborhood", "file": "internal/agent/explorer.go", "line": 992, "kind": "method"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("call-site-line item should be dropped; got Success=true Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "not corroborated") {
		t.Errorf("rejection should explain via the literal-grounding contract phrase; got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "call site") {
		t.Errorf("rejection should mention the call-site mis-read failure mode; got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_FloorGroundingRepairsWrongLineFromGroundedEvidence(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/subagent_runtime.go: showing lines 60-64 of 300 total]\n    60│ func validateRequest(req Request) error {\n    61│ \tif req.Name == \"\" {\n    62│ \t\treturn fmt.Errorf(\"missing name\")\n    63│ \t}\n    64│ \treturn nil\n",
	})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "subagent-validator-def",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/subagent_runtime.go",
		LineStart:       14,
		LineEnd:         14,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "SubAgentValidator",
		GroundingStatus: types.GroundingGrounded,
	}})
	params := json.RawMessage(`{
		"items": [
			{"name": "SubAgentValidator", "file": "internal/agent/subagent_runtime.go", "line": 62, "kind": "type"}
		],
		"completeness": "lower_bound"
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grounded evidence should repair wrong cited line, got Summary=%q", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %+v", got)
	}
	if got[0].Line != 14 {
		t.Fatalf("line=%d, want grounded definition line 14", got[0].Line)
	}
	if !strings.Contains(res.Summary, "auto-canonicalized") {
		t.Fatalf("summary should disclose local canonicalization, got: %s", res.Summary)
	}
}

// TestEmitAnswerSymbol_FloorGroundingAcceptsRealDefinition is the
// positive companion: when the cited line genuinely contains the
// symbol's identifier, Fix D lets the item through.
func TestEmitAnswerSymbol_FloorGroundingAcceptsRealDefinition(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/explorer.go: showing lines 983-987 of 8000 total]\n   983│ \n   984│ // focusedAnchorNeighborhood returns the focused subjects.\n   985│ func (e *explorerEvaluator) focusedAnchorNeighborhood() map[string]bool {\n   986│ \treturn e.focused\n   987│ }\n",
	})
	params := json.RawMessage(`{
        "items": [
          {"name": "explorerEvaluator.focusedAnchorNeighborhood", "file": "internal/agent/explorer.go", "line": 985, "kind": "method"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("real-definition item should be accepted; got Success=false Summary=%q", res.Summary)
	}
}

func TestEmitAnswerSymbol_FloorGroundingDoesNotRejectUnreadLineWindow(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	seedReadFileHistory(ctx, "internal/agent/explorer.go", 30,
		"type explorerEvaluator struct {",
		"\theuristics types.ExploreHeuristics",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"name": "NewExplorerAgent", "file": "internal/agent/explorer.go", "line": 15194, "kind": "function"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unread line window should not be treated as a contradiction; got Summary=%q", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 || got[0].Line != 15194 {
		t.Fatalf("symbol should be preserved for downstream grounding, got %+v", got)
	}
}

func TestEmitAnswerSymbol_FloorGroundingCanonicalizesUnreadLineFromGraph(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	seedReadFileHistory(ctx, "internal/agent/explorer.go", 30,
		"type explorerEvaluator struct {",
		"\theuristics types.ExploreHeuristics",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/explorer.go": {
				RelPath: "internal/agent/explorer.go",
				Symbols: []repomap.Symbol{{
					Name:    "NewExplorerAgent",
					Kind:    "function",
					File:    "internal/agent/explorer.go",
					Line:    15194,
					EndLine: 15199,
				}},
			},
		},
		SymbolDefs: map[string][]*repomap.Symbol{},
	})
	params := json.RawMessage(`{
        "items": [
          {"name": "NewExplorerAgent", "file": "internal/agent/explorer.go", "line": 15195, "kind": "function"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("graph-backed unread line canonicalization should pass; got Summary=%q", res.Summary)
	}
	got, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 1 || got[0].Line != 15194 {
		t.Fatalf("line should canonicalize to graph definition line 15194, got %+v", got)
	}
}

// TestEmitAnswerSymbol_FloorGroundingSkippedWhenFileNotRead pins the
// "let it through when we cannot verify" half of Fix D: when the
// cited file was never read_file'd in this pipeline, we have no line
// text to verify against — the item passes the new gate (downstream
// finalizer grounding remains the second line of defence).
func TestEmitAnswerSymbol_FloorGroundingSkippedWhenFileNotRead(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "SomeFunction", "file": "internal/agent/never_read.go", "line": 42, "kind": "function"}
        ],
        "completeness": "lower_bound"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("item from never-read file should pass the floor gate; got Success=false Summary=%q", res.Summary)
	}
}

// isInsideWorkDir unit boundary tests — the structural prefix logic
// is load-bearing for both tools, so test it directly without going
// through Execute.
func TestIsInsideWorkDir_Boundary(t *testing.T) {
	cases := []struct {
		name     string
		filePath string
		workDir  string
		want     bool
	}{
		{"empty workdir", "internal/agent/foo.go", "", false},
		{"empty file", "", "/tmp/work", false},
		{"exact match", "/tmp/work", "/tmp/work", true},
		{"inside subdir", "/tmp/work/grep.txt", "/tmp/work", true},
		{"inside nested", "/tmp/work/sub/dir/file.go", "/tmp/work", true},
		{"sibling prefix", "/tmp/work-other/file.go", "/tmp/work", false},
		{"workdir trailing slash", "/tmp/work/x.txt", "/tmp/work/", true},
		{"workdir only slash", "x.txt", "/", false},
		{"unrelated path", "internal/agent/foo.go", "/tmp/work", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInsideWorkDir(tc.filePath, tc.workDir); got != tc.want {
				t.Errorf("isInsideWorkDir(%q, %q) = %v, want %v", tc.filePath, tc.workDir, got, tc.want)
			}
		})
	}
}
