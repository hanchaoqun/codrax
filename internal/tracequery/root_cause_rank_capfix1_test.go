package tracequery

// root_cause_rank_capfix1_test.go — CAPFIX-1 pins (§29.150① user ruling
// verbatim 2026-07-19: 「一定要确保 优先保证 折算后提升最大的 链上的 TOP 内容
// 不丢失,非链上的 和背景 次之,尽量减少噪音影响。请按最优方案推荐实施」;
// R-19-a → 链上TOP恒先 + 值披露).
//
// 件1 cross-lane cap survival: an on-chain valued candidate never dies to the
// cap while any non-chain (◇/▒) candidate holds a slot; ◇ adjacent beats ▒
// background; within one lane the published eff decides (desc); survivors are
// emitted in ORIGINAL board order — the cap selects who appears, never
// reorders (榜序数零动).
// 件2 residual cap-death value+subject disclosure (R-2 披露道 extension) with
// honest per-lane counts, zh/EN word-face single point.
// 件3 zero-value/disclosure rows never hold a candidate seat (恒末 structural
// pin).

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func capfix1Row(relevance, comm string, pid int, eff float64) RootCauseRankItem {
	return RootCauseRankItem{
		Type: "runnable_wait", Thread: ThreadRef{Comm: comm, PID: pid},
		ChainRelevance: relevance,
		ImpactMs:       eff, RunnableMs: eff, EffectiveImpactMs: eff, CumulativeImpactMs: eff,
	}
}

// capfix1AssertCrossLaneInvariant sweeps one truncation outcome for the 件1
// invariant: no cap-dead candidate may outrank (strictly, on the cross-lane
// lexicographic key) any surviving candidate — in particular an on-chain
// valued seat never dies while a non-chain/background candidate survives.
func capfix1AssertCrossLaneInvariant(t *testing.T, survivors, capDead []RootCauseRankItem) {
	t.Helper()
	for _, dead := range capDead {
		for _, alive := range survivors {
			dp, ap := capSurvivalLanePriority(dead), capSurvivalLanePriority(alive)
			if dp < ap {
				t.Fatalf("跨道优先反转: a %s-lane row (eff=%.3f) died at the cap while a %s-lane row (eff=%.3f) holds a slot",
					[]string{"chain", "adjacent", "background"}[dp], rootCauseEffectiveImpactMs(dead),
					[]string{"chain", "adjacent", "background"}[ap], rootCauseEffectiveImpactMs(alive))
			}
			if dp == ap && rootCauseEffectiveImpactMs(dead) > rootCauseEffectiveImpactMs(alive) {
				t.Fatalf("同道值序反转: eff=%.3f died while smaller eff=%.3f survives (lane=%d)",
					rootCauseEffectiveImpactMs(dead), rootCauseEffectiveImpactMs(alive), dp)
			}
		}
	}
}

// TestCapfix1CrossLaneSurvivalPriorityInvariant is the 件1 core pin: the
// candidate pool arrives in an ADVERSARIAL order (value-desc across lanes —
// the shape a channel-blind sort would deliver, the ELIM-SELF-FIX degenerate-
// window death shape), and the cap must still keep every on-chain valued seat
// alive before any ◇/▒ row: 帽存活选择键不再继承榜排序. Mutation arm: invert
// capSurvivalLanePriority (background first) — an on-chain seat dies to a
// background seat and this pin reds (链上席被背景席挤=红).
func TestCapfix1CrossLaneSurvivalPriorityInvariant(t *testing.T) {
	items := []RootCauseRankItem{
		capfix1Row("background", "bg-big", 900, 10.0),
		capfix1Row("background", "bg-mid", 901, 9.0),
		capfix1Row("adjacent", "adj", 800, 5.0),
		capfix1Row("on_chain", "chain-a", 100, 0.9),
		capfix1Row("on_chain", "chain-b", 101, 0.5),
	}
	survivors, capDead := selectRootCauseRankCapSurvivors(items, 3)
	if len(survivors) != 3 || len(capDead) != 2 {
		t.Fatalf("census drifted: survivors=%d capDead=%d", len(survivors), len(capDead))
	}
	capfix1AssertCrossLaneInvariant(t, survivors, capDead)
	// Both on-chain seats survive; the adjacent seat takes the third slot;
	// both background rows die despite carrying the LARGEST raw values.
	wantOrder := []string{"adj", "chain-a", "chain-b"}
	for i, item := range survivors {
		if item.Thread.Comm != wantOrder[i] {
			t.Fatalf("survivor set/order drifted (cap selects, never reorders — original board order kept): got[%d]=%s want=%s survivors=%+v",
				i, item.Thread.Comm, wantOrder[i], survivors)
		}
	}
	for _, item := range capDead {
		if !strings.HasPrefix(item.Thread.Comm, "bg-") {
			t.Fatalf("only background rows may die here: %+v", capDead)
		}
	}
}

