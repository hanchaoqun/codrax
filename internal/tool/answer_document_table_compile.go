package tool

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// compileCitationBackedTableRows repairs a common local-model table
// degradation: the model declares a multi-column table via columns[],
// but emits only label/text/citation_ref row carriers and leaves
// items[].cells empty. Rendering that shape as-is creates visibly empty
// trailing columns. The repair is intentionally display-only and
// citation-backed: it never invents answer members, never changes
// citation refs, and only fills row cells from the existing item label,
// item text, and cited file:line/quote.
func compileCitationBackedTableRows(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !shouldCompileCitationBackedTableRows(*block, doc) {
			continue
		}
		zh := answerTableCompilePrefersZH(block.Columns)
		block.Columns = citationBackedTableColumns(zh)
		for ii := range block.Items {
			item := &block.Items[ii]
			label := strings.TrimSpace(item.Label)
			if label == "" {
				continue
			}
			location, quote := citationBackedTableCitationSurface(doc, item.CitationRef)
			note := strings.TrimSpace(item.Text)
			if note == "" {
				note = quote
			}
			item.Cells = []string{location, note}
			fixed++
		}
	}
	return fixed
}

// compileEnumerationDisplayTableRows repairs a richer variant of the same
// local-model degradation handled by compileCitationBackedTableRows. When the
// explorer handed off a complete principal enumeration slate, the finalizer
// sometimes emits a table with correct row labels but empty trailing cells. In
// that case we can fill the cells from the deterministic enumeration-row
// contract instead of asking the model to rewrite the answer.
//
// Safety boundary: this helper only consumes structured aggregate facts,
// support refs, and grounded EvidenceItem rows compiled by
// types.CompileEnumerationDisplaySets. It never parses user prose, model
// thoughts, closure text, or free-form markdown tables; if a row label is not a
// unique exact match, the table is left untouched.
func compileEnumerationDisplayTableRows(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
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
	index := enumerationDisplayRowIndex(sets)
	if len(index) == 0 {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		rows, ok := enumerationDisplayRowsForIncompleteTable(*block, index)
		if !ok {
			continue
		}
		includeCategory := false
		includeLocation := false
		includeNote := false
		for ii, row := range rows {
			if strings.TrimSpace(row.Category) != "" {
				includeCategory = true
			}
			if strings.TrimSpace(row.Location) != "" {
				includeLocation = true
			}
			itemText := ""
			if ii < len(block.Items) {
				itemText = block.Items[ii].Text
			}
			if strings.TrimSpace(row.Note) != "" || strings.TrimSpace(itemText) != "" {
				includeNote = true
			}
		}
		zh := answerTableCompilePrefersZH(block.Columns)
		block.Columns = enumerationDisplayTableColumns(zh, includeCategory, includeLocation, includeNote)
		for ii := range block.Items {
			item := &block.Items[ii]
			row := rows[ii]
			if row.RowID == "" {
				continue
			}
			note := answerTableCompileFirstNonEmptyString(item.Text, row.Note)
			item.Label = answerTableCompileFirstNonEmptyString(row.DisplayLabel, item.Label, row.Member)
			item.Cells = enumerationDisplayTableCells(row, includeCategory, includeLocation, includeNote, note)
			if strings.TrimSpace(item.Text) == "" {
				item.Text = strings.TrimSpace(row.Note)
			}
			if row.HasCitation {
				if ref := appendOrReusePreEmitCitation(doc, types.Citation{
					File: row.Source,
					Line: row.LineStart,
				}); ref >= 0 {
					item.CitationRef = ref
				}
			} else if item.CitationRef >= len(doc.Citations) {
				item.CitationRef = -1
			}
			fixed++
		}
	}
	return fixed
}

func shouldCompileCitationBackedTableRows(block types.AnswerBlock, doc *types.AnswerDocumentV2) bool {
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Columns) < 3 || len(block.Items) == 0 {
		return false
	}
	visible := 0
	compileCandidates := 0
	for _, item := range block.Items {
		label := strings.TrimSpace(item.Label)
		text := strings.TrimSpace(item.Text)
		cells := nonEmptyAnswerTableCompileCells(item.Cells)
		if label == "" && text == "" && len(cells) == 0 {
			continue
		}
		visible++
		if label == "" || len(cells) > 0 {
			return false
		}
		if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) {
			compileCandidates++
		}
	}
	return visible > 0 && compileCandidates == visible
}

