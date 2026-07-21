package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_smr1_relations_test.go — SMR-1 批 pins
// (smr_audit_report §②/§④, 2026-07-12): WO-A1 (unified non-additive pointer,
// three carriers), WO-D2/D4 (trunk/flat aggregate fold with eff dual-list),
// WO-D3 短期臂 (double-merged twin mutual tags), WO-C1 (account-relation
// sentence), WO-B1 (occurrence-series note, 改词保双席).

// --- WO-A1 carrier (a): self binder ⊂ sleep (SMR-S1 残余, 41006/45701 E3) ----

func smr1A1SelfBinderProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// The sleep seat (dominant account).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-sleep",
				Subject: ".ugc.aweme.lite-17267", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 51.109, CumulativeImpactMS: 51.109,
				Confidence: 0.9, LineStart: 100, LineEnd: 200},
			// The binder_wait refinement rows — carved from the sleep clock.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-binder-1",
				Subject: ".ugc.aweme.lite-17267", Object: "binder:496_9-10961", TypeToken: "binder_wait",
				ChainRelevance: "on_chain", ImpactMS: 36.807, CumulativeImpactMS: 36.807,
				Confidence: 0.8, LineStart: 110, LineEnd: 150},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-binder-2",
				Subject: ".ugc.aweme.lite-17267", Object: "binder:227_4-10625", TypeToken: "binder_wait",
				ChainRelevance: "on_chain", ImpactMS: 14.302, CumulativeImpactMS: 14.302,
				Confidence: 0.8, LineStart: 160, LineEnd: 190},
			// A drill row so the tree renders.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop-1",
				Subject: "app-9511", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 15.565, CumulativeImpactMS: 15.565,
				Confidence: 0.8, LineStart: 300, LineEnd: 320},
		},
	}
}

func TestSMR1A1SelfBinderComponentPointer(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1A1SelfBinderProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var sleepTag string
	binderTagged := 0
	for _, row := range model.SelfRows {
		if !row.HasData {
			continue
		}
		if row.Node.IsSleepState() {
			sleepTag = strings.TrimSpace(row.EvidenceTag)
			continue
		}
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken) == "binder_wait" {
			if row.NonAdditiveKind != runtimeTraceProjNonAdditiveComponent || row.NonAdditiveRef == "" {
				t.Fatalf("SMR-S1 残余: the self binder row must carry the component pointer, got %+v", row.NonAdditiveKind)
			}
			binderTagged++
		}
	}
	if binderTagged != 2 {
		t.Fatalf("both binder refinement rows point at the sleep seat, got %d\n%s", binderTagged, fence)
	}
	if sleepTag == "" || !strings.Contains(fence, "不可相加·为["+sleepTag+"]的组成部分") {
		t.Fatalf("行2 must wear the ONE template word (§④ 词面单源):\n%s", fence)
	}
}

// The io self row (98.7% PARTIAL overlap, not a subset — SMR-S1 vnote) must
// NOT be hard-tagged: typed 键不 fire 时不硬 tag.
func TestSMR1A1PartialOverlapIONeverHardTags(t *testing.T) {
	projection := smr1A1SelfBinderProjection()
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-io",
		Subject: ".ugc.aweme.lite-17267", Object: "io_wait", TypeToken: "io_wait",
		ChainRelevance: "on_chain", ImpactMS: 1.347, CumulativeImpactMS: 1.347,
		Confidence: 0.8, LineStart: 210, LineEnd: 220,
	})
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.SelfRows {
		if row.Node.EvidenceID == "self-io" && row.NonAdditiveRef != "" {
			t.Fatalf("io partial-overlap self row must stay untagged (E4 反例): %+v", row.NonAdditiveKind)
		}
	}
}

// --- WO-A1 carrier (b): addition identity (SMR-S5, 31693 E4/E11) -------------

