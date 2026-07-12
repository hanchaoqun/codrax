package tool

// answer_document_projection_rcrc_test.go — PTV8-RCR-C pins (ledger
// docs/design/real_trace_campaign_20260705.md §24.9 维度A 全清单 + §24.12
// C5/C6/C7/C10/C11 + §24.13 裁定二后半 + §24.14 D-5, 2026-07-08):
//
//   G1 — the §20.2 pure-running deficit arm: 行3/子行 give the third
//        closed-set caliber word its structured producer (E8 终形 verbatim);
//        the bare 有效归因X tag dies on the arm; the print-precision identity
//        guard refuses any effective that is NOT the deficit; the 下界 fork
//        follows SupplyFoldUnknownMS.
//   G2 — 行1 词位预留席: the state-composition word survives every width cut
//        (the thread-name head mid-truncates instead); negative — the word is
//        NEVER re-spat below the OwnLine block (the retired #12 revival lane).
//   G3 — 链上L# rides 行2 on structured cause nodes; hop/non-cause chain rows
//        keep the legacy Seg-20 chip.
//   G4 — the NoDeficit two forms: deficit==0 keeps the affirmative sentence
//        byte-identically; 0<deficit<阈 names the deficit (negative: the
//        无供给缺口 literal is banned beside a positive deficit) — plus the
//        UnknownBasis lower-bound fork.
//   G5 — the indicator-table confidence column reads the SAME fold-peer
//        source as the tree 行2 (三面同档).
//   C5 — 承自归因 gate is the precise eff>cum (the 1.1×–1.8× hop-echo band,
//        witness bytes 1.107 > 0.875).
//   C6 — depthless 三面同词 (edge/chip/detail 层级/关系 all speak 父节点未确认;
//        the depth-0 form keeps 深度未解析 byte-identically).
//   C7 — 影响形态 never claims 未分类 beside a typed family token
//        (binder_wait → IO阻塞候选); genuinely word-less rows keep it.
//   C10 — a semantic node's ◇ seat re-bases its % on the typed source window
//        (with the 来自查询窗 tag) and never double-prints the class token.
//   C11 — 睡眠症状 note is guidance-or-nothing; the unseated cause node's
//        detail line wears 置信, never a bare-rank 根因排序 label; the
//        same-tid dual-name pair matches (tid-first) and is DECLARED.
//   裁定二后半 — multi-board rank seats carry 窗X–Ys; the single-board form
//        carries none.
//   D-5 — 目标睡眠/目标症状时长 retired at the P0-A2 compare face.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// rcrcRunningDeficitProjection mirrors opendir_78 E8 (§24.9 gap②): a pure
// running chain node whose engine effective IS the supply-fold deficit
// (0.186 = deficit; raw running ideal+deficit = 2.641 — the ⚠实际 magnitude).
func rcrcRunningDeficitProjection(unknownMS float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"RenderThread-16867", ".ugc.aweme.lite-16547"},
		WindowStartTs: 33872.289,
		WindowEndTs:   33872.409,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rcrc-e8",
			Subject: "RenderThread-16867", Predicate: "root_cause_primary",
			Object: "running", StateKind: "running",
			Rank: 11, ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 1.096, CumulativeImpactMS: 1.096, EffectiveImpactMS: 0.186,
			ActualImpactMS:     2.641,
			SupplyFoldComputed: true, SupplyFoldDeficitMS: 0.186,
			SupplyFoldIdealMS: 2.455, SupplyFoldKnownMS: 2.641 - unknownMS,
			SupplyFoldUnknownMS: unknownMS,
			LineStart:           45689, LineEnd: 79142, Confidence: 0.9,
		}},
	}
}

// --- G1: the §20.2 running-deficit arm (E8 终形 verbatim, §24.9 fix【1】) ------

func TestRCRCRunningDeficitArmE8TerminalForm(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrcRunningDeficitProjection(0), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"⚙ RenderThread-16867 · running",
		"· 算力供给候选·根因排序#11·置信高·链上L1",
		"· 有效归因 0.186ms = running(折算,按大核满频) 0.186ms",
		"· running 原始 2.641ms → 计入 0.186ms(折算,按大核满频)",
		"接近大核满频,缺口仅 0.186ms(已计入有效归因)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("E8 terminal form missing %q:\n%s", want, fence)
		}
	}
	// The legacy bare tag (no space, no "=", no caliber) is dead on the arm,
	// and the small-deficit supply note never denies the number beside it.
	for _, banned := range []string{"有效归因0.186ms", "无供给缺口"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("retired form %q resurfaced on the running-deficit node:\n%s", banned, fence)
		}
	}
	// §24.1 identity: 行1 keeps the window projection; the effective lives on
	// 行3 only.
	if !strings.Contains(fence, "1.096ms") {
		t.Fatalf("行1 must keep the window projection 1.096ms:\n%s", fence)
	}
	// §24.1补: the caliber legend entries render exactly on demand.
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `按大核满频折算` =") {
		t.Fatalf("按大核满频 legend entry must render with the arm:\n%s", legend)
	}
	if strings.Contains(legend, "- `下界` =") {
		t.Fatalf("下界 entry must stay off the fully-known basis:\n%s", legend)
	}
}

