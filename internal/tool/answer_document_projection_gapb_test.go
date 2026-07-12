package tool

// GAP-B (Wave-3.1 投影/显示批) display pins, ledger
// docs/design/real_trace_campaign_20260705.md §27/§28.3, 2026-07-09.
//
//   - G4 (§27.2): the depth attach carries a WINDOW dimension beside the P0-E
//     branch dimension — branch ordinals are per-query-window and collide
//     across windows (huadong_79: W2 hmfs L2 under the W1 touch chain, fake
//     "唤醒 OS_mmi_EventHdr" edge). Zero-value arm audited separately:
//     a window-less node on a windowed trunk keeps the honest 父节点未确认 seat;
//     a window-less TRUNK leaves the gate inert (legacy byte-stable).
//   - G5 (§27.3): trunk same-(thread,dominant-state) occurrences fold into
//     ONE ×N row (threshold 2 — the self-cause-of-itself edge is a semantic
//     error, not a row-count economy); different-state extras keep the
//     成因 decomposition edge.
//   - G6 (§27.3): the key-metric table's 有效归因 column is "—" on
//     wait-symptom target-self rows (typed tier), while 自因四态 self rows
//     (normal tiers, §24.17) keep their value — both directions pinned.
//   - G7 (§27.3): the inversion 行1 word follows the 行1 VALUE's lane
//     (词值同源 — the window projection's dominant state), the composition
//     staying on 行3.
//   - G8 (§27.3): "value(caliber)" super-atoms and bare core-class caliber
//     words never break mid-claim.
//   - G11 (§27.5): the target's own wait-symptom stanza rows relocate into
//     the self-state area (bounded top-K + overflow disclosure).
//   - G17 (§27.4): the hand-off chain label names its members as THREADS.
//
// MUTATION self-checks (手工改坏→确认咬红→恢复, recorded in the batch report):
//   - dropping the trunkWindowed continue-arms reds
//     TestGAPBWindowDomainAttach (the W2 node re-attaches under W1);
//   - dropping the buildTrunk fold call reds
//     TestGAPBTrunkSameStateOccurrenceFold (the self-cause edge returns);
//   - dropping the IsTargetSelfStateRow dash arm reds
//     TestGAPBSymptomRowAttributionDash;
//   - reverting the G7 window-lane switch reds
//     TestGAPBInversionWordFollowsValue;
//   - removing the value+paren fusion or the bare cluster-word atoms reds
//     the two wrap pins;
//   - dropping the relocation pass reds TestGAPBSelfWaitRowsVisible;
//   - reverting the G17 label reds TestGAPBHandoffChainLabelNamesThreads.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- G4: window domain attach --------------------------------------------------

func gapbWindowedTrunkProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		// huadong_79 replica shape: W1 trunk (touch chain), branch 1, window
		// 100.0–100.2; the W2 node below carries the SAME branch ordinal
		// (per-window numbering collision) at ChainDepth 2.
		WakeupPath:                   []string{"hm-real-up-5", "OS_mmi_EventHdr-43103", "user.app-100"},
		WakeupPathBranch:             1,
		WakeupPathQueryWindowStartTs: 100.0,
		WakeupPathQueryWindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "gapb-w2",
				Subject: "hmfs_discard-777", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 2, ChainBranch: 1,
				QueryWindowStartTs: 200.0, QueryWindowEndTs: 200.3,
				ImpactMS: 3.2, CumulativeImpactMS: 3.2, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "gapb-same",
				Subject: "same-win-666", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 2, ChainBranch: 1,
				QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.2,
				ImpactMS: 2.0, CumulativeImpactMS: 2.0, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "gapb-zero",
				Subject: "no-window-555", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 2, ChainBranch: 1,
				ImpactMS: 1.0, CumulativeImpactMS: 1.0, Confidence: 0.8},
			// 复核 P3-1 (2026-07-09): the NESTED-window shape (same start,
			// different end — the window-refinement re-query form). Only the
			// END endpoint distinguishes it from the trunk window, so this
			// fixture is the mutation witness for the End-comparison arm (the
			// original fixtures differed on BOTH endpoints and survived a
			// deleted End comparison).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "gapb-nested",
				Subject: "nested-win-444", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", ChainDepth: 2, ChainBranch: 1,
				QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.35,
				ImpactMS: 0.7, CumulativeImpactMS: 0.7, Confidence: 0.8},
		},
	}
}

