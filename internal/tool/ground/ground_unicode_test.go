package ground

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestQuoteCorroboratesLine_ChineseLiteralViaSubstring is the
// headline regression: pre-fix, a Chinese string literal extracted
// 0 ASCII tokens via identifierTokenRe and the early-return `len(qTokens) == 0`
// branch hard-rejected the quote. Post-fix, the substring fallback
// finds the literal byte-for-byte in the source line. Trigger from
// the user's guess-number game finalizer log (2026-04-28).
func TestQuoteCorroboratesLine_ChineseLiteralViaSubstring(t *testing.T) {
	fileLines := map[int]string{
		9: `print("我已经想好了一个1-100之间的数字，请开始猜测！")`,
	}
	quote := "我已经想好了一个1-100之间的数字，请开始猜测！"
	if !quoteCorroboratesLine(quote, fileLines, 9, 2) {
		t.Errorf("Chinese literal must corroborate when present in cited line; got false")
	}
}

// TestQuoteCorroboratesLine_NeighbourLineMatch covers the ±radius
// behaviour of the substring fallback: the substring may live within
// `radius` lines of the claimed line. Ensures the fallback walks the
// full window, not just the center line.
func TestQuoteCorroboratesLine_NeighbourLineMatch(t *testing.T) {
	fileLines := map[int]string{
		8: `# preamble`,
		9: `print("placeholder")`,
		10: `print("我已经想好了一个1-100之间的数字！")`, // truth lives at +1
	}
	quote := "我已经想好了一个1-100之间的数字！"
	if !quoteCorroboratesLine(quote, fileLines, 9, 2) {
		t.Errorf("Chinese literal at neighbour line must corroborate within ±2 window")
	}
}

// TestQuoteCorroboratesLine_ChineseLiteralAbsent is the false-positive
// guard: when the Chinese quote isn't actually in the cited window,
// substring fallback still rejects it. Stops the substring path from
// being a green light to fabricate.
func TestQuoteCorroboratesLine_ChineseLiteralAbsent(t *testing.T) {
	fileLines := map[int]string{
		9: `print("hello world")`, // ASCII line, no Chinese
	}
	quote := "我已经想好了一个1-100之间的数字！"
	if quoteCorroboratesLine(quote, fileLines, 9, 2) {
		t.Errorf("Chinese literal MUST be rejected when absent from cited window")
	}
}

// TestQuoteCorroboratesLine_ASCIIPathIsByteIdentical is the R6-style
// drift guard for the dominant case: a quote with at least one ASCII
// identifier token MUST take the original token-set path verbatim,
// not the new substring fallback. Without this guard, a future
// refactor could accidentally widen ASCII matching ("foo" matching
// "fooBar") which would erode the precision the token-set buys.
func TestQuoteCorroboratesLine_ASCIIPathIsByteIdentical(t *testing.T) {
	fileLines := map[int]string{
		9: `func ProcessRequest(ctx Context) {}`,
	}
	// Token-set match: ProcessRequest token in source.
	if !quoteCorroboratesLine(`see ProcessRequest below`, fileLines, 9, 2) {
		t.Errorf("ASCII identifier match should corroborate via token-set")
	}
	// Token-set miss: prose-only quote (every word ≥3 chars, but none
	// appears in the line). Should still reject — substring path is
	// gated on tokenSet returning empty.
	if quoteCorroboratesLine(`unrelated commentary about another function`, fileLines, 9, 2) {
		t.Errorf("ASCII prose with non-matching tokens must NOT slip through to substring fallback")
	}
	// Substring-style false-positive guard: "Foo" being a substring of
	// "FooBar" must NOT corroborate when the source has FooBar.
	fileLines[9] = `func FooBar(){}`
	if quoteCorroboratesLine(`Foo`, fileLines, 9, 2) {
		t.Errorf("ASCII substring 'Foo' inside 'FooBar' must NOT corroborate (token-set precision)")
	}
}

// TestQuoteCorroboratesLine_PunctuationOnlyQuoteAbsent guards the
// edge case where the LLM emits a pure-punctuation quote. tokenSet
// returns 0; substring fallback should ALSO reject when the quote is
// effectively empty after trim.
func TestQuoteCorroboratesLine_PunctuationOnlyQuoteAbsent(t *testing.T) {
	fileLines := map[int]string{
		9: `var x = 1`,
	}
	// Whitespace-only quote post-trim → reject.
	if quoteCorroboratesLine(`   `, fileLines, 9, 2) {
		t.Errorf("whitespace-only quote must NOT corroborate")
	}
}

// TestQuoteCorroboratesLine_JapaneseIdentifier covers Unicode
// identifiers (Java / Python permit these). 日本語 in source line and
// in the quote must corroborate.
func TestQuoteCorroboratesLine_JapaneseIdentifier(t *testing.T) {
	fileLines := map[int]string{
		9: `def 計算する():`,
	}
	if !quoteCorroboratesLine(`計算する`, fileLines, 9, 2) {
		t.Errorf("Japanese identifier must corroborate when present")
	}
}

