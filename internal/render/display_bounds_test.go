package render

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- collapseRepeatedSegments ------------------------------------------------

// TestCollapseRepeatedSegments_FoldsConsecutiveRun pins the core fold:
// N consecutive verbatim-identical lines become ONE copy + a "×N
// collapsed" note. This is the display-side fix for the 2026-07-03
// berlin.systrace degenerate response (one sentence repeated thousands
// of times, 124KB committed verbatim to scrollback).
func TestCollapseRepeatedSegments_FoldsConsecutiveRun(t *testing.T) {
	in := "同一句诊断结论。\n同一句诊断结论。\n同一句诊断结论。"
	out := collapseRepeatedSegments(in, "zh")
	if got := strings.Count(out, "同一句诊断结论。"); got != 1 {
		t.Fatalf("expected exactly 1 surviving copy, got %d in %q", got, out)
	}
	if !strings.Contains(out, "（重复 ×3 已折叠）") {
		t.Fatalf("missing zh fold note with exact count: %q", out)
	}
	// en note variant. 3 copies, not 2: since the 2026-07-03 WF1 fix
	// the fold requires run length ≥ minCollapseRun (3) — a pair is
	// legitimate content and passes through (see _PairKeptVerbatim).
	outEn := collapseRepeatedSegments("same line\nsame line\nsame line", "en")
	if !strings.Contains(outEn, "(repeated ×3, collapsed)") {
		t.Fatalf("missing en fold note: %q", outEn)
	}
}

// TestCollapseRepeatedSegments_NonConsecutiveNotFolded pins that the
// fold is strictly CONSECUTIVE-run based: alternating or interleaved
// duplicates are legitimate prose structure and must pass through
// byte-identically (the function returns the original string when no
// run folded, so even whitespace is preserved).
func TestCollapseRepeatedSegments_NonConsecutiveNotFolded(t *testing.T) {
	for _, in := range []string{
		"A\nB\nA",
		"a\nb\na\nb",
		"first\nsecond\nthird",
	} {
		if out := collapseRepeatedSegments(in, "zh"); out != in {
			t.Errorf("non-consecutive duplicates must not fold: in=%q out=%q", in, out)
		}
	}
}

// TestCollapseRepeatedSegments_BlankSeparatedRunFolds pins that blank
// lines INSIDE a run do not break it — degenerate model output often
// separates its repeated sentence with empty lines.
func TestCollapseRepeatedSegments_BlankSeparatedRunFolds(t *testing.T) {
	in := "重复段落\n\n重复段落\n\n重复段落"
	out := collapseRepeatedSegments(in, "zh")
	if got := strings.Count(out, "重复段落"); got != 1 {
		t.Fatalf("blank-separated run must fold to 1 copy, got %d in %q", got, out)
	}
	if !strings.Contains(out, "×3") {
		t.Fatalf("fold note must count all 3 occurrences: %q", out)
	}
}

// TestCollapseRepeatedSegments_SingleLinePassthrough pins the fast
// path: no '\n' → returned verbatim, even if the line itself contains
// internal repetition (segment granularity is the line, by design).
func TestCollapseRepeatedSegments_SingleLinePassthrough(t *testing.T) {
	in := "AAA AAA AAA"
	if out := collapseRepeatedSegments(in, "zh"); out != in {
		t.Fatalf("single-line input must pass through: %q → %q", in, out)
	}
}

// TestCollapseRepeatedSegments_VerbatimCompareIndentVariantsNotFolded
// REPLACES TestCollapseRepeatedSegments_TrimCompareKeepsFirstVerbatim
// (2026-07-03 adversarial review WF1). The old pin asserted TrimSpace-
// based comparison — exactly the behavior that folded indentation-
// variant lines ("\t}" vs "}") of fenced code blocks and destroyed the
// block shape. Comparison is now verbatim byte equality: lines that
// differ only in leading/trailing whitespace are DIFFERENT segments
// and must pass through byte-identically.
func TestCollapseRepeatedSegments_VerbatimCompareIndentVariantsNotFolded(t *testing.T) {
	for _, in := range []string{
		"  padded\npadded  \n padded", // whitespace variants of one word
		"\t\t}\n\t}\n}",               // close-brace ladder, decreasing indent
		"\t}\n}\n\t}",                 // alternating indent variants
	} {
		if out := collapseRepeatedSegments(in, "en"); out != in {
			t.Errorf("indent-variant lines are distinct segments and must not fold: in=%q out=%q", in, out)
		}
	}
	// Verbatim-identical runs (indentation included) still fold, and
	// the surviving copy keeps its indentation.
	out := collapseRepeatedSegments("  same\n  same\n  same", "en")
	if !strings.HasPrefix(out, "  same ") || !strings.Contains(out, "×3") {
		t.Fatalf("verbatim-identical indented run must fold keeping the indent: %q", out)
	}
}

