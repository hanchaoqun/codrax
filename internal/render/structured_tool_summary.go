package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
		lines = append(lines, truncByDisplayWidth(statusReasoningBody.Sprint(fmt.Sprintf("    %d. %s", i+1, item)), evidenceSummaryMaxCols))
	}
	return strings.Join(lines, "\n") + "\n"
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
