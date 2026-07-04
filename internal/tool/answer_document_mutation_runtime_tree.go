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
	// display width including the tag segment (§7.30.2 C4b B4). Overflowing
	// secondary tags elide to a "…" marker; the leading state/impact tag and
	// the trailing [E#] evidence reference always survive.
	//
	// NEW-10 (§7.6 对比场景客户回访 2026-07-04, 客户点名): tightened 120 → 100.
	// Markdown/HTML viewers wrap long lines (pre-wrap) and web monospace fonts
	// render CJK≈1.6-1.8× / emoji at unstable widths — the 100-cell hard cap
	// buys single-line integrity on those surfaces (单行完整性优先). When the
	// drop lane is exhausted, truncatable Keep tags display-truncate to their
	// typed floors (see runtimeTraceProjFitTags); the lossless surfaces stay
	// the detail table + evidence index. F-2 (§7.7 回访聚焦复核 2026-07-04):
	// over-wide NoTruncate carriers (the NEW-3 caliber note, the D3
	// composition split) no longer overflow the row — they wrap intact onto
	// prefix-aligned ↳ continuation lines, so every fence line holds the cap.
	runtimeTraceProjTreeRowMaxWidth = 100
	// runtimeTraceProjTreeKeepTagMinWidth is the NEW-10 display floor for a
	// Keep-class tag in the keep-truncation lane: 9 cells keep a 4-CJK-glyph
	// attribution prefix plus the "…" (e.g. 反转影响…, 候选影响…), so the
	// at-a-glance marker survives even on budget-starved rows.
	runtimeTraceProjTreeKeepTagMinWidth = 9
	// runtimeTraceProjTreeTrunkMaxNodes bounds a long trunk display: deeper
	// middles compress into one omitted marker row (counts + cycle note kept).
	runtimeTraceProjTreeTrunkMaxNodes = 8
	runtimeTraceProjTreeBarWidth      = 10
	// runtimeTraceProjTreeLabelColumnMax caps the SHARED label column (F-3,
	// §7.7 回访聚焦复核 2026-07-04): pre-cap, one deep row (fixed prefix grows 4
	// cells per level + the 20-cell name floor) or one over-wide 🎯 header
	// lifted the column for EVERY row and pushed the whole fence past the
	// NEW-10 row cap. The cap reserves the minimal metric+stub area a fully
	// shaved data row still needs inside the 100-cell budget — derived from
	// the pinned constants and the renderer's own cell formats, never a
	// free-standing number:
	//   " "(1) + bar(10) + " %9.3fms"(12) + " NN%"(5, " %3.0f%%") + "  "(2)
	//   + Keep-tag floor(9) + " · "(3) + elision "…"(1) + " · "(3)
	//   + minimal "[E#]"(4)  = 50
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
	RootFocusUserEntities []string
	// UserWindowStart/UserWindowEnd is the user's originally-requested window in
	// seconds, derived DISPLAY-ONLY from a timestamp-shaped analyzer entity pair
	// (R2 双窗关系行). Zero when absent/ambiguous — the relation line then never
	// renders; nothing else consumes these values.
	UserWindowStart float64
	UserWindowEnd   float64
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
	runtimeTraceProjMarkRootTarget       runtimeTraceProjMark = iota // 🎯 root header
	runtimeTraceProjMarkEdgeDrill                                    // ├─下钻─ edge
	runtimeTraceProjMarkEdgeWake                                     // ├─唤醒─ / └─唤醒─ edge
	runtimeTraceProjMarkEdgeCause                                    // ├─成因─ edge
	runtimeTraceProjMarkEdgeOwn                                      // ├─自身─ own-process caliber edge (F2)
	runtimeTraceProjMarkSemanticSpan                                 // ├─语义─ edge + ✦ icon (always paired)
	runtimeTraceProjMarkIconSleep                                    // 💤 state icon
	runtimeTraceProjMarkIconRunnable                                 // ⏳ state icon
	runtimeTraceProjMarkIconRunning                                  // ⚙ state icon
	runtimeTraceProjMarkIconDState                                   // ⛓ state icon
	runtimeTraceProjMarkIconTransit                                  // ◦ transit / stateless icon
	runtimeTraceProjMarkStateLabel                                   // post-bar dominant-state / impact-shape tag
	runtimeTraceProjMarkUndrillable                                  // ⛔ missing-wakeup marker
	runtimeTraceProjMarkCrossWindow                                  // ⚠跨窗 marker
	runtimeTraceProjMarkRecursOnChain                                // ↺ small-cycle marker
	runtimeTraceProjMarkOmitted                                      // …省略… long-trunk fold row
	runtimeTraceProjMarkIOCaliberNote                                // NEW-3 同段IO另有…口径 note
	runtimeTraceProjMarkAdjacentStanza                               // ◇ 邻近 stanza
	runtimeTraceProjMarkBackgroundStanza                             // ▒ 背景压力 stanza

	// runtimeTraceProjMarkCount is the completeness sentinel — every mark above
	// MUST have a runtimeTraceProjLegendCatalog entry (structurally pinned).
	runtimeTraceProjMarkCount
)

// runtimeTraceProjMarkSet is the nil-safe typed emission set: mark() on a nil
// receiver is a no-op so width-pass label computations and hand-built test
// rows never record.
type runtimeTraceProjMarkSet struct {
	seen [runtimeTraceProjMarkCount]bool
}

func (s *runtimeTraceProjMarkSet) mark(m runtimeTraceProjMark) {
	if s == nil || m < 0 || m >= runtimeTraceProjMarkCount {
		return
	}
	s.seen[m] = true
}

func (s *runtimeTraceProjMarkSet) has(m runtimeTraceProjMark) bool {
	return s != nil && m >= 0 && m < runtimeTraceProjMarkCount && s.seen[m]
}

// runtimeTraceProjLegendEntry is one catalog row: the typed mark plus its zh/en
// legend clause (both full "- …" lines, ready to join).
type runtimeTraceProjLegendEntry struct {
	Mark runtimeTraceProjMark
	ZH   string
	EN   string
}

