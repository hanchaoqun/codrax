package tool

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 13 (batch-owner ruling R-377-13, 2026-11-22), lane face: the
// analysed thread's restriction line on every face (②), badges that follow
// their seat (③) and the ⛓ rest carriers read from the engine (④).

// hmc377R13Rank builds a rank row of a board whose analysed thread is
// targetTID: the typed rank subjects the trace_query rank wire mints on every
// rank row (TraceRankSubjects).
func hmc377R13Rank(id, subject string, tid, targetTID int, typ string, rank int, impact, eff float64, line int) types.TraceCausalProjectionNode {
	node := elimChainNode(id, subject, typ, "runnable", rank, eff, line)
	node.ImpactMS, node.CumulativeImpactMS, node.RunnableMS = impact, impact, impact
	node.RankSubjects = &types.TraceRankSubjects{Target: types.TraceThreadCoordinate{TID: targetTID, TGID: 10600},
		Member: types.TraceThreadCoordinate{TID: tid, TGID: 10600}}
	return node
}

// hmc377R13TreeLines is the fence body between the head lines and the first
// stanza header: the tree's own lines.
func hmc377R13TreeLines(fence string) []string {
	var out []string
	for _, l := range strings.Split(fence, "\n") {
		if strings.HasPrefix(l, "◇ ") || strings.HasPrefix(l, "▒ ") {
			break
		}
		if strings.HasPrefix(l, "├─") || strings.HasPrefix(l, "└─") || strings.HasPrefix(l, "│") || strings.HasPrefix(l, "  ") {
			out = append(out, l)
		}
	}
	return out
}

// hmc377R13RowHead reports whether a tree line opens a row (its connector or
// the self stanza's row lead), as opposed to a row's "· " note line.
func hmc377R13RowHead(l string) bool {
	rails := strings.TrimLeft(l, "│ ")
	return strings.HasPrefix(rails, "├─") || strings.HasPrefix(rails, "└─") || regexp.MustCompile(`^│\s{5}\S`).MatchString(l)
}

