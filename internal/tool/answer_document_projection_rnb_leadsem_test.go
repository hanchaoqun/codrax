package tool

// answer_document_projection_rnb_leadsem_test.go — RNB+LEAD-SEM 显示批
// (docs/design/real_trace_campaign_20260705.md §21/§22, 2026-07-07):
//
//   R1 — the gated composition's runnable component gets a display-only
//        sub-row (⧖ runnable X(全额)·就绪排队积压·gated 分量,不重复计入排序);
//        gate = typed component field non-zero; never a sort/coverage input.
//   R2 — one segment published on both the root_cause_rank lane and the
//        wakeup_causal_impact lane folds into ONE displayed row (the chain
//        row keeps the tree position); the rank row's type word / rank badge
//        / confidence ride the fold note, its E# stays on the evidence index
//        and the lossless stanza. Join key = SFD 同款 (canonical subject +
//        exact line range) + effective-mirror equality + cross-window veto.
//        四证: cmp_01 E7/E8 + opendir E6/E7 (sibling cause form), huadong
//        E4/E5 (trunk + cause child form), huadong E11/E13 (sibling chain
//        form).
//   L1 — a typed cross-window row whose actual total was never captured
//        (ActualImpactMS<=0) wears the value-less ⚠跨窗 marker, never the
//        fake "⚠实际0.000ms" scalar (cmp_01 A④: 16 semantic rows).
//   L2 — lead tier 4 (cmp_01 A①): primary bucket non-empty but fully
//        demoted ∧ on-chain fallback empty-handed → the largest semantic
//        optimization span leads with dedicated wording that never claims
//        主根因; conclusion line and compare cell share the single source.
//   L3 — the 见背景压力段 pointer renders only when the background stanza is
//        non-empty (cmp_01 A② defensive check).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- shared record-level fixtures (real pipeline: observations → compile →
// --- blocks → markdown) ---------------------------------------------------

// rnbAnchor is a 101ms query-window anchor (window mode).
func rnbAnchor() types.ObservationRecord {
	return types.ObservationRecord{
		ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
		Subject: "app-100", Object: "frame",
		Span:      types.ObservationSpan{StartTs: 100.0, EndTs: 100.101},
		RichNotes: []string{"window_source=query_window"},
	}
}

func rnbPath(path string) types.ObservationRecord {
	return types.ObservationRecord{
		ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
		Object: path,
	}
}

// rnbSiblingRankObs / rnbSiblingChainObs mirror the huadong E11/E13 sibling
// form: one segment (sysr-8, :1000-2000) published as a root_cause_rank
// primary (rank=2, gated effective 0.813) AND a wakeup_causal_impact hop row
// (running 2.770 raw, same gated effective mirror). Both wear the typed
// inversion candidacy.
func rnbSiblingRankObs(notes ...string) types.ObservationRecord {
	record := projV3Obs("rnb-rank", "root_cause_primary", "root_cause_primary:sysr",
		"sysr-8", "priority_inversion_candidate", "0.813", 0.813, 1000, 2000,
		append([]string{
			"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=2", "priority_inversion_candidate=true",
			"gated_runnable=0.621", "gated_running_deficit=0.192",
			"effective_impact_ms=0.813",
		}, notes...)...)
	record.RichNotes[1] = "cumulative_impact_ms=2.770"
	record.Confidence = 0.91
	return record
}

func rnbSiblingChainObs(notes ...string) types.ObservationRecord {
	record := projV3Obs("rnb-chain", "wakeup_causal_impact", "wakeup_causal_impact:sysr",
		"sysr-8", "running", "2.770", 2.770, 1000, 2000,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2",
			"dominant_state=running", "priority_inversion_candidate=true",
			"gated_runnable=0.621", "gated_running_deficit=0.192",
			"effective_impact_ms=0.813",
		}, notes...)...)
	record.Confidence = 0.78
	return record
}

func rnbSiblingObs(rank, chain types.ObservationRecord) []types.ObservationRecord {
	return []types.ObservationRecord{rnbAnchor(), rnbPath("waker-3 -> mid-2 -> app-100"), rank, chain}
}

// --- R2: sibling chain form (huadong E11/E13) -------------------------------

func TestRNBSameSegmentTwinFoldSiblingChainFormZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), rnbSiblingObs(rnbSiblingRankObs(), rnbSiblingChainObs()), "")
	despaced := vs2Despace(md)
	// PTV8-RCR-A EVOLUTION RECORD (§24 ①退役/§24.2, 2026-07-08): the RNB R2
	// 同段rank行并入 note is RETIRED — the fold is now a NATIVE single node:
	// rank badge on 行1, the rank row's seat/confidence on 行2, its E# merged
	// into 行1's [E#+E#] bracket. The join/guard engine is untouched.
	if strings.Contains(md, "同段rank行并入") {
		t.Fatalf("the retired fold note must not render:\n%s", md)
	}
	// ONE displayed row: the rank-lane detail row form is gone, the chain-lane
	// row stays (detail table + lossless stanza names).
	if strings.Contains(md, "sysr-8 / 优先级反转候选") {
		t.Fatalf("the rank-lane row must not render as its own row after the fold:\n%s", md)
	}
	if !strings.Contains(md, "sysr-8 / running") {
		t.Fatalf("the chain-lane row must stay the displayed row:\n%s", md)
	}
	// The rank badge moved onto the kept chain row (typed Rank transfer); the
	// row is an inversion cause node → ⇅ glyph + the runnable+running 词位.
	// EVOLUTION RECORD (§29.27.1 徽章跟随席位, 2026-07-11): the badge is the
	// pictograph of the row's PUBLISHED seat (#2 here) — the retired board-
	// position lane wore ➊ on this #2 seat because it was the only board row.
	if !strings.Contains(despaced, "➋⇅sysr-8") {
		t.Fatalf("the kept chain row must wear its seat's badge (➋ for 根因排序#2):\n%s", md)
	}
	// 行2 carries the rank row's confidence (RULE3-1 件2: the ➋ badge
	// carries the seat ordinal; the word no longer restates on 行2).
	if !strings.Contains(despaced, "优先级反转候选·置信高") {
		t.Fatalf("行2 must carry the folded rank row's confidence:\n%s", md)
	}
	if !strings.Contains(despaced, "+E2]") {
		t.Fatalf("行1 must merge the folded rank row's E# ([E#+E#]):\n%s", md)
	}
	// The rank row's E# stays reachable: its evidence-index entry carries the
	// reader-facing rank detail; the lossless block carries the 根因排序 line.
	if !strings.Contains(md, "根因排序第 2 位") || strings.Contains(md, "rank=2") {
		t.Fatalf("the folded rank row's reader-facing detail must stay on the evidence index:\n%s", md)
	}
	if !strings.Contains(despaced, "已并入本行,数值不重复计入") {
		t.Fatalf("the lossless block must carry the folded rank row's seat line:\n%s", md)
	}
}

