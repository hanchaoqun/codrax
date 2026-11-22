package tool

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 105 元宝 应用后台销毁后冷启动 #2 (ftrace-20261019_19.16.06), the analysed
// main thread 58533 in its bindApplication window [10834.099821,
// 10834.620005) (HMC377 round 10, R9P-1). The thread's D account is 4.256 ms
// in nine proven-cause partition seats (vfs_path_open_ng 3.435 ms, …) and its
// iowait 0.375 ms; raw D+IO 4.631 ms (target_window_states; independent raw
// parse fix9/review-holders/out/rawall_ybb2.json 4.248 + 0.383). The top-8 D
// display list is eight (thread, cpu) groups held by six other threads, each
// larger than the thread's largest D group (3.800 ms on CPU 7), so the thread
// is absent from it (round 11 R10T-3 count correction:
// fix11/dcause/logs/ybb2_engine_dgroups.out). Its own chain walked its
// 1.666 ms vfs_path_open_ng D wait;
// before round 10 that RootEvidence row minted a second D seat (rank 10,
// source wakeup_chain) for instants the 3.435 ms partition seat holds, and
// the delivered ◎ and tree showed 5.922 ms
// (fix9/review-holders/out/wt-head_ybb2.fence_zh.txt). Each cause of the
// thread's D/IO is now carried by one published account on the board, the
// tree and ◎; the walked wait stays on the wakeup_chain view.
const (
	hmc377YuanbaoBgKill2Digest = "31dad28dcc9ce89c775467b47efd22bd6cdd1a1c648a28f6c56c06170ca04e09"
	hmc377YuanbaoBgKill2Size   = 56918611
)

func TestHMC377WholeOriginalYuanbaoBgKillTargetDIOHasOneAccountPerCause(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_YUANBAO_BGKILL2", hmc377YuanbaoBgKill2Digest, hmc377YuanbaoBgKill2Size)
	const tid = 58533
	d := hmc377DeliverWindow(t, path, tid, 10834.099821, 10834.620005)
	if d.rank == nil {
		t.Fatal("no native ranking")
	}
	chain := hmc367NativeResult(t, d.results[0], "wakeup_chain")
	states := chain.TargetWindowStates
	if states == nil || states.DStateMs <= 0 {
		t.Fatalf("the analysed thread's window state account must be published: %+v", states)
	}
	raw := states.DStateMs + states.IOWaitMs
	if math.Abs(states.DStateMs-4.256) > 0.0015 || math.Abs(states.IOWaitMs-0.375) > 0.0015 {
		t.Fatalf("raw D/IO account moved: d=%.3f io=%.3f (want 4.256 / 0.375)", states.DStateMs, states.IOWaitMs)
	}
	// The walked wait stays lossless on the wakeup_chain view.
	walked := false
	if chain.WakeupChain != nil {
		for _, root := range chain.WakeupChain.RootEvidence {
			if root.Thread.PID == tid && root.Type == "d_state_or_io_wait" && root.LineStart == 124026 && math.Abs(root.DurationMs-1.666) < 0.0015 {
				walked = true
			}
		}
	}
	if !walked {
		t.Fatal("the walked 1.666 ms D wait (line 124026) must stay on the wakeup_chain view")
	}

	// Board: one account per cause.
	dSum, dioSum, dSeats := 0.0, 0.0, 0
	causes := map[string]int{}
	for _, row := range d.rank.Items {
		if row.Thread.PID != tid || (row.Type != "d_state_or_io_wait" && row.Type != "io_wait") {
			continue
		}
		if strings.HasPrefix(row.Source, "wakeup_chain") {
			t.Fatalf("the walked D wait minted a second D seat beside the partition seats:\n%s", hmc376Board(d.rank))
		}
		dSum += row.DStateMs
		dioSum += row.DStateMs + row.IOWaitMs
		if row.DStateMs > 0 {
			dSeats++
			causes[row.BlockedReasonCaller]++
		}
	}
	for _, row := range d.rank.AbsorbedItems {
		if row.Thread.PID == tid && strings.HasPrefix(row.Source, "wakeup_chain") && (row.Type == "d_state_or_io_wait" || row.Type == "io_wait") {
			t.Fatalf("the walked D wait is not a rank row: %s", hmc376RowLabel(row))
		}
	}
	if dioSum > raw+0.001 {
		t.Fatalf("the analysed thread's D/IO seats sum to %.3f ms, raw D+IO %.3f ms:\n%s", dioSum, raw, hmc376Board(d.rank))
	}
	if math.Abs(dSum-states.DStateMs) > 0.0015 || dSeats != 9 {
		t.Fatalf("the nine partition seats keep the whole %.3f ms D account, got %d seats Σ%.3f:\n%s", states.DStateMs, dSeats, dSum, hmc376Board(d.rank))
	}
	for cause, n := range causes {
		if n != 1 {
			t.Fatalf("cause %q carried by %d D seats", cause, n)
		}
	}

	// Faces: the tree's D/IO account rows of the thread and its ◎ members.
	num := `([0-9]+\.[0-9]{3})`
	treeAccount := regexp.MustCompile(`自身·(?:D-state/iowait|D-state|iowait)(?:\(原因未证\))? (?:合计)?` + num + `ms \[E`)
	elimMember := regexp.MustCompile(num + `ms [█░]+ unyuan\.app\.chat-58533 · (?:IO阻塞|不可中断|D状态)`)
	tree := hmc377R9P1FenceBody(d.zh, "trace-causal-projection")
	elim := hmc377R9P1FenceBody(d.zh, "trace-elim-overview")
	treeSum, elimSum := 0.0, 0.0
	treeRows := 0
	for _, m := range treeAccount.FindAllStringSubmatch(tree, -1) {
		v, _ := strconv.ParseFloat(m[1], 64)
		treeSum += v
		treeRows++
	}
	for _, m := range elimMember.FindAllStringSubmatch(elim, -1) {
		v, _ := strconv.ParseFloat(m[1], 64)
		elimSum += v
	}
	if treeRows == 0 {
		t.Fatalf("the tree must draw the analysed thread's D/IO account rows:\n%s", tree)
	}
	if treeSum > raw+0.001 || elimSum > raw+0.001 {
		t.Fatalf("delivered D/IO rows of the analysed thread exceed raw D+IO %.3f ms: tree Σ%.3f ◎ Σ%.3f", raw, treeSum, elimSum)
	}
}

// hmc377R9P1FenceBody returns the body of the first ```text <name> fence of a rendered
// document.
func hmc377R9P1FenceBody(doc, name string) string {
	start := strings.Index(doc, "```text "+name+"\n")
	if start < 0 {
		return ""
	}
	body := doc[start+len("```text "+name+"\n"):]
	if end := strings.Index(body, "\n```"); end >= 0 {
		body = body[:end]
	}
	return body
}
