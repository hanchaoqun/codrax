package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseStandaloneHiperfRejectsCompressedAndUnknownBeforeChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell adapter")
	}
	tests := []struct {
		name       string
		payload    []byte
		format     string
		reason     string
		wantDetail string
	}{
		{name: "gzip", payload: []byte{0x1f, 0x8b, 0x08, 0x00, 0x01}, format: string(perfInputGzipPerfData), reason: "unsafe_compressed_input_scratch", wantDetail: "fixed decompression scratch"},
		{name: "unknown", payload: []byte("PERF-DATA"), format: "unknown", reason: "unsupported_input_format", wantDetail: "requires exact linux_perf_data"},
		{name: "truncated-gzip-magic", payload: []byte{0x1f}, format: "unknown", reason: "unsupported_input_format", wantDetail: "requires exact linux_perf_data"},
		{name: "empty", payload: nil, format: "unknown", reason: "unsupported_input_format", wantDetail: "requires exact linux_perf_data"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "child-ran")
			tool := filepath.Join(dir, "hiperf_host")
			writeExecutableTestFile(t, tool, `#!/bin/sh
printf 'unexpected child execution\n' >> "$HIPERF_CHILD_MARKER"
exit 99
`)
			t.Setenv("HIPERF_CHILD_MARKER", marker)
			input := filepath.Join(dir, "capture.sys")
			body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", test.payload)...)
			if err := os.WriteFile(input, body, 0o640); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "result.systrace")
			result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, HiperfPath: tool})
			if err != nil {
				t.Fatalf("convert conservative %s payload: %v", test.name, err)
			}
			if _, err := os.Lstat(marker); !os.IsNotExist(err) {
				t.Fatalf("%s payload reached official child: %v", test.name, err)
			}
			sidecar := filepath.Join(dir, "result.perf.data")
			got, err := os.ReadFile(sidecar)
			if err != nil || !bytes.Equal(got, test.payload) {
				t.Fatalf("preserved sidecar mismatch: err=%v got=%x want=%x", err, got, test.payload)
			}
			artifact := artifactByPath(result.Artifacts, sidecar)
			if artifact.Path == "" || artifact.Perf == nil || artifact.Perf.InputFormat != test.format || artifact.Perf.TraceQueryReady {
				t.Fatalf("sidecar format/capability mismatch: %+v", artifact)
			}
			if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].ProviderName != perfProviderNameHiperfProto ||
				result.ProviderDecisions[0].Attempted || result.ProviderDecisions[0].Reason != test.reason ||
				!strings.Contains(result.ProviderDecisions[0].Caveat, test.wantDetail) ||
				result.ProviderDecisions[1].ProviderName != perfProviderNameRawFallback || result.ProviderDecisions[1].Attempted ||
				result.ProviderDecisions[1].Reason != "unsupported_input_format" {
				t.Fatalf("conservative provider decisions mismatch: %+v", result.ProviderDecisions)
			}
			assertNoHiperfPrivateStaging(t, dir)
		})
	}
}

