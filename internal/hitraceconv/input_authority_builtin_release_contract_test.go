package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type sparseBuiltinSpan struct {
	offset int64
	data   []byte
}

type sparseBuiltinInputView struct {
	size   int64
	path   string
	spans  []sparseBuiltinSpan
	counts map[conversionInputStage]int
}

type cancelingBuiltinInputView struct {
	conversionInputView
	cancel      context.CancelFunc
	readCount   int
	cancelAfter int
}

func (input *cancelingBuiltinInputView) ReadAt(buffer []byte, offset int64) (int, error) {
	input.readCount++
	n, err := input.conversionInputView.ReadAt(buffer, offset)
	if input.cancelAfter > 0 && input.readCount == input.cancelAfter {
		input.cancel()
	}
	return n, err
}

func (input *sparseBuiltinInputView) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("negative sparse read offset %d", offset)
	}
	if offset >= input.size {
		return 0, io.EOF
	}
	read := len(buffer)
	if int64(read) > input.size-offset {
		read = int(input.size - offset)
	}
	clear(buffer[:read])
	readEnd := offset + int64(read)
	for _, span := range input.spans {
		spanEnd := span.offset + int64(len(span.data))
		start := max(offset, span.offset)
		end := min(readEnd, spanEnd)
		if start >= end {
			continue
		}
		copy(buffer[int(start-offset):int(end-offset)], span.data[int(start-span.offset):int(end-span.offset)])
	}
	if read < len(buffer) {
		return read, io.EOF
	}
	return read, nil
}

func (input *sparseBuiltinInputView) Size() int64 { return input.size }

func (input *sparseBuiltinInputView) DisplayPath() string { return input.path }

func (input *sparseBuiltinInputView) Validate(stage conversionInputStage) error {
	if input.counts == nil {
		input.counts = make(map[conversionInputStage]int)
	}
	input.counts[stage]++
	return nil
}

func scanMetadataAtPathForTest(ctx context.Context, path string) (meta *traceMetadata, err error) {
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			meta = nil
		}
	}()
	return scanMetadata(ctx, authority, authority.CanonicalPath())
}

func TestReleaseBuiltinReaderAtMetadataRenderParity(t *testing.T) {
	body := syntheticBinaryHitrace(t)
	dir := t.TempDir()
	virtualPath := filepath.Join(dir, "virtual.sys")
	input := newScriptedStandaloneInputView(virtualPath, body)
	meta, err := scanMetadata(context.Background(), input, virtualPath)
	if err != nil {
		t.Fatalf("scan bytes-backed builtin input: %v", err)
	}
	rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
	if err != nil {
		t.Fatalf("render bytes-backed builtin input: %v", err)
	}
	if len(rows) != 1 || missing != 0 || unknown != 0 || suppressed != 0 || first == 0 || last != first {
		t.Fatalf("unexpected bytes-backed render tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
	}
	if meta.binding == nil || meta.binding.input != input || meta.binding.inputSize != int64(len(body)) || meta.binding.sourceNamespace != virtualPath {
		t.Fatalf("metadata lost input binding: %+v", meta.binding)
	}
	if input.counts[conversionInputStageBuiltinMetadata] != 2 || input.counts[conversionInputStageBuiltinRender] != 2 {
		t.Fatalf("builtin entry/exit gates drifted: %+v", input.counts)
	}
	for _, end := range input.readEnds {
		if end > int64(len(body)) {
			t.Fatalf("builtin ReaderAt crossed fixed input: end=%d size=%d", end, len(body))
		}
	}

	inputPath := filepath.Join(dir, "real.sys")
	outputPath := filepath.Join(dir, "real.systrace")
	if err := os.WriteFile(inputPath, body, 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: inputPath, OutputPath: outputPath, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert builtin parity fixture: %v", err)
	}
	got, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := writeRows(&want, rows); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("ReaderAt core and ConvertFile bytes differ:\n--- core ---\n%s\n--- convert ---\n%s", want.Bytes(), got)
	}
}

