package tool

import (
	"fmt"
	"regexp"
	"strconv"
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
	if removed := removeRedundantPrincipalEnumerationIncompleteTables(doc, sets); removed > 0 {
		changed += removed
	}
	if normalizedMarkdown := normalizePrincipalEnumerationAuthoritativeMarkdownTables(doc, ctx, sets); normalizedMarkdown > 0 {
		changed += normalizedMarkdown
	}
	if normalizedStructured := normalizePrincipalEnumerationAuthoritativeStructuredCarriers(doc, ctx, sets); normalizedStructured > 0 {
		changed += normalizedStructured
	}
	if pruned := prunePrincipalEnumerationExtraneousItems(doc, sets); pruned > 0 {
		changed += pruned
	}
	if normalizedCounts := normalizePrincipalEnumerationCarrierItemCounts(doc, sets); normalizedCounts > 0 {
		changed += normalizedCounts
	}
	zh := principalEnumerationPrefersZH(ctx)
	missingBySet := make(map[string][]types.EnumerationDisplayRow, len(sets))
	authorityMissingBySet, authorityCoverageActive := principalEnumerationSourceInventoryAuthorityMissingRowsBySet(doc, ctx, sets)
	for _, set := range sets {
		missingRows := principalEnumerationMissingRows(doc, set)
		if authorityCoverageActive {
			missingRows = authorityMissingBySet[set.ID]
		}
		if len(missingRows) > 0 {
			missingBySet[set.ID] = missingRows
		}
	}
	singleSet := len(sets) == 1
	modelAuthoredCarrierExists := principalEnumerationAnyModelAuthoredCarrierExists(doc, ctx, sets, singleSet)
	if normalizedSourceInventorySummary := normalizePrincipalEnumerationSourceInventorySummary(doc, sets, zh); normalizedSourceInventorySummary > 0 {
		changed += normalizedSourceInventorySummary
	}
	if normalizePrincipalEnumerationSummary(doc, ctx, sets, !modelAuthoredCarrierExists) {
		changed++
	}
	for _, set := range sets {
		if len(set.Rows) == 0 {
			continue
		}
		if annotated := annotatePrincipalEnumerationCoveredBlocks(doc, set); annotated > 0 {
			changed += annotated
		}
		authoredCarrier := principalEnumerationModelAuthoredCarrierExists(doc, ctx, set, singleSet)
		missingRows := missingBySet[set.ID]
		supplementRows := missingRows
		supplementMode := principalEnumerationSupplementMissing
		if authoredCarrier && len(missingRows) == 0 {
			supplementRows = nil
		} else if authoredCarrier && principalEnumerationProseCategorySuppressesMissingSupplement(doc, ctx, set) {
			supplementRows = nil
		} else if principalEnumerationNeedsVerifiedFieldSupplement(doc, set) {
			supplementRows = set.Rows
			supplementMode = principalEnumerationSupplementVerifiedFields
		}
		if len(supplementRows) == 0 {
			if !authoredCarrier {
				if noteRows := principalEnumerationRowsNeedingNoteSupplement(doc, ctx, set); len(noteRows) > 0 {
					supplementRows = noteRows
					supplementMode = principalEnumerationSupplementVerifiedNotes
				}
			}
		}
		supplementRows = principalEnumerationRenderableSupplementRows(supplementRows)
		if len(supplementRows) > 0 {
			doc.Blocks = append(doc.Blocks, buildPrincipalEnumerationRowsBlock(doc, set, supplementRows, zh, supplementMode))
			changed++
		}
		if principalEnumerationSetIsSourceInventoryPrincipalRows(set) {
			if normalizedSections := normalizePrincipalEnumerationSourceInventorySectionBlocks(doc, set); normalizedSections > 0 {
				changed += normalizedSections
			} else if normalizePrincipalEnumerationSectionBlocks(doc, set) > 0 {
				changed++
			}
		} else if normalizePrincipalEnumerationSectionBlocks(doc, set) > 0 {
			changed++
		}
	}
	return changed
}

func prunePrincipalEnumerationExtraneousItems(doc *types.AnswerDocumentV2, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	changed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if preEmitSystemEnumerationRowSupplementBlock(*block) || len(block.Items) == 0 || !principalEnumerationBlockCanCarryRows(*block) {
			continue
		}
		rows, strictSourceInventoryRows := principalEnumerationPruneRowsForBlockAtWithMode(doc, bi, sets)
		if len(rows) == 0 {
			continue
		}
		if !principalEnumerationBlockHasEnumerationFacet(*block) &&
			!principalEnumerationBlockCoversAnyDisplayRow(*block, doc, rows) {
			continue
		}
		out := block.Items[:0]
		for _, item := range block.Items {
			keep := principalEnumerationItemCoversAnyRow(item, doc, rows)
			if strictSourceInventoryRows {
				keep = principalEnumerationItemCoversAnySourceInventoryScopedRow(item, doc, rows)
			}
			if keep {
				out = append(out, item)
				continue
			}
			changed++
		}
		block.Items = out
	}
	return changed
}

func principalEnumerationPruneRowsForBlock(block types.AnswerBlock, sets []types.EnumerationDisplaySet) []types.EnumerationDisplayRow {
	rows, _ := principalEnumerationPruneRowsForBlockWithMode(block, sets)
	return rows
}

func principalEnumerationPruneRowsForBlockAtWithMode(doc *types.AnswerDocumentV2, idx int, sets []types.EnumerationDisplaySet) ([]types.EnumerationDisplayRow, bool) {
	if doc == nil || idx < 0 || idx >= len(doc.Blocks) {
		return nil, false
	}
	if rows, strict := principalEnumerationAdjacentSourceInventoryRowsForCarrier(doc, idx, sets); len(rows) > 0 {
		return rows, strict
	}
	return principalEnumerationPruneRowsForBlockWithMode(doc.Blocks[idx], sets)
}

func principalEnumerationPruneRowsForBlockWithMode(block types.AnswerBlock, sets []types.EnumerationDisplaySet) ([]types.EnumerationDisplayRow, bool) {
	if scoped, ok := principalEnumerationBestSourceInventoryScopedSetForBlock(block, sets); ok {
		return scoped.Rows, true
	}
	if set, ok := principalEnumerationUniqueBlockSet(block, sets); ok {
		if scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(block, set); ok {
			return scoped.Rows, true
		}
		return set.Rows, principalEnumerationSetIsSourceInventoryPrincipalRows(set)
	}
	if principalEnumerationBlockHasEnumerationFacet(block) {
		return principalEnumerationAllRows(sets), false
	}
	return nil, false
}

func principalEnumerationItemCoversAnySourceInventoryScopedRow(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if !principalEnumerationItemExactLabelMatchesRow(item, row) &&
			!principalEnumerationItemStronglyIdentifiesRow(item, row) {
			continue
		}
		if item.CitationRef >= 0 {
			if principalEnumerationItemCitationCompatible(item, doc, row) {
				return true
			}
			continue
		}
		surface := strings.Join([]string{item.Label, item.Text, strings.Join(item.Cells, " ")}, " ")
		if principalEnumerationCandidateLocationCompatible(surface, row) {
			return true
		}
	}
	return false
}

func principalEnumerationBlockCoversAnyDisplayRow(block types.AnswerBlock, doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if principalEnumerationBlockCoversRow(block, doc, row) {
			return true
		}
	}
	return false
}

func principalEnumerationUniqueBlockSet(block types.AnswerBlock, sets []types.EnumerationDisplaySet) (types.EnumerationDisplaySet, bool) {
	if len(sets) == 1 && principalEnumerationBlockHasEnumerationFacet(block) {
		return sets[0], true
	}
	bestIdx, bestScore, ties := -1, 0, 0
	for idx, set := range sets {
		score := principalEnumerationBlockSetScore(block, set)
		if score <= bestScore {
			if score == bestScore && score > 0 {
				ties++
			}
			continue
		}
		bestIdx, bestScore, ties = idx, score, 1
	}
	if bestIdx < 0 || ties != 1 {
		return types.EnumerationDisplaySet{}, false
	}
	if bestScore >= 28 {
		return sets[bestIdx], true
	}
	if len(sets) == 1 && bestScore > 0 && principalEnumerationBlockHasEnumerationFacet(block) {
		return sets[bestIdx], true
	}
	return types.EnumerationDisplaySet{}, false
}

func normalizePrincipalEnumerationCarrierItemCounts(doc *types.AnswerDocumentV2, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	changed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if len(block.Items) == 0 || !principalEnumerationBlockCanCarryRows(*block) {
			continue
		}
		rows, _ := principalEnumerationPruneRowsForBlockAtWithMode(doc, bi, sets)
		if len(rows) == 0 {
			continue
		}
		if !principalEnumerationBlockHasEnumerationFacet(*block) &&
			!principalEnumerationBlockCoversAnyDisplayRow(*block, doc, rows) {
			continue
		}
		expected := len(block.Items)
		if expected <= 0 || !principalEnumerationTextHasMismatchedLocalCount(block.Text, expected) {
			continue
		}
		block.Text = principalEnumerationCarrierCountText(*block, expected)
		changed++
	}
	return changed
}