// R2 display-only / numerator-invariance pin: the conclusion line and the
// coverage line are byte-identical between the folded render and a control
// whose rank row sits on a DIFFERENT segment (no fold) — the fold changes
// display grouping only, never lead selection, never the coverage numerator
// (覆盖分子红线).
func TestRNBSameSegmentTwinFoldKeepsLeadAndCoverageInvariant(t *testing.T) {
	folded := audit730Render(t, audit730Bus(""), rnbSiblingObs(rnbSiblingRankObs(), rnbSiblingChainObs()), "")
	controlRank := rnbSiblingRankObs()
	controlRank.Span.LineStart, controlRank.Span.LineEnd = 3000, 4000
	controlRank.SupportRefs = []string{"berlin.systrace:3000-4000"}
	control := audit730Render(t, audit730Bus(""), rnbSiblingObs(controlRank, rnbSiblingChainObs()), "")
	pick := func(md, token string) string {
		for _, line := range strings.Split(md, "\n") {
			if strings.Contains(line, token) {
				return line
			}
		}
		return ""
	}
	// PTV8-RCR-A: the fold witness is the merged [E#+E#] bracket now.
	if strings.Contains(control, "+E2]") {
		t.Fatalf("line-range mismatch must never fold (join-key precision arm):\n%s", control)
	}
	if !strings.Contains(folded, "+E2]") {
		t.Fatalf("the control pair must fold (positive witness):\n%s", folded)
	}
	// COV+LEAD 批 (§24.11 C-1, 2026-07-08). EVOLUTION RECORD: the coverage line
	// keeps FULL byte-identity across fold/no-fold (覆盖分子红线 unchanged). The
	// lead line's invariant is now FACT identity, not byte identity: the lead
	// election consumes the shared post-aggregation rank board (lead == the ➊
	// row), and the fold changes which FACE of the same segment renders — the
	// folded native node (running form, §24.4) vs the standalone rank row — so
	// the headline words follow the rendered face while subject, seat and the
	// 链上累计 magnitude stay identical and no SUM ever publishes.
	{
		foldedLine, controlLine := pick(folded, "已归因"), pick(control, "已归因")
		if foldedLine == "" || foldedLine != controlLine {
			t.Fatalf("coverage line must be byte-identical across fold/no-fold (覆盖分子红线):\nfolded: %q\ncontrol: %q", foldedLine, controlLine)
		}
	}
	for _, md := range []string{folded, control} {
		lead := pick(md, "**主根因(=已证链上单项最大可消除量):**")
		if !strings.Contains(lead, "**主根因(=已证链上单项最大可消除量):** sysr-8") || !strings.Contains(lead, "链上累计 2.770ms") {
			t.Fatalf("lead must name the same fact (sysr-8, 链上累计 2.770ms) across fold/no-fold:\n%q", lead)
		}
	}
}

// --- R2: trunk + cause child form (huadong E4/E5) ---------------------------

func TestRNBSameSegmentTwinFoldCauseChildForm(t *testing.T) {
	rank := projV3Obs("rnb-rank", "root_cause_primary", "root_cause_primary:rsu",
		"RSU-1963", "priority_inversion_candidate", "0.058", 0.058, 1628546, 1629554,
		"rank=9", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "priority_inversion_candidate=true",
		"gated_runnable=0.058", "gated_running_deficit=0.000",
		"effective_impact_ms=0.058")
	rank.RichNotes[1] = "cumulative_impact_ms=4.115"
	rank.Confidence = 0.91
	chain := projV3Obs("rnb-chain", "wakeup_causal_impact", "wakeup_causal_impact:rsu",
		"RSU-1963", "running", "4.115", 4.115, 1628546, 1629554,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"dominant_state=running", "priority_inversion_candidate=true",
		"gated_runnable=0.058", "gated_running_deficit=0.000",
		"effective_impact_ms=0.058")
	chain.Confidence = 0.78
	md := audit730Render(t, audit730Bus(""),
		[]types.ObservationRecord{rnbAnchor(), rnbPath("RSU-1963 -> app-100"), rank, chain}, "")
	despaced := vs2Despace(md)
	// Pre-fold the rank twin rendered as a state-makeup child of the trunk
	// row (今 ├─构成─, 原 ├─成因─; A2 件4①) — post-fold no such row exists at
	// all in this fixture.
	if strings.Contains(md, "构成─") {
		t.Fatalf("the rank twin must not mint a makeup child row after the fold:\n%s", md)
	}
	// PTV8-RCR-A: the trunk row carries 行2 (rank row's seat/confidence) and
	// merges its E# — the retired note never returns.
	if !strings.Contains(despaced, "优先级反转候选·根因排序#9·置信高") {
		t.Fatalf("the trunk row must carry the folded rank row's 行2 seat:\n%s", md)
	}
	if !strings.Contains(despaced, "+E2]") {
		t.Fatalf("the trunk row must merge the folded rank row's E#:\n%s", md)
	}
	if strings.Contains(md, "同段rank行并入") {
		t.Fatalf("the retired fold note must not render:\n%s", md)
	}
}

