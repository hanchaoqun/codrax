package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/mattn/go-runewidth"
)

const (
	evidenceSummaryMaxItems = 3
	evidenceSummaryMaxCols  = 150
)

var emitEvidenceAcceptedRe = regexp.MustCompile(`^emit_evidence accepted ([0-9]+) item`)

type renderedEvidenceItem struct {
	Anchor   string
	Location string
	Semantic string
	Status   string
}

func formatStructuredToolResultSummary(toolName, paramsJSON, resultSummary, lang string) string {
	switch strings.TrimSpace(toolName) {
	case "emit_evidence":
		return formatEvidenceToolResultSummary(toolName, resultSummary, lang)
	case "emit_analysis":
		return formatAnalysisToolResultSummary(paramsJSON, resultSummary, lang)
	case "emit_answer_document", "emit_answer_document_patch":
		return formatAnswerDocumentToolResultSummary(toolName, resultSummary, lang)
	default:
		return ""
	}
}

type answerDraftPreviewDoc struct {
	Blocks []answerDraftPreviewBlock `json:"blocks"`
}

type answerDraftPreviewBlock struct {
	ID      string                     `json:"id"`
	Kind    string                     `json:"kind"`
	Title   string                     `json:"title"`
	Text    string                     `json:"text"`
	Columns []string                   `json:"columns"`
	Items   []answerDraftPreviewItem   `json:"items"`
	Diagram *answerDraftPreviewDiagram `json:"diagram"`
}

type answerDraftPreviewItem struct {
	Label string   `json:"label"`
	Text  string   `json:"text"`
	Cells []string `json:"cells"`
}

type answerDraftPreviewDiagram struct {
	Kind     string `json:"kind"`
	Language string `json:"language"`
	Body     string `json:"body"`
}

func formatAnswerDocumentDraftPreviewLines(paramsJSON, lang string) []scrollbackLine {
	doc, ok := decodeAnswerDraftPreviewDoc(paramsJSON)
	if !ok || len(doc.Blocks) == 0 {
		return nil
	}
	zh := isZh(lang)
	header := "First draft answer"
	if zh {
		header = "第一稿答案"
	}
	lines := []scrollbackLine{{
		text: "  " + statusObjective.Sprint("•") + " " + statusObjective.Sprint(header),
	}}
	for _, block := range doc.Blocks {
		lines = appendAnswerDraftBlockLines(lines, block, zh)
	}
	return trimScrollbackPreviewBlankLines(lines)
}

func decodeAnswerDraftPreviewDoc(paramsJSON string) (answerDraftPreviewDoc, bool) {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return answerDraftPreviewDoc{}, false
	}
	for _, candidate := range toolparam.JSONRepairCandidates(paramsJSON) {
		if doc, ok := decodeAnswerDraftPreviewDocCandidate(candidate); ok {
			return doc, true
		}
	}
	return answerDraftPreviewDoc{}, false
}

func decodeAnswerDraftPreviewDocCandidate(paramsJSON string) (answerDraftPreviewDoc, bool) {
	var doc answerDraftPreviewDoc
	if json.Unmarshal([]byte(paramsJSON), &doc) == nil && len(doc.Blocks) > 0 {
		return doc, true
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(paramsJSON), &raw) != nil {
		return answerDraftPreviewDoc{}, false
	}
	if blocks, ok := decodeAnswerDraftPreviewBlocks(raw["blocks"]); ok {
		doc.Blocks = blocks
		return doc, true
	}
	return answerDraftPreviewDoc{}, false
}

func decodeAnswerDraftPreviewBlocks(raw json.RawMessage) ([]answerDraftPreviewBlock, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var blocks []answerDraftPreviewBlock
	if json.Unmarshal(raw, &blocks) == nil && len(blocks) > 0 {
		return blocks, true
	}
	var wrapped string
	if json.Unmarshal(raw, &wrapped) != nil {
		return nil, false
	}
	wrapped = strings.TrimSpace(wrapped)
	if wrapped == "" {
		return nil, false
	}
	if json.Unmarshal([]byte(wrapped), &blocks) == nil && len(blocks) > 0 {
		return blocks, true
	}
	var doc answerDraftPreviewDoc
	if json.Unmarshal([]byte(wrapped), &doc) == nil && len(doc.Blocks) > 0 {
		return doc.Blocks, true
	}
	for _, candidate := range toolparam.JSONRepairCandidates(wrapped) {
		if candidate == wrapped {
			continue
		}
		if json.Unmarshal([]byte(candidate), &blocks) == nil && len(blocks) > 0 {
			return blocks, true
		}
		if json.Unmarshal([]byte(candidate), &doc) == nil && len(doc.Blocks) > 0 {
			return doc.Blocks, true
		}
	}
	return nil, false
}

