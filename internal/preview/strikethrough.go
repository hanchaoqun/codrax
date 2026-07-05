package preview

// Double-tilde-only strikethrough for the browser/HTML render face.
//
// goldmark's stock extension.Strikethrough (bundled inside extension.GFM)
// follows cmark-gfm and opens a strikethrough span on a SINGLE tilde run
// (parser.ScanDelimiter minimum 1). Answer prose legitimately uses a lone
// "~" as a range connector — "34579.472865s~34579.587805s", "6~11ms" — so
// two ranges in one paragraph wrapped everything between them in <del>,
// which customers saw as a large struck-through passage
// (specimen: .codrax/output/20260705-181128.544-80798.html).
//
// Ruling (2026-07-05): a single "~" must NEVER trigger strikethrough on
// this face; double "~~" keeps its standard GFM semantics. This file is
// the render-layer single point that implements the ruling: a verbatim
// port of goldmark v1.7.13 extension/strikethrough.go with the delimiter
// scan minimum raised from 1 to 2. AST node and HTML renderer are reused
// from goldmark, so <del> output for ~~text~~ is byte-identical to stock.
//
// Pinned by TestRenderMarkdownHTMLSingleTildeRangeStaysLiteral and the
// source guard TestMarkdownRendererDoesNotUseStockGFMStrikethrough; do
// not swap back to extension.GFM / extension.Strikethrough.

import (
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type doubleTildeDelimiterProcessor struct{}

func (p *doubleTildeDelimiterProcessor) IsDelimiter(b byte) bool {
	return b == '~'
}

func (p *doubleTildeDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (p *doubleTildeDelimiterProcessor) OnMatch(consumes int) gast.Node {
	return extast.NewStrikethrough()
}

var doubleTildeDelimiterProcessorInstance = &doubleTildeDelimiterProcessor{}

type doubleTildeStrikethroughParser struct{}

func (s *doubleTildeStrikethroughParser) Trigger() []byte {
	return []byte{'~'}
}

func (s *doubleTildeStrikethroughParser) Parse(parent gast.Node, block text.Reader, pc parser.Context) gast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()
	// Minimum delimiter run of 2 (stock goldmark uses 1): a lone "~"
	// range connector in prose never opens a strikethrough span.
	node := parser.ScanDelimiter(line, before, 2, doubleTildeDelimiterProcessorInstance)
	if node == nil || node.OriginalLength > 2 || before == '~' {
		return nil
	}
	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

func (s *doubleTildeStrikethroughParser) CloseBlock(parent gast.Node, pc parser.Context) {
	// nothing to do
}

type doubleTildeStrikethrough struct{}

// strikethroughDoubleTildeOnly is the drop-in replacement for
// extension.Strikethrough used by RenderMarkdownHTML. Only ~~text~~
// produces <del>; a single ~ stays literal prose.
var strikethroughDoubleTildeOnly = &doubleTildeStrikethrough{}

func (e *doubleTildeStrikethrough) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&doubleTildeStrikethroughParser{}, 500),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewStrikethroughHTMLRenderer(), 500),
	))
}
