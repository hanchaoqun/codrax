package tool

// answer_document_projection_cap_test.go — CAP batch (§26, docs/design/
// real_trace_campaign_20260705.md, 2026-07-08) display pins:
//
//	C3 三态披露 — the typed capability-source token forks the supply-fold
//	   clause wording (default_table → 按默认算力比粗算; freq_only →
//	   簇结构不可判,按纯频率比折算; empty/pre-CAP → byte-stable legacy);
//	   the R5d discounted component and the inversion detail line carry the
//	   same disclosure; the G1 running-deficit arm's sub-row caliber grows it.
//	C3 图例 — the 下界 entry evolved (核类算力差已计入; the retired pre-CAP
//	   half-sentence's negative pin lives in TestRCRCaliberLegendEntriesOn
//	   Demand) and the two new disclosure words teach through their own
//	   legend seats exactly on demand.
//	Wire-token drift pin — the display-side mirrors of tracequery's
//	   CoreCapabilitySource* constants must stay byte-identical.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- wire-token drift pin ----------------------------------------------------

func TestCAPWireTokensMirrorEngine(t *testing.T) {
	if runtimeTraceCapabilitySourceDefault != tracequery.CoreCapabilitySourceDefault ||
		runtimeTraceCapabilitySourceEvidence != tracequery.CoreCapabilitySourceEvidence ||
		runtimeTraceCapabilitySourceFreqOnly != tracequery.CoreCapabilitySourceFreqOnly {
		t.Fatalf("display capability tokens drifted from the engine constants (core_capability.go)")
	}
}

// --- C3: the supply-fold clause's three-state disclosure ----------------------

func capClauseNode(deficit, ideal, known, unknown, effective float64, source string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
		Object: "running", StateKind: "running", ChainRelevance: "on_chain",
		ImpactMS: known + unknown, EffectiveImpactMS: effective,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: deficit,
		SupplyFoldIdealMS: ideal, SupplyFoldKnownMS: known, SupplyFoldUnknownMS: unknown,
		SupplyFoldCapabilitySource: source, Confidence: 0.8,
	}
}

func TestCAPSupplyFoldClauseThreeStateDisclosure(t *testing.T) {
	dominant := func(source string) string {
		clause, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(5, 15, 20, 0, 5, source), 0, true)
		if !ok {
			t.Fatalf("dominant clause must render")
		}
		return clause
	}
	// default_table: the parenthetical grows the coarse-estimate disclosure.
	if got := dominant(runtimeTraceCapabilitySourceDefault); !strings.Contains(got, "(按大核满频折算,下界,按默认算力比粗算)") {
		t.Fatalf("default-table fold must disclose 按默认算力比粗算:\n%s", got)
	}
	// freq_only: the fail-loud disclosure names what was NOT priced.
	if got := dominant(runtimeTraceCapabilitySourceFreqOnly); !strings.Contains(got, "(按大核满频折算,下界,簇结构不可判,按纯频率比折算)") {
		t.Fatalf("freq_only fold must disclose 簇结构不可判,按纯频率比折算:\n%s", got)
	}
	// empty (pre-CAP record): byte-stable legacy wording, no capability claim.
	got := dominant("")
	if !strings.Contains(got, "(按大核满频折算,下界)") {
		t.Fatalf("pre-CAP record must keep the legacy parenthetical byte-identically:\n%s", got)
	}
	if strings.Contains(got, "算力") || strings.Contains(got, "簇结构") {
		t.Fatalf("a record without the typed token must claim nothing:\n%s", got)
	}
}

// G4 two forms + affirmative (§26 判词重判 display half): the near-fmax and
// the affirmative sentences carry the capability caliber — under freq_only
// the "已按大核满频…无供给缺口" claim is explicitly frequency-only.
func TestCAPNoDeficitVerdictCapabilityDisclosure(t *testing.T) {
	nearFmax, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0.186, 2.455, 2.641, 0, 0.186, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || !strings.Contains(nearFmax, "接近大核满频,缺口仅 0.186ms(已计入有效归因,按默认算力比粗算)") {
		t.Fatalf("G4 counted form must carry the capability caliber:\n%s", nearFmax)
	}
	affirmative, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || !strings.Contains(affirmative, "已按大核满频(或接近)运行,无供给缺口,running 为真实工作量(按默认算力比粗算)") {
		t.Fatalf("affirmative form must carry the capability caliber:\n%s", affirmative)
	}
	freqOnly, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly), 0, true)
	if !ok || !strings.Contains(freqOnly, "已按大核满频(或接近)运行,无供给缺口,running 为真实工作量(簇结构不可判,按纯频率比折算)") {
		t.Fatalf("freq_only affirmative must say the class gap was not priced:\n%s", freqOnly)
	}
}

