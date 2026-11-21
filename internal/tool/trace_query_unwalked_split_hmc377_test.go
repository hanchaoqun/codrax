package tool

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 fix round 3 — the unwalked share has exactly one owner, end to end
// through the public trace_query entry, the delivered causal projection and
// the rendered answer (zero model calls).
//
// Synthetic capture (the dependency-credit capture plus one more wait):
// window [1000.000000, 1000.500000), analysed thread app.main-1100.
// dep.worker-4400 ends TWO waits of app.main:
//
//	S 40.000 ms [1000.400000, 1000.440000]  walked by the chain expansion
//	S  0.800 ms [1000.462200, 1000.463000]  not walked (shorter than the 1 ms
//	                                         chain branch floor)
//
// dep.worker's ready time (hand-computed from the rows):
//
//	[1000.360000, 1000.380000] 20.000 ms  before every wait  → outside
//	[1000.405000, 1000.435000] 30.000 ms  inside the walked wait → walked
//	[1000.455000, 1000.462200]  7.200 ms  before the second wait → outside
//	[1000.462200, 1000.462800]  0.600 ms  inside the unwalked wait → unwalked
//
// full 57.800 = 30.000 walked (the chain seat) + 0.600 unwalked (its own
// seat) + 27.200 outside (the adjacent remainder).
func hmc377UnwalkedSplitTrace() string {
	base := hmc377DependencyCreditTraceWith(false)
	extra := []string{
		hmc377UnwalkedRow(1000455000, 1, "cpu.hog", 3300, 3300, "sched_wakeup: comm=dep.worker pid=4400 prio=120 target_cpu=001"),
		hmc377UnwalkedRow(1000462200, 0, "app.main", 1100, 1100, "sched_switch: prev_comm=app.main prev_pid=1100 prev_prio=110 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120"),
		hmc377UnwalkedRow(1000462800, 1, "cpu.hog", 3300, 3300, "sched_switch: prev_comm=cpu.hog prev_pid=3300 prev_prio=120 prev_state=R ==> next_comm=dep.worker next_pid=4400 next_prio=120"),
		hmc377UnwalkedRow(1000463000, 1, "dep.worker", 4400, 1100, "sched_wakeup: comm=app.main pid=1100 prio=110 target_cpu=000"),
		hmc377UnwalkedRow(1000463010, 0, "swapper/0", 0, 0, "sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=1100 next_prio=110"),
		hmc377UnwalkedRow(1000464000, 1, "dep.worker", 4400, 1100, "sched_switch: prev_comm=dep.worker prev_pid=4400 prev_prio=120 prev_state=S ==> next_comm=cpu.hog next_pid=3300 next_prio=120"),
	}
	lines := strings.Split(strings.TrimRight(base, "\n"), "\n")
	var header, body []string
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			header = append(header, line)
		} else {
			body = append(body, line)
		}
	}
	body = append(body, extra...)
	tsOf := func(line string) float64 {
		m := regexp.MustCompile(`\s(\d+\.\d{6}):`).FindStringSubmatch(line)
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	// stable insertion by timestamp
	for i := 1; i < len(body); i++ {
		for j := i; j > 0 && tsOf(body[j]) < tsOf(body[j-1]); j-- {
			body[j], body[j-1] = body[j-1], body[j]
		}
	}
	return strings.Join(append(header, body...), "\n") + "\n"
}

func hmc377UnwalkedRow(us int64, cpu int, comm string, pid, tgid int, text string) string {
	return fmt.Sprintf("%16s-%-5d (%5d) [%03d] .... %d.%06d: %s", comm, pid, tgid, cpu, us/1000000, us%1000000, text)
}

type hmc377UnwalkedDelivery struct {
	rank    *tracequery.RootCauseRankResult
	zh, en  string
	results []types.ToolResult
}

