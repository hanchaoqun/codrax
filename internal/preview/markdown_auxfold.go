package preview

// §29.9 aux-reference appendix (UXAUX batch; REVISED user ruling
// 2026-07-09 — the earlier <details> fold form was rejected mid-batch;
// this file implements the revision. No <details> may ever be emitted:
// pinned by the negative assertions in markdown_auxfold_test.go).
//
// Customer HTML reports carry auxiliary reference blocks — the tree
// legend ("树读法:") and the metric-table column glossary ("各列口径:")
// — that users skip most of the time and only occasionally consult.
// On the HTML render face ONLY, the revision does three things:
//
//  1. RELOCATE: each recognized block (marker paragraph + its entire
//     following List; nested sublists are the List's descendants and
//     travel with it) moves to a document-end 「阅读参考」 appendix
//     (<section class="aux"> with a weakened heading), placed after ALL
//     body content — including any trailing system-supplement quote
//     blocks — in first-appearance order.
//  2. POINTER: the original site keeps one renderer-emitted small muted
//     line 「（树读法与各列口径见文末「阅读参考」）」. When two moved
//     blocks were adjacent at the same site, only ONE pointer line is
//     left (no duplicates).
//  3. DEDUP: multi-projection reports repeat byte-identical legend /
//     glossary blocks. The appendix keeps ONE copy per byte-identical
//     markdown source span (first occurrence wins); later duplicates
//     still leave a pointer at their original site. Byte-different
//     variants are each kept — the dedup key is the raw source bytes
//     spanned by the block pair, so structurally different blocks can
//     never false-merge.
//
// The Markdown source and the terminal render face (internal/render)
// are byte-untouched: this is the same layer split as the §28.7 CJK
// font batch — presentation policy lives in the HTML face, never in
// the generator.
//
// Recognition is a PRECISE closed set (red line: precise signals for
// hard behavior switches; no fuzzy matching):
//   - a top-level Paragraph whose FLATTENED inline text (all literal
//     text runs concatenated; styling delimiters vanish in the
//     flattening), after TrimSpace, is EXACTLY one of the marker
//     strings below, AND
//   - whose immediate next sibling is a List node,
// is relocated. The gate is exact equality on the FLATTENED text, not
// on the paragraph's source bytes: a fully styled marker paragraph
// ("**树读法:**") flattens to the bare marker and relocates too. That
// is deliberate and lossless — the block's nodes move verbatim through
// the normal render pipeline — and the generator never emits styled
// markers, so the wrapped arm has no production witness either way.
// A marker paragraph without a following List, or a paragraph that
// merely STARTS with a marker ("树读法:xxx…", soft-broken second
// lines), flattens to something longer than the closed-set key and is
// left untouched (conservative arm: prefix forms never move). Only
// direct children of the Document are considered: moving a block out
// of a container (blockquote, list item) to the document end would
// reorder quoted content — the generator emits these blocks at top
// level only, so nested shapes stay byte-untouched (conservative).
//
// Appendix source labels: multi-projection reports relocate several
// near-identical blocks whose only telling context is the projection
// heading above them (e.g. "Trace 因果投影 — <artifact>"). Each
// relocated block therefore records the flattened text of its nearest
// PRECEDING top-level heading at its original site and the appendix
// prints it as a small "—— 来自: <heading>" line before the block.
// Blocks with no preceding heading get no label. Labels are
// presentation-only: they never participate in the dedup key.
//
// Closed-set byte forms: single source = internal/tracefence
// AuxRefMarkers (UXG-1 M1, 2026-07-12) — the same constants the generator's
// marker emitters in internal/tool consume (ASCII colon, the engine-actual
// byte form hex-verified against the customer specimen
// cust_trace_opendir_792.txt lines 70/155: …e6b395 0x3a / …e5be84 0x3a).
// The fullwidth-colon variants are DERIVED here (never a second table
// entry) because the §29.9 ruling text writes the markers with "：".
// Do NOT add other marker words without a new user ruling (闭集只这
// 两个起步).

