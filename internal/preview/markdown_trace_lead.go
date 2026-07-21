package preview

// markdown_trace_lead.go — UX-ANCHOR 件a/件b/件c (§29.61.7 customer feedback,
// 2026-07-14): the projection section's LEAD segment (conclusion line /
// analysis window / four-state account / running decomposition / coverage
// sentences — the generator prose between the section's H2 heading and its
// projection fence) carries the same E# evidence references the fence rows
// carry ([E28], the merged form [E4(+1)] and the ➋[E28] badge+ref pair), but
// the fence-side anchor machinery stopped at the fence bytes. This decorator
// extends it to the lead prose, HTML face only (markdown/terminal bytes are
// untouched):
//
//   件a — [E#] / [E#(+N)] / [E#(+N)+E#] tokens and the generator's
//        parenthesized bare form "(E4)" / "(E3(+2))" become in-page anchor
//        links on the SAME per-fence pairing the fence writer consumes —
//        claimed ordinals only (F5), whole-document bail on any count-identity
//        break (a wrong link is worse than none; unclaimed refs stay plain
//        text, never a dangling href). The 「见 ➌[E#](折算,…)」 pointer, which
//        markdown accidentally parses as an inline LINK (bogus relative href +
//        the caliber note swallowed invisible), is repaired on the same lane
//        (traceLeadRepairAccidentalRefLink).
//   件b — ➊..➎ badge glyphs in lead prose wear a COMPACT body badge
//        (trace-lead-badge + the shared per-rank color pair): smaller,
//        unbolded, light-background — visually subordinate to the body line
//        height. The tree fence's 2ch envelope pill is untouched.
//   件c — the lead anchor links wear a.trace-eref-lead (link color + dotted
//        underline + hover) so the refs read as navigable.
//
// Lead-scope resolution is PRECISE (hard gates read precise signals): for
// each projection fence, walk its preceding document-level siblings backward
// collecting paragraphs/lists (skipping ◎ overview fences and the §29.9 aux
// relocation artifacts, which live inside the lead cluster) until a heading;
// decoration happens ONLY when that boundary is an H2 spelling the
// tracefence.SectionProjectionTitles closed set (single source with the
// tool-side title emitter — optional " — <artifact>" suffix per the
// generated-heading grammar). Any other boundary → the fence has no
// decoratable lead (fail-closed, nothing is guessed).
//
// textContent discipline: decoration wraps the VERBATIM token bytes in inline
// nodes — the rendered text content of every lead block is unchanged.

import (
	"fmt"
	stdhtml "html"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// kindTraceLeadRef is the inline node kind carrying one decorated lead token
// (an E# anchor link or a ➊..➎ compact badge). The node stores the verbatim
// token text; the renderer only wraps it (textContent == markdown bytes).
var kindTraceLeadRef = ast.NewNodeKind("CodraxTraceLeadRef")

type traceLeadRefNode struct {
	ast.BaseInline
	// Value is the verbatim token text ("[E7(+1)]", "E3(+2)", "➋").
	Value string
	// Href is the in-page anchor target ("#trace-e7"); set only for claimed
	// ordinals (F5) — badge nodes leave it empty.
	Href string
	// Rank is the ➊..➎ seat (1-based) for badge nodes; 0 for anchor links.
	Rank int
}

func (n *traceLeadRefNode) Kind() ast.NodeKind { return kindTraceLeadRef }

func (n *traceLeadRefNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Value": n.Value, "Href": n.Href, "Rank": strconv.Itoa(n.Rank),
	}, nil)
}

type traceLeadRefRenderer struct{}

func (traceLeadRefRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindTraceLeadRef, renderTraceLeadRef)
}

