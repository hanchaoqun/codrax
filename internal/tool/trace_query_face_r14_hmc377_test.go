package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
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

// hmc377R14AbsorbedRemainderProjection is the engine-minted n21w shape (105
// 网易云音乐 应用后台销毁后冷启动, board of binder:3447_2-34067, thread
// udk-irq-6-75; native board at cff599c7c
// ~/hmc-work/hmc377/fix13/merge/sweep/out/new/m13/m13_n21w.root_cause_rank.{native,observations}.json):
// the ⛓ chain seat #10 anchors 0.263 of the thread's 1.372 ms ready account
// and claims the 1.109 ms rest (remainder / remainder_seat) — which the engine
// resolved to the thread's CPU-restriction row #9, because that row counts the
// whole remainder: the ◇ remainder seat was carved to nothing and absorbed
// into the restriction row (rank 0, chain_anchor_remainder_seat, its subset
// claim naming the restriction row). The absorbed seat rides on the
// projection's absorbed rows, never as a drawn row. ownRow draws the
// restriction row as its own adjacent row (n21w udk-irq-1-66 [E17]); otherwise
// it is drawn only as a member of a subjectless overflow fold whose seed id is
// its record (n21w 「◦ 其余 4 项(折叠) 16.183ms [E22(+6)]」).
func hmc377R14AbsorbedRemainderProjection(ownRow bool) types.TraceCausalProjection {
	const subject, family = "udk-irq-6-75", "rank_pair:cpu_affinity_or_cpuset>runnable_wait|pid:75|adjacent"
	chain := elimChainNode("r14-chain", subject, "runnable_wait", "runnable", 10, 0.263, 63801)
	chain.ChainAnchorFullMS, chain.ChainAnchoredMS = 1.372, 0.263
	chain.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimRemainder), Role: string(tracequery.RootCauseShareHolderRemainderSeat), ShareMS: 1.109,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r14-cpu"}},
	}
	adjacent := func(id, who, typ string, value float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: who, Predicate: "root_cause_adjacent", Object: typ, TypeToken: typ, StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 9, Tier: "tertiary",
			ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value,
			LineStart: line, LineEnd: line + 9, Confidence: 0.72,
		}
	}
	cpu := adjacent("r14-cpu", subject, "cpu_affinity_or_cpuset", 1.109, 63802)
	cpu.RankFamilyKey, cpu.RestrictedReadyAccountMS, cpu.ShareCountedElsewhereMS, cpu.ShareCountedHereMS = family, 1.372, 0.263, 1.109
	cpu.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderResidualSeat), ShareMS: 0.263, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r14-chain"}},
	}
	if !ownRow {
		cpu.Subject, cpu.Object, cpu.TypeToken, cpu.RankFamilyKey, cpu.ShareClaims = "", "", "", "", nil
		cpu.RestrictedReadyAccountMS, cpu.ShareCountedElsewhereMS, cpu.ShareCountedHereMS = 0, 0, 0
		cpu.ImpactMS, cpu.CumulativeImpactMS, cpu.EffectiveImpactMS, cpu.RunnableMS = 16.183, 16.183, 0, 0
		cpu.MergedEvidenceIDs, cpu.MergedCount, cpu.MergedMinMS, cpu.MergedMaxMS = []string{"r14-other-a", "r14-other-b"}, 3, 1.109, 16.183
		cpu.MergedSubjects, cpu.MergedMaxSubject = []string{subject, "ease.cloudmusic-34044", "RenderThread-28260"}, "ease.cloudmusic-34044"
	}
	rem := adjacent("r14-rem", subject, "runnable_wait", 0, 63801)
	rem.Predicate, rem.Tier, rem.Rank = "root_cause_absorbed", "absorbed", 0
	rem.ChainAnchorRemainderSeat, rem.ChainAnchoredMS, rem.ChainAnchorFullMS, rem.ShareCountedElsewhereMS = true, 0.263, 1.372, 1.109
	rem.AbsorbedByRankFamily, rem.AbsorbedInto = true, family
	rem.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderResidualSeat), ShareMS: 0.263, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r14-chain"}},
		{Part: string(tracequery.RootCauseShareClaimSubset), Role: string(tracequery.RootCauseShareHolderSubsetSeat), ShareMS: 1.109, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r14-cpu"}},
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"app-100"}, WindowStartTs: 5770.663867, WindowEndTs: 5770.863867,
		OnChainCauses: []types.TraceCausalProjectionNode{chain}, AdjacentCauses: []types.TraceCausalProjectionNode{cpu},
		AbsorbedChainRows: []types.TraceCausalProjectionNode{rem},
	}
}

