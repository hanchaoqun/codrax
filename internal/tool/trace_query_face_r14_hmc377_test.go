package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 14 (batch-owner ruling R-377-14, 2026-11-24), lane face: a
// badge sits beside its own rank row's value (③), the 「其中」 line keeps the
// CPU-constraint description (④) and the ⛓ rest reads its carrier's kind from
// the engine item (⑦).

// hmc377R14Constrain stamps the engine's typed CPU-constraint judgment on a
// restriction row, as the rank wire carries it (cpu_constraint_* notes).
func hmc377R14Constrain(node types.TraceCausalProjectionNode, kind string, allowed, excluded []int) types.TraceCausalProjectionNode {
	node.CPUConstraintKind, node.CPUConstraintAllowedCPUs, node.CPUConstraintExcludedCPUs = kind, allowed, excluded
	return node
}

// hmc377R14DescriptionLines returns the tree lines that open a CPU-constraint
// description, each with its wrapped continuation joined.
func hmc377R14DescriptionLines(lines []string, zh bool) []string {
	head := "CPU约束描述:"
	if !zh {
		head = "CPU-constraint description: "
	}
	var out []string
	for i, l := range lines {
		at := strings.Index(l, "· "+head)
		if at < 0 {
			continue
		}
		text := l[at+len("· "):]
		for j := i + 1; j < len(lines) && !hmc377R13RowHead(lines[j]) && !strings.Contains(lines[j], "· "); j++ {
			text += " " + strings.TrimSpace(strings.TrimLeft(lines[j], "│ "))
		}
		out = append(out, text)
	}
	return out
}

