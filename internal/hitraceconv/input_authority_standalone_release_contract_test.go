package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type scriptedStandaloneInputView struct {
	reader    *bytes.Reader
	path      string
	counts    map[conversionInputStage]int
	failStage conversionInputStage
	failCall  int
	reads     int
	readEnds  []int64
}

type cancelingStandaloneReaderAt struct {
	reader *bytes.Reader
	cancel context.CancelFunc
	reads  int
}

func (reader *cancelingStandaloneReaderAt) ReadAt(buffer []byte, offset int64) (int, error) {
	reader.reads++
	n, err := reader.reader.ReadAt(buffer, offset)
	if reader.reads == 2 {
		reader.cancel()
	}
	return n, err
}

func newScriptedStandaloneInputView(path string, data []byte) *scriptedStandaloneInputView {
	return &scriptedStandaloneInputView{
		reader: bytes.NewReader(append([]byte(nil), data...)),
		path:   path,
		counts: make(map[conversionInputStage]int),
	}
}

func (input *scriptedStandaloneInputView) ReadAt(buffer []byte, offset int64) (int, error) {
	input.reads++
	input.readEnds = append(input.readEnds, offset+int64(len(buffer)))
	return input.reader.ReadAt(buffer, offset)
}

func (input *scriptedStandaloneInputView) Size() int64 {
	return input.reader.Size()
}

func (input *scriptedStandaloneInputView) DisplayPath() string {
	return input.path
}

func (input *scriptedStandaloneInputView) Validate(stage conversionInputStage) error {
	input.counts[stage]++
	failedCall := input.failCall > 0 && input.counts[stage] == input.failCall
	failedFrom := input.failCall < 0 && input.counts[stage] >= -input.failCall
	if stage == input.failStage && (failedCall || failedFrom) {
		return conversionInputFailure(ConversionInputCodeGenerationChanged, stage, input.path, errors.New("scripted generation change"))
	}
	return nil
}

func TestReleaseStandaloneReaderAtCensusAndExtractionParity(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "multi-standalone.sys")
	outputPath := filepath.Join(dir, "multi.systrace")
	prefix := bytes.Repeat([]byte{0x5a}, 1024*1024-4)
	nonPerf := syntheticStandaloneProfilerBlock(77, "other-plugin", "2.0", []byte("OTHER"))
	firstPayload := []byte("PERF-PAYLOAD-ONE")
	secondPayload := []byte("PERF-PAYLOAD-TWO")
	firstPerf := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-one", "1.01", firstPayload)
	secondPerf := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-two", "1.02", secondPayload)
	body := append(append(append(append([]byte(nil), prefix...), nonPerf...), firstPerf...), secondPerf...)
	if err := os.WriteFile(inputPath, body, 0o640); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()

	inventory, err := findStandaloneSegmentsFromInput(context.Background(), authority)
	if err != nil {
		t.Fatalf("scan standalone inventory: %v", err)
	}
	if inventory.input != authority || inventory.inputSize != int64(len(body)) || len(inventory.segments) != 3 || !inventory.hasHiperfData() {
		t.Fatalf("unexpected authority-bound inventory: size=%d segments=%+v source_match=%t", inventory.inputSize, inventory.segments, inventory.input == authority)
	}
	wantOffsets := []int64{int64(len(prefix)), int64(len(prefix) + len(nonPerf)), int64(len(prefix) + len(nonPerf) + len(firstPerf))}
	wantTypes := []uint32{77, profilerDataTypeHiperf, profilerDataTypeHiperf}
	wantNames := []string{"other-plugin", "hiperf-one", "hiperf-two"}
	wantVersions := []string{"2.0", "1.01", "1.02"}
	for index, segment := range inventory.segments {
		if segment.Offset != wantOffsets[index] || segment.DataType != wantTypes[index] || segment.PluginName != wantNames[index] || segment.PluginVersion != wantVersions[index] {
			t.Fatalf("segment %d mismatch: %+v", index, segment)
		}
	}

	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _, _, err := extractStandaloneArtifactsWithOptionsAndLedger(
		context.Background(),
		Options{InputPath: inputPath, DisablePerfAdapter: true},
		inventory,
		outputPath,
		standaloneExtractOptions{GeneratePerfTrace: true},
		ledger,
	)
	if err != nil {
		t.Fatalf("extract authority-bound standalone inventory: %v", err)
	}
	var perfArtifacts []Artifact
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactPerfData {
			perfArtifacts = append(perfArtifacts, artifact)
		}
	}
	if len(perfArtifacts) != 2 {
		t.Fatalf("perf artifacts=%+v", artifacts)
	}
	for index, wantPayload := range [][]byte{firstPayload, secondPayload} {
		got, err := os.ReadFile(perfArtifacts[index].Path)
		if err != nil {
			t.Fatal(err)
		}
		segment := inventory.segments[index+1]
		if !bytes.Equal(got, wantPayload) || perfArtifacts[index].SourceOffset != segment.Offset || perfArtifacts[index].SourceBytes != segment.Length {
			t.Fatalf("perf artifact %d lost inventory provenance: artifact=%+v payload=%q want=%q", index, perfArtifacts[index], got, wantPayload)
		}
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("standalone publications failed ledger validation: %v", err)
	}
}

