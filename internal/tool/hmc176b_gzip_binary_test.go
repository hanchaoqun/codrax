package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Real legal format bytes exercise the default Coordinator and public tool;
// these are local synthetic acceptance tests, not device or live-provider QA.
func TestHMC176BGzipBinaryDefaultPublicMatrix(t *testing.T) {
	hmc176DisableExternalProviders(t)
	perf := hmc176PerfData(true)
	embeddedPerf := hmc176Gzip(t, perf)
	profiler := hmc176Profiler("bytrace_plugin", []byte(strings.Repeat(hmc176ScheduleText, 30)+hmc176bTailText))
	for _, tc := range []struct {
		name      string
		decoded   []byte
		wantType  tracequery.EventType
		wantCount int
		format    string
		embedded  bool
		tail      string
		tailStart float64
		tailEnd   float64
	}{
		{name: "perfile2_samples", decoded: perf, wantType: tracequery.EventPerfSample, wantCount: 1, format: "linux_perf_data"},
		{name: "simpleperf_samples", decoded: hmc176Simpleperf(), wantType: tracequery.EventPerfSample, wantCount: 1, format: "simpleperf_report_sample_proto"},
		{name: "rmq_complete_tail", decoded: hmc17NamedBinaryTailFixture(), wantType: tracequery.EventSchedWakeup, wantCount: 12, format: "harmony_rmq", tail: "tail-target", tailStart: 1.0105, tailEnd: 1.012},
		{name: "ohosprof_complete_tail", decoded: profiler, wantType: tracequery.EventSchedWakeup, wantCount: 31, format: "openharmony_profiler", tail: "sealed-tail", tailStart: 1.999, tailEnd: 2.001},
		{name: "ohosprof_embedded_gzip_perf", decoded: append(append([]byte(nil), profiler...), hmc176StandalonePerf(embeddedPerf)...), wantType: tracequery.EventSchedWakeup, wantCount: 31, format: "openharmony_profiler", embedded: true, tail: "sealed-tail", tailStart: 1.999, tailEnd: 2.001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := hmc176Gzip(t, tc.decoded)
			path := hmc176WriteSource(t, original)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			params := map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100}
			payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if payload.EventCount != tc.wantCount || len(payload.Events) != tc.wantCount {
				t.Fatalf("wrong exact event capability: count=%d rows=%d want=%d", payload.EventCount, len(payload.Events), tc.wantCount)
			}
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || payload.SourcePath != material.QueryPath() || conversions.Load() != 1 {
				t.Fatalf("transport lost original/derived identity or prepared reuse: material=%+v conversions=%d", material, conversions.Load())
			}
			managed := filepath.Dir(material.QueryPath())
			for _, event := range payload.Events {
				if event.Type != tc.wantType || event.SourcePath == path {
					t.Fatalf("wrong event capability or compressed source used as text coordinates: %+v", event)
				}
				hmc176bAssertManaged(t, managed, event.SourcePath)
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
			body, err := os.ReadFile(filepath.Join(managed, "preparation.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &receipt); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(original)
			if receipt.Version != "traceinput-v1" || receipt.SourcePath != path || receipt.SourceKind != "gzip" || receipt.SourceBytes != int64(len(original)) || receipt.SourceSHA256 != hex.EncodeToString(digest[:]) || receipt.SourceGeneration == "" || receipt.QueryPath != material.QueryPath() || receipt.Conversion.InputPath != path || receipt.Conversion.InputBytes != receipt.SourceBytes || receipt.Conversion.TextTransport != nil {
				t.Fatalf("transport relabeled outer source or lost complete identity: %+v", receipt)
			}
			transport := receipt.Conversion.GzipInputProvenance
			decodedDigest := sha256.Sum256(tc.decoded)
			if transport == nil || transport.Profile != "gzip_input_v1" || transport.SourceBytes != receipt.SourceBytes || transport.SourceSHA256 != receipt.SourceSHA256 || transport.SourceGeneration != receipt.SourceGeneration || transport.DecodedFormat != tc.format || transport.DecodedBytes != int64(len(tc.decoded)) || transport.DecodedSHA256 != hex.EncodeToString(decodedDigest[:]) || transport.DecodedGeneration == "" {
				t.Fatalf("generic transport lost exact outer/decoded generation: %+v", transport)
			}
			if err := tracebundle.ValidateGzipInputProvenance(transport); err != nil {
				t.Fatalf("producer emitted invalid generic transport receipt: %v", err)
			}
			bundle := hmc176bReadBundle(t, material.QueryPath())
			if !reflect.DeepEqual(bundle.GzipInputProvenance, transport) || bundle.ArchiveProvenance != nil {
				t.Fatalf("bundle changed transport identity or invented archive provenance: %+v", bundle)
			}
			for _, artifact := range receipt.Conversion.Artifacts {
				if artifact.Path != path {
					hmc176bAssertManaged(t, managed, artifact.Path)
				}
				// Generic outer gzip is not an OHOSPROF HIPERF segment and
				// must not borrow the narrower gzip_perf_data authority.
				if !tc.embedded && (artifact.PerfTransform != nil || artifact.Standalone != nil) {
					t.Fatalf("outer transport manufactured standalone perf authority: %+v", artifact)
				}
			}
			systrace := hitraceconv.QueryReadySystracePath(receipt.Conversion)
			perfPath := hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts)
			if tc.wantType == tracequery.EventPerfSample {
				if systrace != "" || perfPath == "" {
					t.Fatalf("sample-only transport gained scheduling authority: %+v", receipt.Conversion)
				}
				if len(bundle.Artifacts) != 1 || bundle.Artifacts[0].Type != hitraceconv.ArtifactPerfTrace {
					t.Fatalf("container transport was promoted to a raw or causal child: %+v", bundle.Artifacts)
				}
			} else if systrace == "" || (!tc.embedded && perfPath != "") {
				t.Fatalf("scheduling transport gained or lost capability: %+v", receipt.Conversion)
			}
			if tc.tail != "" {
				if strings.Contains(material.Preview(), tc.tail) {
					t.Fatal("tail fixture no longer tests bytes beyond bounded preview")
				}
				tailResult := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": tc.tail, "pid": 424242, "time_start": tc.tailStart, "time_end": tc.tailEnd, "limit": 20})
				tail := hmc17NamedPayload(t, tailResult)
				if len(tail.Events) != 1 || !strings.Contains(tailResult.Summary, tc.tail) || tail.TimeStart != tc.tailStart || tail.TimeEnd != tc.tailEnd {
					t.Fatalf("full decoded tail or explicit window was lost: %+v", tail)
				}
			}
			if tc.embedded {
				if perfPath == "" {
					t.Fatal("embedded HIPERF gzip lost sample capability")
				}
				foundTransform, isolated := false, false
				for _, artifact := range receipt.Conversion.Artifacts {
					if artifact.Path == perfPath && artifact.PerfTransform != nil {
						foundTransform = true
						encodedHash, decodedHash := sha256.Sum256(embeddedPerf), sha256.Sum256(perf)
						if artifact.PerfTransform.SourceSHA256 != hex.EncodeToString(encodedHash[:]) || artifact.PerfTransform.DecodedSHA256 != hex.EncodeToString(decodedHash[:]) || artifact.Standalone != nil || artifact.Perf == nil || artifact.Perf.InputFormat != "gzip_perf_data" || !artifact.Perf.TraceQueryReady {
							t.Fatalf("outer gzip replaced exact embedded perf transform authority: %+v", artifact)
						}
						foundSource := false
						for _, source := range receipt.Conversion.Artifacts {
							if source.Path == artifact.PerfTransform.SourceArtifactPath && source.Standalone != nil && source.SHA256 == artifact.PerfTransform.SourceSHA256 && source.Bytes == int64(len(embeddedPerf)) {
								foundSource = true
							}
						}
						if !foundSource {
							t.Fatal("embedded transform lost its exact standalone raw source receipt")
						}
					}
				}
				for _, artifact := range payload.TraceArtifacts {
					if artifact.SourcePath == perfPath {
						isolated = !artifact.CausalCompatible && artifact.ClockAlignment == tracequery.TraceClockAlignmentIsolated && artifact.IsolationReason != ""
					}
				}
				if !foundTransform || !isolated {
					t.Fatalf("numeric clock equality gained causal authority or lost exact transform: transform=%t isolated=%t", foundTransform, isolated)
				}
				samples := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": perfPath, "view": "event_search"}))
				if samples.EventCount != 1 || len(samples.Events) != 1 || samples.Events[0].Type != tracequery.EventPerfSample {
					t.Fatalf("isolated sample child is no longer independently queryable: %+v", samples)
				}
			}
			if again := hmc17NamedQuery(t, bus, params); !again.Success || conversions.Load() != 1 {
				t.Fatalf("warm query reconverted or lost capability: conversions=%d result=%+v", conversions.Load(), again)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named-path query replaced unrelated attachment")
			}
			hmc176AssertOriginal(t, path, original)
		})
	}
}