func appendAnswerDraftBlockLines(lines []scrollbackLine, block answerDraftPreviewBlock, zh bool) []scrollbackLine {
	title := strings.TrimSpace(block.Title)
	if title != "" {
		lines = appendPlainAnswerDraftLines(lines, title)
	}
	if strings.TrimSpace(block.Text) != "" {
		lines = appendPlainAnswerDraftLines(lines, block.Text)
	}
	if len(block.Items) > 0 {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "table") && strings.TrimSpace(block.Text) == "" {
			if table, ok := formatAnswerDraftStructuredTable(block, zh); ok {
				lines = appendPlainAnswerDraftLines(lines, table)
				return lines
			}
		}
		for i, item := range block.Items {
			text := formatAnswerDraftItem(i+1, item)
			if text == "" {
				continue
			}
			lines = appendPlainAnswerDraftLines(lines, text)
		}
	}
	if block.Diagram != nil && strings.TrimSpace(block.Diagram.Body) != "" {
		lines = appendAnswerDraftDiagramLines(lines, *block.Diagram)
	}
	return lines
}

func appendPlainAnswerDraftLines(lines []scrollbackLine, text string) []scrollbackLine {
	text = expandInlineReasoningVisualRows(text)
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	tableMask := MarkdownTableLineMask(raw)
	for i, rawLine := range raw {
		line := strings.TrimRight(rawLine, " \r")
		if strings.TrimSpace(stripAnsiEscapes(line)) == "" {
			lines = append(lines, scrollbackLine{text: "    "})
			continue
		}
		visual := tableMask[i] || ShouldPreserveVisualLine(line)
		lines = append(lines, scrollbackLine{
			text:   "    " + statusReasoningBody.Sprint(line),
			visual: visual,
		})
	}
	return lines
}

