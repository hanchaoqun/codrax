package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_dstate_refine_test.go — DSTATE-REFINE display
// pins (CAL-1 件③, §29.39②/§29.47.2, 2026-07-12), three arms on the 96728 +
// donghu CompThread witness shapes:
//   - arm a: the coverage-proven merged row speaks the refined 「D-state」
//     word (name/shape/peer wording family); unproven rows keep the honest
//     merged 「D-state/iowait」.
//   - arm b: a D-family row refined to the typed io_wait token wears
//     「IO等待候选」, never the family 「D状态候选」 (96728 E14 三面三说法).
//   - arm c: the bare 「· D-state」 tail's emission point merges into the
//     name lane (refined AND mixed forms).
//   - caller: the unanimous blocked_reason symbol disclosed on 行2.

func dstateRefineRecord(id, object string, notes []string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary:" + object,
		Subject:         "CompThread_0-2955",
		Object:          object,
		Value:           "36.757",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 2260, LineEnd: 25862},
		RichNotes:       notes,
		Confidence:      0.82,
	}
}

func dstateRefineModel(t *testing.T, object string, extraNotes ...string) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
	t.Helper()
	notes := append([]string{
		"tier=primary", "rank=1", "type=" + object, "impact_ms=36.757",
		"cumulative_impact_ms=36.757", "effective_impact_ms=36.757",
		"chain_relevance=on_chain", "causality=on_wakeup_chain",
		"dominant_state=d_sleep",
	}, extraNotes...)
	projection := types.TraceCausalProjectionFromObservationRecords(
		[]types.ObservationRecord{dstateRefineRecord("dsr-1", object, notes)})
	projection.WakeupPath = []string{"CompThread_0-2955", ".ugc.aweme.lite-17267"}
	return projection, buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
}

func dstateRefineFence(t *testing.T, object string, extraNotes ...string) string {
	t.Helper()
	_, model := dstateRefineModel(t, object, extraNotes...)
	return runtimeTraceProjTreeFence(model, true)
}

// TestDStateRefineArmARefinedWordAndCallerDisclosure — the donghu CompThread
// shape: coverage-proven merged row → refined 「D-state」 word, 行2 内核调用点,
// and no bare 「· D-state」 tail.
func TestDStateRefineArmARefinedWordAndCallerDisclosure(t *testing.T) {
	fence := dstateRefineFence(t, "d_state_or_io_wait",
		"dstate_all_noniowait=true", "blocked_reason_caller=dma_fence_default_wait")
	if strings.Contains(fence, "D-state/iowait") {
		t.Fatalf("the coverage-proven row must speak the refined D-state word, not the merged form:\n%s", fence)
	}
	if !strings.Contains(fence, "D-state") {
		t.Fatalf("the refined word must stay in the D-state vocabulary:\n%s", fence)
	}
	if !strings.Contains(fence, "内核调用点 dma_fence_default_wait") {
		t.Fatalf("the unanimous caller must disclose on 行2:\n%s", fence)
	}
	// Tri-form (件③ arm b 用户补正): the coverage-proven PURE-D row keeps the
	// 「D状态候选」 category, never the mixed compound.
	if !strings.Contains(fence, "D状态候选") || strings.Contains(fence, "D状态/IO候选") {
		t.Fatalf("the refined pure-D row keeps the D状态候选 category:\n%s", fence)
	}
	// 修复轮 P2-3 crown face: the 主根因 line consumes the proof too — no
	// merged compound beside the refined seat on ANY face of the segment.
	projection, model := dstateRefineModel(t, "d_state_or_io_wait",
		"dstate_all_noniowait=true", "blocked_reason_caller=dma_fence_default_wait")
	if crown := runtimeTraceProjConclusionLine(projection, model, true); strings.Contains(crown, "D-state/iowait") ||
		!strings.Contains(crown, "D-state（d_state_or_io_wait）") {
		t.Fatalf("the crown must speak the refined D4 form: %q", crown)
	}
	// arm c: no bare demoted 「· D-state」 tail line (the name already says it).
	for _, line := range strings.Split(fence, "\n") {
		if strings.TrimSpace(strings.TrimLeft(line, "│ └├─")) == "· D-state" {
			t.Fatalf("the bare D-state tail must not re-tag a name that speaks it: %q", line)
		}
	}
}

