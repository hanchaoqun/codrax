package hitraceconv

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const tracePerfAutoFallbackCaveat = "trace+perf auto conversion uses trace_streamer/SQLite first and falls back to built-in raw trace parsing when SQL is unavailable or fails; explicit trace engine modes do not fall back"

type traceMetadata struct {
	header               fileHeader
	formats              map[int]eventFormat
	formatPoisoned       map[int]bool
	formatPoisonFamilies map[int]pairCriticalFormatFamilyMask
	formatConflictRows   int
	printkFormats        map[uint64]string
	printkPoisoned       map[uint64]bool
	printkMalformed      int
	bodyRejectedRows     int
	bodyRejectReasons    map[string]int
	pairQuarantinedRows  int
	pairPoisonedLanes    int
	pairPoisonedFamilies int
	pairBarrierBudget    bool
	cmdlines             map[int]string
	tgids                map[int]int
	segments             []segmentMeta
}

type renderedRow struct {
	tsNS           uint64
	seq            int
	line           string
	pairKind       pairRenderKind
	structuredPair bool
}

// ConvertFile converts a binary Harmony/OpenHarmony HiTrace capture to a
// ftrace/systrace-compatible text file. It never overwrites the output path.
func ConvertFile(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	input := strings.TrimSpace(opts.InputPath)
	if input == "" {
		return Result{}, fmt.Errorf("input path is required")
	}
	if err := validatePerfParserMode(opts.PerfParser); err != nil {
		return Result{}, err
	}
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return Result{}, err
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(input)
	}
	info, err := os.Stat(input)
	if err != nil {
		return Result{}, err
	}
	if normalizeTraceEngineMode(opts.TraceEngine) == traceEngineTraceStreamer {
		return convertTraceStreamerOnly(ctx, opts, input, info.Size(), output)
	}
	if result, ok, err := maybeConvertDirectSimpleperfPerfData(ctx, opts, input, info.Size(), output); ok || err != nil {
		return result, err
	}
	if err := ensureOutputDoesNotExist(output); err != nil {
		return Result{}, err
	}
	hasTracePerfSidecar, err := inputContainsStandalonePerfSidecar(ctx, input, info.Size())
	if err != nil {
		return Result{}, err
	}
	traceStreamerExport, err := maybeRunTraceStreamerAuto(ctx, opts, input, info.Size(), output)
	if err != nil {
		return Result{}, err
	}
	var initialArtifacts []Artifact
	var initialCaveats []string
	var initialTraceDecisions []TraceProviderDecision
	var initialTraceDBCoverage []TraceDBCoverage
	var initialTraceCoverage []TraceDBCoverage
	if traceStreamerExport.Decision.ProviderName != "" {
		initialTraceDecisions = append(initialTraceDecisions, traceStreamerExport.Decision)
		initialCaveats = append(initialCaveats, traceStreamerExport.Caveats...)
		initialTraceDBCoverage = append(initialTraceDBCoverage, traceStreamerExport.Coverage...)
		initialTraceCoverage = append(initialTraceCoverage, traceStreamerExport.TraceCoverage...)
	}
	keepTraceDBArtifact := opts.KeepTraceDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" ||
		(hasTracePerfSidecar && traceStreamerExport.Artifact.Path != "" && traceStreamerExport.SystraceArtifact.Path == "")
	if traceStreamerExport.Artifact.Path != "" && keepTraceDBArtifact && artifactPathExists(traceStreamerExport.Artifact.Path) {
		initialArtifacts = append(initialArtifacts, traceStreamerExport.Artifact)
	} else if traceStreamerExport.Cleanup != nil {
		defer traceStreamerExport.Cleanup()
	}
	standaloneExtractOpts := standaloneExtractOptions{GeneratePerfTrace: true}
	if traceStreamerExport.SystraceArtifact.Path != "" && traceDBCoverageHasPerfSamples(initialTraceDBCoverage) {
		standaloneExtractOpts.GeneratePerfTrace = false
		standaloneExtractOpts.PrimaryPerfSource = "trace_streamer DB perf_sample rows embedded in systrace"
	}
	standaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifactsWithOptions(ctx, opts, info.Size(), output, standaloneExtractOpts)
	if err != nil {
		if traceStreamerExport.SystraceArtifact.Path != "" {
			_ = os.Remove(traceStreamerExport.SystraceArtifact.Path)
		}
		return Result{}, err
	}
	standaloneArtifacts = append(initialArtifacts, standaloneArtifacts...)
	standaloneCaveats = append(initialCaveats, standaloneCaveats...)
	if traceStreamerExport.SystraceArtifact.Path != "" {
		result := Result{
			InputPath:          input,
			OutputPath:         traceStreamerExport.SystraceArtifact.Path,
			InputBytes:         info.Size(),
			OutputBytes:        traceStreamerExport.OutputBytes,
			Artifacts:          append([]Artifact{traceStreamerExport.SystraceArtifact}, standaloneArtifacts...),
			ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
			TraceDecisions:     append([]TraceProviderDecision(nil), initialTraceDecisions...),
			TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
			TraceCoverage:      append([]TraceDBCoverage(nil), initialTraceCoverage...),
			Caveats:            standaloneCaveats,
			EventsWritten:      traceStreamerExport.EventsWritten,
			FirstTimestampSec:  traceStreamerExport.FirstTimestampSec,
			LastTimestampSec:   traceStreamerExport.LastTimestampSec,
			MissingFormatCount: 0,
			UnknownEventCount:  0,
		}
		normalizeResultCollections(&result)
		if bundleArtifact, err := writeTraceBundleWithAllCoverage(input, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); err != nil {
			return Result{}, err
		} else if bundleArtifact.Path != "" {
			result.BundlePath = bundleArtifact.Path
			result.Artifacts = append(result.Artifacts, bundleArtifact)
		}
		normalizeResultCollections(&result)
		return result, nil
	}
	if traceStreamerSelectionShouldStopBuiltinFallback(opts, traceStreamerExport) {
		result := Result{
			InputPath:         input,
			InputBytes:        info.Size(),
			Artifacts:         append([]Artifact(nil), standaloneArtifacts...),
			ProviderDecisions: append([]PerfProviderDecision(nil), standaloneDecisions...),
			TraceDecisions:    append([]TraceProviderDecision(nil), initialTraceDecisions...),
			TraceDBCoverage:   append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
			TraceCoverage:     append([]TraceDBCoverage(nil), initialTraceCoverage...),
			Caveats:           append(append([]string(nil), initialCaveats...), standaloneCaveats...),
		}
		result.Artifacts = append(initialArtifacts, result.Artifacts...)
		result.Caveats = append(result.Caveats, "systrace output was not produced because selected trace_streamer/SQLite trace conversion did not produce trace_query-ready rows; pass --trace-engine=builtin to use the built-in trace-only converter")
		normalizeResultCollections(&result)
		if bundleArtifact, err := writeTraceBundleWithAllCoverage(input, output, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); err != nil {
			return Result{}, err
		} else if bundleArtifact.Path != "" {
			result.BundlePath = bundleArtifact.Path
			result.Artifacts = append(result.Artifacts, bundleArtifact)
		}
		normalizeResultCollections(&result)
		return result, nil
	}
	if result, ok, err := tryConvertProfilerContainer(ctx, opts, info.Size(), output, standaloneArtifacts, standaloneCaveats, standaloneDecisions, initialTraceDecisions, initialTraceDBCoverage); ok || err != nil {
		return result, err
	}
	meta, err := scanMetadata(ctx, input, info.Size())
	if err != nil {
		if len(standaloneArtifacts) > 0 {
			fallbackCaveat := builtinTraceBodyFallbackFailureCaveat(input, info.Size(), standaloneArtifacts, err)
			result := Result{
				InputPath:  input,
				InputBytes: info.Size(),
				Artifacts:  append([]Artifact(nil), standaloneArtifacts...),
				ProviderDecisions: append([]PerfProviderDecision(nil),
					standaloneDecisions...),
				TraceDecisions: append(initialTraceDecisions,
					traceProviderFailure(
						newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys), opts, input, output),
						"no_renderable_trace_body",
						fallbackCaveat,
					),
				),
				Caveats: append(standaloneCaveats,
					fallbackCaveat),
				TraceDBCoverage: append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
				TraceCoverage:   append([]TraceDBCoverage(nil), initialTraceCoverage...),
			}
			normalizeResultCollections(&result)
			if bundleArtifact, bundleErr := writeTraceBundleWithAllCoverage(input, "", result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); bundleErr != nil {
				return Result{}, bundleErr
			} else if bundleArtifact.Path != "" {
				result.BundlePath = bundleArtifact.Path
				result.Artifacts = append(result.Artifacts, bundleArtifact)
			}
			normalizeResultCollections(&result)
			return result, nil
		}
		return Result{}, err
	}
	rows, missing, unknown, suppressed, first, last, err := renderRows(ctx, input, meta)
	if err != nil {
		return Result{}, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].tsNS == rows[j].tsNS {
			return rows[i].seq < rows[j].seq
		}
		return rows[i].tsNS < rows[j].tsNS
	})
	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return Result{}, err
	}
	writeErr := writeRows(out, rows)
	closeErr := out.Close()
	if writeErr != nil {
		_ = os.Remove(output)
		return Result{}, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(output)
		return Result{}, closeErr
	}
	outInfo, err := os.Stat(output)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		InputPath:   input,
		OutputPath:  output,
		InputBytes:  info.Size(),
		OutputBytes: outInfo.Size(),
		Artifacts: []Artifact{{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     outInfo.Size(),
			Converter: converterVersion,
		}},
		TraceDecisions: append(initialTraceDecisions,
			traceProviderSuccess(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys), opts, input, output),
				Artifact{Type: ArtifactSystrace, Path: output},
			),
		),
		EventsWritten:      len(rows),
		MissingFormatCount: missing,
		UnknownEventCount:  unknown,
		FirstTimestampSec:  float64(first) / 1e9,
		LastTimestampSec:   float64(last) / 1e9,
		TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
		TraceCoverage:      append([]TraceDBCoverage(nil), initialTraceCoverage...),
	}
	result.Artifacts = append(result.Artifacts, standaloneArtifacts...)
	result.ProviderDecisions = append(result.ProviderDecisions, standaloneDecisions...)
	if missing > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw event(s) had no event format and were skipped to keep systrace output compatible with official parsers", missing))
	}
	if unknown > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d event row(s) lacked an official-compatible renderer and were emitted as header-only rows", unknown))
	}
	if suppressed > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw ftrace event row(s) had a missing, mistyped, duplicate, or truncated common-field envelope and were suppressed without fabricating an idle/PID0 header", suppressed))
	}
	if len(meta.formatPoisoned) > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d conflicting or malformed raw ftrace event descriptor ID(s) were quarantined for the complete capture; separate exactly equal parsed descriptor blocks are idempotent, but repetition inside one block is malformed and later descriptors cannot rescue a quarantined ID", len(meta.formatPoisoned)))
	}
	if meta.formatConflictRows > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw ftrace event row(s) referenced a conflicting or malformed descriptor ID and were kept coverage-only instead of being decoded with an ambiguous layout", meta.formatConflictRows))
	}
	if len(meta.printkPoisoned) > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d conflicting or malformed printk address mapping(s) were quarantined for the complete capture; later mappings cannot rescue a quarantined address", len(meta.printkPoisoned)))
	}
	if meta.printkMalformed > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d printk_formats line(s) were malformed without a usable address and were ignored", meta.printkMalformed))
	}
	if meta.bodyRejectedRows > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d governed direct ftrace event row(s) had rejected physical payloads and were kept coverage-only instead of falling back to header-only rows; reasons=%s", meta.bodyRejectedRows, traceDBCountSummary(meta.bodyRejectReasons)))
	}
	if meta.pairQuarantinedRows > 0 || meta.pairPoisonedFamilies > 0 || meta.pairPoisonedLanes > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("direct pair-critical publication completed a full-capture anti-rescue freeze before output: withheld_rows=%d poisoned_lanes=%d poisoned_families=%d budget_fail_closed=%t; rejected Workqueue/DMA/MMC endpoints remain coverage-only and cannot be bridged by neighboring rows", meta.pairQuarantinedRows, meta.pairPoisonedLanes, meta.pairPoisonedFamilies, meta.pairBarrierBudget))
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	normalizeResultCollections(&result)
	if bundleArtifact, err := writeTraceBundleWithAllCoverage(input, output, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); err != nil {
		return Result{}, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
	return result, nil
}

