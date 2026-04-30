package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func RenderDriftBoundedCurrentRootCauseSummary(plan *types.AnswerSurfacePlan, lang string) string {
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	callText, mechanismText := driftBoundedPrimaryClauses(plan, zh)
	switch {
	case callText != "" && mechanismText != "" && callText != mechanismText:
		if zh {
			return fmt.Sprintf("当前仓库能确认的已锚定路径是%s。当前最近的已锚定机制是：%s。更深的旧构建内部解引用点仍未被当前引用直接证明。", callText, mechanismText)
		}
		return fmt.Sprintf("The current repo grounds the path where %s. The nearest grounded current-code mechanism is: %s. Deeper internal dereference details from the older build are still not directly proven by the current citations.", callText, mechanismText)
	case callText != "":
		if zh {
			return fmt.Sprintf("当前仓库能确认的已锚定路径是%s。更深的旧构建内部解引用点仍未被当前引用直接证明。", callText)
		}
		return fmt.Sprintf("The current repo grounds the path where %s. Deeper internal dereference details from the older build are still not directly proven by the current citations.", callText)
	case mechanismText != "":
		if zh {
			return fmt.Sprintf("当前仓库当前能确认的最近机制是：%s。更深的旧构建内部解引用点仍未被当前引用直接证明。", mechanismText)
		}
		return fmt.Sprintf("The nearest grounded current-code mechanism is: %s. Deeper internal dereference details from the older build are still not directly proven by the current citations.", mechanismText)
	default:
		return ""
	}
}

func RenderDriftBoundedCurrentCodeDetailSummary(plan *types.AnswerSurfacePlan, lang string) string {
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	callText, mechanismText := driftBoundedPrimaryClauses(plan, zh)
	seen := map[string]bool{}
	if callText != "" {
		seen[callText] = true
	}
	if mechanismText != "" {
		seen[mechanismText] = true
	}
	var clauses []string
	for _, item := range plan.DriftBoundedSurfaceItems {
		clause := strings.TrimSpace(renderDriftBoundedSurfaceItemClause(item, zh))
		if clause == "" || seen[clause] {
			continue
		}
		seen[clause] = true
		clauses = append(clauses, clause)
		if len(clauses) >= 2 {
			break
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("当前代码还能直接锚定到：%s。", strings.Join(clauses, "；"))
	}
	return fmt.Sprintf("Current cited code also directly anchors: %s.", strings.Join(clauses, "; "))
}

func RenderDriftBoundedErrorTypeCoverageSummary(errorTypes []string, lang string) string {
	if len(errorTypes) == 0 {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	quoted := make([]string, 0, len(errorTypes))
	for _, typ := range errorTypes {
		typ = strings.TrimSpace(typ)
		if typ == "" {
			continue
		}
		quoted = append(quoted, "`"+typ+"`")
	}
	if len(quoted) == 0 {
		return ""
	}
	if zh {
		if len(quoted) == 1 {
			return fmt.Sprintf("附带日志的结构化错误类型是%s。", quoted[0])
		}
		return fmt.Sprintf("附带日志的结构化错误类型依次是%s。", strings.Join(quoted, " -> "))
	}
	if len(quoted) == 1 {
		return fmt.Sprintf("The attached log's structured error type is %s.", quoted[0])
	}
	return fmt.Sprintf("The attached log's structured error types are %s.", strings.Join(quoted, " -> "))
}

func RenderDriftBoundedLogBundleSurfaceSummary(bundle *types.LogBundle, lang string) string {
	if bundle == nil {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	signals := make([]string, 0, len(bundle.Meta.Signals))
	seenSignal := make(map[string]bool)
	for _, signal := range bundle.Meta.Signals {
		name := strings.TrimSpace(string(signal))
		if name == "" || seenSignal[name] {
			continue
		}
		seenSignal[name] = true
		signals = append(signals, "`"+name+"`")
	}
	errorTypes := types.LogBundleErrorTypes(bundle)
	switch {
	case len(signals) > 0 && len(errorTypes) > 0:
		typeSummary := RenderDriftBoundedErrorTypeCoverageSummary(errorTypes, lang)
		if zh {
			return fmt.Sprintf("附带日志显示这是一次%s。%s", strings.Join(signals, " / "), typeSummary)
		}
		return fmt.Sprintf("The attached log is a %s. %s", strings.Join(signals, " / "), typeSummary)
	case len(signals) > 0:
		if zh {
			return fmt.Sprintf("附带日志显示这是一次%s。", strings.Join(signals, " / "))
		}
		return fmt.Sprintf("The attached log is a %s.", strings.Join(signals, " / "))
	default:
		return RenderDriftBoundedErrorTypeCoverageSummary(errorTypes, lang)
	}
}

func renderDriftBoundedCurrentRootCauseSummary(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return ""
	}
	return RenderDriftBoundedCurrentRootCauseSummary(plan, requestedAnswerDocumentLanguage(ctx))
}

func renderDriftBoundedCurrentCodeDetailSummary(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return ""
	}
	return RenderDriftBoundedCurrentCodeDetailSummary(plan, requestedAnswerDocumentLanguage(ctx))
}

func normalizeLogSourceDriftCompletionReason(ctx *types.BusContext, raw string) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return strings.TrimSpace(raw)
	}
	if bounded := strings.TrimSpace(renderDriftBoundedCurrentRootCauseSummary(ctx)); bounded != "" {
		return bounded
	}
	return strings.TrimSpace(raw)
}

