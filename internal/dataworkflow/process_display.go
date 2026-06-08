package dataworkflow

import (
	"fmt"
	"strings"
)

type WorkflowProcessDisplay struct {
	Label    string                         `json:"label,omitempty"`
	Segments []string                       `json:"segments,omitempty"`
	Details  []WorkflowProcessDisplayDetail `json:"details,omitempty"`
}

type WorkflowProcessDisplayDetail struct {
	Key   string `json:"key,omitempty"`
	Class string `json:"class,omitempty"`
	Text  string `json:"text,omitempty"`
}

func BuildWorkflowProcessDisplay(event WorkflowJournalEvent, lang string) WorkflowProcessDisplay {
	zh := processDisplayIsZh(lang)
	display := WorkflowProcessDisplay{
		Label:    processDisplayLabel(zh),
		Segments: processDisplaySegments(event, zh),
		Details:  processDisplayDetails(event, zh),
	}
	return display
}

func processDisplayLabel(zh bool) string {
	if zh {
		return "数据工作流"
	}
	return "data workflow"
}

func processDisplaySegments(event WorkflowJournalEvent, zh bool) []string {
	kind := strings.TrimSpace(event.Kind)
	round := event.Round
	var segs []string
	if zh {
		switch kind {
		case "execute":
			segs = append(segs, fmt.Sprintf("执行第 %d 批", round))
		case "repair":
			segs = append(segs, fmt.Sprintf("修复第 %d 次", round))
		case "patch":
			segs = append(segs, fmt.Sprintf("结构修复第 %d 批", round))
		case "result":
			segs = append(segs, fmt.Sprintf("结果第 %d 批", round))
		case "evaluate":
			segs = append(segs, fmt.Sprintf("评估第 %d 批", round))
		case "continue":
			segs = append(segs, fmt.Sprintf("继续第 %d 批", round+1))
		case "resume":
			segs = append(segs, fmt.Sprintf("恢复第 %d 批", maxProcessRound(round)))
		default:
			segs = append(segs, kind)
		}
		segs = append(segs, "未读源码")
		return cleanStrings(segs)
	}
	switch kind {
	case "execute":
		segs = append(segs, fmt.Sprintf("execute batch %d", round))
	case "repair":
		segs = append(segs, fmt.Sprintf("repair %d", round))
	case "patch":
		segs = append(segs, fmt.Sprintf("structural patch batch %d", round))
	case "result":
		segs = append(segs, fmt.Sprintf("result batch %d", round))
	case "evaluate":
		segs = append(segs, fmt.Sprintf("evaluate batch %d", round))
	case "continue":
		segs = append(segs, fmt.Sprintf("continue batch %d", round+1))
	case "resume":
		segs = append(segs, fmt.Sprintf("resume batch %d", maxProcessRound(round)))
	default:
		segs = append(segs, kind)
	}
	segs = append(segs, "no source read")
	return cleanStrings(segs)
}

func processDisplayDetails(event WorkflowJournalEvent, zh bool) []WorkflowProcessDisplayDetail {
	var details []WorkflowProcessDisplayDetail
	add := func(key, class, prefix, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		details = append(details, WorkflowProcessDisplayDetail{
			Key:   key,
			Class: class,
			Text:  prefix + clampProcessText(value, 180),
		})
	}
	if zh {
		add("goal", "business", "目标：", event.Goal)
		add("batch", "business", "本批：", event.BatchPurpose)
		add("next", "business", "下一步：", event.NextStep)
		add("actions", "business", "动作：", event.ActionSummary)
		if event.Decision != nil {
			add("decision", "decision", "", event.Decision.Reason)
		}
		if event.Guard != nil && !event.Guard.Empty() {
			add("failure", "failure", "原因：", event.Guard.ErrorText())
		}
		for _, detail := range event.AuditDetails {
			add("audit", "audit", "审计：", detail)
		}
		return append(details, processDisplayDefaultDetails(event, zh, len(details) > 0)...)
	}
	add("goal", "business", "Goal: ", event.Goal)
	add("batch", "business", "Batch: ", event.BatchPurpose)
	add("next", "business", "Next: ", event.NextStep)
	add("actions", "business", "Actions: ", event.ActionSummary)
	if event.Decision != nil {
		add("decision", "decision", "", event.Decision.Reason)
	}
	if event.Guard != nil && !event.Guard.Empty() {
		add("failure", "failure", "Reason: ", event.Guard.ErrorText())
	}
	for _, detail := range event.AuditDetails {
		add("audit", "audit", "Audit: ", detail)
	}
	return append(details, processDisplayDefaultDetails(event, zh, len(details) > 0)...)
}

func processDisplayDefaultDetails(event WorkflowJournalEvent, zh bool, hasDetails bool) []WorkflowProcessDisplayDetail {
	if hasDetails {
		return nil
	}
	kind := strings.TrimSpace(event.Kind)
	detail := WorkflowProcessDisplayDetail{Key: "default", Class: "business"}
	if zh {
		switch kind {
		case "execute":
			detail.Text = "动作：执行当前有界数据动作批次，生成可复用产物和结构化审计后再评估下一步。"
		case "result":
			detail.Text = "结果：本批完成，已记录材料消费、生成产物和校验信号。"
		case "evaluate":
			detail.Text = "评估：根据目标、材料覆盖、产物字段、贡献记录和对账状态判断继续、修复或输出。"
		case "continue":
			detail.Text = "继续：上一批仍不足以达成目标，继续规划下一批原子动作。"
		case "resume":
			detail.Text = "恢复：从显式指定的 workflow checkpoint 载入已验证状态，再选择下一批原子动作。"
		case "repair":
			detail.Text = "修复：根据结构化失败原因生成下一批修复动作。"
		case "patch":
			detail.Text = "结构修复：对无歧义的结果结构漂移做安全补丁，业务语义仍由重新计算承担。"
		default:
			return nil
		}
		return []WorkflowProcessDisplayDetail{detail}
	}
	switch kind {
	case "execute":
		detail.Text = "Action: Executing this bounded data action batch; reusable artifacts and structured audit feed the next evaluation."
	case "result":
		detail.Text = "Result: Batch completed; material use, artifacts, and validation signals were recorded."
	case "evaluate":
		detail.Text = "Evaluate: Checking goal progress, material coverage, artifact fields, contributions, and reconcile state."
	case "continue":
		detail.Text = "Continue: The previous batch is not enough yet; planning the next atomic batch."
	case "resume":
		detail.Text = "Resume: Loaded the explicitly supplied workflow checkpoint, then selected the next atomic batch."
	case "repair":
		detail.Text = "Repair: Planning the next repair batch from the structured failure reason."
	case "patch":
		detail.Text = "Patch: Applying safe structural result patches; semantic changes still require recompute."
	default:
		return nil
	}
	return []WorkflowProcessDisplayDetail{detail}
}

func maxProcessRound(round int) int {
	if round > 1 {
		return round
	}
	return 1
}

func processDisplayIsZh(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	return lang == "" || strings.HasPrefix(lang, "zh") || strings.Contains(lang, "chinese")
}
