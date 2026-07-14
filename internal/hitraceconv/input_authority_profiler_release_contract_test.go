package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func profilerAuthorityFixture(route string) []byte {
	line := "worker-7 (7) [001] .... 1.000000: print: B|7|ProfilerAuthority"
	if route == "trace_file" {
		return syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(line)))
	}
	return []byte(strings.Join([]string{profilerSessionJSONTag, line, ""}, "\n"))
}

func TestReleaseProfilerReaderAtHeaderBodyParity(t *testing.T) {
	for _, route := range []string{"trace_file", "session"} {
		t.Run(route, func(t *testing.T) {
			body := profilerAuthorityFixture(route)
			dir := t.TempDir()
			namespace := filepath.Join(dir, route+".htrace")
			input := newScriptedStandaloneInputView(namespace, body)
			binding, err := newProfilerInputBinding(input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
				context.Background(), binding, binding.inputSize, sink)
			if err != nil {
				t.Fatalf("ReaderAt profiler extraction: %v", err)
			}
			if !extracted.Detected || extracted.TextRows != 1 || sink.stats.RowsAccepted != 1 {
				t.Fatalf("unexpected ReaderAt profiler extraction: extracted=%+v sink=%+v", extracted, sink.stats)
			}
			if input.counts[conversionInputStageProfilerHeader] != 2 ||
				input.counts[conversionInputStageProfilerBody] != 2 {
				t.Fatalf("profiler entry/exit gate drifted: %+v", input.counts)
			}
			for _, end := range input.readEnds {
				if end > int64(len(body)) {
					t.Fatalf("profiler ReaderAt crossed fixed input: end=%d size=%d", end, len(body))
				}
			}
			var core bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &core)
			if err != nil {
				t.Fatal(err)
			}
			if stats.RowsWritten != 1 {
				t.Fatalf("ReaderAt core rows=%d want=1", stats.RowsWritten)
			}

			inputPath := filepath.Join(dir, route+"-convert.htrace")
			outputPath := filepath.Join(dir, route+".systrace")
			if err := os.WriteFile(inputPath, body, 0o640); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{
				InputPath: inputPath, OutputPath: outputPath, TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatalf("ConvertFile profiler parity: %v", err)
			}
			converted, err := os.ReadFile(result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(converted, core.Bytes()) {
				t.Fatalf("ReaderAt and ConvertFile bytes differ:\n--- core ---\n%s\n--- convert ---\n%s", core.Bytes(), converted)
			}
		})
	}
}

func TestReleaseProfilerGenerationStagesClearExtraction(t *testing.T) {
	for _, route := range []string{"trace_file", "session"} {
		for _, stage := range []conversionInputStage{
			conversionInputStageProfilerHeader,
			conversionInputStageProfilerBody,
		} {
			for _, failCall := range []int{1, 2} {
				name := fmt.Sprintf("%s/%s/call-%d", route, stage, failCall)
				t.Run(name, func(t *testing.T) {
					body := profilerAuthorityFixture(route)
					namespace := filepath.Join(t.TempDir(), route+".htrace")
					input := newScriptedStandaloneInputView(namespace, body)
					input.failStage = stage
					input.failCall = failCall
					binding, err := newProfilerInputBinding(input, namespace)
					if err != nil {
						t.Fatal(err)
					}
					sink, err := newTraceDBRowSink(t.TempDir(), 128)
					if err != nil {
						t.Fatal(err)
					}
					defer sink.cleanup()
					extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
						context.Background(), binding, binding.inputSize, sink)
					assertProfilerInputError(t, err, ConversionInputCodeGenerationChanged, stage)
					if !profilerExtractionZero(extracted) || sink.publishableRows() != 0 {
						t.Fatalf("profiler generation failure leaked authority: extracted=%+v publishable=%d", extracted, sink.publishableRows())
					}
				})
			}
		}
	}
}

func TestReleaseProfilerHeaderToBodyPhysicalGenerationChanges(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(t *testing.T, path string, original []byte, info os.FileInfo)
	}{
		{
			name: "same-size-restored-mtime",
			mutate: func(t *testing.T, path string, original []byte, info os.FileInfo) {
				t.Helper()
				if err := os.WriteFile(path, bytes.Repeat([]byte{0x6a}, len(original)), info.Mode().Perm()); err != nil {
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
	for _, route := range []string{"trace_file", "session"} {
		for _, mutation := range mutations {
			t.Run(route+"/"+mutation.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), route+".htrace")
				original := profilerAuthorityFixture(route)
				if err := os.WriteFile(path, original, 0o640); err != nil {
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
				binding, err := newProfilerInputBinding(authority, authority.CanonicalPath())
				if err != nil {
					t.Fatal(err)
				}
				header, headerOK, err := readProfilerTraceHeaderFromInput(context.Background(), binding)
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				mutation.mutate(t, path, original, info)
				extracted, err := extractProfilerBodyForRoute(t, route, binding, header, headerOK)
				assertProfilerInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageProfilerBody)
				if !profilerExtractionZero(extracted) {
					t.Fatalf("physical generation change leaked extraction: %+v", extracted)
				}
			})
		}
	}
}

