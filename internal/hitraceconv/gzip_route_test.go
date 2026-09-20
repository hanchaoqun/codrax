package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func gzipRouteCapture(t *testing.T, body []byte, name string) (Options, []byte) {
	t.Helper()
	dir := t.TempDir()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	writer.Name, writer.Comment = name, "capture metadata is not a format or clock authority"
	writer.ModTime = time.Unix(1700000000, 0)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), encoded.Bytes()...)
	input := filepath.Join(dir, "capture.not-a-format-extension")
	if err := os.WriteFile(input, original, 0o400); err != nil {
		t.Fatal(err)
	}
	return Options{InputPath: input, OutputPath: filepath.Join(dir, "converted.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")}, original
}

func assertGzipRoutePrivateClean(t *testing.T, opts Options) {
	t.Helper()
	entries, err := os.ReadDir(opts.RuntimeAnchor)
	if err != nil && !os.IsNotExist(err) || len(entries) != 0 {
		t.Fatalf("conversion retained private runtime files: %v %v", entries, err)
	}
}

func assertGzipRouteFailure(t *testing.T, opts Options, result Result, err error) {
	t.Helper()
	if err == nil || !reflectResultZero(result) {
		t.Fatalf("failed route returned material: %+v %v", result, err)
	}
	for _, path := range []string{opts.OutputPath, traceSidecarBase(opts.InputPath, opts.OutputPath) + ".tracebundle.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("failed gzip route published %q: %v", path, err)
		}
	}
	assertGzipRoutePrivateClean(t, opts)
}