// R-377-13 ②: on a flat board (the wakeup path did not resolve; no self
// stanza) the analysed thread's restriction row is the same non-additive
// 「其中」 line under its ready account as in the self stanza — typed by the
// rank subjects, never by the stanza. Real window at 532dc208d (review-12
// R12P-1, ~/hmc-work/hmc377/fix12/review-population/evidence/D2_flatboard_restriction.txt):
// 105 元宝 终端重启后冷启动 #1 pool-8-thread-1-10807 (y51r), raw ready 136.219:
// 「➊ ⧖ pool-8-thread-1-10807 · CPU亲和… 136.219ms [E1]」 drawn as an
// additive row beside the census account 「➌ … 优先级反转候选… 合计128.026ms
// [E3]」 + 「➎ … runnable 8.193ms [E6]」.
func TestHMC377R13FlatBoardRestrictionIsAnOfWhichLine(t *testing.T) {
	const target, tid = "pool-8-thread-1-10807", 10807
	cpu := hmc377R13Rank("r13-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 1, 136.219, 136.219, 100)
	inv := hmc377R13Rank("r13-inv", target, tid, tid, "priority_inversion_runnable_wait", 3, 128.026, 23.474, 200)
	sched := hmc377R13Rank("r13-sched", target, tid, tid, "scheduler_latency", 2, 66.376, 66.376, 300)
	ready := hmc377R13Rank("r13-ready", target, tid, tid, "runnable_wait", 5, 8.193, 8.193, 400)
	// Another thread's census account and restriction row on the same board:
	// that thread is not the board's analysed thread, so its rows stay rows.
	other := hmc377R13Rank("r13-other", "PPS-IO-pool-thr-10892", 10892, tid, "runnable_wait", 4, 20.0, 20.0, 500)
	otherCPU := hmc377R13Rank("r13-other-cpu", "PPS-IO-pool-thr-10892", 10892, tid, "cpu_affinity_or_cpuset", 6, 12.0, 12.0, 600)
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
		OnChainCauses: []types.TraceCausalProjectionNode{cpu, inv, sched, ready, other, otherCPU},
	}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		if strings.TrimSpace(model.Target) != "" || len(model.SelfRows) != 0 {
			t.Fatalf("%s: the board is flat (no self stanza)", name)
		}
		cpuRow, readyRow, otherCPURow := xlane2FindRowByEvidenceID(&model, "r13-cpu"), xlane2FindRowByEvidenceID(&model, "r13-ready"), xlane2FindRowByEvidenceID(&model, "r13-other-cpu")
		if cpuRow == nil || readyRow == nil || otherCPURow == nil || cpuRow.EvidenceTag == "" || readyRow.EvidenceTag == "" {
			t.Fatalf("%s: every row keeps its evidence number", name)
		}
		lines := hmc377R13TreeLines(fence)
		t.Run(name+"/one 其中 line, no additive row", func(t *testing.T) {
			for _, l := range lines {
				if hmc377R13RowHead(l) && strings.Contains(l, "["+cpuRow.EvidenceTag+"]") {
					t.Fatalf("the analysed thread's restriction must not be drawn as an additive row:\n%s", fence)
				}
			}
			want := "其中CPU亲和/cpuset限制下136.219ms是本线程runnable时间里受限的那一部分,已含在上述自身runnable账内,不另计 ➊ [" + cpuRow.EvidenceTag + "]"
			if !zh {
				want = "of which 136.219ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside its own runnable account above, never added again ➊ [" + cpuRow.EvidenceTag + "]"
			}
			joined := hmc377R12Joined(fence)
			if zh {
				joined, want = strings.ReplaceAll(joined, " ", ""), strings.ReplaceAll(want, " ", "")
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("%q missing:\n%s", want, fence)
			}
		})
		t.Run(name+"/the line hangs after the account block's last row", func(t *testing.T) {
			readyAt, lineAt, nextHead := -1, -1, -1
			for i, l := range lines {
				if hmc377R13RowHead(l) && strings.Contains(l, "["+readyRow.EvidenceTag+"]") {
					readyAt = i
				}
				if strings.Contains(l, "["+cpuRow.EvidenceTag+"]") {
					lineAt = i
				}
			}
			for i := readyAt + 1; readyAt >= 0 && i < len(lines); i++ {
				if hmc377R13RowHead(lines[i]) {
					nextHead = i
					break
				}
			}
			if readyAt < 0 || lineAt <= readyAt || (nextHead >= 0 && lineAt > nextHead) {
				t.Fatalf("the 其中 line follows the account's last row (%s) before the next row: ready %d line %d next %d\n%s",
					readyRow.EvidenceTag, readyAt, lineAt, nextHead, fence)
			}
		})
		t.Run(name+"/another thread's restriction keeps its row", func(t *testing.T) {
			found := false
			for _, l := range lines {
				if hmc377R13RowHead(l) && strings.Contains(l, "["+otherCPURow.EvidenceTag+"]") {
					found = true
				}
			}
			if !found {
				t.Fatalf("a thread that is not the board's analysed thread keeps its restriction row:\n%s", fence)
			}
		})
		t.Run(name+"/◎ unchanged", func(t *testing.T) {
			onBoard := false
			for _, entry := range runtimeTraceProjElimBoard(model) {
				onBoard = onBoard || entry.row.Node.EvidenceID == "r13-cpu"
			}
			if !onBoard {
				t.Fatal("the restriction row keeps its ◎ seat")
			}
		})
	}
	// No census account row of the thread on the board: the restriction row
	// keeps its own row (there is no account to be part of).
	t.Run("no account row", func(t *testing.T) {
		model, fence := hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
			OnChainCauses: []types.TraceCausalProjectionNode{sched, cpu},
		}, true)
		row := xlane2FindRowByEvidenceID(&model, "r13-cpu")
		drawn := false
		for _, l := range hmc377R13TreeLines(fence) {
			drawn = drawn || (row != nil && hmc377R13RowHead(l) && strings.Contains(l, "["+row.EvidenceTag+"]"))
		}
		if !drawn || strings.Contains(fence, "其中CPU亲和") {
			t.Fatalf("with no account row the restriction keeps its own row:\n%s", fence)
		}
	})
	// The restriction row drawn last on the lane (the smallest value — n18w
	// #ActionReaper-11345 0.074ms 「└─ … CPU亲和… [E12]」): the lane still
	// closes on its last drawn row.
	t.Run("the last row folded", func(t *testing.T) {
		small := hmc377R13Rank("r13-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 6, 0.074, 0.074, 100)
		_, fence := hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
			OnChainCauses: []types.TraceCausalProjectionNode{inv, sched, ready, small},
		}, true)
		var heads []string
		for _, l := range hmc377R13TreeLines(fence) {
			if hmc377R13RowHead(l) {
				heads = append(heads, l)
			}
		}
		if len(heads) != 3 || !strings.HasPrefix(heads[len(heads)-1], "└─") {
			t.Fatalf("three rows drawn, the last one closing the lane (└─):\n%s", fence)
		}
		for _, h := range heads[:len(heads)-1] {
			if !strings.HasPrefix(h, "├─") {
				t.Fatalf("only the last drawn row closes the lane:\n%s", fence)
			}
		}
		if !strings.Contains(strings.ReplaceAll(hmc377R12Joined(fence), " ", ""), "其中CPU亲和/cpuset限制下0.074ms") {
			t.Fatalf("the folded row is drawn as its 其中 line:\n%s", fence)
		}
	})
}