// huadong_79 复刻 pin: W1 trunk L1/L2 + a W2 node with the SAME branch ordinal
// and ChainDepth=2 must NOT attach into the tree — it keeps the honest
// 父节点未确认 (depthless) seat. The same-window control attaches; the
// window-less node conservatively stays unattached (缺窗身份≠可挂靠).
func TestGAPBWindowDomainAttach(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(gapbWindowedTrunkProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	kinds := map[string]string{}
	parents := map[string]string{}
	for _, row := range model.TreeRows {
		kinds[row.Node.Subject] = row.Kind
		parents[row.Node.Subject] = row.Parent
	}
	if kinds["hmfs_discard-777"] != runtimeTraceProjTreeRowDepthless {
		t.Fatalf("the cross-window node must keep the honest 父节点未确认 seat, got kind %q parent %q",
			kinds["hmfs_discard-777"], parents["hmfs_discard-777"])
	}
	if kinds["same-win-666"] != runtimeTraceProjTreeRowChain || parents["same-win-666"] != "OS_mmi_EventHdr-43103" {
		t.Fatalf("the same-window node must attach under the W1 trunk, got kind %q parent %q",
			kinds["same-win-666"], parents["same-win-666"])
	}
	if kinds["no-window-555"] != runtimeTraceProjTreeRowDepthless {
		t.Fatalf("a window-less node on a WINDOWED trunk cannot prove domain membership — honest unattached seat, got %q",
			kinds["no-window-555"])
	}
	if kinds["nested-win-444"] != runtimeTraceProjTreeRowDepthless {
		t.Fatalf("复核 P3-1: a NESTED window (same start, refined end) is a different window — honest unattached seat, got %q",
			kinds["nested-win-444"])
	}
	// The fake edge itself must be gone: no rendered fence line may claim the
	// cross-window node wakes the W1 chain.
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "hmfs_discard-777") && strings.Contains(line, "唤醒─") {
			t.Fatalf("fabricated cross-window wake edge rendered: %q\n%s", line, fence)
		}
	}
}

// Legacy control (byte-stable lane): a trunk WITHOUT window identity leaves
// the window gate inert — a windowed node keeps the legacy depth attach
// (absence never manufactures a rejection domain).
func TestGAPBWindowlessTrunkKeepsLegacyAttach(t *testing.T) {
	projection := gapbWindowedTrunkProjection()
	projection.WakeupPathQueryWindowStartTs = 0
	projection.WakeupPathQueryWindowEndTs = 0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.Subject == "hmfs_discard-777" && row.Kind != runtimeTraceProjTreeRowChain {
			t.Fatalf("window-less trunk must keep the legacy attach for windowed nodes, got kind %q", row.Kind)
		}
	}
}

// --- G5: trunk ×2 same-state occurrence fold ------------------------------------

func gapbTrunkOccurrenceProjection() types.TraceCausalProjection {
	subject := "OS_mmi_EventHdr-43103"
	return types.TraceCausalProjection{
		WakeupPath: []string{subject, "user.app-100"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "occ-big",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 4.431, CumulativeImpactMS: 4.431, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "occ-small",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 0.904, CumulativeImpactMS: 0.904, Confidence: 0.8},
			// Different-state extra: a REAL decomposition — keeps the 成因 edge.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "occ-running",
				Subject: subject, Object: "running", StateKind: "running",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 0.5, CumulativeImpactMS: 0.5, Confidence: 0.8},
		},
	}
}

