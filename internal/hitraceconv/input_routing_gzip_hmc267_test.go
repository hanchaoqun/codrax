package hitraceconv

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Public discovery must offer the already-supported inner reader without
// manufacturing a prepared receipt or trusting the gzip Name/extension.
func TestHMC267GZIPRoutingPublicKnownBinaryCandidates(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"rmq", traceArchiveTestBuiltinBody()},
		{"linux_perf", syntheticRawPerfData()},
		{"simpleperf", syntheticSimpleperfProtoStream(false, false)},
		{"ohos_profile", syntheticProfilerTraceRoot()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, tc.body, "../../not-a-format.txt")
			before, err := os.Stat(opts.InputPath)
			if err != nil {
				t.Fatal(err)
			}
			got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
			if got.Container != "gzip" || got.Status != "format_candidate" || got.NativeReader != "trace_query" || got.Reason != "native_trace_preparation_required" || got.ArchiveCandidates != nil || len(got.CandidateViews) != 0 {
				t.Fatalf("ordinary-named supported gzip lost bounded navigation: %+v", got)
			}
			after, err := os.Stat(opts.InputPath)
			if err != nil || before.Mode() != after.Mode() {
				t.Fatal("source mode changed", err)
			}
			body, err := os.ReadFile(opts.InputPath)
			if err != nil || !bytes.Equal(original, body) {
				t.Fatal("source bytes changed", err)
			}
			if _, err := os.Stat(opts.OutputPath); !os.IsNotExist(err) {
				t.Fatal("probe published prepared output", err)
			}
			if files, err := os.ReadDir(opts.RuntimeAnchor); err == nil && len(files) != 0 {
				t.Fatal("probe left staging", files)
			}
		})
	}
}

func TestHMC267GZIPRoutingUnknownAndNestedDoNotGainReaders(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"unknown", []byte("a runtime document, not a binary capture")},
		{"trace_text", []byte("# tracer: nop\n task-42 (42) [000] .... 1.0: tracing_mark_write: B|42|work\n")},
		{"sqlite", append([]byte("SQLite format 3\x00"), make([]byte, 64)...)},
		{"nested_zip", append([]byte("PK\x03\x04"), make([]byte, 64)...)},
		{"nested_gzip", append([]byte{0x1f, 0x8b, 8, 0}, make([]byte, 64)...)},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, _ := gzipRouteCapture(t, tc.body, "PERFILE2.data")
			got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
			if got.NativeReader != "" || got.Status != "unknown" || got.Reason != "container_only_domain_unknown" {
				t.Fatalf("metadata/unknown payload gained reader: %+v", got)
			}
		})
	}
}

// Valid gzip with arbitrarily many zero-output blocks. The native signature is
// small, but reaching it must still account for all actual compressed reads.
func hmc267EmptyBlockGZIP(blocks int) []byte {
	body := append([]byte("PERFILE2"), make([]byte, conversionInputProbeSize-8)...)
	wire := []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255}
	wire = append(wire, bytes.Repeat([]byte{0, 0, 0, 255, 255}, blocks)...)
	wire = append(wire, 1, byte(len(body)), 0, ^byte(len(body)), 255)
	wire = append(wire, body...)
	wire = binary.LittleEndian.AppendUint32(wire, crc32.ChecksumIEEE(body))
	return binary.LittleEndian.AppendUint32(wire, uint32(len(body)))
}

func TestHMC267GZIPRoutingCompressedInputBudgetIncludesOuterProbe(t *testing.T) {
	for _, tc := range []struct {
		name     string
		blocks   int
		rejected bool
	}{
		{"bounded", 100, false},
		{"exceeds_total_with_outer_prefix", int((gzipRoutingProbeCompressedBytes - 100) / 5), true},
		{"zero_output_attack", int(gzipRoutingProbeCompressedBytes/5) + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "capture.data")
			if err := os.WriteFile(path, hmc267EmptyBlockGZIP(tc.blocks), 0600); err != nil {
				t.Fatal(err)
			}
			got := ProbeInputRoutingCapability(context.Background(), path, t.TempDir())
			if tc.rejected {
				if got.NativeReader != "" || got.Status != "unknown" || got.Reason != "gzip_probe_input_budget" {
					t.Fatalf("compressed input cap bypass: %+v", got)
				}
			} else if got.NativeReader != "trace_query" {
				t.Fatalf("bounded prefix lost: %+v", got)
			}
		})
	}
}