// TestDStateRefineArmAUnprovenKeepsMergedWord — no coverage proof → the
// honest merged 「D-state/iowait」 word stays (E16 mixed-form discipline),
// the category speaks the ISOMORPHIC mixed compound 「D状态/IO候选」 (件③
// arm b 用户补正, witness 45261 E9 — 三面一说), and the bare tail still
// never re-tags it (arm c mixed form).
func TestDStateRefineArmAUnprovenKeepsMergedWord(t *testing.T) {
	fence := dstateRefineFence(t, "d_state_or_io_wait")
	if !strings.Contains(fence, "D-state/iowait") {
		t.Fatalf("an unproven merged row keeps the honest two-sided word:\n%s", fence)
	}
	if !strings.Contains(fence, "D状态/IO候选") {
		t.Fatalf("the mixed row's category must speak the mixed compound (三面一说):\n%s", fence)
	}
	if strings.Contains(fence, "D状态候选") {
		t.Fatalf("the bare pure-D category must not ride a mixed row:\n%s", fence)
	}
	if strings.Contains(fence, "内核调用点") {
		t.Fatalf("no caller note → no 内核调用点 disclosure:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "│ └├─"))
		if trimmed == "· D-state" {
			t.Fatalf("the mixed row's bare D-state tail must not survive (arm c): %q", line)
		}
	}
}

// TestDStateRefineProofPropagatesToSameSegmentTwin — 修复轮 P2-3 (冷读
// donghu E8/E9): the wakeup_chain causal-impact TWIN of the proof-carrying
// window_stats fold row (same subject + exact line span = the existing
// same-segment twin key) must speak the SAME refined wordface — the
// pre-fix render put 「D-state·内核调用点」 and 「D-state/iowait(对端未解析)」
// side by side for one physical set of segments.
func TestDStateRefineProofPropagatesToSameSegmentTwin(t *testing.T) {
	proof := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-fold",
		Subject: "CompThread_0-2955", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_sleep", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 36.757, CumulativeImpactMS: 36.757, EffectiveImpactMS: 36.757,
		ChainRelevance: "on_chain", Confidence: 0.82,
		LineStart: 2260, LineEnd: 25862,
		DStateRefinedNonIO: true, BlockedReasonCaller: "dma_fence_default_w",
	}
	twin := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-twin",
		Subject: "CompThread_0-2955", Object: "unknown-thread", TypeToken: "d_state_or_io_wait",
		StateKind: "d_sleep", Predicate: "critical_blocking",
		ImpactMS: 36.757, CumulativeImpactMS: 36.757,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8,
		LineStart: 2260, LineEnd: 25862,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"CompThread_0-2955", ".ugc.aweme.lite-17267"},
		WindowStartTs:     13762.791708,
		WindowEndTs:       13763.024898,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{proof},
		OnChainCauses:     []types.TraceCausalProjectionNode{proof, twin},
	}
	fence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if strings.Contains(fence, "D-state/iowait") {
		t.Fatalf("the same-segment twin must consume the propagated proof — no merged word beside the refined seat:\n%s", fence)
	}
	if !strings.Contains(fence, "D-state(对端未解析)") {
		t.Fatalf("the twin must speak the refined peer wording:\n%s", fence)
	}
}

// TestDStateRefineArmBRefinedIOWaitCategory — the 96728 E14 shape: the row's
// TYPE lane refined to io_wait while the dominant state stayed D → row-1
// speaks iowait, the category consumes the refinement (「IO等待候选」, never
// 「D状态候选」), and no bare D-state tail survives.
func TestDStateRefineArmBRefinedIOWaitCategory(t *testing.T) {
	fence := dstateRefineFence(t, "io_wait")
	if !strings.Contains(fence, "IO等待候选") {
		t.Fatalf("the refined-to-iowait row must wear the IO等待候选 category:\n%s", fence)
	}
	if strings.Contains(fence, "D状态候选") {
		t.Fatalf("the family D状态候选 word must not survive on a refined iowait row (E14 三面三说法):\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		if strings.TrimSpace(strings.TrimLeft(line, "│ └├─")) == "· D-state" {
			t.Fatalf("the refined row's bare D-state tail must not survive (arm c): %q", line)
		}
	}
}