func TestRCRCRunningDeficitArmLowerBoundFork(t *testing.T) {
	// unknown>0: the sub-row caliber grows the 下界 word, the supply note
	// takes the UnknownBasis lower-bound fork (never "无法折算" beside the
	// published deficit), and the 下界 legend entry renders.
	model := buildRuntimeTraceProjTreeModel(rcrcRunningDeficitProjection(0.5), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"· running 原始 2.641ms → 计入 0.186ms(折算,按大核满频,下界)",
		"CPU 频率数据部分缺失,已计部分按大核满频折算:缺口 0.186ms 为下界",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("lower-bound fork missing %q:\n%s", want, fence)
		}
	}
	if strings.Contains(fence, "无法折算") {
		t.Fatalf("the incomplete-data sentence must not deny the published deficit:\n%s", fence)
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `下界` =") {
		t.Fatalf("下界 legend entry must render with the fork:\n%s", legend)
	}
}

func TestRCRCRunningDeficitArmIdentityGuard(t *testing.T) {
	// Mutation guard (§24.1 恒等式): an effective that is NOT the deficit at
	// print precision must never wear the 按大核满频 equation — the arm
	// refuses and the row falls open to the legacy tag.
	projection := rcrcRunningDeficitProjection(0)
	projection.OnChainCauses[0].EffectiveImpactMS = 0.187
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "= running(折算,按大核满频)") {
		t.Fatalf("a non-deficit effective must never wear the fmax equation:\n%s", fence)
	}
	if !strings.Contains(fence, "有效归因0.187ms") {
		t.Fatalf("the identity guard must fail open to the legacy tag:\n%s", fence)
	}
	// The gate itself is typed: no supply fold → no arm.
	node := rcrcRunningDeficitProjection(0).OnChainCauses[0]
	node.SupplyFoldComputed = false
	if runtimeTraceProjCauseRunningDeficitArm(node) {
		t.Fatalf("the arm must require SupplyFoldComputed")
	}
	inversion := rcrcRunningDeficitProjection(0).OnChainCauses[0]
	inversion.PriorityInversionCandidate = true
	if runtimeTraceProjCauseRunningDeficitArm(inversion) {
		t.Fatalf("the arm must exclude inversion composites")
	}
}

// --- G4: the NoDeficit two forms --------------------------------------------

func TestRCRCNoDeficitTwoForms(t *testing.T) {
	// deficit == 0: the affirmative sentence, byte-identical to the pre-C
	// wording.
	zero := rcrcRunningDeficitProjection(0).OnChainCauses[0]
	zero.SupplyFoldDeficitMS = 0
	zero.SupplyFoldIdealMS = 2.641
	zero.EffectiveImpactMS = 0
	clause, _, ok := runtimeTraceProjSupplyFoldClause(zero, 120, true)
	// EVOLUTION RECORD (UXR-1 §29.36.4 ①): the affirmative implication chain
	// compressed to 证据+末端结论 (the legend carries the expanded semantics).
	if !ok || clause != "已按大核满频(或接近)运行·无供给折算" {
		t.Fatalf("deficit==0 must keep the compressed affirmative form: %q", clause)
	}
	// 0 < deficit < 阈 with eff==deficit (§20.2): the counted fork.
	counted := rcrcRunningDeficitProjection(0).OnChainCauses[0]
	clause, keep, ok := runtimeTraceProjSupplyFoldClause(counted, 120, true)
	if !ok || clause != "接近大核满频,缺口仅 0.186ms(已计入有效归因)" || keep != "接近大核满频" {
		t.Fatalf("small-deficit counted fork drifted: %q / %q", clause, keep)
	}
	// 0 < deficit < 阈 with eff≠deficit: the independent-caliber fork — the
	// sentence still names the number, never denies it.
	independent := counted
	independent.EffectiveImpactMS = 1.096
	clause, _, ok = runtimeTraceProjSupplyFoldClause(independent, 120, true)
	if !ok || clause != "接近大核满频,缺口仅 0.186ms(独立口径,不计入有效归因)" {
		t.Fatalf("small-deficit independent fork drifted: %q", clause)
	}
	// Negative (§24.9 F3): the 无供给缺口 literal is banned beside ANY
	// positive deficit.
	for _, node := range []types.TraceCausalProjectionNode{counted, independent} {
		clause, _, _ := runtimeTraceProjSupplyFoldClause(node, 120, true)
		if strings.Contains(clause, "无供给缺口") {
			t.Fatalf("无供给缺口 must never render beside a positive deficit: %q", clause)
		}
	}
}