func renderTraceLeadRef(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := node.(*traceLeadRefNode)
	if !ok {
		return ast.WalkContinue, nil
	}
	switch {
	case n.Rank > 0:
		// 件b compact body badge — same per-rank color pair as the fence pill
		// (trace-rank-N vars), own compact geometry class (server.go CSS).
		_, _ = fmt.Fprintf(w, `<span class="trace-lead-badge trace-rank-%d">%s</span>`,
			n.Rank, stdhtml.EscapeString(n.Value))
	case n.Href != "":
		// Href is renderer-authored ("#trace-e7" / "#trace-g2-e7") — never
		// sourced from document bytes.
		_, _ = fmt.Fprintf(w, `<a class="trace-eref-lead" href="%s">%s</a>`,
			n.Href, stdhtml.EscapeString(n.Value))
	default:
		_, _ = w.WriteString(stdhtml.EscapeString(n.Value))
	}
	return ast.WalkSkipChildren, nil
}

var _ renderer.NodeRenderer = traceLeadRefRenderer{}

// decorateTraceProjectionLeadSegments runs after the fence pairing pass: for
// each projection fence it resolves the lead scope and decorates the lead
// blocks. pairings[k] == nil (count-identity bail / no paired sections) keeps
// every E# ref plain — only the badge styling proceeds.
func decorateTraceProjectionLeadSegments(doc *ast.Document, source []byte, fences []*ast.FencedCodeBlock, elim map[*ast.FencedCodeBlock]int, pairings []*traceAnchorPairing) {
	for k, fence := range fences {
		blocks, ok := traceProjectionLeadBlocks(source, fence, elim)
		if !ok {
			continue
		}
		for _, block := range blocks {
			decorateTraceLeadBlock(block, source, pairings[k])
		}
	}
}

// traceProjectionLeadBlocks walks backward from a projection fence over its
// document-level siblings, collecting the lead paragraphs/lists. ok only when
// the walk terminates at the projection section's own H2 heading (the
// tracefence closed set, optional " — <artifact>" suffix) — any other
// boundary (another fence, a wrapped audit section, an unknown block kind,
// a foreign heading, document start) yields no decoratable lead.
func traceProjectionLeadBlocks(source []byte, fence *ast.FencedCodeBlock, elim map[*ast.FencedCodeBlock]int) ([]ast.Node, bool) {
	var blocks []ast.Node
	for node := fence.PreviousSibling(); node != nil; node = node.PreviousSibling() {
		switch n := node.(type) {
		case *ast.FencedCodeBlock:
			if _, isElim := elim[n]; isElim {
				continue // the ◎ overview is part of the lead cluster
			}
			return nil, false
		case *ast.Heading:
			if n.Level != 2 {
				return nil, false
			}
			title := strings.TrimSpace(inlinePlainText(n, source))
			for _, base := range tracefence.SectionProjectionTitles() {
				if traceGeneratedHeadingMatches(title, base) {
					return blocks, true
				}
			}
			return nil, false
		case *ast.Paragraph, *ast.List:
			blocks = append(blocks, node)
		default:
			switch node.Kind() {
			case kindAuxPointer, kindAuxAppendix, kindAuxSourceLabel:
				continue // §29.9 relocation artifacts inside the lead cluster
			}
			return nil, false
		}
	}
	return nil, false
}

// decorateTraceLeadBlock decorates one lead paragraph/list: consecutive text
// nodes outside code spans / existing links are scanned as ONE contiguous run
// for badge glyphs and E# reference tokens. Run-level scanning matters:
// goldmark's link parser leaves an unmatched "[" as its OWN text node, so a
// per-node scan would never see the "[E7(+1)]" token whole.
func decorateTraceLeadBlock(block ast.Node, source []byte, pairing *traceAnchorPairing) {
	decorateTraceLeadContainer(block, source, pairing)
}