// hmc377R13RankRecord is one rank-board record of the analysed thread
// (TID 10616) as the trace_query rank wire publishes it: the typed rank
// subjects and the notes the projection compile reads (rank, type, values,
// mint source, board identity).
func hmc377R13RankRecord(rank int, typ, source string, ms float64, line int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:pub#root_cause_rank:%d", rank),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Subject:         "image-preload-c-10616",
		Predicate:       "root_cause_tertiary",
		Object:          typ,
		Value:           fmt.Sprintf("%.3f", ms),
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: line, LineEnd: line + 50},
		RichNotes: []string{
			fmt.Sprintf("rank=%d", rank), "type=" + typ,
			fmt.Sprintf("impact_ms=%.3f", ms), fmt.Sprintf("effective_impact_ms=%.3f", ms),
			"source=" + source, "dominant_state=runnable", "chain_relevance=on_chain",
			"selected_window=151.000327..151.200327", "rank_board_target=image-preload-c-10616", "rank_board_params_fingerprint=dd60cf59",
		},
		TraceRankSubjects: &types.TraceRankSubjects{Target: types.TraceThreadCoordinate{TID: 10616, TGID: 10600, Comm: "image-preload-c"},
			Member: types.TraceThreadCoordinate{TID: 10616, TGID: 10600, Comm: "image-preload-c"}},
	}
}

// R-377-13 ②: a wakeup_chain seat of the analysed thread's ready account is
// another ruler of that account, never one of its rows, so the 「其中」 line
// never hangs on it. Real window (y54r image-preload-c-10616, flat board,
// raw ready 30.669): the census account 「➊ … runnable 3条记录 合计30.669ms
// [E1]」, the restriction 「➋ … CPU亲和… 30.669ms [E4]」 and the wakeup_chain
// seat 「➍ … runnable 3条记录 成员最大27.312ms [E6]」 drawn below the account;
// keyed on the census families alone, the line hung under the 27.312 seat
// (~/hmc-work/hmc377/fix13/face/sweep/out/x40/f13/f13_y54r.fence_zh.txt, first
// head run).
func TestHMC377R13OfWhichSkipsTheOtherRuler(t *testing.T) {
	// As on y54r: the census account is the board's #1 (a primary record,
	// aggregated apart from the on-chain bucket — register R11D-0), so the two
	// rulers stay two drawn rows; both are engine family folds of three
	// members.
	census := hmc377R13RankRecord(1, "runnable_wait", "window_stats", 30.669, 181677)
	census.Predicate = "root_cause_primary"
	census.RichNotes = append(census.RichNotes, "member_count=3", "member_max_ms=27.312", "member_min_ms=1.334", "member_fold_caliber=sum_disjoint")
	seat := hmc377R13RankRecord(4, "runnable_wait", "wakeup_chain", 27.312, 181700)
	seat.RichNotes = append(seat.RichNotes, "member_count=3", "member_max_ms=27.312", "member_min_ms=1.203", "member_sum_ms=30.534", "member_fold_caliber=max_overlap_fallback")
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{
		census, hmc377R13RankRecord(2, "cpu_affinity_or_cpuset", "window_stats.cpu_constraints", 30.669, 183827), seat,
	})
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, projection, zh)
		census, cpu, seat := xlane2FindRowByEvidenceID(&model, "trace_query:pub#root_cause_rank:1"), xlane2FindRowByEvidenceID(&model, "trace_query:pub#root_cause_rank:2"), xlane2FindRowByEvidenceID(&model, "trace_query:pub#root_cause_rank:4")
		if census == nil || cpu == nil || seat == nil || !seat.Node.WakeupChainStateSeat() || census.Node.WakeupChainStateSeat() {
			t.Fatalf("zh=%t: the census row, the restriction row and the wakeup_chain seat are three drawn rows:\n%s", zh, fence)
		}
		lines := hmc377R13TreeLines(fence)
		censusAt, lineAt, seatAt := -1, -1, -1
		for i, l := range lines {
			switch {
			case hmc377R13RowHead(l) && strings.Contains(l, "["+census.EvidenceTag+"]"):
				censusAt = i
			case hmc377R13RowHead(l) && strings.Contains(l, "["+seat.EvidenceTag+"]"):
				seatAt = i
			case !hmc377R13RowHead(l) && strings.Contains(l, "["+cpu.EvidenceTag+"]"):
				lineAt = i
			}
		}
		if censusAt < 0 || seatAt < 0 || !(censusAt < lineAt && lineAt < seatAt) {
			t.Fatalf("zh=%t: the 其中 line hangs on the census account [%s], never on the wakeup_chain seat [%s]: %d %d %d\n%s",
				zh, census.EvidenceTag, seat.EvidenceTag, censusAt, lineAt, seatAt, fence)
		}
	}
}

