package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceDBStableRowIDProfilesKeepIdentitySentinelsOutOfRowDomain(t *testing.T) {
	for raw, want := range map[int64]int64{
		0:             0,
		math.MaxInt32: math.MaxInt32,
		math.MinInt32: int64(1) << 31,
		-2:            (int64(1) << 32) - 2,
		-1:            math.MaxUint32,
	} {
		got, ok := traceDBActivityITIDSignedInt32.decodeStableRowID(raw)
		if !ok || got != want {
			t.Fatalf("signed stable row id raw=%d got=(%d,%t), want=%d", raw, got, ok, want)
		}
	}
	for _, raw := range []any{int64(math.MaxInt32) + 1, int64(math.MinInt32) - 1, nil, "-2", float64(-2)} {
		if got, ok := traceDBActivityITIDSignedInt32.decodeStableRowID(raw); ok {
			t.Fatalf("invalid signed stable row id raw=%v decoded=%d", raw, got)
		}
	}
	for raw, want := range map[int64]int64{0: 0, math.MaxUint32: math.MaxUint32} {
		got, ok := traceDBActivityITIDCanonical.decodeStableRowID(raw)
		if !ok || got != want {
			t.Fatalf("canonical stable row id raw=%d got=(%d,%t), want=%d", raw, got, ok, want)
		}
	}
	for _, raw := range []any{int64(-1), int64(math.MaxUint32) + 1, nil, "1", float64(1)} {
		if got, ok := traceDBActivityITIDCanonical.decodeStableRowID(raw); ok {
			t.Fatalf("invalid canonical stable row id raw=%v decoded=%d", raw, got)
		}
	}
}

func TestTraceDBFrameProfileFlowsFromLifecycleCollectorIntoAuthority(t *testing.T) {
	for _, tc := range []struct {
		name        string
		current     bool
		wantProfile traceDBActivityITIDProfile
		wantSource  string
	}{
		{
			name:        "current signed projection",
			current:     true,
			wantProfile: traceDBActivityITIDSignedInt32,
			wantSource:  "current frame_slice id+type signed-int32 producer profile",
		},
		{
			name:        "legacy canonical compatibility",
			current:     false,
			wantProfile: traceDBActivityITIDCanonical,
			wantSource:  "legacy frame_slice no-id/no-type canonical compatibility profile",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := createTraceDBFixture(t, traceDBFrameAuthorityProfileStatements(tc.current))
			tdb, err := openTraceDB(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer tdb.close()
			identities, _, err := tdb.loadThreadIndex(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			collection, err := collectTraceDBLifecycle(context.Background(), tdb.db, identities)
			if err != nil {
				t.Fatal(err)
			}
			if !collection.CreationComplete || !collection.TerminalComplete || !collection.ActivityComplete ||
				collection.FrameProfile != tc.wantProfile || collection.FrameProfileSource != tc.wantSource {
				t.Fatalf("collector frame profile mismatch: %+v", collection)
			}
			authority := newTraceDBSchedulerAuthority(identities, collection)
			if !authority.initialized || !authority.complete || authority.frameProfile != tc.wantProfile ||
				authority.frameProfileSource != tc.wantSource {
				t.Fatalf("authority did not preserve collector profile: %+v", authority)
			}
		})
	}
}

func TestTraceDBFrameCurrentSignedProfileProductionEndToEnd(t *testing.T) {
	path := createTraceDBFixture(t, traceDBFrameAuthorityProfileStatements(true))
	outPath := filepath.Join(t.TempDir(), "frame-current-profile.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("production export: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, endpoint := range []string{
		"S|500|FrameActual-7|hconv-frame-4294967294",
		"F|500|FrameActual-7|hconv-frame-4294967294",
	} {
		if !strings.Contains(body, endpoint) {
			t.Fatalf("current signed frame endpoint %q missing:\n%s", endpoint, body)
		}
	}
	frameCoverage := TraceDBCoverage{}
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "frame_slice" {
			frameCoverage = item
			break
		}
	}
	if frameCoverage.RowsEmitted != 2 || frameCoverage.Skipped != "" ||
		!strings.Contains(frameCoverage.FieldSources["schema_profile"], "current frame_slice id+type signed-int32 producer profile") ||
		!strings.Contains(frameCoverage.FieldSources["identity"], "closed-generation admission") ||
		!strings.Contains(frameCoverage.FieldSources["header_cpu"], "exact checked End=ts+dur") ||
		!strings.Contains(frameCoverage.FieldSources["vsync"], "NULL is upstream INVALID_UINT32/no-vsync") {
		t.Fatalf("production frame authority provenance mismatch: %+v", frameCoverage)
	}
}

func traceDBFrameAuthorityProfileStatements(current bool) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, ref_type TEXT)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"CREATE TABLE sched_slice (ts INT, dur INT, itid INT, end_state TEXT)",
		"CREATE TABLE syscall (ts INT, itid INT)",
		"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT)",
		"CREATE TABLE callstack (ts INT, itid INT)",
	}
	if current {
		statements = append(statements,
			"CREATE TABLE process (id INT, ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 1, 500, 'MainApp')",
			"CREATE TABLE thread (id INT, itid INT, tid INT, ipid INT, name TEXT, start_ts INT, end_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (4294967294, 4294967294, 501, 1, 'HighIdentityThread', NULL, NULL, 0, 1)",
			"INSERT INTO thread_state VALUES (4294967294, 900000, 200000, 3, 'Running')",
			"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (-2, 1000000, 16000, 0, 'actural', 7, 1, 1, -2)",
		)
	} else {
		statements = append(statements,
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'MainApp')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, end_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'LegacyThread', NULL, NULL, 0, 1)",
			"INSERT INTO thread_state VALUES (2, 900000, 200000, 3, 'Running')",
			"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (1000000, 16000, 'actural', 7, 1, 1, 2)",
		)
	}
	return statements
}