func hmc377UnwalkedDeliver(t *testing.T) hmc377UnwalkedDelivery {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unwalked_split.ftrace")
	if err := os.WriteFile(path, []byte(hmc377UnwalkedSplitTrace()), 0o644); err != nil {
		t.Fatal(err)
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": 1100, "target_scope": "thread", "time_start": 1000.0, "time_end": 1000.5})
		if !r.Success {
			t.Fatalf("%s: %s", view, r.Summary)
		}
		results = append(results, r)
	}
	value := hmc367NativeResult(t, results[1], "root_cause_rank")
	deliver := func(language string) string {
		bus := newBusForMutationTest()
		bus.Language = language
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
		bus.ToolResults = results
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "probe."}}}
		res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
		if err != nil || !res.Success {
			t.Fatalf("apply (%s): %v %s", language, err, res.Summary)
		}
		return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), language)
	}
	return hmc377UnwalkedDelivery{rank: value.RootCauseRank, results: results, zh: deliver("zh"), en: deliver("en")}
}

// hmc377Joined squashes a rendered document's wrapped continuation lines.
func hmc377Joined(doc string) string {
	var b strings.Builder
	for _, line := range strings.Split(doc, "\n") {
		b.WriteString(strings.ReplaceAll(strings.TrimLeft(line, "│ \t"), " ", ""))
	}
	return b.String()
}

func TestHMC377UnwalkedShareHasOneOwnerOnTheDeliveredProjection(t *testing.T) {
	d := hmc377UnwalkedDeliver(t)
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }
	if d.rank == nil {
		t.Fatal("no native ranking")
	}
	// Engine: one seat per part.
	var chainSeat, ownSeat, remainder *tracequery.RootCauseRankItem
	for i := range d.rank.Items {
		it := &d.rank.Items[i]
		if it.Thread.PID != 4400 || it.DominantState != "runnable" {
			continue
		}
		switch {
		case strings.HasPrefix(it.Source, "wakeup_chain") && it.ChainRelevance == "on_chain":
			chainSeat = it
		case it.ChainAnchorUnwalkedSeat:
			ownSeat = it
		case it.ChainAnchorRemainderSeat && it.Type == "runnable_wait":
			remainder = it
		}
	}
	if chainSeat == nil || ownSeat == nil || remainder == nil {
		t.Fatalf("dep.worker must publish its walked chain seat, its own unwalked seat and its remainder: chain=%v own=%v rem=%v", chainSeat, ownSeat, remainder)
	}
	if !near(chainSeat.RunnableMs, 30) || !near(ownSeat.RunnableMs, 0.6) || !near(remainder.RunnableMs, 27.2) ||
		!near(remainder.ChainAnchoredMs, 30) || !near(remainder.ChainAnchorUnwalkedMs, 0.6) ||
		!near(remainder.ChainAnchorOutsideMs, 27.2) || !near(remainder.ChainAnchorFullMs, 57.8) {
		t.Fatalf("57.8 = 30 walked + 0.6 unwalked + 27.2 outside: chain %.3f own %.3f remainder %+v", chainSeat.RunnableMs, ownSeat.RunnableMs, *remainder)
	}
	if !near(ownSeat.HostWakeupEdgeAnchorTs, 1000.463) || !rootCauseRankOnChain(*ownSeat) {
		t.Fatalf("the unwalked seat is priced on the chain with the wakeup that ended its wait: %+v", *ownSeat)
	}
	// Delivered faces (zh / en): the ◇ row and the pair sentence name the
	// same three parts, and the pair restores the full account.
	zh := hmc377Joined(d.zh)
	for _, pin := range []string{
		"同源三分:全窗57.800ms=锚定30.000ms(⛓链上席)+唤醒锚定0.600ms(⛓唤醒锚定席)+本行其余27.200ms(目标此时并未在等待本线程,无链上凭证)",
		"三席合计还原全窗账57.800ms",
	} {
		if !strings.Contains(zh, pin) {
			t.Fatalf("zh delivery must carry %q:\n%s", pin, d.zh)
		}
	}
	pair := regexp.MustCompile(`同源三分对席:⛓席=\[(E\d+[^\]]*)\],唤醒锚定席=\[(E\d+[^\]]*)\],◇席=本行;三席合计还原全窗账57\.800ms`)
	m := pair.FindStringSubmatch(zh)
	if m == nil {
		t.Fatalf("the ◇ row's pair sentence names the ⛓ seat and the wakeup-anchored seat by E#:\n%s", d.zh)
	}
	back := regexp.MustCompile(`同源三分对席:◇席=\[(E\d+[^\]]*)\],唤醒锚定席=\[` + regexp.QuoteMeta(m[2]) + `\],⛓席=本行;三席合计还原全窗账57\.800ms`)
	if !back.MatchString(zh) {
		t.Fatalf("the ⛓ chain seat speaks the mirrored pair with the same three seats:\n%s", d.zh)
	}
	if strings.Contains(zh, "锚定30.600ms") || strings.Contains(zh, "其余27.800ms") {
		t.Fatalf("no face may fold the unwalked part into the walked share or the remainder:\n%s", d.zh)
	}
	en := hmc377Joined(d.en)
	if !strings.Contains(en, "same-sourcethree-waysplit:whole-window57.800ms=30.000msanchored(⛓chainseat)+0.600mswakeup-anchored(⛓wakeup-anchoredseat)+thisremainder27.200ms") ||
		!strings.Contains(en, "thethreeseats'sumrestoresthewhole-windowaccount57.800ms") {
		t.Fatalf("en delivery must carry the same three parts:\n%s", d.en)
	}
}

