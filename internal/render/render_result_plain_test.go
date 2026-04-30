package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderResult_PlainTextSkipsGlamour locks the plain-text
// passthrough path. Stage hooks that surface fail-loud diagnostics
// containing identifier-like tokens (e.g. "emit_change_plan") use
// SetResultPlain so the renderer skips glamour markdown — chroma's
// underscore-aware tokenizer would otherwise split the identifier
// into ANSI-colored fragments. Pre-2026-04-29 production trace
// /home/chatpp/pytest 2026-04-29 06:03 captured the broken render:
// the message "planner did not call emit_change_plan" appeared as
// "[ANSI]emit_[/ANSI][ANSI]change_[/ANSI][ANSI]plan[/ANSI]".
func TestRenderResult_PlainTextSkipsGlamour(t *testing.T) {
	r := New(nil, false)
	mut := types.NewMutableState("x")
	// Use SetResultPlain — the renderer must NOT pass this through
	// glamour.
	mut.SetResultPlain("plan stage completed but no ChangePlan was installed on Mutable (planner did not call emit_change_plan)")

	bus := &types.BusContext{Mutable: mut}
	out := r.RenderResult(bus)
	// The full identifier must survive intact, no ANSI fragmentation.
	if !strings.Contains(out, "emit_change_plan") {
		t.Errorf("plain-text result must preserve identifier; got %q", out)
	}
	// No ANSI escape sequences that fragment the identifier.
	// (`[38;5;XXXm` is glamour/chroma's color escape.)
	if strings.Contains(out, "\x1b[38;5;") {
		t.Errorf("plain-text result must not carry chroma ANSI codes; got %q", out)
	}
}

// TestRenderResult_MarkdownStillRendered confirms the regular
// (non-plain) path still glamour-renders content. Real LLM answers
// use SetResult and rely on styled markdown for headings / code
// blocks / tables. We can't easily assert specific ANSI codes
// (depends on glamour theme + terminal), but we can verify the
// raw markdown markers were processed.
func TestRenderResult_MarkdownStillRendered(t *testing.T) {
	r := New(nil, true) // forceColor so glamour produces ANSI even on non-TTY test runner
	mut := types.NewMutableState("x")
	mut.SetResult("**bold** text and `inline code`")

	bus := &types.BusContext{Mutable: mut}
	out := r.RenderResult(bus)
	// Glamour SHOULD have processed the bold marker — output should
	// not contain the literal `**` since glamour renders it as
	// styled bold.
	if strings.Contains(out, "**bold**") {
		t.Errorf("markdown result with **bold** must be glamour-rendered; got %q", out)
	}
}

// TestRenderResult_PaletteSurfacesEveryElement is the integration
// smoke test for the post-2026-04-30 palette redesign. It pushes a
// markdown sample exercising every element the new palette covers
// (H1-H3, Strong, Emph, Strikethrough, inline Code, Link,
// BlockQuote, unordered + ordered list, HR) through the real
// glamour pipeline and asserts the output is non-empty AND not
// byte-identical to the input. A regression that broke glamour
// integration would manifest as either empty output or pass-through
// of literal markdown markers.
//
// We don't pin specific ANSI escape sequences here — the unit-level
// style_palette_test.go pins individual color slots, so any drift
// in those triggers there. This integration test only verifies the
// pipeline still glues together for every element type.
func TestRenderResult_PaletteSurfacesEveryElement(t *testing.T) {
	r := New(nil, true) // forceColor so glamour emits ANSI on non-TTY
	mut := types.NewMutableState("x")
	mut.SetResult(strings.Join([]string{
		"# Title H1",
		"",
		"## Section H2",
		"",
		"### Subsection H3",
		"",
		"**Strong text** with *emphasis* and ~~strikethrough~~ and `inline code`.",
		"",
		"[A link](https://example.com) and a [Linked text](https://example.com/x).",
		"",
		"> Block quote with `inline code` inside.",
		"",
		"- Unordered item one",
		"- Unordered item two",
		"",
		"1. Ordered first",
		"2. Ordered second",
		"",
		"---",
		"",
		"Trailing prose.",
	}, "\n"))

	bus := &types.BusContext{Mutable: mut}
	out := r.RenderResult(bus)
	if out == "" || out == "(no result)\n" {
		t.Fatalf("RenderResult must produce non-empty output for the sampler markdown; got %q", out)
	}
	// Every literal content token must survive the render — the
	// palette should style, not erase content.
	// Glamour may insert leading indents or wrap content for block
	// elements (H1 padding, Item bullets, BlockQuote `│` prefix), so
	// we look for word-level fragments that survive any reasonable
	// reflow / styling, not multi-word phrases that block-formatting
	// could split across lines.
	// Single-word matchers only — glamour splits multi-word strings
	// across separate ANSI segments (`Unordered item` + ` one`),
	// so substrings spanning a word boundary fail even when content
	// is fully present.
	for _, want := range []string{
		"Title",
		"Section",
		"Subsection",
		"Strong",
		"emphasis",
		"strikethrough",
		"inline code",
		"link",
		"Linked",
		"Block",
		"Unordered",
		"Ordered",
		"Trailing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing content token %q", want)
		}
	}
	// The output must contain ANSI escape sequences — proof that
	// glamour styled the prose rather than passing it through raw.
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("rendered output contains no ANSI escapes — palette is not applying; output: %q", out)
	}
	// Markdown markers MUST be consumed by the renderer. Literal
	// `**`, `~~`, leading `#` should not survive into the output.
	for _, banned := range []string{"**Strong text**", "~~strikethrough~~", "# Title H1"} {
		if strings.Contains(out, banned) {
			t.Errorf("rendered output retained literal markdown marker %q — glamour did not consume it", banned)
		}
	}
}

// TestMutableState_PlainFlagToggles locks the SetResult /
// SetResultPlain semantics. SetResult clears the flag; subsequent
// SetResultPlain re-asserts it; SetResult after SetResultPlain
// clears it again.
func TestMutableState_PlainFlagToggles(t *testing.T) {
	mut := types.NewMutableState("x")
	if mut.ResultIsPlain() {
		t.Error("zero state should be non-plain")
	}
	mut.SetResultPlain("plain")
	if !mut.ResultIsPlain() {
		t.Error("SetResultPlain must flip the flag")
	}
	mut.SetResult("markdown")
	if mut.ResultIsPlain() {
		t.Error("SetResult must clear the flag")
	}
}