func TestGzipRouteBuiltinMatchesUncompressedBytesAndBindsOuterProvenance(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	opts, original := gzipRouteCapture(t, body, "../../PERFILE2-not-authority.gz")
	opts.TraceEngine = traceEngineBuiltin
	sourceGeneration, err := filegeneration.FromPath(opts.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.InputPath != opts.InputPath || result.InputBytes != int64(len(original)) || result.OutputPath != opts.OutputPath || result.TextTransport != nil || result.ArchiveProvenance != nil || result.GzipInputProvenance == nil || QueryReadySystracePath(result) != opts.OutputPath {
		t.Fatalf("gzip RMQ route drifted: %+v", result)
	}
	p := result.GzipInputProvenance
	if p.Profile != tracebundle.GzipInputProfileV1 || p.SourceBytes != int64(len(original)) || p.SourceSHA256 != traceArchiveTestSHA(original) || p.SourceGeneration != sourceGeneration.CacheToken() || p.DecodedBytes != int64(len(body)) || p.DecodedSHA256 != traceArchiveTestSHA(body) || p.DecodedFormat != "harmony_rmq" || p.DecodedGeneration == "" {
		t.Fatalf("wrong gzip origin receipt: %+v", p)
	}
	rawPath := filepath.Join(t.TempDir(), "raw.sys")
	if err := os.WriteFile(rawPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := ConvertFile(context.Background(), Options{InputPath: rawPath, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(raw.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(result.OutputPath)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("gzip transport altered builtin bytes: %v", err)
	}
	manifest, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata traceBundleMetadata
	if err := json.Unmarshal(manifest, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.GzipInputProvenance == nil || *metadata.GzipInputProvenance != *p {
		t.Fatalf("result/bundle gzip origin mismatch: %+v %+v", p, metadata.GzipInputProvenance)
	}
	for _, token := range []string{gzipBinaryInputSnapshotLeaf, ".gzip/", "../../PERFILE2-not-authority.gz"} {
		if bytes.Contains(manifest, []byte(token)) {
			t.Fatalf("transport metadata leaked private/unsafe label %q", token)
		}
	}
	for _, artifact := range result.Artifacts {
		if artifact.Standalone != nil || artifact.PerfTransform != nil {
			t.Fatalf("top-level gzip fabricated standalone perf transform: %+v", artifact)
		}
	}
	if _, err := tracequery.BuildIndex(context.Background(), result.BundlePath); err != nil {
		t.Fatalf("gzip provenance bundle cannot be queried: %v", err)
	}
	if got, err := os.ReadFile(opts.InputPath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original changed: %v", err)
	}
	assertGzipRoutePrivateClean(t, opts)
}

func TestGzipRouteTraceStreamerConsumesDecodedBytesAndPrepareAloneRetainsDB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	for _, tc := range []struct {
		name, engine  string
		prepare, keep bool
	}{
		{"explicit-auto", traceEngineAuto, false, false},
		{"explicit-streamer", traceEngineTraceStreamer, false, false},
		{"default-prepare", traceEngineAuto, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := traceArchiveTestBuiltinBody()
			opts, original := gzipRouteCapture(t, body, "pretend-perf.data")
			dir := filepath.Dir(opts.InputPath)
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			opts.TraceStreamerPath, opts.TraceEngine = writeFakeTraceStreamer(t, dir, 0), tc.engine
			consumed, argsLog := filepath.Join(dir, "consumed.bin"), filepath.Join(dir, "args.log")
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_CONSUMED_INPUT", consumed)
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
			convert := ConvertFile
			if tc.prepare {
				convert = PrepareFile
			}
			result, err := convert(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, true) || QueryReadySystracePath(result) == "" {
				t.Fatalf("decoded capture did not use SQL provider: %+v", result)
			}
			got, err := os.ReadFile(consumed)
			if err != nil || !bytes.Equal(got, body) || bytes.Equal(got, original) {
				t.Fatalf("trace_streamer consumed wrong gzip generation: %v", err)
			}
			args, err := os.ReadFile(argsLog)
			if err != nil {
				t.Fatal(err)
			}
			firstArg := strings.Split(strings.TrimSpace(string(args)), "\n")[0]
			if firstArg == opts.InputPath {
				t.Fatal("trace_streamer reopened outer gzip path")
			}
			if runtime.GOOS == "linux" {
				if !strings.HasPrefix(firstArg, "/proc/self/fd/") {
					t.Fatalf("Linux provider lost verified FD transport: %q", firstArg)
				}
			} else if filepath.Base(firstArg) != gzipBinaryInputSnapshotLeaf {
				t.Fatalf("gzip Name selected provider snapshot leaf: %q", firstArg)
			}
			if hasArtifact(result.Artifacts, ArtifactTraceDB) != tc.keep {
				t.Fatalf("KeepTraceDB policy ignored prepare=%v: %+v", tc.prepare, result.Artifacts)
			}
			retainedPath := traceSidecarBase(opts.InputPath, opts.OutputPath) + ".trace.db"
			if _, err := os.Lstat(retainedPath); tc.keep && err != nil || !tc.keep && !os.IsNotExist(err) {
				t.Fatalf("retained DB path ignored prepare=%v: %v", tc.prepare, err)
			}
			for _, artifact := range result.Artifacts {
				if artifact.Type == ArtifactTraceDB {
					if _, err := os.Stat(artifact.Path); err != nil {
						t.Fatalf("retained DB is missing: %v", err)
					}
				}
			}
			if result.GzipInputProvenance == nil || result.GzipInputProvenance.DecodedFormat != "harmony_rmq" {
				t.Fatalf("SQL route lost gzip origin: %+v", result.GzipInputProvenance)
			}
			assertGzipRoutePrivateClean(t, opts)
		})
	}
}

func TestGzipRouteTextIsOnlyByteTransportForBothEntrypoints(t *testing.T) {
	body := []byte("# tracer: nop\nworker-42 (42) [000] .... 1234.567891: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n")
	for _, prepare := range []bool{false, true} {
		name := "explicit"
		if prepare {
			name = "default-prepare"
		}
		t.Run(name, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, body, "capture.perf.data")
			convert := ConvertFile
			if prepare {
				convert = PrepareFile
			}
			result, err := convert(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if result.TextTransport == nil || result.GzipInputProvenance != nil || result.BundlePath != "" || len(result.Artifacts) != 0 || len(result.TraceDecisions) != 0 || len(result.ProviderDecisions) != 0 || len(result.TraceCoverage) != 0 || len(result.TraceDBCoverage) != 0 || result.EventsWritten != 0 || QueryReadySystracePath(result) != "" {
				t.Fatalf("text transport gained semantic producer authority: %+v", result)
			}
			if result.InputPath != opts.InputPath || result.InputBytes != int64(len(original)) || result.OutputBytes != int64(len(body)) {
				t.Fatalf("text transport mislabeled source: %+v", result)
			}
			transport := result.TextTransport
			id, err := filegeneration.FromPath(result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			source, err := filegeneration.FromPath(opts.InputPath)
			if err != nil {
				t.Fatal(err)
			}
			if transport.Profile != GzipTextTransportProfile || transport.SourceGeneration != source.CacheToken() || transport.DecodedGeneration != id.CacheToken() || transport.DecodedSHA256 != traceArchiveTestSHA(body) || transport.SourceSHA256 != traceArchiveTestSHA(original) {
				t.Fatalf("transport receipt mismatch: %+v", transport)
			}
			got, err := os.ReadFile(result.OutputPath)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("text/clock bytes changed: %v", err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
			if err != nil || len(idx.Events) != 1 || idx.Events[0].Ts != 1234.567891 || idx.Events[0].WakeePID != 99 {
				t.Fatalf("transport rebased gzip mtime into source clock: %+v %v", idx, err)
			}
			if _, err := os.Stat(traceSidecarBase(opts.InputPath, opts.OutputPath) + ".tracebundle.json"); !os.IsNotExist(err) {
				t.Fatalf("text transport published semantic bundle: %v", err)
			}
			assertGzipRoutePrivateClean(t, opts)
		})
	}
}

func TestGzipRouteTextRejectsExplicitDatabaseAndStreamerOptions(t *testing.T) {
	for _, field := range []string{"keep-db", "db-output", "streamer-engine", "streamer-path", "streamer-dirs", "archive-member"} {
		t.Run(field, func(t *testing.T) {
			opts, _ := gzipRouteCapture(t, []byte("# tracer: nop\n"), "text.sys")
			switch field {
			case "keep-db":
				opts.KeepTraceDB = true
			case "db-output":
				opts.TraceDBOutputPath = filepath.Join(filepath.Dir(opts.OutputPath), "out.db")
			case "streamer-engine":
				opts.TraceEngine = traceEngineTraceStreamer
			case "streamer-path":
				opts.TraceStreamerPath = filepath.Join(filepath.Dir(opts.InputPath), "not-executed")
			case "streamer-dirs":
				opts.TraceStreamerSoDirs = []string{filepath.Dir(opts.InputPath)}
			case "archive-member":
				opts.ArchiveMember = "claimed.sys"
			}
			result, err := ConvertFile(context.Background(), opts)
			assertGzipRouteFailure(t, opts, result, err)
		})
	}
}

func TestGzipRouteCancellationAndNoReplacePreserveOriginals(t *testing.T) {
	for _, kind := range []string{"text", "rmq"} {
		for _, mode := range []string{"cancel-start", "cancel-complete", "output-exists", "source-collision"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				body := []byte("# tracer: nop\n")
				if kind == "rmq" {
					body = traceArchiveTestBuiltinBody()
				}
				opts, original := gzipRouteCapture(t, body, "capture.sys")
				opts.TraceEngine = traceEngineBuiltin
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				switch mode {
				case "cancel-start", "cancel-complete":
					status := ProgressStatusStarted
					if mode == "cancel-complete" {
						status = ProgressStatusComplete
					}
					opts.Progress = func(event ProgressEvent) {
						if event.Stage == "gzip_input_decompress" && event.Status == status {
							cancel()
						}
					}
				case "output-exists":
					if err := os.WriteFile(opts.OutputPath, []byte("competitor"), 0o600); err != nil {
						t.Fatal(err)
					}
				case "source-collision":
					opts.OutputPath = opts.InputPath
				}
				result, err := ConvertFile(ctx, opts)
				if err == nil || !reflectResultZero(result) {
					t.Fatalf("unsafe conversion succeeded: %+v %v", result, err)
				}
				if strings.HasPrefix(mode, "cancel-") {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("lost cancellation: %v", err)
					}
					assertGzipRouteFailure(t, opts, result, err)
				} else {
					assertGzipRoutePrivateClean(t, opts)
					if _, err := os.Lstat(traceSidecarBase(opts.InputPath, opts.OutputPath) + ".tracebundle.json"); !os.IsNotExist(err) {
						t.Fatalf("output collision published semantic bundle: %v", err)
					}
					if mode == "output-exists" {
						if got, err := os.ReadFile(opts.OutputPath); err != nil || string(got) != "competitor" {
							t.Fatalf("competitor changed: %q %v", got, err)
						}
					}
				}
				if got, err := os.ReadFile(opts.InputPath); err != nil || !bytes.Equal(got, original) {
					t.Fatalf("original changed: %v", err)
				}
			})
		}
	}
}

func TestGzipRouteMetadataCannotAdmitNestedOrUnrecognizedBinary(t *testing.T) {
	nested := gzipTextFixture(t, traceArchiveTestBuiltinBody(), gzip.DefaultCompression)
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"nested-gzip", nested}, {"sqlite", []byte("SQLite format 3\x00payload")}, {"zip", []byte("PK\x03\x04payload")}, {"driver", []byte("MZ\x00\xffpayload")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, _ := gzipRouteCapture(t, tc.body, "legitimate-rmq.sys")
			opts.TraceEngine = traceEngineBuiltin
			result, err := ConvertFile(context.Background(), opts)
			assertGzipRouteFailure(t, opts, result, err)
			var rejected *GzipTextTransportError
			if !errors.As(err, &rejected) || rejected.Code != GzipTextCodeDecodedText {
				t.Fatalf("unknown inner format gained decoder route: %v", err)
			}
		})
	}
}

