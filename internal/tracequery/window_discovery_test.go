package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeWindowDiscoveryTrace(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "discovery.systrace")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func pairingDiscoveryRequest(start, end float64) WindowDiscoveryRequest {
	return WindowDiscoveryRequest{
		Strategy:     WindowDiscoveryPairingIntegrity,
		TimeStart:    start,
		TimeEnd:      end,
		TimeStartSet: true,
		TimeEndSet:   true,
		MaxWindows:   3,
	}
}

func TestPairingWindowDiscoveryRanksAmbiguityAndReservesCrossFamilySeat(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 1.010000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.012000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, pairingDiscoveryRequest(.99, 1.02))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || !result.IdentityComplete || result.EndpointCount != 6 {
		t.Fatalf("discovery completeness/counts = %+v", result)
	}
	if len(result.Candidates) < 2 || result.Candidates[0].Kind != "ambiguous_closed" || result.Candidates[0].Family != WindowDiscoveryFamilyBlock || result.Candidates[0].MaxDepth != 2 {
		t.Fatalf("ambiguity must rank first: %+v", result.Candidates)
	}
	if len(result.Windows) != 2 {
		t.Fatalf("expected block ambiguity + storage schema seat, got %+v", result.Windows)
	}
	families := map[WindowDiscoveryFamily]bool{}
	for _, window := range result.Windows {
		families[window.Family] = true
		if width := (window.EndTs - window.StartTs) * 1000; width <= 0 || width > HardPairingDiscoveryMaxWindowMs+1e-6 {
			t.Fatalf("generated width %.9fms violates hard cap: %+v", width, window)
		}
		if window.CoreStartTs < window.StartTs || window.CoreEndTs > window.EndTs {
			t.Fatalf("window does not cover its endpoint core: %+v", window)
		}
	}
	if !families[WindowDiscoveryFamilyBlock] || !families[WindowDiscoveryFamilyStorage] {
		t.Fatalf("cross-family witness seat missing: %+v", result.Windows)
	}
}

func TestPairingWindowDiscoverySplitsOversizeCohortWithoutLyingAboutCoverage(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.100000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.200000: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [003] .... 1.300000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	req := pairingDiscoveryRequest(.9, 1.4)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock}
	req.MaxWindows = 8
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || !result.Candidates[0].CollectionComplete || result.Candidates[0].FitsSingleWindow || result.Candidates[0].RequiredWindowCount != 4 {
		t.Fatalf("oversize cohort coverage metadata = %+v", result.Candidates)
	}
	if len(result.Windows) != 4 {
		t.Fatalf("all four endpoint clusters must be generated atomically: %+v", result.Windows)
	}
	for _, endpointTs := range []float64{1, 1.1, 1.2, 1.3} {
		covered := false
		for _, window := range result.Windows {
			if endpointTs >= window.StartTs-1e-12 && endpointTs <= window.EndTs+1e-12 {
				covered = true
			}
			if (window.EndTs-window.StartTs)*1000 > 50+1e-6 {
				t.Fatalf("split window exceeded 50ms: %+v", window)
			}
		}
		if !covered {
			t.Fatalf("endpoint %.6f was omitted by split windows: %+v", endpointTs, result.Windows)
		}
	}

	// A smaller fan-out budget must reject the candidate as a whole; emitting
	// three of four slices would look complete while omitting one endpoint.
	req.MaxWindows = 3
	limited, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Windows) != 0 || !strings.Contains(strings.Join(limited.Caveats, "\n"), "generated_windows=0") {
		t.Fatalf("partial candidate leaked through a smaller budget: %+v", limited)
	}
}

func TestPairingWindowDiscoveryOpenAmbiguityIsDisclosedButNotCollected(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 1.010000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.012000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, pairingDiscoveryRequest(.99, 1.02))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) < 2 || result.Candidates[0].Kind != "ambiguous_eof" || result.Candidates[0].CollectionComplete {
		t.Fatalf("EOF ambiguity disclosure = %+v", result.Candidates)
	}
	if len(result.Windows) != 1 || result.Windows[0].Family != WindowDiscoveryFamilyStorage || result.Windows[0].Kind != "schema_probe" {
		t.Fatalf("collector must skip open cohort and use the closed schema probe: %+v", result.Windows)
	}
}

