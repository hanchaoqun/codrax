package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These are deliberately synthetic captures, not alterations of a customer
// trace. Both reuse target/transaction IDs and basename, as independent
// captures legitimately can; only the full source identity separates them.
func b1618CaptureWait(start, end float64, tx, peer int) string {
	return fmt.Sprintf(`target-41 (41) [000] .... %.6f: binder_transaction: transaction=%d dest_proc=%d dest_thread=%d reply=0 flags=0x0 code=0x1
target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=server next_pid=%d next_prio=120
server-%d (%d) [000] .... %.6f: binder_transaction_received: transaction=%d
server-%d (%d) [000] .... %.6f: binder_transaction: transaction=%d dest_proc=41 dest_thread=41 reply=1 flags=0x0 code=0x0
server-%d (%d) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000
server-%d (%d) [000] .... %.6f: sched_switch: prev_comm=server prev_pid=%d prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... %.6f: binder_transaction_received: transaction=%d
`, start-.0001, tx, peer, peer, start, peer, peer, peer, start+.00001, tx, peer, peer, end-.00001, tx+1, peer, peer, end, peer, peer, end+.00001, peer, end+.00002, tx+1)
}

func b1618TraceResult(t *testing.T, dir, path, view string, limit int) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": view, "pid": 41, "thread": "target-41",
		"time_start": 10, "time_end": 10.010, "limit": limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("real trace query %s limit=%d failed: %v %+v", view, limit, err, result)
	}
	return result
}

func b1618CaptureFixtures(t *testing.T) (string, []string, *types.RequestModel) {
	t.Helper()
	dir := t.TempDir()
	paths := []string{"a/trace.ftrace", "b/trace.ftrace"}
	traces := []string{
		b1618CaptureWait(10.001, 10.003, 7, 51),
		b1618CaptureWait(10.004, 10.007, 7, 52) + b1618CaptureWait(10.008, 10.009, 9, 52),
	}
	for i, path := range paths {
		absolute := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(traces[i]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, paths, &types.RequestModel{Intent: types.IntentRootCause, RuntimeTargets: []types.RuntimeTarget{{
		Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit",
	}}}
}

func b1618CompileTraceResults(dir string, rm *types.RequestModel, results []types.ToolResult) types.ObservationLedger {
	return types.CompileObservationLedger(types.ObservationLedgerInput{RepoRoot: dir, RequestModel: rm, ToolResults: results})
}

func TestB1618RealTraceIPCCaptureIsolationAndMatching(t *testing.T) {
	dir, paths, rm := b1618CaptureFixtures(t)
	var results []types.ToolResult
	for _, path := range paths {
		for _, view := range []string{"critical_blocking_calls", "root_cause_rank", "ipc_graph"} {
			results = append(results, b1618TraceResult(t, dir, path, view, 100))
		}
	}
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			ordered := append([]types.ToolResult(nil), results...)
			if reverse {
				for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
					ordered[i], ordered[j] = ordered[j], ordered[i]
				}
			}
			ledger := b1618CompileTraceResults(dir, rm, ordered)
			before, _ := json.Marshal(ledger)
			requests := types.BuildTraceIPCRequestCensusAuthorities(ledger, rm)
			if len(requests) != 2 {
				t.Fatalf("independent same-basename captures need two IPC authorities, not a mixed roster: %+v", requests)
			}
			seen := map[string]bool{}
			captureKeys := map[string]bool{}
			for _, request := range requests {
				if request.ArtifactKey == "" || request.SourceRecordID == "" {
					t.Fatalf("real census lost its full capture or source-result identity: %+v", request)
				}
				captureKeys[request.ArtifactKey] = true
				if len(request.SyncRoster) == 0 {
					t.Fatalf("real fixture must publish a nonempty request roster: %+v", request)
				}
				peer := request.SyncRoster[0].Peer
				want := map[string]int{"server-51": 1, "server-52": 2}[peer]
				if want == 0 || request.TotalRequests != want || len(request.SyncRoster) != want || request.CoverageStatus != "complete" {
					t.Fatalf("capture totals and complete roster must share one source result: %+v", request)
				}
				seen[peer] = true
				for _, row := range request.SyncRoster {
					if row.Peer != peer || (row.TransactionID == 7 && ((peer == "server-51" && row.SendTs != 10.0009) || (peer == "server-52" && row.SendTs != 10.0039))) {
						t.Fatalf("transaction 7 borrowed fields from the other capture: %+v", request)
					}
				}
			}
			if len(seen) != 2 || len(captureKeys) != 2 {
				t.Fatalf("one physical capture disappeared: %+v", requests)
			}
			blocking := types.BuildTraceBlockingWallClockAuthorities(ledger, rm)
			matched := map[string]bool{}
			for _, block := range blocking {
				if block.Type != "binder_wait" {
					continue
				}
				request, ok := matchingTraceIPCRequestCensusAuthority(block, requests)
				if !ok || block.ArtifactKey == "" || block.ArtifactKey != request.ArtifactKey || len(block.Occurrences) == 0 || request.SyncRoster[0].Peer != block.Occurrences[0].Peer {
					t.Fatalf("tool reader matched a foreign capture or lost the unique matching capture: blocking=%+v requests=%+v", block, requests)
				}
				matched[request.SyncRoster[0].Peer] = true
			}
			if len(matched) != 2 {
				t.Fatalf("real fixture must exercise both blocking→request joins, got %v: %+v", matched, blocking)
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("authority/join compilation altered original observation records")
			}
		})
	}
}