func TestReleaseProfilerSymlinkRetargetFailsBodyAndKeepsFrozenNamespace(t *testing.T) {
	for _, route := range []string{"trace_file", "session"} {
		t.Run(route, func(t *testing.T) {
			dir := t.TempDir()
			first := filepath.Join(dir, " first target ")
			second := filepath.Join(dir, "second-target")
			body := profilerAuthorityFixture(route)
			if err := os.WriteFile(first, body, 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(second, body, 0o640); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "capture.htrace")
			if err := os.Symlink(first, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			authority, err := openConversionInputAuthority(link)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			binding, err := newProfilerInputBinding(authority, authority.CanonicalPath())
			if err != nil {
				t.Fatal(err)
			}
			wantNamespace, err := filepath.EvalSymlinks(first)
			if err != nil || binding.sourceNamespace != wantNamespace {
				t.Fatalf("frozen namespace=%q want=%q err=%v", binding.sourceNamespace, wantNamespace, err)
			}
			header, headerOK, err := readProfilerTraceHeaderFromInput(context.Background(), binding)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(second, link); err != nil {
				t.Fatal(err)
			}
			extracted, err := extractProfilerBodyForRoute(t, route, binding, header, headerOK)
			assertProfilerInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageProfilerBody)
			if !profilerExtractionZero(extracted) {
				t.Fatalf("retargeted symlink leaked extraction: %+v", extracted)
			}
		})
	}
}

func TestReleaseProfilerCaptureNamespacePureEntryPreservesBytes(t *testing.T) {
	namespace := filepath.Join(t.TempDir(), " profiler source with spaces ")
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCaptureForNamespace(namespace); err != nil {
		t.Fatalf("open frozen profiler namespace: %v", err)
	}
	if sink.captureSource != namespace {
		t.Fatalf("pure profiler namespace changed bytes: got=%q want=%q", sink.captureSource, namespace)
	}

	invalid, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer invalid.cleanup()
	if err := invalid.openProfilerCaptureForNamespace("relative/source"); traceDBInvariantReason(err) != "profiler_capture_source_namespace_invalid" {
		t.Fatalf("relative frozen profiler namespace error=%v", err)
	}
}