// --- G2: the 行1 reserved seat + the OwnLine re-spit negative ------------------

// rcrcReservedSeatProjection is the opendir_78 E4 shape: a LONG pid-tailed
// thread name whose composed 行1 (subject · runnable+running) overflows the
// name budget — pre-C the composition word was cut and re-spat by the #12
// guarantee as a stray sixth line ("· runnable+running · 链上L1").
func rcrcReservedSeatProjection() types.TraceCausalProjection {
	holder := "#RxComputationVeryLongThreadName-16816"
	target := ".ugc.aweme.lite-16547"
	return types.TraceCausalProjection{
		WakeupPath:    []string{holder, target},
		WindowStartTs: 33872.289,
		WindowEndTs:   33872.409,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "rcrc-e4",
			Subject: holder, Predicate: "wakeup_causal_impact", Object: "running",
			StateKind: "running", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 58.919, CumulativeImpactMS: 58.919, EffectiveImpactMS: 37.410,
			PriorityInversionCandidate: true,
			GatedRunnableMS:            20.713, GatedRunningDeficitMS: 16.697, RunnableMS: 20.713,
			Rank: 2, Confidence: 0.91,
		}},
	}
}

func TestRCRCReservedSeatSurvivesWidthPressure(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrcReservedSeatProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var row1 string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "58.919ms") && strings.Contains(line, "█") {
			row1 = line
			break
		}
	}
	if row1 == "" {
		t.Fatalf("E4-shape 行1 not found:\n%s", fence)
	}
	// The grammar word holds its reserved seat on 行1; the pid tail survives
	// the mid-truncation (T2) — only the name head gave way.
	// EVOLUTION RECORD (GAP-B G7 词值同源, §27.3, 2026-07-09): the reserved
	// word is now the window lane's dominant state (running — the fixture's
	// StateKind), not the gated composition; the composition lives on 行3.
	if !strings.Contains(row1, " · running") {
		t.Fatalf("行1 must keep the reserved state word: %q", row1)
	}
	if !strings.Contains(row1, "…-16816") {
		t.Fatalf("the thread-name head must mid-truncate keeping the pid tail: %q", row1)
	}
	// §24.9 突变 pin: 宽度受压链上成因行绝不在 OwnLine 块下方复吐构成词 — no
	// subordinate line may re-spit the bare word (the 行3 拆写形
	// runnable(全额)… and the 行4 "· running 原始 …" sub-row are different,
	// legal shapes — the re-spit form is the BARE word line, alone or followed
	// by a "·" tag joiner).
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "· running" || strings.HasPrefix(trimmed, "· running ·") {
			t.Fatalf("the state word re-spat below the OwnLine block: %q\n%s", line, fence)
		}
	}
	// The four-line grammar itself is intact under width pressure.
	for _, want := range []string{
		"· 优先级反转候选·根因排序#2·置信高·链上L1",
		"· 有效归因 37.410ms = runnable(全额) 20.713ms + running(折算) 16.697ms",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("four-line grammar lost %q under width pressure:\n%s", want, fence)
		}
	}
}

func TestRCRCBoundaryTruncatePlusDefense(t *testing.T) {
	// §24.9 G2 防御半: '+' joins the boundary set — a composition word that
	// ever falls back onto the boundary cut keeps a readable head instead of
	// vanishing whole.
	if got := runtimeTraceProjBoundaryTruncate("runnable+running", 9); got != "runnable…" {
		t.Fatalf("BoundaryTruncate must cut at '+': %q", got)
	}
}

// --- G3: 链上L# rides 行2; hop rows keep the Seg-20 chip -----------------------