func TestReleaseBuiltinGenerationStagesClearMetadataAndRows(t *testing.T) {
	body := syntheticBinaryHitrace(t)
	namespace := filepath.Join(t.TempDir(), "builtin-stage.sys")

	metadataExit := newScriptedStandaloneInputView(namespace, body)
	metadataExit.failStage = conversionInputStageBuiltinMetadata
	metadataExit.failCall = 2
	meta, err := scanMetadata(context.Background(), metadataExit, namespace)
	assertBuiltinInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageBuiltinMetadata)
	if meta != nil {
		t.Fatalf("metadata exit generation failure leaked metadata: %+v", meta)
	}

	malformed := newScriptedStandaloneInputView(namespace, bytes.Repeat([]byte{0}, fileHeaderSize))
	malformed.failStage = conversionInputStageBuiltinMetadata
	malformed.failCall = 2
	meta, err = scanMetadata(context.Background(), malformed, namespace)
	assertBuiltinInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageBuiltinMetadata)
	if meta != nil {
		t.Fatalf("generation verdict did not dominate malformed header: %+v", meta)
	}

	for _, test := range []struct {
		name     string
		failCall int
	}{
		{name: "render entry", failCall: 1},
		{name: "render exit", failCall: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newScriptedStandaloneInputView(namespace, body)
			meta, err := scanMetadata(context.Background(), input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			input.failStage = conversionInputStageBuiltinRender
			input.failCall = test.failCall
			rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
			assertBuiltinInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageBuiltinRender)
			if len(rows) != 0 || missing != 0 || unknown != 0 || suppressed != 0 || first != 0 || last != 0 {
				t.Fatalf("render generation failure leaked tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
			}
		})
	}
}

func TestReleaseBuiltinCancellationDominatesMetadataAndRender(t *testing.T) {
	body := syntheticBinaryHitrace(t)
	namespace := filepath.Join(t.TempDir(), "builtin-cancel.sys")

	metadataBase := newScriptedStandaloneInputView(namespace, body)
	metadataCtx, metadataCancel := context.WithCancel(context.Background())
	metadataInput := &cancelingBuiltinInputView{
		conversionInputView: metadataBase,
		cancel:              metadataCancel,
		cancelAfter:         1,
	}
	meta, err := scanMetadata(metadataCtx, metadataInput, namespace)
	if !errors.Is(err, context.Canceled) || meta != nil {
		t.Fatalf("metadata cancellation leaked result: meta=%+v err=%T %v", meta, err, err)
	}

	renderBase := newScriptedStandaloneInputView(namespace, body)
	renderCtx, renderCancel := context.WithCancel(context.Background())
	renderInput := &cancelingBuiltinInputView{
		conversionInputView: renderBase,
		cancel:              renderCancel,
	}
	meta, err = scanMetadata(context.Background(), renderInput, namespace)
	if err != nil {
		t.Fatal(err)
	}
	renderInput.readCount = 0
	renderInput.cancelAfter = 1
	rows, missing, unknown, suppressed, first, last, err := renderRows(renderCtx, meta)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("render cancellation=%T %v want context.Canceled", err, err)
	}
	if len(rows) != 0 || missing != 0 || unknown != 0 || suppressed != 0 || first != 0 || last != 0 {
		t.Fatalf("render cancellation leaked tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
	}
}

func TestReleaseBuiltinMetadataToRenderPhysicalGenerationChanges(t *testing.T) {
	tests := []struct {
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "physical.sys")
			original := syntheticBinaryHitrace(t)
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
			meta, err := scanMetadata(context.Background(), authority, authority.CanonicalPath())
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, path, original, info)
			rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
			assertBuiltinInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageBuiltinRender)
			if len(rows) != 0 || missing != 0 || unknown != 0 || suppressed != 0 || first != 0 || last != 0 {
				t.Fatalf("physical generation failure leaked tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
			}
		})
	}
}