func builtinTraceBodyFallbackFailureCaveat(input string, inputSize int64, artifacts []Artifact, err error) string {
	reasons := builtinTraceBodyFallbackReasons(input, inputSize, artifacts)
	if len(reasons) == 0 {
		reasons = append(reasons, "no supported trace body was found")
	}
	message := "systrace output was not produced because " + strings.Join(reasons, "; ")
	if err != nil {
		message += fmt.Sprintf("; built-in sys parser rejected the fallback probe: %v", err)
	}
	return message
}

func builtinTraceBodyFallbackReasons(input string, inputSize int64, artifacts []Artifact) []string {
	var reasons []string
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfData || artifact.SourceOffset != 0 {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("input starts with OpenHarmony profiler standalone perf sidecar data_type=%d plugin=%s version=%s bytes=%d rather than a trace body", artifact.DataType, firstNonEmpty(artifact.PluginName, "unknown"), firstNonEmpty(artifact.PluginVersion, "unknown"), artifact.SourceBytes))
		break
	}
	if off, ok, err := profilerSessionJSONMarkerOffset(input, 64*1024); err == nil && ok {
		reasons = append(reasons, fmt.Sprintf("input contains OpenHarmony profiler SessionJSON package marker at offset %d but no directly renderable systrace rows were extracted before fallback", off))
	}
	if len(reasons) == 0 {
		perfSidecars := 0
		for _, artifact := range artifacts {
			if artifact.Type == ArtifactPerfData {
				perfSidecars++
			}
		}
		if perfSidecars > 0 {
			reasons = append(reasons, fmt.Sprintf("%d standalone perf sidecar artifact(s) were preserved, but no supported trace body was found", perfSidecars))
		}
	}
	_ = inputSize
	return reasons
}

