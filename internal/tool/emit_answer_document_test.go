package tool

import (
	"encoding/json"
	"fmt"
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
		"shape":   "value",
		"summary": "the explorer agent's identifier name in the skill registry, derived from the registration call at the cited location.",
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
		"shape":   "value",
		"summary": "the explorer agent name as registered with the skill bus, looked up via the Register call at the cited file:line.",
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
		"shape":   "config_value",
		"summary": "the database connection URL configured for the application via the database_url key in config/app.yaml.",
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
		"shape":   "value",
		"summary": "the MaxRetries constant defined at internal/types/config.go controls the retry loop ceiling for outbound calls.",
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

// TestEmitAnswerDocument_Symbols_RejectsFabricatedCitation pins the
// session-22 literal-grounding gate on ShapeListOfSymbols. The
// goroutine_dump trace shape: a log frame names `writeSession` at
// `internal/agent/analyzer.go:100`, the file+line pair passes
// os.Stat + the grounder's Tier-1 line-text check (analyzer.go
// does have a line 100), but the LINE CONTENT has zero overlap
// with `writeSession`. Before the per-shape gate, the items
// shipped unchallenged.
func TestEmitAnswerDocument_Symbols_RejectsFabricatedCitation(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 98-104 of 1500 total]\n    98│ }\n    99│ \n   100│ \n   101│ func SomeOtherThing() {\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"symbols_completeness": "lower_bound",
		"symbols": []map[string]interface{}{
			{
				"name":  "writeSession",
				"file":  "internal/agent/analyzer.go",
				"line":  100,
				"kind":  "function",
				"chain": "crash site",
			},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("fabricated symbol citation must be rejected; got Success=true, Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "writeSession") {
		t.Errorf("rejection should name the symbol, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "not corroborated") {
		t.Errorf("rejection should use the literal-grounding contract phrase, got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_Steps_RejectsCitationWithoutIdentifierOverlap
// pins ShapeStepList: a step.Description that names identifiers
// should cite a line where at least one of those identifiers
// appears. Otherwise the cite is decorative at best, misleading
// at worst.
func TestEmitAnswerDocument_Steps_RejectsCitationWithoutIdentifierOverlap(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│ package agent\n     2│ \n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "step_list",
		"steps": []map[string]interface{}{
			{
				"index":        1,
				"description":  "ExplorerAgent dispatches SubAgent via SubAgentRegistry.Register",
				"citation_ref": 0,
			},
		},
		"citations": []map[string]interface{}{{"file": "internal/agent/analyzer.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("step citation without identifier overlap must be rejected; got Success=true, Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "steps[0]") {
		t.Errorf("rejection should name the offending step index, got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_Steps_AcceptsNarrativeStepWithoutIdentifiers
// is the graceful-skip path: a step whose description has no
// identifier tokens (pure narrative) bypasses the gate because
// the check cannot meaningfully cross-reference.
func TestEmitAnswerDocument_Steps_AcceptsNarrativeStepWithoutIdentifiers(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[a.go: showing lines 1-3 of 10 total]\n     1│ package x\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "step_list",
		"steps": []map[string]interface{}{
			// Description with only numeric / non-identifier tokens —
			// regex `[A-Za-z_][A-Za-z0-9_]{2,}` matches nothing, so
			// the gate short-circuits before looking at the cited line.
			{"index": 1, "description": "42 → 404", "citation_ref": 0},
		},
		"citations": []map[string]interface{}{{"file": "a.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("narrative step with no identifier tokens should bypass gate; got Success=false, Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_Steps_AcceptsCitationRefMinusOneEscape pins
// the escape hatch for steps that paraphrase external content:
// citation_ref=-1 bypasses the gate entirely.
func TestEmitAnswerDocument_Steps_AcceptsCitationRefMinusOneEscape(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "step_list",
		"steps": []map[string]interface{}{
			{
				"index":        1,
				"description":  "processRequest failed with nil pointer dereference",
				"citation_ref": -1,
			},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("step with citation_ref=-1 escape must be accepted; got Success=false, Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_Boolean_RejectsUncorroboratedRationale pins
// ShapeBoolean: when the rationale names identifiers that the
// cited line does not mention, the gate fires.
func TestEmitAnswerDocument_Boolean_RejectsUncorroboratedRationale(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[foo.go: showing lines 1-5 of 100 total]\n     1│ package foo\n     2│ \n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "boolean",
		"boolean": map[string]interface{}{
			"decision":     "yes",
			"rationale":    "readyCheck returns true when the ConnectionPool is ready",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "foo.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("boolean rationale without identifier overlap must be rejected; got Success=true, Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "boolean.rationale") {
		t.Errorf("rejection should name the claim location, got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_Boolean_AcceptsCorroboratedRationale is the
// positive path: rationale token appears in the cited window.
func TestEmitAnswerDocument_Boolean_AcceptsCorroboratedRationale(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[foo.go: showing lines 10-15 of 100 total]\n    10│ func readyCheck() bool {\n    11│ \treturn true\n    12│ }\n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "boolean",
		"boolean": map[string]interface{}{
			"decision":     "yes",
			"rationale":    "readyCheck returns true unconditionally",
			"citation_ref": 0,
		},
		"citations": []map[string]interface{}{{"file": "foo.go", "line": 10}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("corroborated boolean citation must be accepted; got Success=false, Summary=%q", res.Summary)
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

func TestEmitAnswerDocument_RejectsExactTargetSubstituteFramingAfterAbsence(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	missingKey := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{missingKey},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: missingKey,
		Kind:  "symbol",
	})

	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "仓库里不存在 `explore_mid_loop_hint_budget` 这个配置键；最接近的等价字段是 `loop_max_midloop_injects`。",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("missing structured exact_resolution must reject")
	}
	if !strings.Contains(res.Summary, "exact-resolution contract violated") {
		t.Fatalf("reject should name exact-resolution contract, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AcceptsExactTargetAbsenceWithContextOnlyFraming(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	missingKey := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{missingKey},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: missingKey,
		Kind:  "symbol",
	})

	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "仓库里不存在 `explore_mid_loop_hint_budget` 这个配置键。相关的同族 explore 启发式默认值在 `DefaultExploreHeuristics()` 中定义，这只是背景上下文，不是该 exact key 的替代项。",
	})
	params = mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related grounded defaults live in `DefaultExploreHeuristics()` and stay context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("absence-first context-only framing should pass, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsAliasMatchAfterAbsenceClosure(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Subject:         "LoopMaxMidLoopInjects",
		Predicate:       "maps",
		Object:          "loop_max_midloop_injects",
		Source:          "internal/types/config.go",
		Summary:         "Related context only: loop_max_midloop_injects is a different namespace than explore_mid_loop_hint_budget.",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status": "alias_match",
			"anchor": "loop_max_midloop_injects",
		},
		"summary": "The nearest nearby knob is loop_max_midloop_injects, but the exact key is absent.",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("absence-closed investigation must not be upgraded to alias_match")
	}
	if !strings.Contains(res.Summary, "already closed as absence") {
		t.Fatalf("reject should point back to absence-closed investigation, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsExactMatchFromTestOnlyConfigProof(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Subject:         target,
		Predicate:       "defines",
		Source:          "internal/types/exact_lookup_test.go",
		Snippet:         `const missing = "explore_mid_loop_hint_budget"`,
		GroundingStatus: types.GroundingGrounded,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status": "exact_match",
		},
		"summary": "A test fixture names the token, but there is no production config proof.",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("test-only config proof must not satisfy exact_match")
	}
	if !strings.Contains(res.Summary, "anchored proof") {
		t.Fatalf("reject should explain anchored-proof requirement, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsExactMatchFromDocOnlyConfigMention(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Subject:         "analysis_contract example",
		Predicate:       "documents",
		Object:          "exact_context_terms example",
		Source:          "internal/skill/analysis_contract.go",
		Snippet:         "Examples: if the exact target is `explore_mid_loop_hint_budget`",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status": "exact_match",
		},
		"summary": "The docs mention the token, but there is no production config definition.",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("documentation-style mention must not satisfy exact_match")
	}
	if !strings.Contains(res.Summary, "anchored proof") {
		t.Fatalf("reject should explain anchored-proof requirement, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AllowsAbsenceWhenOnlyTestMentionsExist(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: target,
		Kind:  "symbol",
	})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Subject:         target,
		Predicate:       "mentions",
		Source:          "internal/types/exact_lookup_test.go",
		Snippet:         `const missing = "explore_mid_loop_hint_budget"`,
		GroundingStatus: types.GroundingGrounded,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related production defaults stay context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("test-only mentions should not contradict absence, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AllowsAbsenceWithProductionContextOnlyMention(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "defaults_to",
		Object:          "MidLoopMinIteration=2",
		Source:          "internal/types/config.go",
		Summary:         "Related context only: explore_mid_loop_hint_budget is absent from this config family.",
		GroundingStatus: types.GroundingGrounded,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related grounded defaults stay context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("production context-only mentions should not contradict absence, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AllowsAbsenceWithFunctionAbsenceSupport(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "FooHandler"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectFunctionName,
				TargetLabel:          "symbol",
				Targets:              []string{target},
				AllowAbsence:         true,
				RequireTargetMention: true,
				AliasRequiresProof:   true,
				RelatedContextPolicy: types.ExactContextGroundedOnly,
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("no function named `FooHandler` exists in the repo")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceMechanism,
		Subject:         "buildFallbackHandlers",
		Predicate:       "describes",
		Summary:         "The fallback path explains why FooHandler is absent from the runtime router.",
		Source:          "internal/agent/router.go",
		LineStart:       42,
		LineEnd:         42,
		GroundingStatus: types.GroundingGrounded,
		ContextRole:     types.EvidenceContextRoleAbsenceSupport,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "buildFallbackHandlers",
		Producer:        EmitEvidenceProducer,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related grounded routing context stays context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("absence-support evidence should not contradict absence for symbol lookups, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AllowsAbsenceWithoutPendingFinding(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("searched the repo and found no config key named `explore_mid_loop_hint_budget`")
	ctx.Mutable.SetInvestigationResultKind("absence")

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related grounded context stays context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("absence should not require a pending finding once the explorer closed with structured absence, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AllowsAbsenceAfterInvestigationWindowReset(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("searched the repo and found no config key named `explore_mid_loop_hint_budget`")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.ResetInvestigationComplete()

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "Related grounded context stays context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("stable accepted absence should survive window reset, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_AbsenceIgnoresUngroundedProductionMention(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("searched the repo and found no config key named `explore_mid_loop_hint_budget`")
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Subject:         target,
		Summary:         "ungrounded grep note mentioning the target text",
		Source:          "internal/config/runtime.go",
		LineStart:       237,
		LineEnd:         237,
		GroundingStatus: types.GroundingUngrounded,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentLoopMaxMidLoopInjects",
		Producer:        EmitEvidenceProducer,
	}})

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "`explore_mid_loop_hint_budget` is absent; any nearby knobs are related context only.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("ungrounded production mention should not contradict absence, got: %q", res.Summary)
	}
}

func TestEmitAnswerDocument_RejectsConfigValueForAbsentConfigTrace(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	target := "explore_mid_loop_hint_budget"
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{target},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:              types.SubjectConfigKey,
				TargetLabel:             "config key",
				Targets:                 []string{target},
				AllowAbsence:            true,
				RequireTargetMention:    true,
				AliasRequiresProof:      true,
				RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
				RelatedContextScopeHint: "same namespace / prefix family",
			},
		},
	}
	ctx.Mutable.SetAbsenceJustification("searched the repo and found no config key named `explore_mid_loop_hint_budget`")
	ctx.Mutable.SetInvestigationResultKind("absence")

	params := mustDocJSON(t, map[string]interface{}{
		"shape": "config_value",
		"exact_resolution": map[string]interface{}{
			"status":       "absent",
			"context_mode": "grounded_context_only",
		},
		"summary": "The exact key is absent; nearby config lineage is related context only.",
		"value": map[string]interface{}{
			"key":          target,
			"literal":      "（不存在）",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("absent config-trace answer should not be accepted as shape=config_value")
	}
	if !strings.Contains(res.Summary, "must not use shape=config_value") {
		t.Fatalf("missing absent config-trace shape guidance: %q", res.Summary)
	}
}

func TestRenderExactResolutionLead_UsesNaturalAbsenceWording(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetLabel: "config key",
		Targets:     []string{"explore_mid_loop_hint_budget"},
	}
	exact := &types.AnswerExactResolution{
		Status:      types.AnswerExactResolutionAbsent,
		ContextMode: types.AnswerExactResolutionContextGroundedOnly,
	}

	gotEN := renderAnswerDocumentExactResolutionLead(contract, exact, "en")
	if !strings.Contains(gotEN, "does not contain the exact config key") {
		t.Fatalf("english absence lead too mechanical or missing: %q", gotEN)
	}
	if !strings.Contains(gotEN, "related background only") {
		t.Fatalf("english grounded-context note missing: %q", gotEN)
	}

	gotZH := renderAnswerDocumentExactResolutionLead(contract, exact, "zh")
	if !strings.Contains(gotZH, "仓库里没有找到精确config key") {
		t.Fatalf("chinese absence lead too mechanical or missing: %q", gotZH)
	}
	if !strings.Contains(gotZH, "只作为相关背景") {
		t.Fatalf("chinese grounded-context note missing: %q", gotZH)
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

func TestEmitAnswerDocument_AcceptsLongSummaryWhenSummaryCapDisabled(t *testing.T) {
	types.SetSummaryCapConfig(types.DefaultSummaryCapConfig())
	t.Cleanup(func() { types.SetSummaryCapConfig(types.DefaultSummaryCapConfig()) })

	tool := &EmitAnswerDocument{}
	// Default summary_cap_enabled=false means there is no hard cap.
	longerThanDefaultCap := strings.Repeat("abcde ", 600) // 3600 chars
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": longerThanDefaultCap,
	})
	res, _ := tool.Execute(newDocBusCtx(""), params)
	if !res.Success {
		t.Fatalf("disabled summary cap rejected long summary: %s", res.Summary)
	}
}

func TestEmitAnswerDocument_SchemaDoesNotAdvertiseCapsWhenDisabled(t *testing.T) {
	types.SetSummaryCapConfig(types.DefaultSummaryCapConfig())
	t.Cleanup(func() { types.SetSummaryCapConfig(types.DefaultSummaryCapConfig()) })

	tool := &EmitAnswerDocument{}
	desc := tool.Description()
	params := string(tool.Parameters())
	combined := desc + "\n" + params
	if !strings.Contains(combined, "summary_cap_enabled=false") {
		t.Fatalf("disabled summary cap guidance missing: %s", combined)
	}
	if strings.Contains(combined, "2500 chars") || strings.Contains(combined, "Per-shape cap") {
		t.Fatalf("disabled schema still advertises concrete caps: %s", combined)
	}
}

func TestEmitAnswerDocument_SchemaAdvertisesCapsWhenEnabled(t *testing.T) {
	enableSummaryCapsForTest(t)

	tool := &EmitAnswerDocument{}
	combined := tool.Description() + "\n" + string(tool.Parameters())
	if !strings.Contains(combined, "Active hard summary caps") {
		t.Fatalf("enabled summary cap guidance missing: %s", combined)
	}
	if !strings.Contains(combined, "explanation <= 2500") {
		t.Fatalf("enabled summary cap guidance missing explanation cap: %s", combined)
	}
}

// TestEmitAnswerDocument_AcceptsLongSummaryOnExplanation locks in the
// 2026-04-17 change: explanation shape now accepts summaries up to
// 2500 chars (multi-paragraph answer body). The old 500-char cap
// forced finalizers to burn retries trimming legitimate prose.
func TestEmitAnswerDocument_AcceptsLongSummaryOnExplanation(t *testing.T) {
	enableSummaryCapsForTest(t)

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

// TestEmitAnswerDocument_TruncatesOverCapQuote verifies that an
// oversize Quote no longer aborts the whole emit. A single long
// citation used to reject every peer in the pool on retry, but legit
// long source lines (deep package imports, multi-arg fmt.Errorf, long
// regex/SQL literals) routinely exceed the 200-char preview ceiling.
// New contract: truncate the Quote preview on a UTF-8 boundary,
// preserve file+line, and let the grounder's QuoteMatched token check
// decide whether the remainder corroborates the line.
func TestEmitAnswerDocument_TruncatesOverCapQuote(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Seed LineIndex so the grounder can reach Tier 1. The cited line
	// shares the identifier "processRequest" with the (truncated) Quote
	// so QuoteMatched=true and the Quote preview survives grounding.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[a.go: showing lines 1-2 of 2 total]\n     1│ func processRequest(ctx context.Context) error { return nil }\n     2│ \n",
	})
	// Construct a Quote that is unambiguously long (> cap) yet still
	// contains the shared identifier at the start so the retained
	// prefix keeps tokens that overlap with line 1.
	longQuote := "func processRequest(ctx context.Context) error { return nil } " + strings.Repeat("x", types.CitationMaxQuoteChars()+200)
	if len(longQuote) <= types.CitationMaxQuoteChars() {
		t.Fatalf("test fixture: longQuote is not actually over cap (len=%d, cap=%d)", len(longQuote), types.CitationMaxQuoteChars())
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "explanation summary long enough to exercise the preview truncation path without tripping the shape-specific validators or natural-language heuristics.",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 1, "quote": longQuote},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("over-cap quote must not reject the emit; Success=false Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "truncated") {
		t.Errorf("Summary must surface truncation warning, got: %q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc == nil || len(doc.Citations) != 1 {
		t.Fatalf("citation must be kept in the pool; got doc=%+v", doc)
	}
	if doc.Citations[0].File != "a.go" || doc.Citations[0].Line != 1 {
		t.Errorf("file+line must be preserved; got %+v", doc.Citations[0])
	}
	if got := len(doc.Citations[0].Quote); got > types.CitationMaxQuoteChars() {
		t.Errorf("Quote must be ≤ cap after truncation; got len=%d cap=%d", got, types.CitationMaxQuoteChars())
	}
}

// TestEmitAnswerDocument_OverCapProseQuoteKeepsAnchor verifies the
// prose-smuggling path: when an oversize Quote is pure prose with no
// identifier tokens shared with the cited line, truncation still runs
// but the grounder's QuoteMatched check clears the Quote text. The
// citation's file+line anchor must survive so the answer still
// references the source, matching the existing behaviour for
// non-oversize mismatched quotes.
func TestEmitAnswerDocument_OverCapProseQuoteKeepsAnchor(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[a.go: showing lines 1-2 of 2 total]\n     1│ func processRequest(ctx context.Context) error { return nil }\n     2│ \n",
	})
	// A long prose quote with zero identifier overlap with line 1.
	// Repeat count is deliberately generous so the fixture stays over
	// the cap if operators raise the default ceiling.
	prose := "this citation paraphrases behaviour in narrative form instead of pasting the literal source line the reader would see in the gutter, which violates the verbatim quote contract. "
	longProse := strings.Repeat(prose, 8)
	if len(longProse) <= types.CitationMaxQuoteChars() {
		t.Fatalf("test fixture: longProse is not over cap (len=%d, cap=%d)", len(longProse), types.CitationMaxQuoteChars())
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "explanation summary long enough to exercise the prose-quote clearance path without tripping the shape-specific validators or natural-language heuristics.",
		"citations": []map[string]interface{}{
			{"file": "a.go", "line": 1, "quote": longProse},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("over-cap prose quote must not reject the emit; Success=false Summary=%q", res.Summary)
	}
	doc := ctx.Mutable.AnswerDocument()
	if doc == nil || len(doc.Citations) != 1 {
		t.Fatalf("file+line anchor must survive prose-quote clearance; got doc=%+v", doc)
	}
	if doc.Citations[0].File != "a.go" || doc.Citations[0].Line != 1 {
		t.Errorf("file+line must be preserved; got %+v", doc.Citations[0])
	}
	if doc.Citations[0].Quote != "" {
		t.Errorf("prose Quote must be cleared by grounder after truncation; got %q", doc.Citations[0].Quote)
	}
}

func TestEmitAnswerDocument_ZeroIsValidCitationRef(t *testing.T) {
	// Critical sentinel test: CitationRef=0 is a VALID pool index
	// (the first citation), distinct from CitationRef=-1 (no citation).
	// This is the foot-gun the sentinel design is meant to catch.
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "value",
		"summary": "the literal 42 returned at a.go:1 is the load-bearing constant the question asks about, cited via pool index 0.",
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
		"shape":   "value",
		"summary": "the literal 42 supersedes the previous explanation answer; this set-semantics replace test confirms second-write wins.",
		"value":   map[string]interface{}{"literal": "42", "citation_ref": -1},
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

// -------- F4.1 diagram-block grounding gate --------

func TestEmitAnswerDocument_DiagramRequired_RejectsMissingDiagram(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			Diagram: &types.DiagramContract{
				Required:       true,
				Minimum:        1,
				PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
			},
		},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "The sequence starts at dispatch and ends in the handler, but this summary has no fenced diagram.",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "dispatch", "citation_ref": -1},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("missing required diagram must reject, got success")
	}
	if !strings.Contains(res.Summary, "diagram required for this dispatch") {
		t.Fatalf("rejection must name the missing diagram contract, got %q", res.Summary)
	}
}

func TestEmitAnswerDocument_DiagramRequired_RejectsPlainCodeFence(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			Diagram: &types.DiagramContract{
				Required:       true,
				Minimum:        1,
				PreferredKinds: []types.DiagramKind{types.DiagramFlow},
			},
		},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "explanation",
		"summary": "This answer includes a fenced block, but it is just code.\n\n" +
			"```go\n" +
			"return value\n" +
			"nextLine()\n" +
			"```",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("plain code fence must not satisfy diagram requirement")
	}
	if !strings.Contains(res.Summary, "diagram required for this dispatch") {
		t.Fatalf("rejection must stay on the missing-diagram gate, got %q", res.Summary)
	}
}

func TestEmitAnswerDocument_DiagramRequired_AcceptsScalarShapeWithGroundedDiagram(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		AnswerContract: types.AnswerContract{
			Diagram: &types.DiagramContract{
				Required:       true,
				Minimum:        1,
				PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
			},
		},
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│package agent\n     2│\n     3│import (\n     4│\t\"context\"\n     5│)\n",
	})
	summary := "The scalar answer comes from the traced dispatch path rather than a standalone constant.\n\n" +
		"```\n" +
		"entry: internal/agent/analyzer.go:5\n" +
		"  -> internal/agent/analyzer.go:5\n" +
		"```\n" +
		"This summary stays long enough to satisfy the value-shape readability contract."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "value",
		"summary": summary,
		"value":   map[string]interface{}{"literal": "explorer", "citation_ref": -1},
		"citations": []map[string]interface{}{
			{"file": "internal/agent/analyzer.go", "line": 5},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("scalar shape with grounded required diagram must pass, got %q", res.Summary)
	}
}

// TestEmitAnswerDocument_DiagramGate_RejectsUngroundedFileInFence
// pins the session-22 F4.1 fix. An explanation whose ASCII call-chain
// diagram names a `.go` file not present in citations[] or the
// log-triage ResolvedFiles must be rejected — that is the exact
// hallucination pattern observed on logtri_go where the finalizer
// wrote "explorer.go (ParseOutput) └─▶ buildAnalysisIR" even though
// the panic frames resolved at analyzer.go:250 and analyzer.go:320.
func TestEmitAnswerDocument_DiagramGate_RejectsUngroundedFileInFence(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Seed a read of analyzer.go so the cited line is grounded.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│ package agent\n     2│ \n     3│ import (\n     4│ \t\"context\"\n     5│ )\n",
	})
	summary := "The panic happens inside buildAnalysisIR. Call chain:\n\n" +
		"```\n" +
		"explorer.go (ParseOutput)\n" +
		"  └─▶ buildAnalysisIR (analyzer.go:5)\n" +
		"```\n" +
		"The nil receiver bubbles up."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/analyzer.go", "line": 5}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("ungrounded `explorer.go` in fenced block must be rejected; got Success=true Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "explorer.go") {
		t.Errorf("rejection must name the offending file, got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_DiagramGate_AcceptsCitedFileInFence is the
// allow-path complement: when every file token inside the fenced
// block IS a citation, the gate passes.
func TestEmitAnswerDocument_DiagramGate_AcceptsCitedFileInFence(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│ package agent\n     2│ \n     3│ import (\n     4│ \t\"context\"\n     5│ )\n",
	})
	summary := "The panic happens inside buildAnalysisIR in analyzer.go. Call chain:\n\n" +
		"```\n" +
		"innermost: buildAnalysisIR (internal/agent/analyzer.go:5)\n" +
		"  ↑ caller: (*analyzerEvaluator).ParseOutput (internal/agent/analyzer.go:5)\n" +
		"```\n" +
		"The nil receiver bubbles up."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/analyzer.go", "line": 5}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("all-cited fenced block must pass the diagram gate; got Success=false Summary=%q", res.Summary)
	}
}

func TestEmitAnswerDocument_DiagramGate_AcceptsUniqueBasenameAlias(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 1-5 of 1500 total]\n     1│package agent\n     2│\n     3│import (\n     4│\t\"context\"\n     5│)\n",
	})
	summary := "The panic happens inside the analyzer path.\n\n" +
		"```\n" +
		"innermost: buildAnalysisIR (analyzer.go:5)\n" +
		"  -> caller: ParseOutput (analyzer.go:5)\n" +
		"```\n"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/analyzer.go", "line": 5}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("unique basename alias must pass the diagram gate; got Success=false Summary=%q", res.Summary)
	}
}

func TestEmitAnswerDocument_DiagramGate_RejectsAmbiguousBasenameAlias(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	graph := newDocGraph(map[string][]docSymbol{
		"internal/config/runtime.go": {{name: "RuntimeConfig", line: 3, end: 10}},
		"cmd/runtime.go":             {{name: "RuntimeMain", line: 3, end: 10}},
	})
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/config/runtime.go", "cmd/runtime.go"},
	})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/config/runtime.go: showing lines 1-3 of 20 total]\n" +
			"     1│package config\n     2│\n     3│type Runtime struct{}\n",
	})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[cmd/runtime.go: showing lines 1-3 of 20 total]\n" +
			"     1│package cmd\n     2│\n     3│func main() {}\n",
	})
	summary := "The dispatch spans two different runtime files.\n\n" +
		"```\n" +
		"runtime.go:3\n" +
		"  -> runtime.go:3\n" +
		"```\n"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": summary,
		"citations": []map[string]interface{}{
			{"file": "internal/config/runtime.go", "line": 3},
			{"file": "cmd/runtime.go", "line": 3},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("ambiguous basename alias must reject, got success")
	}
	for _, want := range []string{
		"runtime.go",
		"`internal/config/runtime.go`",
		"`cmd/runtime.go`",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("rejection must surface %q for disambiguation, got %q", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocument_DiagramGate_HonoursLogTriageResolvedFiles
// pins the allowlist's second source: a file-name in a fenced block
// that is NOT in citations[] but IS in bundle.ResolvedFiles must pass.
// Covers the log-triage authoritative path where the frame anchors
// live in the log section rather than the citation pool.
func TestEmitAnswerDocument_DiagramGate_HonoursLogTriageResolvedFiles(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Publish a log-triage bundle with analyzer.go resolved, but DO
	// NOT cite it — the diagram should still pass because it names a
	// ResolvedFiles entry.
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta:          types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
	})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[cmd/root.go: showing lines 1-3 of 100 total]\n     1│ package cmd\n     2│ \n     3│ import \"context\"\n",
	})
	summary := "The log frame names analyzer.go as the failure site.\n\n" +
		"```\n" +
		"panic at analyzer.go:250 in buildAnalysisIR\n" +
		"```\n" +
		"Citing cmd/root.go for the dispatch entry."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "cmd/root.go", "line": 3}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("fenced block naming a log-triage ResolvedFile must pass; got Success=false Summary=%q", res.Summary)
	}
}

