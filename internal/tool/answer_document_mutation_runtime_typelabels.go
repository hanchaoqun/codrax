package tool

// §7.30.3 D2/D4 — three-tier fidelity for root-cause type tokens:
//   narrative lanes (lead line)   = zh label（raw token） on the zh surface;
//   tree / cause rows             = concise zh label only;
//   lossless detail table         = a dedicated 类型 column keeps the raw
//                                   English token verbatim (audit fidelity).
// The EN surface keeps raw tokens everywhere (they are already aligned).
// Unmapped tokens always render verbatim — labels are never fabricated.
//
// PTV7 (#74, 用户裁定 2026-07-06, 内核状态词英文原词化): labels whose CONTENT
// is a kernel scheduler state speak the canonical English state token
// (running/runnable/sleep/D-state/iowait; the ambiguous producer compound
// keeps its honest two-sided D-state/iowait) — the tag / action / cause lanes
// share ONE token set and the Chinese state semantics live solely in the
// legend's state-icon entries. When a zh label equals its raw token the D4
// combined form collapses to the bare token (no label（label） echo). Product
// compound words (优先级反转候选 / 可运行等待反转), caliber words and
// narrative frames stay Chinese per the same ruling.

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
		return "iowait"
	case "d_state_or_io_wait":
		return "D-state/iowait"
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
		return "runnable"
	case "sleep_wait":
		return "sleep"
	case "cpu_pressure":
		return "CPU竞争压力"
	case "scheduler_latency":
		return "调度延迟"
	case "fragmented_d_state_or_io_wait":
		return "碎片化D-state/iowait"
	case "fragmented_runnable_wait":
		return "碎片化runnable"
	case "fragmented_sleep_wait":
		return "碎片化sleep"
	case "fragmented_running":
		return "碎片化running"
	case "state_churn":
		return "状态切换"
	case "compute_supply":
		return "算力供给"
	case "low_frequency":
		return "低频运行"
	case "cpu_affinity_or_cpuset":
		return "CPU亲和/cpuset限制"
	case "running":
		return "running"
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
		return runtimeTraceSupplyPressureDisplayLabel(true)
	default:
		return ""
	}
}

// runtimeTraceSupplyPressureDisplayLabel is THE display-side label for the
// supply_pressure wire token (CMP-10 §7.4, user adjudication): the metric is
// Σ runnable backlog — DEMAND-side scheduling pressure, PSI-stall family —
// so every display surface names it 调度压力(需求积压) / "scheduling pressure
// (demand backlog)" instead of the misleading "supply". The wire token
// type=supply_pressure itself is deliberately untouched: migrating the token
// is a separate R2' six-spot-sync adjudication (with an alias transition),
// not this display relabel. All display points MUST route through this
// helper; the detail-table 类型 column keeps the raw token for audit
// fidelity.
func runtimeTraceSupplyPressureDisplayLabel(zh bool) string {
	if zh {
		return "调度压力(需求积压)"
	}
	return "scheduling pressure (demand backlog)"
}

// runtimeTraceSupplyPressureToken reports whether raw is exactly the
// supply_pressure wire token (typed token match on the canonical form —
// never a substring heuristic).
func runtimeTraceSupplyPressureToken(raw string) bool {
	return runtimeTraceCausalProjectionCanonicalNode(raw) == "supply_pressure"
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
	// CMP-10 (§7.4): supply_pressure is display-relabeled on BOTH surfaces
	// (the EN raw-token rule is intentionally overridden for this one token —
	// the raw name asserts the wrong side of the demand/supply split).
	if runtimeTraceSupplyPressureToken(raw) {
		return runtimeTraceSupplyPressureDisplayLabel(zh)
	}
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
// 阻塞等待(对端未解析), d_state_or_io_wait → D-state/iowait(对端未解析));
// a RESOLVED peer thread riding the Object slot of a typed peer-relation row
// renders the relation form instead of the bare name (PTV6-C #7:
// "IO等待(对端 udk-irq-1-63)" — same wording home as the unresolved arm;
// the io_latency relation word is deliberately NOT a state token — the typed
// boundary io_latency ∉ state family is pinned, so PTV7 leaves it);
// everything else stays on the raw-string cause lane.
func runtimeTraceCausalProjectionDisplayCauseNameNode(node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return runtimeTraceCausalProjectionUnresolvedPeerText(runtimeTraceCausalProjectionUnresolvedPeerKind(node), zh)
	}
	if kind := runtimeTraceCausalProjectionResolvedPeerObjectKind(node); kind != "" {
		return runtimeTraceCausalProjectionResolvedPeerText(kind, runtimeTraceCausalProjectionDisplayNodeName(strings.TrimSpace(node.Object), zh), zh)
	}
	return runtimeTraceCausalProjectionDisplayCauseName(node.Object, zh)
}

// runtimeTraceCausalProjectionResolvedPeerObjectKind is the single #7 gate
// shared by the display lane and the typed dedupe-identity lane (修正轮 Med
// 2026-07-06: one predicate, so the two can never drift): non-"" exactly when
// the node's type lanes carry a peer-relation kind AND the Object actually IS
// a peer thread — never a recognized type token, never a bare scheduler-state
// token, never a peer-kind token echo, never an aggregate metric (precise
// token-table checks only).
func runtimeTraceCausalProjectionResolvedPeerObjectKind(node types.TraceCausalProjectionNode) string {
	kind := runtimeTraceCausalProjectionResolvedPeerKind(node)
	if kind == "" {
		return ""
	}
	object := strings.TrimSpace(node.Object)
	if object == "" || node.IsAggregateMetric() ||
		runtimeTraceRootCauseTypeZHLabel(object) != "" ||
		runtimeTraceCausalProjectionPeerKindToken(object) != "" ||
		runtimeTraceCausalProjectionCanonicalNode(object) == "io_latency" ||
		runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: object}, true) != "" {
		return ""
	}
	return kind
}