// huadong_79 G5 pin: two same-(thread,state) occurrences render as ONE ×2 row
// (SUM + a–b range) — never as a "├─成因─" child of themselves. The
// different-state extra keeps the honest decomposition edge.
func TestGAPBTrunkSameStateOccurrenceFold(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(gapbTrunkOccurrenceProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var chainRow *runtimeTraceProjTreeRow
	causeStates := []string{}
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if row.Kind == runtimeTraceProjTreeRowChain && row.Node.Subject == "OS_mmi_EventHdr-43103" {
			chainRow = row
		}
		if row.Kind == runtimeTraceProjTreeRowCause {
			causeStates = append(causeStates, row.Node.StateKind)
		}
	}
	if chainRow == nil {
		t.Fatalf("trunk chain row not found")
	}
	if chainRow.Node.MergedCount != 2 || chainRow.Node.MergedMinMS != 0.904 || chainRow.Node.MergedMaxMS != 4.431 {
		t.Fatalf("same-state occurrences must fold to ×2(0.904–4.431ms), got ×%d(%.3f–%.3f)",
			chainRow.Node.MergedCount, chainRow.Node.MergedMinMS, chainRow.Node.MergedMaxMS)
	}
	if chainRow.Node.ImpactMS != 4.431+0.904 {
		t.Fatalf("×2 value must be the member SUM 5.335, got %.3f", chainRow.Node.ImpactMS)
	}
	for _, state := range causeStates {
		if strings.EqualFold(state, "s_sleep") {
			t.Fatalf("a same-state occurrence rendered as its own 成因 child (自因自指形): %v", causeStates)
		}
	}
	if len(causeStates) != 1 || causeStates[0] != "running" {
		t.Fatalf("the different-state extra must keep its 成因 decomposition edge, got %v", causeStates)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "×2(0.904–4.431ms)") {
		t.Fatalf("the ×N grammar tag must render on the folded row:\n%s", fence)
	}
	if !strings.Contains(fence, "5.335ms") {
		t.Fatalf("the folded row must publish the SUM value:\n%s", fence)
	}
}

// --- G6: 有效归因 column symptom gate -------------------------------------------

// 双向 pin: the wait-symptom target-self row's attribution cell is "—" (the
// column definition is 计入根因排序 and the row never seats on that board);
// the 自因四态 self row (normal tier + rank, §24.17) keeps its value.
func TestGAPBSymptomRowAttributionDash(t *testing.T) {
	target := "user.app-100"
	symptom := types.TraceCausalProjection{
		WakeupPath: []string{"up-1", target},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "sym-1",
			Subject: target, Object: "sleep_wait", StateKind: "s_sleep",
			Tier: types.TraceCausalTierTargetSelfState, Rank: 6,
			ChainRelevance: "on_chain",
			ImpactMS:       6.357, CumulativeImpactMS: 6.357, EffectiveImpactMS: 6.357,
			Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(symptom, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) == 0 {
		t.Fatalf("symptom row missing from the key-metric table")
	}
	if got := rows[0].Cells[3]; got != "—" {
		t.Fatalf("wait-symptom self row's 有效归因 cell must be —, got %q", got)
	}
	if !runtimeTraceProjDetailTableLegendFlagsFor(model, true).selfSymptom {
		t.Fatalf("the gated symptom legend flag must raise with the row on the table")
	}

	// Preserve arm: a decomposable self-cause row (normal tier, rank seat)
	// keeps its attribution value — 勿一刀切.
	selfCause := types.TraceCausalProjection{
		WakeupPath: []string{"up-1", target},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "cause-1",
			Subject: target, Object: "running", StateKind: "running",
			Tier: "primary", Rank: 1,
			ChainRelevance: "on_chain",
			ImpactMS:       4.2, CumulativeImpactMS: 4.2, EffectiveImpactMS: 4.2,
			Confidence: 0.8,
		}},
	}
	causeModel := buildRuntimeTraceProjTreeModel(selfCause, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_, causeRows := runtimeTraceProjDetailTable(causeModel, true)
	if len(causeRows) == 0 {
		t.Fatalf("self-cause row missing from the key-metric table")
	}
	if got := causeRows[0].Cells[3]; !strings.Contains(got, "4.200ms") {
		t.Fatalf("自因四态 self row must keep its attribution value, got %q", got)
	}
	if runtimeTraceProjDetailTableLegendFlagsFor(causeModel, true).selfSymptom {
		t.Fatalf("the symptom legend flag must stay quiet without a target_self_state row")
	}
}

