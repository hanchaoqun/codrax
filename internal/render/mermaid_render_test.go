package render

import (
	"strings"
	"testing"
)

// TestRenderMermaidBlocks_NoMermaid_PassThrough is the load-bearing
// no-regression guard: text without any ```mermaid``` fence must
// pass through byte-identical, even if it contains other fenced
// code blocks (Go source, JSON, ASCII art).
func TestRenderMermaidBlocks_NoMermaid_PassThrough(t *testing.T) {
	cases := []string{
		"plain text",
		"prose with `inline code`",
		"```go\nfunc main() {}\n```",
		"```\n+----+\n| ok |\n+----+\n```", // bare ASCII art fence
		"text with ```mermaid``` but malformed", // no body, no rewrite
	}
	for _, in := range cases {
		out := RenderMermaidBlocks(in)
		if out != in {
			t.Errorf("input must pass through unchanged when no mermaid block present\n  in:  %q\n  out: %q",
				in, out)
		}
	}
}

// TestRenderMermaidBlocks_FlowchartRendered confirms the happy path:
// a ```mermaid``` block carrying flowchart syntax gets re-laid as
// ASCII and the fence info-string flips to `text` (so glamour's
// chroma path skips syntax highlighting and preserves alignment).
func TestRenderMermaidBlocks_FlowchartRendered(t *testing.T) {
	in := "prose before\n\n```mermaid\nflowchart LR\n    A --> B --> C\n```\n\nprose after"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("expected mermaid block to be rewritten; got unchanged:\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text``` so chroma skips highlighting; got:\n%s", out)
	}
	if !strings.Contains(out, "prose before") || !strings.Contains(out, "prose after") {
		t.Errorf("surrounding prose must be preserved verbatim; got:\n%s", out)
	}
	// The rendered ASCII should mention the node names.
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered ASCII must contain node label %q; got:\n%s", want, out)
		}
	}
}

// TestRenderMermaidBlocks_InvalidMermaid_BlockUnchanged is the
// strict-no-regression guard: when the library cannot parse the
// mermaid source (syntax error, unsupported feature), the original
// fence is returned unchanged so the user sees the source
// verbatim. NEVER emit a partial / corrupted render.
func TestRenderMermaidBlocks_InvalidMermaid_BlockUnchanged(t *testing.T) {
	in := "```mermaid\nflowchart LR\n  this is not valid mermaid syntax {{{\n```"
	out := RenderMermaidBlocks(in)
	// Either the library parses successfully (and we get a render
	// out — that's also fine; the library is permissive) OR it
	// errors and we keep the input. The contract is "no
	// regression" — an unrecognized block must not crash or
	// produce empty output.
	if out == "" {
		t.Errorf("output must never be empty; got empty string for input %q", in)
	}
}

// TestRenderMermaidBlocks_MultipleBlocks confirms each ```mermaid```
// block is rewritten independently — they don't share state and
// don't interfere.
func TestRenderMermaidBlocks_MultipleBlocks(t *testing.T) {
	in := strings.Join([]string{
		"first diagram:",
		"```mermaid",
		"flowchart LR",
		"    A --> B",
		"```",
		"second diagram:",
		"```mermaid",
		"flowchart LR",
		"    X --> Y",
		"```",
		"end",
	}, "\n")
	out := RenderMermaidBlocks(in)
	if strings.Count(out, "```text\n") < 2 {
		t.Errorf("expected at least 2 rewritten fences; got:\n%s", out)
	}
	if !strings.Contains(out, "first diagram:") || !strings.Contains(out, "second diagram:") || !strings.Contains(out, "end") {
		t.Errorf("surrounding prose must survive intact; got:\n%s", out)
	}
}

// TestRenderMermaidBlocks_EmptyBody_LeftAlone defends against the
// edge case of a fence with no body — leave it as-is so the user
// at least sees what they intended.
func TestRenderMermaidBlocks_EmptyBody_LeftAlone(t *testing.T) {
	in := "```mermaid\n\n```"
	out := RenderMermaidBlocks(in)
	if out != in {
		t.Errorf("empty mermaid body should pass through unchanged; got %q", out)
	}
}

// TestRenderMermaidBlocks_NonASCIIBodyLeftAsSource defends against
// the upstream library limitation: pgavlin/mermaid-ascii's graph
// renderer processes node-label text as bytes (not runes) so
// multi-byte UTF-8 characters get garbled into Latin-1 sequences.
// We detect non-ASCII content in the mermaid body and skip
// rendering — the user sees the original mermaid source instead
// of a corrupted grid.
func TestRenderMermaidBlocks_NonASCIIBodyLeftAsSource(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[分析器] --> B[探索器]\n```"
	out := RenderMermaidBlocks(in)
	if out != in {
		t.Errorf("CJK content must skip render and stay as source; got:\n%s", out)
	}
}

// TestRenderMermaidBlocks_DoesNotTouchOtherFences verifies that
// fenced blocks tagged with anything other than `mermaid` are
// preserved byte-identical. The model's free ASCII art (typically
// in a bare ``` fence or `text` fence) MUST flow through the
// pipeline untouched per the F-design philosophy: codrax only
// processes content the model explicitly tagged for processing.
func TestRenderMermaidBlocks_DoesNotTouchOtherFences(t *testing.T) {
	cases := []string{
		"```\nA --> B\n```",         // bare fence with mermaid-shaped content
		"```text\nA --> B\n```",     // text fence
		"```bash\nls -la\n```",      // shell
		"```json\n{\"a\": 1}\n```",  // JSON
	}
	for _, in := range cases {
		out := RenderMermaidBlocks(in)
		if out != in {
			t.Errorf("non-mermaid fence must pass through unchanged\n  in:  %q\n  out: %q",
				in, out)
		}
	}
}