import (
	stdhtml "html"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// auxRefMarkers is the exact closed set: the (trimmed) full paragraph
// text of an aux-reference marker. Built from the tracefence single source;
// each generator marker also admits its fullwidth-colon variant (§29.9
// ruling wording).
var auxRefMarkers = buildAuxRefMarkers()

func buildAuxRefMarkers() map[string]bool {
	out := make(map[string]bool, 2*len(tracefence.AuxRefMarkers()))
	for _, marker := range tracefence.AuxRefMarkers() {
		out[marker] = true                              // engine-actual form (ASCII colon)
		out[strings.TrimSuffix(marker, ":")+"："] = true // ruling wording (fullwidth colon)
	}
	return out
}

const (
	// auxPointerLineText is the fixed in-place pointer line (revised
	// ruling wording, verbatim — do not reword without a new ruling).
	auxPointerLineText = "（树读法与各列口径见文末「阅读参考」）"
	// auxAppendixTitle is the appendix heading (verbatim per ruling).
	auxAppendixTitle = "阅读参考"
	// auxSourceLabelPrefix prefixes each appendix source label; the
	// label body is the flattened text of the relocated block's nearest
	// preceding heading.
	auxSourceLabelPrefix = "—— 来自: "
)

var (
	// kindAuxPointer is the node kind for the in-place pointer line.
	kindAuxPointer = ast.NewNodeKind("CodraxAuxPointer")
	// kindAuxAppendix is the node kind for the document-end appendix.
	kindAuxAppendix = ast.NewNodeKind("CodraxAuxAppendix")
	// kindAuxSourceLabel is the node kind for the per-block source
	// label line inside the appendix.
	kindAuxSourceLabel = ast.NewNodeKind("CodraxAuxSourceLabel")
)

// auxPointerBlock is the childless in-place pointer line node.
type auxPointerBlock struct {
	ast.BaseBlock
}

func (n *auxPointerBlock) Kind() ast.NodeKind { return kindAuxPointer }

func (n *auxPointerBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// auxAppendixBlock is the document-end appendix container. Its children
// are the relocated marker paragraphs and lists, unchanged — content is
// preserved losslessly, exactly as the report said it.
type auxAppendixBlock struct {
	ast.BaseBlock
}

func (n *auxAppendixBlock) Kind() ast.NodeKind { return kindAuxAppendix }

func (n *auxAppendixBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// auxSourceLabelBlock is the childless per-block source label line
// inside the appendix; Label holds the flattened heading text.
type auxSourceLabelBlock struct {
	ast.BaseBlock
	Label string
}

func (n *auxSourceLabelBlock) Kind() ast.NodeKind { return kindAuxSourceLabel }

func (n *auxSourceLabelBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Label": n.Label}, nil)
}

// auxFoldTransformer relocates marker-paragraph + list pairs into the
// document-end appendix after parsing. It runs as a goldmark
// ASTTransformer, i.e. after all block/inline parsing and after the
// table paragraph transformer, so it sees the final block shapes.
type auxFoldTransformer struct{}

func (auxFoldTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	type refCandidate struct {
		para  *ast.Paragraph
		list  *ast.List
		label string
	}
	// Collect first, mutate after: reparenting nodes mid-iteration
	// would invalidate sibling traversal. The source label is captured
	// here too, while the block still sits among its original siblings
	// (heading order at the original site, not in the appendix).
	var candidates []refCandidate
	// lastHeading tracks the nearest preceding top-level heading's
	// flattened text while scanning the document in order.
	lastHeading := ""
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		if h, ok := node.(*ast.Heading); ok {
			lastHeading = strings.TrimSpace(inlinePlainText(h, source))
			continue
		}
		para, ok := node.(*ast.Paragraph)
		if !ok {
			continue
		}
		if !auxRefMarkers[strings.TrimSpace(inlinePlainText(para, source))] {
			continue
		}
		list, ok := para.NextSibling().(*ast.List)
		if !ok {
			// Conservative arm: a lone marker paragraph with no
			// immediately following list is left exactly as-is.
			continue
		}
		candidates = append(candidates, refCandidate{para: para, list: list, label: lastHeading})
		node = list // the pair is consumed; resume after the list
	}
	if len(candidates) == 0 {
		return
	}
	appendix := &auxAppendixBlock{}
	seen := make(map[string]bool, len(candidates))
	// Document order: pointers merge correctly because when two pairs
	// were adjacent, the second pair's PreviousSibling at mutation time
	// is the pointer just inserted for the first pair.
	for _, c := range candidates {
		if c.para.Parent() != doc || c.list.Parent() != doc {
			continue
		}
		if _, prevIsPointer := c.para.PreviousSibling().(*auxPointerBlock); !prevIsPointer {
			doc.InsertBefore(doc, c.para, &auxPointerBlock{})
		}
		key := auxBlockSourceKey(c.para, c.list, source)
		doc.RemoveChild(doc, c.para)
		doc.RemoveChild(doc, c.list)
		if seen[key] {
			// Byte-identical duplicate: pointer stays at its site,
			// the appendix keeps only the first copy.
			continue
		}
		seen[key] = true
		if c.label != "" {
			// Presentation-only provenance line; never part of the
			// dedup key. Blocks with no preceding heading get none.
			appendix.AppendChild(appendix, &auxSourceLabelBlock{Label: c.label})
		}
		appendix.AppendChild(appendix, c.para)
		appendix.AppendChild(appendix, c.list)
	}
	// After ALL body content, including trailing supplement blocks.
	doc.AppendChild(doc, appendix)
}

// auxBlockSourceKey returns the dedup key for a marker block: the raw
// markdown source bytes spanned by the paragraph + list pair. Byte
// equality on the source span is the precise reading of the ruling's
// "plain-text 字节相同" — identical generator output dedups, while any
// byte difference (values, nesting, markers) keeps both variants.
func auxBlockSourceKey(para *ast.Paragraph, list *ast.List, source []byte) string {
	start, stop := -1, -1
	collect := func(n ast.Node) {
		if n.Type() != ast.TypeBlock {
			return
		}
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			if start < 0 || seg.Start < start {
				start = seg.Start
			}
			if seg.Stop > stop {
				stop = seg.Stop
			}
		}
	}
	collect(para)
	_ = ast.Walk(list, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			collect(n)
		}
		return ast.WalkContinue, nil
	})
	if start >= 0 && stop > start && stop <= len(source) {
		return string(source[start:stop])
	}
	// Fallback for degenerate blocks without line segments: flattened
	// literal text (still deterministic).
	return inlinePlainText(para, source) + "\x00" + inlinePlainText(list, source)
}