// --- G7: 行1 词值同源 -----------------------------------------------------------

func TestGAPBInversionWordFollowsValue(t *testing.T) {
	base := types.TraceCausalProjectionNode{
		Object: "running", PriorityInversionCandidate: true,
		GatedRunnableMS: 0.621, GatedRunningDeficitMS: 2.149,
	}
	// E17/E16 形: the 行1 value is the window lane (ImpactMS) and its typed
	// dominant state is running — the word is running, never the composition.
	running := base
	running.StateKind, running.ImpactMS = "running", 4.115
	if got := runtimeTraceProjInversionStateCompositionWord(running); got != "running" {
		t.Fatalf("行1 word must follow the window lane's dominant state, got %q", got)
	}
	runnable := base
	runnable.StateKind, runnable.ImpactMS = "runnable", 4.115
	if got := runtimeTraceProjInversionStateCompositionWord(runnable); got != "runnable" {
		t.Fatalf("行1 word must follow a runnable-dominant window lane, got %q", got)
	}
	// Lossy absence fails open to the gated composition (legacy).
	stateless := base
	stateless.ImpactMS = 4.115
	if got := runtimeTraceProjInversionStateCompositionWord(stateless); got != "runnable+running" {
		t.Fatalf("a StateKind-less row must keep the composition fallback, got %q", got)
	}
	// Off-window value lane (ImpactMS==0): the composition word stays.
	offWindow := base
	offWindow.StateKind, offWindow.EffectiveImpactMS = "running", 2.770
	if got := runtimeTraceProjInversionStateCompositionWord(offWindow); got != "runnable+running" {
		t.Fatalf("an off-window-lane value keeps the composition word, got %q", got)
	}
}

// --- G8: wrap atoms --------------------------------------------------------------

// 孤儿括注 regression: the "value(caliber)" pair is ONE claim — no wrap width
// may strand "(全额)" on its own line or split it off its value.
func TestGAPBWrapValueCaliberSuperAtom(t *testing.T) {
	text := "有效归因 0.058ms(全额) 计入排序"
	for width := 14; width <= 40; width++ {
		lines := runtimeTraceProjWrapDisplay(text, width)
		if strings.Join(lines, "") != text {
			t.Fatalf("width %d: wrap must stay byte-identical, got %q", width, lines)
		}
		for _, line := range lines {
			if strings.Contains(line, "0.058ms") && !strings.Contains(line, "0.058ms(全额)") {
				t.Fatalf("width %d: the caliber split off its value: %q", width, lines)
			}
			if strings.TrimSpace(line) == "(全额)" {
				t.Fatalf("width %d: orphan caliber line: %q", width, lines)
			}
		}
	}
}