// R-377-13 ②: the account a restriction row belongs to is its own rank
// board's — a second board that ranks the same thread (another parameter set
// or window) holds another account, and the 「其中」 line never hangs on it.
func TestHMC377R13OfWhichStaysOnItsBoard(t *testing.T) {
	const target, tid = "pool-8-thread-1-10807", 10807
	onBoard := func(n types.TraceCausalProjectionNode, fp string) types.TraceCausalProjectionNode {
		n.RankBoardTarget, n.RankBoardParamsFingerprint = target, fp
		return n
	}
	account := onBoard(hmc377R13Rank("r13-account", target, tid, tid, "runnable_wait", 2, 50.0, 50.0, 100), "f1")
	cpu := onBoard(hmc377R13Rank("r13-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 1, 30.0, 30.0, 200), "f1")
	other := onBoard(hmc377R13Rank("r13-other-board", target, tid, tid, "runnable_wait", 3, 10.0, 10.0, 300), "f2")
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WindowStartTs: 149.090584, WindowEndTs: 149.290584,
			OnChainCauses: []types.TraceCausalProjectionNode{account, cpu, other},
		}, zh)
		accountRow, cpuRow, otherRow := xlane2FindRowByEvidenceID(&model, "r13-account"), xlane2FindRowByEvidenceID(&model, "r13-cpu"), xlane2FindRowByEvidenceID(&model, "r13-other-board")
		if accountRow == nil || cpuRow == nil || otherRow == nil {
			t.Fatalf("zh=%t: every row must be drawn", zh)
		}
		lines := hmc377R13TreeLines(fence)
		accountAt, lineAt, otherAt := -1, -1, -1
		for i, l := range lines {
			switch {
			case hmc377R13RowHead(l) && strings.Contains(l, "["+accountRow.EvidenceTag+"]"):
				accountAt = i
			case hmc377R13RowHead(l) && strings.Contains(l, "["+otherRow.EvidenceTag+"]"):
				otherAt = i
			case !hmc377R13RowHead(l) && strings.Contains(l, "["+cpuRow.EvidenceTag+"]"):
				lineAt = i
			}
		}
		if accountAt < 0 || otherAt < 0 || !(accountAt < lineAt && lineAt < otherAt) {
			t.Fatalf("zh=%t: the 其中 line hangs on its own board's account [%s], before the other board's row [%s]: %d %d %d\n%s",
				zh, accountRow.EvidenceTag, otherRow.EvidenceTag, accountAt, lineAt, otherAt, fence)
		}
	}
}

