package render

import (
	"regexp"
	"strings"

	"github.com/pgavlin/mermaid-ascii/pkg/diagram"
	"github.com/pgavlin/mermaid-ascii/pkg/render"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// RenderMermaidBlocks scans `text` for fenced code blocks tagged
// `mermaid` and rewrites each block in place: the mermaid source
// is rendered to deterministically-aligned ASCII via the
// pgavlin/mermaid-ascii library, then the fence is rewritten to
// `text` (so glamour's chroma path skips syntax highlighting and
// preserves monospace alignment).
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

// hasNonASCII reports whether s contains any byte >= 0x80. Faster
// than utf8.ValidString — we only care that something multi-byte
// is present, not whether it's well-formed UTF-8.
func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

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
	// processes node-label text as bytes (not runes), so multi-
	// byte UTF-8 characters get split and rendered as garbled
	// Latin-1 byte sequences. Detect any non-ASCII content and
	// skip — leave the mermaid source as-is so the user reads
	// the original (and a markdown viewer can still render it
	// natively). Strict no-regression: a corrupted render is
	// strictly worse than the original source.
	if hasNonASCII(body) {
		logging.Info("[render/mermaid] non-ASCII content detected; library would garble — block left as source")
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

	rendered, err := render.Render(body, cfg)
	if err != nil {
		logging.Warning("[render/mermaid] render failed (block left unchanged): %v", err)
		return match
	}
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return match
	}

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
