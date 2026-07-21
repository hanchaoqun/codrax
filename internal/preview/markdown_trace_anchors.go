package preview

// markdown_trace_anchors.go — v5 P0 档1 E# in-page anchor links (design
// causal_tree_v5_design_20260711.md §C.3 T-5, user ruling 2026-07-11).
//
// The projection tree's [E#] locator tokens become <a href="#trace-e7"> links
// jumping to the paired lossless surface: the 因果投影明细 stanza heading
// (**[E7] name**) when present, else the 证据索引 roster entry (**E7** — …).
// Pure-attribute decoration: fence textContent and every byte of the detail /
// evidence sections are unchanged — only goldmark node ATTRIBUTES (id on the
// target paragraph / list item, an internal pairing attribute on the fence
// node) are added.
//
// Pairing is ordinal: the k-th projection fence ↔ the k-th detail section ↔
// the k-th evidence section, in document order (the CMP multi-artifact layout
// reuses the single-artifact section builder per artifact, preserving each
// family's relative order). A single-fence document uses the bare "trace-"
// id prefix; a multi-fence document uses "trace-g<k>-". If any populated
// family's count disagrees with the fence count, the document gets NO anchor
// decoration at all — this is a soft decoration lane and a wrong link is
// worse than none (precise signals for hard gates; ambiguous pairing bails
// out whole, it never guesses).

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// traceAnchorPrefixAttr is the internal fence-node attribute carrying the id
// prefix the grid writer uses for [E#] links. It is consumed by
// fencedCodeRenderer only — never rendered as markup (our renderer ignores
// node attributes on fences).
const traceAnchorPrefixAttr = "codraxTraceAnchorPrefix"

// traceAnchorPairing is the per-fence link contract (F5, fix round
// 2026-07-11): the writer mints an <a> ONLY for ordinals that actually
// CLAIMED an id target in this fence's paired sections — an ordinal without
// a target (e.g. the E9 of a merged detail heading "[E7] [E9] name" in a
// document with no evidence roster) stays a plain pinned run instead of a
// dangling link.
type traceAnchorPairing struct {
	prefix  string
	claimed map[int]bool
}

type traceEvidenceAnchorTransformer struct{}

func (traceEvidenceAnchorTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	if doc == nil || reader == nil {
		return
	}
	source := reader.Source()
	var fences []*ast.FencedCodeBlock
	// ELIM-1 (RANK-U Stage 2): ◎ overview fences are EXCLUDED from the
	// fence↔section pairing census (a second counted fence would break the
	// count identity and kill every anchor in the document). Each overview
	// borrows the pairing of its HOST projection fence — the one it PRECEDES
	// (user ruling 2026-07-13: the overview renders before its tree, so its
	// [E#] pointers are forward references into the same section's
	// detail/evidence targets); an overview after the last tree (archive
	// forms) falls back to the preceding one.
	elimSeen := map[*ast.FencedCodeBlock]int{}
	var details, evidences []*traceAuditSectionBlock
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.FencedCodeBlock:
			info := ""
			if n.Info != nil {
				info = string(n.Info.Segment.Value(source))
			}
			if isTraceElimOverviewFence(info) {
				elimSeen[n] = len(fences) // count of projection fences before it
				continue
			}
			if isTraceCausalProjectionFence(info, fencedCodeBody(n, source)) {
				fences = append(fences, n)
			}
		case *traceAuditSectionBlock:
			switch n.Class {
			case traceAuditSectionDetail:
				details = append(details, n)
			case traceAuditSectionEvidence:
				evidences = append(evidences, n)
			}
		}
	}
	if len(fences) == 0 {
		return
	}
	// UX-ANCHOR 件a (§29.61.7, 2026-07-14): the count-identity bail no longer
	// returns before the lead walk — pairings stays nil-filled instead, so the
	// LINK lanes (fence writer + lead decorator) degrade to plain text whole
	// (fail-closed, a wrong link is worse than none) while the lead's compact
	// ➊..➎ badge styling (件b — presentation only, no target to dangle) keeps
	// working off the fence-anchored lead scope.
	pairings := make([]*traceAnchorPairing, len(fences))
	paired := (len(details) > 0 || len(evidences) > 0) &&
		(len(details) == 0 || len(details) == len(fences)) &&
		(len(evidences) == 0 || len(evidences) == len(fences))
	if paired {
		for k, fence := range fences {
			prefix := "trace-"
			if len(fences) > 1 {
				prefix = fmt.Sprintf("trace-g%d-", k+1)
			}
			// Claim targets FIRST, then stamp the fence with the claimed set —
			// the writer links exactly the ordinals that own an id (F5).
			seen := map[int]bool{}
			if len(details) > 0 {
				traceAnchorMarkTargets(details[k], source, prefix, seen)
			}
			if len(evidences) > 0 {
				traceAnchorMarkTargets(evidences[k], source, prefix, seen)
			}
			pairings[k] = &traceAnchorPairing{prefix: prefix, claimed: seen}
			fence.SetAttributeString(traceAnchorPrefixAttr, pairings[k])
		}
		// ◎ overview fences link through their host tree fence's pairing (same
		// claimed set — never a fresh id namespace, never a dangling link). Host
		// = the FOLLOWING projection fence (the overview leads its section, user
		// ruling 2026-07-13); a trailing overview keeps the preceding fence.
		for fence, before := range elimSeen {
			host := before
			if host >= len(pairings) {
				host = len(pairings) - 1
			}
			if host >= 0 && host < len(pairings) && pairings[host] != nil {
				fence.SetAttributeString(traceAnchorPrefixAttr, pairings[host])
			}
		}
	}
	// UX-ANCHOR 件a/件b (§29.61.7): decorate each projection section's LEAD
	// segment (the generator prose between the section's H2 heading and its
	// tree fence) — E# refs become in-page links on the SAME claimed pairing,
	// ➊..➎ glyphs wear the compact body badge. See markdown_trace_lead.go.
	decorateTraceProjectionLeadSegments(doc, source, fences, elimSeen, pairings)
}

