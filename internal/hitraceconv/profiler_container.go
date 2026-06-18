package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	profilerDataTypeProtobuf = uint32(0)
	profilerSessionJSONTag   = "SessionJSON-"
)

type profilerTraceHeader struct {
	Offset        int64
	Length        int64
	Version       uint32
	Segments      uint32
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type profilerPluginData struct {
	Name           string
	Status         uint32
	Data           []byte
	ClockID        uint64
	TvSec          uint64
	TvNsec         uint64
	Version        string
	SampleInterval uint32
}

type profilerContainerExtraction struct {
	Detected           bool
	Kind               string
	Rows               []renderedRow
	Messages           int
	PluginMessages     map[string]int
	StructuredFtrace   int
	TextPluginMessages int
	StandaloneDetected bool
	Caveats            []string
}

func tryConvertProfilerContainer(ctx context.Context, opts Options, inputSize int64, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision) (Result, bool, error) {
	extracted, err := extractProfilerContainerSystraceRows(ctx, opts.InputPath, inputSize)
	if err != nil {
		return Result{}, false, err
	}
	if !extracted.Detected {
		return Result{}, false, nil
	}
	result := Result{
		InputPath:          opts.InputPath,
		InputBytes:         inputSize,
		Artifacts:          append([]Artifact(nil), standaloneArtifacts...),
		ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
		Caveats:            append([]string(nil), extracted.Caveats...),
		MissingFormatCount: 0,
		UnknownEventCount:  extracted.StructuredFtrace,
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	if len(extracted.Rows) > 0 {
		sort.SliceStable(extracted.Rows, func(i, j int) bool {
			if extracted.Rows[i].tsNS == extracted.Rows[j].tsNS {
				return extracted.Rows[i].seq < extracted.Rows[j].seq
			}
			return extracted.Rows[i].tsNS < extracted.Rows[j].tsNS
		})
		out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return Result{}, true, err
		}
		writeErr := writeRows(out, extracted.Rows)
		closeErr := out.Close()
		if writeErr != nil {
			_ = os.Remove(output)
			return Result{}, true, writeErr
		}
		if closeErr != nil {
			_ = os.Remove(output)
			return Result{}, true, closeErr
		}
		info, err := os.Stat(output)
		if err != nil {
			return Result{}, true, err
		}
		result.OutputPath = output
		result.OutputBytes = info.Size()
		result.EventsWritten = len(extracted.Rows)
		result.FirstTimestampSec = float64(extracted.Rows[0].tsNS) / 1e9
		result.LastTimestampSec = float64(extracted.Rows[len(extracted.Rows)-1].tsNS) / 1e9
		result.Artifacts = append([]Artifact{{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     info.Size(),
			Converter: converterVersion + "+openharmony-profiler",
			Caveats:   []string{"generated from OpenHarmony profiler/session text trace payloads"},
		}}, result.Artifacts...)
	} else if len(result.Artifacts) == 0 {
		result.Caveats = append(result.Caveats, "OpenHarmony profiler/session container was detected, but no renderable systrace text rows or sidecar artifacts were found")
	}
	if bundleArtifact, err := writeTraceBundle(opts.InputPath, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions); err != nil {
		return Result{}, true, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	return result, true, nil
}

func extractProfilerContainerSystraceRows(ctx context.Context, path string, inputSize int64) (profilerContainerExtraction, error) {
	header, ok, err := readProfilerTraceHeaderAtPath(path, 0, inputSize)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if ok && header.DataType == profilerDataTypeProtobuf {
		return extractProfilerTraceFile(ctx, path, inputSize, header)
	}
	session, err := extractProfilerSessionPackage(ctx, path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if session.Detected {
		return session, nil
	}
	return profilerContainerExtraction{}, nil
}

func extractProfilerTraceFile(ctx context.Context, path string, inputSize int64, header profilerTraceHeader) (profilerContainerExtraction, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	limit := header.Length
	if limit <= profilerTraceHeaderSize || limit > inputSize {
		limit = inputSize
	}
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_trace_file",
		PluginMessages: map[string]int{},
		Caveats: []string{
			fmt.Sprintf("OpenHarmony profiler TraceFileHeader detected: data_type=%d version=0x%x segments=%d length=%d", header.DataType, header.Version, header.Segments, header.Length),
		},
	}
	off := int64(profilerTraceHeaderSize)
	seq := 0
	for off+4 <= limit {
		if err := ctx.Err(); err != nil {
			return profilerContainerExtraction{}, err
		}
		var lenBuf [4]byte
		if _, err := f.ReadAt(lenBuf[:], off); err != nil {
			if errorsIsEOF(err) {
				break
			}
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message length at %d: %w", off, err)
		}
		n := int64(binary.LittleEndian.Uint32(lenBuf[:]))
		if n <= 0 || off+4+n > limit {
			break
		}
		msg := make([]byte, n)
		if _, err := f.ReadAt(msg, off+4); err != nil {
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message at %d: %w", off+4, err)
		}
		out.Messages++
		if plugin, ok := parseProfilerPluginData(msg); ok {
			name := firstNonEmpty(plugin.Name, "unknown-plugin")
			out.PluginMessages[name]++
			rows := extractSystraceRowsFromBytes(plugin.Data, &seq)
			if len(rows) > 0 {
				out.Rows = append(out.Rows, rows...)
				out.TextPluginMessages++
			} else if strings.EqualFold(name, "ftrace-plugin") {
				out.StructuredFtrace++
			}
		}
		off += 4 + n
	}
	if out.Messages == 0 {
		out.Caveats = append(out.Caveats, "official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable")
	}
	if out.StructuredFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("detected %d ftrace-plugin protobuf message(s); Codrax currently renders embedded text trace payloads and preserves sidecars, but does not inline the full generated OpenHarmony TracePluginResult formatter matrix", out.StructuredFtrace))
	}
	if len(out.Rows) > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from %d profiler plugin message(s)", len(out.Rows), out.TextPluginMessages))
	}
	return out, nil
}

