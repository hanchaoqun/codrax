package hitraceconv

import (
	"bytes"
	"context"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestProfilerPairFixedLedgerOrdinaryPathCallsiteGatesPinned(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "streamerdb_sorter.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var addContext *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "addContext" {
			addContext = fn
			break
		}
	}
	if addContext == nil {
		t.Fatal("traceDBRowSink.addContext AST missing")
	}
	wantGuards := map[string]string{
		"preflightProfilerPairFixedMutation": "row.pairKind != pairRenderUnknown || eventDelta != nil && *eventDelta != (traceDBProfilerEventDelta{})",
		"commitProfilerPairFixedRow":         "row.pairKind != pairRenderUnknown",
	}
	seen := make(map[string]int, len(wantGuards))
	var ancestors []ast.Node
	ast.Inspect(addContext.Body, func(node ast.Node) bool {
		if node == nil {
			ancestors = ancestors[:len(ancestors)-1]
			return true
		}
		if call, ok := node.(*ast.CallExpr); ok {
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok {
				if want, tracked := wantGuards[selector.Sel.Name]; tracked {
					guards := profilerPairFixedCallGuards(t, fset, ancestors)
					matched := false
					for _, guard := range guards {
						matched = matched || guard == want
					}
					if !matched {
						t.Fatalf("%s enclosing gates=%q missing ordinary-path gate=%q", selector.Sel.Name, guards, want)
					}
					seen[selector.Sel.Name]++
				}
			}
		}
		ancestors = append(ancestors, node)
		return true
	})
	for name := range wantGuards {
		if seen[name] != 1 {
			t.Fatalf("addContext %s callsites=%d want=1", name, seen[name])
		}
	}
}

func profilerPairFixedCallGuards(t *testing.T, fset *token.FileSet, ancestors []ast.Node) []string {
	t.Helper()
	var guards []string
	for i := len(ancestors) - 1; i >= 0; i-- {
		ifStmt, ok := ancestors[i].(*ast.IfStmt)
		if !ok {
			continue
		}
		var rendered bytes.Buffer
		if err := format.Node(&rendered, fset, ifStmt.Cond); err != nil {
			t.Fatal(err)
		}
		guards = append(guards, strings.Join(strings.Fields(rendered.String()), " "))
	}
	return guards
}

func TestProfilerNonbudgetRawMetadataRejectedBeforeMutation(t *testing.T) {
	for _, family := range []struct {
		name string
		kind pairRenderKind
	}{
		{name: "unknown", kind: pairRenderUnknown},
		{name: "workqueue", kind: pairRenderWorkqueue},
		{name: "dma_fence", kind: pairRenderDMAFence},
	} {
		for _, metadata := range []struct {
			name  string
			lane  string
			table string
		}{
			{name: "lane", lane: "raw-lane"},
			{name: "table", table: "raw-table"},
			{name: "lane_and_table", lane: "raw-lane", table: "raw-table"},
		} {
			t.Run(family.name+"/"+metadata.name, func(t *testing.T) {
				sink, err := newTraceDBRowSink(t.TempDir(), 8)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
					t.Fatal(err)
				}
				delta := traceDBProfilerEventDelta{}
				delta.poisonKinds[pairRenderF2FS] = true
				err = sink.addProfilerEventContext(context.Background(), renderedRow{
					tsNS: 1, seq: 1, line: "row", pairKind: family.kind,
					pairLane: metadata.lane, pairTable: metadata.table,
				}, delta)
				if reason := traceDBInvariantReason(err); reason != "profiler_nonbudget_pair_metadata_forbidden" {
					t.Fatalf("reason=%q err=%v", reason, err)
				}
				if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || sink.nextIngestOrdinal != 0 ||
					sink.profilerSourceProof.count != 0 || sink.poisoned[pairRenderF2FS] ||
					sink.legacyPairProof.observations != 0 || sink.blockPairProof.observations != 0 ||
					!sink.pairFixedLedger.pristine() {
					t.Fatalf("rejected raw metadata mutated sink: stats=%+v rows=%d ordinal=%d proof=%+v poisoned=%v legacy=%+v block=%+v",
						sink.stats, len(sink.rows), sink.nextIngestOrdinal, sink.profilerSourceProof,
						sink.poisoned, sink.legacyPairProof, sink.blockPairProof)
				}
			})
		}
	}
}