func rootCauseRankOnChain(item tracequery.RootCauseRankItem) bool {
	return strings.TrimSpace(item.ChainRelevance) == "on_chain"
}

// hmc377DeliverWindow runs the public entry on one captured window and
// returns the delivered documents (zh / en: the report language of the
// mutation itself).
func hmc377DeliverWindow(t *testing.T, path string, tid int, start, end float64) hmc377UnwalkedDelivery {
	t.Helper()
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": tid, "target_scope": "thread", "time_start": start, "time_end": end})
		if !r.Success {
			t.Fatalf("%s: %s", view, r.Summary)
		}
		results = append(results, r)
	}
	value := hmc367NativeResult(t, results[1], "root_cause_rank")
	deliver := func(language string) string {
		bus := newBusForMutationTest()
		bus.Language = language
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
		bus.ToolResults = results
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "probe."}}}
		res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
		if err != nil || !res.Success {
			t.Fatalf("apply (%s): %v %s", language, err, res.Summary)
		}
		return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), language)
	}
	return hmc377UnwalkedDelivery{rank: value.RootCauseRank, results: results, zh: deliver("zh"), en: deliver("en")}
}

// hmc377SplitSentences parses every split sentence of a delivered zh document
// (wrapped lines joined) and checks that its named parts add up to the full
// account it states; it returns the parts per full account.
func hmc377SplitSentences(t *testing.T, doc string) map[string][][]float64 {
	t.Helper()
	joined := hmc377Joined(doc)
	num := `([0-9]+\.[0-9]{3})`
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`同源二分:全窗` + num + `ms=本行锚定` + num + `ms\+其余` + num + `ms`),
		regexp.MustCompile(`同源二分:全窗` + num + `ms=锚定` + num + `ms\([^)]*\)\+本行其余` + num + `ms`),
		regexp.MustCompile(`同源三分:全窗` + num + `ms=本行锚定` + num + `ms\+唤醒锚定` + num + `ms\([^)]*\)\+其余` + num + `ms`),
		regexp.MustCompile(`同源三分:全窗` + num + `ms=锚定` + num + `ms\([^)]*\)\+唤醒锚定` + num + `ms\([^)]*\)\+本行其余` + num + `ms`),
	}
	out := map[string][][]float64{}
	for _, re := range patterns {
		for _, m := range re.FindAllStringSubmatch(joined, -1) {
			var parts []float64
			sum := 0.0
			for _, raw := range m[2:] {
				v, _ := strconv.ParseFloat(raw, 64)
				parts = append(parts, v)
				sum += v
			}
			full, _ := strconv.ParseFloat(m[1], 64)
			if math.Abs(sum-full) > 0.0015 {
				t.Errorf("split sentence parts %v do not add up to %.3f: %s", parts, full, m[0])
			}
			out[m[1]] = append(out[m[1]], parts)
		}
	}
	return out
}

