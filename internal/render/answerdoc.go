package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderAnswerDocument renders an AnswerDocumentV2 (block-only
// carrier) into the user-visible markdown string. B5 落地. The
// renderer iterates blocks in declared order; each block kind has a
// dedicated helper. Citations + Snippets blocks render after the
// main blocks, mirroring V1's RenderAnswerDocument tail.
//
// Per docs/migration/block_only_carrier.md §5.5 design:
//   - NO shape switch — the renderer never reads doc.Shape (V2 has
//     none); it dispatches on per-block Kind.
//   - The renderer NEVER writes to the document — output is the
//     final string only. Per the feedback_no_system_backfill_to_user
//     _panel red line, we cannot mutate doc.Blocks even to upgrade
//     missing block IDs.
//   - Caveat / surface_role / claim_uses are read for display
//     decoration but never modified.
func RenderAnswerDocument(doc *types.AnswerDocumentV2, lang string) string {
	if doc == nil {
		return ""
	}
	docLang := normalizeAnswerDocLang(lang)
	structuredDiagramBodies := answerDocumentStructuredDiagramBodyKeys(doc)
	var b strings.Builder

	for _, blk := range doc.Blocks {
		blk = stripDuplicateStructuredDiagramFencesFromBlock(blk, structuredDiagramBodies)
		renderAnswerDocV2Block(&b, blk, doc, docLang)
	}

	if len(doc.MissingRequestedRoles) > 0 {
		renderAnswerDocV2MissingRequestedRoles(&b, doc.MissingRequestedRoles, docLang)
	}

	if len(doc.Caveats) > 0 {
		renderAnswerDocV2Caveats(&b, doc.Caveats, docLang)
	}

	// Reuse V1's citation pool + snippet renderers; both already
	// take their input by Citation / CodeSnippet slice — they don't
	// care about V1 vs V2 docs.
	if len(doc.Citations) > 0 {
		renderAnswerDocV2Citations(&b, doc.Citations, docLang)
	}
	if len(doc.Snippets) > 0 {
		renderAnswerDocV2Snippets(&b, doc.Snippets, docLang)
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// RenderAnswerDocumentWithAttachments renders the structured V2 answer
// plus best-effort visible fragments recovered from malformed model
// output. Attachments are appended after the validated answer so they
// never masquerade as citation-checked blocks, but model-authored
// content that would otherwise be lost remains inspectable.
func RenderAnswerDocumentWithAttachments(doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, lang string) string {
	base := RenderAnswerDocument(doc, lang)
	extra := renderAnswerDisplayAttachments(doc, attachments, normalizeAnswerDocLang(lang))
	if strings.TrimSpace(extra) == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return extra
	}
	return strings.TrimRight(base, "\n") + "\n\n" + extra
}

// renderAnswerDocV2Block dispatches on block.Kind. Unknown / empty
// kind silently skips — schema validation (B3) already guarantees
// every block has a valid kind, so this branch is a defensive
// no-op only.
func renderAnswerDocV2Block(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	switch blk.Kind {
	case types.BlockSummary:
		renderV2BlockSummary(b, blk, lang)
	case types.BlockSection:
		renderV2BlockSection(b, blk, doc, lang)
	case types.BlockOrderedList:
		renderV2BlockOrderedList(b, blk, doc, lang)
	case types.BlockBulletList:
		renderV2BlockBulletList(b, blk, doc, lang)
	case types.BlockScalar:
		renderV2BlockScalar(b, blk, doc, lang)
	case types.BlockDecision:
		renderV2BlockDecision(b, blk, doc, lang)
	case types.BlockTable:
		renderV2BlockTable(b, blk, doc, lang)
	case types.BlockDiagram:
		renderV2BlockDiagram(b, blk, lang)
	case types.BlockCaveat:
		renderV2BlockCaveat(b, blk, lang)
	}
}

func renderV2BlockSummary(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "## %s\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
}

func renderV2BlockSection(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	heading := strings.TrimSpace(blk.Title)
	if heading == "" {
		// A Section without an explicit Title is rendered without a
		// heading line; the body still appears.
	} else {
		fmt.Fprintf(b, "### %s\n\n", heading)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	rendered := 0
	for _, it := range blk.Items {
		// Skip items that contribute no user-visible prose. Some LLM
		// emits attach typed claim annotations (items[i].claim_use)
		// without Label/Text — those are contract-layer signals, not
		// answer prose. Without this guard the renderer prints "- "
		// per signal-only item.
		s := renderV2BlockItem(it, doc, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", s)
		rendered++
	}
	if rendered > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockOrderedList(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	idx := 0
	for _, it := range blk.Items {
		s := renderV2BlockItem(it, doc, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		idx++
		fmt.Fprintf(b, "%d. %s\n", idx, s)
	}
	if idx > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockBulletList(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	rendered := 0
	for _, it := range blk.Items {
		s := renderV2BlockItem(it, doc, lang)
		if strings.TrimSpace(s) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", s)
		rendered++
	}
	if rendered > 0 {
		b.WriteString("\n")
	}
}

func renderV2BlockScalar(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	prefix := "Value:"
	if lang == answerDocLangZH {
		prefix = "值："
	}
	literal := renderUserSurfaceText(blk.Text)
	if len(blk.Items) > 0 && literal == "" {
		// Scalar may use first item's Label as literal when Text is
		// empty (B3 schema accepts both shapes).
		literal = renderUserSurfaceText(blk.Items[0].Label)
	}
	if literal == "" {
		return
	}
	fmt.Fprintf(b, "**%s** `%s`", prefix, literal)
	cite := blockTopCitation(blk, doc)
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "*%s*\n\n", blk.Title)
	}
}

func renderV2BlockDecision(b *strings.Builder, blk types.AnswerBlock, doc *types.AnswerDocumentV2, lang answerDocLang) {
	prefix := "Decision:"
	if lang == answerDocLangZH {
		prefix = "结论："
	}
	body := renderUserSurfaceText(blk.Text)
	if blk.CurrentStatusVerdict != "" {
		verdict := "`" + string(blk.CurrentStatusVerdict) + "`"
		if body != "" {
			body = verdict + " — " + body
		} else {
			body = verdict
		}
	}
	if blk.ErrorGranularityVerdict != "" {
		verdict := "`" + string(blk.ErrorGranularityVerdict) + "`"
		if body != "" {
			body = verdict + " — " + body
		} else {
			body = verdict
		}
	}
	fmt.Fprintf(b, "**%s** %s", prefix, body)
	cite := blockTopCitation(blk, doc)
	if cite != "" {
		fmt.Fprintf(b, " (%s)", cite)
	}
	b.WriteString("\n\n")
}

func renderV2BlockTable(b *strings.Builder, blk types.AnswerBlock, _ *types.AnswerDocumentV2, lang answerDocLang) {
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderV2TableText(blk); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
		return
	}
	if len(blk.Items) == 0 {
		return
	}
	if renderV2StructuredTable(b, blk, lang) {
		return
	}
	rendered := 0
	for _, it := range blk.Items {
		if strings.TrimSpace(it.Label) == "" && strings.TrimSpace(it.Text) == "" {
			continue
		}
		rendered++
	}
	if rendered == 0 {
		return
	}
	labelHeader, detailHeader := renderV2TableFallbackHeaders(lang)
	fmt.Fprintf(b, "| %s | %s |\n|---|---|\n", labelHeader, detailHeader)
	for _, it := range blk.Items {
		label := strings.TrimSpace(it.Label)
		text := strings.TrimSpace(it.Text)
		if label == "" && text == "" {
			continue
		}
		fmt.Fprintf(b, "| %s | %s |\n", escapePipe(label), escapePipe(text))
	}
	b.WriteString("\n")
}

func renderV2TableText(blk types.AnswerBlock) string {
	text := renderUserSurfaceText(blk.Text)
	if text != "" {
		return text
	}
	var parts []string
	seen := make(map[string]bool)
	for _, it := range blk.Items {
		for _, raw := range []string{it.Label, it.Text} {
			candidate := renderUserSurfaceText(raw)
			if candidate == "" || !looksLikeMarkdownTable(candidate) {
				continue
			}
			key := strings.TrimSpace(candidate)
			if seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, candidate)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

func looksLikeMarkdownTable(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return false
	}
	for i := 0; i+1 < len(lines); i++ {
		header := strings.TrimSpace(lines[i])
		separator := strings.TrimSpace(lines[i+1])
		if header == "" || separator == "" || !strings.Contains(header, "|") {
			continue
		}
		if isMarkdownTableSeparator(separator) {
			return true
		}
	}
	return false
}

func isMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !strings.Contains(line, "-") {
		return false
	}
	line = strings.Trim(line, "|")
	cells := strings.Split(line, "|")
	valid := 0
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		cell = strings.Trim(cell, ":")
		if len(cell) < 3 {
			return false
		}
		for _, r := range cell {
			if r != '-' {
				return false
			}
		}
		valid++
	}
	return valid >= 2
}

func renderV2StructuredTable(b *strings.Builder, blk types.AnswerBlock, lang answerDocLang) bool {
	columns := renderV2NormalizeTableStrings(blk.Columns)
	hasStructuredCarrier := len(columns) > 0
	rows := make([]renderV2TableRow, 0, len(blk.Items))
	hasLabel := false
	maxCells := 0
	for _, it := range blk.Items {
		label := strings.TrimSpace(it.Label)
		if len(renderV2NormalizeTableStrings(it.Cells)) > 0 {
			hasStructuredCarrier = true
		}
		cells := renderV2TableItemCells(it)
		if label == "" && len(cells) == 0 {
			continue
		}
		if label != "" {
			hasLabel = true
		}
		if len(cells) > maxCells {
			maxCells = len(cells)
		}
		rows = append(rows, renderV2TableRow{label: label, cells: cells})
	}
	if !hasStructuredCarrier || len(rows) == 0 || (len(columns) == 0 && maxCells == 0) {
		return false
	}
	headers := renderV2StructuredTableHeaders(columns, hasLabel, maxCells, lang)
	if len(headers) == 0 {
		return false
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(renderV2EscapedTableCells(headers), " | "))
	b.WriteString("|")
	for range headers {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, row := range rows {
		cells := make([]string, 0, len(headers))
		if hasLabel {
			cells = append(cells, row.label)
		}
		cells = append(cells, row.cells...)
		for len(cells) < len(headers) {
			cells = append(cells, "")
		}
		if len(cells) > len(headers) {
			cells = cells[:len(headers)]
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(renderV2EscapedTableCells(cells), " | "))
	}
	b.WriteString("\n")
	return true
}

type renderV2TableRow struct {
	label string
	cells []string
}

func renderV2TableItemCells(it types.AnswerBlockItem) []string {
	cells := renderV2NormalizeTableStrings(it.Cells)
	text := strings.TrimSpace(it.Text)
	if len(cells) == 0 {
		if text == "" {
			return nil
		}
		return []string{text}
	}
	if text != "" && !renderV2TableCellsContain(cells, text) {
		cells = append(cells, text)
	}
	return cells
}

func renderV2TableCellsContain(cells []string, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	for _, cell := range cells {
		if strings.TrimSpace(cell) == text {
			return true
		}
	}
	return false
}

func renderV2StructuredTableHeaders(columns []string, hasLabel bool, maxCells int, lang answerDocLang) []string {
	if hasLabel {
		switch {
		case len(columns) == maxCells+1:
			return renderV2FillTableHeaders(columns, len(columns), lang, true)
		case len(columns) >= maxCells+1 && maxCells > 0:
			return renderV2FillTableHeaders(columns[:maxCells+1], maxCells+1, lang, true)
		case len(columns) > 0:
			headers := append([]string{renderV2TableRowHeader(lang)}, columns...)
			return renderV2FillTableHeaders(headers, maxCells+1, lang, true)
		default:
			return renderV2FillTableHeaders([]string{renderV2TableRowHeader(lang)}, maxCells+1, lang, true)
		}
	}
	width := maxCells
	if len(columns) > width {
		width = len(columns)
	}
	return renderV2FillTableHeaders(columns, width, lang, false)
}

func renderV2FillTableHeaders(headers []string, width int, lang answerDocLang, hasLabel bool) []string {
	if width <= 0 {
		return nil
	}
	out := append([]string(nil), headers...)
	for len(out) < width {
		out = append(out, renderV2SyntheticTableHeader(len(out), width, lang, hasLabel))
	}
	for i, h := range out {
		if strings.TrimSpace(h) == "" {
			out[i] = renderV2SyntheticTableHeader(i, width, lang, hasLabel)
		}
	}
	return out
}

func renderV2SyntheticTableHeader(index, width int, lang answerDocLang, hasLabel bool) string {
	if hasLabel && index == 0 {
		return renderV2TableRowHeader(lang)
	}
	if width == 2 && hasLabel && index == 1 {
		_, detail := renderV2TableFallbackHeaders(lang)
		return detail
	}
	if lang == answerDocLangZH {
		return fmt.Sprintf("列 %d", index+1)
	}
	return fmt.Sprintf("Column %d", index+1)
}

func renderV2TableRowHeader(lang answerDocLang) string {
	label, _ := renderV2TableFallbackHeaders(lang)
	return label
}

func renderV2NormalizeTableStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderV2EscapedTableCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = escapePipe(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(cell), "\r\n", "\n"), "\n", "<br>"))
	}
	return out
}

