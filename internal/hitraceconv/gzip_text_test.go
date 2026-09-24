package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func gzipTextFixture(t *testing.T, body []byte, level int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, level)
	if err != nil {
		t.Fatal(err)
	}
	writer.Name = "../../not-an-output-path.sys"
	writer.Comment = "metadata is not a clock or source label"
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func gzipTextOptions(t *testing.T, body []byte) Options {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.without-gzip-extension")
	if err := os.WriteFile(input, body, 0o400); err != nil {
		t.Fatal(err)
	}
	return Options{InputPath: input, OutputPath: filepath.Join(dir, "decoded.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")}
}

func assertGzipTextClean(t *testing.T, opts Options, published bool) {
	t.Helper()
	if entries, err := os.ReadDir(opts.RuntimeAnchor); err != nil && !os.IsNotExist(err) || len(entries) != 0 {
		t.Fatalf("private staging remains: entries=%v err=%v", entries, err)
	}
	if _, err := os.Lstat(opts.OutputPath); published && err != nil || !published && !os.IsNotExist(err) {
		t.Fatalf("unexpected publication state published=%v: %v", published, err)
	}
}

func TestGzipTextTransportPreservesCompleteBytesIdentityAndClock(t *testing.T) {
	body := []byte("# tracer: nop\n" + strings.Repeat("# 中文 annotation\n", 200) + "worker-42 ( 42) [000] d..2 91234.123456: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n")
	compressed := gzipTextFixture(t, body, gzip.DefaultCompression)
	opts := gzipTextOptions(t, compressed)
	original, err := filegeneration.FromPath(opts.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := PrepareGzipTraceText(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	sourceSHA, decodedSHA := sha256.Sum256(compressed), sha256.Sum256(body)
	if result.Profile != GzipTextTransportProfile || result.SourcePath != opts.InputPath || result.DecodedPath != opts.OutputPath ||
		result.SourceBytes != int64(len(compressed)) || result.DecodedBytes != int64(len(body)) ||
		result.SourceSHA256 != hex.EncodeToString(sourceSHA[:]) || result.DecodedSHA256 != hex.EncodeToString(decodedSHA[:]) ||
		result.SourceGeneration != original.CacheToken() {
		t.Fatalf("bad transport receipt: %+v", result)
	}
	decoded, err := os.ReadFile(result.DecodedPath)
	if err != nil || !bytes.Equal(decoded, body) {
		t.Fatalf("decoded bytes changed: %v", err)
	}
	decodedID, err := filegeneration.FromPath(result.DecodedPath)
	if err != nil || result.DecodedGeneration == "" || result.DecodedGeneration != decodedID.CacheToken() {
		t.Fatalf("decoded publication generation missing: %+v %v", result, err)
	}
	gotOriginal, err := os.ReadFile(opts.InputPath)
	if err != nil || !bytes.Equal(gotOriginal, compressed) {
		t.Fatalf("original changed: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.DecodedPath)
	if err != nil || len(idx.Events) != 1 || idx.Events[0].WakeePID != 99 || idx.Events[0].Ts != 91234.123456 {
		t.Fatalf("full tail/clock changed: idx=%+v err=%v", idx, err)
	}
	assertGzipTextClean(t, opts, true)
}

func TestGzipTextTransportDoesNotRequireTraceKeywords(t *testing.T) {
	opts := gzipTextOptions(t, gzipTextFixture(t, []byte("arbitrary valid UTF-8 text\n"), gzip.DefaultCompression))
	if _, err := PrepareGzipTraceText(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	assertGzipTextClean(t, opts, true)
}

func TestGzipTextTransportRejectsMalformedWithoutPublication(t *testing.T) {
	valid := gzipTextFixture(t, []byte("# trace text\n"), gzip.DefaultCompression)
	badCRC := append([]byte(nil), valid...)
	badCRC[len(badCRC)-8] ^= 0xff
	badSize := append([]byte(nil), valid...)
	badSize[len(badSize)-4] ^= 0xff
	badHeader := append([]byte(nil), valid...)
	badHeader[3] |= 0xe0
	for _, tc := range []struct {
		name string
		body []byte
		code string
	}{
		{"short", []byte{0x1f, 0x8b}, GzipTextCodeResourceLimit},
		{"header", badHeader, GzipTextCodeInvalidHeader},
		{"crc", badCRC, GzipTextCodeIntegrity},
		{"size", badSize, GzipTextCodeIntegrity},
		{"truncated", valid[:len(valid)-1], GzipTextCodeIntegrity},
		{"trailing", append(append([]byte(nil), valid...), 0), GzipTextCodeTrailingData},
		{"multiple-members", append(append([]byte(nil), valid...), valid...), GzipTextCodeTrailingData},
		{"empty", gzipTextFixture(t, nil, gzip.DefaultCompression), GzipTextCodeDecodedEmpty},
		{"late-nul", gzipTextFixture(t, append(bytes.Repeat([]byte("# text\n"), 20000), 0), gzip.DefaultCompression), GzipTextCodeDecodedText},
		{"utf8", gzipTextFixture(t, []byte("# text\n\xff"), gzip.DefaultCompression), GzipTextCodeDecodedText},
		{"nested-gzip", gzipTextFixture(t, valid, gzip.DefaultCompression), GzipTextCodeDecodedText},
		{"ratio", gzipTextFixture(t, bytes.Repeat([]byte("a"), 8<<20), gzip.BestCompression), GzipTextCodeResourceLimit},
		{"optional-header-budget", gzipHiperfFixtureWithName(t, []byte("# text\n"), strings.Repeat("x", int(hiperfGzipMaxOptionalFieldBytes)+1)), GzipTextCodeResourceLimit},
		{"line-limit", gzipTextFixture(t, bytes.Repeat([]byte("a"), attachment.TracePhysicalLineMaxBytes+1), gzip.NoCompression), GzipTextCodeDecodedText},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := gzipTextOptions(t, tc.body)
			result, err := PrepareGzipTraceText(context.Background(), opts)
			var rejected *GzipTextTransportError
			if result != (GzipTextTransportResult{}) || !errors.As(err, &rejected) || rejected.Code != tc.code {
				t.Fatalf("bad rejection: result=%+v err=%v", result, err)
			}
			if rejected.SourceGeneration != "" {
				t.Fatal("terminal rejection granted binary fallback receipt")
			}
			assertGzipTextClean(t, opts, false)
		})
	}
}

func TestGzipTextTransportBinaryFallbackRequiresWholeMemberIntegrity(t *testing.T) {
	for _, tc := range []struct {
		body   []byte
		format string
	}{
		{[]byte("PERFILE2\x00payload"), "linux_perf_data"},
		{[]byte("SIMPLEPERF\x00payload"), "simpleperf_report_sample_proto"},
		{[]byte("OHOSPROF\x00payload"), "openharmony_profiler"},
		{[]byte{0xce, 0x0a, 0, 0}, "harmony_rmq"},
		{[]byte{0x49, 0xdf, 0, 0}, "openharmony_raw"},
		{[]byte("SQLite format 3\x00body"), "sqlite"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			encoded := gzipTextFixture(t, tc.body, gzip.DefaultCompression)
			for _, corrupt := range []bool{false, true} {
				body := append([]byte(nil), encoded...)
				if corrupt {
					body[len(body)-8] ^= 0xff
				}
				opts := gzipTextOptions(t, body)
				result, err := PrepareGzipTraceText(context.Background(), opts)
				var rejected *GzipTextTransportError
				if result != (GzipTextTransportResult{}) || !errors.As(err, &rejected) {
					t.Fatalf("bad fallback verdict: result=%+v err=%v", result, err)
				}
				if corrupt {
					if rejected.Code != GzipTextCodeIntegrity || rejected.SourceGeneration != "" {
						t.Fatalf("corrupt binary got fallback: %+v", rejected)
					}
				} else {
					id, err := filegeneration.FromPath(opts.InputPath)
					if err != nil {
						t.Fatal(err)
					}
					digest := sha256.Sum256(body)
					if rejected.Code != GzipTextCodeDecodedBinary || rejected.DecodedFormat != tc.format || rejected.SourceBytes != int64(len(body)) || rejected.SourceSHA256 != hex.EncodeToString(digest[:]) || rejected.SourceGeneration != id.CacheToken() {
						t.Fatalf("incomplete fallback receipt: %+v", rejected)
					}
				}
				assertGzipTextClean(t, opts, false)
			}
		})
	}
}

func TestGzipTextTransportCancellationAndSourceChangeSuppressFallback(t *testing.T) {
	for _, tc := range []struct {
		name            string
		cancel, replace bool
		status          string
	}{
		{"start-cancel", true, false, ProgressStatusStarted},
		{"finish-cancel", true, false, ProgressStatusFailed},
		{"finish-source-replacement", false, true, ProgressStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := gzipTextOptions(t, gzipTextFixture(t, []byte("PERFILE2payload"), gzip.DefaultCompression))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts.Progress = func(event ProgressEvent) {
				if event.Stage != "gzip_text_decompress" || event.Status != tc.status {
					return
				}
				if tc.cancel {
					cancel()
				}
				if tc.replace {
					if err := os.Rename(opts.InputPath, opts.InputPath+".original"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(opts.InputPath, []byte("replacement"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := PrepareGzipTraceText(ctx, opts)
			var rejected *GzipTextTransportError
			if err == nil || result != (GzipTextTransportResult{}) || errors.As(err, &rejected) {
				t.Fatalf("hard failure retained fallback: result=%+v err=%v", result, err)
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			assertGzipTextClean(t, opts, false)
		})
	}
}

func TestGzipTextTransportNoReplaceAndReadOnlyOriginalParent(t *testing.T) {
	opts := gzipTextOptions(t, gzipTextFixture(t, []byte("# tracer: nop\n"), gzip.DefaultCompression))
	if err := os.WriteFile(opts.OutputPath, []byte("competitor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareGzipTraceText(context.Background(), opts); err == nil {
		t.Fatal("overwrote competitor")
	}
	if body, err := os.ReadFile(opts.OutputPath); err != nil || string(body) != "competitor" {
		t.Fatalf("competitor changed: %v", err)
	}
	opts.OutputPath = opts.InputPath
	if _, err := PrepareGzipTraceText(context.Background(), opts); err == nil {
		t.Fatal("accepted input/output collision")
	}
	parent := filepath.Dir(opts.InputPath)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	outDir := t.TempDir()
	opts.OutputPath, opts.RuntimeAnchor = filepath.Join(outDir, "decoded.systrace"), filepath.Join(outDir, "runtime")
	if _, err := PrepareGzipTraceText(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	assertGzipTextClean(t, opts, true)
}

func TestGzipTextHardFailureNeverUnwrapsBinaryAuthorization(t *testing.T) {
	verdict := &GzipTextTransportError{Code: GzipTextCodeDecodedBinary, DecodedFormat: "linux_perf_data"}
	for _, hard := range []error{context.Canceled, io.ErrClosedPipe, os.ErrPermission} {
		got := gzipTextHardFailure(verdict, hard)
		var rejected *GzipTextTransportError
		if errors.As(got, &rejected) || !errors.Is(got, hard) {
			t.Fatalf("hard error flattening lost contract: %v", got)
		}
	}
}

func TestGzipTextTransportLateHardFailureRollsBackPublishedText(t *testing.T) {
	for _, action := range []string{"cancel", "replace-source", "rewrite-source"} {
		t.Run(action, func(t *testing.T) {
			compressed := gzipTextFixture(t, []byte("# tracer: nop\n"), gzip.DefaultCompression)
			opts := gzipTextOptions(t, compressed)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts.Progress = func(event ProgressEvent) {
				if event.Stage != "gzip_text_decompress" || event.Status != ProgressStatusComplete {
					return
				}
				if _, err := os.Stat(opts.OutputPath); err != nil {
					t.Fatalf("fixture did not reach late publication gate: %v", err)
				}
				switch action {
				case "cancel":
					cancel()
				case "replace-source":
					if err := os.Rename(opts.InputPath, opts.InputPath+".old"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(opts.InputPath, compressed, 0o600); err != nil {
						t.Fatal(err)
					}
				case "rewrite-source":
					if err := os.Chmod(opts.InputPath, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(opts.InputPath, compressed, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := PrepareGzipTraceText(ctx, opts)
			var rejected *GzipTextTransportError
			if err == nil || result != (GzipTextTransportResult{}) || errors.As(err, &rejected) {
				t.Fatalf("late hard gate failed: result=%+v err=%v", result, err)
			}
			assertGzipTextClean(t, opts, false)
		})
	}
}

func TestGzipTextTransportLateNoReplacePreservesCompetitor(t *testing.T) {
	opts := gzipTextOptions(t, gzipTextFixture(t, []byte("# tracer: nop\n"), gzip.DefaultCompression))
	opts.Progress = func(event ProgressEvent) {
		if event.Stage != "gzip_text_decompress" || event.Status != ProgressStatusStarted {
			return
		}
		if err := os.WriteFile(opts.OutputPath, []byte("competitor"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := PrepareGzipTraceText(context.Background(), opts)
	var rejected *GzipTextTransportError
	if err == nil || result != (GzipTextTransportResult{}) || errors.As(err, &rejected) {
		t.Fatalf("publication collision was not a hard failure: %+v %v", result, err)
	}
	if body, err := os.ReadFile(opts.OutputPath); err != nil || string(body) != "competitor" {
		t.Fatalf("competitor changed: %q %v", body, err)
	}
	assertGzipTextClean(t, opts, true)
}

type gzipTextFaultInput struct {
	size    int64
	readErr error
}

func (input gzipTextFaultInput) Size() int64                         { return input.size }
func (input gzipTextFaultInput) DisplayPath() string                 { return "capture.gz" }
func (input gzipTextFaultInput) Validate(conversionInputStage) error { return nil }
func (input gzipTextFaultInput) ReadAt([]byte, int64) (int, error)   { return 0, input.readErr }

func TestGzipTextTransportCompressedCapAndReadFailure(t *testing.T) {
	oversized := &gzipTextCheckedInput{conversionInputView: gzipTextFaultInput{size: hiperfGzipMaxCompressedBytes + 1, readErr: errors.New("must not read oversized source")}}
	var rejected *HiperfGzipError
	if err := preflightHiperfGzipHeader(oversized); !errors.As(err, &rejected) || rejected.Code != hiperfGzipCodeResourceLimit || oversized.readErr != nil {
		t.Fatalf("compressed cap did not precede reads: %v", err)
	}
	fault := &gzipTextCheckedInput{conversionInputView: gzipTextFaultInput{size: 100, readErr: os.ErrPermission}}
	if _, _, err := inflateGzipTraceText(context.Background(), Options{}, fault, sealedConversionPublicationTarget{}, time.Time{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("source I/O failure was mislabeled: %v", err)
	} else {
		var transport *GzipTextTransportError
		if errors.As(err, &transport) {
			t.Fatalf("source I/O failure gained data-local type: %v", err)
		}
	}
}

func TestGzipTextTransportSameBytePrivateReplacementFailsCreatorBinding(t *testing.T) {
	body := []byte("# tracer: nop\n")
	opts := gzipTextOptions(t, gzipTextFixture(t, body, gzip.DefaultCompression))
	input, err := openConversionInputAuthority(opts.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	target, err := prepareSealedConversionPublicationTarget(opts.OutputPath, ".codrax-gzip-text-*", opts.RuntimeAnchor)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Cleanup()
	result, generation, err := inflateGzipTraceText(context.Background(), opts, &gzipTextCheckedInput{conversionInputView: input}, target, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target.StagingPath, target.StagingPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.StagingPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, false)
	if err != nil {
		t.Fatal(err)
	}
	defer sealed.Close()
	if err := validateGzipTextSealed(context.Background(), sealed, generation, result.DecodedPath, result.DecodedBytes, result.DecodedSHA256); err == nil {
		t.Fatal("same-byte private replacement inherited creator generation")
	}
	if _, err := os.Lstat(opts.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("private replacement published output: %v", err)
	}
}