func TestProfilerBlockSequenceAuthorityResetDoesNotReviveLaneAccounts(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
		t.Fatal(err)
	}
	for _, row := range []renderedRow{
		{tsNS: 10, seq: 2, line: "issue", pairKind: pairRenderBlock,
			pairLane: "request", pairTable: "block_rq_issue"},
		{tsNS: 11, seq: 1, line: "complete", pairKind: pairRenderBlock,
			pairLane: "request", pairTable: "block_rq_complete"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if sink.pairAuthorityFailure != "block_physical_sequence_regression" ||
		!sink.poisoned[pairRenderBlock] || len(sink.pairLaneRegistries[pairRenderBlock].states) != 0 ||
		len(sink.pairLaneRows[pairRenderBlock]) != 0 || len(sink.pairTableRows[pairRenderBlock]) != 0 ||
		len(sink.poisonedLanes[pairRenderBlock]) != 0 || len(sink.blockLaneClocks) != 0 {
		t.Fatalf("authority reset revived Block lane state: failure=%q poisoned=%v registry=%+v lanes=%v tables=%v poison_lanes=%v clocks=%v",
			sink.pairAuthorityFailure, sink.poisoned[pairRenderBlock],
			sink.pairLaneRegistries[pairRenderBlock], sink.pairLaneRows[pairRenderBlock],
			sink.pairTableRows[pairRenderBlock], sink.poisonedLanes[pairRenderBlock], sink.blockLaneClocks)
	}
	if sink.pairRows[pairRenderBlock] != 2 || sink.pairTableTotals[pairRenderBlock]["block_rq_issue"] != 1 ||
		sink.pairTableTotals[pairRenderBlock]["block_rq_complete"] != 1 || sink.withheldPairRowsForKind(pairRenderBlock) != 2 {
		t.Fatalf("authority reset lost scalar totals: rows=%v tables=%v withheld=%d",
			sink.pairRows, sink.pairTableTotals[pairRenderBlock], sink.withheldPairRowsForKind(pairRenderBlock))
	}
	block := sink.pairFixedLedger.families[pairRenderBlock]
	issue := sink.pairFixedLedger.endpoints[profilerPairEndpointBlockRQIssue]
	complete := sink.pairFixedLedger.endpoints[profilerPairEndpointBlockRQComplete]
	if block != (profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{staged: 2, withheld: 2}, poisoned: true,
	}) || issue != (profilerPairFixedCounts{staged: 1, withheld: 1}) ||
		complete != (profilerPairFixedCounts{staged: 1, withheld: 1}) {
		t.Fatalf("authority reset fixed ledger drifted: block=%+v issue=%+v complete=%+v",
			block, issue, complete)
	}
	if err := sink.validateProfilerPairAccounting(); err != nil {
		t.Fatalf("authority-reset accounting failed: %v", err)
	}
}

func TestProfilerBlockTimestampLanePoisonPreflightRejectsBeforeMutation(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{
		tsNS: 10, seq: 1, line: "issue", pairKind: pairRenderBlock,
		pairLane: "request", pairTable: "block_rq_issue",
	}); err != nil {
		t.Fatal(err)
	}
	id, found := sink.pairLaneRegistries[pairRenderBlock].idFor("request")
	if !found {
		t.Fatal("seed Block lane identity missing")
	}
	state, found := sink.pairLaneRegistries[pairRenderBlock].state(id)
	if !found {
		t.Fatal("seed Block lane state missing")
	}
	ordinal, found := profilerPairEndpointBlockRQIssue.familyOrdinal(pairRenderBlock)
	if !found {
		t.Fatal("Block issue endpoint ordinal missing")
	}
	// Keep the lane locally representable while making its historical fold
	// exceed the fixed family total. Only the timestamp-rollback poison branch
	// observes this split before seal.
	state.endpointCounts[ordinal].rows = 2
	beforeLedger, beforeState := sink.pairFixedLedger, *state
	beforeClock := sink.blockLaneClocks["request"]
	beforeRows, beforeOrdinal := sink.stats.RowsAccepted, sink.nextIngestOrdinal
	beforeProof := sink.profilerSourceProof.count
	delta := traceDBProfilerEventDelta{}
	delta.poisonKinds[pairRenderMMC] = true
	err = sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 9, seq: 2, line: "complete", pairKind: pairRenderBlock,
		pairLane: "request", pairTable: "block_rq_complete",
	}, delta)
	if reason := traceDBInvariantReason(err); reason != "profiler_pair_fixed_ledger_plan_invalid" {
		t.Fatalf("reason=%q err=%v", reason, err)
	}
	if sink.pairFixedLedger != beforeLedger || *state != beforeState ||
		sink.blockLaneClocks["request"] != beforeClock || sink.stats.RowsAccepted != beforeRows ||
		sink.nextIngestOrdinal != beforeOrdinal || sink.profilerSourceProof.count != beforeProof ||
		sink.poisoned[pairRenderMMC] || sink.pairRows[pairRenderBlock] != 1 ||
		sink.pairTableTotals[pairRenderBlock]["block_rq_issue"] != 1 ||
		sink.pairTableTotals[pairRenderBlock]["block_rq_complete"] != 0 {
		t.Fatalf("timestamp poison preflight partially committed: ledger=%+v state=%+v clock=%+v stats=%+v ordinal=%d proof=%+v poisoned=%v rows=%v tables=%v",
			sink.pairFixedLedger, *state, sink.blockLaneClocks["request"], sink.stats,
			sink.nextIngestOrdinal, sink.profilerSourceProof, sink.poisoned,
			sink.pairRows, sink.pairTableTotals[pairRenderBlock])
	}
}

