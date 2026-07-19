package tool

// answer_document_projection_supp_fullcap_test.go — SUPP-ORACLE 件③ (F6 满帽
// E# pin; DISPATCH-IND 批2, 2026-07-14; 复核 R8 残口 §29.71).
//
// R8 (design §4, dispatch_ind_design_20260714): supplement records enter the
// E# evidence index and the fold families — "预期无冲突,同源编译" was the
// design-time expectation, and the SUPP-CORE review deferred the adversarial
// witness. This pin is that witness, on the FULL-CAP shape:
//
//   model observations fill the evidence cap (bus.EvidenceItems saturates
//   ObservationExtractLedgerEvidenceLimit) AND the model's trace dispatch
//   mints a population large enough to engage the producer/fold caps
//   (typed CapacityTruncated per-view row budget + E#(+N) merge folds +
//   ×N family folds) — then the supplement appends its rank/chain +
//   critical records on top. (The rendered tree is the flat 链不可上溯
//   form: 30 one-shot wakers elect no ≥2-node path — deliberate, it is the
//   h2-FAIL-family shape the supplement exists for.)
//
// Assertions (件③ triple + determinism):
//   ① E# 连续性 — the rendered evidence index is exactly E1..EN, in order,
//     no gap, no duplicate;
//   ② 锚全可达 — every E# reference on ANY rendered surface (lead/◎
//     overview, tree fence, key-metric table, detail blocks, evidence index)
//     resolves to an entry of the printed index (no dangling pointer past
//     the roster — the 修复轮 D1 「证据 E39」 shape, now under supplement
//     population growth);
//   ③ 明细表完整 — every printed index entry has its lossless detail block
//     ([E#] heading), and every [E#]-tagged key-metric table row pairs with
//     the same-numbered detail block (cross-surface ordinal consistency —
//     「E# 序数不位移」: no surface re-derives its own numbering);
//   ④ determinism — two independent compile+render passes over the same bus
//     are byte-identical (supplement records merge at a deterministic
//     position; ordinals cannot shift between passes).
//
// FULL-CHAIN engine-minted fixture (§28.7 复核纪律②): trace text →
// (&TraceQuery{}).Execute (the SAME runner model dispatch uses) →
// RunTraceQuerySystemSupplement → ObservationLedgerInputFromBusContext →
// CompileObservationLedger → CompileTraceCausalProjectionSet →
// runtimeTraceCausalProjectionCluster. No hand-built projection nodes.
//
// MUTATION self-checks:
//   - renumbering the index (e.g. skipping ids) reds ①;
//   - minting an E# tag without registering the entry (or re-deriving
//     numbers on a consumer surface) reds ②/③;
//   - a non-deterministic supplement merge order reds ④.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// suppFullCapTrace generates an engine-parseable ftrace: worker-200 (the
// user-focus target) blocks in D `cycles` times inside 3.0..3.0+cycles*20ms,
// each segment closed by a wakeup from a DIFFERENT peer thread (peer-301…)
// and carrying one of three distinct blocked_reason callers — so the
// projection population (self D families + per-waker rows + background
// rows) exceeds the per-view row budget (typed CapacityTruncated) and the
// merge/family fold machinery is genuinely engaged.
func suppFullCapTrace(cycles int) string {
	var b strings.Builder
	b.WriteString("        app-100 (100) [001] .... 2.990000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	callers := []string{
		"dma_fence_default_wait+0x1a8/0x2b8",
		"rwsem_down_write_slowpath+0x210/0x4f0",
		"jbd2_log_wait_commit+0x88/0x110",
	}
	for i := 0; i < cycles; i++ {
		base := 3.0 + float64(i)*0.020
		peerPID := 301 + i
		peer := fmt.Sprintf("peer%02d", i)
		caller := callers[i%len(callers)]
		// worker runs, then blocks in D.
		fmt.Fprintf(&b, "     worker-200 (200) [002] .... %.6f: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n", base)
		fmt.Fprintf(&b, "     worker-200 (200) [002] .... %.6f: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120\n", base+0.001)
		fmt.Fprintf(&b, "     worker-200 (200) [002] .... %.6f: sched_blocked_reason: pid=200 iowait=0 caller=%s\n", base+0.0011, caller)
		// the cycle's own peer wakes it after a distinct D duration.
		fmt.Fprintf(&b, "     %s-%d (%d) [003] .... %.6f: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=30\n", peer, peerPID, peerPID, base+0.002, peer, peerPID)
		wake := base + 0.004 + float64(i)*0.0001
		fmt.Fprintf(&b, "     %s-%d (%d) [003] .... %.6f: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002\n", peer, peerPID, peerPID, wake)
		fmt.Fprintf(&b, "     %s-%d (%d) [003] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=30 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120\n", peer, peerPID, peerPID, wake+0.0005, peer, peerPID)
		fmt.Fprintf(&b, "     worker-200 (200) [002] .... %.6f: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20\n", wake+0.001)
		fmt.Fprintf(&b, "     worker-200 (200) [002] .... %.6f: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n", base+0.015)
	}
	end := 3.0 + float64(cycles)*0.020 + 0.010
	fmt.Fprintf(&b, "        app-100 (100) [001] .... %.6f: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n", end)
	return b.String()
}

