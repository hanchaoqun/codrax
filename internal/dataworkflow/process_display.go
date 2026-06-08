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
	Value string `json:"value,omitempty"`
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
		case "completion_gate":
			segs = append(segs, fmt.Sprintf("终态校验第 %d 批", maxProcessRound(round)))
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
	case "completion_gate":
		segs = append(segs, fmt.Sprintf("completion gate batch %d", maxProcessRound(round)))
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
		value = clampProcessText(value, 180)
		details = append(details, WorkflowProcessDisplayDetail{
			Key:   key,
			Class: class,
			Value: value,
			Text:  prefix + value,
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
			add("blocker", "business", "当前阻塞：", processDisplayGuardBlocker(event.Guard, event.ViolationSummary, zh))
			add("repair", "decision", "可继续：", processDisplayGuardRepair(event.Guard, event.ViolationSummary, zh))
			add("failure", "failure", "原因：", event.Guard.ErrorText())
		} else if event.Decision == nil {
			add(processDisplayReasonKey(event), processDisplayReasonClass(event), processDisplayReasonPrefix(event, zh), event.Reason)
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
		add("blocker", "business", "Blocked: ", processDisplayGuardBlocker(event.Guard, event.ViolationSummary, zh))
		add("repair", "decision", "Next: ", processDisplayGuardRepair(event.Guard, event.ViolationSummary, zh))
		add("failure", "failure", "Reason: ", event.Guard.ErrorText())
	} else if event.Decision == nil {
		add(processDisplayReasonKey(event), processDisplayReasonClass(event), processDisplayReasonPrefix(event, zh), event.Reason)
	}
	for _, detail := range event.AuditDetails {
		add("audit", "audit", "Audit: ", detail)
	}
	return append(details, processDisplayDefaultDetails(event, zh, len(details) > 0)...)
}

func processDisplayGuardBlocker(guard *GuardResult, summary WorkflowViolationSummary, zh bool) string {
	violation := processDisplayFirstViolation(guard, summary)
	code := strings.TrimSpace(violation.Code)
	if code == "" {
		code = strings.TrimSpace(summary.FirstCode)
	}
	input := firstNonEmptyProcessText(violation.InputAlias, summary.FirstInputAlias)
	field := firstNonEmptyProcessText(violation.Field, summary.FirstField, strings.Join(cleanStrings(violation.MissingFields), ", "), strings.Join(summary.FirstMissingFields, ", "))
	param := firstNonEmptyProcessText(violation.Param, summary.FirstParam)
	operation := firstNonEmptyProcessText(violation.Operation, summary.FirstOperation)
	reason := firstNonEmptyProcessText(violation.Reason, summary.FirstReason, guardErrorText(guard), code)
	if zh {
		switch code {
		case "field_contract_violation":
			return appendProcessDisplayContext("本步引用的字段在输入中不存在。", input, field, param, operation, zh)
		case "value_contract_violation":
			return appendProcessDisplayContext("字段值不满足本步操作需要的值类型或形状。", input, field, param, operation, zh)
		case "field_inference_violation":
			return appendProcessDisplayContext("本步还没有足够结构信息来确定应使用哪些字段。", input, field, param, operation, zh)
		case "action_param_violation":
			return appendProcessDisplayContext("本步参数结构不符合动作 schema。", input, field, param, operation, zh)
		case "action_limit_violation":
			return appendProcessDisplayContext(processDisplayLimitText("本步输出超过当前安全上限，需要拆成更小批次。", violation, summary, zh), input, field, param, operation, zh)
		case "result_schema_mismatch":
			return appendProcessDisplayContext("本步没有产出系统可验证的结构化结果。", input, field, param, operation, zh)
		case "output_contract_violation":
			return appendProcessDisplayContext("最终答案还没有满足用户要求的输出格式。", input, field, param, operation, zh)
		case "zero_match_filter":
			return appendProcessDisplayContext("当前过滤条件没有匹配到可进入下一步的记录。", input, field, param, operation, zh)
		case "unmatched_resolution":
			return appendProcessDisplayContext("当前记录与参考/映射产物没有形成有效匹配。", input, field, param, operation, zh)
		case "zero_eligible_records":
			return appendProcessDisplayContext("当前批次没有得到符合下一步条件的记录。", input, field, param, operation, zh)
		case ViolationStageNoProgress:
			return "连续批次没有推进结构化进展，需要调整下一批原子动作。"
		default:
			return reason
		}
	}
	switch code {
	case "field_contract_violation":
		return appendProcessDisplayContext("This step references fields that do not exist in the selected input.", input, field, param, operation, zh)
	case "value_contract_violation":
		return appendProcessDisplayContext("Field values do not satisfy the value shape required by this operation.", input, field, param, operation, zh)
	case "field_inference_violation":
		return appendProcessDisplayContext("This step does not have enough structural information to infer the fields to use.", input, field, param, operation, zh)
	case "action_param_violation":
		return appendProcessDisplayContext("This step has an action parameter that does not match the typed schema.", input, field, param, operation, zh)
	case "action_limit_violation":
		return appendProcessDisplayContext(processDisplayLimitText("This step exceeded the current safety limit and needs a smaller batch.", violation, summary, zh), input, field, param, operation, zh)
	case "result_schema_mismatch":
		return appendProcessDisplayContext("This step did not emit a verifiable structured result.", input, field, param, operation, zh)
	case "output_contract_violation":
		return appendProcessDisplayContext("The final answer does not satisfy the requested output format yet.", input, field, param, operation, zh)
	case "zero_match_filter":
		return appendProcessDisplayContext("The current filters matched no records for the next step.", input, field, param, operation, zh)
	case "unmatched_resolution":
		return appendProcessDisplayContext("The current records did not match the reference or mapping artifact.", input, field, param, operation, zh)
	case "zero_eligible_records":
		return appendProcessDisplayContext("The current batch produced no records eligible for the next step.", input, field, param, operation, zh)
	case ViolationStageNoProgress:
		return "Repeated batches did not advance structured workflow progress; the next atomic action needs adjustment."
	default:
		return reason
	}
}

