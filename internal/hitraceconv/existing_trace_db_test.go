package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func existingTraceDBFixture(t *testing.T) string {
	t.Helper()
	return createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)", "INSERT INTO process VALUES (1, 200, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 200, 1, 'main', 1, 2), (2, 201, 1, 'worker', 0, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (0, 1000000, 1, 'S', 120, 1), (1000000, 1000000, 1, 'R', 120, 2), (2000000, 1000000, 1, 'S', 120, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"CREATE TABLE callstack (callid INT, ts INT)",
		"CREATE TABLE syscall (itid INT, ts INT)",
		"CREATE TABLE native_hook (itid INT, start_ts INT)",
		"CREATE TABLE frame_slice (itid INT, ts INT)",
		"CREATE TABLE vendor_details (payload BLOB, optional TEXT)", "INSERT INTO vendor_details VALUES (x'00FF42', NULL)",
	})
}

func existingTraceDBOptions(t *testing.T, input string) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{InputPath: input, OutputPath: filepath.Join(dir, "prepared.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")}
}

func assertExistingTraceDBFailureClean(t *testing.T, opts Options, result Result, err error) {
	t.Helper()
	if err == nil || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("failure must return no publication: result=%+v err=%v", result, err)
	}
	for _, path := range []string{opts.OutputPath, traceSidecarBase(opts.InputPath, opts.OutputPath) + ".tracebundle.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("failed preparation retained publication %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(opts.RuntimeAnchor)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed preparation retained staging: %+v", entries)
	}
}

