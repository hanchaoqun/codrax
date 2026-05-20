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
	if principalEnumerationSystemSupplementSuppressed(doc, ctx) {
		return 0
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
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
		missingRows = principalEnumerationRenderableSupplementRows(missingRows)
		if len(missingRows) > 0 {
			doc.Blocks = append(doc.Blocks, buildPrincipalEnumerationRowsBlock(doc, set, missingRows, zh))
			changed++
		}
		if normalizePrincipalEnumerationSectionBlocks(doc, set) > 0 {
			changed++
		}
	}
	return changed
}

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

func buildPrincipalEnumerationRowsBlock(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet, rows []types.EnumerationDisplayRow, zh bool) types.AnswerBlock {
	blockSet := set
	blockSet.Rows = append([]types.EnumerationDisplayRow(nil), rows...)
	shape := principalEnumerationTableShapeForRows(blockSet.Rows, nil)
	block := types.AnswerBlock{
		ID:          uniqueAnswerBlockID(doc, "principal_enum_"+sanitizeEnumerationBlockID(set.ID)),
		Kind:        types.BlockTable,
		Title:       principalEnumerationRowsBlockTitle(set, rows, zh),
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimDefinitionFact,
			FacetID:   string(types.FacetEnumerationItem),
		}},
		Columns: principalEnumerationTableColumns(zh, shape),
	}
	block.Items = principalEnumerationItemsForSet(doc, blockSet, block.Kind, nil, shape)
	return block
}

func principalEnumerationRenderableSupplementRows(rows []types.EnumerationDisplayRow) []types.EnumerationDisplayRow {
	if len(rows) == 0 {
		return nil
	}
	shape := principalEnumerationTableShapeForRows(rows, nil)
	out := make([]types.EnumerationDisplayRow, 0, len(rows))
	for _, row := range rows {
		if !principalEnumerationRowCompatibleWithTableShape(row, nil, shape) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func principalEnumerationRowsBlockTitle(set types.EnumerationDisplaySet, rows []types.EnumerationDisplayRow, zh bool) string {
	label := strings.TrimSpace(set.Label)
	if label == "" {
		label = "成员清单"
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

func principalEnumerationTableColumns(zh bool, shape principalEnumerationTableShape) []string {
	columns := []string{"Name"}
	if zh {
		columns = []string{"符号名称"}
	}
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
	}
	if principalEnumerationCommitSurfaceMatchesRow(surfaces, row) {
		return true
	}
	return false
}

func principalEnumerationCommitSurfaceMatchesRow(surfaces []string, row types.EnumerationDisplayRow) bool {
	rowHashes := []string{
		principalEnumerationLeadingCommitHash(row.DisplayLabel),
		principalEnumerationLeadingCommitHash(row.Member),
	}
	for _, surface := range surfaces {
		surfaceHash := principalEnumerationLeadingCommitHash(surface)
		if surfaceHash == "" {
			continue
		}
		for _, rowHash := range rowHashes {
			if principalEnumerationCommitHashPrefixMatch(surfaceHash, rowHash) {
				return true
			}
		}
	}
	return false
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