func TestRCRCChainDepthChipConsolidation(t *testing.T) {
	// Structured cause node (the running-deficit fixture): the layer word is
	// INSIDE 行2 and the standalone chip line is gone.
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(rcrcRunningDeficitProjection(0), newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(fence, "·置信高·链上L1") {
		t.Fatalf("行2 must carry the chain layer:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "· 链上L1" || strings.HasPrefix(trimmed, "· 链上L1 ·") {
			t.Fatalf("the standalone Seg-20 chip must not double-print on a structured row: %q", line)
		}
	}
	// Hop (non-cause) chain row keeps the legacy chip.
	hop := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "rcrc-hop",
			Subject: "worker-9", Predicate: "wakeup_causal_impact", Object: "s_sleep",
			StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 12.0, Confidence: 0.8,
		}},
	}
	hopFence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(hop, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(hopFence, "链上L1") {
		t.Fatalf("hop rows must keep the legacy chip:\n%s", hopFence)
	}
}

// --- G5 + C-4/§24.11: the confidence column reads the fold-peer source --------

func TestRCRCConfidenceSingleSourceAcrossFaces(t *testing.T) {
	// The rcr opendir fixture's E7 row folds the rank twin (confidence 0.91 →
	// 高) while node.Confidence is 0.78 (中): tree 行2, detail block and the
	// indicator table must all say 高.
	projection := rcrOpendirProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "优先级反转候选·根因排序#2·置信高") {
		t.Fatalf("tree 行2 must speak the fold-peer confidence:\n%s", fence)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	var e7 []string
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "[E7(+1)+E8]") || strings.Contains(row.Cells[0], "E7") {
			e7 = row.Cells
			break
		}
	}
	if e7 == nil {
		t.Fatalf("E7 indicator row not found")
	}
	if got := e7[len(e7)-1]; got != "高" {
		t.Fatalf("indicator confidence column must read the fold-peer source (高), got %q", got)
	}
}

// --- C5: the precise inherited gate (eff > cum) --------------------------------

func TestRCRCInheritedGatePreciseEffAboveCum(t *testing.T) {
	// cmp_78_01 witness bytes: E7 hop eff 1.107 > own 0.875 (1.27× — inside
	// the band the 10× ratio waved through).
	hop := func(eff float64) types.TraceCausalProjection {
		return types.TraceCausalProjection{
			WakeupPath:    []string{"LaunchPoolT7-6711", "main-6565"},
			WindowStartTs: 100.0,
			WindowEndTs:   101.8,
			OnChainCauses: []types.TraceCausalProjectionNode{{
				Role: types.TraceCausalRoleCausalHop, EvidenceID: "rcrc-c5",
				Subject: "LaunchPoolT7-6711", Predicate: "wakeup_causal_impact",
				Object: "s_sleep", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ChainDepth: 2,
				ImpactMS: 0.875, CumulativeImpactMS: 0.875, EffectiveImpactMS: eff,
				Confidence: 0.8,
			}},
		}
	}
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(hop(1.107), newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(fence, "承自归因1.107ms") {
		t.Fatalf("eff>cum must wear 承自归因:\n%s", fence)
	}
	if strings.Contains(fence, "有效归因1.107ms") {
		t.Fatalf("the bare 有效归因 word must not ship on an inherited magnitude:\n%s", fence)
	}
	// Below the cum: the plain Q1 tag stays.
	below := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(hop(0.800), newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(below, "有效归因0.800ms") || strings.Contains(below, "承自归因") {
		t.Fatalf("eff<cum must keep the plain tag:\n%s", below)
	}
	// Print-equal stays non-inherited (one measurement — the PTV6-D (c) fold).
	if runtimeTraceProjEffectiveInherited(types.TraceCausalProjectionNode{
		EffectiveImpactMS: 1.0004, CumulativeImpactMS: 1.0001}) {
		t.Fatalf("print-equal values must stay non-inherited")
	}
}

// --- C6 + 裁定二后半: the multi-board unattached fixture ------------------------

// rcrcMultiBoardUnattachedProjection carries (a) a trunk rank row in the
// anchor query window, (b) a depthless DEPTH-KNOWN row (ChainDepth 2, no
// attach point) ranked #1 on a DIFFERENT query window — the #1×2 collision
// plus the unattached 三面同词 shape. Registered as a revisit76 sentinel
// fixture (ChainSeatUnattached / RankSeatWindow marks).
func rcrcMultiBoardUnattachedProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rcrc-b1",
				Subject: "worker-9", Predicate: "root_cause_primary",
				Object: "runnable_wait", StateKind: "runnable",
				Rank: 1, ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 30, CumulativeImpactMS: 30, EffectiveImpactMS: 30,
				QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.2, Confidence: 0.9},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "rcrc-b2",
				Subject: "detached-7", Predicate: "root_cause_primary",
				Object: "io_latency",
				Rank:   1, ChainRelevance: "on_chain", ChainDepth: 2,
				ImpactMS: 12, CumulativeImpactMS: 12, EffectiveImpactMS: 12,
				QueryWindowStartTs: 100.3, QueryWindowEndTs: 100.4, Confidence: 0.8},
		},
	}
}

