package hitraceconv

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func gzipHiperfFixture(t *testing.T, body []byte) []byte {
	t.Helper()
	return gzipHiperfFixtureWithName(t, body, "")
}

func gzipHiperfFixtureWithName(t *testing.T, body []byte, name string) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	writer.Name = name
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func convertStandaloneHiperfGzipFixture(t *testing.T, compressed []byte, opts Options) (Result, string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", compressed,
	)...)
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "result.systrace")
	opts.InputPath = input
	opts.OutputPath = output
	if opts.TraceStreamerPath == "" {
		opts.TraceStreamerPath = filepath.Join(dir, "missing-trace_streamer")
	}
	result, err := ConvertFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("convert standalone HIPERF gzip: %v", err)
	}
	return result, dir
}

func TestStandaloneHiperfGzipRawRoundTripBindsTransformProvenance(t *testing.T) {
	raw := syntheticRawPerfData()
	compressed := gzipHiperfFixture(t, raw)
	result, dir := convertStandaloneHiperfGzipFixture(t, compressed, Options{PerfParser: "raw"})

	sourcePath := filepath.Join(dir, "result.perf.data")
	perfTracePath := filepath.Join(dir, "result.perftrace")
	source := artifactByPath(result.Artifacts, sourcePath)
	perfTrace := artifactByPath(result.Artifacts, perfTracePath)
	if source.Path == "" || source.Perf == nil || source.Perf.InputFormat != string(perfInputGzipPerfData) {
		t.Fatalf("gzip source artifact mismatch: %+v", source)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, compressed) {
		t.Fatalf("gzip source generation changed: err=%v got=%d want=%d", err, len(got), len(compressed))
	}
	if perfTrace.Path == "" || perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != perfProviderKindRawFallback ||
		perfTrace.Perf.InputFormat != string(perfInputGzipPerfData) || perfTrace.PerfTransform == nil {
		t.Fatalf("gzip-derived raw perftrace mismatch: %+v", perfTrace)
	}
	sourceDigest := sha256.Sum256(compressed)
	decodedDigest := sha256.Sum256(raw)
	transform := perfTrace.PerfTransform
	if transform.Profile != perfInputTransformGzipV1 || transform.SourceArtifactPath != sourcePath ||
		transform.SourceBytes != int64(len(compressed)) || transform.SourceSHA256 != hex.EncodeToString(sourceDigest[:]) ||
		transform.DecodedBytes != int64(len(raw)) || transform.DecodedSHA256 != hex.EncodeToString(decodedDigest[:]) {
		t.Fatalf("gzip transform receipt mismatch: %+v", transform)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].ProviderName != perfProviderNameHiperfProto ||
		result.ProviderDecisions[0].InputFormat != string(perfInputGzipPerfData) || result.ProviderDecisions[0].Attempted ||
		!result.ProviderDecisions[1].Succeeded || result.ProviderDecisions[1].InputFormat != string(perfInputGzipPerfData) {
		t.Fatalf("gzip raw provider route mismatch: %+v", result.ProviderDecisions)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.BundlePath)
	if err != nil || idx == nil || len(idx.Events) == 0 {
		t.Fatalf("gzip-derived bundle did not round-trip through tracequery: idx=%+v err=%v", idx, err)
	}
	manifestBody, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest traceBundleMetadata
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatal(err)
	}
	wireSource := artifactByPath(manifest.Artifacts, filepath.Base(sourcePath))
	wirePerfTrace := artifactByPath(manifest.Artifacts, filepath.Base(perfTracePath))
	if wireSource.Path == "" || wirePerfTrace.PerfTransform == nil ||
		wirePerfTrace.PerfTransform.SourceArtifactPath != wireSource.Path || filepath.IsAbs(wireSource.Path) {
		t.Fatalf("bundle transform is not relocatable: source=%+v perftrace=%+v", wireSource, wirePerfTrace)
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipOfficialConsumesDecodedGeneration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell adapter")
	}
	dir := t.TempDir()
	raw := syntheticRawPerfData()
	compressed := gzipHiperfFixture(t, raw)
	decodedFixture := filepath.Join(dir, "decoded.perf.data")
	protoFixture := filepath.Join(dir, "fixture.proto")
	if err := os.WriteFile(decodedFixture, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protoFixture, syntheticHiperfProtoStream(), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(dir, "hiperf_host")
	writeExecutableTestFile(t, tool, `#!/bin/sh
in=""
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-i" ]; then shift; in="$1"; fi
  if [ "$1" = "-o" ]; then shift; out="$1"; fi
  shift
done
cmp "$HIPERF_DECODED_FIXTURE" "$in" || exit 42
cp "$HIPERF_PROTO_FIXTURE" "$out" || exit 43
`)
	t.Setenv("HIPERF_DECODED_FIXTURE", decodedFixture)
	t.Setenv("HIPERF_PROTO_FIXTURE", protoFixture)
	result, resultDir := convertStandaloneHiperfGzipFixture(t, compressed, Options{PerfParser: "official", HiperfPath: tool})
	perfTrace := artifactByPath(result.Artifacts, filepath.Join(resultDir, "result.perftrace"))
	if perfTrace.Path == "" || perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != perfProviderKindOfficialHarmony ||
		perfTrace.Perf.InputFormat != string(perfInputGzipPerfData) || perfTrace.PerfTransform == nil {
		t.Fatalf("official gzip result mismatch: %+v", perfTrace)
	}
	if len(result.ProviderDecisions) != 1 || !result.ProviderDecisions[0].Succeeded ||
		result.ProviderDecisions[0].InputFormat != string(perfInputGzipPerfData) {
		t.Fatalf("official gzip decision mismatch: %+v", result.ProviderDecisions)
	}
	assertNoHiperfPrivateStaging(t, resultDir)
}

