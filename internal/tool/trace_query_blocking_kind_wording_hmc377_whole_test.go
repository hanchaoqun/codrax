package tool

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 D2 — whole-original public-entry regressions. Opt-in per capture
// (skip when the variable is unset, fail when byte size or SHA-256 differs).
// Every expected number and relation comes from the independent raw oracles
// (design/blocking-kind-wording/oracle_wake_relay.py and
// oracle_lock_spans.py: standalone Python over the raw ftrace text), never
// from this engine.

type hmc377RelayWant struct {
	tid, waker, upstream int // upstream 0 = the waker did the awaited work
	sleepMs              float64
}

// hmc377PublicBlockingRun runs the public queries an investigation makes and
// enters the real projection materialization for both languages.
func hmc377PublicBlockingRun(t *testing.T, path string, pid int, start, end float64) (tracequery.Result, map[string]string) {
	t.Helper()
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var results []types.ToolResult
	var blocking tracequery.Result
	for _, view := range []string{"wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "target_scope": "thread", "time_start": start, "time_end": end})
		results = append(results, r)
		if view == "critical_blocking_calls" {
			blocking = hmc367NativeResult(t, r, view)
		}
	}
	faces := map[string]string{}
	for _, lang := range []string{"zh", "en"} {
		bus := newBusForMutationTest()
		bus.Language = lang
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
		bus.ToolResults = results
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model_conclusion", Kind: types.BlockSection, Title: "结论", Text: "x"}}}
		if !materializeRuntimeTraceCausalProjectionBlock(doc, bus) {
			t.Fatal("projection not materialized")
		}
		encoded, _ := json.Marshal(doc.Blocks)
		var text strings.Builder
		for _, block := range doc.Blocks {
			text.WriteString(block.Text + "\n")
			for _, item := range block.Items {
				text.WriteString(item.Text + "\n" + strings.Join(item.Cells, " | ") + "\n")
			}
		}
		faces[lang] = text.String()
		faces[lang+"_json"] = string(encoded)
	}
	return blocking, faces
}

func TestHMC377WholeOriginalRecordedWaitNeverReadsAsLock(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }
	cases := []struct {
		name, env, digest string
		size              int64
		tid               int
		start, end        float64
		want              []hmc377RelayWant
	}{
		{
			name: "device-reboot cold start, one frame callback (batch 376 case B)", env: "CODRAX_HMC376_NETEASE_REBOOT_FRAME",
			digest: "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", size: 104144939,
			tid: 11077, start: 153.763595, end: 153.887340,
			want: []hmc377RelayWant{
				{tid: 11077, waker: 11098, sleepMs: 123.003}, // protected: the target's collection wait
				{tid: 11876, waker: 11077, upstream: 11098, sleepMs: 123.462},
				{tid: 11877, waker: 11876, upstream: 11098, sleepMs: 123.620},
			},
		},
		{
			name: "killed-in-background cold start", env: "CODRAX_HMC376_NETEASE_GCWAIT",
			digest: "ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", size: 106869666,
			tid: 34044, start: 5775.523119, end: 5775.790999,
			want: []hmc377RelayWant{
				{tid: 34044, waker: 34060, sleepMs: 247.304},
				{tid: 34496, waker: 34044, upstream: 34060, sleepMs: 247.433},
				{tid: 34886, waker: 34496, upstream: 34060, sleepMs: 265.260},
			},
		},
		{
			name: "container-restart cold start", env: "CODRAX_HMC376_NETEASE_CONTAINER_FRAME",
			digest: "45e304445ea5f661a21c36df65b87863e48142e26e82bfab2fc5a9d582a408f8", size: 95407103,
			tid: 42477, start: 2151.959183, end: 2152.225413,
			want: []hmc377RelayWant{
				{tid: 42477, waker: 42491, sleepMs: 247.514},
				{tid: 42831, waker: 42477, upstream: 42491, sleepMs: 247.178},
				{tid: 43208, waker: 42831, upstream: 42491, sleepMs: 264.759},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := hmc376Original(t, c.env, c.digest, c.size)
			blocking, faces := hmc377PublicBlockingRun(t, path, c.tid, c.start, c.end)
			if blocking.CriticalBlocking == nil {
				t.Fatal("no critical blocking result")
			}
			rows := map[int]tracequery.CriticalBlockingCandidate{}
			// What is asserted is about the published rows: every wait this
			// window publishes as a blocking row is a recorded collection
			// wait. Other threads may record a short contention elsewhere in
			// the window (the container capture has seven, all after the
			// target's wake); none of them is a published blocking row.
			for _, item := range blocking.CriticalBlocking.Items {
				if item.Type == "blocking_span" {
					if item.BlockingKind != "" {
						t.Fatalf("no wait this window publishes as a blocking row is a recorded lock or monitor wait, yet a row carries kind %q: %+v", item.BlockingKind, item)
					}
					rows[item.Thread.PID] = item
				}
			}
			for _, w := range c.want {
				row, ok := rows[w.tid]
				if !ok {
					t.Fatalf("recorded wait of tid %d missing", w.tid)
				}
				if row.Peer.PID != w.waker || !near(row.WaitSleepMs, w.sleepMs) {
					t.Errorf("tid %d: woken by %d with sleep %.3fms; raw oracle: woken by %d, sleep %.3fms", w.tid, row.Peer.PID, row.WaitSleepMs, w.waker, w.sleepMs)
				}
				if (w.upstream != 0) != row.WakeRelay || row.WakeRelayUpstream.PID != w.upstream {
					t.Errorf("tid %d: relay=%v upstream=%d; raw oracle: relay=%v upstream=%d", w.tid, row.WakeRelay, row.WakeRelayUpstream.PID, w.upstream != 0, w.upstream)
				}
				// Fellow waiters stay background; the target's own wait stays on the chain.
				wantChain := "background"
				if w.tid == c.tid {
					wantChain = "on_chain"
				}
				if row.ChainRelevance != wantChain {
					t.Errorf("tid %d: chain relevance %q, want %q", w.tid, row.ChainRelevance, wantChain)
				}
			}
			for lang, text := range faces {
				for _, banned := range []string{"持锁", "锁竞争", "lock-holder", "lock contention", "holds lock"} {
					if strings.Contains(text, banned) {
						t.Fatalf("%s projection names a lock (%q), though no row it publishes for this window carries a lock or monitor record", lang, banned)
					}
				}
			}
			if !strings.Contains(faces["zh"], "证据类型：阻塞等待") || !strings.Contains(faces["zh"], "唤醒由同在等待的") {
				t.Fatal("zh projection lost the recorded-wait word or the relay form")
			}
		})
	}
}

