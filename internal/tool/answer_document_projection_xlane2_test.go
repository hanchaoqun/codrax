package tool

// answer_document_projection_xlane2_test.go — XLANE-2 件1 pins (§29.104.1
// 立案 / §29.104.2 定谳④, customer witness runnable2.txt, 2026-07-17): the
// semantic family member-subset demotion.
//
// Witness geometry: E34 (类校验 8次 9.586ms, 根因排序#1, SELF-SEM lane) =
// E35 (4次, on-chain overlap lane, seatless) ∪ E49 (4次, non-chain lane) —
// the same physical spans double-minted across query steps through the three
// semantic lanes; E35's members are verbatim inside E34's. Post-fix: each
// subset seat wears 不可相加·为[E34]成员子集(整席降道), leaves the ◎
// population/census into the dedicated footnote, and every value stays
// byte-identical (降道=席位口径变化非值变化).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// xlane2SemanticFamilyNode builds one semantic family seat in the exact
// production shape (rank_family_fold mint: trace_semantic_span predicate,
// FamilyMember* lane, complete typed line ranges).
func xlane2SemanticFamilyNode(id string, count int, lineRanges [][2]int, eff, cum float64) types.TraceCausalProjectionNode {
	envStart, envEnd := lineRanges[0][0], lineRanges[0][1]
	for _, r := range lineRanges {
		if r[0] < envStart {
			envStart = r[0]
		}
		if r[1] > envEnd {
			envEnd = r[1]
		}
	}
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: id,
		Subject: "ease.cloudmusic-63993", Predicate: "trace_semantic_span",
		Object: "class_verification", SemanticClass: "class_verification",
		SpanName: "VerifyClass org.chromium.android_webview.AwContents",
		ImpactMS: cum, CumulativeImpactMS: cum, EffectiveImpactMS: eff,
		FamilyMemberCount: count, FamilyMemberMaxMS: 2.762, FamilyMemberMinMS: 0.116,
		FamilyFoldCaliber:      "interval_union",
		FamilyMemberRoster:     []string{"VerifyClass org.chromium.android_webview.AwContents 2.762ms"},
		FamilyMemberLineRanges: lineRanges,
		LineStart:              envStart, LineEnd: envEnd, Confidence: 0.7,
	}
}

// xlane2MemberSubsetProjection is the runnable2 E34/E35/E49 witness shape.
func xlane2MemberSubsetProjection() types.TraceCausalProjection {
	e34Ranges := [][2]int{{100, 110}, {120, 130}, {140, 150}, {160, 170}, {180, 190}, {200, 210}, {220, 230}, {240, 250}}
	e34 := xlane2SemanticFamilyNode("xlane2-e34", 8, e34Ranges, 9.586, 9.586)
	e34.ChainRelevance = "on_chain"
	e34.Causality = "self_deterministic"
	e34.OnChainBasis = "self_deterministic_span"
	e34.Rank = 1
	e34.Tier = "primary"
	// E35: the overlap-lane re-mint of E34's first four members (seatless —
	// 未入根因排序; the ⛓ semantic census form).
	e35 := xlane2SemanticFamilyNode("xlane2-e35", 4, e34Ranges[:4], 6.182, 6.376)
	e35.ChainRelevance = "on_chain"
	e35.Causality = "on_wakeup_chain"
	// E49: the non-chain lane re-mint of the other four members (◇ census
	// form pre-demotion).
	e49 := xlane2SemanticFamilyNode("xlane2-e49", 4, e34Ranges[4:], 3.210, 3.210)
	e49.ChainRelevance = "adjacent"
	e49.Causality = "adjacent_to_wakeup_chain"
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs:           17729.471126,
		WindowEndTs:             17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("xlane2-dep", "shadowhook-task-64305", "runnable_wait", "runnable", 2, 8.608, 300),
			// A D-state chain row keeps the ⛓ glyph's icon legend lit beside
			// the ◎ channel word (the representative-fixture convention).
			elimChainNode("xlane2-dio", "workSharkThread-64796", "d_state_or_io_wait", "d_sleep", 3, 5.368, 320),
		},
		SemanticSpans: []types.TraceCausalProjectionNode{e34, e35, e49},
	}
}

