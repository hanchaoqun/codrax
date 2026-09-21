package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Account relations retain their two measurements. A display hull is only
// their location: even overlapping hulls cannot prove that the measured
// members intersect, and absent timestamps cannot prove either relation.
func TestTraceAccountRelationUnknownProjectionDisplay(t *testing.T) {
	for _, shape := range []string{"family_hole", "missing_timestamps", "same_line_accounts"} {
		for _, rename := range []bool{false, true} {
			for _, zh := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/renamed=%t/zh=%t", shape, rename, zh), func(t *testing.T) {
					projection := traceAccountRelationUnknownProjection(t, shape)
					if rename {
						for i := range projection.OnChainCauses {
							projection.OnChainCauses[i].Subject = fmt.Sprintf("renamed-worker-%d", i/2)
						}
						// Keep the two accounting rows owned by the same thread.
						last := len(projection.OnChainCauses) - 1
						projection.OnChainCauses[last].Subject = projection.OnChainCauses[last-1].Subject
						projection.WakeupPath = []string{"renamed-source-8", "renamed-target-9"}
					}
					before, _ := json.Marshal(projection)
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
					var related []*runtimeTraceProjTreeRow
					for _, row := range runtimeTraceProjSMR1AllRows(&model) {
						if row.AccountRelRef != "" && row.AccountRelSameSourceFullMS == 0 {
							related = append(related, row)
							if row.AccountRelDisjoint {
								t.Fatal("an overlapping or missing hull cannot prove disjointness")
							}
						}
					}
					if len(related) != 2 || related[0].AccountRelRef != strings.TrimSpace(related[1].EvidenceTag) ||
						related[1].AccountRelRef != strings.TrimSpace(related[0].EvidenceTag) {
						t.Fatalf("both existing account rows and their reciprocal identities must survive: %+v", related)
					}
					rowBefore, _ := json.Marshal([]types.TraceCausalProjectionNode{related[0].Node, related[1].Node})
					fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
					want := "实际区间关系未证(不能直接相加)"
					if !zh {
						want = "actual interval relation unproven (do not add directly)"
					}
					if strings.Count(strings.ReplaceAll(fence, " ", ""), strings.ReplaceAll(want, " ", "")) != 2 {
						t.Errorf("both account rows must disclose the unknown relation %q:\n%s", want, fence)
					}
					for _, wrong := range []string{"物理时间重叠(不可相加)", "physical time overlaps (never additive)", "物理时间不相交·账目关系", "physical time disjoint · account relation"} {
						if rspaFenceContains(fence, wrong) {
							t.Errorf("location envelopes minted a physical relation %q:\n%s", wrong, fence)
						}
					}
					legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, zh), "\n") + "\n" +
						strings.Join(runtimeTraceProjReaderLegendLines(model.Marks, zh, false), "\n")
					for _, wrong := range []string{"物理时间重叠或不相交,行内句按 typed 区间推导", "different coverage sets, overlapping physical time", "state whether physical time overlaps"} {
						if strings.Contains(legend, wrong) {
							t.Errorf("legend reintroduced a stronger relation: %s", wrong)
						}
					}
					rowAfter, _ := json.Marshal([]types.TraceCausalProjectionNode{related[0].Node, related[1].Node})
					after, _ := json.Marshal(projection)
					if string(before) != string(after) || string(rowBefore) != string(rowAfter) {
						t.Fatal("display wording changed source measurements, values, rank or relation carriers")
					}
				})
			}
		}
	}
}

func traceAccountRelationUnknownProjection(t *testing.T, shape string) types.TraceCausalProjection {
	t.Helper()
	if shape == "same_line_accounts" {
		return smr1C1AccountPairProjection()
	}
	projection := smr1C1FamilyChainProjection()
	if shape == "missing_timestamps" {
		return projection
	}
	// A two-member family has a hole containing the other account's interval.
	// The real intervals are deliberately disjoint; the display retains only
	// their outer envelope, as the projection wire does for a family row.
	familyMembers := [][2]float64{{1, 1.002}, {1.008, 1.012}}
	peer := [2]float64{1.004, 1.005}
	for _, member := range familyMembers {
		if types.TraceCausalProjectionIntervalsOverlap(member[0], member[1], peer[0], peer[1]) {
			t.Fatal("fixture must have no physical member intersection")
		}
	}
	projection.WindowStartTs, projection.WindowEndTs = 1, 1.02
	family, chain := &projection.OnChainCauses[1], &projection.OnChainCauses[2]
	family.StartTs, family.EndTs = familyMembers[0][0], familyMembers[1][1]
	family.ImpactMS, family.CumulativeImpactMS = 6, 6
	family.FamilyMemberCount, family.FamilyMemberMinMS, family.FamilyMemberMaxMS = 2, 2, 4
	family.FamilyMemberSumMS = 6
	chain.StartTs, chain.EndTs = peer[0], peer[1]
	chain.ImpactMS, chain.CumulativeImpactMS = 1, 1.5
	for _, node := range []*types.TraceCausalProjectionNode{family, chain} {
		node.QueryWindowStartTs, node.QueryWindowEndTs = 1, 1.02
	}
	return projection
}

func TestTraceAccountRelationKnownProjectionDisplayPreserved(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, tc := range []struct {
			name       string
			projection types.TraceCausalProjection
			zhWant     string
			enWant     string
		}{
			{"disjoint", smr1C1DisjointPairProjection(), "物理时间不相交·账目关系", "physical time disjoint · account relation"},
			{"same_source", rspaSameSourceSplitProjection(), "合计还原全窗账 36.757ms", "restores the full-window account 36.757ms"},
			{"exact_cross_direction", axiomv2CrossDirectionProjection(), "同段重叠", "overlaps ["},
		} {
			t.Run(fmt.Sprintf("%s/zh=%t", tc.name, zh), func(t *testing.T) {
				before, _ := json.Marshal(tc.projection)
				model := buildRuntimeTraceProjTreeModel(tc.projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, zh))
				want := tc.zhWant
				if !zh {
					want = tc.enWant
				}
				if !rspaFenceContains(fence, want) {
					t.Errorf("exact typed relation lost its own wording %q:\n%s", want, fence)
				}
				for _, row := range runtimeTraceProjSMR1AllRows(&model) {
					if tc.name == "disjoint" && row.AccountRelRef != "" && !row.AccountRelDisjoint {
						t.Fatal("disjoint proof was erased")
					}
					if tc.name == "same_source" && row.AccountRelRef != "" && row.AccountRelSameSourceFullMS != 36.757 {
						t.Fatal("same-source full account changed")
					}
				}
				after, _ := json.Marshal(tc.projection)
				if string(before) != string(after) {
					t.Fatal("display changed the typed projection")
				}
			})
		}
	}
}
