package tool

// §7.6 对比场景客户回访 pins (2026-07-04, trace_cmp_cust.txt — the
// bindApplication comparison rerun; docs/design/
// customer_dead_session_audit_20260703.md §7.6):
//
//   NEW-1 — the tree-legend wake edge names ONE direction, stated twice
//           consistently (该行唤醒其父行;父行依赖该行). The former wording
//           juxtaposed two opposite verbs ("唤醒/依赖其父行") and the customer
//           explicitly asked "谁唤醒谁".
//   NEW-3 — same-subject same-segment IO calibers (typed token set
//           {io_burst_episode, io_wait}, pairwise-overlapping line intervals)
//           fold into the max-impact row; the folded calibers surface as one
//           caliber note with every evidence id kept. Different subjects /
//           non-overlapping intervals never fold. As corrected by F2
//           (adversarial re-review 2026-07-04): only the DEPTHLESS
//           process-level IO row of the 🎯 target's own process (typed
//           same-trailing-pid gate, own edge stamped at build) drops the wake
//           claim — it renders ├─自身─ on the fence, 自身进程IO in the
//           relation column, and its own legend entry (three consistent
//           surfaces). The chain-ATTACHED variant (resolved ChainDepth ≥ 1)
//           keeps 唤醒 on every surface: its wake edge is data-real and
//           drives the on-chain attribution numerator.
//   NEW-5 — the enumeration primary column header forks on the verbatim
//           " -> " chain-description surface: a UNIFORM chain table (ALL
//           non-empty rows carry the arrow — F4, adversarial re-review
//           2026-07-04) reads 链路/条目; mixed and plain symbol tables keep
//           符号名称 (display-only; NEW-2 lives in the rewritten compare/cmp6
//           pins).
//   NEW-6 — the coverage line self-explains the residual vs. own-caliber
//           tension ("残差 90%" next to a 232ms own-process IO row read as a
//           contradiction): when a target-own / same-process IO caliber row
//           exists OUTSIDE the attribution numerator (typed: depthless
//           runtimeTraceProjOwnProcessIORow rows + self IO-token rows outside
//           the symptom denominator), the residual sentence appends the
//           overlap-explanation clause with the NEW-3 grouped primary value
//           and its evidence tag verbatim. No such row → byte-identical line.
//   NEW-7 — the 树读法 legend is dynamic: typed marks recorded at each fence
//           emission site; legend = 2 fixed head clauses + only the emitted
//           catalog entries (stable order). Pinned bidirectionally over
//           representative shapes plus a catalog-completeness pin
//           (mark-constant set == catalog key set).
//   NEW-9 — capacity-truncation disclosure (adversarial re-review 2026-07-04):
//           a trace_query result with a non-empty typed compaction channel
//           stamps its typed observations with capacity_truncated=true (single
//           producer helper); the projection compile lifts the note into
//           TraceCausalProjection.CapacityTruncated and the projection
//           section's evidence-index header discloses the truncation. Present
//           and absent shapes both pinned.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- NEW-1: legend wake-direction wording -----------------------------------

// Updated for NEW-7 (dynamic legend, same §7.6 batch): the wake entry renders
// only when the fence actually emitted a wake edge, so the pin now renders a
// real projection (the revisit 6.0 IO shape carries depth-1 wake rows) and the
// fence BEFORE reading the lead. The NEW-1 wording itself is verbatim-preserved
// in the catalog's wake entry.
func TestTraceProjectionLegendWakeDirectionUnambiguous(t *testing.T) {
	projection := revisit76IOProjection()
	zhModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(zhModel, true); !strings.Contains(fence, "唤醒─") {
		t.Fatalf("fixture must emit a wake edge for the legend entry to render:\n%s", fence)
	}
	zhLead := runtimeTraceProjLeadText(projection, zhModel, "zh", true)
	if !strings.Contains(zhLead, "- `└─唤醒─` = 该行唤醒其父行(父行的等待由该行结束;父行依赖该行)。") {
		t.Fatalf("zh legend must state the wake direction twice, consistently:\n%s", zhLead)
	}
	if strings.Contains(zhLead, "该行唤醒/依赖其父行") {
		t.Fatalf("the ambiguous two-opposite-verbs wording must be gone:\n%s", zhLead)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	runtimeTraceProjTreeFence(enModel, false)
	enLead := runtimeTraceProjLeadText(projection, enModel, "en", false)
	if !strings.Contains(enLead, "- `└─wakes─` = this row WAKES its parent row (the parent's wait ends on this row; the parent depends on it).") {
		t.Fatalf("en legend must mirror the unambiguous direction:\n%s", enLead)
	}
	if strings.Contains(enLead, "wakes/feeds its parent") {
		t.Fatalf("the old en wording must be gone:\n%s", enLead)
	}
}

// --- NEW-3: same-subject IO caliber fold -------------------------------------

func revisit76IONode(id, subject, token string, impact float64, lineStart, lineEnd int) types.TraceCausalProjectionNode {
	node := types.TraceCausalProjectionNode{
		Role:               types.TraceCausalRoleRootCauseContext,
		EvidenceID:         id,
		Subject:            subject,
		Object:             token,
		TypeToken:          token,
		ChainRelevance:     "on_chain",
		ChainDepth:         1,
		ImpactMS:           impact,
		CumulativeImpactMS: impact,
		LineStart:          lineStart,
		LineEnd:            lineEnd,
		SupportRefs:        []string{fmt.Sprintf("6.0B138_3900.sys.systrace:%d-%d", lineStart, lineEnd)},
		Confidence:         0.8,
	}
	// WO-N1 (SMR-1 批, 2026-07-12) + P1-1 (修复轮 2026-07-13): the NEW-3
	// connectivity gate reads the members' WALL-CLOCK segments (行号包络连通
	// 判被禁), and the PRODUCTION lanes emit item.StartTs/EndTs on the
	// ObservationSpan — the fixture ts pair models that real emission
	// (derived from the same line geometry so the witness overlap relations
	// hold).
	if lineStart > 0 && lineEnd >= lineStart {
		node.StartTs = 100.0 + float64(lineStart)*1e-5
		node.EndTs = 100.0 + float64(lineEnd)*1e-5
	}
	return node
}

// revisit76IOProjection mirrors the 6.0 revisit shape: 🎯 main-21538, and the
// process-level subject com.xs.fm.lite-21538 publishing FOUR overlapping IO
// rows (io_burst_episode 232.428/226.153 + io_wait 112.011/107.672).
func revisit76IOProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-21538"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.5,
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("io-b1", "com.xs.fm.lite-21538", "io_burst_episode", 232.428, 1000, 2000),
			revisit76IONode("io-b2", "com.xs.fm.lite-21538", "io_burst_episode", 226.153, 1100, 1900),
			revisit76IONode("io-w1", "com.xs.fm.lite-21538", "io_wait", 112.011, 1200, 1800),
			revisit76IONode("io-w2", "com.xs.fm.lite-21538", "io_wait", 107.672, 1250, 1750),
		},
	}
}

func TestTraceProjectionSameSubjectIOCalibersFoldIntoPrimaryRow(t *testing.T) {
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(revisit76IOProjection(), evidence, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// The revisit shape: 4 IO rows → 1 primary row + one caliber note.
	if !strings.Contains(fence, "232.428") {
		t.Fatalf("the max-impact caliber must stay the primary row:\n%s", fence)
	}
	// PTV5 C09/C16 (#68): zh labels ride the D4 combined form; the longer note
	// may T3-wrap between tokens, so the pin checks the caliber list and the
	// evidence roster as two whole substrings (tokens themselves never split).
	// EVOLUTION RECORD (IOFAM-SELF 件②, 2026-07-12): the roster is layered —
	// each member wears its measuring-layer word (完成端到端/调度等待).
	if !strings.Contains(fence, "同段IO另有 完成端到端·IO突发（io_burst_episode） 226.153ms") ||
		!strings.Contains(fence, "调度等待·iowait（io_wait）") ||
		!strings.Contains(fence, "112.011/107.672ms 等口径;证据") ||
		!strings.Contains(fence, "E2、E3、E4") {
		t.Fatalf("the folded calibers must surface as ONE note with all evidence ids:\n%s", fence)
	}
	// Folded values appear exactly once (inside the note) — no sibling rows.
	for _, v := range []string{"226.153", "112.011", "107.672"} {
		if strings.Count(fence, v) != 1 {
			t.Fatalf("folded caliber %s must not render its own row:\n%s", v, fence)
		}
	}
	// Every caliber (primary + 3 folded) keeps an evidence-index entry.
	if got := len(evidence.order); got != 4 {
		t.Fatalf("all four IO observations must stay on the evidence index, got %d", got)
	}
}

func TestTraceProjectionIOFoldDetailTableMirrorAndChainAttachedKeepsWake(t *testing.T) {
	// F2(a): revisit76IOProjection's IO rows are chain-ATTACHED (ChainDepth 1),
	// so their wake edge is data-real and BOTH surfaces keep 唤醒 — the pre-F2
	// NEW-3 rewrite to 自身进程IO on this shape contradicted the fence.
	model := buildRuntimeTraceProjTreeModel(revisit76IOProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "唤醒─") || strings.Contains(fence, "自身─") {
		t.Fatalf("chain-attached IO row keeps the data-real wake edge on the fence:\n%s", fence)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 1 {
		t.Fatalf("the fold keeps one primary detail row (transit rows carry no data): %+v", rows)
	}
	// PTV4 T10: the caliber-note and relation mirrors live in the (b) vertical
	// lossless blocks (the (a) key table carries the duration quad only).
	full := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(full, "同段IO口径: 同段IO另有 完成端到端·IO突发（io_burst_episode） 226.153ms、调度等待·iowait（io_wait） 112.011/107.672ms 等口径") {
		t.Fatalf("the lossless surface must mirror the caliber note on the primary block:\n%s", full)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 关系 ▸ 影响点: 唤醒 ▸ … → split "- 关系: 唤醒 <parent>" line (明细块)
	if !strings.Contains(full, "- 关系: 唤醒 main-21538") {
		t.Fatalf("chain-attached IO row keeps the wake relation (fence-consistent):\n%s", full)
	}
}

func TestTraceProjectionDepthlessOwnProcessIORowThreeSurfaceConsistency(t *testing.T) {
	// F2(b): the DEPTHLESS own-process IO caliber row (revisit 6.0 residual
	// shape) renders the own edge on all three surfaces — fence ├─自身─ (never
	// 唤醒─ on that row), relation column 自身进程IO, and the legend's own
	// entry — instead of the pre-F2 fence/table/legend contradiction.
	projection := revisit76ResidualProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "自身─") {
		t.Fatalf("depthless own-process IO row must draw the own edge:\n%s", fence)
	}
	if strings.Contains(fence, "唤醒─") {
		t.Fatalf("no wake edge exists in this shape — the own row must not claim one:\n%s", fence)
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "- `├─自身─` = 目标自身/同进程的口径行(同段墙钟的另一口径),非唤醒边。") {
		t.Fatalf("legend must explain the own edge:\n%s", lead)
	}
	// PTV4 T10: the relation mirror lives in the (b) vertical lossless blocks.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 关系 ▸ 影响点 → split "- 关系: …" line (明细块)
	full := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(full, "- 关系: 自身进程IO") {
		t.Fatalf("own-process IO relation must mirror the own edge:\n%s", full)
	}
	// en mirror: fence edge and relation stay isomorphic.
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "own─") || strings.Contains(enFence, "wakes─") {
		t.Fatalf("en fence must mirror the own edge:\n%s", enFence)
	}
	if enFull := runtimeTraceProjDetailFullText(enModel, false); !strings.Contains(enFull, "- relation: own-process IO") {
		t.Fatalf("en own-process IO relation mismatch:\n%s", enFull)
	}
}

func TestTraceProjectionIOFoldNegativeShapes(t *testing.T) {
	// Different subjects never fold, even with overlapping intervals.
	distinct := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "main-21538"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("a", "com.alpha-11", "io_burst_episode", 50.0, 100, 200),
			revisit76IONode("b", "com.beta-22", "io_wait", 40.0, 120, 180),
		},
	}
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(distinct, nil, true), true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("different subjects must not fold:\n%s", fence)
	}
	for _, v := range []string{"50.000", "40.000"} {
		if !strings.Contains(fence, v) {
			t.Fatalf("unfoldeed row %s must keep rendering:\n%s", v, fence)
		}
	}
	// Non-overlapping intervals (two genuinely distinct bursts) never fold.
	disjoint := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "main-21538"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("c", "com.alpha-11", "io_burst_episode", 50.0, 100, 200),
			revisit76IONode("d", "com.alpha-11", "io_wait", 40.0, 300, 400),
		},
	}
	fence = runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(disjoint, nil, true), true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("non-overlapping intervals must not fold:\n%s", fence)
	}
	// A member without a valid line interval vetoes the whole group.
	noInterval := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "main-21538"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("e", "com.alpha-11", "io_burst_episode", 50.0, 100, 200),
			revisit76IONode("f", "com.alpha-11", "io_wait", 40.0, 0, 0),
		},
	}
	fence = runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(noInterval, nil, true), true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("a missing line interval must veto the fold (fail closed):\n%s", fence)
	}
	// Non-IO tokens never enter the fold set.
	nonIO := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "main-21538"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("g", "com.alpha-11", "runnable_wait", 50.0, 100, 200),
			revisit76IONode("h", "com.alpha-11", "runnable_wait", 40.0, 120, 180),
		},
	}
	fence = runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(nonIO, nil, true), true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("non-IO tokens must not fold:\n%s", fence)
	}
}

