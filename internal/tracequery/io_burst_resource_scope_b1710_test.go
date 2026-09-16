package tracequery

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// A real target request closes its S/D wait, but carries no inode identity.
// Another thread owns the larger block request and exact inode/storage pair.
// Same-thread page-cache activity is deliberately present too: proximity and
// a matching PID still do not identify the resource behind a scheduler wait.
func b1710PublicFixture(t *testing.T, state string, ioWait int) (*Index, Result) {
	t.Helper()
	body := strings.Join([]string{
		"idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=20",
		"writer-50 (50) [002] .... 5.000010: mm_filemap_add_to_page_cache: dev 8:0 ino 0xaa page=0000000000000000 pfn=1 ofs=0",
		"writer-50 (50) [002] .... 5.000020: mm_filemap_add_to_page_cache: dev 8:0 ino 0xbb page=0000000000000000 pfn=2 ofs=0",
		"worker-41 (41) [001] .... 5.000030: mm_filemap_add_to_page_cache: dev 8:1 ino 0xcc page=0000000000000000 pfn=3 ofs=0",
		"writer-50 (50) [002] .... 5.000040: block_rq_issue: 8,0 R 4096 () 999 + 8 [writer]",
		"writer-50 (50) [002] .... 5.000050: ext4_da_write_begin: dev 8,0 ino 0xaa pos 0 len 4096 flags 0",
		"writer-50 (50) [002] .... 5.000070: ext4_da_write_end: dev 8,0 ino 0xaa pos 0 len 4096 copied 4096",
		"worker-41 (41) [001] .... 5.000100: block_rq_issue: 8,1 R 4096 () 123 + 8 [worker]",
		fmt.Sprintf("worker-41 (41) [001] .... 5.000120: sched_switch: prev_comm=worker prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120", state),
		"irq-2 (2) [001] .... 5.000199: block_rq_complete: 8,1 R () 123 + 8 [0]",
		"irq-2 (2) [001] .... 5.000210: sched_wakeup: comm=worker pid=41 prio=20 target_cpu=001",
		fmt.Sprintf("irq-2 (2) [001] .... 5.000210: sched_blocked_reason: pid=41 iowait=%d caller=wait_for_buffer+0x4/0x10", ioWait),
		"idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=20",
		"irq-3 (3) [002] .... 5.000340: block_rq_complete: 8,0 R () 999 + 8 [0]",
		"worker-41 (41) [001] .... 5.000400: sched_switch: prev_comm=worker prev_pid=41 prev_prio=20 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120",
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "resource-scope.systrace")
	writeBundleProvenanceFixture(t, path, body)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "window_stats", PID: 41, TimeStart: 5, TimeEnd: 5.0004, MinDurationMs: .001, Limit: 20})
	if res.WindowStats == nil {
		t.Fatal("public query did not expose window statistics")
	}
	return idx, res
}

