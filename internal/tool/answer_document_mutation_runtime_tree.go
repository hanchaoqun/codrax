package tool

// Presentation v3 (docs/design/trace_projection_presentation_v3_20260702.md):
// the Trace Causal Projection section renders as a 3-block cluster —
//   1. a lead block whose Text carries a fact-only conclusion line, the
//      requested-window anchor + coverage subtraction, one tree-reading
//      sentence, and the MAIN monospace tree inside a ```text fence
//      (byte-identical across HTML / markdown / terminal — zero mermaid);
//   2. one lossless detail table (the full duration quad, per-row relation,
//      impact shape, evidence + confidence);
//   3. a file-grouped evidence index.
// The tree is anchored at the user-focused thread (🎯 root = last wakeup-path
// node); four edge kinds only (下钻 / 唤醒 / 语义 / 成因); bars are scaled to
// the requested window when the precise anchor exists and deterministically
// fall back to the batch max otherwise (never a fabricated window).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

const (
	runtimeTraceProjTreeRowChain      = "chain"
	runtimeTraceProjTreeRowCause      = "cause"
	runtimeTraceProjTreeRowSemantic   = "semantic"
	runtimeTraceProjTreeRowDepthless  = "depthless"
	runtimeTraceProjTreeRowSelf       = "self"
	runtimeTraceProjTreeRowAdjacent   = "adjacent"
	runtimeTraceProjTreeRowBackground = "background"
	runtimeTraceProjTreeRowOmitted    = "omitted"

	runtimeTraceProjTreeEdgeDrill    = "drill"
	runtimeTraceProjTreeEdgeWake     = "wake"
	runtimeTraceProjTreeEdgeSemantic = "semantic"
	runtimeTraceProjTreeEdgeCause    = "cause"
	// runtimeTraceProjTreeEdgeOwn (F2, adversarial re-review 2026-07-04) marks
	// the depthless own-process IO caliber lane: 目标自身/同进程的口径行 — the
	// row re-describes the target's own wall clock through an IO caliber, so
	// drawing the depthless default ├─唤醒─ asserted a wake relation the data
	// never carried (three-surface contradiction with the relation column and
	// the NEW-1 legend). Stamped ONCE at model build via
	// runtimeTraceProjOwnProcessIONode; fence edge, relation cell and legend
	// all read this typed edge.
	runtimeTraceProjTreeEdgeOwn = "own"

	// runtimeTraceProjTreeLabelWidth is the display-cell budget of the tree
	// label column (prefix + edge + icon + name); bars/ms/tags align after it.
	// NEW-10 (§7.6): tightened 56 → 44 as part of the 100-cell row budget —
	// with the bar+ms+% cells (~27) a 56-cell label left the tag lane no room
	// for even the floored Keep stubs. Names beyond the budget display-truncate
	// exactly as before (B1 name-scoped truncation, 20-cell floor unchanged);
	// the detail table keeps full names.
	runtimeTraceProjTreeLabelWidth = 44
	// runtimeTraceProjTreeNameMinWidth is the readability floor for the NAME
	// portion of a tree row label (§7.30.2 C4b B1): a deep prefix/edge/icon may
	// never squeeze the name below this many display cells — the name budget is
	// LabelWidth minus the actual fixed-part width, floored here; deep rows
	// widen the shared label column instead of eating the name.
	runtimeTraceProjTreeNameMinWidth = 20
	// runtimeTraceProjTreeRowMaxWidth caps a rendered tree/stanza row's total
	// display width including the tag segment.
	//
	// NEW-10 (§7.6 对比场景客户回访 2026-07-04, 客户点名): tightened 120 → 100.
	// Markdown/HTML viewers wrap long lines (pre-wrap) and web monospace fonts
	// render CJK≈1.6-1.8× / emoji at unstable widths — the 100-cell hard cap
	// buys single-line integrity on those surfaces (单行完整性优先).
	//
	// PTV4 T1 (#65, 2026-07-05): the former B4 drop lane + keep-truncation
	// lane + ↳ continuation lane are all RETIRED. A row whose full tag set
	// fits stays one line; otherwise the main line keeps only the essentials
	// (label + bar + ms + % + ⚠/⊘/[E#] Keep marks) and every other tag
	// demotes WHOLE to "· " subordinate detail lines — nothing is elided; the
	// (a) key-metric table + (b) per-node blocks stay the lossless surfaces.
	runtimeTraceProjTreeRowMaxWidth = 100
	// runtimeTraceProjTreeKeepTagMinWidth once floored the NEW-10
	// keep-truncation lane (retired by PTV4 T1). It survives ONLY as an input
	// of the derived runtimeTraceProjTreeLabelColumnMax reserve below — the
	// shared label-column cap is pinned at 50 (TestTraceProjectionNew10Width-
	// ConstantsPinned), so this constant is part of that pinned formula, not a
	// live truncation floor.
	runtimeTraceProjTreeKeepTagMinWidth = 9
	// runtimeTraceProjTreeTrunkMaxNodes bounds a long trunk display: deeper
	// middles compress into one omitted marker row (counts + cycle note kept).
	runtimeTraceProjTreeTrunkMaxNodes = 8
	runtimeTraceProjTreeBarWidth      = 10
	// runtimeTraceProjTreeLabelColumnMax caps the SHARED label column (F-3,
	// §7.7 回访聚焦复核 2026-07-04): pre-cap, one deep row (fixed prefix grows 4
	// cells per level + the 20-cell name floor) or one over-wide 🎯 header
	// lifted the column for EVERY row and pushed the whole fence past the
	// NEW-10 row cap. The cap reserves a minimal metric+mark area a data row
	// still needs inside the 100-cell budget — derived from the pinned
	// constants and the renderer's own cell formats, never a free-standing
	// number (PTV4 kept the formula and its pinned value 50; the "Keep-tag
	// floor / elision" terms are historical inputs of that reserve, the drop
	// lane itself is retired):
	//   " "(1) + bar(10) + " %9.3fms"(12) + " NN%"(5, " %3.0f%%") + "  "(2)
	//   + reserve(9) + " · "(3) + "…"(1) + " · "(3) + minimal "[E#]"(4) = 50
	// Rows whose fixed part + name floor exceed the cap truncate the NAME
	// further (B1 semantics — the detail table keeps full names); the 🎯 header
	// itself never truncates (it renders unpadded past the column and the
	// NEW-10 header wrap moves the scale note to its own line).
	runtimeTraceProjTreeLabelColumnMax = runtimeTraceProjTreeRowMaxWidth -
		(1 + runtimeTraceProjTreeBarWidth + 12 + 5 + 2 + runtimeTraceProjTreeKeepTagMinWidth + 3 + 1 + 3 + 4)
)

type runtimeTraceProjTreeRow struct {
	Node        types.TraceCausalProjectionNode
	Kind        string
	Edge        string
	Depth       int
	Indent      int
	Last        bool
	Ancestors   []bool // per ancestor level: more siblings follow (renders │)
	Parent      string // display name of the parent node (typed 影响点)
	HasData     bool   // false = bare path transit node without its own row
	Omitted     int
	CyclePeriod int
	CycleCount  int
	// OmittedHead / OmittedTail carry the first/last two node names of the
	// folded trunk middle (PTV4 T8, pure display upgrade — the names were
	// always in the typed wakeup path). Rendered mid-truncated (T2).
	OmittedHead []string
	OmittedTail []string
	// Badge is the PTV4 T6 TOP-N root-cause badge rank (1..3; 0 = none):
	// among the rendered rows whose node carries the engine's typed
	// root_cause_rank (Node.Rank > 0) and that sit on the chain lanes, the
	// top three by effective attribution. Assigned ONCE at model build from
	// typed fields only — never a prose judgment, never an LLM signal. The
	// badge is an independent token, NOT a state glyph (the one-state-glyph
	// invariant counts state icons only).
	Badge       int
	EvidenceTag string
	// RecursOnChain marks a trunk row whose canonical subject already appeared
	// earlier on the rendered chain (target root first, then depth 1..K) — the
	// small-cycle shape (A→B→A) the ≥6-node cycle detector cannot see (H11,
	// customer audit 2026-07-03). Display-only annotation; the chain itself is
	// never truncated because of it.
	//
	// V4 (customer revisit 2026-07-03): the former DedupFold row flag moved to
	// the typed Node.DuplicatePublications field — one home shared by the
	// aggregation layer's pre-R2 dedup pass and this layer's H6 fold. The ×N
	// label forks on that typed count, never on guessing from the numbers.
	RecursOnChain bool
	// FlatChain marks rows of a flat-fallback render (no ≥2-node wakeup path —
	// model.Target empty): the tree header states the chain could not be traced
	// upstream, so the layer chip / 因果位置 cell must not claim "on-chain" for
	// these rows (CMP-7a). Display-only; the node's typed causality is
	// untouched.
	FlatChain bool
	// IOFoldPeers carries the same-subject same-segment IO caliber rows folded
	// into this primary row (NEW-3, §7.6 对比场景客户回访 2026-07-04): one
	// underlying IO burst published as several near-equal calibers
	// (io_burst_episode + io_wait over overlapping line spans) rendered as four
	// sibling rows. The peers render as one load-bearing caliber note on this
	// row — values + evidence ids all kept; the underlying observations and
	// projection buckets are untouched (display grouping only).
	IOFoldPeers []runtimeTraceProjIOFoldPeer
	// marks is the NEW-7 emission collector for this render pass. The fence
	// renderer stamps model.Marks onto its per-row COPIES right before calling
	// the row-render helpers, so every mark is recorded AT the emission site
	// (nil-safe: width-pass rows and test-constructed rows carry nil and record
	// nothing). Never set by the model builder.
	marks *runtimeTraceProjMarkSet
}

// runtimeTraceProjIOFoldPeer is one folded same-segment IO caliber: the raw
// typed token, its display impact and its registered evidence tag.
type runtimeTraceProjIOFoldPeer struct {
	Token       string
	ImpactMS    float64
	EvidenceTag string
}

type runtimeTraceProjTreeModel struct {
	Target     string
	TreeRows   []runtimeTraceProjTreeRow // trunk + attached (flattened, render order)
	SelfRows   []runtimeTraceProjTreeRow // target's own state rows (under root)
	Adjacent   []runtimeTraceProjTreeRow
	Background []runtimeTraceProjTreeRow
	WindowMS   float64 // >0 = window mode; 0 = fallback (BarMaxMS denominator)
	BarMaxMS   float64
	// TrunkLen is the resolved wakeup-path trunk length (0 = flat mode); it is
	// the same value the 裁定1 demotion gate ran against, so lead selection can
	// re-apply exactly that gate instead of a diverging copy.
	TrunkLen int
	// WakeupChainRecommendedNotRun mirrors the typed projection flag (§7.30
	// 裁定3): a chain_required=true state_drilldown recommendation existed but no
	// wakeup_chain-family observation ran, so the flat-fallback header can name
	// the actual coverage cause instead of the opaque "path unresolved".
	WakeupChainRecommendedNotRun bool
	// RootFocusAnchorOnly selects the fence root label lane (R2, customer audit
	// 2026-07-03 C4a): the typed analyzer entity comparison RAN and the 🎯 root
	// matched none of the user's entities, so the label must say ‹分析锚点线程›
	// instead of falsely claiming ‹用户关注线程›. False is the fail-open default:
	// when no typed entity context reached the renderer the legacy label stays.
	RootFocusAnchorOnly bool
	// RootFocusUserEntities lists the user's thread/pid-shaped entities for the
	// anchor-only explanation line (display-only roster; empty = no note line).
	// Shared by the 🎯 anchor-only note (R2) and the flat-fallback anchor note
	// (RN-13(a)) — the two lanes are mutually exclusive on model.Target.
	RootFocusUserEntities []string
	// FlatAnchorThread / FlatAnchorMismatch (RN-13(a), §7.9 runnable 主导场景审计
	// 2026-07-04): the flat-fallback isomorph of RootFocusAnchorOnly. In flat
	// mode there is no 🎯 root, so the analysis anchor is the RN-3 lead row's
	// subject (model.LeadKey — the same typed selection the conclusion line
	// consumes). Mismatch=true only when the typed entity comparison RAN
	// (thread/pid-shaped user entities exist) and the anchor matched none of
	// them; the header then explains the anchor and the next-step lane offers
	// the wakeup_chain recovery hint. False is the fail-open default: matched
	// anchor or no typed entities keeps the flat header byte-identical.
	FlatAnchorThread   string
	FlatAnchorMismatch bool
	// UserWindowStart/UserWindowEnd is the user's originally-requested window in
	// seconds, derived DISPLAY-ONLY from a timestamp-shaped analyzer entity pair
	// (R2 双窗关系行). Zero when absent/ambiguous — the relation line then never
	// renders; nothing else consumes these values.
	UserWindowStart float64
	UserWindowEnd   float64
	// LeadKey is the node key of the row the conclusion line actually consumes
	// (RN-3(b), §7.9 runnable 主导场景审计 2026-07-04) — computed once at model
	// build via runtimeTraceProjLeadSelect, "" when no lead exists. The detail
	// table's 因果位置·优先级 column reads it so the "主根因 · 主要关注" label
	// can only sit on the consumed node: an unconsumed primary-tier BACKGROUND
	// row (engine tier=primary/rank=1 直投 into the background bucket) demotes
	// to 背景 · 支撑参考(rank=N) instead of contradicting a conclusion that
	// says no on-chain primary was located. Display-only; typed causality on
	// the nodes is untouched.
	LeadKey string
	// Marks collects the tree marks actually emitted by THIS model's fence
	// render (NEW-7, §7.6 对比场景客户回访 2026-07-04): a typed set recorded at
	// each emission site — never re-derived by scanning rendered text. The
	// 树读法 legend renders exactly the catalog entries whose mark is present.
	// Callers must render the fence before the lead text (the cluster function
	// already does); a nil set (hand-built test models) renders no dynamic
	// legend entries.
	Marks *runtimeTraceProjMarkSet
}

// --- NEW-7 dynamic tree legend (§7.6 对比场景客户回访 2026-07-04) --------------
//
// The 树读法 legend was a STATIC list that drifted from the actual tree: marks
// the customer's render really contained (🎯 ⏳ ⚙ ⛓ ◦ ├─下钻─ ⚠跨窗 ↺) had no
// legend entry, while ⛔ and ├─成因─ were explained without appearing. The
// renderer now keeps a typed mark enum, records every emitted mark kind at the
// emission site, and the legend = two fixed head clauses + exactly the catalog
// entries whose mark was emitted (stable catalog order). The catalog below is
// the EXHAUSTIVE directory of every mark this renderer can emit; the
// runtimeTraceProjMarkCount sentinel pins catalog completeness structurally
// (TestTraceProjectionLegendCatalogCoversEveryMark): adding a mark constant
// without a catalog entry explodes the build's tests.

type runtimeTraceProjMark int

const (
	runtimeTraceProjMarkRootTarget           runtimeTraceProjMark = iota // 🎯 root header
	runtimeTraceProjMarkEdgeDrill                                        // ├─下钻─ edge
	runtimeTraceProjMarkEdgeWake                                         // ├─唤醒─ / └─唤醒─ edge
	runtimeTraceProjMarkEdgeCause                                        // ├─成因─ edge
	runtimeTraceProjMarkEdgeOwn                                          // ├─自身─ own-process caliber edge (F2)
	runtimeTraceProjMarkSemanticSpan                                     // ├─语义─ edge + ✦ icon (always paired)
	runtimeTraceProjMarkIconSleep                                        // ☾ state icon (PTV4 T5: was 💤)
	runtimeTraceProjMarkIconRunnable                                     // ⧖ state icon (PTV4 T5: was ⏳)
	runtimeTraceProjMarkIconRunning                                      // ⚙ state icon
	runtimeTraceProjMarkIconDState                                       // ⛓ state icon
	runtimeTraceProjMarkIconTransit                                      // ◦ 中转 transit icon (PTV4 T4: the two ◦ senses split)
	runtimeTraceProjMarkIconNoDominant                                   // ◦ 无主导态 stateless data-row icon (PTV4 T4)
	runtimeTraceProjMarkBadge                                            // ❶❷❸ TOP-N root-cause badge (PTV4 T6)
	runtimeTraceProjMarkStateLabel                                       // dominant-state / impact-shape tag
	runtimeTraceProjMarkUndrillable                                      // ⊘ missing-wakeup marker (PTV4 T5: was ⛔)
	runtimeTraceProjMarkCrossWindow                                      // ⚠实际Xms cross-window marker (PTV4 T4: data kept, semantics in legend)
	runtimeTraceProjMarkRecursOnChain                                    // ↺ small-cycle marker
	runtimeTraceProjMarkChainDepthChip                                   // 链上L# chain-depth chip (PTV4 T9)
	runtimeTraceProjMarkOmitted                                          // …省略N节点… long-trunk fold row (PTV4 T8 roster)
	runtimeTraceProjMarkBarScale                                         // bar full-scale caliber line (PTV4 T7 口径组)
	runtimeTraceProjMarkMergedSum                                        // ×N(a–b) same-kind SUM aggregate (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkMergedDedup                                      // ×N同值 duplicate-publication fold (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkMergedMax                                        // ×N(a–b)取最大 cross-thread fold (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkOverWindowShare                                  // 占窗>100% multi-CPU/multi-span cumulative share (PTV4 T4)
	runtimeTraceProjMarkWholeWindowIdle                                  // 整窗等待 whole-window idle annotation (PTV4 T4)
	runtimeTraceProjMarkInheritedAttribution                             // 承自归因 inherited-attribution annotation (PTV4 T4)
	runtimeTraceProjMarkIOCaliberNote                                    // NEW-3 同段IO另有…口径 note
	runtimeTraceProjMarkPeriodicSource                                   // VS-1 周期性信号源 tag
	runtimeTraceProjMarkAdjacentStanza                                   // ◇ 邻近 stanza
	runtimeTraceProjMarkBackgroundStanza                                 // ▒ 背景压力 stanza

	// runtimeTraceProjMarkCount is the completeness sentinel — every mark above
	// MUST have a runtimeTraceProjLegendCatalog entry (structurally pinned).
	runtimeTraceProjMarkCount
)

// runtimeTraceProjMarkSet is the nil-safe typed emission set: mark() on a nil
// receiver is a no-op so width-pass label computations and hand-built test
// rows never record. PTV4 T7: alongside the boolean, the set records each
// mark's first-emission sequence (边组 renders in tree order) and its emission
// count (记号组 renders in frequency order) — still recorded ONLY at emission
// sites, never re-derived from rendered text.
type runtimeTraceProjMarkSet struct {
	seen  [runtimeTraceProjMarkCount]bool
	order [runtimeTraceProjMarkCount]int
	count [runtimeTraceProjMarkCount]int
	next  int
}

func (s *runtimeTraceProjMarkSet) mark(m runtimeTraceProjMark) {
	if s == nil || m < 0 || m >= runtimeTraceProjMarkCount {
		return
	}
	if !s.seen[m] {
		s.seen[m] = true
		s.next++
		s.order[m] = s.next
	}
	s.count[m]++
}

func (s *runtimeTraceProjMarkSet) has(m runtimeTraceProjMark) bool {
	return s != nil && m >= 0 && m < runtimeTraceProjMarkCount && s.seen[m]
}

func (s *runtimeTraceProjMarkSet) firstSeen(m runtimeTraceProjMark) int {
	if s == nil || m < 0 || m >= runtimeTraceProjMarkCount {
		return 0
	}
	return s.order[m]
}

func (s *runtimeTraceProjMarkSet) emissions(m runtimeTraceProjMark) int {
	if s == nil || m < 0 || m >= runtimeTraceProjMarkCount {
		return 0
	}
	return s.count[m]
}

// Legend groups (PTV4 T7): the dynamic legend renders three titled groups —
// 边组 in tree (first-emission) order, 记号组 in emission-frequency order,
// 口径组 in stable catalog order.
const (
	runtimeTraceProjLegendGroupEdge    = "edge"
	runtimeTraceProjLegendGroupMark    = "mark"
	runtimeTraceProjLegendGroupCaliber = "caliber"
)

// runtimeTraceProjLegendEntry is one catalog row: the typed mark plus its zh/en
// legend clause (both full "- …" lines, ready to join) and its T7 group.
type runtimeTraceProjLegendEntry struct {
	Mark  runtimeTraceProjMark
	Group string
	ZH    string
	EN    string
}

// runtimeTraceProjLegendCatalog is the full, ordered mark directory of the tree
// renderer. Wording notes:
//   - the └─唤醒─ entry keeps the NEW-1 direction wording VERBATIM (客户点名
//     "谁唤醒谁" — one direction, stated twice consistently; PTV4 T7 re-ruled:
//     严禁改动);
//   - PTV4 T7: the drill/cause edges use the same parent-child explicit style
//     (下钻 verbatim per ruling: 父行在等什么);
//   - PTV4 T4: parenthesized in-row explanations moved here — the row keeps
//     only the mark + data (防回潮 pin scans the fence for regressions).
func runtimeTraceProjLegendCatalog() []runtimeTraceProjLegendEntry {
	return []runtimeTraceProjLegendEntry{
		{runtimeTraceProjMarkRootTarget, runtimeTraceProjLegendGroupMark,
			"- `🎯` = 树根:本次分析锚定的关注线程。",
			"- `🎯` = tree root: the focused thread this analysis anchors on."},
		{runtimeTraceProjMarkEdgeDrill, runtimeTraceProjLegendGroupEdge,
			"- `├─下钻─` = 父行在等什么:该子行就是父行等待的直接原因。",
			"- `├─drill─` = what the parent row is waiting on: this child row IS the direct cause of the parent's wait."},
		{runtimeTraceProjMarkEdgeWake, runtimeTraceProjLegendGroupEdge,
			"- `└─唤醒─` = 该行唤醒其父行(父行的等待由该行结束;父行依赖该行)。",
			"- `└─wakes─` = this row WAKES its parent row (the parent's wait ends on this row; the parent depends on it)."},
		{runtimeTraceProjMarkEdgeCause, runtimeTraceProjLegendGroupEdge,
			"- `├─成因─` = 该子行是父行状态的成因:同一线程的成因分解。",
			"- `├─cause─` = this child row is a cause of the parent row's state: same-thread cause decomposition."},
		{runtimeTraceProjMarkEdgeOwn, runtimeTraceProjLegendGroupEdge,
			"- `├─自身─` = 目标自身/同进程的口径行(同段墙钟的另一口径),非唤醒边。",
			"- `├─own─` = an own-/same-process caliber row of the target (another caliber of the same wall clock), not a wake edge."},
		{runtimeTraceProjMarkSemanticSpan, runtimeTraceProjLegendGroupEdge,
			"- `├─语义─`/`✦` = 该位置的语义 span(业务阶段),非调度状态行。",
			"- `├─span─`/`✦` = a semantic span (business phase) at this position, not a scheduler-state row."},
		{runtimeTraceProjMarkIconSleep, runtimeTraceProjLegendGroupMark,
			"- `☾` = 睡眠等待;症状非根因,其唤醒子行即下钻结果。",
			"- `☾` = sleep wait; a symptom, not a root cause — its wake child IS the drilldown result."},
		{runtimeTraceProjMarkIconRunnable, runtimeTraceProjLegendGroupMark,
			"- `⧖` = 可运行等待(已就绪,等待 CPU)。",
			"- `⧖` = runnable wait (ready, waiting for a CPU)."},
		{runtimeTraceProjMarkIconRunning, runtimeTraceProjLegendGroupMark,
			"- `⚙` = 运行占用(正在 CPU 上执行)。",
			"- `⚙` = running (executing on a CPU)."},
		{runtimeTraceProjMarkIconDState, runtimeTraceProjLegendGroupMark,
			"- `⛓` = D状态/IO阻塞(不可中断等待)。",
			"- `⛓` = D-state / IO block (uninterruptible wait)."},
		{runtimeTraceProjMarkIconTransit, runtimeTraceProjLegendGroupMark,
			"- `◦ 中转` = 链路中转节点,本轮无独立影响行。",
			"- `◦ transit` = a chain transit node with no standalone impact row this run."},
		{runtimeTraceProjMarkIconNoDominant, runtimeTraceProjLegendGroupMark,
			"- `◦ 无主导态` = 该行无主导调度状态,状态标签沿用影响形态。",
			"- `◦ no dominant state` = the row exposed no dominant scheduler state; its state tag keeps the impact-shape value."},
		{runtimeTraceProjMarkBadge, runtimeTraceProjLegendGroupMark,
			"- `❶❷❸` = 链上根因关注点 TOP3(按有效归因排序)。",
			"- `❶❷❸` = TOP-3 on-chain root-cause focus points (ordered by effective attribution)."},
		{runtimeTraceProjMarkStateLabel, runtimeTraceProjLegendGroupMark,
			"- 状态标签(睡眠等待/可运行等待/运行占用/IO阻塞/D状态)来自该行主导调度状态;无主导状态的行沿用影响形态。",
			"- The state tag (sleep wait / runnable wait / running / IO wait / D-state) is the row's dominant scheduler state; rows without one keep their impact-shape value."},
		{runtimeTraceProjMarkUndrillable, runtimeTraceProjLegendGroupMark,
			"- `⊘链止` = 窗口内无匹配 sched_wakeup,链止于此。",
			"- `⊘chain ends` = no matching sched_wakeup in the window; the chain ends there."},
		{runtimeTraceProjMarkCrossWindow, runtimeTraceProjLegendGroupMark,
			"- `⚠实际Xms` = 实际状态跨出分析窗口(实际共 X ms),时长条只画窗口内投影。",
			"- `⚠actual Xms` = the underlying state extends beyond the analysis window (X ms in total); the bar draws only the in-window projection."},
		{runtimeTraceProjMarkRecursOnChain, runtimeTraceProjLegendGroupMark,
			"- `↺` = 该线程在链上重复出现(小循环形态)。",
			"- `↺` = this thread recurs on the chain (small-cycle shape)."},
		{runtimeTraceProjMarkChainDepthChip, runtimeTraceProjLegendGroupCaliber,
			"- `链上L#` = 该行在唤醒链上的层深(明细表因果位置列同源)。",
			"- `chain L#` = the row's depth on the wakeup chain (same source as the detail table's causal-position column)."},
		{runtimeTraceProjMarkOmitted, runtimeTraceProjLegendGroupCaliber,
			"- `…省略N节点` = 长链中段折叠(行内列出首尾各2个节点),完整链路见原始 trace_query 记录。",
			"- `…N nodes omitted` = the middle of a long chain is folded (the row lists the first/last two nodes); the full chain remains in the trace_query record."},
		{runtimeTraceProjMarkBarScale, runtimeTraceProjLegendGroupCaliber,
			"- 时长条:满格=树头声明的尺度(关注窗口全长;窗口未采集时为本批最大投影)。",
			"- Bars: full scale = the caliber declared in the tree header (the full window; the batch max projection when no window was captured)."},
		{runtimeTraceProjMarkMergedSum, runtimeTraceProjLegendGroupCaliber,
			"- `×N(a–b)` = 同一(线程,原因)的 N 次实例合并,数值为总和,a–b 为单次范围。",
			"- `×N(a–b)` = N instances of one (thread, cause) merged; the value is the SUM, a–b the per-instance range."},
		{runtimeTraceProjMarkMergedDedup, runtimeTraceProjLegendGroupCaliber,
			"- `×N同值` = 同一测量被重复发布 N 次,数值就是那一次测量,不是 N 份。",
			"- `×N same-value` = one measurement published N times; the value IS that single measurement, never N shares."},
		{runtimeTraceProjMarkMergedMax, runtimeTraceProjLegendGroupCaliber,
			"- `×N(a–b)取最大` = 跨线程折叠 N 项,数值取成员最大;墙钟跨线程不可加和,不求和。",
			"- `×N(a–b) max` = N cross-thread rows folded; the value is the member MAX — wall clock never sums across threads."},
		{runtimeTraceProjMarkOverWindowShare, runtimeTraceProjLegendGroupCaliber,
			"- 占窗>100% = 跨CPU/多段累计投影,可合法超出窗口长度(时长条已封顶)。",
			"- A >100% window share = a multi-CPU / multi-span cumulative projection that may legitimately exceed the window (the bar is capped)."},
		{runtimeTraceProjMarkWholeWindowIdle, runtimeTraceProjLegendGroupCaliber,
			"- `整窗等待` = 该行投影覆盖整个窗口(疑似空闲线程)。",
			"- `whole-window wait` = the row's projection covers the whole window (likely an idle thread)."},
		{runtimeTraceProjMarkInheritedAttribution, runtimeTraceProjLegendGroupCaliber,
			"- `承自归因` = 该行有效归因承自其所在等待区间,非本行实测。",
			"- `inherited attribution` = the row's effective attribution is inherited from its enclosing wait interval, not measured on this row."},
		{runtimeTraceProjMarkIOCaliberNote, runtimeTraceProjLegendGroupCaliber,
			"- `同段IO另有…口径` = 同一线程同段 IO 的多口径合并显示;数值与证据保留,不重复计入归因。",
			"- `same-segment IO also measured …` = several calibers of one IO segment folded for display; values and evidence kept, never double counted."},
		{runtimeTraceProjMarkPeriodicSource, runtimeTraceProjLegendGroupCaliber,
			"- `周期性信号源` = 该行是固定周期的信号发生器,期内睡眠为正常节拍;有效归因只计可运行等待与信号迟到量,窗口投影保留原始值。",
			"- `periodic signal source` = this row is a fixed-period signal generator; in-period sleep is normal cadence. Attribution counts only runnable wait plus signal lateness; the window projection keeps the raw value."},
		{runtimeTraceProjMarkAdjacentStanza, runtimeTraceProjLegendGroupCaliber,
			"- `◇` = 邻近区段:与主链时间相邻,不在唤醒路径上。",
			"- `◇` = adjacent stanza: time-adjacent to the chain, not on the wakeup path."},
		{runtimeTraceProjMarkBackgroundStanza, runtimeTraceProjLegendGroupCaliber,
			"- `▒` = 背景压力区段:环境证据,不计入链上归因,需结合 on-chain 证据解读。",
			"- `▒` = background-pressure stanza: environmental evidence, not chain attribution; read with on-chain evidence."},
	}
}