func decorateTraceLeadContainer(node ast.Node, source []byte, pairing *traceAnchorPairing) {
	for child := node.FirstChild(); child != nil; {
		if head, ok := child.(*ast.Text); ok {
			run := []*ast.Text{head}
			next := head.NextSibling()
			for next != nil {
				tail, ok := next.(*ast.Text)
				// A run extends over source-CONTIGUOUS sibling text nodes
				// only, and never across a rendered line break — tokens do
				// not span lines.
				if !ok || tail.Segment.Start != run[len(run)-1].Segment.Stop ||
					run[len(run)-1].SoftLineBreak() || run[len(run)-1].HardLineBreak() {
					break
				}
				run = append(run, tail)
				next = tail.NextSibling()
			}
			decorateTraceLeadRun(node, run, source, pairing)
			child = next
			continue
		}
		if link, ok := child.(*ast.Link); ok {
			next := child.NextSibling()
			traceLeadRepairAccidentalRefLink(node, link, source, pairing)
			child = next
			continue
		}
		switch child.Kind() {
		case ast.KindCodeSpan, ast.KindAutoLink, ast.KindImage:
			// Backtick legend quotes (`➊..➎`) are teaching text — verbatim.
		default:
			decorateTraceLeadContainer(child, source, pairing)
		}
		child = child.NextSibling()
	}
}

// traceLeadRepairAccidentalRefLink handles the generator lead shape
// 「见 ➌[E1](折算,不计入四态合计)」: markdown parses the [E#] ref plus its
// trailing parenthetical as an inline LINK, so the HTML face rendered a bogus
// relative href AND swallowed the caliber note into it (invisible text). In
// the lead scope this shape is deterministically the generator's plain prose
// — rebuild it as a decorated E# ref (anchor-linked when the pairing claimed
// the ordinal, plain text otherwise — never a dangling href) followed by the
// verbatim parenthetical text. Real links (URL / fragment destinations, or a
// label off the E# grammar) stay untouched.
func traceLeadRepairAccidentalRefLink(parent ast.Node, link *ast.Link, source []byte, pairing *traceAnchorPairing) {
	if parent == nil || len(link.Title) > 0 {
		return
	}
	dest := string(link.Destination)
	if dest == "" || strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") {
		return
	}
	label := strings.TrimSpace(inlinePlainText(link, source))
	token := "[" + label + "]"
	consumed, ordinal, ok := traceProjectionEvidenceRefToken(token, 0)
	if !ok || consumed != token {
		return
	}
	ref := &traceLeadRefNode{Value: token}
	if pairing != nil && pairing.claimed[ordinal] {
		ref.Href = fmt.Sprintf("#%se%d", pairing.prefix, ordinal)
	}
	parent.InsertBefore(parent, link, ref)
	// The parenthetical caliber note returns to visible text, verbatim.
	parent.InsertBefore(parent, link, &traceLeadRefNode{Value: "(" + dest + ")"})
	parent.RemoveChild(parent, link)
}

// traceLeadToken is one decoration within a text node's value: [start,end)
// byte offsets, plus either a badge rank or an anchor ordinal.
type traceLeadToken struct {
	start, end int
	rank       int // >0: ➊..➎ badge
	ordinal    int // >0: claimed E# anchor
}

// decorateTraceLeadRun scans one contiguous text-node run and, when tokens
// are found, replaces the whole run with plain text segments + decoration
// nodes. The run covers source[start:stop] contiguously, so replacement
// segments may freely span the original node boundaries; the LAST node's
// line-break flags survive on the trailing piece.
func decorateTraceLeadRun(parent ast.Node, run []*ast.Text, source []byte, pairing *traceAnchorPairing) {
	if len(run) == 0 || parent == nil {
		return
	}
	start := run[0].Segment.Start
	stop := run[len(run)-1].Segment.Stop
	value := string(source[start:stop])
	tokens := traceLeadScanTokens(value, pairing)
	if len(tokens) == 0 {
		return
	}
	last := run[len(run)-1]
	var nodes []ast.Node
	prev := 0
	for _, tk := range tokens {
		if tk.start > prev {
			nodes = append(nodes, ast.NewTextSegment(text.NewSegment(start+prev, start+tk.start)))
		}
		ref := &traceLeadRefNode{Value: value[tk.start:tk.end], Rank: tk.rank}
		if tk.ordinal > 0 && pairing != nil {
			ref.Href = fmt.Sprintf("#%se%d", pairing.prefix, tk.ordinal)
		}
		nodes = append(nodes, ref)
		prev = tk.end
	}
	var tail *ast.Text
	if prev < len(value) {
		tail = ast.NewTextSegment(text.NewSegment(start+prev, stop))
	} else if last.SoftLineBreak() || last.HardLineBreak() {
		// The break flag lives on the (now fully consumed) run tail — keep it
		// on an empty trailing segment so line breaks survive the split.
		tail = ast.NewTextSegment(text.NewSegment(stop, stop))
	}
	if tail != nil {
		tail.SetSoftLineBreak(last.SoftLineBreak())
		tail.SetHardLineBreak(last.HardLineBreak())
		nodes = append(nodes, tail)
	}
	for _, n := range nodes {
		parent.InsertBefore(parent, run[0], n)
	}
	for _, n := range run {
		parent.RemoveChild(parent, n)
	}
}

