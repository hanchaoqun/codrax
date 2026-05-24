package tool

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizePrincipalEnumerationRowBlocks is the deterministic presentation
// compiler for accepted exhaustive member-set answers. It turns the typed
// handoff produced by exploration into rich visible table/list rows inside the
// model's existing answer shape, instead of forcing another finalizer rewrite
// or appending a second dry fallback list.
//
// Safety boundary: the row source is only types.CompileEnumerationDisplaySets
// (accepted aggregate_facts + grounded evidence/support refs). Model-authored
// block prose is used only to choose which existing block should display a set;
// it never creates, removes, or reclassifies members.
func normalizePrincipalEnumerationRowBlocks(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return 0
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return 0
	}
	if answerDocumentRuntimeObservationOnly(ctx) || principalEnumerationSystemSupplementSuppressed(doc, ctx) {
		return 0
	}
	sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, plan)
	if len(sets) == 0 {
		return 0
	}
	changed := normalizePrincipalEnumerationItemCitationRefs(doc, sets)
	zh := principalEnumerationPrefersZH(ctx)
	missingBySet := make(map[string][]types.EnumerationDisplayRow, len(sets))
	for _, set := range sets {
		missingRows := principalEnumerationMissingRows(doc, set)
		if len(missingRows) > 0 {
			missingBySet[set.ID] = missingRows
		}
	}
	if normalizePrincipalEnumerationSummary(doc, ctx, sets) {
		changed++
	}
	for _, set := range sets {
		if len(set.Rows) == 0 {
			continue
		}
		if annotated := annotatePrincipalEnumerationCoveredBlocks(doc, set); annotated > 0 {
			changed += annotated
		}
		missingRows := missingBySet[set.ID]
		supplementRows := missingRows
		supplementMode := principalEnumerationSupplementMissing
		if principalEnumerationNeedsVerifiedFieldSupplement(doc, set) {
			supplementRows = set.Rows
			supplementMode = principalEnumerationSupplementVerifiedFields
		}
		if len(supplementRows) == 0 {
			if noteRows := principalEnumerationRowsNeedingNoteSupplement(doc, set); len(noteRows) > 0 {
				supplementRows = noteRows
				supplementMode = principalEnumerationSupplementVerifiedNotes
			}
		}
		supplementRows = principalEnumerationRenderableSupplementRows(supplementRows)
		if len(supplementRows) > 0 {
			doc.Blocks = append(doc.Blocks, buildPrincipalEnumerationRowsBlock(doc, set, supplementRows, zh, supplementMode))
			changed++
		}
		if normalizePrincipalEnumerationSectionBlocks(doc, set) > 0 {
			changed++
		}
	}
	return changed
}

type principalEnumerationSupplementMode int

const (
	principalEnumerationSupplementMissing principalEnumerationSupplementMode = iota
	principalEnumerationSupplementVerifiedFields
	principalEnumerationSupplementVerifiedNotes
)

func principalEnumerationSystemSupplementSuppressed(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil || doc.ExactResolution == nil {
		return false
	}
	if doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Predicates.IsCategoryEnumeration || rm.Intent == types.IntentEnumerate {
		return false
	}
	return true
}

func normalizePrincipalEnumerationItemCitationRefs(doc *types.AnswerDocumentV2, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	index := principalEnumerationExactLabelRowIndex(sets)
	if len(index) == 0 {
		return 0
	}
	changed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !principalEnumerationBlockCanCarryRows(*block) || strings.TrimSpace(block.Text) != "" {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			row, ok := index[normalizeEnumerationDisplayTableKey(item.Label)]
			if !ok || !principalEnumerationItemCitationRefShouldCorrect(*item, doc, row) {
				continue
			}
			ref := appendOrReusePreEmitCitation(doc, types.Citation{File: row.Source, Line: row.LineStart})
			if ref < 0 || item.CitationRef == ref {
				continue
			}
			item.CitationRef = ref
			changed++
		}
	}
	return changed
}

func principalEnumerationExactLabelRowIndex(sets []types.EnumerationDisplaySet) map[string]types.EnumerationDisplayRow {
	candidates := make(map[string]types.EnumerationDisplayRow)
	ambiguous := make(map[string]bool)
	add := func(raw string, row types.EnumerationDisplayRow) {
		key := normalizeEnumerationDisplayTableKey(raw)
		if key == "" || ambiguous[key] {
			return
		}
		if existing, ok := candidates[key]; ok && existing.RowID != row.RowID {
			delete(candidates, key)
			ambiguous[key] = true
			return
		}
		candidates[key] = row
	}
	for _, set := range sets {
		for _, row := range set.Rows {
			add(row.DisplayLabel, row)
			add(row.Member, row)
			if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
				add(label, row)
			}
		}
	}
	return candidates
}

func principalEnumerationItemCitationRefShouldCorrect(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil || !row.HasCitation || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
		return false
	}
	if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
		return false
	}
	if principalEnumerationItemCitationCompatible(item, doc, row) {
		return false
	}
	if !principalEnumerationCitationFileMatches(doc.Citations[item.CitationRef], row) {
		return false
	}
	surface := types.AnswerBlockItemVisibleSurface(item)
	locations := aggregateToolLocationPattern.FindAllString(surface, -1)
	if len(locations) > 0 && !principalEnumerationCandidateLocationCompatible(surface, row) {
		return false
	}
	return true
}

func principalEnumerationCitationFileMatches(cit types.Citation, row types.EnumerationDisplayRow) bool {
	return principalEnumerationLocationKeyMatches(
		principalEnumerationLocationKey(cit.File),
		principalEnumerationLocationKey(row.Source),
	) || principalEnumerationLocationKeyMatches(
		principalEnumerationLocationKey(row.Source),
		principalEnumerationLocationKey(cit.File),
	)
}

