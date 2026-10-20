package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHMC268GZIPTextRoutingPublicCandidates(t *testing.T) {
	for _, body := range []string{
		"# tracer: nop\n# header\n worker-42 (42) [000] .... 1.000000: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n",
		" worker-42 (42) [001] d..2 2.000000: device_future_event: value=1\n",
	} {
		opts, _ := gzipRouteCapture(t, []byte(body), "not-a-trace.bin")
		got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
		if got.NativeReader != "trace_query" || got.Status != "format_candidate" || got.Container != "gzip" || got.Reason != "native_trace_preparation_required" {
			t.Fatalf("existing text reader lost named gzip navigation: %+v", got)
		}
	}
}

func TestHMC268GZIPTextRoutingPhysicalOriginAndBoundedPrefix(t *testing.T) {
	header := " worker-42 (42) [000] .... 1.000000: future_device_event: "
	for _, tc := range []struct {
		name, body string
		candidate  bool
	}{
		{"crlf_comments", "\r\n # tracer: nop\r\n\t# metadata\r\n" + header + "payload\r\n", true},
		{"long_payload", header + strings.Repeat("x", 4<<20), true},
		{"near_limit_comment", "#" + strings.Repeat("x", gzipRoutingProbeTextBytes-len(header)-3) + "\n" + header, true},
		{"header_only", "# tracer: nop\n# metadata\n", false},
		{"prose_before_event", "this document quotes a trace:\n" + header + "value\n", false},
		{"binary_before_event", "\x00\xff\n" + header + "value\n", false},
		// A comm can contain spaces and colons. Navigation must not invent a
		// prose-keyword gate stricter than the parser's physical row grammar.
		{"comm_with_spaces_and_colon", "quoted text: " + header + "value\n", true},
		{"header_missing_colon", strings.TrimSuffix(header, ": "), false},
		{"event_beyond_prefix", "#" + strings.Repeat("x", gzipRoutingProbeTextBytes) + "\n" + header, false},
		{"partial_header_at_limit", "#" + strings.Repeat("x", gzipRoutingProbeTextBytes-15) + "\n" + header, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, []byte(tc.body), "../../metadata.sys")
			before, err := os.Stat(opts.InputPath)
			if err != nil {
				t.Fatal(err)
			}
			got := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
			if (got.NativeReader == "trace_query") != tc.candidate {
				t.Fatalf("physical origin/budget mismatch: %+v", got)
			}
			if !tc.candidate && (got.Status != "unknown" || got.Reason != "container_only_domain_unknown") {
				t.Fatalf("bounded absence became unsupported: %+v", got)
			}
			after, err := os.Stat(opts.InputPath)
			if err != nil || after.Mode() != before.Mode() {
				t.Fatal("source permissions changed", err)
			}
			current, err := os.ReadFile(opts.InputPath)
			if err != nil || !bytes.Equal(current, original) {
				t.Fatal("source bytes changed", err)
			}
			if _, err := os.Stat(opts.OutputPath); !os.IsNotExist(err) {
				t.Fatal("navigation prepared output", err)
			}
			if files, err := os.ReadDir(opts.RuntimeAnchor); err == nil && len(files) != 0 {
				t.Fatal("navigation left staging", files)
			}
		})
	}
}

