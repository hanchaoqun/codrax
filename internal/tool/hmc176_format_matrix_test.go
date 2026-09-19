package tool

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This matrix exercises legal synthetic format bytes, not device captures or
// external converter substitutes. It proves the default preparation/public
// query seam on the current host; it is not cross-platform or live-model QA.
func TestHMC176DefaultFormatCapabilityMatrix(t *testing.T) {
	hmc176DisableExternalProviders(t)
	profiler := hmc176Profiler("bytrace_plugin", []byte(hmc176ScheduleText))
	perf := hmc176PerfData(true)
	for _, tc := range []struct {
		name      string
		body      []byte
		wantKind  string
		wantType  tracequery.EventType
		wantCount int
		zipMember string
		gzipPerf  bool
	}{
		{name: "ohosprof_bytrace", body: profiler, wantKind: string(attachment.BinaryTraceFormatOHOSProfile), wantType: tracequery.EventSchedWakeup, wantCount: 1},
		{name: "perfile2_samples", body: perf, wantKind: string(attachment.BinaryTraceFormatLinuxPerf), wantType: tracequery.EventPerfSample, wantCount: 1},
		{name: "simpleperf_report_samples", body: hmc176Simpleperf(), wantKind: "simpleperf_report_sample_proto", wantType: tracequery.EventPerfSample, wantCount: 1},
		{name: "ohosprof_rootless_gzip_perfile2_samples", body: hmc176StandalonePerf(hmc176Gzip(t, perf)), wantKind: string(attachment.BinaryTraceFormatOHOSProfile), wantType: tracequery.EventPerfSample, wantCount: 1, gzipPerf: true},
		{name: "ohosprof_embedded_gzip_perfile2", body: append(hmc176Profiler("bytrace_plugin", []byte(hmc176ScheduleText)), hmc176StandalonePerf(hmc176Gzip(t, perf))...), wantKind: string(attachment.BinaryTraceFormatOHOSProfile), wantType: tracequery.EventSchedWakeup, wantCount: 1, gzipPerf: true},
		{name: "zip_ohosprof", body: hmc176Zip(t, map[string][]byte{"capture.sys": profiler, "README.txt": []byte("capture notes")}), wantKind: "zip", wantType: tracequery.EventSchedWakeup, wantCount: 1, zipMember: "capture.sys"},
		{name: "zip_rmq", body: hmc176Zip(t, map[string][]byte{"nested/capture.htrace": hmc17NamedBinaryTailFixture()}), wantKind: "zip", wantType: tracequery.EventSchedWakeup, wantCount: 12, zipMember: "nested/capture.htrace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, preparer, conversions := hmc17NamedPathContext(t)
			path := hmc176WriteSource(t, tc.body)
			bus.AttachedHitrace = hmc176UnrelatedText
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100})
			payload := hmc17NamedPayload(t, result)
			if len(payload.Events) != tc.wantCount || payload.EventCount != tc.wantCount {
				t.Fatalf("wrong exact event capability: count=%d rows=%d want=%d", payload.EventCount, len(payload.Events), tc.wantCount)
			}
			for _, event := range payload.Events {
				if event.Type != tc.wantType || event.SourcePath == path || !filepath.IsAbs(event.SourcePath) {
					t.Fatalf("wrong event capability or original binary used as text coordinates: %+v", event)
				}
			}
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || conversions.Load() != 1 {
				t.Fatalf("format transport lost source/derived separation or reused conversion: material=%+v conversions=%d", material, conversions.Load())
			}
			if payload.SourcePath != material.QueryPath() {
				t.Fatalf("public result did not use complete prepared material: %q != %q", payload.SourcePath, material.QueryPath())
			}
			var receipt struct {
				Version          string             `json:"version"`
				SourcePath       string             `json:"source_path"`
				SourceKind       string             `json:"source_kind"`
				SourceBytes      int64              `json:"source_bytes"`
				SourceSHA256     string             `json:"source_sha256"`
				SourceGeneration string             `json:"source_generation"`
				QueryPath        string             `json:"query_path"`
				Conversion       hitraceconv.Result `json:"conversion"`
			}
			body, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &receipt); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(tc.body)
			if receipt.Version != "traceinput-v1" || receipt.SourcePath != path || receipt.SourceKind != tc.wantKind || receipt.SourceBytes != int64(len(tc.body)) || receipt.SourceSHA256 != hex.EncodeToString(digest[:]) || receipt.SourceGeneration == "" || receipt.QueryPath != material.QueryPath() {
				t.Fatalf("inaccurate format/source receipt: %+v", receipt)
			}
			for _, artifact := range receipt.Conversion.Artifacts {
				if artifact.Path == path { // Direct perf preserves the original as provenance.
					continue
				}
				rel, err := filepath.Rel(filepath.Dir(material.QueryPath()), artifact.Path)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					t.Fatalf("derived artifact escaped managed output directory: path=%q err=%v", artifact.Path, err)
				}
			}
			if tc.zipMember != "" {
				archive := receipt.Conversion.ArchiveProvenance
				if archive == nil || archive.Member != tc.zipMember || archive.Selection != "unique_candidate" || archive.ArchiveSHA256 != receipt.SourceSHA256 || archive.ArchiveBytes != receipt.SourceBytes || archive.MemberSHA256 == "" || archive.MemberBytes <= 0 {
					t.Fatalf("ZIP member selection was not preserved: %+v", archive)
				}
			}
			if tc.wantType == tracequery.EventPerfSample {
				if hitraceconv.QueryReadySystracePath(receipt.Conversion) != "" || hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts) == "" {
					t.Fatalf("sample-only capture was promoted to scheduling capability: %+v", receipt.Conversion)
				}
			} else if hitraceconv.QueryReadySystracePath(receipt.Conversion) == "" || (!tc.gzipPerf && hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts) != "") {
				t.Fatalf("scheduling-only capture acquired nonexistent sample capability: %+v", receipt.Conversion)
			}
			if tc.wantType == tracequery.EventSchedWakeup {
				missingStreamer, builtinReady := false, false
				for _, decision := range receipt.Conversion.TraceDecisions {
					missingStreamer = missingStreamer || decision.ProviderName == "trace_streamer_db" && decision.Reason == "trace_streamer_unavailable" && !decision.Attempted && !decision.TraceQueryReady
					builtinReady = builtinReady || decision.Fallback && decision.Succeeded && decision.TraceQueryReady
				}
				if !missingStreamer || !builtinReady {
					t.Fatalf("missing external provider was not accurately distinguished from working built-in capability: %+v", receipt.Conversion.TraceDecisions)
				}
			}
			if tc.gzipPerf {
				perfPath := hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts)
				if perfPath == "" {
					t.Fatal("embedded HIPERF gzip did not publish a query-ready sample artifact")
				}
				foundTransform, correctClockReceipt := false, false
				for _, artifact := range receipt.Conversion.Artifacts {
					if artifact.Path == perfPath && artifact.PerfTransform != nil {
						foundTransform = true
						decoded := sha256.Sum256(perf)
						compressed := sha256.Sum256(hmc176Gzip(t, perf))
						if artifact.PerfTransform.SourceSHA256 != hex.EncodeToString(compressed[:]) || artifact.PerfTransform.DecodedSHA256 != hex.EncodeToString(decoded[:]) || artifact.Perf == nil || artifact.Perf.InputFormat != "gzip_perf_data" || !artifact.Perf.TraceQueryReady {
							t.Fatalf("gzip perf transform lost encoded/decoded identity or capability: %+v", artifact)
						}
					}
				}
				for _, artifact := range payload.TraceArtifacts {
					if artifact.SourcePath == perfPath {
						if tc.wantType == tracequery.EventPerfSample {
							correctClockReceipt = artifact.CausalCompatible && artifact.ClockAlignment == tracequery.TraceClockAlignmentIdentity
						} else {
							correctClockReceipt = !artifact.CausalCompatible && artifact.ClockAlignment == tracequery.TraceClockAlignmentIsolated && artifact.IsolationReason != ""
						}
					}
				}
				if !foundTransform || !correctClockReceipt {
					t.Fatalf("HIPERF sample transport must retain exact transform and clock receipts: transform=%t clock=%t artifacts=%+v", foundTransform, correctClockReceipt, payload.TraceArtifacts)
				}
				// Equal numeric timestamps do not authorize a cross-clock join.
				// The receipt-approved sample artifact remains queryable on its
				// own clock through the same public tool.
				samples := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": perfPath, "view": "event_search"}))
				if len(samples.Events) != 1 || samples.Events[0].Type != tracequery.EventPerfSample || samples.EventCount != 1 || conversions.Load() != 1 {
					t.Fatalf("isolated sample capability was lost or capture reconverted: events=%+v conversions=%d", samples.Events, conversions.Load())
				}
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named-path query replaced unrelated attachment")
			}
			again := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100})
			if !again.Success || conversions.Load() != 1 {
				t.Fatalf("warm public query lost prepared capability or reconverted: conversions=%d summary=%s", conversions.Load(), again.Summary)
			}
			hmc176AssertOriginal(t, path, tc.body)
		})
	}
}

