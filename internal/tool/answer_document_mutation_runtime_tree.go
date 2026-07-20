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
// node); six edge kinds only (下钻 / 唤醒 / 语义 / 成因 / 自身 /
// 链上·深度未解析); bars are scaled to the requested window when the precise
// anchor exists and deterministically fall back to the batch max otherwise
// (never a fabricated window).

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
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
	// runtimeTraceProjTreeRowCycleFold (PTV8-LAD L1, §24.11 维度A / §24.8
	// 循环梯子病理, 2026-07-08) is the run-length CYCLE fold row: a consecutive
	// k-tuple (k≤3) repeating ≥2 times inside the long-trunk folded middle
	// renders as ONE "↺ 循环×N: A ⇄ B" line — member names in full (用户实体
	// 整名不截 = 重要信息永不省略), children continue from the run end at one
	// extra indent level. The huadong_78 witness rendered the same shape as a
	// 14-row / 14-level zero-information ladder.
	runtimeTraceProjTreeRowCycleFold = "cycle_fold"

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
	// runtimeTraceProjTreeEdgeChainUnresolved (PTV6 #1b, presentation v3 §5)
	// marks the depthless remaining-on-chain lane: an on-chain row whose
	// ChainDepth never resolved. The pre-PTV6 hardcoded default was the WAKE
	// edge — a bare 唤醒 claim hanging off a non-waker (specimen donghu_short:
	// background critical_blocking rows rendered as └─唤醒─ children of the 🎯
	// target). The dedicated edge claims exactly what the data carries —
	// on-chain membership with an unresolved tree position — never a wake
	// relation. Stamped ONCE at model build; fence edge, relation cell and
	// legend entry all read the resulting row.Edge (F2 同型).
	runtimeTraceProjTreeEdgeChainUnresolved = "chain_unresolved"

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
	// runtimeTraceProjTreeIndentCap (PTV8-LAD L4 / AL3, §24.11 维度A + §24.8
	// 重要度分层总则, 2026-07-08) caps the RENDERED ancestor-rail depth of every
	// tree line — main rows and subordinate "· " lines alike (one shared rail
	// builder, two consumers). Beyond the cap the shallowest rails collapse
	// into a fixed 2-cell "⋯ " leader, so the fixed lead is BOUNDED: display
	// geometry stops being an unbounded function of chain depth (the huadong_78
	// ladder pushed rows to w=144 > the 100-cell cap and shredded subordinate
	// payloads into a 20-cell column). Chain-depth semantics are untouched —
	// the 链上L# chip and the detail blocks keep the true depth.
	runtimeTraceProjTreeIndentCap = 12
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
	// runtimeTraceProjSelfWaitRelocateMax (GAP-B G11, §27.5, 2026-07-09)
	// bounds the wait-symptom target-self rows relocated from the ◇/▒ stanzas
	// into the self-state area: the top K by display magnitude move (sorted,
	// deterministic); the remainder keep their stanza seats and the self area
	// discloses their count + single max (有界防洪泛, §24.8 重要信息先行).
	runtimeTraceProjSelfWaitRelocateMax = 4
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
	// CoverageFragmentSecondary (CR-3 修复轮追加件, 2026-07-12; 56643
	// witness): among rows sharing (subject, state class, full-window
	// total), true on every row whose covered fragment is NOT the group
	// max — the RN-12 coverage tag then speaks 另一片段 instead of the
	// (now false) 最大片段. Display wording rank only; values untouched.
	CoverageFragmentSecondary bool
	// CycleTuple (PTV8-LAD L1, §24.11 维度A) carries the cycle-fold row's
	// repeating tuple member names IN FULL (整名不截 — the row exists to
	// disclose exactly these identities; CyclePeriod = len, CycleCount = the
	// consecutive repeat count). Set only on Kind == CycleFold rows.
	CycleTuple []string
	// OmittedHead / OmittedTail carry the first/last two node names of the
	// folded trunk middle (PTV4 T8, pure display upgrade — the names were
	// always in the typed wakeup path). Rendered mid-truncated (T2).
	OmittedHead []string
	OmittedTail []string
	// Badge is the ❶..❺ TOP-5 root-cause badge (1..5; 0 = none): the row's
	// PUBLISHED seat ordinal when it is 1..5 (§29.27.1 徽章跟随席位 —
	// runtimeTraceProjRowSeatBadgeOrdinal is the single authority; every
	// rendered surface of a TOP-5 seat wears its glyph). Assigned ONCE at
	// model build from typed fields only — never a prose judgment, never an
	// LLM signal. The badge is an independent token, NOT a state glyph (the
	// one-state-glyph invariant counts state icons only).
	Badge       int
	EvidenceTag string
	// DrillTargetRendered (PTV8-RCR-B, UXA 域A #25, 2026-07-08): the row's
	// typed DrilldownTarget names a thread that IS rendered as a row of this
	// tree — the tree edge already answers "查上游", so the per-row
	// 睡眠症状→查上游 guidance tag yields (it stays on rows whose upstream is
	// NOT in the tree). Computed once at model build from rendered subjects.
	DrillTargetRendered bool
	// MicroAnchorFoldDepthlessMembers / MicroAnchorFoldRankLo / -Hi (RNB-5B
	// 件⑦ + 修复轮 P1-1/U3, 2026-07-15): fold-pass carriers — how many folded
	// members were Depthless-kind rows (the F8 census counts them by member,
	// they were individually counted pre-fold), and the folded members' rank
	// ordinal range (all-ranked folds only) for the detail block's honest
	// seat-memory note 「根因排序#a~#b(折叠合一)」. Display inputs only.
	MicroAnchorFoldDepthlessMembers int
	MicroAnchorFoldRankLo           int
	MicroAnchorFoldRankHi           int
	// MicroAnchorFoldCrossBoardPeerBoards (修补轮 件F, 2026-07-16): the
	// distinct peer-board anchor labels of the folded members' cross-board
	// mutual pointers — the members' own 件3 sentences leave with their rows,
	// so the fold row speaks ONE representative note 「本折叠行内成员被另板席
	// 互指(板锚 …)」 (诚实不湮灭; the reverse refs resolve through the fold
	// bracket). Wording input only.
	MicroAnchorFoldCrossBoardPeerBoards []string
	// SelfRunnableTwoRuler (RULER2-1, §29.150② / R-19-b, 2026-07-19): the
	// self runnable two-ruler accounting record stamped onto its LEAD seat
	// row (runtimeTraceProjStampSelfRunnableTwoRuler — unique typed host
	// match or nothing); the 行2 按两把尺记账 cross-row sentence renders from
	// it. Wording input only. nil on every non-lead row.
	SelfRunnableTwoRuler *types.TraceCausalProjectionSelfRunnableTwoRuler
	// SelfTwoRulerParticipantRank (DISPHYG-3 件6, §29.158 P3-2, 2026-07-20):
	// the two-ruler record's NON-lead participant board ordinal stamped onto
	// this rendered ordinal-less compact row (unique typed host match or
	// nothing — see runtimeTraceProjStampSelfTwoRulerParticipants). Wording
	// input only: the row-tail 根因排序#N cross-reference chip reads it.
	SelfTwoRulerParticipantRank int
	// SelfWallClockQualifier (RNB-5B 默认小件c, §29.95 UX-4 对称, 2026-07-15):
	// this self-stanza cause seat wears the 「自身·墙钟席」 Row2/◎ qualifier
	// even though its lane was not minted by the SELF-ALL basis arm (family
	// io seats / satellites — see runtimeTraceProjStampSelfWallClockQualifiers).
	// Wording input only.
	SelfWallClockQualifier bool
	// SelfQualifierForeignSubject (XLANE-1 件3 词面 rider, §29.104.2 定谳⑤,
	// 2026-07-15): the row's canonical Subject provably differs from the
	// projection tree's target — the 「自身·墙钟席/自身·确定性优化」 qualifier
	// words are target-exclusive (自身 = 分析目标) and NEVER render on such a
	// row (witness runnable2 E29/E32: shadowhook rows minted as legitimate
	// self seats in a shadowhook-target query step wore 自身·墙钟席 after
	// fusing into the ease.cloudmusic tree). The upstream OnChainBasis mint
	// is innocent (§29.104.2 任务④ located it) — only the display wearing
	// gate forks on this flag. Stamped at model build by
	// runtimeTraceProjStampSelfQualifierSubjectGate (canonical key
	// comparison; empty subject or flat/target-less mode stamps nothing —
	// fail open to the legacy wearing). Wording input only.
	SelfQualifierForeignSubject bool
	// RankWindowChipNoEndpoints (RNB-5B 件⑨, §29.96.2 终判⑨, 2026-07-15):
	// the stamped chip is the endpoint-less 多窗(端点见明细) form — a
	// multi-window merged seat whose chip window is typed-unresolvable states
	// the multi-window fact without guessing endpoints. Mark routing input
	// only (the chip text renders verbatim on both faces).
	RankWindowChipNoEndpoints bool
	// RankWindowChip (PTV8-RCR-C, §24.13 裁定二后半, 2026-07-08): the seat's
	// query-window identity tag ("窗X–Ys"), stamped at model build ONLY when
	// rank seats from ≥2 typed query windows render in this report (multi-board
	// #1×2 collision) AND this row carries its own typed window. "" = the
	// single-board form and window-less rows stay byte-identical (窗身份
	// typed, absence never guesses).
	// XLANE-3 件2 (§29.104.2 定谳③, 2026-07-16): the chip additionally
	// carries the board-anchor half (·板锚 <target>) when the row's window
	// hosts ≥2 distinct board targets, and the params half (·参数#<fp>) when
	// its (window, target) hosts ≥2 distinct fingerprints — the typed board
	// identity triple spelled out exactly where each half disambiguates.
	RankWindowChip string
	// RankBoardAnchorChip / RankBoardParamsChip (XLANE-3 件2): the stamped
	// chip carries the board-anchor / params half — legend routing inputs
	// only (the wearing site marks the matching legend entries).
	RankBoardAnchorChip bool
	RankBoardParamsChip bool
	// CrossBoardFamilyRefs / CrossBoardFamilyMoreCount /
	// CrossBoardFamilyPeerBoards (XLANE-3 件3, §29.104.2 定谳③, 2026-07-16):
	// the cross-board same-thread same-state-family mutual pointer — this
	// seat's physical thread holds same-family seats on OTHER boards (typed
	// triple identity). Refs = the peer seats' evidence tags (render order,
	// cap 2; MoreCount = the honest remainder); PeerBoards = the distinct
	// peer board anchor labels. Values untouched everywhere — the row speaks
	// ONE disclosure sentence 「同线程同状态族账另见另板席…不可跨板相加」 both
	// directions. Stamped by runtimeTraceProjStampCrossBoardFamilyNotes;
	// wording inputs only.
	CrossBoardFamilyRefs       []string
	CrossBoardFamilyMoreCount  int
	CrossBoardFamilyPeerBoards []string
	// SeatOrdinalStale (UXR-1 复核 P2-3, 2026-07-11): the row's displayed
	// ordinal EXCEEDS its own channel's rendered population — a stale
	// persisted artifact form (an old-engine GLOBAL ordinal note replayed
	// under the §29.36.2 per-channel spaces would otherwise be re-worded as a
	// fresh channel ordinal, e.g. global rank=9 rendered 邻近影响#9 on a
	// one-row ◇ stanza). Fail-close: the seat chip and the detail seat line
	// drop (symmetric with the background channel's stale-Rank fail-close);
	// the row itself, its tier and its confidence stay rendered. Stamped once
	// at model build; display-only — no gate/sort lane reads it.
	SeatOrdinalStale bool
	// MentionFloorOnChain / MentionFloorTopN (UXR-1 §29.36.3 通道4 提及义务,
	// user ruling 2026-07-11): this ✦ row is an ON-CHAIN semantic row WITHOUT
	// a channel-1 seat — the SEM-LEAD mention floor as an explicit channel
	// member. Its 行2 speaks the typed obligation word 「优化点·未入根因排序
	// 前N」 (N = the rendered chain board size; 0 drops the 前N tail). Stamped
	// once at model build from typed fields (chain relevance + displayed seat)
	// — never a prose judgment; no gate/sort lane reads it.
	MentionFloorOnChain bool
	MentionFloorTopN    int
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
	// UserFocusForced (§22 B1-b F2, huadong_01 audit 2026-07-07): this trunk row
	// sits INSIDE the long-trunk folded middle but names a typed user entity
	// (projection.WakeupPathUserEntityHits — AnchorUserEntities 同源, single
	// comparator at the compile root), so the fold split around it and it
	// renders as its own row instead of vanishing into the "…省略N节点" roster.
	// Data rows render normally (own line numbers / values); no-data transit
	// rows swap the bare 中转 token for the named 用户关注线程(中转) token.
	// Display-only; never a sort or gate input.
	UserFocusForced bool
	// IOFoldPeers carries the same-subject same-segment IO caliber rows folded
	// into this primary row (NEW-3, §7.6 对比场景客户回访 2026-07-04): one
	// underlying IO burst published as several near-equal calibers
	// (io_burst_episode + io_wait over overlapping line spans) rendered as four
	// sibling rows. The peers render as one load-bearing caliber note on this
	// row — values + evidence ids all kept; the underlying observations and
	// projection buckets are untouched (display grouping only).
	IOFoldPeers []runtimeTraceProjIOFoldPeer
	// RankFoldPeers carries the same-segment RANK-lane row(s) folded into this
	// chain-lane row (§21/§22 RNB R2, 2026-07-07): the engine publishes ONE
	// inversion-candidate segment through two lanes (root_cause_rank +
	// wakeup_causal_impact) and the tree rendered both as sibling/cause rows.
	// The chain row keeps the tree position/edge semantics; the rank row's
	// rank badge / confidence transfer into 行2 (runtimeTraceProjCause-
	// RankConfidence), its E# stays registered on the evidence index, and its
	// numerator-relevant magnitudes ride the invariance carriers below so the
	// coverage arithmetic is byte-identical to the two-row render (显示≠归因
	// red line). TypeWord is carried but currently has NO word-face consumer:
	// its only reader (the retired fold note) was removed in aabccb6f — the
	// field stays per the §29.40 OM-6 ruling (A 臂), and its 「链上并入」
	// word-face arm pends the v5 P2c batch (P2c 词面臂待接; info-contract
	// census tracks it as known_gap OM-6). Display grouping only — the
	// projection buckets and the rank funnel are untouched.
	RankFoldPeers []runtimeTraceProjRankFoldPeer
	// ActualScope is the CR-2 组③ P7 typed scope verdict for this row's actual
	// channel (runtimeTraceProjActualWindowScope against the model's analysis
	// window), stamped once at model build — the ⚠/实际 word faces read it so
	// a value-only overshoot can never mint the 跨出分析窗 claim (冷读案19).
	// Zero (None) on hand-built rows and rows without an actual overshoot.
	ActualScope runtimeTraceProjActualScope
	// SameSegMirrorPeers carries the raw-state (root_evidence lane) copies of
	// this row's segments folded in by the CR-2 P5 MEMBER arm (WO-D1①,
	// legacy lane since v5 P1 件① retired the equality arm to the engine):
	// E# merges into the bracket, the raw state word takes the 行2 状态 slot
	// when this row lacks one, and the 行2 wears the typed 同段镜像 tag.
	// Annotation only — never an ms account.
	SameSegMirrorPeers []runtimeTraceProjSameSegMirrorPeer
	// ValueMirrorRef marks this un-merged AGGREGATE-lane row as the µs-equal
	// value mirror of exactly one ×N merged candidate row (修复轮 C-2/A1,
	// 冷读 tieba E6/E18, 2026-07-12: one physical five-segment runnable time
	// published as a ×5 candidate row AND an aggregate reference row — two
	// bare 23.748ms rows read additively). Carries the candidate row's E#
	// for the 行2 mirror tag; accounts/values untouched on both rows.
	ValueMirrorRef string
	// FamilyMirrorRef / FamilyMirrorSegMin/MaxMS mark this MERGED row as the
	// same-segment twin of a family row carrying CAL-1 segment truth (CR-2 P5
	// family arm, F-1 残口: donghu E8/E9 — the critical_blocking ×4 twin's
	// members are per-CPU group SUMS, so its 「单次 a~b」 claim was false).
	// Ref = the family row's evidence tag; SegMin/Max = the family's TRUE
	// single-segment extrema propagated for the twin's honest range wording.
	// Display wording only; no gate/sort lane reads these.
	FamilyMirrorRef    string
	FamilyMirrorSegMin float64
	FamilyMirrorSegMax float64
	// SelfSymptomFoldPeers carries a target_self_state rank-lane view that was
	// proven to describe the same focused-thread scheduler-state segment as
	// this self row.  The state row remains the sole display/accounting seat;
	// the peer contributes only its evidence id and the explicit symptom note.
	// This is deliberately separate from RankFoldPeers: target_self_state has
	// no board ordinal and must never acquire rank-fold accounting carriers.
	SelfSymptomFoldPeers []runtimeTraceProjSelfSymptomFoldPeer
	// SelfSymptomRelocated (GAP-B G11, §27.5, 2026-07-09): this SelfRows row
	// is a wait-symptom target-self rank row RELOCATED from the ◇/▒ stanza
	// buckets into the target's own state area (typed tier target_self_state
	// + canonical subject == 🎯 target). It renders with the symptom
	// disclosure note (症状而非根因 — the sleep-row family wording); it never
	// gains a rank seat by moving (the rank board and the symptom denominator
	// read typed node fields, not the row's stanza).
	SelfSymptomRelocated bool
	// AbsorbedChainPeers (G1 跨车道对账 display half, §27.2-G1, 2026-07-09)
	// carries the chain-lane critical_blocking observations the ENGINE
	// absorbed into this family row (projection.AbsorbedChainRows joined by
	// verbatim RankFamilyKey == AbsorbedInto equality). The compile relocated
	// their bucket seats, so they never render as parallel tree/stanza rows;
	// their E# stay registered on the evidence index and the family detail
	// stanza prints the 链上并入 disclosure with the E# list — 信息守恒:
	// evidence index / system supplement (raw observations untouched) / audit
	// tokens all lossless. Attached once per family key (deterministic first
	// rendered family row).
	AbsorbedChainPeers []runtimeTraceProjAbsorbedChainPeer
	// NonAdditiveRef / NonAdditiveKind (WO-A1, SMR-1 批, smr_audit_report §④,
	// 2026-07-12) is the unified「不可相加/包含」cross-seat pointer arm — the
	// §29.50.1 过渡候选反向指针 generalized to three carriers (self seat /
	// aggregate↔member / cross-lane addition identity). Typed judgment only
	// (registry state family + µs value relations); the word face is minted by
	// the ONE template (runtimeTraceProjSameSegMirrorTagTexts). 过渡臂盘点
	// (v5 P1 件① 落地, 2026-07-13): the pointer's carriers are NESTED-account
	// shapes (component ⊂ seat / member ⊂ aggregate) — not duplicate seats of
	// one segment set, so the engine one-seat mint (equal-fingerprint scope,
	// B.2) does not cover them by definition. The arm stays; 退役条件 moves
	// to a dedicated containment adjudication (CASE-1 扩围 / CASE-3).
	NonAdditiveRef  string
	NonAdditiveKind runtimeTraceProjNonAdditiveKind
	// SelfGapSemanticOverlapClauses (XLANE-2 件2, 裁定④ 披露式拆分,
	// 2026-07-17): the self-gap seat's RESOLVED semantic-overlap clauses —
	// per partner the typed interval-intersection wall clock + the partner
	// row's evidence tag (resolved by verbatim line-envelope identity at
	// model build, runtimeTraceProjStampSelfGapSemanticOverlaps). Drives the
	// row-level 「其中 X ms 与语义席[E#]重叠」 line only; 主值零动.
	SelfGapSemanticOverlapClauses []runtimeTraceProjSelfGapOverlapClause
	// CrossDirectionOverlapClauses (AXIOM-V2 件2, 公理 v2 user rulings
	// 2026-07-18): the RESOLVED cross-direction mutual clauses — per partner
	// the typed interval-intersection wall clock + the partner row's evidence
	// tag + the partner's fix direction (resolved by verbatim line-envelope
	// identity at model build, runtimeTraceProjStampCrossDirectionOverlaps;
	// BOTH seats of a pair resolve or NEITHER renders — 宁漏勿假指). Drives
	// the row-level 「与[E#](修向 X)同段重叠 Y ms…收益不叠加」 line only;
	// 主值零动.
	CrossDirectionOverlapClauses []runtimeTraceProjCrossDirectionClause
	// SemanticMemberSubsetOf (XLANE-2 件1, §29.104.1/.2 定谳④, 2026-07-17):
	// the superset seat's evidence tag when this semantic family seat's
	// COMPLETE typed member line-range set is a PROPER SUBSET of a same-board
	// same-subject semantic seat's set (engine member_line_ranges only —
	// prose/name matching forbidden; incomplete sets never judge, 行号缺席
	// fail-open 保原状). Doubles as the ◎ exclusion verdict: the row leaves
	// the ◎ population/semantic census into the dedicated subset footnote.
	// The word face rides the NonAdditive pointer (kind MemberSubset); value
	// channels and engine ordinals stay untouched (降道=席位口径变化非值变化).
	SemanticMemberSubsetOf string
	// SubordinateComponentSeat (P2a rider 件2b, §29.58.1 b, 2026-07-13): this
	// SELF row is a typed COMPONENT of another self row (WO-A1 carrier a —
	// the binder ⊂ sleep carve) and was RESEATED directly under its owning
	// seat by runtimeTraceProjSeatSelfComponentRows; the renderer prefixes
	// the ↳ subordinate connector (结构管关系 — the containment used to live
	// only in the pointer word while the row sat同级 among the state rows).
	SubordinateComponentSeat bool
	// MergedTwinMirrorRef (WO-D3 短期臂, SMR-1 批 S3-TPF/S8-TPF, 2026-07-12)
	// marks this ×N MERGED row as the same-source twin of exactly one OTHER
	// ×N merged row with the identical member fingerprint (µs display + count
	// + member extrema + query window) — the double-merged shape all three
	// pre-SMR mirror arms structurally missed. Mutual (both rows point);
	// tag-only, accounts untouched. v5 P1 件① 盘点 (2026-07-13): the
	// witnessed carriers converge at engine arm C
	// (traceCausalProjectionConvergeMergedTwinSeats) and no longer reach this
	// layer; the tag stays for the engine's deliberate fail-open pairs
	// (diverging cum/eff = W-A, ranked seats, ⌗-side, ≥3 copies).
	MergedTwinMirrorRef string
	// BranchTwinFoldPeers (WO-D2/D4, SMR-1 批 S2-TPF/SMR-S4, 2026-07-12)
	// carries the flat「父节点未确认」aggregate copy folded into this
	// trunk-attached aggregate (typed full-equality fingerprint: subject +
	// state + ×N count + member extrema + display + query window; trunk keeps
	// the seat = 已解析链位信息严格更多). The peer's E# joins the bracket and
	// its diverging effective-attribution caliber is disclosed VERBATIM
	// (eff 双列 — D4: fold must dual-list both occurrence accounts).
	BranchTwinFoldPeers []runtimeTraceProjBranchTwinFoldPeer
	// AccountRelRef / AccountRelOwn / AccountRelPeer (WO-C1, SMR-1 批
	// SMR-S6/S15/S1-TPF 过渡, 2026-07-12): the C-type account-relation
	// sentence — W-A's「双行存续」ruling gains its missing reason half. Three
	// prohibitions (S6 vnote): never「同段」, never a coverage-direction pair
	// (全窗 vs 发生段 hints an ≥ that is FALSE on the witness), never a
	// quantified overlap ms (cross-lane ts inventory unbuilt — typed
	// unprovable). 过渡: the S1-TPF pair keeps this sentence until CASE-3
	// adjudicates its dedicated arm.
	AccountRelRef string
	AccountRelOwn string
	// AccountRelDisjoint (修复轮三 R2-F2, 冷读 witness tieba E4↔E25,
	// 2026-07-13): the pair's typed occurrence hulls are PROVABLY disjoint
	// (hull-disjoint ⇒ member-disjoint, precise) — the sentence speaks
	// 「物理时间不相交」 instead of the overlap template (SMR 行级判定必须
	// 论证成员级可达性; an unconditional 重叠 claim on partition-sibling
	// seats was false). Hull overlap or missing ts stays the existing
	// overlap wording (hull noise cannot prove member overlap — fail-open
	// to the current template, 禁量化重叠 ms unchanged).
	AccountRelDisjoint bool
	AccountRelPeer     string
	// AccountRelSameSourceFullMS / AccountRelSameSourceAnchoredSide (RSPA
	// §29.61.10b, 2026-07-14): the same-source bipartition relation pair — a
	// migrated window state seat split into the ⛓ credential-anchored half and
	// the ◇ remainder half over ONE segment set (typed engine fields
	// Node.ChainAnchoredMS/ChainAnchorFullMS/ChainAnchorRemainderSeat). When
	// FullMS > 0 the AccountRel sentence renders the RSPA same-source template
	// (合计还原全窗账 — the ONLY additive seat relation: anchored + remainder
	// == full exactly, same-source disjoint bipartition) instead of the
	// two-accounting-systems form; AnchoredSide selects which half speaks
	// 本行. W-A stays untouched for every other pair (wall clock across
	// different accounts remains non-additive).
	AccountRelSameSourceFullMS       float64
	AccountRelSameSourceAnchoredSide bool
	// ChainAnchorTwinInvisible (RNB-1 D1 修复轮, §29.88 复核, 2026-07-14): the
	// 行2 bipartition disclosure names the OTHER half of the split (「(⛓链上
	// 席)」/「(◇余段席)」/divergent 「链席另列自账」). When the twin did not
	// survive to any rendered surface (capacity truncation / bucket fold) the
	// claim would dangle — the visibility sweep (runtimeTraceProjStamp-
	// ChainAnchorTwinVisibility, same row universe as the smr1 pairing pass)
	// stamps this bit and the sentence downgrades to the honest 未上榜/见明细
	// form. Stamped only on rows carrying the decomposition pair.
	ChainAnchorTwinInvisible bool
	// AccountRelMirror* (件1 修复轮, 2026-07-14; donghu E12(+3) witness): the
	// full-window MIRROR row relation — an UNDECOMPOSED same-thread same-state
	// row (another lane's face, e.g. critical_blocking) whose display value
	// equals the bipartition pair's full account at 3 decimals. The mirror row
	// carries both halves' refs and speaks 「同段镜像·全窗账=[⛓]+[◇] 二分席之
	// 和,不可与二分席相加」; each half carries the back-pointer ref. Typed
	// value identity only (3dp equality against Node.ChainAnchorFullMS); ≥2
	// candidate pairs fail open.
	AccountRelMirrorAnchoredRef  string
	AccountRelMirrorRemainderRef string
	AccountRelMirrorRef          string
	// OccurrenceSeries* (WO-B1, SMR-1 批 SMR-S8/S10/S11, 2026-07-12): the
	// B-type same-identity multi-occurrence short note — typed provably
	// DISJOINT same-(thread, state, object, type, window) single rows carry
	// their own occurrence interval, the sibling refs and the series total
	// (disjoint same-identity segments are the ONE legitimately additive
	// shape; §29.50.4③ 合计参赛指针 — the total rides the pointer word since
	// no clean total seat exists to point at, 禁虚指 E#). 改词保双席: the
	// note never removes a seat.
	OccurrenceSeriesRefs    []string
	OccurrenceSeriesTotalMS float64
	OccurrenceSeriesCount   int
	// CoverageMergedTwinCount (WO-A1 词面统一 复放追修, 96717 E12/E15 形,
	// 2026-07-12): this UNMERGED row's covered value is typed-provably an
	// engine-side ×N total — its same-span W-A twin is a MergedCount>1 row
	// with the µs-identical display, or a same-(subject,state) occurrence
	// series' additive total µs-equals it. The RN-12 coverage word then
	// speaks 链上覆盖合计(×N) instead of the false single-fragment claim.
	// Display wording input only.
	CoverageMergedTwinCount int
	// OverflowProjectionRef (P2-2 跨口径穿透, 2026-07-13): the resolved E# of
	// the rendered row whose account the pool's contents PROJECT (typed
	// Node.OverflowProjectionEvidenceID resolved post-build). Wording input
	// only — 「同一物理时间的口径投影·与[E#]不可相加」.
	OverflowProjectionRef string
	// OverflowProjectionHumble (P2-2 谦逊注 fallback, 2026-07-13; 29424 复放:
	// the WIRE-side fold (folded_* notes) publishes min/max but no Σ, so the
	// cross-caliber projection identity is typed-UNREACHABLE there — the
	// coordinator's fallback is an advisory humble note, never a hard tag):
	// true when the ref-less fold's subject also holds rendered same-family
	// seats. SOFT wording only (heuristic gate is legal for soft guidance);
	// names NO specific E# (an unprovable pointer would be 指错). Typed-sum
	// reachability (folded_sum note / CASE-1 组值库存) retires this arm.
	OverflowProjectionHumble bool
	// OverflowMirrorRefs (WO-D1③ 多引用 tag, SMR-1 批 SMR-S9, 2026-07-12):
	// the resolved E# tags of the rendered rows whose physical time the
	// overflow fold's headline re-publishes (typed
	// Node.OverflowMirrorEvidenceIDs resolved post-build). Wording input only.
	OverflowMirrorRefs []string
	// SelfNonChainSeat (SELF-LANE §29.58.3 处置 a, 2026-07-13): this
	// target-subject row was RELOCATED out of the ◇ 邻近区段 into the self
	// stanza — display placement only. Its typed channel identity stays
	// adjacent (node.ChainRelevance untouched — chip word, ordinal channel,
	// caliber words all read the node), and the renderer adds the 「非链」
	// qualifier so the self stanza never implies an on-chain proof it lacks.
	SelfNonChainSeat bool
	// CrossChannelChainRef / CrossChannelAdjacentRef (SELF-LANE §29.58.3 处置
	// b, 2026-07-13): the cross-channel same-thread mutual pointers (SMR-1
	// relation-sentence family) — an adjacent-channel row whose thread also
	// holds an on-chain seat points at that thread's LARGEST on-chain seat
	// (「本线程另有链上席 [E#]」), and that on-chain seat points back at the
	// thread's largest adjacent-channel seat (「本线程另有邻近席 [E#]」), so a
	// reader can always tell deliberate segmentation from duplication.
	// Wording input only; accounts untouched.
	CrossChannelChainRef    string
	CrossChannelAdjacentRef string
	// CrossChannelCaliberRef (RNB-5B 件②, §29.96.2 终判②, 2026-07-15): the
	// chain seat's pointer at the thread's largest ⌗ side-rail row (typed
	// self_caliber_side) — 「本线程另有口径旁栏行 [E#]」: the former 邻近席
	// word claimed a channel seat the token retires. Wording input only.
	CrossChannelCaliberRef string
	// BlockingWaitSleepRef / BlockingWaitSleepPeerRef (XERR1-FIX 件1 互指,
	// §29.104.4; E6/E7 账目关系先例): a converged payload-less blocking_span
	// row (basis wait_segments) whose Σ carries a SLEEP component shares that
	// physical sleep time with the thread's own sleep-family window seat —
	// the two accounts cross-reference (never invite addition). Ref = the
	// sleep seat's E# on the blocking row; PeerRef = the blocking row's E# on
	// the sleep seat. Wording input only; both values untouched.
	BlockingWaitSleepRef     string
	BlockingWaitSleepPeerRef string
	// GatedShareClaimRefs (LEVELMERGE-1 件2, 2026-07-18): the resolved [E#]
	// tags of the claiming priority-inversion seat(s) — resolved from the
	// typed GatedShareClaimSeats line intervals all-or-nothing (any span
	// unresolvable/ambiguous → the slice stays empty and the 行2 sentence
	// keeps the generic 本线程反转席 noun, 宁漏勿假指). Wording input only.
	GatedShareClaimRefs []string
	// AggregateMemberRefs / AggregateSeatRef (LEVELMERGE-1 件3 两向互指,
	// 2026-07-18): the aggregate-seat ↔ member-occurrence pointer pair —
	// MemberRefs rides the seat row (构成段见[E#…], all-or-nothing: every
	// member resolved or none), SeatRef rides each member view row
	// (归因已计入[E#](聚合席)). Wording inputs only; accounts untouched.
	AggregateMemberRefs []string
	AggregateSeatRef    string
	// marks is the NEW-7 emission collector for this render pass. The fence
	// renderer stamps model.Marks onto its per-row COPIES right before calling
	// the row-render helpers, so every mark is recorded AT the emission site
	// (nil-safe: width-pass rows and test-constructed rows carry nil and record
	// nothing). Never set by the model builder.
	marks *runtimeTraceProjMarkSet
	// ValueSlot (DISPLAY-HYG 二轮 复核件1, catalog C3 错位行, 2026-07-17) is
	// the fence-wide shared ms value-cell content width. The family-stem arm
	// (合计X.XXXms) can exceed the legacy 11-cell " %9.3fms" slot (合计 4 +
	// 6-char float + ms = 12 — witness 101345:306 / 092738:184 the % column
	// drifted right by 1 on exactly those rows), so the fence builder
	// pre-computes max(11, widest family-stem cell) over the rendered row set
	// and stamps it here — every arm then pads to ONE column and the ms
	// tails align. 0 (width-pass rows, direct test constructions) means the
	// legacy 11-cell slot, byte-identical.
	ValueSlot int
}

// runtimeTraceProjNonAdditiveKind is the WO-A1 pointer's typed direction word
// selector — the word face forks on it inside the ONE template function.
type runtimeTraceProjNonAdditiveKind int

const (
	runtimeTraceProjNonAdditiveNone runtimeTraceProjNonAdditiveKind = iota
	// runtimeTraceProjNonAdditiveComponent — this row's account is a typed
	// COMPONENT of [ref]'s account (addition identity X=Y+Z with a typed
	// complement, or the structural binder⊂sleep self carve).
	runtimeTraceProjNonAdditiveComponent
	// runtimeTraceProjNonAdditiveContains — the symmetric high-seat face:
	// this row's account already CONTAINS [ref]'s.
	runtimeTraceProjNonAdditiveContains
	// runtimeTraceProjNonAdditiveMember — this single-occurrence row's value
	// is µs-identical to a member of [ref]'s ×N aggregate (derivable member
	// multiset) — the aggregate already counts it.
	runtimeTraceProjNonAdditiveMember
	// runtimeTraceProjNonAdditiveMemberSubset (XLANE-2 件1, §29.104.1/.2
	// 定谳④, 2026-07-17) — this semantic family seat's COMPLETE typed member
	// line-range set is a PROPER SUBSET of [ref]'s (same board, same subject;
	// engine member_line_ranges only — prose/name matching forbidden): the
	// witness E34=E35∪E49 cross-step double-mint family. The row demotes out
	// of the ◎ population/census (dedicated footnote) with values and engine
	// ordinals untouched.
	runtimeTraceProjNonAdditiveMemberSubset
)

// runtimeTraceProjBranchTwinFoldPeer is one folded flat aggregate copy: its
// registered evidence tag plus the diverging eff caliber for the dual-listing
// disclosure (zero when the peer published none — absence never invents).
type runtimeTraceProjBranchTwinFoldPeer struct {
	EvidenceTag       string
	EffectiveImpactMS float64
}

// runtimeTraceProjIOFoldPeer is one folded same-segment IO caliber: the raw
// typed token, its display impact and its registered evidence tag.
type runtimeTraceProjIOFoldPeer struct {
	Token       string
	ImpactMS    float64
	EvidenceTag string
}

// runtimeTraceProjAbsorbedChainPeer is one engine-absorbed chain-lane
// observation attached to its family row (G1, §27.2-G1): the registered
// evidence tag is the stanza disclosure's payload — the values themselves
// already live inside the family row's combined account (raw sums strictly
// equal by the engine's membership proof), so the note never re-prints an ms
// that could be double-read.
type runtimeTraceProjAbsorbedChainPeer struct {
	EvidenceTag string
}

type runtimeTraceProjTreeModel struct {
	Target string
	// RankBoardEffSumMS (冷读扩臂④ 板级警示, SMR-1 修复轮 2026-07-13): the
	// Σ of the rank-seated rows' effective attributions when it EXCEEDS the
	// analysis-window length (typed precise: only over-window mints the
	// field; 96717 复放: 七席 Σ=120.528ms > 窗 114.940ms with no board-level
	// sentence). Drives ONE tree-head line 「席位间物理时间可重叠,不可直接
	// 相加」; never a gate/sort input. 0 = silent (within window).
	// XLANE-3 件3 (§29.104.2 定谳③, 2026-07-16): the Σ is PER BOARD (typed
	// triple identity via runtimeTraceProjStableRankBoardIDs) — the field
	// holds the largest single board's over-window Σ, never a cross-board
	// addition. RankBoardEffSumMultiBoard=true = the report renders ≥2 boards
	// and the head sentence scopes its claim to 同板 (single-board reports
	// keep the legacy wording byte-identically).
	RankBoardEffSumMS         float64
	RankBoardEffSumMultiBoard bool
	// BusinessSpanMentions (SPANVIS-1, user ruling 2026-07-19 定形原则): the
	// pure-advisory business-span mention rows — verbatim typed transports of
	// the projection side channel (strict-parsed upstream; the model build
	// re-validates the display gates). Rendered as the ◈ non-ordinal advisory
	// block at the tree fence tail and as the ◎ overview's business-lead
	// footnote; NEVER a row model — structurally invisible to every board /
	// census / conservation / ordinal population.
	BusinessSpanMentions       []types.TraceCausalProjectionBusinessSpanMention
	BusinessSpanMentionOmitted int
	// GatedCompositeEdgeShareDisclosures (PARTSPLIT-1, §29.150④): the
	// R4-mirror refusal NON-SEAT disclosure side channel — verbatim typed
	// transport (render re-validates the X+Y==account identity); consumed by
	// the ◎ chain-section mention block only. NEVER a row model.
	GatedCompositeEdgeShareDisclosures []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure
	// SelfRunnableTwoRulerAccountings (RULER2-1, §29.150②): the self
	// runnable two-ruler accounting side channel — verbatim typed transport
	// (render re-validates both same-ruler Σ identities); consumed by the
	// 行2 按两把尺记账 sentence on the stamped LEAD seat row only. NEVER a row
	// model, never a census/conservation input.
	SelfRunnableTwoRulerAccountings []types.TraceCausalProjectionSelfRunnableTwoRuler
	TreeRows                        []runtimeTraceProjTreeRow // trunk + attached (flattened, render order)
	SelfRows                        []runtimeTraceProjTreeRow // target's own state rows (under root)
	Adjacent                        []runtimeTraceProjTreeRow
	Background                      []runtimeTraceProjTreeRow
	WindowMS                        float64 // >0 = window mode; 0 = fallback (BarMaxMS denominator)
	// WindowStartTs/EndTs are the analysis-window endpoints behind WindowMS
	// (CR-2 组③ P7): the ⚠ containment gate compares a row's typed actual
	// interval against THESE endpoints — 「实际状态跨出分析窗」 is a claim
	// about the analysis window, never about the row's own occurrence
	// sub-window. Zero in fallback mode (no window claim, no ⚠ — RN-2b).
	WindowStartTs float64
	WindowEndTs   float64
	BarMaxMS      float64
	// BarScaleWallClockAnchored (DISPHYG-3 件3, §29.155 P2 残形, 2026-07-20):
	// true when BarMaxMS was anchored by a wall-clock row (the normal shape);
	// false = the fail-open lane (every valued row cross-thread-aggregate or
	// non-wall-clock), where the windowless scale sentences fork to the
	// honest no-ruler wording. Wording input only — BarMaxMS itself is
	// byte-identical on every shape.
	BarScaleWallClockAnchored bool
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
	// TargetUserElected mirrors the typed compile-side B1 anchor election
	// (TraceCausalProjection.WakeupPathUserElected, §12.3 裁定3): the 🎯 root
	// IS a user-entity thread because a typed user-entity match ELECTED its
	// wakeup path at compile time. The R2 comparison short-circuits on it so
	// the ‹用户关注线程› label can never disagree with an entity-elected anchor
	// even when the renderer-side entity list diverges from the compile-side
	// one (e.g. the election came from a frame_target_resolution
	// explicit_query_target subject or a runtime_targets pid absent from
	// AnalyzerHints — the Q4-E entity-starvation family).
	TargetUserElected bool
	// RootFocusUserEntities lists the user's thread/pid-shaped entities for the
	// anchor-only explanation line (display-only roster; empty = no note line).
	// Shared by the 🎯 anchor-only note (R2) and the flat-fallback anchor note
	// (RN-13(a)) — the two lanes are mutually exclusive on model.Target.
	RootFocusUserEntities []string
	// TargetUserAliasEntity (PTV8-RCR-C, §24.12 C11 同 tid 双名归一声明,
	// 2026-07-08): the user entity that names the SAME tid as the ⊚ root under
	// a different display name (com.xs.fm.lite-6565 vs main-6565). Non-empty
	// only when the R2 comparison matched by tid with differing name halves —
	// the tree-note lane then declares the normalization. "" = no note.
	TargetUserAliasEntity string
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
	// SoloArtifact (PTV8-LAD L7, §24.14 补2 D-4 单工件面, 2026-07-08) marks the
	// single-artifact render lane: set from the cluster builder's own id-prefix
	// fact (the legacy base prefix = the one-projection cluster), never
	// inferred from labels. It gates ONLY the tree-head ±10% user-window
	// deviation line — the comparison face carries its own folded D-4 note
	// (COV-2), so the per-side tree heads must not repeat it there (批名即界).
	SoloArtifact bool
	// SelfWaitOverflowCount / SelfWaitOverflowMaxMS (GAP-B G11, §27.5,
	// 2026-07-09): how many wait-symptom target-self stanza rows stayed in
	// their ◇/▒ seats after the bounded top-K relocation into SelfRows, and
	// the largest single display magnitude among them — the self area's
	// overflow disclosure line reads both (有界防洪泛 + 永不静默丢: the rows
	// themselves keep their stanza seats and detail entries).
	SelfWaitOverflowCount int
	SelfWaitOverflowMaxMS float64
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
	runtimeTraceProjMarkRootTarget                runtimeTraceProjMark = iota // 🎯 root header
	runtimeTraceProjMarkEdgeDrill                                             // ├─下钻─ edge
	runtimeTraceProjMarkEdgeWake                                              // ├─唤醒─ / └─唤醒─ edge
	runtimeTraceProjMarkEdgeCause                                             // ├─成因─ edge
	runtimeTraceProjMarkEdgeOwn                                               // ├─自身─ own-process caliber edge (F2)
	runtimeTraceProjMarkEdgeChainUnresolved                                   // └─链上·深度未解析─ depthless on-chain edge (PTV6 #1b)
	runtimeTraceProjMarkSemanticSpan                                          // ├─语义─ edge + ✦ icon (always paired)
	runtimeTraceProjMarkIconSleep                                             // ☾ state icon (PTV4 T5: was 💤)
	runtimeTraceProjMarkIconRunnable                                          // ⧖ state icon (PTV4 T5: was ⏳)
	runtimeTraceProjMarkIconRunning                                           // ⚙ state icon
	runtimeTraceProjMarkIconDState                                            // ⛓ state icon
	runtimeTraceProjMarkIconTransit                                           // ◦ 中转 transit icon (PTV4 T4: the two ◦ senses split)
	runtimeTraceProjMarkIconNoDominant                                        // ◦ 无主导态 stateless data-row icon (PTV4 T4)
	runtimeTraceProjMarkBadge                                                 // ❶❷❸ TOP-N root-cause badge (PTV4 T6)
	runtimeTraceProjMarkStateLabel                                            // dominant-state / impact-shape tag
	runtimeTraceProjMarkUndrillable                                           // ⊘ missing-wakeup marker (PTV4 T5: was ⛔)
	runtimeTraceProjMarkCrossWindow                                           // ⚠实际Xms cross-window marker (PTV4 T4: data kept, semantics in legend)
	runtimeTraceProjMarkCrossWindowNoActual                                   // ⚠跨窗 value-less cross-window marker (§21 LEAD-SEM 前置 L1: actual 未采集时禁 0.000 假标量)
	runtimeTraceProjMarkRecursOnChain                                         // ↺ small-cycle marker
	runtimeTraceProjMarkChainDepthChip                                        // 链上L# chain-depth chip (PTV4 T9)
	runtimeTraceProjMarkOmitted                                               // …省略N节点… long-trunk fold row (PTV4 T8 roster)
	runtimeTraceProjMarkBarScale                                              // bar full-scale caliber line (PTV4 T7 口径组)
	runtimeTraceProjMarkMergedSum                                             // ×N(a~b) same-kind SUM aggregate (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkMergedDedup                                           // ×N同值 duplicate-publication fold (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkMergedMax                                             // ×N(a~b)取最大 cross-thread fold (PTV4 T4 ×N 三式)
	runtimeTraceProjMarkMergedUnion                                           // ×N(a~b)union cross-query-window union caliber (§11-N2, ×N 第四式)
	runtimeTraceProjMarkMergedWindowMax                                       // ×N(a~b)跨窗取最大 overlapping-query-window MAX caliber (§21 CWD, ×N 第五式)
	runtimeTraceProjMarkMergedMultiWindowNoShare                              // ×N 多窗合并行不显示占窗% (§21.1 CWD-2 ①, huadong E19)
	runtimeTraceProjMarkOverWindowShare                                       // 占窗>100% multi-CPU/multi-span cumulative share (PTV4 T4)
	runtimeTraceProjMarkWholeWindowIdle                                       // 整窗等待 whole-window idle annotation (PTV4 T4)
	runtimeTraceProjMarkInheritedAttribution                                  // 承自归因 inherited-attribution annotation (PTV4 T4)
	runtimeTraceProjMarkIOCaliberNote                                         // NEW-3 同段IO另有…口径 note
	runtimeTraceProjMarkPeriodicSource                                        // VS-1 周期性信号源 tag
	runtimeTraceProjMarkAdjacentStanza                                        // ◇ 邻近 stanza
	runtimeTraceProjMarkBackgroundStanza                                      // ▒ 背景压力 stanza
	runtimeTraceProjMarkImpactCaliberFallback                                 // PTV5 C00 主行回退口径词 (链上累计/有效归因/实际状态/累计(跨线程))
	runtimeTraceProjMarkCoverageLine                                          // PTV5 Q2 树头覆盖行(已归因/未归因)口径
	runtimeTraceProjMarkOnChainOverflowFold                                   // PTV5 PTS → P2a rider 件1: 折叠行(其余N项(折叠),车道在边词,零静默丢弃;链上+区段两车道共用)
	runtimeTraceProjMarkStanzaCrossThreadCum                                  // PTV6-C ruling A ◇/▒ 行 累计(跨线程) 族词
	runtimeTraceProjMarkCandidateShapeClass                                   // PTV6-D (b) 候选影响 类别词降维:行内删,图例承载
	runtimeTraceProjMarkUserFocusTransit                                      // §22 B1-b F2 折叠段内用户关注线程强制展开(中转形态)
	runtimeTraceProjMarkTraceGapBlindSpot                                     // §22 PTV7-SPN F5 trace_gap 数据盲区 行内披露(用户措辞裁定)
	runtimeTraceProjMarkSemanticSourceWindowShare                             // DCS E5 语义行 %基=自身查询窗+「来自查询窗」标注 (§23 H2, cmp_01 E2 83%对锚窗)

	// PTV8-RCR-A (§24.1-§24.3, 2026-07-08). EVOLUTION RECORD: the §21 RNB R1
	// runtimeTraceProjMarkGatedRunnableSubRow (⧖ runnable gated 分量 子行) and
	// the §21/§22 RNB R2 runtimeTraceProjMarkRankFoldNote (同段rank行并入 note)
	// constants are RETIRED here — the four-line cause-node grammar replaced
	// both: the runnable component rides the 行3/拆解子行 lanes and the folded
	// rank row's badge/rank/confidence/E# rise into 行1/行2 (§24.2). The RNB
	// join/guard engine (runtimeTraceProjFoldSameSegmentLaneTwins) is untouched
	// — only the rendering seat changed.
	runtimeTraceProjMarkIconLock           // ⊗ 锁竞争·持锁 glyph (§24.3 影响形态闭集)
	runtimeTraceProjMarkIconInversion      // ⇅ 优先级反转 glyph (§24.3)
	runtimeTraceProjMarkIconInterrupt      // ↯ 中断活动族 glyph (§24.3)
	runtimeTraceProjMarkIconBlindSpot      // ◌ 数据盲区 glyph (§24.3)
	runtimeTraceProjMarkCauseIdentityRow   // 行2 类别·根因排序#N·置信 (§24.1/§24.2)
	runtimeTraceProjMarkEffectiveBreakdown // 行3 有效归因 V = …「=」分解行 (§24.1)
	runtimeTraceProjMarkCaliberFull        // 口径词 全额 (§24.1补 图例强制)
	// R5 (§29.88.12 单基准, 2026-07-15): the two conversion caliber seats
	// (按下游消费核 gated / 按大核满频 fold) unified onto ONE 全域最大核最高频
	// basis word family with ONE legend seat — one fold, one explanation
	// (the counted component and the folded value are the same number).
	runtimeTraceProjMarkCaliberGlobalMaxFmax // 口径词 折算,按全域最大核最高频[,运行频点非最高] (R5/R5b)
	runtimeTraceProjMarkCaliberLowerBound    // 口径词 下界 解释条 (§24.1补 用户问"下界"何意)
	runtimeTraceProjMarkCaliberSingleMax     // 口径词 单次最大(共N次) (§24.2 事件类)

	// PTV8-RCR-B (UXA 横扫批, 2026-07-08): three new gated seats.
	runtimeTraceProjMarkBarScaleFallback        // 时长条回退尺度条 (UXA 域A #13: 窗口未采集分支单独成条,按需出场)
	runtimeTraceProjMarkStanzaDiscount          // ◇/▒ 行 折算 判别词条 (UXA 域A #19: 折算半句拆为独立条目,按需出场)
	runtimeTraceProjMarkEffectiveAttributionTag // 有效归因 词条 (UXA 域A #31: 行内常显 tag 的图例教学点)

	// PTV8-RCR-C (§24.9/§24.12/§24.13, 2026-07-08): two new gated seats.
	runtimeTraceProjMarkChainSeatUnattached // 链上L#(父节点未确认) depthless 三面同词 (§24.12 C6; CAL-1 件④a 更名,旧词 未接入树)
	runtimeTraceProjMarkRankSeatWindow      // 根因排序#N·窗X–Ys 多榜窗标 chip (§24.13 裁定二后半)

	// XLANE-3 件2/件3 (§29.104.2 定谳③, 2026-07-16): three new gated seats.
	runtimeTraceProjMarkRankBoardAnchor      // chip 板锚 <target> 同窗多板区分半 (件2)
	runtimeTraceProjMarkRankBoardParams      // chip 参数#<fp> 同窗同目标参数区分半 (件2)
	runtimeTraceProjMarkCrossBoardFamilyNote // 同线程同状态族跨板互指句 (件3)

	// PTV8-LAD (§24.11 维度A / §24.8, 2026-07-08): one new gated seat.
	runtimeTraceProjMarkCycleFold // ↺ 循环×N: A ⇄ B run-length cycle fold row (L1)

	// RCM-2 (§24.7.1①/§24.10/§24.12 维度A ③, 2026-07-08): the family-merge
	// caliber ladder's three display words (fifth closed-set word + the max
	// fallback + the count form), each with its own on-demand legend entry.
	runtimeTraceProjMarkFamilyTotal     // 口径词 合计(共N段,同线程) — 第五口径词 (D1)
	runtimeTraceProjMarkFamilyMemberMax // 口径词 成员最大(共N段,重叠未拆) (D1)
	runtimeTraceProjMarkFamilyCountSum  // 口径词 计数合计(共N项,同线程) (D1)

	// CAP (§26 C3, 2026-07-08): the capability-fold disclosure words, each
	// with its own on-demand legend entry (括注扩展须配图例条目).
	runtimeTraceProjMarkCaliberDefaultCapability  // 括注 按默认算力比粗算 (§26 默认表披露)
	runtimeTraceProjMarkCaliberFreqOnlyCapability // 披露 簇结构不可判,按纯频率比折算 (§26 fail-loud 退回)
	// TOMBSTONE (R5 §29.88.12, 2026-07-15):
	// runtimeTraceProjMarkCaliberReferenceClusterFmax — the demoted
	// fold-basis legend seat (按小核/中核/超大核满频折算, CAP 复核 F1) is
	// RETIRED: the R5 trace-global basis never demotes, so the words have no
	// producer.

	// CAP-2 (§28.4/§28.5, 2026-07-09): the two cluster-structure-evidence
	// upgrade words of the former 簇结构不可判 degrade, each with its own
	// on-demand legend entry.
	runtimeTraceProjMarkCaliberComovementTopology // 口径词 按实测频点共动分簇折算 (Tier-1)
	runtimeTraceProjMarkCaliberKeyedRailTopology  // 口径词 按簇轨实测折算(成员按锚点连续推定) (Tier-2)

	// DISPLAY-WRAP 件③(a) (§29.104.18.1 B2, 2026-07-16): the same-node
	// repeat-suppression short words — a node spells each long caliber phrase
	// in full ONCE (first occurrence, display order); later same-node
	// occurrences collapse to these reference words. Wording only — caliber
	// identity and every value stay byte-identical.
	runtimeTraceProjMarkCaliberStatedBasis      // 口径词 按前述基准 (按全域最大核最高频 同节点免重复短写)
	runtimeTraceProjMarkCaliberStatedClustering // 口径词 分簇口径同前 (按实测频点共动分簇折算 同节点免重复短写)

	// DISP-2 (Wave-3.2 G2/G19/GAP-A P3-6 显示半场, §27.2/§27.5, 2026-07-09):
	// three new gated seats.
	runtimeTraceProjMarkTraceGapBelowFloor    // ◇ 盲区判据二 窗内无≥阈值等待区间·链止 (G2 措辞按 kind 分形)
	runtimeTraceProjMarkAllZeroFoldNote       // 全零折叠行一行注 窗内无有效时长 (G19, 取代 ×N(0.000–0.000)取最大)
	runtimeTraceProjMarkFamilyCountEquivalent // 计数当量X(非墙钟) 对照写法词条 (GAP-A P3-6, 随 count 家族行按需出场; §29.55③ 两形一裁 ms 后缀退役)

	// G12-ENG (§29.1, 2026-07-09): one new gated seat.
	runtimeTraceProjMarkValuelessFoldMembers // 无时长值成员 混合折叠行词条 (E23 ×2 同值伪形修根)

	// 审计 #62 ① (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the
	// on-chain semantic dual-caliber word 链上计入(共N段,同线程) + its
	// 窗口投影合计 union disclosure (intersection<union partial-overlap form).
	runtimeTraceProjMarkFamilyChainIntersection

	// §29.27② (COV-4, 2026-07-11): the four-state coverage account — the
	// focused thread's full-window wall-clock partition + the running-segment
	// attribution line (确定性工作/供给折算影响/自身执行). Renders in the
	// LEAD (coverage region), only when Σ(states) balances the window.
	runtimeTraceProjMarkFourStateAccount

	// UXR-1 §29.36.3 (通道4 提及义务, 2026-07-11): the on-chain semantic
	// mention-obligation word 优化点·未入根因排序前N — the SEM-LEAD mention
	// floor as an explicit channel member.
	runtimeTraceProjMarkSemanticMentionFloor
	// UXR-1 §29.36② (2026-07-11): ⧗ — the off-chain (◇/▒) D-state/IO form
	// glyph; ⛓ renders only on the chain channel (glyph/stanza/channel 三面
	// 同一来源).
	runtimeTraceProjMarkIconDStateOffChain

	// P9 arm c (§29.42 案1 BINDER-MISATTR, 2026-07-12): the frame-pacing idle
	// teaching seat — renders whenever a pacing_idle row is in the tree (its
	// type word 帧间空闲(等待下一帧) rides the typelabels table).
	runtimeTraceProjMarkPacingIdle

	// 复核 P2-1 (2026-07-12): the generic periodic-idle teaching seat — the
	// arm-c fork for measured periodic (non-frame) wakers; type word
	// 周期空闲(等待下一周期信号).
	runtimeTraceProjMarkPeriodicIdle

	// CAL-1 件⑤/件⑥b (2026-07-12): the cadence-idle row's dedicated state
	// icon (glyph bytes single-sourced in tracefence.GlyphPacing) — the
	// pacing_idle / periodic_idle independent rows lead with it instead of
	// the neutral transit mark; the legend entry teaches the glyph plus the
	// 行2 「节拍吻合」 typed mint word.
	runtimeTraceProjMarkIconPacing

	// V2-P0 ⌗ 口径旁栏 (design §6.1 新裁定 A, 2026-07-12): the caliber-side
	// row disclosure — count-equivalent / composite-score rows that left the
	// ordinal space but keep their rendered channel seat.
	runtimeTraceProjMarkCaliberSideRow

	// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
	// fold row — chain-lane bipartition cut seats <0.1ms folded into one
	// counted ⛓ row publishing the members' account Σ.
	runtimeTraceProjMarkMicroAnchorFold

	// RNB-5B 件⑨ (§29.96.2 终判⑨, 2026-07-15): the endpoint-less multi-window
	// chip — a merged seat spanning multiple query windows whose chip window
	// is typed-unresolvable states 多窗(端点见明细) instead of guessing.
	runtimeTraceProjMarkMultiWindowNoEndpoints

	// RNB-5B 修复轮 D2 (2026-07-15): the ⌗ row-head glyph — 口径旁栏 rows
	// (count-equivalent / composite-score family) wear ⌗ instead of a
	// scheduler-state or channel glyph.
	runtimeTraceProjMarkIconCaliberSide

	// CR-2 组② P5 同段收敛 (§29.42 P5, 2026-07-12): the same-segment mirror
	// tag — a raw-state copy folded into its richer row (equality arm, 14704
	// E1/E2) or a merged twin pointing at its source family row (family arm,
	// donghu E8/E9). Annotation only; values never double-count.
	runtimeTraceProjMarkSameSegMirror

	// CR-2 组③ P7 (§29.42 P7, 2026-07-12): the episode-scope actual word —
	// the actual exceeds this row's own occurrence projection but stays
	// INSIDE the analysis window (the former value-only ⚠ was false here).
	runtimeTraceProjMarkActualBeyondEpisode
	// CR-2 组③ P7: the scope-less actual disclosure — the actual value
	// overshoots but its physical interval was not published, so no window
	// verdict is claimed (宁漏勿假).
	runtimeTraceProjMarkActualNoInterval

	// WO-A1 (SMR-1 批, smr_audit_report §④, 2026-07-12): the unified
	// 「不可相加/包含」cross-seat pointer word family (为…组成部分 / 已含 /
	// 为…成员) — typed judgment, one template, three carriers.
	runtimeTraceProjMarkNonAdditivePointer

	// WO-C1 (SMR-1 批, 2026-07-12): the C-type account-relation sentence —
	// same thread, same state family, two account systems with differing
	// coverage sets (W-A 双行存续 + 理由句).
	runtimeTraceProjMarkAccountRelation

	// WO-B1 (SMR-1 批, 2026-07-12): the B-type occurrence-series short note —
	// disjoint same-identity occurrences carry their interval, sibling refs
	// and the series total.
	runtimeTraceProjMarkOccurrenceSeries

	// P2a rider 件3 (§29.58.2 binder ⋈ 分裂裁定, 2026-07-13): the dedicated
	// binder IPC-wait glyph mark — BinderWait no longer borrows
	// IconNoDominant, so a binder-only report lights the ⋈ entry instead of
	// the ◦ 数据行 entry (F1 图例修真). Legend entry is GENERATED from the
	// §24.3 form table (GeneratedLegend, single source).
	runtimeTraceProjMarkIconBinderWait

	// P2a rider 件2b (§29.58.1 b, 2026-07-13): the ↳ subordinate-component
	// connector — a component row (self binder ⊂ sleep carve) renders
	// directly under its owning seat with the connector in the badge slot;
	// the word face 「组成部分·不可相加」 stays on the WO-A1 pointer tag.
	runtimeTraceProjMarkSubordinateComponent

	// SELF-SEM (§29.61.1 user ruling, RANK-U Stage 1, 2026-07-13): the
	// 「自身·确定性优化」 Row2 qualifier — the analysis target's own
	// deterministic semantic work admitted to the on-chain channel on the
	// typed self basis (node.OnChainBasis == self_deterministic_span), with
	// no wakeup-edge claim. Rendered wherever the cause identity row renders.
	runtimeTraceProjMarkSelfDeterministicBasis

	// SELF-ALL (§29.61.2/§29.61.2a user rulings, 2026-07-13): the
	// 「自身·墙钟席」 Row2 qualifier — the analysis target's own wall-clock
	// seat (blocked-state family / IO facet / runnable / running) admitted to
	// the on-chain channel on the typed self basis (node.OnChainBasis ==
	// self_wall_clock_interval), with no wakeup-edge claim; effective
	// attribution rides the same per-state ladder as every on-chain row.
	runtimeTraceProjMarkSelfWallClockBasis

	// SELF-LANE (§29.58.3 处置 a, 2026-07-13): the 「非链」 qualifier on a
	// target-subject row RELOCATED out of the ◇ 邻近区段 into the self stanza —
	// display placement only (the row's typed channel stays adjacent, its
	// ordinal/caliber words unchanged): the 邻近 word promises OTHER threads
	// competing nearby, and the subject is never its own neighbour.
	runtimeTraceProjMarkSelfNonChainSeat

	// SELF-LANE (§29.58.3 处置 b, 2026-07-13): the cross-channel same-thread
	// mutual pointer pair (「本线程另有链上席 [E#]」/「本线程另有邻近席 [E#]」)
	// — SMR-1 relation-sentence family; lit at the ONE template's emission.
	runtimeTraceProjMarkCrossChannelPointer

	// ELIM-1 (◎ 窗内可消除量总览, rank_order_v2_design_20260712.md R16⑥,
	// GREENLIT 2026-07-12; RANK-U Stage 2, 2026-07-13): the overview stanza's
	// region mark — lit exactly when the ◎ fence renders under the tree; its
	// catalog entry is the R16⑥ legend promise sentence. The overview is NOT
	// a fourth §29.27.1 mark surface (design R7 boundary): it wears no
	// ordinals and no badges, so the 三面记号一致 invariant owes it nothing.
	runtimeTraceProjMarkElimOverview

	// ELIM-V2 方向分组制 (2026-07-18) mark family — the ◎ chain block's
	// fix-direction sections and their guard word faces (each mark lights at
	// its ONE ◎ emission site; 词条-图例双向):

	// ▸ section head (方向词 + 最大可消 恒发; 节序=节内最大可消降序).
	runtimeTraceProjMarkElimDirectionSection
	// the 方向未定/复合 fail-open tail section (unresolved registry direction
	// — never guessed, always last in the chain block).
	runtimeTraceProjMarkElimDirectionUnresolved
	// the L1 section subtotal 小计 X ms(区间互斥) — published only on typed
	// pairwise-exclusive member envelopes; Σ == the µs sum of the member rows.
	runtimeTraceProjMarkElimSectionSubtotal
	// the L2 non-addable word 成员区间重叠,合计不可直加 (measured envelope
	// overlap — the section publishes NO subtotal).
	runtimeTraceProjMarkElimSectionNonAddable
	// the ·∩[E#] cross-direction overlap chip + the merged pair footnote —
	// transcription of the tree rows' typed 互指句 pairs (件2 wire carrier
	// only; carrier absent → nothing renders).
	runtimeTraceProjMarkElimCrossDirectionChip
	// the ◇ row's ·方向=X transcription word (the ◇ block is unsectioned;
	// same single word table as the section heads).
	runtimeTraceProjMarkElimAdjacentDirectionWord
	// the ◇ block head 「◇ 邻近(条件可消上界 · 不入方向守恒)」 separating the
	// direction sections from the unsectioned adjacent block.
	runtimeTraceProjMarkElimAdjacentBlockHead
	// the 守恒尾行 checker transcription (pass line or per-direction violation
	// disclosure — AXIOM-V2 件3 finding, §29.104.13 非致命不硬拦).
	runtimeTraceProjMarkElimConservation

	// RSPA §29.61.10a (2026-07-14): the 行2 同源二分 decomposition disclosure
	// — BOTH halves of a re-anchored window state seat (the ⛓ clipped anchored
	// half and the ◇ remainder half) name the 全窗=锚定+其余 split (typed
	// Node.ChainAnchoredMS / ChainAnchorFullMS / ChainAnchorRemainderSeat
	// only; remainder = full − anchored is display arithmetic over the ONE
	// engine-minted pair, never a new account).
	runtimeTraceProjMarkChainAnchorSplit

	// RSPA §29.61.10b (2026-07-14): the WO-C1-family same-source seat relation
	// sentence (合计还原全窗账) — the ⛓ anchored seat and its ◇ remainder seat
	// cross-reference [E#] both ways; deliberately its OWN mark (the generic
	// 账目关系 entry teaches 不可相加, while this pair is the ONLY additive
	// seat relation — one legend entry cannot speak both).
	runtimeTraceProjMarkChainAnchorRelation

	// INV-SUPPLY 件① (§29.61.11/.11a, 2026-07-14): the compound type-word
	// suffix 「·供给缺口主导」 — a supply-gap-dominant inversion seat's 行2
	// category word and its ◎ overview transcription (同词, one composer:
	// runtimeTraceProjInversionSupplyGapCompoundWord). Lit at both emission
	// sites; the entry teaches the typed criterion and the non-additive
	// caliber boundary.
	runtimeTraceProjMarkSupplyGapDominant

	// INV-SUPPLY 件③ (§29.61.11, 2026-07-14): the ◎ overview seat's
	// eliminable-composition leverage note 「可消除构成: 调度修复 X +
	// 频点/热策略 Y」 — a transcription of the seat's OWN 行3 attribution
	// split (runtimeTraceProjInversionComponents, the balance-gated builder)
	// relabeled by leverage direction; a constituent display, never a Σ row.
	runtimeTraceProjMarkElimComposition

	// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): the multi-window merged row's
	// member-window-span disclosure word 成员跨K窗 — the seat's 窗X~Ys chip
	// names only the SEAT-SUPPLYING member's query window (typed
	// RankQueryWindow pair), so a merged row whose members span >1 query
	// windows qualifies the chip with 「(供席成员窗,成员跨K窗)」 and the ◎
	// overview transcribes the span word beside the seat it values (the row's
	// Σ spans those same windows). One emitter
	// (runtimeTraceProjMergedMemberWindowSpanWord); per-member windows stay on
	// the detail blocks' 窗来源 lane.
	runtimeTraceProjMarkMergedMemberWindowSpan

	// RNB-1 (§29.88 R2, 2026-07-14): the case-A' downgraded relation sentence
	// 账目关系(锚定权属失合) — a migrated ◇ remainder seat whose pid's chain
	// seat is present but does not provably hold the census-anchored account
	// (typed Node.ChainAnchorOwnershipDivergent + the double-Σ pair). The
	// additive 同源二分 wording is forbidden on this row; both Σs and their
	// delta are disclosed inline.
	runtimeTraceProjMarkChainAnchorDivergent

	// RNB-1 R4 (§29.88.2, 2026-07-14): the whole-seat lane-demotion
	// disclosure 无链上凭证(整席降道) — an indivisible on-chain account that
	// cannot show a typed causal-edge anchored share rides the ◇ adjacent
	// channel whole (values untouched): affinity/cpuset satellites,
	// inversion-retyped window seats, zero-credential D/IO view rows.
	runtimeTraceProjMarkChainCredentialDemoted

	// HULL-CRED (§29.104 终判③, 2026-07-17): the per-segment-proven fork of
	// the R4 demotion word — 无链上凭证(逐段核验,整席降道). The row's hull
	// intersected the anchor windows, but its COMPLETE typed segment
	// inventory proved every real segment outside them (the pre-fix
	// fake-credential keep-⛓ shape); the fork renders ONLY when the decoded
	// inventory rides the row (claim gated on proof — an inventory-less
	// marker keeps the generic R4 bytes).
	runtimeTraceProjMarkChainCredentialSegmentDisjoint

	// HULL-CRED (§29.104 终判③, 2026-07-17): the envelope-tier honest word
	// (包络级凭证) — a keep-⛓ D/IO view row whose chain lane was retained on
	// the conservative envelope/census fail-open verdict (segment inventory
	// absent: cost-degraded or legacy ledger shapes; ONCHAIN-FIX-2 件3: also
	// a truncated prefix that proved no intersection — 缺证≠证无); the word
	// discloses the credential granularity, the lane and every value stay
	// untouched.
	runtimeTraceProjMarkChainCredentialEnvelope

	// ONCHAIN-FIX-2 件3 (Q6 已追认, 2026-07-18): the truncated lower-bound
	// prefix word 凭证清单不完整,实际锚定不小于所证 — a keep-⛓ row whose published segment
	// credential is the ledger's immutable checked PREFIX of a beyond-cap
	// D/IO group (≥1 prefix segment truly intersects an anchor window). The
	// proven intersection is a LOWER BOUND: the uncollected segments beyond
	// the cap can only add to it, never subtract (「实际锚定不小于此值」 —
	// the 下界 caliber family). Renders only beside the published inventory
	// on the current keep-⛓ lane; lane and every value untouched.
	runtimeTraceProjMarkChainCredentialTruncatedLowerBound

	// ONCHAIN-FIX-1 件1 (mint audit 命题2 不一致①, 2026-07-18): the
	// identity-inheritance honest word 身份继承(链窗级,无区间凭证) — an
	// on-chain row that published NO typed interval and inherited the chain
	// lane from bare thread identity (its pid is a chain member; the
	// documented fail-open conservative keep). The pre-fix shape fabricated
	// the overlap value from the whole node-window wall clock on exactly
	// these rows; the word replaces the fabricated number and discloses the
	// credential tier. Weaker than every adjudicated vocabulary: the
	// HULL-CRED per-segment / envelope words and the demotion words all
	// suppress it.
	runtimeTraceProjMarkChainIdentityInheritance

	// XLANE-1 件1 (§29.104.1/§29.104.2, 2026-07-15): the represented-by-
	// chain-seat whole-seat demotion disclosure 锚定份由链席代表(整席降道) —
	// a FULLY-anchored runnable-family satellite whose same-pid chain-lane
	// runnable seat physically intersects its segments rides the ◇ adjacent
	// channel whole (values untouched). Deliberately a SEPARATE word family
	// from the R4 无链上凭证 entry: this seat HAS credential; the demotion
	// reason is seat representation (one physical account, one full chain
	// seat).
	runtimeTraceProjMarkChainAnchorRepresented

	// R3-IMPL (§29.88.1 user ruling, 2026-07-15): the host-edge-anchored
	// semantic seat's credential disclosure 边锚定(宿主→目标) — a NON-target
	// thread's deterministic semantic span seated on the chain tier by the
	// HOST's own in-window typed wakeup edge toward the target (typed
	// Node.OnChainBasis == "host_wakeup_edge_pre_span", single-field fork —
	// the SELF-SEM qualifier discipline). The sentence speaks the R4 family
	// language (边=凭证,边前=有效,边后=解除); the value is the pre-edge
	// in-window projection, a boundary-crossing span bisects (its post-edge
	// share rides a ◇ ChainAnchorRemainderSeat clone wearing the existing
	// 同源二分 sentence). ONCHAIN-3c (2026-07-19): the same mark covers the
	// state-seat sibling basis "host_wakeup_edge_pre_state" (runnable / D-IO
	// state seats of bare-census-edge hosts; value = the segment inventory's
	// pre-edge share sum) — the 行2 sentence and the legend fork the value
	// clause on the single basis field.
	runtimeTraceProjMarkHostEdgeAnchored

	// XLANE-2 件1 (§29.104.1/.2 定谳④, 2026-07-17): the semantic member-subset
	// whole-seat demotion word 为[E#]成员子集(整席降道) — a semantic
	// family seat whose COMPLETE typed member line-range set is a proper
	// subset of a same-board same-subject semantic seat's set (the witness
	// E34=E35∪E49 cross-step double-mint). Values and engine ordinals stay
	// untouched; the ◎ face excludes the row into a dedicated footnote.
	runtimeTraceProjMarkSemanticMemberSubset

	// XLANE-2 件2 (user ruling §29.104.17 ④ 披露式拆分, 2026-07-17): the
	// self-gap seat's semantic-overlap disclosure 其中 X ms 与语义席[E#]重叠 —
	// the self running supply-fold deficit seat and the target's own semantic
	// seats bill the same physical running wall clock through two calibers;
	// X is the exact typed interval intersection. Disclosure only (主值零动,
	// 硬扣除不做 — the ruling explicitly rejects a value deduction).
	runtimeTraceProjMarkSelfGapSemanticOverlap

	// AXIOM-V2 件1 (user rulings 2026-07-18): the 行2 fix-direction attribute
	// word 修向 X — the registry repair-direction class (closed set; attribute
	// axis only, 序数芯片本体零动: ordinals, sort and every value untouched).
	runtimeTraceProjMarkFixDirection

	// AXIOM-V2 件2 (公理 v2, 2026-07-18): the cross-direction mutual-overlap
	// clause 与[E#](修向 X)同段重叠 Y ms…收益不叠加 — two strict on-chain
	// full seats of one thread/window/board/caliber(墙钟) across DIFFERENT
	// fix directions share the same physical wall clock (typed support-
	// interval intersection); both seats speak the clause or neither.
	runtimeTraceProjMarkCrossDirectionOverlap

	// XERR1-FIX 件2 (§29.104.3/.4, 2026-07-15): the payload-less blocking_span
	// basis-form glyphs — ⊖ 阻塞等待候选 (value converged to the waiter's
	// Σ(sleep+D+iowait) inside span∩window) and ⊓ span 包络(含运行)
	// (convergence impossible; the envelope makes no blocking claim). Legend
	// entries are generated from the §24.3 spec table (GeneratedLegend).
	runtimeTraceProjMarkIconBlockingWait
	runtimeTraceProjMarkIconSpanEnvelope

	// XERR1-FIX 件1 互指 (§29.104.4; E6/E7 账目关系先例): the blocking↔sleep
	// mutual-pointer sentence pair — the converged row's sleep share and the
	// thread's own sleep seat cover the same physical time in two accounts.
	runtimeTraceProjMarkBlockingWaitSleepRelation

	// ELIM-GAP 件D (§29.104.15, 2026-07-16): the occurrence-segment account
	// caliber short word 「(发生段账目)」 — a C5-guarded typed-producer row
	// (inversion gated composite / §20.2 running deficit) whose published
	// effective sits above its own window projection says how the value was
	// taken, honoring the 关键指标 glossary promise (与窗口投影不同时,行内
	// 口径词说明取值方式; witness cust_total_del E16/E18/E19).
	runtimeTraceProjMarkOccurrenceSegmentAccount

	// GATED-CAL 件1 (§29.104.16.1 M3 四面一根, 2026-07-16): the gated-composite
	// caliber word 「构成,见明细」 — a value composed of runnable(全额) +
	// running(折算) must never impersonate a single caliber (the A2 witness
	// 「(全额)」 false cover / the bare ◎ state word / the naked 有效归因X tag /
	// the window-projection cell). One word source
	// (runtimeTraceProjGatedCompositeShortWord), lit at every emission face.
	runtimeTraceProjMarkGatedCompositeCaliber

	// LEVELMERGE-1 件2 (方案 P 区间分账, user ruling 2026-07-18): the
	// gated-share split word family — the (pid,runnable) chain aggregate
	// seat's account splits against the same thread's priority-inversion
	// seat(s): the residual seat keeps competing with 全账=已计入反转席份+
	// 本席残余 (identity claimed+residual==full), the demoted A constituent
	// row wears 已计入反转席[E#](分账构成份) on the ◇ adjacent lane.
	runtimeTraceProjMarkGatedShareSplit

	// LEVELMERGE-1 件2 fail-open (裁定④ §29.104.17 句形): the overlap
	// disclosure clause 其中 X ms 与反转席[E#]重叠 — a partial typed interval
	// inventory witnesses the overlap (lower bound over available real
	// segments) with every published value untouched (no value split).
	runtimeTraceProjMarkGatedShareOverlap

	// LEVELMERGE-1 件3 (两向互指, user ruling 2026-07-18): the aggregate-seat
	// ↔ member-occurrence cross-reference pair — the seat row lists its
	// constituent-segment view rows 构成段见[E#…], each member view row points
	// back 归因已计入[E#](聚合席),本行为构成段,不另计 (the census §1.3-1
	// missing-direction pair; all-or-nothing resolution, 宁漏勿假指).
	runtimeTraceProjMarkAggregateMemberCrossRef

	// SPANTOP-1 (§29.131, user ruling 2026-07-18): the semantic family seat's
	// constituent top-3 sub-rows + counted remainder — 成员 name (tail-kept
	// truncation) + 单段 in-window wall clock + 行a..b line range, sorted by
	// in-window contribution desc, plus 另有 N 段 合计X ms · 全清单见明细[E#].
	// Pure display decomposition of the seat's own 行1 total (µs identity
	// gated, all-or-nothing): the sub-rows never compete, never mint a seat,
	// never wear ⛓ and never enter any population/census denominator — the
	// seat row holds every credential (逐段级 HULL-CRED 语义).
	runtimeTraceProjMarkFamilySpanTop

	// SPANVIS-1 (user ruling 2026-07-19 定形原则): the ◈ pure-advisory
	// business-span mention face — on-chain (incl. self) span families whose
	// in-window total cleared the significance floor, listed as non-ordinal
	// advisory rows on the tree fence and as the ◎ overview's business-lead
	// footnote. The rows never compete, never mint a seat, never wear ⛓/bar/
	// badge and never enter any population/census/conservation denominator
	// (不参与根因排序 — 业务视角线索 only).
	runtimeTraceProjMarkBusinessSpanMention

	// PARTSPLIT-1 (§29.150④ user ruling, 2026-07-19): the R4-mirror-refused
	// gated composite seat's 边前份披露 pre-edge-share disclosure family —
	// the seat-row 行2 分账 sub-line and the ◎ chain-section NON-SEAT mention
	// row both wear this mark (◈ two-face precedent). Disclosure only: the
	// seat's value/lane/ordinal stay untouched (R4 整席不拆 floor), the
	// mention takes no ordinal and never joins a section maximum, and the
	// pre-edge share is never additive to the seat's own published value
	// (same segments).
	runtimeTraceProjMarkGatedCompositeEdgeShare

	// RULER2-1 (§29.150② user ruling / R-19-b, 2026-07-19): the self
	// runnable account two-ruler cross-row disclosure family — the LEAD seat
	// row's 行2 按两把尺记账 sentence wears this mark. Disclosure only: every
	// seat's value/lane/ordinal stays untouched, same-ruler subtotals are
	// the only additions (µs identity re-validated), and NO cross-ruler sum
	// is ever computed or rendered (M3 禁混尺).
	runtimeTraceProjMarkSelfRunnableTwoRuler

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
	// PTV8-RCR-A §24.3: the four new impact-form glyph entries (⊗/⇅/↯/◌) are
	// GENERATED from the single-source form table — never hand-written here
	// (两列单源; see runtimeTraceProjImpactFormLegendEntries).
	catalog := []runtimeTraceProjLegendEntry{
		// PTV8-RCR-A §24.3 (2026-07-08). EVOLUTION RECORD: 🎯 → ⊚ — the tree's
		// only colored emoji leaves (无亮色 hard rule; 复核 F3: EAW-Neutral
		// glyph, single source runtimeTraceProjRootGlyph); wording unchanged.
		{runtimeTraceProjMarkRootTarget, runtimeTraceProjLegendGroupMark,
			"- `" + runtimeTraceProjRootGlyph + "` = 树根:本次分析锚定的关注线程。",
			"- `" + runtimeTraceProjRootGlyph + "` = tree root: the focused thread this analysis anchors on."},
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
			"- `├─own─` = an own-/same-process caliber row of the focused thread (another caliber of the same wall clock), not a wake edge."},
		// PTV6 #1b (v3 §5): the depthless on-chain lane's dedicated edge — the
		// row never claims a wake/drill relation and never invents a tree
		// position (NEW-7: the entry renders only when the edge is emitted).
		// EVOLUTION RECORD (UXR-1 §29.36④, 2026-07-11): both entries follow the
		// lane-prefix simplification — the edge reads the bare `└─链上─` and the
		// auxiliary word (深度未解析 / 父节点未确认) rides the 行2 chip family.
		{runtimeTraceProjMarkEdgeChainUnresolved, runtimeTraceProjLegendGroupEdge,
			"- `└─链上─`+行注`链上·深度未解析` = 链上项但树位深度未解析:不声称唤醒/下钻关系,不编造树位;该行 [E#] 经证据索引给出 trace 行号区间。",
			"- `└─on-chain─` + row note `on-chain · depth unresolved` = an on-chain row whose tree depth is unresolved: no wake/drill relation is claimed and no position is invented; the row's [E#] resolves to trace line spans via the evidence index."},
		// PTV8-RCR-C (§24.12 C6 三面同词, 2026-07-08): the depth-KNOWN sibling
		// of the entry above — the engine resolved the row's chain layer but no
		// attach point exists in this tree; the row chip and the detail 层级
		// cell speak ONE word family (the old fork spoke 深度未解析 / 链上L1 /
		// 深度1(未接入链) on one row).
		{runtimeTraceProjMarkChainSeatUnattached, runtimeTraceProjLegendGroupEdge,
			"- `父节点未确认` = 该行在链上且引擎已给出层数(行注/明细作 链上L#(父节点未确认)),但本树内未找到可挂靠的父节点:不声称唤醒/下钻关系,不编造树位。",
			"- `parent unconfirmed` = the row is on the chain with an engine-resolved layer (chip/detail read chain L# (parent unconfirmed)), but no attach point exists in this tree: no wake/drill relation is claimed and no position is invented."},
		// PTV8-RCR-A §24.3: ✦ joins the impact-form closed set as the
		// deterministic-optimization glyph — the entry names the family.
		{runtimeTraceProjMarkSemanticSpan, runtimeTraceProjLegendGroupEdge,
			"- `├─语义─`/`✦` = 该位置的语义 span(业务阶段,确定性优化族),非调度状态行。",
			"- `├─span─`/`✦` = a semantic span (business phase, deterministic-optimization family) at this position, not a scheduler-state row."},
		// PTV7 (#74, 用户裁定 2026-07-06): the state-icon entries are the SINGLE
		// point carrying the Chinese semantics of the canonical English state
		// words (图例中文注解单点) — rows speak only the token; the gloss lives
		// here, one entry per display word, bidirectionally pinned
		// (TestPTV7LegendStateAnnotationBidirectional).
		// PTV8-RCR-B (UXA 域A #9, 2026-07-08). EVOLUTION RECORD: 「其唤醒子行即
		// 下钻结果」倒装+内部动词连用 → 直陈「根因看子行」(客户化令 §24 ④).
		{runtimeTraceProjMarkIconSleep, runtimeTraceProjLegendGroupMark,
			"- `☾/sleep` = 睡眠等待(等事件/等唤醒);睡眠是症状而非根因,根因看它的下钻/唤醒子行。",
			"- `☾/sleep` = a sleep wait (waiting on an event/wake); sleep is the symptom, not the root cause — look at its drill/wake child rows."},
		{runtimeTraceProjMarkIconRunnable, runtimeTraceProjLegendGroupMark,
			"- `⧖/runnable` = 就绪等待(有资格运行但未获得 CPU)。",
			"- `⧖/runnable` = ready to run, waiting for a CPU."},
		{runtimeTraceProjMarkIconRunning, runtimeTraceProjLegendGroupMark,
			"- `⚙/running` = 运行占用(正在 CPU 上执行)。",
			"- `⚙/running` = executing on a CPU."},
		{runtimeTraceProjMarkIconDState, runtimeTraceProjLegendGroupMark,
			"- `⛓/D-state·iowait` = 不可中断等待/IO阻塞(链上行)。",
			"- `⛓/D-state·iowait` = an uninterruptible / IO-blocked wait (on-chain row)."},
		// UXR-1 §29.36② (2026-07-11): ⛓ visually claims chain membership, so
		// the ◇/▒ D-state/IO family wears its own glyph (三面同一来源).
		{runtimeTraceProjMarkIconDStateOffChain, runtimeTraceProjLegendGroupMark,
			"- `⧗/D-state·iowait` = 不可中断等待/IO阻塞(◇/▒ 非链上行,与 ⛓ 同族)。",
			"- `⧗/D-state·iowait` = an uninterruptible / IO-blocked wait (◇/▒ off-chain row, same family as ⛓)."},
		// RNB-5B 修复轮 D2 (2026-07-15): the ⌗ 口径旁栏 row-head glyph — a
		// count row falling into the ⛓ arm claimed a channel plus an
		// uninterruptible wait it never measured.
		{runtimeTraceProjMarkIconCaliberSide, runtimeTraceProjLegendGroupMark,
			"- `⌗`(行首)= 口径旁栏行(计数当量/综合评分族)的行首记号:非调度状态、非通道宣称;数值口径见行内 `⌗口径旁栏` 词。",
			"- `⌗` (row head) = the caliber side-rail row marker (count-equivalent / composite-score family): no scheduler state and no channel is asserted; the value's caliber rides the in-row `⌗ caliber-side` word."},
		// PTV8-RCR-B (UXA 域A #11 + 域D #23 本轮/本批→本报告, 2026-07-08).
		// EVOLUTION RECORD: 「影响行」「本轮」渲染器内部词 → 客户视角「未单独计量」.
		{runtimeTraceProjMarkIconTransit, runtimeTraceProjLegendGroupMark,
			"- `◦ 中转` = 唤醒链的中间经过节点,本报告未单独计量其影响。",
			"- `◦ transit` = an intermediate hop on the wakeup chain; this report does not measure its impact separately."},
		// PTV6-D (b) (#75 标本归因 #10): the per-row 2-word 无主导态 chip is
		// retired — the ◦ icon on a DATA row is the marker and this entry
		// carries the class word (行内不逐行重复).
		// PTV8-RCR-B (UXA 域A #7, 2026-07-08). EVOLUTION RECORD: 「无主导态」重复
		// 定义+「(类别词不逐行重复)」渲染器实现注记 → 一句直陈.
		// EVOLUTION RECORD (UXR-1 §29.36.1, 2026-07-11): 有影响形态词的行戴
		// 形态族 glyph (typed form table) — ◦ 只留真正无类型词的行, so the
		// entry now states BOTH halves symmetrically (未识别影响类型 + 无主导
		// 调度状态; the sched-absence fact stays carried, the glyph carries
		// the strongest resolved semantics).
		{runtimeTraceProjMarkIconNoDominant, runtimeTraceProjLegendGroupMark,
			"- `◦`(数据行) = 未识别出具体影响类型且无主导调度状态的行;有形态词的行戴各自形态族记号,该行的已知信息见行内说明或明细。",
			"- `◦` (data row) = a row with no recognized impact type and no dominant scheduler state; rows with a form word wear their form family's glyph — this row's known facts live in the inline note or the detail table."},
		// P2a rider 件2b (§29.58.1 b, 2026-07-13): the subordinate-component
		// connector — 结构管关系 (the ↳ + placement say "component of the row
		// above"), 词面管语义 (the row keeps its 「为[E#]的组成部分·不可相加」
		// pointer tag).
		{runtimeTraceProjMarkSubordinateComponent, runtimeTraceProjLegendGroupMark,
			"- `" + tracefence.GlyphSubordinate + "` = 该行是紧邻上一行账目的组成部分(从属视图);两行数值不可相加,从属关系另见行内「为[E#]的组成部分」标注。",
			"- `" + tracefence.GlyphSubordinate + "` = this row is a component of the row directly above (subordinate view); the two values never add, and the containment is also named by the row's \"component of [E#]\" tag."},
		// PTV5 C01/C24 (#68): badges land on chain/cause/depthless rows — flat
		// renders included — so the entry does not claim 链上 (CMP-7a: flat
		// renders never claim on-chain). The sort key is visible per row via
		// the Q1 有效归因 tag.
		// PTV8-RCR-B (UXA 域D #5 根因族, 2026-07-08). EVOLUTION RECORD: 「根因
		// 关注点 TOP3」与已裁「根因排序#N」不同源 → 同族「根因排序前三」.
		// §29.27.1 (用户裁定 2026-07-11). EVOLUTION RECORD: 前三 → 前五
		// (❶..❺,徽章跟随席位), wording per the ruling verbatim.
		{runtimeTraceProjMarkBadge, runtimeTraceProjLegendGroupMark,
			"- `❶..❺` = 根因排序前五(依有效归因)。",
			"- `❶..❺` = the top-5 root-cause seats (by effective attribution)."},
		// PTV8-RCR-B (UXA 域A #8 REVISE 缩写稿, 2026-07-08). EVOLUTION RECORD:
		// 「类型 token 自带的状态语义/沿用影响形态」内部推导话术 → 五词枚举直陈.
		{runtimeTraceProjMarkStateLabel, runtimeTraceProjLegendGroupMark,
			"- 行内 sleep/runnable/running/iowait/D-state = 该行的主导调度状态。",
			"- Inline sleep / runnable / running / iowait / D-state = the row's dominant scheduler state."},
		// PTV8-RCR-B (UXA 域D #8 两面合词, 2026-07-08). EVOLUTION RECORD: 与
		// 关键指标图例 ⊘ 条同词(sched_wakeup 为 trace 事件名保英文).
		{runtimeTraceProjMarkUndrillable, runtimeTraceProjLegendGroupMark,
			"- `⊘链止` = 窗口内无匹配唤醒事件(sched_wakeup),链止于此。",
			"- `⊘chain ends` = no matching wakeup event (sched_wakeup) in the window; the chain ends there."},
		// PTV8-RCR-B (UXA 域D #33 窗族: 分析窗口→分析窗, 2026-07-08).
		// CR-2 组③ P7 (2026-07-12): 确证 + 单次成员 — the ⚠ claim now requires
		// the typed interval verdict, and merged rows disclose the seed caliber.
		{runtimeTraceProjMarkCrossWindow, runtimeTraceProjLegendGroupMark,
			"- `⚠实际Xms` = 实际状态区间确证跨出分析窗(实际共 X ms),时长条只画窗口内投影;合并行的实际值为合并种子单次成员(标注 单次成员),非族合计。",
			"- `⚠actual Xms` = the state's actual interval provably crosses the analysis window (X ms in total); the bar draws only the in-window projection; on a merged row the actual is the merge seed's single member (marked single member), never the family total."},
		// §21 LEAD-SEM 前置 L1 (cmp_01 A④, 2026-07-07): the value-less fork of
		// the ⚠ marker — the row is typed cross-window but its actual total was
		// never captured (ActualImpactMS<=0), so the marker states the fact
		// WITHOUT the fake "实际0.000ms" scalar (16 semantic rows on the real
		// specimen all claimed an actual of 0.000ms they never measured).
		{runtimeTraceProjMarkCrossWindowNoActual, runtimeTraceProjLegendGroupMark,
			"- `⚠跨窗` = 实际状态跨出分析窗,但窗外实际总时长未采集(无值);时长条只画窗口内投影。",
			"- `⚠cross-window` = the underlying state extends beyond the analysis window, but its actual total was not captured (no value); the bar draws only the in-window projection."},
		{runtimeTraceProjMarkRecursOnChain, runtimeTraceProjLegendGroupMark,
			"- `↺` = 该线程在链上重复出现(小循环形态)。",
			"- `↺` = this thread recurs on the chain (small-cycle shape)."},
		// PTV8-RCR-B (UXA 域A #12 + 域B #27: 明细「深度N」链上臂并同词, 2026-07-08).
		// EVOLUTION RECORD: 「层深/同源」实现血统词 → 「层数/一致」.
		{runtimeTraceProjMarkChainDepthChip, runtimeTraceProjLegendGroupCaliber,
			"- `链上L#` = 该行在唤醒链上的层数(与明细「层级」行一致)。",
			"- `chain L#` = the row's layer number on the wakeup chain (matches the detail blocks' level line)."},
		// PTV8-RCR-C (§24.13 裁定二后半, 2026-07-08): the multi-board window
		// tag — several query windows each mint their own root-cause board, so
		// a bare #1×2 collision needs the seat's window identity spelled out.
		{runtimeTraceProjMarkRankSeatWindow, runtimeTraceProjLegendGroupCaliber,
			"- `根因排序#N·窗X~Ys` = 本报告包含多个查询窗、各窗有各自的根因排序;窗标注明该榜位属于哪个查询窗,不同查询窗的 #N 不可跨窗比较。",
			"- `root-cause rank #N · window X~Ys` = this report carries several query windows, each with its own root-cause board; the window tag names the board a seat belongs to — #N ordinals from different windows never compare."},
		// XLANE-3 件2 (§29.104.2 定谳③, 2026-07-16): the board-anchor and
		// params halves of the seat-ordinal board identity triple (channel
		// noun composed from the tracefence single source — UXG-1 F2).
		{runtimeTraceProjMarkRankBoardAnchor, runtimeTraceProjLegendGroupCaliber,
			"- `板锚 <线程>` = 同一查询窗内存在多块" + tracefence.SeatChannelChainZH + "板(各查询步的目标线程不同,各板有各自的 #N);板锚注明该榜位属于以哪个目标线程为主体的查询板,不同板的 #N 不可跨板比较。",
			"- `board <thread>` = one query window hosts several " + tracefence.SeatChannelChainEN + " boards (query steps with different target threads, each board with its own #N); the board anchor names the target thread whose query board a seat belongs to — #N ordinals from different boards never compare."},
		{runtimeTraceProjMarkRankBoardParams, runtimeTraceProjLegendGroupCaliber,
			"- `参数#<指纹>` = 同窗同目标线程存在参数不同的多块" + tracefence.SeatChannelChainZH + "板;参数指纹注明该榜位属于哪次查询参数下的板,不同参数板的 #N 不可跨板比较。",
			"- `params #<fingerprint>` = one window and one target host several " + tracefence.SeatChannelChainEN + " boards whose query knobs differ; the params fingerprint names the knob set a seat's board ranked under — #N ordinals from different params boards never compare."},
		// XLANE-3 件3: the cross-board same-thread same-family mutual pointer.
		{runtimeTraceProjMarkCrossBoardFamilyNote, runtimeTraceProjLegendGroupCaliber,
			"- `同线程同状态族账另见…(跨板)` = 同一物理线程的同一状态族在多块查询板上各有席位;各板独立成账、口径各异,席位值不可跨板相加。",
			"- `this thread's same state family also holds … (cross-board)` = one physical thread's one state family holds seats on several query boards; each board keeps its own account and caliber — seat values never add across boards."},
		// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): the merged-row member-window
		// span disclosure — chip qualifier + ◎ transcription, one emitter.
		{runtimeTraceProjMarkMergedMemberWindowSpan, runtimeTraceProjLegendGroupCaliber,
			"- `成员跨K窗` = ×N 合并行的成员来自 K 个查询窗;席位窗标(若有)只是供席成员的查询窗,不代表整行的窗身份;各成员窗见明细「窗来源」。",
			"- `members span K windows` = the ×N merged row's members come from K query windows; the seat's window tag (if any) names only the seat-supplying member's query window, never the whole row's window identity; per-member windows live in the detail blocks' window sources."},
		{runtimeTraceProjMarkOmitted, runtimeTraceProjLegendGroupCaliber,
			"- `…省略N节点` = 长链中段折叠(行内列出首尾各2个节点),中段节点名不在本报告逐一展开。",
			"- `…N nodes omitted` = the middle of a long chain is folded (the row lists the first/last two nodes); the folded middle nodes are not expanded one by one in this report."},
		// PTV8-LAD L1 (§24.11 维度A / §24.8 循环梯子, 2026-07-08): the run-length
		// cycle fold row — a consecutive repeating tuple inside the folded chain
		// middle collapses into one counted line with its member names in full.
		{runtimeTraceProjMarkCycleFold, runtimeTraceProjLegendGroupCaliber,
			"- `↺ 循环×N: A ⇄ B` = 长链中段的连续循环折叠:所列线程按此顺序连续重复 N 轮,循环内的逐行占位已并入本行(成员名完整列出,不截断)。",
			"- `↺ cycle ×N: A ⇄ B` = a consecutive cycle folded inside the chain middle: the listed threads repeat in this order N times; the per-hop placeholder rows are folded into this one line (member names listed in full, never truncated)."},
		// §21.1 CWD-2 复核收尾② (W1-a 图例互斥): the full-scale claim and the
		// multi-window no-share entry can co-render on one tree — this entry
		// itself scopes the multi-window merged rows' bars to relative scale
		// so the two entries never read as contradictory.
		// PTV8-RCR-B (UXA 域A #13, 2026-07-08). EVOLUTION RECORD: 「树头声明的
		// 尺度」抽象 + 未采集回退分支在已采集报告里纯属噪声 → 两分支各自成条,
		// 按 ScaleNote 分支出场(BarScaleFallback 为回退臂新 mark).
		{runtimeTraceProjMarkBarScale, runtimeTraceProjLegendGroupCaliber,
			"- 时长条:满格 = 树头标注的长度(本报告为分析窗全长);多窗合并行的时长条只作相对量级(见其专项条目)。",
			"- Bars: full scale = the length noted in the tree header (the full analysis window in this report); multi-window merged rows' bars are relative scale only (see their dedicated entry)."},
		{runtimeTraceProjMarkBarScaleFallback, runtimeTraceProjLegendGroupCaliber,
			"- 时长条:窗口未采集,满格 = 本报告最大时长(不显示占窗百分比);多窗合并行的时长条只作相对量级(见其专项条目)。",
			"- Bars: no window captured — full scale = this report's largest duration (no window percentages); multi-window merged rows' bars are relative scale only (see their dedicated entry)."},
		{runtimeTraceProjMarkMergedSum, runtimeTraceProjLegendGroupCaliber,
			"- `N次(a~b)` = 同一(线程,原因)的 N 次实例合并,数值为总和,a~b 为单次范围。",
			"- `n=N(a~b)` = N instances of one (thread, cause) merged; the value is the SUM, a~b the per-instance range."},
		// PTV6-B (聚合幻影修复, 2026-07-06): the entry discloses the near-lane
		// boundary-resampling drift and the max caliber (verbatim pin
		// TestPTV6LegendWordingVerbatimPins; the row tag token ×N同值 未动).
		// 修正轮 Low: the ≤3% figure is the SINGLE-STEP band — transitive folds
		// may drift beyond it, so the entry names no percentage and states the
		// caliber as "the largest published copy in the fold".
		{runtimeTraceProjMarkMergedDedup, runtimeTraceProjLegendGroupCaliber,
			"- `N次同值` = 同一测量被重复发布 N 次(边界重取样时数值可有漂移,显示取合并中的最大一次发布),数值就是那一次测量,不是 N 份。",
			"- `n=N same-value` = one measurement published N times (values may drift under boundary resampling; the display keeps the largest published copy in the fold); the value IS that single measurement, never N shares."},
		// PTV8-RCR-B (UXA 域A #14, 2026-07-08). EVOLUTION RECORD: 「折叠/成员」
		// 实现词、「不可加和,不求和」同义反复 → 先原因后做法;canonical 词
		// 「墙钟跨线程不可加和」三面同词(域B #11 REVISE 基准).
		{runtimeTraceProjMarkMergedMax, runtimeTraceProjLegendGroupCaliber,
			"- `N线程取最大(单项a~b)` = N 个线程的同类行合并为一行;墙钟跨线程不可加和,数值取其中最大一项,a~b 为单项范围。",
			"- `N-thread max(each a~b)` = same-kind rows from N threads merged into one; wall clock never sums across threads, so the value is the largest member, a~b the per-member range."},
		// RCM-2 (§24.12 维度A ③ verbatim, 2026-07-08): the FIFTH caliber word.
		// MANDATED ADJACENCY: this entry sits IMMEDIATELY AFTER the ×N取最大
		// cross-thread entry above — the two must read side by side so
		// 同线程可加 and 跨线程不可加和 never look contradictory (口径组 renders
		// in stable catalog order; adjacency pinned by
		// TestRCM2FifthCaliberLegendVerbatimAndAdjacency).
		{runtimeTraceProjMarkFamilyTotal, runtimeTraceProjLegendGroupCaliber,
			"- `合计(共N段,同线程)` = 同线程墙钟段求和(重叠段取并集),同线程可加;跨线程仍不可加和。",
			"- `total (N segments, same thread)` = same-thread wall-clock segments summed (overlapping segments as their interval union); same-thread wall clock adds legally — across threads it still never sums."},
		{runtimeTraceProjMarkFamilyMemberMax, runtimeTraceProjLegendGroupCaliber,
			"- `成员最大(共N段,重叠未拆)` = 同线程 N 段重叠且无法逐段核销,数值取成员最大(诚实下界,非求和);原始和见明细。",
			"- `member max (N segments, overlap not deducted)` = the N same-thread segments overlap and cannot be deducted per segment, so the value is the member MAX (an honest lower bound, never a sum); the raw sum lives in the detail blocks."},
		// 审计 #62 ① (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the
		// on-chain semantic dual-caliber entry — assembled from the existing
		// closed-set tokens (链上/计入/(共N段,同线程)/窗口投影/合计), rendered
		// exactly when a partial-overlap on-chain semantic row prints the word.
		{runtimeTraceProjMarkFamilyChainIntersection, runtimeTraceProjLegendGroupCaliber,
			"- `链上计入(共N段,同线程)` = 有效归因只计成员段与同线程链窗的精确交集;行旁「窗口投影合计」为全部成员段的并集口径,两口径同处披露、不相加。",
			"- `on-chain counted (N segments, same thread)` = the attribution counts ONLY the exact intersection of the member segments with the same-thread chain windows; the adjacent complete window-projection total is the full member-union caliber — both calibers are disclosed side by side, never added."},
		{runtimeTraceProjMarkFamilyCountSum, runtimeTraceProjLegendGroupCaliber,
			"- `计数合计(共N项,同线程)` = 计数类指标按同线程成员相加(计数可加,与墙钟时长无关)。",
			"- `count total (N items, same thread)` = count-class members of one thread added up (counts add; unrelated to wall-clock duration)."},
		// DISP-2 / GAP-A P3-6 (§28.7 3.2 队列, 2026-07-09): the 计数当量 marker
		// the count-family engine faces print (member roster / raw-Σ note /
		// Summary — rootCauseCountEquivalentValue 三面同源) gets its teaching
		// entry, riding exactly the renders where the count caliber word above
		// appears (co-marked via runtimeTraceProjMarkFamilyCaliber).
		// EVOLUTION RECORD (§29.55 观察③ 两形一裁, 2026-07-14): the taught
		// form 计数当量Xms → 计数当量X(非墙钟) — the value never wears an ms
		// suffix (带 ms=口径谎); the entry follows the minted form.
		{runtimeTraceProjMarkFamilyCountEquivalent, runtimeTraceProjLegendGroupCaliber,
			"- `计数当量X(非墙钟)` = 计数类数值的对照写法:按计数换算的当量值(毫秒尺度),非墙钟时长,故不带 ms 后缀,不与时长行相加。",
			"- `计数当量X(非墙钟)` (count-equivalent X, not wall clock) = the comparison form of a count-class magnitude: a count-derived equivalent on the millisecond scale, not wall-clock duration — it never wears an ms suffix and is never added to duration rows."},
		// §11-N2 (2026-07-06, real_trace_campaign ledger): the cross-query-window
		// union caliber gets its own form token — the plain ×N(a~b) entry claims
		// "数值为总和" and must stay truthful, so a union row NEVER wears the sum
		// form (NEW-7: this entry renders exactly when a union row is emitted).
		// The raw Σ and the window-source roster live in the (b) lossless block.
		{runtimeTraceProjMarkMergedUnion, runtimeTraceProjLegendGroupCaliber,
			"- `N次(a~b)union` = 跨查询窗重叠段不重复计:N 次实例来自不同查询窗且时间重叠,数值为区间并集投影(非求和),a~b 为单次范围;原始和与窗来源见明细。",
			"- `n=N(a~b)union` = cross-query-window overlap counted once: the N instances come from DIFFERENT query windows and overlap in time; the value is the interval-union projection (never the SUM), a~b the per-instance range; the raw sum and the window sources live in the detail blocks."},
		// §21 CWD (cmp_01 revisit 2026-07-07): the overlapping-query-window MAX
		// caliber gets its own form token (×N 第五式) — the sum entry claims
		// 数值为总和 and the union entry claims per-segment deduction; a MAX
		// row may wear neither.
		// PTV8-RCR-B (UXA 域D #33 窗族: 重叠窗→互相重叠的查询窗; 无损块→明细,
		// 2026-07-08).
		{runtimeTraceProjMarkMergedWindowMax, runtimeTraceProjLegendGroupCaliber,
			"- `N次跨窗取最大(单项a~b)` = N 次实例来自互相重叠的查询窗,互相重叠的查询窗量值不可求和且重叠段无法逐段核销,数值取成员最大(以该成员自身查询窗为基),a~b 为单次范围;原始和与窗来源见明细。",
			"- `n=N cross-window max(each a~b)` = the N instances come from OVERLAPPING query windows: overlapping-window magnitudes never sum and the overlap cannot be deducted per segment, so the value is the member MAX (normalized over that member's own query window), a~b the per-instance range; the raw sum and the window sources live in the detail blocks."},
		// §21.1 CWD-2 ① (huadong_01 revisit E19 witness, 2026-07-07): a merged
		// ×N row whose members span multiple query windows renders NO anchor-
		// window share — this entry says why the % cell is absent on exactly
		// those rows (branch-5 不出密度 template migrated to the %-face).
		{runtimeTraceProjMarkMergedMultiWindowNoShare, runtimeTraceProjLegendGroupCaliber,
			"- 多窗合并行不显示占窗% = 该行成员横跨多个查询窗,合并值与单一锚定窗不同基,不作跨窗除法(时长条仅示意相对量级);成员窗来源见明细。",
			"- multi-window merged rows show no window share = the row's members span multiple query windows, so the merged value shares no base with the single anchor window and is never divided across bases (the bar is relative scale only); the member windows live in the detail blocks' window sources."},
		// DCS E5 (ledger §23/§23.1 H2, cmp_01 E2 witness, 2026-07-08): a
		// semantic row measured in a DIFFERENT query window used to divide its
		// raw duration by the anchor window length ("83% 对锚窗" while the span
		// never touched the anchor window). The % now divides by the row's OWN
		// typed source query window and says so inline; rows whose source
		// window matches the anchor keep the legacy share byte-identically.
		{runtimeTraceProjMarkSemanticSourceWindowShare, runtimeTraceProjLegendGroupCaliber,
			"- 语义行「来自查询窗 X」= 该行数值测自另一查询窗,占窗% 以其自身查询窗为基(非本树锚定窗;时长条仅示意相对量级)。",
			"- a semantic row's 「from query window X」= the row was measured in another query window; its window share divides by that OWN query window, not this tree's anchor window (the bar is relative scale only)."},
		// PTV6-B (聚合幻影修复, 2026-07-06; 修正轮 Med 如实化): the entry
		// discloses exactly what the pipeline does — NEAR-duplicate (≤3%)
		// same-thread overlapping measurements fold as duplicate publications,
		// while clearly-different overlapping measurements legitimately
		// R2-accumulate as multiple spans (B 批 S3 设计依据) — no over-claim
		// that every same-thread overlap is folded away.
		{runtimeTraceProjMarkOverWindowShare, runtimeTraceProjLegendGroupCaliber,
			"- 占窗>100% = 跨CPU/多段累计,可合法超过窗口长度(时长条已封顶);同一线程几乎相同的重复记录(差异≤3%)只计一次,明显不同的重叠段分段累计。",
			"- A >100% window share = a multi-CPU / multi-span cumulative that may legitimately exceed the window (the bar is capped); near-identical same-thread duplicate records (≤3% apart) count once, clearly different overlapping segments accumulate per segment."},
		// PTV5 C10 (#68): the trigger is ≥99% (≤100.1%) on BACKGROUND rows only
		// — the entry states its own bounds instead of "整个窗口" 过宽.
		{runtimeTraceProjMarkWholeWindowIdle, runtimeTraceProjLegendGroupCaliber,
			"- `整窗等待` = 该行几乎覆盖整个窗口(≥99%),多为空闲或常驻等待线程,仅作背景参考。",
			"- `whole-window wait` = the row covers nearly the whole window (≥99%); usually an idle or resident waiting thread, background reference only."},
		{runtimeTraceProjMarkInheritedAttribution, runtimeTraceProjLegendGroupCaliber,
			"- `承自归因` = 该行有效归因承自其所在等待区间,非本行实测。",
			"- `inherited attribution` = the row's effective attribution is inherited from its enclosing wait interval, not measured on this row."},
		// ELIM-GAP 件D (§29.104.15, 2026-07-16): the C5-guarded typed-producer
		// rows' occurrence-segment caliber word — on-demand entry, lit exactly
		// where the word renders (词条-图例双向). DISPLAY-HYG 二轮 (§29.112
		// P3②, 2026-07-17): the former 「可略高于/slightly above」 adverb
		// carried a smallness claim no typed signal backs (the producer gate
		// is eff>cum with NO upper bound) — the entry now states direction
		// only and points at the two printed columns for the actual gap.
		{runtimeTraceProjMarkOccurrenceSegmentAccount, runtimeTraceProjLegendGroupCaliber,
			"- `(发生段账目)` = 该行有效归因按其自身发生段账目核算,可高于窗口投影列的落窗裁剪值(高出幅度以两列实值之差为准,不设固定上界);非承自其所在等待区间,亦不作跨窗声明。",
			"- `(occurrence-segment account)` = the row's effective attribution is computed over its own occurrence-segment accounting and may sit above the window-clipped projection column (by whatever the two printed values differ; no fixed bound is asserted); not inherited from an enclosing wait interval, and no cross-window claim is made."},
		// GATED-CAL 件1 (§29.104.16.1 M3, 2026-07-16): the gated-composite
		// caliber word — on-demand entry, lit exactly where the word renders
		// (词条-图例双向; one word source across the 行2 tail, the bare-tag
		// belt, the ◎ note arm and the window-projection cell).
		{runtimeTraceProjMarkGatedCompositeCaliber, runtimeTraceProjLegendGroupCaliber,
			"- `构成,见明细` = 该数值为多分量构成(runnable(全额)+running(折算)),非单一口径;各分量口径与拆解见该行「有效归因 V = …」分解行或明细块。",
			"- `composite, see the detail blocks` = the value is a multi-component composite (runnable in full + discounted running), not one single caliber; per-component calibers and the split live on the row's attribution breakdown line or its detail block."},
		{runtimeTraceProjMarkIOCaliberNote, runtimeTraceProjLegendGroupCaliber,
			"- `同段IO另有…等口径` = 同一线程同段 IO 的多口径合并显示;数值与证据保留,不重复计入归因;席行数值=最大墙钟成员自值(下界),家族总量见成员行。",
			"- `same-segment IO also measured …` = several calibers of one IO segment folded for display; values and evidence kept, never double counted; the seat value is the largest wall-clock member's own value (a lower bound) — the family total lives on the member lines."},
		{runtimeTraceProjMarkPeriodicSource, runtimeTraceProjLegendGroupCaliber,
			"- `周期性信号源` = 该行是固定周期的信号发生器,期内睡眠为正常节拍;有效归因只计 runnable 与信号迟到量,窗口投影保留原始值。",
			"- `periodic signal source` = this row is a fixed-period signal generator; in-period sleep is normal cadence. Attribution counts only runnable plus signal lateness; the window projection keeps the raw value."},
		{runtimeTraceProjMarkAdjacentStanza, runtimeTraceProjLegendGroupCaliber,
			"- `◇` = 邻近区段:与唤醒链时间相邻,不在唤醒链上。",
			"- `◇` = adjacent stanza: time-adjacent to the wakeup chain, not on it."},
		{runtimeTraceProjMarkBackgroundStanza, runtimeTraceProjLegendGroupCaliber,
			"- `▒` = 背景压力区段:环境证据,不计入链上归因,需结合链上证据解读。",
			"- `▒` = background-pressure stanza: environmental evidence, not chain attribution; read with on-chain evidence."},
		// ELIM-1 R16⑥ (rank_order_v2_design_20260712.md §2.1, GREENLIT
		// 2026-07-12; RANK-U Stage 2): the ◎ overview's promise sentence —
		// 图例是承诺面, wording changes require a ruling. The R7 boundary
		// clause (not a fourth §29.27.1 mark surface) lives on the mark
		// constant; the channel-identity teaching (⛓链上/◇邻近, 基石 B) rides
		// this one entry instead of repeating per row (§29.36.4 冗余判据).
		// §29.61.12 (用户裁定 2026-07-14, INV-SUPPLY 件④). EVOLUTION RECORD: the
		// promise sentence's ordering clause 「按发布有效归因值纯降序」 →
		// 「⛓ 链上块整块在前、◇ 邻近块在后,块内按发布有效归因值降序」 (因果
		// 等级分块: RSPA 后无凭证 ◇ 余段可数值压 ⛓,纯值降序会让其视觉盖过有
		// 凭证因果), and the row-lead channel words gained their glyph-word
		// space (`⛓链上` → `⛓ 链上`, 记号词距).
		// ELIM-V2 方向分组制 (2026-07-18) EVOLUTION RECORD: the ordering clause
		// gains the fix-direction sections (⛓ 块内节=修复方向,节序=节内最大
		// 可消降序) and the standing anti-addition rule 方向间收益不可相加 —
		// the promise sentence moves in lockstep with the new ◎ layout.
		{runtimeTraceProjMarkElimOverview, runtimeTraceProjLegendGroupCaliber,
			"- `◎` = 窗内可消除量总览:跨「链上/邻近」两通道、同尺(目标线程窗内墙钟ms)持值行的导航索引,⛓ 链上块整块在前且按修复方向分节(`▸` 节头,节序=节内最大可消降序、节内按发布有效归因值降序,方向间收益不可相加)、◇ 邻近块在后不分节(行内 `·方向=X` 转录);只转录值、通道身份、口径注记与 [E#] 指针,不铸序数、不佩戴徽章、不跨方向求和、不加冕,榜位与徽章唯一归属见下方主榜;行首 `⛓ 链上`=已证可消除量,`◇ 邻近`=条件可消除上界(因果候选成立时至多好这么多);满格=全区最大值(链上条短=诚实);计数当量/复合分数/背景压力口径不参与汇排,以脚注提及;序数仍不可跨通道比较,可跨通道并列的只是同尺数值。",
			"- `◎` = eliminable-in-window overview: a navigation index over the valued rows of the on-chain/adjacent channels on ONE ruler (the focused thread's in-window wall-clock ms); the ⛓ on-chain block renders whole and in fix-direction sections (`▸` heads; section order = max eliminable desc, published effective attribution desc within each section; gains never add across directions), then the ◇ adjacent block unsectioned (inline `· direction=X` transcription); it only transcribes values, channel identity, caliber notes and [E#] pointers — no ordinals, no badges, no cross-direction sums, no crowns; seats and badges belong solely to the main board below. Leading `⛓ on-chain` = proven eliminable amount; `◇ adjacent` = conditional upper bound (at most this much if the causal candidate holds); the full bar is the board-wide maximum (short chain bars are honest). Count-equivalent / composite-score / background-pressure calibers never join the ranking and are footnoted; ordinals still never compare across channels — only same-ruler values sit side by side."},
		// ELIM-V2 方向分组制 mark entries (2026-07-18; each renders exactly
		// with its ◎ word face — 词条-图例双向):
		{runtimeTraceProjMarkElimDirectionSection, runtimeTraceProjLegendGroupMark,
			"- `▸ <方向> · 最大可消 X ms` = ◎ 链上块的修复方向节头:方向词来自 registry 属性轴闭集,「最大可消」恒为该节最大席值的逐字转录(原始值在其席行本体);节序=节内最大可消降序,节内按发布值降序;节头零序数零徽章,方向间收益不可相加。",
			"- `▸ <direction> · max eliminable X ms` = a fix-direction section head of the ◎ chain block: the direction word comes from the registry attribute-axis closed set, and 「max eliminable」 is the verbatim transcription of the section's largest seat value (the original lives on its member row); sections order by max eliminable desc, members by published value desc; heads carry no ordinal and no badge, and gains never add across directions."},
		{runtimeTraceProjMarkElimSectionSubtotal, runtimeTraceProjLegendGroupMark,
			"- 节头 `小计 X ms(区间互斥)` = 该节成员席的 µs 级求和,仅当每席带忠实 typed 时间包络且两两互斥(包络互斥 ⇒ 支撑段互斥,同段物理时间零重复计费)才发布;小计可由下方成员席行逐 µs 重构;跨方向、跨板、未证互斥一律不发。",
			"- head `subtotal X ms (disjoint intervals)` = the µs-level sum of the section's member seats, published ONLY when every seat carries a faithful typed time envelope and the envelopes are pairwise exclusive (envelope exclusivity ⇒ support exclusivity — no physical time double-billed); the subtotal reconstructs µs-for-µs from the member rows below; never across directions, boards, or unproven exclusivity."},
		{runtimeTraceProjMarkElimSectionNonAddable, runtimeTraceProjLegendGroupMark,
			"- 节头 `成员区间重叠,合计不可直加` = 该节成员的 typed 时间包络实测重叠:直接相加会重复计费同段物理时间,故不发小计,只发最大可消。",
			"- head `member intervals overlap; do not add` = the section members' typed time envelopes measurably overlap: adding the values would double-bill the shared physical time, so no subtotal is published — only the max eliminable."},
		{runtimeTraceProjMarkElimDirectionUnresolved, runtimeTraceProjLegendGroupMark,
			"- `▸ 方向未定/复合` = 链上块尾节(fail-open):registry 未解析或复合方向的席位落此,不猜方向、零小计;席行既有口径注记与脚注原样保留。",
			"- `▸ direction unresolved/composite` = the chain block's tail section (fail-open): seats whose registry direction is unresolved or composite land here — no guessed direction, no subtotal; the rows keep their existing caliber notes and footnotes as they are."},
		{runtimeTraceProjMarkElimCrossDirectionChip, runtimeTraceProjLegendGroupMark,
			"- `·∩[E#]` 与 `· ∩ 跨方向重叠对(…)` 脚注 = 真实 typed 跨方向重叠对的 ◎ 转录:两席作用于同段物理时间,修其一后另一席空间会缩,收益不叠加;完整互指句权威在因果树席行,◎ 只转录;无 typed 重叠对载体则两者均不发。",
			"- `·∩[E#]` and the `· ∩ cross-direction overlap pair(s) (…)` footnote = the ◎ transcription of REAL typed cross-direction overlap pairs: the two seats act on the same physical segment — fixing one shrinks the other seat's headroom, the gains never add; the authoritative full mutual clause lives on the causal-tree rows (◎ only transcribes); with no typed pair carrier neither renders."},
		{runtimeTraceProjMarkElimAdjacentDirectionWord, runtimeTraceProjLegendGroupMark,
			"- ◇ 行内 `·方向=X` = 邻近席修复方向的转录词(同一 registry 闭集词表):◇ 块不分节,方向仍可见;方向未解析的席不佩(不猜)。",
			"- inline `· direction=X` on ◇ rows = the adjacent seat's fix-direction transcription (same registry closed word table): the ◇ block stays unsectioned yet the direction stays visible; unresolved seats wear nothing (never guessed)."},
		{runtimeTraceProjMarkElimAdjacentBlockHead, runtimeTraceProjLegendGroupMark,
			"- `◇ 邻近(条件可消上界 · 不入方向守恒)` = 邻近块头:◇ 席是条件可消除上界,不进入方向守恒种群,也不入任何节小计。",
			"- `◇ adjacent (conditional upper bound · outside direction conservation)` = the adjacent block head: ◇ seats are conditional upper bounds — they never enter the direction-conservation population nor any section subtotal."},
		{runtimeTraceProjMarkElimConservation, runtimeTraceProjLegendGroupMark,
			"- `· 守恒:…(检查器)` / `· 守恒违例:…` = 方向守恒检查器结论的转录:通过态=各方向支撑区间并集皆不超物理窗;违例态=某方向席位支撑区间并集之和超窗(同段物理时间被重复计费),逐方向披露、只披露不改值、永不拦发射。",
			"- `· conservation: … (checker)` / `· conservation excess: …` = the direction-conservation checker's verdict transcribed: pass = every direction's support-interval union fits the physical window; excess = one direction's per-seat support unions sum past the window (same-direction physical time double-billed) — disclosed per direction, values untouched, emission never blocked."},
		// INV-SUPPLY 件① (§29.61.11/.11a, 2026-07-14): the compound type-word
		// suffix's teaching entry — the threshold interpolates from the ONE
		// shared constant (types.TraceSupplyGapDominanceShare) so the legend
		// promise can never drift from the criterion.
		// RNB-1 C-2② (§29.88.10 R7-2, 2026-07-14): the criterion gained the
		// constitutive precondition (eff 构成含 running 折算分量) — the legend
		// sentence moves in lockstep with the predicate (词条-图例双向同步).
		{runtimeTraceProjMarkSupplyGapDominant, runtimeTraceProjLegendGroupCaliber,
			fmt.Sprintf("- `供给缺口主导` = 类型词复合后缀:该席有效归因构成含 running(折算)分量,且已发布的供给折算缺口量 ≥ 有效归因×%.0f%%(两值均为已发布 typed 值,纯比较),席位影响以频点/算力供给缺口成分为主;缺口为独立口径,不与有效归因相加,构成拆解见该行行3与明细;构成中无供给分量的席位不佩此词。", types.TraceSupplyGapDominanceShare*100),
			fmt.Sprintf("- `supply-gap dominant` = a compound type-word suffix: the seat's effective composition CONTAINS a folded running component, and its published supply-fold deficit is ≥ %.0f%% of its published effective attribution (both engine-published typed values, pure comparison) — the seat's impact is dominated by the frequency/compute-supply gap component; the deficit is an independent caliber and never adds to the attribution (the split lives on the row's line 3 and the detail block); a seat whose composition carries no supply component never wears the word.", types.TraceSupplyGapDominanceShare*100)},
		// INV-SUPPLY 件③ (§29.61.11, 2026-07-14): the ◎ leverage note's
		// teaching entry — a transcription of the seat's OWN 行3 attribution
		// split relabeled by leverage direction; a constituent display, never
		// a Σ row (零求和红线).
		// RNB-1 C-3 (§29.88.11 R7a, 2026-07-14) EVOLUTION RECORD: the note
		// relocated from an interstitial sub-line under its seat row to the
		// dedicated ◎ 构成拆解 section (entries `[E#] 可消除构成: …`, board
		// seat order) — the legend sentence names the new home in lockstep;
		// content bytes and the mark semantics are unchanged.
		{runtimeTraceProjMarkElimComposition, runtimeTraceProjLegendGroupCaliber,
			"- ◎ 构成拆解区「可消除构成」条目(按 [E#] 索引对应席行) = 该席有效归因的构成按消除杠杆转录:调度修复=runnable(全额)分量、频点/热策略=running(折算)分量,数值与该行行3拆解逐字同源;是构成陈列,非合计行,不与其他行相加。",
			"- the ◎ composition-breakdown section's 「eliminable composition」 entries (indexed to their seat rows by [E#]) = the seat's OWN attribution split transcribed by elimination lever: scheduling fix = the runnable (in-full) component, frequency/thermal policy = the running (discounted) component — the numbers are byte-identical with that row's line-3 breakdown; a constituent display, never a total row, never added across rows."},
		// PTV5 C00 (#68 用户裁定 2026-07-05): a fallback-sourced main-line ms is
		// identifiable at the point of reading — the inline caliber word reuses
		// the (a)-table caliber vocabulary; the semantics live here.
		{runtimeTraceProjMarkImpactCaliberFallback, runtimeTraceProjLegendGroupCaliber,
			"- 行内 `链上累计`/`有效归因`/`实际状态`/`累计(跨线程)` 口径词 = 该行无窗口投影值,主行数值为所标口径(不显示占窗百分比)。",
			"- An inline `chain total`/`attribution`/`actual state`/`cross-thread cum` caliber word = the row has no window-projection value; the main-line duration is that caliber (no window-share percentage)."},
		// PTV5 Q2 (#68 用户裁定 2026-07-05): the tree-header coverage sentence's
		// caliber gets its own legend entry — attributed = the chain's depth-1
		// cumulative toward the target, residual by subtraction only.
		// §29.61.6 (用户立案 2026-07-14, 词面批): the entry taught the
		// arithmetic but not the epistemic status of 未归因 — a user read it
		// three ways (不需要深挖 / 无需深挖的正常 / 没有深挖的未知). The
		// appended sentence states the ruling meaning: NOT-normal / not
		// needs-no-explanation — the portion not yet covered by any published
		// cause (undiscovered causes / unexplored windows / unrecognized idle
		// possible, the system does not rule), while RECOGNIZED normal idle
		// (∿ 帧间空闲) is split onto its own row and the self running
		// residual never wears the 未归因 word (tree.go 自身执行 ruling).
		{runtimeTraceProjMarkCoverageLine, runtimeTraceProjLegendGroupCaliber,
			"- 已归因/未归因 = 树头覆盖句的口径:只统计第一层直接原因行对关注线程的影响;未归因 = 关注线程等待(或整窗)时长 − 已归因;各层时长在墙钟上互相包含,不能逐层相加;未归因≠正常/无需解释:是尚未被已发布原因覆盖的部分(可能含未发现原因/未探查窗/未识别空闲,系统不判定);已识别的正常空闲(如帧间空闲)另行单列。",
			"- attributed/unattributed = the tree-header coverage caliber: only the depth-1 direct-cause rows' impact on the focused thread is counted; unattributed = the focused thread's wait (or whole-window) duration minus attributed; layer durations contain each other on the wall clock, so layers never add up; unattributed ≠ normal / needs-no-explanation: it is the portion not yet covered by any published cause (it may hold undiscovered causes, unexplored windows or unrecognized idle — no verdict is made); recognized normal idle (e.g. inter-frame idle) is listed on its own row."},
		// §29.27② (COV-4 用户裁定, 2026-07-11): the four-state coverage
		// account's caliber entry — full-window wall-clock partition,
		// window-denominator percentages (a different base from the wait-
		// denominator sentence), IO as an in-state attribution label, the
		// converted supply pointer never joining wall-clock arithmetic
		// (§7.30 S1 负面先例), and the ruling-verbatim running-residual word.
		{runtimeTraceProjMarkFourStateAccount, runtimeTraceProjLegendGroupCaliber,
			"- 全窗四态 = 关注线程在分析窗内的墙钟四态分区(running+runnable+sleep+D-state),四态合计=分析窗;百分比以分析窗为分母,与「已归因/未归因」句的等待分母不同基,各项百分比各自取整,合计可±1%;IO等待 = sleep/D-state 内的归因标签,不另加和;「含未覆盖段 X 折入」 = 该态含窗界外推段(窗首承窗前已证状态的前缀,或窗尾无关闭事件的开区间后缀),无窗内事件对闭合,时长已计入该态本体,此为披露非加项;「供给折算影响」为折算口径,只作对照,不计入墙钟合计;「自身执行(无确定性可优化工作)」= running 中扣除确定性工作后的残余。",
			"- Full-window four states = the focused thread's wall-clock partition over the analysis window (running+runnable+sleep+D-state); the four-state total equals the window; percentages use the window denominator (a different base from the attributed/unattributed wait sentence) and round independently, so they may total ±1%; IO wait is an attribution label inside the sleep/D-state wall clock, never a fifth addend; incl. uncovered segment X folded in = the state contains a window-boundary extrapolated segment (a head prefix carried from the proven pre-window state, or a tail suffix left open with no closing event) that no in-window event pair closes — its duration is already inside the state value, so this is disclosure, never an addend; supply-converted impact is a converted caliber for cross-checking only, never added to the wall clock; own execution (no deterministic optimizable work) is the running remainder after deterministic work."},
		// UXR-1 §29.36.3 (通道4 提及义务, 2026-07-11): the on-chain semantic
		// mention-obligation seat — the SEM-LEAD 提及地板 as an explicit
		// channel member (no silent-disappearance path).
		{runtimeTraceProjMarkSemanticMentionFloor, runtimeTraceProjLegendGroupCaliber,
			"- `优化点·未入根因排序前N` = 链上语义工作(编译/校验/纹理等确定性可优化点)未进根因排序前N,仍按提及义务列示;其量值不参与排序。",
			"- `optimization point · below the top-N root-cause board` = on-chain semantic work (deterministic optimizable work such as compile/verify/texture) outside the top-N root-cause board, listed under the mention obligation; its magnitude never joins the ordering."},
		// PTV5 PTS (#68 用户裁定 2026-07-05): on-chain overflow beyond the bucket
		// cap folds with a count — never a silent drop.
		// EVOLUTION RECORD (P2a rider 件1, §29.55.3 处置更新 + §29.58.2 F2,
		// 2026-07-13): 「其余N项(链上折叠)」→「其余N项(折叠)」 — 边词管车道
		// (链上─ restored on the fold row; the stanza folds wear their section
		// lane word 邻近─/背景─), 行名管折叠 (lane word deduped out of the
		// name), 记号位留形态族 (the state-mark slot keeps inheriting the
		// merged node's impact form — ◦/◌/… carry true information).
		// R9 (§29.93.2, 2026-07-15): 行1 只留计数标签;头名成员(带榜位指针)
		// 下沉行2 「· 成员 …」,其余成员照旧见明细 — 图例句随词面同步。
		{runtimeTraceProjMarkOnChainOverflowFold, runtimeTraceProjLegendGroupCaliber,
			"- `其余N项(折叠)` = 超出逐行上限的项折叠为一行计数,所属车道见行首边词(如 `链上─`/`背景─`),数值取成员最大(墙钟跨线程不可加和);行2 `· 成员 …` 预览头名成员,全部成员见明细与证据索引。",
			"- `N more (folded)` = rows beyond the per-row cap fold into one counted row; the row's leading edge word names its lane (e.g. `on-chain─`/`background─`), and the value is the member MAX (wall clock never sums across threads). Line 2 (`· member …`) previews the head member; all members live in the detail blocks and the evidence index."},
		// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
		// fold family — its value is the members' ACCOUNT Σ (合计 per the user
		// ruling), never the member MAX, so it carries its own legend seat.
		// 修复轮 P2-4/U2 (2026-07-15): honest reach — the detail blocks carry
		// the member roster and the single-seat RANGE, not per-seat values
		// (those stay reachable through each ◇ twin's 同源二分 sentence); the
		// Σ caliber names its account nature (账目相加,非墙钟并集).
		{runtimeTraceProjMarkMicroAnchorFold, runtimeTraceProjLegendGroupCaliber,
			"- `其余N项微额锚定席` = 链上剪切席中单席 <0.1ms 的微额锚定席折叠为一行;数值为各席锚定账合计(账目合计口径:账目相加,非墙钟并集);凭证语义保留(仍属 ⛓ 链上通道);行2 `· 成员 …` 预览头名成员;单席范围与合计见行内,逐席锚定值经各 ◇ 孪生行的同源二分句可达,证据经 [E#] 索引。",
			"- `N more micro anchored seats` = chain-lane anchored cut seats each below 0.1ms fold into one counted row; the value is the seats' account sum (account-sum caliber: accounts added, not a wall-clock union); the credential semantics are preserved (still the ⛓ on-chain channel); line 2 (`· member …`) previews the head member; the single-seat range and the total ride the row, per-seat anchored values stay reachable through each ◇ twin's same-source bipartition sentence, and evidence resolves via the [E#] index."},
		// RNB-5B 件⑨ (§29.96.2 终判⑨, 2026-07-15): the endpoint-less
		// multi-window chip — the typed multi-window fact without endpoint
		// claims (member windows live on the detail 窗来源 lane).
		{runtimeTraceProjMarkMultiWindowNoEndpoints, runtimeTraceProjLegendGroupCaliber,
			"- `多窗(端点见明细)` = 多窗合并席的窗标端点不可解:该席成员来自多个查询窗且无单一供席成员窗可标,端点不猜测;各成员窗见明细「窗来源」。",
			"- `multi-window(endpoints in detail)` = a multi-window merged seat whose window chip endpoints are unresolvable: members span several query windows with no single seat-member window to name, and no endpoints are guessed; per-member windows live on the detail window-source lane."},
		// DISP-2 G19 (§27.5, 2026-07-09): the all-zero fold row's honest
		// one-line note — a member-MAX claim over 0.000–0.000 taught nothing,
		// so the ×N(0.000–0.000)取最大 tag is retired on that shape (EVOLUTION
		// RECORD at the tag fork; members/evidence stay reachable via the
		// detail blocks and the evidence index — zero information loss).
		{runtimeTraceProjMarkAllZeroFoldNote, runtimeTraceProjLegendGroupCaliber,
			"- `窗内无有效时长` = 该折叠行全部成员在窗内均无可计量时长,不作取最大声明;逐成员与证据见明细与证据索引。",
			"- `no in-window effective duration` = every member of that fold carries no measurable in-window duration, so no member-MAX claim is made; members and evidence live in the detail blocks and the evidence index."},
		// G12-ENG (§29.1, 2026-07-09): the MIXED fold's honest range wording —
		// the legacy ×N(a~b) claim over every member fabricated the huadong_79
		// E23 "×2 both at 14.272ms" same-value double from one valued member
		// plus one zero-duration marker aggregate.
		{runtimeTraceProjMarkValuelessFoldMembers, runtimeTraceProjLegendGroupCaliber,
			"- `无时长值` = 该合并/折叠行中无可计量时长的成员;不参与取最大与 a~b 范围,仅计入成员数(「全部无时长值」=所有成员均无);逐成员见明细与证据索引。",
			"- `without measurable duration` = members of that merged/fold row carrying no measurable duration; they never join the member-MAX or the a~b range and only count toward the member total (\"all without measurable duration\" = every member); members live in the detail blocks and the evidence index."},
		// PTV6-C ruling A (#73, 用户裁定 2026-07-06): 有效归因/链上累计 belong to
		// the chain universe — a ◇/▒ stanza row shows the same data under the
		// cross-thread cumulative family word.
		{runtimeTraceProjMarkStanzaCrossThreadCum, runtimeTraceProjLegendGroupCaliber,
			"- `累计(跨线程)` = ◇/▒ 区段行的时长口径:多线程时间累计,不计入链上已归因。",
			"- `cross-thread cum` = the duration caliber of ◇/▒ stanza rows: multi-thread time accumulated, never counted into the on-chain attribution."},
		// PTV8-RCR-B (UXA 域A #19, 2026-07-08): the 折算 half-sentence becomes
		// its own on-demand entry — taught exactly when a discount tag renders.
		{runtimeTraceProjMarkStanzaDiscount, runtimeTraceProjLegendGroupCaliber,
			"- `折算` = 该行折算后的有效值,仅在与累计值不同时并列显示。",
			"- `discounted` = the row's discounted effective value, shown beside the cumulative only when the two differ."},
		// PTV6-D (b) (#75 标本归因 #10) → PTV8-RCR-B (UXA 域A #20 verify /
		// 域B #21, 2026-07-08). EVOLUTION RECORD: the 「候选影响」 class word is
		// retired from every face (行面禁词 pin kept; block field says 未分类) —
		// the entry now self-explains. Typed branch gate unchanged: renders
		// exactly when a row's shape cell fell through to the generic arm.
		{runtimeTraceProjMarkCandidateShapeClass, runtimeTraceProjLegendGroupCaliber,
			"- 无类型词的行 = 未识别出具体影响类型;逐行影响形态见明细。",
			"- Rows without a type word = no concrete impact type was identified; each row's impact-shape line lives in the detail blocks."},
		// §22 B1-b F2 (huadong_01 audit 2026-07-07): a typed user entity inside
		// the long-trunk folded middle is force-expanded to its own row; when
		// the position carries no impact row this run, the row states the
		// user-focus identity instead of the anonymous 中转 token.
		// PTV8-LAD L3 (§24.11 维度A / §24.8 图标化令, 2026-07-08). EVOLUTION
		// RECORD: the 18-cell 「用户关注线程(中转)」 long label out-widened the
		// thread name it existed to disclose (huadong_78 ladder) — the row now
		// wears the 3-cell ⊚中转 short token (⊚ = the root user-focus glyph,
		// EAW-verified single cell, shared constant); the semantics live here.
		{runtimeTraceProjMarkUserFocusTransit, runtimeTraceProjLegendGroupMark,
			"- `" + runtimeTraceProjRootGlyph + "中转` = 折叠段内命中的用户关注线程强制单独成行:该位置为唤醒链中转,本报告未单独计量其影响。",
			"- `" + runtimeTraceProjRootGlyph + "transit` = a user-focus thread inside a folded segment is force-expanded to its own row; the position is a wakeup-chain transit whose impact this report does not measure separately."},
		// §22 PTV7-SPN F5 (用户措辞裁定 2026-07-07, 措辞一字不改; missing_wakeup
		// 图例措辞族 beside the ⊘ entry): the trace_gap diagnostic marker's
		// legend home — the zh face keys on the 数据盲区 display word, the EN
		// face keeps the raw token (D2: EN surfaces render tokens verbatim).
		{runtimeTraceProjMarkTraceGapBlindSpot, runtimeTraceProjLegendGroupMark,
			"- `数据盲区` = 窗内无调度数据,下钻链止。",
			"- `trace_gap` = no scheduler data inside the window; the drill chain ends there."},
		// DISP-2 G2 (§27.2 判据措辞如实, 2026-07-09): the typed no_eligible_wait
		// criterion's own entry — the row IS still a 数据盲区, but the window
		// held scheduler intervals that all sat below the minimum-duration
		// floor, and the legacy "窗内无调度数据" wording over-claimed on that
		// form (复核 P3-5 precise fact). Renders exactly with the forked inline
		// disclosure below.
		{runtimeTraceProjMarkTraceGapBelowFloor, runtimeTraceProjLegendGroupMark,
			"- `窗内无≥阈值等待区间` = 数据盲区判据之二:窗内有调度区间但均低于最小时长阈值,下钻链止。",
			"- `no in-window wait ≥ floor` = the data blind spot's second criterion: scheduler intervals exist in the window but all sit below the minimum-duration floor; the drill chain ends there."},
		// P9 arm c (§29.42 案1 BINDER-MISATTR, 2026-07-12): the frame-pacing
		// idle teaching entry — the pacing_idle row's type word semantics. The
		// segment is NORMAL frame cadence (length ≈ one frame period, ended by
		// a frame-signal dispatch-chain waker), never a peer block and never a
		// root-cause contender.
		{runtimeTraceProjMarkPacingIdle, runtimeTraceProjLegendGroupMark,
			"- `帧间空闲(等待下一帧)` = 该睡眠段长≈一个帧周期且由帧信号分发链唤醒终结:线程在等待下一帧信号,属正常帧节拍;不属对端阻塞,不计入根因排序。",
			"- `pacing_idle (waiting for the next frame)` = the sleep segment's length matches one frame period and it is ended by a frame-signal dispatch-chain waker: the thread is waiting for its next frame tick — normal frame cadence, not a peer block, excluded from root-cause ranking."},
		// 复核 P2-1 (2026-07-12): the generic periodic fork — the waker is a
		// measured periodic signal source (timer/audio style) but NOT on the
		// frame-signal dispatch chain; the frame promise words never render
		// for it.
		{runtimeTraceProjMarkPeriodicIdle, runtimeTraceProjLegendGroupMark,
			"- `周期空闲(等待下一周期信号)` = 该睡眠段长≈唤醒者的实测信号周期:线程在等待下一次周期信号,属正常节拍;不属对端阻塞,不计入根因排序。",
			"- `periodic_idle (waiting for the next periodic signal)` = the sleep segment's length matches the waker's measured signal period: the thread is waiting for its next periodic signal — normal cadence, not a peer block, excluded from root-cause ranking."},
		// CAL-1 件⑤ PACE-ROW + 件⑥b (2026-07-12): the cadence-idle rows'
		// dedicated glyph (bytes from the tracefence directory) and the 行2
		// 「节拍吻合」 typed mint word — 节拍吻合 renders only on rows whose
		// idle token the engine minted under the cadence-fit proof.
		{runtimeTraceProjMarkIconPacing, runtimeTraceProjLegendGroupMark,
			"- `" + tracefence.GlyphPacing + "/节拍空闲` = 周期节拍空闲行(帧间空闲/周期空闲):正常节拍等待,上下文行族,不参与根因排序;其行2「节拍吻合」= 段长与实测节拍周期吻合(引擎 typed 铸造条件)。",
			"- `" + tracefence.GlyphPacing + "/cadence idle` = a cadence-idle row (pacing_idle / periodic_idle): a normal cadence wait in the context-row family, excluded from root-cause ranking; its row-2 「cadence fit」 marks the engine's typed mint condition (segment length matches the measured cadence period)."},
		// V2-P0 (design §6.1 新裁定 A, 2026-07-12): the ⌗ 口径旁栏 teaching
		// entry — the two-scale-ruler red line at row level.
		// CALSIDE-1 件3 (2026-07-19, 用户显示裁定): the entry teaches the two
		// new promises — the in-row class word shares its seat row's single
		// word source (件1), and the non-wall-clock value wears no bar and no
		// window % (件2 F7; the 时长条/占窗% rulers are wall-clock pools).
		{runtimeTraceProjMarkCaliberSideRow, runtimeTraceProjLegendGroupMark,
			"- `⌗口径旁栏` = 计数当量/综合评分类行:该行数值不是墙钟时长(计数当量=按事件计数折算;综合评分=跨单位合成分),不占序数、不参与根因排序、不佩戴徽章;行内类别词与其席行同源;非墙钟数值不画时长条、不标占窗%(不与墙钟同池比较);行照常显示并经 [E#] 互链。",
			"- `⌗ caliber-side` = a count-equivalent / composite-score row: its value is NOT wall-clock time (count equivalent = derived from event counts; composite score = a cross-unit blend); it takes no ordinal, never competes for root-cause ranking and wears no badge; its in-row class word shares its seat row's word source; a non-wall-clock value draws no duration bar and no window % (never pooled against the wall clock) — the row still renders with its [E#] links."},
		// CR-2 组② P5 (2026-07-12): the same-segment mirror teaching entry —
		// one physical segment published on several lanes converges at render.
		{runtimeTraceProjMarkSameSegMirror, runtimeTraceProjLegendGroupMark,
			"- `同段镜像` = 同一物理段/同一物理时间在多条通道重复发布:同段同值的裸状态行已并入所指行(其 [E#] 并入行首括号);合并行行2指向同源家族行(该行 N次(a~b) 为按分组的组合计值区间,单段真值见家族行明细);同值双行形行2互指同一物理时间,两行数值不可相加。",
			"- `same-seg mirror` = one physical segment / one physical time published on several lanes: the same-value raw-state row is merged into the surviving row (its [E#] joins the bracket); a merged row's row 2 points at its source family row (that row's n=N(a~b) range holds per-group sums — true single-segment extrema live in the family row's detail); on the equal-value two-row form, row 2 cross-references the same physical time and the two rows are never additive."},
		// WO-A1 (SMR-1 批, 2026-07-12): the non-additive pointer teaching entry
		// — one physical time on two seats must never be summed by the reader.
		{runtimeTraceProjMarkNonAdditivePointer, runtimeTraceProjLegendGroupMark,
			"- `不可相加指针` = 同线程同状态族的两席共享同一段物理时间:「为[E#]的组成部分/已含[E#]/为[E#]成员」标注从属·包含·成员关系(typed 判定:加法恒等式/成员值 µs 全等/结构性子账),两席数值不可相加;各席账目照发。",
			"- `non-additive pointer` = two seats of one thread's one state family share the same physical time: 「component of [E#] / already contains [E#] / member of [E#]」 marks the subset/containment/membership relation (typed judgment: addition identity / µs-identical member value / structural sub-account); the two seats must never be summed — each keeps its own account."},
		// XERR1-FIX 件1 互指 (§29.104.4, 2026-07-15): the blocking↔sleep
		// mutual-pointer teaching entry (E6/E7 账目关系先例的阻塞等待特化).
		{runtimeTraceProjMarkBlockingWaitSleepRelation, runtimeTraceProjLegendGroupMark,
			"- `等待段含 sleep 分量` = 阻塞等待行的等待段合计(span∩窗内 sleep+D+iowait)与同线程 sleep 席互指:sleep 分量与 sleep 席同段物理时间、两套账目各计一次,两行数值不可相加。",
			"- `wait segments include a sleep share` = the blocking-wait row's wait-segment total (sleep+D+iowait inside span∩window) cross-references the thread's own sleep seat: the sleep share is the same physical time counted once in each of two accounts — the two rows are never additive."},
		// WO-C1 (SMR-1 批, 2026-07-12): the account-relation sentence entry.
		{runtimeTraceProjMarkAccountRelation, runtimeTraceProjLegendGroupMark,
			"- `账目关系` = 同线程同状态族的两行来自两套账目体系(覆盖集不同;物理时间重叠或不相交,行内句按 typed 区间推导如实标注):行内句标出双方口径自述与互指 [E#];两行数值不可直较,双行均为诚实账目(W-A 不同账目绝不折)。",
			"- `account relation` = two rows of one thread's one state family come from two accounting systems (different coverage sets, overlapping physical time): the inline sentence names both calibers and cross-references [E#]; the two values are neither additive nor directly comparable — both rows are honest accounts (W-A: different accounts never fold)."},
		// RSPA §29.61.10a (2026-07-14): the same-source bipartition teaching
		// entry — the 行2 disclosure names the split, this entry names the rule.
		{runtimeTraceProjMarkChainAnchorSplit, runtimeTraceProjLegendGroupMark,
			"- `同源二分` = 一份全窗账按链上凭证拆成 ⛓锚定席 + ◇余段席,两席不相交、相加还原全窗值(唯一可相加形):锚定席=发生段∩typed 唤醒依赖跳变窗(留在链上通道),余段席=全窗−锚定(无链上凭证,记 ◇ 邻近通道)。",
			"- `same-source split` = one full-window account split by its chain credentials into the ⛓ anchored seat + the ◇ remainder seat; the two seats are disjoint and sum back exactly to the full-window value (the ONLY additive form): anchored = segments ∩ typed wakeup-dependency jump windows (stays on the chain channel), remainder = full − anchored (no chain credential, seated on the ◇ adjacent channel)."},
		// RSPA §29.61.10b (2026-07-14): the same-source relation sentence entry
		// — sits beside the generic 账目关系 entry but teaches the OPPOSITE
		// additivity verdict, so it owns its own seat.
		{runtimeTraceProjMarkChainAnchorRelation, runtimeTraceProjLegendGroupMark,
			"- `合计还原全窗账` = 同线程同状态的 ⛓锚定席 与 ◇余段席 互指 [E#]:两席出自同一份全窗账、不相交,相加恰还原全窗值;此关系是唯一可相加的席位对,其余跨行墙钟仍不可相加。",
			"- `restores the full-window account` = the ⛓ anchored seat and its ◇ remainder seat of one thread + state cross-reference [E#]: both halves come from ONE full-window account, are disjoint, and sum exactly back to it — the only additive seat pair; wall clock across any other rows stays non-additive."},
		// RNB-1 (§29.88 R2, 2026-07-14): the case-A' downgraded relation entry
		// — the same-source split's honest form when the chain seat does not
		// hold the anchored share at the same value.
		{runtimeTraceProjMarkChainAnchorDivergent, runtimeTraceProjLegendGroupMark,
			"- `账目关系(锚定权属失合)` = 同源二分的降级形:全窗账仍=锚定+其余(记账精确),但链上席位并未以同值持有锚定份(链席自账Σ与锚定账Σ失合,行内披露双Σ与差值);余段照记 ◇ 邻近,链席另列自账,两席不可相加。",
			"- `account relation (anchored-ownership divergence)` = the downgraded form of the same-source split: the full-window account still equals anchored + remainder (ledger-exact), but the chain seat does not hold the anchored share at the same value (its own Σ diverges from the anchored-ledger Σ; both Σs and the delta are disclosed inline); the remainder stays ◇ adjacent, the chain seat keeps its own separate account — the two seats are never additive."},
		// RNB-1 R4 (§29.88.2, 2026-07-14): the whole-seat demotion entry.
		{runtimeTraceProjMarkChainCredentialDemoted, runtimeTraceProjLegendGroupMark,
			"- `无链上凭证(整席降道)` = 该行整席账目未能出示 typed 因果边锚定份(边=凭证,边前=有效,边后=解除):不可拆分的账目(卫星行/反转改型席/零锚定 D/IO 视图行)整席记 ◇ 邻近,数值零动;锚定份(如有)由正式席位另行代表。",
			"- `no chain credential (whole-seat demotion)` = the row's whole account shows no typed causal-edge anchored share (edge=credential, pre-edge=effective, post-edge=released): an indivisible account (satellite row / inversion-retyped seat / zero-anchored D-IO view row) rides the ◇ adjacent channel whole with values untouched; any anchored share is represented by the formal seats."},
		// HULL-CRED (§29.104 终判③, 2026-07-17): the per-segment-proven fork
		// of the R4 entry — the demotion is adjudicated on the row's COMPLETE
		// typed segment inventory, never on hull endpoints (hull noise).
		{runtimeTraceProjMarkChainCredentialSegmentDisjoint, runtimeTraceProjLegendGroupMark,
			"- `无链上凭证(逐段核验,整席降道)` = 该行携带完整 typed 逐段区间清单,逐段与锚窗(typed 唤醒依赖跳变窗)求交无一段真相交(行包络虽与锚窗有交,交叠全部落在段间空隙——包络端点是嘈声,不作凭证):整席记 ◇ 邻近,数值零动;与「无链上凭证(整席降道)」同族,凭证等级更强(逐段核验而非账级)。",
			"- `no chain credential (per-segment verified; whole-seat demotion)` = the row carries its COMPLETE typed segment inventory and NOT ONE segment truly intersects the anchor windows (typed wakeup-dependency jump windows) — the row's envelope did intersect, but the overlap lies entirely in the gaps between segments (hull endpoints are noise, never credential): the whole seat rides the ◇ adjacent channel with values untouched; same family as `no chain credential (whole-seat demotion)`, with the stronger per-segment adjudication."},
		// HULL-CRED (§29.104 终判③, 2026-07-17): the envelope-tier honest
		// word on the conservative keep-⛓ arms.
		{runtimeTraceProjMarkChainCredentialEnvelope, runtimeTraceProjLegendGroupMark,
			"- `(包络级凭证)` = 该 ⛓ 行的链上通道位由保守判定保留(行包络∩锚窗有交,或无区间时按 pid 级账目凭证),逐段区间清单缺席(成本退化档/旧工件形)或仅存不交的截断前缀(部分清单不交不能证无,ONCHAIN-FIX-2 件3),未经完整逐段∩锚窗核验:通道与数值零动,仅诚实披露凭证粒度;携带逐段清单的 ⛓ 行不佩此词。",
			"- `(envelope-level credential)` = this ⛓ row's chain-lane seat was retained by the conservative verdict (envelope∩anchor-window intersection, or the pid-level account credential on interval-less rows) with the per-segment inventory absent (cost-degraded / legacy ledger shapes) or holding only a non-intersecting truncated prefix (a partial list cannot prove absence) — no COMPLETE per-segment adjudication ran: channel and values untouched, the word only discloses the credential granularity; ⛓ rows carrying their segment inventory never wear it."},
		// ONCHAIN-FIX-2 件3 (Q6 已追认, 2026-07-18): the truncated lower-bound
		// prefix entry — the 下界 caliber word on a prefix-verified keep-⛓ row.
		{runtimeTraceProjMarkChainCredentialTruncatedLowerBound, runtimeTraceProjLegendGroupMark,
			"- `(凭证清单不完整,实际锚定不小于所证)` = 该 ⛓ 行的逐段凭证清单是超帽账组的已核前缀(账本只保前 N 段,溢出闩存):前缀内已有段与锚窗真相交,故链上凭证成立;所证交叠为保守最小——未采集的后续段只会增加不会减少(实际锚定不小于此清单所证);前缀全不交时不判「逐段核验降道」而退「(包络级凭证)」(缺证≠证无)。",
			"- `(credential inventory incomplete; anchored share is at least the proven)` = this ⛓ row's per-segment credential list is the checked PREFIX of a beyond-cap ledger group (the ledger keeps the first N segments, overflow latched): ≥1 prefix segment truly intersects an anchor window, so the chain credential holds; the proven overlap is a conservative minimum — the uncollected later segments can only add, never subtract (the actual anchored share is not below what this list proves); a fully non-intersecting prefix falls back to `(envelope-level credential)` instead of the per-segment demotion (partial evidence never proves absence)."},
		// ONCHAIN-FIX-1 件1 (2026-07-18): the identity-inheritance honest-word
		// entry — the weakest credential tier below the envelope word (no
		// interval at all, identity only; the fabricated overlap it replaces
		// is retired).
		{runtimeTraceProjMarkChainIdentityInheritance, runtimeTraceProjLegendGroupMark,
			"- `身份继承(链窗级,无区间凭证)` = 该 ⛓ 行未发布 typed 区间,仅凭线程身份(其线程是链成员)继承链上通道位(既裁 fail-open 保守面,无凭证形禁猜):不铸重叠值(旧形曾把整节点窗墙钟伪造为 overlap,已废),通道与数值零动,仅诚实披露凭证层级;凭证等级弱于「(包络级凭证)」(彼有 pid 级账目/包络凭证,此仅身份);经逐段/包络判定或降道的行不佩此词。",
			"- `identity inheritance (chain-window tier, no interval credential)` = this ⛓ row published NO typed interval and inherited the chain-lane seat from bare thread identity (its thread is a chain member — the adjudicated fail-open conservative keep: credential-less shapes are never guessed off the chain): no overlap value is minted (the retired pre-fix shape fabricated the whole node-window wall clock as overlap), channel and values untouched, the word only discloses the credential tier; weaker than `(envelope-level credential)` (which holds pid-level account / envelope credential — this row holds identity only); rows carrying a per-segment / envelope verdict or a demotion never wear it."},
		// XLANE-1 件1 (§29.104.1/§29.104.2, 2026-07-15): the represented-by-
		// chain-seat demotion entry — the 行2 sentence names this seat's
		// disposition, this entry names the rule and its boundary against the
		// R4 无链上凭证 form.
		{runtimeTraceProjMarkChainAnchorRepresented, runtimeTraceProjLegendGroupMark,
			"- `锚定份由链席代表(整席降道)` = 该席账目全额锚定于 typed 唤醒依赖窗内(有凭证),且同线程链上席已在链上通道代表同段物理时间:本席为诊断投影整席记 ◇ 邻近、不重复参赛,数值零动;与「无链上凭证(整席降道)」不同——本席有凭证,降道理由是同段物理时间恰一全额席。",
			"- `anchored share represented by the chain seat (whole-seat demotion)` = this seat's whole account is anchored inside typed wakeup-dependency windows (it HAS credential) and the thread's chain-lane seat already represents the same physical time on the chain tier: the seat is a diagnostic projection and rides the ◇ adjacent channel whole without competing again, values untouched; distinct from `no chain credential (whole-seat demotion)` — this seat holds credential, and the demotion reason is one-full-seat-per-physical-time."},
		// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the split pair's
		// account entry — one rule, two row faces (residual seat + demoted
		// constituent row).
		{runtimeTraceProjMarkGatedShareSplit, runtimeTraceProjLegendGroupMark,
			"- `分账(已计入反转席份+残余)` = 同线程 (线程,runnable) 聚合账与反转席分支窗物理重叠的份额已由反转席 gated 复合全额计入:聚合席只以残余份参赛,已计入份降为 ◇ 构成行(指向反转席[E#],不参赛不相加);已计入份+残余份==修前全账(同一段集两不重叠份,可加还原)。",
			"- `split account (inversion-counted share + residual)` = the share of a (thread,runnable) aggregate account physically overlapping the same thread's priority-inversion seat branch windows is already counted in full by that seat's gated composite: the aggregate seat competes with its residual only, the counted share demotes to a ◇ constituent row (pointing at the inversion seat [E#], never competing, never additive); counted + residual == the pre-split account (one segment set, two disjoint shares, additive back)."},
		// LEVELMERGE-1 件2 fail-open (裁定④ 句形): the overlap disclosure.
		{runtimeTraceProjMarkGatedShareOverlap, runtimeTraceProjLegendGroupMark,
			"- `其中X ms与反转席[E#]重叠` = 该聚合账与同线程反转席分支窗物理重叠 X ms(按现有真段区间测得,实际重叠不小于此值),但 typed 区间清单不完整,未做值拆分:主值零动,两行数值不可相加。",
			"- `X ms overlaps the inversion seat [E#]` = X ms of this aggregate account physically overlaps the same thread's priority-inversion seat branch windows (measured over the available real segments — the true overlap is at least this), but the typed interval inventory is incomplete so no value split was performed: the published value is unchanged and the two rows are never additive."},
		// PARTSPLIT-1 (§29.150④ user ruling, 2026-07-19): the R4-mirror-
		// refused gated composite seat's pre-edge-share disclosure — one rule,
		// two faces (the seat-row 行2 分账 sub-line + the ◎ non-seat mention
		// row; ◈ two-face precedent, both light this mark).
		{runtimeTraceProjMarkGatedCompositeEdgeShare, runtimeTraceProjLegendGroupMark,
			"- `边前份披露(R4拒转·整席不拆)` = gated 复合席(优先级反转 runnable 等待族)携边后份时,R4-mirror 门拒绝换道、整席不拆的分账测度披露:边前份 X=凭证边前段合计(凭证:R3 边凭证=宿主自身对目标的窗内 typed 唤醒边),边后份 Y=边界后段合计(边界后,不入链上),X+Y=本席 runnable 全窗账逐 µs 恒等;仅披露——席值/车道/序数零动,边前份与本席已发布值同段、不与之相加;◎ 总览以非席披露行提及(不占序数、不参与节头「最大可消」、不入任何守恒/普查分母)。",
			"- `pre-edge share disclosure (R4 refused conversion · whole seat unsplit)` = the split-MEASURE disclosure on a gated composite seat (priority-inversion runnable-wait family) carrying a post-edge share, where the R4-mirror gate refused the lane conversion and the seat stays whole: pre-edge share X = the segment sum before the credential edge (credential: the R3 edge credential — the host's own in-window typed wakeup edge toward the target), post-edge share Y = the segment sum after the boundary (never on-chain), X + Y == the seat's runnable full-window account to the µs; disclosure only — seat value/lane/ordinal untouched, the pre-edge share covers the same segments as the seat's published value and is never additive to it; the ◎ overview mentions it through a non-seat disclosure row (no ordinal, never inside a section head's max-eliminable, never in any conservation/census denominator)."},
		// RULER2-1 (§29.150② user ruling / R-19-b, 2026-07-19): the self
		// runnable two-ruler cross-row accounting entry — the 行2 sentence
		// states the split, this entry names the rule (同尺可加/跨尺禁加).
		// POOL2-1 件⑥ (§29.160⑥ user ruling 2026-07-20): the entry closes
		// with the two-axis orthogonality declaration — 「墙钟席」-family worn
		// tag words name the seat's VALUE-caliber axis while the 尺 names its
		// BOOKING-lane axis; the §29.158 P3 juxtaposition (a seat wearing
		// 自身·墙钟席 while the sentence books it on the 唤醒边锚尺) is two
		// independent axes speaking at once, not a contradiction.
		{runtimeTraceProjMarkSelfRunnableTwoRuler, runtimeTraceProjLegendGroupMark,
			"- `自身runnable账按两把尺记账` = 目标线程自身的 runnable 席分属两把已闭合的尺:自身墙钟尺(self_wall_clock 口径,自身墙钟区间入链上)与唤醒边锚尺(on_wakeup_chain 口径,typed 唤醒边锚定);同尺内席位同一度量,可加并给同尺小计(逐 µs 恒等),跨尺度量基不同、绝不相加、不给合计数(禁混尺);单尺多席不发此句(既有同尺合并面管),载体缺席静默;「墙钟席」等佩词=该席值的口径轴,「尺」=归账车道轴:两轴独立,同席可各佩其一,非矛盾;句中参与席如其行未铸行2身份行(紧凑合并行),该行行尾佩 根因排序#N 对照记号(同一榜位序数,便于与「N 席」逐行对照)。",
			"- `self runnable account split across two rulers` = the target's own runnable seats ride two CLOSED rulers: the self wall-clock ruler (self_wall_clock caliber — the target's own wall-clock intervals on the chain tier) and the wakeup-edge-anchored ruler (on_wakeup_chain caliber — anchored by typed wakeup edges); seats within ONE ruler share a measure, may add, and publish a same-ruler subtotal (µs identity), while the two rulers measure on different bases — never additive across rulers, no combined total (mixed-ruler sums banned); a single-ruler board never speaks this sentence (the existing same-ruler fold faces own that shape), and absent carriers stay silent; worn tag words like `墙钟席` (wall-clock seat) name the seat's VALUE-caliber axis while the ruler names its BOOKING-lane axis — two independent axes, one seat may wear one of each; no contradiction; a participating seat whose row minted no 行2 identity line (a compact merged row) wears the 根因排序#N cross-reference chip on its row tail (the same board ordinal space, so the sentence's seat count is checkable row-by-row)."},
		// LEVELMERGE-1 件3 (两向互指, 2026-07-18): the aggregate-seat ↔
		// member-occurrence pointer pair.
		{runtimeTraceProjMarkAggregateMemberCrossRef, runtimeTraceProjLegendGroupMark,
			"- `构成段见[E#…]` / `归因已计入[E#](聚合席),本行为构成段,不另计` = 聚合席与其成员逐次行的两向互指:席行数值已计入全部构成段,构成段行为无损明细展示,其物理时间不再另计、不与席行相加。",
			"- `constituent segments at [E#…]` / `attribution already counted at [E#] (the aggregate seat); this row is a constituent segment, not counted again` = the two-way pointer pair between an aggregate seat and its member occurrence rows: the seat value already counts every constituent segment; the member rows are lossless detail display whose physical time is never counted again nor added to the seat."},
		// SPANTOP-1 (§29.131, 2026-07-18): the constituent top-3 sub-row +
		// counted remainder entry — one rule for the whole block.
		{runtimeTraceProjMarkFamilySpanTop, runtimeTraceProjLegendGroupMark,
			"- `成员…单段X ms 行a..b` + `另有 N 段 合计Y ms` = 族席行1合计的构成分解:按窗内墙钟贡献降序列前3个成员 span(单段=该成员窗内墙钟,行a..b=其 trace 行号区间),其余合并为余行;前3单段+余行合计==席行合计(逐µs可加还原,成员永不静默消失);构成子行纯展示——不参赛、不铸席、不佩⛓、不进任何守恒/普查分母,凭证由席行持有。",
			"- `member … segment X ms lines a..b` + `N more segments · total Y ms` = the decomposition of the family seat's 行1 total: the top 3 member spans by in-window wall-clock contribution (segment = that member's in-window wall clock, lines a..b = its trace line range), the rest folded into one counted remainder; top-3 segments + remainder == the seat total (additive back to the µs; members never silently vanish). Constituent sub-rows are pure display — they never compete, never mint a seat, never wear ⛓ and never enter any conservation/census denominator; every credential stays on the seat row."},
		// SPANVIS-1 (user ruling 2026-07-19 定形原则): the ◈ business-span
		// mention entry — one rule for the whole advisory block (tree face +
		// ◎ 旁栏 footnote light the same mark).
		{runtimeTraceProjMarkBusinessSpanMention, runtimeTraceProjLegendGroupMark,
			"- `◈ 业务span提示` = 链上(含自身)业务 span 的时长/频次线索行(树面题「业务span提示」,◎ 总览题「业务优化线索」,同佩此记号):同线程同名 span 全量聚族(含显示帽下未逐行展示的段),列 单次最大/次数/合计(均为窗内墙钟投影原始值)与行号区间;各族合计间不可相加(区间可重叠/嵌套);纯业务视角提示——不参与根因排序、不铸席、不佩⛓、无序数无占比条、不进任何守恒/普查分母;凭证词如实标注该线程的链上依据(自身/链上节点/唤醒边凭证)。",
			"- `◈ business span leads` = duration/frequency lead rows for on-chain (incl. self) business spans (the tree block heads \"business span leads\", the overview footnote heads \"business optimization leads\" — both wear this mark): same-thread same-name spans folded over the FULL inventory (including segments below the display cap), listing max single / count / total (raw in-window wall-clock projections) plus the trace line range; family totals are not additive to each other (intervals may overlap or nest); business-view leads only — never in root-cause ranking, no seat, no ⛓, no ordinal, no bar, and no conservation/census denominator; the credential word states the thread's on-chain basis honestly (self / chain member / wakeup-edge credential)."},
		// XLANE-2 件1 (§29.104.1/.2 定谳④, 2026-07-17): the semantic
		// member-subset demotion entry — the 行2 pointer names the superset
		// seat, this entry names the rule (typed line-range set inclusion; the
		// legacy 不可相加指针 entry above keeps its own three-word closed set).
		{runtimeTraceProjMarkSemanticMemberSubset, runtimeTraceProjLegendGroupMark,
			"- `为[E#]成员子集(整席降道)` = 同板同主体两语义族席位,本席全部成员 span 的 trace 行号集为[E#]席成员集的真子集(typed 行号集包含判定,禁名称匹配):本席账目为其成员子集视图、不重复参赛,数值零动;◎ 总览以专用脚注代表本席。",
			"- `member subset of [E#] (whole-seat demotion)` = two semantic family seats of one board and one subject where this seat's complete member-span trace line-range set is a PROPER SUBSET of [E#]'s (typed line-range set inclusion — never name matching): this seat's account is a member-subset view and does not compete again, values untouched; the ◎ overview represents it through a dedicated footnote."},
		// XLANE-2 件2 (裁定④ 披露式拆分, 2026-07-17): the self-gap semantic-
		// overlap clause entry — the 行内 clause states the fact, this entry
		// names the rule (披露不扣除).
		{runtimeTraceProjMarkSelfGapSemanticOverlap, runtimeTraceProjLegendGroupMark,
			"- `其中 X ms 与语义席[E#]重叠` = 自身缺口席与目标线程自身语义席共享同段物理墙钟的披露:X 为两席 typed 区间交集墙钟,仅披露不扣除(主值零动),两席数值不可相加。",
			"- `of which X ms overlaps semantic seat [E#]` = the self-gap seat and the target's own semantic seat share the same physical wall clock: X is the exact typed interval intersection — disclosure only, never a deduction (values untouched); the two seats are never additive."},
		// AXIOM-V2 件1 (user rulings 2026-07-18): the fix-direction attribute
		// word entry — the 行2 word names the class, this entry names the axis
		// (attribute only; ordinals and values untouched).
		{runtimeTraceProjMarkFixDirection, runtimeTraceProjLegendGroupMark,
			"- `修向 X` = 该席的修复方向归类(registry 属性轴,闭集:调度供给/锁与优先级/IO与依赖/内存/频率与热治理/自身工作量):仅标注修复指导方向,不改变排序、序数与任何数值;方向未定的席不佩戴。",
			"- `fix-direction X` = the seat's repair-direction class (registry attribute axis; closed set: scheduling supply / lock & priority / IO & dependency / memory / frequency & thermal / own workload): guidance annotation only — ordering, ordinals and every value unchanged; unresolved seats wear nothing."},
		// AXIOM-V2 件2 (公理 v2 跨方向重叠=合法共存全额 + 互指披露, user
		// rulings 2026-07-18): the cross-direction mutual-overlap clause
		// entry — the 行内 clause states the pair, this entry names the rule
		// (口径词 同段重叠; overlap ≤ min of the two support unions by
		// construction).
		{runtimeTraceProjMarkCrossDirectionOverlap, runtimeTraceProjLegendGroupMark,
			"- `与[E#](修向 X)同段重叠 Y ms…收益不叠加` = 同线程同窗同板同口径(墙钟)、修复方向不同的两个严格链上全额席,其 typed 支撑区间交集为 Y ms(同段重叠,恒有 Y ≤ 两席支撑区间较小者):跨方向对同段时间的净收益各自合法,修其一后另一席空间会缩,收益不能相加;互指句成对出现(缺任一载体则两边都不发),仅披露不扣除(主值零动);低于显著阈(相对两席较小发布值)的极小重叠不发句,降入记号道保持可审计。",
			"- `overlaps [E#] (fix-direction X) by Y ms … gains do not add` = two strict on-chain full seats of one thread/window/board/caliber (wall clock) across DIFFERENT fix directions whose typed support-interval intersection is Y ms (same-segment overlap; Y ≤ the smaller support union by construction): each direction's net gain over the shared segment is legitimate on its own, yet fixing one shrinks the other seat's headroom — the gains never add; the mutual clauses appear in pairs (a missing carrier drops BOTH sides) and disclose without deducting (values untouched); an overlap below the significance floor (relative to the smaller seat's published value) speaks no clause and demotes to the audit token lane."},
		// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic
		// seat's credential entry — the 行2 sentence names this seat's
		// credential, this entry names the rule. ONCHAIN-3c (2026-07-19): the
		// same mark now also covers the state-seat sibling basis, so the
		// entry names BOTH carrier forms (图例是承诺面 — a state seat wearing
		// the mark under a span-only legend would be a legend lie).
		{runtimeTraceProjMarkHostEdgeAnchored, runtimeTraceProjLegendGroupMark,
			"- `边锚定(宿主→目标)` = 非目标线程的确定性语义 span 或 runnable/D-IO 状态席以宿主自身对目标的窗内 typed 唤醒边为链上凭证(直接裸边或宿主自身链上跳边,凭证沿链传递;边=凭证,边前=有效,边后=解除):边前份计链上席(span 值=边前段窗内投影,状态席值=状态段清单边前份合计;边界=最晚窗内凭证边),跨边按边界二分(边后部分记 ◇ 邻近余段),无边整段留 ◇/▒ 不入链上。",
			"- `edge-anchored (host→target)` = a non-target thread's deterministic semantic span or runnable/D-IO state seat earns its chain seat from the HOST's own in-window typed wakeup edge toward the target (a direct raw edge or the host's own chain-hop edge — the credential travels down the chain; edge=credential, pre-edge=effective, post-edge=released): the pre-edge share seats on-chain (span value = its pre-edge in-window projection; state-seat value = the segment inventory's pre-edge share sum; the boundary is the LATEST in-window credential edge), a boundary-crossing account bisects at the edge (the post-edge share rides the ◇ adjacent remainder), and an edge-less account never enters the chain tier."},
		// WO-B1 (SMR-1 批, 2026-07-12): the occurrence-series note entry.
		{runtimeTraceProjMarkOccurrenceSeries, runtimeTraceProjLegendGroupMark,
			"- `发生段` = 同(线程,状态,对端)的多次独立发生各占一行:行内给出本次发生的墙钟区间与其余次的 [E#] 互指;各段不相交(typed 区间证明),故给出可相加的合计值。",
			"- `occurrence segment` = independent occurrences of one (thread, state, counterpart) identity each keep a row: the note states this occurrence's wall-clock interval and cross-references the sibling [E#]s; the segments are provably disjoint (typed intervals), so the additive series total is stated."},
		// CR-2 组③ P7 (2026-07-12): the episode-scope actual word — inside the
		// analysis window, so deliberately NOT the ⚠ glyph.
		{runtimeTraceProjMarkActualBeyondEpisode, runtimeTraceProjLegendGroupMark,
			"- `实际Xms(超出发生段,窗内)` = 实际状态时长超出该行自身的发生段投影,但整段仍在分析窗内(不跨分析窗,故不标 ⚠);时长条只画该行投影;合并行的实际值为合并种子单次成员(括注 ·单次成员),非族合计。",
			"- `actual X ms (beyond own episode, inside window)` = the state's actual duration exceeds this row's own episode projection while the whole segment stays inside the analysis window (nothing crosses the analysis window, so no ⚠); the bar draws the row projection only; on a merged row the actual is the merge seed's single member (noted single member), never the family total."},
		{runtimeTraceProjMarkActualNoInterval, runtimeTraceProjLegendGroupMark,
			"- `实际Xms(区间未发布)` = 该行另有实际状态时长口径,但其物理区间未随数据发布,不作跨窗判定;时长条只画该行投影;合并行的实际值为合并种子单次成员(括注 ·单次成员),非族合计。",
			"- `actual X ms (interval unpublished)` = the row carries an actual-duration caliber whose physical interval was not published — no window-crossing verdict is claimed; the bar draws the row projection only; on a merged row the actual is the merge seed's single member (noted single member), never the family total."},
		// PTV8-RCR-A (§24.1/§24.2, 2026-07-08). EVOLUTION RECORD: the §21 RNB
		// R1 `⧖ runnable …gated 分量,不重复计入排序` sub-row entry and the
		// §21/§22 RNB R2 `同段rank行并入` note entry are RETIRED — the
		// four-line cause-node grammar below replaced both seats (the runnable
		// component rides the 拆解子行, the folded rank row's rank/confidence/
		// E# rise into 行2/行1). The entries below carry the new grammar.
		// R5 措辞补充 (rank_order_v2_design_20260712.md §8 R5, GREENLIT
		// 2026-07-12): the former sentence could read as "values are not
		// comparable either" — the supplement states the §29.36.2 boundary
		// precisely: ordinals never compare across channels; same-ruler
		// wall-clock VALUES may sit side by side (the ◎ overview is that face).
		// C2④ 裁定③ (§29.104.17 ③, sweep M10, 2026-07-17). EVOLUTION RECORD:
		// the confidence-tier disclosure clause joins this entry (the 行2 chip's
		// definition home) — 置信档 is each evidence lane's numeric confidence
		// folded through fixed thresholds (runtimeTraceProjConfidenceTier, one
		// implementation, three faces); lanes assign different confidence
		// constants, so 板#1 置信中 beside 板#2 置信高 (witness
		// cust_span_vs_prio) is a lane-baseline artifact, never a cross-row
		// evidence-strength verdict and no basis for overturning seat order.
		// Lane-constant convergence itself stays a deferred ruling (裁定③ 缓).
		{runtimeTraceProjMarkCauseIdentityRow, runtimeTraceProjLegendGroupCaliber,
			"- 成因行身份行「类别·根因排序#N·置信」 = 该行参与根因排序的类别、榜位与置信档;「邻近影响#N」为 ◇ 邻近区段自己的独立排序(同线程墙钟口径),与「根因排序#N」不可跨通道比较(序数不可跨通道比较;可跨通道并列的只是同尺墙钟数值,◎ 总览即此);▒ 背景行不设榜位。同段被 rank 与链两车道各发一行时已合并为一行,rank 行的 E# 并入行尾 [E#+E#],数值不重复计入。置信档(高/中/低)=各证据车道数值置信按固定阈值折词,不同车道基准不同,不作跨行证据强度比较(置信档差异不作为推翻榜位次序的依据)。",
			"- A cause row's identity line 「category · root-cause rank #N · confidence」 = the row's ranking category, seat and confidence tier; 「adjacent-impact #N」 is the ◇ adjacent stanza's OWN independent ordering (same-thread wall-clock caliber), never comparable with 「root-cause rank #N」 (ordinals never compare across channels; only same-ruler wall-clock values sit side by side — the ◎ overview is that face); ▒ background rows carry no seat. A segment published on both the rank and the chain lane is already ONE row here, with the rank row's E# merged into the trailing [E#+E#] and no value double-counted. The confidence tier (high/mid/low) = each evidence lane's numeric confidence folded through fixed thresholds; lanes use different baselines, so the tier never compares evidence strength across rows (a tier difference is no basis for overturning seat order)."},
		// SELF-SEM (§29.61.1 user ruling, RANK-U Stage 1, 2026-07-13): the
		// self-basis qualifier's teaching seat — renders exactly when the
		// qualifier renders (typed node.OnChainBasis single field).
		{runtimeTraceProjMarkSelfDeterministicBasis, runtimeTraceProjLegendGroupCaliber,
			"- `自身·确定性优化` = 目标线程自身运行段内的确定性语义工作(类校验/JIT/着色器编译等):在查询窗内即按链上通道参与根因排序,数值为窗内投影并集(自身墙钟,已证可消除量);该行不含任何唤醒边、不宣称跨线程唤醒关系。",
			"- `self·deterministic-optimization` = deterministic semantic work inside the target thread's own running segments (class verification / JIT / shader compile …): in-window it competes on the on-chain root-cause channel with its window-projection union value (the target's own wall clock, a proven eliminable amount); the row carries NO wakeup edge and claims no cross-thread wakeup relation."},
		// SELF-ALL (§29.61.2/§29.61.2a user rulings, 2026-07-13): the wall-clock
		// self-basis qualifier's teaching seat — renders exactly when the
		// qualifier renders (typed node.OnChainBasis single field).
		{runtimeTraceProjMarkSelfWallClockBasis, runtimeTraceProjLegendGroupCaliber,
			"- `自身·墙钟席` = 目标线程自身的墙钟等待/占用席位(D状态/IO/runnable/running 等):其墙钟区间在查询窗内即按链上通道参与根因排序;该行不含任何唤醒边、不宣称跨线程唤醒关系;有效归因与其他链上行同一阶梯(running=供给折算、runnable=全额、D/IO=墙钟合计)。",
			"- `self·wall-clock-seat` = the target thread's own wall-clock wait/occupancy seat (D-state / IO / runnable / running …): with its wall-clock interval inside the query window it competes on the on-chain root-cause channel; the row carries NO wakeup edge and claims no cross-thread wakeup relation; its effective attribution rides the same ladder as every other on-chain row (running=supply-folded, runnable=in full, D/IO=wall-clock sum)."},
		// SELF-LANE (§29.58.3 处置 a, 2026-07-13): the relocated non-chain self
		// row's qualifier — placement moved, channel identity did not.
		{runtimeTraceProjMarkSelfNonChainSeat, runtimeTraceProjLegendGroupCaliber,
			"- `非链` = 目标自身的非链上行(无链上证明或非墙钟口径):显示归位自身段(「邻近区段」词面只留其它线程),其通道身份、口径词与序数不变。",
			"- `non-chain` = the target's own row without on-chain proof (or on a non-wall-clock caliber): it is displayed inside the self stanza (the adjacent stanza's wording is reserved for OTHER threads), while its channel identity, caliber words and ordinals stay unchanged."},
		// SELF-LANE (§29.58.3 处置 b, 2026-07-13): the cross-channel pointer
		// pair's teaching seat.
		// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): the third pointer word —
		// a pointer at the target-self ⌗ side-rail row says 口径旁栏行 (the
		// 邻近席 word claimed a channel seat the self_caliber_side token
		// retires); same mark, one teaching seat.
		{runtimeTraceProjMarkCrossChannelPointer, runtimeTraceProjLegendGroupCaliber,
			"- `本线程另有链上席/邻近席/口径旁栏行 [E#]` = 同一线程在另一处还有席位的互指指针:两席各记各账(通道/口径不同,不可相加),指针只帮助区分刻意分账与重复;「口径旁栏行」指向自身计数当量/复合分数行(非因果通道席)。",
			"- `this thread also holds an on-chain/adjacent seat / caliber side-rail row [E#]` = the mutual cross-seat pointer for one thread seated in more than one place: each seat keeps its own account (different channels/calibers, never additive); the pointer only distinguishes deliberate segmentation from duplication; the side-rail form points at the target's own count-equivalent/composite-score row (not a causal-channel seat)."},
		{runtimeTraceProjMarkEffectiveBreakdown, runtimeTraceProjLegendGroupCaliber,
			"- `有效归因 V = …` 分解行 = 有效归因的构成:各分量按括注口径计入,分量计入之和恒等于 V;其下「分量 原始 → 计入(口径)」子行为逐项拆解。",
			"- The `attribution V = …` breakdown line = the composition of the effective attribution: each component counts under its parenthesized caliber and the counted parts sum exactly to V; the 「component raw → counted (caliber)」 sub-rows underneath unpack it item by item."},
		// PTV8-RCR-B (UXA 域A #31, 2026-07-08): the 有效归因 word itself gets a
		// legend seat — the row tag appeared 6× per specimen before its
		// definition; taught exactly when the tag renders (new mark).
		{runtimeTraceProjMarkEffectiveAttributionTag, runtimeTraceProjLegendGroupCaliber,
			"- `有效归因` = 该行计入根因排序的影响时长(完整口径见关键指标表)。",
			"- `attribution` = the impact duration this row contributes to the root-cause ranking (full caliber in the key-metric table)."},
		{runtimeTraceProjMarkCaliberFull, runtimeTraceProjLegendGroupCaliber,
			"- `全额` = 该分量按原始时长全额计入。",
			"- `in full` = the component counts at its full raw duration."},
		{runtimeTraceProjMarkCaliberGlobalMaxFmax, runtimeTraceProjLegendGroupCaliber,
			"- `折算,按全域最大核最高频`/`按全域最大核最高频折算`(簇结构不可判时写 `全域最高频`)= 该数值按全域最大核最高频点(全 trace 治理时间线)为基准折算,非原始时长;计入值与供给折算缺口同源可互推(单基准单算法);`运行频点非最高` = 该段实际运行核/频点低于该基准(缺口成因)。",
			"- `discounted, at the global max-core peak frequency` / `folded at the global max-core peak frequency` (`the global peak frequency` when the cluster structure is unjudged) = the value is folded against the global max core's peak frequency point (full-trace governance timeline), not a raw duration; the counted value and the supply-fold deficit are one fold (single basis, single algorithm); `running below peak frequency` names the gap's cause."},
		// §24.1补 (用户问"下界"何意, 2026-07-07 — 图例文案账本原文一字不改).
		// CAP (§26 C3, 2026-07-08). EVOLUTION RECORD: the original half-sentence
		// "折算未计大核单周期优势" is RETIRED (negative pin) — the capability
		// fold now prices the core-class advantage (default table or, once
		// wired, measured evidence), so the lower bound's only residue is the
		// missing-frequency slices counted 0. Rows on the fail-loud freq_only
		// fallback say so inline ("按纯频率比折算") and are the stated
		// exception.
		{runtimeTraceProjMarkCaliberLowerBound, runtimeTraceProjLegendGroupCaliber,
			"- `下界` = 保守最小值:频率数据缺失的片段计 0;核类算力差已计入(默认或实测,标注「按纯频率比折算」的行除外);真实可消除量只多不少。",
			"- `lower bound` = a conservative minimum: slices with missing frequency data count 0; the core-class capability gap is already priced in (default or measured — rows marked 「frequency-ratio fold only」 excepted); the truly removable amount can only be larger."},
		// CAP (§26 C3, 2026-07-08): the capability disclosure words' legend
		// seats — 默认表粗算必须披露, and the fail-loud freq_only fallback
		// teaches what it did NOT price.
		{runtimeTraceProjMarkCaliberDefaultCapability, runtimeTraceProjLegendGroupCaliber,
			"- `按默认算力比粗算` = 核类算力差按默认比例计入(同频点:中核=小核×2.3,大核=中核×1.1≈小核×2.53,超大核=大核×1.2≈小核×3.036),非厂商实测算力表。",
			"- `default capability-ratio estimate` = the core-class capability gap is priced with the default ratios (at one frequency point: middle=small×2.3, big=middle×1.1≈small×2.53, prime=big×1.2≈small×3.036), not a vendor-measured capability table."},
		// EVOLUTION RECORD (UXR-1 §29.36.4 ①): the entry teaches both row
		// forms — the caliber-suffix long form and the compressed no-deficit
		// parenthetical (簇结构不可判,按频率比); the expanded semantics live
		// here so the row never repeats them.
		{runtimeTraceProjMarkCaliberFreqOnlyCapability, runtimeTraceProjLegendGroupCaliber,
			"- `按纯频率比折算`/`按频率比` = 簇结构不可判、或仅单簇有频点采样(单簇内频点等价):核类算力差未计入,仅按频率比对全域最高频点(全 trace)折算(该形下不写核类词);真实缺口只多不少。",
			"- `frequency-ratio fold only` / `frequency-ratio basis` = the cluster structure could not be judged, or only a single cluster carries frequency samples (equivalent within one cluster) — the core-class capability gap is NOT priced — the fold uses the frequency ratio alone against the global peak frequency point (full trace; no core-class word in that form); the true deficit can only be larger."},
		// CAP-2 (§28.4/§28.5, 2026-07-09): the two structure-evidence upgrade
		// words — each entry names its membership provenance AND keeps the
		// default-ratio coarseness disclosure (图例单点承载).
		{runtimeTraceProjMarkCaliberComovementTopology, runtimeTraceProjLegendGroupCaliber,
			"- `按实测频点共动分簇折算` = 簇成员按 trace 内 cpu_frequency 实测频点共动分组(同变化点同值合并);核类算力差按默认算力比计入,非厂商实测算力表。",
			"- `measured co-moving frequency clusters` = cluster membership is measured from co-moving in-trace cpu_frequency change points (equal timelines merge); the core-class gap is priced with the default capability ratios, not a vendor-measured table."},
		{runtimeTraceProjMarkCaliberKeyedRailTopology, runtimeTraceProjLegendGroupCaliber,
			"- `按簇轨实测折算(成员按锚点连续推定)` = 簇频率取自 cpu_id 键控簇轨实测时间线(六重结构门全过);簇成员按锚点连续分段推定,非逐核实测;核类算力差按默认算力比计入。",
			"- `measured cluster-rail fold (membership by anchor contiguity)` = cluster frequency comes from a cpu_id-keyed rail timeline (all six structural gates passed); membership is PRESUMED by anchor contiguity, not measured per core; the core-class gap uses the default capability ratios."},
		// DISPLAY-WRAP 件③(a) (§29.104.18.1 B2, 2026-07-16): the same-node
		// repeat-suppression reference words — the node's FIRST occurrence
		// keeps the full caliber phrase; the entry teaches what the later
		// short words point back to (值与口径零变).
		{runtimeTraceProjMarkCaliberStatedBasis, runtimeTraceProjLegendGroupCaliber,
			"- `按前述基准` = 同一节点上文已全拼的同一折算基准(按全域最大核最高频)的免重复短写;基准、口径与数值零变。",
			"- `at the stated basis` = repeat-free shorthand for the SAME fold basis this node already spelled in full (the global max-core peak frequency); basis, caliber and values unchanged."},
		{runtimeTraceProjMarkCaliberStatedClustering, runtimeTraceProjLegendGroupCaliber,
			"- `分簇口径同前` = 同一节点上文已全拼的同一分簇口径(按实测频点共动分簇折算)的免重复短写;口径与数值零变。",
			"- `same clustering caliber as stated` = repeat-free shorthand for the SAME clustering caliber this node already spelled in full (measured co-moving frequency clusters); caliber and values unchanged."},
		// TOMBSTONE (R5 §29.88.12, 2026-07-15): the demoted-reference basis
		// legend seat (按小核/中核/超大核满频折算, CAP 复核 F1) retired with
		// its words — the R5 trace-global basis never demotes.
		{runtimeTraceProjMarkCaliberSingleMax, runtimeTraceProjLegendGroupCaliber,
			"- `单次最大(a~b,共N次)` = 合并的 N 次实例中取单次最大者计入有效归因,a~b 为单次范围;行1 的 N次 计数与数值为合并计数与合并投影。",
			"- `single max (a~b, of N)` = of the N merged instances, the single largest one counts into the attribution, a~b is the per-instance range; the n=N count and the value on line 1 are the merged count and the merged projection."},
	}
	return append(catalog, runtimeTraceProjImpactFormLegendEntries()...)
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
	// PTV8-RCR-B (UXA 域A layout-②, 2026-07-08): frequency order used to split
	// the state-icon family apart (☾/⚙ separated by 🎯/状态标签) — the group
	// now sorts by semantic family FIRST (state icons as one readable cluster),
	// then by emission frequency inside each family (stable catalog order on
	// ties, unchanged).
	stateIconFamily := map[runtimeTraceProjMark]bool{
		runtimeTraceProjMarkIconSleep:    true,
		runtimeTraceProjMarkIconRunnable: true,
		runtimeTraceProjMarkIconRunning:  true,
		runtimeTraceProjMarkIconDState:   true,
		runtimeTraceProjMarkStateLabel:   true,
	}
	family := func(m runtimeTraceProjMark) int {
		if stateIconFamily[m] {
			return 0
		}
		return 1
	}
	sort.SliceStable(markEntries, func(i, j int) bool {
		if fi, fj := family(markEntries[i].Mark), family(markEntries[j].Mark); fi != fj {
			return fi < fj
		}
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
	// PTV8-RCR-B (UXA 域A 漏审B, 2026-07-08): the 「(按出现频次)/(按树内出现
	// 顺序)」 group-header parentheticals were renderer-internal notes with zero
	// customer information — dropped.
	appendGroup("- 边:", "- Edges:", edges)
	appendGroup("- 记号:", "- Marks:", markEntries)
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

// runtimeTraceProjUserEntityTrunkIndexes maps the projection's typed
// user-entity path hits (§22 B1-b F2, WakeupPathUserEntityHits — indexes into
// the RAW projection.WakeupPath) onto TRUNK indexes (trunk[i] = path[len-2-i],
// the root element itself is not a trunk member). The clean pass drops blank
// elements, so the mapping walks the raw path with a parallel clean counter —
// never re-deriving entity matches (the compile-root comparator is the single
// source). nil when there are no hits (the no-signal fold stays byte-stable).
func runtimeTraceProjUserEntityTrunkIndexes(projection types.TraceCausalProjection, cleanPath []string) map[int]bool {
	if len(projection.WakeupPathUserEntityHits) == 0 || len(cleanPath) < 2 {
		return nil
	}
	hits := map[int]bool{}
	for _, idx := range projection.WakeupPathUserEntityHits {
		hits[idx] = true
	}
	out := map[int]bool{}
	clean := 0
	for raw, item := range projection.WakeupPath {
		if strings.TrimSpace(item) == "" {
			continue // dropped by runtimeTraceCausalProjectionCleanPath — keep index parity
		}
		if hits[raw] && clean <= len(cleanPath)-2 {
			out[len(cleanPath)-2-clean] = true
		}
		clean++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runtimeTraceProjFoldSeg is one typed fold segment of the long-trunk middle
// (PTV8-LAD L1, §24.11 维度A): a PLAIN segment (CycleLen == 0 — the legacy
// "…省略N节点" roster row) or a CYCLE segment (CycleLen > 0 — a consecutive
// CycleLen-tuple repeating CycleCount times, rendered as ONE "↺ 循环×N: A ⇄ B"
// row). Both cover trunk[start, End).
type runtimeTraceProjFoldSeg struct {
	End        int
	CycleLen   int // tuple size k (1..3); 0 = plain fold segment
	CycleCount int // consecutive repeats N (≥2 when CycleLen > 0)
}

// runtimeTraceProjFoldSegments splits the long-trunk fold range
// [omitStart, omitEnd) into typed fold segments (PTV8-LAD L1 run-length lane,
// §24.11 维度A — replacing the per-hit splitter whose segment-local view had
// no channel for the huadong_78 (user⇄VSync)×5 repetition):
//   - run-length detection first: at each position the maximal-coverage
//     consecutive k-tuple run (k ≤ 3, canonical node equality, ≥2 full
//     repeats; ties prefer the smaller k) becomes a CYCLE segment;
//   - §22 B1-b F2 as revised by §24.11 (F2 展开一次): a forced user-entity
//     index INSIDE a cycle run merges into the cycle row — the row names every
//     tuple member in full, so it carries the identity-disclosure duty; forced
//     indexes OUTSIDE cycle runs keep their own force-expanded row exactly as
//     before (returned in expanded);
//   - remaining unforced stretches become PLAIN segments (no repetition → the
//     legacy single roster row, byte-stable with the pre-LAD fold).
//
// The detection is generic run-length over canonical node names — never a
// VSync/user-entity special case; once the P0-E engine fix publishes true
// branches the detector goes naturally inert. omitStart < 0 (short trunk) →
// nil, nil.
func runtimeTraceProjFoldSegments(trunk []string, omitStart, omitEnd int, forced map[int]bool) (map[int]runtimeTraceProjFoldSeg, map[int]bool) {
	if omitStart < 0 {
		return nil, nil
	}
	canon := func(i int) string { return runtimeTraceCausalProjectionCanonicalNode(trunk[i]) }
	// Pass 1: greedy left-to-right maximal run detection over the fold range.
	segments := map[int]runtimeTraceProjFoldSeg{}
	covered := map[int]bool{}
	for i := omitStart; i < omitEnd; {
		bestK, bestReps := 0, 0
		for k := 1; k <= 3 && i+2*k <= omitEnd; k++ {
			reps := 1
			for i+(reps+1)*k <= omitEnd {
				match := true
				for j := 0; j < k; j++ {
					if canon(i+j) != canon(i+reps*k+j) {
						match = false
						break
					}
				}
				if !match {
					break
				}
				reps++
			}
			if reps >= 2 && reps*k > bestReps*bestK {
				bestK, bestReps = k, reps
			}
		}
		if bestK == 0 {
			i++
			continue
		}
		end := i + bestK*bestReps
		segments[i] = runtimeTraceProjFoldSeg{End: end, CycleLen: bestK, CycleCount: bestReps}
		for j := i; j < end; j++ {
			covered[j] = true
		}
		i = end
	}
	// Pass 2: forced indexes outside cycle runs keep their expanded row; the
	// unforced uncovered stretches group into plain segments.
	expanded := map[int]bool{}
	segStart := -1
	flush := func(end int) {
		if segStart >= 0 {
			segments[segStart] = runtimeTraceProjFoldSeg{End: end}
			segStart = -1
		}
	}
	for i := omitStart; i < omitEnd; i++ {
		if covered[i] {
			flush(i)
			continue
		}
		if forced[i] {
			flush(i)
			expanded[i] = true
			continue
		}
		if segStart < 0 {
			segStart = i
		}
	}
	flush(omitEnd)
	return segments, expanded
}

// runtimeTraceProjTrunkPlainStateOccurrence (GAP-B G5, §27.3
// real_trace_campaign_20260705.md, 2026-07-09) reports whether a trunk-subject
// node is a PLAIN scheduler-state occurrence row — the only shape the trunk's
// ×2 same-(thread,state) fold may consume. Precise typed signals only: a
// registered dominant-state token, and NONE of the special display grammars
// (already-merged ×N/union/cross-window forms, duplicate publications, engine
// family contenders, supply-fold accounting, gated inversion composites, lock
// rows) — those forms carry engine accounting a display-side re-merge would
// corrupt, so they fail open to the legacy sibling/cause rendering.
func runtimeTraceProjTrunkPlainStateOccurrence(node types.TraceCausalProjectionNode) bool {
	state := strings.ToLower(strings.TrimSpace(node.StateKind))
	if state == "" || !types.TraceStateKindRegistered(state) {
		return false
	}
	// 复核 P1-1 (2026-07-09): a wakeup_causal_aggregate row is a DERIVED VIEW
	// whose per-hop member rows are fully retained beside it (the engine
	// publishes both; neither family carries a type= note, so the TypeToken
	// pair compares equal-empty) — merging the view with its own members
	// double-counts the identical wall clock (REPRO: agg 5.335 + occ 4.431 +
	// occ 0.904 → ×3 "10.670ms", exactly 2× the truth). Precise typed
	// predicate exclusion: an aggregate view is never a plain occurrence.
	if strings.TrimSpace(node.Predicate) == "wakeup_causal_aggregate" {
		return false
	}
	// RNB-2 件2 (§29.88 W3 病①, 2026-07-15): a re-anchored bipartition seat
	// (◇ remainder / ⛓ clipped) or an R4 lane-demoted seat carries engine
	// account identity a display-side re-merge would corrupt — same fail-open
	// direction as the special grammars below (the R2 pass forks these on the
	// anchorForm group key; the trunk ×2 fold excludes them outright).
	if node.ChainAnchorFullMS > 0 || node.ChainAnchorRemainderSeat || node.ChainCredentialLaneDemoted ||
		node.ChainAnchorRepresentedByChainSeat {
		return false
	}
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): a gated-share split half (the
	// residual seat and the demoted constituent row alike) or an overlap-
	// disclosure seat carries the same engine account identity — a trunk
	// re-merge would re-Σ residual and full calibers (identical fail-open
	// direction as the RNB-2 exclusion above; the aggregate-lane groups fork
	// on the same accounts via traceCausalProjectionAnchorFormKey).
	if node.GatedShareFullMS > 0 || node.GatedShareConstituentSeat || node.GatedShareOverlapDisclosureMS > 0 {
		return false
	}
	return node.MergedCount <= 1 && node.DuplicatePublications <= 1 &&
		!node.OnChainOverflowFold && !node.MergedIntervalUnion && !node.MergedCrossWindowMax &&
		node.FamilyMemberCount == 0 && !node.SupplyFoldComputed &&
		node.GatedRunnableMS == 0 && node.GatedRunningDeficitMS == 0 &&
		strings.TrimSpace(node.BlockingKind) == ""
}

// runtimeTraceProjTrunkSameStateOccurrencePair reports whether extra is a
// re-occurrence of main's OWN (thread, dominant-state, cause-token) identity —
// same canonical StateKind + Object + TypeToken on two plain occurrence rows
// (subject equality is the caller's bucket key). Periodic/undrillable
// mismatches never merge (their grammars would chimera).
func runtimeTraceProjTrunkSameStateOccurrencePair(main, extra types.TraceCausalProjectionNode) bool {
	if !runtimeTraceProjTrunkPlainStateOccurrence(main) || !runtimeTraceProjTrunkPlainStateOccurrence(extra) {
		return false
	}
	if main.PeriodicSource != extra.PeriodicSource ||
		strings.TrimSpace(main.UndrillableReason) != strings.TrimSpace(extra.UndrillableReason) {
		return false
	}
	canon := runtimeTraceCausalProjectionCanonicalNode
	return canon(main.StateKind) == canon(extra.StateKind) &&
		canon(main.Object) == canon(extra.Object) &&
		canon(main.TypeToken) == canon(extra.TypeToken)
}

// runtimeTraceProjTrunkFoldSameStateOccurrences (GAP-B G5, §27.3, 2026-07-09)
// folds a trunk subject's same-(thread, dominant-state) occurrence rows into
// its main row as the established R2 ×N form (SUM value + per-instance a~b
// range, via the ONE types-side merge authority). WHY THRESHOLD 2 while the R2
// aggregation pass keeps ≥3: rendering a thread's second same-state occurrence
// as its own "├─成因─" child claims the thread CAUSED ITSELF (semantic error —
// huadong_79 witness: OS_mmi_EventHdr sleep 0.904 hung under its own sleep
// 4.431), which must be eliminated at the first repetition; the R2 ≥3
// threshold is a row-count economy for LIST renders, not an error repair, so
// it stays untouched. Different-state extras keep their honest cause-
// decomposition edge (状态分解是真实拆解, never a self-cause claim).
func runtimeTraceProjTrunkFoldSameStateOccurrences(main types.TraceCausalProjectionNode,
	extra []types.TraceCausalProjectionNode) (types.TraceCausalProjectionNode, []types.TraceCausalProjectionNode) {
	if len(extra) == 0 {
		return main, extra
	}
	members := []types.TraceCausalProjectionNode{main}
	kept := make([]types.TraceCausalProjectionNode, 0, len(extra))
	for _, node := range extra {
		if runtimeTraceProjTrunkSameStateOccurrencePair(main, node) {
			members = append(members, node)
			continue
		}
		kept = append(kept, node)
	}
	if len(members) == 1 {
		return main, extra
	}
	return types.TraceCausalProjectionMergeOccurrenceRows(members), kept
}

// runtimeTraceProjTrunkDomainAdmit is THE chain-domain admission gate (P0-E
// branch arm + GAP-B G4 window arm, unified for 复核 P2-1, 2026-07-09):
// whether a node's typed (branch, window) identity is consistent with the
// elected trunk's chain domain. ONE helper consumed by BOTH capture surfaces —
// the depth attach AND the same-name trunk consumption (P2-1 REPRO: a W2 node
// whose canonical subject collides with a W1 trunk subject hijacked the trunk
// main/extra selection through the name-keyed pass, which carried NEITHER
// gate). A rejected node is simply not consumed — it keeps its honest
// 父节点未确认/stanza seat downstream.
//
// Branch arm (P0-E 复核收尾①, ledger §22.1): on a BRANCH trunk only
// SAME-BRANCH nodes are domain members — ChainBranch==0 is NOT a legacy pass
// (the engine honestly stamps 0 on cross-branch aggregates and the note
// zero-drops, so 0 conflates "no identity" with "known cross-branch";
// 有损信号禁作硬门). A branch-stamped node on a LEGACY trunk has no trunk
// identity to verify against — rejected.
//
// Window arm (GAP-B G4, §27.2): on a WINDOWED trunk the node's query-window
// identity must MATCH (typed endpoints, ONE shared tolerance). Zero-value arm
// audited separately (§22.2 教训): a window-less node cannot PROVE domain
// membership and is rejected (缺窗身份≠可挂靠); a window-less TRUNK leaves the
// arm inert (absence never manufactures a rejection domain).
//
// Relevance arm (UXR-1 复核 P2-2 裁定, 2026-07-11): a typed OFF-chain row
// (chain_relevance adjacent/background) must never take a Kind="chain"
// trunk/attach seat — the PRIMARY/rank lane carries no relevance admission
// gate, so an adjacent-relevance PrimaryRootCauses row whose canonical
// subject collides with a trunk subject used to hijack the trunk capture and
// wear ❶ while its chip word said 邻近影响 (the CLOSE-1 same-page identity
// split). A rejected row stays unconsumed and renders on its OWN channel's
// stanza seat (the offChainStrays sweep). Empty relevance stays admitted
// (fail-open, matches the ordinal-channel authority).
func runtimeTraceProjTrunkDomainAdmit(node types.TraceCausalProjectionNode,
	electedBranch int, trunkWindowed bool, trunkWS, trunkWE float64) bool {
	switch strings.TrimSpace(node.ChainRelevance) {
	case "adjacent", "background":
		return false
	}
	if trunkWindowed {
		if node.QueryWindowStartTs <= 0 || node.QueryWindowEndTs <= node.QueryWindowStartTs {
			return false
		}
		if math.Abs(node.QueryWindowStartTs-trunkWS) > types.TraceCausalProjectionSameWindowToleranceS ||
			math.Abs(node.QueryWindowEndTs-trunkWE) > types.TraceCausalProjectionSameWindowToleranceS {
			return false
		}
	}
	if electedBranch > 0 {
		return node.ChainBranch == electedBranch
	}
	return node.ChainBranch == 0
}

// --- model construction ------------------------------------------------------

func buildRuntimeTraceProjTreeModel(projection types.TraceCausalProjection, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) runtimeTraceProjTreeModel {
	model := runtimeTraceProjTreeModel{
		WindowMS:                     projection.WindowDurationMS(),
		WindowStartTs:                projection.WindowStartTs,
		WindowEndTs:                  projection.WindowEndTs,
		WakeupChainRecommendedNotRun: projection.WakeupChainRecommendedNotRun,
		Marks:                        &runtimeTraceProjMarkSet{},
		// SPANVIS-1: the advisory mention side channel travels verbatim (the
		// compile already strict-parsed every row all-or-nothing); the display
		// composer applies its own precise gates per row.
		BusinessSpanMentions:       projection.BusinessSpanMentions,
		BusinessSpanMentionOmitted: projection.BusinessSpanMentionOmitted,
		// PARTSPLIT-1 (§29.150④): the R4-mirror refusal disclosure side
		// channel travels verbatim the same way (render re-validates).
		GatedCompositeEdgeShareDisclosures: projection.GatedCompositeEdgeShareDisclosures,
		// RULER2-1 (§29.150②): the self runnable two-ruler accounting side
		// channel travels verbatim the same way (render re-validates both
		// same-ruler Σ identities; the stamp pass resolves the lead row).
		SelfRunnableTwoRulerAccountings: projection.SelfRunnableTwoRulerAccountings,
	}
	path := runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)
	if len(path) >= 2 {
		model.Target = path[len(path)-1]
		// B1 (§12.3 裁定3): carry the compile-side anchor election to the 🎯
		// root label lane (runtimeTraceProjApplyUserFocus short-circuit).
		model.TargetUserElected = projection.WakeupPathUserElected
	}
	targetKey := runtimeTraceCausalProjectionCanonicalNode(model.Target)

	// On-chain node universe: primaries + on-chain bucket + hops, deduped by
	// node key (buckets deliberately overlap; see the aggregation layer).
	// Semantic spans are excluded here — their classified copies also live in
	// OnChainCauses, but they render exclusively through the ✦ 语义 lane (a span
	// consumed as a same-subject "cause" row would appear twice).
	//
	// EVOLUTION RECORD (SEM-LEAD §29.7-2 ①, ledger
	// real_trace_campaign_20260705.md, 2026-07-10): the exclusion arm no
	// longer means exclusion from COMPETITION — the ✦ 语义 lane row is the
	// entity's ONE seat, and an on-chain semantic row carrying the engine's
	// rank seat joins the shared rank board / lead election / ❶❷❸ badges
	// (runtimeTraceProjRankBoard semantic-kind arm; the rank-lane twin folds
	// into the ✦ row below instead of double-seating — E9/E13 双席合一).
	// Non-chain semantic rows keep the background comprehensive board +
	// mention gate untouched (§23.1 后半).
	chainUniverse := runtimeTraceProjDedupNodes(
		runtimeTraceProjExcludeSemanticSpans(
			append(append(append([]types.TraceCausalProjectionNode{},
				runtimeTraceCausalProjectionPrimaryRoots(projection)...),
				projection.OnChainCauses...),
				projection.SupportingHops...)))
	adjacentCauses := append([]types.TraceCausalProjectionNode(nil), projection.AdjacentCauses...)
	backgroundCauses := append([]types.TraceCausalProjectionNode(nil), projection.BackgroundCauses...)
	// SEM-LEAD/B9: build the ✦ 语义 lane node list first, then fold rank-lane
	// twins across ALL relevance buckets before any tree/stanza bucketing. A
	// precisely matched on-chain, adjacent, or background rank twin therefore
	// cannot retain a second depthless/◇/▒ seat; mismatches remain fail-open.
	semantics := append([]types.TraceCausalProjectionNode(nil), projection.SemanticSpans...)
	var semanticRankTwinPeers map[string][]types.TraceCausalProjectionNode
	chainUniverse, adjacentCauses, backgroundCauses, semantics, semanticRankTwinPeers =
		runtimeTraceProjFoldSemanticRankLaneTwinsAcrossBuckets(chainUniverse, adjacentCauses, backgroundCauses, semantics)
	// Causal main tree purity: only typed on-chain semantic work may occupy a
	// `├─语义─` seat. Adjacent/background semantic work keeps one stanza seat
	// (after B9 twin folding) and is still published by the independent
	// deterministic-optimization table.
	treeSemantics := make([]types.TraceCausalProjectionNode, 0, len(semantics))
	adjacentSemanticSeats := make(map[string]bool, len(adjacentCauses))
	for _, node := range adjacentCauses {
		adjacentSemanticSeats[runtimeTraceCausalProjectionNodeKey(node)] = true
	}
	backgroundSemanticSeats := make(map[string]bool, len(backgroundCauses))
	for _, node := range backgroundCauses {
		backgroundSemanticSeats[runtimeTraceCausalProjectionNodeKey(node)] = true
	}
	for _, span := range semantics {
		lane, ok := runtimeTraceProjSemanticTwinLane(span)
		switch {
		case ok && lane == "on_chain":
			treeSemantics = append(treeSemantics, span)
		case ok && lane == "adjacent":
			key := runtimeTraceCausalProjectionNodeKey(span)
			if !adjacentSemanticSeats[key] {
				adjacentCauses = append(adjacentCauses, span)
				adjacentSemanticSeats[key] = true
			}
		default:
			// A semantic span not proved on-chain must never mint a causal-tree
			// edge. Keep one lossless context seat even when the independent
			// context bucket was capped (or an older projection omitted the
			// relevance token); the node's typed relevance remains untouched.
			key := runtimeTraceCausalProjectionNodeKey(span)
			if !backgroundSemanticSeats[key] {
				backgroundCauses = append(backgroundCauses, span)
				backgroundSemanticSeats[key] = true
			}
		}
	}
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
	// 修复轮 P2-3 (冷读 donghu E8/E9, 2026-07-12): the refined-D proof and
	// the 等待对象 caller propagate across SAME-SEGMENT twin rows (the
	// existing runtimeTraceProjSameSegmentTwinKey — canonical subject + exact
	// engine line span) BEFORE any wordface renders: the window_stats fold
	// row carried 「D-state·等待对象」 while its wakeup_chain twin still spoke
	// 「D-state/iowait(对端未解析)」 beside it, breaking the 图例「已合并为
	// 一行」 promise family. The double-seat merge itself stays CR-2 P5 —
	// only the proof fields sync (one physical set of segments, one proof).
	runtimeTraceProjPropagateDStateProofToTwins(chainNodes)
	// NEW-3 (§7.6 回访): fold same-subject same-segment IO calibers into their
	// max-impact row BEFORE the subject buckets are built, so the peers never
	// mint sibling tree rows or same-subject cause rows. The fold map is
	// re-attached to the surviving primary's row after flatten (its row Kind —
	// self or tree — is only known then).
	chainNodes, ioFoldPeers := runtimeTraceProjFoldSameSubjectIONodes(chainNodes)
	// WO-N1 (SMR-1 批 SMR-S13, smr_audit_report §②, 2026-07-12): the NEW-3
	// same-segment IO fold reaches the ◇/▒ direct-mint lanes too — critical
	// rows minted straight into the adjacent/background stanzas escaped the
	// chainNodes-only fold (8411 witness: ▒ E23 io family beside ▒ E24 whose
	// wall clock sits inside it, additive read ≈4.23ms double-count). Per-lane
	// fold only (chain↔stanza pairs stay the D3 mutual-tag arm's business);
	// the wall-clock connectivity gate (see runtimeTraceProjIOOverlapComponents)
	// keeps the disjoint E25 shape独立行 by construction.
	if len(adjacentCauses) > 1 {
		var peers map[string][]types.TraceCausalProjectionNode
		adjacentCauses, peers = runtimeTraceProjFoldSameSubjectIONodes(adjacentCauses)
		for key, nodes := range peers {
			if ioFoldPeers == nil {
				ioFoldPeers = map[string][]types.TraceCausalProjectionNode{}
			}
			ioFoldPeers[key] = append(ioFoldPeers[key], nodes...)
		}
	}
	if len(backgroundCauses) > 1 {
		var peers map[string][]types.TraceCausalProjectionNode
		backgroundCauses, peers = runtimeTraceProjFoldSameSubjectIONodes(backgroundCauses)
		for key, nodes := range peers {
			if ioFoldPeers == nil {
				ioFoldPeers = map[string][]types.TraceCausalProjectionNode{}
			}
			ioFoldPeers[key] = append(ioFoldPeers[key], nodes...)
		}
	}
	// RNB R2 (§21/§22, 2026-07-07): fold the rank-lane twin of a same-segment
	// pair into its chain-lane node BEFORE the subject buckets, so the rank
	// twin never mints a sibling/cause tree row (成因形与兄弟形 both covered by
	// construction — the fold precedes tree-position assignment). The peers
	// are re-attached to the surviving row after flatten (evidence + note).
	chainNodes, rankFoldPeers := runtimeTraceProjFoldSameSegmentLaneTwins(chainNodes)
	// CR-2 组② P5 member arm (WO-D1①; the equality arm retired to the engine
	// one-seat mint in v5 P1 件①, 2026-07-13): a legacy raw root_evidence
	// member re-issue folds into its ×N seat BEFORE the subject buckets. The
	// peers re-attach to the surviving row after flatten.
	chainNodes, sameSegMirrorPeers := runtimeTraceProjFoldSameSegmentContextMirrors(chainNodes)
	// WO-D2/D4 (SMR-1 批 S2-TPF/SMR-S4, 2026-07-12): the trunk/flat
	// same-source ×N aggregate pair folds BEFORE the subject buckets too —
	// the flat 父节点未确认 copy of one physical aggregate never mints its
	// second seat (56643 E5/E19 witness). Same trunk-domain gate inputs the
	// depth attach consumes below.
	chainNodes, branchTwinPeers := runtimeTraceProjFoldBranchTwinAggregates(chainNodes,
		projection.WakeupPathBranch,
		projection.WakeupPathQueryWindowStartTs > 0 && projection.WakeupPathQueryWindowEndTs > projection.WakeupPathQueryWindowStartTs,
		projection.WakeupPathQueryWindowStartTs, projection.WakeupPathQueryWindowEndTs)
	// SEM-LEAD (§29.7-2 ③): the folded on-chain rank twin of a semantic row
	// rides the SAME RankFoldPeers carrier as the RNB fold — 行1 [E#+E#]
	// bracket, detail 根因排序 line, bar-scale/disclosure MAX invariance.
	if len(semanticRankTwinPeers) > 0 {
		if rankFoldPeers == nil {
			rankFoldPeers = map[string][]types.TraceCausalProjectionNode{}
		}
		for key, nodes := range semanticRankTwinPeers {
			rankFoldPeers[key] = append(rankFoldPeers[key], nodes...)
		}
	}
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
			// ANSWERFACE-1 件4 (§29.140 G8, 2026-07-19): the SELF-TWIN fold
			// (same physical sleep segment published through the
			// wakeup_causal_impact and root_cause_target_self_state views)
			// previously ran ONLY on the adjacent/background relocation lane
			// below — an ON-CHAIN symptom row walked this chain-universe pass
			// instead and minted a byte-identical second 自身·sleep row with
			// zero mutual reference (semantic_span E1/E2 witness). Same typed
			// matcher, same fail-open discipline: every join key must agree
			// (subject/state class/display+cumulative calibers/segment
			// start/selected window) or the row keeps its own seat.
			if node.IsTargetSelfStateRow() {
				if twin, ok := runtimeTraceProjSelfSymptomTwinIndex(model.SelfRows, node); ok {
					model.SelfRows[twin].SelfSymptomFoldPeers = append(
						model.SelfRows[twin].SelfSymptomFoldPeers,
						runtimeTraceProjSelfSymptomFoldPeer{
							EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
						},
					)
					continue
				}
			}
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
	if len(trunk) > runtimeTraceProjTreeTrunkMaxNodes {
		head := runtimeTraceProjTreeTrunkMaxNodes/2 + 1
		tail := runtimeTraceProjTreeTrunkMaxNodes - head
		omitStart, omitEnd = head, len(trunk)-tail
	}
	// §22 B1-b F2 (huadong_01 audit 2026-07-07), revised by PTV8-LAD (§24.11
	// 维度A F2 展开一次, 2026-07-08): typed user entities inside the folded
	// trunk middle must not vanish behind "…省略N节点" — a hit inside a
	// run-length CYCLE segment is named in full by the cycle row itself (the
	// disclosure duty rides that row); every other hit keeps its own
	// force-expanded row (projection.WakeupPathUserEntityHits, the compile-root
	// comparator's output; the fold layer never re-derives entity matches). No
	// hits and no repetition → exactly one plain segment [omitStart, omitEnd),
	// byte-stable with the pre-B1-b fold.
	//
	// PTV8-LAD L1 EVOLUTION RECORD (§24.11/§24.8): the former index-0-anchored
	// whole-path detector (runtimeTraceCausalProjectionRepeatingPath) is
	// RETIRED — it returned (0,0) on every mid-path cycle (the huadong_78
	// ladder carried ×0 disclosures), and its note could only ride the FIRST
	// fold row. The run-length lane detects mid-path cycles directly and each
	// cycle row carries its own ×N count.
	forcedTrunk := runtimeTraceProjUserEntityTrunkIndexes(projection, path)
	foldSegments, forcedExpanded := runtimeTraceProjFoldSegments(trunk, omitStart, omitEnd, forcedTrunk)
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

	// SEM-LEAD: `semantics` was built (and rank-twin folded) beside the chain
	// universe above — this bucketing consumes the post-fold rows.
	semanticBySubject := map[string][]types.TraceCausalProjectionNode{}
	for _, span := range treeSemantics {
		key := runtimeTraceCausalProjectionCanonicalNode(span.Subject)
		semanticBySubject[key] = append(semanticBySubject[key], span)
	}
	semanticConsumed := map[string]bool{}

	// P0-E CHAIN-PATH (ledger §22.1): the depth attach carries a CHAIN DOMAIN.
	// The elected trunk is ONE real branch (typed WakeupPathBranch); a node
	// measured in a DIFFERENT branch must never fabricate a trunk position off
	// its same-valued depth (the fake-L26/L27 family's attach half) — it keeps
	// its honest 父节点未确认/stanza seat below. rootDepth re-bases engine depths
	// onto the truncated-election trunk (displayed root = engine depth
	// rootDepth). Nodes WITHOUT branch identity keep the legacy depth attach
	// byte-identically (absence never guesses a domain). GAP-B G4 (§27.2,
	// 2026-07-09): the domain is (branch, WINDOW, depth) — see the
	// window-consistency arm inside the loop.
	electedBranch := projection.WakeupPathBranch
	rootDepth := projection.WakeupPathRootDepth
	// GAP-B G4 (§27.2, real_trace_campaign_20260705.md, 2026-07-09): the attach
	// domain carries a WINDOW dimension beside the P0-E branch dimension.
	// Branch ordinals are numbered per query window by the engine (each query
	// starts at 1), so (branch, depth) alone COLLIDES across windows — the
	// huadong_79 witness attached W2's hmfs_discard L2 node under the W1 touch
	// chain's L1 and the detail face fabricated a "关系: 唤醒 OS_mmi_EventHdr"
	// edge (the real path is hmfs→VSyncGenerator). trunkWS/trunkWE is the
	// elected trunk's OWN typed selected_window identity (P0-E 残洞第二形).
	trunkWS := projection.WakeupPathQueryWindowStartTs
	trunkWE := projection.WakeupPathQueryWindowEndTs
	trunkWindowed := trunkWS > 0 && trunkWE > trunkWS
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
			continue // trunk pass consumes below (same domain gate applied there — 复核 P2-1)
		}
		if !runtimeTraceProjTrunkDomainAdmit(node, electedBranch, trunkWindowed, trunkWS, trunkWE) {
			continue
		}
		rel := node.ChainDepth
		if electedBranch > 0 {
			rel = node.ChainDepth - rootDepth
		}
		if rel > 0 && rel <= len(trunk) {
			depthAttach[rel] = append(depthAttach[rel], node)
			consume(node)
		}
	}

	// Recursive trunk build (depth d node's child = depth d+1 node).
	var buildTrunk func(idx int, parentName string) []*runtimeTraceProjTreeNode
	buildTrunk = func(idx int, parentName string) []*runtimeTraceProjTreeNode {
		if idx >= len(trunk) {
			return nil
		}
		if seg, ok := foldSegments[idx]; ok {
			// PTV8-LAD L1 (§24.11 维度A): a CYCLE segment renders as one
			// "↺ 循环×N: A ⇄ B" row — tuple member names in full (整名不截),
			// children continue from the run end at ONE extra indent level
			// (the huadong_78 shape spent 14 levels on the same information).
			if seg.CycleLen > 0 {
				row := runtimeTraceProjTreeRow{
					Kind: runtimeTraceProjTreeRowCycleFold, Omitted: seg.End - idx,
					Depth: idx + 1 + rootDepth, CyclePeriod: seg.CycleLen, CycleCount: seg.CycleCount,
					CycleTuple: append([]string(nil), trunk[idx:idx+seg.CycleLen]...),
				}
				cycle := &runtimeTraceProjTreeNode{row: row}
				cycle.children = buildTrunk(seg.End, "…")
				return []*runtimeTraceProjTreeNode{cycle}
			}
			// PTV4 T8: the plain fold row names the folded segment's first/last
			// two nodes (the names were always in the typed path — display
			// upgrade only). ≤4 omitted nodes list fully via the head roster.
			// §22 B1-b F2: a force-expanded user-entity trunk index is NOT part
			// of any segment — the fold row covers [idx, segEnd) and the forced
			// node builds as a normal trunk row below.
			segEnd := seg.End
			var head, tail []string
			if segEnd-idx <= 4 {
				head = append(head, trunk[idx:segEnd]...)
			} else {
				head = append(head, trunk[idx:idx+2]...)
				tail = append(tail, trunk[segEnd-2:segEnd]...)
			}
			row := runtimeTraceProjTreeRow{
				Kind: runtimeTraceProjTreeRowOmitted, Omitted: segEnd - idx,
				Depth: idx + 1 + rootDepth, OmittedHead: head, OmittedTail: tail,
			}
			omitted := &runtimeTraceProjTreeNode{row: row}
			omitted.children = buildTrunk(segEnd, "…")
			return []*runtimeTraceProjTreeNode{omitted}
		}
		subject := trunk[idx]
		// P0-E CHAIN-PATH: rel keys the tree STRUCTURE (position on the
		// displayed trunk — indentation, drill-edge choice, depthAttach); the
		// row's published Depth is the engine's TRUE chain depth (rel +
		// rootDepth — nonzero rootDepth only on a truncated mid-chain
		// election), so the 链上L# chip and the detail 层级 face agree.
		rel := idx + 1
		depth := rel + rootDepth
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(subject)
		var main types.TraceCausalProjectionNode
		var extra []types.TraceCausalProjectionNode
		hasData := false
		for _, node := range bySubject[subjectKey] {
			if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
				continue
			}
			// 复核 P2-1 (2026-07-09): the same-name capture carries the SAME
			// chain-domain gate as the depth attach (one shared helper) — a
			// cross-window/cross-branch node whose canonical subject collides
			// with this trunk subject must never hijack the trunk main/extra
			// selection off its name alone. Rejected nodes stay unconsumed and
			// take their honest depthless/stanza seat below.
			if !runtimeTraceProjTrunkDomainAdmit(node, electedBranch, trunkWindowed, trunkWS, trunkWE) {
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
			// GAP-B G5 (§27.3, 2026-07-09): same-(thread, dominant-state)
			// re-occurrences fold into the main row as ONE ×N form instead of
			// rendering as the thread's own "├─成因─" children (自因自指形 —
			// see the fold helper's threshold-2 rationale). Different-state
			// extras keep the cause-decomposition edge below.
			main, extra = runtimeTraceProjTrunkFoldSameStateOccurrences(main, extra)
		} else {
			main = types.TraceCausalProjectionNode{Subject: subject, ChainDepth: depth}
		}
		edge := runtimeTraceProjTreeEdgeWake
		if rel == 1 {
			edge = runtimeTraceProjTreeEdgeDrill
		}
		trunkNode := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
			Node: main, Kind: runtimeTraceProjTreeRowChain, Edge: edge, Depth: depth,
			Parent: parentName, HasData: hasData, RecursOnChain: recurs[idx],
			// §22 B1-b F2 (PTV8-LAD F2 展开一次): flag rows force-expanded OUT
			// of the folded middle — the transit renderer swaps the anonymous
			// 中转 token for the ⊚中转 user-focus token on them (data rows
			// render normally). Hits merged into a cycle row are NOT flagged:
			// the cycle row already names them in full.
			UserFocusForced: forcedExpanded[idx],
			EvidenceTag:     runtimeTraceProjEvidenceTag(main, evidence, zh),
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
		for _, node := range depthAttach[rel+1] {
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
				Depth: 1 + rootDepth, Parent: model.Target, HasData: true,
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			}})
		}
	}
	// Remaining on-chain rows (no trunk membership, no resolvable depth) — a
	// typed-faithful "depth unresolved" branch instead of an invented position.
	var offChainStrays []types.TraceCausalProjectionNode
	for _, node := range chainNodes {
		if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
			continue
		}
		// [Med 修正轮 2026-07-06] a background/adjacent-relevance row in the
		// chain universe (the PRIMARY/rank lane carries no #1a admission gate)
		// must NOT take the depthless lane — the 链上·深度未解析 edge would
		// claim chain identity for a typed OFF-chain row while the #2 defense
		// suppressed its honest stanza seat. Leave it UNCONSUMED so its bucket
		// copy renders in ◇/▒ below; the stray pass after the stanza loops
		// seats any copy the bucket cap dropped (PTS 永不静默丢).
		if rel := strings.TrimSpace(node.ChainRelevance); rel == "background" || rel == "adjacent" {
			offChainStrays = append(offChainStrays, node)
			continue
		}
		consume(node)
		// PTV6 #1b (v3 §5, specimen donghu_short): the depthless lane's default
		// edge is the DEDICATED 链上·深度未解析 edge — never the wake edge. The
		// pre-PTV6 hardcoded runtimeTraceProjTreeEdgeWake here asserted a bare
		// 唤醒 relation the data never carried, hanging background-classified
		// rows off the 🎯 target as phantom wakers (#9 负向 pin: 禁裸唤醒边挂非
		// waker). The typed node predicate decides ONCE at build time; every
		// display surface (fence edge, relation cell, legend entry) reads the
		// resulting row.Edge.
		edge := runtimeTraceProjTreeEdgeChainUnresolved
		// F2: a process-level IO caliber row of the 🎯 target's OWN process is
		// not an upstream waker — it keeps its own dedicated edge.
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
	for _, span := range treeSemantics {
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

	// GAP-B G11 (§27.5, real_trace_campaign_20260705.md, 2026-07-09): the
	// target's OWN wait-symptom rank rows (typed tier target_self_state +
	// canonical subject == 🎯 target) relocate from the ◇/▒ stanza buckets
	// into the self-state area under the root. FILTER POINT this repairs: the
	// SelfRows lane fed exclusively off the CHAIN universe (primaries +
	// on-chain + hops — the bySubject[targetKey] pass above), so a self
	// binder_wait row classified background/adjacent never reached the 树根
	// stanza (huadong_79: the 3.527ms binder_wait self rows lived only in the
	// system-supplement stanza while a 0.011ms D-state was in the self area —
	// §24.8 重要信息永不省略抵触). Bounded: top-K by display magnitude
	// relocate (sorted, deterministic); the remainder KEEP their stanza seats
	// (lossless) and the self area discloses their count + single max.
	if targetKey != "" {
		var selfWait []types.TraceCausalProjectionNode
		for _, bucket := range [][]types.TraceCausalProjectionNode{adjacentCauses, backgroundCauses} {
			for _, node := range bucket {
				if consumed[runtimeTraceCausalProjectionNodeKey(node)] {
					continue
				}
				if !node.IsTargetSelfStateRow() ||
					runtimeTraceCausalProjectionCanonicalNode(node.Subject) != targetKey {
					continue
				}
				selfWait = append(selfWait, node)
			}
		}
		sort.SliceStable(selfWait, func(i, j int) bool {
			return runtimeTraceProjNodeDisplayImpact(selfWait[i]) > runtimeTraceProjNodeDisplayImpact(selfWait[j])
		})
		relocatedCount := 0
		for _, node := range selfWait {
			// SELF-TWIN (2026-07-10 customer witness): wakeup_causal_impact and
			// root_cause_target_self_state can publish the same focused-thread
			// sleep segment through two views.  When the typed subject/state,
			// selected window, display/cumulative calibers and segment start all
			// agree, keep the scheduler-state row as the single seat and attach
			// only the symptom peer's evidence.  Any ambiguity fails open.
			if twin, ok := runtimeTraceProjSelfSymptomTwinIndex(model.SelfRows, node); ok {
				consume(node)
				model.SelfRows[twin].SelfSymptomFoldPeers = append(
					model.SelfRows[twin].SelfSymptomFoldPeers,
					runtimeTraceProjSelfSymptomFoldPeer{
						EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
					},
				)
				continue
			}
			if relocatedCount >= runtimeTraceProjSelfWaitRelocateMax {
				model.SelfWaitOverflowCount++
				// 复核 P3-2 (2026-07-09): the disclosure word is 单条最大 — an
				// ×N merged row's display impact is the member SUM, so the
				// single-instance magnitude is its MergedMaxMS (the symptom
				// census reads the same lane).
				v := runtimeTraceProjNodeDisplayImpact(node)
				if node.MergedCount > 1 && node.MergedMaxMS > 0 {
					v = node.MergedMaxMS
				}
				if v > model.SelfWaitOverflowMaxMS {
					model.SelfWaitOverflowMaxMS = v
				}
				continue
			}
			consume(node)
			model.SelfRows = append(model.SelfRows, runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				SelfSymptomRelocated: true,
				EvidenceTag:          runtimeTraceProjEvidenceTag(node, evidence, zh),
			})
			relocatedCount++
		}
	}

	// PTV6 #2 (双席防御): a node key the chain universe already consumed
	// (self / trunk / depth-attach / depthless row) never seats a SECOND copy
	// in the ◇/▒ stanzas. Post-#1a the specimen's background rows no longer
	// enter the chain universe at all, so this is a fallback defense — any
	// future lane that double-casts one node into both surfaces renders it
	// once, on the chain (the stanza copy is the duplicate; the (a) table's
	// 双席 declaration follows the actual seats and drops with it).
	// WO-G1 (SMR-1 批 SMR-S12a, smr_audit_report §②, 2026-07-12): G2 双发布
	// 去重扩臂 — the existing G2 arm covers the「◇席 vs 溢出折叠」pair; this
	// adds the「树链止 vs ◇席」pair.
	//
	// Drop 口径 (P3 修复轮落档, 2026-07-13): the suppressed stanza copy is a
	// VALUELESS marker of the SAME single-minted fact (one expandChain
	// nil-interesting mint per thread) — the kept chain seat carries the full
	// typed criterion word, and the 91869 witness copies carry no E# of their
	// own, so nothing is silently lost; a copy WITH its own evidence id keeps
	// its index registration through the raw observation store (evidence is
	// never seat-gated). 零静默消失 holds: the fact stays disclosed on the
	// surviving seat. The expandChain nil-interesting arm mints
	// ONE trace_gap fact per thread, but the root_evidence chain copy AND the
	// rank-lane ◇ copy both seated (91869 double-◌ witness, two mutually
	// exclusive reason words on one page). The chain-stop seat wins (the 链止
	// position carries the fact); a stanza trace_gap copy of the SAME subject
	// and a compatible query window never seats a second ◌. Different query
	// windows keep both seats (two facts). Precise typed inputs only.
	chainGapSeated := map[string][]types.TraceCausalProjectionNode{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for _, r := range rows {
			if !r.HasData || !runtimeTraceProjTraceGapNode(r.Node) {
				continue
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(r.Node.Subject)
			if subject == "" {
				continue
			}
			chainGapSeated[subject] = append(chainGapSeated[subject], r.Node)
		}
	}
	gapSeatDuplicate := func(node types.TraceCausalProjectionNode) bool {
		if !runtimeTraceProjTraceGapNode(node) {
			return false
		}
		for _, seated := range chainGapSeated[runtimeTraceCausalProjectionCanonicalNode(node.Subject)] {
			sw, se := seated.QueryWindowStartTs, seated.QueryWindowEndTs
			nw, ne := node.QueryWindowStartTs, node.QueryWindowEndTs
			if sw > 0 && se > sw && nw > 0 && ne > nw &&
				(math.Abs(sw-nw) > types.TraceCausalProjectionSameWindowToleranceS ||
					math.Abs(se-ne) > types.TraceCausalProjectionSameWindowToleranceS) {
				continue // distinct query windows = two facts, both honest seats
			}
			return true
		}
		return false
	}
	adjacentSeen := map[string]bool{}
	for _, node := range runtimeTraceProjAdjacentNodesForDisplay(adjacentCauses) {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if consumed[key] {
			continue
		}
		if gapSeatDuplicate(node) {
			continue // WO-G1: 树链止 seat already speaks this thread's gap fact
		}
		adjacentSeen[key] = true
		model.Adjacent = append(model.Adjacent, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true,
			EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
		})
	}
	backgroundSeen := map[string]bool{}
	for _, node := range backgroundCauses {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if consumed[key] {
			continue // PTV6 #2: chain seat wins — no 链/▒ double seat
		}
		if gapSeatDuplicate(node) {
			continue // WO-G1: 树链止 seat already speaks this thread's gap fact
		}
		backgroundSeen[key] = true
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
	// [Med 修正轮 2026-07-06] stray pass: an off-chain-relevance chain-universe
	// row whose stanza-bucket copy never rendered (bucket cap dropped it, or
	// the buckets never carried it) still gets its honest ◇/▒ seat here —
	// routed by its OWN typed relevance, deduped against every seat already
	// taken (PTS 永不静默丢; the demote lane's normalization applies).
	for _, node := range offChainStrays {
		key := runtimeTraceCausalProjectionNodeKey(node)
		if consumed[key] || adjacentSeen[key] || backgroundSeen[key] {
			continue
		}
		if gapSeatDuplicate(node) {
			continue // WO-G1: 树链止 seat already speaks this thread's gap fact
		}
		tag := runtimeTraceProjEvidenceTag(node, evidence, zh)
		if node.Role == types.TraceCausalRolePrimaryRootCause || node.Role == types.TraceCausalRoleCausalHop {
			node.Role = types.TraceCausalRoleRootCauseContext
		}
		if strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_primary") {
			node.Predicate = "root_cause_context"
		}
		if strings.TrimSpace(node.ChainRelevance) == "adjacent" {
			adjacentSeen[key] = true
			model.Adjacent = append(model.Adjacent, runtimeTraceProjTreeRow{
				Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true,
				EvidenceTag: tag,
			})
			continue
		}
		backgroundSeen[key] = true
		model.Background = append(model.Background, runtimeTraceProjTreeRow{
			Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
			EvidenceTag: tag,
		})
	}
	// UXR-1 §29.36.2 (通道3 口径分组, 2026-07-11): the ▒ stanza is an UNORDERED
	// channel — no ordinals — so its display order is caliber-grouped instead:
	// cross-thread aggregate rows (typed subject_kind=aggregate_metric; cpu·ms
	// calibers) group first, wall-clock rows follow, fixed magnitude order
	// inside each group (两把尺不同组,组内量级可比). Unconditional stable
	// regroup — the former demote/stray-only magnitude sort is subsumed.
	sort.SliceStable(model.Background, func(i, j int) bool {
		ai := model.Background[i].Node.IsAggregateMetric()
		aj := model.Background[j].Node.IsAggregateMetric()
		if ai != aj {
			return ai
		}
		return runtimeTraceProjNodeDisplayImpact(model.Background[i].Node) >
			runtimeTraceProjNodeDisplayImpact(model.Background[j].Node)
	})

	// WO-N1 (SMR-1 批, 2026-07-12): the stanza-lane NEW-3 folds attach only
	// AFTER the ◇/▒ populations exist (the 收尾 P1 dead-code lesson — the
	// post-flatten attach above ran on empty stanza slices). The caliber note
	// is the folded peers' only display carrier on these faces too.
	if len(ioFoldPeers) > 0 {
		attachIOFold := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				if len(rows[i].IOFoldPeers) > 0 {
					continue // chain/self faces attached above
				}
				for _, peer := range ioFoldPeers[runtimeTraceCausalProjectionNodeKey(rows[i].Node)] {
					rows[i].IOFoldPeers = append(rows[i].IOFoldPeers, runtimeTraceProjIOFoldPeer{
						Token:       strings.TrimSpace(peer.TypeToken),
						ImpactMS:    runtimeTraceProjNodeDisplayImpact(peer),
						EvidenceTag: runtimeTraceProjEvidenceTag(peer, evidence, zh),
					})
				}
			}
		}
		attachIOFold(model.Adjacent)
		attachIOFold(model.Background)
	}

	// RNB R2 / B9: attach every folded rank-lane peer only after ALL four
	// display populations have been materialized. On-chain pairs attach to the
	// surviving tree/self row; adjacent/background pairs attach to their sole
	// stanza row. The peer remains an evidence/detail carrier. Bar/coverage
	// invariance still reads the peer's magnitudes exactly as before.
	//
	// UXR-1 (§29.36.2, 2026-07-11): an ADJACENT peer's Rank is the 邻近影响
	// channel's own ordinal — it transfers (the chip printer words it per
	// channel, so no on-chain claim is possible); only a BACKGROUND peer's
	// stale ordinal zeroes (通道3 无序数).
	if len(rankFoldPeers) > 0 {
		attach := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				for _, peer := range rankFoldPeers[runtimeTraceCausalProjectionNodeKey(rows[i].Node)] {
					peerRank := peer.Rank
					if lane, ok := runtimeTraceProjSemanticTwinLane(peer); ok && lane == "background" {
						peerRank = 0
					}
					// ELIM-V2 (2026-07-18): the folded rank twin's engine-
					// stamped fix direction follows its seat onto the surviving
					// row (verbatim engine value, empty-slot fill only — the ◎
					// section key must not strand the RNB-fold carriage in the
					// 方向未定 tail while its twin published a direction).
					if strings.TrimSpace(rows[i].Node.FixDirection) == "" {
						rows[i].Node.FixDirection = peer.FixDirection
					}
					rows[i].RankFoldPeers = append(rows[i].RankFoldPeers, runtimeTraceProjRankFoldPeer{
						TypeWord:           strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(peer, zh)),
						Rank:               peerRank,
						Confidence:         peer.Confidence,
						EvidenceTag:        runtimeTraceProjEvidenceTag(peer, evidence, zh),
						CumulativeImpactMS: peer.CumulativeImpactMS,
						DisplayImpactMS:    runtimeTraceProjNodeDisplayImpact(peer),
						TargetImpactMS:     peer.TargetImpactMS,
					})
				}
			}
		}
		attach(model.SelfRows)
		attach(model.TreeRows)
		attach(model.Adjacent)
		attach(model.Background)
	}

	// CR-2 组② P5 member arm (legacy lane): attach the folded raw-state mirror copies to
	// the surviving row (bracket E#, 行2 状态 slot + 同段镜像 tag). Same
	// post-flatten position as the rank-fold peers above; the evidence tag
	// registration keeps the mirror observation reachable on the index.
	if len(sameSegMirrorPeers) > 0 {
		attach := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				for _, peer := range sameSegMirrorPeers[runtimeTraceCausalProjectionNodeKey(rows[i].Node)] {
					stateWord := ""
					if rows[i].Node.StateKind == "" {
						stateWord = strings.TrimSpace(peer.Predicate)
					}
					rows[i].SameSegMirrorPeers = append(rows[i].SameSegMirrorPeers, runtimeTraceProjSameSegMirrorPeer{
						EvidenceTag: runtimeTraceProjEvidenceTag(peer, evidence, zh),
						StateWord:   stateWord,
						Valueless:   runtimeTraceProjNodeDisplayImpact(peer) <= 0,
					})
				}
			}
		}
		attach(model.SelfRows)
		attach(model.TreeRows)
		attach(model.Adjacent)
		attach(model.Background)
	}

	// WO-D2/D4: attach the folded flat aggregate copies (E# into the bracket,
	// eff caliber dual-listed on 行2) — same post-flatten position as the
	// mirror peers above.
	if len(branchTwinPeers) > 0 {
		attach := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				for _, peer := range branchTwinPeers[runtimeTraceCausalProjectionNodeKey(rows[i].Node)] {
					rows[i].BranchTwinFoldPeers = append(rows[i].BranchTwinFoldPeers, runtimeTraceProjBranchTwinFoldPeer{
						EvidenceTag:       runtimeTraceProjEvidenceTag(peer, evidence, zh),
						EffectiveImpactMS: peer.EffectiveImpactMS,
					})
				}
			}
		}
		attach(model.SelfRows)
		attach(model.TreeRows)
		attach(model.Adjacent)
		attach(model.Background)
	}

	// CR-2 组② P5 family arm (F-1 残口, donghu E8/E9): mark a merged
	// critical_blocking twin whose fingerprint (canonical subject + member
	// count + µs-equal total) matches a family row carrying CAL-1 segment
	// truth — the twin's 行2 wears the family-mirror tag and its detail range
	// speaks the group-sum caliber with the family's true single-segment
	// extrema (段 inventory 传播到该 lane).
	runtimeTraceProjMarkFamilyMirrorTwins(&model)

	// 修复轮 C-2/A1: the µs-equal cross-lane value mirror (aggregate row ↔
	// ×N candidate row) — typed fingerprint, tag-only convergence.
	runtimeTraceProjMarkValueMirrorTwins(&model)

	// SELF-LANE (§29.58.3 处置 a, SELF-ALL 批 件②, 2026-07-13): after the
	// SELF-ALL engine promotion the ◇ residual target rows are the honest
	// non-chain leftovers (no on-chain proof / non-wall-clock caliber) — they
	// relocate into the self stanza wearing the 「非链」 qualifier: the 邻近
	// word promises OTHER threads competing nearby, and the subject is never
	// its own neighbour (062104 witness). Runs BEFORE every relation pass so
	// the SMR-1 family and the cross-channel pointers see the final seating.
	runtimeTraceProjRelocateSelfNonChainSeats(&model, targetKey)

	// SMR-1 批 relation passes (2026-07-12) — order matters (四臂判定互斥,
	// 矩阵判型即路由): D3 double-merged twins first (equality shapes), then
	// the A1 non-additive pointers (containment/membership), then the C1
	// account sentences (different-account leftovers), then the B1 disjoint
	// occurrence notes.
	runtimeTraceProjMarkMergedTwinMirrors(&model)
	// XLANE-2 件1 (§29.104.1/.2 定谳④, 2026-07-17): the semantic member-subset
	// demotion — the most-typed relation of the family (engine line-range set
	// inclusion) stamps before the generic pointer arms; later passes skip
	// related rows through the shared RelationFree gate.
	runtimeTraceProjMarkSemanticMemberSubsets(&model)
	// XLANE-2 件2 (裁定④): resolve the self-gap seat's typed overlap roster
	// into [E#] clauses (verbatim line-envelope identity).
	runtimeTraceProjStampSelfGapSemanticOverlaps(&model)
	// AXIOM-V2 件2 (公理 v2, 2026-07-18): resolve the cross-direction overlap
	// pair rosters into mutual [E#] clauses (verbatim line-envelope identity,
	// 同板 gate, both-or-neither reciprocity prune).
	runtimeTraceProjStampCrossDirectionOverlaps(&model)
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): resolve the gated-share split
	// rows' claim-seat line intervals into inversion-seat [E#] refs
	// (all-or-nothing; the 行2 sentences keep a generic noun otherwise).
	runtimeTraceProjStampGatedShareSplit(&model)
	// RULER2-1 (§29.150②): resolve each two-ruler accounting record's LEAD
	// seat row (unique typed host match or nothing — 缺载体静默).
	runtimeTraceProjStampSelfRunnableTwoRuler(&model)
	// LEVELMERGE-1 件3 (两向互指, 2026-07-18): the aggregate-seat ↔ member
	// occurrence pointer pair (ORD-A membership predicate on typed node
	// fields; ≥2 members, exactly one seat, same board, all-or-nothing).
	runtimeTraceProjStampAggregateMemberCrossRefs(&model)
	runtimeTraceProjMarkNonAdditivePointers(&model)
	// P2a rider 件2b (§29.58.1 b, 2026-07-13): reseat self COMPONENT rows
	// (binder ⊂ sleep carve) directly under their owning seat with the ↳
	// connector — must run right after the WO-A1 pass that mints the typed
	// pointers this pass consumes.
	//
	// 件2c ∿ 层级核验注记 (§29.58.1 c, typed 关系核验 2026-07-13, 引擎铸造点
	// 直读): cadence-idle (pacing_idle/periodic_idle) rows are minted from a
	// physically-sleep segment, BUT their account is CARVED OUT of the sleep
	// seat's account — the engine publishes the idle row under the segment's
	// causal-impact evidence span "so the display same-fact fold engages by
	// construction" (tracequery findBinderWaitsForChain ENG-2 追修: the
	// segment's own sleep record folds INTO the idle row), and the display R2
	// family fold excludes idle rows from the sleep ×N family (CAL-1 件⑤).
	// The two accounts are therefore DISJOINT segment sets, not a subset
	// (witness 20260713-062104: sleep ×7 members ≤14.302ms vs idle segment
	// 15.758ms — the idle segment is NOT among the sleep members). Ruling c)
	// verdict: 独立上下文 — the ∿ row keeps its sibling level and never takes
	// the ↳ treatment; the carrier-(a) census exclusion in
	// runtimeTraceProjMarkNonAdditivePointers stays the typed mirror of this
	// fact.
	runtimeTraceProjSeatSelfComponentRows(&model)
	runtimeTraceProjMarkAccountRelations(&model, zh)
	// XERR1-FIX 件1 互指 (§29.104.4, E6/E7 账目关系先例): the converged
	// blocking row ↔ the thread's own sleep seat mutual pointers.
	runtimeTraceProjMarkBlockingWaitSleepRelations(&model)
	// SELF-LANE (§29.58.3 处置 b, 2026-07-13): the cross-channel same-thread
	// mutual pointers join the relation-sentence family — after the account
	// relations so the four-arm routing above stays untouched.
	runtimeTraceProjMarkCrossChannelSameThread(&model)
	// XLANE-3 件3 (§29.104.2 定谳③, 2026-07-16): the cross-board same-thread
	// same-state-family mutual pointers — after every other relation arm so
	// the one-relation-one-sentence peer exclusion reads their final refs.
	runtimeTraceProjStampCrossBoardFamilyNotes(&model, zh)
	runtimeTraceProjStampOccurrenceSeries(&model)
	runtimeTraceProjMarkSeriesAggregateSeats(&model)
	runtimeTraceProjResolveOverflowMirrorRefs(&model)
	runtimeTraceProjStampOverflowSeriesMirrors(&model)
	runtimeTraceProjResolveOverflowProjectionRefs(&model)

	// CR-2 组③ P7: stamp every row's typed actual-scope verdict against the
	// analysis window (one authority, every face reads the stamp).
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			rows[i].ActualScope = runtimeTraceProjActualWindowScope(rows[i].Node, model.WindowStartTs, model.WindowEndTs)
		}
	}

	// G1 跨车道对账 (§27.2-G1, 2026-07-09): register every engine-absorbed
	// chain-lane observation on the evidence index (E# preserved even if the
	// family row itself fell off a render cap — 永不静默丢) and attach the
	// peer set to the FIRST rendered family row carrying the matching
	// verbatim RankFamilyKey; the detail stanza prints the 链上并入
	// disclosure from it. The compile already relocated these nodes out of
	// every bucket, so no render face seats them as rows.
	//
	// 收尾 P1 (对抗复核 REPRO, 2026-07-09): this pass MUST run after EVERY
	// row lane is populated — TreeRows/SelfRows (flatten + G11 relocation)
	// AND the ◇/▒ stanza loops above. Its original seat right after the
	// rankFoldPeers attach scanned model.Adjacent/model.Background while they
	// were still empty slices, so an off-chain family row (background io
	// family — a real production shape) could never receive its 链上并入
	// note: dead code on exactly the stanza lanes it claimed to cover.
	if len(projection.AbsorbedChainRows) > 0 {
		absorbedByFamily := map[string][]runtimeTraceProjAbsorbedChainPeer{}
		for _, node := range projection.AbsorbedChainRows {
			key := strings.TrimSpace(node.AbsorbedInto)
			if key == "" {
				continue
			}
			absorbedByFamily[key] = append(absorbedByFamily[key], runtimeTraceProjAbsorbedChainPeer{
				EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
			})
		}
		claimed := map[string]bool{}
		attach := func(rows []runtimeTraceProjTreeRow) {
			for i := range rows {
				key := strings.TrimSpace(rows[i].Node.RankFamilyKey)
				if key == "" || claimed[key] {
					continue
				}
				if peers := absorbedByFamily[key]; len(peers) > 0 {
					rows[i].AbsorbedChainPeers = peers
					claimed[key] = true
				}
			}
		}
		attach(model.SelfRows)
		attach(model.TreeRows)
		attach(model.Adjacent)
		attach(model.Background)
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
	// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): fold the chain-lane micro
	// anchored cut seats (<0.1ms) into one counted ⛓ row — after every
	// relation pass (members' ◇ twins keep their minted sentences; the fold
	// bracket keeps their E#s on-page) and before bar scaling / badges.
	runtimeTraceProjFoldMicroAnchorSeats(&model)
	// RNB-5B 默认小件c (§29.95 UX-4 对称): the 自身·墙钟席 qualifier covers
	// the whole self wall-clock cause-seat family (runs after every SelfRows
	// population pass).
	runtimeTraceProjStampSelfWallClockQualifiers(&model)
	// XLANE-1 件3 词面 rider (§29.104.2 定谳⑤): foreign-subject rows never
	// wear the target-exclusive 自身· qualifier words (runs after every row
	// population/stamp pass so it sees the final row sets).
	runtimeTraceProjStampSelfQualifierSubjectGate(&model)
	model.BarMaxMS, model.BarScaleWallClockAnchored = runtimeTraceProjModelMaxImpact(model)
	// RN-3(b): pin the conclusion-consumed node's key on the model so the
	// detail table's 因果位置·优先级 column follows the SAME selection
	// (runtimeTraceProjLeadSelect is deterministic on (projection, model), so
	// the conclusion line re-running it later cannot disagree).
	// §21 LEAD-SEM: the semantic tier-4 lead is an optimization-span
	// statement, NOT a consumed 主根因 — it never claims LeadKey (the flat 🎯
	// anchor lane and the 因果位置 demotion gate keep their legacy behavior).
	if lead, lane := runtimeTraceProjLeadSelect(projection, model); lead != nil &&
		lane != runtimeTraceProjLeadLaneSemanticFallback {
		model.LeadKey = runtimeTraceCausalProjectionNodeKey(*lead)
	}
	runtimeTraceProjAssignTopBadges(&model)
	// PTV8-RCR-B (UXA 域A #25): mark rows whose drilldown target is itself a
	// rendered tree row (exact trimmed-subject match over the rendered rows —
	// transit rows included, they are visible tree positions; the ⊚ root
	// header counts too).
	rendered := map[string]bool{}
	if target := strings.TrimSpace(model.Target); target != "" {
		rendered[target] = true
	}
	for _, r := range model.TreeRows {
		if subject := strings.TrimSpace(r.Node.Subject); subject != "" {
			rendered[subject] = true
		}
	}
	for i := range model.TreeRows {
		if target := strings.TrimSpace(model.TreeRows[i].Node.DrilldownTarget); target != "" && rendered[target] {
			model.TreeRows[i].DrillTargetRendered = true
		}
	}
	// UXR-1 复核 P2-3: stale-artifact channel ordinals fail-close BEFORE the
	// window-chip stamp (a stale seat neither wears a window chip nor flips
	// the report into multi-board mode).
	runtimeTraceProjStampStaleChannelOrdinals(&model)
	runtimeTraceProjStampRankWindowChips(&model, zh)
	// B6 (2026-07-10): when an overflow roster repeats a thread that already
	// owns a visible rank seat, turn the apparent duplicate into a navigation
	// pointer ("见榜位#N" / "see root-cause rank #N"). Exact canonical subject + unique
	// rendered ordinal only; ambiguous same-subject ordinals stay unchanged.
	runtimeTraceProjAnnotateFoldRosterRankPointers(&model, zh)
	// UXR-1 §29.36.3 (通道4 提及义务显式化): stamp the mention-obligation seat
	// on every on-chain semantic row without a channel-1 ordinal.
	runtimeTraceProjStampSemanticMentionFloor(&model)
	// CR-3 修复轮追加件 (2026-07-12): rank same-thread coverage fragments so
	// only the true max wears the 最大片段 word.
	runtimeTraceProjStampCoverageFragmentRank(&model)
	// 冷读扩臂④ (SMR-1 修复轮, 2026-07-13): the board-level over-window sum.
	// XLANE-3 件3 (§29.104.2 定谳③, 2026-07-16): the Σ is PER BOARD — a
	// multi-step report's seats belong to several boards (typed triple
	// identity, runtimeTraceProjStableRankBoardIDs), and summing across them
	// is exactly the cross-board addition fallacy this face warns about
	// (donghu 形③: the fused Σ 355.562 mixed two boards over one 233.190
	// window while NEITHER board exceeded it). The sentence mints on the
	// largest single board's Σ only; a single-board report groups into one
	// board and stays byte-identical.
	if model.WindowMS > 0 {
		seats, boardIDs := runtimeTraceProjSigmaFaceSeats(&model)
		sums := map[string]float64{}
		for _, row := range seats {
			sums[boardIDs[row]] += row.Node.EffectiveImpactMS
		}
		for _, sum := range sums {
			if sum > model.WindowMS && sum > model.RankBoardEffSumMS {
				model.RankBoardEffSumMS = sum
			}
		}
		if model.RankBoardEffSumMS > 0 && len(sums) > 1 {
			model.RankBoardEffSumMultiBoard = true
		}
	}
	return model
}

// runtimeTraceProjSigmaFaceSeats is the Σ face's seat population (deduped
// valued rank seats over the four lanes) plus their shared-index board IDs —
// extracted so the coverage board scope (件A 修补轮, 2026-07-16) reads the
// SAME population and the SAME identity keys as the Σ face (板身份键与 Σ 面
// 同源).
func runtimeTraceProjSigmaFaceSeats(model *runtimeTraceProjTreeModel) ([]*runtimeTraceProjTreeRow, map[*runtimeTraceProjTreeRow]string) {
	var seats []*runtimeTraceProjTreeRow
	seen := map[string]bool{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			node := rows[i].Node
			if !rows[i].HasData || node.Rank <= 0 || node.EffectiveImpactMS <= 0 {
				continue
			}
			key := runtimeTraceCausalProjectionNodeKey(node)
			if seen[key] {
				continue
			}
			seen[key] = true
			seats = append(seats, &rows[i])
		}
	}
	return seats, runtimeTraceProjStableRankBoardIDs(seats)
}

// runtimeTraceProjCoverageBoardScope is the 件A (修补轮, 2026-07-16) coverage
// board scope: which rank board does the coverage face's SUBJECT (the 🎯
// target's own account) sit on, and does the report span several boards at
// all. Precise typed inputs only:
//
//   - multi: the Σ-face seat population spans ≥2 distinct board IDs (shared
//     index — same key space as the Σ face and the chip census);
//   - subject board: the A2 symptom-denominator board when one exists (the
//     target's own SelfRows account names its board), else the UNIQUE seat
//     board whose typed target label canonically equals the tree target.
//     Ambiguity (params forks of the target, no named target board) leaves
//     the subject board unresolved — consumers then fail toward silence,
//     never toward a cross-board claim.
//
// The subject board is carried by COMPONENTS (verbatim target + fingerprint
// halves, window half under the shared cluster tolerance) so membership
// tests never compare IDs minted from two different populations. Single-board
// reports (multi=false) keep every legacy consumer byte-identical: the scope
// only ever bites on multi-board fusions.
type runtimeTraceProjCoverageBoardScope struct {
	multi     bool
	subjectOK bool
	target    string
	fp        string
	winStart  float64
	winEnd    float64
	winOK     bool
}

// rowOnSubjectBoard reports whether a node PROVABLY sits on the subject
// board: verbatim target + fingerprint halves equal, window halves equal
// under the shared cluster tolerance. An identity-less node can never prove
// board membership on a multi-board report (absence fails toward exclusion
// exactly where a cross-board claim would otherwise mint).
func (scope runtimeTraceProjCoverageBoardScope) rowOnSubjectBoard(node types.TraceCausalProjectionNode) bool {
	if !scope.subjectOK {
		return false
	}
	target := strings.TrimSpace(node.RankBoardTarget)
	if target == "" || target != scope.target {
		return false
	}
	if strings.TrimSpace(node.RankBoardParamsFingerprint) != scope.fp {
		return false
	}
	start, end, ok := runtimeTraceProjRankChipWindow(node)
	if ok != scope.winOK {
		return false
	}
	if ok && (math.Abs(start-scope.winStart) > types.TraceCausalProjectionSameWindowToleranceS ||
		math.Abs(end-scope.winEnd) > types.TraceCausalProjectionSameWindowToleranceS) {
		return false
	}
	return true
}

// adoptBoardOf stamps the subject board components from a carrier node.
func (scope *runtimeTraceProjCoverageBoardScope) adoptBoardOf(node types.TraceCausalProjectionNode) {
	scope.target = strings.TrimSpace(node.RankBoardTarget)
	scope.fp = strings.TrimSpace(node.RankBoardParamsFingerprint)
	scope.winStart, scope.winEnd, scope.winOK = runtimeTraceProjRankChipWindow(node)
	scope.subjectOK = scope.target != ""
}

func runtimeTraceProjCoverageBoardScopeFor(model runtimeTraceProjTreeModel) runtimeTraceProjCoverageBoardScope {
	seats, boardIDs := runtimeTraceProjSigmaFaceSeats(&model)
	distinct := map[string]bool{}
	for _, row := range seats {
		distinct[boardIDs[row]] = true
	}
	scope := runtimeTraceProjCoverageBoardScope{multi: len(distinct) >= 2}
	if !scope.multi {
		return scope
	}
	// The A2 denominator board first: the coverage sentence's subject IS the
	// target's own admitted account.
	if carrier, ok := runtimeTraceProjSymptomDenominatorBoard(model); ok {
		scope.adoptBoardOf(carrier)
		return scope
	}
	// Else the unique seat board anchored on the tree target.
	targetKey := runtimeTraceCausalProjectionCanonicalNode(model.Target)
	if targetKey == "" {
		return scope
	}
	anchored := map[string]bool{}
	var carrier *runtimeTraceProjTreeRow
	for _, row := range seats {
		label := strings.TrimSpace(row.Node.RankBoardTarget)
		if label == "" || runtimeTraceCausalProjectionCanonicalNode(label) != targetKey {
			continue
		}
		anchored[boardIDs[row]] = true
		carrier = row
	}
	if len(anchored) == 1 && carrier != nil {
		scope.adoptBoardOf(carrier.Node)
	}
	return scope
}

// runtimeTraceProjStampCoverageFragmentRank (CR-3 修复轮追加件, 2026-07-12;
// 56643 witness: NetworkService-60595 的两条链上 runnable 行 7.843/6.754 —
// 两次独立发生,合法分行 — 从属披露都铸「链上仅覆盖其中最大片段 X」,第二行
// 宣称为假且同页两行互斥): among rows sharing (subject, state class,
// full-window total), only the TRUE max covered fragment keeps 最大片段;
// every other row is stamped secondary and speaks 另一片段. Ties keep the
// word on every tied max (both ARE largest). Display wording only.
func runtimeTraceProjStampCoverageFragmentRank(model *runtimeTraceProjTreeModel) {
	if model == nil {
		return
	}
	key := func(node types.TraceCausalProjectionNode) (string, float64, bool) {
		full := node.FullWindowStateMS
		class := types.TraceCausalProjectionStateClass(node.StateKind)
		covered := runtimeTraceProjNodeDisplayImpact(node)
		if full <= 0 || strings.TrimSpace(node.FullWindowStateSource) == "" || class == "" || covered <= 0 {
			return "", 0, false
		}
		return strings.TrimSpace(node.Subject) + "|" + string(class) + "|" + strconv.FormatFloat(full, 'f', 3, 64), covered, true
	}
	groupMax := map[string]float64{}
	for i := range model.TreeRows {
		if k, covered, ok := key(model.TreeRows[i].Node); ok && covered > groupMax[k] {
			groupMax[k] = covered
		}
	}
	for i := range model.TreeRows {
		if k, covered, ok := key(model.TreeRows[i].Node); ok && covered < groupMax[k] {
			model.TreeRows[i].CoverageFragmentSecondary = true
		}
	}
	// WO-A1 词面统一 复放追修 (96717 E12/E15 形, 2026-07-12): an UNMERGED rank
	// row whose covered value is typed-provably an engine ×N total must not
	// claim 最大片段 either. Two precise linkages:
	//   (a) its same-span W-A twin is a MergedCount>1 row with the µs-equal
	//       display (the rank lane republished the merged occurrence sum);
	//   (b) a same-(subject, state class) occurrence series' additive total
	//       µs-equals it (the rank seat IS the 合计参赛 seat of the series).
	mergedTwinBySpan := map[string]int{}
	for i := range model.TreeRows {
		node := model.TreeRows[i].Node
		if node.MergedCount <= 1 {
			continue
		}
		if k := runtimeTraceProjSameSegmentTwinKey(node); k != "" {
			mergedTwinBySpan[k+"\x00"+fmt.Sprintf("%.3f", runtimeTraceProjNodeDisplayImpact(node))] = node.MergedCount
		}
	}
	type seriesRef struct {
		subject string
		class   string
		total   float64
		count   int
	}
	var series []seriesRef
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for i := range rows {
			if rows[i].OccurrenceSeriesCount < 2 || rows[i].OccurrenceSeriesTotalMS <= 0 {
				continue
			}
			series = append(series, seriesRef{
				subject: runtimeTraceCausalProjectionCanonicalNode(rows[i].Node.Subject),
				class:   types.TraceCausalProjectionStateClass(rows[i].Node.StateKind),
				total:   rows[i].OccurrenceSeriesTotalMS,
				count:   rows[i].OccurrenceSeriesCount,
			})
		}
	}
	for i := range model.TreeRows {
		node := model.TreeRows[i].Node
		if node.MergedCount > 1 || node.FullWindowStateMS <= 0 {
			continue
		}
		display := runtimeTraceProjNodeDisplayImpact(node)
		if display <= 0 {
			continue
		}
		if k := runtimeTraceProjSameSegmentTwinKey(node); k != "" {
			if n := mergedTwinBySpan[k+"\x00"+fmt.Sprintf("%.3f", display)]; n > 1 {
				model.TreeRows[i].CoverageMergedTwinCount = n
				continue
			}
		}
		subject := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		class := types.TraceCausalProjectionStateClass(node.StateKind)
		for _, sr := range series {
			if sr.subject == subject && sr.class == class &&
				math.Abs(sr.total-display) < types.TraceCausalProjectionSameValueTieMS {
				model.TreeRows[i].CoverageMergedTwinCount = sr.count
				break
			}
		}
	}
}

// runtimeTraceProjStampSemanticMentionFloor (UXR-1 §29.36.3, user ruling
// 2026-07-11) marks the channel-4 mention-obligation rows: an ON-CHAIN
// semantic ✦ row without a displayed 根因排序 seat is the SEM-LEAD mention
// floor's explicit channel member — it renders unconditionally (凡 on_chain
// 语义行必渲染,无静默消失路径: entering TOP N promotes it to channel 1
// instead), and its 行2 names the obligation with the rendered chain board
// size. Typed inputs only (chain relevance token + the same displayed-seat
// resolver 行2 prints); display wording input, never a gate/sort lane.
func runtimeTraceProjStampSemanticMentionFloor(model *runtimeTraceProjTreeModel) {
	topN := 0
	groups := [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background}
	for _, rows := range groups {
		for i := range rows {
			if !rows[i].HasData {
				continue
			}
			if runtimeTraceProjRowOrdinalChannel(rows[i]) != runtimeTraceProjOrdinalChannelChain {
				continue
			}
			if rank, _ := runtimeTraceProjCauseRankConfidence(rows[i]); rank > topN {
				topN = rank
			}
		}
	}
	for _, rows := range groups {
		for i := range rows {
			row := &rows[i]
			if !row.HasData || row.Kind != runtimeTraceProjTreeRowSemantic {
				continue
			}
			if strings.TrimSpace(row.Node.ChainRelevance) != "on_chain" {
				continue
			}
			if rank, _ := runtimeTraceProjCauseRankConfidence(*row); rank > 0 {
				continue // channel 1 seat — 可冕可戴, not the obligation lane
			}
			row.MentionFloorOnChain = true
			row.MentionFloorTopN = topN
		}
	}
}

// runtimeTraceProjSelfSymptomTwinIndex proves that a relocated
// root_cause_target_self_state sleep row is the rank-lane view of exactly one
// already-rendered wakeup_causal_impact scheduler-state row. This is display
// reconciliation only: it never changes either node or any numeric projection
// bucket. Every join arm is typed and exact (apart from the repository-wide
// selected-window endpoint tolerance); zero/ambiguous inputs fail open.
func runtimeTraceProjSelfSymptomTwinIndex(rows []runtimeTraceProjTreeRow, symptom types.TraceCausalProjectionNode) (int, bool) {
	if !symptom.IsTargetSelfStateRow() ||
		strings.TrimSpace(symptom.Predicate) != "root_cause_target_self_state" ||
		types.TraceCausalProjectionStateClass(symptom.StateKind) != "sleep" ||
		symptom.LineStart <= 0 || symptom.LineEnd < symptom.LineStart ||
		symptom.QueryWindowStartTs <= 0 || symptom.QueryWindowEndTs <= symptom.QueryWindowStartTs {
		return 0, false
	}
	subject := runtimeTraceCausalProjectionCanonicalNode(symptom.Subject)
	impact := runtimeTraceProjNodeDisplayImpact(symptom)
	if subject == "" || impact <= 0 || symptom.CumulativeImpactMS <= 0 {
		return 0, false
	}
	found := -1
	for i := range rows {
		row := rows[i]
		base := row.Node
		if row.Kind != runtimeTraceProjTreeRowSelf || row.SelfSymptomRelocated ||
			base.Role != types.TraceCausalRoleCausalHop ||
			strings.TrimSpace(base.Predicate) != "wakeup_causal_impact" ||
			types.TraceCausalProjectionStateClass(base.StateKind) != "sleep" ||
			runtimeTraceCausalProjectionCanonicalNode(base.Subject) != subject ||
			base.LineStart != symptom.LineStart || base.LineEnd < symptom.LineEnd ||
			runtimeTraceProjNodeDisplayImpact(base) != impact ||
			base.CumulativeImpactMS != symptom.CumulativeImpactMS ||
			base.QueryWindowStartTs <= 0 || base.QueryWindowEndTs <= base.QueryWindowStartTs ||
			math.Abs(base.QueryWindowStartTs-symptom.QueryWindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
			math.Abs(base.QueryWindowEndTs-symptom.QueryWindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS {
			continue
		}
		if found >= 0 {
			return 0, false // more than one possible carrier: never guess
		}
		found = i
	}
	return found, found >= 0
}

// runtimeTraceProjAnnotateFoldRosterRankPointers is a display-only B6 pass.
// MergedSubjects is mutated only on the model's node copies, after every
// identity/sort/lead/badge/window decision has completed. The raw projection
// roster remains untouched and the full subject text remains present before
// the pointer suffix — no evidence or member is removed.
func runtimeTraceProjAnnotateFoldRosterRankPointers(model *runtimeTraceProjTreeModel, zh bool) {
	if model == nil {
		return
	}
	type rankSeat struct {
		rank      int
		ambiguous bool
	}
	seats := map[string]rankSeat{}
	groups := [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background}
	for _, rows := range groups {
		for _, row := range rows {
			if !row.HasData || row.Node.OnChainOverflowFold {
				continue
			}
			// UXR-1 (§29.36.2): the roster pointer names a 根因排序 board seat
			// — only chain-channel ordinals qualify (an adjacent 邻近影响#N is
			// a different channel's number; pointing "见榜位#N" at it would
			// cross the 两把尺 boundary the channels exist to keep apart).
			if runtimeTraceProjRowOrdinalChannel(row) != runtimeTraceProjOrdinalChannelChain {
				continue
			}
			rank, _ := runtimeTraceProjCauseRankConfidence(row)
			if rank <= 0 {
				continue
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject)
			if subject == "" {
				continue
			}
			seat, exists := seats[subject]
			if !exists {
				seats[subject] = rankSeat{rank: rank}
				continue
			}
			// A pointer is safe only when the canonical subject owns exactly one
			// visible seat occurrence. Two query windows can legitimately publish
			// the same subject at the same ordinal (for example both rank #2);
			// the ordinal alone then names neither seat uniquely even though each
			// row carries its own window chip.
			seat.ambiguous = true
			seats[subject] = seat
		}
	}
	if len(seats) == 0 {
		return
	}
	annotate := func(rows []runtimeTraceProjTreeRow) {
		for i := range rows {
			if !rows[i].Node.OnChainOverflowFold || len(rows[i].Node.MergedSubjects) == 0 {
				continue
			}
			// Node copies are shallow: detach the roster before adding display
			// suffixes so the typed projection and another language render stay
			// byte-identical.
			rows[i].Node.MergedSubjects = append([]string(nil), rows[i].Node.MergedSubjects...)
			for j, raw := range rows[i].Node.MergedSubjects {
				seat, ok := seats[runtimeTraceCausalProjectionCanonicalNode(raw)]
				if !ok || seat.ambiguous || seat.rank <= 0 {
					continue
				}
				if zh {
					rows[i].Node.MergedSubjects[j] = fmt.Sprintf("%s(见榜位#%d)", raw, seat.rank)
				} else {
					rows[i].Node.MergedSubjects[j] = fmt.Sprintf("%s (see %s #%d)", raw, tracefence.SeatChannelChainEN, seat.rank)
				}
			}
		}
	}
	annotate(model.TreeRows)
	annotate(model.SelfRows)
	annotate(model.Adjacent)
	annotate(model.Background)
}

// runtimeTraceProjStampStaleChannelOrdinals (UXR-1 复核 P2-3, 2026-07-11)
// fail-closes stale-artifact ordinals on the ◇ adjacent channel: an
// old-engine note carries a GLOBAL ordinal (pre-§29.36.2 single rankPos
// space), and replaying it under the per-channel wording would re-mint the
// old global position as a fresh channel ordinal (global rank=9 rendered
// 邻近影响#9 on a one-row ◇ stanza = a seat claim the rendered board cannot
// carry). The background side already fail-closes structurally (通道3 无序数);
// this pass is the ◇-side symmetric arm: a displayed ordinal EXCEEDING the
// adjacent channel's MEMBER population is marked stale, and the chip / detail
// seat line drop (runtimeTraceProjSeatChipWord + the detail face read the
// flag).
//
// Population = channel MEMBERS, not bare rendered rows: display-side folds
// legitimately collapse several channel members into one rendered row while
// the survivor ADOPTS a member's ordinal (the §11-N2 occurrence merge keeps
// the rank member's identity — typed MergedCount; the RNB/B9 rank-twin fold
// rides RankFoldPeers), so each rendered adjacent row counts its typed
// member-preserving carriers: max(1, MergedCount) + len(RankFoldPeers).
// Engine ordinals are contiguous 1..K over channel members (K ≤ population),
// so a legitimate board — folded or not — is never touched; only an ordinal
// no member set can account for fail-closes. Chain-channel ordinals are not
// in scope here (their population spans the engine's cross-board seats; no
// adjudicated witness form).
func runtimeTraceProjStampStaleChannelOrdinals(model *runtimeTraceProjTreeModel) {
	groups := [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background}
	population := 0
	for _, rows := range groups {
		for i := range rows {
			if !rows[i].HasData ||
				runtimeTraceProjRowOrdinalChannel(rows[i]) != runtimeTraceProjOrdinalChannelAdjacent {
				continue
			}
			members := 1
			if rows[i].Node.MergedCount > 1 {
				members = rows[i].Node.MergedCount
			}
			population += members + len(rows[i].RankFoldPeers)
		}
	}
	for _, rows := range groups {
		for i := range rows {
			if runtimeTraceProjRowOrdinalChannel(rows[i]) != runtimeTraceProjOrdinalChannelAdjacent {
				continue
			}
			if rank, _ := runtimeTraceProjCauseRankConfidence(rows[i]); rank > population {
				rows[i].SeatOrdinalStale = true
			}
		}
	}
}

// runtimeTraceProjStampRankWindowChips (PTV8-RCR-C, §24.13 裁定二后半,
// 2026-07-08): when rank seats from TWO OR MORE typed query windows render in
// one report, every seat ordinal carries its window identity — the bare
// 根因排序#1 ×2 collision gave the reader two "top seats" with no board
// identity (§24.11 C-5 / cmp_78_01). Typed inputs only: the row's own
// QueryWindow identity (absence never guesses — a rank row without a typed
// window stays untagged even on the multi-board form); the single-board form
// stays byte-identical.
//
// XLANE-3 件1/件2 (§29.104.2 定谳③ + §29.104.9 形③, 2026-07-16): board
// identity is the typed TRIPLE (query window, board target subject, params
// fingerprint) — the window-endpoint-only key left two same-window
// different-target steps fused into one projection as indistinguishable
// ordinal domains (donghu 形③: 根因排序#1..#3 各×2, zero disambiguation).
// The census now additionally counts distinct non-empty RankBoardTarget per
// window and distinct non-empty params fingerprints per (window, target);
// ≥2 of either flips the report into multi-board mode, and the chip appends
// the discriminating half only where the collision actually lives (板锚 when
// the row's window hosts ≥2 board targets; 参数# when its (window, target)
// hosts ≥2 fingerprints). Absence never splits and never guesses: rows
// without the typed board notes keep the legacy window-only identity
// byte-identically, and never mint an extra board on their own.
func runtimeTraceProjStampRankWindowChips(model *runtimeTraceProjTreeModel, zh bool) {
	groups := [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background}
	// 修补轮 P3 统一 (2026-07-16): the census derives from the ONE board-
	// identity index (window clustering + verbatim target/params halves) —
	// the former exact-float windowKey second implementation is retired.
	chipRow := func(row *runtimeTraceProjTreeRow) bool {
		if !row.HasData {
			return false
		}
		// UXR-1 (§29.36.2): background rows carry no seat chip (通道3
		// 无序数) — a stale persisted Rank there neither receives a window
		// chip nor flips the report into multi-board mode. 复核 P2-3: a
		// stale-marked channel ordinal is chipless the same way.
		if runtimeTraceProjRowOrdinalChannel(*row) == runtimeTraceProjOrdinalChannelBackground ||
			row.SeatOrdinalStale {
			return false
		}
		// 件F (2026-07-16): the micro anchored-seat fold row carries its
		// members' UNIFORM board identity and its detail face still prints
		// their ordinal range — it is chip population despite Rank==0.
		if row.Node.MicroAnchorFold {
			return true
		}
		rank, _ := runtimeTraceProjCauseRankConfidence(*row)
		return rank > 0
	}
	var population []*runtimeTraceProjTreeRow
	for _, rows := range groups {
		for i := range rows {
			if chipRow(&rows[i]) {
				population = append(population, &rows[i])
			}
		}
	}
	index := runtimeTraceProjRankBoardIndexFor(population)
	// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): the multi-board threshold stays
	// the ordinary rows' gate; a MULTI-WINDOW MERGED seat additionally stamps
	// its chip even on the single-board form, because there the chip is no
	// longer board disambiguation — the seat's ordinal window (typed
	// RankQueryWindow pair = the seat-supplying MEMBER's window) is only part
	// of the row's window identity and must say so: the chip carries the
	// 「(供席成员窗,成员跨K窗)」 qualifier (one span-word emitter). A merged
	// row without a resolvable chip window stays untagged (absence never
	// guesses); its span disclosure still rides the ◎ transcription and the
	// detail 窗来源 lane.
	//
	// 修补轮 件E (2026-07-16): the chip trigger splits into two lanes —
	//   - multiWindow (≥2 explicit window clusters): every population row
	//     with a resolvable window wears the window chip (the legacy
	//     PTV8-RCR-C form, byte-identical);
	//   - boardSplit (a window hosts ≥2 boards through the target/params
	//     halves — the unnamed legacy board counts): only rows CARRYING the
	//     typed board target wear chips; identity-less rows keep the bare
	//     legacy form (无名板零 chip — absence never wears a board claim).
	multiWindow := index.windowClusters >= 2
	for _, row := range population {
		spanWord, merged := runtimeTraceProjMergedMemberWindowSpanWord(row.Node, zh)
		target := strings.TrimSpace(row.Node.RankBoardTarget)
		windowID := index.windowIDs[row]
		boardSplitHere := index.boardSplitInWindow(windowID) && target != ""
		if !multiWindow && !merged && !boardSplitHere {
			continue
		}
		start, end, ok := runtimeTraceProjRankChipWindow(row.Node)
		if !ok {
			// RNB-5B 件⑨ (§29.96.2 终判⑨, 2026-07-15): a MULTI-WINDOW
			// merged seat whose chip window is unresolvable states the
			// typed multi-window FACT without guessing endpoints — the
			// member windows stay on the detail 窗来源 lane. Ordinary
			// (non-merged) rows keep the untagged legacy form.
			if merged {
				if zh {
					row.RankWindowChip = "多窗(端点见明细)"
				} else {
					// One unbroken token (no inner space until the tail) so
					// the row-cap wrap cannot split the term from its
					// qualifier mid-word (legend probe stays a substring).
					row.RankWindowChip = "multi-window(endpoints in detail)"
				}
				row.RankWindowChipNoEndpoints = true
			}
			continue
		}
		chip := fmt.Sprintf("窗%.3f~%.3fs", start, end)
		if !zh {
			chip = fmt.Sprintf("window %.3f~%.3fs", start, end)
		}
		if merged {
			if zh {
				chip += "(供席成员窗," + spanWord + ")"
			} else {
				chip += " (seat member's; " + spanWord + ")"
			}
		}
		// XLANE-3 件2: the board-anchor half rides ONLY where the window
		// half alone is ambiguous — this row's window cluster hosts ≥2
		// distinct board targets (the unnamed legacy board counts — 件E).
		// Verbatim canonical target label (勿启发式截断).
		if target != "" && len(index.targetsInWindow[windowID]) >= 2 {
			if zh {
				chip += "·板锚 " + target
			} else {
				chip += " · board " + target
			}
			row.RankBoardAnchorChip = true
		}
		// XLANE-3 件2: the params half rides ONLY where (window, target)
		// still collides — two boards with one target and one window whose
		// rank knobs differ. Verbatim engine fingerprint.
		if fp := strings.TrimSpace(row.Node.RankBoardParamsFingerprint); fp != "" && target != "" &&
			len(index.paramsInBoard[windowID+"\x00"+target]) >= 2 {
			if zh {
				chip += "·参数#" + fp
			} else {
				chip += " · params #" + fp
			}
			row.RankBoardParamsChip = true
		}
		row.RankWindowChip = chip
	}
}

// runtimeTraceProjRankChipWindow resolves the query window a rank ordinal's
// 窗X–Ys chip names: the row's own typed QueryWindow identity first, else the
// merge-preserved RankQueryWindow pair (DISP-3, §29.8 P2-⑧ E22 窗标回归形:
// the §11-N2 merge zeroes the row-level pair whenever ×N members span
// windows, and the ◇ seat's 根因排序#N chip silently lost its board identity
// on exactly the multi-window reports the chip exists for — huadong_792 E22
// "根因排序#2·置信中" vs the pre-merge huadong_79 chips). ok=false when
// neither pair is set: absence never guesses, the seat stays untagged.
func runtimeTraceProjRankChipWindow(n types.TraceCausalProjectionNode) (float64, float64, bool) {
	if n.QueryWindowStartTs > 0 && n.QueryWindowEndTs > n.QueryWindowStartTs {
		return n.QueryWindowStartTs, n.QueryWindowEndTs, true
	}
	if n.RankQueryWindowStartTs > 0 && n.RankQueryWindowEndTs > n.RankQueryWindowStartTs {
		return n.RankQueryWindowStartTs, n.RankQueryWindowEndTs, true
	}
	return 0, 0, false
}

// runtimeTraceProjAssignTopBadges stamps the ❶..❺ TOP-5 root-cause badges.
//
// EVOLUTION RECORD (§29.27.1 用户裁定 2026-07-11, ledger
// real_trace_campaign_20260705.md — 徽章跟随席位, badge follows the SEAT):
// the retired PTV4-T6 lane assigned badge ordinals by position on the display
// board (EffectiveImpactMS-sorted, one-seat-per-subject dedupe, chain-lane
// kinds only) — badges followed row SHAPE, not the published seat. Witness
// (opendir_792/textup_792): the tree's #1 lock row wore ❶ while the #2 drill
// row and the #3 IO family fold sat bare, and on vc_710 ❷ landed on seat #4
// and ❸ on seat #5 (每 lane 逐个实现的典型逐 SHAPE 病). Per the ruling the
// badge is now the PICTOGRAPH OF THE PUBLISHED SEAT ORDINAL: every rendered
// row whose displayed root-cause seat is #1..#5 wears the matching glyph, on
// every lane / row shape / render surface (tree, unattached, drill, stanza,
// semantic ✦), from this SINGLE emission authority. The seat source is the
// same displayed-seat resolver 行2 prints (runtimeTraceProjCauseRankConfidence
// — node Rank or the min folded rank-twin peer), so the badge can never
// disagree with the row's own 根因排序#N text. The PTV6 #11 one-seat-per-
// subject dedupe is retired WITH the lane: seats are engine-exclusive per
// ordinal (§29.19 ORD), and a seat rendered on several surfaces wears its
// badge on each (multi-board renders disambiguate via the §24.13 window chip,
// exactly like the ordinal text itself).
//
// Negative gates (defense in depth, shared with the lead-election board):
// target-self wait-symptom tier rows and on-chain overflow fold rosters never
// wear a badge even against a stale engine Rank (SYM §24.13 / PTS); seatless
// rows (Rank 0 — symptom-demoted, data_gap diagnostics) never wear one by
// construction. 误伤面双向 pin: TestCov4BadgeNegativeGates.
func runtimeTraceProjAssignTopBadges(model *runtimeTraceProjTreeModel) {
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			rows[i].Badge = runtimeTraceProjRowSeatBadgeOrdinal(rows[i])
		}
	}
}

// runtimeTraceProjRowValidSeat is the §29.30.1 SINGLE shared "有效持席"
// (valid-seat) gate consumed by BOTH the badge authority
// (runtimeTraceProjRowSeatBadgeOrdinal) and the lead-election board
// (runtimeTraceProjRankBoard) — one implementation, so "❶ on row A while
// 主根因 crowns row B" and "a zero-impact row crowned" same-page
// contradictions are impossible by construction (远端同事要求,用户确认
// 2026-07-11; the board's former second predicate copy is retired into this
// helper). A seat is valid iff ALL of:
//   - HasData (rendered data row),
//   - displayed seat ordinal ∈ 1..runtimeTraceProjBadgeTopN
//     (runtimeTraceProjCauseRankConfidence — node Rank or the min folded
//     rank-twin peer, the same resolver 行2 prints),
//   - EffectiveImpactMS > 0 (zero/negative effective attribution holds no
//     seat: it neither wears a glyph nor competes for the crown; the bare
//     「#N」 ordinal chip and the row's tree seat stay untouched),
//   - not context_only (6eb633a1 typed tier: causal-path evidence, never a
//     contender — keyed on the tier even for stale positive rank/eff pairs),
//   - not target_self_state (SYM §24.13: the target's own wait symptom),
//   - not an on-chain overflow fold roster (PTS: counted roster, no focus).
//
// Returns the displayed seat ordinal for badge emission; ok=false → 0.
func runtimeTraceProjRowValidSeat(row runtimeTraceProjTreeRow) (int, bool) {
	if !runtimeTraceProjRowSharedSeatArm(row) {
		return 0, false
	}
	// UXR-1 复核 P2-2(b) 裁定 (2026-07-11): the CHANNEL belt — badge/crown
	// only ever follow a CHAIN-channel seat (根因排序#N is the only ordinal
	// the ❶..❺ glyphs and the 主根因 election picture). A row whose typed
	// relevance routes it to the ◇/▒ ordinal channels holds no valid seat
	// here even when a stale persisted Rank>0 leaks onto a tree-lane row
	// (defense in depth beside the P2-2(a) trunk-admission relevance arm; the
	// stanza-kind arms below stay as the lane-kind component of the gate).
	if runtimeTraceProjRowOrdinalChannel(row) != runtimeTraceProjOrdinalChannelChain {
		return 0, false
	}
	// CLOSE-1 复核 F1 (2026-07-11): lane-kind legality is a COMPONENT of seat
	// validity — the election-legal row kinds (chain/cause/depthless/self/
	// semantic-on_chain) are decided HERE, so the badge authority and the
	// election board answer "which rows can hold a valid seat" with one
	// implementation. Background/adjacent stanza rows and non-chain semantic
	// rows hold no valid seat even against a stale positive Rank/eff pair:
	// no glyph (the bare 「#N」 chip stays), no election — a ❶-wearing
	// stanza row beside a differently-crowned 主根因 is the same-page split
	// this gate exists to kill.
	//
	// EVOLUTION RECORD (§29.27.1 → §29.30.1 精化, CLOSE-1 复核 F1): §29.27.1's
	// "凡 Rank∈TOP N 的行,无论 lane/行形/渲染面一律佩戴" is REFINED, not
	// reversed — 佩戴 = 有效持席 (the §29.30.1 valid-seat gate), and lane
	// legality is a component of seat validity. Tree-face lanes (树内 /
	// 父节点未确认 / 下钻 / 自因 / 链上语义 ✦) keep their badges; demoted stanza
	// faces pair with the honest-fallback crown lanes (§7.30 见背景压力段)
	// instead of a glyph.
	switch row.Kind {
	case runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowCause, runtimeTraceProjTreeRowDepthless:
	case runtimeTraceProjTreeRowSelf:
		// §29.30/§29.30.1: a SelfRows-lane seat is valid only through the
		// §24.17 self-cause four-family closed set (runnable/running/IO/
		// D-state). A plain-sleep / binder / lock self row that arrives
		// WITHOUT its engine symptom tier (stale/legacy persisted form)
		// holds no seat on either face — barring it from the crown but
		// letting it wear ❶ would re-open the split above. External
		// (non-self-lane) sleep/lock rows are untouched.
		if !runtimeTraceProjSelfCauseFamilyRow(row) {
			return 0, false
		}
	case runtimeTraceProjTreeRowSemantic:
		// SEM-LEAD (§29.7-2 ①): only an ON-CHAIN semantic row holds a seat
		// (its engine rank arrived via the twin fold); non-chain semantic
		// rows keep the background comprehensive board + mention gate
		// (§23.1 后半) — typed relevance, never a prose judgment.
		if strings.TrimSpace(row.Node.ChainRelevance) != "on_chain" {
			return 0, false
		}
	default:
		return 0, false
	}
	rank, _ := runtimeTraceProjCauseRankConfidence(row)
	if rank < 1 || rank > runtimeTraceProjBadgeTopN {
		return 0, false
	}
	return rank, true
}

// runtimeTraceProjRowSharedSeatArm is the SHARED first arm of the §29.30.1
// valid-seat gate, extracted per ELIM-1 B1 (rank_order_v2_design_20260712.md
// §2.2 共享臂抽取, GREENLIT 2026-07-12; RANK-U Stage 2 commit C): the typed
// row-shape conditions BOTH the badge/lead valid-seat gate above AND the
// ◎ 窗内可消除量总览 admission gate consume —
//
//	HasData ∧ 发布 EffectiveImpactMS > 0
//	∧ tier ∉ {context_only, target_self_state} ∧ 非 overflow fold roster.
//
// Pure refactor (§29.30.1 单门原则延伸, design R4): semantics are verbatim the
// former runtimeTraceProjRowValidSeat first-if — the existing badge/lead/board
// pins are the refactor-correctness judges (全绿零改判据). A second
// independent implementation of these arms is forbidden; per-arm rationale
// lives on runtimeTraceProjRowValidSeat.
func runtimeTraceProjRowSharedSeatArm(row runtimeTraceProjTreeRow) bool {
	return row.HasData && !row.Node.OnChainOverflowFold && !row.Node.IsTargetSelfStateRow() &&
		!row.Node.IsContextOnlyRow() && row.Node.EffectiveImpactMS > 0
}

// runtimeTraceProjRowSeatBadgeOrdinal is the §29.27.1 single badge authority:
// the row's DISPLAYED seat ordinal when it holds a valid seat
// (runtimeTraceProjRowValidSeat — §29.30.1 shared gate), 0 otherwise. Typed
// fields only.
func runtimeTraceProjRowSeatBadgeOrdinal(row runtimeTraceProjTreeRow) int {
	rank, ok := runtimeTraceProjRowValidSeat(row)
	if !ok {
		return 0
	}
	return rank
}

// runtimeTraceProjBadgeTopN is the ❶..❺ badge population bound (§29.27.1 ②:
// TOP 5; ❹ U+2779 / ❺ U+277A sit in the same dingbat block and East-Asian-
// width class as ❶❷❸ — no new width class).
const runtimeTraceProjBadgeTopN = 5

// runtimeTraceProjRankBoard is the SINGLE post-aggregation election board of
// the lead-election primary lane (LEAD 修, ledger §24.11 C-1,
// real_trace_campaign_20260705.md, 2026-07-08): rendered CHAIN-lane rows
// (chain / cause / depthless kinds) carrying the engine's typed
// root_cause_rank (Node.Rank > 0), overflow fold rows excluded (counted
// rosters, never a root-cause focus). Ordered by EffectiveImpactMS descending,
// stable — ties (and the entire eff≤0 tail) keep render order.
//
// EVOLUTION RECORD (§29.27.1, 2026-07-11): the ❶..❺ badge lane no longer
// derives from this board's POSITIONS — badges are the pictograph of each
// row's published seat ordinal (runtimeTraceProjRowSeatBadgeOrdinal, single
// authority) so a board here stays the lead-election population only. On the
// dominant single-window shape the engine orders ordinals by the same
// published eff (§29.22.1 序数键==发布 eff), so board[0] IS the ❶ row there;
// on cross-window ordinal collisions each window's #1 wears ❶ with its
// §24.13 window chip and the lead is still board[0] (eff-max), wearing its
// own seat's glyph (两车道恒等 pin evolved: TestCOVLeadPrimaryIsBadgeOneRow).
//
// EVOLUTION RECORD (§24.11 C-1, huadong_78 witness): the lead election
// previously read projection.PrimaryRootCauses — a PRE-aggregation bucket
// capped at 10 after an in-path-class-first sort — so the rank#1 inversion row
// (E9, ×9 aggregate, class on-chain-only) was evicted from the election pool
// while it wore ❶ on the rendered tree, and the target's own binder-wait
// symptom row was crowned 主根因 against the tree's own badges. The election
// population and the badge population are now the same board; no cap applies
// before election (种群统一 only — the engine rank lanes and §20 direction
// rulings are untouched).
func runtimeTraceProjRankBoard(rows []runtimeTraceProjTreeRow) []*runtimeTraceProjTreeRow {
	var board []*runtimeTraceProjTreeRow
	for i := range rows {
		row := &rows[i]
		// §29.30.1 (2026-07-11) + CLOSE-1 复核 F1: the election admission IS
		// the badge's valid-seat gate — ONE shared implementation (HasData ∧
		// displayed seat 1..TopN ∧ EffectiveImpactMS>0 ∧ 非 context_only ∧
		// 非 target_self_state ∧ 非 overflow fold ∧ lane-kind legality:
		// chain/cause/depthless/self-四族/semantic-on_chain). The former
		// inline predicate copies (SYM §24.13 self-state arm / PTS overflow
		// arm / 6eb633a1 context_only + eff>0 arms / the SEM-LEAD §29.7-2 ①
		// semantic-on_chain arm and the board's own kind switch) are retired
		// into runtimeTraceProjRowValidSeat; per-arm rationale lives there.
		if _, ok := runtimeTraceProjRowValidSeat(*row); !ok {
			continue
		}
		board = append(board, row)
	}
	boardIDs := runtimeTraceProjStableRankBoardIDs(board)
	groups := map[string][]*runtimeTraceProjTreeRow{}
	var groupOrder []string
	for _, row := range board {
		id := boardIDs[row]
		if _, ok := groups[id]; !ok {
			groupOrder = append(groupOrder, id)
		}
		groups[id] = append(groups[id], row)
	}
	for _, id := range groupOrder {
		sort.SliceStable(groups[id], func(i, j int) bool {
			if groups[id][i].Node.Rank != groups[id][j].Node.Rank {
				return groups[id][i].Node.Rank < groups[id][j].Node.Rank
			}
			return groups[id][i].Node.EffectiveImpactMS > groups[id][j].Node.EffectiveImpactMS
		})
	}
	sort.SliceStable(groupOrder, func(i, j int) bool {
		iMax, jMax := 0.0, 0.0
		for _, row := range groups[groupOrder[i]] {
			iMax = math.Max(iMax, row.Node.EffectiveImpactMS)
		}
		for _, row := range groups[groupOrder[j]] {
			jMax = math.Max(jMax, row.Node.EffectiveImpactMS)
		}
		if iMax != jMax {
			return iMax > jMax
		}
		return groupOrder[i] < groupOrder[j]
	})
	flattened := make([]*runtimeTraceProjTreeRow, 0, len(board))
	for _, id := range groupOrder {
		flattened = append(flattened, groups[id]...)
	}
	return flattened
}

// runtimeTraceProjRankBoardIndex is the SINGLE board-identity authority
// (XLANE-3 修补轮 板身份单值源统一, 2026-07-16): every face that judges "which
// board does this row sit on" — the Σ face, the election grouping, the chip
// census, the cross-board mutual-pointer stamper, the coverage board scope —
// derives from THIS one index, so a partial-identity row (e.g. target set,
// fingerprint absent) is judged identically everywhere (the former
// runtimeTraceProjCrossBoardKey second implementation is retired).
type runtimeTraceProjRankBoardIndex struct {
	// ids: full typed-triple board ID per row (window cluster \x00 target
	// \x00 params fingerprint). Key separators are unprintable \x00 so an
	// adversarial comm containing '|' or '·' can never collide two boards
	// (键分隔符注入面).
	ids map[*runtimeTraceProjTreeRow]string
	// windowIDs: the window-cluster half alone.
	windowIDs map[*runtimeTraceProjTreeRow]string
	// targetsInWindow: window cluster → the distinct board-target labels of
	// its rows, INCLUDING "" when identity-less rows are present — a mixed
	// legacy/new window hosts the named board(s) plus the unnamed board.
	targetsInWindow map[string]map[string]bool
	// paramsInBoard: (window cluster \x00 target) → distinct non-empty
	// fingerprints.
	paramsInBoard map[string]map[string]bool
	// windowClusters: the count of distinct explicit window clusters.
	windowClusters int
}

func runtimeTraceProjRankBoardIndexFor(rows []*runtimeTraceProjTreeRow) *runtimeTraceProjRankBoardIndex {
	type windowRow struct {
		row        *runtimeTraceProjTreeRow
		start, end float64
	}
	var explicit []windowRow
	for _, row := range rows {
		if start, end, ok := runtimeTraceProjRankChipWindow(row.Node); ok {
			explicit = append(explicit, windowRow{row: row, start: start, end: end})
		}
	}
	sort.SliceStable(explicit, func(i, j int) bool {
		if explicit[i].start != explicit[j].start {
			return explicit[i].start < explicit[j].start
		}
		return explicit[i].end < explicit[j].end
	})
	index := &runtimeTraceProjRankBoardIndex{
		ids:             map[*runtimeTraceProjTreeRow]string{},
		windowIDs:       map[*runtimeTraceProjTreeRow]string{},
		targetsInWindow: map[string]map[string]bool{},
		paramsInBoard:   map[string]map[string]bool{},
	}
	clusterCount := 0
	anchorStart, anchorEnd := 0.0, 0.0
	currentID := ""
	for _, item := range explicit {
		if currentID == "" || math.Abs(item.start-anchorStart) > types.TraceCausalProjectionSameWindowToleranceS ||
			math.Abs(item.end-anchorEnd) > types.TraceCausalProjectionSameWindowToleranceS {
			clusterCount++
			anchorStart, anchorEnd = item.start, item.end
			currentID = fmt.Sprintf("window=%03d:%.6f..%.6f", clusterCount, anchorStart, anchorEnd)
		}
		index.windowIDs[item.row] = currentID
	}
	index.windowClusters = clusterCount
	for _, row := range rows {
		if index.windowIDs[row] != "" {
			continue
		}
		// Missing-window rows inherit only when the population holds exactly
		// one explicit window cluster — never a guess between two.
		if clusterCount == 1 {
			index.windowIDs[row] = fmt.Sprintf("window=%03d:%.6f..%.6f", 1, anchorStart, anchorEnd)
		} else {
			index.windowIDs[row] = "window=unspecified"
		}
	}
	// XLANE-3 件1 → 修补轮 件E (2026-07-16): the window cluster subdivides by
	// the rows' VERBATIM typed board target and params fingerprint — the
	// former single-value inheritance is RETIRED (混合 legacy/new 返病: one
	// step's stripped seats inherited the other step's named target and the
	// two boards re-fused, reprinting the cross-board Σ 病句). Identity-less
	// rows now form their own unnamed board inside their window cluster
	// (纯 legacy 全剥离形 = every row unnamed = one board, byte-identical
	// legacy behavior; absence still never mints a NAMED board).
	for _, row := range rows {
		windowID := index.windowIDs[row]
		target := strings.TrimSpace(row.Node.RankBoardTarget)
		if index.targetsInWindow[windowID] == nil {
			index.targetsInWindow[windowID] = map[string]bool{}
		}
		// The unnamed "" label counts as a board ONLY through NODE-level
		// seats (node.Rank > 0 — a rank seat that lost its notes IS the
		// unnamed legacy board, 件E). A merely resolver-ranked row (display
		// fold carrying a twin's ordinal, node.Rank==0) must not mint a
		// phantom unnamed board — the pure single-step form legitimately
		// mixes noted seats with note-less fold hosts.
		if target != "" || row.Node.Rank > 0 {
			index.targetsInWindow[windowID][target] = true
		}
		boardHalf := windowID + "\x00" + target
		if fp := strings.TrimSpace(row.Node.RankBoardParamsFingerprint); fp != "" {
			if index.paramsInBoard[boardHalf] == nil {
				index.paramsInBoard[boardHalf] = map[string]bool{}
			}
			index.paramsInBoard[boardHalf][fp] = true
		}
		index.ids[row] = boardHalf + "\x00" + strings.TrimSpace(row.Node.RankBoardParamsFingerprint)
	}
	return index
}

// boardSplitInWindow reports whether a window cluster hosts ≥2 distinct
// boards through the target/params halves: ≥2 distinct target labels (the
// unnamed "" label counts — a mixed legacy/new window IS split), or ≥2
// distinct fingerprints under one (window, target).
func (index *runtimeTraceProjRankBoardIndex) boardSplitInWindow(windowID string) bool {
	if len(index.targetsInWindow[windowID]) >= 2 {
		return true
	}
	for target := range index.targetsInWindow[windowID] {
		if len(index.paramsInBoard[windowID+"\x00"+target]) >= 2 {
			return true
		}
	}
	return false
}

// boardSplit reports whether ANY window cluster hosts ≥2 boards.
func (index *runtimeTraceProjRankBoardIndex) boardSplit() bool {
	for windowID := range index.targetsInWindow {
		if index.boardSplitInWindow(windowID) {
			return true
		}
	}
	return false
}

// runtimeTraceProjStableRankBoardIDs clusters explicit query windows once,
// outside any sort comparator (fixed cluster anchor = transitive relation);
// the board ID is the typed TRIPLE (window cluster, board target, params
// fingerprint) from the ONE shared index above.
func runtimeTraceProjStableRankBoardIDs(rows []*runtimeTraceProjTreeRow) map[*runtimeTraceProjTreeRow]string {
	return runtimeTraceProjRankBoardIndexFor(rows).ids
}

// runtimeTraceProjSelfCauseFamilyRow reports whether a target-self row's
// typed impact form belongs to the §24.17 self-cause four-family closed set
// (自因可拆解族: runnable → 调度压力 / running → 算力供给 / IO → IO阻塞 /
// D-state → D状态) — the families whose root causes are systemic actionable
// items rather than a peer (§24.17 原则②). The form resolver is the SAME
// §24.3 typed table the 行2 category word reads
// (runtimeTraceProjImpactFormForNode — never a prose judgment), so the
// election arm and the row's own displayed identity cannot drift.
func runtimeTraceProjSelfCauseFamilyRow(row runtimeTraceProjTreeRow) bool {
	switch runtimeTraceProjImpactFormForNode(row.Node, row.Kind) {
	case runtimeTraceProjImpactFormRunning, runtimeTraceProjImpactFormRunnable,
		runtimeTraceProjImpactFormDState, runtimeTraceProjImpactFormIOBlock,
		// 件③ tri-form (2026-07-12): the mixed/unproven merged family and
		// the refined-to-iowait rows stay 自因四态 members (same D/IO family
		// as before the category-word fork).
		runtimeTraceProjImpactFormDStateIOMixed, runtimeTraceProjImpactFormIOWaitRefined:
		return true
	}
	return false
}

// runtimeTraceProjLeadElectionRows is the §29.30 lead-election population
// (用户裁定 2026-07-11, 方案 a): every seat-holding row — TreeRows ∪ SelfRows
// — gated per-row by the shared valid-seat arm inside
// runtimeTraceProjRankBoard. Before this ruling the population was
// TreeRows-only, so a self-cause row holding seat #1 wore ❶ (§29.27.1 badge
// follows the seat) while 主根因 crowned another row — the 序值倒挂 lesson's
// crown edition. Adjacent/background stanza rows stay out: a demoted
// candidate keeps the §7.30 裁定1 honest-fallback lanes (见背景压力段), never
// a crown.
func runtimeTraceProjLeadElectionRows(model runtimeTraceProjTreeModel) []runtimeTraceProjTreeRow {
	if len(model.SelfRows) == 0 {
		return model.TreeRows
	}
	rows := make([]runtimeTraceProjTreeRow, 0, len(model.TreeRows)+len(model.SelfRows))
	rows = append(rows, model.TreeRows...)
	rows = append(rows, model.SelfRows...)
	return rows
}

// runtimeTraceProjBadgeGlyph maps the typed seat ordinal to its badge glyph.
// Empty for rank 0 / out-of-range (badges are seats 1..5 only — §29.27.1 ②:
// ❹/❺ come from the same U+2776.. dingbat block and East-Asian-width class as
// ❶❷❸, no new width class).
// EVOLUTION RECORD (UXG-1 M1, 2026-07-12): the badge bytes live in
// internal/tracefence.BadgeGlyphs — the single family source shared with the
// preview chip classifier (traceProjectionRankToken).
func runtimeTraceProjBadgeGlyph(rank int) string {
	badges := tracefence.BadgeGlyphs()
	if rank < 1 || rank > len(badges) {
		return ""
	}
	return badges[rank-1]
}

// runtimeTraceProjSeatOrdinalToken renders a seat ordinal with its §29.27.1
// badge glyph inline ("❷#2"). Single word source for the detail-table face
// and the coverage-account face — the fence face wears the same glyph at the
// row head instead (三面记号一致). 复核 C-1: the glyph keys on the row's
// GATED Badge (the single runtimeTraceProjRowSeatBadgeOrdinal authority),
// never on the raw rank — a stale-Rank symptom row whose tree face is bare
// must not grow a ❶ on the detail face (双面一致); gated rows and seats past
// TOP-5 keep the pre-batch bare "#N" chip form.
func runtimeTraceProjSeatOrdinalToken(rank, badge int) string {
	return runtimeTraceProjBadgeGlyph(badge) + fmt.Sprintf("#%d", rank)
}

// runtimeTraceProjCausalPositionLayerCell is the CMP-7a display wrapper over
// the causal-position layer cell: a flat-fallback row whose chain relevance
// would read "on-chain" renders the flat form instead — the header just said
// the wakeup chain could not be traced upstream, and both surfaces (tree layer
// chip, lossless-table 因果位置 column) must agree with it. Every other layer
// value passes through verbatim.
func runtimeTraceProjCausalPositionLayerCell(node types.TraceCausalProjectionNode, zh, flatChain bool) string {
	layer := runtimeTraceCausalProjectionLayerCell(node, zh)
	// PTV5 C30 (#68): the zh on_chain cell now reads 链上 — the CMP-7a flat
	// rewrite matches both spellings so a flat render still never claims the
	// chain in either language.
	if flatChain && (layer == "on-chain" || layer == "链上") {
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
// UXG-0 D11 (2026-07-11): the same gate covers Kind==Adjacent primary-tier
// non-lead rows — a ◇ stanza row never prints 主根因(优先处理); it wears the
// §29.36.2 adjacent channel word instead (audit fact stays on 邻近影响#N).
func runtimeTraceProjDetailPositionCell(row runtimeTraceProjTreeRow, leadKey string, zh bool) string {
	node := row.Node
	// PTV8-RCR-B (UXA 域B #24 / B#3 verify 关注线程 family, 2026-07-08): the
	// focused thread's own rows never fall to the 支撑 default arm (the block
	// used to say 层级=目标状态 beside 因果位置=支撑, self-contradicting).
	// Typed row-kind judgment only.
	if row.Kind == runtimeTraceProjTreeRowSelf {
		if zh {
			return "关注线程自身"
		}
		return "the focused thread itself"
	}
	if node.IsContextOnlyRow() {
		return runtimeTraceProjDetailPositionMerged(node, zh, row.FlatChain)
	}
	if (row.Kind == runtimeTraceProjTreeRowBackground || row.Kind == runtimeTraceProjTreeRowAdjacent) &&
		runtimeTraceProjPrimaryTierNode(node) &&
		(leadKey == "" || runtimeTraceCausalProjectionNodeKey(node) != leadKey) {
		display := node
		display.Role = types.TraceCausalRoleRootCauseContext
		if strings.HasPrefix(strings.TrimSpace(display.Predicate), "root_cause_primary") {
			display.Predicate = "root_cause_context"
		}
		// EVOLUTION RECORD (UXR-1 §29.36.2, 2026-07-11): the former
		// "(根因排序#N)" audit parenthetical on this demoted-background arm is
		// RETIRED — the background channel carries no ordinal (通道3 无序数),
		// and a stale persisted Rank must not resurrect the 4165
		// 根因排序#1-in-▒ contradiction on the detail face either
		// (三面同一来源: chip, fence and detail all fork on the channel).
		//
		// UXG-0 D11 (2026-07-11): the gate covers the ◇ channel too — an
		// unconsumed primary-tier record rendered in the Adjacent stanza must
		// not print 主根因(优先处理) beside a conclusion that consumed another
		// lead; it demotes to its OWN channel word (邻近(参考) / adjacent
		// (reference), §29.36.2 通道词). The audit fact stays on the channel's
		// 邻近影响#N seat line — never a resurrected 根因排序 word.
		display.ChainRelevance = "background"
		if row.Kind == runtimeTraceProjTreeRowAdjacent {
			display.ChainRelevance = "adjacent"
		}
		return runtimeTraceProjDetailPositionMerged(display, zh, row.FlatChain)
	}
	return runtimeTraceProjDetailPositionMerged(node, zh, row.FlatChain)
}

// runtimeTraceProjDetailPositionMerged (PTV8-RCR-B, UXA 域B #16, 2026-07-08).
// EVOLUTION RECORD: the two-word 「因果位置·优先级」 pairs were three-fifths
// synonym repetition (支撑·支撑参考 / 邻近链·邻近参考 / 背景·支撑参考) — one
// field, priority folded into a parenthetical exactly where it adds
// information. Both word tables (layer + priority) were functions of the same
// typed judgments, so the merge is lossless; the DCS deterministic-
// optimization tier keeps its own arm (§23.1 承重). SEM-LEAD (§29.7-2)
// later promoted typed on-chain semantic work to a full root-cause ranking
// participant, so that arm must distinguish on-chain participation from the
// off-chain "optimization only" form.
func runtimeTraceProjDetailPositionMerged(node types.TraceCausalProjectionNode, zh, flatChain bool) string {
	layer := runtimeTraceProjCausalPositionLayerCell(node, zh, flatChain)
	if zh {
		switch layer {
		case "链路上下文", "邻近上下文", "背景上下文", "上下文":
			return layer + "(不参与根因排序)"
		case "主根因":
			return "主根因(优先处理)"
		case "确定性优化点":
			if runtimeTraceProjSemanticParticipatesInRootCauseRanking(node) {
				return "确定性优化点(链上参与根因排序)"
			}
			return "确定性优化点(优化项,非根因)"
		case "链上":
			return "链上(重点)"
		case "邻近链":
			return "邻近(参考)"
		case "背景":
			return "背景(参考)"
		case "支撑":
			return "支撑(参考)"
		}
		return layer
	}
	switch layer {
	case "chain context", "adjacent context", "background context", "context":
		return layer + " (not ranked)"
	case "primary":
		return "primary (handle first)"
	case "deterministic optimization", "semantic":
		if runtimeTraceProjSemanticParticipatesInRootCauseRanking(node) {
			return layer + " (on-chain root-cause ranking participant)"
		}
		return layer + " (optimization item, not a root cause)"
	case "on-chain":
		return "on-chain (important)"
	case "adjacent":
		// UXG-0 D11 (2026-07-11). EVOLUTION RECORD: the en parenthetical
		// aligns with the zh channel word 邻近(参考) — "(context)" drifted
		// from the 参考 face and forked the §29.36.2 channel word bilingually.
		return "adjacent (reference)"
	case "background":
		return "background (context)"
	case "support":
		return "support (context)"
	}
	return layer
}

// runtimeTraceProjSemanticParticipatesInRootCauseRanking is the display-side
// mirror of the typed SEM-LEAD admission: only a node already classified into
// the semantic/deterministic-optimization display family and carrying a
// resolved on-chain relevance token may claim participation. The caller's
// layer switch proves the semantic family; this helper proves the relation.
// No rank value is required: on-chain semantic work keeps the mention and
// participation obligation even when it falls outside the visible root TOP N.
func runtimeTraceProjSemanticParticipatesInRootCauseRanking(node types.TraceCausalProjectionNode) bool {
	if node.IsContextOnlyRow() {
		return false
	}
	switch strings.TrimSpace(node.ChainRelevance) {
	case "on_chain":
		return true
	case "adjacent", "background":
		// Explicit relevance is the canonical producer judgment and outranks a
		// stale/conflicting causality token, exactly like the projection parser.
		return false
	case "":
		// Older producers may omit relevance while still carrying a typed
		// causality token; resolve that compatibility shape below.
	default:
		return false
	}
	switch strings.TrimSpace(node.Causality) {
	// SELF-SEM (§29.61.1) / SELF-ALL (§29.61.2): the self tokens denote
	// on-chain membership (self_wall_clock never rides a semantic row today —
	// closed-set consistency arm).
	case "on_wakeup_chain", "on_dependency_chain", "self_deterministic", "self_wall_clock":
		return true
	default:
		return false
	}
}

// runtimeTraceProjPrimaryTierNode reports the engine's primary-tier typing on
// a node — the SAME two precise signals the layer/priority cells key on (Role
// enum + root_cause_primary predicate prefix), extracted so the RN-3(b)
// display gate and those cells cannot drift.
func runtimeTraceProjPrimaryTierNode(node types.TraceCausalProjectionNode) bool {
	if node.IsContextOnlyRow() {
		return false
	}
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
	// SEM-LEAD-P0: an engine-typed on-chain semantic rank row is never
	// background pressure. If its independent semantic twin cannot fold (or
	// the subject/depth cannot attach to the visible trunk), keep the honest
	// depthless on-chain seat; relation incompleteness may change layout, never
	// causal lane. Exact typed signals only.
	onChainSemantic := strings.TrimSpace(node.SemanticClass) != "" &&
		(strings.TrimSpace(node.ChainRelevance) == "on_chain" ||
			strings.TrimSpace(node.Causality) == "on_wakeup_chain" ||
			// SELF-SEM (§29.61.1) / SELF-ALL (§29.61.2): self-basis on-chain rows.
			strings.TrimSpace(node.Causality) == "self_deterministic" ||
			strings.TrimSpace(node.Causality) == "self_wall_clock")
	if onChainSemantic {
		return false
	}
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
// the typed IO caliber tokens of the fold set. Exact match on the
// producer's verbatim TypeToken ("type=" rich note) only — never StateKind,
// never Object prose.
//
// EVOLUTION RECORD (IOFAM-SELF, CAL-1 件② §29.47.4①, 2026-07-12): the NEW-3
// two-caliber pair {io_burst_episode, io_wait} widened to the FULL IO facet
// family closed set — single source: the ◇ engine family table exported as
// tracequery.CausalIOFacetFamilyToken (io_wait / io_latency /
// io_burst_episode / block_io_by_inode / page_cache_churn). The 64414 witness
// rendered the same physical IO episode as FIVE flat self rows (io_latency
// 3.670 / block_io 2.694+2.116 / io_wait 1.347+1.248) with THREE ❶ — family
// members must never each carry a lead seat (徽章单点权威=席行唯一).
func runtimeTraceProjSameSegmentIOToken(node types.TraceCausalProjectionNode) bool {
	return tracequery.CausalIOFacetFamilyToken(strings.TrimSpace(strings.ToLower(node.TypeToken)))
}

// runtimeTraceProjIOFoldWallClockFacet — IOFAM-SELF seat eligibility: only a
// WALL-CLOCK facet may hold the family seat (the SHARED registry caliber arm
// decides — composite scores and count facets are roster-only members, 链上
// lane 禁裸 ms 席位).
func runtimeTraceProjIOFoldWallClockFacet(node types.TraceCausalProjectionNode) bool {
	if !runtimeTraceProjSameSegmentIOToken(node) {
		return false
	}
	return tracequery.CausalTokenCaliberSideClass(strings.TrimSpace(strings.ToLower(node.TypeToken))) == tracequery.CausalCaliberSideNone
}

// runtimeTraceProjIOFacetLayerWord — the IOFAM-SELF layered-roster word of a
// facet token (调度等待 / 完成端到端 / 块设备层 / 页缓存层): which measuring
// layer produced the member's value. Closed typed set; "" for tokens outside
// the family (callers keep the bare form).
func runtimeTraceProjIOFacetLayerWord(token string, zh bool) string {
	switch strings.TrimSpace(strings.ToLower(token)) {
	case "io_wait":
		if zh {
			return "调度等待"
		}
		return "scheduler-wait"
	case "io_latency", "io_burst_episode":
		if zh {
			return "完成端到端"
		}
		return "end-to-end"
	case "block_io_by_inode":
		if zh {
			return "块设备层"
		}
		return "block-device"
	case "page_cache_churn":
		if zh {
			return "页缓存层"
		}
		return "page-cache"
	}
	return ""
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
		// SELF-ALL 修复轮 件2 F2 (2026-07-13): the typed proof basis joins the
		// fold key — the ENGINE fold key gained this dimension (两把尺禁混折,
		// rootCauseFamilyFoldLaneKey) while the display fold did not, so the
		// overlap-PROVEN 1.347ms seat (#12) folded into the self-basis
		// family's 「同段IO另有」 note although the engine interval probe
		// proves the segments are pairwise DISJOINT (the 同段 claim was false
		// and the seat went invisible). Same single-implementation discipline:
		// mixed-basis seats render as separate rows; same-basis facets keep
		// folding exactly as before.
		key += "\x00basis=" + strings.TrimSpace(node.OnChainBasis)
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], i)
	}
	folded := map[int]bool{}
	foldPeers := map[string][]types.TraceCausalProjectionNode{}
	for _, key := range groupOrder {
		candidates := groups[key]
		if len(candidates) < 2 {
			continue
		}
		// 修复轮 P2-2 (复核 overlay 实证, 2026-07-12): the widened family made
		// the all-pairs overlap gate a GROUP-level veto — one same-subject
		// page_cache_churn member elsewhere in the window vetoed the whole
		// group and revived the 64414 flat-row disease. The fold now works
		// per overlap CONNECTED COMPONENT (interval-union connectivity, the
		// same 同段 notion as the ◇ engine fold): the veto shrinks to "not in
		// this component". Fail-closed arms preserved — a member without a
		// valid line interval joins no component and keeps its own row.
		for _, members := range runtimeTraceProjIOOverlapComponents(nodes, candidates) {
			if len(members) < 2 {
				continue
			}
			// IOFAM-SELF (件②): the seat goes to the max-impact WALL-CLOCK
			// facet — a composite-score/count member may never hold the
			// family seat on the chain lane (禁裸 ms 席位); a component with
			// no wall-clock facet fails open (the V2-P0 ⌗ lane already words
			// those rows honestly).
			primary := -1
			for _, idx := range members {
				if !runtimeTraceProjIOFoldWallClockFacet(nodes[idx]) {
					continue
				}
				if primary < 0 || runtimeTraceProjNodeDisplayImpact(nodes[idx]) > runtimeTraceProjNodeDisplayImpact(nodes[primary]) {
					primary = idx
				}
			}
			if primary < 0 {
				continue
			}
			primaryKey := runtimeTraceCausalProjectionNodeKey(nodes[primary])
			absorbed := map[string]bool{runtimeTraceCausalProjectionCanonicalNode(nodes[primary].EvidenceID): true}
			for _, id := range nodes[primary].MergedEvidenceIDs {
				absorbed[runtimeTraceCausalProjectionCanonicalNode(id)] = true
			}
			for _, idx := range members {
				if idx == primary {
					continue
				}
				// SELF-ALL 修复轮 件1 F1 连带 (2026-07-13, seat #9 witness): a
				// SEATED row (typed engine Rank > 0) is never absorbed as a
				// fold peer — the ⌗ note carries no ordinal, so folding a
				// ranked facet (io_burst #9 beside the equal-value io_latency
				// #10) punched a hole in the contiguous seat sequence. Same
				// whitelist philosophy as the compile-cap exemption (CR-2 P4);
				// unranked facets keep folding exactly as before.
				if nodes[idx].Rank > 0 {
					continue
				}
				folded[idx] = true
				foldPeers[primaryKey] = append(foldPeers[primaryKey], nodes[idx])
				// 件② E# 并 merged_ids: the seat row's [E#(+N)] tag absorbs
				// the members' evidence identities (the roster note keeps the
				// precise per-member pointers; index registration unchanged).
				for _, id := range append([]string{nodes[idx].EvidenceID}, nodes[idx].MergedEvidenceIDs...) {
					id = strings.TrimSpace(id)
					canon := runtimeTraceCausalProjectionCanonicalNode(id)
					if id == "" || absorbed[canon] {
						continue
					}
					absorbed[canon] = true
					nodes[primary].MergedEvidenceIDs = append(nodes[primary].MergedEvidenceIDs, id)
				}
			}
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

// runtimeTraceProjIOOverlapComponents — 修复轮 P2-2 (2026-07-12; EVOLUTION
// RECORD: supersedes the NEW-3 all-pairs gate runtimeTraceProjIOMembers-
// PairwiseOverlap, whose group-level veto let one distant member unfold the
// whole family): partitions a candidate set into overlap CONNECTED COMPONENTS
// (interval-union connectivity — the same 同段 notion as the ◇ engine fold);
// each component of size ≥2 folds independently.
//
// EVOLUTION RECORD (WO-N1, SMR-1 批 SMR-S13, smr_audit_report §②,
// 2026-07-12): the connectivity edge is now the members' WALL-CLOCK segment
// overlap (typed StartTs/EndTs), never row-number/line-interval containment —
// 席位行号包络连通判被禁 (the vnote 实锤: an ×N family row's LINE envelope
// 4600–15029 swallowed the wall-clock-DISJOINT E25 at lines 13814–14292 in
// row-number space; 56643 E10 stamped 2.411 as「同段」the same way). A member
// without a valid wall-clock interval joins NO component (fail-closed: it
// keeps its own row — absence never proves 同段). E25-class disjoint members
// therefore stay independent rows by construction (保留独立行).
func runtimeTraceProjIOOverlapComponents(nodes []types.TraceCausalProjectionNode, candidates []int) [][]int {
	var valid []int
	for _, idx := range candidates {
		if nodes[idx].StartTs > 0 && nodes[idx].EndTs > nodes[idx].StartTs {
			valid = append(valid, idx)
		}
	}
	assigned := make([]bool, len(valid))
	var components [][]int
	for i := range valid {
		if assigned[i] {
			continue
		}
		component := []int{valid[i]}
		assigned[i] = true
		for grew := true; grew; {
			grew = false
			for j := range valid {
				if assigned[j] {
					continue
				}
				for _, member := range component {
					if runtimeTraceProjTimeSpansOverlap(nodes[member], nodes[valid[j]]) {
						component = append(component, valid[j])
						assigned[j] = true
						grew = true
						break
					}
				}
			}
		}
		components = append(components, component)
	}
	return components
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
		token := g.token
		if zh {
			// PTV5 C09/C16 (#68): the zh tree face speaks the D4 combined form
			// label（raw_token） — the typelabels table already maps the IO
			// caliber tokens (io_wait→iowait, io_burst_episode→IO突发, …); the
			// raw token stays inline for audit fidelity, unmapped tokens pass
			// through verbatim. PTV7 (#74): a label equal to its raw token
			// collapses to the bare token (same rule as the D4 narrative lane).
			if label := runtimeTraceRootCauseTypeZHLabel(g.token); label != "" && label != g.token {
				token = label + "（" + g.token + "）"
			}
		}
		// IOFAM-SELF (件② §29.47.4①, 2026-07-12): the roster is LAYERED — each
		// member wears its measuring-layer word (调度等待/完成端到端/块设备层/
		// 页缓存层), and non-wall-clock members never print bare ms: the
		// composite score wears 「(综合评分,非墙钟)」 (微词面① 2026-07-12:
		// 「分数」首读 fraction 歧义), the count facet wears the
		// 计数当量 family word (both from the SHARED registry caliber arm).
		if layer := runtimeTraceProjIOFacetLayerWord(g.token, zh); layer != "" {
			token = layer + "·" + token
		}
		values := strings.Join(g.values, "/")
		switch tracequery.CausalTokenCaliberSideClass(strings.TrimSpace(strings.ToLower(g.token))) {
		case tracequery.CausalCaliberSideCompositeScore:
			if zh {
				parts = append(parts, strings.TrimSpace(token+" "+values+"(综合评分,非墙钟)"))
			} else {
				parts = append(parts, strings.TrimSpace(token+" "+values+" (score, not wall clock)"))
			}
			continue
		case tracequery.CausalCaliberSideCount:
			if zh {
				parts = append(parts, strings.TrimSpace(token+" 计数当量"+values+"(非墙钟)"))
			} else {
				parts = append(parts, strings.TrimSpace(token+" 计数当量"+values+" (count-equivalent, not wall clock)"))
			}
			continue
		}
		parts = append(parts, strings.TrimSpace(token+" "+values+"ms"))
	}
	// Catalog B12 (DISPLAY-HYG 二轮, §29.104.18.1, 2026-07-17): the evidence
	// pointer tail wears the document-wide bracket style ([E33]、[E35(+1)])
	// — the former bare 「证据 E33」 was the report's only unbracketed E#
	// face (引用双风格), and a bare tail orphaned at a wrap boundary loses
	// its reference identity; the bracket form is the self-contained wrap
	// atom the 件①(d) E#-ref fusion already protects.
	refs := make([]string, 0, len(tags))
	for _, tag := range tags {
		refs = append(refs, "["+tag+"]")
	}
	if zh {
		// 微词面② (用户裁定 2026-07-12): the trailing 「口径」 dangled like a
		// broken sentence after a T3 wrap — 「等口径」 reads whole on its own
		// line (minimal change; the legend entry stays the semantics home).
		text := "同段IO另有 " + strings.Join(parts, "、") + " 等口径"
		if len(refs) > 0 {
			text += ";证据 " + strings.Join(refs, "、")
		}
		return text
	}
	text := "same-segment IO also measured " + strings.Join(parts, ", ")
	if len(refs) > 0 {
		text += "; evidence " + strings.Join(refs, ", ")
	}
	return text
}

// --- RNB R2 same-segment two-lane fold (§21/§22, 2026-07-07) --------------------
//
// The engine publishes ONE priority-inversion-candidate segment through TWO
// lanes — the root_cause_rank funnel (Object=priority_inversion_candidate,
// rank/tier/confidence) and the wakeup_causal_impact hop lane (Object=the
// dominant state, tree position/edge) — and the tree rendered both, as
// sibling cause rows (cmp_01 E7/E8, opendir E6/E7), a trunk row + cause child
// (huadong E4/E5) or sibling wake rows (huadong E11/E13). The fold keeps the
// CHAIN row (树位/边语义) and merges the rank row's rank badge / confidence
// into 行2; the rank row's E# stays registered on the evidence index and
// mirrored in the lossless stanza. The rank row's TYPE WORD is carried on
// RankFoldPeers.TypeWord but is not yet worded anywhere (its one-time fold
// note reader was retired in aabccb6f; §29.40 OM-6 ruling keeps the field —
// the word-face arm pends v5 P2c). Engine work (the P0-E de-double-publish)
// is untouched — this is the display half.

// runtimeTraceProjRankFoldPeer is one folded rank-lane view of a segment: the
// display annotation payload plus the numerator-invariance carriers (the
// folded row's own coverage-caliber magnitudes — consumed ONLY by
// runtimeTraceProjChainDepthCumulative / runtimeTraceProjModelMaxImpact /
// the runtimeTraceProjUnadmittedOnChainDisclosure MAX (复核 W-B) so the
// coverage numerator, bar scale and unadmitted-disclosure magnitude stay
// identical to the two-row render; they never render as row values. The W-A
// cumulative-equality fold guard bounds the cumulative carrier: a fold only
// happens between agreeing (or absent) cumulative accounts).
type runtimeTraceProjRankFoldPeer struct {
	TypeWord           string
	Rank               int
	Confidence         float64
	EvidenceTag        string
	CumulativeImpactMS float64
	DisplayImpactMS    float64
	// TargetImpactMS carries the folded rank row's typed TargetBlockedMs
	// caliber (COV §24.9 D-1) so the coverage-numerator invariance holds: the
	// peer competes with the same 已由链上解释 ladder it would have used as a
	// standalone row.
	TargetImpactMS float64
}

type runtimeTraceProjSelfSymptomFoldPeer struct {
	EvidenceTag string
}

// runtimeTraceProjSameSegmentTwinKey is the SFD-precedent same-segment
// identity (d5f40952, §15.A display half): canonical subject + the exact
// evidence line range — both lanes carry the engine's OWN verbatim
// LineStart/LineEnd (四证: cmp_01 both :103611-113217, opendir both
// :45689-79142, huadong both :1628546-1629554 / :1027796-1029202), a
// published precise signal, never a label/similarity heuristic. Empty when
// the node lacks a valid line span or a resolvable real subject — such rows
// never fold.
func runtimeTraceProjSameSegmentTwinKey(node types.TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	if !types.TraceCausalProjectionKnownSubject(node.Subject) {
		return ""
	}
	return runtimeTraceCausalProjectionCanonicalNode(node.Subject) +
		"\x00" + strconv.Itoa(node.LineStart) + "\x00" + strconv.Itoa(node.LineEnd)
}

// runtimeTraceProjPropagateDStateProofToTwins — 修复轮 P2-3 (2026-07-12):
// sync the DSTATE-REFINE proof fields across same-segment twin rows (the
// SFD-precedent twin key: canonical subject + exact engine line span). The
// engine mints the proof only on the window_stats D/IO fold row; the
// wakeup_chain causal-impact twin of the SAME physical segments must speak
// the same refined wordface (同段词面互斥灭). Proof and caller travel as a
// unit; rows with their own proof are never overwritten.
func runtimeTraceProjPropagateDStateProofToTwins(nodes []types.TraceCausalProjectionNode) {
	proofs := map[string]types.TraceCausalProjectionNode{}
	for _, node := range nodes {
		if !node.DStateRefinedNonIO {
			continue
		}
		if key := runtimeTraceProjSameSegmentTwinKey(node); key != "" {
			proofs[key] = node
		}
	}
	if len(proofs) == 0 {
		return
	}
	for i := range nodes {
		if nodes[i].DStateRefinedNonIO {
			continue
		}
		key := runtimeTraceProjSameSegmentTwinKey(nodes[i])
		donor, ok := proofs[key]
		if !ok {
			continue
		}
		nodes[i].DStateRefinedNonIO = true
		if nodes[i].BlockedReasonCaller == "" {
			nodes[i].BlockedReasonCaller = donor.BlockedReasonCaller
		}
	}
}

// runtimeTraceProjSameSegMirrorPeer carries a raw-state (root_evidence lane)
// copy of a segment folded into its richer same-segment row — CR-2 组② P5
// 同段收敛 equality arm (witness 14704 E1/E2: one running segment 54.599ms
// published as a candidate-lane row AND a bare context row). The peer
// contributes its evidence id (bracket + index) and, when the surviving row
// lacks a state word, the raw state word for the 行2 状态 slot. Annotation
// only — never an ms account.
type runtimeTraceProjSameSegMirrorPeer struct {
	EvidenceTag string
	StateWord   string
	// Valueless marks a mirror copy that carried no display value of its own
	// (修复轮 P4-c: the detail wording then says 同段(无独立值) instead of
	// claiming 同值).
	Valueless bool
}

// runtimeTraceProjSameSegMirrorRawArm marks the raw-state lane of the P5
// equality fold: the reduced-shape root_evidence wakeup witness (typed lane
// identity = the system-minted observation ID; its Predicate carries the bare
// state/type token). Merged/fold/seat rows never qualify.
func runtimeTraceProjSameSegMirrorRawArm(node types.TraceCausalProjectionNode) bool {
	return strings.Contains(node.EvidenceID, "root_evidence:") &&
		node.MergedCount <= 1 && !node.OnChainOverflowFold && node.Rank == 0 &&
		strings.TrimSpace(node.Predicate) != ""
}

// runtimeTraceProjFoldSameSegmentContextMirrors is the CR-2 组② P5 同段收敛
// display fold — MEMBER ARM ONLY since v5 P1 件① (2026-07-13).
//
// EVOLUTION RECORD (CR-2 P5 → v5 P1 件① B.2, 指回 v3 D1「每节点全章恰 2 次」):
// the EQUALITY arm (raw-state root_evidence copy of one segment folding into
// the richer row of the SAME twin key + µs-value fingerprint, witness 14704
// E1/E2) is RETIRED here — the engine one-seat mint now owns every one of
// its carriers at the aggregation order (types/trace_causal_projection_
// oneseat.go arm A + the pre-existing R1 lane, per-carrier pins in
// trace_causal_projection_oneseat_test.go: valued twin = R1, valueless raw /
// eff-lane-valued keeper = arm A), so the pair never reaches this layer.
// The engine form renders the design-sanctioned merged_ids face ([E#(+N)] +
// state-word back-fill) instead of the interim 同段镜像已并入 tag.
//
// The MEMBER arm below stays as the LEGACY-lane defense: current production
// raw witnesses carry the typed dominant_state note (v5 P1 件① emission) and
// converge at engine arm B; records from OLDER blob sessions can lack both
// the note and a registered state-word Predicate (d_state_or_io_wait /
// runnable_wait root types), where the causal-token REGISTRY lookup this
// display layer performs (runtimeTraceProjSMR1StateFamily) is the only
// family proof — the engine deliberately owns no registry copy (注册表单源).
// 退役条件: the legacy-record carrier expires (or gains a typed state
// identity through re-emission); until then this arm folds what the engine
// provably could not.
func runtimeTraceProjFoldSameSegmentContextMirrors(nodes []types.TraceCausalProjectionNode) ([]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	foldInto := map[int]int{} // raw node index -> keeper node index
	// P2-3 必要性否决 (SMR-1 修复轮, 2026-07-13): the raw copy's line interval
	// must sit INSIDE the keeper's line envelope — a NECESSITY veto, not a
	// sufficiency proof (行号包络禁令 bans envelope containment as PROOF of
	// 同段; using it as a negative gate only rejects impossible memberships).
	//
	// WO-D1① (SMR-1 批 SMR-S2, smr_audit_report §②, 2026-07-12; 25846 witness:
	// the root_evidence lane re-published each member SEGMENT of the ×3 sleep
	// seat as its own bare row — E1 105.794 ×3 beside E3 80.751 / E5 16.164 /
	// E6 8.879, a five-seat table summing to 292.3ms vs 真值 105.8): the
	// equality arm's keeper gate (MergedCount ≤ 1 + exact span) structurally
	// missed MERGED keepers — 既有 twin 臂全部因指纹过严整体逃逸. The member
	// arm folds a raw root_evidence copy into the ×N keeper when the raw's
	// value is µs-IDENTICAL to a losslessly DERIVABLE member value of the
	// keeper (成员盘存隶属 by µs identity — 禁行号包络连通判) and the state
	// families agree (registry lanes, never word faces). ◌ missing_wakeup /
	// trace_gap raws never fold here (their honesty seats are WO-A1(member)/
	// WO-G1's business — the ⊘链止 face must stay). 按 P5 先例只迁注记 —
	// annotation only, never an ms account.
	for i, node := range nodes {
		if _, done := foldInto[i]; done || !runtimeTraceProjSameSegMirrorRawArm(node) {
			continue
		}
		if node.Undrillable() || runtimeTraceProjTraceGapNode(node) ||
			runtimeTraceCausalProjectionCanonicalNode(node.Predicate) == "missing_wakeup" {
			continue
		}
		rawValue := runtimeTraceProjNodeDisplayImpact(node)
		if rawValue <= 0 {
			continue
		}
		rawFamily := runtimeTraceProjSMR1StateFamily(node)
		if rawFamily == "" {
			continue
		}
		keeper, ambiguous := -1, false
		for j := range nodes {
			if j == i || nodes[j].MergedCount <= 1 || nodes[j].OnChainOverflowFold {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(nodes[j].Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(node.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(nodes[j]) != rawFamily {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(node, nodes[j]) {
				continue
			}
			if !runtimeTraceProjSMR1LineWithinEnvelope(node, nodes[j]) {
				continue // P2-3: outside the keeper's envelope = impossible membership
			}
			hit := false
			for _, member := range runtimeTraceProjSMR1MergedMemberValues(nodes[j]) {
				if runtimeTraceProjSMR1ValuesEqual(rawValue, member) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
			if keeper >= 0 {
				ambiguous = true
				break
			}
			keeper = j
		}
		if keeper < 0 || ambiguous {
			continue // fail-open: the honest extra row beats a guessed fold
		}
		foldInto[i] = keeper
	}
	if len(foldInto) == 0 {
		return nodes, nil
	}
	dropped := map[int]bool{}
	rawByKeeperIdx := map[int][]int{}
	for rawIdx, keeperIdx := range foldInto {
		dropped[rawIdx] = true
		rawByKeeperIdx[keeperIdx] = append(rawByKeeperIdx[keeperIdx], rawIdx)
	}
	kept := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	peers := map[string][]types.TraceCausalProjectionNode{}
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		for _, rawIdx := range rawByKeeperIdx[i] {
			peers[runtimeTraceCausalProjectionNodeKey(node)] = append(
				peers[runtimeTraceCausalProjectionNodeKey(node)], nodes[rawIdx])
		}
		kept = append(kept, node)
	}
	return kept, peers
}

// runtimeTraceProjSameSegMirrorTagTexts builds the 行2 mirror tags (CR-2 组②
// P5) for BOTH row faces (tree/stanza tags builder + self-row demoted parts):
// equality arm — the folded raw-state copy's state word takes the 行2 状态
// slot only when this row carries none (词位取最高车道, the raw word fills a
// gap, never overrides); family arm — the merged twin points at its source
// family row. Marks the shared legend entry on emission.
// runtimeTraceProjHostEdgeViaWordZH maps the HostWakeupEdgeAnchorVia closed-set
// wire token to the zh credential-inventory word (R3-IMPL 修复轮件2, 冷读
// P3-2 — 图例同词: 直接裸边 / 链上跳边). An unknown token passes through
// verbatim (fail-open wording; absence never guesses a nicer word). The EN
// sentence keeps the wire token itself.
func runtimeTraceProjHostEdgeViaWordZH(via string) string {
	switch via {
	case "direct":
		return "直接裸边"
	case "chain_hop":
		return "链上跳边"
	case "direct+chain_hop":
		return "直接+链上跳边"
	default:
		return via
	}
}

func runtimeTraceProjSameSegMirrorTagTexts(row runtimeTraceProjTreeRow, zh bool) []string {
	var out []string
	if len(row.SameSegMirrorPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		word := ""
		for _, peer := range row.SameSegMirrorPeers {
			if peer.StateWord != "" {
				word = peer.StateWord
				break
			}
		}
		text := "同段镜像已并入"
		if !zh {
			text = "same-seg mirror merged"
		}
		if word != "" {
			if zh {
				text = "状态 " + word + "·" + text
			} else {
				text = "state " + word + " · " + text
			}
		}
		out = append(out, text)
	}
	if row.FamilyMirrorRef != "" {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同段镜像·与家族行同源"
		if !zh {
			text = "same-seg mirror of the family row"
		}
		out = append(out, text)
	}
	if row.ValueMirrorRef != "" {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同段镜像·与[" + row.ValueMirrorRef + "]同一物理时间,不可相加"
		if !zh {
			text = "same-seg mirror · same physical time as [" + row.ValueMirrorRef + "], never additive"
		}
		out = append(out, text)
	}
	// WO-D1③ 多引用 tag (SMR-1 批 SMR-S9, 2026-07-12; 31552 E25): the
	// overflow fold's headline re-publishes the combined physical time of the
	// referenced rendered rows — same 同段镜像 lexicon, multi-ref form.
	if len(row.OverflowMirrorRefs) > 0 {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同段镜像·取最大值与[" + strings.Join(row.OverflowMirrorRefs, "]+[") + "]同一物理时间,不可相加"
		if !zh {
			text = "same-seg mirror · the max re-publishes the physical time of [" + strings.Join(row.OverflowMirrorRefs, "]+[") + "], never additive"
		}
		out = append(out, text)
	}
	// P2-2 跨口径穿透 (SMR-1 修复轮, 2026-07-13): the pool projects a rendered
	// row's account through another caliber — same 同段镜像 lexicon family.
	if row.OverflowProjectionRef != "" {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同段镜像·同一物理时间的口径投影·与[" + row.OverflowProjectionRef + "]不可相加"
		if !zh {
			text = "same-seg mirror · caliber projection of the same physical time · never additive with [" + row.OverflowProjectionRef + "]"
		}
		out = append(out, text)
	}
	// P2-2 谦逊注 (soft advisory, unmarked — heuristic gate is legal for soft
	// wording only; the typed lanes above own the hard tags).
	if row.OverflowProjectionHumble {
		text := "折叠成员线程另有已渲染席位,墙钟可能重叠,勿与之直加"
		if !zh {
			text = "the folded members' thread also holds rendered seats; wall clocks may overlap — do not add across"
		}
		out = append(out, text)
	}
	// WO-D3 短期臂 (SMR-1 批 S3-TPF, 2026-07-12): the double-merged twin pair
	// wears mutual mirror tags (同段镜像 lexicon — same legend promise).
	if row.MergedTwinMirrorRef != "" {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同段镜像·与[" + row.MergedTwinMirrorRef + "]同源,不可相加"
		if !zh {
			text = "same-seg mirror · same source as [" + row.MergedTwinMirrorRef + "], never additive"
		}
		out = append(out, text)
	}
	// WO-D2/D4 (SMR-1 批 S2-TPF/SMR-S4, 2026-07-12): the folded flat aggregate
	// disclosure — E# joined the bracket; the diverging eff caliber dual-lists
	// VERBATIM (值不重计,证据照发).
	for _, peer := range row.BranchTwinFoldPeers {
		row.marks.mark(runtimeTraceProjMarkSameSegMirror)
		text := "同源聚合已并入[" + peer.EvidenceTag + "](父节点未确认份,同一物理时间不重复计入)"
		if peer.EffectiveImpactMS > 0 {
			text = fmt.Sprintf("同源聚合已并入[%s](父节点未确认份,同一物理时间不重复计入;该份有效归因口径 %.3fms 与本行分列)",
				peer.EvidenceTag, peer.EffectiveImpactMS)
		}
		if !zh {
			text = "same-source aggregate merged [" + peer.EvidenceTag + "] (parent-unconfirmed copy, same physical time, not re-counted)"
			if peer.EffectiveImpactMS > 0 {
				text = fmt.Sprintf("same-source aggregate merged [%s] (parent-unconfirmed copy, same physical time, not re-counted; its effective caliber %.3fms listed beside this row's)",
					peer.EvidenceTag, peer.EffectiveImpactMS)
			}
		}
		out = append(out, text)
	}
	// WO-A1 (SMR-1 批, §④ 词面单源, 2026-07-12): the non-additive pointer —
	// ONE template, direction word forks on the typed kind. 禁逐处手写.
	if row.NonAdditiveRef != "" && row.NonAdditiveKind != runtimeTraceProjNonAdditiveNone {
		// XLANE-2 件1: the member-subset kind carries its OWN dedicated legend
		// entry (marked at its arm below) — the generic three-word entry's
		// closed set stays byte-identical for the legacy kinds.
		if row.NonAdditiveKind != runtimeTraceProjNonAdditiveMemberSubset {
			row.marks.mark(runtimeTraceProjMarkNonAdditivePointer)
		}
		// One stable family head 「不可相加·」/"non-additive · " keeps the
		// word face bidirectionally probeable (legend ⇔ fence contract) and
		// distinct from the mirror family's trailing ",不可相加".
		var text string
		switch row.NonAdditiveKind {
		case runtimeTraceProjNonAdditiveComponent:
			text = "不可相加·为[" + row.NonAdditiveRef + "]的组成部分"
			if !zh {
				text = "non-additive · component of [" + row.NonAdditiveRef + "]"
			}
		case runtimeTraceProjNonAdditiveContains:
			text = "不可相加·已含[" + row.NonAdditiveRef + "]"
			if !zh {
				text = "non-additive · already contains [" + row.NonAdditiveRef + "]"
			}
		case runtimeTraceProjNonAdditiveMember:
			text = "不可相加·为[" + row.NonAdditiveRef + "]成员"
			if !zh {
				text = "non-additive · member of [" + row.NonAdditiveRef + "]"
			}
		case runtimeTraceProjNonAdditiveMemberSubset:
			// XLANE-2 件1: the member-subset demotion word — 互指 only (the
			// superset seat carries the family account; this seat's values stay
			// untouched, 降道=席位口径变化非值变化). Deliberately WITHOUT the
			// 不可相加· family head: the word owns a dedicated legend entry and
			// the generic three-word entry's closed set stays untouched.
			row.marks.mark(runtimeTraceProjMarkSemanticMemberSubset)
			text = "为[" + row.NonAdditiveRef + "]成员子集(整席降道)"
			if !zh {
				text = "member subset of [" + row.NonAdditiveRef + "] (whole-seat demotion)"
			}
		}
		if text != "" {
			out = append(out, text)
		}
	}
	// XLANE-2 件2 (user ruling §29.104.17 ④ 披露式拆分): the self-gap seat's
	// resolved semantic-overlap clauses — ONE line, per-partner clauses in
	// resolved order (engine order: overlap DESC). 主值零动: the clause
	// discloses the shared physical wall clock and deducts nothing.
	if len(row.SelfGapSemanticOverlapClauses) > 0 {
		row.marks.mark(runtimeTraceProjMarkSelfGapSemanticOverlap)
		parts := make([]string, 0, len(row.SelfGapSemanticOverlapClauses))
		for _, clause := range row.SelfGapSemanticOverlapClauses {
			if zh {
				parts = append(parts, fmt.Sprintf("%.3fms 与语义席[%s]重叠", clause.OverlapMS, clause.Ref))
			} else {
				parts = append(parts, fmt.Sprintf("%.3fms overlaps semantic seat [%s]", clause.OverlapMS, clause.Ref))
			}
		}
		if zh {
			out = append(out, "其中 "+strings.Join(parts, "、"))
		} else {
			out = append(out, "of which "+strings.Join(parts, "; "))
		}
	}
	// AXIOM-V2 件2 (公理 v2, user rulings 2026-07-18): the cross-direction
	// mutual clauses — ONE line, per-partner clauses in resolved order
	// (engine order: overlap DESC); the shared tail names the rule. Both
	// seats of a pair render it or neither (reciprocity pruned at the stamp).
	// 主值零动: the clause discloses the shared physical wall clock and
	// deducts nothing.
	if len(row.CrossDirectionOverlapClauses) > 0 {
		row.marks.mark(runtimeTraceProjMarkCrossDirectionOverlap)
		parts := make([]string, 0, len(row.CrossDirectionOverlapClauses))
		for _, clause := range row.CrossDirectionOverlapClauses {
			word, wordOK := runtimeTraceProjFixDirectionWord(clause.Direction, zh)
			if zh {
				if wordOK {
					parts = append(parts, fmt.Sprintf("与[%s](修向 %s)同段重叠 %.3fms", clause.Ref, word, clause.OverlapMS))
				} else {
					parts = append(parts, fmt.Sprintf("与[%s]同段重叠 %.3fms", clause.Ref, clause.OverlapMS))
				}
			} else {
				if wordOK {
					parts = append(parts, fmt.Sprintf("overlaps [%s] (fix-direction %s) by %.3fms", clause.Ref, word, clause.OverlapMS))
				} else {
					parts = append(parts, fmt.Sprintf("overlaps [%s] by %.3fms", clause.Ref, clause.OverlapMS))
				}
			}
		}
		if zh {
			out = append(out, strings.Join(parts, "、")+":作用于同段时间,修其一后另一席空间会缩,收益不叠加")
		} else {
			out = append(out, strings.Join(parts, "; ")+" — same physical segment: fixing one shrinks the other seat's headroom, the gains do not add")
		}
	}
	// RSPA §29.61.10a (2026-07-14): the same-source bipartition 行2 disclosure
	// — every row carrying the typed decomposition pair names the split; the
	// remainder value is display arithmetic over the ONE engine-minted pair
	// (full − anchored), never a new account. Emitted HERE (the shared 行2
	// relation/disclosure composer) so the tree, ◇/▒ stanza and self faces all
	// speak it from one site.
	if row.Node.ChainAnchorFullMS > 0 {
		full := row.Node.ChainAnchorFullMS
		anchored := row.Node.ChainAnchoredMS
		rem := full - anchored
		if rem < 0 {
			rem = 0
		}
		// The en face joins the "=" compact (like the zh form): the spaced
		// "ms = " token is the EffectiveBreakdown 行3 probe's documented
		// uniqueness invariant (revisit76LegendProbes) and must stay unique
		// to that grammar line.
		var text string
		switch {
		case row.Node.ChainAnchorRemainderSeat && row.Node.ChainAnchorOwnershipDivergent:
			// RNB-1 case A' (§29.88 R2, 2026-07-14): the chain seat did not
			// provably hold the anchored share — the additive 同源二分 claim
			// (「(⛓链上席)」 ownership word) downgrades to the double-account
			// relation: the census split stays ledger-exact, both Σs and the
			// delta are disclosed, and no addition with the chain seat is
			// invited.
			row.marks.mark(runtimeTraceProjMarkChainAnchorDivergent)
			laneMS := row.Node.ChainAnchorChainLaneMS
			censusMS := row.Node.ChainAnchorCensusMS
			delta := laneMS - censusMS
			if delta < 0 {
				delta = -delta
			}
			// RNB-1 D1 修复轮: the 「链席另列自账」 pointer downgrades when
			// the chain seat is on no rendered surface (dangling backstop).
			chainSeatWord, chainSeatWordEN := "链席另列自账", "the chain seat keeps its own separate account"
			if row.ChainAnchorTwinInvisible {
				chainSeatWord = "链席未上本榜(见明细)"
				chainSeatWordEN = "the chain seat is not on this board (see the detail index)"
			}
			// RNB-2 件6 (§29.90 残留③ P4, 2026-07-15): the sentence carries TWO
			// anchored quantities — this row's own anchored share (group-stamp /
			// satellite interval account) and the pid-census anchored ledger Σ.
			// On window seats they µs-coincide and one word is honest; when the
			// typed floats DIFFER the bare 「锚定」 word would pun two values in
			// one sentence — both mentions gain their account qualifier
			// (precise-signal word fork; equal pairs keep the pinned bytes).
			anchoredNoun, censusNoun := "锚定", "锚定账Σ"
			anchoredNounEN, censusNounEN := "anchored", "the anchored-ledger Σ"
			if math.Abs(anchored-censusMS) >= runtimeTraceProjSMR1ValueTieMS {
				anchoredNoun, censusNoun = "本席锚定", "pid全窗锚定账Σ"
				anchoredNounEN, censusNounEN = "anchored by this seat's own account", "the pid-wide anchored-ledger Σ"
			}
			text = fmt.Sprintf("账目关系(锚定权属失合):全窗%.3fms=%s%.3fms+本行其余%.3fms(无链上凭证);链席自账Σ%.3fms 与%s%.3fms 失合(差%.3fms),%s,两席不可相加",
				full, anchoredNoun, anchored, rem, laneMS, censusNoun, censusMS, delta, chainSeatWord)
			if !zh {
				text = fmt.Sprintf("account relation (anchored-ownership divergence): whole-window %.3fms=%.3fms %s + this remainder %.3fms (no chain credential); the chain seat's own Σ %.3fms diverges from %s %.3fms (delta %.3fms) — %s, the two seats are never additive",
					full, anchored, anchoredNounEN, rem, laneMS, censusNounEN, censusMS, delta, chainSeatWordEN)
			}
		case row.Node.ChainAnchorRemainderSeat:
			row.marks.mark(runtimeTraceProjMarkChainAnchorSplit)
			// RNB-1 D1 修复轮: the 「(⛓链上席)」 ownership word downgrades
			// when no ⛓ counterpart rendered (dangling backstop; wording
			// only — the ledger split stays exact).
			anchoredWord, anchoredWordEN := "(⛓链上席)", "(⛓ chain seat)"
			if strings.TrimSpace(row.Node.SemanticClass) != "" {
				// R3-IMPL (§29.88.1): a bisected SEMANTIC span's chain twin
				// is a ✦ semantic seat — the ownership glyph follows the
				// twin's actual family (⛓ would point at a state-icon row
				// that does not exist on a semantic pair; typed
				// SemanticClass fork, 记号-图例双向 discipline).
				anchoredWord, anchoredWordEN = "(✦链上席)", "(✦ chain seat)"
			}
			switch {
			case anchored <= 0:
				// RNB-2 件3 (§29.88 W3 病③, 2026-07-15): anchored==0 means NO
				// seat holds any anchored share — both ownership brackets
				// (「(⛓链上席)」 and the D1 「席不可见」 downgrade) would name
				// a holder that does not exist. The zero-VALUE form wins over
				// the twin-VISIBILITY form (two orthogonal arms: D1 handles
				// 「席不可见」, this handles 「值为零」).
				anchoredWord = "(无锚定段)"
				anchoredWordEN = "(no anchored share exists)"
			case row.ChainAnchorTwinInvisible:
				anchoredWord = "(锚定席未上本榜,见明细)"
				anchoredWordEN = "(anchored seat not on this board; see the detail index)"
			}
			text = fmt.Sprintf("同源二分:全窗%.3fms=锚定%.3fms%s+本行其余%.3fms(无链上凭证)", full, anchored, anchoredWord, rem)
			if !zh {
				text = fmt.Sprintf("same-source split: full-window %.3fms=%.3fms anchored %s + this remainder %.3fms (no chain credential)", full, anchored, anchoredWordEN, rem)
			}
		default:
			row.marks.mark(runtimeTraceProjMarkChainAnchorSplit)
			remainderWord, remainderWordEN := "(◇余段席)", "(◇ remainder seat)"
			if row.ChainAnchorTwinInvisible {
				remainderWord = "(余段席溢出未上榜,见明细)"
				remainderWordEN = "(remainder seat overflowed off this board; see the detail index)"
			}
			text = fmt.Sprintf("同源二分:全窗%.3fms=本行锚定%.3fms+其余%.3fms%s", full, anchored, rem, remainderWord)
			if !zh {
				text = fmt.Sprintf("same-source split: full-window %.3fms=this row %.3fms anchored + remainder %.3fms %s", full, anchored, rem, remainderWordEN)
			}
		}
		out = append(out, text)
	} else if row.Node.MergedChainAnchorMemberAccounts {
		// RNB-2 件2 (§29.88 W3 病①, 2026-07-15): the ×N merged row whose
		// members carried ChainAnchor bipartition accounts — the per-seat
		// triples were cleared by the merge body (seed 三元组不得冒充合并行
		// 账); the qualifier says where the split accounts live instead.
		row.marks.mark(runtimeTraceProjMarkChainAnchorSplit)
		// UXG1 F2: the section noun rides the tracefence constant, never a
		// hand-copied literal.
		text := "同源二分账留在各成员(种子成员账,不代表本合并行合计);成员拆分见" + tracefence.SectionEvidenceZH
		if !zh {
			text = "the same-source split accounts stay on the individual members (seed-member accounts, never this merged row's total); see the evidence index for the member splits"
		}
		out = append(out, text)
	}
	// RNB-1 R4 (§29.88.2, 2026-07-14): the whole-seat lane-demotion
	// disclosure — the row's channel moved to ◇ with every value untouched;
	// this line says WHY it sits there (edge=credential rule).
	if row.Node.ChainCredentialLaneDemoted {
		// HULL-CRED (§29.104 终判③, 2026-07-17): the per-segment-proven fork
		// — spoken ONLY when the demote marker AND its decoded segment
		// inventory ride the row together (claim gated on proof); every
		// other demoted row keeps the generic R4 bytes below unchanged.
		if row.Node.ChainCredentialSegmentDisjoint && len(row.Node.ChainCredentialSegments) > 0 {
			row.marks.mark(runtimeTraceProjMarkChainCredentialSegmentDisjoint)
			text := "无链上凭证(逐段核验,整席降道,见图例)"
			if !zh {
				text = "no chain credential (per-segment verified; whole-seat demotion; see legend)"
			}
			out = append(out, text)
		} else {
			row.marks.mark(runtimeTraceProjMarkChainCredentialDemoted)
			// DISPLAY-WRAP 件③(b) (§29.104.18.1 B3, 2026-07-16): the rule
			// sentence body (该席账目未能出示 typed 因果边锚定份,整席记 ◇ 邻近,
			// 数值零动) lives in the legend's 无链上凭证(整席降道) entry — the
			// row keeps the chip word + the legend pointer only (witness: the
			// full sentence reprinted ×5 across ◇ rows).
			text := "无链上凭证(整席降道,见图例)"
			if !zh {
				text = "no chain credential (whole-seat demotion; see legend)"
			}
			out = append(out, text)
		}
	}
	// HULL-CRED (§29.104 终判③, 2026-07-17): the envelope-tier honest word on
	// a keep-⛓ row — credential granularity disclosure only (fail-open
	// 保守留道不变,只加诚实词); never rendered beside a demotion. The engine's
	// four-arm verdict never sets both bools, but a corrupted / foreign
	// artifact can — the display arm re-gates on !LaneDemoted (便宜修轮件2,
	// symmetric with the disjoint word's claim-gated-on-proof gate above) so
	// the contradictory word pair 「无链上凭证」+「(包络级凭证)」 can never
	// share a row; the demotion chip wins (the conservative face).
	// ONCHAIN-FIX-2 件1 (2026-07-18): the word now also rides rank-lane
	// hull-only keeps (same note key/legend — 零新词) and re-gates on the
	// CURRENT on-chain lane (链上面与降道面不同行共存 — a later fold/absorb
	// pass may move a stamped row's channel; the identity word below already
	// carries this gate).
	if row.Node.ChainCredentialEnvelopeLevel && !row.Node.ChainCredentialLaneDemoted &&
		strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" {
		row.marks.mark(runtimeTraceProjMarkChainCredentialEnvelope)
		text := "(包络级凭证,见图例)"
		if !zh {
			text = "(envelope-level credential; see legend)"
		}
		out = append(out, text)
	}
	// ONCHAIN-FIX-2 件3 (Q6 已追认, 2026-07-18): the truncated lower-bound
	// prefix word on a keep-⛓ row whose published credential is the checked
	// prefix of a beyond-cap group — 「实际锚定不小于此值」 caliber. Gated on
	// the decoded inventory riding beside it (claim on proof), the current
	// on-chain lane and no demotion (a corrupted / foreign artifact could set
	// contradictory bits — display re-gates like the sibling arms).
	if row.Node.ChainCredentialSegmentsTruncated && len(row.Node.ChainCredentialSegments) > 0 &&
		!row.Node.ChainCredentialLaneDemoted && !row.Node.ChainCredentialSegmentDisjoint &&
		strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" {
		row.marks.mark(runtimeTraceProjMarkChainCredentialTruncatedLowerBound)
		text := "(凭证清单不完整,实际锚定不小于所证,见图例)"
		if !zh {
			text = "(credential inventory incomplete; anchored share is at least the proven; see legend)"
		}
		out = append(out, text)
	}
	// ONCHAIN-FIX-1 件1 (2026-07-18): the identity-inheritance honest word on
	// an interval-less fail-open keep-⛓ row — credential-tier disclosure only
	// (the fabricated whole-node-window overlap value it replaces is retired
	// engine-side). Renders ONLY on the current on-chain lane (链上面与降道面
	// 不同行共存) and only when NO stronger credential vocabulary rides the
	// row: the demotion words, the HULL-CRED per-segment inventory and the
	// envelope word all win (a corrupted / foreign artifact could set
	// contradictory bits — the display re-gates like the envelope arm above).
	if row.Node.ChainIdentityInheritance && !row.Node.ChainCredentialLaneDemoted &&
		!row.Node.ChainCredentialEnvelopeLevel && len(row.Node.ChainCredentialSegments) == 0 &&
		strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" {
		row.marks.mark(runtimeTraceProjMarkChainIdentityInheritance)
		text := "身份继承(链窗级,无区间凭证,见图例)"
		if !zh {
			text = "identity inheritance (chain-window tier, no interval credential; see legend)"
		}
		out = append(out, text)
	}
	// XLANE-1 件1 (§29.104.1/§29.104.2, 2026-07-15): the represented-by-
	// chain-seat demotion disclosure — the honest sibling of the R4 sentence
	// above (this seat HAS credential; the reason is seat representation).
	// The chain-seat pointer reuses the typed cross-channel ref when the
	// stamp pass resolved one (the [E#] the reader can jump to); a ref-less
	// render keeps the generic 本线程链上席 noun instead of guessing an E#.
	if row.Node.ChainAnchorRepresentedByChainSeat {
		row.marks.mark(runtimeTraceProjMarkChainAnchorRepresented)
		seatWordZH, seatWordEN := "本线程链上席", "this thread's chain-lane seat"
		if ref := strings.TrimSpace(row.CrossChannelChainRef); ref != "" {
			seatWordZH = "链席[" + ref + "]"
			seatWordEN = "the chain seat [" + ref + "]"
		}
		text := "锚定份由" + seatWordZH + "代表(整席降道):该席账目全额锚定于 typed 唤醒依赖窗内(有凭证),同段物理时间已由链上席全额代表,本席为诊断投影记 ◇ 邻近、不重复参赛,数值不变"
		if !zh {
			text = "anchored share represented by " + seatWordEN + " (whole-seat demotion): this seat's whole account is anchored inside typed wakeup-dependency windows (it HAS credential) and the same physical time is already fully represented on the chain tier — this diagnostic projection rides the ◇ adjacent channel without competing again, values unchanged"
		}
		out = append(out, text)
	}
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the gated-share split
	// word family. The inversion-seat pointer consumes the resolved [E#]
	// refs when the stamp pass matched every typed claim-seat line interval;
	// a ref-less render keeps the generic 本线程反转席 noun (宁漏勿假指).
	if row.Node.GatedShareConstituentSeat {
		// 修补轮 件2⑤ (2026-07-18): the constituent arm gates on the typed
		// marker bool, not the account float — the ×N merged Σ form keeps
		// GatedShareConstituentSeat=true while the per-seat-ledger clear
		// zeroed claimed/full (「本行」 grammar has no true referent on a
		// member Σ), and the ◎ census still claims the row, so the row-2 face
		// must self-explain instead of going silent (◎ 脚注不再单面).
		row.marks.mark(runtimeTraceProjMarkGatedShareSplit)
		invWordZH, invWordEN := "本线程反转席", "this thread's priority-inversion seat"
		if len(row.GatedShareClaimRefs) > 0 {
			invWordZH = "反转席[" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
			invWordEN = "the priority-inversion seat [" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
		}
		if row.Node.GatedShareFullMS > 0 {
			text := fmt.Sprintf("分账构成份·归因已计入%s:本行 %.3fms 为全账 %.3fms 中与反转席分支窗重叠、已由其 gated 复合计入的份额,不参赛、不与之相加",
				invWordZH, row.Node.GatedShareClaimedMS, row.Node.GatedShareFullMS)
			if !zh {
				text = fmt.Sprintf("split-account constituent share · attribution already counted at %s: this row's %.3fms is the share of the %.3fms account overlapping that seat's branch windows, counted by its gated composite — never competing, never additive with it",
					invWordEN, row.Node.GatedShareClaimedMS, row.Node.GatedShareFullMS)
			}
			out = append(out, text)
		} else {
			text := "分账构成份(合并行):各成员份额已由同线程反转席 gated 复合计入,成员级分账见明细行,不参赛、不与反转席相加"
			if !zh {
				text = "split-account constituent share (merged row): every member share is already counted by the same thread's priority-inversion seat gated composite; the per-member decomposition lives on the member detail rows — never competing, never additive with that seat"
			}
			out = append(out, text)
		}
	} else if row.Node.GatedShareFullMS > 0 {
		row.marks.mark(runtimeTraceProjMarkGatedShareSplit)
		invWordZH, invWordEN := "本线程反转席", "this thread's priority-inversion seat"
		if len(row.GatedShareClaimRefs) > 0 {
			invWordZH = "反转席[" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
			invWordEN = "the priority-inversion seat [" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
		}
		residual := row.Node.GatedShareFullMS - row.Node.GatedShareClaimedMS
		if residual < 0 {
			residual = 0
		}
		text := fmt.Sprintf("分账残余席:全账 %.3fms = 已计入%s份 %.3fms + 本席残余 %.3fms(同一段集两不重叠份,可加还原全账)",
			row.Node.GatedShareFullMS, invWordZH, row.Node.GatedShareClaimedMS, residual)
		if !zh {
			text = fmt.Sprintf("split-account residual seat: full account %.3fms = %.3fms counted at %s + this seat's residual %.3fms (one segment set, two disjoint shares, additive back)",
				row.Node.GatedShareFullMS, row.Node.GatedShareClaimedMS, invWordEN, residual)
		}
		out = append(out, text)
	}
	// LEVELMERGE-1 件2 fail-open (裁定④ §29.104.17 句形): the overlap
	// disclosure clause — published value untouched, no value split.
	if row.Node.GatedShareOverlapDisclosureMS > 0 {
		row.marks.mark(runtimeTraceProjMarkGatedShareOverlap)
		invWordZH, invWordEN := "本线程反转席", "this thread's priority-inversion seat"
		if len(row.GatedShareClaimRefs) > 0 {
			invWordZH = "反转席[" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
			invWordEN = "the priority-inversion seat [" + strings.Join(row.GatedShareClaimRefs, "][") + "]"
		}
		text := fmt.Sprintf("其中 %.3fms 与%s重叠(按现有真段区间测得,实际重叠不小于此值;typed 区间清单不完整,未做值拆分,主值不变,不可相加)",
			row.Node.GatedShareOverlapDisclosureMS, invWordZH)
		if !zh {
			text = fmt.Sprintf("%.3fms of this account overlaps %s (measured over the available real segments — the true overlap is at least this; the typed interval inventory is incomplete, no value split, published value unchanged, never additive)",
				row.Node.GatedShareOverlapDisclosureMS, invWordEN)
		}
		out = append(out, text)
	}
	// LEVELMERGE-1 件3 (两向互指, 2026-07-18): the aggregate-seat ↔ member
	// pointer pair — both directions stamped all-or-nothing at model build.
	if len(row.AggregateMemberRefs) > 0 {
		row.marks.mark(runtimeTraceProjMarkAggregateMemberCrossRef)
		text := "构成段见[" + strings.Join(row.AggregateMemberRefs, "][") + "](本席数值已计入全部构成段,构成段行不另计)"
		if !zh {
			text = "constituent segments at [" + strings.Join(row.AggregateMemberRefs, "][") + "] (the seat value already counts every constituent segment; the member rows are not counted again)"
		}
		out = append(out, text)
	}
	if row.AggregateSeatRef != "" {
		row.marks.mark(runtimeTraceProjMarkAggregateMemberCrossRef)
		text := "归因已计入[" + row.AggregateSeatRef + "](聚合席),本行为构成段,不另计"
		if !zh {
			text = "attribution already counted at [" + row.AggregateSeatRef + "] (the aggregate seat); this row is a constituent segment, not counted again"
		}
		out = append(out, text)
	}
	// R3-IMPL (§29.88.1 user ruling, 2026-07-15): the host-edge-anchored
	// semantic seat's credential disclosure — typed OnChainBasis single-field
	// fork (the SELF-SEM qualifier discipline), R4-family 边=凭证 wording.
	// The boundary ts/via ride the typed pair when present (µs-verifiable
	// against the raw wakeup line); absent parts are omitted, never guessed.
	// ONCHAIN-3c (2026-07-19): the state-seat sibling basis forks to its own
	// value-form clause on the SAME single field (span=pre-edge window
	// projection; state=pre-edge segment-inventory Σ) — same mark, same
	// legend row, same boundary detail composer.
	if basis := strings.TrimSpace(row.Node.OnChainBasis); basis == "host_wakeup_edge_pre_span" || basis == "host_wakeup_edge_pre_state" {
		row.marks.mark(runtimeTraceProjMarkHostEdgeAnchored)
		detail := ""
		if row.Node.HostWakeupEdgeAnchorTS > 0 {
			// 修复轮件1 (冷读 P3-1): 「最晚相关边」 — the boundary IS the
			// latest in-window credential edge; 「最近」 was ambiguous when
			// several edges share the window (名实不符).
			via := strings.TrimSpace(row.Node.HostWakeupEdgeAnchorVia)
			if via != "" {
				// 修复轮件2 (冷读 P3-2): the zh sentence speaks the zh
				// credential-inventory words (图例同词); the EN sentence
				// keeps the closed-set wire token.
				detail = fmt.Sprintf("(最晚相关边 %.6fs,凭证=%s)", row.Node.HostWakeupEdgeAnchorTS, runtimeTraceProjHostEdgeViaWordZH(via))
				if !zh {
					detail = fmt.Sprintf(" (latest credential edge %.6fs, via=%s)", row.Node.HostWakeupEdgeAnchorTS, via)
				}
			} else {
				detail = fmt.Sprintf("(最晚相关边 %.6fs)", row.Node.HostWakeupEdgeAnchorTS)
				if !zh {
					detail = fmt.Sprintf(" (latest credential edge %.6fs)", row.Node.HostWakeupEdgeAnchorTS)
				}
			}
		}
		valueClause := "计入值=span 边前段窗内投影"
		if basis == "host_wakeup_edge_pre_state" {
			valueClause = "计入值=状态段清单边前份合计"
		}
		text := "边锚定(宿主→目标):本席凭宿主线程自身对目标的窗内 typed 唤醒边入链上(边=凭证,边前=有效,边后=解除)," + valueClause + detail
		if !zh {
			valueClauseEN := "the counted value is the span's pre-edge in-window projection"
			if basis == "host_wakeup_edge_pre_state" {
				valueClauseEN = "the counted value is the state-segment inventory's pre-edge share sum"
			}
			text = "edge-anchored (host→target): this seat rides the chain tier on the HOST thread's own in-window typed wakeup edge toward the analysis target (edge=credential, pre-edge=effective, post-edge=released); " + valueClauseEN + detail
		}
		out = append(out, text)
	}
	// PARTSPLIT-1 (§29.150④, 2026-07-19): the R4-mirror-refused gated
	// composite seat's 分账 sub-line — self-contained composer (typed
	// admission + µs identity re-validation live in
	// answer_document_mutation_runtime_partsplit.go).
	if text, ok := runtimeTraceProjGatedCompositeEdgeShareTagText(row, zh); ok {
		out = append(out, text)
	}
	// RULER2-1 (§29.150②, 2026-07-19): the self runnable account two-ruler
	// cross-row sentence on the stamped LEAD seat row — self-contained
	// composer (typed admission + both same-ruler µs identity re-validations
	// live in answer_document_mutation_runtime_ruler2.go).
	if text, ok := runtimeTraceProjSelfRunnableTwoRulerTagText(row, zh); ok {
		out = append(out, text)
	}
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): the affinity/cpuset seat's
	// constraint DESCRIPTION — the judgment payload the engine decided on
	// (allowed set vs observed exclusion + group + basis kind), so the seat is
	// never a bare 「CPU亲和/cpuset限制」 assertion. Typed fields only; parts
	// absent from the payload are omitted (absence never guesses). Soft
	// descriptive note — no legend mark (完成闭合凭证 precedent).
	if row.Node.CPUConstraintKind != "" || len(row.Node.CPUConstraintAllowedCPUs) > 0 ||
		strings.TrimSpace(row.Node.CPUConstraintCPUSet) != "" {
		var parts []string
		if len(row.Node.CPUConstraintAllowedCPUs) > 0 {
			if zh {
				parts = append(parts, "允许核 "+runtimeTraceProjCPUListWord(row.Node.CPUConstraintAllowedCPUs))
				if len(row.Node.CPUConstraintExcludedCPUs) > 0 {
					parts = append(parts, "排除全域观测核 "+runtimeTraceProjCPUListWord(row.Node.CPUConstraintExcludedCPUs))
				} else {
					parts = append(parts, "未排除任何全域观测核")
				}
			} else {
				parts = append(parts, "allowed CPUs "+runtimeTraceProjCPUListWord(row.Node.CPUConstraintAllowedCPUs))
				if len(row.Node.CPUConstraintExcludedCPUs) > 0 {
					parts = append(parts, "excludes observed CPUs "+runtimeTraceProjCPUListWord(row.Node.CPUConstraintExcludedCPUs))
				} else {
					parts = append(parts, "excludes no observed CPU")
				}
			}
		}
		// R5a (§29.88.4 场景② 按核档, 2026-07-15) mention OBLIGATION: the
		// binding provably shut out a bigger core tier — the engine minted
		// the proof pair, the seat MUST say so (与「优化点无条件入正文」同族;
		// zero pair = negative arm, nothing renders — 禁无中生有).
		if row.Node.CPUConstraintAllowedMaxTierKHz > 0 && row.Node.CPUConstraintGlobalMaxTierKHz > 0 {
			// Catalog B11 (DISPLAY-HYG 二轮, §29.104.18.1, 2026-07-17): the
			// tier pair speaks GHz like every other frequency face (the
			// supply-fold clause's %.2fGHz ÷1e6 convention) — this was the
			// report's only raw %dkHz emission, and the naked `<` gains its
			// spaces. Same typed pair, display conversion only.
			//
			// 双单位形注记 (复核件4①, 2026-07-17, 暂不改): the SAME tier
			// values stay raw kHz on their other faces — the trace_query
			// text/k=v rows (freq=/weighted_freq=/observed_max_freq=%dkHz),
			// the cpu_constraint_* typed note keys (R2' wire lane, 零动),
			// the engine Summary vocabulary, and this row's own detail-block
			// mirror of the wire fields. Display GHz is the READER face
			// only; converging the four faces is a wire-key ruling
			// (RANKDIS-M18 键改名先例), never a display-hygiene edit.
			if zh {
				parts = append(parts, fmt.Sprintf("绑核排除更大核档(允许核最高档 %.2fGHz < 全域最大核档 %.2fGHz)",
					float64(row.Node.CPUConstraintAllowedMaxTierKHz)/1e6, float64(row.Node.CPUConstraintGlobalMaxTierKHz)/1e6))
			} else {
				parts = append(parts, fmt.Sprintf("binding excludes a bigger core tier (allowed max tier %.2fGHz < global max tier %.2fGHz)",
					float64(row.Node.CPUConstraintAllowedMaxTierKHz)/1e6, float64(row.Node.CPUConstraintGlobalMaxTierKHz)/1e6))
			}
		}
		if set := strings.TrimSpace(row.Node.CPUConstraintCPUSet); set != "" {
			if zh {
				parts = append(parts, "cpuset组 "+set)
			} else {
				parts = append(parts, "cpuset group "+set)
			}
		}
		if strings.Contains(row.Node.CPUConstraintPolicy, "restricted=true") {
			if zh {
				parts = append(parts, "策略 restricted=true")
			} else {
				parts = append(parts, "policy restricted=true")
			}
		}
		if kind := strings.TrimSpace(row.Node.CPUConstraintKind); kind != "" {
			if zh {
				parts = append(parts, "判定依据 "+kind)
			} else {
				parts = append(parts, "basis "+kind)
			}
		}
		if len(parts) > 0 {
			head := "CPU约束描述:"
			if !zh {
				head = "CPU-constraint description: "
			}
			out = append(out, head+strings.Join(parts, " · "))
		}
	}
	// WO-C1 (SMR-1 批, 2026-07-12): the account-relation sentence — 口径自述 +
	// 互指 only; 禁「同段」字面, 禁覆盖方向暗示, 禁量化重叠 ms (S6 vnote 三禁令).
	if row.AccountRelRef != "" {
		if row.AccountRelSameSourceFullMS > 0 {
			// RSPA §29.61.10b (2026-07-14): the same-source seat pair speaks
			// the bipartition relation (the ONLY additive seat relation —
			// anchored + remainder == full exactly); the generic two-accounts
			// template below would falsely claim 不可相加 on this pair.
			row.marks.mark(runtimeTraceProjMarkChainAnchorRelation)
			// DISPLAY-WRAP 件③(b) (§29.104.18.1 B3, 2026-07-16): the rule
			// clauses (锚定席=凭证锚定段合计 / 余段席=窗内其余段无链上凭证 /
			// 两席同源不相交) live in the legend's 同源二分 + 合计还原全窗账
			// entries — the row keeps its OWN facts only: the seat-role
			// assignment, the [E#] cross-pointer and the restored full-window
			// value (witness: the three-line rule explanation reprinted ×5).
			var text string
			if row.AccountRelSameSourceAnchoredSide {
				text = fmt.Sprintf("同源二分对席:◇席=[%s],⛓席=本行;合计还原全窗账 %.3fms(规则见图例)",
					row.AccountRelRef, row.AccountRelSameSourceFullMS)
				if !zh {
					text = fmt.Sprintf("same-source split pair: ◇ seat = [%s], ⛓ seat = this row; their sum restores the full-window account %.3fms (rule in the legend)",
						row.AccountRelRef, row.AccountRelSameSourceFullMS)
				}
			} else {
				text = fmt.Sprintf("同源二分对席:⛓席=[%s],◇席=本行;合计还原全窗账 %.3fms(规则见图例)",
					row.AccountRelRef, row.AccountRelSameSourceFullMS)
				if !zh {
					text = fmt.Sprintf("same-source split pair: ⛓ seat = [%s], ◇ seat = this row; their sum restores the full-window account %.3fms (rule in the legend)",
						row.AccountRelRef, row.AccountRelSameSourceFullMS)
				}
			}
			out = append(out, text)
		} else {
			row.marks.mark(runtimeTraceProjMarkAccountRelation)
			// 修复轮三 R2-F2: the overlap/disjoint claim is DERIVED (typed hull
			// disjointness), never an unconditional template; the disjoint form
			// adds no additive invitation (账目自识别句保留 — 跨账目体系与其它行
			// 仍有双计面).
			// DISPLAY-WRAP 件③(b) (§29.104.18.1 B3, 2026-07-16): the rule
			// half-sentence 「两套账目覆盖集不同」 lives in the legend's
			// 账目关系 entry — the row keeps its own facts (the [E#] pair,
			// the typed overlap/disjoint verdict, the two accounts'
			// self-descriptions) behind the legend-keyed chip word.
			text := "与[" + row.AccountRelRef + "]同线程同状态族·物理时间重叠(不可相加)·账目关系(见图例):本行=" +
				row.AccountRelOwn + ",[" + row.AccountRelRef + "]=" + row.AccountRelPeer
			if row.AccountRelDisjoint {
				text = "与[" + row.AccountRelRef + "]同线程同状态族·物理时间不相交·账目关系(见图例):本行=" +
					row.AccountRelOwn + ",[" + row.AccountRelRef + "]=" + row.AccountRelPeer
			}
			if !zh {
				text = "same thread, same state family as [" + row.AccountRelRef + "] · physical time overlaps (never additive) · account relation (see legend): this row = " +
					row.AccountRelOwn + ", [" + row.AccountRelRef + "] = " + row.AccountRelPeer
				if row.AccountRelDisjoint {
					text = "same thread, same state family as [" + row.AccountRelRef + "] · physical time disjoint · account relation (see legend): this row = " +
						row.AccountRelOwn + ", [" + row.AccountRelRef + "] = " + row.AccountRelPeer
				}
			}
			out = append(out, text)
		}
	}
	// 件1 (修复轮, 2026-07-14): the full-window MIRROR row relation — the
	// undecomposed lane face of a bipartitioned account speaks the mirror
	// sentence (冷读给定句形); each half carries the back-pointer. Same
	// relation mark family as the bipartition sentence (one legend home).
	if row.AccountRelMirrorAnchoredRef != "" && row.AccountRelMirrorRemainderRef != "" {
		row.marks.mark(runtimeTraceProjMarkChainAnchorRelation)
		text := fmt.Sprintf("同段镜像·全窗账=[%s]+[%s] 二分席之和,不可与二分席相加",
			row.AccountRelMirrorAnchoredRef, row.AccountRelMirrorRemainderRef)
		if !zh {
			text = fmt.Sprintf("same-segment mirror · this full-window account = the sum of the split seats [%s]+[%s]; never add it to those seats",
				row.AccountRelMirrorAnchoredRef, row.AccountRelMirrorRemainderRef)
		}
		out = append(out, text)
	}
	if ref := strings.TrimSpace(row.AccountRelMirrorRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkChainAnchorRelation)
		text := "全窗账镜像行 [" + ref + "](另一车道面,不可相加)"
		if !zh {
			text = "full-window mirror row [" + ref + "] (another lane's face; never additive)"
		}
		out = append(out, text)
	}
	// SELF-LANE (§29.58.3 处置 b, 2026-07-13): the cross-channel same-thread
	// mutual pointers — ONE template each direction, typed refs stamped by
	// runtimeTraceProjMarkCrossChannelSameThread.
	if ref := strings.TrimSpace(row.CrossChannelChainRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkCrossChannelPointer)
		text := "本线程另有链上席 [" + ref + "]"
		if !zh {
			text = "this thread also holds an on-chain seat [" + ref + "]"
		}
		out = append(out, text)
	}
	if ref := strings.TrimSpace(row.CrossChannelAdjacentRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkCrossChannelPointer)
		text := "本线程另有邻近席 [" + ref + "]"
		if !zh {
			text = "this thread also holds an adjacent seat [" + ref + "]"
		}
		out = append(out, text)
	}
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): the pointer at a ⌗ side-rail
	// row speaks the 口径旁栏 word — the 邻近席 word claimed a channel seat
	// the self_caliber_side token retires.
	if ref := strings.TrimSpace(row.CrossChannelCaliberRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkCrossChannelPointer)
		text := "本线程另有口径旁栏行 [" + ref + "]"
		if !zh {
			text = "this thread also holds a caliber side-rail row [" + ref + "]"
		}
		out = append(out, text)
	}
	// XLANE-3 件3 (§29.104.2 定谳③, 2026-07-16): the cross-board same-thread
	// same-state-family mutual pointer — the peer board's seats are named,
	// values untouched, and the sentence forbids cross-board addition.
	if len(row.CrossBoardFamilyRefs) > 0 {
		row.marks.mark(runtimeTraceProjMarkCrossBoardFamilyNote)
		refs := make([]string, 0, len(row.CrossBoardFamilyRefs))
		for _, ref := range row.CrossBoardFamilyRefs {
			refs = append(refs, "["+ref+"]")
		}
		refText := strings.Join(refs, "、")
		if !zh {
			refText = strings.Join(refs, ", ")
		}
		if row.CrossBoardFamilyMoreCount > 0 {
			if zh {
				refText += fmt.Sprintf("等%d席", len(row.CrossBoardFamilyRefs)+row.CrossBoardFamilyMoreCount)
			} else {
				refText += fmt.Sprintf(" and %d more seats", row.CrossBoardFamilyMoreCount)
			}
		}
		boardText := strings.Join(row.CrossBoardFamilyPeerBoards, "、")
		if !zh {
			boardText = strings.Join(row.CrossBoardFamilyPeerBoards, ", ")
		}
		var text string
		if zh {
			text = "同线程同状态族账另见另板席 " + refText + "(板锚 " + boardText + ";各板独立成账、口径各异,不可跨板相加)"
		} else {
			text = "this thread's same state family also holds cross-board seats " + refText +
				" (board " + boardText + "; boards keep independent accounts and calibers — never add across boards)"
		}
		out = append(out, text)
	}
	// 修补轮 件F (2026-07-16): the micro fold's representative cross-board
	// note — folded members were mutual-pointed by another board's seats; one
	// sentence stands for them (their [E#] stay resolvable via the fold
	// bracket). Same legend home as the per-row sentence above.
	if len(row.MicroAnchorFoldCrossBoardPeerBoards) > 0 {
		row.marks.mark(runtimeTraceProjMarkCrossBoardFamilyNote)
		boardText := strings.Join(row.MicroAnchorFoldCrossBoardPeerBoards, "、")
		text := "本折叠行内成员被另板席互指(板锚 " + boardText + ";各板独立成账、口径各异,不可跨板相加)"
		if !zh {
			boardText = strings.Join(row.MicroAnchorFoldCrossBoardPeerBoards, ", ")
			text = "members inside this fold are mutual-pointed by another board's seats (board " + boardText +
				"; boards keep independent accounts and calibers — never add across boards)"
		}
		out = append(out, text)
	}
	// XERR1-FIX 件3 (§29.104.4 ③): the budget-sanity ⚠ disclosure — the row's
	// span-envelope claim exceeds the waiter's own non-running total over the
	// same span∩window interval (F-2 same-basis verdict minted engine-side;
	// 值+预算随行, no clamp). On a holder-subject rank record the budget
	// describes the WAITER (the record's peer), and the sentence says so.
	if row.Node.BlockingWaitBudgetExceeded {
		envelope := row.Node.BlockingSpanEnvelopeMS
		if envelope <= 0 {
			// LEGACY wire only (pre-XERR1-EXT payload-typed records): those
			// rows kept the envelope AS the published value, so the fallback
			// figure is correct there. New records always carry the typed
			// envelope beside the budget trio (twin port / rich notes).
			envelope = runtimeTraceProjNodeDisplayImpact(row.Node)
		}
		text := fmt.Sprintf("⚠ span 包络 %.3fms > 窗内非 running %.3fms:含 running %.3fms,非阻塞等待段",
			envelope, row.Node.BlockingWaitBudgetNonRunningMS, row.Node.BlockingWaitBudgetRunningMS)
		if !zh {
			text = fmt.Sprintf("⚠ span envelope %.3fms > in-window non-running %.3fms: contains running %.3fms, not blocking-wait segments",
				envelope, row.Node.BlockingWaitBudgetNonRunningMS, row.Node.BlockingWaitBudgetRunningMS)
		}
		if row.Node.BlockingSubjectIsHolder {
			// 件G② 复核修 (2026-07-16): per-lane annotation — the zh suffix
			// must never leak onto the EN face (双词面各说各).
			if zh {
				text += "(预算主体=等待方)"
			} else {
				text += " (budget subject = the waiter)"
			}
		}
		out = append(out, text)
	}
	// XERR1-FIX 件1 互指 (E6/E7 账目关系先例): the converged blocking row and
	// the thread's own sleep seat share physical sleep time — mutual pointers,
	// never addition.
	if ref := strings.TrimSpace(row.BlockingWaitSleepRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkBlockingWaitSleepRelation)
		text := "等待段含 sleep 分量,与[" + ref + "]自身 sleep 席同段物理时间(两账口径不同,不可相加)"
		if !zh {
			text = "the wait segments include a sleep share physically inside [" + ref + "]'s own sleep seat (two calibers, never additive)"
		}
		out = append(out, text)
	}
	if ref := strings.TrimSpace(row.BlockingWaitSleepPeerRef); ref != "" {
		row.marks.mark(runtimeTraceProjMarkBlockingWaitSleepRelation)
		text := "阻塞等待行[" + ref + "]的等待段落在本席同段物理时间(不可相加)"
		if !zh {
			text = "the blocking-wait row [" + ref + "]'s wait segments fall inside this seat's physical time (never additive)"
		}
		out = append(out, text)
	}
	// RSPA M-IO §29.61.10c (2026-07-14): the io_latency row whose completion
	// thread performed the wakeup that ended an anchored D/IO wait of a chain
	// thread wears the typed per-IO credential as a small demoted context word
	// (soft note, no legend mark — the 等待对象 precedent).
	if row.Node.ResourceCompletionClosure {
		if zh {
			out = append(out, "完成闭合凭证")
		} else {
			out = append(out, "completion-closure credential")
		}
	}
	// WO-B1 (SMR-1 批, 2026-07-12): the occurrence-series short note — the
	// interval identifies the occurrence (preferred over 第N次 ordinals, S10
	// vnote), the sibling refs cross-link, and the series total rides the
	// pointer word itself (§29.50.4③; 禁虚指 E# — no clean total seat exists).
	if len(row.OccurrenceSeriesRefs) > 0 && row.Node.StartTs > 0 && row.Node.EndTs > row.Node.StartTs {
		row.marks.mark(runtimeTraceProjMarkOccurrenceSeries)
		refs := strings.Join(row.OccurrenceSeriesRefs, "]+[")
		text := fmt.Sprintf("发生段 %.6fs~%.6fs·与[%s]不相交(共%d段,合计 %.3fms)",
			row.Node.StartTs, row.Node.EndTs, refs, row.OccurrenceSeriesCount, row.OccurrenceSeriesTotalMS)
		if !zh {
			text = fmt.Sprintf("occurrence %.6fs~%.6fs · disjoint from [%s] (%d segments, series total %.3fms)",
				row.Node.StartTs, row.Node.EndTs, refs, row.OccurrenceSeriesCount, row.OccurrenceSeriesTotalMS)
		}
		out = append(out, text)
	}
	return out
}

// runtimeTraceProjMarkFamilyMirrorTwins is the CR-2 组② P5 family arm (F-1
// 残口移交, ledger §29.49; witness donghu 20260712-133933 E8/E9): a MERGED
// critical_blocking row whose fingerprint — canonical subject + member count +
// µs-equal total — matches exactly one family row carrying CAL-1 segment truth
// is the same physical segment set on a second lane. The twin row gains the
// typed mirror mark and the family's TRUE single-segment extrema (段 inventory
// 传播到该 lane), so its detail range can stop claiming the per-CPU group sums
// as 「单次」 segments. Marked ONLY when the family extrema actually differ
// from the twin's merged range (equal extrema = single-segment groups, the
// legacy wording is already true — zero touch). Display wording only.
func runtimeTraceProjMarkFamilyMirrorTwins(model *runtimeTraceProjTreeModel) {
	type famRef struct {
		ref              string
		minMS, maxMS     float64
		count            int
		total            float64
		winStart, winEnd float64
	}
	famsBySubject := map[string][]famRef{}
	lanes := [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background}
	for _, rows := range lanes {
		for i := range rows {
			node := rows[i].Node
			if !rows[i].HasData || node.FamilyMemberCount <= 1 ||
				node.FamilyMemberMinMS <= 0 || node.FamilyMemberMaxMS <= 0 {
				continue
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
			if subject == "" {
				continue
			}
			famsBySubject[subject] = append(famsBySubject[subject], famRef{
				ref:   strings.TrimSpace(rows[i].EvidenceTag),
				minMS: node.FamilyMemberMinMS, maxMS: node.FamilyMemberMaxMS,
				count:    node.FamilyMemberCount,
				total:    runtimeTraceProjNodeDisplayImpact(node),
				winStart: node.QueryWindowStartTs, winEnd: node.QueryWindowEndTs,
			})
		}
	}
	if len(famsBySubject) == 0 {
		return
	}
	for _, rows := range lanes {
		for i := range rows {
			node := rows[i].Node
			if !rows[i].HasData || strings.TrimSpace(node.Predicate) != "critical_blocking" ||
				node.MergedCount <= 1 {
				continue
			}
			var match *famRef
			ambiguous := false
			for j, fam := range famsBySubject[runtimeTraceCausalProjectionCanonicalNode(node.Subject)] {
				if fam.count != node.MergedCount ||
					!runtimeTraceProjRound3Equal(fam.total, runtimeTraceProjNodeDisplayImpact(node)) {
					continue
				}
				if match != nil {
					ambiguous = true
					break
				}
				match = &famsBySubject[runtimeTraceCausalProjectionCanonicalNode(node.Subject)][j]
			}
			if match == nil || ambiguous {
				continue // fail-open: no fingerprint, or two candidate sources
			}
			// Equal extrema = the groups ARE single segments; the legacy
			// per-instance wording is already true — zero touch.
			if runtimeTraceProjRound3Equal(match.minMS, node.MergedMinMS) &&
				runtimeTraceProjRound3Equal(match.maxMS, node.MergedMaxMS) {
				continue
			}
			// 修复轮 R-P3-3: cross-window re-measurements never mirror (the
			// same veto every same-segment fold carries — SFD F1 family).
			if match.winStart > 0 && match.winEnd > match.winStart &&
				node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs &&
				(math.Abs(match.winStart-node.QueryWindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
					math.Abs(match.winEnd-node.QueryWindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS) {
				continue
			}
			rows[i].FamilyMirrorRef = match.ref
			rows[i].FamilyMirrorSegMin = match.minMS
			rows[i].FamilyMirrorSegMax = match.maxMS
		}
	}
}

// runtimeTraceProjMarkValueMirrorTwins is the 修复轮 C-2/A1 value-mirror arm
// (冷读 tieba E6/E18, 2026-07-12): an un-merged AGGREGATE-lane row (predicate
// wakeup_causal_aggregate) whose fingerprint — canonical subject + state +
// µs-equal display AND cumulative values + same typed query window — matches
// exactly ONE ×N merged candidate row is the same physical time published on
// a second lane (the witness pair shared five segments to the µs). The
// aggregate row wears the typed mirror tag pointing at the candidate's E#;
// both rows keep their own values and accountings (the two eff calibers are
// deliberately different rulers — the tag exists to kill the ADDITIVE
// reading, never to merge accounts). Ambiguity (0 or ≥2 candidates) and any
// fingerprint miss fail open to the two-row render.
func runtimeTraceProjMarkValueMirrorTwins(model *runtimeTraceProjTreeModel) {
	fingerprint := func(node types.TraceCausalProjectionNode) string {
		subject := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if subject == "" {
			return ""
		}
		display := runtimeTraceProjNodeDisplayImpact(node)
		if display <= 0 {
			return ""
		}
		return subject + "\x00" + strings.TrimSpace(node.StateKind) + "\x00" +
			fmt.Sprintf("%.3f|%.3f|%.3f..%.3f", display, node.CumulativeImpactMS,
				node.QueryWindowStartTs, node.QueryWindowEndTs)
	}
	lanes := [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background}
	candidates := map[string][]string{}
	for _, rows := range lanes {
		for i := range rows {
			node := rows[i].Node
			if !rows[i].HasData || node.MergedCount <= 1 || node.OnChainOverflowFold {
				continue
			}
			if key := fingerprint(node); key != "" {
				candidates[key] = append(candidates[key], strings.TrimSpace(rows[i].EvidenceTag))
			}
		}
	}
	if len(candidates) == 0 {
		return
	}
	for _, rows := range lanes {
		for i := range rows {
			node := rows[i].Node
			if !rows[i].HasData || node.MergedCount > 1 ||
				strings.TrimSpace(node.Predicate) != "wakeup_causal_aggregate" {
				continue
			}
			key := fingerprint(node)
			if key == "" {
				continue
			}
			refs := candidates[key]
			if len(refs) != 1 || refs[0] == "" {
				continue // ambiguity fails open
			}
			rows[i].ValueMirrorRef = refs[0]
		}
	}
}

// runtimeTraceProjRankFoldRankArm / ChainArm classify the two lanes of one
// segment. Precise producer signals only:
//   - rank arm  = root_cause_* funnel predicate + a published engine rank;
//   - chain arm = wakeup_causal_impact hop predicate + on_chain relevance.
//
// Shared arm requirements (双臂, SFD 复核 F2 mirror): the row IS an inversion
// candidate (typed flag / exact Object token), is NOT a ×N aggregate
// (MergedCount>1 sums/envelopes many segments) and NOT a periodic source
// (the VS-1 discount lane owns those rows' semantics).
func runtimeTraceProjRankFoldArmEligible(node types.TraceCausalProjectionNode) bool {
	return !node.IsContextOnlyRow() && runtimeTraceCausalProjectionInversionRow(node) &&
		node.MergedCount <= 1 && !node.PeriodicSource
}

func runtimeTraceProjRankFoldRankArm(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceProjRankFoldArmEligible(node) && node.Rank > 0 &&
		strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_")
}

func runtimeTraceProjRankFoldChainArm(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceProjRankFoldArmEligible(node) &&
		strings.HasPrefix(strings.TrimSpace(node.Predicate), "wakeup_causal_impact") &&
		strings.TrimSpace(node.ChainRelevance) == "on_chain"
}

// runtimeTraceProjFoldSameSegmentLaneTwins folds the rank-lane twin of a
// same-segment pair into its chain-lane row (RNB R2). Runs on the chain-node
// universe BEFORE the subject buckets are built (NEW-3 position), so the rank
// twin never mints a sibling/cause tree row; the returned peer map (keyed by
// the KEPT node's key) is re-attached to the surviving row after flatten.
//
// Precision rules (硬边界, fail-open to the two-row render on every miss):
//   - join key = canonical subject + exact line range (SFD 同款, above);
//   - exactly ONE rank arm and ONE chain arm under a key — any other shape
//     (two rank views, two chain views) is ambiguity and never folds;
//   - effective mirror equality: both lanes carry the engine's ONE
//     rank-lane gated effective (R5d single source mirrored onto the hop
//     row); a differing effective is a different accounting — the pre-P0-E
//     raw-vs-gated twin shape (§15.B RCX² 退档: q6 E4 58.919 vs E7 37.410)
//     stays two rows, engine 根治 owns it. This is also the SFD F3/F4
//     conflict rule on this lane: the fold transfers ANNOTATION only (type
//     word/rank/confidence/E#) — never an ms account, never a fold group;
//   - cross-window veto (SFD F1 mirror): both arms declaring their own typed
//     selected_window with any endpoint beyond the F-2 ±1ms tolerance were
//     measured in DIFFERENT query windows and never fold.
//
// The kept chain node adopts the rank (typed transfer, display model only —
// the ❶❷❸ badge lane and the fold note read it; projection buckets, the rank
// funnel and every EffectiveImpactMs consumer are untouched). Coverage
// invariance rides the peer carriers (see runtimeTraceProjRankFoldPeer).
func runtimeTraceProjFoldSameSegmentLaneTwins(nodes []types.TraceCausalProjectionNode) ([]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	type group struct {
		rankIdx  []int
		chainIdx []int
	}
	groups := map[string]*group{}
	for i, node := range nodes {
		rankArm := runtimeTraceProjRankFoldRankArm(node)
		chainArm := runtimeTraceProjRankFoldChainArm(node)
		if !rankArm && !chainArm {
			continue
		}
		key := runtimeTraceProjSameSegmentTwinKey(node)
		if key == "" {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
		}
		if rankArm {
			g.rankIdx = append(g.rankIdx, i)
		} else {
			g.chainIdx = append(g.chainIdx, i)
		}
	}
	foldInto := map[int]int{} // rank node index -> chain node index
	for _, g := range groups {
		if len(g.rankIdx) != 1 || len(g.chainIdx) != 1 {
			continue // ambiguity fails open (SFD donor-conflict rule)
		}
		rank, chain := nodes[g.rankIdx[0]], nodes[g.chainIdx[0]]
		if rank.EffectiveImpactMS <= 0 || rank.EffectiveImpactMS != chain.EffectiveImpactMS {
			continue // not the engine's same-segment mirror — never fold
		}
		// 复核 W-A (RNB 收尾 2026-07-07): the mirror guard's SECOND equality —
		// two lanes whose CUMULATIVE accounts disagree are different-scope
		// accountings of the segment (cmp_01 E7/E8 实证: rank cum 47.503 counts
		// the enclosing chain scope while the hop row's own cum is 28.230) and
		// never fold ("不同账目绝不折", same standard as the effective arm).
		// Without this arm the folded rank cum could enter a trunk Chain row's
		// depth-MAX via the peer carriers while pre-fold it was a Cause row
		// that never competed (coverage-numerator invariance falsified on the
		// constructed 9.999-vs-4.115 shape). Fail-open: the two-row render
		// stays; the fold witnesses with agreeing accounts (huadong E4/E5
		// 4.115==4.115, E11/E13 2.770==2.770, opendir E6/E7 58.919==58.919)
		// are untouched.
		if rank.CumulativeImpactMS > 0 && chain.CumulativeImpactMS > 0 &&
			rank.CumulativeImpactMS != chain.CumulativeImpactMS {
			continue // diverging cumulative accounts (W-A) — never fold
		}
		if rank.QueryWindowStartTs > 0 && rank.QueryWindowEndTs > rank.QueryWindowStartTs &&
			chain.QueryWindowStartTs > 0 && chain.QueryWindowEndTs > chain.QueryWindowStartTs &&
			(math.Abs(rank.QueryWindowStartTs-chain.QueryWindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
				math.Abs(rank.QueryWindowEndTs-chain.QueryWindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS) {
			continue // cross-window re-measurement (SFD F1 mirror) — never fold
		}
		foldInto[g.rankIdx[0]] = g.chainIdx[0]
	}
	if len(foldInto) == 0 {
		return nodes, nil
	}
	dropped := map[int]bool{}
	rankByChainIdx := map[int]int{}
	for rankIdx, chainIdx := range foldInto {
		dropped[rankIdx] = true
		rankByChainIdx[chainIdx] = rankIdx
	}
	kept := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	peers := map[string][]types.TraceCausalProjectionNode{}
	for i, node := range nodes {
		if dropped[i] {
			continue
		}
		if rankIdx, ok := rankByChainIdx[i]; ok {
			if node.Rank <= 0 {
				node.Rank = nodes[rankIdx].Rank
				// SYM (§24.13 裁定一): the typed tier travels WITH the rank it
				// annotates — a folded self-state rank twin must not launder
				// its board exclusion through the fold (the kept chain node
				// would otherwise wear Rank>0 with no tier identity).
				if strings.TrimSpace(node.Tier) == "" {
					node.Tier = nodes[rankIdx].Tier
				}
				// DISPLAY-WRAP 件② (§29.104.18.1 B1 修根, 2026-07-16): the
				// ordinal's BOARD identity travels WITH the adopted seat — the
				// XLANE-3 件1 aggregate-merge discipline, previously missing on
				// THIS display-level fold. A fold host wearing Rank>0 with an
				// empty RankBoardTarget minted a phantom UNNAMED board in the
				// chip census, flipping a SINGLE-board single-window report
				// into multi-board mode: every named seat then wore the
				// zero-disambiguation 窗…·板锚 chip 39/38 times (donghu 17267
				// witness). Empty-slot caller only; a host that already
				// carries its own board identity keeps it untouched.
				if strings.TrimSpace(node.RankBoardTarget) == "" {
					node.RankBoardTarget = nodes[rankIdx].RankBoardTarget
				}
				if strings.TrimSpace(node.RankBoardParamsFingerprint) == "" {
					node.RankBoardParamsFingerprint = nodes[rankIdx].RankBoardParamsFingerprint
				}
			}
			// §29.50.5 (v5 P1 批 件②, 2026-07-13): the folded rank twin's
			// typed D/IO proof family travels with the surviving chain node —
			// one physical set of segments, one proof (the P2-3 propagation
			// only syncs rows that BOTH still render; the fold survivor must
			// not lose the 等待对象 / 原因未证 word inputs). OR-monotone
			// booleans, empty-slot caller, residual pair moves as a pair.
			rankTwin := nodes[rankIdx]
			node.DStateRefinedNonIO = node.DStateRefinedNonIO || rankTwin.DStateRefinedNonIO
			node.DStateCauseUnprovenRemainder = node.DStateCauseUnprovenRemainder || rankTwin.DStateCauseUnprovenRemainder
			if node.BlockedReasonCaller == "" {
				node.BlockedReasonCaller = rankTwin.BlockedReasonCaller
			}
			if node.BlockedReasonWindowCount == 0 && rankTwin.BlockedReasonWindowCount > 0 {
				node.BlockedReasonWindowCount = rankTwin.BlockedReasonWindowCount
				node.BlockedReasonWindowCaller = rankTwin.BlockedReasonWindowCaller
			}
			peers[runtimeTraceCausalProjectionNodeKey(node)] = append(
				peers[runtimeTraceCausalProjectionNodeKey(node)], nodes[rankIdx])
		}
		kept = append(kept, node)
	}
	return kept, peers
}

// PTV8-RCR-A (§24 ③裁定/§24.2, 2026-07-08). EVOLUTION RECORD: the R2 fold
// note renderer runtimeTraceProjRankFoldNoteText (同段rank行并入: …) is
// RETIRED — the folded rank row's rank/confidence ride the cause node's 行2
// and its E# merges into 行1's bracket; the detail block carries the 根因排序
// line. The join/guard engine above is untouched.

// --- SEM-LEAD semantic-family two-lane fold (§29.7-2 ③, ledger
// real_trace_campaign_20260705.md, 2026-07-10) --------------------------------
//
// The engine publishes ONE semantic span entity through TWO
// observation channels — the trace_semantic_span typed channel (the ✦ 语义
// lane row: class word, family roster, caliber) and the root_cause_* rank
// funnel (rank ordinal, tier, effective attribution) — and the display seated
// BOTH (792-textup witness: E9 「✦ 纹理上传 ×11 … [E9]」 + E13
// 「链上·父节点未确认 ❶⚙ Texture upload(15573)… ×11 [E13]」, one 11-span family
// on two E# seats). The fold keeps the SEMANTIC row (✦ 词位 = 类名, roster,
// caliber word — §29.7-2 ④ 行1 类名) and folds the rank row into it: the rank
// ordinal/tier transfer onto the node, the rank row's E# rides the shared
// RankFoldPeers carrier (行1 [E#+E#] bracket, detail 根因排序 line,
// bar-scale/disclosure MAX invariance — the RNB R2 vocabulary verbatim);
// the carrier's TypeWord field is stored but word-face-less today (§29.40
// OM-6 ruling: field kept, 词面臂 pends v5 P2c).
//
// B9 production witness (cust_trace_vc_710.txt, 2026-07-10) widened the
// relevance scope: the same off-chain class_verification family occupied a
// `├─语义─` seat and a `◇邻近区段` seat. Matching now admits on_chain,
// adjacent, and background pairs, but the relevance lane is part of the exact
// join key and MUST agree on both arms. The surviving semantic row retains the
// off-chain BackgroundRank; no off-chain row gains an on-chain board seat.
//
// Precision rules (硬边界, fail-open to the two-row render on every miss):
//   - join key = canonical subject + typed SemanticClass token + the exact
//     evidence line range (both lanes carry the engine's OWN verbatim family
//     envelope / span lines — never a name or substring; 语义类身份必须
//     typed token);
//   - exactly ONE rank arm and ONE semantic arm under a key — any other
//     shape is ambiguity and never folds;
//   - value mirror equality: both lanes carry the engine's ONE participation
//     value. On-chain semantic rows mirror rank participation (the exact
//     member∩chain intersection, §29.7-2 ② + the SEM-LEAD intersection
//     caliber) against the semantic record's typed SemanticChainProjectedMS —
//     the semantic DISPLAY value stays the complete member union (§24.10
//     lossless observation) and must NOT be the mirror (审计 #5, §29.25/
//     §29.26: intersection<union on every partial-overlap family re-opened
//     the twin seats). Rows without the typed intersection keep the legacy
//     display-impact mirror; a differing value is a different accounting and
//     never folds;
//   - family mirror: both lanes carry the same typed member count;
//   - cross-window veto (SFD F1 mirror): both arms declaring their own typed
//     selected_window beyond the ±1ms tolerance never fold.
func runtimeTraceProjSemanticRankTwinArm(node types.TraceCausalProjectionNode) bool {
	_, laneOK := runtimeTraceProjSemanticTwinLane(node)
	return !node.IsContextOnlyRow() && laneOK && strings.TrimSpace(node.SemanticClass) != "" &&
		node.Rank > 0 &&
		strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_")
}

func runtimeTraceProjSemanticLaneTwinArm(node types.TraceCausalProjectionNode) bool {
	_, laneOK := runtimeTraceProjSemanticTwinLane(node)
	return laneOK && runtimeTraceCausalProjectionSemanticSpanRow(node) &&
		strings.TrimSpace(node.SemanticClass) != ""
}

func runtimeTraceProjSemanticTwinLane(node types.TraceCausalProjectionNode) (string, bool) {
	lane := strings.TrimSpace(node.ChainRelevance)
	if lane == "" {
		switch strings.TrimSpace(node.Causality) {
		// SELF-SEM (§29.61.1) / SELF-ALL (§29.61.2): the self tokens denote
		// on-chain membership.
		case "on_wakeup_chain", "on_dependency_chain", "self_deterministic", "self_wall_clock":
			lane = "on_chain"
		case "adjacent_to_wakeup_chain", "adjacent_to_dependency_chain":
			lane = "adjacent"
		case "background", "off_chain":
			lane = "background"
		}
	}
	switch lane {
	case "on_chain", "adjacent", "background":
		return lane, true
	default:
		return "", false
	}
}

func runtimeTraceProjSemanticTwinKey(node types.TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	if !types.TraceCausalProjectionKnownSubject(node.Subject) {
		return ""
	}
	lane, ok := runtimeTraceProjSemanticTwinLane(node)
	if !ok {
		return ""
	}
	return lane + "\x00" + runtimeTraceCausalProjectionCanonicalNode(node.Subject) +
		"\x00" + strings.TrimSpace(node.SemanticClass) +
		"\x00" + strconv.Itoa(node.LineStart) + "\x00" + strconv.Itoa(node.LineEnd)
}

// runtimeTraceProjFoldSemanticRankLaneTwinsDetailed folds one precisely
// matched rank-lane twin into its trace_semantic_span entity (SEM-LEAD
// §29.7-2 ③ + B9). The lane is part of the exact key: on-chain survivors
// adopt Rank/Tier and remain board candidates; adjacent/background survivors
// retain BackgroundRank and never gain an on-chain ordinal. The returned peer
// map (keyed by the kept semantic entity) preserves the rank E#/confidence and
// accounting invariance carriers for the single rendered seat.
//
// 复核 P3-1 (theoretical form, recorded): a FAIL-OPEN remnant rank twin (any
// guard miss — value/member/window/ambiguity) stays in the chain universe and
// may be CONSUMED by the trunk main/extra selection when the trunk-domain
// gate admits it (same branch + same window) — it then renders as the trunk
// main row or a ├─成因─ child instead of the witness's depthless seat, i.e.
// the two-seat fail-open shape can also appear as trunk-row + ✦-row. Both
// rows stay honest (each publishes its own account; the 行1 class-word arm in
// runtimeTraceProjRowNameBase covers the family form on every seat kind), so
// the remnant is a display redundancy, never a double-count. No production
// witness — the folded pairs are removed before display bucketing by
// construction.
func runtimeTraceProjFoldSemanticRankLaneTwinsDetailed(rankNodes []types.TraceCausalProjectionNode,
	semantics []types.TraceCausalProjectionNode) ([]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode, map[int]bool) {
	type group struct {
		rankIdx []int
		semIdx  []int
	}
	groups := map[string]*group{}
	for i, node := range rankNodes {
		if !runtimeTraceProjSemanticRankTwinArm(node) {
			continue
		}
		key := runtimeTraceProjSemanticTwinKey(node)
		if key == "" {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
		}
		g.rankIdx = append(g.rankIdx, i)
	}
	if len(groups) == 0 {
		return semantics, nil, nil
	}
	for i, node := range semantics {
		if !runtimeTraceProjSemanticLaneTwinArm(node) {
			continue
		}
		key := runtimeTraceProjSemanticTwinKey(node)
		if key == "" {
			continue
		}
		if g := groups[key]; g != nil {
			g.semIdx = append(g.semIdx, i)
		}
	}
	dropped := map[int]bool{}
	peers := map[string][]types.TraceCausalProjectionNode{}
	folded := false
	for _, g := range groups {
		if len(g.rankIdx) != 1 || len(g.semIdx) != 1 {
			continue // ambiguity fails open (SFD donor-conflict rule)
		}
		rank := rankNodes[g.rankIdx[0]]
		sem := &semantics[g.semIdx[0]]
		if sem.SemanticChainProjectedMS > 0 {
			// 审计 #5 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10):
			// SAME-SOURCE value mirror for on-chain semantic twins. The rank
			// lane's participation value is the exact member∩chain intersection
			// (SEM-LEAD intersection caliber), while the semantic lane's DISPLAY
			// value stays the complete member union (§24.10 lossless
			// observation) — so a display-vs-display mirror is structurally
			// false whenever intersection < union (every partial-overlap
			// family/single span) and re-opened the E9/E13 twin seats with two
			// contradicting 有效归因 values. Mirror rank participation against
			// the semantic record's typed intersection instead: both arms carry
			// the engine's ONE participation value.
			participation := rank.EffectiveImpactMS
			if participation <= 0 {
				participation = runtimeTraceProjNodeDisplayImpact(rank)
			}
			if participation <= 0 || !runtimeTraceProjRound3Equal(participation, sem.SemanticChainProjectedMS) {
				continue // not the engine's one participation value — never fold
			}
		} else if rankV, semV := runtimeTraceProjNodeDisplayImpact(rank), runtimeTraceProjNodeDisplayImpact(*sem); rankV <= 0 || rankV != semV {
			continue // not the engine's one participation value — never fold
		}
		if rank.FamilyMemberCount != sem.FamilyMemberCount {
			continue // different member accounting — never fold
		}
		if rank.QueryWindowStartTs > 0 && rank.QueryWindowEndTs > rank.QueryWindowStartTs &&
			sem.QueryWindowStartTs > 0 && sem.QueryWindowEndTs > sem.QueryWindowStartTs &&
			(math.Abs(rank.QueryWindowStartTs-sem.QueryWindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
				math.Abs(rank.QueryWindowEndTs-sem.QueryWindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS) {
			continue // cross-window re-measurement (SFD F1 mirror) — never fold
		}
		// Typed identity transfer: the rank seat IS the semantic row's
		// participation identity (§24.7 呈现逻辑统一令: 成因行身份 = 根因排序
		// 参赛身份). Values are NOT transferred wholesale — the value mirror
		// above already proved both lanes publish the same participation
		// value; only absent-on-the-semantic-side account fields adopt.
		lane, _ := runtimeTraceProjSemanticTwinLane(*sem)
		// DISPLAY-WRAP 件② (§29.104.18.1 B1 修根, 2026-07-16): whenever the
		// rank ordinal transfers onto the surviving semantic row, its BOARD
		// identity travels with it (XLANE-3 件1 discipline — an ordinal
		// without its board mints a phantom unnamed board in the chip census
		// and flips a single-board report into multi-board chips). Empty-slot
		// caller; the background arm keeps Rank 0 and adopts no board claim.
		adoptBoard := func() {
			if strings.TrimSpace(sem.RankBoardTarget) == "" {
				sem.RankBoardTarget = rank.RankBoardTarget
			}
			if strings.TrimSpace(sem.RankBoardParamsFingerprint) == "" {
				sem.RankBoardParamsFingerprint = rank.RankBoardParamsFingerprint
			}
		}
		switch lane {
		case "on_chain":
			sem.Rank = rank.Rank
			adoptBoard()
		case "adjacent":
			// UXR-1 (§29.36.2, 2026-07-11): the adjacent twin's Rank is the
			// 邻近影响 channel's own ordinal — the survivor adopts it (the chip
			// printer words it per channel, never as an on-chain seat).
			// BackgroundRank transfers as CARRIER preservation only: the field
			// has no display-face reader (§29.40 W-2 exemption) — the §23.1
			// mention obligation rides the LLM-face background_rank= wire note
			// (internal/tool/trace_query.go), not this Node field.
			sem.Rank = rank.Rank
			adoptBoard()
			if rank.BackgroundRank > 0 {
				sem.BackgroundRank = rank.BackgroundRank
			}
		default:
			// Background rows carry no ordinal (通道3 无序数). The folded rank
			// record remains reachable through RankFoldPeers/E#; BackgroundRank
			// transfers as carrier preservation for the wire face — no display
			// consumer reads it (§29.40 W-2; the mention obligation is the
			// LLM-face background_rank= note, not this field).
			sem.Rank = 0
			if rank.BackgroundRank > 0 {
				sem.BackgroundRank = rank.BackgroundRank
			}
		}
		if strings.TrimSpace(sem.Tier) == "" {
			sem.Tier = rank.Tier
		}
		if sem.EffectiveImpactMS <= 0 {
			sem.EffectiveImpactMS = rank.EffectiveImpactMS
			// EPUB 复核 INFO: the published marker travels WITH the adopted
			// value (OR-monotone, same rule as the R1 merge arm) — this was
			// the last value/marker separation point.
			sem.EffectiveImpactPublished = sem.EffectiveImpactPublished || rank.EffectiveImpactPublished
		}
		if sem.CumulativeImpactMS <= 0 {
			sem.CumulativeImpactMS = rank.CumulativeImpactMS
		}
		if sem.ChainDepth <= 0 {
			sem.ChainDepth = rank.ChainDepth
		}
		if sem.Confidence <= 0 {
			sem.Confidence = rank.Confidence
		}
		dropped[g.rankIdx[0]] = true
		peers[runtimeTraceCausalProjectionNodeKey(*sem)] = append(
			peers[runtimeTraceCausalProjectionNodeKey(*sem)], rank)
		folded = true
	}
	if !folded {
		return semantics, nil, nil
	}
	return semantics, peers, dropped
}

// runtimeTraceProjFoldSemanticRankLaneTwins is the unit-grain wrapper used by
// the precision-guard tests and single-bucket callers.
func runtimeTraceProjFoldSemanticRankLaneTwins(rankNodes []types.TraceCausalProjectionNode,
	semantics []types.TraceCausalProjectionNode) ([]types.TraceCausalProjectionNode, []types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	semantics, peers, dropped := runtimeTraceProjFoldSemanticRankLaneTwinsDetailed(rankNodes, semantics)
	if len(dropped) == 0 {
		return rankNodes, semantics, nil
	}
	kept := make([]types.TraceCausalProjectionNode, 0, len(rankNodes)-len(dropped))
	for i, node := range rankNodes {
		if !dropped[i] {
			kept = append(kept, node)
		}
	}
	return kept, semantics, peers
}

// runtimeTraceProjFoldSemanticRankLaneTwinsAcrossBuckets runs one ambiguity
// census across chain/adjacent/background rank populations, then removes each
// proven rank twin from its original bucket. Running one joined census is
// load-bearing: two rank arms under one exact key fail open even if they came
// from different projection buckets. A surviving on-chain semantic entity
// keeps its causal-tree seat; an off-chain entity keeps exactly one seat in
// its original ◇/▒ bucket (or is inserted there when that bucket's independent
// semantic copy was capped).
func runtimeTraceProjFoldSemanticRankLaneTwinsAcrossBuckets(
	chainNodes, adjacentNodes, backgroundNodes, semantics []types.TraceCausalProjectionNode,
) ([]types.TraceCausalProjectionNode, []types.TraceCausalProjectionNode, []types.TraceCausalProjectionNode,
	[]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	all := make([]types.TraceCausalProjectionNode, 0, len(chainNodes)+len(adjacentNodes)+len(backgroundNodes))
	all = append(all, chainNodes...)
	adjacentStart := len(all)
	all = append(all, adjacentNodes...)
	backgroundStart := len(all)
	all = append(all, backgroundNodes...)

	semantics, peers, dropped := runtimeTraceProjFoldSemanticRankLaneTwinsDetailed(all, semantics)
	if len(dropped) == 0 {
		return chainNodes, adjacentNodes, backgroundNodes, semantics, nil
	}
	foldedSemantic := map[string]types.TraceCausalProjectionNode{}
	for _, sem := range semantics {
		key := runtimeTraceCausalProjectionNodeKey(sem)
		if len(peers[key]) > 0 {
			foldedSemantic[key] = sem
		}
	}
	replacementByRank := map[string]types.TraceCausalProjectionNode{}
	for semKey, rankPeers := range peers {
		sem, ok := foldedSemantic[semKey]
		if !ok {
			continue
		}
		lane, _ := runtimeTraceProjSemanticTwinLane(sem)
		if lane == "on_chain" {
			continue
		}
		for _, rank := range rankPeers {
			replacementByRank[runtimeTraceCausalProjectionNodeKey(rank)] = sem
		}
	}
	filter := func(nodes []types.TraceCausalProjectionNode, offset int, offChainBucket bool) []types.TraceCausalProjectionNode {
		out := make([]types.TraceCausalProjectionNode, 0, len(nodes))
		semanticCopies := map[string]bool{}
		if offChainBucket {
			for _, node := range nodes {
				if runtimeTraceCausalProjectionSemanticSpanRow(node) {
					semanticCopies[runtimeTraceCausalProjectionNodeKey(node)] = true
				}
			}
		}
		for i, node := range nodes {
			if dropped[offset+i] {
				if replacement, ok := replacementByRank[runtimeTraceCausalProjectionNodeKey(node)]; ok &&
					!semanticCopies[runtimeTraceCausalProjectionNodeKey(replacement)] {
					out = append(out, replacement)
				}
				continue
			}
			if offChainBucket && runtimeTraceCausalProjectionSemanticSpanRow(node) {
				if replacement, ok := foldedSemantic[runtimeTraceCausalProjectionNodeKey(node)]; ok {
					node = replacement
				}
			}
			out = append(out, node)
		}
		return out
	}
	chainNodes = filter(chainNodes, 0, false)
	adjacentNodes = filter(adjacentNodes, adjacentStart, true)
	backgroundNodes = filter(backgroundNodes, backgroundStart, true)
	return chainNodes, adjacentNodes, backgroundNodes, semantics, peers
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
// background stanza already runs, then one duplicate-measurement fold — same
// canonical subject + same canonical object + same canonical TypeToken, the
// projected ms matching on the exact lane (pure float equality) or the PTV6-B
// near lane (bounded band + sentinel gates, both consumed from the types
// exports — see runtimeTraceProjSameAdjacentMeasurement), AND a precise
// line/time overlap (RF2a, adversarial review 2026-07-03) merge into the
// first row's DuplicatePublications/MergedEvidenceIDs. The real customer
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

// runtimeTraceProjSameAdjacentMeasurement is the duplicate-measurement
// identity of the display safety net: canonical subject + canonical object +
// canonical TypeToken equal, the positive projected ms matching on one of TWO
// value lanes, AND the two rows precisely overlap in the artifact — line-range
// intersection or time-span intersection (RF2a, adversarial review 2026-07-03:
// two REAL irq bursts at different moments can quantize to the same %.3f ms;
// folding them halves the reported contribution). When neither location lane
// is determinate for the pair the fold fails open to two rows — value
// proximity alone never merges.
//
// Value lanes (PTV6-B mirror, 2026-07-06 — the former exact-only fork with the
// types-layer V4 fold is gone):
//   - exact lane: pure float equality (pre-PTV6-B behavior, byte-identical);
//   - near lane: the bounded boundary-resampling band, consumed from the ONE
//     exported types authority (TraceCausalProjectionNearDuplicateValues —
//     the band constant is never copied), gated on REAL non-sentinel SUBJECT
//     and OBJECT identities (TraceCausalProjectionKnownSubject — a sentinel
//     on either leg carries no identity for the "one republished measurement"
//     assertion).
func runtimeTraceProjSameAdjacentMeasurement(a, b types.TraceCausalProjectionNode) bool {
	if runtimeTraceCausalProjectionCanonicalNode(a.Subject) != runtimeTraceCausalProjectionCanonicalNode(b.Subject) ||
		runtimeTraceCausalProjectionCanonicalNode(a.Object) != runtimeTraceCausalProjectionCanonicalNode(b.Object) ||
		runtimeTraceCausalProjectionCanonicalNode(a.TypeToken) != runtimeTraceCausalProjectionCanonicalNode(b.TypeToken) {
		return false
	}
	sameValue := a.ImpactMS > 0 && a.ImpactMS == b.ImpactMS
	// [Med 修正轮 2026-07-06] the sentinel gate covers BOTH identity legs
	// (types-layer near lane verbatim): an unknown-thread SUBJECT carries no
	// identity for the "one republished measurement" assertion either.
	nearValue := !sameValue && types.TraceCausalProjectionKnownSubject(a.Subject) &&
		types.TraceCausalProjectionKnownSubject(a.Object) &&
		types.TraceCausalProjectionNearDuplicateValues(a.ImpactMS, b.ImpactMS)
	return (sameValue || nearValue) &&
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
// the surviving first-occurrence row: publication count + evidence union, and
// on the PTV6-B near lane the member-MAX value (the rows measured the same
// amount; a sum would double-count the wall clock — max never invents a
// number). V4: writes the typed DuplicatePublications field shared with the
// aggregation-layer pass; the former MergedCount/MergedMin/Max writes are gone
// (those carry SUM-aggregate semantics), and the former subject-roster append
// was dead code — the fold identity requires equal canonical subjects.
func runtimeTraceProjAbsorbAdjacentDuplicate(survivor *types.TraceCausalProjectionNode, dup types.TraceCausalProjectionNode) {
	// Near lane only (PTV6-B mirror; the types-layer absorb's rule verbatim):
	// when the two publications' values differ — inside the ≤3% band, or the
	// identity would not have matched — the fold keeps the LARGEST boundary
	// estimate of the one fact (显示取最大, ×N同值 legend wording). The exact
	// lane takes neither branch and stays byte-identical to pre-PTV6-B.
	if dup.ImpactMS != survivor.ImpactMS {
		if dup.ImpactMS > survivor.ImpactMS {
			survivor.ImpactMS = dup.ImpactMS
		}
		if dup.CumulativeImpactMS > survivor.CumulativeImpactMS {
			survivor.CumulativeImpactMS = dup.CumulativeImpactMS
		}
	}
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
	// RANKDIS-M18: io_pressure is structurally an aggregate metric but its
	// published value is a mixed-unit composite score, not a thread/cpu-ms
	// cumulative. The value carries its composite word at the shared render
	// sites; never add this ms-caliber suffix or derive score/window density.
	if runtimeTraceProjCompositeValueCaliber(node) {
		return ""
	}
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
	concurrency := runtimeTraceProjCrossThreadConcurrencyToken(node)
	switch {
	case queueDepth && zh:
		suffix += fmt.Sprintf("·≈平均排队深度 %.1f", density)
	case queueDepth:
		suffix += fmt.Sprintf(" ≈avg queue depth %.1f", density)
	case concurrency && zh:
		// PTV6-D (d) (#75 标本归因 #10): the irq-family density carries its
		// semantics (supply_pressure 族 ≈平均排队深度 先例) — cumulative
		// cpu·ms over the window IS the average in-window concurrency.
		suffix += fmt.Sprintf("·≈窗内并发 %.1f×", density)
	case concurrency:
		suffix += fmt.Sprintf(" ≈avg concurrency %.1f×", density)
	case zh:
		suffix += fmt.Sprintf("·≈均值 %.1f", density)
	default:
		suffix += fmt.Sprintf(" ≈mean %.1f", density)
	}
	return suffix
}

// runtimeTraceProjCrossThreadDensityWindowMS is the shared CMP-9 normalization
// denominator: the node's own precise span when valid, else the window base
// the value was actually measured over, else the projection window in window
// mode, else 0 (no density — never an estimate). Shared by the stanza suffix
// above and the F3 compare-overview cell so both surfaces normalize over the
// SAME window.
//
// §21 CWD (cmp_01 revisit 2026-07-07, D-新P0 排队深度方向反转 display half):
// numerator and denominator must share ONE window base. The specimen bug was
// a merged ×4 cross-window SUM (34008.569ms over ~501ms of query windows)
// divided by the 101ms anchor window — 平均排队深度 336.7 vs 449.3 inverted
// the flagship comparison's direction (tool truth 57.43 vs 43.49, 7.0 higher).
// Three same-base lanes replace the blind anchor-window fallback:
//   - a MergedCrossWindowMax row divides its MAX-member numerator by that
//     member's OWN typed query window (MergedMaxWindowStartTs/EndTs) — this
//     lane resolves FIRST, because a merged row's Span is the member-impact
//     ENVELOPE (multi-member), never the base the single MAX member was
//     measured over; no recorded window → no density, never a cross-base
//     estimate;
//   - a row carrying its own typed QueryWindow identity divides by THAT
//     window (the base the value was measured over), not the anchor;
//   - a merged row whose members span >1 known query windows without a
//     resolvable single base renders NO density — the anchor window is not
//     the base of a cross-window numerator.
//
// Rows without any window identity keep the anchor-window fallback
// byte-identically (fail-open: absence of identity never blocks the legacy
// single-window read).
func runtimeTraceProjCrossThreadDensityWindowMS(node types.TraceCausalProjectionNode, denom float64, windowMode bool) float64 {
	if node.MergedCrossWindowMax {
		if node.MergedMaxWindowStartTs > 0 && node.MergedMaxWindowEndTs > node.MergedMaxWindowStartTs {
			return (node.MergedMaxWindowEndTs - node.MergedMaxWindowStartTs) * 1000
		}
		return 0
	}
	if node.StartTs > 0 && node.EndTs > node.StartTs {
		return (node.EndTs - node.StartTs) * 1000
	}
	if node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs {
		return (node.QueryWindowEndTs - node.QueryWindowStartTs) * 1000
	}
	if node.MergedCount > 1 && len(node.MergedQueryWindows) > 1 {
		return 0
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

// runtimeTraceProjCrossThreadConcurrencyToken reports whether the node's typed
// kind token is an interrupt-family activity metric (PTV6-D (d), same token
// precedence as the queue-depth fork): for those, cumulative cpu·ms divided by
// the window is the average number of CPUs concurrently busy with the
// activity — the density wording says so (≈窗内并发 X.X×) instead of the
// neutral ≈均值. io_pressure / cpu_frequency_limit keep the neutral word.
func runtimeTraceProjCrossThreadConcurrencyToken(node types.TraceCausalProjectionNode) bool {
	switch runtimeTraceCausalProjectionCanonicalNode(firstNonEmptyAnswerString(node.TypeToken, node.Object)) {
	case "irq_burst", "irq_activity", "ipi_activity":
		return true
	}
	return false
}

func runtimeTraceProjNodeDisplayImpact(node types.TraceCausalProjectionNode) float64 {
	v, _ := runtimeTraceProjNodeDisplayImpactSource(node)
	return v
}

// runtimeTraceProjCPUListWord renders a sorted CPU-id list compactly with
// consecutive runs joined ("0-1,3-11") — the RNB-2 件5 constraint-description
// word face. Input is the typed sorted list from the wire decode.
func runtimeTraceProjCPUListWord(cpus []int) string {
	if len(cpus) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(cpus); {
		j := i
		for j+1 < len(cpus) && cpus[j+1] == cpus[j]+1 {
			j++
		}
		if b.Len() > 0 {
			b.WriteString(",")
		}
		if j > i {
			b.WriteString(fmt.Sprintf("%d-%d", cpus[i], cpus[j]))
		} else {
			b.WriteString(fmt.Sprintf("%d", cpus[i]))
		}
		i = j + 1
	}
	return b.String()
}

// runtimeTraceProjImpactSource enumerates which typed field the display-impact
// fallback chain resolved to. PTV5 C00 (#68 用户裁定 2026-07-05): the caliber
// of the main-line ms must be identifiable at the point of reading — a
// fallback-sourced value carries its (a)-table caliber word inline and never
// publishes a window-share percentage (the % denominator is the window, so a
// non-window-projection numerator could fake a >100% share and pull the
// 占窗>100% legend semantics it does not have).
type runtimeTraceProjImpactSource int

const (
	runtimeTraceProjImpactSourceNone runtimeTraceProjImpactSource = iota
	runtimeTraceProjImpactSourceWindow
	runtimeTraceProjImpactSourceCumulative
	runtimeTraceProjImpactSourceEffective
	runtimeTraceProjImpactSourceActual
)

// runtimeTraceProjNodeDisplayImpactSource is the single fallback chain behind
// runtimeTraceProjNodeDisplayImpact, returning the value together with its
// typed source (precise signal — one field comparison per step).
func runtimeTraceProjNodeDisplayImpactSource(node types.TraceCausalProjectionNode) (float64, runtimeTraceProjImpactSource) {
	if node.ImpactMS > 0 {
		return node.ImpactMS, runtimeTraceProjImpactSourceWindow
	}
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS, runtimeTraceProjImpactSourceCumulative
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS, runtimeTraceProjImpactSourceEffective
	}
	if node.ActualImpactMS > 0 {
		return node.ActualImpactMS, runtimeTraceProjImpactSourceActual
	}
	return node.ActualImpactMS, runtimeTraceProjImpactSourceNone
}

// runtimeTraceProjRowFallbackCaliberWord returns the C00 caliber word this
// row's main line will carry ("" when none) — the SINGLE source for the tag
// emission (runtimeTraceProjRowMetricParts) and the name-budget reserve
// (runtimeTraceProjRowMainReserve), so the two can never drift. Cross-thread
// aggregates carry their own unit suffix and stay out; a periodic row whose
// display falls back to the EFFECTIVE lane already carries the VS-1 tag with
// the same value (复核 Low 双 carrier 角, 2026-07-06) — no second carrier.
// PTV6-C ruling A (#73): the C00 word table splits by row kind — a ◇/▒
// stanza row whose fallback source is an attribution lane (cumulative /
// effective) wears the 累计(跨线程) family word instead of the on-chain
// attribution vocabulary; 实际状态 is not an attribution word and stays.
func runtimeTraceProjRowFallbackCaliberWord(node types.TraceCausalProjectionNode, kind string, zh bool) string {
	if runtimeTraceProjCrossThreadAggregateType(node) {
		return ""
	}
	impact, source := runtimeTraceProjNodeDisplayImpactSource(node)
	if impact <= 0 || source == runtimeTraceProjImpactSourceWindow {
		return ""
	}
	if node.PeriodicSource && source == runtimeTraceProjImpactSourceEffective {
		return ""
	}
	// RCM-2 D1 (§24.22 F6 置顶收口, 2026-07-08): a FAMILY row's value is a
	// same-thread family total — the 累计(跨线程) stanza word would mislabel it
	// cross-thread (the F6 witness), and the on-chain attribution words would
	// hide the fold caliber. The family caliber word is the row's word on
	// EVERY kind; an unknown caliber makes NO claim (fail-open, never the
	// banned cross-thread word — negative pin
	// TestRCM2FamilyRowNeverWearsCrossThreadCumWord).
	if runtimeTraceProjFamilyRow(node) {
		if word, _, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok {
			return word
		}
		return ""
	}
	if runtimeTraceProjStanzaRowKind(kind) &&
		(source == runtimeTraceProjImpactSourceCumulative || source == runtimeTraceProjImpactSourceEffective) {
		return runtimeTraceProjCrossThreadCumWord(zh)
	}
	return runtimeTraceProjImpactCaliberWord(source, zh)
}

// runtimeTraceProjImpactCaliberWord maps a non-window display-impact source to
// its (a)-table caliber word ("" for the window projection / no-value cases —
// the default caliber needs no word).
func runtimeTraceProjImpactCaliberWord(source runtimeTraceProjImpactSource, zh bool) string {
	switch source {
	case runtimeTraceProjImpactSourceCumulative:
		if zh {
			return "链上累计"
		}
		return "chain total"
	case runtimeTraceProjImpactSourceEffective:
		if zh {
			return "有效归因"
		}
		return "attribution"
	case runtimeTraceProjImpactSourceActual:
		if zh {
			return "实际状态"
		}
		return "actual state"
	}
	return ""
}

// runtimeTraceProjModelMaxImpact returns the fallback bar-scale maximum and
// whether a WALL-CLOCK anchor row backed it. anchored=false is the CALSIDE P2
// residual shape (DISPHYG-3 件3, §29.155 P2 filing, 2026-07-20): every valued
// row is cross-thread-aggregate or non-wall-clock (计数当量/综合评分 families)
// — the returned magnitude is then NOT a milliseconds fact, so the windowless
// scale sentences fork to the honest no-ruler wording instead of printing a
// false 「满格=本报告最大X.XXXms」 unit claim over a bar-less board. The VALUE
// channel is untouched: BarMaxMS keeps the fail-open magnitude byte-for-byte.
func runtimeTraceProjModelMaxImpact(model runtimeTraceProjTreeModel) (float64, bool) {
	// CMP-3: the bar full-scale anchors WALL-CLOCK values only — a cross-thread
	// cumulative aggregate (supply_pressure 101084.884ms in a 2.1s window) once
	// became the fallback scale and crushed every real 807ms row to one cell.
	max := 0.0
	fallback := 0.0
	consider := func(rows []runtimeTraceProjTreeRow) {
		for _, row := range rows {
			v := runtimeTraceProjNodeDisplayImpact(row.Node)
			// RNB R2 bar-scale invariance: a rank row folded into this chain
			// row anchored this scale pre-fold — its display magnitude stays
			// in the competition (folded arms are never cross-thread
			// aggregates by classification).
			for _, peer := range row.RankFoldPeers {
				if peer.DisplayImpactMS > v {
					v = peer.DisplayImpactMS
				}
			}
			if v > fallback {
				fallback = v
			}
			if runtimeTraceProjCrossThreadAggregateType(row.Node) {
				continue
			}
			// CALSIDE-1 件2 (F7, §29.147): the two non-wall-clock value
			// families leave the wall-clock scale competition the same way —
			// their rows draw no bar, and a count-equivalent/composite-score
			// magnitude as the fallback ruler would print a false
			// 「本报告最大Xms」 unit claim over a non-ms value.
			if runtimeTraceProjNonWallClockValueCaliber(row.Node) {
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
		return fallback, false
	}
	return max, true
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
	if model.TargetUserElected {
		// B1 (§12.3 裁定3): the anchor path was ELECTED by a typed user-entity
		// match at compile time — the root IS the user's thread even when the
		// renderer-side entity list is starved or divergent (compile-side
		// frame_target_resolution / runtime_targets lanes). Keep
		// ‹用户关注线程›; a disclaimer here would contradict the election.
		// §24.12 C11: an elected root under a different display name than the
		// user's entity still declares the dual-name normalization.
		model.TargetUserAliasEntity = runtimeTraceProjTargetUserEntityAlias(target, focus.Entities)
		return
	}
	if runtimeTraceProjTargetMatchesUserEntities(target, focus.Entities) {
		// 🎯 root really is a user-named thread — keep ‹用户关注线程›. §24.12
		// C11 (同 tid 双名归一声明): a tid-decided match whose display names
		// differ declares the pair explicitly (the reader typed one name and
		// reads the other).
		model.TargetUserAliasEntity = runtimeTraceProjTargetUserEntityAlias(target, focus.Entities)
		return
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
//     (RF1a);
//   - PTV8-RCR-C (§24.12 C11 同 tid 双名, 2026-07-08): tid equality DECIDES
//     when BOTH sides expose a -pid tail (§11-N7 tid-first rule, mirroring
//     the compile-side anchor comparator traceCausalProjectionAnchorLabelMatchesEntity)
//     — the user's com.xs.fm.lite-6565 and the trace's main-6565 are ONE
//     thread; without this arm the root mislabeled itself ‹分析锚点线程›. The
//     dual-name normalization is DECLARED via
//     runtimeTraceProjTargetUserEntityAlias below.
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
			// §24.12 C11: both sides carry a -pid tail — tid decides (双名).
			if _, epid, ok := runtimeTraceProjSplitNamePid(entity); ok && epid == pid {
				return true
			}
		}
	}
	return false
}

// runtimeTraceProjTargetUserEntityAlias (PTV8-RCR-C, §24.12 C11 同 tid 双名
// 归一声明, 2026-07-08) returns the first user entity naming the SAME tid as
// the target under a DIFFERENT display name (com.xs.fm.lite-6565 vs main-6565
// — the process-name and thread-name faces of one thread). "" when no such
// pair exists (the note lane stays byte-identical). Precise signals only:
// both sides expose a -pid tail, integer-equal, verbatim-different non-empty
// name halves.
func runtimeTraceProjTargetUserEntityAlias(target string, entities []string) string {
	name, pid, hasPid := runtimeTraceProjSplitNamePid(target)
	if !hasPid {
		return ""
	}
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" || entity == target {
			continue
		}
		if ename, epid, ok := runtimeTraceProjSplitNamePid(entity); ok && epid == pid &&
			ename != "" && ename != name {
			return entity
		}
	}
	return ""
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
	// EVOLUTION RECORD (v5 P0 重-3, user ruling 2026-07-11, design
	// causal_tree_v5_design_20260711.md §C.3): the opener carries the typed
	// second info token so the HTML preview classifies on a precise signal
	// instead of content sniffing. First token stays "text" — terminal /
	// markdown / glamour behavior is byte-identical (parity-pinned); fence
	// CONTENT lines are untouched.
	b.WriteString(tracefence.Opener + "\n")
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
		// PTV8-RCR-C (§24.12 C11 同 tid 双名归一声明): the root matched the
		// user's entity by tid under a DIFFERENT display name — declare the
		// pair once, right under the header, so the reader who typed
		// com.xs.fm.lite-6565 recognizes main-6565 as the same thread.
		if alias := strings.TrimSpace(model.TargetUserAliasEntity); alias != "" {
			if zh {
				b.WriteString("- " + model.Target + " 即你指定的 " + alias + "(同一 tid 的双名,已归一)\n")
			} else {
				b.WriteString("- " + model.Target + " IS your specified " + alias + " (two names of one tid, normalized)\n")
			}
		}
	} else {
		// UXR-1 §29.36① (2026-07-11): the unified 「⊘ <短结论>(<短因>)」 banner
		// wears the existing 无匹配唤醒/链止 closed-set glyph — record its mark
		// at THIS emission site so the ⊘ legend entry renders with it.
		model.Marks.mark(runtimeTraceProjMarkUndrillable)
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
				b.WriteString("- 分析锚=" + anchor + "(非用户关注对象;用户关注 " + strings.Join(model.RootFocusUserEntities, "、") + " 的唤醒链未在本报告查询)\n")
			} else {
				b.WriteString("- analysis anchor = " + anchor + " (not the user-specified focus; the wakeup chain for " + strings.Join(model.RootFocusUserEntities, ", ") + " was not queried for this report)\n")
			}
		}
	}
	selfWindowMS := 0.0
	if windowMode {
		selfWindowMS = denom
	}
	for _, row := range model.SelfRows {
		row.marks = model.Marks // NEW-7: record at the emission site of this pass
		for _, line := range runtimeTraceProjSelfRowLines(row, selfWindowMS, zh) {
			b.WriteString(line + "\n")
		}
	}
	// GAP-B G11 (§27.5): the bounded self-wait relocation's overflow
	// disclosure — the rows themselves keep their ◇/▒ stanza seats below
	// (lossless); the self area names their count and single max so the
	// reader never mistakes the top-K for the whole population.
	if model.SelfWaitOverflowCount > 0 {
		// P2a rider 件2a F3 (§29.58.2 F3, 2026-07-13): this stanza-level
		// side-note follows the §29.58.1 a) one-level-deeper arm (its host is
		// the whole self stanza; the independent mint point rides the same
		// geometry as the per-row "· " notes).
		if zh {
			b.WriteString(fmt.Sprintf("│       · 另有 %d 条自身等待症状行(单条最大 %.3fms)未在此逐行展示,见 ◇/▒ 区段与明细\n",
				model.SelfWaitOverflowCount, model.SelfWaitOverflowMaxMS))
		} else {
			b.WriteString(fmt.Sprintf("│       · %d more self wait-symptom rows (single max %.3fms) are not listed here; see the ◇/▒ stanzas and the detail blocks\n",
				model.SelfWaitOverflowCount, model.SelfWaitOverflowMaxMS))
		}
	}
	if len(model.TreeRows) > 0 && strings.TrimSpace(model.Target) != "" {
		b.WriteString("│\n")
	}
	// DISPLAY-HYG 二轮 复核件1 (catalog C3): ONE shared value slot across the
	// trunk and both stanzas — stamped on the render copies exactly like the
	// marks collector (family-free renders compute the legacy 11 and stay
	// byte-identical).
	valueSlot := runtimeTraceProjTreeValueSlot(model, zh)
	for _, row := range model.TreeRows {
		row.marks = model.Marks
		row.ValueSlot = valueSlot
		b.WriteString(runtimeTraceProjTreeRowLine(row, width, denom, windowMode, zh))
		b.WriteString("\n")
	}
	if len(model.Adjacent) > 0 {
		model.Marks.mark(runtimeTraceProjMarkAdjacentStanza)
		b.WriteString("\n")
		// PTV4 T4: the stanza semantics live in the legend's ◇ entry — the
		// header keeps only the mark + name. PTV5 C02 (#68): the header uses
		// the legend's own noun 邻近区段 — the former 邻近链 claimed the chain
		// identity the ◇ definition explicitly denies (不在唤醒路径上).
		if zh {
			b.WriteString("◇ 邻近区段\n")
		} else {
			b.WriteString("◇ Adjacent\n")
		}
		for _, row := range model.Adjacent {
			row.marks = model.Marks
			row.ValueSlot = valueSlot
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
			row.ValueSlot = valueSlot
			b.WriteString(runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh))
			b.WriteString("\n")
		}
	}
	// SPANVIS-1 (user ruling 2026-07-19 定形原则 面1): the ◈ pure-advisory
	// business-span mention block — non-ordinal mention rows at the fence
	// tail (no badge, no bar, no tier; strings only, structurally invisible
	// to every board/census/conservation population). Empty = zero bytes.
	if mentionLines := runtimeTraceProjBusinessSpanMentionLines(model, zh); len(mentionLines) > 0 {
		model.Marks.mark(runtimeTraceProjMarkBusinessSpanMention)
		b.WriteString("\n")
		for _, line := range mentionLines {
			b.WriteString(line)
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
	// 复核 P1 簇二 (2026-07-06) → PTV8-RCR-C §24.9 G2 (2026-07-08): the width
	// pass measures the SAME fitted name the label composer renders (ONE
	// helper carrying both the main reserve and the grammar-word keep suffix),
	// so the shared column measures exactly what each row will render.
	consider := func(fixed, name string, row runtimeTraceProjTreeRow) {
		fixedW := runewidth.StringWidth(fixed)
		if need := fixedW + runewidth.StringWidth(runtimeTraceProjRowNameFitted(fixedW, row, name, zh)); need > width {
			width = need
		}
	}
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
			continue
		}
		fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
		consider(fixed, name, row)
	}
	// Stanza rows pad to the same column (their bars share the start column);
	// their fixed part IS runtimeTraceProjStanzaRowLine's composer (nil marks:
	// the width pass records nothing).
	for _, rows := range [][]runtimeTraceProjTreeRow{model.Adjacent, model.Background} {
		for _, row := range rows {
			consider(runtimeTraceProjStanzaRowFixed(row, nil, zh),
				runtimeTraceProjRowName(row, zh), row)
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
	// PTV8-LAD L4 (§24.11 维度A P1). EVOLUTION RECORD: this arm's former
	// clamp-1 carried an "unreachable via the trunk cap" claim that huadong_78
	// FALSIFIED — B1-b fold splitting makes fixedW > 50 reachable and rows
	// rendered "◦ …" with a 1-cell name (identity evaporated on the very rows
	// F2 expanded to disclose it). The cap-vs-floor conflict now keeps the
	// 8-cell identity floor (the omitted-row roster's pid-tail floor, T2
	// mid-truncation keeps the pid): when the shared-column cap and the name
	// floor collide, the NAME wins its last 8 cells and the row runs past the
	// column instead of vaporizing the identity (§24.8 重要信息永不省略). With
	// the L4 indent cap fixedW is bounded, so the overrun is itself bounded.
	if budget < 8 {
		budget = 8
	}
	return budget
}

// runtimeTraceProjTreeHeaderLabel composes the 🎯 root header label (target +
// provenance chip, §7.30 C4a R2). Shared by the fence render and the width
// pass so the column measurement can never drift from the emitted header.
func runtimeTraceProjTreeHeaderLabel(model runtimeTraceProjTreeModel, zh bool) string {
	// PTV8-RCR-A §24.3: 🎯 → ⊚ (无亮色 hard rule — the fence's only colored
	// emoji leaves; the root mark stays single-cell text presentation, shared
	// constant with the legend + width pin; 复核 F3: EAW-Neutral glyph).
	// EVOLUTION RECORD (v5 P0 备-2, 2026-07-11): the four provenance chips
	// moved verbatim into internal/tracefence (shared with the preview
	// classifier's legacy ⊚ arm — byte-identical output).
	header := runtimeTraceProjRootGlyph + " " + model.Target
	switch {
	case model.RootFocusAnchorOnly && zh:
		header += " " + tracefence.TargetChipAnchorZH
	case model.RootFocusAnchorOnly:
		header += " " + tracefence.TargetChipAnchorEN
	case zh:
		header += " " + tracefence.TargetChipFocusedZH
	default:
		header += " " + tracefence.TargetChipFocusedEN
	}
	return header
}

// runtimeTraceProjTreeLabelParts splits a tree row label into its fixed part
// (prefix + edge + icon + separators) and the name, so truncation can target
// the name alone instead of the composed label (B1).
func runtimeTraceProjTreeLabelParts(row runtimeTraceProjTreeRow, zh bool) (string, string) {
	edge := runtimeTraceProjEdgeLabel(row.Edge, zh)
	// EVOLUTION RECORD (UXR-1 §29.36④, 2026-07-11, supersedes the C6 edge-face
	// placement): the former 链上·未接入树─ edge rewrite is RETIRED — both
	// depthless variants render the simplified 链上─ edge (lane 前缀简化), and
	// the 父节点未确认 / 深度未解析 auxiliary words live on the 行2 chip + detail
	// faces only (the C6 word family itself is unchanged — one word, two
	// remaining faces).
	if row.Kind == runtimeTraceProjTreeRowDepthless && strings.TrimSpace(row.Parent) == "" {
		// Flat fallback (no resolved target): a hanging "wakes" edge word would
		// claim a wake relation with no wakee — render a bare branch instead.
		edge = ""
	}
	// EVOLUTION RECORD (P2a rider 件1, §29.55.3 处置更新, 用户裁定形
	// `链上─ ◌ 其余 2 项(折叠)`, 2026-07-13): the PTS fold-row edge-omission
	// arm is RETIRED — 边词管车道: the lane word is TRUE of every folded
	// member (the former omission rationale targeted wake-RELATION edges,
	// which a counted roster indeed does not carry, but 链上─ claims lane
	// membership only), and restoring it puts the fold's state mark back in
	// its siblings' glyph column (并列自明). The row NAME dropped its 「链上」
	// in exchange (行名管折叠, net width +1); the PTS count+roster-head pin
	// covers the new geometry. Flat renders keep the bare branch through the
	// Parent=="" arm above (CMP-7a: flat renders never claim on-chain).
	if edge != "" && !row.Node.OnChainOverflowFold {
		// NEW-7 edge mark, recorded AFTER the flat-fallback suppression so a
		// suppressed edge never claims a legend entry. The typed switch mirrors
		// runtimeTraceProjEdgeLabel exactly (default = wake, same as its default
		// arm) — keep the two in lockstep. Fold rows render the 链上─ edge but
		// record their OWN fold mark (at the name mint) instead of the
		// EdgeChainUnresolved entry, whose wording promises the 链上·深度未解析
		// 行注 chip the fold row deliberately never wears
		// (runtimeTraceProjChainDepthChipEligible exclusion — 图例是承诺面).
		switch row.Edge {
		case runtimeTraceProjTreeEdgeDrill:
			row.marks.mark(runtimeTraceProjMarkEdgeDrill)
		case runtimeTraceProjTreeEdgeSemantic:
			row.marks.mark(runtimeTraceProjMarkSemanticSpan)
		case runtimeTraceProjTreeEdgeCause:
			row.marks.mark(runtimeTraceProjMarkEdgeCause)
		case runtimeTraceProjTreeEdgeOwn:
			row.marks.mark(runtimeTraceProjMarkEdgeOwn)
		case runtimeTraceProjTreeEdgeChainUnresolved:
			// §24.12 C6: the depth-known unattached variant records ITS entry
			// (the 深度未解析 entry would describe an edge that never rendered).
			if runtimeTraceProjDepthlessUnattachedRow(row) {
				row.marks.mark(runtimeTraceProjMarkChainSeatUnattached)
			} else {
				row.marks.mark(runtimeTraceProjMarkEdgeChainUnresolved)
			}
		default:
			row.marks.mark(runtimeTraceProjMarkEdgeWake)
		}
	}
	badge := ""
	if glyph := runtimeTraceProjBadgeGlyph(row.Badge); glyph != "" {
		// PTV4 T6: the TOP-N badge sits right before the state glyph. It is an
		// independent token, never a state glyph (the one-glyph invariant
		// counts state icons only). UXG-0 D5 (2026-07-11): one space between
		// badge and state glyph — every state icon's LEFT neighbor is now
		// constructively a space (the right one always was), which is the
		// overflow budget the D4 HTML icon boxes spend. Single grid source:
		// all three faces (terminal/Markdown/HTML) change together.
		row.marks.mark(runtimeTraceProjMarkBadge)
		badge = glyph + " "
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
// only — T1 main-row invariant). 复核 P1 簇二 (2026-07-06) carve-out: the
// row's OWN main-line Keep suffix (the C00 caliber word) joins the budget as
// a reserve — the name mid-truncates and gives way so the essentials hold the
// 100-cell cap (PTV4 ↺ 预留同型).
func runtimeTraceProjTreeLabel(fixed, name string, width int) string {
	return runtimeTraceProjTreeLabelReserve(fixed, name, width, 0)
}

func runtimeTraceProjTreeLabelReserve(fixed, name string, width, reserve int) string {
	budget := runtimeTraceProjTreeNameBudgetReserve(runewidth.StringWidth(fixed), reserve)
	if runewidth.StringWidth(name) > budget {
		name = runtimeTraceProjTruncateName(name, budget)
	}
	label := fixed + name
	if pad := width - runewidth.StringWidth(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return label
}

// runtimeTraceProjTreeLabelRow is the ROW-aware label composer (PTV8-RCR-C
// §24.9 G2): it fits the name through the shared runtimeTraceProjRowNameFitted
// (grammar-word keep suffix + main reserve, one judgment with the width pass)
// and pads to the shared column.
func runtimeTraceProjTreeLabelRow(fixed string, row runtimeTraceProjTreeRow, name string, width int, zh bool) string {
	label := fixed + runtimeTraceProjRowNameFitted(runewidth.StringWidth(fixed), row, name, zh)
	if pad := width - runewidth.StringWidth(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return label
}

// runtimeTraceProjTreeNameBudgetReserve subtracts a row's main-line Keep
// suffix reserve from the B1 name budget. The readability floor ERODES by the
// reserve too (never below the 8-cell identity floor the omitted-row roster
// already uses — a T2 mid-truncated name keeps its pid tail there): the
// caliber word is load-bearing 口径 information, so on a floor-squeezed row
// the NAME gives way (复核 P1 簇二 裁定: 名字被中截让位), not the word. Deep
// rows whose fixed rails alone exhaust every yield keep the documented
// quantified-floor discipline (marks render whole past the cap, recorded
// as-is — see the essentials-floor note in runtimeTraceProjRowLineWithMetrics).
func runtimeTraceProjTreeNameBudgetReserve(fixedW, reserve int) int {
	budget := runtimeTraceProjTreeNameBudget(fixedW)
	if reserve <= 0 {
		return budget
	}
	budget -= reserve
	floor := runtimeTraceProjTreeNameMinWidth - reserve
	if floor < 8 {
		floor = 8
	}
	if budget < floor {
		budget = floor
	}
	return budget
}

// runtimeTraceProjRowMainReserve (复核 P1 簇二, 2026-07-06): the width this
// row's OWN main-line Keep suffix will add after the label — today the C00
// caliber word (" · <word>") — reserved OUT of the name budget by the label
// composer AND the shared width pass (both call this one helper, no drift).
// The ↺ suffix keeps its own reserve in runtimeTraceProjRowLineWithMetrics.
func runtimeTraceProjRowMainReserve(row runtimeTraceProjTreeRow, zh bool) int {
	if !row.HasData {
		return 0
	}
	if word := runtimeTraceProjRowFallbackCaliberWord(row.Node, row.Kind, zh); word != "" {
		return runewidth.StringWidth(" · " + word)
	}
	return 0
}

// runtimeTraceProjRowNameKeepSuffix (PTV8-RCR-C, §24.9 维度A G2, 2026-07-08)
// runtimeTraceProjMergeCountChip renders the 行1 merge-count chip (§24.2
// 上移行1). EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「 ×N」→zh
// 「 N次」/ en 「 n=N」 — the ×N marker read as multiplication and was
// semantically overloaded across the merge forms; the chip now speaks the
// same count vocabulary as the data tokens (tracefence display-table ⑥).
func runtimeTraceProjMergeCountChip(count int, zh bool) string {
	if zh {
		return fmt.Sprintf(" %d次", count)
	}
	return fmt.Sprintf(" n=%d", count)
}

// returns the UN-CUTTABLE tail of a chain-universe cause node's 行1: the
// state-composition 词位 (§24.7 用户令: 链上行行1 = glyph+线程名+·+状态构成 —
// typed families only: the inversion composition word and the pure
// scheduler-state words; lock/span/aggregate name lanes keep their legacy
// discipline) plus the §24.2 ×N chip. The seat is reserved out of the name
// budget with the RowMainReserve discipline — truncation eats only the
// thread-name head (MidTruncateKeepPid), never the grammar word. Before this
// seat existed, the word was a plain name suffix: a width cut dropped it and
// the #12 guarantee re-spat it below the OwnLine block as a stray sixth line
// (opendir_78 E4 gap①).
func runtimeTraceProjRowNameKeepSuffix(row runtimeTraceProjTreeRow, zh bool) string {
	if !row.HasData {
		return ""
	}
	// UXR-1 §29.36④: the ×N同值 chip rides the name tail on EVERY data row —
	// reserved out of the name budget so a width cut can never eat it (the
	// chip is grammar, not name; RowName appends it LAST, so it is the
	// outermost suffix on every arm below).
	dedup := ""
	if row.Node.DuplicatePublications > 1 {
		dedup = " " + runtimeTraceProjDedupFoldTagText(row.Node.DuplicatePublications, zh)
	}
	// 76684 行1 修 (SMR-1 批, 2026-07-12): the generic-unresolved shape's
	// state word joins the keep lane even on rank-less rows — the shared
	// label column otherwise truncated 「 · iowait」 off 行1 and the #12
	// guarantee could only re-add it as a demotable tag (the witness lost the
	// state word to 行2 while the peer word held 行1).
	if (!runtimeTraceProjCauseNodeRow(row) && runtimeTraceProjGenericUnresolvedStateNameWord(row.Node, zh) == "") ||
		!runtimeTraceProjChainUniverseRowKind(row.Kind) {
		return dedup
	}
	xn := ""
	if runtimeTraceProjCauseEventFoldRow(row) {
		xn = runtimeTraceProjMergeCountChip(row.Node.MergedCount, zh)
	} else if runtimeTraceProjFamilyRow(row.Node) {
		// RCM-2 D2: the family count chip is grammar, not name — reserved out
		// of the name budget like the event-form chip (a width cut eats the
		// name head, never the count).
		xn = runtimeTraceProjMergeCountChip(row.Node.FamilyMemberCount, zh)
	}
	word, token := runtimeTraceProjRowCauseWordToken(row, zh)
	stateWord := (token == "priority_inversion_candidate" &&
		runtimeTraceCausalProjectionInversionRow(row.Node)) ||
		runtimeTraceProjStateTokenClass(token) != ""
	// RCM-2 D2 (§24.7 行1 词位): a family contender's TYPE word is its 词位 —
	// 「块设备IO(inode) ×2」 must survive a name squeeze whole (the subject
	// head mid-truncates instead), exactly like the state-composition words.
	if word == "" || !(stateWord || runtimeTraceProjFamilyRow(row.Node)) {
		return xn + dedup
	}
	name := runtimeTraceProjRowNameBase(row, zh)
	switch {
	case name == word:
		return word + xn + dedup
	case strings.HasSuffix(name, " · "+word):
		return " · " + word + xn + dedup
	}
	return xn + dedup
}

// runtimeTraceProjDedupRowShortStateWord (§29.58.5 ③, 2026-07-13): the SHORT
// state word a dedup fold row's 行1 keeps (主行三要素) when the width fit
// dropped the full cause word — typed lanes only (StateKind label first, else
// the TypeToken state word through the same refined-class chain the 裁定4 tag
// uses); "" when the node carries no typed state (labels are never fabricated).
func runtimeTraceProjDedupRowShortStateWord(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	if label := strings.TrimSpace(runtimeTraceProjStateKindLabel(node, zh)); label != "" {
		return label
	}
	if class := runtimeTraceCausalProjectionTypeTokenStateClass(node); class != "" {
		return strings.TrimSpace(runtimeTraceCausalProjectionTypeTokenStateWord(
			runtimeTraceCausalProjectionRefinedStateClass(node, class), zh))
	}
	// Peer-relation forms (IO等待(对端 X) …): the short word is the relation
	// form's own state head — SAME wording home as the composers (133136 E29
	// witness form 「主体 · IO等待 2次同值」).
	if kind := runtimeTraceCausalProjectionResolvedPeerKind(node); kind != "" {
		return runtimeTraceCausalProjectionPeerRelationShortWord(kind, zh)
	}
	if kind := runtimeTraceCausalProjectionUnresolvedPeerKind(node); kind != "" {
		return runtimeTraceCausalProjectionPeerRelationShortWord(kind, zh)
	}
	return ""
}

// runtimeTraceProjRowNameFitted is THE single name-budget fit (§24.9 G2): the
// keep suffix is carved out of the budget as a reserve and re-attached whole
// after the head truncation; every other name keeps the legacy TruncateName
// byte-identically. Shared by the width pass and the label composer so the
// measured column and the rendered label can never drift.
func runtimeTraceProjRowNameFitted(fixedW int, row runtimeTraceProjTreeRow, name string, zh bool) string {
	// UXG-0 D5 (2026-07-11): the badge→state-glyph gap is badge geometry, not
	// name pressure — wearing a seat must never cost the row a name cell (the
	// opendir E4 lock row lost its adjudicated 行1 cause word 持锁阻塞 to that
	// off-by-one). The gap cell is refunded HERE — the ONE fit shared by the
	// width pass and the label composer — so pre-D5 name forms hold and the
	// measured column still equals the rendered label. The gate is exactly the
	// emitters' gap condition (a seatless/stale row added no gap).
	if runtimeTraceProjBadgeGlyph(row.Badge) != "" {
		fixedW--
	}
	reserve := runtimeTraceProjRowMainReserve(row, zh)
	if keep := runtimeTraceProjRowNameKeepSuffix(row, zh); keep != "" && strings.HasSuffix(name, keep) {
		head := strings.TrimSuffix(name, keep)
		budget := runtimeTraceProjTreeNameBudgetReserve(fixedW, reserve+runewidth.StringWidth(keep))
		if head != "" && runewidth.StringWidth(head) > budget {
			head = runtimeTraceProjTruncateName(head, budget)
			// §29.58.5 ③ (user ruling 2026-07-13): a dedup fold row's 行1 keeps
			// its STATE WORD in short form (主行三要素 — 「主体 · IO等待 2次同值」)
			// even when the width cut dropped the full cause word; the full form
			// (with the peer tail) stays on the row-2 guarantee copy. Typed
			// state lanes only (no guess); a typed floor like the PTS fold-name
			// floor — the row may run past the shared column rather than lose
			// the state word.
			if row.Node.DuplicatePublications > 1 {
				causeWord, _ := runtimeTraceProjRowCauseWordToken(row, zh)
				// The keep lane may already reserve the state word (chain-row
				// grammar 词位) — judge the WHOLE rendered name, never the
				// head alone (tieba CookieMonster witness: a second
				// 「· runnable」 would double-speak).
				if causeWord != "" && !strings.Contains(head+keep, causeWord) {
					if short := runtimeTraceProjDedupRowShortStateWord(row, zh); short != "" &&
						!strings.Contains(head+keep, short) {
						head += " · " + short
					}
				}
			}
		}
		return head + keep
	}
	budget := runtimeTraceProjTreeNameBudgetReserve(fixedW, reserve)
	// EVOLUTION RECORD (R9 §29.93.2, 2026-07-15): the P2a fold head-name
	// protection floor (runtimeTraceProjFoldNameProtectedWidth — count stem +
	// whole first roster member incl. B6 pointer allowed to run past the
	// shared column) is RETIRED with the inline preview itself: fold-row line
	// 1 is now the bare counted label (「其余 N 项(折叠)」), which always fits
	// the standard column, and the head member + pointer live on the
	// subordinate line 2 (runtimeTraceProjFoldMemberSinkLine). The PTS
	// 「计数+头名永不截断」 promise is kept BY CONSTRUCTION on the new faces.
	if runewidth.StringWidth(name) > budget {
		name = runtimeTraceProjTruncateName(name, budget)
	}
	return name
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
			// PTV8-RCR-B (UXA 域A #24/漏审A, 2026-07-08). EVOLUTION RECORD:
			// the suffix used to tail-truncate mid-word into residue ("s_s…"
			// from a 7-rune token) while the subordinate/detail surfaces
			// re-print the whole word. The cut now lands only on a WORD
			// boundary ((/·/空格//): "持锁阻塞(等待方 …)" still compacts to
			// "持锁阻塞…" (PTV8-RCR-A E4 行1 形态保持), while a token whose
			// FIRST word cannot fit yields its seat whole (整词让位,不留残词).
			if objBudget := width - subjW - 3; objBudget >= 2 {
				if runewidth.StringWidth(rest) <= objBudget {
					return subject + " · " + rest
				}
				if cut := runtimeTraceProjBoundaryTruncate(rest, objBudget); cut != "" {
					return subject + " · " + cut
				}
			}
			// No boundary prefix fits beside the identity tail — drop the
			// suffix and keep the pid-tailed subject whole (身份载重段优先).
			if subjW <= width {
				return subject
			}
			return runtimeTraceProjMidTruncateKeepPid(subject, width)
		}
	}
	return strings.TrimRight(runtimeTraceProjPadDisplay(name, width), " ")
}

// runtimeTraceProjBoundaryTruncate (PTV8-RCR-B, UXA 域A #24/漏审A,
// 2026-07-08) tail-truncates text to fit budget cells, cutting ONLY at a word
// boundary — immediately before a "(", "（", "·", "/", "+", or space — and
// appends "…". "" when no boundary-aligned prefix fits (the caller yields the
// seat instead of leaving an opaque mid-word residue like "s_s…").
// PTV8-RCR-C (§24.9 G2 防御半): '+' joins the boundary set so a composition
// word that ever falls back onto this lane cuts to "runnable…" instead of
// vanishing whole — the grammar words themselves ride the reserved 行1 seat
// (runtimeTraceProjRowNameKeepSuffix) and normally never reach this cut.
func runtimeTraceProjBoundaryTruncate(text string, budget int) string {
	if budget < 2 {
		return ""
	}
	runes := []rune(text)
	best := -1
	w := 0
	for i, r := range runes {
		if i > 0 {
			switch r {
			case '(', '（', '·', '/', '+', ' ':
				if w <= budget-1 {
					best = i
				}
			}
		}
		w += runewidth.RuneWidth(r)
	}
	if best <= 0 {
		return ""
	}
	return string(runes[:best]) + "…"
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
	// DISPLAY-HYG 二轮 catalog B5 (§29.104.18.1, 2026-07-16 witness L193
	// 「CompThrea…-2955」吞掉全文唯一区分位 `_0`): the middle cut keeps a TAIL
	// segment of the head too — the head's trailing runes ("_0", "#6",
	// worker suffixes) are the distinguishing segment on pool-thread names.
	// Up to 4 cells of head tail survive when the head budget leaves ≥3
	// cells for the prefix (headBudget ≥ 7); tighter budgets keep the legacy
	// prefix-only form byte-identically ("RS…-1963" / "…-59843" pins).
	if headBudget >= 7 && runewidth.StringWidth(head) > headBudget {
		var suffix []rune
		w := 0
		for i := len(runes) - 1; i >= 0; i-- {
			rw := runewidth.RuneWidth(runes[i])
			if w+rw > 4 {
				break
			}
			w += rw
			suffix = append([]rune{runes[i]}, suffix...)
		}
		prefix := runes[:len(runes)-len(suffix)]
		for len(prefix) > 0 && runewidth.StringWidth(string(prefix)) > headBudget-w {
			prefix = prefix[:len(prefix)-1]
		}
		return string(prefix) + "…" + string(suffix) + tail
	}
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
//
// EVOLUTION RECORD (UXR-1 §29.36①, user ruling 2026-07-11): the 唤醒链不可
// 上溯类 banner is the unified typed form 「⊘ <短结论>(<短因>)」 — ⊘ reuses
// the existing "无匹配唤醒,链止" closed-set glyph (zero new glyphs), the long
// sentence forms (95072 witness) compress to 短结论(短因), and the "以下各行
// 按层级平铺" render note LEFT the head line (the 树读法 legend's own head
// clause already teaches it — the head never repeats the legend). The opaque
// fallback carries no parenthetical: absence never invents a cause.
// EVOLUTION RECORD (v5 P0 备-2, 2026-07-11): the six banner strings moved
// verbatim into internal/tracefence — the single source shared with the
// preview classifier's legacy-fallback whitelist (byte-identical output).
func runtimeTraceProjFlatFallbackHeader(model runtimeTraceProjTreeModel, zh bool) string {
	switch {
	case model.missingWakeup():
		if zh {
			return tracefence.FlatHeadMissingWakeupZH
		}
		return tracefence.FlatHeadMissingWakeupEN
	case model.WakeupChainRecommendedNotRun:
		if zh {
			return tracefence.FlatHeadNotDrilledZH
		}
		return tracefence.FlatHeadNotDrilledEN
	default:
		if zh {
			return tracefence.FlatHeadUnresolvedZH
		}
		return tracefence.FlatHeadUnresolvedEN
	}
}

// runtimeTraceProjHolderSiteCompact (PTV8-RCR-B, UXA 域A #29, 2026-07-08):
// signature-aware display compaction for the 持有点 tree tag. A plain head cut
// on a Java signature keeps the return type (the least useful part) and drops
// 类.方法(文件:行) (the part the customer needs on the row). Deterministic
// string work, display-only — the full signature stays lossless in the detail
// block's 持有点/span lines and the raw record.
func runtimeTraceProjHolderSiteCompact(site string, maxRunes int) string {
	site = strings.TrimSpace(site)
	if maxRunes <= 1 || len([]rune(site)) <= maxRunes {
		return runtimeTraceCausalProjectionCompactCellText(site, maxRunes)
	}
	tailKeep := func(raw string) string {
		runes := []rune(raw)
		if len(runes) <= maxRunes {
			return runtimeTraceCausalProjectionMarkdownSafe(raw)
		}
		return runtimeTraceCausalProjectionMarkdownSafe("…" + string(runes[len(runes)-(maxRunes-1):]))
	}
	// Locate a trailing "(file:line)" group — the coordinate the row must keep.
	if !strings.HasSuffix(site, ")") {
		return tailKeep(site)
	}
	open := strings.LastIndex(site, "(")
	if open <= 0 {
		return tailKeep(site)
	}
	fileLine := site[open:]
	if !strings.ContainsRune(fileLine, ':') {
		return tailKeep(site)
	}
	head := strings.TrimSpace(site[:open])
	// Strip a trailing argument list "(…)" off the head.
	if head != "" && strings.HasSuffix(head, ")") {
		if i := strings.LastIndex(head, "("); i > 0 {
			head = strings.TrimSpace(head[:i])
		}
	}
	// Drop the return type (everything before the last space).
	if j := strings.LastIndex(head, " "); j >= 0 {
		head = head[j+1:]
	}
	// Keep the last two dot segments: Class.method.
	if segs := strings.Split(head, "."); len(segs) > 2 {
		head = strings.Join(segs[len(segs)-2:], ".")
	}
	if head == "" {
		return tailKeep(site)
	}
	return tailKeep(head + fileLine)
}

// runtimeTraceProjNoRulerClause is the ONE word face (词面单点) of the
// DISPHYG-3 件3 honest no-ruler sentence family (§29.155 P2 残形, 2026-07-20):
// a windowless board with NO wall-clock scale anchor draws zero bars, so a
// 「满格=本报告最大X.XXXms」 claim would (a) wear a false ms unit over a
// count-equivalent/composite/cpu·ms magnitude and (b) declare a full-bar
// ruler on a bar-less board. The clause uses the regime-neutral 「·」
// separator so the fence head (half-width regime) and the lead prose sentence
// (full-width regime) share ONE face without forking (C8 discipline).
// withValues forks the degenerate all-zero shape: a board without any
// positive value must not claim 「本板值均非墙钟」 (there are no values to
// classify) — it drops the value claim and keeps only the no-ruler fact.
func runtimeTraceProjNoRulerClause(withValues, zh bool) string {
	if zh {
		if withValues {
			return "本板值均非墙钟·" + tracefence.NoRulerMarkZH
		}
		return "本板无持值行·" + tracefence.NoRulerMarkZH
	}
	if withValues {
		return "all board values are non-wall-clock · " + tracefence.NoRulerMarkEN
	}
	return "no valued rows on this board · " + tracefence.NoRulerMarkEN
}

// EVOLUTION RECORD (v5 P0 备-2, 2026-07-11): the 满格= / bar full = markers
// are composed from internal/tracefence — the same constants the preview
// classifier's legacy scale-signature arm consumes (byte-identical output).
// DISPHYG-3 件3 (2026-07-20): the windowless branch forks on the typed
// wall-clock-anchor signal — the un-anchored shape speaks the honest
// no-ruler clause and mints NO 满格= marker (newly generated fences classify
// on the typed info token; the ScaleMark sniffing lane is archive-only, so
// dropping the marker on this shape breaks no classification).
func runtimeTraceProjScaleNote(model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS > 0 {
		if zh {
			return fmt.Sprintf(tracefence.ScaleMarkZH+"窗口%.3fms", model.WindowMS)
		}
		return fmt.Sprintf(tracefence.ScaleMarkEN+" window %.3fms", model.WindowMS)
	}
	if !model.BarScaleWallClockAnchored {
		clause := runtimeTraceProjNoRulerClause(model.BarMaxMS > 0, zh)
		if zh {
			return "窗口起止未采集·" + clause + "(不显示占窗%)"
		}
		return "window bounds not captured; " + clause + " (no window %)"
	}
	if zh {
		return fmt.Sprintf("窗口起止未采集·"+tracefence.ScaleMarkZH+"本报告最大%.3fms(回退尺度,不显示占窗%%)", model.BarMaxMS)
	}
	return fmt.Sprintf("window bounds not captured; "+tracefence.ScaleMarkEN+" report max %.3fms (fallback scale, no window %%)", model.BarMaxMS)
}

// runtimeTraceProjSeatSelfComponentRows (P2a rider 件2b, §29.58.1 b,
// 2026-07-13) reseats every SELF row that is a typed COMPONENT of another
// self row (WO-A1 carrier a: NonAdditiveKind==Component with the ref
// resolving to a sibling self row's evidence tag) to the position DIRECTLY
// under its owning seat, and stamps SubordinateComponentSeat so the renderer
// prefixes the ↳ connector. 沿折叠行裁定分工原则: 结构管关系 (placement + ↳),
// 词面管语义 (the 「为[E#]的组成部分·不可相加」 pointer tag stays). Components
// pointing outside the self stanza (chain-lane carriers b/c) are untouched.
// Deterministic: hosts keep their order; components follow their host in
// their original relative order.
// runtimeTraceProjRelocateSelfNonChainSeats (SELF-LANE §29.58.3 处置 a,
// 2026-07-13) relocates every TARGET-subject row out of the ◇ adjacent stanza
// into the self stanza: after the SELF-ALL engine promotion (§29.61.2) the
// remaining target rows there are the honest non-chain residuals — rows
// without a typed wall-clock interval or on a non-wall-clock ⌗ caliber — and
// the 邻近区段 wording promises OTHER threads competing nearby (主体非自己
// 邻居, 062104 witness). Display placement ONLY: the row keeps its node
// verbatim (channel identity, ordinal, caliber words unchanged) and gains the
// typed SelfNonChainSeat qualifier seat.
func runtimeTraceProjRelocateSelfNonChainSeats(model *runtimeTraceProjTreeModel, targetKey string) {
	if model == nil || strings.TrimSpace(targetKey) == "" || len(model.Adjacent) == 0 {
		return
	}
	kept := make([]runtimeTraceProjTreeRow, 0, len(model.Adjacent))
	for _, row := range model.Adjacent {
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != targetKey {
			kept = append(kept, row)
			continue
		}
		row.Kind = runtimeTraceProjTreeRowSelf
		row.SelfNonChainSeat = true
		model.SelfRows = append(model.SelfRows, row)
	}
	model.Adjacent = kept
}

// runtimeTraceProjMarkCrossChannelSameThread (SELF-LANE §29.58.3 处置 b,
// 2026-07-13) stamps the cross-channel same-thread mutual pointers: every
// adjacent-CHANNEL row (◇ stanza rows and relocated 非链 self rows alike —
// the channel is the node's typed relevance, not the display placement) whose
// subject also holds an on-chain seat points at that thread's LARGEST on-chain
// seat, and that seat points back at the thread's largest adjacent-channel
// seat. ONE pointer each way (largest display impact; ties by first-rendered
// order) — a roster would out-shout the accounts it annotates. Wording input
// only; every account and ordinal stays untouched.
func runtimeTraceProjMarkCrossChannelSameThread(model *runtimeTraceProjTreeModel) {
	if model == nil {
		return
	}
	type seat struct {
		rows []runtimeTraceProjTreeRow
		idx  int
		lane int
	}
	lanes := [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background}
	chainBest := map[string]seat{}
	adjBest := map[string]seat{}
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): the target-self ⌗ side-rail
	// rows (typed self_caliber_side) are NOT channel seats — they never enter
	// the chain/adjacent best maps (a 「本线程另有链上席/邻近席」 pointer naming
	// a ⌗ row would be the exact channel lie the token retires). They register
	// on their own map; the thread's chain seat points at the largest one with
	// the 口径旁栏行 word, and the ⌗ row keeps the forward 链上席 pointer —
	// same one-each-way + 双向或双无 disciplines.
	caliberBest := map[string]seat{}
	// SELF-ALL 修复轮 件4 F4 (2026-07-13): ONE eligibility predicate for BOTH
	// the collection loop and the stamping loop — the collection required
	// HasData∧EvidenceTag while the stamp only matched subjects, so a
	// tag-less ◌ blind-spot row wore 「本线程另有链上席」 while its chain seat
	// could never point back (adjBest never saw the tag-less row): a one-way
	// pointer against the 双向 promise. 双向或双无.
	eligible := func(row runtimeTraceProjTreeRow) bool {
		return row.HasData && strings.TrimSpace(row.EvidenceTag) != ""
	}
	better := func(best seat, rows []runtimeTraceProjTreeRow, i int) bool {
		if best.rows == nil {
			return true
		}
		// Prefer a RANKED seat over a context row (席 means a seat); at equal
		// seat class the larger display impact wins.
		bestRanked := best.rows[best.idx].Node.Rank > 0
		candRanked := rows[i].Node.Rank > 0
		if bestRanked != candRanked {
			return candRanked
		}
		return runtimeTraceProjNodeDisplayImpact(rows[i].Node) >
			runtimeTraceProjNodeDisplayImpact(best.rows[best.idx].Node)
	}
	for lane, rows := range lanes {
		for i := range rows {
			if !eligible(rows[i]) {
				continue
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(rows[i].Node.Subject)
			if subject == "" || subject == "unknown-thread" || subject == "unknown" {
				continue
			}
			if strings.TrimSpace(rows[i].Node.ChainRelevance) == "self_caliber_side" {
				if better(caliberBest[subject], rows, i) {
					caliberBest[subject] = seat{rows: rows, idx: i, lane: lane}
				}
				continue
			}
			switch runtimeTraceProjRowOrdinalChannel(rows[i]) {
			case runtimeTraceProjOrdinalChannelChain:
				if better(chainBest[subject], rows, i) {
					chainBest[subject] = seat{rows: rows, idx: i, lane: lane}
				}
			case runtimeTraceProjOrdinalChannelAdjacent:
				if better(adjBest[subject], rows, i) {
					adjBest[subject] = seat{rows: rows, idx: i, lane: lane}
				}
			}
		}
	}
	if len(chainBest) == 0 || (len(adjBest) == 0 && len(caliberBest) == 0) {
		return
	}
	// 同段噪声闸: the pointer's purpose is cross-STANZA linkage — a pair whose
	// two seats already render in the SAME display stanza (the relocated 非链
	// self rows beside the target's own chain seats) points at a neighbour the
	// reader is already looking at, so it stamps nothing.
	sameStanza := func(subject string) bool {
		best, okChain := chainBest[subject]
		peer, okAdj := adjBest[subject]
		return okChain && okAdj && best.lane == peer.lane
	}
	// 件② same-stanza noise gate for the caliber pair (same rationale as the
	// chain/adjacent gate above).
	sameStanzaCaliber := func(subject string) bool {
		best, okChain := chainBest[subject]
		peer, okCal := caliberBest[subject]
		return okChain && okCal && best.lane == peer.lane
	}
	for lane, rows := range lanes {
		for i := range rows {
			if !eligible(rows[i]) {
				continue // F4: 双向或双无 — the stamp shares the collection predicate
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(rows[i].Node.Subject)
			if subject == "" {
				continue
			}
			if strings.TrimSpace(rows[i].Node.ChainRelevance) == "self_caliber_side" {
				// 件②: the ⌗ side-rail row keeps the forward 链上席 pointer
				// (accurate — the peer IS a chain seat).
				if sameStanzaCaliber(subject) {
					continue
				}
				if best, ok := chainBest[subject]; ok && best.lane != lane {
					if tag := strings.TrimSpace(best.rows[best.idx].EvidenceTag); tag != "" &&
						tag != strings.TrimSpace(rows[i].EvidenceTag) {
						rows[i].CrossChannelChainRef = tag
					}
				}
				continue
			}
			if sameStanza(subject) && runtimeTraceProjRowOrdinalChannel(rows[i]) == runtimeTraceProjOrdinalChannelAdjacent {
				continue
			}
			switch runtimeTraceProjRowOrdinalChannel(rows[i]) {
			case runtimeTraceProjOrdinalChannelAdjacent:
				if best, ok := chainBest[subject]; ok && best.lane != lane {
					if tag := strings.TrimSpace(best.rows[best.idx].EvidenceTag); tag != "" &&
						tag != strings.TrimSpace(rows[i].EvidenceTag) {
						rows[i].CrossChannelChainRef = tag
					}
				}
			case runtimeTraceProjOrdinalChannelChain:
				// Only the thread's LARGEST on-chain seat carries the reverse
				// pointer (one pointer each way).
				best, ok := chainBest[subject]
				if !ok || strings.TrimSpace(rows[i].EvidenceTag) == "" ||
					best.rows[best.idx].EvidenceTag != rows[i].EvidenceTag {
					continue
				}
				if peer, okAdj := adjBest[subject]; okAdj && !sameStanza(subject) && peer.lane != lane {
					if tag := strings.TrimSpace(peer.rows[peer.idx].EvidenceTag); tag != "" &&
						tag != strings.TrimSpace(rows[i].EvidenceTag) {
						rows[i].CrossChannelAdjacentRef = tag
					}
				}
				// 件②: the reverse pointer at the ⌗ side rail — the channel
				// word would lie, so the dedicated 口径旁栏行 template speaks.
				if peer, okCal := caliberBest[subject]; okCal && !sameStanzaCaliber(subject) && peer.lane != lane {
					if tag := strings.TrimSpace(peer.rows[peer.idx].EvidenceTag); tag != "" &&
						tag != strings.TrimSpace(rows[i].EvidenceTag) {
						rows[i].CrossChannelCaliberRef = tag
					}
				}
			}
		}
	}
}

func runtimeTraceProjSeatSelfComponentRows(model *runtimeTraceProjTreeModel) {
	rows := model.SelfRows
	if len(rows) < 2 {
		return
	}
	tags := map[string]bool{}
	for i := range rows {
		if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" {
			tags[tag] = true
		}
	}
	componentsByHost := map[string][]int{}
	componentIdx := map[int]bool{}
	for i := range rows {
		if rows[i].NonAdditiveKind != runtimeTraceProjNonAdditiveComponent {
			continue
		}
		ref := strings.TrimSpace(rows[i].NonAdditiveRef)
		if ref == "" || !tags[ref] || ref == strings.TrimSpace(rows[i].EvidenceTag) {
			continue
		}
		componentsByHost[ref] = append(componentsByHost[ref], i)
		componentIdx[i] = true
	}
	if len(componentsByHost) == 0 {
		return
	}
	out := make([]runtimeTraceProjTreeRow, 0, len(rows))
	seated := make([]bool, len(rows))
	for i := range rows {
		if seated[i] || componentIdx[i] {
			continue
		}
		out = append(out, rows[i])
		seated[i] = true
		for _, j := range componentsByHost[strings.TrimSpace(rows[i].EvidenceTag)] {
			row := rows[j]
			row.SubordinateComponentSeat = true
			out = append(out, row)
			seated[j] = true
		}
	}
	// Safety net: a component whose host row never emitted (unreachable while
	// tags[] gates membership above) keeps its seat instead of vanishing.
	for i := range rows {
		if !seated[i] {
			out = append(out, rows[i])
		}
	}
	model.SelfRows = out
}

// runtimeTraceProjSelfRowLines renders one self row for the fence. PTV4 T1 →
// PTV6-D (a) 悬崖消除: the whole row stays a single line whenever it holds
// the 100-cell row cap; over the cap, the main line keeps the essentials
// (state glyph + state/name + value + ⊘ marker + [E#]) PLUS the leading
// detail parts that still fit inline (prefix fill, legacy order before the
// trailing [E#]), and the overflow parts PACK into "· " subordinate detail
// lines — nothing is elided (the FitTags drop lane stays retired).
func runtimeTraceProjSelfRowLines(row runtimeTraceProjTreeRow, windowMS float64, zh bool) []string {
	lead := "│     "
	// §29.58.5 ① (user 精化裁定, 2026-07-13): a reseated COMPONENT row indents
	// ONE LEVEL DEEPER than its owning seat — the ↳ connector falls into the
	// indent position (从宿主分支义) and the row wears a SINGLE form mark (⋈).
	// The pre-ruling form put ↳ at the host mark column, so the two 2ch glyph
	// envelopes sat side by side and read as a double icon (133136 witness
	// 「↳ ⋈ 自身·binder」). The row is distinguished from "· " side notes at
	// the same depth family by its prefix glyph (与旁注靠前缀字形区分).
	//
	// EVOLUTION RECORD (§29.58.5b 终裁, user 2026-07-13, 225901 工件复核后):
	// the component row's form evolved across the rulings — ◦ 同级行 → ↳⋈
	// 同级双记号 (P2a 件2b) → 深缩进+↳+⋈ (this arm). A retirement of ↳ (deep
	// indent as the sole structure encoding) was considered and REVERSED by
	// the final ruling: deep-indent + ↳ + ⋈ DOUBLE encoding of the
	// containment structure is the deliberate terminal form (结构管关系(缩进+
	// 连接符)/记号管形态(⋈)/词面管语义(为[E#]的组成部分)). The
	// optimization-table ↳ member/fold cells remain a separate, unrelated
	// semantics (P2a 件4).
	if row.SubordinateComponentSeat {
		lead += "  "
	}
	// SELF-ALL (§29.61.2/§29.61.2a, 2026-07-13): a self row that IS a ranked
	// cause node renders the SAME cause grammar as every chain row — 行2
	// identity (类别·[自身·墙钟席·]根因排序#N·置信), 行3 「=」breakdown and
	// the 拆解子行 as dedicated note lines (同形纪律: ONE composer —
	// runtimeTraceProjCauseStructuredParts — two stanzas; the witness form is
	// the promoted ◇→self wall-clock seat wearing its chain-channel ordinal).
	// Non-cause self rows keep the legacy single-line/packed form
	// byte-identically.
	main, structuredLines, demoted, identityGroups := runtimeTraceProjSelfRowParts(row, windowMS, zh)
	// legacy layout order for the single-line form: essentials interleave with
	// the detail parts exactly where they were built; the E# ref stays last.
	single := lead + strings.Join(runtimeTraceProjSelfInlineOrder(main, demoted), " ")
	if len(structuredLines) == 0 && (len(demoted) == 0 || runewidth.StringWidth(single) <= runtimeTraceProjTreeRowMaxWidth) {
		return []string{single}
	}
	keep := 0
	for keep < len(demoted) {
		trial := lead + strings.Join(runtimeTraceProjSelfInlineOrder(main, demoted[:keep+1]), " ")
		if runewidth.StringWidth(trial) > runtimeTraceProjTreeRowMaxWidth {
			break
		}
		keep++
	}
	if len(structuredLines) > 0 {
		// The cause grammar owns rows 2..N — every detail part demotes below
		// it (the identity line always sits directly under 行1, tree-row
		// discipline).
		keep = 0
	}
	lines := []string{lead + strings.Join(runtimeTraceProjSelfInlineOrder(main, demoted[:keep]), " ")}
	// P2a rider 件2a (§29.58.1 a, 2026-07-13): the "· " side-note block
	// indents ONE LEVEL DEEPER than its host row (aligned with the note's own
	// wrapped continuations) — the self stanza has no rails/edges, so a note
	// at the host's own lead sat in the very glyph column that separates the
	// state rows (三层级一视觉形 witness 20260713-062104).
	for i, text := range structuredLines {
		// DISPLAY-WRAP 件①(c): the identity line (always structuredLines[0])
		// splits by its semantic groups under width pressure.
		if i == 0 && len(identityGroups) > 1 {
			lines = append(lines, runtimeTraceProjIdentityGroupLines(lead+"  ", identityGroups, zh)...)
			continue
		}
		lines = append(lines, runtimeTraceProjSubordinateLines(lead+"  ", text)...)
	}
	return append(lines, runtimeTraceProjSubordinatePackedLines(lead+"  ", demoted[keep:])...)
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

// runtimeTraceProjSelfRowParts builds a self row's essential (main-line) parts,
// its cause-grammar lines (SELF-ALL §29.61.2/§29.61.2a: rows 2..N of a ranked
// self cause node — the SAME composer as every chain row, 同形纪律) and its
// demotable detail parts (PTV4 T1 split). structured is nil for non-cause
// rows (their layout is byte-identical to the pre-SELF-ALL form).
// The 4th return (DISPLAY-WRAP 件①(c)) carries the 行2 identity line's
// semantic groups when the board chip rides it (nil otherwise) — the stanza
// renderer splits the identity line at group boundaries under width pressure.
func runtimeTraceProjSelfRowParts(row runtimeTraceProjTreeRow, windowMS float64, zh bool) (mainParts, structuredLines, demotedParts, identityGroups []string) {
	node := row.Node
	var main, demoted []string
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	// A runnable self row names the focused thread's scheduler state, not the
	// causal object token (runnable_wait). Use the shared state label on both
	// languages so the compact tree reads 自身·runnable / own·runnable.
	// (UX-1 修复轮 2026-07-15: the former nameIsStateWord flag retired — the
	// state-tag dedupe arm below keys on the name match itself.)
	//
	// DISPLAY-WRAP 件④(b) (§29.104.18.1 A4, 2026-07-16): the override applies
	// ONLY when the display name IS the raw causal token (the unmapped
	// fallback the override was built for) — a mapped TYPE word stays on the
	// head, 与表同名 (the witness rendered three indistinguishable
	// 「⧖ 自身·runnable」 heads for 优先级反转·可运行等待 / 调度延迟 /
	// runnable rows; the (a) table distinguished them all along). The
	// runnable_wait row keeps its byte-identical 自身·runnable head (its
	// mapped label IS the state word).
	if strings.EqualFold(strings.TrimSpace(node.StateKind), "runnable") &&
		(name == "" || name == strings.TrimSpace(node.Object)) {
		if stateName := strings.TrimSpace(runtimeTraceProjStateKindLabel(node, zh)); stateName != "" {
			name = stateName
		}
	}
	// SELF-ALL (§29.61.2/§29.61.2a): compute the cause grammar ONCE (marks are
	// emission-counted; a second call would skew the legend frequency order).
	var structured runtimeTraceProjCauseStructured
	structuredOK := false
	if structured, structuredOK = runtimeTraceProjCauseStructuredParts(row, zh); structuredOK {
		structuredLines = append(structuredLines, structured.IdentityRow)
		identityGroups = structured.IdentityGroups
		if structured.Breakdown != "" {
			structuredLines = append(structuredLines, structured.Breakdown)
		}
		structuredLines = append(structuredLines, structured.SubRows...)
		// RNB-5B 默认小件a (§29.95 UX-2 「最大席最寡言」, 2026-07-15): the
		// SELF running-deficit seat's stanza block carries the SAME
		// single-source supply-fold mechanism clause the flat/chain rows wear
		// (R5b 运行频点非最高 mention + THERM thermal-press sentence,
		// runtimeTraceProjSupplyFoldClause — 与平铺面同源同词). The self
		// composer never reaches the metric-parts tag site, so the biggest
		// seat used to be the only fold seat WITHOUT its mechanism sentence.
		// Same legend-mark discipline as that site's non-inversion branches.
		if runtimeTraceProjCauseRunningDeficitArm(node) {
			if clause, _, ok := runtimeTraceProjSupplyFoldClause(node, windowMS, zh); ok {
				structuredLines = append(structuredLines, clause)
				switch runtimeTraceProjSupplyFoldVerdictFor(node, windowMS) {
				case runtimeTraceProjSupplyFoldTriple, runtimeTraceProjSupplyFoldWithDemand, runtimeTraceProjSupplyFoldDominant:
					row.marks.mark(runtimeTraceProjMarkCaliberGlobalMaxFmax)
					row.marks.mark(runtimeTraceProjMarkCaliberLowerBound)
					if capMark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource); ok {
						row.marks.mark(capMark)
					}
				}
			}
		}
	}
	selfPrefix := "自身·"
	selfOnly := "自身"
	if !zh {
		selfPrefix = "own·"
		selfOnly = "own"
	}
	// §29.27.1 (徽章跟随席位, 2026-07-11): a self row holding a TOP-5 seat
	// (自因四态 participant — e.g. the textup_792 tree-head IO rows on seats
	// #2/#3) wears its ❶..❺ glyph before the state glyph, exactly like every
	// other render surface. Wait-symptom self rows are seatless (Rank=0 +
	// tier defense) and stay bare by construction.
	badge := runtimeTraceProjBadgeGlyph(row.Badge)
	if badge != "" {
		row.marks.mark(runtimeTraceProjMarkBadge)
		// UXG-0 D5: badge→state-glyph gap, same as the tree/stanza emitters.
		badge += " "
	}
	// CAL-1 件⑤ PACE-ROW (§29.47.4②, 2026-07-12): a typed cadence-idle row
	// (standalone idle rank row OR the R1 survivor that adopted the idle
	// view's TypeToken) stands as its OWN self row — 帧间空闲 is semantically
	// alien to the 等依赖 sleep family and folding them diluted both (the
	// engine already minted the independent row; the display no longer folds
	// it in — the ×N family exclusion lives in the R2 group key). Row 1
	// leads with the dedicated cadence glyph + the idle type word; 行2 mints
	// 「节拍吻合·上下文(不参与根因排序)」 below.
	idleKind := runtimeTraceProjIdleRowKind(node)
	if idleKind != "" {
		idleName := idleKind
		if zh {
			if label := runtimeTraceRootCauseTypeZHLabel(idleKind); label != "" {
				idleName = label
			}
		}
		if idleKind == "periodic_idle" {
			row.marks.mark(runtimeTraceProjMarkPeriodicIdle)
		} else {
			row.marks.mark(runtimeTraceProjMarkPacingIdle)
		}
		main = append(main, badge+runtimeTraceProjStateIcon(node, row.Kind, true, row.marks)+" "+selfPrefix+idleName)
	} else if node.IsSleepState() {
		// PTV6-C #8 (#73, 标本归因 2026-07-06): the sleep self row speaks the
		// 裁定4 StateKindLabel (PTV7: ☾ sleep, the canonical display word)
		// instead of the raw scheduler token (☾ s_sleep) — the raw token stays
		// lossless on the (b) detail block's full name / 类型 lanes and the
		// evidence surfaces. An unmapped state token keeps its verbatim form
		// (labels are never fabricated).
		label := runtimeTraceProjStateKindLabel(node, zh)
		if label == "" {
			label = strings.TrimSpace(node.StateKind)
		}
		if label == "" {
			label = "sleep"
		}
		row.marks.mark(runtimeTraceProjMarkIconSleep)
		main = append(main, badge+tracefence.GlyphSleep+" "+selfPrefix+label)
	} else if name != "" {
		// NEW-10 (§7.6 记号区规整): every self row leads with exactly one state
		// glyph (sleep rows already carry ☾) — a constant one-glyph slot keeps
		// same-depth rows aligned on proportional web fonts.
		main = append(main, badge+runtimeTraceProjStateIcon(node, row.Kind, true, row.marks)+" "+selfPrefix+name)
	} else {
		main = append(main, badge+runtimeTraceProjStateIcon(node, row.Kind, true, row.marks)+" "+selfOnly)
	}
	// P2a rider 件2b (§29.58.1 b, 2026-07-13): a reseated component row (↳
	// seat stamped by runtimeTraceProjSeatSelfComponentRows) leads with the
	// subordinate connector in the badge-family slot — 结构管关系; the
	// containment word face stays on the WO-A1 pointer tag below.
	if row.SubordinateComponentSeat && len(main) > 0 {
		row.marks.mark(runtimeTraceProjMarkSubordinateComponent)
		main[0] = tracefence.GlyphSubordinate + " " + main[0]
	}
	// WO-D1① 无值披露臂 (SMR-1 批 SMR-S2 ◌ 带值行, 2026-07-12; 25846 E4:
	// the missing_wakeup ◌ row re-carried a sleep member's 80.751ms as what
	// read like a fourth account): an undrillable self row whose value is a
	// PROVEN member of another seat (WO-A1 member pointer) keeps its ⊘链止
	// honesty seat but drops the duplicated ms from 行1 — the value lives in
	// the referenced seat's account and the 不可相加·为[E#]成员 tag says so
	// (吸收=佩记号, 数值不重计; the observation itself stays lossless).
	if v := runtimeTraceProjNodeDisplayImpact(node); v > 0 &&
		!(node.Undrillable() && row.NonAdditiveKind == runtimeTraceProjNonAdditiveMember) {
		value := fmt.Sprintf("%.3fms", v)
		// SELF-ALL/SELF-LANE (2026-07-13): the self 行1 value speaks the SAME
		// caliber words as the stanza value cell — a count-class magnitude must
		// never print as bare wall-clock ms (G3/DISP-2 计数当量 discipline), a
		// composite score never as a duration, and an engine family total wears
		// its fold stem (合计/成员最大 — the tree-row 行1 convention).
		switch {
		case runtimeTraceProjCompositeValueCaliber(node):
			// Typed value caliber is independent of row placement. This covers
			// both M18 context-only io_pressure and legacy block_io rows.
			value = runtimeTraceProjCompositeScoreValueText(v, zh)
		case tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount:
			// §29.55 观察③ 两形一裁 (2026-07-14): the 行1 form 计数当量Xms is
			// retired — the count-equivalent value never wears an ms suffix;
			// ONE form via the shared helper.
			row.marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
			value = runtimeTraceProjCountEquivalentValueText(v, zh)
		default:
			if prefix := runtimeTraceProjFamilyValuePrefix(node, zh); prefix != "" {
				value = prefix + value
			}
		}
		main = append(main, value)
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels (PTV4
	// T4 ×N 三式: data inline, semantics in the legend's 口径组).
	// UXR-1 §29.36④: the ×N同值 chip rides the name cell (孤行灭, same form
	// as the detail table) — main[0] gains the chip; no demoted orphan.
	if node.DuplicatePublications > 1 {
		row.marks.mark(runtimeTraceProjMarkMergedDedup)
		if len(main) > 0 {
			main[0] += " " + runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)
		}
	}
	if node.MergedCount > 1 {
		// §11-N2: the union caliber wears its own form token — the sum form's
		// legend entry claims 数值为总和 and must stay truthful. §21 CWD: the
		// cross-window MAX caliber likewise wears its own form (第五式).
		if node.MergedIntervalUnion {
			row.marks.mark(runtimeTraceProjMarkMergedUnion)
			if runtimeTraceProjMergedValuelessWordRenders(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers) // G12-ENG 复核 P2-1 连带
			}
			demoted = append(demoted, runtimeTraceProjMergedUnionTagText(node, zh))
		} else if node.MergedCrossWindowMax {
			row.marks.mark(runtimeTraceProjMarkMergedWindowMax)
			if runtimeTraceProjMergedValuelessWordRenders(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers) // G12-ENG (§29.1 + 复核 P1-1)
			}
			demoted = append(demoted, runtimeTraceProjMergedCrossWindowMaxTagText(node, zh))
		} else {
			// G12-ENG 复核 P2-2: the all-valueless R2 row wears NO ×N(a~b) sum
			// notation (nothing summed), so the sum legend entry must not
			// render for it — same fork discipline as the G19 all-zero fold.
			if runtimeTraceProjMergedAllValueless(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
			} else {
				row.marks.mark(runtimeTraceProjMarkMergedSum)
				if runtimeTraceProjMergedValuelessWordRenders(node) {
					row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
				}
			}
			demoted = append(demoted, runtimeTraceProjMergedSumTagText(node, zh))
		}
	}
	// 裁定4 applies to the target's own status rows too (lock_001 customer
	// report, 2026-07-03); sleep rows keep their dedicated wording below;
	// cadence-idle rows carry their own row-2 word (below) instead of a
	// state tag.
	wordless := !node.IsSleepState() && idleKind == ""
	if !node.IsSleepState() && idleKind == "" {
		stateTag := runtimeTraceProjStateKindLabel(node, zh)
		genericShape := false
		if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
			runtimeTraceCausalProjectionInversionRow(node) {
			stateTag, genericShape = runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
		}
		switch {
		case stateTag == "":
		case genericShape:
			// PTV6-D (b): the generic category word leaves the self row too —
			// same typed branch signal + legend carrier as the tree rows.
			row.marks.mark(runtimeTraceProjMarkCandidateShapeClass)
		case structuredOK && structured.SuppressShapeWord:
			// SELF-ALL (§29.61.2) — the tree-row §24.2 行尾形态词撤 mirrored:
			// the shape word RELOCATED onto the 行2 category slot of the cause
			// grammar this self row now renders; re-tagging it here would
			// double-speak (typed branch relocation, never a string dedupe).
			wordless = false
		case structuredOK && stateTag == name:
			// SELF-ALL (§29.61.2): the 行1 name already speaks the state word
			// (自身·runnable) and the cause grammar names the category on 行2 —
			// the bare state tag would be a third same-word seat.
			// ELIM-SELF-FIX 修复轮 UX-1 (冷读官, 2026-07-15): the arm keys on
			// the NAME MATCH itself, no longer on the runnable-only
			// nameIsStateWord flag — the self running fold seat's 行1 is
			// 自身·running via the generic cause-name lane, and its trailing
			// bare 「· running」 orphan carried zero information.
			wordless = false
		case row.SelfNonChainSeat && name != "" && strings.Contains(name, stateTag):
			// SELF-LANE (§29.58.3 a): a relocated row's 行1 name is the SAME
			// composed cause word (自身·页缓存抖动) — re-tagging the shape word
			// beside it would double-speak (display-face dedupe, cosmetic only;
			// the ⌗ caliber word below stays).
			wordless = false
		default:
			row.marks.mark(runtimeTraceProjMarkStateLabel)
			demoted = append(demoted, stateTag)
			wordless = false
		}
	}
	// DISPHYG-3 件6 (§29.158 P3-2): the two-ruler sentence's non-lead
	// participant compact row wears its board-ordinal cross-reference chip
	// (same composer as the stanza tag-rail site — one word face).
	if chip := runtimeTraceProjSelfTwoRulerParticipantChip(row, zh); chip != "" {
		demoted = append(demoted, chip)
	}
	// SELF-LANE (§29.58.3 处置 a, 2026-07-13): a relocated non-chain self row
	// wears the 「非链」 qualifier FIRST — the self stanza sits inside the
	// chain-universe display, and this row's channel identity is honest
	// adjacent (placement moved, proof did not). The ⌗ caliber-side word
	// keeps stanza-face parity (the ◇ renderer's V2-P0 arm).
	//
	// RNB-5B 件② (§29.96.2 终判②, 2026-07-15): a target-self row carrying the
	// NON-CHANNEL self_caliber_side wire token no longer claims any channel
	// word — the 「非链」 qualifier (a channel-identity statement about an
	// adjacent-lane row) is replaced by the ⌗ caliber word alone: the row IS
	// the 口径旁栏 lane, R8 keeps it out of ◇ semantics, and §29.83 keeps its
	// magnitude off the causal channels. Legacy adjacent-relevance relocated
	// rows keep the 非链 word byte-identically.
	if row.SelfNonChainSeat {
		if strings.TrimSpace(node.ChainRelevance) != "self_caliber_side" {
			row.marks.mark(runtimeTraceProjMarkSelfNonChainSeat)
			if zh {
				demoted = append(demoted, "非链")
			} else {
				demoted = append(demoted, "non-chain")
			}
		}
		if node.IsCaliberSideRow() {
			row.marks.mark(runtimeTraceProjMarkCaliberSideRow)
			if tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount {
				row.marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
			}
			demoted = append(demoted, runtimeTraceProjCaliberSideWord(node, zh))
		}
	}
	// 修复轮二 件B (2026-07-13): the proven wait object discloses on the D/IO
	// self row inline — with no rank-family seat in the ledger this row is
	// the ONLY carrier of the refined proof, and the 等待对象 word must not
	// depend on the dispatch shape (same engine-typed symbol as the rcr
	// identity line; absent = absent).
	if caller := strings.TrimSpace(node.BlockedReasonCaller); caller != "" {
		if zh {
			demoted = append(demoted, "等待对象 "+caller)
		} else {
			demoted = append(demoted, "wait object "+caller)
		}
	}
	if idleKind != "" {
		// CAL-1 件⑤ 行2: the cadence-fit context word — 「节拍吻合」 is the
		// typed mint-condition wordface (see runtimeTraceProjIdleRowKind);
		// the context half keeps the row out of ranking wording-wise too
		// (the tier defense already keeps it seatless).
		if zh {
			demoted = append(demoted, "节拍吻合·上下文(不参与根因排序)")
		} else {
			demoted = append(demoted, "cadence fit · context (not ranked)")
		}
	} else if node.IsSleepState() {
		// PTV5 C03 (#68): "主要" is a share claim — it renders only when the
		// sleep projection actually covers ≥50% of the window (precise
		// ImpactMS-vs-windowMS comparison); smaller shares and windowless
		// renders keep the neutral segment wording.
		if windowMS > 0 && node.ImpactMS >= windowMS*0.5 {
			if zh {
				demoted = append(demoted, "窗口内主要处于等待唤醒")
			} else {
				demoted = append(demoted, "mostly waiting for wakeup inside the window")
			}
		} else if zh {
			demoted = append(demoted, "该段处于等待唤醒")
		} else {
			demoted = append(demoted, "waiting for wakeup in this segment")
		}
	}
	// ENG-2 (复核冷读 CP1-③, 2026-07-12) — EVOLUTION RECORD (CAL-1 件⑤,
	// 2026-07-12): the 「其中 …」 annotation arm is RETIRED as the primary
	// carrier on self rows and demoted to the FOLD FALLBACK — a typed
	// cadence-idle row now stands alone (row 1 speaks the idle word, the
	// annotation would double-speak), so the tag renders only on rows that
	// still carry an absorbed idle share without the idle row identity
	// (e.g. a survivor whose own TypeToken blocked the adoption).
	if text, mark, ok := runtimeTraceProjIdleCadenceTag(node, zh); ok && idleKind == "" {
		row.marks.mark(mark)
		demoted = append(demoted, text)
	}
	// CR-2 组② P5: the same-segment mirror tags on the self-row face (the
	// 14704 witness pair is a target self row) — shared wording/mark helper
	// with the tree/stanza face.
	demoted = append(demoted, runtimeTraceProjSameSegMirrorTagTexts(row, zh)...)
	if node.Undrillable() {
		// PTV5 C06 (#68): bare ⊘链止, matching the tree-row form — the typed
		// UndrillableReason enum stays off the panel (semantics live in the
		// legend's ⊘ entry; the raw enum keeps its (a)-table ⊘ legend home).
		// PTV8-RCR-B (UXA 域A #30 REVISE 补主语稿, 2026-07-08): a self row that
		// otherwise carries NO descriptive word states the missing_wakeup fact
		// inline (无唤醒记录·⊘链止) — the second same-value sibling used to
		// read as a second unexplained account. Exact typed reason match;
		// worded rows keep the bare marker byte-identically.
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		tag := tracefence.GlyphUndrillable + "链止"
		if !zh {
			tag = tracefence.GlyphUndrillable + "chain ends"
		}
		if wordless && strings.TrimSpace(node.UndrillableReason) == "missing_wakeup" {
			if zh {
				tag = "无唤醒记录·⊘链止"
			} else {
				tag = "missing_wakeup·⊘chain ends"
			}
		}
		main = append(main, tag)
	}
	// GAP-B G11 (§27.5, 2026-07-09): a relocated wait-symptom rank row renders
	// with the sleep-row family's symptom disclosure — 症状身份, never a rank
	// seat (the row's rank ordinal stays on the audit surfaces; the board
	// lanes already exclude target_self_state rows).
	if row.SelfSymptomRelocated || len(row.SelfSymptomFoldPeers) > 0 {
		if zh {
			demoted = append(demoted, "症状而非根因,根因看下钻/唤醒子行与对端")
		} else {
			demoted = append(demoted, "symptom, not the root cause — see the drill/wake rows and the counterpart")
		}
	}
	// NEW-3: a fold whose primary landed on the target's own state lane still
	// carries the caliber note (the peers' only display carrier).
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		demoted = append(demoted, runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh))
	}
	// PTV8-RCR-A (§24.2) — EVOLUTION RECORD (SELF-ALL §29.61.2, 2026-07-13):
	// the RankFoldPeers identity arm is SUBSUMED — a fold-peer self row is a
	// cause node (runtimeTraceProjCauseNodeRow admits len(RankFoldPeers)>0),
	// so the full cause grammar above already renders its identity line (plus
	// breakdown/sub-rows it previously lacked); a second identity append here
	// would double-render 行2.
	// DISPLAY-WRAP 件④(a) (§29.104.18.1 A3, 2026-07-16): the PTV5 Q1
	// 有效归因常显 lane reaches the SELF stanza — a ranked self cause row
	// whose 行3 failed open (family Σ ≠ engine effective at print precision;
	// witness E6: family 合计4.991ms beside the engine's published 2.116ms)
	// rendered NO effective anywhere on its tree rows while the (a) table
	// published the value. Same gates as the tree-row lane (value>0, not
	// periodic/inherited, 行1 not already the effective lane, 行2/行3 did not
	// consume it); the caliber stays unstated — the typed payload carries
	// none and absence never guesses (the metrics-table glossary remains the
	// caliber pointer).
	if structuredOK && !structured.ConsumedEffective &&
		node.EffectiveImpactMS > 0 && !node.PeriodicSource &&
		!runtimeTraceProjEffectiveInherited(node) {
		if _, impactSource := runtimeTraceProjNodeDisplayImpactSource(node); impactSource != runtimeTraceProjImpactSourceEffective {
			row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
			text := fmt.Sprintf("有效归因 %.3fms", node.EffectiveImpactMS)
			if !zh {
				text = fmt.Sprintf("attribution %.3fms", node.EffectiveImpactMS)
			}
			if word, ok := runtimeTraceProjOccurrenceSegmentAccountWord(node, zh); ok {
				text += word
				row.marks.mark(runtimeTraceProjMarkOccurrenceSegmentAccount)
			} else if word, ok := runtimeTraceProjBareEffectiveCaliberBeltWord(node, row.marks, zh); ok {
				// GATED-CAL 件1④ (§29.104.16.1 M3, 2026-07-16): the SELF-stanza
				// twin of the tree-row belt — same !ConsumedEffective premise,
				// same typed-producer floor word (the A3 lane's 宁缺勿造 stays
				// for every row OUTSIDE the two typed-producer arms).
				text += word
			}
			demoted = append(demoted, text)
		}
	}
	if ref := runtimeTraceProjCauseEvidenceRef(row); ref != "" {
		main = append(main, ref)
	}
	// DISPLAY-WRAP 件③(a): same-node caliber-phrase repeat suppression, in
	// stanza display order (grammar lines 行2..N, then the demoted parts).
	var dedupTexts []*string
	for i := range structuredLines {
		dedupTexts = append(dedupTexts, &structuredLines[i])
	}
	for i := range demoted {
		dedupTexts = append(dedupTexts, &demoted[i])
	}
	runtimeTraceProjDedupNodeCaliberPhrases(dedupTexts, row.marks, zh)
	return main, structuredLines, demoted, identityGroups
}

// runtimeTraceProjAncestorRails renders a row's ancestor rails with the
// PTV8-LAD L4 indent cap (§24.11 维度A / AL3): at most the DEEPEST
// runtimeTraceProjTreeIndentCap levels draw their 4-cell rails; deeper rows
// collapse the shallower levels into one fixed 2-cell "⋯ " leader (EAW-
// verified single-cell glyph), so the fixed lead of main rows AND subordinate
// lines is bounded and the name/payload budgets stop degrading with depth.
// The ONE builder is shared by runtimeTraceProjTreePrefix and
// runtimeTraceProjRowContinuationIndent — the two faces can never drift.
func runtimeTraceProjAncestorRails(ancestors []bool) string {
	var b strings.Builder
	if over := len(ancestors) - runtimeTraceProjTreeIndentCap; over > 0 {
		b.WriteString("⋯ ")
		ancestors = ancestors[over:]
	}
	for _, more := range ancestors {
		if more {
			b.WriteString("│   ")
		} else {
			b.WriteString("    ")
		}
	}
	return b.String()
}

func runtimeTraceProjTreePrefix(row runtimeTraceProjTreeRow) string {
	var b strings.Builder
	b.WriteString(runtimeTraceProjAncestorRails(row.Ancestors))
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
	case runtimeTraceProjTreeEdgeChainUnresolved:
		// PTV6 #1b (v3 §5): the depthless lane's dedicated edge — never the
		// default wake word (恢复硬编码唤醒边必红 pin reads this arm).
		//
		// EVOLUTION RECORD (UXR-1 §29.36④, 2026-07-11): the 深度未解析 /
		// 父节点未确认 auxiliary words LEFT the lane prefix (lane 前缀简化,释放
		// 关键行宽度) — the edge claims exactly the on-chain membership, and
		// the auxiliary word rides 行2 through the chain-layer chip family
		// (链上L#(父节点未确认) / 链上·深度未解析 — the C6 word family keeps its
		// chip + detail faces; only the edge face simplified).
		if zh {
			return "链上─"
		}
		return "on-chain─"
	default:
		if zh {
			return "唤醒─"
		}
		return "wakes─"
	}
}

// runtimeTraceProjDepthlessUnattachedRow (PTV8-RCR-C, §24.12 C6, 2026-07-08)
// is the depth-KNOWN depthless shape: the engine resolved the row's chain
// layer (Depth>0) but the tree found no attach point. The old three surfaces
// forked on it (edge 深度未解析 / chip 链上L1 / detail 深度1(未接入链)) —
// every surface now reads the ONE 父节点未确认 word family through the helpers
// below. Flat renders keep their own header-explained wording (CMP-7a).
func runtimeTraceProjDepthlessUnattachedRow(row runtimeTraceProjTreeRow) bool {
	return row.Kind == runtimeTraceProjTreeRowDepthless && row.Depth > 0 && !row.FlatChain
}

// runtimeTraceProjChainDepthChipEligible is the ONE gate shared by the Seg-20
// chip site and the 行2 identity builder (§24.9 G3 收编 — two diverging gates
// could double- or zero-print the layer): chain-lane rows with a resolved
// depth only; flat renders never claim 链上 (CMP-7a).
//
// UXR-1 §29.36④ (2026-07-11): the depth-UNRESOLVED depthless shape joins the
// gate — its 深度未解析 auxiliary word left the lane prefix (edge simplified
// to 链上─) and needs its 行2 seat, exactly where the unattached variant's
// word already lives.
func runtimeTraceProjChainDepthChipEligible(row runtimeTraceProjTreeRow) bool {
	if row.FlatChain {
		return false
	}
	if row.Kind == runtimeTraceProjTreeRowDepthless && row.Depth <= 0 {
		// Depth-unresolved arm: only rows actually drawing the simplified
		// 链上─ edge relocate the 深度未解析 word here — own-caliber /
		// overflow-fold depthless rows never claimed it.
		return row.Edge == runtimeTraceProjTreeEdgeChainUnresolved && !row.Node.OnChainOverflowFold
	}
	return row.Depth > 0 &&
		(row.Kind == runtimeTraceProjTreeRowChain || row.Kind == runtimeTraceProjTreeRowDepthless)
}

// runtimeTraceProjChainDepthChipWord renders the layer word — the §24.12 C6
// single source consumed by the Seg-20 chip, the 行2 identity join and the
// detail 层级 cell (三面同词).
func runtimeTraceProjChainDepthChipWord(row runtimeTraceProjTreeRow, zh bool) string {
	if runtimeTraceProjDepthlessUnattachedRow(row) {
		if zh {
			return fmt.Sprintf("链上L%d(父节点未确认)", row.Depth)
		}
		return fmt.Sprintf("chain L%d (parent unconfirmed)", row.Depth)
	}
	if row.Kind == runtimeTraceProjTreeRowDepthless && row.Depth <= 0 {
		// UXR-1 §29.36④: the depth-unresolved word relocated off the lane
		// prefix onto this 行2 seat (same C6 word family, new face).
		if zh {
			return "链上·深度未解析"
		}
		return "on-chain · depth unresolved"
	}
	if zh {
		return fmt.Sprintf("链上L%d", row.Depth)
	}
	return fmt.Sprintf("chain L%d", row.Depth)
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
	// CAL-1 件⑥b (2026-07-12): a typed cadence-idle row wears its dedicated
	// glyph on every surface — the row's semantic is 正常节拍空闲, not the
	// underlying sleep state and not a neutral transit hop.
	if runtimeTraceProjIdleRowKind(node) != "" {
		marks.mark(runtimeTraceProjMarkIconPacing)
		return tracefence.GlyphPacing
	}
	// RNB-5B 修复轮 D2 (冷读偏离, 主会话默认裁定 2026-07-15): a ⌗ 口径旁栏 row
	// wears its OWN row-head glyph — the impact-form table dropped the
	// self_caliber_side count row into the ⛓ arm (a channel claim plus an
	// uninterruptible-wait misread on a count-equivalent row), and the ◇-lane
	// caliber rows' ⧗ was a scheduler-state claim the same way. ⌗ matches the
	// row's own word family (⌗口径旁栏): no scheduler state, no channel
	// asserted. Typed family gate — the same tier/registry pair every ⌗
	// surface reads.
	if node.IsCaliberSideRow() ||
		tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) != tracequery.CausalCaliberSideNone {
		marks.mark(runtimeTraceProjMarkIconCaliberSide)
		return "⌗"
	}
	// PTV8-RCR-A §24.3 (2026-07-08). EVOLUTION RECORD: the glyph now resolves
	// through the single-source impact-form table (状态 icons kept their
	// glyphs and marks; lock/inversion/interrupt/blind-spot/IO-event rows left
	// the meaningless ◦ for their family glyphs — IO延迟等 typed 事件归族).
	// The PTV6-C #3 typed type-lane state arms live inside the classifier.
	form := runtimeTraceProjImpactFormForNode(node, kind)
	if form == runtimeTraceProjImpactFormFallback {
		if hasData {
			marks.mark(runtimeTraceProjMarkIconNoDominant)
		} else {
			marks.mark(runtimeTraceProjMarkIconTransit)
		}
		return tracefence.GlyphNeutral
	}
	spec, ok := runtimeTraceProjImpactFormSpecFor(form)
	if !ok {
		marks.mark(runtimeTraceProjMarkIconNoDominant)
		return tracefence.GlyphNeutral
	}
	// UXR-1 §29.36② (4165 ◇段内戴⛓ 分叉形): the ⛓ glyph visually claims chain
	// membership, so it renders ONLY on the chain channel — off-chain (◇/▒)
	// D-state/IO rows wear ⧗ (same D-state/IO form family). Width note (复核
	// P2-4 勘正): ⧗ U+29D7 is EAW-NEUTRAL — 1 cell in both width contexts,
	// the ⧖ U+29D6 width class; it is NOT the ⛓ width class (⛓ U+26D3 is
	// East-Asian-Ambiguous). Pinned by the single-cell/EAW loop in
	// TestRCRImpactFormGlyphsSingleCellNoVS16. glyph / stanza / channel all
	// fork on the ONE chain-relevance source (三面同一来源): the row Kind for
	// stanza rows, the node's typed relevance elsewhere — mirrored by the
	// caller-side row channel authority (empty relevance stays chain,
	// fail-open).
	if spec.Glyph == tracefence.GlyphIOChain && runtimeTraceProjIconOffChain(node, kind) {
		marks.mark(runtimeTraceProjMarkIconDStateOffChain)
		return runtimeTraceProjOffChainDStateGlyph
	}
	marks.mark(spec.Mark)
	return spec.Glyph
}

// runtimeTraceProjIconOffChain reports whether the row the icon renders for
// sits on a non-chain channel (◇ adjacent / ▒ background) — the stanza Kind
// first (placement is relevance-derived), the node's typed relevance as the
// non-stanza mirror. Empty relevance = chain universe (fail-open, matches
// runtimeTraceProjNodeOrdinalChannel).
func runtimeTraceProjIconOffChain(node types.TraceCausalProjectionNode, kind string) bool {
	switch kind {
	case runtimeTraceProjTreeRowAdjacent, runtimeTraceProjTreeRowBackground:
		return true
	}
	switch strings.TrimSpace(node.ChainRelevance) {
	case "adjacent", "background":
		return true
	}
	return false
}

// runtimeTraceProjNoDominantStateRow reports whether a DATA row renders the ◦
// icon through the default (no dominant scheduler state) arm. PTV6-D (b): the
// T4 inline 无主导态 chip this used to gate is RETIRED (the icon + legend
// carry the class word); the predicate stays as the documented mirror of
// runtimeTraceProjStateIcon's switch — the #3 boundary pins consume it.
func runtimeTraceProjNoDominantStateRow(node types.TraceCausalProjectionNode, kind string) bool {
	if kind == runtimeTraceProjTreeRowSemantic || node.IsSleepState() {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running", "runnable", "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		return false
	}
	// PTV6-C #3: the typed type-lane state suppresses the 无主导态 chip — the
	// row carries a state family (icon + state word), it is not stateless.
	if runtimeTraceCausalProjectionTypeTokenStateClass(node) != "" {
		return false
	}
	return true
}

// runtimeTraceProjRowName wraps the base name with the §24.2 event-form ×N
// count (×N 上移行1): the merged count joins 行1's 词位 while the (a~b) range
// and the 单次最大 caliber ride 行3. Same typed gate as the structured
// builder (one helper, no drift).
func runtimeTraceProjRowName(row runtimeTraceProjTreeRow, zh bool) string {
	name := runtimeTraceProjRowNameBase(row, zh)
	if runtimeTraceProjCauseEventFoldRow(row) {
		name += runtimeTraceProjMergeCountChip(row.Node.MergedCount, zh)
	} else if runtimeTraceProjFamilyRow(row.Node) {
		// RCM-2 D2 行1 (§24.2 上移行1 同款): the family member count rides
		// the 词位 (witness ✦ VerifyClass 14次 / ⛓ 块设备IO(inode) 2次); the
		// caliber stem rides the value cell, the roster rides the sub-rows.
		name += runtimeTraceProjMergeCountChip(row.Node.FamilyMemberCount, zh)
	}
	if row.Node.DuplicatePublications > 1 {
		// UXR-1 §29.36④ (140554 witness 孤行灭): the ×N同值 chip rides the
		// 词位 — the same form the detail table prints — instead of a lone
		// 「· ×2同值」 orphan line; the legend entry keeps the semantics.
		name += " " + runtimeTraceProjDedupFoldTagText(row.Node.DuplicatePublications, zh)
	}
	return name
}

func runtimeTraceProjRowNameBase(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	if runtimeTraceCausalProjectionSemanticSpanRow(node) {
		// RCM-2 D2 (§24.10, 2026-07-08): a semantic FAMILY row's 词位 is the
		// typed semantic-class word (类型词行1词位) — one member's span name
		// must not impersonate the ×N family; the member names live on the
		// roster sub-rows and the detail stanza. Single-span rows keep the
		// span name byte-identically (单成员退化零演化).
		name := ""
		if runtimeTraceProjFamilyRow(node) {
			if word := runtimeTraceProjFamilySemanticClassWord(node, zh); word != "" {
				name = word
			}
		}
		if name == "" {
			name = strings.TrimSpace(node.SpanName)
			if name == "" {
				name = strings.TrimSpace(node.Object)
			}
		}
		// An on-chain semantic row is structurally nested at its host/trunk
		// position. An off-chain stanza row has no such parent, so its sole
		// publication seat keeps the host beside the optimization name.
		if row.Kind != runtimeTraceProjTreeRowSemantic {
			host := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
			if host != "" && name != "" {
				return host + " · " + name
			}
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
	// F1 (§22 PTV7-SPN P0): a generic row whose parsed span name reached the
	// display model shows the REAL name in the object slot —
	// "oney.hmn.berlin-42591 · H:ReceiveVsync(trace span)" — instead of the bare
	// type word. SpanName non-empty is the whole gate (precise boolean, soft
	// display face); semantic rows keep their dedicated arm above.
	if spanWord := runtimeTraceCausalProjectionSpanNameObjectWord(node, zh); spanWord != "" {
		object = spanWord
	}
	// SEM-LEAD (§29.7-2 ④, ledger real_trace_campaign_20260705.md,
	// 2026-07-10; per-CLASS with RCM-2 D2): a semantic FAMILY row keeps the
	// typed class 词位 on EVERY seat kind — the rank-lane / stanza seat of a
	// family must not let one member's span name impersonate the ×N family
	// (792-textup E13 行1 = largest member name "Texture upload(15573)
	// 1140x1856"). Typed gate (FamilyMemberCount + SemanticClass token),
	// never a name heuristic; single-span rows are untouched.
	if runtimeTraceProjFamilyRow(node) && strings.TrimSpace(node.SemanticClass) != "" {
		if word := runtimeTraceProjFamilySemanticClassWord(node, zh); word != "" {
			object = word
		}
	}
	// PTV8-RCR-A §24.1 (task-verbatim E7 case): an inversion cause node's 词位
	// is its state composition (runnable+running) — the composite's identity
	// word 优先级反转候选 rides 行2, so the old bare-state object would claim
	// a single state for a two-state composite.
	if word := runtimeTraceProjInversionStateCompositionWord(node); word != "" &&
		runtimeTraceProjCauseNodeRow(row) {
		object = word
	}
	// 76684 行1 形态词回退修 (SMR-1 批, 2026-07-12): the generic unresolved
	// shape names 「主体 · <state label>」 — 状态词永在行1; the peer word rides
	// its own tag (see the tags builder + the EVOLUTION RECORD on the helper).
	if word := runtimeTraceProjGenericUnresolvedStateNameWord(node, zh); word != "" {
		object = word
	}
	if row.Kind == runtimeTraceProjTreeRowCause {
		// Same-subject cause decomposition: the subject is already the parent
		// trunk row; show only the cause word.
		if object != "" {
			return object
		}
		return subject
	}
	if node.MicroAnchorFold {
		// RNB-5B 件⑦: the micro anchored-seat fold names its own family — the
		// generic 折叠 word would hide the credential semantics the ruling
		// preserves. R9 line-1 discipline rides along (bare counted label).
		row.marks.mark(runtimeTraceProjMarkMicroAnchorFold)
		return runtimeTraceProjMicroAnchorFoldName(node, zh)
	}
	if node.OnChainOverflowFold {
		// PTS (#68 用户裁定 2026-07-05): the on-chain overflow fold row names
		// its count explicitly — 零静默丢弃; members' names ride the roster
		// suffix, semantics live in the legend's 折叠 entry.
		// EVOLUTION RECORD (P2a rider 件1, §29.55.3 处置更新 2026-07-13, 用户
		// 裁定形 `链上─ ◌ 其余 2 项(折叠)`): the lane word 「链上」 deduped out
		// of the name — the restored 链上─ edge word carries the lane (with the
		// former edge-omission arm retired in runtimeTraceProjTreeLabelParts),
		// and the state-mark slot keeps the merged node's impact form.
		// EVOLUTION RECORD (R9 §29.93.2 用户裁定, 2026-07-15): the inline
		// member preview (roster + B6 榜位 pointer) left line 1 — it blew the
		// label column and pushed the bar off the grid (witness: the
		// WifiHandlerThre-12073 fold row). Line 1 keeps ONLY the bare counted
		// label; the preview sinks to the subordinate line 2 minted in
		// runtimeTraceProjRowMetricParts (信息零损只换行).
		row.marks.mark(runtimeTraceProjMarkOnChainOverflowFold)
		if zh {
			return fmt.Sprintf("其余 %d 项(折叠)", node.MergedCount)
		}
		return fmt.Sprintf("%d more (folded)", node.MergedCount)
	}
	if node.MergedCount > 1 && node.Subject == "" {
		// The fold line keeps the folded rows' thread names (customer
		// 2026-07-03: a bare "其余 N 项合并" lost every thread identity).
		// P2a rider 件1 F2 (§29.58.2, 2026-07-13): a ◇/▒ stanza cross-thread
		// fold joins the 边词/行名分工 arm — same dedup name form 其余N项(折叠)
		// (the section lane word rides the stanza fold's edge word, minted in
		// runtimeTraceProjStanzaRowFixed) and the same legend seat. Non-stanza
		// subjectless folds (hand-built/legacy shapes; production R3 folds all
		// render in the ▒/◇ buckets) keep the legacy 合并 wording.
		// EVOLUTION RECORD (R9 §29.93.2, 2026-07-15): the stanza fold faces
		// (背景─/邻近─ = the ◇ 区) slim the same way as the chain fold — line 1
		// bare counted label, member preview on the subordinate line 2 (全部
		// 折叠行发射面一体); the legacy non-stanza 合并 shape rides along.
		if runtimeTraceProjStanzaRowKind(row.Kind) {
			row.marks.mark(runtimeTraceProjMarkOnChainOverflowFold)
			if zh {
				return fmt.Sprintf("其余 %d 项(折叠)", node.MergedCount)
			}
			return fmt.Sprintf("%d more (folded)", node.MergedCount)
		}
		if zh {
			return fmt.Sprintf("其余 %d 项合并", node.MergedCount)
		}
		return fmt.Sprintf("%d more folded", node.MergedCount)
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

// runtimeTraceProjChainUniverseRowKind is the ruling-A universe gate (PTV6-C
// #73, 用户裁定 2026-07-06): the chain/cause/depthless kinds are the only rows
// whose durations may wear the on-chain attribution vocabulary (有效归因 /
// 链上累计).
func runtimeTraceProjChainUniverseRowKind(kind string) bool {
	switch kind {
	case runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowCause, runtimeTraceProjTreeRowDepthless:
		return true
	}
	return false
}

// runtimeTraceProjStanzaRowKind identifies the ◇/▒ stanza kinds — the rows
// whose same data re-words onto the 累计(跨线程) family (ruling A kind-split;
// single wording home below).
func runtimeTraceProjStanzaRowKind(kind string) bool {
	switch kind {
	case runtimeTraceProjTreeRowAdjacent, runtimeTraceProjTreeRowBackground:
		return true
	}
	return false
}

// runtimeTraceProjCrossThreadCumWord / TagText are THE wording home of the
// ruling-A ◇/▒ family word: the value is a cross-thread cumulative, never an
// on-chain attribution — the tag says so where the reader is. The bare word
// doubles as the C00 fallback caliber word on stanza rows.
func runtimeTraceProjCrossThreadCumWord(zh bool) string {
	if zh {
		return "累计(跨线程)"
	}
	return "cross-thread cum"
}

func runtimeTraceProjCrossThreadCumTagText(v float64, zh bool) string {
	if zh {
		return fmt.Sprintf("%s%.3fms", runtimeTraceProjCrossThreadCumWord(true), v)
	}
	return fmt.Sprintf("%s %.3fms", runtimeTraceProjCrossThreadCumWord(false), v)
}

// runtimeTraceProjDedupFoldTagText is the dedupe-exclusive same-value label
// (RF2b, adversarial review 2026-07-03): a duplicate-publication row's ms is
// ONE measurement that was published N times, so it must never share the R2
// sum-aggregate form. PTV4 T4 (三式): the row keeps only the data token —
// the "重复发布/数值不变" semantics live in the legend's 口径组 entry.
// Callers fork on the typed Node.DuplicatePublications count.
//
// EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「×N同值」→「N次同值」/
// en 「n=N same-value」 — the ×N marker was semantically overloaded (the
// same spelling meant instance count on sum rows and thread count on max
// rows) and 「×」 read as multiplication in arithmetic-dense reports. The
// zh count word 次 and the en k=v count form n= dissolve the overload; the
// full five-form vocabulary lives in tracefence display-table ⑥.
func runtimeTraceProjDedupFoldTagText(count int, zh bool) string {
	if zh {
		return fmt.Sprintf("%d次同值", count)
	}
	return fmt.Sprintf("n=%d same-value", count)
}

// runtimeTraceProjMergedSumTagText is the R2 SUM aggregate's inline data
// token (PTV4 T4 三式): count + per-instance range only; the SUM semantics
// live in the legend's 口径组 entry. Count word per language (WF-xn:
// zh N次 / en n=N).
//
// G12-ENG 复核 P2-2 (2026-07-09): the mixed shape binds the range to the
// VALUED members; the standalone all-zero R2 shape (the hmfs ×4 blocked_reason
// aggregate when it does NOT overflow — same E23 class) says 全部无时长值
// instead of the ×N(0.000–0.000ms) pseudo-value (G19 covers only the
// subjectless fold; this is its R2 twin).
// EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「×N(a~b)」→「N次(a~b)」/
// en 「n=N(a~b)」 (semantic-overload dissolution — see the dedup tag note).
func runtimeTraceProjMergedSumTagText(node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceProjMergedAllValueless(node) {
		if zh {
			return fmt.Sprintf("%d次(全部无时长值)", node.MergedCount)
		}
		return fmt.Sprintf("n=%d (all without measurable duration)", node.MergedCount)
	}
	if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		if zh {
			return fmt.Sprintf("%d次(有值%d项 %s,%d项无时长值)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
		}
		return fmt.Sprintf("n=%d (%d valued %s, %d without measurable duration)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
	}
	if zh {
		return fmt.Sprintf("%d次(%.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("n=%d(%.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjMergedUnionTagText is the §11-N2 cross-query-window union
// row's inline data token (×N 第四式): count + per-instance range + the union
// form suffix. The 不重复计/非求和 semantics live in the legend's 口径组
// entry; the raw Σ and the window-source roster live in the (b) lossless
// block. All-valued rows stay language-neutral (numbers + the ASCII form
// token), like the SUM form. Callers fork on the typed
// Node.MergedIntervalUnion flag — a union row must NEVER wear the plain
// ×N(a~b) sum form (its legend entry claims 数值为总和).
//
// G12-ENG 复核 P2-1 连带: the mixed shape binds the range to the VALUED
// members (same E23-class honesty as the max/sum/CWD tags — the fence and the
// (b) block must not contradict on one row).
// EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「×N(a~b)union」→
// 「N次(a~b)union」/ en 「n=N(a~b)union」.
func runtimeTraceProjMergedUnionTagText(node types.TraceCausalProjectionNode, zh bool) string {
	if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		if zh {
			return fmt.Sprintf("%d次(有值%d项 %s,%d项无时长值)union", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
		}
		return fmt.Sprintf("n=%d (%d valued %s, %d without measurable duration) union", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
	}
	if zh {
		return fmt.Sprintf("%d次(%.3f~%.3fms)union", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("n=%d(%.3f~%.3fms)union", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjMergedAllValueless is the standalone all-zero R2 shape's
// typed gate (G12-ENG 复核 P2-2): every merged member is valueless. Distinct
// from the G19 subjectless-fold arm (runtimeTraceProjAllZeroFoldRow), which
// keeps its own note wording.
func runtimeTraceProjMergedAllValueless(node types.TraceCausalProjectionNode) bool {
	return node.MergedCount > 1 && node.MergedValuelessCount >= node.MergedCount
}

// runtimeTraceProjMergedValuelessWordRenders reports whether a merged row's
// tag renders the 无时长值 family word (mixed OR all-valueless) — the mark
// gate keeping the legend's 双向契约.
func runtimeTraceProjMergedValuelessWordRenders(node types.TraceCausalProjectionNode) bool {
	if _, _, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		return true
	}
	return runtimeTraceProjMergedAllValueless(node)
}

// runtimeTraceProjMergedPerInstanceText renders the (b) detail forms'
// per-instance segment (G12-ENG 复核 P2-1/P2-2 三面同词): the valued range on
// all-valued rows (legacy byte-identical), the valued split on mixed rows,
// the honest no-value wording on all-valueless rows.
func runtimeTraceProjMergedPerInstanceText(node types.TraceCausalProjectionNode, zh bool) string {
	if runtimeTraceProjMergedAllValueless(node) {
		if zh {
			return "全部无时长值"
		}
		return "all without measurable duration"
	}
	if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		if zh {
			return fmt.Sprintf("有值%d项单次 %s,另%d项无时长值", valued, runtimeTraceProjMergedRangeText(node), valueless)
		}
		return fmt.Sprintf("%d valued member(s) each %s, %d without measurable duration", valued, runtimeTraceProjMergedRangeText(node), valueless)
	}
	if zh {
		return fmt.Sprintf("单次 %.3f~%.3fms", node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("each %.3f~%.3fms", node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjMergedMaxTagText is the R3 cross-thread fold's inline data
// token (PTV4 T4 ×N 三式): the 取最大/不求和 semantics live in the legend's
// 口径组 entry; the member roster stays via the name lane / detail blocks.
//
// G12-ENG (§29.1, 2026-07-09): a MIXED fold (some members valueless) binds the
// min–max range to the VALUED member count instead of the total — the legacy
// "×2(14.272–14.272ms)取最大" over one 14.272ms member plus one zero-duration
// blocked_reason aggregate fabricated a second 14.272ms observation under the
// valueless member's subject (huadong_79 E23 → customer G12 raw-trace audit).
// All-valued folds render byte-identically to the legacy form.
// EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「×N(a~b)取最大」→
// 「N线程取最大(单项a~b)」/ en 「N-thread max(each a~b)」 — the count here is
// THREADS, the exact meaning the overloaded ×N hid (ruling witness: the
// same ×N read as N instances on sum rows).
func runtimeTraceProjMergedMaxTagText(node types.TraceCausalProjectionNode, zh bool) string {
	if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		if zh {
			return fmt.Sprintf("%d线程取最大(有值%d项 单项%s,%d项无时长值)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
		}
		return fmt.Sprintf("%d-thread max(%d valued %s, %d without measurable duration)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
	}
	if zh {
		return fmt.Sprintf("%d线程取最大(单项%.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("%d-thread max(each %.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjMergedValuedSplit splits a merged row's member count into
// valued vs valueless members (typed MergedValuelessCount, G12-ENG §29.1).
// mixed is true only for the both-kinds shape — the all-valued legacy form and
// the all-zero G19 form keep their existing render arms byte-identical.
func runtimeTraceProjMergedValuedSplit(node types.TraceCausalProjectionNode) (valued, valueless int, mixed bool) {
	valueless = node.MergedValuelessCount
	valued = node.MergedCount - valueless
	return valued, valueless, valueless > 0 && valued > 0
}

// runtimeTraceProjMergedRangeText renders the valued members' display range:
// the single value when the range is degenerate (one valued member, or
// min == max), the a~b form otherwise. Numbers + units only.
func runtimeTraceProjMergedRangeText(node types.TraceCausalProjectionNode) string {
	if node.MergedMinMS == node.MergedMaxMS {
		return fmt.Sprintf("%.3fms", node.MergedMaxMS)
	}
	return fmt.Sprintf("%.3f~%.3fms", node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjAllZeroFoldNoteText is the DISP-2 G19 one-line note for the
// all-zero fold shape (§27.5, 2026-07-09) — the honest replacement of the
// retired ×N(0.000–0.000ms)取最大 tag on that shape, shared by the fence tag
// and the (a) table token so the two faces can never drift. The (数据盲区)
// qualifier renders only on the typed all-members-are-data-gaps fold
// (MergedAllDataGap — never inferred from the zero values alone).
func runtimeTraceProjAllZeroFoldNoteText(node types.TraceCausalProjectionNode, zh bool) string {
	if node.MergedAllDataGap {
		if zh {
			return "窗内无有效时长(数据盲区),见明细"
		}
		return "no in-window effective duration (data blind spots); see the detail blocks"
	}
	if zh {
		return "窗内无有效时长,见明细"
	}
	return "no in-window effective duration; see the detail blocks"
}

// runtimeTraceProjMergedCrossWindowMaxTagText is the §21-CWD overlapping-
// query-window MAX row's inline data token (×N 第五式): count + per-instance
// range + the cross-window-max form suffix. The 不求和/窗基 semantics live in
// the legend's 口径组 entry; the raw Σ and the window-source roster live in
// the (b) lossless block. Callers fork on the typed Node.MergedCrossWindowMax
// flag — a MAX row must never wear the plain ×N(a~b) sum form (its legend
// entry claims 数值为总和) nor the union form (per-segment deduction).
func runtimeTraceProjMergedCrossWindowMaxTagText(node types.TraceCausalProjectionNode, zh bool) string {
	// G12-ENG (§29.1): same mixed-fold honesty as the ×N取最大 tag — the
	// range binds to the valued member count only.
	if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
		if zh {
			return fmt.Sprintf("%d次跨窗取最大(有值%d项 单项%s,%d项无时长值)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
		}
		return fmt.Sprintf("n=%d cross-window max(%d valued %s, %d without measurable duration)", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
	}
	if zh {
		return fmt.Sprintf("%d次跨窗取最大(单项%.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
	}
	return fmt.Sprintf("n=%d cross-window max(each %.3f~%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
}

// runtimeTraceProjPeriodicCrossWindowSumClause is the row-face caliber
// disclosure for a CROSS-WINDOW merged periodic row, gated on the typed
// value-caliber flags (MergedIntervalUnion / MergedCrossWindowMax — the same
// flags the value-channel tag forks on). Empty on every non-periodic,
// non-merged or single-window shape (word face fully absent — byte-identical
// legacy rows).
//
// EVOLUTION RECORD (PERIODIC-DEDUP, §29.104 ① 终判, 2026-07-15): the 终判⑤
// (§29.96.2) original sentence read 「逐次折减相加,与行值去重口径分账」 —
// honest while the 有效归因 Σ deliberately stayed un-deduped beside the
// deduped value channel (two rulers, booked apart). The §29.104 ① ruling
// unified the calibers: the Σ-effective lane now consumes the SAME
// same-segment proof as the value channel (window slots + occurrence-interval
// overlap, traceCausalProjectionPeriodicDiscountCounted) and a cross-window
// re-measured occurrence's discount counts once, so the 分账 clause would be
// a stale lie. The sentence states the unified rule with the 「已证」
// qualifier IN the word face (复核 UX病1, 2026-07-15: the first rewording
// said bare 「同段重测…只计一次」, which read as a row-value claim and was
// FALSE on the §21 CWD unprovable shape — a windowed member without an
// occurrence interval proves nothing, the Σ honestly keeps every copy there).
// 「已证同段」 makes the sentence a conditional rule that is true on every
// gated shape: distinct/unproven occurrences add, PROVEN re-measurements
// count once. Deliberately no typed proven/unproven fork (zero-R2'-cost
// ruling — a fork would need a new carried field for a word-face split the
// qualifier already makes honest).
func runtimeTraceProjPeriodicCrossWindowSumClause(node types.TraceCausalProjectionNode, zh bool) string {
	if !node.PeriodicSource || node.MergedCount <= 1 ||
		(!node.MergedIntervalUnion && !node.MergedCrossWindowMax) {
		return ""
	}
	if zh {
		return "(跨窗周期合计:逐次折减相加,已证同段重测折减只计一次)"
	}
	return " (cross-window periodic sum: per-occurrence discounts added; a proven same-segment re-measurement counts once)"
}

// runtimeTraceProjMultiWindowMergedRow is the §21.1 CWD-2 ① typed key
// (huadong E19/E1 witnesses): a merged ×N row whose member roster spans
// MULTIPLE known query windows. Such a row's magnitude has no single window
// base, so every anchor-window share/percentage face (tree-row % cell,
// conclusion-line 占窗 share) and the coverage window consensus fork on this
// ONE predicate — single source. Load-bearing pairing: the aggregator zeroes
// the row-level QueryWindow identity for exactly these mixed rosters
// (trace_causal_projection_aggregate.go, union.singleWindow), so row-level
// identity alone can never see them.
func runtimeTraceProjMultiWindowMergedRow(node types.TraceCausalProjectionNode) bool {
	return node.MergedCount > 1 && len(node.MergedQueryWindows) > 1
}

// runtimeTraceProjMergedMemberWindowSpanWord is the ONE emitter of the
// CASE3-D4 伴生 member-window-span disclosure word (§29.84 件④, 2026-07-14):
// 「成员跨K窗」 for a merged ×N row whose member roster spans multiple known
// query windows (same typed key as every other multi-window face —
// runtimeTraceProjMultiWindowMergedRow, zero new signals). Consumed by the
// seat window chip's 「(供席成员窗,成员跨K窗)」 qualifier and the ◎ overview
// row transcription; per-member windows stay on the detail 窗来源 lane.
func runtimeTraceProjMergedMemberWindowSpanWord(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	if !runtimeTraceProjMultiWindowMergedRow(node) {
		return "", false
	}
	if zh {
		return fmt.Sprintf("成员跨%d窗", len(node.MergedQueryWindows)), true
	}
	return fmt.Sprintf("members span %d windows", len(node.MergedQueryWindows)), true
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

// runtimeTraceProjFoldMemberSinkLine — R9 (§29.93.2 用户裁定, 2026-07-15):
// the fold row's member preview sinks from line 1 to this subordinate line 2,
// reusing the member-row family shape (成员 word family + counted 见明细
// trailer). The head member arrives ALREADY carrying its B6 榜位 pointer
// suffix when one resolved (runtimeTraceProjAnnotateFoldRosterRankPointers
// runs before render). 信息零损只换行 — the full roster stays on the detail
// block, whose name face keeps the complete inventory.
func runtimeTraceProjFoldMemberSinkLine(node types.TraceCausalProjectionNode, zh bool) string {
	if len(node.MergedSubjects) == 0 {
		return ""
	}
	head := node.MergedSubjects[0]
	rest := node.MergedCount - 1
	if zh {
		if rest > 0 {
			return fmt.Sprintf("成员 %s · 其余 %d 项见明细", head, rest)
		}
		return "成员 " + head
	}
	if rest > 0 {
		return fmt.Sprintf("member %s · %d more in the detail blocks", head, rest)
	}
	return "member " + head
}

// runtimeTraceProjFoldMemberSinkRow reports whether a row is a fold-row face
// whose line-1 label slimmed under R9 (the exact predicate family of the
// name mints: chain overflow fold / stanza fold / legacy subjectless 合并).
func runtimeTraceProjFoldMemberSinkRow(row runtimeTraceProjTreeRow) bool {
	return (row.Node.OnChainOverflowFold || runtimeTraceProjSubjectlessFoldRow(row.Node)) &&
		len(row.Node.MergedSubjects) > 0
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

// runtimeTraceProjBarShareText — catalog B10 (DISPLAY-HYG 二轮,
// §29.104.18.1, 2026-07-17): the tree bar's 5-cell share text. A NONZERO
// share whose %.0f print rounds to 0 says `<1%` — the bar beside it always
// shows ≥1 filled cell for value>0 (runtimeTraceProjBar's floor), so
// 「█░░… 0%」 self-contradicted (witness ×11). True zero keeps `0%`
// byte-identically; the judgment reads the ACTUAL printed form (never a
// second threshold that could drift from %.0f rounding). Same " NN%" cell
// layout on both arms.
func runtimeTraceProjBarShareText(share float64) string {
	text := fmt.Sprintf(" %3.0f%%", share)
	if share > 0 && strings.TrimSpace(strings.TrimSuffix(text, "%")) == "0" {
		return fmt.Sprintf(" %3s%%", "<1")
	}
	return text
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
	if row.Kind == runtimeTraceProjTreeRowCycleFold {
		// PTV8-LAD L1: the cycle row emits the ↺ token, so it records the
		// recurs mark too (NEW-7: marks record at the emission site of their
		// token; the ↺ legend statement — the thread recurs on the chain — is
		// literally true of every tuple member).
		row.marks.mark(runtimeTraceProjMarkCycleFold)
		row.marks.mark(runtimeTraceProjMarkRecursOnChain)
		return runtimeTraceProjCycleFoldRowLine(row, zh)
	}
	fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
	left := runtimeTraceProjTreeLabelRow(fixed, row, name, width, zh)
	var line string
	if !row.HasData {
		// NEW-10 (§7.6): transit rows carry no bar to align — render compact.
		// PTV4 T4: the parenthesized explanation is retired; the ◦ transit
		// sense carries the 2-word inline token, the legend's ◦ 中转 entry
		// holds the semantics.
		left = strings.TrimRight(left, " ")
		switch {
		case row.UserFocusForced:
			// §22 B1-b F2: a user-focus thread force-expanded out of the
			// folded trunk middle must state its identity — the anonymous
			// 中转 token would hide exactly what the expansion disclosed.
			// PTV8-LAD L3 (§24.8 图标化令). EVOLUTION RECORD: the 18-cell
			// 「用户关注线程(中转)」 label ate the name budget of the very row
			// it existed to identify — the 3-cell ⊚中转 short token (root
			// user-focus glyph, shared constant) replaces it; the legend entry
			// carries the semantics.
			row.marks.mark(runtimeTraceProjMarkUserFocusTransit)
			if zh {
				line = left + " " + runtimeTraceProjRootGlyph + "中转"
			} else {
				line = left + " " + runtimeTraceProjRootGlyph + "transit"
			}
		case zh:
			line = left + " 中转"
		default:
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
// typed path). The former trailing intermediate-record pointer clause is fully
// retired (PTV6-C ruling C — the legend's 省略行 entry now states the fold
// honestly instead). PTV8-LAD L1 (§24.11 维度A). EVOLUTION RECORD: the
// "(检测到N节点循环约M轮)" clause is retired with its index-0 whole-path
// detector — cycles now fold into their own counted CycleFold rows.
func runtimeTraceProjOmittedRowLine(row runtimeTraceProjTreeRow, zh bool) string {
	prefix := runtimeTraceProjTreePrefix(row)
	head := fmt.Sprintf("…省略%d节点", row.Omitted)
	if !zh {
		head = fmt.Sprintf("…%d nodes omitted", row.Omitted)
	}
	names := append([]string(nil), row.OmittedHead...)
	tailStart := len(names)
	names = append(names, row.OmittedTail...)
	if len(names) == 0 {
		return prefix + " " + head + "…"
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
		line := prefix + " " + head + ": " + roster(budget)
		if runewidth.StringWidth(line) <= runtimeTraceProjTreeRowMaxWidth || budget <= 8 {
			return line
		}
	}
}

// runtimeTraceProjCycleFoldRowLine renders the PTV8-LAD L1 run-length cycle
// fold row: "↺ 循环×N: A ⇄ B" — the repeat count plus the tuple member names
// IN FULL (§24.8 重要信息永不省略: the row exists to disclose exactly these
// identities, so the names never truncate; the line renders unpadded like the
// ⊚ header when it outgrows the shared column). The folded per-hop rows are
// reconstructible as tuple × count — strictly more information than the plain
// roster's first/last-two form.
func runtimeTraceProjCycleFoldRowLine(row runtimeTraceProjTreeRow, zh bool) string {
	sep := " ⇄ "
	members := strings.Join(row.CycleTuple, sep)
	if zh {
		return fmt.Sprintf("%s ↺ 循环×%d: %s", runtimeTraceProjTreePrefix(row), row.CycleCount, members)
	}
	return fmt.Sprintf("%s ↺ cycle ×%d: %s", runtimeTraceProjTreePrefix(row), row.CycleCount, members)
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
	// width pass in runtimeTraceProjTreeLabelColumn exactly (single composer:
	// runtimeTraceProjStanzaRowFixed).
	left := runtimeTraceProjTreeLabelRow(runtimeTraceProjStanzaRowFixed(row, row.marks, zh),
		row, runtimeTraceProjRowName(row, zh), width, zh)
	return runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
}

// runtimeTraceProjStanzaRowFixed is the ONE stanza fixed-part composer shared
// by the render pass and the width pass (they were two hand-mirrored copies).
// §29.27.1 (徽章跟随席位): a stanza row whose published seat is TOP-5 wears
// its ❶..❺ glyph exactly like a tree row — the badge slot sits before the
// state glyph, mirroring runtimeTraceProjTreeLabelParts.
func runtimeTraceProjStanzaRowFixed(row runtimeTraceProjTreeRow, marks *runtimeTraceProjMarkSet, zh bool) string {
	badge := ""
	if glyph := runtimeTraceProjBadgeGlyph(row.Badge); glyph != "" {
		if marks != nil {
			marks.mark(runtimeTraceProjMarkBadge)
		}
		// UXG-0 D5: badge→state-glyph gap, same as the tree/self emitters.
		badge = glyph + " "
	}
	// P2a rider 件1 F2 (§29.58.2, 2026-07-13): the stanza cross-thread fold
	// row wears its SECTION's lane word in the edge slot (边词管车道 — the
	// same division of labor as the 链上─ chain fold; the row name deduped to
	// 其余N项(折叠)). Sibling stanza rows carry no edge, so the fold's state
	// mark sits right of the lane word instead of the shared glyph column —
	// the lane word is the load-bearing information there.
	// DISPHYG-3 件2 (§29.150⑪ C4 user ruling, 2026-07-20). EVOLUTION RECORD:
	// the stanza fold's edge word wears the tree-connector form
	// (├─邻近─/├─背景─ — the 「├─链上─ ◦」 family), unifying the fold-row
	// edge-word face across lanes. Fixed ├─ deliberately: stanza rows carry
	// no tree rails and no Last semantics, so the chain lane's positional
	// └─ variant has no producer here (delegated wording choice).
	edge := ""
	if runtimeTraceProjStanzaFoldRow(row) {
		edge = "├─" + runtimeTraceProjStanzaLaneEdgeWord(row.Kind, zh) + " "
	}
	return "    " + edge + badge + runtimeTraceProjStateIcon(row.Node, row.Kind, true, marks) + " "
}

// runtimeTraceProjStanzaFoldRow is the ONE gate of the F2 stanza-fold form —
// shared by the name mint (runtimeTraceProjRowName subjectless-fold arm) and
// the edge-word mint above so the two faces can never fork: a subjectless
// cross-thread fold row seated in a ◇/▒ stanza (the production R3 background
// fold shape; OnChainOverflowFold rows are the chain lane's own fold).
func runtimeTraceProjStanzaFoldRow(row runtimeTraceProjTreeRow) bool {
	return runtimeTraceProjStanzaRowKind(row.Kind) && !row.Node.OnChainOverflowFold &&
		row.Node.MergedCount > 1 && strings.TrimSpace(row.Node.Subject) == ""
}

// runtimeTraceProjStanzaLaneEdgeWord is the stanza fold row's lane word
// (§29.58.2 F2 边词=该区段车道词): the section noun the ◇/▒ headers already
// teach, in the tree edge-word form (trailing ─, same as 链上─).
func runtimeTraceProjStanzaLaneEdgeWord(kind string, zh bool) string {
	if kind == runtimeTraceProjTreeRowAdjacent {
		if zh {
			return "邻近─"
		}
		return "adjacent─"
	}
	if zh {
		return "背景─"
	}
	return "background─"
}

// runtimeTraceProjRowCauseWordToken returns the display cause segment of
// this row's name — exactly the value the RowName composer placed after the
// "subject · " separator (cause rows: the whole name IS the cause word) —
// together with the TYPED token that display derives from (the #6/#12 dedupe
// identity; 修正轮 Med: display strings never judge the dedupe). ("", "") for
// the row families whose name is not a subject·cause composition (aggregate
// metrics, lock-contention D1 names, semantic spans, folds).
func runtimeTraceProjRowCauseWordToken(row runtimeTraceProjTreeRow, zh bool) (string, string) {
	node := row.Node
	if row.Kind == runtimeTraceProjTreeRowSemantic || node.IsAggregateMetric() ||
		node.OnChainOverflowFold || node.MergedCount > 1 && strings.TrimSpace(node.Subject) == "" {
		return "", ""
	}
	if runtimeTraceCausalProjectionBlockingName(node, zh) != "" {
		return "", ""
	}
	// F1 (§22 PTV7-SPN): mirror the RowName composer's span-word override —
	// the #12 guarantee then protects the FULL span name across a name-cell
	// cut; the typed dedupe identity stays the cause token.
	if spanWord := runtimeTraceCausalProjectionSpanNameObjectWord(node, zh); spanWord != "" {
		return spanWord, runtimeTraceCausalProjectionCauseDisplayToken(node)
	}
	// PTV8-RCR-A §24.1: mirror the inversion 词位 override (runnable+running)
	// — the guarantee protects the composition word the row actually renders;
	// the typed dedupe identity stays the inversion token.
	if word := runtimeTraceProjInversionStateCompositionWord(node); word != "" &&
		runtimeTraceProjCauseNodeRow(row) {
		return word, "priority_inversion_candidate"
	}
	// 76684 行1 修: mirror the generic-unresolved state 词位 override — the
	// guarantee protects the state word the row actually renders. P3 (修复轮
	// 2026-07-13, 2609 形宽度): the type-lane form parks the state on
	// TypeToken (StateKind empty) — the dedupe token follows the SAME typed
	// lane the word came from, so the #6 near-synonym dedupe can still match.
	if word := runtimeTraceProjGenericUnresolvedStateNameWord(node, zh); word != "" {
		token := runtimeTraceCausalProjectionCanonicalNode(node.StateKind)
		if token == "" {
			token = runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)
		}
		return word, token
	}
	return strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh)),
		runtimeTraceCausalProjectionCauseDisplayToken(node)
}

// runtimeTraceProjApplyCauseWordGuarantee is the PTV6-C #12 cause full-word
// guarantee (#73, 标本归因 2026-07-06), fused with the #6 near-synonym dedupe:
//   - name cell truncated across the cause word ("… · 优先级反…") → the FIRST
//     subordinate slot carries the FULL cause word (never lost to the cut);
//   - a tag whose TYPED identity equals the cause word's typed identity drops
//     (全词一处+数据一处) — 修正轮 Med (2026-07-06): the judge is the typed
//     token pair (canonical equality or same state family, zh/en 双面同判),
//     never a display-string collision (the retired substring lane ate the
//     裁定4 state tag on the EN face: running ⊂ running_burst, and missed the
//     EN runnable_wait double). MainRow keep-marks (⚠/⊘/[E#]) never touched.
func runtimeTraceProjApplyCauseWordGuarantee(left string, row runtimeTraceProjTreeRow, tags []runtimeTraceProjTag, zh bool) []runtimeTraceProjTag {
	causeWord, causeToken := runtimeTraceProjRowCauseWordToken(row, zh)
	if causeWord == "" {
		return tags
	}
	kept := tags[:0]
	for _, tag := range tags {
		if !tag.MainRow && runtimeTraceProjTypedSameCause(tag.DedupeToken, causeToken) {
			continue
		}
		kept = append(kept, tag)
	}
	if strings.Contains(left, causeWord) {
		return kept
	}
	// PTV8-RCR-A (§24.1): a cause node whose 行2 identity line already
	// carries the full cause word (the relocated category) needs no prepended
	// guarantee copy — the word is whole on the row's own subordinate line.
	for _, tag := range kept {
		if tag.OwnLine && strings.Contains(tag.Text, causeWord) {
			return kept
		}
	}
	// UXR-1 §29.36④ (140554 witness form): when the name cell was cut across
	// the cause word, the row-2 guarantee copy carries the ×N同值 chip WITH it
	// (与明细表同形 — the chip belongs to the name wherever the name renders).
	if row.Node.DuplicatePublications > 1 {
		causeWord += " " + runtimeTraceProjDedupFoldTagText(row.Node.DuplicatePublications, zh)
	}
	return append([]runtimeTraceProjTag{{Text: causeWord}}, kept...)
}

// runtimeTraceProjTypedSameCause reports whether two typed tokens carry the
// same cause identity: canonical equality, or membership in the same
// scheduler-state family (runnable_wait ≡ runnable — the 全词一处 case where
// the cause word IS the same state wait the 裁定4 tag speaks). Empty tokens
// never match — a tag without a typed identity is never dedupable.
func runtimeTraceProjTypedSameCause(a, b string) bool {
	a = runtimeTraceCausalProjectionCanonicalNode(a)
	b = runtimeTraceCausalProjectionCanonicalNode(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ca, cb := runtimeTraceProjStateTokenClass(a), runtimeTraceProjStateTokenClass(b)
	return ca != "" && ca == cb
}

// runtimeTraceProjActionJointFamily maps a typed token onto the ACTION-word
// family space (b3 修, 2026-07-06): the blocking action word (PTV7:
// D-state/iowait) covers the d_state / io_wait / d_state_or_io_wait classes
// jointly, so the family compare must treat them as one; running/runnable
// map through unchanged.
// "" = no action family (sleep and non-state tokens).
func runtimeTraceProjActionJointFamily(token string) string {
	switch runtimeTraceProjStateTokenClass(token) {
	case "running":
		return "running"
	case "runnable":
		return "runnable"
	case "d_state", "io_wait", "d_state_or_io_wait":
		return "blocking_io"
	}
	return ""
}

// runtimeTraceProjStateTokenClass maps a typed token to its scheduler-state
// family for the dedupe identity compare — exact token membership (the
// ambiguous d_state_or_io_wait family is its OWN class: it never folds into
// the more specific D-state/iowait single-state words, and vice versa).
func runtimeTraceProjStateTokenClass(token string) string {
	switch runtimeTraceCausalProjectionCanonicalNode(token) {
	case "running", "fragmented_running":
		return "running"
	case "runnable", "runnable_wait", "fragmented_runnable_wait":
		return "runnable"
	case "sleep", "s_sleep", "sleep_wait", "fragmented_sleep_wait":
		return "sleep"
	case "d_state", "d_sleep", "uninterruptible_sleep":
		return "d_state"
	case "io_wait":
		return "io_wait"
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		return "d_state_or_io_wait"
	}
	return ""
}

// runtimeTraceProjRowLineWithMetrics assembles label + bar/ms cells + tags
// (PTV4 T1, 按需拆行 → PTV6-D (a) 悬崖消除): when EVERY tag fits inline within
// the 100-cell row cap the row stays one line; otherwise the main line keeps
// the essentials (label + bar + ms + % + the MainRow marks ⚠/⊘/[E#] — never
// truncated, never moved down) PLUS the leading run of ordinary tags that
// still fits beside them (prefix fill — the former all-or-nothing demotion
// dropped every tag over a 1-cell overflow), and the overflow tags PACK into
// "· " subordinate detail lines (streamed " · " separation, a new line only
// when the next tag does not fit; T3 boundary-aware wrap for over-wide single
// tags; nothing is elided — the FitTags DropOrder lane stays retired).
func runtimeTraceProjRowLineWithMetrics(left string, row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	base, tags := runtimeTraceProjRowMetricParts(row, denom, windowMode, zh)
	tags = runtimeTraceProjApplyCauseWordGuarantee(left, row, tags, zh)
	// DISPLAY-WRAP 件③(a): same-node caliber-phrase repeat suppression, in
	// display order — OwnLine grammar lines render first, ordinary tags after
	// them; MainRow keep-marks carry no caliber phrases. (The 行2 identity
	// tag carries chips/confidence only — its IdentityGroups stay in sync.)
	var dedupTexts []*string
	for i := range tags {
		if tags[i].OwnLine {
			dedupTexts = append(dedupTexts, &tags[i].Text)
		}
	}
	for i := range tags {
		if !tags[i].OwnLine && !tags[i].MainRow {
			dedupTexts = append(dedupTexts, &tags[i].Text)
		}
	}
	runtimeTraceProjDedupNodeCaliberPhrases(dedupTexts, row.marks, zh)
	if len(tags) == 0 {
		return left + " " + base
	}
	// PTV8-RCR-A (§24.1): a row carrying structured grammar lines keeps 行1
	// grammar-clean — only the MainRow keep-marks stay inline; every OwnLine
	// tag renders as its own subordinate "· " line in order, and the remaining
	// ordinary tags pack below them (never inline, deterministic layout).
	for _, tag := range tags {
		if !tag.OwnLine {
			continue
		}
		var mainTexts []string
		var ownLines []runtimeTraceProjTag
		var packed []string
		for _, t := range tags {
			switch {
			case t.MainRow:
				mainTexts = append(mainTexts, t.Text)
			case t.OwnLine:
				ownLines = append(ownLines, t)
			default:
				packed = append(packed, t.Text)
			}
		}
		line := left + " " + base
		if len(mainTexts) > 0 {
			line += "  " + strings.Join(mainTexts, " · ")
		}
		indent := runtimeTraceProjRowContinuationIndent(row)
		for _, own := range ownLines {
			// DISPLAY-WRAP 件①(c): the 行2 identity line splits by its
			// semantic groups under width pressure (fitting rows and every
			// other OwnLine keep the generic T3 wrap byte-identically).
			conts := runtimeTraceProjSubordinateLines(indent, own.Text)
			if len(own.IdentityGroups) > 1 {
				conts = runtimeTraceProjIdentityGroupLines(indent, own.IdentityGroups, zh)
			}
			for _, cont := range conts {
				line += "\n" + cont
			}
		}
		for _, cont := range runtimeTraceProjSubordinatePackedLines(indent, packed) {
			line += "\n" + cont
		}
		return line
	}
	// UXR-1 §29.36④ (行2 限定词槽): a Row2 tag pulls EVERY ordinary tag down
	// with it — 行1 keeps only the metric cells + MainRow keep-marks, and the
	// subordinate stream packs qualifier-first (emission order already places
	// the qualifier before its reasons; 裸尾巴/主行中缝变体 both die here).
	for _, tag := range tags {
		if !tag.Row2 {
			continue
		}
		var mainTexts, packed []string
		for _, t := range tags {
			if t.MainRow {
				mainTexts = append(mainTexts, t.Text)
				continue
			}
			packed = append(packed, t.Text)
		}
		line := left + " " + base
		if len(mainTexts) > 0 {
			line += "  " + strings.Join(mainTexts, " · ")
		}
		indent := runtimeTraceProjRowContinuationIndent(row)
		for _, cont := range runtimeTraceProjSubordinatePackedLines(indent, packed) {
			line += "\n" + cont
		}
		return line
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
	// PTV6-D (a) 悬崖消除 (打包非折叠 red line): the main line keeps every
	// MainRow Keep 记号 unconditionally PLUS the leading ordinary tags that
	// still fit beside them — each candidate is judged against the SAME
	// reserve-aware budget (Med-1: one budget for every width judgment) with
	// ALL remaining MainRow tags included, so an accepted tag can never push a
	// Keep 记号 over the cap. Prefix discipline: after the first ordinary tag
	// demotes, every later ordinary tag demotes too — the reading order stays
	// main line → subordinate stream (first-fit would reorder information
	// across the fold). Every demoted tag renders whole below (packed).
	var mainTexts, demoted []string
	for i, tag := range tags {
		if tag.MainRow {
			mainTexts = append(mainTexts, tag.Text)
			continue
		}
		if len(demoted) > 0 {
			demoted = append(demoted, tag.Text)
			continue
		}
		trial := append(append([]string(nil), mainTexts...), tag.Text)
		for _, later := range tags[i+1:] {
			if later.MainRow {
				trial = append(trial, later.Text)
			}
		}
		if runewidth.StringWidth(left+" "+base+"  "+strings.Join(trial, " · ")) <= mainBudget {
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
				// zh 103 / en 112 at ancestors=3. PTV8-LAD L4 (§24.11 维度B)
				// EVOLUTION RECORD: the former "plateauing from ancestors ≥ 5"
				// claim relied on the label column cap alone and huadong_78
				// FALSIFIED it (B1-b fold splitting grew rails without bound —
				// w=144 measured); the plateau is now structural by
				// construction: rails cap at runtimeTraceProjTreeIndentCap
				// levels and the name keeps its 8-cell floor, so the overflow
				// ceiling is bounded at every depth. Recorded as-is; the T1
				// integrity floor keeps those marks whole rather than
				// truncating them.
				squeezed := strings.Join(strings.Fields(base), " ")
				trimmed = strings.TrimRight(left, " ") + " " + squeezed + " " + strings.Join(mainTexts, " · ")
			}
			line = trimmed
		}
	}
	indent := runtimeTraceProjRowContinuationIndent(row)
	for _, cont := range runtimeTraceProjSubordinatePackedLines(indent, demoted) {
		line += "\n" + cont
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
	// PTV8-LAD L4: the capped shared rail builder bounds the subordinate lead,
	// so the "· " payload width has a real floor at every depth (the huadong_78
	// E7 notes shredded into a 20-cell column under a ~78-cell lead).
	b.WriteString(runtimeTraceProjAncestorRails(row.Ancestors))
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
// never truncation — the wrap keeps a break space at its chunk's end and the
// EMITTED physical line trims that invisible trailing space, DISPLAY-WRAP
// 件①(f)/catalog C1).
func runtimeTraceProjSubordinateLines(indent, text string) []string {
	lead := indent + "· "
	width := runtimeTraceProjTreeRowMaxWidth - runewidth.StringWidth(lead)
	if width < runtimeTraceProjTreeNameMinWidth {
		width = runtimeTraceProjTreeNameMinWidth
	}
	chunks := runtimeTraceProjWrapDisplay(text, width)
	out := make([]string, 0, len(chunks))
	prefix := lead
	for i, chunk := range chunks {
		if i > 0 {
			// A break space heads its continuation chunk (byte-identity form)
			// and the emitted line drops it (件①(f) boundary-space trim).
			chunk = strings.TrimLeft(chunk, " ")
		}
		out = append(out, strings.TrimRight(prefix+chunk, " "))
		prefix = indent + "  "
	}
	return out
}

// runtimeTraceProjIdentityGroupLines renders the 行2 identity line under
// width pressure by its SEMANTIC GROUPS (DISPLAY-WRAP 件①(c), §29.104.18
// 修向②, user-requested 分行显示 form): a fitting row keeps the single-line
// form byte-identically; an over-wide row breaks BETWEEN groups only —
// greedy whole-group packing, every continuation line OPENS with the `·`
// separator (never a dangling tail, 形1) and a short tail group moves down
// whole (形2 短尾防孤 by construction). A group alone wider than the width
// falls back to the chip-boundary wrap inside itself.
func runtimeTraceProjIdentityGroupLines(indent string, groups []string, zh bool) []string {
	sep := "·"
	if !zh {
		sep = " · "
	}
	full := strings.Join(groups, sep)
	lead := indent + "· "
	width := runtimeTraceProjTreeRowMaxWidth - runewidth.StringWidth(lead)
	if width < runtimeTraceProjTreeNameMinWidth {
		width = runtimeTraceProjTreeNameMinWidth
	}
	if len(groups) < 2 || runewidth.StringWidth(full) <= width {
		return runtimeTraceProjSubordinateLines(indent, full)
	}
	opener := strings.TrimLeft(sep, " ")
	lines := []string{groups[0]}
	for _, group := range groups[1:] {
		if cand := lines[len(lines)-1] + sep + group; runewidth.StringWidth(cand) <= width {
			lines[len(lines)-1] = cand
			continue
		}
		lines = append(lines, opener+group)
	}
	out := make([]string, 0, len(lines))
	prefix := lead
	for _, line := range lines {
		for i, chunk := range runtimeTraceProjWrapDisplay(line, width) {
			if i > 0 {
				chunk = strings.TrimLeft(chunk, " ")
			}
			out = append(out, strings.TrimRight(prefix+chunk, " "))
			prefix = indent + "  "
		}
	}
	return out
}

// runtimeTraceProjSubordinatePackedLines renders the demoted tags as a PACKED
// "· " subordinate stream (PTV6-D (a), 打包非折叠): tags flow in order into
// 100-cell lines separated by " · " — a new line starts ONLY when the next
// tag does not fit the current one (every physical line begins at a tag
// boundary with the "· " marker). A tag wider than a whole fresh line
// atom-wraps through runtimeTraceProjSubordinateLines (T3 wrap, never
// truncation) and the following tag opens a fresh "· " line — a packed
// neighbor never rides a mid-tag continuation line. Every tag renders whole,
// in order; packing changes line geometry only, never content.
func runtimeTraceProjSubordinatePackedLines(indent string, texts []string) []string {
	// PTV8-RCR-B (UXA 域A layout-⑥, 2026-07-08). EVOLUTION RECORD: the packed
	// stream used to break purely by width (up to four calibers glued on one
	// line) — a subordinate line now holds AT MOST TWO notes; width pressure
	// still breaks earlier, and an over-wide single note still wraps whole.
	var out []string
	line := ""
	packed := 0
	flush := func() {
		if line != "" {
			// DISPLAY-WRAP 件①(f): emitted physical lines never carry
			// trailing spaces (catalog C1).
			out = append(out, strings.TrimRight(line, " "))
			line = ""
			packed = 0
		}
	}
	for _, text := range texts {
		if line != "" {
			if cand := line + " · " + text; packed < 2 &&
				runewidth.StringWidth(cand) <= runtimeTraceProjTreeRowMaxWidth {
				line = cand
				packed++
				continue
			}
			flush()
		}
		if cand := indent + "· " + text; runewidth.StringWidth(cand) <= runtimeTraceProjTreeRowMaxWidth {
			line = cand
			packed = 1
			continue
		}
		out = append(out, runtimeTraceProjSubordinateLines(indent, text)...)
	}
	flush()
	return out
}

// runtimeTraceProjWrapAtomCompounds (PTV8-LAD L5, §24.11 维度A F5 / 维度B
// P2-1, DL2 先例扩展) registers the four-line-grammar compound words whose
// mid-word break the huadong_78 E7 notes exhibited ("有效归\n因" / "下\n界" /
// "根因\n排序#9"): each entry fuses into ONE wrap atom, so a break can never
// bisect the claim — even at a capped-boundary width. The closed set mirrors
// the §24.1/§24.2 grammar vocabulary (rank ordinal, attribution words, caliber
// words, confidence tiers); it is a display-wrap boundary table only — never
// a parser and never a gate. Longest-first inside shared prefixes.
var runtimeTraceProjWrapAtomCompounds = []string{
	// UXG-1 修复轮 F2 (2026-07-12): the two live ordinal-chip atoms DERIVE
	// from the tracefence channel words — this table is a byte-form consumer
	// (a channel-word edit that skipped it would silently un-fuse the chip at
	// wrap boundaries, the most dangerous drift face). The retired 背景榜位#
	// entry stays a literal: it matches legacy persisted payload replays only
	// and has no tracefence row by design (§29.36.2 chip retirement).
	tracefence.SeatChannelChainZH + "#", // fuses with the trailing rank digits (根因排序#9 is one atom)
	// UXR-1 (§29.36.2): the adjacent-channel ordinal chip fuses with its
	// digits like the rank ordinal above (邻近影响#3 is one atom).
	tracefence.SeatChannelAdjacentZH + "#",
	"背景榜位#",
	"有效归因",
	"承自归因",
	"链上累计",
	// R5 (§29.88.12 单基准, 2026-07-15): the unified basis words replace
	// 按大核满频/按下游消费核/按小核·中核·超大核满频 (retired with their
	// algorithms). Longest-first inside the shared 按全域 prefix; the R5b
	// mention word joins too (a wrap must never bisect 运行频点非最高
	// mid-claim).
	"按全域最大核最高频",
	"按全域最高频",
	"运行频点非最高",
	// CAP (§26 C3): the capability disclosure words join the unbreakable set
	// (a wrap must never bisect 默认算力比/纯频率比 mid-claim). Longest-first
	// inside the shared 按 prefix family.
	"按默认算力比粗算",
	"按纯频率比折算",
	// UXR-1 §29.36.4 ①: the compressed no-deficit parenthetical's short form
	// (簇结构不可判,按频率比) — the long form above matches first.
	"按频率比",
	// CAP-2 (§28.4/§28.5): the structure-evidence upgrade words and the THERM
	// press words join the unbreakable set (a wrap must never bisect
	// 共动分簇/锚点连续推定/受热限压至 mid-claim). Longest-first inside the
	// shared 按 prefix family is immaterial here (all diverge by rune 2).
	"按实测频点共动分簇折算",
	"按簇轨实测折算",
	"锚点连续推定",
	"受热限压至",
	"簇结构不可判",
	"跨窗取最大",
	"单次最大",
	// RCM-2 D1: the family caliber vocabulary joins the unbreakable set (a
	// wrap must never bisect 合计/成员最大/同线程 mid-claim). 计数合计 keeps
	// its own entry — the bare 合计 entry cannot match at its 计 start rune.
	"计数合计",
	// DISP-2 / GAP-A P3-6 (2026-07-09): the count-class comparison marker the
	// engine roster/Σ-note faces print (rootCauseCountEquivalentValue) — a
	// wrap must never bisect 计数当量 mid-claim (G8 折行劈词 family). Shares
	// the 计数 prefix with 计数合计 above; they diverge at the third rune, so
	// order between the two is immaterial.
	"计数当量",
	"成员最大",
	// RSPA §29.61.10a/b (2026-07-14): the same-source bipartition word heads
	// join the unbreakable set — the 行2 disclosure head (同源二分:全窗X…)
	// and the relation sentence's additive-identity claim must never bisect
	// at a wrap boundary (they are the bidirectional legend probes). The
	// 合计还原全窗账 entry sits BEFORE the bare 合计 entry (longest-first
	// inside the shared 合计 prefix); 同源二分:全窗 diverges from 同线程 at
	// rune 2, so order there is immaterial.
	"同源二分:全窗",
	"合计还原全窗账",
	// INV-SUPPLY 件① (§29.61.11, 2026-07-14): the compound type-word suffix
	// joins the unbreakable set — a 行2 wrap must never bisect 供给缺口主导
	// mid-claim (G8 折行劈词 family). No shared prefix with the 供给折算缺口
	// clause words (those live in whole clauses, not this atom table).
	"供给缺口主导",
	"合计",
	"同线程",
	"重叠未拆",
	"重叠段已并",
	"原始和",
	"见明细",
	"置信高", "置信中", "置信低",
	"下界",
	"全额",
	"折算",
	// GAP-B G8(b) (§27.3, 2026-07-09): the BARE core-class caliber words join
	// the unbreakable set — they appear alone in the CAP capability legend
	// ("中核=小核×2.3…") and the supply-fold clause ("降频/小核导致的跑慢
	// 成分"), and the per-rune CJK atomizer split them mid-claim ("小/核").
	// 超大核 leads its shared-suffix family (longest-first discipline: at the
	// 超 rune only 超大核 can match, but the entry order keeps the table's
	// stated invariant legible).
	"超大核",
	"大核",
	"中核",
	"小核",
}

func init() {
	// WF-xn (§29.52.1, 2026-07-12): the merge-count family word heads derive
	// from the tracefence display-table ⑥ single source (次跨窗取最大 / 次同值
	// / 次union / 线程取最大 / 次) — a family-word edit that skipped this
	// table would silently un-fuse the count from its word at wrap
	// boundaries. Appended AFTER the literal table so existing longest-first
	// relationships are untouched; the family shares no prefixes with the
	// literals above.
	runtimeTraceProjWrapAtomCompounds = append(runtimeTraceProjWrapAtomCompounds,
		tracefence.MergeCountWrapAtoms()...)
}

// runtimeTraceProjWrapCompoundAt reports the registered compound starting at
// runes[i] ("" / 0 when none). The 根因排序# entry additionally swallows the
// trailing ordinal digits so "#9" can never open a line without its word.
func runtimeTraceProjWrapCompoundAt(runes []rune, i int) (string, int) {
	for _, compound := range runtimeTraceProjWrapAtomCompounds {
		cr := []rune(compound)
		if i+len(cr) > len(runes) {
			continue
		}
		match := true
		for j, r := range cr {
			if runes[i+j] != r {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		n := len(cr)
		if compound == tracefence.SeatChannelChainZH+"#" || compound == tracefence.SeatChannelAdjacentZH+"#" || compound == "背景榜位#" {
			for i+n < len(runes) && runes[i+n] >= '0' && runes[i+n] <= '9' {
				n++
			}
		}
		return string(runes[i : i+n]), n
	}
	return "", 0
}

// runtimeTraceProjMergeCountFamilyAtom reports whether a wrap atom is a
// WF-xn merge-count family word (tracefence display-table ⑥) — the atoms
// that fuse with their leading count digits.
func runtimeTraceProjMergeCountFamilyAtom(text string) bool {
	for _, word := range tracefence.MergeCountWrapAtoms() {
		if text == word {
			return true
		}
	}
	return false
}

// runtimeTraceProjCaliberDedupEntry is one DISPLAY-WRAP 件③(a) same-node
// repeat-suppression rule: the node's first occurrence of full stays; later
// same-node occurrences rewrite to short (mark-gated legend reference word).
type runtimeTraceProjCaliberDedupEntry struct {
	full, short string
	mark        runtimeTraceProjMark
}

// runtimeTraceProjCaliberDedupEntries — the CLOSED phrase set (§29.104.18.1
// B2 census: 按实测频点共动分簇折算 ×30 / 按全域最大核最高频 ×26 witness
// repeats, 2-3 per node). 受热限压至 is deliberately absent: its long unit is
// the whole THERM clause, which renders at most once per node — there is no
// same-node repetition to suppress (cross-node repetition is out of scope,
// DISPLAY-HYG). Byte forms mirror the single-source producers
// (runtimeTraceProjFoldBasisWord / runtimeTraceProjCapabilityCaliberClauseTopo).
func runtimeTraceProjCaliberDedupEntries(zh bool) []runtimeTraceProjCaliberDedupEntry {
	if zh {
		return []runtimeTraceProjCaliberDedupEntry{
			{"按实测频点共动分簇折算", "分簇口径同前", runtimeTraceProjMarkCaliberStatedClustering},
			{"按全域最大核最高频", "按前述基准", runtimeTraceProjMarkCaliberStatedBasis},
		}
	}
	return []runtimeTraceProjCaliberDedupEntry{
		{"measured co-moving frequency clusters (default capability ratios)",
			"same clustering caliber as stated", runtimeTraceProjMarkCaliberStatedClustering},
		{"the global max-core peak frequency", "the stated basis", runtimeTraceProjMarkCaliberStatedBasis},
	}
}

// runtimeTraceProjDedupNodeCaliberPhrases rewrites ONE node's display texts
// in display order (DISPLAY-WRAP 件③(a)): per registered phrase, the first
// occurrence stays in full and every later occurrence collapses to the
// legend-taught reference word. Values and judgments never change — the pass
// touches the phrase bytes only. Marks fire exactly when a rewrite happened
// (词条-图例双向).
func runtimeTraceProjDedupNodeCaliberPhrases(texts []*string, marks *runtimeTraceProjMarkSet, zh bool) {
	for _, e := range runtimeTraceProjCaliberDedupEntries(zh) {
		seen := false
		for _, t := range texts {
			if t == nil || !strings.Contains(*t, e.full) {
				continue
			}
			var b strings.Builder
			s := *t
			changed := false
			for {
				i := strings.Index(s, e.full)
				if i < 0 {
					b.WriteString(s)
					break
				}
				if !seen {
					seen = true
					b.WriteString(s[:i+len(e.full)])
				} else {
					b.WriteString(s[:i])
					b.WriteString(e.short)
					changed = true
				}
				s = s[i+len(e.full):]
			}
			if changed {
				*t = b.String()
				if marks != nil {
					marks.mark(e.mark)
				}
			}
		}
	}
}

// runtimeTraceProjWrapCJKWordRune (DISPLAY-WRAP 件①(a), §29.104.18.1 A1,
// 2026-07-16) reports whether a rune may join a CJK word-run atom: any
// non-ASCII rune EXCEPT the `·` chip separator and the CJK punctuation set
// (those stay their own atoms so the punct-aware break rules keep firing).
// Display-wrap boundary predicate only — never a parser and never a gate.
func runtimeTraceProjWrapCJKWordRune(r rune) bool {
	if r < 0x80 || r == '·' {
		return false
	}
	switch r {
	case '，', '。', '；', '：', '、', '（', '）', '《', '》', '「', '」', '『', '』', '【', '】', '…', '—', '～', '？', '！':
		return false
	}
	return true
}

// runtimeTraceProjWrapWordAtom reports whether a wrap atom is a CJK word
// shape (word-run or registered compound) — the left half of the 件①(d)
// word+value unbreakable pair (「受热限压至 1.88GHz」/「合计 20.816ms」/
// 「板锚 .ugc.aweme.lite-17267」: a break between a word and the value it
// qualifies orphans the value, catalog 形4).
func runtimeTraceProjWrapWordAtom(text string) bool {
	for _, r := range text {
		return runtimeTraceProjWrapCJKWordRune(r)
	}
	return false
}

// runtimeTraceProjWrapValueAtom (GAP-B G8(a), §27.3, 2026-07-09) reports
// whether a wrap atom is a VALUE shape — a pure-ASCII run carrying at least
// one digit and ending alphanumeric/percent ("0.058ms", "37.410", "49%") —
// the shapes whose immediately-following short caliber parenthetical fuses
// into one super-atom (a value and its caliber are one claim; the orphan
// "(全额)" line was the huadong_79 witness).
func runtimeTraceProjWrapValueAtom(text string) bool {
	if text == "" {
		return false
	}
	hasDigit := false
	for _, r := range text {
		if r >= 0x80 {
			return false
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	last := text[len(text)-1]
	return hasDigit && (last >= '0' && last <= '9' ||
		last >= 'a' && last <= 'z' || last >= 'A' && last <= 'Z' || last == '%')
}

// runtimeTraceProjWrapDisplay splits text into display chunks of at most
// width cells, breaking ONLY at atom boundaries (PTV4 T3): an atom is a
// maximal run of ASCII non-space, non-`·` runes — tokens like "14.597ms"
// never split — or a maximal run of CJK word runes (DISPLAY-WRAP 件①(a),
// §29.104.18.1 A1: two-rune words like 为主/证据/对象/该簇 can never bisect —
// the former per-rune "CJK wraps naturally" atomization IS the word-internal
// break) — or a registered compound word (PTV8-LAD L5, the table above);
// spaces, `·` separators and CJK punctuation are break opportunities. Chunk
// concatenation is BYTE-IDENTICAL to the input (a break space stays at the
// end of its chunk — wrap only, never loss; the physical-line emitters trim
// the invisible trailing break space, 件①(f)/C1). An atom wider than the
// whole width owns its own line(s) and hard-splits only then (unavoidable,
// deterministic).
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
		// PTV8-LAD L5: registered compounds fuse before per-rune atomization
		// (grouping only — byte concatenation stays identical).
		if compound, n := runtimeTraceProjWrapCompoundAt(runes, i); n > 0 {
			atoms = append(atoms, atom{text: compound, w: runewidth.StringWidth(compound)})
			i += n
			continue
		}
		r := runes[i]
		switch {
		case r == ' ':
			atoms = append(atoms, atom{text: " ", w: 1})
			i++
		case r == '(' || r == ')' || r == ',':
			// PTV8-RCR-B 收尾 (复核 M6, 2026-07-08): ASCII brackets/commas are
			// their own atoms — an ASCII run like "1.853ms(" used to swallow
			// them, so the punct-aware break rules below never fired on
			// ASCII-adjacent text (the supply-fold clause witness).
			atoms = append(atoms, atom{text: string(r), w: 1})
			i++
		case r < 0x80 && r != '·':
			j := i
			for j < len(runes) && runes[j] < 0x80 && runes[j] != ' ' && runes[j] != '·' &&
				runes[j] != '(' && runes[j] != ')' && runes[j] != ',' {
				j++
			}
			s := string(runes[i:j])
			atoms = append(atoms, atom{text: s, w: runewidth.StringWidth(s)})
			i = j
		case runtimeTraceProjWrapCJKWordRune(r):
			// DISPLAY-WRAP 件①(a): a maximal CJK word run is ONE atom — a
			// break can only land at its edges (punctuation / ASCII / `·` /
			// space / a registered compound start), never inside a word. The
			// run stops where a compound begins so the compound keeps its own
			// atom (its protection against rune-level hard splits survives).
			j := i + 1
			for j < len(runes) && runtimeTraceProjWrapCJKWordRune(runes[j]) {
				if _, n := runtimeTraceProjWrapCompoundAt(runes, j); n > 0 {
					break
				}
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
	// WF-xn (§29.52.1, 2026-07-12): a merge-count family word fuses with its
	// LEADING count digits ("4次(…)" / "6线程取最大(…)" / "n=…" ASCII runs
	// stay whole by construction) — the count and its word are one claim, a
	// break between them would orphan the word. Grouping only; byte
	// concatenation stays identical.
	for i := 1; i < len(atoms); i++ {
		if !runtimeTraceProjMergeCountFamilyAtom(atoms[i].text) {
			continue
		}
		prev := atoms[i-1].text
		if prev == "" || prev[len(prev)-1] < '0' || prev[len(prev)-1] > '9' {
			continue
		}
		atoms[i-1] = atom{text: prev + atoms[i].text, w: atoms[i-1].w + atoms[i].w}
		atoms = append(atoms[:i], atoms[i+1:]...)
		i--
	}
	// PTV8-RCR-B (UXA 域D layout-L2, 2026-07-08). EVOLUTION RECORD: the wrap
	// used to break inside parentheses ("调度压力(需求积压\n)" / a lone "折算)"
	// opening a line) — closing punctuation never starts a line and an opening
	// bracket never ends one: the offending atoms move down WITH their
	// neighbor. Byte concatenation stays identical (break position only).
	closePunct := map[string]bool{")": true, "）": true, ",": true, "，": true, "、": true, ";": true, "；": true, "。": true}
	// DISPLAY-WRAP 件①(b) (§29.104.18 形1, 2026-07-16): the `·` chip separator
	// joins the "never ends a line" set — a chip-chain break moves the
	// separator DOWN so the continuation opens with `·` (the witness form
	// 「…板锚 X·\n置信高」 dangled it at EOL six times). Same carry lanes as
	// the opening brackets.
	openPunct := map[string]bool{"(": true, "（": true, "·": true}
	// PTV8-RCR-C (§24.9 G2/G3 随批, DL2 家族, 2026-07-08): a SHORT ASCII
	// parenthetical group fuses into ONE atom ("(in full)" — the caliber
	// words' EN faces) so a mid-parenthetical space can never split a caliber
	// claim across lines ("60.000ms(in\n full)"). Non-nested "( … )" runs of
	// ≤12 display cells only; longer parentheticals keep the L2 punct-aware
	// break rules. Byte concatenation stays identical (grouping only).
	for i := 0; i < len(atoms); i++ {
		if atoms[i].text != "(" {
			continue
		}
		w := atoms[i].w
		end := -1
		for j := i + 1; j < len(atoms) && w <= 12; j++ {
			w += atoms[j].w
			if atoms[j].text == "(" {
				break
			}
			if atoms[j].text == ")" {
				end = j
				break
			}
		}
		if end < 0 || w > 12 {
			continue
		}
		var b strings.Builder
		for j := i; j <= end; j++ {
			b.WriteString(atoms[j].text)
		}
		atoms[i] = atom{text: b.String(), w: w}
		atoms = append(atoms[:i+1], atoms[end+1:]...)
		// GAP-B G8(a) (§27.3, 2026-07-09): a short parenthetical that
		// immediately follows a VALUE atom (an ASCII run carrying a digit —
		// "0.058ms", "37.410") binds to it as ONE super-atom, so the caliber
		// note can never open a line as an orphan ("0.058ms\n(全额)" — the
		// huadong_79 witness). Values and their calibers are one claim; byte
		// concatenation stays identical (grouping only). Word+parenthetical
		// pairs ("runnable(全额)") keep the plain fusion — only the
		// value-caliber bond is load-bearing here.
		if i > 0 && runtimeTraceProjWrapValueAtom(atoms[i-1].text) {
			atoms[i-1] = atom{text: atoms[i-1].text + atoms[i].text, w: atoms[i-1].w + atoms[i].w}
			atoms = append(atoms[:i], atoms[i+1:]...)
			i--
		}
	}
	// DISPLAY-WRAP 件①(d) E# 引用 (§29.104.18 修向④, 2026-07-16): a bracketed
	// evidence reference ("[E13(+1)+E52]") fuses into ONE atom — the ASCII
	// paren split above fragments it ("[E13(+1)" + "+E52]") and a break inside
	// the bracket orphans half a reference. "[E…" through the first "]"-tailed
	// atom, ≤24 display cells; wider forms keep the punct-aware rules. Byte
	// concatenation stays identical (grouping only). Every fusion cap below
	// additionally bounds by the LINE width: at hostile narrow widths a fused
	// super-atom would only reach the rune-level hard-split lane — the
	// un-fused token-boundary break is strictly better there.
	fusionCap := func(cap int) int {
		if width < cap {
			return width
		}
		return cap
	}
	refCap := fusionCap(24)
	for i := 0; i < len(atoms); i++ {
		if !strings.HasPrefix(atoms[i].text, "[E") || strings.HasSuffix(atoms[i].text, "]") {
			continue
		}
		w := atoms[i].w
		end := -1
		for j := i + 1; j < len(atoms) && w <= refCap; j++ {
			w += atoms[j].w
			if strings.Contains(atoms[j].text, "[") {
				break
			}
			if strings.HasSuffix(atoms[j].text, "]") {
				end = j
				break
			}
		}
		if end < 0 || w > refCap {
			continue
		}
		var b strings.Builder
		for j := i; j <= end; j++ {
			b.WriteString(atoms[j].text)
		}
		atoms[i] = atom{text: b.String(), w: w}
		atoms = append(atoms[:i+1], atoms[end+1:]...)
	}
	// DISPLAY-WRAP 件①(d) 词+数值 (catalog 形4, 2026-07-16): a CJK word atom
	// directly qualifying a VALUE atom (joined by nothing or one space) fuses
	// into ONE claim — 「受热限压至 ␠\n1.88GHz」 / 「有效归因\n3.429ms」 broke
	// the value off its word. ≤28 display cells (the 板锚+thread-label chip
	// fits; degenerate long pairs keep the word-boundary break). Byte
	// concatenation stays identical (grouping only).
	// The pair's right half is a VALUE atom or a value+caliber super-atom
	// minted by the GAP-B pass above ("1.347ms(全额)" — the caliber already
	// fused onto its value; the word joins the same one claim).
	wordPairValue := func(text string) bool {
		if runtimeTraceProjWrapValueAtom(text) {
			return true
		}
		if i := strings.IndexByte(text, '('); i > 0 && strings.HasSuffix(text, ")") {
			return runtimeTraceProjWrapValueAtom(text[:i])
		}
		return false
	}
	for i := 0; i+1 < len(atoms); i++ {
		if !runtimeTraceProjWrapWordAtom(atoms[i].text) {
			continue
		}
		j := i + 1
		if atoms[j].text == " " && j+1 < len(atoms) {
			j++
		}
		if !wordPairValue(atoms[j].text) {
			continue
		}
		w := 0
		var b strings.Builder
		for k := i; k <= j; k++ {
			w += atoms[k].w
			b.WriteString(atoms[k].text)
		}
		if w > fusionCap(28) {
			continue
		}
		atoms[i] = atom{text: b.String(), w: w}
		atoms = append(atoms[:i+1], atoms[j+1:]...)
	}
	// DISPLAY-WRAP 件①(d) 顿号清单项 (catalog 形3, 2026-07-16): a 、-joined
	// list chains into ONE atom (「E34、E35(+1)、\nE36(+1)」 orphaned the tail
	// reference). Neighbor atoms must touch the 、 (no spaces); ≤30 display
	// cells per fused chain. Byte concatenation stays identical.
	for i := 1; i+1 < len(atoms); i++ {
		if atoms[i].text != "、" || atoms[i-1].text == " " || atoms[i+1].text == " " {
			continue
		}
		w := atoms[i-1].w + atoms[i].w + atoms[i+1].w
		if w > fusionCap(30) {
			continue
		}
		atoms[i-1] = atom{text: atoms[i-1].text + atoms[i].text + atoms[i+1].text, w: w}
		atoms = append(atoms[:i], atoms[i+2:]...)
		i--
	}
	// DISPLAY-HYG 二轮 复核件2 (§29.114 P3 「[E5]= 悬行尾」 twin face,
	// 2026-07-17): a bare "=" atom fuses LEFT with its anchor (plus the
	// single separating space — the EN "anchor = rhs" form), so the operator
	// can neither open a continuation naked (the width-57 EN witness broke
	// between "[E8]" and " = ") nor end a line away from its anchor — the
	// fused atom ends "=", which the breakLine "="-tail carry below moves
	// whole. ≤20 display cells; a wider anchor keeps the token-boundary
	// break and the operator may then still strand (the same honest bound as
	// every width-capped fusion). Byte concatenation stays identical.
	eqCap := fusionCap(20)
	for i := 1; i < len(atoms); i++ {
		if atoms[i].text != "=" {
			continue
		}
		j := i - 1
		if atoms[j].text == " " && j > 0 {
			j--
		}
		if atoms[j].text == " " || openPunct[atoms[j].text] || closePunct[atoms[j].text] ||
			atoms[j].text == "=" {
			continue
		}
		w := 0
		for k := j; k <= i; k++ {
			w += atoms[k].w
		}
		if w > eqCap {
			continue
		}
		var fused strings.Builder
		for k := j; k <= i; k++ {
			fused.WriteString(atoms[k].text)
		}
		atoms[j] = atom{text: fused.String(), w: w}
		atoms = append(atoms[:j+1], atoms[i+1:]...)
		i = j
	}
	var out []string
	var lineAtoms []atom
	lineW := 0
	flush := func() {
		if len(lineAtoms) > 0 {
			var b strings.Builder
			for _, la := range lineAtoms {
				b.WriteString(la.text)
			}
			out = append(out, b.String())
		}
		lineAtoms = nil
		lineW = 0
	}
	appendAtom := func(a atom) {
		lineAtoms = append(lineAtoms, a)
		lineW += a.w
	}
	breakLine := func(next atom) []atom {
		// Pull trailing open-brackets (and, for a closing-punct next atom, the
		// whole trailing punct chain plus one anchor atom) down to the next
		// line so no line ends "(" or starts ")". Loop form (复核 M6): a chain
		// like "…下界)" + next "," moves down together. The close-chain pop
		// never empties the line (its anchor stays); the open-punct pop MAY
		// empty it (PTV8-LAD L5 co-repair: a line holding only "(" must not
		// flush alone when the next compound atom fills the width — the empty
		// line then flushes as a no-op and the caller's all-open lane fuses
		// the carry into a hard split; no re-entry, no loop).
		var carry []atom
		if closePunct[next.text] {
			for len(lineAtoms) > 1 {
				last := lineAtoms[len(lineAtoms)-1]
				lineAtoms = lineAtoms[:len(lineAtoms)-1]
				lineW -= last.w
				carry = append([]atom{last}, carry...)
				if !closePunct[last.text] && !openPunct[last.text] {
					break
				}
			}
		}
		// DISPLAY-WRAP 件①(b)/(f): trailing SPACES travel down with the carry
		// too — a break at an EN " · " separator used to leave "… ·" (after
		// the C1 trim) at EOL; the space+separator now HEAD the continuation
		// ("· window …") and byte concatenation stays identical (the emitters
		// trim the invisible boundary spaces).
		//
		// DISPLAY-HYG 二轮 (§29.114 P3 「[E5]= 悬行尾」, 2026-07-17): an atom
		// ending "=" never ends a line either — the "=" promises a right-hand
		// side, so 「…,[E8]=\n按链上聚合归账」 bisected the assignment claim
		// right after its operator. A self-anchored ref/key atom ("[E8]=",
		// "row_window=") carries itself down and stops; a BARE "=" atom
		// additionally pulls its left-hand anchor so the continuation never
		// opens with a naked operator. Byte concatenation stays identical
		// (break position only).
		for len(lineAtoms) > 0 {
			last := lineAtoms[len(lineAtoms)-1]
			eqTail := strings.HasSuffix(last.text, "=")
			if last.text != " " && !openPunct[last.text] && !eqTail {
				break
			}
			lineAtoms = lineAtoms[:len(lineAtoms)-1]
			lineW -= last.w
			carry = append([]atom{last}, carry...)
			if eqTail {
				if last.text == "=" {
					for len(lineAtoms) > 1 && lineAtoms[len(lineAtoms)-1].text == " " {
						sp := lineAtoms[len(lineAtoms)-1]
						lineAtoms = lineAtoms[:len(lineAtoms)-1]
						lineW -= sp.w
						carry = append([]atom{sp}, carry...)
					}
					if len(lineAtoms) > 0 {
						anchor := lineAtoms[len(lineAtoms)-1]
						if anchor.text != " " && !openPunct[anchor.text] {
							lineAtoms = lineAtoms[:len(lineAtoms)-1]
							lineW -= anchor.w
							carry = append([]atom{anchor}, carry...)
						}
					}
				}
				break
			}
		}
		flush()
		return carry
	}
	// hardSplit renders text as its own line(s), splitting by runes only (no
	// boundary inside it fits the width). Shared by the over-wide-atom lane
	// and the PTV8-LAD L5 open-punct carry lane below.
	hardSplit := func(text string) {
		part := []rune(text)
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
	}
	// popOpenPunctCarry pulls the line's trailing open-punct chain (PTV8-LAD
	// L5 co-repair: compound atoms like the 10-cell 按大核满频 made the
	// hard-split lanes reachable right after an opening bracket — the flushed
	// line must never end "(", so the chain travels INTO the split; byte
	// concatenation stays identical).
	//
	// NOTE (DISPLAY-WRAP 件① 复核): this carry feeds the rune-blind hardSplit
	// lane — SPACES deliberately do NOT pop here (a carried break space would
	// occupy the split line's head and shift the rune split into a fused
	// token: the " 计数当" / " · 14"+"次跨窗" regressions); they stay at the
	// flushed line's end exactly as before (invisible, emitter-trimmed).
	popOpenPunctCarry := func() string {
		carry := ""
		for len(lineAtoms) > 0 && openPunct[lineAtoms[len(lineAtoms)-1].text] {
			last := lineAtoms[len(lineAtoms)-1]
			lineAtoms = lineAtoms[:len(lineAtoms)-1]
			lineW -= last.w
			carry = last.text + carry
		}
		// DISPLAY-HYG 二轮 (§29.114 P3 「[E5]= 悬行尾」): the "="-tail rule
		// applies on the over-wide lane too — a flushed line must not end
		// with a hanging assignment operator when the RHS hard-splits (the
		// emitters trim trailing spaces, so a "= " tail strands the same
		// way). The carried ref/key shifts the rune split by its own width
		// only; a bare "=" pulls its left-hand anchor exactly as in
		// breakLine. Trailing spaces pop ONLY when an "="-tail sits under
		// them (the plain-space no-pop NOTE above stays in force).
		sp := 0
		for len(lineAtoms)-sp > 0 && lineAtoms[len(lineAtoms)-1-sp].text == " " {
			sp++
		}
		if idx := len(lineAtoms) - 1 - sp; idx >= 0 && strings.HasSuffix(lineAtoms[idx].text, "=") {
			for ; sp > 0; sp-- {
				last := lineAtoms[len(lineAtoms)-1]
				lineAtoms = lineAtoms[:len(lineAtoms)-1]
				lineW -= last.w
				carry = last.text + carry
			}
			last := lineAtoms[len(lineAtoms)-1]
			lineAtoms = lineAtoms[:len(lineAtoms)-1]
			lineW -= last.w
			carry = last.text + carry
			if last.text == "=" {
				for len(lineAtoms) > 1 && lineAtoms[len(lineAtoms)-1].text == " " {
					spat := lineAtoms[len(lineAtoms)-1]
					lineAtoms = lineAtoms[:len(lineAtoms)-1]
					lineW -= spat.w
					carry = spat.text + carry
				}
				if len(lineAtoms) > 0 {
					anchor := lineAtoms[len(lineAtoms)-1]
					if anchor.text != " " && !openPunct[anchor.text] {
						lineAtoms = lineAtoms[:len(lineAtoms)-1]
						lineW -= anchor.w
						carry = anchor.text + carry
					}
				}
			}
		}
		return carry
	}
	for _, a := range atoms {
		if a.w > width {
			// Over-wide single atom: it owns its line(s). Handled BEFORE the
			// normal break so the pathological-width fallback can never strand
			// a carried "(" on its own line.
			carry := popOpenPunctCarry()
			flush()
			hardSplit(carry + a.text)
			continue
		}
		if lineW+a.w > width && lineW > 0 {
			for _, c := range breakLine(a) {
				appendAtom(c)
			}
			if lineW+a.w > width && lineW > 0 {
				// Pathological width: the carried neighbors alone overflow.
				// A pure open-punct line (breakLine's never-empty guard left
				// it behind) fuses into a hard split of the next atom — no
				// line may end "("; anything else takes a plain break.
				allOpen := len(lineAtoms) > 0
				for _, la := range lineAtoms {
					if !openPunct[la.text] {
						allOpen = false
						break
					}
				}
				if allOpen {
					carry := popOpenPunctCarry()
					flush()
					hardSplit(carry + a.text)
					continue
				}
				// DISPLAY-HYG 二轮 (§29.114 P3 「[E5]= 悬行尾」) NOTE: at this
				// pathological width the carried anchor+"=" plus the next
				// atom cannot share any line — fusing them into a hard split
				// would bisect a registered compound (the GAPB cluster-word
				// pin caught exactly that), and atom integrity outranks the
				// no-"="-strand rule. The plain flush below may therefore
				// still strand "=" at EOL when width < anchor+"="+atom; the
				// rule holds at every realistic display width (row cap 68 /
				// ◎ cap 100).
				flush()
			}
		}
		appendAtom(a)
	}
	flush()
	// PTV8-LAD L5 MIRROR, generalized (R5 word-family co-repair, 2026-07-15):
	// no emitted line may OPEN with closing punctuation, whatever lane
	// produced it. The R5 basis words widened the compound atoms (18-cell
	// 按全域最大核最高频 / 14-cell 运行频点非最高), making the latent strand
	// shapes reachable: a close-punct atom right after an over-wide atom's
	// hard split, and the pathological lane where the pulled anchor + punct
	// overflow the width together. ONE enforcement pass: a line's leading
	// close-punct chain moves UP to the previous line when it fits there,
	// else the previous line's smallest non-punct-anchored suffix moves DOWN
	// in front of the chain (with a re-split when the fused line would
	// overflow). Byte concatenation stays identical; lines stay within width.
	fixed := make([]string, 0, len(out))
	for _, line := range out {
		for line != "" {
			r := []rune(line)
			chainEnd := 0
			for chainEnd < len(r) && closePunct[string(r[chainEnd])] {
				chainEnd++
			}
			if chainEnd == 0 || len(fixed) == 0 {
				fixed = append(fixed, line)
				break
			}
			chain := string(r[:chainEnd])
			rest := string(r[chainEnd:])
			prev := fixed[len(fixed)-1]
			if runewidth.StringWidth(prev)+runewidth.StringWidth(chain) <= width {
				// The chain fits on the previous line — move it up.
				fixed[len(fixed)-1] = prev + chain
				line = rest
				continue
			}
			// Steal the previous line's smallest suffix that STARTS with a
			// non-punct rune (the chain's anchor); the whole previous line
			// moves when no earlier non-punct rune remains to keep.
			prevRunes := []rune(prev)
			idx := len(prevRunes) - 1
			for idx > 0 && closePunct[string(prevRunes[idx])] {
				idx--
			}
			stolen := string(prevRunes[idx:])
			remain := string(prevRunes[:idx])
			if remain == "" {
				fixed = fixed[:len(fixed)-1]
			} else {
				fixed[len(fixed)-1] = remain
			}
			head := stolen + chain
			if runewidth.StringWidth(head)+runewidth.StringWidth(rest) <= width {
				line = head + rest
				continue // now starts with a non-punct rune
			}
			fixed = append(fixed, head)
			line = rest
		}
	}
	out = fixed
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
	// Seg is the PTV8-RCR-B (UXA 域D #36, 2026-07-08) five-segment note order
	// for NON-cause rows: ① qualitative (10-12: shape/action word, 持有点,
	// 影响点, roster) → ② position (20: 链上L#) → ③ magnitudes (30-33: ×N →
	// cumulative → discounted → effective/inherited/periodic) → ④ caliber
	// clauses (34: supply fold / full-window coverage). Set at the build site
	// (precise emission identity, never text sniffing); ordinary tags are
	// stable-sorted by Seg before layout, MainRow/OwnLine tags never move.
	Seg int
	// DedupeToken is the tag's TYPED identity for the #6/#12 全词一处 dedupe
	// (PTV6-C 修正轮 Med, 2026-07-06): a tag folds into the row's cause word
	// ONLY when their typed tokens agree (canonical equality or same state
	// family) — display strings never judge (the retired substring lane ate
	// the 裁定4 state tag on the EN face: running ⊂ running_burst). "" =
	// never dedupable; the 裁定4 state tag is thereby deletable exclusively
	// through its own typed identity, never through a display collision.
	DedupeToken string
	// OwnLine marks a PTV8-RCR structured grammar line (行2/行3/拆解子行,
	// §24.1): it never joins 行1 and always renders as its own subordinate
	// "· " line, in tag order, before the packed ordinary tags. A row carrying
	// any OwnLine tag keeps 行1 grammar-clean: only MainRow keep-marks stay
	// inline; every ordinary tag demotes.
	OwnLine bool
	// IdentityGroups (DISPLAY-WRAP 件①(c)): the 行2 identity tag's semantic
	// groups for width-pressure group-boundary splitting (nil on every other
	// tag; see runtimeTraceProjCauseStructured.IdentityGroups).
	IdentityGroups []string
	// Row2 (UXR-1 §29.36④+.4④, user rulings 2026-07-11) marks the 行2
	// 限定词槽: the participation qualifier (上下文·不参与根因排序 et al.)
	// NEVER joins 行1 — the witness family showed it mid-value-column, at the
	// 行1 tail and as a lone orphan line (four position variants). A row
	// carrying a Row2 tag demotes EVERY ordinary tag with it, packed onto the
	// subordinate stream with the qualifier FIRST (限定词前、原因后 — the
	// state word / reason clauses become the qualifier's row-2 followers, so
	// bare 「· running」 orphan tails cannot exist).
	Row2 bool
}

// runtimeTraceProjStateKindLabel is the bar-row state attribution (§7.30
// 裁定4) AND the single authoritative display-alias mapping for the TSH
// StateKind universe (PTV7 #74, 用户裁定 2026-07-06: 内核状态词英文原词化).
// Every universe token maps onto its canonical industry display word —
// s_sleep/sleep/sleep_wait→sleep, d_sleep/d_state/uninterruptible_sleep→
// D-state, io_wait→iowait, running/runnable→themselves — and the word is
// FACE-INVARIANT: zh and en speak the same token (双面分叉消除; the zh
// parameter stays because every wording helper threads the face flag and
// non-state wording homes still localize). The Chinese semantics live at a
// single point: the legend's state-icon entries (☾/sleep ⧖/runnable
// ⚙/running ⛓/D-state·iowait), never repeated per row. Alignment pinned by
// TestPTV7StateKindDisplayAliasUniverseAlignment. Empty when the node
// exposes no StateKind — callers then fall back to the impact-shape cell
// value instead of fabricating a state.
func runtimeTraceProjStateKindLabel(node types.TraceCausalProjectionNode, zh bool) string {
	_ = zh // PTV7: state words are face-invariant tokens.
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait":
		return "sleep"
	case "runnable":
		return "runnable"
	case "running":
		return "running"
	case "io_wait":
		return "iowait"
	case "d_state", "d_sleep", "uninterruptible_sleep":
		return "D-state"
	}
	return ""
}

// runtimeTraceProjFamilyStemCellWidth — DISPLAY-HYG 二轮 复核件1 (catalog
// C3): the family-stem value cell's natural content width (prefix +
// "%.3fms"), 0 on every row outside that exact arm (cross-thread /
// composite / clamped-count / no-value rows keep their own faces). Shared
// by the fence-level slot pre-pass and readable beside the emitting arm so
// the two can never disagree on which rows widen the slot.
func runtimeTraceProjFamilyStemCellWidth(row runtimeTraceProjTreeRow, zh bool) int {
	node := row.Node
	if runtimeTraceProjCrossThreadAggregateType(node) ||
		runtimeTraceProjCompositeValueCaliber(node) ||
		runtimeTraceProjFamilyCountSumClamped(node) {
		return 0
	}
	impact, _ := runtimeTraceProjNodeDisplayImpactSource(node)
	if impact <= 0 &&
		(runtimeTraceProjDiagnosticLaneNode(node) || runtimeTraceProjAllZeroFoldRow(node)) {
		return 0
	}
	prefix := runtimeTraceProjFamilyValuePrefix(node, zh)
	if prefix == "" {
		return 0
	}
	return runewidth.StringWidth(prefix + fmt.Sprintf("%.3fms", impact))
}

// runtimeTraceProjTreeValueSlot computes the fence-wide shared value-cell
// content width: the legacy 11 (" %9.3fms" = 9 + "ms") widened only by
// family-stem cells actually on this render — family-free fences stay
// byte-identical by construction.
func runtimeTraceProjTreeValueSlot(model runtimeTraceProjTreeModel, zh bool) int {
	slot := 11
	consider := func(rows []runtimeTraceProjTreeRow) {
		for _, row := range rows {
			if w := runtimeTraceProjFamilyStemCellWidth(row, zh); w > slot {
				slot = w
			}
		}
	}
	consider(model.TreeRows)
	consider(model.Adjacent)
	consider(model.Background)
	return slot
}

// runtimeTraceProjRowMetricParts renders the fixed metric cells (bar + ms +
// window %) and returns the tag list separately so the caller can fit the tag
// segment into the remaining row-width budget (B4).
func runtimeTraceProjRowMetricParts(row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) (string, []runtimeTraceProjTag) {
	node := row.Node
	// DISPLAY-HYG 二轮 复核件1 (catalog C3): every value arm below pads its
	// cell to the ONE shared slot so the ms tails / % column stay aligned
	// across family and plain rows (0 = legacy 11-cell slot).
	valueSlot := row.ValueSlot
	if valueSlot < 11 {
		valueSlot = 11
	}
	impact, impactSource := runtimeTraceProjNodeDisplayImpactSource(node)
	var b strings.Builder
	crossThread := runtimeTraceProjCrossThreadAggregateType(node)
	compositeValue := runtimeTraceProjCompositeValueCaliber(node)
	// CALSIDE-1 件2 (F7 假单位修, §29.147 独立立案, 2026-07-19): a row whose
	// value renders in a non-wall-clock form (计数当量/综合评分 families) draws
	// NO wall-clock bar and NO window % below — same cross-unit reasoning as
	// the CMP-3 cross-thread arm.
	nonWallClockValue := runtimeTraceProjNonWallClockValueCaliber(node)
	// F5 (§22 PTV7-SPN, 用户裁定 2026-07-07): a value-less diagnostic-lane row
	// (trace_gap 数据盲区 etc.) and the ×N(0.000–0.000) all-zero fold row draw
	// NO bar and NO fake 0.000ms — the cells render the — no-value form the
	// detail table already uses for zero (树表两口径对齐). Typed judgment only
	// (registry lane / numeric fold shape + the display-impact fallback chain).
	noValue := !crossThread && impact <= 0 &&
		(runtimeTraceProjDiagnosticLaneNode(node) || runtimeTraceProjAllZeroFoldRow(node))
	switch {
	case crossThread:
		// CMP-3: a cross-thread cumulative aggregate draws NO bar — its cpu·ms
		// value is not on the wall-clock scale the bar column encodes, so any
		// bar (full, capped or proportional) would misread as a wall-clock
		// share. Blank cells keep the column alignment; the number carries the
		// unit annotation + normalized density instead.
		b.WriteString(strings.Repeat(" ", runtimeTraceProjTreeBarWidth))
	case noValue:
		b.WriteString(strings.Repeat(" ", runtimeTraceProjTreeBarWidth))
	case nonWallClockValue:
		// CALSIDE-1 件2: the ⌗-family bar cell blanks — a count-equivalent /
		// composite-score magnitude drawn against the wall-clock ruler (满格 =
		// 窗口 / 区最大) is a cross-unit visual; the value's caliber word
		// carries the magnitude semantics instead.
		b.WriteString(strings.Repeat(" ", runtimeTraceProjTreeBarWidth))
	default:
		b.WriteString(runtimeTraceProjBar(impact, denom, row.Kind == runtimeTraceProjTreeRowBackground))
	}
	if !crossThread && !noValue && !nonWallClockValue {
		// PTV4 T7 口径组: the bar-scale caliber legend line is gated on a bar
		// actually rendering (cross-thread aggregates and no-value rows draw
		// no bar). PTV8-RCR-B (UXA 域A #13): the windowed and the no-window
		// fallback scales are separate on-demand entries — the same
		// windowMode branch the ScaleNote renders on picks which is taught.
		if windowMode {
			row.marks.mark(runtimeTraceProjMarkBarScale)
		} else {
			row.marks.mark(runtimeTraceProjMarkBarScaleFallback)
		}
	}
	if noValue {
		// Width-matches the shared value slot (legacy " %9.3fms" = 1 + 9 +
		// "ms") so the ms column stays aligned across mixed rows.
		dash := "—"
		if pad := valueSlot - runewidth.StringWidth(dash); pad > 0 {
			b.WriteString(" " + strings.Repeat(" ", pad) + dash)
		} else {
			b.WriteString(" " + dash)
		}
	} else if compositeValue {
		// Unit caliber and seat/tier are independent: io_pressure remains a
		// context-only aggregate while its value shares the suffix-free word
		// face with the legacy block_io_by_inode composite row.
		b.WriteString(" " + runtimeTraceProjCompositeScoreValueText(impact, zh))
	} else if prefix := runtimeTraceProjFamilyValuePrefix(node, zh); prefix != "" {
		// RCM-2 D2 行1 (witness 「✦ VerifyClass ×14 合计7.124ms 9%」): a family
		// row's main-line duration wears the compact caliber stem directly, so
		// the merged magnitude is identifiable at the point of reading; the
		// full 合计(共N段,同线程) word rides 行3 and its legend entry follows
		// the word (marked here too — the stem must never render untaught).
		if _, caliberMark, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok {
			runtimeTraceProjMarkFamilyCaliber(row.marks, caliberMark)
		}
		if runtimeTraceProjFamilyCountSumClamped(node) {
			// §29.55 观察③ 两形一裁 (2026-07-14): the clamped count seat's
			// stem is 计数当量 — its value is not wall-clock ms, so the 行1
			// composite speaks the ONE suffix-free form (same typed condition
			// that minted the stem; 合计/成员最大 wall-clock stems keep ms).
			b.WriteString(" " + runtimeTraceProjCountEquivalentValueText(impact, zh))
		} else {
			// 复核件1 (C3): the stem cell right-aligns inside the shared
			// slot — the slot pre-pass measured THIS cell via the same
			// helper, so it always fits and the ms tail lands on the column.
			cell := prefix + fmt.Sprintf("%.3fms", impact)
			if pad := valueSlot - runewidth.StringWidth(cell); pad > 0 {
				cell = strings.Repeat(" ", pad) + cell
			}
			b.WriteString(" " + cell)
		}
	} else if runtimeTraceProjCountEquivalentValueCaliber(node) {
		// CALSIDE-1 件2 (F7, §29.147): the non-family count-class row's value
		// column spoke a bare wall-clock ms suit (witness 17874:316
		// 「7.200ms 6%」 on a ⌗ 计数当量 row) — it adopts the ONE suffix-free
		// single-source form, the same arm the self 行1 / detail table /
		// ◎ footnote already read (三面同源; the composite arm above is the
		// established precedent shape). Family-clamped seats keep their stem
		// arm above byte-identically.
		row.marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
		b.WriteString(" " + runtimeTraceProjCountEquivalentValueText(impact, zh))
	} else {
		b.WriteString(strings.Repeat(" ", valueSlot-11) + fmt.Sprintf(" %9.3fms", impact))
	}
	if crossThread && !compositeValue {
		b.WriteString(runtimeTraceProjCrossThreadAggregateSuffix(node, denom, windowMode, zh))
	}
	// PTV5 C00: the window-share percentage and the H8 over-window mark are
	// WINDOW-PROJECTION statements — a fallback-sourced value (chain cumulative
	// / effective attribution / actual state) publishes neither: the % would
	// divide a non-projection numerator by the window and could fake a >100%
	// share, pulling the 占窗>100% legend semantics it does not have.
	// CALSIDE-1 件2 (F7, §29.147): the two non-wall-clock value families
	// publish NO window share — the witness 「6%」/「4%」 divided a
	// count-equivalent / composite-score numerator by the wall-clock window
	// (cross-unit fake); the % column stays empty like the other %-less rows.
	semanticSourceWindowTag := ""
	if windowMode && denom > 0 && impact > 0 && !crossThread && !nonWallClockValue &&
		impactSource == runtimeTraceProjImpactSourceWindow {
		// §21.1 CWD-2 ① (huadong_01 revisit E19 witness, 2026-07-07): a merged
		// ×N row whose members span MULTIPLE query windows (typed key:
		// MergedCount>1 ∧ >1 roster windows — the disjoint-window legal SUM,
		// the union and the cross-window MAX calibers alike) has no single
		// window base shared with the anchor denominator, so it renders NO
		// window share — the specimen drew "63%" from a ×14 cross-window sleep
		// SUM (63.831ms whose members straddle two query windows) against one
		// 101ms anchor window. Branch-5 template of
		// runtimeTraceProjCrossThreadDensityWindowMS (不出密度) migrated to
		// the %-face: the bar stays (relative scale), the % cell is
		// suppressed, the legend entry (mark below) says why, and the member
		// windows are disclosed on the lossless block's 窗来源 lane. Rows
		// whose roster resolves to ≤1 known window keep the legacy share
		// byte-identically (绝不跨窗分子÷单锚窗分母打 %).
		if runtimeTraceProjMultiWindowMergedRow(node) {
			row.marks.mark(runtimeTraceProjMarkMergedMultiWindowNoShare)
		} else if base, ok := runtimeTraceProjSemanticSourceWindowShareBaseMS(row, denom); ok {
			// DCS E5 (ledger §23/§23.1 H2): the semantic row carries a typed
			// SOURCE query window that differs from the anchor denominator —
			// the share divides by the source window (the only base the
			// numerator was measured against) and the 来自查询窗 tag below
			// names it. Same-window rows never reach this lane (byte-stable).
			b.WriteString(runtimeTraceProjBarShareText(impact / base * 100))
			row.marks.mark(runtimeTraceProjMarkSemanticSourceWindowShare)
			if impact > base*1.001 {
				row.marks.mark(runtimeTraceProjMarkOverWindowShare)
			}
			semanticSourceWindowTag = runtimeTraceProjSemanticSourceWindowTag(node, zh)
		} else {
			b.WriteString(runtimeTraceProjBarShareText(impact / denom * 100))
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
	}
	var tags []runtimeTraceProjTag
	// R9 (§29.93.2, 2026-07-15): the fold row's line 2 — member preview +
	// 榜位 pointer sink (line 1 keeps the bare counted label; see the name
	// mints in runtimeTraceProjRowNameBase).
	if runtimeTraceProjFoldMemberSinkRow(row) {
		if sink := runtimeTraceProjFoldMemberSinkLine(node, zh); sink != "" {
			tags = append(tags, runtimeTraceProjTag{Text: sink, OwnLine: true})
		}
	}
	if semanticSourceWindowTag != "" {
		// DCS E5: the 来自查询窗 disclosure travels with the % it re-bases.
		tags = append(tags, runtimeTraceProjTag{Text: semanticSourceWindowTag, Seg: 31})
	}
	// PTV5 C00: a fallback-sourced main-line ms carries its (a)-table caliber
	// word as a Keep tag (never demoted away from the number it qualifies);
	// the semantics live in the legend's 口径词 entry. Single-source condition
	// (shared with the name-budget reserve): runtimeTraceProjRowFallbackCaliberWord.
	if word := runtimeTraceProjRowFallbackCaliberWord(node, row.Kind, zh); word != "" {
		row.marks.mark(runtimeTraceProjMarkImpactCaliberFallback)
		// PTV6-C ruling A: a stanza row's fallback caliber word riding the
		// 累计(跨线程) family carries the family's legend entry too (NEW-7:
		// word and legend move together; 实际状态 fallbacks stay out).
		if word == runtimeTraceProjCrossThreadCumWord(zh) {
			row.marks.mark(runtimeTraceProjMarkStanzaCrossThreadCum)
		}
		// RCM-2 D1: a family row's fallback slot speaks the family caliber
		// word (never 累计(跨线程) — F6) and teaches it via its own entry.
		if runtimeTraceProjFamilyRow(node) {
			if _, caliberMark, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok {
				runtimeTraceProjMarkFamilyCaliber(row.marks, caliberMark)
			}
		}
		tags = append(tags, runtimeTraceProjTag{Text: word, MainRow: true})
	}
	// Context-only rows keep their state/duration/evidence on the tree, but
	// name their non-ranking status at the point of reading. This is a typed
	// tier disclosure, not a conclusion inferred from Rank==0.
	if node.IsContextOnlyRow() {
		text := "上下文·不参与根因排序"
		if !zh {
			text = "context · not ranked"
		}
		// UXR-1 §29.36④+.4④: the participation qualifier owns the 行2
		// 限定词槽 — Row2 demotes it (and its reason followers) off 行1
		// unconditionally, killing the four position variants and the bare
		// 「· running」 orphan tails in one rule. Seg 9 keeps it FIRST on the
		// packed row-2 stream (限定词前、原因后).
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 9, Row2: true})
	}
	// V2-P0 ⌗ 口径旁栏 (rank_order_v2_design_20260712.md §6.1 新裁定 A,
	// 2026-07-12): a caliber-side row names its value class where the
	// participation qualifier sits — typed tier disclosure (never inferred
	// from Rank==0), caliber class from the SHARED registry arm.
	// DISPHYG-3 件5 (CALSIDE-1 P3 备案 E29/E30 语序不对称, 2026-07-20).
	// EVOLUTION RECORD: the ⌗ word's seat moves Seg 9 → Seg 13 — the row's
	// CATEGORY word leads 行2 and the ⌗ caliber word follows it, unifying
	// with the majority order (the self ⌗ grammar row's identity line, the
	// name-guarantee relocation shape, and the ◎ footnote all speak
	// category-first with the ⌗ word after). Seg 13 sits after the category/
	// state family (10) and peer-disclosure (11) seats and before the chain
	// chip (20); Seg 12 stays the semantic-span seat (never a ⌗ row).
	if node.IsCaliberSideRow() {
		row.marks.mark(runtimeTraceProjMarkCaliberSideRow)
		// A count-class caliber word speaks 计数当量 — its comparison-form
		// legend entry (计数当量X(非墙钟)) rides along (词条-图例双向; typed
		// class, never a substring probe).
		if tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount {
			row.marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
		}
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjCaliberSideWord(node, zh), Seg: 13, Row2: true})
	}
	// DISPHYG-3 件6 (§29.158 P3-2, 2026-07-20): the two-ruler sentence's
	// non-lead participant seat — a compact merged row whose absorbed board
	// ordinal never rendered — wears the minimal channel-worded
	// cross-reference chip (根因排序#N) so the sentence's 「N 席」 claim is
	// checkable row-by-row. Stamped at model build only (typed unique host
	// match); Seg 14 places it after the state/category family words.
	if chip := runtimeTraceProjSelfTwoRulerParticipantChip(row, zh); chip != "" {
		tags = append(tags, runtimeTraceProjTag{Text: chip, Seg: 14})
	}
	// PTV8-RCR-A (§24.1/§24.2): a cause node renders the four-line grammar —
	// 行2 identity, 行3 「=」breakdown and the 拆解子行 land as OwnLine tags in
	// fixed order; the legacy seats they replace (row-tail shape word, the
	// mechanism sentence on inversion nodes, the plain 有效归因X tag, the
	// ×N(a~b) tag on event-form rows) are suppressed via the typed flags below.
	structured, structuredOK := runtimeTraceProjCauseStructuredParts(row, zh)
	if structuredOK {
		tags = append(tags, runtimeTraceProjTag{Text: structured.IdentityRow, OwnLine: true,
			IdentityGroups: structured.IdentityGroups})
		if structured.Breakdown != "" {
			tags = append(tags, runtimeTraceProjTag{Text: structured.Breakdown, OwnLine: true})
		}
		for _, sub := range structured.SubRows {
			tags = append(tags, runtimeTraceProjTag{Text: sub, OwnLine: true})
		}
	} else if word := runtimeTraceProjMentionFloorWord(row, zh); word != "" {
		// UXR-1 §29.36.3 (通道4): a seat-less on-chain semantic row without
		// the cause grammar still names its mention obligation (the cause-row
		// form carries it inside 行2 — one word source, two seats).
		row.marks.mark(runtimeTraceProjMarkSemanticMentionFloor)
		tags = append(tags, runtimeTraceProjTag{Text: word, Seg: 10})
	}
	// 裁定4: every bar row states WHAT the duration was (typed StateKind label;
	// impact-shape value when no state was exposed — never fabricated).
	// §7.30.3 D1/D3: typed lock-contention rows and gated-composite inversion
	// rows always carry their semantic label — the shape cell wins over any
	// single-state claim (an inversion composite is NOT "running").
	stateTag := runtimeTraceProjStateKindLabel(node, zh)
	// PTV6-C 修正轮 Med (2026-07-06): the state tag's typed dedupe identity —
	// its own StateKind on the label path; on the shape-cell path only the
	// two token-derived arms carry one (inversion word / TypeToken state
	// word, mirrored branch order of ImpactShapeCell with a display-equality
	// branch-consistency check). Lock words, candidate words and aggregate
	// labels stay "" (never dedupable).
	stateTagToken := ""
	if stateTag != "" {
		stateTagToken = runtimeTraceCausalProjectionCanonicalNode(node.StateKind)
	}
	shapeCellUsed := stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
		runtimeTraceCausalProjectionInversionRow(node)
	genericShape := false
	if shapeCellUsed {
		stateTag, genericShape = runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
		switch {
		case strings.TrimSpace(node.BlockingKind) != "":
			stateTagToken = ""
		case runtimeTraceCausalProjectionInversionRow(node):
			stateTagToken = "priority_inversion_candidate"
		case runtimeTraceCausalProjectionTypeTokenStateClass(node) != "" &&
			stateTag == runtimeTraceCausalProjectionTypeTokenStateWord(
				runtimeTraceCausalProjectionRefinedStateClass(node, runtimeTraceCausalProjectionTypeTokenStateClass(node)), zh):
			stateTagToken = runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)
		default:
			stateTagToken = ""
		}
	}
	// PTV4 T4 的 ◦ 无主导态 2-word chip is RETIRED from the row face (PTV6-D
	// (b), #75 标本归因 #10: 7×/标本 pure category repetition) — the ◦ icon is
	// the per-row marker and the legend's ◦ entry (gated on the icon's own
	// IconNoDominant mark at its emission site) carries the class semantics.
	if stateTag != "" {
		switch {
		case genericShape:
			// PTV6-D (b): the generic 候选影响 category word (5×/标本) leaves
			// the row face too — typed branch signal, never a string compare;
			// the class semantics ride the dedicated legend entry below and
			// each row's full shape cell stays lossless in the detail table.
			row.marks.mark(runtimeTraceProjMarkCandidateShapeClass)
		case structuredOK && structured.SuppressShapeWord:
			// PTV8-RCR-A §24.2 行尾形态词撤: the shape word RELOCATED onto the
			// 行2 category slot (typed branch relocation, never a string
			// dedupe) — the row tail stays empty.
		case runtimeTraceProjDFamilyTailRedundant(node):
			// DSTATE-REFINE arm c (件③, 96728 E14/E16): the D-family bare
			// tail's emission point merges into the name lane — a row whose
			// cause name already speaks the family word never re-tags it
			// (同行三面三说法灭); name-silent rows keep the tail above.
		case runtimeTraceProjGenericUnresolvedStateNameWord(node, zh) != "":
			// 76684 行1 修 (SMR-1 批, 2026-07-12): the generic-unresolved
			// name now speaks the state word in 行1 — re-tagging it here
			// would double-speak (same discipline as the D-family arm above).
		default:
			row.marks.mark(runtimeTraceProjMarkStateLabel)
			tags = append(tags, runtimeTraceProjTag{Text: stateTag, DedupeToken: stateTagToken, Seg: 10})
		}
	}
	// 76684 行1 修 (SMR-1 批, 2026-07-12): the unresolved-peer FACT keeps its
	// own demotable carrier when the name slot switched to the state word —
	// 「对端线程未解析」 rides the tag rail (行尾记号位或行2, per the WO) so
	// the honest sentinel disclosure never vanishes (零省略).
	if runtimeTraceProjGenericUnresolvedStateNameWord(node, zh) != "" {
		text := "对端线程未解析"
		if !zh {
			text = "unresolved wait peer"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 11})
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
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 10})
	}
	// foldWindowMS feeds the VS-2 fold verdict/tag below AND the F-4
	// composition-suppression check: window mode's denom IS model.WindowMS.
	foldWindowMS := 0.0
	if windowMode {
		foldWindowMS = denom
	}
	// PTV8-RCR-A (§24 ②, 2026-07-08). EVOLUTION RECORD: the §7.30.3 D3
	// 影响构成 tag is RETIRED — the four-line grammar's 行3 「=」breakdown and
	// its 拆解子行 (OwnLine tags above) carry the inversion composition with
	// per-component calibers; on the rare fail-open shape the lossless detail
	// block keeps the composition line (runtimeTraceProjInversionCompositionText).
	// VS-1 (§7.8): a periodic signal source's in-period sleep is cadence, not
	// impact — the row keeps the DATA (period + discounted attribution) while
	// the bar/ms keep the raw window projection. PTV4 T4: the "期内睡眠为正常
	// 节拍" semantics live in the legend's 周期性信号源 entry (already
	// verbatim there); the inline tag carries only the marker + data.
	if node.PeriodicSource {
		row.marks.mark(runtimeTraceProjMarkPeriodicSource)
		// PTV8-RCR-B (UXA 域A #31): the tag prints the 有效归因 word.
		row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
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
		// 行面口径披露句 (终判⑤ §29.96.2 mint; reworded by PERIODIC-DEDUP
		// §29.104 ①, 2026-07-15): a cross-window merged periodic row's value
		// channel wears the union/跨窗取最大 dedup caliber and its 有效归因 Σ
		// now shares the SAME same-segment proof (proven re-measurements count
		// once, distinct occurrences add) — the sentence states that unified
		// rule (循 union/CWD 口径句家族: typed flags fork, zh 主/EN 槽).
		if clause := runtimeTraceProjPeriodicCrossWindowSumClause(node, zh); clause != "" {
			text += clause
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
	}
	// VS-2 (§7.10): a folded running-dominant row states its mechanism
	// composition inline (Keep + ContinuationLane; single-source clause —
	// see answer_document_mutation_runtime_supplyfold.go). The affirmative
	// no-deficit branch and the honest "频点数据不全" branch render too:
	// exclusion is information.
	//
	// PTV8-RCR-A (§24 ②, 2026-07-08). EVOLUTION RECORD: on INVERSION cause
	// nodes the 机制构成 mechanism SENTENCE branches (Triple/WithDemand) are
	// RETIRED (行3+拆解子行 took the composition's seat; the supply-fold
	// deficit keeps a lossless home on the detail block's 供给折算 line). The
	// §21 RNB R1 `⧖ runnable …gated 分量,不重复计入排序` sub-row that used to
	// follow is RETIRED with it — the runnable component now IS a 拆解子行.
	// The non-sentence branches (Dominant/NoDeficit/UnknownBasis — the §15.A
	// SFD twin-join deliverable) and every non-inversion verdict render
	// unchanged, emitting the §24.1补 caliber legend marks exactly when their
	// words appear.
	if tag, ok := runtimeTraceProjSupplyFoldTag(node, foldWindowMS, zh); ok {
		verdict := runtimeTraceProjSupplyFoldVerdictFor(node, foldWindowMS)
		mechanismSentence := verdict == runtimeTraceProjSupplyFoldTriple ||
			verdict == runtimeTraceProjSupplyFoldWithDemand
		// CAP (§26 C3): the capability disclosure words carry their legend
		// entry wherever they render — every clause branch except the bare
		// "无法折算" no-fold form speaks them (supplyfold.go), and the FAIL-1
		// detail-line home below carries them too.
		capMark, capMarkOK := runtimeTraceProjCapabilityCaliberMarkTopo(node.SupplyFoldCapabilitySource, node.SupplyFoldTopologySource)
		// R5 (§29.88.12 单基准): ONE basis legend seat — the global-max fmax
		// entry covers both word forms (全域最大核最高频 / freq_only 全域最高
		// 频); the demoted-reference seat retired with its words.
		if structuredOK && mechanismSentence && runtimeTraceCausalProjectionInversionRow(node) {
			// 复核 FAIL-1 (§24.1补 图例破洞): the clause is suppressed here
			// (§24 ②) but the deficit still reaches the reader through the
			// detail block's 供给折算 line — its basis/下界 words carry
			// their legend entries wherever they render (the 下界 explanation
			// is the entry the user personally asked for).
			if runtimeTraceProjInversionSupplyFoldDetailLine(node, zh) != "" {
				row.marks.mark(runtimeTraceProjMarkCaliberGlobalMaxFmax)
				row.marks.mark(runtimeTraceProjMarkCaliberLowerBound)
				if capMarkOK {
					row.marks.mark(capMark)
				}
			}
		} else {
			switch verdict {
			case runtimeTraceProjSupplyFoldTriple, runtimeTraceProjSupplyFoldWithDemand, runtimeTraceProjSupplyFoldDominant:
				row.marks.mark(runtimeTraceProjMarkCaliberGlobalMaxFmax)
				row.marks.mark(runtimeTraceProjMarkCaliberLowerBound)
				if capMarkOK {
					row.marks.mark(capMark)
				}
			case runtimeTraceProjSupplyFoldNoDeficit:
				// CAP (§26 判词重判): the affirmative / near-basis forms carry
				// ONLY the capability disclosure (they never speak 按…折算/
				// 下界 as caliber words).
				if capMarkOK {
					row.marks.mark(capMark)
				}
			case runtimeTraceProjSupplyFoldUnknownBasis:
				// PTV8-RCR-C §24.9 G4 co-repair: the deficit-bearing unknown-
				// basis form speaks the basis…下界 words — its caliber legend
				// entries follow the words (the no-deficit form keeps none).
				if node.SupplyFoldDeficitMS > 0 {
					row.marks.mark(runtimeTraceProjMarkCaliberGlobalMaxFmax)
					row.marks.mark(runtimeTraceProjMarkCaliberLowerBound)
					if capMarkOK {
						row.marks.mark(capMark)
					}
				}
			}
			tag.Seg = 34
			tags = append(tags, tag)
		}
	}
	// RN-1 (§7.9, RN-B lane): a runnable row with a compiled same-window
	// occupier roster says WHO held the CPU inline (helper appended at file
	// end; Keep + ContinuationLane — the occupier names/values have no other
	// fence carrier).
	if tag, ok := runtimeTraceProjOccupierTag(node, zh); ok {
		tag.Seg = 12
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
		if tag, ok := runtimeTraceProjFullWindowCoverageTag(node, zh, row.CoverageFragmentSecondary, row.CoverageMergedTwinCount); ok {
			tag.Seg = 34
			tags = append(tags, tag)
		}
	}
	// §7.30.3 D1: the parsed holder site is auditable detail; the raw record
	// keeps it lossless. PTV8-RCR-B (UXA 域A #29, 2026-07-08). EVOLUTION
	// RECORD: the 40-rune HEAD cut kept a Java signature's return type and
	// dropped the load-bearing 类.方法(文件:行) — signature-aware compaction
	// (strip return type/args, keep method + file:line; tail-keep fallback).
	if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
		text := "持有点 " + runtimeTraceProjHolderSiteCompact(site, 40)
		if !zh {
			text = "held at " + runtimeTraceProjHolderSiteCompact(site, 40)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 11})
	}
	// PTV4 T9 (‹链上L#› 三路分流): the former ‹layer›priority chip is retired
	// from the tree — the depth VALUE stays as a compact chip (subordinate
	// line on width pressure), the 关注 semantics moved to the T6 ❶❷❸ badges,
	// and the detail blocks keep the full 因果位置·优先级 cell. Chain-lane
	// rows with a resolved depth only; flat renders never claim 链上 (CMP-7a).
	// PTV8-RCR-C (§24.9 G3 收编): a structured cause node carries the layer
	// inside 行2 (runtimeTraceProjCauseStructuredParts, same shared gate) —
	// this Seg-20 seat then stays silent; hop/non-cause chain rows keep it.
	// §24.12 C6: the depthless unattached shape speaks the 三面同词 word.
	if !structuredOK && runtimeTraceProjChainDepthChipEligible(row) {
		row.marks.mark(runtimeTraceProjMarkChainDepthChip)
		if runtimeTraceProjDepthlessUnattachedRow(row) {
			row.marks.mark(runtimeTraceProjMarkChainSeatUnattached)
		}
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjChainDepthChipWord(row, zh), Seg: 20})
	}
	if action, actionFamily := runtimeTraceCausalProjectionActionCellWithFamily(node, zh); action != "" &&
		row.Kind != runtimeTraceProjTreeRowBackground {
		// PTV5 C12 (#68): a semantic-span row whose shape tag already carries
		// the class token (shape-cell path + SemanticClass, precise) trims the
		// action cell to the bare word — the same class printed twice on
		// adjacent subordinate lines. Detail blocks keep the full cell.
		// PTV8-RCR-C (§24.12 C10, 2026-07-08). EVOLUTION RECORD: the gate was
		// the ✦ seat KIND — the same node's ◇ stanza seat double-printed the
		// class on one line (语义优化span·class_verification · 优化点·
		// class_verification). The typed node identity now decides for every
		// seat.
		if (row.Kind == runtimeTraceProjTreeRowSemantic ||
			runtimeTraceCausalProjectionSemanticSpanRow(node)) && shapeCellUsed &&
			strings.TrimSpace(node.SemanticClass) != "" {
			if zh {
				action = tracefence.ActionWordZH
			} else {
				action = tracefence.ActionWordENShort
			}
		}
		// PTV6-C #6 (#73) + b3 第三标本修 (2026-07-06): a state-restating
		// action word (typed family non-"") yields whenever the SAME family
		// is already spoken on the row — by the 裁定4 state tag (pure
		// StateKindLabel path) or by the cause word (typed token classes,
		// zh/en 双面同判); and an inversion row suppresses the category word
		// ENTIRELY — the cause full word 优先级反转候选 + 影响构成 carry the
		// complete semantics, while a state-driven word (执行/算力 on a
		// runnable-dominated composite) contradicted the gated split.
		// Action cells carrying other information (sleep drill guidance,
		// candidate words, 优化点) are untouched.
		suppress := false
		switch {
		case row.DrillTargetRendered && node.IsSleepState() && !node.Undrillable() &&
			strings.TrimSpace(node.DrilldownTarget) != "":
			// PTV8-RCR-B (UXA 域A #25, 2026-07-08). EVOLUTION RECORD: the
			// 睡眠症状→查上游 tag repeated on every sleep row while the tree
			// edge below it already pointed at the rendered upstream — it now
			// renders only when the upstream is NOT a row of this tree.
			suppress = true
		case runtimeTraceCausalProjectionInversionRow(node):
			suppress = true
		case actionFamily == "":
			suppress = false
		case !shapeCellUsed && stateTag != "":
			// The state tag derives from node.StateKind — the same source the
			// action family read (fallback only fires when StateKind is
			// empty, and then stateTag is empty too).
			suppress = true
		case runtimeTraceProjActionJointFamily(runtimeTraceCausalProjectionCauseDisplayToken(node)) == actionFamily:
			suppress = true
		}
		if !suppress {
			tags = append(tags, runtimeTraceProjTag{Text: action, Seg: 10})
		}
	}
	// F5 (§22 PTV7-SPN, 用户措辞裁定 2026-07-07, 措辞一字不改): a trace_gap row
	// states what it IS inline — 窗内无调度数据·链止 — in the slot the retired
	// candidate chip used to fake; the legend's 数据盲区 entry carries the full
	// semantics (missing_wakeup 图例措辞族). Exact typed token match.
	//
	// EVOLUTION RECORD (DISP-2 G2 措辞按 kind 分形, §27.2/§28.1 user ruling
	// 2026-07-09): the single wording over-claimed on the no_eligible_wait
	// criterion (intervals EXIST but all sit below the MinDurationMs floor —
	// 复核 P3-5). The inline disclosure now forks on the typed trace_gap_kind
	// enum: no_eligible_wait speaks 窗内无≥阈值等待区间·链止 with its own
	// legend entry; no_sched_data AND a missing kind (legacy replays, 有损缺失
	// fail-open) keep the 2026-07-07 ruling wording byte-identically.
	if runtimeTraceProjTraceGapNode(node) {
		text, mark := "窗内无调度数据·链止", runtimeTraceProjMarkTraceGapBlindSpot
		if !zh {
			text = "no in-window scheduler data · chain ends"
		}
		if strings.TrimSpace(node.TraceGapKind) == tracequery.TraceGapKindNoEligibleWait {
			text, mark = "窗内无≥阈值等待区间·链止", runtimeTraceProjMarkTraceGapBelowFloor
			if !zh {
				text = "no in-window wait ≥ floor · chain ends"
			}
		}
		row.marks.mark(mark)
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 10})
	}
	// P9 arm c (§29.42 案1, 2026-07-12): a pacing_idle row's type word
	// 帧间空闲(等待下一帧) rides the typelabels table; the mark carries the
	// teaching legend entry exactly when such a row renders (词条-图例双向).
	// Exact typed token match — never a substring heuristic. 复核 P2-1: the
	// generic periodic fork carries its own word + entry the same way.
	if runtimeTraceProjPacingIdleNode(node) {
		row.marks.mark(runtimeTraceProjMarkPacingIdle)
	}
	if runtimeTraceProjPeriodicIdleNode(node) {
		row.marks.mark(runtimeTraceProjMarkPeriodicIdle)
	}
	// ENG-2 (复核冷读 CP1-③, 2026-07-12): a row that ABSORBED a typed idle
	// view (the R1 same-fact fold of a pacing_idle/periodic_idle row into
	// its scheduler-state twin, or the ×N merge carrying such members)
	// states the annotation inline — 「其中 X.XXXms 帧间空闲(等待下一帧)」 —
	// so the idle reclassification survives onto the rendered seat instead
	// of dying in SecondaryObjects. Rows whose OWN token is the idle lane
	// already speak the cause word and skip the redundant tag.
	if text, mark, ok := runtimeTraceProjIdleCadenceTag(node, zh); ok {
		row.marks.mark(mark)
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
	}
	// stanzaCumEmitted feeds the ruling-A 折算 discriminator below: when the
	// cum and effective values BOTH publish on a stanza row with different
	// values, the second tag needs its own word (同词异值无判别 — 修正轮 Med).
	stanzaCumEmitted := false
	// PTV6-C 修正轮 Low (2026-07-06): cross-thread aggregates carry the
	// cpu·ms unit suffix on the main number — a plain-ms cum/effective tag
	// next to it stacks two calibers on one row (same idle gate as the C00
	// caliber word, which already excludes aggregates).
	if node.CumulativeImpactMS > 0 && impact > 0 && node.CumulativeImpactMS != impact && !crossThread {
		// PTV6-C ruling A (#73, 用户裁定 2026-07-06): the chain-attribution
		// word 链上累计 belongs to the chain universe; a ◇/▒ stanza row shows
		// the SAME data under the 累计(跨线程) family word (single wording
		// home, kind-split — the row is off the wakeup path, so an on-chain
		// attribution word would contradict its own stanza legend).
		switch {
		case runtimeTraceProjFamilyRow(node) && runtimeTraceProjStanzaRowKind(row.Kind):
			// RCM-2 D1 (F6 negative pin): a family total on a ◇/▒ seat must
			// never wear the 累计(跨线程) word — its 行3 family caliber (and
			// the detail stanza) carry the accounting instead.
		case runtimeTraceProjStanzaRowKind(row.Kind):
			row.marks.mark(runtimeTraceProjMarkStanzaCrossThreadCum)
			tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjCrossThreadCumTagText(node.CumulativeImpactMS, zh), Seg: 31})
			stanzaCumEmitted = true
		case runtimeTraceProjChainUniverseRowKind(row.Kind) &&
			node.EffectiveImpactMS == node.CumulativeImpactMS &&
			!node.PeriodicSource && impactSource != runtimeTraceProjImpactSourceEffective:
			// PTV6-D (c) (#75 标本归因 #10, stanza-lane precedent 同折): an
			// EQUAL-value 链上累计/有效归因 pair on a chain-universe row is one
			// measurement — the Q1 有效归因 tag below (user ruling 2026-07-05,
			// 常显 + badge sort key) is the surviving carrier, the redundant
			// 链上累计 copy folds. Differing values keep BOTH words (pinned by
			// TestPTV6CRulingAChainUniverseKeepsAttributionWords). The guard
			// mirrors the Q1 emission gate exactly, so the fold can never fire
			// when the effective carrier itself stays silent (eff>0 is implied
			// by the equality with the emitted cum>0; inherited needs eff>cum
			// (§24.12 C5 precise gate) and can never be value-equal).
		default:
			text := fmt.Sprintf("链上累计%.3fms", node.CumulativeImpactMS)
			if !zh {
				text = fmt.Sprintf("chain cum %.3fms", node.CumulativeImpactMS)
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 31})
		}
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels (PTV4
	// T4 ×N 三式 — data inline, semantics in the legend's 口径组).
	// UXR-1 §29.36④ (140554 witness): the ×N同值 chip RELOCATED onto the 词位
	// (runtimeTraceProjRowName appends it after the name, same form as the
	// detail table) — only the mark records here; the standalone Seg-30 tag
	// that produced lone 「· ×2同值」 orphan lines is retired.
	if node.DuplicatePublications > 1 {
		row.marks.mark(runtimeTraceProjMarkMergedDedup)
	}
	if node.MergedCount > 1 && !(structuredOK && structured.ConsumedMergedTag) {
		// PTV8-RCR-A §24.2: on the event form the ×N count already rode 行1
		// and the (a~b,共N次) range rides 行3 — the legacy inline tag stays off.
		var text string
		if node.MicroAnchorFold {
			// RNB-5B 件⑦: the micro anchored-seat fold speaks its own account-
			// sum caliber — the generic 取最大 form would mislabel the Σ value.
			row.marks.mark(runtimeTraceProjMarkMicroAnchorFold)
			text = runtimeTraceProjMicroAnchorFoldTagText(node, zh)
		} else if runtimeTraceProjSubjectlessFoldRow(node) && runtimeTraceProjAllZeroFoldRow(node) {
			// EVOLUTION RECORD (DISP-2 G19, §27.5, 2026-07-09): the all-zero
			// fold shape used to wear ×N(0.000–0.000ms)取最大 — a member-MAX
			// claim over nothing but zeros (huadong_79 "×9(0.000–0.000ms)取最
			// 大" noise witness). The shape now speaks the honest one-line note
			// (typed numeric shape + fold identity; the row name keeps the
			// 其余N项(折叠) count and the members stay reachable via the
			// detail blocks and the evidence index — zero information loss).
			row.marks.mark(runtimeTraceProjMarkAllZeroFoldNote)
			text = runtimeTraceProjAllZeroFoldNoteText(node, zh)
		} else if runtimeTraceProjSubjectlessFoldRow(node) {
			// V3: the R3 cross-thread fold publishes the member MAX (取最大
			// legend entry; wall clock never sums across threads).
			row.marks.mark(runtimeTraceProjMarkMergedMax)
			if _, _, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
				// G12-ENG (§29.1): mixed fold → the 无时长值 legend entry
				// teaches the honest range wording exactly when it renders.
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
			}
			text = runtimeTraceProjMergedMaxTagText(node, zh)
		} else if node.MergedIntervalUnion {
			// §11-N2: cross-query-window union caliber — its own form token,
			// never the sum form (whose legend entry claims 数值为总和).
			row.marks.mark(runtimeTraceProjMarkMergedUnion)
			if runtimeTraceProjMergedValuelessWordRenders(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers) // G12-ENG 复核 P2-1 连带
			}
			text = runtimeTraceProjMergedUnionTagText(node, zh)
		} else if node.MergedCrossWindowMax {
			// §21 CWD: overlapping-query-window MAX caliber (×N 第五式).
			// G12-ENG 复核 P1-1: the mixed CWD tag renders the 无时长值 word —
			// its legend entry must ride along (词条-图例双向契约).
			row.marks.mark(runtimeTraceProjMarkMergedWindowMax)
			if runtimeTraceProjMergedValuelessWordRenders(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
			}
			text = runtimeTraceProjMergedCrossWindowMaxTagText(node, zh)
		} else {
			// G12-ENG 复核 P2-2: mixed/all-zero R2 sum rows render the
			// 无时长值 family word — same legend contract. The all-valueless
			// row wears NO ×N(a~b) sum notation (nothing summed), so the sum
			// legend entry must not render for it (G19 fork discipline).
			if runtimeTraceProjMergedAllValueless(node) {
				row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
			} else {
				row.marks.mark(runtimeTraceProjMarkMergedSum)
				if runtimeTraceProjMergedValuelessWordRenders(node) {
					row.marks.mark(runtimeTraceProjMarkValuelessFoldMembers)
				}
			}
			text = runtimeTraceProjMergedSumTagText(node, zh)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 30})
	}
	if len(node.SecondaryObjects) > 0 {
		// PTV6-C #6: 影响点 tokens ride the single display helper — PTV7: bare
		// state tokens collapse onto the canonical display word (runnable /
		// sleep（s_sleep） when the alias differs from the raw token); mapped
		// type tokens keep the D4 label（token） combined form; EN keeps raw.
		// PTV8-RCR-B (UXA 域A #23, 2026-07-08). EVOLUTION RECORD: a BARE
		// scheduler-state token no longer enters the 影响点 slot — the slot
		// promises "who was impacted" and the row's state is already carried
		// by the icon + state word (the fourth restatement read as "影响点是
		// 睡觉"); the detail block keeps the full roster, so nothing is lost.
		// Exact state-word-table membership only (precise signal).
		display := make([]string, 0, len(node.SecondaryObjects))
		for _, token := range node.SecondaryObjects {
			if runtimeTraceCausalProjectionImpactPointBareState(token) {
				continue
			}
			display = append(display, runtimeTraceCausalProjectionImpactPointDisplay(token, zh))
		}
		if len(display) > 0 {
			joined := strings.Join(display, "/")
			text := "影响点 " + joined
			if !zh {
				text = "impact point " + joined
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 11})
		}
	}
	if crossThread {
		// 修正轮 Low: aggregates keep their own cpu·ms caliber — the whole
		// inherited/effective plain-ms tag family idles (same gate as above).
	} else if runtimeTraceProjEffectiveInherited(node) && !runtimeTraceProjStanzaRowKind(row.Kind) {
		// PTV4 T4: marker + data inline; the "非本行实测" semantics live in
		// the legend's 承自归因 entry. 修正轮 Low: 承自归因 is chain-universe
		// vocabulary — a ◇/▒ row falls through to the stanza family word
		// below instead (the inherited lane sat ungated between two gated
		// lanes).
		row.marks.mark(runtimeTraceProjMarkInheritedAttribution)
		text := fmt.Sprintf("承自归因%.3fms", node.EffectiveImpactMS)
		if !zh {
			text = fmt.Sprintf("inherited attribution %.3fms", node.EffectiveImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
	} else if structuredOK && structured.ConsumedEffective {
		// PTV8-RCR-A §24.1/§24.2: the effective value already rides 行2's
		// degenerate tail or 行3's 「=」breakdown — the plain tag stays off.
	} else if node.EffectiveImpactMS > 0 && !node.PeriodicSource &&
		impactSource != runtimeTraceProjImpactSourceEffective {
		// PTV5 Q1 (#68 用户裁定 2026-07-05): 有效归因常显 — every data row with a
		// positive effective attribution carries the value inline (demotes
		// whole to a subordinate line on width pressure). Gate = value>0,
		// precise. No double print: periodic rows embed the value in the VS-1
		// tag, the inherited form keeps its 承自归因 label above, and the C00
		// fallback word already qualifies a main number that IS the effective
		// value. The ❶❷❸ sort key is thereby visible at its row (C07).
		// PTV6-C ruling A (#73, 用户裁定 2026-07-06): the 有效归因 attribution
		// word is RESTRICTED to the chain universe (chain/cause/depthless);
		// a ◇/▒ stanza row renders the same data under the 累计(跨线程)
		// family word — deduped against an identical cross-thread tag above
		// (equal-value shape). Other kinds (semantic spans) carry neither.
		switch {
		case runtimeTraceProjChainUniverseRowKind(row.Kind):
			// PTV8-RCR-B (UXA 域A #31): the word gets its legend seat.
			row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
			text := fmt.Sprintf("有效归因 %.3fms", node.EffectiveImpactMS)
			if !zh {
				text = fmt.Sprintf("attribution %.3fms", node.EffectiveImpactMS)
			}
			// ELIM-GAP 件D (§29.104.15, 2026-07-16): a C5-guarded typed-producer
			// row whose eff sits above its own window projection says HOW the
			// value was taken (发生段账目) — the 关键指标 glossary's
			// 与窗口投影不同时行内口径词 promise, previously unfulfilled on
			// exactly these rows (E16/E18/E19 witness).
			if word, ok := runtimeTraceProjOccurrenceSegmentAccountWord(node, zh); ok {
				text += word
				row.marks.mark(runtimeTraceProjMarkOccurrenceSegmentAccount)
			} else if word, ok := runtimeTraceProjBareEffectiveCaliberBeltWord(node, row.marks, zh); ok {
				// GATED-CAL 件1④ (§29.104.16.1 M3, 2026-07-16): this lane runs
				// exactly when 行2/行3 did NOT consume the value (the enclosing
				// structured.ConsumedEffective check IS the C5 premise, read for
				// real) — a typed-producer value never ships the bare tag naked.
				text += word
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
		case runtimeTraceProjStanzaRowKind(row.Kind):
			// 修正轮 Med: beside an already-emitted cum tag, an EQUAL effective
			// value folds away (one measurement, one tag); a DIFFERING value
			// wears the 折算 discriminator word (the effective lane's neutral
			// stanza word, zh/en 对等) — two same-word tags with two values
			// gave the reader no way to tell them apart.
			switch {
			case runtimeTraceProjFamilyRow(node):
				// RCM-2 D1 (F6): family rows never wear the cross-thread word
				// on ANY lane — the 行1 caliber stem + detail stanza carry the
				// same-thread accounting (the 行3 arm consumed the balanced
				// shapes; this is the fail-open remainder).
			case stanzaCumEmitted && node.EffectiveImpactMS == node.CumulativeImpactMS:
				// folded: the cum tag already carries this measurement.
			case stanzaCumEmitted:
				// PTV8-RCR-B (UXA 域A #19): the discriminator word teaches via
				// its own on-demand entry (the cum entry no longer glosses it).
				row.marks.mark(runtimeTraceProjMarkStanzaDiscount)
				text := fmt.Sprintf("折算 %.3fms", node.EffectiveImpactMS)
				if !zh {
					text = fmt.Sprintf("discounted %.3fms", node.EffectiveImpactMS)
				}
				tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 32})
			case types.TraceCausalProjectionKnownSubject(node.Subject):
				// DISP-3 (§29.8 P2-⑧ 后半 "区段行'累计(跨线程)'误挂单线程行",
				// 2026-07-09): the fallback stanza word claims 多线程时间累计,
				// but a SINGLE-thread stanza row's effective is neither
				// cross-thread nor a cumulative — the value is the row's rank
				// participation caliber (huadong_792 E22: 累计(跨线程)9.169ms
				// on oney's own ×3 row, where 9.169 is the per-instance max the
				// rank lane counts; cmp_792 proj2 E23 同族). The fork engages
				// ONLY on a RESOLVED thread subject — the unknown-thread
				// sentinel row (PTV6-C ruling A pinned irq_burst form) is not
				// provably single-thread and keeps the family word
				// byte-identically. Three precise sub-forks, all on typed
				// numeric identities:
				//   - effective == the row's published main number: one
				//     measurement, no second tag (与窗口投影相等时即窗口投影列
				//     数值 — the (a)-table legend clause already teaches it);
				//   - a ×N merged row whose effective IS the per-instance MAX:
				//     the §24.2 行3 equation verbatim (有效归因 V = 单次最大
				//     (a~b,共N次)) — the caliber word and its legend entry are
				//     the existing closed-set pair, no new vocabulary;
				//   - residual (rank-seated, effective ≠ both): the bare
				//     有效归因 word — true by the word's own definition for a
				//     seated contender (§24.7 呈现逻辑统一令); rank-less
				//     residuals emit nothing here (the (a) table carries the
				//     value losslessly).
				// The multi-thread R3 fold keeps 累计(跨线程) byte-identically
				// on the arm below (the word is exactly right there).
				switch {
				case runtimeTraceProjRound3Equal(node.EffectiveImpactMS, impact):
					// value already on the main number — no tag.
				case node.MergedCount > 1 && node.MergedMaxMS > 0 &&
					runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.MergedMaxMS):
					row.marks.mark(runtimeTraceProjMarkCaliberSingleMax)
					row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
					text := fmt.Sprintf("有效归因 %.3fms = 单次最大(%.3f~%.3fms,共%d次)",
						node.EffectiveImpactMS, node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
					if !zh {
						text = fmt.Sprintf("attribution %.3fms = single max (%.3f~%.3fms, of %d)",
							node.EffectiveImpactMS, node.MergedMinMS, node.MergedMaxMS, node.MergedCount)
					}
					tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
				default:
					if rank, _ := runtimeTraceProjCauseRankConfidence(row); rank > 0 {
						row.marks.mark(runtimeTraceProjMarkEffectiveAttributionTag)
						text := fmt.Sprintf("有效归因 %.3fms", node.EffectiveImpactMS)
						if !zh {
							text = fmt.Sprintf("attribution %.3fms", node.EffectiveImpactMS)
						}
						tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
					}
				}
			default:
				row.marks.mark(runtimeTraceProjMarkStanzaCrossThreadCum)
				tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjCrossThreadCumTagText(node.EffectiveImpactMS, zh), Seg: 31})
			}
		}
	}
	// RN-2b (§7.9): the ⚠ marker's semantics DEPEND on a resolved projection
	// window. No window → no ⚠ (tree tag, detail mirror and — via the NEW-7
	// typed mark — the legend entry all fall silent together). PTV4 T4: the
	// inline "跨窗" explanation moved to the legend; the tag keeps marker +
	// actual value (⚠实际Xms). MainRow: a T1 Keep 记号.
	//
	// §21 LEAD-SEM 前置 L1 (cmp_01 A④, 2026-07-07): a typed cross-window row
	// whose actual total was never captured (ActualImpactMS<=0 — the semantic
	// span-work lane publishes only effective_impact_ms, never actual_*) MUST
	// NOT print the fake "实际0.000ms" scalar — it demotes to the value-less
	// ⚠跨窗 marker (precise boolean fork, mirroring the detail table's
	// existing ActualImpactMS>0 guard). The detail mirror already renders "—".
	// CR-2 组③ P7 (2026-07-12): the word face forks on the typed interval
	// verdict — ⚠ only when the actual interval provably leaves the analysis
	// window; an in-window overshoot speaks the episode word; an interval-less
	// actual states the dual-basis fact without a scope claim. ×N merged rows
	// additionally disclose that the value is the merge seed's SINGLE member
	// actual (F-5: 「实际6.936 < 行值17.442」 read as a paradox without it).
	if windowMode {
		// Model-built rows carry the analysis-window verdict stamp; a
		// zero-value stamp (hand-built rows / direct callers) recomputes with
		// unknown endpoints — the typed WithinRequestedWindow arm still
		// resolves, and value-only overshoots degrade to the scope-less
		// disclosure (never a fabricated ⚠).
		scope := row.ActualScope
		if scope == runtimeTraceProjActualScopeNone {
			scope = runtimeTraceProjActualWindowScope(node, 0, 0)
		}
		switch scope {
		case runtimeTraceProjActualScopeAnalysisWindow:
			if node.ActualImpactMS > 0 {
				row.marks.mark(runtimeTraceProjMarkCrossWindow)
				text := fmt.Sprintf("⚠实际%.3fms%s", node.ActualImpactMS, runtimeTraceProjActualMemberQualifier(node, zh))
				if !zh {
					text = fmt.Sprintf("⚠actual %.3fms%s", node.ActualImpactMS, runtimeTraceProjActualMemberQualifier(node, zh))
				}
				tags = append(tags, runtimeTraceProjTag{Text: text, MainRow: true})
			} else {
				row.marks.mark(runtimeTraceProjMarkCrossWindowNoActual)
				text := "⚠跨窗"
				if !zh {
					text = "⚠cross-window"
				}
				tags = append(tags, runtimeTraceProjTag{Text: text, MainRow: true})
			}
		case runtimeTraceProjActualScopeEpisode:
			// Unlike ⚠ (a warning Keep mark), the in-window/scope-less actual
			// disclosures are caliber information — demote-eligible tags, so
			// the longer word faces never crowd the MainRow essentials.
			// 修复轮 P4-b: scope word + member qualifier share ONE paren
			// (「(超出发生段,窗内·单次成员)」, never consecutive parens).
			row.marks.mark(runtimeTraceProjMarkActualBeyondEpisode)
			text := fmt.Sprintf("实际%.3fms(%s)", node.ActualImpactMS, runtimeTraceProjActualScopeParen("超出发生段,窗内", node, true))
			if !zh {
				text = fmt.Sprintf("actual %.3fms (%s)", node.ActualImpactMS, runtimeTraceProjActualScopeParen("beyond own episode, inside window", node, false))
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
		case runtimeTraceProjActualScopeNoInterval:
			row.marks.mark(runtimeTraceProjMarkActualNoInterval)
			text := fmt.Sprintf("实际%.3fms(%s)", node.ActualImpactMS, runtimeTraceProjActualScopeParen("区间未发布", node, true))
			if !zh {
				text = fmt.Sprintf("actual %.3fms (%s)", node.ActualImpactMS, runtimeTraceProjActualScopeParen("interval unpublished", node, false))
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 33})
		}
	}
	// NEW-3: the folded same-segment IO calibers' values and evidence tags live
	// ONLY on this note (plus the evidence index) — load-bearing, never elided;
	// demotes intact to a subordinate line on width pressure.
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh), Seg: 31})
	}
	// CR-2 组② P5 同段收敛 (2026-07-12): the mirror tags (shared helper with
	// the self-row face — one wording, one mark).
	for _, text := range runtimeTraceProjSameSegMirrorTagTexts(row, zh) {
		tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 31})
	}
	// PTV8-RCR-A (§24 ③裁定/§24.2, 2026-07-08). EVOLUTION RECORD: the RNB R2
	// 同段rank行并入 note is RETIRED — the folded rank row's rank/confidence
	// ride 行2 (runtimeTraceProjCauseRankConfidence) and its E# merges into
	// 行1's [E#+E#] bracket (runtimeTraceProjCauseEvidenceRef); the lossless
	// detail block keeps the 根因排序 line. The RNB join/guard engine itself
	// is untouched.
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if parent != "" {
			text := "span 位于 " + parent + " 内"
			if !zh {
				text = "span inside " + parent
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, Seg: 12})
		}
	}
	if node.Undrillable() {
		// PTV4 T4/T5: bare ⊘链止 — the sched_wakeup explanation lives in the
		// legend's ⊘ entry. MainRow: a T1 Keep 记号.
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		text := tracefence.GlyphUndrillable + "链止"
		if !zh {
			text = tracefence.GlyphUndrillable + "chain ends"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, MainRow: true})
	}
	if ref := runtimeTraceProjCauseEvidenceRef(row); ref != "" {
		// The E# locator is a T1 Keep 记号 — always on the main line, whole.
		// PTV8-RCR-A §24.2: a folded rank twin's E# merges into the bracket
		// ([E7(+1)+E8]) — the rank row stays reachable via the evidence index.
		tags = append(tags, runtimeTraceProjTag{Text: ref, MainRow: true})
	}
	// PTV8-RCR-B (UXA 域D #36, 2026-07-08): fixed five-segment note order for
	// the ORDINARY tags — same-shaped nodes read same-shaped (the specimen's
	// E4/E5 tag orders drifted). Stable sort by Seg among ordinary tags only;
	// MainRow keep-marks and the RCR OwnLine grammar lines never move.
	ordinary := make([]int, 0, len(tags))
	for i, tag := range tags {
		if !tag.MainRow && !tag.OwnLine {
			ordinary = append(ordinary, i)
		}
	}
	segSorted := make([]runtimeTraceProjTag, 0, len(ordinary))
	for _, i := range ordinary {
		segSorted = append(segSorted, tags[i])
	}
	sort.SliceStable(segSorted, func(a, b int) bool { return segSorted[a].Seg < segSorted[b].Seg })
	for k, i := range ordinary {
		tags[i] = segSorted[k]
	}
	return b.String(), tags
}

// runtimeTraceProjEffectiveInherited flags the contradictory-number shape a
// real customer render exposed: an EffectiveImpactMS above the row's own
// cumulative means the attribution was inherited from the enclosing wait
// interval, and MUST be annotated instead of shown bare.
//
// PTV8-RCR-C (§24.12 C5, 2026-07-08). EVOLUTION RECORD: the gate was the
// noisy eff > 10×cum RATIO — the whole 1.1×–1.8× hop-echo band (cmp_78_01:
// 8+6 chain rows whose 有效归因 exceeded their own projection, four of them
// even exceeding the row's physical 实际状态) shipped the bare 有效归因 word
// with no caliber. PRECISE gate now: any effective above the row's own
// cumulative is inherited (承自归因 + 承自注 window base). Print-equal values
// stay non-inherited — one measurement, the PTV6-D (c) equal-value fold
// unchanged.
func runtimeTraceProjEffectiveInherited(node types.TraceCausalProjectionNode) bool {
	// 复核 Low (2026-07-06, periodic∧inherited 互斥核查): a periodic source's
	// EffectiveImpactMS is COMPUTED (runnable + lateness, VS-1), never
	// inherited — a heuristic could misfire on a tiny cumulative and stack a
	// second 承自归因 carrier next to the VS-1 tag. Typed guard.
	if node.PeriodicSource {
		return false
	}
	// §24.12 C5 typed-producer guards: an effective whose origin is TYPED is
	// never "inherited from the enclosing wait interval" even when it exceeds
	// the row's window-clipped cumulative by a hair — the inversion gated
	// composite (runnable full + discounted running) and the §20.2 running
	// deficit both carry their own producers and their own caliber words.
	if runtimeTraceCausalProjectionInversionRow(node) &&
		(node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0) {
		return false
	}
	if runtimeTraceProjCauseRunningDeficitArm(node) {
		return false
	}
	// RCM-2 (§24.22 M2, 2026-07-08): a family row's effective is a TYPED fold
	// product (single formula over the member accounting) — never "inherited
	// from the enclosing wait interval"; the family caliber word is its label.
	if runtimeTraceProjFamilyRow(node) {
		return false
	}
	return node.EffectiveImpactMS > 0 && node.CumulativeImpactMS > 0 &&
		node.EffectiveImpactMS > node.CumulativeImpactMS &&
		!runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.CumulativeImpactMS)
}

// runtimeTraceProjOccurrenceSegmentAccountWord (ELIM-GAP 件D, §29.104.15,
// 2026-07-16) renders the short caliber word for the two §24.12 C5
// typed-producer guard arms above: an inversion gated composite
// (GatedRunnableMS/GatedRunningDeficitMS producer) or a §20.2 running-deficit
// row whose published effective sits ABOVE the row's own window-clipped
// projection — the value was computed over the row's occurrence-segment
// accounting, not the window projection. The 关键指标 glossary promises
// 「有效归因…与窗口投影不同时,行内口径词说明取值方式」, and the C5 guards
// deliberately keep these rows OFF the 承自归因 lane, so they shipped the
// bare word (cust_total_del witness E16/E18/E19: 有效归因 7.510/3.342/2.350
// beside 窗口投影 7.486/3.336/2.345). Exactly the eff>cum band the inherited
// gate would otherwise catch; print-equal pairs and eff≤cum rows stay
// wordless byte-identically (negative arm), and the periodic guard sits
// above both arms exactly as in runtimeTraceProjEffectiveInherited.
func runtimeTraceProjOccurrenceSegmentAccountWord(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	if node.PeriodicSource {
		return "", false
	}
	guarded := (runtimeTraceCausalProjectionInversionRow(node) &&
		(node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0)) ||
		runtimeTraceProjCauseRunningDeficitArm(node)
	if !guarded {
		return "", false
	}
	if !(node.EffectiveImpactMS > 0 && node.CumulativeImpactMS > 0 &&
		node.EffectiveImpactMS > node.CumulativeImpactMS &&
		!runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.CumulativeImpactMS)) {
		return "", false
	}
	if zh {
		return "(发生段账目)", true
	}
	return " (occurrence-segment account)", true
}

// runtimeTraceProjBareEffectiveCaliberBeltWord (GATED-CAL 件1④, §29.104.16.1
// M3, 2026-07-16) is the C5-guard floor word for the bare 有效归因X tag. The
// two §24.12 C5 typed-producer guard arms keep their rows OFF the 承自归因
// lane on the PREMISE that the producer's own 行3 caliber words reach the
// reader — a premise the guards never verified: on the unbalanced fail-open
// (and every non-cause-row shape) the structured 行3 does NOT consume the
// value, and the Q1 lane shipped the tag naked (the callers check the ACTUAL
// ConsumedEffective state before asking here — 前提改读实际). The floor:
//
//   - gated composite (both typed components > 0) → 构成,见明细 (one word
//     source with the 行2 tail / ◎ note / projection cell);
//   - §20.2 running-deficit identity → the established 折算,按… short word
//     (same composer as 行3, its legend marks ride along).
//
// Rows outside both arms keep the bare tag byte-identically (absence never
// guesses a caliber), and the (发生段账目) word wins wherever the bare-tag
// lane still runs (the callers try it first). 修补轮 件D勘正 (2026-07-17):
// rows whose 行3 equation now renders (the 件1② relaxation — donghu E18
// witness) left this lane ALTOGETHER, upgrading their former (发生段账目)
// short word to the full derivation: an in-declaration upgrade, not a
// byte-preservation claim; the ELIM-GAP 件D word remains pinned on the lanes
// that still carry it.
func runtimeTraceProjBareEffectiveCaliberBeltWord(node types.TraceCausalProjectionNode, marks *runtimeTraceProjMarkSet, zh bool) (string, bool) {
	if node.PeriodicSource {
		return "", false
	}
	if runtimeTraceProjGatedCompositeSeat(node) {
		marks.mark(runtimeTraceProjMarkGatedCompositeCaliber)
		if zh {
			return "(" + runtimeTraceProjGatedCompositeShortWord(true) + ")", true
		}
		return " (" + runtimeTraceProjGatedCompositeShortWord(false) + ")", true
	}
	if runtimeTraceProjCauseRunningDeficitArm(node) {
		word, wordMarks := runtimeTraceProjSupplyDiscountShortWord(node, zh)
		for _, m := range wordMarks {
			marks.mark(m)
		}
		if zh {
			return "(" + word + ")", true
		}
		return " (" + word + ")", true
	}
	return "", false
}

// runtimeTraceProjInheritedWindowBaseSuffix renders the §21-CWD window-base
// clause of the 承自注 (cmp_01 revisit 2026-07-07, P2 E29 随 B 同款): the
// inherited effective magnitude was measured over the donor wait interval's
// query window, not this row's rendered window. Typed sources only — the
// row's own QueryWindow identity first (the effective note rode the same
// record), else the merged member window roster (KNOWN sources, never claimed
// exhaustive), else "" (absence never guesses a window; legacy note
// byte-identical).
func runtimeTraceProjInheritedWindowBaseSuffix(node types.TraceCausalProjectionNode, zh bool) string {
	if node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs {
		if zh {
			return fmt.Sprintf(";窗基=查询窗 %.3f~%.3fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
		}
		return fmt.Sprintf("; window base = query window %.3f~%.3fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
	}
	if len(node.MergedQueryWindows) > 0 {
		windows := make([]string, 0, len(node.MergedQueryWindows))
		for _, w := range node.MergedQueryWindows {
			windows = append(windows, fmt.Sprintf("%.3f~%.3fs", w.StartTs, w.EndTs))
		}
		sep := "、"
		if !zh {
			sep = ", "
		}
		if zh {
			return fmt.Sprintf(";窗基=成员查询窗 %s(非本行渲染窗)", strings.Join(windows, sep))
		}
		return fmt.Sprintf("; window base = member query windows %s (not this row's rendered window)", strings.Join(windows, sep))
	}
	return ""
}

// runtimeTraceProjSemanticSourceWindowShareBaseMS (DCS E5, ledger §23/§23.1
// H2, cmp_01 E2 witness "83.893ms · 83% 对锚窗", 2026-07-08) returns the
// window-share denominator for a SEMANTIC row that carries its own typed
// QueryWindow identity DIFFERING from the render denominator: the numerator
// was measured over that source query window, so dividing by any other window
// fabricates a share. ok=false (no typed source window — absence never
// guesses — or the source matches the render window within the ±1ms F-2
// tolerance) keeps the legacy anchor-based share byte-identically. Standalone
// lane on purpose (PTV8-RCR node-shape rework friction).
func runtimeTraceProjSemanticSourceWindowShareBaseMS(row runtimeTraceProjTreeRow, denomMS float64) (float64, bool) {
	// PTV8-RCR-C (§24.12 C10 ◇席窗基披露, 2026-07-08). EVOLUTION RECORD: the
	// gate was the ✦ seat KIND only — the SAME semantic node's ◇/▒ stanza seat
	// silently re-based its % on the anchor window (cmp_78_01 E41: tree 1%
	// against its 81ms source window vs ◇ face 0% against 1800ms, no 来自查询窗
	// note). The typed NODE identity now decides, so every seat of a semantic
	// span re-bases (and discloses) identically.
	if row.Kind != runtimeTraceProjTreeRowSemantic &&
		!runtimeTraceCausalProjectionSemanticSpanRow(row.Node) {
		return 0, false
	}
	return runtimeTraceProjSemanticSourceWindowRebaseMS(row.Node, denomMS)
}

// runtimeTraceProjSemanticSourceWindowRebaseMS is THE shared E5 source-window
// rebase judgment (PTV8-RCR-A A6, §23.2 残余⑥ 合一): a node carrying its own
// typed source query window that differs from the render denominator beyond
// the ±1ms F-2 tolerance re-bases on that source window. The tree % lane
// (ShareBaseMS above) and the share wording lane
// (runtimeTraceProjSemanticSpanShareText) both read THIS one pair-judgment —
// the former per-caller reimplementations could drift on the boundary.
func runtimeTraceProjSemanticSourceWindowRebaseMS(node types.TraceCausalProjectionNode, denomMS float64) (float64, bool) {
	if node.QueryWindowStartTs <= 0 || node.QueryWindowEndTs <= node.QueryWindowStartTs {
		return 0, false
	}
	sourceMS := (node.QueryWindowEndTs - node.QueryWindowStartTs) * 1000
	if sourceMS <= 0 || math.Abs(sourceMS-denomMS) <= 1.0 {
		return 0, false
	}
	return sourceMS, true
}

// runtimeTraceProjSemanticSourceWindowTag renders the E5 inline disclosure —
// the typed source query window the re-based % divides by.
func runtimeTraceProjSemanticSourceWindowTag(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("来自查询窗 %.3f~%.3fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
	}
	return fmt.Sprintf("from query window %.3f~%.3fs", node.QueryWindowStartTs, node.QueryWindowEndTs)
}

// runtimeTraceProjCrossWindow marks a node whose underlying state extends
// beyond its in-window projection (deterministic comparison; also honors the
// typed WithinRequestedWindow=false drill marker).
//
// EVOLUTION RECORD (DISP-3, §29.8 P3 "E7 ⚠消失回归",
// real_trace_campaign_20260705.md, 2026-07-09; witness pair huadong_79 E8
// "1.433ms ⚠" vs huadong_792 E7 same四元组 no-⚠). The former baseline was the
// LARGER of per-layer projection and chain total — that over-generalized the
// dual-scope carve-out ("an actual equal to the chain total does not cross
// anything", v3 b8762441) into masking every actual sitting in the
// (projection, chain-total) band, although actual > own projection IS the ⚠
// definition (实际状态跨出分析窗). Two precise repairs:
//   - the dual-scope carve-out is now an EQUALITY carve-out (Round3Equal on
//     actual vs chain total — the duplicated-measurement shape it was built
//     for), not a ≤ band;
//   - a ×N merged row compares per-instance: its surviving actual belongs to
//     ONE member (the merge seed), so the SUM baseline was caliber-mismatched
//     (cmp_792 E7: actual 5.957 vs ×4 SUM 11.804 masked the member that
//     crossed its window). MergedMaxMS ≥ every member display, so the
//     comparison stays conservative — it can under-flag a small crossed
//     member, never over-flag. 复核 P2-1 (2026-07-10): the merged branch
//     additionally applies the MEMBER-level dual-scope carve-out against the
//     actual donor's own pre-merge chain total (the row-level pair is SUM-
//     overwritten and can never match), and suppresses ⚠ outright when the
//     donor field is absent (宁漏勿假).
//
// runtimeTraceProjActualScope is the CR-2 组③ P7 typed scope verdict behind
// the actual-value word faces (ledger §29.42 P7, witness 冷读案19 「⚠ 词面 11
// 行全假」 + CAL-1 冷读 F-5, 2026-07-12): the ⚠ glyph's legend promise is
// 「实际状态跨出分析窗」 — a claim about the ANALYSIS window that the former
// value-only comparison could not prove (donghu E5: actual 16.433 > projection
// 15.565 because the segment crossed its own OCCURRENCE sub-window while
// sitting fully inside the analysis window; the µs endpoints were on the wire
// all along). The word face now forks on the typed interval:
//   - AnalysisWindow → ⚠实际Xms (the interval provably leaves the window, or
//     the engine's own WithinRequestedWindow=false drill marker says so);
//   - Episode → 实际Xms(超出发生段,窗内) — the actual exceeds this row's own
//     occurrence projection but stays inside the analysis window;
//   - NoInterval → 实际Xms(区间未发布) — the dual-basis fact without any
//     scope claim (宁漏勿假: no interval, no window verdict);
//   - None → no tag.
type runtimeTraceProjActualScope int

const (
	runtimeTraceProjActualScopeNone runtimeTraceProjActualScope = iota
	runtimeTraceProjActualScopeAnalysisWindow
	runtimeTraceProjActualScopeEpisode
	runtimeTraceProjActualScopeNoInterval
)

// runtimeTraceProjActualContainmentToleranceS is the ⚠ containment slack
// (修复轮 R-P3-1, 2026-07-12): numerically the shared F-2 endpoint tolerance
// TODAY, minted under its OWN name because the SEMANTICS differ — this is
// interval CONTAINMENT slack (how far an actual endpoint may poke past the
// analysis window before 跨出 is claimed), not same-window equality. A future
// F-2 re-adjudication must re-decide this value separately.
const runtimeTraceProjActualContainmentToleranceS = types.TraceCausalProjectionSameWindowToleranceS

// runtimeTraceProjActualWindowScope computes the typed scope verdict. winStart/
// winEnd are the ANALYSIS-window endpoints (model.WindowStartTs/EndTs); zero
// endpoints (fallback mode / callers without a window identity) can never
// prove a crossing, so the verdict degrades to the scope-less disclosure.
// Containment tolerance = runtimeTraceProjActualContainmentToleranceS.
func runtimeTraceProjActualWindowScope(node types.TraceCausalProjectionNode, winStart, winEnd float64) runtimeTraceProjActualScope {
	if node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow {
		return runtimeTraceProjActualScopeAnalysisWindow // typed engine drill marker
	}
	if !runtimeTraceProjCrossWindow(node) {
		return runtimeTraceProjActualScopeNone
	}
	if node.ActualWindowStartTs <= 0 || node.ActualWindowEndTs <= node.ActualWindowStartTs {
		return runtimeTraceProjActualScopeNoInterval
	}
	if winStart <= 0 || winEnd <= winStart {
		return runtimeTraceProjActualScopeNoInterval // no analysis-window identity to judge against
	}
	tol := runtimeTraceProjActualContainmentToleranceS
	if node.ActualWindowStartTs >= winStart-tol && node.ActualWindowEndTs <= winEnd+tol {
		return runtimeTraceProjActualScopeEpisode
	}
	return runtimeTraceProjActualScopeAnalysisWindow
}

// runtimeTraceProjActualMemberQualifier is the F-5 merged-row honesty
// qualifier (CR-2 组③ P7): a ×N merged row's actual travels verbatim from the
// merge SEED — one member's physical duration beside a SUM row value read as
// 「实际 < 窗口投影」 paradox (tieba E4: 17.442 行值 vs ⚠实际6.936). "" on
// unmerged rows (byte-identical legacy face).
func runtimeTraceProjActualMemberQualifier(node types.TraceCausalProjectionNode, zh bool) string {
	if node.MergedCount <= 1 {
		return ""
	}
	if zh {
		return "(单次成员)"
	}
	return " (single member)"
}

// runtimeTraceProjActualScopeParen composes a scope word with the merged-seed
// member qualifier inside ONE paren (修复轮 P4-b: 「(超出发生段,窗内·单次
// 成员)」 — the former consecutive-paren form read as two independent chips).
func runtimeTraceProjActualScopeParen(scopeWord string, node types.TraceCausalProjectionNode, zh bool) string {
	if node.MergedCount <= 1 {
		return scopeWord
	}
	if zh {
		return scopeWord + "·单次成员"
	}
	return scopeWord + "; single member"
}

// Rows without a projection keep the chain-total fallback baseline (C00
// fallback rows), and actual ≤ baseline shapes stay byte-identical.
//
// EVOLUTION RECORD (CR-2 组③ P7, 2026-07-12): this predicate is now the VALUE
// arm only ("the actual exceeds what this row projects") — the ⚠ word face
// additionally requires the typed interval verdict
// (runtimeTraceProjActualWindowScope): value-only evidence can no longer mint
// the 跨出分析窗 claim (冷读案19: 11 rows all false).
func runtimeTraceProjCrossWindow(node types.TraceCausalProjectionNode) bool {
	if node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow {
		return true
	}
	if node.ActualImpactMS <= 0 {
		return false
	}
	if node.MergedCount > 1 {
		// DISP-3 复核 P2-1 (2026-07-10): a merged row's actual travels
		// VERBATIM from the merge seed while the row cumulative is overwritten
		// with the member SUM — the row-level equality carve-out below can
		// structurally never see a dual-scope SEED again (berlin E2 REPRO:
		// seed 21.300/27.900/actual 27.900, its own no-⚠ shape, merged with a
		// 25.000 member wore a fabricated ⚠ against MergedMaxMS). The
		// member-level carve-out compares against the donor member's own
		// pre-merge chain total; a merged row WITHOUT the donor field (any
		// path that did not travel the merge authority) suppresses ⚠ outright
		// — a fake ⚠ fabricates, a missing ⚠ merely under-discloses (宁漏勿假).
		donorCum := node.MergedActualDonorCumulativeMS
		if donorCum <= 0 {
			return false
		}
		if runtimeTraceProjRound3Equal(node.ActualImpactMS, donorCum) {
			return false
		}
		if node.MergedMaxMS <= 0 {
			return false
		}
		return node.ActualImpactMS > node.MergedMaxMS*1.001
	}
	baseline := node.ImpactMS
	if baseline <= 0 {
		baseline = node.CumulativeImpactMS
	}
	if baseline <= 0 || node.ActualImpactMS <= baseline*1.001 {
		return false
	}
	// Dual-scope duplicate: the actual channel re-published the chain total —
	// one measurement, nothing crossed.
	return !runtimeTraceProjRound3Equal(node.ActualImpactMS, node.CumulativeImpactMS)
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
	// PTV5 C11 (#68): a flat render (empty model.Target — the same typed
	// signal every flat surface reads) has no upstream-tracing structure, so
	// the fixed head clause states the flat layout instead of contradicting
	// the fence's own "按层级平铺展示" header.
	flatHead := strings.TrimSpace(model.Target) == ""
	if zh {
		headClause := "- 自上而下 = 从关注线程向上游追溯。"
		if flatHead {
			headClause = "- 各行按层级平铺,不构成上下游链。"
		}
		lines := []string{
			tracefence.AuxTreeLegendMarker,
			headClause,
			// PTV6-C ruling C (#73, 用户裁定 2026-07-06): the grounding clause
			// points at the report's own evidence index (which now carries
			// trace line/time coordinates) — the intermediate trace_query
			// record file is no longer a user-facing pointer target.
			// PTV8-RCR-B (UXA 域A #6, 2026-07-08). EVOLUTION RECORD: 「不是额外
			// 推测」防御性废词删除;E#(+N) 记法教学前移到首次出现之前(原教学
			// 远在证据索引导语,导语版保留).
			"- 时长与排序均来自 trace 证据;行尾 [E#] 可在文末证据索引查到对应 trace 行号/时间区间,E#(+N) 表示另合并 N 条同类观测(与 N次 实例合并计数是两种口径,互不换算)。",
		}
		lines = append(lines, runtimeTraceProjLegendGroupLines(model.Marks, true)...)
		sections = append(sections, strings.Join(lines, "\n"))
	} else {
		headClause := "- Top-down = tracing upstream from the focused thread."
		if flatHead {
			headClause = "- Rows are laid out flat by level; they do not form an upstream chain."
		}
		lines := []string{
			"Tree reading:",
			headClause,
			"- Durations and ranks come from trace evidence; a trailing [E#] resolves to trace line/time spans in the evidence index at the end, and E#(+N) means N more observations of the same kind were merged in (a different count from n=N instance merging; the two never convert).",
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
		//
		// PTV5 C13 (#68 P1, 2026-07-05): the parenthetical names ONLY the
		// root_cause_ family — the gate ignores wakeup_chain observations, so
		// listing wakeup_chain here asserted "wakeup_chain never ran" on runs
		// that ran exactly that view (假陈述). The sentence lists exactly what
		// the typed flag reads.
		switch {
		case !projection.RootCauseFamilyObserved:
			if zh {
				sections = append(sections, "背景层: 本报告未运行背景统计(root_cause_rank),背景层无数据;如需背景压力证据,可追问一次背景压力分析(root_cause_rank)。")
			} else {
				sections = append(sections, "Background layer: this report ran no background statistics (root_cause_rank), so this layer has no data. For background-pressure evidence, ask a follow-up background-pressure analysis (root_cause_rank).")
			}
		case zh:
			sections = append(sections, "背景层: 已查背景统计,但没有拿到有数据支撑的背景压力证据;这不等于背景没有影响,只表示本报告没有可审计的背景统计。")
		default:
			sections = append(sections, "Background layer: background statistics were checked, but produced no data-backed background-pressure evidence. This does not prove there was no background influence; it only means this report lacks auditable background statistics.")
		}
	}
	return strings.Join(sections, "\n\n")
}

// runtimeTraceProjConclusionLine is FACT-ONLY: subject, cause, magnitudes and
// the typed drilldown target. It never emits advice/should-sentences — the
// system must not ghost-write the user-facing recommendation surface.
// runtimeTraceProjSelfCrownTargetKey resolves the analysis-target identity
// for the §29.30 self-cause crown wording from TYPED carriers only: the
// wakeup-path target (tree render), else the four-state account's subject,
// else the unanimous subject of the engine's target_self_state stamps (the
// tid-first subject==target match minted engine-side travels on that tier —
// the FLAT render has no model.Target, cmp_78 witness shape). "" when no
// carrier is present or the stamped subjects disagree (absence never
// guesses).
func runtimeTraceProjSelfCrownTargetKey(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) string {
	if key := runtimeTraceCausalProjectionCanonicalNode(model.Target); key != "" {
		return key
	}
	if account := projection.TargetStateAccount; account != nil {
		if key := runtimeTraceCausalProjectionCanonicalNode(account.Subject); key != "" {
			return key
		}
	}
	key := ""
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			if !rows[i].Node.IsTargetSelfStateRow() {
				continue
			}
			subject := runtimeTraceCausalProjectionCanonicalNode(rows[i].Node.Subject)
			if subject == "" {
				continue
			}
			if key == "" {
				key = subject
				continue
			}
			if key != subject {
				return "" // ambiguous stamps: never guess an identity
			}
		}
	}
	return key
}

// runtimeTraceProjSelfCauseCrownState resolves the crowned node's §24.17
// self-cause state token + §24.3 family category word for the §29.30 crown
// wording (自因成因形): the node is the focused thread's OWN row (typed
// target identity via runtimeTraceProjSelfCrownTargetKey) and its typed
// impact form belongs to the self-cause four-family closed set. The state
// vocabulary is the §29.27 four-state account's kernel state words
// (running/runnable/D-state zh-en 同词; the IO label pair IO等待 / IO wait);
// the category word is the SAME §24.3 table row the tree 行2 speaks — one
// typed form resolution mints both (no drift). state=="" → the crown keeps
// the external-cause sentence byte-identically (外因句式零变).
func runtimeTraceProjSelfCauseCrownState(primary types.TraceCausalProjectionNode, projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) (state, category string) {
	if primary.IsTargetSelfStateRow() || primary.IsContextOnlyRow() {
		return "", ""
	}
	target := runtimeTraceProjSelfCrownTargetKey(projection, model)
	if target == "" || runtimeTraceCausalProjectionCanonicalNode(primary.Subject) != target {
		return "", ""
	}
	form := runtimeTraceProjImpactFormForNode(primary, "")
	switch form {
	case runtimeTraceProjImpactFormRunning:
		state = "running"
	case runtimeTraceProjImpactFormRunnable:
		state = "runnable"
	case runtimeTraceProjImpactFormDState:
		state = "D-state"
	case runtimeTraceProjImpactFormDStateIOMixed:
		// 件③ tri-form: the crown speaks the same merged compound the row
		// face speaks (三面一说).
		state = "D-state/iowait"
	case runtimeTraceProjImpactFormIOBlock:
		if zh {
			state = "IO等待"
		} else {
			state = "IO wait"
		}
	default:
		return "", ""
	}
	if spec, ok := runtimeTraceProjImpactFormSpecFor(form); ok {
		if zh {
			category = spec.CategoryZH
		} else {
			category = spec.CategoryEN
		}
	}
	// A5 反转词位 (sweep M8, 2026-07-17): the crown's category word follows the
	// SAME per-token composer the tree 行2 now speaks for a state-form row
	// carrying a priority-inversion family token (E6-shape self seat: Object=
	// priority_inversion_runnable_wait, StateKind=runnable) — without this the
	// crown would restate the form table's 调度压力候选 beside a 行2 saying
	// 优先级反转·可运行等待, re-opening the exact drift this function's
	// contract ("one typed form resolution mints both") forbids. Flag-lane
	// rows never reach here (FormInversion is outside the four-family set).
	if word, ok := runtimeTraceProjInversionFamilyWord(primary, zh); ok {
		category = word
	}
	// CLOSE-1 复核捎带 V-2 (2026-07-11): the D family's category word restates
	// the state token (「D-state D状态候选」/ "D-state D-state candidate" —
	// the runnable-precedent duplication shape), so the crown keeps the
	// kernel state word alone. The IO pair stays (等待 vs 阻塞候选 are
	// distinct morphemes, reviewed and kept); runnable/running categories
	// are semantically distinct family words.
	if form == runtimeTraceProjImpactFormDState || form == runtimeTraceProjImpactFormDStateIOMixed {
		// 件③: the mixed compound restates its state token the same way.
		category = ""
	}
	return state, category
}

// runtimeTraceProjSelfCauseCrownName is the §29.30 self-cause crown head —
// the ONE morpheme source shared by the conclusion line and the comparison
// primary cell (「主根因: 关注线程自身 {state}…」, never the external
// thread-name form). The 关注线程自身 word is the same self row-kind lane
// token the detail 位置 cell speaks.
func runtimeTraceProjSelfCauseCrownName(state string, zh bool) string {
	if zh {
		return "关注线程自身 " + state
	}
	return "the focused thread itself " + state
}

func runtimeTraceProjConclusionLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	primary, lane := runtimeTraceProjLeadSelect(projection, model)
	onChainFallback := lane == runtimeTraceProjLeadLaneOnChainFallback
	if primary != nil && lane == runtimeTraceProjLeadLaneSemanticFallback {
		// §21 LEAD-SEM (cmp_01 A①): the semantic tier-4 lead is an
		// optimization-span statement, never a 主根因 claim — the dedicated
		// single-source wording carries no "主根因:" prefix (负向 pin).
		if zh {
			return runtimeTraceProjSemanticLeadText(*primary, model, true) + "。"
		}
		return runtimeTraceProjSemanticLeadText(*primary, model, false) + "."
	}
	if primary == nil {
		// SYM (§24.13 裁定一, 2026-07-08): the honest-fallback verdict carries
		// the target-self symptom disclosure whenever ranked self rows exist
		// ("" on every legacy shape — bytes unchanged).
		selfNote := runtimeTraceProjTargetSelfSymptomNote(model, zh)
		if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
			if selfNote == "" {
				return "" // legacy no-rank-data shape, byte-stable
			}
			// SYM (§24.13 裁定一): rank data exists but EVERY ranked row is
			// the focused thread's own symptom (the all-self degenerate
			// board) — the ladder skip left the primary bucket empty, so the
			// lead speaks the honest fallback plus the disclosure instead of
			// crowning the target's own wait as its own root cause. The
			// LEAD-SEM semantic lane stays unreachable here by design (the
			// empty-primary gate is the ruling's 诚实回退 lane).
			if zh {
				return "**主根因:** 窗口内未定位到链上主根因" + selfNote + "。"
			}
			return "**Primary root cause:** no on-chain primary root cause was located in the window" + selfNote + "."
		}
		// Every primary candidate was demoted to the background stanza (§7.30
		// 裁定1) and no data-bearing on-chain row could lead either (RN-3(a))
		// — the lead must say so instead of naming a demoted row as the
		// primary root cause and contradicting the tree below it.
		//
		// §21 LEAD-SEM L3 (cmp_01 A② defensive check): the 见背景压力段
		// pointer renders only when the background stanza is actually
		// non-empty — an empty stanza would make the pointer dangle against
		// the 背景层 two-state split below.
		if zh {
			if len(model.Background) > 0 {
				// DISPHYG-3 件1 (C8): prose-sentence top-level clause comma
				// is full-width (see the window-line regime note).
				return "**主根因:** 窗口内未定位到链上主根因，见背景压力段" + selfNote + "。"
			}
			return "**主根因:** 窗口内未定位到链上主根因" + selfNote + "。"
		}
		if len(model.Background) > 0 {
			return "**Primary root cause:** no on-chain primary root cause was located in the window — see the background-pressure stanza" + selfNote + "."
		}
		return "**Primary root cause:** no on-chain primary root cause was located in the window" + selfNote + "."
	}
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(*primary, zh))
	// D4: the narrative lane uses the 中文（english_token） combined format on
	// the zh surface (tree rows stay concise zh; the table keeps raw tokens).
	cause := strings.TrimSpace(runtimeTraceCausalProjectionNarrativeCauseName(primary.Object, zh))
	// 修复轮 P2-3 crown face (2026-07-12): a crowned refined-D row's cause
	// word consumes the proof — the merged compound must not resurface on
	// the 主根因 line beside the refined seat (同段词面互斥灭; the D4 raw
	// token parenthetical stays for audit fidelity on the zh face).
	if zh && primary.DStateRefinedNonIO &&
		runtimeTraceCausalProjectionCanonicalNode(primary.Object) == "d_state_or_io_wait" {
		cause = runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: "d_state"}, true) +
			"（d_state_or_io_wait）"
	}
	if primary.IsAggregateMetric() {
		// The metric semantic name already carries the Object type word.
		cause = ""
	}
	// §29.30 (用户裁定 2026-07-11): a crowned SELF-CAUSE row speaks the 自因成因形
	// crown — head 「关注线程自身 {state}」 (never the external thread-name
	// syntax), and, for a running lead whose §29.27 four-state account is
	// provable, the Tier-A parenthetical decomposition
	// (确定性工作 X · 供给折算影响 Y · 自身执行 Z — the account running line's
	// OWN parts, single morpheme+value source; separators stay non-additive,
	// §7.30 S1 lesson) replaces the cause word + magnitude grammar. Every
	// non-self crown keeps the external sentence byte-identically (负向 pin:
	// vc_710 外因主导帧回归).
	selfState, selfCategory := runtimeTraceProjSelfCauseCrownState(*primary, projection, model, zh)
	var selfAccountParts []string
	if selfState != "" {
		name = runtimeTraceProjSelfCauseCrownName(selfState, zh)
		// Tier B cause word = the row's own §24.3 category (调度压力候选/…) —
		// the narrative Object name would restate the state token
		// (「runnable runnable(runnable_wait)」 duplication form).
		cause = selfCategory
		if selfState == "running" {
			if account := runtimeTraceProjFourStateAccountProvable(projection, model); account != nil {
				if parts, ok := runtimeTraceProjFourStateRunningParts(account, model, zh); ok && len(parts) > 0 {
					// 复核观察② (留档, 2026-07-11): when no rendered row carries
					// the typed supply-fold deficit, the parts builder omits the
					// 供给折算影响 component — BY THE ACCOUNT LINE'S OWN
					// absence-never-guesses semantics (runtimeTraceProjFourState
					// SupplyPointer: no rendered carrier → no converted claim).
					// The crown reuses the SAME parts, so the two faces cannot
					// disagree; the omission is the shared refusal, not a crown
					// fork — acceptable by construction, no extra pointer text.
					selfAccountParts = parts
				}
			}
		}
	}
	// PTV5 C21/C22 (#68 用户裁定 2026-07-05): the headline magnitude carries
	// its caliber word at the point of reading (链上累计/有效归因/窗口投影/…,
	// same (a)-table vocabulary), and a periodic source renders its discounted
	// attribution EXPLICITLY — 0.000ms included (0 IS the finding, F5(a) 同理)
	// — instead of the former silent ms>0 drop. 复核 Med (2026-07-06): the
	// (占窗X%) share is a WINDOW-PROJECTION statement — the C00 同源门 applies
	// here too, so a non-window-source magnitude prints its caliber word and
	// never a share (no fake >100% lane on the headline either).
	ms, msWord, msPeriodic, msWindowSource := runtimeTraceProjConclusionMagnitude(*primary, zh)
	var b strings.Builder
	if zh {
		b.WriteString("**主根因:** ")
	} else {
		b.WriteString("**Primary root cause:** ")
	}
	b.WriteString(name)
	if selfAccountParts != nil {
		// Tier A (§29.30 constraint ②): the account decomposition IS the
		// magnitude statement — the cause word and the generic ms grammar
		// below stay silent (their values would double-speak the parts).
		b.WriteString("(" + strings.Join(selfAccountParts, " · ") + ")")
	} else if cause != "" {
		b.WriteString(" " + cause)
	}
	// P0-E 锁车道修3 (§24.9-C F5): a lead whose lock-holder identity is an
	// INFERENCE never states the hold as a payload fact — the qualifier rides
	// the conclusion line (typed holder_source lanes; detail stanza carries
	// the full origin sentence).
	if runtimeTraceProjBlockingHolderInferred(*primary) {
		if zh {
			b.WriteString("(持有者推断)")
		} else {
			b.WriteString(" (holder inferred)")
		}
	}
	if selfAccountParts == nil && primary.MergedCount > 1 && primary.MergedMaxMS > 0 {
		// V1 (customer revisit 2026-07-03): a ×N aggregate's SUM never publishes
		// as the headline hard fact — show the per-instance max with the count;
		// the window share follows the same single-instance value.
		// §21.1 CWD-2 复核收尾③ (W1-b 结论行 % 面): a MULTI-WINDOW merged
		// lead's single-instance max was measured in its member's own query
		// window — never provably the anchor window — so the (占窗N%) share
		// is suppressed on the same typed key as the tree-row %-face
		// (runtimeTraceProjMultiWindowMergedRow; member windows stay
		// disclosed on the row's 窗来源 lane). ≤1-window rosters keep the
		// legacy share byte-identically.
		shareable := model.WindowMS > 0 && !runtimeTraceProjMultiWindowMergedRow(*primary)
		if zh {
			b.WriteString(fmt.Sprintf(" 单次最大 %.3fms(共%d次)", primary.MergedMaxMS, primary.MergedCount))
			if shareable {
				b.WriteString(fmt.Sprintf("(占窗%.0f%%)", primary.MergedMaxMS/model.WindowMS*100))
			}
		} else {
			b.WriteString(fmt.Sprintf(" single max %.3fms (of %d)", primary.MergedMaxMS, primary.MergedCount))
			if shareable {
				b.WriteString(fmt.Sprintf(" (%.0f%% of window)", primary.MergedMaxMS/model.WindowMS*100))
			}
		}
	} else if selfAccountParts == nil && (ms > 0 || msPeriodic) {
		if msWord != "" {
			b.WriteString(" " + msWord)
		}
		b.WriteString(fmt.Sprintf(" %.3fms", ms))
		if msPeriodic {
			if zh {
				b.WriteString("(期内节拍已折算)")
			} else {
				b.WriteString(" (in-period cadence discounted)")
			}
		}
		// GATED-CAL 件2 (§29.104.16.1 M4, 2026-07-16): a self/semantic family
		// lead names its same-thread family accounting at the point of reading
		// — the 行3-form 「 = 合计(共N段,同线程)」 equation suffix (identity-
		// gated inside the helper; "" on every other lead).
		b.WriteString(runtimeTraceProjConclusionFamilyCaliberSuffix(*primary, zh))
		// §21.1 CWD-2 复核收尾③ (W1-b, fail-closed twin of the merged branch
		// above): a multi-window merged lead that lands here (MergedMaxMS
		// absent) must not divide its window-sourced magnitude by the anchor
		// window either — same typed key, same suppression.
		if msWindowSource && ms > 0 && model.WindowMS > 0 && !runtimeTraceProjMultiWindowMergedRow(*primary) {
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
	// PTV8-RCR-A (§24 ②): an inversion primary never carries the 机制构成
	// mechanism sentence (Triple/WithDemand branches) — the conclusion appends
	// the 行3-form attribution breakdown instead (single-source with the
	// fence's 「=」line); non-sentence verdicts and non-inversion primaries
	// keep the clause byte-identically.
	conclusionVerdict := runtimeTraceProjSupplyFoldVerdictFor(*primary, model.WindowMS)
	conclusionMechanism := conclusionVerdict == runtimeTraceProjSupplyFoldTriple ||
		conclusionVerdict == runtimeTraceProjSupplyFoldWithDemand
	if runtimeTraceCausalProjectionInversionRow(*primary) && (conclusionMechanism || conclusionVerdict == runtimeTraceProjSupplyFoldNone) {
		// 复核 FAIL-2: the equation renders through THE shared template
		// (runtimeTraceProjAttributionEquation) — never a private copy; 复核
		// F4: the degenerate single-full shape appends no equation (the fence
		// folds it into 行2's tail; a one-term composite is no story).
		if components, total, ok := runtimeTraceProjInversionComponents(*primary, zh); ok &&
			!runtimeTraceProjInversionDegenerateSingleFull(components) {
			word := "有效归因"
			if !zh {
				word = "attribution"
			}
			if zh {
				// DISPHYG-3 件1 (C8): prose-sentence top-level clause comma.
				b.WriteString("，")
			} else {
				b.WriteString(", ")
			}
			b.WriteString(word + " " + runtimeTraceProjAttributionEquation(total, components))
		}
	} else if clause, _, ok := runtimeTraceProjSupplyFoldClause(*primary, model.WindowMS, zh); ok && selfAccountParts == nil {
		// Tier A suppression: the 供给折算影响 component inside the account
		// parenthetical already speaks the fold — the standalone clause would
		// double-state the same converted figure.
		if zh {
			// DISPHYG-3 件1 (C8): prose-sentence top-level clause comma.
			b.WriteString("，")
		} else {
			b.WriteString(", ")
		}
		b.WriteString(clause)
	}
	if target := strings.TrimSpace(primary.DrilldownTarget); target != "" && primary.IsSleepState() {
		if zh {
			b.WriteString("，下钻到 " + target)
		} else {
			b.WriteString(", drills down to " + target)
		}
	} else if primary.IsSleepState() && primary.Undrillable() {
		if zh {
			b.WriteString("，⊘窗口内无匹配唤醒、无法继续下钻")
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

// runtimeTraceProjLeadPrimary picks the lead-line primary. Two lanes:
//
//  1. the RANKED lane (LEAD 修, ledger §24.11 C-1, 2026-07-08): the shared
//     post-aggregation rank board (runtimeTraceProjRankBoard — the SAME
//     population, order and key the ❶❷❸ badge lane seats from), head row
//     wins. A primary-lane lead is therefore ALWAYS the ❶-badged row when a
//     ❶ exists (两车道恒等 pin: TestCOVLeadPrimaryIsBadgeOneRow);
//  2. no board row: the largest single-instance effective attribution among
//     the non-demoted primary roots (the V1 rankless lane, unchanged — it
//     covers no-rank shapes, e.g. the sole-periodic 0.176ms conclusion).
//
// EVOLUTION RECORD (§24.11 C-1): the former ranked lane read the
// PRE-aggregation projection.PrimaryRootCauses bucket (cap=10 after an
// in-path-class-first sort) with lowest-Rank-wins — window-local rank ordinals
// collide across query boards (the specimen carried two #1), and the cap
// evicted the tree's own ❶ row (E9 rank#1 inversion, eff 6.430) so the
// target's own binder-wait symptom row (4.577) was crowned 主根因 against the
// report's own badges. The board's eff-descending key IS the engine's rank key
// generalized across boards (sortRootCauseRankItems orders by effective
// attribution within a window); rank semantics themselves are untouched
// (§20 方向盲区裁定点另案). The V1/F4 ×N-SUM discipline is preserved by
// construction: the board key is EffectiveImpactMS (never a merged
// window-projection SUM), and the rankless lane keeps the
// runtimeTraceProjLeadSelectionValue caliber.
//
// An EMPTY primary bucket still returns nil unconditionally (the legacy
// no-rank-data gate lives in runtimeTraceProjLeadSelect — lane 1 only replaces
// the election-population half, never the no-conclusion shapes).
func runtimeTraceProjLeadPrimary(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) *types.TraceCausalProjectionNode {
	roots := runtimeTraceCausalProjectionPrimaryRoots(projection)
	if len(roots) == 0 {
		return nil
	}
	if board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model)); len(board) > 0 {
		return &board[0].Node
	}
	var best *types.TraceCausalProjectionNode
	bestValue := 0.0
	for i := range roots {
		if roots[i].IsContextOnlyRow() {
			continue
		}
		// UXR-1 复核 P2-2(b) 裁定 (2026-07-11): 加冕仅链通道行 — a typed
		// off-chain-relevance root (adjacent/background) never crowns through
		// the rankless value lane either (the same channel belt the shared
		// valid-seat gate wears; a PRIMARY-lane row with adjacent relevance is
		// exactly the P2-2 name-key leak shape). Empty relevance stays on the
		// chain universe (fail-open, matches the ordinal-channel authority).
		if runtimeTraceProjNodeOrdinalChannel(roots[i]) != runtimeTraceProjOrdinalChannelChain {
			continue
		}
		// §29.30.1 (2026-07-11): a zero-CAP running root — the engine's
		// supply-fold PUBLISHED a computed zero deficit (typed
		// SupplyFoldComputed ∧ deficit≤0, the §20.2 authoritative "已按大核
		// 满频…无供给缺口" verdict) — lost its seat at the shared valid-seat
		// gate and must not re-crown through this value lane either (远端
		// 点名负对照: 零 CAP running 不得重新加冕; the crown would contradict
		// the row's own no-deficit verdict). This arm stays beside the EPUB
		// generalization below: it keys on the fold telemetry, so it also
		// covers eff-UNPUBLISHED zero-CAP forms the marker cannot see.
		if roots[i].Rank > 0 && roots[i].SupplyFoldComputed && roots[i].SupplyFoldDeficitMS <= 0 {
			continue
		}
		// EPUB (§29.31 立案 → 本批, 2026-07-11): the GENERAL published-eff≤0
		// refusal arm the former SCOPE NOTE here waited for. The engine
		// PUBLISHED this root's effective attribution (typed
		// EffectiveImpactPublished — note presence at the decode single point,
		// never re-derived from the float64 zero) and it is ≤0: crowning it
		// would contradict the engine's own published zero, so it neither
		// crowns nor re-enters through the fallback lanes (RN-3(a) already
		// hard-gates eff>0). Typed semantic exemptions keep their crowns:
		//   - PeriodicSource (#68 用户裁定 2026-07-05): a periodic root
		//     competes with its DISCOUNTED attribution even at exactly 0 and
		//     the conclusion words 「有效归因 0.000ms(期内节拍已折算)」 —
		//     the discounted-zero crown IS the load-bearing verdict there;
		//   - eff-UNPUBLISHED roots (marker false — supply-fold triple /
		//     lock-holder / witness-golden shapes): the VS2 故意 pin form
		//     stays byte-identical (the row leads, and the 行3 有效归因 total
		//     REFUSES to render — 值拒造, never fabricated from components).
		// Exemptions are typed flags only — never word-face or noisy signals
		// (架构红线: precise signals for hard gates).
		if roots[i].EffectiveImpactPublished && roots[i].EffectiveImpactMS <= 0 && !roots[i].PeriodicSource {
			continue
		}
		if runtimeTraceProjNodeDemotedToBackground(roots[i], model.TrunkLen) {
			continue
		}
		if v := runtimeTraceProjLeadSelectionValue(roots[i]); best == nil || v > bestValue {
			best, bestValue = &roots[i], v
		}
	}
	return best
}

// runtimeTraceProjLeadLane is the typed lane the single lead-selection
// surface resolved through (§21 LEAD-SEM upgraded the former boolean): the
// wording of the conclusion line / compare cell forks on it — the semantic
// FALLBACK lane must NEVER wear the 主根因 claim (负向 pin: 禁"主根因:"
// 前缀冒称).
//
// EVOLUTION RECORD (SEM-LEAD §29.7-2 ①, ledger
// real_trace_campaign_20260705.md, 2026-07-10): the ban's scope NARROWED to
// the tier-4 fallback lane (a rankless semantic span named because the lead
// came back empty-handed). An ON-CHAIN semantic row seated on the shared
// rank board resolves through the PRIMARY lane like every board row and DOES
// wear 主根因 when it is board[0] (792-textup "主根因: 纹理上传"
// 追认为正确行为) — the fallback wording stays for rows that never earned a
// rank seat.
type runtimeTraceProjLeadLane int

const (
	runtimeTraceProjLeadLaneNone runtimeTraceProjLeadLane = iota
	runtimeTraceProjLeadLanePrimary
	runtimeTraceProjLeadLaneOnChainFallback
	runtimeTraceProjLeadLaneSemanticFallback
)

// runtimeTraceProjLeadSelect is the SINGLE lead-selection surface consumed by
// the conclusion line, the comparison-overview primary cell and the model
// build (LeadKey) — one implementation, deterministic on (projection, model),
// so the three consumers can never disagree. Order:
//  1. the primary lanes (runtimeTraceProjLeadPrimary — LEAD 修 §24.11 C-1:
//     the ranked lane reads the shared post-aggregation rank board, so a
//     primary-lane lead is the ❶ row whenever ❶ exists; the V1 rankless
//     lane is unchanged);
//  2. RN-3(a) (§7.9 runnable 主导场景审计 2026-07-04): the primary bucket has
//     rows but NONE survived the 裁定1 demotion gate (the former 未定位
//     branch) → fall back to the largest data-bearing ON-CHAIN row of the
//     rendered tree (chain/flat rows, discounted single-instance value) — the
//     customer's flat runnable 635.981ms/42% row was on the table while the
//     conclusion said nothing on-chain was located;
//  3. §21 LEAD-SEM (cmp_01 A①, 2026-07-07): the on-chain fallback came back
//     empty-handed too → the largest data-bearing SEMANTIC row of the
//     rendered tree (typed engine data, never synthesized — the 6.0 specimen
//     had a deterministic JIT optimization span at 83% of the window while
//     the lead pointed at low-confidence background aggregates). The caller
//     words it as an optimization-span statement, never a 主根因 claim;
//  4. still nothing → nil, and the caller keeps the 未定位/背景压力段 text.
//
// An EMPTY primary bucket keeps the legacy no-conclusion behavior (the
// fallbacks only replace the contradiction case, not the no-rank-data case).
// The second return names the lane that produced the lead (callers append
// the RN-3(a) short note / the LEAD-SEM wording).
func runtimeTraceProjLeadSelect(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (*types.TraceCausalProjectionNode, runtimeTraceProjLeadLane) {
	if primary := runtimeTraceProjLeadPrimary(projection, model); primary != nil {
		return primary, runtimeTraceProjLeadLanePrimary
	}
	if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
		// SEM-LEAD (§29.7-2 ①, ledger real_trace_campaign_20260705.md,
		// 2026-07-10): a window whose ONLY on-chain competitors are semantic
		// rows can leave the primary-ROOT set empty (审计 #66 comment repair:
		// post-retirement RANK-lane semantic rows DO mint ordinary
		// primary/secondary/tertiary tiers — types.go
		// RootCauseTierDeterministicOptimization record; but the primary-root
		// compile buckets key on the rank-lane records, and the surviving ✦
		// OBSERVATION-lane seat that adopted the rank ordinal via the twin
		// fold is not a primary root), so the legacy empty-primary gate
		// structurally denied them the crown the ruling grants (必须能参赛且
		// 有机会登顶). When board seat ❶ IS an on-chain semantic row with a
		// positive attribution, it crowns through the primary lane; every
		// other empty-primary shape keeps the legacy no-conclusion behavior
		// byte-identically (fail-open).
		if board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model)); len(board) > 0 &&
			board[0].Kind == runtimeTraceProjTreeRowSemantic &&
			strings.TrimSpace(board[0].Node.ChainRelevance) == "on_chain" &&
			board[0].Node.EffectiveImpactMS > 0 {
			return &board[0].Node, runtimeTraceProjLeadLanePrimary
		}
		return nil, runtimeTraceProjLeadLaneNone
	}
	if fallback := runtimeTraceProjLeadOnChainFallback(model); fallback != nil {
		return fallback, runtimeTraceProjLeadLaneOnChainFallback
	}
	if semantic := runtimeTraceProjLeadSemanticFallback(model); semantic != nil {
		return semantic, runtimeTraceProjLeadLaneSemanticFallback
	}
	return nil, runtimeTraceProjLeadLaneNone
}

// runtimeTraceProjLeadSemanticFallback picks the §21 LEAD-SEM tier-4 lead:
// among all rendered data-bearing trace_semantic_span rows, the one with the
// largest discounted single-instance value (runtimeTraceProjLeadSelectionValue
// — the SAME caliber as the RN-3(a) lane: ×N SUMs never compete). Rows whose
// typed WithinRequestedWindow marker says in-window are preferred over
// drilled-out-of-window rows (优先窗内行 — typed pointer only, never the
// arithmetic cross-window heuristic); a 0-value best returns nil rather than
// publishing a 0ms "largest span". Ties keep the earlier render-order row.
func runtimeTraceProjLeadSemanticFallback(model runtimeTraceProjTreeModel) *types.TraceCausalProjectionNode {
	var best, bestInWindow *types.TraceCausalProjectionNode
	bestValue, bestInWindowValue := 0.0, 0.0
	groups := [][]runtimeTraceProjTreeRow{model.TreeRows, model.Adjacent, model.Background}
	for _, rows := range groups {
		for i := range rows {
			row := &rows[i]
			if !row.HasData || (row.Kind != runtimeTraceProjTreeRowSemantic &&
				!runtimeTraceCausalProjectionSemanticSpanRow(row.Node)) {
				continue
			}
			if row.Node.IsContextOnlyRow() {
				continue
			}
			v := runtimeTraceProjLeadSelectionValue(row.Node)
			if v > bestValue {
				best, bestValue = &row.Node, v
			}
			inWindow := row.Node.WithinRequestedWindow == nil || *row.Node.WithinRequestedWindow
			if inWindow && v > bestInWindowValue {
				bestInWindow, bestInWindowValue = &row.Node, v
			}
		}
	}
	if bestInWindow != nil {
		return bestInWindow
	}
	return best
}

// runtimeTraceProjSemanticLeadText is the SINGLE §21 LEAD-SEM wording source
// shared by the conclusion line and the comparison primary cell (no trailing
// period — callers add their own). Fixed form per the cmp_01 A① fix
// direction: it states that no on-chain primary was located and names the
// window's largest semantic optimization span WITHOUT claiming a root cause
// (负向 pin: never a "主根因:" prefix). The 占窗 share follows the C00 同源门
// (only a window-projection magnitude may publish one); the 见背景压力段
// pointer renders only when the background stanza is non-empty (L3, cmp_01
// A② defensive check — the semantic lane made the formerly unreachable empty
// shape reachable).
func runtimeTraceProjSemanticLeadText(node types.TraceCausalProjectionNode, model runtimeTraceProjTreeModel, zh bool) string {
	ms := runtimeTraceProjLeadSelectionValue(node)
	// RCM-2 D3 (零链括注同步的姊妹面): a family lead names the class + ×N and
	// qualifies the magnitude with the family stem — a same-thread total must
	// not read as one span's duration. Non-family nodes stay byte-identical.
	name, valueCell := runtimeTraceProjSemanticCellParts(node, ms, zh)
	share := ""
	if text := runtimeTraceProjSemanticSpanShareText(node, ms, model, zh); text != "" {
		if zh {
			share = text + ","
		} else {
			share = text + ", "
		}
	}
	pointer := ""
	if len(model.Background) > 0 {
		if zh {
			pointer = ",见背景压力段"
		} else {
			pointer = "; see the background-pressure stanza"
		}
	}
	if zh {
		// DISPHYG-3 件1 (C8): prose-sentence top-level clause semicolon is
		// full-width (the caller appends 。); parenthetical interiors keep
		// their half-width bytes (shared word-face level, see the window-line
		// regime note).
		return fmt.Sprintf("未定位到链上主根因；窗口内最大语义优化span: %s %s(%s语义优化span·无唤醒链%s)", name, valueCell, share, pointer)
	}
	return fmt.Sprintf("no on-chain primary root cause located; largest in-window semantic optimization span: %s %s (%ssemantic optimization span · no wakeup chain%s)", name, valueCell, share, pointer)
}

// runtimeTraceProjSemanticSpanShareText is the SINGLE share-wording source for
// semantic optimization spans on the conclusion line, the comparison primary
// cell and the F3b 确定性优化点 column (DCS E6 + LEAD-SEM 协调, ledger §23.1):
// the C00 same-source gate first (only a window-projection magnitude may
// publish a share), then the E5 source-window base — a row measured in a
// DIFFERENT typed query window divides by ITS OWN window ("占其查询窗N%"),
// never raw-duration ÷ anchor length; rows without a typed source window (or
// matching the anchor within ±1ms) keep the legacy 占窗 form byte-identically.
// "" = no share publishable.
func runtimeTraceProjSemanticSpanShareText(node types.TraceCausalProjectionNode, ms float64, model runtimeTraceProjTreeModel, zh bool) string {
	shareOK := false
	switch {
	case runtimeTraceProjFamilyRow(node):
		// RCM-2 D3 (witness 占其查询窗9%): the family participation value is a
		// WINDOW-CLIPPED interval total by engine construction (§24.22 M1 —
		// members are window-clipped spans, overlaps as their union), so the
		// share is publishable; the E5 source-window rebase below still picks
		// the base (占其查询窗 when the family was measured elsewhere).
		shareOK = true
	case node.MergedCount > 1 && node.MergedMaxMS > 0:
		// The per-instance max IS a window projection (same V1 headline rule).
		shareOK = true
	case node.EffectiveImpactMS > 0:
		// The selection value fell to the attribution caliber — not a window
		// projection, so no share (C00).
		shareOK = false
	default:
		_, source := runtimeTraceProjNodeDisplayImpactSource(node)
		shareOK = source == runtimeTraceProjImpactSourceWindow
	}
	if !shareOK || ms <= 0 || model.WindowMS <= 0 {
		return ""
	}
	// A6 (§23.2 残余⑥): the source-window rebase judgment is the SAME shared
	// helper the tree % lane reads — one ±1ms boundary, no drift.
	base := model.WindowMS
	sourceBased := false
	if sourceMS, ok := runtimeTraceProjSemanticSourceWindowRebaseMS(node, model.WindowMS); ok {
		base, sourceBased = sourceMS, true
	}
	pct := ms / base * 100
	switch {
	case zh && sourceBased:
		return fmt.Sprintf("占其查询窗%.0f%%", pct)
	case zh:
		return fmt.Sprintf("占窗%.0f%%", pct)
	case sourceBased:
		return fmt.Sprintf("%.0f%% of its query window", pct)
	default:
		return fmt.Sprintf("%.0f%% of window", pct)
	}
}

// runtimeTraceProjSemanticTopSpan resolves the model's TOP semantic
// optimization span through the SAME typed selection as the LEAD-SEM fallback
// (in-window rows preferred, ×N SUMs never compete, 0-value never leads) —
// one selector, three consumers (conclusion fallback, F3b column, zero-chain
// primary-cell presence note), so they can never name different spans.
func runtimeTraceProjSemanticTopSpan(model runtimeTraceProjTreeModel) (*types.TraceCausalProjectionNode, float64, bool) {
	node := runtimeTraceProjLeadSemanticFallback(model)
	if node == nil {
		return nil, 0, false
	}
	ms := runtimeTraceProjLeadSelectionValue(*node)
	if ms <= 0 {
		return nil, 0, false
	}
	return node, ms, true
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
		// PTS (#68): the on-chain overflow fold row is a counted roster, not a
		// nameable cause — it never leads a conclusion (typed flag, precise).
		if row.Node.OnChainOverflowFold {
			continue
		}
		if row.Node.IsContextOnlyRow() {
			continue
		}
		// Precise-signal hard gate: RN-3 may only use an explicitly published
		// positive effective attribution. Missing/zero must never fall back to
		// a raw duration from RootEvidence or critical-blocking supporting rows.
		if row.Node.EffectiveImpactMS <= 0 {
			continue
		}
		// SYM (§24.13 裁定一, 复核 F1, 2026-07-08): the target's own state rows
		// never lead through the RN-3(a) fallback lane either — post-SYM this
		// shape is MORE reachable (the ladder skip empties the primary slots a
		// self row used to hold), and a self row is naturally the window's
		// largest on-chain wait, so without this arm the fallback re-crowned
		// exactly the 加冕 §24.13 retires ("主根因: main-6565 binder等待…
		// (链不可上溯,按窗口内最大链上等待)"). With every fallback candidate
		// self, the nil return routes the conclusion to the honest-fallback
		// branch, which already carries the symptom disclosure.
		if row.Node.IsTargetSelfStateRow() {
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
			return "(链不可上溯,按窗口内最大链上等待)"
		}
		return " (chain not traceable upstream; largest on-chain wait in the window)"
	}
	if zh {
		return "(根因排序候选均降背景,按窗口内最大链上等待)"
	}
	return " (all ranked candidates demoted to background; largest on-chain wait in the window)"
}

// runtimeTraceProjTargetSelfSymptomNote is the SYM (§24.13 裁定一, 2026-07-08)
// symptom-disclosure parenthetical for the honest-fallback lead lanes: when the
// report carries ranked rows whose subject is the analysis target itself
// (typed tier token, engine tid-first identity — never a label comparison),
// the 未定位到链上主根因 verdict additionally discloses the target's own
// wait/lock-hold magnitude and points at its own state rows. Magnitude = the
// single largest runtimeTraceProjLeadSelectionValue over those rows (复核 F2:
// the SAME single-instance caliber every lead lane competes with — a ×N
// merged row contributes its per-instance MergedMaxMS, never its SUMmed
// ImpactMS, and a periodic row contributes its discounted attribution, never
// raw cadence sleep). 单项最大 wording engages when several rows compete OR
// the winning value is itself a per-instance max of a merged roster — the
// rows overlap on one thread's wall clock, so Σ would double count; MAX never
// invents. "" when no such row renders, or when the largest magnitude is not
// positive (never a 0ms disclosure) — every legacy no-lead shape stays
// byte-stable. Shared by the conclusion line and the comparison primary cell
// (same single-source discipline as runtimeTraceProjLeadSelect).
func runtimeTraceProjTargetSelfSymptomNote(model runtimeTraceProjTreeModel, zh bool) string {
	count := 0
	max := 0.0
	multi := false
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			// EVOLUTION RECORD (跨批 X1, GAP-B 收尾 2026-07-09): the former
			// `Rank <= 0` arm is RETIRED. GAP-A's G9 ordinal renumbering stops
			// assigning rank ordinals to tier=target_self_state rows entirely
			// (序数只分给携榜位显示身份的行), so the two predicates became
			// mutually exclusive and this §24.16 disclosure silently died on
			// every engine-produced shape. The typed tier token is the precise
			// signal and alone sufficient — it is minted exactly on the
			// engine's tid-first subject==target wait-symptom match.
			if !row.HasData || !row.Node.IsTargetSelfStateRow() {
				continue
			}
			count++
			if row.Node.MergedCount > 1 {
				multi = true // the published value is a per-instance max already
			}
			if v := runtimeTraceProjLeadSelectionValue(row.Node); v > max {
				max = v
			}
		}
	}
	if count == 0 || max <= 0 {
		return ""
	}
	if count > 1 {
		multi = true
	}
	if zh {
		if multi {
			return fmt.Sprintf("(关注线程自身等待/持锁 单项最大 %.3fms,见关注线程自身行)", max)
		}
		return fmt.Sprintf("(关注线程自身等待/持锁 %.3fms,见关注线程自身行)", max)
	}
	if multi {
		return fmt.Sprintf(" (the focused thread itself waited/held a lock — single largest %.3fms; see its own state rows)", max)
	}
	return fmt.Sprintf(" (the focused thread itself waited/held a lock for %.3fms; see its own state rows)", max)
}

// runtimeTraceProjLeadSelectionValue is the rank-fallback ordering key for the
// conclusion line: the single-instance effective attribution. A ×N aggregate
// contributes its per-instance max — the merged SUM is a window-projection
// total across instances and must never compete against single-instance hard
// facts (V1, customer revisit 2026-07-03).
func runtimeTraceProjLeadSelectionValue(node types.TraceCausalProjectionNode) float64 {
	if node.IsContextOnlyRow() {
		return 0
	}
	if node.PeriodicSource {
		// VS-1 (§7.8): a periodic source competes with its DISCOUNTED
		// attribution only, even when it is exactly 0 (pure in-period cadence)
		// — the raw display impact would re-admit the cadence sleep the
		// discount exists to keep out of the conclusion.
		return node.EffectiveImpactMS
	}
	if runtimeTraceProjFamilyRow(node) {
		// RCM-2 D3 (§24.12 维度A 施工图强制项 ①, 2026-07-08): a family row
		// competes with its PUBLISHED combined participation value (合并量
		// 参赛 — same-thread totals are legal under the family caliber
		// ladder). This typed lane sits ABOVE the Merged* member-MAX discount
		// arm below and must NEVER fall through to it: that arm exists for
		// cross-thread ×N folds (墙钟跨线程不可加和) and would collapse the
		// same-thread family total back to its largest member, killing the
		// §24.10 合计参赛 ruling on arrival (pinned negative:
		// TestRCM2LeadSelectionValueNeverTakesMergedDiscountLane).
		return runtimeTraceProjFamilyPublishedMS(node)
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

// runtimeTraceProjConclusionMagnitude is the PTV5 C21/C22 headline magnitude
// (#68 用户裁定 2026-07-05): value + its (a)-table caliber word + the periodic
// flag + whether the value is a WINDOW-PROJECTION (复核 Med 2026-07-06: only
// that source may publish a 占窗 share — the C00 同源门). A periodic source's
// value routes through the F2 single-source override
// (runtimeTraceProjPeriodicHeadlineMS — authoritative at exactly 0); otherwise
// the chain cumulative leads, then the display-impact fallback chain names
// its own source. Word "" only when no magnitude exists at all.
func runtimeTraceProjConclusionMagnitude(primary types.TraceCausalProjectionNode, zh bool) (float64, string, bool, bool) {
	if primary.PeriodicSource {
		word := "有效归因"
		if !zh {
			word = "attribution"
		}
		return runtimeTraceProjPeriodicHeadlineMS(primary, 0), word, true, false
	}
	// 审计 #5/#62 (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): an
	// on-chain semantic lead whose participation is the exact member∩chain
	// intersection must not headline the complete union under the 链上累计
	// word (the union is the §24.10 lossless observation, NOT the on-chain
	// participation) — the headline publishes the engine's effective under
	// the (a)-table 有效归因 vocabulary; full-overlap leads (intersection ==
	// union) keep the legacy word byte-identically.
	if intersection, dual := runtimeTraceProjSemanticChainDualCaliber(primary); dual {
		v := intersection
		if primary.EffectiveImpactMS > 0 {
			// The engine-published effective wins; it equals the typed
			// intersection on every genuine fold.
			v = primary.EffectiveImpactMS
		}
		word := "有效归因"
		if !zh {
			word = "attribution"
		}
		return v, word, false, false
	}
	// GATED-CAL 件2 (§29.104.16.1 M4, 2026-07-16). EVOLUTION RECORD: a
	// SELF-basis or SEMANTIC-class primary used to headline its cumulative
	// under the cross-thread drill vocabulary 链上累计 — but on these seats
	// the cumulative is a SAME-THREAD family/self total (the witness: 8
	// segments of the target's own class_verification work headlined as
	// 「链上累计 9.586ms」), and the chain word drove the model to fabricate a
	// cross-thread credential host (cust_span_vs_prio). Typed causality/self-
	// basis precise gate: these seats headline the engine-published effective
	// under the (a)-table 有效归因 word (the family caliber word rides the
	// conclusion line as the 行3-form equation suffix — see
	// runtimeTraceProjConclusionFamilyCaliberSuffix at the caller). TRUE
	// drill-down chain seats keep 链上累计 byte-identically, and an
	// effective-less self/semantic seat falls through to the display-impact
	// fallback chain whose words name their own source (拒渲绝不造数 — never
	// a fabricated attribution claim).
	if runtimeTraceProjConclusionSelfSemanticSeat(primary) {
		if primary.EffectiveImpactMS > 0 {
			word := "有效归因"
			if !zh {
				word = "attribution"
			}
			return primary.EffectiveImpactMS, word, false, false
		}
		// Effective-less self/semantic seat: fall PAST the 链上累计 arm (the
		// word stays reserved for true drill-down chain seats — 只留真下钻链席)
		// into the display-impact fallback chain below, whose words name their
		// own source.
	} else if primary.CumulativeImpactMS > 0 {
		word := "链上累计"
		if !zh {
			word = "chain total"
		}
		return primary.CumulativeImpactMS, word, false, false
	}
	v, source := runtimeTraceProjNodeDisplayImpactSource(primary)
	if source == runtimeTraceProjImpactSourceWindow {
		word := "窗口投影"
		if !zh {
			word = "window projection"
		}
		return v, word, false, true
	}
	return v, runtimeTraceProjImpactCaliberWord(source, zh), false, false
}

// runtimeTraceProjConclusionSelfSemanticSeat (GATED-CAL 件2, §29.104.16.1 M4,
// 2026-07-16) is the typed causality/self-basis gate for the headline word
// fork: the primary is the target's OWN seat — a semantic-class family (the
// engine mints SemanticClass exclusively on trace_semantic_span records) or a
// self-basis row (node.OnChainBasis, the Stage-1 single field — the same
// closed set the 自身· qualifier words key on). Everything else is a genuine
// drill-down chain seat and keeps the 链上累计 vocabulary byte-identically.
func runtimeTraceProjConclusionSelfSemanticSeat(node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.SemanticClass) != "" {
		return true
	}
	switch strings.TrimSpace(node.OnChainBasis) {
	case "self_deterministic_span", "self_wall_clock_interval":
		return true
	}
	return false
}

// runtimeTraceProjConclusionFamilyCaliberSuffix (GATED-CAL 件2) renders the
// headline's family caliber suffix 「 = 合计(共N段,同线程)」 — the 行3
// equation grammar verbatim (word + raw-Σ disclosure from the same
// single-source composers), appended right after the 有效归因 magnitude so
// the same-thread family accounting is named AT the point of reading. Gates
// (all precise): the 件2 seat fork fired (same predicate), the row is a
// family fold with a known caliber word, the published family value IS the
// headlined effective at print precision (the "=" identity — 拒渲绝不造数),
// and the value is NOT the inversion machinery's gated product (件1② twin:
// the 合计 word must never mislabel a gated composite). "" everywhere else —
// the headline then carries the plain 有效归因 word alone.
func runtimeTraceProjConclusionFamilyCaliberSuffix(primary types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceProjConclusionSelfSemanticSeat(primary) || primary.EffectiveImpactMS <= 0 ||
		primary.PeriodicSource || !runtimeTraceProjFamilyRow(primary) {
		return ""
	}
	if runtimeTraceProjFamilyValueIsGatedComposite(primary) {
		return ""
	}
	if _, dual := runtimeTraceProjSemanticChainDualCaliber(primary); dual {
		// The dual-caliber lead keeps its own 链上计入 wording lane (审计 #62 ①).
		return ""
	}
	word, _, ok := runtimeTraceProjFamilyCaliberWord(primary, zh)
	if !ok || !runtimeTraceProjRound3Equal(runtimeTraceProjFamilyPublishedMS(primary), primary.EffectiveImpactMS) {
		return ""
	}
	return " = " + word + runtimeTraceProjFamilySumDisclosure(primary, zh)
}

// runtimeTraceProjCoverageVerdict is the single typed arithmetic decision
// shared by the window-line renderer and the model-prose overclaim guard.
// Comparable means the existing coverage lanes proved one denominator on the
// same basis as AttributedMS. It is deliberately false for cross-base,
// denominator-census collapse, beyond-jitter overshoot, and missing-window /
// missing-attribution shapes. No prose or formatted percentage participates.
type runtimeTraceProjCoverageVerdict struct {
	HasData       bool
	Comparable    bool
	AttributedMS  float64
	DenominatorMS float64
	SymptomMS     float64

	HopResidueCount int
	HopResidueMaxMS float64
	CrossBase       bool
	CensusExcluded  int
	CensusMaxMS     float64
	CensusAllOff    bool

	ChainWindowStart    float64
	ChainWindowEnd      float64
	ChainWindowMS       float64
	ChainWindowMismatch bool
	HopOvershootSleepMS float64
}

// LowCoverage reports the precise <=20% condition used to weaken only
// model-authored whole-frame root-cause prose. Multiplication avoids a
// separately rounded percentage: the full-precision typed values decide.
func (v runtimeTraceProjCoverageVerdict) LowCoverage() bool {
	return v.Comparable && v.DenominatorMS > 0 && v.AttributedMS >= 0 &&
		v.AttributedMS*5 <= v.DenominatorMS
}

func runtimeTraceProjCoverageVerdictFor(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) runtimeTraceProjCoverageVerdict {
	verdict := runtimeTraceProjCoverageVerdict{
		AttributedMS: runtimeTraceProjDepth1Cumulative(model),
	}
	verdict.HasData = verdict.AttributedMS > 0 || runtimeTraceProjChainHasPeriodicData(model)
	if model.WindowMS <= 0 || !verdict.HasData {
		return verdict
	}

	verdict.SymptomMS, _, verdict.HopResidueCount, verdict.HopResidueMaxMS =
		runtimeTraceProjTargetSymptomAdmission(model)
	_, _, _, verdict.CrossBase = runtimeTraceProjCoverageWindowConsensus(model)
	verdict.CensusExcluded, verdict.CensusMaxMS, verdict.CensusAllOff =
		runtimeTraceProjSymptomDenominatorCensus(projection, model)

	if verdict.SymptomMS > 0 {
		// These are the exact first two non-arithmetic arms in
		// runtimeTraceProjWindowLine: a positively mixed window base, or a
		// denominator population proven to have collapsed.
		if verdict.CrossBase ||
			(verdict.CensusExcluded > 0 && verdict.CensusMaxMS > verdict.SymptomMS) {
			return verdict
		}
		if verdict.AttributedMS <= verdict.SymptomMS ||
			verdict.AttributedMS-verdict.SymptomMS <= runtimeTraceProjSymptomOvershootJitterMS {
			verdict.Comparable = true
			verdict.DenominatorMS = verdict.SymptomMS
		}
		// Beyond-jitter symptom overshoot is intentionally incomparable.
		return verdict
	}

	// The renderer's whole-window arithmetic is reachable only when the
	// attribution itself does not overrun the analysis window. A positive
	// mixed-base signal is never safe for the prose guard, even though legacy
	// renderer wording may fail open when it cannot name a single source
	// window.
	if verdict.AttributedMS > model.WindowMS || verdict.CrossBase {
		return verdict
	}
	if ws, we, ok := runtimeTraceProjChainDataQueryWindow(model); ok &&
		runtimeTraceProjCoverageWindowBaseMismatch(projection, ws, we) {
		verdict.ChainWindowStart = ws
		verdict.ChainWindowEnd = we
		verdict.ChainWindowMS = (we - ws) * 1000
		verdict.ChainWindowMismatch = true
		if verdict.ChainWindowMS > 0 && verdict.AttributedMS <= verdict.ChainWindowMS {
			verdict.Comparable = true
			verdict.DenominatorMS = verdict.ChainWindowMS
		}
		return verdict
	}

	verdict.HopOvershootSleepMS = runtimeTraceProjHopOnlyOvershootSleepMS(model, verdict.AttributedMS)
	if verdict.HopOvershootSleepMS > 0 && verdict.AttributedMS > verdict.HopOvershootSleepMS {
		if verdict.AttributedMS-verdict.HopOvershootSleepMS > runtimeTraceProjSymptomOvershootJitterMS {
			return verdict
		}
		// The jitter arm reports the attributed share of the analysis window.
	}
	verdict.Comparable = true
	verdict.DenominatorMS = model.WindowMS
	return verdict
}

// runtimeTraceProjFourStateAccountLines renders the §29.27② four-state
// coverage account (COV-4 用户裁定 2026-07-11): the focused thread's
// FULL-WINDOW wall-clock partition (running + runnable + sleep + D-state;
// io_wait = the typed IO attribution label INSIDE the D-state wall clock,
// never a fifth addend) plus the running-segment attribution line. nil (no
// lines — every legacy coverage arm stays byte-identical) unless ALL the
// precise admission gates hold:
//   - the compile attached a typed TargetStateAccount whose canonical subject
//     IS the rendered 🎯 target;
//   - the account's typed window matches the analysis window (F-2 ±1ms per
//     endpoint — defense in depth over the compile-side admission);
//   - Σ(five state lanes) == analysis window within the shared jitter
//     tolerance (Σ四态==窗口恒等式 — 不平衡拒渲不造数: an incomplete head
//     carry-in / unobserved prefix / stopped-dead gap makes the partition
//     unprovable and the account honestly refuses).
//
// Percentages are WALL-CLOCK over the window denominator only. The
// supply-converted pointer publishes as 「见 ❸[E#]」 with the 折算 caliber and
// NEVER joins the wall-clock arithmetic (§7.30 S1 负面先例: 排序合成分数以 ms
// 硬事实发布→四态和 119% — 禁折算值进墙钟百分比). The running residual wears
// the ruling-verbatim word 自身执行(无确定性可优化工作), never 未归因.
// runtimeTraceProjFourStateAccountProvable is the SINGLE admission gate of
// the §29.27 four-state account (extracted for the §29.30 self-cause crown —
// the crown's Tier-A decomposition consumes the SAME gates, never a second
// implementation): typed account present, subject == the focused target,
// account window == projection window (F-2 准入禁猜), and the Σ==窗 identity
// at DISPLAY precision (复核 B-1: the printed %.3f faces of Σ and the window
// must be equal — 不平衡拒渲不造数). nil when any gate refuses.
func runtimeTraceProjFourStateAccountProvable(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) *types.TraceCausalProjectionTargetStateAccount {
	account := projection.TargetStateAccount
	if account == nil || model.WindowMS <= 0 {
		return nil
	}
	target := runtimeTraceCausalProjectionCanonicalNode(model.Target)
	if target == "" || runtimeTraceCausalProjectionCanonicalNode(account.Subject) != target {
		return nil
	}
	if math.Abs(account.WindowStartTs-projection.WindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
		math.Abs(account.WindowEndTs-projection.WindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS {
		return nil
	}
	sum := account.RunningMS + account.RunnableMS + account.SleepMS + account.DStateMS + account.IOWaitMS
	// 复核 B-1 (2026-07-11). EVOLUTION RECORD: the first cut borrowed the
	// 0.5ms symptom-overshoot jitter constant here — 200× looser than the
	// claim the line prints, so a 0.1ms unobserved gap could render
	// "= 151.100ms(四态合计=分析窗)" beside a 151.382ms header (visible
	// self-contradiction) while masking a real coverage hole. The identity
	// gate is now the DISPLAY-PRECISION claim itself: the printed %.3f faces
	// of Σ and the window must be equal (≤0.0005ms true divergence). Beyond
	// it → refuse, fall back to the legacy coverage arms (不平衡拒渲不造数).
	if fmt.Sprintf("%.3f", sum) != fmt.Sprintf("%.3f", model.WindowMS) {
		return nil // Σ四态 ≠ 窗口(显示精度): the partition is unprovable.
	}
	return account
}

func runtimeTraceProjFourStateAccountLines(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) []string {
	account := runtimeTraceProjFourStateAccountProvable(projection, model)
	if account == nil {
		return nil
	}
	sum := account.RunningMS + account.RunnableMS + account.SleepMS + account.DStateMS + account.IOWaitMS
	model.Marks.mark(runtimeTraceProjMarkFourStateAccount)
	pct := func(v float64) float64 { return v / model.WindowMS * 100 }
	dState := account.DStateMS + account.IOWaitMS
	// The IO attribution label renders INSIDE its state term (never a fifth
	// addend): the D term carries the D-opened io_wait carve-out; the sleep
	// term carries the 复核 A-1 S+iowait refinement (G12 §29.13 Harmony
	// platform form — the dominant IO-wait shape on this campaign's traces).
	// Defensive precision guard: a refinement claiming more than its own
	// lane is unprovable and renders no label (拒标不造数).
	ioLabel := func(v, lane float64) string {
		if v <= 0 || v > lane+0.0005 {
			return ""
		}
		if zh {
			return fmt.Sprintf(",其中 IO等待 %.3fms", v)
		}
		return fmt.Sprintf(", incl. IO wait %.3fms", v)
	}
	sleepIOClause := ioLabel(account.SleepIOWaitMS, account.SleepMS)
	dIOClause := ioLabel(account.IOWaitMS, dState)
	runningFoldClause := runtimeTraceProjFourStateBoundaryFoldClause(account, account.RunningMS, zh, "running")
	runnableFoldClause := runtimeTraceProjFourStateBoundaryFoldClause(account, account.RunnableMS, zh, "runnable")
	sleepFoldClause := runtimeTraceProjFourStateBoundaryFoldClause(account, account.SleepMS, zh, "sleep")
	dFoldClause := runtimeTraceProjFourStateBoundaryFoldClause(account, dState, zh, "d_state", "io_wait")
	var lines []string
	if zh {
		lines = append(lines, fmt.Sprintf("- 关注线程全窗四态: running %.3fms(%.0f%%%s) + runnable %.3fms(%.0f%%%s) + sleep %.3fms(%.0f%%%s%s) + D-state %.3fms(%.0f%%%s%s) = %.3fms(四态合计=分析窗)。",
			account.RunningMS, pct(account.RunningMS), runningFoldClause, account.RunnableMS, pct(account.RunnableMS), runnableFoldClause,
			account.SleepMS, pct(account.SleepMS), sleepIOClause, sleepFoldClause, dState, pct(dState), dIOClause, dFoldClause, sum))
	} else {
		lines = append(lines, fmt.Sprintf("- Focused-thread full-window states: running %.3fms (%.0f%%%s) + runnable %.3fms (%.0f%%%s) + sleep %.3fms (%.0f%%%s%s) + D-state %.3fms (%.0f%%%s%s) = %.3fms (four-state total = analysis window).",
			account.RunningMS, pct(account.RunningMS), runningFoldClause, account.RunnableMS, pct(account.RunnableMS), runnableFoldClause,
			account.SleepMS, pct(account.SleepMS), sleepIOClause, sleepFoldClause, dState, pct(dState), dIOClause, dFoldClause, sum))
	}
	if line := runtimeTraceProjFourStateRunningLine(account, model, zh); line != "" {
		lines = append(lines, line)
	}
	return lines
}

// runtimeTraceProjFourStateBoundaryFoldClause (§29.140 G6, ANSWERFACE-1 件2
// 词面单点) renders the in-term 「,含未覆盖段 X.XXXms 折入」 disclosure for one
// four-state partition term. It sums the typed head-carry (window-head prefix
// carried from the recovered pre-window scheduler state) and tail-open
// (window-tail suffix flushed from the final open interval, no in-window
// closing event) components whose published lane belongs to the term. The
// value is ALREADY inside the term (disclosure only, never an addend); a fold
// claiming more than its own term is unprovable and renders nothing
// (拒标不造数 — same guard family as the IO attribution label).
func runtimeTraceProjFourStateBoundaryFoldClause(account *types.TraceCausalProjectionTargetStateAccount, termMS float64, zh bool, lanes ...string) string {
	if account == nil {
		return ""
	}
	var v float64
	for _, lane := range lanes {
		if lane == "" {
			continue
		}
		if account.HeadCarryState == lane {
			v += account.HeadCarryMS
		}
		if account.TailOpenState == lane {
			v += account.TailOpenMS
		}
	}
	if v <= 0 || v > termMS+0.0005 {
		return ""
	}
	if zh {
		return fmt.Sprintf(",含未覆盖段 %.3fms 折入", v)
	}
	return fmt.Sprintf(", incl. uncovered segment %.3fms folded in", v)
}

// runtimeTraceProjFourStateRunningLine renders the account's running-segment
// attribution line: 「running X: 确定性工作 A ❷[E#] · 供给折算影响 B 见 ❸[E#]
// (折算,不计入四态合计) · 自身执行(无确定性可优化工作) C」. All wall-clock
// components are same-thread same-partition and therefore subtract; the
// converted supply figure is a POINTER (对照口径), never an addend. "" when
// running is zero or the deterministic work overshoots running beyond the
// jitter tolerance (component unprovable — refuse the line, keep the
// partition line).
func runtimeTraceProjFourStateRunningLine(account *types.TraceCausalProjectionTargetStateAccount, model runtimeTraceProjTreeModel, zh bool) string {
	parts, ok := runtimeTraceProjFourStateRunningParts(account, model, zh)
	if !ok {
		return ""
	}
	if zh {
		return fmt.Sprintf("- running %.3fms: %s。", account.RunningMS, strings.Join(parts, " · "))
	}
	return fmt.Sprintf("- running %.3fms: %s.", account.RunningMS, strings.Join(parts, " · "))
}

// runtimeTraceProjFourStateRunningParts builds the running-segment
// attribution components (确定性工作 / 供给折算影响 / 自身执行) — the SINGLE
// morpheme+value source shared by the account's running line above and the
// §29.30 self-cause crown's Tier-A parenthetical (词素取 COV-4 覆盖账闭集,
// one authority — the crown can never speak values the account line would
// not). ok=false when running is zero or the deterministic component is
// unprovable (same refusal the line applied).
func runtimeTraceProjFourStateRunningParts(account *types.TraceCausalProjectionTargetStateAccount, model runtimeTraceProjTreeModel, zh bool) ([]string, bool) {
	if account.RunningMS <= 0 {
		return nil, false
	}
	deterministic := account.DeterministicRunningMS
	if deterministic > account.RunningMS+runtimeTraceProjSymptomOvershootJitterMS {
		return nil, false
	}
	if deterministic > account.RunningMS {
		deterministic = account.RunningMS // boundary jitter clamps to the partition
	}
	var parts []string
	if deterministic > 0 {
		tag, count, best := runtimeTraceProjFourStateSemanticPointer(model)
		switch {
		case tag == "":
			if zh {
				parts = append(parts, fmt.Sprintf("确定性工作 %.3fms", deterministic))
			} else {
				parts = append(parts, fmt.Sprintf("deterministic work %.3fms", deterministic))
			}
		case count == 1 && runtimeTraceProjRound3Equal(deterministic, best):
			// Exact single-family match: the pointer row IS the value.
			if zh {
				parts = append(parts, fmt.Sprintf("确定性工作 %.3fms%s", deterministic, tag))
			} else {
				parts = append(parts, fmt.Sprintf("deterministic work %.3fms%s", deterministic, tag))
			}
		default:
			// 复核 E-P3: the account value is the ∩running union across the
			// target's semantic families while the pointer names ONE row —
			// disclose the relation so X beside a Y-valued row never reads
			// as a contradiction (复核核定词形).
			if zh {
				parts = append(parts, fmt.Sprintf("确定性工作 %.3fms(共%d类,最大 %.3fms 见%s)", deterministic, count, best, tag))
			} else {
				parts = append(parts, fmt.Sprintf("deterministic work %.3fms (%d classes, largest %.3fms, see%s)", deterministic, count, best, tag))
			}
		}
	}
	if value, tag, ok := runtimeTraceProjFourStateSupplyPointer(model); ok {
		if zh {
			parts = append(parts, fmt.Sprintf("供给折算影响 %.3fms 见%s(折算,不计入四态合计)", value, tag))
		} else {
			parts = append(parts, fmt.Sprintf("supply-converted impact %.3fms, see%s (converted; not in the four-state total)", value, tag))
		}
	}
	residual := account.RunningMS - deterministic
	if residual < 0 {
		residual = 0
	}
	if zh {
		parts = append(parts, fmt.Sprintf("自身执行(无确定性可优化工作) %.3fms", residual))
	} else {
		parts = append(parts, fmt.Sprintf("own execution (no deterministic optimizable work) %.3fms", residual))
	}
	return parts, true
}

// runtimeTraceProjFourStateSemanticPointer resolves the 确定性工作 component's
// evidence pointer: the LARGEST rendered target-thread semantic row's §29.27.1
// seat badge + [E#] (三面记号一致 — the same glyph/tag pair the tree and the
// detail table wear), plus the rendered family COUNT and the pointer row's
// own published value (复核 E-P3: the running line discloses 共N类/最大 Y so
// the ∩running union beside a single row's value never reads contradictory).
// tag "" when no such row rendered (the value still publishes — the typed
// account is the source; the pointer is navigation only).
func runtimeTraceProjFourStateSemanticPointer(model runtimeTraceProjTreeModel) (string, int, float64) {
	target := runtimeTraceCausalProjectionCanonicalNode(model.Target)
	var best *runtimeTraceProjTreeRow
	bestValue := 0.0
	count := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			row := &rows[i]
			if row.Kind != runtimeTraceProjTreeRowSemantic || !row.HasData {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != target {
				continue
			}
			count++
			if value := runtimeTraceProjFamilyPublishedMS(row.Node); value > bestValue {
				bestValue, best = value, row
			}
		}
	}
	if best == nil || strings.TrimSpace(best.EvidenceTag) == "" {
		return "", 0, 0
	}
	return " " + runtimeTraceProjBadgeGlyph(best.Badge) + "[" + best.EvidenceTag + "]", count, bestValue
}

// runtimeTraceProjFourStateSupplyPointer resolves the 供给折算影响 pointer:
// the focused thread's OWN running row carrying the engine supply-fold
// deficit (typed SupplyFoldComputed + positive deficit). Returns the deficit
// (converted caliber — pointer only, never an addend) and the row's badge+E#
// tag. ok=false when no such row rendered (the account then makes NO
// converted claim — absence never guesses).
func runtimeTraceProjFourStateSupplyPointer(model runtimeTraceProjTreeModel) (float64, string, bool) {
	target := runtimeTraceCausalProjectionCanonicalNode(model.Target)
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			row := &rows[i]
			if !row.HasData || runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != target {
				continue
			}
			if !row.Node.SupplyFoldComputed || row.Node.SupplyFoldDeficitMS <= 0 {
				continue
			}
			if types.TraceCausalProjectionStateClass(row.Node.StateKind) != "" &&
				strings.TrimSpace(strings.ToLower(row.Node.StateKind)) != "running" {
				continue
			}
			tag := ""
			if strings.TrimSpace(row.EvidenceTag) != "" {
				tag = " " + runtimeTraceProjBadgeGlyph(row.Badge) + "[" + row.EvidenceTag + "]"
			}
			return row.Node.SupplyFoldDeficitMS, tag, true
		}
	}
	return 0, "", false
}

func runtimeTraceProjWindowLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	if model.WindowMS <= 0 {
		// DISPHYG-3 件3 (§29.155 P2 残形): the no-anchor shape forks to the
		// honest no-ruler sentence (shared clause single point); the anchored
		// mixed shape keeps its legacy wording with only the C8 prose-comma
		// conversion (件1).
		if !model.BarScaleWallClockAnchored {
			if zh {
				return "分析窗起止未采集: " + runtimeTraceProjNoRulerClause(model.BarMaxMS > 0, true) + "，不显示占窗百分比、不画时长条(系统不估算窗口)。"
			}
			return "Window bounds not captured: " + runtimeTraceProjNoRulerClause(model.BarMaxMS > 0, false) + " — no window percentages, no bars (the system never estimates a window)."
		}
		if zh {
			return "分析窗起止未采集: 不显示占窗百分比，树内时长条满格=本报告最大投影(回退尺度,系统不估算窗口)。"
		}
		return "Window bounds not captured: no window percentages; tree bars scale to the largest projection in this report (fallback scale — the system never estimates a window)."
	}
	var b strings.Builder
	if zh {
		// DISPHYG-3 件1 (§29.150⑩ C8 user ruling, 2026-07-20). EVOLUTION
		// RECORD: the C8 witness line 「…s,共 233.190ms。」 mixed a half-width
		// clause comma with the full-width 。 in ONE system-minted prose
		// sentence. Regime constitution (按区分制成文): system-minted
		// NON-BULLET prose sentences in the answer region (this window line
		// + the conclusion line) carry FULL-WIDTH top-level clause commas
		// (，) with 。; fence bodies (树/◎) stay half-width and never print
		// 。; bullet legend/account lines and section-leader sentences remain
		// the presentation block's established half-width regime;
		// parenthetical interiors and fence-shared word-face tokens (caliber
		// phrases, alias forms) keep their shared bytes on every surface
		// (词面单点 — converting one copy would fork the face).
		fmt.Fprintf(&b, "分析窗 %.3f~%.3fs，共 %.3fms。", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	} else {
		fmt.Fprintf(&b, "Analysis window %.3f~%.3fs, %.3fms total.", projection.WindowStartTs, projection.WindowEndTs, model.WindowMS)
	}
	// §29.27② (COV-4, 用户裁定 2026-07-11): the four-state coverage account —
	// ADDITIVE lines between the window header and the wait-attribution arms
	// (every legacy arm below stays byte-identical on every shape; the account
	// renders ONLY when the typed partition provably balances the window).
	for _, line := range runtimeTraceProjFourStateAccountLines(projection, model, zh) {
		b.WriteString("\n" + line)
	}
	// Coverage = depth-1 cumulative vs window, by SUBTRACTION only — chain
	// values overlap on the wall clock and must never be summed across layers.
	// VS-1 F5(a) (adversarial review 2026-07-04): a periodic chain row whose
	// attribution legitimately discounts to exactly 0 still yields a coverage
	// sentence — "on-chain 已归因 0.000ms" IS the finding (the wait was normal
	// cadence), not a missing-data state; the >0 short-circuit alone would
	// silently drop the line exactly when the discount did its job.
	coverage := runtimeTraceProjCoverageVerdictFor(projection, model)
	if attributed := coverage.AttributedMS; coverage.HasData {
		// PTV5 Q2 (#68 用户裁定 2026-07-05): the coverage sentence's caliber has
		// its own dynamic-legend entry — marked exactly when the sentence
		// renders (the lead builds the legend after this line runs).
		model.Marks.mark(runtimeTraceProjMarkCoverageLine)
		// V2 (customer revisit 2026-07-03): when the 🎯 target published its own
		// state rows, the coverage denominator is the TARGET SYMPTOM duration,
		// not the whole window — a target that slept 11.7ms of a 101ms window
		// once rendered as "残差 97%". Falls back to the whole window (wording
		// unchanged) ONLY when no self-state symptom row exists; attribution
		// exceeding the symptom keeps the symptom denominator (§15.D gap③
		// below — the old whole-window demotion fabricated residual).
		// RN-6 (§7.9): the denominator family includes runnable, so the wording
		// says 等待(sleep/D-state/runnable) instead of claiming everything was
		// sleep/blocked. PTV7 (#74): the parenthetical state words speak the
		// canonical tokens on both faces; the sentence frame stays localized.
		// §15.D gap③ (P0-A1 显示批, q6/q8 witness): once the target published a
		// symptom denominator, attribution overshooting it must NEVER silently
		// demote the denominator to the whole window — q6's 112.223ms lock span
		// sat 0.048ms past the target's 112.175ms sleep and the whole-window
		// recast rendered "94% attributed + 6.838ms unattributed residual"
		// against the 119.061ms window: a fabricated residual for a fully
		// explained wait. Two overshoot regimes on one numeric fork (wording
		// only — both sides publish the same two magnitudes, no hard gate):
		//   - within runtimeTraceProjSymptomOvershootJitterMS: state-boundary
		//     jitter; the F1 nesting premise holds to edge-timestamp
		//     granularity → full symptom coverage, raw caliber total disclosed
		//     verbatim (min-clamp precedent: runtimeTraceProjResidualOwnCaliberNote);
		//   - beyond it: the nesting premise itself is broken (the chain wall
		//     clock provably extends past the symptom segments, so per-ms
		//     containment is unverified) → both magnitudes, no percentage, no
		//     residual — and still never a whole-window recast.
		symptom := coverage.SymptomMS
		hopResidueCount, hopResidueMaxMS := coverage.HopResidueCount, coverage.HopResidueMaxMS
		// §21.1 CWD-2 ③ (CWD 复核留账②, symptom 分母车道): the three symptom-
		// denominator arms below divide (or subtract) the chain-cumulative
		// numerator against the target's own state-row sum — two DIFFERENT
		// row populations. When the coverage-feeding rows carry POSITIVELY
		// disagreeing typed query windows (the consensus core's conflict
		// signal — never mere absence), the numerator and denominator window
		// bases cannot be proven identical and the percentage/residual
		// arithmetic must not render: both magnitudes publish with the base
		// disclosure instead (same CWD gate family as the whole-window branch
		// below; windowless and agreeing-window shapes keep every legacy
		// wording byte-identically).
		crossBase := coverage.CrossBase
		// §24.11 C-3 census (COV 批 + coordinator supplement cmp_78_01 7.0 侧,
		// 2026-07-08): the denominator-population census runs on EVERY
		// symptom-denominator arm — the collapse is a Role/StateKind-lane
		// exclusion, not only a cross-window one (cmp_78_01: the parenthetical
		// claimed "(sleep/D-state/runnable)" while a 456.725ms sleep hop view
		// and a binder wait were silently excluded and only two tiny runnable
		// rows fed the 3.262ms denominator).
		censusExcluded, censusMax, censusAllOffWindow := coverage.CensusExcluded, coverage.CensusMaxMS, coverage.CensusAllOff
		switch {
		case symptom > 0 && crossBase:
			// §24.11 C-3 (COV 批, huadong_78 witness, 2026-07-08): when the
			// symptom denominator's population is not the target's full wait —
			// typed census: ≥1 of the target's own wait-view rows stayed out of
			// the denominator, i.e. 入分母行数 < 目标状态行总数 — the sentence
			// switches FORM instead of letting the counted slice (0.011ms)
			// masquerade as the 全称 "关注线程等待". Wording fork only, both
			// magnitudes still publish; a census of zero exclusions keeps the
			// legacy crossBase wording. The "在其他查询窗" clause renders only
			// when EVERY excluded row provably carries an off-window identity.
			// The form drops the "(sleep/D-state/runnable)" family claim: a
			// parenthetical must never claim a state family the denominator
			// silently excluded (cmp_78_01 7.0 侧, coordinator supplement).
			if excluded, excludedMax, allOffWindow := censusExcluded, censusMax, censusAllOffWindow; excluded > 0 {
				switch {
				case zh && allOffWindow:
					fmt.Fprintf(&b, "\n- 仅计入分析窗内直接等待 %.3fms;另有 %d 条关注线程状态行在其他查询窗,未计入分母(单项最大 %.3fms);链上单项最大 %.3fms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。",
						symptom, excluded, excludedMax, attributed)
				case zh:
					fmt.Fprintf(&b, "\n- 仅计入分析窗内直接等待 %.3fms;另有 %d 条关注线程状态行未计入分母(单项最大 %.3fms);链上单项最大 %.3fms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。",
						symptom, excluded, excludedMax, attributed)
				case allOffWindow:
					fmt.Fprintf(&b, "\n- Only the direct wait inside the analysis window, %.3fms, is counted; %d more focused-thread state row(s) live in other query windows and are not in the denominator (single largest %.3fms); largest single on-chain caliber %.3fms — the chain/self data spans multiple query windows and the numerator/denominator window bases cannot be proven identical: no coverage percentage, no unattributed residual.",
						symptom, excluded, excludedMax, attributed)
				default:
					fmt.Fprintf(&b, "\n- Only the direct wait inside the analysis window, %.3fms, is counted; %d more focused-thread state row(s) are not in the denominator (single largest %.3fms); largest single on-chain caliber %.3fms — the chain/self data spans multiple query windows and the numerator/denominator window bases cannot be proven identical: no coverage percentage, no unattributed residual.",
						symptom, excluded, excludedMax, attributed)
				}
			} else if zh {
				fmt.Fprintf(&b, "\n- 关注线程等待(sleep/D-state/runnable) %.3fms;链上单项最大 %.3fms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。",
					symptom, attributed)
			} else {
				fmt.Fprintf(&b, "\n- Focused-thread wait (sleep/D-state/runnable) %.3fms; the largest single on-chain caliber is %.3fms — the chain/self data spans multiple query windows and the numerator/denominator window bases cannot be proven identical: no coverage percentage, no unattributed residual.",
					symptom, attributed)
			}
		case symptom > 0 && censusExcluded > 0 && censusMax > symptom:
			// §24.11 C-3 + cmp_78_01 7.0 侧 (coordinator supplement,
			// 2026-07-08): Role/StateKind-lane denominator collapse WITHOUT a
			// provable window conflict — an excluded wait-view row's
			// single-instance magnitude EXCEEDS the admitted denominator, so
			// the F1 nesting premise is numerically impossible (a nested hop
			// view can never exceed its enclosing state sum) and the admitted
			// slice provably under-represents the target's wait. The 全称
			// "(sleep/D-state/runnable)" claim and every %/residual arithmetic
			// against the collapsed denominator are suppressed; both
			// magnitudes still publish. Healthy nested shapes (excluded ≤
			// admitted) keep every legacy arm byte-identically — this is a
			// wording fork on precise numeric comparisons, never a gate.
			switch {
			case zh && censusAllOffWindow:
				fmt.Fprintf(&b, "\n- 仅计入分析窗内直接等待 %.3fms;另有 %d 条关注线程状态行在其他查询窗,未计入分母(单项最大 %.3fms);链上单项最大 %.3fms — 分母未覆盖关注线程全部状态行,不给出覆盖百分比,不计未归因。",
					symptom, censusExcluded, censusMax, attributed)
			case zh:
				fmt.Fprintf(&b, "\n- 仅计入分析窗内直接等待 %.3fms;另有 %d 条关注线程状态行未计入分母(单项最大 %.3fms);链上单项最大 %.3fms — 分母未覆盖关注线程全部状态行,不给出覆盖百分比,不计未归因。",
					symptom, censusExcluded, censusMax, attributed)
			case censusAllOffWindow:
				fmt.Fprintf(&b, "\n- Only the direct wait inside the analysis window, %.3fms, is counted; %d more focused-thread state row(s) live in other query windows and are not in the denominator (single largest %.3fms); largest single on-chain caliber %.3fms — the denominator does not cover all focused-thread state rows: no coverage percentage, no unattributed residual.",
					symptom, censusExcluded, censusMax, attributed)
			default:
				fmt.Fprintf(&b, "\n- Only the direct wait inside the analysis window, %.3fms, is counted; %d more focused-thread state row(s) are not in the denominator (single largest %.3fms); largest single on-chain caliber %.3fms — the denominator does not cover all focused-thread state rows: no coverage percentage, no unattributed residual.",
					symptom, censusExcluded, censusMax, attributed)
			}
		case symptom > 0 && attributed <= symptom:
			residual := symptom - attributed
			if zh {
				fmt.Fprintf(&b, "\n- 关注线程等待(sleep/D-state/runnable) %.3fms 中链上已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)。",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			} else {
				fmt.Fprintf(&b, "\n- Of the focused thread's %.3fms wait time (sleep/D-state/runnable), on-chain attributed %.3fms (%.0f%%), unattributed %.3fms (%.0f%%).",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			}
			b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
			b.WriteString(runtimeTraceProjPeriodicCadenceCoverageNote(model, residual, zh))
			b.WriteString(runtimeTraceProjHopAdmissionResidueNote(hopResidueCount, hopResidueMaxMS, zh))
		case symptom > 0 && attributed-symptom <= runtimeTraceProjSymptomOvershootJitterMS:
			// EVOLUTION RECORD (§24.11 C-3 后半场, COV 批, 2026-07-08): the
			// numerator wording 各链上口径合计 → 链上单项最大 on every arm —
			// runtimeTraceProjDepth1Cumulative is a single-row MAX (墙钟红线:
			// 不可求和), and "合计" claimed a sum it never was (huadong_78:
			// "各链上口径合计 4.431ms" = the single E15 row). 名实对齐:措辞随
			// 取值,取值不动.
			if zh {
				fmt.Fprintf(&b, "\n- 关注线程等待(sleep/D-state/runnable) %.3fms 中链上已归因 %.3fms(100%%),未归因 0.000ms(0%%);链上单项最大 %.3fms,略超关注线程等待 %.3fms(状态段边界抖动;不计未归因)。",
					symptom, symptom, attributed, attributed-symptom)
			} else {
				fmt.Fprintf(&b, "\n- Of the focused thread's %.3fms wait time (sleep/D-state/runnable), on-chain attributed %.3fms (100%%), unattributed 0.000ms (0%%). The largest single on-chain caliber is %.3fms, %.3fms past the focused-thread wait (state-boundary jitter; not unattributed residual).",
					symptom, symptom, attributed, attributed-symptom)
			}
			b.WriteString(runtimeTraceProjHopAdmissionResidueNote(hopResidueCount, hopResidueMaxMS, zh))
		case symptom > 0:
			if zh {
				fmt.Fprintf(&b, "\n- 关注线程等待(sleep/D-state/runnable) %.3fms;链上单项最大 %.3fms,超出关注线程等待 %.3fms — 两口径墙钟未对齐,不给出覆盖百分比,差值不计为未归因。",
					symptom, attributed, attributed-symptom)
			} else {
				fmt.Fprintf(&b, "\n- Focused-thread wait (sleep/D-state/runnable) %.3fms; the largest single on-chain caliber is %.3fms, %.3fms beyond the focused-thread wait — the two calibers' wall clocks do not align: no coverage percentage, and the difference is not unattributed residual.",
					symptom, attributed, attributed-symptom)
			}
			if attributed > model.WindowMS {
				if zh {
					b.WriteString("其实际状态跨出窗口,见 ⚠ 标记。")
				} else {
					b.WriteString(" The underlying state crosses the window; see ⚠ marks.")
				}
			}
		case attributed <= model.WindowMS:
			// §21 CWD (cmp_01 revisit 2026-07-07, repeat-P0 覆盖句窗基错配):
			// when every windowed chain-data row shares ONE query window that
			// is NOT the anchor window, the whole-window division below would
			// divide a cross-window numerator by the anchor-window denominator
			// — the specimen printed "on-chain 已归因 94.466ms/94%,未归因残差
			// 6.534ms/6%" against a 101ms anchor window whose chain the
			// numerator never measured, fabricating the residual. Same-base
			// rendering instead: the denominator and its label switch to the
			// chain-data window, explicitly named. Precise signals only (typed
			// per-row selected_window identity vs the typed anchor endpoints);
			// windowless or mixed-window chains keep the legacy rendering
			// byte-identically (fail-open).
			if ws, we := coverage.ChainWindowStart, coverage.ChainWindowEnd; coverage.ChainWindowMismatch {
				chainWinMS := coverage.ChainWindowMS
				if attributed <= chainWinMS {
					residual := chainWinMS - attributed
					if zh {
						fmt.Fprintf(&b, "\n- 链上已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)(口径:链上数据来自查询窗 %.3f~%.3fs 共 %.3fms,分母取该查询窗,非上句分析窗;两窗基不可混除)。",
							attributed, attributed/chainWinMS*100, residual, residual/chainWinMS*100, ws, we, chainWinMS)
					} else {
						fmt.Fprintf(&b, "\n- On-chain attributed %.3fms/%.0f%%, unattributed %.3fms/%.0f%% (caliber: the chain data comes from query window %.3f~%.3fs, %.3fms total — the denominator is that window, not the analysis window above; the two window bases never divide across).",
							attributed, attributed/chainWinMS*100, residual, residual/chainWinMS*100, ws, we, chainWinMS)
					}
					b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
					b.WriteString(runtimeTraceProjPeriodicCadenceCoverageNote(model, residual, zh))
				} else {
					// Numerator exceeds even its own window — no percentage,
					// no residual (魔术数不出厂), both magnitudes disclosed.
					if zh {
						fmt.Fprintf(&b, "\n- 链上已归因 %.3fms(链上数据来自查询窗 %.3f~%.3fs,非上句分析窗;窗基不同,不给出覆盖百分比,不计未归因)。",
							attributed, ws, we)
					} else {
						fmt.Fprintf(&b, "\n- On-chain attributed %.3fms (the chain data comes from query window %.3f~%.3fs, not the analysis window above; different window bases — no coverage percentage, no unattributed residual).",
							attributed, ws, we)
					}
				}
			} else if hopSleep := coverage.HopOvershootSleepMS; hopSleep > 0 &&
				attributed > hopSleep && attributed-hopSleep <= runtimeTraceProjSymptomOvershootJitterMS {
				// PTV8-RCR-A A5 (§15.D 同款边界抖动臂, hop 车道; UXA 域A 条4
				// 终稿文案, opendir_02 witness): the target's hop-view sleep is
				// FULLY explained on-chain (attribution overshoots it by pure
				// state-boundary jitter) — the whole-window division above
				// would fabricate a "未归因残差 7.777ms/6%" for a fully
				// explained wait. Wording fork only; both magnitudes publish.
				if zh {
					fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms 已全部由链上解释(链上单项最大 %.3fms,略超 %.3fms,属状态段边界抖动);占分析窗 %.0f%%。",
						hopSleep, attributed, attributed-hopSleep, attributed/model.WindowMS*100)
				} else {
					fmt.Fprintf(&b, "\n- The focused thread's %.3fms sleep is fully explained on-chain (the largest single on-chain caliber is %.3fms, %.3fms past it — state-boundary jitter); %.0f%% of the analysis window.",
						hopSleep, attributed, attributed-hopSleep, attributed/model.WindowMS*100)
				}
			} else if hopSleep > 0 && attributed > hopSleep {
				// A5 beyond-jitter arm (禁猜红线): the nesting premise is
				// unverified — both magnitudes, no percentage, no residual,
				// never a whole-window recast.
				if zh {
					fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms;链上单项最大 %.3fms,超出关注线程睡眠 %.3fms — 两口径墙钟未对齐,不给出覆盖百分比,差值不计为未归因。",
						hopSleep, attributed, attributed-hopSleep)
				} else {
					fmt.Fprintf(&b, "\n- Focused-thread sleep %.3fms; the largest single on-chain caliber is %.3fms, %.3fms beyond it — the two calibers' wall clocks do not align: no coverage percentage, and the difference is not unattributed residual.",
						hopSleep, attributed, attributed-hopSleep)
				}
			} else {
				residual := model.WindowMS - attributed
				if zh {
					fmt.Fprintf(&b, "\n- 链上已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)。",
						attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
				} else {
					fmt.Fprintf(&b, "\n- On-chain attributed %.3fms/%.0f%%, unattributed residual %.3fms/%.0f%%.",
						attributed, attributed/model.WindowMS*100, residual, residual/model.WindowMS*100)
				}
				b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
				b.WriteString(runtimeTraceProjPeriodicCadenceCoverageNote(model, residual, zh))
			}
			// PTV5 Q2 (#68 用户裁定 2026-07-05): hop-only 形态 — the target
			// published NO state-view symptom row (the F1 denominator stays
			// whole-window, arithmetic untouched) but its sleep-family hop-view
			// self rows re-describe the blocked wall clock; the info line
			// relates the two existing magnitudes so the whole-window share is
			// not the only reading. attributed > sleep now rides the A5
			// overshoot arms above instead of silently skipping (§15.D gap③
			// 伪残差 killed on the hop lane too).
			if runtimeTraceProjTargetSymptomMS(model) <= 0 && attributed > 0 {
				if hopSleep, hopWinStart, hopWinEnd := runtimeTraceProjHopOnlyTargetSleep(model); hopSleep > 0 && attributed <= hopSleep {
					// CR-2 组③ P7 / F-3 (冷读 F-3, 2026-07-12): the 「X 中 Y
					// 已由链上解释」 wording claims Y ⊆ X — false when the
					// chain-explained mass IS the cadence-idle segment PACE-ROW
					// carved OUT of the sleep family (donghu: 15.758 pacing vs
					// ×7=35.351 family, disjoint by construction). Precise
					// signal: a target-self cadence-idle row whose display
					// value Round3-equals the numerator. The fork states two
					// separate facts; every other shape keeps the legacy arms
					// byte-identically.
					if idleWord, ok := runtimeTraceProjTargetIdleCarveMatch(model, attributed, zh); ok {
						if zh {
							fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms(不含%s);链上解释的 %.3fms 为独立成行的%s段,不在上句睡眠合计内。",
								hopSleep, idleWord, attributed, idleWord)
						} else {
							fmt.Fprintf(&b, "\n- Focused-thread sleep %.3fms (excluding %s); the %.3fms explained on-chain is the separately rendered %s segment, outside the sleep total above.",
								hopSleep, idleWord, attributed, idleWord)
						}
					} else {
						// §21 CWD hard gate (precise numeric comparison — the one
						// signal kind allowed to gate a disclosure FORM): a target
						// sleep LONGER than the window it is printed next to is a
						// naked self-contradiction ("目标睡眠 115.902ms" beside
						// "共 101.000ms"). Such a magnitude must carry its base:
						// the hop row's own typed query window when it provably
						// differs from the anchor (F-2 tolerance), else a neutral
						// beyond-the-window clause (true both for a state crossing
						// out of the window and for an unknown base — never a
						// guessed 非关注窗口 claim). hopSleep ≤ window keeps the
						// legacy wording byte-identically.
						switch {
						case hopSleep > model.WindowMS && hopWinStart > 0 && hopWinEnd > hopWinStart &&
							runtimeTraceProjCoverageWindowBaseMismatch(projection, hopWinStart, hopWinEnd):
							if zh {
								fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms(取自查询窗 %.3f~%.3fs,非上句分析窗)中 %.3fms 已由链上解释。", hopSleep, hopWinStart, hopWinEnd, attributed)
							} else {
								fmt.Fprintf(&b, "\n- Of the focused thread's %.3fms sleep (from query window %.3f~%.3fs, not the analysis window above), %.3fms is explained on-chain.", hopSleep, hopWinStart, hopWinEnd, attributed)
							}
						case hopSleep > model.WindowMS:
							if zh {
								fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms(该状态时长超出上句分析窗)中 %.3fms 已由链上解释。", hopSleep, attributed)
							} else {
								fmt.Fprintf(&b, "\n- Of the focused thread's %.3fms sleep (its state duration extends beyond the analysis window above), %.3fms is explained on-chain.", hopSleep, attributed)
							}
						case zh:
							fmt.Fprintf(&b, "\n- 关注线程睡眠 %.3fms 中 %.3fms 已由链上解释。", hopSleep, attributed)
						default:
							fmt.Fprintf(&b, "\n- Of the focused thread's %.3fms sleep, %.3fms is explained on-chain.", hopSleep, attributed)
						}
					}
				}
			}
		default:
			if zh {
				fmt.Fprintf(&b, "\n- 链上已归因 %.3fms(其实际状态跨出窗口,见 ⚠ 标记)。", attributed)
			} else {
				fmt.Fprintf(&b, "\n- On-chain attributed %.3fms (the underlying state crosses the window; see ⚠ marks).", attributed)
			}
		}
	}
	// P0-A2 §12.3-4 裁定4① (F7: OUTSIDE the coverage-sentence gate — a pure
	// depthless / self-heavy shape whose numerator is 0 renders no coverage
	// sentence at all, yet that is precisely the shape most in need of the
	// disclosure). Honestly say how many on-chain rows the numerator could not
	// count and their single largest magnitude. Zero-caliber-risk additive
	// wording; empty (byte-identity) when every on-chain row was countable.
	if note := runtimeTraceProjUnadmittedOnChainDisclosureNote(model, zh); note != "" {
		// PTV8-RCR-B (UXA 域A layout-⑤, 2026-07-08): the disclosure is its own
		// "- " line — the header's three facts no longer glue into one run-on.
		b.WriteString("\n- " + note)
	}
	// 冷读扩臂④ 板级警示 (SMR-1 修复轮, 2026-07-13): when the rank seats'
	// effective attributions Σ exceeds the window length, one honest board
	// sentence — the seats' physical times overlap and never sum (typed
	// precise: the field mints only on over-window, so within-window reports
	// stay byte-identical).
	if model.RankBoardEffSumMS > 0 && model.WindowMS > 0 {
		// XLANE-3 件3: on a multi-board report the Σ is one board's account —
		// the sentence scopes its claim to 同板 (the legacy all-seats wording
		// would claim a population it no longer sums).
		if model.RankBoardEffSumMultiBoard {
			if zh {
				b.WriteString(fmt.Sprintf("\n- 同板根因席位有效归因合计最大 %.3fms 超过窗长 %.3fms:席位间物理时间可重叠,不可直接相加;不同板的席位更不可跨板相加。",
					model.RankBoardEffSumMS, model.WindowMS))
			} else {
				b.WriteString(fmt.Sprintf("\n- One board's rank-seat effective attributions sum to %.3fms, exceeding the %.3fms window: the seats' physical times can overlap and must not be added — and seats from different boards never add across boards.",
					model.RankBoardEffSumMS, model.WindowMS))
			}
		} else if zh {
			b.WriteString(fmt.Sprintf("\n- 各根因席位有效归因合计 %.3fms 超过窗长 %.3fms:席位间物理时间可重叠,不可直接相加。",
				model.RankBoardEffSumMS, model.WindowMS))
		} else {
			b.WriteString(fmt.Sprintf("\n- The rank seats' effective attributions sum to %.3fms, exceeding the %.3fms window: the seats' physical times can overlap and must not be added.",
				model.RankBoardEffSumMS, model.WindowMS))
		}
	}
	// R2 双窗关系行: when a user-requested window was derivable from the typed
	// entity pair and the projection window is a small sub-window of it (strict
	// numeric comparison: projection < 50% of the user window), say explicitly
	// how the two windows relate — the berlin customer saw a 101ms projection
	// with no mention of the 3.3s window they actually asked about.
	if model.UserWindowEnd > model.UserWindowStart && model.UserWindowStart > 0 {
		userMS := (model.UserWindowEnd - model.UserWindowStart) * 1000
		switch {
		case model.WindowMS < userMS*0.5:
			// PTV5 C25 (#68): the sub-window comes from engine anchoring — the
			// system never verified representativeness, so the line says 聚焦
			// (focused), not 代表性 (representative).
			if zh {
				fmt.Fprintf(&b, "\n- 用户请求窗 %.3f~%.3fs(共 %.1fs);本因果树的分析窗只取其中一段,全窗指标见 Trace 指标快照",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			} else {
				fmt.Fprintf(&b, "\n- User-requested window %.3f~%.3fs (%.1fs total); this causal tree's analysis window is one slice of it — full-window metrics live in the Trace Metric Snapshot",
					model.UserWindowStart, model.UserWindowEnd, userMS/1000)
			}
		case model.SoloArtifact:
			// PTV8-LAD L7 (§24.14 补2, D-4 ±10% 容差裁定的单工件面): the
			// analysis window deviates from the user's stated duration by more
			// than ±10% → one tree-head sentence names both lengths and the
			// deviation (COV-2 同判据 helper family; the comparison face folds
			// its own per-side note). The <50% slice shape above keeps its
			// richer R2 relation line byte-identically (it already relates the
			// two windows — no double disclosure); ≤±10% stays silent.
			deviation, beyond := runtimeTraceProjUserWindowDeviationPct(model.WindowMS, userMS)
			if !beyond {
				break
			}
			switch {
			case deviation > 0 && zh:
				fmt.Fprintf(&b, "\n- 分析窗 %.3fms,较你指定的 %.3fms 长 %.1f%%:窗口按数据边界对齐构造", model.WindowMS, userMS, deviation)
			case deviation > 0:
				fmt.Fprintf(&b, "\n- The analysis window %.3fms is %.1f%% longer than your requested %.3fms: the window is constructed by aligning to data boundaries", model.WindowMS, deviation, userMS)
			case zh:
				fmt.Fprintf(&b, "\n- 分析窗 %.3fms,较你指定的 %.3fms 短 %.1f%%:窗口按数据边界对齐构造", model.WindowMS, userMS, -deviation)
			default:
				fmt.Fprintf(&b, "\n- The analysis window %.3fms is %.1f%% shorter than your requested %.3fms: the window is constructed by aligning to data boundaries", model.WindowMS, -deviation, userMS)
			}
		}
	}
	// PTV5 Q3 (#68 用户裁定 2026-07-05): ≥2 distinct typed query windows →
	// the tree header declares the count (R2 关系行先例 — one added "- " line,
	// no arithmetic) and points at the per-window-grouped metric snapshot.
	// Single-window compiles stay byte-identical. 复核 Low (2026-07-06): a
	// cap-truncated list renders a LOWER BOUND (≥N), never a fake exact count.
	if len(projection.QueryWindows) >= 2 {
		count := fmt.Sprintf("%d", len(projection.QueryWindows))
		if projection.QueryWindowsTruncated {
			count = fmt.Sprintf("≥%d", len(projection.QueryWindows))
		}
		if zh {
			fmt.Fprintf(&b, "\n- 本报告数据来自 %s 个查询窗(本因果树基于其中之一);各窗指标见 Trace 指标快照", count)
		} else {
			fmt.Fprintf(&b, "\n- This report draws on %s query windows (this causal tree is based on one of them); per-window metrics live in the Trace Metric Snapshot", count)
		}
	}
	return b.String()
}

// runtimeTraceProjHopOnlyOvershootSleepMS feeds the A5 hop-lane overshoot
// arms (PTV8-RCR-A, §15.D gap③ hop 车道): the target's hop-view sleep total,
// but ONLY on the hop-only shape (no state-view symptom denominator) with a
// positive numerator — every other shape returns 0 and the coverage sentence
// keeps its legacy arms byte-identically. Wording fork only, never a gate.
func runtimeTraceProjHopOnlyOvershootSleepMS(model runtimeTraceProjTreeModel, attributed float64) float64 {
	if attributed <= 0 || runtimeTraceProjTargetSymptomMS(model) > 0 {
		return 0
	}
	hopSleep, _, _ := runtimeTraceProjHopOnlyTargetSleep(model)
	return hopSleep
}

// runtimeTraceProjSymptomOvershootJitterMS is the state-boundary jitter
// allowance for the coverage denominator (§15.D gap③, P0-A1 显示批): a depth-1
// attribution span's wall clock may overhang the target's summed symptom-state
// segments by edge-timestamp granularity — the q6 witness was a 112.223ms
// resolved lock span against a 112.175ms target sleep, 0.048ms of pure
// boundary jitter that silently demoted the denominator to the whole window
// (94% + a fabricated 6.838ms residual). Within this allowance the symptom
// stays the denominator at full coverage; beyond it the F1 nesting premise is
// treated as broken and the coverage sentence publishes both magnitudes with
// no percentage. Absolute (not relative): boundary jitter scales with edge
// count/timestamp granularity, never with symptom duration. This constant
// forks WORDING between two honest renderings of the same two numbers — it is
// never a hard gate (precise-signals red line).
const runtimeTraceProjSymptomOvershootJitterMS = 0.5

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
	total, _, _, _ := runtimeTraceProjTargetSymptomAdmission(model)
	return total
}

// runtimeTraceProjTargetSymptomAdmission is the ONE denominator-admission
// authority behind runtimeTraceProjTargetSymptomMS and the §24.11 C-3 census
// (two formerly hand-mirrored admission clauses — the census must skip exactly
// the rows the denominator admitted). Returns the symptom total plus the
// per-index admitted flags over model.SelfRows.
//
// EVOLUTION RECORD (§29.27② COV-4, 2026-07-11): this WAIT denominator is no
// longer the coverage account's only face — when the typed TargetStateAccount
// provably balances the analysis window, the FULL-WINDOW four-state partition
// renders ABOVE the wait-attribution sentence
// (runtimeTraceProjFourStateAccountLines; window-denominator percentages,
// running attribution, ruling-verbatim 自身执行 residual). This admission
// authority itself is unchanged: the wait sentence keeps the wait denominator
// (its base is disclosed as 不同基 in the account's legend entry), and every
// account-less shape stays byte-identical.
//
// DISP-3 (§29.8 P2-⑤ "textup 覆盖句分母排除目标 sleep", 2026-07-09): the F1
// hop-view exclusion exists because a hop view re-describes wall clock
// "already counted by its enclosing state segment" — but when the target
// published NO sleep-family STATE-view row at all, its dominant sleep exists
// ONLY as the wakeup_causal_impact hop view and the exclusion left a rump
// denominator (textup_792: two iowait state rows, 0.365ms, beside a 108.500ms
// sleep hop — the census arm then suppressed every percentage over a
// denominator that was never the target's wait). Repair arm, precise typed
// signals only: when ≥1 state row was admitted AND none of them is
// sleep-family (IsSleepState), the LARGEST sleep-state hop view joins the
// denominator (MAX over hop views, never a Σ — nested hop views overlap on
// the wall clock; disjoint scheduler states of one thread may then add).
// symptom==0 shapes (no state rows at all — huadong/opendir hop-only form)
// and sleep-state-admitted shapes stay byte-identical.
//
// 复核 P3-2 (2026-07-10): the hop admission is a MAX admission — when several
// eligible sleep-hop candidates compete, only the largest joins the
// denominator. The losers' magnitudes must not vanish silently under a 全称
// percentage, so the function also returns their count and single largest
// value (residue) for the coverage line's disclosure clause (wording only —
// the denominator value and every arm decision are untouched; zero on every
// shape where the hop arm did not engage or had no competitor).
// runtimeTraceProjSymptomAdmissionCandidates lists the SelfRows indices the
// admission's STATE-row loop can admit (its exact non-hop filters, shared by
// the loop and the A2 board pre-pass so the two can never disagree).
func runtimeTraceProjSymptomAdmissionCandidates(model runtimeTraceProjTreeModel) []int {
	var candidates []int
	for i, row := range model.SelfRows {
		if row.Node.Role == types.TraceCausalRoleCausalHop || row.SelfSymptomRelocated {
			continue
		}
		if !runtimeTraceProjSymptomFamilyStateKind(row.Node) || row.Node.ImpactMS <= 0 {
			continue
		}
		candidates = append(candidates, i)
	}
	return candidates
}

// runtimeTraceProjSymptomBoardGroups runs the A2 (修补轮, 2026-07-16)
// per-board election over the admission candidates: candidates carrying a
// typed board identity group by the shared triple index; the board with the
// largest candidate ImpactMS sum wins (tie → lexicographically smaller board
// ID — deterministic). Returns the per-index exclusion set (candidates of
// LOSING named boards; empty unless ≥2 named boards exist) and a carrier row
// index of the winning/only named board (-1 = no named board).
func runtimeTraceProjSymptomBoardGroups(model runtimeTraceProjTreeModel) (map[int]bool, int) {
	candidates := runtimeTraceProjSymptomAdmissionCandidates(model)
	var named []*runtimeTraceProjTreeRow
	rowIdx := map[*runtimeTraceProjTreeRow]int{}
	for _, i := range candidates {
		row := &model.SelfRows[i]
		if strings.TrimSpace(row.Node.RankBoardTarget) == "" {
			continue // identity-less: never provably foreign, always stays
		}
		named = append(named, row)
		rowIdx[row] = i
	}
	if len(named) == 0 {
		return nil, -1
	}
	boardIDs := runtimeTraceProjStableRankBoardIDs(named)
	sums := map[string]float64{}
	var order []string
	for _, row := range named {
		id := boardIDs[row]
		if _, seen := sums[id]; !seen {
			order = append(order, id)
		}
		sums[id] += row.Node.ImpactMS
	}
	sort.Strings(order)
	best := order[0]
	for _, id := range order[1:] {
		if sums[id] > sums[best] {
			best = id
		}
	}
	carrier := -1
	excluded := map[int]bool{}
	for _, row := range named {
		if boardIDs[row] == best {
			if carrier < 0 || rowIdx[row] < carrier {
				carrier = rowIdx[row]
			}
			continue
		}
		if len(order) >= 2 {
			excluded[rowIdx[row]] = true
		}
	}
	return excluded, carrier
}

// runtimeTraceProjSymptomBoardExclusions is the admission-facing half of the
// A2 election: the candidate indices whose named board lost. Empty (legacy
// byte-identity) on every shape with ≤1 named board among the candidates.
func runtimeTraceProjSymptomBoardExclusions(model runtimeTraceProjTreeModel) map[int]bool {
	excluded, _ := runtimeTraceProjSymptomBoardGroups(model)
	return excluded
}

// runtimeTraceProjSymptomDenominatorBoard names the symptom denominator's
// board: a carrier node of the winning (or only) NAMED board among the
// admission candidates. ok=false when the denominator has no named board
// (pure identity-less shapes).
func runtimeTraceProjSymptomDenominatorBoard(model runtimeTraceProjTreeModel) (types.TraceCausalProjectionNode, bool) {
	_, carrier := runtimeTraceProjSymptomBoardGroups(model)
	if carrier < 0 {
		return types.TraceCausalProjectionNode{}, false
	}
	return model.SelfRows[carrier].Node, true
}

func runtimeTraceProjTargetSymptomAdmission(model runtimeTraceProjTreeModel) (float64, []bool, int, float64) {
	admitted := make([]bool, len(model.SelfRows))
	total := 0.0
	sleepStateAdmitted := false
	// 修补轮 件A2 (2026-07-16, donghu 参数分叉 witness: 同窗同 target 双步
	// MinDurationMs 0.5/5.0 → 分母 257.635 > 窗 233.190 — 板2 的聚合 runnable
	// 席 17.815 与板1 的发生段席非孪生双计): the denominator is PER BOARD.
	// When the candidate state rows span ≥2 NAMED boards (typed triple
	// identity, shared index), only the largest board's rows stay in the
	// denominator; identity-less rows (plain state views, hop views) are not
	// provably foreign and always stay (absence never splits — the pure
	// legacy and single-step shapes are byte-identical). The excluded rows
	// flow into the C-3 census as 未计入分母 members through the admitted
	// flags, and their values stay untouched on their own rendered rows.
	boardExcluded := runtimeTraceProjSymptomBoardExclusions(model)
	for i, row := range model.SelfRows {
		if boardExcluded[i] {
			continue // A2: another board's account — never summed into this denominator
		}
		if row.Node.Role == types.TraceCausalRoleCausalHop {
			continue // blocked-wait/attribution hop view: wall clock already counted by its enclosing state segment
		}
		// 复核 P1-2 (GAP-B 收尾, 2026-07-09): a G11-RELOCATED wait-symptom rank
		// row (Role=RootCauseContext on the production shape, dominant_state
		// often sleep) re-describes wall clock that already lives inside the
		// target's own state segments — summing it beside the outer sleep row
		// is exactly the F1-forbidden nesting shape (REPRO: 10.0 outer sleep +
		// relocated 8.0 → 18.000 denominator). The typed relocation flag skips
		// it here and in the census below; its magnitude stays disclosed on
		// its own rendered self row.
		if row.SelfSymptomRelocated {
			continue
		}
		if !runtimeTraceProjSymptomFamilyStateKind(row.Node) {
			continue // running/stateless rows are not wait symptom time
		}
		if row.Node.ImpactMS > 0 {
			total += row.Node.ImpactMS
			admitted[i] = true
			if row.Node.IsSleepState() {
				sleepStateAdmitted = true
			}
		}
	}
	residueCount, residueMax := 0, 0.0
	if total > 0 && !sleepStateAdmitted {
		// Window-base guards (the pinned §24.11 C-3 crossBase shapes stay
		// byte-identical): a hop that is itself a multi-window merge, a
		// drilled-outside-the-request row, or any positively disagreeing
		// window among the coverage-feeding rows leaves the admission closed —
		// the census/crossBase disclosure arms then speak, exactly as pinned
		// (huadong_78 ×29 multi-window sleep view witness).
		if _, _, _, conflict := runtimeTraceProjCoverageWindowConsensus(model); !conflict {
			bestIdx, best := -1, 0.0
			eligible := []int{}
			for i, row := range model.SelfRows {
				if row.Node.Role != types.TraceCausalRoleCausalHop || row.SelfSymptomRelocated {
					continue
				}
				if !row.Node.IsSleepState() {
					continue
				}
				if runtimeTraceProjMultiWindowMergedRow(row.Node) {
					continue
				}
				if row.Node.WithinRequestedWindow != nil && !*row.Node.WithinRequestedWindow {
					continue
				}
				if v := runtimeTraceProjNodeDisplayImpact(row.Node); v > 0 {
					eligible = append(eligible, i)
					if v > best {
						best, bestIdx = v, i
					}
				}
			}
			if bestIdx >= 0 {
				total += best
				admitted[bestIdx] = true
				// P3-2: the MAX admission's silent losers — same candidate
				// universe as the arm itself minus the winner.
				for _, i := range eligible {
					if i == bestIdx {
						continue
					}
					residueCount++
					if v := runtimeTraceProjNodeDisplayImpact(model.SelfRows[i].Node); v > residueMax {
						residueMax = v
					}
				}
			}
		}
	}
	return total, admitted, residueCount, residueMax
}

// runtimeTraceProjHopAdmissionResidueNote renders the 复核 P3-2 disclosure of
// the MAX admission's silent losers (wording only; the existing census
// sentence's own phrase verbatim — zero new vocabulary): appended to the
// percentage-rendering coverage arms exactly when ≥1 eligible sleep-hop
// candidate with a positive value lost the MAX race. "" on every other shape
// (single-hop, hop-arm-not-engaged and healthy-nested forms stay
// byte-identical).
func runtimeTraceProjHopAdmissionResidueNote(count int, maxMS float64, zh bool) string {
	if count <= 0 || maxMS <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("另有 %d 条关注线程状态行未计入分母(单项最大 %.3fms)。", count, maxMS)
	}
	return fmt.Sprintf(" %d more focused-thread state row(s) are not in the denominator (single largest %.3fms).", count, maxMS)
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
	// Catalog B12 (DISPLAY-HYG 二轮): the ref wears the document-wide
	// bracket style — this was the report's only (E#) parenthetical face.
	if zh {
		ref := ""
		if tag != "" {
			ref = "[" + tag + "]"
		}
		return fmt.Sprintf("未归因中最大 %.3fms 与自身 IO 口径行%s重叠解释,未计入链上归因以防双计。", value, ref)
	}
	ref := ""
	if tag != "" {
		ref = " [" + tag + "]"
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
// runtimeTraceProjHopOnlyTargetSleepMS is the PTV5 Q2 hop-only magnitude
// (#68 用户裁定 2026-07-05): when the target published NO state-view symptom
// row (runtimeTraceProjTargetSymptomMS == 0 — the F1 ruling keeps hop views
// out of the coverage DENOMINATOR), its SLEEP-STATE hop-view self rows still
// re-describe the blocked wall clock. The info line reuses the LARGEST such
// row's display value — MAX, never a sum: nested hop views overlap on the
// wall clock. 0 when no sleep-state hop-view self row exists.
func runtimeTraceProjHopOnlyTargetSleepMS(model runtimeTraceProjTreeModel) float64 {
	ms, _, _ := runtimeTraceProjHopOnlyTargetSleep(model)
	return ms
}

// runtimeTraceProjHopOnlyTargetSleep is the full-fat variant: alongside the
// magnitude it returns the selected max row's own typed query window (zero
// when that row carried no selected_window identity). §21 CWD (cmp_01 revisit
// 2026-07-07, repeat-P0 覆盖句窗基错配): the hop-view sleep can come from a
// DIFFERENT query window than the projection anchor — "目标睡眠 115.902ms"
// printed beside "共 101.000ms" with no window base was a naked
// self-contradiction; callers use the returned window to name the base.
func runtimeTraceProjHopOnlyTargetSleep(model runtimeTraceProjTreeModel) (float64, float64, float64) {
	max, winStart, winEnd := 0.0, 0.0, 0.0
	for _, row := range model.SelfRows {
		if row.Node.Role != types.TraceCausalRoleCausalHop {
			continue
		}
		if !row.Node.IsSleepState() {
			continue
		}
		if v := runtimeTraceProjNodeDisplayImpact(row.Node); v > max {
			max = v
			winStart, winEnd = 0, 0
			if row.Node.QueryWindowStartTs > 0 && row.Node.QueryWindowEndTs > row.Node.QueryWindowStartTs {
				winStart, winEnd = row.Node.QueryWindowStartTs, row.Node.QueryWindowEndTs
			}
		}
	}
	return max, winStart, winEnd
}

// runtimeTraceProjTargetIdleCarveMatch is the F-3 carve detector (CR-2 组③
// P7, 2026-07-12): a rendered TARGET-SELF cadence-idle row (pacing_idle /
// periodic_idle — the PACE-ROW carve-out of the sleep family) whose display
// value Round3-equals the coverage numerator proves the chain-explained mass
// is the carved segment, NOT a subset of the sleep-family total. Returns the
// idle row's display word for the reworded sentence; ok=false on every other
// shape (fail-open to the legacy wording).
func runtimeTraceProjTargetIdleCarveMatch(model runtimeTraceProjTreeModel, attributed float64, zh bool) (string, bool) {
	for _, row := range model.SelfRows {
		kind := runtimeTraceProjIdleRowKind(row.Node)
		if kind == "" || !row.HasData {
			continue
		}
		if !runtimeTraceProjRound3Equal(runtimeTraceProjNodeDisplayImpact(row.Node), attributed) {
			continue
		}
		word := "帧间空闲"
		if kind == "periodic_idle" {
			word = "周期空闲"
		}
		if !zh {
			word = "pacing idle"
			if kind == "periodic_idle" {
				word = "periodic idle"
			}
		}
		return word, true
	}
	return "", false
}

// runtimeTraceProjChainDataQueryWindow returns the SINGLE typed query window
// shared by every windowed coverage-feeding row — the data-bearing chain rows
// (the depth-1 numerator lane) and the target's own self rows (the admitted
// numerator lane + the hop-only sleep magnitude). ok=false when no such row
// carries a window identity, or when the windowed rows disagree beyond the
// F-2 ±1ms endpoint tolerance (a mixed-window chain has no single base to
// name; the caller keeps the legacy rendering — fail-open, precise signals
// only). §21 CWD (cmp_01 revisit 2026-07-07, repeat-P0 覆盖句窗基错配): the
// anchor election can pick an anchor window that carries NO chain data while
// every chain row came from another query window — the coverage arithmetic
// must not divide that cross-window numerator by the anchor-window
// denominator. EVOLUTION RECORD (§21.1 CWD-2 ④, 2026-07-07): when data-bearing
// chain rows exist, ≥1 of them must itself carry the window identity for a
// window to be named — a windowed self row alone no longer establishes the
// "chain-data window" claim (see runtimeTraceProjCoverageWindowConsensus).
func runtimeTraceProjChainDataQueryWindow(model runtimeTraceProjTreeModel) (float64, float64, bool) {
	start, end, ok, conflict := runtimeTraceProjCoverageWindowConsensus(model)
	if conflict {
		return 0, 0, false
	}
	return start, end, ok
}

// runtimeTraceProjCoverageWindowConsensus is the shared consensus core behind
// runtimeTraceProjChainDataQueryWindow. It scans the same two coverage-feeding
// lanes (data-bearing chain rows + the target's self rows) and returns:
//   - (start, end, ok): the single typed query window every windowed row
//     agreed on (F-2 ±1ms endpoint tolerance), ok=false when no row carried
//     an identity;
//   - conflict=true when two windowed rows POSITIVELY disagreed beyond the
//     tolerance — the precise "分子分母窗基不可证同基" signal the §21.1
//     CWD-2 ③ symptom-denominator gate consumes (rows without identity never
//     vote and never veto, so pure-legacy windowless models stay
//     conflict-free and byte-identical).
//
// §21.1 CWD-2 ④ (chain 窗共识收紧, CWD 复核留账③b): when data-bearing chain
// rows exist, at least ONE of them must carry the window identity for the
// consensus to establish — previously a windowed SELF row alone could name a
// "chain-data window" that no chain row (the actual coverage numerator) ever
// attested. Precise existence predicate only; the no-chain-rows shape keeps
// the self-row consensus unchanged.
func runtimeTraceProjCoverageWindowConsensus(model runtimeTraceProjTreeModel) (float64, float64, bool, bool) {
	start, end, ok, conflict := 0.0, 0.0, false, false
	consider := func(node types.TraceCausalProjectionNode) bool {
		// §21.1 CWD-2 复核收尾① (对抗复核 M3 残口; huadong E1 witness:
		// VSyncGenerator ×9 包络横跨两查询窗): a multi-window merged row
		// POSITIVELY attests >1 query windows while the aggregator zeroes its
		// row-level QueryWindow identity (mixed rosters never claim a single
		// window) — without this arm the row sat silently in the coverage
		// numerator and the symptom %-face divided a cross-window magnitude
		// by a single-window denominator. Same typed key as the W1 %-face
		// (runtimeTraceProjMultiWindowMergedRow — zero new signals);
		// single-window rosters fall through to the row-identity lanes below.
		if runtimeTraceProjMultiWindowMergedRow(node) {
			conflict = true
			return true
		}
		if node.QueryWindowStartTs <= 0 || node.QueryWindowEndTs <= node.QueryWindowStartTs {
			return false // no identity — never votes, never vetoes
		}
		if !ok {
			start, end, ok = node.QueryWindowStartTs, node.QueryWindowEndTs, true
			return true
		}
		if math.Abs(node.QueryWindowStartTs-start) > types.TraceCausalProjectionSameWindowToleranceS ||
			math.Abs(node.QueryWindowEndTs-end) > types.TraceCausalProjectionSameWindowToleranceS {
			conflict = true
		}
		return true
	}
	chainDataRows, chainWindowed := 0, false
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || !row.HasData {
			continue
		}
		chainDataRows++
		if consider(row.Node) {
			chainWindowed = true
		}
	}
	for _, row := range model.SelfRows {
		consider(row.Node)
	}
	if conflict {
		return 0, 0, false, true
	}
	if chainDataRows > 0 && !chainWindowed {
		return 0, 0, false, false
	}
	return start, end, ok, false
}

// runtimeTraceProjCoverageWindowBaseMismatch reports whether the single
// chain-data query window differs from the projection's anchor window beyond
// the F-2 ±1ms endpoint tolerance — the precise §21-CWD cross-window-coverage
// signal (typed endpoints on both sides; no anchor window → no claim).
func runtimeTraceProjCoverageWindowBaseMismatch(projection types.TraceCausalProjection, ws, we float64) bool {
	if projection.WindowStartTs <= 0 || projection.WindowEndTs <= projection.WindowStartTs {
		return false
	}
	return math.Abs(ws-projection.WindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
		math.Abs(we-projection.WindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS
}

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

// runtimeTraceProjTargetSelfStateViewRow is the typed 目标自身状态行 predicate
// (COV §24.9 D-2, real_trace_campaign_20260705.md, 2026-07-08): the SelfRows
// row re-describes the 🎯 target's OWN scheduler-state wall clock — i.e. the
// coverage denominator's body (state-view symptom rows) or its hop-view
// re-description (Role=causal_hop sleep/wait rows, the very rows
// runtimeTraceProjHopOnlyTargetSleep selects from) — never an unexplained
// on-chain cause row. The opendir_78 witness: the "另有 4 条链上行未计入"
// disclosure counted the target's own ×3 sleep hop view (E2, the 115.353ms
// denominator body of the sentence right above it) as uncounted CAUSE rows.
// Wait-family typed signals only (Role enum + StateKind enum + the registered
// state-word universe) — a target self row without a state classification
// (e.g. the blocking_span self-lock row) stays a countable disclosure member
// (计数如实), and so does a blocked-wait hop row ON a counterpart (resolved
// peer or the F6 state/type-token reject like lock_contention): only a PURE
// state re-description — wait-family StateKind whose influence point is itself
// a registered scheduler-state token (or absent) — is the denominator body.
func runtimeTraceProjTargetSelfStateViewRow(row runtimeTraceProjTreeRow) bool {
	if row.Node.Role == types.TraceCausalRoleCausalHop {
		if !row.Node.IsSleepState() && !runtimeTraceProjWaitFamilyStateKind(row.Node) {
			return false
		}
		object := strings.ToLower(strings.TrimSpace(row.Node.Object))
		return object == "" || types.TraceStateKindRegistered(object)
	}
	return runtimeTraceProjSymptomFamilyStateKind(row.Node)
}

// runtimeTraceProjTargetSelfWaitViewRow is the typed 目标等待视图 predicate for
// the §24.11 C-3 denominator census: a state/hop wait view
// (runtimeTraceProjTargetSelfStateViewRow) OR a stateless self row whose typed
// token family is a wait form (the §24.3 impact-form registry lanes ⛓/☾/⧖ —
// the huadong_78 binder_wait self row carries a wait token but no StateKind).
// A row with a PRESENT non-wait StateKind (running/…) is occupancy, never wait
// time, and stays out on both lanes.
func runtimeTraceProjTargetSelfWaitViewRow(row runtimeTraceProjTreeRow) bool {
	if runtimeTraceProjTargetSelfStateViewRow(row) {
		return true
	}
	if strings.TrimSpace(row.Node.StateKind) != "" {
		return false
	}
	for _, token := range []string{row.Node.TypeToken, row.Node.Object, row.Node.Predicate} {
		switch runtimeTraceProjImpactFormTokenFamily(runtimeTraceCausalProjectionCanonicalNode(token)) {
		// PTV8-RCR-C 复核收尾 (2026-07-08): binder_wait left the ⛓ IOBlock
		// family (IPC ≠ block IO) but stays a WAIT view — the COV census
		// counted the huadong binder-wait 4.577 through the old membership and
		// must keep counting it through the new family row (等待症状族).
		// SYM-2 §24.17 R2 (2026-07-08): the D-state family split off IOBlock
		// for its 行2 word only — a D-state wait stays a WAIT view here.
		case runtimeTraceProjImpactFormIOBlock, runtimeTraceProjImpactFormDState,
			runtimeTraceProjImpactFormDStateIOMixed, runtimeTraceProjImpactFormIOWaitRefined,
			runtimeTraceProjImpactFormSleep,
			runtimeTraceProjImpactFormRunnable, runtimeTraceProjImpactFormBinderWait:
			return true
		}
	}
	return false
}

// runtimeTraceProjSymptomDenominatorCensus is the §24.11 C-3 (COV 批,
// real_trace_campaign_20260705.md, 2026-07-08) denominator-population census
// over the target's SelfRows: how many of the target's own WAIT-view rows were
// left OUT of the symptom denominator (runtimeTraceProjTargetSymptomMS admits
// only non-hop symptom-family state rows with a positive projection), the
// largest SINGLE-INSTANCE magnitude among them (×N → MergedMaxMS, periodic →
// discounted attribution; 墙钟不可加和), and whether every excluded row
// PROVABLY lives outside the anchor analysis window (typed row query-window
// identity vs the typed anchor endpoints, or the multi-window merged marker —
// absence of identity never claims a window). huadong_78 witness: the
// crossBase coverage sentence published "关注线程等待 0.011ms" while the
// denominator held ONE D-state row and the target's binder-wait 4.577,
// critical_blocking ×4 and sleep 6.661 ×29 hop views were silently excluded —
// a ~1000× understated 全称 wait claim. The census feeds a WORDING fork only
// (form-switch), never a gate (precise signals: typed row counts + enum
// predicates + numeric window comparisons).
func runtimeTraceProjSymptomDenominatorCensus(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel) (int, float64, bool) {
	excluded, maxMS := 0, 0.0
	allOffWindow := true
	// DISP-3 (§29.8 P2-⑤): the census reads the SAME admission authority the
	// denominator uses (runtimeTraceProjTargetSymptomAdmission) instead of a
	// hand-mirrored clause — the sleep-hop repair arm therefore drops its row
	// out of the "未计入分母" roster on arrival.
	_, admitted, _, _ := runtimeTraceProjTargetSymptomAdmission(model)
	for i, row := range model.SelfRows {
		if !row.HasData {
			continue
		}
		// 复核 P1-2 (2026-07-09): relocated G11 rows re-describe wall clock
		// already inside the target's state segments — they are neither
		// denominator members nor under-represented exclusions (their
		// magnitude renders on their own self rows).
		if row.SelfSymptomRelocated {
			continue
		}
		if admitted[i] {
			continue // the exact TargetSymptomMS admission: this row IS the denominator
		}
		if !runtimeTraceProjTargetSelfWaitViewRow(row) {
			continue
		}
		// RNB-5B 件② 连带 (F3 同族, 2026-07-15): a ⌗ caliber-side value
		// (计数当量/综合评分) must never publish as this census's bare-ms
		// 单项最大 — the row is not a wall-clock wait view (the 17267 witness:
		// the relocated 计数当量 81.616 printed as 「单项最大 81.616ms」 the
		// moment the ② side-rail carriage made it visible to SelfRows).
		if row.Node.IsCaliberSideRow() ||
			tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken)) != tracequery.CausalCaliberSideNone {
			continue
		}
		v := runtimeTraceProjNodeDisplayImpact(row.Node)
		if row.Node.MergedCount > 1 && row.Node.MergedMaxMS > 0 {
			v = row.Node.MergedMaxMS
		}
		if row.Node.PeriodicSource {
			v = row.Node.EffectiveImpactMS
		}
		if v <= 0 {
			continue // no wall clock of its own → nothing under-represented
		}
		excluded++
		if v > maxMS {
			maxMS = v
		}
		offWindow := runtimeTraceProjMultiWindowMergedRow(row.Node) ||
			(row.Node.QueryWindowStartTs > 0 && row.Node.QueryWindowEndTs > row.Node.QueryWindowStartTs &&
				runtimeTraceProjCoverageWindowBaseMismatch(projection, row.Node.QueryWindowStartTs, row.Node.QueryWindowEndTs))
		if !offWindow {
			allOffWindow = false
		}
	}
	if excluded == 0 {
		return 0, 0, false
	}
	return excluded, maxMS, allOffWindow
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
//
// 修补轮 件A1 (2026-07-16, donghu fused witness: the fused 2955-target report
// annotated CompThread's unattributed residual with 「其中 56.229ms 为周期性
// 信号源期内正常节拍」 whose 56.229 came ENTIRELY from the 9163 board's
// AudioOut chain rows — 跨板假归属): on a MULTI-BOARD report the Σ counts
// only rows PROVABLY on the coverage subject's board (shared board identity,
// 与 Σ 面同源); a periodic row that cannot prove board membership
// (identity-less, or the subject board itself is unresolved) drops — the
// sentence then honestly disappears rather than annotating one board's
// residual with another board's cadence. Single-board reports keep the
// legacy sum byte-identically.
func runtimeTraceProjPeriodicCadenceMS(model runtimeTraceProjTreeModel) float64 {
	scope := runtimeTraceProjCoverageBoardScopeFor(model)
	total := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || !row.HasData || !row.Node.PeriodicSource {
			continue
		}
		if scope.multi && !scope.rowOnSubjectBoard(row.Node) {
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
		return fmt.Sprintf("其中 %.3fms 为周期性信号源期内正常节拍(不计归因,也不属未解释等待)。", cadence)
	}
	return fmt.Sprintf(" Of that, %.3fms is a periodic signal source's normal in-period cadence (not attributed, not unexplained residual).", cadence)
}

// runtimeTraceProjDepth1Cumulative is the on-chain attribution numerator. It is
// the MAX (never Σ — wall clock does not add across rows that overlap the
// target's blocked interval) of two typed lanes:
//   - the depth-resolved chain cumulative (runtimeTraceProjDepth1CumulativeChain,
//     the original H10-shallow-fallback lane, byte-preserved);
//   - the admitted TARGET-SELF lane (P0-A2 §12.3-4 裁定4②, coordinator lane
//     ruling 2026-07-07): the 🎯 target's OWN blocked-wait rows in model.SelfRows
//     whose influence point (Object) is a resolved thread identity — the q6/q8
//     lock/binder case where the target itself blocked on a NAMED holder/peer
//     (the row carries EffectiveImpactMS + a resolved Object and lives in
//     SelfRows, not the depthless stray lane). These are the true carriers of
//     "the target's own wait already explained on-chain" that the depth-only
//     numerator dropped, leaving self-heavy shapes at a falsely-low coverage.
//     Rows whose counterpart is a bare STATE/TYPE token (sleep_wait,
//     lock_contention, …) are NOT resolved counterparts and stay residual +
//     disclosed (F6). The own-process IO caliber lane is excluded (F4/NEW-6
//     double-count guard). Requires a non-empty target (F5: a flat model with
//     target=="" never mints subjectless numerator mass).
//
// Boundary (裁定3): this fixes the ANCHOR-CORRECT self-heavy shape only. The F1
// q1-B2 witness (anchor = VSyncGenerator, NOT the user thread 42591) keeps its
// depth-1 numerator of 0.051 unchanged — that is CORRECT for a VSync anchor and
// is a B1 anchor-selection problem, not a P0-A2 coverage problem. Do not treat
// the q1-B2 depthless rows as the thing this lane admits.
func runtimeTraceProjDepth1Cumulative(model runtimeTraceProjTreeModel) float64 {
	chain := runtimeTraceProjDepth1CumulativeChain(model)
	admitted := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model)
	if admitted > chain {
		return admitted
	}
	return chain
}

// runtimeTraceProjDepth1CumulativeChain is the depth-resolved lane of the
// coverage numerator — the pre-P0-A2 body of runtimeTraceProjDepth1Cumulative,
// kept byte-identical so the depth-only shapes (P0-A1 q6/q8 witness, berlin H10
// fallback, VS-1 periodic) are unchanged: the admitted target-self MAX above only
// ever RAISES the numerator when a target-own resolved-counterpart self row
// exceeds it.
func runtimeTraceProjDepth1CumulativeChain(model runtimeTraceProjTreeModel) float64 {
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

// runtimeTraceProjChainDepthCumulative returns the largest 已由链上解释
// caliber among data-bearing chain rows at exactly the given depth.
//
// EVOLUTION RECORD (COV §24.9 D-1, real_trace_campaign_20260705.md,
// 2026-07-08): each row's caliber now consumes the typed TargetImpactMS
// (engine TargetBlockedMs — how much of the target's own blocked wall clock
// this row's chain actually explains) FIRST, and only falls back to the legacy
// CumulativeImpactMS channel when the row exposed no target caliber. The
// cumulative channel is display-overwritten by §20.1 on inversion∧running rank
// rows (opendir_78 witness: E4 cumulative 58.919 raw vs target_impact 112.175
// → the coverage sentence fabricated "已归因45%/未归因55%" against a ~97%
// explained wait). Rows without the note keep every legacy byte. 严禁伪造残差:
// the numerator is never clamped toward the denominator — when the typed
// caliber still under-explains, the honest arms disclose the shortfall as-is.
func runtimeTraceProjChainDepthCumulative(model runtimeTraceProjTreeModel, depth int) float64 {
	max := 0.0
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowChain || row.Depth != depth || !row.HasData {
			continue
		}
		v := row.Node.TargetImpactMS
		if v <= 0 {
			v = row.Node.CumulativeImpactMS
		}
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
		// RNB R2 numerator invariance (覆盖分子红线): a rank row folded into
		// this chain row would have competed in this same depth-MAX pre-fold
		// — its own caliber stays in the competition so the coverage
		// numerator is byte-identical to the two-row render. (Folded arms are
		// never periodic by classification. COV §24.9 D-1: the peer's typed
		// target caliber competes first, same ladder as the row itself.)
		for _, peer := range row.RankFoldPeers {
			pv := peer.TargetImpactMS
			if pv <= 0 {
				pv = peer.CumulativeImpactMS
			}
			if pv <= 0 {
				pv = peer.DisplayImpactMS
			}
			if pv > max {
				max = pv
			}
		}
	}
	return max
}

// runtimeTraceProjResolvedCounterpartObject reports whether a node's influence
// point (Object) names a RESOLVED thread identity (F6, coordinator 2026-07-07),
// as opposed to a bare scheduler-state / block-type token. TraceCausalProjection
// KnownSubject alone was too loose — it only rejects ""/"unknown-thread", so
// Object=="sleep_wait" / "d_state_or_io_wait" / "lock_contention" /
// "monitor_contention" (the state words hop rows carry in Object) were treated as
// resolved peers. The precise typed form check: a thread label carries a pid
// structure — a trailing "-<digits>" (name-pid, runtimeTraceProjSplitNamePid), a
// "pid=<digits>" handle (runtimeTraceProjPidHandleForm), or a bare pure-digit pid
// (runtimeTraceProjPureInt). State/type tokens carry none of these and are
// rejected structurally (no substring blacklist).
func runtimeTraceProjResolvedCounterpartObject(object string) bool {
	object = strings.TrimSpace(object)
	if object == "" {
		return false
	}
	if !types.TraceCausalProjectionKnownSubject(object) {
		return false
	}
	if _, _, ok := runtimeTraceProjSplitNamePid(object); ok {
		return true
	}
	if _, ok := runtimeTraceProjPidHandleForm(object); ok {
		return true
	}
	if _, ok := runtimeTraceProjPureInt(object); ok {
		return true
	}
	return false
}

// runtimeTraceProjTargetSelfNumeratorAdmits reports whether a target-self row
// (model.SelfRows) is admissible into the coverage numerator (P0-A2 §12.3-4
// 裁定4②, coordinator lane ruling 2026-07-07). SelfRows are the 🎯 target's own
// state / blocked-wait views by construction — the q6/q8 lock/binder case where
// the target blocked on a NAMED holder/peer lands here, carrying EffectiveImpactMS
// and a resolved Object. There is deliberately NO subject==target clause (SelfRows
// are all the target — that redundant clause was the very thing that killed the
// witness on the real builder). Precise typed criteria only:
//   - HasData;
//   - Edge != own (F4/NEW-6: own-process IO caliber is the residual-overlap lane
//     the coverage sentence already discloses as double-count-avoided);
//   - EffectiveImpactMS > 0 (a real attribution caliber, not a bare state view);
//   - the influence point Object is a resolved thread identity (F6).
func runtimeTraceProjTargetSelfNumeratorAdmits(row runtimeTraceProjTreeRow) bool {
	if !row.HasData || row.Edge == runtimeTraceProjTreeEdgeOwn {
		return false
	}
	if row.Node.EffectiveImpactMS <= 0 {
		return false
	}
	return runtimeTraceProjResolvedCounterpartObject(row.Node.Object)
}

// runtimeTraceProjAdmittedTargetSelfNumeratorMS is the admitted target-self lane
// of the coverage numerator (P0-A2 §12.3-4 裁定4②). The admitted rows are all the
// target's own blocked wall clock and overlap each other, so the caliber is a
// single MAX — NEVER a Σ (墙钟重叠禁加和; union is a later precise upgrade). The
// value is the EffectiveImpactMS attribution caliber (the same discounted caliber
// ranking/attribution uses; a periodic row's Effective is already discounted). 0
// for a flat model with target=="" (F5: never mints subjectless numerator mass)
// or when no self row qualifies (byte-identity for the P0-A1 witness).
func runtimeTraceProjAdmittedTargetSelfNumeratorMS(model runtimeTraceProjTreeModel) float64 {
	if strings.TrimSpace(model.Target) == "" {
		return 0
	}
	max := 0.0
	for _, row := range model.SelfRows {
		if !runtimeTraceProjTargetSelfNumeratorAdmits(row) {
			continue
		}
		if v := row.Node.EffectiveImpactMS; v > max {
			max = v
		}
	}
	return max
}

// runtimeTraceProjUnadmittedOnChainDisclosure is the P0-A2 §12.3-4 裁定4① honest
// disclosure of the coverage numerator's incompleteness (coordinator scope
// ruling 2026-07-07). Two typed sets, both wall-clock rows the numerator could
// NOT count:
//   - target-self rows NOT admitted by runtimeTraceProjTargetSelfNumeratorAdmits
//     but that carry a real attribution caliber (EffectiveImpactMS>0) — chiefly
//     the F6 UNRESOLVED-counterpart self rows (Object is a state/type token). The
//     own-edge lane is excluded symmetrically with admission (F4: no row gets two
//     contradictory exclusion reasons). EVOLUTION RECORD (COV §24.9 D-2,
//     2026-07-08): the target's own STATE/hop-view rows
//     (runtimeTraceProjTargetSelfStateViewRow — the coverage denominator's body)
//     are excluded from this lane: they are the 被解释对象, not uncounted cause
//     rows (opendir_78: the ×3 sleep hop view inflated "另有 4 条" where only the
//     self-lock row was a real uncounted member);
//   - depthless on-chain data rows (Kind==depthless) with a data caliber that the
//     depth-only numerator never counted, own-edge excluded (F4).
//
// The demoted-to-background lane is NOT counted: the builder overwrites the
// demoted display copy's ChainRelevance to "background" before it enters
// model.Background (tree.go:1014) and keeps no typed original relevance, so an
// ==\"on_chain\" test would be dead code — the scope is honestly narrowed (F3)
// rather than fabricated.
//
// Returns (count N, largest single value X, has-overflow-fold). X is a single-row
// MAX, never a Σ (墙钟不可加和). Each row's caliber is its display impact, except a
// periodic row which contributes its discounted EffectiveImpactMS (F8, same
// discipline as the numerator). N counts a MergedCount>1 fold row as its member
// count, not as 1 (F8). N==0 → no disclosure (byte-identity for the P0-A1
// depthless-free witness).
func runtimeTraceProjUnadmittedOnChainDisclosure(model runtimeTraceProjTreeModel) (int, float64, bool) {
	count := 0
	maxMS := 0.0
	folded := false
	// 修补轮 件A3 (2026-07-16): the census scopes by board on multi-board
	// reports (choice ① 按板分域) — a row PROVABLY on another board than the
	// coverage subject (typed board identity mismatch, or a named target
	// canonically different from the tree target when the subject board is
	// unresolved) is that board's account, not an uncounted row of THIS
	// sentence's account. Identity-less rows are never provably foreign and
	// stay counted (absence never splits); single-board reports are
	// byte-identical.
	scope := runtimeTraceProjCoverageBoardScopeFor(model)
	targetKey := runtimeTraceCausalProjectionCanonicalNode(model.Target)
	foreignBoard := func(node types.TraceCausalProjectionNode) bool {
		if !scope.multi {
			return false
		}
		label := strings.TrimSpace(node.RankBoardTarget)
		if label == "" {
			return false
		}
		if scope.subjectOK {
			return !scope.rowOnSubjectBoard(node)
		}
		return targetKey != "" && runtimeTraceCausalProjectionCanonicalNode(label) != targetKey
	}
	consider := func(node types.TraceCausalProjectionNode) {
		if foreignBoard(node) {
			return
		}
		if node.MergedCount > 1 {
			count += node.MergedCount
			folded = true
		} else {
			count++
		}
		v := runtimeTraceProjNodeDisplayImpact(node)
		if node.PeriodicSource {
			v = node.EffectiveImpactMS
		}
		if v > maxMS {
			maxMS = v
		}
	}
	for _, row := range model.SelfRows {
		if row.Edge == runtimeTraceProjTreeEdgeOwn || !row.HasData {
			continue
		}
		if runtimeTraceProjTargetSelfNumeratorAdmits(row) {
			continue
		}
		if row.Node.EffectiveImpactMS <= 0 {
			continue // no attribution caliber → not an "explained on-chain" candidate
		}
		// SELF-ALL 修复轮 件3 F3 (2026-07-13): the census claims 「链上行」 and
		// prints bare wall-clock ms — two typed gates keep it honest against
		// the SELF-LANE relocated rows now living in SelfRows:
		//   - channel: a typed-adjacent row (非链 relocation is display
		//     placement only) is NOT an on-chain row — the channel word reads
		//     the node's own relevance, never the display stanza;
		//   - caliber: a ⌗ caliber-side value (计数当量/综合评分) must never
		//     enter a bare-ms census MAX (G3/DISP-2 — the 81.616 count
		//     equivalent printed as 「单项最大 81.616ms」).
		if runtimeTraceProjNodeOrdinalChannel(row.Node) != runtimeTraceProjOrdinalChannelChain {
			continue
		}
		if row.Node.IsCaliberSideRow() {
			continue
		}
		// COV §24.9 D-2 (opendir_78): the target's OWN state/hop-view rows are
		// the 被解释对象 — the sentence above already used them as its
		// denominator — never "uncounted on-chain rows". Counting the ×3 sleep
		// hop view here read as "4 more causal rows were left out" when 3 of
		// the 4 were the denominator's own body. Typed predicate; the
		// unclassified self rows (e.g. the blocking_span self-lock row) stay
		// counted (计数如实).
		if runtimeTraceProjTargetSelfStateViewRow(row) {
			continue
		}
		consider(row.Node)
	}
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowDepthless || !row.HasData {
			// RNB-5B 修复轮 P1-1 (F8, 2026-07-15): the micro anchored-seat fold
			// row (Kind=Chain by construction) absorbed Depthless rows that were
			// individually counted here pre-fold — the F8 contract counts fold
			// rows by member, so those members enter the census through the
			// typed fold carrier (donghu 2955: 21→18 silent shrink without this
			// arm). The MAX competes on the largest SINGLE member value
			// (MergedMaxMS), never the fold's account Σ (X is a single-row
			// value by contract).
			if row.HasData && row.Node.MicroAnchorFold && row.MicroAnchorFoldDepthlessMembers > 0 &&
				!foreignBoard(row.Node) {
				count += row.MicroAnchorFoldDepthlessMembers
				folded = true
				if row.Node.MergedMaxMS > maxMS {
					maxMS = row.Node.MergedMaxMS
				}
			}
			continue
		}
		if row.Edge == runtimeTraceProjTreeEdgeOwn {
			continue
		}
		if foreignBoard(row.Node) {
			continue // A3: another board's row — neither counted nor MAX-competing
		}
		consider(row.Node)
		// 复核 W-B (RNB 收尾 2026-07-07): a rank twin folded into this
		// depthless row competed in this MAX pre-fold — its display magnitude
		// stays in the competition. The count keeps the actually-rendered row
		// count (行数诚实: post-fold there IS one fewer rendered row).
		for _, peer := range row.RankFoldPeers {
			if peer.DisplayImpactMS > maxMS {
				maxMS = peer.DisplayImpactMS
			}
		}
	}
	// SEM-LEAD (§29.7-2 ③, 2026-07-10): an on-chain rank twin folded into a
	// ✦ 语义 row rendered as a depthless row pre-fold and competed in this
	// MAX — the W-B rule verbatim: the magnitude stays in the competition,
	// the count keeps the actually-rendered row count (行数诚实).
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowSemantic || !row.HasData {
			continue
		}
		for _, peer := range row.RankFoldPeers {
			if peer.DisplayImpactMS > maxMS {
				maxMS = peer.DisplayImpactMS
			}
		}
	}
	return count, maxMS, folded
}

// runtimeTraceProjUnadmittedOnChainDisclosureNote renders the P0-A2 §12.3-4
// 裁定4① disclosure sentence, or "" when no on-chain row was left out of the
// numerator. Pure additive wording (zero-caliber risk) — it states N items,
// their largest single magnitude X, and that wall clock is not summable; it
// never changes the numerator or the percentages the coverage sentence printed.
// F8: plain-language "未计入的链上行" (no internal 行话); the "见明细表/树"
// pointer keeps the reader on an in-answer surface.
func runtimeTraceProjUnadmittedOnChainDisclosureNote(model runtimeTraceProjTreeModel, zh bool) string {
	n, x, _ := runtimeTraceProjUnadmittedOnChainDisclosure(model)
	if n <= 0 || x <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("另有 %d 条链上行未计入上句已归因数值(单项最大 %.3fms;墙钟不可加和,详见明细/树)。", n, x)
	}
	return fmt.Sprintf("A further %d on-chain row(s) are not counted in the attributed figure above (single largest %.3fms; wall clock not summable, see the detail blocks/tree).", n, x)
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
// runtimeTraceProjDetailTableLegendFlags reports which gated (a)-table legend
// rows this model's table actually needs (PTV5 C33/C34, #68): the ×N forms
// and the dual-seat note have entries ONLY when the notation appears — every
// other render stays byte-identical. Reads the SAME detail rows + seat map
// the table renders from.
type runtimeTraceProjDetailTableLegendFlags struct {
	mergedSum   bool
	mergedMax   bool
	mergedDedup bool
	multiSeat   bool
	// mergedUnion (§11-N2): a union-caliber ×N row is on the table. It is a
	// SEPARATE flag so a union row never raises mergedSum — the (a)-table's
	// gated "×N(a~b) = 数值为总和" line must never gloss a union value; the
	// union form's semantics ride the tree legend entry and the (b) lossless
	// block (原始和 + 窗来源).
	mergedUnion bool
	// mergedWindowMax (§21 CWD): a cross-window MAX ×N row is on the table —
	// same separation discipline as mergedUnion (never raises mergedSum), and
	// the (a)-table legend gets its own gated line.
	mergedWindowMax bool
	// family (RCM-2 D3, §24.7.1/§24.10): a family-merge contender row is on
	// the table (×N合计 token) — its own gated legend line; never raises any
	// Merged* flag (isolated typed lanes).
	family bool
	// selfSymptom (GAP-B G6, §27.3, 2026-07-09): a wait-symptom target-self
	// row (typed tier target_self_state) is on the table — its 有效归因 cell
	// renders "—" (the row never seats on the rank board), and the gated
	// legend line says so.
	selfSymptom bool
	// allZeroFold (DISP-2 G19, §27.5, 2026-07-09): an all-zero subjectless
	// fold row is on the table — its token is the 窗内无有效时长 note (never
	// the ×N取最大 form), so it must NOT raise mergedMax (whose gated line
	// claims a member-MAX value the note row does not carry).
	allZeroFold bool
	// stanzaChainTotal (DISP-2 G3 表列口径, §27.2, 2026-07-09): a ◇/▒ stanza
	// row carrying a cumulative value is on the table — its 链上累计 cell
	// renders "—" (the column's definition is the on-chain accumulation toward
	// the focused thread, which an off-chain row does not make), and the gated
	// legend line says where the value lives instead.
	stanzaChainTotal bool
	// gatedProjection (GATED-CAL 件1③, §29.104.16.1 M3-c, 2026-07-16): an
	// inversion seat whose window-projection cell value IS the gated composite
	// is on the table — the cell wears the 构成,见明细 annotation and this
	// gated line carries the inversion-seat qualifier the column legend
	// promises (图例反转席限定词).
	gatedProjection bool
	// scoreIOPressure / scoreBlockIO / countEquivalent / countClamp
	// (SCORE-DERIV, §29.104.22.1 user ruling 2026-07-17): the 阅读参考 formula
	// entries — 「让客户大概知道公式」, weight constants HIDDEN (值只在代码 +
	// docs/design/score_derivation_20260717.md). Each entry renders exactly
	// when its word face is on the render (承诺面双向): the flags read the
	// SAME typed predicates the value word faces read —
	//   scoreIOPressure: io_pressure token + composite value caliber (the
	//     M18 Unit=composite_score lane; runtimeTraceProjCompositeValueCaliber);
	//   scoreBlockIO: caliber-side CompositeScore class (block_io_by_inode);
	//   countEquivalent: caliber-side Count class or a count_sum family fold
	//     (the 计数当量 word family's two producers);
	//   countClamp: runtimeTraceProjFamilyCountSumClamped — the same typed
	//     comparison the 超上限截断 word reads.
	scoreIOPressure bool
	scoreBlockIO    bool
	countEquivalent bool
	countClamp      bool
	// fixDirection (AXIOM-V2 护栏③, user rulings 2026-07-18): the 根因排序键
	// definition entry (键=折算后可消除量,跨方向可比不可相加) — renders
	// exactly when a fix-direction word face or a cross-direction mutual
	// clause is on the render, read from the SAME emission-site marks the
	// tree legend consumes (承诺面双向; SCORE-DERIV zero-digit discipline).
	fixDirection bool
	// businessSpanMention (SPANVIS-1 件4 阅读参考层, user ruling 2026-07-19):
	// the ◈ dual-lever reading-reference entry (次数多而单段小→业务流程/调用
	// 次数方向;单段长→单次运行时长方向) — renders exactly when the ◈ word
	// face is on the render, read from the SAME emission-site mark the tree
	// legend consumes (承诺面双向; SCORE-DERIV precedent — teaches reading,
	// never judges a row).
	businessSpanMention bool
}

func runtimeTraceProjDetailTableLegendFlagsFor(model runtimeTraceProjTreeModel, zh bool) runtimeTraceProjDetailTableLegendFlags {
	detailRows := runtimeTraceProjDetailRows(model)
	seats := runtimeTraceProjDetailSeats(detailRows, zh)
	var flags runtimeTraceProjDetailTableLegendFlags
	// AXIOM-V2 护栏③: the same emission-site marks the tree legend consumes
	// (the fence renders before this legend, so the marks are final here).
	flags.fixDirection = model.Marks.has(runtimeTraceProjMarkFixDirection) ||
		model.Marks.has(runtimeTraceProjMarkCrossDirectionOverlap)
	// SPANVIS-1 件4: same emission-site mark as the ◈ legend entry.
	flags.businessSpanMention = model.Marks.has(runtimeTraceProjMarkBusinessSpanMention)
	emitted := map[string]bool{}
	for _, row := range detailRows {
		node := row.Node
		key := strings.TrimSpace(node.EvidenceID)
		if key != "" && emitted[key] {
			continue
		}
		emitted[key] = true
		if node.MergedCount > 1 {
			if runtimeTraceProjSubjectlessFoldRow(node) && runtimeTraceProjAllZeroFoldRow(node) {
				// DISP-2 G19: the note form, never the ×N取最大 claim.
				flags.allZeroFold = true
			} else if runtimeTraceProjSubjectlessFoldRow(node) {
				flags.mergedMax = true
			} else if node.MergedIntervalUnion {
				flags.mergedUnion = true
			} else if node.MergedCrossWindowMax {
				flags.mergedWindowMax = true
			} else {
				flags.mergedSum = true
			}
		}
		if node.DuplicatePublications > 1 {
			flags.mergedDedup = true
		}
		if runtimeTraceProjFamilyRow(node) {
			flags.family = true
		}
		if node.IsTargetSelfStateRow() ||
			(row.Kind == runtimeTraceProjTreeRowSelf && node.IsSleepState() &&
				node.EffectiveImpactMS > 0) {
			// DISP-3 G6-b: the gated legend line rides BOTH arms of the
			// attribution-cell dash (rank-lane tier token + chain-lane self
			// sleep rows). The chain-lane arm additionally requires a positive
			// effective — only then did the dash repair a printed value; a
			// value-less self sleep row keeps its legacy legend byte-stable.
			flags.selfSymptom = true
		}
		if runtimeTraceProjStanzaRowKind(row.Kind) && node.CumulativeImpactMS > 0 &&
			!runtimeTraceProjCrossThreadAggregateType(node) {
			// CMP-3 F6 carve-out mirrored: aggregate-metric rows keep their
			// annotated cells, so they never raise the dash's gated line.
			flags.stanzaChainTotal = true
		}
		if runtimeTraceProjGatedCompositeProjectionCell(node) {
			// GATED-CAL 件1③: same typed gate as the cell annotation (single
			// predicate — the line and the annotation can never fork).
			flags.gatedProjection = true
		}
		// SCORE-DERIV (§29.104.22.1): the formula-entry flags read the same
		// typed predicates the value word faces read (承诺面双向 by shared
		// gate, never a re-derivation).
		//
		// Flag asymmetry, deliberate (§29.123 P3③ 注记, DISPLAY-HYG 二轮):
		// scoreBlockIO keys on the caliber-side CLASS alone because
		// causalTokenCompositeScoreRows registers exactly one token today
		// (block_io_by_inode) and its ⌗ word face rides the class arm;
		// scoreIOPressure instead needs token + composite value caliber
		// because io_pressure deliberately keeps class==None (M18 键改名批,
		// ⌗ 降道无裁定) and its word face publishes on the Unit lane. If a
		// second CompositeScore-class producer ever registers, the block_io
		// entry TEXT must fork per token before the class-only key here can
		// stay an honest formula promise.
		canonical := runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)
		switch tracequery.CausalTokenCaliberSideClass(canonical) {
		case tracequery.CausalCaliberSideCompositeScore:
			flags.scoreBlockIO = true
		case tracequery.CausalCaliberSideCount:
			flags.countEquivalent = true
		}
		if canonical == "io_pressure" && runtimeTraceProjCompositeValueCaliber(node) {
			flags.scoreIOPressure = true
		}
		if strings.TrimSpace(node.FamilyFoldCaliber) == tracequery.RootCauseMemberFoldCaliberCountSum {
			flags.countEquivalent = true
			if runtimeTraceProjFamilyCountSumClamped(node) {
				flags.countClamp = true
			}
		}
		if key != "" && len(seats[key]) > 1 {
			flags.multiSeat = true
		}
	}
	return flags
}

// runtimeTraceProjDetailSeats builds the per-EvidenceID seat-glyph roster the
// dual-seat note reads (extracted so the table and the legend flags consume
// ONE implementation).
func runtimeTraceProjDetailSeats(detailRows []runtimeTraceProjTreeRow, zh bool) map[string][]string {
	seats := map[string][]string{}
	for _, row := range detailRows {
		key := strings.TrimSpace(row.Node.EvidenceID)
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
	return seats
}

func runtimeTraceProjDetailTable(model runtimeTraceProjTreeModel, zh bool) ([]string, []types.AnswerBlockItem) {
	// PTV8-RCR-B (UXA 域B #13, 2026-07-08). EVOLUTION RECORD: the E# printed
	// twice per row (node column [E1] + "E1 · 中" cell) — the last column now
	// carries the confidence tier only.
	columns := []string{"节点[E#]", "窗口投影", "链上累计", "有效归因", "实际状态", "置信"}
	if !zh {
		columns = []string{"Node [E#]", "Window projection", "Chain total", "Attribution", "Actual state", "Confidence"}
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
	seats := runtimeTraceProjDetailSeats(detailRows, zh)
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
			// ×N data token inline (T4 三式 + §11-N2 第四式 + §21-CWD 第五式);
			// the form semantics + member roster live in the (b) block and the
			// legend.
			if node.MicroAnchorFold {
				// RNB-5B 件⑦: the account-sum caliber mirrors the fence tag
				// (shared helper) — never the member-MAX claim.
				name += " " + runtimeTraceProjMicroAnchorFoldTagText(node, zh)
			} else if runtimeTraceProjSubjectlessFoldRow(node) && runtimeTraceProjAllZeroFoldRow(node) {
				// DISP-2 G19: the all-zero fold's honest note mirrors the fence
				// tag (shared helper) — never a member-MAX claim over zeros.
				name += " " + runtimeTraceProjAllZeroFoldNoteText(node, zh)
			} else if runtimeTraceProjSubjectlessFoldRow(node) {
				name += " " + runtimeTraceProjMergedMaxTagText(node, zh)
			} else if node.MergedIntervalUnion {
				name += " " + runtimeTraceProjMergedUnionTagText(node, zh)
			} else if node.MergedCrossWindowMax {
				name += " " + runtimeTraceProjMergedCrossWindowMaxTagText(node, zh)
			} else {
				name += " " + runtimeTraceProjMergedSumTagText(node, zh)
			}
		}
		if node.DuplicatePublications > 1 {
			name += " " + runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)
		}
		// RCM-2 D3 表面: the family contender's ×N token + caliber stem beside
		// the name (关键指标表 family 一行); the full caliber word, roster and
		// distinguishing keys live on the (b) family stanza.
		if token := runtimeTraceProjFamilyTableToken(node, zh); token != "" {
			name += " " + token
		} else if node.IsCaliberSideRow() &&
			tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount {
			// CR-3 修复轮 (冷读 F-CR3-9, 2026-07-12): a SINGLE-member
			// count-class ⌗ row keeps its 计数当量 marker on the table face
			// too — the ×N family form carried it inside the fold token
			// (×2计数当量), and the degenerate single row silently printed a
			// bare ms into the wall-clock columns (tieba E24 7.200ms).
			if zh {
				name += " 计数当量"
			} else {
				name += " count-equivalent"
			}
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
		// QH2-A 件2 站① (§29.55 观察③ 族裁延伸, 2026-07-14): a composite-score
		// ⌗ row's value cells never wear the wall-clock ms suit — the CR-3
		// count carve above only recognized CausalCaliberSideCount, so the
		// composite class (token 恰一 block_io_by_inode) printed bare ms into
		// the wall-clock columns. The cells adopt the roster/树行1
		// single-source form <value>(综合评分,非墙钟); zero/absent values keep
		// the dash, every non-composite row stays byte-identical.
		// RANKDIS-M18: value caliber is carried independently of board
		// placement. io_pressure remains context_only (not caliber_side) but
		// still publishes a composite score and must use the same value face.
		compositeCaliber := runtimeTraceProjCompositeValueCaliber(node)
		// RNB-5B 修复轮 U6/P3-⑦ (2026-07-15): the COUNT class joins the QH2-A
		// composite carve — a ⌗ count row's value cells wore the wall-clock ms
		// suit (17267 witness: | 81.616ms | ×3) while its 行1 and the ◎ ⌗
		// footnote speak suffix-free count-equivalent forms. Same single-source
		// value text as the roster/树行1 (计数当量X(非墙钟)).
		countCaliber := node.IsCaliberSideRow() &&
			tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount
		annotated := func(v float64) string {
			cell := msCell(v)
			if compositeCaliber && v > 0 {
				cell = runtimeTraceProjCompositeScoreValueText(v, zh)
			}
			if countCaliber && v > 0 {
				cell = runtimeTraceProjCountEquivalentValueText(v, zh)
			}
			return runtimeTraceProjDetailCrossThreadCell(cell, v, crossThread && !compositeCaliber, zh)
		}
		effective := annotated(node.EffectiveImpactMS)
		// 审计 #5 (§29.25/§29.26, 2026-07-10): an unfolded on-chain semantic
		// row carries no engine effective note, but its typed intersection IS
		// the engine's participation value — the cell mirrors the tree 行3's
		// dual-caliber claim instead of a dash beside it (三面同源).
		if node.EffectiveImpactMS <= 0 {
			if v := runtimeTraceProjSemanticChainIntersectionMS(node); v > 0 {
				effective = annotated(v)
			}
		}
		if runtimeTraceProjEffectiveInherited(node) {
			// PTV8-RCR-B (UXA 域B #28 verify, 2026-07-08): merge into one
			// paren group when the cross-thread annotation already opened one
			// ("(跨线程累计,非墙钟)(承自等待区间)" 版面 wart).
			if zh {
				if strings.HasSuffix(effective, ")") {
					effective = strings.TrimSuffix(effective, ")") + ";承自等待区间)"
				} else {
					effective += "(承自等待区间)"
				}
			} else {
				if strings.HasSuffix(effective, ")") {
					effective = strings.TrimSuffix(effective, ")") + "; inherited)"
				} else {
					effective += " (inherited)"
				}
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
		// GAP-B G6 (§27.3, real_trace_campaign_20260705.md, 2026-07-09): the
		// column's own definition is "计入根因排序的影响时长" — a WAIT-SYMPTOM
		// target-self row never seats on the rank board (SYM §24.13 裁定一), so
		// printing its engine effective here claimed a ranking contribution the
		// row does not make (huadong_79: 目标 sleep 行照印 6.357ms). Typed tier
		// gate only (IsTargetSelfStateRow): the engine mints target_self_state
		// EXCLUSIVELY on the 等待症状族 (SYM-2 §24.17 narrowed arm), so the
		// 自因四态 self rows (runnable/running/IO/D-state — they compete with
		// normal tiers and rank seats) keep their attribution value by
		// construction (勿一刀切, both directions pinned).
		//
		// DISP-3 G6-b (§29.8 P1, 2026-07-09): the tier gate only covers the
		// RANK lane — the wakeup_causal_impact lane's target-self sleep rows
		// carry no tier token and bypassed it (huadong_792 E1 printed 6.661ms;
		// cmp_792 E2/E3 + proj2 E3/E5). The second arm keys on the display's
		// own typed classification: a SelfRows-lane seat (label-routed
		// subject==target) whose dominant StateKind is the sleep family. It is
		// deliberately NARROWER than the tier arm — a tid-matched dual-name
		// chain row (cmp_792 proj2 E16, Kind=depthless) keeps its value: its
		// effective feeds the visible 承自 chain (E17 "13.054ms(承自等待区间)"
		// references it — E16 承自链需保), and self 自因族 rows (io/D/runnable/
		// running StateKinds) keep their values on this arm too.
		if node.IsTargetSelfStateRow() || node.IsContextOnlyRow() ||
			(row.Kind == runtimeTraceProjTreeRowSelf && node.IsSleepState()) {
			effective = dash
		}
		actual := annotated(node.ActualImpactMS)
		// RN-2b: no anchor window → no ⚠ (same gate as the tree tag).
		// CR-2 组③ P7 (2026-07-12): the cell mirrors the tree tag's typed
		// scope verdict — ⚠ only on a proven analysis-window crossing; an
		// in-window overshoot speaks the episode word; interval-less actuals
		// state so. F-5: a ×N merged row's cell value is the merge seed's
		// SINGLE member actual — disclosed regardless of scope (the column
		// definition promises 「该状态的真实完整时长」 and the fold row's
		// reader otherwise reads it as the family account).
		if model.WindowMS > 0 && node.ActualImpactMS > 0 {
			switch row.ActualScope {
			case runtimeTraceProjActualScopeAnalysisWindow:
				actual += " ⚠" + runtimeTraceProjActualMemberQualifier(node, zh)
			case runtimeTraceProjActualScopeEpisode:
				// 修复轮 P4-b: one paren for scope + member qualifier.
				if zh {
					actual += "(" + runtimeTraceProjActualScopeParen("超出发生段,窗内", node, true) + ")"
				} else {
					actual += " (" + runtimeTraceProjActualScopeParen("beyond own episode, inside window", node, false) + ")"
				}
			case runtimeTraceProjActualScopeNoInterval:
				if zh {
					actual += "(" + runtimeTraceProjActualScopeParen("区间未发布", node, true) + ")"
				} else {
					actual += " (" + runtimeTraceProjActualScopeParen("interval unpublished", node, false) + ")"
				}
			default:
				actual += runtimeTraceProjActualMemberQualifier(node, zh)
			}
		} else if node.ActualImpactMS > 0 && actual != dash {
			actual += runtimeTraceProjActualMemberQualifier(node, zh)
		}
		// PTV8-RCR-C (§24.9 维度A F4 / §24.11 C-4, 2026-07-08). EVOLUTION
		// RECORD: this column read node.Confidence while the tree 行2 and the
		// detail block read the fold-peer confidence — one merged row wore two
		// tiers on two faces. The three faces now share ONE source
		// (runtimeTraceProjCauseRankConfidence; rows without fold peers are
		// byte-identical).
		_, rowConfidence := runtimeTraceProjCauseRankConfidence(row)
		confidence := runtimeTraceProjConfidenceTier(rowConfidence, zh)
		if confidence == "" {
			confidence = dash
		}
		// DISP-2 G3 表列口径 (§27.2, 2026-07-09): the 链上累计 column's own
		// definition is "沿唤醒链向关注线程累计的投影时长" — a ◇/▒ stanza row
		// is off the wakeup chain (its seat kind IS the typed off-chain
		// verdict, the same kind-split ruling A applies on the tree face), so
		// printing its cumulative there claimed an on-chain accumulation the
		// row does not make (huadong_79 E11/E12/E13: adjacent count rows wore
		// 链上累计==窗口投影). The cell renders "—" (G6 precedent); the value
		// keeps its honest homes — the stanza row's 累计(跨线程) tag when it
		// differs from the projection, the 窗口投影 column when equal, and the
		// C00 fallback main-line value when no projection exists.
		// CMP-3 F6 carve-out: cross-thread AGGREGATE-METRIC rows keep their
		// annotated cells on ALL duration columns (customer compare audit
		// adjudication) — the (跨线程累计,非墙钟) annotation already denies
		// the on-chain wall-clock reading, so the dash has nothing to repair.
		chainTotal := annotated(node.CumulativeImpactMS)
		if runtimeTraceProjStanzaRowKind(row.Kind) && !crossThread {
			chainTotal = dash
		}
		// GATED-CAL 件1③ (§29.104.16.1 M3-c, 2026-07-16): an inversion seat
		// whose window-projection cell VALUE is the engine's gated composite
		// (query.go 覆写根 publishes ImpactMs = gated on inversion rank rows;
		// print-precision typed identity) violates the column's 「状态落在分析
		// 窗内的时长」 promise — the cell wears the composite annotation and
		// the legend's inversion-seat qualifier line rides the gated flags
		// (A2 witness E28: 窗口投影 3.429 == runnable 2.181 + running 折算
		// 1.248). Genuine state projections beside gated fields (E13-shape)
		// stay byte-identical.
		projection := annotated(node.ImpactMS)
		if runtimeTraceProjGatedCompositeProjectionCell(node) {
			// The word's teaching on THIS surface rides the table's own gated
			// legend line (runtimeTraceProjDetailTableLegendFlags.gatedProjection
			// — the tree's dynamic legend has already rendered by table time).
			if zh {
				projection += "(" + runtimeTraceProjGatedCompositeShortWord(true) + ")"
			} else {
				projection += " (" + runtimeTraceProjGatedCompositeShortWord(false) + ")"
			}
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				runtimeTraceCausalProjectionMarkdownSafe(name),
				projection, chainTotal,
				effective, actual,
				confidence,
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
		return tracefence.GlyphOptimization
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

// runtimeTraceProjUnresolvedFoldName (PTV8-RCR-B, UXA 域B #22 verify,
// 2026-07-08): a subjectless merged fold whose peers are unresolved used to
// wear THREE names (tree "其余 N 项合并(roster…)", table/block bare
// "对端线程未解析") — the table/block now share the tree's fold stem; the
// roster stays on the tree row (2026-07-03 customer ruling) and the ×N 明细
// line. "" for every other shape. P2a rider 件1 F2 (§29.58.2, 2026-07-13):
// the stem follows the stanza fold's dedup form 其余N项(折叠) — the carriers
// of this name are exactly the R3 stanza folds whose tree face changed.
func runtimeTraceProjUnresolvedFoldName(node types.TraceCausalProjectionNode, zh bool) string {
	if node.MergedCount <= 1 || node.OnChainOverflowFold ||
		strings.TrimSpace(node.Subject) != "" ||
		!runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		return ""
	}
	if zh {
		return fmt.Sprintf("其余 %d 项(折叠)(对端线程未解析)", node.MergedCount)
	}
	return fmt.Sprintf("%d more (folded) (peer threads unresolved)", node.MergedCount)
}

// runtimeTraceProjDetailFullName composes a node's FULL display name with NO
// cell caps (PTV4 T10 (b): the 28/22/36-rune CompactCellText caps are
// withdrawn on this surface — more lossless than the pre-T10 table).
func runtimeTraceProjDetailFullName(node types.TraceCausalProjectionNode, zh bool) string {
	if node.IsAggregateMetric() {
		return strings.TrimSpace(runtimeTraceCausalProjectionAggregateMetricName(node, zh))
	}
	if fold := runtimeTraceProjUnresolvedFoldName(node, zh); fold != "" {
		return fold
	}
	if blocking := runtimeTraceCausalProjectionBlockingName(node, zh); blocking != "" {
		if runtimeTraceCausalProjectionKnownSubject(node.Subject) {
			return strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh)) + " / " + blocking
		}
		return blocking
	}
	subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplaySubjectName(node, zh))
	object := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if runtimeTraceCausalProjectionSemanticSpanRow(node) &&
		strings.TrimSpace(node.SpanName) != "" {
		object = strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(node.SpanName, zh))
	} else if spanWord := runtimeTraceCausalProjectionSpanNameObjectWord(node, zh); spanWord != "" {
		// F1 (§22 PTV7-SPN P0): this surface promises 完整名称不截断 — a
		// generic span row's full name MUST carry the real span name
		// ("oney.hmn.berlin-42591 / H:ReceiveVsync(trace span)"), never only the
		// type word. Shared helper with the tree row and the (a) node cell.
		object = spanWord
	}
	// SEM-LEAD (§29.7-2 ④): a semantic FAMILY row's name is the typed class
	// word on this face too (词值同源 with the tree 行1 / (a) node cell) —
	// the member span names stay lossless on the roster stanza lines below.
	if runtimeTraceProjFamilyRow(node) && strings.TrimSpace(node.SemanticClass) != "" {
		if word := runtimeTraceProjFamilySemanticClassWord(node, zh); word != "" {
			object = word
		}
	}
	switch {
	case subject != "" && object != "":
		return subject + " / " + object
	case subject != "":
		return subject
	case object != "":
		return object
	default:
		// PTS (复核 Low, 2026-07-06): the overflow fold row's lossless block
		// names its count + roster — never the anonymous fallback. P2a rider
		// 件1 (§29.55.3, 2026-07-13): lockstep with the tree row-name mint —
		// 其余N项(折叠); the lane lives on the tree face's edge word and this
		// block's own 因果位置 line.
		if node.MicroAnchorFold {
			// RNB-5B 件⑦: the detail block names the micro-fold family with
			// its full member roster (lossless inventory face).
			return runtimeTraceProjMicroAnchorFoldName(node, zh) + runtimeTraceProjMergedSubjectsSuffix(node, zh)
		}
		if node.OnChainOverflowFold {
			if zh {
				return fmt.Sprintf("其余 %d 项(折叠)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
			}
			return fmt.Sprintf("%d more (folded)%s", node.MergedCount, runtimeTraceProjMergedSubjectsSuffix(node, zh))
		}
		// Catalog B9 (DISPLAY-HYG 二轮, §29.104.18.1, 2026-07-17): a
		// subject/object-less node that still carries a member preview names
		// itself by its members ("keva-3-17439、… 等 5 线程") instead of the
		// opaque placeholder — the reader can locate the threads. Nodes
		// without any preview keep the placeholder (absence never invents).
		if preview := runtimeTraceProjAnonymousNodePreviewName(node, zh); preview != "" {
			return preview
		}
		// PTV5 C39 (#68): the zh panel's subject/object-less fallback speaks
		// zh; the EN face keeps the neutral phrase.
		if zh {
			return "(未命名因果节点)"
		}
		return "trace causal node"
	}
}

// runtimeTraceProjAnonymousNodePreviewName — catalog B9 (DISPLAY-HYG 二轮):
// the member-preview name for a subject/object-less node. Built from the
// SAME MergedSubjects roster the fold suffix reads; "" when no preview
// exists (the caller keeps its placeholder — never fabricated).
func runtimeTraceProjAnonymousNodePreviewName(node types.TraceCausalProjectionNode, zh bool) string {
	if len(node.MergedSubjects) == 0 {
		return ""
	}
	if zh {
		name := strings.Join(node.MergedSubjects, "、")
		if node.MergedCount > len(node.MergedSubjects) {
			return fmt.Sprintf("%s 等 %d 线程", name, node.MergedCount)
		}
		if len(node.MergedSubjects) > 1 {
			return fmt.Sprintf("%s 共%d线程", name, len(node.MergedSubjects))
		}
		return name
	}
	name := strings.Join(node.MergedSubjects, ", ")
	if node.MergedCount > len(node.MergedSubjects) {
		return fmt.Sprintf("%s, … (%d threads)", name, node.MergedCount)
	}
	if len(node.MergedSubjects) > 1 {
		return fmt.Sprintf("%s (%d threads)", name, len(node.MergedSubjects))
	}
	return name
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
	// PTV8-RCR-B (UXA 域B #23, 2026-07-08): blocks whose rendered name AND
	// field set are byte-equal merge into ONE block with the evidence numbers
	// side by side ("**[E1] [E2] name**") — the merge key is exact rendered
	// bytes (precise signal; any differing field keeps separate blocks), and
	// only tagged blocks merge (the evidence roster keeps every E# entry).
	type detailStanza struct {
		tags []string
		name string
		body string
	}
	var ordered []*detailStanza
	index := map[string]*detailStanza{}
	for _, row := range runtimeTraceProjDetailRows(model) {
		node := row.Node
		tag := strings.TrimSpace(row.EvidenceTag)
		name := runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjDetailFullName(node, zh))
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
		// PTV8-RCR-B (UXA 域B 漏审 S1, 2026-07-08). EVOLUTION RECORD: the
		// 完整名称 line was byte-identical to the block heading (same
		// runtimeTraceProjDetailFullName, no truncation on either) — deleted;
		// the heading carries the lossless-name promise.
		// CR-3 件③ P11 (2026-07-12, 冷读案8 裸线程名死指针): the process
		// attribution slot — the trace-published tgid (+ owning process comm
		// when resolved) directly under the unchanged identity heading, so a
		// bare thread name is always traceable to its process. Engine-minted
		// only; a node without the typed pair renders nothing.
		add("进程", "process", runtimeTraceProjDetailProcessCell(node))
		add("层级", "layer", runtimeTraceProjDetailLayerCell(row, zh, flat))
		add("因果位置", "causal position", runtimeTraceProjDetailPositionCell(row, model.LeadKey, zh))
		typeToken := runtimeTraceCausalProjectionRawTypeToken(node)
		add("类型", "type", runtimeTraceCausalProjectionMarkdownSafe(typeToken))
		// PTV8-RCR-B (UXA 域B #17, 2026-07-08). EVOLUTION RECORD: the
		// 「关系 ▸ 影响点」 composite line is split — ▸ doubled as field
		// separator and direction arrow (and degraded to "?" on the customer's
		// terminal); each field is its own keyed line and the relation speaks
		// a full clause (word tables in runtimeTraceProjDetailRelationCell).
		add("关系", "relation", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjDetailRelationCell(row, zh, flat)))
		if len(node.SecondaryObjects) > 0 {
			// PTV6-C #6: same D4 中文（token） display as the tree tag (single
			// helper); the raw tokens stay lossless on the 类型 column and the
			// raw observation record. Full roster here (the tree tag may
			// suppress bare state tokens; this surface never does).
			display := make([]string, 0, len(node.SecondaryObjects))
			for _, token := range node.SecondaryObjects {
				display = append(display, runtimeTraceCausalProjectionImpactPointDisplay(token, zh))
			}
			joined := strings.Join(display, " / ")
			add("影响点", "impact points", runtimeTraceCausalProjectionMarkdownSafe(joined))
		}
		// PTV6-C ruling B (#73): the inversion cell now speaks the cause full
		// word (优先级反转候选) — 反转影响 is deleted; the D3 composition keeps
		// its single carrier on the fence tag (never elided there).
		// PTV8-RCR-C (§24.12 C7, 2026-07-08). EVOLUTION RECORD: the generic
		// arm claimed 未分类(该行无具体状态/类型词) beside 类型: binder_wait —
		// a typed-family row now takes its §24.3 family word (两列单源表;
		// genuinely word-less rows keep the generic arm byte-identically).
		shape, genericShape := runtimeTraceCausalProjectionImpactShapeCellTyped(node, zh)
		if genericShape {
			if family := runtimeTraceProjImpactFormFamilyWord(node, zh); family != "" {
				shape = family
			}
		}
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
		// PTV8-RCR-A (§24 ②): on inversion nodes the 机制构成 mechanism
		// SENTENCE branches are retired — the 有效归因构成/拆解/供给折算 lines
		// below carry the same data in the four-line grammar's wording;
		// non-sentence verdicts mirror unchanged.
		detailVerdict := runtimeTraceProjSupplyFoldVerdictFor(node, model.WindowMS)
		detailMechanism := detailVerdict == runtimeTraceProjSupplyFoldTriple ||
			detailVerdict == runtimeTraceProjSupplyFoldWithDemand
		if !(runtimeTraceCausalProjectionInversionRow(node) && detailMechanism) {
			if clause, _, ok := runtimeTraceProjSupplyFoldClause(node, model.WindowMS, zh); ok {
				if shape == "" {
					shape = clause
				} else {
					shape += "·" + clause
				}
			}
		}
		add("影响形态", "impact shape", shape)
		// RNB-5B 修复轮 U3 (席位记忆, 2026-07-15): the micro anchored-seat fold
		// row remembers its folded members' ordinal range on the detail face
		// (诚实加性 — the ordinals were engine-published and their rows no
		// longer render individually; the fold row is not a cause node, so the
		// seat section below never speaks for them). All-ranked folds only
		// (宁缺勿假). Channel word from the ONE shared source (the fold rides
		// the ⛓ chain channel by construction).
		if row.Node.MicroAnchorFold && row.MicroAnchorFoldRankLo > 0 {
			channel := runtimeTraceProjRowOrdinalChannel(row)
			if wordZH, ok := runtimeTraceProjSeatChannelWord(channel, true); ok {
				wordEN, _ := runtimeTraceProjSeatChannelWord(channel, false)
				// 修补轮 件F (2026-07-16): the folded ordinal range carries the
				// same stamped board/window chip as every other seat face — a
				// bare 「#4~#6」 on a multi-board report is exactly the ordinal
				// collision the chip exists for. Single-board reports stamp no
				// chip and stay byte-identical.
				chip := ""
				if c := strings.TrimSpace(row.RankWindowChip); c != "" {
					if zh {
						chip = "·" + c
					} else {
						chip = " · " + c
					}
				}
				if zh {
					add(wordZH, wordEN, fmt.Sprintf("#%d~#%d(折叠合一)%s", row.MicroAnchorFoldRankLo, row.MicroAnchorFoldRankHi, chip))
				} else {
					add(wordZH, wordEN, fmt.Sprintf("#%d~#%d (folded into one)%s", row.MicroAnchorFoldRankLo, row.MicroAnchorFoldRankHi, chip))
				}
			}
		}
		// PTV8-RCR-A (§24.1/§24.2): the cause-node grammar's lossless mirror —
		// 行2 榜位/置信 (with the folded rank twin's E#), 行3 breakdown, the
		// 拆解子行 and the inversion node's supply-fold deficit line (the
		// retired Triple sentence's data half, unified sub-row grammar).
		if runtimeTraceProjCauseNodeRow(row) {
			rank, confidence := runtimeTraceProjCauseRankConfidence(row)
			// UXR-1 (§29.36.2, 2026-07-11): the detail seat line follows the
			// ordinal CHANNEL — 根因排序 label on the chain channel, 邻近影响 on
			// the ◇ adjacent channel, and NO seat line on the ▒ background
			// channel (通道3 无序数; stale Rank falls to the bare 置信 form).
			// 复核 P2-3: a SeatOrdinalStale ordinal (beyond its channel's
			// rendered population) falls to the bare 置信 form the same way.
			// 复核 P3-⑦: the label word comes from the ONE channel-word source
			// shared with the fence chip (runtimeTraceProjSeatChannelWord).
			channel := runtimeTraceProjRowOrdinalChannel(row)
			channelWordZH, channelWorded := runtimeTraceProjSeatChannelWord(channel, true)
			channelWordEN, _ := runtimeTraceProjSeatChannelWord(channel, false)
			seated := rank > 0 && channelWorded && !row.SeatOrdinalStale
			var seat []string
			if seated {
				// §29.27.1 ③ (三面记号一致): the detail face inlines the seat's
				// ❶..❺ glyph from the single token source, gated by the row's
				// Badge (复核 C-1 — same negative gate as the tree face).
				seat = append(seat, runtimeTraceProjSeatOrdinalToken(rank, row.Badge))
				// §24.13 裁定二后半: the multi-board window tag rides the seat
				// ordinal on this face too (same stamped chip as the tree 行2).
				if chip := strings.TrimSpace(row.RankWindowChip); chip != "" {
					seat = append(seat, chip)
				}
			}
			if tier := runtimeTraceProjConfidenceTier(confidence, zh); tier != "" {
				switch {
				case !seated:
					// PTV8-RCR-C (§24.12 C11 明细空榜位字段形, 2026-07-08).
					// EVOLUTION RECORD: an unseated cause node rendered
					// 「根因排序: 置信中」 — a rank label over a bare confidence
					// tier. The bare tier now rides its own 置信 label below.
					seat = append(seat, tier)
				case zh:
					seat = append(seat, "置信"+tier)
				default:
					seat = append(seat, "confidence "+tier)
				}
			}
			for _, peer := range row.RankFoldPeers {
				if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
					if zh {
						seat = append(seat, "rank行["+tag+"]已并入本行,数值不重复计入")
					} else {
						seat = append(seat, "rank row ["+tag+"] folded into this row; no value double-counted")
					}
				}
			}
			if len(seat) > 0 {
				sep := "·"
				if !zh {
					sep = " · "
				}
				switch {
				case seated:
					// 复核 P3-⑦: label = the shared channel word (one source,
					// two faces — the fence chip prints the same word + #N).
					add(channelWordZH, channelWordEN, strings.Join(seat, sep))
				default:
					add("置信", "confidence", strings.Join(seat, sep))
				}
			}
			if runtimeTraceCausalProjectionInversionRow(node) {
				// 复核 FAIL-2: the SAME shared equation/sub-row templates the
				// fence 行3 renders — a private copy here could drift with
				// every pin green. 复核 F4: the degenerate single-full shape
				// mirrors the two-line form (no equation, no 拆解 line — the
				// (a) table + 行2 tail already carry the value).
				if components, total, ok := runtimeTraceProjInversionComponents(node, zh); ok {
					if !runtimeTraceProjInversionDegenerateSingleFull(components) {
						add("有效归因构成", "attribution makeup",
							runtimeTraceProjAttributionEquation(total, components))
						sep := ";"
						if !zh {
							sep = "; "
						}
						add("拆解", "breakdown", strings.Join(runtimeTraceProjAttributionSubRows(components, zh), sep))
					}
				} else if node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0 {
					// Fail-open shape: the composition data stays lossless via
					// the single-source component text (no retired wording).
					add("有效归因构成", "attribution makeup",
						runtimeTraceProjInversionCompositionText(node, zh))
				}
				add("供给折算", "supply fold", runtimeTraceProjInversionSupplyFoldDetailLine(node, zh))
			}
		}
		// 审计 #62 ① (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the
		// on-chain semantic dual-caliber stanza line — the SAME single-source
		// word/disclosure helpers as the fence 行3 (两面同源), with the
		// stanza's own 供对照 pointer convention.
		if intersection, dual := runtimeTraceProjSemanticChainDualCaliber(node); dual {
			add("链上计入", "on-chain counted",
				runtimeTraceCausalProjectionMarkdownSafe(fmt.Sprintf("%s;%s%s",
					runtimeTraceProjFmtMS(intersection),
					runtimeTraceProjSemanticChainIntersectionWord(node, zh),
					runtimeTraceProjSemanticChainUnionDisclosure(node, zh, true))))
		}
		// RCM-2 D3 明细面 (§24.7.1 ① 区分键不能丢 + roster 全量在明细): the
		// family stanza — fold caliber word (+ raw-Σ disclosure), the FULL wire
		// roster with its counted account, the typed inode/dev distinguishing
		// keys and the family's own typed query window.
		if runtimeTraceProjFamilyRow(node) {
			caliber, _, ok := runtimeTraceProjFamilyCaliberWord(node, zh)
			if !ok {
				// Unknown caliber token: verbatim raw token (never fabricated).
				caliber = strings.TrimSpace(node.FamilyFoldCaliber)
			}
			// 复核 F-2 (2026-07-08): the raw-Σ note comes from the SINGLE
			// caliber-forked source (runtimeTraceProjFamilySumDetailNote) — the
			// hand-written copy here glued the union clause 「重叠段已并」 onto
			// the max arm's 「重叠未拆」 caliber word (one line, two
			// contradictory overlap claims).
			caliber += runtimeTraceProjFamilySumDetailNote(node, zh)
			if node.FamilyMemberMaxMS > 0 {
				// DISPLAY-WRAP 件④(c) (§29.104.18.1 A6, 2026-07-16): a COUNT
				// class family's member range never wears the wall-clock ms
				// suit (the legend promises 不带 ms 后缀; witness E46
				// 「单段 34.800~84.300ms」 beside its own count-equivalent
				// 行1) — the range adopts the 计数当量X(非墙钟) form family.
				countRange := node.IsCaliberSideRow() &&
					tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount
				switch {
				case countRange && zh:
					caliber += fmt.Sprintf(";单段 计数当量%.3f~%.3f(非墙钟)", node.FamilyMemberMinMS, node.FamilyMemberMaxMS)
				case countRange:
					caliber += fmt.Sprintf("; each count-equivalent %.3f~%.3f (not wall clock)", node.FamilyMemberMinMS, node.FamilyMemberMaxMS)
				case zh:
					caliber += fmt.Sprintf(";单段 %.3f~%.3fms", node.FamilyMemberMinMS, node.FamilyMemberMaxMS)
				default:
					caliber += fmt.Sprintf("; each %.3f~%.3fms", node.FamilyMemberMinMS, node.FamilyMemberMaxMS)
				}
			}
			add("家族合并", "family fold", runtimeTraceCausalProjectionMarkdownSafe(caliber))
			if len(node.FamilyMemberRoster) > 0 {
				sep := ";"
				if !zh {
					sep = "; "
				}
				// SPANTOP-1 件3 (§29.131, 2026-07-18): the stanza is the tree
				// top-3's 全清单 counterpart — when the typed line-range set is
				// complete AND aligned with the listed roster, every member
				// entry carries its own 行a..b locator (strict all-or-nothing:
				// a partial or misaligned set annotates nothing, the bare
				// roster stays byte-identical).
				entries := node.FamilyMemberRoster
				if len(node.FamilyMemberLineRanges) == node.FamilyMemberCount &&
					len(entries) == node.FamilyMemberCount {
					annotated := make([]string, 0, len(entries))
					for i, entry := range entries {
						if zh {
							annotated = append(annotated, fmt.Sprintf("%s(行%d..%d)", entry,
								node.FamilyMemberLineRanges[i][0], node.FamilyMemberLineRanges[i][1]))
						} else {
							annotated = append(annotated, fmt.Sprintf("%s (lines %d..%d)", entry,
								node.FamilyMemberLineRanges[i][0], node.FamilyMemberLineRanges[i][1]))
						}
					}
					entries = annotated
				}
				roster := strings.Join(entries, sep)
				if zh {
					roster = fmt.Sprintf("(共%d,列%d)%s", node.FamilyMemberCount, len(node.FamilyMemberRoster), roster)
				} else {
					roster = fmt.Sprintf("(%d total, %d listed) %s", node.FamilyMemberCount, len(node.FamilyMemberRoster), roster)
				}
				// Catalog A5 + C12 (DISPLAY-HYG 二轮): the stanza's member field
				// wears the SAME chips as the tree 行4+ roster rows (one chip
				// authority per chip — full-window account / verbatim span;
				// plain rows keep the bare 成员/members key byte-identically).
				add("成员"+runtimeTraceProjFamilyMemberFullWindowChip(node, true)+runtimeTraceProjFamilyMemberVerbatimSpanChip(node, true),
					"members"+runtimeTraceProjFamilyMemberFullWindowChip(node, false)+runtimeTraceProjFamilyMemberVerbatimSpanChip(node, false),
					runtimeTraceCausalProjectionMarkdownSafe(roster))
			}
			var keys []string
			if inode := strings.TrimSpace(node.Inode); inode != "" {
				keys = append(keys, "inode="+inode)
			}
			if dev := strings.TrimSpace(node.Dev); dev != "" {
				keys = append(keys, "dev="+dev)
			}
			if len(keys) > 0 {
				add("区分键", "distinguishing keys", runtimeTraceCausalProjectionMarkdownSafe(strings.Join(keys, " ")))
			}
			if node.QueryWindowStartTs > 0 && node.QueryWindowEndTs > node.QueryWindowStartTs {
				add("家族窗", "family window", fmt.Sprintf("%.3f~%.3fs", node.QueryWindowStartTs, node.QueryWindowEndTs))
			}
		}
		// G1 跨车道对账 (§27.2-G1, 2026-07-09): the 链上并入 disclosure — the
		// absorbed chain-lane observations' E# stay citable here (信息守恒:
		// their values are INSIDE this row's combined account by the engine's
		// membership proof, so the note carries identity only, never a second
		// ms). Bounded roster with a counted overflow (§24.7.1 ① 折叠必带计数
		// 披露). 收尾 P2-b (对抗复核, 2026-07-09): the note sits OUTSIDE the
		// family-row arm above — when the family contender itself merged into
		// an R2 ×N row, the carrier keeps ONLY RankFamilyKey (the F-1 chimera
		// clear wipes the family grammar fields), so a family-arm-gated note
		// would be unreachable exactly where the attach still lands. Rendering
		// keys solely on the attached peers, which only an engine-stamped
		// RankFamilyKey match can produce.
		if len(row.AbsorbedChainPeers) > 0 {
			add("链上并入", "chain-lane absorbed",
				runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjAbsorbedChainNote(row.AbsorbedChainPeers, zh)))
		}
		if node.MergedCount > 1 {
			var form string
			if node.MicroAnchorFold {
				// RNB-5B 件⑦: the (b) lossless mirror of the account-sum caliber
				// (三面同词 with the fence tag and the (a) token).
				form = runtimeTraceProjMicroAnchorFoldTagText(node, zh)
			} else if runtimeTraceProjSubjectlessFoldRow(node) && runtimeTraceProjAllZeroFoldRow(node) {
				// DISP-2 G19 (§27.5, 2026-07-09): the all-zero fold's lossless
				// mirror — no member-MAX claim over zeros (三面同词 with the
				// fence note and the (a) token).
				qualifier := ""
				if node.MergedAllDataGap {
					qualifier = "(数据盲区)"
					if !zh {
						qualifier = " (data blind spots)"
					}
				}
				form = fmt.Sprintf("%d线程折叠,成员窗内均无可计量时长%s,不作取最大声明", node.MergedCount, qualifier)
				if !zh {
					form = fmt.Sprintf("%d-thread fold; no member carries a measurable in-window duration%s, so no member-MAX claim is made", node.MergedCount, qualifier)
				}
			} else if runtimeTraceProjSubjectlessFoldRow(node) {
				// PTV8-RCR-B (UXA 域B #20, 2026-07-08). EVOLUTION RECORD:
				// 「取最大口径…不求和」→ canonical 墙钟跨线程不可加和 (三面同词).
				// G12-ENG (§29.1, 2026-07-09): the MIXED fold binds 「各 a~b ms」
				// to the VALUED members only — the legacy form claimed the range
				// over every member and fabricated the huadong_79 E23 same-value
				// double (one real 14.272ms member beside a zero-duration
				// blocked_reason aggregate read as ×2 both-at-14.272ms).
				if valued, valueless, mixed := runtimeTraceProjMergedValuedSplit(node); mixed {
					form = fmt.Sprintf("%d线程取最大(墙钟跨线程不可加和),有值%d项各 %s,另%d项无时长值", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
					if !zh {
						form = fmt.Sprintf("%d-thread max (wall clock never sums across threads), %d valued member(s) each %s, %d without measurable duration", node.MergedCount, valued, runtimeTraceProjMergedRangeText(node), valueless)
					}
				} else {
					form = fmt.Sprintf("%d线程取最大(墙钟跨线程不可加和),各 %.3f~%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
					if !zh {
						form = fmt.Sprintf("%d-thread max (wall clock never sums across threads), each %.3f~%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
					}
				}
			} else if node.MergedIntervalUnion {
				// §11-N2: the union caliber discloses itself + the lossless raw
				// Σ for cross-checking; K = the distinct member query windows.
				// G12-ENG 复核 P2-1: the per-instance segment rides the shared
				// valued-split helper — the fence tag and this (b) line must
				// never contradict on one mixed row.
				k := len(node.MergedQueryWindows)
				form = fmt.Sprintf("%d次union口径(%d 窗重叠段不重复计),原始和 %.3fms 供对照,%s", node.MergedCount, k, node.MergedSumMS, runtimeTraceProjMergedPerInstanceText(node, zh))
				if !zh {
					form = fmt.Sprintf("n=%d union caliber (overlap across %d windows counted once), raw sum %.3fms for cross-checking, %s", node.MergedCount, k, node.MergedSumMS, runtimeTraceProjMergedPerInstanceText(node, zh))
				}
			} else if node.MergedCrossWindowMax {
				// §21 CWD: the cross-window MAX caliber discloses itself, the
				// max member's own window base and the lossless raw Σ.
				// G12-ENG 复核 P2-1: per-instance segment via the shared
				// valued-split helper (同上).
				k := len(node.MergedQueryWindows)
				form = fmt.Sprintf("%d次跨窗取最大(%d 个查询窗互相重叠,互相重叠的查询窗量值不可求和),原始和 %.3fms 供对照,%s", node.MergedCount, k, node.MergedSumMS, runtimeTraceProjMergedPerInstanceText(node, zh))
				if !zh {
					form = fmt.Sprintf("n=%d cross-window MAX caliber (%d overlapping windows; overlapping-window magnitudes never sum), raw sum %.3fms for cross-checking, %s", node.MergedCount, k, node.MergedSumMS, runtimeTraceProjMergedPerInstanceText(node, zh))
				}
				if node.MergedMaxWindowStartTs > 0 && node.MergedMaxWindowEndTs > node.MergedMaxWindowStartTs {
					if zh {
						form += fmt.Sprintf(";最大成员窗基=查询窗 %.3f~%.3fs", node.MergedMaxWindowStartTs, node.MergedMaxWindowEndTs)
					} else {
						form += fmt.Sprintf("; max-member window base = query window %.3f~%.3fs", node.MergedMaxWindowStartTs, node.MergedMaxWindowEndTs)
					}
				}
			} else if runtimeTraceProjMergedAllValueless(node) {
				// G12-ENG 复核 P2-2: the standalone all-zero R2 row (the hmfs ×4
				// blocked_reason shape when it does NOT overflow) — no SUM claim
				// and no 0.000 pseudo-range over marker rows.
				form = fmt.Sprintf("同一线程 %d 次实例合并,全部无时长值", node.MergedCount)
				if !zh {
					form = fmt.Sprintf("%d instances of one thread merged, all without measurable duration", node.MergedCount)
				}
			} else if row.FamilyMirrorRef != "" {
				// CR-2 组② P5 family arm (F-1 残口, donghu E8/E9): the merged
				// twin's members are the family row's per-CPU group SUMS —
				// 「单次 a~b」 claimed segments that do not exist (the reader
				// would hunt a 16ms stall that is really 11 × 2–4ms waits).
				// The range keeps its own values under the group-sum caliber
				// word, and the family's true single-segment extrema ride in
				// (段 inventory 传播).
				form = fmt.Sprintf("同一线程 %d 次实例合并求和,各成员 %.3f~%.3fms(按CPU分组合计,非单段;单段真值 %.3f~%.3fms 见家族行[%s]明细)",
					node.MergedCount, node.MergedMinMS, node.MergedMaxMS,
					row.FamilyMirrorSegMin, row.FamilyMirrorSegMax, row.FamilyMirrorRef)
				if !zh {
					form = fmt.Sprintf("%d instances of one thread merged as a SUM, members %.3f~%.3fms each (per-CPU group sums, not single segments; true single-segment range %.3f~%.3fms — see family row [%s])",
						node.MergedCount, node.MergedMinMS, node.MergedMaxMS,
						row.FamilyMirrorSegMin, row.FamilyMirrorSegMax, row.FamilyMirrorRef)
				}
			} else {
				// PTV8-RCR-B (UXA 域B #19, 2026-07-08). EVOLUTION RECORD:
				// 「求和口径」→ 客户话「同一线程 N 次实例合并求和」.
				// G12-ENG 复核 P2-2: the per-instance segment rides the shared
				// valued-split helper (mixed rows disclose their valueless part).
				form = fmt.Sprintf("同一线程 %d 次实例合并求和,%s", node.MergedCount, runtimeTraceProjMergedPerInstanceText(node, zh))
				if !zh {
					form = fmt.Sprintf("%d instances of one thread merged as a SUM, %s", node.MergedCount, runtimeTraceProjMergedPerInstanceText(node, zh))
				}
			}
			// PTV8-RCR-B (UXA 域B #19/#20, 2026-07-08): a roster that is
			// exactly the row's own subject says nothing (and the trailing 等
			// falsely implied unlisted members) — suppressed; a truncated
			// roster now states its own account (共N,列K).
			rosterSelfOnly := len(node.MergedSubjects) == 1 &&
				strings.TrimSpace(node.MergedSubjects[0]) == strings.TrimSpace(node.Subject)
			if len(node.MergedSubjects) > 0 && !rosterSelfOnly {
				sep := "、"
				if !zh {
					sep = ", "
				}
				roster := strings.Join(node.MergedSubjects, sep)
				if node.MergedCount > len(node.MergedSubjects) {
					if zh {
						form += fmt.Sprintf(";成员(共%d,列%d): %s 等", node.MergedCount, len(node.MergedSubjects), roster)
					} else {
						form += fmt.Sprintf("; members (%d total, %d listed): %s, …", node.MergedCount, len(node.MergedSubjects), roster)
					}
				} else if zh {
					form += ";成员: " + roster
				} else {
					form += "; members: " + roster
				}
			}
			add("合并明细", "merge detail", runtimeTraceCausalProjectionMarkdownSafe(form))
			// §11-N2 窗身份 (联动 q1-B6): the union row's member query windows,
			// ascending. Gated on the union caliber; §21 CWD added the
			// cross-window MAX caliber. EVOLUTION RECORD (§21.1 CWD-2 ①,
			// huadong E19 witness, 2026-07-07 — supersedes the former "the
			// disjoint cross-window roster disclosure belongs to the q1-B6
			// window-identity batch" deferral): a multi-window plain-SUM row
			// now suppresses its anchor-window share on the %-face, so THIS
			// roster is the reader's base disclosure and renders for every
			// merged row whose members span >1 known query windows.
			// Single-window and windowless SUM renders stay byte-identical.
			// The typed roster lists the KNOWN sources only — never claimed
			// exhaustive.
			if (node.MergedIntervalUnion || node.MergedCrossWindowMax || len(node.MergedQueryWindows) > 1) &&
				len(node.MergedQueryWindows) > 0 {
				windows := make([]string, 0, len(node.MergedQueryWindows))
				for _, w := range node.MergedQueryWindows {
					windows = append(windows, fmt.Sprintf("%.3f~%.3fs", w.StartTs, w.EndTs))
				}
				sep := "、"
				if !zh {
					sep = ", "
				}
				add("窗来源", "window sources", strings.Join(windows, sep))
			}
		}
		// CR-2 组② P5 member arm (legacy lane): the folded raw-state mirror copies stay
		// reachable — the lossless block names each absorbed E# explicitly.
		if len(row.SameSegMirrorPeers) > 0 {
			tags := make([]string, 0, len(row.SameSegMirrorPeers))
			anyValueless := false
			for _, peer := range row.SameSegMirrorPeers {
				if peer.Valueless {
					anyValueless = true
				}
				if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" {
					tags = append(tags, "["+tag+"]")
				}
			}
			if len(tags) > 0 {
				// 修复轮 P4-c: a valueless mirror never claims 同值 — the
				// wording says 同段(无独立值) instead.
				word, wordEN := "同段同值", "same-segment same-value"
				if anyValueless {
					word, wordEN = "同段(无独立值)", "same-segment (no independent value)"
				}
				line := fmt.Sprintf("裸状态车道%s镜像 %s 已并入本行,数值不重复计入", word, strings.Join(tags, " "))
				if !zh {
					line = fmt.Sprintf("raw-state lane %s mirror %s merged into this row; the value is never double-counted", wordEN, strings.Join(tags, " "))
				}
				add("同段合并", "same-seg merge", line)
			}
		}
		if node.DuplicatePublications > 1 {
			dup := fmt.Sprintf("%d次同值(同一测量被重复发布,数值为单次测量)", node.DuplicatePublications)
			if !zh {
				dup = fmt.Sprintf("n=%d same-value (one measurement republished; the value is that single measurement)", node.DuplicatePublications)
			}
			add("重复发布", "duplicate publications", dup)
		}
		if len(row.IOFoldPeers) > 0 {
			add("同段IO口径", "same-segment IO calibers", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh)))
		}
		// PTV8-RCR-A (§24.2). EVOLUTION RECORD: the RNB R2 同段rank行 mirror
		// line is RETIRED — the folded rank row's seat/confidence/E# live on
		// the 根因排序 line above (无损块不丢 rank 行, wording renewed).
		if runtimeTraceProjEffectiveInherited(node) {
			inherited := fmt.Sprintf("有效归因 %.3fms 承自等待区间,非本行实测", node.EffectiveImpactMS)
			if !zh {
				inherited = fmt.Sprintf("attribution %.3fms inherited from the wait interval, not measured on this row", node.EffectiveImpactMS)
			}
			// §21 CWD (cmp_01 revisit 2026-07-07, P2 E29): the inherited
			// magnitude names its window base — the specimen's 2994.269ms came
			// from a 150ms-window wait interval, ~19× this row's own window
			// projection, with no base named. Typed sources only: the row's
			// own query window when it carries one, else the merged member
			// roster (known sources); no identity → no label, never a guess.
			inherited += runtimeTraceProjInheritedWindowBaseSuffix(node, zh)
			add("承自注", "inherited note", inherited)
		}
		// DIAG A2 (§28.11-3(b) D-10, 2026-07-09): the two-caliber actual
		// disclosure — the row's ⚠实际 face carries the STATE-SEGMENT actual
		// while the raw record's actual_total is the THREAD-LEVEL total; when
		// the producer stamped the typed divergence note (>10% apart), this
		// line states BOTH sources so the two faces stop reading as a
		// contradiction. Gate = the typed enum note + both values present
		// (fail-safe: a missing half renders nothing); neither value is
		// judged or edited (不猜哪个对).
		if node.ActualCaliberNote == types.TraceActualCaliberStateSegmentVsThreadTotal &&
			node.ActualImpactMS > 0 && node.ActualTotalMS > 0 {
			caliber := fmt.Sprintf("状态段 %.3fms/线程合计 %.3fms(两口径,来源不同)", node.ActualImpactMS, node.ActualTotalMS)
			if !zh {
				caliber = fmt.Sprintf("state segment %.3fms / thread-level total %.3fms (two calibers, different sources)", node.ActualImpactMS, node.ActualTotalMS)
			}
			add("实际口径", "actual calibers", caliber)
		}
		// XERR1-FIX 件1/件3 (§29.104.3/.4) + XERR1-EXT 裁定⑤ (§29.104.17): the
		// blocking_span value-basis disclosure + the budget-sanity ⚠ line on
		// the lossless detail face — BOTH payload lanes (the payload-typed
		// row's value converged too; its lock word family stays untouched
		// elsewhere). Typed gates only; absence renders nothing. On a
		// holder-subject rank record the account describes the WAITER (the
		// record's peer) — the 件G② referent discipline applies per lane.
		if basis := strings.TrimSpace(node.BlockingValueBasis); basis != "" {
			text := ""
			switch basis {
			case tracequery.BlockingValueBasisWaitSegments:
				text = fmt.Sprintf("等待段合计(span∩窗内 sleep+D+iowait=%.3fms;span 包络 %.3fms 含运行,非阻塞等待值)",
					node.BlockingWaitSegmentMS, node.BlockingSpanEnvelopeMS)
				if !zh {
					text = fmt.Sprintf("wait-segment total (sleep+D+iowait inside span∩window = %.3fms; the span envelope %.3fms contains run time and is not a blocking-wait value)",
						node.BlockingWaitSegmentMS, node.BlockingSpanEnvelopeMS)
				}
			case tracequery.BlockingValueBasisSpanEnvelope:
				text = fmt.Sprintf("span 包络 %.3fms(含运行;span 窗内无该线程时间线,等待段不可得,非阻塞等待实测)", node.BlockingSpanEnvelopeMS)
				if !zh {
					text = fmt.Sprintf("span envelope %.3fms (includes running; no thread timeline inside the span window, wait segments underivable — not a measured blocking wait)", node.BlockingSpanEnvelopeMS)
				}
			}
			if text != "" {
				if node.BlockingSubjectIsHolder {
					// XERR1-EXT: per-lane referent annotation (件G② 双词面各说各).
					if zh {
						text += "(账目主体=等待方)"
					} else {
						text += " (account subject = the waiter)"
					}
				}
				add("值口径", "value basis", text)
			}
		}
		// XERR1-FIX 修补 件F (冷读 P3-3, 2026-07-16): the partial-coverage
		// lower-bound disclosure — the waiter's account did not tile the whole
		// span∩window interval, so the converged value is a proven lower
		// bound. Typed gate only; the span-window ms derives from the node's
		// own typed endpoints (the engine's convergence interval); endpoints
		// absent → the sentence keeps its claim without numbers (不造数).
		// XERR1-EXT: payload-typed rows keep the numberless form — their
		// convergence interval is the fold VALUE-WINNER interval, which is
		// not on the wire, so deriving a denominator from the display
		// endpoints could print the WRONG window on a folded row (不造数,
		// 宁漏勿假指). The holder-subject referent follows the 件G②
		// discipline per lane.
		if node.BlockingWaitCoveragePartial {
			coverage := "等待段账目未满覆盖 span 窗:收敛值为已证下界"
			if !zh {
				coverage = "the wait-segment account does not fully cover the span window: the converged value is a proven lower bound"
			}
			if windowMS := (node.EndTs - node.StartTs) * 1000; windowMS > 0 && node.BlockingWaitAccountCoveredMS > 0 &&
				strings.TrimSpace(node.BlockingKind) == "" {
				coverage = fmt.Sprintf("等待段账目未满覆盖 span 窗(账目 %.3fms/span 窗 %.3fms):收敛值为已证下界",
					node.BlockingWaitAccountCoveredMS, windowMS)
				if !zh {
					coverage = fmt.Sprintf("the wait-segment account covers only %.3fms of the %.3fms span window: the converged value is a proven lower bound",
						node.BlockingWaitAccountCoveredMS, windowMS)
				}
			}
			if node.BlockingSubjectIsHolder {
				if zh {
					coverage += "(账目主体=等待方)"
				} else {
					coverage += " (account subject = the waiter)"
				}
			}
			add("覆盖核查", "coverage check", coverage)
		}
		if node.BlockingWaitBudgetExceeded {
			envelope := node.BlockingSpanEnvelopeMS
			if envelope <= 0 {
				envelope = runtimeTraceProjNodeDisplayImpact(node)
			}
			budget := fmt.Sprintf("⚠ span 包络 %.3fms > 窗内非 running %.3fms:含 running %.3fms,非阻塞等待段",
				envelope, node.BlockingWaitBudgetNonRunningMS, node.BlockingWaitBudgetRunningMS)
			if !zh {
				budget = fmt.Sprintf("⚠ span envelope %.3fms > in-window non-running %.3fms: contains running %.3fms, not blocking-wait segments",
					envelope, node.BlockingWaitBudgetNonRunningMS, node.BlockingWaitBudgetRunningMS)
			}
			if node.BlockingSubjectIsHolder {
				// 件G② 复核修 (2026-07-16): per-lane annotation — the zh
				// suffix must never leak onto the EN face (双词面各说各).
				if zh {
					budget += "(预算主体=等待方)"
				} else {
					budget += " (budget subject = the waiter)"
				}
			}
			add("等待预算核查", "wait-budget check", budget)
		}
		if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
			add("持有点", "held at", runtimeTraceCausalProjectionMarkdownSafe(site))
		}
		// BLOCKFROM (DISP-2 Wave-3.2, 2026-07-09): the WAITER-side blocking
		// call site of a monitor contention — the span's verbatim "blocking
		// from ..." segment (typed blocking_from_site note; wire-contract name
		// pinned with the TEX engine batch). 等待点 beside 持有点: WHERE the
		// waiter blocked vs WHERE the holder held. Verbatim on this lossless
		// surface (no truncation — 明细不截断纪律); empty renders nothing.
		if site := strings.TrimSpace(node.BlockingFromSite); site != "" {
			add("等待点", "blocking from", runtimeTraceCausalProjectionMarkdownSafe(site))
		}
		// P0-E 锁车道修3 (§24.9-C F5): the inferred-holder origin reaches the
		// detail face — pre-P0-E the engine caveat existed but no user face
		// consumed it (置信"中" was the only residue). Typed enum fork only.
		if runtimeTraceProjBlockingHolderInferred(node) {
			origin := ""
			switch node.BlockingHolderSource {
			case tracequery.CounterpartSourceWakeupEdge:
				if node.BlockingOwnerTidRaw > 0 {
					// LOCKNS-FIX 修补 件A (冷读 P2-F1+P3-F7 同族, 2026-07-16):
					// the presence clause forks on the typed
					// owner_tid_presence verdict — the legacy 「不在本 trace」
					// claim was FALSE on the collision / comm-mismatch shapes
					// (and contradicted the same board's engine collision
					// Summary). Missing note (legacy wire) or "absent" keeps
					// the legacy sentence byte-identically (fail-open).
					switch node.BlockingOwnerTidPresence {
					case tracequery.OwnerTidPresenceCollision:
						origin = fmt.Sprintf("唤醒边推断(payload owner tid %d 在本 trace 中存在,但为容器命名空间撞号,非持有者归因依据;由等待方的收尾唤醒边推得,非 payload 证实)", node.BlockingOwnerTidRaw)
						if !zh {
							origin = fmt.Sprintf("inferred from the waiter's closing wakeup edge (payload owner tid %d is present in this trace only as a container-namespace numeric collision, not a holder-attribution basis; not payload-confirmed)", node.BlockingOwnerTidRaw)
						}
					case tracequery.OwnerTidPresenceCommMismatch:
						origin = fmt.Sprintf("唤醒边推断(payload owner tid %d 在本 trace 中在场但线程名与 payload 所记不符,非持有者归因依据;由等待方的收尾唤醒边推得,非 payload 证实)", node.BlockingOwnerTidRaw)
						if !zh {
							origin = fmt.Sprintf("inferred from the waiter's closing wakeup edge (payload owner tid %d is present in this trace but its thread name never matches the payload's owner comm, not a holder-attribution basis; not payload-confirmed)", node.BlockingOwnerTidRaw)
						}
					default:
						origin = fmt.Sprintf("唤醒边推断(payload owner %d 不在本 trace;由等待方的收尾唤醒边推得,非 payload 证实)", node.BlockingOwnerTidRaw)
						if !zh {
							origin = fmt.Sprintf("inferred from the waiter's closing wakeup edge (payload owner %d absent from this trace; not payload-confirmed)", node.BlockingOwnerTidRaw)
						}
					}
				} else {
					origin = "唤醒边推断(payload 未证实持有者;由等待方的收尾唤醒边推得)"
					if !zh {
						origin = "inferred from the waiter's closing wakeup edge (no payload-confirmed holder)"
					}
				}
			case tracequery.CounterpartSourceNsSpanDerivation:
				origin = fmt.Sprintf("ns-span 推导(payload owner %d 为容器命名空间 id;由 trace_mark 发射对映射,非 payload 证实)", node.BlockingOwnerTidRaw)
				if !zh {
					origin = fmt.Sprintf("derived via ns-span emission pairs (payload owner %d is a container-namespace id; not payload-confirmed)", node.BlockingOwnerTidRaw)
				}
				// LOCKNS-FIX 件6 / OM-10 关账 (§29.104.12, 2026-07-16): the
				// typed ②×③ identity-unification declaration reaches the
				// origin line — ns-span emission-pair derivation and the
				// closing wakeup edge independently named the SAME host
				// thread (§18.E.1). Per-lane wording (双词面各说各); empty
				// (single-lane derivations, legacy records) renders nothing.
				if strings.TrimSpace(node.BlockingHolderNsUnification) != "" {
					if zh {
						origin += "(发射对×收尾唤醒两道互证:owner 与释放线程为同一物理线程)"
					} else {
						origin += " (emission pairs × closing wakeup cross-corroborated: owner and releasing thread are one physical thread)"
					}
				}
			}
			if origin != "" {
				add("持有者来历", "holder origin", origin)
			}
		}
		// LOCKNS-FIX 件4 (§29.104.12 G3, 2026-07-16): the UNRESOLVED-holder
		// disclosure — a typed contention row whose holder no lane could name
		// previously rendered NO origin line at all (静默口). Typed gates
		// only: contention semantics present ∧ no peer ∧ no resolution lane ∧
		// no withdrawal witness (the 归因撤回 line below already words that
		// shape). The raw-tid fork words the sentinel/ownerless form vs the
		// unresolvable payload-tid form; 有主形 renders nothing here.
		if strings.TrimSpace(node.BlockingKind) != "" &&
			strings.TrimSpace(node.BlockingPeer) == "" &&
			strings.TrimSpace(node.BlockingHolderSource) == "" &&
			strings.TrimSpace(node.BlockingHolderContradiction) == "" &&
			node.BlockingHolderContradictionParts == nil {
			// 修补 件C (冷读 P3-F3, 2026-07-16): the add() label already says
			// 持有者未解析/holder unresolved — the value carries only the WHY
			// (the pre-fix render doubled the phrase on both lanes:
			// 「持有者未解析: 持有者未解析(…)」).
			unresolved := "哨兵值/无主 payload,且无收尾唤醒边"
			if !zh {
				unresolved = "ownerless sentinel payload and no closing wakeup edge"
			}
			if node.BlockingOwnerTidRaw > 0 {
				unresolved = fmt.Sprintf("payload owner tid %d 无法定位,亦无收尾唤醒边", node.BlockingOwnerTidRaw)
				if !zh {
					unresolved = fmt.Sprintf("payload owner tid %d could not be located and no closing wakeup edge exists", node.BlockingOwnerTidRaw)
				}
			}
			add("持有者未解析", "holder unresolved", unresolved)
		}
		// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): the unknown-morphology
		// fail-open disclosure — the span speaks lock-owner vocabulary but
		// matched no registered contention morphology, so no holder
		// attribution was minted (payload-less lane, value basis discipline
		// untouched). Typed marker gate; absence renders nothing.
		if node.BlockingOwnerKeyUnregistered {
			check := "owner 未解析(形态未注册):span 文本含 owner 词但无注册锁竞争形态匹配,未铸持有者归因"
			if !zh {
				check = "owner unresolved (morphology unregistered): the span text speaks owner vocabulary but matches no registered lock-contention shape; no holder attribution is minted"
			}
			add("持有者核查", "holder check", check)
		}
		if handoff := strings.TrimSpace(node.BlockingHolderHandoff); handoff != "" {
			// 修2: the payload hand-off chain — the named holder is the FINAL
			// holder; the span is the wait envelope, never one thread's tenure.
			// GAP-B G17 (§27.4, 2026-07-09): the label names the chain members
			// as THREADS — the bare "A --> B" string read as a network 链路 to
			// the report's LLM consumer (huadong_79 misread witness).
			text := handoff + "(锁在等待期内换手;所示持有者为最后持有者,非全段持有)"
			if !zh {
				text = handoff + " (the lock changed hands during the wait; the named holder is the FINAL holder, not the whole-span holder)"
			}
			add("持有者移交链(线程)", "holder hand-off chain (threads)", runtimeTraceCausalProjectionMarkdownSafe(text))
		}
		if contradiction := strings.TrimSpace(node.BlockingHolderContradiction); contradiction != "" ||
			node.BlockingHolderContradictionParts != nil {
			// 修2 同锁自相矛盾守护: the withdrawn attribution's witness.
			// G10-EN 根修 (QH2-A, 2026-07-14; §28.7 留账): each lane words the
			// witness from the typed components — the zh sentence is
			// byte-identical to the legacy engine mint (single wording
			// source, types.TraceHolderSelfContradictionWitness.WitnessText),
			// the EN face carries a full English sentence instead of the zh
			// body verbatim. Legacy records without the component notes keep
			// the verbatim witness on both lanes (lossless fallback; absence
			// never fabricates an EN sentence).
			if parts := node.BlockingHolderContradictionParts; parts != nil {
				contradiction = parts.WitnessText(zh)
			}
			text := "持有者归因已撤回(推断持有者自身同锁排队):" + contradiction
			if !zh {
				text = "holder attribution withdrawn (inferred holder was itself queued on the same lock): " + contradiction
			}
			add("持有者归因撤回", "holder attribution withdrawn", runtimeTraceCausalProjectionMarkdownSafe(text))
		}
		if roster := strings.TrimSpace(node.OccupierSummary); roster != "" {
			add("同窗占用者", "same-window occupiers", runtimeTraceCausalProjectionMarkdownSafe(runtimeTraceProjOccupierRosterDisplay(roster, zh)+"(cpu·ms)"))
		}
		if coverage, ok := runtimeTraceProjFullWindowCoverageTag(node, zh, row.CoverageFragmentSecondary, row.CoverageMergedTwinCount); ok {
			add("全窗合计", "full-window total", runtimeTraceCausalProjectionMarkdownSafe(coverage.Text))
		}
		// F1 (§22 PTV7-SPN P0): the parsed span name keeps its keyed lossless
		// line — SpanName non-empty is the whole gate (typed note, verbatim
		// value). PTV8-RCR-B (UXA 域B #18, 2026-07-08). EVOLUTION RECORD: the
		// 400+-char raw span text used to sit mid-block between 类型 and 关系
		// — the verbatim value (untouched, §22.2.1 原文保真) now closes the
		// block in code style, after the readable extracted fields.
		if span := strings.TrimSpace(node.SpanName); span != "" {
			add("span 原文", "span source", "`"+runtimeTraceCausalProjectionMarkdownSafe(span)+"`")
		}
		// DISPLAY-WRAP 件③(a): the same-node repeat suppression covers the
		// lossless block too (witness L595-597: one node spelled 按实测频点
		// 共动分簇折算 three times in three consecutive lines). The block is
		// self-anchoring — its own first line keeps the full phrase — so the
		// reference word never points outside the block; marks are fence-lane
		// vocabulary and deliberately not fired here (the legend renders
		// before this face; the fence pass lights them whenever the fence
		// wears the words).
		var dedupLines []*string
		for i := range lines {
			dedupLines = append(dedupLines, &lines[i])
		}
		runtimeTraceProjDedupNodeCaliberPhrases(dedupLines, nil, zh)
		body := strings.Join(lines, "\n")
		if tag != "" {
			key := name + "\n" + body
			if prev, ok := index[key]; ok {
				prev.tags = append(prev.tags, tag)
				continue
			}
			stanza := &detailStanza{tags: []string{tag}, name: name, body: body}
			index[key] = stanza
			ordered = append(ordered, stanza)
			continue
		}
		ordered = append(ordered, &detailStanza{name: name, body: body})
	}
	stanzas := make([]string, 0, len(ordered))
	for _, stanza := range ordered {
		heading := "**"
		for _, tag := range stanza.tags {
			heading += "[" + tag + "] "
		}
		heading += stanza.name + "**"
		stanzas = append(stanzas, heading+"\n"+stanza.body)
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
		// PTV8-RCR-B (UXA 域B #3 verify 关注线程 family, 2026-07-08).
		if zh {
			return "关注线程状态"
		}
		return "the focused thread's state"
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
			// PTV8-RCR-C (§24.12 C6 三面同词, 2026-07-08). EVOLUTION RECORD:
			// this cell said 深度1(未接入链) while the row chip said 链上L1 and
			// the edge said 深度未解析 — three calibers on one row. All three
			// faces now read the shared 链上L#(父节点未确认) word.
			return runtimeTraceProjChainDepthChipWord(row, zh)
		}
		if zh {
			return "链上·深度未解析"
		}
		return "on-chain · depth unresolved"
	case runtimeTraceProjTreeRowCause:
		// PTV8-RCR-B (UXA 域B #27 verify, 2026-07-08). EVOLUTION RECORD: the
		// chain arms speak the tree's 链上L# word (两套层深记号并一);
		// detached/flat/semantic arms keep 深度N (CMP-7a: no chain claim).
		if zh {
			return fmt.Sprintf("成因·链上L%d", row.Depth)
		}
		return fmt.Sprintf("cause · chain L%d", row.Depth)
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
				return fmt.Sprintf("链上L%d", row.Depth)
			}
			return fmt.Sprintf("chain L%d", row.Depth)
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
	// Semantic identity outranks display placement: an off-chain semantic
	// span now lives in a ◇/▒ stanza rather than the causal tree, but its
	// relation remains “span hosted by <thread>”, not generic context support.
	if runtimeTraceCausalProjectionSemanticSpanRow(row.Node) {
		host := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if host == "" {
			if zh {
				return "语义span"
			}
			return "span"
		}
		if zh {
			return host + " 的语义span"
		}
		return "semantic span of " + host
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
		// PTV5 C31 (#68): zh face matches the sibling 链上·深度未解析 cell.
		if zh {
			return "链上·影响点未解析"
		}
		return "on-chain · impact point unresolved"
	}
	// PTV8-RCR-B (UXA 域B #17, 2026-07-08). EVOLUTION RECORD: the
	// 「label ▸ parent」 composite (direction to be guessed, ▸ mojibake on the
	// customer terminal) becomes a full clause per edge: 成因 = "X 的成因",
	// 下钻 = "由 X 下钻得到", 唤醒 = "唤醒 X". Parent-less arms keep the bare
	// word.
	switch row.Edge {
	case runtimeTraceProjTreeEdgeOwn:
		// F2: the own-edge row re-describes the target's own process wall
		// clock — never a wake claim (there is no upstream relation to point
		// at). Mirrors the fence's ├─自身─ edge.
		if zh {
			return "自身进程IO"
		}
		return "own-process IO"
	case runtimeTraceProjTreeEdgeChainUnresolved:
		// PTV6 #1c: the relation word follows the edge — never 唤醒 (the
		// attach point is exactly what is unresolved; same wording as the C31
		// sibling cell and the layer cell).
		// PTV8-RCR-C (§24.12 C6): the depth-known unattached shape follows its
		// edge word too — 深度未解析 here would re-open the fork one field
		// below the unified 层级 cell.
		if runtimeTraceProjDepthlessUnattachedRow(row) {
			if zh {
				return "链上·父节点未确认"
			}
			return "on-chain · parent unconfirmed"
		}
		if zh {
			return "链上·深度未解析"
		}
		return "on-chain · depth unresolved"
	case runtimeTraceProjTreeEdgeDrill:
		if parent == "" {
			if zh {
				return "下钻"
			}
			return "drill"
		}
		if zh {
			return "由 " + parent + " 下钻得到"
		}
		return "reached by drilling from " + parent
	case runtimeTraceProjTreeEdgeSemantic:
		// PTV8-RCR-C (§24.12 维度C C-新5 宿主如实, 2026-07-08). EVOLUTION
		// RECORD: this arm read the tree ATTACH anchor (row.Parent) — a span
		// hosted on a foreign thread (cmp_78_01 E41/E42: binder:8815_1-6581 /
		// uawei.hwid.core-10353) was asserted as "main-6565 的语义span" while
		// the tree 行3 correctly said span 位于 <宿主> 内. The relation now
		// names the HOST thread (node.Subject, same source as the tree line);
		// the anchor stays visible on the tree structure itself.
		if host := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh)); host != "" {
			parent = host
		}
		if parent == "" {
			if zh {
				return "语义span"
			}
			return "span"
		}
		if zh {
			return parent + " 的语义span"
		}
		return "semantic span of " + parent
	case runtimeTraceProjTreeEdgeCause:
		if parent == "" {
			if zh {
				return "成因"
			}
			return "cause"
		}
		if zh {
			return parent + " 的成因"
		}
		return "cause of " + parent
	default:
		if parent == "" {
			if zh {
				return "唤醒"
			}
			return "wakes"
		}
		if zh {
			return "唤醒 " + parent
		}
		return "wakes " + parent
	}
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
	// PTV5 C35 (#68): the E#(+N) merge notation gets its intro half-sentence
	// exactly when some indexed node carries merged observations — rosters
	// without the notation stay byte-identical.
	if evidence.hasMergedEvidence {
		if zh {
			intro += "E#(+N) 表示该行另合并了 N 条同类观测,合并明细见对应条目的审计 merged_ids。"
		} else {
			intro += " E#(+N) means the row absorbed N more same-kind observations; the merge detail lives in that entry's audit merged_ids."
		}
	}
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
			intro += " 全部证据位于 `" + runtimeTraceCausalProjectionMarkdownSafe(display) + "`,各条只标注行号或时间区间。"
		} else {
			intro += " All locators live in `" + runtimeTraceCausalProjectionMarkdownSafe(display) + "`; entries carry only a line or time span."
		}
	}
	grouped := uniform && sharedFile != "" && len(entries) > 1
	items := make([]types.AnswerBlockItem, 0, len(entries))
	for _, entry := range entries {
		locator := runtimeTraceCausalProjectionEvidenceDisplayRefWithWindow(entry.Ref, entry.Window)
		if grouped {
			// Grouped mode: the shared file name is stated once in the intro; each
			// entry keeps only its own window (preferred, 裁定6) or line range.
			// PTV8-RCR-B (UXA 域C #6, 2026-07-08). EVOLUTION RECORD: the bare
			// machine suffix (":45696-79136") reads as locator syntax — the
			// grouped display now says 行 X–Y (en-dash, same form as the 详见
			// tail).
			if entry.Window != "" {
				locator = runtimeTraceCausalProjectionMarkdownSafe(entry.Window)
			} else if _, suffix := runtimeTraceCausalProjectionSplitLineSuffix(entry.Ref); suffix != "" {
				lineRange := strings.ReplaceAll(strings.TrimPrefix(suffix, ":"), "-", "–")
				if zh {
					locator = "行 " + runtimeTraceCausalProjectionMarkdownSafe(lineRange)
				} else {
					locator = "lines " + runtimeTraceCausalProjectionMarkdownSafe(lineRange)
				}
			}
		}
		if entry.SyntheticLine {
			// CMP-7b: an absence observation (missing_wakeup) has no trace row of
			// its own — its line span is interval bookkeeping, so a "file:44"
			// locator reads as a real row. Display keeps only the artifact name;
			// the raw record retains the interval lines for audit. A bare line
			// ref with no artifact name keeps its legacy display — stripping it
			// would leave nothing auditable on the panel.
			// PTV8-RCR-B (UXA 域C #8 REVISE, 2026-07-08): in GROUPED mode the
			// basename repeats the intro verbatim (zero information) — the
			// entry states the synthetic fact instead; its own time window (if
			// any) stays. Non-grouped (multi-artifact) keeps the basename —
			// the artifact identity is load-bearing there.
			if grouped {
				if entry.Window != "" {
					locator = runtimeTraceCausalProjectionMarkdownSafe(entry.Window)
				} else if zh {
					locator = "无独立 trace 行(区间性推断观测,无单行坐标)"
				} else {
					locator = "no standalone trace line (interval-inferred observation, no single-line coordinate)"
				}
			} else if synthetic := runtimeTraceCausalProjectionSyntheticEvidenceLocator(entry); synthetic != "" {
				locator = synthetic
			}
		}
		if locator == "" {
			locator = "trace_query"
		}
		// F2 (§22 PTV7-SPN): part-boundary cut at the 96-rune audit ceiling —
		// see runtimeTraceCausalProjectionAuditCellText for the two-half
		// rationale (the 72-rune mid-token cut lost confidence + predicate).
		// RCM-2 D4: a family entry's ceiling widens — its member_count/
		// member_fold_caliber tokens are load-bearing (the E# stands for N
		// members; a cut here would hide the merge from the audit face).
		auditCeiling := 96
		if entry.FamilyAudit {
			auditCeiling = 160
		}
		// SELF-ALL rider (2026-07-13): an absorbed chain-lane entry's
		// absorbed_into=<family key> pointer is load-bearing (G1 第三面无损)
		// and the family key now carries the typed proof-basis lane dimension —
		// widen exactly like FamilyAudit so the pointer never drops.
		if entry.AbsorbedAudit {
			auditCeiling = 160
		}
		// 修复轮 件5 (2026-07-14): the origin=system_supplement provenance
		// token must never part-boundary-drop — widen like FamilyAudit.
		if entry.SupplementAudit && auditCeiling < 160 {
			auditCeiling = 160
		}
		// DIAG A1 (§28.11-3(a)): same-value fold entries widen further — the
		// tie witness is TWO tokens (subjects roster + per-member line
		// intervals) and both are the reason the E# is auditable without the
		// raw trace. 280 holds the worst REAL prefix (tier=deterministic_
		// optimization + causality=adjacent_to_wakeup_chain + rank +
		// confidence, ≈98 runes) plus a two-member roster of 40-rune thread
		// labels with 7-digit line coordinates — the E23 customer shape at its
		// widest — pinned by TestDiagSameValueAuditWorstCasePrefix; a 4-member
		// roster of extreme labels may still part-boundary-drop the lines
		// token (documented residual — the member cap bounds the face).
		if entry.SameValueAudit {
			auditCeiling = 280
		}
		audit := runtimeTraceCausalProjectionAuditCellText(entry.Details, auditCeiling)
		// PTV6-C ruling C (#73, 用户裁定 2026-07-06): the former "完整定位见原始
		// trace_query 记录" deflection tail is retired — when the display
		// locator actually dropped the entry's line range (window-preferred
		// display), the tail INLINES the trace source coordinate ("详见
		// <basename> 行 X–Y"); a display that only trimmed machine-local blob
		// directories adds no tail (the artifact basename already leads the
		// grouped intro, and blob paths are deliberately off the panel).
		// PTV8-RCR-B (UXA 域D layout-L3, 2026-07-08). EVOLUTION RECORD: the
		// zh format string mixed a half-width colon with a full-width
		// semicolon in one line — the system index face is uniformly
		// half-width (等宽对齐).
		var text string
		if zh {
			text = fmt.Sprintf("定位: %s; 审计: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
		} else {
			text = fmt.Sprintf("locator: %s; audit: %s", locator, runtimeTraceCausalProjectionMarkdownSafe(audit))
		}
		text += runtimeTraceProjEvidenceCoordinateTail(entry, locator, grouped, zh)
		items = append(items, types.AnswerBlockItem{
			ID:          strings.ToLower(entry.ID),
			Label:       entry.ID,
			Text:        text,
			CitationRef: -1,
		})
	}
	return intro, items
}

// runtimeTraceProjEvidenceCoordinateTail renders the PTV6-C ruling-C
// replacement for the retired intermediate-record pointer: exactly when the
// displayed locator dropped the entry's LINE RANGE (window-preferred display,
// 裁定6), the tail restores the trace source coordinate inline — "；详见
// <basename> 行 X–Y". Synthetic-line entries (CMP-7b interval bookkeeping)
// never claim a line coordinate; displays that already show the line range
// (or only trimmed blob directories, which are deliberately off the panel)
// get no tail at all.
func runtimeTraceProjEvidenceCoordinateTail(entry runtimeTraceCausalProjectionEvidenceEntry, display string, grouped, zh bool) string {
	if entry.SyntheticLine {
		return ""
	}
	pathPart, suffix := runtimeTraceCausalProjectionSplitLineSuffix(strings.TrimSpace(entry.Ref))
	lineRange := strings.TrimSpace(strings.TrimPrefix(suffix, ":"))
	// PTV8-RCR-B (UXA 域C #6 verify 耦合点, 2026-07-08): the containment check
	// normalizes the en-dash display form back to the raw hyphen before
	// comparing — a display that already shows "行 45696–79136" must not grow
	// a duplicate tail.
	if lineRange == "" || strings.Contains(strings.ReplaceAll(display, "–", "-"), lineRange) {
		return ""
	}
	rangeDisplay := runtimeTraceCausalProjectionMarkdownSafe(strings.ReplaceAll(lineRange, "-", "–"))
	if grouped {
		// PTV8-RCR-B (UXA 域C #7, 2026-07-08). EVOLUTION RECORD: grouped mode
		// repeated the 49-char basename three times right after the intro
		// declared it — the coordinate joins the 定位 field instead ("[a~b s],
		// 行 X–Y"); non-grouped keeps the 详见 tail verbatim (裁定措辞载体).
		if zh {
			return ",行 " + rangeDisplay
		}
		return ", lines " + rangeDisplay
	}
	base := strings.TrimPrefix(runtimeTraceCausalProjectionPathTail(pathPart, 1), "…/")
	if base == "" || types.TraceCausalProjectionPlaceholderArtifactToken(base) {
		if zh {
			return "；详见 trace 行 " + rangeDisplay
		}
		return "; see trace lines " + rangeDisplay
	}
	if zh {
		return "；详见 " + runtimeTraceCausalProjectionMarkdownSafe(base) + " 行 " + rangeDisplay
	}
	return "; see " + runtimeTraceCausalProjectionMarkdownSafe(base) + " lines " + rangeDisplay
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
func runtimeTraceProjFullWindowCoverageTag(node types.TraceCausalProjectionNode, zh, secondary bool, mergedTwinCount int) (runtimeTraceProjTag, bool) {
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
	// 件3 (修复轮, 2026-07-14; JankManager witness): a re-anchored row's RN-12
	// donor total is the CAPPED display-list value (16.687) while the row's
	// own 同源二分 line speaks the census full account (31.191) — 五表单源:
	// the decomposition floats are the same-source authority, so the tag
	// reads THEM. The ◇ remainder half suppresses the tag entirely (its
	// covered value is the OFF-chain remainder — any 「链上…」 claim over it
	// would be false; the 同源二分 line already carries the whole account).
	if node.ChainAnchorRemainderSeat {
		return runtimeTraceProjTag{}, false
	}
	if node.ChainAnchorFullMS > 0 {
		full = node.ChainAnchorFullMS
	}
	// PTV7 (#74, 用户裁定 2026-07-06): the state class IS the canonical
	// display word on both faces — the class TABLE keys stay untouched.
	// EVOLUTION RECORD (用户重裁 2026-07-08, UXA D#30 终稿, supersedes the
	// RN-12 ledger-verbatim "top 片段" R02 禁动面): "top 片段" becomes
	// 其中最大片段 / "its largest fragment", and the raw source token
	// (top_sleep / state_drilldown) leaves the prose — it stays verbatim on
	// the audit faces (system supplement / evidence index) per the §22.2.1
	// backstop; `source` presence still gates the note (typed provenance).
	displayClass := class
	// CR-3 修复轮追加件 (2026-07-12, 56643 witness): only the group-max row
	// may say 最大片段 — a same-thread sibling covering a SMALLER fragment
	// speaks 另一片段 (two rows both claiming "largest" were mutually
	// exclusive on one page). Single-row groups keep the original bytes.
	//
	// WO-A1 词面统一 (SMR-1 批 SMR-S14 残余, smr_audit_report §②, 2026-07-12):
	// an ×N MERGED row's covered value is the member SUM, so 「最大片段」 on it
	// is a false single-fragment claim (56643 E7/E11: "最大片段 19.933ms(77%)"
	// where 19.933 = ×3 合计 and the largest single fragment is 8.307) — the
	// merged form speaks 「链上覆盖合计(×N)」 instead. Unmerged rows keep the
	// pinned bytes.
	fragmentZH, fragmentEN := "链上仅覆盖其中最大片段", "the chain covers only its largest fragment"
	if secondary {
		fragmentZH, fragmentEN = "本行覆盖其中另一片段", "this row covers another fragment of it,"
	}
	mergedN := node.MergedCount
	if mergedN <= 1 && mergedTwinCount > 1 {
		mergedN = mergedTwinCount // 96717 追修: engine ×N total on an unmerged rank twin
	}
	if mergedN > 1 {
		fragmentZH = fmt.Sprintf("链上覆盖合计(共%d次)", mergedN)
		fragmentEN = fmt.Sprintf("the chain covers an n=%d total of", mergedN)
		if secondary {
			fragmentZH = fmt.Sprintf("本行覆盖其中另一部分·合计(共%d次)", mergedN)
			fragmentEN = fmt.Sprintf("this row covers another n=%d total slice of it,", mergedN)
		}
	}
	// 件3 (修复轮, 2026-07-14): the ⛓ clipped half's covered value is the
	// anchored Σ over ALL member segments inside the dependency windows —
	// 「最大片段」 would be a false single-fragment claim (JankManager: 1.759
	// spans several anchored slices).
	if node.ChainAnchorFullMS > 0 && !node.ChainAnchorRemainderSeat {
		fragmentZH, fragmentEN = "链上锚定合计", "the chain-anchored total is"
	}
	var text string
	switch {
	case node.FullWindowStateSameWindow:
		text = fmt.Sprintf("窗内 %s 合计 %.3fms,%s %.3fms(%.0f%%)",
			displayClass, full, fragmentZH, covered, covered/full*100)
		if !zh {
			text = fmt.Sprintf("full-window %s total %.3fms; %s %.3fms (%.0f%%)",
				class, full, fragmentEN, covered, covered/full*100)
		}
	case node.FullWindowStateWindowStart > 0 && node.FullWindowStateWindowEnd > node.FullWindowStateWindowStart:
		text = fmt.Sprintf("另一查询窗(%.3fs~%.3fs)内 %s 合计 %.3fms,%s %.3fms(%.0f%%)",
			node.FullWindowStateWindowStart, node.FullWindowStateWindowEnd,
			displayClass, full, fragmentZH, covered, covered/full*100)
		if !zh {
			text = fmt.Sprintf("%s total %.3fms in another query window (%.3fs~%.3fs); %s %.3fms (%.0f%%)",
				class, full, node.FullWindowStateWindowStart, node.FullWindowStateWindowEnd,
				fragmentEN, covered, covered/full*100)
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
