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

// TestRenderMermaidBlocks_DisabledMasterSwitch_PassThrough pins
// the CLI single-shot contract: when SetMermaidRenderingEnabled
// is false, mermaid blocks must ship as source so users piping
// output to file / markdown viewers / mermaid-cli get the
// authoritative source. The REPL path keeps the default-enabled
// behaviour for interactive terminal reading.
func TestRenderMermaidBlocks_DisabledMasterSwitch_PassThrough(t *testing.T) {
	in := "prose\n\n```mermaid\nflowchart LR\n    A --> B\n```\n\nend"
	// Save and restore the master switch so this test doesn't
	// leak state into the other tests in the suite.
	prev := mermaidRenderingEnabled
	t.Cleanup(func() { SetMermaidRenderingEnabled(prev) })

	SetMermaidRenderingEnabled(false)
	out := RenderMermaidBlocks(in)
	if out != in {
		t.Errorf("master switch off must pass mermaid block through unchanged\n  in:  %q\n  out: %q",
			in, out)
	}

	// Re-enable to confirm the toggle works both directions.
	SetMermaidRenderingEnabled(true)
	out2 := RenderMermaidBlocks(in)
	if out2 == in {
		t.Errorf("master switch on must rewrite mermaid block; got unchanged:\n%s", out2)
	}
}

// TestRenderMermaidBlocks_CJKLabelsRenderViaAdapter verifies the
// CJK adapter path: wide-rune labels (CJK / Hiragana / Katakana /
// Hangul / full-width) substitute to 2-byte ASCII placeholders
// before the library renders, then the original wide runes
// restore in the output. Box widths stay aligned because both
// placeholders and wide runes occupy exactly 2 display cells.
func TestRenderMermaidBlocks_CJKLabelsRenderViaAdapter(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[分析器] --> B[探索器]\n```"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("CJK labels must be rendered (not left as source); got unchanged:\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	for _, want := range []string{"分析器", "探索器"} {
		if !strings.Contains(out, want) {
			t.Errorf("CJK label %q must survive restoration in the rendered grid; got:\n%s", want, out)
		}
	}
}

// TestRenderMermaidBlocks_NarrowMultibyteRendersWithFallback pins
// the post-2026-05-02 contract: narrow multi-byte runes (accented
// Latin / emoji / arrows / bullets / dashes / ellipsis) no longer
// abort rendering. Mapped runes (→ → -> etc) substitute via
// narrowRuneFallback; unmapped runes (café's é, emoji) substitute
// to '?'. Either way the diagram renders and the user sees an
// aligned ASCII grid instead of raw mermaid source.
//
// Pre-2026-05-02 this case was "leave as source"; it now must
// render. Reasons: (a) raw source displayed under a ```mermaid```
// fence got chroma-highlighted and looked like a successful
// render, deceiving users; (b) common decoration runes (→ … —)
// now have ASCII equivalents that preserve diagram meaning; (c)
// the failure path's fallback fence still leaves the original
// source visible to the user, just under a ```text``` tag with a
// ⚠ leader.
func TestRenderMermaidBlocks_NarrowMultibyteRendersWithFallback(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[café] --> B\n```"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("narrow multi-byte rune must NOT block render anymore; got unchanged:\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	// Unmapped narrow rune ('é') falls back to '?'; the rest of
	// the box must still render.
	if !strings.Contains(out, "caf?") {
		t.Errorf("expected '?' fallback for unmapped 'é'; got:\n%s", out)
	}
}

