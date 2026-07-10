package preview

// Runtime-trace reports deliberately put decisions and causal leads before
// their lossless audit appendix. The appendix is essential, but a long
// per-node roster should not have the same visual weight as the conclusion.
// This HTML-only transformer gives two deterministic, generated H2 chapters a
// presentation hook:
//
//   - causal projection detail: compact two-column audit region on wide screens
//   - evidence index: compact single-column reference region
//
// Markdown and terminal output stay unchanged. Recognition is an exact closed
// set, optionally followed by the generator's exact " — <artifact>" suffix;
// no keyword/fuzzy inference drives the structural switch. The detail wrapper
// ends at the next top-level H1/H2; the evidence wrapper additionally ends at
// its OWN generated body's end (heading + one intro paragraph + one bullet
// list). Every original child node and its order are preserved.
//
// EVOLUTION RECORD (审计 #56, §29.25 处置委托 + §29.26 待主会话落账,
// 2026-07-10): the evidence index is the report's LAST H2 by pinned design, so
// the former "wrap until the next H1/H2" loop swallowed every trailing
// disclosure surface — `> ` caveat blockquotes, the `**说明**：`/`**引用**：`/
// `**关键代码**：` bold paragraphs and the §29.9 阅读参考 appendix (a custom
// auxAppendix node appended before this transformer runs at priority 600) —
// into the visually subordinate .86rem audit box, demoting adjudicated
// disclosure surfaces (§29.9/§29.14/§29.17). The evidence wrapper now stops at
// the exact structural shape its generator emits (renderV2ListOrTableHeading:
// H2 + intro paragraph + bullet list); anything else stays a document-level
// sibling. Both wrappers also refuse to consume the aux custom nodes.

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type traceAuditSectionClass uint8

const (
	traceAuditSectionNone traceAuditSectionClass = iota
	traceAuditSectionDetail
	traceAuditSectionEvidence
)

var kindTraceAuditSection = ast.NewNodeKind("CodraxTraceAuditSection")

type traceAuditSectionBlock struct {
	ast.BaseBlock
	Class traceAuditSectionClass
}

func (n *traceAuditSectionBlock) Kind() ast.NodeKind { return kindTraceAuditSection }

func (n *traceAuditSectionBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Class": n.className()}, nil)
}

func (n *traceAuditSectionBlock) className() string {
	if n == nil {
		return ""
	}
	switch n.Class {
	case traceAuditSectionDetail:
		return "trace-projection-detail"
	case traceAuditSectionEvidence:
		return "trace-projection-evidence"
	default:
		return ""
	}
}

type traceAuditSectionTransformer struct{}

func (traceAuditSectionTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	if doc == nil || reader == nil {
		return
	}
	source := reader.Source()
	type candidate struct {
		heading *ast.Heading
		class   traceAuditSectionClass
	}
	var candidates []candidate
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Level != 2 {
			continue
		}
		class := traceAuditHeadingClass(strings.TrimSpace(inlinePlainText(heading, source)))
		if class != traceAuditSectionNone {
			candidates = append(candidates, candidate{heading: heading, class: class})
		}
	}

	for _, candidate := range candidates {
		heading := candidate.heading
		if heading == nil || heading.Parent() != doc {
			continue
		}
		section := &traceAuditSectionBlock{Class: candidate.class}
		doc.InsertBefore(doc, heading, section)
		paragraphs, lists := 0, 0
		for node := ast.Node(heading); node != nil; {
			next := node.NextSibling()
			if node != heading && !traceAuditSectionAdmitsChild(candidate.class, node, source, &paragraphs, &lists) {
				break
			}
			doc.RemoveChild(doc, node)
			section.AppendChild(section, node)
			if candidate.class == traceAuditSectionEvidence && lists >= 1 {
				// The generated evidence body ends at its own bullet list.
				break
			}
			node = next
		}
	}
}

// traceAuditTrailingDisclosureMarkers — the exact renderer-owned heading words
// of the trailing document-level disclosure surfaces (single producers:
// renderAnswerDocV2Caveats / MissingRequestedRoles / Citations / Snippets in
// internal/render/answerdoc.go render each as one bold-only paragraph).
// Matching requires BOTH the leading Strong emphasis node and the exact word
// prefix — a generated detail stanza heading (**[E1] name**) never matches.
var traceAuditTrailingDisclosureMarkers = []string{
	"说明：", "Caveats:",
	"缺失的请求层：", "Missing requested layers:",
	"引用：", "Citations:",
	"关键代码：", "Key snippets:",
}

