package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildPairingTopologyGateIndexes(t *testing.T, name string, lines []string, derive bool) (windowed, full *Index, q Query) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".systrace")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeStart: 2, TimeEndSet: true, TimeEnd: 3}
	var err error
	if derive {
		full, err = BuildIndex(context.Background(), path)
		if err == nil {
			windowed, err = BuildIndexWithOptions(context.Background(), path, opts)
		}
	} else {
		windowed, err = BuildIndexWithOptions(context.Background(), path, opts)
		if err == nil {
			full, err = BuildIndex(context.Background(), path)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if !windowed.Windowed || len(windowed.Events) != 2 || len(full.Events) != 4 {
		t.Fatalf("fixture did not exercise cropped pairing topology: windowed=%t retained=%d full=%d", windowed.Windowed, len(windowed.Events), len(full.Events))
	}
	return windowed, full, Query{TimeStart: 2, TimeEnd: 3, TimeStartSet: true, TimeEndSet: true}
}

func TestWindowedPairingTopologyGateFailsClosedForColdAndDerivedIndexes(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		check func(*testing.T, *Index, *Index, Query)
	}{
		{
			name: "workqueue",
			lines: []string{
				`worker-10 (10) [001] .... 1.000000: workqueue_execute_start: work=0x1 function=flush`,
				`worker-10 (10) [001] .... 2.100000: workqueue_execute_start: work=0x1 function=flush`,
				`worker-10 (10) [001] .... 2.200000: workqueue_execute_end: work=0x1`,
				`worker-10 (10) [001] .... 3.100000: workqueue_execute_end: work=0x1`,
			},
			check: func(t *testing.T, windowed, full *Index, q Query) {
				rows, caveats := computeWorkqueueActivity(windowed, q, 8)
				control, controlCaveats := computeWorkqueueActivity(full, q, 8)
				if len(rows) != 0 || !containsSubstring(caveats, "family=workqueue windowed_pairing_topology_incomplete=true") {
					t.Fatalf("cropped Workqueue topology failed open: rows=%+v caveats=%v", rows, caveats)
				}
				if len(control) != 1 || control[0].PairedCount != 0 || control[0].AmbiguousCohortCount != 1 || !containsSubstring(controlCaveats, "workqueue_pairing_ambiguous=true") {
					t.Fatalf("full Workqueue control lost complete-topology ambiguity: rows=%+v caveats=%v", control, controlCaveats)
				}
			},
		},
		{
			name: "dma",
			lines: []string{
				`display-20 (20) [001] .... 1.000000: dma_fence_wait_start: driver=d timeline=t context=1 seqno=2`,
				`display-20 (20) [001] .... 2.100000: dma_fence_wait_start: driver=d timeline=t context=1 seqno=2`,
				`display-20 (20) [001] .... 2.200000: dma_fence_wait_end: driver=d timeline=t context=1 seqno=2`,
				`display-20 (20) [001] .... 3.100000: dma_fence_wait_end: driver=d timeline=t context=1 seqno=2`,
			},
			check: func(t *testing.T, windowed, full *Index, q Query) {
				rows, caveats := computeDMAFenceActivity(windowed, q, 8)
				control, controlCaveats := computeDMAFenceActivity(full, q, 8)
				if len(rows) != 0 || !containsSubstring(caveats, "family=dma_fence windowed_pairing_topology_incomplete=true") {
					t.Fatalf("cropped DMA topology failed open: rows=%+v caveats=%v", rows, caveats)
				}
				if len(control) != 1 || control[0].PairedCount != 0 || control[0].AmbiguousCohortCount != 1 || !containsSubstring(controlCaveats, "dma_fence_pairing_ambiguous=true") {
					t.Fatalf("full DMA control lost complete-topology ambiguity: rows=%+v caveats=%v", control, controlCaveats)
				}
			},
		},
		{
			name: "block",
			lines: []string{
				`io-30 (30) [001] .... 1.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 [io]`,
				`io-30 (30) [001] .... 2.100000: block_rq_issue: 8,0 R 4096 () 32 + 8 [io]`,
				`io-30 (30) [001] .... 2.200000: block_rq_complete: 8,0 R () 32 + 8 [0]`,
				`io-30 (30) [001] .... 3.100000: block_rq_complete: 8,0 R () 32 + 8 [0]`,
			},
			check: func(t *testing.T, windowed, full *Index, q Query) {
				got := computeBlockIOLatencies(windowed, q, 8)
				control := computeBlockIOLatencies(full, q, 8)
				if len(got.latencies) != 0 || !containsSubstring(got.caveats, "family=block_io windowed_pairing_topology_incomplete=true") {
					t.Fatalf("cropped Block topology failed open: latencies=%+v caveats=%v", got.latencies, got.caveats)
				}
				if len(control.latencies) != 0 || !containsSubstring(control.caveats, "block_io_pairing_ambiguous=true") {
					t.Fatalf("full Block control lost complete-topology ambiguity: %+v", control)
				}
			},
		},
		{
			name: "storage",
			lines: []string{
				`io-40 (40) [001] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096`,
				`io-40 (40) [001] .... 2.100000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096`,
				`io-40 (40) [001] .... 2.200000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
				`io-40 (40) [001] .... 3.100000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096`,
			},
			check: func(t *testing.T, windowed, full *Index, q Query) {
				rows, caveats := computeStorageLatencyByLayer(windowed, q, nil, 8)
				control, controlCaveats := computeStorageLatencyByLayer(full, q, nil, 8)
				if len(rows) != 0 || !containsSubstring(caveats, "family=storage_latency windowed_pairing_topology_incomplete=true") {
					t.Fatalf("cropped Storage topology failed open: rows=%+v caveats=%v", rows, caveats)
				}
				if len(control) != 1 || control[0].PairedCount != 0 || control[0].AmbiguousCohortCount != 1 || !containsSubstring(controlCaveats, "storage_latency_pairing_ambiguous=true") {
					t.Fatalf("full Storage control lost complete-topology ambiguity: rows=%+v caveats=%v", control, controlCaveats)
				}
			},
		},
		{
			name: "binder",
			lines: []string{
				`client-10 (10) [001] .... 1.000000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
				`client-10 (10) [001] .... 2.100000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
				`server-20 (20) [002] .... 2.200000: binder_transaction_received: transaction=7`,
				`server-20 (20) [002] .... 3.100000: binder_transaction_received: transaction=7`,
			},
			check: func(t *testing.T, windowed, full *Index, q Query) {
				graph := BuildIPCGraph(windowed, q)
				control := BuildIPCGraph(full, q)
				if len(graph.Edges) != 0 || !containsSubstring(graph.Caveats, "binder_pairing_fail_closed=true; windowed_pairing_topology_incomplete=true") {
					t.Fatalf("cropped Binder topology failed open: %+v", graph)
				}
				if len(control.Edges) != 0 || !containsSubstring(control.Caveats, "binder_pairing_ambiguous=true") {
					t.Fatalf("full Binder control lost complete-topology ambiguity: %+v", control)
				}
			},
		},
	}
	for _, mode := range []struct {
		name   string
		derive bool
	}{{name: "cold"}, {name: "derived", derive: true}} {
		for _, tc := range tests {
			tc := tc
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				windowed, full, q := buildPairingTopologyGateIndexes(t, mode.name+"-"+tc.name, tc.lines, mode.derive)
				tc.check(t, windowed, full, q)
			})
		}
	}
}

func TestPairingTopologyCompletenessProofIsPrecise(t *testing.T) {
	if !completePhysicalPairingTopology(&Index{}) {
		t.Fatal("non-windowed full/hand-built index must remain complete by construction")
	}
	if completePhysicalPairingTopology(&Index{Windowed: true}) {
		t.Fatal("cropped index without a topology proof was admitted")
	}
	if !completePhysicalPairingTopology(&Index{Windowed: true, pairingTopologyComplete: true}) {
		t.Fatal("typed future topology proof was ignored")
	}
}