// TestCollapseRepeatedSegments_PairKeptVerbatim pins the ≥3 run
// threshold added by the 2026-07-03 WF1 fix: a pair of identical
// lines is common legitimate structure (two same-depth "}" closers in
// one snippet, a repeated emphasis line) and must pass through
// byte-identically; only runs of minCollapseRun (3) and above fold.
func TestCollapseRepeatedSegments_PairKeptVerbatim(t *testing.T) {
	for _, in := range []string{
		"same line\nsame line",
		"}\n}",
		"重复\n重复\n其他",
	} {
		if out := collapseRepeatedSegments(in, "zh"); out != in {
			t.Errorf("a run of 2 must not fold: in=%q out=%q", in, out)
		}
	}
	if out := collapseRepeatedSegments("x\nx\nx", "en"); !strings.Contains(out, "×3") {
		t.Fatalf("a run of exactly 3 is the fold boundary and must fold: %q", out)
	}
}

// TestCollapseRepeatedSegments_FencedCodeBlockShapePreserved is the
// end-user scenario behind the WF1 finding: a fenced code block whose
// nested closing braces sit at different indentation levels must
// survive the display collapse byte-identically — under the old
// TrimSpace compare the "\t\t}\n\t}\n}" ladder folded into one line
// with a "×3" note, visually corrupting the code.
func TestCollapseRepeatedSegments_FencedCodeBlockShapePreserved(t *testing.T) {
	in := "```go\nfunc a() {\n\tif b {\n\t\tfor c {\n\t\t\tdo()\n\t\t}\n\t}\n}\n```"
	if out := collapseRepeatedSegments(in, "zh"); out != in {
		t.Fatalf("nested close-brace ladder must not fold:\nin=%q\nout=%q", in, out)
	}
}

// --- capDisplayText ----------------------------------------------------------

// TestCapDisplayText_BoundaryExact pins the no-op boundary: exactly
// displayTextCapRunes runes must survive untouched (no note).
func TestCapDisplayText_BoundaryExact(t *testing.T) {
	in := strings.Repeat("x", displayTextCapRunes)
	if out := capDisplayText(in, "zh"); out != in {
		t.Fatalf("exact-cap input must pass through unchanged")
	}
}

// TestCapDisplayText_OverCapTruncatesWithCount pins the over-cap
// behavior: keep displayTextCapRunes runes, append an explicit note
// carrying the ORIGINAL rune count so the user knows how much the
// model actually produced.
func TestCapDisplayText_OverCapTruncatesWithCount(t *testing.T) {
	total := displayTextCapRunes + 123
	in := strings.Repeat("y", total)
	out := capDisplayText(in, "zh")
	if !strings.HasPrefix(out, strings.Repeat("y", displayTextCapRunes)) {
		t.Fatalf("kept prefix must be the first %d runes", displayTextCapRunes)
	}
	if strings.Count(out, "y") != displayTextCapRunes {
		t.Fatalf("kept body must be exactly %d runes, got %d", displayTextCapRunes, strings.Count(out, "y"))
	}
	if !strings.Contains(out, fmt.Sprintf("原文 %d 字符", total)) {
		t.Fatalf("zh truncation note must carry original rune count %d: %q", total, out[len(out)-80:])
	}
	outEn := capDisplayText(in, "en")
	if !strings.Contains(outEn, fmt.Sprintf("truncated, %d chars total", total)) {
		t.Fatalf("en truncation note must carry original rune count %d", total)
	}
}

// TestCapDisplayText_UTF8Safe pins rune-boundary slicing: a CJK
// payload cut at the cap must remain valid UTF-8 with no split
// multi-byte encoding, and the kept body must be exactly the cap in
// RUNES (not bytes).
func TestCapDisplayText_UTF8Safe(t *testing.T) {
	in := strings.Repeat("汉", displayTextCapRunes+5)
	out := capDisplayText(in, "zh")
	if !utf8.ValidString(out) {
		t.Fatalf("truncated output must be valid UTF-8")
	}
	if got := strings.Count(out, "汉"); got != displayTextCapRunes {
		t.Fatalf("kept CJK body must be %d runes, got %d", displayTextCapRunes, got)
	}
}