func smr1A1AdditionIdentityProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"threadpoolforeg-60555", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E4 shape: the trunk D-state ×3 aggregate (17.442).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e4",
				Subject: "threadpoolforeg-60555", Predicate: "wakeup_causal_impact",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 17.442, CumulativeImpactMS: 17.442,
				MergedCount: 3, MergedMinMS: 4.426, MergedMaxMS: 6.768,
				Confidence: 0.8, LineStart: 8712, LineEnd: 15131},
			// E11 shape: the ➍ io_wait rank seat 17.819 = 17.442 + 0.377 with
			// the typed d/io split notes decoded onto the node.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e11",
				Subject: "threadpoolforeg-60555", Predicate: "root_cause_tertiary",
				Object: "io_wait", TypeToken: "io_wait", StateKind: "io_wait",
				ChainRelevance: "on_chain", Rank: 4, Tier: "tertiary",
				ImpactMS: 17.819, CumulativeImpactMS: 24.951,
				DStateSplitMS: 17.442, IOWaitSplitMS: 0.377,
				Confidence: 0.82, LineStart: 8712, LineEnd: 15140},
		},
	}
}

func TestSMR1A1AdditionIdentityComponentPointer(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1A1AdditionIdentityProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var trunk, rank *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		switch row.Node.EvidenceID {
		case "e4":
			trunk = row
		case "e11":
			rank = row
		}
	}
	if trunk == nil || rank == nil {
		t.Fatalf("both seats must render (留+指针, never a fold): %v %v\n%s", trunk, rank, fence)
	}
	if trunk.NonAdditiveKind != runtimeTraceProjNonAdditiveComponent ||
		trunk.NonAdditiveRef != strings.TrimSpace(rank.EvidenceTag) {
		t.Fatalf("E4 (17.442 = the d_state side of E11's typed split) must point component→E11: %+v", trunk.NonAdditiveKind)
	}
	if rank.NonAdditiveKind != runtimeTraceProjNonAdditiveContains ||
		rank.NonAdditiveRef != strings.TrimSpace(trunk.EvidenceTag) {
		t.Fatalf("E11 wears the symmetric 已含 face: %+v", rank.NonAdditiveKind)
	}
	if !strings.Contains(fence, "不可相加·为[") || !strings.Contains(fence, "不可相加·已含[") {
		t.Fatalf("both word faces must render:\n%s", fence)
	}
}

// No typed split = no identity proof = no tag (absence never guesses).
func TestSMR1A1NoSplitNoPointer(t *testing.T) {
	projection := smr1A1AdditionIdentityProjection()
	projection.OnChainCauses[1].DStateSplitMS = 0
	projection.OnChainCauses[1].IOWaitSplitMS = 0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].NonAdditiveRef != "" {
			t.Fatalf("without the typed complement the pair must stay untagged: %+v", model.TreeRows[i].Node.EvidenceID)
		}
	}
}

// --- WO-A1 carrier (c): aggregate↔member (S5-TPF 42729 E17/E4; SMR-S3) -------

func smr1A1MemberProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"networkservice-60595", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// The ×3 sleep aggregate (SMR-S3 trunk E4: 25.558 = 6.620+8.169+10.769).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "agg",
				Subject: "networkservice-60595", Predicate: "wakeup_causal_impact",
				Object: "sleep_wait", TypeToken: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 2,
				ImpactMS: 25.558, CumulativeImpactMS: 25.558,
				MergedCount: 3, MergedMinMS: 6.620, MergedMaxMS: 10.769,
				Confidence: 0.8, LineStart: 5000, LineEnd: 9000},
			// The 游离位 member re-publications (E17/E18: middle + min members).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e17",
				Subject: "networkservice-60595", Predicate: "wakeup_causal_impact",
				Object: "sleep_wait", TypeToken: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 12,
				ImpactMS: 8.169, CumulativeImpactMS: 8.169,
				Confidence: 0.8, LineStart: 6000, LineEnd: 6400},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e18",
				Subject: "networkservice-60595", Predicate: "wakeup_causal_impact",
				Object: "sleep_wait", TypeToken: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 12,
				ImpactMS: 6.620, CumulativeImpactMS: 6.620,
				Confidence: 0.8, LineStart: 7000, LineEnd: 7300},
		},
	}
}

