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
// no keyword/fuzzy inference drives the structural switch. The wrapper ends at
// the next top-level H1/H2, preserving every original child node and its order.

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
		for node := ast.Node(heading); node != nil; {
			next := node.NextSibling()
			if node != heading {
				if nextHeading, ok := node.(*ast.Heading); ok && nextHeading.Level <= 2 {
					break
				}
			}
			doc.RemoveChild(doc, node)
			section.AppendChild(section, node)
			node = next
		}
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
