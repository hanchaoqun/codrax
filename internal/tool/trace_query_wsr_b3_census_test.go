package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WSR §8 b3 renderer pins (real_trace_campaign_20260705 §8.1). Same
// donghu-shaped fixture as the engine pins in
// internal/tracequery/process_domain_census_test.go: tgid 59566 has 39
// window-observed threads, NetworkService-60595 runs a fragmented 13.135ms
// across cpu0/1/2 and never survives the global top-8 roster, and exactly
// four tieba threads survive into the legacy process_cpu_load rollup
// (the "threads=4" masquerade the census row corrects).
func wsrB3CensusTraceText() string {
	var b strings.Builder
	b.WriteString(`          <idle>-0 (-----) [000] .... 10.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog1 next_pid=70001 next_prio=20
          <idle>-0 (-----) [001] .... 10.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog3 next_pid=70003 next_prio=20
          <idle>-0 (-----) [002] .... 10.000000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog5 next_pid=70005 next_prio=20
          <idle>-0 (-----) [003] .... 10.000000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hog7 next_pid=70007 next_prio=20
        hog5-70005 (70005) [002] .... 10.071000: sched_switch: prev_comm=hog5 prev_pid=70005 prev_prio=20 prev_state=S ==> next_comm=hog6 next_pid=70006 next_prio=20
        hog3-70003 (70003) [001] .... 10.076000: sched_switch: prev_comm=hog3 prev_pid=70003 prev_prio=20 prev_state=S ==> next_comm=hog4 next_pid=70004 next_prio=20
        hog1-70001 (70001) [000] .... 10.080000: sched_switch: prev_comm=hog1 prev_pid=70001 prev_prio=20 prev_state=S ==> next_comm=hog2 next_pid=70002 next_prio=20
        hog7-70007 (70007) [003] .... 10.140000: sched_switch: prev_comm=hog7 prev_pid=70007 prev_prio=20 prev_state=S ==> next_comm=tb7 next_pid=59573 next_prio=20
        hog2-70002 (70002) [000] .... 10.144000: sched_wakeup: comm=tb1 pid=59567 prio=20 target_cpu=000
         tb7-59573 (59566) [003] .... 10.144000: sched_switch: prev_comm=tb7 prev_pid=59573 prev_prio=20 prev_state=S ==> next_comm=tb8 next_pid=59574 next_prio=20
         tb8-59574 (59566) [003] .... 10.148000: sched_switch: prev_comm=tb8 prev_pid=59574 prev_prio=20 prev_state=S ==> next_comm=tb9 next_pid=59575 next_prio=20
        hog2-70002 (70002) [000] .... 10.149000: sched_switch: prev_comm=hog2 prev_pid=70002 prev_prio=20 prev_state=S ==> next_comm=tb1 next_pid=59567 next_prio=20
        hog4-70004 (70004) [001] .... 10.152000: sched_wakeup: comm=tb3 pid=59569 prio=20 target_cpu=001
         tb9-59575 (59566) [003] .... 10.152000: sched_switch: prev_comm=tb9 prev_pid=59575 prev_prio=20 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
         tb1-59567 (59566) [000] .... 10.155000: sched_switch: prev_comm=tb1 prev_pid=59567 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
        hog4-70004 (70004) [001] .... 10.155000: sched_switch: prev_comm=hog4 prev_pid=70004 prev_prio=20 prev_state=S ==> next_comm=tb3 next_pid=59569 next_prio=20
         tb3-59569 (59566) [001] .... 10.155500: sched_wakeup: comm=tb4 pid=59570 prio=20 target_cpu=001
NetworkService-60595 (59566) [000] .... 10.156000: sched_wakeup: comm=tb2 pid=59568 prio=20 target_cpu=000
         tb3-59569 (59566) [001] .... 10.158000: sched_switch: prev_comm=tb3 prev_pid=59569 prev_prio=20 prev_state=S ==> next_comm=tb4 next_pid=59570 next_prio=20
NetworkService-60595 (59566) [000] .... 10.160000: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=tb2 next_pid=59568 next_prio=20
        hog6-70006 (70006) [002] .... 10.161000: sched_switch: prev_comm=hog6 prev_pid=70006 prev_prio=20 prev_state=S ==> next_comm=tb5 next_pid=59571 next_prio=20
         tb4-59570 (59566) [001] .... 10.162000: sched_switch: prev_comm=tb4 prev_pid=59570 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
         tb2-59568 (59566) [000] .... 10.164000: sched_switch: prev_comm=tb2 prev_pid=59568 prev_prio=20 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
         tb5-59571 (59566) [002] .... 10.164000: sched_switch: prev_comm=tb5 prev_pid=59571 prev_prio=20 prev_state=S ==> next_comm=tb6 next_pid=59572 next_prio=20
NetworkService-60595 (59566) [001] .... 10.166635: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
         tb6-59572 (59566) [002] .... 10.168000: sched_switch: prev_comm=tb6 prev_pid=59572 prev_prio=20 prev_state=S ==> next_comm=NetworkService next_pid=60595 next_prio=20
NetworkService-60595 (59566) [002] .... 10.171500: sched_switch: prev_comm=NetworkService prev_pid=60595 prev_prio=20 prev_state=S ==> next_comm=swapper/2 next_pid=0 next_prio=120
com.baidu.tieba-59566 (59566) [000] .... 10.180000: print: C|59566|wsr_census_probe|1
`)
	for i := 1; i <= 28; i++ {
		fmt.Fprintf(&b, "         tbw%02d-%d (59566) [001] .... 10.%06d: print: C|59566|wsr_census_probe|1\n", i, 59575+i, 180000+i*100)
	}
	return b.String()
}