func normalizePrincipalEnumerationAuthoritativeMarkdownTables(doc *types.AnswerDocumentV2, ctx *types.BusContext, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	changed := 0
	zh := principalEnumerationPrefersZH(ctx)
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) == "" {
			continue
		}
		set, ok := principalEnumerationUniqueBlockSet(*block, sets)
		if !ok || !principalEnumerationSetIsSourceInventoryPrincipalRows(set) {
			continue
		}
		scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(*block, set)
		if !ok || len(scoped.Rows) == 0 {
			continue
		}
		stats := principalEnumerationAuthoredMarkdownTableStats(*block, scoped)
		if stats.dataRows == 0 || stats.missingRows > 0 {
			continue
		}
		shape := principalEnumerationAuthoritativeMarkdownTableShape(*block, scoped.Rows)
		block.Text = ""
		block.Columns = principalEnumerationTableColumns(zh, shape, scoped.Rows)
		block.Items = principalEnumerationItemsForSet(doc, scoped, block.Kind, nil, shape)
		block.SurfaceRole = types.SurfacePrincipal
		block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
		block.ClaimUses = appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
		changed++
	}
	return changed
}

func normalizePrincipalEnumerationAuthoritativeStructuredCarriers(doc *types.AnswerDocumentV2, ctx *types.BusContext, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	changed := 0
	zh := principalEnumerationPrefersZH(ctx)
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if preEmitSystemEnumerationRowSupplementBlock(*block) ||
			!principalEnumerationBlockCanCarryRows(*block) ||
			(block.Kind == types.BlockTable && strings.TrimSpace(block.Text) != "") {
			continue
		}
		scoped, ok := principalEnumerationSourceInventoryStructuredCarrierSetForBlock(*block, sets)
		if !ok || len(scoped.Rows) == 0 || !principalEnumerationSetIsSourceInventoryPrincipalRows(scoped) {
			continue
		}
		emptySectionCarrier := principalEnumerationEmptySourceInventorySectionCarrier(doc, bi, scoped)
		if len(block.Items) == 0 && !emptySectionCarrier {
			continue
		}
		if !emptySectionCarrier &&
			!principalEnumerationBlockHasEnumerationFacet(*block) &&
			!principalEnumerationBlockCoversAnyDisplayRow(*block, doc, scoped.Rows) {
			continue
		}
		if !emptySectionCarrier && !principalEnumerationCarrierTouchesAnyRow(*block, doc, scoped.Rows) {
			continue
		}
		shape := principalEnumerationTableShapeForSet(scoped, nil)
		if block.Kind == types.BlockTable {
			block.Columns = principalEnumerationTableColumns(zh, shape, scoped.Rows)
			block.Items = principalEnumerationItemsForSet(doc, scoped, block.Kind, nil, shape)
		} else {
			block.Items = principalEnumerationStructuredSourceInventoryItemsForSet(doc, scoped)
		}
		block.SurfaceRole = types.SurfacePrincipal
		block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
		block.ClaimUses = appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
		if block.Kind == types.BlockSection || block.Kind == types.BlockOrderedList || block.Kind == types.BlockBulletList {
			block.Text = principalEnumerationCarrierCountText(*block, len(scoped.Rows))
		}
		changed++
	}
	return changed
}

func principalEnumerationSourceInventoryStructuredCarrierSetForBlock(block types.AnswerBlock, sets []types.EnumerationDisplaySet) (types.EnumerationDisplaySet, bool) {
	if scoped, ok := principalEnumerationBestSourceInventoryScopedSetForBlock(block, sets); ok {
		return scoped, true
	}
	set, ok := principalEnumerationUniqueBlockSet(block, sets)
	if !ok || !principalEnumerationSetIsSourceInventoryPrincipalRows(set) || len(set.Rows) == 0 {
		return types.EnumerationDisplaySet{}, false
	}
	if !principalEnumerationBlockHasEnumerationFacet(block) &&
		principalEnumerationBlockSetScore(block, set) <= 0 {
		return types.EnumerationDisplaySet{}, false
	}
	return set, true
}

func principalEnumerationEmptySourceInventorySectionCarrier(doc *types.AnswerDocumentV2, idx int, scoped types.EnumerationDisplaySet) bool {
	if doc == nil || idx < 0 || idx >= len(doc.Blocks) || len(scoped.Rows) == 0 {
		return false
	}
	block := doc.Blocks[idx]
	if block.Kind != types.BlockSection ||
		len(block.Items) > 0 ||
		len(block.Columns) > 0 ||
		block.Diagram != nil ||
		strings.TrimSpace(block.Title) == "" {
		return false
	}
	return !principalEnumerationSectionHasAdjacentCarrier(doc, idx, scoped)
}

func principalEnumerationStructuredSourceInventoryItemsForSet(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) []types.AnswerBlockItem {
	shape := principalEnumerationTableShapeForSet(set, nil)
	items := make([]types.AnswerBlockItem, 0, len(set.Rows))
	for _, row := range set.Rows {
		label := firstNonEmptyAnswerString(row.DisplayLabel, row.Member)
		note := principalEnumerationRowNote(row, nil)
		citationRef := -1
		if row.HasCitation && strings.TrimSpace(row.Source) != "" && row.LineStart > 0 {
			citationRef = appendOrReusePreEmitCitation(doc, types.Citation{File: row.Source, Line: row.LineStart})
		}
		item := types.AnswerBlockItem{
			ID:          firstNonEmptyAnswerString(row.RowID, uniqueAnswerBlockID(doc, "enum_item")),
			Label:       label,
			Text:        principalEnumerationStructuredSourceInventoryItemText(row, note, shape),
			CitationRef: citationRef,
		}
		items = append(items, item)
	}
	return items
}

func principalEnumerationStructuredSourceInventoryItemText(row types.EnumerationDisplayRow, note string, shape principalEnumerationTableShape) string {
	parts := []string{}
	if surface := principalEnumerationCleanSourceInventoryDisplaySurface(row, principalEnumerationPreferredSourceInventorySurface(row)); surface != "" &&
		!preEmitAggregateScalarValueAppears(surface, row.DisplayLabel) {
		parts = append(parts, surface)
	}
	if shape.includeLocation && strings.TrimSpace(row.Location) != "" {
		parts = append(parts, strings.TrimSpace(row.Location))
	}
	if shape.includePackage {
		if pkg := strings.TrimSpace(enumerationDisplayPackageCell(row)); pkg != "" {
			parts = append(parts, "package="+pkg)
		}
	}
	if note = principalEnumerationCleanSourceInventoryDisplayNote(row, note, parts); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(dedupPreEmitStringCandidates(parts), "；")
}

func principalEnumerationPreferredSourceInventorySurface(row types.EnumerationDisplayRow) string {
	return strings.Join(principalEnumerationPreferredSourceInventorySurfaces(row), ", ")
}

func principalEnumerationPreferredSourceInventorySurfaces(row types.EnumerationDisplayRow) []string {
	generic := principalEnumerationGenericRowSurfaceTermKeys(row)
	var out []string
	for _, term := range row.SurfaceTerms {
		term = strings.TrimSpace(term)
		if term == "" || generic[normalizeEnumerationDisplayTableKey(term)] {
			continue
		}
		if strings.Contains(term, "/") || aggregateToolLocationPattern.MatchString(term) {
			continue
		}
		out = append(out, term)
	}
	return dedupPreEmitStringCandidates(out)
}

func principalEnumerationRowsHavePreferredSourceInventorySurface(rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if len(principalEnumerationPreferredSourceInventorySurfaces(row)) > 0 {
			return true
		}
	}
	return false
}

func principalEnumerationCleanSourceInventoryDisplaySurface(row types.EnumerationDisplayRow, surface string) string {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return ""
	}
	surface = principalEnumerationCollapseAdjacentRepeatedWordGroups(surface)
	label := strings.TrimSpace(firstNonEmptyAnswerString(row.DisplayLabel, row.Member))
	if label == "" {
		return surface
	}
	labelKey := normalizeEnumerationDisplayTableKey(label)
	if labelKey == "" {
		return surface
	}
	parts := strings.Fields(surface)
	if len(parts) == 0 {
		return surface
	}
	labelParts := strings.Fields(label)
	if len(labelParts) == 0 {
		return surface
	}
	last := -1
	for i := 0; i+len(labelParts) <= len(parts); i++ {
		if normalizeEnumerationDisplayTableKey(strings.Join(parts[i:i+len(labelParts)], " ")) == labelKey {
			last = i
		}
	}
	if last <= 0 {
		return surface
	}
	// If a generated surface repeats the same row label through multiple
	// aliases, keep the nearest alias ending at the last label. This is display
	// polish only; row identity and citation authority remain unchanged.
	start := last
	for start > 0 && last-start < 4 {
		prev := strings.ToLower(parts[start-1])
		if normalizeEnumerationDisplayTableKey(prev) == labelKey {
			break
		}
		if strings.Contains(prev, "/") || strings.Contains(prev, ":") {
			break
		}
		start--
	}
	out := strings.Join(parts[start:last+len(labelParts)], " ")
	outParts := strings.Fields(out)
	if len(outParts) > len(labelParts) &&
		normalizeEnumerationDisplayTableKey(strings.Join(outParts[:len(labelParts)], " ")) == labelKey {
		labelOccurrences := 0
		for i := 0; i+len(labelParts) <= len(outParts); i++ {
			if normalizeEnumerationDisplayTableKey(strings.Join(outParts[i:i+len(labelParts)], " ")) == labelKey {
				labelOccurrences++
			}
		}
		if labelOccurrences > 1 {
			out = strings.Join(outParts[len(labelParts):], " ")
		}
	}
	return out
}

