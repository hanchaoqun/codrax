package hitraceconv

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	hiperfProtoMagic            = "HIPERF_PB_"
	hiperfProtoVersion   uint16 = 1
	hiperfAdapterVersion        = converterVersion + "+hiperf-proto"
)

type hiperfProtoFile struct {
	ID            uint32
	Path          string
	FunctionNames []string
}

type hiperfProtoThread struct {
	TID  uint32
	PID  uint32
	Name string
}

type hiperfProtoFrame struct {
	SymbolsVaddr   uint64
	SymbolsFileID  uint32
	SymbolsFileSet bool
	FunctionNameID int32
	FunctionSet    bool
	LoadedVaddr    uint64
}

type hiperfProtoSample struct {
	TimeNS       uint64
	TID          uint32
	Frames       []hiperfProtoFrame
	EventCount   uint64
	ConfigNameID uint32
	ConfigSet    bool
}

type hiperfProtoData struct {
	Files       map[uint32]hiperfProtoFile
	Threads     map[uint32]hiperfProtoThread
	ConfigNames []string
	Samples     []hiperfProtoSample
}

func maybeConvertHiperfPerfData(ctx context.Context, opts Options, perfPath, perfTracePath string) (Artifact, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.DisablePerfAdapter {
		return Artifact{}, "HIPERF_DATA perf.data extracted; perftrace generation disabled, so .perftrace was not generated", nil
	}
	if rawPerfParserRequired(opts) {
		return maybeConvertRawPerfData(ctx, opts, perfPath, perfTracePath)
	}
	tool, source := resolveHiperfTool(opts)
	if tool == "" {
		return maybeRawPerfFallback(ctx, opts, perfPath, perfTracePath, "HIPERF_DATA perf.data extracted; no official hiperf_host/hiperf adapter was configured or found")
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", err
	}
	protoFile, err := os.CreateTemp(filepath.Dir(perfTracePath), "."+filepath.Base(perfTracePath)+".*.proto")
	if err != nil {
		return Artifact{}, "", err
	}
	protoPath := protoFile.Name()
	if err := protoFile.Close(); err != nil {
		_ = os.Remove(protoPath)
		return Artifact{}, "", err
	}
	_ = os.Remove(protoPath)
	defer os.Remove(protoPath)

	args := []string{"report", "--proto", "-i", perfPath, "-o", protoPath}
	if len(opts.HiperfSymbolDirs) > 0 {
		args = append(args, "--symbol-dir", strings.Join(opts.HiperfSymbolDirs, ","))
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	output, runErr := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Artifact{}, "", ctxErr
	}
	if runErr != nil {
		return maybeRawPerfFallback(ctx, opts, perfPath, perfTracePath, fmt.Sprintf("official hiperf adapter %q failed (%s)%s", tool, runErr, boundedCommandOutput(output)))
	}
	if err := ConvertHiperfProtoFileToPerfTrace(ctx, protoPath, perfTracePath); err != nil {
		_ = os.Remove(perfTracePath)
		return maybeRawPerfFallback(ctx, opts, perfPath, perfTracePath, fmt.Sprintf("official hiperf adapter %q produced unreadable protobuf (%v)", tool, err))
	}
	info, err := os.Stat(perfTracePath)
	if err != nil {
		return Artifact{}, "", err
	}
	return Artifact{
		Type:      ArtifactPerfTrace,
		Path:      perfTracePath,
		Bytes:     info.Size(),
		Converter: hiperfAdapterVersion,
		Caveats: []string{
			fmt.Sprintf("generated from perf.data through %s; sample CPU is unavailable in OpenHarmony report_sample.proto and is emitted as cpu=-1", source),
		},
	}, "", nil
}

func maybeRawPerfFallback(ctx context.Context, opts Options, perfPath, perfTracePath, prior string) (Artifact, string, error) {
	if !rawPerfParserAllowed(opts) {
		if prior != "" {
			return Artifact{}, prior + "; raw perf.data fallback disabled by perf parser mode, so .perftrace was not generated", nil
		}
		return Artifact{}, "raw perf.data fallback disabled by perf parser mode, so .perftrace was not generated", nil
	}
	artifact, caveat, err := maybeConvertRawPerfData(ctx, opts, perfPath, perfTracePath)
	if err != nil || artifact.Path != "" {
		if prior != "" && artifact.Path != "" {
			artifact.Caveats = append([]string{prior + "; fell back to raw perf.data parser"}, artifact.Caveats...)
		}
		return artifact, caveat, err
	}
	if prior != "" && caveat != "" {
		return Artifact{}, prior + "; " + caveat, nil
	}
	return Artifact{}, firstNonEmpty(caveat, prior), nil
}

