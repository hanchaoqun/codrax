package hitraceconv

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTraceDBIdentityCurrentAliasAndInternalIDBoundaries(t *testing.T) {
	maxID := maxTraceDBInternalID
	sentinel := maxID + 1
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (0, 0, 0, 'swapper')",
		"INSERT INTO process VALUES (4294967294, 4294967294, 123, 'max-process')",
		"INSERT INTO process VALUES (4294967295, 4294967295, 124, 'sentinel-process')",
		"INSERT INTO process VALUES (CAST(1 AS TEXT), CAST(1 AS TEXT), 125, 'text-process')",
		"INSERT INTO process VALUES (2.0, 2.0, 126, 'real-process')",
		"INSERT INTO process VALUES (NULL, 3, 127, 'null-source-process')",
		"INSERT INTO process VALUES (x'04', 4, 128, 'blob-source-process')",
		"INSERT INTO process VALUES (5, 5, NULL, 'null-pid-process')",
		"INSERT INTO process VALUES (6, 6, x'31', 'blob-pid-process')",
		"INSERT INTO process VALUES (12, 12, 2147483647, 'max-pid-process')",
		"INSERT INTO process VALUES (13, 13, 2147483648, 'overflow-pid-process')",
		"INSERT INTO process VALUES (14, 14, 140, 'switch-boundary-process')",
		"INSERT INTO process VALUES (-1, -1, 129, 'negative-process')",
		"INSERT INTO process VALUES (4294967296, 4294967296, 130, 'overflow-process')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (0, 0, 0, 0, 'swapper', 0, 1, 0)",
		"INSERT INTO thread VALUES (4294967294, 4294967294, 456, 4294967294, 'max-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (4294967295, 4294967295, 457, 4294967294, 'sentinel-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (CAST(1 AS TEXT), CAST(1 AS TEXT), 458, 0, 'text-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (2.0, 2.0, 459, 0, 'real-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (NULL, 3, 3, 0, 'null-source-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (x'04', 4, 4, 0, 'blob-source-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (5, 5, NULL, 0, 'null-tid-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (6, 6, 6, NULL, 'null-owner-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (7, 7, 7, 0, 'null-start-thread', NULL, 0, 1)",
		"INSERT INTO thread VALUES (8, 8, 8, 0, 'null-main-thread', 0, NULL, 1)",
		"INSERT INTO thread VALUES (9, 9, 9, 0, 'null-switch-thread', 0, 0, NULL)",
		"INSERT INTO thread VALUES (10, 10, 10, 0, 'invalid-main-thread', 0, 2, 1)",
		"INSERT INTO thread VALUES (12, 12, 2147483647, 12, 'max-tid-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (13, 13, 2147483648, 12, 'overflow-tid-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (14, 14, 140, 14, 'max-switch-thread', 0, 0, 4294967295)",
		"INSERT INTO thread VALUES (15, 15, 150, 14, 'overflow-switch-thread', 0, 0, 4294967296)",
		"INSERT INTO thread VALUES (-1, -1, 11, 0, 'negative-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (4294967296, 4294967296, 12, 0, 'overflow-thread', 0, 0, 1)",
	})
	index, coverage := loadTraceDBIdentityFixture(t, path)
	processZero, processZeroOK := index.Processes[0]
	threadZero, threadZeroOK := index.ByITID[0]
	processZeroMap, processZeroMapOK := index.ProcessIDToIPID[0]
	threadZeroMap, threadZeroMapOK := index.ThreadIDToITID[0]
	if !processZeroOK || !threadZeroOK || !processZeroMapOK || !threadZeroMapOK ||
		processZeroMap != 0 || threadZeroMap != 0 ||
		index.ProcessIDToIPID[maxID] != maxID || index.ThreadIDToITID[maxID] != maxID {
		t.Fatalf("valid internal/source ID boundary lost: %+v", index)
	}
	if processZero.PID != 0 || threadZero.TID != 0 ||
		index.Processes[maxID].PID != 123 || index.ByITID[maxID].TID != 456 ||
		index.Processes[12].PID != 2147483647 || index.ByITID[12].TID != 2147483647 ||
		index.ByITID[14].SwitchCount != 4294967295 {
		t.Fatalf("canonical boundary identities lost: %+v", index)
	}
	if itid, callID, reason := traceDBResolveCallstackEmitterIdentity(index, false, true, nil, int64(0)); reason != "" || itid != 0 || callID != 0 {
		t.Fatalf("valid callstack internal ID zero was treated as missing: itid=%d callid=%d reason=%q", itid, callID, reason)
	}
	if _, ok := index.ProcessIDToIPID[sentinel]; ok {
		t.Fatalf("UINT32_MAX process sentinel entered source map: %+v", index.ProcessIDToIPID)
	}
	if _, ok := index.ThreadIDToITID[sentinel]; ok {
		t.Fatalf("UINT32_MAX thread sentinel entered source map: %+v", index.ThreadIDToITID)
	}
	if _, ok := index.Processes[1]; ok {
		t.Fatalf("numeric TEXT process identity was coerced: %+v", index.Processes[1])
	}
	if _, ok := index.ByITID[2]; ok {
		t.Fatalf("integral REAL thread identity was coerced: %+v", index.ByITID[2])
	}
	for _, ipid := range []int64{3, 4, 5, 6, 13} {
		if _, ok := index.Processes[ipid]; ok || !index.AmbiguousIPID[ipid] {
			t.Fatalf("malformed process cohort %d survived strict audit: %+v", ipid, index)
		}
	}
	if thread, ok := index.ByITID[7]; !ok || thread.RegistrationHint.Known || thread.RegistrationHint.Tainted {
		t.Fatalf("NULL thread.start_ts changed canonical identity or became a hard timestamp: %+v", index)
	}
	for _, itid := range []int64{3, 4, 5, 6, 8, 9, 10, 13, 15} {
		if _, ok := index.ByITID[itid]; ok || !index.AmbiguousITID[itid] {
			t.Fatalf("malformed thread cohort %d survived strict audit: %+v", itid, index)
		}
	}
	if _, _, reason := traceDBResolveCallstackEmitterIdentity(index, false, true, nil, sentinel); reason != "invalid_callid" {
		t.Fatalf("UINT32_MAX callid sentinel reason=%q, want invalid_callid", reason)
	}
	if len(coverage) != 3 || !strings.Contains(coverage[1].Skipped, "process row(s) rejected") ||
		!strings.Contains(coverage[2].Skipped, "thread row(s) rejected") {
		t.Fatalf("strict identity rejections missing from coverage: %+v", coverage)
	}
}