// xlane2FindRowByEvidenceID returns the model row carrying the evidence id.
func xlane2FindRowByEvidenceID(model *runtimeTraceProjTreeModel, id string) *runtimeTraceProjTreeRow {
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			if rows[i].Node.EvidenceID == id {
				return &rows[i]
			}
		}
	}
	return nil
}

// 件1 core (病形红→修后绿): both subset seats demote toward the union seat —
// the 行2 pointer names E34's tag on each, E34 itself stays pointer-free, and
// every published value stays on the fence (值通道零动).
func TestXLANE2MemberSubsetDemotesAndPointsAtUnionSeat(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(xlane2MemberSubsetProjection(),
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
			e34 := xlane2FindRowByEvidenceID(&model, "xlane2-e34")
			e35 := xlane2FindRowByEvidenceID(&model, "xlane2-e35")
			e49 := xlane2FindRowByEvidenceID(&model, "xlane2-e49")
			if e34 == nil || e35 == nil || e49 == nil {
				t.Fatalf("fixture drifted: all three semantic seats must render as rows")
			}
			superTag := strings.TrimSpace(e34.EvidenceTag)
			if superTag == "" {
				t.Fatalf("fixture drifted: the union seat must carry an evidence tag")
			}
			for _, sub := range []*runtimeTraceProjTreeRow{e35, e49} {
				if sub.SemanticMemberSubsetOf != superTag ||
					sub.NonAdditiveKind != runtimeTraceProjNonAdditiveMemberSubset ||
					sub.NonAdditiveRef != superTag {
					t.Fatalf("件1: the subset seat %s must demote toward the union seat %q: %+v",
						sub.Node.EvidenceID, superTag, sub.SemanticMemberSubsetOf)
				}
			}
			if e34.SemanticMemberSubsetOf != "" || e34.NonAdditiveKind == runtimeTraceProjNonAdditiveMemberSubset {
				t.Fatalf("件1: the union seat must stay undemoted")
			}
			word := "为[" + superTag + "]成员子集(整席降道)"
			if !zh {
				word = "member subset of [" + superTag + "] (whole-seat demotion)"
			}
			if n := strings.Count(fence, word); n != 2 {
				t.Fatalf("件1: exactly the two subset seats must wear the pointer word (got %d):\n%s", n, fence)
			}
			if !model.Marks.has(runtimeTraceProjMarkSemanticMemberSubset) {
				t.Fatalf("件1: the dedicated legend mark must record at the emission site")
			}
			// 值通道零动: every published value stays.
			for _, value := range []string{"9.586", "6.376", "3.210"} {
				if !strings.Contains(fence, value) {
					t.Fatalf("件1: value %s must stay untouched on the fence:\n%s", value, fence)
				}
			}
		})
	}
}

// 件1 ◎ face: the subset seats leave the semantic census into the dedicated
// footnote; the footnote count equals the excluded census (closure identity —
// the represented-lane precedent extended to the subset lane).
func TestXLANE2MemberSubsetElimFootnoteClosure(t *testing.T) {
	model, fence := elimRenderOverview(t, xlane2MemberSubsetProjection(), true)
	census := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if !runtimeTraceProjElimMemberSubsetExcluded(rows[i]) {
				continue
			}
			census++
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" && !strings.Contains(fence, "["+tag+"]") {
				t.Fatalf("subset lane: excluded row [%s] must be named in the disclosure footnote:\n%s", tag, fence)
			}
		}
	}
	if census != 2 {
		t.Fatalf("fixture drifted: exactly the two subset seats must be census-excluded, got %d:\n%s", census, fence)
	}
	disclosed := -1
	for _, line := range strings.Split(fence, "\n") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "· 为语义席成员子集(降道):%d 行,见明细", &n); err == nil {
			disclosed = n
		}
	}
	if disclosed != census {
		t.Fatalf("subset lane: footnote count %d != excluded census %d (closure identity):\n%s", disclosed, census, fence)
	}
	// The demoted seats must not ALSO sit inside the semantic census footnote
	// counts (no double presence): with both class_verification census rows
	// demoted, no ⛓/◇ 语义 census line may remain for them.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "语义") && strings.Contains(line, "xlane2-e35") {
			t.Fatalf("subset lane: a demoted seat must not stay in the semantic census line: %q", line)
		}
	}
}