func TestHMC176BZipPerfSourceIsContainerNotRawArtifact(t *testing.T) {
	hmc176DisableExternalProviders(t)
	perf := hmc176PerfData(true)
	original := hmc176Zip(t, map[string][]byte{"nested/capture.sys": perf})
	path := hmc176WriteSource(t, original)
	bus, preparer, conversions := hmc17NamedPathContext(t)
	bus.AttachedHitrace = hmc176UnrelatedText
	params := map[string]any{"source": "path", "path": path, "view": "event_search"}
	payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
	if payload.EventCount != 1 || len(payload.Events) != 1 || payload.Events[0].Type != tracequery.EventPerfSample {
		t.Fatalf("ZIP perf capability changed: %+v", payload)
	}
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	bundle := hmc176bReadBundle(t, material.QueryPath())
	outerHash, innerHash := sha256.Sum256(original), sha256.Sum256(perf)
	archive := bundle.ArchiveProvenance
	if archive == nil || archive.Format != "zip" || archive.Member != "nested/capture.sys" || archive.ArchiveBytes != int64(len(original)) || archive.ArchiveSHA256 != hex.EncodeToString(outerHash[:]) || archive.MemberBytes != int64(len(perf)) || archive.MemberSHA256 != hex.EncodeToString(innerHash[:]) || archive.Selection != "unique_candidate" || bundle.GzipInputProvenance != nil {
		t.Fatalf("ZIP provenance was lost or borrowed gzip authority: %+v", bundle)
	}
	if len(bundle.Artifacts) != 1 || bundle.Artifacts[0].Type != hitraceconv.ArtifactPerfTrace || bundle.Systrace != "" || bundle.Artifacts[0].PerfTransform != nil || bundle.Artifacts[0].Standalone != nil {
		t.Fatalf("outer ZIP path with inner byte size was promoted to raw/causal child: %+v", bundle.Artifacts)
	}
	for _, artifact := range payload.TraceArtifacts {
		if artifact.SourcePath == path {
			t.Fatalf("encoded ZIP became a causal text coordinate: %+v", artifact)
		}
		hmc176bAssertManaged(t, filepath.Dir(material.QueryPath()), artifact.SourcePath)
	}
	if again := hmc17NamedQuery(t, bus, params); !again.Success || conversions.Load() != 1 {
		t.Fatalf("ZIP sample query failed reuse: conversions=%d result=%+v", conversions.Load(), again)
	}
	if material.SourcePath() != path || material.QueryPath() == path || material.SelfContainedText() || bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
		t.Fatal("ZIP query lost original identity or replaced unrelated attachment")
	}
	hmc176AssertOriginal(t, path, original)
}

