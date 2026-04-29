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