func TestTraceDBIdentityRejectsDivergentCurrentThreadAliasOrthogonally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (7, 7, 500, 'demo')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (41, 9, 501, 7, 'bad-by-source', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 502, 7, 'good', 0, 0, 1)",
	})
	index, _ := loadTraceDBIdentityFixture(t, path)
	if index.Processes[7].PID != 500 {
		t.Fatalf("orthogonal process alias was not valid: %+v", index.Processes)
	}
	if _, ok := index.ByITID[9]; ok || !index.AmbiguousITID[9] || !index.AmbiguousThreadID[41] {
		t.Fatalf("divergent thread aliases survived: %+v", index)
	}
	if _, ok := index.ByITID[10]; !ok {
		t.Fatalf("valid thread sibling was lost: %+v", index)
	}
	assertTraceDBThreadAbsentFromSecondaryIndexes(t, index, 9)
}

func TestTraceDBIdentityRejectsDivergentCurrentProcessAliasOrthogonally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (31, 7, 500, 'ambiguous')",
		"INSERT INTO process VALUES (8, 8, 600, 'control')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (9, 9, 501, 31, 'source-interpretation', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 502, 7, 'canonical-interpretation', 0, 0, 1)",
		"INSERT INTO thread VALUES (11, 11, 601, 8, 'control', 0, 0, 1)",
	})
	index, _ := loadTraceDBIdentityFixture(t, path)
	if _, ok := index.Processes[7]; ok || !index.AmbiguousIPID[7] || !index.AmbiguousProcessID[31] {
		t.Fatalf("divergent process aliases survived: %+v", index)
	}
	for _, itid := range []int64{9, 10} {
		if _, ok := index.ByITID[itid]; ok || !index.AmbiguousITID[itid] {
			t.Fatalf("thread %d survived unresolved process namespace: %+v", itid, index)
		}
		assertTraceDBThreadAbsentFromSecondaryIndexes(t, index, itid)
	}
	if control, ok := index.ByITID[11]; !ok || control.IPID != 8 || index.Processes[8].PID != 600 {
		t.Fatalf("valid process/thread sibling was lost: %+v", index)
	}
}

