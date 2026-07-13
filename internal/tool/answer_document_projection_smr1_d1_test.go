package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WO-D1① (SMR-1 批 SMR-S2, smr_audit_report §②, 2026-07-12; 25846 witness):
// T7@ZeusThreadPo-61839 rendered FIVE self seats for one sleep account —
// E1 sleep 105.794 3次(8.879–80.751) + root_evidence re-issues E3 80.751 /
// E5 16.164 / E6 8.879 + the ◌ missing_wakeup row carrying 80.751 — the
// key-metric table could be summed to 292.3ms vs 真值 105.8 (2.76×). Post-fix:
// the re-issues fold into the merged seat (member-value µs identity), the ◌
// row keeps its ⊘链止 honesty seat but drops the duplicated ms (无值披露臂 +
// 不可相加·为[E1]成员 pointer).

func smr1D1S2Projection() types.TraceCausalProjection {
	self := "t7@zeusthreadpo-61839"
	raw := func(id string, ms float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:" + id,
			Subject: self, Predicate: "sleep_wait",
			Tier: types.TraceCausalTierContextOnly, ChainRelevance: "on_chain",
			ImpactMS: ms, Confidence: 0.75, LineStart: line, LineEnd: line + 50,
		}
	}
	undrillable := raw("4", 80.751, 700)
	undrillable.Predicate = "missing_wakeup"
	undrillable.UndrillableReason = "missing_wakeup"
	return types.TraceCausalProjection{
		WakeupPath:    []string{"binder:43397_19-23088", "T7@ZeusThreadPo-61839"},
		WindowStartTs: 1000.000,
		WindowEndTs:   1000.144557,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E1: the merged sleep seat (×3, sum 105.794 = 8.879+16.164+80.751).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e1",
				Subject: self, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain",
				ImpactMS:       105.794, CumulativeImpactMS: 105.794,
				MergedCount: 3, MergedMinMS: 8.879, MergedMaxMS: 80.751,
				Confidence: 0.9, LineStart: 100, LineEnd: 900},
			raw("1", 80.751, 200),
			raw("2", 16.164, 400),
			raw("3", 8.879, 600),
			undrillable,
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop",
				Subject: "binder:43397_19-23088", Predicate: "wakeup_causal_impact",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 13.898, CumulativeImpactMS: 13.898,
				Confidence: 0.8, LineStart: 950, LineEnd: 980},
		},
	}
}

func TestSMR1D1S2MemberReissuesFoldIntoMergedSeat(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1D1S2Projection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var seats []*runtimeTraceProjTreeRow
	for i := range model.SelfRows {
		if model.SelfRows[i].HasData {
			seats = append(seats, &model.SelfRows[i])
		}
	}
	// 25846 pin: 五行→折后席数 = 2 (the merged sleep seat + the ◌ honesty seat).
	if len(seats) != 2 {
		t.Fatalf("五行→折后席数: want 2 self seats, got %d\n%s", len(seats), fence)
	}
	var merged, gap *runtimeTraceProjTreeRow
	for _, row := range seats {
		if row.Node.Undrillable() {
			gap = row
		} else if row.Node.MergedCount == 3 {
			merged = row
		}
	}
	if merged == nil || gap == nil {
		t.Fatalf("the merged seat and the ⊘链止 seat must both survive\n%s", fence)
	}
	// The three re-issues ride the mirror peer carrier (annotation only).
	if len(merged.SameSegMirrorPeers) != 3 {
		t.Fatalf("three root_evidence re-issues must fold as peers, got %d", len(merged.SameSegMirrorPeers))
	}
	if merged.Node.ImpactMS != 105.794 {
		t.Fatalf("吸收=佩记号,数值不重计: %v", merged.Node.ImpactMS)
	}
	// ◌ 无值披露臂: the honesty seat keeps ⊘链止 but no duplicated ms; the
	// member pointer says where the value lives.
	if gap.NonAdditiveKind != runtimeTraceProjNonAdditiveMember {
		t.Fatalf("the ◌ row must carry the member pointer, got %v", gap.NonAdditiveKind)
	}
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "链止") && strings.Contains(line, "80.751ms") {
			t.Fatalf("◌ 无值披露臂: the honesty seat must not re-carry the member value:\n%s", line)
		}
	}
	if !strings.Contains(fence, "链止") {
		t.Fatalf("the ⊘链止 honesty face must stay:\n%s", fence)
	}
}

// A raw copy whose value matches NO derivable member keeps its own honest row.
func TestSMR1D1S2NonMemberRawStaysSeated(t *testing.T) {
	projection := smr1D1S2Projection()
	projection.OnChainCauses[2].ImpactMS = 17.000 // ≠ derivable middle 16.164
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	count := 0
	for i := range model.SelfRows {
		if model.SelfRows[i].HasData {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("non-member raw copies never fold (µs 恒等才算成员), got %d seats", count)
	}
}