func TestEmitAnswerDocument_DiagramGate_AcceptsGroundedPathLiteralFromCitationWindow(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[config/template.yaml: showing lines 10-14 of 40 total]\n" +
			"    10│ # precedence notes\n" +
			"    11│ # effective order:\n" +
			"    12│ #   code defaults < <deploy>/service.yaml < CLI flags\n" +
			"    13│ features:\n" +
			"    14│   enabled: true\n",
	})
	summary := "The effective config follows the documented precedence.\n\n" +
		"```\n" +
		"code defaults\n" +
		"  -> service.yaml\n" +
		"  -> CLI flags\n" +
		"```\n" +
		"The runtime config filename is grounded by the cited precedence comment."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "config/template.yaml", "line": 12}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("diagram should accept path literal grounded by cited line text; got Success=false Summary=%q", res.Summary)
	}
}

// -------- log-triage coverage gate (session-22 follow-up) --------

// TestEmitAnswerDocument_LogTriageCoverage_RejectsMissingCauseLink
// pins the logtri_java failure: an answer that names one layer of a
// Caused-by chain but omits others is rejected. The real trace had
// RuntimeException → NullPointerException → IOException; the LLM
// wrote a summary mentioning only NullPointerException and invented
// an unrelated servlet stack, silently losing the root cause.
func TestEmitAnswerDocument_LogTriageCoverage_RejectsMissingCauseLink(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "java",
			Signals: []types.LogSignal{types.SignalDB, types.SignalNetwork},
		},
		Errors: []types.LogError{
			{
				Type: "java.lang.RuntimeException",
				Cause: &types.LogError{
					Type: "java.lang.NullPointerException",
					Cause: &types.LogError{
						Type: "java.io.IOException",
					},
				},
			},
		},
	})
	// Summary mentions only the middle cause — root (IOException)
	// and outer (RuntimeException) are silently dropped.
	summary := "该异常是 NullPointerException，典型的空指针访问错误。答案来自附带日志。"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": summary,
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("summary missing RuntimeException + IOException must be rejected; got Success=true Summary=%q", res.Summary)
	}
	// Reject message must list the specific missed types.
	for _, want := range []string{"RuntimeException", "IOException"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("reject message must name missing type %q; got %q", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocument_LogTriageCoverage_AcceptsFullCoverage is the
// happy path: all three chain links named at least once, in any order,
// passes the gate.
func TestEmitAnswerDocument_LogTriageCoverage_AcceptsFullCoverage(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Lang: "java", Signals: []types.LogSignal{types.SignalDB}},
		Errors: []types.LogError{
			{
				Type: "java.lang.RuntimeException",
				Cause: &types.LogError{
					Type:  "java.lang.NullPointerException",
					Cause: &types.LogError{Type: "java.io.IOException"},
				},
			},
		},
	})
	summary := "RuntimeException 是顶层异常，其根因是由 IOException（数据库连接被拒绝）触发的 NullPointerException。外部日志，无仓库引用。"
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": summary,
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("full-coverage summary must pass gate; got Success=false Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_LogTriageCoverage_NoBundleNoCheck confirms
// the gate is inert when there is no log-triage bundle (normal
// non-log questions).
func TestEmitAnswerDocument_LogTriageCoverage_NoBundleNoCheck(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("") // no SetLogTriage call
	// Seed read_file so any citation path grounds if present.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[a.go: showing lines 1-3 of 10 total]\n     1│ package a\n",
	})
	summary := "Some answer with no error types at all."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "a.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("no bundle → gate inert; got rejected Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_LogTriageCoverage_ShapeAgnostic verifies the
