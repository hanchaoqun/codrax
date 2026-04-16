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
	if !strings.Contains(out, "Complete answer") {
		t.Errorf("missing complete header: %q", out)
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
	if !strings.Contains(out, "至少包含以下符号") {
		t.Errorf("zh lower_bound header missing: %q", out)
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
	if !strings.Contains(out, "non-authoritative") {
		t.Errorf("unknown header missing: %q", out)
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

// -------- caveats + edge cases --------

func TestRenderAnswerDocument_Caveats_Appended(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "short answer",
		Caveats: []string{"investigation was bounded", "one file unread"},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "Caveats") {
		t.Errorf("caveats header missing: %q", out)
	}
	if !strings.Contains(out, "investigation was bounded") || !strings.Contains(out, "one file unread") {
		t.Errorf("caveat bodies missing: %q", out)
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
	out := RenderAnswerDocument(doc, "japanese")
	if !strings.Contains(out, "Complete answer") {
		t.Errorf("lang fallback did not pick English: %q", out)
	}
}
