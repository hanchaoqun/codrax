package tool

import (
	"encoding/json"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// newDocGraph builds a minimal repomap.Graph indexing the given files
// and symbols. Used by the Tier-1-peer pool-defence tests.
func newDocGraph(files map[string][]docSymbol) *repomap.Graph {
	g := &repomap.Graph{FileIndex: make(map[string]*repomap.FileInfo)}
	for path, syms := range files {
		fi := &repomap.FileInfo{RelPath: path}
		for _, s := range syms {
			fi.Symbols = append(fi.Symbols, repomap.Symbol{
				Name: s.name, Kind: "function", Line: s.line, EndLine: s.end,
			})
		}
		g.FileIndex[path] = fi
	}
	return g
}

// newDocBusCtx returns a BusContext wired with a fresh MutableState so
// emit_answer_document can land its Set call. WorkDir is deliberately
// left unset by default; tests that exercise the blob-leak guard pass
// it explicitly.
func newDocBusCtx(workDir string) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState(""),
		WorkDir: workDir,
	}
}

func mustDocJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestEmitAnswerDocument_RejectsMissingShape — the schema declares
// shape as required; a call without shape is rejected at decode time.
func TestEmitAnswerDocument_RejectsMissingShape(t *testing.T) {
	tool := &EmitAnswerDocument{}
	res, _ := tool.Execute(newDocBusCtx(""), json.RawMessage(`{"summary": "hello"}`))
	if res.Success {
		t.Error("missing shape: Success = true, want false")
	}
	if !strings.Contains(res.Summary, "shape") {
		t.Errorf("missing shape: error message does not mention shape: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsUnknownShape(t *testing.T) {
	tool := &EmitAnswerDocument{}
	res, _ := tool.Execute(newDocBusCtx(""), json.RawMessage(`{"shape": "not_a_shape"}`))
	if res.Success {
		t.Error("unknown shape: Success = true, want false")
	}
}

func TestEmitAnswerDocument_RejectsUnknownField(t *testing.T) {
	tool := &EmitAnswerDocument{}
	res, _ := tool.Execute(newDocBusCtx(""), json.RawMessage(`{"shape": "explanation", "summary": "hi", "mystery_field": 1}`))
	if res.Success {
		t.Error("unknown field accepted; expected DisallowUnknownFields reject")
	}
}

// -------- list_of_symbols --------

func TestEmitAnswerDocument_ListOfSymbols_Happy(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "list_of_symbols",
		"symbols": []map[string]interface{}{
			{"name": "Foo", "file": "a.go", "line": 10, "kind": "function"},
			{"name": "Bar", "file": "b.go", "line": 20, "kind": "method"},
		},
		"symbols_completeness": "lower_bound",
		"citations":            []map[string]interface{}{{"file": "a.go", "line": 10}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("list_of_symbols happy path: Success = false, Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc == nil || doc.Shape != types.ShapeListOfSymbols {
		t.Fatalf("document not stored: %+v", doc)
	}
	if len(doc.Symbols) != 2 {
		t.Errorf("symbols len = %d, want 2", len(doc.Symbols))
	}
	if doc.SymbolsCompleteness != types.CompletenessLowerBound {
		t.Errorf("completeness = %q, want lower_bound", doc.SymbolsCompleteness)
	}
}

func TestEmitAnswerDocument_ListOfSymbols_RequiresCompleteness(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "list_of_symbols",
		"symbols": []map[string]interface{}{
			{"name": "Foo", "file": "a.go", "line": 10, "kind": "function"},
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("missing symbols_completeness: Success = true, want false")
	}
}

func TestEmitAnswerDocument_ListOfSymbols_RejectsStepsField(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"symbols":              []map[string]interface{}{{"name": "Foo", "file": "a.go", "line": 1, "kind": "function"}},
		"symbols_completeness": "complete",
		"steps":                []map[string]interface{}{{"index": 1, "description": "x", "citation_ref": -1}},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	// 2026-04-17 hardening: non-zero forbidden fields reject the call
	// with a structured error so the LLM sees the feedback on the
	// next turn. Silent scrub was letting the LLM keep the wrong
	// field across multiple iterations because no signal surfaced.
	if res.Success {
		t.Errorf("list_of_symbols with non-zero steps: must reject, got success")
	}
	if !strings.Contains(res.Summary, "forbids steps") {
		t.Errorf("rejection must name the offending field: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_ListOfSymbols_TolerateZeroStepsArray — an
// empty steps array is semantically absent; the LLM's JSON
// serializer commonly emits it even when the shape forbids the
// field. Tolerate zero-length; reject non-zero.
func TestEmitAnswerDocument_ListOfSymbols_TolerateZeroStepsArray(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"symbols":              []map[string]interface{}{{"name": "Foo", "file": "a.go", "line": 1, "kind": "function"}},
		"symbols_completeness": "complete",
		"steps":                []map[string]interface{}{},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if !res.Success {
		t.Errorf("empty steps[] is semantically absent and must not reject: %s", res.Summary)
	}
}

// TestEmitAnswerDocument_Explanation_RejectsZombieBoolean pins the
// exact failure mode from trace 1776439797257469553: the LLM sent
// shape=explanation with a non-zero boolean{decision:"否", rationale:
// "…"}. Old behaviour silently scrubbed it and accepted; new
// behaviour rejects so the LLM corrects on retry.
func TestEmitAnswerDocument_Explanation_RejectsZombieBoolean(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"boolean": map[string]interface{}{"decision": "否", "rationale": "spurious", "citation_ref": -1},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Errorf("explanation with non-zero boolean: must reject, got success")
	}
	if !strings.Contains(res.Summary, "forbids boolean") {
		t.Errorf("rejection must name boolean: %q", res.Summary)
	}
}

// -------- step_list --------

func TestEmitAnswerDocument_StepList_Happy(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "Three distinct branches.",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "First branch", "citation_ref": 0},
			{"index": 2, "description": "Second branch", "citation_ref": 1},
			{"index": 3, "description": "Third branch", "citation_ref": -1},
		},
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 10},
			{"file": "b.go", "line": 20},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("step_list happy: Success = false, Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Steps) != 3 {
		t.Errorf("steps len = %d, want 3", len(doc.Steps))
	}
	if doc.Steps[2].CitationRef != types.CitationRefUnset {
		t.Errorf("step 3 CitationRef = %d, want -1", doc.Steps[2].CitationRef)
	}
}

func TestEmitAnswerDocument_StepList_RejectsOutOfRangeCitationRef(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "step_list",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "x", "citation_ref": 5},
		},
		"citations": []map[string]interface{}{{"file": "a.go", "line": 1}},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("out-of-range citation_ref: Success = true, want false")
	}
}

