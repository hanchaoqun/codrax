package render

import (
	"regexp"
	"strings"

	"github.com/pgavlin/mermaid-ascii/pkg/diagram"
	"github.com/pgavlin/mermaid-ascii/pkg/render"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// mermaidRenderingEnabled is the master switch for the
// mermaid → ASCII transformation. Default true (REPL / interactive
// terminal mode); set to false by single-shot CLI runners so the
// model's mermaid source ships verbatim to stdout. CLI users
// commonly pipe codrax output to a file, a markdown renderer, a
// mermaid-cli, or paste into a doc — all of those consume mermaid
// source natively and would not benefit from a pre-baked ASCII
// grid. REPL users read the answer immediately in a terminal so
// the ASCII transformation is the right default.
//
// Process-global because there is exactly one render mode per
// invocation; cmd/root.go's runSingleShot toggles it off before
// orchestrator dispatch.
var mermaidRenderingEnabled = true

// SetMermaidRenderingEnabled is the master switch toggle. Pass
// false to disable the mermaid → ASCII transformation; mermaid
// fenced blocks then pass through unchanged.
func SetMermaidRenderingEnabled(enabled bool) {
	mermaidRenderingEnabled = enabled
}

// RenderMermaidBlocks scans `text` for fenced code blocks tagged
// `mermaid` and rewrites each block in place: the mermaid source
// is rendered to deterministically-aligned ASCII via the
// pgavlin/mermaid-ascii library, then the fence is rewritten to
// `text` (so glamour's chroma path skips syntax highlighting and
// preserves monospace alignment). Returns text unchanged when the
// master switch (mermaidRenderingEnabled) is off.
//
// Failure semantics — strictly safe / no regression possible:
//
//   - Library returns an error → block left unchanged (user sees
//     mermaid source verbatim, no worse than before)
//   - Renderer panics on a corner-case DAG → recover() catches,
//     block left unchanged, warning logged
//   - Block is empty / whitespace-only → left unchanged
//   - No `mermaid` blocks present → text returned unchanged
//
// The function is the integration point for the answer pipeline:
// emit_answer_document calls it once on the AnswerDocument.Summary
// after all other normalisation passes have run, so the rendered
// ASCII flows through the rest of the rendering chain untouched.
//
// Cross-platform: pgavlin/mermaid-ascii is pure Go (single import
// dep, MIT licensed), uses runewidth for width calculations, no
// syscalls or shell-out. Output is the same on every platform.
func RenderMermaidBlocks(text string) string {
	if !mermaidRenderingEnabled {
		return text
	}
	if text == "" {
		return text
	}
	if !strings.Contains(text, "```mermaid") {
		return text
	}
	return mermaidFenceRe.ReplaceAllStringFunc(text, replaceMermaidFence)
}

// mermaidFenceRe matches a fenced block whose info-string starts
// with `mermaid` (case-sensitive, mermaid spec). The capture group
// holds the body (without trailing newline). `(?s)` makes `.`
// match newlines so the body spans multiple lines. Non-greedy `+?`
// handles two adjacent blocks correctly.
//
// Pattern shape:
//   ``` mermaid          ← optional whitespace tolerated after the tag
//   <BODY>
//   ```
var mermaidFenceRe = regexp.MustCompile("(?s)```mermaid[^\\n]*\\n(.*?)\\n```")


// replaceMermaidFence is the per-match closure handed to
// ReplaceAllStringFunc. `match` is the full fence (including the
// opening ```mermaid line and closing ```).
//
// The function lifts the body, attempts to render via the library,
// and on success rewrites the fence to:
//
//   ```text
//   <RENDERED ASCII>
//   ```
//
// On any failure the original `match` is returned unchanged —
// strict no-regression contract.
func replaceMermaidFence(match string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warning("[render/mermaid] panic during render (block left unchanged): %v", r)
			out = match
		}
	}()

	// Extract body. The regex guarantees match shape; body lives
	// between the first newline after ```mermaid and the closing
	// ```.
	bodyStart := strings.Index(match, "\n")
	if bodyStart < 0 {
		return match
	}
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyEnd <= bodyStart {
		return match
	}
	body := strings.TrimSpace(match[bodyStart+1 : bodyEnd])
	if body == "" {
		return match
	}

	// Library limitation: pgavlin/mermaid-ascii's graph renderer
	// width-counts node labels by byte length (not runewidth), so
	// multi-byte UTF-8 characters break the box arithmetic. We
	// route through a CJK adapter that pre-substitutes every
	// wide rune (CJK / Hiragana / Katakana / Hangul / full-width)
	// with a 2-byte ASCII placeholder, calls the library, and
	// restores the original characters in the rendered output.
	// Cell arithmetic stays consistent because each wide rune
	// occupies exactly 2 display cells AND each 2-byte ASCII
	// placeholder occupies exactly 2 display cells, so the box
	// widths the library computes match what the terminal draws.
	//
	// When the adapter cannot safely substitute (narrow
	// multi-byte rune like accented Latin or emoji, or the
	// placeholder pool is exhausted), we fall back to the
	// original "leave as source" path — the user reads readable
	// mermaid syntax, never garbled bytes.
	adapter := newCJKAdapter(body)
	preparedBody, _, substErr := adapter.substitute(body)
	if substErr != nil {
		logging.Info("[render/mermaid] cjk adapter cannot substitute (%v); block left as source", substErr)
		return match
	}

	cfg := diagram.DefaultConfig()
	// Force ASCII-only rendering. The library defaults to Unicode
	// box-drawing characters which are East-Asian-Width AMBIGUOUS
	// — exactly the alignment failure mode we are solving by
	// going through the library in the first place. ASCII-only
	// output is unambiguously 1-cell per char on every terminal /
	// locale / monospace font.
	cfg.UseAscii = true
	// CLI style (no HTML colour markup); plain text body.
	cfg.StyleType = "cli"

	rendered, err := render.Render(preparedBody, cfg)
	if err != nil {
		logging.Warning("[render/mermaid] render failed (block left unchanged): %v", err)
		return match
	}
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return match
	}
	// Restore wide runes in the rendered grid. The adapter walks
	// the placeholder map and byte-replaces each occurrence; the
	// pool was chosen to exclude any 2-byte pair already present
	// in the source so we don't risk false-positive replacements.
	rendered = adapter.restore(rendered)

	// Rewrap as `text` fence so chroma doesn't tokenize the box-
	// drawing chars as code (which it would if the fence stayed
	// `mermaid` or had no tag and chroma auto-detected). `text` is
	// an explicit "no syntax highlighting" hint chroma honours.
	var b strings.Builder
	b.Grow(len(rendered) + 16)
	b.WriteString("```text\n")
	b.WriteString(rendered)
	b.WriteString("\n```")
	return b.String()
}