func TestReleaseStandaloneScanGenerationChangeHasTypedStage(t *testing.T) {
	input := newScriptedStandaloneInputView("scripted-scan.sys", syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf", "1.0", []byte("payload")))
	input.failStage = conversionInputStageStandaloneScan
	input.failCall = 2
	inventory, err := findStandaloneSegmentsFromInput(context.Background(), input)
	assertStandaloneGenerationError(t, err, conversionInputStageStandaloneScan)
	if inventory.input != nil || inventory.inputSize != 0 || len(inventory.segments) != 0 {
		t.Fatalf("failed scan leaked inventory: %+v", inventory)
	}
	if input.counts[conversionInputStageStandaloneScan] != 2 {
		t.Fatalf("standalone scan did not execute entry and exit gates: %+v", input.counts)
	}
}

func TestReleaseConversionInputStageGatePreservesSecondaryFailure(t *testing.T) {
	secondary := errors.New("secondary cleanup failure")
	input := newScriptedStandaloneInputView("stage-gate.sys", nil)
	input.failStage = conversionInputStageStandaloneScan
	input.failCall = 1
	err := completeConversionInputStage(context.Background(), input, conversionInputStageStandaloneScan, secondary)
	assertStandaloneGenerationError(t, err, conversionInputStageStandaloneScan)
	if !errors.Is(err, secondary) {
		t.Fatalf("generation gate discarded secondary failure: %v", err)
	}

	input = newScriptedStandaloneInputView("stage-gate-single.sys", nil)
	input.failStage = conversionInputStageStandaloneScan
	input.failCall = 1
	err = completeConversionInputStage(context.Background(), input, conversionInputStageStandaloneScan, nil)
	if _, ok := err.(*ConversionInputError); !ok {
		t.Fatalf("single generation failure lost concrete identity: %T %v", err, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = completeConversionInputStage(ctx, input, conversionInputStageStandaloneScan, secondary)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, secondary) {
		t.Fatalf("cancellation gate discarded sentinel or secondary failure: %v", err)
	}

	persistent := newScriptedStandaloneInputView("stage-gate-idempotent.sys", nil)
	persistent.failStage = conversionInputStageStandaloneScan
	persistent.failCall = -1
	first := completeConversionInputStage(context.Background(), persistent, conversionInputStageStandaloneScan, nil)
	assertStandaloneGenerationError(t, first, conversionInputStageStandaloneScan)
	second := completeConversionInputStage(context.Background(), persistent, conversionInputStageStandaloneScan, first)
	if second != first {
		t.Fatalf("repeated generation gate duplicated a single concrete failure: first=%T %v second=%T %v", first, first, second, second)
	}
	joined := completeConversionInputStage(context.Background(), persistent, conversionInputStageStandaloneScan, secondary)
	repeatedJoined := completeConversionInputStage(context.Background(), persistent, conversionInputStageStandaloneScan, joined)
	if repeatedJoined != joined || !errors.Is(repeatedJoined, secondary) {
		t.Fatalf("repeated generation gate duplicated or discarded joined failure: first=%v second=%v", joined, repeatedJoined)
	}
	canceledCtx, canceled := context.WithCancel(context.Background())
	canceled()
	first = completeConversionInputStage(canceledCtx, persistent, conversionInputStageStandaloneScan, nil)
	second = completeConversionInputStage(canceledCtx, persistent, conversionInputStageStandaloneScan, first)
	if first != context.Canceled || second != first {
		t.Fatalf("repeated cancellation gate lost concrete sentinel: first=%T %v second=%T %v", first, first, second, second)
	}
}

func TestReleaseStandaloneExtractGenerationChangeRollsBackSidecar(t *testing.T) {
	for _, failCall := range []int{2, 3} {
		t.Run(map[int]string{2: "after-copy-before-adapter", 3: "final-batch-gate"}[failCall], func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "scripted-extract.sys")
			outputPath := filepath.Join(dir, "scripted.systrace")
			payload := []byte("NOT-A-PERF-FILE")
			block := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf", "1.0", payload)
			input := newScriptedStandaloneInputView(inputPath, block)
			input.failStage = conversionInputStageStandaloneExtract
			input.failCall = failCall
			inventory := standaloneSegmentInventory{
				inputSize: int64(len(block)),
				segments:  []standaloneSegment{{Offset: 0, Length: int64(len(block)), DataType: profilerDataTypeHiperf, PluginName: "hiperf", PluginVersion: "1.0"}},
				input:     input,
			}
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			artifacts, caveats, decisions, err := extractStandaloneArtifactsWithOptionsAndLedger(
				context.Background(),
				Options{InputPath: inputPath, DisablePerfAdapter: true},
				inventory,
				outputPath,
				standaloneExtractOptions{GeneratePerfTrace: true},
				ledger,
			)
			err = joinConversionCleanupError(err, ledger)
			assertStandaloneGenerationError(t, err, conversionInputStageStandaloneExtract)
			if len(artifacts) != 0 || len(caveats) != 0 || len(decisions) != 0 {
				t.Fatalf("generation failure leaked extraction result: artifacts=%+v caveats=%+v decisions=%+v", artifacts, caveats, decisions)
			}
			assertNoStandalonePublication(t, inputPath, outputPath)
		})
	}
}