func TestReleaseBuiltinForgedMetadataFailsTypedBeforeRows(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(meta *traceMetadata)
	}{
		{name: "binding size mismatch", mutate: func(meta *traceMetadata) { meta.binding.inputSize++ }},
		{name: "invalid namespace", mutate: func(meta *traceMetadata) { meta.binding.sourceNamespace = "relative/source.sys" }},
		{name: "range overflow", mutate: func(meta *traceMetadata) {
			meta.segments[0].Offset = math.MaxInt64 - 1
			meta.segments[0].Size = 4096
		}},
		{name: "header mismatch", mutate: func(meta *traceMetadata) { meta.segments[0].Size++ }},
		{name: "physical reorder", mutate: func(meta *traceMetadata) {
			meta.segments[0], meta.segments[1] = meta.segments[1], meta.segments[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := syntheticBinaryHitrace(t)
			namespace := filepath.Join(t.TempDir(), "forged.sys")
			input := newScriptedStandaloneInputView(namespace, body)
			meta, err := scanMetadata(context.Background(), input, namespace)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(meta)
			rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
			assertBuiltinInputError(t, err, ConversionInputCodeInternalContract, conversionInputStageBuiltinRender)
			if len(rows) != 0 || missing != 0 || unknown != 0 || suppressed != 0 || first != 0 || last != 0 {
				t.Fatalf("forged metadata leaked tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
			}
		})
	}
}

func TestReleaseBuiltinSparseInputAboveFourGiB(t *testing.T) {
	format := []byte(syntheticEventFormat())
	page := syntheticRawPage()
	unknownHeaderOffset := int64(fileHeaderSize)
	unknownPayloadOffset := unknownHeaderOffset + segmentHdrSize
	eventHeaderOffset := unknownPayloadOffset + int64(^uint32(0))
	eventPayloadOffset := eventHeaderOffset + segmentHdrSize
	rawHeaderOffset := eventPayloadOffset + int64(len(format))
	rawPayloadOffset := rawHeaderOffset + segmentHdrSize
	terminalOffset := rawPayloadOffset + int64(len(page))
	fixedSize := terminalOffset + segmentHdrSize
	input := &sparseBuiltinInputView{
		size: fixedSize,
		path: filepath.Join(t.TempDir(), "sparse-over-4g.sys"),
		spans: []sparseBuiltinSpan{
			{offset: 0, data: releaseBuiltinHeader(harmonyRMQMagic, harmonyRMQFileType, harmonyRMQVersion)},
			{offset: unknownHeaderOffset, data: builtinSegmentHeader(99, ^uint32(0))},
			{offset: eventHeaderOffset, data: builtinSegmentHeader(segmentEventsFormat, uint32(len(format)))},
			{offset: eventPayloadOffset, data: format},
			{offset: rawHeaderOffset, data: builtinSegmentHeader(segmentRawTrace, uint32(len(page)))},
			{offset: rawPayloadOffset, data: page},
			{offset: terminalOffset, data: builtinSegmentHeader(
				uint32(profilerTraceHeaderMagic&0xffffffff),
				uint32((profilerTraceHeaderMagic>>32)&0xffffffff),
			)},
		},
	}
	if input.Size() <= 1<<32 || rawPayloadOffset <= 1<<32 {
		t.Fatalf("sparse fixture did not cross 4GiB: size=%d raw=%d", input.Size(), rawPayloadOffset)
	}
	meta, err := scanMetadata(context.Background(), input, input.path)
	if err != nil {
		t.Fatalf("scan >4GiB sparse input: %v", err)
	}
	rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
	if err != nil {
		t.Fatalf("render >4GiB sparse input: %v", err)
	}
	if len(rows) != 1 || missing != 0 || unknown != 0 || suppressed != 0 || first == 0 || last != first {
		t.Fatalf("unexpected >4GiB render tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
	}
	if input.counts[conversionInputStageBuiltinMetadata] != 2 || input.counts[conversionInputStageBuiltinRender] != 2 {
		t.Fatalf("sparse input lost stage gates: %+v", input.counts)
	}
	nearLimit := segmentMeta{Offset: math.MaxInt64 - 3, Size: 4}
	if err := validateBuiltinMetadataSegment(input, math.MaxInt64, nearLimit); err == nil {
		t.Fatal("near-MaxInt64 segment range overflow escaped checked subtraction")
	}
}

func TestReleaseBuiltinPairNamespaceIsFrozenAndBytePreserving(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, " target with spaces ")
	if err := os.WriteFile(target, syntheticBinaryHitrace(t), 0o640); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := newDirectPairCaptureBarrierForNamespace(canonical)
	if err != nil || barrier.source != canonical {
		t.Fatalf("pure namespace constructor changed legal path bytes: barrier=%+v err=%v", barrier, err)
	}
	for _, invalid := range []string{"relative/source.sys", canonical + string(os.PathSeparator) + "."} {
		if _, err := newDirectPairCaptureBarrierForNamespace(invalid); err == nil {
			t.Fatalf("pure namespace constructor accepted non-authoritative namespace %q", invalid)
		}
	}

	link := filepath.Join(dir, "capture-link.sys")
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
	defer authority.Close()
	meta, err := scanMetadata(context.Background(), authority, authority.CanonicalPath())
	if err != nil {
		t.Fatal(err)
	}
	if meta.binding.sourceNamespace != canonical {
		t.Fatalf("metadata namespace=%q want frozen target=%q", meta.binding.sourceNamespace, canonical)
	}
	other := filepath.Join(dir, "other.sys")
	if err := os.WriteFile(other, syntheticBinaryHitrace(t), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	rows, missing, unknown, suppressed, first, last, err := renderRows(context.Background(), meta)
	assertBuiltinInputError(t, err, ConversionInputCodeGenerationChanged, conversionInputStageBuiltinRender)
	if len(rows) != 0 || missing != 0 || unknown != 0 || suppressed != 0 || first != 0 || last != 0 {
		t.Fatalf("retargeted symlink leaked render tuple: rows=%+v missing=%d unknown=%d suppressed=%d first=%d last=%d", rows, missing, unknown, suppressed, first, last)
	}
}

func TestReleaseBuiltinAuthorityStructure(t *testing.T) {
	convertBody := sourceGenerationFunctionBody(t, "convert.go", "ConvertFile")
	for _, required := range []string{
		"scanMetadata(ctx, authority, authority.CanonicalPath())",
		"renderRows(ctx, meta)",
	} {
		if strings.Count(convertBody, required) != 1 {
			t.Fatalf("ConvertFile lost singleton builtin authority step %q:\n%s", required, convertBody)
		}
	}
	assertSourceGenerationOrder(t, convertBody,
		"scanMetadata(ctx, authority, authority.CanonicalPath())",
		"renderRows(ctx, meta)",
		"sort.SliceStable(rows",
		"os.OpenFile(output",
	)
	metaAt := strings.Index(convertBody, "meta, err := scanMetadata(ctx, authority")
	if metaAt < 0 {
		t.Fatalf("ConvertFile lost builtin metadata authority call:\n%s", convertBody)
	}
	builtinTail := convertBody[metaAt:]
	partialAt := strings.Index(builtinTail, "hasAnalyzableStandaloneSidecar")
	typedAt := strings.Index(builtinTail, "errors.As(err, &inputErr)")
	contextAt := strings.Index(builtinTail, "errors.Is(err, context.Canceled)")
	if partialAt < 0 || typedAt < 0 || contextAt < 0 || typedAt >= partialAt || contextAt >= partialAt {
		t.Fatalf("builtin input/context failures no longer dominate partial sidecar success:\n%s", convertBody)
	}

	for _, function := range []string{"scanMetadata", "renderRows"} {
		body := sourceGenerationFunctionBody(t, "convert.go", function)
		for _, forbidden := range []string{"os.Open(", "os.ReadFile(", "os.Stat(", "filepath.EvalSymlinks("} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s regained source filesystem operation %q:\n%s", function, forbidden, body)
			}
		}
		if !strings.Contains(body, "io.NewSectionReader(") {
			t.Fatalf("%s lost fixed ReaderAt boundary:\n%s", function, body)
		}
	}
	scanBody := sourceGenerationFunctionBody(t, "convert.go", "scanMetadata")
	if strings.Count(scanBody, "completeConversionInputStage(ctx, input, conversionInputStageBuiltinMetadata") < 2 ||
		!strings.Contains(scanBody, "binding: &builtinInputBinding{") {
		t.Fatalf("metadata scanner lost authority binding or entry/exit gates:\n%s", scanBody)
	}
	renderBody := sourceGenerationFunctionBody(t, "convert.go", "renderRows")
	if strings.Count(renderBody, "completeConversionInputStage(ctx, input, conversionInputStageBuiltinRender") < 2 ||
		!strings.Contains(renderBody, "newDirectPairCaptureBarrierForNamespace(meta.binding.sourceNamespace)") ||
		strings.Contains(renderBody, "newDirectPairCaptureBarrier(") ||
		strings.Contains(renderBody, "for pageOff := 0; pageOff+tracePageSize") {
		t.Fatalf("builtin renderer lost stage, frozen namespace, or 32-bit loop contract:\n%s", renderBody)
	}
	fallbackBody := sourceGenerationFunctionBody(t, "convert.go", "builtinTraceBodyFallbackReasons")
	if !strings.Contains(fallbackBody, "profilerSessionJSONMarkerOffsetAt(input, input.Size()") || strings.Contains(fallbackBody, "profilerSessionJSONMarkerOffset(") {
		t.Fatalf("builtin fallback diagnostic regained source path reopen:\n%s", fallbackBody)
	}
	pureBarrier := sourceGenerationFunctionBody(t, "direct_pair_barrier.go", "newDirectPairCaptureBarrierForNamespace")
	for _, forbidden := range []string{"filepath.Abs(", "filepath.EvalSymlinks(", "os.Stat("} {
		if strings.Contains(pureBarrier, forbidden) {
			t.Fatalf("pure pair namespace constructor regained filesystem resolution %q:\n%s", forbidden, pureBarrier)
		}
	}
	assertBuiltinBindingHasOneProductionConstructor(t)
}