// runtimeTraceProjLegendGroupLines renders the PTV4 T7 three-group dynamic
// legend: exactly the catalog entries whose typed mark THIS render emitted —
// 边组 in first-emission (tree) order, 记号组 in emission-frequency order
// (stable catalog order on ties), 口径组 in stable catalog order. Entry lines
// keep their catalog wording verbatim (NEW-1 included) and nest under the
// group header with a two-space indent. Empty groups render no header.
func runtimeTraceProjLegendGroupLines(marks *runtimeTraceProjMarkSet, zh bool) []string {
	catalog := runtimeTraceProjLegendCatalog()
	collect := func(group string) []runtimeTraceProjLegendEntry {
		var out []runtimeTraceProjLegendEntry
		for _, entry := range catalog {
			if entry.Group == group && marks.has(entry.Mark) {
				out = append(out, entry)
			}
		}
		return out
	}
	edges := collect(runtimeTraceProjLegendGroupEdge)
	sort.SliceStable(edges, func(i, j int) bool {
		return marks.firstSeen(edges[i].Mark) < marks.firstSeen(edges[j].Mark)
	})
	markEntries := collect(runtimeTraceProjLegendGroupMark)
	sort.SliceStable(markEntries, func(i, j int) bool {
		return marks.emissions(markEntries[i].Mark) > marks.emissions(markEntries[j].Mark)
	})
	calibers := collect(runtimeTraceProjLegendGroupCaliber)
	var lines []string
	appendGroup := func(headerZH, headerEN string, entries []runtimeTraceProjLegendEntry) {
		if len(entries) == 0 {
			return
		}
		if zh {
			lines = append(lines, headerZH)
		} else {
			lines = append(lines, headerEN)
		}
		for _, entry := range entries {
			if zh {
				lines = append(lines, "  "+entry.ZH)
			} else {
				lines = append(lines, "  "+entry.EN)
			}
		}
	}
	appendGroup("- 边(按树内出现顺序):", "- Edges (in tree order):", edges)
	appendGroup("- 记号(按出现频次):", "- Marks (by frequency):", markEntries)
	appendGroup("- 口径:", "- Calibers:", calibers)
	return lines
}

// runtimeTraceProjModelHasPeriodicRow reports whether any rendered row carries
// the typed VS-1 periodic-source flag — gates ONLY the display of the
// discount-caliber legend line (precise boolean; nothing structural).
func runtimeTraceProjModelHasPeriodicRow(model runtimeTraceProjTreeModel) bool {
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.PeriodicSource {
				return true
			}
		}
	}
	return false
}

// missingWakeup reports whether any rendered row carries the typed
// missing_wakeup undrillable reason — the OTHER flat-fallback cause (§7.30
// 裁定3): the sleep interval had no sched_wakeup record in the selected window.
func (m runtimeTraceProjTreeModel) missingWakeup() bool {
	for _, rows := range [][]runtimeTraceProjTreeRow{m.TreeRows, m.SelfRows, m.Adjacent, m.Background} {
		for _, row := range rows {
			if strings.TrimSpace(row.Node.UndrillableReason) == "missing_wakeup" {
				return true
			}
		}
	}
	return false
}

type runtimeTraceProjTreeNode struct {
	row      runtimeTraceProjTreeRow
	children []*runtimeTraceProjTreeNode
}

// --- model construction ------------------------------------------------------

func buildRuntimeTraceProjTreeModel(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) runtimeTraceProjTreeModel {
	model := runtimeTraceProjTreeModel{
		WindowMS:                     projection.WindowDurationMS(),
		WakeupChainRecommendedNotRun: projection.WakeupChainRecommendedNotRun,
		Marks:                        &runtimeTraceProjMarkSet{},
	}
	path := runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)
	if len(path) >= 2 {
		model.Target = path[len(path)-1]
	}
	targetKey := runtimeTraceCausalProjectionCanonicalNode(model.Target)

	// On-chain node universe: primaries + on-chain bucket + hops, deduped by
	// node key (buckets deliberately overlap; see the aggregation layer).
	// Semantic spans are excluded here — their classified copies also live in
	// OnChainCauses, but they render exclusively through the ✦ 语义 lane (a span
	// consumed as a same-subject "cause" row would appear twice).
	chainUniverse := runtimeTraceProjDedupNodes(
		runtimeTraceProjExcludeSemanticSpans(
			append(append(append([]types.TraceCausalProjectionNode{},
				runtimeTraceCausalProjectionPrimaryRoots(projection)...),
				projection.OnChainCauses...),
				projection.SupportingHops...)))
	// §7.30 裁定1: only relation-resolved rows may enter the on-chain tree.
	// Aggregate-metric rows and unknown-thread rows whose depth cannot attach
	// demote to the background-pressure stanza (merged with the existing
	// background rows) instead of rendering as on-chain placeholders.
	trunkLen := 0
	if len(path) >= 2 {
		trunkLen = len(path) - 1
	}
	model.TrunkLen = trunkLen
	chainNodes := make([]types.TraceCausalProjectionNode, 0, len(chainUniverse))
	var demoted []types.TraceCausalProjectionNode
	for _, node := range chainUniverse {
		if runtimeTraceProjNodeDemotedToBackground(node, trunkLen) {
			demoted = append(demoted, node)
			continue
		}
		chainNodes = append(chainNodes, node)
	}
	// NEW-3 (§7.6 回访): fold same-subject same-segment IO calibers into their
	// max-impact row BEFORE the subject buckets are built, so the peers never
	// mint sibling tree rows or same-subject cause rows. The fold map is
	// re-attached to the surviving primary's row after flatten (its row Kind —
	// self or tree — is only known then).
	chainNodes, ioFoldPeers := runtimeTraceProjFoldSameSubjectIONodes(chainNodes)
	bySubject := map[string][]types.TraceCausalProjectionNode{}
	for _, node := range chainNodes {
		key := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		bySubject[key] = append(bySubject[key], node)
	}
	consumed := map[string]bool{}
	consume := func(node types.TraceCausalProjectionNode) {
		consumed[runtimeTraceCausalProjectionNodeKey(node)] = true
	}

	// Target's own rows (state / blocked-wait views of the focused thread).
	if targetKey != "" {
		for _, node := range bySubject[targetKey] {
			consume(node)
			model.SelfRows = append(model.SelfRows, runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			})
		}
	}

	// Trunk: path minus target, ordered depth 1..K (nearest upstream first),
	// bounded for very long chains.
	var trunk []string
	for i := len(path) - 2; i >= 0; i-- {
		trunk = append(trunk, path[i])
	}
	omitStart, omitEnd := -1, -1
	cyclePeriod, cycleCount := 0, 0
	if len(trunk) > runtimeTraceProjTreeTrunkMaxNodes {
		head := runtimeTraceProjTreeTrunkMaxNodes/2 + 1
		tail := runtimeTraceProjTreeTrunkMaxNodes - head
		omitStart, omitEnd = head, len(trunk)-tail
		cyclePeriod, cycleCount = runtimeTraceCausalProjectionRepeatingPath(path)
	}
	// H11: mark trunk rows whose canonical subject already appeared earlier on
	// the rendered chain (root target first, then depth 1..K). This catches the
	// small-cycle shape (VSyncGenerator→tppmgr→VSyncGenerator) that the ≥6-node
	// cycle detector above never sees. Canonical verbatim equality only.
	recurs := make([]bool, len(trunk))
	seenOnChain := map[string]bool{}
	if targetKey != "" {
		seenOnChain[targetKey] = true
	}
	for i, subject := range trunk {
		key := runtimeTraceCausalProjectionCanonicalNode(subject)
		if seenOnChain[key] {
			recurs[i] = true
		}
		seenOnChain[key] = true
	}

	semantics := append([]types.TraceCausalProjectionNode(nil), projection.SemanticSpans...)
	semanticBySubject := map[string][]types.TraceCausalProjectionNode{}
	for _, span := range semantics {
		key := runtimeTraceCausalProjectionCanonicalNode(span.Subject)
		semanticBySubject[key] = append(semanticBySubject[key], span)
	}
	semanticConsumed := map[string]bool{}

	depthAttach := map[int][]types.TraceCausalProjectionNode{}
	trunkKeys := map[string]int{}
	for i, subject := range trunk {
		trunkKeys[runtimeTraceCausalProjectionCanonicalNode(subject)] = i + 1
	}
	for _, node := range chainNodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if consumed[key] {
			continue
		}
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if _, onTrunk := trunkKeys[subjectKey]; onTrunk {
			continue // trunk pass consumes below
		}
		if node.ChainDepth > 0 && node.ChainDepth <= len(trunk) {
			depthAttach[node.ChainDepth] = append(depthAttach[node.ChainDepth], node)
			consume(node)
		}
	}

	// Recursive trunk build (depth d node's child = depth d+1 node).
	var buildTrunk func(idx int, parentName string) []*runtimeTraceProjTreeNode
	buildTrunk = func(idx int, parentName string) []*runtimeTraceProjTreeNode {
		if idx >= len(trunk) {
			return nil
		}
		if idx == omitStart {
			// PTV4 T8: the fold row names the folded segment's first/last two
			// nodes (the names were always in the typed path — display upgrade
			// only). ≤4 omitted nodes list fully via the head roster.
			var head, tail []string
			if omitEnd-omitStart <= 4 {
				head = append(head, trunk[omitStart:omitEnd]...)
			} else {
				head = append(head, trunk[omitStart:omitStart+2]...)
				tail = append(tail, trunk[omitEnd-2:omitEnd]...)
			}
			omitted := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Kind: runtimeTraceProjTreeRowOmitted, Omitted: omitEnd - omitStart,
				CyclePeriod: cyclePeriod, CycleCount: cycleCount, Depth: idx + 1,
				OmittedHead: head, OmittedTail: tail,
			}}
			omitted.children = buildTrunk(omitEnd, "…")
			return []*runtimeTraceProjTreeNode{omitted}
		}
		subject := trunk[idx]
		depth := idx + 1
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(subject)
		var main types.TraceCausalProjectionNode
		var extra []types.TraceCausalProjectionNode
		hasData := false
		for _, node := range bySubject[subjectKey] {
			if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
				continue
			}
			if !hasData {
				main, hasData = node, true
				continue
			}
			if runtimeTraceProjNodeDisplayImpact(node) > runtimeTraceProjNodeDisplayImpact(main) {
				extra = append(extra, main)
				main = node
			} else {
				extra = append(extra, node)
			}
		}
		if hasData {
			consume(main)
			for _, node := range extra {
				consume(node)
			}
		} else {
			main = types.TraceCausalProjectionNode{Subject: subject, ChainDepth: depth}
		}
		edge := runtimeTraceProjTreeEdgeWake
		if depth == 1 {
			edge = runtimeTraceProjTreeEdgeDrill
		}
		trunkNode := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: main, Kind: runtimeTraceProjTreeRowChain, Edge: edge, Depth: depth,
			Parent: parentName, HasData: hasData, RecursOnChain: recurs[idx],
			EvidenceTag: runtimeTraceProjEvidenceTag(main, evidence, zh),
		}}
		for _, node := range extra {
			trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowCause, Edge: runtimeTraceProjTreeEdgeCause,
				Depth: depth, Parent: subject, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
		// Semantic spans attach under THEIR subject's wakee — i.e. as this
		// node's children when the span's subject is this node's upstream child
		// subject. Handled after the child trunk is built: spans whose subject
		// is trunk[idx+1] become siblings of that deeper trunk node, faithful
		// to the typed impact point (presentation v3 §5).
		children := buildTrunk(idx+1, subject)
		trunkNode.children = append(trunkNode.children, children...)
		if idx+1 < len(trunk) {
			deeperKey := runtimeTraceCausalProjectionCanonicalNode(trunk[idx+1])
			if !semanticConsumed[deeperKey] {
				for _, span := range semanticBySubject[deeperKey] {
					trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
						Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
						Depth: span.ChainDepth, Parent: subject, HasData: true,
						EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
					}})
				}
				semanticConsumed[deeperKey] = true
			}
		}
		for _, node := range depthAttach[depth+1] {
			trunkNode.children = append(trunkNode.children, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
				Depth: depth + 1, Parent: subject, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
		return []*runtimeTraceProjTreeNode{trunkNode}
	}

	var roots []*runtimeTraceProjTreeNode
	roots = append(roots, buildTrunk(0, model.Target)...)
	// Semantic spans whose subject is the depth-1 trunk node hang off the root.
	if len(trunk) > 0 {
		d1Key := runtimeTraceCausalProjectionCanonicalNode(trunk[0])
		if !semanticConsumed[d1Key] {
			for _, span := range semanticBySubject[d1Key] {
				roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
					Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
					Depth: span.ChainDepth, Parent: model.Target, HasData: true,
					EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
				}})
			}
			semanticConsumed[d1Key] = true
		}
		for _, node := range depthAttach[1] {
			roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
				Depth: 1, Parent: model.Target, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
	}
	// Remaining on-chain rows (no trunk membership, no resolvable depth) — a
	// typed-faithful "depth unresolved" branch instead of an invented position.
	for _, node := range chainNodes {
		if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
			continue
		}
		consume(node)
		edge := runtimeTraceProjTreeEdgeWake
		// F2: a process-level IO caliber row of the 🎯 target's OWN process is
		// not an upstream waker — hard-coding the wake edge here made the fence
		// claim 唤醒 while the relation column said 自身进程IO. The typed node
		// predicate decides ONCE at build time; every display surface (fence
		// edge, relation cell, legend entry) reads the resulting row.Edge.
		if runtimeTraceProjOwnProcessIONode(node, model.Target) {
			edge = runtimeTraceProjTreeEdgeOwn
		}
		roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowDepthless, Edge: edge,
			Depth: node.ChainDepth, Parent: model.Target, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		}})
	}
	// Orphan semantic spans (subject not on the trunk at all).
	for _, span := range semantics {
		key := runtimeTraceCausalProjectionCanonicalNode(span.Subject)
		if semanticConsumed[key] {
			continue
		}
		roots = append(roots, &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: span, Kind: runtimeTraceProjTreeRowSemantic, Edge: runtimeTraceProjTreeEdgeSemantic,
			Depth: span.ChainDepth, Parent: model.Target, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(span, evidence, zh),
		}})
	}

	var flatten func(nodes []*runtimeTraceProjTreeNode, indent int, ancestors []bool)
	flatten = func(nodes []*runtimeTraceProjTreeNode, indent int, ancestors []bool) {
		for i, n := range nodes {
			last := i == len(nodes)-1
			row := n.row
			row.Indent = indent
			row.Last = last
			row.Ancestors = append([]bool(nil), ancestors...)
			model.TreeRows = append(model.TreeRows, row)
			flatten(n.children, indent+1, append(append([]bool(nil), ancestors...), !last))
		}
	}
	flatten(roots, 0, nil)

	// NEW-3: attach the folded IO calibers to the primary's row (target self
	// row or tree row) and register every folded node on the evidence index —
	// the caliber note is the peers' display carrier, the index keeps their
	// full locators.
	if len(ioFoldPeers) > 0 {
		attach := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				for _, peer := range ioFoldPeers[runtimeTraceCausalProjectionNodeKey(rows[i].Node)] {
					rows[i].IOFoldPeers = append(rows[i].IOFoldPeers, runtimeTraceProjIOFoldPeer{
						Token:       strings.TrimSpace(peer.TypeToken),
						ImpactMS:    runtimeTraceProjNodeDisplayImpact(peer),
						EvidenceTag: runtimeTraceProjEvidenceTag(peer, evidence, zh),
					})
				}
			}
		}
		attach(model.SelfRows)
		attach(model.TreeRows)
	}

	for _, node := range runtimeTraceProjAdjacentNodesForDisplay(projection.AdjacentCauses) {
		model.Adjacent = append(model.Adjacent, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		})
	}
	backgroundSeen := map[string]bool{}
	for _, node := range projection.BackgroundCauses {
		backgroundSeen[runtimeTraceCausalProjectionNodeKey(node)] = true
		model.Background = append(model.Background, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		})
	}
	// 裁定1 demoted rows merge into the same background stanza. The evidence
	// roster entry is taken from the ORIGINAL node (audit keeps the raw typed
	// provenance); the display copy is then normalized to background semantics —
	// its former on-chain/primary labeling was the pollution being removed. The
	// raw observation record itself is untouched.
	for _, node := range demoted {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if backgroundSeen[key] {
			continue
		}
		backgroundSeen[key] = true
		tag := runtimeTraceProjEvidenceTag(node, evidence, zh)
		node.ChainRelevance = "background"
		if node.Role == types.TraceCausalRolePrimaryRootCause || node.Role == types.TraceCausalRoleCausalHop {
			node.Role = types.TraceCausalRoleRootCauseContext
		}
		if strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary") {
			node.Predicate = "root_cause_context"
		}
		model.Background = append(model.Background, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			EvidenceTag: tag,
		})
	}
	if len(demoted) > 0 {
		sort.SliceStable(model.Background, func(i, j int) bool {
			return runtimeTraceProjNodeDisplayImpact(model.Background[i].Node) >
				runtimeTraceProjNodeDisplayImpact(model.Background[j].Node)
		})
	}

	// CMP-7a: flat-fallback renders (no resolved target) stamp every row so the
	// layer chip and the detail table's 因果位置 cell agree with the flat header
	// instead of claiming "on-chain" (customer compare audit 2026-07-03 §7).
	if strings.TrimSpace(model.Target) == "" {
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
			for i := range rows {
				rows[i].FlatChain = true
			}
		}
	}
	model.BarMaxMS = runtimeTraceProjModelMaxImpact(model)
	// RN-3(b): pin the conclusion-consumed node's key on the model so the
	// detail table's 因果位置·优先级 column follows the SAME selection
	// (runtimeTraceProjLeadSelect is deterministic on (projection, model), so
	// the conclusion line re-running it later cannot disagree).
	if lead, _ := runtimeTraceProjLeadSelect(projection, model); lead != nil {
		model.LeadKey = runtimeTraceCausalProjectionNodeKey(*lead)
	}
	runtimeTraceProjAssignTopBadges(&model)
	return model
}

// runtimeTraceProjAssignTopBadges stamps the PTV4 T6 ❶❷❸ badges: among the
// rendered CHAIN-lane rows (chain / cause / depthless kinds — the stanza
// families are off-chain by construction) whose node carries the engine's
// typed root_cause_rank (Node.Rank > 0, the root_cause_rank↔projection
// association the compile layer already established) AND a positive effective
// attribution, the top three by EffectiveImpactMS descending (ties keep render
// order). Typed fields only — no prose judgment, no new LLM signal; clearing
// a node's Rank removes its badge (pinned by mutation test).
func runtimeTraceProjAssignTopBadges(model *runtimeTraceProjTreeModel) {
	type candidate struct {
		row   *runtimeTraceProjTreeRow
		value float64
	}
	var candidates []candidate
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if !row.HasData || row.Node.Rank <= 0 {
			continue
		}
		switch row.Kind {
		case runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowCause, runtimeTraceProjTreeRowDepthless:
		default:
			continue
		}
		if row.Node.EffectiveImpactMS <= 0 {
			continue
		}
		candidates = append(candidates, candidate{row: row, value: row.Node.EffectiveImpactMS})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].value > candidates[j].value
	})
	for i := 0; i < len(candidates) && i < 3; i++ {
		candidates[i].row.Badge = i + 1
	}
}

// runtimeTraceProjBadgeGlyph maps the typed badge rank to its glyph. Empty for
// rank 0 / out-of-range (defensive; badges are assigned 1..3 only).
func runtimeTraceProjBadgeGlyph(rank int) string {
	switch rank {
	case 1:
		return "❶"
	case 2:
		return "❷"
	case 3:
		return "❸"
	}
	return ""
}

// runtimeTraceProjCausalPositionLayerCell is the CMP-7a display wrapper over
// the causal-position layer cell: a flat-fallback row whose chain relevance
// would read "on-chain" renders the flat form instead — the header just said
// the wakeup chain could not be traced upstream, and both surfaces (tree layer
// chip, lossless-table 因果位置 column) must agree with it. Every other layer
// value passes through verbatim.
func runtimeTraceProjCausalPositionLayerCell(node types.TraceCausalProjectionNode, zh, flatChain bool) string {
	layer := runtimeTraceCausalProjectionLayerCell(node, zh)
	if flatChain && layer == "on-chain" {
		if zh {
			return "平铺(链不可上溯)"
		}
		return "flat (chain not traceable)"
	}
	return layer
}