// "小/核" 拆分 regression: the bare core-class caliber words are unbreakable
// atoms at every width that can hold them.
func TestGAPBWrapClusterWordsUnbreakable(t *testing.T) {
	text := "同频点:中核=小核×2.3,大核=中核×1.1,超大核=大核×1.2,降频/小核导致跑慢"
	words := []string{"超大核", "大核", "中核", "小核"}
	for width := 8; width <= 40; width++ {
		lines := runtimeTraceProjWrapDisplay(text, width)
		if strings.Join(lines, "") != text {
			t.Fatalf("width %d: wrap must stay byte-identical, got %q", width, lines)
		}
		// Every line-break byte offset must fall OUTSIDE every cluster-word
		// occurrence (a break strictly inside an occurrence is a mid-claim cut).
		offset := 0
		breaks := map[int]bool{}
		for _, line := range lines[:len(lines)-1] {
			offset += len(line)
			breaks[offset] = true
		}
		for _, word := range words {
			for from := 0; ; {
				j := strings.Index(text[from:], word)
				if j < 0 {
					break
				}
				start := from + j
				for cut := start + 1; cut < start+len(word); cut++ {
					if breaks[cut] {
						t.Fatalf("width %d: line break inside %q at byte %d: %q", width, word, cut, lines)
					}
				}
				from = start + len(word)
			}
		}
	}
}

// --- G11: target self wait rows visible ------------------------------------------

func TestGAPBSelfWaitRowsVisible(t *testing.T) {
	target := "user.app-100"
	node := func(id string, ms float64, subject string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: subject, Object: "binder_wait", TypeToken: "binder_wait",
			// 复核 P1-2 fixture 真路径 (2026-07-09): the production shape
			// carries dominant_state=sleep — the original StateKind-less
			// fixture accidentally bypassed the symptom-family denominator
			// admission and pinned the double-count path green.
			StateKind: "s_sleep",
			// 跨批 X1: symptom rows carry NO ordinal post-G9 (production shape).
			Tier: types.TraceCausalTierTargetSelfState, Rank: 0,
			ChainRelevance: "background",
			ImpactMS:       ms, CumulativeImpactMS: ms, EffectiveImpactMS: ms,
			Confidence: 0.8,
		}
	}
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"up-1", target},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			node("bw-1", 3.527, target),
			node("bw-2", 2.100, target),
			node("bw-3", 1.500, target),
			node("bw-4", 0.900, target),
			node("bw-5", 0.400, target),
			// Cursor-shape defense: an engine self-state row of a DIFFERENT
			// query's target (subject ≠ 🎯 target) never relocates.
			node("bw-peer", 5.000, "peer-7"),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) != runtimeTraceProjSelfWaitRelocateMax {
		t.Fatalf("top-%d wait rows must relocate into the self area, got %d",
			runtimeTraceProjSelfWaitRelocateMax, len(model.SelfRows))
	}
	for i, row := range model.SelfRows {
		if !row.SelfSymptomRelocated {
			t.Fatalf("relocated row %d must carry the symptom identity flag", i)
		}
	}
	if model.SelfRows[0].Node.EvidenceID != "bw-1" || model.SelfRows[3].Node.EvidenceID != "bw-4" {
		t.Fatalf("relocation must be sorted by magnitude, got %q..%q",
			model.SelfRows[0].Node.EvidenceID, model.SelfRows[3].Node.EvidenceID)
	}
	if model.SelfWaitOverflowCount != 1 || model.SelfWaitOverflowMaxMS != 0.400 {
		t.Fatalf("overflow disclosure must count the stanza remainder, got %d/%.3f",
			model.SelfWaitOverflowCount, model.SelfWaitOverflowMaxMS)
	}
	background := map[string]bool{}
	for _, row := range model.Background {
		background[row.Node.EvidenceID] = true
	}
	if !background["bw-5"] || !background["bw-peer"] {
		t.Fatalf("the overflow row and the foreign-subject row must keep their stanza seats, got %v", background)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "症状而非根因") {
		t.Fatalf("relocated rows must render with the symptom disclosure:\n%s", fence)
	}
	if !strings.Contains(fence, "另有 1 条自身等待症状行(单条最大 0.400ms)") {
		t.Fatalf("the bounded relocation must disclose its overflow:\n%s", fence)
	}
	// 无榜位: the self area never speaks a rank seat for the symptom rows.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "症状而非根因") && strings.Contains(line, "根因排序#") {
			t.Fatalf("a relocated symptom row must not wear a rank seat: %q", line)
		}
	}
}