// xlane2AssertSubsetUntouched asserts a whole render stays byte-free of the
// 件1 subset lane: zero subset words on the tree fence, mark unlit, zero ◎
// footnote (fail-open 保原状 / 宁漏勿假指 shared assertion body).
func xlane2AssertSubsetUntouched(t *testing.T, projection types.TraceCausalProjection) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "成员子集") {
		t.Fatalf("负向臂: no subset word may render:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkSemanticMemberSubset) {
		t.Fatalf("负向臂: the subset mark must stay unlit")
	}
	elim := runtimeTraceProjElimOverviewFence(projection, model, true)
	if strings.Contains(elim, "为语义席成员子集") {
		t.Fatalf("负向臂: no subset footnote may render:\n%s", elim)
	}
}

// 件1 negative arms: no typed line sets / partial overlap / equal sets /
// provably different boards — the seats stay untouched (fail-open 保原状,
// zero subset words, zero footnote).
func TestXLANE2MemberSubsetNegativeArms(t *testing.T) {
	assertUntouched := xlane2AssertSubsetUntouched
	t.Run("no line sets (legacy)", func(t *testing.T) {
		projection := xlane2MemberSubsetProjection()
		for i := range projection.SemanticSpans {
			projection.SemanticSpans[i].FamilyMemberLineRanges = nil
		}
		assertUntouched(t, projection)
	})
	t.Run("partial overlap is not a subset", func(t *testing.T) {
		projection := xlane2MemberSubsetProjection()
		// E35 keeps three shared ranges and one range outside E34's set.
		projection.SemanticSpans[1].FamilyMemberLineRanges = [][2]int{{100, 110}, {120, 130}, {140, 150}, {900, 910}}
		projection.SemanticSpans[2].FamilyMemberLineRanges = [][2]int{{920, 930}, {940, 950}, {960, 970}, {980, 990}}
		assertUntouched(t, projection)
	})
	t.Run("equal sets never demote", func(t *testing.T) {
		projection := xlane2MemberSubsetProjection()
		projection.SemanticSpans = projection.SemanticSpans[:2]
		projection.SemanticSpans[1].FamilyMemberCount = 8
		projection.SemanticSpans[1].FamilyMemberLineRanges = projection.SemanticSpans[0].FamilyMemberLineRanges
		assertUntouched(t, projection)
	})
	t.Run("provably different boards never compare", func(t *testing.T) {
		projection := xlane2MemberSubsetProjection()
		projection.SemanticSpans[0].RankBoardTarget = "ease.cloudmusic-63993"
		for i := 1; i < len(projection.SemanticSpans); i++ {
			projection.SemanticSpans[i].RankBoardTarget = "shadowhook-task-64305"
		}
		assertUntouched(t, projection)
	})
}

// xlane2SelfGapOverlapProjection is the 件2 display fixture (裁定④): the
// self-gap seat (production rank shape: self_wall_clock basis, deficit
// disclosure) carrying two typed overlap entries whose line envelopes match
// the target's own semantic seats.
func xlane2SelfGapOverlapProjection() types.TraceCausalProjection {
	selfGap := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xlane2-selfgap",
		Subject: "ease.cloudmusic-63993", Predicate: "root_cause_secondary",
		Object: "running", TypeToken: "running", StateKind: "running",
		ChainRelevance: "on_chain", Causality: "self_wall_clock",
		OnChainBasis: "self_wall_clock_interval",
		ImpactMS:     70.0, CumulativeImpactMS: 70.0, EffectiveImpactMS: 20.0,
		Rank: 2, Tier: "secondary", Confidence: 0.86,
		LineStart: 1, LineEnd: 500,
		SelfGapSemanticOverlaps: []types.TraceCausalProjectionSelfGapSemanticOverlap{
			{OverlapMS: 15.0, LineStart: 600, LineEnd: 620},
			{OverlapMS: 5.0, LineStart: 700, LineEnd: 705},
		},
	}
	family := xlane2SemanticFamilyNode("xlane2-sem-fam", 2, [][2]int{{600, 610}, {612, 620}}, 15.0, 15.0)
	family.ChainRelevance = "on_chain"
	family.Causality = "self_deterministic"
	family.OnChainBasis = "self_deterministic_span"
	family.Rank = 1
	family.Tier = "primary"
	family.LineStart, family.LineEnd = 600, 620
	single := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "xlane2-sem-one",
		Subject: "ease.cloudmusic-63993", Predicate: "trace_semantic_span",
		Object: "class_verification", SemanticClass: "class_verification",
		SpanName:       "VerifyClass org.chromium.ui.base.ViewAndroidDelegate",
		ChainRelevance: "on_chain", Causality: "self_deterministic",
		OnChainBasis: "self_deterministic_span",
		ImpactMS:     5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
		LineStart: 700, LineEnd: 705, Confidence: 0.7,
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs:           17729.471126,
		WindowEndTs:             17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{
			selfGap,
			elimChainNode("xlane2-dio2", "workSharkThread-64796", "d_state_or_io_wait", "d_sleep", 3, 5.368, 800),
		},
		SemanticSpans: []types.TraceCausalProjectionNode{family, single},
	}
}