func principalEnumerationCollapseAdjacentRepeatedWordGroups(surface string) string {
	parts := strings.Fields(strings.TrimSpace(surface))
	if len(parts) < 2 {
		return strings.TrimSpace(surface)
	}
	for {
		changed := false
		for width := len(parts) / 2; width >= 1; width-- {
			for i := 0; i+2*width <= len(parts); i++ {
				if !principalEnumerationWordGroupEqual(parts[i:i+width], parts[i+width:i+2*width]) {
					continue
				}
				parts = append(append([]string{}, parts[:i+width]...), parts[i+2*width:]...)
				changed = true
				break
			}
			if changed {
				break
			}
		}
		if !changed {
			break
		}
	}
	return strings.Join(parts, " ")
}

func principalEnumerationWordGroupEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if normalizeEnumerationDisplayTableKey(a[i]) != normalizeEnumerationDisplayTableKey(b[i]) {
			return false
		}
	}
	return true
}

func principalEnumerationCleanSourceInventoryDisplayNote(row types.EnumerationDisplayRow, note string, visibleParts []string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	segments := splitPrincipalEnumerationDisplayNoteSegments(note)
	if len(segments) == 0 {
		segments = []string{note}
	}
	pkg := strings.TrimSpace(enumerationDisplayPackageCell(row))
	location := strings.TrimSpace(firstNonEmptyAnswerString(row.Location, row.Source))
	rawSurface := principalEnumerationPreferredSourceInventorySurface(row)
	surface := principalEnumerationCleanSourceInventoryDisplaySurface(row, rawSurface)
	typedValues := []string{
		pkg,
		location,
		rawSurface,
		surface,
		row.DisplayLabel,
		row.Member,
	}
	typedValues = append(typedValues, visibleParts...)
	var kept []string
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if principalEnumerationSourceInventoryDisplayNoteSegmentIsRedundantTypedValue(segment, typedValues) {
			continue
		}
		kept = append(kept, segment)
	}
	return strings.Join(dedupPreEmitStringCandidates(kept), "，")
}

func splitPrincipalEnumerationDisplayNoteSegments(note string) []string {
	return strings.FieldsFunc(note, func(r rune) bool {
		switch r {
		case '；', ';', '，', ',':
			return true
		default:
			return false
		}
	})
}

func principalEnumerationSourceInventoryDisplayNoteSegmentIsRedundantTypedValue(segment string, typedValues []string) bool {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return true
	}
	if principalEnumerationDisplayNoteValueMatchesAny(segment, typedValues) {
		return true
	}
	if value, ok := principalEnumerationDisplayNoteStructuredPayload(segment); ok {
		if principalEnumerationDisplayNoteValueMatchesAny(value, typedValues) {
			return true
		}
	}
	return false
}

func principalEnumerationDisplayNoteStructuredPayload(segment string) (string, bool) {
	key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
	if !ok || !principalEnumerationDisplayNoteSchemaKey(key) {
		return "", false
	}
	value = strings.TrimSpace(value)
	if before, _, ok := strings.Cut(value, " @ "); ok {
		value = strings.TrimSpace(before)
	}
	return value, true
}

func principalEnumerationDisplayNoteSchemaKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 48 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func principalEnumerationDisplayNoteValueMatchesAny(value string, typedValues []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	valueKey := normalizeEnumerationDisplayTableKey(value)
	if valueKey == "" {
		return false
	}
	for _, typedValue := range typedValues {
		typedValue = strings.TrimSpace(typedValue)
		if typedValue == "" {
			continue
		}
		typedKey := normalizeEnumerationDisplayTableKey(typedValue)
		if valueKey == typedKey || principalEnumerationDisplayNoteLocationValueMatches(value, typedValue) {
			return true
		}
	}
	return false
}

func principalEnumerationDisplayNoteLocationValueMatches(value, typedValue string) bool {
	if !aggregateToolLocationPattern.MatchString(value) && !aggregateToolLocationPattern.MatchString(typedValue) {
		return false
	}
	return principalEnumerationSurfaceContainsLocation(value, typedValue)
}

func principalEnumerationAuthoritativeMarkdownTableShape(block types.AnswerBlock, rows []types.EnumerationDisplayRow) principalEnumerationTableShape {
	shape := principalEnumerationTableShapeForRows(rows, nil)
	// Markdown table text is model-authored presentation. When the typed
	// source-inventory row authority lets us safely rebuild the table, preserve
	// the requested mechanical columns (name/location/package/module). Notes are
	// carried over only when the existing table's data rows already contain
	// typed-row residual description content; column headings are model-authored
	// display text and must not decide answer-shape behaviour.
	shape.includeNote = shape.includeNote && principalEnumerationMarkdownTableCarriesTypedNoteContent(block, rows)
	return shape
}

func principalEnumerationMarkdownTableCarriesTypedNoteContent(block types.AnswerBlock, rows []types.EnumerationDisplayRow) bool {
	if len(rows) == 0 || strings.TrimSpace(block.Text) == "" {
		return false
	}
	for _, cells := range principalEnumerationMarkdownTableRows(block.Text) {
		for _, row := range rows {
			if strings.TrimSpace(row.Note) == "" ||
				!principalEnumerationMarkdownRowCoversRow(cells, row) {
				continue
			}
			if principalEnumerationMarkdownRowHasAuthoredDescription(cells, row) {
				return true
			}
		}
	}
	return false
}

func principalEnumerationTextHasMismatchedLocalCount(text string, expected int) bool {
	text = strings.TrimSpace(text)
	if text == "" || expected <= 0 {
		return false
	}
	values := preEmitCountLikeIntegers(text)
	if len(values) == 0 {
		return false
	}
	if len(values) == 1 {
		return values[0] != expected
	}
	for _, value := range values {
		if value == expected {
			return false
		}
	}
	return true
}

func principalEnumerationCarrierCountText(block types.AnswerBlock, expected int) string {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = "成员"
	}
	if containsHan(title) || containsHan(block.Text) {
		return fmt.Sprintf("%s共 %d 项：", title, expected)
	}
	return fmt.Sprintf("%s has %d item(s):", title, expected)
}

func normalizePrincipalEnumerationSourceInventorySummary(doc *types.AnswerDocumentV2, sets []types.EnumerationDisplaySet, zh bool) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	var sourceSet types.EnumerationDisplaySet
	for _, set := range sets {
		if principalEnumerationSetIsSourceInventoryPrincipalRows(set) {
			sourceSet = set
			break
		}
	}
	if len(sourceSet.Rows) == 0 {
		return 0
	}
	entries := principalEnumerationSourceInventoryScopedSummaryEntries(doc, sourceSet)
	if len(entries) < 2 {
		return 0
	}
	text := principalEnumerationSourceInventoryScopedSummaryText(entries, zh)
	if text == "" {
		return 0
	}
	idx := -1
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind != types.BlockSummary {
			continue
		}
		idx = i
		break
	}
	if idx < 0 {
		return 0
	}
	if strings.TrimSpace(doc.Blocks[idx].Text) == text {
		return 0
	}
	doc.Blocks[idx].Text = text
	doc.Blocks[idx].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[idx].FacetIDs = mergeStringSet(doc.Blocks[idx].FacetIDs, []string{string(types.FacetEnumerationItem)})
	doc.Blocks[idx].ClaimUses = appendRenderedClaimUseIfMissing(doc.Blocks[idx].ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
	return 1
}

type principalEnumerationSourceInventorySummaryEntry struct {
	label string
	count int
}

func principalEnumerationSourceInventoryScopedSummaryEntries(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) []principalEnumerationSourceInventorySummaryEntry {
	seen := map[string]bool{}
	var out []principalEnumerationSourceInventorySummaryEntry
	for _, block := range doc.Blocks {
		scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(block, set)
		if !ok || len(scoped.Rows) == 0 {
			continue
		}
		label := strings.TrimSpace(scoped.Label)
		key := principalEnumerationLabelKey(label)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, principalEnumerationSourceInventorySummaryEntry{label: label, count: len(scoped.Rows)})
	}
	return out
}

func principalEnumerationSourceInventoryScopedSummaryText(entries []principalEnumerationSourceInventorySummaryEntry, zh bool) string {
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := strings.TrimSpace(entry.label)
		if label == "" || entry.count <= 0 {
			continue
		}
		if zh {
			parts = append(parts, fmt.Sprintf("%s %d 项", label, entry.count))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %d item(s)", label, entry.count))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if zh {
		return "本答案按已验证 source_inventory 行集列出：" + strings.Join(parts, "、") + "。"
	}
	return "This answer lists the verified source_inventory row sets: " + strings.Join(parts, ", ") + "."
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func principalEnumerationItemCoversAnyRow(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if principalEnumerationItemWeaklyIdentifiesRow(item, row) ||
			principalEnumerationStructuredItemCoversRow(item, doc, row) {
			return true
		}
	}
	return false
}