// runtimeTraceCausalProjectionCauseDisplayToken returns the TYPED token the
// node-aware cause display derives from — the #6/#12 dedupe identity beside
// runtimeTraceCausalProjectionDisplayCauseNameNode (branch-for-branch mirror
// via the shared gate above; 修正轮 Med: the dedupe judges typed tokens,
// display strings only present). "" when the display derives from no single
// typed token (generic unresolved wording).
func runtimeTraceCausalProjectionCauseDisplayToken(node types.TraceCausalProjectionNode) string {
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return runtimeTraceCausalProjectionUnresolvedPeerKind(node)
	}
	if kind := runtimeTraceCausalProjectionResolvedPeerObjectKind(node); kind != "" {
		return kind
	}
	return runtimeTraceCausalProjectionCanonicalNode(node.Object)
}

// runtimeTraceCausalProjectionTypeTokenStateClass maps a typed TypeToken whose
// semantics ARE a scheduler state onto its state family (PTV6-C #3, #73 标本
// 归因 2026-07-06). Non-empty ONLY when the node exposes no StateKind of its
// own — the display layer never overrides a producer-published state, it only
// reads the state the producer parked on the type lane. Exact typed-token
// membership; inversion composites (priority_inversion_*) are deliberately
// absent (D3: no single state may claim a gated composite).
func runtimeTraceCausalProjectionTypeTokenStateClass(node types.TraceCausalProjectionNode) string {
	if strings.TrimSpace(node.StateKind) != "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(node.TypeToken)) {
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		return "d_state_or_io_wait"
	case "runnable_wait", "fragmented_runnable_wait":
		return "runnable"
	case "sleep_wait", "fragmented_sleep_wait":
		return "s_sleep"
	case "running", "fragmented_running":
		return "running"
	case "io_wait":
		return "io_wait"
	}
	return ""
}

// runtimeTraceCausalProjectionTypeTokenStateWord renders the #3 state-family
// word for a TypeTokenStateClass value: the ambiguous d_state_or_io_wait
// family keeps its honest two-sided word (PTV7: the canonical D-state/iowait
// compound, face-invariant); every single-state class reuses the 裁定4
// StateKindLabel vocabulary verbatim (single wording home).
func runtimeTraceCausalProjectionTypeTokenStateWord(class string, zh bool) string {
	if class == "d_state_or_io_wait" {
		return "D-state/iowait"
	}
	return runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: class}, zh)
}

// runtimeTraceCausalProjectionImpactPointDisplay renders one 影响点 token in
// the D4 label（token） combined form on the zh surface (PTV6-C #6, #73 标本
// 归因 2026-07-06): bare scheduler-state tokens speak the 裁定4 state
// vocabulary, recognized type tokens ride the existing D4 narrative lane, and
// unmapped tokens render verbatim — labels are never fabricated. PTV7 (#74):
// the state vocabulary IS the canonical token set, so an identity echo
// (runnable（runnable）) collapses to the bare token; alias tokens keep the
// combined form for audit fidelity (sleep（s_sleep）). EN keeps raw tokens
// (already aligned).
func runtimeTraceCausalProjectionImpactPointDisplay(token string, zh bool) string {
	token = strings.TrimSpace(token)
	// Compound impact points arrive slash-joined from the producer
	// ("priority_inversion_runnable_wait/runnable") — each member maps
	// independently; the join stays "/" (same separator the tag itself uses).
	if strings.Contains(token, "/") {
		members := strings.Split(token, "/")
		for i, member := range members {
			members[i] = runtimeTraceCausalProjectionImpactPointDisplay(member, zh)
		}
		return strings.Join(members, "/")
	}
	if zh {
		if label := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: token}, true); label != "" {
			if label == token {
				return token
			}
			return label + "（" + token + "）"
		}
	}
	return runtimeTraceCausalProjectionNarrativeCauseName(token, zh)
}

// runtimeTraceCausalProjectionNarrativeCauseName is the narrative lane (D4):
// on the zh surface a recognized type token renders as label（english_token）,
// e.g. 优先级反转候选（priority_inversion_candidate）; EN and unmapped tokens
// render verbatim. PTV7 (#74): a label equal to its raw token (the running
// state-family label) collapses to the bare token — no label（label） echo.
func runtimeTraceCausalProjectionNarrativeCauseName(raw string, zh bool) string {
	raw = strings.TrimSpace(raw)
	if zh {
		if label := runtimeTraceRootCauseTypeZHLabel(raw); label != "" {
			if label == raw {
				return raw
			}
			return label + "（" + raw + "）"
		}
	}
	// CMP-10 (§7.4): the EN narrative also carries the demand-backlog
	// relabel while keeping the raw wire token for audit fidelity.
	if runtimeTraceSupplyPressureToken(raw) {
		return runtimeTraceSupplyPressureDisplayLabel(false) + " (" + raw + ")"
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
