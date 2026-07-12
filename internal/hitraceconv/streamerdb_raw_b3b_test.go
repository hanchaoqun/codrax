package hitraceconv

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func traceDBRawB3BSubjectAuthority() traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(10, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "old-process"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "new-process"}
	index.Processes[3] = traceDBProcess{IPID: 3, PID: 0, Name: "kernel"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 10, IPID: 1, Name: "old-thread"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 10, IPID: 2, Name: "new-thread"}
	index.ByITID[3] = traceDBThread{ITID: 3, TID: 77, IPID: 3, Name: "kworker"}
	index.ObservedPublicTID[55] = true
	index.RejectedPublicTID[56] = true
	buildTraceDBThreadSecondaryIndexes(&index)
	return newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		Lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
			10: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 2, NewIPID: 2}}},
		}},
		CreationComplete: true, TerminalComplete: true, ActivityComplete: true,
	})
}

func TestTraceDBRawB3BSubjectClosedSetAndLifecycle(t *testing.T) {
	authority := traceDBRawB3BSubjectAuthority()
	tests := []struct {
		name        string
		raw         traceDBRawEvent
		explicitCPU bool
		pairing     bool
		kind        traceDBRawSubjectKind
		tid         int64
		tgid        int64
		itid        int64
		reason      string
	}{
		{name: "old generation before cut", raw: traceDBRawEvent{TS: 99, ITID: 1, ITIDKnown: true}, kind: traceDBRawSubjectCanonicalThread, tid: 10, tgid: 100, itid: 1},
		{name: "old generation at cut", raw: traceDBRawEvent{TS: 100, ITID: 1, ITIDKnown: true}, reason: "lifecycle_rejected_subject"},
		{name: "new generation at cut", raw: traceDBRawEvent{TS: 100, ITID: 2, ITIDKnown: true}, kind: traceDBRawSubjectCanonicalThread, tid: 10, tgid: 200, itid: 2},
		{name: "user explicit tid zero conflicts", raw: traceDBRawEvent{TS: 99, ITID: 1, ITIDKnown: true, TID: 0, TIDKnown: true}, reason: "canonical_public_identity_conflict"},
		{name: "user explicit pid zero conflicts", raw: traceDBRawEvent{TS: 99, ITID: 1, ITIDKnown: true, PID: 0, PIDKnown: true}, reason: "canonical_public_identity_conflict"},
		{name: "kernel tid zero conflicts", raw: traceDBRawEvent{TS: 50, ITID: 3, ITIDKnown: true, TID: 0, TIDKnown: true, PID: 0, PIDKnown: true}, reason: "canonical_public_identity_conflict"},
		{name: "kernel exact tid and pid zero", raw: traceDBRawEvent{TS: 50, ITID: 3, ITIDKnown: true, TID: 77, TIDKnown: true, PID: 0, PIDKnown: true}, kind: traceDBRawSubjectKernelThread, tid: 77, tgid: 77, itid: 3},
		{name: "kernel positive pid conflict", raw: traceDBRawEvent{TS: 50, ITID: 3, ITIDKnown: true, PID: 100, PIDKnown: true}, reason: "canonical_public_identity_conflict"},
		{name: "idle exact zero claims", raw: traceDBRawEvent{TS: 50, ITID: 0, ITIDKnown: true, TID: 0, TIDKnown: true, PID: 0, PIDKnown: true}, kind: traceDBRawSubjectIdle, itid: 0},
		{name: "idle nonzero claim", raw: traceDBRawEvent{TS: 50, ITID: 0, ITIDKnown: true, TID: 1, TIDKnown: true}, reason: "idle_public_identity_conflict"},
		{name: "tid reuse remains ambiguous", raw: traceDBRawEvent{TS: 99, TID: 10, TIDKnown: true}, reason: "canonical_subject_ambiguous"},
		{name: "pid narrows reused tid", raw: traceDBRawEvent{TS: 100, TID: 10, TIDKnown: true, PID: 200, PIDKnown: true}, kind: traceDBRawSubjectCanonicalThread, tid: 10, tgid: 200, itid: 2},
		{name: "observed rejected candidate", raw: traceDBRawEvent{TS: 50, TID: 55, TIDKnown: true}, explicitCPU: true, reason: "rejected_public_tid_candidate"},
		{name: "explicit rejected candidate", raw: traceDBRawEvent{TS: 50, TID: 56, TIDKnown: true}, explicitCPU: true, reason: "rejected_public_tid_candidate"},
		{name: "source only inventory withheld", raw: traceDBRawEvent{TS: 50, TID: 90, TIDKnown: true, PID: 91, PIDKnown: true}, explicitCPU: true, reason: "source_only_inventory_withheld"},
		{name: "source only needs explicit cpu", raw: traceDBRawEvent{TS: 50, TID: 90, TIDKnown: true}, reason: "source_only_requires_explicit_cpu"},
		{name: "source only cannot pair", raw: traceDBRawEvent{TS: 50, TID: 90, TIDKnown: true}, explicitCPU: true, pairing: true, reason: "source_only_pairing_forbidden"},
		{name: "pid only", raw: traceDBRawEvent{TS: 50, PID: 91, PIDKnown: true}, explicitCPU: true, reason: "missing_thread_identity"},
		{name: "capture lower bound covers source only", raw: traceDBRawEvent{TS: 9, TID: 90, TIDKnown: true}, explicitCPU: true, reason: "pre_capture_timestamp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, reason := traceDBResolveRawSubject(test.raw, authority, test.explicitCPU, test.pairing)
			if reason != test.reason {
				t.Fatalf("reason=%q subject=%+v, want %q", reason, subject, test.reason)
			}
			if reason == "" && (subject.Kind != test.kind || subject.TID != test.tid || subject.TGID != test.tgid || subject.ITID != test.itid) {
				t.Fatalf("subject=%+v, want kind=%v tid=%d tgid=%d itid=%d", subject, test.kind, test.tid, test.tgid, test.itid)
			}
		})
	}

	poisoned := authority
	poisoned.lifecycle.GlobalPoison = []int64{50}
	if _, reason := traceDBResolveRawSubject(traceDBRawEvent{TS: 50, ITID: 0, ITIDKnown: true}, poisoned, true, false); reason != "idle_lifecycle_rejected" {
		t.Fatalf("idle global poison reason=%q", reason)
	}
	conflictingIdle := authority
	conflictingIdle.identities.AmbiguousITID[0] = true
	if _, reason := traceDBResolveRawSubject(traceDBRawEvent{TS: 50, ITID: 0, ITIDKnown: true}, conflictingIdle, true, false); reason != "idle_lifecycle_rejected" {
		t.Fatalf("conflicting idle reason=%q", reason)
	}
	for name, lifecycle := range map[string]traceDBLifecycleIndex{
		"thread point poison":  {ByTID: map[int64]traceDBLifecycleLane{10: {PoisonPoints: []int64{99}}}},
		"process point poison": {ByPID: map[int64]traceDBLifecycleLane{100: {PoisonPoints: []int64{99}}}},
		"global point poison":  {GlobalPoison: []int64{99}},
	} {
		poisoned := authority
		poisoned.lifecycle = lifecycle
		if _, reason := traceDBResolveRawSubject(traceDBRawEvent{TS: 99, ITID: 1, ITIDKnown: true}, poisoned, true, false); reason != "lifecycle_rejected_subject" {
			t.Fatalf("%s reason=%q", name, reason)
		}
	}
	incomplete := authority
	incomplete.complete = false
	if _, reason := traceDBResolveRawSubject(traceDBRawEvent{TS: 99, ITID: 1, ITIDKnown: true}, incomplete, true, false); reason != "lifecycle_rejected_subject" {
		t.Fatalf("incomplete authority reason=%q", reason)
	}
}