// 件2 core: the self-gap row renders the per-partner overlap clauses with
// resolved [E#] pointers (typed line-envelope identity), values untouched.
func TestXLANE2SelfGapOverlapClauseRenders(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(xlane2SelfGapOverlapProjection(),
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
			famTag := strings.TrimSpace(xlane2FindRowByEvidenceID(&model, "xlane2-sem-fam").EvidenceTag)
			oneTag := strings.TrimSpace(xlane2FindRowByEvidenceID(&model, "xlane2-sem-one").EvidenceTag)
			if famTag == "" || oneTag == "" {
				t.Fatalf("fixture drifted: both semantic partners must carry tags")
			}
			line := "其中 15.000ms 与语义席[" + famTag + "]重叠、5.000ms 与语义席[" + oneTag + "]重叠"
			if !zh {
				line = "of which 15.000ms overlaps semantic seat [" + famTag + "]; 5.000ms overlaps semantic seat [" + oneTag + "]"
			}
			if !strings.Contains(fence, line) {
				t.Fatalf("件2: the overlap clause line must render with resolved [E#]s (%q):\n%s", line, fence)
			}
			if !model.Marks.has(runtimeTraceProjMarkSelfGapSemanticOverlap) {
				t.Fatalf("件2: the legend mark must record at the emission site")
			}
			// 主值零动: the seat's published values stay.
			for _, value := range []string{"70.000", "20.000"} {
				if !strings.Contains(fence, value) {
					t.Fatalf("件2: value %s must stay untouched:\n%s", value, fence)
				}
			}
		})
	}
}

// 件2 negative arms: no typed overlaps → zero bytes; an unresolvable partner
// drops its clause only (宁漏勿假指), the resolvable sibling keeps its clause.
func TestXLANE2SelfGapOverlapNegativeArms(t *testing.T) {
	t.Run("no overlaps zero bytes", func(t *testing.T) {
		projection := xlane2SelfGapOverlapProjection()
		projection.OnChainCauses[0].SelfGapSemanticOverlaps = nil
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
		if strings.Contains(fence, "与语义席[") || model.Marks.has(runtimeTraceProjMarkSelfGapSemanticOverlap) {
			t.Fatalf("负向臂: no typed overlaps must render zero clause bytes:\n%s", fence)
		}
	})
	t.Run("unresolvable partner drops its clause only", func(t *testing.T) {
		projection := xlane2SelfGapOverlapProjection()
		// The single-span partner's envelope no longer matches any row.
		projection.OnChainCauses[0].SelfGapSemanticOverlaps[1].LineStart = 9000
		projection.OnChainCauses[0].SelfGapSemanticOverlaps[1].LineEnd = 9005
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
		famTag := strings.TrimSpace(xlane2FindRowByEvidenceID(&model, "xlane2-sem-fam").EvidenceTag)
		if !strings.Contains(fence, "其中 15.000ms 与语义席["+famTag+"]重叠") {
			t.Fatalf("宁漏勿假指: the resolvable clause must survive:\n%s", fence)
		}
		// Suffix-safe probes (15.000ms contains 5.000ms): the dropped clause
		// can only reappear as a list member (、5.000ms) or a lone clause head
		// (其中 5.000ms).
		if strings.Contains(fence, "、5.000ms 与语义席[") || strings.Contains(fence, "其中 5.000ms") {
			t.Fatalf("宁漏勿假指: the unresolvable clause must drop, never guess an E#:\n%s", fence)
		}
	})
}

