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
	if got := dominant(runtimeTraceCapabilitySourceDefault); !strings.Contains(got, "(运行频点非最高,按全域最大核最高频折算,下界,按默认算力比粗算)") {
		t.Fatalf("default-table fold must disclose 按默认算力比粗算:\n%s", got)
	}
	// freq_only: the fail-loud disclosure names what was NOT priced.
	// EVOLUTION RECORD (UXR-1 §29.36.4 ② 核类词诚实门): 簇结构不可判 forbids
	// the core-class word — the fold basis degrades to the class-less 按满频.
	if got := dominant(runtimeTraceCapabilitySourceFreqOnly); !strings.Contains(got, "(运行频点非最高,按全域最高频折算,下界,簇结构不可判,按纯频率比折算)") {
		t.Fatalf("freq_only fold must disclose 簇结构不可判 without a class word:\n%s", got)
	}
	if got := dominant(runtimeTraceCapabilitySourceFreqOnly); strings.Contains(got, "大核") || strings.Contains(got, "小核") {
		t.Fatalf("§29.36.4 ②: no core-class word under 簇结构不可判:\n%s", got)
	}
	// empty (pre-CAP record): no capability claim; the basis word is the
	// unified judged form (R5 supersedes the pre-CAP byte-stability contract
	// — the whole word family re-based).
	got := dominant("")
	if !strings.Contains(got, "(运行频点非最高,按全域最大核最高频折算,下界)") {
		t.Fatalf("pre-CAP record must keep the unified parenthetical (no capability claim):\n%s", got)
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
	if !ok || !strings.Contains(nearFmax, "接近全域最大核最高频,缺口仅 0.186ms(运行频点非最高,已计入有效归因,按默认算力比粗算)") {
		t.Fatalf("G4 counted form must carry the capability caliber:\n%s", nearFmax)
	}
	// EVOLUTION RECORD (UXR-1 §29.36.4 ① 推论链压缩, a4/2549 witness): the
	// A⟹B⟹C affirmative sentence compressed to 证据+末端结论+口径括注.
	affirmative, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || !strings.Contains(affirmative, "已按全域最大核最高频(或接近)运行·无供给折算(按默认算力比粗算)") {
		t.Fatalf("affirmative form must carry the compressed claim + capability caliber:\n%s", affirmative)
	}
	// §29.36.4 ②: the freq_only affirmative is the ruling's exact compressed
	// form — no core-class word beside 簇结构不可判.
	freqOnly, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly), 0, true)
	if !ok || !strings.Contains(freqOnly, "已按全域最高频(或接近)运行·无供给折算(簇结构不可判,按频率比)") {
		t.Fatalf("freq_only affirmative must speak the §29.36.4 compressed form:\n%s", freqOnly)
	}
	if strings.Contains(freqOnly, "大核") {
		t.Fatalf("§29.36.4 ②: no core-class word under 簇结构不可判:\n%s", freqOnly)
	}
}

