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
// compound words (优先级反转候选 / 优先级反转·可运行等待), caliber words and
// narrative frames stay Chinese per the same ruling.

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
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
		// INV-SUPPLY §29.61.11: bytes in tracefence (UXG-1 M1; feed shares them).
		return tracefence.InversionCandidateWordZH
	case "priority_inversion_runnable_wait":
		return "优先级反转·可运行等待"
	case "io_latency":
		return "IO延迟"
	case "io_wait":
		return "iowait"
	case "d_state_or_io_wait":
		return "D-state/iowait"
	case "binder_wait":
		return "binder等待"
	case "pacing_idle":
		// P9 arm c (§29.42 案1, 2026-07-12): frame-pacing idle sleep — the
		// display word family the ruling fixed (帧间空闲/等待下一帧); the
		// legend entry teaches the semantics (runtime_tree.go).
		return "帧间空闲(等待下一帧)"
	case "periodic_idle":
		// 复核 P2-1 (2026-07-12): the arm-c generic fork — a measured
		// periodic (non-frame) waker; the frame promise words never render
		// on this token.
		return "周期空闲(等待下一周期信号)"
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
	case "texture_upload":
		// Evolution 2026-07-10: the Chinese decision/UX lane uses the customer-
		// ruled term 纹理上传. Raw span names, member rosters and audit wire tokens
		// remain verbatim, so this display mapping does not rewrite trace facts.
		return "纹理上传"
	case "gc_pause":
		return "GC暂停"
	case "cpu_frequency_limit":
		return "频率受限"
	case "trace_span":
		return "trace span"
	case "blocking_span":
		// PTV8-RCR-B (UXA 域A #1 / 域D 漏审 S1, 2026-07-08): the lead sentence
		// and the compare primary cell rendered the bare wire token — the zh
		// word matches the E4 row / monitor_contention wording (one token, one
		// translation); the raw token stays on the detail 类型 row (D2).
		return "持锁阻塞"
	case "missing_wakeup":
		// PTV8-RCR-B (UXA 域A #22 / 域B #15 / 域D #7, 任务令终词 2026-07-08):
		// the data-gap marker's display word (无唤醒记录) — display-only, the
		// registry wakeup_chain lane is untouched (红线 §7.2.1/§7.4/§7.5) and
		// the raw token stays on the detail 类型 row / evidence predicate.
		return "无唤醒记录"
	case "trace_gap":
		// §22 PTV7-SPN F5 (用户措辞裁定 2026-07-07): the diagnostic trace_gap
		// marker's display word — the raw token stays on the detail table's
		// 类型 column; registry LabelZhRef column moved in lockstep (golden
		// EVOLUTION RECORD in causal_token_registry_golden_test.go).
		return "数据盲区"
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

// TraceRootCauseTypeZHLabel exposes the root-cause type display lexicon to
// the system cross-check appendix (HEADLINE-ELIM 件2, §29.104.14.1,
// docs/design/real_trace_campaign_20260705.md, 2026-07-16): the appendix's
// headline-cause juxtaposition arm must speak the SAME zh word the report
// surfaces render for a published cause-class token — one lexicon, one
// source (观测/引擎单一值源; a hand copy in the consumer package would be the
// third wording home). Pure read wrapper: unmapped tokens return "" and the
// caller keeps the raw token, exactly like every in-package consumer.
func TraceRootCauseTypeZHLabel(token string) string {
	return runtimeTraceRootCauseTypeZHLabel(token)
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
		// PTV8-RCR-B (UXA 域A #24 / 域B #14 verify, 2026-07-08). EVOLUTION
		// RECORD: a BARE scheduler-state token on the cause-name lane used to
		// fall through verbatim (huadong "s_s…"/"s_sleep" double exposure
		// while the same row's state word said sleep) — it now rides the PTV7
		// alias combined form (sleep（s_sleep）; identity tokens collapse to
		// the bare word), the same treatment the 影响点 lane already had.
		if label := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: strings.TrimSpace(raw)}, true); label != "" {
			raw := strings.TrimSpace(raw)
			if label == raw {
				return raw
			}
			return label + "（" + raw + "）"
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
	// §29.50.5 (v5 P1 批 件②, 2026-07-13): the honest-remainder qualifier —
	// the typed engine marker is the ONE precise gate (it mints only on the
	// D/IO remainder seat beside sibling cause seat(s)), so every cause-word
	// arm of a remainder row wears it: 「D-state(原因未证)」 /
	// 「D-state/iowait(原因未证)」 forms per the §29.50.5 ruling.
	qualify := func(word string) string {
		if !node.DStateCauseUnprovenRemainder || strings.TrimSpace(word) == "" {
			return word
		}
		if zh {
			return word + "(原因未证)"
		}
		return word + " (cause unproven)"
	}
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return qualify(runtimeTraceCausalProjectionUnresolvedPeerText(runtimeTraceCausalProjectionUnresolvedPeerKindNode(node), zh))
	}
	if kind := runtimeTraceCausalProjectionResolvedPeerObjectKind(node); kind != "" {
		if kind == "d_state_or_io_wait" && node.DStateRefinedNonIO {
			kind = "d_state_refined"
		}
		return qualify(runtimeTraceCausalProjectionResolvedPeerText(kind, runtimeTraceCausalProjectionDisplayNodeName(strings.TrimSpace(node.Object), zh), zh))
	}
	// DSTATE-REFINE arm a (件③): a raw-lane merged cause word consumes the
	// engine's refined-D proof — 「D-state」 instead of 「D-state/iowait」.
	if runtimeTraceCausalProjectionCanonicalNode(node.Object) == "d_state_or_io_wait" && node.DStateRefinedNonIO {
		return qualify(runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: "d_state"}, zh))
	}
	return qualify(runtimeTraceCausalProjectionDisplayCauseName(node.Object, zh))
}