func TestTraceDBIdentityNamesAreOptionalDisplayMetadata(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid)",
		"INSERT INTO process VALUES (1, 1, 100)",
		"CREATE TABLE thread (id, itid, tid, ipid, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 2, 101, 1, 0, 0, 1)",
	})
	index, coverage := loadTraceDBIdentityFixture(t, path)
	if index.Processes[1].PID != 100 || index.ByITID[2].TID != 101 ||
		index.Processes[1].Name != "" || index.ByITID[2].Name != "" {
		t.Fatalf("missing display names changed hard identity: index=%+v coverage=%+v", index, coverage)
	}
	if containsString(coverage[1].ColumnsMissing, "name") || containsString(coverage[2].ColumnsMissing, "name") {
		t.Fatalf("display-only name was advertised as a hard missing column: %+v", coverage)
	}
}

func TestTraceDBLifetimeMetadataNeverPoisonsOrSelectsCanonicalIdentity(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantKnown   bool
		wantTainted bool
		wantValue   int64
	}{
		{name: "null", value: "NULL"},
		{name: "zero", value: "0", wantKnown: true},
		{name: "positive", value: "123", wantKnown: true, wantValue: 123},
		{name: "numeric text", value: "CAST(123 AS TEXT)", wantTainted: true},
		{name: "integral real", value: "123.0", wantTainted: true},
		{name: "blob", value: "x'313233'", wantTainted: true},
		{name: "negative", value: "-1", wantTainted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := createTraceDBFixture(t, []string{
				"CREATE TABLE trace_range (start_ts)",
				"INSERT INTO trace_range VALUES (10)",
				"CREATE TABLE process (id, ipid, pid, name, start_ts)",
				"INSERT INTO process VALUES (1, 1, 100, 'process', " + test.value + ")",
				"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, end_ts, is_main_thread, switch_count)",
				"INSERT INTO thread VALUES (2, 2, 101, 1, 'thread', " + test.value + ", " + test.value + ", 0, 1)",
			})
			index, coverage := loadTraceDBIdentityFixture(t, path)
			process, processOK := index.Processes[1]
			thread, threadOK := index.ByITID[2]
			if !processOK || !threadOK || index.AmbiguousIPID[1] || index.AmbiguousITID[2] {
				t.Fatalf("lifetime metadata changed canonical identity: index=%+v coverage=%+v", index, coverage)
			}
			for label, metadata := range map[string]traceDBTimestampMetadata{
				"process.start": process.RegistrationHint,
				"thread.start":  thread.RegistrationHint,
				"thread.end":    thread.ObservedEndHint,
			} {
				if metadata.Known != test.wantKnown || metadata.Tainted != test.wantTainted || metadata.Value != test.wantValue {
					t.Fatalf("%s metadata=%+v, want known=%t tainted=%t value=%d", label, metadata, test.wantKnown, test.wantTainted, test.wantValue)
				}
			}
			if test.wantTainted && (!strings.Contains(coverage[1].Skipped, "metadata ignored for hard identity") ||
				!strings.Contains(coverage[2].Skipped, "metadata ignored for hard identity")) {
				t.Fatalf("tainted metadata was not disclosed: %+v", coverage)
			}
		})
	}
}