func TestEmitAnswerDocument_StepList_RejectsNegativeIndex(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "step_list",
		"steps": []map[string]interface{}{
			{"index": 0, "description": "x", "citation_ref": -1},
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("zero index: Success = true, want false")
	}
}

// -------- value + config_value --------

func TestEmitAnswerDocument_Value_Happy(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{
			"literal":      "explorer",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "a.go", "line": 42}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("value happy: Success = false, Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Value == nil || doc.Value.Literal != "explorer" {
		t.Errorf("value mismatch: %+v", doc.Value)
	}
}

func TestEmitAnswerDocument_ConfigValue_RequiresKey(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "config_value",
		"value": map[string]interface{}{
			"literal":      "explorer",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("config_value without key: Success = true, want false")
	}
}

// -------- boolean --------

func TestEmitAnswerDocument_Boolean_Happy(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "boolean",
		"boolean": map[string]interface{}{
			"decision":     "yes",
			"rationale":    "because the evidence at foo.go:1 confirms it",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "foo.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("boolean happy: Success = false, Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Boolean == nil || doc.Boolean.Decision != true {
		t.Errorf("boolean decision: %+v", doc.Boolean)
	}
}

func TestEmitAnswerDocument_Boolean_RejectsHedge(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "boolean",
		"boolean": map[string]interface{}{
			"decision":     "maybe",
			"rationale":    "r",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("boolean decision='maybe': Success = true, want false")
	}
}

// -------- explanation --------

func TestEmitAnswerDocument_Explanation_RequiresSummary(t *testing.T) {
	tool := &EmitAnswerDocument{}
	res, _ := tool.Execute(newDocBusCtx(""), json.RawMessage(`{"shape": "explanation"}`))
	if res.Success {
		t.Error("explanation without summary: Success = true, want false")
	}
}

func TestEmitAnswerDocument_Explanation_Happy(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "The codrax architecture decomposes user tasks through a 6-layer pipeline.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("explanation happy: Success = false, Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Shape != types.ShapeExplanation || doc.Summary == "" {
		t.Errorf("explanation round-trip: %+v", doc)
	}
}

// -------- cross-cutting validators --------

func TestEmitAnswerDocument_RejectsSummaryOverCap(t *testing.T) {
	// 2026-04-17: summary cap is now per-shape (SummaryCapByShape).
	// Exercise all three tiers so a future shape addition that forgets
	// to populate the map gets caught here (via the default fallback).
	cases := []struct {
		shape types.AnswerShape
		cap   int
		extra map[string]interface{}
	}{
		{types.ShapeExplanation, types.SummaryCapFor(types.ShapeExplanation), nil},
		{types.ShapeValue, types.SummaryCapFor(types.ShapeValue), map[string]interface{}{
			"value": map[string]interface{}{"literal": "x", "citation_ref": -1},
		}},
		{types.ShapeBoolean, types.SummaryCapFor(types.ShapeBoolean), map[string]interface{}{
			"boolean": map[string]interface{}{"decision": "true", "rationale": "r", "citation_ref": -1},
		}},
	}
	for _, tc := range cases {
		payload := map[string]interface{}{
			"shape":   string(tc.shape),
			"summary": strings.Repeat("a", tc.cap+1),
		}
		for k, v := range tc.extra {
			payload[k] = v
		}
		tool := &EmitAnswerDocument{}
		res, _ := tool.Execute(newDocBusCtx(""), mustDocJSON(t, payload))
		if res.Success {
			t.Errorf("shape=%s with summary %d (cap %d): must reject, got success", tc.shape, tc.cap+1, tc.cap)
		}
	}
}

// TestEmitAnswerDocument_AcceptsLongSummaryOnExplanation locks in the
// 2026-04-17 change: explanation shape now accepts summaries up to
// 2500 chars (multi-paragraph answer body). The old 500-char cap
// forced finalizers to burn retries trimming legitimate prose.
func TestEmitAnswerDocument_AcceptsLongSummaryOnExplanation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	// 1500 chars: above the historical 500 cap, well under the new
	// 2500 cap. Proves the explanation path uses the generous cap.
	longEnough := strings.Repeat("abcde ", 300) // 1800 chars
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": longEnough,
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if !res.Success {
		t.Errorf("1800-char explanation summary: must accept (cap=%d), got rejection: %s",
			types.SummaryCapFor(types.ShapeExplanation), res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsCitationInWorkDir(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "/tmp/codrax-trace-abc/blob.txt", "line": 1},
		},
	})
	res, _ := tool.Execute(newDocBusCtx("/tmp/codrax-trace-abc"), params)
	if res.Success {
		t.Error("blob-path citation: Success = true, want false (blob leak gate)")
	}
}

