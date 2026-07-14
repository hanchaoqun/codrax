package preview

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/hanchaoqun/codrax/internal/markdownext"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// RenderMarkdownHTML converts markdown into safe HTML for the local
// preview page. Raw HTML from the markdown is never emitted as markup:
// rawHTMLLiteralRenderer re-emits it as ESCAPED literal text (see its
// doc for the ruling), so tokens like "<anonymous>" / "Vec<int>" stay
// visible while keeping a zero-execution surface. The only custom HTML
// emitted here is renderer-authored scaffolding: fenced code blocks
// (mermaid fences become <div class="mermaid"> nodes, ordinary fences
// escaped <pre><code>), the §29.9 aux-reference appendix (pointer
// lines + <section class="aux">, see markdown_auxfold.go), and exact-title
// runtime-trace audit regions used only for compact report presentation (see
// markdown_trace_sections.go).
func RenderMarkdownHTML(markdown []byte) (string, error) {
	var out bytes.Buffer
	md := goldmark.New(
		// extension.GFM minus stock Strikethrough: the stock parser
		// opens <del> on a SINGLE tilde, mis-rendering prose range
		// connectors like "6~11ms" (ruling 2026-07-05: single "~"
		// never strikes; "~~" keeps GFM semantics). See
		// internal/markdownext. Do not collapse back to extension.GFM.
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Table,
			extension.TaskList,
			markdownext.StrikethroughDoubleTildeOnly,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			// §29.9 aux-reference appendix: exact marker paragraphs
			// ("树读法:"/"各列口径:") plus their following list move
			// to a document-end 「阅读参考」 appendix, leaving an
			// in-place pointer line — HTML face only; see
			// markdown_auxfold.go for the closed set and ruling.
			parser.WithASTTransformers(
				util.Prioritized(auxFoldTransformer{}, 500),
				// Run after auxFoldTransformer: any legend/glossary pair
				// relocated to the appendix must not remain inside a compact
				// projection-detail region at its former location.
				util.Prioritized(traceAuditSectionTransformer{}, 600),
				// Run after traceAuditSectionTransformer: E# anchor pairing
				// needs the detail/evidence section wrappers in place
				// (v5 P0 档1, markdown_trace_anchors.go).
				util.Prioritized(traceEvidenceAnchorTransformer{}, 700),
			),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(fencedCodeRenderer{}, 500),
				util.Prioritized(rawHTMLLiteralRenderer{}, 500),
				util.Prioritized(auxRefRenderer{}, 500),
				util.Prioritized(traceAuditSectionRenderer{}, 500),
			),
		),
	)
	if err := md.Convert(markdown, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

// RenderStandaloneMarkdownHTML converts markdown into a single self-contained
// HTML document. It reuses the same renderer as the live preview server, but
// inlines the Mermaid browser runtime so the saved file can be opened directly
// without a local server, CDN, or markdown application.
func RenderStandaloneMarkdownHTML(title string, markdown []byte) (string, error) {
	body, err := RenderMarkdownHTML(markdown)
	if err != nil {
		return "", err
	}
	mermaidJS, err := assets.ReadFile(mermaidAssetPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(title) == "" {
		title = "Codrax Markdown Preview"
	}
	return renderHTMLPage(pageArgs{
		Title:     title,
		BodyHTML:  body,
		MermaidJS: string(mermaidJS),
		Lang:      markdownDocumentLanguage(markdown),
	}), nil
}

func markdownDocumentLanguage(markdown []byte) string {
	var han, latin int
	for _, r := range string(markdown) {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r):
			latin++
		}
	}
	// Language is a document-level presentation choice. A Chinese entity,
	// path or thread name inside an otherwise English report must not flip the
	// page chrome. Four Han characters plus a dominant-share threshold keeps
	// short mixed identifiers neutral while recognizing ordinary zh prose.
	if han >= 4 && han*3 >= latin {
		return "zh-CN"
	}
	return "en"
}

type fencedCodeRenderer struct{}