func TestReleaseStandalonePhysicalMutationDuringAdapterFailsAtExtractStage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX report adapter")
	}
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string, original []byte, info os.FileInfo)
	}{
		{
			name: "same-size-restored-mtime",
			mutate: func(t *testing.T, path string, original []byte, info os.FileInfo) {
				t.Helper()
				if err := os.WriteFile(path, bytes.Repeat([]byte{0x6d}, len(original)), info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "grow",
			mutate: func(t *testing.T, path string, original []byte, info os.FileInfo) {
				t.Helper()
				if err := os.WriteFile(path, append(append([]byte(nil), original...), 0x7f), info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "truncate",
			mutate: func(t *testing.T, path string, original []byte, info os.FileInfo) {
				t.Helper()
				if err := os.WriteFile(path, original[:len(original)/2], info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "atomic-replace",
			mutate: func(t *testing.T, path string, original []byte, info os.FileInfo) {
				t.Helper()
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, original, info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			original := append(syntheticBinaryHitrace(t), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf", "1.0", syntheticRawPerfData())...)
			if err := os.WriteFile(input, original, 0o640); err != nil {
				t.Fatal(err)
			}
			probe, err := openConversionInputAuthority(input)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := probe.Close(); err != nil {
				t.Fatal(err)
			}
			originalInfo, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			tool, signal, resume := writeSourceGenerationBlockingSimpleperfTool(t, dir)
			defer func() { _ = os.WriteFile(resume, []byte("test cleanup\n"), 0o600) }()
			done := make(chan sourceGenerationConversionOutcome, 1)
			go func() {
				result, err := ConvertFile(context.Background(), Options{
					InputPath: input, OutputPath: output,
					TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
					HiperfPath:        tool,
				})
				done <- sourceGenerationConversionOutcome{result: result, err: err}
			}()
			waitForSourceGenerationSignalOrEarlyResult(t, signal, done)
			test.mutate(t, input, original, originalInfo)
			if err := os.WriteFile(resume, []byte("continue\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			select {
			case outcome := <-done:
				assertStandaloneGenerationError(t, outcome.err, conversionInputStageStandaloneExtract)
				if !reflect.DeepEqual(outcome.result, Result{}) {
					t.Fatalf("generation failure leaked result authority: %+v", outcome.result)
				}
			case <-time.After(sourceGenerationFixtureTimeout):
				t.Fatal("conversion did not leave the controlled standalone adapter stage")
			}
			assertNoStandalonePublication(t, input, output)
		})
	}
}

func TestReleaseStandaloneCancellationDuringAdapterRollsBackTransaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX report adapter")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "cancel.sys")
	output := filepath.Join(dir, "cancel.systrace")
	body := append(syntheticBinaryHitrace(t), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf", "1.0", syntheticRawPerfData())...)
	if err := os.WriteFile(input, body, 0o640); err != nil {
		t.Fatal(err)
	}
	probe, err := openConversionInputAuthority(input)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	tool, signal, resume := writeSourceGenerationBlockingSimpleperfTool(t, dir)
	defer func() { _ = os.WriteFile(resume, []byte("test cleanup\n"), 0o600) }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan sourceGenerationConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(ctx, Options{
			InputPath: input, OutputPath: output,
			TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
			HiperfPath:        tool,
		})
		done <- sourceGenerationConversionOutcome{result: result, err: err}
	}()
	waitForSourceGenerationSignalOrEarlyResult(t, signal, done)
	cancel()
	select {
	case outcome := <-done:
		if !errors.Is(outcome.err, context.Canceled) || !reflect.DeepEqual(outcome.result, Result{}) {
			t.Fatalf("standalone cancellation identity/result drifted: result=%+v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(sourceGenerationFixtureTimeout):
		t.Fatal("canceled standalone adapter did not terminate")
	}
	assertNoStandalonePublication(t, input, output)
}

func TestReleaseStandalonePrimaryPerfSourceSkipsPayloadRead(t *testing.T) {
	payload := bytes.Repeat([]byte{0x7a}, 256*1024)
	block := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf", "1.0", payload)
	input := newScriptedStandaloneInputView("skip-payload.sys", block)
	inventory := standaloneSegmentInventory{
		inputSize: int64(len(block)),
		segments: []standaloneSegment{{
			Offset: 0, Length: int64(len(block)), DataType: profilerDataTypeHiperf,
			PluginName: "hiperf", PluginVersion: "1.0",
		}},
		input: input,
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, caveats, decisions, err := extractStandaloneArtifactsWithOptionsAndLedger(
		context.Background(),
		Options{InputPath: input.path},
		inventory,
		filepath.Join(t.TempDir(), "skip.systrace"),
		standaloneExtractOptions{GeneratePerfTrace: false, PrimaryPerfSource: "trace_streamer DB perf_sample rows"},
		ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 0 || len(decisions) != 0 || len(caveats) != 1 || !strings.Contains(caveats[0], "trace_streamer DB perf_sample rows") {
		t.Fatalf("unexpected skip result: artifacts=%+v caveats=%+v decisions=%+v", artifacts, caveats, decisions)
	}
	for _, end := range input.readEnds {
		if end > profilerStandalonePayloadBase {
			t.Fatalf("skip path read standalone payload: read_ends=%v payload_base=%d", input.readEnds, profilerStandalonePayloadBase)
		}
	}
	if input.reads != 1 || input.reader.Size() != int64(len(block)) || input.counts[conversionInputStageStandaloneExtract] != 2 {
		t.Fatalf("skip path lost header verification or fixed input/entry/exit gates: reads=%d ends=%v size=%d counts=%+v", input.reads, input.readEnds, input.reader.Size(), input.counts)
	}
}

func TestReleaseStandaloneRangeCopyCancellationRollsBack(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "cancel.perf.data")
	body := bytes.Repeat([]byte{0x42}, 256*1024)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelingStandaloneReaderAt{reader: bytes.NewReader(body), cancel: cancel}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	_, err = copyRangeToFileWithLedger(ctx, reader, 0, int64(len(body)), output, ledger)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("range copy cancellation identity=%v", err)
	}
	if reader.reads < 2 {
		t.Fatalf("fixture did not cancel during range copy: reads=%d", reader.reads)
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("canceled range copy retained sidecar %s: %v", output, statErr)
	}
}

func TestReleaseStandaloneForgedInventoryFailsTypedBeforePublication(t *testing.T) {
	for _, test := range []struct {
		name    string
		segment standaloneSegment
	}{
		{name: "negative offset", segment: standaloneSegment{Offset: -1, Length: profilerTraceHeaderSize}},
		{name: "short header", segment: standaloneSegment{Offset: 0, Length: profilerTraceHeaderSize - 1}},
		{name: "range past fixed input", segment: standaloneSegment{Offset: 1024, Length: 1025}},
		{name: "legal range without authority header", segment: standaloneSegment{Offset: 0, Length: 2048, DataType: profilerDataTypeHiperf}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "forged.systrace")
			input := newScriptedStandaloneInputView(filepath.Join(dir, "forged.sys"), make([]byte, 2048))
			inventory := standaloneSegmentInventory{inputSize: input.Size(), segments: []standaloneSegment{test.segment}, input: input}
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, err = extractStandaloneArtifactsWithOptionsAndLedger(
				context.Background(), Options{InputPath: input.path}, inventory, output,
				standaloneExtractOptions{GeneratePerfTrace: true}, ledger,
			)
			var typed *ConversionInputError
			if !errors.As(err, &typed) || typed.Code != ConversionInputCodeInternalContract || typed.Stage != conversionInputStageStandaloneExtract.String() {
				t.Fatalf("forged inventory error=%T %v", err, err)
			}
			assertNoStandalonePublication(t, input.path, output)
		})
	}
}

func TestReleaseConvertFileStandaloneCensusIsSingleAuthorityOwned(t *testing.T) {
	convertBody := sourceGenerationFunctionBody(t, "convert.go", "ConvertFile")
	if count := strings.Count(convertBody, "findStandaloneSegmentsFromInput(ctx, authority)"); count != 1 {
		t.Fatalf("ConvertFile standalone census count=%d want=1:\n%s", count, convertBody)
	}
	for _, forbidden := range []string{"inputContainsStandalonePerfSidecar(", "findStandaloneSegmentsAtPathForStatus("} {
		if strings.Contains(convertBody, forbidden) {
			t.Fatalf("ConvertFile regained standalone path scanner %q:\n%s", forbidden, convertBody)
		}
	}
	for _, required := range []string{
		"convertTraceStreamerOnly(ctx, opts, plan, standaloneInventory, output, ledger)",
		"extractStandaloneArtifactsWithOptionsAndLedger(ctx, opts, standaloneInventory, output, standaloneExtractOpts, ledger)",
	} {
		if !strings.Contains(convertBody, required) {
			t.Fatalf("ConvertFile did not pass the one inventory through %q:\n%s", required, convertBody)
		}
	}

	scanBody := sourceGenerationFunctionBody(t, "standalone.go", "findStandaloneSegmentsFromInput")
	if strings.Count(scanBody, "completeConversionInputStage(ctx, input, conversionInputStageStandaloneScan") < 2 || strings.Contains(scanBody, "os.Open(") {
		t.Fatalf("authority scanner lost entry/exit gates or reopened a path:\n%s", scanBody)
	}
	extractBody := sourceGenerationFunctionBody(t, "standalone.go", "extractStandaloneArtifactsWithOptionsAndLedger")
	if strings.Count(extractBody, "completeConversionInputStage(ctx, input, conversionInputStageStandaloneExtract") < 3 || strings.Contains(extractBody, "os.Open(") {
		t.Fatalf("authority extractor lost stage gates or reopened a path:\n%s", extractBody)
	}
	copyAt := strings.Index(extractBody, "copyRangeToFileWithLedger(")
	if copyAt < 0 {
		t.Fatalf("authority extractor lost range copy:\n%s", extractBody)
	}
	assertSourceGenerationOrder(t, extractBody[copyAt:],
		"copyRangeToFileWithLedger(",
		"completeConversionInputStage(ctx, input, conversionInputStageStandaloneExtract, nil)",
		"maybeConvertHiperfPerfData(",
	)
	for _, check := range []struct {
		file     string
		function string
		want     string
	}{
		{file: "standalone.go", function: "readStandaloneSegmentAt", want: "reader io.ReaderAt"},
		{file: "standalone.go", function: "copyRangeToFileWithLedger", want: "in io.ReaderAt"},
	} {
		body := sourceGenerationFunctionBody(t, check.file, check.function)
		if !strings.Contains(body, check.want) {
			t.Fatalf("%s lost ReaderAt contract %q:\n%s", check.function, check.want, body)
		}
	}
	explicitBody := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "convertTraceStreamerOnly")
	for _, forbidden := range []string{"findStandaloneSegments", "statusInputContainsStandalonePerfSidecar", "os.Open("} {
		if strings.Contains(explicitBody, forbidden) {
			t.Fatalf("explicit trace_streamer path regained source reopen %q:\n%s", forbidden, explicitBody)
		}
	}
	statusBody := sourceGenerationFunctionBody(t, "standalone.go", "findStandaloneSegmentsAtPathForStatus")
	if strings.Count(statusBody, "openConversionInputAuthority(") != 1 || strings.Contains(statusBody, "os.Open(") {
		t.Fatalf("status standalone wrapper is not a one-authority compatibility lane:\n%s", statusBody)
	}
	assertStandaloneInventoryHasOneProductionConstructor(t)
}

func assertStandaloneInventoryHasOneProductionConstructor(t *testing.T) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve standalone release-contract test path")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	constructors := make([]token.Position, 0, 1)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || len(literal.Elts) == 0 {
				return true
			}
			ident, ok := literal.Type.(*ast.Ident)
			if ok && ident.Name == "standaloneSegmentInventory" {
				constructors = append(constructors, fset.Position(literal.Pos()))
			}
			return true
		})
	}
	if len(constructors) != 1 || filepath.Base(constructors[0].Filename) != "standalone.go" {
		t.Fatalf("standalone inventory must have one production constructor in scanner, got %v", constructors)
	}
	scanBody := sourceGenerationFunctionBody(t, "standalone.go", "findStandaloneSegmentsFromInput")
	if !strings.Contains(scanBody, "standaloneSegmentInventory{inputSize: size, segments: segments, input: input}") {
		t.Fatalf("the sole standalone inventory constructor escaped the authority scanner:\n%s", scanBody)
	}
}

func assertStandaloneGenerationError(t *testing.T, err error, stage conversionInputStage) {
	t.Helper()
	var typed *ConversionInputError
	if !errors.As(err, &typed) || typed.Code != ConversionInputCodeGenerationChanged || typed.Stage != stage.String() {
		t.Fatalf("error=%T %v want typed source generation failure at %s", err, err, stage)
	}
}

func assertNoStandalonePublication(t *testing.T, input, output string) {
	t.Helper()
	base := traceSidecarBase(input, output)
	patterns := []string{
		base + "*.perf.data",
		base + "*.perf_*.data",
		base + "*.perftrace",
		base + ".tracebundle.json",
		output,
		filepath.Join(filepath.Dir(base), "."+filepath.Base(base)+"*.simpleperf"),
		filepath.Join(filepath.Dir(base), "."+filepath.Base(base)+"*.hiperf"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("failed standalone transaction retained publication for %q: %v", pattern, matches)
		}
	}
}