func TestRCRCDepthlessUnattachedThreeFacesOneWord(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrcMultiBoardUnattachedProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// Edge face.
	// EVOLUTION RECORD (UXR-1 §29.36④, 2026-07-11): the lane prefix
	// simplified to 链上─ — the 父节点未确认 auxiliary word lives on the 行2 chip
	// + detail faces only (the C6 word family keeps two faces).
	if !strings.Contains(fence, "链上─") {
		t.Fatalf("the simplified on-chain edge must render:\n%s", fence)
	}
	if strings.Contains(fence, "链上·父节点未确认─") {
		t.Fatalf("the unattached auxiliary word must leave the lane prefix (§29.36④):\n%s", fence)
	}
	// Chip face (行2 of the structured cause row).
	if !strings.Contains(fence, "链上L2(父节点未确认)") {
		t.Fatalf("the 行2 chip must speak the unified word:\n%s", fence)
	}
	// Detail faces (层级 + 关系).
	detail := runtimeTraceProjDetailFullText(model, true)
	for _, want := range []string{"链上L2(父节点未确认)", "链上·父节点未确认"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail face lost the unified word %q:\n%s", want, detail)
		}
	}
	// The depth-KNOWN row never claims 深度未解析 / 未接入链 anywhere.
	for _, banned := range []string{"链上·深度未解析", "未接入链", "深度2"} {
		if strings.Contains(fence, banned) || strings.Contains(detail, banned) {
			t.Fatalf("forked caliber %q resurfaced:\n%s\n%s", banned, fence, detail)
		}
	}
	// Depth-0 control: the legacy 深度未解析 family stays byte-identical.
	zeroDepth := rcrcMultiBoardUnattachedProjection()
	zeroDepth.OnChainCauses[1].ChainDepth = 0
	zeroDepth.OnChainCauses[1].Rank = 0
	zeroDepth.OnChainCauses[1].Predicate = "wakeup_causal_impact"
	zeroModel := buildRuntimeTraceProjTreeModel(zeroDepth, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	zeroFence := runtimeTraceProjTreeFence(zeroModel, true)
	// UXR-1 §29.36④: the depth-0 form keeps the 深度未解析 word on 行2.
	if !strings.Contains(zeroFence, "链上·深度未解析") {
		t.Fatalf("the depth-0 form must keep 深度未解析 on 行2:\n%s", zeroFence)
	}
	if strings.Contains(zeroFence, "父节点未确认") {
		t.Fatalf("the depth-0 form must not claim the unattached word:\n%s", zeroFence)
	}
}