// 件2 R2' twin pin: the projection decode cap mirrors the engine emission cap
// (two packages, one roster bound — pinned where both are visible).
func TestXLANE2SelfGapOverlapCapsMirror(t *testing.T) {
	if types.TraceCausalProjectionSelfGapSemanticOverlapCap != tracequery.SelfGapSemanticOverlapPartnerCap {
		t.Fatalf("decode cap %d != engine emission cap %d",
			types.TraceCausalProjectionSelfGapSemanticOverlapCap, tracequery.SelfGapSemanticOverlapPartnerCap)
	}
}

// 件1 incomplete-set decode belt: a line-range set whose count mismatches the
// member count never judges (the display-side arm of the all-or-nothing
// discipline; the strict decode is pinned separately in types).
func TestXLANE2IncompleteLineSetFailsOpen(t *testing.T) {
	projection := xlane2MemberSubsetProjection()
	projection.SemanticSpans[1].FamilyMemberLineRanges = projection.SemanticSpans[1].FamilyMemberLineRanges[:2]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if row := xlane2FindRowByEvidenceID(&model, "xlane2-e35"); row == nil || row.SemanticMemberSubsetOf != "" {
		t.Fatalf("行号不完整的席必须 fail-open 保原状")
	}
	// The complete sibling still judges (the arm is per-seat, never a
	// whole-report abort).
	if row := xlane2FindRowByEvidenceID(&model, "xlane2-e49"); row == nil || row.SemanticMemberSubsetOf == "" {
		t.Fatalf("完整行号集的席仍应照判")
	}
}

// --- 修补轮 (双复核后, 2026-07-17): compat 臂正向 pin / 指纹小门 / 在席臂 -----

// xlane2CompatWitnessProjection — 修补轮 件1① : the PRODUCTION witness
// emission form. Only the rank observation lane emits the board identity
// notes (trace_query.go rank-notes block: rank_board_target =
// result.RootCauseRank.Target, plus the params fingerprint); span-family
// records carry selected_window / member_line_ranges but never rank_board
// notes. The fused witness board is therefore a NAMED superset seat ×
// UNNAMED span-family subset seats — the full-triple board ids DIFFER, the
// ids-equal fast path can never pair them, and only the named/unnamed compat
// arm of runtimeTraceProjSemanticSubsetSameBoard carries the verdict.
func xlane2CompatWitnessProjection() types.TraceCausalProjection {
	projection := xlane2MemberSubsetProjection()
	for i := range projection.SemanticSpans {
		// Every step's records share the one selected stats window.
		projection.SemanticSpans[i].QueryWindowStartTs = 17729.471126
		projection.SemanticSpans[i].QueryWindowEndTs = 17729.622508
	}
	projection.SemanticSpans[0].RankBoardTarget = "ease.cloudmusic-63993"
	projection.SemanticSpans[0].RankBoardParamsFingerprint = "aaaa1111"
	return projection
}

// xlane2ModelRowsIndex renders one projection and returns the model, the row
// pointers the board index was built over, and the index itself (unit-level
// access to the 件1 same-board gate).
func xlane2ModelRowsIndex(t *testing.T, projection types.TraceCausalProjection) (*runtimeTraceProjTreeModel, map[string]*runtimeTraceProjTreeRow, *runtimeTraceProjRankBoardIndex) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	all := runtimeTraceProjSMR1AllRows(&model)
	rows := map[string]*runtimeTraceProjTreeRow{}
	for _, row := range all {
		if id := strings.TrimSpace(row.Node.EvidenceID); id != "" {
			rows[id] = row
		}
	}
	return &model, rows, runtimeTraceProjRankBoardIndexFor(all)
}

