package tracequery

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

func TestPairingWindowDiscoveryStorageOwnerIndexScopesResetAndPreservesBudgets(t *testing.T) {
	t.Parallel()
	storageEvent := func(line int, ts float64, pid int, dev, name string) Event {
		return Event{
			Line: line, Ts: ts, PID: pid, Type: EventStorage, Name: name,
			FieldText: fmt.Sprintf("dev=%s op=read bytes=4096", dev),
		}
	}
	request := pairingDiscoveryRequest(.5, 10)
	request.Families = []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage}
	request.EndpointLimit = 128
	request.ActiveLaneLimit = 64
	discovery := newPairingWindowDiscovery(request, "/trace/storage-owner-index.systrace")
	line := 1
	for pid := 1; pid <= 32; pid++ {
		if !discovery.observe(storageEvent(line, 1+float64(line)/10000, pid, fmt.Sprintf("12,%d", pid), "scsi_dispatch_cmd_start")) {
			t.Fatalf("unrelated lane %d unexpectedly exhausted a budget", pid)
		}
		line++
	}
	for minor := 100; minor < 103; minor++ {
		if !discovery.observe(storageEvent(line, 1+float64(line)/10000, 40, fmt.Sprintf("12,%d", minor), "scsi_dispatch_cmd_start")) {
			t.Fatalf("target lane %d unexpectedly exhausted a budget", minor)
		}
		line++
	}
	targetOwner := pairingDiscoveryOwner{source: discovery.source, pid: 40}
	if len(discovery.lanes) != 35 || len(discovery.ownerLanes[targetOwner]) != 3 || len(discovery.laneOwners) != 35 {
		t.Fatalf("storage owner index was not created symmetrically: lanes=%d target=%d reverse=%d", len(discovery.lanes), len(discovery.ownerLanes[targetOwner]), len(discovery.laneOwners))
	}
	endpointCount := discovery.endpointCount
	for i := 0; i < 10_000; i++ {
		discovery.resetStoragePID(99_999, Event{Line: line + i, Ts: 2})
	}
	if discovery.endpointCount != endpointCount || discovery.budgetStopped || len(discovery.lanes) != 35 || len(discovery.ownerLanes[targetOwner]) != 3 {
		t.Fatalf("unrelated resets changed topology or budget state: endpoints=%d/%d stopped=%t lanes=%d target=%d", discovery.endpointCount, endpointCount, discovery.budgetStopped, len(discovery.lanes), len(discovery.ownerLanes[targetOwner]))
	}
	discovery.resetStoragePID(40, Event{Line: line + 10_001, Ts: 2})
	if len(discovery.lanes) != 32 || len(discovery.ownerLanes[targetOwner]) != 0 || len(discovery.laneOwners) != 32 || discovery.stats[WindowDiscoveryFamilyStorage].LifecycleResetLaneCount != 3 {
		t.Fatalf("target reset did not remove exactly its indexed lanes: lanes=%d target=%d reverse=%d stats=%+v", len(discovery.lanes), len(discovery.ownerLanes[targetOwner]), len(discovery.laneOwners), discovery.stats[WindowDiscoveryFamilyStorage])
	}
	if !discovery.observe(storageEvent(line+10_002, 2.1, 1, "12,1", "scsi_dispatch_cmd_done")) {
		t.Fatal("legal close unexpectedly stopped discovery")
	}
	ownerOne := pairingDiscoveryOwner{source: discovery.source, pid: 1}
	if len(discovery.ownerLanes[ownerOne]) != 0 || len(discovery.lanes) != 31 || len(discovery.laneOwners) != 31 {
		t.Fatalf("normal close did not remove its owner index: lanes=%d owner=%d reverse=%d", len(discovery.lanes), len(discovery.ownerLanes[ownerOne]), len(discovery.laneOwners))
	}
	discovery.finalize(&Index{LineCount: line + 10_002, LastTs: 2.1}, TraceSourceVersion{})
	if len(discovery.lanes) != 0 || len(discovery.laneOwners) != 0 || len(discovery.ownerLanes) != 0 {
		t.Fatalf("EOF did not clear owner indexes: lanes=%d reverse=%d owners=%d", len(discovery.lanes), len(discovery.laneOwners), len(discovery.ownerLanes))
	}

	t.Run("endpoint budget", func(t *testing.T) {
		req := request
		req.EndpointLimit = 2
		req.ActiveLaneLimit = 8
		d := newPairingWindowDiscovery(req, "/trace/endpoint-budget.systrace")
		if !d.observe(storageEvent(1, 1, 1, "12,1", "scsi_dispatch_cmd_start")) || !d.observe(storageEvent(2, 1.1, 2, "12,2", "scsi_dispatch_cmd_start")) {
			t.Fatal("endpoint budget stopped before its exact limit")
		}
		for i := 0; i < 1_000; i++ {
			d.resetStoragePID(99_999, Event{Line: 3 + i, Ts: 1.2})
		}
		if d.observe(storageEvent(1003, 1.3, 3, "12,3", "scsi_dispatch_cmd_start")) || !d.budgetStopped || d.endpointCount != 2 || len(d.lanes) != 2 {
			t.Fatalf("unrelated resets bypassed endpoint budget: stopped=%t endpoints=%d lanes=%d", d.budgetStopped, d.endpointCount, len(d.lanes))
		}
	})

	t.Run("active lane budget", func(t *testing.T) {
		req := request
		req.EndpointLimit = 8
		req.ActiveLaneLimit = 2
		d := newPairingWindowDiscovery(req, "/trace/active-budget.systrace")
		if !d.observe(storageEvent(1, 1, 1, "12,1", "scsi_dispatch_cmd_start")) || !d.observe(storageEvent(2, 1.1, 2, "12,2", "scsi_dispatch_cmd_start")) {
			t.Fatal("active lane budget stopped before its exact limit")
		}
		for i := 0; i < 1_000; i++ {
			d.resetStoragePID(99_999, Event{Line: 3 + i, Ts: 1.2})
		}
		if d.observe(storageEvent(1003, 1.3, 3, "12,3", "scsi_dispatch_cmd_start")) || !d.budgetStopped || d.endpointCount != 3 || len(d.lanes) != 2 {
			t.Fatalf("unrelated resets bypassed active-lane budget: stopped=%t endpoints=%d lanes=%d", d.budgetStopped, d.endpointCount, len(d.lanes))
		}
	})
}