func TestB1710PublicSchedulerBurstDoesNotBorrowWholeWindowResources(t *testing.T) {
	for _, ioWait := range []int{0, 1} {
		t.Run(fmt.Sprintf("D_iowait_%d", ioWait), func(t *testing.T) {
			_, res := b1710PublicFixture(t, "D", ioWait)
			found := false
			for _, row := range res.WindowStats.IOBurstEpisodes {
				if row.Thread.PID != 41 || (row.DominantSignal != "scheduler_iowait" && row.DominantSignal != "d_state_or_io_wait") {
					continue
				}
				found = true
				if math.Abs(row.DurationMs-.090) > 1e-6 || row.RootCauseEligibility != "context_only_derived_projection" {
					t.Errorf("scheduler episode lost its own wait/eligibility: %+v", row)
				}
				if row.TopInode != "" || row.TopDev != "" || row.TopEntryName != "" || row.PageCacheChurn != 0 ||
					row.FileIOBytes != 0 || row.BlockMaxLatencyMs != 0 || row.StorageMaxLatencyMs != 0 {
					t.Errorf("scheduler wait borrowed unjoined whole-window resource fields: %+v", row)
				}
				for _, key := range []string{"block_max=", "storage_max=", "inode=", "file_bytes=", "page_cache_churn="} {
					if strings.Contains(row.Summary, key) {
						t.Errorf("unassigned resource must not become a displayed zero/unknown value (%s): %s", key, row.Summary)
					}
				}
			}
			if !found {
				t.Fatal("the scheduler wait episode must remain visible")
			}
			foundFact := false
			for _, fact := range res.EvidencePack {
				if fact.Predicate == "io_burst_episode" && fact.Subject == "worker-41" {
					foundFact = true
					if fact.Object != "" {
						t.Errorf("public evidence fact minted an unproved wait resource identity: %+v", fact)
					}
					for _, inode := range []string{"0xaa", "0xbb", "0xcc"} {
						if strings.Contains(fact.Summary, "inode="+inode) {
							t.Errorf("public wait summary retained a borrowed inode: %+v", fact)
						}
					}
				}
			}
			if !foundFact {
				t.Fatal("resource isolation must not delete the public scheduler evidence fact")
			}
		})
	}
}

func TestB1710PublicBackgroundAndExactWaitAuthorityRemain(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			idx, res := b1710PublicFixture(t, state, 1)
			stats := res.WindowStats
			pressure := stats.IOPressureSummary
			if pressure == nil || pressure.PageCacheChurn != 3 || pressure.TopInode == "" || pressure.FileIOBytes <= 0 ||
				math.Abs(pressure.BlockMaxLatencyMs-.300) > 1e-6 || pressure.StorageMaxLatencyMs <= 0 ||
				len(stats.PageCacheByInode) != 3 || len(stats.IOLatencies) != 2 {
				t.Fatalf("complete whole-window resource background must remain intact: pressure=%+v cache=%+v IO=%+v", pressure, stats.PageCacheByInode, stats.IOLatencies)
			}
			exactInode := false
			for _, episode := range stats.IOBurstEpisodes {
				if episode.Thread.PID == 50 && episode.TopInode == "0xaa" && episode.RootCauseEligibility == "eligible_exact_chain_host_work" {
					exactInode = episode.StorageMaxLatencyMs > 0 && episode.PageCacheChurn == 1
				}
			}
			if !exactInode {
				t.Fatalf("producer-owned inode/storage episode must remain: episodes=%+v inode=%+v storage=%+v", stats.IOBurstEpisodes, stats.BlockIOByInode, stats.StorageLatencyByLayer)
			}
			q := Query{PID: 41, TimeStart: 5, TimeEnd: 5.0004, MinDurationMs: .001, Limit: 20}
			q.View = "thread_timeline"
			timeline := Run(idx, q)
			if timeline.Timeline == nil {
				t.Fatal("target timeline missing")
			}
			waitFound := false
			for _, row := range timeline.Timeline.Intervals {
				if math.Abs(row.StartTs-5.000120) < 1e-9 && math.Abs(row.EndTs-5.000210) < 1e-9 {
					waitFound = row.PrevStateRaw == state && math.Abs(row.DurationMs-.090) < 1e-6
				}
			}
			if !waitFound {
				t.Fatalf("raw S/D wait and its exact boundaries must survive: %+v", timeline.Timeline.Intervals)
			}
			q.View = "root_cause_rank"
			rank := Run(idx, q)
			if rank.RootCauseRank == nil {
				t.Fatal("root-cause result missing")
			}
			closed := false
			for _, row := range append(rank.RootCauseRank.Items, rank.RootCauseRank.AbsorbedItems...) {
				if row.Type == "io_latency" && row.Thread.PID == 41 && row.ResourceCompletionClosure && row.ChainRelevance == "on_chain" {
					closed = math.Abs(row.EffectiveImpactMs-.090) < 1e-6
				}
			}
			if !closed {
				t.Fatal("exact request completion must retain its on-chain 90us S/D wait authority")
			}
		})
	}
}
