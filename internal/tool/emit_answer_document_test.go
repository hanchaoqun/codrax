package tool

import (
	"encoding/json"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// enableSummaryCapsForTest flips the package-level summaryCapConfig's
// master switch on for the duration of a test and restores the
// default (Enabled=false) on cleanup. Needed because emit_answer_document
// accepts any length when the switch is off — tests that exercise the
// cap-reject path have to opt in.
func enableSummaryCapsForTest(t *testing.T) {
	t.Helper()
	cfg := types.DefaultSummaryCapConfig()
	cfg.Enabled = true
	types.SetSummaryCapConfig(cfg)
	t.Cleanup(func() { types.SetSummaryCapConfig(types.DefaultSummaryCapConfig()) })
}

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

func TestEmitAnswerDocument_ListOfSymbols_ScrubsStepsWhenRequiredSatisfied(t *testing.T) {
	// Session-8 Fix α' (trace 1776453454793969437): when shape=X's
	// REQUIRED fields are already filled correctly, forbidden fields
	// are JSON noise — silent-scrub them with a note in the success
	// Summary instead of rejecting. Rejection burned 18 retries
	// earlier because the LLM kept filling steps AND boolean on a
	// step_list call; scrubbing accepts the valid answer on the
	// first try and tells the LLM what was dropped.
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"symbols":              []map[string]interface{}{{"name": "Foo", "file": "a.go", "line": 1, "kind": "function"}},
		"symbols_completeness": "complete",
		"steps":                []map[string]interface{}{{"index": 1, "description": "x", "citation_ref": -1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("list_of_symbols with required filled + stray steps: must accept with scrub, got reject: %s", res.Summary)
	}
	// Stray field must be scrubbed from the stored doc.
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Steps) != 0 {
		t.Errorf("steps[] must be scrubbed, got %d items", len(doc.Steps))
	}
	// Summary must note the scrub so the LLM sees it happened.
	if !strings.Contains(res.Summary, "scrubbed unused field") {
		t.Errorf("Summary must note the scrub: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "steps[]") {
		t.Errorf("Summary must name the scrubbed field: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_ListOfSymbols_RejectsWhenRequiredMissing —
// the reject path still fires when the SHAPE's required field is
// missing. Scrub is only for "answer there, noise alongside".
func TestEmitAnswerDocument_ListOfSymbols_RejectsWhenRequiredMissing(t *testing.T) {
	tool := &EmitAnswerDocument{}
	// No symbols[] — required field missing. Even if forbidden fields
	// are present, we should reject because the shape is unsatisfied.
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"symbols":              []map[string]interface{}{},
		"symbols_completeness": "complete",
		"steps":                []map[string]interface{}{{"index": 1, "description": "x", "citation_ref": -1}},
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if res.Success {
		t.Errorf("empty symbols[] must reject, got success")
	}
	if !strings.Contains(res.Summary, "requires symbols[]") {
		t.Errorf("rejection must name the missing required field: %q", res.Summary)
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

// TestEmitAnswerDocument_Explanation_ScrubsZombieBoolean pins the
// session-8 Fix α' behavior: shape=explanation with a valid summary
// AND a stray boolean{decision:"否", rationale:...} silent-scrubs
// the boolean and accepts the call. Earlier session-7 behavior
// rejected, which caused the trace 1776453454793969437 loop (LLM
// kept answering correctly + filling the stray field).
func TestEmitAnswerDocument_Explanation_ScrubsZombieBoolean(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "valid answer body",
		"boolean": map[string]interface{}{"decision": "否", "rationale": "spurious", "citation_ref": -1},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("explanation with valid summary + stray boolean: must accept with scrub, got reject: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Boolean != nil {
		t.Errorf("boolean must be scrubbed from stored doc, got %+v", doc.Boolean)
	}
	if !strings.Contains(res.Summary, "boolean{}") {
		t.Errorf("scrub note must name the boolean field: %q", res.Summary)
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

func TestEmitAnswerDocument_StepList_RejectsEnglishProseWhenLanguageIsChinese(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "zh"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "这是一个中文摘要。",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "If there is no cache, it calls fullScan.", "citation_ref": 0},
		},
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 10},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("english step prose under zh request: Success = true, want false")
	}
	if !strings.Contains(res.Summary, "steps[0].description") {
		t.Fatalf("language rejection should name the offending field, got %q", res.Summary)
	}
}

func TestEmitAnswerDocument_StepList_AllowsChineseProseWithCodeIdentifiers(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Language = "zh"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "`buildOrLoadGraph` 会根据缓存情况选择不同路径。",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "如果没有缓存，就调用 fullScan。", "citation_ref": 0},
			{"index": 2, "description": "如果没有变更，就调用 loadFromCache。", "citation_ref": 1},
		},
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 10},
			{"file": "a.go", "line": 20},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("chinese prose with code identifiers should pass, got reject: %q", res.Summary)
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

// TestEmitAnswerDocument_Value_RejectsFabricatedCitation is the
// session-22 regression for the "partial" eval case: the attached
// log named an error function (processRequest) that does not exist
// anywhere in the repo, but the LLM fabricated a citation at
// `internal/agent/analyzer.go:1` (which actually starts with
// `package agent`) to satisfy the tool schema. The new literal-
// grounding gate must catch the mismatch BEFORE the answer ships
// and direct the LLM toward the honest escape (citation_ref=-1 +
// summary caveat for externally-sourced literals).
func TestEmitAnswerDocument_Value_RejectsFabricatedCitation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Seed a read_file result so the grounder's LineIndex picks up
	// the cited file at line 1. The line content has zero overlap
	// with the claimed literal. Banner uses the gutter format
	// `N│ text` that ground.buildLineIndex parses.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│ package agent\n     2│ \n     3│ import (\n     4│ \t\"context\"\n     5│ )\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{
			"literal":      "processRequest",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "internal/agent/analyzer.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("fabricated citation must be rejected; got Success=true, Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "not corroborated") {
		t.Errorf("rejection message must name the corroboration failure, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "citation_ref=-1") {
		t.Errorf("rejection message must name the citation_ref=-1 escape, got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_Value_AcceptsCitationRefMinusOneEscape is
// the defensive-allow complement: when the LLM honestly declares
// no grounded source (CitationRef=-1), the literal-grounding gate
// must bypass entirely. Mirrors the customer trace's correct fix
// — the LLM paraphrases from the attached log, acknowledges no
// repo code backs the literal, and ships.
func TestEmitAnswerDocument_Value_AcceptsCitationRefMinusOneEscape(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Even with a read_file populating LineIndex, CitationRef=-1
	// short-circuits the new gate.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│ package agent\n     2│ \n     3│ import (\n     4│ \t\"context\"\n     5│ )\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "value",
		"summary": "基于附带的运行时日志，错误函数是 processRequest。仓库内未包含该函数定义（属于外部应用）。",
		"value": map[string]interface{}{
			"literal":      "processRequest",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("CitationRef=-1 honest escape must be accepted; got Success=false, Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_Value_AcceptsCorroboratedCitation is the
// happy path under the new gate: a value.literal whose identifier
// token DOES appear in the cited file's ±3-line window passes
// through. Guards against the gate becoming over-restrictive.
func TestEmitAnswerDocument_Value_AcceptsCorroboratedCitation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Seed read_file with a line whose text contains the literal
	// as a whole identifier token. The cited line is :12 and the
	// literal "explorer" appears at line 10 — within the ±3 window.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/skill/defaults.go: showing lines 10-15 of 200 total]\n    10│ skill.Register(\"explorer\", ...)\n    11│ \n    12│ // explorer is the Turn A agent\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{
			"literal":      "explorer",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "internal/skill/defaults.go", "line": 12}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("corroborated citation must be accepted; got Success=false, Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_ConfigValue_KeyCorroboratesAsLiteral pins
// the config_value branch: the literal-grounding gate unions
// literal tokens WITH key tokens, so a config leaf cited at a
// definition line where only the key appears still passes. Common
// pattern: YAML leaf at line N has `database_url: "postgres://..."`
// — citing that line with value={key="database_url", literal="postgres://..."}
// must match even though "postgres" may not be tokenised identically.
func TestEmitAnswerDocument_ConfigValue_KeyCorroboratesAsLiteral(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Cited line contains the KEY but not the literal.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[config/app.yaml: showing lines 1-5 of 50 total]\n     1│ database_url: \"postgres://localhost\"\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "config_value",
		"value": map[string]interface{}{
			"key":          "database_url",
			"literal":      "postgres://localhost",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "config/app.yaml", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("config_value key corroboration must be accepted; got Success=false, Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_Value_NumericLiteralDegrades pins the
// degrade path for literals with no identifier-shape tokens
// (numeric / single-character). The gate cannot meaningfully
// cross-reference so it skips, letting the pre-existing checks
// (file-in-ReadSet, grounder) remain the gates.
func TestEmitAnswerDocument_Value_NumericLiteralDegrades(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/types/config.go: showing lines 1-5 of 100 total]\n     1│ const MaxRetries = 5\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		"value": map[string]interface{}{
			"literal":      "5",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "internal/types/config.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("numeric literal must skip the literal-grounding gate; got Success=false, Summary=%q", res.Summary)
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
	enableSummaryCapsForTest(t)
	// Summary cap is per-shape via types.SummaryCapFor(shape, itemCount).
	// Exercise scalar + item-scaled shapes so a future shape addition
	// that forgets to extend SummaryCapConfig gets caught here.
	cases := []struct {
		shape types.AnswerShape
		cap   int
		extra map[string]interface{}
	}{
		{types.ShapeExplanation, types.SummaryCapFor(types.ShapeExplanation, 0), nil},
		{types.ShapeValue, types.SummaryCapFor(types.ShapeValue, 0), map[string]interface{}{
			"value": map[string]interface{}{"literal": "x", "citation_ref": -1},
		}},
		{types.ShapeBoolean, types.SummaryCapFor(types.ShapeBoolean, 0), map[string]interface{}{
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
			types.SummaryCapFor(types.ShapeExplanation, 0), res.Summary)
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
		"shape":   "explanation",
		"summary": "x",
		"caveats": []string{"note 1", "note 2"},
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

// TestEmitAnswerDocument_Explanation_ScrubsStrayValue — steps / value
// / boolean remain forbidden on explanation shape, but under session-8
// Fix α' they are silent-scrubbed (with a Summary note) when the
// required summary field is satisfied. The cluttered answer ships
// with the stray field removed.
func TestEmitAnswerDocument_Explanation_ScrubsStrayValue(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "valid answer body",
		"value":   map[string]interface{}{"literal": "42", "citation_ref": -1},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("explanation with valid summary + stray value: must accept with scrub, got: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Value != nil {
		t.Errorf("value must be scrubbed, got %+v", doc.Value)
	}
	if !strings.Contains(res.Summary, "value{}") {
		t.Errorf("scrub note must name value: %q", res.Summary)
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
	if !strings.Contains(res.Summary, "not in the investigation's read-files list") {
		t.Errorf("whitelist reject warning must name the rule: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "allowed files") {
		t.Errorf("whitelist reject warning must list allowed files: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/types/subagent.go") {
		t.Errorf("allowed list must include the actual read file: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_CitationPathCanonicalisation pins the
// absolute-vs-relative path reconciliation (session 8 follow-up): the
// explorer reads files with relative paths and the finalizer LLM
// frequently cites with absolute paths (mirroring whatever format the
// extractor's hypothesis verdict used). Without canonicalisation the
// whitelist `readFiles[c.File]` check saw two different strings and
// dropped every citation; with canonicalisation both sides become the
// same repo-relative form and the citation is accepted.
//
// Regression scenario mirrors the reported log (iSulad README):
//   - repoRoot = /mnt/d/repo
//   - Turn A read_file path = "README.md" (relative)
//   - Finalizer citation file = "/mnt/d/repo/README.md" (absolute)
//
// Both must canonicalise to "README.md" and the citation must survive.
// README.md has zero symbols in the graph — Tier 2's zero-symbol
// fallback accepts any positive line number, so the only gate that
// could drop this citation is the whitelist.
func TestEmitAnswerDocument_CitationPathCanonicalisation(t *testing.T) {
	graph := newDocGraph(map[string][]docSymbol{
		"README.md": {}, // no symbols — Tier 2 admits any line via prologue fallback
	})
	ctx := newDocBusCtx("")
	ctx.RepoRoot = "/mnt/d/repo"
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"README.md"}, // explorer read via relative path
	})

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "iSulad is a lightweight container runtime.",
		"citations": []map[string]interface{}{
			// Absolute — pre-fix this hit the whitelist and dropped.
			{"file": "/mnt/d/repo/README.md", "line": 7},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("absolute-path citation against relative-path readFiles must survive: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if len(doc.Citations) != 1 {
		t.Fatalf("expected 1 citation kept after canonicalisation, got %d; summary=%q", len(doc.Citations), res.Summary)
	}
	// Persisted citation carries the canonical form so downstream
	// renderers (Key Anchors block, evidence catalog) show a stable
	// repo-relative path instead of the user's absolute prefix.
	if doc.Citations[0].File != "README.md" {
		t.Errorf("persisted citation must be canonical, got %q", doc.Citations[0].File)
	}
	if strings.Contains(res.Summary, "not in the investigation's read-files list") {
		t.Errorf("whitelist must not fire after canonicalisation: %q", res.Summary)
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
	if strings.Contains(res.Summary, "not in the investigation's read-files") {
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

// TestEmitAnswerDocument_ShapeAutoCorrect_ScrubsStrayAfterRequiredFilled
// pins session-8 Fix α' on the auto-correct path: LLM chose boolean,
// analyzer target was explanation, LLM also filled a valid summary
// so the resolved-shape required field is satisfied. The stray
// boolean{} gets silent-scrubbed with a note in the Summary and
// the answer ships. Previous session-7 behavior rejected here, but
// the rejection burned retries without teaching the LLM anything
// it could act on — the answer body was already correct.
func TestEmitAnswerDocument_ShapeAutoCorrect_ScrubsStrayAfterRequiredFilled(t *testing.T) {
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
	}
	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "boolean",
		"summary": "valid explanation answer body",
		"boolean": map[string]interface{}{"decision": "true", "rationale": "r", "citation_ref": -1},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("shape auto-corrected + required satisfied + stray field: must accept with scrub, got: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc.Shape != types.ShapeExplanation {
		t.Errorf("resolved shape must be explanation, got %s", doc.Shape)
	}
	if doc.Boolean != nil {
		t.Errorf("stray boolean must be scrubbed, got %+v", doc.Boolean)
	}
	// Both the shape-correction and the scrub notes must surface.
	if !strings.Contains(res.Summary, "Shape correction:") {
		t.Errorf("Summary must carry shape correction note: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "boolean{}") {
		t.Errorf("Summary must name the scrubbed boolean field: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_FailWithContext_AggregatesCitationWarnings pins
// session-8 Fix α (trace 1776450670620195562): when the shape switch
// rejects a call, the failure Summary must include every earlier
// validation signal (shape correction note + citation grounding
// warnings) so the LLM sees ALL problems on one retry instead of
// fixing them one-at-a-time and burning tokens to a rate-limit.
//
// Under Fix α', the "boolean stray + steps filled" combo now succeeds
// (scrub path), so the failure trigger here is the REQUIRED step[]
// being empty — shape=step_list with no steps + bad citations. The
// aggregate message must carry both the "requires steps[]" error
// and the citation-grounding warnings.
func TestEmitAnswerDocument_FailWithContext_AggregatesCitationWarnings(t *testing.T) {
	graph := newDocGraph(map[string][]docSymbol{
		"a.go": {{name: "Foo", line: 10, end: 30}},
	})
	ctx := newDocBusCtx("")
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"a.go"},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeStepList},
	}

	tool := &EmitAnswerDocument{}
	// Failure cocktail in ONE call:
	//   - shape=step_list required steps[] is EMPTY → reject
	//   - citations[0] references `wrong.go` not in Turn A ReadFiles — whitelist drop
	//   - citations[1] has a fabricated quote on an indexed file — Tier-1 peer drop
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "x",
		"steps":   []map[string]interface{}{}, // empty required field → fail
		"citations": []map[string]interface{}{
			{"file": "wrong.go", "line": 5},
			{"file": "a.go", "line": 15, "quote": "fabricated prose"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected reject, got success: %s", res.Summary)
	}
	// Primary error: required steps missing.
	if !strings.Contains(res.Summary, "requires steps[]") {
		t.Errorf("Summary must carry the primary structural error: %q", res.Summary)
	}
	// Aggregated: the citation warnings must be visible too.
	if !strings.Contains(res.Summary, "Citation grounding") {
		t.Errorf("Summary must include aggregated citation grounding warnings: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "wrong.go") {
		t.Errorf("Summary must name the whitelist-dropped file: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_EnrichsDocWithSnippets pins the
// render-only Snippets field: after the LLM's emit_answer_document
// call lands, the tool populates Snippets from the read_file gutter
// at each citation line, deterministically. LLM never writes this;
// the field survives without requiring the LLM to author code blocks.
func TestEmitAnswerDocument_EnrichsDocWithSnippets(t *testing.T) {
	// Prepare context with:
	//   - read_file result for "a.go" so the gutter index has lines
	//     60-65 populated (snippet extractor reads these).
	//   - SearchGraph indexing "a.go" so citation whitelist passes.
	//   - TurnA.ReadFiles naming "a.go" so whitelist passes.
	ctx := newDocBusCtx("")
	graph := newDocGraph(map[string][]docSymbol{
		"a.go": {{name: "Foo", line: 60, end: 100}, {name: "Bar", line: 50, end: 58}},
	})
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"a.go"}})

	// Synthesize a read_file gutter so Tier 1 validates + snippets
	// have real text to extract.
	readResult := types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[a.go: showing lines 58-67 of 200]\n" +
			"  58│ }\n" +
			"  59│ \n" +
			"  60│ func Foo() {\n" +
			"  61│     bar()\n" +
			"  62│     return\n" +
			"  63│ }\n" +
			"  64│ \n" +
			"  65│ func baz() {\n" +
			"  66│     return\n" +
			"  67│ }\n",
	}
	ctx.ToolResults = []types.ToolResult{readResult}

	tool := &EmitAnswerDocument{}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "Foo calls Bar which returns x.",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 61, "quote": "bar()"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("emit must succeed: %s", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()

	// Snippets: one cluster around line 61 (±2 → 59-63).
	if len(doc.Snippets) == 0 {
		t.Fatalf("expected at least one snippet, got none")
	}
	s := doc.Snippets[0]
	if s.File != "a.go" {
		t.Errorf("snippet file = %q, want a.go", s.File)
	}
	if s.Language != "go" {
		t.Errorf("snippet language = %q, want go", s.Language)
	}
	if !strings.Contains(s.Code, "func Foo()") {
		t.Errorf("snippet body must contain the cited symbol: %q", s.Code)
	}
}

// TestExtractCodeSnippets_ClustersAdjacentCitations pins the
// adjacency-collapse rule: two citations into the same file with
// overlapping ±2 windows merge into one snippet rather than
// rendering two near-identical blocks.
func TestExtractCodeSnippets_ClustersAdjacentCitations(t *testing.T) {
	ctx := newDocBusCtx("")
	readResult := types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[a.go: showing lines 60-70 of 200]\n" +
			"  60│ func A() {}\n" +
			"  61│ \n" +
			"  62│ func B() {}\n" +
			"  63│ \n" +
			"  64│ func C() {}\n" +
			"  65│ \n" +
			"  66│ func D() {}\n" +
			"  67│ \n" +
			"  68│ func E() {}\n" +
			"  69│ \n" +
			"  70│ func F() {}\n",
	}
	ctx.ToolResults = []types.ToolResult{readResult}
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		Citations: []types.Citation{
			{File: "a.go", Line: 62},
			{File: "a.go", Line: 64}, // window overlaps with 62's → merge
		},
	}
	got := extractCodeSnippets(ctx, doc, 5)
	if len(got) != 1 {
		t.Fatalf("adjacent citations must merge into 1 snippet, got %d", len(got))
	}
	// Merged range: 60-66 (62-2 .. 64+2).
	if got[0].StartLine != 60 || got[0].EndLine != 66 {
		t.Errorf("merged range = %d-%d, want 60-66", got[0].StartLine, got[0].EndLine)
	}
}