// gate fires on every shape's summary, not just explanation. A
// step_list answer whose summary ignores the log's error types must
// also be rejected.
func TestEmitAnswerDocument_LogTriageCoverage_ShapeAgnostic(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{
			{Type: "python.TypeError"},
		},
	})
	// step_list shape, summary deliberately omits TypeError.
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "Some unrelated walkthrough of runtime behaviour.",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "start", "citation_ref": -1},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("step_list missing log type must be rejected; got Success=true Summary=%q", res.Summary)
	}
}

// TestCollectLogTriageTypes_WalksCauseChain pins the helper so a
// future refactor cannot silently flatten the cause recursion.
func TestCollectLogTriageTypes_WalksCauseChain(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{
				Type: "A",
				Cause: &types.LogError{
					Type: "B",
					Cause: &types.LogError{
						Type: "C",
					},
				},
			},
			{Type: "D"},
		},
	}
	got := collectLogTriageTypes(bundle)
	want := []string{"A", "B", "C", "D"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("position %d: got %q, want %q", i, got[i], w)
		}
	}
}

// TestLogTriageTypeCovered_MultiTokenTypes covers the token-length
// threshold and case-insensitive match directly.
func TestLogTriageTypeCovered_MultiTokenTypes(t *testing.T) {
	cases := []struct {
		name    string
		errType string
		summary string
		want    bool
	}{
		{"FQN class name matched verbatim", "java.io.IOException", "the ioexception was raised", true},
		{"class name mixed case", "java.lang.NullPointerException", "A NullPointerException occurred.", true},
		{"prose-shaped type matched on one keyword", "runtime error: invalid memory address or nil pointer dereference", "this is a pointer dereference", true},
		{"prose-shaped type not matched", "runtime error: invalid memory address or nil pointer dereference", "generic panic crash observed", false},
		{"short-only type falls back to substring match", "OOM", "oom triggered", true},
		{"short-only type substring miss", "OOM", "memory pressure rose", false},
	}
	for _, c := range cases {
		got := logTriageTypeCovered(c.errType, strings.ToLower(c.summary))
		if got != c.want {
			t.Errorf("%s: got %v want %v (type=%q summary=%q)", c.name, got, c.want, c.errType, c.summary)
		}
	}
}