func TestReleaseStandaloneHiperfChildFailureFallsBackFromBoundedViewWithoutPrivateLeak(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell adapter")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "hiperf_host")
	writeExecutableTestFile(t, tool, `#!/bin/sh
in=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-i" ]; then shift; in="$1"; fi
  shift
done
printf 'failed private input=%s\n' "$in" >&2
exit 7
`)
	payload := syntheticRawPerfData()
	input := filepath.Join(dir, "capture.sys")
	body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", payload)...)
	if err := os.WriteFile(input, body, 0o640); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "result.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, HiperfPath: tool})
	if err != nil {
		t.Fatalf("raw fallback after child exit: %v", err)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_adapter_failed" ||
		!strings.Contains(result.ProviderDecisions[0].Caveat, "child output suppressed") ||
		!result.ProviderDecisions[1].Succeeded || !result.ProviderDecisions[1].Fallback {
		t.Fatalf("official-to-raw decisions mismatch: %+v", result.ProviderDecisions)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ToLower(encoded), []byte(".codrax-hiperf-input-")) || bytes.Contains(encoded, []byte("failed private input=")) {
		t.Fatalf("private child argv/output escaped result: %s", encoded)
	}
	got, err := os.ReadFile(filepath.Join(dir, "result.perf.data"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("raw fallback sidecar mismatch: err=%v got=%d want=%d", err, len(got), len(payload))
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestReleaseStandaloneHiperfPublicationCollisionPreservesCompetitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell adapter")
	}
	dir := t.TempDir()
	proto := filepath.Join(dir, "fixture.proto")
	if err := os.WriteFile(proto, syntheticHiperfProtoStream(), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(dir, "hiperf_host")
	writeExecutableTestFile(t, tool, `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then shift; out="$1"; fi
  shift
done
cp "$HIPERF_PROTO_FIXTURE" "$out" || exit 50
printf 'external competitor\n' > "$HIPERF_PUBLIC_SIDECAR" || exit 51
`)
	t.Setenv("HIPERF_PROTO_FIXTURE", proto)
	input := filepath.Join(dir, "capture.sys")
	body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", syntheticRawPerfData())...)
	if err := os.WriteFile(input, body, 0o640); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "result.systrace")
	sidecar := filepath.Join(dir, "result.perf.data")
	t.Setenv("HIPERF_PUBLIC_SIDECAR", sidecar)
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, HiperfPath: tool})
	if err == nil || !reflectResultZero(result) {
		t.Fatalf("publication collision did not fail transaction: result=%+v err=%v", result, err)
	}
	got, readErr := os.ReadFile(sidecar)
	if readErr != nil || string(got) != "external competitor\n" {
		t.Fatalf("competitor was changed or removed: err=%v body=%q", readErr, got)
	}
	for _, path := range []string{output, filepath.Join(dir, "result.perftrace"), filepath.Join(dir, "result.tracebundle.json")} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("failed collision leaked transaction output %s: %v", path, statErr)
		}
	}
	assertNoHiperfPrivateStaging(t, dir)
}

func TestReleaseStandaloneHiperfPayloadProbeCannotReadContainerOrNeighbor(t *testing.T) {
	unknownPayload := []byte("NOT-PERF")
	first := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", unknownPayload)
	second := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", syntheticRawPerfData())
	container := append(append([]byte(nil), first...), second...)
	view := newScriptedStandaloneInputView("future.perf.data", container)
	inventory, err := findStandaloneSegmentsFromInput(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := newStandaloneHiperfPayloadView(inventory, 0, "future.perf.data")
	if err != nil {
		t.Fatal(err)
	}
	format, err := detectPerfInputFormatFromView(context.Background(), payload, conversionInputStageStandaloneExtract)
	if err != nil || format != perfInputUnknown {
		t.Fatalf("neighbor magic escaped payload bound: format=%q err=%v", format, err)
	}
	buffer := make([]byte, len(unknownPayload)+64)
	n, readErr := payload.ReadAt(buffer, 0)
	if n != len(unknownPayload) || !errors.Is(readErr, io.EOF) || !bytes.Equal(buffer[:n], unknownPayload) {
		t.Fatalf("bounded payload read mismatch: n=%d err=%v body=%q", n, readErr, buffer[:n])
	}
}

func TestReleaseStandaloneHiperfBoundedViewCannotUseLinuxWholeFileFD(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux inherited-FD capability test")
	}
	dir := t.TempDir()
	payloadBytes := syntheticRawPerfData()
	block := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", payloadBytes)
	path := filepath.Join(dir, "container.sys")
	if err := os.WriteFile(path, block, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := findStandaloneSegmentsFromInput(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := newStandaloneHiperfPayloadView(inventory, 0, filepath.Join(dir, "future.perf.data"))
	if err != nil {
		t.Fatal(err)
	}
	staging, err := newPrivateConversionDir(dir, ".fd-negative-*")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.FinalizeCleanup()
	lease, err := newExternalToolInputLease(context.Background(), payload, staging, "payload.perf.data", externalToolInputVerifiedLinuxFD)
	if lease != nil {
		_ = lease.Close()
		t.Fatal("bounded HIPERF payload unexpectedly received a whole-file inherited FD lease")
	}
	var typed *ConversionInputError
	if !errors.As(err, &typed) || typed.Code != ConversionInputCodeInternalContract || typed.Stage != conversionInputStageExternalTool.String() {
		t.Fatalf("bounded whole-file capability rejection=%T %v", err, err)
	}
}

func TestHiperfProtoPrivatePathCensusCoversEveryPublishedStringField(t *testing.T) {
	private := filepath.Join(t.TempDir(), ".codrax-hiperf-input-secret", "payload.perf.data")
	identity := capturePrivatePathIdentity(filepath.Dir(private))
	tests := map[string]hiperfProtoData{
		"file_path":     {Files: map[uint32]hiperfProtoFile{1: {Path: private}}},
		"function_name": {Files: map[uint32]hiperfProtoFile{1: {FunctionNames: []string{private}}}},
		"thread_name":   {Threads: map[uint32]hiperfProtoThread{1: {Name: private}}},
		"config_name":   {ConfigNames: []string{private}},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if !hiperfProtoDataContainsPrivatePath(data, identity) {
				t.Fatalf("private path escaped field %s", name)
			}
		})
	}
	public := hiperfProtoData{
		Files:       map[uint32]hiperfProtoFile{1: {Path: "/system/lib/libfoo.so", FunctionNames: []string{"doWork"}}},
		Threads:     map[uint32]hiperfProtoThread{1: {Name: "RenderThread"}},
		ConfigNames: []string{"cpu-cycles"},
	}
	if hiperfProtoDataContainsPrivatePath(public, identity) {
		t.Fatal("public hiperf strings were falsely classified as private")
	}
}