func TestB1618RealTraceIPCDifferentResultLimitsRemainSeparate(t *testing.T) {
	dir, paths, rm := b1618CaptureFixtures(t)
	blockingResult := b1618TraceResult(t, dir, paths[1], "critical_blocking_calls", 100)
	narrow := b1618TraceResult(t, dir, paths[1], "ipc_graph", 1)
	full := b1618TraceResult(t, dir, paths[1], "ipc_graph", 100)
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			results := []types.ToolResult{blockingResult, narrow, full}
			if reverse {
				results[1], results[2] = results[2], results[1]
			}
			ledger := b1618CompileTraceResults(dir, rm, results)
			before, _ := json.Marshal(ledger)
			requests := types.BuildTraceIPCRequestCensusAuthorities(ledger, rm)
			if len(requests) != 2 {
				t.Fatalf("different limit/result cohorts must not become one complete census: %+v", requests)
			}
			if requests[0].ArtifactKey == "" || requests[0].ArtifactKey != requests[1].ArtifactKey || requests[0].SourceRecordID == "" || requests[0].SourceRecordID == requests[1].SourceRecordID {
				t.Fatalf("limits must retain the same capture but distinct result identities: %+v", requests)
			}
			var complete, partial int
			for _, request := range requests {
				if request.CoverageStatus == "complete" {
					complete++
					if request.TotalRequests != 2 || len(request.SyncRoster) != 2 {
						t.Fatalf("full result lost its own roster: %+v", request)
					}
				} else {
					partial++
					if !strings.Contains(request.CoverageStatus, "lower_bound") || len(request.SyncRoster) != 1 {
						t.Fatalf("truncated result borrowed the other result's rows: %+v", request)
					}
				}
			}
			if complete != 1 || partial != 1 {
				t.Fatalf("expected distinct full and partial query results: %+v", requests)
			}
			blocks := types.BuildTraceBlockingWallClockAuthorities(ledger, rm)
			if len(blocks) == 0 {
				t.Fatal("real fixture must exercise the tool matcher")
			}
			for _, block := range blocks {
				if got, ok := matchingTraceIPCRequestCensusAuthority(block, requests); ok {
					t.Fatalf("multiple result cohorts must not select the first result: %+v", got)
				}
			}
			after, _ := json.Marshal(ledger)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("ambiguous display join changed the original observations")
			}
		})
	}
}

func TestB1618IPCRequestMatcherRequiresUniqueExactScope(t *testing.T) {
	key := types.TraceCausalProjectionRecordArtifactIdentity(types.ObservationRecord{SourceRef: types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/A/trace.ftrace",
	}})
	if key == "" {
		t.Fatal("test requires the existing full-capture identity constructor")
	}
	block := types.TraceBlockingWallClockAuthority{ArtifactKey: key, ArtifactLabel: "trace.ftrace", SelectedWindow: "10.000000..10.010000", Subject: "target-41"}
	request := types.TraceIPCRequestCensusAuthority{ArtifactKey: key, ArtifactLabel: "trace.ftrace", SelectedWindow: block.SelectedWindow, Subject: block.Subject, SourceRecordID: "result-a#census", TotalRequests: 3}
	tests := []struct {
		name   string
		mutate func(*types.TraceBlockingWallClockAuthority, *[]types.TraceIPCRequestCensusAuthority)
		want   bool
	}{
		{"unique full scope", func(_ *types.TraceBlockingWallClockAuthority, _ *[]types.TraceIPCRequestCensusAuthority) {}, true},
		{"different display labels same capture", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].ArtifactLabel = "reader alias"
		}, true},
		{"same basename different capture", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].ArtifactKey = strings.ReplaceAll(key, "/A/", "/B/")
		}, false},
		{"case distinct full capture", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].ArtifactKey = strings.ReplaceAll(key, "/A/", "/a/")
		}, false},
		{"unknown blocking capture", func(b *types.TraceBlockingWallClockAuthority, _ *[]types.TraceIPCRequestCensusAuthority) {
			b.ArtifactKey = ""
		}, false},
		{"unknown census capture", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].ArtifactKey = ""
		}, false},
		{"both unknown capture", func(b *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			b.ArtifactKey, (*rs)[0].ArtifactKey = "", ""
		}, false},
		{"wrong target", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].Subject = "target-42"
		}, false},
		{"both missing target", func(b *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			b.Subject, (*rs)[0].Subject = "", ""
		}, false},
		{"wrong exact window", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			(*rs)[0].SelectedWindow = "10.000000..10.010001"
		}, false},
		{"both missing window", func(b *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			b.SelectedWindow, (*rs)[0].SelectedWindow = "", ""
		}, false},
		{"multiple distinct result cohorts", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			other := (*rs)[0]
			other.SourceRecordID, other.TotalRequests = "result-b#census", 1
			*rs = append(*rs, other)
		}, false},
		{"foreign candidate before unique match", func(_ *types.TraceBlockingWallClockAuthority, rs *[]types.TraceIPCRequestCensusAuthority) {
			other := (*rs)[0]
			other.ArtifactKey, other.TotalRequests = key+"-foreign", 1
			*rs = append([]types.TraceIPCRequestCensusAuthority{other}, *rs...)
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, rs := block, []types.TraceIPCRequestCensusAuthority{request}
			tc.mutate(&b, &rs)
			beforeBlock, beforeRequests := b, append([]types.TraceIPCRequestCensusAuthority(nil), rs...)
			got, ok := matchingTraceIPCRequestCensusAuthority(b, rs)
			if ok != tc.want || (ok && got.TotalRequests != request.TotalRequests) {
				t.Fatalf("exact unique scope match=%t, want=%t; selected=%+v", ok, tc.want, got)
			}
			if !reflect.DeepEqual(beforeBlock, b) || !reflect.DeepEqual(beforeRequests, rs) {
				t.Fatal("display matching mutated caller-owned authority records")
			}
		})
	}
}