func extractProfilerSessionPackage(ctx context.Context, path string) (profilerContainerExtraction, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	probe := make([]byte, 64*1024)
	n, err := f.Read(probe)
	if err != nil && err != io.EOF {
		return profilerContainerExtraction{}, err
	}
	if !bytes.Contains(probe[:n], []byte(profilerSessionJSONTag)) {
		return profilerContainerExtraction{}, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return profilerContainerExtraction{}, err
	}
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_session_package",
		PluginMessages: map[string]int{},
		Caveats: []string{
			"OpenHarmony profiler session package marker SessionJSON- detected; using section/text extraction instead of legacy binary hitrace segment parsing",
		},
	}
	seq := 0
	reader := bufio.NewReaderSize(f, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return profilerContainerExtraction{}, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineRows := extractSystraceRowsFromBytes(line, &seq)
			out.Rows = append(out.Rows, lineRows...)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return profilerContainerExtraction{}, readErr
		}
	}
	if len(out.Rows) > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from profiler session package payload", len(out.Rows)))
	} else {
		out.Caveats = append(out.Caveats, "session package did not contain directly renderable systrace text rows; attach extracted sidecars or export ftrace/bytrace text with the official profiler tooling")
	}
	return out, nil
}

func readProfilerTraceHeaderAtPath(path string, off int64, fileSize int64) (profilerTraceHeader, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerTraceHeader{}, false, err
	}
	defer f.Close()
	header, ok := readProfilerTraceHeaderAt(f, off, fileSize)
	return header, ok, nil
}

func readProfilerTraceHeaderAt(r io.ReaderAt, off int64, fileSize int64) (profilerTraceHeader, bool) {
	if off < 0 || off+profilerTraceHeaderSize > fileSize {
		return profilerTraceHeader{}, false
	}
	header := make([]byte, profilerTraceHeaderSize)
	if _, err := r.ReadAt(header, off); err != nil {
		return profilerTraceHeader{}, false
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return profilerTraceHeader{}, false
	}
	length := int64(binary.LittleEndian.Uint64(header[8:16]))
	if length < profilerTraceHeaderSize {
		return profilerTraceHeader{}, false
	}
	return profilerTraceHeader{
		Offset:        off,
		Length:        length,
		Version:       binary.LittleEndian.Uint32(header[16:20]),
		Segments:      binary.LittleEndian.Uint32(header[20:24]),
		DataType:      binary.LittleEndian.Uint32(header[56:60]),
		PluginName:    cString(header[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize]),
		PluginVersion: cString(header[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize]),
	}, true
}