// R-377-14 ⑦ (review-13 R13P-4, completes R-377-13 ④): the ⛓ row's rest reads
// its carrier's kind from the engine item — the absorbed remainder seat's
// subset claim names the row that counts the rest a CPU-restriction seat —
// never from the rows that happen to be drawn. Real window at cff599c7c and
// 532dc208d (new n21w, fence line 150): 「⧖ udk-irq-6-75 · runnable 0.263ms
// [E11]」 said 「同源二分:全窗1.372ms=本行锚定0.263ms+其余1.109ms(◇余段席)」
// while no ◇ row of the thread is drawn: the remainder seat is an absorbed
// item of restriction row #9, drawn only inside 「◦ 其余 4 项(折叠) [E22(+6)]」.
// The holder kind was learnt only from subset claims on drawn rows, so the
// absorbed seat's claim was never read.
func TestHMC377R14RestCarrierOfAnAbsorbedRemainderSeat(t *testing.T) {
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		t.Run(name+"/restriction drawn only inside a fold row", func(t *testing.T) {
			model, fence := hmc377R12Fence(t, hmc377R14AbsorbedRemainderProjection(false), zh)
			chain := xlane2FindRowByEvidenceID(&model, "r14-chain")
			if chain == nil || chain.EvidenceTag == "" || len(model.absorbedChainRows) != 1 {
				t.Fatalf("the ⛓ row is drawn and the absorbed remainder seat rides the model:\n%s", fence)
			}
			for _, row := range runtimeTraceProjSMR1AllRows(&model) {
				if row.Node.ChainAnchorRemainderSeat {
					t.Fatalf("fixture: no ◇ remainder row of the thread is drawn:\n%s", fence)
				}
			}
			joined := hmc377R12Joined(fence)
			plain, seeDetail := "其余1.109ms(◇余段席)", "其余1.109ms(◇余段席见明细)"
			if !zh {
				plain, seeDetail = "remainder 1.109ms (◇ remainder seat)", "remainder 1.109ms (◇ remainder seat: see the detail index)"
			}
			if strings.Contains(joined, plain) {
				t.Errorf("the rest names a ◇ remainder seat (%q) though no ◇ row of the thread is drawn and the folded restriction row counts the 1.109ms:\n%s", plain, fence)
			}
			if !strings.Contains(joined, seeDetail) {
				t.Errorf("expected the see-detail form %q:\n%s", seeDetail, fence)
			}
			if chain.ChainAnchorRestSeeDetail != true || len(chain.ChainAnchorRestCarriers) != 0 {
				t.Errorf("the row stamps the see-detail form and names no carrier: %+v / %t", chain.ChainAnchorRestCarriers, chain.ChainAnchorRestSeeDetail)
			}
		})
		// The same engine shape with the restriction row drawn as its own
		// row names it by [E#] (n21w udk-irq-1-66 「其余3.224ms(CPU亲和/cpuset
		// 限制[E17]计入)」): the absorbed seat's claim and the drawn row agree.
		t.Run(name+"/restriction drawn as its own row", func(t *testing.T) {
			model, fence := hmc377R12Fence(t, hmc377R14AbsorbedRemainderProjection(true), zh)
			cpu := xlane2FindRowByEvidenceID(&model, "r14-cpu")
			if cpu == nil || cpu.EvidenceTag == "" {
				t.Fatalf("the restriction row is drawn:\n%s", fence)
			}
			want := "其余1.109ms(CPU亲和/cpuset限制[" + cpu.EvidenceTag + "]计入)"
			if !zh {
				want = "remainder 1.109ms (counted by the CPU affinity/cpuset restriction [" + cpu.EvidenceTag + "])"
			}
			if joined := hmc377R12Joined(fence); !strings.Contains(joined, want) {
				t.Fatalf("%q missing:\n%s", want, fence)
			}
		})
	}
}
