package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R11SubsetShareProjection is the representative hand shape of the
// round-11 face (the 105 网易云 后台销毁 #4 nputmethod.anco-38135 geometry,
// case A): a chain seat holding the walked share, the thread's
// cpu_affinity_or_cpuset ◇ seat keeping its restricted instants outside it,
// and the ◇ runnable remainder whose rest is partly counted on that seat.
func hmc377R11SubsetShareProjection() types.TraceCausalProjection {
	chain := elimChainNode("r11-chain", "nputmethod.anco-38135", "runnable_wait", "runnable", 1, 7.413, 41)
	adjacent := func(id, typ string, value float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "nputmethod.anco-38135", Predicate: "root_cause_adjacent", Object: typ, TypeToken: typ, StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 2, Tier: "tertiary",
			ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value,
			LineStart: line, LineEnd: line + 9, Confidence: 0.76,
		}
	}
	restriction := adjacent("r11-cpu", "cpu_affinity_or_cpuset", 29.555, 60)
	restriction.ChainAnchorRemainderSeat, restriction.ChainAnchoredMS, restriction.ChainAnchorFullMS = true, 7.413, 36.968
	restriction.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 7.413,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-chain"}},
	}
	remainder := adjacent("r11-rem", "runnable_wait", 68.749, 50)
	remainder.ChainAnchorRemainderSeat, remainder.ChainAnchoredMS, remainder.ChainAnchorFullMS = true, 7.413, 105.717
	remainder.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 7.413,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-chain"}},
		{Part: string(tracequery.RootCauseShareClaimSubset), Role: string(tracequery.RootCauseShareHolderSubsetSeat), ShareMS: 29.555,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-cpu"}},
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"app-100"},
		WindowStartTs:           10.0,
		WindowEndTs:             11.0,
		OnChainCauses: []types.TraceCausalProjectionNode{chain,
			elimChainNode("r11-dio", "worker-300", "d_state_or_io_wait", "d_sleep", 6, 3.0, 600)},
		AdjacentCauses: []types.TraceCausalProjectionNode{remainder, restriction},
	}
}

func TestHMC377R11SubsetShareHandShape(t *testing.T) {
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(hmc377R11SubsetShareProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := strings.Join(strings.Fields(runtimeTraceProjTreeFence(model, zh)), " ")
		cpu := xlane2FindRowByEvidenceID(&model, "r11-cpu")
		if cpu == nil || cpu.EvidenceTag == "" {
			t.Fatal("the restriction row must be drawn")
		}
		want := "其余98.304ms(无链上凭证;其中CPU亲和/cpuset限制下29.555ms另行计入[" + cpu.EvidenceTag + "],本行计68.749ms)"
		if !zh {
			want = "remainder 98.304ms (no chain credential; of which 29.555ms under the CPU affinity/cpuset restriction is counted separately on [" + cpu.EvidenceTag + "], this row 68.749ms)"
		}
		if !strings.Contains(strings.ReplaceAll(fence, "cpuset 限制", "cpuset限制"), want) {
			t.Fatalf("zh=%t: %q missing:\n%s", zh, want, fence)
		}
		if strings.Contains(fence, "合计还原全窗账 105.717ms") {
			t.Fatalf("zh=%t: the remainder must not claim the two-seat restoration:\n%s", zh, fence)
		}
		if !model.Marks.has(runtimeTraceProjMarkChainAnchorSubsetShare) {
			t.Fatalf("zh=%t: the 另行计入 mark must be set", zh)
		}
	}
}