// TestEmitAnswerDocument_DiagramGate_IgnoresNonCodeExtensions guards
// against over-rejection on auxiliary prose: `error.log`, `output.txt`,
// and similar non-code extensions inside a fence are treated as prose
// and never trigger the gate.
func TestEmitAnswerDocument_DiagramGate_IgnoresNonCodeExtensions(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[a.go: showing lines 1-2 of 10 total]\n     1│ package main\n     2│ \n",
	})
	summary := "See the pipeline:\n\n" +
		"```\n" +
		"stdin → error.log → output.txt\n" +
		"```\n" +
		"Final step in a.go."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "a.go", "line": 1}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("non-code extensions inside fence must be ignored; got Success=false Summary=%q", res.Summary)
	}
}

// --- Session 24 codename-grounding gate tests ---

// seedCitedLineWindow wires a read_file ToolResult that exposes one
// line's content via the gutter format expected by ground.BuildContext.
// Keeps the codename-gate tests short — each just declares one
// line:text pair and the citation's line number.
func seedCitedLineWindow(ctx *types.BusContext, path string, line int, text string) {
	body := "[" + path + ": showing lines " + fmt.Sprint(line) + "-" + fmt.Sprint(line) + " of 99999 total]\n" +
		"  " + fmt.Sprint(line) + "│ " + text + "\n"
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  body,
	})
}