// runtimeTraceCausalProjectionUnresolvedPeerKindNode is the node-aware
// unresolved-peer kind: the raw typed kind refined by the arm-a proof
// (d_state_or_io_wait → d_state_refined when proven).
func runtimeTraceCausalProjectionUnresolvedPeerKindNode(node types.TraceCausalProjectionNode) string {
	kind := runtimeTraceCausalProjectionUnresolvedPeerKind(node)
	if kind == "d_state_or_io_wait" && node.DStateRefinedNonIO {
		return "d_state_refined"
	}
	return kind
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

// runtimeTraceCausalProjectionRefinedStateClass — DSTATE-REFINE arm a (CAL-1
// 件③, §29.39②/§29.47.2, 2026-07-12): the node's #3 state-family class after
// consuming the engine's typed refined-D proof — a merged d_state_or_io_wait
// class refines to the unambiguous "d_state" when the engine minted
// DStateRefinedNonIO (io_wait share zero ∧ blocked_reason 全覆盖∧全0); every
// other class (and every unproven row) passes through verbatim, keeping the
// honest merged 「D-state/iowait」 word.
func runtimeTraceCausalProjectionRefinedStateClass(node types.TraceCausalProjectionNode, class string) string {
	if class == "d_state_or_io_wait" && node.DStateRefinedNonIO {
		return "d_state"
	}
	return class
}

// runtimeTraceProjGenericUnresolvedStateNameWord — 76684 行1 形态词回退修
// (SMR-1 批 coordinator witness, 2026-07-12): the GENERIC unresolved-peer
// shape (unknown-thread sentinel Object + NO typed peer kind) used to put
// 「对端线程未解析」 in the row-1 name slot; with the wide CJK word the shared
// label column truncated the name to the bare subject, the #12 cause-word
// guarantee relocated 「对端线程未解析」 to the FIRST row tag, and width
// pressure demoted the STATE word (iowait) to 行2 — 行1 lost its 三要素 state
// word (违 PTV4 行1 三要素/零省略; 96728 对照形 kept 「主体 · iowait」).
//
// EVOLUTION RECORD (回退定位): the 96728 control renders the state word in
// 行1 because its Object lane carried the raw state token; the regression
// entered when the unresolved-peer rows' Object moved to the honest
// unknown-thread SENTINEL (CR-3 件② P10 era, blocked_reason 消费义务/诚实
// 哨兵批) — the generic peer word then took the name slot and the
// truncation+guarantee interplay pushed the state word off 行1. Fix: the
// row-1 name speaks 「主体 · <state label>」 (状态词永在行1); the
// unresolved-peer fact keeps its own demotable tag (行尾/行2 — WO 允许).
// Typed gates only: sentinel Object + empty typed peer kind + a real
// StateKind label. Kind-carrying forms (D-state/iowait(对端未解析)) already
// speak the state family in 行1 and stay byte-identical.
func runtimeTraceProjGenericUnresolvedStateNameWord(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return ""
	}
	if runtimeTraceCausalProjectionUnresolvedPeerKindNode(node) != "" {
		return ""
	}
	if label := strings.TrimSpace(runtimeTraceProjStateKindLabel(node, zh)); label != "" {
		return label
	}
	// 2609 复放复核 (2026-07-12): the live shape parks the state on the TYPE
	// lane (StateKind empty, TypeToken=io_wait) — the SAME typed table the
	// shape cell reads (#3 state family), one more consumer.
	if class := runtimeTraceCausalProjectionTypeTokenStateClass(node); class != "" {
		return strings.TrimSpace(runtimeTraceCausalProjectionTypeTokenStateWord(
			runtimeTraceCausalProjectionRefinedStateClass(node, class), zh))
	}
	return ""
}