func assertBuiltinBindingHasOneProductionConstructor(t *testing.T) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve builtin release-contract test path")
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
			if !ok {
				return true
			}
			ident, ok := literal.Type.(*ast.Ident)
			if ok && ident.Name == "builtinInputBinding" {
				constructors = append(constructors, fset.Position(literal.Pos()))
			}
			return true
		})
	}
	if len(constructors) != 1 || filepath.Base(constructors[0].Filename) != "convert.go" {
		t.Fatalf("builtin input binding must have one production constructor in scanner, got %v", constructors)
	}
	scanBody := sourceGenerationFunctionBody(t, "convert.go", "scanMetadata")
	if !strings.Contains(scanBody, "binding: &builtinInputBinding{") {
		t.Fatalf("the sole builtin input binding constructor escaped metadata scan:\n%s", scanBody)
	}
}

func builtinSegmentHeader(segmentType, segmentSize uint32) []byte {
	header := make([]byte, segmentHdrSize)
	binary.LittleEndian.PutUint32(header[0:4], segmentType)
	binary.LittleEndian.PutUint32(header[4:8], segmentSize)
	return header
}

func assertBuiltinInputError(t *testing.T, err error, code ConversionInputErrorCode, stage conversionInputStage) {
	t.Helper()
	var typed *ConversionInputError
	if !errors.As(err, &typed) || typed.Code != code || typed.Stage != stage.String() {
		t.Fatalf("error=%T %v want code=%s stage=%s", err, err, code, stage)
	}
}