// TestCapfix1AdjacentBeatsBackground pins the ◇>▒ half of the lexicographic
// key on its own: one slot, a huge background row against a tiny adjacent
// row — the adjacent row survives (非链上次之,背景再次之).
func TestCapfix1AdjacentBeatsBackground(t *testing.T) {
	items := []RootCauseRankItem{
		capfix1Row("background", "bg", 900, 9.0),
		capfix1Row("adjacent", "adj", 800, 0.1),
	}
	survivors, capDead := selectRootCauseRankCapSurvivors(items, 1)
	if len(survivors) != 1 || survivors[0].Thread.Comm != "adj" {
		t.Fatalf("◇ must outlive ▒ at the cap regardless of value: %+v", survivors)
	}
	capfix1AssertCrossLaneInvariant(t, survivors, capDead)
}

// TestCapfix1WithinLaneEffDescOrderPreserved pins the within-lane half:
// same-lane survival is by published eff desc, and the survivors keep their
// ORIGINAL relative board order (mutation arms: eff asc = red; emitting in
// survival-key order = red).
func TestCapfix1WithinLaneEffDescOrderPreserved(t *testing.T) {
	items := []RootCauseRankItem{
		capfix1Row("on_chain", "small-first", 100, 1.0),
		capfix1Row("on_chain", "big", 101, 5.0),
		capfix1Row("on_chain", "mid", 102, 3.0),
	}
	survivors, capDead := selectRootCauseRankCapSurvivors(items, 2)
	if len(survivors) != 2 || survivors[0].Thread.Comm != "big" || survivors[1].Thread.Comm != "mid" {
		t.Fatalf("within-lane survival must be eff desc with board order preserved: %+v", survivors)
	}
	if len(capDead) != 1 || capDead[0].Thread.Comm != "small-first" {
		t.Fatalf("the smallest same-lane row dies: %+v", capDead)
	}
}

// TestCapfix1ProductionSortPrefixEquivalence pins the 排序语义零动 claim from
// the other side: on a STAMPED board in production sort order
// (sortRootCauseRankItems chainAware — relevance-first, then eff desc), the
// cross-lane survivor set is EXACTLY the old strict prefix, so every
// §29.136/§29.139 published board stays byte-identical (the witness replay's
// per-seat fate table before/after is unchanged; only the 件2 disclosure
// clause is new).
func TestCapfix1ProductionSortPrefixEquivalence(t *testing.T) {
	items := []RootCauseRankItem{
		capfix1Row("background", "bg-a", 900, 30.0),
		capfix1Row("on_chain", "chain-a", 100, 4.0),
		capfix1Row("adjacent", "adj-a", 800, 6.0),
		capfix1Row("on_chain", "chain-b", 101, 2.0),
		capfix1Row("background", "bg-b", 901, 1.5),
		capfix1Row("adjacent", "adj-b", 801, 0.4),
		capfix1Row("on_chain", "chain-c", 102, 0.2),
	}
	sortRootCauseRankItems(items, true)
	for limit := 1; limit < len(items); limit++ {
		survivors, capDead := selectRootCauseRankCapSurvivors(items, limit)
		if len(survivors) != limit {
			t.Fatalf("limit=%d: %d survivors", limit, len(survivors))
		}
		for i := range survivors {
			if survivors[i].Thread.Comm != items[i].Thread.Comm {
				t.Fatalf("limit=%d: survivor set must equal the production-sort prefix (帽只决定谁出现): got[%d]=%s want=%s",
					limit, i, survivors[i].Thread.Comm, items[i].Thread.Comm)
			}
		}
		capfix1AssertCrossLaneInvariant(t, survivors, capDead)
	}
}