func TestEmitAnswerDocument_RejectsZeroLineCitation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 0},
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("line=0 citation: Success = true, want false (line-hallucination guard)")
	}
}

func TestEmitAnswerDocument_RejectsQuoteOverCap(t *testing.T) {
	tool := &EmitAnswerDocument{}
	longQuote := strings.Repeat("x", types.CitationMaxQuoteChars+1)
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 1, "quote": longQuote},
		},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("over-cap quote: Success = true, want false")
	}
}

func TestEmitAnswerDocument_ZeroIsValidCitationRef(t *testing.T) {
	// Critical sentinel test: CitationRef=0 is a VALID pool index
	// (the first citation), distinct from CitationRef=-1 (no citation).
	// This is the foot-gun the sentinel design is meant to catch.
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{
			"literal":      "42",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "a.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Errorf("citation_ref=0: Success = false, Summary=%q", res.Summary)
	}
	if ctx.Mutable.AnswerDocument().Value.CitationRef != 0 {
		t.Errorf("CitationRef round-trip: got %d, want 0", ctx.Mutable.AnswerDocument().Value.CitationRef)
	}
}

func TestEmitAnswerDocument_Replaces(t *testing.T) {
	// Set semantics: a second call fully replaces the first.
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	first := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "first",
	})
	if res, _ := tool.Execute(ctx, first); !res.Success {
		t.Fatal("first call failed")
	}
	second := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{"literal": "42", "citation_ref": -1},
	})
	if res, _ := tool.Execute(ctx, second); !res.Success {
		t.Fatal("second call failed")
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Shape != types.ShapeValue {
		t.Errorf("second call did not replace: shape=%s", doc.Shape)
	}
	if doc.Summary == "first" {
		t.Error("second call did not clear stale summary")
	}
}

