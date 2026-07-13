package tool

// answer_document_projection_smr1_repair_test.go — SMR-1 修复轮 pins
// (coordinator SHIP-WITH-FIXES 工单 + 冷读 coldread_smr1_report.md,
// 2026-07-13): P2-2 跨口径穿透, P2-3 必要性否决, 冷读扩臂①②④⑤, P3 守卫独立
// pin 群.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- 冷读① F-1: series aggregate seat (E13 ↔ E7+E8) --------------------------

func smr1RepairSeriesSeatProjection() types.TraceCausalProjection {
	projection := smr1B1OccurrenceProjection()
	// The rank seat whose display IS the series total (14.597-form → here
	// 20.816 = 15.565 + 5.251).
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e13-seat",
		Subject: "app-9511", Predicate: "root_cause_secondary",
		Object: "runnable_wait", TypeToken: "priority_inversion_runnable_wait", StateKind: "s_sleep",
		ChainRelevance: "on_chain", Rank: 6, Tier: "secondary",
		ImpactMS: 20.816, CumulativeImpactMS: 21.100,
		Confidence: 0.9, LineStart: 9000, LineEnd: 9400,
	})
	return projection
}

func TestSMR1RepairSeriesAggregateSeatPointers(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1RepairSeriesSeatProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var seat *runtimeTraceProjTreeRow
	components := 0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		switch {
		case row.Node.EvidenceID == "e13-seat":
			seat = row
		case row.OccurrenceSeriesCount == 2:
			if row.NonAdditiveKind != runtimeTraceProjNonAdditiveComponent {
				t.Fatalf("series rows must point 组成部分 at the aggregate seat: %+v", row.Node.EvidenceID)
			}
			components++
		}
	}
	if seat == nil || components != 2 {
		t.Fatalf("fixture shape drifted: seat=%v components=%d", seat != nil, components)
	}
	if seat.NonAdditiveKind != runtimeTraceProjNonAdditiveContains ||
		!strings.Contains(seat.NonAdditiveRef, "]+[") {
		t.Fatalf("the aggregate seat wears the multi-ref 已含 face, got kind=%v ref=%q",
			seat.NonAdditiveKind, seat.NonAdditiveRef)
	}
}

// --- 冷读② F-2: family-difference identity (E9 ↔ E14) ------------------------

func smr1RepairFamilyDiffProjection() types.TraceCausalProjection {
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
			// E9 shape: ×3 critical twin (13.418 = 15.317 − 1.899; max 4.884).
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e9",
				Subject: "threadpoolforeg-60555", Predicate: "critical_blocking",
				Object: "unknown-thread", StateKind: "io_wait",
				ChainRelevance: "on_chain",
				ImpactMS:       13.418, CumulativeImpactMS: 13.418,
				MergedCount: 3, MergedMinMS: 4.265, MergedMaxMS: 4.884,
				Confidence: 0.8, LineStart: 4599, LineEnd: 15028},
			// E14 shape: the ×4 D/IO family seat.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e14",
				Subject: "threadpoolforeg-60555", Predicate: "root_cause_secondary",
				Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
				ChainRelevance: "on_chain", Rank: 5, Tier: "secondary",
				ImpactMS: 15.317, CumulativeImpactMS: 15.317,
				FamilyMemberCount: 4, FamilyMemberMinMS: 1.899, FamilyMemberMaxMS: 4.884,
				FamilyFoldCaliber: "sum_disjoint",
				Confidence:        0.8, LineStart: 4599, LineEnd: 15028},
		},
	}
}

func TestSMR1RepairFamilyDifferenceIdentityPointers(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1RepairFamilyDiffProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var twin, family *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		switch model.TreeRows[i].Node.EvidenceID {
		case "e9":
			twin = &model.TreeRows[i]
		case "e14":
			family = &model.TreeRows[i]
		}
	}
	if twin == nil || family == nil {
		t.Fatalf("fixture shape drifted")
	}
	if twin.NonAdditiveKind != runtimeTraceProjNonAdditiveComponent ||
		twin.NonAdditiveRef != strings.TrimSpace(family.EvidenceTag) {
		t.Fatalf("E9 (N−1 全等形) must point 组成部分→family, got %v %q", twin.NonAdditiveKind, twin.NonAdditiveRef)
	}
	if family.NonAdditiveKind != runtimeTraceProjNonAdditiveContains {
		t.Fatalf("the family seat wears the 已含 face: %v", family.NonAdditiveKind)
	}
}