// R-377-13 ② (review-12 R12P-5 / R12N-3): when the self ready account spans
// several rows, the 「其中」 line hangs on the account block after its last
// row — never on one particular row picked by order. Real window at 532dc208d
// (~/hmc-work/hmc377/fix12/review-pins/probe/n07w.out): 105 元宝 tquic_network_t-46761
// (n07w), raw ready 161.048 = 112.009 + 46.928 + 2.111 over three census rows
// with different CPU members; the line hung under the first (112.009) by
// order, and mutants M4b/U16 (last candidate) and M4c (inversion rows only)
// moved it while every test passed.
func TestHMC377R13OfWhichHangsOnTheAccountBlock(t *testing.T) {
	const target, tid = "tquic_network_t-46761", 46761
	self := func(id, typ string, rank int, impact, eff float64, line int) types.TraceCausalProjectionNode {
		node := hmc377R13Rank(id, target, tid, tid, typ, rank, impact, eff, line)
		node.Causality, node.OnChainBasis = "self_wall_clock", "self_wall_clock_interval"
		return node
	}
	edge := self("r13-edge", "runnable_wait", 2, 112.009, 112.009, 100)
	inv := self("r13-inv", "priority_inversion_runnable_wait", 14, 46.928, 0.072, 200)
	running := self("r13-run", "running", 3, 25.755, 8.367, 300)
	running.StateKind = "running"
	wall := self("r13-wall", "runnable_wait", 5, 2.111, 2.111, 400)
	dio := self("r13-dio", "d_state_or_io_wait", 8, 0.644, 0.644, 500)
	dio.StateKind = "d_sleep"
	cpu := self("r13-cpu", "cpu_affinity_or_cpuset", 13, 0.129, 0.129, 600)
	dep := hmc377R13Rank("r13-dep", "Thread-28-46781", 46781, tid, "runnable_wait", 4, 4.406, 4.406, 700)
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"Thread-28-46781", target}, WindowStartTs: 3734.575541, WindowEndTs: 3734.775541,
		OnChainCauses: []types.TraceCausalProjectionNode{dep, edge, inv, running, wall, dio, cpu},
	}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		tag := func(id string) string {
			row := xlane2FindRowByEvidenceID(&model, id)
			if row == nil || row.EvidenceTag == "" {
				t.Fatalf("%s: %s must be drawn", name, id)
			}
			return row.EvidenceTag
		}
		lines := hmc377R13TreeLines(fence)
		at := func(evidenceTag string, head bool) int {
			for i, l := range lines {
				if strings.Contains(l, "["+evidenceTag+"]") && hmc377R13RowHead(l) == head {
					return i
				}
			}
			return -1
		}
		edgeAt, invAt, wallAt, dioAt, lineAt := at(tag("r13-edge"), true), at(tag("r13-inv"), true), at(tag("r13-wall"), true), at(tag("r13-dio"), true), at(tag("r13-cpu"), false)
		if edgeAt < 0 || invAt < 0 || wallAt < 0 || dioAt < 0 || lineAt < 0 {
			t.Fatalf("%s: rows %d %d %d %d, 其中 line %d:\n%s", name, edgeAt, invAt, wallAt, dioAt, lineAt, fence)
		}
		if at(tag("r13-cpu"), true) >= 0 {
			t.Fatalf("%s: the restriction is never an additive self row:\n%s", name, fence)
		}
		if !(edgeAt < invAt && invAt < wallAt && wallAt < lineAt && lineAt < dioAt) {
			t.Fatalf("%s: the 其中 line hangs after the account block's last row [%s] (rows %d %d %d), before the next self row (%d); got line %d:\n%s",
				name, tag("r13-wall"), edgeAt, invAt, wallAt, dioAt, lineAt, fence)
		}
		for i := wallAt + 1; i < lineAt; i++ {
			if hmc377R13RowHead(lines[i]) {
				t.Fatalf("%s: another row sits between the account's last row and the 其中 line:\n%s", name, fence)
			}
		}
	}
}