func TestEmitAnswerDocument_RejectsMissingMutable(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := &types.BusContext{} // no Mutable
	res, _ := tool.Execute(ctx, json.RawMessage(`{"shape": "explanation", "summary": "x"}`))
	if res.Success {
		t.Error("missing Mutable: Success = true, want false")
	}
}

func TestEmitAnswerDocument_Caveats_RoundTrip(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":    "explanation",
		"summary":  "x",
		"caveats":  []string{"note 1", "note 2"},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("caveats happy: Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Caveats) != 2 {
		t.Errorf("caveats len = %d, want 2", len(doc.Caveats))
	}
}

// TestEmitAnswerDocument_DropsQuoteClearedWhenNoTier1Peer pins the
// 2026-04-17 fabricated-citation defence. When every citation grounds
// at Tier 2 only (the LLM never read the files it cites) AND at least
// one quote was cleared as fabricated, ALL quote-cleared citations
// are dropped — a pool with no Tier 1 proven peer cannot anchor
// invented prose. Mirrors the failure mode in trace
// 1776439797257469553 where two cited files had never been read yet
// both citations passed the old Tier 2 "file indexed" check with
// fabricated quotes.
func TestEmitAnswerDocument_DropsQuoteClearedWhenNoTier1Peer(t *testing.T) {
	// Repomap graph with two indexed files; no read_file results.
	graph := newDocGraph(map[string][]docSymbol{
		"a.go": {{name: "Foo", line: 10, end: 30}},
		"b.go": {{name: "Bar", line: 5, end: 20}},
	})
	ctx := newDocBusCtx("")
	ctx.Mutable.SetSearchGraph(graph)

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 15, "quote": "Fabricated prose about Foo's mechanism"},
			{"file": "b.go", "line": 10, "quote": "Fabricated prose about Bar's purpose"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("call must succeed with empty citations pool after drop: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Citations) != 0 {
		t.Errorf("fabricated-quote Tier 2-only pool must be dropped to 0 cites, got %d: %+v", len(doc.Citations), doc.Citations)
	}
	if !strings.Contains(res.Summary, "no peer citation is Tier 1") {
		t.Errorf("drop reason must name the Tier 1 peer rule, got: %s", res.Summary)
	}
}

