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
	"strconv"
	"strings"
)

const (
	simpleperfProtoMagic     = "SIMPLEPERF"
	simpleperfProtoVersion   = uint16(1)
	simpleperfProtoConverter = converterVersion + "+simpleperf-report-proto"
)

type simpleperfProtoFrame struct {
	VaddrInFile   uint64
	FileID        uint32
	FileSet       bool
	SymbolID      int32
	SymbolSet     bool
	ExecutionType uint32
}

type simpleperfProtoSample struct {
	TimeNS       uint64
	ThreadID     int32
	Frames       []simpleperfProtoFrame
	EventCount   uint64
	EventTypeID  uint32
	EventTypeSet bool
}

type simpleperfProtoFile struct {
	ID             uint32
	Path           string
	Symbols        []string
	MangledSymbols []string
}

type simpleperfProtoThread struct {
	TID  uint32
	PID  uint32
	Name string
}

type simpleperfProtoMeta struct {
	EventTypes     []string
	AppPackageName string
	TraceOffCPU    bool
	TraceOffCPUSet bool
}

type simpleperfProtoContextSwitch struct {
	SwitchOn    bool
	SwitchOnSet bool
	TimeNS      uint64
	TID         uint32
}

type simpleperfProtoData struct {
	Files           map[uint32]simpleperfProtoFile
	Threads         map[uint32]simpleperfProtoThread
	Meta            simpleperfProtoMeta
	Samples         []simpleperfProtoSample
	ContextSwitches []simpleperfProtoContextSwitch
}

func maybeConvertSimpleperfProtoWithDecision(ctx context.Context, opts Options, perfPath, perfTracePath string, stage string) (Artifact, string, []PerfProviderDecision, error) {
	decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameSimpleperfProto), opts, perfPath, perfInputSimpleperfReportProto, perfTracePath)
	if err := ConvertSimpleperfProtoFileToPerfTrace(ctx, perfPath, perfTracePath); err != nil {
		caveat := fmt.Sprintf("Android SIMPLEPERF report-sample proto could not be converted (%v)", err)
		decision = perfProviderFailure(decision, "official_proto_unreadable", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	info, err := os.Stat(perfTracePath)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{decision}, err
	}
	artifact := Artifact{
		Type:      ArtifactPerfTrace,
		Path:      perfTracePath,
		Bytes:     info.Size(),
		Converter: simpleperfProtoConverter,
		Perf:      perfCapabilityForSimpleperfReportProto("Android cmd_report_sample.proto"),
		Caveats: []string{
			"generated from Android SIMPLEPERF report-sample protobuf; sample CPU is unavailable in cmd_report_sample.proto and is emitted as cpu=-1",
		},
	}
	decision = perfProviderSuccess(decision, artifact)
	return artifact, "", []PerfProviderDecision{decision}, nil
}

func ConvertSimpleperfProtoFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := readSimpleperfProtoFile(ctx, inputPath)
	if err != nil {
		return err
	}
	if len(data.Samples) == 0 {
		return fmt.Errorf("simpleperf protobuf contains no sample records")
	}
	if err := ensureOutputDoesNotExist(outputPath); err != nil {
		return err
	}
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	writeErr := writeSimpleperfProtoPerfTrace(ctx, w, data)
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

func readSimpleperfProtoFile(ctx context.Context, path string) (simpleperfProtoData, error) {
	f, err := os.Open(path)
	if err != nil {
		return simpleperfProtoData{}, err
	}
	defer f.Close()
	var magic [len(simpleperfProtoMagic)]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf magic: %w", err)
	}
	if string(magic[:]) != simpleperfProtoMagic {
		return simpleperfProtoData{}, fmt.Errorf("unsupported simpleperf protobuf magic %q", string(magic[:]))
	}
	var versionBuf [2]byte
	if _, err := io.ReadFull(f, versionBuf[:]); err != nil {
		return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf version: %w", err)
	}
	if version := binary.LittleEndian.Uint16(versionBuf[:]); version != simpleperfProtoVersion {
		return simpleperfProtoData{}, fmt.Errorf("unsupported simpleperf protobuf version %d", version)
	}
	data := simpleperfProtoData{
		Files:   map[uint32]simpleperfProtoFile{},
		Threads: map[uint32]simpleperfProtoThread{},
	}
	var sizeBuf [4]byte
	for {
		if err := ctx.Err(); err != nil {
			return simpleperfProtoData{}, err
		}
		if _, err := io.ReadFull(f, sizeBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return simpleperfProtoData{}, fmt.Errorf("truncated simpleperf protobuf record size: %w", err)
			}
			return simpleperfProtoData{}, err
		}
		size := binary.LittleEndian.Uint32(sizeBuf[:])
		if size == 0 {
			break
		}
		record := make([]byte, size)
		if _, err := io.ReadFull(f, record); err != nil {
			return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf record: %w", err)
		}
		if err := decodeSimpleperfRecord(record, &data); err != nil {
			return simpleperfProtoData{}, err
		}
	}
	return data, nil
}