// TestEmitAnswerDocument_CodenameGate_RejectsUngroundedFallbackLabel
// is the headline case: the u3a failing-run fingerprint. Summary
// introduces `S2` as a sequence extension, but the cited ±3 window
// contains only `S1` — the LLM pattern-completed from prior. Reject.
func TestEmitAnswerDocument_CodenameGate_RejectsUngroundedFallbackLabel(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2426, "// Fallback S1: when the LLM soft-stops in Phase 1")
	summary := "Primary path checks investigationComplete. Fallback S1 fires on ERM satisfaction; " +
		"Fallback S2 fires at max iterations."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/explorer.go", "line": 2426}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("ungrounded `S2` / `Fallback S2` must be rejected; got Success=true Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "S2") {
		t.Errorf("rejection must name the offending codename; got: %q", res.Summary)
	}
}

// TestEmitAnswerDocument_CodenameGate_AcceptsGroundedLabel is the
// allow-path: summary says `S1` and the cited ±3 window contains `S1`.
// Must pass even though `S2` is never mentioned, because `S1` alone is
// the only codename token extracted.
func TestEmitAnswerDocument_CodenameGate_AcceptsGroundedLabel(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2426, "// Fallback S1: when the LLM soft-stops in Phase 1")
	summary := "The primary path checks investigationComplete; Fallback S1 fires when ERM is satisfied."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/explorer.go", "line": 2426}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("grounded `S1` / `Fallback S1` must pass; got Success=false Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_CodenameGate_LineScopeNotFileScope pins the
