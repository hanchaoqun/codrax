package preview

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// RenderMarkdownHTML converts markdown into safe HTML for the local
// preview page. Raw HTML from the markdown stays disabled by goldmark's
// default renderer; the only custom HTML emitted here is for fenced code
// blocks, where mermaid fences become <div class="mermaid"> nodes and
// ordinary fences become escaped <pre><code>.
func RenderMarkdownHTML(markdown []byte) (string, error) {
	var out bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(fencedCodeRenderer{}, 500),
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
	}), nil
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
	_, _ = fmt.Fprint(w, "<pre><code")
	if cls := safeCodeClass(lang); cls != "" {
		_, _ = fmt.Fprintf(w, ` class="language-%s"`, cls)
	}
	_, _ = fmt.Fprint(w, ">")
	_, _ = fmt.Fprint(w, stdhtml.EscapeString(body))
	_, _ = fmt.Fprint(w, "</code></pre>\n")
	return ast.WalkSkipChildren, nil
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