// TestCapDisplayText_ByteLongRuneShortCJKUntouched pins the fast-path
// boundary: a CJK string whose BYTE length exceeds the cap but whose
// RUNE count does not must pass through unchanged (the byte-length
// check is only a cheap pre-filter; the authoritative comparison is
// rune-based).
func TestCapDisplayText_ByteLongRuneShortCJKUntouched(t *testing.T) {
	in := strings.Repeat("汉", 1000) // 3000 bytes > cap, 1000 runes < cap
	if len(in) <= displayTextCapRunes {
		t.Fatalf("test setup: byte length must exceed cap")
	}
	if out := capDisplayText(in, "zh"); out != in {
		t.Fatalf("rune-short CJK input must pass through unchanged")
	}
}

// --- formatter integration ---------------------------------------------------

// TestFormatReasoningLines_DegeneratePayloadBounded is the end-to-end
// pin for the customer-visible fix: a 124KB-class same-sentence
// payload entering the shared reasoning formatter (used by TTY
// scrollback, non-TTY stdout AND the [render] INFO log mirror) must
// come out as a handful of bounded lines — collapse first, then cap.
func TestFormatReasoningLines_DegeneratePayloadBounded(t *testing.T) {
	sentence := "The dispatch loop retries the same gate because the gate rejects the dispatch loop."
	payload := strings.Repeat(sentence+"\n", 3000)
	if len(payload) < 100_000 {
		t.Fatalf("test setup: payload must be 100KB-class, got %d bytes", len(payload))
	}
	lines := formatReasoningLines("explorer-1", types.StageExplore, 11, payload, false, "zh")
	total := 0
	joined := ""
	for _, line := range lines {
		total += len(line.text)
		joined += stripAnsiEscapes(line.text) + "\n"
	}
	// Collapsed output = 1 sentence + fold note + prefix decoration;
	// anything over a few KB means the fold did not run.
	if total > 8*1024 {
		t.Fatalf("degenerate payload must render bounded, got %d bytes over %d lines", total, len(lines))
	}
	if !strings.Contains(joined, "×3000") {
		t.Fatalf("fold note with true repeat count missing: %q", joined)
	}
	if got := strings.Count(joined, sentence); got != 1 {
		t.Fatalf("expected exactly 1 surviving sentence copy, got %d", got)
	}
}

// TestFormatReasoningLines_LongDistinctPayloadTruncated pins the cap
// leg for payloads the collapse cannot shrink (all-distinct lines):
// the truncation note must surface in the rendered output.
func TestFormatReasoningLines_LongDistinctPayloadTruncated(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 4*displayTextCapRunes; i++ {
		fmt.Fprintf(&b, "distinct diagnostic line %06d\n", i)
	}
	lines := formatReasoningLines("explorer-1", types.StageExplore, 3, b.String(), false, "zh")
	joined := ""
	for _, line := range lines {
		joined += stripAnsiEscapes(line.text) + "\n"
	}
	if !strings.Contains(joined, "已截断") {
		t.Fatalf("truncation note missing from rendered output: %.200q", joined)
	}
}

// --- same-round duplicate echo skip -------------------------------------------

// TestRenderer_SameRoundContentEchoSkipped pins the renderer-side
// dedupe of the reasoning-then-content double echo. BaseAgent emits
// resp.ReasoningContent and resp.Content as two consecutive
// EventAgentReasoning events; when both carry the same text the second
// is display noise. Multi-line (fenced) payloads are used because the
// single-line shape was already suppressed by lastCommittedLine — the
// multi-line shape is the class this dedupe adds.
func TestRenderer_SameRoundContentEchoSkipped(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()

	payload := "```\nprobe line one\nprobe line two\n```"
	ev := Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentExplorer,
		Stage:     types.StageExplore,
		Iteration: 4,
		Reasoning: payload,
	}
	emit(ev) // reasoning channel
	emit(ev) // content channel mirrors reasoning verbatim → must be skipped
	if got := strings.Count(buf.String(), "probe line one"); got != 1 {
		t.Fatalf("verbatim same-round echo must render once, got %d\n%s", got, buf.String())
	}

	// A DIFFERENT payload in the same round is new information and
	// must still render.
	ev.Reasoning = "```\nanother line\nprobe line three\n```"
	emit(ev)
	if !strings.Contains(buf.String(), "probe line three") {
		t.Fatalf("differing same-round content must not be skipped\n%s", buf.String())
	}

	// The same payload in a NEW round is a fresh statement — the
	// dedupe key includes the iteration, so it renders again.
	ev.Reasoning = payload
	ev.Iteration = 5
	emit(ev)
	if got := strings.Count(buf.String(), "probe line one"); got != 2 {
		t.Fatalf("new-round repeat must render, got %d occurrences\n%s", got, buf.String())
	}
}

