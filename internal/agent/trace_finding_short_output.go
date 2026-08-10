package agent

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

type traceShortRootCauseJSON struct {
	ThreadName string `json:"thread_name"`
	RootCause  string `json:"root_cause"`
}

func renderTraceFindingShortRootCause(ctx *types.AgentContext, lang string) string {
	if ctx == nil || ctx.Mutable == nil || !traceShortRootCauseRequested(ctx) {
		return ""
	}
	return renderTraceFindingShortRootCauseValue(ctx.Mutable.TraceFinding(), lang)
}

func renderTraceFindingShortRootCauseValue(finding *types.TraceFindingV1, lang string) string {
	if finding == nil {
		return ""
	}
	title := "## Short root cause"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh") {
		title = "## 简短根因"
	}
	if finding.PrimaryCause == nil {
		reason := "无法从原始 Trace 的结构化证据中确定根因"
		if finding.Unresolved != nil && strings.TrimSpace(finding.Unresolved.Reason) != "" {
			reason = compactFindingText(finding.Unresolved.Reason, 140)
		}
		payload, _ := json.MarshalIndent(traceShortRootCauseJSON{RootCause: reason}, "", "  ")
		return title + "\n\n```json\n" + string(payload) + "\n```"
	}

	short := traceShortRootCauseFromDecision(*finding.PrimaryCause)
	payload, _ := json.MarshalIndent(short, "", "  ")
	return title + "\n\n```json\n" + string(payload) + "\n```"
}

// mergeTraceFindingWithDetailedAnswer is deliberately a byte-preserving
// no-op when the optional supplement is absent. When present, it only adds a
// prefix; the model's original long answer remains an unchanged suffix.
func mergeTraceFindingWithDetailedAnswer(prose, supplement, lang string) string {
	supplement = strings.TrimSpace(supplement)
	if supplement == "" || strings.Contains(prose, supplement) {
		return prose
	}
	if strings.TrimSpace(prose) == "" {
		return supplement
	}
	detailTitle := "## Detailed analysis"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh") {
		detailTitle = "## 完整分析"
	}
	return supplement + "\n\n---\n\n" + detailTitle + "\n\n" + prose
}

func traceShortRootCauseFromDecision(decision types.TraceCauseDecision) traceShortRootCauseJSON {
	thread := firstShortFindingValue(decision.SubjectName, "未知")
	resource := compactTraceResourceName(firstShortFindingValue(decision.ResourceName, decision.SubjectName, "未知资源"))
	phase := firstShortFindingValue(decision.PhaseName, usableTracePhase(decision.Phase), decision.SubjectName, "当前")
	token := strings.ToLower(strings.TrimSpace(decision.Token.Token))
	lane := strings.ToLower(strings.TrimSpace(decision.Token.Lane))

	rootCause := ""
	switch {
	case token == "binder_wait":
		rootCause = thread + "线程同步binder"
	case token == "priority_inversion_candidate" || token == "priority_inversion_runnable_wait":
		rootCause = thread + "线程优先级反转"
	case token == "gc_pause" || token == "memory_gc":
		rootCause = "GC耗时长"
	case token == "jit_compile" || token == "runtime_compile" || token == "class_verification":
		rootCause = thread + "线程JIT编译耗时"
	case token == "shader_compile":
		rootCause = thread + "线程Shader编译"
	case token == "sleep_wait" || token == "fragmented_sleep_wait" || token == "missing_wakeup" ||
		token == "pacing_idle" || token == "periodic_idle":
		rootCause = thread + "线程阻塞"
	case token == "blocking_span" || lane == "lock_contention" || strings.TrimSpace(decision.BlockingKind) != "":
		rootCause = resource + "锁竞争"
	case lane == "io_blocking" || token == "page_cache_churn" || token == "memory_reclaim" || token == "memory_page_fault":
		rootCause = thread + "线程IO阻塞"
	case lane == "compute_delivery" || token == "compute_supply" || token == "low_frequency" || token == "cpu_frequency_limit":
		rootCause = "供给不足"
	case lane == "scheduling_demand":
		rootCause = thread + "线程CPU调度延迟"
	case lane == "cpu_work" || lane == "irq_aggregate":
		rootCause = phase + "阶段高负载"
	case lane == "memory_pressure":
		rootCause = "GC耗时长"
	default:
		// Diagnostic rows are not expected to survive candidate compilation.
		// If a future token reaches this boundary, keep the value inside the
		// user's closed vocabulary instead of leaking an internal wire token.
		rootCause = phase + "阶段高负载"
	}
	return traceShortRootCauseJSON{ThreadName: thread, RootCause: rootCause}
}

func compactTraceResourceName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	lower := strings.ToLower(value)
	for _, prefix := range []string{"lock contention on ", "monitor contention on ", "contention on "} {
		if strings.HasPrefix(lower, prefix) {
			value = strings.TrimSpace(value[len(prefix):])
			lower = strings.ToLower(value)
			break
		}
	}
	for _, marker := range []string{" (owner tid:", " owner_tid=", " (owner="} {
		if index := strings.Index(lower, marker); index >= 0 {
			value = strings.TrimSpace(value[:index])
			break
		}
	}
	return compactFindingText(value, 100)
}

func usableTracePhase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "unknown") {
		return ""
	}
	return value
}

func firstShortFindingValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func compactFindingText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
