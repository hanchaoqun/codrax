package preview

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/hanchaoqun/codrax/internal/markdownext"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// RenderMarkdownHTML converts markdown into safe HTML for the local
// preview page. Raw HTML from the markdown is never emitted as markup:
// rawHTMLLiteralRenderer re-emits it as ESCAPED literal text (see its
// doc for the ruling), so tokens like "<anonymous>" / "Vec<int>" stay
// visible while keeping a zero-execution surface. The only custom HTML
// emitted here is renderer-authored scaffolding: fenced code blocks
// (mermaid fences become <div class="mermaid"> nodes, ordinary fences
// escaped <pre><code>) and the §29.9 aux-reference appendix (pointer
// lines + <section class="aux">, see markdown_auxfold.go).
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
			),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(fencedCodeRenderer{}, 500),
				util.Prioritized(rawHTMLLiteralRenderer{}, 500),
				util.Prioritized(auxRefRenderer{}, 500),
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
	projectionTree := isTraceCausalProjectionFence(info, body)
	if projectionTree {
		// The generated trace tree is a horizontally scrollable, information-
		// dense region in the standalone report.  Give the page CSS and assistive
		// technology a precise hook without changing one byte of the authoritative
		// Markdown fence shared by terminal / Markdown / HTML surfaces.
		_, _ = fmt.Fprint(w, `<pre class="trace-projection-tree" role="region" aria-label="Trace causal projection tree" tabindex="0"><code`)
	} else {
		_, _ = fmt.Fprint(w, "<pre><code")
	}
	if cls := safeCodeClass(lang); cls != "" {
		_, _ = fmt.Fprintf(w, ` class="language-%s"`, cls)
	}
	_, _ = fmt.Fprint(w, ">")
	if projectionTree {
		writeTraceProjectionGrid(w, body)
	} else {
		_, _ = fmt.Fprint(w, stdhtml.EscapeString(body))
	}
	_, _ = fmt.Fprint(w, "</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// isTraceCausalProjectionFence recognizes only the deterministic ```text
// shape emitted by runtimeTraceProjTreeFence.  The scale declaration is the
// second half of the signature: a customer-authored text diagram that happens
// to begin with the same root glyph must keep ordinary code-block styling.
// This remains presentation-only; it neither rewrites nor reflows the fence.
func isTraceCausalProjectionFence(info, body string) bool {
	if !strings.EqualFold(firstInfoToken(info), "text") {
		return false
	}
	first := firstNonEmptyLine(body)
	hasScale := strings.Contains(body, "满格=") || strings.Contains(body, "bar full =")
	if !hasScale {
		return false
	}
	if strings.HasPrefix(first, "⊚ ") && (strings.Contains(first, "‹用户关注线程›") || strings.Contains(first, "‹分析锚点线程›") || strings.Contains(first, "<user-focused thread>") || strings.Contains(first, "<analysis anchor thread>")) {
		return true
	}
	for _, prefix := range []string{
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

// writeTraceProjectionGrid makes the HTML face independent of installed CJK
// monospace fonts. Browser fallback may draw Han, box-drawing and ordinal
// glyphs from different faces; inline cells pin every rune to the 1/2-column
// geometry already used by the deterministic tree renderer. The text itself
// is unchanged and remains copyable/accessibility-visible.
func writeTraceProjectionGrid(w util.BufWriter, body string) {
	for _, r := range body {
		switch r {
		case '\r':
			continue
		case '\n':
			_, _ = fmt.Fprint(w, "\n")
			continue
		}
		width := runewidth.RuneWidth(r)
		if width < 0 {
			width = 1
		}
		if width > 2 {
			width = 2
		}
		_, _ = fmt.Fprintf(w, `<span class="trace-cell trace-cell-%d">%s</span>`, width, stdhtml.EscapeString(string(r)))
	}
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