// 件1① 产线 witness 形: the named rank-lane superset seat × the unnamed
// span-family subset seats 照降 — with the load-bearing precondition pinned
// (different full-triple ids), so a dead compat arm can never hide behind the
// ids-equal path again.
func TestXLANE2CompatArmProductionWitnessForm(t *testing.T) {
	projection := xlane2CompatWitnessProjection()
	model, rows, index := xlane2ModelRowsIndex(t, projection)
	e34, e35, e49 := rows["xlane2-e34"], rows["xlane2-e35"], rows["xlane2-e49"]
	if e34 == nil || e35 == nil || e49 == nil {
		t.Fatalf("fixture drifted: all three semantic seats must render as rows")
	}
	if index.ids[e34] == index.ids[e35] || index.ids[e34] == index.ids[e49] {
		t.Fatalf("fixture drifted: named vs unnamed seats must carry DIFFERENT full-triple board ids (the compat arm must be load-bearing)")
	}
	superTag := strings.TrimSpace(e34.EvidenceTag)
	if superTag == "" {
		t.Fatalf("fixture drifted: the union seat must carry an evidence tag")
	}
	for _, sub := range []*runtimeTraceProjTreeRow{e35, e49} {
		if sub.SemanticMemberSubsetOf != superTag {
			t.Fatalf("件1① compat 臂: the unnamed subset seat %s must demote toward the named union seat %q (got %q)",
				sub.Node.EvidenceID, superTag, sub.SemanticMemberSubsetOf)
		}
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(*model, true))
	if n := strings.Count(fence, "为["+superTag+"]成员子集(整席降道)"); n != 2 {
		t.Fatalf("件1①: exactly the two subset seats must wear the pointer word (got %d):\n%s", n, fence)
	}
}

// 件1② 歧义形: one window cluster hosting TWO named board targets — an
// identity-less span-family seat never guesses which named board it belongs
// to, so the whole hop renders zero subset bytes (宁漏勿假指).
func TestXLANE2CompatArmAmbiguousTwoNamedTargetsSkips(t *testing.T) {
	projection := xlane2CompatWitnessProjection()
	projection.OnChainCauses[0].QueryWindowStartTs = 17729.471126
	projection.OnChainCauses[0].QueryWindowEndTs = 17729.622508
	projection.OnChainCauses[0].RankBoardTarget = "shadowhook-task-64305"
	projection.OnChainCauses[0].RankBoardParamsFingerprint = "bbbb2222"
	xlane2AssertSubsetUntouched(t, projection)
}

// 件1 refuse 分支 unit pins (cold-read 点名): the compat arm's namedCount>1
// branch and its params-fork branch each refuse, and the witness pair is the
// positive control (single named target, single fingerprint → compatible).
func TestXLANE2CompatArmRefuseBranches(t *testing.T) {
	t.Run("positive control: single named target single fingerprint", func(t *testing.T) {
		_, rows, index := xlane2ModelRowsIndex(t, xlane2CompatWitnessProjection())
		if !runtimeTraceProjSemanticSubsetSameBoard(index, rows["xlane2-e35"], rows["xlane2-e34"]) {
			t.Fatalf("the witness named/unnamed pair must be board-compatible")
		}
	})
	t.Run("namedCount>1 refuses", func(t *testing.T) {
		projection := xlane2CompatWitnessProjection()
		projection.OnChainCauses[0].QueryWindowStartTs = 17729.471126
		projection.OnChainCauses[0].QueryWindowEndTs = 17729.622508
		projection.OnChainCauses[0].RankBoardTarget = "shadowhook-task-64305"
		projection.OnChainCauses[0].RankBoardParamsFingerprint = "bbbb2222"
		_, rows, index := xlane2ModelRowsIndex(t, projection)
		if runtimeTraceProjSemanticSubsetSameBoard(index, rows["xlane2-e35"], rows["xlane2-e34"]) {
			t.Fatalf("two named targets in one window: an unnamed row must never guess between them")
		}
	})
	t.Run("params fork on the named board refuses", func(t *testing.T) {
		projection := xlane2CompatWitnessProjection()
		projection.OnChainCauses[0].QueryWindowStartTs = 17729.471126
		projection.OnChainCauses[0].QueryWindowEndTs = 17729.622508
		projection.OnChainCauses[0].RankBoardTarget = "ease.cloudmusic-63993"
		projection.OnChainCauses[0].RankBoardParamsFingerprint = "cccc3333"
		_, rows, index := xlane2ModelRowsIndex(t, projection)
		if runtimeTraceProjSemanticSubsetSameBoard(index, rows["xlane2-e35"], rows["xlane2-e34"]) {
			t.Fatalf("a named board carrying ≥2 params fingerprints must refuse the unnamed pairing")
		}
		// The whole hop stays byte-free too (整跳零子集词).
		xlane2AssertSubsetUntouched(t, projection)
	})
}