func TestRCRCMultiBoardRankSeatsCarryWindowTags(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrcMultiBoardUnattachedProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// Both #1 seats name their boards — the collision is resolved.
	for _, want := range []string{
		"根因排序#1·窗100.000–100.200s",
		"根因排序#1·窗100.300–100.400s",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("multi-board seat must carry its window tag %q:\n%s", want, fence)
		}
	}
	// The detail 根因排序 line carries the same chip.
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "#1·窗100.300–100.400s") {
		t.Fatalf("the detail seat must carry the window tag:\n%s", detail)
	}
	// Single-board negative: one window (or windowless boards) never mints
	// the tag — the pre-C byte form survives.
	single := rcrcMultiBoardUnattachedProjection()
	single.OnChainCauses[1].QueryWindowStartTs = 100.0
	single.OnChainCauses[1].QueryWindowEndTs = 100.2
	singleFence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(single, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if strings.Contains(singleFence, "·窗100") {
		t.Fatalf("the single-board form must not carry window tags:\n%s", singleFence)
	}
	windowless := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(rcrOpendirProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if strings.Contains(windowless, "·窗") {
		t.Fatalf("windowless boards must not carry window tags:\n%s", windowless)
	}
}

// --- C7: 影响形态 never impersonates 未分类 beside a typed family token --------

func TestRCRCImpactShapeFamilyWordNeverUnclassified(t *testing.T) {
	binder := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-6565"},
		WindowStartTs: 100.0,
		WindowEndTs:   101.8,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rcrc-c7",
			Subject: "worker-9", Predicate: "root_cause_primary", Object: "binder_wait",
			TypeToken: "binder_wait",
			Rank:      1, ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 3.843, CumulativeImpactMS: 3.843, EffectiveImpactMS: 3.843,
			Confidence: 0.4,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(binder, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	detail := runtimeTraceProjDetailFullText(model, true)
	// PTV8-RCR-C 复核收尾 (2026-07-08). EVOLUTION RECORD: binder_wait left the
	// ⛓ IOBlock family (IPC wait-on-peer 等待症状族 ≠ block IO; typelabels
	// binder等待) — its own §24.3 table row speaks binder等待候选, and the row
	// glyph borrows the ◦ 无形态兜底 (a dedicated IPC glyph awaits a ruling).
	if !strings.Contains(detail, "影响形态: binder等待候选") {
		t.Fatalf("binder_wait must take its own family word:\n%s", detail)
	}
	for _, banned := range []string{"IO阻塞候选", "未分类"} {
		if strings.Contains(detail, banned) {
			t.Fatalf("%q must never impersonate a binder IPC row:\n%s", banned, detail)
		}
	}
	if strings.Contains(fence, "⛓") {
		t.Fatalf("a binder IPC row must not wear the ⛓ block-IO glyph:\n%s", fence)
	}
	// Direct helper pins: the blind-spot family speaks its data-gap word; a
	// token-less node keeps the honest generic arm.
	if got := runtimeTraceProjImpactFormFamilyWord(types.TraceCausalProjectionNode{Predicate: "trace_gap"}, true); got != "数据盲区(窗内数据缺口,非成因)" {
		t.Fatalf("trace_gap family word = %q", got)
	}
	if got := runtimeTraceProjImpactFormFamilyWord(types.TraceCausalProjectionNode{Object: "trace_span"}, true); got != "" {
		t.Fatalf("a family-less token must keep the generic arm: %q", got)
	}
}

// --- G1 × §24.2: the running-deficit form outranks the event form --------------

// TestRCRCRunningDeficitBeatsEventFold (复核收尾 fix-2): a merged running node
// whose per-instance MAX coincides with the supply-fold deficit
// (MergedCount>1 ∧ MergedMaxMS == eff == deficit) is a §20.2 node FIRST — the
// fmax equation renders, ×N never rises into 行1 and the 单次最大 caliber
// never impersonates the deficit (one gate ordering: CauseEventFoldRow's
// running-deficit exclusion, mutation-replayed red).
func TestRCRCRunningDeficitBeatsEventFold(t *testing.T) {
	projection := rcrcRunningDeficitProjection(0)
	projection.OnChainCauses[0].MergedCount = 3
	projection.OnChainCauses[0].MergedMinMS = 0.050
	projection.OnChainCauses[0].MergedMaxMS = 0.186 // == eff == deficit
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "· 有效归因 0.186ms = running(折算,按大核满频) 0.186ms") {
		t.Fatalf("the fmax equation must win the coincidence: \n%s", fence)
	}
	if strings.Contains(fence, "单次最大") {
		t.Fatalf("the 单次最大 caliber must never impersonate a §20.2 deficit:\n%s", fence)
	}
	var row1 string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "1.096ms") && strings.Contains(line, "█") {
			row1 = line
			break
		}
	}
	if row1 == "" || strings.Contains(row1, "×3") {
		t.Fatalf("×N must not rise into 行1 on the deficit form: %q\n%s", row1, fence)
	}
}

// --- G2 corner: the floor-clamped budget still holds the reserved seat ---------

// TestRCRCReservedSeatFloorClampCorner (复核收尾 fix-3, M4 角形): a fixed part
// wide enough to clamp the name budget onto the erosion floor still returns
// head+keep whole — the grammar word never yields, the pid tail survives the
// 8-cell identity floor.
func TestRCRCReservedSeatFloorClampCorner(t *testing.T) {
	projection := rcrcReservedSeatProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	row := model.TreeRows[0]
	name := runtimeTraceProjRowName(row, true)
	keep := runtimeTraceProjRowNameKeepSuffix(row, true)
	// EVOLUTION RECORD (GAP-B G7, 2026-07-09): the reserved grammar word is
	// the window lane's dominant state now (词值同源).
	if keep != " · running" {
		t.Fatalf("keep suffix = %q", keep)
	}
	// A 60-cell fixed part clamps the budget to the erosion floor (≥8): the
	// fitted name must still end with the whole grammar word and keep the pid
	// tail on the mid-truncated head.
	fitted := runtimeTraceProjRowNameFitted(60, row, name, true)
	if !strings.HasSuffix(fitted, keep) {
		t.Fatalf("floor-clamped fit must keep the grammar word whole: %q", fitted)
	}
	head := strings.TrimSuffix(fitted, keep)
	if !strings.HasSuffix(head, "-16816") {
		t.Fatalf("the pid identity tail must survive the floor clamp: %q", fitted)
	}
}

