package tracequery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func b1709Endpoints(family, comm string) (string, string) {
	if family == "bio" {
		return "block_bio_queue: 8,0 R 123 + 8 [" + comm + "]", "block_bio_complete: 8,0 R 123 + 8 [0]"
	}
	return "block_rq_issue: 8,0 R 4096 () 123 + 8 [" + comm + "]", "block_rq_complete: 8,0 R () 123 + 8 [0]"
}

// All assertions consume public BuildIndex/Run output. The split fixture uses
// two real, separately attested artifacts rather than hand-editing an Index.
func b1709Run(t *testing.T, issue, done, state string, split bool) (*Index, Result) {
	t.Helper()
	left := "idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=20\n" +
		"worker-41 (41) [001] .... 5.000100: " + issue + "\n" +
		fmt.Sprintf("worker-41 (41) [001] .... 5.000120: sched_switch: prev_comm=worker prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", state)
	right := "irq-2 (2) [001] .... 5.000199: " + done + "\n" +
		"irq-2 (2) [001] .... 5.000210: sched_wakeup: comm=worker pid=41 prio=20 target_cpu=001\n" +
		"idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=20\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "block.systrace")
	if split {
		writeBundleProvenanceFixture(t, filepath.Join(dir, "issue.systrace"), left)
		writeBundleProvenanceFixture(t, filepath.Join(dir, "complete.systrace"), right)
		path = filepath.Join(dir, "split.tracebundle.json")
		writeBundleProvenanceFixture(t, path, `{"version":"test","systrace":"issue.systrace","artifacts":[{"type":"systrace","path":"issue.systrace"},{"type":"systrace","path":"complete.systrace"}]}`)
	} else {
		writeBundleProvenanceFixture(t, path, left+right)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := Run(idx, Query{View: "root_cause_rank", PID: 41, TimeStart: 5, TimeEnd: 5.001, MinDurationMs: .001, Limit: 12})
	if result.WindowStats == nil || result.RootCauseRank == nil {
		t.Fatal("public query did not expose IO/rank output")
	}
	return idx, result
}

func TestB1709PublicBracketCommPreservesRQAndBIOClosure(t *testing.T) {
	for _, family := range []string{"rq", "bio"} {
		for _, state := range []string{"S", "D"} {
			for _, comm := range []string{"worker", "[pool]worker", "pool[io]", "pool]io", "[pool", "[pool] [io]"} {
				t.Run(family+"/"+state+"/"+comm, func(t *testing.T) {
					issue, done := b1709Endpoints(family, comm)
					_, result := b1709Run(t, issue, done, state, false)
					if len(result.WindowStats.IOLatencies) != 1 {
						t.Fatalf("opaque comm %q lost its exact issue/complete pair: IO=%+v", comm, result.WindowStats.IOLatencies)
					}
					io := result.WindowStats.IOLatencies[0]
					wantState, caliber := string(StateSSleep), BlockIOWaitCaliberIssueToComplete
					if state == "D" {
						wantState = string(StateDSleep)
					}
					if family == "bio" {
						caliber = BlockIOWaitCaliberBIOQueueToComplete
					}
					if io.EndpointFamily != "block_"+family || io.WaitCaliber != caliber || fmt.Sprintf("%.3f", io.DurationMs) != "0.099" ||
						io.IssueLine != 2 || io.CompleteLine != 4 || io.IssueThread.PID != 41 || io.CompleteThread.PID != 2 {
						t.Fatalf("request identity/residence must remain exact: %+v", io)
					}
					if !io.CompletionWokeIssuer || io.WakeupLine != 5 || io.IssuerBlockedState != wantState ||
						io.CausalWaitCaliber != BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked || fmt.Sprintf("%.3f", io.IssuerBlockedMs) != "0.090" {
						t.Fatalf("S/D issuer wait must remain separate from 99us request residence: %+v", io)
					}
					found := false
					for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
						if row.Type == "io_latency" && row.Thread.PID == 41 {
							found = row.ChainRelevance == "on_chain" && row.ResourceCompletionClosure && fmt.Sprintf("%.3f", row.EffectiveImpactMs) == "0.090"
						}
					}
					if !found {
						t.Fatal("completion-closed IO lost its public on-chain proof")
					}
				})
			}
		}
	}
}

