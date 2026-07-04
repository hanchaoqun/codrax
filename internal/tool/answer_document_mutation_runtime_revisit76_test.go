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
	return types.TraceCausalProjectionNode{
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
	if !strings.Contains(fence, "同段IO另有 io_burst_episode 226.153ms、io_wait 112.011/107.672ms 口径;证据 E2、E3、E4") {
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
	cells := rows[0].Cells
	// Cells: 层级(0) 因果位置(1) 节点/原因(2) 类型(3) 关系(4) …
	if !strings.Contains(cells[2], "(同段IO另有 io_burst_episode 226.153ms、io_wait 112.011/107.672ms 口径") {
		t.Fatalf("the lossless surface must mirror the caliber note on the primary row: %q", cells[2])
	}
	if !strings.HasPrefix(cells[4], "唤醒 ▸ ") {
		t.Fatalf("chain-attached IO row keeps the wake relation cell (fence-consistent): %q", cells[4])
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
	_, rows := runtimeTraceProjDetailTable(model, true)
	var ownRelation string
	for _, row := range rows {
		if strings.Contains(row.Cells[2], "com.xs.fm.lite-21538") {
			ownRelation = row.Cells[4]
		}
	}
	if ownRelation != "自身进程IO" {
		t.Fatalf("own-process IO relation cell must mirror the own edge: %q", ownRelation)
	}
	// en mirror: fence edge and relation stay isomorphic.
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "own─") || strings.Contains(enFence, "wakes─") {
		t.Fatalf("en fence must mirror the own edge:\n%s", enFence)
	}
	_, enRows := runtimeTraceProjDetailTable(enModel, false)
	ownRelation = ""
	for _, row := range enRows {
		if strings.Contains(row.Cells[2], "com.xs.fm.lite-21538") {
			ownRelation = row.Cells[4]
		}
	}
	if ownRelation != "own-process IO" {
		t.Fatalf("en own-process IO relation cell mismatch: %q", ownRelation)
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
	if !strings.Contains(line, "on-chain 已归因 112.011ms") {
		t.Fatalf("attributed must stay the chain lane's cumulative:\n%s", line)
	}
	// NEW-6 cites the DEPTHLESS side (the residual-overlap lane), never the
	// attributed chain row's value.
	if !strings.Contains(line, "残差中最大 232.428ms 与自身 IO 口径行(E3)重叠解释") {
		t.Fatalf("NEW-6 clause must take the depthless side value:\n%s", line)
	}
	// Same-lane overlap beside the cross-lane pair still folds (no regression
	// of the NEW-3 pins): a second DEPTHLESS io_wait folds into the depthless
	// primary while the chain row stays untouched.
	projection.OnChainCauses = append(projection.OnChainCauses,
		depthless("io-own-wait", "io_wait", 107.672, 1250, 1750))
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "同段IO另有 io_wait 107.672ms 口径;证据 E4") {
		t.Fatalf("same-lane depthless calibers must keep folding:\n%s", fence)
	}
	if !strings.Contains(fence, "112.011") {
		t.Fatalf("the chain-attached row must survive the same-lane fold:\n%s", fence)
	}
	if line := runtimeTraceProjWindowLine(projection, model, true); !strings.Contains(line, "on-chain 已归因 112.011ms") {
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
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 1 {
		t.Fatalf("expected the single IO detail row: %+v", rows)
	}
	if !strings.HasPrefix(rows[0].Cells[4], "唤醒 ▸ ") {
		t.Fatalf("a different-pid IO row keeps its wake relation: %q", rows[0].Cells[4])
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
	// (Wording pin updated for RN-6 §7.9: 目标等待(睡眠/阻塞/就绪) — the
	// denominator family now includes runnable.)
	if !strings.Contains(line, "目标等待(睡眠/阻塞/就绪) 260.000ms 中 on-chain 已归因 26.000ms(10%),未归因 234.000ms(90%)。") {
		t.Fatalf("coverage line must keep the symptom-denominator form:\n%s", line)
	}
	// NEW-6: the appended clause carries the NEW-3 grouped primary value
	// (232.428, the fold survivor — never a folded peer's) and its evidence tag
	// verbatim (E3 = the primary IO row's index entry).
	if !strings.Contains(line, "残差中最大 232.428ms 与自身 IO 口径行(E3)重叠解释,未计入链归因以防双计。") {
		t.Fatalf("coverage line must self-explain the own-caliber residual overlap:\n%s", line)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	en := runtimeTraceProjWindowLine(projection, enModel, false)
	if !strings.Contains(en, "Up to 232.428ms of the residual is co-explained by the own-process IO caliber row (E3); it is excluded from the chain attribution to avoid double counting.") {
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
	if !strings.Contains(line, "残差中最大 150.000ms 与自身 IO 口径行(E9)重叠解释,未计入链归因以防双计。") {
		t.Fatalf("self-lane hop-view IO caliber must carry the clause:\n%s", line)
	}
	// The published amount is bounded by the residual itself: a caliber row can
	// overlap attributed wall clock too, and the clause must never claim more
	// residual than exists (residual 260-200=60 < caliber 150).
	capped := runtimeTraceProjWindowLine(projection, mkModel(200), true)
	if !strings.Contains(capped, "残差中最大 60.000ms 与自身 IO 口径行(E9)重叠解释") {
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
		runtimeTraceProjMarkRootTarget:       {"🎯", "🎯"},
		runtimeTraceProjMarkEdgeDrill:        {"下钻─", "drill─"},
		runtimeTraceProjMarkEdgeWake:         {"唤醒─", "wakes─"},
		runtimeTraceProjMarkEdgeCause:        {"成因─", "cause─"},
		runtimeTraceProjMarkEdgeOwn:          {"自身─", "own─"},
		runtimeTraceProjMarkSemanticSpan:     {"✦", "✦"},
		runtimeTraceProjMarkIconSleep:        {"💤", "💤"},
		runtimeTraceProjMarkIconRunnable:     {"⏳", "⏳"},
		runtimeTraceProjMarkIconRunning:      {"⚙", "⚙"},
		runtimeTraceProjMarkIconDState:       {"⛓", "⛓"},
		runtimeTraceProjMarkIconTransit:      {"◦", "◦"},
		runtimeTraceProjMarkStateLabel:       {"", ""},
		runtimeTraceProjMarkUndrillable:      {"⛔", "⛔"},
		runtimeTraceProjMarkCrossWindow:      {"⚠跨窗", "⚠crosses window"},
		runtimeTraceProjMarkRecursOnChain:    {"↺", "↺"},
		runtimeTraceProjMarkOmitted:          {"省略", "nodes omitted"},
		runtimeTraceProjMarkIOCaliberNote:    {"同段IO另有", "same-segment IO also measured"},
		runtimeTraceProjMarkPeriodicSource:   {"周期性信号源", "periodic signal source"},
		runtimeTraceProjMarkAdjacentStanza:   {"◇", "◇"},
		runtimeTraceProjMarkBackgroundStanza: {"▒", "▒"},
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

// revisit76AssertLegendBidirectional renders one shape and asserts the NEW-7
// two-way contract: (a) typed marks ⇔ rendered legend entries; (b) for every
// probed mark, its fence token appears IFF its legend entry renders.
func revisit76AssertLegendBidirectional(t *testing.T, name string, projection types.TraceCausalProjection, zh bool) *runtimeTraceProjMarkSet {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	fence := runtimeTraceProjTreeFence(model, zh)
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
		if emitted := strings.Contains(fence, probe); emitted != rendered {
			t.Fatalf("%s: bidirectional violation for mark %d (probe %q): fence emits=%v, legend explains=%v\nfence:\n%s\nlead:\n%s",
				name, entry.Mark, probe, emitted, rendered, fence, lead)
		}
	}
	return model.Marks
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
	if strings.Contains(plain.Text, "按容量截断") {
		t.Fatalf("untruncated projection must not carry the disclosure:\n%s", plain.Text)
	}
	// Present shape: the typed flag (lifted from the producer's
	// capacity_truncated note at compile) adds exactly one disclosure sentence.
	projection.CapacityTruncated = true
	truncated := findEvidence(runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}))
	if truncated == nil {
		t.Fatalf("truncated fixture must render an evidence-index block")
	}
	if !strings.Contains(truncated.Text, "部分来源结果按容量截断(rank 头部完整保留);完整明细见原始 trace_query 记录。") {
		t.Fatalf("evidence-index header must disclose the capacity truncation:\n%s", truncated.Text)
	}
	en := findEvidence(runtimeTraceCausalProjectionCluster(projection, "en", runtimeTraceProjUserFocus{}))
	if en == nil || !strings.Contains(en.Text, "Some source results were capacity-truncated (rank heads fully kept); the full detail remains in the original trace_query records.") {
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