// --- C10: the semantic ◇ seat re-bases and single-prints the class -------------

func rcrcSemanticAdjacentProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-6565"},
		WindowStartTs: 100.0,
		WindowEndTs:   101.8, // 1800ms anchor
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rcrc-chain",
			Subject: "worker-9", Object: "running_burst", StateKind: "running",
			ChainRelevance: "on_chain", ImpactMS: 30, Confidence: 0.8,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "rcrc-span",
			Subject: "binder:8815_1-6581", Predicate: "trace_semantic_span",
			SemanticClass: "class_verification", SpanName: "VerifyClassLoaderContextMatch",
			ChainRelevance: "adjacent", ImpactMS: 0.672,
			QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.081, // 81ms source
			Confidence: 0.82,
		}},
	}
}

func TestRCRCSemanticAdjacentSeatWindowBaseAndSingleClass(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrcSemanticAdjacentProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var seat string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "0.672ms") {
			seat = line
			break
		}
	}
	if seat == "" {
		t.Fatalf("◇ seat not found:\n%s", fence)
	}
	// % re-based on the 81ms source window (0.672/81 ≈ 1%), never the 1800ms
	// anchor (0%), with the 来自查询窗 disclosure riding the row.
	if !strings.Contains(seat, " 1%") {
		t.Fatalf("the ◇ seat %% must re-base on the typed source window: %q", seat)
	}
	if !strings.Contains(fence, "来自查询窗") {
		t.Fatalf("the source-window disclosure must ride the ◇ seat:\n%s", fence)
	}
	// The class token prints ONCE (the shape tag carries it; the action cell
	// trims to the bare word on every seat of a semantic node).
	if strings.Contains(fence, "优化点·类校验") {
		t.Fatalf("the class token double-print resurfaced:\n%s", fence)
	}
	if !strings.Contains(fence, "语义优化span·类校验") {
		t.Fatalf("the shape tag must keep the class token once:\n%s", fence)
	}
}

// --- C11: the sleep-note unification / unseated 置信 line / dual-name tid ------

func TestRCRCSleepNoteGuidanceOrNothing(t *testing.T) {
	// Guidance renders exactly when a known upstream exists to chase.
	guided := types.TraceCausalProjectionNode{StateKind: "s_sleep", DrilldownTarget: "worker-9"}
	if word, _ := runtimeTraceCausalProjectionActionCellWithFamily(guided, true); word != "睡眠症状→查上游" {
		t.Fatalf("guided sleep note = %q", word)
	}
	// The bare restatement and the ·缺唤醒边 variant are retired.
	bare := types.TraceCausalProjectionNode{StateKind: "s_sleep"}
	if word, _ := runtimeTraceCausalProjectionActionCellWithFamily(bare, true); word != "" {
		t.Fatalf("the bare 睡眠症状 restatement must be retired: %q", word)
	}
	undrillable := types.TraceCausalProjectionNode{StateKind: "s_sleep", UndrillableReason: "missing_wakeup"}
	if word, _ := runtimeTraceCausalProjectionActionCellWithFamily(undrillable, true); word != "" {
		t.Fatalf("the ·缺唤醒边 variant must be retired (⊘链止 carries the fact): %q", word)
	}
}

func TestRCRCUnseatedCauseNodeConfidenceLabel(t *testing.T) {
	// A rank-less inversion cause node (cmp_78_01 E6 shape): the detail line
	// wears the 置信 label — never 「根因排序: 置信中」.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "rcrc-c11b",
			Subject: "worker-9", Predicate: "wakeup_causal_impact",
			Object: "running", StateKind: "running",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 5.769, CumulativeImpactMS: 5.769, EffectiveImpactMS: 1.280,
			PriorityInversionCandidate: true,
			GatedRunnableMS:            0.748, GatedRunningDeficitMS: 0.532,
			Confidence: 0.7,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_ = runtimeTraceProjTreeFence(model, true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "置信: 中") {
		t.Fatalf("the unseated cause node must wear the bare 置信 label:\n%s", detail)
	}
	if strings.Contains(detail, "根因排序: 置信") {
		t.Fatalf("the empty-seat 根因排序 label form must be retired:\n%s", detail)
	}
}