// HMC377 fix round 3, on the complete original captures (opt-in): every
// delivered split sentence's parts add up to the full account, the ⛓ chain
// row and the ◇ remainder row of one thread state the same parts, and the
// three-way pairs that carried 「合计还原全窗账」 before round 2 carry it again
// (Honeybird reboot TID 10566: Executor$Worker-10894 206.862 = 29.904 walked +
// 0.012 unwalked + 176.946; Douyin reboot5 TID 11105: #tp-io-29-11317 94.225 =
// 1.589 + 0.042 + 92.594 — the raw-oracle parts). Item 5: every one of the
// analysed thread's own D seats is published, so its merged family line on
// Douyin reboot5 is the complete 15-occurrence 24.612 ms account.
func TestHMC377WholeOriginalDeliveredSplitsNameEveryPartOnce(t *testing.T) {
	cases := []struct {
		name, variable, digest string
		size                   int64
		tid                    int
		start, end             float64
		threeWay               []string
	}{
		{"honeybird reboot", "CODRAX_HMC377_HONEYBIRD_REBOOT", "54405241db707b901057b41a5c94e69a3fc233425fdf7a1f745e43585cf248d4", 124269104,
			10566, 162.372184, 163.616645, []string{"206.862ms=本行锚定29.904ms+唤醒锚定0.012ms", "206.862ms=锚定29.904ms(⛓链上席)+唤醒锚定0.012ms"}},
		{"douyin reboot5", "CODRAX_HMC377_DOUYIN_REBOOT", "8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", 138438125,
			11105, 140.911304, 141.835458, []string{"94.225ms=本行锚定1.589ms+唤醒锚定0.042ms", "94.225ms=锚定1.589ms(⛓链上席)+唤醒锚定0.042ms"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := hmc376Original(t, c.variable, c.digest, c.size)
			d := hmc377DeliverWindow(t, path, c.tid, c.start, c.end)
			byFull := hmc377SplitSentences(t, d.zh)
			for full, variants := range byFull {
				for _, v := range variants[1:] {
					if len(v) != len(variants[0]) {
						t.Errorf("full %s: rows state the split with different parts %v", full, variants)
						continue
					}
					for i := range v {
						if math.Abs(v[i]-variants[0][i]) > 0.0005 {
							t.Errorf("full %s: the ⛓ and ◇ rows state different parts %v", full, variants)
						}
					}
				}
			}
			zh := hmc377Joined(d.zh)
			for _, pin := range c.threeWay {
				if !strings.Contains(zh, pin) {
					t.Errorf("the three-way split %q is missing:\n%s", pin, d.zh)
				}
			}
			// HMC377 fix round 4: both rows of the three-way pair restore the
			// full account — with the three seats when the wakeup-anchored
			// seat is on this board and publishes the account's part, with
			// the three typed parts when the lookup does not prove it drawn
			// (its value cannot be checked against the part there; round 7,
			// R-377-8: that seat's word is the see-detail form).
			pairs := regexp.MustCompile(`同源三分对席:[^;]*;[^(]*合计还原全窗账`).FindAllString(zh, -1)
			if len(pairs) < 2 {
				t.Errorf("both rows of the three-way pair restore the full account, found %d", len(pairs))
			}
			for _, pair := range pairs {
				if strings.Contains(pair, "唤醒锚定席见明细") && strings.Contains(pair, "三席合计还原") {
					t.Errorf("an off-board wakeup-anchored seat's value is unknown here — no three-seat sum claim: %s", pair)
				}
			}
			en := hmc377Joined(d.en)
			if n := strings.Count(en, "thethreeseats'sumrestoresthewhole-windowaccount") + strings.Count(en, "therest)restorethewhole-windowaccount"); n < 2 {
				t.Errorf("en: both rows of the three-way pair restore the full account, found %d", n)
			}
			// Every pair sentence names E#s that are on the page.
			tags := map[string]bool{}
			for _, m := range regexp.MustCompile(`\[(E\d+)`).FindAllStringSubmatch(zh, -1) {
				tags[m[1]] = true
			}
			for _, m := range regexp.MustCompile(`同源[二三]分对席:([^;]*);`).FindAllStringSubmatch(zh, -1) {
				for _, ref := range regexp.MustCompile(`\[(E\d+)`).FindAllStringSubmatch(m[1], -1) {
					if !tags[ref[1]] {
						t.Errorf("a pair sentence names %s, which is not on the page: %s", ref[1], m[0])
					}
				}
			}
			if c.tid == 11105 && !strings.Contains(zh, "IO阻塞·不可中断(原因未证)15次") {
				t.Errorf("the analysed thread's own D family line must count every published occurrence (15):\n%s", d.zh)
			}
		})
	}
}
