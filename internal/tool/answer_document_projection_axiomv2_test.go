package tool

// answer_document_projection_axiomv2_test.go — AXIOM-V2 批 display pins
// (user rulings 2026-07-18):
//
//   pin② the cross-direction mutual clause renders on BOTH seats of a pair
//        with resolved [E#]s (and on NEITHER when one side's carrier is
//        absent — 宁漏勿假指 both-or-neither).
//   pin⑤ ordinal-chip family absence (根因排序三护栏之① 的现状半): every
//        `#N` ordinal face on representative renders belongs to the
//        established seat-chip family (根因排序/邻近影响 + derived pointer/
//        badge/fold faces); fix-direction words NEVER compose with an
//        ordinal.
//   护栏② the 行2 修向 word renders for resolved directions only.
//   护栏③ the 根因排序键 definition sentence rides the 阅读参考 legend
//        exactly when a direction face is on the render (承诺面双向).

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// axiomv2CrossDirectionProjection mirrors the cust_span_runnable E26×E5
// geometry: the target's running supply-fold deficit seat (frequency_thermal
// 向) and its class_verification semantic family seat (self_workload 向) with
// symmetric cross-direction overlap entries.
func axiomv2CrossDirectionProjection() types.TraceCausalProjection {
	running := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "axv2-running",
		Subject: "ease.cloudmusic-63993", Predicate: "root_cause_secondary",
		Object: "running", TypeToken: "running", StateKind: "running",
		ChainRelevance: "on_chain", Causality: "self_wall_clock",
		OnChainBasis: "self_wall_clock_interval",
		ImpactMS:     107.084, CumulativeImpactMS: 107.084, EffectiveImpactMS: 4.843,
		Rank: 2, Tier: "secondary", Confidence: 0.86,
		LineStart: 26424, LineEnd: 61077,
		FixDirection: "frequency_thermal",
		CrossDirectionOverlaps: []types.TraceCausalProjectionCrossDirectionOverlap{
			{OverlapMS: 9.586, LineStart: 40424, LineEnd: 40925,
				Direction: "self_workload", Basis: "semantic_member_intervals"},
		},
	}
	family := xlane2SemanticFamilyNode("axv2-sem-fam", 2, [][2]int{{40424, 40700}, {40702, 40925}}, 9.586, 9.586)
	family.ChainRelevance = "on_chain"
	family.Causality = "self_deterministic"
	family.OnChainBasis = "self_deterministic_span"
	family.Rank = 1
	family.Tier = "primary"
	family.LineStart, family.LineEnd = 40424, 40925
	family.FixDirection = "self_workload"
	family.CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{
		{OverlapMS: 9.586, LineStart: 26424, LineEnd: 61077,
			Direction: "frequency_thermal", Basis: "self_running_intervals"},
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs:           17729.471126,
		WindowEndTs:             17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{
			running,
			elimChainNode("axv2-dio", "workSharkThread-64796", "d_state_or_io_wait", "d_sleep", 3, 5.368, 800),
		},
		SemanticSpans: []types.TraceCausalProjectionNode{family},
	}
}