func TestReleaseProfilerStableSymlinkConvertParity(t *testing.T) {
	for _, route := range []string{"trace_file", "session"} {
		t.Run(route, func(t *testing.T) {
			dir := t.TempDir()
			targetDir := filepath.Join(dir, " profiler stable target ")
			if err := os.Mkdir(targetDir, 0o750); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(targetDir, "capture.htrace")
			link := filepath.Join(dir, "capture.htrace")
			if err := os.WriteFile(target, profilerAuthorityFixture(route), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			authority, err := openConversionInputAuthority(link)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			binding, err := newProfilerInputBinding(authority, authority.CanonicalPath())
			if err != nil {
				authority.Close()
				t.Fatal(err)
			}
			canonical, err := filepath.EvalSymlinks(target)
			if err != nil || binding.sourceNamespace != canonical {
				authority.Close()
				t.Fatalf("stable symlink namespace=%q want=%q err=%v", binding.sourceNamespace, canonical, err)
			}
			if err := authority.Close(); err != nil {
				t.Fatal(err)
			}

			directOutput := filepath.Join(dir, "direct.systrace")
			linkedOutput := filepath.Join(dir, "linked.systrace")
			direct, err := ConvertFile(context.Background(), Options{
				InputPath: target, OutputPath: directOutput, TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatal(err)
			}
			linked, err := ConvertFile(context.Background(), Options{
				InputPath: link, OutputPath: linkedOutput, TraceEngine: traceEngineBuiltin,
			})
			if err != nil {
				t.Fatal(err)
			}
			directBytes, err := os.ReadFile(direct.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			linkedBytes, err := os.ReadFile(linked.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(directBytes, linkedBytes) {
				t.Fatalf("stable symlink changed profiler bytes:\n--- direct ---\n%s\n--- linked ---\n%s", directBytes, linkedBytes)
			}
		})
	}
}

func TestReleaseProfilerGateCancellationMatrix(t *testing.T) {
	traceBody := profilerAuthorityFixture("trace_file")
	traceNamespace := filepath.Join(t.TempDir(), "trace-cancel.htrace")
	traceInput := newScriptedStandaloneInputView(traceNamespace, traceBody)
	traceBinding, err := newProfilerInputBinding(traceInput, traceNamespace)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		suffix string
		call   func(context.Context) (profilerContainerExtraction, error)
	}{
		{
			name:   "header-entry",
			suffix: ".readProfilerTraceHeaderFromInput",
			call: func(ctx context.Context) (profilerContainerExtraction, error) {
				_, _, err := readProfilerTraceHeaderFromInput(ctx, traceBinding)
				return profilerContainerExtraction{}, err
			},
		},
		{
			name:   "header-exit",
			suffix: ".readProfilerTraceHeaderFromInput.func1",
			call: func(ctx context.Context) (profilerContainerExtraction, error) {
				header, ok, err := readProfilerTraceHeaderFromInput(ctx, traceBinding)
				if err == nil || ok || header != (profilerTraceHeader{}) {
					t.Fatalf("header exit cancellation leaked header: header=%+v ok=%t err=%v", header, ok, err)
				}
				return profilerContainerExtraction{}, err
			},
		},
		{
			name:   "trace-body-exit",
			suffix: ".extractProfilerTraceFileFromInput.func1",
			call: func(ctx context.Context) (profilerContainerExtraction, error) {
				header, ok, err := readProfilerTraceHeaderFromInput(context.Background(), traceBinding)
				if err != nil || !ok {
					t.Fatalf("prepare TraceFile header: ok=%t err=%v", ok, err)
				}
				sink, err := newTraceDBRowSink(t.TempDir(), 128)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				return extractProfilerTraceFileFromInput(ctx, traceBinding, header, sink, maxProfilerPluginFrameBytes)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: test.suffix, cancelAt: 1, err: context.Canceled,
			}
			extracted, err := test.call(ctx)
			if !errors.Is(err, context.Canceled) || ctx.polls != 1 || !profilerExtractionZero(extracted) {
				t.Fatalf("gate cancellation escaped atomic result: polls=%d extracted=%+v err=%T %v", ctx.polls, extracted, err, err)
			}
		})
	}

	for _, test := range []struct {
		name   string
		suffix string
	}{
		{name: "session-body-entry", suffix: ".extractProfilerSessionPackageFromInput"},
		{name: "session-body-exit", suffix: ".extractProfilerSessionPackageFromInput.func1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := profilerAuthorityFixture("session")
			namespace := filepath.Join(t.TempDir(), "session-cancel.htrace")
			input := newScriptedStandaloneInputView(namespace, body)
			binding, err := newProfilerInputBinding(input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok, err := readProfilerTraceHeaderFromInput(context.Background(), binding); err != nil || ok {
				t.Fatalf("prepare Session header miss: ok=%t err=%v", ok, err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			ctx := &profilerOuterCancellationContext{
				Context: context.Background(), targetSuffix: test.suffix, cancelAt: 1, err: context.DeadlineExceeded,
			}
			extracted, err := extractProfilerSessionPackageFromInput(
				ctx, binding, binding.inputSize, sink, maxProfilerTextLineBytes)
			if !errors.Is(err, context.DeadlineExceeded) || ctx.polls != 1 || !profilerExtractionZero(extracted) {
				t.Fatalf("Session gate cancellation escaped atomic result: polls=%d extracted=%+v err=%T %v", ctx.polls, extracted, err, err)
			}
		})
	}
}

func TestReleaseProfilerShortAndNoMarkerRemainUndetected(t *testing.T) {
	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "empty", body: nil},
		{name: "short", body: []byte("not-a-profiler")},
		{name: "full-header-no-marker", body: bytes.Repeat([]byte{0x5a}, profilerTraceHeaderSize+128)},
	} {
		t.Run(test.name, func(t *testing.T) {
			namespace := filepath.Join(t.TempDir(), test.name+".htrace")
			input := newScriptedStandaloneInputView(namespace, test.body)
			binding, err := newProfilerInputBinding(input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
				context.Background(), binding, binding.inputSize, sink)
			if err != nil || !profilerExtractionZero(extracted) || sink.stats.RowsAccepted != 0 {
				t.Fatalf("non-profiler input was detected: extracted=%+v sink=%+v err=%v", extracted, sink.stats, err)
			}
			if input.counts[conversionInputStageProfilerHeader] != 2 || input.counts[conversionInputStageProfilerBody] != 2 {
				t.Fatalf("non-profiler gate counts drifted: %+v", input.counts)
			}
		})
	}
}

func TestReleaseProfilerForgedBindingAndHeaderReaderFailClosed(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(canceled, nil, 0, nil); !errors.Is(err, context.Canceled) || !profilerExtractionZero(extracted) {
		t.Fatalf("profiler context did not dominate forged nil binding: extracted=%+v err=%T %v", extracted, err, err)
	}

	body := profilerAuthorityFixture("trace_file")
	namespace := filepath.Join(t.TempDir(), "forged.htrace")
	for _, test := range []struct {
		name   string
		mutate func(*profilerInputBinding)
	}{
		{name: "size", mutate: func(binding *profilerInputBinding) { binding.inputSize++ }},
		{name: "namespace", mutate: func(binding *profilerInputBinding) { binding.sourceNamespace = "relative/source" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newScriptedStandaloneInputView(namespace, body)
			binding, err := newProfilerInputBinding(input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(binding)
			_, _, err = readProfilerTraceHeaderFromInput(context.Background(), binding)
			assertProfilerInputError(t, err, ConversionInputCodeInternalContract, conversionInputStageProfilerHeader)
		})
	}

	input := newScriptedStandaloneInputView(namespace, body)
	binding, err := newProfilerInputBinding(input, namespace)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
		context.Background(), binding, binding.inputSize+1, nil)
	assertProfilerInputError(t, err, ConversionInputCodeInternalContract, conversionInputStageProfilerBody)
	if !profilerExtractionZero(extracted) {
		t.Fatalf("invalid Session boundary leaked extraction: %+v", extracted)
	}

	for _, readErr := range []error{errors.New("reader-at-fault"), io.EOF, io.ErrUnexpectedEOF} {
		fault := &profilerFaultReaderAt{err: readErr}
		if header, ok, err := readProfilerTraceHeaderAtExact(fault, 0, profilerTraceHeaderSize); !errors.Is(err, readErr) || ok || header != (profilerTraceHeader{}) {
			t.Fatalf("fixed-range header reader failure was hidden: header=%+v ok=%t err=%T %v want=%v", header, ok, err, err, readErr)
		}
		fault = &profilerFaultReaderAt{err: readErr}
		if offset, ok, err := profilerSessionJSONMarkerOffsetAt(fault, 64, 64); !errors.Is(err, readErr) || ok || offset != 0 {
			t.Fatalf("fixed-range Session marker failure was hidden: offset=%d ok=%t err=%T %v want=%v", offset, ok, err, err, readErr)
		}
	}
	fault := &profilerFaultReaderAt{err: errors.New("boundary must not read")}
	reads := fault.reads
	if _, ok, err := readProfilerTraceHeaderAtExact(fault, math.MaxInt64-8, math.MaxInt64); err != nil || ok || fault.reads != reads {
		t.Fatalf("overflow-safe short header boundary read=%t err=%v reads=%d/%d", ok, err, fault.reads, reads)
	}
	if offset, ok, err := profilerSessionJSONMarkerOffsetAt(fault, 0, 64); err != nil || ok || offset != 0 || fault.reads != reads {
		t.Fatalf("empty Session boundary read unexpectedly: offset=%d ok=%t err=%v reads=%d/%d", offset, ok, err, fault.reads, reads)
	}
}

func TestReleaseProfilerInputAuthorityStructure(t *testing.T) {
	convertBody := sourceGenerationFunctionBody(t, "convert.go", "ConvertFile")
	if strings.Count(convertBody, "tryConvertProfilerContainerWithLedger(ctx, opts, authority,") != 1 {
		t.Fatalf("ConvertFile lost unique profiler authority handoff:\n%s", convertBody)
	}
	tryBody := sourceGenerationFunctionBody(t, "profiler_container.go", "tryConvertProfilerContainerWithLedger")
	assertSourceGenerationOrder(t, tryBody,
		"newProfilerInputBinding(authority, authority.CanonicalPath())",
		"openProfilerCaptureForNamespace(binding.sourceNamespace)",
		"extractProfilerContainerSystraceRowsWithSessionLimitFromInput(",
		"completeConversionInputStage(ctx, binding.input, conversionInputStageProfilerBody, nil)",
		"sealProfilerCaptureContext(ctx)",
		"os.OpenFile(output",
	)
	for _, function := range []string{
		"readProfilerTraceHeaderFromInput",
		"extractProfilerContainerSystraceRowsWithSessionLimitFromInput",
		"extractProfilerTraceFileFromInput",
		"extractProfilerSessionPackageFromInput",
		"extractProfilerSessionPackageAt",
	} {
		body := sourceGenerationFunctionBody(t, "profiler_container.go", function)
		for _, forbidden := range []string{"os.Open(", "os.ReadFile(", "os.Stat(", "filepath.EvalSymlinks("} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s regained source path operation %q:\n%s", function, forbidden, body)
			}
		}
	}
	headerBody := sourceGenerationFunctionBody(t, "profiler_container.go", "readProfilerTraceHeaderFromInput")
	if strings.Count(headerBody, "completeConversionInputStage(ctx, input, conversionInputStageProfilerHeader") < 2 ||
		!strings.Contains(sourceGenerationFunctionBody(t, "profiler_container.go", "readProfilerTraceHeaderAtExact"), "io.NewSectionReader(") {
		t.Fatalf("profiler header lost stage gates or fixed ReaderAt section:\n%s", headerBody)
	}
	for _, function := range []string{"extractProfilerTraceFileFromInput", "extractProfilerSessionPackageFromInput"} {
		body := sourceGenerationFunctionBody(t, "profiler_container.go", function)
		if strings.Count(body, "completeConversionInputStage(ctx, input, conversionInputStageProfilerBody") < 2 {
			t.Fatalf("%s lost profiler body entry/exit gate:\n%s", function, body)
		}
	}
	pureCapture := sourceGenerationFunctionBody(t, "streamerdb_sorter.go", "openProfilerCaptureForNamespace")
	for _, forbidden := range []string{"filepath.Abs(", "filepath.EvalSymlinks(", "os.Stat("} {
		if strings.Contains(pureCapture, forbidden) {
			t.Fatalf("pure profiler namespace entry regained filesystem resolution %q:\n%s", forbidden, pureCapture)
		}
	}
	assertProfilerBindingSingleProductionConsumer(t)
}

