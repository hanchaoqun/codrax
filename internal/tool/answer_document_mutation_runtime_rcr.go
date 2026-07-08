package tool

// answer_document_mutation_runtime_rcr.go — PTV8-RCR-A (ledger
// docs/design/real_trace_campaign_20260705.md §24/§24.1/§24.2/§24.3, user
// ruling 2026-07-07 "混乱无比" full display re-audit): the cause-node
// four-line grammar and the impact-form glyph/category single-source table.
//
// Four-line grammar (§24.1, all cause nodes isomorphic):
//   行1  glyph + 词位(状态构成|类型词) + ×N + bar + 窗口投影 + % + ⚠ + [E#(+E#)]
//   行2  类别·根因排序#N·置信
//   行3  有效归因 V = 分量(口径) a [+ 分量(口径) b]
//   行4+ 拆解子行「分量 原始 raw → 计入 x(口径)」 | 影响点清单
// Identity pins (§24.1, machine-checked on the REAL compile chain):
//   Σ计入 == 有效归因 == 行3 right-hand sum (the builder refuses to render a
//   non-balancing "=" row — fail-open to the plain effective tag);
//   行1 value == 窗口投影 == bar base (both read ONE display-impact source).
// Degenerate form (§24.2): single component with 计入==原始 → 行3 folds into
// 行2's tail (two-line form). Degradation is allowed, variants are not.
//
// Caliber-word closed set (§24.1/§24.2, four words, each with a mandatory
// on-demand legend entry — §24.1补): 全额 / 折算,按下游消费核 /
// 折算,按大核满频,下界 / 单次最大(共N次).
//
// Glyph table (§24.3, user ruling 2026-07-07 "成因 glyph 对应影响形态设计,
// 不要亮色"): the glyph column and the 行2 category-word column live in ONE
// typed table below (两列单源, hand-sync forbidden); legend entries for the
// glyph marks are generated from the same table. Three hard rules: text
// presentation (no VS16), single display cell per glyph (runewidth-checked by
// TestTraceProjectionImpactFormGlyphsSingleCellNoVS16), three-surface
// consistency. 🎯 → ⊚ (the tree loses its only colored emoji; 复核 F3:
// EAW-Neutral glyph, never the Ambiguous ◎).