// inlinePlainText concatenates the literal text content of a block's
// inline tree — the FLATTENED text: styling delimiters ("**", "`", …)
// are not part of any Text segment and vanish, so "**树读法:**"
// flattens to "树读法:". Used by the marker gate (exact equality on
// this flattening; see the file doc for the styled-marker boundary),
// the heading source labels, and the dedup-key fallback. Any extra
// literal content (soft-broken second line, trailing words) makes the
// concatenation longer than a closed-set key, so mixed paragraphs
// never match.
func inlinePlainText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			// P3-1: same bounds guard as the auxBlockSourceKey main
			// path — a corrupted / out-of-range segment is skipped,
			// never sliced (defense-in-depth parity; Segment.Value
			// would panic on out-of-range offsets).
			if t.Segment.Start < 0 || t.Segment.Stop < t.Segment.Start || t.Segment.Stop > len(source) {
				return ast.WalkContinue, nil
			}
			b.Write(t.Segment.Value(source))
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// auxRefRenderer emits the pointer line and the appendix section.
//
// Security boundary (same trust class as fencedCodeRenderer's
// <div class="mermaid">): this markup is RENDERER-EMITTED trusted
// scaffolding, never raw HTML sourced from the markdown document —
// rawHTMLLiteralRenderer still escapes every markdown-authored tag.
// The relocated children (the original paragraph + list) render through
// the standard node pipeline with its normal escaping, so tokens like
// "<anonymous>" inside a moved list stay escaped-literal and the XSS
// surface does not grow. Pointer text and appendix title are
// compile-time constants but are EscapeString'd anyway (defense in
// depth).
type auxRefRenderer struct{}

func (r auxRefRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindAuxPointer, r.renderPointer)
	reg.Register(kindAuxAppendix, r.renderAppendix)
	reg.Register(kindAuxSourceLabel, r.renderSourceLabel)
}

func (r auxRefRenderer) renderPointer(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<p class="aux">`)
		_, _ = w.WriteString(stdhtml.EscapeString(auxPointerLineText))
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r auxRefRenderer) renderSourceLabel(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*auxSourceLabelBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	if entering {
		// Label text is markdown-authored heading text: escaped
		// verbatim, same anti-XSS pipeline as everything else.
		_, _ = w.WriteString(`<p class="aux-src">`)
		_, _ = w.WriteString(stdhtml.EscapeString(auxSourceLabelPrefix + n.Label))
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func (r auxRefRenderer) renderAppendix(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<section class="aux">` + "\n<h2>")
		_, _ = w.WriteString(stdhtml.EscapeString(auxAppendixTitle))
		_, _ = w.WriteString("</h2>\n")
	} else {
		_, _ = w.WriteString("</section>\n")
	}
	return ast.WalkContinue, nil
}

// Interface guards.
var (
	_ parser.ASTTransformer = auxFoldTransformer{}
	_ renderer.NodeRenderer = auxRefRenderer{}
)