// pin② + 护栏②: both seats speak the mutual clause with resolved [E#]s, the
// 行2 修向 words render, marks record, values stay untouched.
func TestAXIOMV2MutualClauseRendersBothSides(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(axiomv2CrossDirectionProjection(),
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
			runRow := xlane2FindRowByEvidenceID(&model, "axv2-running")
			famRow := xlane2FindRowByEvidenceID(&model, "axv2-sem-fam")
			if runRow == nil || famRow == nil {
				t.Fatalf("fixture drifted: both seats must render")
			}
			runTag := strings.TrimSpace(runRow.EvidenceTag)
			famTag := strings.TrimSpace(famRow.EvidenceTag)
			if runTag == "" || famTag == "" {
				t.Fatalf("fixture drifted: both seats must carry tags")
			}
			onRunning := "与[" + famTag + "](修向 自身工作量)同段重叠 9.586ms:作用于同段时间,修其一后另一席空间会缩,收益不叠加"
			onFamily := "与[" + runTag + "](修向 频率与热治理)同段重叠 9.586ms:作用于同段时间,修其一后另一席空间会缩,收益不叠加"
			dirRunning, dirFamily := "修向 频率与热治理", "修向 自身工作量"
			if !zh {
				// The EN probes stop before the width-budget wrap point (the
				// shared tail is probed separately below).
				onRunning = "overlaps [" + famTag + "] (fix-direction own workload) by 9.586ms"
				onFamily = "overlaps [" + runTag + "] (fix-direction frequency & thermal) by 9.586ms"
				dirRunning, dirFamily = "fix-direction frequency & thermal", "fix-direction own workload"
				if !strings.Contains(fence, "gains do not add") {
					t.Fatalf("pin②: the EN clause tail must render:\n%s", fence)
				}
			}
			for _, want := range []string{onRunning, onFamily, dirRunning, dirFamily} {
				if !strings.Contains(fence, want) {
					t.Fatalf("pin②/护栏②: %q must render:\n%s", want, fence)
				}
			}
			if !model.Marks.has(runtimeTraceProjMarkCrossDirectionOverlap) ||
				!model.Marks.has(runtimeTraceProjMarkFixDirection) {
				t.Fatalf("legend marks must record at the emission sites")
			}
			// 主值零动: published values stay.
			for _, value := range []string{"107.084", "4.843", "9.586"} {
				if !strings.Contains(fence, value) {
					t.Fatalf("value %s must stay on the render:\n%s", value, fence)
				}
			}
		})
	}
}

