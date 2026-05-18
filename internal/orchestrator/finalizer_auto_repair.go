package orchestrator

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	answertool "github.com/hanchaoqun/codrax/internal/tool"
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

func (o *Orchestrator) recoverRejectedFinalizerDraftAfterTransientFailure(c types.AnswerContract) *agent.StageOutput {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	doc, source := o.finalizerRecoveryDraftCandidate()
	if doc == nil || len(doc.Blocks) == 0 {
		return nil
	}
	changed := false
	if view := types.BuildAnswerSemanticViewForBusContext(o.busCtx); view != nil {
		viewChanged, ok := answertool.NormalizeAnswerDocumentForRecovery(doc, view, o.busCtx)
		if !ok {
			return nil
		}
		changed = viewChanged || changed
	}
	if rs := o.busCtx.Mutable.RetryState(); rs != nil {
		facets, inlineIdents, ok := recoverableRetryStateFinalizerMetadata(rs)
		if ok {
			changed = addFacetIDsToPrincipalAnswerBlock(doc, facets) || changed
			changed = stripUnsupportedInlineIdentifierBackticks(doc, inlineIdents) || changed
		}
	}
	if !changed && source != "accepted" {
		return nil
	}
	renderDoc := doc
	render.ApplyAuthorityHedging(renderDoc, finalizerAutoRepairAuthorityEvidencePool(o.busCtx), o.busCtx.Language)
	answer := render.RenderAnswerDocumentWithAttachments(
		renderDoc,
		o.busCtx.Mutable.AnswerDisplayAttachments(),
		o.busCtx.Language,
	)
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	out := &agent.StageOutput{
		FinalAnswer: answer,
		Data:        marshalFinalizerAutoRepairStageData(answer),
	}
	o.busCtx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	res := runContractCheck(out, c, o.busCtx.Mutable, o)
	if len(res.Violations) > 0 {
		// Do not turn a genuinely invalid structured draft into a final
		// answer. The recovery path is only for deterministic metadata
		// repairs that pass the same contract check normal finalizer output
		// must pass.
		o.busCtx.Mutable.ResetAnswerDocumentV2()
		if source == "rejected" {
			o.busCtx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
		}
		return nil
	}
	out.FinalAnswer = appendFinalizerRecoveredDraftCaveat(out.FinalAnswer, o.busCtx.Language)
	out.Data = marshalFinalizerAutoRepairStageData(out.FinalAnswer)
	return out
}

func (o *Orchestrator) finalizerRecoveryDraftCandidate() (*types.AnswerDocumentV2, string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil, ""
	}
	if doc := o.busCtx.Mutable.LastRejectedAnswerDocumentV2(); doc != nil && len(doc.Blocks) > 0 {
		return doc, "rejected"
	}
	if rs := o.busCtx.Mutable.RetryState(); rs != nil && len(rs.PrevEmitJSON) > 0 {
		var doc types.AnswerDocumentV2
		if err := json.Unmarshal(rs.PrevEmitJSON, &doc); err == nil && len(doc.Blocks) > 0 {
			return &doc, "retry_state"
		}
	}
	if doc := o.busCtx.Mutable.AnswerDocumentV2(); doc != nil && len(doc.Blocks) > 0 {
		return doc, "accepted"
	}
	return nil, ""
}

func recoverableRetryStateFinalizerMetadata(rs *types.RetryState) ([]string, map[string]bool, bool) {
	if rs == nil || len(rs.ActiveViolations) == 0 {
		return nil, nil, true
	}
	seenFacets := map[string]bool{}
	var facets []string
	inlineIdents := map[string]bool{}
	for _, v := range rs.ActiveViolations {
		switch v.Kind {
		case types.ViolFacetUncovered:
			m := finalizerAutoRepairFacetRE.FindStringSubmatch(v.Detail)
			if len(m) < 2 {
				return nil, nil, false
			}
			facet := strings.TrimSpace(m[1])
			if facet == "" || !isFinalizerAutoRepairableFacet(facet) {
				return nil, nil, false
			}
			if !seenFacets[facet] {
				seenFacets[facet] = true
				facets = append(facets, facet)
			}
		case types.ViolInlineIdentifierHallucinated:
			found := false
			for _, m := range finalizerAutoRepairInlineIdentRE.FindAllStringSubmatch(v.Detail, -1) {
				if len(m) < 2 {
					continue
				}
				ident := strings.TrimSpace(m[1])
				if ident == "" {
					continue
				}
				inlineIdents[ident] = true
				found = true
			}
			if !found {
				return nil, nil, false
			}
		default:
			return nil, nil, false
		}
	}
	sort.Strings(facets)
	return facets, inlineIdents, true
}

func appendFinalizerRecoveredDraftCaveat(answer, lang string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "en") {
		return answer + "\n\n---\n\n**Supplement:** The final answer pass hit a transient model stream failure after producing this structured draft. Codrax preserved the draft and only completed deterministic non-visible metadata repairs before rendering it."
	}
	return answer + "\n\n---\n\n**补充说明：** 成文阶段在产出这版结构化草稿后遇到模型流式响应瞬时失败；系统保留该草稿，仅补齐确定性的非可见结构元数据后展示。"
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