func TestTraceDBLifetimeMetadataColumnsAreOptionalAndConflictsAreOrderIndependent(t *testing.T) {
	load := func(t *testing.T, reverse bool) (traceDBThreadIndex, []TraceDBCoverage) {
		t.Helper()
		rows := []string{
			"INSERT INTO thread VALUES (2, 101, 1, 'first', 10, 20, 0, 1)",
			"INSERT INTO thread VALUES (2, 101, 1, 'second', NULL, 30, 0, 2)",
		}
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 100, 'process')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, end_ts, is_main_thread, switch_count)",
		}
		statements = append(statements, orderedTraceDBStatements(rows, reverse)...)
		return loadTraceDBIdentityFixture(t, createTraceDBFixture(t, statements))
	}
	forward, forwardCoverage := load(t, false)
	reverse, reverseCoverage := load(t, true)
	if !reflect.DeepEqual(forward, reverse) || forwardCoverage[2].Skipped != reverseCoverage[2].Skipped {
		t.Fatalf("metadata conflict depends on row order:\nforward=%+v %+v\nreverse=%+v %+v", forward, forwardCoverage, reverse, reverseCoverage)
	}
	thread, ok := forward.ByITID[2]
	if !ok || !thread.RegistrationHint.Tainted || !thread.ObservedEndHint.Tainted || forward.AmbiguousITID[2] {
		t.Fatalf("metadata conflict poisoned identity or escaped tri-state taint: %+v", forward)
	}

	missingPath := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid)", "INSERT INTO process VALUES (1, 100)",
		"CREATE TABLE thread (itid, tid, ipid, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 101, 1, 0, 1)",
	})
	missing, coverage := loadTraceDBIdentityFixture(t, missingPath)
	if _, ok := missing.ByITID[2]; !ok || containsString(coverage[2].ColumnsMissing, "start_ts") || containsString(coverage[2].ColumnsMissing, "end_ts") {
		t.Fatalf("optional lifetime columns became hard schema requirements: index=%+v coverage=%+v", missing, coverage)
	}
}

func TestTraceDBLifetimeMetadataHasNoHardConsumer(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(currentFile), "streamerdb_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	allowedRegistrationSelector := map[string]bool{
		"loadStrictProcessIndex":           true,
		"loadStrictThreadIndex":            true,
		"exportTraceDBThreadRegistrations": true,
		"sortedTraceDBThreads":             true,
	}
	allowedObservedEndSelector := map[string]bool{
		"loadStrictThreadIndex": true,
	}
	allowedRegistrationHelperCall := map[string]bool{
		"exportTraceDBThreadRegistrations": true,
		"sortedTraceDBThreads":             true,
	}
	allowedTraceStartSelector := map[string]bool{
		"exportTraceDBThreadRegistrations": true,
		"traceDBBeforeCaptureStart":        true,
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		name := filepath.Base(path)
		for _, retired := range []string{".StartTS", "ByTIDIncarnation"} {
			if strings.Contains(text, retired) {
				t.Fatalf("retired start-based generation authority %q returned in %s", retired, name)
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functionName := function.Name.Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch item := node.(type) {
				case *ast.SelectorExpr:
					switch item.Sel.Name {
					case "RegistrationHint":
						if !allowedRegistrationSelector[functionName] {
							t.Fatalf("registration-only timestamp acquired a consumer in %s.%s", name, functionName)
						}
					case "ObservedEndHint":
						if !allowedObservedEndSelector[functionName] {
							t.Fatalf("non-authoritative end hint acquired a consumer in %s.%s", name, functionName)
						}
					case "TraceStart", "TraceStartKnown":
						if !allowedTraceStartSelector[functionName] {
							t.Fatalf("capture-start provenance bypassed its single authority in %s.%s", name, functionName)
						}
					}
				case *ast.CallExpr:
					identifier, ok := item.Fun.(*ast.Ident)
					if ok && identifier.Name == "traceDBRegistrationTimestamp" && !allowedRegistrationHelperCall[functionName] {
						t.Fatalf("registration helper acquired a non-display caller in %s.%s", name, functionName)
					}
				}
				return true
			})
		}
	}
}