func exportTraceDBRawB3BFixture(t *testing.T, statements []string, authority traceDBSchedulerAuthority,
	running traceDBSchedulerRunningIndex,
) ([]TraceDBCoverage, string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBRawFtraceFamilies(context.Background(), tdb, sink, authority, running,
		filepath.Join(t.TempDir(), "raw-b3b.ftrace"))
	if err != nil {
		t.Fatalf("export raw B3-b fixture: %v", err)
	}
	rows := append([]renderedRow(nil), sink.rows...)
	sortRenderedRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return coverage, strings.Join(lines, "\n")
}

func TestTraceDBRawB3BTypedRunningStatusesAndExplicitCPU0(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	for id := int64(1); id <= 4; id++ {
		index.Processes[id] = traceDBProcess{IPID: id, PID: 100 + id}
		index.ByITID[id] = traceDBThread{ITID: id, TID: 10 + id, IPID: id, Name: "worker"}
	}
	buildTraceDBThreadSecondaryIndexes(&index)
	authority := traceDBTestCompleteSchedulerAuthority(index)
	running := traceDBSchedulerRunningIndex{
		intervals: map[int64][]traceDBRunningInterval{
			1: {{Start: 0, End: 2000, CPU: 7, PrefixMaxEnd: 2000}},
		},
		sourceTaintedITID:     map[int64]bool{2: true},
		lifecycleRejectedITID: map[int64]bool{3: true},
		rejectedITID:          map[int64]bool{2: true, 3: true},
		initialized:           true,
	}
	coverage, body := exportTraceDBRawB3BFixture(t, []string{
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 1)",
		"INSERT INTO args VALUES (2, 1, 0, 2)",
		"INSERT INTO args VALUES (3, 1, 0, 3)",
		"INSERT INTO args VALUES (4, 1, 0, 4)",
		"INSERT INTO args VALUES (5, 1, 0, 5)",
		"INSERT INTO args VALUES (6, 1, 0, 6)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'workqueue_execute_start', 0, 2, 1)",
		"INSERT INTO raw VALUES (2, 1001, 'workqueue_execute_start', NULL, 1, 2)",
		"INSERT INTO raw VALUES (3, 1002, 'workqueue_execute_start', NULL, 2, 3)",
		"INSERT INTO raw VALUES (4, 1003, 'workqueue_execute_start', NULL, 3, 4)",
		"INSERT INTO raw VALUES (5, 1004, 'workqueue_execute_start', NULL, 4, 5)",
		"INSERT INTO raw VALUES (6, 1005, 'workqueue_execute_start', 4096, 1, 6)",
	}, authority, running)
	if strings.Count(body, "workqueue_execute_start:") != 2 || !strings.Contains(body, "[000]") || !strings.Contains(body, "[007]") {
		t.Fatalf("explicit CPU0 or typed Running known row lost:\n%s", body)
	}
	for _, reason := range []string{
		"tainted_running_cpu_witness=1",
		"lifecycle_rejected_running_cpu_witness=1",
		"unknown_running_cpu_witness=1",
		"invalid_cpu=1",
	} {
		if !coverageHasSkipped(coverage, "raw_ftrace", "workqueue", reason) {
			t.Fatalf("typed Running coverage missing %q: %+v", reason, coverage)
		}
	}
}