func TestStandaloneHiperfGzipOfficialFailureFallsBackToDecodedRaw(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell adapter")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "hiperf_host")
	writeExecutableTestFile(t, tool, `#!/bin/sh
exit 7
`)
	result, resultDir := convertStandaloneHiperfGzipFixture(t, gzipHiperfFixture(t, syntheticRawPerfData()), Options{HiperfPath: tool})
	perfTrace := artifactByPath(result.Artifacts, filepath.Join(resultDir, "result.perftrace"))
	if perfTrace.Path == "" || perfTrace.Perf == nil || perfTrace.Perf.ProviderKind != perfProviderKindRawFallback ||
		perfTrace.Perf.InputFormat != string(perfInputGzipPerfData) || perfTrace.PerfTransform == nil {
		t.Fatalf("official failure lost gzip raw fallback provenance: %+v", perfTrace)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_adapter_failed" ||
		!result.ProviderDecisions[0].Attempted || result.ProviderDecisions[0].InputFormat != string(perfInputGzipPerfData) ||
		!result.ProviderDecisions[1].Succeeded || !result.ProviderDecisions[1].Fallback ||
		result.ProviderDecisions[1].InputFormat != string(perfInputGzipPerfData) {
		t.Fatalf("official-to-raw gzip decisions mismatch: %+v", result.ProviderDecisions)
	}
	assertNoHiperfPrivateStaging(t, resultDir)
}

func TestStandaloneHiperfGzipToolUnavailableFallsBackToDecodedRaw(t *testing.T) {
	t.Setenv("CODRAX_HIPERF_HOST", "")
	t.Setenv("PATH", t.TempDir())
	result, dir := convertStandaloneHiperfGzipFixture(t, gzipHiperfFixture(t, syntheticRawPerfData()), Options{})
	perfTrace := artifactByPath(result.Artifacts, filepath.Join(dir, "result.perftrace"))
	if perfTrace.Path == "" || perfTrace.PerfTransform == nil || perfTrace.Perf == nil ||
		perfTrace.Perf.ProviderKind != perfProviderKindRawFallback || perfTrace.Perf.InputFormat != string(perfInputGzipPerfData) {
		t.Fatalf("tool-unavailable gzip fallback mismatch: %+v", perfTrace)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_tool_unavailable" ||
		result.ProviderDecisions[0].Attempted || !result.ProviderDecisions[1].Succeeded || !result.ProviderDecisions[1].Fallback {
		t.Fatalf("tool-unavailable gzip decisions mismatch: %+v", result.ProviderDecisions)
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipDisabledPreservesCompressedEvidenceWithoutDecode(t *testing.T) {
	compressed := gzipHiperfFixture(t, syntheticRawPerfData())
	var progress []ProgressEvent
	result, dir := convertStandaloneHiperfGzipFixture(t, compressed, Options{
		DisablePerfAdapter: true,
		Progress:           func(event ProgressEvent) { progress = append(progress, event) },
	})
	if len(result.ProviderDecisions) != 1 || result.ProviderDecisions[0].ProviderName != perfProviderNamePerftraceDisabled ||
		result.ProviderDecisions[0].InputFormat != string(perfInputGzipPerfData) || result.ProviderDecisions[0].Attempted {
		t.Fatalf("disabled gzip decision mismatch: %+v", result.ProviderDecisions)
	}
	for _, event := range progress {
		if event.Stage == "hiperf_gzip_decompress" {
			t.Fatalf("disabled route decoded gzip input: %+v", event)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, "result.perf.data")); err != nil || !bytes.Equal(got, compressed) {
		t.Fatalf("disabled route lost compressed evidence: err=%v", err)
	}
	if artifactByPath(result.Artifacts, filepath.Join(dir, "result.perftrace")).Path != "" {
		t.Fatalf("disabled route published perftrace: %+v", result.Artifacts)
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipMultipleSegmentsKeepIndependentTransforms(t *testing.T) {
	dir := t.TempDir()
	rawA := syntheticRawPerfData()
	rawB := append([]byte(nil), rawA...)
	compressedB := gzipHiperfFixture(t, rawB)
	input := filepath.Join(dir, "capture.sys")
	body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", rawA)...)
	body = append(body, syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", compressedB)...)
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "result.systrace"), PerfParser: "raw",
		TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
	})
	if err != nil {
		t.Fatalf("convert multi-segment HIPERF fixture: %v", err)
	}
	plainTrace := artifactByPath(result.Artifacts, filepath.Join(dir, "result.perftrace"))
	gzipTrace := artifactByPath(result.Artifacts, filepath.Join(dir, "result_2.perftrace"))
	if plainTrace.Path == "" || plainTrace.PerfTransform != nil || plainTrace.Perf == nil ||
		plainTrace.Perf.InputFormat != string(perfInputLinuxPerfData) {
		t.Fatalf("plain segment acquired gzip provenance: %+v", plainTrace)
	}
	if gzipTrace.Path == "" || gzipTrace.PerfTransform == nil || gzipTrace.Perf == nil ||
		gzipTrace.Perf.InputFormat != string(perfInputGzipPerfData) ||
		gzipTrace.PerfTransform.SourceArtifactPath != filepath.Join(dir, "result.perf_2.data") {
		t.Fatalf("gzip segment lost independent provenance: %+v", gzipTrace)
	}
	if _, err := tracequery.BuildIndex(context.Background(), result.BundlePath); err != nil {
		t.Fatalf("multi-segment gzip bundle rejected: %v", err)
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipComposesWithSecureArchiveIntake(t *testing.T) {
	dir := t.TempDir()
	compressed := gzipHiperfFixture(t, syntheticRawPerfData())
	member := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", compressed,
	)...)
	input := filepath.Join(dir, "capture.zip")
	traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "nested/capture.sys", body: member, method: zip.Deflate})
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: filepath.Join(dir, "result.systrace"), PerfParser: "raw",
		TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
	})
	if err != nil {
		t.Fatalf("convert archived HIPERF gzip: %v", err)
	}
	if result.ArchiveProvenance == nil || result.ArchiveProvenance.Member != "nested/capture.sys" {
		t.Fatalf("archive provenance missing: %+v", result.ArchiveProvenance)
	}
	perfTrace := artifactByPath(result.Artifacts, filepath.Join(dir, "result.perftrace"))
	if perfTrace.PerfTransform == nil || perfTrace.PerfTransform.SourceArtifactPath != filepath.Join(dir, "result.perf.data") {
		t.Fatalf("inner gzip transform missing: %+v", perfTrace)
	}
	if _, err := tracequery.BuildIndex(context.Background(), result.BundlePath); err != nil {
		t.Fatalf("archive+gzip bundle rejected: %v", err)
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipRejectsInvalidDataWithoutChildOrPerftrace(t *testing.T) {
	valid := gzipHiperfFixture(t, syntheticRawPerfData())
	badCRC := append([]byte(nil), valid...)
	badCRC[len(badCRC)-8] ^= 0xff
	badISize := append([]byte(nil), valid...)
	badISize[len(badISize)-4] ^= 0xff
	truncated := append([]byte(nil), valid[:len(valid)-1]...)
	badHeader := append([]byte(nil), valid...)
	badHeader[3] |= 0xe0
	ratioBody := append([]byte(perfMagic2), bytes.Repeat([]byte{0}, 8<<20)...)
	tests := []struct {
		name string
		data []byte
		code string
	}{
		{name: "short", data: []byte{0x1f, 0x8b, 0x08, 0, 1}, code: hiperfGzipCodeResourceLimit},
		{name: "header", data: badHeader, code: hiperfGzipCodeInvalidHeader},
		{name: "crc", data: badCRC, code: hiperfGzipCodeIntegrity},
		{name: "isize", data: badISize, code: hiperfGzipCodeIntegrity},
		{name: "truncated", data: truncated, code: hiperfGzipCodeIntegrity},
		{name: "trailing", data: append(append([]byte(nil), valid...), 0), code: hiperfGzipCodeTrailingData},
		{name: "concatenated", data: append(append([]byte(nil), valid...), valid...), code: hiperfGzipCodeTrailingData},
		{name: "non-perf", data: gzipHiperfFixture(t, []byte("NOT-PERF-DATA")), code: hiperfGzipCodeDecodedFormat},
		{name: "empty", data: gzipHiperfFixture(t, nil), code: hiperfGzipCodeIntegrity},
		{name: "optional-field-budget", data: gzipHiperfFixtureWithName(t, syntheticRawPerfData(), strings.Repeat("x", int(hiperfGzipMaxOptionalFieldBytes)+1)), code: hiperfGzipCodeResourceLimit},
		{name: "ratio", data: gzipHiperfFixture(t, ratioBody), code: hiperfGzipCodeResourceLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, dir := convertStandaloneHiperfGzipFixture(t, test.data, Options{})
			if len(result.ProviderDecisions) != 2 {
				t.Fatalf("invalid gzip decisions=%+v", result.ProviderDecisions)
			}
			for _, decision := range result.ProviderDecisions {
				if decision.InputFormat != string(perfInputGzipPerfData) || decision.Attempted ||
					decision.Succeeded || decision.Reason != test.code || !strings.Contains(decision.Caveat, test.code) {
					t.Fatalf("invalid gzip decision mismatch: %+v", decision)
				}
			}
			if got, err := os.ReadFile(filepath.Join(dir, "result.perf.data")); err != nil || !bytes.Equal(got, test.data) {
				t.Fatalf("invalid gzip evidence was not preserved: err=%v", err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "result.perftrace")); !os.IsNotExist(err) {
				t.Fatalf("invalid gzip published perftrace: %v", err)
			}
			assertNoHiperfPrivateStaging(t, dir)
		})
	}
}

func TestStandaloneHiperfGzipCancellationRollsBack(t *testing.T) {
	dir := t.TempDir()
	compressed := gzipHiperfFixture(t, append([]byte(perfMagic2), bytes.Repeat([]byte{1}, 1<<20)...))
	input := filepath.Join(dir, "capture.sys")
	body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", compressed)...)
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result, err := ConvertFile(ctx, Options{
		InputPath: input, OutputPath: filepath.Join(dir, "result.systrace"), PerfParser: "raw",
		TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
		Progress: func(event ProgressEvent) {
			if event.Stage == "hiperf_gzip_decompress" && event.Status == ProgressStatusStarted {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) || !reflectResultZero(result) {
		t.Fatalf("gzip cancellation did not fail closed: result=%+v err=%v", result, err)
	}
	for _, leaf := range []string{"result.systrace", "result.perf.data", "result.perftrace", "result.tracebundle.json"} {
		if _, statErr := os.Lstat(filepath.Join(dir, leaf)); !os.IsNotExist(statErr) {
			t.Fatalf("gzip cancellation leaked %s: %v", leaf, statErr)
		}
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestStandaloneHiperfGzipBundleTransformTamperFailsClosed(t *testing.T) {
	result, _ := convertStandaloneHiperfGzipFixture(t, gzipHiperfFixture(t, syntheticRawPerfData()), Options{PerfParser: "raw"})
	original, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*traceBundleMetadata)
	}{
		{name: "missing", mutate: func(bundle *traceBundleMetadata) { bundlePerfTrace(t, bundle).PerfTransform = nil }},
		{name: "source-path", mutate: func(bundle *traceBundleMetadata) {
			bundlePerfTrace(t, bundle).PerfTransform.SourceArtifactPath = "missing.perf.data"
		}},
		{name: "source-hash", mutate: func(bundle *traceBundleMetadata) {
			bundlePerfTrace(t, bundle).PerfTransform.SourceSHA256 = strings.Repeat("0", 64)
		}},
		{name: "decoded-hash", mutate: func(bundle *traceBundleMetadata) {
			bundlePerfTrace(t, bundle).PerfTransform.DecodedSHA256 = strings.Repeat("x", 64)
		}},
		{name: "decoded-size", mutate: func(bundle *traceBundleMetadata) { bundlePerfTrace(t, bundle).PerfTransform.DecodedBytes = 0 }},
		{name: "profile", mutate: func(bundle *traceBundleMetadata) { bundlePerfTrace(t, bundle).PerfTransform.Profile = "future" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bundle traceBundleMetadata
			if err := json.Unmarshal(original, &bundle); err != nil {
				t.Fatal(err)
			}
			test.mutate(&bundle)
			body, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(result.BundlePath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			idx, buildErr := tracequery.BuildIndex(context.Background(), result.BundlePath)
			if buildErr == nil || idx != nil {
				t.Fatalf("tampered gzip transform was accepted: idx=%+v err=%v", idx, buildErr)
			}
		})
	}
}

func bundlePerfTrace(t *testing.T, bundle *traceBundleMetadata) *Artifact {
	t.Helper()
	for index := range bundle.Artifacts {
		if bundle.Artifacts[index].Type == ArtifactPerfTrace {
			return &bundle.Artifacts[index]
		}
	}
	t.Fatal("bundle fixture has no perftrace")
	return nil
}