// TestTraceProjectionIOFoldNeverCrossesChainLanes pins F-1 (§7.6 回访聚焦复核
// 2026-07-04) on the review's counter-shape: the SAME subject publishes a
// chain-ATTACHED depth-1 io_wait (112.011ms — the only depth-1 data row, its
// cumulative drives the attributed numerator) and a DEPTHLESS io_burst_episode
// (232.428ms) over overlapping lines. Pre-F-1 the subject-only grouping folded
// the chain row into the depthless max-impact primary: attributed dropped to
// 0, the fence lost the data-real wakeup row, and the NEW-6 clause inverted.
// The group key now carries the chain lane — cross-lane pairs never fold,
// same-lane pairs keep folding.
func TestTraceProjectionIOFoldNeverCrossesChainLanes(t *testing.T) {
	depthless := func(id, token string, impact float64, ls, le int) types.TraceCausalProjectionNode {
		n := revisit76IONode(id, "com.xs.fm.lite-21538", token, impact, ls, le)
		n.ChainDepth = 0
		return n
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-21538"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.5,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-sleep",
				Subject: "main-21538", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 400.0,
				LineStart: 500, LineEnd: 900,
				SupportRefs: []string{"6.0B138_3900.sys.systrace:500-900"},
				Confidence:  0.9,
			},
			revisit76IONode("io-chain", "com.xs.fm.lite-21538", "io_wait", 112.011, 1200, 1800), // ChainDepth 1
			depthless("io-own", "io_burst_episode", 232.428, 1000, 2000),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "同段IO另有") {
		t.Fatalf("cross-lane calibers must NOT fold:\n%s", fence)
	}
	if !strings.Contains(fence, "112.011") || !strings.Contains(fence, "232.428") {
		t.Fatalf("both lanes must keep their own rows:\n%s", fence)
	}
	if !strings.Contains(fence, "唤醒─") {
		t.Fatalf("the chain-attached wakeup row must stay on the fence:\n%s", fence)
	}
	if !strings.Contains(fence, "自身─") {
		t.Fatalf("the depthless row keeps the own-process edge:\n%s", fence)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: on-chain 已归因 → 链上已归因;残差中最大 → 未归因中最大 (归因族)
	if !strings.Contains(line, "链上已归因 112.011ms") {
		t.Fatalf("attributed must stay the chain lane's cumulative:\n%s", line)
	}
	// NEW-6 cites the DEPTHLESS side (the residual-overlap lane), never the
	// attributed chain row's value.
	if !strings.Contains(line, "未归因中最大 232.428ms 与自身 IO 口径行(E3)重叠解释") {
		t.Fatalf("NEW-6 clause must take the depthless side value:\n%s", line)
	}
	// Same-lane overlap beside the cross-lane pair still folds (no regression
	// of the NEW-3 pins): a second DEPTHLESS io_wait folds into the depthless
	// primary while the chain row stays untouched.
	projection.OnChainCauses = append(projection.OnChainCauses,
		depthless("io-own-wait", "io_wait", 107.672, 1250, 1750))
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段IO另有 调度等待·iowait（io_wait） 107.672ms 等口径;证据 E4") {
		t.Fatalf("same-lane depthless calibers must keep folding:\n%s", fence)
	}
	if !strings.Contains(fence, "112.011") {
		t.Fatalf("the chain-attached row must survive the same-lane fold:\n%s", fence)
	}
	if line := runtimeTraceProjWindowLine(projection, model, true); !strings.Contains(line, "链上已归因 112.011ms") {
		t.Fatalf("attributed unchanged by the same-lane fold:\n%s", line)
	}
}

func TestTraceProjectionOwnProcessRelationNegativeKeepsWake(t *testing.T) {
	// The subject's trailing pid differs from the 🎯 pid → the wake relation
	// stays exactly as before (the correction is same-pid only).
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "main-21538"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			revisit76IONode("x", "com.other-9999", "io_burst_episode", 60.0, 100, 200),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if _, rows := runtimeTraceProjDetailTable(model, true); len(rows) != 1 {
		t.Fatalf("expected the single IO detail row: %+v", rows)
	}
	// PTV4 T10: the relation mirror lives in the (b) vertical blocks.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 关系 ▸ 影响点: 唤醒 ▸ … → split "- 关系: 唤醒 <parent>" line (明细块)
	if full := runtimeTraceProjDetailFullText(model, true); !strings.Contains(full, "- 关系: 唤醒 main-21538") {
		t.Fatalf("a different-pid IO row keeps its wake relation:\n%s", full)
	}
}

// --- NEW-6: coverage-line residual vs. own-caliber self-explanation -----------

// revisit76ResidualProjection mirrors the REAL 6.0 coverage shape (unlike
// revisit76IOProjection, whose IO rows are chain-attached at depth 1 and hence
// inside the attribution numerator): a 260ms target sleep symptom, a 26ms
// depth-1 chain attribution (→ 残差 90%), and the four process-level IO caliber
// rows UNATTACHED to the chain (ChainDepth 0 → depthless), exactly the rows the
// customer read as contradicting the residual.
func revisit76ResidualProjection() types.TraceCausalProjection {
	ioNode := func(id, token string, impact float64, ls, le int) types.TraceCausalProjectionNode {
		n := revisit76IONode(id, "com.xs.fm.lite-21538", token, impact, ls, le)
		n.ChainDepth = 0
		return n
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-21538"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.5,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-sleep",
				Subject: "main-21538", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 260.0,
				LineStart: 500, LineEnd: 900,
				SupportRefs: []string{"6.0B138_3900.sys.systrace:500-900"},
				Confidence:  0.9,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "wake-worker",
				Subject: "worker-9", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 26.0, CumulativeImpactMS: 26.0,
				LineStart: 950, LineEnd: 990,
				SupportRefs: []string{"6.0B138_3900.sys.systrace:950-990"},
				Confidence:  0.85,
			},
			ioNode("io-b1", "io_burst_episode", 232.428, 1000, 2000),
			ioNode("io-b2", "io_burst_episode", 226.153, 1100, 1900),
			ioNode("io-w1", "io_wait", 112.011, 1200, 1800),
			ioNode("io-w2", "io_wait", 107.672, 1250, 1750),
		},
	}
}

func TestTraceProjectionCoverageLineExplainsOwnCaliberResidualOverlap(t *testing.T) {
	projection := revisit76ResidualProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	// The revisit 6.0 coverage shape: symptom denominator + 90% residual.
	// (Wording pin updated for RN-6 §7.9 + PTV7 canonical tokens: the
	// denominator family now includes runnable.)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 目标等待 → 关注线程等待;on-chain 已归因 → 链上已归因;残差中最大 → 未归因中最大;未计入链归因 → 未计入链上归因 (归因族/其他族)
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 260.000ms 中链上已归因 26.000ms(10%),未归因 234.000ms(90%)。") {
		t.Fatalf("coverage line must keep the symptom-denominator form:\n%s", line)
	}
	// NEW-6: the appended clause carries the NEW-3 grouped primary value
	// (232.428, the fold survivor — never a folded peer's) and its evidence tag
	// verbatim (E3 = the primary IO row's index entry).
	// 件② E# 并 merged_ids: the fold survivor tag carries the absorbed ids.
	if !strings.Contains(line, "未归因中最大 232.428ms 与自身 IO 口径行(E3(+3))重叠解释,未计入链上归因以防双计。") {
		t.Fatalf("coverage line must self-explain the own-caliber residual overlap:\n%s", line)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	en := runtimeTraceProjWindowLine(projection, enModel, false)
	if !strings.Contains(en, "Up to 232.428ms of the residual is co-explained by the own-process IO caliber row (E3(+3)); it is excluded from the chain attribution to avoid double counting.") {
		t.Fatalf("en coverage line must mirror the overlap clause:\n%s", en)
	}
}