func TestHMC176BGzipBinaryRejectionsNeverPublishOrBorrow(t *testing.T) {
	hmc176DisableExternalProviders(t)
	perfGzip := hmc176Gzip(t, hmc176PerfData(true))
	badCRC := append([]byte(nil), perfGzip...)
	badCRC[len(badCRC)-8] ^= 1
	for _, tc := range []struct {
		name string
		body []byte
		want string
	}{
		{name: "bad_crc_after_valid_perf", body: badCRC, want: "gzip_integrity_failed"},
		{name: "concatenated_members", body: append(append([]byte(nil), perfGzip...), perfGzip...), want: "gzip_trailing_data"},
		{name: "nested_gzip", body: hmc176Gzip(t, perfGzip), want: "gzip"},
		{name: "nested_zip", body: hmc176Gzip(t, hmc176Zip(t, map[string][]byte{"capture.sys": hmc176PerfData(true)})), want: "gzip"},
		{name: "sqlite_is_not_trace_binary", body: hmc176Gzip(t, append([]byte("SQLite format 3\x00"), make([]byte, 64)...)), want: "gzip"},
		{name: "unknown_binary", body: hmc176Gzip(t, []byte{0, 1, 2, 3, 4, 0xff}), want: "gzip"},
		{name: "perf_inventory_only", body: hmc176Gzip(t, hmc176PerfData(false)), want: "no_query_ready_material"},
		{name: "unknown_plugin_inventory_only", body: hmc176Gzip(t, hmc176Profiler("future-plugin", []byte{8, 1})), want: "no_query_ready_material"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := hmc176WriteSource(t, tc.body)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
			if result.Success || len(result.Observations) != 0 || result.RawRef != "" || !strings.Contains(result.Summary, tc.want) || strings.Contains(result.Summary, "invalid_magic") {
				t.Fatalf("bad transport/inventory acquired evidence or semantic fallback: %+v", result)
			}
			material, err := preparer.Prepare(context.Background(), path)
			if material != nil || err == nil || !strings.Contains(err.Error(), tc.want) || conversions.Load() != 2 || len(preparer.PreparedMaterials()) != 0 {
				t.Fatalf("rejected preparation minted/cached capability or lost reason: material=%+v conversions=%d err=%v", material, conversions.Load(), err)
			}
			if matches, err := filepath.Glob(filepath.Join(bus.WorkDir, ".codrax", "trace-input-*")); err != nil || len(matches) != 0 {
				t.Fatalf("rejected preparation retained output: paths=%v err=%v", matches, err)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("rejected named path replaced unrelated attachment")
			}
			hmc176AssertOriginal(t, path, tc.body)
		})
	}
}

