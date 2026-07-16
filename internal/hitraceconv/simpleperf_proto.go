package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracewire"
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

func maybeConvertSimpleperfProtoWithDecision(ctx context.Context, opts Options, perfPath, perfTracePath string, stage string, ledger *conversionFileLedger) (Artifact, string, []PerfProviderDecision, error) {
	decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameSimpleperfProto), opts, perfPath, perfInputSimpleperfReportProto, perfTracePath)
	if err := convertSimpleperfProtoFileToPerfTraceWithLedger(ctx, perfPath, perfTracePath, ledger); err != nil {
		if ownedTraceOutputHardFailure(err) {
			return Artifact{}, "", []PerfProviderDecision{decision}, err
		}
		caveat := fmt.Sprintf("Android SIMPLEPERF report-sample proto could not be converted (%v)", err)
		decision = perfProviderFailure(decision, "official_proto_unreadable", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	artifact, err := newValidatedPerfTraceArtifact(
		ledger, perfTracePath, ownedTracePerfSimpleperfProto, perfInputSimpleperfReportProto,
		"Android cmd_report_sample.proto", []string{
			"generated from Android SIMPLEPERF report-sample protobuf; sample CPU is unavailable in cmd_report_sample.proto and is emitted as cpu=-1",
		},
	)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{decision}, err
	}
	decision, err = perfProviderSuccess(decision, artifact, ledger)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{decision}, err
	}
	return artifact, "", []PerfProviderDecision{decision}, nil
}

func maybeConvertSimpleperfProtoFromInputWithDecision(ctx context.Context, opts Options, input directPerfInputBinding, perfTracePath string, stage string, ledger *conversionFileLedger) (Artifact, string, []PerfProviderDecision, error) {
	if err := input.validate(); err != nil {
		return Artifact{}, "", nil, err
	}
	decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameSimpleperfProto), opts, input.displayPath, perfInputSimpleperfReportProto, perfTracePath)
	if err := convertSimpleperfProtoInputToPerfTraceWithLedger(ctx, input, perfTracePath, ledger); err != nil {
		if directPerfInputBoundaryError(err) || ownedTraceOutputHardFailure(err) {
			return Artifact{}, "", nil, err
		}
		caveat := fmt.Sprintf("Android SIMPLEPERF report-sample proto could not be converted (%v)", err)
		decision = perfProviderFailure(decision, "official_proto_unreadable", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	artifact, err := newValidatedPerfTraceArtifact(
		ledger, perfTracePath, ownedTracePerfSimpleperfProto, perfInputSimpleperfReportProto,
		"Android cmd_report_sample.proto", []string{
			"generated from Android SIMPLEPERF report-sample protobuf; sample CPU is unavailable in cmd_report_sample.proto and is emitted as cpu=-1",
		},
	)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{decision}, err
	}
	decision, err = perfProviderSuccess(decision, artifact, ledger)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{decision}, err
	}
	return artifact, "", []PerfProviderDecision{decision}, nil
}

func ConvertSimpleperfProtoFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	return runConversionInputTransaction(ctx, inputPath, func(authority *conversionInputAuthority, ledger *conversionFileLedger) error {
		input, err := newDirectPerfInputBinding(authority, perfInputSimpleperfReportProto)
		if err != nil {
			return err
		}
		return convertSimpleperfProtoInputToPerfTraceWithLedger(ctx, input, outputPath, ledger)
	})
}

func convertSimpleperfProtoFileToPerfTraceWithLedger(ctx context.Context, inputPath, outputPath string, ledger *conversionFileLedger) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := readSimpleperfProtoFile(ctx, inputPath)
	if err != nil {
		return err
	}
	return writeSimpleperfProtoDataToPerfTraceWithLedger(ctx, data, outputPath, ledger)
}

func convertSimpleperfProtoInputToPerfTraceWithLedger(ctx context.Context, input directPerfInputBinding, outputPath string, ledger *conversionFileLedger) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.validate(); err != nil {
		return err
	}
	if err := completeConversionInputStage(ctx, input.input, conversionInputStageDirectPerfRead, nil); err != nil {
		return err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input.input, conversionInputStageDirectPerfRead, err)
	}()
	data, readErr := readSimpleperfProtoAt(ctx, input.input, input.inputSize)
	if err := completeConversionInputStage(ctx, input.input, conversionInputStageDirectPerfRead, readErr); err != nil {
		return err
	}
	return writeSimpleperfProtoDataToPerfTraceWithLedger(ctx, data, outputPath, ledger)
}

func writeSimpleperfProtoDataToPerfTraceWithLedger(ctx context.Context, data simpleperfProtoData, outputPath string, ledger *conversionFileLedger) error {
	if len(data.Samples) == 0 {
		return fmt.Errorf("simpleperf protobuf contains no sample records")
	}
	_, err := writeValidatedOwnedPerfTraceWithLedger(
		ctx, ownedPerfTraceWriteSpec{Profile: ownedTracePerfSimpleperfProto, ExpectedRows: len(data.Samples)}, outputPath, ledger,
		func(writer io.Writer) error { return writeSimpleperfProtoPerfTrace(ctx, writer, data) },
	)
	return err
}