// TestVerifyLineAnchor_ChineseLiteralThroughGroundContext exercises
// the full public path emit_answer_symbol.go calls, not just the
// inner helper. Builds a real Context with a LineIndex and invokes
// VerifyLineAnchor as the symbol grounder does.
func TestVerifyLineAnchor_ChineseLiteralThroughGroundContext(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"guess_number.py": {
				9: `print("我已经想好了一个1-100之间的数字，请开始猜测！")`,
			},
		},
	}
	matched, ok := VerifyLineAnchor(gc, "guess_number.py", 9,
		"我已经想好了一个1-100之间的数字，请开始猜测！", 2)
	if !ok {
		t.Errorf("Chinese literal anchor must verify via substring fallback; got !ok")
	}
	if matched != 9 {
		t.Errorf("matched line: got %d, want 9", matched)
	}
}

// TestVerifyLineAnchor_ChineseLiteralAbsentRejects mirrors the
// false-positive guard at the public entry point.
func TestVerifyLineAnchor_ChineseLiteralAbsentRejects(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"guess_number.py": {
				9: `print("hello world")`,
			},
		},
	}
	_, ok := VerifyLineAnchor(gc, "guess_number.py", 9,
		"我已经想好了一个1-100之间的数字！", 2)
	if ok {
		t.Errorf("absent Chinese literal must NOT verify")
	}
}

// TestVerifyLineAnchor_ASCIIPathStillStrict re-asserts the ASCII
// precision invariant on the public entry point: substring inside
// another identifier must NOT match. Pairs with the helper-level
// guard so accidental refactors that bypass either layer still fail.
func TestVerifyLineAnchor_ASCIIPathStillStrict(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"x.go": {
				1: `func FooBar() {}`,
			},
		},
	}
	if _, ok := VerifyLineAnchor(gc, "x.go", 1, "Foo", 2); ok {
		t.Errorf("ASCII 'Foo' must not match 'FooBar' line via VerifyLineAnchor")
	}
	if _, ok := VerifyLineAnchor(gc, "x.go", 1, "FooBar", 2); !ok {
		t.Errorf("ASCII 'FooBar' must match exact identifier")
	}
}

// TestRecoverSnippetFuzzy_UnicodeSubstringFallback covers the
// recoverSnippetFuzzy path: when the LLM-supplied evidence Snippet
// contains zero ASCII identifier tokens (Chinese / Japanese), the
// pre-fix code returned (_, _, false) immediately. Post-fix, the
// helper scans the ±15-line window for a byte-level substring match
// and reports the first hit. Mirrors the symbol/quote fallback
// shape introduced in the same H patch.
func TestRecoverSnippetFuzzy_UnicodeSubstringFallback(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"a.py": {
				9: `print("我已经想好了一个数字！")`,
			},
		},
	}
	it := &types.EvidenceItem{
		Source:    "a.py",
		LineStart: 9,
		Snippet:   "我已经想好了一个数字！",
	}
	_, line, ok := recoverSnippetFuzzy(it, gc)
	if !ok {
		t.Errorf("Unicode snippet substring fallback should succeed; got !ok")
	}
	if line != 9 {
		t.Errorf("matched line: got %d, want 9", line)
	}
}

// TestRecoverSnippetFuzzy_UnicodeSubstringAbsent guards the false-
// positive: when the Unicode snippet is not present in the window,
// the substring fallback still returns false.
func TestRecoverSnippetFuzzy_UnicodeSubstringAbsent(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"a.py": {
				9: `print("hello world")`,
			},
		},
	}
	it := &types.EvidenceItem{
		Source:    "a.py",
		LineStart: 9,
		Snippet:   "我已经想好了一个数字！",
	}
	_, _, ok := recoverSnippetFuzzy(it, gc)
	if ok {
		t.Errorf("absent Unicode snippet must NOT recover")
	}
}

// TestRecoverSnippetFuzzy_ASCIIPathByteIdentical re-affirms the
// dominant ASCII case (token-set scoring with 0.6 threshold) is
// untouched by the Unicode fallback addition.
func TestRecoverSnippetFuzzy_ASCIIPathByteIdentical(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"a.go": {
				9:  `func ProcessRequest(ctx Context) Response {`,
				10: `   return doWork(ctx)`,
			},
		},
	}
	it := &types.EvidenceItem{
		Source:    "a.go",
		LineStart: 9,
		Snippet:   "ProcessRequest doWork ctx Context Response",
	}
	_, line, ok := recoverSnippetFuzzy(it, gc)
	if !ok {
		t.Errorf("ASCII fuzzy match should succeed at threshold 0.6")
	}
	if line == 0 {
		t.Errorf("matched line: got 0; expected 9 or 10")
	}
}

// TestGroundCitation_ChineseQuoteSurvivesGrounding is the end-to-end
// test through GroundCitation: a citation with a Chinese quote
// against a source line that contains it returns
// Valid + QuoteMatched=true so the caller keeps the quote instead
// of clearing it. Pre-fix this returned QuoteMatched=false → 12
// quotes cleared in the user's trigger trace.
func TestGroundCitation_ChineseQuoteSurvivesGrounding(t *testing.T) {
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"guess_number.py": {
				9: `print("我已经想好了一个1-100之间的数字，请开始猜测！")`,
			},
		},
	}
	c := types.Citation{
		File:  "guess_number.py",
		Line:  9,
		Quote: "我已经想好了一个1-100之间的数字，请开始猜测！",
	}
	rep := GroundCitation(c, gc)
	if !rep.Valid {
		t.Errorf("Chinese quote citation must be Valid; got %+v", rep)
	}
	if !rep.QuoteMatched {
		t.Errorf("Chinese quote must be QuoteMatched (survives line-text corroboration); got %+v", rep)
	}
}