func TestTraceProjectionCoverageLineOmitsOverlapClauseWithoutOwnCaliberRows(t *testing.T) {
	// No own-caliber rows at all → the coverage line stays byte-identical.
	projection := revisit76ResidualProjection()
	projection.OnChainCauses = projection.OnChainCauses[:2]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if strings.Contains(line, "重叠解释") {
		t.Fatalf("no own-caliber row → no overlap clause:\n%s", line)
	}
	if !strings.HasSuffix(line, "未归因 234.000ms(90%)。") {
		t.Fatalf("coverage line must end exactly at the residual sentence:\n%s", line)
	}
	// Chain-ATTACHED calibers (depth 1, revisit76IOProjection) sit inside the
	// attribution numerator — citing them as residual overlap would contradict
	// "未计入链归因", so the clause must not fire either.
	attached := revisit76IOProjection()
	attachedModel := buildRuntimeTraceProjTreeModel(attached, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if line := runtimeTraceProjWindowLine(attached, attachedModel, true); strings.Contains(line, "重叠解释") {
		t.Fatalf("chain-attached calibers are attributed, not residual overlap:\n%s", line)
	}
}

func TestTraceProjectionOverlapClauseSelfLaneAndResidualCap(t *testing.T) {
	// Self lane: the 🎯 target's OWN hop-view IO caliber row (Role=causal_hop,
	// outside the symptom denominator) carries the clause too.
	mkModel := func(chainCum float64) runtimeTraceProjTreeModel {
		return runtimeTraceProjTreeModel{
			Target:   "main-21538",
			WindowMS: 500,
			SelfRows: []runtimeTraceProjTreeRow{
				{Node: types.TraceCausalProjectionNode{Subject: "main-21538", StateKind: "s_sleep", ImpactMS: 260},
					Kind: runtimeTraceProjTreeRowSelf, HasData: true},
				{Node: types.TraceCausalProjectionNode{Subject: "main-21538", TypeToken: "io_wait",
					Role: types.TraceCausalRoleCausalHop, ImpactMS: 150},
					Kind: runtimeTraceProjTreeRowSelf, HasData: true, EvidenceTag: "E9"},
			},
			TreeRows: []runtimeTraceProjTreeRow{
				{Node: types.TraceCausalProjectionNode{Subject: "worker-9", ChainDepth: 1,
					ImpactMS: chainCum, CumulativeImpactMS: chainCum},
					Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true},
			},
		}
	}
	projection := types.TraceCausalProjection{WindowStartTs: 100.0, WindowEndTs: 100.5}
	line := runtimeTraceProjWindowLine(projection, mkModel(26), true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 残差中最大 → 未归因中最大;未计入链归因 → 未计入链上归因 (归因族)
	if !strings.Contains(line, "未归因中最大 150.000ms 与自身 IO 口径行(E9)重叠解释,未计入链上归因以防双计。") {
		t.Fatalf("self-lane hop-view IO caliber must carry the clause:\n%s", line)
	}
	// The published amount is bounded by the residual itself: a caliber row can
	// overlap attributed wall clock too, and the clause must never claim more
	// residual than exists (residual 260-200=60 < caliber 150).
	capped := runtimeTraceProjWindowLine(projection, mkModel(200), true)
	if !strings.Contains(capped, "未归因中最大 60.000ms 与自身 IO 口径行(E9)重叠解释") {
		t.Fatalf("overlap amount must cap at the residual:\n%s", capped)
	}
}

// --- NEW-7: dynamic legend + bidirectional catalog pins ------------------------

// TestTraceProjectionLegendCatalogCoversEveryMark is the catalog-completeness
// pin: the renderer's typed mark-constant set and the legend catalog's key set
// are EQUAL. Adding a mark constant (before the runtimeTraceProjMarkCount
// sentinel) without a catalog entry — or a catalog entry without a mark —
// explodes here.
func TestTraceProjectionLegendCatalogCoversEveryMark(t *testing.T) {
	entries := runtimeTraceProjLegendCatalog()
	if len(entries) != int(runtimeTraceProjMarkCount) {
		t.Fatalf("catalog has %d entries, want %d — every renderer mark needs exactly one legend entry", len(entries), int(runtimeTraceProjMarkCount))
	}
	seen := map[runtimeTraceProjMark]bool{}
	for _, entry := range entries {
		if entry.Mark < 0 || entry.Mark >= runtimeTraceProjMarkCount {
			t.Fatalf("catalog entry with out-of-range mark %d", entry.Mark)
		}
		if seen[entry.Mark] {
			t.Fatalf("duplicate catalog entry for mark %d", entry.Mark)
		}
		seen[entry.Mark] = true
		if !strings.HasPrefix(entry.ZH, "- ") || !strings.HasPrefix(entry.EN, "- ") {
			t.Fatalf("catalog entry %d must be a full '- ' legend line: %q / %q", entry.Mark, entry.ZH, entry.EN)
		}
	}
	// NEW-1 wording preserved verbatim inside the catalog's wake entry.
	for _, entry := range entries {
		if entry.Mark == runtimeTraceProjMarkEdgeWake {
			if entry.ZH != "- `└─唤醒─` = 该行唤醒其父行(父行的等待由该行结束;父行依赖该行)。" {
				t.Fatalf("NEW-1 zh wake wording must survive in the catalog: %q", entry.ZH)
			}
		}
	}
}

type revisit76LegendProbe struct{ zh, en string }

// revisit76LegendProbes maps every mark to a verbatim fence token for the
// bidirectional pin. runtimeTraceProjMarkStateLabel deliberately has no probe:
// the emitted tag text is the per-row state/shape VALUE (no single token); its
// direction-B guarantee is structural — the mark is recorded at the Keep-class
// tag append, which the width fit never elides.
func revisit76LegendProbes() map[runtimeTraceProjMark]revisit76LegendProbe {
	return map[runtimeTraceProjMark]revisit76LegendProbe{
		// PTV8-RCR-A §24.3: 🎯 → ⊚ (无亮色 hard rule).
		runtimeTraceProjMarkRootTarget: {"⊚", "⊚"},
		// ELIM-1 (RANK-U Stage 2): the ◎ overview region mark — the probe
		// scans the combined tree+overview surface (the overview is its own
		// fence rendered right under the tree).
		runtimeTraceProjMarkElimOverview: {"◎", "◎"},
		runtimeTraceProjMarkEdgeDrill:    {"下钻─", "drill─"},
		runtimeTraceProjMarkEdgeWake:     {"唤醒─", "wakes─"},
		runtimeTraceProjMarkEdgeCause:    {"成因─", "cause─"},
		runtimeTraceProjMarkEdgeOwn:      {"自身─", "own─"},
		// PTV6 #1b: the depthless on-chain lane's dedicated edge word (the mark
		// records only when the edge label actually renders — fold rows and
		// flat renders suppress the word and record nothing).
		// UXR-1 §29.36④: the edge simplified to 链上─; the probe follows the
		// relocated 行2 word (unique to the depth-unresolved shape).
		runtimeTraceProjMarkEdgeChainUnresolved: {"链上·深度未解析", "on-chain · depth unresolved"},
		runtimeTraceProjMarkSemanticSpan:        {"✦", "✦"},
		runtimeTraceProjMarkIconSleep:           {"☾", "☾"},
		runtimeTraceProjMarkIconRunnable:        {"⧖", "⧖"},
		runtimeTraceProjMarkIconRunning:         {"⚙", "⚙"},
		runtimeTraceProjMarkIconDState:          {"⛓", "⛓"},
		// PTV4 T4: the transit sense probes on its inline word (the ◦ glyph is
		// shared, so it cannot be a bidirectional probe). PTV6-D (b): the
		// no-dominant 2-word chip is RETIRED from the row face — no fence
		// probe; the mark records at the icon's own emission arm (structural,
		// like StateLabel) and direction A (mark ⇔ legend entry) still asserts.
		runtimeTraceProjMarkIconTransit:    {"中转", "transit"},
		runtimeTraceProjMarkIconNoDominant: {"", ""},
		runtimeTraceProjMarkBadge:          {"❶", "❶"},
		runtimeTraceProjMarkStateLabel:     {"", ""},
		runtimeTraceProjMarkUndrillable:    {"⊘", "⊘"},
		runtimeTraceProjMarkCrossWindow:    {"⚠实际", "⚠actual"},
		// DCS E5 (§23.1 H2): the semantic source-window share re-base tag.
		runtimeTraceProjMarkSemanticSourceWindowShare: {"来自查询窗", "from query window"},
		runtimeTraceProjMarkRecursOnChain:             {"↺", "↺"},
		runtimeTraceProjMarkChainDepthChip:            {"链上L", "chain L"},
		runtimeTraceProjMarkOmitted:                   {"省略", "nodes omitted"},
		// PTV8-RCR-B (UXA 域A #13 + 复核 6b, 2026-07-08). EVOLUTION RECORD:
		// the bar glyph █ is shared by the two scale regimes, so each entry
		// probes on its OWN ScaleNote branch token instead (the header prints
		// exactly one of them, on the same windowMode branch that picks the
		// mark) — direction B stays armed for both.
		runtimeTraceProjMarkBarScale: {"满格=窗口", "bar full = window"},
		// ×N 三式: the sum form's "×N(" prefix also opens the max form, so the
		// sum probe is the ptv4 fixture's VERBATIM sum token (language-neutral
		// by design) — deleting the sum-tag emission now reds this probe
		// instead of staying green (复核给定的突变形态).
		runtimeTraceProjMarkMergedSum:   {"3次(10.000~30.000ms)", "n=3(10.000~30.000ms)"},
		runtimeTraceProjMarkMergedDedup: {"同值", "same-value"},
		// §21 CWD disambiguation (same discipline as the verbatim sum probe
		// above): the bare "取最大" is a substring of the cross-window form's
		// "跨窗取最大", so the R3 cross-thread probe anchors on its ")" prefix
		// — ")取最大" matches the R3 tag only.
		// WF-xn (§29.52.1): the R3 form leads with the thread-count word —
		// 「N线程取最大(」 zh / "-thread max(" en; neither is a substring of
		// the cross-window form (次跨窗取最大 / cross-window max).
		runtimeTraceProjMarkMergedMax: {"线程取最大(", "-thread max("},
		// §11-N2 ×N 第四式: the cross-query-window union form's ")union" suffix
		// is the stable language-neutral token (the ") max" precedent) — the
		// plain sum form never carries it.
		runtimeTraceProjMarkMergedUnion: {")union", ")union"},
		// §21 CWD ×N 第五式: the overlapping-query-window MAX form's suffix
		// tokens (zh form word / en ") cross-window max" suffix).
		runtimeTraceProjMarkMergedWindowMax: {"次跨窗取最大(", " cross-window max("},
		// §21.1 CWD-2 ① (huadong E19): the multi-window merged row's fence
		// manifestation is the ABSENCE of the % cell — no positive fence
		// token exists, so no fence probe; direction A (mark ⇔ legend entry)
		// still asserts, and the dedicated CWD-2 pins assert the % absence.
		runtimeTraceProjMarkMergedMultiWindowNoShare: {"", ""},
		runtimeTraceProjMarkOverWindowShare:          {"250%", "250%"},
		runtimeTraceProjMarkWholeWindowIdle:          {"整窗等待", "whole-window wait"},
		runtimeTraceProjMarkInheritedAttribution:     {"承自归因", "inherited attribution"},
		runtimeTraceProjMarkIOCaliberNote:            {"同段IO另有", "same-segment IO also measured"},
		runtimeTraceProjMarkPeriodicSource:           {"周期性信号源", "periodic signal source"},
		runtimeTraceProjMarkAdjacentStanza:           {"◇", "◇"},
		runtimeTraceProjMarkBackgroundStanza:         {"▒", "▒"},
		// PTV5 C00: no fence probe — the fallback caliber word 链上累计 is also
		// the 链上累计Xms data tag's prefix (mark-less), so the token cannot be
		// bidirectional; the mark records at the MainRow Keep-tag append,
		// which the width fit never elides (structural, like StateLabel).
		runtimeTraceProjMarkImpactCaliberFallback: {"", ""},
		// PTV5 Q2: the coverage caliber line renders in the LEAD, not the
		// fence — no fence probe; direction A (mark ⇔ legend entry) still
		// asserts.
		runtimeTraceProjMarkCoverageLine: {"", ""},
		// §29.27② (COV-4): the four-state account renders in the LEAD, not the
		// fence — no fence probe; direction A (mark ⇔ legend entry) asserts.
		runtimeTraceProjMarkFourStateAccount: {"", ""},
		// UXR-1 §29.36.3 (通道4 提及义务): the mention-obligation word head is
		// unique to the ✦ obligation seat.
		runtimeTraceProjMarkSemanticMentionFloor: {"优化点·未入根因排序", "optimization point · "},
		// UXR-1 §29.36②: the off-chain D-state/IO glyph (glyph IS the probe).
		runtimeTraceProjMarkIconDStateOffChain: {"⧗", "⧗"},
		// P9 arm c (§29.42 案1, 2026-07-12): the frame-pacing idle row's type
		// word (zh rides the typelabels table; the EN face keeps the raw
		// token — D2: EN surfaces render tokens verbatim).
		runtimeTraceProjMarkPacingIdle: {"帧间空闲", "pacing_idle"},
		// 复核 P2-1 (2026-07-12): the generic periodic fork's type word.
		runtimeTraceProjMarkPeriodicIdle: {"周期空闲", "periodic_idle"},
		// CAL-1 件⑤/⑥b (2026-07-12): the cadence-idle row glyph (glyph IS
		// the probe, same as the off-chain D-state icon).
		runtimeTraceProjMarkIconPacing: {"∿", "∿"},
		// P2a rider 件3 (§29.58.2 裁定, 2026-07-13): the dedicated binder
		// IPC-wait glyph (glyph IS the probe; ◦/IconNoDominant borrow retired).
		runtimeTraceProjMarkIconBinderWait: {"⋈", "⋈"},
		// P2a rider 件2b (§29.58.1 b, 2026-07-13): the ↳ subordinate-component
		// connector (glyph IS the probe).
		runtimeTraceProjMarkSubordinateComponent: {"↳", "↳"},
		// SELF-SEM (§29.61.1, RANK-U Stage 1, 2026-07-13): the Row2 self-basis
		// qualifier (word IS the probe; zh-en 同词).
		runtimeTraceProjMarkSelfDeterministicBasis: {"自身·确定性优化", "self·deterministic-optimization"},
		// SELF-ALL (§29.61.2/§29.61.2a, 2026-07-13): the wall-clock self-basis
		// qualifier (word IS the probe; zh-en 同词).
		runtimeTraceProjMarkSelfWallClockBasis: {"自身·墙钟席", "self·wall-clock-seat"},
		// SELF-LANE (§29.58.3 处置 a, 2026-07-13): the relocated non-chain self
		// row's qualifier.
		runtimeTraceProjMarkSelfNonChainSeat: {"非链", "non-chain"},
		// SELF-LANE (§29.58.3 处置 b, 2026-07-13): the cross-channel mutual
		// pointer pair (shared word stem 本线程另有/this thread also holds).
		runtimeTraceProjMarkCrossChannelPointer: {"本线程另有", "this thread also holds an"},
		// V2-P0 (2026-07-12): the ⌗ 口径旁栏 disclosure word.
		runtimeTraceProjMarkCaliberSideRow: {"⌗口径旁栏", "⌗ caliber-side"},
		// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
		// fold family word (行1 label / board line / detail mirror).
		runtimeTraceProjMarkMicroAnchorFold: {"项微额锚定席", "micro anchored seats"},
		// RNB-5B 件⑨ (§29.96.2 终判⑨, 2026-07-15): the endpoint-less
		// multi-window chip word.
		runtimeTraceProjMarkMultiWindowNoEndpoints: {"多窗(端点见明细)", "multi-window(endpoints"},
		// RNB-5B 修复轮 D2 (2026-07-15): the ⌗ row-head glyph (glyph+space —
		// the in-row ⌗口径旁栏 word has no trailing space, so the probe is
		// icon-specific).
		runtimeTraceProjMarkIconCaliberSide: {"⌗ ", "⌗ "},
		// CR-2 组② P5: the same-segment mirror tag (equality arm 同段镜像已并入
		// / family arm 同段镜像·与家族行同源 — the probe hits the shared stem).
		runtimeTraceProjMarkSameSegMirror: {"同段镜像", "same-seg mirror"},
		// SMR-1 批 (2026-07-12): the three relation-word families (heads are
		// deliberately unique vs the mirror family's trailing ",不可相加").
		runtimeTraceProjMarkNonAdditivePointer: {"不可相加·", "non-additive · "},
		runtimeTraceProjMarkAccountRelation:    {"两套账目覆盖集不同", "accounting"},
		runtimeTraceProjMarkOccurrenceSeries:   {"不相交(共", "disjoint from ["},
		// RSPA §29.61.10a/b (2026-07-14): the same-source bipartition word
		// family — the 行2 split disclosure head and the relation sentence's
		// additive-identity tail. The zh probes are registered wrap atoms
		// (never bisected); the en probes are single space-free atoms
		// ("full-window" opens both disclosure forms and rides the relation
		// sentence too — legal, since the relation only stamps renders whose
		// remainder row also fires the split mark; "restores" is unique to
		// the relation sentence).
		runtimeTraceProjMarkChainAnchorSplit:    {"同源二分:全窗", "full-window"},
		runtimeTraceProjMarkChainAnchorRelation: {"合计还原全窗账", "restores"},
		// RNB-1 (§29.88 R2/R4, 2026-07-14): the case-A' downgraded relation
		// head and the whole-seat demotion disclosure head — both 行2 line
		// openers, verbatim in the legend entries (bidirectional).
		runtimeTraceProjMarkChainAnchorDivergent:   {"账目关系(锚定权属失合)", "anchored-ownership divergence"},
		runtimeTraceProjMarkChainCredentialDemoted: {"无链上凭证(整席降道)", "whole-seat demotion"},
		// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic
		// seat's 行2 credential sentence head — verbatim in the legend entry.
		runtimeTraceProjMarkHostEdgeAnchored: {"边锚定(宿主→目标)", "edge-anchored (host→target)"},
		// INV-SUPPLY 件①/件③ (§29.61.11, 2026-07-14): the compound type-word
		// suffix (行2 + ◎ 同词, one composer) and the ◎ leverage note head
		// (the ◎ head's 可消除量 shares no substring with 可消除构成).
		runtimeTraceProjMarkSupplyGapDominant: {"供给缺口主导", "supply-gap dominant"},
		runtimeTraceProjMarkElimComposition:   {"可消除构成", "eliminable composition"},
		// CR-2 组③ P7: the typed actual-scope word faces.
		runtimeTraceProjMarkActualBeyondEpisode: {"超出发生段,窗内", "beyond own episode, inside window"},
		runtimeTraceProjMarkActualNoInterval:    {"区间未发布", "interval unpublished"},
		// PTV5 PTS → P2a rider 件1 (§29.55.3 处置更新 + §29.58.2 F2,
		// 2026-07-13): the fold row's dedup name stem — the lane word moved to
		// the edge slot; stanza folds share the name form and the mark.
		runtimeTraceProjMarkOnChainOverflowFold: {"项(折叠)", "more (folded)"},
		// PTV6-C ruling A (#73): the ◇/▒ cross-thread cumulative family word.
		runtimeTraceProjMarkStanzaCrossThreadCum: {"累计(跨线程)", "cross-thread cum"},
		// PTV6-D (b): the generic candidate category word is DELETED from the
		// row face by design (legend carries the class) — no fence probe;
		// direction A (mark ⇔ legend entry) still asserts.
		runtimeTraceProjMarkCandidateShapeClass: {"", ""},
		// §22 B1-b F2 → PTV8-LAD L3 (§24.8 图标化令): the fold force-expanded
		// user-focus transit row's short token ⊚中转/⊚transit (EVOLUTION
		// RECORD: the 18-cell 用户关注线程(中转) long label is retired). The
		// glyph-prefixed form is the probe — the bare 中转/transit word belongs
		// to the IconTransit probe and the ⊚ header renders "⊚ <name>" with a
		// space, never the fused token.
		runtimeTraceProjMarkUserFocusTransit: {runtimeTraceProjRootGlyph + "中转", runtimeTraceProjRootGlyph + "transit"},
		// §22 PTV7-SPN F5 (用户措辞裁定): the trace_gap row's inline disclosure
		// (窗内无调度数据·链止) is the fence token; the legend's 数据盲区 entry
		// renders exactly with it.
		runtimeTraceProjMarkTraceGapBlindSpot: {"窗内无调度数据", "no in-window scheduler data"},
		// §21 LEAD-SEM 前置 L1: the value-less cross-window marker — the ⚠跨窗
		// token never collides with the ⚠实际/⚠actual value form.
		runtimeTraceProjMarkCrossWindowNoActual: {"⚠跨窗", "⚠cross-window"},
		// PTV8-RCR-A (§24.1-§24.3, 2026-07-08). EVOLUTION RECORD: the §21 RNB
		// R1 GatedRunnableSubRow probe (就绪排队积压/ready-queue) and the R2
		// RankFoldNote probe (同段rank行并入) are RETIRED with their marks —
		// the four-line grammar probes below took their seats.
		//
		// §24.3 impact-form glyphs (single-source table; the glyph IS the probe).
		runtimeTraceProjMarkIconLock:      {"⊗", "⊗"},
		runtimeTraceProjMarkIconInversion: {"⇅", "⇅"},
		runtimeTraceProjMarkIconInterrupt: {"↯", "↯"},
		runtimeTraceProjMarkIconBlindSpot: {"◌", "◌"},
		// §24.1 行2 identity line (类别·根因排序#N·置信).
		runtimeTraceProjMarkCauseIdentityRow: {"根因排序#", "root-cause rank #"},
		// §24.1 行3 「=」breakdown line — the "ms = " token is unique to the
		// breakdown form (the degenerate 行2 tail has no "=", the scale note's
		// 满格=窗口 has no "ms " prefix before its "=").
		runtimeTraceProjMarkEffectiveBreakdown: {"ms = ", "ms = "},
		// §24.1补 caliber words (each word carries its own on-demand entry).
		runtimeTraceProjMarkCaliberFull: {"(全额)", "(in full)"},
		// R5 (§29.88.12 单基准, 2026-07-15): ONE conversion caliber seat — the
		// component and value word forms share the basis bytes, so one mark,
		// one probe, one legend entry (词条-图例双向 stays collision-free).
		runtimeTraceProjMarkCaliberGlobalMaxFmax: {"按全域最", "global"},
		runtimeTraceProjMarkCaliberLowerBound:    {"下界", "lower bound"},
		runtimeTraceProjMarkCaliberSingleMax:     {"单次最大(", "single max ("},
		// PTV8-RCR-B (UXA 横扫批 + 复核 6b, 2026-07-08): the fallback scale
		// probes on its own ScaleNote branch token (see BarScale above).
		runtimeTraceProjMarkBarScaleFallback: {"满格=本报告最大", "bar full = report max"},
		// UXA 域A #19: the stanza 折算 discriminator tag ("折算 3.500ms" — the
		// trailing space separates it from the caliber words 折算,按…/(折算)).
		runtimeTraceProjMarkStanzaDiscount: {"折算 ", "discounted "},
		// UXA 域A #31: the 有效归因 word rides many tag forms (Q1 tag / 行2
		// tail / 行3 head / periodic tag) — structural, direction A only.
		runtimeTraceProjMarkEffectiveAttributionTag: {"", ""},
		// PTV8-RCR-C (§24.12 C6): the depthless unattached 三面同词 family —
		// the word rides the edge, the L# chip and the detail 层级 cell.
		runtimeTraceProjMarkChainSeatUnattached: {"父节点未确认", "parent unconfirmed"},
		// PTV8-RCR-C (§24.13 裁定二后半): the multi-board seat window tag
		// (根因排序#1·窗X–Ys); the zh no-space join / en spaced join tokens.
		runtimeTraceProjMarkRankSeatWindow: {"·窗", "· window "},
		// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): the merged-row member-window
		// span word — one emitter feeds the chip qualifier and the ◎ line, and
		// the probe scans the combined tree+overview surface.
		runtimeTraceProjMarkMergedMemberWindowSpan: {"成员跨", "members span "},
		// PTV8-LAD L1 (§24.11 维度A): the run-length cycle fold row's count
		// token — the bare ↺ belongs to the RecursOnChain probe (the cycle row
		// deliberately records BOTH marks: it emits the ↺ token).
		runtimeTraceProjMarkCycleFold: {"循环×", "cycle ×"},
		// RCM-2 D1 (§24.7.1/§24.10/§24.12 维度A ③): the family caliber ladder's
		// three words. The 合计 probe anchors on its own tail 「段,同线程)」 —
		// the count word says 项,同线程 and the max word says 重叠未拆, so the
		// three probes never cross-match.
		runtimeTraceProjMarkFamilyTotal:     {"段,同线程)", "segments, same thread)"},
		runtimeTraceProjMarkFamilyMemberMax: {"重叠未拆", "overlap not deducted"},
		runtimeTraceProjMarkFamilyCountSum:  {"计数合计", "count total ("},
		// CAP (§26 C3, 2026-07-08): the capability disclosure words — the
		// default-table parenthetical and the fail-loud freq_only fallback.
		// The zh probes are wrap atoms; the EN probes are single hyphenated
		// tokens (the space-wrap can split a multi-word phrase mid-line, and
		// the zh freq_only 按纯频率比折算 word also appears in the 下界 LEGEND
		// text — 簇结构不可判/frequency-ratio stay unique to the clause).
		runtimeTraceProjMarkCaliberDefaultCapability:  {"按默认算力比粗算", "capability-ratio"},
		runtimeTraceProjMarkCaliberFreqOnlyCapability: {"簇结构不可判", "frequency-ratio"},
		// CAP-2 (§28.4/§28.5): the two structure-evidence upgrade words. The
		// zh probes sit inside registered wrap atoms (never bisected); the EN
		// probes are single hyphenated tokens (space-wrap safe).
		runtimeTraceProjMarkCaliberComovementTopology: {"共动分簇", "co-moving"},
		runtimeTraceProjMarkCaliberKeyedRailTopology:  {"按簇轨实测", "cluster-rail"},
		// DISP-2 G2 (§27.2 措辞按 kind 分形): the no_eligible_wait blind-spot
		// row's forked inline disclosure — never a substring of the legacy
		// 窗内无调度数据 form, so the two probes stay disjoint.
		runtimeTraceProjMarkTraceGapBelowFloor: {"窗内无≥阈值等待区间", "no in-window wait ≥ floor"},
		// DISP-2 G19 (§27.5): the all-zero fold row's one-line note (the
		// ×N(0.000~0.000ms)取最大 claim is retired on that shape).
		runtimeTraceProjMarkAllZeroFoldNote: {"窗内无有效时长", "no in-window effective duration"},
		// DISP-2 / GAP-A P3-6: the 计数当量 marker rides the count family's
		// roster sub-rows (engine-real roster entries carry it verbatim on
		// both faces — the engine helper has no EN form; G10-class debt noted
		// in §28.7), so the zh token doubles as the EN-face probe.
		runtimeTraceProjMarkFamilyCountEquivalent: {"计数当量", "计数当量"},
		// G12-ENG (§29.1 + 复核 P2-2): the 无时长值 family word — renders
		// exactly when a merged row folds valueless members (mixed form
		// "N项无时长值" / standalone all-zero R2 form "全部无时长值"; the E23
		// fabricated-double witness class). The shared stem is the probe so
		// both forms satisfy the bidirectional contract.
		runtimeTraceProjMarkValuelessFoldMembers: {"无时长值", "without measurable duration"},
		// 审计 #62 ① (§29.25 处置委托 + §29.26 待主会话落账, 2026-07-10): the
		// on-chain semantic dual-caliber word — renders exactly when a
		// partial-overlap on-chain semantic row publishes the intersection
		// under 链上计入 beside the 窗口投影合计 union disclosure.
		runtimeTraceProjMarkFamilyChainIntersection: {"链上计入", "on-chain counted"},
	}
}

// revisit76CAPDemotedReferenceProjection (CAP 复核 F1, 2026-07-08) exercises
// the demoted fold-basis words (按小核满频折算) and their shared legend seat:
// a Dominant-verdict fold whose typed reference class moved off big.
func revisit76CAPDemotedReferenceProjection() types.TraceCausalProjection {
	projection := revisit76CAPCapabilityProjection(runtimeTraceCapabilitySourceDefault)
	projection.OnChainCauses[0].SupplyFoldReferenceClass = "small"
	return projection
}

// revisit76CAPCapabilityProjection (CAP §26 C3, 2026-07-08) exercises the
// capability disclosure words on the Dominant supply-fold verdict: source
// selects the default-table parenthetical or the fail-loud freq_only fallback.
func revisit76CAPCapabilityProjection(source string) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "cap-1",
			Subject: "worker-9", Object: "running", StateKind: "running",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 20.0, CumulativeImpactMS: 20.0,
			SupplyFoldComputed: true, SupplyFoldDeficitMS: 5.0, SupplyFoldIdealMS: 15.0,
			SupplyFoldKnownMS: 20.0, SupplyFoldCapabilitySource: source, Confidence: 0.8,
		}},
	}
}