func TestProfilerPairFixedLedgerSinkPoisonTransitions(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
		t.Fatal(err)
	}
	rows := []renderedRow{
		{tsNS: 1, seq: 1, line: "lane-a-structured", pairKind: pairRenderF2FS,
			pairLane: "lane-a", structuredPair: true, profilerEventField: 4011},
		{tsNS: 2, seq: 2, line: "lane-a-end", pairKind: pairRenderF2FS,
			pairLane: "lane-a", pairTable: "f2fs_write_end"},
		{tsNS: 3, seq: 3, line: "lane-b-end", pairKind: pairRenderF2FS,
			pairLane: "lane-b", pairTable: "f2fs_write_end"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	sink.poisonPairLane(pairRenderF2FS, "lane-a")
	afterFirstPoison := sink.pairFixedLedger
	sink.poisonPairLane(pairRenderF2FS, "lane-a")
	if sink.pairFixedLedger != afterFirstPoison {
		t.Fatal("repeated exact-lane poison double-counted the fixed ledger")
	}
	if err := sink.add(renderedRow{
		tsNS: 4, seq: 4, line: "lane-a-after-poison", pairKind: pairRenderF2FS,
		pairLane: "lane-a", pairTable: "f2fs_write_begin",
	}); err != nil {
		t.Fatal(err)
	}
	family := sink.pairFixedLedger.families[pairRenderF2FS]
	begin := sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin]
	end := sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd]
	if family != (profilerPairFixedFamilyLedger{profilerPairFixedCounts: profilerPairFixedCounts{
		staged: 4, structured: 1, withheld: 3, structuredWithheld: 1,
	}}) || begin != (profilerPairFixedCounts{
		staged: 2, structured: 1, withheld: 2, structuredWithheld: 1,
	}) || end != (profilerPairFixedCounts{staged: 2, withheld: 1}) {
		t.Fatalf("lane poison transition drifted: family=%+v begin=%+v end=%+v", family, begin, end)
	}
	if state, ok := sink.pairLaneRegistries[pairRenderF2FS].state(2); !ok || state.poisoned {
		t.Fatalf("clean sibling lane was poisoned: state=%+v ok=%t", state, ok)
	}

	sink.poisonPairKind(pairRenderF2FS)
	if err := sink.add(renderedRow{
		tsNS: 5, seq: 5, line: "post-family", pairKind: pairRenderF2FS,
		pairLane: "lane-b", pairTable: "f2fs_write_end",
	}); err != nil {
		t.Fatal(err)
	}
	family = sink.pairFixedLedger.families[pairRenderF2FS]
	end = sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd]
	if family != (profilerPairFixedFamilyLedger{
		profilerPairFixedCounts: profilerPairFixedCounts{
			staged: 5, structured: 1, withheld: 5, structuredWithheld: 1,
		}, poisoned: true,
	}) || end != (profilerPairFixedCounts{staged: 3, withheld: 3}) ||
		len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 ||
		len(sink.pairLaneRows[pairRenderF2FS]) != 0 {
		t.Fatalf("family poison transition drifted: family=%+v end=%+v registry=%+v lanes=%v",
			family, end, sink.pairLaneRegistries[pairRenderF2FS], sink.pairLaneRows[pairRenderF2FS])
	}
	if err := sink.validateProfilerPairAccounting(); err != nil {
		t.Fatalf("fixed transition parity failed: %v", err)
	}
}