func removeRedundantPrincipalEnumerationIncompleteTables(doc *types.AnswerDocumentV2, sets []types.EnumerationDisplaySet) int {
	if doc == nil || len(sets) == 0 {
		return 0
	}
	index := enumerationDisplayRowIndex(sets)
	if len(index) == 0 {
		return 0
	}
	locationIndex := enumerationDisplayRowLocationIndex(sets)
	out := doc.Blocks[:0]
	removed := 0
	for i, block := range doc.Blocks {
		if principalEnumerationIncompleteTableCoveredOutsideBlock(doc, i, block) {
			removed++
			continue
		}
		rows, ok := enumerationDisplayRowsForIncompleteTable(block, doc.Citations, index, locationIndex)
		if ok && principalEnumerationTableRowsAreShellOnly(block) && principalEnumerationRowsCoveredOutsideBlock(doc, i, rows) {
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

func principalEnumerationTableRowsAreShellOnly(block types.AnswerBlock) bool {
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Items) == 0 {
		return false
	}
	for _, item := range block.Items {
		if strings.TrimSpace(item.Text) != "" || len(nonEmptyAnswerTableCompileCells(item.Cells)) > 0 {
			return false
		}
	}
	return true
}

func principalEnumerationIncompleteTableCoveredOutsideBlock(doc *types.AnswerDocumentV2, skip int, block types.AnswerBlock) bool {
	if doc == nil || block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Items) == 0 {
		return false
	}
	for _, item := range block.Items {
		if strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Text) != "" || len(nonEmptyAnswerTableCompileCells(item.Cells)) > 0 {
			return false
		}
		if !principalEnumerationIncompleteTableItemCoveredOutsideBlock(doc, skip, item) {
			return false
		}
	}
	return true
}

func principalEnumerationIncompleteTableItemCoveredOutsideBlock(doc *types.AnswerDocumentV2, skip int, item types.AnswerBlockItem) bool {
	label := strings.TrimSpace(item.Label)
	for i, block := range doc.Blocks {
		if i == skip {
			continue
		}
		surface := types.AnswerBlockVisibleSurface(block)
		if !principalSupportSurfaceTermAppears(label, surface) {
			continue
		}
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
			return true
		}
		cit := doc.Citations[item.CitationRef]
		if principalSupportCitationSurfaceVisible(cit, surface) || strings.Contains(surface, strings.TrimSpace(cit.File)) {
			return true
		}
	}
	return false
}

func principalEnumerationRowsCoveredOutsideBlock(doc *types.AnswerDocumentV2, skip int, rows []types.EnumerationDisplayRow) bool {
	if doc == nil || len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		covered := false
		for i, block := range doc.Blocks {
			if i == skip {
				continue
			}
			if principalEnumerationVisibleSurfaceCoversRow(types.AnswerBlockVisibleSurface(block), row) ||
				principalEnumerationBlockCoversRow(block, doc, row) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
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
	rows := principalEnumerationAllRows(sets)
	if len(index) == 0 && len(rows) == 0 {
		return 0
	}
	changed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !principalEnumerationBlockCanCarryRows(*block) {
			continue
		}
		if block.Kind == types.BlockTable && strings.TrimSpace(block.Text) != "" {
			continue
		}
		blockRows := rows
		blockIndex := index
		if scopedRows, _ := principalEnumerationPruneRowsForBlockAtWithMode(doc, bi, sets); len(scopedRows) > 0 {
			blockRows = scopedRows
			blockIndex = principalEnumerationScopedExactLabelRowIndex([]types.EnumerationDisplaySet{{
				ID:    "scoped-block",
				Label: strings.TrimSpace(block.Title),
				Rows:  scopedRows,
			}})
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			row, ok := principalEnumerationUniqueItemRow(*item, blockRows, blockIndex)
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

func principalEnumerationAllRows(sets []types.EnumerationDisplaySet) []types.EnumerationDisplayRow {
	var rows []types.EnumerationDisplayRow
	for _, set := range sets {
		rows = append(rows, set.Rows...)
	}
	return rows
}

func principalEnumerationUniqueItemRow(
	item types.AnswerBlockItem,
	rows []types.EnumerationDisplayRow,
	exactIndex map[string]types.EnumerationDisplayRow,
) (types.EnumerationDisplayRow, bool) {
	if row, ok := exactIndex[normalizeEnumerationDisplayTableKey(item.Label)]; ok {
		return row, true
	}
	bestStrength := 0
	var best []types.EnumerationDisplayRow
	seenBest := map[string]bool{}
	for _, row := range rows {
		if !principalEnumerationItemWeaklyIdentifiesRow(item, row) {
			continue
		}
		strength := principalEnumerationItemRowMatchStrength(item, row)
		if strength <= 0 {
			continue
		}
		if strength < bestStrength {
			continue
		}
		if strength > bestStrength {
			bestStrength = strength
			best = best[:0]
			seenBest = map[string]bool{}
		}
		key := principalEnumerationRowIdentityKey(row)
		if seenBest[key] {
			continue
		}
		seenBest[key] = true
		best = append(best, row)
	}
	if len(best) != 1 {
		return types.EnumerationDisplayRow{}, false
	}
	return best[0], true
}

func principalEnumerationItemWeaklyIdentifiesRow(item types.AnswerBlockItem, row types.EnumerationDisplayRow) bool {
	surface := types.AnswerBlockItemVisibleSurface(item)
	if principalEnumerationRowRequiresExactLocationIdentity(row) &&
		principalEnumerationSurfaceHasExplicitLocation(surface) &&
		!principalEnumerationItemSurfaceHasExactRowLocation(surface, row) {
		return false
	}
	if principalEnumerationAnySurfaceMatchesRow([]string{strings.TrimSpace(item.Label), surface}, row) {
		return true
	}
	return principalEnumerationItemSurfaceHasRowLocation(surface, row)
}

func principalEnumerationSurfaceHasExplicitLocation(surface string) bool {
	return len(aggregateToolLocationPattern.FindAllString(strings.TrimSpace(surface), -1)) > 0
}

func principalEnumerationRowRequiresExactLocationIdentity(row types.EnumerationDisplayRow) bool {
	return strings.TrimSpace(row.SetLabel) == "source inventory principal rows" &&
		strings.TrimSpace(row.Source) != "" &&
		row.LineStart > 0
}

func principalEnumerationItemStronglyIdentifiesRow(item types.AnswerBlockItem, row types.EnumerationDisplayRow) bool {
	return principalEnumerationItemRowMatchStrength(item, row) >= 2
}

func principalEnumerationItemRowMatchStrength(item types.AnswerBlockItem, row types.EnumerationDisplayRow) int {
	surface := types.AnswerBlockItemVisibleSurface(item)
	if principalEnumerationItemSurfaceHasExactRowLocation(surface, row) {
		return 4
	}
	if principalEnumerationItemSurfaceHasTypedSurfaceTerm(surface, row) {
		return 3
	}
	if principalEnumerationItemSurfaceHasRowAttribute(surface, row) {
		if principalEnumerationItemExactLabelMatchesRow(item, row) {
			return 3
		}
		return 0
	}
	if principalEnumerationItemSurfaceHasRowLocation(surface, row) {
		return 2
	}
	if principalEnumerationItemExactLabelMatchesRow(item, row) {
		return 1
	}
	return 0
}

func principalEnumerationItemSurfaceHasRowLocation(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" || strings.TrimSpace(row.Source) == "" {
		return false
	}
	for _, loc := range aggregateToolLocationPattern.FindAllString(surface, -1) {
		candidate := principalEnumerationLocationKey(loc)
		if principalEnumerationLocationKeyMatches(candidate, principalEnumerationLocationKey(row.Location)) ||
			principalEnumerationLocationKeyMatches(candidate, principalEnumerationLocationKey(fmt.Sprintf("%s:%d", row.Source, row.LineStart))) {
			return true
		}
	}
	if strings.TrimSpace(row.Location) != "" && principalEnumerationSurfaceContainsLocation(surface, row.Location) {
		return true
	}
	return principalEnumerationSurfaceContainsLocation(surface, row.Source)
}

func principalEnumerationItemSurfaceHasExactRowLocation(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
		return false
	}
	for _, loc := range aggregateToolLocationPattern.FindAllString(surface, -1) {
		candidate := principalEnumerationLocationKey(loc)
		if principalEnumerationLocationKeyMatches(candidate, principalEnumerationLocationKey(row.Location)) ||
			principalEnumerationLocationKeyMatches(candidate, principalEnumerationLocationKey(fmt.Sprintf("%s:%d", row.Source, row.LineStart))) {
			return true
		}
	}
	return strings.TrimSpace(row.Location) != "" && principalEnumerationSurfaceContainsLocation(surface, row.Location)
}

func principalEnumerationSurfaceContainsLocation(surface, location string) bool {
	surfaceKey := principalEnumerationLocationKey(surface)
	locationKey := principalEnumerationLocationKey(location)
	if surfaceKey == "" || locationKey == "" {
		return false
	}
	return strings.Contains(surfaceKey, locationKey)
}

func principalEnumerationItemSurfaceHasRowAttribute(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	for _, attr := range row.Attributes {
		for _, candidate := range []string{attr.Name, attr.Location, answerTableCompileLocationKey(attr.Source, attr.Line)} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if preEmitAggregateScalarValueAppears(candidate, surface) || types.CodeSurfaceAppearsAsToken(candidate, surface) {
				return true
			}
		}
	}
	return false
}

