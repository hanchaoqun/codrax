package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gzipExistingDBBody(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile(existingTraceDBFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func assertGzipExistingDBFailure(t *testing.T, opts Options, result Result, err error) {
	t.Helper()
	assertGzipRouteFailure(t, opts, result, err)
	if path := retainedTraceDBOutputPath(opts, opts.InputPath, opts.OutputPath); path != "" {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("failed route retained DB %s: %v", path, statErr)
		}
	}
}

func TestGzipExistingTraceDBRejectsUnqueryableAndNonClosedPayloads(t *testing.T) {
	body := gzipExistingDBBody(t)
	wal := append([]byte(nil), body...)
	wal[18], wal[19] = 2, 2
	unknownMode := append([]byte(nil), body...)
	unknownMode[18] = 0
	unrelated, err := os.ReadFile(createTraceDBFixture(t, []string{"CREATE TABLE unrelated (value)", "INSERT INTO unrelated VALUES ('not trace')"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"WAL-header", wal}, {"unknown-header-mode", unknownMode},
		{"short-SQLite-header", []byte("SQLite format 3\x00payload")},
		{"corrupt-pages", body[:101]}, {"unqueryable-database", unrelated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, _ := gzipRouteCapture(t, tc.body, "claimed-valid.db")
			opts.KeepTraceDB = true
			result, err := ConvertFile(t.Context(), opts)
			assertGzipExistingDBFailure(t, opts, result, err)
		})
	}
}

func TestGzipExistingTraceDBIntegrityAndExplicitEngineBoundaries(t *testing.T) {
	body := gzipExistingDBBody(t)
	for _, mode := range []string{"CRC", "ISIZE", "truncated", "second-member", "trailing-byte", "builtin-engine", "archive-member"} {
		t.Run(mode, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, body, "trusted-looking.db")
			opts.KeepTraceDB = true
			changed := append([]byte(nil), original...)
			switch mode {
			case "CRC":
				changed[len(changed)-8] ^= 1
			case "ISIZE":
				changed[len(changed)-4] ^= 1
			case "truncated":
				changed = changed[:len(changed)-1]
			case "second-member":
				changed = append(changed, gzipTextFixture(t, body, gzip.DefaultCompression)...)
			case "trailing-byte":
				changed = append(changed, 0)
			case "builtin-engine":
				opts.TraceEngine = traceEngineBuiltin
				opts.KeepTraceDB = false
			case "archive-member":
				opts.ArchiveMember = "claimed-valid.db"
			}
			if err := os.Chmod(opts.InputPath, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(opts.InputPath, changed, 0o400); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(t.Context(), opts)
			assertGzipExistingDBFailure(t, opts, result, err)
			if got, readErr := os.ReadFile(opts.InputPath); readErr != nil || !bytes.Equal(got, changed) {
				t.Fatalf("failed intake changed original: %v", readErr)
			}
		})
	}
}

func TestGzipExistingTraceDBGenerationChangesAndCancellationRollback(t *testing.T) {
	body := gzipExistingDBBody(t)
	for _, stage := range []string{"gzip_input_decompress", "existing_trace_db_snapshot", "existing_trace_db_export"} {
		for _, change := range []string{"source-replacement", "decoded-replacement", "cancel"} {
			t.Run(stage+"/"+change, func(t *testing.T) {
				opts, original := gzipRouteCapture(t, body, "capture.db")
				opts.KeepTraceDB = true
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				fired := false
				opts.Progress = func(event ProgressEvent) {
					if fired || event.Stage != stage || event.Status != ProgressStatusComplete {
						return
					}
					fired = true
					if change == "cancel" {
						cancel()
						return
					}
					path, data := opts.InputPath, original
					if change == "decoded-replacement" {
						matches, err := filepath.Glob(filepath.Join(opts.RuntimeAnchor, ".*.gzip", gzipBinaryInputSnapshotLeaf))
						if err != nil || len(matches) != 1 {
							t.Fatalf("decoded held snapshot unavailable: %v %v", matches, err)
						}
						path, data = matches[0], body
					}
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					replacement := path + ".replacement"
					if err := os.WriteFile(replacement, data, 0o400); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, path); err != nil {
						t.Fatal(err)
					}
				}
				result, err := ConvertFile(ctx, opts)
				assertGzipExistingDBFailure(t, opts, result, err)
				if !fired {
					t.Fatalf("requested boundary never reached: %v", err)
				}
				if change == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			})
		}
	}
}

func TestGzipExistingTraceDBRetainedPublicationNeverReplacesCompetitor(t *testing.T) {
	body := gzipExistingDBBody(t)
	for _, mode := range []string{"early", "late", "source-collision", "output-collision"} {
		t.Run(mode, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, body, "capture.db")
			opts.TraceDBOutputPath = filepath.Join(filepath.Dir(opts.OutputPath), "retained.db")
			fired := false
			switch mode {
			case "early":
				if err := os.WriteFile(opts.TraceDBOutputPath, []byte("competitor"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "late":
				opts.Progress = func(event ProgressEvent) {
					if !fired && event.Stage == "existing_trace_db_snapshot" && event.Status == ProgressStatusComplete {
						fired = true
						if err := os.WriteFile(opts.TraceDBOutputPath, []byte("competitor"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "source-collision":
				opts.TraceDBOutputPath = opts.InputPath
			case "output-collision":
				opts.TraceDBOutputPath = opts.OutputPath
			}
			result, err := ConvertFile(t.Context(), opts)
			assertGzipRouteFailure(t, opts, result, err)
			if mode == "early" || mode == "late" {
				if got, err := os.ReadFile(opts.TraceDBOutputPath); err != nil || string(got) != "competitor" {
					t.Fatalf("competing retained file changed: %q %v", got, err)
				}
			}
			if mode == "late" && !fired {
				t.Fatal("late publication boundary not exercised")
			}
			if got, err := os.ReadFile(opts.InputPath); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("original changed: %v", err)
			}
		})
	}
}

func TestGzipExistingTraceDBNameCannotSelectSidecarsOrExposePrivateSource(t *testing.T) {
	body := gzipExistingDBBody(t)
	claimedDB := filepath.Join(t.TempDir(), "not-the-source.db")
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.WriteFile(claimedDB+suffix, []byte("unrelated"), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	opts, _ := gzipRouteCapture(t, body, claimedDB)
	opts.TraceEngine = traceEngineTraceStreamer
	opts.TraceStreamerPath = filepath.Join(t.TempDir(), "absent-converter")
	result, err := ConvertFile(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{claimedDB, gzipBinaryInputSnapshotLeaf, opts.RuntimeAnchor, "existing-db-"} {
		if bytes.Contains(manifest, []byte(token)) {
			t.Fatalf("private or claimed source escaped into manifest: %q", token)
		}
	}
	if !strings.Contains(strings.Join(result.Caveats, " "), "pre-compression database lifecycle is not established") {
		t.Fatal("gzip incorrectly establishes original DB closure")
	}
	assertGzipRoutePrivateClean(t, opts)
}