func TestHMC267GZIPRoutingNavigationDoesNotGrantTransportIntegrity(t *testing.T) {
	for _, name := range []string{"crc", "isize", "truncated_tail", "trailing_bytes", "second_member", "signature_only"} {
		t.Run(name, func(t *testing.T) {
			body := traceArchiveTestBuiltinBody()
			if name == "signature_only" {
				body = append([]byte("PERFILE2"), make([]byte, 64)...)
			}
			opts, wire := gzipRouteCapture(t, body, "trusted-looking.data")
			switch name {
			case "crc":
				wire[len(wire)-8] ^= 0xff
			case "isize":
				wire[len(wire)-4] ^= 0xff
			case "truncated_tail":
				wire = wire[:len(wire)-3]
			case "trailing_bytes":
				wire = append(wire, []byte("extra")...)
			case "second_member":
				wire = append(wire, wire...)
			}
			if err := os.Chmod(opts.InputPath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(opts.InputPath, wire, 0400); err != nil {
				t.Fatal(err)
			}
			got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
			if got.NativeReader != "trace_query" || got.Status != "format_candidate" {
				t.Fatalf("prefix probe unexpectedly claimed full validation: %+v", got)
			}
			opts.TraceEngine = traceEngineBuiltin
			result, err := PrepareFile(context.Background(), opts)
			if QueryReadySystracePath(result) != "" || QueryReadyPerfTracePath(result.Artifacts) != "" {
				t.Fatalf("prefix granted readiness: %+v err=%v", result, err)
			}
			if name == "signature_only" {
				// Preserving an honest inventory-only bundle is allowed. It must
				// not be confused with a queryable capture or measured zero rows.
				if err != nil || result.BundlePath == "" {
					t.Fatalf("expected preserved unavailable binary inventory: %+v %v", result, err)
				}
				if _, err := tracequery.BuildIndex(context.Background(), result.BundlePath); err == nil {
					t.Fatal("signature-only inventory became native query authority")
				}
			} else {
				if err == nil {
					t.Fatal("invalid transport unexpectedly prepared")
				}
				assertNoTraceArchivePublication(t, opts.OutputPath)
			}
			after, err := os.ReadFile(opts.InputPath)
			if err != nil || !bytes.Equal(wire, after) {
				t.Fatal("failed query changed source", err)
			}
		})
	}
}

type hmc267ProbeContext struct {
	context.Context
	checks, at int
	action     func()
}

func (c *hmc267ProbeContext) Err() error {
	c.checks++
	if c.checks == c.at {
		c.action()
	}
	return c.Context.Err()
}

func TestHMC267GZIPRoutingCancellationAndGenerationAreNotNavigation(t *testing.T) {
	for _, name := range []string{"cancelled", "mid_probe_cancel", "same_bytes_replaced", "malformed_header"} {
		t.Run(name, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, traceArchiveTestBuiltinBody(), "capture.data")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var probeCtx context.Context = ctx
			switch name {
			case "cancelled":
				cancel()
			case "mid_probe_cancel":
				probeCtx = &hmc267ProbeContext{Context: ctx, at: 3, action: cancel}
			case "same_bytes_replaced":
				probeCtx = &hmc267ProbeContext{Context: ctx, at: 2, action: func() {
					replacement := filepath.Join(filepath.Dir(opts.InputPath), "replacement")
					if err := os.WriteFile(replacement, original, 0400); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, opts.InputPath); err != nil {
						t.Fatal(err)
					}
				}}
			case "malformed_header":
				original[2] = 0
				if err := os.Chmod(opts.InputPath, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(opts.InputPath, original, 0400); err != nil {
					t.Fatal(err)
				}
			}
			got := ProbeInputRoutingCapability(probeCtx, opts.InputPath, opts.RuntimeAnchor)
			if got.NativeReader != "" || got.Status != "unknown" {
				t.Fatalf("cancelled/stale/invalid input kept reader: %+v", got)
			}
		})
	}
}

func TestHMC267GZIPRoutingPrefixDoesNotConsumeFullDecodedBody(t *testing.T) {
	// A valid prefix precedes many highly-compressible bytes. Navigation remains
	// bounded; full preparation, unlike the probe, must validate the tail.
	body := append([]byte("PERFILE2"), make([]byte, 4<<20)...)
	opts, wire := gzipRouteCapture(t, body, "capture.data")
	wire[len(wire)-8] ^= 0xff
	if err := os.Chmod(opts.InputPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.InputPath, wire, 0400); err != nil {
		t.Fatal(err)
	}
	got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
	if got.NativeReader != "trace_query" {
		t.Fatalf("bounded prefix scanned decoded tail: %+v", got)
	}
	// Independent standard-library validation confirms the injected CRC defect.
	r, err := gzip.NewReader(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, r)
	if err != gzip.ErrChecksum {
		t.Fatalf("fixture did not contain bad CRC: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