func decodeSimpleperfRecord(record []byte, data *simpleperfProtoData) error {
	return walkProtoFields(record, func(field int, wire int, raw []byte, v uint64) error {
		if wire != 2 {
			return nil
		}
		switch field {
		case 1:
			sample, err := decodeSimpleperfSample(raw)
			if err != nil {
				return err
			}
			data.Samples = append(data.Samples, sample)
		case 3:
			file, err := decodeSimpleperfFile(raw)
			if err != nil {
				return err
			}
			data.Files[file.ID] = file
		case 4:
			thread, err := decodeSimpleperfThread(raw)
			if err != nil {
				return err
			}
			data.Threads[thread.TID] = thread
		case 5:
			meta, err := decodeSimpleperfMeta(raw)
			if err != nil {
				return err
			}
			data.Meta = meta
		case 6:
			cs, err := decodeSimpleperfContextSwitch(raw)
			if err != nil {
				return err
			}
			data.ContextSwitches = append(data.ContextSwitches, cs)
		}
		_ = v
		return nil
	})
}

func decodeSimpleperfSample(raw []byte) (simpleperfProtoSample, error) {
	var sample simpleperfProtoSample
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				sample.TimeNS = v
			}
		case 2:
			if wire == 0 {
				sample.ThreadID = int32(uint32(v))
			}
		case 3:
			if wire == 2 {
				frame, err := decodeSimpleperfFrame(data)
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
				sample.EventTypeID = uint32(v)
				sample.EventTypeSet = true
			}
		}
		return nil
	})
	return sample, err
}

func decodeSimpleperfFrame(raw []byte) (simpleperfProtoFrame, error) {
	var frame simpleperfProtoFrame
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		if wire != 0 {
			return nil
		}
		switch field {
		case 1:
			frame.VaddrInFile = v
		case 2:
			frame.FileID = uint32(v)
			frame.FileSet = true
		case 3:
			frame.SymbolID = int32(uint32(v))
			frame.SymbolSet = true
		case 4:
			frame.ExecutionType = uint32(v)
		}
		_ = data
		return nil
	})
	return frame, err
}

func decodeSimpleperfFile(raw []byte) (simpleperfProtoFile, error) {
	var file simpleperfProtoFile
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
				file.Symbols = append(file.Symbols, string(data))
			}
		case 4:
			if wire == 2 {
				file.MangledSymbols = append(file.MangledSymbols, string(data))
			}
		}
		return nil
	})
	return file, err
}

func decodeSimpleperfThread(raw []byte) (simpleperfProtoThread, error) {
	var thread simpleperfProtoThread
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

func decodeSimpleperfMeta(raw []byte) (simpleperfProtoMeta, error) {
	var meta simpleperfProtoMeta
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 2 {
				meta.EventTypes = append(meta.EventTypes, string(data))
			}
		case 2:
			if wire == 2 {
				meta.AppPackageName = string(data)
			}
		case 6:
			if wire == 0 {
				meta.TraceOffCPU = v != 0
				meta.TraceOffCPUSet = true
			}
		}
		return nil
	})
	return meta, err
}

func decodeSimpleperfContextSwitch(raw []byte) (simpleperfProtoContextSwitch, error) {
	var cs simpleperfProtoContextSwitch
	err := walkProtoFields(raw, func(field int, wire int, data []byte, v uint64) error {
		if wire != 0 {
			return nil
		}
		switch field {
		case 1:
			cs.SwitchOn = v != 0
			cs.SwitchOnSet = true
		case 2:
			cs.TimeNS = v
		case 3:
			cs.TID = uint32(v)
		}
		_ = data
		return nil
	})
	return cs, err
}