// 修补轮 件5: the compat arm's fingerprint blind-spot gate — an unnamed row
// carrying its OWN params fingerprint that differs from the named board's
// single fingerprint is provably another board and skips (宁漏勿假指); an
// equal or absent fingerprint stays compatible (精确信号 only refuses on the
// positive mismatch witness). The absent arm is TestXLANE2CompatArmProduction-
// WitnessForm itself (fingerprint-less span-family rows 照降).
func TestXLANE2CompatArmFingerprintMismatchSkips(t *testing.T) {
	t.Run("fpA named × fpB unnamed skips (per-seat)", func(t *testing.T) {
		projection := xlane2CompatWitnessProjection()
		projection.SemanticSpans[1].RankBoardParamsFingerprint = "bbbb2222"
		model, rows, index := xlane2ModelRowsIndex(t, projection)
		if runtimeTraceProjSemanticSubsetSameBoard(index, rows["xlane2-e35"], rows["xlane2-e34"]) {
			t.Fatalf("件5: an unnamed row with its own mismatched fingerprint must skip")
		}
		if rows["xlane2-e35"].SemanticMemberSubsetOf != "" {
			t.Fatalf("件5: the mismatched-fingerprint seat must stay undemoted")
		}
		// The gate is per-seat precise: the fingerprint-less sibling 照降.
		superTag := strings.TrimSpace(rows["xlane2-e34"].EvidenceTag)
		if superTag == "" || rows["xlane2-e49"].SemanticMemberSubsetOf != superTag {
			t.Fatalf("件5: the fingerprint-less sibling must still demote toward %q", superTag)
		}
		_ = model
	})
	t.Run("equal fingerprints stay compatible", func(t *testing.T) {
		projection := xlane2CompatWitnessProjection()
		projection.SemanticSpans[1].RankBoardParamsFingerprint = "aaaa1111"
		_, rows, index := xlane2ModelRowsIndex(t, projection)
		if !runtimeTraceProjSemanticSubsetSameBoard(index, rows["xlane2-e35"], rows["xlane2-e34"]) {
			t.Fatalf("件5: an equal fingerprint must stay board-compatible (照旧)")
		}
		superTag := strings.TrimSpace(rows["xlane2-e34"].EvidenceTag)
		if superTag == "" || rows["xlane2-e35"].SemanticMemberSubsetOf != superTag {
			t.Fatalf("件5: the equal-fingerprint seat must demote as before")
		}
	})
}

// 修补轮 件2 负臂: two same-subject semantic seats sharing ONE verbatim line
// envelope make the partner match ambiguous — the WHOLE clause stays absent
// (zero 「其中…重叠」 bytes, mark unlit), never a first-match guess.
func TestXLANE2SelfGapOverlapAmbiguousPartnerWholeClauseAbsent(t *testing.T) {
	projection := xlane2SelfGapOverlapProjection()
	// The single-span seat moves onto the family seat's exact envelope: the
	// 600..620 overlap entry now matches TWO rows (ambiguous), and the
	// 700..705 entry matches none (unresolvable) — zero clauses survive.
	projection.SemanticSpans[1].LineStart = 600
	projection.SemanticSpans[1].LineEnd = 620
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "与语义席[") || strings.Contains(fence, "其中 15.000ms") {
		t.Fatalf("件2 歧义臂: an ambiguous partner must drop the WHOLE clause, never first-match:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkSelfGapSemanticOverlap) {
		t.Fatalf("件2 歧义臂: the legend mark must stay unlit")
	}
}