// core invariant: even when the fabricated token appears ELSEWHERE in
// the cited file, the gate still rejects because the ±3 window around
// the citation does not contain it. This differentiates our gate from
// a whole-file substring check.
func TestEmitAnswerDocument_CodenameGate_LineScopeNotFileScope(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Cite line 2426 — only S1 in window.
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2426, "// Fallback S1: when the LLM soft-stops in Phase 1")
	// ALSO seed a far-away line that happens to contain `S2` as an
	// unrelated token (the real explorer.go has this at line 9001 in
	// the entity-overlap filter).
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 9001, "// S2 (2026-04-12 early-stop audit): the symbol name must overlap")
	summary := "Fallback S2 handles max-iteration cutoff."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "internal/agent/explorer.go", "line": 2426}},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("file-scope presence of `S2` must NOT satisfy the gate when the cited window lacks it; got Success=true Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_CodenameGate_SkipsOnNoCodenameTokens guards
// the degrade path: prose full of CamelCase identifiers but no
// codename-shape tokens passes through untouched.
func TestEmitAnswerDocument_CodenameGate_SkipsOnNoCodenameTokens(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	seedCitedLineWindow(ctx, "a.go", 10, "func ShouldStop() bool { return false }")
	summary := "ShouldStop returns false. It is called by BaseAgent.Execute and observed by LoopController."
	params := mustDocJSON(t, map[string]interface{}{
		"shape":     "explanation",
		"summary":   summary,
		"citations": []map[string]interface{}{{"file": "a.go", "line": 10}},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("prose with CamelCase but no codename tokens must pass; got Success=false Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_CodenameGate_StepDescriptionIsChecked pins
// the step_list branch: hallucinated `S2` in steps[i].description
// (not in summary) must still be rejected. u3a-20260422-010419/run-5's
// failure mode was exactly this path.
func TestEmitAnswerDocument_CodenameGate_StepDescriptionIsChecked(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Seed two distinct cited windows so steps[0] (investigationComplete)
	// and steps[1-2] (Fallback S*) can ground on the appropriate line.
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2421, "if e.investigationComplete {")
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2426, "// Fallback S1: when the LLM soft-stops in Phase 1")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "step_list",
		"summary": "Termination layering.",
		"steps": []map[string]interface{}{
			{"index": 1, "description": "Primary path on investigationComplete.", "citation_ref": 0},
			{"index": 2, "description": "Fallback S1 fires on ERM satisfaction.", "citation_ref": 1},
			{"index": 3, "description": "Fallback S2 fires at max iterations.", "citation_ref": 1},
		},
		"citations": []map[string]interface{}{
			{"file": "internal/agent/explorer.go", "line": 2421},
			{"file": "internal/agent/explorer.go", "line": 2426},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("ungrounded `S2` in step description must be rejected; got Success=true Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "steps[2]") {
		t.Errorf("rejection must point at the specific step index; got: %q", res.Summary)
	}
}

// TestEmitEvidence_CodenameGate_RejectsUngroundedLabel covers the
// upstream hook: explorer-side emit_evidence items get the same check
// against their own Source:[LineStart..LineEnd] window.
func TestEmitEvidence_CodenameGate_RejectsUngroundedLabel(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newDocBusCtx("")
	seedCitedLineWindow(ctx, "internal/agent/explorer.go", 2418, "func (e *explorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {")
	params := mustDocJSON(t, map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"kind":          "direct",
				"subject":       "explorerEvaluator",
				"predicate":     "defines",
				"object":        "bool",
				"source":        "internal/agent/explorer.go",
				"line_start":    2418,
				"anchor_kind":   "definition",
				"anchor_symbol": "ShouldStop",
				"summary":       "S2 终止条件：若 LLM 本轮发出了工具调用则直接返回 false 继续循环",
			},
		},
	})
	res, _ := tool.Execute(ctx, params)
	// The call itself may succeed (item rejected individually), but the
	// rejection message must name the offending codename.
	if !strings.Contains(res.Summary, "S2") {
		t.Errorf("evidence summary containing ungrounded `S2` must be flagged; got Summary=%q Success=%v", res.Summary, res.Success)
	}
}

// TestEmitAnswerDocument_ValueShape_RejectsEmptySummary pins Fix G1:
// shape=value with empty (or too-short) summary is hard-rejected so
// the answer body cannot ship as just "值为 `<literal>`." (10 chars).
// The Fix F prompt change ("ALWAYS fill summary") was advisory and
// the LLM honoured it ~50% on s7a evals; this validator brings the
// failure rate to 0% by forcing a fill or accepting the reject.
func TestEmitAnswerDocument_ValueShape_RejectsEmptySummary(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape": "value",
		// summary intentionally omitted
		"value": map[string]interface{}{
			"literal":      "17677",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("empty-summary on shape=value must reject; got Success=true Summary=%q", res.Summary)
	}
	for _, want := range []string{
		"shape=value requires summary",
		"subject",
		"methodology",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("rejection message missing %q.\nFull rejection:\n%s", want, res.Summary)
		}
	}
}