func processDisplayGuardRepair(guard *GuardResult, summary WorkflowViolationSummary, zh bool) string {
	violation := processDisplayFirstViolation(guard, summary)
	hints := cleanStrings(append(append([]string(nil), violation.RepairActionHints...), summary.FirstRepairActionHints...))
	if len(hints) == 0 {
		return ""
	}
	if zh {
		return "优先生成下一批结构化动作：" + strings.Join(processDisplayActionHintLabels(hints, zh), "、")
	}
	return "Prefer the next typed action batch: " + strings.Join(processDisplayActionHintLabels(hints, zh), ", ")
}

func processDisplayFirstViolation(guard *GuardResult, summary WorkflowViolationSummary) WorkflowViolation {
	if guard != nil {
		for _, violation := range guard.Violations {
			if strings.TrimSpace(violation.Code) != "" {
				return violation
			}
		}
	}
	return WorkflowViolation{
		Code:              summary.FirstCode,
		Severity:          summary.FirstSeverity,
		Repairability:     summary.FirstRepairability,
		ActionID:          summary.FirstActionID,
		ActionKind:        summary.FirstActionKind,
		InputAlias:        summary.FirstInputAlias,
		OutputAlias:       summary.FirstOutputAlias,
		Field:             summary.FirstField,
		Operation:         summary.FirstOperation,
		Param:             summary.FirstParam,
		MissingFields:     append([]string(nil), summary.FirstMissingFields...),
		RepairActionHints: append([]string(nil), summary.FirstRepairActionHints...),
		Limit:             summary.FirstLimit,
		Observed:          summary.FirstObserved,
		Reason:            summary.FirstReason,
	}
}

func appendProcessDisplayContext(base, input, field, param, operation string, zh bool) string {
	var parts []string
	if input != "" {
		if zh {
			parts = append(parts, "输入 "+input)
		} else {
			parts = append(parts, "input "+input)
		}
	}
	if field != "" {
		if zh {
			parts = append(parts, "字段 "+field)
		} else {
			parts = append(parts, "field "+field)
		}
	}
	if param != "" {
		if zh {
			parts = append(parts, "参数 "+param)
		} else {
			parts = append(parts, "param "+param)
		}
	}
	if operation != "" {
		if zh {
			parts = append(parts, "操作 "+operation)
		} else {
			parts = append(parts, "operation "+operation)
		}
	}
	if len(parts) == 0 {
		return base
	}
	return base + "（" + strings.Join(parts, " · ") + "）"
}

func processDisplayLimitText(base string, violation WorkflowViolation, summary WorkflowViolationSummary, zh bool) string {
	limit := violation.Limit
	if limit == 0 {
		limit = summary.FirstLimit
	}
	observed := violation.Observed
	if observed == 0 {
		observed = summary.FirstObserved
	}
	if limit == 0 && observed == 0 {
		return base
	}
	if zh {
		return fmt.Sprintf("%s（当前 %d / 上限 %d）", base, observed, limit)
	}
	return fmt.Sprintf("%s (observed %d / limit %d)", base, observed, limit)
}