func TestTraceDBIdentitySchemaDiscoveryUsesSQLiteASCIICaseFolding(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE TRACE_RANGE (START_TS)",
		"INSERT INTO TRACE_RANGE VALUES (0)",
		"CREATE TABLE PROCESS (ID, IPID, PID, NAME)",
		"INSERT INTO PROCESS VALUES (7, 7, 500, 'demo')",
		"CREATE TABLE THREAD (ID, ITID, TID, IPID, NAME, START_TS, IS_MAIN_THREAD, SWITCH_COUNT)",
		"INSERT INTO THREAD VALUES (41, 9, 501, 7, 'divergent', 0, 0, 1)",
		"INSERT INTO THREAD VALUES (10, 10, 502, 7, 'control', 0, 0, 1)",
	})
	index, coverage := loadTraceDBIdentityFixture(t, path)
	if !index.HasProcessIDColumn || !index.HasThreadIDColumn || index.TraceStart != 0 || !index.TraceStartKnown {
		t.Fatalf("SQLite ASCII-insensitive schema profile was missed: index=%+v coverage=%+v", index, coverage)
	}
	if _, ok := index.ByITID[9]; ok || !index.AmbiguousITID[9] || !index.AmbiguousThreadID[41] {
		t.Fatalf("uppercase ID column fell back to legacy profile: %+v", index)
	}
	if control, ok := index.ByITID[10]; !ok || control.TID != 502 {
		t.Fatalf("valid uppercase-schema sibling was lost: %+v", index)
	}
}

func TestTraceDBIdentityCurrentProfileNeverFallsBackWhenHardSchemaIncomplete(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, name)",
		"INSERT INTO process VALUES (1, 1, 'missing-pid')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 2, 101, 1, 'must-not-fallback', 0, 0, 1)",
	})
	index, coverage := loadTraceDBIdentityFixture(t, path)
	if !index.HasProcessIDColumn || !containsString(coverage[1].ColumnsMissing, "pid") {
		t.Fatalf("current process profile was not retained across incomplete schema: index=%+v coverage=%+v", index, coverage)
	}
	if _, ok := index.ByITID[2]; ok || !index.AmbiguousITID[2] {
		t.Fatalf("thread.ipid fell back to legacy canonical interpretation: %+v", index)
	}
}