func renderV2TableFallbackHeaders(lang answerDocLang) (string, string) {
	if lang == answerDocLangZH {
		return "项目", "说明"
	}
	return "Item", "Details"
}

func renderV2BlockDiagram(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	if blk.Diagram == nil {
		return
	}
	d := blk.Diagram
	body := normalizeDiagramBodyForRender(d.Body)
	if body == "" {
		return
	}
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "**%s**\n\n", blk.Title)
	}
	if text := renderUserSurfaceText(blk.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	lang := strings.TrimSpace(d.Language)
	if lang == "" {
		lang = "mermaid"
	}
	fmt.Fprintf(b, "```%s\n%s\n```\n\n", lang, body)
}

const (
	maxAnswerDisplayAttachments = 4
	maxAnswerDisplayBodyRunes   = 16000
)

func renderAnswerDisplayAttachments(doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, lang answerDocLang) string {
	if len(attachments) == 0 {
		return ""
	}
	seen := answerDocumentVisibleAttachmentKeys(doc, lang)
	var b strings.Builder
	rendered := 0
	for _, att := range attachments {
		if rendered >= maxAnswerDisplayAttachments {
			break
		}
		body := trimDisplayAttachmentBody(att.Body)
		if body == "" {
			continue
		}
		key := displayAttachmentKey(att.Kind, body)
		if seen[key] {
			continue
		}
		seen[key] = true
		if rendered == 0 {
			b.WriteString(displayAttachmentPanelStart(lang))
		}
		renderDisplayAttachment(&b, att, body, lang)
		rendered++
	}
	if rendered == 0 {
		return ""
	}
	b.WriteString(displayAttachmentPanelEnd())
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func answerDocumentVisibleAttachmentKeys(doc *types.AnswerDocumentV2, lang answerDocLang) map[string]bool {
	seen := map[string]bool{}
	if doc == nil {
		return seen
	}
	if full := strings.TrimSpace(RenderAnswerDocument(doc, answerDocLangCode(lang))); full != "" {
		seen[displayAttachmentKey(types.AnswerDisplayAttachmentMarkdown, full)] = true
		seen[displayAttachmentKey(types.AnswerDisplayAttachmentText, full)] = true
	}
	for _, blk := range doc.Blocks {
		if blk.Diagram != nil {
			body := normalizeDiagramBodyForRender(blk.Diagram.Body)
			if body != "" {
				seen[displayAttachmentKey(types.AnswerDisplayAttachmentDiagram, body)] = true
			}
		}
		if text := strings.TrimSpace(blk.Text); text != "" {
			seen[displayAttachmentKey(types.AnswerDisplayAttachmentMarkdown, text)] = true
			seen[displayAttachmentKey(types.AnswerDisplayAttachmentText, text)] = true
		}
	}
	return seen
}

func answerDocLangCode(lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "zh"
	}
	return "en"
}