func readSimpleperfProtoFile(ctx context.Context, path string) (simpleperfProtoData, error) {
	f, err := os.Open(path)
	if err != nil {
		return simpleperfProtoData{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return simpleperfProtoData{}, err
	}
	return readSimpleperfProtoAt(ctx, f, info.Size())
}

func readSimpleperfProtoAt(ctx context.Context, reader io.ReaderAt, size int64) (simpleperfProtoData, error) {
	if reader == nil {
		return simpleperfProtoData{}, fmt.Errorf("simpleperf protobuf reader is nil")
	}
	if size < 0 {
		return simpleperfProtoData{}, fmt.Errorf("simpleperf protobuf size is negative: %d", size)
	}
	return readSimpleperfProto(ctx, io.NewSectionReader(reader, 0, size), size)
}

func readSimpleperfProto(ctx context.Context, reader io.Reader, size int64) (simpleperfProtoData, error) {
	if size < 0 {
		return simpleperfProtoData{}, fmt.Errorf("simpleperf protobuf size is negative: %d", size)
	}
	var magic [len(simpleperfProtoMagic)]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf magic: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return simpleperfProtoData{}, err
	}
	if string(magic[:]) != simpleperfProtoMagic {
		return simpleperfProtoData{}, fmt.Errorf("unsupported simpleperf protobuf magic %q", string(magic[:]))
	}
	var versionBuf [2]byte
	if _, err := io.ReadFull(reader, versionBuf[:]); err != nil {
		return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf version: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return simpleperfProtoData{}, err
	}
	if version := binary.LittleEndian.Uint16(versionBuf[:]); version != simpleperfProtoVersion {
		return simpleperfProtoData{}, fmt.Errorf("unsupported simpleperf protobuf version %d", version)
	}
	remaining := size - int64(len(magic)+len(versionBuf))
	data := simpleperfProtoData{
		Files:   map[uint32]simpleperfProtoFile{},
		Threads: map[uint32]simpleperfProtoThread{},
	}
	var sizeBuf [4]byte
	for {
		if err := ctx.Err(); err != nil {
			return simpleperfProtoData{}, err
		}
		if _, err := io.ReadFull(reader, sizeBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return simpleperfProtoData{}, fmt.Errorf("truncated simpleperf protobuf record size: %w", err)
			}
			return simpleperfProtoData{}, err
		}
		remaining -= int64(len(sizeBuf))
		size := binary.LittleEndian.Uint32(sizeBuf[:])
		if size == 0 {
			break
		}
		if err := ctx.Err(); err != nil {
			return simpleperfProtoData{}, err
		}
		if remaining < 0 || int64(size) > remaining {
			return simpleperfProtoData{}, fmt.Errorf("simpleperf protobuf record size %d exceeds fixed input remainder %d", size, remaining)
		}
		if uint64(size) > uint64(^uint(0)>>1) {
			return simpleperfProtoData{}, fmt.Errorf("simpleperf protobuf record size %d exceeds host allocation limit", size)
		}
		record := make([]byte, size)
		if _, err := io.ReadFull(reader, record); err != nil {
			return simpleperfProtoData{}, fmt.Errorf("read simpleperf protobuf record: %w", err)
		}
		remaining -= int64(size)
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
		period := sample.EventCount
		if period == 0 {
			period = 1
		}
		weight, err := tracewire.CheckedPerfSampleWeight(period)
		if err != nil {
			return err
		}
		leaf, callchain, err := resolveSimpleperfProtoSampleFrames(ctx, sample, data.Files)
		if err != nil {
			return err
		}
		event := "unknown"
		if sample.EventTypeSet && int(sample.EventTypeID) < len(data.Meta.EventTypes) {
			event = data.Meta.EventTypes[sample.EventTypeID]
		}
		ts := float64(sample.TimeNS) / 1e9
		source := "simpleperf_report_proto"
		symbolizationStatus := perfTraceSymbolizationStatus(leaf.symbol, leaf.dso, source)
		callchainStatus := perfTraceCallchainStatus(callchain, source)
		sampleKind := simpleperfProtoSampleKind(sample, data.Meta, contextByThread[uint32(sample.ThreadID)])
		body, err := tracewire.BuildPerfSampleBody(tracewire.PerfSampleRow{
			Layout:              tracewire.PerfSampleLayoutBase,
			CPU:                 -1,
			CPUKnown:            false,
			PID:                 int64(pid),
			TID:                 int64(tid),
			ThreadComm:          firstNonEmpty(thread.Name, comm),
			SampleWeight:        weight,
			Event:               event,
			Symbol:              leaf.symbol,
			DSO:                 leaf.dso,
			IP:                  leaf.ip,
			Callchain:           callchain,
			Source:              tracewire.PerfSampleSourceSimpleperfReportProto,
			SampleKind:          tracewire.PerfSampleKind(sampleKind),
			SymbolizationStatus: tracewire.PerfSymbolizationStatus(symbolizationStatus),
			Clock:               tracewire.PerfSampleClockSimpleperfRecord,
			ClockConfidence:     tracewire.PerfClockConfidenceAssumed,
			CallchainStatus:     tracewire.PerfCallchainStatus(callchainStatus),
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%16s-%-5d (%5d) [%03d] .... %12.6f: %s\n",
			perfTraceHeaderComm(comm), tid, pid, 0, ts, body); err != nil {
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

func resolveSimpleperfProtoSampleFrames(ctx context.Context, sample simpleperfProtoSample, files map[uint32]simpleperfProtoFile) (resolvedSimpleperfProtoFrame, string, error) {
	if len(sample.Frames) == 0 {
		return resolvedSimpleperfProtoFrame{symbol: "unknown", dso: "unknown"}, "unknown", nil
	}
	leaf := resolveSimpleperfProtoFrame(sample.Frames[0], files)
	var builder tracewire.PerfCallchainBuilder
	for i := len(sample.Frames) - 1; i >= 0; i-- {
		frame := resolveSimpleperfProtoFrame(sample.Frames[i], files)
		part := frame.symbol
		if frame.dso != "" && frame.dso != "unknown" {
			part += "@" + frame.dso
		}
		if err := builder.AppendFrame(ctx, part); err != nil {
			return resolvedSimpleperfProtoFrame{}, "", err
		}
	}
	return leaf, builder.String(), nil
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