// --- G17: hand-off chain label -----------------------------------------------------

func TestGAPBHandoffChainLabelNamesThreads(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"holder-1", "user.app-100"},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "lock-1",
			Subject: "holder-1", Object: "blocking_span", BlockingKind: "lock_contention",
			BlockingHolderHandoff: "threadA-11 --> threadB-22",
			ChainRelevance:        "on_chain", ChainDepth: 1,
			ImpactMS: 12.0, CumulativeImpactMS: 12.0, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	zh := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(zh, "持有者移交链(线程)") {
		t.Fatalf("zh hand-off label must name the members as threads:\n%s", zh)
	}
	en := runtimeTraceProjDetailFullText(model, false)
	if !strings.Contains(en, "holder hand-off chain (threads)") {
		t.Fatalf("en hand-off label must name the members as threads:\n%s", en)
	}
}

// --- 复核收尾 pins (SHIP-WITH-FIXES, 2026-07-09) -----------------------------------

// P1-1 (tool half): a wakeup_causal_aggregate row is a DERIVED VIEW whose
// per-hop member rows are retained beside it — the trunk ×N fold must never
// merge the view with its own members (the REPRO summed 5.335+4.431+0.904
// into a ×3 "10.670ms", exactly 2× the truth).
func TestGAPBAggregateViewNeverMergesWithMembers(t *testing.T) {
	subject := "OS_mmi_EventHdr-43103"
	projection := types.TraceCausalProjection{
		WakeupPath: []string{subject, "user.app-100"},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "agg-1",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				Predicate:      "wakeup_causal_aggregate",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 5.335, CumulativeImpactMS: 5.335, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "occ-1",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				Predicate:      "wakeup_causal_impact",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 4.431, CumulativeImpactMS: 4.431, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "occ-2",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				Predicate:      "wakeup_causal_impact",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 0.904, CumulativeImpactMS: 0.904, Confidence: 0.8},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var chain *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Kind == runtimeTraceProjTreeRowChain &&
			model.TreeRows[i].Node.Subject == subject {
			chain = &model.TreeRows[i]
		}
	}
	if chain == nil {
		t.Fatalf("trunk chain row not found")
	}
	if chain.Node.MergedCount > 1 || chain.Node.ImpactMS != 5.335 {
		t.Fatalf("the aggregate view must never merge with its members, got ×%d %.3fms",
			chain.Node.MergedCount, chain.Node.ImpactMS)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "10.670") {
		t.Fatalf("view+member double count leaked into the fence:\n%s", fence)
	}
}

// P2-1: the same-name trunk capture carries the SAME chain-domain gate as the
// depth attach — a cross-window node whose canonical subject collides with a
// trunk subject must never hijack the trunk main/extra selection.
func TestGAPBTrunkSubjectCaptureRespectsWindowDomain(t *testing.T) {
	subject := "OS_mmi_EventHdr-43103"
	projection := types.TraceCausalProjection{
		WakeupPath:                   []string{subject, "user.app-100"},
		WakeupPathQueryWindowStartTs: 100.0,
		WakeupPathQueryWindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "w1-occ",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1,
				QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.2,
				ImpactMS: 4.431, CumulativeImpactMS: 4.431, Confidence: 0.8},
			// The W2 hijacker: larger magnitude, same canonical subject.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "w2-hijack",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1,
				QueryWindowStartTs: 200.0, QueryWindowEndTs: 200.3,
				ImpactMS: 9.9, CumulativeImpactMS: 9.9, Confidence: 0.8},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var chainID, depthlessID string
	for _, row := range model.TreeRows {
		switch row.Kind {
		case runtimeTraceProjTreeRowChain:
			chainID = row.Node.EvidenceID
		case runtimeTraceProjTreeRowDepthless:
			depthlessID = row.Node.EvidenceID
		}
	}
	if chainID != "w1-occ" {
		t.Fatalf("the trunk main must be the SAME-WINDOW node, got %q", chainID)
	}
	if depthlessID != "w2-hijack" {
		t.Fatalf("the cross-window same-name node must keep the honest unattached seat, got %q", depthlessID)
	}
}