func (f fencedCodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, f.renderFencedCodeBlock)
}

func (f fencedCodeRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block, ok := node.(*ast.FencedCodeBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	info := ""
	if block.Info != nil {
		info = string(block.Info.Segment.Value(source))
	}
	lang := firstInfoToken(info)
	body := fencedCodeBody(block, source)
	if browserShouldRenderMermaid(info, body) {
		_, _ = fmt.Fprint(w, `<div class="mermaid">`+"\n")
		_, _ = fmt.Fprint(w, stdhtml.EscapeString(normalizeBrowserMermaid(info, body)))
		_, _ = fmt.Fprint(w, "\n</div>\n")
		return ast.WalkSkipChildren, nil
	}
	elimOverview := isTraceElimOverviewFence(info)
	projectionTree := isTraceCausalProjectionFence(info, body) || elimOverview
	switch {
	case elimOverview:
		// ELIM-1 (RANK-U Stage 2, 2026-07-13): the ◎ 窗内可消除量总览 fence
		// shares the projection grid treatment (same class = same monospace
		// grid CSS) plus its own hook class; classified on its OWN typed
		// second info token — a brand-new form with no archives, so no
		// content-sniffing fallback exists for it.
		_, _ = fmt.Fprint(w, `<pre class="trace-projection-tree trace-elim-overview" role="region" aria-label="Trace eliminable-in-window overview" tabindex="0"><code`)
	case projectionTree:
		// The generated trace tree is a horizontally scrollable, information-
		// dense region in the standalone report.  Give the page CSS and assistive
		// technology a precise hook without changing one byte of the authoritative
		// Markdown fence shared by terminal / Markdown / HTML surfaces.
		_, _ = fmt.Fprint(w, `<pre class="trace-projection-tree" role="region" aria-label="Trace causal projection tree" tabindex="0"><code`)
	default:
		_, _ = fmt.Fprint(w, "<pre><code")
	}
	if cls := safeCodeClass(lang); cls != "" {
		_, _ = fmt.Fprintf(w, ` class="language-%s"`, cls)
	}
	_, _ = fmt.Fprint(w, ">")
	if projectionTree {
		writeTraceProjectionGrid(w, body, traceProjectionAnchorPairing(block))
	} else {
		_, _ = fmt.Fprint(w, stdhtml.EscapeString(body))
	}
	_, _ = fmt.Fprint(w, "</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// isTraceCausalProjectionFence recognizes the deterministic tree fence minted
// by runtimeTraceProjTreeFence (internal/tool).
//
// v5 P0 hard gate (重-3, user ruling 2026-07-11, design
// causal_tree_v5_design_20260711.md §C.3): the generator's opener carries a
// typed second info token (```text trace-causal-projection); EXACT equality
// on that token IS the classification — a precise signal, per the CLAUDE.md
// red line (hard gates read precise signals). Content sniffing is DEMOTED to
// the legacy fallback below, which exists only so ARCHIVED reports rendered
// before the token keep their trace-tree presentation.
// Presentation-only either way: the fence is never rewritten or reflowed.
func isTraceCausalProjectionFence(info, body string) bool {
	if !strings.EqualFold(firstInfoToken(info), "text") {
		return false
	}
	if secondInfoToken(info) == tracefence.InfoToken {
		return true
	}
	return isLegacyTraceCausalProjectionBody(body)
}

// isTraceElimOverviewFence recognizes the ◎ 窗内可消除量总览 fence minted by
// runtimeTraceProjElimOverviewFence (internal/tool; ELIM-1, RANK-U Stage 2).
// EXACT typed-token equality only — the overview post-dates the token era, so
// no legacy body-sniffing arm exists (or may ever be added) for it.
func isTraceElimOverviewFence(info string) bool {
	return strings.EqualFold(firstInfoToken(info), "text") &&
		secondInfoToken(info) == tracefence.ElimInfoToken
}

// isLegacyTraceCausalProjectionBody is the DEMOTED content-sniffing lane
// (archive recognition only — new fences classify on the typed info token
// above). The scale declaration is the second half of the signature: a
// customer-authored text diagram that happens to begin with the same root
// glyph must keep ordinary code-block styling.
//
// EVOLUTION RECORD (v5 P0 备-2, 2026-07-11, supersedes the UXG-0 D1
// hand-mirrored list): the whitelist now DERIVES from internal/tracefence —
// the same constants the generator emits — so the generator-emittable head
// set and this fallback can never drift apart (census-pinned by the
// strip-token arm of TestUXG0FenceHeadsClassifiedByPreview, internal/tool).
// Only the pre-UXR-1 archive heads stay as local literals: they are no
// longer generator-emittable and exist purely for reports archived before
// 2026-07-11.
func isLegacyTraceCausalProjectionBody(body string) bool {
	first := firstNonEmptyLine(body)
	hasScale := false
	for _, mark := range tracefence.ScaleNoteMarkers() {
		if strings.Contains(body, mark) {
			hasScale = true
			break
		}
	}
	if !hasScale {
		return false
	}
	if strings.HasPrefix(first, tracefence.RootGlyph+" ") {
		for _, chip := range tracefence.TargetProvenanceChips() {
			if strings.Contains(first, chip) {
				return true
			}
		}
	}
	for _, head := range tracefence.FlatFallbackHeads() {
		if strings.HasPrefix(first, legacyFlatHeadPrefix(head)) {
			return true
		}
	}
	for _, prefix := range []string{
		// Pre-UXR-1 flat heads, kept ADDITIVELY so archived reports rendered
		// before 2026-07-11 keep their trace-tree presentation.
		"(睡眠区间在查询窗内无 sched_wakeup 记录",
		"(本报告未做唤醒链下钻",
		"(唤醒链路径未解析",
		"(the sleep interval has no sched_wakeup record",
		"(no wakeup-chain drilldown was run",
		"(wakeup path unresolved",
	} {
		if strings.HasPrefix(first, prefix) {
			return true
		}
	}
	return false
}

// legacyFlatHeadPrefix cuts a generator banner head after its first opening
// parenthesis: archives minted while a 短因 parenthetical wording was
// evolving keep classifying as long as the 短结论 head is intact — the
// exact pre-token UXG-0 D1 matching behavior, now derived instead of
// hand-mirrored. Heads without a parenthetical match whole.
// F6 (fix round 2026-07-11): the cut set covers BOTH paren families —
// fullwidth （ U+FF08 and ASCII ( U+0028 (the current closed set spells
// its parens ASCII; the fullwidth arm future-proofs a zh wording
// evolution). It was previously spelled as two ASCII parens.
func legacyFlatHeadPrefix(head string) string {
	if i := strings.IndexAny(head, "\uff08("); i >= 0 {
		_, size := utf8.DecodeRuneInString(head[i:])
		return head[:i+size]
	}
	return head
}

func secondInfoToken(info string) string {
	fields := strings.FieldsFunc(strings.TrimSpace(info), unicode.IsSpace)
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

// writeTraceProjectionGrid makes the HTML face independent of installed CJK
// monospace fonts. The text itself is unchanged (textContent == fence bytes)
// and remains copyable/accessibility-visible.
//
// v5 P0 run-segmentation model (design causal_tree_v5_design_20260711.md
// §C.3/C-9, user ruling 2026-07-11) — decoration granularity is the token
// run, five classes:
//
//  1. pure-ASCII runs   — ONE span pinned to its whole width (mono ASCII
//     metrics are reliable; collapsing per-rune cells cuts the DOM ~5x);
//  2. box-drawing rails — per-rune 1ch cells (C-9: fallback fonts draw the
//     U+2500 series full-width, so run-level pinning would overlap ink);
//  3. CJK/mixed runes   — per-rune 1/2ch cells (the only reliable answer
//     when the user's font is not CJK 2:1);
//  4. state-mark envelope slots — a directory glyph and its generator-
//     guaranteed companion space fuse into ONE 2ch inline-grid slot
//     (重-1; T-6 honest cut: only "glyph + existing companion space" pairs;
//     rank badges ❶..❺ keep their 1ch chip cells until P2a);
//  5. bar blocks █▒░    — per-rune 1ch cells (block glyphs are full-cell
//     ink by design, the one physically-correct 1ch mark class).
//
// Renderer-authored token surfaces (rank chips / channel ordinals / action
// words / E# refs) keep their exact grid widths. 档1 decorations (T-5): each
// physical line wraps in a .trace-line span (newlines stay OUTSIDE the span);
// ◇/▒ stanza heads add .trace-stanza-head; [E#] tokens become in-page anchor
// links when the anchor transformer paired this fence with its detail/
// evidence sections AND the ordinal claimed a target id (F5: unclaimed
// ordinals stay plain runs — never a dangling link).
func writeTraceProjectionGrid(w util.BufWriter, body string, anchor *traceAnchorPairing) {
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			_, _ = fmt.Fprint(w, "\n")
		}
		if line == "" {
			continue
		}
		classes := "trace-line"
		if strings.HasPrefix(line, "◇ ") || strings.HasPrefix(line, "▒ ") ||
			strings.HasPrefix(line, tracefence.ElimGlyph+" ") {
			classes += " trace-stanza-head"
		}
		_, _ = fmt.Fprintf(w, `<span class="%s">`, classes)
		writeTraceProjectionLineRuns(w, line, anchor)
		_, _ = fmt.Fprint(w, "</span>")
	}
}

func writeTraceProjectionLineRuns(w util.BufWriter, line string, anchor *traceAnchorPairing) {
	var ascii strings.Builder
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		run := ascii.String()
		_, _ = fmt.Fprintf(w, `<span class="trace-run" style="width:%dch">%s</span>`,
			len(run), stdhtml.EscapeString(run))
		ascii.Reset()
	}
	for offset := 0; offset < len(line); {
		if token, rank, width, adjacent, ok := traceProjectionRankToken(line, offset); ok {
			flushASCII()
			badge := false
			for _, glyph := range tracefence.BadgeGlyphs() {
				if strings.HasPrefix(token, glyph) {
					badge = true
					break
				}
			}
			switch {
			case badge && strings.HasPrefix(line[offset+len(token):], " "):
				// CAL-1 件⑥a (2026-07-12, T-6 fulfilled early): the badge and
				// its UXG-0 D5 companion space fuse into ONE 2ch inline-grid
				// envelope slot — the same v5 P0 geometry as the state marks.
				// The colored pill stretches to the full 2ch, overflow stays
				// visible and the dingbat ink is never clipped; textContent
				// keeps both bytes.
				_, _ = fmt.Fprintf(w, `<span class="trace-slot trace-rank-pill trace-rank-%d"><span class="trace-ink">%s </span></span>`,
					rank, stdhtml.EscapeString(token))
				offset += len(token) + 1
				continue
			case badge:
				// No companion space (pre-D5 archives): 1ch standalone slot,
				// centered, overflow visible — the space-less icon fallback.
				_, _ = fmt.Fprintf(w, `<span class="trace-slot trace-slot-1 trace-rank-pill trace-rank-%d"><span class="trace-ink">%s</span></span>`,
					rank, stdhtml.EscapeString(token))
			case adjacent:
				// UXG-0 D2: ◇ channel ordinal — shared chip geometry, neutral
				// channel color class instead of a per-rank root-cause color
				// (§29.36.2: ordinals never compare across channels).
				_, _ = fmt.Fprintf(w, `<span class="trace-rank-ordinal trace-rank-adjacent trace-rank-width-%d">%s</span>`,
					width, stdhtml.EscapeString(token))
			default:
				_, _ = fmt.Fprintf(w, `<span class="trace-rank-ordinal trace-rank-%d trace-rank-width-%d">%s</span>`,
					rank, width, stdhtml.EscapeString(token))
			}
			offset += len(token)
			continue
		}
		if token, width, ok := traceProjectionActionToken(line, offset); ok {
			flushASCII()
			_, _ = fmt.Fprintf(w, `<span class="trace-action-token trace-action-width-%d">%s</span>`,
				width, stdhtml.EscapeString(token))
			offset += len(token)
			continue
		}
		if token, first, ok := traceProjectionEvidenceRefToken(line, offset); ok {
			flushASCII()
			if anchor != nil && anchor.claimed[first] {
				// 档1 E# anchor link (pure-attribute decoration): jumps to the
				// paired detail stanza / evidence index entry. Only CLAIMED
				// ordinals link (F5) — every minted href has a real target.
				_, _ = fmt.Fprintf(w, `<a class="trace-eref" style="width:%dch" href="#%se%d">%s</a>`,
					len(token), anchor.prefix, first, stdhtml.EscapeString(token))
			} else {
				_, _ = fmt.Fprintf(w, `<span class="trace-run" style="width:%dch">%s</span>`,
					len(token), stdhtml.EscapeString(token))
			}
			offset += len(token)
			continue
		}
		r, size := utf8.DecodeRuneInString(line[offset:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if r == '\r' {
			offset += size
			continue
		}
		if class := traceProjectionIconClass(r); class != "" {
			flushASCII()
			if strings.HasPrefix(line[offset+size:], " ") {
				// 2ch envelope: the mark plus its companion space are ONE slot;
				// textContent keeps both bytes.
				_, _ = fmt.Fprintf(w, `<span class="trace-slot trace-icon trace-icon-%s"><span class="trace-ink">%s </span></span>`,
					class, stdhtml.EscapeString(string(r)))
				offset += size + 1
				continue
			}
			// No companion space (⊘ inside the ⊘链止 keep-mark): 1ch slot,
			// centered, overflow stays visible — never clipped.
			_, _ = fmt.Fprintf(w, `<span class="trace-slot trace-slot-1 trace-icon trace-icon-%s"><span class="trace-ink">%s</span></span>`,
				class, stdhtml.EscapeString(string(r)))
			offset += size
			continue
		}
		if r >= 0x20 && r < 0x7f {
			ascii.WriteByte(byte(r))
			offset += size
			continue
		}
		flushASCII()
		width := runewidth.RuneWidth(r)
		if width < 0 {
			width = 1
		}
		if width > 2 {
			width = 2
		}
		switch {
		case r >= 0x2500 && r <= 0x257f:
			_, _ = fmt.Fprintf(w, `<span class="trace-cell trace-cell-%d trace-rail">%s</span>`, width, stdhtml.EscapeString(string(r)))
		case r == '█' || r == '▒' || r == '░':
			_, _ = fmt.Fprintf(w, `<span class="trace-cell trace-cell-%d trace-bar">%s</span>`, width, stdhtml.EscapeString(string(r)))
		default:
			_, _ = fmt.Fprintf(w, `<span class="trace-cell trace-cell-%d">%s</span>`, width, stdhtml.EscapeString(string(r)))
		}
		offset += size
	}
	flushASCII()
}

// traceProjectionEvidenceRefToken recognizes the renderer-authored evidence
// locator token — [E7], the merged_ids form [E8(+6)], and the folded-twin
// compound [E7(+1)+E8] — an ASCII-only closed grammar. Returns the token and
// its FIRST evidence ordinal (the anchor target). Anything deviating from
// the grammar stays on the plain ASCII-run path.
func traceProjectionEvidenceRefToken(line string, offset int) (string, int, bool) {
	rest := line[offset:]
	if !strings.HasPrefix(rest, "[E") {
		return "", 0, false
	}
	i := 2
	digits := func() (int, bool) {
		start := i
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == start {
			return 0, false
		}
		n, err := strconv.Atoi(rest[start:i])
		if err != nil {
			return 0, false
		}
		return n, true
	}
	mergeGroup := func() bool { // optional "(+N)"
		if i >= len(rest) || rest[i] != '(' {
			return true
		}
		j := i + 1
		if j >= len(rest) || rest[j] != '+' {
			return false
		}
		j++
		start := j
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j == start || j >= len(rest) || rest[j] != ')' {
			return false
		}
		i = j + 1
		return true
	}
	first, ok := digits()
	if !ok || !mergeGroup() {
		return "", 0, false
	}
	for i < len(rest) && rest[i] == '+' {
		i++
		if i >= len(rest) || rest[i] != 'E' {
			return "", 0, false
		}
		i++
		if _, ok := digits(); !ok || !mergeGroup() {
			return "", 0, false
		}
	}
	if i >= len(rest) || rest[i] != ']' {
		return "", 0, false
	}
	return rest[:i+1], first, true
}

// traceProjectionIconClass resolves a rune against the single-source
// state-mark directory (internal/tracefence, UXG-1 M1 — this function was the
// hand-copied preview mirror of the tool-side §24.3 glyph table until
// 2026-07-12). Only that exact set rides the v5 P0 envelope slot (2ch with
// its companion space, 1ch when standing alone) — the authoritative
// character, textContent and grid geometry stay unchanged. Unknown/customer
// text remains on the ordinary rune/run path. The UXG-0 D3 exclusions
// (⚠/◇/▒ deliberately not boxed) are documented on the directory itself.
func traceProjectionIconClass(r rune) string {
	return tracefence.StateMarkClass(r)
}

// traceProjectionActionToken highlights only the generator's actionable
// optimization word inside a causal tree (single source:
// tracefence.ActionTokens, UXG-1 M1). Width is the existing terminal-grid
// width, so the HTML emphasis cannot move any following metric column.
func traceProjectionActionToken(line string, offset int) (string, int, bool) {
	rest := line[offset:]
	for _, token := range tracefence.ActionTokens() {
		if strings.HasPrefix(rest, token) {
			return token, runewidth.StringWidth(token), true
		}
	}
	return "", 0, false
}

// traceProjectionRankToken recognizes only renderer-authored rank surfaces.
// A circled badge remains one fixed grid cell and clips its own fallback glyph,
// preventing it from overprinting the adjacent state icon without consuming
// label space. The ordinal arm highlights only
// #1..#5 immediately following the closed zh/en channel-worded seat phrases;
// arbitrary names and evidence ids are never restyled. §29.27.1: the badge
// family is ❶..❺ (TOP-5, badge follows the seat).
//
// UXG-0 D2 (2026-07-11): the §29.36.2 ◇ channel ordinal (邻近影响#N /
// adjacent-impact #N) is recognized as its own arm — adjacent=true — so the
// chip keeps the shared ordinal geometry but wears the NEUTRAL channel color
// (user ruling: 同样式、中性色区分通道) instead of a root-cause rank color.
// UXG-1 M1 (2026-07-12): the channel phrases and the badge family now come
// from internal/tracefence — the same constants runtimeTraceProjSeatChannelWord
// and runtimeTraceProjBadgeGlyph (internal/tool) consume; the hand copy UXG-0
// D2 flagged as "the last one allowed" is retired (zh chips are word#N, en
// chips word␣#N).
func traceProjectionRankToken(line string, offset int) (token string, rank, width int, adjacent, ok bool) {
	rest := line[offset:]
	for rank, token := range tracefence.BadgeGlyphs() {
		if strings.HasPrefix(rest, token) {
			return token, rank + 1, 1, false, true
		}
	}
	previous := line[:offset]
	switch {
	case strings.HasSuffix(previous, tracefence.SeatChannelChainZH) ||
		strings.HasSuffix(previous, tracefence.SeatChannelChainEN+" "):
		adjacent = false
	case strings.HasSuffix(previous, tracefence.SeatChannelAdjacentZH) ||
		strings.HasSuffix(previous, tracefence.SeatChannelAdjacentEN+" "):
		adjacent = true
	default:
		return "", 0, 0, false, false
	}
	for rank := 1; rank <= 5; rank++ {
		token := fmt.Sprintf("#%d", rank)
		if !strings.HasPrefix(rest, token) {
			continue
		}
		if len(rest) > len(token) && rest[len(token)] >= '0' && rest[len(token)] <= '9' {
			continue
		}
		return token, rank, 2, adjacent, true
	}
	return "", 0, 0, false, false
}

// rawHTMLLiteralRenderer renders raw-HTML markdown nodes as ESCAPED
// literal text instead of goldmark's safe-mode "<!-- raw HTML omitted -->"
// placeholder.
//
// Answer prose legitimately carries angle-bracket tokens that goldmark
// parses as raw HTML — JS stack frames ("<anonymous>"), generics
// ("Vec<int>"), template instantiations — and the safe-mode placeholder
// silently destroyed that load-bearing information (customer-visible as
// "raw HTML omitted" in the middle of a stack trace). goldmark's
// html.WithUnsafe is NOT an acceptable fix: it would emit the markup
// verbatim and reopen the XSS surface.
//
// Ruling (RFH #66): escape-and-display, never drop. Content passes
// through stdhtml.EscapeString so the browser shows the literal token
// ("&lt;anonymous&gt;" renders as "<anonymous>") with a zero-execution
// surface — the anti-XSS guarantee is "never executed", not "never
// displayed". Pinned by TestRenderMarkdownHTMLEscapesRawHTMLInstead-
// OfDropping and the anti-XSS assertions in server_test.go.
type rawHTMLLiteralRenderer struct{}

func (r rawHTMLLiteralRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
}

func (r rawHTMLLiteralRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*ast.RawHTML)
	if !ok {
		return ast.WalkContinue, nil
	}
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		_, _ = w.WriteString(stdhtml.EscapeString(string(segment.Value(source))))
	}
	return ast.WalkSkipChildren, nil
}

func (r rawHTMLLiteralRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*ast.HTMLBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	if entering {
		for i := 0; i < n.Lines().Len(); i++ {
			segment := n.Lines().At(i)
			_, _ = w.WriteString(stdhtml.EscapeString(string(segment.Value(source))))
		}
	} else if n.HasClosure() {
		_, _ = w.WriteString(stdhtml.EscapeString(string(n.ClosureLine.Value(source))))
	}
	return ast.WalkContinue, nil
}

func fencedCodeBody(block *ast.FencedCodeBlock, source []byte) string {
	lines := block.Lines()
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
	}
	return b.String()
}