func TestSMR1A1MemberPointerOnFreeSeatCopies(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1A1MemberProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var aggTag string
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "agg" {
			aggTag = strings.TrimSpace(model.TreeRows[i].EvidenceTag)
		}
	}
	tagged := 0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		switch row.Node.EvidenceID {
		case "e17", "e18":
			if row.NonAdditiveKind != runtimeTraceProjNonAdditiveMember || row.NonAdditiveRef != aggTag {
				t.Fatalf("游离成员行 %s must wear 为[%s]成员, got kind=%v ref=%q\n%s",
					row.Node.EvidenceID, aggTag, row.NonAdditiveKind, row.NonAdditiveRef, fence)
			}
			tagged++
		}
	}
	if tagged != 2 {
		t.Fatalf("改词保双席: both member rows stay seated AND tagged, got %d", tagged)
	}
	if !strings.Contains(fence, "不可相加·为["+aggTag+"]成员") {
		t.Fatalf("member word face must render:\n%s", fence)
	}
}

// A value that matches NO derivable member never tags (µs 恒等才算成员盘存).
func TestSMR1A1NonMemberValueStaysBare(t *testing.T) {
	projection := smr1A1MemberProjection()
	projection.OnChainCauses[1].ImpactMS = 8.200 // ≠ derivable middle 8.169
	projection.OnChainCauses[1].CumulativeImpactMS = 8.200
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e17" && model.TreeRows[i].NonAdditiveRef != "" {
			t.Fatalf("non-member value must stay bare (防巧合假阳)")
		}
	}
}

// --- WO-D2/D4: trunk/flat aggregate fold (S2-TPF 56643 E5/E19) ---------------

func smr1D2BranchTwinProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:                   []string{"threadpoolforeg-60555", "com.baidu.tieba-59566"},
		WakeupPathBranch:             1,
		WakeupPathQueryWindowStartTs: 34579.472865,
		WakeupPathQueryWindowEndTs:   34579.587805,
		WindowStartTs:                34579.472865,
		WindowEndTs:                  34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E5 shape: the resolved trunk hop copy (branch 1 = elected).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e5",
				Subject: "threadpoolforeg-60555", Predicate: "wakeup_causal_impact",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
				ChainRelevance: "on_chain", ChainDepth: 1, ChainBranch: 1,
				ImpactMS: 17.442, CumulativeImpactMS: 17.442,
				MergedCount: 3, MergedMinMS: 4.426, MergedMaxMS: 6.768,
				EffectiveImpactMS: 6.936, EffectiveImpactPublished: true,
				QueryWindowStartTs: 34579.472865, QueryWindowEndTs: 34579.587805,
				Confidence: 0.8, LineStart: 8712, LineEnd: 15131},
			// E19 shape: the flat 父节点未确认 copy of the SAME ×3 aggregate
			// (foreign branch → never trunk-admissible), eff episode differs.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e19",
				Subject: "threadpoolforeg-60555", Predicate: "wakeup_causal_impact",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
				ChainRelevance: "on_chain", ChainDepth: 3, ChainBranch: 3,
				ImpactMS: 17.442, CumulativeImpactMS: 17.442,
				MergedCount: 3, MergedMinMS: 4.426, MergedMaxMS: 6.768,
				EffectiveImpactMS: 6.325, EffectiveImpactPublished: true,
				QueryWindowStartTs: 34579.472865, QueryWindowEndTs: 34579.587805,
				Confidence: 0.8, LineStart: 8714, LineEnd: 15120},
		},
	}
}