func TestHMC176DefaultFormatFailuresNeverBorrowAttachment(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, tc := range []struct {
		name string
		body []byte
		want string
	}{
		{name: "ohosprof_unknown_plugin_inventory", body: hmc176Profiler("future-plugin", []byte{8, 1}), want: "no_query_ready_material"},
		{name: "perfile2_no_samples", body: hmc176PerfData(false), want: "no_query_ready_material"},
		{name: "zip_multiple_members", body: hmc176Zip(t, map[string][]byte{"a.sys": hmc176Profiler("bytrace_plugin", []byte(hmc176ScheduleText)), "b.htrace": hmc17NamedBinaryTailFixture()}), want: "multiple_trace_members"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, preparer, conversions := hmc17NamedPathContext(t)
			path := hmc176WriteSource(t, tc.body)
			bus.AttachedHitrace = hmc176UnrelatedText
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
			if result.Success || len(result.Observations) != 0 || result.RawRef != "" || !strings.Contains(result.Summary, tc.want) {
				t.Fatalf("unsupported/inventory input gained query evidence or lost reason: %+v", result)
			}
			material, err := preparer.Prepare(context.Background(), path)
			if material != nil || err == nil || !strings.Contains(err.Error(), tc.want) || conversions.Load() != 2 {
				t.Fatalf("failed preparation minted or cached usable material: material=%+v err=%v", material, err)
			}
			if matches, err := filepath.Glob(filepath.Join(bus.WorkDir, ".codrax", "trace-input-*")); err != nil || len(matches) != 0 {
				t.Fatalf("failed preparation retained managed artifacts: paths=%v err=%v", matches, err)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("failed named source changed unrelated attachment")
			}
			hmc176AssertOriginal(t, path, tc.body)
		})
	}
}

