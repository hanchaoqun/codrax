package orchestrator

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	finalizerAutoRepairFacetRE       = regexp.MustCompile(`required facet "([^"]+)"`)
	finalizerAutoRepairInlineIdentRE = regexp.MustCompile(`ident="([^"]+)"`)
)

// tryAutoRepairFinalizerAnswerDocument applies deterministic repairs for
// post-finalizer violations that do not require another LLM pass:
// missing non-visible facet_ids metadata and inline-code backticks around
// identifiers that the validator already says should be ordinary prose.
func (o *Orchestrator) tryAutoRepairFinalizerAnswerDocument(out *agent.StageOutput, violations []types.Violation) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || out == nil || len(violations) == 0 {
		return false
	}
	doc := o.busCtx.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		return false
	}
	facets := finalizerAutoRepairFacetIDs(violations)
	inlineIdents := finalizerAutoRepairInlineIdents(violations)
	changed := false
	if len(facets) > 0 {
		changed = addFacetIDsToPrincipalAnswerBlock(doc, facets) || changed
	}
	if len(inlineIdents) > 0 {
		changed = stripUnsupportedInlineIdentifierBackticks(doc, inlineIdents) || changed
	}
	if !changed {
		return false
	}
	o.busCtx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	renderDoc := doc
	render.ApplyAuthorityHedging(renderDoc, finalizerAutoRepairAuthorityEvidencePool(o.busCtx), o.busCtx.Language)
	out.FinalAnswer = render.RenderAnswerDocumentWithAttachments(
		renderDoc,
		o.busCtx.Mutable.AnswerDisplayAttachments(),
		o.busCtx.Language,
	)
	out.Data = marshalFinalizerAutoRepairStageData(out.FinalAnswer)
	return true
}

func finalizerAutoRepairFacetIDs(violations []types.Violation) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range violations {
		if v.Kind != types.ViolFacetUncovered {
			continue
		}
		if !strings.Contains(v.Detail, "evidence-sufficient") {
			continue
		}
		m := finalizerAutoRepairFacetRE.FindStringSubmatch(v.Detail)
		if len(m) < 2 {
			continue
		}
		facet := strings.TrimSpace(m[1])
		if facet == "" || seen[facet] || !isFinalizerAutoRepairableFacet(facet) {
			continue
		}
		seen[facet] = true
		out = append(out, facet)
	}
	sort.Strings(out)
	return out
}

func isFinalizerAutoRepairableFacet(facet string) bool {
	switch types.AnswerFacetKind(facet) {
	case types.FacetCurrentCodePath, types.FacetResolvedLiteralOrSymbol:
		// These facets are carrier metadata over already-rendered code
		// paths / symbols. If the validator says typed evidence is already
		// sufficient, adding the missing facet_id does not invent a new
		// user-visible claim or change the requested answer shape.
		return true
	default:
		// Shape-bearing facets such as enumeration_item, bucket_label,
		// diagram_spine, path edges, and guards must stay with the normal
		// validator/retry path; auto-declaring them could hide a genuinely
		// missing section, table, list, or diagram.
		return false
	}
}

func finalizerAutoRepairInlineIdents(violations []types.Violation) map[string]bool {
	out := map[string]bool{}
	for _, v := range violations {
		if v.Kind != types.ViolInlineIdentifierHallucinated {
			continue
		}
		for _, m := range finalizerAutoRepairInlineIdentRE.FindAllStringSubmatch(v.Detail, -1) {
			if len(m) < 2 {
				continue
			}
			ident := strings.TrimSpace(m[1])
			if ident != "" {
				out[ident] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addFacetIDsToPrincipalAnswerBlock(doc *types.AnswerDocumentV2, facets []string) bool {
	if doc == nil || len(facets) == 0 {
		return false
	}
	var target *types.AnswerBlock
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		if blk.Kind == types.BlockCaveat {
			continue
		}
		if blk.SurfaceRole == types.SurfacePrincipal {
			target = blk
			break
		}
	}
	if target == nil {
		for i := range doc.Blocks {
			if doc.Blocks[i].Kind != types.BlockCaveat {
				target = &doc.Blocks[i]
				break
			}
		}
	}
	if target == nil {
		return false
	}
	changed := false
	for _, facet := range facets {
		if answerDocumentHasFacet(doc, facet) {
			continue
		}
		target.FacetIDs = append(target.FacetIDs, facet)
		changed = true
	}
	if changed {
		target.FacetIDs = dedupeSortedStrings(target.FacetIDs)
	}
	return changed
}

func answerDocumentHasFacet(doc *types.AnswerDocumentV2, facet string) bool {
	if doc == nil || facet == "" {
		return false
	}
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		for _, f := range blk.FacetIDs {
			if f == facet {
				return true
			}
		}
		for _, cu := range blk.ClaimUses {
			if cu.FacetID == facet {
				return true
			}
		}
	}
	return false
}

func stripUnsupportedInlineIdentifierBackticks(doc *types.AnswerDocumentV2, idents map[string]bool) bool {
	if doc == nil || len(idents) == 0 {
		return false
	}
	changed := false
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		if out, ok := stripUnsupportedInlineIdentifierBackticksFromText(blk.Title, idents); ok {
			blk.Title = out
			changed = true
		}
		if out, ok := stripUnsupportedInlineIdentifierBackticksFromText(blk.Text, idents); ok {
			blk.Text = out
			changed = true
		}
		for j := range blk.Items {
			if out, ok := stripUnsupportedInlineIdentifierBackticksFromText(blk.Items[j].Text, idents); ok {
				blk.Items[j].Text = out
				changed = true
			}
		}
	}
	return changed
}

func stripUnsupportedInlineIdentifierBackticksFromText(text string, idents map[string]bool) (string, bool) {
	if text == "" || len(idents) == 0 {
		return text, false
	}
	matches := inlineBacktickIdentRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, false
	}
	var b strings.Builder
	last := 0
	changed := false
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 || m[3] < 0 {
			continue
		}
		token := text[m[2]:m[3]]
		ident := labelLeadingSymbolIdentifier(token)
		if !idents[ident] {
			continue
		}
		b.WriteString(text[last:m[0]])
		b.WriteString(token)
		last = m[1]
		changed = true
	}
	if !changed {
		return text, false
	}
	b.WriteString(text[last:])
	return b.String(), true
}

func finalizerAutoRepairAuthorityEvidencePool(ctx *types.BusContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	var emitted []types.EvidenceItem
	if ctx.Mutable != nil {
		emitted = ctx.Mutable.EmittedEvidence()
	}
	merged, _ := agent.MergeEvidenceItemsIfChanged(ctx.EvidenceItems, emitted)
	return append([]types.EvidenceItem(nil), merged...)
}

func marshalFinalizerAutoRepairStageData(finalAnswer string) json.RawMessage {
	data, err := json.Marshal(struct {
		FinalAnswer string `json:"final_answer"`
	}{FinalAnswer: finalAnswer})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func dedupeSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