// --- R2: sibling cause form under a lock main row (opendir E6/E7 verbatim
// --- coordinates: cum 58.919 == 58.919). cmp_01 E7/E8 is the SAME
// --- tree-position form but its two lanes publish DIVERGING cumulative
// --- accounts (47.503 vs 28.230, 标本 :416/:417) — under the 复核 W-A second
// --- equality it stays a two-row render and is pinned as the cum_mismatch
// --- guard arm below (不同账目绝不折) --------------------------------------

func TestRNBSameSegmentTwinFoldSiblingCauseForm(t *testing.T) {
	lock := projV3Obs("rnb-lock", "root_cause_primary", "root_cause_primary:lock",
		"#RxComputationT-16816", "monitor_contention", "112.223", 112.223, 45000, 45500,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep", "effective_impact_ms=112.223")
	// opendir E7: this rank lane rides the SECONDARY funnel (tier=secondary,
	// rank=2) — the root_cause_* prefix arm covers every funnel tier.
	rank := projV3Obs("rnb-rank", "root_cause_secondary", "root_cause_secondary:rxc",
		"#RxComputationT-16816", "priority_inversion_candidate", "37.410", 37.410, 45689, 79142,
		"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "priority_inversion_candidate=true",
		"gated_runnable=20.713", "gated_running_deficit=16.697",
		"effective_impact_ms=37.410")
	rank.RichNotes[1] = "cumulative_impact_ms=58.919"
	rank.Confidence = 0.91
	chain := projV3Obs("rnb-chain", "wakeup_causal_impact", "wakeup_causal_impact:rxc",
		"#RxComputationT-16816", "running", "58.919", 58.919, 45689, 79142,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"dominant_state=running", "priority_inversion_candidate=true",
		"gated_runnable=20.713", "gated_running_deficit=16.697",
		"effective_impact_ms=37.410")
	chain.Confidence = 0.78
	md := audit730Render(t, audit730Bus(""),
		[]types.ObservationRecord{rnbAnchor(), rnbPath("#RxComputationT-16816 -> app-100"), lock, rank, chain}, "")
	despaced := vs2Despace(md)
	// Exactly ONE makeup child remains (the chain-lane running row); the rank
	// twin folded into it instead of minting a second ├─构成─ sibling (A2
	// 件4①: 成因→构成). Two occurrences = the single tree row + its legend
	// entry.
	if got := strings.Count(md, "构成─"); got != 2 {
		t.Fatalf("exactly one makeup child (plus its legend entry) must remain after the fold, got %d:\n%s", got, md)
	}
	if strings.Contains(md, "#RxComputationT-16816 / 优先级反转候选") {
		t.Fatalf("the rank twin must not keep its own row:\n%s", md)
	}
	// PTV8-RCR-A: 行2 confidence + merged E# replace the retired fold note
	// (RULE3-1 件2: the ➋ badge carries the adopted seat ordinal).
	if !strings.Contains(despaced, "优先级反转候选·置信高") {
		t.Fatalf("the kept cause row must carry the folded rank row's 行2 confidence:\n%s", md)
	}
	if !strings.Contains(despaced, "+E3]") {
		t.Fatalf("the kept cause row must merge the folded rank row's E#:\n%s", md)
	}
	// The lock main row is untouched (different segment, not an inversion
	// arm) — its rank-1 lead survives.
	if !strings.Contains(md, "**主根因(=已证链上单项最大可消除量):** #RxComputationT-16816") {
		t.Fatalf("the lock rank-1 lead must be untouched by the fold:\n%s", md)
	}
}

// --- R2 guards (fail-open to the two-row render) ----------------------------

func TestRNBSameSegmentTwinFoldGuards(t *testing.T) {
	for name, mutate := range map[string]func(*types.ObservationRecord, *types.ObservationRecord){
		"line_range": func(rank, chain *types.ObservationRecord) {
			rank.Span.LineStart, rank.Span.LineEnd = 3000, 4000
			rank.SupportRefs = []string{"berlin.systrace:3000-4000"}
		},
		"subject": func(rank, chain *types.ObservationRecord) {
			rank.Subject = "other-9"
		},
		// Effective mirror mismatch = a different accounting (the pre-P0-E
		// raw-vs-gated twin shape, §15.B RCX² 退档) — never folded, the
		// engine de-double-publish owns it.
		"effective_mismatch": func(rank, chain *types.ObservationRecord) {
			for i, note := range rank.RichNotes {
				if note == "effective_impact_ms=0.813" {
					rank.RichNotes[i] = "effective_impact_ms=2.770"
				}
			}
		},
		// Cross-window veto (SFD F1 mirror): both arms declare their own
		// typed selected_window and the endpoints diverge beyond ±1ms.
		"cross_window": func(rank, chain *types.ObservationRecord) {
			rank.RichNotes = append(rank.RichNotes, "selected_window=100.000000..100.101000")
			chain.RichNotes = append(chain.RichNotes, "selected_window=100.200000..100.301000")
		},
		// 复核 W-A second equality: diverging CUMULATIVE accounts never fold
		// (不同账目绝不折) — cmp_01 E7/E8 verbatim accounts (标本 :416/:417):
		// the rank lane's cum counts the enclosing chain scope (47.503) while
		// the hop row's own cum is 28.230. Without this arm the folded rank
		// cum could raise a trunk Chain row's depth-MAX coverage numerator a
		// pre-fold Cause row never competed in.
		"cum_mismatch": func(rank, chain *types.ObservationRecord) {
			rank.RichNotes[1] = "cumulative_impact_ms=47.503"
			chain.RichNotes[1] = "cumulative_impact_ms=28.230"
		},
	} {
		rank, chain := rnbSiblingRankObs(), rnbSiblingChainObs()
		mutate(&rank, &chain)
		md := audit730Render(t, audit730Bus(""), rnbSiblingObs(rank, chain), "")
		// PTV8-RCR-A: the fold witness is the merged [E#+E#] bracket (digit
		// form — the 行2 legend entry's [E#+E#] template never matches it).
		if strings.Contains(md, "+E2]") {
			t.Fatalf("%s: guard must veto the fold (fail-open, two rows stay):\n%s", name, md)
		}
	}
}

// The ×N / ambiguity arms of the fold classifier, pinned at the unit level
// (双臂 pin, SFD 复核 F2 mirror): an aggregate arm never joins; two rank views
// under one key are ambiguity and never fold.
func TestRNBSameSegmentTwinFoldUnitGuards(t *testing.T) {
	rank := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "u-rank",
		Subject: "sysr-8", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
		Rank: 2, ChainRelevance: "on_chain", PriorityInversionCandidate: true,
		ImpactMS: 0.813, EffectiveImpactMS: 0.813, LineStart: 1000, LineEnd: 2000, Confidence: 0.91,
	}
	chain := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "u-chain",
		Subject: "sysr-8", Predicate: "wakeup_causal_impact", Object: "running",
		StateKind: "running", ChainRelevance: "on_chain", PriorityInversionCandidate: true,
		ImpactMS: 2.770, EffectiveImpactMS: 0.813, LineStart: 1000, LineEnd: 2000, Confidence: 0.78,
	}
	if kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain}); len(kept) != 1 || len(peers) != 1 {
		t.Fatalf("control pair must fold: kept=%d peers=%d", len(kept), len(peers))
	}
	merged := chain
	merged.MergedCount = 2
	if kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, merged}); len(kept) != 2 || peers != nil {
		t.Fatalf("a ×N aggregate arm must never join (kept=%d)", len(kept))
	}
	secondRank := rank
	secondRank.EvidenceID = "u-rank-2"
	secondRank.Rank = 3
	if kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, secondRank, chain}); len(kept) != 3 || peers != nil {
		t.Fatalf("two rank views under one key are ambiguity and must never fold (kept=%d)", len(kept))
	}
}