// runtimeTraceProjLegendCatalog is the full, ordered mark directory of the tree
// renderer. Wording notes:
//   - the └─唤醒─ entry keeps the NEW-1 direction wording VERBATIM (客户点名
//     "谁唤醒谁" — one direction, stated twice consistently);
//   - the 💤 / ├─成因─ / ⛔ / state-label entries keep the pre-NEW-7 legend
//     wording verbatim (established, already customer-reviewed lines).
func runtimeTraceProjLegendCatalog() []runtimeTraceProjLegendEntry {
	return []runtimeTraceProjLegendEntry{
		{runtimeTraceProjMarkRootTarget,
			"- `🎯` = 树根:本次分析锚定的关注线程。",
			"- `🎯` = tree root: the focused thread this analysis anchors on."},
		{runtimeTraceProjMarkEdgeDrill,
			"- `├─下钻─` = 根症状的下钻结果:该行是根等待的直接上游。",
			"- `├─drill─` = drilldown from the root symptom: this row is the root wait's direct upstream."},
		{runtimeTraceProjMarkEdgeWake,
			"- `└─唤醒─` = 该行唤醒其父行(父行的等待由该行结束;父行依赖该行)。",
			"- `└─wakes─` = this row WAKES its parent row (the parent's wait ends on this row; the parent depends on it)."},
		{runtimeTraceProjMarkEdgeCause,
			"- `├─成因─` = 同一线程的成因分解。",
			"- `├─cause─` = same-thread cause decomposition."},
		{runtimeTraceProjMarkEdgeOwn,
			"- `├─自身─` = 目标自身/同进程的口径行(同段墙钟的另一口径),非唤醒边。",
			"- `├─own─` = an own-/same-process caliber row of the target (another caliber of the same wall clock), not a wake edge."},
		{runtimeTraceProjMarkSemanticSpan,
			"- `├─语义─`/`✦` = 该位置的语义 span(业务阶段),非调度状态行。",
			"- `├─span─`/`✦` = a semantic span (business phase) at this position, not a scheduler-state row."},
		{runtimeTraceProjMarkIconSleep,
			"- `💤` = 睡眠等待;症状非根因,其唤醒子行即下钻结果。",
			"- `💤` = sleep wait; a symptom, not a root cause — its wake child IS the drilldown result."},
		{runtimeTraceProjMarkIconRunnable,
			"- `⏳` = 可运行等待(已就绪,等待 CPU)。",
			"- `⏳` = runnable wait (ready, waiting for a CPU)."},
		{runtimeTraceProjMarkIconRunning,
			"- `⚙` = 运行占用(正在 CPU 上执行)。",
			"- `⚙` = running (executing on a CPU)."},
		{runtimeTraceProjMarkIconDState,
			"- `⛓` = D状态/IO阻塞(不可中断等待)。",
			"- `⛓` = D-state / IO block (uninterruptible wait)."},
		{runtimeTraceProjMarkIconTransit,
			"- `◦` = 链路中转或无主导调度状态的行。",
			"- `◦` = chain transit or no dominant scheduler state."},
		{runtimeTraceProjMarkStateLabel,
			"- 时长条后的状态标签(睡眠等待/可运行等待/运行占用/IO阻塞/D状态)来自该行主导调度状态;无主导状态的行沿用影响形态。",
			"- The state tag after each bar (sleep wait / runnable wait / running / IO wait / D-state) is the row's dominant scheduler state; rows without one keep their impact-shape value."},
		{runtimeTraceProjMarkUndrillable,
			"- `⛔` = 窗口内无匹配 sched_wakeup,链止于此。",
			"- `⛔` = no matching sched_wakeup in the window; the chain ends there."},
		{runtimeTraceProjMarkCrossWindow,
			"- `⚠跨窗` = 实际状态跨出分析窗口,时长条只画窗口内投影。",
			"- `⚠crosses window` = the underlying state extends beyond the analysis window; the bar draws only the in-window projection."},
		{runtimeTraceProjMarkRecursOnChain,
			"- `↺` = 该线程在链上重复出现(小循环形态)。",
			"- `↺` = this thread recurs on the chain (small-cycle shape)."},
		{runtimeTraceProjMarkOmitted,
			"- `…省略…` = 长链中段折叠,完整链路见原始 trace_query 记录。",
			"- `…omitted…` = the middle of a long chain is folded; the full chain remains in the trace_query record."},
		{runtimeTraceProjMarkIOCaliberNote,
			"- `同段IO另有…口径` = 同一线程同段 IO 的多口径合并显示;数值与证据保留,不重复计入归因。",
			"- `same-segment IO also measured …` = several calibers of one IO segment folded for display; values and evidence kept, never double counted."},
		{runtimeTraceProjMarkAdjacentStanza,
			"- `◇` = 邻近区段:与主链时间相邻,不在唤醒路径上。",
			"- `◇` = adjacent stanza: time-adjacent to the chain, not on the wakeup path."},
		{runtimeTraceProjMarkBackgroundStanza,
			"- `▒` = 背景压力区段:环境证据,不计入链上归因。",
			"- `▒` = background-pressure stanza: environmental evidence, not chain attribution."},
	}
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
			omitted := &runtimeTraceProjTreeNode{row: runtimeTraceProjTreeRow{
				Kind: runtimeTraceProjTreeRowOmitted, Omitted: omitEnd - omitStart,
				CyclePeriod: cyclePeriod, CycleCount: cycleCount, Depth: idx + 1,
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
	return model
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
		return
	}
	if runtimeTraceProjTargetMatchesUserEntities(target, focus.Entities) {
		return // 🎯 root really is a user-named thread — keep ‹用户关注线程›
	}
	model.RootFocusAnchorOnly = true
	model.RootFocusUserEntities = runtimeTraceProjThreadOrPidEntities(focus.Entities)
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
		if zh {
			b.WriteString("◇ 邻近链 — 与主链时间相邻,不在唤醒路径上\n")
		} else {
			b.WriteString("◇ Adjacent — time-adjacent to the chain, not on the wakeup path\n")
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
		if zh {
			b.WriteString("▒ 背景压力 — 环境证据,不计入链上归因,需结合 on-chain 证据解读\n")
		} else {
			b.WriteString("▒ Background pressure — environmental evidence, not chain attribution; read with on-chain evidence\n")
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
			consider("    "+runtimeTraceProjStateIcon(row.Node, row.Kind, nil)+" ", runtimeTraceProjRowName(row, zh))
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
	fixed := runtimeTraceProjTreePrefix(row) + edge + " " +
		runtimeTraceProjStateIcon(row.Node, row.Kind, row.marks) + " "
	return fixed, runtimeTraceProjRowName(row, zh)
}

// runtimeTraceProjTreeLabel composes a row label with name-scoped truncation
// (B1): the name budget is the base label width minus the actual fixed-part
// display width, floored at the readability minimum, then the whole label pads
// (never truncates) to the shared column width.
func runtimeTraceProjTreeLabel(fixed, name string, width int) string {
	budget := runtimeTraceProjTreeNameBudget(runewidth.StringWidth(fixed))
	if runewidth.StringWidth(name) > budget {
		name = runtimeTraceProjPadDisplay(name, budget)
	}
	label := fixed + name
	if pad := width - runewidth.StringWidth(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	return label
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

// runtimeTraceProjSelfRowLines renders one self row for the fence (F-2 §7.7
// 回访聚焦复核 2026-07-04): the legacy single line whenever it holds the NEW-10
// row cap; over the cap, the NEW-3 caliber note — the only self-row part with
// no per-row width bound and the lane's only NoTruncate carrier — moves intact
// onto its own ↳ continuation line(s), and the main line keeps everything else
// (state, value, [E#]).
func runtimeTraceProjSelfRowLines(row runtimeTraceProjTreeRow, zh bool) []string {
	const lead = "│     "
	full := lead + runtimeTraceProjSelfRowText(row, zh)
	if len(row.IOFoldPeers) == 0 || runewidth.StringWidth(full) <= runtimeTraceProjTreeRowMaxWidth {
		return []string{full}
	}
	trimmed := row
	trimmed.IOFoldPeers = nil
	// The measuring call above already recorded the caliber-note mark (NEW-7:
	// the note still renders — on the continuation lane).
	lines := []string{lead + runtimeTraceProjSelfRowText(trimmed, zh)}
	return append(lines, runtimeTraceProjNoteContinuationLines("│     ",
		runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh))...)
}

func runtimeTraceProjSelfRowText(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	var parts []string
	name := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if node.IsSleepState() {
		state := strings.TrimSpace(node.StateKind)
		if state == "" {
			state = "sleep"
		}
		row.marks.mark(runtimeTraceProjMarkIconSleep)
		parts = append(parts, "💤 "+state)
	} else if name != "" {
		// NEW-10 (§7.6 记号区规整): every self row leads with exactly one state
		// glyph (sleep rows already carry 💤) — glyph-less rows made same-depth
		// emoji counts vary, so proportional web fonts drifted rows by variable
		// offsets; a constant one-glyph slot collapses the drift to a constant.
		parts = append(parts, runtimeTraceProjStateIcon(node, row.Kind, row.marks)+" "+name)
	} else {
		parts = append(parts, runtimeTraceProjStateIcon(node, row.Kind, row.marks))
	}
	if v := runtimeTraceProjNodeDisplayImpact(node); v > 0 {
		parts = append(parts, fmt.Sprintf("%.3fms", v))
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels.
	if node.DuplicatePublications > 1 {
		parts = append(parts, runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh))
	}
	if node.MergedCount > 1 {
		if zh {
			parts = append(parts, fmt.Sprintf("×%d合并(单次%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		} else {
			parts = append(parts, fmt.Sprintf("×%d merged (each %.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS))
		}
	}
	// 裁定4 applies to the target's own status rows too: the tree legend
	// promises "rows without a dominant state keep their impact-shape
	// value", and every other row family (chain / cause / adjacent /
	// background / merged) already renders it — a lock-contention self
	// row must say 锁竞争·阻塞 at a glance instead of a bare duration
	// (lock_001 customer report, 2026-07-03). Sleep rows keep their
	// dedicated wording below.
	if !node.IsSleepState() {
		stateTag := runtimeTraceProjStateKindLabel(node, zh)
		if stateTag == "" || strings.TrimSpace(node.BlockingKind) != "" ||
			runtimeTraceCausalProjectionInversionRow(node) {
			stateTag = runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		}
		if stateTag != "" {
			row.marks.mark(runtimeTraceProjMarkStateLabel)
			parts = append(parts, stateTag)
		}
	}
	if node.IsSleepState() {
		if zh {
			parts = append(parts, "窗口内主要处于等待唤醒")
		} else {
			parts = append(parts, "mostly waiting for wakeup inside the window")
		}
	}
	if node.Undrillable() {
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		if zh {
			parts = append(parts, "⛔窗口内无匹配 sched_wakeup("+node.UndrillableReason+"),无法下钻")
		} else {
			parts = append(parts, "⛔no matching sched_wakeup in the window ("+node.UndrillableReason+") — cannot drill")
		}
	}
	// NEW-3: a fold whose primary landed on the target's own state lane still
	// carries the caliber note (the peers' only display carrier).
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		parts = append(parts, runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh))
	}
	if row.EvidenceTag != "" {
		parts = append(parts, "["+row.EvidenceTag+"]")
	}
	return strings.Join(parts, " ")
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
// detail-cell callers pass nil and record nothing).
func runtimeTraceProjStateIcon(node types.TraceCausalProjectionNode, kind string, marks *runtimeTraceProjMarkSet) string {
	if kind == runtimeTraceProjTreeRowSemantic {
		marks.mark(runtimeTraceProjMarkSemanticSpan)
		return "✦"
	}
	if node.IsSleepState() {
		marks.mark(runtimeTraceProjMarkIconSleep)
		return "💤"
	}
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "running":
		marks.mark(runtimeTraceProjMarkIconRunning)
		return "⚙"
	case "runnable":
		marks.mark(runtimeTraceProjMarkIconRunnable)
		return "⏳"
	case "d_state", "io_wait", "d_sleep", "uninterruptible_sleep":
		marks.mark(runtimeTraceProjMarkIconDState)
		return "⛓"
	default:
		marks.mark(runtimeTraceProjMarkIconTransit)
		return "◦"
	}
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
// measurement that was published N times, so it must never share the upstream
// R2 sum-aggregate rendering form "×N合并(单次…)" — a reader could not tell a
// single measurement from a total. Callers fork on the typed
// Node.DuplicatePublications count (V4: one home for both fold producers).
func runtimeTraceProjDedupFoldTagText(count int, zh bool) string {
	if zh {
		return fmt.Sprintf("×%d同值合并(重复发布)", count)
	}
	return fmt.Sprintf("×%d same-value merge (duplicate publication)", count)
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
		prefix := runtimeTraceProjTreePrefix(row)
		note := ""
		if zh {
			note = fmt.Sprintf("…省略%d个链路节点…", row.Omitted)
			if row.CyclePeriod > 0 && row.CycleCount > 0 {
				note += fmt.Sprintf("(检测到%d节点循环约%d轮)", row.CyclePeriod, row.CycleCount)
			}
			note += " 完整链路见原始 trace_query 记录"
		} else {
			note = fmt.Sprintf("…%d chain nodes omitted…", row.Omitted)
			if row.CyclePeriod > 0 && row.CycleCount > 0 {
				note += fmt.Sprintf(" (≈%d-node cycle ×%d)", row.CyclePeriod, row.CycleCount)
			}
			note += " full chain remains in the trace_query record"
		}
		return prefix + " " + note
	}
	fixed, name := runtimeTraceProjTreeLabelParts(row, zh)
	left := runtimeTraceProjTreeLabel(fixed, name, width)
	var line string
	if !row.HasData {
		// NEW-10 (§7.6): transit rows carry no bar to align — the old pad to
		// the shared column only pushed the note toward/past the row cap.
		// Render them compact; bar rows keep the aligned column.
		left = strings.TrimRight(left, " ")
		if zh {
			line = left + " (链路中转,本轮无独立影响行)"
		} else {
			line = left + " (chain transit, no standalone impact row this run)"
		}
	} else {
		line = runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
	}
	// H11 small-cycle annotation: the canonical subject already appeared earlier
	// on the rendered chain — say so at end of row instead of letting the repeat
	// read as a distinct thread. Display-only; never truncates the chain. F-2:
	// on a multi-line render (↳ continuation lane) the suffix belongs to the
	// MAIN row, never to a continuation chunk.
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

// runtimeTraceProjRecursSuffix is the H11 small-cycle end-of-row annotation.
// Shared by the append site above and the NEW-10 width reserve in
// runtimeTraceProjRowLineWithMetrics so the two can never drift.
func runtimeTraceProjRecursSuffix(zh bool) string {
	if zh {
		return " ↺(线程在链上重复出现)"
	}
	return " ↺ (recurs on chain)"
}

func runtimeTraceProjStanzaRowLine(row runtimeTraceProjTreeRow, width int, denom float64, windowMode, zh bool) string {
	// NEW-10 (§7.6 记号区规整): stanza rows carry the same single state glyph
	// slot as tree rows (typed StateKind switch, ◦ default) — previously they
	// rendered glyph-less, so same-depth rows had varying emoji counts and web
	// font drift shifted them by varying offsets. The fixed part mirrors the
	// width pass in runtimeTraceProjTreeLabelColumn exactly.
	left := runtimeTraceProjTreeLabel("    "+runtimeTraceProjStateIcon(row.Node, row.Kind, row.marks)+" ",
		runtimeTraceProjRowName(row, zh), width)
	return runtimeTraceProjRowLineWithMetrics(left, row, denom, windowMode, zh)
}

// runtimeTraceProjRowLineWithMetrics assembles label + bar/ms cells + tags
// under the total row-width cap (§7.30.2 C4b B4): the tag segment gets the
// remaining budget and elides secondary tags when it would overflow. F-2
// (§7.7 回访聚焦复核 2026-07-04): a fit still over budget means only NoTruncate
// carriers / floored stubs remain — ContinuationLane carriers (the NEW-3
// caliber note, the D3 composition split) then move onto their own
// prefix-aligned ↳ continuation line(s), end-first, re-fitting after each move
// so droppable tags regain any freed room; the main row returns under the cap
// and the carrier content survives byte-identical (wrap, never truncation).
// The return value is multi-line in that case (main row + continuations).
func runtimeTraceProjRowLineWithMetrics(left string, row runtimeTraceProjTreeRow, denom float64, windowMode, zh bool) string {
	base, tags := runtimeTraceProjRowMetricParts(row, denom, windowMode, zh)
	if len(tags) == 0 {
		return left + " " + base
	}
	budget := runtimeTraceProjTreeRowMaxWidth -
		runewidth.StringWidth(left) - 1 - runewidth.StringWidth(base) - 2
	if row.RecursOnChain {
		// NEW-10: the H11 ↺ suffix appends after fitting — reserve its display
		// width so the finished row still holds the cap.
		budget -= runewidth.StringWidth(runtimeTraceProjRecursSuffix(zh))
	}
	fitted := runtimeTraceProjFitTags(tags, budget)
	if runewidth.StringWidth(fitted) <= budget {
		return left + " " + base + "  " + fitted
	}
	remaining := append([]runtimeTraceProjTag(nil), tags...)
	var moved []runtimeTraceProjTag
	for runewidth.StringWidth(fitted) > budget {
		idx := -1
		for i := len(remaining) - 1; i >= 0; i-- {
			if remaining[i].ContinuationLane {
				idx = i
				break
			}
		}
		if idx < 0 {
			break // only non-continuation stubs left: legacy over-cap floor
		}
		moved = append([]runtimeTraceProjTag{remaining[idx]}, moved...)
		remaining = append(remaining[:idx], remaining[idx+1:]...)
		fitted = runtimeTraceProjFitTags(remaining, budget)
	}
	line := left + " " + base + "  " + fitted
	if len(moved) == 0 {
		return line
	}
	indent := runtimeTraceProjRowContinuationIndent(row)
	for _, tag := range moved {
		for _, cont := range runtimeTraceProjNoteContinuationLines(indent, tag.Text) {
			line += "\n" + cont
		}
	}
	return line
}

// runtimeTraceProjRowContinuationIndent derives the rail indent a continuation
// line renders under (F-2): the row's ancestor rails verbatim, then the row's
// own rail (│ while siblings follow, blank after a └─ row) so the tree drawing
// stays intact under the wrapped note. Stanza rows (◇/▒ families, no rails)
// indent by their fixed 4-cell lead + icon slot.
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

// runtimeTraceProjNoteContinuationLines wraps a ContinuationLane carrier onto
// continuation line(s) under the NEW-10 row cap (F-2, same lane the NEW-10
// header wrap already uses for the scale note): "↳ " marks the first line,
// later lines align under it; every line holds the cap and the chunk
// concatenation is byte-identical to the original text (wrap only — a
// truncated carrier would delete values with no other fence home).
func runtimeTraceProjNoteContinuationLines(indent, text string) []string {
	lead := indent + "↳ "
	width := runtimeTraceProjTreeRowMaxWidth - runewidth.StringWidth(lead)
	if width < runtimeTraceProjTreeNameMinWidth {
		width = runtimeTraceProjTreeNameMinWidth
	}
	var out []string
	prefix := lead
	runes := []rune(text)
	for len(runes) > 0 {
		w, i := 0, 0
		for i < len(runes) {
			rw := runewidth.RuneWidth(runes[i])
			if i > 0 && w+rw > width {
				break
			}
			w += rw
			i++
		}
		out = append(out, prefix+string(runes[:i]))
		prefix = indent + "  "
		runes = runes[i:]
	}
	return out
}

// runtimeTraceProjTag is one tag cell of a tree/stanza row with its typed
// elision class (§7.30.2 C4b B4). DropOrder is assigned at the tag's build
// site — a precise typed signal, never re-derived from the rendered text.
type runtimeTraceProjTag struct {
	Text string
	// DropOrder: 0 = load-bearing, never dropped (the leading state/impact
	// attribution, ⚠ cross-window and ⛔ missing_wakeup markers, the [E#]
	// evidence reference); 1 = typed action lane (drops last among the
	// droppable); 2 = detail extras the lossless table mirrors (chain cum,
	// merged range, impact points, attribution note, span host); 3 =
	// layer/priority chip (first to go — the detail table's 因果位置·优先级
	// column is the authoritative surface for it).
	DropOrder int
	// NoTruncate (NEW-10 §7.6): this Keep tag's full text is a carrier with no
	// display-cell substitute — the [E#] evidence reference (locator) and the
	// NEW-3 caliber note (the folded values' + evidence ids' only fence
	// carrier). The keep-truncation lane never shortens it. Set at the build
	// site only.
	NoTruncate bool
	// ContinuationLane (F-2 §7.7 回访聚焦复核 2026-07-04): this NoTruncate
	// carrier may move onto its own prefix-aligned continuation line(s) when
	// the fitted row would exceed the row cap — the NEW-3 caliber note and the
	// D3 composition split (both grow without a per-row bound: peers /
	// wording). The [E#] locator stays on its main row (it is small and IS the
	// row's identity). Wrap only, never truncation: the continuation chunks
	// concatenate byte-identically to the tag text. Set at the build site only.
	ContinuationLane bool
	// MinKeep (NEW-10): display floor for the keep-truncation lane; 0 uses
	// runtimeTraceProjTreeKeepTagMinWidth. Build sites raise it when the tag's
	// leading marker phrase (the token the dynamic legend explains, NEW-7) is
	// wider than the default floor — truncation must never eat the marker.
	MinKeep int
}

const (
	runtimeTraceProjTagKeep      = 0
	runtimeTraceProjTagAction    = 1
	runtimeTraceProjTagExtra     = 2
	runtimeTraceProjTagLayerChip = 3
)

// runtimeTraceProjFitTags joins the tag segment within the row-width budget
// (B4): when the full join overflows, droppable tags elide to a "…" marker in
// typed DropOrder (layer chip first, then table-mirrored extras end-first,
// then the action lane). Load-bearing tags — the leading state attribution
// (裁定4), ⚠/⛔ markers (gaps c/e) and the [E#] evidence reference — always
// survive being DROPPED. NEW-10 (§7.6, 单行完整性优先): when the drop lane is
// exhausted and the Keep-only join still overflows, truncatable Keep tags
// display-truncate end-first (runewise "…", same display-cell mechanism as
// the label lane) down to their typed floors; typed NoTruncate carriers (the
// [E#] ref, the NEW-3 caliber note) are never shortened. The detail table +
// evidence index stay the lossless surface for everything elided or shaved.
func runtimeTraceProjFitTags(tags []runtimeTraceProjTag, budget int) string {
	if len(tags) == 0 {
		return ""
	}
	texts := make([]string, len(tags))
	for i := range tags {
		texts[i] = tags[i].Text
	}
	assemble := func(dropped map[int]bool, elisionMark bool) string {
		var parts []string
		elided := false
		for i := range tags {
			if dropped[i] {
				if !elided && elisionMark {
					parts = append(parts, "…")
					elided = true
				}
				continue
			}
			parts = append(parts, texts[i])
		}
		return strings.Join(parts, " · ")
	}
	candidate := assemble(nil, true)
	if runewidth.StringWidth(candidate) <= budget {
		return candidate
	}
	dropped := map[int]bool{}
	for order := runtimeTraceProjTagLayerChip; order >= runtimeTraceProjTagAction; order-- {
		for i := len(tags) - 1; i >= 0; i-- {
			if tags[i].DropOrder != order {
				continue
			}
			dropped[i] = true
			candidate = assemble(dropped, true)
			if runewidth.StringWidth(candidate) <= budget {
				return candidate
			}
		}
	}
	// NEW-10 keep-truncation lane: shave truncatable Keep tags end-first, each
	// only as far as needed and never below its floor (default keeps a 4-glyph
	// attribution prefix; build sites raise it to protect a wider marker
	// phrase). NoTruncate carriers are skipped entirely.
	truncatedAny := false
	for i := len(tags) - 1; i >= 0; i-- {
		if dropped[i] || tags[i].NoTruncate {
			continue
		}
		floor := tags[i].MinKeep
		if floor <= 0 {
			floor = runtimeTraceProjTreeKeepTagMinWidth
		}
		w := runewidth.StringWidth(texts[i])
		if w <= floor {
			continue
		}
		target := w - (runewidth.StringWidth(candidate) - budget)
		if target < floor {
			target = floor
		}
		texts[i] = strings.TrimRight(runtimeTraceProjPadDisplay(tags[i].Text, target), " ")
		truncatedAny = true
		candidate = assemble(dropped, true)
		if runewidth.StringWidth(candidate) <= budget {
			return candidate
		}
	}
	// Every floor applied and still over: a display-truncated tag already ends
	// in "…", so the standalone elision marker is redundant disclosure — drop
	// it when that alone brings the row under the cap.
	if truncatedAny && len(dropped) > 0 {
		if slim := assemble(dropped, false); runewidth.StringWidth(slim) <= budget {
			return slim
		}
	}
	// Readability floor: only load-bearing stubs and NoTruncate carriers remain
	// — keep them even over budget; state/⚠/⛔/E# must never be dropped.
	return candidate
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
	b.WriteString(fmt.Sprintf(" %9.3fms", impact))
	if crossThread {
		b.WriteString(runtimeTraceProjCrossThreadAggregateSuffix(node, denom, windowMode, zh))
	}
	if windowMode && denom > 0 && impact > 0 && !crossThread {
		b.WriteString(fmt.Sprintf(" %3.0f%%", impact/denom*100))
		// H8: an over-window share (cross-CPU / multi-span cumulative values can
		// legitimately exceed the wall-clock window) must not run naked — the bar
		// is already capped, so the number carries the explanation inline. The
		// *1.001 tolerance mirrors runtimeTraceProjCrossWindow: WindowMS comes
		// from an anchor float subtraction, so an EXACT whole-window projection
		// (101.000ms vs 100.9999…ms) must not read as "over the window" (V3,
		// customer revisit 2026-07-03).
		if impact > denom*1.001 {
			if zh {
				b.WriteString("(跨CPU/多段累计)")
			} else {
				b.WriteString(" (multi-CPU/multi-span cumulative)")
			}
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
	if stateTag != "" {
		// NEW-7: the state/impact-shape tag is Keep-class (never elided), so
		// marking at append time cannot record a tag the width fit later drops.
		row.marks.mark(runtimeTraceProjMarkStateLabel)
		tags = append(tags, runtimeTraceProjTag{Text: stateTag, DropOrder: runtimeTraceProjTagKeep})
	}
	// V3 (customer revisit 2026-07-03): a background row whose projection covers
	// ≥99% of the window — without exceeding it — waited out the whole window:
	// the renderer face of the producer-side Q2 idle whole-window-sleeper fold
	// signal. Over-window values (H8 tolerance: > denom*1.001) are the
	// multi-CPU cumulative shape — an ACTIVE burst, never tagged idle. F3: the
	// full judgment (incl. the typed wait-family StateKind guard) lives in the
	// shared helper; the detail table mirrors the same call.
	if windowMode && runtimeTraceProjWholeWindowIdleRow(row, denom) {
		text := "整窗等待(疑似空闲)"
		marker := "整窗等待"
		if !zh {
			text = "whole-window wait (likely idle)"
			marker = "whole-window wait"
		}
		// NEW-10: the keep-truncation floor protects the marker phrase; the
		// full annotation stays lossless on the detail table's shape cell.
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep,
			MinKeep: runewidth.StringWidth(marker) + 1})
	}
	// §7.30.3 D3: the inversion composite shows its gated composition — the
	// split is load-bearing and never elides. NEW-10: also NoTruncate — the
	// two gated numbers have no other display carrier (same sanctioned
	// row-cap overflow class as the NEW-3 caliber note).
	if runtimeTraceCausalProjectionInversionRow(node) &&
		(node.GatedRunnableMS > 0 || node.GatedRunningDeficitMS > 0) {
		text := fmt.Sprintf("影响构成: 可运行等待 %.3fms + 运行折算 %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
		if !zh {
			text = fmt.Sprintf("composition: runnable %.3fms + discounted running %.3fms", node.GatedRunnableMS, node.GatedRunningDeficitMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep,
			NoTruncate: true, ContinuationLane: true})
	}
	// §7.30.3 D1: the parsed holder site is auditable detail — droppable on
	// width pressure; the raw record keeps it lossless.
	if site := strings.TrimSpace(node.BlockingHolderSite); site != "" {
		text := "持有点 " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		if !zh {
			text = "held at " + runtimeTraceCausalProjectionCompactCellText(site, 40)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	layer := runtimeTraceProjCausalPositionLayerCell(node, zh, row.FlatChain)
	priority := runtimeTraceCausalProjectionPriorityCell(node, zh)
	if row.Kind != runtimeTraceProjTreeRowBackground {
		// background stanza header already states the layer; keep those rows lean
		tags = append(tags, runtimeTraceProjTag{Text: "‹" + layer + "›" + priority, DropOrder: runtimeTraceProjTagLayerChip})
	}
	if action := runtimeTraceCausalProjectionActionCell(node, zh); action != "" &&
		row.Kind != runtimeTraceProjTreeRowBackground {
		tags = append(tags, runtimeTraceProjTag{Text: action, DropOrder: runtimeTraceProjTagAction})
	}
	if node.CumulativeImpactMS > 0 && impact > 0 && node.CumulativeImpactMS != impact {
		text := fmt.Sprintf("链上累计%.3fms", node.CumulativeImpactMS)
		if !zh {
			text = fmt.Sprintf("chain cum %.3fms", node.CumulativeImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	// RF2b/V4: the duplicate-publication fold (single measurement) and the R2
	// sum aggregate are independent typed signals with distinct labels.
	if node.DuplicatePublications > 1 {
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh), DropOrder: runtimeTraceProjTagExtra})
	}
	if node.MergedCount > 1 {
		var text string
		if runtimeTraceProjSubjectlessFoldRow(node) {
			// V3: the R3 cross-thread fold publishes the member MAX — say so
			// instead of letting the ×N read as the sum form.
			text = fmt.Sprintf("×%d合并·各%.3f–%.3fms(取最大值,不求和)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			if !zh {
				text = fmt.Sprintf("×%d merged · each %.3f–%.3fms (max shown, not summed)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		} else {
			text = fmt.Sprintf("×%d合并·单次%.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			if !zh {
				text = fmt.Sprintf("×%d merged · each %.3f–%.3fms", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if len(node.SecondaryObjects) > 0 {
		joined := strings.Join(node.SecondaryObjects, "/")
		text := "影响点 " + joined
		if !zh {
			text = "impact point " + joined
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if runtimeTraceProjEffectiveInherited(node) {
		text := fmt.Sprintf("有效归因%.3fms(承自等待区间,非本行实测)", node.EffectiveImpactMS)
		if !zh {
			text = fmt.Sprintf("attribution %.3fms (inherited from the wait interval, not this row)", node.EffectiveImpactMS)
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
	}
	if runtimeTraceProjCrossWindow(node) {
		row.marks.mark(runtimeTraceProjMarkCrossWindow)
		text := fmt.Sprintf("⚠跨窗(实际%.3fms)", node.ActualImpactMS)
		marker := "⚠跨窗"
		if !zh {
			text = fmt.Sprintf("⚠crosses window (actual %.3fms)", node.ActualImpactMS)
			marker = "⚠crosses window"
		}
		// NEW-10: the keep-truncation floor protects the ⚠ marker phrase (the
		// token the dynamic legend explains); the actual-ms detail stays
		// lossless on the detail table's 实际状态 column.
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep,
			MinKeep: runewidth.StringWidth(marker) + 1})
	}
	// NEW-3: the folded same-segment IO calibers' values and evidence tags live
	// ONLY on this note (plus the evidence index) — load-bearing, never elided
	// or shaved (NoTruncate). F-2: when the row cannot hold it, the note moves
	// intact onto its own ↳ continuation line(s) instead of overflowing the
	// NEW-10 row cap.
	if len(row.IOFoldPeers) > 0 {
		row.marks.mark(runtimeTraceProjMarkIOCaliberNote)
		tags = append(tags, runtimeTraceProjTag{Text: runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh),
			DropOrder: runtimeTraceProjTagKeep, NoTruncate: true, ContinuationLane: true})
	}
	if row.Kind == runtimeTraceProjTreeRowSemantic {
		parent := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(row.Node.Subject, zh))
		if parent != "" {
			text := "(span 位于 " + parent + " 内)"
			if !zh {
				text = "(span inside " + parent + ")"
			}
			tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagExtra})
		}
	}
	if node.Undrillable() {
		row.marks.mark(runtimeTraceProjMarkUndrillable)
		text := "⛔无匹配唤醒·链止"
		if !zh {
			text = "⛔no matching wakeup · chain ends"
		}
		tags = append(tags, runtimeTraceProjTag{Text: text, DropOrder: runtimeTraceProjTagKeep})
	}
	if row.EvidenceTag != "" {
		// NEW-10: the E# locator is NoTruncate — a shaved reference locates
		// nothing.
		tags = append(tags, runtimeTraceProjTag{Text: "[" + row.EvidenceTag + "]",
			DropOrder: runtimeTraceProjTagKeep, NoTruncate: true})
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
		for _, entry := range runtimeTraceProjLegendCatalog() {
			if model.Marks.has(entry.Mark) {
				lines = append(lines, entry.ZH)
			}
		}
		sections = append(sections, strings.Join(lines, "\n"))
	} else {
		lines := []string{
			"Tree reading:",
			"- Top-down = tracing upstream from the focused thread.",
			"- Durations, ranks and E# tags locate structured trace_query evidence — never extra speculation.",
		}
		for _, entry := range runtimeTraceProjLegendCatalog() {
			if model.Marks.has(entry.Mark) {
				lines = append(lines, entry.EN)
			}
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if len(model.Background) == 0 {
		if zh {
			sections = append(sections, "背景层: 当前结构化 trace_query 结果没有产出可承重的 off-chain/background 行;这不等于背景没有影响,只表示本轮证据没有给出可审计的背景统计。")
		} else {
			sections = append(sections, "Background layer: the structured trace_query result did not produce load-bearing off-chain/background rows. This does not prove there was no background influence; it only means this run lacks auditable background statistics.")
		}
	}
	return strings.Join(sections, "\n\n")
}

// runtimeTraceProjConclusionLine is FACT-ONLY: subject, cause, magnitudes and
// the typed drilldown target. It never emits advice/should-sentences — the
// system must not ghost-write the user-facing recommendation surface.
func runtimeTraceProjConclusionLine(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	primary := runtimeTraceProjLeadPrimary(projection, model.TrunkLen)
	if primary == nil {
		if len(runtimeTraceCausalProjectionPrimaryRoots(projection)) == 0 {
			return ""
		}
		// Every primary candidate was demoted to the background stanza (§7.30
		// 裁定1) — the lead must say so instead of naming a demoted row as the
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
	if target := strings.TrimSpace(primary.DrilldownTarget); target != "" && primary.IsSleepState() {
		if zh {
			b.WriteString(",下钻到 " + target)
		} else {
			b.WriteString(", drills down to " + target)
		}
	} else if primary.IsSleepState() && primary.Undrillable() {
		if zh {
			b.WriteString(",⛔窗口内无匹配唤醒、无法继续下钻")
		} else {
			b.WriteString(", ⛔ no matching wakeup in the window — cannot drill further")
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

// runtimeTraceProjLeadSelectionValue is the rank-fallback ordering key for the
// conclusion line: the single-instance effective attribution. A ×N aggregate
// contributes its per-instance max — the merged SUM is a window-projection
// total across instances and must never compete against single-instance hard
// facts (V1, customer revisit 2026-07-03).
func runtimeTraceProjLeadSelectionValue(node types.TraceCausalProjectionNode) float64 {
	if node.MergedCount > 1 {
		return node.MergedMaxMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return runtimeTraceProjNodeDisplayImpact(node)
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
	if attributed := runtimeTraceProjDepth1Cumulative(model); attributed > 0 {
		// V2 (customer revisit 2026-07-03): when the 🎯 target published its own
		// state rows, the coverage denominator is the TARGET SYMPTOM duration,
		// not the whole window — a target that slept 11.7ms of a 101ms window
		// once rendered as "残差 97%". Falls back to the whole window (wording
		// unchanged) when no self-state row exists or the attribution exceeds
		// the symptom duration.
		if symptom := runtimeTraceProjTargetSymptomMS(model); symptom > 0 && attributed <= symptom {
			residual := symptom - attributed
			if zh {
				fmt.Fprintf(&b, " 目标睡眠/阻塞 %.3fms 中 on-chain 已归因 %.3fms(%.0f%%),未归因 %.3fms(%.0f%%)。",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			} else {
				fmt.Fprintf(&b, " Of the target's %.3fms sleep/blocked time, on-chain attributed %.3fms (%.0f%%), unattributed %.3fms (%.0f%%).",
					symptom, attributed, attributed/symptom*100, residual, residual/symptom*100)
			}
			b.WriteString(runtimeTraceProjResidualOwnCaliberNote(model, residual, zh))
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
// rows whose typed StateKind is in the sleep/D/blocked wait family enter the
// denominator: a 10ms sleep with an 8ms binder wait nested inside it is a
// 10ms symptom (never 18ms), and a running/runnable self row is not
// sleep/blocked time at all. Precise typed signals only (Role enum +
// StateKind enum), never prose.
func runtimeTraceProjTargetSymptomMS(model runtimeTraceProjTreeModel) float64 {
	total := 0.0
	for _, row := range model.SelfRows {
		if row.Node.Role == types.TraceCausalRoleCausalHop {
			continue // blocked-wait/attribution hop view: wall clock already counted by its enclosing state segment
		}
		if !runtimeTraceProjWaitFamilyStateKind(row.Node) {
			continue // running/runnable/stateless rows are not sleep/blocked symptom time
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
//     stayed OUT of the symptom denominator (causal_hop views / non-wait-family
//     states — the same two typed exclusions runtimeTraceProjTargetSymptomMS
//     applies): same-wall-clock re-descriptions, not extra time.
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
		if row.Node.Role == types.TraceCausalRoleCausalHop || !runtimeTraceProjWaitFamilyStateKind(row.Node) {
			consider(row)
		}
	}
	return best, tag, found
}

// runtimeTraceProjWaitFamilyStateKind reports whether the node's typed dominant
// scheduler state belongs to the sleep/D/blocked wait family. Precise typed
// enum check (never a prose substring): running/runnable and rows WITHOUT a
// StateKind (e.g. metric aggregates that never exposed a dominant state) are
// NOT waits. Shared by the target-symptom denominator (F1) and the
// whole-window idle annotation (F3) so the two gates cannot drift.
func runtimeTraceProjWaitFamilyStateKind(node types.TraceCausalProjectionNode) bool {
	switch strings.TrimSpace(strings.ToLower(node.StateKind)) {
	case "s_sleep", "sleep", "sleep_wait",
		"d_state", "d_sleep", "uninterruptible_sleep", "io_wait":
		return true
	}
	return false
}

// runtimeTraceProjWholeWindowIdleRow is the SINGLE definition of the V3
// "整窗等待(疑似空闲)" annotation — the tree stanza tag and the detail-table
// mirror both call it (F3: two hand-synced copies were the drift risk). True
// only for a background row whose typed dominant state is in the sleep/D/
// blocked wait family AND whose projection covers ≥99% of the window without
// exceeding it (H8 tolerance: over-window cumulative rows are the multi-CPU
// ACTIVE shape, never idle). A whole-window running CPU hog or a stateless
// cpu·ms aggregate row that happens to ≈ the window never takes the tag.
func runtimeTraceProjWholeWindowIdleRow(row runtimeTraceProjTreeRow, windowMS float64) bool {
	if row.Kind != runtimeTraceProjTreeRowBackground || windowMS <= 0 {
		return false
	}
	if !runtimeTraceProjWaitFamilyStateKind(row.Node) {
		return false
	}
	impact := runtimeTraceProjNodeDisplayImpact(row.Node)
	return impact >= windowMS*0.99 && impact <= windowMS*1.001
}

func runtimeTraceProjDepth1Cumulative(model runtimeTraceProjTreeModel) float64 {
	if v := runtimeTraceProjChainDepthCumulative(model, 1); v > 0 {
		return v
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
		if v > max {
			max = v
		}
	}
	return max
}

// --- lossless detail table ------------------------------------------------------

func runtimeTraceProjDetailTable(model runtimeTraceProjTreeModel, zh bool) ([]string, []types.AnswerBlockItem) {
	// D2 audit fidelity: the 类型 column keeps the raw English type token that
	// the zh tree label replaced (both language surfaces carry the column).
	columns := []string{"层级", "因果位置·优先级", "节点/原因", "类型", "关系 ▸ 影响点", "影响形态", "窗口投影", "链上累计", "有效归因", "实际状态", "证据·置信"}
	if !zh {
		columns = []string{"Layer", "Causal position · priority", "Node / cause", "Type", "Relation ▸ impact point", "Impact shape", "Window projection", "Chain total", "Attribution", "Actual state", "Evidence · confidence"}
	}
	dash := "—"
	msCell := func(v float64) string {
		if v <= 0 {
			return dash
		}
		return fmt.Sprintf("%.3fms", v)
	}
	// Flat mode (no wakeup-path trunk): EVERY row is depthless, so per-row
	// "depth unresolved" / "impact point unresolved" placeholders carry zero
	// information and spam the table — the header already names the flat cause
	// (§7.30 裁定3) and the causal-position column renders the flat marker
	// (平铺/链不可上溯, CMP-7a) instead of on-chain. The placeholders render
	// only when a trunk exists and a named row cannot attach.
	flat := strings.TrimSpace(model.Target) == ""
	var rows []types.AnswerBlockItem
	addRow := func(row runtimeTraceProjTreeRow) {
		if row.Kind == runtimeTraceProjTreeRowOmitted || !row.HasData {
			return
		}
		node := row.Node
		layer := runtimeTraceProjDetailLayerCell(row, zh, flat)
		position := runtimeTraceProjCausalPositionLayerCell(node, zh, row.FlatChain) + " · " + runtimeTraceCausalProjectionPriorityCell(node, zh)
		name := runtimeTraceCausalProjectionNodeSubjectCell(node, zh)
		// RF2b/V4: the duplicate-publication fold (single measurement) and the
		// R2 sum aggregate are independent typed signals with distinct labels.
		if node.DuplicatePublications > 1 {
			name += " " + runtimeTraceProjDedupFoldTagText(node.DuplicatePublications, zh)
		}
		if node.MergedCount > 1 {
			if runtimeTraceProjSubjectlessFoldRow(node) {
				// V3: the cross-thread fold publishes the member MAX, and (V5)
				// the table cell keeps the same member roster the tree fold line
				// already shows — "对端线程未解析 ×6" lost every thread identity.
				if zh {
					name += fmt.Sprintf(" ×%d(各%.3f–%.3fms,取最大值)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				} else {
					name += fmt.Sprintf(" ×%d (each %.3f–%.3fms, max shown)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
				}
				name += runtimeTraceProjMergedSubjectsSuffix(node, zh)
			} else {
				name += fmt.Sprintf(" ×%d(%.3f–%.3fms)", node.MergedCount, node.MergedMinMS, node.MergedMaxMS)
			}
		}
		if node.Undrillable() {
			name += " ⛔"
		}
		// NEW-3 mirror on the lossless surface: the folded calibers no longer
		// render as separate table rows, so the primary row's cell carries
		// their values + evidence ids (the evidence index keeps the locators).
		if len(row.IOFoldPeers) > 0 {
			name += "(" + runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh) + ")"
		}
		// NEW-3/F2: the depthless own-process IO row carries the typed own edge
		// stamped at build time — the relation cell's edge switch renders it as
		// 自身进程IO, consistent with the fence's ├─自身─ edge and the legend.
		relation := runtimeTraceProjDetailRelationCell(row, zh, flat)
		// §7.30.2 C4b: the typed R1-merge impact points (SecondaryObjects) must
		// stay lossless on the table — the tree's 影响点 tag is width-elidable.
		if len(node.SecondaryObjects) > 0 {
			joined := strings.Join(node.SecondaryObjects, "/")
			if zh {
				relation += " ▸ 影响点 " + joined
			} else {
				relation += " ▸ impact point " + joined
			}
		}
		shape := runtimeTraceCausalProjectionImpactShapeCell(node, zh)
		if shape == "" {
			shape = dash
		}
		// V3: mirror the whole-window-wait annotation on the lossless surface —
		// F3: the SAME shared judgment as the stanza tag (typed wait-family
		// StateKind guard included; over-window cumulative rows are the H8
		// shape and never take it).
		if runtimeTraceProjWholeWindowIdleRow(row, model.WindowMS) {
			idle := "整窗等待(疑似空闲)"
			if !zh {
				idle = "whole-window wait (likely idle)"
			}
			if shape == dash {
				shape = idle
			} else {
				shape += "·" + idle
			}
		}
		// CMP-3 mirror on the lossless surface, ALL duration columns (F6,
		// adversarial review 2026-07-04): a cross-thread aggregate row's
		// 链上累计/有效归因/实际状态 mirrors previously sat naked next to an
		// annotated 窗口投影 — same typed classifier, same helper, four cells.
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
		actual := annotated(node.ActualImpactMS)
		if runtimeTraceProjCrossWindow(node) && node.ActualImpactMS > 0 {
			actual += " ⚠"
		}
		evidence := row.EvidenceTag
		if evidence == "" {
			evidence = dash
		}
		if tier := runtimeTraceProjConfidenceTier(node.Confidence, zh); tier != "" {
			evidence += " · " + tier
		}
		typeToken := runtimeTraceCausalProjectionRawTypeToken(node)
		if typeToken == "" {
			typeToken = dash
		}
		rows = append(rows, types.AnswerBlockItem{
			Cells: []string{
				layer, position,
				runtimeTraceCausalProjectionMarkdownSafe(name),
				runtimeTraceCausalProjectionMarkdownSafe(typeToken),
				runtimeTraceCausalProjectionMarkdownSafe(relation),
				shape,
				annotated(node.ImpactMS), annotated(node.CumulativeImpactMS),
				effective, actual,
				evidence,
			},
			CitationRef: -1,
		})
	}
	for _, row := range model.SelfRows {
		addRow(row)
	}
	for _, row := range model.TreeRows {
		addRow(row)
	}
	for _, row := range model.Adjacent {
		addRow(row)
	}
	for _, row := range model.Background {
		addRow(row)
	}
	return columns, rows
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
		if zh {
			intro += " 全部证据位于 `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`,各条只列行号区间。"
		} else {
			intro += " All locators live in `" + runtimeTraceCausalProjectionMarkdownSafe(sharedFile) + "`; entries list only line ranges."
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