// P2-1 branch variant: the same-name capture rejects cross-branch nodes on a
// BRANCH trunk (one shared domain helper — both arms bite on both surfaces).
func TestGAPBTrunkSubjectCaptureRespectsBranchDomain(t *testing.T) {
	subject := "OS_mmi_EventHdr-43103"
	projection := types.TraceCausalProjection{
		WakeupPath:       []string{subject, "user.app-100"},
		WakeupPathBranch: 1,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "b1-occ",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 1,
				ImpactMS: 2.0, CumulativeImpactMS: 2.0, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "b3-hijack",
				Subject: subject, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 3,
				ImpactMS: 7.7, CumulativeImpactMS: 7.7, Confidence: 0.8},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.EvidenceID == "b3-hijack" && row.Kind == runtimeTraceProjTreeRowChain {
			t.Fatalf("a cross-branch same-name node hijacked the trunk row: %+v", row.Node)
		}
		if row.Node.EvidenceID == "b1-occ" && row.Kind != runtimeTraceProjTreeRowChain {
			t.Fatalf("the same-branch node must keep the trunk seat, got kind %q", row.Kind)
		}
	}
}

// P1-2: relocated G11 rows re-describe wall clock already inside the target's
// own state segments — the symptom denominator and its census must skip them
// (REPRO: outer sleep 10.0 + relocated 8.0 → 18.000 denominator, the
// F1-forbidden nesting shape).
func TestGAPBRelocatedRowsStayOutOfSymptomDenominator(t *testing.T) {
	target := "user.app-100"
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"up-1", target},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "outer-sleep",
			Subject: target, Object: "sleep_wait", StateKind: "s_sleep",
			ChainRelevance: "on_chain",
			ImpactMS:       10.0, CumulativeImpactMS: 10.0, Confidence: 0.8,
		}},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "nested-binder",
				Subject: target, Object: "binder_wait", TypeToken: "binder_wait",
				StateKind: "s_sleep", Tier: types.TraceCausalTierTargetSelfState,
				ChainRelevance: "background",
				ImpactMS:       8.0, CumulativeImpactMS: 8.0, EffectiveImpactMS: 8.0,
				Confidence: 0.8,
			},
			// The StateKind-LESS wait-token shape (huadong binder rows carry a
			// wait token but no dominant state): the denominator mirror never
			// admits it, so ONLY the census skip arm keeps it out of the
			// "未计入" under-representation disclosure — it is fully
			// represented on its own relocated self row.
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "nested-binder-2",
				Subject: target, Object: "binder_wait", TypeToken: "binder_wait",
				Tier:           types.TraceCausalTierTargetSelfState,
				ChainRelevance: "background",
				ImpactMS:       3.5, CumulativeImpactMS: 3.5, EffectiveImpactMS: 3.5,
				Confidence: 0.8,
			},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	relocated := false
	for _, row := range model.SelfRows {
		if row.Node.EvidenceID == "nested-binder" && row.SelfSymptomRelocated {
			relocated = true
		}
	}
	if !relocated {
		t.Fatalf("fixture drifted: the nested binder row must relocate into SelfRows")
	}
	if got := runtimeTraceProjTargetSymptomMS(model); got != 10.0 {
		t.Fatalf("the symptom denominator must skip relocated rows (10.000), got %.3f", got)
	}
	if excluded, _, _ := runtimeTraceProjSymptomDenominatorCensus(projection, model); excluded != 0 {
		t.Fatalf("the census must not count relocated rows as under-represented exclusions, got %d", excluded)
	}
}