// --- R1: the gated runnable component sub-row --------------------------------

// PTV8-RCR-A EVOLUTION RECORD (§24 ⑤退役/§24.1, 2026-07-08): the R1
// `⧖ runnable …gated 分量,不重复计入排序` display sub-row and the 影响构成
// disclosure are RETIRED — the four-line grammar's 行3 「=」breakdown and the
// 拆解子行 carry the composition with per-component calibers; the machine
// identity Σ计入==V guards the numbers.
func TestRNBGatedRunnableComponentSubRow(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), rnbSiblingObs(rnbSiblingRankObs(), rnbSiblingChainObs()), "")
	despaced := vs2Despace(md)
	// 行3 + the two 拆解子行 (0.621 + 0.192 == 0.813, identity holds).
	if !strings.Contains(despaced, "有效归因0.813ms=runnable(全额)0.621ms+running(折算)0.192ms") {
		t.Fatalf("行3 breakdown must render:\n%s", md)
	}
	if !strings.Contains(despaced, "runnable原始0.621ms→计入0.621ms(全额)") {
		t.Fatalf("runnable 拆解子行 must render:\n%s", md)
	}
	if !strings.Contains(despaced, "running原始2.770ms→计入0.192ms(折算,按全域最大核最高频,运行频点非最高)") {
		t.Fatalf("running 拆解子行 must render:\n%s", md)
	}
	// Retired seats never return.
	for _, banned := range []string{"gated 分量", "gated分量", "不重复计入排序", "影响构成"} {
		if strings.Contains(despaced, vs2Despace(banned)) {
			t.Fatalf("retired wording %q leaked:\n%s", banned, md)
		}
	}
	// The caliber legend entries render on demand (§24.1补).
	for _, want := range []string{"- `全额` =", "- `折算,按全域最大核最高频`/`按全域最大核最高频折算`"} {
		if !strings.Contains(md, want) {
			t.Fatalf("caliber legend entry %q must render:\n%s", want, md)
		}
	}
	// Display-only 负向: the components never sum into any published value.
	if strings.Contains(despaced, "1.434") {
		t.Fatalf("the runnable component must never sum into any published value:\n%s", md)
	}
}