func TestTraceDBRawB3BSourceOnlyIsCoverageOnlyWithoutForgingStandardWire(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.ObservedPublicTID[61] = true
	authority := traceDBTestCompleteSchedulerAuthority(index)
	coverage, body := exportTraceDBRawB3BFixture(t, []string{
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 's_dev')",
		"INSERT INTO data_dict VALUES (2, 'i_ino')",
		"INSERT INTO data_dict VALUES (3, 'index')",
		"INSERT INTO data_dict VALUES (4, 'work')",
		"INSERT INTO data_dict VALUES (5, 'function')",
		"INSERT INTO data_dict VALUES (6, 'pfn')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 272629844)",
		"INSERT INTO args VALUES (1, 2, 0, 123)",
		"INSERT INTO args VALUES (1, 3, 0, 0)",
		"INSERT INTO args VALUES (1, 6, 0, 101)",
		"INSERT INTO args VALUES (2, 4, 0, 1)",
		"INSERT INTO args VALUES (2, 5, 0, 2)",
		"CREATE TABLE raw (id, ts, name, cpu, tid, pid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'mm_filemap_add_to_page_cache', 2, 60, 50, 1)",
		"INSERT INTO raw VALUES (2, 1001, 'workqueue_execute_start', 2, 60, 50, 2)",
		"INSERT INTO raw VALUES (3, 1002, 'mm_filemap_add_to_page_cache', 2, 61, 50, 1)",
		"INSERT INTO raw VALUES (4, 1003, 'mm_filemap_add_to_page_cache', 4096, 62, 50, 1)",
	}, authority, traceDBSchedulerRunningIndex{initialized: true})
	if body != "" {
		t.Fatalf("source-only rows repurposed a standard ftrace event instead of remaining coverage-only:\n%s", body)
	}
	if !coverageHasSkipped(coverage, "raw_ftrace", "workqueue", "source_only_pairing_forbidden=1") ||
		!coverageHasSkipped(coverage, "raw_ftrace", "page_cache", "source_only_inventory_withheld=1") ||
		!coverageHasSkipped(coverage, "raw_ftrace", "page_cache", "rejected_public_tid_candidate=1") ||
		!coverageHasSkipped(coverage, "raw_ftrace", "page_cache", "invalid_cpu=1") {
		t.Fatalf("source-only rejection coverage incomplete: %+v", coverage)
	}
}