func TestSMR1D2BranchTwinAggregateFoldsToTrunkSeat(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1D2BranchTwinProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var seats []*runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if row.HasData && runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == "threadpoolforeg-60555" {
			seats = append(seats, row)
		}
	}
	if len(seats) != 1 {
		t.Fatalf("one physical ×3 aggregate must hold ONE seat (双 15%% bar 灭), got %d\n%s", len(seats), fence)
	}
	seat := seats[0]
	if seat.Node.EvidenceID != "e5" {
		t.Fatalf("the trunk copy keeps the seat (已解析链位信息严格更多), got %s", seat.Node.EvidenceID)
	}
	if len(seat.BranchTwinFoldPeers) != 1 || seat.BranchTwinFoldPeers[0].EffectiveImpactMS != 6.325 {
		t.Fatalf("D4: the folded copy's eff caliber must dual-list verbatim: %+v", seat.BranchTwinFoldPeers)
	}
	if !strings.Contains(fence, "同源聚合已并入[") || !strings.Contains(fence, "6.325ms 与本行分列") {
		t.Fatalf("行2 must disclose the fold + the dual-listed eff:\n%s", fence)
	}
	if ref := runtimeTraceProjCauseEvidenceRef(*seat); !strings.Contains(ref, "+") {
		t.Fatalf("零静默消失: the folded copy's E# must join the bracket, got %q", ref)
	}
}

// ⊂ variants (non-equal extrema) stay two rows — 泛化臂无 typed inventory
// 不可证, 留 v5 P1.
func TestSMR1D2SubsetVariantStaysTwoRows(t *testing.T) {
	projection := smr1D2BranchTwinProjection()
	projection.OnChainCauses[1].MergedMaxMS = 6.768 - 0.5
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for i := range model.TreeRows {
		if model.TreeRows[i].HasData &&
			runtimeTraceCausalProjectionCanonicalNode(model.TreeRows[i].Node.Subject) == "threadpoolforeg-60555" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("non-identical fingerprints never fold (精确信号才可硬折), got %d", count)
	}
}

// --- WO-D3 短期臂: double-merged twin mutual tags (S3-TPF 42729 E9/E15) ------

func smr1D3TwinProjection() types.TraceCausalProjection {
	node := func(id, token string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: id,
			Subject: "threadpoolforeg-60555", Predicate: "critical_blocking",
			Object: "unknown-thread", TypeToken: token, StateKind: "io_wait",
			ChainRelevance: "on_chain",
			ImpactMS:       13.418, CumulativeImpactMS: 13.418,
			MergedCount: 3, MergedMinMS: 4.265, MergedMaxMS: 4.884,
			Confidence: 0.8, LineStart: 8712, LineEnd: 15131,
		}
	}
	a := node("e9", "")
	b := node("e15", "d_state_or_io_wait")
	b.LineStart, b.LineEnd = 8714, 15120
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop",
				Subject: "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 20.0, CumulativeImpactMS: 20.0,
				Confidence: 0.8, LineStart: 50, LineEnd: 60},
			a, b,
		},
	}
}

// 若双行则必互指 (run 依赖间歇形容忍单行): when both merged twins render, they
// must cross-reference each other.
func TestSMR1D3DoubleMergedTwinsMutualTags(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1D3TwinProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var rows []*runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if row.HasData && (row.Node.EvidenceID == "e9" || row.Node.EvidenceID == "e15") {
			rows = append(rows, row)
		}
	}
	if len(rows) == 1 {
		return // the V4 root arm folded the pair pre-merge — nothing to mirror
	}
	if len(rows) != 2 {
		t.Fatalf("twin fixture must render 1 (root-folded) or 2 rows, got %d", len(rows))
	}
	if rows[0].MergedTwinMirrorRef != strings.TrimSpace(rows[1].EvidenceTag) ||
		rows[1].MergedTwinMirrorRef != strings.TrimSpace(rows[0].EvidenceTag) {
		t.Fatalf("双合并行互指形: both rows must cross-reference, got %q / %q",
			rows[0].MergedTwinMirrorRef, rows[1].MergedTwinMirrorRef)
	}
	if !strings.Contains(fence, "同段镜像·与[") || !strings.Contains(fence, "]同源,不可相加") {
		t.Fatalf("行2 must wear the mutual mirror tag:\n%s", fence)
	}
}

