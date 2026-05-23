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
	if answerDocumentRuntimeObservationOnly(ctx) {
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
		shape, ok := enumerationDisplayExistingTableShape(*block, rows)
		if !ok {
			continue
		}
		for ii := range block.Items {
			item := &block.Items[ii]
			row := rows[ii]
			if row.RowID == "" {
				continue
			}
			note := answerTableCompileFirstNonEmptyString(item.Text, row.Note)
			item.Cells = enumerationDisplayTableCellsForShape(row, note, shape)
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

type enumerationDisplayColumnRole string

const (
	enumerationDisplayColumnCategory enumerationDisplayColumnRole = "category"
	enumerationDisplayColumnLocation enumerationDisplayColumnRole = "location"
	enumerationDisplayColumnNote     enumerationDisplayColumnRole = "note"
)

type enumerationDisplayExistingShape struct {
	roles []enumerationDisplayColumnRole
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

func enumerationDisplayExistingTableShape(block types.AnswerBlock, rows []types.EnumerationDisplayRow) (enumerationDisplayExistingShape, bool) {
	if len(rows) == 0 || len(block.Columns) < 2 {
		return enumerationDisplayExistingShape{}, false
	}
	// With item.Label present, the renderer uses the label as the first
	// visible column and items[].cells as the remaining columns. Therefore
	// inline repair is lossless only when the model's existing header count
	// already matches label + cells. Unknown headers are not interpreted; a
	// later supplement can carry deterministic rows in a clearly separated
	// system block instead of rewriting the model table.
	tail := block.Columns[1:]
	roles := make([]enumerationDisplayColumnRole, 0, len(tail))
	seen := map[enumerationDisplayColumnRole]bool{}
	for _, column := range tail {
		role := enumerationDisplayColumnRoleForHeader(column)
		if role == "" || seen[role] {
			return enumerationDisplayExistingShape{}, false
		}
		seen[role] = true
		roles = append(roles, role)
	}
	shape := enumerationDisplayExistingShape{roles: roles}
	for ii, row := range rows {
		note := strings.TrimSpace(row.Note)
		if ii < len(block.Items) {
			note = answerTableCompileFirstNonEmptyString(block.Items[ii].Text, note)
		}
		if !enumerationDisplayRowCompatibleWithExistingShape(row, note, shape) {
			return enumerationDisplayExistingShape{}, false
		}
	}
	return shape, true
}

func enumerationDisplayColumnRoleForHeader(raw string) enumerationDisplayColumnRole {
	header := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(raw)), " "))
	if header == "" {
		return ""
	}
	switch {
	case strings.Contains(header, "类别") ||
		strings.Contains(header, "分类") ||
		strings.Contains(header, "category") ||
		header == "kind":
		return enumerationDisplayColumnCategory
	case strings.Contains(header, "位置") ||
		strings.Contains(header, "定义") ||
		strings.Contains(header, "文件") ||
		strings.Contains(header, "行号") ||
		strings.Contains(header, "location") ||
		strings.Contains(header, "file") ||
		strings.Contains(header, "line"):
		return enumerationDisplayColumnLocation
	case strings.Contains(header, "说明") ||
		strings.Contains(header, "描述") ||
		strings.Contains(header, "备注") ||
		strings.Contains(header, "职责") ||
		strings.Contains(header, "作用") ||
		strings.Contains(header, "note") ||
		strings.Contains(header, "detail") ||
		strings.Contains(header, "summary") ||
		strings.Contains(header, "description"):
		return enumerationDisplayColumnNote
	default:
		return ""
	}
}

func enumerationDisplayRowCompatibleWithExistingShape(row types.EnumerationDisplayRow, note string, shape enumerationDisplayExistingShape) bool {
	for _, role := range shape.roles {
		if strings.TrimSpace(enumerationDisplayCellForRole(row, note, role)) == "" {
			return false
		}
	}
	return true
}

func normalizeEnumerationDisplayTableKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`")
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func enumerationDisplayTableCellsForShape(row types.EnumerationDisplayRow, note string, shape enumerationDisplayExistingShape) []string {
	cells := make([]string, 0, len(shape.roles))
	for _, role := range shape.roles {
		cells = append(cells, strings.TrimSpace(enumerationDisplayCellForRole(row, note, role)))
	}
	return cells
}

func enumerationDisplayCellForRole(row types.EnumerationDisplayRow, note string, role enumerationDisplayColumnRole) string {
	switch role {
	case enumerationDisplayColumnCategory:
		return row.Category
	case enumerationDisplayColumnLocation:
		return row.Location
	case enumerationDisplayColumnNote:
		return note
	default:
		return ""
	}
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