// TestCapfix1ZeroValueRowsNeverHoldCandidateSeats is the 件3 pin (零值恒末,
// §29.136 修根后现状钉死): zero-value / disclosure rows never enter the
// candidate pool — they cannot occupy a board slot while ANY valued candidate
// dies at the cap, they publish AFTER every candidate row, and they never
// appear in the capDead value disclosure.
func TestCapfix1ZeroValueRowsNeverHoldCandidateSeats(t *testing.T) {
	items := []RootCauseRankItem{
		// Zero-value rows arrive FIRST in board order. The sleep_wait row is a
		// NON-target context row (the RenderThread-59891 shape): its ONLY route
		// to the side lane is the precise eff<=0 arm — the load-bearing gate
		// this pin's mutation arm (eff<=0 → false) escapes through nothing.
		{Type: "trace_gap", Thread: ThreadRef{Comm: "idle-a", PID: 200}},
		{Type: "sleep_wait", Thread: ThreadRef{Comm: "render", PID: 300}, ChainRelevance: "background", CumulativeImpactMs: 50},
		capfix1Row("on_chain", "chain-a", 101, 5.0),
		capfix1Row("on_chain", "chain-b", 102, 4.0),
		capfix1Row("background", "bg-a", 900, 3.0),
	}
	out, capDead, candidateTotal, candidateEmitted, _, _ := truncateRootCauseRankCandidatesAndSideRows(items, 2)
	if candidateTotal != 3 || candidateEmitted != 2 {
		t.Fatalf("zero rows must not count as candidates: %d/%d", candidateEmitted, candidateTotal)
	}
	for i := 0; i < candidateEmitted; i++ {
		if rootCauseEffectiveImpactMs(out[i]) <= 0 {
			t.Fatalf("零值行占位: a zero-value row holds candidate slot %d: %+v", i, out[i])
		}
	}
	for _, item := range out[candidateEmitted:] {
		if rootCauseEffectiveImpactMs(item) > 0 && !rootCauseRankItemIsZeroSeatDisclosure(item) {
			t.Fatalf("valued candidate leaked into the side region: %+v", item)
		}
	}
	if len(capDead) != 1 || capDead[0].Thread.Comm != "bg-a" {
		t.Fatalf("the background candidate dies (never a zero row): %+v", capDead)
	}
	capfix1AssertCrossLaneInvariant(t, out[:candidateEmitted], capDead)
}

// TestCapfix1CapDeathValueClauseWordFace pins the 件2 disclosure word face
// byte-exactly on both language halves from the single point (per-lane counts
// honest; headline = overall largest by published eff; the largest ON-CHAIN
// casualty is additionally named when it is not the headline — the ruling's
// 链上 TOP 内容 never loses its value disclosure). Mutation arm: swapping the
// zh lane words (链上↔背景) reds this pin.
func TestCapfix1CapDeathValueClauseWordFace(t *testing.T) {
	if clause := rootCauseRankCapDeathValueClause(nil); clause != "" {
		t.Fatalf("no cap deaths → no clause, got %q", clause)
	}
	capDead := []RootCauseRankItem{
		func() RootCauseRankItem {
			row := capfix1Row("on_chain", "RenderThread", 59891, 0.378)
			row.Type = "running"
			return row
		}(),
		func() RootCauseRankItem {
			row := capfix1Row("background", "hilogd.pst", 474, 30.724)
			row.Type = "low_frequency"
			return row
		}(),
		capfix1Row("adjacent", "ThreadPoolForeg", 60555, 0.171),
	}
	got := rootCauseRankCapDeathValueClause(capDead)
	want := "; 3 valued candidate row(s) did not enter the published board (on_chain=1/adjacent=1/background=1), largest low_frequency hilogd.pst-474 30.724ms (background), largest on_chain running RenderThread-59891 0.378ms (另有 3 行未入发布面(链上 1/邻近 1/背景 1),最大 hilogd.pst-474 30.724ms(背景);链上最大 RenderThread-59891 0.378ms)"
	if got != want {
		t.Fatalf("件2 word face drifted:\n got %q\nwant %q", got, want)
	}
	// When the on-chain casualty IS the headline, it is not named twice.
	chainOnly := []RootCauseRankItem{capfix1Row("on_chain", "keva-3", 17439, 1.354)}
	got = rootCauseRankCapDeathValueClause(chainOnly)
	want = "; 1 valued candidate row(s) did not enter the published board (on_chain=1/adjacent=0/background=0), largest runnable_wait keva-3-17439 1.354ms (on_chain) (另有 1 行未入发布面(链上 1/邻近 0/背景 0),最大 keva-3-17439 1.354ms(链上))"
	if got != want {
		t.Fatalf("件2 chain-headline face drifted:\n got %q\nwant %q", got, want)
	}
	// Subject fail-open: a row with no thread identity speaks its typed
	// metric token, never "unknown-thread" (§7.30 discipline — today's
	// aggregate-metric types are structurally eff=0 and side-laned, so this
	// arm is defensive for any future thread-less valued candidate).
	if got := rootCauseRankCapDeathSubject(RootCauseRankItem{Type: "cpu_pressure"}); got != "cpu_pressure" {
		t.Fatalf("thread-less subject must be the metric token, got %q", got)
	}
	if got := rootCauseRankCapDeathSubject(capfix1Row("background", "worker", 42, 1.0)); got != "worker-42" {
		t.Fatalf("threaded subject must be the thread label, got %q", got)
	}
}