const hmc176ScheduleText = "matrix-worker-42 (42) [000] .... 1.000000: sched_wakeup: comm=matrix-target pid=43 prio=120 target_cpu=000\n"
const hmc176UnrelatedText = "unrelated-worker-99 (99) [000] .... 9.000000: sched_wakeup: comm=unrelated-target pid=100 prio=120 target_cpu=000\n"

func hmc176DisableExternalProviders(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CODRAX_TRACE_STREAMER", "CODRAX_HIPERF_HOST", "CODRAX_SIMPLEPERF_REPORT_SAMPLE"} {
		t.Setenv(key, filepath.Join(t.TempDir(), "missing-provider"))
	}
}

func hmc176WriteSource(t *testing.T, body []byte) string {
	t.Helper()
	// A misleading extension makes content authority part of every case.
	path := filepath.Join(t.TempDir(), "capture bytes 原件.txt")
	if err := os.WriteFile(path, body, 0400); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc176AssertOriginal(t *testing.T, path string, original []byte) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original capture modified: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 1 {
		t.Fatalf("derived artifacts written beside original: entries=%v err=%v", entries, err)
	}
}

// Matches the legal OHOSPROF framing in hitraceconv/convert_test.go. The
// payload is an actual plugin protobuf, not a converter-produced text stub.
func hmc176Profiler(plugin string, data []byte) []byte {
	message := hmc176ProtoBytes(1, []byte(plugin))
	message = append(message, hmc176ProtoVarint(2, 0)...)
	message = append(message, hmc176ProtoBytes(3, data)...)
	message = append(message, hmc176ProtoVarint(4, 7)...)
	message = append(message, hmc176ProtoVarint(5, 1)...)
	message = append(message, hmc176ProtoVarint(6, 2)...)
	message = append(message, hmc176ProtoBytes(7, []byte("1.02"))...)
	body := make([]byte, 1024+4+len(message))
	copy(body, "OHOSPROF")
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[20:24], 2)
	binary.LittleEndian.PutUint32(body[1024:1028], uint32(len(message)))
	copy(body[1028:], message)
	digest := sha256.Sum256(body[1024:])
	copy(body[24:56], digest[:])
	return body
}