// runtimeTraceProjDFamilyTailRedundant — DSTATE-REFINE arm c (件③, witness
// 96728 E14/E16, 2026-07-12): the D-family bare state tail (the 「· D-state」
// form) is REDUNDANT exactly when the row-1 cause name already speaks the
// same family word — a raw d_state_or_io_wait / io_wait Object (label
// D-state/iowait, refined D-state, iowait) or the D-family peer-relation
// wording (D-state/iowait(对端…)). Rows whose name does NOT speak the state
// (e.g. the generic 对端线程未解析 wording) keep the informative tail. Typed
// token/kind comparisons only — never a substring dedupe.
func runtimeTraceProjDFamilyTailRedundant(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "d_state", "d_sleep", "uninterruptible_sleep", "io_wait":
	default:
		return false
	}
	switch runtimeTraceCausalProjectionCanonicalNode(node.Object) {
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait", "io_wait":
		// 修复轮 P3-2: the fragmented family token speaks the same merged
		// word on the name lane (same zh label family) — same redundancy.
		return true
	}
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		switch runtimeTraceCausalProjectionUnresolvedPeerKindNode(node) {
		case "d_state_or_io_wait", "d_state_refined":
			return true
		}
	}
	if runtimeTraceCausalProjectionResolvedPeerObjectKind(node) == "d_state_or_io_wait" {
		return true
	}
	return false
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

// runtimeTraceCausalProjectionImpactPointBareState (PTV8-RCR-B, UXA 域A #23,
// 2026-07-08): reports whether ONE 影响点 token is a bare scheduler-state
// token (exact canonical state-word-table hit — precise typed set, never a
// substring heuristic). The 影响点 slot promises "who was impacted"; a state
// token there reads as "the impact point is sleeping" while the row's state
// is already carried by the icon + state word — the tag suppresses such
// tokens (display-only; the detail block's 关系/影响点 lines keep the full
// roster, zero information loss).
func runtimeTraceCausalProjectionImpactPointBareState(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, "/") {
		return false
	}
	return runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: token}, true) != ""
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

// runtimeTraceCausalProjectionSemanticSpanRow is the ONE semantic-span row
// predicate the display gates share (typed Role enum / exact producer
// predicate — never prose). Extracted for F1 (§22 PTV7-SPN): after the span
// name consumption gate relaxed to SpanName non-empty, the semantic gate's
// only remaining display job is the wider objectLimit split.
func runtimeTraceCausalProjectionSemanticSpanRow(node types.TraceCausalProjectionNode) bool {
	return node.Role == types.TraceCausalRoleSemanticSpan ||
		strings.TrimSpace(node.Predicate) == "trace_semantic_span"
}

// runtimeTraceCausalProjectionSpanNameObjectWord is THE shared F1 helper (§22
// PTV7-SPN P0, huadong_01 E21=H:ReceiveVsync): a generic row whose typed
// span_name note reached the display model (node.SpanName non-empty — precise
// boolean, soft display face, zero hard gates) puts the REAL name in the
// object word slot with the type word folded in parens:
// "H:ReceiveVsync(trace span)". Semantic-span rows return "" — their dedicated
// arms keep the pre-PTV7 name-only rendering (control pinned), and the
// semantic gate keeps only the objectLimit width split. Consumed by the three
// display faces (tree row / detail-table node cell / lossless full name) so
// they can never drift apart again.
func runtimeTraceCausalProjectionSpanNameObjectWord(node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceCausalProjectionSemanticSpanRow(node) {
		return ""
	}
	name := strings.TrimSpace(node.SpanName)
	if name == "" {
		return ""
	}
	name = strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(name, zh))
	typeWord := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if typeWord == "" || typeWord == name {
		return name
	}
	return name + "(" + typeWord + ")"
}