// runtimeTraceProjDetailPositionCell renders the detail table's
// 因果位置·优先级 cell. RN-3(b) (§7.9 runnable 主导场景审计 2026-07-04): the
// "主根因 · 主要关注" label follows the node the conclusion line ACTUALLY
// consumed (typed node-key equality with model.LeadKey) — an UNCONSUMED
// primary-tier row sitting in the background stanza (engine tier=primary/
// rank=1 直投: the record carried chain_relevance=background, so the RAW
// primary-role copy entered BackgroundCauses and bypassed the 裁定1 demotion
// normalization) demotes on DISPLAY to 背景 · 支撑参考(rank=N) instead of
// contradicting a conclusion that says no on-chain primary was located. The
// rank annotation stays for audit. On-chain primary-tier rows keep their
// labels unchanged (engine-tier facts on the chain contradict nothing — the
// single-artifact rank-lead goldens are pinned label-stable), and a consumed
// background row keeps 主根因 (除非确被结论消费 cuts exactly the other way).
func runtimeTraceProjDetailPositionCell(row runtimeTraceProjTreeRow, leadKey string, zh bool) string {
	node := row.Node
	if row.Kind == runtimeTraceProjTreeRowBackground && runtimeTraceProjPrimaryTierNode(node) &&
		(leadKey == "" || runtimeTraceCausalProjectionNodeKey(node) != leadKey) {
		display := node
		display.Role = types.TraceCausalRoleRootCauseContext
		if strings.HasPrefix(strings.TrimSpace(display.Predicate), "root_cause_primary") {
			display.Predicate = "root_cause_context"
		}
		display.ChainRelevance = "background"
		cell := runtimeTraceProjCausalPositionLayerCell(display, zh, row.FlatChain) + " · " +
			runtimeTraceCausalProjectionPriorityCell(display, zh)
		if node.Rank > 0 {
			if zh {
				cell += fmt.Sprintf("(rank=%d)", node.Rank)
			} else {
				cell += fmt.Sprintf(" (rank=%d)", node.Rank)
			}
		}
		return cell
	}
	return runtimeTraceProjCausalPositionLayerCell(node, zh, row.FlatChain) + " · " +
		runtimeTraceCausalProjectionPriorityCell(node, zh)
}

// runtimeTraceProjPrimaryTierNode reports the engine's primary-tier typing on
// a node — the SAME two precise signals the layer/priority cells key on (Role
// enum + root_cause_primary predicate prefix), extracted so the RN-3(b)
// display gate and those cells cannot drift.
func runtimeTraceProjPrimaryTierNode(node types.TraceCausalProjectionNode) bool {
	return node.Role == types.TraceCausalRolePrimaryRootCause ||
		strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary")
}

// runtimeTraceProjNodeDemotedToBackground implements §7.30 裁定1: a row may sit
// on the on-chain tree only when its relation is resolved. Typed
// aggregate-metric rows (window-scoped cpu/io/irq/ipi/frequency/supply pressure
// with no thread subject) and unknown-thread sentinel rows whose typed chain
// depth cannot attach inside the wakeup trunk demote to the background-pressure
// stanza. Precise typed signals only — never a prose heuristic.
func runtimeTraceProjNodeDemotedToBackground(node types.TraceCausalProjectionNode, trunkLen int) bool {
	if node.IsAggregateMetric() {
		return true
	}
	switch runtimeTraceCausalProjectionCanonicalNode(node.Subject) {
	case "unknown-thread", "unknown":
	default:
		return false
	}
	return node.ChainDepth <= 0 || node.ChainDepth > trunkLen
}

// --- NEW-3 same-subject same-segment IO caliber fold (§7.6 回访 2026-07-04) ---

// runtimeTraceProjSameSegmentIOToken reports whether the node carries one of
// the typed IO caliber tokens of the NEW-3 fold set. Exact match on the
// producer's verbatim TypeToken ("type=" rich note) only — never StateKind,
// never Object prose. io_burst_episode ⊇ io_wait describe the same segment
// through two calibers; other tokens never enter the fold.
func runtimeTraceProjSameSegmentIOToken(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(strings.ToLower(node.TypeToken)) {
	case "io_burst_episode", "io_wait":
		return true
	}
	return false
}

// runtimeTraceProjFoldSameSubjectIONodes implements the NEW-3 display grouping
// (对比场景客户回访 2026-07-04, §7.6): the SAME thread subject
// (com.xs.fm.lite-21538) published one IO segment through several calibers —
// io_burst_episode 232.428/226.153ms + io_wait 112.011/107.672ms, heavily
// overlapping line spans, near-equal but NOT equal values (so the V4
// exact-value dedup correctly does not fire) — and the tree showed four
// sibling IO rows for one burst. Rows with the same canonical subject, a typed
// IO caliber token and PAIRWISE-overlapping line intervals fold into the
// max-impact row; the folded calibers surface as a load-bearing note on that
// primary row with every evidence id kept (the caller registers each folded
// node on the evidence index). Precise signals only: verbatim canonical
// subject + typed token set + interval-overlap booleans. A group member
// without a valid line interval, or any non-overlapping pair, keeps the whole
// group unfolded (fail closed). Display grouping only — the underlying
// observations and the projection buckets are untouched.
//
// F-1 (§7.6 回访聚焦复核 2026-07-04): the group key carries the CHAIN LANE, not
// just the canonical subject. A chain-ATTACHED caliber row (resolved
// ChainDepth ≥ 1) and a depthless row (ChainDepth ≤ 0) of the same subject sit
// in different attribution lanes: the attached row's cumulative drives the
// on-chain attribution numerator (F2) while the depthless row is the NEW-6
// residual-overlap lane. Folding across lanes deleted the only depth-N data
// row — attributed dropped to 0, the fence lost a data-real wakeup row, and
// the NEW-6 clause inverted. Same lane only: both depthless, or the same
// resolved depth (one integer comparison on the typed ChainDepth — precise
// signal, display grouping contract preserved).
func runtimeTraceProjFoldSameSubjectIONodes(nodes []types.TraceCausalProjectionNode) ([]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	groups := map[string][]int{}
	var groupOrder []string
	for i, node := range nodes {
		if node.IsAggregateMetric() || !runtimeTraceProjSameSegmentIOToken(node) {
			continue
		}
		key := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if key == "" {
			continue
		}
		key += "\x00lane=" + strconv.Itoa(runtimeTraceProjChainLane(node))
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], i)
	}
	folded := map[int]bool{}
	foldPeers := map[string][]types.TraceCausalProjectionNode{}
	for _, key := range groupOrder {
		members := groups[key]
		if len(members) < 2 || !runtimeTraceProjIOMembersPairwiseOverlap(nodes, members) {
			continue
		}
		primary := members[0]
		for _, idx := range members[1:] {
			if runtimeTraceProjNodeDisplayImpact(nodes[idx]) > runtimeTraceProjNodeDisplayImpact(nodes[primary]) {
				primary = idx
			}
		}
		primaryKey := runtimeTraceCausalProjectionNodeKey(nodes[primary])
		for _, idx := range members {
			if idx == primary {
				continue
			}
			folded[idx] = true
			foldPeers[primaryKey] = append(foldPeers[primaryKey], nodes[idx])
		}
	}
	if len(folded) == 0 {
		return nodes, nil
	}
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes)-len(folded))
	for i, node := range nodes {
		if folded[i] {
			continue
		}
		out = append(out, node)
	}
	return out, foldPeers
}

// runtimeTraceProjChainLane normalizes a node's typed ChainDepth into its
// display/attribution lane (F-1): every unattached depth (≤ 0) is the ONE
// depthless lane; each resolved depth ≥ 1 is its own chain lane.
func runtimeTraceProjChainLane(node types.TraceCausalProjectionNode) int {
	if node.ChainDepth <= 0 {
		return 0
	}
	return node.ChainDepth
}

// runtimeTraceProjIOMembersPairwiseOverlap is the NEW-3 interval gate: every
// member must expose a valid 1-based line interval and every PAIR must
// intersect (the same boolean the strict duplicate fold uses). One
// non-overlapping pair — two genuinely distinct IO bursts — vetoes the fold.
func runtimeTraceProjIOMembersPairwiseOverlap(nodes []types.TraceCausalProjectionNode, members []int) bool {
	for _, idx := range members {
		if nodes[idx].LineStart <= 0 || nodes[idx].LineEnd < nodes[idx].LineStart {
			return false
		}
	}
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			if !runtimeTraceProjLineSpansOverlap(nodes[members[i]], nodes[members[j]]) {
				return false
			}
		}
	}
	return true
}

// runtimeTraceProjIOFoldNoteText renders the NEW-3 caliber note carried by the
// fold's primary row: folded values grouped per raw token in first-appearance
// order, plus every folded evidence tag ("同段IO另有 io_wait
// 112.011/107.672ms、io_burst_episode 226.153ms 口径;证据 E3、E4、E5"). The
// note is the folded rows' only remaining display carrier, so callers must
// treat it as load-bearing (never elided).
func runtimeTraceProjIOFoldNoteText(peers []runtimeTraceProjIOFoldPeer, zh bool) string {
	type tokenGroup struct {
		token  string
		values []string
	}
	var groups []tokenGroup
	index := map[string]int{}
	var tags []string
	for _, peer := range peers {
		token := strings.TrimSpace(peer.Token)
		i, ok := index[token]
		if !ok {
			i = len(groups)
			index[token] = i
			groups = append(groups, tokenGroup{token: token})
		}
		groups[i].values = append(groups[i].values, fmt.Sprintf("%.3f", peer.ImpactMS))
		if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
			tags = append(tags, tag)
		}
	}
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		parts = append(parts, strings.TrimSpace(g.token+" "+strings.Join(g.values, "/")+"ms"))
	}
	if zh {
		text := "同段IO另有 " + strings.Join(parts, "、") + " 口径"
		if len(tags) > 0 {
			text += ";证据 " + strings.Join(tags, "、")
		}
		return text
	}
	text := "same-segment IO also measured " + strings.Join(parts, ", ")
	if len(tags) > 0 {
		text += "; evidence " + strings.Join(tags, ", ")
	}
	return text
}

// runtimeTraceProjOwnProcessIONode is the NEW-3/F2 typed predicate for an
// own-process IO caliber node: an IO caliber token, and the subject's trailing
// -pid integer equal to the target's trailing -pid integer while the labels
// differ (the equal-label case is the self-row lane, which already renders
// 自身状态). Evaluated ONCE at model build to stamp the depthless row's own
// edge (runtimeTraceProjTreeEdgeOwn) — downstream surfaces read the edge.
func runtimeTraceProjOwnProcessIONode(node types.TraceCausalProjectionNode, target string) bool {
	if !runtimeTraceProjSameSegmentIOToken(node) {
		return false
	}
	target = strings.TrimSpace(target)
	_, targetPid, ok := runtimeTraceProjSplitNamePid(target)
	if !ok {
		return false
	}
	subject := strings.TrimSpace(node.Subject)
	_, subjectPid, ok := runtimeTraceProjSplitNamePid(subject)
	if !ok || subjectPid != targetPid {
		return false
	}
	return runtimeTraceCausalProjectionCanonicalNode(subject) != runtimeTraceCausalProjectionCanonicalNode(target)
}

// runtimeTraceProjOwnProcessIORow reports whether a rendered row is the
// depthless own-process IO caliber lane (NEW-3 as corrected by F2, adversarial
// re-review 2026-07-04): Kind depthless + the own edge stamped at build time.
// The chain-ATTACHED variant (resolved ChainDepth ≥ 1, Kind chain) is excluded
// on purpose — its wake edge comes from typed chain data and its cumulative
// drives the on-chain attribution numerator, so the pre-F2 relation rewrite to
// 自身进程IO contradicted a data-real 唤醒 edge; chain rows keep 唤醒 on every
// surface.
func runtimeTraceProjOwnProcessIORow(row runtimeTraceProjTreeRow) bool {
	return row.Kind == runtimeTraceProjTreeRowDepthless && row.Edge == runtimeTraceProjTreeEdgeOwn
}

func runtimeTraceProjExcludeSemanticSpans(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		if node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span" {
			continue
		}
		out = append(out, node)
	}
	return out
}

func runtimeTraceProjDedupNodes(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

// runtimeTraceProjAdjacentNodesForDisplay prepares the adjacent stanza (H6,
// customer audit 2026-07-03): first the same typed node-key dedupe the
// background stanza already runs, then one strictly-equal duplicate-measurement
// fold — same canonical subject + same canonical object + same canonical
// TypeToken + EXACTLY equal projected ms (pure float equality, never ±ε) AND a
// precise line/time overlap (RF2a, adversarial review 2026-07-03) merge into
// the first row's DuplicatePublications/MergedEvidenceIDs. The real customer
// shape was two irq_activity rows with identical 35.350ms over overlapping
// line ranges (793201-830007 vs 793204-830012) rendering as two stanza rows.
//
// V4 (customer revisit 2026-07-03): the load-bearing fold now lives in the
// aggregation layer (traceCausalProjectionDedupDuplicatePublications, pre-R2,
// all buckets) so ≥3 duplicates can no longer be SUM-grabbed by R2 first. This
// display pass is retained as the safety net for projections that did not go
// through the record compile, and it writes the SAME typed field
// (Node.DuplicatePublications) instead of its former private MergedCount +
// row-flag semantics — one home, one label fork.
func runtimeTraceProjAdjacentNodesForDisplay(nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	seen := map[string]bool{}
	var out []types.TraceCausalProjectionNode
	for _, node := range nodes {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		merged := false
		for i := range out {
			// Upstream ×N aggregates carry SUM semantics; only fold single
			// measurements into single measurements (or into a duplicate fold —
			// its publication count accumulates).
			if node.MergedCount > 1 || out[i].MergedCount > 1 {
				continue
			}
			if !runtimeTraceProjSameAdjacentMeasurement(out[i], node) {
				continue
			}
			runtimeTraceProjAbsorbAdjacentDuplicate(&out[i], node)
			merged = true
			break
		}
		if merged {
			continue
		}
		out = append(out, node)
	}
	return out
}

// runtimeTraceProjSameAdjacentMeasurement is the strict duplicate-measurement
// identity: canonical subject + canonical object + canonical TypeToken equal,
// the positive projected ms exactly equal, AND the two rows precisely overlap
// in the artifact — line-range intersection or time-span intersection (RF2a,
// adversarial review 2026-07-03: two REAL irq bursts at different moments can
// quantize to the same %.3f ms; folding them halves the reported contribution).
// When neither location lane is determinate for the pair the fold fails open
// to two rows — value equality alone never merges.
func runtimeTraceProjSameAdjacentMeasurement(a, b types.TraceCausalProjectionNode) bool {
	return runtimeTraceCausalProjectionCanonicalNode(a.Subject) == runtimeTraceCausalProjectionCanonicalNode(b.Subject) &&
		runtimeTraceCausalProjectionCanonicalNode(a.Object) == runtimeTraceCausalProjectionCanonicalNode(b.Object) &&
		runtimeTraceCausalProjectionCanonicalNode(a.TypeToken) == runtimeTraceCausalProjectionCanonicalNode(b.TypeToken) &&
		a.ImpactMS > 0 && a.ImpactMS == b.ImpactMS &&
		(runtimeTraceProjLineSpansOverlap(a, b) || runtimeTraceProjTimeSpansOverlap(a, b))
}

// runtimeTraceProjLineSpansOverlap is the boolean line-range intersection; both
// nodes must expose a valid range of their own (same guard style as the typed
// peer-alias fold's traceCausalProjectionSpansOverlap in internal/types).
func runtimeTraceProjLineSpansOverlap(a, b types.TraceCausalProjectionNode) bool {
	if a.LineStart <= 0 || a.LineEnd < a.LineStart || b.LineStart <= 0 || b.LineEnd < b.LineStart {
		return false
	}
	return a.LineStart <= b.LineEnd && b.LineStart <= a.LineEnd
}

// runtimeTraceProjTimeSpansOverlap is the boolean time-span intersection; both
// nodes must expose a valid span of their own. Local isomorph of the unexported
// types-layer traceCausalProjectionSpansOverlap
// (internal/types/trace_causal_projection_aggregate.go) — keep the two aligned.
func runtimeTraceProjTimeSpansOverlap(a, b types.TraceCausalProjectionNode) bool {
	if a.StartTs <= 0 || a.EndTs <= a.StartTs || b.StartTs <= 0 || b.EndTs <= b.StartTs {
		return false
	}
	return a.StartTs < b.EndTs && b.StartTs < a.EndTs
}

// runtimeTraceProjAbsorbAdjacentDuplicate folds one duplicate measurement into
// the surviving first-occurrence row: publication count + evidence union only —
// the projected value stays the survivor's (the rows measured the same amount;
// a sum would double-count the wall clock). V4: writes the typed
// DuplicatePublications field shared with the aggregation-layer pass; the
// former MergedCount/MergedMin/Max writes are gone (those carry SUM-aggregate
// semantics), and the former subject-roster append was dead code — the fold
// identity requires equal canonical subjects.
func runtimeTraceProjAbsorbAdjacentDuplicate(survivor *types.TraceCausalProjectionNode, dup types.TraceCausalProjectionNode) {
	if survivor.DuplicatePublications < 1 {
		survivor.DuplicatePublications = 1
	}
	add := dup.DuplicatePublications
	if add < 1 {
		add = 1
	}
	survivor.DuplicatePublications += add
	absorbed := map[string]bool{runtimeTraceCausalProjectionCanonicalNode(survivor.EvidenceID): true}
	for _, id := range survivor.MergedEvidenceIDs {
		absorbed[runtimeTraceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || absorbed[runtimeTraceCausalProjectionCanonicalNode(raw)] {
			return
		}
		absorbed[runtimeTraceCausalProjectionCanonicalNode(raw)] = true
		survivor.MergedEvidenceIDs = append(survivor.MergedEvidenceIDs, raw)
	}
	appendID(dup.EvidenceID)
	for _, id := range dup.MergedEvidenceIDs {
		appendID(id)
	}
}

// runtimeTraceProjCrossThreadAggregateType reports whether a node is a
// CROSS-THREAD cumulative aggregate row (CMP-3, customer compare audit
// 2026-07-03 §7.2): a window/CPU-scoped aggregate metric whose ms value sums
// thread·ms (cpu·ms) across threads/CPUs and therefore is NOT wall clock —
// supply_pressure 101084.884ms inside a 2.1s window is the customer shape.
// Two precise typed signals must BOTH hold:
//   - the typed subject_kind=aggregate_metric marker (IsAggregateMetric —
//     structurally no thread subject; rows with a real thread subject, e.g.
//     the H8 irq/151-dpu burst, keep their existing bar + >100% annotation);
//   - the producer's typed kind token (TypeToken first, the Object cause lane
//     second — same precedence as the H20 shape lane) is in the cross-thread
//     cumulative set below, kept in lockstep with the engine's
//     rootCauseAggregateMetricTypes (internal/tracequery/query.go).
//
// Consumers: bar-scale anchoring excludes these rows (the scale anchors
// wall-clock values only), and their rendered value carries the
// "(跨线程累计,非墙钟)" unit annotation plus the CMP-9 normalized density.
func runtimeTraceProjCrossThreadAggregateType(node types.TraceCausalProjectionNode) bool {
	if !node.IsAggregateMetric() {
		return false
	}
	token := runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)
	if token == "" {
		token = runtimeTraceCausalProjectionCanonicalNode(node.Object)
	}
	switch token {
	case "supply_pressure", "cpu_pressure", "io_pressure",
		"irq_burst", "irq_activity", "ipi_activity", "cpu_frequency_limit":
		return true
	}
	return false
}

// runtimeTraceProjCrossThreadAggregateSuffix renders the CMP-3 unit annotation
// for a cross-thread aggregate value: the "(跨线程累计,非墙钟)" label plus,
// when a precise window is known, the CMP-9 normalized density (value/window
// length — for supply/CPU pressure that ratio IS the average run-queue depth,
// the only cross-window-comparable reading of the aggregate). The window is
// the node's own span when valid, else the projection window in window mode.
// Display-only; exact division, no estimation.
func runtimeTraceProjCrossThreadAggregateSuffix(node types.TraceCausalProjectionNode, denom float64, windowMode, zh bool) string {
	suffix := "(跨线程累计,非墙钟)"
	if !zh {
		suffix = " (cross-thread cumulative, not wall clock)"
	}
	windowMS := runtimeTraceProjCrossThreadDensityWindowMS(node, denom, windowMode)
	impact := runtimeTraceProjNodeDisplayImpact(node)
	if windowMS <= 0 || impact <= 0 {
		return suffix
	}
	density := impact / windowMS
	queueDepth := runtimeTraceProjCrossThreadQueueDepthToken(node)
	switch {
	case queueDepth && zh:
		suffix += fmt.Sprintf("·≈平均排队深度 %.1f", density)
	case queueDepth:
		suffix += fmt.Sprintf(" ≈avg queue depth %.1f", density)
	case zh:
		suffix += fmt.Sprintf("·≈均值 %.1f", density)
	default:
		suffix += fmt.Sprintf(" ≈mean %.1f", density)
	}
	return suffix
}

// runtimeTraceProjCrossThreadDensityWindowMS is the shared CMP-9 normalization
// denominator: the node's own precise span when valid, else the projection
// window in window mode, else 0 (no density — never an estimate). Shared by
// the stanza suffix above and the F3 compare-overview cell so both surfaces
// normalize over the SAME window.
func runtimeTraceProjCrossThreadDensityWindowMS(node types.TraceCausalProjectionNode, denom float64, windowMode bool) float64 {
	if node.StartTs > 0 && node.EndTs > node.StartTs {
		return (node.EndTs - node.StartTs) * 1000
	}
	if windowMode && denom > 0 {
		return denom
	}
	return 0
}

// runtimeTraceProjCrossThreadQueueDepthToken reports whether the node's typed
// kind token (TypeToken first, Object cause lane second — same precedence as
// the classifier) is a run-queue-depth pressure metric, forking the density
// wording between ≈平均排队深度 and the neutral ≈均值.
func runtimeTraceProjCrossThreadQueueDepthToken(node types.TraceCausalProjectionNode) bool {
	switch runtimeTraceCausalProjectionCanonicalNode(firstNonEmptyAnswerString(node.TypeToken, node.Object)) {
	case "supply_pressure", "cpu_pressure":
		return true
	}
	return false
}

func runtimeTraceProjNodeDisplayImpact(node types.TraceCausalProjectionNode) float64 {
	if node.ImpactMS > 0 {
		return node.ImpactMS
	}
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return node.ActualImpactMS
}

func runtimeTraceProjModelMaxImpact(model runtimeTraceProjTreeModel) float64 {
	// CMP-3: the bar full-scale anchors WALL-CLOCK values only — a cross-thread
	// cumulative aggregate (supply_pressure 101084.884ms in a 2.1s window) once
	// became the fallback scale and crushed every real 807ms row to one cell.
	max := 0.0
	fallback := 0.0
	consider := func(rows []runtimeTraceProjTreeRow) {
		for _, row := range rows {
			v := runtimeTraceProjNodeDisplayImpact(row.Node)
			if v > fallback {
				fallback = v
			}
			if runtimeTraceProjCrossThreadAggregateType(row.Node) {
				continue
			}
			if v > max {
				max = v
			}
		}
	}
	consider(model.TreeRows)
	consider(model.SelfRows)
	consider(model.Adjacent)
	consider(model.Background)
	if max <= 0 {
		// Fail-open: a batch made ONLY of cross-thread aggregates has no
		// wall-clock anchor at all; keep the batch max so the scale note never
		// claims a 0.000ms full bar (the aggregate rows draw no bars either way).
		return fallback
	}
	return max
}

func runtimeTraceProjEvidenceTag(node types.TraceCausalProjectionNode, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) string {
	id := evidence.add(node, zh)
	if id == "" {
		return ""
	}
	if n := len(node.MergedEvidenceIDs); n > 0 {
		return fmt.Sprintf("%s(+%d)", id, n)
	}
	return id
}

// --- user-focus comparison (R2) -------------------------------------------------

// runtimeTraceProjUserFocus carries the typed analyzer entity context the
// projection renderer may compare the 🎯 root against (R2, customer audit
// 2026-07-03 C4a). Entities is AnalyzerHints.Entities ∪ ExactTargets verbatim —
// never RawRequest, never model prose. An empty list means the comparison
// cannot run and every consumer fails open to legacy behavior.
type runtimeTraceProjUserFocus struct {
	Entities []string
}

// runtimeTraceProjApplyUserFocus runs the precise root-vs-entity comparison and
// the display-only user-window derivation on a built model. No typed entity
// context → the model keeps its zero values (legacy label, no relation line).
func runtimeTraceProjApplyUserFocus(model *runtimeTraceProjTreeModel, focus runtimeTraceProjUserFocus) {
	if model == nil || len(focus.Entities) == 0 {
		return
	}
	if start, end, ok := runtimeTraceProjUserWindowFromEntities(focus.Entities); ok {
		model.UserWindowStart, model.UserWindowEnd = start, end
	}
	target := strings.TrimSpace(model.Target)
	if target == "" {
		// RN-13(a) (§7.9): flat-fallback lane — no 🎯 root, so the comparison
		// runs against the RN-3 lead row's subject (the model's own typed
		// analysis anchor). Every guard fails open to the byte-identical
		// legacy flat header: no lead, an unknown-thread sentinel anchor, a
		// matching anchor, or no thread/pid-shaped user entity.
		anchor := strings.TrimSpace(runtimeTraceProjFlatAnchorSubject(*model))
		if anchor == "" || runtimeTraceCausalProjectionUnknownSentinel(anchor) {
			return
		}
		if runtimeTraceProjTargetMatchesUserEntities(anchor, focus.Entities) {
			return
		}
		entities := runtimeTraceProjThreadOrPidEntities(focus.Entities)
		if len(entities) == 0 {
			return
		}
		model.FlatAnchorMismatch = true
		model.FlatAnchorThread = anchor
		model.RootFocusUserEntities = entities
		return
	}
	if runtimeTraceProjTargetMatchesUserEntities(target, focus.Entities) {
		return // 🎯 root really is a user-named thread — keep ‹用户关注线程›
	}
	model.RootFocusAnchorOnly = true
	model.RootFocusUserEntities = runtimeTraceProjThreadOrPidEntities(focus.Entities)
}

// runtimeTraceProjFlatAnchorSubject resolves the flat render's analysis-anchor
// thread: the subject of the row the conclusion line consumes (model.LeadKey,
// pinned at model build via the single runtimeTraceProjLeadSelect surface).
// "" when no lead exists or its row is not on the rendered tree — callers
// fail open.
func runtimeTraceProjFlatAnchorSubject(model runtimeTraceProjTreeModel) string {
	key := strings.TrimSpace(model.LeadKey)
	if key == "" {
		return ""
	}
	for _, row := range model.TreeRows {
		if runtimeTraceCausalProjectionNodeKey(row.Node) == key {
			return strings.TrimSpace(row.Node.Subject)
		}
	}
	return ""
}

// runtimeTraceProjTargetMatchesUserEntities decides whether the 🎯 root names a
// user entity via PRECISE signals only (架构红线: hard-ish display switches read
// integer equality / verbatim equality, never fuzzy containment):
//   - whole target verbatim equal to an entity;
//   - the target's name part (before the trailing -pid) verbatim equal;
//   - the target's pid integer (trailing -pid tail, or the bare "pid=N" handle
//     traceThreadLabel emits when the Comm was never resolved — every
//     wakeup-path label passes through it; RF1b, adversarial review
//     2026-07-03) equal to a pure-digit or "pid=N"-shaped entity's integer
//     (RF1a).
func runtimeTraceProjTargetMatchesUserEntities(target string, entities []string) bool {
	name, pid, hasPid := runtimeTraceProjSplitNamePid(target)
	if !hasPid {
		// RF1b target side: a bare pid handle has no name part to compare.
		pid, hasPid = runtimeTraceProjPidHandleForm(target)
	}
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		if entity == target {
			return true
		}
		if hasPid && name != "" && entity == name {
			return true
		}
		if hasPid {
			if n, ok := runtimeTraceProjPureInt(entity); ok && n == pid {
				return true
			}
			// RF1a entity side: the analyzer may hand the focus as "pid=N".
			if n, ok := runtimeTraceProjPidHandleForm(entity); ok && n == pid {
				return true
			}
		}
	}
	return false
}