// TestEmitAnswerDocument_ValueShape_AcceptsFilledSummary is the
// positive companion: a 1-2 sentence summary naming subject + how
// passes the Fix G1 validator.
func TestEmitAnswerDocument_ValueShape_AcceptsFilledSummary(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "value",
		"summary": "the total line count of all non-test .go files under internal/tool, computed via `find internal/tool -name '*.go' ! -name '*_test.go' -exec wc -l {} + | tail -1`.",
		"value": map[string]interface{}{
			"literal":      "17677",
			"citation_ref": -1,
		},
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("filled summary on shape=value must accept; got Success=false Summary=%q", res.Summary)
	}
}

// TestEmitAnswerDocument_KeptZero_EmitsEscapePathBlock pins Fix E:
// when (a) the call fails for any reason AND (b) every LLM-supplied
// citation was dropped by grounding (kept=0 out of N>0), the
// rejection text surfaces the escape-path block so the next-iter
// retry has structured ways out instead of resubmitting the same
// wrong file:line addresses. Regression target was
// eval/results/u3a-20260422-171414/run-1 where the finalizer retried
// 3× with identical-shape payloads, then triggered an LLM API 400
// ("invalid function arguments json string") on the 4th attempt and
// salvaged a half-answer.
//
// Setup: shape=list_of_symbols. symbols[i] cites a line that the
// read_file gutter shows but does NOT contain the symbol name, so
// validateSymbolsLiteralGrounding rejects the call. citations[i] use
// lines OUTSIDE the gutter so the grounder drops them all → kept=0.
// Both signals fire, the rejection runs through failWithContext, and
// Fix E's escape block appears.
func TestEmitAnswerDocument_KeptZero_EmitsEscapePathBlock(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	// Gutter shows lines 98-104; line 100 has neither writeSession
	// nor any other identifier overlap with the symbol name claimed
	// by symbols[0]. validateSymbolsLiteralGrounding rejects.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/analyzer.go: showing lines 98-104 of 1500 total]\n    98│ }\n    99│ \n   100│ \n   101│ func SomeOtherThing() {\n   102│ \treturn\n   103│ }\n   104│ \n",
	})
	params := mustDocJSON(t, map[string]interface{}{
		"shape":                "list_of_symbols",
		"summary":              "writeSession ferries the boundary",
		"symbols_completeness": "lower_bound",
		"symbols": []map[string]interface{}{
			// Line 100 IS in the gutter (so Fix D's HasFileInIndex
			// gate fires) but contains no `writeSession` identifier;
			// validateSymbolsLiteralGrounding will then reject the
			// call since the symbol literal does not corroborate.
			{"name": "writeSession", "file": "internal/agent/analyzer.go", "line": 100, "kind": "function", "rationale": "the boundary helper"},
		},
		"citations": []map[string]interface{}{
			// Lines 200/300 are OUTSIDE the gutter; grounder drops
			// both (no Tier 1, no repomap graph in this test) → kept=0.
			{"file": "internal/agent/analyzer.go", "line": 200, "quote": "writeSession at the boundary"},
			{"file": "internal/agent/analyzer.go", "line": 300, "quote": "writeSession at the boundary"},
		},
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected rejection; got Success=true Summary=%q", res.Summary)
	}
	for _, want := range []string{
		"ALL citations failed grounding",
		"finalizer agent has no read_file tool",
		"completeness='unknown'",
		"call site",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("Fix E escape-path block missing %q.\nFull rejection:\n%s", want, res.Summary)
		}
	}
}