// runtimeTraceProjDiagnosticLaneNode reports whether the node's typed cause
// token rides the registry's diagnostic lane (§22 PTV7-SPN F5): exact
// canonical-token lookup against the causal-token registry — the semantic
// single source (internal/tracequery/causal_token_registry.go) — over the
// same TypeToken→Object→Predicate lane precedence the other typed-kind
// helpers use. Display split only (no candidate chip, no 0.000ms bar); the
// registry's Lane/Additivity/Subject positions are read, never written.
func runtimeTraceProjDiagnosticLaneNode(node types.TraceCausalProjectionNode) bool {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		spec, ok := tracequery.CausalTokenSpecFor(runtimeTraceCausalProjectionCanonicalNode(token))
		if ok && spec.Lane == tracequery.CausalLaneDiagnostic {
			return true
		}
	}
	return false
}

// runtimeTraceProjFoldSeedGapMasked (ELIM-GAP 件C, §29.104.15, 2026-07-16) is
// the ONE typed guard for word faces that read the token lanes on an overflow
// FOLD row: the fold constructor inherits overflow[0]'s Predicate verbatim
// (types/trace_causal_projection.go traceCausalProjectionOverflowFoldRow), so
// a MIXED fold whose seed member happened to be a trace_gap row wore the
// whole-row blind-spot word face (「窗内无调度数据·链止」 tag + ◌ glyph +
// detail 数据盲区 family word) over members that DO carry scheduler data
// (cust_total_del witness E22(+2): 2 valued members, max 24.000ms, one of
// them holding 榜位#12). The typed member truth is MergedAllDataGap
// (traceCausalProjectionDataGapRow over every member, G19): masked=true means
// the seed token must NOT speak for the fold; the fold keeps its neutral
// counted word face (其余N项(折叠) + the valued-split ×N accounting).
// Pure-gap folds (MergedAllDataGap) keep every blind-spot face byte-identically.
func runtimeTraceProjFoldSeedGapMasked(node types.TraceCausalProjectionNode) bool {
	return node.OnChainOverflowFold && !node.MergedAllDataGap
}

// runtimeTraceProjTraceGapNode is the exact typed token match for the
// trace_gap diagnostic marker (§22 PTV7-SPN F5 用户措辞裁定: 显示词=数据盲区,
// 行内披露=窗内无调度数据·链止) — same lane precedence as above, never a
// substring heuristic. ELIM-GAP 件C: on an overflow fold row the inherited
// seed token is masked unless every member is a data gap (typed truth
// outranks the seed word — the fold-level 词面读 typed 真相 gate).
func runtimeTraceProjTraceGapNode(node types.TraceCausalProjectionNode) bool {
	if runtimeTraceProjFoldSeedGapMasked(node) {
		return false
	}
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if runtimeTraceCausalProjectionCanonicalNode(token) == "trace_gap" {
			return true
		}
	}
	return false
}

// runtimeTraceProjIdleCadenceTag renders the ENG-2 (复核冷读 CP1-③,
// 2026-07-12) absorbed idle-cadence annotation for one node: the R1
// same-fact fold / ×N merge carried a typed pacing_idle / periodic_idle
// view onto this seat (IdleCadenceMS/Kind), and the seat states it inline —
// 「其中 X.XXXms 帧间空闲(等待下一帧)」 (EN keeps the raw token, D2
// discipline) — with the matching teaching legend mark. Rows whose OWN
// token IS the idle lane already speak the cause word and return ok=false
// (no redundant tag).
func runtimeTraceProjIdleCadenceTag(node types.TraceCausalProjectionNode, zh bool) (string, runtimeTraceProjMark, bool) {
	if node.IdleCadenceMS <= 0 || node.IdleCadenceKind == "" {
		return "", 0, false
	}
	// Redundancy exclusion keys on the CAUSE-WORD source (canonical Object)
	// only: a standalone idle row's display word already IS the idle label.
	// A same-fact SURVIVOR that merely ADOPTED the folded idle view's
	// TypeToken keeps its own scheduler-state word (e.g. Object=s_sleep), so
	// TypeToken/Predicate must NOT suppress the annotation (ENG-2 追修,
	// 2026-07-12 — the ×8 donghu seat carried the adopted token and lost the
	// wording to this very exclusion).
	switch runtimeTraceCausalProjectionCanonicalNode(node.Object) {
	case "pacing_idle", "periodic_idle":
		return "", 0, false
	}
	mark := runtimeTraceProjMarkPacingIdle
	if node.IdleCadenceKind == "periodic_idle" {
		mark = runtimeTraceProjMarkPeriodicIdle
	}
	word := node.IdleCadenceKind
	if zh {
		if label := runtimeTraceRootCauseTypeZHLabel(node.IdleCadenceKind); label != "" {
			word = label
		}
		return fmt.Sprintf("其中 %.3fms %s", node.IdleCadenceMS, word), mark, true
	}
	return fmt.Sprintf("of which %.3fms %s", node.IdleCadenceMS, word), mark, true
}