// runtimeTraceProjPidHandleForm matches the literal "pid=N" thread handle
// (character-class check: the fixed prefix plus pure digits — "pidx=42591"
// and "pid=42591abc" both fail). Local isomorph of the unexported types-layer
// traceCausalProjectionPidPeerForm
// (internal/types/trace_causal_projection_aggregate.go) — keep the two
// character-class checks aligned.
func runtimeTraceProjPidHandleForm(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "pid=") {
		return 0, false
	}
	return runtimeTraceProjPureInt(strings.TrimPrefix(s, "pid="))
}

// runtimeTraceProjSplitNamePid splits the canonical thread label form name-pid
// (character-class validation: non-empty name, pure-digit tail after the last
// '-'). ok=false when the label does not carry a pid tail.
func runtimeTraceProjSplitNamePid(label string) (string, int, bool) {
	idx := strings.LastIndex(label, "-")
	if idx <= 0 || idx == len(label)-1 {
		return "", 0, false
	}
	pid, ok := runtimeTraceProjPureInt(label[idx+1:])
	if !ok {
		return "", 0, false
	}
	return label[:idx], pid, true
}

// runtimeTraceProjPureInt parses a pure-digit string (character-class check —
// display/comparison lane only, exempted from the no-keyword-matching rule).
func runtimeTraceProjPureInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// runtimeTraceProjThreadOrPidEntities selects the user entities that LOOK like
// a thread/pid handle (pure-digit pid, the "pid=N" handle form — RF1,
// adversarial review 2026-07-03 — or the name-pid label form) for the
// anchor-only note's "用户关注: …" roster. Display-only; capped at 2.
func runtimeTraceProjThreadOrPidEntities(entities []string) []string {
	var out []string
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		if _, ok := runtimeTraceProjPureInt(entity); ok {
			out = append(out, entity)
		} else if _, ok := runtimeTraceProjPidHandleForm(entity); ok {
			out = append(out, entity)
		} else if _, _, ok := runtimeTraceProjSplitNamePid(entity); ok {
			out = append(out, entity)
		}
		if len(out) == 2 {
			break
		}
	}
	return out
}

// runtimeTraceProjUserWindowFromEntities derives the user-requested window from
// the typed entity set: EXACTLY two timestamp-shaped entities (character-class:
// digits '.' digits, optional trailing "s") with start < end. Anything else —
// zero, one, three or more stamps, or a non-increasing pair — is ambiguous and
// yields nothing. Display-only lane (R2 双窗关系行).
func runtimeTraceProjUserWindowFromEntities(entities []string) (float64, float64, bool) {
	var stamps []float64
	for _, entity := range entities {
		v, ok := runtimeTraceProjTimestampShaped(strings.TrimSpace(entity))
		if !ok {
			continue
		}
		stamps = append(stamps, v)
		if len(stamps) > 2 {
			return 0, 0, false
		}
	}
	if len(stamps) != 2 || stamps[0] <= 0 || stamps[1] <= stamps[0] {
		return 0, 0, false
	}
	return stamps[0], stamps[1], true
}

// runtimeTraceProjTimestampShaped validates the timestamp character class:
// digits '.' digits with an optional trailing seconds unit "s".
func runtimeTraceProjTimestampShaped(s string) (float64, bool) {
	s = strings.TrimSuffix(s, "s")
	dot := strings.IndexByte(s, '.')
	if dot <= 0 || dot == len(s)-1 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if i == dot {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// --- tree fence rendering -----------------------------------------------------

func runtimeTraceProjTreeFence(model runtimeTraceProjTreeModel, zh bool) string {
	if len(model.TreeRows) == 0 && len(model.SelfRows) == 0 &&
		len(model.Adjacent) == 0 && len(model.Background) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```text\n")
	windowMode := model.WindowMS > 0
	denom := model.WindowMS
	if !windowMode {
		denom = model.BarMaxMS
	}
	width := runtimeTraceProjTreeLabelColumn(model, zh)
	// Header: target anchor + explicit bar-scale declaration. The root label is
	// honest about provenance (R2, customer audit 2026-07-03 C4a): ‹用户关注线程›
	// only when the typed analyzer entity comparison matched (or never ran —
	// fail-open keeps the legacy label); a mismatch renders ‹分析锚点线程› plus a
	// quiet one-line note naming the user's actual focus entities.
	if strings.TrimSpace(model.Target) != "" {
		model.Marks.mark(runtimeTraceProjMarkRootTarget)
		header := runtimeTraceProjTreeHeaderLabel(model, zh)
		// F-3: the 🎯 anchor NEVER truncates — a header wider than the capped
		// column renders unpadded (pad-only, the pre-cap PadDisplay could
		// truncate it); the NEW-10 wrap below still moves the scale note off
		// an over-long header line.
		headerCell := header
		if pad := width - runewidth.StringWidth(header); pad > 0 {
			headerCell += strings.Repeat(" ", pad)
		}
		headerLine := headerCell + " " + runtimeTraceProjScaleNote(model, zh)
		if runewidth.StringWidth(headerLine) > runtimeTraceProjTreeRowMaxWidth {
			// NEW-10 (§7.6): a long scale note (fallback-scale wording) would
			// push the header past the row cap — wrap it to its own line with
			// both facts intact instead of truncating either.
			b.WriteString(header + "\n")
			b.WriteString(runtimeTraceProjScaleNote(model, zh) + "\n")
		} else {
			b.WriteString(headerLine)
			b.WriteString("\n")
		}
		if model.RootFocusAnchorOnly && len(model.RootFocusUserEntities) > 0 {
			if zh {
				b.WriteString("- 根为唤醒链锚点线程,非用户指定关注对象(用户关注: " + strings.Join(model.RootFocusUserEntities, "、") + ")\n")
			} else {
				b.WriteString("- root is the wakeup-chain anchor thread, not the user-specified focus (user focus: " + strings.Join(model.RootFocusUserEntities, ", ") + ")\n")
			}
		}
	} else {
		flatLine := runtimeTraceProjFlatFallbackHeader(model, zh) + "  " + runtimeTraceProjScaleNote(model, zh)
		if runewidth.StringWidth(flatLine) > runtimeTraceProjTreeRowMaxWidth {
			// NEW-10: same wrap as the 🎯 branch — the scale note takes its own
			// line rather than pushing the flat-fallback reason past the cap.
			b.WriteString(runtimeTraceProjFlatFallbackHeader(model, zh) + "\n")
			b.WriteString(runtimeTraceProjScaleNote(model, zh) + "\n")
		} else {
			b.WriteString(flatLine + "\n")
		}
		// RN-13(a) (§7.9): the flat header explains the analysis anchor when it
		// is NOT the user's focused thread — the customer's flat runnable render
		// anchored on an FFRT worker while the user asked about pid 6565, with
		// no disclosure anywhere. Typed lane only (FlatAnchorMismatch set by
		// runtimeTraceProjApplyUserFocus); matched anchor / no typed entities
		// keep the header byte-identical. The anchor name flows through the
		// RN-4-aware display helper (comm-truncation placeholders never leak).
		if model.FlatAnchorMismatch && strings.TrimSpace(model.FlatAnchorThread) != "" && len(model.RootFocusUserEntities) > 0 {
			anchor := runtimeTraceCausalProjectionDisplayNodeName(model.FlatAnchorThread, zh)
			if zh {
				b.WriteString("- 分析锚=" + anchor + "(非用户关注对象;用户关注 " + strings.Join(model.RootFocusUserEntities, "、") + " 的唤醒链未在本轮查询)\n")
			} else {
				b.WriteString("- analysis anchor = " + anchor + " (not the user-specified focus; the wakeup chain for " + strings.Join(model.RootFocusUserEntities, ", ") + " was not queried this round)\n")
			}
		}
	}
	for _, row := range model.SelfRows {
		row.marks = model.Marks // NEW-7: record at the emission site of this pass
		for _, line := range runtimeTraceProjSelfRowLines(row, zh) {
			b.WriteString(line + "\n")
		}
	}
	if len(model.TreeRows) > 0 && strings.TrimSpace(model.Target) != "" {
		b.WriteString("│\n")
	}
	for _, row := range model.TreeRows {
		row.marks = model.Marks
		b.WriteString(runtimeTraceProjTreeRowLine(row, width, denom, windowMode, zh))
		b.WriteString("\n")
	}
	if len(model.Adjacent) > 0 {
		model.Marks.mark(runtimeTraceProjMarkAdjacentStanza)
		b.WriteString("\n")
		// PTV4 T4: the stanza semantics live in the legend's ◇ entry — the
		// header keeps only the mark + name.
		if zh {
			b.WriteString("◇ 邻近链\n")
		} else {
			b.WriteString("◇ Adjacent\n")
		}
		for _, row := range model.Adjacent {
			row.marks = model.Marks
			b.WriteString(runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	if len(model.Background) > 0 {
		model.Marks.mark(runtimeTraceProjMarkBackgroundStanza)
		b.WriteString("\n")
		// PTV4 T4: the stanza semantics live in the legend's ▒ entry — the
		// header keeps only the mark + name.
		if zh {
			b.WriteString("▒ 背景压力\n")
		} else {
			b.WriteString("▒ Background pressure\n")
		}
		for _, row := range model.Background {
			row.marks = model.Marks
			b.WriteString(runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	b.WriteString("```")
	return b.String()
}

// runtimeTraceProjTreeLabelColumn returns the shared label-column width for a
// fence render (§7.30.2 C4b B1 + NEW-10 §7.6): the MAX actual label need of
// this fence — the 🎯 header, every bar row and every stanza row — no longer
// floored at the 56-cell base. NEW-10 rationale: the flat 56-cell pad taxed
// every narrow fence ~15-25 blank cells per row and pushed the tag lane past
// the 100-cell row cap; shrink-to-fit returns that budget to the Keep tags.
// All bar rows still pad to this ONE column, so bars keep a single aligned
// start column at every level, and the 56-cell base survives unchanged as the
// per-row NAME budget (B1 name-scoped truncation + 20-cell floor). Rows
// without metrics (omitted markers, bare transit nodes — NEW-10 renders those
// compact/unpadded) never widen the column — they carry no bar to align.
func runtimeTraceProjTreeLabelColumn(model runtimeTraceProjTreeModel, zh bool) int {
	width := 0
	if strings.TrimSpace(model.Target) != "" {
		// The header pads to the column — include its own width so a fitting
		// 🎯 anchor lines the bars up under the scale note. F-3: the header no
		// longer sets a column ABOVE the cap; past it the header renders
		// unpadded (never truncated) and rows keep the capped column.
		width = runewidth.StringWidth(runtimeTraceProjTreeHeaderLabel(model, zh))
	}
	consider := func(fixed, name string) {
		fixedW := runewidth.StringWidth(fixed)
		budget := runtimeTraceProjTreeNameBudget(fixedW)
		nameW := runewidth.StringWidth(name)
		if nameW > budget {
			nameW = budget
		}
		if need := fixedW + nameW; need > width {
			width = need
		}
	}
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
			continue
		}
		fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
		consider(fixed, name)
	}
	// Stanza rows pad to the same column (their bars share the start column);
	// their fixed part mirrors runtimeTraceProjStanzaRowLine exactly (nil marks:
	// the width pass records nothing).
	for _, rows := range [][]runtimeTraceProjTreeRow{model.Adjacent, model.Background} {
		for _, row := range rows {
			consider("    "+runtimeTraceProjStateIcon(row.Node, row.Kind, true, nil)+" ", runtimeTraceProjRowName(row, zh))
		}
	}
	// F-3: the shared column never exceeds the cap — an over-wide 🎯 header
	// (already measured into width above) renders unpadded instead of lifting
	// every row past the row budget.
	if width > runtimeTraceProjTreeLabelColumnMax {
		width = runtimeTraceProjTreeLabelColumnMax
	}
	return width
}

// runtimeTraceProjTreeNameBudget is the SINGLE definition of a row's NAME
// display budget given its fixed-part width (F-3: the width pass and the label
// composer both call it — two hand-synced copies were the drift risk): the
// 44-cell base minus the fixed part, floored at the 20-cell readability
// minimum (B1), then capped so fixed + name can never exceed the shared-column
// cap — past the cap the name truncates further (B1 semantics, detail table
// lossless) instead of the floor lifting the whole fence over the row budget.
func runtimeTraceProjTreeNameBudget(fixedW int) int {
	budget := runtimeTraceProjTreeLabelWidth - fixedW
	if budget < runtimeTraceProjTreeNameMinWidth {
		budget = runtimeTraceProjTreeNameMinWidth
	}
	if maxName := runtimeTraceProjTreeLabelColumnMax - fixedW; budget > maxName {
		budget = maxName
	}
	if budget < 1 {
		budget = 1 // unreachable via the trunk cap; keeps the truncation sane
	}
	return budget
}

// runtimeTraceProjTreeHeaderLabel composes the 🎯 root header label (target +
// provenance chip, §7.30 C4a R2). Shared by the fence render and the width
// pass so the column measurement can never drift from the emitted header.
func runtimeTraceProjTreeHeaderLabel(model runtimeTraceProjTreeModel, zh bool) string {
	header := "🎯 " + model.Target
	switch {
	case model.RootFocusAnchorOnly && zh:
		header += " ‹分析锚点线程›"
	case model.RootFocusAnchorOnly:
		header += " <analysis anchor thread>"
	case zh:
		header += " ‹用户关注线程›"
	default:
		header += " <user-focused thread>"
	}
	return header
}

// runtimeTraceProjTreeLabelParts splits a tree row label into its fixed part
// (prefix + edge + icon + separators) and the name, so truncation can target
// the name alone instead of the composed label (B1).
func runtimeTraceProjTreeLabelParts(row runtimeTraceProjTreeRow, zh bool) (string, string) {
	edge := runtimeTraceProjEdgeLabel(row.Edge, zh)
	if row.Kind == runtimeTraceProjTreeRowDepthless && strings.TrimSpace(row.Parent) == "" {
		// Flat fallback (no resolved target): a hanging "wakes" edge word would
		// claim a wake relation with no wakee — render a bare branch instead.
		edge = ""
	}
	if edge != "" {
		// NEW-7 edge mark, recorded AFTER the flat-fallback suppression so a
		// suppressed edge never claims a legend entry. The typed switch mirrors
		// runtimeTraceProjEdgeLabel exactly (default = wake, same as its default
		// arm) — keep the two in lockstep.
		switch row.Edge {
		case runtimeTraceProjTreeEdgeDrill:
			row.marks.mark(runtimeTraceProjMarkEdgeDrill)
		case runtimeTraceProjTreeEdgeSemantic:
			row.marks.mark(runtimeTraceProjMarkSemanticSpan)
		case runtimeTraceProjTreeEdgeCause:
			row.marks.mark(runtimeTraceProjMarkEdgeCause)
		case runtimeTraceProjTreeEdgeOwn:
			row.marks.mark(runtimeTraceProjMarkEdgeOwn)
		default:
			row.marks.mark(runtimeTraceProjMarkEdgeWake)
		}
	}
	badge := ""
	if glyph := runtimeTraceProjBadgeGlyph(row.Badge); glyph != "" {
		// PTV4 T6: the TOP-N badge sits right before the state glyph. It is an
		// independent token, never a state glyph (the one-glyph invariant
		// counts state icons only).
		row.marks.mark(runtimeTraceProjMarkBadge)
		badge = glyph
	}
	fixed := runtimeTraceProjTreePrefix(row) + edge + " " + badge +
		runtimeTraceProjStateIcon(row.Node, row.Kind, row.HasData, row.marks) + " "
	return fixed, runtimeTraceProjRowName(row, zh)
}

