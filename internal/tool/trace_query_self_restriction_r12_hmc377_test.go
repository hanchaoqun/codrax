package tool

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 12 (batch-owner ruling R-377-12 ④, 2026-11-22): the analysed
// thread's restriction row is a non-additive 「其中」 line under its self ready
// account, keeping its [E#]. Real windows at fa2f01465 (review-population
// logs/final/tgtcpu_head_dio.txt, tools/tgtcpu11.py): 15 bind windows w16–w30
// drew the self whole-ready-account row and the self restriction row as two
// additive rows, e.g. w24 unyuan.app.chat-60814 「⧖ 自身·优先级反转候选·runnable
// 合计5.905ms [E6]」 beside 「⧖ 自身·CPU亲和/cpuset限制 0.650ms [E7]」 on 5.905ms
// of raw ready time (out/dio/head/head_w24.fence_zh.txt; 11 of the 15 already
// at main d5ef1c6fe).

// R-377-12 ④: the analysed thread's CPU-restriction self row is drawn as a
// non-additive 「其中」 line under its self ready account, keeping its [E#];
// it stays a ◎ member (◎ is unchanged).
func TestHMC377R12SelfRestrictionIsAnOfWhichLine(t *testing.T) {
	const target = "unyuan.app.chat-60814"
	self := func(id, typ string, impact, eff float64, rank, line int) types.TraceCausalProjectionNode {
		node := elimChainNode(id, target, typ, "runnable", rank, eff, line)
		node.ImpactMS, node.CumulativeImpactMS, node.RunnableMS = impact, impact, impact
		node.Causality, node.OnChainBasis = "self_wall_clock", "self_wall_clock_interval"
		return node
	}
	ready := self("r12-self-ready", "priority_inversion_runnable_wait", 5.905, 1.734, 13, 47110)
	restriction := self("r12-self-cpu", "cpu_affinity_or_cpuset", 0.650, 0.650, 16, 47114)
	dep := elimChainNode("r12-dep", "RSUniRenderThre-1783", "runnable_wait", "runnable", 1, 3.0, 100)
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"RSUniRenderThre-1783", target}, WindowStartTs: 10.0, WindowEndTs: 11.0,
		OnChainCauses: []types.TraceCausalProjectionNode{dep, ready, restriction},
	}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		readyRow, cpuRow := xlane2FindRowByEvidenceID(&model, "r12-self-ready"), xlane2FindRowByEvidenceID(&model, "r12-self-cpu")
		if readyRow == nil || cpuRow == nil || readyRow.EvidenceTag == "" || cpuRow.EvidenceTag == "" {
			t.Fatalf("%s: both self rows keep their evidence numbers", name)
		}
		t.Run(name+"/one 其中 line, no additive row", func(t *testing.T) {
			lines := strings.Split(fence, "\n")
			head := regexp.MustCompile(`⧖\s+(自身|own)·CPU`)
			for _, l := range lines {
				if head.MatchString(l) {
					t.Fatalf("the restriction must not be drawn as a second self row:\n%s", fence)
				}
			}
			want := "其中CPU亲和/cpuset限制下0.650ms是本线程runnable时间里受限的那一部分,已含在上述自身runnable账内,不另计 [" + cpuRow.EvidenceTag + "]"
			if !zh {
				want = "of which 0.650ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside its own runnable account above, never added again [" + cpuRow.EvidenceTag + "]"
			}
			joined := hmc377R12Joined(fence)
			if zh {
				// A zh wrap may break inside a word: compare without spaces.
				joined, want = strings.ReplaceAll(joined, " ", ""), strings.ReplaceAll(want, " ", "")
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("%q missing:\n%s", want, fence)
			}
			// The line sits under its host row, before the next self row.
			hostAt, lineAt := -1, -1
			for i, l := range lines {
				if strings.Contains(l, "["+readyRow.EvidenceTag+"]") && hostAt < 0 {
					hostAt = i
				}
				if strings.Contains(l, "["+cpuRow.EvidenceTag+"]") {
					lineAt = i
				}
			}
			if hostAt < 0 || lineAt <= hostAt {
				t.Fatalf("the 其中 line must follow its host row: host %d line %d\n%s", hostAt, lineAt, fence)
			}
			for i := hostAt + 1; i < lineAt; i++ {
				if regexp.MustCompile(`^│\s{5}\S`).MatchString(lines[i]) {
					t.Fatalf("another self row sits between the host and its 其中 line:\n%s", fence)
				}
			}
		})
		t.Run(name+"/◎ unchanged", func(t *testing.T) {
			onBoard := false
			for _, entry := range runtimeTraceProjElimBoard(model) {
				if entry.row.Node.EvidenceID == "r12-self-cpu" {
					onBoard = true
				}
			}
			if !onBoard {
				t.Fatal("the restriction row keeps its ◎ seat")
			}
		})
	}
}
