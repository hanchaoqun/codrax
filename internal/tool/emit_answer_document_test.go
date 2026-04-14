package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// newDocBusCtx returns a BusContext wired with a fresh MutableState so
// emit_answer_document can land its Set call. WorkDir is deliberately
// left unset by default; tests that exercise the blob-leak guard pass
// it explicitly.
func newDocBusCtx(workDir string) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState(types.TaskList{}),
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
	if res.Success {
		t.Error("list_of_symbols with steps: Success = true; should reject forbidden field")
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
		"summary": "The codrax architecture decomposes user tasks through a 5-layer pipeline.",
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
	tool := &EmitAnswerDocument{}
	longSummary := strings.Repeat("a", types.AnswerDocumentMaxSummaryChars+1)
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": longSummary,
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Error("over-cap summary: Success = true, want false")
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