// runtimeTraceProjTreeLabel composes a row label with name-scoped truncation
// (B1): the name budget is the base label width minus the actual fixed-part
// display width, floored at the readability minimum, then the whole label pads
// (never truncates) to the shared column width. PTV4 T2: names carrying a
// -pid identity tail truncate in the MIDDLE so the tail survives; tag
// pressure NEVER shrinks the name (the budget depends on the fixed part
// only — T1 main-row invariant).
func runtimeTraceProjTreeLabel(fixed, name string, width int) string {
	budget := runtimeTraceProjTreeNameBudget(runewidth.StringWidth(fixed))
	if runewidth.StringWidth(name) > budget {
		name = runtimeTraceProjTruncateName(name, budget)
	}
	label := fixed + name
	if pad := width - runewidth.StringWidth(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return label
}

// runtimeTraceProjTruncateName display-truncates a row name to a cell budget
// (PTV4 T2): a name with a pure-digit -pid tail keeps the tail whole and
// truncates the head in the middle ("CookieMon…-59843" — the pid tail is the
// identity-bearing segment). A composed "subject · cause" name whose SUBJECT
// carries the pid tail truncates the subject the same way (keeping the cause
// suffix when it fits, dropping it when even the subject alone is squeezed).
// Names without any pid tail (cause words, span names) keep the legacy tail
// truncation.
func runtimeTraceProjTruncateName(name string, width int) string {
	if runewidth.StringWidth(name) <= width {
		return name
	}
	if _, _, ok := runtimeTraceProjSplitNamePid(name); ok {
		return runtimeTraceProjMidTruncateKeepPid(name, width)
	}
	// Composed name: the first " · " segment may be the pid-tailed subject.
	if idx := strings.Index(name, " · "); idx > 0 {
		subject, rest := name[:idx], strings.TrimPrefix(name[idx:], " · ")
		if _, _, ok := runtimeTraceProjSplitNamePid(subject); ok {
			subjW := runewidth.StringWidth(subject)
			// The whole pid-tailed subject fits: keep it verbatim and
			// tail-truncate the cause/span suffix (T2: 无 pid 尾仍尾截断).
			if objBudget := width - subjW - 3; objBudget >= 2 {
				return subject + " · " +
					strings.TrimRight(runtimeTraceProjPadDisplay(rest, objBudget), " ")
			}
			// The cause suffix leaves no room beside the identity tail — drop
			// the suffix and keep the pid-tailed subject (身份载重段优先).
			return runtimeTraceProjMidTruncateKeepPid(subject, width)
		}
	}
	return strings.TrimRight(runtimeTraceProjPadDisplay(name, width), " ")
}

// runtimeTraceProjMidTruncateKeepPid is the T2 middle cut for a name whose
// trailing -pid segment must survive whole.
func runtimeTraceProjMidTruncateKeepPid(name string, width int) string {
	idx := strings.LastIndex(name, "-")
	if idx <= 0 {
		return strings.TrimRight(runtimeTraceProjPadDisplay(name, width), " ")
	}
	head, tail := name[:idx], name[idx:]
	headBudget := width - runewidth.StringWidth(tail) - 1
	if headBudget < 0 {
		// Degenerate budget: even "…"+tail alone would overflow — legacy tail
		// cut. (复核 off-by-one: headBudget == 0 is the exact-fit "…-59843"
		// form and stays in the pid-keeping lane.)
		return strings.TrimRight(runtimeTraceProjPadDisplay(name, width), " ")
	}
	runes := []rune(head)
	for len(runes) > 0 && runewidth.StringWidth(string(runes)) > headBudget {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…" + tail
}

// runtimeTraceProjFlatFallbackHeader names WHY the tree renders flat (§7.30
// 裁定3, two typed causes): a missing_wakeup row means the sleep interval had no
// sched_wakeup record in the selected window; the recommended-not-run flag means
// the wakeup-chain drilldown was never executed this round. Both are precise
// typed signals; the opaque "path unresolved" wording stays only as the last
// fallback.
func runtimeTraceProjFlatFallbackHeader(model runtimeTraceProjTreeModel, zh bool) string {
	switch {
	case model.missingWakeup():
		if zh {
			return "(睡眠区间在所选窗口内无 sched_wakeup 记录,唤醒链无法上溯——按层级平铺展示)"
		}
		return "(the sleep interval has no sched_wakeup record in the selected window — the wakeup chain cannot be traced upstream; layers rendered flat)"
	case model.WakeupChainRecommendedNotRun:
		if zh {
			return "(本轮未执行唤醒链下钻,建议 trace_query view=wakeup_chain——按层级平铺展示)"
		}
		return "(wakeup-chain drilldown was not run this round — recommend trace_query view=wakeup_chain; layers rendered flat)"
	default:
		if zh {
			return "(唤醒链路径未解析——按层级平铺展示)"
		}
		return "(wakeup path unresolved — layers rendered flat)"
	}
}

func runtimeTraceProjScaleNote(model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS > 0 {
		if zh {
			return fmt.Sprintf("满格=窗口%.3fms", model.WindowMS)
		}
		return fmt.Sprintf("bar full = window %.3fms", model.WindowMS)
	}
	if zh {
		return fmt.Sprintf("窗口起止未采集·满格=本批最大%.3fms(回退尺度,不显示占窗%%)", model.BarMaxMS)
	}
	return fmt.Sprintf("window bounds not captured; bar full = batch max %.3fms (fallback scale, no window %%)", model.BarMaxMS)
}

// runtimeTraceProjSelfRowLines renders one self row for the fence. PTV4 T1:
// the whole row stays a single line whenever it holds the 100-cell row cap;
// over the cap, the main line keeps only the essentials (state glyph +
// state/name + value + ⊘ marker + [E#]) and every other part moves to
// "· "-prefixed subordinate detail lines — nothing is elided (the FitTags
// drop lane is retired).
func runtimeTraceProjSelfRowLines(row runtimeTraceProjTreeRow, zh bool) []string {
	const lead = "│     "
	main, demoted := runtimeTraceProjSelfRowParts(row, zh)
	// legacy layout order for the single-line form: essentials interleave with
	// the detail parts exactly where they were built; the E# ref stays last.
	single := lead + strings.Join(runtimeTraceProjSelfInlineOrder(main, demoted), " ")
	if len(demoted) == 0 || runewidth.StringWidth(single) <= runtimeTraceProjTreeRowMaxWidth {
		return []string{single}
	}
	lines := []string{lead + strings.Join(main, " ")}
	for _, part := range demoted {
		lines = append(lines, runtimeTraceProjSubordinateLines(lead, part)...)
	}
	return lines
}

// runtimeTraceProjSelfInlineOrder restores the legacy inline order (detail
// parts before the trailing [E#] essential) for the fits-on-one-line form.
func runtimeTraceProjSelfInlineOrder(main, demoted []string) []string {
	if len(main) == 0 {
		return demoted
	}
	last := main[len(main)-1]
	if strings.HasPrefix(last, "[") && strings.HasSuffix(last, "]") {
		out := append(append([]string(nil), main[:len(main)-1]...), demoted...)
		return append(out, last)
	}
	return append(append([]string(nil), main...), demoted...)
}

// runtimeTraceProjSelfRowParts builds a self row's essential (main-line) parts
// and its demotable detail parts (PTV4 T1 split).
func runtimeTraceProjSelfRowParts(row runtimeTraceProjTreeRow, zh bool) ([]string, []string) {
	node := row.Node
	var main, demoted []string
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if node.IsSleepState() {
		state := strings.TrimSpace(node.StateKind)
		if state == "" {
			state = "sleep"
		}
		row.marks.mark(runtimeTraceProjMarkIconSleep)
		main = append(main, "☾ "+state)
	} else if name != "" {
		// NEW-10 (§7.6 记号区规整): every self row leads with exactly one state
		// glyph (sleep rows already carry ☾) — a constant one-glyph slot keeps
		// same-depth rows aligned on proportional web fonts.
		main = append(main, runtimeTraceProjStateIcon(node, row.Kind, true, row.marks)+" "+name)
	} else {
		main = append(main, runtimeTraceProjStateIcon(node, row.Kind, true, row.marks))
	}
	if v := runtimeTraceProjNodeDisplayImpact(node); v > 0 {
		main = append(main, fmt.Sprintf("%.3fms", v))
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels (PTV4
	// T4 ×N 三式: data inline, semantics in the legend's 口径组).
	if node.DuplicatePublications > 1 {
		row.marks.mark(runtimeTraceProjMarkMergedDedup)
		demoted = append(demoted, runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh))
	}
	if node.MergedCount > 1 {
		row.marks.mark(runtimeTraceProjMarkMergedSum)
		demoted = append(demoted, runtimeTraceProjMergedSumTagText(node))
	}
	// 裁定4 applies to the target's own status rows too (lock_001 customer
	// report, 2026-07-03); sleep rows keep their dedicated wording below.
	if !node.IsSleepState() {
		stateTag := runtimeTraceProjStateKindLabel(node, zh)
		if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
			runtimeTraceCausalProjectionInversionRow(node) {
			stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		}
		if stateTag != "" {
			row.marks.mark(runtimeTraceProjMarkStateLabel)
			demoted = append(demoted, stateTag)
		}
	}
	if node.IsSleepState() {
		if zh {
			demoted = append(demoted, "窗口内主要处于等待唤醒")
		} else {
			demoted = append(demoted, "mostly waiting for wakeup inside the window")
		}
	}
	if node.Undrillable() {
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		if zh {
			main = append(main, "⊘链止("+node.UndrillableReason+")")
		} else {
			main = append(main, "⊘chain ends ("+node.UndrillableReason+")")
		}
	}
	// NEW-3: a fold whose primary landed on the target's own state lane still
	// carries the caliber note (the peers' only display carrier).
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		demoted = append(demoted, runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh))
	}
	if row.EvidenceTag != "" {
		main = append(main, "["+row.EvidenceTag+"]")
	}
	return main, demoted
}

func runtimeTraceProjTreePrefix(row runtimeTraceProjTreeRow) string {
	var b strings.Builder
	for _, more := range row.Ancestors {
		if more {
			b.WriteString("│   ")
		} else {
			b.WriteString("    ")
		}
	}
	if row.Last {
		b.WriteString("└─")
	} else {
		b.WriteString("├─")
	}
	return b.String()
}

func runtimeTraceProjEdgeLabel(edge string, zh bool) string {
	switch edge {
	case runtimeTraceProjTreeEdgeDrill:
		if zh {
			return "下钻─"
		}
		return "drill─"
	case runtimeTraceProjTreeEdgeSemantic:
		if zh {
			return "语义─"
		}
		return "span─"
	case runtimeTraceProjTreeEdgeCause:
		if zh {
			return "成因─"
		}
		return "cause─"
	case runtimeTraceProjTreeEdgeOwn:
		// F2: own-process caliber lane — never the wake edge word.
		if zh {
			return "自身─"
		}
		return "own─"
	default:
		if zh {
			return "唤醒─"
		}
		return "wakes─"
	}
}

// runtimeTraceProjStateIcon picks the row's state icon and records the NEW-7
// mark for the icon it actually emits (marks nil-safe; the width pass and the
// detail-cell callers pass nil and record nothing). PTV4 T5: the sleep /
// runnable icons are the single-cell text glyphs ☾ / ⧖ (were the 2-cell emoji
// 💤 / ⏳). PTV4 T4: the two ◦ senses record SEPARATE marks — hasData=false is
// the chain-transit sense, a data row landing on the default arm is the
// no-dominant-state sense (its 2-word inline tag is appended by the tag
// builder, not here).
func runtimeTraceProjStateIcon(node types.TraceCausalProjectionNode, kind string, hasData bool, marks *runtimeTraceProjMarkSet) string {
	if kind == runtimeTraceProjTreeRowSemantic {
		marks.mark(runtimeTraceProjMarkSemanticSpan)
		return "✦"
	}
	if node.IsSleepState() {
		marks.mark(runtimeTraceProjMarkIconSleep)
		return "☾"
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running":
		marks.mark(runtimeTraceProjMarkIconRunning)
		return "⚙"
	case "runnable":
		marks.mark(runtimeTraceProjMarkIconRunnable)
		return "⧖"
	case "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		marks.mark(runtimeTraceProjMarkIconDState)
		return "⛓"
	default:
		if hasData {
			marks.mark(runtimeTraceProjMarkIconNoDominant)
		} else {
			marks.mark(runtimeTraceProjMarkIconTransit)
		}
		return "◦"
	}
}

// runtimeTraceProjNoDominantStateRow reports whether a DATA row renders the ◦
// icon through the default (no dominant scheduler state) arm — the T4 "◦
// 无主导态" inline word's gate. Mirrors runtimeTraceProjStateIcon's switch.
func runtimeTraceProjNoDominantStateRow(node types.TraceCausalProjectionNode, kind string) bool {
	if kind == runtimeTraceProjTreeRowSemantic || node.IsSleepState() {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running", "runnable", "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		return false
	}
	return true
}

func runtimeTraceProjRowName(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		name := strings.TrimSpace(node.SpanName)
		if name == "" {
			name = strings.TrimSpace(node.Object)
		}
		return name
	}
	if node.IsAggregateMetric() {
		// The metric IS the subject; the Object type word is already folded into
		// the semantic name (裁定2 rendering half).
		return strings.TrimSpace(runtimeTraceCausalProjectionAggregateMetricName(node, zh))
	}
	// §7.30.3 D1: lock-contention rows render their typed semantics (with the
	// parsed holder) instead of an unresolved-peer label + bare duration.
	if blocking := runtimeTraceCausalProjectionBlockingName(node, zh); blocking != "" {
		if runtimeTraceCausalProjectionKnownSubject(node.Subject) {
			return strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh)) + " · " + blocking
		}
		return blocking
	}
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	// D2: tree and cause rows show the concise zh label for recognized type
	// tokens (the raw token stays on the detail table's 类型 column).
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if row.Kind == runtimeTraceProjTreeRowCause {
		// Same-subject cause decomposition: the subject is already the parent
		// trunk row; show only the cause word.
		if object != "" {
			return object
		}
		return subject
	}
	if node.MergedCount > 1 && node.Subject == "" {
		// The fold line keeps the folded rows' thread names (customer
		// 2026-07-03: a bare "其余 N 项合并" lost every thread identity).
		if zh {
			return fmt.Sprintf("其余 %d 项合并%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
		}
		return fmt.Sprintf("%d more folded%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
	}
	switch {
	case subject != "" && object != "":
		return subject + " · " + object
	case subject != "":
		return subject
	default:
		return object
	}
}

// runtimeTraceProjDedupFoldTagText is the dedupe-exclusive ×N label (RF2b,
// adversarial review 2026-07-03): a duplicate-publication row's ms is ONE
// measurement that was published N times, so it must never share the R2
// sum-aggregate form ×N(a–b). PTV4 T4 (×N 三式): the row keeps only the data
// token — the "重复发布/数值不变" semantics live in the legend's 口径组 entry.
// Callers fork on the typed Node.DuplicatePublications count.
func runtimeTraceProjDedupFoldTagText(count int, zh bool) string {
	if zh {
		return fmt.Sprintf("×%d同值", count)
	}
	return fmt.Sprintf("×%d same-value", count)
}

// runtimeTraceProjMergedSumTagText is the R2 ×N SUM aggregate's inline data
// token (PTV4 T4 ×N 三式): count + per-instance range only; the SUM semantics
// live in the legend's 口径组 entry. Language-neutral (numbers + units).
func runtimeTraceProjMergedSumTagText(node types.TraceCausalProjectionNode) string {
	return fmt.Sprintf("×%d(%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjMergedMaxTagText is the R3 cross-thread fold's inline data
// token (PTV4 T4 ×N 三式): the 取最大/不求和 semantics live in the legend's
// 口径组 entry; the member roster stays via the name lane / detail blocks.
func runtimeTraceProjMergedMaxTagText(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("×%d(%.3f–%.3fms)取最大", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("×%d(%.3f–%.3fms) max", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjSubjectlessFoldRow identifies the R3 background fold row
// (typed identity: a merged row with no thread subject of its own — exactly the
// key the "其余 N 项合并" name lane already forks on). It is the only merged
// row family whose published value is the member MAX (V3, customer revisit
// 2026-07-03): the members are different threads, whose wall clocks never sum.
func runtimeTraceProjSubjectlessFoldRow(node types.TraceCausalProjectionNode) bool {
	return node.MergedCount > 1 && strings.TrimSpace(node.Subject) == ""
}

// runtimeTraceProjMergedSubjectsSuffix names the folded members' thread
// subjects on a merged row: "(A、B 等)" — the trailing 等/… appears only when
// MergedCount exceeds the preserved roster (the typed cap lives at the
// aggregation site). Empty when the row carries no MergedSubjects.
func runtimeTraceProjMergedSubjectsSuffix(node types.TraceCausalProjectionNode, zh bool) string {
	if len(node.MergedSubjects) == 0 {
		return ""
	}
	if zh {
		suffix := strings.Join(node.MergedSubjects, "、")
		if node.MergedCount > len(node.MergedSubjects) {
			suffix += " 等"
		}
		return "(" + suffix + ")"
	}
	suffix := strings.Join(node.MergedSubjects, ", ")
	if node.MergedCount > len(node.MergedSubjects) {
		suffix += ", …"
	}
	return " (" + suffix + ")"
}

func runtimeTraceProjBar(value, denom float64, background bool) string {
	if denom <= 0 || value <= 0 {
		return strings.Repeat("░", runtimeTraceProjTreeBarWidth)
	}
	filled := int(value/denom*float64(runtimeTraceProjTreeBarWidth) + 0.5)
	if filled < 1 {
		filled = 1
	}
	if filled > runtimeTraceProjTreeBarWidth {
		filled = runtimeTraceProjTreeBarWidth
	}
	cell := "█"
	if background {
		cell = "▒"
	}
	return strings.Repeat(cell, filled) + strings.Repeat("░", runtimeTraceProjTreeBarWidth-filled)
}

// runtimeTraceProjPadDisplay pads (or runewise truncates with …) to a display
// width so bars align in monospace surfaces; CJK/emoji measured via runewidth.
func runtimeTraceProjPadDisplay(s string, width int) string {
	w := runewidth.StringWidth(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	runes := []rune(s)
	for len(runes) > 0 && runewidth.StringWidth(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	out := string(runes) + "…"
	if pad := width - runewidth.StringWidth(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

func runtimeTraceProjTreeRowLine(row runtimeTraceProjTreeRow, width int, denom float64, windowMode, zh bool) string {
	if row.Kind == runtimeTraceProjTreeRowOmitted {
		row.marks.mark(runtimeTraceProjMarkOmitted)
		return runtimeTraceProjOmittedRowLine(row, zh)
	}
	fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
	left := runtimeTraceProjTreeLabel(fixed, name, width)
	var line string
	if !row.HasData {
		// NEW-10 (§7.6): transit rows carry no bar to align — render compact.
		// PTV4 T4: the parenthesized explanation is retired; the ◦ transit
		// sense carries the 2-word inline token, the legend's ◦ 中转 entry
		// holds the semantics.
		left = strings.TrimRight(left, " ")
		if zh {
			line = left + " 中转"
		} else {
			line = left + " transit"
		}
	} else {
		line = runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
	}
	// H11 small-cycle annotation (PTV4 T4: bare ↺ — the legend entry holds the
	// explanation). Display-only; never truncates the chain. On a multi-line
	// render the suffix belongs to the MAIN row, never a subordinate line.
	if row.RecursOnChain {
		row.marks.mark(runtimeTraceProjMarkRecursOnChain)
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i] + runtimeTraceProjRecursSuffix(zh) + line[i:]
		} else {
			line += runtimeTraceProjRecursSuffix(zh)
		}
	}
	return line
}

// runtimeTraceProjOmittedRowLine renders the long-trunk fold row (PTV4 T8):
// the omitted count PLUS the folded segment's first/last two node names
// (mid-truncated per T2 — pure display upgrade, the names were always in the
// typed path) and the cycle note when the detector fired. The former trailing
// "完整链路见原始 trace_query 记录" clause moved to the legend's 省略行 entry.
func runtimeTraceProjOmittedRowLine(row runtimeTraceProjTreeRow, zh bool) string {
	prefix := runtimeTraceProjTreePrefix(row)
	head := fmt.Sprintf("…省略%d节点", row.Omitted)
	if !zh {
		head = fmt.Sprintf("…%d nodes omitted", row.Omitted)
	}
	cycle := ""
	if row.CyclePeriod > 0 && row.CycleCount > 0 {
		if zh {
			cycle = fmt.Sprintf("(检测到%d节点循环约%d轮)", row.CyclePeriod, row.CycleCount)
		} else {
			cycle = fmt.Sprintf(" (≈%d-node cycle ×%d)", row.CyclePeriod, row.CycleCount)
		}
	}
	names := append([]string(nil), row.OmittedHead...)
	tailStart := len(names)
	names = append(names, row.OmittedTail...)
	if len(names) == 0 {
		return prefix + " " + head + "…" + cycle
	}
	roster := func(budget int) string {
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = runtimeTraceProjTruncateName(name, budget)
		}
		joined := strings.Join(parts[:tailStart], "→")
		if tailStart < len(names) {
			joined += "→…→" + strings.Join(parts[tailStart:], "→")
		}
		return joined
	}
	// Deterministic character-budget fit: shrink the per-name budget (floor 8)
	// until the row holds the 100-cell cap; the floor keeps identity readable.
	// (T2 mid-truncation keeps the pid tail whenever "…"+tail fits the budget —
	// at the 8-cell floor that covers pids up to 6 digits; wider tails fall to
	// the legacy tail cut, 复核勘误 of the former "every budget" claim.)
	for budget := 18; ; budget-- {
		line := prefix + " " + head + ": " + roster(budget) + cycle
		if runewidth.StringWidth(line) <= runtimeTraceProjTreeRowMaxWidth || budget <= 8 {
			return line
		}
	}
}

// runtimeTraceProjRecursSuffix is the H11 small-cycle end-of-row annotation
// (PTV4 T4: bare mark — the parenthesized explanation lives in the legend).
// Shared by the append site above and the width reserve in
// runtimeTraceProjRowLineWithMetrics so the two can never drift.
func runtimeTraceProjRecursSuffix(zh bool) string {
	_ = zh
	return " ↺"
}

func runtimeTraceProjStanzaRowLine(row runtimeTraceProjTreeRow, width int, denom float64, windowMode, zh bool) string {
	// NEW-10 (§7.6 记号区规整): stanza rows carry the same single state glyph
	// slot as tree rows (typed StateKind switch, ◦ default) — previously they
	// rendered glyph-less, so same-depth rows had varying emoji counts and web
	// font drift shifted them by varying offsets. The fixed part mirrors the
	// width pass in runtimeTraceProjTreeLabelColumn exactly.
	left := runtimeTraceProjTreeLabel("    "+runtimeTraceProjStateIcon(row.Node, row.Kind, true, row.marks)+" ",
		runtimeTraceProjRowName(row, zh), width)
	return runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
}

// runtimeTraceProjRowLineWithMetrics assembles label + bar/ms cells + tags
// (PTV4 T1, 按需拆行): when EVERY tag fits inline within the 100-cell row cap
// the row stays one line; otherwise the main line keeps ONLY the essentials
// (label + bar + ms + % + the MainRow marks ⚠/⊘/[E#] — never truncated, never
// moved down) and ALL remaining tags demote to "· "-prefixed subordinate
// detail lines under the row's rails (T3 boundary-aware wrap; nothing is
// elided — the FitTags DropOrder lane is retired).
func runtimeTraceProjRowLineWithMetrics(left string, row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	base, tags := runtimeTraceProjRowMetricParts(row, denom, windowMode, zh)
	if len(tags) == 0 {
		return left + " " + base
	}
	// Med-1 (PTV4 复核): ONE reserve-aware budget for EVERY main-line width
	// judgment — the H11 ↺ suffix appends after assembly, so the single-line
	// fit, the demoted main line AND both alignment-yield rechecks all compare
	// against cap − reserve (HEAD's budget-=reserve discipline restored; two
	// diverging comparison lanes produced 101-102-cell demote-form rows).
	mainBudget := runtimeTraceProjTreeRowMaxWidth
	if row.RecursOnChain {
		mainBudget -= runewidth.StringWidth(runtimeTraceProjRecursSuffix(zh))
	}
	texts := make([]string, 0, len(tags))
	for _, tag := range tags {
		texts = append(texts, tag.Text)
	}
	single := left + " " + base + "  " + strings.Join(texts, " · ")
	if runewidth.StringWidth(single) <= mainBudget {
		return single
	}
	var mainTexts, demoted []string
	for _, tag := range tags {
		if tag.MainRow {
			mainTexts = append(mainTexts, tag.Text)
		} else {
			demoted = append(demoted, tag.Text)
		}
	}
	line := left + " " + base
	if len(mainTexts) > 0 {
		line += "  " + strings.Join(mainTexts, " · ")
		// The essentials themselves must hold the (reserve-aware) cap: yield
		// the label's trailing alignment padding first, then the double-space
		// tag gap + the base's ms-column padding (content — name, bar, ms,
		// Keep marks — never shrinks; if the irreducible essentials still
		// overflow, they render whole: the T1 integrity floor outranks the cap
		// only then — see the quantified floor note below).
		if runewidth.StringWidth(line) > mainBudget {
			trimmed := strings.TrimRight(left, " ") + " " + base + "  " + strings.Join(mainTexts, " · ")
			if runewidth.StringWidth(trimmed) > mainBudget {
				// Final alignment yield: collapse the base's ms-column padding
				// runs (space runs only — every glyph and digit survives).
				//
				// Quantified essentials floor (PTV4 复核实测, parity-or-better
				// vs the pre-batch floor — the old lane also kept ⚠/⛔ stubs +
				// E# over the cap on these rows): with the COMMON mark pairs
				// (⚠实际Xms + [E#], or ⊘链止 + [E#]) every depth measures
				// ≤ 99 cells (zh ≤ 97 / en ≤ 99 at ancestors 2-7). Only the
				// rare TRIPLE coincidence — one row that is cross-window AND
				// undrillable AND evidence-tagged — still overflows: measured
				// zh 103 / en 112 at ancestors=3, plateauing at zh 109 /
				// en 118 from ancestors ≥ 5 (the label column caps at 50, so
				// deeper rows stop growing). Recorded as-is; the T1 integrity
				// floor keeps those marks whole rather than truncating them.
				squeezed := strings.Join(strings.Fields(base), " ")
				trimmed = strings.TrimRight(left, " ") + " " + squeezed + " " + strings.Join(mainTexts, " · ")
			}
			line = trimmed
		}
	}
	indent := runtimeTraceProjRowContinuationIndent(row)
	for _, text := range demoted {
		for _, cont := range runtimeTraceProjSubordinateLines(indent, text) {
			line += "\n" + cont
		}
	}
	return line
}

// runtimeTraceProjRowContinuationIndent derives the rail indent a subordinate
// detail line renders under (F-2 rails, PTV4 T1): the row's ancestor rails
// verbatim, then the row's own rail (│ while siblings follow, blank after a
// └─ row) so the tree drawing stays intact under the demoted tags. Stanza
// rows (◇/▒ families, no rails) indent by their fixed 4-cell lead + icon
// slot; self rows pass their own lead explicitly.
func runtimeTraceProjRowContinuationIndent(row runtimeTraceProjTreeRow) string {
	switch row.Kind {
	case runtimeTraceProjTreeRowAdjacent, runtimeTraceProjTreeRowBackground:
		return "      "
	}
	var b strings.Builder
	for _, more := range row.Ancestors {
		if more {
			b.WriteString("│   ")
		} else {
			b.WriteString("    ")
		}
	}
	if row.Last {
		b.WriteString("  ")
	} else {
		b.WriteString("│ ")
	}
	return b.String()
}

// runtimeTraceProjSubordinateLines renders ONE demoted tag as "· "-prefixed
// subordinate detail line(s) under the row's rails (PTV4 T1): the first line
// carries the "· " marker, wrapped continuations align under the text. Every
// line holds the 100-cell cap; chunk concatenation loses nothing (T3 wrap,
// never truncation).
func runtimeTraceProjSubordinateLines(indent, text string) []string {
	lead := indent + "· "
	width := runtimeTraceProjTreeRowMaxWidth - runewidth.StringWidth(lead)
	if width < runtimeTraceProjTreeNameMinWidth {
		width = runtimeTraceProjTreeNameMinWidth
	}
	chunks := runtimeTraceProjWrapDisplay(text, width)
	out := make([]string, 0, len(chunks))
	prefix := lead
	for _, chunk := range chunks {
		out = append(out, prefix+chunk)
		prefix = indent + "  "
	}
	return out
}

// runtimeTraceProjWrapDisplay splits text into display chunks of at most
// width cells, breaking ONLY at atom boundaries (PTV4 T3): an atom is a
// maximal run of ASCII non-space, non-`·` runes — tokens like "14.597ms"
// never split — or a single non-ASCII rune (CJK wraps naturally); spaces and
// `·` separators are break opportunities. Chunk concatenation is
// BYTE-IDENTICAL to the input (a break space stays at the end of its chunk —
// wrap only, never loss). An atom wider than the whole width owns its own
// line(s) and hard-splits only then (unavoidable, deterministic).
func runtimeTraceProjWrapDisplay(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	type atom struct {
		text string
		w    int
	}
	var atoms []atom
	runes := []rune(text)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == ' ':
			atoms = append(atoms, atom{text: " ", w: 1})
			i++
		case r < 0x80 && r != '·':
			j := i
			for j < len(runes) && runes[j] < 0x80 && runes[j] != ' ' && runes[j] != '·' {
				j++
			}
			s := string(runes[i:j])
			atoms = append(atoms, atom{text: s, w: runewidth.StringWidth(s)})
			i = j
		default:
			s := string(r)
			atoms = append(atoms, atom{text: s, w: runewidth.StringWidth(s)})
			i++
		}
	}
	var out []string
	var line strings.Builder
	lineW := 0
	flush := func() {
		if line.Len() > 0 {
			out = append(out, line.String())
		}
		line.Reset()
		lineW = 0
	}
	for _, a := range atoms {
		if lineW+a.w > width && lineW > 0 {
			flush()
		}
		if a.w > width {
			// Over-wide single atom: it owns its line(s); hard-split by runes
			// only here (no boundary exists inside it that fits).
			flush()
			part := []rune(a.text)
			for len(part) > 0 {
				w, i := 0, 0
				for i < len(part) {
					rw := runewidth.RuneWidth(part[i])
					if i > 0 && w+rw > width {
						break
					}
					w += rw
					i++
				}
				out = append(out, string(part[:i]))
				part = part[i:]
			}
			continue
		}
		line.WriteString(a.text)
		lineW += a.w
	}
	flush()
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// runtimeTraceProjTag is one tag cell of a tree/stanza row (PTV4 T1). The
// former DropOrder/NoTruncate/MinKeep elision machinery is RETIRED — no tag
// is ever elided or shaved; a row that cannot hold all tags inline demotes
// every non-MainRow tag to subordinate "· " detail lines instead.
type runtimeTraceProjTag struct {
	Text string
	// MainRow marks the T1 Keep 记号 that never leave the main line and never
	// move down: the ⚠实际Xms cross-window mark, the ⊘链止 mark and the [E#]
	// evidence reference. Set at the build site only.
	MainRow bool
}

// runtimeTraceProjStateKindLabel is the bar-row state attribution (§7.30
// 裁定4): a localized label for the node's typed dominant scheduler state.
// Empty when the node exposes no StateKind — callers then fall back to the
// impact-shape cell value instead of fabricating a state.
func runtimeTraceProjStateKindLabel(node types.TraceCausalProjectionNode, zh bool) string {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait":
		if zh {
			return "睡眠等待"
		}
		return "sleep wait"
	case "runnable":
		if zh {
			return "可运行等待"
		}
		return "runnable wait"
	case "running":
		if zh {
			return "运行占用"
		}
		return "running"
	case "io_wait":
		if zh {
			return "IO阻塞"
		}
		return "IO wait"
	case "d_state", "d_sleep", "uninterruptible_sleep":
		if zh {
			return "D状态"
		}
		return "D-state"
	}
	return ""
}

// runtimeTraceProjRowMetricParts renders the fixed metric cells (bar + ms +
// window %) and returns the tag list separately so the caller can fit the tag
// segment into the remaining row-width budget (B4).
func runtimeTraceProjRowMetricParts(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) (string, []runtimeTraceProjTag) {
	node := row.Node
	impact := runtimeTraceProjNodeDisplayImpact(node)
	var b strings.Builder
	crossThread := runtimeTraceProjCrossThreadAggregateType(node)
	if crossThread {
		// CMP-3: a cross-thread cumulative aggregate draws NO bar — its cpu·ms
		// value is not on the wall-clock scale the bar column encodes, so any
		// bar (full, capped or proportional) would misread as a wall-clock
		// share. Blank cells keep the column alignment; the number carries the
		// unit annotation + normalized density instead.
		b.WriteString(strings.Repeat(" ", runtimeTraceProjTreeBarWidth))
	} else {
		b.WriteString(runtimeTraceProjBar(impact, denom, row.Kind == runtimeTraceProjTreeRowBackground))
	}
	if !crossThread {
		// PTV4 T7 口径组: the bar-scale caliber legend line is gated on a bar
		// actually rendering (cross-thread aggregates draw no bar).
		row.marks.mark(runtimeTraceProjMarkBarScale)
	}
	b.WriteString(fmt.Sprintf(" %9.3fms", impact))
	if crossThread {
		b.WriteString(runtimeTraceProjCrossThreadAggregateSuffix(node, denom, windowMode, zh))
	}
	if windowMode && denom > 0 && impact > 0 && !crossThread {
		b.WriteString(fmt.Sprintf(" %3.0f%%", impact/denom*100))
		// H8: an over-window share (cross-CPU / multi-span cumulative values can
		// legitimately exceed the wall-clock window) must not run naked. The
		// *1.001 tolerance mirrors runtimeTraceProjCrossWindow (V3). PTV4 T4:
		// the inline "(跨CPU/多段累计)" explanation moved to the legend's
		// 口径组 (占窗>100% entry), gated on this typed mark — the number
		// itself is the marker.
		if impact > denom*1.001 {
			row.marks.mark(runtimeTraceProjMarkOverWindowShare)
		}
	}
	var tags []runtimeTraceProjTag
	// 裁定4: every bar row states WHAT the duration was (typed StateKind label;
	// impact-shape value when no state was exposed — never fabricated).
	// §7.30.3 D1/D3: typed lock-contention rows and gated-composite inversion
	// rows always carry their semantic label — the shape cell wins over any
	// single-state claim (an inversion composite is NOT "running").
	stateTag := runtimeTraceProjStateKindLabel(node, zh)
	if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
		runtimeTraceCausalProjectionInversionRow(node) {
		stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
	}
	// PTV4 T4 (◦ 二义拆分): a DATA row rendering the ◦ icon says which sense
	// applies — the 2-word 无主导态 token; the legend's ◦ 无主导态 entry holds
	// the semantics. Transit (no-data) rows carry 中转 on the main line.
	if row.HasData && runtimeTraceProjNoDominantStateRow(node, row.Kind) {
		word := "无主导态"
		if !zh {
			word = "no dominant state"
		}
		tags = append(tags, runtimeTraceProjTag{Text: word})
	}
	if stateTag != "" {
		row.marks.mark(runtimeTraceProjMarkStateLabel)
		tags = append(tags, runtimeTraceProjTag{Text: stateTag})
	}
	// V3 (customer revisit 2026-07-03): a background row whose projection covers
	// ≥99% of the window — without exceeding it — waited out the whole window.
	// Over-window values (H8 tolerance) are the multi-CPU cumulative shape — an
	// ACTIVE burst, never tagged idle. F3: the full judgment lives in the
	// shared helper; the detail blocks mirror the same call. PTV4 T4: the
	// "(疑似空闲)" semantics moved to the legend's 整窗等待 entry.
	if windowMode && runtimeTraceProjWholeWindowIdleRow(row, denom) {
		row.marks.mark(runtimeTraceProjMarkWholeWindowIdle)
		text := "整窗等待"
		if !zh {
			text = "whole-window wait"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// foldWindowMS feeds the VS-2 fold verdict/tag below AND the F-4
	// composition-suppression check: window mode's denom IS model.WindowMS.
	foldWindowMS := 0.0
	if windowMode {
		foldWindowMS = denom
	}
	// §7.30.3 D3: the inversion composite shows its gated composition — the
	// split is load-bearing (demotes to a subordinate line on width pressure,
	// never elided). F-4 (统一复核 2026-07-04): suppressed when this row's
	// VS-2 fold clause is the Triple branch — that clause already embeds the
	// SAME single-source composition text (H5-class inflation otherwise).
	if runtimeTraceCausalProjectionInversionRow(node) &&
		(node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0) &&
		!runtimeTraceProjSupplyFoldEmbedsInversionComposition(node, foldWindowMS) {
		// Composition wording single-sourced (RN-16 运行折算 lint lane) —
		// see runtimeTraceProjInversionCompositionText.
		text := "影响构成: " + runtimeTraceProjInversionCompositionText(node, true)
		if !zh {
			text = "composition: " + runtimeTraceProjInversionCompositionText(node, false)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// VS-1 (§7.8): a periodic signal source's in-period sleep is cadence, not
	// impact — the row keeps the DATA (period + discounted attribution) while
	// the bar/ms keep the raw window projection. PTV4 T4: the "期内睡眠为正常
	// 节拍" semantics live in the legend's 周期性信号源 entry (already
	// verbatim there); the inline tag carries only the marker + data.
	if node.PeriodicSource {
		row.marks.mark(runtimeTraceProjMarkPeriodicSource)
		period := ""
		if node.DetectedPeriodMS > 0 {
			period = fmt.Sprintf("(周期≈%.1fms)", node.DetectedPeriodMS)
			if !zh {
				period = fmt.Sprintf(" (period ≈%.1fms)", node.DetectedPeriodMS)
			}
		}
		text := fmt.Sprintf("周期性信号源%s·有效归因 %.3fms", period, node.EffectiveImpactMS)
		if !zh {
			text = fmt.Sprintf("periodic signal source%s · attribution %.3fms", period, node.EffectiveImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// VS-2 (§7.10): a folded running-dominant row states its mechanism
	// composition inline (Keep + ContinuationLane; single-source clause —
	// see answer_document_mutation_runtime_supplyfold.go). The affirmative
	// no-deficit branch and the honest "频点数据不全" branch render too:
	// exclusion is information. (foldWindowMS hoisted above the D3
	// composition tag for the F-4 suppression check.)
	if tag, ok := runtimeTraceProjSupplyFoldTag(node, foldWindowMS, zh); ok {
		tags = append(tags, tag)
	}
	// RN-1 (§7.9, RN-B lane): a runnable row with a compiled same-window
	// occupier roster says WHO held the CPU inline (helper appended at file
	// end; Keep + ContinuationLane — the occupier names/values have no other
	// fence carrier).
	if tag, ok := runtimeTraceProjOccupierTag(node, zh); ok {
		tags = append(tags, tag)
	}
	// RN-12 (§7.9, RN-C lane): a chain/flat row whose subject has a same-ledger
	// full-window total for the same state class states how much of that total
	// the chain actually covers — without it the customer read the 635.981ms
	// top fragment as "the tree is truncated" while the full-window runnable
	// total (2528.721ms) sat uncross-referenced in the same ledger. Chain-lane
	// rows only (the compile side attaches to the chain universe; the stanza
	// kinds are excluded here as the pinned display gate).
	if row.Kind != runtimeTraceProjTreeRowAdjacent && row.Kind != runtimeTraceProjTreeRowBackground {
		if tag, ok := runtimeTraceProjFullWindowCoverageTag(node, zh); ok {
			tags = append(tags, tag)
		}
	}
	// §7.30.3 D1: the parsed holder site is auditable detail; the raw record
	// keeps it lossless.
	if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
		text := "持有点 " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		if !zh {
			text = "held at " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// PTV4 T9 (‹链上L#› 三路分流): the former ‹layer›priority chip is retired
	// from the tree — the depth VALUE stays as a compact chip (subordinate
	// line on width pressure), the 关注 semantics moved to the T6 ❶❷❸ badges,
	// and the detail blocks keep the full 因果位置·优先级 cell. Chain-lane
	// rows with a resolved depth only; flat renders never claim 链上 (CMP-7a).
	if !row.FlatChain && row.Depth > 0 &&
		(row.Kind == runtimeTraceProjTreeRowChain || row.Kind == runtimeTraceProjTreeRowDepthless) {
		row.marks.mark(runtimeTraceProjMarkChainDepthChip)
		text := fmt.Sprintf("链上L%d", row.Depth)
		if !zh {
			text = fmt.Sprintf("chain L%d", row.Depth)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	if action := runtimeTraceCausalProjectionActionCell(node, zh); action != "" &&
		row.Kind != runtimeTraceProjTreeRowBackground {
		tags = append(tags, runtimeTraceProjTag{Text: action})
	}
	if node.CumulativeImpactMS > 0 && impact > 0 && node.CumulativeImpactMS != impact {
		text := fmt.Sprintf("链上累计%.3fms", node.CumulativeImpactMS)
		if !zh {
			text = fmt.Sprintf("chain cum %.3fms", node.CumulativeImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels (PTV4
	// T4 ×N 三式 — data inline, semantics in the legend's 口径组).
	if node.DuplicatePublications > 1 {
		row.marks.mark(runtimeTraceProjMarkMergedDedup)
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)})
	}
	if node.MergedCount > 1 {
		var text string
		if runtimeTraceProjSubjectlessFoldRow(node) {
			// V3: the R3 cross-thread fold publishes the member MAX (取最大
			// legend entry; wall clock never sums across threads).
			row.marks.mark(runtimeTraceProjMarkMergedMax)
			text = runtimeTraceProjMergedMaxTagText(node, zh)
		} else {
			row.marks.mark(runtimeTraceProjMarkMergedSum)
			text = runtimeTraceProjMergedSumTagText(node)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	if len(node.SecondaryObjects) > 0 {
		joined := strings.Join(node.SecondaryObjects, "/")
		text := "影响点 " + joined
		if !zh {
			text = "impact point " + joined
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	if runtimeTraceProjEffectiveInherited(node) {
		// PTV4 T4: marker + data inline; the "非本行实测" semantics live in
		// the legend's 承自归因 entry.
		row.marks.mark(runtimeTraceProjMarkInheritedAttribution)
		text := fmt.Sprintf("承自归因%.3fms", node.EffectiveImpactMS)
		if !zh {
			text = fmt.Sprintf("inherited attribution %.3fms", node.EffectiveImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text})
	}
	// RN-2b (§7.9): the ⚠ marker's semantics DEPEND on a resolved projection
	// window. No window → no ⚠ (tree tag, detail mirror and — via the NEW-7
	// typed mark — the legend entry all fall silent together). PTV4 T4: the
	// inline "跨窗" explanation moved to the legend; the tag keeps marker +
	// actual value (⚠实际Xms). MainRow: a T1 Keep 记号.
	if windowMode && runtimeTraceProjCrossWindow(node) {
		row.marks.mark(runtimeTraceProjMarkCrossWindow)
		text := fmt.Sprintf("⚠实际%.3fms", node.ActualImpactMS)
		if !zh {
			text = fmt.Sprintf("⚠actual %.3fms", node.ActualImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, MainRow: true})
	}
	// NEW-3: the folded same-segment IO calibers' values and evidence tags live
	// ONLY on this note (plus the evidence index) — load-bearing, never elided;
	// demotes intact to a subordinate line on width pressure.
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh)})
	}
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if parent != "" {
			text := "span 位于 " + parent + " 内"
			if !zh {
				text = "span inside " + parent
			}
			tags = append(tags, runtimeTraceProjTag{Text: text})
		}
	}
	if node.Undrillable() {
		// PTV4 T4/T5: bare ⊘链止 — the sched_wakeup explanation lives in the
		// legend's ⊘ entry. MainRow: a T1 Keep 记号.
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		text := "⊘链止"
		if !zh {
			text = "⊘chain ends"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, MainRow: true})
	}
	if row.EvidenceTag != "" {
		// The E# locator is a T1 Keep 记号 — always on the main line, whole.
		tags = append(tags, runtimeTraceProjTag{Text: "[" + row.EvidenceTag + "]", MainRow: true})
	}
	return b.String(), tags
}

// runtimeTraceProjEffectiveInherited flags the contradictory-number shape a
// real customer render exposed: an EffectiveImpactMS an order of magnitude
// above the row's own cumulative means the attribution was inherited from the
// enclosing wait interval, and MUST be annotated instead of shown bare.
func runtimeTraceProjEffectiveInherited(node types.TraceCausalProjectionNode) bool {
	return node.EffectiveImpactMS > 0 && node.CumulativeImpactMS > 0 &&
		node.EffectiveImpactMS > 10*node.CumulativeImpactMS
}

// runtimeTraceProjCrossWindow marks a node whose underlying state extends
// beyond its in-window projection (deterministic comparison; also honors the
// typed WithinRequestedWindow=false drill marker). The baseline is the LARGER
// of per-layer projection and chain total — an actual equal to the chain total
// does not cross anything (comparing actual against the smaller per-layer value
// alone over-flags dual-scope rows).
func runtimeTraceProjCrossWindow(node types.TraceCausalProjectionNode) bool {
	if node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow {
		return true
	}
	baseline := node.ImpactMS
	if node.CumulativeImpactMS > baseline {
		baseline = node.CumulativeImpactMS
	}
	return node.ActualImpactMS > 0 && baseline > 0 && node.ActualImpactMS > baseline*1.001
}

// --- lead text ----------------------------------------------------------------

func runtimeTraceProjLeadText(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, lang string, zh bool) string {
	var sections []string
	if line := runtimeTraceProjConclusionLine(projection, model, zh); line != "" {
		sections = append(sections, line)
	}
	sections = append(sections, runtimeTraceProjWindowLine(projection, model, zh))
	// Customer 2026-07-03: the reading note was one run-on paragraph and
	// unreadable — itemized, one short clause per line, deliberately plain
	// (no bold, no emoji beyond the glyphs it explains).
	//
	// NEW-7 (§7.6 对比场景客户回访 2026-07-04, 客户点名): the legend is DYNAMIC —
	// two fixed head clauses (top-down semantics, E# locatability) plus exactly
	// the catalog entries whose typed mark the fence render actually emitted,
	// in stable catalog order. The NEW-1 wake-direction wording lives verbatim
	// in the catalog's wake entry.
	if zh {
		lines := []string{
			"树读法:",
			"- 自上而下 = 从关注线程向上游追溯。",
			"- 时长、排序与 E# 均可定位到原始 trace_query 结构化证据,不是额外推测。",
		}
		lines = append(lines, runtimeTraceProjLegendGroupLines(model.Marks, true)...)
		sections = append(sections, strings.Join(lines, "\n"))
	} else {
		lines := []string{
			"Tree reading:",
			"- Top-down = tracing upstream from the focused thread.",
			"- Durations, ranks and E# tags locate structured trace_query evidence — never extra speculation.",
		}
		lines = append(lines, runtimeTraceProjLegendGroupLines(model.Marks, false)...)
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if len(model.Background) == 0 {
		// 两态拆分 (2026-07-05, specimen real_trace_e1_dual_window_normalized-
		// 20260705-212408): "the background-statistics view never ran" and "the
		// view ran but its background bucket came back empty" are different
		// facts — folded into one sentence, every no-background render read
		// like the same data gap. The split keys on the typed
		// RootCauseFamilyObserved compile flag (exact root_cause_ prefix),
		// never on prose. Wording stays jargon-free on both branches (去行话:
		// no 可承重 / off-chain in the lead).
		switch {
		case !projection.RootCauseFamilyObserved:
			if zh {
				sections = append(sections, "背景层: 本轮未运行产出背景统计的视图(root_cause_rank / wakeup_chain),背景层无数据;如需背景压力证据,可继续 trace_query view=root_cause_rank。")
			} else {
				sections = append(sections, "Background layer: no background-statistics view (root_cause_rank / wakeup_chain) ran this round, so this layer has no data. For background-pressure evidence, continue with trace_query view=root_cause_rank.")
			}
		case zh:
			sections = append(sections, "背景层: 背景统计视图已运行,但没有产出有数据支撑的背景/环境压力证据;这不等于背景没有影响,只表示本轮证据没有给出可审计的背景统计。")
		default:
			sections = append(sections, "Background layer: the background-statistics view ran, but produced no data-backed background/context pressure evidence. This does not prove there was no background influence; it only means this run lacks auditable background statistics.")
		}
	}
	return strings.Join(sections, "\n\n")
}

// runtimeTraceProjConclusionLine is FACT-ONLY: subject, cause, magnitudes and
// the typed drilldown target. It never emits advice/should-sentences — the
// system must not ghost-write the user-facing recommendation surface.
func runtimeTraceProjConclusionLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	primary, onChainFallback := runtimeTraceProjLeadSelect(projection, model)
	if primary == nil {
		if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
			return ""
		}
		// Every primary candidate was demoted to the background stanza (§7.30
		// 裁定1) and no data-bearing on-chain row could lead either (RN-3(a))
		// — the lead must say so instead of naming a demoted row as the
		// primary root cause and contradicting the tree below it.
		if zh {
			return "**主根因:** 窗口内未定位到链上主根因,见背景压力段。"
		}
		return "**Primary root cause:** no on-chain primary root cause was located in the window — see the background-pressure stanza."
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*primary, zh))
	// D4: the narrative lane uses the 中文（english_token） combined format on
	// the zh surface (tree rows stay concise zh; the table keeps raw tokens).
	cause := strings.TrimSpace(runtimeTraceCausalProjectionNarrativeCauseName(primary.Object, zh))
	if primary.IsAggregateMetric() {
		// The metric semantic name already carries the Object type word.
		cause = ""
	}
	ms := primary.CumulativeImpactMS
	if ms <= 0 {
		ms = runtimeTraceProjNodeDisplayImpact(*primary)
	}
	ms = runtimeTraceProjPeriodicHeadlineMS(*primary, ms)
	var b strings.Builder
	if zh {
		b.WriteString("**主根因:** ")
	} else {
		b.WriteString("**Primary root cause:** ")
	}
	b.WriteString(name)
	if cause != "" {
		b.WriteString(" " + cause)
	}
	if primary.MergedCount > 1 && primary.MergedMaxMS > 0 {
		// V1 (customer revisit 2026-07-03): a ×N aggregate's SUM never publishes
		// as the headline hard fact — show the per-instance max with the count;
		// the window share follows the same single-instance value.
		if zh {
			b.WriteString(fmt.Sprintf(" 单次最大 %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount))
			if model.WindowMS > 0 {
				b.WriteString(fmt.Sprintf("(占窗%.0f%%)", primary.MergedMaxMS/model.WindowMS*100))
			}
		} else {
			b.WriteString(fmt.Sprintf(" single max %.3fms ×%d", primary.MergedMaxMS, primary.MergedCount))
			if model.WindowMS > 0 {
				b.WriteString(fmt.Sprintf(" (%.0f%% of window)", primary.MergedMaxMS/model.WindowMS*100))
			}
		}
	} else if ms > 0 {
		b.WriteString(fmt.Sprintf(" %.3fms", ms))
		if model.WindowMS > 0 {
			if zh {
				b.WriteString(fmt.Sprintf("(占窗%.0f%%)", ms/model.WindowMS*100))
			} else {
				b.WriteString(fmt.Sprintf(" (%.0f%% of window)", ms/model.WindowMS*100))
			}
		}
	}
	if onChainFallback {
		// RN-3(a): the lead is the largest on-chain wait, not an engine-ranked
		// primary — the short note keeps the conclusion honest about that.
		b.WriteString(runtimeTraceProjLeadFallbackNote(model, zh))
	}
	// VS-2 (§7.10): a folded running-dominant lead carries the mechanism
	// composition clause on the conclusion line itself (each magnitude with
	// its own unit, mechanisms joined never summed). The lead selection above
	// stayed rank/attribution-driven — the fold verdict only WORDS the row it
	// already leads with.
	if clause, _, ok := runtimeTraceProjSupplyFoldClause(*primary, model.WindowMS, zh); ok {
		if zh {
			b.WriteString(",")
		} else {
			b.WriteString(", ")
		}
		b.WriteString(clause)
	}
	if target := strings.TrimSpace(primary.DrilldownTarget); target != "" && primary.IsSleepState() {
		if zh {
			b.WriteString(",下钻到 " + target)
		} else {
			b.WriteString(", drills down to " + target)
		}
	} else if primary.IsSleepState() && primary.Undrillable() {
		if zh {
			b.WriteString(",⊘窗口内无匹配唤醒、无法继续下钻")
		} else {
			b.WriteString(", ⊘ no matching wakeup in the window — cannot drill further")
		}
	}
	b.WriteString("。")
	if !zh {
		return strings.TrimSuffix(b.String(), "。") + "."
	}
	return b.String()
}

// runtimeTraceProjLeadPrimary picks the lead-line primary from the primary
// roots that SURVIVED the 裁定1 background demotion gate — a row rendered in
// the background stanza must never be named as the primary root cause. Nil when
// no primary exists or all of them were demoted.
//
// Selection order (V1, customer revisit 2026-07-03):
//  1. the engine's typed rank — the lowest positive Rank wins (the audit lane
//     already published rank=1; the conclusion must consume it instead of
//     re-ranking by displayed ms);
//  2. no ranked candidate: the largest single-instance effective attribution
//     (runtimeTraceProjLeadSelectionValue).
//
// A ×N aggregate's SUM (window projection total) never participates in the
// selection — same ruling family as S1 (排序合成分数不得以 ms 硬事实发布):
// the real customer conclusion named a ×7 hmfs_discard sum of 13.324ms over
// the engine's rank=1 running row of 4.115ms and contradicted its own body.
func runtimeTraceProjLeadPrimary(projection types.TraceCausalProjection, trunkLen int) *types.TraceCausalProjectionNode {
	roots := runtimeTraceCausalProjectionPrimaryRoots(projection)
	var ranked *types.TraceCausalProjectionNode
	for i := range roots {
		if runtimeTraceProjNodeDemotedToBackground(roots[i], trunkLen) {
			continue
		}
		if roots[i].Rank <= 0 {
			continue
		}
		// F4: rank TIES break on the single-instance selection value (per-instance
		// max for ×N aggregates), never on bucket order — the primary bucket sorts
		// by cumulative (= the R2 SUM), so "first row wins" re-admitted the merged
		// SUM through the very lane built to keep it out (a ×7 SUM of 13.324 beat
		// a single instance of 4.115 on a rank tie). Still-equal values keep the
		// earlier row (deterministic bucket order).
		if ranked == nil || roots[i].Rank < ranked.Rank ||
			(roots[i].Rank == ranked.Rank &&
				runtimeTraceProjLeadSelectionValue(roots[i]) > runtimeTraceProjLeadSelectionValue(*ranked)) {
			ranked = &roots[i]
		}
	}
	if ranked != nil {
		return ranked
	}
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	for i := range roots {
		if runtimeTraceProjNodeDemotedToBackground(roots[i], trunkLen) {
			continue
		}
		if v := runtimeTraceProjLeadSelectionValue(roots[i]); best == nil || v > bestValue {
			best, bestValue = &roots[i], v
		}
	}
	return best
}

// runtimeTraceProjLeadSelect is the SINGLE lead-selection surface consumed by
// the conclusion line, the comparison-overview primary cell and the model
// build (LeadKey) — one implementation, deterministic on (projection, model),
// so the three consumers can never disagree. Order:
//  1. the V1 primary-bucket lanes (runtimeTraceProjLeadPrimary, unchanged);
//  2. RN-3(a) (§7.9 runnable 主导场景审计 2026-07-04): the primary bucket has
//     rows but NONE survived the 裁定1 demotion gate (the former 未定位
//     branch) → fall back to the largest data-bearing ON-CHAIN row of the
//     rendered tree (chain/flat rows, discounted single-instance value) — the
//     customer's flat runnable 635.981ms/42% row was on the table while the
//     conclusion said nothing on-chain was located;
//  3. still nothing → nil, and the caller keeps the 未定位/背景压力段 text.
//
// An EMPTY primary bucket keeps the legacy no-conclusion behavior (the
// fallback only replaces the contradiction case, not the no-rank-data case).
// The second return reports that the on-chain fallback lane produced the lead
// (callers append the RN-3(a) short note).
func runtimeTraceProjLeadSelect(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (*types.TraceCausalProjectionNode, bool) {
	if primary := runtimeTraceProjLeadPrimary(projection, model.TrunkLen); primary != nil {
		return primary, false
	}
	if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
		return nil, false
	}
	if fallback := runtimeTraceProjLeadOnChainFallback(model); fallback != nil {
		return fallback, true
	}
	return nil, false
}

// runtimeTraceProjLeadOnChainFallback picks the RN-3(a) fallback lead: among
// the rendered tree's data-bearing on-chain rows (Kind chain = trunk/attached
// wake rows; Kind depthless = flat-fallback and depth-unresolved on-chain
// rows), the one with the largest discounted single-instance value
// (runtimeTraceProjLeadSelectionValue — the SAME caliber as the rankless V1
// lane: ×N SUMs and raw periodic cadence never compete). Cause/semantic/self/
// adjacent/background rows never lead here; a 0-value best (e.g. a periodic
// row discounted to exactly 0) returns nil rather than publishing a 0ms
// "largest wait". Ties keep the earlier render-order row (deterministic).
func runtimeTraceProjLeadOnChainFallback(model runtimeTraceProjTreeModel) *types.TraceCausalProjectionNode {
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if !row.HasData {
			continue
		}
		switch row.Kind {
		case runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowDepthless:
		default:
			continue
		}
		if v := runtimeTraceProjLeadSelectionValue(row.Node); v > bestValue {
			best, bestValue = &row.Node, v
		}
	}
	return best
}

// runtimeTraceProjLeadFallbackNote is the RN-3(a) short note appended to the
// conclusion line / comparison primary cell when the lead came from the
// on-chain fallback lane. The wording forks on the typed flat signal (empty
// model.Target = no ≥2-node wakeup path — the same condition every flat
// surface reads): a flat render says the chain could not be traced upstream;
// a trunked render says the ranked candidates all demoted to background.
func runtimeTraceProjLeadFallbackNote(model runtimeTraceProjTreeModel, zh bool) string {
	if strings.TrimSpace(model.Target) == "" {
		if zh {
			return "(链不可上溯,按窗口内最大 on-chain 等待)"
		}
		return " (chain not traceable upstream; largest on-chain wait in the window)"
	}
	if zh {
		return "(rank 候选均降背景,按窗口内最大 on-chain 等待)"
	}
	return " (all ranked candidates demoted to background; largest on-chain wait in the window)"
}

// runtimeTraceProjLeadSelectionValue is the rank-fallback ordering key for the
// conclusion line: the single-instance effective attribution. A ×N aggregate
// contributes its per-instance max — the merged SUM is a window-projection
// total across instances and must never compete against single-instance hard
// facts (V1, customer revisit 2026-07-03).
func runtimeTraceProjLeadSelectionValue(node types.TraceCausalProjectionNode) float64 {
	if node.PeriodicSource {
		// VS-1 (§7.8): a periodic source competes with its DISCOUNTED
		// attribution only, even when it is exactly 0 (pure in-period cadence)
		// — the raw display impact would re-admit the cadence sleep the
		// discount exists to keep out of the conclusion.
		return node.EffectiveImpactMS
	}
	if node.MergedCount > 1 {
		return node.MergedMaxMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return runtimeTraceProjNodeDisplayImpact(node)
}

// runtimeTraceProjPeriodicHeadlineMS is the SINGLE periodic-source magnitude
// override for headline surfaces — the conclusion line and the comparison
// overview's primary cell (F2, adversarial review 2026-07-04, forbids a second
// implementation): a periodic row's stated magnitude is its discounted
// attribution (runnable + lateness), never a raw cadence-dominated value —
// authoritative even at exactly 0. Non-periodic rows pass their raw magnitude
// through unchanged. Precise boolean gate on the typed flag.
func runtimeTraceProjPeriodicHeadlineMS(node types.TraceCausalProjectionNode, raw float64) float64 {
	if node.PeriodicSource {
		return node.EffectiveImpactMS
	}
	return raw
}

func runtimeTraceProjWindowLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS <= 0 {
		if zh {
			return "关注窗口起止未采集: 不显示占窗百分比,树内时长条满格=本批最大投影(回退尺度,系统不估算窗口)。"
		}
		return "Window bounds not captured: no window percentages; tree bars scale to the batch max projection (fallback scale — the system never estimates a window)."
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "关注窗口 %.3fs → %.3fs,共 %.3fms。", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	} else {
		fmt.Fprintf(&b, "Requested window %.3fs → %.3fs, %.3fms total.", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	}
	// Coverage = depth-1 cumulative vs window, by SUBTRACTION only — chain
	// values overlap on the wall clock and must never be summed across layers.
	// VS-1 F5(a) (adversarial review 2026-07-04): a periodic chain row whose
	// attribution legitimately discounts to exactly 0 still yields a coverage
	// sentence — "on-chain 已归因 0.000ms" IS the finding (the wait was normal
	// cadence), not a missing-data state; the >0 short-circuit alone would
	// silently drop the line exactly when the discount did its job.
	if attributed := runtimeTraceProjDepth1Cumulative(model); attributed > 0 || runtimeTraceProjChainHasPeriodicData(model) {
		// V2 (customer revisit 2026-07-03): when the 🎯 target published its own
		// state rows, the coverage denominator is the TARGET SYMPTOM duration,
		// not the whole window — a target that slept 11.7ms of a 101ms window
		// once rendered as "残差 97%". Falls back to the whole window (wording
		// unchanged) when no self-state row exists or the attribution exceeds
		// the symptom duration.
		// RN-6 (§7.9): the denominator family includes runnable, so the wording
		// says 等待(睡眠/阻塞/就绪) instead of claiming everything was
		// sleep/blocked.
		if symptom := runtimeTraceProjTargetSymptomMS(model); symptom > 0 && attributed <= symptom {
			residual := symptom - attributed
			if zh {
				fmt.Fprintf(&b, " 目标等待(睡眠/阻塞/就绪) %.3fms 中 on-chain 已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)。",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			} else {
				fmt.Fprintf(&b, " Of the target's %.3fms wait time (sleep/blocked/runnable), on-chain attributed %.3fms (%.0f%%), unattributed %.3fms (%.0f%%).",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			}
			b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
			b.WriteString(runtimeTraceProjPeriodicCadenceCoverageNote(model, residual, zh))
		} else if attributed <= model.WindowMS {
			residual := model.WindowMS - attributed
			if zh {
				fmt.Fprintf(&b, " on-chain 已归因 %.3fms/%.0f%%,未归因残差 %.3fms/%.0f%%。",
					attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
			} else {
				fmt.Fprintf(&b, " On-chain attributed %.3fms/%.0f%%, unattributed residual %.3fms/%.0f%%.",
					attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
			}
			b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
			b.WriteString(runtimeTraceProjPeriodicCadenceCoverageNote(model, residual, zh))
		} else {
			if zh {
				fmt.Fprintf(&b, " on-chain 已归因 %.3fms(其实际状态跨出窗口,见 ⚠ 标记)。", attributed)
			} else {
				fmt.Fprintf(&b, " On-chain attributed %.3fms (the underlying state crosses the window; see ⚠ marks).", attributed)
			}
		}
	}
	// R2 双窗关系行: when a user-requested window was derivable from the typed
	// entity pair and the projection window is a small sub-window of it (strict
	// numeric comparison: projection < 50% of the user window), say explicitly
	// how the two windows relate — the berlin customer saw a 101ms projection
	// with no mention of the 3.3s window they actually asked about.
	if model.UserWindowEnd > model.UserWindowStart && model.UserWindowStart > 0 {
		userMS := (model.UserWindowEnd - model.UserWindowStart) * 1000
		if model.WindowMS < userMS*0.5 {
			if zh {
				fmt.Fprintf(&b, "\n- 用户请求窗 %.3fs → %.3fs(共 %.1fs);本投影取其中代表性子窗,全窗指标见 Trace 指标快照",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			} else {
				fmt.Fprintf(&b, "\n- User-requested window %.3fs → %.3fs (%.1fs total); this projection covers a representative sub-window — full-window metrics live in the Trace Metric Snapshot",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			}
		}
	}
	return b.String()
}

// runtimeTraceProjTargetSymptomMS is the 🎯 target's own symptom duration: the
// sum of the target's self STATE-view rows' window projections (V2, customer
// revisit 2026-07-03). Summation is legal HERE only — these are one thread's
// own scheduler-state segments inside the window; cross-thread and cross-layer
// values still never add. 0 when the target exposed no qualifying self-state
// row (callers fall back to the whole window).
//
// F1 (adversarial review 2026-07-03): SelfRows deliberately mixes TWO typed
// views of the focused thread — its scheduler-STATE rows and hop-view rows
// (Role=causal_hop: critical_blocking / wakeup_causal_* / root_evidence:*)
// that re-describe wall clock already inside a state segment. Only state-view
// rows whose typed StateKind is in the symptom family enter the denominator:
// a 10ms sleep with an 8ms binder wait nested inside it is a 10ms symptom
// (never 18ms), and a running self row is not wait time at all. RN-6 (§7.9):
// the family is sleep/D/blocked PLUS runnable — a runnable-dominant target's
// ready-queue wait is its symptom (the customer 7.0 comparison rendered 目标
// 症状 "—" against a 42% runnable row). Precise typed signals only (Role enum
// + StateKind enum), never prose.
func runtimeTraceProjTargetSymptomMS(model runtimeTraceProjTreeModel) float64 {
	total := 0.0
	for _, row := range model.SelfRows {
		if row.Node.Role == types.TraceCausalRoleCausalHop {
			continue // blocked-wait/attribution hop view: wall clock already counted by its enclosing state segment
		}
		if !runtimeTraceProjSymptomFamilyStateKind(row.Node) {
			continue // running/stateless rows are not wait symptom time
		}
		if row.Node.ImpactMS > 0 {
			total += row.Node.ImpactMS
		}
	}
	return total
}

// runtimeTraceProjResidualOwnCaliberNote implements NEW-6 (§7.6 对比场景客户回访
// 2026-07-04, 客户追问"残差包含 on-chain 吗"): the coverage line's "残差 90%"
// visually contradicted the tree's own-process IO row (232ms) — that row is the
// SAME wall-clock segment as the target's D-state read through another caliber,
// deliberately excluded from the attribution numerator to avoid double
// counting, and the reader had to infer that. When such a row exists, the
// coverage sentence now says so itself. Precise signals only
// (runtimeTraceProjOwnCaliberIOPrimaryRow); no qualifying row → empty string
// (the coverage line stays byte-identical).
//
// The published amount is min(caliber value, residual): a caliber row may
// legitimately exceed the residual (it overlaps attributed wall clock too), and
// "残差中最大 X" must never claim more than the residual itself.
func runtimeTraceProjResidualOwnCaliberNote(model runtimeTraceProjTreeModel, residual float64, zh bool) string {
	if residual <= 0 {
		return ""
	}
	value, tag, ok := runtimeTraceProjOwnCaliberIOPrimaryRow(model)
	if !ok || value <= 0 {
		return ""
	}
	if value > residual {
		value = residual
	}
	if zh {
		ref := ""
		if tag != "" {
			ref = "(" + tag + ")"
		}
		return fmt.Sprintf(" 残差中最大 %.3fms 与自身 IO 口径行%s重叠解释,未计入链归因以防双计。", value, ref)
	}
	ref := ""
	if tag != "" {
		ref = " (" + tag + ")"
	}
	return fmt.Sprintf(" Up to %.3fms of the residual is co-explained by the own-process IO caliber row%s; it is excluded from the chain attribution to avoid double counting.", value, ref)
}

// runtimeTraceProjOwnCaliberIOPrimaryRow finds the largest target-own /
// same-process IO caliber row that is NOT inside the attribution numerator —
// the NEW-3 grouped primary (fold survivors carry the group max), with its
// evidence tag verbatim. Two precise typed lanes:
//   - tree lane: rows passing the NEW-3/F2 runtimeTraceProjOwnProcessIORow
//     gate (depthless own-edge rows only) — a chain-attached IO row (resolved
//     depth) already sits inside the depth-cumulative attribution lane, so
//     citing it as a residual overlap would contradict "未计入链归因";
//   - self lane: the 🎯 target's own rows with a typed IO caliber token that
//     stayed OUT of the symptom denominator (causal_hop views /
//     non-symptom-family states — the same two typed exclusions
//     runtimeTraceProjTargetSymptomMS applies; RN-6 widened both to the
//     symptom family together so the lanes stay complementary):
//     same-wall-clock re-descriptions, not extra time.
func runtimeTraceProjOwnCaliberIOPrimaryRow(model runtimeTraceProjTreeModel) (float64, string, bool) {
	best, tag, found := 0.0, "", false
	consider := func(row runtimeTraceProjTreeRow) {
		v := runtimeTraceProjNodeDisplayImpact(row.Node)
		if !found || v > best {
			best, tag, found = v, strings.TrimSpace(row.EvidenceTag), true
		}
	}
	for _, row := range model.TreeRows {
		if runtimeTraceProjOwnProcessIORow(row) {
			consider(row)
		}
	}
	for _, row := range model.SelfRows {
		if !runtimeTraceProjSameSegmentIOToken(row.Node) {
			continue
		}
		if row.Node.Role == types.TraceCausalRoleCausalHop || !runtimeTraceProjSymptomFamilyStateKind(row.Node) {
			consider(row)
		}
	}
	return best, tag, found
}

// runtimeTraceProjWaitFamilyStateKind reports whether the node's typed dominant
// scheduler state belongs to the sleep/D/blocked wait family. Precise typed
// enum check (never a prose substring): running/runnable and rows WITHOUT a
// StateKind (e.g. metric aggregates that never exposed a dominant state) are
// NOT waits. This is the whole-window idle annotation's family (F3) and the
// base of the symptom family below — RN-6 (§7.9) split the two gates on
// purpose: a runnable thread is WAITING for the symptom accounting but is
// never "疑似空闲".
func runtimeTraceProjWaitFamilyStateKind(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait",
		"d_state", "d_sleep", "uninterruptible_sleep", "io_wait":
		return true
	}
	return false
}

// runtimeTraceProjSymptomFamilyStateKind is the target-symptom state family
// (F1 as extended by RN-6, §7.9 runnable 主导场景审计 2026-07-04): the sleep/D/
// blocked wait family PLUS the runnable family (typed enum runnable /
// runnable_wait). A runnable-dominant target (customer 7.0 shape: sleep=0,
// runnable 635.981ms = 42% of the window) IS waiting — on the ready queue —
// yet its symptom denominator and the comparison table's 目标症状 cell
// rendered "—" next to a tree that showed the 42% runnable row. running and
// stateless rows stay excluded (running is occupancy, not wait time; the V
// 批 F1 hop-view exclusion and double-count defenses are untouched).
func runtimeTraceProjSymptomFamilyStateKind(node types.TraceCausalProjectionNode) bool {
	if runtimeTraceProjWaitFamilyStateKind(node) {
		return true
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "runnable", "runnable_wait":
		return true
	}
	return false
}

// runtimeTraceProjWholeWindowIdleRow is the SINGLE definition of the V3
// "整窗等待(疑似空闲)" annotation — the tree stanza tag and the detail-table
// mirror both call it (F3: two hand-synced copies were the drift risk). True
// only for a background row in the wait family AND whose projection covers
// ≥99% of the window without exceeding it (H8 tolerance: over-window
// cumulative rows are the multi-CPU ACTIVE shape, never idle). A whole-window
// running CPU hog or a stateless cpu·ms aggregate row that happens to ≈ the
// window never takes the tag.
//
// RN-8 (§7.9 runnable 主导场景审计 2026-07-04): the wait-family guard accepts
// EITHER the typed dominant StateKind (F3 lane, unchanged) OR — when the row
// exposed NO StateKind at all — an exact typed wait token
// (runtimeTraceProjWaitFamilyTypeTokenOnly): the customer's 8×101ms
// d_state_or_io_wait background rows carried the type token but no dominant
// state, rendered bare, and the model read them as an "IO 突发". running/
// runnable rows stay excluded on both lanes (a present StateKind always wins
// — the token lane never overrides a non-wait state).
func runtimeTraceProjWholeWindowIdleRow(row runtimeTraceProjTreeRow, windowMS float64) bool {
	if row.Kind != runtimeTraceProjTreeRowBackground || windowMS <= 0 {
		return false
	}
	if !runtimeTraceProjWaitFamilyStateKind(row.Node) &&
		!runtimeTraceProjWaitFamilyTypeTokenOnly(row.Node) {
		return false
	}
	impact := runtimeTraceProjNodeDisplayImpact(row.Node)
	return impact >= windowMS*0.99 && impact <= windowMS*1.001
}

// runtimeTraceProjWaitFamilyTypeTokenOnly is the RN-8 stateless lane of the
// idle guard: the row exposed NO dominant StateKind and its typed token
// (verbatim TypeToken or Predicate, exact canonical match — never Object
// prose) is one of the wait-only tokens d_state_or_io_wait / blocking_span.
// Both tokens describe blocked wall clock by construction; any row that DID
// expose a StateKind is judged by the state lane alone.
func runtimeTraceProjWaitFamilyTypeTokenOnly(node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.StateKind) != "" {
		return false
	}
	for _, token := range []string{node.TypeToken, node.Predicate} {
		switch runtimeTraceCausalProjectionCanonicalNode(token) {
		case "d_state_or_io_wait", "blocking_span":
			return true
		}
	}
	return false
}

// runtimeTraceProjChainHasPeriodicData reports whether any data-bearing CHAIN
// row carries the typed VS-1 periodic-source flag — the precise gate for the
// F5 coverage lanes (a periodic row's 0 attribution is a finding, not absent
// data). Chain rows only: adjacent/background periodic rows never feed the
// attribution numerator.
func runtimeTraceProjChainHasPeriodicData(model runtimeTraceProjTreeModel) bool {
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowChain && row.HasData && row.Node.PeriodicSource {
			return true
		}
	}
	return false
}

// runtimeTraceProjPeriodicCadenceMS is the F5(b) cadence amount: Σ over the
// data-bearing periodic chain rows of (raw on-chain cumulative − discounted
// attribution) — the wall clock the discount reclassified as normal signal
// cadence. Callers clamp it to the residual they are annotating.
func runtimeTraceProjPeriodicCadenceMS(model runtimeTraceProjTreeModel) float64 {
	total := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || !row.HasData || !row.Node.PeriodicSource {
			continue
		}
		raw := row.Node.CumulativeImpactMS
		if raw <= 0 {
			raw = runtimeTraceProjNodeDisplayImpact(row.Node)
		}
		if d := raw - row.Node.EffectiveImpactMS; d > 0 {
			total += d
		}
	}
	return total
}

// runtimeTraceProjPeriodicCadenceCoverageNote is the F5(b) third coverage
// item: with a periodic chain row on the table, the residual sentence names
// how much of the unattributed time is the periodic source's normal in-period
// cadence — deliberately counted NEITHER as attribution NOR as unexplained
// residual. Clamped to the residual: the note must never claim more than the
// residual itself. Empty (coverage line byte-stable) without a periodic row.
func runtimeTraceProjPeriodicCadenceCoverageNote(model runtimeTraceProjTreeModel, residual float64, zh bool) string {
	cadence := runtimeTraceProjPeriodicCadenceMS(model)
	if cadence <= 0 || residual <= 0 {
		return ""
	}
	if cadence > residual {
		cadence = residual
	}
	if zh {
		return fmt.Sprintf(" 其中 %.3fms 为周期性信号源期内正常节拍(不计归因、不属未解释残差)。", cadence)
	}
	return fmt.Sprintf(" Of that, %.3fms is a periodic signal source's normal in-period cadence (not attributed, not unexplained residual).", cadence)
}

func runtimeTraceProjDepth1Cumulative(model runtimeTraceProjTreeModel) float64 {
	if v := runtimeTraceProjChainDepthCumulative(model, 1); v > 0 {
		return v
	}
	// VS-1 F5(c) (adversarial review 2026-07-04): H10's premise is "every
	// depth-1 trunk node is a bare TRANSIT hop with no data of its own". A
	// data-bearing depth-1 periodic row whose attribution legitimately
	// discounts to exactly 0 does NOT satisfy that premise — falling through
	// to a deeper layer would resurrect the raw cadence-dominated cumulative
	// the discount just removed.
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowChain && row.Depth == 1 && row.HasData && row.Node.PeriodicSource {
			return 0
		}
	}
	// H10 fallback (berlin shape): every depth-1 trunk node was a bare transit
	// hop with no data row of its own, which silently dropped the whole
	// attributed/residual coverage line. Fall back to the SHALLOWEST depth that
	// carries a data-bearing chain row: its cumulative already contains every
	// deeper on-chain layer by wall-clock containment, so it is the tightest
	// available lower bound of on-chain attributed time. Max within ONE depth
	// only — values are never summed across layers (墙钟不可加和).
	minDepth := 0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || !row.HasData || row.Depth <= 1 {
			continue
		}
		if minDepth == 0 || row.Depth < minDepth {
			minDepth = row.Depth
		}
	}
	if minDepth == 0 {
		return 0
	}
	return runtimeTraceProjChainDepthCumulative(model, minDepth)
}

// runtimeTraceProjChainDepthCumulative returns the largest cumulative impact
// among data-bearing chain rows at exactly the given depth.
func runtimeTraceProjChainDepthCumulative(model runtimeTraceProjTreeModel, depth int) float64 {
	max := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || row.Depth != depth || !row.HasData {
			continue
		}
		v := row.Node.CumulativeImpactMS
		if v <= 0 {
			v = runtimeTraceProjNodeDisplayImpact(row.Node)
		}
		if row.Node.PeriodicSource {
			// VS-1 (§7.8): a periodic source's raw cumulative is cadence-
			// dominated. The coverage numerator consumes the SAME discounted
			// caliber as ranking (runnable + lateness), so "on-chain attributed"
			// never claims normal signal cadence as explained wait time.
			v = row.Node.EffectiveImpactMS
		}
		if v > max {
			max = v
		}
	}
	return max
}

// --- lossless detail table ------------------------------------------------------

// runtimeTraceProjDetailRows collects the data-bearing rendered rows in the
// canonical detail order (self → tree → adjacent → background) — shared by
// the T10 (a) key-metric table and the (b) lossless vertical blocks so the
// two surfaces can never disagree on membership or order.
func runtimeTraceProjDetailRows(model runtimeTraceProjTreeModel) []runtimeTraceProjTreeRow {
	var out []runtimeTraceProjTreeRow
	collect := func(rows []runtimeTraceProjTreeRow) {
		for _, row := range rows {
			if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
				continue
			}
			out = append(out, row)
		}
	}
	collect(model.SelfRows)
	collect(model.TreeRows)
	collect(model.Adjacent)
	collect(model.Background)
	return out
}

// runtimeTraceProjDetailTable renders the PTV4 T10 (a) key-metric table:
// AT MOST six columns (pinned) — node identity (with its E# cross-reference
// into the (b) blocks) plus the duration quad plus evidence·confidence. All
// qualitative attributes (type token, causal position, relation, impact
// shape, ×N rosters, full uncapped names) live in the (b) vertical lossless
// blocks (runtimeTraceProjDetailFullText).
//
// 复核 (双宿去重): one node classified into TWO display lanes (the E12 shape —
// a semantic span that is also an adjacent row, same typed node key) rendered
// two byte-identical (a) rows, which a reader would sum. The (a) table now
// keeps ONE row per node key with an explicit seat note (`✦/◇双席`); the (b)
// blocks keep BOTH stanzas (their layer/relation lines differ — lossless).
func runtimeTraceProjDetailTable(model runtimeTraceProjTreeModel, zh bool) ([]string, []types.AnswerBlockItem) {
	columns := []string{"节点[E#]", "窗口投影", "链上累计", "有效归因", "实际状态", "证据·置信"}
	if !zh {
		columns = []string{"Node [E#]", "Window projection", "Chain total", "Attribution", "Actual state", "Evidence · confidence"}
	}
	dash := "—"
	msCell := func(v float64) string {
		if v <= 0 {
			return dash
		}
		return fmt.Sprintf("%.3fms", v)
	}
	detailRows := runtimeTraceProjDetailRows(model)
	// The dual-seat identity is the TYPED EvidenceID only — the composite
	// (subject, object, predicate) node-key fallback can collide for rows that
	// are genuinely distinct display rows (e.g. two evidence-less background
	// rows of one subject with different dominant states), and folding those
	// would hide data. No evidence id → no dedupe (fail open to two rows).
	seatKey := func(node types.TraceCausalProjectionNode) string {
		return strings.TrimSpace(node.EvidenceID)
	}
	seats := map[string][]string{}
	for _, row := range detailRows {
		key := seatKey(row.Node)
		if key == "" {
			continue
		}
		glyph := runtimeTraceProjDetailSeatGlyph(row, zh)
		known := false
		for _, existing := range seats[key] {
			if existing == glyph {
				known = true
				break
			}
		}
		if !known {
			seats[key] = append(seats[key], glyph)
		}
	}
	emitted := map[string]bool{}
	var rows []types.AnswerBlockItem
	for _, row := range detailRows {
		node := row.Node
		key := seatKey(node)
		if key != "" && emitted[key] {
			continue // dual-seat copy: the first seat's row carries the note
		}
		emitted[key] = true
		name := runtimeTraceCausalProjectionNodeSubjectCell(node, zh)
		if node.MergedCount > 1 {
			// ×N data token inline (T4 三式); the form semantics + member
			// roster live in the (b) block and the legend.
			if runtimeTraceProjSubjectlessFoldRow(node) {
				name += " " + runtimeTraceProjMergedMaxTagText(node, zh)
			} else {
				name += " " + runtimeTraceProjMergedSumTagText(node)
			}
		}
		if node.DuplicatePublications > 1 {
			name += " " + runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)
		}
		if node.Undrillable() {
			name += " ⊘"
		}
		if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
			name += " [" + tag + "]"
		}
		if s := seats[key]; len(s) > 1 {
			joined := strings.Join(s, "/")
			switch {
			case zh && len(s) == 2:
				name += " " + joined + "双席"
			case zh:
				name += " " + joined + "多席"
			case len(s) == 2:
				name += " " + joined + " dual-seat"
			default:
				name += " " + joined + " multi-seat"
			}
		}
		// CMP-3 mirror (F6): every duration cell of a cross-thread aggregate
		// row carries the unit annotation.
		crossThread := runtimeTraceProjCrossThreadAggregateType(node)
		annotated := func(v float64) string {
			return runtimeTraceProjDetailCrossThreadCell(msCell(v), v, crossThread, zh)
		}
		effective := annotated(node.EffectiveImpactMS)
		if runtimeTraceProjEffectiveInherited(node) {
			if zh {
				effective += "(承自等待区间)"
			} else {
				effective += " (inherited)"
			}
		}
		// VS-1 (§7.8): a periodic row's attribution cell shows the discounted
		// value explicitly — 0.000ms included — with its composition.
		if node.PeriodicSource {
			if zh {
				effective = fmt.Sprintf("%.3fms(可运行+迟到量)", node.EffectiveImpactMS)
			} else {
				effective = fmt.Sprintf("%.3fms (runnable + lateness)", node.EffectiveImpactMS)
			}
		}
		actual := annotated(node.ActualImpactMS)
		// RN-2b: no anchor window → no ⚠ (same gate as the tree tag).
		if model.WindowMS > 0 && runtimeTraceProjCrossWindow(node) && node.ActualImpactMS > 0 {
			actual += " ⚠"
		}
		evidence := row.EvidenceTag
		if evidence == "" {
			evidence = dash
		}
		if tier := runtimeTraceProjConfidenceTier(node.Confidence, zh); tier != "" {
			evidence += " · " + tier
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionMarkdownSafe(name),
				annotated(node.ImpactMS), annotated(node.CumulativeImpactMS),
				effective, actual,
				evidence,
			},
			CitationRef: -1,
		})
	}
	return columns, rows
}

// runtimeTraceProjDetailSeatGlyph names one (a)-table seat of a node for the
// dual-seat note: the lane glyph the fence already uses for that family
// (typed row Kind switch — never prose).
func runtimeTraceProjDetailSeatGlyph(row runtimeTraceProjTreeRow, zh bool) string {
	switch row.Kind {
	case runtimeTraceProjTreeRowSemantic:
		return "✦"
	case runtimeTraceProjTreeRowAdjacent:
		return "◇"
	case runtimeTraceProjTreeRowBackground:
		return "▒"
	case runtimeTraceProjTreeRowSelf:
		if zh {
			return "自身"
		}
		return "self"
	default:
		if zh {
			return "链"
		}
		return "chain"
	}
}

// runtimeTraceProjDetailFullName composes a node's FULL display name with NO
// cell caps (PTV4 T10 (b): the 28/22/36-rune CompactCellText caps are
// withdrawn on this surface — more lossless than the pre-T10 table).
func runtimeTraceProjDetailFullName(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsAggregateMetric() {
		return strings.TrimSpace(runtimeTraceCausalProjectionAggregateMetricName(node, zh))
	}
	if blocking := runtimeTraceCausalProjectionBlockingName(node, zh); blocking != "" {
		if runtimeTraceCausalProjectionKnownSubject(node.Subject) {
			return strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh)) + " / " + blocking
		}
		return blocking
	}
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if (node.Role == types.TraceCausalRoleSemanticSpan || strings.TrimSpace(node.Predicate) == "trace_semantic_span") &&
		strings.TrimSpace(node.SpanName) != "" {
		object = strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.SpanName, zh))
	}
	switch {
	case subject != "" && object != "":
		return subject + " / " + object
	case subject != "":
		return subject
	case object != "":
		return object
	default:
		return "trace causal node"
	}
}

// runtimeTraceProjDetailFullText renders the PTV4 T10 (b) lossless vertical
// blocks: one stanza per rendered node — "**[E#] full name**" plus "键: 值"
// lines carrying every qualitative attribute the (a) table and the tree do
// not: layer, causal position · priority, FULL uncapped name, raw type token,
// relation ▸ impact points, impact shape (with the idle / periodic / VS-2
// clauses verbatim), the ×N form + full member roster, the same-segment IO
// calibers, the inherited-attribution note, occupier roster and the
// full-window coverage cross-reference. Hard floor: every item the tree
// demotes or omits has a reachable lossless home here.
func runtimeTraceProjDetailFullText(model runtimeTraceProjTreeModel, zh bool) string {
	flat := strings.TrimSpace(model.Target) == ""
	var stanzas []string
	for _, row := range runtimeTraceProjDetailRows(model) {
		node := row.Node
		tag := strings.TrimSpace(row.EvidenceTag)
		heading := "**"
		if tag != "" {
			heading += "[" + tag + "] "
		}
		heading += runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjDetailFullName(node, zh)) + "**"
		var lines []string
		add := func(zhKey, enKey, value string) {
			value = strings.TrimSpace(value)
			if value == "" {
				return
			}
			key := zhKey
			if !zh {
				key = enKey
			}
			lines = append(lines, "- "+key+": "+value)
		}
		add("完整名称", "full name", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjDetailFullName(node, zh)))
		add("层级", "layer", runtimeTraceProjDetailLayerCell(row, zh, flat))
		add("因果位置·优先级", "causal position · priority", runtimeTraceProjDetailPositionCell(row, model.LeadKey, zh))
		typeToken := runtimeTraceCausalProjectionRawTypeToken(node)
		add("类型", "type", runtimeTraceCausalProjectionMarkdownSafe(typeToken))
		relation := runtimeTraceProjDetailRelationCell(row, zh, flat)
		if len(node.SecondaryObjects) > 0 {
			joined := strings.Join(node.SecondaryObjects, "/")
			if zh {
				relation += " ▸ 影响点 " + joined
			} else {
				relation += " ▸ impact point " + joined
			}
		}
		add("关系 ▸ 影响点", "relation ▸ impact point", runtimeTraceCausalProjectionMarkdownSafe(relation))
		shape := runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		// V3/F3: the whole-window-wait annotation mirrors the SAME shared
		// judgment as the fence tag — full semantics on this surface.
		if runtimeTraceProjWholeWindowIdleRow(row, model.WindowMS) {
			idle := "整窗等待(疑似空闲)"
			if !zh {
				idle = "whole-window wait (likely idle)"
			}
			if shape == "" {
				shape = idle
			} else {
				shape += "·" + idle
			}
		}
		// VS-1 mirror: full cadence semantics (the fence tag keeps data only).
		if node.PeriodicSource {
			period := ""
			if node.DetectedPeriodMS > 0 {
				period = fmt.Sprintf(",周期≈%.1fms", node.DetectedPeriodMS)
				if !zh {
					period = fmt.Sprintf(", period ≈%.1fms", node.DetectedPeriodMS)
				}
			}
			periodicNote := "周期性信号源(期内睡眠为正常节拍" + period + ")"
			if !zh {
				periodicNote = "periodic signal source (in-period sleep is normal cadence" + period + ")"
			}
			if shape == "" {
				shape = periodicNote
			} else {
				shape += "·" + periodicNote
			}
		}
		// VS-2 mirror: the SAME single-source clause (no width pressure here).
		if clause, _, ok := runtimeTraceProjSupplyFoldClause(node, model.WindowMS, zh); ok {
			if shape == "" {
				shape = clause
			} else {
				shape += "·" + clause
			}
		}
		add("影响形态", "impact shape", shape)
		if node.MergedCount > 1 {
			var form string
			if runtimeTraceProjSubjectlessFoldRow(node) {
				form = fmt.Sprintf("×%d 取最大口径(墙钟跨线程不可加和,不求和),各 %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				if !zh {
					form = fmt.Sprintf("×%d member-MAX caliber (wall clock never sums across threads), each %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				}
			} else {
				form = fmt.Sprintf("×%d 求和口径,单次 %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				if !zh {
					form = fmt.Sprintf("×%d SUM caliber, each %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				}
			}
			if len(node.MergedSubjects) > 0 {
				sep := "、"
				if !zh {
					sep = ", "
				}
				roster := strings.Join(node.MergedSubjects, sep)
				if node.MergedCount > len(node.MergedSubjects) {
					if zh {
						roster += " 等"
					} else {
						roster += ", …"
					}
				}
				if zh {
					form += ";成员: " + roster
				} else {
					form += "; members: " + roster
				}
			}
			add("×N 明细", "×N detail", runtimeTraceCausalProjectionMarkdownSafe(form))
		}
		if node.DuplicatePublications > 1 {
			dup := fmt.Sprintf("×%d 同值(同一测量被重复发布,数值为单次测量)", node.DuplicatePublications)
			if !zh {
				dup = fmt.Sprintf("×%d same-value (one measurement republished; the value is that single measurement)", node.DuplicatePublications)
			}
			add("重复发布", "duplicate publications", dup)
		}
		if len(row.IOFoldPeers) > 0 {
			add("同段IO口径", "same-segment IO calibers", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh)))
		}
		if runtimeTraceProjEffectiveInherited(node) {
			inherited := fmt.Sprintf("有效归因 %.3fms 承自等待区间,非本行实测", node.EffectiveImpactMS)
			if !zh {
				inherited = fmt.Sprintf("attribution %.3fms inherited from the wait interval, not measured on this row", node.EffectiveImpactMS)
			}
			add("承自注", "inherited note", inherited)
		}
		if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
			add("持有点", "held at", runtimeTraceCausalProjectionMarkdownSafe(site))
		}
		if roster := strings.TrimSpace(node.OccupierSummary); roster != "" {
			add("同窗占用者", "same-window occupiers", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjOccupierRosterDisplay(roster, zh)+"(cpu·ms)"))
		}
		if coverage, ok := runtimeTraceProjFullWindowCoverageTag(node, zh); ok {
			add("全窗合计", "full-window total", runtimeTraceCausalProjectionMarkdownSafe(coverage.Text))
		}
		stanzas = append(stanzas, heading+"\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(stanzas, "\n\n")
}

// runtimeTraceCausalProjectionSyntheticEvidenceLocator renders the display
// locator of a synthetic-line entry (CMP-7b): the artifact basename (plus the
// entry's own time window when it has one) — never a line suffix. Returns ""
// when the ref carries NO artifact name (bare line=N form): the caller then
// keeps the legacy display, because stripping a bare ref would leave nothing
// auditable on the panel.
func runtimeTraceCausalProjectionSyntheticEvidenceLocator(entry runtimeTraceCausalProjectionEvidenceEntry) string {
	pathPart, _ := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref)
	if _, bare := runtimeTraceCausalProjectionBareLineRef(entry.Ref); bare {
		return ""
	}
	tail := strings.TrimPrefix(runtimeTraceCausalProjectionPathTail(pathPart, 1), "…/")
	if tail == "" || types.TraceCausalProjectionPlaceholderArtifactToken(tail) {
		// Lane placeholders ("attached_trace"/"trace_query"/"runtime_artifact")
		// are not artifact names — stripping the line suffix would leave a
		// zero-information token as the whole locator (F1, review 2026-07-04).
		return ""
	}
	if entry.Window != "" {
		return runtimeTraceCausalProjectionCompactCellText(tail, 48) + " " + runtimeTraceCausalProjectionMarkdownSafe(entry.Window)
	}
	return runtimeTraceCausalProjectionCompactCellText(tail, 64)
}

// runtimeTraceProjDetailCrossThreadCell mirrors the CMP-3 unit annotation on
// one lossless-table ms cell (F6): every positive duration cell of a
// cross-thread aggregate row carries the annotation; dashes and wall-clock
// rows pass through untouched.
func runtimeTraceProjDetailCrossThreadCell(cell string, value float64, crossThread, zh bool) string {
	if !crossThread || value <= 0 {
		return cell
	}
	if zh {
		return cell + "(跨线程累计,非墙钟)"
	}
	return cell + " (cross-thread cumulative)"
}

func runtimeTraceProjDetailLayerCell(row runtimeTraceProjTreeRow, zh, flat bool) string {
	switch row.Kind {
	case runtimeTraceProjTreeRowSelf:
		if zh {
			return "目标状态"
		}
		return "target state"
	case runtimeTraceProjTreeRowAdjacent:
		if zh {
			return "◇ 邻近"
		}
		return "◇ adjacent"
	case runtimeTraceProjTreeRowBackground:
		if zh {
			return "▒ 背景"
		}
		return "▒ background"
	case runtimeTraceProjTreeRowDepthless:
		if flat {
			// No trunk exists, so "detached/unresolved" would describe every
			// single row — render the plain typed depth (or a dash) instead.
			if row.Depth > 0 {
				if zh {
					return fmt.Sprintf("深度%d", row.Depth)
				}
				return fmt.Sprintf("depth %d", row.Depth)
			}
			return "—"
		}
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("深度%d(未接入链)", row.Depth)
			}
			return fmt.Sprintf("depth %d (detached)", row.Depth)
		}
		if zh {
			return "链上·深度未解析"
		}
		return "on-chain · depth unresolved"
	case runtimeTraceProjTreeRowCause:
		if zh {
			return fmt.Sprintf("成因·深度%d", row.Depth)
		}
		return fmt.Sprintf("cause · depth %d", row.Depth)
	case runtimeTraceProjTreeRowSemantic:
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("语义·深度%d", row.Depth)
			}
			return fmt.Sprintf("span · depth %d", row.Depth)
		}
		if zh {
			return "语义span"
		}
		return "span"
	default:
		if row.Depth > 0 {
			if zh {
				return fmt.Sprintf("深度%d", row.Depth)
			}
			return fmt.Sprintf("depth %d", row.Depth)
		}
		if zh {
			return "链上"
		}
		return "on-chain"
	}
}

