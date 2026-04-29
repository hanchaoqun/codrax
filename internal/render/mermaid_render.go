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
	// Two-stage prefilter so chitchat / fallback prose carrying
	// non-mermaid fenced blocks (```bash```, ```json```, ```go```)
	// short-circuits BEFORE the regex scan:
	//
	//   1. Must contain at least one fenced block ("```").
	//   2. Must contain either an explicit `mermaid` tag OR one of
	//      the body-keyword markers used by looksLikeMermaidBody.
	//
	// Step 2 is a fast strings.Contains on the source — a single
	// pass over the text — instead of running the regex on every
	// fence in long replies that have no diagram. If neither
	// indicator is present the function returns text byte-identical.
	if !strings.Contains(text, "```") {
		return text
	}
	if !mayContainMermaid(text) {
		return text
	}
	return fencedBlockRe.ReplaceAllStringFunc(text, maybeReplaceMermaidFence)
}

// mayContainMermaid is the cheap prefilter for RenderMermaidBlocks.
// True iff the text either carries a `mermaid` tag or a known
// diagram-type keyword anywhere — both quick substring checks. A
// false positive here just costs one regex scan; a false negative
// would silently skip rendering, so the keyword set MUST stay in
// sync with mermaidBodyKeywords (the per-block decision used inside
// looksLikeMermaidBody).
func mayContainMermaid(text string) bool {
	if strings.Contains(text, "```mermaid") {
		return true
	}
	for _, kw := range mermaidBodyKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// fencedBlockRe matches any fenced code block. We dispatch in
// maybeReplaceMermaidFence based on info-string + body shape so a
// model-emitted bare ``` fence whose body starts with a mermaid
// diagram-type keyword still gets rendered. The capture group holds
// the info-string (info[0]) and the body (info[1]).
//
// Pre-2026-04-30 the regex was tag-anchored at ```mermaid only, so
// the user reported that a model-emitted bare ``` fence containing
// `flowchart LR ...` printed as raw text. The model commonly drops
// the tag when the surrounding prose already names the diagram type;
// we cope by detecting mermaid-shaped bodies regardless of tag.
//
// Pattern shape:
//   ``` <info-string-zero-or-more-chars>
//   <BODY>
//   ```
var fencedBlockRe = regexp.MustCompile("(?s)```([^\\n]*)\\n(.*?)\\n```")

// mermaidFenceRe is retained for the original ```mermaid``` shape.
// Used by replaceMermaidFence's body extraction (it lifts the body
// from match using string indexing, not the regex captures, so
// callers downstream remain stable).
var mermaidFenceRe = regexp.MustCompile("(?s)```mermaid[^\\n]*\\n(.*?)\\n```")

// mermaidBodyKeywords lists the diagram-type tokens that mark a
// mermaid block. The first non-empty body line of an untagged fence
// must begin with one of these (case-sensitive — mermaid spec).
// We deliberately omit shorter / ambiguous tokens like "graph"
// alone might match (it's the umbrella alias for flowchart in
// mermaid, but rare in modern diagrams). Including it explicitly
// here is fine because any false-positive falls through to
// replaceMermaidFence's library-render error path which leaves the
// block as source.
var mermaidBodyKeywords = []string{
	"flowchart",
	"graph",
	"sequenceDiagram",
	"classDiagram",
	"stateDiagram",
	"stateDiagram-v2",
	"erDiagram",
	"journey",
	"gantt",
	"pie",
	"mindmap",
	"timeline",
	"gitGraph",
	"requirementDiagram",
	"C4Context",
}

// looksLikeMermaidBody returns true when body's first non-empty
// trimmed line begins with a known mermaid diagram-type keyword.
// Used to opt untagged fences into mermaid rendering.
func looksLikeMermaidBody(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, kw := range mermaidBodyKeywords {
			if line == kw || strings.HasPrefix(line, kw+" ") || strings.HasPrefix(line, kw+"\t") {
				return true
			}
		}
		return false
	}
	return false
}