func TestRNBGatedRunnableComponentSubRowAbsentWhenComponentZero(t *testing.T) {
	rank, chain := rnbSiblingRankObs(), rnbSiblingChainObs()
	for _, record := range []*types.ObservationRecord{&rank, &chain} {
		for i, note := range record.RichNotes {
			if note == "gated_runnable=0.621" {
				record.RichNotes[i] = "gated_runnable=0.000"
			}
		}
	}
	md := audit730Render(t, audit730Bus(""), rnbSiblingObs(rank, chain), "")
	despaced := vs2Despace(md)
	// PTV8-RCR-A: with the runnable component zeroed the identity no longer
	// balances (0.192 ≠ 0.813) — the 「=」breakdown REFUSES to render
	// (恒等式 fail-open) and no runnable 拆解子行 appears.
	if strings.Contains(despaced, "ms=runnable(") || strings.Contains(despaced, "runnable原始") {
		t.Fatalf("a zero runnable component must render no breakdown (identity fail-open):\n%s", md)
	}
	if !strings.Contains(despaced, "有效归因0.813ms") {
		t.Fatalf("the plain single-source attribution tag must stay:\n%s", md)
	}
}

// --- L1: the value-less ⚠跨窗 marker ----------------------------------------

func TestRNBCrossWindowMarkerNeverPrintsFakeZeroActual(t *testing.T) {
	within := false
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "t-1", Object: "jit_compile", ImpactMS: 5.277,
			WithinRequestedWindow: &within,
		},
		Kind: runtimeTraceProjTreeRowSemantic, HasData: true,
	}
	_, tags := runtimeTraceProjRowMetricParts(row, 101.0, true, true)
	var joined []string
	for _, tag := range tags {
		joined = append(joined, tag.Text)
	}
	text := strings.Join(joined, " · ")
	if strings.Contains(text, "实际0.000") || strings.Contains(text, "⚠实际") {
		t.Fatalf("an uncaptured actual must never print the fake 0.000 scalar: %q", text)
	}
	if !strings.Contains(text, "⚠跨窗") {
		t.Fatalf("the value-less cross-window marker must render: %q", text)
	}
	// Control: a captured actual keeps the legacy value form.
	row.Node.ActualImpactMS = 31.439
	_, tags = runtimeTraceProjRowMetricParts(row, 101.0, true, true)
	joined = joined[:0]
	for _, tag := range tags {
		joined = append(joined, tag.Text)
	}
	text = strings.Join(joined, " · ")
	if !strings.Contains(text, "⚠实际31.439ms") || strings.Contains(text, "⚠跨窗") {
		t.Fatalf("a captured actual must keep the legacy ⚠实际 value form: %q", text)
	}
}

// --- L2/L3 fixtures ----------------------------------------------------------

// rnbLeadSemObs is the cmp_01 6.0 shape: every primary is a background-demoted
// aggregate, no on-chain row exists, and the tree carries deterministic
// semantic optimization spans that sit OUTSIDE the anchor window (actual never
// captured — the L1 ⚠跨窗 form) with the largest at 83% of the window.
func rnbLeadSemObs() []types.ObservationRecord {
	agg := projV3Obs("lsm-agg", "root_cause_primary", "root_cause_primary:supply",
		"unknown-thread", "supply_pressure", "45375.684", 45375.684, 300, 400,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"subject_kind=aggregate_metric", "type=supply_pressure")
	sem := func(id, name string, impact float64, lineStart, lineEnd int, startTs, endTs float64) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "trace_semantic_span", ClaimKey: "trace_semantic_span:" + id,
			Subject: "Jit thread pool-8870", Object: "jit_compile", Value: "", Unit: "ms", Confidence: 0.66,
			SupportRefs: []string{"berlin.systrace:500-600"},
			Span:        types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd, StartTs: startTs, EndTs: endTs},
			RichNotes: []string{
				"impact_ms=" + name, "semantic_class=jit_compile",
				"span_name=" + name,
			},
		}
	}
	big := sem("lsm-jit", "JIT compiling long com.example.Foo", 83.893, 500, 600, 99.500, 99.584)
	big.RichNotes[0] = "impact_ms=83.893"
	big.RichNotes = append(big.RichNotes, "cumulative_impact_ms=83.893")
	small := sem("lsm-verify", "VerifyClass com.example.Bar", 5.132, 700, 800, 99.300, 99.306)
	small.RichNotes[0] = "impact_ms=5.132"
	small.RichNotes = append(small.RichNotes, "cumulative_impact_ms=5.132")
	small.SupportRefs = []string{"berlin.systrace:700-800"}
	return []types.ObservationRecord{rnbAnchor(), agg, big, small}
}

const rnbLeadSemZHFixedForm = "未定位到链上主根因；窗口内最大语义优化span:JITcompilinglongcom.example.Foo83.893ms(占窗83%,语义优化span·无唤醒链,见背景压力段)"

func TestLeadSemSemanticFallbackConclusionZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), rnbLeadSemObs(), "")
	despaced := vs2Despace(md)
	if !strings.Contains(despaced, rnbLeadSemZHFixedForm) {
		t.Fatalf("the LEAD-SEM tier-4 conclusion must render the fixed form:\n%s", md)
	}
	// 负向 pin (禁冒称): the semantic FALLBACK lead never wears the 主根因
	// claim. EVOLUTION RECORD (SEM-LEAD §29.7-2 ①, 2026-07-10,
	// real_trace_campaign_20260705.md): scope NARROWED to the tier-4 fallback
	// lane — this fixture's semantic rows carry no rank seat, which is
	// exactly the lane the ban still governs; an ON-CHAIN RANK-SEATED
	// semantic row crowns 主根因 through the primary lane (pinned by the
	// SEM-LEAD board/lead tests).
	// CROWNPOS-1: the definitional-prefix form 「主根因(=…):」 carries no bare
	// "主根因:" substring — the ban needs both arms or the new form escapes.
	if strings.Contains(md, "主根因:") || strings.Contains(md, "主根因(=") ||
		strings.Contains(md, "Primary root cause:") || strings.Contains(md, "Primary root cause (=") {
		t.Fatalf("the semantic fallback lead must never claim 主根因:\n%s", md)
	}
	// L1 end-to-end: the out-of-window semantic rows wear the value-less
	// ⚠跨窗 marker, never the fake 0.000 actual.
	if !strings.Contains(md, "⚠跨窗") || strings.Contains(md, "实际0.000") {
		t.Fatalf("out-of-window semantic rows must wear ⚠跨窗 without a fake actual:\n%s", md)
	}
}