import (
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjRootGlyph is the tree-root mark (§24.3 无亮色 hard rule +
// 复核 F3 裁定 2026-07-08: 🎯 → ⊚ U+229A, EAW=Neutral — the earlier ◎ U+25CE
// is East-Asian-Ambiguous and double-width on CJK terminals, escaping the
// single-cell rule). Consumed by the header composer, the legend entry and
// the single-cell width pin — one constant, three surfaces.
const runtimeTraceProjRootGlyph = "⊚"

// --- §24.3 impact-form table (glyph + category, ONE typed source) -------------

type runtimeTraceProjImpactForm int

const (
	runtimeTraceProjImpactFormNone runtimeTraceProjImpactForm = iota
	// ⚙ 运行占用·算力供给 (running / compute-supply family).
	runtimeTraceProjImpactFormRunning
	// ☾ 睡眠 (symptom, never a cause category).
	runtimeTraceProjImpactFormSleep
	// ⛓ IO阻塞族 (§24.3: io_latency 等 typed 事件归族,不再挂无意义 ◦).
	runtimeTraceProjImpactFormIOBlock
	// ⛓ D状态族 (SYM-2 §24.17 R2, 2026-07-08): D-state rows leave the IO
	// family's category word — same glyph (⛓ carries the D-state icon
	// semantics since PTV4), own 行2 word (D状态候选).
	runtimeTraceProjImpactFormDState
	// ⧖ 调度压力 (runnable / scheduler-latency family; §7.4 demand-side word).
	runtimeTraceProjImpactFormRunnable
	// ⊗ 锁竞争·持锁 (typed BlockingKind rows).
	runtimeTraceProjImpactFormLock
	// ⇅ 优先级反转 (inversion candidates).
	runtimeTraceProjImpactFormInversion
	// ✦ 确定性优化 (semantic spans + deterministic compile/verify classes).
	runtimeTraceProjImpactFormDeterministicOpt
	// ↯ 中断活动族 (irq/ipi/workqueue/dma-fence).
	runtimeTraceProjImpactFormInterrupt
	// ◌ 数据盲区 (trace_gap / missing_wakeup — data gaps, not causes).
	runtimeTraceProjImpactFormBlindSpot
	// binder 等待 (IPC wait-on-peer — 等待症状族, NOT block IO).
	// PTV8-RCR-C 复核收尾 (2026-07-08). EVOLUTION RECORD: binder_wait rode the
	// ⛓ IOBlock family since RCR-A — harmless while the family word had no
	// consumer, but the C7 FamilyWord lane would have called IPC "IO阻塞候选"
	// (typelabels 已定 binder等待). Own table row now; the glyph borrows the
	// ◦ 无形态兜底 (a dedicated IPC glyph extends the §24.3 closed set and
	// needs a user ruling — ledger 记账,本批不扩).
	runtimeTraceProjImpactFormBinderWait
	// ◦ 无形态兜底 (no dominant state, no typed family).
	runtimeTraceProjImpactFormFallback
)

// runtimeTraceProjImpactFormSpec is one row of the §24.3 single-source table:
// the glyph column and the 行2 category-word column MUST come from here —
// never hand-synced copies (两列单源, pinned by
// TestTraceProjectionImpactFormTableSingleSource).
type runtimeTraceProjImpactFormSpec struct {
	Form  runtimeTraceProjImpactForm
	Glyph string
	// CategoryZH/EN is the 行2 成因类别 family word ("" = the form mints no
	// category: sleep is a symptom, blind spots are data gaps, ◦ has no shape).
	// Lock rows override with their precise typed shape word (持锁/阻塞 split).
	CategoryZH string
	CategoryEN string
	// SemanticsZH/EN feed the generated legend entry for the form's glyph mark
	// (runtimeTraceProjImpactFormLegendEntries). Empty = no generated entry
	// (the form's glyph legend lives on a pre-existing pinned entry).
	SemanticsZH string
	SemanticsEN string
	// Mark is the glyph's NEW-7 legend mark; runtimeTraceProjMarkCount-safe.
	Mark runtimeTraceProjMark
	// GeneratedLegend: this spec generates its own catalog entry (the four new
	// §24.3 glyphs); the state-icon glyphs keep their PTV7-pinned entries.
	GeneratedLegend bool
}

// runtimeTraceProjImpactFormSpecs is the exhaustive §24.3 form directory.
func runtimeTraceProjImpactFormSpecs() []runtimeTraceProjImpactFormSpec {
	return []runtimeTraceProjImpactFormSpec{
		{Form: runtimeTraceProjImpactFormRunning, Glyph: "⚙",
			CategoryZH: "算力供给候选", CategoryEN: "compute-supply candidate",
			Mark: runtimeTraceProjMarkIconRunning},
		{Form: runtimeTraceProjImpactFormSleep, Glyph: "☾",
			Mark: runtimeTraceProjMarkIconSleep},
		{Form: runtimeTraceProjImpactFormIOBlock, Glyph: "⛓",
			CategoryZH: "IO阻塞候选", CategoryEN: "IO-blocking candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// SYM-2 §24.17 R2 (2026-07-08): the D-state family's OWN table row —
		// glyph/mark stay on the existing ⛓ D-state icon semantics (legend
		// entry unchanged), only the 行2 category word splits from IO阻塞候选.
		{Form: runtimeTraceProjImpactFormDState, Glyph: "⛓",
			CategoryZH: "D状态候选", CategoryEN: "D-state candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// EVOLUTION RECORD (SYM-2 §24.17 R2, 2026-07-08): 就绪排队候选 →
		// 调度压力候选 — the runnable family's 行2 word joins the §7.4
		// ruling-locked demand-side vocabulary (调度压力/需求积压; the ⧖ legend
		// keeps its state semantics 就绪等待).
		{Form: runtimeTraceProjImpactFormRunnable, Glyph: "⧖",
			CategoryZH: "调度压力候选", CategoryEN: "scheduling-pressure candidate",
			Mark: runtimeTraceProjMarkIconRunnable},
		{Form: runtimeTraceProjImpactFormLock, Glyph: "⊗",
			CategoryZH: "锁竞争·持锁", CategoryEN: "lock contention · holder",
			SemanticsZH: "锁竞争(持锁/被锁阻塞)", SemanticsEN: "lock contention (holding / blocked on a lock)",
			Mark: runtimeTraceProjMarkIconLock, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormInversion, Glyph: "⇅",
			CategoryZH: "优先级反转候选", CategoryEN: "priority-inversion candidate",
			SemanticsZH: "优先级反转(低优先级持有资源阻塞高优先级)", SemanticsEN: "priority inversion (a low-priority holder blocks a high-priority waiter)",
			Mark: runtimeTraceProjMarkIconInversion, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormDeterministicOpt, Glyph: "✦",
			CategoryZH: "确定性优化候选", CategoryEN: "deterministic-optimization candidate",
			Mark: runtimeTraceProjMarkSemanticSpan},
		{Form: runtimeTraceProjImpactFormInterrupt, Glyph: "↯",
			CategoryZH: "中断活动候选", CategoryEN: "interrupt-activity candidate",
			SemanticsZH: "中断活动(IRQ/IPI/工作队列等)", SemanticsEN: "interrupt activity (IRQ/IPI/workqueue …)",
			Mark: runtimeTraceProjMarkIconInterrupt, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormBlindSpot, Glyph: "◌",
			SemanticsZH: "数据缺失记号(窗内数据缺口,非成因)", SemanticsEN: "a data-gap marker (missing in-window data, not a cause)",
			Mark: runtimeTraceProjMarkIconBlindSpot, GeneratedLegend: true},
		// PTV8-RCR-C 复核收尾: binder IPC waits leave the ⛓ IO family — the
		// category word rides the typelabels binder等待 typed word (+候选 同族
		// 模式); the glyph borrows ◦ until a dedicated IPC glyph is ruled.
		{Form: runtimeTraceProjImpactFormBinderWait, Glyph: "◦",
			CategoryZH: "binder等待候选", CategoryEN: "binder-wait candidate",
			Mark: runtimeTraceProjMarkIconNoDominant},
		{Form: runtimeTraceProjImpactFormFallback, Glyph: "◦",
			Mark: runtimeTraceProjMarkIconNoDominant},
	}
}

// runtimeTraceProjImpactFormSpecFor resolves one table row by form.
func runtimeTraceProjImpactFormSpecFor(form runtimeTraceProjImpactForm) (runtimeTraceProjImpactFormSpec, bool) {
	for _, spec := range runtimeTraceProjImpactFormSpecs() {
		if spec.Form == form {
			return spec, true
		}
	}
	return runtimeTraceProjImpactFormSpec{}, false
}

// runtimeTraceProjImpactFormLegendEntries generates the catalog entries of
// the GeneratedLegend specs from the table itself (§24.3 图例条目随 glyph 表
// 单源生成) — glyph and semantics interpolate from the spec, so a table edit
// moves the row face and the legend together (two-source drift bites red).
func runtimeTraceProjImpactFormLegendEntries() []runtimeTraceProjLegendEntry {
	var out []runtimeTraceProjLegendEntry
	for _, spec := range runtimeTraceProjImpactFormSpecs() {
		if !spec.GeneratedLegend {
			continue
		}
		out = append(out, runtimeTraceProjLegendEntry{
			Mark:  spec.Mark,
			Group: runtimeTraceProjLegendGroupMark,
			ZH:    "- `" + spec.Glyph + "` = " + spec.SemanticsZH + "。",
			EN:    "- `" + spec.Glyph + "` = " + spec.SemanticsEN + ".",
		})
	}
	return out
}

// runtimeTraceProjImpactFormTokenFamily maps a canonical typed token onto its
// §24.3 impact-form family. Exact typed-token membership only — unmapped
// tokens return None and the caller falls through to the state lane.
func runtimeTraceProjImpactFormTokenFamily(token string) runtimeTraceProjImpactForm {
	switch token {
	case "io_latency", "io_wait",
		"io_pressure", "io_burst_episode", "block_io_by_inode",
		"file_io_hot_inode", "page_cache_churn":
		return runtimeTraceProjImpactFormIOBlock
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		// SYM-2 §24.17 R2 (2026-07-08): the D-without-observed-IO producer
		// compound (rootTypeForDominantState: StateDSleep ∧ io==0) speaks its
		// own D状态候选 word; observed-IO waits stay on the ⛓ IO family above.
		return runtimeTraceProjImpactFormDState
	case "binder_wait":
		// PTV8-RCR-C 复核收尾: IPC wait-on-peer is its own family (等待症状族)
		// — never block IO (the C7 lane must say binder等待候选, not IO阻塞候选).
		return runtimeTraceProjImpactFormBinderWait
	case "irq_burst", "irq_activity", "ipi_activity", "workqueue_activity", "dma_fence_activity":
		return runtimeTraceProjImpactFormInterrupt
	case "trace_gap", "missing_wakeup":
		return runtimeTraceProjImpactFormBlindSpot
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile":
		return runtimeTraceProjImpactFormDeterministicOpt
	case "compute_supply", "low_frequency", "cpu_frequency_limit", "cpu_affinity_or_cpuset",
		"running", "fragmented_running":
		return runtimeTraceProjImpactFormRunning
	case "runnable_wait", "fragmented_runnable_wait", "scheduler_latency",
		"cpu_pressure", "supply_pressure":
		return runtimeTraceProjImpactFormRunnable
	case "sleep_wait", "fragmented_sleep_wait":
		return runtimeTraceProjImpactFormSleep
	}
	return runtimeTraceProjImpactFormNone
}

// runtimeTraceProjImpactFormFamilyWord (PTV8-RCR-C, §24.12 C7, 2026-07-08)
// resolves the §24.3 family word for a row whose TYPED token belongs to an
// impact-form family — the detail 影响形态 cell must never claim 未分类(该行
// 无具体状态/类型词) beside a 类型: binder_wait line (cmp_78_01 rank#1 lead
// 行明细自相矛盾, 禁「未分类」冒名). Category-bearing families speak the
// table's 行2 column (两列单源); the blind-spot family speaks its data-gap
// semantics; families without a category word (sleep rides the state lane,
// ◦ has genuinely no word) return "" and the generic arm stays honest for
// genuinely word-less rows.
func runtimeTraceProjImpactFormFamilyWord(node types.TraceCausalProjectionNode, zh bool) string {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		form := runtimeTraceProjImpactFormTokenFamily(runtimeTraceCausalProjectionCanonicalNode(token))
		if form == runtimeTraceProjImpactFormNone {
			continue
		}
		if form == runtimeTraceProjImpactFormBlindSpot {
			if zh {
				return "数据盲区(窗内数据缺口,非成因)"
			}
			return "data blind spot (missing in-window data, not a cause)"
		}
		if spec, ok := runtimeTraceProjImpactFormSpecFor(form); ok && spec.CategoryZH != "" {
			if zh {
				return spec.CategoryZH
			}
			return spec.CategoryEN
		}
		return ""
	}
	return ""
}

// runtimeTraceProjImpactFormForNode classifies one node onto the §24.3 form.
// Typed precedence (never prose): semantic kind → lock → inversion → the
// node's OWN scheduler state (the pre-RCR glyph precedence, byte-stable for
// every stateful row) → typed token family (TypeToken → Object → Predicate,
// the registry lane order — this is the §24.3 "IO延迟等 typed 事件归族" arm
// for STATELESS rows) → fallback.
func runtimeTraceProjImpactFormForNode(node types.TraceCausalProjectionNode, kind string) runtimeTraceProjImpactForm {
	if kind == runtimeTraceProjTreeRowSemantic {
		return runtimeTraceProjImpactFormDeterministicOpt
	}
	if strings.TrimSpace(node.BlockingKind) != "" {
		return runtimeTraceProjImpactFormLock
	}
	if runtimeTraceCausalProjectionInversionRow(node) {
		return runtimeTraceProjImpactFormInversion
	}
	if node.IsSleepState() {
		return runtimeTraceProjImpactFormSleep
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running":
		return runtimeTraceProjImpactFormRunning
	case "runnable":
		return runtimeTraceProjImpactFormRunnable
	case "d_state", "d_sleep", "uninterruptible_sleep":
		// SYM-2 §24.17 R2 (2026-07-08): D-state STATE rows split off the IO
		// family word (same ⛓ glyph/legend); io_wait keeps the IO family.
		return runtimeTraceProjImpactFormDState
	case "io_wait":
		return runtimeTraceProjImpactFormIOBlock
	}
	switch runtimeTraceCausalProjectionTypeTokenStateClass(node) {
	case "running":
		return runtimeTraceProjImpactFormRunning
	case "runnable":
		return runtimeTraceProjImpactFormRunnable
	case "s_sleep":
		return runtimeTraceProjImpactFormSleep
	case "d_state_or_io_wait":
		return runtimeTraceProjImpactFormDState
	case "io_wait":
		return runtimeTraceProjImpactFormIOBlock
	}
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		if form := runtimeTraceProjImpactFormTokenFamily(runtimeTraceCausalProjectionCanonicalNode(token)); form != runtimeTraceProjImpactFormNone {
			return form
		}
	}
	return runtimeTraceProjImpactFormFallback
}

// --- §24.1/§24.2 cause-node four-line grammar ---------------------------------

// runtimeTraceProjCauseNodeRow is the 成因节点 identity gate (§24 用户裁定①:
// 成因行身份 = 根因排序参赛身份): a data row that entered the root-cause
// ranking (typed engine Rank, incl. the RNB fold-adopted rank) or is an
// inversion composite carrying gated components. Precise signals only.
func runtimeTraceProjCauseNodeRow(row runtimeTraceProjTreeRow) bool {
	if !row.HasData || row.Node.OnChainOverflowFold {
		return false
	}
	if row.Node.Rank > 0 || len(row.RankFoldPeers) > 0 {
		return true
	}
	// RCM-2 (§24.7.1 ②/§24.10, 2026-07-08): an engine family merge IS a
	// ranking participant by construction (合并量参赛 — the fold exists to
	// enter the boards as ONE contender), so its display identity is the cause
	// grammar even on the observation-lane copy whose seat rides the
	// background comprehensive board (行2 背景榜位#N) or is not yet numbered.
	if runtimeTraceProjFamilyRow(row.Node) {
		return true
	}
	return runtimeTraceCausalProjectionInversionRow(row.Node) &&
		(row.Node.GatedRunnableMS > 0 || row.Node.GatedRunningDeficitMS > 0)
}

// runtimeTraceProjCauseCategoryWord resolves the 行2 category word. Typed
// relocation, never a string dedupe: lock rows speak their precise shape word
// (持锁/阻塞 split), typed non-state shape words relocate from the retired
// row-tail slot, pure state rows take the form table's family column.
// relocated=true means the shape-cell word MOVED here and the 行尾形态词 slot
// stays empty (§24.2 行尾形态词撤).
func runtimeTraceProjCauseCategoryWord(node types.TraceCausalProjectionNode, kind string, zh bool) (word string, relocated bool) {
	form := runtimeTraceProjImpactFormForNode(node, kind)
	switch form {
	case runtimeTraceProjImpactFormLock:
		word, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
		return word, true
	case runtimeTraceProjImpactFormInversion:
		if zh {
			return runtimeTraceRootCauseTypeZHLabel("priority_inversion_candidate"), true
		}
		return "priority_inversion_candidate", true
	case runtimeTraceProjImpactFormDeterministicOpt:
		// RCM-2 (§24.1 类别词族 + §24.10, 2026-07-08): deterministic-
		// optimization contenders (semantic span families included) speak the
		// form table's category word on 行2 (确定性优化候选) — the semantic
		// span-shape cell (语义优化span·<class>) keeps its own tag seat and
		// never relocates into the identity line. Rank-lane deterministic rows
		// already resolved here through the generic fallthrough below —
		// byte-identical for them (same table row).
		if spec, ok := runtimeTraceProjImpactFormSpecFor(form); ok {
			if zh {
				return spec.CategoryZH, false
			}
			return spec.CategoryEN, false
		}
	}
	// Typed non-state shape words (IO阻塞候选 / 页缓存抖动 / 中断突发 …)
	// relocate whole from the shape cell; a shape cell that carries a pure
	// scheduler-state word (typed class) or the generic fallback stays put and
	// the table's family column speaks instead.
	shape, generic := runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
	stateWord := strings.TrimSpace(node.StateKind) != "" ||
		runtimeTraceCausalProjectionTypeTokenStateClass(node) != ""
	if shape != "" && !generic && !stateWord {
		return shape, true
	}
	if spec, ok := runtimeTraceProjImpactFormSpecFor(form); ok {
		if zh {
			return spec.CategoryZH, false
		}
		return spec.CategoryEN, false
	}
	return "", false
}

// runtimeTraceProjCauseRankConfidence resolves the 行2 榜位/置信 pair: the
// fold-adopted rank plus, when a rank-lane twin folded in (RNB), THAT row's
// confidence (the ranking identity rides the rank lane — §24.2 fold peer 数据
// 入行2 榜位).
func runtimeTraceProjCauseRankConfidence(row runtimeTraceProjTreeRow) (int, float64) {
	rank := row.Node.Rank
	confidence := row.Node.Confidence
	for _, peer := range row.RankFoldPeers {
		if peer.Rank > 0 && (rank <= 0 || peer.Rank < rank) {
			rank = peer.Rank
		}
		if peer.Confidence > 0 {
			confidence = peer.Confidence
		}
	}
	return rank, confidence
}

// runtimeTraceProjCauseEventFoldRow is the §24.2 event-class form gate: a
// chain-universe cause node whose merged instances published a per-instance
// MAX equal (at print precision) to the row's effective attribution — the
// 有效归因 V = 单次最大(a–b,共N次) identity holds by typed data, so the ×N
// count rises into 行1 and the range rides 行3. Shared by the name composer
// and the structured builder (one gate, no drift).
func runtimeTraceProjCauseEventFoldRow(row runtimeTraceProjTreeRow) bool {
	node := row.Node
	if !runtimeTraceProjCauseNodeRow(row) || !runtimeTraceProjChainUniverseRowKind(row.Kind) {
		return false
	}
	if node.PeriodicSource || runtimeTraceProjEffectiveInherited(node) ||
		runtimeTraceCausalProjectionInversionRow(node) {
		return false
	}
	// PTV8-RCR-C §24.9 G1: a §20.2 running-deficit node is its OWN form — an
	// eff==deficit that coincides with the merged MAX must still speak the
	// 按大核满频 caliber, never 单次最大 (one gate ordering, shared by the
	// name composer's ×N suffix and the structured builder).
	if runtimeTraceProjCauseRunningDeficitArm(node) {
		return false
	}
	if _, source := runtimeTraceProjNodeDisplayImpactSource(node); source == runtimeTraceProjImpactSourceEffective {
		return false
	}
	return node.MergedCount > 1 && node.MergedMaxMS > 0 && node.EffectiveImpactMS > 0 &&
		runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.MergedMaxMS)
}

// runtimeTraceProjAttributionComponent is one 行3/子行 component of the
// effective attribution: 计入 under its own caliber, with the known 原始.
type runtimeTraceProjAttributionComponent struct {
	Word string // canonical component word (runnable / running / …)
	// RawMS is the component's 原始 magnitude (0 = unknown; the sub-row then
	// cannot render and the whole "=" form fails open).
	RawMS float64
	// InMS is the 计入 magnitude (Σ InMS must equal the effective total at
	// print precision — the §24.1 identity pin).
	InMS float64
	// CaliberShort rides 行3 ("全额"/"折算"); CaliberFull rides the sub-row
	// parenthesis (closed set: 全额 / 折算,按下游消费核 / 折算,按大核满频,下界).
	CaliberShort string
	CaliberFull  string
	// Marks to emit when the component renders (caliber legend on demand).
	Marks []runtimeTraceProjMark
}

// runtimeTraceProjCauseStructured is the built four-line-grammar material for
// one cause node (rows 2..N; 行1 stays on the metric lane — same display
// impact, same bar base: the §24.1 second identity holds by construction).
type runtimeTraceProjCauseStructured struct {
	IdentityRow string   // 行2 (with the degenerate 行3 tail folded in, if any)
	Breakdown   string   // 行3 ("" = degenerate or no decomposition)
	SubRows     []string // 行4+ 拆解子行
	// NameXNSuffix moves the ×N count into 行1's 词位 (§24.2 ×N 上移行1);
	// non-empty only on the event form.
	NameXNSuffix string
	// ConsumedMergedTag: the event form carries the (a–b,共N次) range on 行3 —
	// the legacy ×N(a–b) tag stays off this row.
	ConsumedMergedTag bool
	// ConsumedEffective: 行2/行3 already carry the effective value — the
	// legacy 有效归因X tag stays off this row.
	ConsumedEffective bool
	// SuppressShapeWord: the category word relocated from the shape-cell slot
	// (§24.2 行尾形态词撤) — the row-tail state/shape tag stays off.
	SuppressShapeWord bool
}

func runtimeTraceProjFmtMS(v float64) string { return fmt.Sprintf("%.3fms", v) }

// runtimeTraceProjRound3Equal is the print-precision identity check: two
// magnitudes are identical exactly when their %.3f renderings agree.
func runtimeTraceProjRound3Equal(a, b float64) bool {
	return math.Round(a*1000) == math.Round(b*1000)
}

// runtimeTraceProjInversionComponents builds the inversion composite's typed
// components (RCX² per-component rulers: runnable counted IN FULL, only the
// running deficit rides the downstream-consumer-core fold). ok=false when a
// component's 原始 is unknowable or the printed identity Σ计入==V would not
// balance — the "=" form then fails open (never a fabricated number).
func runtimeTraceProjInversionComponents(node types.TraceCausalProjectionNode, zh bool) ([]runtimeTraceProjAttributionComponent, float64, bool) {
	// The 行3 head claims 有效归因 — only the ENGINE-PUBLISHED effective may
	// wear that word (显示≠归因: an unpublished effective is never fabricated
	// from the component sum; such nodes keep 行2 + the detail-block
	// composition text without a total claim).
	if node.EffectiveImpactMS <= 0 || node.PeriodicSource {
		return nil, 0, false
	}
	total := runtimeTraceProjInversionGatedTotalMS(node)
	if total <= 0 {
		return nil, 0, false
	}
	var components []runtimeTraceProjAttributionComponent
	if node.GatedRunnableMS > 0 {
		// Producer contract (tracequery GatedRunnableMs: "runnable time
		// counted in full"): the component's 原始 IS its 计入 — the 全额
		// caliber claim holds by typed definition, never via the row's
		// whole-window RunnableMS (which may cover more than this segment).
		raw := node.GatedRunnableMS
		full := "全额"
		if !zh {
			full = "in full"
		}
		components = append(components, runtimeTraceProjAttributionComponent{
			Word: "runnable", RawMS: raw, InMS: node.GatedRunnableMS,
			CaliberShort: full, CaliberFull: full,
			Marks: []runtimeTraceProjMark{runtimeTraceProjMarkCaliberFull},
		})
	}
	if node.GatedRunningDeficitMS > 0 {
		raw := 0.0
		if strings.TrimSpace(strings.ToLower(node.StateKind)) == "running" ||
			runtimeTraceCausalProjectionTypeTokenStateClass(node) == "running" {
			raw = runtimeTraceProjNodeDisplayImpact(node)
		}
		if raw <= 0 && node.SupplyFoldComputed {
			raw = runtimeTraceProjSupplyFoldRunningMS(node)
		}
		if raw <= 0 {
			return nil, 0, false // unknown 原始 — the sub-row cannot render
		}
		short, full := "折算", "折算,按下游消费核"
		if !zh {
			short, full = "discounted", "discounted, at the downstream consumer core"
		}
		marks := []runtimeTraceProjMark{runtimeTraceProjMarkCaliberConsumerCore}
		// CAP (§26 C3): the discounted component's sub-row parenthesis carries
		// the typed capability caliber (行3 keeps the short closed-set word).
		full += runtimeTraceProjCapabilityCaliberSuffix(node.GatedCapabilitySource, zh)
		if capMark, ok := runtimeTraceProjCapabilityCaliberMark(node.GatedCapabilitySource); ok {
			marks = append(marks, capMark)
		}
		components = append(components, runtimeTraceProjAttributionComponent{
			Word: "running", RawMS: raw, InMS: node.GatedRunningDeficitMS,
			CaliberShort: short, CaliberFull: full,
			Marks: marks,
		})
	}
	if len(components) == 0 {
		return nil, 0, false
	}
	sum := 0.0
	for _, c := range components {
		sum += math.Round(c.InMS * 1000)
	}
	if sum != math.Round(total*1000) {
		return nil, 0, false // identity would not balance at print precision
	}
	return components, total, true
}

// runtimeTraceProjAttributionEquation is THE 行3 equation template (复核
// FAIL-2, 2026-07-08): "V = 分量(口径) x [+ 分量(口径) y]" — the conclusion
// line, the fence 行3 and the detail block's 有效归因构成 line all render
// THIS one string (three hand copies could drift joiner/format apart with
// every pin still green).
func runtimeTraceProjAttributionEquation(total float64, components []runtimeTraceProjAttributionComponent) string {
	parts := make([]string, 0, len(components))
	for _, c := range components {
		parts = append(parts, fmt.Sprintf("%s(%s) %s", c.Word, c.CaliberShort, runtimeTraceProjFmtMS(c.InMS)))
	}
	return runtimeTraceProjFmtMS(total) + " = " + strings.Join(parts, " + ")
}

// runtimeTraceProjAttributionSubRows is THE 拆解子行 template (FAIL-2 twin):
// 「分量 原始 raw → 计入 x(口径)」 — fence sub-rows and the detail block's
// 拆解 line share it.
func runtimeTraceProjAttributionSubRows(components []runtimeTraceProjAttributionComponent, zh bool) []string {
	out := make([]string, 0, len(components))
	for _, c := range components {
		if zh {
			out = append(out, fmt.Sprintf("%s 原始 %s → 计入 %s(%s)", c.Word, runtimeTraceProjFmtMS(c.RawMS), runtimeTraceProjFmtMS(c.InMS), c.CaliberFull))
		} else {
			out = append(out, fmt.Sprintf("%s raw %s → counted %s (%s)", c.Word, runtimeTraceProjFmtMS(c.RawMS), runtimeTraceProjFmtMS(c.InMS), c.CaliberFull))
		}
	}
	return out
}

// runtimeTraceProjInversionDegenerateSingleFull is the §24.1 degeneration
// gate on an inversion composite (复核 F4 裁定 2026-07-08, "允许退化" read as
// mandatory): exactly ONE component, counted at its full raw amount
// (计入==原始 — by producer contract the runnable(全额) component) → 行3
// folds into 行2's tail and the sub-row is omitted (two-line form). Multiple
// components or a discounted 计入 keep the four-line form.
func runtimeTraceProjInversionDegenerateSingleFull(components []runtimeTraceProjAttributionComponent) bool {
	return len(components) == 1 && components[0].Word == "runnable" &&
		runtimeTraceProjRound3Equal(components[0].InMS, components[0].RawMS)
}

// runtimeTraceProjInversionStateCompositionWord is 行1's 词位 for an
// inversion cause node (§24 用户裁定① / task-verbatim E7 case): the state
// composition "runnable+running" when both gated components are live, the
// single live component's state word otherwise, "" when the composite carries
// no component (the caller keeps the legacy name).
func runtimeTraceProjInversionStateCompositionWord(node types.TraceCausalProjectionNode) string {
	if !runtimeTraceCausalProjectionInversionRow(node) {
		return ""
	}
	switch {
	case node.GatedRunnableMS > 0 && node.GatedRunningDeficitMS > 0:
		return "runnable+running"
	case node.GatedRunnableMS > 0:
		return "runnable"
	case node.GatedRunningDeficitMS > 0:
		return "running"
	}
	return ""
}

// runtimeTraceProjCauseRunningDeficitArm (PTV8-RCR-C, §24.9 维度A gap② /
// §20.2 显示半场, 2026-07-08) gates the pure-running supply-deficit form: a
// NON-inversion running-typed cause node whose supply fold ran and whose
// engine-published effective IS the big-cluster-fmax deficit (print-precision
// identity — the §20.2 running arm publishes EffectiveImpactMs =
// SupplyFoldDeficitMs, authoritative). The identity check is the fail-open
// guard: an effective from any other lane must never wear the 按大核满频
// caliber word (显示≠归因).
func runtimeTraceProjCauseRunningDeficitArm(node types.TraceCausalProjectionNode) bool {
	if runtimeTraceCausalProjectionInversionRow(node) || !node.SupplyFoldComputed {
		return false
	}
	running := strings.TrimSpace(strings.ToLower(node.StateKind)) == "running" ||
		runtimeTraceCausalProjectionTypeTokenStateClass(node) == "running"
	return running && node.SupplyFoldDeficitMS > 0 &&
		runtimeTraceProjSupplyFoldRunningMS(node) > 0 &&
		runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.SupplyFoldDeficitMS)
}