func TestPrepareExistingTraceDBReadOnlyQueryableFidelity(t *testing.T) {
	input := existingTraceDBFixture(t)
	nonDB := filepath.Join(filepath.Dir(input), "capture.no-db-extension")
	if err := os.Rename(input, nonDB); err != nil {
		t.Fatal(err)
	}
	input = nonDB
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(input, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(input), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(input), 0o755) })
	opts := existingTraceDBOptions(t, input)
	opts.KeepTraceDB = true
	// A converter cannot be needed: even an explicitly configured absent
	// executable does not participate in this separate read-only route.
	opts.TraceStreamerPath = filepath.Join(t.TempDir(), "must-not-be-invoked")
	result, err := PrepareExistingTraceDB(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(before)
	if result.ExistingTraceDBSource == nil || result.ExistingTraceDBSource.Path != input ||
		result.ExistingTraceDBSource.Bytes != int64(len(before)) || result.ExistingTraceDBSource.SHA256 != hex.EncodeToString(sum[:]) ||
		result.ExistingTraceDBSource.Generation == "" || QueryReadySystracePath(result) != opts.OutputPath || result.BundlePath == "" {
		t.Fatalf("source / owned query receipt missing: %+v", result)
	}
	if err := ValidateExistingTraceDBSource(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("source bytes changed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(input))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
		t.Fatalf("source-side files changed: %v %v", entries, err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var switches []tracequery.Event
	for _, event := range idx.Events {
		if event.Type == tracequery.EventSchedSwitch {
			switches = append(switches, event)
		}
	}
	if len(switches) != 2 || switches[0].Ts != .001 || switches[1].Ts != .002 {
		for _, coverage := range result.TraceDBCoverage {
			if coverage.Family == "scheduler" || coverage.Family == "resolver" || strings.HasPrefix(coverage.Family, "resolver.lifecycle") {
				t.Logf("scheduler admission: %+v", coverage)
			}
		}
		t.Fatalf("exact scheduler projection changed: %+v", switches)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var vendorID int
	records := readTraceDBTextFidelityWire(t, string(body))
	for key, record := range records {
		if key.Kind == "schema" {
			var schema traceDBTextFidelitySchema
			if err := json.Unmarshal(traceDBTextFidelityWirePayload(t, key, record), &schema); err != nil {
				t.Fatal(err)
			}
			if string(traceDBTextFidelityDecodedBytes(t, schema.Table)) == "vendor_details" {
				vendorID = schema.TableID
			}
		}
	}
	var preserved bool
	for key, record := range records {
		if key.Kind != "row" || key.TableID != vendorID {
			continue
		}
		var row traceDBTextFidelityRow
		if err := json.Unmarshal(traceDBTextFidelityWirePayload(t, key, record), &row); err != nil {
			t.Fatal(err)
		}
		preserved = len(row.Cells) == 2 && bytes.Equal(traceDBTextFidelityDecodedBytes(t, row.Cells[0]), []byte{0, 255, 66}) && row.Cells[1].Storage == "null"
	}
	if !preserved {
		t.Fatal("unadapted table exact blob/null values were not preserved")
	}
	entries, err = os.ReadDir(opts.RuntimeAnchor)
	if err != nil || len(entries) != 0 {
		t.Fatalf("success retained private staging: %v %v", entries, err)
	}
}

func TestPrepareExistingTraceDBRejectsAuxiliaryAndWALModes(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		for _, kind := range []string{"file", "directory", "dangling-link"} {
			t.Run(suffix+"/"+kind, func(t *testing.T) {
				input := existingTraceDBFixture(t)
				var err error
				switch kind {
				case "file":
					err = os.WriteFile(input+suffix, nil, 0o600)
				case "directory":
					err = os.Mkdir(input+suffix, 0o700)
				case "dangling-link":
					err = os.Symlink("absent", input+suffix)
				}
				if err != nil {
					t.Skipf("fixture unavailable: %v", err)
				}
				opts := existingTraceDBOptions(t, input)
				result, err := PrepareExistingTraceDB(t.Context(), opts)
				assertExistingTraceDBFailureClean(t, opts, result, err)
				if !errors.Is(err, errTraceStreamerDBAuxiliaryState) || ValidateExistingTraceDBSource(t.Context(), input) == nil {
					t.Fatalf("auxiliary state not rejected: %v", err)
				}
			})
		}
	}
	t.Run("checkpointed-WAL-header", func(t *testing.T) {
		input := existingTraceDBFixture(t)
		db, err := sql.Open("sqlite", input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		opts := existingTraceDBOptions(t, input)
		result, err := PrepareExistingTraceDB(t.Context(), opts)
		assertExistingTraceDBFailureClean(t, opts, result, err)
		if !strings.Contains(err.Error(), "header mode") {
			t.Fatalf("checkpointed WAL header was not rejected: %v", err)
		}
	})
}

func TestPrepareExistingTraceDBSourceChangesAndCancellationRollback(t *testing.T) {
	for _, stage := range []string{"existing_trace_db_snapshot", "existing_trace_db_export"} {
		for _, change := range []string{"same-size-mtime-replace", "in-place", "new-sidecar", "cancel"} {
			t.Run(stage+"/"+change, func(t *testing.T) {
				input := existingTraceDBFixture(t)
				opts := existingTraceDBOptions(t, input)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				fired := false
				opts.Progress = func(event ProgressEvent) {
					if fired || event.Stage != stage || event.Status != ProgressStatusComplete {
						return
					}
					fired = true
					switch change {
					case "cancel":
						cancel()
					case "new-sidecar":
						if err := os.WriteFile(input+"-wal", nil, 0o600); err != nil {
							t.Fatal(err)
						}
					case "in-place":
						file, err := os.OpenFile(input, os.O_WRONLY, 0)
						if err != nil {
							t.Fatal(err)
						}
						_, err = file.WriteAt([]byte{9}, 60)
						if err := traceDBJoinPreservingSingle(err, file.Close()); err != nil {
							t.Fatal(err)
						}
					case "same-size-mtime-replace":
						info, err := os.Stat(input)
						if err != nil {
							t.Fatal(err)
						}
						body, err := os.ReadFile(input)
						if err != nil {
							t.Fatal(err)
						}
						other := input + ".replacement"
						if err := os.WriteFile(other, body, 0o600); err != nil {
							t.Fatal(err)
						}
						if err := os.Chtimes(other, info.ModTime(), info.ModTime()); err != nil {
							t.Fatal(err)
						}
						if err := os.Rename(other, input); err != nil {
							t.Fatal(err)
						}
					}
				}
				result, err := PrepareExistingTraceDB(ctx, opts)
				assertExistingTraceDBFailureClean(t, opts, result, err)
				if !fired {
					t.Fatalf("requested boundary did not execute: %v", err)
				}
				if change == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation: %v", err)
				}
			})
		}
	}
}

func TestPrepareExistingTraceDBUnqueryableInputAndOutputCollision(t *testing.T) {
	for _, statements := range [][]string{
		{"CREATE TABLE unrelated (text)", "INSERT INTO unrelated VALUES ('hello')"},
		{"CREATE TABLE sched_slice (ts)"},
		{"CREATE TABLE sched_slice (ts, dur, cpu, end_state, priority, itid)", "INSERT INTO sched_slice VALUES ('bad', 1, 0, 'S', 120, 1)"},
	} {
		input := createTraceDBFixture(t, statements)
		opts := existingTraceDBOptions(t, input)
		result, err := PrepareExistingTraceDB(t.Context(), opts)
		assertExistingTraceDBFailureClean(t, opts, result, err)
	}
	input := existingTraceDBFixture(t)
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		opts := existingTraceDBOptions(t, input)
		opts.OutputPath = input + suffix
		result, err := PrepareExistingTraceDB(t.Context(), opts)
		assertExistingTraceDBFailureClean(t, opts, result, err)
	}
	for _, body := range [][]byte{[]byte("not a database"), append([]byte("SQLite format 3\x00"), make([]byte, 84)...)} {
		path := filepath.Join(t.TempDir(), "invalid.db")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		opts := existingTraceDBOptions(t, path)
		result, err := PrepareExistingTraceDB(t.Context(), opts)
		assertExistingTraceDBFailureClean(t, opts, result, err)
	}
}