// traceLeadScanTokens scans one text value for lead decorations, in one
// left-to-right pass: ➊..➎ badges always decorate; E# reference tokens
// (bracketed grammar shared with the fence writer, plus the generator's
// parenthesized bare form) decorate ONLY when the pairing claimed the
// ordinal — unclaimed or unpaired refs are consumed without decoration so a
// compound token can never half-link.
func traceLeadScanTokens(value string, pairing *traceAnchorPairing) []traceLeadToken {
	var tokens []traceLeadToken
	claimed := func(ordinal int) bool {
		return pairing != nil && pairing.claimed[ordinal]
	}
	for offset := 0; offset < len(value); {
		badge := false
		for i, glyph := range tracefence.BadgeGlyphs() {
			if strings.HasPrefix(value[offset:], glyph) {
				tokens = append(tokens, traceLeadToken{start: offset, end: offset + len(glyph), rank: i + 1})
				offset += len(glyph)
				badge = true
				break
			}
		}
		if badge {
			continue
		}
		if token, first, ok := traceProjectionEvidenceRefToken(value, offset); ok {
			if claimed(first) {
				tokens = append(tokens, traceLeadToken{start: offset, end: offset + len(token), ordinal: first})
			}
			offset += len(token)
			continue
		}
		if token, first, ok := traceLeadBareEvidenceToken(value, offset); ok {
			if claimed(first) {
				tokens = append(tokens, traceLeadToken{start: offset, end: offset + len(token), ordinal: first})
			}
			offset += len(token)
			continue
		}
		offset++
	}
	return tokens
}

// traceLeadBareEvidenceToken recognizes the PARENTHESIZED bare evidence tag
// — "(E4)" / "(E3(+2))". 勘正 (DISPLAY-HYG 二轮 复核件4②, 2026-07-17): the
// former minting site (runtimeTraceProjResidualOwnCaliberNote's
// "(" + tag + ")") switched to the document-wide bracket style "[E3]" in the
// catalog B12 unification, which the main [E#] token lane recognizes — this
// recognizer now serves ARCHIVED pre-B12 reports only (legacy-fallback
// posture, same as the tracefence content-sniffing arm; behavior kept
// byte-identical so old archives keep their anchors). The grammar stays
// exact: the previous byte must be '(' and the byte after the tag must be
// ')' (the wrapping paren closes immediately), so prose words, thread names
// ("Thread-E5") and pid parentheticals ("E2(1234)") never match. Returns the
// tag (parens excluded) and its ordinal.
func traceLeadBareEvidenceToken(value string, offset int) (string, int, bool) {
	if offset == 0 || value[offset-1] != '(' {
		return "", 0, false
	}
	rest := value[offset:]
	if len(rest) < 2 || rest[0] != 'E' {
		return "", 0, false
	}
	i := 1
	start := i
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == start {
		return "", 0, false
	}
	ordinal, err := strconv.Atoi(rest[start:i])
	if err != nil {
		return "", 0, false
	}
	if strings.HasPrefix(rest[i:], "(+") { // optional merge group "(+N)"
		j := i + 2
		digits := j
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j == digits || j >= len(rest) || rest[j] != ')' {
			return "", 0, false
		}
		i = j + 1
	}
	if i >= len(rest) || rest[i] != ')' {
		return "", 0, false
	}
	return rest[:i], ordinal, true
}