func normalizePrincipalEnumerationSummary(doc *types.AnswerDocumentV2, ctx *types.BusContext, sets []types.EnumerationDisplaySet) bool {
	if doc == nil || len(sets) == 0 {
		return false
	}
	text := principalEnumerationSummaryText(ctx, sets)
	if strings.TrimSpace(text) == "" {
		return false
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind != types.BlockSummary {
			continue
		}
		changed := false
		if strings.TrimSpace(doc.Blocks[i].Text) == "" {
			doc.Blocks[i].Text = text
			changed = true
		}
		if doc.Blocks[i].SurfaceRole != types.SurfacePrincipal {
			doc.Blocks[i].SurfaceRole = types.SurfacePrincipal
			changed = true
		}
		facetIDs := mergeStringSet(doc.Blocks[i].FacetIDs, []string{string(types.FacetEnumerationItem)})
		if !stringSlicesEqual(doc.Blocks[i].FacetIDs, facetIDs) {
			doc.Blocks[i].FacetIDs = facetIDs
			changed = true
		}
		claimUses := appendRenderedClaimUseIfMissing(doc.Blocks[i].ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
		if len(claimUses) != len(doc.Blocks[i].ClaimUses) {
			doc.Blocks[i].ClaimUses = claimUses
			changed = true
		}
		return changed
	}
	doc.Blocks = append([]types.AnswerBlock{{
		ID:          uniqueAnswerBlockID(doc, "principal_enum_summary"),
		Kind:        types.BlockSummary,
		Text:        text,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimDefinitionFact,
			FacetID:   string(types.FacetEnumerationItem),
		}},
	}}, doc.Blocks...)
	return true
}

func principalEnumerationSummaryText(ctx *types.BusContext, sets []types.EnumerationDisplaySet) string {
	zh := principalEnumerationPrefersZH(ctx)
	parts := make([]string, 0, len(sets))
	for _, set := range sets {
		label := strings.TrimSpace(set.Label)
		if label == "" {
			label = "member set"
		}
		count := len(set.Rows)
		if zh {
			parts = append(parts, fmt.Sprintf("%s %d 项", label, count))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %d item(s)", label, count))
		}
	}
	if zh {
		suffix := ""
		if principalEnumerationHasTypedExclusion(ctx) {
			suffix = "；已按结构化排除/可见性策略移除不属于主答案范围的候选"
		}
		return "本轮按已验收的结构化调查清单列出：" + strings.Join(parts, "、") + "。每行使用已落地的定义锚点和证据说明" + suffix + "。"
	}
	suffix := ""
	if principalEnumerationHasTypedExclusion(ctx) {
		suffix = "; typed exclusion/visibility policy has removed candidates outside the answer scope"
	}
	return "This answer renders the accepted structured investigation slate: " + strings.Join(parts, ", ") + ". Each row uses a grounded definition anchor and evidence note" + suffix + "."
}

func principalEnumerationPrefersZH(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return true
	}
	lang := strings.ToLower(strings.TrimSpace(ctx.AnalysisIR.RequestModel.Language))
	return lang == "" || strings.HasPrefix(lang, "zh")
}

func principalEnumerationHasTypedExclusion(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.AnswerExclusionPolicy != nil && rm.AnswerExclusionPolicy.Active() {
		return true
	}
	return rm.AnswerVisibilityProfile != nil && rm.AnswerVisibilityProfile.ExcludesPrivateSymbols()
}

func annotatePrincipalEnumerationCoveredBlocks(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) int {
	if doc == nil || len(set.Rows) == 0 {
		return 0
	}
	changed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if !principalEnumerationBlockCanCarryRows(*block) || !principalEnumerationBlockCoversAnyRow(*block, doc, set) {
			continue
		}
		if block.SurfaceRole != types.SurfacePrincipal {
			block.SurfaceRole = types.SurfacePrincipal
			changed++
		}
		facetIDs := mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
		if !stringSlicesEqual(block.FacetIDs, facetIDs) {
			block.FacetIDs = facetIDs
			changed++
		}
		claimUses := appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
		if len(claimUses) != len(block.ClaimUses) {
			block.ClaimUses = claimUses
			changed++
		}
	}
	return changed
}

func principalEnumerationMissingRows(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) []types.EnumerationDisplayRow {
	if doc == nil || len(set.Rows) == 0 {
		return nil
	}
	var missing []types.EnumerationDisplayRow
	for _, row := range set.Rows {
		if !principalEnumerationDocumentCoversRow(doc, row) {
			missing = append(missing, row)
		}
	}
	return missing
}

func principalEnumerationDocumentCoversRow(doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil {
		return false
	}
	if principalEnumerationRuntimeArtifactRowCoveredByAnyVisibleText(doc, row) {
		return true
	}
	if principalEnumerationRowCoveredByAnyVisibleText(doc, row) {
		return true
	}
	for _, block := range doc.Blocks {
		if !principalEnumerationBlockCanCarryRows(block) {
			continue
		}
		if principalEnumerationBlockCoversRow(block, doc, row) {
			return true
		}
	}
	return false
}

func principalEnumerationRowCoveredByAnyVisibleText(doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if principalEnumerationVisibleSurfaceCoversRow(types.AnswerBlockVisibleSurface(block), row) {
			return true
		}
	}
	return false
}

func principalEnumerationRuntimeArtifactRowCoveredByAnyVisibleText(doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if !principalEnumerationRowHasOrigin(row, types.AnswerEvidenceOriginRuntimeArtifact) {
		return false
	}
	needles := []string{
		strings.TrimSpace(row.DisplayLabel),
		strings.TrimSpace(row.Member),
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
		needles = append(needles, strings.TrimSpace(label))
	}
	for _, block := range doc.Blocks {
		surface := types.AnswerBlockVisibleSurface(block)
		if strings.TrimSpace(surface) == "" {
			continue
		}
		for _, needle := range needles {
			if strings.TrimSpace(needle) == "" {
				continue
			}
			if strings.Contains(surface, needle) {
				return true
			}
		}
		if principalEnumerationRuntimeArtifactNumericShorthandCovered(surface, row) {
			return true
		}
	}
	return false
}

func principalEnumerationRuntimeArtifactNumericShorthandCovered(surface string, row types.EnumerationDisplayRow) bool {
	id, ok := principalEnumerationRuntimeArtifactNumberedMember(row)
	if !ok {
		return false
	}
	if !strings.Contains(strings.ToLower(surface), "goroutine") {
		return false
	}
	return containsDecimalToken(surface, id)
}