func principalEnumerationItemSurfaceHasTypedSurfaceTerm(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" || len(row.SurfaceTerms) == 0 {
		return false
	}
	generic := principalEnumerationGenericRowSurfaceTermKeys(row)
	for _, term := range row.SurfaceTerms {
		term = strings.TrimSpace(term)
		if term == "" || generic[normalizeEnumerationDisplayTableKey(term)] {
			continue
		}
		if preEmitAggregateScalarValueAppears(term, surface) ||
			types.CodeSurfaceAppearsAsToken(term, surface) {
			return true
		}
	}
	return false
}

func principalEnumerationGenericRowSurfaceTermKeys(row types.EnumerationDisplayRow) map[string]bool {
	out := map[string]bool{}
	for _, raw := range []string{row.DisplayLabel, row.Subject, row.AnchorSymbol, row.Member} {
		key := normalizeEnumerationDisplayTableKey(raw)
		if key != "" {
			out[key] = true
		}
	}
	if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
		if key := normalizeEnumerationDisplayTableKey(label); key != "" {
			out[key] = true
		}
	}
	for _, term := range row.SurfaceTerms {
		termKey := normalizeEnumerationDisplayTableKey(term)
		if termKey == "" {
			continue
		}
		for _, other := range row.SurfaceTerms {
			otherKey := normalizeEnumerationDisplayTableKey(other)
			if otherKey == "" || otherKey == termKey {
				continue
			}
			if strings.Contains(otherKey, termKey) {
				out[termKey] = true
				break
			}
		}
	}
	return out
}