func TestHMC176BGzipBinaryWarmGenerationChangeDoesNotFallback(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, changed := range []string{"source", "derived"} {
		t.Run(changed, func(t *testing.T) {
			path := hmc176WriteSource(t, hmc176Gzip(t, hmc176PerfData(true)))
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			params := map[string]any{"source": "path", "path": path, "view": "event_search"}
			hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			mutationPath := path
			if changed == "derived" {
				mutationPath = material.QueryPath()
			}
			body, err := os.ReadFile(mutationPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(mutationPath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mutationPath, append(body, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			result := hmc17NamedQuery(t, bus, params)
			if result.Success || len(result.Observations) != 0 || result.RawRef != "" || conversions.Load() != 1 {
				t.Fatalf("changed %s reused old evidence or silently reconverted: conversions=%d result=%+v", changed, conversions.Load(), result)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("generation failure changed unrelated attachment")
			}
		})
	}
}

const hmc176bTailText = "matrix-worker-42 (42) [000] .... 2.000000: sched_wakeup: comm=sealed-tail pid=424242 prio=120 target_cpu=000\n"

type hmc176bBundle struct {
	GzipInputProvenance *tracebundle.GzipInputProvenance    `json:"gzip_input_provenance"`
	ArchiveProvenance   *hitraceconv.TraceArchiveProvenance `json:"archive_provenance"`
	Systrace            string                              `json:"systrace"`
	Artifacts           []hitraceconv.Artifact              `json:"artifacts"`
}

func hmc176bReadBundle(t *testing.T, path string) hmc176bBundle {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bundle hmc176bBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func hmc176bAssertManaged(t *testing.T, managed, path string) {
	t.Helper()
	rel, err := filepath.Rel(managed, path)
	if err != nil || !filepath.IsAbs(path) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("derived output escaped managed location: path=%q managed=%q err=%v", path, managed, err)
	}
}