// TestRenderer_EchoSkipComparesCollapsedForm pins that the dedupe
// compares the COLLAPSED payloads: a content channel that repeats the
// reasoning sentence a different number of times carries different
// information (different ×N) and must NOT be skipped.
func TestRenderer_EchoSkipComparesCollapsedForm(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()

	base := Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentExplorer,
		Stage:     types.StageExplore,
		Iteration: 7,
	}
	// 4 vs 3 repeats (was 3 vs 2 before the 2026-07-03 WF1 fix: a run
	// of 2 no longer folds, so both payloads must clear the ≥3 fold
	// threshold for the collapse-count comparison to be exercised).
	first := base
	first.Reasoning = "```\nrepeat me\nrepeat me\nrepeat me\nrepeat me\n```"
	emit(first)
	second := base
	second.Reasoning = "```\nrepeat me\nrepeat me\nrepeat me\n```"
	emit(second)
	out := buf.String()
	if !strings.Contains(out, "×4") || !strings.Contains(out, "×3") {
		t.Fatalf("different collapse counts are different payloads; both must render\n%s", out)
	}
}

// --- echo-dedupe lifecycle (2026-07-03 review WF2) -----------------------------

// TestRenderer_EchoDedupeResetOnNewLLMRequest pins the WF2 fix: the
// reasoning→content echo-dedupe state is cleared on every
// EventAgentThinking (the per-request boundary event), so its scope is
// exactly ONE response's two emissions. The regression class: paths
// with a constant iteration — chit-chat is always Iteration 0 — where
// a later turn's REAL reasoning matched the stale key+text and was
// silently swallowed forever.
func TestRenderer_EchoDedupeResetOnNewLLMRequest(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()

	payload := "```\nturn greeting line one\nturn greeting line two\n```"
	reasoning := Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentExplorer,
		Stage:     types.StageExplore,
		Iteration: 0, // chit-chat shape: iteration never advances
		Reasoning: payload,
	}
	thinking := Event{
		Kind:      EventAgentThinking,
		Agent:     types.AgentExplorer,
		Stage:     types.StageExplore,
		Iteration: 0,
	}

	// Turn 1: request starts, reasoning renders, content echo skipped.
	emit(thinking)
	emit(reasoning)
	emit(reasoning) // content channel mirrors reasoning → same response, skipped
	if got := strings.Count(buf.String(), "turn greeting line one"); got != 1 {
		t.Fatalf("same-response echo must render once, got %d\n%s", got, buf.String())
	}

	// Turn 2: a NEW request with the SAME key and SAME text — this is
	// real new reasoning and must render again.
	emit(thinking)
	emit(reasoning)
	if got := strings.Count(buf.String(), "turn greeting line one"); got != 2 {
		t.Fatalf("identical reasoning after a new LLM request must render, got %d\n%s", got, buf.String())
	}
	// ... and turn 2's own content echo is still deduped within the
	// new response.
	emit(reasoning)
	if got := strings.Count(buf.String(), "turn greeting line one"); got != 2 {
		t.Fatalf("turn 2 content echo must still be skipped, got %d\n%s", got, buf.String())
	}
}

// TestRenderer_EchoDedupeResetOnLLMRequestStart pins the same reset on
// the EventLLMRequestStart boundary (direct dispatches that do not go
// through EventAgentThinking).
func TestRenderer_EchoDedupeResetOnLLMRequestStart(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()

	payload := "```\ndirect dispatch prose\nsecond line\n```"
	reasoning := Event{
		Kind:      EventAgentReasoning,
		Agent:     types.AgentExplorer,
		Stage:     types.StageExplore,
		Iteration: 0,
		Reasoning: payload,
	}
	emit(reasoning)
	emit(reasoning) // same response → skipped
	emit(Event{Kind: EventLLMRequestStart, Agent: types.AgentExplorer, Stage: types.StageExplore})
	emit(reasoning) // new request → must render
	if got := strings.Count(buf.String(), "direct dispatch prose"); got != 2 {
		t.Fatalf("EventLLMRequestStart must reset the echo dedupe, got %d\n%s", got, buf.String())
	}
}