// The general ⊂ shape (identities not exact) stays untagged — CASE-1 territory.
func TestSMR1RepairFamilyDiffIdentityFailsOpenOnMismatch(t *testing.T) {
	projection := smr1RepairFamilyDiffProjection()
	projection.OnChainCauses[2].FamilyMemberMinMS = 1.500 // 15.317−1.500 ≠ 13.418
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e9" && model.TreeRows[i].NonAdditiveRef != "" {
			t.Fatalf("inexact identities never tag (⊂ 泛化留 CASE-1/v5 P1)")
		}
	}
}

// --- 冷读④ 板级警示 -----------------------------------------------------------

func TestSMR1RepairBoardOverWindowWarning(t *testing.T) {
	projection := smr1C1FamilyChainProjection()
	// Push the seats' eff Σ over the window (114.940ms window).
	projection.OnChainCauses[1].EffectiveImpactMS = 80.0
	projection.OnChainCauses[2].EffectiveImpactMS = 60.0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if model.RankBoardEffSumMS <= 0 {
		t.Fatalf("over-window board sum must mint the warning field")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "席位间物理时间可重叠,不可直接相加") {
		t.Fatalf("the board warning line must render:\n%s", lead)
	}
	// Within-window boards stay silent (byte identity).
	projection.OnChainCauses[1].EffectiveImpactMS = 10.0
	projection.OnChainCauses[2].EffectiveImpactMS = 10.0
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if model.RankBoardEffSumMS != 0 {
		t.Fatalf("within-window board must stay silent")
	}
}

// --- 冷读⑤ F-8: same-coverage pair speaks the mirror word ---------------------

func TestSMR1RepairSameCoveragePairSpeaksMirrorWord(t *testing.T) {
	projection := smr1C1AccountPairProjection()
	// Make the pair µs-display-equal over one span (the E6/E10 shape) while
	// cums still diverge.
	projection.OnChainCauses[0].ImpactMS = 19.933
	projection.OnChainCauses[0].CumulativeImpactMS = 19.933
	projection.OnChainCauses[0].MergedCount = 3
	projection.OnChainCauses[0].MergedMinMS = 3.344
	projection.OnChainCauses[0].MergedMaxMS = 8.307
	projection.OnChainCauses[1].ImpactMS = 19.933
	projection.OnChainCauses[1].CumulativeImpactMS = 21.254
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if !row.HasData || runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) != "keva-1-17437" {
			continue
		}
		count++
		if row.AccountRelRef != "" {
			t.Fatalf("同覆盖对 must NOT claim 覆盖集不同 (F-8 指错比没有更糟): %+v", row.Node.EvidenceID)
		}
		if row.ValueMirrorRef == "" && row.MergedTwinMirrorRef == "" {
			t.Fatalf("同覆盖对 must wear the mirror word: %+v", row.Node.EvidenceID)
		}
	}
	if count != 2 {
		t.Fatalf("双行存续: got %d", count)
	}
}

// --- P2-3 必要性否决 -----------------------------------------------------------

func TestSMR1RepairEnvelopeVetoBlocksOutsideMember(t *testing.T) {
	projection := smr1A1MemberProjection()
	// Move e17 OUTSIDE the aggregate's line envelope — the µs value still
	// matches a derivable member, but membership is impossible.
	projection.OnChainCauses[1].LineStart = 20000
	projection.OnChainCauses[1].LineEnd = 20400
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e17" && model.TreeRows[i].NonAdditiveRef != "" {
			t.Fatalf("P2-3: a row outside the aggregate's envelope must never wear the member pointer")
		}
	}
}

