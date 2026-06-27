package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// compileCitationBackedTableRows is intentionally a no-op. Earlier versions
// tried to "repair" structured table carriers by filling items[].cells from
// citation refs, but that still rewrote the model-authored table surface
// (columns / visible row shape) and could collapse richer user-facing tables
// into a system-preferred Name/Location/Notes shape. Renderer-side empty-column
// compaction is acceptable because it only displays existing model content; any
// future deterministic supplement must be a separate, clearly labelled block.
func compileCitationBackedTableRows(doc *types.AnswerDocumentV2) int {
	return 0
}

// compileEnumerationDisplayTableRows is also intentionally a no-op. Even when
// the deterministic enumeration-row contract has richer location/note data,
// writing that data into the model's existing table would still alter the
// model-authored visible answer shape. The safe pattern is: preserve the table
// exactly, then add a separate labelled supplement only when a higher-level
// contract decides the missing verified fields are important enough to show.
func compileEnumerationDisplayTableRows(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	return 0
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
		if label == "" || text != "" || len(cells) > 0 {
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