func traceDBCoverageHasPerfSamples(coverage []TraceDBCoverage) bool {
	for _, item := range coverage {
		if strings.EqualFold(strings.TrimSpace(item.Family), "perf") &&
			strings.EqualFold(strings.TrimSpace(item.Table), "perf_sample") &&
			item.RowsEmitted > 0 {
			return true
		}
	}
	return false
}

func traceStreamerSelectionShouldStopBuiltinFallback(opts Options, traceStreamerExport traceStreamerExportResult) bool {
	if selectedTraceEngineMode(opts.TraceEngine) != traceEngineTraceStreamer {
		return false
	}
	return !isAutoTraceEngineMode(opts.TraceEngine)
}

func scanMetadata(ctx context.Context, path string, size int64) (*traceMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	header, err := readFileHeader(f)
	if err != nil {
		return nil, err
	}
	if header.FileType != harmonyRMQFileType {
		return nil, &BuiltinSysDecodeError{
			Code:     builtinSysDecodeUnsupportedFileType,
			FileType: header.FileType,
			Detail:   "the built-in decoder supports Harmony RMQ file_type=1 only; OpenHarmony/Linux file_type=0 requires trace_streamer",
		}
	}
	meta := &traceMetadata{
		header:               header,
		formats:              map[int]eventFormat{},
		formatPoisoned:       map[int]bool{},
		formatPoisonFamilies: map[int]pairCriticalFormatFamilyMask{},
		printkFormats:        map[uint64]string{},
		printkPoisoned:       map[uint64]bool{},
		cmdlines:             map[int]string{},
		tgids:                map[int]int{},
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		typ, segSize, err := readSegmentHeader(f)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read segment header at %d: %w", pos, err)
		}
		if isProfilerTraceHeaderPrefix(typ, segSize) {
			break
		}
		payloadOffset, _ := f.Seek(0, io.SeekCurrent)
		if segSize > uint32(size) || payloadOffset+int64(segSize) > size {
			return nil, fmt.Errorf("invalid segment type=%d size=%d at offset=%d", typ, segSize, pos)
		}
		meta.segments = append(meta.segments, segmentMeta{Type: typ, Size: segSize, Offset: payloadOffset})
		switch {
		case typ == segmentEventsFormat || typ == segmentCmdlines || typ == segmentTGIDs || typ == segmentPrintk:
			data := make([]byte, segSize)
			if _, err := io.ReadFull(f, data); err != nil {
				return nil, fmt.Errorf("read segment type=%d: %w", typ, err)
			}
			switch typ {
			case segmentEventsFormat:
				formats, err := parseEventFormats(data)
				if err != nil {
					return nil, err
				}
				catalog := eventFormatCatalog{
					Formats:          meta.formats,
					Poisoned:         meta.formatPoisoned,
					PoisonedFamilies: meta.formatPoisonFamilies,
				}
				mergeEventFormatCatalog(&catalog, formats)
				meta.formats = catalog.Formats
				meta.formatPoisoned = catalog.Poisoned
				meta.formatPoisonFamilies = catalog.PoisonedFamilies
			case segmentCmdlines:
				for pid, comm := range parseCmdlines(data) {
					meta.cmdlines[pid] = comm
				}
			case segmentTGIDs:
				for pid, tgid := range parseTGIDs(data) {
					meta.tgids[pid] = tgid
				}
			case segmentPrintk:
				catalog := printkFormatCatalog{Formats: meta.printkFormats, Poisoned: meta.printkPoisoned, Malformed: meta.printkMalformed}
				mergePrintkFormatCatalog(&catalog, parsePrintkFormats(data))
				meta.printkFormats, meta.printkPoisoned, meta.printkMalformed = catalog.Formats, catalog.Poisoned, catalog.Malformed
			}
		default:
			if _, err := f.Seek(int64(segSize), io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("skip segment type=%d: %w", typ, err)
			}
		}
	}
	if len(meta.formats) == 0 && len(meta.formatPoisoned) == 0 {
		return nil, fmt.Errorf("no event format segment found; not a supported binary hitrace file")
	}
	return meta, nil
}