// --- WO-C1: account-relation sentence (SMR-S15 45701 E14/E25) ----------------

func smr1C1AccountPairProjection() types.TraceCausalProjection {
	// The pinned W-A pair VERBATIM (TestCR2FixRankChainCumDivergenceStaysTwoRows
	// fixture — the two-row survival is that test's pin; THIS test pins the
	// missing reason half).
	return types.TraceCausalProjection{
		WakeupPath:    []string{"keva-1-17437", "aweme-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-chain",
				Subject: "keva-1-17437", Object: "runnable", StateKind: "runnable",
				Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
				Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 2.181, CumulativeImpactMS: 2.181, EffectiveImpactMS: 3.399,
				MergedCount: 2, MergedMinMS: 1.218, MergedMaxMS: 2.181,
				PriorityInversionCandidate: true,
				Confidence:                 0.78, LineStart: 20817, LineEnd: 21412},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-rank",
				Subject: "keva-1-17437", Object: "runnable_wait", TypeToken: "runnable_wait",
				StateKind: "runnable", Predicate: "root_cause_secondary", Rank: 5, Tier: "secondary",
				ImpactMS: 3.399, CumulativeImpactMS: 4.710, EffectiveImpactMS: 3.399,
				PriorityInversionCandidate: true,
				ChainRelevance:             "on_chain", Confidence: 0.91, LineStart: 20817, LineEnd: 21412},
		},
	}
}

func TestSMR1C1AccountRelationSentenceOnWATwinPair(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1C1AccountPairProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	count := 0
	related := 0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if !row.HasData || runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != "keva-1-17437" {
			continue
		}
		count++
		if row.AccountRelRef != "" {
			related++
		}
	}
	// W-A 双行存续 (the pinned count==2 stays) + 理由句补齐 (the new half).
	if count != 2 {
		t.Fatalf("W-A 不同账目绝不折: both rows must stay seated, got %d", count)
	}
	if related != 2 {
		t.Fatalf("both rows must carry the account-relation sentence, got %d", related)
	}
	if !strings.Contains(fence, "账目关系(见图例):本行=") {
		t.Fatalf("the sentence must render on 行2:\n%s", fence)
	}
	// 三禁令 (S6 vnote): the sentence itself must not mint the banned faces.
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "账目关系(见图例)") {
			continue
		}
		if strings.Contains(line, "同段") {
			t.Fatalf("禁「同段」字面 in the account sentence:\n%s", line)
		}
		if strings.Contains(line, "全窗") {
			t.Fatalf("禁覆盖方向暗示 (全窗 vs 发生段 pairing) in the account sentence:\n%s", line)
		}
		if strings.Contains(line, "重叠 ") && strings.Contains(line, "ms") && strings.Contains(line, "重叠约") {
			t.Fatalf("禁量化重叠 ms in the account sentence:\n%s", line)
		}
	}
}

// --- WO-B1: occurrence-series note (SMR-S8/S10/S11) --------------------------