func renderDriftBoundedSurfaceItemClause(item types.EvidenceItem, zh bool) string {
	location := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if location != "" && item.LineStart > 0 {
		location = fmt.Sprintf("%s:%d", location, item.LineStart)
	}
	subject := strings.TrimSpace(item.Subject)
	object := strings.TrimSpace(firstNonEmpty(item.Object, item.AnchorSymbol))
	name := strings.TrimSpace(firstNonEmpty(item.AnchorSymbol, item.Subject, item.Object))
	actorName := strings.TrimSpace(firstNonEmpty(item.Subject, item.AnchorSymbol, item.Object))
	switch {
	case driftBoundedSurfaceItemIsCallLike(item) && subject != "" && object != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 调用 `%s`", subject, location, object)
		}
		return fmt.Sprintf("`%s` calls `%s` at `%s`", subject, object, location)
	case item.AnchorKind == types.AnchorCondition && strings.TrimSpace(item.Condition) != "" && actorName != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 先检查 `%s`", actorName, location, strings.TrimSpace(item.Condition))
		}
		return fmt.Sprintf("`%s` checks `%s` at `%s`", actorName, strings.TrimSpace(item.Condition), location)
	case item.AnchorKind == types.AnchorDefinition && name != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 的当前定义锚点在 `%s`", name, location)
		}
		return fmt.Sprintf("the current definition anchor for `%s` is `%s`", name, location)
	case item.AnchorKind == types.AnchorReturn && actorName != "" && object != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 返回 `%s`", actorName, location, object)
		}
		return fmt.Sprintf("`%s` returns `%s` at `%s`", actorName, object, location)
	case item.AnchorKind == types.AnchorAssignment && subject != "" && object != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 赋值 `%s`", subject, location, object)
		}
		return fmt.Sprintf("`%s` assigns `%s` at `%s`", subject, object, location)
	}
	text := strings.TrimSpace(types.EvidenceStructuredSemanticLine(item, false))
	if text == "" {
		return ""
	}
	if location == "" {
		return text
	}
	if zh {
		return fmt.Sprintf("%s（`%s`）", text, location)
	}
	return fmt.Sprintf("%s (`%s`)", text, location)
}

func driftBoundedPrimaryClauses(plan *types.AnswerSurfacePlan, zh bool) (string, string) {
	if plan == nil {
		return "", ""
	}
	callText := ""
	mechanismText := ""
	for _, item := range plan.DriftBoundedSurfaceItems {
		clause := renderDriftBoundedSurfaceItemClause(item, zh)
		if clause == "" {
			continue
		}
		if callText == "" && driftBoundedSurfaceItemIsCallLike(item) {
			callText = clause
			continue
		}
		if mechanismText == "" {
			mechanismText = clause
		}
	}
	return callText, mechanismText
}

func driftBoundedSurfaceItemIsCallLike(item types.EvidenceItem) bool {
	if item.AnchorKind == types.AnchorCall && strings.TrimSpace(item.Subject) != "" && strings.TrimSpace(firstNonEmpty(item.Object, item.AnchorSymbol)) != "" {
		return true
	}
	return types.IsCallLikeEvidencePredicate(item.Predicate)
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			return s
		}
	}
	return ""
}
