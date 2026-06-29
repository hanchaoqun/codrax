package tool

import (
	"strconv"
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
	labelIndex := enumerationDisplayRowIndex(sets)
	locationIndex := enumerationDisplayRowLocationIndex(sets)
	changed := 0
	for blockIdx := range doc.Blocks {
		rows, ok := enumerationDisplayRowsForIncompleteTable(doc.Blocks[blockIdx], doc.Citations, labelIndex, locationIndex)
		implicitColumns := false
		if !ok {
			rows, ok = enumerationDisplayRowsForImplicitNoColumnTable(doc.Blocks[blockIdx], doc.Citations, labelIndex, locationIndex)
			implicitColumns = ok
		}
		if !ok {
			continue
		}
		shape, ok := enumerationDisplayExistingTableShape(doc.Blocks[blockIdx], rows)
		if !ok && !implicitColumns &&
			enumerationDisplayBlockColumnsHaveLocationHeader(doc.Blocks[blockIdx].Columns) &&
			enumerationDisplayBlockRowsAllCitationBacked(doc.Blocks[blockIdx], doc.Citations) {
			shape, ok = enumerationDisplayImplicitCitationBackedTableShape(rows)
			if ok {
				doc.Blocks[blockIdx].Columns = enumerationDisplayImplicitTableColumns(ctx, shape, rows)
			}
		}
		if !ok && implicitColumns {
			shape, ok = enumerationDisplayImplicitTypedAttributeTableShape(rows)
			if ok {
				doc.Blocks[blockIdx].Columns = enumerationDisplayImplicitTableColumns(ctx, shape, rows)
			}
		}
		if !ok || !enumerationDisplayShapeCanCompileInline(shape, doc.Blocks[blockIdx], doc.Citations) {
			continue
		}
		if enumerationDisplayTableNeedsNoteColumn(doc.Blocks[blockIdx], rows, shape) {
			shape.roles = append(shape.roles, enumerationDisplayColumnNote)
			doc.Blocks[blockIdx].Columns = append(doc.Blocks[blockIdx].Columns, enumerationDisplayDefaultNoteColumn(ctx))
		}
		for itemIdx := range doc.Blocks[blockIdx].Items {
			note := answerTableCompileFirstNonEmptyString(doc.Blocks[blockIdx].Items[itemIdx].Text, rows[itemIdx].Note)
			doc.Blocks[blockIdx].Items[itemIdx].Cells = enumerationDisplayTableCellsForShape(rows[itemIdx], note, shape)
			doc.Blocks[blockIdx].Items[itemIdx].Text = ""
			changed++
		}
	}
	return changed
}

func normalizeEnumerationDisplayRequestedFieldSurfaces(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return 0
	}
	fieldLabel := enumerationDisplayRequestedPackageLikeLabel(ctx, ctx.AnalysisIR.RequestModel.SourceInventoryProfile)
	if fieldLabel == "" {
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
	labelIndex := enumerationDisplayRowIndex(sets)
	locationIndex := enumerationDisplayRowLocationIndex(sets)
	fixed := 0
	for blockIdx := range doc.Blocks {
		block := &doc.Blocks[blockIdx]
		if block.Kind != types.BlockOrderedList && block.Kind != types.BlockBulletList {
			continue
		}
		for itemIdx := range block.Items {
			row, ok := enumerationDisplayRowForListItem(block.Items[itemIdx], doc.Citations, labelIndex, locationIndex)
			if !ok {
				continue
			}
			values := enumerationDisplayMissingPackageLikeValues(block.Items[itemIdx], row)
			if len(values) == 0 {
				continue
			}
			suffix := enumerationDisplayRequestedAttributeSuffix(ctx, fieldLabel, values)
			if suffix == "" {
				continue
			}
			if block.Items[itemIdx].Text = appendEnumerationDisplayItemSuffix(block.Items[itemIdx].Text, suffix, ctx); strings.Contains(block.Items[itemIdx].Text, suffix) {
				fixed++
			}
		}
	}
	return fixed
}

func enumerationDisplayRequestedPackageLikeLabel(ctx *types.BusContext, profile *types.SourceInventoryProfile) string {
	if profile == nil || !profile.Active() {
		return ""
	}
	zh := principalEnumerationPrefersZH(ctx)
	for _, field := range profile.RequestedFields {
		switch field {
		case types.SourceInventoryFieldPackage:
			if zh {
				return "包路径"
			}
			return "Package"
		case types.SourceInventoryFieldModule:
			if zh {
				return "模块"
			}
			return "Module"
		case types.SourceInventoryFieldNamespace:
			if zh {
				return "命名空间"
			}
			return "Namespace"
		}
	}
	return ""
}