func TestHMC268GZIPTextRoutingDoesNotGrantCompleteTransport(t *testing.T) {
	for _, name := range []string{"valid", "crc", "isize", "truncated_tail", "trailing_bytes", "second_member", "late_nul", "late_invalid_utf8"} {
		t.Run(name, func(t *testing.T) {
			body := []byte(" worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|work\n#" + strings.Repeat("x", 4*gzipRoutingProbeTextBytes) + "\n")
			if name == "late_nul" {
				body = append(body, 0)
			}
			if name == "late_invalid_utf8" {
				body = append(body, 0xff)
			}
			opts, wire := gzipRouteCapture(t, body, "capture.txt")
			switch name {
			case "crc":
				wire[len(wire)-8] ^= 0xff
			case "isize":
				wire[len(wire)-4] ^= 0xff
			case "truncated_tail":
				wire = wire[:len(wire)-3]
			case "trailing_bytes":
				wire = append(wire, 'x')
			case "second_member":
				wire = append(wire, wire...)
			}
			if err := os.Chmod(opts.InputPath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(opts.InputPath, wire, 0400); err != nil {
				t.Fatal(err)
			}
			probe := ProbeInputRoutingCapability(context.Background(), opts.InputPath, opts.RuntimeAnchor)
			if probe.NativeReader != "trace_query" {
				t.Fatalf("navigation consumed full tail: %+v", probe)
			}
			result, err := PrepareFile(context.Background(), opts)
			if name == "valid" {
				if err != nil || result.TextTransport == nil || result.OutputBytes != int64(len(body)) || result.EventsWritten != 0 || result.BundlePath != "" {
					t.Fatalf("text transport was not complete or gained semantic authority: %+v %v", result, err)
				}
				decoded, err := os.ReadFile(result.OutputPath)
				if err != nil || !bytes.Equal(decoded, body) {
					t.Fatal("complete text changed", err)
				}
			} else {
				if err == nil || result.TextTransport != nil {
					t.Fatalf("invalid tail acquired readiness: %+v %v", result, err)
				}
				assertNoTraceArchivePublication(t, opts.OutputPath)
			}
			after, err := os.ReadFile(opts.InputPath)
			if err != nil || !bytes.Equal(after, wire) {
				t.Fatal("preparation changed source", err)
			}
		})
	}
}

func TestHMC268GZIPTextRoutingActualCompressedBudgetAndSourceGeneration(t *testing.T) {
	for _, name := range []string{"bounded_empty_blocks", "excess_empty_blocks", "cancelled", "replaced"} {
		t.Run(name, func(t *testing.T) {
			body := []byte(" worker-42 (42) [000] .... 1.000000: future_device_event: value\n")
			blocks := 100
			if name == "excess_empty_blocks" {
				blocks = int((gzipRoutingProbeCompressedBytes - 100) / 5)
			}
			wire := []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255}
			wire = append(wire, bytes.Repeat([]byte{0, 0, 0, 255, 255}, blocks)...)
			wire = append(wire, 1, byte(len(body)), 0, ^byte(len(body)), 255)
			wire = append(wire, body...)
			wire = binary.LittleEndian.AppendUint32(wire, crc32.ChecksumIEEE(body))
			wire = binary.LittleEndian.AppendUint32(wire, uint32(len(body)))
			path := filepath.Join(t.TempDir(), "trace.data")
			if err := os.WriteFile(path, wire, 0400); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var probeCtx context.Context = ctx
			if name == "cancelled" {
				probeCtx = &hmc267ProbeContext{Context: ctx, at: 3, action: cancel}
			}
			if name == "replaced" {
				probeCtx = &hmc267ProbeContext{Context: ctx, at: 2, action: func() {
					replacement := filepath.Join(filepath.Dir(path), "replacement")
					if err := os.WriteFile(replacement, wire, 0400); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, path); err != nil {
						t.Fatal(err)
					}
				}}
			}
			got := ProbeInputRoutingCapability(probeCtx, path, t.TempDir())
			if name == "bounded_empty_blocks" {
				if got.NativeReader != "trace_query" {
					t.Fatalf("bounded input lost navigation: %+v", got)
				}
			} else if got.NativeReader != "" || got.Status != "unknown" {
				t.Fatalf("budget/cancel/generation violation became navigation: %+v", got)
			}
			if name == "excess_empty_blocks" && got.Reason != "gzip_probe_input_budget" {
				t.Fatalf("compressed budget not enforced: %+v", got)
			}
		})
	}
}