// revisit76CAP2TopologyProjection (CAP-2 §28.4/§28.5, 2026-07-09) exercises
// the structure-evidence upgrade words on the Dominant supply-fold verdict:
// topo selects the Tier-1 co-movement wording or the Tier-2 keyed-rail
// wording (with the THERM press sentence riding the keyed-rail form).
func revisit76CAP2TopologyProjection(topo string, thermalKHz int) types.TraceCausalProjection {
	projection := revisit76CAPCapabilityProjection(runtimeTraceCapabilitySourceDefault)
	projection.OnChainCauses[0].SupplyFoldTopologySource = topo
	projection.OnChainCauses[0].ThermalCapKHz = thermalKHz
	return projection
}

// revisit76RCM2FamilyProjection (RCM-2, §24.7.1/§24.10, 2026-07-08) exercises
// the family-merge display half's three caliber forms on one render: the
// cmp_78 E27-E42 semantic family (sum_disjoint, ×14 合计 + background board
// seat), the opendir_78 E5/E6 generic inode family on its overlap arm
// (max_overlap_fallback + raw-Σ disclosure) and a count_sum advisory family.
func revisit76RCM2FamilyProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rcm2-max",
				Subject: "RxComputationT-16816", Object: "block_io_by_inode",
				TypeToken: "block_io_by_inode", ChainRelevance: "on_chain",
				ChainDepth: 1, Rank: 3,
				ImpactMS: 1.136, CumulativeImpactMS: 1.136, EffectiveImpactMS: 1.136,
				FamilyMemberCount: 2, FamilyMemberMaxMS: 1.136, FamilyMemberMinMS: 0.462,
				FamilyMemberSumMS: 1.598, FamilyFoldCaliber: "max_overlap_fallback",
				FamilyMemberRoster: []string{"inode=286395 dev=254:2 1.136ms", "inode=300123 dev=254:2 0.462ms"},
				Dev:                "254:2", Confidence: 0.8},
			// DISP-2 (2026-07-09): roster entries carry the engine-real
			// 计数当量X(非墙钟) marker (rootCauseCountEquivalentValue renders
			// every count-family roster value through it — fixture 取引擎
			// 实铸形, §28.7 新纪律; §29.55③ 两形一裁 ms 后缀退役), so the
			// FamilyCountEquivalent probe is bidirectional on this shape.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rcm2-count",
				Subject: "worker-9", Object: "state_churn", TypeToken: "state_churn",
				ChainRelevance: "on_chain", Rank: 4,
				ImpactMS: 5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
				FamilyMemberCount: 3, FamilyFoldCaliber: "count_sum",
				FamilyMemberRoster: []string{"churn a 计数当量2.000(非墙钟)", "churn b 计数当量2.000(非墙钟)", "churn c 计数当量1.000(非墙钟)"},
				Confidence:         0.8},
		},
		SemanticSpans: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "rcm2-sem",
				Subject: "worker-9", Predicate: "trace_semantic_span",
				Object: "class_verification", SemanticClass: "class_verification",
				SpanName: "VerifyClass com.demo.Big",
				ImpactMS: 7.124, EffectiveImpactMS: 7.124, BackgroundRank: 1,
				FamilyMemberCount: 14, FamilyMemberMaxMS: 2.424, FamilyMemberMinMS: 0.040,
				FamilyFoldCaliber: "sum_disjoint",
				FamilyMemberRoster: []string{
					"VerifyClass com.demo.Big 2.424ms",
					"VerifyClass com.demo.Mid 1.900ms",
					"VerifyClass com.demo.Small 0.800ms",
					"VerifyClass com.demo.Tiny 0.500ms",
				},
				Confidence: 0.7},
		},
	}
}