// maybeReplaceMermaidFence dispatches a fenced-block match to either
// the mermaid renderer or pass-through. Three cases:
//
//   1. info-string starts with `mermaid` → render via library
//   2. info-string is empty / `text` AND body looks like mermaid →
//      render via library (the model dropped the tag)
//   3. anything else → leave unchanged (untouched contract for
//      `go`, `bash`, `json`, `diff`, etc.)
func maybeReplaceMermaidFence(match string) string {
	// Find newline that ends the info-string line.
	nl := strings.Index(match, "\n")
	if nl < 0 {
		return match
	}
	infoLine := strings.TrimSpace(match[3:nl]) // skip leading ```
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyEnd <= nl {
		return match
	}
	body := match[nl+1 : bodyEnd]

	// Case 1: explicit mermaid tag.
	if strings.HasPrefix(infoLine, "mermaid") {
		return replaceMermaidFence(match)
	}
	// Case 2: untagged or `text`-tagged fence whose body shape is
	// mermaid. We only match the empty-info-string and `text` cases
	// to keep the contract narrow — a body with `flowchart` keywords
	// inside a ```bash``` fence is unambiguously not a diagram.
	if (infoLine == "" || infoLine == "text") && looksLikeMermaidBody(body) {
		// Synthesize a `mermaid`-tagged match so the renderer's
		// body-extraction logic stays load-bearing on a single
		// shape. We use the explicit (rendered, ok) signal — NOT
		// string-equality against the synthesized form — so the
		// failure path returns the ORIGINAL match (not the synth)
		// regardless of how the failure manifests inside the
		// renderer.
		synth := "```mermaid\n" + body + "\n```"
		if rendered, ok := renderMermaidFenceBody(synth); ok {
			return rendered
		}
		return match
	}
	return match
}


// replaceMermaidFence is the per-match closure handed to
// ReplaceAllStringFunc. `match` is the full fence (including the
// opening ```mermaid line and closing ```).
//
// On success it returns the rewritten fence (```text``` wrapping the
// rendered ASCII grid). On failure it returns `match` unchanged —
// strict no-regression contract.
//
// Callers that need to distinguish success from "left unchanged on
// purpose" (e.g. maybeReplaceMermaidFence which synthesises a fake
// `mermaid`-tagged match for untagged-but-mermaid-shaped bodies and
// must NOT leak the synthesised form back to the user when render
// fails) should use renderMermaidFenceBody directly — that helper
// returns an explicit (rendered, ok) pair so success vs failure is
// not inferred from string identity.
func replaceMermaidFence(match string) string {
	if rendered, ok := renderMermaidFenceBody(match); ok {
		return rendered
	}
	return match
}

// renderMermaidFenceBody is the explicit (rendered, ok) core of
// mermaid → ASCII transformation. ok=false means the caller should
// keep the ORIGINAL source unchanged (whatever shape it was — bare
// fence, text-tagged fence, mermaid-tagged fence). This separates
// "successfully rendered" from "no-op return value" so callers
// don't have to compare strings to detect failure.
//
// All four failure modes (panic, missing body, library reject,
// empty render) collapse to ok=false here.
func renderMermaidFenceBody(match string) (out string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warning("[render/mermaid] panic during render (block left unchanged): %v", r)
			out = ""
			ok = false
		}
	}()

	// Extract body. The regex guarantees match shape; body lives
	// between the first newline after ```mermaid and the closing
	// ```.
	bodyStart := strings.Index(match, "\n")
	if bodyStart < 0 {
		return "", false
	}
	bodyEnd := strings.LastIndex(match, "\n```")
	if bodyEnd <= bodyStart {
		return "", false
	}
	body := strings.TrimSpace(match[bodyStart+1 : bodyEnd])
	if body == "" {
		return "", false
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
		return "", false
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
		return "", false
	}
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return "", false
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
	return b.String(), true
}