func TestTraceDBRawB3BExplicitPublicZeroClaimsRemainPresent(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 101}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 0}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 11, IPID: 1, Name: "user"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 77, IPID: 2, Name: "kernel"}
	buildTraceDBThreadSecondaryIndexes(&index)
	authority := traceDBTestCompleteSchedulerAuthority(index)
	coverage, body := exportTraceDBRawB3BFixture(t, []string{
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 's_dev')",
		"INSERT INTO data_dict VALUES (2, 'i_ino')",
		"INSERT INTO data_dict VALUES (3, 'index')",
		"INSERT INTO data_dict VALUES (4, 'pfn')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 272629844)",
		"INSERT INTO args VALUES (1, 2, 0, 123)",
		"INSERT INTO args VALUES (1, 3, 0, 0)",
		"INSERT INTO args VALUES (1, 4, 0, 101)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, tid, pid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000, 'mm_filemap_add_to_page_cache', 2, 1, 0, 101, 1)",
		"INSERT INTO raw VALUES (2, 1001, 'mm_filemap_add_to_page_cache', 2, 1, 11, 0, 1)",
		"INSERT INTO raw VALUES (3, 1002, 'mm_filemap_add_to_page_cache', 2, 1, 11, 101, 1)",
		"INSERT INTO raw VALUES (4, 1003, 'mm_filemap_add_to_page_cache', 2, 2, 0, 0, 1)",
		"INSERT INTO raw VALUES (5, 1004, 'mm_filemap_add_to_page_cache', 2, 2, 77, 0, 1)",
	}, authority, traceDBSchedulerRunningIndex{initialized: true})
	if strings.Count(body, "mm_filemap_add_to_page_cache:") != 2 ||
		!strings.Contains(body, "user-11") || !strings.Contains(body, "kernel-77") {
		t.Fatalf("explicit public zero claims were erased or valid user/kernel rows lost:\n%s", body)
	}
	if !coverageHasSkipped(coverage, "raw_ftrace", "page_cache", "canonical_public_identity_conflict=3") {
		t.Fatalf("explicit user zero conflicts were not disclosed: %+v", coverage)
	}
}