func enumerationDisplayRowForListItem(
	item types.AnswerBlockItem,
	citations []types.Citation,
	labelIndex map[string]types.EnumerationDisplayRow,
	locationIndex map[string]types.EnumerationDisplayRow,
) (types.EnumerationDisplayRow, bool) {
	if item.CitationRef >= 0 && item.CitationRef < len(citations) {
		if row, ok := locationIndex[normalizeEnumerationDisplayLocationKey(answerTableCompileCitationLocationKey(citations[item.CitationRef]))]; ok {
			return row, true
		}
	}
	row, ok := labelIndex[normalizeEnumerationDisplayTableKey(item.Label)]
	return row, ok
}

func enumerationDisplayMissingPackageLikeValues(item types.AnswerBlockItem, row types.EnumerationDisplayRow) []string {
	surface := types.AnswerBlockItemVisibleSurface(item)
	seen := map[string]bool{}
	var out []string
	for _, attr := range row.Attributes {
		if attr.Role != types.AnswerCandidateRolePackage {
			continue
		}
		value := strings.TrimSpace(attr.Name)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		if strings.Contains(surface, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func enumerationDisplayRequestedAttributeSuffix(ctx *types.BusContext, label string, values []string) string {
	label = strings.TrimSpace(label)
	if label == "" || len(values) == 0 {
		return ""
	}
	value := strings.Join(values, ", ")
	if principalEnumerationPrefersZH(ctx) {
		return label + "：" + value
	}
	return label + ": " + value
}

func appendEnumerationDisplayItemSuffix(text, suffix string, ctx *types.BusContext) string {
	text = strings.TrimSpace(text)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" || strings.Contains(text, suffix) {
		return text
	}
	if text == "" {
		return suffix
	}
	if principalEnumerationPrefersZH(ctx) {
		return text + "；" + suffix
	}
	return text + "; " + suffix
}

type enumerationDisplayColumnRole string

const (
	enumerationDisplayColumnCategory enumerationDisplayColumnRole = "category"
	enumerationDisplayColumnLocation enumerationDisplayColumnRole = "location"
	enumerationDisplayColumnNote     enumerationDisplayColumnRole = "note"
	enumerationDisplayColumnPackage  enumerationDisplayColumnRole = "package"
)

type enumerationDisplayExistingShape struct {
	roles []enumerationDisplayColumnRole
}

func enumerationDisplayImplicitTypedAttributeTableShape(rows []types.EnumerationDisplayRow) (enumerationDisplayExistingShape, bool) {
	if len(rows) == 0 {
		return enumerationDisplayExistingShape{}, false
	}
	shape := enumerationDisplayExistingShape{}
	if enumerationDisplayRowsHaveLocation(rows) {
		shape.roles = append(shape.roles, enumerationDisplayColumnLocation)
	}
	if enumerationDisplayRowsHavePackageCell(rows) {
		shape.roles = append(shape.roles, enumerationDisplayColumnPackage)
	}
	if enumerationDisplayRowsHaveNote(rows) {
		shape.roles = append(shape.roles, enumerationDisplayColumnNote)
	}
	if !enumerationDisplayShapeHasTypedAttributeColumn(shape) {
		return enumerationDisplayExistingShape{}, false
	}
	for _, row := range rows {
		if !enumerationDisplayRowCompatibleWithExistingShape(row, row.Note, shape) {
			return enumerationDisplayExistingShape{}, false
		}
	}
	return shape, true
}

func enumerationDisplayImplicitCitationBackedTableShape(rows []types.EnumerationDisplayRow) (enumerationDisplayExistingShape, bool) {
	if len(rows) == 0 || !enumerationDisplayRowsHaveLocation(rows) {
		return enumerationDisplayExistingShape{}, false
	}
	shape := enumerationDisplayExistingShape{roles: []enumerationDisplayColumnRole{enumerationDisplayColumnLocation}}
	if enumerationDisplayRowsHavePackageCell(rows) {
		shape.roles = append(shape.roles, enumerationDisplayColumnPackage)
	}
	if enumerationDisplayRowsHaveNote(rows) {
		shape.roles = append(shape.roles, enumerationDisplayColumnNote)
	}
	for _, row := range rows {
		if !enumerationDisplayRowCompatibleWithExistingShape(row, row.Note, shape) {
			return enumerationDisplayExistingShape{}, false
		}
	}
	return shape, true
}

func enumerationDisplayRowsHaveLocation(rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.Location) == "" {
			return false
		}
	}
	return len(rows) > 0
}