func runtimeTraceProjDetailRelationCell(row runtimeTraceProjTreeRow, zh, flat bool) string {
	parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Parent, zh))
	// 复核: rows attached after the long-trunk fold carry the builder's "…"
	// parent sentinel — say what it IS instead of leaking the raw glyph as a
	// pseudo thread name ("唤醒 ▸ …").
	if parent == "…" {
		if zh {
			parent = "(折叠段)"
		} else {
			parent = "(folded segment)"
		}
	}
	switch row.Kind {
	case runtimeTraceProjTreeRowSelf:
		if zh {
			return "自身状态"
		}
		return "own state"
	case runtimeTraceProjTreeRowAdjacent:
		if zh {
			return "邻近支撑(无直接唤醒边)"
		}
		return "adjacent support (no direct wake edge)"
	case runtimeTraceProjTreeRowBackground:
		if zh {
			return "背景支撑"
		}
		return "background support"
	}
	if row.Kind == runtimeTraceProjTreeRowDepthless && parent == "" {
		if flat {
			// Flat mode: every row lacks an attach point by construction; the
			// per-row placeholder is pure spam next to the flat-cause header.
			return "—"
		}
		if zh {
			return "on-chain·影响点未解析"
		}
		return "on-chain · impact point unresolved"
	}
	label := ""
	switch row.Edge {
	case runtimeTraceProjTreeEdgeOwn:
		// F2: the own-edge row re-describes the target's own process wall
		// clock — never a wake claim, and no "▸ parent" suffix (there is no
		// upstream relation to point at). Mirrors the fence's ├─自身─ edge.
		if zh {
			return "自身进程IO"
		}
		return "own-process IO"
	case runtimeTraceProjTreeEdgeDrill:
		if zh {
			label = "下钻"
		} else {
			label = "drill"
		}
	case runtimeTraceProjTreeEdgeSemantic:
		if zh {
			label = "语义span"
		} else {
			label = "span"
		}
	case runtimeTraceProjTreeEdgeCause:
		if zh {
			label = "成因"
		} else {
			label = "cause"
		}
	default:
		if zh {
			label = "唤醒"
		} else {
			label = "wakes"
		}
	}
	if parent == "" {
		return label
	}
	return label + " ▸ " + parent
}