func TestPairingWindowDiscoveryStorageResetUsesOwnerInverseIndex(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "window_discovery.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	functions := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok {
			functions[fn.Name.Name] = fn
		}
	}
	callName := func(call *ast.CallExpr) string {
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			return fun.Name
		case *ast.SelectorExpr:
			return fun.Sel.Name
		default:
			return ""
		}
	}
	hasCall := func(fn *ast.FuncDecl, name string) bool {
		found := false
		if fn != nil {
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok && callName(call) == name {
					found = true
				}
				return true
			})
		}
		return found
	}
	reset := functions["resetStoragePID"]
	rangesAllLanes := false
	if reset != nil {
		ast.Inspect(reset.Body, func(node ast.Node) bool {
			rangeStmt, ok := node.(*ast.RangeStmt)
			if !ok {
				return true
			}
			selector, ok := rangeStmt.X.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "lanes" {
				rangesAllLanes = true
			}
			return true
		})
	}
	if reset == nil || rangesAllLanes || !hasCall(reset, "storageLaneKeys") || !hasCall(reset, "dropStorageLaneOwner") ||
		!hasCall(functions["observe"], "addStorageLaneOwner") || !hasCall(functions["observe"], "dropStorageLaneOwner") ||
		!hasCall(functions["finalize"], "dropStorageLaneOwner") {
		t.Fatalf("storage reset lost bounded owner-index maintenance: reset=%t full_scan=%t lookup=%t reset_drop=%t create=%t close=%t eof=%t",
			reset != nil, rangesAllLanes, hasCall(reset, "storageLaneKeys"), hasCall(reset, "dropStorageLaneOwner"),
			hasCall(functions["observe"], "addStorageLaneOwner"), hasCall(functions["observe"], "dropStorageLaneOwner"), hasCall(functions["finalize"], "dropStorageLaneOwner"))
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
