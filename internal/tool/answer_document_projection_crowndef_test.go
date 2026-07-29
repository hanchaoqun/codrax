package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_crowndef_test.go — B+ (user ruling 2026-07-28):
// 主根因 becomes a DEFINED term of art. CROWNPOS-1 (第二裁, same day): the
// short definitional parenthetical rides the TERM itself — the crowned head
// prefix is 「**主根因(=已证链上单项最大可消除量):**」, never a tail note (the
// byte-exact prefix+no-tail pin lives in the close1 headline test). The
// legend carries the full definition (election over credentials + effective
// attribution, never a mechanism verdict), and both LLM word surfaces teach
// it. 「首要可消除项」 is the reserved rename should plan A ever be ruled.

func TestPrimaryCrownDefinitionLegendEntry(t *testing.T) {
	// The definition rides the EXISTING ➊-anchored Badge legend entry (the
	// 词条-图例双向 discipline: the anchor token lives in the fence).
	marks := &runtimeTraceProjMarkSet{}
	marks.mark(runtimeTraceProjMarkBadge)
	zh := strings.Join(runtimeTraceProjLegendGroupLines(marks, true), "\n")
	if !strings.Contains(zh, "主根因=已证链上候选中单项最大可消除量的持有席") ||
		!strings.Contains(zh, "非机理层裁定") {
		t.Fatalf("zh badge legend must carry the crown definition:\n%s", zh)
	}
	en := strings.Join(runtimeTraceProjLegendGroupLines(marks, false), "\n")
	if !strings.Contains(en, "largest single proven on-chain eliminable contribution") ||
		!strings.Contains(en, "never a mechanism-level verdict") {
		t.Fatalf("en badge legend must carry the crown definition:\n%s", en)
	}
}

func TestPrimaryCrownTeachingOnBothLLMFaces(t *testing.T) {
	tq := &TraceQuery{}
	for name, face := range map[string]string{
		"description": tq.Description(),
		"parameters":  string(tq.Parameters()),
	} {
		for _, want := range []string{
			"DEFINED term of art",
			"largest single PROVEN on-chain eliminable contribution",
			"never a mechanism-level verdict",
		} {
			if !strings.Contains(face, want) {
				t.Fatalf("%s must teach the crown definition, missing %q", name, want)
			}
		}
	}
}

// --- CROWNCAL-1 (user ruling 2026-07-28, report 20260728-202128): the crown
// definition sits beside the printed magnitude, so when the "=" form is
// suppressed (degenerate single-full composite) or unbuildable, a magnitude
// printed on a NON-effective caliber must still carry the elected eliminable
// value: 「链上累计 2.262ms，有效归因 1.661ms(全额)」. Equal-valued seats stay
// byte-identical (no arm).

func crownCalInversionNode(cumulative, effective, gatedRunnable float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-crowncal",
		Subject: "CookieMonsterCl-59843", Object: "priority_inversion_candidate",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		PriorityInversionCandidate: true,
		ImpactMS:                   effective, CumulativeImpactMS: cumulative, EffectiveImpactMS: effective,
		GatedRunnableMS: gatedRunnable,
		ChainRelevance:  "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		Confidence: 0.9,
	}
}

func crownCalProjection(node types.TraceCausalProjectionNode) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"CookieMonsterCl-59843", "com.baidu.tieba-59566"},
		WindowStartTs:     34579.472865,
		WindowEndTs:       34579.475857,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{node},
		OnChainCauses:     []types.TraceCausalProjectionNode{node},
	}
	return projection, buildRuntimeTraceProjTreeModel(projection, nil, true)
}

func TestCrownCalDegenerateUnequalCarriesEffectiveArm(t *testing.T) {
	// The customer shape: single runnable(全额) component (degenerate — the
	// "=" form suppressed) whose chain covers only the largest fragment, so
	// cumulative 2.262 ≠ elected effective 1.661.
	projection, model := crownCalProjection(crownCalInversionNode(2.262, 1.661, 1.661))
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "链上累计 2.262ms，有效归因 1.661ms(全额)。") {
		t.Fatalf("the definitional-adjacent magnitude must carry the elected effective:\n%s", line)
	}
	lineEN := runtimeTraceProjConclusionLine(projection, model, false)
	if !strings.Contains(lineEN, "chain total 2.262ms, attribution 1.661ms (in full).") {
		t.Fatalf("EN twin must carry the effective arm:\n%s", lineEN)
	}
}

func TestCrownCalEqualValuesStaySilent(t *testing.T) {
	// Full chain coverage: cumulative == effective — the healthy shape keeps
	// the bare magnitude byte-identically (no arm, no one-term equation).
	projection, model := crownCalProjection(crownCalInversionNode(1.661, 1.661, 1.661))
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if strings.Contains(line, "有效归因") {
		t.Fatalf("equal-valued degenerate seat must not grow an arm:\n%s", line)
	}
	if !strings.Contains(line, "链上累计 1.661ms。") {
		t.Fatalf("equal-valued seat must keep the bare magnitude:\n%s", line)
	}
}
