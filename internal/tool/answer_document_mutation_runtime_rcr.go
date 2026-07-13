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

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjRootGlyph is the tree-root mark (§24.3 无亮色 hard rule +
// 复核 F3 裁定 2026-07-08: 🎯 → ⊚ U+229A, EAW=Neutral — the earlier ◎ U+25CE
// is East-Asian-Ambiguous and double-width on CJK terminals, escaping the
// single-cell rule). Consumed by the header composer, the legend entry and
// the single-cell width pin — one constant, three surfaces.
// EVOLUTION RECORD (v5 P0 备-2, 2026-07-11): the byte now lives in
// internal/tracefence.RootGlyph — the single source shared with the preview
// classifier's legacy ⊚ arm.
const runtimeTraceProjRootGlyph = tracefence.RootGlyph

// runtimeTraceProjOffChainDStateGlyph is the ◇/▒ D-state/IO row glyph (UXR-1
// §29.36②: ⛓ claims chain membership, so off-chain rows wear ⧗ instead).
// ⧗ U+29D7 is EAW-Neutral (1 cell in BOTH width contexts — the ⧖ U+29D6
// width class; NOT the ⛓ U+26D3 class, which is East-Asian-Ambiguous and
// measures 2 under EastAsianWidth). One constant, two surfaces: the icon
// resolver and the single-cell/EAW width pin (UXR-1 复核 P2-4).
// EVOLUTION RECORD (UXG-1 M1, 2026-07-12): the byte lives in
// internal/tracefence — the state-mark directory shared with the preview
// icon classifier.
const runtimeTraceProjOffChainDStateGlyph = tracefence.GlyphOffChainDState

// --- §24.3 impact-form table (glyph + category, ONE typed source) -------------

type runtimeTraceProjImpactForm int

const (
	runtimeTraceProjImpactFormNone runtimeTraceProjImpactForm = iota
	// ⚙ 运行占用·算力供给 (running / compute-supply family). The
	// ranking value is the producer's discounted supply deficit, never the raw
	// running wall clock; semantic work is classified before this state arm.
	runtimeTraceProjImpactFormRunning
	// ☾ 睡眠 (symptom, never a cause category).
	runtimeTraceProjImpactFormSleep
	// ⛓ IO阻塞族 (§24.3: io_latency 等 typed 事件归族,不再挂无意义 ◦).
	runtimeTraceProjImpactFormIOBlock
	// ⛓ D状态族 (SYM-2 §24.17 R2, 2026-07-08): D-state rows leave the IO
	// family's category word — same glyph (⛓ carries the D-state icon
	// semantics since PTV4), own 行2 word (D状态候选).
	runtimeTraceProjImpactFormDState
	// ⛓ IO等待候选 (DSTATE-REFINE arm b, CAL-1 件③ §29.47.2, 2026-07-12): a
	// D-family row REFINED to the typed io_wait token (the engine's
	// blocked_reason iowait proof) — the category word consumes the
	// refinement instead of the family 「D状态候选」 its dominant D state
	// would mint (96728 E14 同行三面三说法灭). Same ⛓ glyph family.
	runtimeTraceProjImpactFormIOWaitRefined
	// ⛓ D状态/IO候选 (件③ arm b 用户补正, witness 45261 E9, 2026-07-12): the
	// UNREFINED merged d_state_or_io_wait family row — 行1 speaks the merged
	// compound (D-state/iowait), so the category word is the mixed compound
	// too (三面一说); the refined pure-D form keeps 「D状态候选」. Tri-form
	// final: 全iowait→IO等待候选 / 细化纯D→D状态候选 / 混合或记录不全→本形.
	runtimeTraceProjImpactFormDStateIOMixed
	// ⧖ 调度压力 (runnable / scheduler-latency family; §7.4 demand-side word).
	runtimeTraceProjImpactFormRunnable
	// ⊗ 锁竞争·持锁 (typed BlockingKind rows).
	runtimeTraceProjImpactFormLock
	// ⇅ 优先级反转候选 (inversion candidates).
	runtimeTraceProjImpactFormInversion
	// ✦ 确定性优化 (semantic spans + deterministic compile/verify classes).
	runtimeTraceProjImpactFormDeterministicOpt
	// ↯ 中断活动族 (irq/ipi/workqueue/dma-fence).
	runtimeTraceProjImpactFormInterrupt
	// ◌ 数据盲区 (trace_gap / missing_wakeup — data gaps, not causes).
	runtimeTraceProjImpactFormBlindSpot
	// ⋈ binder 等待 (IPC wait-on-peer — 等待症状族, NOT block IO).
	// PTV8-RCR-C 复核收尾 (2026-07-08). EVOLUTION RECORD: binder_wait rode the
	// ⛓ IOBlock family since RCR-A — harmless while the family word had no
	// consumer, but the C7 FamilyWord lane would have called IPC "IO阻塞候选"
	// (typelabels 已定 binder等待). EVOLUTION RECORD (P2a rider 件3, §29.58.2
	// 裁定 2026-07-13): the interim ◦ 无形态兜底 borrow is RETIRED — the user
	// ruling extends the §24.3 closed set with the dedicated ⋈ U+22C8 glyph
	// (fourth split precedent; census §3 first choice), and the Mark splits
	// off IconNoDominant so a binder-only report stops lighting the ◦ 数据行
	// legend entry (F1 承诺面 falsity, 062916 witness).
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
		{Form: runtimeTraceProjImpactFormRunning, Glyph: tracefence.GlyphRunning,
			CategoryZH: "算力供给候选", CategoryEN: "compute-supply candidate",
			Mark: runtimeTraceProjMarkIconRunning},
		{Form: runtimeTraceProjImpactFormSleep, Glyph: tracefence.GlyphSleep,
			Mark: runtimeTraceProjMarkIconSleep},
		{Form: runtimeTraceProjImpactFormIOBlock, Glyph: tracefence.GlyphIOChain,
			CategoryZH: "IO阻塞候选", CategoryEN: "IO-blocking candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// SYM-2 §24.17 R2 (2026-07-08): the D-state family's OWN table row —
		// glyph/mark stay on the existing ⛓ D-state icon semantics (legend
		// entry unchanged), only the 行2 category word splits from IO阻塞候选.
		{Form: runtimeTraceProjImpactFormDState, Glyph: tracefence.GlyphIOChain,
			CategoryZH: "D状态候选", CategoryEN: "D-state candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// DSTATE-REFINE arm b (件③, 2026-07-12): the refined-to-iowait row's
		// own category word (glyph/mark stay on the ⛓ family).
		{Form: runtimeTraceProjImpactFormIOWaitRefined, Glyph: tracefence.GlyphIOChain,
			CategoryZH: "IO等待候选", CategoryEN: "IO-wait candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// 件③ arm b 用户补正 (2026-07-12): the mixed/unproven merged family's
		// compound category word (isomorphic with the 行1 merged word).
		{Form: runtimeTraceProjImpactFormDStateIOMixed, Glyph: tracefence.GlyphIOChain,
			CategoryZH: "D状态/IO候选", CategoryEN: "D-state/IO candidate",
			Mark: runtimeTraceProjMarkIconDState},
		// EVOLUTION RECORD (SYM-2 §24.17 R2, 2026-07-08): 就绪排队候选 →
		// 调度压力候选 — the runnable family's 行2 word joins the §7.4
		// ruling-locked demand-side vocabulary (调度压力/需求积压; the ⧖ legend
		// keeps its state semantics 就绪等待).
		{Form: runtimeTraceProjImpactFormRunnable, Glyph: tracefence.GlyphRunnable,
			CategoryZH: "调度压力候选", CategoryEN: "scheduling-pressure candidate",
			Mark: runtimeTraceProjMarkIconRunnable},
		{Form: runtimeTraceProjImpactFormLock, Glyph: tracefence.GlyphLock,
			CategoryZH: "锁竞争·持锁", CategoryEN: "lock contention · holder",
			SemanticsZH: "锁竞争(持锁/被锁阻塞)", SemanticsEN: "lock contention (holding / blocked on a lock)",
			Mark: runtimeTraceProjMarkIconLock, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormInversion, Glyph: tracefence.GlyphInversion,
			CategoryZH: "优先级反转候选", CategoryEN: "priority-inversion candidate",
			SemanticsZH: "优先级反转候选(低优先级依赖/持有资源可能阻塞高优先级)", SemanticsEN: "a priority-inversion candidate (a lower-priority dependency/holder may block a higher-priority waiter)",
			Mark: runtimeTraceProjMarkIconInversion, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormDeterministicOpt, Glyph: tracefence.GlyphOptimization,
			CategoryZH: "语义优化候选", CategoryEN: "semantic-optimization candidate",
			Mark: runtimeTraceProjMarkSemanticSpan},
		{Form: runtimeTraceProjImpactFormInterrupt, Glyph: tracefence.GlyphInterrupt,
			CategoryZH: "中断活动候选", CategoryEN: "interrupt-activity candidate",
			SemanticsZH: "中断活动(IRQ/IPI/工作队列等)", SemanticsEN: "interrupt activity (IRQ/IPI/workqueue …)",
			Mark: runtimeTraceProjMarkIconInterrupt, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormBlindSpot, Glyph: tracefence.GlyphBlindSpot,
			SemanticsZH: "数据缺失记号(窗内数据缺口,非成因)", SemanticsEN: "a data-gap marker (missing in-window data, not a cause)",
			Mark: runtimeTraceProjMarkIconBlindSpot, GeneratedLegend: true},
		// PTV8-RCR-C 复核收尾: binder IPC waits leave the ⛓ IO family — the
		// category word rides the typelabels binder等待 typed word (+候选 同族
		// 模式). P2a rider 件3 (§29.58.2 裁定, 2026-07-13): the ⋈ dedicated
		// glyph + own Mark + generated legend entry replace the ◦/IconNoDominant
		// borrow (F1 图例修真 — the ◦ entry's 「有形态词的行戴各自形态族记号」
		// promise becomes true).
		{Form: runtimeTraceProjImpactFormBinderWait, Glyph: tracefence.GlyphBinderWait,
			CategoryZH: "binder等待候选", CategoryEN: "binder-wait candidate",
			SemanticsZH: "binder IPC 等待(等待对端处理并返回;等待症状族,非 IO 阻塞)", SemanticsEN: "a binder IPC wait (waiting on the peer to process and reply; wait-symptom family, not block IO)",
			Mark: runtimeTraceProjMarkIconBinderWait, GeneratedLegend: true},
		{Form: runtimeTraceProjImpactFormFallback, Glyph: tracefence.GlyphNeutral,
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
	case "blocking_span":
		// UXR-1 §29.36.1 (140554 ◦配对 witness): a lock-contention span row
		// whose BlockingKind never resolved (对端未解析 form) still carries the
		// 阻塞等待 form word — it wears the lock family glyph via THIS typed
		// token, never ◦ beside a form word (icon says unknown, text says
		// blocking = the mixed signal the ruling closed).
		return runtimeTraceProjImpactFormLock
	case "irq_burst", "irq_activity", "ipi_activity", "workqueue_activity", "dma_fence_activity":
		return runtimeTraceProjImpactFormInterrupt
	case "trace_gap", "missing_wakeup":
		return runtimeTraceProjImpactFormBlindSpot
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause":
		// TEX §28.1 fifth semantic class (DISP-2 收尾, TEX 复核 F2, 2026-07-09):
		// texture_upload rides the deterministic-optimization family with the
		// exact same treatment as the four sibling classes — a missing arm here
		// dropped texture rows to FormNone (影响形态/族词车道缺席, §24.12 C7
		// 病理形). The registry semantic_class fold lane is the enumeration
		// source; TestDisp2ImpactFormSwitchCoversSemanticClassLane pins the
		// alignment so a sixth class cannot miss this switch silently.
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
		// 件③ tri-form (2026-07-12): the merged D/IO family word forks on
		// the refined-D proof here too (one fork rule, two consumers — the
		// FormForNode classifier carries the same arm).
		if form == runtimeTraceProjImpactFormDState {
			switch runtimeTraceCausalProjectionCanonicalNode(token) {
			case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
				if !node.DStateRefinedNonIO {
					form = runtimeTraceProjImpactFormDStateIOMixed
				}
			}
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
	// A typed semantic-work identity outranks its scheduler state for the row
	// form: VerifyClass/JIT/Shader is the actionable cause class; running is
	// how that work executed. The state remains explicitly disclosed beside
	// the semantic category by runtimeTraceProjCauseSemanticStateIdentity.
	for _, token := range []string{node.TypeToken, node.Object, node.SemanticClass} {
		if runtimeTraceProjImpactFormTokenFamily(runtimeTraceCausalProjectionCanonicalNode(token)) == runtimeTraceProjImpactFormDeterministicOpt {
			return runtimeTraceProjImpactFormDeterministicOpt
		}
	}
	if node.IsSleepState() {
		return runtimeTraceProjImpactFormSleep
	}
	// DSTATE-REFINE arm b (件③, witness 96728 E14): a row whose TYPE lane was
	// refined to io_wait while its dominant STATE stayed in the D family
	// consumes the refinement — 「IO等待候选」, never the D-state family word
	// the state arm below would mint (类别词消费细化态). Exact typed tokens —
	// the RAW TypeToken (TypeTokenStateClass deliberately blanks itself when
	// a StateKind is present, and this shape ALWAYS has the D StateKind).
	if strings.ToLower(strings.TrimSpace(node.TypeToken)) == "io_wait" {
		switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
		case "d_state", "d_sleep", "uninterruptible_sleep":
			return runtimeTraceProjImpactFormIOWaitRefined
		}
	}
	// 件③ arm b 用户补正 (witness 45261 E9): a MERGED d_state_or_io_wait
	// family row forks its category on the refined-D proof — proven pure D
	// keeps 「D状态候选」, mixed/coverage-incomplete speaks the compound
	// 「D状态/IO候选」 isomorphic with its 行1 merged word (三面一说). Exact
	// typed family tokens only; single-state D rows fall to the ladder below.
	for _, token := range []string{node.TypeToken, node.Object} {
		switch runtimeTraceCausalProjectionCanonicalNode(token) {
		case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
			if node.DStateRefinedNonIO {
				return runtimeTraceProjImpactFormDState
			}
			return runtimeTraceProjImpactFormDStateIOMixed
		}
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
	if !row.HasData || row.Node.OnChainOverflowFold || row.Node.IsContextOnlyRow() {
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
		// form table's category word on 行2 (语义优化候选) — the semantic
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

// --- UXR-1 §29.36.2/§29.36.3 ordinal channels (user rulings 2026-07-11) ------

// runtimeTraceProjOrdinalChannel values — the display mirror of the engine's
// rootCauseOrdinalChannel closed set. Glyph lane, stanza membership, chip
// word and detail seat label all fork on THIS one classifier (三面同一来源).
const (
	runtimeTraceProjOrdinalChannelChain      = "chain"      // 通道1 根因排序#N
	runtimeTraceProjOrdinalChannelAdjacent   = "adjacent"   // 通道2 邻近影响#N
	runtimeTraceProjOrdinalChannelBackground = "background" // 通道3 无序数
)

// runtimeTraceProjNodeOrdinalChannel resolves a node's ordinal channel from
// the typed chain-relevance single source (node.ChainRelevance is already the
// causality-resolved value from the ONE wire parser). Empty relevance stays
// on the chain channel — chainless boards carry no relevance and remain ONE
// caliber-uniform ordinal space (mirrors the engine's fail-open arm).
func runtimeTraceProjNodeOrdinalChannel(node types.TraceCausalProjectionNode) string {
	switch strings.TrimSpace(node.ChainRelevance) {
	case "adjacent":
		return runtimeTraceProjOrdinalChannelAdjacent
	case "background":
		return runtimeTraceProjOrdinalChannelBackground
	default:
		return runtimeTraceProjOrdinalChannelChain
	}
}

// runtimeTraceProjRowOrdinalChannel is the display-face channel authority for
// a RENDERED row: a stanza row's channel IS its stanza (the ◇/▒ Kind is the
// relevance-derived placement, so chip word and stanza can never fork even on
// stale persisted forms whose bucket and relevance disagree); every other row
// resolves through the node's typed relevance.
func runtimeTraceProjRowOrdinalChannel(row runtimeTraceProjTreeRow) string {
	switch row.Kind {
	case runtimeTraceProjTreeRowAdjacent:
		return runtimeTraceProjOrdinalChannelAdjacent
	case runtimeTraceProjTreeRowBackground:
		return runtimeTraceProjOrdinalChannelBackground
	}
	return runtimeTraceProjNodeOrdinalChannel(row.Node)
}

// runtimeTraceProjSeatChannelWord is THE single channel-word source (UXR-1
// 复核 P3-⑦, 2026-07-11): both ordinal-seat display faces — the fence chip
// (runtimeTraceProjSeatChipWord) and the detail table's seat-line label —
// consume this ONE constructor, so the channel wording can never fork between
// the two surfaces (the M3 mutation now bites both). ok=false on the
// background channel (通道3 无序数).
//
// EVOLUTION RECORD (UXG-1 M1, 2026-07-12): the phrase BYTES moved to
// internal/tracefence — the HTML chip classifier traceProjectionRankToken
// (internal/preview/markdown.go) reads the same constants, retiring the hand
// mirror UXG-0 D2 flagged as "the last one allowed" (根因排序#N / 邻近影响#N
// zh, root-cause rank #N / adjacent-impact #N en).
func runtimeTraceProjSeatChannelWord(channel string, zh bool) (string, bool) {
	switch channel {
	case runtimeTraceProjOrdinalChannelAdjacent:
		if zh {
			return tracefence.SeatChannelAdjacentZH, true
		}
		return tracefence.SeatChannelAdjacentEN, true
	case runtimeTraceProjOrdinalChannelBackground:
		return "", false
	default:
		if zh {
			return tracefence.SeatChannelChainZH, true
		}
		return tracefence.SeatChannelChainEN, true
	}
}

// runtimeTraceProjSeatChipWord renders the channel-worded ordinal chip
// (§29.36.2 配套不变量: 序数 chip 词面必带通道名,禁裸 #N). ok=false on the
// background channel — 通道3 无序数: even a stale persisted Rank>0 on a
// background row never prints a seat chip (the 4165 根因排序#1-in-▒
// same-page contradiction form is structurally unreachable) — and on a
// SeatOrdinalStale row (UXR-1 复核 P2-3): an ordinal beyond its own channel's
// rendered population is a stale-artifact replay and fail-closes symmetrically
// instead of being re-worded as a fresh channel ordinal.
func runtimeTraceProjSeatChipWord(row runtimeTraceProjTreeRow, rank int, zh bool) (string, bool) {
	if rank <= 0 || row.SeatOrdinalStale {
		return "", false
	}
	word, ok := runtimeTraceProjSeatChannelWord(runtimeTraceProjRowOrdinalChannel(row), zh)
	if !ok {
		return "", false
	}
	if zh {
		return fmt.Sprintf("%s#%d", word, rank), true
	}
	return fmt.Sprintf("%s #%d", word, rank), true
}

// runtimeTraceProjMentionFloorWord renders the channel-4 mention-obligation
// word (UXR-1 §29.36.3): "" when the row is not a stamped mention-floor seat.
// N = the rendered chain board size; a boardless report drops the 前N tail
// (absence never invents a board).
func runtimeTraceProjMentionFloorWord(row runtimeTraceProjTreeRow, zh bool) string {
	if !row.MentionFloorOnChain {
		return ""
	}
	// UXG-1 M1 (2026-07-12): the action-word head composes from the
	// tracefence single source (byte-identical) — the preview action-token
	// classifier emphasizes exactly this head.
	if row.MentionFloorTopN > 0 {
		if zh {
			return fmt.Sprintf("%s·未入%s前%d", tracefence.ActionWordZH, tracefence.SeatChannelChainZH, row.MentionFloorTopN)
		}
		return fmt.Sprintf("%s · below the top-%d root-cause board", tracefence.ActionWordEN, row.MentionFloorTopN)
	}
	if zh {
		return tracefence.ActionWordZH + "·未入" + tracefence.SeatChannelChainZH
	}
	return tracefence.ActionWordEN + " · not on the root-cause board"
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
		// DISP-3 P2-① (§29.8 "拆解行'原始'分量取行值非引擎 raw" — G7 词值同源的
		// 拆解行漏面, real_trace_campaign_20260705.md, 2026-07-09). EVOLUTION
		// RECORD: the display-impact arm used to OUTRANK the engine fold raw —
		// on a running-dominant row the 拆解子行 printed the row's whole-window
		// display value (runnable included) as "running 原始", while the 供给
		// 折算 line three rows below printed the engine's fold raw for the SAME
		// component (cmp_792 E8 detail block: 拆解 "running 原始 1.392ms" vs
		// 供给折算 "running 原始 2.681ms" — one block, two contradicting raws).
		// The engine-published fold raw (the very number the 供给折算 face
		// consumes — runtimeTraceProjSupplyFoldRunningMS) now leads; the
		// display-impact arm survives only as the no-fold fallback, so rows
		// whose two channels agree (huadong_792 E15: 5.943 both) stay
		// byte-identical.
		raw := 0.0
		if node.SupplyFoldComputed {
			raw = runtimeTraceProjSupplyFoldRunningMS(node)
		}
		if raw <= 0 && (strings.TrimSpace(strings.ToLower(node.StateKind)) == "running" ||
			runtimeTraceCausalProjectionTypeTokenStateClass(node) == "running") {
			raw = runtimeTraceProjNodeDisplayImpact(node)
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
		full += runtimeTraceProjCapabilityCaliberSuffixTopo(node.GatedCapabilitySource, node.GatedTopologySource, zh)
		if capMark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(node.GatedCapabilitySource, node.GatedTopologySource); ok {
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
// inversion cause node (§24 用户裁定① / task-verbatim E7 case).
//
// EVOLUTION RECORD (GAP-B G7 词值同源, §27.3 real_trace_campaign_20260705.md,
// 2026-07-09): the word position previously spoke the GATED ATTRIBUTION
// composition (runnable+running / the live component) while 行1's VALUE is the
// window-projection lane (ImpactMS — the engine's dominant_state projection,
// pinned by TestRCRIdentityRow1KeepsWindowProjection). The two sourced
// DIFFERENT typed lanes, so the huadong_79 E17 row read "runnable 4.115ms"
// over a running projection and E16 read "runnable+running 2.770ms" while the
// runnable 0.621 lived outside the printed value. The word now follows the
// VALUE: when 行1 shows the window lane (ImpactMS>0), the word is the typed
// dominant-state token that lane measures (StateKind, closed runnable/running
// set); the attribution composition stays fully disclosed on 行3's
// "有效归因 V = …" decomposition (既有). A row without a usable StateKind (or
// whose 行1 value fell back off the window lane) keeps the gated-composition
// word — the lossy absence never breaks the display (fail-open to legacy).
func runtimeTraceProjInversionStateCompositionWord(node types.TraceCausalProjectionNode) string {
	if !runtimeTraceCausalProjectionInversionRow(node) {
		return ""
	}
	if node.GatedRunnableMS <= 0 && node.GatedRunningDeficitMS <= 0 {
		// No gated decomposition → no composition word (the caller keeps the
		// legacy concise category label; G7 repairs only the decomposed rows).
		return ""
	}
	if node.ImpactMS > 0 {
		// 行1 value == window lane (the display-impact fallback chain's first
		// arm) — the word is that lane's own dominant state. Only the two
		// inversion component states own a word seat here; every other
		// dominant state falls through to the gated composition below
		// (fall-through declared in the StateKind pin ledger).
		switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
		case "running":
			return "running"
		case "runnable":
			return "runnable"
		}
	}
	switch {
	case node.GatedRunnableMS > 0 && node.GatedRunningDeficitMS > 0:
		return "runnable+running"
	case node.GatedRunnableMS > 0:
		return "runnable"
	}
	return "running"
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
	// SELF-SEM (§29.61.1 user ruling, RANK-U Stage 1, 2026-07-13): the Row2
	// qualifier slot wears 「自身·确定性优化」 on the typed self basis — ONE
	// field (node.OnChainBasis, minted engine-side), never a
	// subject∧class∧relevance recomposition. zh-en 同词纪律: the en form is
	// the same compound word.
	if strings.TrimSpace(node.OnChainBasis) == "self_deterministic_span" {
		if zh {
			identity = append(identity, "自身·确定性优化")
		} else {
			identity = append(identity, "self·deterministic-optimization")
		}
		row.marks.mark(runtimeTraceProjMarkSelfDeterministicBasis)
	}
	if state := runtimeTraceProjCauseSemanticStateIdentity(node, zh); state != "" {
		identity = append(identity, state)
	}
	rank, confidence := runtimeTraceProjCauseRankConfidence(row)
	// UXR-1 (§29.36.2, 2026-07-11): the seat chip is CHANNEL-worded — 根因排序#N
	// on the chain channel, 邻近影响#N on the ◇ adjacent channel, and NO chip on
	// the ▒ background channel (通道3 无序数; a stale persisted Rank never
	// resurrects the 4165 根因排序#1-in-▒ contradiction). 禁裸 #N.
	//
	// EVOLUTION RECORD (§29.36.2 supersedes RCM-2 D2/§23.2 E1b chip half): the
	// 背景榜位#N chip is RETIRED — the background board is unordered for the
	// reader (口径混杂,序数不可比). Node.BackgroundRank itself has NO
	// display-face consumer (§29.40 W-2 exemption): the §23.1 mention
	// obligation is soft guidance on the LLM face (the background_rank= wire
	// note emitted by internal/tool/trace_query.go), never a chip and never a
	// display gate on this field.
	if chip, ok := runtimeTraceProjSeatChipWord(row, rank, zh); ok {
		identity = append(identity, chip)
		// PTV8-RCR-C (§24.13 裁定二后半): the multi-board window tag binds to
		// the seat ordinal (根因排序#1·窗X — stamped at model build only when
		// ≥2 rank boards render; the single-board form carries none).
		if windowChip := strings.TrimSpace(row.RankWindowChip); windowChip != "" {
			identity = append(identity, windowChip)
			row.marks.mark(runtimeTraceProjMarkRankSeatWindow)
		}
	} else if word := runtimeTraceProjMentionFloorWord(row, zh); word != "" {
		// UXR-1 §29.36.3 (通道4): the seat-less on-chain semantic row names
		// its mention obligation where a seat chip would sit.
		identity = append(identity, word)
		row.marks.mark(runtimeTraceProjMarkSemanticMentionFloor)
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
	// DSTATE-REFINE caller 等待对象族 (件③, witness CompThread 12/12 iowait=0
	// dma_fence_default_wait): the unanimous blocked_reason semantic caller
	// discloses on 行2 — engine-minted symbol only (absence never guesses).
	if caller := strings.TrimSpace(node.BlockedReasonCaller); caller != "" {
		if zh {
			identity = append(identity, "等待对象 "+caller)
		} else {
			identity = append(identity, "wait object "+caller)
		}
	} else if node.BlockedReasonWindowCount > 0 {
		// CR-3 件② P10 (冷读案7 GPU-fence witness, 2026-07-12): the row
		// consumed no marker, yet the window HOLDS sched_blocked_reason
		// records for this thread — disclose the unconsumed residual so the
		// mechanism marker is never silently ignored next to an 未解析 word.
		identity = append(identity, runtimeTraceProjBlockedReasonResidualWord(node, zh))
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
	semIntersection, semDual := runtimeTraceProjSemanticChainDualCaliber(node)
	if runtimeTraceProjFamilyRow(node) || semDual {
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
		if semDual {
			intersection := semIntersection
			// 审计 #62 ① (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10):
			// the on-chain semantic dual-caliber form — the participation is
			// the exact member∩chain intersection while 行1 keeps the complete
			// window-projection union (§24.10 lossless caliber), so the 行3
			// 「合计(共N段,同线程)」 word would claim the union beside an
			// intersection value. 行3 speaks 链上计入(共N段,同线程) with the
			// union disclosed alongside; identity gate = the published value
			// equals the typed intersection at print precision (an engine
			// effective disagreeing with the typed intersection fails open —
			// 拒渲绝不造数, legacy effective tag lane keeps the value).
			if v > 0 && runtimeTraceProjRound3Equal(v, intersection) && !node.PeriodicSource {
				out.Breakdown = fmt.Sprintf("%s %s = %s%s", effectiveWord,
					runtimeTraceProjFmtMS(v),
					runtimeTraceProjSemanticChainIntersectionWord(node, zh),
					runtimeTraceProjSemanticChainUnionDisclosure(node, zh, false))
				out.SubRows = runtimeTraceProjFamilyRosterSubRows(node, zh)
				row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
				row.marks.mark(runtimeTraceProjMarkFamilyChainIntersection)
				out.ConsumedEffective = true
			}
			handled = true
		} else {
			balanced := effective <= 0 || impact <= 0 || runtimeTraceProjRound3Equal(effective, impact)
			if wordOK && v > 0 && balanced && !node.PeriodicSource {
				out.Breakdown = fmt.Sprintf("%s %s = %s%s", effectiveWord,
					runtimeTraceProjFmtMS(v), word, runtimeTraceProjFamilySumDisclosure(node, zh))
				out.SubRows = runtimeTraceProjFamilyRosterSubRows(node, zh)
				row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
				runtimeTraceProjMarkFamilyCaliber(row.marks, caliberMark)
				out.ConsumedEffective = true
			}
			handled = true
		}
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
			if capMark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(node.GatedCapabilitySource, node.GatedTopologySource); ok && node.GatedRunningDeficitMS > 0 {
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
		// UXR-1 §29.36.4 ② (核类词诚实门): 簇结构不可判 (typed freq_only
		// source) forbids the core-class word on the sub-row caliber too —
		// 「折算,按大核满频」 beside 「簇结构不可判…」 was the claim-vs-caveat
		// contradiction form; the class-less 折算,按满频 degrades honestly and
		// its class-word legend seat stays off with the word.
		refWord, _ := runtimeTraceProjFoldReferenceClusterWord(node.SupplyFoldReferenceClass, zh)
		var componentMarks []runtimeTraceProjMark
		if node.SupplyFoldCapabilitySource == runtimeTraceCapabilitySourceFreqOnly {
			refWord = ""
		} else {
			componentMarks = append(componentMarks, runtimeTraceProjFoldReferenceMark(node.SupplyFoldReferenceClass))
		}
		short := "折算,按" + refWord + "满频"
		if !zh {
			short = "discounted, at " + refWord + " fmax"
			if refWord == "" {
				short = "discounted, at fmax"
			}
		}
		full := short
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
		full += runtimeTraceProjCapabilityCaliberSuffixTopo(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource, zh)
		if capMark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource); ok {
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
			out.Breakdown = fmt.Sprintf("%s %s = 单次最大(%.3f~%.3fms,共%d次)",
				effectiveWord, runtimeTraceProjFmtMS(effective), node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
		} else {
			out.Breakdown = fmt.Sprintf("%s %s = single max (%.3f~%.3fms, of %d)",
				effectiveWord, runtimeTraceProjFmtMS(effective), node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
		}
		row.marks.mark(runtimeTraceProjMarkEffectiveBreakdown)
		row.marks.mark(runtimeTraceProjMarkCaliberSingleMax)
		out.NameXNSuffix = runtimeTraceProjMergeCountChip(node.MergedCount, zh)
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

// runtimeTraceProjCauseSemanticStateIdentity keeps a ranked semantic-work
// row's execution state next to its ranking identity. The customer witness
// rendered "VerifyClass" with a CPU-execution category while the typed
// dominant_state=running appeared only on a later detail line, making the
// causal caliber easy to miss. Exact semantic-class tokens + producer-minted
// StateKind only; missing state never guesses and non-semantic rows keep their
// established compact grammar.
func runtimeTraceProjCauseSemanticStateIdentity(node types.TraceCausalProjectionNode, zh bool) string {
	state := runtimeTraceProjStateKindLabel(node, zh)
	if state == "" {
		return ""
	}
	for _, token := range []string{node.TypeToken, node.Object, node.SemanticClass} {
		if runtimeTraceProjImpactFormTokenFamily(runtimeTraceCausalProjectionCanonicalNode(token)) != runtimeTraceProjImpactFormDeterministicOpt {
			continue
		}
		if zh {
			return "状态" + state
		}
		return "state " + state
	}
	return ""
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
	for _, peer := range row.SelfSymptomFoldPeers {
		if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
			ref += "+" + tag
		}
	}
	// CR-2 组② P5 member arm (legacy lane): the folded raw-state mirror's E# joins the
	// bracket the same way (the mirror observation stays reachable).
	for _, peer := range row.SameSegMirrorPeers {
		if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
			ref += "+" + tag
		}
	}
	// WO-D2/D4 (SMR-1 批, 2026-07-12): the folded flat aggregate copy's E#
	// joins the bracket the same way (零静默消失 — the folded observation
	// stays reachable through the index).
	for _, peer := range row.BranchTwinFoldPeers {
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
	// reference cluster. CAP-2: the wording upgrades on the topology token.
	// UXR-1 §29.36.4 ② (核类词诚实门): 簇结构不可判 forbids the core-class
	// word on this face too — the class-less 按满频 form degrades honestly.
	capSuffix := runtimeTraceProjCapabilityCaliberSuffixTopo(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource, zh)
	refWord, _ := runtimeTraceProjFoldReferenceClusterWord(node.SupplyFoldReferenceClass, zh)
	refWordEN := refWord + " "
	if node.SupplyFoldCapabilitySource == runtimeTraceCapabilitySourceFreqOnly {
		refWord = ""
		refWordEN = ""
	}
	if zh {
		return fmt.Sprintf("running 原始 %s → 供给折算缺口 %s(折算,按%s满频,下界%s;独立口径,不计入有效归因)",
			runtimeTraceProjFmtMS(raw), runtimeTraceProjFmtMS(node.SupplyFoldDeficitMS), refWord, capSuffix)
	}
	return fmt.Sprintf("running raw %s → supply-fold deficit %s (discounted at %sfmax, lower bound%s; independent caliber, not counted into attribution)",
		runtimeTraceProjFmtMS(raw), runtimeTraceProjFmtMS(node.SupplyFoldDeficitMS), refWordEN, capSuffix)
}

// runtimeTraceProjBlockedReasonResidualWord renders the CR-3 件② P10
// unconsumed-marker disclosure (2026-07-12, 冷读案7 GPU-fence witness):
// the row consumed no blocked_reason caller, yet the window holds markers
// for its thread — 「窗内存在 N 条 blocked_reason 记录(caller=…,未核销)」.
// Engine-minted count + symbols only; absence never guesses.
func runtimeTraceProjBlockedReasonResidualWord(node types.TraceCausalProjectionNode, zh bool) string {
	caller := strings.TrimSpace(node.BlockedReasonWindowCaller)
	if zh {
		if caller != "" {
			return fmt.Sprintf("窗内存在 %d 条 blocked_reason 记录(caller=%s,未核销)", node.BlockedReasonWindowCount, caller)
		}
		return fmt.Sprintf("窗内存在 %d 条 blocked_reason 记录(未核销)", node.BlockedReasonWindowCount)
	}
	if caller != "" {
		return fmt.Sprintf("window holds %d blocked_reason record(s) (caller=%s, unconsumed)", node.BlockedReasonWindowCount, caller)
	}
	return fmt.Sprintf("window holds %d blocked_reason record(s) (unconsumed)", node.BlockedReasonWindowCount)
}

// runtimeTraceProjDetailProcessCell renders the CR-3 件③ P11 process
// attribution value (「tgid=G comm=P」; comm omitted when the engine could
// not resolve the owning process comm — the thread's own comm never
// substitutes). Same value on both language faces (identity tokens).
func runtimeTraceProjDetailProcessCell(node types.TraceCausalProjectionNode) string {
	if node.ProcessTGID <= 0 {
		return ""
	}
	if comm := strings.TrimSpace(node.ProcessComm); comm != "" {
		return fmt.Sprintf("tgid=%d comm=%s", node.ProcessTGID, comm)
	}
	return fmt.Sprintf("tgid=%d", node.ProcessTGID)
}