func principalEnumerationRowIdentityKey(row types.EnumerationDisplayRow) string {
	if strings.TrimSpace(row.RowID) != "" {
		return row.RowID
	}
	return strings.Join([]string{
		row.SetID,
		row.DisplayLabel,
		row.Member,
		row.Source,
		strconv.Itoa(row.LineStart),
	}, "\x00")
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

func principalEnumerationScopedExactLabelRowIndex(sets []types.EnumerationDisplaySet) map[string]types.EnumerationDisplayRow {
	candidates := principalEnumerationExactLabelRowIndex(sets)
	ambiguous := map[string]bool{}
	for key := range candidates {
		ambiguous[key] = false
	}
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
			for _, key := range principalEnumerationRowKeys(row) {
				add(key, row)
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

func normalizePrincipalEnumerationSummary(doc *types.AnswerDocumentV2, ctx *types.BusContext, sets []types.EnumerationDisplaySet, allowSystemText bool) bool {
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
		if allowSystemText && strings.TrimSpace(doc.Blocks[i].Text) == "" {
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
	if !allowSystemText {
		return false
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

func principalEnumerationAnyModelAuthoredCarrierExists(doc *types.AnswerDocumentV2, ctx *types.BusContext, sets []types.EnumerationDisplaySet, singleSet bool) bool {
	for _, set := range sets {
		if principalEnumerationModelAuthoredCarrierExists(doc, ctx, set, singleSet) {
			return true
		}
	}
	return false
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

func principalEnumerationSourceInventoryAuthorityMissingRowsBySet(
	doc *types.AnswerDocumentV2,
	ctx *types.BusContext,
	sets []types.EnumerationDisplaySet,
) (map[string][]types.EnumerationDisplayRow, bool) {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil || len(sets) == 0 {
		return nil, false
	}
	if !types.SourceInventoryPrincipalNavigationActive(ctx.AnalysisIR.RequestModel) {
		return nil, false
	}
	hasSourceInventoryPrincipalSet := false
	for _, set := range sets {
		if principalEnumerationSetIsSourceInventoryPrincipalRows(set) {
			hasSourceInventoryPrincipalSet = true
			break
		}
	}
	if !hasSourceInventoryPrincipalSet {
		return nil, false
	}
	auth := BuildSourceInventoryAnswerPreEmitAuthority(ctx, preEmitStableAggregateFacts(ctx), doc)
	if auth.EnumerationCoverage.RowCount == 0 {
		return nil, false
	}
	missing := make(map[string][]types.EnumerationDisplayRow, len(sets))
	for _, row := range auth.EnumerationCoverage.MissingRows {
		if principalEnumerationDocumentCoversRow(doc, row) {
			continue
		}
		setID := strings.TrimSpace(row.SetID)
		if setID == "" {
			continue
		}
		missing[setID] = append(missing[setID], row)
	}
	return missing, true
}

func principalEnumerationSetIsSourceInventoryPrincipalRows(set types.EnumerationDisplaySet) bool {
	if strings.TrimSpace(set.Label) == "source inventory principal rows" {
		return true
	}
	for _, row := range set.Rows {
		if strings.TrimSpace(row.SetLabel) == "source inventory principal rows" {
			return true
		}
	}
	return false
}

func principalEnumerationDocumentCoversRow(doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if doc == nil {
		return false
	}
	if types.AnswerDocumentCoversEnumerationDisplayRow(doc, row) {
		return true
	}
	if principalEnumerationRuntimeArtifactRowCoveredByAnyVisibleText(doc, row) {
		return true
	}
	if !principalEnumerationRowRequiresStructuredCarrier(row) && principalEnumerationRowCoveredByAnyVisibleText(doc, row) {
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

func principalEnumerationRowRequiresStructuredCarrier(row types.EnumerationDisplayRow) bool {
	return !types.AnswerEvidenceOriginsAreOriginSpecificOnly(row.EvidenceOrigins)
}

func principalEnumerationModelAuthoredCarrierExists(doc *types.AnswerDocumentV2, ctx *types.BusContext, set types.EnumerationDisplaySet, singleSet bool) bool {
	if doc == nil || len(set.Rows) == 0 {
		return false
	}
	if types.AnswerDocumentCoversEnumerationDisplaySet(doc, set) {
		return true
	}
	for _, block := range doc.Blocks {
		if preEmitSystemEnumerationRowSupplementBlock(block) {
			continue
		}
		surface := strings.TrimSpace(types.AnswerBlockVisibleSurface(block))
		if surface == "" && len(block.Items) == 0 {
			continue
		}
		if !principalEnumerationBlockCanCarryRows(block) {
			if principalEnumerationProseCategoryCarrierSuppressesSupplement(ctx, block, set) {
				return true
			}
			continue
		}
		if principalEnumerationBlockHasEnumerationFacet(block) {
			return true
		}
		if principalEnumerationBlockSetScore(block, set) > 0 {
			return true
		}
		if principalEnumerationBlockCoversAnyRow(block, doc, set) {
			return true
		}
		if singleSet && block.SurfaceRole == types.SurfacePrincipal {
			return true
		}
		if singleSet && surface != "" {
			return true
		}
	}
	return false
}

func principalEnumerationProseCategorySuppressesMissingSupplement(doc *types.AnswerDocumentV2, ctx *types.BusContext, set types.EnumerationDisplaySet) bool {
	if doc == nil || len(set.Rows) == 0 {
		return false
	}
	for _, block := range doc.Blocks {
		if principalEnumerationProseCategoryCarrierSuppressesSupplement(ctx, block, set) {
			return true
		}
	}
	return false
}

func principalEnumerationProseCategoryCarrierSuppressesSupplement(ctx *types.BusContext, block types.AnswerBlock, set types.EnumerationDisplaySet) bool {
	rm := principalEnumerationEffectiveRequestModel(ctx)
	if rm == nil || rm.Intent != types.IntentExplain || rm.Scenario != types.ScenarioArchitectureExplain {
		return false
	}
	switch block.Kind {
	case types.BlockSummary, types.BlockSection:
	default:
		return false
	}
	return principalEnumerationBlockSetScore(block, set) >= 28
}

func principalEnumerationBlockHasEnumerationFacet(block types.AnswerBlock) bool {
	for _, facet := range block.FacetIDs {
		if strings.TrimSpace(facet) == string(types.FacetEnumerationItem) {
			return true
		}
	}
	for _, claim := range block.ClaimUses {
		if strings.TrimSpace(claim.FacetID) == string(types.FacetEnumerationItem) {
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
	if len(cells) == 0 {
		return false
	}
	joined := strings.Join(cells, " ")
	surfaces := append(append([]string{}, cells...), joined)
	relationPartsCovered := principalEnumerationRelationPartsCoveredByCells(cells, row)
	if !principalEnumerationAnySurfaceMatchesRow(surfaces, row) && !relationPartsCovered {
		return false
	}
	if principalEnumerationCandidateLocationCompatible(joined, row) {
		return true
	}
	return relationPartsCovered && principalEnumerationMarkdownRowFileCompatible(joined, row)
}

func principalEnumerationRelationPartsCoveredByCells(cells []string, row types.EnumerationDisplayRow) bool {
	if len(cells) == 0 {
		return false
	}
	for _, candidate := range principalEnumerationRelationSurfaceCandidates(row) {
		left, right, ok := types.AnswerAggregateMemberRelationParts(candidate)
		if !ok {
			continue
		}
		if principalEnumerationRelationPartCoveredByCells(left, cells) &&
			principalEnumerationRelationPartCoveredByCells(right, cells) {
			return true
		}
	}
	return false
}

func principalEnumerationRelationSurfaceCandidates(row types.EnumerationDisplayRow) []string {
	raw := principalEnumerationRowSurfaceCandidates(row)
	for _, candidate := range append([]string{row.DisplayLabel, row.Member}, raw...) {
		raw = append(raw, types.AnswerAggregateMemberDisplayCandidates(candidate)...)
	}
	return dedupPreEmitStringCandidates(raw)
}

func principalEnumerationRelationPartCoveredByCells(part string, cells []string) bool {
	part = strings.TrimSpace(part)
	if part == "" {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		if principalEnumerationCoveragePartAppears(part, cell) {
			return true
		}
	}
	return false
}

func principalEnumerationMarkdownRowFileCompatible(surface string, row types.EnumerationDisplayRow) bool {
	if strings.TrimSpace(row.Source) == "" {
		return true
	}
	locations := aggregateToolLocationPattern.FindAllString(surface, -1)
	if len(locations) == 0 {
		return true
	}
	rowFile := principalEnumerationLocationFileKey(row.Source)
	if rowFile == "" {
		return false
	}
	for _, loc := range locations {
		candidate := principalEnumerationLocationFileKey(loc)
		if candidate == "" {
			continue
		}
		if principalEnumerationLocationKeyMatches(candidate, rowFile) ||
			principalEnumerationLocationKeyMatches(rowFile, candidate) {
			return true
		}
	}
	return false
}

func principalEnumerationStructuredItemCoversRow(item types.AnswerBlockItem, doc *types.AnswerDocumentV2, row types.EnumerationDisplayRow) bool {
	if principalEnumerationStructuredItemCoversDecoratedBase(item, doc, row) {
		return true
	}
	surface := types.AnswerBlockItemVisibleSurface(item)
	surfaces := []string{surface, item.Label}
	relationPartsCovered := principalEnumerationRelationPartsCoveredByCells(surfaces, row)
	if !principalEnumerationAnySurfaceMatchesRow(surfaces, row) && !relationPartsCovered {
		return false
	}
	if principalEnumerationCandidateLocationCompatible(surface, row) {
		return true
	}
	if relationPartsCovered && principalEnumerationMarkdownRowFileCompatible(surface, row) {
		return true
	}
	if principalEnumerationItemCitationCompatible(item, doc, row) {
		return true
	}
	return (principalEnumerationItemExactLabelMatchesRow(item, row) || principalEnumerationItemCanUseFileOnlyCitationFallback(surface, row)) &&
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

func principalEnumerationItemCanUseFileOnlyCitationFallback(surface string, row types.EnumerationDisplayRow) bool {
	if !row.HasCitation || row.LineStart <= 0 {
		return true
	}
	return len(aggregateToolLocationPattern.FindAllString(surface, -1)) == 0
}

func principalEnumerationItemExactLabelMatchesRow(item types.AnswerBlockItem, row types.EnumerationDisplayRow) bool {
	labelKey := normalizeEnumerationDisplayTableKey(item.Label)
	if labelKey == "" {
		return false
	}
	return principalEnumerationRowKeySet(row)[labelKey]
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
	return types.AnswerBlockKindRendersStructuredItems(block.Kind)
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
	coreKey := principalEnumerationLabelCoreKey(set.Label)
	if coreKey == "" {
		coreKey = principalEnumerationLabelCoreKey(set.ID)
	}
	surfaceCoreKey := principalEnumerationLabelCoreKey(surface)
	if coreKey != "" && surfaceCoreKey != "" && coreKey != key {
		if surfaceCoreKey == coreKey {
			return 32
		}
		if strings.Contains(surfaceCoreKey, coreKey) {
			return 28
		}
	}
	return 0
}

func principalEnumerationLabelCoreKey(label string) string {
	return principalEnumerationLabelKey(stripPrincipalEnumerationParentheticalQualifiers(label))
}

func stripPrincipalEnumerationParentheticalQualifiers(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	var b strings.Builder
	depth := 0
	for _, r := range label {
		switch r {
		case '(', '（', '[', '［', '{', '｛':
			depth++
			continue
		case ')', '）', ']', '］', '}', '｝':
			if depth > 0 {
				depth--
				continue
			}
		}
		if depth > 0 {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func buildPrincipalEnumerationRowsBlock(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet, rows []types.EnumerationDisplayRow, zh bool, mode principalEnumerationSupplementMode) types.AnswerBlock {
	blockSet := set
	blockSet.Rows = append([]types.EnumerationDisplayRow(nil), rows...)
	shape := principalEnumerationTableShapeForSet(blockSet, nil)
	block := types.AnswerBlock{
		ID:                  uniqueAnswerBlockID(doc, "principal_enum_"+sanitizeEnumerationBlockID(set.ID)),
		Kind:                types.BlockTable,
		Title:               principalEnumerationRowsBlockTitle(set, rows, zh, mode),
		SurfaceRole:         types.SurfacePrincipal,
		SystemGeneratedKind: principalEnumerationRowsBlockSystemKind(mode),
		FacetIDs:            []string{string(types.FacetEnumerationItem)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimDefinitionFact,
			FacetID:   string(types.FacetEnumerationItem),
		}},
		Columns: principalEnumerationTableColumns(zh, shape, blockSet.Rows),
	}
	block.Items = principalEnumerationItemsForSet(doc, blockSet, block.Kind, nil, shape)
	return block
}

func principalEnumerationRowsBlockSystemKind(mode principalEnumerationSupplementMode) types.AnswerSystemGeneratedBlockKind {
	switch mode {
	case principalEnumerationSupplementMissing:
		return types.AnswerSystemGeneratedPrincipalEnumerationMissing
	case principalEnumerationSupplementVerifiedFields:
		return types.AnswerSystemGeneratedPrincipalEnumerationFields
	case principalEnumerationSupplementVerifiedNotes:
		return types.AnswerSystemGeneratedPrincipalEnumerationNotes
	default:
		return types.AnswerSystemGeneratedPrincipalEnumerationRows
	}
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

func principalEnumerationRowsNeedingNoteSupplement(doc *types.AnswerDocumentV2, ctx *types.BusContext, set types.EnumerationDisplaySet) []types.EnumerationDisplayRow {
	if doc == nil || len(set.Rows) == 0 {
		return nil
	}
	if !principalEnumerationVerifiedNoteSupplementAllowed(ctx) {
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

func principalEnumerationVerifiedNoteSupplementAllowed(ctx *types.BusContext) bool {
	rm := principalEnumerationEffectiveRequestModel(ctx)
	if rm == nil {
		return false
	}
	if rm.SourceInventoryProfile != nil &&
		rm.SourceInventoryProfile.Active() &&
		rm.SourceInventoryProfile.RequestsField(types.SourceInventoryFieldSummary) &&
		!types.HasTypedRelationMemberSetShape(*rm) {
		return true
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			switch dim.Role {
			case types.RequestedAnswerDimensionFunctionOrPurpose,
				types.RequestedAnswerDimensionImpact,
				types.RequestedAnswerDimensionComparisonAxis:
				return true
			}
		}
	}
	return false
}

func principalEnumerationEffectiveRequestModel(ctx *types.BusContext) *types.RequestModel {
	if ctx == nil {
		return nil
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm
		}
	}
	if ctx.AnalysisIR != nil {
		return &ctx.AnalysisIR.RequestModel
	}
	return nil
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
		if principalEnumerationBlockCanCarryRows(block) {
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
			continue
		}
		if principalEnumerationProseBlockHasAuthoredDescription(block, row) {
			return true
		}
	}
	return false
}

func principalEnumerationProseBlockHasAuthoredDescription(block types.AnswerBlock, row types.EnumerationDisplayRow) bool {
	if preEmitSystemEnumerationRowSupplementBlock(block) {
		return false
	}
	switch block.Kind {
	case types.BlockSummary, types.BlockSection, types.BlockScalar, types.BlockDecision, types.BlockCaveat:
	default:
		return false
	}
	surface := strings.TrimSpace(types.AnswerBlockVisibleSurface(block))
	if surface == "" {
		return false
	}
	for _, segment := range principalEnumerationDescriptionSegments(surface) {
		if !principalEnumerationSurfaceCoversRowForDescription(segment, row) {
			continue
		}
		if principalEnumerationSurfaceHasAuthoredDescription(segment, row) {
			return true
		}
	}
	return principalEnumerationSurfaceCoversRowForDescription(surface, row) &&
		principalEnumerationSurfaceHasAuthoredDescription(surface, row)
}

func principalEnumerationSurfaceCoversRowForDescription(surface string, row types.EnumerationDisplayRow) bool {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return false
	}
	if principalEnumerationVisibleSurfaceCoversRow(surface, row) {
		return true
	}
	for _, candidate := range principalEnumerationRowSurfaceCandidates(row) {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if preEmitAggregateMemberAppearsInText(candidate, surface) {
			return true
		}
	}
	return false
}

func principalEnumerationSurfaceHasAuthoredDescription(surface string, row types.EnumerationDisplayRow) bool {
	residual := principalEnumerationCleanCellText(surface)
	if residual == "" {
		return false
	}
	residual = aggregateToolLocationPattern.ReplaceAllString(residual, " ")
	for _, candidate := range principalEnumerationDescriptionRemovalCandidates(row) {
		residual = replaceCaseInsensitiveLiteral(residual, candidate, " ")
	}
	residual = strings.Trim(residual, " \t\r\n`*_()（）[]【】{}|,，;；:：.-")
	residual = strings.Join(strings.Fields(residual), " ")
	return principalEnumerationDescriptionCellLooksMeaningful(residual)
}

func principalEnumerationDescriptionRemovalCandidates(row types.EnumerationDisplayRow) []string {
	raw := principalEnumerationRowSurfaceCandidates(row)
	if category := strings.TrimSpace(row.Category); category != "" {
		raw = append(raw, category)
	}
	if pkg := strings.TrimSpace(enumerationDisplayPackageCell(row)); pkg != "" {
		raw = append(raw, pkg)
	}
	for _, attr := range row.Attributes {
		if name := strings.TrimSpace(attr.Name); name != "" {
			raw = append(raw, name)
		}
		if location := strings.TrimSpace(attr.Location); location != "" {
			raw = append(raw, location)
		}
	}
	for _, relationSurface := range principalEnumerationRelationSurfaceCandidates(row) {
		left, right, ok := types.AnswerAggregateMemberRelationParts(relationSurface)
		if !ok {
			continue
		}
		raw = append(raw, left, right)
	}
	return dedupPreEmitStringCandidates(raw)
}

func principalEnumerationDescriptionSegments(surface string) []string {
	surface = strings.ReplaceAll(surface, "\r\n", "\n")
	var out []string
	start := 0
	flush := func(end int) {
		if end < start {
			return
		}
		segment := strings.TrimSpace(surface[start:end])
		if segment != "" {
			out = append(out, segment)
		}
	}
	for idx, r := range surface {
		switch r {
		case '\n', '。', '.', '；', ';', '！', '!', '？', '?':
			flush(idx)
			start = idx + len(string(r))
		}
	}
	flush(len(surface))
	return out
}

func replaceCaseInsensitiveLiteral(s, old, new string) string {
	old = strings.TrimSpace(old)
	if s == "" || old == "" {
		return s
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(old))
	return re.ReplaceAllString(s, new)
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
	cellKey := principalEnumerationNoteCoverageKey(cell)
	for _, candidate := range principalEnumerationDescriptionRemovalCandidates(row) {
		if principalEnumerationNoteCoverageKey(cell) == principalEnumerationNoteCoverageKey(candidate) {
			return false
		}
	}
	if row.Location != "" && principalEnumerationCandidateLocationCompatible(cell, row) &&
		len(aggregateToolLocationPattern.FindAllString(cell, -1)) > 0 {
		withoutLocations := strings.TrimSpace(aggregateToolLocationPattern.ReplaceAllString(cell, ""))
		if withoutLocations == "" {
			return false
		}
	}
	if cellKey == "" {
		return false
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
	locationIndex := enumerationDisplayRowLocationIndex([]types.EnumerationDisplaySet{set})
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" {
			continue
		}
		rows, ok := enumerationDisplayRowsForIncompleteTable(block, doc.Citations, index, locationIndex)
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
	includeSurface  bool
	includeLocation bool
	includePackage  bool
	includeNote     bool
}

func principalEnumerationTableShapeForSet(set types.EnumerationDisplaySet, existingNotes map[string]string) principalEnumerationTableShape {
	shape := principalEnumerationTableShapeForRows(set.Rows, existingNotes)
	if principalEnumerationSetIsSourceInventoryPrincipalRows(set) && principalEnumerationRowsHavePreferredSourceInventorySurface(set.Rows) {
		shape.includeSurface = true
	}
	return shape
}

func principalEnumerationTableShapeForRows(rows []types.EnumerationDisplayRow, existingNotes map[string]string) principalEnumerationTableShape {
	var shape principalEnumerationTableShape
	for _, row := range rows {
		if strings.TrimSpace(row.Location) != "" {
			shape.includeLocation = true
		}
		if strings.TrimSpace(enumerationDisplayPackageCell(row)) != "" {
			shape.includePackage = true
		}
		if strings.TrimSpace(principalEnumerationRowNote(row, existingNotes)) != "" {
			shape.includeNote = true
		}
	}
	return shape
}

func principalEnumerationTableColumns(zh bool, shape principalEnumerationTableShape, rows []types.EnumerationDisplayRow) []string {
	columns := []string{principalEnumerationPrimaryColumnLabel(zh, rows)}
	if shape.includeSurface {
		if zh {
			columns = append(columns, "标记")
		} else {
			columns = append(columns, "Surface")
		}
	}
	if shape.includeLocation {
		if zh {
			columns = append(columns, "定义位置")
		} else {
			columns = append(columns, "Location")
		}
	}
	if shape.includePackage {
		if zh {
			columns = append(columns, "包路径")
		} else {
			columns = append(columns, "Package")
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
	if shape.includeSurface {
		cells = append(cells, strings.TrimSpace(principalEnumerationPreferredSourceInventorySurface(row)))
	}
	if shape.includeLocation {
		cells = append(cells, strings.TrimSpace(row.Location))
	}
	if shape.includePackage {
		cells = append(cells, strings.TrimSpace(enumerationDisplayPackageCell(row)))
	}
	if shape.includeNote {
		cells = append(cells, strings.TrimSpace(note))
	}
	return cells
}

func principalEnumerationRowCompatibleWithTableShape(row types.EnumerationDisplayRow, existingNotes map[string]string, shape principalEnumerationTableShape) bool {
	if shape.includeSurface && strings.TrimSpace(principalEnumerationPreferredSourceInventorySurface(row)) == "" {
		return false
	}
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
	generic := principalEnumerationGenericRowSurfaceTermKeys(row)
	for _, term := range row.SurfaceTerms {
		if generic[normalizeEnumerationDisplayTableKey(term)] {
			continue
		}
		raw = append(raw, term)
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

func principalEnumerationLocationFileKey(raw string) string {
	key := principalEnumerationLocationKey(raw)
	if key == "" {
		return ""
	}
	if surface, ok := types.ParseAnswerSourceLocationSurface(key); ok {
		return principalEnumerationLocationKey(surface.File)
	}
	if idx := strings.LastIndex(key, ":"); idx > 0 && idx < len(key)-1 {
		return key[:idx]
	}
	return key
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
	raw = append(raw, principalEnumerationRowSurfaceCandidates(row)...)
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
		bestIdx = principalEnumerationAdjacentSectionBlockIndex(doc, set)
	}
	if bestIdx < 0 {
		return 0
	}
	changed := 0
	block := &doc.Blocks[bestIdx]
	sectionSet := set
	if scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(*block, set); ok {
		sectionSet = scoped
		title = principalEnumerationBlockTitle(sectionSet)
		text = fmt.Sprintf("%s共 %d 项；完整成员、定义位置和说明见对应表格。", strings.TrimSpace(sectionSet.Label), len(sectionSet.Rows))
	}
	if !principalEnumerationSectionBlockIsGeneratedShell(*block, sectionSet, title, text) {
		title, text = principalEnumerationAdjacentSectionTitleText(*block, sectionSet)
	}
	if block.Text != text {
		block.Text = text
		changed++
	}
	if block.Title != title {
		block.Title = title
		changed++
	}
	block.SystemGeneratedKind = types.AnswerSystemGeneratedPrincipalEnumerationSection
	block.SurfaceRole = types.SurfacePrincipal
	block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
	block.ClaimUses = appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))

	if removed := removeRedundantPrincipalEnumerationSectionBlocks(doc, sectionSet, title, text, bestIdx); removed > 0 {
		changed += removed
	}
	return changed
}

func normalizePrincipalEnumerationSourceInventorySectionBlocks(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) int {
	if doc == nil || !principalEnumerationSetIsSourceInventoryPrincipalRows(set) || len(set.Rows) == 0 {
		return 0
	}
	changed := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if !principalEnumerationSectionBlockCanSummarizeAdjacentCarrier(*block, set) {
			continue
		}
		scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(*block, set)
		if !ok || len(scoped.Rows) == 0 || !principalEnumerationSectionHasAdjacentCarrier(doc, i, scoped) {
			continue
		}
		title, text := principalEnumerationAdjacentSectionTitleText(*block, scoped)
		if block.Title != title {
			block.Title = title
			changed++
		}
		if block.Text != text {
			block.Text = text
			changed++
		}
		block.SurfaceRole = types.SurfacePrincipal
		block.FacetIDs = mergeStringSet(block.FacetIDs, []string{string(types.FacetEnumerationItem)})
		block.ClaimUses = appendRenderedClaimUseIfMissing(block.ClaimUses, types.ClaimDefinitionFact, string(types.FacetEnumerationItem))
	}
	return changed
}

func principalEnumerationSourceInventoryScopedSetForBlock(block types.AnswerBlock, set types.EnumerationDisplaySet) (types.EnumerationDisplaySet, bool) {
	if !principalEnumerationSetIsSourceInventoryPrincipalRows(set) || len(set.Rows) <= 1 {
		return types.EnumerationDisplaySet{}, false
	}
	label := principalEnumerationSourceInventoryScopedLabel(block.Title)
	if label == "" {
		return types.EnumerationDisplaySet{}, false
	}
	labelKey := principalEnumerationLabelKey(label)
	if labelKey == "" {
		return types.EnumerationDisplaySet{}, false
	}
	var rows []types.EnumerationDisplayRow
	for _, row := range set.Rows {
		if !principalEnumerationSourceInventoryRowMatchesScopedLabel(row, labelKey) {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 || len(rows) == len(set.Rows) {
		return types.EnumerationDisplaySet{}, false
	}
	scoped := set
	scoped.ID = set.ID + ":" + sanitizeEnumerationBlockID(label)
	scoped.Label = label
	scoped.Value = strconv.Itoa(len(rows))
	scoped.Rows = rows
	return scoped, true
}

func principalEnumerationAdjacentSourceInventoryRowsForCarrier(doc *types.AnswerDocumentV2, idx int, sets []types.EnumerationDisplaySet) ([]types.EnumerationDisplayRow, bool) {
	if doc == nil || idx <= 0 || idx >= len(doc.Blocks) || len(sets) == 0 {
		return nil, false
	}
	carrier := doc.Blocks[idx]
	if !principalEnumerationBlockCanCarryRows(carrier) || !principalEnumerationBlockHasEnumerationFacet(carrier) {
		return nil, false
	}
	if strings.TrimSpace(carrier.Title) != "" {
		return nil, false
	}
	section := doc.Blocks[idx-1]
	if !principalEnumerationSectionBlockCanSummarizeAdjacentCarrier(section, types.EnumerationDisplaySet{}) {
		return nil, false
	}
	if strings.TrimSpace(section.Title) == "" {
		return nil, false
	}
	var best []types.EnumerationDisplayRow
	ties := 0
	for _, set := range sets {
		if !principalEnumerationSetIsSourceInventoryPrincipalRows(set) {
			continue
		}
		scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(section, set)
		if !ok || len(scoped.Rows) == 0 {
			continue
		}
		if !principalEnumerationCarrierTouchesAnyRow(carrier, doc, scoped.Rows) {
			continue
		}
		if len(scoped.Rows) > len(best) {
			best = scoped.Rows
			ties = 1
			continue
		}
		if len(scoped.Rows) == len(best) {
			ties++
		}
	}
	if len(best) == 0 || ties != 1 {
		return nil, false
	}
	return best, true
}

func principalEnumerationCarrierTouchesAnyRow(block types.AnswerBlock, doc *types.AnswerDocumentV2, rows []types.EnumerationDisplayRow) bool {
	for _, item := range block.Items {
		if _, ok := principalEnumerationUniqueItemRow(item, rows, principalEnumerationExactLabelRowIndex([]types.EnumerationDisplaySet{{
			ID:    "adjacent-source-inventory",
			Label: strings.TrimSpace(block.Title),
			Rows:  rows,
		}})); ok {
			return true
		}
		for _, row := range rows {
			if principalEnumerationStructuredItemCoversRow(item, doc, row) ||
				principalEnumerationItemWeaklyIdentifiesRow(item, row) {
				return true
			}
		}
	}
	if strings.TrimSpace(block.Text) != "" {
		for _, row := range rows {
			if principalEnumerationVisibleSurfaceCoversRow(block.Text, row) {
				return true
			}
		}
	}
	return false
}

func principalEnumerationBestSourceInventoryScopedSetForBlock(block types.AnswerBlock, sets []types.EnumerationDisplaySet) (types.EnumerationDisplaySet, bool) {
	var best types.EnumerationDisplaySet
	bestScore := 0
	ties := 0
	for _, set := range sets {
		scoped, ok := principalEnumerationSourceInventoryScopedSetForBlock(block, set)
		if !ok || len(scoped.Rows) == 0 {
			continue
		}
		score := principalEnumerationBlockSetScore(block, scoped)
		if score <= bestScore {
			if score == bestScore && score > 0 {
				ties++
			}
			continue
		}
		best = scoped
		bestScore = score
		ties = 1
	}
	if bestScore == 0 || ties != 1 {
		return types.EnumerationDisplaySet{}, false
	}
	return best, true
}

func principalEnumerationSourceInventoryScopedLabel(title string) string {
	label := strings.TrimSpace(stripPrincipalEnumerationParentheticalQualifiers(title))
	if label == "" {
		return ""
	}
	for {
		trimmed := strings.TrimSpace(label)
		next := strings.TrimSpace(strings.TrimSuffix(trimmed, "一览"))
		next = strings.TrimSpace(strings.TrimSuffix(next, "列表"))
		next = strings.TrimSpace(strings.TrimSuffix(next, "详情"))
		next = strings.TrimSpace(strings.TrimSuffix(next, "明细"))
		if next == "" || next == trimmed {
			return trimmed
		}
		label = next
	}
}

func principalEnumerationSourceInventoryRowMatchesScopedLabel(row types.EnumerationDisplayRow, labelKey string) bool {
	if labelKey == "" {
		return false
	}
	for _, term := range row.SurfaceTerms {
		termKey := principalEnumerationLabelKey(term)
		if termKey == "" {
			continue
		}
		if strings.Contains(labelKey, termKey) || strings.Contains(termKey, labelKey) {
			return true
		}
	}
	return false
}

func principalEnumerationAdjacentSectionTitleText(block types.AnswerBlock, set types.EnumerationDisplaySet) (string, string) {
	label := strings.TrimSpace(stripPrincipalEnumerationParentheticalQualifiers(block.Title))
	if label == "" {
		label = strings.TrimSpace(set.Label)
	}
	if label == "" {
		label = "成员清单"
	}
	return fmt.Sprintf("%s（%d）", label, len(set.Rows)),
		fmt.Sprintf("%s共 %d 项；完整成员、定义位置和说明见对应表格。", label, len(set.Rows))
}

func principalEnumerationAdjacentSectionBlockIndex(doc *types.AnswerDocumentV2, set types.EnumerationDisplaySet) int {
	if doc == nil || len(set.Rows) == 0 {
		return -1
	}
	for i := range doc.Blocks {
		if !principalEnumerationSectionBlockCanSummarizeAdjacentCarrier(doc.Blocks[i], set) {
			continue
		}
		if principalEnumerationSectionHasAdjacentCarrier(doc, i, set) {
			return i
		}
	}
	return -1
}

func principalEnumerationSectionBlockCanSummarizeAdjacentCarrier(block types.AnswerBlock, set types.EnumerationDisplaySet) bool {
	if block.Kind != types.BlockSection || len(block.Items) > 0 || len(block.Columns) > 0 || block.Diagram != nil {
		return false
	}
	return true
}

func principalEnumerationSectionHasAdjacentCarrier(doc *types.AnswerDocumentV2, idx int, set types.EnumerationDisplaySet) bool {
	section := doc.Blocks[idx]
	for i := idx + 1; i < len(doc.Blocks); i++ {
		block := doc.Blocks[i]
		if principalEnumerationBlockCanCarryRows(block) {
			return principalEnumerationBlockCoversAnyRow(block, doc, set) &&
				principalEnumerationSectionTitleMatchesSetOrCarrier(section.Title, set, block)
		}
		if strings.TrimSpace(types.AnswerBlockVisibleSurface(block)) == "" {
			continue
		}
		return false
	}
	return false
}

func principalEnumerationSectionTitleMatchesSetOrCarrier(title string, set types.EnumerationDisplaySet, carrier types.AnswerBlock) bool {
	if principalEnumerationSetLabelMatchScore(title, set) >= 35 ||
		principalEnumerationSectionTitleMatchesTypedSurface(title, set) {
		return true
	}
	titleKey := principalEnumerationLabelKey(title)
	carrierKey := principalEnumerationLabelKey(carrier.Title)
	return titleKey != "" && carrierKey != "" && titleKey == carrierKey
}

func principalEnumerationSectionTitleMatchesTypedSurface(title string, set types.EnumerationDisplaySet) bool {
	titleKey := principalEnumerationLabelKey(title)
	if titleKey == "" {
		return false
	}
	for _, row := range set.Rows {
		for _, term := range row.SurfaceTerms {
			termKey := principalEnumerationLabelKey(term)
			if termKey == "" {
				continue
			}
			if strings.Contains(titleKey, termKey) || strings.Contains(termKey, titleKey) {
				return true
			}
		}
	}
	return false
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
	if block.SystemGeneratedKind != types.AnswerSystemGeneratedPrincipalEnumerationSection {
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
	return false
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
