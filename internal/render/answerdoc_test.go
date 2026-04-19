package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderAnswerDocument_Nil returns empty string without panic.
func TestRenderAnswerDocument_Nil(t *testing.T) {
	if got := RenderAnswerDocument(nil, "en"); got != "" {
		t.Errorf("nil doc: got %q, want empty", got)
	}
}

// -------- list_of_symbols --------

func TestRenderAnswerDocument_ListOfSymbols_Complete_EN(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{
			{Name: "Foo", File: "a.go", Line: 10, Kind: "function"},
			{Name: "Bar", File: "b.go", Line: 20, Kind: "method"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	// Complete answers have no completeness tag — the answer speaks
	// for itself. Symbols are listed directly.
	if strings.Contains(out, "Complete answer") {
		t.Errorf("complete should not have a header in the body: %q", out)
	}
	if !strings.Contains(out, "**Foo**") || !strings.Contains(out, "a.go:10") {
		t.Errorf("missing Foo/a.go: %q", out)
	}
	if !strings.Contains(out, "**Bar**") || !strings.Contains(out, "b.go:20") {
		t.Errorf("missing Bar/b.go: %q", out)
	}
}

func TestRenderAnswerDocument_ListOfSymbols_LowerBound_ZH(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessLowerBound,
		Symbols:             []types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 10}},
	}
	out := RenderAnswerDocument(doc, "zh")
	// lower_bound renders as a quiet footer tag, not a header.
	if !strings.Contains(out, "已确认信息") {
		t.Errorf("zh lower_bound footer tag missing: %q", out)
	}
	if !strings.Contains(out, "Foo") {
		t.Errorf("symbol missing in zh output: %q", out)
	}
}

func TestRenderAnswerDocument_ListOfSymbols_Unknown_EN(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessUnknown,
		Symbols:             []types.AnswerSymbol{{Name: "X", File: "a.go", Line: 1}},
	}
	out := RenderAnswerDocument(doc, "en")
	// unknown renders as a quiet footer tag.
	if !strings.Contains(out, "non-authoritative") {
		t.Errorf("unknown footer tag missing: %q", out)
	}
}

// -------- step_list --------

func TestRenderAnswerDocument_StepList_EN_WithCitations(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: "Three branches dispatch the request.",
		Steps: []types.AnswerStep{
			{Index: 1, Description: "First branch", CitationRef: 0},
			{Index: 2, Description: "Second branch", CitationRef: 1},
			{Index: 3, Description: "Third branch", CitationRef: types.CitationRefUnset},
		},
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "b.go", Line: 20},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Three branches dispatch") {
		t.Errorf("summary missing: %q", out)
	}
	if !strings.Contains(out, "1. First branch") {
		t.Errorf("step 1 missing: %q", out)
	}
	if !strings.Contains(out, "a.go:10") {
		t.Errorf("cite 0 missing: %q", out)
	}
	if !strings.Contains(out, "b.go:20") {
		t.Errorf("cite 1 missing: %q", out)
	}
	// Step 3 has CitationRef=-1: no cite suffix.
	lines := strings.Split(out, "\n")
	var step3 string
	for _, l := range lines {
		if strings.HasPrefix(l, "3. ") {
			step3 = l
			break
		}
	}
	if step3 == "" {
		t.Fatalf("step 3 line not found: %q", out)
	}
	if strings.Contains(step3, ":") && strings.Contains(step3, "`") {
		// A cite suffix would be like `[a.go:10]`. Its absence is the
		// invariant we want.
		if strings.Contains(step3, "[`") {
			t.Errorf("step 3 unexpectedly rendered a cite: %q", step3)
		}
	}
}

func TestRenderAnswerDocument_StepList_ZH(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "第一步", CitationRef: -1},
		},
	}
	out := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(out, "步骤") {
		t.Errorf("zh step_list header missing: %q", out)
	}
}

// -------- value + config_value --------

func TestRenderAnswerDocument_Value_EN(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeValue,
		Value:     &types.AnswerValue{Literal: "explorer", CitationRef: 0},
		Citations: []types.Citation{{File: "a.go", Line: 42}},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "`explorer`") {
		t.Errorf("literal missing: %q", out)
	}
	if !strings.Contains(out, "a.go:42") {
		t.Errorf("citation missing: %q", out)
	}
}