const (
	suppFullCapCycles      = 30
	suppFullCapWindowStart = 3.0
	suppFullCapWindowEnd   = 3.0 + float64(suppFullCapCycles)*0.020
)

// suppFullCapContext builds the attached-trace bus with the worker-200 user
// target AND the evidence-items lane saturated past the extract cap — the
// 「模型观测填满证据帽」 half of the full-cap shape (the ledger-input builder
// must trim to exactly ObservationExtractLedgerEvidenceLimit and the
// supplement lane must ride regardless).
func suppFullCapContext(t *testing.T) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(suppFullCapTrace(suppFullCapCycles)), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("为什么 worker 线程卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	// NOTE: filler labels deliberately avoid the E<digit> token shape — the
	// anchor scan below reads every rendered surface.
	for i := 0; i < types.ObservationExtractLedgerEvidenceLimit+12; i++ {
		ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
			ID:      fmt.Sprintf("fill-%03d", i),
			Source:  fmt.Sprintf("notes/fill_%03d.md", i),
			Summary: fmt.Sprintf("cap filler observation %03d", i),
		})
	}
	return ctx
}

func suppFullCapModelCall(t *testing.T, ctx *types.BusContext, params string) {
	t.Helper()
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("trace_query %s failed: %v", params, err)
	}
	if !res.Success {
		t.Fatalf("trace_query %s unsuccessful: %s", params, res.Summary)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
}

// suppFullCapBlocks compiles the ledger and renders the full projection
// cluster (lead + ◎ overview + tree, key-metric table, detail blocks,
// evidence index) — the SAME single-artifact builder production uses.
func suppFullCapBlocks(t *testing.T, ctx *types.BusContext) []types.AnswerBlock {
	t.Helper()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) == 0 {
		t.Fatal("no projection compiled")
	}
	return runtimeTraceCausalProjectionCluster(set.Projections[0], "zh", runtimeTraceProjUserFocus{})
}