func principalEnumerationRuntimeArtifactNumberedMember(row types.EnumerationDisplayRow) (string, bool) {
	for _, candidate := range []string{row.DisplayLabel, row.Member} {
		candidate = strings.TrimSpace(strings.ToLower(candidate))
		if !strings.HasPrefix(candidate, "goroutine ") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(candidate, "goroutine "))
		if id == "" {
			continue
		}
		allDigits := true
		for _, r := range id {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return id, true
		}
	}
	return "", false
}

func containsDecimalToken(surface, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	for start := 0; start < len(surface); {
		idx := strings.Index(surface[start:], token)
		if idx < 0 {
			return false
		}
		pos := start + idx
		beforeOK := pos == 0 || !isASCIIDigit(surface[pos-1])
		after := pos + len(token)
		afterOK := after >= len(surface) || !isASCIIDigit(surface[after])
		if beforeOK && afterOK {
			return true
		}
		start = after
	}
	return false
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func principalEnumerationBlockCoversAnyRow(block types.AnswerBlock, doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) bool {
	for _, row := range set.Rows {
		if principalEnumerationBlockCoversRow(block, doc, row) {
			return true
		}
	}
	return false
}

func principalEnumerationBlockCoversRow(block types.AnswerBlock, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if text := strings.TrimSpace(block.Text); text != "" {
		for _, cells := range principalEnumerationMarkdownTableRows(text) {
			if principalEnumerationMarkdownRowCoversRow(cells, row) {
				return true
			}
		}
	}
	for _, item := range block.Items {
		if principalEnumerationStructuredItemCoversRow(item, doc, row) {
			return true
		}
	}
	return false
}

func principalEnumerationMarkdownRowCoversRow(cells []string, row types.EnumerationDisplayRow) bool {
	if len(cells) == 0 || !principalEnumerationAnySurfaceMatchesRow(cells, row) {
		return false
	}
	return principalEnumerationCandidateLocationCompatible(strings.Join(cells, " "), row)
}

func principalEnumerationStructuredItemCoversRow(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if principalEnumerationStructuredItemCoversDecoratedBase(item, doc, row) {
		return true
	}
	surface := types.AnswerBlockItemVisibleSurface(item)
	if !principalEnumerationAnySurfaceMatchesRow([]string{surface, item.Label}, row) {
		return false
	}
	if principalEnumerationCandidateLocationCompatible(surface, row) {
		return true
	}
	return principalEnumerationItemCitationCompatible(item, doc, row) ||
		principalEnumerationItemCitationFileCompatible(item, doc, row)
}

func principalEnumerationStructuredItemCoversDecoratedBase(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	bases := principalEnumerationRowDecoratedBaseCandidates(row)
	if len(bases) == 0 {
		return false
	}
	surface := types.AnswerBlockItemVisibleSurface(item)
	labelKey := normalizeEnumerationDisplayTableKey(item.Label)
	for _, base := range bases {
		baseKey := normalizeEnumerationDisplayTableKey(base)
		if baseKey == "" {
			continue
		}
		if labelKey != baseKey && !preEmitAggregateScalarValueAppears(base, surface) {
			continue
		}
		if len(aggregateToolLocationPattern.FindAllString(surface, -1)) > 0 &&
			principalEnumerationCandidateLocationCompatible(surface, row) {
			return true
		}
		if principalEnumerationItemCitationCompatible(item, doc, row) ||
			principalEnumerationItemCitationFileCompatible(item, doc, row) {
			return true
		}
	}
	return false
}

func principalEnumerationRowDecoratedBaseCandidates(row types.EnumerationDisplayRow) []string {
	raw := []string{row.DisplayLabel, row.Member}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
		raw = append(raw, label)
	}
	var out []string
	seen := map[string]bool{}
	for _, candidate := range raw {
		base, _, ok := types.AnswerAggregateDecoratedLabelParts(candidate)
		if !ok {
			continue
		}
		base = strings.TrimSpace(base)
		key := strings.ToLower(base)
		if base == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, base)
	}
	return out
}

func principalEnumerationItemCitationCompatible(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil || item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) || row.Source == "" || row.LineStart <= 0 {
		return false
	}
	cit := doc.Citations[item.CitationRef]
	return principalEnumerationLocationKeyMatches(
		principalEnumerationLocationKey(fmt.Sprintf("%s:%d", cit.File, cit.Line)),
		principalEnumerationLocationKey(fmt.Sprintf("%s:%d", row.Source, row.LineStart)),
	)
}

func principalEnumerationItemCitationFileCompatible(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil || item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) || row.Source == "" {
		return false
	}
	return principalEnumerationCitationFileMatches(doc.Citations[item.CitationRef], row)
}

func principalEnumerationBlockCanCarryRows(block types.AnswerBlock) bool {
	switch block.Kind {
	case types.BlockTable, types.BlockOrderedList, types.BlockBulletList:
		return true
	default:
		return false
	}
}

func principalEnumerationBlockSetScore(block types.AnswerBlock, set types.EnumerationDisplaySet) int {
	surface := types.AnswerBlockVisibleSurface(block)
	if strings.TrimSpace(surface) == "" {
		return 0
	}
	score := principalEnumerationSetLabelMatchScore(surface, set)
	overlap := 0
	for _, row := range set.Rows {
		if preEmitAggregateMemberAppearsInText(row.Member, surface) ||
			preEmitAggregateMemberAppearsInText(row.DisplayLabel, surface) {
			overlap++
		}
	}
	score += overlap * 10
	if overlap == len(set.Rows) && overlap > 0 {
		score += 20
	}
	return score
}

func principalEnumerationSetLabelMatchScore(surface string, set types.EnumerationDisplaySet) int {
	key := principalEnumerationLabelKey(set.Label)
	if key == "" {
		key = principalEnumerationLabelKey(set.ID)
	}
	surfaceKey := principalEnumerationLabelKey(surface)
	if key == "" || surfaceKey == "" {
		return 0
	}
	if surfaceKey == key {
		return 40
	}
	if strings.Contains(surfaceKey, key) {
		return 35
	}
	return 0
}