// OHOSPROF data_type=HIPERF is the existing bounded gzip-perf lane. A
// top-level gzip(perf.data) is a separate, currently unimplemented route and
// is deliberately not recorded here as an intentionally unsupported format.
func hmc176StandalonePerf(data []byte) []byte {
	body := make([]byte, 1024+len(data))
	copy(body, "OHOSPROF")
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[56:60], 1)
	copy(body[108:236], "hiperf-plugin")
	copy(body[236:244], "1.0")
	copy(body[1024:], data)
	digest := sha256.Sum256(data)
	copy(body[24:56], digest[:])
	return body
}

func hmc176ProtoVarint(field int, value uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(field<<3)), value)
}

func hmc176ProtoBytes(field int, body []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3|2))
	out = binary.AppendUvarint(out, uint64(len(body)))
	return append(out, body...)
}

// Minimal PERFILE2 perf_event_attr + PERF_RECORD_SAMPLE, matching the
// admission fixture in raw_perf_normalization_identity_a1a_test.go.
func hmc176PerfData(sample bool) []byte {
	const headerSize, attrSize = 104, 48
	const sampleType = 1<<0 | 1<<1 | 1<<2 | 1<<7 | 1<<8 // IP/TID/TIME/CPU/PERIOD
	out := make([]byte, headerSize+attrSize)
	copy(out, "PERFILE2")
	for offset, value := range map[int]uint64{8: headerSize, 16: attrSize, 24: headerSize, 32: attrSize, 40: headerSize + attrSize} {
		binary.LittleEndian.PutUint64(out[offset:offset+8], value)
	}
	binary.LittleEndian.PutUint32(out[headerSize+4:], 40)
	binary.LittleEndian.PutUint64(out[headerSize+24:], sampleType)
	if sample {
		record := make([]byte, 48)
		binary.LittleEndian.PutUint32(record, 9)
		binary.LittleEndian.PutUint16(record[6:], uint16(len(record)))
		binary.LittleEndian.PutUint64(record[8:], 0x1234)
		binary.LittleEndian.PutUint32(record[16:], 42)
		binary.LittleEndian.PutUint32(record[20:], 42)
		binary.LittleEndian.PutUint64(record[24:], 1_000_000_000)
		binary.LittleEndian.PutUint32(record[32:], 0)
		binary.LittleEndian.PutUint64(record[40:], 99)
		binary.LittleEndian.PutUint64(out[48:], uint64(len(record)))
		out = append(out, record...)
	}
	return out
}

// SIMPLEPERF report-sample protobuf framing from traceinput/prepare_test.go.
func hmc176Simpleperf() []byte {
	var out bytes.Buffer
	out.WriteString("SIMPLEPERF\x01\x00")
	message := func(field int, parts ...[]byte) []byte {
		return hmc176ProtoBytes(field, bytes.Join(parts, nil))
	}
	record := func(body []byte) {
		var size [4]byte
		binary.LittleEndian.PutUint32(size[:], uint32(len(body)))
		out.Write(size[:])
		out.Write(body)
	}
	record(message(5, message(1, []byte("cpu-cycles"))))
	record(message(3, hmc176ProtoVarint(1, 0), message(2, []byte("/system/lib64/libfoo.so")), message(3, []byte("main"))))
	record(message(4, hmc176ProtoVarint(1, 42), hmc176ProtoVarint(2, 42), message(3, []byte("matrix-worker"))))
	record(message(1, hmc176ProtoVarint(1, 1_000_000_000), hmc176ProtoVarint(2, 42), message(3, hmc176ProtoVarint(1, 0x1234), hmc176ProtoVarint(2, 0), hmc176ProtoVarint(3, 0)), hmc176ProtoVarint(4, 99), hmc176ProtoVarint(5, 0)))
	out.Write(make([]byte, 4))
	return out.Bytes()
}

func hmc176Gzip(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func hmc176Zip(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := members[name]
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