// revisit76UXAWindowlessProjection (PTV8-RCR-B, UXA 域A #13) is the no-window
// shape with a plain bar row: the bar draws against the report-max fallback
// scale, so the BarScaleFallback legend entry renders (and the windowed
// BarScale entry does not).
func revisit76UXAWindowlessProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath: []string{"worker-9", "app-100"},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
			Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
			ImpactMS: 30, Confidence: 0.8,
		}},
	}
}

// revisit76UXAStanzaDiscountProjection (PTV8-RCR-B, UXA 域A #19) is the ◇
// stanza row whose cum and effective DIFFER: the 折算 discriminator tag
// renders with its own on-demand legend entry.
func revisit76UXAStanzaDiscountProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 30, Confidence: 0.8},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "adj-5",
				Object: "running_burst", StateKind: "running", ChainRelevance: "adjacent",
				ImpactMS: 10, CumulativeImpactMS: 18, EffectiveImpactMS: 7, Confidence: 0.8},
		},
	}
}

// revisit76PTV5FoldCaliberProjection (PTV5 #68) exercises the C00 fallback
// caliber word (a data row with NO window projection — cumulative-sourced
// main ms) and the PTS on-chain overflow fold row (zero-silent-drop counted
// fold).
func revisit76PTV5FoldCaliberProjection() types.TraceCausalProjection {
	fallback := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ptv5-fallback",
		Subject: "chained-3", Object: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "on_chain", CumulativeImpactMS: 25.0, Confidence: 0.8,
	}
	fold := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ptv5-fold-1",
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		ImpactMS: 12.0, CumulativeImpactMS: 12.0,
		MergedCount: 3, MergedMinMS: 2.0, MergedMaxMS: 12.0,
		MergedSubjects: []string{"of-1", "of-2"},
		Confidence:     0.8,
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"chained-3", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		OnChainCauses: []types.TraceCausalProjectionNode{fallback, fold},
	}
}

// revisit76PTV4BadgeMergeProjection (PTV4) exercises the T4/T6/T9 marks the
// older shapes never emitted: ❶❷❸ badges (typed Rank + effective attribution),
// the 链上L# chip, the ×N 三式 (sum / dedup / cross-thread max), the >100%
// over-window share, the whole-window idle row and the inherited-attribution
// note.
func revisit76PTV4BadgeMergeProjection() types.TraceCausalProjection {
	chain := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ptv4-rank1",
		Subject: "worker-9", Object: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "on_chain", ChainDepth: 1, Rank: 1,
		ImpactMS: 60.0, CumulativeImpactMS: 60.0, EffectiveImpactMS: 60.0,
		MergedCount: 3, MergedMinMS: 10.0, MergedMaxMS: 30.0,
		Confidence: 0.8,
	}
	dedup := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ptv4-rank2",
		Subject: "worker-9", Object: "io_latency",
		ChainRelevance: "on_chain", Rank: 2,
		ImpactMS: 35.0, CumulativeImpactMS: 3.0, EffectiveImpactMS: 40.0,
		DuplicatePublications: 2,
		Confidence:            0.8,
	}
	idle := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ptv4-idle",
		Subject: "idler-4", Object: "sleep_wait", StateKind: "s_sleep",
		ChainRelevance: "background", ImpactMS: 99.8, Confidence: 0.8,
	}
	over := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ptv4-over",
		Subject: "irq/151-dpu", Object: "irq_burst",
		ChainRelevance: "background", ImpactMS: 250.0, Confidence: 0.8,
	}
	maxFold := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ptv4-maxfold",
		Object: "unknown-thread", ChainRelevance: "background",
		ImpactMS: 42.0, CumulativeImpactMS: 42.0,
		MergedCount: 4, MergedMinMS: 12.0, MergedMaxMS: 42.0,
		MergedSubjects: []string{"bd-1", "bd-2"},
		Confidence:     0.8,
	}
	return types.TraceCausalProjection{
		WakeupPath:        []string{"worker-9", "app-100"},
		WindowStartTs:     100.0,
		WindowEndTs:       100.1,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{chain, dedup},
		BackgroundCauses:  []types.TraceCausalProjectionNode{idle, over, maxFold},
	}
}

// revisit76PeriodicProjection is the VS-1 berlin shape: a periodic signal
// source (VSyncGenerator ≈8.3ms cadence) whose in-period sleep is normal
// cadence — the row carries the cadence Keep tag and its legend entry.
func revisit76PeriodicProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"VSyncGenerator-610", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.05,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "VSyncGenerator-610",
			Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
			ImpactMS: 36.256, CumulativeImpactMS: 36.361, EffectiveImpactMS: 0.176,
			PeriodicSource: true, DetectedPeriodMS: 8.302, PeriodicLatenessMS: 0.071,
			Confidence: 0.8,
		}},
	}
}