// The partial-missing lower-bound form carries the caliber; the bare
// "无法折算" no-fold form claims NOTHING even when the token is present
// (nothing was folded — no pricing to disclose).
func TestCAPUnknownBasisDeficitCapabilityDisclosure(t *testing.T) {
	partial, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0.4, 4.0, 3.0, 1.4, 0.4, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || !strings.Contains(partial, "缺口 0.400ms 为下界,按默认算力比粗算") {
		t.Fatalf("partial-missing form must carry the capability caliber:\n%s", partial)
	}
	bare, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 4.4, 3.0, 1.4, 0, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || bare != "CPU 频率数据不全,无法折算" {
		t.Fatalf("the no-fold sentence must stay bare (no capability claim):\n%s", bare)
	}
}

// --- C3: the G1 running-deficit arm's sub-row caliber -------------------------

func capRunningDeficitProjection(source string) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"RenderThread-16867", ".ugc.aweme.lite-16547"},
		WindowStartTs: 33872.289,
		WindowEndTs:   33872.409,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "cap-e8",
			Subject: "RenderThread-16867", Predicate: "root_cause_primary",
			Object: "running", StateKind: "running",
			Rank: 11, ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 1.096, CumulativeImpactMS: 1.096, EffectiveImpactMS: 0.186,
			ActualImpactMS:     2.641,
			SupplyFoldComputed: true, SupplyFoldDeficitMS: 0.186,
			SupplyFoldIdealMS: 2.455, SupplyFoldKnownMS: 2.641,
			SupplyFoldCapabilitySource: source,
			LineStart:                  45689, LineEnd: 79142, Confidence: 0.9,
		}},
	}
}

func TestCAPRunningDeficitArmSubRowCapability(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(capRunningDeficitProjection(runtimeTraceCapabilitySourceDefault), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// 行3 keeps the SHORT closed-set word; the sub-row parenthesis grows the
	// capability caliber (§24.1 单一子行文法 + §26 C3).
	for _, want := range []string{
		"· 有效归因 0.186ms = running(折算,按大核满频) 0.186ms",
		"· running 原始 2.641ms → 计入 0.186ms(折算,按大核满频,按默认算力比粗算)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("G1 arm capability caliber missing %q:\n%s", want, fence)
		}
	}
	// The disclosure word teaches through its own legend seat, on demand.
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `按默认算力比粗算` =") {
		t.Fatalf("按默认算力比粗算 legend entry must render with the word:\n%s", legend)
	}
	// freq_only variant: the fail-loud words ride the same sub-row seat with
	// their own legend entry.
	foModel := buildRuntimeTraceProjTreeModel(capRunningDeficitProjection(runtimeTraceCapabilitySourceFreqOnly), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	foFence := runtimeTraceProjTreeFence(foModel, true)
	if !strings.Contains(foFence, "计入 0.186ms(折算,按大核满频,簇结构不可判,按纯频率比折算)") {
		t.Fatalf("freq_only sub-row caliber missing:\n%s", foFence)
	}
	foLegend := strings.Join(runtimeTraceProjLegendGroupLines(foModel.Marks, true), "\n")
	if !strings.Contains(foLegend, "- `按纯频率比折算` =") {
		t.Fatalf("按纯频率比折算 legend entry must render with the word:\n%s", foLegend)
	}
	if strings.Contains(foLegend, "- `按默认算力比粗算` =") {
		t.Fatalf("the default-table entry must stay off the freq_only shape:\n%s", foLegend)
	}
}

// --- C3: the R5d discounted component + the inversion detail line -------------

func TestCAPInversionCompositionCapabilityDisclosure(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "worker-9", PriorityInversionCandidate: true,
		EffectiveImpactMS: 37.410,
		GatedRunnableMS:   20.713, GatedRunningDeficitMS: 16.697,
		GatedCapabilitySource: runtimeTraceCapabilitySourceDefault,
		SupplyFoldComputed:    true, SupplyFoldDeficitMS: 16.697, SupplyFoldIdealMS: 30.0,
		SupplyFoldKnownMS:          46.697,
		SupplyFoldCapabilitySource: runtimeTraceCapabilitySourceDefault,
	}
	// The fail-open lossless mirror (detail-block composition text).
	text := runtimeTraceProjInversionCompositionText(node, true)
	if !strings.Contains(text, "running 折算 16.697ms(按下游消费核折算,按默认算力比粗算)") {
		t.Fatalf("composition text must disclose the R5d capability caliber:\n%s", text)
	}
	if !strings.Contains(text, "runnable 20.713ms(全额)") {
		t.Fatalf("the runnable component wears no fold ruler:\n%s", text)
	}
	// The structured 拆解子行 caliber (行4+): short word on 行3, full caliber
	// with the capability disclosure in the sub-row parenthesis.
	components, total, ok := runtimeTraceProjInversionComponents(node, true)
	if !ok || total != 37.410 || len(components) != 2 {
		t.Fatalf("inversion components must build: ok=%v total=%v n=%d", ok, total, len(components))
	}
	if components[1].CaliberFull != "折算,按下游消费核,按默认算力比粗算" {
		t.Fatalf("R5d sub-row caliber = %q, want the capability-disclosing form", components[1].CaliberFull)
	}
	// The inversion node's supply-fold detail line (lossless home) carries the
	// fold-side disclosure too.
	detail := runtimeTraceProjInversionSupplyFoldDetailLine(node, true)
	if !strings.Contains(detail, "(折算,按大核满频,下界,按默认算力比粗算;独立口径,不计入有效归因)") {
		t.Fatalf("inversion supply-fold detail line must carry the capability caliber:\n%s", detail)
	}
	// Pre-CAP record: every surface stays byte-stable with no claim.
	bare := node
	bare.GatedCapabilitySource = ""
	bare.SupplyFoldCapabilitySource = ""
	if got := runtimeTraceProjInversionCompositionText(bare, true); !strings.Contains(got, "running 折算 16.697ms(按下游消费核折算)") {
		t.Fatalf("pre-CAP composition text must stay byte-stable:\n%s", got)
	}
	if got := runtimeTraceProjInversionSupplyFoldDetailLine(bare, true); !strings.Contains(got, "(折算,按大核满频,下界;独立口径,不计入有效归因)") {
		t.Fatalf("pre-CAP detail line must stay byte-stable:\n%s", got)
	}
}