func buildPrincipalEnumerationRowsBlock(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet, rows []types.EnumerationDisplayRow, zh bool, mode principalEnumerationSupplementMode) types.AnswerBlock {
	blockSet := set
	blockSet.Rows = append([]types.EnumerationDisplayRow(nil), rows...)
	shape := principalEnumerationTableShapeForRows(blockSet.Rows, nil)
	block := types.AnswerBlock{
		ID:          uniqueAnswerBlockID(doc, "principal_enum_"+sanitizeEnumerationBlockID(set.ID)),
		Kind:        types.BlockTable,
		Title:       principalEnumerationRowsBlockTitle(set, rows, zh, mode),
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimDefinitionFact,
			FacetID:   string(types.FacetEnumerationItem),
		}},
		Columns: principalEnumerationTableColumns(zh, shape, blockSet.Rows),
	}
	block.Items = principalEnumerationItemsForSet(doc, blockSet, block.Kind, nil, shape)
	return block
}

func principalEnumerationNeedsVerifiedFieldSupplement(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) bool {
	if doc == nil || len(set.Rows) == 0 {
		return false
	}
	// Do not publish a second "complete system table" just because a
	// model-authored table layout is incompatible with the deterministic
	// compiler. That turns a local carrier repair into a competing answer
	// surface. When the table already names the intended rows but cannot be
	// safely rewritten in place, the separate supplement is framed as verified
	// fields for those rows, not as a replacement table.
	if principalEnumerationHasIncompatibleStructuredTableAttempt(doc, set) {
		return true
	}
	return false
}

