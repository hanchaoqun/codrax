package tool

// answer_document_projection_clustertie_test.go — CLUSTERTIE-1 显示半
// (§29.197③ 同因短语上收, 2026-07-21) pin family: the board-level freq_only
// cause hoist. Customer witness (cust_report_xx.txt): the fmax_tie cause
// phrase 「簇最高频并列,核类排序不可判」 repeated verbatim inside 13 fence
// rows' fold parentheticals — the customer read the repetition as total
// failure. The hoist declares the shared cause ONCE on the tree head and the
// rows keep only the legend-taught short 「按频率比」; the detail blocks keep
// the full cause words (无损明细, RULE3-1 件1(c) 同构).

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/types"
)

func clustertieNode(subject string, rank int, reason string, start, end float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: subject,
		Object: "running", StateKind: "running", ChainRelevance: "on_chain",
		Causality: "on_wakeup_chain",
		ImpactMS:  20, EffectiveImpactMS: 5,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 5,
		SupplyFoldIdealMS: 15, SupplyFoldKnownMS: 20, SupplyFoldUnknownMS: 0,
		SupplyFoldCapabilitySource:         runtimeTraceCapabilitySourceFreqOnly,
		SupplyFoldCapabilityFreqOnlyReason: reason,
		Rank:                               rank, Confidence: 0.8,
		StartTs: start, EndTs: end,
	}
}

// clustertieCauseHoistProjection — representative shape: two on-chain running
// seats whose supply folds both degrade freq_only with ONE typed cause
// (fmax_tie) — the hoist population form.
func clustertieCauseHoistProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:              []string{"worker-9", "app-100"},
		WindowStartTs:           925.410,
		WindowEndTs:             925.610,
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			clustertieNode("worker-9", 1, runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.42, 925.46),
			clustertieNode("helper-11", 2, runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.47, 925.50),
			// A fold-less ⛓ seat so the ◎ board's ⛓ glyph is legend-taught
			// (mark-legend 双向); it neither joins nor vetoes the hoist
			// population (no freq_only lane on it).
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "io-dep-13",
				Object: "d_state_or_io_wait", StateKind: "d_sleep", ChainRelevance: "on_chain",
				Causality: "on_wakeup_chain", ImpactMS: 2.0, EffectiveImpactMS: 2.0,
				Rank: 3, Confidence: 0.7, StartTs: 925.51, EndTs: 925.52},
		},
	}
}

// The hoist arm: ≥2 rows, one typed cause — the head declares once, the rows
// drop the cause phrase and keep the short 按频率比 marker; the detail face
// keeps the full cause words.
func TestClustertieCauseHoistDeclaresOnceRowsKeepShortMarker(t *testing.T) {
	for _, zh := range []bool{true, false} {
		proj := clustertieCauseHoistProjection()
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		squashed := partsplitSquash(fence)
		head, cause, short := "本板成因:", "簇最高频并列,核类排序不可判", ",按频率比)"
		if !zh {
			head, cause, short = "board cause:", "cluster peak frequencies tie — class order unjudgeable", ", frequency-ratio basis)"
		}
		if got := strings.Count(squashed, partsplitSquash(head)); got != 1 {
			t.Fatalf("zh=%v: the board-cause declaration must render exactly once, got %d:\n%s", zh, got, fence)
		}
		if got := strings.Count(squashed, partsplitSquash(cause)); got != 1 {
			t.Fatalf("zh=%v: the cause phrase must appear ONLY in the head declaration (rows keep the short marker), got %d:\n%s", zh, got, fence)
		}
		if got := strings.Count(squashed, partsplitSquash(short)); got < 2 {
			t.Fatalf("zh=%v: both fence rows must keep the short frequency-ratio marker, got %d:\n%s", zh, got, fence)
		}
		if !model.Marks.has(runtimeTraceProjMarkFreqOnlyCauseHoisted) {
			t.Fatalf("zh=%v: the hoist mark must record at the head emission site", zh)
		}
		// 无损明细: the detail face keeps the full per-row cause words.
		detail := runtimeTraceProjDetailFullText(model, zh)
		if !strings.Contains(partsplitSquash(detail), partsplitSquash(cause)) {
			t.Fatalf("zh=%v: the detail face must keep the full cause words:\n%s", zh, detail)
		}
	}
}