// runtimeTraceProjPeriodicIdleNode is the exact typed token match for the
// arm-c GENERIC periodic fork (复核 P2-1, 2026-07-12: 显示词=周期空闲(等待
// 下一周期信号)) — same lane precedence, never a substring heuristic.
func runtimeTraceProjPeriodicIdleNode(node types.TraceCausalProjectionNode) bool {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if runtimeTraceCausalProjectionCanonicalNode(token) == "periodic_idle" {
			return true
		}
	}
	return false
}

// runtimeTraceProjPacingIdleNode is the exact typed token match for the P9
// arm-c frame-pacing idle marker (§29.42 案1, 2026-07-12: 显示词=帧间空闲(
// 等待下一帧), 图例教学条随行出场) — same lane precedence as the trace_gap
// predicate above, never a substring heuristic.
func runtimeTraceProjPacingIdleNode(node types.TraceCausalProjectionNode) bool {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if runtimeTraceCausalProjectionCanonicalNode(token) == "pacing_idle" {
			return true
		}
	}
	return false
}

// runtimeTraceProjIdleRowKind resolves a row's typed cadence-idle lane
// (CAL-1 件⑤ PACE-ROW, §29.47.4②, 2026-07-12): the canonical idle token —
// "pacing_idle" / "periodic_idle" — when the row IS a cadence-idle row
// (standalone idle rank row, or the R1 same-fact survivor that adopted the
// idle view's TypeToken), "" otherwise. The token doubles as the typed mint
// witness for the 行2 「节拍吻合」 word: the engine mints these tokens only
// after proving the cadence fit (segment length ≈ frame/measured period,
// waker on the dispatch chain — P9 arm c), so the wordface never outruns
// the proof.
// runtimeTraceProjCaliberSideWord renders the ⌗ 口径旁栏 row-2 word (V2-P0,
// rank_order_v2_design_20260712.md §6.1, 2026-07-12): the value-class word
// comes from the SHARED registry arm (tracequery.CausalTokenCaliberSideClass
// — the same single implementation the engine ordinal guard and capacity
// side-lane consume); a row whose token no longer resolves keeps the generic
// form (the tier itself is the precise signal).
func runtimeTraceProjCaliberSideWord(node types.TraceCausalProjectionNode, zh bool) string {
	class := tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken))
	if zh {
		switch class {
		case tracequery.CausalCaliberSideCount:
			return "⌗口径旁栏·计数当量(非墙钟,不占序数)"
		case tracequery.CausalCaliberSideCompositeScore:
			return "⌗口径旁栏·综合评分(非墙钟,不占序数)"
		}
		return "⌗口径旁栏(非墙钟,不占序数)"
	}
	// zh-en 同词 for the kernel caliber tokens (the 计数当量 family word
	// renders byte-identically on both faces — same discipline as the
	// RCM-2 family legend, probe {"计数当量","计数当量"}).
	switch class {
	case tracequery.CausalCaliberSideCount:
		return "⌗ caliber-side · 计数当量 (count-equivalent, not wall clock, no ordinal)"
	case tracequery.CausalCaliberSideCompositeScore:
		return "⌗ caliber-side · 综合评分 (composite score, not wall clock, no ordinal)"
	}
	return "⌗ caliber-side (not wall clock, no ordinal)"
}

func runtimeTraceProjIdleRowKind(node types.TraceCausalProjectionNode) string {
	if runtimeTraceProjPacingIdleNode(node) {
		return "pacing_idle"
	}
	if runtimeTraceProjPeriodicIdleNode(node) {
		return "periodic_idle"
	}
	return ""
}