// R-377-14 ④ (review-13 R13P-2): wherever the analysed thread's restriction is
// a 「其中」 line, the engine's CPU-constraint judgment — allowed cores,
// excluded observed cores, basis — is printed with it, from the same typed
// fields and in the words the row form uses. Supply-side facts are
// must-not-lose. Real windows at cff599c7c
// (~/hmc-work/hmc377/fix13/review-population/logs/own/cpudesc_head.tsv): 105
// 元宝 终端重启后冷启动 #1 pool-8-thread-1-10807 (y51r, flat board), rank 1,
// 136.219 ms: main and 532dc208d printed 「· CPU约束描述:允许核 4-7 · 排除全域
// 观测核 0-3 · 判定依据 sched_switch_next_info」 under the row; the 「其中」 line
// printed nothing and the cores were on no face of the document (27 of 275
// windows, 14 of the fifth 40; self-stanza form e.g. zr05 #tp-default-716-9853
// rank 1, 126.941 ms).
func TestHMC377R14OfWhichLineKeepsTheCPUConstraintDescription(t *testing.T) {
	const kind = "sched_switch_next_info"
	t.Run("flat board", func(t *testing.T) {
		const target, tid = "pool-8-thread-1-10807", 10807
		cpu := hmc377R14Constrain(hmc377R13Rank("r14-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 1, 136.219, 136.219, 100), kind, []int{4, 5, 6, 7}, []int{0, 1, 2, 3})
		inv := hmc377R13Rank("r14-inv", target, tid, tid, "priority_inversion_runnable_wait", 3, 128.026, 23.474, 200)
		ready := hmc377R13Rank("r14-ready", target, tid, tid, "runnable_wait", 5, 8.193, 8.193, 400)
		// Another thread's restriction row keeps its row: its description is
		// the row form's — the words the 「其中」 line must use.
		other := hmc377R13Rank("r14-other", "PPS-IO-pool-thr-10892", 10892, tid, "runnable_wait", 4, 20.0, 20.0, 500)
		otherCPU := hmc377R14Constrain(hmc377R13Rank("r14-other-cpu", "PPS-IO-pool-thr-10892", 10892, tid, "cpu_affinity_or_cpuset", 6, 12.0, 12.0, 600), kind, []int{4, 5, 6, 7}, []int{0, 1, 2, 3})
		projection := types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
			OnChainCauses: []types.TraceCausalProjectionNode{cpu, inv, ready, other, otherCPU},
		}
		for _, zh := range []bool{true, false} {
			name := map[bool]string{true: "zh", false: "en"}[zh]
			model, fence := hmc377R12Fence(t, projection, zh)
			cpuRow, otherCPURow := xlane2FindRowByEvidenceID(&model, "r14-cpu"), xlane2FindRowByEvidenceID(&model, "r14-other-cpu")
			if cpuRow == nil || otherCPURow == nil || strings.TrimSpace(model.Target) != "" {
				t.Fatalf("%s: a flat board with both restriction rows in the model", name)
			}
			lines := hmc377R13TreeLines(fence)
			want := "CPU约束描述:允许核 4-7 · 排除全域观测核 0-3 · 判定依据 sched_switch_next_info"
			if !zh {
				want = "CPU-constraint description: allowed CPUs 4-7 · excludes observed CPUs 0-3 · basis sched_switch_next_info"
			}
			descriptions := hmc377R14DescriptionLines(lines, zh)
			if len(descriptions) != 2 || descriptions[0] != want || descriptions[1] != want {
				t.Fatalf("%s: the 其中 line and the other thread's row each print %q (one wording), got %q:\n%s", name, want, descriptions, fence)
			}
			// The description follows the 「其中」 line inside the same block:
			// after the line's [E#], before the next row head.
			lineAt, descAt, nextHead := -1, -1, -1
			for i, l := range lines {
				if lineAt < 0 && !hmc377R13RowHead(l) && strings.Contains(l, "["+cpuRow.EvidenceTag+"]") {
					lineAt = i
				}
				if lineAt >= 0 && descAt < 0 && i > lineAt && strings.Contains(l, strings.SplitN(want, " ", 2)[0]) {
					descAt = i
				}
				if lineAt >= 0 && nextHead < 0 && i > lineAt && hmc377R13RowHead(l) {
					nextHead = i
				}
			}
			if lineAt < 0 || descAt != lineAt+1 || (nextHead >= 0 && descAt > nextHead) {
				t.Fatalf("%s: the description is the line right after the 其中 line (%d), got %d (next row %d):\n%s", name, lineAt, descAt, nextHead, fence)
			}
		}
	})
	t.Run("self stanza", func(t *testing.T) {
		const target, tid = "tquic_network_t-46761", 46761
		self := func(id, typ string, rank int, impact, eff float64, line int) types.TraceCausalProjectionNode {
			node := hmc377R13Rank(id, target, tid, tid, typ, rank, impact, eff, line)
			node.Causality, node.OnChainBasis = "self_wall_clock", "self_wall_clock_interval"
			return node
		}
		edge := self("r14-edge", "runnable_wait", 2, 112.009, 112.009, 100)
		dio := self("r14-dio", "d_state_or_io_wait", 8, 0.644, 0.644, 500)
		dio.StateKind = "d_sleep"
		dep := hmc377R13Rank("r14-dep", "Thread-28-46781", 46781, tid, "runnable_wait", 4, 4.406, 4.406, 700)
		for _, c := range []struct {
			name             string
			allowed, exclude []int
			zh, en           string
		}{
			// The 21 self-stanza windows of the 275 carry the basis word only
			// (dio w16..w30, n07w, y41c..y54c).
			{"basis only", nil, nil, "CPU约束描述:判定依据 sched_switch_next_info", "CPU-constraint description: basis sched_switch_next_info"},
			// zr05: a core list on a rank-1 cause.
			{"core list", []int{4, 5, 6, 7}, []int{0, 1, 2, 3},
				"CPU约束描述:允许核 4-7 · 排除全域观测核 0-3 · 判定依据 sched_switch_next_info",
				"CPU-constraint description: allowed CPUs 4-7 · excludes observed CPUs 0-3 · basis sched_switch_next_info"},
		} {
			cpu := hmc377R14Constrain(self("r14-cpu", "cpu_affinity_or_cpuset", 13, 0.129, 0.129, 600), kind, c.allowed, c.exclude)
			projection := types.TraceCausalProjection{
				RootCauseFamilyObserved: true, WakeupPath: []string{"Thread-28-46781", target}, WindowStartTs: 3734.575541, WindowEndTs: 3734.775541,
				OnChainCauses: []types.TraceCausalProjectionNode{dep, edge, dio, cpu},
			}
			for _, zh := range []bool{true, false} {
				model, fence := hmc377R12Fence(t, projection, zh)
				cpuRow := xlane2FindRowByEvidenceID(&model, "r14-cpu")
				if cpuRow == nil || len(model.SelfRows) == 0 {
					t.Fatalf("%s zh=%t: the restriction row is a self row of the model", c.name, zh)
				}
				want := c.zh
				if !zh {
					want = c.en
				}
				lines := hmc377R13TreeLines(fence)
				descriptions := hmc377R14DescriptionLines(lines, zh)
				if len(descriptions) != 1 || descriptions[0] != want {
					t.Fatalf("%s zh=%t: the 其中 line prints %q, got %q:\n%s", c.name, zh, want, descriptions, fence)
				}
				lineAt := -1
				for i, l := range lines {
					if !hmc377R13RowHead(l) && strings.Contains(l, "["+cpuRow.EvidenceTag+"]") {
						lineAt = i
					}
				}
				if lineAt < 0 || lineAt+1 >= len(lines) || !strings.Contains(lines[lineAt+1], strings.SplitN(want, " ", 2)[0]) || hmc377R13RowHead(lines[lineAt+1]) {
					t.Fatalf("%s zh=%t: the description is the note line right after the 其中 line:\n%s", c.name, zh, fence)
				}
			}
		}
	})
	// A restriction row the engine minted no constraint judgment for prints no
	// description (absence never guesses).
	t.Run("no judgment, no line", func(t *testing.T) {
		const target, tid = "pool-8-thread-1-10807", 10807
		cpu := hmc377R13Rank("r14-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 1, 136.219, 136.219, 100)
		ready := hmc377R13Rank("r14-ready", target, tid, tid, "runnable_wait", 5, 8.193, 8.193, 400)
		_, fence := hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
			OnChainCauses: []types.TraceCausalProjectionNode{cpu, ready},
		}, true)
		if !strings.Contains(fence, "其中CPU亲和") || strings.Contains(fence, "CPU约束描述") {
			t.Fatalf("the 其中 line stands alone when the row carries no constraint judgment:\n%s", fence)
		}
	})
}