func enumerationDisplayRowsHavePackageCell(rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(enumerationDisplayPackageCell(row)) != "" {
			return true
		}
	}
	return false
}

func enumerationDisplayRowsHaveNote(rows []types.EnumerationDisplayRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.Note) == "" {
			return false
		}
	}
	return len(rows) > 0
}

func enumerationDisplayImplicitTableColumns(ctx *types.BusContext, shape enumerationDisplayExistingShape, rows []types.EnumerationDisplayRow) []string {
	zh := principalEnumerationPrefersZH(ctx)
	columns := []string{principalEnumerationPrimaryColumnLabel(zh, rows)}
	for _, role := range shape.roles {
		columns = append(columns, enumerationDisplayColumnHeader(ctx, role))
	}
	return columns
}

func enumerationDisplayColumnHeader(ctx *types.BusContext, role enumerationDisplayColumnRole) string {
	zh := principalEnumerationPrefersZH(ctx)
	switch role {
	case enumerationDisplayColumnCategory:
		if zh {
			return "类别"
		}
		return "Category"
	case enumerationDisplayColumnLocation:
		if zh {
			return "定义位置"
		}
		return "Location"
	case enumerationDisplayColumnPackage:
		if zh {
			return "包路径"
		}
		return "Package"
	case enumerationDisplayColumnNote:
		return enumerationDisplayDefaultNoteColumn(ctx)
	default:
		if zh {
			return "说明"
		}
		return "Notes"
	}
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

func enumerationDisplayRowLocationIndex(sets []types.EnumerationDisplaySet) map[string]types.EnumerationDisplayRow {
	candidates := make(map[string]types.EnumerationDisplayRow)
	ambiguous := make(map[string]bool)
	add := func(key string, row types.EnumerationDisplayRow) {
		key = normalizeEnumerationDisplayLocationKey(key)
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
			add(row.Location, row)
			add(answerTableCompileLocationKey(row.Source, row.LineStart), row)
			add(row.CitationKey, row)
		}
	}
	return candidates
}