// runtimeTraceProjCauseStructuredParts builds rows 2..N for one cause node.
// marks are emitted HERE (at the wording's real emission site) so the caliber
// legend entries render exactly on demand (§24.1补).
func runtimeTraceProjCauseStructuredParts(row runtimeTraceProjTreeRow, zh bool) (runtimeTraceProjCauseStructured, bool) {
	if !runtimeTraceProjCauseNodeRow(row) {
		return runtimeTraceProjCauseStructured{}, false
	}
	node := row.Node
	out := runtimeTraceProjCauseStructured{}
	sep := "·"
	if !zh {
		sep = " · "
	}
	category, relocated := runtimeTraceProjCauseCategoryWord(node, row.Kind, zh)
	out.SuppressShapeWord = relocated
	var identity []string
	if category != "" {
		identity = append(identity, category)
	}
	rank, confidence := runtimeTraceProjCauseRankConfidence(row)
	if rank > 0 {
		if zh {
			identity = append(identity, fmt.Sprintf("根因排序#%d", rank))
		} else {
			identity = append(identity, fmt.Sprintf("root-cause rank #%d", rank))
		}
		// PTV8-RCR-C (§24.13 裁定二后半): the multi-board window tag binds to
		// the seat ordinal (根因排序#1·窗X — stamped at model build only when
		// ≥2 rank boards render; the single-board form carries none).
		if chip := strings.TrimSpace(row.RankWindowChip); chip != "" {
			identity = append(identity, chip)
			row.marks.mark(runtimeTraceProjMarkRankSeatWindow)
		}
	} else if node.BackgroundRank > 0 {
		// RCM-2 D2 (§24.10 链上 tier 道与非链背景综合排序道同规, 2026-07-08):
		// a contender seated on the background comprehensive board wears its
		// typed seat the way an on-chain seat wears 根因排序#N — never the
		// on-chain word for an off-chain seat (板别如实).
		if zh {
			identity = append(identity, fmt.Sprintf("背景榜位#%d", node.BackgroundRank))
		} else {
			identity = append(identity, fmt.Sprintf("background seat #%d", node.BackgroundRank))
		}
	}
	if tier := runtimeTraceProjConfidenceTier(confidence, zh); tier != "" {
		if zh {
			identity = append(identity, "置信"+tier)
		} else {
			identity = append(identity, "confidence "+tier)
		}
	}
	// PTV8-RCR-C (§24.9 G3 链上L# 收编): a structured cause node carries its
	// chain layer INSIDE 行2 (类别·根因排序#N·置信·链上L#) — the old Seg-20
	// chip lane stays for hop/non-cause chain rows only (same shared gate;
	// §24.12 C6: the depthless unattached shape speaks the 三面同词 word).
	if runtimeTraceProjChainDepthChipEligible(row) {
		identity = append(identity, runtimeTraceProjChainDepthChipWord(row, zh))
		row.marks.mark(runtimeTraceProjMarkChainDepthChip)
		if runtimeTraceProjDepthlessUnattachedRow(row) {
			row.marks.mark(runtimeTraceProjMarkChainSeatUnattached)
		}
	}
	effectiveWord := "有效归因"
	if !zh {
		effectiveWord = "attribution"
	}
	impact := runtimeTraceProjNodeDisplayImpact(node)
	_, impactSource := runtimeTraceProjNodeDisplayImpactSource(node)
	effective := node.EffectiveImpactMS
	eligible := runtimeTraceProjChainUniverseRowKind(row.Kind) && effective > 0 &&
		!node.PeriodicSource && !runtimeTraceProjEffectiveInherited(node) &&
		impactSource != runtimeTraceProjImpactSourceEffective
	handled := false
	if runtimeTraceProjFamilyRow(node) {
		// RCM-2 D2 (§24.10/§24.12 维度A ④, 2026-07-08): the family form OWNS
		// the whole 行3/子行 seat — 行3 = 「有效归因 V = 合计(共N段,同线程)」
		// (fifth caliber word per the typed fold-caliber ladder; union < Σ
		// appends the raw-sum disclosure), 子行 = roster top-3 + counted
		// trailer (§24.7.1 ① 区分键不能丢 + roster 折叠必带计数披露). It runs
		// on EVERY row kind (semantic families are not chain-universe rows)
		// and takes precedence over the inversion/event/degenerate arms — a
		// family total must never be re-worded 全额/单次最大. Identity pin:
		// V == 发布值 (engine effective when published, else the display
		// impact); when the two channels BOTH exist and disagree at print
		// precision the "=" claim fails open (拒渲绝不造数 — the plain
		// effective tag then stays on its legacy lane).
		word, caliberMark, wordOK := runtimeTraceProjFamilyCaliberWord(node, zh)
		v := runtimeTraceProjFamilyPublishedMS(node)
		balanced := effective <= 0 || impact <= 0 || runtimeTraceProjRound3Equal(effective, impact)
		if wordOK && v > 0 && balanced && !node.PeriodicSource {
			out.Breakdown = fmt.Sprintf("%s %s = %s%s", effectiveWord,
				runtimeTraceProjFmtMS(v), word, runtimeTraceProjFamilySumDisclosure(node, zh))
			out.SubRows = runtimeTraceProjFamilyRosterSubRows(node, zh)
			row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
			row.marks.mark(caliberMark)
			out.ConsumedEffective = true
		}
		handled = true
	}
	if !handled && eligible && runtimeTraceCausalProjectionInversionRow(node) {
		if components, total, ok := runtimeTraceProjInversionComponents(node, zh); ok {
			switch {
			case runtimeTraceProjInversionDegenerateSingleFull(components):
				// 复核 F4 (§24.1 退化规则按字面执行): single runnable(全额)
				// component with 计入==原始 — 行3 folds into 行2's tail,
				// sub-row omitted (统一逻辑优先; the identity total==InMS is
				// implied by the balance check in the components builder).
				full := "全额"
				if !zh {
					full = "in full"
				}
				identity = append(identity, fmt.Sprintf("%s %s(%s)", effectiveWord, runtimeTraceProjFmtMS(total), full))
				row.marks.mark(runtimeTraceProjMarkCaliberFull)
			default:
				out.SubRows = runtimeTraceProjAttributionSubRows(components, zh)
				out.Breakdown = effectiveWord + " " + runtimeTraceProjAttributionEquation(total, components)
				row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
				for _, c := range components {
					for _, m := range c.Marks {
						row.marks.mark(m)
					}
				}
			}
			out.ConsumedEffective = true
			handled = true
		} else if node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0 {
			// 复核 FAIL-1 (fail-open lossless mirror): the detail block will
			// render the composition text (runnable …(全额)+ running 折算
			// …(按下游消费核折算)) — the caliber legend entries follow the
			// words wherever they reach the reader.
			row.marks.mark(runtimeTraceProjMarkCaliberFull)
			row.marks.mark(runtimeTraceProjMarkCaliberConsumerCore)
			// CAP (§26 C3): the composition text's discounted component
			// carries the capability disclosure — its legend follows.
			if capMark, ok := runtimeTraceProjCapabilityCaliberMark(node.GatedCapabilitySource); ok && node.GatedRunningDeficitMS > 0 {
				row.marks.mark(capMark)
			}
		}
		// else fail-open: no balancing decomposition — the degenerate arm
		// below may still fold the effective into 行2's tail (计入==原始);
		// otherwise the plain effective tag stays on the legacy lane. The
		// 机制构成 sentence stays retired on inversion cause nodes regardless
		// (§24 ②, enforced at the supply-fold tag site).
	}
	if !handled && eligible && runtimeTraceProjCauseRunningDeficitArm(node) {
		// PTV8-RCR-C §24.9 G1 (§20.2 纯 running 缺口臂): the third closed-set
		// caliber word gets its structured producer — 行3 = 有效归因 V =
		// running(折算,按大核满频) V; 子行 = running 原始(ideal+deficit 恒等式)
		// → 计入 deficit[,下界 当 UnknownMS>0]. The legacy bare 有效归因X tag
		// (opendir_78 E8 gap②) dies via ConsumedEffective; identity Σ计入==V
		// holds by construction (single component, InMS == engine effective).
		// CAP 复核 F1: the basis word follows the actual reference cluster
		// (byte-identical for the big-class basis).
		refWord, _ := runtimeTraceProjFoldReferenceClusterWord(node.SupplyFoldReferenceClass, zh)
		short := "折算,按" + refWord + "满频"
		if !zh {
			short = "discounted, at " + refWord + " fmax"
		}
		full := short
		componentMarks := []runtimeTraceProjMark{runtimeTraceProjFoldReferenceMark(node.SupplyFoldReferenceClass)}
		if node.SupplyFoldUnknownMS > 0 {
			if zh {
				full += ",下界"
			} else {
				full += ", lower bound"
			}
			componentMarks = append(componentMarks, runtimeTraceProjMarkCaliberLowerBound)
		}
		// CAP (§26 C3): the sub-row parenthesis carries the fold's typed
		// capability caliber (行3 keeps the short closed-set word).
		full += runtimeTraceProjCapabilityCaliberSuffix(node.SupplyFoldCapabilitySource, zh)
		if capMark, ok := runtimeTraceProjCapabilityCaliberMark(node.SupplyFoldCapabilitySource); ok {
			componentMarks = append(componentMarks, capMark)
		}
		components := []runtimeTraceProjAttributionComponent{{
			Word: "running", RawMS: runtimeTraceProjSupplyFoldRunningMS(node), InMS: effective,
			CaliberShort: short, CaliberFull: full, Marks: componentMarks,
		}}
		out.SubRows = runtimeTraceProjAttributionSubRows(components, zh)
		out.Breakdown = effectiveWord + " " + runtimeTraceProjAttributionEquation(effective, components)
		row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
		for _, m := range componentMarks {
			row.marks.mark(m)
		}
		out.ConsumedEffective = true
		handled = true
	}
	if !handled && eligible && runtimeTraceProjCauseEventFoldRow(row) {
		// §24.2 event-class form: ×N moves onto 行1, the (a–b,共N次) range and
		// the 单次最大 caliber ride 行3. Identity: V == 单次最大 (typed check).
		if zh {
			out.Breakdown = fmt.Sprintf("%s %s = 单次最大(%.3f–%.3fms,共%d次)",
				effectiveWord, runtimeTraceProjFmtMS(effective), node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
		} else {
			out.Breakdown = fmt.Sprintf("%s %s = single max (%.3f–%.3fms, of %d)",
				effectiveWord, runtimeTraceProjFmtMS(effective), node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
		}
		row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
		row.marks.mark(runtimeTraceProjMarkCaliberSingleMax)
		out.NameXNSuffix = fmt.Sprintf(" ×%d", node.MergedCount)
		out.ConsumedMergedTag = true
		out.ConsumedEffective = true
		handled = true
	}
	if !handled && eligible && runtimeTraceProjRound3Equal(effective, impact) {
		// §24.2 degenerate form: single account, 计入==原始 — 行3 folds into
		// 行2's tail as the two-line form.
		full := "全额"
		if !zh {
			full = "in full"
		}
		identity = append(identity, fmt.Sprintf("%s %s(%s)", effectiveWord, runtimeTraceProjFmtMS(effective), full))
		row.marks.mark(runtimeTraceProjMarkCaliberFull)
		out.ConsumedEffective = true
	}
	if len(identity) == 0 {
		return runtimeTraceProjCauseStructured{}, false
	}
	out.IdentityRow = strings.Join(identity, sep)
	if node.RunnableBelowRTPreempted {
		// SYM-2 §24.17 R2 (2026-07-08): the 行2 tail discloses that the
		// target's own runnable wait ran below RT and was displaced by an
		// RT-class competitor — precise typed flag minted ONLY by the engine's
		// self-runnable stamp (ohos_cfs target + overlapping ohos_rt
		// competitor); the display never re-derives it.
		if zh {
			out.IdentityRow += "(优先级低于RT)"
		} else {
			out.IdentityRow += " (priority below RT)"
		}
	}
	row.marks.mark(runtimeTraceProjMarkCauseIdentityRow)
	if out.ConsumedEffective {
		// PTV8-RCR-B (UXA 域A #31): every lane that prints the 有效归因 word
		// (行2 degenerate tail / 行3 breakdown) teaches it via its legend seat.
		row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
	}
	return out, true
}

// runtimeTraceProjCauseEvidenceRef composes 行1's evidence bracket with the
// folded rank-lane twin's E# merged in (§24.2 [E#(+E#)] — the rank row's E#
// rises into 行1; its rank/confidence ride 行2). "" when the row carries no
// evidence tag of its own.
func runtimeTraceProjCauseEvidenceRef(row runtimeTraceProjTreeRow) string {
	if row.EvidenceTag == "" {
		return ""
	}
	ref := row.EvidenceTag
	for _, peer := range row.RankFoldPeers {
		if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
			ref += "+" + tag
		}
	}
	return "[" + ref + "]"
}

// runtimeTraceProjInversionSupplyFoldDetailLine is the lossless-block home of
// an inversion node's supply-fold deficit (the retired Triple mechanism
// sentence's data half, §24 ②): the unified sub-row grammar with the
// 供给折算缺口 phrasing folded into the caliber parenthesis (§24.1 单一子行
// 文法), explicitly outside the effective attribution (independent caliber,
// never additive — 墙钟红线). "" when the fold never ran or found no deficit.
func runtimeTraceProjInversionSupplyFoldDetailLine(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceCausalProjectionInversionRow(node) || !node.SupplyFoldComputed || node.SupplyFoldDeficitMS <= 0 {
		return ""
	}
	raw := runtimeTraceProjSupplyFoldRunningMS(node)
	if raw <= 0 {
		return ""
	}
	// CAP (§26 C3): the caliber parenthesis carries the fold's typed
	// capability disclosure. 复核 F1: the basis word follows the actual
	// reference cluster.
	capSuffix := runtimeTraceProjCapabilityCaliberSuffix(node.SupplyFoldCapabilitySource, zh)
	refWord, _ := runtimeTraceProjFoldReferenceClusterWord(node.SupplyFoldReferenceClass, zh)
	if zh {
		return fmt.Sprintf("running 原始 %s → 供给折算缺口 %s(折算,按%s满频,下界%s;独立口径,不计入有效归因)",
			runtimeTraceProjFmtMS(raw), runtimeTraceProjFmtMS(node.SupplyFoldDeficitMS), refWord, capSuffix)
	}
	return fmt.Sprintf("running raw %s → supply-fold deficit %s (discounted at %s fmax, lower bound%s; independent caliber, not counted into attribution)",
		runtimeTraceProjFmtMS(raw), runtimeTraceProjFmtMS(node.SupplyFoldDeficitMS), refWord, capSuffix)
}