// TestEmitAnswerDocument_KeepsQuoteClearedWhenTier1PeerPresent — if
// the pool contains at least one Tier 1 proven citation (the LLM
// actually read that file), the quote-cleared peers keep their
// file:line as honest "I saw this location" markers. The rule is a
// pool-level sanity check, not a per-item reject.
func TestEmitAnswerDocument_KeepsQuoteClearedWhenTier1PeerPresent(t *testing.T) {
	// File a.go is BOTH indexed AND read (gutter index populated);
	// file b.go is only indexed.
	graph := newDocGraph(map[string][]docSymbol{
		"a.go": {{name: "Foo", line: 10, end: 30}},
		"b.go": {{name: "Bar", line: 5, end: 20}},
	})
	ctx := newDocBusCtx("")
	ctx.Mutable.SetSearchGraph(graph)
	ctx.ToolResults = []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 10-12 of 30]\n  10│ func Foo() {\n  11│     return\n  12│ }\n"},
	}

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 10, "quote": "func Foo() {"}, // Tier 1 QuoteMatched
			{"file": "b.go", "line": 10, "quote": "Fabricated about Bar"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("call must succeed: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Citations) != 2 {
		t.Errorf("Tier 1 peer present; all cites must survive, got %d", len(doc.Citations))
	}
	// Fabricated quote on b.go must still be CLEARED (per-item rule),
	// even though the peer rescues the file:line.
	for _, c := range doc.Citations {
		if c.File == "b.go" && c.Quote != "" {
			t.Errorf("b.go quote must be cleared, got %q", c.Quote)
		}
	}
}

// docSymbol and newDocGraph are minimal helpers so the above tests
// can build a repomap Graph without pulling the full Graph builder.
type docSymbol struct {
	name string
	line int
	end  int
}