func TestPairingWindowDiscoveryReplaysPrefixAndCollectsCarryInEndpoints(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 0.800000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 0.810000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 0.900000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.010000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	req := pairingDiscoveryRequest(1.0, 1.02)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].FirstLine != 3 || result.Candidates[0].LastLine != 4 {
		t.Fatalf("closed pre-window pair survived as false carry-in: %+v", result.Candidates)
	}
	if len(result.Windows) != 2 || result.Windows[0].CoreStartTs != .9 || result.Windows[1].CoreEndTs != 1.01 {
		t.Fatalf("carry-in start and in-window completion were not collected atomically: %+v", result.Windows)
	}
	if result.ScopedEndpointCount != 1 || result.Families[0].CompletedPairCount != 1 {
		t.Fatalf("scope/replay accounting = result:%+v family:%+v", result, result.Families)
	}
}

func TestPairingWindowDiscoveryBudgetAndLifecycleFailClosed(t *testing.T) {
	budgetPath := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	req := pairingDiscoveryRequest(.9, 1.1)
	req.EndpointLimit = 2
	limited, err := DiscoverWindows(context.Background(), budgetPath, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Complete || limited.IdentityComplete || !limited.BudgetStopped || len(limited.Windows) != 0 {
		t.Fatalf("endpoint budget did not fail closed: %+v", limited)
	}

	lifecyclePath := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.001000: sched_switch: prev_comm=io prev_pid=40 prev_prio=20 prev_state=X ==> next_comm=idle/0 next_pid=0 next_prio=120
 io-40 (40) [003] .... 1.002000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	req = pairingDiscoveryRequest(.9, 1.1)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	lifecycle, err := DiscoverWindows(context.Background(), lifecyclePath, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.IdentityComplete || len(lifecycle.Windows) != 1 || lifecycle.Windows[0].Kind != "schema_probe" {
		t.Fatalf("TID generation cut crossed or erased the new valid pair: %+v", lifecycle)
	}
	if len(lifecycle.Families) != 1 || lifecycle.Families[0].LifecycleResetLaneCount != 1 || lifecycle.Families[0].ClosedAmbiguousCount != 0 {
		t.Fatalf("lifecycle accounting = %+v", lifecycle.Families)
	}
}

func TestPairingWindowDiscoverySameLaneRollbackCannotGenerateTimeWindow(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 2.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.900000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 2.100000: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [003] .... 2.200000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	req := pairingDiscoveryRequest(1.8, 2.3)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock}
	result, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdentityComplete || len(result.Windows) != 0 || len(result.Candidates) != 1 || result.Candidates[0].CollectionBlockedReason != "same_lane_timestamp_rollback" {
		t.Fatalf("rollback cohort generated a safe-looking time window: %+v", result)
	}
	if result.ClockRegressions == 0 || result.Families[0].TimestampRollbackCount != 1 {
		t.Fatalf("rollback evidence was not disclosed: %+v", result)
	}
}

func TestPairingWindowDiscoveryExplicitZeroSecondScopeAndDeterminism(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `
 io-40 (40) [003] .... 0.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 0.001000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	req := pairingDiscoveryRequest(0, .01)
	req.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock}
	first, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("identical discovery runs diverged:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.Windows) != 1 || first.Windows[0].StartTs != 0 || first.Windows[0].CoreStartTs != 0 {
		t.Fatalf("explicit ts=0 scope was treated as unset: %+v", first.Windows)
	}
}

func TestWindowDiscoveryRequestValidationFailLoud(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, `io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]`)
	cases := []WindowDiscoveryRequest{
		{},
		{Strategy: "guess"},
		{Strategy: WindowDiscoveryPairingIntegrity, TimeStartSet: true, TimeStart: 1},
		{Strategy: WindowDiscoveryPairingIntegrity, TimeStartSet: true, TimeEndSet: true, TimeStart: math.NaN(), TimeEnd: 2},
		{Strategy: WindowDiscoveryPairingIntegrity, Families: []WindowDiscoveryFamily{"io"}},
		{Strategy: WindowDiscoveryPairingIntegrity, MaxWindows: HardWindowDiscoveryMaxWindows + 1},
		{Strategy: WindowDiscoveryPairingIntegrity, MaxWindowMs: 51},
		{Strategy: WindowDiscoveryPairingIntegrity, TimeStartSet: true, TimeEndSet: true, TimeStart: 1, TimeEnd: 2, LineStart: 1},
	}
	for i, request := range cases {
		if _, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, request); err == nil {
			t.Errorf("case %d must fail loud: %+v", i, request)
		}
	}
}