func principalEnumerationRowsNeedingNoteSupplement(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) []types.EnumerationDisplayRow {
	if doc == nil || len(set.Rows) == 0 {
		return nil
	}
	visible := preEmitVisibleAnswerSurface(doc)
	var out []types.EnumerationDisplayRow
	for _, row := range set.Rows {
		if strings.TrimSpace(row.Note) == "" || !principalEnumerationDocumentCoversRow(doc, row) {
			continue
		}
		if principalEnumerationNoteVisibleInSurface(visible, row.Note) {
			continue
		}
		if principalEnumerationRowHasAuthoredDescription(doc, row) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func principalEnumerationNoteVisibleInSurface(surface, note string) bool {
	surfaceKey := principalEnumerationNoteCoverageKey(surface)
	noteKey := principalEnumerationNoteCoverageKey(note)
	if surfaceKey == "" || noteKey == "" {
		return false
	}
	return strings.Contains(surfaceKey, noteKey)
}

func principalEnumerationNoteCoverageKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Trim(raw, "`*_")
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

func principalEnumerationRowHasAuthoredDescription(doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if !principalEnumerationBlockCanCarryRows(block) {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			for _, cells := range principalEnumerationMarkdownTableRows(text) {
				if principalEnumerationMarkdownRowCoversRow(cells, row) &&
					principalEnumerationMarkdownRowHasAuthoredDescription(cells, row) {
					return true
				}
			}
		}
		for _, item := range block.Items {
			if principalEnumerationStructuredItemCoversRow(item, doc, row) &&
				principalEnumerationStructuredItemHasAuthoredDescription(item, row) {
				return true
			}
		}
	}
	return false
}

func principalEnumerationMarkdownRowHasAuthoredDescription(cells []string, row types.EnumerationDisplayRow) bool {
	for _, cell := range cells {
		if principalEnumerationCellIsAuthoredDescription(cell, row) {
			return true
		}
	}
	return false
}

func principalEnumerationStructuredItemHasAuthoredDescription(item types.AnswerBlockItem, row types.EnumerationDisplayRow) bool {
	if principalEnumerationCellIsAuthoredDescription(item.Text, row) {
		return true
	}
	for _, cell := range item.Cells {
		if principalEnumerationCellIsAuthoredDescription(cell, row) {
			return true
		}
	}
	return false
}

func principalEnumerationCellIsAuthoredDescription(raw string, row types.EnumerationDisplayRow) bool {
	cell := principalEnumerationCleanCellText(raw)
	if cell == "" {
		return false
	}
	for _, candidate := range principalEnumerationRowSurfaceCandidates(row) {
		if principalEnumerationNoteCoverageKey(cell) == principalEnumerationNoteCoverageKey(candidate) {
			return false
		}
	}
	if category := strings.TrimSpace(row.Category); category != "" &&
		principalEnumerationNoteCoverageKey(cell) == principalEnumerationNoteCoverageKey(category) {
		return false
	}
	if row.Location != "" && principalEnumerationCandidateLocationCompatible(cell, row) &&
		len(aggregateToolLocationPattern.FindAllString(cell, -1)) > 0 {
		withoutLocations := strings.TrimSpace(aggregateToolLocationPattern.ReplaceAllString(cell, ""))
		if withoutLocations == "" {
			return false
		}
	}
	return principalEnumerationDescriptionCellLooksMeaningful(cell)
}

func principalEnumerationCleanCellText(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`*_")
	raw = strings.ReplaceAll(raw, "<br>", " ")
	raw = strings.ReplaceAll(raw, "<br/>", " ")
	raw = strings.ReplaceAll(raw, "<br />", " ")
	return strings.Join(strings.Fields(raw), " ")
}

func principalEnumerationDescriptionCellLooksMeaningful(cell string) bool {
	if cell == "" {
		return false
	}
	meaningful := 0
	for _, r := range cell {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			meaningful++
		}
	}
	return meaningful >= 2
}

func principalEnumerationHasIncompatibleStructuredTableAttempt(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) bool {
	if doc == nil || len(set.Rows) == 0 {
		return false
	}
	index := enumerationDisplayRowIndex([]types.EnumerationDisplaySet{set})
	if len(index) == 0 {
		return false
	}
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" {
			continue
		}
		rows, ok := enumerationDisplayRowsForIncompleteTable(block, index)
		if !ok || !principalEnumerationRowsHaveLocationOrNote(rows) {
			continue
		}
		if _, ok := enumerationDisplayExistingTableShape(block, rows); !ok {
			return true
		}
	}
	return false
}

func principalEnumerationRowsHaveLocationOrNote(rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.Location) != "" || strings.TrimSpace(row.Note) != "" {
			return true
		}
	}
	return false
}

type principalEnumerationMarkdownTableStats struct {
	dataRows       int
	matchedRows    int
	missingRows    int
	duplicateRows  int
	unexpectedRows int
}

func principalEnumerationAuthoredMarkdownTableStats(block types.AnswerBlock, set types.EnumerationDisplaySet) principalEnumerationMarkdownTableStats {
	var stats principalEnumerationMarkdownTableStats
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) == "" || len(set.Rows) == 0 {
		return stats
	}
	rows := principalEnumerationMarkdownTableRows(block.Text)
	stats.dataRows = len(rows)
	if len(rows) == 0 {
		return stats
	}
	expected := map[string]bool{}
	for _, row := range set.Rows {
		if key := principalEnumerationPrimaryRowKey(row); key != "" {
			expected[key] = true
		}
	}
	seen := map[string]bool{}
	for _, cells := range rows {
		key, ok := principalEnumerationMarkdownRowMatchedKey(cells, set)
		if !ok || key == "" {
			stats.unexpectedRows++
			continue
		}
		stats.matchedRows++
		if seen[key] {
			stats.duplicateRows++
			continue
		}
		seen[key] = true
	}
	for key := range expected {
		if !seen[key] {
			stats.missingRows++
		}
	}
	return stats
}

func principalEnumerationMarkdownRowMatchedKey(cells []string, set types.EnumerationDisplaySet) (string, bool) {
	matched := ""
	for _, row := range set.Rows {
		if !principalEnumerationMarkdownRowCoversRow(cells, row) {
			continue
		}
		key := principalEnumerationPrimaryRowKey(row)
		if key == "" {
			continue
		}
		if matched != "" && matched != key {
			return "", false
		}
		matched = key
	}
	if matched == "" {
		return "", false
	}
	return matched, true
}

func principalEnumerationRenderableSupplementRows(rows []types.EnumerationDisplayRow) []types.EnumerationDisplayRow {
	if len(rows) == 0 {
		return nil
	}
	shape := principalEnumerationTableShapeForRows(rows, nil)
	out := make([]types.EnumerationDisplayRow, 0, len(rows))
	for _, row := range rows {
		if principalEnumerationRuntimeArtifactCoordinateOnly(row) {
			continue
		}
		if !principalEnumerationRowCompatibleWithTableShape(row, nil, shape) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func principalEnumerationRuntimeArtifactCoordinateOnly(row types.EnumerationDisplayRow) bool {
	if !principalEnumerationRowHasOrigin(row, types.AnswerEvidenceOriginRuntimeArtifact) {
		return false
	}
	if strings.TrimSpace(row.Location) != "" || strings.TrimSpace(row.Note) != "" {
		return false
	}
	surface := strings.TrimSpace(firstNonEmptyAnswerString(row.DisplayLabel, row.Member))
	if surface == "" {
		return true
	}
	lower := strings.ToLower(surface)
	return strings.Contains(lower, "@runtime:") ||
		strings.Contains(lower, "@artifact:") ||
		strings.HasSuffix(lower, ":0")
}

func principalEnumerationRowHasOrigin(row types.EnumerationDisplayRow, origin types.AnswerEvidenceOrigin) bool {
	for _, candidate := range row.EvidenceOrigins {
		if candidate == origin {
			return true
		}
	}
	return false
}

func principalEnumerationRowsBlockTitle(set types.EnumerationDisplaySet, rows []types.EnumerationDisplayRow, zh bool, mode principalEnumerationSupplementMode) string {
	label := strings.TrimSpace(set.Label)
	if label == "" {
		label = "成员清单"
	}
	if mode == principalEnumerationSupplementMissing {
		if zh {
			return fmt.Sprintf("系统按已验证证据补充缺失成员：%s（%d）", label, len(rows))
		}
		return fmt.Sprintf("System-verified missing member supplement: %s (%d)", label, len(rows))
	}
	if mode == principalEnumerationSupplementVerifiedFields {
		if zh {
			return fmt.Sprintf("系统按已验证证据补充可校验字段：%s（%d）", label, len(rows))
		}
		return fmt.Sprintf("System-verified field supplement: %s (%d)", label, len(rows))
	}
	if mode == principalEnumerationSupplementVerifiedNotes {
		if zh {
			return fmt.Sprintf("系统按已验证证据补充说明：%s（%d）", label, len(rows))
		}
		return fmt.Sprintf("System-verified note supplement: %s (%d)", label, len(rows))
	}
	if zh {
		return fmt.Sprintf("系统按已验证证据补充成员：%s（%d）", label, len(rows))
	}
	return fmt.Sprintf("System-verified member supplement: %s (%d)", label, len(rows))
}

type principalEnumerationTableShape struct {
	includeLocation bool
	includeNote     bool
}

func principalEnumerationTableShapeForRows(rows []types.EnumerationDisplayRow, existingNotes map[string]string) principalEnumerationTableShape {
	var shape principalEnumerationTableShape
	for _, row := range rows {
		if strings.TrimSpace(row.Location) != "" {
			shape.includeLocation = true
		}
		if strings.TrimSpace(principalEnumerationRowNote(row, existingNotes)) != "" {
			shape.includeNote = true
		}
	}
	return shape
}

func principalEnumerationTableColumns(zh bool, shape principalEnumerationTableShape, rows []types.EnumerationDisplayRow) []string {
	columns := []string{principalEnumerationPrimaryColumnLabel(zh, rows)}
	if shape.includeLocation {
		if zh {
			columns = append(columns, "定义位置")
		} else {
			columns = append(columns, "Location")
		}
	}
	if shape.includeNote {
		if zh {
			columns = append(columns, "说明")
		} else {
			columns = append(columns, "Notes")
		}
	}
	return columns
}

func principalEnumerationPrimaryColumnLabel(zh bool, rows []types.EnumerationDisplayRow) string {
	if principalEnumerationRowsUseFilePrimarySurface(rows) {
		if zh {
			return "文件"
		}
		return "File"
	}
	origin, ok := principalEnumerationUniformPrimaryOrigin(rows)
	if !ok {
		if zh {
			return "项目"
		}
		return "Item"
	}
	switch origin {
	case types.AnswerEvidenceOriginVCSMetadata:
		if zh {
			return "提交"
		}
		return "Commit"
	case types.AnswerEvidenceOriginVCSDiff:
		if zh {
			return "变更"
		}
		return "Change"
	case types.AnswerEvidenceOriginRuntimeArtifact:
		if zh {
			return "观察项"
		}
		return "Observation"
	case types.AnswerEvidenceOriginCommandMeasurement:
		if zh {
			return "命令结果"
		}
		return "Command result"
	case types.AnswerEvidenceOriginRepoNegativeSearch:
		if zh {
			return "负向查询"
		}
		return "Negative check"
	case types.AnswerEvidenceOriginCrossRepoIndex:
		if zh {
			return "仓库项"
		}
		return "Repository item"
	case types.AnswerEvidenceOriginExternalDocument,
		types.AnswerEvidenceOriginWebPage,
		types.AnswerEvidenceOriginMCPResource,
		types.AnswerEvidenceOriginConnectorResource:
		if zh {
			return "资源项"
		}
		return "Resource"
	default:
		if zh {
			return "符号名称"
		}
		return "Name"
	}
}

func principalEnumerationRowsUseFilePrimarySurface(rows []types.EnumerationDisplayRow) bool {
	if len(rows) == 0 {
		return false
	}
	checked := 0
	for _, row := range rows {
		surface := strings.TrimSpace(firstNonEmptyAnswerString(row.DisplayLabel, row.Member))
		if surface == "" {
			continue
		}
		if label, location, ok := types.ParseAnswerSupportRefMemberLocation(surface); ok {
			if strings.TrimSpace(label) != "" {
				surface = strings.TrimSpace(label)
			} else {
				surface = location.File
			}
		}
		if _, ok := types.ParseAnswerFilePathSurface(surface); ok {
			checked++
			continue
		}
		if _, ok := types.ParseAnswerSourceLocationSurface(surface); ok {
			checked++
			continue
		}
		return false
	}
	return checked > 0
}

func principalEnumerationUniformPrimaryOrigin(rows []types.EnumerationDisplayRow) (types.AnswerEvidenceOrigin, bool) {
	var out types.AnswerEvidenceOrigin
	for _, row := range rows {
		origin := principalEnumerationPrimaryOrigin(row)
		if out == "" {
			out = origin
			continue
		}
		if origin != out {
			return "", false
		}
	}
	if out == "" {
		return types.AnswerEvidenceOriginCurrentSource, true
	}
	return out, true
}

func principalEnumerationPrimaryOrigin(row types.EnumerationDisplayRow) types.AnswerEvidenceOrigin {
	for _, origin := range row.EvidenceOrigins {
		if origin == types.AnswerEvidenceOriginUnknown || !origin.IsValid() {
			continue
		}
		return origin
	}
	return types.AnswerEvidenceOriginCurrentSource
}

func principalEnumerationItemsForSet(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet, kind types.AnswerBlockKind, existingNotes map[string]string, shape principalEnumerationTableShape) []types.AnswerBlockItem {
	items := make([]types.AnswerBlockItem, 0, len(set.Rows))
	for _, row := range set.Rows {
		label := firstNonEmptyAnswerString(row.DisplayLabel, row.Member)
		note := principalEnumerationRowNote(row, existingNotes)
		citationRef := -1
		if row.HasCitation && strings.TrimSpace(row.Source) != "" && row.LineStart > 0 {
			citationRef = appendOrReusePreEmitCitation(doc, types.Citation{File: row.Source, Line: row.LineStart})
		}
		item := types.AnswerBlockItem{
			ID:          firstNonEmptyAnswerString(row.RowID, uniqueAnswerBlockID(doc, "enum_item")),
			Label:       label,
			Text:        note,
			CitationRef: citationRef,
		}
		if kind == types.BlockTable {
			item.Cells = principalEnumerationTableCells(row, note, shape)
		}
		items = append(items, item)
	}
	return items
}

func principalEnumerationTableCells(row types.EnumerationDisplayRow, note string, shape principalEnumerationTableShape) []string {
	cells := make([]string, 0, 2)
	if shape.includeLocation {
		cells = append(cells, strings.TrimSpace(row.Location))
	}
	if shape.includeNote {
		cells = append(cells, strings.TrimSpace(note))
	}
	return cells
}

func principalEnumerationRowCompatibleWithTableShape(row types.EnumerationDisplayRow, existingNotes map[string]string, shape principalEnumerationTableShape) bool {
	if shape.includeLocation && strings.TrimSpace(row.Location) == "" {
		return false
	}
	if shape.includeNote && strings.TrimSpace(principalEnumerationRowNote(row, existingNotes)) == "" {
		return false
	}
	return true
}

func principalEnumerationRowNote(row types.EnumerationDisplayRow, existingNotes map[string]string) string {
	for _, key := range principalEnumerationRowKeys(row) {
		if note := strings.TrimSpace(existingNotes[key]); note != "" {
			return note
		}
	}
	return strings.TrimSpace(row.Note)
}

func principalEnumerationMarkdownTableRows(text string) [][]string {
	var rows [][]string
	hasSeparator := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		cells := principalEnumerationMarkdownTableCells(line)
		if len(cells) < 2 {
			continue
		}
		if principalEnumerationMarkdownSeparatorRow(cells) {
			if len(rows) == 1 {
				hasSeparator = true
			}
			continue
		}
		rows = append(rows, cells)
	}
	if hasSeparator && len(rows) > 1 {
		return rows[1:]
	}
	return rows
}

func principalEnumerationMarkdownTableCells(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" || !strings.Contains(line, "|") {
		return nil
	}
	raw := strings.Split(line, "|")
	cells := make([]string, 0, len(raw))
	for i, cell := range raw {
		if (i == 0 || i == len(raw)-1) && strings.TrimSpace(cell) == "" {
			continue
		}
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells
}

func principalEnumerationMarkdownSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func principalEnumerationAnySurfaceMatchesRow(surfaces []string, row types.EnumerationDisplayRow) bool {
	keys := principalEnumerationRowKeySet(row)
	for _, surface := range surfaces {
		if keys[normalizeEnumerationDisplayTableKey(surface)] {
			return true
		}
		if principalEnumerationVisibleSurfaceCoversRow(surface, row) {
			return true
		}
	}
	if principalEnumerationCommitSurfaceMatchesRow(surfaces, row) {
		return true
	}
	return false
}

func principalEnumerationVisibleSurfaceCoversRow(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	if !principalEnumerationCandidateLocationCompatible(surface, row) {
		return false
	}
	if principalEnumerationCommitSurfaceMatchesRow([]string{surface}, row) {
		return true
	}
	for _, candidate := range principalEnumerationRowSurfaceCandidates(row) {
		if preEmitDecoratedAggregateMemberAppearsInText(candidate, surface) ||
			preEmitAggregateScalarValueAppears(candidate, surface) ||
			principalEnumerationLooseSurfaceCoversCandidate(surface, candidate) {
			return true
		}
	}
	return false
}

func principalEnumerationRowSurfaceCandidates(row types.EnumerationDisplayRow) []string {
	raw := []string{row.DisplayLabel, row.Member}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
		raw = append(raw, label)
	}
	return dedupPreEmitStringCandidates(raw)
}

func principalEnumerationLooseSurfaceCoversCandidate(surface, candidate string) bool {
	parts := principalEnumerationCoverageParts(candidate)
	if len(parts) < 2 {
		return false
	}
	codeParts := 0
	nonCodeParts := 0
	nonCodeCovered := false
	for _, part := range parts {
		covered := principalEnumerationCoveragePartAppears(part, surface)
		if principalEnumerationCoveragePartCodeLike(part) {
			if !principalEnumerationCoverageCodePartRequired(part) {
				continue
			}
			codeParts++
			if !covered {
				return false
			}
			continue
		}
		nonCodeParts++
		if covered {
			nonCodeCovered = true
		}
	}
	if codeParts >= 2 {
		return nonCodeParts == 0 || nonCodeCovered
	}
	for _, part := range parts {
		if !principalEnumerationCoveragePartAppears(part, surface) {
			return false
		}
	}
	return true
}

func principalEnumerationCoveragePartAppears(part, surface string) bool {
	if preEmitAggregateDisplayPartAppears(part, surface) ||
		types.CodeSurfaceAppearsAsToken(part, surface) {
		return true
	}
	return principalEnumerationCodePartAppearsAsAlias(part, surface)
}

func principalEnumerationCoverageParts(candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil
	}
	fields := strings.FieldsFunc(candidate, func(r rune) bool {
		switch r {
		case '(', ')', '[', ']', '{', '}', ',', '，', ';', '；', ':', '：', '/', '|':
			return true
		default:
			return unicode.IsSpace(r)
		}
	})
	var out []string
	seen := map[string]bool{}
	for _, field := range fields {
		part := strings.TrimSpace(strings.Trim(field, "`\"'"))
		if !principalEnumerationCoveragePartUseful(part) {
			continue
		}
		key := strings.ToLower(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	return out
}