func normalizeBrowserMermaid(info, body string) string {
	body = strings.TrimRight(body, "\n")
	body = prependBrowserMermaidInfoDirective(info, body)
	return mermaidcompat.NormalizeSourceForMarkdown(body)
}

func browserShouldRenderMermaid(info, body string) bool {
	info = strings.TrimSpace(info)
	if strings.EqualFold(firstInfoToken(info), "mermaid") {
		return true
	}
	if mermaidcompat.InfoLineStartsWithKeyword(info) {
		return true
	}
	if info == "" || strings.EqualFold(info, "text") {
		return mermaidcompat.LooksLikeBody(body)
	}
	return false
}

func prependBrowserMermaidInfoDirective(info, body string) string {
	directive, _ := mermaidcompat.InfoLineDirective(info)
	if directive == "" {
		return body
	}
	directiveToken := firstInfoToken(directive)
	bodyToken := firstInfoToken(firstNonEmptyLine(body))
	if directiveToken == "" || strings.EqualFold(directiveToken, bodyToken) {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return directive
	}
	return directive + "\n" + body
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func firstInfoToken(info string) string {
	info = strings.TrimSpace(info)
	if info == "" {
		return ""
	}
	fields := strings.FieldsFunc(info, unicode.IsSpace)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0])
}

func safeCodeClass(lang string) string {
	if lang == "" {
		return ""
	}
	for _, r := range lang {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '+' || r == '.' {
			continue
		}
		return ""
	}
	return stdhtml.EscapeString(lang)
}
