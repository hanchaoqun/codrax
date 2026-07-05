package render

// Terminal markdown pipeline (RFH #66).
//
// glamour v1.0.0's NewTermRenderer hardcodes extension.GFM into an
// UNEXPORTED goldmark instance, with no option to swap extensions or
// parsers. Two customer-visible defects live at that parse layer, so
// styles/options cannot fix them:
//
//  1. Stock GFM strikethrough opens on a SINGLE tilde: prose range
//     connectors ("34579.472865s~34579.587805s", "6~11ms") made
//     everything between two ranges render struck-through (SGR 9),
//     and the "~" bytes themselves were consumed. Ruling 2026-07-05
//     (shared with the HTML face, see internal/markdownext): a single
//     "~" never strikes; "~~" keeps GFM semantics.
//  2. Raw-HTML-shaped prose tokens — stack frames ("<anonymous>"),
//     generics ("Vec<int>") — parse as RawHTML/HTMLBlock nodes which
//     glamour's ANSI renderer feeds through bluemonday.StrictPolicy,
//     silently deleting them. Ruling (same as the HTML face): display
//     literally, never drop. On the terminal face the cleanest zero-
//     loss form is to not parse raw HTML at all: the bytes stay Text
//     nodes and render verbatim (a terminal has no execution surface).
//
// newTerminalMarkdown therefore rebuilds glamour's five-line goldmark
// construction 1:1 — same extensions in the same order (with our
// double-tilde strikethrough in the stock slot), same AutoHeadingID
// parser option, same ansi.NewRenderer at glamour's highPriority=1000
// — minus goldmark's RawHTML inline parser and HTMLBlock block parser.
// Parity with glamour is MECHANICALLY pinned:
//   - TestTerminalMarkdownByteParityWithGlamour renders a corpus
//     covering every element family through both pipelines and
//     requires byte-identical output;
//   - TestTerminalParserListsTrackGoldmarkDefaults requires the local
//     parser lists to be exactly goldmark's defaults minus the two
//     HTML parsers, so a goldmark upgrade that changes the defaults
//     fails loudly instead of drifting.
// Behavior pins: markdown_terminal_test.go (single ~ literal + no
// SGR 9; ~~ still struck; <anonymous>/Vec<int> survive).

import (
	"bytes"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/hanchaoqun/codrax/internal/markdownext"
)

// glamourANSIPriority mirrors glamour v1.0.0's unexported highPriority
// constant used when registering its ANSI node renderer.
const glamourANSIPriority = 1000

// terminalMarkdown is the terminal-face replacement for
// *glamour.TermRenderer. Renderer.RenderMarkdown treats it exactly as
// it treated glamour: render on success, fall back to raw text on
// error.
type terminalMarkdown struct {
	md goldmark.Markdown
}

// newTerminalMarkdown mirrors glamour.NewTermRenderer(
// WithStyles(styles), WithWordWrap(0)) — see the file header for why
// glamour cannot be used directly and which two deviations are made.
func newTerminalMarkdown(styles ansi.StyleConfig) *terminalMarkdown {
	md := goldmark.New(
		// WithParser must precede WithParserOptions: goldmark applies
		// options in order, and WithParserOptions mutates the parser
		// installed at that point.
		goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(terminalBlockParsers()...),
			parser.WithInlineParsers(terminalInlineParsers()...),
			parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		)),
		// glamour extension order: GFM (= Linkify, Table,
		// Strikethrough, TaskList), then DefinitionList. Our
		// double-tilde extension sits in the stock Strikethrough
		// slot. Do not collapse back to extension.GFM — its bundled
		// Strikethrough opens on a single tilde.
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Table,
			markdownext.StrikethroughDoubleTildeOnly,
			extension.TaskList,
			extension.DefinitionList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	ar := ansi.NewRenderer(ansi.Options{
		// Word wrap disabled — glamour counts runes, not display
		// columns, so CJK text overflows; the REPL does its own
		// ANSI-aware, display-width wrapping after rendering.
		WordWrap:     0,
		ColorProfile: termenv.TrueColor,
		Styles:       styles,
	})
	md.SetRenderer(
		renderer.NewRenderer(
			renderer.WithNodeRenderers(
				util.Prioritized(ar, glamourANSIPriority),
			),
		),
	)
	return &terminalMarkdown{md: md}
}

// Render mirrors glamour.TermRenderer.Render/RenderBytes.
func (t *terminalMarkdown) Render(in string) (string, error) {
	if t == nil || t.md == nil {
		return in, nil
	}
	var buf bytes.Buffer
	if err := t.md.Convert([]byte(in), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// terminalBlockParsers is goldmark v1.7.13 parser.DefaultBlockParsers()
// verbatim MINUS NewHTMLBlockParser (priority 900): an HTML-shaped
// block stays a paragraph whose bytes render literally instead of
// being stripped by glamour's bluemonday pass.
// TestTerminalParserListsTrackGoldmarkDefaults pins the "defaults
// minus exactly {900}" relationship against goldmark upgrades.
func terminalBlockParsers() []util.PrioritizedValue {
	return []util.PrioritizedValue{
		util.Prioritized(parser.NewSetextHeadingParser(), 100),
		util.Prioritized(parser.NewThematicBreakParser(), 200),
		util.Prioritized(parser.NewListParser(), 300),
		util.Prioritized(parser.NewListItemParser(), 400),
		util.Prioritized(parser.NewCodeBlockParser(), 500),
		util.Prioritized(parser.NewATXHeadingParser(), 600),
		util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
		util.Prioritized(parser.NewBlockquoteParser(), 800),
		util.Prioritized(parser.NewParagraphParser(), 1000),
	}
}

// terminalInlineParsers is goldmark v1.7.13 parser.DefaultInlineParsers()
// verbatim MINUS NewRawHTMLParser (priority 400): "<anonymous>" /
// "Vec<int>" stay Text nodes and render verbatim. AutoLink (300) still
// runs first, so <https://…> / <user@host> autolinks are unaffected.
// TestTerminalParserListsTrackGoldmarkDefaults pins the "defaults
// minus exactly {400}" relationship against goldmark upgrades.
func terminalInlineParsers() []util.PrioritizedValue {
	return []util.PrioritizedValue{
		util.Prioritized(parser.NewCodeSpanParser(), 100),
		util.Prioritized(parser.NewLinkParser(), 200),
		util.Prioritized(parser.NewAutoLinkParser(), 300),
		util.Prioritized(parser.NewEmphasisParser(), 500),
	}
}
