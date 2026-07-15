package tool

// answer_document_projection_elim_semantic_test.go — RNB-2 件4 pins (ELIM-SEM
// 方案A, §29.88 R1 用户裁定 + W4-a/E30 形; ledger
// docs/design/real_trace_campaign_20260705.md, 2026-07-15):
//
//	(a) chain semantic FALLBACK seat — a seated on-chain semantic member cut
//	    by TOP5 re-enters as ONE ordinary member line at the chain segment
//	    tail (◇-max 保底同构; §29.42.4 零铸造 — transcribed existing member);
//	(b) negative control — a semantic member already in TOP5 appends nothing;
//	(c) 多类同时出榜 = 单一最大席 + 计数披露 footnote;
//	(d) W4-a — seatless ◇ semantic rows get the counted census footnote;
//	(e) E30 form — seatless ⛓ semantic rows get the symmetric footnote.
//
// MUTATION self-checks (cp-copy recovery): dropping the semanticFallback scan
// reds (a)+(c); dropping the semScan census reds (d)+(e).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// elimSemanticSeatedNode is a SEATED on-chain semantic rank member (the SEM
// twin-fold adopted-seat carriage: SemanticClass + Rank>0).
func elimSemanticSeatedNode(id, subject string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
	node := elimChainNode(id, subject, "class_verification", "", rank, eff, line)
	node.Predicate = "trace_semantic_span"
	node.SemanticClass = "class_verification"
	return node
}

// elimSemanticSeatlessNode is a valued ✦-form semantic row OUTSIDE the rank
// population (no rank identity — the W4-a/E30 census material).
func elimSemanticSeatlessNode(id, subject, class, relevance string, value float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: subject, Object: class, TypeToken: class, SemanticClass: class,
		Predicate: "trace_semantic_span", ChainRelevance: relevance,
		ImpactMS: value, CumulativeImpactMS: value,
		LineStart: line, LineEnd: line + 5, Confidence: 0.7,
	}
}

// elimSemanticFallbackProjection: 6 plain chain rank rows (eff 10..5.5) bury
// the seated semantic member (eff semEff) below TOP5 — the E29 rank#7 shape.
func elimSemanticFallbackProjection(semEff float64) types.TraceCausalProjection {
	chain := []types.TraceCausalProjectionNode{
		elimChainNode("E-c1", "w1-1", "runnable_wait", "runnable", 1, 10.0, 100),
		elimChainNode("E-c2", "w2-2", "runnable_wait", "runnable", 2, 9.0, 110),
		elimChainNode("E-c3", "w3-3", "runnable_wait", "runnable", 3, 8.0, 120),
		elimChainNode("E-c4", "w4-4", "d_state_or_io_wait", "d_sleep", 4, 7.0, 130),
		elimChainNode("E-c5", "w5-5", "d_state_or_io_wait", "d_sleep", 5, 6.0, 140),
		elimChainNode("E-c6", "w6-6", "runnable_wait", "runnable", 6, 5.5, 150),
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"w1-1", "app-9"},
		WindowStartTs:           100.0, WindowEndTs: 100.2,
		OnChainCauses: chain,
		// The seated semantic member rides the ✦ 语义 lane (SemanticSpans) —
		// the SEM adopted-seat carriage (SemanticClass + Rank>0).
		SemanticSpans: []types.TraceCausalProjectionNode{
			elimSemanticSeatedNode("E-sem", "host-7", 7, semEff, 160),
		},
	}
}

// (a) the fallback seat: rank#7 semantic member renders as the 6th member
// line — ordinary member form, chain segment tail.
func TestElimChainSemanticFallbackSeatAppends(t *testing.T) {
	_, fence := elimRenderOverview(t, elimSemanticFallbackProjection(4.0), true)
	members := elimOverviewMemberLines(fence)
	if len(members) != 6 {
		t.Fatalf("TOP5 + the semantic fallback seat = 6 member lines, got %d:\n%s", len(members), fence)
	}
	last := members[len(members)-1]
	if !strings.Contains(last, "4.000ms") || !strings.Contains(last, "类校验") ||
		!strings.Contains(last, "⛓ 链上") || !strings.Contains(last, "[E") {
		t.Fatalf("the fallback seat must transcribe the largest off-board chain semantic member:\n%s", last)
	}
	for _, line := range members[:5] {
		if strings.Contains(line, "类校验") {
			t.Fatalf("fixture: no semantic member may sit in TOP5:\n%s", fence)
		}
	}
	// 零铸造: no new ordinal / no lead marker on the fallback line.
	if strings.Contains(last, "#") {
		t.Fatalf("the fallback seat renders as an ordinary member line (zero ordinals):\n%s", last)
	}
}

