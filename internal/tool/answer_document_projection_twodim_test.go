package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_twodim_test.go — TWODIM-1 (§18 双维度审计处置,
// user ruling 2026-07-28): root causes have TWO dimensions — the rule-priced
// eliminable board AND raw time occupancy guiding NEW fix directions. These
// pins cover the G1 outlet (the 未计价占用 aux account for on-chain
// context-only genuine occupancy) and the teaching word faces.

func twodimProjectionWithUnpricedRunning() types.TraceCausalProjection {
	projection := elimBoardProjection()
	unpriced := elimChainNode("E-ctx", "worker-7777", "running", "running", 0, 41.500, 400)
	unpriced.Tier = types.TraceCausalTierContextOnly
	unpriced.Rank = 0
	unpriced.ChainRelevance = "on_chain"
	projection.OnChainCauses = append(projection.OnChainCauses, unpriced)
	return projection
}

// G1: an on-chain row whose raw occupancy is genuine but priced to zero
// (context_only) rides the 排除≠消失 aux account with the own-workload lever
// — it must never silently vanish from the ◎ guidance page again.
func TestTwoDimUnpricedOccupancyAuxRow(t *testing.T) {
	_, fence := elimRenderOverview(t, twodimProjectionWithUnpricedRunning(), true)
	if !strings.Contains(fence, "· 未计价占用") ||
		!strings.Contains(fence, "真实占时·杠杆=自身工作量(新方向)") {
		t.Fatalf("the unpriced-occupancy aux row must render with the own-workload lever:\n%s", fence)
	}
	if !strings.Contains(fence, "41.500ms") {
		t.Fatalf("the aux row must carry the largest raw value:\n%s", fence)
	}
	// Negative arm: no on-chain context-only valued rows → no aux row.
	_, clean := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(clean, "未计价占用") {
		t.Fatalf("the aux row must be population-gated:\n%s", clean)
	}
	// EN face.
	_, en := elimRenderOverview(t, twodimProjectionWithUnpricedRunning(), false)
	if !strings.Contains(en, "unpriced occupancy") || !strings.Contains(en, "lever: own workload") {
		t.Fatalf("en face must carry the same account:\n%s", en)
	}
}

// G1b/G3③: both LLM word surfaces teach the two-dimension frame and the
// blocking_span pricing arm.
func TestTwoDimTeachingOnBothLLMFaces(t *testing.T) {
	tq := &TraceQuery{}
	for name, face := range map[string]string{
		"description": tq.Description(),
		"parameters":  string(tq.Parameters()),
	} {
		for _, want := range []string{
			"root causes have TWO dimensions",
			"raw time occupancy that guides NEW fix directions",
			"blocking_span rows price by their converged blocked wall clock",
			"未计价占用",
		} {
			if !strings.Contains(face, want) {
				t.Fatalf("%s must teach the two-dimension frame, missing %q", name, want)
			}
		}
	}
}