func TestB1709BracketCommInvalidatesV40Cache(t *testing.T) {
	if ParserVersion != "tracequery-v43" {
		t.Fatalf("opaque block comm parsing requires a new cache generation: %q", ParserVersion)
	}
	cache := newTraceIndexCache(1 << 20)
	oldKey := parseCacheKey{path: "block.trace", size: 1, modUnix: 1, version: "tracequery-v40"}
	currentKey := oldKey
	currentKey.version = ParserVersion
	cache.Store(oldKey, &Index{Path: oldKey.path, Events: []Event{{Type: EventUnknown}}})
	if _, ok := cache.Load(oldKey); !ok {
		t.Fatal("old-generation fixture was not admitted to the cache")
	}
	if _, ok := cache.Load(currentKey); ok {
		t.Fatal("v41 parser reused an index that may have lost opaque bracket comm requests")
	}
	cache.Store(currentKey, &Index{Path: currentKey.path, Events: []Event{{Type: EventUnknown}}})
	if _, ok := cache.Load(currentKey); !ok {
		t.Fatal("current-generation index was not reusable")
	}
}

func TestB1709PublicBracketCommDoesNotRelaxEndpointGuards(t *testing.T) {
	for _, family := range []string{"rq", "bio"} {
		issue, done := b1709Endpoints(family, "[pool]worker")
		for _, tc := range []struct {
			name, issue, done string
			split             bool
		}{
			{"cross_source", issue, done, true},
			{"wrong_dev", issue, strings.Replace(done, "8,0", "8,1", 1), false},
			{"wrong_op", issue, strings.Replace(done, " R ", " W ", 1), false},
			{"wrong_sector", issue, strings.Replace(done, "123", "124", 1), false},
			{"wrong_length", issue, strings.Replace(done, "+ 8", "+ 16", 1), false},
			{"missing_outer_close", strings.TrimSuffix(issue, "]"), done, false},
			{"embedded_newline", strings.Replace(issue, "[pool]", "[pool\n]", 1), done, false},
			{"sector_overflow", strings.Replace(issue, "123", "9223372036854775808", 1), done, false},
			{"length_overflow", strings.Replace(issue, "+ 8", "+ 4294967296", 1), done, false},
			{"completion_status_overflow", issue, strings.Replace(done, "[0]", "[2147483648]", 1), false},
			{"completion_status_not_number", issue, strings.Replace(done, "[0]", "[[pool]worker]", 1), false},
		} {
			t.Run(family+"/"+tc.name, func(t *testing.T) {
				_, result := b1709Run(t, tc.issue, tc.done, "S", tc.split)
				if len(result.WindowStats.IOLatencies) != 0 {
					t.Fatalf("malformed/mismatched endpoints acquired a pair: %+v", result.WindowStats.IOLatencies)
				}
				for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
					if row.Type == "io_latency" && (row.ResourceCompletionClosure || row.ChainRelevance == "on_chain") {
						t.Fatalf("rejected endpoints minted causal IO: %+v", row)
					}
				}
			})
		}
	}
	issue, done := b1709Endpoints("rq", "[pool]worker")
	_, result := b1709Run(t, strings.Replace(issue, "4096", "4294967296", 1), done, "S", false)
	if len(result.WindowStats.IOLatencies) != 0 {
		t.Fatal("overflowed RQ byte count acquired latency authority")
	}
}

func TestB1709PublicLegacyBracketCommRemainsInventoryOnly(t *testing.T) {
	for _, event := range []string{"block_rq_insert", "block_getrq"} {
		t.Run(event, func(t *testing.T) {
			_, done := b1709Endpoints("rq", "worker")
			idx, result := b1709Run(t, event+": 8,0 R 123 + 8 [[pool]worker]", done, "S", false)
			found := false
			for _, row := range idx.Events {
				if row.Name == event {
					found = row.BlockIOFields != nil && row.BlockIOFields.IdentityParsed && row.BlockIOFields.IdentityValid && row.BlockIOFields.Sector == 123 && row.BlockIOFields.Len == 8
				}
			}
			if !found || result.WindowStats.BlockIssueCount != 1 {
				t.Fatal("opaque bracket comm lost the legacy inventory identity")
			}
			if len(result.WindowStats.IOLatencies) != 0 {
				t.Fatal("legacy inventory was promoted to a latency endpoint")
			}
		})
	}
}