func principalEnumerationCoveragePartUseful(part string) bool {
	part = strings.TrimSpace(part)
	if part == "" {
		return false
	}
	runes := []rune(part)
	if len(runes) == 1 && !isASCIIAlphaNum(runes[0]) {
		return false
	}
	allDigits := true
	for _, r := range runes {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return false
	}
	lower := strings.ToLower(part)
	switch lower {
	case "个", "项", "row", "rows", "item", "items":
		return false
	default:
		return true
	}
}

func principalEnumerationCoveragePartCodeLike(part string) bool {
	for _, r := range part {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '.' || r == '-' || r == '=' {
			return true
		}
	}
	return false
}

func principalEnumerationCoverageCodePartRequired(part string) bool {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "advisory", "main", "stage", "stages", "conditional", "pipeline":
		return false
	default:
		return true
	}
}

func principalEnumerationCodePartAppearsAsAlias(part, surface string) bool {
	key := principalEnumerationCompactCodeKey(part)
	if len(key) < 4 {
		return false
	}
	for _, token := range principalEnumerationCodeTokens(surface) {
		tokenKey := principalEnumerationCompactCodeKey(token)
		if len(tokenKey) < 4 {
			continue
		}
		if tokenKey == key || strings.HasSuffix(tokenKey, key) || strings.HasSuffix(key, tokenKey) {
			return true
		}
	}
	return false
}