// TestRenderMermaidBlocks_ArrowAndDashFallback verifies the most
// common decoration-rune substitutions (→ → "->", … → "...", —
// → "-"). Trigger from the real failure log 2026-05-02:
// `clearAndWrite["清除→写入→重绘"]` froze the entire flowchart
// renderer because '→' was not wide-display.
func TestRenderMermaidBlocks_ArrowAndDashFallback(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[\"清除→写入→重绘\"] --> B[\"end…\"] --> C[\"x — y\"]\n```"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("arrow/ellipsis/dash labels must render via fallback; got unchanged:\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	// Wide CJK runes restore back through the cjkAdapter restore
	// path; narrow decoration runes survive as their ASCII
	// equivalents.
	for _, want := range []string{"清除", "写入", "重绘", "->", "..."} {
		if !strings.Contains(out, want) {
			t.Errorf("expected token %q (CJK preserved or decoration substituted) in:\n%s", want, out)
		}
	}
}

// TestRenderMermaidBlocks_BrTagInLabel verifies the L2 HTML
// normalisation: `<br/>` inside a label gets folded to a space so
// pgavlin's flowchart parser accepts it. Mermaid spec allows
// `<br/>` for multi-line labels; pgavlin's subset does not.
func TestRenderMermaidBlocks_BrTagInLabel(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[\"line1<br/>line2\"] --> B[\"x<br />y\"]\n```"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("<br/> labels must render via normalisation; got unchanged:\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	// "line1 line2" survives as a single label; "<br" must be gone.
	if strings.Contains(out, "<br") {
		t.Errorf("<br/> must be replaced; got raw <br in:\n%s", out)
	}
}

// TestRenderMermaidBlocks_HtmlEntitiesInLabel verifies HTML named
// entities (&amp; &lt; &gt; &nbsp;) collapse to their plain-text
// form via normalizeMermaidLabels.
func TestRenderMermaidBlocks_HtmlEntitiesInLabel(t *testing.T) {
	in := "```mermaid\nflowchart LR\n    A[\"x&amp;y\"] --> B[\"a&lt;b\"]\n```"
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("HTML entities must render via normalisation; got unchanged:\n%s", out)
	}
	if strings.Contains(out, "&amp;") || strings.Contains(out, "&lt;") {
		t.Errorf("HTML entities must be expanded; got raw entity in:\n%s", out)
	}
}

// TestRenderMermaidBlocks_UnsupportedKindShortCircuits verifies the
// L2 routing split: classDiagram / stateDiagram / etc are valid
// Mermaid the LLM is right to emit, but pgavlin's subset cannot
// draw. The renderer short-circuits BEFORE the library call,
// rewrites the fence to ```text``` (so chroma never highlights it),
// and prepends a # ⚠ leader so the user sees an explicit signal.
// The original source survives verbatim for copy-paste into a real
// Mermaid renderer.
func TestRenderMermaidBlocks_UnsupportedKindShortCircuits(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		body    string
	}{
		{"classDiagram", "classDiagram", "classDiagram\n    Animal <|-- Duck\n    Animal <|-- Fish"},
		{"stateDiagram", "stateDiagram", "stateDiagram\n    [*] --> Idle\n    Idle --> Running"},
		{"erDiagram", "erDiagram", "erDiagram\n    CUSTOMER ||--o{ ORDER : places"},
		{"gantt", "gantt", "gantt\n    title A Gantt Diagram\n    section S1\n    task1 :a, 2024-01-01, 30d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := "```mermaid\n" + tc.body + "\n```"
			out := RenderMermaidBlocks(in)
			if out == in {
				t.Fatalf("unsupported kind %q must short-circuit to fallback fence; got unchanged:\n%s", tc.kind, out)
			}
			if !strings.Contains(out, "```text\n") {
				t.Errorf("unsupported kind output must use ```text``` fence (no chroma deception); got:\n%s", out)
			}
			if !strings.Contains(out, "# ·") {
				t.Errorf("unsupported kind output must inject # ⚠ leader; got:\n%s", out)
			}
			if !strings.Contains(out, tc.kind) {
				t.Errorf("unsupported kind output must reference kind %q for the user; got:\n%s", tc.kind, out)
			}
			// Original body content must survive for copy-paste.
			for _, line := range strings.Split(tc.body, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				if !strings.Contains(out, strings.TrimSpace(line)) {
					t.Errorf("original line %q must survive in output; got:\n%s", line, out)
				}
			}
		})
	}
}

// TestRenderMermaidBlocks_FailurePathNeverLeavesMermaidFence is the
// red-line guard: every failure path (parse error, panic,
// unsupported kind, library reject) must rewrite the fence to
// ```text``` so chroma never syntax-highlights mermaid source.
// A bare ```mermaid``` tag escaping past renderer = visual deception
// (user sees colored output, thinks it rendered).
func TestRenderMermaidBlocks_FailurePathNeverLeavesMermaidFence(t *testing.T) {
	cases := []string{
		// Genuinely broken Mermaid that should fail parser.
		"```mermaid\nflowchart LR\n    {unclosed bracket\n```",
		// Unsupported kind.
		"```mermaid\nclassDiagram\n    Animal <|-- Duck\n```",
	}
	for _, in := range cases {
		out := RenderMermaidBlocks(in)
		if strings.Contains(out, "```mermaid") {
			t.Errorf("failure path must NEVER leave ```mermaid fence in output; got:\n%s", out)
		}
	}
}

// TestMermaidStats_CountersIncrementCorrectly verifies the L3
// stats hooks fire on the expected outcomes.
func TestMermaidStats_CountersIncrementCorrectly(t *testing.T) {
	ResetMermaidStats()
	// One success.
	RenderMermaidBlocks("```mermaid\nflowchart LR\n    A --> B\n```")
	// One unsupported kind.
	RenderMermaidBlocks("```mermaid\nclassDiagram\n    A <|-- B\n```")
	// One fallback substitution (arrow rune).
	RenderMermaidBlocks("```mermaid\nflowchart LR\n    A[\"x→y\"] --> B\n```")

	s := GetMermaidStats()
	if s.Attempts < 2 {
		t.Errorf("expected ≥2 attempts (success + fallback), got %d", s.Attempts)
	}
	if s.Succeeded < 2 {
		t.Errorf("expected ≥2 successes (the success and the fallback path also succeeded), got %d", s.Succeeded)
	}
	if got := s.UnsupportedKind["classDiagram"]; got != 1 {
		t.Errorf("expected classDiagram count 1, got %d", got)
	}
	if s.FallbackSubstituted < 1 {
		t.Errorf("expected ≥1 fallback-substituted block, got %d", s.FallbackSubstituted)
	}
	if s.FallbackRunes < 1 {
		t.Errorf("expected ≥1 fallback rune total, got %d", s.FallbackRunes)
	}
}

// TestTryRenderMermaidBlocks_DiagsForEachOutcome verifies the
// per-block diagnostics shape used by the L4 gate.
func TestTryRenderMermaidBlocks_DiagsForEachOutcome(t *testing.T) {
	ResetMermaidStats()
	in := strings.Join([]string{
		"```mermaid",
		"flowchart LR",
		"    A --> B",
		"```",
		"",
		"```mermaid",
		"classDiagram",
		"    A <|-- B",
		"```",
	}, "\n")
	_, diags := TryRenderMermaidBlocks(in)
	if len(diags) < 1 {
		t.Fatalf("expected at least one diagnostic for the unsupported block; got 0")
	}
	foundUnsupported := false
	for _, d := range diags {
		if d.Outcome == OutcomeUnsupportedKind && d.Keyword == "classDiagram" {
			foundUnsupported = true
		}
	}
	if !foundUnsupported {
		t.Errorf("expected OutcomeUnsupportedKind diag for classDiagram; got: %+v", diags)
	}
}

// TestRenderMermaidBlocks_FlattensSubgraphs pins the subgraph
// flatten compatibility shim. Models routinely emit
// `subgraph cluster ... end` for architectural questions even
// though the skill prompt warns the renderer subset doesn't
// support it. Pre-2026-04-30 the library rejected and the user
// saw raw Mermaid source. Now the renderer strips the cluster
// declarations and renders a flat flowchart instead.
func TestRenderMermaidBlocks_FlattensSubgraphs(t *testing.T) {
	in := strings.Join([]string{
		"```mermaid",
		"flowchart TD",
		"    Start[\"启动\"]",
		"    subgraph PreStages [\"条件预阶段\"]",
		"        Pre1[\"日志诊断\"]",
		"        Pre2[\"性能诊断\"]",
		"    end",
		"    subgraph Main [\"主流水线\"]",
		"        A[\"分析\"]",
		"        B[\"探索\"]",
		"    end",
		"    Start --> Pre1",
		"    Pre1 --> A",
		"    A --> B",
		"```",
	}, "\n")
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("subgraph mermaid block must be rendered (was passed through):\n%s", out)
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	// Every node label must survive the flatten + render.
	for _, want := range []string{"启动", "日志诊断", "性能诊断", "分析", "探索"} {
		if !strings.Contains(out, want) {
			t.Errorf("flattened render must contain node %q; got:\n%s", want, out)
		}
	}
}

// TestRenderMermaidBlocks_DoesNotTouchOtherFences verifies that
// fenced blocks tagged with anything other than `mermaid` are
// preserved byte-identical UNLESS the body is unambiguously a
// mermaid diagram (first non-empty line is a known diagram-type
// keyword like `flowchart`/`sequenceDiagram`). The model commonly
// drops the `mermaid` tag when surrounding prose names the diagram
// type; we render those bodies anyway. Non-mermaid-shaped content
// (raw arrows, shell, JSON) still passes through untouched.
func TestRenderMermaidBlocks_DoesNotTouchOtherFences(t *testing.T) {
	cases := []string{
		"```\nA --> B\n```",         // bare fence; body NOT a known diagram keyword
		"```text\nA --> B\n```",     // text fence; same
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

// TestRenderMermaidBlocks_UntaggedFlowchartRendered pins the
// session-fix for "model emits flowchart LR without the mermaid
// tag". User reported a final answer that printed `flowchart LR`
// as raw text because the fence was bare. We now detect mermaid
// keyword bodies inside bare ``` fences AND `text`-tagged fences
// and render them through the library.
func TestRenderMermaidBlocks_UntaggedFlowchartRendered(t *testing.T) {
	cases := []string{
		"prose\n\n```\nflowchart LR\n    A --> B --> C\n```\n\nend",
		"prose\n\n```text\nflowchart LR\n    A --> B --> C\n```\n\nend",
	}
	for _, in := range cases {
		out := RenderMermaidBlocks(in)
		if out == in {
			t.Errorf("untagged flowchart must be rendered (was passed through):\n%s", out)
			continue
		}
		if !strings.Contains(out, "```text\n") {
			t.Errorf("rendered output must rewrap fence as ```text```; got:\n%s", out)
		}
		for _, want := range []string{"prose", "end", "A", "B", "C"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected token %q in rendered output:\n%s", want, out)
			}
		}
	}
}

// TestRenderMermaidBlocks_UntaggedNonMermaidUnchanged guards the
// narrow contract: untagged fences whose body does NOT begin with
// a known mermaid keyword must NOT be touched. This prevents the
// detector from over-reaching into ASCII art / pseudo-code blocks.
func TestRenderMermaidBlocks_UntaggedNonMermaidUnchanged(t *testing.T) {
	cases := []string{
		"```\nA --> B (raw arrow, no flowchart keyword)\n```",
		"```\nfoo bar baz\n```",
		"```\n+----+\n| ok |\n+----+\n```",
	}
	for _, in := range cases {
		out := RenderMermaidBlocks(in)
		if out != in {
			t.Errorf("untagged non-mermaid fence must pass through unchanged\n  in:  %q\n  out: %q",
				in, out)
		}
	}
}