func smr1B1OccurrenceProjection() types.TraceCausalProjection {
	node := func(id string, impact, startTs, endTs float64, lineStart, lineEnd int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: id,
			Subject: "app-9511", Predicate: "wakeup_causal_impact",
			Object: "sleep_wait", TypeToken: "sleep_wait", StateKind: "s_sleep",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: impact, CumulativeImpactMS: impact,
			StartTs: startTs, EndTs: endTs,
			Confidence: 0.8, LineStart: lineStart, LineEnd: lineEnd,
		}
	}
	// 31552 E5/E10 shape: two disjoint occurrences 15.565 + 5.251 = 20.816.
	a := node("e5", 15.565, 13762.900000, 13762.915565, 4000, 4400)
	b := node("e10", 5.251, 13762.950000, 13762.955251, 8000, 8300)
	b.ChainDepth = 1
	b.ChainBranch = 7 // 父节点未确认 lane — the seat is honest and stays
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{a, b},
	}
}

func TestSMR1B1DisjointOccurrencesIntervalNoteAndTotal(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1B1OccurrenceProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	count := 0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if !row.HasData || runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != "app-9511" {
			continue
		}
		count++
		if len(row.OccurrenceSeriesRefs) != 1 || row.OccurrenceSeriesCount != 2 {
			t.Fatalf("each occurrence must reference its sibling: %+v", row.OccurrenceSeriesRefs)
		}
		if row.OccurrenceSeriesTotalMS != 15.565+5.251 {
			t.Fatalf("the series total is the disjoint sum, got %.3f", row.OccurrenceSeriesTotalMS)
		}
	}
	// 护栏③ 改词保双席: repair_mid 已见丢席形 — both seats MUST survive.
	if count != 2 {
		t.Fatalf("改词保双席: got %d seats", count)
	}
	if !strings.Contains(fence, "不相交(共2段,合计 20.816ms)") {
		t.Fatalf("the series note must render with the additive total:\n%s", fence)
	}
	// WF-xn B1 (§29.52.1 时间区间词面族定形): the occurrence-segment
	// timestamps use the unified interval tilde — the en-dash form (misread
	// as minus in arithmetic-dense reports) is retired from the time family.
	if !strings.Contains(fence, "s~") || strings.Contains(fence, "s–") {
		t.Fatalf("发生段 timestamps must use the tilde interval form:\n%s", fence)
	}
}

// Overlapping same-identity occurrences are NOT the B shape — no note.
func TestSMR1B1OverlappingOccurrencesFailOpen(t *testing.T) {
	projection := smr1B1OccurrenceProjection()
	projection.OnChainCauses[1].StartTs = 13762.910000 // overlaps E5
	projection.OnChainCauses[1].EndTs = 13762.960000
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if len(model.TreeRows[i].OccurrenceSeriesRefs) > 0 {
			t.Fatalf("overlapping intervals must fail open (不相交才可加): %+v", model.TreeRows[i].Node.EvidenceID)
		}
	}
}

// --- WO-C1 pair class (2): family window seat ↔ chain rank seat (SMR-S6 /
// S1-TPF 过渡句, 56643 E12/E15 · 31693 E11/E14) --------------------------------

func smr1C1FamilyChainProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop",
				Subject: "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 20.0, CumulativeImpactMS: 20.0,
				Confidence: 0.8, LineStart: 50, LineEnd: 60},
			// E15 shape: the ➎ D/IO family seat (window_stats 互斥账, ×4 成员).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e15",
				Subject: "threadpoolforeg-60555", Predicate: "root_cause_secondary",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
				ChainRelevance: "on_chain", Rank: 5, Tier: "secondary",
				ImpactMS: 15.317, CumulativeImpactMS: 15.317,
				FamilyMemberCount: 4, FamilyMemberMinMS: 4.265, FamilyMemberMaxMS: 4.884,
				FamilyFoldCaliber: "sum_disjoint",
				Confidence:        0.8, LineStart: 4600, LineEnd: 15029},
			// E12 shape: the ➍ io_wait chain rank seat (链上聚合账).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e12",
				Subject: "threadpoolforeg-60555", Predicate: "root_cause_tertiary",
				Object: "io_wait", TypeToken: "io_wait", StateKind: "io_wait",
				ChainRelevance: "on_chain", Rank: 4, Tier: "tertiary",
				ImpactMS: 17.819, CumulativeImpactMS: 24.951,
				Confidence: 0.82, LineStart: 8712, LineEnd: 15140},
		},
	}
}