func TestProfilerPairFixedLedgerPreflightRejectsOverflowAtomically(t *testing.T) {
	t.Run("capture-wide counter", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
			t.Fatal(err)
		}
		sink.pairFixedLedger.families[pairRenderF2FS].staged = int(^uint(0) >> 1)
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin].staged = int(^uint(0) >> 1)
		before := sink.pairFixedLedger
		delta := traceDBProfilerEventDelta{}
		delta.poisonKinds[pairRenderMMC] = true
		err = sink.addProfilerEventContext(context.Background(), renderedRow{
			tsNS: 1, seq: 1, line: "overflow", pairKind: pairRenderF2FS,
			pairLane: "lane", pairTable: "f2fs_write_begin",
		}, delta)
		if reason := traceDBInvariantReason(err); reason != "profiler_pair_fixed_ledger_plan_invalid" {
			t.Fatalf("reason=%q err=%v", reason, err)
		}
		if sink.pairFixedLedger != before || sink.stats.RowsAccepted != 0 ||
			sink.nextIngestOrdinal != 0 || sink.profilerSourceProof.count != 0 ||
			sink.poisoned[pairRenderMMC] || len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 {
			t.Fatalf("fixed overflow partially committed: ledger=%+v stats=%+v ordinal=%d proof=%+v poisoned=%v registry=%+v",
				sink.pairFixedLedger, sink.stats, sink.nextIngestOrdinal, sink.profilerSourceProof,
				sink.poisoned, sink.pairLaneRegistries[pairRenderF2FS])
		}
	})

	t.Run("exact lane counter", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{
			tsNS: 1, seq: 1, line: "seed", pairKind: pairRenderF2FS,
			pairLane: "lane", pairTable: "f2fs_write_begin",
		}); err != nil {
			t.Fatal(err)
		}
		state := &sink.pairLaneRegistries[pairRenderF2FS].states[0]
		state.endpointCounts = [profilerPairFamilyEndpointCapacity]profilerPairLaneEndpointCounts{}
		ordinal, _ := profilerPairEndpointF2FSWriteBegin.familyOrdinal(pairRenderF2FS)
		state.endpointCounts[ordinal].rows = uint32(profilerPairBarrierMaxObservations)
		beforeLedger, beforeState := sink.pairFixedLedger, *state
		beforeRows, beforeProof := sink.stats.RowsAccepted, sink.profilerSourceProof.count
		err = sink.add(renderedRow{
			tsNS: 2, seq: 2, line: "overflow", pairKind: pairRenderF2FS,
			pairLane: "lane", pairTable: "f2fs_write_begin",
		})
		if reason := traceDBInvariantReason(err); reason != "profiler_pair_fixed_lane_plan_invalid" {
			t.Fatalf("reason=%q err=%v", reason, err)
		}
		if sink.pairFixedLedger != beforeLedger || *state != beforeState ||
			sink.stats.RowsAccepted != beforeRows || sink.profilerSourceProof.count != beforeProof {
			t.Fatalf("lane overflow partially committed: ledger=%+v state=%+v stats=%+v proof=%+v",
				sink.pairFixedLedger, *state, sink.stats, sink.profilerSourceProof)
		}
	})
}

func TestProfilerPairFixedLedgerParityRejectsSplitBrain(t *testing.T) {
	newFixture := func(t *testing.T) *traceDBRowSink {
		t.Helper()
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sink.cleanup() })
		if err := sink.add(renderedRow{
			tsNS: 1, seq: 1, line: "row", pairKind: pairRenderF2FS,
			pairLane: "lane", pairTable: "f2fs_write_begin",
		}); err != nil {
			t.Fatal(err)
		}
		return sink
	}
	for _, test := range []struct {
		name   string
		want   string
		mutate func(*traceDBRowSink)
	}{
		{name: "fixed withheld only", want: "profiler_pair_fixed_ledger_family_mismatch", mutate: func(sink *traceDBRowSink) {
			sink.pairFixedLedger.families[pairRenderF2FS].withheld = 1
			sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin].withheld = 1
		}},
		{name: "lane total", want: "profiler_pair_fixed_lane_total_mismatch", mutate: func(sink *traceDBRowSink) {
			sink.pairLaneRegistries[pairRenderF2FS].states[0].endpointCounts[4].rows++
		}},
		{name: "wrong endpoint", want: "profiler_pair_fixed_lane_endpoint_mismatch", mutate: func(sink *traceDBRowSink) {
			state := &sink.pairLaneRegistries[pairRenderF2FS].states[0]
			state.endpointCounts[4].rows = 0
			state.endpointCounts[5].rows = 1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink := newFixture(t)
			test.mutate(sink)
			if got := traceDBInvariantReason(sink.validateProfilerPairAccounting()); got != test.want {
				t.Fatalf("reason=%q want=%q", got, test.want)
			}
		})
	}
}