func suppFullCapCorpus(blocks []types.AnswerBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		b.WriteString(block.Title)
		b.WriteString("\n")
		b.WriteString(block.Text)
		b.WriteString("\n")
		for _, item := range block.Items {
			b.WriteString(item.Label)
			b.WriteString("\t")
			b.WriteString(item.Text)
			for _, cell := range item.Cells {
				b.WriteString("\t")
				b.WriteString(cell)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func suppFullCapBlockBySuffix(blocks []types.AnswerBlock, suffix string) *types.AnswerBlock {
	for i := range blocks {
		if strings.HasSuffix(blocks[i].ID, suffix) {
			return &blocks[i]
		}
	}
	return nil
}

func TestSupplementFullCapEvidenceIndexIntegrity(t *testing.T) {
	ctx := suppFullCapContext(t)
	window := fmt.Sprintf(`"time_start":%.6f,"time_end":%.6f`, suppFullCapWindowStart, suppFullCapWindowEnd)
	// The model dispatch: the rank-less, critical-less family (event_search +
	// window_stats + thread_timeline) — the supplement must append BOTH
	// engine views on top of the saturated lanes.
	suppFullCapModelCall(t, ctx, `{"view":"event_search","pid":200,`+window+`}`)
	suppFullCapModelCall(t, ctx, `{"view":"window_stats","pid":200,`+window+`}`)
	suppFullCapModelCall(t, ctx, `{"view":"thread_timeline","pid":200,`+window+`}`)

	// Typed full-cap precondition ①: the evidence-items lane saturates the
	// extract cap exactly (the +12 overflow is trimmed by the input builder).
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	if len(input.EvidenceItems) != types.ObservationExtractLedgerEvidenceLimit {
		t.Fatalf("evidence lane must saturate the cap: got %d, want %d",
			len(input.EvidenceItems), types.ObservationExtractLedgerEvidenceLimit)
	}

	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || out.SkipReason != "" {
		t.Fatalf("supplement must execute on the full-cap shape: %+v", out)
	}
	if len(out.Executed) != 2 || out.Executed[0] != "root_cause_rank" || out.Executed[1] != "critical_blocking_calls" {
		t.Fatalf("full-cap shape needs [root_cause_rank critical_blocking_calls], got %v", out.Executed)
	}

	// The supplement records survive the saturated lanes (no silent drop).
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	stamped := 0
	for _, record := range ledger.Records {
		if record.SystemSupplement {
			stamped++
		}
	}
	if stamped == 0 {
		t.Fatal("supplement-minted records must survive the cap-saturated compile")
	}

	// Typed full-cap precondition ②: the model dispatch genuinely engaged the
	// producer row budget (per-view capacity truncation — the projection's own
	// typed flag, rendered as the index-header disclosure below).
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) == 0 {
		t.Fatal("no projection compiled")
	}
	if !set.Projections[0].CapacityTruncated {
		t.Fatal("full-cap shape must engage the per-view capacity truncation flag")
	}

	blocks := suppFullCapBlocks(t, ctx)
	corpus := suppFullCapCorpus(blocks)

	// ① E# 连续性 — the printed index is exactly E1..EN in order.
	evidenceBlock := suppFullCapBlockBySuffix(blocks, "_evidence")
	if evidenceBlock == nil {
		t.Fatalf("no evidence index block rendered:\n%s", corpus)
	}
	for i, item := range evidenceBlock.Items {
		if want := fmt.Sprintf("E%d", i+1); item.Label != want {
			t.Fatalf("evidence index ordinal displaced at position %d: got %q, want %q", i, item.Label, want)
		}
	}
	n := len(evidenceBlock.Items)
	// Population guard: the shape must stay adversarial (rank seats + merge
	// folds + context rows + background rows all present). A shrunken fixture
	// would hollow the pin out silently.
	if n < 12 {
		t.Fatalf("full-cap population collapsed to %d index entries — fixture no longer adversarial:\n%s", n, corpus)
	}
	if !strings.Contains(corpus, "部分查询结果超过单次返回上限") {
		t.Fatalf("capacity-truncation disclosure must render on the index header:\n%s", corpus)
	}
	if !strings.Contains(corpus, "E#(+N)") || !regexp.MustCompile(`\[E[0-9]+\(\+[0-9]+\)\]`).MatchString(corpus) {
		t.Fatalf("merge-fold population must be engaged (E#(+N) notation present):\n%s", corpus)
	}

	// ② 锚全可达 — every E<num> reference on every rendered surface resolves
	// inside the printed index (fixture guarantees no stray E<digit> token:
	// thread labels, callers and filler paths carry none).
	refRe := regexp.MustCompile(`E([0-9]+)`)
	refs := refRe.FindAllStringSubmatch(corpus, -1)
	if len(refs) == 0 {
		t.Fatal("no E# references rendered at all")
	}
	for _, m := range refs {
		num, err := strconv.Atoi(m[1])
		if err != nil || num < 1 || num > n {
			t.Fatalf("dangling E# anchor %q past the printed index (N=%d)", m[0], n)
		}
	}

	// ③ 明细表完整 + cross-surface ordinal consistency — every printed index
	// entry has its lossless detail block ([E#] heading; identical nodes share
	// one block with ids side by side), and every [E#]-tagged key-metric table
	// row pairs with the same-numbered detail heading naming the same thread.
	detailFull := suppFullCapBlockBySuffix(blocks, "_detail_full")
	if detailFull == nil {
		t.Fatalf("no lossless detail block rendered:\n%s", corpus)
	}
	for i := 1; i <= n; i++ {
		if !strings.Contains(detailFull.Text, fmt.Sprintf("[E%d]", i)) &&
			!strings.Contains(detailFull.Text, fmt.Sprintf("[E%d(", i)) {
			t.Fatalf("index entry E%d has no lossless detail block:\n%s", i, detailFull.Text)
		}
	}
	headingRe := regexp.MustCompile(`(?m)^\*\*(\[E[0-9]+[^\n]*)\*\*$`)
	headingByNum := map[int]string{}
	for _, m := range headingRe.FindAllStringSubmatch(detailFull.Text, -1) {
		for _, tag := range refRe.FindAllStringSubmatch(m[1], -1) {
			num, _ := strconv.Atoi(tag[1])
			headingByNum[num] = m[1]
		}
	}
	table := suppFullCapBlockBySuffix(blocks, "_detail")
	if table == nil || len(table.Items) == 0 {
		t.Fatalf("no key-metric table rendered:\n%s", corpus)
	}
	tagRe := regexp.MustCompile(`\[E([0-9]+)`)
	pairedRows := 0
	for _, row := range table.Items {
		if len(row.Cells) == 0 {
			continue
		}
		name := row.Cells[0]
		m := tagRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		heading, ok := headingByNum[num]
		if !ok {
			t.Fatalf("table row %q tags E%d but no detail heading carries it", name, num)
		}
		nameOnly := strings.TrimSpace(regexp.MustCompile(`\[E[0-9][^\]]*\]`).ReplaceAllString(name, ""))
		threadLabel := strings.TrimSpace(strings.SplitN(nameOnly, " / ", 2)[0])
		// CHAIN-BUDGET (2026-07-18): the wider default chain engages the
		// on-chain overflow fold row on this fixture ("其余 N 项(折叠)…"),
		// whose TABLE name carries the merge data token (N线程取最大(单项a~b))
		// after the shared display name while the detail heading carries the
		// name alone — both surfaces still share the identical name stem, so
		// the pairing assertion accepts heading-name-anchors-table-name too.
		headingName := strings.TrimSpace(regexp.MustCompile(`\[E[0-9][^\]]*\]`).ReplaceAllString(heading, ""))
		if threadLabel == "" || !(strings.Contains(heading, threadLabel) || (headingName != "" && strings.HasPrefix(threadLabel, headingName))) {
			t.Fatalf("cross-surface ordinal displacement: table row %q (E%d) vs detail heading %q", name, num, heading)
		}
		pairedRows++
	}
	if pairedRows == 0 {
		t.Fatal("no [E#]-tagged table rows found — pairing assertion hollowed out")
	}

	// ④ determinism — a second independent compile+render over the same bus
	// is byte-identical (ordinals cannot shift between passes).
	if again := suppFullCapCorpus(suppFullCapBlocks(t, ctx)); again != corpus {
		t.Fatal("two independent compile+render passes disagree — E# ordinals shifted")
	}
}

// TestSupplementH3WindowMintsIOFacetEndToEndWord — SUPP-ORACLE 件② engine arm
// (2026-07-14): the h3 golden oracle's 「完成端到端」 member word was a
// CONDITIONAL soft oracle because the io-facet seat minted only when the
// model's dispatch carried a rank/bundle-family view (§29.55.2 勘注 / §29.68
// B4). SUPP-CORE makes the rank family dispatch-independent — this pin holds
// the exact supplement arm on the exact h3 shape: donghu.ftrace, target
// .ugc.aweme.lite-17267, window 13762.791708..13763.024898, model dispatch =
// window_stats ONLY (the sweep shape the word was ABSENT under before
// SUPP-CORE) → the supplement appends rank+critical and the io-facet seat
// with the 完成端到端·IO延迟（io_latency） member word mints. Together with
// the 2026-07-14 replay witnesses (031014 2/2 PASS, word present under
// model-dispatched rank) this is the engine basis for hardening the h3
// EXPECT_CONTAINS oracle.
func TestSupplementH3WindowMintsIOFacetEndToEndWord(t *testing.T) {
	const donghu = "../../eval/fixtures/real_traces/donghu.ftrace"
	raw, err := os.ReadFile(donghu)
	if err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析 IO 等待与 IO 延迟")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, PID: 17267, Thread: ".ugc.aweme.lite",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	suppFullCapModelCall(t, ctx, `{"view":"window_stats","pid":17267,"time_start":13762.791708,"time_end":13763.024898}`)

	out := RunTraceQuerySystemSupplement(ctx)
	if !out.Attempted || out.SkipReason != "" {
		t.Fatalf("supplement must execute on the window_stats-only h3 shape: %+v", out)
	}
	if len(out.Executed) == 0 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("h3 shape needs the rank view supplemented, got %v", out.Executed)
	}

	corpus := suppFullCapCorpus(suppFullCapBlocks(t, ctx))
	// The 完成端到端 member word (typed composed form: measuring-layer word ·
	// caliber（token）) — the delivery bar the hardened h3 EXPECT_CONTAINS
	// asserts. Engine-true forms witnessed 2026-07-14 (replays 031014 r1/r2 +
	// this supplement arm): the io_latency sidebar members ride the io-facet
	// seat exactly when the critical family is in the ledger, which the
	// supplement now guarantees.
	if !strings.Contains(corpus, "完成端到端·IO延迟（io_latency）") {
		t.Fatalf("the supplement arm must mint the 完成端到端 io-facet member word:\n%s", corpus)
	}
	// The composite caliber stays disclosed with its measuring-layer word and
	// the 非墙钟 mark (DET-1: the 2.694 member election is deterministic).
	if !strings.Contains(corpus, "块设备层·块设备IO(inode)（block_io_by_inode） 2.694(综合评分,非墙钟)") {
		t.Fatalf("the composite caliber sidebar must render with the deterministic 2.694 member:\n%s", corpus)
	}
	// The wall-clock max member (trace-exact 1.347ms) stays on the seat.
	if !strings.Contains(corpus, "1.347ms") {
		t.Fatalf("the wall-clock 1.347ms member must render:\n%s", corpus)
	}
}