func runtimeTraceProjConfidenceTier(confidence float64, zh bool) string {
	switch {
	case confidence >= 0.85:
		if zh {
			return "高"
		}
		return "high"
	case confidence >= 0.6:
		if zh {
			return "中"
		}
		return "mid"
	case confidence > 0:
		if zh {
			return "低"
		}
		return "low"
	default:
		return ""
	}
}

// --- file-grouped evidence index ---------------------------------------------

// runtimeTraceProjEvidenceBlockParts renders the evidence roster grouped by
// artifact file: when every locator shares one file, the file name appears once
// in the block text and each entry keeps only its line-range suffix (a real
// customer render burned 29 lines repeating one truncated file name).
func runtimeTraceProjEvidenceBlockParts(evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) (string, []types.AnswerBlockItem) {
	if evidence == nil || len(evidence.order) == 0 {
		return "", nil
	}
	// H19: an entry whose ref is the naked line=N / lines=N-M form (its source
	// node carried no SupportRefs path) breaks locator uniformity ("lines=…"
	// next to "berlin.systrace:…"). When every path-carrying entry of THIS
	// roster agrees on exactly one artifact, adopt that artifact for the bare
	// entries (display copy only). Ambiguous multi-artifact rosters keep the
	// bare form rather than guessing an artifact.
	entries := append([]runtimeTraceCausalProjectionEvidenceEntry(nil), evidence.order...)
	if shared := runtimeTraceCausalProjectionSoleArtifactPath(entries); shared != "" {
		for i := range entries {
			if lineRange, ok := runtimeTraceCausalProjectionBareLineRef(entries[i].Ref); ok {
				entries[i].Ref = shared + ":" + lineRange
			}
		}
	}
	sharedFile := ""
	uniform := true
	for _, entry := range entries {
		pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref)
		if suffix == "" || strings.TrimSpace(pathPart) == "" {
			uniform = false
			break
		}
		if sharedFile == "" {
			sharedFile = pathPart
			continue
		}
		if pathPart != sharedFile {
			uniform = false
			break
		}
	}
	intro := runtimeTraceCausalProjectionEvidenceText(zh)
	if uniform && sharedFile != "" && len(entries) > 1 {
		// PTV4 T11 (RTC f1): the grouped intro shows the artifact BASENAME —
		// the verbatim sharedFile is a machine-local blob absolute path
		// (/…/.codrax/blob/…/attached_trace.txt), the only display surface
		// that still leaked it. Sibling surfaces (synthetic locator, audit
		// block) already basename; entries keep their own suffixes untouched.
		display := strings.TrimPrefix(runtimeTraceCausalProjectionPathTail(sharedFile, 1), "…/")
		if display == "" {
			display = sharedFile
		}
		if zh {
			intro += " 全部证据位于 `" + runtimeTraceCausalProjectionMarkdownSafe(display) + "`,各条只列行号区间。"
		} else {
			intro += " All locators live in `" + runtimeTraceCausalProjectionMarkdownSafe(display) + "`; entries list only line ranges."
		}
	}
	items := make([]types.AnswerBlockItem, 0, len(entries))
	for _, entry := range entries {
		locator := runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow(entry.Ref, entry.Window)
		if uniform && sharedFile != "" && len(entries) > 1 {
			// Grouped mode: the shared file name is stated once in the intro; each
			// entry keeps only its own window (preferred, 裁定6) or line range.
			if entry.Window != "" {
				locator = runtimeTraceCausalProjectionMarkdownSafe(entry.Window)
			} else if _, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref); suffix != "" {
				locator = runtimeTraceCausalProjectionMarkdownSafe(suffix)
			}
		}
		if entry.SyntheticLine {
			// CMP-7b: an absence observation (missing_wakeup) has no trace row of
			// its own — its line span is interval bookkeeping, so a "file:44"
			// locator reads as a real row. Display keeps only the artifact name;
			// the raw record retains the interval lines for audit. A bare line
			// ref with no artifact name keeps its legacy display — stripping it
			// would leave nothing auditable on the panel.
			if synthetic := runtimeTraceCausalProjectionSyntheticEvidenceLocator(entry); synthetic != "" {
				locator = synthetic
			}
		}
		if locator == "" {
			locator = "trace_query"
		}
		audit := runtimeTraceCausalProjectionCompactCellText(entry.Details, 72)
		var text string
		if zh {
			text = fmt.Sprintf("定位: %s；审计: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "；完整定位见原始 trace_query 记录"
			}
		} else {
			text = fmt.Sprintf("locator: %s; audit: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
			if runtimeTraceCausalProjectionEvidenceRefShortened(entry.Ref, locator) {
				text += "; full locator remains in the original trace_query record"
			}
		}
		items = append(items, types.AnswerBlockItem{
			ID:          strings.ToLower(entry.ID),
			Label:       entry.ID,
			Text:        text,
			CitationRef: -1,
		})
	}
	return intro, items
}