// revisit76RichChainProjection is the 7.0-ish rich shape: an 11-hop path (long
// trunk → omitted fold) with a recurring subject (↺), runnable/running/D-state
// icons, a cross-window row (⚠跨窗), a semantic span (✦) and a sleeping 🎯
// self row.
func revisit76RichChainProjection() types.TraceCausalProjection {
	node := func(subject, state string, impact float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, Subject: subject,
			Object: "runnable_wait", StateKind: state, ChainRelevance: "on_chain",
			ImpactMS: impact, Confidence: 0.8,
		}
	}
	cross := node("p2-2", "runnable", 30)
	cross.ActualImpactMS = 80 // > baseline*1.001 → ⚠跨窗
	return types.TraceCausalProjection{
		// path renders trunk [p1-1 p2-2 p3-3 p1-1 p5-5 … p10-10]: >8 nodes →
		// omitted middle; the second p1-1 (visible head) recurs on chain.
		WakeupPath: []string{"p10-10", "p9-9", "p8-8", "p7-7", "p6-6", "p5-5",
			"p1-1", "p3-3", "p2-2", "p1-1", "target-7"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			node("target-7", "s_sleep", 50),
			node("p1-1", "running", 40),
			cross,
			node("p3-3", "d_state", 20),
		},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, Subject: "p1-1",
			SpanName: "bindApplication", ImpactMS: 5, Confidence: 0.8,
		}},
	}
}

// revisit76P9PacingIdleProjection (P9 arm c, §29.42 案1, 2026-07-12) is the
// frame-pacing idle shape: a written-off binder candidate segment re-minted
// as a pacing_idle context row (tier context_only, no board seat) beside an
// ordinary on-chain cause — the donghu 15.758ms/16.738ms witness geometry.
func revisit76P9PacingIdleProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.894,
		WindowEndTs:   13763.010,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "app-9511",
			Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
			ImpactMS: 30, Confidence: 0.8,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: ".ugc.aweme.lite-17267",
			Object: "pacing_idle", StateKind: "s_sleep", ChainRelevance: "adjacent",
			Tier:     types.TraceCausalTierContextOnly,
			ImpactMS: 15.758, Confidence: 0.85,
		}},
	}
}

// revisit76P21PeriodicIdleProjection (复核 P2-1, 2026-07-12) is the generic
// periodic-idle fork: a measured periodic (non-frame) waker's idle segment —
// same context seat, its own 周期空闲 word + legend entry.
func revisit76P21PeriodicIdleProjection() types.TraceCausalProjection {
	proj := revisit76P9PacingIdleProjection()
	proj.AdjacentCauses[0].Object = "periodic_idle"
	return proj
}

// revisit76FlatUndrillableProjection is the flat ⛔ shape: no ≥2-node wakeup
// path, one undrillable missing_wakeup sleep row.
func revisit76FlatUndrillableProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "app-1",
			Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
			ImpactMS: 9, UndrillableReason: "missing_wakeup", Confidence: 0.8,
		}},
	}
}

// revisit76BerlinLayersProjection is the berlin-ish layered shape: a same-
// subject cause decomposition (├─成因─) plus adjacent (◇) and background (▒)
// stanzas.
func revisit76BerlinLayersProjection() types.TraceCausalProjection {
	node := func(subject, object, state string, impact float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, Subject: subject,
			Object: object, StateKind: state, ChainRelevance: "on_chain",
			ImpactMS: impact, Confidence: 0.8,
		}
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			node("worker-9", "running_burst", "running", 30),
			node("worker-9", "monitor_contention", "", 20), // same-subject → cause row
		},
		AdjacentCauses:   []types.TraceCausalProjectionNode{node("adj-5", "running_burst", "running", 10)},
		BackgroundCauses: []types.TraceCausalProjectionNode{node("bg-6", "runnable_wait", "runnable", 12)},
	}
}

// revisit76PTV6DepthlessProjection is the PTV6 #1b shape: a resolved trunk
// plus one on-chain data row whose subject is off-trunk with no resolvable
// depth — the remaining-on-chain lane renders it with the dedicated
// 链上·深度未解析 edge (never the wake edge).
func revisit76PTV6DepthlessProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 30, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, Subject: "detached-7",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ImpactMS: 12, Confidence: 0.8},
		},
	}
}

// revisit76PTV6CStanzaCrossCumProjection (PTV6-C ruling A, #73) is the ◇
// stanza row carrying chain-cum + effective values: both must render the
// 累计(跨线程) family word (equal values dedupe to ONE tag), never the
// chain-universe attribution vocabulary.
func revisit76PTV6CStanzaCrossCumProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 30, Confidence: 0.8},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "adj-5",
				Object: "running_burst", StateKind: "running", ChainRelevance: "adjacent",
				ImpactMS: 10, CumulativeImpactMS: 18, EffectiveImpactMS: 18, Confidence: 0.8},
		},
	}
}

// revisit76SelfSemBasisProjection (SELF-SEM §29.61.1, RANK-U Stage 1,
// 2026-07-13) is the endless_loop/donghu 970481 witness geometry: the analysis
// target's own VerifyClass family (window-union 13.006ms) admitted to the
// on-chain channel on the typed self basis — seat #2 behind the 26.392ms
// runnable dependency — whose Row2 wears the 自身·确定性优化 qualifier.
func revisit76SelfSemBasisProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs: 17729.471126,
		WindowEndTs:   17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "shadowhook-task-64305",
			Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
			ImpactMS: 26.392, CumulativeImpactMS: 26.392, EffectiveImpactMS: 26.392,
			Rank: 1, Tier: "primary", Confidence: 0.8, EvidenceID: "selfsem-dep",
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "selfsem-fam",
			Subject: "ease.cloudmusic-63993", Predicate: "trace_semantic_span",
			Object: "class_verification", SemanticClass: "class_verification",
			SpanName:       "VerifyClass com.netease.cloudmusic.Foo",
			ChainRelevance: "on_chain", Causality: "self_deterministic",
			OnChainBasis: "self_deterministic_span",
			ImpactMS:     13.006, CumulativeImpactMS: 13.006, EffectiveImpactMS: 13.006,
			Rank: 2, Tier: "secondary",
			FamilyMemberCount: 14, FamilyMemberMaxMS: 3.1, FamilyMemberMinMS: 0.2,
			FamilyFoldCaliber: "interval_union", FamilyMemberSumMS: 13.247,
			FamilyMemberRoster: []string{"VerifyClass com.netease.cloudmusic.Foo 3.100ms"},
			LineStart:          3, LineEnd: 7, Confidence: 0.82,
		}},
	}
}

// revisit76SelfAllWallClockProjection (SELF-ALL §29.61.2/§29.61.2a +
// SELF-LANE §29.58.3, 2026-07-13) is the donghu 133136 witness geometry: the
// target's own io_latency wall-clock family promoted to the on-chain channel
// on the typed self basis (Row2 自身·墙钟席 + 根因排序#6), its non-wall-clock
// ⌗ page-cache residual relocating 非链 into the self stanza, and a NON-target
// thread seated on both channels wearing the cross-channel mutual pointers.
func revisit76SelfAllWallClockProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-io",
				Subject: ".ugc.aweme.lite-17267", Predicate: "root_cause_tertiary",
				Object: "io_latency", TypeToken: "io_latency",
				ChainRelevance: "on_chain", Causality: "self_wall_clock",
				OnChainBasis: "self_wall_clock_interval",
				ImpactMS:     3.264, CumulativeImpactMS: 3.264, EffectiveImpactMS: 3.264,
				Rank: 6, Tier: "tertiary",
				FamilyMemberCount: 5, FamilyMemberMaxMS: 1.248, FamilyMemberMinMS: 0.865,
				FamilyFoldCaliber: "interval_union", FamilyMemberSumMS: 5.111,
				FamilyMemberRoster: []string{"dev=12,80 op=RCVHS sector=126160840 1.248ms"},
				LineStart:          20486, LineEnd: 20517, Confidence: 0.86},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-peer-chain",
				Subject: "keva-3-17439", Object: "running_burst", StateKind: "running",
				ChainRelevance: "on_chain", ChainDepth: 1, ImpactMS: 2.579, CumulativeImpactMS: 2.579,
				EffectiveImpactMS: 2.579, Rank: 3, Tier: "tertiary", Confidence: 0.8,
				LineStart: 100, LineEnd: 120},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-pagecache",
				Subject: ".ugc.aweme.lite-17267", Predicate: "root_cause_caliber_side",
				Object: "page_cache_churn", TypeToken: "page_cache_churn",
				ChainRelevance: "adjacent", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
				EffectiveImpactMS: 81.616, Tier: "caliber_side", Confidence: 0.72,
				LineStart: 200, LineEnd: 260},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-peer-adj",
				Subject: "keva-3-17439", Object: "io_latency", TypeToken: "io_latency",
				ChainRelevance: "adjacent", ImpactMS: 0.871, CumulativeImpactMS: 0.871,
				EffectiveImpactMS: 0.871, Confidence: 0.8, LineStart: 300, LineEnd: 320},
		},
	}
}

// revisit76AssertLegendBidirectional renders one shape and asserts the NEW-7
// two-way contract: (a) typed marks ⇔ rendered legend entries; (b) for every
// probed mark, its fence token appears IFF its legend entry renders.
func revisit76AssertLegendBidirectional(t *testing.T, name string, projection types.TraceCausalProjection, zh bool) *runtimeTraceProjMarkSet {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	fence := runtimeTraceProjTreeFence(model, zh)
	// ELIM-1 (RANK-U Stage 2): mirror the production render order — the ◎
	// overview fence renders after the tree and BEFORE the lead text, so its
	// legend mark participates in the bidirectional sweep. Probes scan the
	// combined fence surface.
	if elim := runtimeTraceProjElimOverviewFence(projection, model, zh); elim != "" {
		fence += "\n" + elim
	}
	lang := "zh"
	if !zh {
		lang = "en"
	}
	lead := runtimeTraceProjLeadText(projection, model, lang, zh)
	probes := revisit76LegendProbes()
	if len(probes) != int(runtimeTraceProjMarkCount) {
		t.Fatalf("probe table must cover every mark: %d/%d", len(probes), int(runtimeTraceProjMarkCount))
	}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		text, probe := entry.ZH, probes[entry.Mark].zh
		if !zh {
			text, probe = entry.EN, probes[entry.Mark].en
		}
		rendered := strings.Contains(lead, text)
		if model.Marks.has(entry.Mark) != rendered {
			t.Fatalf("%s: mark %d emitted=%v but legend rendered=%v\nlead:\n%s",
				name, entry.Mark, model.Marks.has(entry.Mark), rendered, lead)
		}
		if probe == "" {
			continue
		}
		emitted := strings.Contains(fence, probe)
		if entry.Mark == runtimeTraceProjMarkBadge {
			// §29.27.1: the badge family is ❶..❺ (glyph follows the seat), so
			// the fence may emit ANY family member — probe the whole family.
			emitted = false
			for r := 1; r <= runtimeTraceProjBadgeTopN; r++ {
				if strings.Contains(fence, runtimeTraceProjBadgeGlyph(r)) {
					emitted = true
					break
				}
			}
		}
		if entry.Mark == runtimeTraceProjMarkCauseIdentityRow {
			// UXR-1 (§29.36.2): the identity-line seat chip is a CHANNEL
			// family (根因排序#/邻近影响#) — the fence may emit either member,
			// and an unseated identity line ends on its 置信 tier instead.
			second := "邻近影响#"
			tier := "·置信"
			if !zh {
				second = "adjacent-impact #"
				tier = "confidence "
			}
			emitted = emitted || strings.Contains(fence, second) || strings.Contains(fence, tier)
		}
		if emitted != rendered {
			t.Fatalf("%s: bidirectional violation for mark %d (probe %q): fence emits=%v, legend explains=%v\nfence:\n%s\nlead:\n%s",
				name, entry.Mark, probe, emitted, rendered, fence, lead)
		}
	}
	return model.Marks
}