func wsrB3ExecuteWindowStats(t *testing.T, withPID bool) types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wsr_b3.systrace"), []byte(wsrB3CensusTraceText()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	pid := ""
	if withPID {
		pid = `"pid":59566,`
	}
	params := json.RawMessage(`{"source":"path","path":"wsr_b3.systrace","view":"window_stats",` + pid + `"time_start":10.0,"time_end":10.2,"trace_flavor":"harmony_hitrace"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query window_stats failed: %s", res.Summary)
	}
	return res
}

func wsrB3LinesWithPrefix(text, prefix string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

// TestTraceQueryWindowStatsRendersProcessDomainCensus pins the WSR §8 b3
// rendered lane on the pid run: honest threads= census, the cross-CPU merged
// NetworkService roster row, the PTS fold row (count + aggregate), the
// caliber/unit caveats — while the legacy process_cpu_load masquerade row
// keeps its surviving-roster shape in the same output.
//
// WSR 核验 F2: the caveat assertions pin CLAUSE SURVIVAL on the RENDERED
// face — including each caveat's tail clause — so a re-merged >200-byte
// caveat whose core clause falls past the sanitizeForBanner cut goes red
// here (pinning only a surviving prefix was the verified blind spot).
func TestTraceQueryWindowStatsRendersProcessDomainCensus(t *testing.T) {
	res := wsrB3ExecuteWindowStats(t, true)
	for _, want := range []string{
		"- process_domain_census(进程域普查) process=com.baidu.tieba-59566 threads=39 running_threads=10 running_total=49.135cpu·ms cpus=0,1,2,3",
		"- process_domain_census_thread NetworkService-60595 running=13.135ms cpus=0,1,2",
		"- process_domain_census_thread tb1-59567 running=6.000ms cpus=0",
		"- process_domain_census_fold remaining_threads=2 running_total=6.000cpu·ms",
		"其余 2 线程合计",
		// Caliber caveats: head AND tail clauses of each entry survive.
		"- process_domain_census_caveat=threads=39 is the true in-window census",
		"in-window thread observations",
		"their threads= counts survivors, not the process",
		"- process_domain_census_caveat=threads= 为该进程时窗内全部可见线程普查,非展示名册幸存者数",
		// Unit caveats: cross-CPU merge soundness + CMP-3 cpu·ms contract,
		// CJK tail included (its survival proves no mid-caveat truncation).
		"- process_domain_census_caveat=per-thread running merges the same thread's non-overlapping cross-CPU segments (wall-additive for one thread)",
		"跨线程合计为 cpu·ms,不可当作墙钟耗时",
		// Legacy face untouched in the same output: the surviving-roster
		// rollup keeps reporting exactly the b3 masquerade shape.
		"- process_cpu_load process=com.baidu.tieba-59566 threads=4 running=6.000ms runnable=14.500ms",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("window_stats summary missing %q:\n%s", want, res.Summary)
		}
	}
	if !utf8.ValidString(res.Summary) {
		t.Fatal("window_stats summary must be valid UTF-8 (rune-safe banner truncation)")
	}
}

// TestTraceQueryProcessDomainCensusGlobalFaceBytesPin is the byte-level
// additive-lane pin: the census rows exist ONLY on the pid run, and the
// global leaderboard faces (top_running + process_cpu_load rendered lines)
// are byte-identical between the pid run and the target-less run.
func TestTraceQueryProcessDomainCensusGlobalFaceBytesPin(t *testing.T) {
	withPID := wsrB3ExecuteWindowStats(t, true)
	noPID := wsrB3ExecuteWindowStats(t, false)
	if strings.Contains(noPID.Summary, "process_domain_census") {
		t.Fatalf("target-less window_stats must not render the census lane:\n%s", noPID.Summary)
	}
	if !strings.Contains(withPID.Summary, "process_domain_census") {
		t.Fatal("pid window_stats must render the census lane")
	}
	for _, prefix := range []string{"- top_running ", "- process_cpu_load "} {
		got := wsrB3LinesWithPrefix(withPID.Summary, prefix)
		want := wsrB3LinesWithPrefix(noPID.Summary, prefix)
		if len(got) == 0 {
			t.Fatalf("expected %q lines in the pid run", prefix)
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%q lines must be byte-identical with/without pid:\nwith pid:\n%s\nwithout pid:\n%s",
				prefix, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}