func processDisplayActionHintLabels(hints []string, zh bool) []string {
	hints = cleanStrings(hints)
	out := make([]string, 0, len(hints))
	for _, hint := range hints {
		if zh {
			switch strings.TrimSpace(hint) {
			case "inspect_material":
				out = append(out, "检查材料或产物")
			case "extract_records":
				out = append(out, "抽取记录")
			case "derive_rules":
				out = append(out, "抽取规则")
			case "derive_fields":
				out = append(out, "补齐或派生字段")
			case "filter_records":
				out = append(out, "筛选记录")
			case "value_distribution":
				out = append(out, "查看字段取值分布")
			case "join_records":
				out = append(out, "关联记录")
			case "enrich_records":
				out = append(out, "应用参考信息")
			case "coverage_diff":
				out = append(out, "检查覆盖差异")
			case "mapping_candidate":
				out = append(out, "生成匹配候选")
			case "normalize_entities":
				out = append(out, "归一记录")
			case "apply_entity_resolutions":
				out = append(out, "应用匹配结果")
			case "qualify_records":
				out = append(out, "标记可用记录")
			case "expand_records":
				out = append(out, "展开记录")
			case "group_records":
				out = append(out, "分组汇总")
			case "compute_contributions":
				out = append(out, "计算贡献")
			case "reconcile_artifacts":
				out = append(out, "对账校验")
			case "assemble_answer":
				out = append(out, "生成最终输出")
			default:
				out = append(out, hint)
			}
			continue
		}
		out = append(out, hint)
	}
	return out
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
			detail.Value = "执行当前有界数据动作批次，生成可复用产物和结构化审计后再评估下一步。"
			detail.Text = "动作：执行当前有界数据动作批次，生成可复用产物和结构化审计后再评估下一步。"
		case "result":
			detail.Value = "本批完成，已记录材料消费、生成产物和校验信号。"
			detail.Text = "结果：本批完成，已记录材料消费、生成产物和校验信号。"
		case "evaluate":
			detail.Value = "根据目标、材料覆盖、产物字段、贡献记录和对账状态判断继续、修复或输出。"
			detail.Text = "评估：根据目标、材料覆盖、产物字段、贡献记录和对账状态判断继续、修复或输出。"
		case "continue":
			detail.Value = "上一批仍不足以达成目标，继续规划下一批原子动作。"
			detail.Text = "继续：上一批仍不足以达成目标，继续规划下一批原子动作。"
		case "resume":
			detail.Value = "从显式指定的 workflow checkpoint 载入已验证状态，再选择下一批原子动作。"
			detail.Text = "恢复：从显式指定的 workflow checkpoint 载入已验证状态，再选择下一批原子动作。"
		case "repair":
			detail.Value = "根据结构化失败原因生成下一批修复动作。"
			detail.Text = "修复：根据结构化失败原因生成下一批修复动作。"
		case "patch":
			detail.Value = "对无歧义的结果结构漂移做安全补丁，业务语义仍由重新计算承担。"
			detail.Text = "结构修复：对无歧义的结果结构漂移做安全补丁，业务语义仍由重新计算承担。"
		case "completion_gate":
			detail.Value = "检查最终答案是否已经满足材料、结构化记录、对账和输出契约。"
			detail.Text = "终态校验：检查最终答案是否已经满足材料、结构化记录、对账和输出契约。"
		default:
			return nil
		}
		return []WorkflowProcessDisplayDetail{detail}
	}
	switch kind {
	case "execute":
		detail.Value = "Executing this bounded data action batch; reusable artifacts and structured audit feed the next evaluation."
		detail.Text = "Action: Executing this bounded data action batch; reusable artifacts and structured audit feed the next evaluation."
	case "result":
		detail.Value = "Batch completed; material use, artifacts, and validation signals were recorded."
		detail.Text = "Result: Batch completed; material use, artifacts, and validation signals were recorded."
	case "evaluate":
		detail.Value = "Checking goal progress, material coverage, artifact fields, contributions, and reconcile state."
		detail.Text = "Evaluate: Checking goal progress, material coverage, artifact fields, contributions, and reconcile state."
	case "continue":
		detail.Value = "The previous batch is not enough yet; planning the next atomic batch."
		detail.Text = "Continue: The previous batch is not enough yet; planning the next atomic batch."
	case "resume":
		detail.Value = "Loaded the explicitly supplied workflow checkpoint, then selected the next atomic batch."
		detail.Text = "Resume: Loaded the explicitly supplied workflow checkpoint, then selected the next atomic batch."
	case "repair":
		detail.Value = "Planning the next repair batch from the structured failure reason."
		detail.Text = "Repair: Planning the next repair batch from the structured failure reason."
	case "patch":
		detail.Value = "Applying safe structural result patches; semantic changes still require recompute."
		detail.Text = "Patch: Applying safe structural result patches; semantic changes still require recompute."
	case "completion_gate":
		detail.Value = "Checking whether the final answer satisfies material, ledger, reconcile, and output contracts."
		detail.Text = "Completion gate: Checking whether the final answer satisfies material, ledger, reconcile, and output contracts."
	default:
		return nil
	}
	return []WorkflowProcessDisplayDetail{detail}
}

func processDisplayReasonKey(event WorkflowJournalEvent) string {
	if processDisplayStatusLooksFailure(event.Status) {
		return "failure"
	}
	return "reason"
}

func processDisplayReasonClass(event WorkflowJournalEvent) string {
	if processDisplayStatusLooksFailure(event.Status) {
		return "failure"
	}
	return "decision"
}

func processDisplayReasonPrefix(event WorkflowJournalEvent, zh bool) string {
	if processDisplayStatusLooksFailure(event.Status) {
		if zh {
			return "原因："
		}
		return "Reason: "
	}
	return ""
}

func processDisplayStatusLooksFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "blocked", "rejected":
		return true
	default:
		return false
	}
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