func extractProfilerBodyForRoute(t *testing.T, route string, binding *profilerInputBinding,
	header profilerTraceHeader, headerOK bool,
) (profilerContainerExtraction, error) {
	t.Helper()
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if route == "trace_file" {
		if !headerOK || header.DataType != profilerDataTypeProtobuf {
			t.Fatalf("TraceFile fixture lost protobuf header: ok=%t header=%+v", headerOK, header)
		}
		return extractProfilerTraceFileFromInput(context.Background(), binding, header, sink, maxProfilerPluginFrameBytes)
	}
	if headerOK && header.DataType == profilerDataTypeProtobuf {
		t.Fatalf("Session fixture unexpectedly gained protobuf header: %+v", header)
	}
	return extractProfilerSessionPackageFromInput(context.Background(), binding, binding.inputSize, sink, maxProfilerTextLineBytes)
}

func profilerExtractionZero(extracted profilerContainerExtraction) bool {
	return !extracted.Detected && extracted.Kind == "" && extracted.Messages == 0 &&
		len(extracted.PluginMessages) == 0 && extracted.TextRows == 0 &&
		len(extracted.TraceCoverage) == 0 && len(extracted.Caveats) == 0
}

func assertProfilerInputError(t *testing.T, err error, code ConversionInputErrorCode, stage conversionInputStage) {
	t.Helper()
	var typed *ConversionInputError
	if !errors.As(err, &typed) || typed.Code != code || typed.Stage != stage.String() {
		t.Fatalf("error=%T %v want code=%s stage=%s", err, err, code, stage)
	}
}

type profilerFaultReaderAt struct {
	err   error
	reads int
}

func (reader *profilerFaultReaderAt) ReadAt([]byte, int64) (int, error) {
	reader.reads++
	return 0, reader.err
}

func assertProfilerBindingSingleProductionConsumer(t *testing.T) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve profiler authority test path")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	constructors := 0
	consumers := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		constructors += strings.Count(source, "&profilerInputBinding{")
		consumers += strings.Count(source, "newProfilerInputBinding(")
	}
	// One function declaration plus one production call from the controller.
	if constructors != 1 || consumers != 2 {
		t.Fatalf("profiler input binding authorities drifted: constructors=%d declaration+calls=%d", constructors, consumers)
	}
}

var _ io.ReaderAt = (*profilerFaultReaderAt)(nil)