func writeSimpleperfProtoPerfTrace(ctx context.Context, w io.Writer, data simpleperfProtoData) error {
	samples := append([]simpleperfProtoSample(nil), data.Samples...)
	sort.SliceStable(samples, func(i, j int) bool {
		if samples[i].TimeNS == samples[j].TimeNS {
			return samples[i].ThreadID < samples[j].ThreadID
		}
		return samples[i].TimeNS < samples[j].TimeNS
	})
	contextByThread := simpleperfContextSwitchesByThread(data.ContextSwitches)
	if _, err := io.WriteString(w, systraceHeader); err != nil {
		return err
	}
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return err
		}
		tid := int(sample.ThreadID)
		thread := data.Threads[uint32(sample.ThreadID)]
		pid := int(thread.PID)
		if pid <= 0 {
			pid = tid
		}
		comm := sanitizePerfTraceComm(firstNonEmpty(thread.Name, fmt.Sprintf("tid%d", tid)))
		leaf, callchain := resolveSimpleperfProtoSampleFrames(sample, data.Files)
		event := "unknown"
		if sample.EventTypeSet && int(sample.EventTypeID) < len(data.Meta.EventTypes) {
			event = data.Meta.EventTypes[sample.EventTypeID]
		}
		period := sample.EventCount
		if period == 0 {
			period = 1
		}
		ts := float64(sample.TimeNS) / 1e9
		source := "simpleperf_report_proto"
		symbolizationStatus := perfTraceSymbolizationStatus(leaf.symbol, leaf.dso, source)
		callchainStatus := perfTraceCallchainStatus(callchain, source)
		sampleKind := simpleperfProtoSampleKind(sample, data.Meta, contextByThread[uint32(sample.ThreadID)])
		if _, err := fmt.Fprintf(w, "%16s-%-6d (%5d) [%03d] .... %12.6f: perf_sample: cpu=-1 cpu_known=false pid=%d tid=%d thread_comm=%s period=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=%s sample_kind=%s symbolization_status=%s clock=simpleperf_record clock_confidence=assumed callchain_status=%s\n",
			comm, tid, pid, 0, ts, pid, tid, quoteTraceValue(firstNonEmpty(thread.Name, comm)), period, quoteTraceValue(event), quoteTraceValue(leaf.symbol), quoteTraceValue(leaf.dso), quoteTraceValue(leaf.ip), quoteTraceValue(callchain), source, sampleKind, symbolizationStatus, callchainStatus); err != nil {
			return err
		}
	}
	return nil
}

func simpleperfContextSwitchesByThread(switches []simpleperfProtoContextSwitch) map[uint32][]simpleperfProtoContextSwitch {
	out := map[uint32][]simpleperfProtoContextSwitch{}
	for _, cs := range switches {
		out[cs.TID] = append(out[cs.TID], cs)
	}
	for tid := range out {
		sort.SliceStable(out[tid], func(i, j int) bool {
			return out[tid][i].TimeNS < out[tid][j].TimeNS
		})
	}
	return out
}

func simpleperfProtoSampleKind(sample simpleperfProtoSample, meta simpleperfProtoMeta, switches []simpleperfProtoContextSwitch) string {
	if !meta.TraceOffCPU {
		return "on_cpu"
	}
	idx := sort.Search(len(switches), func(i int) bool {
		return switches[i].TimeNS > sample.TimeNS
	}) - 1
	if idx < 0 {
		return "unknown"
	}
	if !switches[idx].SwitchOnSet {
		return "unknown"
	}
	if switches[idx].SwitchOn {
		return "on_cpu"
	}
	return "off_cpu"
}

type resolvedSimpleperfProtoFrame struct {
	symbol string
	dso    string
	ip     string
}

func resolveSimpleperfProtoSampleFrames(sample simpleperfProtoSample, files map[uint32]simpleperfProtoFile) (resolvedSimpleperfProtoFrame, string) {
	if len(sample.Frames) == 0 {
		return resolvedSimpleperfProtoFrame{symbol: "unknown", dso: "unknown"}, "unknown"
	}
	resolved := make([]resolvedSimpleperfProtoFrame, 0, len(sample.Frames))
	for _, frame := range sample.Frames {
		resolved = append(resolved, resolveSimpleperfProtoFrame(frame, files))
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

func resolveSimpleperfProtoFrame(frame simpleperfProtoFrame, files map[uint32]simpleperfProtoFile) resolvedSimpleperfProtoFrame {
	var dso string
	var symbol string
	if frame.FileSet {
		if file, ok := files[frame.FileID]; ok {
			dso = file.Path
			if frame.SymbolSet && frame.SymbolID >= 0 && int(frame.SymbolID) < len(file.Symbols) {
				symbol = file.Symbols[frame.SymbolID]
			}
			if symbol == "" && frame.SymbolSet && frame.SymbolID >= 0 && int(frame.SymbolID) < len(file.MangledSymbols) {
				symbol = file.MangledSymbols[frame.SymbolID]
			}
		}
	}
	ip := ""
	if frame.VaddrInFile != 0 {
		ip = "0x" + strconv.FormatUint(frame.VaddrInFile, 16)
	}
	if symbol == "" {
		symbol = firstNonEmpty(ip, "unknown")
	}
	if dso == "" {
		dso = "unknown"
	}
	return resolvedSimpleperfProtoFrame{symbol: symbol, dso: dso, ip: ip}
}
