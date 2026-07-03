package tool

// §7.30.3 D2/D4 — three-tier fidelity for root-cause type tokens:
//   narrative lanes (lead line)   = zh label（raw token） on the zh surface;
//   tree / cause rows             = concise zh label only;
//   lossless detail table         = a dedicated 类型 column keeps the raw
//                                   English token verbatim (audit fidelity).
// The EN surface keeps raw tokens everywhere (they are already aligned).
// Unmapped tokens always render verbatim — labels are never fabricated.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceRootCauseTypeZHLabel maps a deterministic root-cause type token
// (the rootCauseTypeWeight universe plus the default-weight producers
// io_latency / sleep_wait / ipi_activity) to its concise Chinese tree label.
// Returns "" for unmapped tokens — callers keep the original token. A
// coverage test scans the rootCauseTypeWeight case set so newly added types
// cannot silently miss a translation.
func runtimeTraceRootCauseTypeZHLabel(token string) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "priority_inversion_candidate":
		return "优先级反转候选"
	case "priority_inversion_runnable_wait":
		return "可运行等待反转"
	case "io_latency":
		return "IO延迟"
	case "io_wait":
		return "IO等待"
	case "d_state_or_io_wait":
		return "D态或IO等待"
	case "binder_wait":
		return "binder等待"
	case "io_pressure":
		return "IO压力"
	case "io_burst_episode":
		return "IO突发"
	case "block_io_by_inode":
		return "块设备IO(inode)"
	case "file_io_hot_inode":
		return "热点文件IO"
	case "page_cache_churn":
		return "页缓存抖动"
	case "runnable_wait":
		return "可运行等待"
	case "sleep_wait":
		return "睡眠等待"
	case "cpu_pressure":
		return "CPU竞争压力"
	case "scheduler_latency":
		return "调度延迟"
	case "fragmented_d_state_or_io_wait":
		return "碎片化D态或IO等待"
	case "fragmented_runnable_wait":
		return "碎片化可运行等待"
	case "fragmented_sleep_wait":
		return "碎片化睡眠等待"
	case "fragmented_running":
		return "碎片化运行"
	case "state_churn":
		return "状态切换"
	case "compute_supply":
		return "算力供给"
	case "low_frequency":
		return "低频运行"
	case "cpu_affinity_or_cpuset":
		return "CPU亲和/cpuset限制"
	case "running":
		return "运行"
	case "jit_compile":
		return "JIT编译"
	case "class_verification":
		return "类校验"
	case "shader_compile":
		return "着色器编译"
	case "runtime_compile":
		return "运行时编译"
	case "cpu_frequency_limit":
		return "频率受限"
	case "trace_span":
		return "跟踪span"
	case "irq_burst":
		return "中断突发"
	case "irq_activity":
		return "中断活动"
	case "ipi_activity":
		return "核间中断"
	case "workqueue_activity":
		return "工作队列活动"
	case "dma_fence_activity":
		return "DMA fence活动"
	case "supply_pressure":
		return "供给压力"
	default:
		return ""
	}
}

// runtimeTraceAggregateTypeShapeLabel is the H20 impact-shape lane (customer
// audit 2026-07-03): when a row's impact shape would otherwise fall back to
// the generic 候选影响 / candidate word, these aggregate-activity tokens carry
// their own typed shape wording. Typed enum membership only — unmapped tokens
// return "" and the caller keeps the generic fallback.
func runtimeTraceAggregateTypeShapeLabel(token string, zh bool) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "irq_burst":
		if zh {
			return "IRQ突发"
		}
		return "IRQ burst"
	case "irq_activity":
		if zh {
			return "IRQ活动"
		}
		return "IRQ activity"
	case "page_cache_churn":
		if zh {
			return "页缓存抖动"
		}
		return "page-cache churn"
	default:
		return ""
	}
}

// runtimeTraceCausalProjectionDisplayCauseName is the tree/cause-row lane
// (D2): the zh surface shows the concise Chinese label for a recognized type
// token; the EN surface and unmapped tokens render verbatim (via the display
// sentinel mapping).
func runtimeTraceCausalProjectionDisplayCauseName(raw string, zh bool) string {
	if zh {
		if label := runtimeTraceRootCauseTypeZHLabel(raw); label != "" {
			return label
		}
	}
	return runtimeTraceCausalProjectionDisplayNodeName(raw, zh)
}

// runtimeTraceCausalProjectionDisplayCauseNameNode is the node-aware cause
// lane: when the row's Object is the unknown-thread sentinel, the wording is
// specialized by the row's typed kind token (blocking_span →
// 阻塞等待(对端未解析), d_state_or_io_wait → D状态/IO等待(对端未解析));
// everything else stays on the raw-string cause lane.
func runtimeTraceCausalProjectionDisplayCauseNameNode(node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return runtimeTraceCausalProjectionUnresolvedPeerText(runtimeTraceCausalProjectionUnresolvedPeerKind(node), zh)
	}
	return runtimeTraceCausalProjectionDisplayCauseName(node.Object, zh)
}

// runtimeTraceCausalProjectionNarrativeCauseName is the narrative lane (D4):
// on the zh surface a recognized type token renders as 中文（english_token）,
// e.g. 优先级反转候选（priority_inversion_candidate）; EN and unmapped tokens
// render verbatim.
func runtimeTraceCausalProjectionNarrativeCauseName(raw string, zh bool) string {
	raw = strings.TrimSpace(raw)
	if zh {
		if label := runtimeTraceRootCauseTypeZHLabel(raw); label != "" {
			return label + "（" + raw + "）"
		}
	}
	return runtimeTraceCausalProjectionDisplayNodeName(raw, zh)
}

// runtimeTraceCausalProjectionRawTypeToken supplies the detail table's 类型
// column (D2 audit fidelity): the raw English type token backing this row —
// the Object when it is a recognized type token, the typed semantic class for
// span rows, or the typed BlockingKind for contention rows. "" = no type
// token (the caller renders a dash).
func runtimeTraceCausalProjectionRawTypeToken(node types.TraceCausalProjectionNode) string {
	if token := strings.ToLower(strings.TrimSpace(node.Object)); runtimeTraceRootCauseTypeZHLabel(token) != "" {
		return token
	}
	if class := strings.ToLower(strings.TrimSpace(node.SemanticClass)); class != "" && runtimeTraceRootCauseTypeZHLabel(class) != "" {
		return class
	}
	if kind := strings.TrimSpace(node.BlockingKind); kind != "" {
		return kind
	}
	return ""
}