// runtimeTraceProjOccupierTag renders the RN-1 (§7.9) same-window occupier
// attribution of a runnable row as its tail tag: the compiled typed
// OccupierSummary roster (thread:cpu·ms each, subject-matched at compile
// time) behind a localized label. PTV4 T1: a demotable tag — the occupier
// names and their cpu·ms values have no other fence carrier, so it is never
// elided; on width pressure it moves WHOLE onto "· " subordinate detail
// line(s). Appended by the RN-B lane: new helper + one call site only.
func runtimeTraceProjOccupierTag(node types.TraceCausalProjectionNode, zh bool) (runtimeTraceProjTag, bool) {
	roster := strings.TrimSpace(node.OccupierSummary)
	if roster == "" {
		return runtimeTraceProjTag{}, false
	}
	// F-3 (统一复核 2026-07-04): roster member names pass the RN-4
	// comm-truncation rewrite — "<...>-49706" is a SYSTEM placeholder, not a
	// thread name, and rendered verbatim it read as line noise. Display-only:
	// the producer's occupier_N notes/Summary keep the verbatim token for
	// audit; grouping/match keys never touch this surface.
	roster = runtimeTraceProjOccupierRosterDisplay(roster, zh)
	text := "同窗占用者: " + roster + "(cpu·ms)"
	if !zh {
		text = "same-window occupiers: " + roster + " (cpu·ms)"
	}
	// PTV4 T1: never elided/shaved — demotes intact to a subordinate line.
	return runtimeTraceProjTag{Text: text}, true
}

// runtimeTraceProjOccupierRosterDisplay rewrites RN-4 comm-truncation
// placeholder names inside the RN-1 occupier roster ("name:12.345ms" members
// joined by "、", the compile-side traceCausalProjectionOccupierRoster shape).
// Member values split at the LAST ':' so real thread names containing colons
// (e.g. binder:486_1-10803) stay intact; only names passing the exact
// runtimeTraceCausalProjectionCommTruncatedTid match ("<...>-" + pure digits)
// rewrite through the single R1 wording helper. Everything else is verbatim.
func runtimeTraceProjOccupierRosterDisplay(roster string, zh bool) string {
	members := strings.Split(roster, "、")
	for i, member := range members {
		idx := strings.LastIndex(member, ":")
		if idx <= 0 {
			continue
		}
		if tid, ok := runtimeTraceCausalProjectionCommTruncatedTid(member[:idx]); ok {
			members[i] = runtimeTraceCausalProjectionUnrecordedThreadText(tid, zh) + member[idx:]
		}
	}
	return strings.Join(members, "、")
}

// runtimeTraceProjFullWindowCoverageTag renders the RN-12 (§7.9) coverage
// cross-reference of a chain/flat row as its tail note: the same-ledger
// full-window per-state total (typed FullWindowStateMS/Source, compile-gated
// at the exact ×1.2 threshold) against the fragment the chain actually shows
// (the row's display projection, ×N merged SUM included), with the coverage
// percentage from precise division. PTV4 T1: a demotable tag — the total,
// its source family and the percentage have no other fence carrier, so it is
// never elided; on width pressure it moves WHOLE onto "· " subordinate
// detail line(s) (T3 wrap keeps tokens like 14.597ms intact). The state word
// comes from the SAME exported class table the compile side used
// (types.TraceCausalProjectionStateClass).
//
// F-2 (统一复核 2026-07-04): the "窗内" wording is reserved for the typed
// same-window verdict (compile-side ±1ms comparison of the carrier's
// selected_window against the projection anchor). A total measured in a
// DIFFERENT query window labels that window explicitly — the recovery
// dual-window shape otherwise rendered "窗内 runnable 合计 2528.721ms" inside
// a 300ms 关注窗 (an arithmetically impossible claim). Totals without window
// endpoints never reach this tag (compile-side 禁猜 drop); the endpoint guard
// here is defensive only.
func runtimeTraceProjFullWindowCoverageTag(node types.TraceCausalProjectionNode, zh bool) (runtimeTraceProjTag, bool) {
	full := node.FullWindowStateMS
	source := strings.TrimSpace(node.FullWindowStateSource)
	if full <= 0 || source == "" {
		return runtimeTraceProjTag{}, false
	}
	class := types.TraceCausalProjectionStateClass(node.StateKind)
	if class == "" {
		return runtimeTraceProjTag{}, false
	}
	covered := runtimeTraceProjNodeDisplayImpact(node)
	if covered <= 0 {
		return runtimeTraceProjTag{}, false
	}
	var text string
	switch {
	case node.FullWindowStateSameWindow:
		text = fmt.Sprintf("窗内 %s 合计 %.3fms(%s),链上仅覆盖 top 片段 %.3fms(%.0f%%)",
			class, full, source, covered, covered/full*100)
		if !zh {
			text = fmt.Sprintf("full-window %s total %.3fms (%s); the chain covers only the top fragment %.3fms (%.0f%%)",
				class, full, source, covered, covered/full*100)
		}
	case node.FullWindowStateWindowStart > 0 && node.FullWindowStateWindowEnd > node.FullWindowStateWindowStart:
		text = fmt.Sprintf("另一查询窗(%.3fs–%.3fs)内 %s 合计 %.3fms(%s),链上仅覆盖 top 片段 %.3fms(%.0f%%)",
			node.FullWindowStateWindowStart, node.FullWindowStateWindowEnd,
			class, full, source, covered, covered/full*100)
		if !zh {
			text = fmt.Sprintf("%s total %.3fms in another query window (%.3fs–%.3fs) (%s); the chain covers only the top fragment %.3fms (%.0f%%)",
				class, full, node.FullWindowStateWindowStart, node.FullWindowStateWindowEnd,
				source, covered, covered/full*100)
		}
	default:
		// Defensive: neither a same-window verdict nor labelable endpoints —
		// a window claim would be a guess, so no note at all.
		return runtimeTraceProjTag{}, false
	}
	// PTV4 T1: never elided/shaved — demotes intact to a subordinate line
	// (T3 wrap keeps tokens like 14.597ms whole).
	return runtimeTraceProjTag{Text: text}, true
}