// TestCapfix1RealTraceCapDeathRosterDisclosure replays the §29.136/§29.139
// cap-death rosters through two of the four real-trace witness boards
// (production BuildRootCauseRank render) and pins the 件2 disclosure verbatim
// against them:
//   - tieba flag board: OS_FFRT_2_146-61802 runnable 19.984 (post-relaning a
//     ▒ background row) is the LARGEST residual cap death → headline names it
//     with value+subject (the §29.150① 验收 row: 存活或带值 caveat — it
//     carries the value caveat), and NO on-chain candidate dies there;
//   - donghu 17267 board: keva-3-17439 io_wait 1.354 is the largest ON-CHAIN
//     residual cap death (all 12 slots hold higher-eff on-chain seats — the
//     §29.136 legal-cap-death form) → the chain supplement names it.
//
// The cross-lane invariant is swept over every published/pool pair on both
// boards (链上席永不死于非链上占位).
func TestCapfix1RealTraceCapDeathRosterDisclosure(t *testing.T) {
	requireRealTrace := func(path string) *Index {
		t.Helper()
		if _, err := os.Stat(path); err != nil {
			t.Skipf("real-trace fixture absent: %v", err)
		}
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		return idx
	}
	assertBoard := func(rank RootCauseRankResult, wantCaveatBits []string) {
		t.Helper()
		var published, pool []RootCauseRankItem
		published = append(published, rank.Items...)
		pool = append(pool, rank.preTruncationItems...)
		// Sweep: no on-chain valued candidate is pool-only while a published
		// candidate on a lower-priority lane holds a slot with LOWER priority
		// class (the 件1 invariant on the production render).
		publishedKeys := map[string]bool{}
		for _, item := range published {
			publishedKeys[fmt.Sprintf("%s|%s|%d", item.Type, threadKey(item.Thread), item.LineStart)] = true
		}
		worstPublished := -1
		for _, item := range published {
			if rootCauseRankItemIsZeroSeatDisclosure(item) || item.ChainAnchorRemainderSeat ||
				item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
				item.GatedShareConstituentSeat {
				continue
			}
			if p := capSurvivalLanePriority(item); p > worstPublished {
				worstPublished = p
			}
		}
		for _, item := range pool {
			if rootCauseRankItemIsZeroSeatDisclosure(item) || item.ChainAnchorRemainderSeat ||
				item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
				item.GatedShareConstituentSeat {
				continue
			}
			key := fmt.Sprintf("%s|%s|%d", item.Type, threadKey(item.Thread), item.LineStart)
			if publishedKeys[key] {
				continue
			}
			if capSurvivalLanePriority(item) < worstPublished {
				t.Fatalf("件1 invariant broken on the production render: %s-lane pool row %s %s died while a lower-priority lane holds a candidate slot",
					[]string{"chain", "adjacent", "background"}[capSurvivalLanePriority(item)], item.Type, threadLabel(item.Thread))
			}
		}
		joined := strings.Join(rank.Caveats, "\n")
		for _, bit := range wantCaveatBits {
			if !strings.Contains(joined, bit) {
				t.Fatalf("件2 disclosure bit missing from the board caveats:\nwant substring %q\nin:\n%s", bit, joined)
			}
		}
	}

	tieba := requireRealTrace("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	flagBoard := BuildRootCauseRank(tieba, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	assertBoard(flagBoard, []string{
		"largest runnable_wait OS_FFRT_2_146-61802 19.984ms (background)",
		"最大 OS_FFRT_2_146-61802 19.984ms(背景)",
	})

	donghu := requireRealTrace("../../eval/fixtures/real_traces/donghu.ftrace")
	board17267 := BuildRootCauseRank(donghu, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	assertBoard(board17267, []string{
		"largest on_chain io_wait keva-3-17439 1.354ms",
		"链上最大 keva-3-17439 1.354ms",
	})
}