// The comparison overview's primary cell flows through the SAME single source
// (cmp_01 A① fix direction: compare cell 经单源自动同改): side B's cell and
// its conclusion line carry the same semantic-fallback text.
func TestLeadSemSemanticFallbackCompareCellSameSource(t *testing.T) {
	const artifactA = "7.0B30SP22_7315.systrace"
	const artifactB = "6.0B138_3900.sys.systrace"
	stamp := func(records []types.ObservationRecord, artifact string) []types.ObservationRecord {
		for i := range records {
			records[i].ID = artifact + "-" + records[i].ID
			records[i].ClaimKey = artifact + ":" + records[i].ClaimKey
			records[i].SourceRef = types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: artifact, ArtifactKind: "trace",
			}
		}
		return records
	}
	sideA := stamp([]types.ObservationRecord{
		rnbAnchor(),
		projV3Obs("a-run", "root_cause_primary", "root_cause_primary:a-run",
			"RSUniRenderThre-1963", "running", "807.276", 807.276, 32642, 199899,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=running", "effective_impact_ms=807.276"),
	}, artifactA)
	sideB := stamp(rnbLeadSemObs(), artifactB)
	bus := audit730Bus("")
	md := audit730Render(t, bus, append(sideA, sideB...), "")
	despaced := vs2Despace(md)
	if got := strings.Count(despaced, "未定位到链上主根因；窗口内最大语义优化span:"); got < 2 {
		t.Fatalf("conclusion line AND compare cell must carry the single-source semantic text (got %d):\n%s", got, md)
	}
	if strings.Contains(md, "未定位到链上主根因(见背景压力段)") {
		t.Fatalf("the legacy nil-lead cell must not render when the semantic lane leads:\n%s", md)
	}
}

// Control: a surviving ranked primary keeps tier 1 — the semantic lane never
// fires beside a real lead (L2 第4级禁用 mutation bites the positive pin
// above; this control pins the other direction).
func TestLeadSemSemanticFallbackNeverFiresBesideRankedPrimary(t *testing.T) {
	records := append(rnbLeadSemObs(),
		projV3Obs("real-run", "root_cause_primary", "root_cause_primary:real",
			"worker-7", "running", "12.000", 12.0, 900, 950,
			"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running", "effective_impact_ms=12.000"),
		rnbPath("worker-7 -> app-100"))
	md := audit730Render(t, audit730Bus(""), records, "")
	if strings.Contains(md, "窗口内最大语义优化span") {
		t.Fatalf("the semantic lane must never fire beside a surviving primary:\n%s", md)
	}
	if !strings.Contains(md, "**主根因(=已证链上单项最大可消除量):** worker-7") {
		t.Fatalf("the ranked primary must lead:\n%s", md)
	}
}

// A 0-value semantic best returns nil — the legacy 未定位 text stays (the
// lane never publishes a 0ms "largest span").
func TestLeadSemZeroValueSemanticRowsKeepLegacyText(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{{
			Node: types.TraceCausalProjectionNode{Subject: "t-1", Object: "jit_compile"},
			Kind: runtimeTraceProjTreeRowSemantic, HasData: true,
		}},
	}
	if got := runtimeTraceProjLeadSemanticFallback(model); got != nil {
		t.Fatalf("a 0-value semantic row must never lead: %+v", got)
	}
}

// 优先窗内行: an in-window semantic row wins over a LARGER drilled-out row
// (typed WithinRequestedWindow preference only).
func TestLeadSemPrefersInWindowSemanticRow(t *testing.T) {
	out := false
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{
			{Node: types.TraceCausalProjectionNode{Subject: "t-1", Object: "jit_compile",
				SpanName: "big-out", ImpactMS: 80.0, WithinRequestedWindow: &out},
				Kind: runtimeTraceProjTreeRowSemantic, HasData: true},
			{Node: types.TraceCausalProjectionNode{Subject: "t-2", Object: "jit_compile",
				SpanName: "small-in", ImpactMS: 10.0},
				Kind: runtimeTraceProjTreeRowSemantic, HasData: true},
		},
	}
	got := runtimeTraceProjLeadSemanticFallback(model)
	if got == nil || got.SpanName != "small-in" {
		t.Fatalf("the in-window row must be preferred: %+v", got)
	}
}

// --- L3: the background-pointer non-empty check ------------------------------

