package tool

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 12 (R11P-2, owner ruling R-377-12 ⑤): two rulers of one
// thread's same-state account are never summed. On real windows the rank
// board publishes the analysed thread's ready time as window_stats census
// seats (which add up to the census account) and as a wakeup_chain seat
// inside that account; the projection's ×N same-kind merge summed both
// rulers, and .root-causes.json, the tree and ◎ published cpu_scheduling_delay
// above the window or above the thread's ready time
// (hmc377/fix11/review-population/raw/evidence/twoRuler_{n20w,n22w,n24w}.txt).
// Opt-in per capture (skip when the variable is unset, fail when byte size or
// SHA-256 differs). The ready totals are the independent raw state machine's
// (hmc377/fix11/review-population/tools/rawstate.py over the ftrace text).

func TestHMC377R12WholeOriginalTwoRulerMergeNeverSums(t *testing.T) {
	cases := []struct {
		name, env, digest string
		size              int64
		tid               int
		start, end        float64
		rawReadyMS        float64 // rawstate.py
		drawn             bool    // the merged row renders on the tree and ◎
	}{
		{
			// n24w: census 158.904 + 0.123, wakeup_chain seat 32.616;
			// fa2f01465 published 191.643 on the tree, ◎ and side file.
			name: "device-reboot cold start, pool-5-thread-1", env: "CODRAX_HMC376_NETEASE_REBOOT_FRAME",
			digest: "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", size: 104144939,
			tid: 11405, start: 149.811770, end: 150.011770, rawReadyMS: 167.116, drawn: true,
		},
		{
			// n20w: census 169.059 + 0.934, wakeup_chain seat 100.748;
			// fa2f01465 published 270.741 in the side file for a 200 ms window.
			name: "container-restart cold start, ty.MainActivity", env: "CODRAX_HMC376_NETEASE_CONTAINER_FRAME",
			digest: "45e304445ea5f661a21c36df65b87863e48142e26e82bfab2fc5a9d582a408f8", size: 95407103,
			tid: 43012, start: 2149.324822, end: 2149.524822, rawReadyMS: 169.993,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := hmc376Original(t, c.env, c.digest, c.size)
			faces := hmc377R11QueryPID(t, path, c.tid, c.start, c.end)
			window := (c.end - c.start) * 1000
			subject := fmt.Sprintf("-%d", c.tid)
			ownReady := func(node types.TraceCausalProjectionNode) bool {
				return strings.HasSuffix(strings.TrimSpace(node.Subject), subject) && strings.TrimSpace(node.Object) == "runnable_wait"
			}
			var merged []types.TraceCausalProjectionNode
			t.Run("projection", func(t *testing.T) {
				for _, bucket := range [][]types.TraceCausalProjectionNode{faces.projection.PrimaryRootCauses, faces.projection.RankedSeats, faces.projection.OnChainCauses} {
					for _, node := range bucket {
						if !ownReady(node) {
							continue
						}
						if node.EffectiveImpactMS > c.rawReadyMS+0.0025 || node.EffectiveImpactMS > window+0.0015 {
							t.Errorf("%s publishes %.3f ms of the thread's ready time; raw ready %.3f, window %.3f (merged=%d, ids=%v)",
								node.EvidenceID, node.EffectiveImpactMS, c.rawReadyMS, window, node.MergedCount, node.MergedEvidenceIDs)
						}
						if len(node.MergedEvidenceIDs) > 0 {
							merged = append(merged, node)
						}
					}
				}
				if len(merged) == 0 {
					t.Fatal("prerequisite: the census seats and the wakeup_chain seat share one projection row")
				}
			})
			t.Run("side_file", func(t *testing.T) {
				examined := 0
				defer func() {
					if examined == 0 {
						t.Error("prerequisite: the side file carries the thread's runnable_wait candidate")
					}
				}()
				for _, cand := range faces.candidates {
					d := cand.Decision
					if d.Magnitude == nil || !strings.HasSuffix(strings.TrimSpace(d.SubjectName), subject) || d.Token.Token != "runnable_wait" {
						continue
					}
					examined++
					if d.Magnitude.Value > c.rawReadyMS+0.0025 || d.Magnitude.Value > window+0.0015 {
						t.Errorf("side-file runnable_wait of TID %d = %.3f ms; raw ready %.3f, window %.3f", c.tid, d.Magnitude.Value, c.rawReadyMS, window)
					}
				}
			})
			if !c.drawn || len(merged) == 0 {
				return
			}
			t.Run("tree_overview_side_file_agree", func(t *testing.T) {
				lead := merged[0]
				value := lead.EffectiveImpactMS
				var tree, overview, side []float64
				for _, row := range runtimeTraceProjSMR1AllRows(&faces.model) {
					if row.HasData && row.Node.EvidenceID == lead.EvidenceID {
						tree = append(tree, runtimeTraceProjNodeDisplayImpact(row.Node))
					}
				}
				for _, entry := range runtimeTraceProjElimBoard(faces.model) {
					if entry.row.Node.EvidenceID == lead.EvidenceID {
						overview = append(overview, entry.row.Node.EffectiveImpactMS)
					}
				}
				for _, cand := range faces.candidates {
					d := cand.Decision
					if d.Magnitude == nil || d.Token.Token != "runnable_wait" || !strings.HasSuffix(strings.TrimSpace(d.SubjectName), subject) {
						continue
					}
					for _, ref := range d.EvidenceRefs {
						if strings.Contains(ref, lead.EvidenceID) {
							side = append(side, d.Magnitude.Value)
							break
						}
					}
				}
				for face, values := range map[string][]float64{"tree": tree, "◎": overview, "side file": side} {
					if len(values) != 1 || math.Abs(values[0]-value) > 0.0015 {
						t.Errorf("%s carries %v for %s; the projection row publishes %.3f", face, values, lead.EvidenceID, value)
					}
				}
			})
		})
	}
}