// Real lock evidence keeps its words (positive control). Douyin Lite reboot
// cold start: the main thread waits 50.246 ms (envelope) in "monitor
// contention with owner #tp-io-18 (2220)" (raw L189297–L192435); thread 11293
// is that owner (its sched_wakeup comm is #tp-io-18). CORRECTED RATIONALE
// (HMC377 D2-R3): 11293 is NOT parked on a second monitor — every payload
// names the same Runtime.loadLibrary0 monitor (L186109, L189297, L190953),
// and 11293 itself contended for it for about 24.9 ms of the main thread's
// 50.246 ms wait ([141.651862,141.675168), [141.696056,141.719079)). This pin
// guards only that a typed monitor payload keeps its lock words on the waiter
// and holder rows; whether an inferred holder that is itself contending for
// the same monitor should be named the holder is an open holder-inference
// question, not validated here. Raw oracle: oracle_lock_spans.py.
func TestHMC377WholeOriginalRealMonitorContentionKeepsLockWords(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT", "8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", 138438125)
	blocking, faces := hmc377PublicBlockingRun(t, path, 11105, 141.670, 141.730)
	var waiter *tracequery.CriticalBlockingCandidate
	for i := range blocking.CriticalBlocking.Items {
		item := &blocking.CriticalBlocking.Items[i]
		if item.Type == "blocking_span" && item.Thread.PID == 11105 && item.LineStart == 189297 {
			waiter = item
		}
		if item.WakeRelay {
			t.Fatalf("a typed contention row was re-read as a relay: %+v", item)
		}
	}
	if waiter == nil || waiter.BlockingKind != "monitor_contention" || waiter.Peer.PID != 11293 || waiter.LineEnd != 192435 {
		t.Fatalf("the raw monitor contention of the main thread is not the typed waiter row: %+v", waiter)
	}
	zh := faces["zh"]
	// RE-BASED (HMC377 integration with the thread-identity lane): the
	// holder's display name. Old "#LogReaper-11293" was the row-header name
	// (the saved command line at read-out time); raw scheduler payload runs
	// of TID 11293 are .ugc.aweme.lite (creation) → super-pool-19 →
	// #tp-io-18 141.542866–141.834587 (L174836–L208239) → … → #LogReaper only
	// from 142.018268 (L232452), after this window. The one-name-per-thread
	// window rule names it #tp-io-18 — the same name the contention payload
	// records for the owner ("owner #tp-io-18 (2220)"). Lock words unchanged.
	for _, want := range []string{"持锁阻塞(等待方 .ugc.aweme.lite-11105;持有者推断)", "锁竞争等待(持有者 #tp-io-18-11293·推断)", "证据类型：锁竞争等待", "证据类型：持锁阻塞"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("real lock evidence lost %q", want)
		}
	}
}