// traceAnchorMarkTargets assigns id attributes inside one detail/evidence
// section: detail stanza heading paragraphs (flattened text "[E7] name" or
// the merged "[E7] [E9] name" — the first UNSEEN ordinal takes the id, the
// remaining ordinals resolve on the evidence roster) and evidence roster
// list items (flattened text "E7 — 定位: …"). First occurrence wins; ids are
// never duplicated.
func traceAnchorMarkTargets(section *traceAuditSectionBlock, source []byte, prefix string, seen map[int]bool) {
	if section == nil {
		return
	}
	_ = ast.Walk(section, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.(type) {
		case *ast.Paragraph, *ast.ListItem:
		default:
			return ast.WalkContinue, nil
		}
		ordinal, ok := traceAnchorTargetOrdinal(strings.TrimSpace(inlinePlainText(node, source)), seen)
		if !ok {
			return ast.WalkContinue, nil
		}
		seen[ordinal] = true
		node.SetAttributeString("id", []byte(fmt.Sprintf("%se%d", prefix, ordinal)))
		if _, isItem := node.(*ast.ListItem); isItem {
			// The list item's own paragraph/text-block would be re-visited and
			// could double-claim; the item is the anchor, skip its subtree.
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkSkipChildren, nil
	})
}

// traceAnchorTargetOrdinal extracts the anchor ordinal from a flattened
// block text: the detail heading form "[E7]…" (first unseen ordinal among
// its leading [E#] tags) or the evidence roster form "E7 — …" / "E7(+2) — …".
func traceAnchorTargetOrdinal(text string, seen map[int]bool) (int, bool) {
	if strings.HasPrefix(text, "[E") {
		pos := 0
		for pos < len(text) {
			token, ordinal, ok := traceProjectionEvidenceRefToken(text, pos)
			if !ok {
				return 0, false
			}
			if !seen[ordinal] {
				return ordinal, true
			}
			pos += len(token)
			for pos < len(text) && text[pos] == ' ' {
				pos++
			}
			if pos >= len(text) || text[pos] != '[' {
				return 0, false
			}
		}
		return 0, false
	}
	if !strings.HasPrefix(text, "E") {
		return 0, false
	}
	i := 1
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	// The roster label ends here: either the whole text (bare label) or a
	// space / merge-group boundary ("E7 — …", "E7(+2) — …").
	if i < len(text) && text[i] != ' ' && text[i] != '(' {
		return 0, false
	}
	ordinal := 0
	for _, c := range text[start:i] {
		ordinal = ordinal*10 + int(c-'0')
	}
	if seen[ordinal] {
		return 0, false
	}
	return ordinal, true
}

// traceProjectionAnchorPairing reads the pairing the transformer stamped on
// a projection fence node (nil when the document-level pairing bailed out —
// the grid writer then renders plain [E#] runs, no links).
func traceProjectionAnchorPairing(node ast.Node) *traceAnchorPairing {
	value, ok := node.AttributeString(traceAnchorPrefixAttr)
	if !ok {
		return nil
	}
	pairing, ok := value.(*traceAnchorPairing)
	if !ok {
		return nil
	}
	return pairing
}

var _ parser.ASTTransformer = traceEvidenceAnchorTransformer{}