// TestSelfSemCrownedFormSelfConsistent (件5, 修复轮 复核 F4 2026-07-13; 设计
// 裁定④ 默认形固化): a self-basis row whose eff TOPS the board is crownable —
// the fence renders ❶ + 根因排序#1 + the 自身·确定性优化 qualifier on ONE ✦
// row (词面自洽: crown, seat and self basis co-render without a wake-edge
// claim), and the bundle_top_cause banner leads with the same row carrying
// the honest on-chain identity.
func TestSelfSemCrownedFormSelfConsistent(t *testing.T) {
	proj := revisit76SelfSemBasisProjection()
	// Crown shape: the self family out-ranks the runnable dependency.
	proj.OnChainCauses[0].Rank = 2
	proj.OnChainCauses[0].Tier = "secondary"
	proj.OnChainCauses[0].ImpactMS = 6.392
	proj.OnChainCauses[0].CumulativeImpactMS = 6.392
	proj.OnChainCauses[0].EffectiveImpactMS = 6.392
	proj.SemanticSpans[0].Rank = 1
	proj.SemanticSpans[0].Tier = "primary"
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var crowned string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "❶") && strings.Contains(line, "✦") {
			crowned = line
			break
		}
	}
	if crowned == "" {
		t.Fatalf("the board-topping self row must wear ❶ on its ✦ seat:\n%s", fence)
	}
	if !strings.Contains(fence, "自身·确定性优化·根因排序#1") {
		t.Fatalf("crown 词面自洽: qualifier and #1 seat must co-render on 行2:\n%s", fence)
	}
	if strings.Contains(crowned, "唤醒─") {
		t.Fatalf("the crowned self row must not claim a wake edge: %q", crowned)
	}
	// bundle_top_cause banner: the positional crown speaks the self row's
	// honest identity (on-chain relevance + the no-wakeup-claim summary).
	var b strings.Builder
	writeTraceFrameRootCauseBundleSummary(&b, &tracequery.FrameRootCauseBundle{
		Target: tracequery.ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
		Window: tracequery.TimeWindow{StartTs: 17729.471126, EndTs: 17729.622508},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 17729.471126, EndTs: 17729.622508},
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "class_verification",
				Thread:   tracequery.ThreadRef{Comm: "ease.cloudmusic", PID: 63993},
				ImpactMs: 13.006, CumulativeImpactMs: 13.006, EffectiveImpactMs: 13.006,
				Causality: "self_deterministic", ChainRelevance: "on_chain",
				OnChainBasis:  tracequery.RootCauseOnChainBasisSelfDeterministicSpan,
				SemanticClass: "class_verification", MemberCount: 14,
				Summary: "class verification family n=14 span(s) on the analysis target's own thread totalled 13.006ms window projection (deterministic self work counted on-chain without any wakeup-edge claim)",
			}},
		},
	})
	banner := b.String()
	if !strings.Contains(banner, "bundle_top_cause type=class_verification") ||
		!strings.Contains(banner, "chain_relevance=on_chain") {
		t.Fatalf("bundle_top_cause must lead with the crowned self row's on-chain identity:\n%s", banner)
	}
	if !strings.Contains(banner, "without any wakeup-edge claim") {
		t.Fatalf("the crown banner must keep the honest no-wakeup-claim wording:\n%s", banner)
	}
}

// TestSelfSemFenceRowFormNoWakeEdge (SELF-SEM §29.61.1 显示负向 pin): the
// self-basis semantic row renders through the ✦ 语义 lane with the Row2
// qualifier and its chain-channel seat — and NEVER behind a fabricated wake
// edge (不铸唤醒边: the only 唤醒─ edges in the fence belong to the real
// trunk, none may lead into the ✦ row's line).
func TestSelfSemFenceRowFormNoWakeEdge(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(revisit76SelfSemBasisProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "自身·确定性优化") {
		t.Fatalf("Row2 must wear the self qualifier:\n%s", fence)
	}
	if !strings.Contains(fence, "根因排序#2") {
		t.Fatalf("the self row must keep its chain-channel seat #2:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "✦") && strings.Contains(line, "唤醒─") {
			t.Fatalf("a semantic ✦ row must never render behind a wake edge: %q", line)
		}
	}
	semanticEdge := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "语义─") && strings.Contains(line, "✦") {
			semanticEdge = true
			break
		}
	}
	if !semanticEdge {
		t.Fatalf("the self row must render through the ✦ 语义 lane:\n%s", fence)
	}
}

func TestTraceProjectionLegendBidirectionalAcrossRepresentativeShapes(t *testing.T) {
	fixtures := []struct {
		name string
		proj types.TraceCausalProjection
	}{
		{"revisit_6_0_io_fold", revisit76IOProjection()},
		// F2: the residual shape exercises the depthless own-process IO edge
		// (├─自身─) — its legend entry renders exactly when the edge is drawn.
		{"revisit_6_0_residual_own_edge", revisit76ResidualProjection()},
		{"revisit_7_0_rich_chain", revisit76RichChainProjection()},
		{"flat_undrillable", revisit76FlatUndrillableProjection()},
		{"berlin_cause_adjacent_background", revisit76BerlinLayersProjection()},
		// VS-1: the periodic-source cadence tag and its legend entry.
		{"berlin_periodic_source", revisit76PeriodicProjection()},
		// PTV4: badges, 链上L# chip, ×N 三式, over-window share, whole-window
		// idle, inherited attribution.
		{"ptv4_badges_merges", revisit76PTV4BadgeMergeProjection()},
		// PTV5 (#68): C00 fallback caliber word + PTS on-chain overflow fold.
		{"ptv5_fold_caliber", revisit76PTV5FoldCaliberProjection()},
		// PTV6 #1b: the depthless remaining-on-chain lane's dedicated edge.
		{"ptv6_depthless_chain_unresolved", revisit76PTV6DepthlessProjection()},
		// PTV6-C ruling A: the ◇/▒ 累计(跨线程) family word + its legend entry.
		{"ptv6c_stanza_cross_cum", revisit76PTV6CStanzaCrossCumProjection()},
		// §11-N2: the cross-query-window union caliber (×N 第四式) and its
		// legend entry (fixture home: answer_document_projection_n2_union_test.go).
		{"n2_cross_window_union", n2UnionProjection()},
		// §21 CWD: the overlapping-query-window MAX caliber (×N 第五式) and its
		// legend entry (fixture home: answer_document_projection_cwd_test.go).
		{"cwd_cross_window_max", cwdCrossWindowMaxProjection()},
		// §21.1 CWD-2 ① (huadong E19): the multi-window merged SUM row whose
		// anchor-window share is suppressed (fixture home:
		// answer_document_projection_cwd2_test.go).
		{"cwd2_multi_window_sum", cwd2MultiWindowSumProjection()},
		// §22 B1-b F2: a typed user entity inside the folded trunk middle is
		// force-expanded as a named transit row (fixture home:
		// answer_document_mutation_runtime_tree_anchor_b1b_test.go).
		{"b1b_user_focus_fold", b1bUserFocusFoldProjection()},
		// §22 PTV7-SPN F5: the trace_gap 数据盲区 disclosure row (fixture home:
		// answer_document_mutation_runtime_ptv7_spn_test.go).
		{"ptv7_spn_trace_gap", ptv7SpnTraceGapProjection()},
		// DISP-2 G2 (§27.2 措辞按 kind 分形): the no_eligible_wait blind-spot
		// row's forked disclosure + legend entry (same fixture home).
		{"disp2_trace_gap_below_floor", ptv7SpnTraceGapBelowFloorProjection()},
		// P9 arm c (§29.42 案1, 2026-07-12): the frame-pacing idle row's type
		// word + teaching legend entry.
		{"p9_pacing_idle", revisit76P9PacingIdleProjection()},
		// 复核 P2-1 (2026-07-12): the generic periodic fork's word + entry.
		{"p2_1_periodic_idle", revisit76P21PeriodicIdleProjection()},
		// DISP-2 G19 (§27.5): the all-zero on-chain fold's one-line note
		// (same fixture home).
		{"disp2_all_zero_fold", disp2AllZeroFoldProjection()},
		// P2a rider 件2/件3 (§29.58.1/§29.58.2, 复核 P2-1 2026-07-13): the
		// self-stanza carve shape lights the ⋈ binder-wait glyph and the ↳
		// subordinate-component connector — bidirectional legend parity for
		// both new marks (fixture home:
		// answer_document_projection_p2a_rider_test.go).
		{"p2a_self_carve", p2aSelfCarveProjection()},
		// §21/§22 RNB R1+R2: the same-segment two-lane fold note + the gated
		// runnable component sub-row (fixture home:
		// answer_document_projection_rnb_leadsem_test.go).
		{"rnb_twin_fold", rnbTwinFoldProjection()},
		// §21 LEAD-SEM 前置 L1: the value-less ⚠跨窗 marker on an out-of-window
		// row whose actual total was never captured (same fixture home).
		{"leadsem_cross_window_no_actual", leadSemCrossWindowNoActualProjection()},
		// DCS E5 (§23.1 H2): a semantic row measured in a DIFFERENT typed
		// query window re-bases its % on that source window (fixture home:
		// answer_document_mutation_runtime_dcs_test.go).
		{"dcs_semantic_source_window", dcsSemanticSourceWindowProjection()},
		// PTV8-RCR-A (§24.1-§24.3): the opendir E4-E8 node group — ⊗/⇅
		// glyphs, the 行2/行3 grammar marks and the 全额/按下游消费核/
		// 单次最大 caliber entries (fixture home:
		// answer_document_projection_rcr_test.go).
		{"rcr_opendir_node_group", rcrOpendirProjection()},
		// PTV8-RCR-A §24.1补: the Dominant supply-fold verdict carries the
		// 按大核满频/下界 caliber entries (same fixture home).
		{"rcr_supply_fold_dominant", rcrSupplyFoldDominantProjection()},
		// PTV8-RCR-B (UXA 域A #13/#19): the no-window fallback bar scale and
		// the stanza 折算 discriminator entries.
		{"uxa_windowless_fallback_scale", revisit76UXAWindowlessProjection()},
		{"uxa_stanza_discount", revisit76UXAStanzaDiscountProjection()},
		// PTV8-RCR-C (§24.12 C6 + §24.13 裁定二后半): the depthless unattached
		// 三面同词 word and the multi-board seat window tag (fixture home:
		// answer_document_projection_rcrc_test.go).
		{"rcrc_multi_board_unattached", rcrcMultiBoardUnattachedProjection()},
		// PTV8-LAD L1 (§24.11 维度A): the run-length cycle fold row + the ⊚中转
		// short transit token on the huadong_78 ladder shape (fixture home:
		// answer_document_projection_lad_test.go).
		{"lad_cycle_fold_ladder", ladHuadongLadderProjection()},
		// RCM-2 (§24.7.1/§24.10): the family-merge caliber ladder's three
		// display words (合计/成员最大/计数合计) + their legend entries.
		{"rcm2_family_forms", revisit76RCM2FamilyProjection()},
		// CAP (§26 C3): the capability disclosure words on the Dominant
		// supply-fold verdict — default table vs the fail-loud freq_only
		// fallback (fixture above).
		{"cap_capability_default", revisit76CAPCapabilityProjection(runtimeTraceCapabilitySourceDefault)},
		{"cap_capability_freq_only", revisit76CAPCapabilityProjection(runtimeTraceCapabilitySourceFreqOnly)},
		// CAP-2 (§28.4/§28.5): the Tier-1/Tier-2 structure-evidence upgrade
		// words + their legend entries (THERM sentence rides the rail form).
		{"cap2_topology_comovement", revisit76CAP2TopologyProjection(runtimeTraceCapabilityTopologyComovement, 0)},
		{"cap2_topology_keyed_rail", revisit76CAP2TopologyProjection(runtimeTraceCapabilityTopologyKeyedRail, 1850000)},
		// CAP 复核 F1: the demoted-basis word + its legend seat.
		{"cap_reference_demoted", revisit76CAPDemotedReferenceProjection()},
		// G12-ENG (§29.1): the mixed valued+valueless fold (huadong_79 E23
		// shape) + its 无时长值 legend entry (fixture home:
		// answer_document_projection_valueless_fold_g12_test.go).
		{"g12_mixed_valueless_fold", g12MixedFoldProjection()},
		// 审计 #62 ① (§29.25/§29.26, 2026-07-10): the partial-overlap on-chain
		// semantic family's dual-caliber 行3 (链上计入 + 窗口投影合计) —
		// fixture home: answer_document_projection_dispw1_test.go.
		{"dispw1_semantic_chain_intersection", dispW1SemanticIntersectionProjection()},
		// §29.27② (COV-4, 2026-07-11): the four-state coverage account + its
		// 全窗四态 legend entry on the balanced running-dominant shape
		// (fixture home: answer_document_projection_cov4_test.go).
		{"cov4_four_state_account", cov4RunningDominantProjection()},
		// UXR-1 §29.36.3 (通道4 提及义务): an on-chain semantic row without a
		// channel-1 seat wears the mention-obligation word + legend entry.
		{"uxr1_mention_floor", revisit76UXR1MentionFloorProjection()},
		// V2-P0 (design §6.1 新裁定 A, 2026-07-12): the ⌗ 口径旁栏 rows
		// (count + composite-score) + their legend entry (fixture home:
		// answer_document_projection_v2p0_test.go).
		{"v2p0_caliber_side", types.TraceCausalProjectionFromObservationRecords(v2p0CaliberRecords())},
		// CR-2 组② P5 (2026-07-12; equality arm retired to the engine one-seat
		// mint in v5 P1 件① — the mark's remaining producers are the FAMILY
		// arm and the member arm): the same-segment mirror tag + its legend
		// entry (fixture home: answer_document_projection_cr2_p5_test.go).
		{"cr2_p5_same_seg_mirror", cr2P5CensusMirrorProjection()},
		// CR-2 组③ P7 (2026-07-12): the typed actual-scope word faces —
		// in-window overshoot (episode) + interval-less disclosure (fixture
		// home: answer_document_projection_cr2_p7_test.go).
		{"cr2_p7_actual_episode", cr2P7Projection(cr2P7Node(16.433, 15.565, 13762.991547, 13763.008274))},
		{"cr2_p7_actual_no_interval", cr2P7Projection(cr2P7Node(16.433, 15.565, 0, 0))},
		// SMR-1 批 (2026-07-12): the three relation-word families + their
		// legend entries (fixture homes:
		// answer_document_projection_smr1_relations_test.go).
		{"smr1_a1_non_additive_pointer", smr1A1SelfBinderProjection()},
		{"smr1_c1_account_relation", smr1C1FamilyChainProjection()},
		{"smr1_b1_occurrence_series", smr1B1OccurrenceProjection()},
		// SELF-SEM (§29.61.1, RANK-U Stage 1, 2026-07-13): the self-basis
		// on-chain semantic family — Row2 自身·确定性优化 qualifier + its
		// legend entry (fixture home: answer_document_projection_selfsem_test.go).
		{"selfsem_basis_qualifier", revisit76SelfSemBasisProjection()},
		// SELF-ALL (§29.61.2/§29.61.2a) + SELF-LANE (§29.58.3, 2026-07-13):
		// the promoted wall-clock self seat (自身·墙钟席 qualifier), the
		// relocated 非链 residual and the cross-channel mutual pointers
		// (fixture home: answer_document_projection_selfall_test.go).
		{"selfall_wall_clock_seat", revisit76SelfAllWallClockProjection()},
		// RSPA §29.61.10a/b (2026-07-14): the same-source bipartition — the
		// 行2 同源二分 disclosure on both halves and the 合计还原全窗账
		// relation sentence + their legend entries (fixture home:
		// answer_document_projection_rspa_test.go).
		{"rspa_same_source_split", rspaSameSourceSplitProjection()},
		// INV-SUPPLY 件①/件③ (§29.61.11/.11a, 2026-07-14): the supply-gap-
		// dominant inversion seat — 行2/◎ compound word + the ◎ leverage note
		// + their legend entries (fixture home:
		// answer_document_projection_elim_test.go, 090607 witness ❶ shape).
		{"inv_supply_compound_seat", elimInvSupplyCompoundProjection()},
		// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): the multi-window merged ◇
		// seat — the 窗X~Ys chip's 「(供席成员窗,成员跨K窗)」 qualifier + its
		// legend entry (fixture home:
		// answer_document_projection_case3d4_test.go, huadong_792 E22 shape
		// re-valued onto the canonical MergedSum probe values).
		{"case3d4_member_window_span", case3d4MemberWindowSpanProjection()},
		// RNB-1 (§29.88 R2/R4, 2026-07-14): the case-A' ownership-divergent
		// remainder + the R4 whole-seat lane-demoted satellite (fixture home:
		// answer_document_projection_rspa_test.go, keva-1/logd.writer shapes).
		{"rnb_divergent_demoted", rspaRNBDivergentDemotedProjection()},
		// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic
		// seat — 行2 边锚定(宿主→目标) credential sentence + its legend entry
		// (fixture home: answer_document_projection_r3_edge_anchor_test.go,
		// SCAN-3 positive sentinel shape).
		{"r3_host_edge_anchored", r3HostEdgeAnchoredProjection()},
		// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
		// fold row (其余N项微额锚定席) + its legend entry (fixture home:
		// answer_document_projection_rnb5b_test.go, donghu-2955 micro shape).
		{"rnb5b_micro_anchor_fold", rnb5bMicroAnchorFoldProjection()},
		// RNB-5B 件⑨ (§29.96.2 终判⑨, 2026-07-15): the endpoint-less
		// multi-window chip + its legend entry (same fixture home).
		{"rnb5b_multi_window_no_endpoints", rnb5bMultiWindowNoEndpointProjection()},
	}
	union := map[runtimeTraceProjMark]bool{}
	for _, fixture := range fixtures {
		marks := revisit76AssertLegendBidirectional(t, fixture.name+"/zh", fixture.proj, true)
		revisit76AssertLegendBidirectional(t, fixture.name+"/en", fixture.proj, false)
		for m := runtimeTraceProjMark(0); m < runtimeTraceProjMarkCount; m++ {
			if marks.has(m) {
				union[m] = true
			}
		}
	}
	// Fixture completeness: every catalog mark is exercised by at least one
	// representative shape, so a stale/never-reachable catalog entry cannot
	// hide behind the dynamic gate.
	for m := runtimeTraceProjMark(0); m < runtimeTraceProjMarkCount; m++ {
		if !union[m] {
			t.Fatalf("no representative fixture exercises mark %d — extend the fixture set", m)
		}
	}
}