func principalEnumerationCodeTokens(surface string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range surface {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' || r == '=' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func principalEnumerationCompactCodeKey(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isASCIIAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

func principalEnumerationCommitSurfaceMatchesRow(surfaces []string, row types.EnumerationDisplayRow) bool {
	rowHashes := []string{
		principalEnumerationLeadingCommitHash(row.DisplayLabel),
		principalEnumerationLeadingCommitHash(row.Member),
	}
	for _, surface := range surfaces {
		for _, surfaceHash := range principalEnumerationCommitHashesInSurface(surface) {
			if surfaceHash == "" {
				continue
			}
			for _, rowHash := range rowHashes {
				if principalEnumerationCommitHashPrefixMatch(surfaceHash, rowHash) {
					return true
				}
			}
		}
	}
	return false
}

func principalEnumerationCommitHashesInSurface(surface string) []string {
	var out []string
	if hash := principalEnumerationLeadingCommitHash(surface); hash != "" {
		out = append(out, hash)
	}
	for _, token := range principalEnumerationCodeTokens(surface) {
		if hash := principalEnumerationLeadingCommitHash(token); hash != "" {
			out = append(out, hash)
		}
	}
	return dedupPreEmitStringCandidates(out)
}

func principalEnumerationCommitHashPrefixMatch(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if len(a) < 7 || len(b) < 7 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func principalEnumerationLeadingCommitHash(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`*_[](){}")
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r)
		case r >= 'A' && r <= 'F':
			b.WriteRune(unicode.ToLower(r))
		default:
			goto done
		}
		if b.Len() > 40 {
			return ""
		}
	}
done:
	if b.Len() < 7 {
		return ""
	}
	return b.String()
}

func principalEnumerationCandidateLocationCompatible(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" || row.Location == "" {
		return true
	}
	locations := aggregateToolLocationPattern.FindAllString(surface, -1)
	if len(locations) == 0 {
		return true
	}
	rowLoc := principalEnumerationLocationKey(row.Location)
	rowExact := principalEnumerationLocationKey(fmt.Sprintf("%s:%d", row.Source, row.LineStart))
	for _, loc := range locations {
		candidate := principalEnumerationLocationKey(loc)
		if principalEnumerationLocationKeyMatches(candidate, rowLoc) ||
			principalEnumerationLocationKeyMatches(candidate, rowExact) {
			return true
		}
	}
	return false
}

func principalEnumerationLocationKeyMatches(candidate, rowLocation string) bool {
	if candidate == "" || rowLocation == "" {
		return false
	}
	if candidate == rowLocation {
		return true
	}
	return strings.HasSuffix(rowLocation, "/"+candidate)
}

func principalEnumerationLocationKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`")
	raw = strings.ReplaceAll(raw, "\\", "/")
	for strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	return raw
}

func principalEnumerationPrimaryRowKey(row types.EnumerationDisplayRow) string {
	for _, key := range principalEnumerationRowKeys(row) {
		if key != "" {
			return key
		}
	}
	return ""
}

func principalEnumerationRowKeySet(row types.EnumerationDisplayRow) map[string]bool {
	out := map[string]bool{}
	for _, key := range principalEnumerationRowKeys(row) {
		out[key] = true
	}
	return out
}

func principalEnumerationRowKeys(row types.EnumerationDisplayRow) []string {
	raw := []string{row.DisplayLabel, row.Member}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
		raw = append(raw, label)
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(row.Member); ok {
		raw = append(raw, surface.File)
	}
	var out []string
	seen := map[string]bool{}
	for _, value := range raw {
		key := normalizeEnumerationDisplayTableKey(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func normalizePrincipalEnumerationSectionBlocks(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) int {
	if doc == nil || len(set.Rows) == 0 {
		return 0
	}
	text := fmt.Sprintf("%s共 %d 项；完整成员、定义位置和说明见对应表格。", strings.TrimSpace(set.Label), len(set.Rows))
	title := principalEnumerationBlockTitle(set)
	bestIdx := -1
	bestScore := 0
	for i := range doc.Blocks {
		block := doc.Blocks[i]
		if !principalEnumerationSectionBlockIsGeneratedShell(block, set, title, text) {
			continue
		}
		score := principalEnumerationBlockSetScore(block, set)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return 0
	}
	changed := 0
	block := &doc.Blocks[bestIdx]
	if block.Text != text {
		block.Text = text
		changed++
	}
	if block.Title != title {
		block.Title = title
		changed++
	}
	block.SurfaceRole = types.SurfacePrincipal
	block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
	block.ClaimUses = appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))

	if removed := removeRedundantPrincipalEnumerationSectionBlocks(doc, set, title, text, bestIdx); removed > 0 {
		changed += removed
	}
	return changed
}

func removeRedundantPrincipalEnumerationSectionBlocks(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet, title, text string, keepIdx int) int {
	if doc == nil || keepIdx < 0 || keepIdx >= len(doc.Blocks) {
		return 0
	}
	out := doc.Blocks[:0]
	removed := 0
	for i := range doc.Blocks {
		block := doc.Blocks[i]
		if i == keepIdx {
			out = append(out, block)
			continue
		}
		if principalEnumerationSectionBlockIsRedundant(block, set, title, text) {
			removed++
			continue
		}
		out = append(out, block)
	}
	if removed > 0 {
		doc.Blocks = out
	}
	return removed
}

func principalEnumerationSectionBlockIsGeneratedShell(block types.AnswerBlock, set types.EnumerationDisplaySet, title, text string) bool {
	if block.Kind != types.BlockSection || len(block.Items) > 0 || len(block.Columns) > 0 || block.Diagram != nil {
		return false
	}
	blockTitle := strings.TrimSpace(block.Title)
	blockText := strings.TrimSpace(block.Text)
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if blockTitle != title {
		return false
	}
	if blockText == "" || blockText == text {
		return true
	}
	label := strings.TrimSpace(set.Label)
	return label != "" &&
		strings.Contains(blockText, label) &&
		strings.Contains(blockText, "完整成员、定义位置和说明见对应表格")
}

func principalEnumerationSectionBlockIsRedundant(block types.AnswerBlock, set types.EnumerationDisplaySet, title, text string) bool {
	if !principalEnumerationSectionBlockIsGeneratedShell(block, set, title, text) {
		return false
	}
	blockTitle := strings.TrimSpace(block.Title)
	blockText := strings.TrimSpace(block.Text)
	if blockTitle == strings.TrimSpace(title) && blockText == strings.TrimSpace(text) {
		return true
	}
	if blockText == "" && blockTitle == strings.TrimSpace(title) {
		return true
	}
	return false
}

func principalEnumerationBlockTitle(set types.EnumerationDisplaySet) string {
	label := strings.TrimSpace(set.Label)
	if label == "" {
		label = "成员清单"
	}
	return fmt.Sprintf("%s（%d）", label, len(set.Rows))
}

func appendRenderedClaimUseIfMissing(in []types.RenderedClaimUse, form types.ClaimForm, facet string) []types.RenderedClaimUse {
	for _, use := range in {
		if use.ClaimForm == form && strings.TrimSpace(use.FacetID) == facet {
			return in
		}
	}
	return append(in, types.RenderedClaimUse{ClaimForm: form, FacetID: facet})
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstNonEmptyAnswerString(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}

func sanitizeEnumerationBlockID(raw string) string {
	key := principalEnumerationLabelKey(raw)
	if key == "" {
		return "rows"
	}
	var b strings.Builder
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	return strings.Trim(b.String(), "_")
}

func principalEnumerationLabelKey(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