// --- C3: note emission + parse round trip -------------------------------------

func TestCAPCapabilityNoteEmissionAndParse(t *testing.T) {
	basis := &tracequery.SupplyFoldBasis{KnownMs: 5, CapabilitySource: tracequery.CoreCapabilitySourceDefault}
	notes := traceQueryTypedSupplyFoldRichNotes(basis, 1, 4)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, types.TraceNoteKeyFoldCapability+"="+tracequery.CoreCapabilitySourceDefault) {
		t.Fatalf("fold_capability note must emit with the fold accounting:\n%s", joined)
	}
	// No token (pre-CAP aggregate re-serialization) → no note.
	bare := traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{KnownMs: 5}, 1, 4)
	if strings.Contains(strings.Join(bare, "\n"), types.TraceNoteKeyFoldCapability) {
		t.Fatalf("an empty capability source must emit no fold_capability note:\n%v", bare)
	}
}

// --- 复核 F1: 判词随实际基准簇 (demoted reference wording) --------------------

func TestCAPDemotedReferenceWording(t *testing.T) {
	// Clause: the Dominant form's basis word follows the typed demoted class.
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	node.SupplyFoldReferenceClass = "small"
	clause, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, true)
	if !ok || !strings.Contains(clause, "(按小核满频折算,下界,按默认算力比粗算)") {
		t.Fatalf("demoted basis must speak 按小核满频, never 按大核满频:\n%s", clause)
	}
	if strings.Contains(clause, "大核") {
		t.Fatalf("a demoted basis must not claim any 大核 word:\n%s", clause)
	}
	// The affirmative form is re-based too (Probe B 判词如实: the pre-fix code
	// claimed 已按大核满频 for a small-only governance window).
	affirmative := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault)
	affirmative.SupplyFoldReferenceClass = "small"
	sentence, _, ok := runtimeTraceProjSupplyFoldClause(affirmative, 0, true)
	if !ok || !strings.Contains(sentence, "已按小核满频(或接近)运行,无供给缺口") {
		t.Fatalf("demoted affirmative must re-base its claim:\n%s", sentence)
	}
	// G1 arm: 行3/子行 caliber words follow the class; the demoted legend seat
	// renders and the 按大核满频 entry stays off.
	projection := capRunningDeficitProjection(runtimeTraceCapabilitySourceDefault)
	projection.OnChainCauses[0].SupplyFoldReferenceClass = "middle"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"· 有效归因 0.186ms = running(折算,按中核满频) 0.186ms",
		"· running 原始 2.641ms → 计入 0.186ms(折算,按中核满频,按默认算力比粗算)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("demoted G1 arm caliber missing %q:\n%s", want, fence)
		}
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `按小核满频折算/按中核满频折算/按超大核满频折算` =") {
		t.Fatalf("the demoted-basis legend seat must render with the word:\n%s", legend)
	}
	if strings.Contains(legend, "- `按大核满频折算` =") {
		t.Fatalf("the big-basis legend entry must stay off a demoted shape:\n%s", legend)
	}
	// Pre-CAP / big-basis records keep every string byte-identically (the
	// producer emits the reference note only on demotion).
	big := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	bigClause, _, ok := runtimeTraceProjSupplyFoldClause(big, 0, true)
	if !ok || !strings.Contains(bigClause, "(按大核满频折算,下界,按默认算力比粗算)") {
		t.Fatalf("the big-basis wording must stay byte-identical:\n%s", bigClause)
	}
}
