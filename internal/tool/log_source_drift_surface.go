package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderDriftBoundedCurrentRootCauseSummary returns the prose for
// drift-bounded answers. Round-11 user red line: drift mode now fires
// for both IntentRootCause AND IntentTrace (R9), so this prose MUST
// be intent-neutral. The pre-fix tail clause "Deeper internal
// dereference details from the older build..." was panic-specific
// and confused IntentTrace users (their question wasn't about
// dereference). Replaced with intent-neutral "details from the
// older artifact build are not directly proven by the current
// citations" — covers panic / call-path / config-trace / perf
// scenarios uniformly.
//
// Function name retains "RootCause" suffix for backward compat
// (callers across orchestrator + emit_answer_document) but the
// prose itself no longer assumes the user's question is root-cause.
func RenderDriftBoundedCurrentRootCauseSummary(plan *types.AnswerSurfacePlan, lang string) string {
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	callText, mechanismText := driftBoundedPrimaryClauses(plan, zh)
	tail := driftBoundedTailDisclosure(zh)
	switch {
	case callText != "" && mechanismText != "" && callText != mechanismText:
		if zh {
			return fmt.Sprintf("当前仓库能确认的已锚定路径是%s。当前最近的已锚定机制是：%s。%s", callText, mechanismText, tail)
		}
		return fmt.Sprintf("The current repo grounds the path where %s. The nearest grounded current-code mechanism is: %s. %s", callText, mechanismText, tail)
	case callText != "":
		if zh {
			return fmt.Sprintf("当前仓库能确认的已锚定路径是%s。%s", callText, tail)
		}
		return fmt.Sprintf("The current repo grounds the path where %s. %s", callText, tail)
	case mechanismText != "":
		if zh {
			return fmt.Sprintf("当前仓库当前能确认的最近机制是：%s。%s", mechanismText, tail)
		}
		return fmt.Sprintf("The nearest grounded current-code mechanism is: %s. %s", mechanismText, tail)
	default:
		return ""
	}
}

// driftBoundedTailDisclosure returns the intent-neutral closing
// sentence for the drift-bounded summary. Avoids panic-specific
// vocabulary ("dereference", "解引用") so IntentTrace and
// IntentRootCause user questions get equally appropriate prose.
func driftBoundedTailDisclosure(zh bool) string {
	if zh {
		return "更深的旧构建细节仍未被当前引用直接证明。"
	}
	return "Deeper details from the older artifact build are not directly proven by the current citations."
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
	for _, item := range types.DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
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

// RenderDriftBoundedLogBundleSurfaceSummary returns prose summarising
// the attached LogBundle's signal + error types. Round-11 user red
// line: prose phrasing must NOT hard-assume the artifact is a
// "panic-style" log. Signal-set may be timeout / db / network /
// validation / logic / performance / other — for these the
// pre-fix "this is a [validation]" sentence was awkward. Now uses
// "The attached log surfaced these signals: ..." which fits any
// signal subset, and the artifact name uses "log" only when the
// bundle is genuinely a LogBundle (perf-trace prose is rendered
// by RenderDriftBoundedPerfBundleSurfaceSummary below).
func RenderDriftBoundedLogBundleSurfaceSummary(bundle *types.LogBundle, lang string) string {
	if bundle == nil {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	canonicalSignals := make(map[string]bool)
	for _, signal := range types.AllLogSignals() {
		name := strings.TrimSpace(string(signal))
		if name != "" {
			canonicalSignals[strings.ToLower(name)] = true
		}
	}
	signals := make([]string, 0, len(bundle.Meta.Signals))
	seenSignal := make(map[string]bool)
	for _, signal := range bundle.Meta.Signals {
		name := strings.TrimSpace(string(signal))
		key := strings.ToLower(name)
		if name == "" || seenSignal[key] {
			continue
		}
		seenSignal[key] = true
		signals = append(signals, "`"+name+"`")
	}
	for _, entity := range bundle.Entities {
		name := strings.TrimSpace(entity)
		key := strings.ToLower(name)
		if name == "" || !canonicalSignals[key] || seenSignal[key] {
			continue
		}
		seenSignal[key] = true
		signals = append(signals, "`"+name+"`")
	}
	errorTypes := types.LogBundleErrorTypes(bundle)
	switch {
	case len(signals) > 0 && len(errorTypes) > 0:
		typeSummary := RenderDriftBoundedErrorTypeCoverageSummary(errorTypes, lang)
		if zh {
			return fmt.Sprintf("附带日志的信号集是%s。%s", strings.Join(signals, " / "), typeSummary)
		}
		return fmt.Sprintf("The attached log surfaced these signals: %s. %s", strings.Join(signals, " / "), typeSummary)
	case len(signals) > 0:
		if zh {
			return fmt.Sprintf("附带日志的信号集是%s。", strings.Join(signals, " / "))
		}
		return fmt.Sprintf("The attached log surfaced these signals: %s.", strings.Join(signals, " / "))
	default:
		return RenderDriftBoundedErrorTypeCoverageSummary(errorTypes, lang)
	}
}

// RenderDriftBoundedPerfBundleSurfaceSummary is the perf-trace
// counterpart to RenderDriftBoundedLogBundleSurfaceSummary.
// Round-11: perf-only / trace-only attached scenarios get correct
// "perf trace" prose instead of being silently force-fit into
// log-bundle prose (which said "附带日志..." even when only a
// PerfBundle was attached).
func RenderDriftBoundedPerfBundleSurfaceSummary(bundle *types.PerfBundle, lang string) string {
	if bundle == nil {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	signals := make([]string, 0, len(bundle.Meta.Signals))
	seen := make(map[string]bool)
	for _, sig := range bundle.Meta.Signals {
		name := strings.TrimSpace(sig)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		signals = append(signals, "`"+name+"`")
	}
	if len(signals) == 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("附带的性能轨迹的信号集是%s。", strings.Join(signals, " / "))
	}
	return fmt.Sprintf("The attached perf trace surfaced these signals: %s.", strings.Join(signals, " / "))
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
	actorName := strings.TrimSpace(firstNonEmpty(item.Subject, item.OwnerSymbol, item.AnchorSymbol, item.Object))
	if item.AnchorKind == types.AnchorCondition || item.AnchorKind == types.AnchorReturn || item.AnchorKind == types.AnchorAssignment {
		actorName = strings.TrimSpace(firstNonEmpty(item.OwnerSymbol, item.Subject, item.AnchorSymbol, item.Object))
	}
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
	for _, item := range types.DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
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