// P3-2: the overflow disclosure word is 单条最大 — an ×N merged overflow row
// contributes its per-instance MergedMaxMS, never its SUMmed display impact.
func TestGAPBSelfWaitOverflowMaxIsSingleInstance(t *testing.T) {
	target := "user.app-100"
	node := func(id string, ms float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: target, Object: "binder_wait", TypeToken: "binder_wait",
			StateKind: "s_sleep", Tier: types.TraceCausalTierTargetSelfState,
			ChainRelevance: "background",
			ImpactMS:       ms, CumulativeImpactMS: ms, EffectiveImpactMS: ms,
			Confidence: 0.8,
		}
	}
	merged := node("bw-merged", 5.0) // display impact = the ×3 member SUM
	merged.MergedCount = 3
	merged.MergedMinMS = 1.0
	merged.MergedMaxMS = 2.0
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"up-1", target},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			node("bw-1", 10.0), node("bw-2", 9.0), node("bw-3", 8.0), node("bw-4", 7.0),
			merged, // sorts fifth → overflow
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if model.SelfWaitOverflowCount != 1 {
		t.Fatalf("fixture drifted: the merged row must overflow, got count %d", model.SelfWaitOverflowCount)
	}
	if model.SelfWaitOverflowMaxMS != 2.0 {
		t.Fatalf("单条最大 must be the ×N row's per-instance MergedMaxMS (2.000), got %.3f",
			model.SelfWaitOverflowMaxMS)
	}
}

// 跨批 X1: a symptom row WITHOUT a rank ordinal (the G9 production shape,
// Rank=0) must still produce the §24.16 disclosure sentence — the retired
// `Rank <= 0` gate arm silently killed it on every engine shape.
func TestGAPBSelfSymptomNoteSurvivesRankZero(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		Background: []runtimeTraceProjTreeRow{{
			Node: types.TraceCausalProjectionNode{
				Subject: "main-6565", Object: "binder_wait", TypeToken: "binder_wait",
				Tier: types.TraceCausalTierTargetSelfState, Rank: 0,
				ImpactMS: 4.577, CumulativeImpactMS: 4.577, EffectiveImpactMS: 4.577,
			},
			Kind: runtimeTraceProjTreeRowBackground, HasData: true,
		}},
	}
	note := runtimeTraceProjTargetSelfSymptomNote(model, true)
	if !strings.Contains(note, "4.577ms") || !strings.Contains(note, "关注线程自身等待/持锁") {
		t.Fatalf("a Rank=0 symptom row must still produce the disclosure, got %q", note)
	}
}

// 跨批 X2: a count-class family's raw-Σ detail note wears the engine's
// 计数当量 marker (三面同源) — never the bare wall-clock ms form; the
// wall-clock calibers keep their legacy forms byte-identically.
func TestGAPBCountFamilyDetailNoteCountEquivalent(t *testing.T) {
	count := types.TraceCausalProjectionNode{
		FamilyMemberCount: 2, FamilyMemberSumMS: 198.3,
		FamilyFoldCaliber: tracequery.RootCauseMemberFoldCaliberCountSum,
	}
	note := runtimeTraceProjFamilySumDetailNote(count, true)
	if !strings.Contains(note, "计数当量198.300ms") {
		t.Fatalf("count-class Σ must wear the 计数当量 marker, got %q", note)
	}
	if strings.Contains(note, "原始和 198.300ms") {
		t.Fatalf("count-class Σ must never print the bare wall-clock form, got %q", note)
	}
	en := runtimeTraceProjFamilySumDetailNote(count, false)
	if !strings.Contains(en, "count-equivalent 198.300ms") {
		t.Fatalf("EN count-class Σ must carry the count-equivalent marker, got %q", en)
	}
	wall := count
	wall.FamilyFoldCaliber = tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback
	if got := runtimeTraceProjFamilySumDetailNote(wall, true); got != ";原始和 198.300ms 供对照" {
		t.Fatalf("wall-clock calibers keep the legacy bare form, got %q", got)
	}
}