func TestSMR1RepairEnvelopeVetoBlocksOutsideRawFold(t *testing.T) {
	projection := smr1D1S2Projection()
	// Move one re-issue OUTSIDE the merged seat's envelope (value unchanged).
	projection.OnChainCauses[1].LineStart = 5000
	projection.OnChainCauses[1].LineEnd = 5050
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for i := range model.SelfRows {
		if model.SelfRows[i].HasData {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("P2-3: the out-of-envelope raw copy must keep its own seat, got %d seats", count)
	}
}

// --- P3: 96717 four-guard independent pins ------------------------------------

// Guard 1 (两把尺): a ⌗ composite-score row never enters an account pair.
func TestSMR1RepairC1CaliberSideGuard(t *testing.T) {
	projection := smr1C1AccountPairProjection()
	projection.OnChainCauses[0].TypeToken = "block_io_by_inode"
	projection.OnChainCauses[0].Object = "block_io_by_inode"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].AccountRelRef != "" {
			t.Fatalf("⌗ rows never grow account sentences (两把尺)")
		}
	}
}

// Guard 2 (同口径 skip): identical caliber self-descriptions never claim 两套账目.
func TestSMR1RepairC1IdenticalCaliberGuard(t *testing.T) {
	projection := smr1C1AccountPairProjection()
	projection.OnChainCauses[1].Rank = 0 // both sides now 发生段 caliber
	projection.OnChainCauses[1].Predicate = "wakeup_causal_impact"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		if model.TreeRows[i].AccountRelRef != "" {
			t.Fatalf("identical calibers cannot claim 两套账目 (self-contradictory sentence)")
		}
	}
}

// Guard 3 (A1-related skip): a pair already related by the containment pointer
// never double-tags with the account sentence.
func TestSMR1RepairC1A1RelatedGuard(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1A1AdditionIdentityProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if row.NonAdditiveRef != "" && row.AccountRelRef == row.NonAdditiveRef && row.AccountRelRef != "" {
			t.Fatalf("A1-related pairs never double-tag with the account sentence")
		}
	}
}

// Guard 4 (family class-1 exclusion): a family seat never enters pair class 1
// (its critical twin is CASE-1 territory).
func TestSMR1RepairC1FamilyClass1Guard(t *testing.T) {
	projection := smr1RepairFamilyDiffProjection()
	// Break the carrier-(d) identities so no pointer relates the pair; the
	// same-span cum-divergent shape would otherwise enter class 1.
	projection.OnChainCauses[2].FamilyMemberMinMS = 1.500
	projection.OnChainCauses[1].CumulativeImpactMS = 14.000
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for i := range model.TreeRows {
		row := &model.TreeRows[i]
		if row.Node.EvidenceID == "e14" && row.AccountRelRef == "" {
			continue
		}
		if row.Node.EvidenceID == "e9" && row.AccountRelRef != "" {
			// pair class 2 may still relate rank rows — but never through the
			// family's class-1 lane against its own critical twin's span.
			if row.AccountRelRef == strings.TrimSpace(model.TreeRows[i].EvidenceTag) {
				t.Fatalf("family seats route through pair class 2 only")
			}
		}
	}
}

// P2-2 谦逊注 fallback pin (29424 复放形: the WIRE-fold pool — folded_* notes,
// no Σ — whose subject also holds rendered same-family seats): the advisory
// humble note renders; no E# is named (unprovable pointer would be 指错).
func TestSMR1RepairPoolHumbleNoteOnWireFold(t *testing.T) {
	projection := smr1N1Projection()
	// A wire-fold-shaped pool row (OnChainOverflowFold via folded_* decode
	// semantics: MergedCount + extrema, no sum) whose roster head is the
	// rendered io-family subject.
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "wirefold",
		Subject: "threadpoolforeg-60555 等", Predicate: "io_wait", Object: "io_wait",
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		MergedCount: 3, MergedMinMS: 4.558, MergedMaxMS: 6.936,
		MergedSubjects: []string{"threadpoolforeg-60555"},
		ImpactMS:       6.936, CumulativeImpactMS: 6.936,
		Confidence: 0.88, LineStart: 8712, LineEnd: 15131,
	})
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	found := false
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			if rows[i].Node.EvidenceID == "wirefold" {
				found = true
				if !rows[i].OverflowProjectionHumble {
					t.Fatalf("the ref-less wire-fold pool must wear the humble note")
				}
			}
		}
	}
	if !found {
		t.Fatalf("wire-fold row missing from the render")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "墙钟可能重叠,勿与之直加") {
		t.Fatalf("humble note must render:\n%s", fence)
	}
}