func answerDocumentStructuredDiagramBodyKeys(doc *types.AnswerDocumentV2) map[string]bool {
	seen := map[string]bool{}
	if doc == nil {
		return seen
	}
	for _, blk := range doc.Blocks {
		if blk.Kind != types.BlockDiagram || blk.Diagram == nil {
			continue
		}
		if key := diagramBodyDedupKey(blk.Diagram.Body); key != "" {
			seen[key] = true
		}
	}
	return seen
}

func stripDuplicateStructuredDiagramFencesFromBlock(blk types.AnswerBlock, diagramBodies map[string]bool) types.AnswerBlock {
	if len(diagramBodies) == 0 {
		return blk
	}
	blk.Text = stripDuplicateStructuredDiagramFences(blk.Text, diagramBodies)
	for i := range blk.Items {
		blk.Items[i].Label = stripDuplicateStructuredDiagramFences(blk.Items[i].Label, diagramBodies)
		blk.Items[i].Text = stripDuplicateStructuredDiagramFences(blk.Items[i].Text, diagramBodies)
	}
	return blk
}

func stripDuplicateStructuredDiagramFences(text string, diagramBodies map[string]bool) string {
	if strings.TrimSpace(text) == "" || len(diagramBodies) == 0 || !strings.Contains(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") {
			out = append(out, line)
			i++
			continue
		}
		info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "```" {
				end = j
				break
			}
		}
		if end < 0 {
			out = append(out, line)
			i++
			continue
		}
		body := strings.Join(lines[i+1:end], "\n")
		if answerDocFenceCanDedupStructuredDiagram(info) && diagramBodies[diagramBodyDedupKey(body)] {
			i = end + 1
			continue
		}
		out = append(out, lines[i:end+1]...)
		i = end + 1
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func answerDocFenceCanDedupStructuredDiagram(info string) bool {
	info = strings.TrimSpace(info)
	if info == "" {
		return true
	}
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return true
	}
	return strings.EqualFold(fields[0], "mermaid")
}