func TestGzipRoutePerfRejectsExplicitTraceOnlyOptionsWithoutPublication(t *testing.T) {
	for _, field := range []string{"keep-db", "db-output", "streamer-path"} {
		t.Run(field, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, syntheticRawPerfData(), "claimed-trace.sys")
			opts.PerfParser = "raw"
			wantOption := ""
			switch field {
			case "keep-db":
				opts.KeepTraceDB, wantOption = true, "--keep-trace-db"
			case "db-output":
				opts.TraceDBOutputPath, wantOption = filepath.Join(filepath.Dir(opts.OutputPath), "unpublished.db"), "--trace-db-output"
			case "streamer-path":
				opts.TraceStreamerPath, wantOption = filepath.Join(filepath.Dir(opts.InputPath), "never-executed"), "--trace-streamer"
			}
			result, err := ConvertFile(context.Background(), opts)
			assertGzipRouteFailure(t, opts, result, err)
			if !strings.Contains(err.Error(), wantOption) {
				t.Fatalf("decoded direct-perf option conflict was mislabeled: %v", err)
			}
			for _, suffix := range []string{".perftrace", ".perf.data"} {
				if _, err := os.Stat(traceSidecarBase(opts.InputPath, opts.OutputPath) + suffix); !os.IsNotExist(err) {
					t.Fatalf("option conflict published %s: %v", suffix, err)
				}
			}
			if opts.TraceDBOutputPath != "" {
				if _, err := os.Stat(opts.TraceDBOutputPath); !os.IsNotExist(err) {
					t.Fatalf("option conflict retained DB: %v", err)
				}
			}
			if got, err := os.ReadFile(opts.InputPath); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("option conflict changed original: %v", err)
			}
		})
	}
}