// runtimeTraceProjAllZeroFoldRow identifies the ×N(0.000–0.000) all-zero fold
// shape (§22 PTV7-SPN F5): a merged row none of whose members carried a
// measured value AND whose own display-impact fallback chain resolved to
// nothing. Pure typed numeric comparisons; such a row wears the same no-value
// form as the diagnostic lane (no candidate chip, no 0.000ms bar — the detail
// table already renders — there).
func runtimeTraceProjAllZeroFoldRow(node types.TraceCausalProjectionNode) bool {
	return node.MergedCount > 1 && node.MergedMinMS <= 0 && node.MergedMaxMS <= 0 &&
		runtimeTraceProjNodeDisplayImpact(node) <= 0
}

// runtimeTraceProjElimVerdictTokenWord — OMGCLEAN-1 件11 终版 (§29.175.17
// 判词文法定谳, 2026-07-20): THE ◎ diagnosis-face verdict-word consumption
// mapping — one family, one head-verdict word root; refinements are ·限定
// suffixes on the root, never a second root; bare kernel state words retire
// from the ◎ board face (they stay verbatim on the tree state face /
// state_churn / the detail table — 树面原词臂). Display-only consumption
// mapping over the typed token the ◎ class word actually derived from (the
// RowCauseWordToken / CauseDisplayToken dedupe identity — 精确信号, never a
// produced-word substring): the registry tokens themselves are untouched
// (supply_pressure display-split precedent; §7.2.1 red line). ok=false =
// outside the ruled table → the caller keeps every existing word path
// byte-identically (优先级反转·* / binder等待 / 语义类 / sleep 维持 by
// absence — sleep has no generic diagnosis reading, §29.175.16 负臂).
//
// Ruled table (§29.175.17 verbatim): 调度供给族 = 调度延迟 (scheduler_latency
// + runnable_wait 统一) / 调度延迟·碎片化 (fragmented_runnable_wait) /
// 调度延迟·CPU竞争 (cpu_pressure); IO与依赖族 = IO阻塞 (io_wait) /
// IO阻塞·不可中断(原因未证) (d_state_or_io_wait 素形 — the engine-refined
// non-IO proof keeps its own refined word: the proof outranks the merged
// root) / IO阻塞·设备延迟 (io_latency); 频率与热治理族 = 低频运行 root on the
// running 折算席 (the ·折算 qualifier already rides the row's caliber slot —
// one 折算 word source, the grammar completes as 低频运行 … ·折算).
func runtimeTraceProjElimVerdictTokenWord(node types.TraceCausalProjectionNode, token string, zh bool) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "scheduler_latency", "runnable_wait", "runnable":
		if zh {
			return "调度延迟", true
		}
		return "scheduling latency", true
	case "fragmented_runnable_wait":
		if zh {
			return "调度延迟·碎片化", true
		}
		return "scheduling latency·fragmented", true
	case "cpu_pressure":
		if zh {
			return "调度延迟·CPU竞争", true
		}
		return "scheduling latency·CPU contention", true
	case "io_wait":
		if zh {
			return "IO阻塞", true
		}
		return "IO blocking", true
	case "d_state_or_io_wait":
		if node.DStateRefinedNonIO {
			// The typed refined-D proof (io share zero ∧ blocked_reason 全覆盖)
			// says NON-IO — the IO阻塞 root would overclaim; the refined word
			// path stays (absence from the table keeps the existing word).
			return "", false
		}
		if zh {
			return "IO阻塞·不可中断(原因未证)", true
		}
		return "IO blocking·uninterruptible (cause unproven)", true
	case "io_latency":
		if zh {
			return "IO阻塞·设备延迟", true
		}
		return "IO blocking·device latency", true
	case "running", "fragmented_running":
		// 低频运行 root only on the DISCOUNTED running seat (折算席): the
		// supply-deficit arm, or the merged fold whose published eff is a
		// discount of its own raw account (the same typed eff≠projection
		// signal the row's ·折算 caliber word rides — one gate family). An
		// undiscounted running row keeps its existing word (absence never
		// wears a frequency claim).
		if runtimeTraceProjCauseRunningDeficitArm(node) ||
			(node.EffectiveImpactMS > 0 && node.ImpactMS > 0 &&
				!runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.ImpactMS)) {
			if zh {
				return "低频运行", true
			}
			return "low-frequency running", true
		}
	}
	return "", false
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