// The partial-missing lower-bound form carries the caliber; the bare
// "无法折算" no-fold form claims NOTHING even when the token is present
// (nothing was folded — no pricing to disclose).
func TestCAPUnknownBasisDeficitCapabilityDisclosure(t *testing.T) {
	partial, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0.4, 4.0, 3.0, 1.4, 0.4, runtimeTraceCapabilitySourceDefault), 0, true)
	if !ok || !strings.Contains(partial, "缺口 0.400ms 为下界(运行频点非最高,按默认算力比粗算)") {
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
		"· 有效归因 0.186ms = running(折算,按全域最大核最高频) 0.186ms",
		"· running 原始 2.641ms → 计入 0.186ms(折算,按前述基准,按默认算力比粗算)",
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
	// EVOLUTION RECORD (UXR-1 §29.36.4 ② 核类词诚实门): the sub-row caliber
	// drops the core-class word beside 簇结构不可判 (class-less 按满频 form).
	if !strings.Contains(foFence, "计入 0.186ms(折算,按全域最高频,簇结构不可判,按纯频率比折算)") {
		t.Fatalf("freq_only sub-row caliber missing:\n%s", foFence)
	}
	if strings.Contains(foFence, "按全域最大核最高频,簇结构不可判") {
		t.Fatalf("§29.36.4 ②: no core-class word beside 簇结构不可判:\n%s", foFence)
	}
	foLegend := strings.Join(runtimeTraceProjLegendGroupLines(foModel.Marks, true), "\n")
	// EVOLUTION RECORD (UXR-1 §29.36.4 ①): the entry teaches both row forms.
	if !strings.Contains(foLegend, "- `按纯频率比折算`/`按频率比` =") {
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
	if !strings.Contains(text, "running 折算 16.697ms(运行频点非最高,按全域最大核最高频折算,按默认算力比粗算)") {
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
	if components[1].CaliberFull != "折算,按全域最大核最高频,运行频点非最高,按默认算力比粗算" {
		t.Fatalf("R5d sub-row caliber = %q, want the capability-disclosing form", components[1].CaliberFull)
	}
	// The inversion node's supply-fold detail line (lossless home) carries the
	// fold-side disclosure too.
	detail := runtimeTraceProjInversionSupplyFoldDetailLine(node, true)
	if !strings.Contains(detail, "(运行频点非最高,折算,按全域最大核最高频,下界,按默认算力比粗算;与有效归因中 running 计入同源同值)") {
		t.Fatalf("inversion supply-fold detail line must carry the capability caliber:\n%s", detail)
	}
	// Pre-CAP record: every surface stays byte-stable with no claim.
	bare := node
	bare.GatedCapabilitySource = ""
	bare.SupplyFoldCapabilitySource = ""
	if got := runtimeTraceProjInversionCompositionText(bare, true); !strings.Contains(got, "running 折算 16.697ms(运行频点非最高,按全域最大核最高频折算)") {
		t.Fatalf("pre-CAP composition text must stay byte-stable:\n%s", got)
	}
	if got := runtimeTraceProjInversionSupplyFoldDetailLine(bare, true); !strings.Contains(got, "(运行频点非最高,折算,按全域最大核最高频,下界;与有效归因中 running 计入同源同值)") {
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

// --- R5 (§29.88.12): 单基准词面 — reference_class no longer forks the word --

// EVOLUTION RECORD (R5 单基准, 2026-07-15): the 复核-F1 demoted-reference
// wording (判词随实际基准簇 — 按小核/中核/超大核满频折算 + its legend seat)
// is RETIRED with the demotion arm itself: the R5 basis is trace-global and
// never demotes, so a stale/legacy reference_class note must NOT fork the
// word — every record speaks the unified 全域最大核最高频 basis form, and
// the retired class words never render.
func TestCAPDemotedReferenceWording(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	node.SupplyFoldReferenceClass = "small"
	clause, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, true)
	if !ok || !strings.Contains(clause, "(运行频点非最高,按全域最大核最高频折算,下界,按默认算力比粗算)") {
		t.Fatalf("the unified basis word must render regardless of reference_class:\n%s", clause)
	}
	for _, banned := range []string{"按小核满频", "按中核满频", "按超大核满频", "按大核满频"} {
		if strings.Contains(clause, banned) {
			t.Fatalf("retired demoted/legacy basis word %q leaked:\n%s", banned, clause)
		}
	}
	// The affirmative form speaks the same unified basis.
	affirmative := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault)
	affirmative.SupplyFoldReferenceClass = "small"
	sentence, _, ok := runtimeTraceProjSupplyFoldClause(affirmative, 0, true)
	if !ok || !strings.Contains(sentence, "已按全域最大核最高频(或接近)运行·无供给折算") {
		t.Fatalf("affirmative form must speak the unified basis:\n%s", sentence)
	}
	// G1 arm: 行3/子行 caliber words are class-independent; the retired
	// demoted legend seat never renders.
	projection := capRunningDeficitProjection(runtimeTraceCapabilitySourceDefault)
	projection.OnChainCauses[0].SupplyFoldReferenceClass = "middle"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"· 有效归因 0.186ms = running(折算,按全域最大核最高频) 0.186ms",
		"· running 原始 2.641ms → 计入 0.186ms(折算,按前述基准,按默认算力比粗算)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("unified G1 arm caliber missing %q:\n%s", want, fence)
		}
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if strings.Contains(legend, "按小核满频折算") {
		t.Fatalf("the retired demoted-basis legend seat must never render:\n%s", legend)
	}
	if !strings.Contains(legend, "- `折算,按全域最大核最高频`/`按全域最大核最高频折算`") {
		t.Fatalf("the unified basis legend entry must render with the word:\n%s", legend)
	}
}