func renderRows(ctx context.Context, path string, meta *traceMetadata) ([]renderedRow, int, int, int, uint64, uint64, error) {
	if meta != nil {
		meta.formatConflictRows = 0
		meta.bodyRejectedRows = 0
		meta.bodyRejectReasons = make(map[string]int)
		meta.pairQuarantinedRows = 0
		meta.pairPoisonedLanes = 0
		meta.pairPoisonedFamilies = 0
		meta.pairBarrierBudget = false
	}
	pairBarrier, err := newDirectPairCaptureBarrier(path)
	if err != nil {
		return nil, 0, 0, 0, 0, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, 0, 0, 0, err
	}
	defer f.Close()
	var rows []renderedRow
	missing := 0
	unknown := 0
	suppressed := 0
	var first uint64
	var last uint64
	seq := 0
	rc := renderContext{
		cmdlines:       meta.cmdlines,
		tgids:          meta.tgids,
		printkFormats:  meta.printkFormats,
		printkPoisoned: meta.printkPoisoned,
	}
	for _, seg := range meta.segments {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		if !isRawTraceSegment(seg.Type, meta.header.CPUNum) {
			continue
		}
		// The Harmony raw-trace contract stores fixed 4096-byte
		// RmqConsumerData pages. Treat a remainder as truncation instead of
		// silently dropping bytes at the end of the segment.
		if seg.Size%tracePageSize != 0 {
			return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
				Code:        builtinSysDecodePartialPageSegment,
				FileType:    meta.header.FileType,
				SegmentType: seg.Type,
				Offset:      seg.Offset,
				Detail:      fmt.Sprintf("raw segment size %d is not a multiple of the %d-byte RMQ page size", seg.Size, tracePageSize),
			}
		}
		if _, err := f.Seek(seg.Offset, io.SeekStart); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		data := make([]byte, seg.Size)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		for pageOff := 0; pageOff+tracePageSize <= len(data); pageOff += tracePageSize {
			page := data[pageOff : pageOff+tracePageSize]
			ph, ok := parsePageHeader(page)
			if !ok {
				continue
			}
			maxBodyLen := len(page) - pageHeaderSize
			if ph.Length > uint64(maxBodyLen) {
				return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
					Code:        builtinSysDecodePageLength,
					FileType:    meta.header.FileType,
					SegmentType: seg.Type,
					Offset:      seg.Offset + int64(pageOff+8),
					Detail:      fmt.Sprintf("RMQ page declares %d data byte(s), maximum is %d", ph.Length, maxBodyLen),
				}
			}
			body := page[pageHeaderSize : pageHeaderSize+int(ph.Length)]
			for off := 0; off < len(body); {
				if len(body)-off < eventHeaderSize {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeTruncatedEventHeader,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ page ended with %d byte(s), fewer than the %d-byte event header", len(body)-off, eventHeaderSize),
					}
				}
				eh, ok := parseEventHeader(body[off:])
				if !ok {
					// Harmony's reference parser treats a zero-sized record as an
					// explicit end-of-page sentinel. Stop at it; never inspect the
					// physical bytes that follow the logical terminator.
					break
				}
				contentStart := off + eventHeaderSize
				contentEnd := contentStart + int(eh.Size)
				next := contentStart + eh.AlignedSize
				if contentEnd > len(body) || next > len(body) {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeEventBounds,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ event size=%d aligned_size=%d exceeds the %d-byte logical page body", eh.Size, eh.AlignedSize, len(body)),
					}
				}
				content := body[contentStart:contentEnd]
				if len(content) < 2 {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeEventBounds,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+contentStart),
						Detail:      "RMQ event content is too short to contain an event id",
					}
				}
				eventID := int(binary.LittleEndian.Uint16(content[:2]))
				offsetNS := uint64(eh.TimestampOffsetNS)
				if ph.TimestampNS > ^uint64(0)-offsetNS {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeTimestampOverflow,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ timestamp base=%d plus offset=%d overflows uint64 nanoseconds", ph.TimestampNS, offsetNS),
					}
				}
				ts := ph.TimestampNS + offsetNS
				format, ok := meta.formats[eventID]
				if !ok {
					if meta.formatPoisoned[eventID] {
						pairBarrier.poisonFormatFamilies(meta.formatPoisonFamilies[eventID])
						meta.formatConflictRows++
						off = next
						continue
					}
					missing++
					off = next
					continue
				}
				line, admission, reason, envelopeOK, pairAudit := renderEventLineDecisionWithPairAudit(rc, ts, ph.CPU, format, content)
				pairBarrier.observe(pairAudit)
				if !envelopeOK || line == "" {
					// The format was known, but the raw common-field envelope was
					// not. Keep the record coverage-only: emitting even a header-only
					// row here would turn missing/mistyped PID bytes into idle PID 0.
					suppressed++
					off = next
					continue
				}
				if admission == bodyRejected {
					if reason == "" {
						reason = "unspecified"
					}
					meta.bodyRejectedRows++
					meta.bodyRejectReasons[format.Name+"_"+reason]++
					off = next
					continue
				}
				if admission == bodyUnsupported {
					line = renderEventHeaderLine(rc, ts, ph.CPU, format, content)
					if line == "" {
						suppressed++
						off = next
						continue
					}
					unknown++
				}
				if len(rows) == 0 || ts < first {
					first = ts
				}
				if ts > last {
					last = ts
				}
				row := renderedRow{tsNS: ts, seq: seq, line: line}
				if pairAudit.EndpointAdmitted {
					row.pairKind = pairAudit.Kind
					pairBarrier.addPublishedRow(seq, pairAudit)
				}
				rows = append(rows, row)
				seq++
				off = next
			}
		}
	}
	rows = pairBarrier.filter(rows)
	meta.pairQuarantinedRows = pairBarrier.poisonedRows
	meta.pairPoisonedLanes = pairBarrier.poisonedLaneCount()
	meta.pairPoisonedFamilies = pairBarrier.poisonedFamilyCount()
	meta.pairBarrierBudget = pairBarrier.budgetFailed
	first, last = 0, 0
	for index, row := range rows {
		if index == 0 || row.tsNS < first {
			first = row.tsNS
		}
		if index == 0 || row.tsNS > last {
			last = row.tsNS
		}
	}
	return rows, missing, unknown, suppressed, first, last, nil
}

func writeRows(w io.Writer, rows []renderedRow) error {
	bw := bufio.NewWriterSize(w, 256*1024)
	if err := writeSystraceHeader(bw); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := bw.WriteString(row.line); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeSystraceHeader(w io.Writer) error {
	_, err := io.WriteString(w, systraceHeader)
	return err
}