// TestEmitAnswerDocument_Explanation_AcceptsAnchorSkeleton pins the
// 2026-04-17 change: shape=explanation allows optional symbols[] as
// an anchor skeleton for multi-topic answers. Each symbol carries a
// file:line that the renderer draws beneath the summary prose. The
// steps/value/boolean mutex is unchanged — those remain forbidden.
func TestEmitAnswerDocument_Explanation_AcceptsAnchorSkeleton(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "explorer delegates work to SubExplorer via ProposeSubAgents.",
		"symbols": []map[string]interface{}{
			{"name": "ProposeSubAgents", "file": "internal/tool/propose_sub_agents.go", "line": 18, "kind": "type", "rationale": "sub-topic 1: proposal schema"},
			{"name": "SubExplorer", "file": "internal/agent/sub_explorer.go", "line": 25, "kind": "type", "rationale": "sub-topic 2: execution path"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("explanation + symbols[] skeleton must succeed: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Shape != types.ShapeExplanation {
		t.Errorf("shape must stay explanation, got %s", doc.Shape)
	}
	if len(doc.Symbols) != 2 {
		t.Errorf("skeleton symbols must round-trip, got %d items", len(doc.Symbols))
	}
	// Completeness NOT required on this path — the slate is auxiliary.
	if doc.SymbolsCompleteness != "" {
		t.Errorf("explanation skeleton must leave SymbolsCompleteness zero, got %q", doc.SymbolsCompleteness)
	}
}

// TestEmitAnswerDocument_Explanation_StillForbidsValue — steps / value
// / boolean remain forbidden on explanation shape; only symbols[] is
// newly allowed.
func TestEmitAnswerDocument_Explanation_StillForbidsValue(t *testing.T) {
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"value":   map[string]interface{}{"literal": "42", "citation_ref": -1},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Errorf("explanation with value{}: must reject, got success")
	}
	if !strings.Contains(res.Summary, "forbids value") {
		t.Errorf("rejection must name value: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_WhitelistsCitationFilesToTurnAReads pins the
// session-8 whitelist: citations referencing a file Turn A did not
// read are dropped with a warning that lists the allowed set. The
// warning format lets the LLM re-emit against a correct path in one
// round-trip instead of iterating through the grounder's tier cascade.
func TestEmitAnswerDocument_WhitelistsCitationFilesToTurnAReads(t *testing.T) {
	graph := newDocGraph(map[string][]docSymbol{
		"internal/types/subagent.go": {{name: "SubAgent", line: 10, end: 30}},
		"internal/agent/subagent.go": {{name: "SubExplorer", line: 20, end: 40}},
	})
	ctx := newDocBusCtx("")
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/types/subagent.go"},
	})

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "internal/agent/subagent.go", "line": 25}, // wrong file — similar path
			{"file": "internal/types/subagent.go", "line": 15}, // correct
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("call with 1 good + 1 bad cite must succeed, got reject: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Citations) != 1 {
		t.Errorf("expected 1 cite kept (whitelist path), got %d", len(doc.Citations))
	}
	if !strings.Contains(res.Summary, "not in Turn A's ReadFiles list") {
		t.Errorf("whitelist reject warning must name the rule: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "allowed files") {
		t.Errorf("whitelist reject warning must list allowed files: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/types/subagent.go") {
		t.Errorf("allowed list must include the actual read file: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_WhitelistSkippedWhenNoTurnA — no TurnA
// snapshot means pre-pipeline tests / sub-agent contexts. The
// whitelist check must skip so legacy call paths stay green.
func TestEmitAnswerDocument_WhitelistSkippedWhenNoTurnA(t *testing.T) {
	graph := newDocGraph(map[string][]docSymbol{
		"a.go": {{name: "Foo", line: 10, end: 30}},
	})
	ctx := newDocBusCtx("")
	ctx.Mutable.SetSearchGraph(graph)
	// No SetTurnAArtifacts — the whitelist must not fire.

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "x",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 15},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("no-TurnA path must accept; got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "not in Turn A's ReadFiles") {
		t.Errorf("whitelist must be inactive without a TurnA snapshot: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_ShapeAutoCorrect_AcceptsCleanFallback pins
// the session-8 success path: LLM chose a wrong shape but left the
// forbidden fields zero-valued. Zero-sweep nil's the empty Boolean
// pointer, auto-correct falls back to explanation (canCorrect=false
// because boolean.decision was empty), call accepts. Summary
// announces the correction so both LLM retry and REPL operator see
// what happened.
func TestEmitAnswerDocument_ShapeAutoCorrect_AcceptsCleanFallback(t *testing.T) {
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "boolean",
		"summary": "resolved explanation body",
		"boolean": map[string]interface{}{"decision": "", "rationale": "", "citation_ref": -1},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("clean fallback (empty boolean) should succeed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "Shape correction:") {
		t.Errorf("Summary must announce shape correction, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "LLM chose boolean") {
		t.Errorf("Summary must name the LLM's original shape: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "explanation") {
		t.Errorf("Summary must name the resolved shape: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_ShapeAutoCorrect_RejectsNonZeroForbidden pins
// the session-8 Bug 3 fix: when the LLM "shotgun"-fills fields
// across shapes (trace 1776448040358685830 iter=0: shape=boolean +
// non-zero boolean{decision,rationale} + steps[...] + symbols[...]
// + value{...} all at once), the previous silent-scrub path let the
// call pass with the wrong-shape fields quietly nil'd; the LLM saw
// ok=true and repeated the same scattershot on the next turn.
// Session-8: auto-correct resolves shape, then the downstream
// rejectForbiddenFields fires on any non-zero forbidden field and
// returns a specific "shape=X forbids Y" error so the LLM corrects.
func TestEmitAnswerDocument_ShapeAutoCorrect_RejectsNonZeroForbidden(t *testing.T) {
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	tool := &EmitAnswerDocument{}
	// LLM chose boolean with a non-zero decision. Auto-correct
	// resolves shape to explanation but boolean is still non-nil.
	// rejectForbiddenFields must fire.
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "boolean",
		"summary": "x",
		"boolean": map[string]interface{}{"decision": "true", "rationale": "r", "citation_ref": -1},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Errorf("non-zero boolean after shape correction must reject, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "forbids boolean") {
		t.Errorf("rejection must name the offending field: %q", res.Summary)
	}
}