func TestRCRCSameTidDualNameMatchAndDeclaration(t *testing.T) {
	// §11-N7 tid-first: both sides expose a -pid tail → tid decides.
	if !runtimeTraceProjTargetMatchesUserEntities("main-6565", []string{"com.xs.fm.lite-6565"}) {
		t.Fatalf("tid-equal dual names must match")
	}
	if runtimeTraceProjTargetMatchesUserEntities("main-6565", []string{"com.xs.fm.lite-21538"}) {
		t.Fatalf("different tids must not match")
	}
	if got := runtimeTraceProjTargetUserEntityAlias("main-6565", []string{"com.xs.fm.lite-6565"}); got != "com.xs.fm.lite-6565" {
		t.Fatalf("alias = %q", got)
	}
	if got := runtimeTraceProjTargetUserEntityAlias("main-6565", []string{"main-6565"}); got != "" {
		t.Fatalf("same-name entities never declare: %q", got)
	}
	// Render half: the header keeps ‹用户关注线程› and declares the pair once.
	projection := rcrcSemanticAdjacentProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"com.xs.fm.lite-6565"}})
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "‹用户关注线程›") {
		t.Fatalf("the tid-matched root must keep the user-focus label:\n%s", fence)
	}
	if !strings.Contains(fence, "- main-6565 即你指定的 com.xs.fm.lite-6565(同一 tid 的双名,已归一)") {
		t.Fatalf("the dual-name normalization must be declared:\n%s", fence)
	}
	// No alias → no declaration line (byte-identical control).
	control := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjApplyUserFocus(&control, runtimeTraceProjUserFocus{Entities: []string{"main-6565"}})
	if fence := runtimeTraceProjTreeFence(control, true); strings.Contains(fence, "双名") {
		t.Fatalf("a same-name match must not declare:\n%s", fence)
	}
}

// --- D-5: the P0-A2 compare face retires the 目标 words -------------------------

func TestRCRCCompareFaceRetiresTargetWords(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rcrHopOnlyProjection(112.175, 100.000), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// COV-2 (§24.14 B-1, 2026-07-08). EVOLUTION RECORD: cell returns typed arm
	// + window base alongside the text (wording unchanged on this face).
	cell, _, _ := runtimeTraceProjCompareTargetSymptomCell(rcrHopOnlyProjection(112.175, 100.000), model, true)
	if !strings.Contains(cell, "唤醒链采样到的关注线程睡眠合计") {
		t.Fatalf("the hop-view cell must speak the 关注线程 family: %q", cell)
	}
	block := runtimeTraceProjCompareOverviewBlock([]types.TraceCausalProjection{
		rcrHopOnlyProjection(112.175, 100.000), rcrOpendirProjection(),
	}, types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("compare overview must render")
	}
	var faces []string
	faces = append(faces, block.Text)
	faces = append(faces, block.Columns...)
	for _, item := range block.Items {
		faces = append(faces, item.Label, item.Text)
		faces = append(faces, item.Cells...)
	}
	joined := strings.Join(faces, "\n")
	if !strings.Contains(joined, "关注线程症状时长") {
		t.Fatalf("the compare header must speak the 关注线程 family:\n%s", joined)
	}
	for _, banned := range []string{"目标睡眠", "目标症状时长"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("retired word %q shipped on the compare face:\n%s", banned, joined)
		}
	}
}

// --- C-新5: the semantic relation names the HOST thread ------------------------

func TestRCRCSemanticRelationNamesHost(t *testing.T) {
	// An orphan semantic span hosted on a FOREIGN thread hangs off the ⊚
	// anchor (Parent = main-6565): the relation must name the HOST, never the
	// tree attach anchor (cmp_78_01 E41/E42 pseudo-binding).
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "main-6565"},
		WindowStartTs: 100.0,
		WindowEndTs:   101.8,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rcrc-chain2",
			Subject: "worker-9", Object: "running_burst", StateKind: "running",
			ChainRelevance: "on_chain", ImpactMS: 30, Confidence: 0.8,
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "rcrc-span2",
			Subject: "binder:8815_1-6581", Predicate: "trace_semantic_span",
			SemanticClass: "class_verification", SpanName: "VerifyClassLoaderContextMatch",
			ImpactMS: 0.672, Confidence: 0.82,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_ = runtimeTraceProjTreeFence(model, true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "binder:8815_1-6581 的语义span") {
		t.Fatalf("the semantic relation must name the host thread:\n%s", detail)
	}
	if strings.Contains(detail, "main-6565 的语义span") {
		t.Fatalf("the tree-anchor pseudo-binding resurfaced:\n%s", detail)
	}
}
