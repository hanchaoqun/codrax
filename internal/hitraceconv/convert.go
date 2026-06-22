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

type traceMetadata struct {
	header   fileHeader
	formats  map[int]eventFormat
	cmdlines map[int]string
	tgids    map[int]int
	segments []segmentMeta
}

type renderedRow struct {
	tsNS uint64
	seq  int
	line string
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
	traceStreamerExport, err := maybeRunTraceStreamerAuto(ctx, opts, input, output)
	if err != nil {
		return Result{}, err
	}
	var initialArtifacts []Artifact
	var initialCaveats []string
	var initialTraceDecisions []TraceProviderDecision
	if traceStreamerExport.Decision.ProviderName != "" {
		initialTraceDecisions = append(initialTraceDecisions, traceStreamerExport.Decision)
	}
	if traceStreamerExport.Artifact.Path != "" && (opts.KeepTraceDB || strings.TrimSpace(opts.TraceDBOutputPath) != "") {
		initialArtifacts = append(initialArtifacts, traceStreamerExport.Artifact)
		initialCaveats = append(initialCaveats, traceStreamerExport.Caveats...)
	} else if traceStreamerExport.Cleanup != nil {
		defer traceStreamerExport.Cleanup()
	}
	standaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifacts(ctx, opts, info.Size(), output)
	if err != nil {
		return Result{}, err
	}
	standaloneArtifacts = append(initialArtifacts, standaloneArtifacts...)
	standaloneCaveats = append(initialCaveats, standaloneCaveats...)
	if result, ok, err := tryConvertProfilerContainer(ctx, opts, info.Size(), output, standaloneArtifacts, standaloneCaveats, standaloneDecisions, initialTraceDecisions); ok || err != nil {
		return result, err
	}
	meta, err := scanMetadata(ctx, input, info.Size())
	if err != nil {
		if len(standaloneArtifacts) > 0 {
			result := Result{
				InputPath:  input,
				InputBytes: info.Size(),
				Artifacts:  append([]Artifact(nil), standaloneArtifacts...),
				ProviderDecisions: append([]PerfProviderDecision(nil),
					standaloneDecisions...),
				TraceDecisions: append(initialTraceDecisions,
					traceProviderFailure(
						newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys), opts, input, output),
						"unsupported_sys_binary_container",
						fmt.Sprintf("built-in sys binary parser could not render the trace body: %v", err),
					),
				),
				Caveats: append(standaloneCaveats,
					fmt.Sprintf("systrace output was not produced because the input did not match the built-in sys binary trace container: %v", err)),
			}
			if bundleArtifact, bundleErr := writeTraceBundle(input, "", result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions); bundleErr != nil {
				return Result{}, bundleErr
			} else if bundleArtifact.Path != "" {
				result.BundlePath = bundleArtifact.Path
				result.Artifacts = append(result.Artifacts, bundleArtifact)
			}
			return result, nil
		}
		return Result{}, err
	}
	rows, missing, unknown, first, last, err := renderRows(ctx, input, meta)
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
	}
	result.Artifacts = append(result.Artifacts, standaloneArtifacts...)
	result.ProviderDecisions = append(result.ProviderDecisions, standaloneDecisions...)
	if missing > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw event(s) had no event format and were skipped to keep systrace output compatible with official parsers", missing))
	}
	if unknown > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d event row(s) lacked an official-compatible renderer and were emitted as header-only rows", unknown))
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	if bundleArtifact, err := writeTraceBundle(input, output, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions); err != nil {
		return Result{}, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	return result, nil
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
	meta := &traceMetadata{
		header:   header,
		formats:  map[int]eventFormat{},
		cmdlines: map[int]string{},
		tgids:    map[int]int{},
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
		case typ == segmentEventsFormat || typ == segmentCmdlines || typ == segmentTGIDs:
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
				for id, format := range formats {
					meta.formats[id] = format
				}
			case segmentCmdlines:
				for pid, comm := range parseCmdlines(data) {
					meta.cmdlines[pid] = comm
				}
			case segmentTGIDs:
				for pid, tgid := range parseTGIDs(data) {
					meta.tgids[pid] = tgid
				}
			}
		default:
			if _, err := f.Seek(int64(segSize), io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("skip segment type=%d: %w", typ, err)
			}
		}
	}
	if len(meta.formats) == 0 {
		return nil, fmt.Errorf("no event format segment found; not a supported binary hitrace file")
	}
	return meta, nil
}

func renderRows(ctx context.Context, path string, meta *traceMetadata) ([]renderedRow, int, int, uint64, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	defer f.Close()
	var rows []renderedRow
	missing := 0
	unknown := 0
	var first uint64
	var last uint64
	seq := 0
	rc := renderContext{cmdlines: meta.cmdlines, tgids: meta.tgids}
	for _, seg := range meta.segments {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, 0, err
		}
		if !isRawTraceSegment(seg.Type, meta.header.CPUNum) {
			continue
		}
		if _, err := f.Seek(seg.Offset, io.SeekStart); err != nil {
			return nil, 0, 0, 0, 0, err
		}
		data := make([]byte, seg.Size)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, 0, 0, 0, 0, err
		}
		for pageOff := 0; pageOff+tracePageSize <= len(data); pageOff += tracePageSize {
			page := data[pageOff : pageOff+tracePageSize]
			ph, ok := parsePageHeader(page)
			if !ok {
				continue
			}
			body := page[pageHeaderSize:]
			for off := 0; off+eventHeaderSize <= len(body); {
				eh, ok := parseEventHeader(body[off:])
				if !ok {
					break
				}
				contentStart := off + eventHeaderSize
				contentEnd := contentStart + int(eh.Size)
				next := contentStart + eh.AlignedSize
				if contentEnd > len(body) || next > len(body) {
					break
				}
				content := body[contentStart:contentEnd]
				if len(content) < 2 {
					break
				}
				eventID := int(binary.LittleEndian.Uint16(content[:2]))
				ts := ph.TimestampNS + uint64(eh.TimestampOffsetNS)
				format, ok := meta.formats[eventID]
				if !ok {
					missing++
					off = next
					continue
				}
				line, known := renderEventLine(rc, ts, ph.CPU, format, content)
				if !known {
					unknown++
					line = renderEventHeaderLine(rc, ts, ph.CPU, format, content)
				}
				if first == 0 || ts < first {
					first = ts
				}
				if ts > last {
					last = ts
				}
				rows = append(rows, renderedRow{tsNS: ts, seq: seq, line: line})
				seq++
				off = next
			}
		}
	}
	return rows, missing, unknown, first, last, nil
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