func TestSMR1C1FamilyChainAccountSentence(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1C1FamilyChainProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var family, chain *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		switch row.Node.EvidenceID {
		case "e15":
			family = row
		case "e12":
			chain = row
		}
	}
	if family == nil || chain == nil {
		t.Fatalf("W-A 双行存续: both accounting systems keep their seats\n%s", fence)
	}
	if family.AccountRelRef != strings.TrimSpace(chain.EvidenceTag) ||
		chain.AccountRelRef != strings.TrimSpace(family.EvidenceTag) {
		t.Fatalf("the two account systems must cross-reference: %q / %q",
			family.AccountRelRef, chain.AccountRelRef)
	}
	if !strings.Contains(family.AccountRelOwn, "按状态类互斥归账") ||
		!strings.Contains(family.AccountRelPeer, "按链上聚合归账") {
		t.Fatalf("caliber self-descriptions must ride the typed selector: own=%q peer=%q",
			family.AccountRelOwn, family.AccountRelPeer)
	}
	if !strings.Contains(fence, "账目关系(见图例):本行=") {
		t.Fatalf("the sentence must render:\n%s", fence)
	}
}

// 96879 复放追修 pin: the root_evidence binder rows carry the typed token on
// the Predicate lane — the component pointer must fire on them too.
func TestSMR1A1SelfBinderPredicateLane(t *testing.T) {
	projection := smr1A1SelfBinderProjection()
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].TypeToken == "binder_wait" {
			projection.OnChainCauses[i].TypeToken = ""
			projection.OnChainCauses[i].Predicate = "binder_wait"
			projection.OnChainCauses[i].EvidenceID = "trace_query:t#root_evidence:" +
				projection.OnChainCauses[i].EvidenceID
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tagged := 0
	for _, row := range model.SelfRows {
		if row.NonAdditiveKind == runtimeTraceProjNonAdditiveComponent {
			tagged++
		}
	}
	if tagged == 0 {
		t.Fatalf("Predicate-lane binder rows must carry the component pointer (96879 形)")
	}
}

// 96879 复放追修 pin (E29 形): a pool fold row whose headline µs-equals a
// rendered occurrence series' additive total wears the multi-ref mirror tag.
func TestSMR1D1PoolHeadlineSeriesMirror(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1B1OccurrenceProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// Hand-mint the pool row shape (the caps in unit fixtures never overflow):
	// headline 20.816 = the two occurrence rows' series total.
	model.TreeRows = append(model.TreeRows, runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowDepthless, HasData: true, EvidenceTag: "E29(+5)",
		Node: types.TraceCausalProjectionNode{
			Subject: "app-9511、TimerDispatch-9510 等", OnChainOverflowFold: true,
			MergedCount: 6, MergedSubjects: []string{"app-9511", "TimerDispatch-9510"},
			ImpactMS: 20.816, CumulativeImpactMS: 20.816,
		},
	})
	runtimeTraceProjStampOverflowSeriesMirrors(&model)
	fold := &model.TreeRows[len(model.TreeRows)-1]
	if len(fold.OverflowMirrorRefs) != 2 {
		t.Fatalf("headline=series-total pool rows must carry the multi-ref mirror, got %+v", fold.OverflowMirrorRefs)
	}
}