// (b) negative control: a semantic member inside TOP5 appends nothing (one
// semantic member line total, five member lines total).
func TestElimChainSemanticFallbackNotTriggeredWhenSeatedInTop(t *testing.T) {
	projection := elimSemanticFallbackProjection(4.0)
	// Promote the semantic member into TOP5 (eff 9.5 > E-c2's 9.0).
	for i := range projection.SemanticSpans {
		if projection.SemanticSpans[i].EvidenceID == "E-sem" {
			projection.SemanticSpans[i].EffectiveImpactMS = 9.5
			projection.SemanticSpans[i].ImpactMS = 9.5
			projection.SemanticSpans[i].CumulativeImpactMS = 9.5
			projection.SemanticSpans[i].Rank = 2
		}
	}
	_, fence := elimRenderOverview(t, projection, true)
	members := elimOverviewMemberLines(fence)
	if len(members) != 5 {
		t.Fatalf("an in-TOP5 semantic member must not trigger the fallback (want 5 member lines, got %d):\n%s", len(members), fence)
	}
	semLines := 0
	for _, line := range members {
		if strings.Contains(line, "类校验") {
			semLines++
		}
	}
	if semLines != 1 {
		t.Fatalf("exactly one semantic member line expected, got %d:\n%s", semLines, fence)
	}
}

// (c) 多类同时出榜 = 单一最大席 + 计数披露.
func TestElimChainSemanticFallbackSingleSeatWithCountDisclosure(t *testing.T) {
	projection := elimSemanticFallbackProjection(4.0)
	second := elimSemanticSeatedNode("E-sem2", "host-8", 8, 3.0, 170)
	second.SemanticClass = "jit_compile"
	second.TypeToken = "jit_compile"
	second.Object = "jit_compile"
	projection.SemanticSpans = append(projection.SemanticSpans, second)
	_, fence := elimRenderOverview(t, projection, true)
	members := elimOverviewMemberLines(fence)
	if len(members) != 6 {
		t.Fatalf("exactly ONE fallback seat even with two off-board semantic members (got %d lines):\n%s", len(members), fence)
	}
	if !strings.Contains(members[5], "类校验") || strings.Contains(fence, "JIT编译 3.000ms") {
		t.Fatalf("the single seat must be the LARGEST off-board semantic member:\n%s", fence)
	}
	if !strings.Contains(fence, "· ⛓ 语义类持席行另有 1 行未入榜(TOP5 值切),见明细") {
		t.Fatalf("the count disclosure footnote must name the remaining off-board seats:\n%s", fence)
	}
	// en mirror.
	_, fenceEN := elimRenderOverview(t, projection, false)
	if !strings.Contains(fenceEN, "· ⛓ 1 more seated semantic-class row(s) cut by TOP5 — see the detail table") {
		t.Fatalf("en count disclosure missing:\n%s", fenceEN)
	}
}

// (d)+(e) W4-a / E30 census footnotes for seatless semantic rows.
func TestElimSeatlessSemanticCensusFootnotes(t *testing.T) {
	projection := elimBoardProjection()
	projection.SemanticSpans = append(projection.SemanticSpans,
		elimSemanticSeatlessNode("E-adj1", "t1-11", "class_verification", "adjacent", 2.079, 400),
		elimSemanticSeatlessNode("E-adj2", "t2-12", "class_verification", "adjacent", 0.5, 410),
		elimSemanticSeatlessNode("E-adj3", "t3-13", "jit_compile", "adjacent", 0.4, 420),
		elimSemanticSeatlessNode("E-e30", "RenderThread-64334", "texture_upload", "on_chain", 1.439, 430),
	)
	_, fence := elimRenderOverview(t, projection, true)
	// Evidence ordinals are allocated at model build — pin the byte form
	// around the [E#] slot instead of a hard tag number.
	if !strings.Contains(fence, "· ◇ 语义优化 3 行(类校验2、JIT编译1,最大 2.079ms [E") ||
		!strings.Contains(fence, "])见邻近段(未铸序数,不参与汇排)") {
		t.Fatalf("the ◇ W4-a census footnote must render:\n%s", fence)
	}
	if !strings.Contains(fence, "· ⛓ 语义优化 1 行(纹理上传1,最大 1.439ms [E") ||
		!strings.Contains(fence, "])见主树语义行(未入根因排序,不参与汇排)") {
		t.Fatalf("the ⛓ E30-form census footnote must render:\n%s", fence)
	}
	// Seatless rows never become members (§29.42.4 zero minting).
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "2.079ms") || strings.Contains(line, "纹理上传") {
			t.Fatalf("a seatless semantic row must stay a footnote, never a member:\n%s", line)
		}
	}
	// en mirror.
	_, fenceEN := elimRenderOverview(t, projection, false)
	if !strings.Contains(fenceEN, "semantic-optimization row(s)") ||
		!strings.Contains(fenceEN, "largest 2.079ms [E") ||
		!strings.Contains(fenceEN, "largest 1.439ms [E") {
		t.Fatalf("en census footnotes missing:\n%s", fenceEN)
	}
	// Negative: a board WITHOUT seatless semantic rows renders neither line.
	_, bare := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(bare, "语义优化") {
		t.Fatalf("no census line without seatless semantic rows:\n%s", bare)
	}
}
