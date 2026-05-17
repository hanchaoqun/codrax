package preview

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strconv"
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
	info := string(block.Info.Segment.Value(source))
	lang := firstInfoToken(info)
	body := fencedCodeBody(block, source)
	if strings.EqualFold(lang, "mermaid") {
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
	body = mermaidcompat.NormalizeSequenceStops(body)
	return normalizeBrowserMermaidSubgraphs(body)
}

func prependBrowserMermaidInfoDirective(info, body string) string {
	directive := browserMermaidInfoDirective(info)
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

func browserMermaidInfoDirective(info string) string {
	info = strings.TrimSpace(info)
	if !strings.EqualFold(firstInfoToken(info), "mermaid") {
		return ""
	}
	if len(info) <= len("mermaid") {
		return ""
	}
	return strings.TrimSpace(info[len("mermaid"):])
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

// normalizeBrowserMermaidSubgraphs repairs the common LLM shape
// `subgraph Explorer System` into Mermaid's explicit
// `subgraph Explorer_System [Explorer System]` form. It only touches
// flowchart/graph bodies and only when the subgraph title has multiple
// bare tokens; already explicit Mermaid forms are preserved.
func normalizeBrowserMermaidSubgraphs(body string) string {
	if !strings.Contains(body, "subgraph") || !browserMermaidIsFlowchart(body) {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		leading := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
		indent := line[:leading]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "subgraph ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "subgraph "))
		if !browserMermaidNeedsSubgraphTitleRepair(rest) {
			continue
		}
		lines[i] = indent + "subgraph " + browserMermaidSubgraphID(rest, i) + " [" + rest + "]"
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

func browserMermaidIsFlowchart(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line == "graph" || strings.HasPrefix(line, "graph ") ||
			line == "flowchart" || strings.HasPrefix(line, "flowchart ")
	}
	return false
}

func browserMermaidNeedsSubgraphTitleRepair(rest string) bool {
	if rest == "" || strings.ContainsAny(rest, "[]\"") {
		return false
	}
	return len(strings.Fields(rest)) > 1
}

func browserMermaidSubgraphID(title string, lineIndex int) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range title {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	id := strings.Trim(b.String(), "_")
	if id == "" {
		id = "subgraph"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "sg_" + id
	}
	return id + "_" + strconv.Itoa(lineIndex+1)
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