func appendAnswerDraftDiagramLines(lines []scrollbackLine, diagram answerDraftPreviewDiagram) []scrollbackLine {
	language := strings.TrimSpace(diagram.Language)
	if language == "" {
		language = "mermaid"
	}
	lines = append(lines, scrollbackLine{text: "    " + statusReasoningBody.Sprint("```"+language), visual: true})
	for _, rawLine := range strings.Split(strings.ReplaceAll(diagram.Body, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(rawLine, " \r")
		lines = append(lines, scrollbackLine{
			text:   "    " + statusReasoningBody.Sprint(line),
			visual: true,
		})
	}
	lines = append(lines, scrollbackLine{text: "    " + statusReasoningBody.Sprint("```"), visual: true})
	return lines
}

func formatAnswerDraftItem(index int, item answerDraftPreviewItem) string {
	label := strings.TrimSpace(item.Label)
	text := strings.TrimSpace(item.Text)
	cells := trimAnswerDraftStringSlice(item.Cells)
	if len(cells) > 0 {
		row := strings.Join(cells, " | ")
		if label != "" {
			row = label + " | " + row
		}
		if text != "" {
			row = row + " | " + text
		}
		return fmt.Sprintf("%d. %s", index, row)
	}
	switch {
	case label != "" && text != "":
		return fmt.Sprintf("%d. %s — %s", index, label, text)
	case label != "":
		return fmt.Sprintf("%d. %s", index, label)
	case text != "":
		return fmt.Sprintf("%d. %s", index, text)
	default:
		return ""
	}
}

func formatAnswerDraftStructuredTable(block answerDraftPreviewBlock, zh bool) (string, bool) {
	columns := trimAnswerDraftStringSlice(block.Columns)
	hasStructured := len(columns) > 0
	hasLabel := false
	maxCells := 0
	type row struct {
		label string
		cells []string
	}
	var rows []row
	for _, item := range block.Items {
		label := strings.TrimSpace(item.Label)
		cells := trimAnswerDraftStringSlice(item.Cells)
		if len(cells) > 0 {
			hasStructured = true
		}
		text := strings.TrimSpace(item.Text)
		if len(cells) == 0 && text != "" && len(columns) > 0 {
			cells = []string{text}
		} else if len(cells) > 0 && text != "" {
			cells = append(cells, text)
		}
		if label == "" && len(cells) == 0 {
			continue
		}
		if label != "" {
			hasLabel = true
		}
		if len(cells) > maxCells {
			maxCells = len(cells)
		}
		rows = append(rows, row{label: label, cells: cells})
	}
	if !hasStructured || len(rows) == 0 {
		return "", false
	}
	headers := answerDraftTableHeaders(columns, hasLabel, maxCells, zh)
	if len(headers) == 0 {
		return "", false
	}
	renderRows := make([][]string, 0, len(rows))
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
		renderRows = append(renderRows, cells)
	}
	headers, renderRows = compactAnswerDraftEmptyTableColumns(headers, renderRows)
	if len(headers) == 0 {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "| %s |\n", strings.Join(escapeAnswerDraftTableCells(headers), " | "))
	b.WriteString("|")
	for range headers {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, cells := range renderRows {
		fmt.Fprintf(&b, "| %s |\n", strings.Join(escapeAnswerDraftTableCells(cells), " | "))
	}
	return strings.TrimRight(b.String(), "\n"), true
}

func answerDraftTableHeaders(columns []string, hasLabel bool, maxCells int, zh bool) []string {
	itemHeader, detailHeader := "Item", "Details"
	if zh {
		itemHeader, detailHeader = "项目", "说明"
	}
	if hasLabel {
		if len(columns) == maxCells+1 {
			return answerDraftFillHeaders(columns, len(columns), zh, hasLabel)
		}
		if maxCells == 0 && len(columns) > 0 {
			return answerDraftFillHeaders(columns[:1], 1, zh, hasLabel)
		}
		headers := append([]string{itemHeader}, columns...)
		if len(headers) == 1 && maxCells == 1 {
			headers = append(headers, detailHeader)
		}
		return answerDraftFillHeaders(headers, maxCells+1, zh, hasLabel)
	}
	width := maxCells
	if len(columns) > width {
		width = len(columns)
	}
	return answerDraftFillHeaders(columns, width, zh, hasLabel)
}

func compactAnswerDraftEmptyTableColumns(headers []string, rows [][]string) ([]string, [][]string) {
	if len(headers) == 0 || len(rows) == 0 {
		return headers, rows
	}
	keep := make([]bool, len(headers))
	kept := 0
	for col := range headers {
		for _, row := range rows {
			if col < len(row) && strings.TrimSpace(row[col]) != "" {
				keep[col] = true
				kept++
				break
			}
		}
	}
	if kept == 0 {
		keep[0] = true
		kept = 1
	}
	if kept == len(headers) {
		return headers, rows
	}
	outHeaders := make([]string, 0, kept)
	for col, h := range headers {
		if keep[col] {
			outHeaders = append(outHeaders, h)
		}
	}
	outRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		out := make([]string, 0, kept)
		for col := range headers {
			if !keep[col] {
				continue
			}
			if col < len(row) {
				out = append(out, row[col])
			} else {
				out = append(out, "")
			}
		}
		outRows = append(outRows, out)
	}
	return outHeaders, outRows
}

func answerDraftFillHeaders(headers []string, width int, zh bool, hasLabel bool) []string {
	if width <= 0 {
		return nil
	}
	out := append([]string(nil), headers...)
	for len(out) < width {
		n := len(out) + 1
		if zh {
			out = append(out, fmt.Sprintf("列 %d", n))
		} else {
			out = append(out, fmt.Sprintf("Column %d", n))
		}
	}
	if hasLabel && strings.TrimSpace(out[0]) == "" {
		if zh {
			out[0] = "项目"
		} else {
			out[0] = "Item"
		}
	}
	return out
}

func trimAnswerDraftStringSlice(in []string) []string {
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

func escapeAnswerDraftTableCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		cell = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(cell), "\r\n", "\n"), "\n", "<br>")
		out[i] = strings.ReplaceAll(cell, "|", "\\|")
	}
	return out
}