// Single-row negative arm: one freq_only row hoists nothing — the per-row
// cause phrase stays byte-identical (pre-batch form) and no head line renders.
func TestClustertieCauseHoistSingleRowKeepsInlineCause(t *testing.T) {
	proj := clustertieCauseHoistProjection()
	proj.OnChainCauses = proj.OnChainCauses[:1]
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	squashed := partsplitSquash(fence)
	if strings.Contains(squashed, partsplitSquash("本板成因:")) {
		t.Fatalf("a single freq_only row must not hoist:\n%s", fence)
	}
	if !strings.Contains(squashed, partsplitSquash("簇最高频并列,核类排序不可判,按频率比")) {
		t.Fatalf("the un-hoisted row must keep the full inline cause phrase:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkFreqOnlyCauseHoisted) {
		t.Fatalf("no hoist — no mark")
	}
}

// Mixed-cause negative arm: two rows with two DIFFERENT typed causes never
// hoist (the declaration would be ambiguous — fail-open, rows keep their own
// causes inline).
func TestClustertieCauseHoistMixedCausesFailOpen(t *testing.T) {
	proj := clustertieCauseHoistProjection()
	proj.OnChainCauses[1].SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	squashed := partsplitSquash(fence)
	if strings.Contains(squashed, partsplitSquash("本板成因:")) {
		t.Fatalf("mixed causes must not hoist:\n%s", fence)
	}
	for _, want := range []string{"簇最高频并列,核类排序不可判", "仅单簇有频点采样"} {
		if !strings.Contains(squashed, partsplitSquash(want)) {
			t.Fatalf("each un-hoisted row keeps its own cause %q:\n%s", want, fence)
		}
	}
}

// Legacy reason-less negative arm: two freq_only rows whose wire carries no
// typed cause keep the ruled generic wording byte-identically (absence never
// mints a declaration).
func TestClustertieCauseHoistReasonlessLegacyBytesStand(t *testing.T) {
	proj := clustertieCauseHoistProjection()
	proj.OnChainCauses[0].SupplyFoldCapabilityFreqOnlyReason = ""
	proj.OnChainCauses[1].SupplyFoldCapabilityFreqOnlyReason = ""
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	squashed := partsplitSquash(fence)
	if strings.Contains(squashed, partsplitSquash("本板成因:")) {
		t.Fatalf("reason-less legacy records must not hoist:\n%s", fence)
	}
	if got := strings.Count(squashed, partsplitSquash("簇结构不可判,按频率比")); got < 2 {
		t.Fatalf("legacy rows keep the ruled generic wording, got %d:\n%s", got, fence)
	}
}

// Head width budget (A2 F3 discipline): the declaration line holds the head
// budget on both faces for EVERY closed-set cause.
func TestClustertieCauseHoistHeadWidthBudget(t *testing.T) {
	for _, reason := range runtimeTraceCapabilityFreqOnlyReasonClosedSet {
		if reason == "" {
			continue
		}
		for _, zh := range []bool{true, false} {
			line := "- 本板成因: " + runtimeTraceProjFreqOnlyCauseShort(reason, true) + "——「按频率比」行同此因,板级一次声明,行内不再复读"
			if !zh {
				line = "- board cause: " + runtimeTraceProjFreqOnlyCauseShort(reason, false) + " (stated once)"
			}
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("zh=%v reason=%s: head declaration width %d exceeds the row budget %d: %q",
					zh, reason, w, runtimeTraceProjTreeRowMaxWidth, line)
			}
		}
	}
}