func TestRenderAnswerDocument_ConfigValue_ZH(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:     types.ShapeConfigValue,
		Value:     &types.AnswerValue{Key: "default_agent", Literal: "explorer", CitationRef: 0},
		Citations: []types.Citation{{File: "config.yaml", Line: 5}},
	}
	out := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(out, "default_agent") || !strings.Contains(out, "explorer") {
		t.Errorf("config value round-trip failed: %q", out)
	}
	if !strings.Contains(out, "配置项") {
		t.Errorf("zh config header missing: %q", out)
	}
}

// -------- boolean --------

func TestRenderAnswerDocument_Boolean_YES_EN(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeBoolean,
		Boolean: &types.AnswerBoolean{Decision: true, Rationale: "the flag gates the path", CitationRef: -1},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.HasPrefix(out, "**YES**") {
		t.Errorf("boolean lead not YES: %q", out)
	}
	if !strings.Contains(out, "the flag gates the path") {
		t.Errorf("rationale missing: %q", out)
	}
}

func TestRenderAnswerDocument_Boolean_NO_ZH(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeBoolean,
		Boolean: &types.AnswerBoolean{Decision: false, Rationale: "标志默认关闭", CitationRef: -1},
	}
	out := RenderAnswerDocument(doc, "zh")
	if !strings.HasPrefix(out, "**否**") {
		t.Errorf("zh boolean lead not 否: %q", out)
	}
}

// -------- explanation + fallback --------

func TestRenderAnswerDocument_Explanation_WithPool(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "Codrax uses a 6-layer pipeline.",
		Citations: []types.Citation{
			{File: "CLAUDE.md", Line: 10, Quote: "orchestration layer"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Codrax uses a 6-layer pipeline") {
		t.Errorf("summary missing: %q", out)
	}
	if !strings.Contains(out, "CLAUDE.md:10") {
		t.Errorf("citation missing: %q", out)
	}
	if !strings.Contains(out, "orchestration layer") {
		t.Errorf("quote missing: %q", out)
	}
}

// TestRenderAnswerDocument_Explanation_WithSkeleton pins the 2026-04-17
// change: shape=explanation with non-empty symbols[] renders a Key
// Anchors block between the summary prose and the citation pool.
// English and Chinese variants both get the section header.
func TestRenderAnswerDocument_Explanation_WithSkeleton(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "Explorer dispatches work to SubExplorer via ProposeSubAgents.",
		Symbols: []types.AnswerSymbol{
			{Name: "ProposeSubAgents", File: "internal/tool/propose_sub_agents.go", Line: 18, Rationale: "sub-topic 1: proposal schema"},
			{Name: "SubExplorer", File: "internal/agent/sub_explorer.go", Line: 25, Rationale: "sub-topic 2: execution path"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Key anchors:") {
		t.Errorf("English skeleton header missing: %q", out)
	}
	if !strings.Contains(out, "ProposeSubAgents") || !strings.Contains(out, "propose_sub_agents.go:18") {
		t.Errorf("first anchor missing: %q", out)
	}
	if !strings.Contains(out, "SubExplorer") || !strings.Contains(out, "sub_explorer.go:25") {
		t.Errorf("second anchor missing: %q", out)
	}
	if !strings.Contains(out, "sub-topic 1") || !strings.Contains(out, "sub-topic 2") {
		t.Errorf("rationale rendered: %q", out)
	}

	outZH := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(outZH, "关键锚点") {
		t.Errorf("Chinese skeleton header missing: %q", outZH)
	}
}

// TestRenderAnswerDocument_Explanation_NoSkeletonWhenEmpty locks the
// backwards-compatible path: explanation without symbols[] renders
// as before — no spurious Key Anchors block.
func TestRenderAnswerDocument_Explanation_NoSkeletonWhenEmpty(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "single-topic answer.",
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Contains(out, "Key anchors") {
		t.Errorf("empty symbols[] must not render anchor block: %q", out)
	}
}

// -------- caveats + edge cases --------

func TestRenderAnswerDocument_Caveats_Appended(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "short answer",
		Caveats: []string{"investigation was bounded", "one file unread"},
	}
	out := RenderAnswerDocument(doc, "en")
	// Caveats render as italic lines in the footer, not a bold header.
	if !strings.Contains(out, "investigation was bounded") || !strings.Contains(out, "one file unread") {
		t.Errorf("caveat bodies missing: %q", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("footer separator missing: %q", out)
	}
}

func TestRenderAnswerDocument_LookupCitation_OutOfRange(t *testing.T) {
	// CitationRef=5 with a 1-entry pool must render WITHOUT a citation
	// suffix and WITHOUT crashing. The schema validator prevents this
	// in the normal path, but the renderer must still be defensive.
	doc := &types.AnswerDocument{
		Shape: types.ShapeStepList,
		Steps: []types.AnswerStep{
			{Index: 1, Description: "oops", CitationRef: 5},
		},
		Citations: []types.Citation{{File: "a.go", Line: 1}},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "oops") {
		t.Errorf("step body missing: %q", out)
	}
	// A.go:1 exists in the pool but is NOT referenced by CitationRef=5;
	// since the fallback is to silently drop the cite suffix, we assert
	// the line does not appear.
	if strings.Contains(out, "[`a.go:1`]") {
		t.Errorf("out-of-range CitationRef leaked a cite: %q", out)
	}
}

func TestRenderAnswerDocument_LangFallback(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols:             []types.AnswerSymbol{{Name: "X", File: "a.go", Line: 1}},
	}
	// "japanese" is not a recognized locale → fallback to English.
	// Complete claims have no tag, so just verify symbols render.
	out := RenderAnswerDocument(doc, "japanese")
	if !strings.Contains(out, "**X**") {
		t.Errorf("lang fallback did not render symbol: %q", out)
	}
}

// TestRenderAnswerDocument_Snippets — Snippets section renders as
// language-tagged fenced code blocks with file:line-line headers.
// Single-line snippets render as "file:N" without the range suffix.
func TestRenderAnswerDocument_Snippets(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "The mechanism works like this.",
		Snippets: []types.CodeSnippet{
			{File: "a.go", StartLine: 10, EndLine: 14, Language: "go",
				Code: "func Foo() {\n    bar()\n}"},
			{File: "b.py", StartLine: 5, EndLine: 5, Language: "python",
				Code: "def baz(): pass"},
		},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Key snippets:") {
		t.Errorf("English header missing: %q", out)
	}
	if !strings.Contains(out, "`a.go:10-14`") {
		t.Errorf("range header missing for multi-line snippet: %q", out)
	}
	if !strings.Contains(out, "`b.py:5`\n") {
		t.Errorf("single-line header must omit range: %q", out)
	}
	if !strings.Contains(out, "```go") {
		t.Errorf("language tag missing for Go snippet: %q", out)
	}
	if !strings.Contains(out, "```python") {
		t.Errorf("language tag missing for Python snippet: %q", out)
	}
	if !strings.Contains(out, "func Foo()") || !strings.Contains(out, "def baz()") {
		t.Errorf("snippet bodies missing: %q", out)
	}

	// Chinese locale header.
	outZH := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(outZH, "关键代码") {
		t.Errorf("Chinese header missing: %q", outZH)
	}
}