func resolveHiperfTool(opts Options) (string, string) {
	if path := strings.TrimSpace(opts.HiperfPath); path != "" {
		return path, "configured hiperf tool"
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_HIPERF_HOST")); path != "" {
		return path, "CODRAX_HIPERF_HOST"
	}
	for _, name := range []string{"hiperf_host", "hiperf"} {
		if path, err := exec.LookPath(name); err == nil && strings.TrimSpace(path) != "" {
			return path, name + " on PATH"
		}
	}
	return "", ""
}

func boundedCommandOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return ""
	}
	const max = 400
	if len(text) > max {
		text = text[:max] + "..."
	}
	return ": " + strings.ReplaceAll(text, "\n", " | ")
}

func ConvertHiperfProtoFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := readHiperfProtoFile(ctx, inputPath)
	if err != nil {
		return err
	}
	if len(data.Samples) == 0 {
		return fmt.Errorf("hiperf protobuf contains no sample records")
	}
	if err := ensureOutputDoesNotExist(outputPath); err != nil {
		return err
	}
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	writeErr := writeHiperfPerfTrace(ctx, w, data)
	flushErr := w.Flush()
	closeErr := out.Close()
	if writeErr != nil {
		_ = os.Remove(outputPath)
		return writeErr
	}
	if flushErr != nil {
		_ = os.Remove(outputPath)
		return flushErr
	}
	if closeErr != nil {
		_ = os.Remove(outputPath)
		return closeErr
	}
	return nil
}

func readHiperfProtoFile(ctx context.Context, path string) (hiperfProtoData, error) {
	f, err := os.Open(path)
	if err != nil {
		return hiperfProtoData{}, err
	}
	defer f.Close()
	var magic [len(hiperfProtoMagic)]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf magic: %w", err)
	}
	if string(magic[:]) != hiperfProtoMagic {
		return hiperfProtoData{}, fmt.Errorf("unsupported hiperf protobuf magic %q", string(magic[:]))
	}
	var versionBuf [2]byte
	if _, err := io.ReadFull(f, versionBuf[:]); err != nil {
		return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf version: %w", err)
	}
	if version := binary.LittleEndian.Uint16(versionBuf[:]); version != hiperfProtoVersion {
		return hiperfProtoData{}, fmt.Errorf("unsupported hiperf protobuf version %d", version)
	}
	data := hiperfProtoData{
		Files:   map[uint32]hiperfProtoFile{},
		Threads: map[uint32]hiperfProtoThread{},
	}
	var sizeBuf [4]byte
	for {
		if err := ctx.Err(); err != nil {
			return hiperfProtoData{}, err
		}
		if _, err := io.ReadFull(f, sizeBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return hiperfProtoData{}, fmt.Errorf("truncated hiperf protobuf record size: %w", err)
			}
			return hiperfProtoData{}, err
		}
		size := binary.LittleEndian.Uint32(sizeBuf[:])
		if size == 0 {
			break
		}
		record := make([]byte, size)
		if _, err := io.ReadFull(f, record); err != nil {
			return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf record: %w", err)
		}
		if err := decodeHiperfRecord(record, &data); err != nil {
			return hiperfProtoData{}, err
		}
	}
	return data, nil
}

func decodeHiperfRecord(record []byte, data *hiperfProtoData) error {
	return walkProtoFields(record, func(field int, wire int, raw []byte, v uint64) error {
		if wire != 2 {
			return nil
		}
		switch field {
		case 1:
			sample, err := decodeHiperfSample(raw)
			if err != nil {
				return err
			}
			data.Samples = append(data.Samples, sample)
		case 3:
			file, err := decodeHiperfFile(raw)
			if err != nil {
				return err
			}
			data.Files[file.ID] = file
		case 4:
			thread, err := decodeHiperfThread(raw)
			if err != nil {
				return err
			}
			data.Threads[thread.TID] = thread
		case 5:
			info, err := decodeHiperfInfo(raw)
			if err != nil {
				return err
			}
			if len(info) > 0 {
				data.ConfigNames = info
			}
		}
		_ = v
		return nil
	})
}