func renderDisplayAttachment(b *strings.Builder, att types.AnswerDisplayAttachment, body string, lang answerDocLang) {
	title := strings.TrimSpace(att.Title)
	switch att.Kind {
	case types.AnswerDisplayAttachmentDiagram:
		body = normalizeDiagramBodyForRender(body)
		if body == "" {
			return
		}
		if title == "" {
			if lang == answerDocLangZH {
				title = "保留的图"
			} else {
				title = "Preserved diagram"
			}
		}
		fmt.Fprintf(b, "#### %s\n\n", title)
		fenceLang := strings.TrimSpace(att.Language)
		if fenceLang == "" {
			fenceLang = "mermaid"
		}
		fmt.Fprintf(b, "```%s\n%s\n```\n\n", fenceLang, body)
	default:
		if title == "" {
			if lang == answerDocLangZH {
				title = "保留的原文"
			} else {
				title = "Preserved text"
			}
		}
		fmt.Fprintf(b, "#### %s\n\n%s\n\n", title, renderUserSurfaceText(body))
	}
}

func displayAttachmentPanelStart(lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "---\n\n> **系统保留内容**\n>\n> 下面内容来自模型已生成但未能完整进入结构化答案的输出，系统按原文保留展示；这部分不是上方已校验结构化答案的主体，请按补充参考阅读。\n\n"
	}
	return "---\n\n> **System-preserved content**\n>\n> The following content was generated by the model but could not be fully preserved in the structured answer. It is shown verbatim as supplemental reference, not as part of the validated structured answer above.\n\n"
}

