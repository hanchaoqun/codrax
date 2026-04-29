package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderDriftBoundedCurrentRootCauseSummary(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return ""
	}
	zh := emitAnswerDocIsZh(ctx)
	callText := ""
	mechanismText := ""
	for _, item := range plan.DriftBoundedSurfaceItems {
		if callText == "" && driftBoundedSurfaceItemIsCallLike(item) {
			callText = renderDriftBoundedSurfaceItemClause(item, zh)
			continue
		}
		if mechanismText == "" {
			mechanismText = renderDriftBoundedSurfaceItemClause(item, zh)
		}
	}
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
		return renderDriftBoundedRootCauseFallbackSummary(ctx)
	}
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
	switch {
	case driftBoundedSurfaceItemIsCallLike(item) && subject != "" && object != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 调用 `%s`", subject, location, object)
		}
		return fmt.Sprintf("`%s` calls `%s` at `%s`", subject, object, location)
	case item.AnchorKind == types.AnchorCondition && strings.TrimSpace(item.Condition) != "" && name != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 先检查 `%s`", name, location, strings.TrimSpace(item.Condition))
		}
		return fmt.Sprintf("`%s` checks `%s` at `%s`", name, strings.TrimSpace(item.Condition), location)
	case item.AnchorKind == types.AnchorDefinition && name != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 的当前定义锚点在 `%s`", name, location)
		}
		return fmt.Sprintf("the current definition anchor for `%s` is `%s`", name, location)
	case item.AnchorKind == types.AnchorReturn && name != "" && object != "" && location != "":
		if zh {
			return fmt.Sprintf("`%s` 在 `%s` 返回 `%s`", name, location, object)
		}
		return fmt.Sprintf("`%s` returns `%s` at `%s`", name, object, location)
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