func TestLeadSemBackgroundPointerRequiresNonEmptyStanza(t *testing.T) {
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "agg",
			Subject: "unknown-thread", Predicate: "root_cause_primary",
			Object: "supply_pressure", SubjectKind: "aggregate_metric",
			Rank: 1, ChainRelevance: "on_chain", ImpactMS: 100, Confidence: 0.4,
		}},
	}
	empty := runtimeTraceProjTreeModel{}
	if got := runtimeTraceProjConclusionLine(projection, empty, true); got != "**主根因:** 窗口内未定位到链上主根因。" {
		t.Fatalf("an empty background stanza must drop the pointer clause: %q", got)
	}
	if got := runtimeTraceProjComparePrimaryCell(projection, empty, true); got != "未定位到链上主根因" {
		t.Fatalf("the compare cell must drop the pointer clause too: %q", got)
	}
	withBackground := runtimeTraceProjTreeModel{Background: []runtimeTraceProjTreeRow{{
		Node: types.TraceCausalProjectionNode{Subject: "bg-1", ImpactMS: 5}, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
	}}}
	if got := runtimeTraceProjConclusionLine(projection, withBackground, true); got != "**主根因:** 窗口内未定位到链上主根因，见背景压力段。" {
		t.Fatalf("a non-empty background stanza must keep the pointer clause: %q", got)
	}
	if got := runtimeTraceProjComparePrimaryCell(projection, withBackground, true); got != "未定位到链上主根因(见背景压力段)" {
		t.Fatalf("the compare cell must keep the pointer clause: %q", got)
	}
	// The semantic-lane text obeys the same check (L3-in-L2).
	node := types.TraceCausalProjectionNode{Subject: "t-1", Object: "jit_compile",
		SpanName: "JIT compiling", ImpactMS: 83.893}
	emptyText := runtimeTraceProjSemanticLeadText(node, runtimeTraceProjTreeModel{WindowMS: 101}, true)
	if strings.Contains(emptyText, "见背景压力段") {
		t.Fatalf("the semantic text must drop the pointer on an empty stanza: %q", emptyText)
	}
	fullText := runtimeTraceProjSemanticLeadText(node, runtimeTraceProjTreeModel{WindowMS: 101, Background: withBackground.Background}, true)
	// DISPHYG-3 件1 (C8): the semantic lead's pointer clause is PAREN-INTERNAL
	// (「(…,见背景压力段)」) — parenthetical interiors keep the half-width
	// bytes; only the sentence's top-level clause marks went full-width.
	if !strings.Contains(fullText, ",见背景压力段") {
		t.Fatalf("the semantic text must keep the pointer on a non-empty stanza: %q", fullText)
	}
}

// --- revisit76 bidirectional fixture homes -----------------------------------

// rnbTwinFoldProjection exercises the R2 fold note + the R1 gated runnable
// sub-row through the legend bidirectional harness (runnable-dominant shape:
// the ⧖ state icon rides the kept chain row). The two lanes' cumulative
// accounts AGREE (复核 W-A second equality — diverging accounts like cmp_01
// E7/E8's 47.503-vs-28.230 never fold and are pinned on the guard table).
func rnbTwinFoldProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-3", "mid-2", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.101,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rnb-rank",
			Subject: "sysr-8", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
			Rank: 2, Tier: "primary", ChainRelevance: "on_chain", ChainDepth: 2,
			ImpactMS: 28.717, CumulativeImpactMS: 28.230, EffectiveImpactMS: 28.717,
			PriorityInversionCandidate: true, LineStart: 1000, LineEnd: 2000, Confidence: 0.91,
		}},
		// PTV8-RCR-A: running-dominant chain twin (opendir E6/E7 form) — the
		// running component's 原始 is the row's own projection, so the fixture
		// exercises the 行3 breakdown + 拆解子行 caliber marks.
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "rnb-chain",
			Subject: "sysr-8", Predicate: "wakeup_causal_impact", Object: "running",
			StateKind: "running", ChainRelevance: "on_chain", ChainDepth: 2,
			ImpactMS: 28.230, CumulativeImpactMS: 28.230, EffectiveImpactMS: 28.717,
			PriorityInversionCandidate: true, GatedRunnableMS: 28.230, GatedRunningDeficitMS: 0.487,
			RunnableMS: 28.230, LineStart: 1000, LineEnd: 2000, Confidence: 0.78,
		}},
	}
}

// leadSemCrossWindowNoActualProjection exercises the L1 value-less ⚠跨窗
// marker (an out-of-window semantic row with no captured actual) through the
// legend bidirectional harness; the demoted unknown-thread primary keeps the
// background stanza non-empty (an aggregate-metric primary would drag the
// CMP-3 "(cross-thread cumulative…)" suffix into the fence and collide with
// the StanzaCrossThreadCum probe).
func leadSemCrossWindowNoActualProjection() types.TraceCausalProjection {
	out := false
	return types.TraceCausalProjection{
		WindowStartTs:           100.0,
		WindowEndTs:             100.101,
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "lsm-bg",
			Subject: "unknown-thread", Predicate: "root_cause_primary",
			Object: "cpu_pressure",
			Rank:   1, ChainRelevance: "on_chain", ImpactMS: 45.375, Confidence: 0.4,
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "lsm-sem",
			Subject: "Jit thread pool-8870", Predicate: "trace_semantic_span",
			Object: "jit_compile", SpanName: "JIT compiling long com.example.Foo",
			SemanticClass: "jit_compile", ImpactMS: 83.893, CumulativeImpactMS: 83.893,
			StartTs: 99.5, EndTs: 99.584, WithinRequestedWindow: &out,
			LineStart: 500, LineEnd: 600, Confidence: 0.66,
		}},
	}
}

// --- render smoke for the two bidirectional fixture homes (keeps them honest
// --- when the harness fixture list evolves) ----------------------------------