func displayAttachmentPanelEnd() string {
	return "---\n\n"
}

func trimDisplayAttachmentBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	rs := []rune(body)
	if len(rs) <= maxAnswerDisplayBodyRunes {
		return body
	}
	return string(rs[:maxAnswerDisplayBodyRunes]) + "\n..."
}

func displayAttachmentKey(kind, body string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.TrimSpace(body)
}

func normalizeDiagramBodyForRender(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if !strings.HasPrefix(body, "```") {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		return body
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(first, "```") || last != "```" {
		return body
	}
	inner := strings.Join(lines[1:len(lines)-1], "\n")
	return strings.TrimSpace(inner)
}

func diagramBodyDedupKey(body string) string {
	body = normalizeDiagramBodyForRender(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimSpace(body)
}

func renderV2BlockCaveat(b *strings.Builder, blk types.AnswerBlock, _ answerDocLang) {
	body := renderUserSurfaceText(blk.Text)
	if body == "" {
		return
	}
	// Caveat blocks are rendered with a leading marker so the user
	// can spot them at a glance. Mirror the docs/architecture.md
	// guidance: caveats are out-of-band notes, not principal answer.
	if strings.TrimSpace(blk.Title) != "" {
		fmt.Fprintf(b, "> **%s** %s\n\n", blk.Title, body)
		return
	}
	fmt.Fprintf(b, "> %s\n\n", body)
}

// renderV2BlockItem returns the inline string for one item: Label
// + optional Text + optional citation marker. Used by ordered /
// bullet / section lists.
func renderV2BlockItem(it types.AnswerBlockItem, doc *types.AnswerDocumentV2, _ answerDocLang) string {
	parts := make([]string, 0, 3)
	if l := renderUserSurfaceText(it.Label); l != "" {
		parts = append(parts, "**"+l+"**")
	}
	if t := renderUserSurfaceText(it.Text); t != "" {
		parts = append(parts, t)
	}
	out := strings.Join(parts, " — ")
	if doc != nil && it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
		cite := renderCitationDisplay(doc.Citations[it.CitationRef])
		if cite != "" {
			out = out + " (" + cite + ")"
		}
	}
	return out
}

// blockTopCitation pulls the first valid citation reference from a
// block's items[] / claim_uses[]. Returns "" when no usable cite
// exists.
func blockTopCitation(blk types.AnswerBlock, doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	for _, it := range blk.Items {
		if it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) {
			return renderCitationDisplay(doc.Citations[it.CitationRef])
		}
	}
	return ""
}