func TestTraceDBRawB3BSharedAuthoritiesAreStructurallyPinned(t *testing.T) {
	t.Helper()
	isIdentifier := func(expression ast.Expr, name string) bool {
		identifier, ok := expression.(*ast.Ident)
		return ok && identifier.Name == name
	}
	isSelector := func(expression ast.Expr, receiver, name string) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return false
		}
		identifier, ok := selector.X.(*ast.Ident)
		return ok && identifier.Name == receiver
	}
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(current), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	type callSite struct {
		function string
		call     *ast.CallExpr
	}
	calls := map[string][]callSite{}
	functions := map[string]int{}
	rawSourceQueries, rawSourceQueryOrders := 0, 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functions[function.Name.Name]++
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if literal, ok := node.(*ast.BasicLit); ok && function.Name.Name == "exportTraceDBRawFtraceFamilies" {
					upper := strings.ToUpper(literal.Value)
					if strings.Contains(upper, "SELECT") && strings.Contains(upper, "FROM RAW") {
						rawSourceQueries++
						if strings.Contains(upper, "ORDER BY") {
							rawSourceQueryOrders++
						}
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch callee := call.Fun.(type) {
				case *ast.Ident:
					name = callee.Name
				case *ast.SelectorExpr:
					name = callee.Sel.Name
				}
				if name != "" {
					calls[name] = append(calls[name], callSite{function: function.Name.Name, call: call})
				}
				return true
			})
		}
	}
	if rawSourceQueries != 1 || rawSourceQueryOrders != 0 {
		t.Fatalf("raw pass-1 source query count=%d ORDER BY count=%d; ordering must remain in the bounded indexed private stage", rawSourceQueries, rawSourceQueryOrders)
	}

	rawDispatches := calls["exportTraceDBRawFtraceFamilies"]
	if len(rawDispatches) != 1 || rawDispatches[0].function != "exportTraceDBExtendedFamilies" {
		t.Fatalf("raw production dispatches=%+v", rawDispatches)
	}
	dispatch := rawDispatches[0].call
	if len(dispatch.Args) != 6 || !isIdentifier(dispatch.Args[0], "ctx") || !isIdentifier(dispatch.Args[1], "tdb") ||
		!isIdentifier(dispatch.Args[2], "sink") || !isIdentifier(dispatch.Args[3], "authority") || !isIdentifier(dispatch.Args[4], "lifecycleRunning") ||
		!isSelector(dispatch.Args[5], "syncSpans", "artifactSource") {
		t.Fatal("raw dispatch does not consume the shared scheduler authority and typed Running value")
	}
	if functions["traceDBExtendedRunningCPUAt"] != 0 || len(calls["traceDBExtendedRunningCPUAt"]) != 0 {
		t.Fatalf("retired legacy raw Running authority remains: declarations=%d calls=%+v",
			functions["traceDBExtendedRunningCPUAt"], calls["traceDBExtendedRunningCPUAt"])
	}
	for _, forbidden := range []string{"loadThreadIndex", "collectTraceDBLifecycle", "loadRunningIntervals", "loadSchedulerRunningIndex", "loadExtendedLegacyRunningIntervals"} {
		for _, site := range calls[forbidden] {
			if site.function == "exportTraceDBRawFtraceFamilies" || site.function == "traceDBResolveRawSubject" || site.function == "traceDBAdmitRawCanonicalSubject" {
				t.Fatalf("raw B3-b authority reloaded through %s in %s", forbidden, site.function)
			}
		}
	}
	if len(calls["traceDBResolveRawSubject"]) != 1 || calls["traceDBResolveRawSubject"][0].function != "exportTraceDBRawFtraceFamilies" {
		t.Fatalf("raw point-admission closure resolve=%+v", calls["traceDBResolveRawSubject"])
	}
	resolvePos := calls["traceDBResolveRawSubject"][0].call.Pos()
	lookupPos := token.NoPos
	addPos := token.NoPos
	sealPos := token.NoPos
	publishPos := token.NoPos
	for _, site := range calls["lookupCPUAt"] {
		if site.function == "exportTraceDBRawFtraceFamilies" {
			lookupPos = site.call.Pos()
		}
	}
	for _, site := range calls["add"] {
		if site.function == "exportTraceDBRawFtraceFamilies" {
			if addPos != token.NoPos {
				t.Fatal("raw exporter regained a second typed-stage submitter")
			}
			addPos = site.call.Pos()
		}
	}
	for _, site := range calls["seal"] {
		if site.function == "exportTraceDBRawFtraceFamilies" {
			sealPos = site.call.Pos()
		}
	}
	for _, site := range calls["publish"] {
		if site.function == "exportTraceDBRawFtraceFamilies" {
			publishPos = site.call.Pos()
		}
	}
	if resolvePos == token.NoPos || lookupPos == token.NoPos || addPos == token.NoPos || sealPos == token.NoPos || publishPos == token.NoPos ||
		!(resolvePos < lookupPos && lookupPos < addPos && addPos < sealPos && sealPos < publishPos) {
		t.Fatalf("raw lifecycle/CPU/freeze order resolve=%d lookup=%d add=%d seal=%d publish=%d", resolvePos, lookupPos, addPos, sealPos, publishPos)
	}
	for _, site := range calls["addTraceDBInstantRow"] {
		if site.function == "exportTraceDBRawFtraceFamilies" {
			t.Fatal("raw pass 1 regained a direct sink publisher")
		}
	}
	if sites := calls["traceDBPublishFrozenRawRecord"]; len(sites) != 1 || sites[0].function != "publish" {
		t.Fatalf("frozen raw publisher closure=%+v", sites)
	}
	if sites := calls["FingerprintPairingEndpoint"]; len(sites) != 1 || sites[0].function != "fingerprintPairingEndpoint" {
		t.Fatalf("pairing adapters regained a second endpoint fingerprint authority: %+v", sites)
	}
	if sites := calls["LaneKey"]; len(sites) != 1 || sites[0].function != "pairingEndpointLaneKey" {
		t.Fatalf("pairing adapters regained a second lane namespace authority: %+v", sites)
	}
	pointCalls := 0
	for _, site := range calls["threadPointAllows"] {
		if site.function == "traceDBAdmitRawCanonicalSubject" {
			pointCalls++
		}
	}
	idlePointCalls := 0
	for _, site := range calls["schedulerPointAllows"] {
		if site.function == "traceDBResolveRawSubject" {
			idlePointCalls++
		}
	}
	if pointCalls != 1 || idlePointCalls != 1 {
		t.Fatalf("raw canonical/idle point gates=%d/%d", pointCalls, idlePointCalls)
	}
}