func enumerationDisplayRowsForIncompleteTable(
	block types.AnswerBlock,
	citations []types.Citation,
	labelIndex map[string]types.EnumerationDisplayRow,
	locationIndex map[string]types.EnumerationDisplayRow,
) ([]types.EnumerationDisplayRow, bool) {
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
		row, ok := enumerationDisplayRowForTableItem(item, citations, labelIndex, locationIndex)
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

func enumerationDisplayRowsForImplicitNoColumnTable(
	block types.AnswerBlock,
	citations []types.Citation,
	labelIndex map[string]types.EnumerationDisplayRow,
	locationIndex map[string]types.EnumerationDisplayRow,
) ([]types.EnumerationDisplayRow, bool) {
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Items) == 0 || len(block.Columns) != 0 {
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
		row, ok := enumerationDisplayRowForTableItem(item, citations, labelIndex, locationIndex)
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

func enumerationDisplayRowForTableItem(
	item types.AnswerBlockItem,
	citations []types.Citation,
	labelIndex map[string]types.EnumerationDisplayRow,
	locationIndex map[string]types.EnumerationDisplayRow,
) (types.EnumerationDisplayRow, bool) {
	if item.CitationRef >= 0 && item.CitationRef < len(citations) {
		if row, ok := locationIndex[normalizeEnumerationDisplayLocationKey(answerTableCompileCitationLocationKey(citations[item.CitationRef]))]; ok {
			return row, true
		}
	}
	row, ok := labelIndex[normalizeEnumerationDisplayTableKey(item.Label)]
	return row, ok
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
	packageColumnHasValue := false
	for ii, row := range rows {
		note := strings.TrimSpace(row.Note)
		if ii < len(block.Items) {
			note = answerTableCompileFirstNonEmptyString(block.Items[ii].Text, note)
		}
		if strings.TrimSpace(enumerationDisplayPackageCell(row)) != "" {
			packageColumnHasValue = true
		}
		if !enumerationDisplayRowCompatibleWithExistingShape(row, note, shape) {
			return enumerationDisplayExistingShape{}, false
		}
	}
	if seen[enumerationDisplayColumnPackage] && !packageColumnHasValue {
		return enumerationDisplayExistingShape{}, false
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
	case strings.Contains(header, "包") ||
		strings.Contains(header, "模块") ||
		strings.Contains(header, "命名空间") ||
		strings.Contains(header, "package") ||
		strings.Contains(header, "module") ||
		strings.Contains(header, "namespace"):
		return enumerationDisplayColumnPackage
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
		if role == enumerationDisplayColumnPackage {
			continue
		}
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
	case enumerationDisplayColumnPackage:
		return enumerationDisplayPackageCell(row)
	default:
		return ""
	}
}

func enumerationDisplayShapeHasTypedAttributeColumn(shape enumerationDisplayExistingShape) bool {
	for _, role := range shape.roles {
		if role == enumerationDisplayColumnPackage {
			return true
		}
	}
	return false
}

func enumerationDisplayShapeCanCompileInline(shape enumerationDisplayExistingShape, block types.AnswerBlock, citations []types.Citation) bool {
	if enumerationDisplayShapeHasTypedAttributeColumn(shape) {
		return true
	}
	if !enumerationDisplayShapeHasLocationColumn(shape) {
		return false
	}
	return enumerationDisplayBlockColumnsHaveLocationHeader(block.Columns) &&
		enumerationDisplayBlockRowsAllCitationBacked(block, citations)
}

func enumerationDisplayShapeHasLocationColumn(shape enumerationDisplayExistingShape) bool {
	for _, role := range shape.roles {
		if role == enumerationDisplayColumnLocation {
			return true
		}
	}
	return false
}

func enumerationDisplayBlockColumnsHaveLocationHeader(columns []string) bool {
	for _, column := range columns {
		if enumerationDisplayColumnRoleForHeader(column) == enumerationDisplayColumnLocation {
			return true
		}
	}
	return false
}

func enumerationDisplayBlockRowsAllCitationBacked(block types.AnswerBlock, citations []types.Citation) bool {
	visible := 0
	for _, item := range block.Items {
		if strings.TrimSpace(item.Label) == "" &&
			strings.TrimSpace(item.Text) == "" &&
			len(nonEmptyAnswerTableCompileCells(item.Cells)) == 0 {
			continue
		}
		visible++
		if item.CitationRef < 0 || item.CitationRef >= len(citations) {
			return false
		}
		cit := citations[item.CitationRef]
		if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
			return false
		}
	}
	return visible > 0
}

func enumerationDisplayTableNeedsNoteColumn(block types.AnswerBlock, rows []types.EnumerationDisplayRow, shape enumerationDisplayExistingShape) bool {
	for _, role := range shape.roles {
		if role == enumerationDisplayColumnNote {
			return false
		}
	}
	for idx, item := range block.Items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		note := strings.TrimSpace(item.Text)
		if idx < len(rows) {
			note = answerTableCompileFirstNonEmptyString(item.Text, rows[idx].Note)
		}
		if note == "" {
			return false
		}
		return true
	}
	return false
}

func enumerationDisplayDefaultNoteColumn(ctx *types.BusContext) string {
	if ctx != nil && ctx.AnalysisIR != nil && strings.EqualFold(strings.TrimSpace(ctx.AnalysisIR.RequestModel.Language), "zh") {
		return "说明"
	}
	return "Notes"
}

func enumerationDisplayPackageCell(row types.EnumerationDisplayRow) string {
	var values []string
	seen := map[string]bool{}
	for _, attr := range row.Attributes {
		if attr.Role != types.AnswerCandidateRolePackage {
			continue
		}
		value := strings.TrimSpace(attr.Name)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return strings.Join(values, ", ")
}

func normalizeEnumerationDisplayLocationKey(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if raw == "" {
		return ""
	}
	for strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	return strings.ToLower(raw)
}

func answerTableCompileLocationKey(source string, line int) string {
	source = strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
	if source == "" || line <= 0 {
		return ""
	}
	return source + ":" + strconv.Itoa(line)
}

func answerTableCompileCitationLocationKey(cit types.Citation) string {
	return answerTableCompileLocationKey(cit.File, cit.Line)
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