// pin② negative arms: a one-sided wire roster (reciprocity broken) renders
// the clause on NEITHER seat; a partner whose own direction contradicts the
// entry drops the pair (stale persisted form).
func TestAXIOMV2MutualClauseBothOrNeither(t *testing.T) {
	oneSided := axiomv2CrossDirectionProjection()
	oneSided.SemanticSpans[0].CrossDirectionOverlaps = nil // the family lost its half
	model := buildRuntimeTraceProjTreeModel(oneSided,
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "收益不叠加") {
		t.Fatalf("both-or-neither: a one-sided roster must render NO mutual clause:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkCrossDirectionOverlap) {
		t.Fatalf("both-or-neither: the mark must not record on a pruned pair")
	}

	stale := axiomv2CrossDirectionProjection()
	// The running seat's entry claims the partner is self_workload, but the
	// family's own published direction says memory → stale, drop the pair.
	stale.SemanticSpans[0].FixDirection = "memory"
	model = buildRuntimeTraceProjTreeModel(stale,
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "收益不叠加") {
		t.Fatalf("stale direction mismatch must drop the pair:\n%s", fence)
	}
}

// 护栏③: the 根因排序键 definition sentence rides the 阅读参考 legend exactly
// when a direction face is on the render (承诺面双向).
func TestAXIOMV2ReadingReferenceSortKeyEntry(t *testing.T) {
	sortKeyZH := "- 根因排序键 = 各席折算后可消除的提升空间(即 有效归因):跨修复方向同一口径下可比、不可相加(同段重叠收益不叠加,见行内互指句);修向 = 修复方向归类(registry 属性轴),不改变排序与数值。"
	text := scoreDerivClusterText(t, axiomv2CrossDirectionProjection(), "zh")
	if !strings.Contains(text, sortKeyZH) {
		t.Fatalf("护栏③: the sort-key definition sentence must render with the direction faces:\n%s", text)
	}
	// 负臂: a board without any direction face renders no sentence.
	bare := scoreDerivClusterText(t, xlane2SelfGapOverlapProjection(), "zh")
	if strings.Contains(bare, "根因排序键") {
		t.Fatalf("护栏③ 负臂: a direction-free board must not render the sentence:\n%s", bare)
	}
}

// pin: the projection decode cap mirrors the engine emission cap (single
// producer, single decoder).
func TestAXIOMV2CrossDirectionCapMirrorsEngine(t *testing.T) {
	if types.TraceCausalProjectionCrossDirectionOverlapCap != tracequery.RootCauseCrossDirectionOverlapPartnerCap {
		t.Fatalf("decode cap %d != engine cap %d",
			types.TraceCausalProjectionCrossDirectionOverlapCap, tracequery.RootCauseCrossDirectionOverlapPartnerCap)
	}
}

// pin⑤ (护栏① 现状半): every `#N` ordinal face on representative renders
// belongs to the established seat-chip family — the channel-worded chips
// (根因排序#N / 邻近影响#N + EN), the 见榜位#N pointer, the ❶❷❸ badge ranks
// and the micro-fold #lo~#hi range; fix-direction words NEVER wear an
// ordinal (◎ 方向节零序数 stays for ELIM-V2, but the direction WORDS minting
// ordinals would already break the axis here).
func TestAXIOMV2OrdinalChipFamilyAbsencePin(t *testing.T) {
	fixtures := []struct {
		name string
		proj types.TraceCausalProjection
	}{
		{"axiomv2_cross_direction", axiomv2CrossDirectionProjection()},
		{"xlane2_self_gap", xlane2SelfGapOverlapProjection()},
		{"rnb5b_micro_fold", rnb5bMicroAnchorFoldProjection()},
		{"levelmerge_gated_share", levelmergeGatedShareProjection()},
		{"hullcred_tiers", hullcredCredentialTiersProjection()},
	}
	chipRe := regexp.MustCompile(`#\d+`)
	badgeRunes := []rune("❶❷❸❹❺❻❼❽❾❿")
	isBadge := func(r rune) bool {
		for _, badge := range badgeRunes {
			if r == badge {
				return true
			}
		}
		return false
	}
	directionWordsZH := []string{"调度供给", "锁与优先级", "IO与依赖", "内存", "频率与热治理", "自身工作量"}
	for _, fixture := range fixtures {
		for _, zh := range []bool{true, false} {
			model := buildRuntimeTraceProjTreeModel(fixture.proj,
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
			for _, loc := range chipRe.FindAllStringIndex(fence, -1) {
				prefixStart := loc[0] - 60
				if prefixStart < 0 {
					prefixStart = 0
				}
				prefix := fence[prefixStart:loc[0]]
				allowed := strings.HasSuffix(prefix, "根因排序") ||
					strings.HasSuffix(prefix, "邻近影响") ||
					strings.HasSuffix(prefix, "root-cause rank ") ||
					strings.HasSuffix(prefix, "adjacent-impact ") ||
					strings.HasSuffix(prefix, "见榜位") ||
					strings.HasSuffix(prefix, "~") // #lo~#hi second half
				if !allowed {
					runes := []rune(prefix)
					if len(runes) > 0 && isBadge(runes[len(runes)-1]) {
						allowed = true // ❶#N badge-rank face
					}
				}
				if !allowed && strings.HasPrefix(fence[loc[1]:], "~#") {
					allowed = true // #lo~#hi first half (fold range)
				}
				if !allowed {
					t.Fatalf("%s: ordinal face %q outside the seat-chip family (context %q)",
						fixture.name, fence[loc[0]:loc[1]], prefix)
				}
			}
			// Direction words never compose with an ordinal (attribute axis).
			for _, word := range directionWordsZH {
				if strings.Contains(fence, word+"#") {
					t.Fatalf("%s: direction word %q wears an ordinal", fixture.name, word)
				}
			}
			if regexp.MustCompile(`(修向|fix-direction)[^\n·]*#\d`).MatchString(fence) {
				t.Fatalf("%s: a fix-direction fragment composes with an ordinal", fixture.name)
			}
		}
	}
}