// TestRenderAnswerDocument_RelationDiagram — RelationDiagram renders
// wrapped in a plain ``` fence so the monospace column math
// survives terminal rendering (pterm collapses whitespace outside
// fences).
func TestRenderAnswerDocument_RelationDiagram(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:           types.ShapeExplanation,
		Summary:         "x",
		RelationDiagram: "Foo\n │ calls\n ▼\nBar",
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Flow:") {
		t.Errorf("English flow header missing: %q", out)
	}
	if !strings.Contains(out, "Foo") || !strings.Contains(out, "Bar") {
		t.Errorf("diagram body missing: %q", out)
	}
	if !strings.Contains(out, "```\nFoo") {
		t.Errorf("diagram must be wrapped in a plain fence: %q", out)
	}

	outZH := RenderAnswerDocument(doc, "zh")
	if !strings.Contains(outZH, "调用/关系链路图") {
		t.Errorf("Chinese flow header missing: %q", outZH)
	}
}

// TestRenderAnswerDocument_NoSnippetsNoRenderBlock — empty Snippets
// skips the Key snippets section entirely (no blank header).
func TestRenderAnswerDocument_NoSnippetsNoRenderBlock(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "x",
	}
	out := RenderAnswerDocument(doc, "en")
	if strings.Contains(out, "Key snippets") {
		t.Errorf("empty Snippets must not render header: %q", out)
	}
	if strings.Contains(out, "Flow:") {
		t.Errorf("empty diagram must not render header: %q", out)
	}
}