// R-377-13 ③ (review-12 R12P-4): the 「其中」 line carries the restriction
// row's seat, so it wears the seat's ➊..➎ badge — the detail index's badge
// appears on exactly one tree line. Real windows at 532dc208d
// (~/hmc-work/hmc377/fix12/review-population/evidence/D3_badges.txt): y41c
// 46571 「[E6] … 根因排序: ➌ #3」 in the detail index while the tree's 「其中…
// 不另计 [E6]」 line wore no ➌ (also y54c 10299).
func TestHMC377R13OfWhichLineWearsItsBadge(t *testing.T) {
	const target, tid = "unyuan.app.chat-46571", 46571
	self := func(id, typ string, rank int, impact, eff float64, line int) types.TraceCausalProjectionNode {
		node := hmc377R13Rank(id, target, tid, tid, typ, rank, impact, eff, line)
		node.Causality, node.OnChainBasis = "self_wall_clock", "self_wall_clock_interval"
		return node
	}
	account := self("r13-account", "priority_inversion_runnable_wait", 7, 4.644, 0.8, 100)
	cpu := self("r13-cpu", "cpu_affinity_or_cpuset", 3, 3.196, 3.196, 200)
	dep := hmc377R13Rank("r13-dep", "RenderThread-46600", 46600, tid, "runnable_wait", 1, 9.0, 9.0, 300)
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"RenderThread-46600", target}, WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{dep, account, cpu},
	}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		cpuRow := xlane2FindRowByEvidenceID(&model, "r13-cpu")
		if cpuRow == nil || cpuRow.Badge != 3 {
			t.Fatalf("%s: the restriction row holds seat #3 and its badge", name)
		}
		var carriers []string
		for _, l := range hmc377R13TreeLines(fence) {
			if strings.Contains(l, "➌") {
				carriers = append(carriers, l)
			}
		}
		if len(carriers) != 1 || !strings.Contains(carriers[0], "➌ ["+cpuRow.EvidenceTag+"]") {
			t.Fatalf("%s: ➌ appears on exactly one tree line, the 其中 line, before [%s]; got %q:\n%s", name, cpuRow.EvidenceTag, carriers, fence)
		}
		if !model.Marks.has(runtimeTraceProjMarkBadge) {
			t.Fatalf("%s: the drawn badge records its mark (the mini legend teaches ➊..➎)", name)
		}
	}
	// The 其中 line is the only line wearing a badge: the mini legend still
	// teaches ➊..➎ (the mark is recorded where the glyph is drawn).
	t.Run("the only badge on the board", func(t *testing.T) {
		quiet := hmc377R13Rank("r13-dep", "RenderThread-46600", 46600, tid, "runnable_wait", 9, 9.0, 9.0, 300)
		lone := self("r13-account", "priority_inversion_runnable_wait", 8, 4.644, 0.8, 100)
		model, fence := hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WakeupPath: []string{"RenderThread-46600", target}, WindowStartTs: 10.0, WindowEndTs: 10.2,
			OnChainCauses: []types.TraceCausalProjectionNode{quiet, lone, cpu},
		}, true)
		if row := xlane2FindRowByEvidenceID(&model, "r13-cpu"); row == nil || !strings.Contains(fence, "不另计 ➌ ["+row.EvidenceTag+"]") {
			t.Fatalf("the 其中 line wears ➌:\n%s", fence)
		}
		if !strings.Contains(fence, "➊..➎该板TOP5") {
			t.Fatalf("the mini legend teaches the badge the 其中 line wears:\n%s", fence)
		}
	})
}

// R-377-13 ②: on a chain tree the analysed thread is the tree's root, drawn
// in the self stanza; a trunk thread is a chain thread even when another
// query board ranked it as its own target, so its restriction row keeps its
// seat on the trunk (the flat-board form applies only where no self stanza
// draws the analysed thread).
func TestHMC377R13TrunkThreadKeepsItsRestrictionRow(t *testing.T) {
	const target, tid, hop, hopTID = "app-100", 100, "worker-200", 200
	main := hmc377R13Rank("r13-hop", hop, hopTID, hopTID, "runnable_wait", 1, 40.0, 40.0, 100)
	cpu := hmc377R13Rank("r13-hop-cpu", hop, hopTID, hopTID, "cpu_affinity_or_cpuset", 2, 20.0, 20.0, 200)
	model, fence := hmc377R12Fence(t, types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{hop, target}, WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{main, cpu},
	}, true)
	row := xlane2FindRowByEvidenceID(&model, "r13-hop-cpu")
	if row == nil || row.EvidenceTag == "" {
		t.Fatalf("the trunk thread's restriction row must be drawn:\n%s", fence)
	}
	drawn := false
	for _, l := range hmc377R13TreeLines(fence) {
		drawn = drawn || (hmc377R13RowHead(l) && strings.Contains(l, "["+row.EvidenceTag+"]"))
	}
	if !drawn || strings.Contains(fence, "其中CPU亲和") {
		t.Fatalf("the trunk thread's restriction row keeps its own row:\n%s", fence)
	}
}