func TestReleaseStandaloneHiperfStructurePinsOneLeaseAndLatePublication(t *testing.T) {
	provider := sourceGenerationFunctionBody(t, "hiperf_proto.go", "maybeConvertHiperfPerfDataFromInput")
	for _, required := range []string{"inputLease.Command(", "validateExternalToolCommandBoundary(", "maybeRawPerfFallbackFromStandaloneInput("} {
		if !strings.Contains(provider, required) {
			t.Fatalf("hiperf provider lost %q:\n%s", required, provider)
		}
	}
	for _, forbidden := range []string{"exec.CommandContext(", "detectPerfInputFormat(", "maybeConvertRawPerfDataWithDecision(", "maybeRawPerfFallback(ctx"} {
		if strings.Contains(provider, forbidden) {
			t.Fatalf("hiperf provider regained path/second command lane %q:\n%s", forbidden, provider)
		}
	}
	seal := sourceGenerationFunctionBody(t, "external_tool_input_lease.go", "sealExternalToolInputSnapshot")
	if !strings.Contains(seal, "detachSnapshotAsSealed()") || strings.Contains(seal, "AdoptRegularChild(") || strings.Contains(seal, "os.Open(") {
		t.Fatalf("snapshot terminal transition regained close/reopen lane:\n%s", seal)
	}
	resolver := sourceGenerationFunctionBody(t, "hiperf_proto.go", "resolveHiperfProviderTool")
	if strings.Count(resolver, "externalToolInputSnapshotOnly") != 1 || strings.Contains(resolver, "externalToolInputVerifiedLinuxFD") {
		t.Fatalf("hiperf resolver is not mechanically snapshot-only:\n%s", resolver)
	}
}

func artifactByPath(artifacts []Artifact, path string) Artifact {
	for _, artifact := range artifacts {
		if artifact.Path == path {
			return artifact
		}
	}
	return Artifact{}
}

func reflectResultZero(result Result) bool {
	encoded, err := json.Marshal(result)
	if err != nil {
		return false
	}
	var empty Result
	want, err := json.Marshal(empty)
	return err == nil && bytes.Equal(encoded, want)
}

func assertNoHiperfPrivateStaging(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".codrax-hiperf-input-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("private HIPERF staging survived: %v", matches)
	}
}

func writeExecutableTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
