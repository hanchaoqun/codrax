package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOInflightPublicUnprovenEndpointsStayUnavailable(t *testing.T) {
	const issue = "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n"
	const done = "irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 R () 123 + 8 [0]\n"
	for _, tc := range []struct {
		name, body string
	}{
		{"missing_start", done},
		{"missing_completion", issue},
		{"ambiguous_identical_requests", issue + strings.Replace(issue, "1.001000", "1.002000", 1) + strings.Replace(done, "1.004000", "1.003000", 1) + done},
		{"mismatched_family", issue + "irq-2 (2) [003] .... 1.004000: block_bio_complete: 8,0 R 123 + 8 [0]\n"},
		{"malformed_endpoint", strings.Replace(issue, "4096", "-1", 1) + done},
		{"storage_lifecycle_reset", "io-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096\n" +
			"io-40 (40) [003] .... 1.002000: sched_switch: prev_comm=io prev_pid=40 prev_prio=120 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120\n" +
			"io-40 (40) [003] .... 1.004000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := ioInflightPublicStats(t, tc.body, 1, 1.01)
			for _, row := range stats.StorageLatencyByLayer {
				if row.PairedCount != 0 {
					t.Fatalf("fixture unexpectedly acquired old pairing authority: %+v", row)
				}
			}
			wire := ioInflightPublicDecode(t, stats)
			for _, group := range wire.Groups {
				if group.Values != nil || group.AcceptedPairCount != 0 || len(group.Segments) != 0 {
					t.Fatalf("unproven endpoint topology became measured in-flight zero/count: %+v", group)
				}
			}
			withheld := false
			for _, coverage := range wire.Coverage {
				if coverage.AcceptedPairCount != 0 {
					t.Fatalf("unproven topology gained complete pairs: %+v", coverage)
				}
				withheld = withheld || coverage.Status != IOInFlightCoverageAvailable &&
					(coverage.UnpairedStartCount+coverage.UnpairedDoneCount+coverage.PairingSuppressedCount+coverage.RejectedEndpointRows > 0 || len(coverage.Reasons) > 0)
			}
			if !withheld {
				t.Fatalf("missing pairing-quality disclosure: %+v", wire.Coverage)
			}
		})
	}
}

func TestIOInflightPublicRecoveredPairDoesNotAbsorbRejectedPopulation(t *testing.T) {
	body := "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n" +
		"io-41 (41) [003] .... 1.002000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"irq-2 (2) [003] .... 1.004000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"io-40 (40) [003] .... 1.005000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.008000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
		"io-40 (40) [003] .... 1.009000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n"
	wire := ioInflightPublicDecode(t, ioInflightPublicStats(t, body, 1, 1.010))
	if len(wire.Groups) != 1 {
		t.Fatalf("recovered exact request missing: %+v", wire)
	}
	ioInflightPublicValues(t, wire.Groups[0], 1, 4, 1, .3, 3, 3)
	found := false
	for _, c := range wire.Coverage {
		if c.Family == "block" {
			found = true
			if c.Status != IOInFlightCoveragePartial || c.AcceptedPairCount != 1 || c.AmbiguousCohortCount != 1 || c.PairingSuppressedCount != 2 || c.UnpairedStartCount != 1 {
				t.Fatalf("partial accepted population lost excluded topology: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("no block-family coverage")
	}
}

func TestIOInflightPublicWindowedAndRelationScopedDoNotClaimWholePopulation(t *testing.T) {
	for _, relation := range []bool{false, true} {
		t.Run(map[bool]string{false: "windowed_topology", true: "relation_subset"}[relation], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "windowed.systrace")
			body := "io-40 (40) [003] .... 0.999000: sched_wakeup: comm=io pid=40 prio=120 target_cpu=3\n" +
				ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.003}) +
				"io-40 (40) [003] .... 1.020000: sched_wakeup: comm=io pid=40 prio=120 target_cpu=3\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{AllowWindowedParse: true, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true, RelationScoped: relation, ScopePID: 40})
			if err != nil || idx == nil || !idx.Windowed || idx.RelationScoped != relation {
				t.Fatalf("fixture did not enter real windowed/relation parser: %+v %v", idx, err)
			}
			stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
			if relation {
				if stats.IOInFlight != nil {
					t.Fatalf("relation-only subset claimed global IO: %+v", stats.IOInFlight)
				}
				return
			}
			wire := ioInflightPublicDecode(t, stats)
			for _, g := range wire.Groups {
				if g.Values != nil {
					t.Fatalf("windowed topology minted complete population: %+v", g)
				}
			}
			for _, c := range wire.Coverage {
				if c.TopologyComplete || c.Status != IOInFlightCoverageUnavailable {
					t.Fatalf("windowed pairing proof was widened: %+v", c)
				}
			}
		})
	}
}

func TestIOInflightPublicPointWindowDoesNotDivideByZero(t *testing.T) {
	stats := ioInflightPublicStats(t, ioInflightPublicRQ(ioInflightPublicPair{1.001, 1.006}), 1.004, 1.004)
	wire := ioInflightPublicDecode(t, stats)
	if wire.Window != nil || wire.WindowUnavailableReason == "" {
		t.Fatalf("point query became a positive-width denominator: %+v", wire)
	}
	for _, group := range wire.Groups {
		if group.Values != nil || len(group.Segments) != 0 {
			t.Fatalf("point query fabricated rates/durations: %+v", group)
		}
	}
}