func TestTraceDBIdentityPoisonIsOrderIndependent(t *testing.T) {
	processRows := []string{
		"INSERT INTO process VALUES (1, 100, 'zeta-process')",
		"INSERT INTO process VALUES (1, 100, 'alpha-process')",
		"INSERT INTO process VALUES (2, 200, 'conflict-a')",
		"INSERT INTO process VALUES (2, 201, 'conflict-b')",
		"INSERT INTO process VALUES (3, 300, 'valid-before-malformed')",
		"INSERT INTO process VALUES (3, CAST(300 AS TEXT), 'malformed-sibling')",
	}
	threadRows := []string{
		"INSERT INTO thread VALUES (10, 101, 1, 'zeta-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 101, 1, 'alpha-thread', 0, 1, 3)",
		"INSERT INTO thread VALUES (20, 201, 1, 'conflict-a', 0, 0, 1)",
		"INSERT INTO thread VALUES (20, 202, 1, 'conflict-b', 0, 0, 1)",
		"INSERT INTO thread VALUES (30, 301, 1, 'valid-before-malformed', 0, 0, 1)",
		"INSERT INTO thread VALUES (30, CAST(301 AS TEXT), 1, 'malformed-sibling', 0, 0, 1)",
	}
	load := func(t *testing.T, reverse bool) (traceDBThreadIndex, []TraceDBCoverage) {
		t.Helper()
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
		}
		statements = append(statements, orderedTraceDBStatements(processRows, reverse)...)
		statements = append(statements, "CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)")
		statements = append(statements, orderedTraceDBStatements(threadRows, reverse)...)
		return loadTraceDBIdentityFixture(t, createTraceDBFixture(t, statements))
	}
	forward, forwardCoverage := load(t, false)
	reverse, reverseCoverage := load(t, true)
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("identity poison depends on row order:\nforward=%+v\nreverse=%+v", forward, reverse)
	}
	if forwardCoverage[1].Skipped != reverseCoverage[1].Skipped || forwardCoverage[2].Skipped != reverseCoverage[2].Skipped {
		t.Fatalf("identity coverage depends on row order:\nforward=%+v\nreverse=%+v", forwardCoverage, reverseCoverage)
	}
	if forward.Processes[1].Name != "zeta-process" || forward.ByITID[10].Name != "zeta-thread" ||
		!forward.ByITID[10].IsMainThread || forward.ByITID[10].SwitchCount != 3 {
		t.Fatalf("display rename/soft metadata split a hard identity: %+v", forward)
	}
	for _, ipid := range []int64{2, 3} {
		if _, ok := forward.Processes[ipid]; ok || !forward.AmbiguousIPID[ipid] {
			t.Fatalf("poisoned process %d survived: %+v", ipid, forward)
		}
	}
	for _, itid := range []int64{20, 30} {
		if _, ok := forward.ByITID[itid]; ok || !forward.AmbiguousITID[itid] {
			t.Fatalf("poisoned thread %d survived: %+v", itid, forward)
		}
		assertTraceDBThreadAbsentFromSecondaryIndexes(t, forward, itid)
	}
}

func TestTraceDBIdentityCurrentCrossAliasPoisonClosureIsOrderIndependent(t *testing.T) {
	processRows := []string{
		"INSERT INTO process VALUES (31, 31, 310, 'valid-source-before-conflict')",
		"INSERT INTO process VALUES (31, 7, 310, 'cross-alias-conflict')",
		"INSERT INTO process VALUES (7, 7, 70, 'valid-canonical-before-conflict')",
		"INSERT INTO process VALUES (8, 8, 80, 'control')",
	}
	threadRows := []string{
		"INSERT INTO thread VALUES (41, 41, 410, 8, 'valid-source-before-conflict', 0, 0, 1)",
		"INSERT INTO thread VALUES (41, 9, 410, 8, 'cross-alias-conflict', 0, 0, 1)",
		"INSERT INTO thread VALUES (9, 9, 90, 8, 'valid-canonical-before-conflict', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 100, 8, 'control', 0, 0, 1)",
	}
	load := func(t *testing.T, reverse bool) (traceDBThreadIndex, []TraceDBCoverage) {
		t.Helper()
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (id, ipid, pid, name)",
		}
		statements = append(statements, orderedTraceDBStatements(processRows, reverse)...)
		statements = append(statements, "CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)")
		statements = append(statements, orderedTraceDBStatements(threadRows, reverse)...)
		return loadTraceDBIdentityFixture(t, createTraceDBFixture(t, statements))
	}
	forward, forwardCoverage := load(t, false)
	reverse, reverseCoverage := load(t, true)
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("current cross-alias poison depends on row order:\nforward=%+v\nreverse=%+v", forward, reverse)
	}
	if forwardCoverage[1].Skipped != reverseCoverage[1].Skipped || forwardCoverage[2].Skipped != reverseCoverage[2].Skipped {
		t.Fatalf("current cross-alias coverage depends on row order:\nforward=%+v\nreverse=%+v", forwardCoverage, reverseCoverage)
	}
	for _, id := range []int64{7, 31} {
		if _, ok := forward.Processes[id]; ok || !forward.AmbiguousIPID[id] || !forward.AmbiguousProcessID[id] {
			t.Fatalf("process cross-alias poison closure left ID %d authoritative: %+v", id, forward)
		}
		if _, ok := forward.ProcessIDToIPID[id]; ok {
			t.Fatalf("process source map revived poisoned ID %d: %+v", id, forward.ProcessIDToIPID)
		}
	}
	for _, id := range []int64{9, 41} {
		if _, ok := forward.ByITID[id]; ok || !forward.AmbiguousITID[id] || !forward.AmbiguousThreadID[id] {
			t.Fatalf("thread cross-alias poison closure left ID %d authoritative: %+v", id, forward)
		}
		if _, ok := forward.ThreadIDToITID[id]; ok {
			t.Fatalf("thread source map revived poisoned ID %d: %+v", id, forward.ThreadIDToITID)
		}
		assertTraceDBThreadAbsentFromSecondaryIndexes(t, forward, id)
	}
	if forward.Processes[8].PID != 80 || forward.ByITID[10].TID != 100 {
		t.Fatalf("cross-alias poison leaked into valid control identity: %+v", forward)
	}
}