func decodeHiperfSample(raw []byte) (hiperfProtoSample, error) {
	var sample hiperfProtoSample
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				sample.TimeNS = v
			}
		case 2:
			if wire == 0 {
				sample.TID = uint32(v)
			}
		case 3:
			if wire == 2 {
				frame, err := decodeHiperfFrame(data)
				if err != nil {
					return err
				}
				sample.Frames = append(sample.Frames, frame)
			}
		case 4:
			if wire == 0 {
				sample.EventCount = v
			}
		case 5:
			if wire == 0 {
				sample.ConfigNameID = uint32(v)
				sample.ConfigSet = true
			}
		}
		return nil
	})
	return sample, err
}

func decodeHiperfFrame(raw []byte) (hiperfProtoFrame, error) {
	var frame hiperfProtoFrame
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		if wire != 0 {
			return nil
		}
		switch field {
		case 1:
			frame.SymbolsVaddr = v
		case 2:
			frame.SymbolsFileID = uint32(v)
			frame.SymbolsFileSet = true
		case 3:
			frame.FunctionNameID = int32(v)
			frame.FunctionSet = true
		case 4:
			frame.LoadedVaddr = v
		}
		_ = data
		return nil
	})
	return frame, err
}

func decodeHiperfFile(raw []byte) (hiperfProtoFile, error) {
	var file hiperfProtoFile
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				file.ID = uint32(v)
			}
		case 2:
			if wire == 2 {
				file.Path = string(data)
			}
		case 3:
			if wire == 2 {
				file.FunctionNames = append(file.FunctionNames, string(data))
			}
		}
		return nil
	})
	return file, err
}

func decodeHiperfThread(raw []byte) (hiperfProtoThread, error) {
	var thread hiperfProtoThread
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				thread.TID = uint32(v)
			}
		case 2:
			if wire == 0 {
				thread.PID = uint32(v)
			}
		case 3:
			if wire == 2 {
				thread.Name = string(data)
			}
		}
		return nil
	})
	return thread, err
}

func decodeHiperfInfo(raw []byte) ([]string, error) {
	var configNames []string
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		if field == 1 && wire == 2 {
			configNames = append(configNames, string(data))
		}
		_ = v
		return nil
	})
	return configNames, err
}

func walkProtoFields(data []byte, fn func(field int, wire int, raw []byte, v uint64) error) error {
	for len(data) > 0 {
		key, n, ok := consumeProtoVarint(data)
		if !ok {
			return fmt.Errorf("malformed protobuf field key")
		}
		data = data[n:]
		field := int(key >> 3)
		wire := int(key & 0x7)
		switch wire {
		case 0:
			v, n, ok := consumeProtoVarint(data)
			if !ok {
				return fmt.Errorf("malformed protobuf varint field %d", field)
			}
			if err := fn(field, wire, nil, v); err != nil {
				return err
			}
			data = data[n:]
		case 1:
			if len(data) < 8 {
				return fmt.Errorf("truncated protobuf fixed64 field %d", field)
			}
			if err := fn(field, wire, data[:8], binary.LittleEndian.Uint64(data[:8])); err != nil {
				return err
			}
			data = data[8:]
		case 2:
			l, n, ok := consumeProtoVarint(data)
			if !ok || l > uint64(len(data[n:])) {
				return fmt.Errorf("truncated protobuf bytes field %d", field)
			}
			raw := data[n : n+int(l)]
			if err := fn(field, wire, raw, 0); err != nil {
				return err
			}
			data = data[n+int(l):]
		case 5:
			if len(data) < 4 {
				return fmt.Errorf("truncated protobuf fixed32 field %d", field)
			}
			if err := fn(field, wire, data[:4], uint64(binary.LittleEndian.Uint32(data[:4]))); err != nil {
				return err
			}
			data = data[4:]
		default:
			return fmt.Errorf("unsupported protobuf wire type %d for field %d", wire, field)
		}
	}
	return nil
}