// --- NEW-9: evidence-index capacity-truncation disclosure ----------------------

func TestTraceProjectionEvidenceIndexDisclosesCapacityTruncation(t *testing.T) {
	findEvidence := func(blocks []types.AnswerBlock) *types.AnswerBlock {
		for i := range blocks {
			if strings.HasSuffix(blocks[i].ID, "_evidence") {
				return &blocks[i]
			}
		}
		return nil
	}
	// Absent shape: no typed truncation flag → header byte-identical.
	projection := revisit76ResidualProjection()
	plain := findEvidence(runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}))
	if plain == nil {
		t.Fatalf("fixture must render an evidence-index block")
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 按容量截断(rank 头部完整保留),超出容量的尾部行未纳入本索引 → 超过单次返回上限:各自排序靠前的部分完整保留,超限的尾部行不在本索引内 (证据索引族)
	if strings.Contains(plain.Text, "超过单次返回上限") || strings.Contains(plain.Text, "按容量截断") {
		t.Fatalf("untruncated projection must not carry the disclosure:\n%s", plain.Text)
	}
	// Present shape: the typed flag (lifted from the producer's
	// capacity_truncated note at compile) adds exactly one disclosure sentence.
	projection.CapacityTruncated = true
	truncated := findEvidence(runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}))
	if truncated == nil {
		t.Fatalf("truncated fixture must render an evidence-index block")
	}
	// PTV6-C ruling C (#73): the disclosure states the fact without the
	// intermediate-record deflection (负向臂 below).
	if !strings.Contains(truncated.Text, " 部分查询结果超过单次返回上限:各自排序靠前的部分完整保留,超限的尾部行不在本索引内。") {
		t.Fatalf("evidence-index header must disclose the capacity truncation:\n%s", truncated.Text)
	}
	if strings.Contains(truncated.Text, "见原始 trace_query 记录") {
		t.Fatalf("retired intermediate-record pointer resurfaced:\n%s", truncated.Text)
	}
	en := findEvidence(runtimeTraceCausalProjectionCluster(projection, "en", runtimeTraceProjUserFocus{}))
	if en == nil || !strings.Contains(en.Text, "Some query results exceeded the per-call return limit: the top of each result's own ordering is fully kept; the over-limit tail rows are not in this index.") {
		t.Fatalf("en evidence-index header must mirror the disclosure: %+v", en)
	}
}

// --- NEW-5: enumeration primary column header fork ----------------------------

func TestPrincipalEnumerationHeaderForksOnChainDescriptionRows(t *testing.T) {
	// F4 (adversarial re-review 2026-07-04): the fork requires a UNIFORM chain
	// table — every non-empty row carries " -> ".
	chainRows := []types.EnumerationDisplayRow{
		{Member: "binder release -> monitor contention -> main-21538"},
		{Member: "UIThread sleep -> binder reply -> worker-7"},
	}
	if got := principalEnumerationPrimaryColumnLabel(true, chainRows); got != "链路/条目" {
		t.Fatalf("uniform chain-description rows must not sit under 符号名称: %q", got)
	}
	if got := principalEnumerationPrimaryColumnLabel(false, chainRows); got != "Chain / entry" {
		t.Fatalf("en chain header mismatch: %q", got)
	}
	// F4 pin: a MIXED table (one chain row inside symbol rows) keeps 符号名称 —
	// the pre-F4 any-row flip misdescribed every symbol cell.
	mixedRows := []types.EnumerationDisplayRow{
		{Member: "binder release -> monitor contention -> main-21538"},
		{Member: "ParseConfig"},
	}
	if got := principalEnumerationPrimaryColumnLabel(true, mixedRows); got != "符号名称" {
		t.Fatalf("a mixed table must keep the 符号名称 header: %q", got)
	}
	if got := principalEnumerationPrimaryColumnLabel(false, mixedRows); got != "Name" {
		t.Fatalf("en mixed header mismatch: %q", got)
	}
	symbolRows := []types.EnumerationDisplayRow{
		{Member: "ParseConfig"},
		// A bare "->" without spaces (C++ member access) is NOT a chain form.
		{Member: "cfg->Load"},
	}
	if got := principalEnumerationPrimaryColumnLabel(true, symbolRows); got != "符号名称" {
		t.Fatalf("symbol rows keep the 符号名称 header: %q", got)
	}
	if got := principalEnumerationPrimaryColumnLabel(false, symbolRows); got != "Name" {
		t.Fatalf("en symbol header mismatch: %q", got)
	}
}

// revisit76UXR1MentionFloorProjection (UXR-1 §29.36.3, 2026-07-11): a chain
// board (rank #1 runnable) plus an ON-CHAIN semantic span WITHOUT a seat —
// the channel-4 mention-obligation shape (✦ row + 优化点·未入根因排序前N).
func revisit76UXR1MentionFloorProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "uxr1-rank",
			Subject: "worker-9", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", Predicate: "root_cause_primary",
			Rank: 1, Tier: "primary", ChainRelevance: "on_chain",
			Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 12.0, CumulativeImpactMS: 12.0, EffectiveImpactMS: 12.0,
			LineStart: 10, LineEnd: 20, Confidence: 0.8,
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "uxr1-sem",
			Subject: "worker-9", Predicate: "trace_semantic_span",
			Object: "class_verification", SpanName: "VerifyClass demo.App",
			SemanticClass: "class_verification", ChainRelevance: "on_chain",
			Causality: "on_wakeup_chain",
			ImpactMS:  2.4, CumulativeImpactMS: 2.4, EffectiveImpactMS: 2.4,
			LineStart: 30, LineEnd: 40, Confidence: 0.7,
		}},
		// §29.36② glyph fork: a ▒ D-state row wears ⧗ (off-chain family).
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "uxr1-bg",
			Subject: "io-daemon-77", Object: "d_state_or_io_wait",
			TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			Predicate: "root_cause_context", ChainRelevance: "background",
			Causality: "background",
			ImpactMS:  5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
			LineStart: 50, LineEnd: 60, Confidence: 0.6,
		}},
	}
}