func TestRNBLeadSemBidirectionalFixturesRenderTheirMarks(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rnbTwinFoldProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// PTV8-RCR-A: the fixture exercises the four-line grammar marks (行2 seat
	// + 行3 breakdown + caliber words) instead of the retired R1/R2 tokens.
	// RULE3-1 件2: TOP5 seats badge instead of wording the ordinal — the
	// four-line-grammar probe follows the badge glyph.
	for _, token := range []string{"➋", "ms = ", "(全额)", "按全域最大核最高频"} {
		if !strings.Contains(fence, token) {
			t.Fatalf("rnbTwinFoldProjection must exercise %q:\n%s", token, fence)
		}
	}
	model = buildRuntimeTraceProjTreeModel(leadSemCrossWindowNoActualProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⚠跨窗") || strings.Contains(fence, "实际0.000") {
		t.Fatalf("leadSemCrossWindowNoActualProjection must exercise the value-less marker:\n%s", fence)
	}
}

// --- 复核收尾 (SHIP-WITH-FIXES, 2026-07-07) -----------------------------------

// 复核 W-B: a depthless twin fold must not shrink the P0-A2 unadmitted-
// disclosure MAX — pre-fold the rank row's display magnitude competed in it.
// The count follows the actually-rendered rows (行数诚实: one row post-fold).
func TestRNBDepthlessFoldKeepsUnadmittedDisclosureMax(t *testing.T) {
	// No wakeup path, no chain_depth: both arms land on the depthless lane.
	// The rank arm's display magnitude (28.717) exceeds the chain arm's
	// (12.000); the cumulative accounts agree (W-A guard) and the effective
	// mirror matches, so the pair folds.
	rank := projV3Obs("rnb-dl-rank", "root_cause_primary", "root_cause_primary:dl",
		"depthless-7", "priority_inversion_candidate", "28.717", 28.717, 1000, 2000,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"priority_inversion_candidate=true", "gated_runnable=8.500",
		"effective_impact_ms=8.500")
	rank.RichNotes[1] = "cumulative_impact_ms=12.000"
	rank.Confidence = 0.91
	chain := projV3Obs("rnb-dl-chain", "wakeup_causal_impact", "wakeup_causal_impact:dl",
		"depthless-7", "running", "12.000", 12.0, 1000, 2000,
		"chain_relevance=on_chain", "causality=on_wakeup_chain",
		"dominant_state=running", "priority_inversion_candidate=true",
		"gated_runnable=8.500", "effective_impact_ms=8.500")
	chain.Confidence = 0.78
	md := audit730Render(t, audit730Bus(""), []types.ObservationRecord{rnbAnchor(), rank, chain}, "")
	// PTV8-RCR-A: the fold witness is the merged [E#+E#] bracket.
	if !strings.Contains(md, "+E2]") {
		t.Fatalf("the depthless twin pair must fold:\n%s", md)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 另有 N 项未计入的
	// 链上行 → 另有 N 条链上行未计入上句已归因数值 (归因族 disclosure bullet).
	if !strings.Contains(md, "另有 1 条链上行未计入上句已归因数值(单项最大 28.717ms") {
		t.Fatalf("the disclosure MAX must keep the folded rank row's magnitude (count = rendered rows):\n%s", md)
	}
}

// 复核 M5: the semantic tier-4 FALLBACK lead never claims LeadKey — the flat
// 🎯 anchor lane and the detail-table demotion gate keep their legacy
// behavior (语义 fallback 不冒锚,平铺头保持 unresolved). Positive control
// first: the lane IS engaged on this model.
//
// EVOLUTION RECORD (SEM-LEAD §29.7-2 ①, 2026-07-10): fallback-lane scope
// only — a rank-seated on-chain semantic row resolving through the PRIMARY
// lane claims LeadKey like every crowned lead (SEM-LEAD board/lead pins).
func TestLeadSemSemanticLaneNeverClaimsLeadKey(t *testing.T) {
	projection := leadSemCrossWindowNoActualProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLaneSemanticFallback {
		t.Fatalf("fixture must resolve through the semantic lane (lead=%v lane=%d)", lead, lane)
	}
	if model.LeadKey != "" {
		t.Fatalf("the semantic lane must never claim LeadKey, got %q", model.LeadKey)
	}
}

// 复核建议④: the C00 share-suppression arms of the semantic lead text — a
// non-window-sourced magnitude (attribution / cumulative caliber) never
// publishes a 占窗 share; the window-projection control does.
func TestLeadSemSemanticTextShareSuppressionArms(t *testing.T) {
	model := runtimeTraceProjTreeModel{WindowMS: 101}
	effective := types.TraceCausalProjectionNode{Subject: "t-1", Object: "jit_compile",
		SpanName: "eff-span", EffectiveImpactMS: 9.0}
	if text := runtimeTraceProjSemanticLeadText(effective, model, true); strings.Contains(text, "占窗") || strings.Contains(text, "%") {
		t.Fatalf("an attribution-sourced magnitude must not publish a share: %q", text)
	}
	cumulative := types.TraceCausalProjectionNode{Subject: "t-2", Object: "jit_compile",
		SpanName: "cum-span", CumulativeImpactMS: 9.0}
	if text := runtimeTraceProjSemanticLeadText(cumulative, model, true); strings.Contains(text, "占窗") || strings.Contains(text, "%") {
		t.Fatalf("a cumulative-sourced magnitude must not publish a share: %q", text)
	}
	windowed := types.TraceCausalProjectionNode{Subject: "t-3", Object: "jit_compile",
		SpanName: "win-span", ImpactMS: 50.5}
	if text := runtimeTraceProjSemanticLeadText(windowed, model, true); !strings.Contains(text, "占窗50%") {
		t.Fatalf("a window-projection magnitude must publish its share: %q", text)
	}
}