func consumeProtoVarint(data []byte) (uint64, int, bool) {
	var v uint64
	for i, b := range data {
		if i == 10 {
			return 0, 0, false
		}
		v |= uint64(b&0x7f) << (uint(i) * 7)
		if b < 0x80 {
			return v, i + 1, true
		}
	}
	return 0, 0, false
}

func writeHiperfPerfTrace(ctx context.Context, w io.Writer, data hiperfProtoData) error {
	samples := append([]hiperfProtoSample(nil), data.Samples...)
	sort.SliceStable(samples, func(i, j int) bool {
		if samples[i].TimeNS == samples[j].TimeNS {
			return samples[i].TID < samples[j].TID
		}
		return samples[i].TimeNS < samples[j].TimeNS
	})
	if _, err := io.WriteString(w, systraceHeader); err != nil {
		return err
	}
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return err
		}
		thread := data.Threads[sample.TID]
		tid := int(sample.TID)
		pid := int(thread.PID)
		if pid <= 0 {
			pid = tid
		}
		comm := sanitizePerfTraceComm(firstNonEmpty(thread.Name, fmt.Sprintf("tid%d", tid)))
		leaf, callchain := resolveHiperfSampleFrames(sample, data.Files)
		event := "unknown"
		if sample.ConfigSet && int(sample.ConfigNameID) < len(data.ConfigNames) {
			event = data.ConfigNames[sample.ConfigNameID]
		}
		period := sample.EventCount
		if period == 0 {
			period = 1
		}
		ts := float64(sample.TimeNS) / 1e9
		if _, err := fmt.Fprintf(w, "%16s-%-6d (%5d) [%03d] .... %12.6f: perf_sample: cpu=-1 pid=%d tid=%d thread_comm=%s period=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=hiperf_proto clock=monotonic_raw\n",
			comm, tid, pid, 0, ts, pid, tid, quoteTraceValue(firstNonEmpty(thread.Name, comm)), period, quoteTraceValue(event), quoteTraceValue(leaf.symbol), quoteTraceValue(leaf.dso), quoteTraceValue(leaf.ip), quoteTraceValue(callchain)); err != nil {
			return err
		}
	}
	return nil
}

type resolvedHiperfFrame struct {
	symbol string
	dso    string
	ip     string
}

func resolveHiperfSampleFrames(sample hiperfProtoSample, files map[uint32]hiperfProtoFile) (resolvedHiperfFrame, string) {
	if len(sample.Frames) == 0 {
		return resolvedHiperfFrame{symbol: "unknown", dso: "unknown"}, "unknown"
	}
	resolved := make([]resolvedHiperfFrame, 0, len(sample.Frames))
	for _, frame := range sample.Frames {
		resolved = append(resolved, resolveHiperfFrame(frame, files))
	}
	leaf := resolved[0]
	chainParts := make([]string, 0, len(resolved))
	for i := len(resolved) - 1; i >= 0; i-- {
		part := resolved[i].symbol
		if resolved[i].dso != "" && resolved[i].dso != "unknown" {
			part += "@" + resolved[i].dso
		}
		chainParts = append(chainParts, part)
	}
	return leaf, strings.Join(chainParts, ";")
}

func resolveHiperfFrame(frame hiperfProtoFrame, files map[uint32]hiperfProtoFile) resolvedHiperfFrame {
	var dso string
	var symbol string
	if frame.SymbolsFileSet {
		if file, ok := files[frame.SymbolsFileID]; ok {
			dso = file.Path
			if frame.FunctionSet && frame.FunctionNameID >= 0 && int(frame.FunctionNameID) < len(file.FunctionNames) {
				symbol = file.FunctionNames[frame.FunctionNameID]
			}
		}
	}
	ip := ""
	if frame.SymbolsVaddr != 0 {
		ip = "0x" + strconv.FormatUint(frame.SymbolsVaddr, 16)
	}
	if symbol == "" {
		symbol = firstNonEmpty(ip, "unknown")
	}
	if dso == "" {
		dso = "unknown"
	}
	return resolvedHiperfFrame{symbol: symbol, dso: dso, ip: ip}
}

func sanitizePerfTraceComm(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "hiperf"
	}
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsSpace(r) || r == '|' {
			b.WriteByte('_')
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "hiperf"
	}
	return out
}

func quoteTraceValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return `""`
	}
	return strconv.Quote(raw)
}