// 5200 复放追修 pin: a cadence-idle row (s_sleep StateKind, pacing_idle token)
// beside the sleep seat must not turn the seat census ambiguous — the binder
// pointer still fires.
func TestSMR1A1SelfBinderIdleRowDoesNotBlockSeatCensus(t *testing.T) {
	projection := smr1A1SelfBinderProjection()
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-idle",
		Subject: ".ugc.aweme.lite-17267", Object: "pacing_idle", TypeToken: "pacing_idle",
		StateKind: "s_sleep", ChainRelevance: "on_chain",
		ImpactMS: 15.758, CumulativeImpactMS: 15.758,
		Confidence: 0.8, LineStart: 250, LineEnd: 260,
	})
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tagged := 0
	for _, row := range model.SelfRows {
		if row.NonAdditiveKind == runtimeTraceProjNonAdditiveComponent {
			tagged++
		}
	}
	if tagged == 0 {
		t.Fatalf("the idle row must not block the sleep-seat census (5200 形)")
	}
}

// --- 修复轮三 R2-F2: derived overlap/disjoint claim (tieba E4↔E25 冷读
// witness, 2026-07-13): the pair-class-(2) sentence's 「物理时间重叠」 was an
// unconditional template — for a partition-sibling pair whose typed hulls
// are disjoint (the tieba remainder [34579.531..586] vs the hmfs_get_dnode
// seat [34579.4869..4872]) the claim was FALSE. The claim is now DERIVED:
// provably-disjoint hulls speak 「物理时间不相交」 (no additive invitation,
// account self-identifications preserved); overlap/missing-ts pairs keep
// the existing overlap template verbatim (fail-open — hull overlap cannot
// prove member overlap; 禁量化重叠 ms unchanged).
// MUTATION self-check: dropping the derivation (unconditional template)
// reds TestSMR1C1DisjointPairSpeaksDisjointWord; loosening it to fire on
// overlapping hulls reds TestSMR1C1FamilyChainAccountSentenceKeepsOverlapWord.

func smr1C1DisjointPairProjection() types.TraceCausalProjection {
	projection := smr1C1FamilyChainProjection()
	for i := range projection.OnChainCauses {
		node := &projection.OnChainCauses[i]
		switch node.EvidenceID {
		case "e15":
			// The tieba E4 remainder shape: 3-member family seat, late hull.
			node.ImpactMS, node.CumulativeImpactMS = 10.433, 10.433
			node.FamilyMemberCount = 3
			node.StartTs, node.EndTs = 34579.531242, 34579.585906
		case "e12":
			// The tieba E25 shape: single proven-cause seat, early hull —
			// provably disjoint from the remainder's hull.
			node.ImpactMS, node.CumulativeImpactMS = 0.171, 0.171
			node.StartTs, node.EndTs = 34579.486987, 34579.487158
		}
	}
	return projection
}

func TestSMR1C1DisjointPairSpeaksDisjointWord(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1C1DisjointPairProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var family *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e15" {
			family = &model.TreeRows[i]
		}
	}
	if family == nil || family.AccountRelRef == "" {
		t.Fatalf("the account pair must still mint (改词保双席):\n%s", fence)
	}
	if !family.AccountRelDisjoint {
		t.Fatalf("typed disjoint hulls must derive the disjoint verdict: %+v", family.Node)
	}
	if !strings.Contains(fence, "物理时间不相交·账目关系(见图例):本行=") {
		t.Fatalf("the disjoint pair must speak 不相交 (never the overlap template):\n%s", fence)
	}
	if strings.Contains(fence, "物理时间重叠(不可相加)·账目关系(见图例):本行=按状态类互斥归账") {
		t.Fatalf("the false overlap claim must not survive on the disjoint pair:\n%s", fence)
	}
}

// The ts-less legacy pair (the original fixture) keeps the overlap template
// verbatim — absence of interval identity never claims disjointness.
func TestSMR1C1FamilyChainAccountSentenceKeepsOverlapWord(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1C1FamilyChainProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "物理时间重叠(不可相加)·账目关系(见图例):本行=") {
		t.Fatalf("the unprovable pair keeps the overlap template:\n%s", fence)
	}
	if strings.Contains(fence, "物理时间不相交") {
		t.Fatalf("no disjoint claim may mint without typed proof:\n%s", fence)
	}
}