// 修补轮 件3: the two XLANE-2 legend entry HEADS are byte-locked. The
// bidirectional probe (成员子集/member subset) scans the FENCE face, and the
// direction-A legend check reads the same catalog it renders from — so an
// entry-head drift previously turned nothing red (the body words satisfied
// every probe incidentally). The teaching token IS a promise face: verbatim.
func TestXLANE2LegendEntryHeadsVerbatim(t *testing.T) {
	heads := map[runtimeTraceProjMark][2]string{
		runtimeTraceProjMarkSemanticMemberSubset: {
			"- `为[E#]成员子集(整席降道)` = ",
			"- `member subset of [E#] (whole-seat demotion)` = ",
		},
		runtimeTraceProjMarkSelfGapSemanticOverlap: {
			"- `其中 X ms 与语义席[E#]重叠` = ",
			"- `of which X ms overlaps semantic seat [E#]` = ",
		},
	}
	seen := 0
	for _, entry := range runtimeTraceProjLegendCatalog() {
		want, ok := heads[entry.Mark]
		if !ok {
			continue
		}
		seen++
		if !strings.HasPrefix(entry.ZH, want[0]) {
			t.Fatalf("图例词条头逐字锁 (zh, mark %d): want prefix %q, got %q", entry.Mark, want[0], entry.ZH)
		}
		if !strings.HasPrefix(entry.EN, want[1]) {
			t.Fatalf("图例词条头逐字锁 (en, mark %d): want prefix %q, got %q", entry.Mark, want[1], entry.EN)
		}
	}
	if seen != len(heads) {
		t.Fatalf("catalog must carry both XLANE-2 entries (saw %d/%d)", seen, len(heads))
	}
}

// 修补轮 件4: the SEATED subset seat (node.Rank > 0) leaves the ◎ population
// through the exclusion arm's population half — its board bar disappears, the
// dedicated footnote counts and names it, and the §29.112 closure identity
// (rendered + cut + represented == population, disclosure closure included)
// still closes. (The witness form's seatless rows exercise only the semantic-
// census half; this pin wakes the dormant population half.)
func TestXLANE2SeatedSubsetSeatLeavesElimPopulation(t *testing.T) {
	projection := xlane2MemberSubsetProjection()
	projection.SemanticSpans[1].Rank = 4
	projection.SemanticSpans[1].Tier = "secondary"
	model, fence := elimRenderOverview(t, projection, true)
	e35 := xlane2FindRowByEvidenceID(&model, "xlane2-e35")
	if e35 == nil || e35.Node.Rank != 4 || strings.TrimSpace(e35.SemanticMemberSubsetOf) == "" {
		t.Fatalf("fixture drifted: the seated subset seat must render demoted (Rank=4)")
	}
	if !runtimeTraceProjElimRankItemRow(*e35) || runtimeTraceProjElimSemanticCensusRow(*e35) {
		t.Fatalf("fixture drifted: the seated seat must be a rank-item row (population half), not a census row")
	}
	if runtimeTraceProjElimEligible(*e35) {
		t.Fatalf("件4: the seated subset seat must leave the ◎ population")
	}
	if !runtimeTraceProjElimMemberSubsetExcluded(*e35) {
		t.Fatalf("件4: the exclusion census must count the seated seat")
	}
	tag := strings.TrimSpace(e35.EvidenceTag)
	if tag == "" {
		t.Fatalf("fixture drifted: the seated seat must carry an evidence tag")
	}
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "["+tag+"]") {
			t.Fatalf("件4: the seated subset seat's ◎ bar must disappear: %q", line)
		}
	}
	// Footnote count includes the seated seat (census = seated e35 + seatless
	// e49) and names its tag.
	census := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if runtimeTraceProjElimMemberSubsetExcluded(rows[i]) {
				census++
			}
		}
	}
	if census != 2 {
		t.Fatalf("fixture drifted: want the two subset seats census-excluded, got %d", census)
	}
	disclosed := -1
	for _, line := range strings.Split(fence, "\n") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "· 为语义席成员子集(降道):%d 行,见明细", &n); err == nil {
			disclosed = n
		}
	}
	if disclosed != census {
		t.Fatalf("件4: footnote count %d != excluded census %d (closure identity):\n%s", disclosed, census, fence)
	}
	if !strings.Contains(fence, "["+tag+"]") {
		t.Fatalf("件4: the footnote must name the seated seat [%s]:\n%s", tag, fence)
	}
	// The §29.112 closure identity (carrier/disclosure/accounting) closes on
	// the seated-arm shape too.
	elimGapAssertBoardAccounting(t, projection)
}