func enumerationDisplayRowIndex(sets []types.EnumerationDisplaySet) map[string]types.EnumerationDisplayRow {
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
			add(row.Location, row)
			if label, _, ok := types.ParseAnswerSupportRefMemberLocation(row.Member); ok {
				add(label, row)
			}
			if surface, ok := types.ParseAnswerSourceLocationSurface(row.Member); ok {
				add(surface.File, row)
			}
		}
	}
	return candidates
}

func enumerationDisplayRowsForIncompleteTable(block types.AnswerBlock, index map[string]types.EnumerationDisplayRow) ([]types.EnumerationDisplayRow, bool) {
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Items) == 0 || len(block.Columns) < 2 {
		return nil, false
	}
	rows := make([]types.EnumerationDisplayRow, len(block.Items))
	visible := 0
	for i, item := range block.Items {
		label := strings.TrimSpace(item.Label)
		text := strings.TrimSpace(item.Text)
		cells := nonEmptyAnswerTableCompileCells(item.Cells)
		if label == "" && text == "" && len(cells) == 0 {
			continue
		}
		visible++
		if label == "" || len(cells) > 0 {
			return nil, false
		}
		row, ok := index[normalizeEnumerationDisplayTableKey(label)]
		if !ok {
			return nil, false
		}
		rows[i] = row
	}
	if visible == 0 {
		return nil, false
	}
	return rows, true
}

func normalizeEnumerationDisplayTableKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`")
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func enumerationDisplayTableColumns(zh bool, includeCategory bool, includeLocation bool, includeNote bool) []string {
	columns := []string{"Name"}
	if zh {
		columns = []string{"符号名称"}
	}
	if includeCategory {
		if zh {
			columns = append(columns, "类别")
		} else {
			columns = append(columns, "Category")
		}
	}
	if includeLocation {
		if zh {
			columns = append(columns, "定义位置")
		} else {
			columns = append(columns, "Location")
		}
	}
	if includeNote {
		if zh {
			columns = append(columns, "说明")
		} else {
			columns = append(columns, "Notes")
		}
	}
	return columns
}

func enumerationDisplayTableCells(row types.EnumerationDisplayRow, includeCategory bool, includeLocation bool, includeNote bool, note string) []string {
	cells := make([]string, 0, 3)
	if includeCategory {
		cells = append(cells, strings.TrimSpace(row.Category))
	}
	if includeLocation {
		cells = append(cells, strings.TrimSpace(row.Location))
	}
	if includeNote {
		cells = append(cells, strings.TrimSpace(note))
	}
	return cells
}

func answerTableCompileFirstNonEmptyString(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}

func nonEmptyAnswerTableCompileCells(cells []string) []string {
	out := make([]string, 0, len(cells))
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			out = append(out, cell)
		}
	}
	return out
}

func answerTableCompilePrefersZH(values []string) bool {
	for _, value := range values {
		for _, r := range value {
			if unicode.Is(unicode.Han, r) {
				return true
			}
		}
	}
	return false
}

func citationBackedTableColumns(zh bool) []string {
	if zh {
		return []string{"符号名称", "定义位置", "说明"}
	}
	return []string{"Name", "Location", "Notes"}
}

func citationBackedTableCitationSurface(doc *types.AnswerDocumentV2, ref int) (location string, quote string) {
	if doc == nil || ref < 0 || ref >= len(doc.Citations) {
		return "", ""
	}
	cit := doc.Citations[ref]
	file := strings.TrimSpace(cit.File)
	if file != "" && cit.Line > 0 {
		location = fmt.Sprintf("%s:%d", file, cit.Line)
	} else {
		location = file
	}
	quote = strings.TrimSpace(cit.Quote)
	return location, quote
}