func trimScrollbackPreviewBlankLines(lines []scrollbackLine) []scrollbackLine {
	for len(lines) > 1 && strings.TrimSpace(stripAnsiEscapes(lines[len(lines)-1].text)) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func formatAnswerDocumentToolResultSummary(toolName, summary, lang string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName != "emit_answer_document" && toolName != "emit_answer_document_patch" {
		return ""
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	zh := isZh(lang)
	if strings.Contains(summary, " accepted: ") {
		return formatAnswerDocumentAcceptedSummary(summary, zh)
	}
	return formatAnswerDocumentRejectedSummary(summary, zh)
}

var answerDocAcceptedCountsRe = regexp.MustCompile(`blocks=([0-9]+).*citations=([0-9]+)`)
var answerDocSummaryListPrefixRe = regexp.MustCompile(`^[0-9]+[.)]\s+`)

func formatAnswerDocumentAcceptedSummary(summary string, zh bool) string {
	counts := answerDocAcceptedCountsRe.FindStringSubmatch(summary)
	detail := ""
	if len(counts) == 3 {
		if zh {
			detail = fmt.Sprintf("答案草稿已写入：%s 个区块 · %s 条引用", counts[1], counts[2])
		} else {
			detail = fmt.Sprintf("Answer draft written: %s block(s) · %s citation(s)", counts[1], counts[2])
		}
	} else if zh {
		detail = "答案草稿已写入"
	} else {
		detail = "Answer draft written"
	}
	return "  " + statusMeta.Sprint("•") + " " + statusMeta.Sprint(detail) + "\n"
}

func formatAnswerDocumentRejectedSummary(summary string, zh bool) string {
	items := answerDocumentRejectActions(summary, 3)
	if len(items) == 0 {
		first := firstNonEmptyLine(summary)
		if first == "" {
			return ""
		}
		items = append(items, first)
	}
	header := "Answer document check failed"
	if zh {
		header = "成文校验未通过"
	}
	lines := []string{"  " + statusMeta.Sprint("•") + " " + statusMeta.Sprint(header)}
	for i, item := range items {
		lines = appendWrappedStructuredToolSummaryLine(lines, fmt.Sprintf("    %d. ", i+1), item)
	}
	return strings.Join(lines, "\n") + "\n"
}

func appendWrappedStructuredToolSummaryLine(lines []string, prefix, body string) []string {
	raw := prefix + strings.TrimSpace(body)
	wrapped := wrapWithHangingIndent(raw, strings.Repeat(" ", len(prefix)), evidenceSummaryMaxCols)
	for _, line := range strings.Split(wrapped, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, statusReasoningBody.Sprint(line))
	}
	return lines
}

func wrapWithHangingIndent(s, hangingIndent string, maxCols int) string {
	if maxCols <= 0 {
		return s
	}
	leadingLen := len(s) - len(strings.TrimLeft(s, " \t"))
	leadingIndent := s[:leadingLen]
	words := strings.Fields(strings.TrimSpace(s))
	if len(words) == 0 {
		return s
	}
	lines := make([]string, 0, 2)
	current := leadingIndent + words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if runewidth.StringWidth(stripAnsiEscapes(candidate)) <= maxCols {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = hangingIndent + word
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

func answerDocumentRejectActions(summary string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	var currentField string
	for _, raw := range strings.Split(summary, "\n") {
		line := strings.TrimSpace(raw)
		line = answerDocSummaryListPrefixRe.ReplaceAllString(line, "")
		switch {
		case strings.HasPrefix(line, "Field:"):
			currentField = trimSummaryCodeMarkers(strings.TrimSpace(strings.TrimPrefix(line, "Field:")))
		case strings.HasPrefix(line, "Action:"):
			action := strings.TrimSpace(strings.TrimPrefix(line, "Action:"))
			if action == "" {
				continue
			}
			if currentField != "" {
				action = currentField + ": " + action
			}
			out = append(out, action)
			currentField = ""
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func trimSummaryCodeMarkers(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`")
	return strings.TrimSpace(s)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func formatEvidenceToolResultSummary(toolName, summary, lang string) string {
	if strings.TrimSpace(toolName) != "emit_evidence" {
		return ""
	}
	count, items := parseEmitEvidenceSummary(summary)
	if count == 0 && len(items) == 0 {
		return ""
	}
	if count == 0 {
		count = len(items)
	}
	zh := isZh(lang)
	var lines []string
	if zh {
		lines = append(lines, "  "+statusMeta.Sprint("•")+" "+statusMeta.Sprint(fmt.Sprintf("证据 %d 条", count)))
	} else {
		lines = append(lines, "  "+statusMeta.Sprint("•")+" "+statusMeta.Sprint(fmt.Sprintf("Evidence: %d item(s)", count)))
	}
	limit := len(items)
	if limit > evidenceSummaryMaxItems {
		limit = evidenceSummaryMaxItems
	}
	for i := 0; i < limit; i++ {
		item := items[i]
		line := fmt.Sprintf("    %d. %s %s @ %s", i+1, localizedEvidenceStatus(item.Status, lang), item.Anchor, item.Location)
		if item.Semantic != "" {
			line += " — " + item.Semantic
		}
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint(line), evidenceSummaryMaxCols))
	}
	if count > limit {
		more := count - limit
		if zh {
			lines = append(lines, statusMeta.Sprint(fmt.Sprintf("    … 还有 %d 条", more)))
		} else {
			lines = append(lines, statusMeta.Sprint(fmt.Sprintf("    … %d more", more)))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

type analysisSummaryPayload struct {
	Intent        string   `json:"intent"`
	Scenario      string   `json:"scenario"`
	Complexity    string   `json:"complexity"`
	QuestionKind  string   `json:"question_kind"`
	PredicateAxis string   `json:"predicate_axis"`
	Keywords      []string `json:"keywords"`
	Entities      []string `json:"entities"`
	SubTopics     []struct {
		Title    string   `json:"title"`
		Entities []string `json:"entities"`
	} `json:"sub_topics"`
	AnswerSubject *struct {
		Kind       string   `json:"kind"`
		EntityAxes []string `json:"entity_axes"`
	} `json:"answer_subject"`
	DiagramHint *struct {
		Kind string `json:"kind"`
	} `json:"diagram_hint"`
	RequestedAnswerDimensions *struct {
		IsDimensionedAnswer bool `json:"is_dimensioned_answer"`
		Dimensions          []struct {
			Label    string `json:"label"`
			Role     string `json:"role"`
			Required bool   `json:"required"`
			Index    int    `json:"index"`
		} `json:"dimensions"`
	} `json:"requested_answer_dimensions"`
	CurrentSourceExplanationProfile *struct {
		IsCurrentSourceExplanationRequested bool     `json:"is_current_source_explanation_requested"`
		Modes                               []string `json:"modes"`
		TargetTerms                         []string `json:"target_terms"`
	} `json:"current_source_explanation_profile"`
	ExactTargets  []string `json:"exact_targets"`
	RequiredFiles []struct {
		Path string `json:"path"`
	} `json:"required_files"`
}

func formatAnalysisToolResultSummary(paramsJSON, resultSummary, lang string) string {
	var p analysisSummaryPayload
	if strings.TrimSpace(paramsJSON) == "" || json.Unmarshal([]byte(paramsJSON), &p) != nil {
		p = analysisSummaryFromResult(resultSummary)
	}
	if p.Intent == "" && p.Scenario == "" && p.Complexity == "" && p.QuestionKind == "" && len(p.Entities) == 0 && len(p.Keywords) == 0 {
		return ""
	}
	zh := isZh(lang)
	lines := []string{"  " + statusMeta.Sprint("•") + " " + statusMeta.Sprint(analysisHeader(zh))}
	classLine := joinNonEmpty([]string{
		labelValue("intent", "意图", p.Intent, zh),
		labelValue("kind", "类型", p.QuestionKind, zh),
		labelValue("scenario", "场景", p.Scenario, zh),
		labelValue("complexity", "复杂度", p.Complexity, zh),
		labelValue("axis", "谓词轴", p.PredicateAxis, zh),
	}, " · ")
	if classLine != "" {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+classLine), evidenceSummaryMaxCols))
	}
	if p.AnswerSubject != nil && p.AnswerSubject.Kind != "" {
		subject := labelValue("answer", "答案主体", p.AnswerSubject.Kind, zh)
		if len(p.AnswerSubject.EntityAxes) > 0 {
			subject += " · " + limitedListPhrase("axes", "轴", p.AnswerSubject.EntityAxes, 2, zh)
		}
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+subject), evidenceSummaryMaxCols))
	}
	if len(p.Entities) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("entities", "实体", p.Entities, 5, zh)), evidenceSummaryMaxCols))
	}
	if len(p.Keywords) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("keywords", "关键词", p.Keywords, 6, zh)), evidenceSummaryMaxCols))
	}
	if len(p.SubTopics) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+subTopicPhrase(p.SubTopics, zh)), evidenceSummaryMaxCols))
	}
	if dims := requestedAnswerDimensionSummaryValues(p); len(dims) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("answer dimensions", "答案维度", dims, 5, zh)), evidenceSummaryMaxCols))
	}
	if modes := currentSourceExplanationSummaryValues(p); len(modes) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("current-source link", "源码关联", modes, 4, zh)), evidenceSummaryMaxCols))
	}
	if p.DiagramHint != nil && p.DiagramHint.Kind != "" {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+labelValue("diagram", "图", p.DiagramHint.Kind, zh)), evidenceSummaryMaxCols))
	}
	if len(p.ExactTargets) > 0 {
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("targets", "精确目标", p.ExactTargets, 4, zh)), evidenceSummaryMaxCols))
	}
	if len(p.RequiredFiles) > 0 {
		paths := make([]string, 0, len(p.RequiredFiles))
		for _, f := range p.RequiredFiles {
			if strings.TrimSpace(f.Path) != "" {
				paths = append(paths, f.Path)
			}
		}
		if len(paths) > 0 {
			lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint("    "+limitedListPhrase("files", "建议文件", paths, 3, zh)), evidenceSummaryMaxCols))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func requestedAnswerDimensionSummaryValues(p analysisSummaryPayload) []string {
	profile := p.RequestedAnswerDimensions
	if profile == nil || !profile.IsDimensionedAnswer || len(profile.Dimensions) == 0 {
		return nil
	}
	dims := append([]struct {
		Label    string `json:"label"`
		Role     string `json:"role"`
		Required bool   `json:"required"`
		Index    int    `json:"index"`
	}(nil), profile.Dimensions...)
	sort.SliceStable(dims, func(i, j int) bool {
		if dims[i].Index == dims[j].Index {
			return i < j
		}
		if dims[i].Index <= 0 {
			return false
		}
		if dims[j].Index <= 0 {
			return true
		}
		return dims[i].Index < dims[j].Index
	})
	out := make([]string, 0, len(dims))
	for _, dim := range dims {
		label := sanitizeEvidenceSummaryText(dim.Label)
		if label == "" {
			label = sanitizeEvidenceSummaryText(dim.Role)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	return out
}

func currentSourceExplanationSummaryValues(p analysisSummaryPayload) []string {
	profile := p.CurrentSourceExplanationProfile
	if profile == nil || !profile.IsCurrentSourceExplanationRequested {
		return nil
	}
	out := make([]string, 0, len(profile.Modes)+len(profile.TargetTerms))
	for _, mode := range profile.Modes {
		if mode = sanitizeEvidenceSummaryText(mode); mode != "" {
			out = append(out, mode)
		}
	}
	for _, term := range profile.TargetTerms {
		if term = sanitizeEvidenceSummaryText(term); term != "" {
			out = append(out, term)
		}
	}
	return out
}

func analysisSummaryFromResult(summary string) analysisSummaryPayload {
	var p analysisSummaryPayload
	fields := strings.Fields(strings.TrimSpace(summary))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "intent":
			p.Intent = value
		case "scenario":
			p.Scenario = value
		case "complexity":
			p.Complexity = value
		case "kind":
			p.QuestionKind = value
		case "axis":
			p.PredicateAxis = value
		case "diagram_hint":
			p.DiagramHint = &struct {
				Kind string `json:"kind"`
			}{Kind: value}
		}
	}
	return p
}

func analysisHeader(zh bool) string {
	if zh {
		return "分析结果"
	}
	return "Analysis"
}

func labelValue(enLabel, zhLabel, value string, zh bool) string {
	value = sanitizeEvidenceSummaryText(value)
	if value == "" {
		return ""
	}
	if zh {
		return zhLabel + " " + value
	}
	return enLabel + " " + value
}

func joinNonEmpty(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, sep)
}

func limitedListPhrase(enLabel, zhLabel string, values []string, limit int, zh bool) string {
	clean := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = sanitizeEvidenceSummaryText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		clean = append(clean, value)
	}
	label := enLabel
	if zh {
		label = zhLabel
	}
	if len(clean) == 0 {
		return label + " 0"
	}
	if limit <= 0 || limit > len(clean) {
		limit = len(clean)
	}
	visible := strings.Join(clean[:limit], ", ")
	if len(clean) > limit {
		if zh {
			visible += fmt.Sprintf(", +%d", len(clean)-limit)
		} else {
			visible += fmt.Sprintf(", +%d more", len(clean)-limit)
		}
	}
	if zh {
		return fmt.Sprintf("%s %d 个：%s", label, len(clean), visible)
	}
	return fmt.Sprintf("%s %d: %s", label, len(clean), visible)
}

func subTopicPhrase(topics []struct {
	Title    string   `json:"title"`
	Entities []string `json:"entities"`
}, zh bool) string {
	titles := make([]string, 0, len(topics))
	for _, topic := range topics {
		title := sanitizeEvidenceSummaryText(topic.Title)
		if title == "" && len(topic.Entities) > 0 {
			title = sanitizeEvidenceSummaryText(topic.Entities[0])
		}
		if title != "" {
			titles = append(titles, title)
		}
	}
	if zh {
		return limitedListPhrase("subtopics", "子问题", titles, 3, zh)
	}
	return limitedListPhrase("subtopics", "子问题", titles, 3, zh)
}

func parseEmitEvidenceSummary(summary string) (int, []renderedEvidenceItem) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return 0, nil
	}
	lines := strings.Split(summary, "\n")
	count := 0
	if len(lines) > 0 {
		if m := emitEvidenceAcceptedRe.FindStringSubmatch(strings.TrimSpace(lines[0])); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				count = n
			}
		}
	}
	items := make([]renderedEvidenceItem, 0)
	for i := 0; i < len(lines); i++ {
		item, ok := parseEmitEvidenceItemLine(lines[i])
		if !ok {
			continue
		}
		if i+1 < len(lines) {
			item.Status = parseEmitEvidenceStatusLine(lines[i+1])
		}
		items = append(items, item)
	}
	return count, items
}

func parseEmitEvidenceItemLine(line string) (renderedEvidenceItem, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return renderedEvidenceItem{}, false
	}
	closeIdx := strings.Index(line, "]")
	if closeIdx < 1 || closeIdx+1 >= len(line) {
		return renderedEvidenceItem{}, false
	}
	body := strings.TrimSpace(line[closeIdx+1:])
	atIdx := strings.LastIndex(body, " @ ")
	if atIdx <= 0 {
		return renderedEvidenceItem{}, false
	}
	left := strings.TrimSpace(body[:atIdx])
	right := strings.TrimSpace(body[atIdx+3:])
	semantic := ""
	if idx := strings.Index(right, " — "); idx >= 0 {
		semantic = strings.TrimSpace(right[idx+len(" — "):])
		right = strings.TrimSpace(right[:idx])
	}
	location := parseEvidenceLocation(right)
	if location == "" {
		return renderedEvidenceItem{}, false
	}
	anchor := left
	if fields := strings.Fields(left); len(fields) > 1 {
		anchor = strings.Join(fields[1:], " ")
	}
	anchor = strings.TrimSpace(anchor)
	if anchor == "" || anchor == "-" {
		anchor = "evidence"
	}
	return renderedEvidenceItem{
		Anchor:   sanitizeEvidenceSummaryText(anchor),
		Location: sanitizeEvidenceSummaryText(location),
		Semantic: sanitizeEvidenceSummaryText(semantic),
	}, true
}

func parseEvidenceLocation(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx+1 >= len(s) {
		return ""
	}
	line := strings.TrimSpace(s[idx+1:])
	if _, err := strconv.Atoi(line); err != nil {
		return ""
	}
	file := strings.TrimSpace(s[:idx])
	if file == "" {
		return ""
	}
	return file + ":" + line
}

func parseEmitEvidenceStatusLine(line string) string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "→ grounded"):
		return "grounded"
	case strings.HasPrefix(line, "→ recovered"):
		return "recovered"
	case strings.HasPrefix(line, "→ ungrounded"):
		return "ungrounded"
	default:
		return ""
	}
}

func localizedEvidenceStatus(status, lang string) string {
	zh := isZh(lang)
	switch status {
	case "grounded":
		if zh {
			return "已落地"
		}
		return "grounded"
	case "recovered":
		if zh {
			return "已校正"
		}
		return "recovered"
	case "ungrounded":
		if zh {
			return "未落地"
		}
		return "ungrounded"
	default:
		if zh {
			return "记录"
		}
		return "recorded"
	}
}

func sanitizeEvidenceSummaryText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}