func TestTraceDBTraceStartStrictSingletonAndType(t *testing.T) {
	tests := []struct {
		name       string
		values     []string
		want       int64
		wantAccept bool
	}{
		{name: "zero", values: []string{"0"}, wantAccept: true},
		{name: "positive", values: []string{"123"}, want: 123, wantAccept: true},
		{name: "numeric text", values: []string{"CAST(123 AS TEXT)"}},
		{name: "integral real", values: []string{"123.0"}},
		{name: "blob", values: []string{"x'313233'"}},
		{name: "null", values: []string{"NULL"}},
		{name: "negative", values: []string{"-1"}},
		{name: "empty"},
		{name: "duplicate", values: []string{"123", "123"}},
		{name: "conflict", values: []string{"123", "124"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := []string{"CREATE TABLE trace_range (start_ts)"}
			for _, value := range test.values {
				statements = append(statements, "INSERT INTO trace_range VALUES ("+value+")")
			}
			path := createTraceDBFixture(t, statements)
			tdb, err := openTraceDB(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer tdb.close()
			got, coverage, err := tdb.traceStart(context.Background())
			if err != nil {
				t.Fatalf("trace start audit: %v", err)
			}
			if test.wantAccept {
				if got != test.want || coverage.RowsEmitted != 1 || coverage.Skipped != "" {
					t.Fatalf("valid trace start rejected: got=%d coverage=%+v", got, coverage)
				}
			} else if got != 0 || coverage.RowsEmitted != 0 || coverage.Skipped == "" {
				t.Fatalf("invalid trace start was accepted: got=%d coverage=%+v", got, coverage)
			}
			index, _, err := tdb.loadThreadIndex(context.Background())
			if err != nil {
				t.Fatalf("load identity with trace-start provenance: %v", err)
			}
			if index.TraceStartKnown != test.wantAccept || index.TraceStart != test.want {
				t.Fatalf("trace-start provenance=(value=%d known=%t), want (%d,%t)", index.TraceStart, index.TraceStartKnown, test.want, test.wantAccept)
			}
		})
	}
}

func loadTraceDBIdentityFixture(t *testing.T, path string) (traceDBThreadIndex, []TraceDBCoverage) {
	t.Helper()
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, coverage, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatalf("load identity index: %v", err)
	}
	return index, coverage
}

func orderedTraceDBStatements(items []string, reverse bool) []string {
	out := append([]string(nil), items...)
	if reverse {
		for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
			out[left], out[right] = out[right], out[left]
		}
	}
	return out
}

func assertTraceDBThreadAbsentFromSecondaryIndexes(t *testing.T, index traceDBThreadIndex, itid int64) {
	t.Helper()
	for _, items := range index.ByTIDCandidates {
		for _, item := range items {
			if item.ITID == itid {
				t.Fatalf("poisoned ITID %d survived ByTIDCandidates: %+v", itid, index.ByTIDCandidates)
			}
		}
	}
	for _, items := range index.ByProcess {
		for _, item := range items {
			if item.ITID == itid {
				t.Fatalf("poisoned ITID %d survived ByProcess: %+v", itid, index.ByProcess)
			}
		}
	}
}