func renderAnswerDocV2Caveats(b *strings.Builder, caveats []string, lang answerDocLang) {
	heading := "**Caveats:**"
	if lang == answerDocLangZH {
		heading = "**说明**："
	}
	fmt.Fprintf(b, "\n%s\n\n", heading)
	for _, c := range caveats {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(c))
	}
}

func renderAnswerDocV2MissingRequestedRoles(b *strings.Builder, roles []types.AnswerMissingRequestedRole, lang answerDocLang) {
	if len(roles) == 0 {
		return
	}
	if lang == answerDocLangZH {
		b.WriteString("\n**缺失的请求层**：\n\n")
	} else {
		b.WriteString("\n**Missing requested layers:**\n\n")
	}
	for _, role := range roles {
		line := renderMissingRequestedRoleLine(role, lang)
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", line)
	}
	b.WriteString("\n")
}

func renderMissingRequestedRoleLine(role types.AnswerMissingRequestedRole, lang answerDocLang) string {
	label := strings.TrimSpace(role.Label)
	switch role.Role {
	case types.EvidenceDiagramRoleConfig:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供匹配的配置项。", label)
			}
			return "配置文件层没有为这个精确目标提供匹配的配置项。"
		}
		if label != "" {
			return fmt.Sprintf("No %s entry binds this exact target.", label)
		}
		return "No config-file entry binds this exact target."
	case types.EvidenceDiagramRoleOverride:
		if strings.EqualFold(label, "cli") {
			if lang == answerDocLangZH {
				return "CLI 标志或命令行覆盖层未绑定这个精确目标。"
			}
			return "No CLI flag or command-line override binding exists for this exact target."
		}
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 覆盖层未绑定这个精确目标。", label)
			}
			return "覆盖层未绑定这个精确目标。"
		}
		if label != "" {
			return fmt.Sprintf("No %s override binding exists for this exact target.", label)
		}
		return "No override binding exists for this exact target."
	case types.EvidenceDiagramRoleDefault:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供默认绑定。", label)
			}
			return "代码默认层没有为这个精确目标提供绑定。"
		}
		if label != "" {
			return fmt.Sprintf("No %s binding exists for this exact target.", label)
		}
		return "No code-default binding exists for this exact target."
	case types.EvidenceDiagramRoleRuntime:
		if lang == answerDocLangZH {
			if label != "" {
				return fmt.Sprintf("%s 层没有为这个精确目标提供运行时绑定。", label)
			}
			return "运行时层没有为这个精确目标提供绑定。"
		}
		if label != "" {
			return fmt.Sprintf("No %s binding exists for this exact target.", label)
		}
		return "No runtime binding exists for this exact target."
	default:
		return ""
	}
}

func renderAnswerDocV2Citations(b *strings.Builder, citations []types.Citation, lang answerDocLang) {
	if lang == answerDocLangZH {
		b.WriteString("\n**引用**：\n\n")
	} else {
		b.WriteString("\n**Citations:**\n\n")
	}
	for _, c := range citations {
		fmt.Fprintf(b, "- %s\n", renderCitationDisplay(c))
	}
}

func renderAnswerDocV2Snippets(b *strings.Builder, snippets []types.CodeSnippet, lang answerDocLang) {
	if lang == answerDocLangZH {
		b.WriteString("\n**关键代码**：\n\n")
	} else {
		b.WriteString("\n**Key snippets:**\n\n")
	}
	for _, s := range snippets {
		header := s.File
		if s.StartLine > 0 {
			header = fmt.Sprintf("%s:%d", s.File, s.StartLine)
			if s.EndLine > s.StartLine {
				header = fmt.Sprintf("%s-%d", header, s.EndLine)
			}
		}
		fmt.Fprintf(b, "📄 **`%s`**\n\n```%s\n%s\n```\n\n", header, s.Language, s.Code)
	}
}

// escapePipe replaces unescaped pipe characters in markdown table
// cells so a Label / Text containing "|" doesn't break the table.
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func renderUserSurfaceText(s string) string {
	s = StripAuthorityArtifactsForRender(s)
	return strings.TrimSpace(s)
}