// traceAuditTrailingDisclosureParagraph reports whether the node is one of the
// closed-set trailing disclosure heading paragraphs (复核 R2 hardening).
func traceAuditTrailingDisclosureParagraph(node ast.Node, source []byte) bool {
	paragraph, ok := node.(*ast.Paragraph)
	if !ok {
		return false
	}
	if first := paragraph.FirstChild(); first == nil || first.Kind() != ast.KindEmphasis {
		return false
	}
	text := strings.TrimSpace(inlinePlainText(paragraph, source))
	for _, marker := range traceAuditTrailingDisclosureMarkers {
		if strings.HasPrefix(text, marker) {
			return true
		}
	}
	return false
}

// traceAuditSectionAdmitsChild decides whether one document-level sibling
// belongs INSIDE the audit wrapper (审计 #56 termination fix + 复核 R2 detail
// hardening, §29.25 处置委托 + §29.26 待主会话落账, 2026-07-10). Both classes
// stop at the next H1/H2, never consume the §29.9 aux custom nodes, refuse
// every node kind their generators cannot emit (caveat blockquotes, fences,
// thematic breaks, raw HTML), and refuse the exact closed-set trailing
// disclosure headings. The evidence class additionally admits only its
// generator's exact body shape — at most ONE intro paragraph followed by at
// most ONE bullet list (renderV2ListOrTableHeading). The detail class body is
// unbounded paragraph/list stanzas (`**[E#] name**` heading + roster list
// pairs from runtimeTraceProjDetailFullText) plus legacy H3+ sub-headings —
// those stay admitted (复核 R2: the former form consumed EVERYTHING up to the
// next H1/H2, so a detail chapter without a following evidence block was the
// report's last H2 and swallowed the trailing disclosures).
func traceAuditSectionAdmitsChild(class traceAuditSectionClass, node ast.Node, source []byte, paragraphs, lists *int) bool {
	if nextHeading, ok := node.(*ast.Heading); ok && nextHeading.Level <= 2 {
		return false
	}
	switch node.Kind() {
	case kindAuxPointer, kindAuxAppendix, kindAuxSourceLabel:
		return false
	}
	if traceAuditTrailingDisclosureParagraph(node, source) {
		return false
	}
	if class != traceAuditSectionEvidence {
		switch node.(type) {
		case *ast.Paragraph, *ast.List, *ast.Heading:
			return true
		default:
			return false
		}
	}
	switch node.(type) {
	case *ast.Paragraph:
		if *paragraphs >= 1 || *lists >= 1 {
			return false
		}
		*paragraphs++
		return true
	case *ast.List:
		if *lists >= 1 {
			return false
		}
		*lists++
		return true
	default:
		return false
	}
}

func traceAuditHeadingClass(title string) traceAuditSectionClass {
	for _, base := range []string{
		"因果投影明细(逐节点完整属性)",
		"Causal Projection Detail (full attributes per node)",
	} {
		if traceGeneratedHeadingMatches(title, base) {
			return traceAuditSectionDetail
		}
	}
	for _, base := range []string{"证据索引", "Evidence Index"} {
		if traceGeneratedHeadingMatches(title, base) {
			return traceAuditSectionEvidence
		}
	}
	return traceAuditSectionNone
}

func traceGeneratedHeadingMatches(title, base string) bool {
	if title == base {
		return true
	}
	prefix := base + " — "
	return strings.HasPrefix(title, prefix) && strings.TrimSpace(strings.TrimPrefix(title, prefix)) != ""
}

type traceAuditSectionRenderer struct{}

func (traceAuditSectionRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindTraceAuditSection, renderTraceAuditSection)
}

func renderTraceAuditSection(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	section, ok := node.(*traceAuditSectionBlock)
	if !ok || section.className() == "" {
		return ast.WalkContinue, nil
	}
	if entering {
		_, _ = w.WriteString(`<section class="` + section.className() + `">` + "\n")
	} else {
		_, _ = w.WriteString("</section>\n")
	}
	return ast.WalkContinue, nil
}

var (
	_ parser.ASTTransformer = traceAuditSectionTransformer{}
	_ renderer.NodeRenderer = traceAuditSectionRenderer{}
)