func parseProfilerPluginData(data []byte) (profilerPluginData, bool) {
	var out profilerPluginData
	off := 0
	for off < len(data) {
		field, wire, ok := readProtoKey(data, &off)
		if !ok {
			return out, false
		}
		switch field {
		case 1:
			if wire != 2 {
				return out, false
			}
			out.Name, ok = readProtoString(data, &off)
		case 2:
			var v uint64
			v, ok = readProtoVarint(data, &off)
			out.Status = uint32(v)
		case 3:
			if wire != 2 {
				return out, false
			}
			out.Data, ok = readProtoBytes(data, &off)
		case 4:
			out.ClockID, ok = readProtoVarint(data, &off)
		case 5:
			out.TvSec, ok = readProtoVarint(data, &off)
		case 6:
			out.TvNsec, ok = readProtoVarint(data, &off)
		case 7:
			if wire != 2 {
				return out, false
			}
			out.Version, ok = readProtoString(data, &off)
		case 8:
			var v uint64
			v, ok = readProtoVarint(data, &off)
			out.SampleInterval = uint32(v)
		default:
			ok = skipProtoField(data, &off, wire)
		}
		if !ok {
			return out, false
		}
	}
	return out, out.Name != "" || len(out.Data) > 0
}

func extractSystraceRowsFromBytes(data []byte, seq *int) []renderedRow {
	if len(data) == 0 {
		return nil
	}
	normalized := bytes.ReplaceAll(data, []byte{0}, []byte{'\n'})
	scanner := bufio.NewScanner(bytes.NewReader(normalized))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var rows []renderedRow
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		ts, ok := systraceLineTimestampNS(line)
		if !ok {
			continue
		}
		rows = append(rows, renderedRow{tsNS: ts, seq: *seq, line: line})
		*seq++
	}
	return rows
}

func systraceLineTimestampNS(line string) (uint64, bool) {
	for searchFrom := 0; searchFrom < len(line); {
		colon := strings.Index(line[searchFrom:], ":")
		if colon < 0 {
			return 0, false
		}
		colon += searchFrom
		if colon > 0 {
			prefix := line[:colon]
			fields := strings.Fields(prefix)
			if len(fields) > 0 {
				if ts, err := strconv.ParseFloat(fields[len(fields)-1], 64); err == nil && ts >= 0 {
					rest := strings.TrimSpace(line[colon+1:])
					nextColon := strings.Index(rest, ":")
					if nextColon > 0 && isTraceEventName(rest[:nextColon]) {
						return uint64(ts * 1e9), true
					}
				}
			}
		}
		searchFrom = colon + 1
	}
	return 0, false
}

func isTraceEventName(event string) bool {
	if event == "" || len(event) > 128 {
		return false
	}
	for _, r := range event {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func readProtoKey(data []byte, off *int) (field int, wire int, ok bool) {
	key, ok := readProtoVarint(data, off)
	if !ok || key == 0 {
		return 0, 0, false
	}
	return int(key >> 3), int(key & 0x7), true
}

func readProtoVarint(data []byte, off *int) (uint64, bool) {
	var out uint64
	for shift := uint(0); shift < 64 && *off < len(data); shift += 7 {
		b := data[*off]
		*off++
		out |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return out, true
		}
	}
	return 0, false
}

func readProtoBytes(data []byte, off *int) ([]byte, bool) {
	n, ok := readProtoVarint(data, off)
	if !ok || n > uint64(len(data)-*off) {
		return nil, false
	}
	start := *off
	*off += int(n)
	return data[start:*off], true
}

func readProtoString(data []byte, off *int) (string, bool) {
	b, ok := readProtoBytes(data, off)
	if !ok {
		return "", false
	}
	return string(b), true
}

func skipProtoField(data []byte, off *int, wire int) bool {
	switch wire {
	case 0:
		_, ok := readProtoVarint(data, off)
		return ok
	case 1:
		if *off+8 > len(data) {
			return false
		}
		*off += 8
		return true
	case 2:
		n, ok := readProtoVarint(data, off)
		if !ok || n > uint64(len(data)-*off) {
			return false
		}
		*off += int(n)
		return true
	case 5:
		if *off+4 > len(data) {
			return false
		}
		*off += 4
		return true
	default:
		return false
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
