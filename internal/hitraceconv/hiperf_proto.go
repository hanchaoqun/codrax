package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tracewire"
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

func maybeConvertHiperfPerfDataFromInput(
	ctx context.Context,
	opts Options,
	input standaloneHiperfInputBinding,
	resolution hiperfToolResolution,
	inputLease *externalToolInputLease,
	adapterDir *privateConversionDir,
	perfTracePath string,
	ledger *conversionFileLedger,
) (artifact Artifact, caveat string, decisions []PerfProviderDecision, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.validate(); err != nil {
		return Artifact{}, "", nil, err
	}
	perfPath := input.displayPath
	inputFormat := input.inputFormat
	stage := perfProviderStageStandaloneHiperf
	if inputLease == nil || inputLease.source != input.input || inputLease.transport != externalToolInputTransportSnapshot ||
		resolution.ExternalInputProfile != externalToolInputSnapshotOnly || adapterDir == nil {
		return Artifact{}, "", nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			perfPath,
			fmt.Errorf("standalone hiperf provider has no exact snapshot-only input lease"),
		)
	}
	privateStagingIdentity := capturePrivatePathIdentity(adapterDir.Path())
	defer func() {
		redactPerfProviderPrivateOutputs(&artifact, &caveat, &decisions, &err, privateStagingIdentity)
	}()
	if opts.DisablePerfAdapter {
		caveat := "HIPERF_DATA perf.data extracted; perftrace generation disabled, so .perftrace was not generated"
		decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNamePerftraceDisabled), opts, perfPath, inputFormat, perfTracePath)
		decision = perfProviderSkipped(decision, true, "perftrace_generation_disabled", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	if rawPerfParserRequired(opts) {
		officialDecision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameHiperfProto), opts, perfPath, inputFormat, perfTracePath)
		officialDecision = perfProviderSkipped(officialDecision, false, "skipped_by_raw_parser_mode", "official hiperf adapter skipped because raw perf parser mode was requested")
		artifact, caveat, decisions, err := maybeConvertRawPerfDataFromStandaloneInputWithDecision(ctx, opts, input, perfTracePath, "", stage, false, ledger)
		return artifact, caveat, append([]PerfProviderDecision{officialDecision}, decisions...), err
	}
	officialDecision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameHiperfProto), opts, perfPath, inputFormat, perfTracePath)
	if inputFormat != perfInputLinuxPerfData {
		reason := "unsupported_input_format"
		caveat := fmt.Sprintf("HIPERF_DATA sidecar preserved; official hiperf requires exact %s input, got %s", perfInputLinuxPerfData, firstNonEmpty(string(inputFormat), "unknown"))
		if inputFormat == perfInputGzipPerfData {
			reason = "unsafe_compressed_input_scratch"
			caveat = "HIPERF_DATA gzip sidecar preserved; official hiperf was not run because supported upstream versions use a fixed decompression scratch path"
		}
		officialDecision = perfProviderSkipped(officialDecision, true, reason, caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackFromStandaloneInput(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	tool, source := resolution.Tool, resolution.Source
	if tool == "" {
		caveat := "HIPERF_DATA perf.data extracted; no official hiperf_host/hiperf adapter was configured or found"
		officialDecision = perfProviderSkipped(officialDecision, true, "official_tool_unavailable", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackFromStandaloneInput(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	protoPath, err := adapterDir.ChildPath("report_sample.proto")
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}

	afterInput := []string{"-o", protoPath}
	if len(opts.HiperfSymbolDirs) > 0 {
		afterInput = append(afterInput, "--symbol-dir", strings.Join(opts.HiperfSymbolDirs, ","))
	}
	if err := adapterDir.Validate(); err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	cmd, err := inputLease.Command(ctx, tool, []string{"report", "--proto", "-i"}, afterInput)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	output, runErr, commandStart, commandStarted := runCommandWithProgressUntilExit(opts, cmd, "hiperf_adapter", "running official hiperf adapter")
	if boundaryErr := validateExternalToolCommandBoundary(ctx, inputLease, adapterDir, runErr); boundaryErr != nil {
		progressFinished(opts, "hiperf_adapter", "hiperf command boundary rejected", tool, "", commandStart, ProgressStatusFailed)
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, boundaryErr
	}
	commandStatus := ProgressStatusComplete
	if runErr != nil {
		commandStatus = ProgressStatusFailed
	}
	commandMessage := terminalProgressMessage("running official hiperf adapter", commandStatus)
	if !commandStarted {
		commandMessage = "external command failed to start"
	}
	progressFinished(opts, "hiperf_adapter", commandMessage, tool, "", commandStart, commandStatus)
	if runErr != nil {
		caveat := fmt.Sprintf("official hiperf adapter %q failed (%s)%s", tool, runErr, boundedPerfAdapterCommandOutput(output, "hiperf"))
		officialDecision = perfProviderFailure(officialDecision, "official_adapter_failed", caveat)
		artifact, rawCaveat, rawDecisions, err := maybeRawPerfFallbackFromStandaloneInput(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), err
	}
	sealedProto, err := adapterDir.AdoptRegularChild("report_sample.proto", true)
	if err != nil {
		caveat := fmt.Sprintf("official hiperf adapter %q produced unreadable protobuf (%v)", tool, err)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, fallbackErr := maybeRawPerfFallbackFromStandaloneInput(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), fallbackErr
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, sealedProto.Close())
	}()
	data, parseErr := readHiperfProtoAt(ctx, sealedProto, sealedProto.Size())
	parseErr = finishSealedConversionFile(sealedProto, parseErr)
	if parseErr == nil && len(data.Samples) == 0 {
		parseErr = fmt.Errorf("hiperf protobuf contains no sample records")
	}
	if parseErr == nil && hiperfProtoDataContainsPrivatePath(data, privateStagingIdentity) {
		parseErr = fmt.Errorf("hiperf protobuf contains a private adapter input identity")
	}
	if parseErr == nil {
		parseErr = writeHiperfProtoDataToPerfTraceWithLedger(ctx, data, perfTracePath, ledger)
	}
	if parseErr != nil {
		if ownedTraceOutputHardFailure(parseErr) {
			return Artifact{}, "", []PerfProviderDecision{officialDecision}, parseErr
		}
		caveat := fmt.Sprintf("official hiperf adapter %q produced unreadable protobuf (%v)", tool, parseErr)
		officialDecision = perfProviderFailure(officialDecision, "official_output_unreadable", caveat)
		artifact, rawCaveat, rawDecisions, fallbackErr := maybeRawPerfFallbackFromStandaloneInput(ctx, opts, input, perfTracePath, caveat, stage, ledger)
		return artifact, rawCaveat, append([]PerfProviderDecision{officialDecision}, rawDecisions...), fallbackErr
	}
	artifact, err = newValidatedPerfTraceArtifact(
		ledger, perfTracePath, ownedTracePerfHiperfProto, inputFormat, source, []string{
			fmt.Sprintf("generated from perf.data through %s; sample CPU is unavailable in OpenHarmony report_sample.proto and is emitted as cpu=-1", source),
		},
	)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	officialDecision, err = perfProviderSuccess(officialDecision, artifact, ledger)
	if err != nil {
		return Artifact{}, "", []PerfProviderDecision{officialDecision}, err
	}
	return artifact, "", []PerfProviderDecision{officialDecision}, nil
}

func hiperfProtoDataContainsPrivatePath(data hiperfProtoData, privateIdentity privatePathIdentity) bool {
	contains := func(value string) bool {
		for _, prefix := range privateIdentity.prefixes {
			if replaceAllASCIIPathFold(value, prefix, "<private>") != value {
				return true
			}
		}
		return false
	}
	for _, file := range data.Files {
		if contains(file.Path) {
			return true
		}
		for _, name := range file.FunctionNames {
			if contains(name) {
				return true
			}
		}
	}
	for _, thread := range data.Threads {
		if contains(thread.Name) {
			return true
		}
	}
	for _, name := range data.ConfigNames {
		if contains(name) {
			return true
		}
	}
	return false
}

func maybeRawPerfFallbackFromStandaloneInput(ctx context.Context, opts Options, input standaloneHiperfInputBinding, perfTracePath, prior string, stage string, ledger *conversionFileLedger) (Artifact, string, []PerfProviderDecision, error) {
	if err := input.validate(); err != nil {
		return Artifact{}, "", nil, err
	}
	if !rawPerfParserAllowed(opts) {
		if prior != "" {
			caveat := prior + "; raw perf.data fallback disabled by perf parser mode, so .perftrace was not generated"
			decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, input.displayPath, input.inputFormat, perfTracePath)
			decision = perfProviderSkipped(decision, false, "disabled_by_parser_mode", caveat)
			return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
		}
		caveat := "raw perf.data fallback disabled by perf parser mode, so .perftrace was not generated"
		decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, input.displayPath, input.inputFormat, perfTracePath)
		decision = perfProviderSkipped(decision, false, "disabled_by_parser_mode", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	artifact, caveat, decisions, err := maybeConvertRawPerfDataFromStandaloneInputWithDecision(ctx, opts, input, perfTracePath, prior, stage, true, ledger)
	if err != nil || artifact.Path != "" {
		if prior != "" && artifact.Path != "" {
			artifact.Caveats = append([]string{prior + "; fell back to raw perf.data parser"}, artifact.Caveats...)
		}
		return artifact, caveat, decisions, err
	}
	if prior != "" && caveat != "" {
		return Artifact{}, prior + "; " + caveat, decisions, nil
	}
	return Artifact{}, firstNonEmpty(caveat, prior), decisions, nil
}

type hiperfToolResolution struct {
	Tool                 string
	Source               string
	ExternalInputProfile externalToolInputProfile
}

func resolveHiperfProviderTool(opts Options) hiperfToolResolution {
	tool, source := resolveHiperfTool(opts)
	return hiperfToolResolution{
		Tool: tool, Source: source, ExternalInputProfile: externalToolInputSnapshotOnly,
	}
}

func resolveHiperfTool(opts Options) (string, string) {
	if path := strings.TrimSpace(opts.HiperfPath); path != "" {
		return path, "configured hiperf tool"
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_HIPERF_HOST")); path != "" {
		return path, "CODRAX_HIPERF_HOST"
	}
	for _, candidate := range hiperfCodraxBinaryDirCandidates() {
		if traceToolPathUsable(candidate, nil) {
			return candidate, "codrax executable directory"
		}
	}
	for _, name := range hiperfBinaryNames() {
		if path, err := exec.LookPath(name); err == nil && strings.TrimSpace(path) != "" {
			return path, name + " on PATH"
		}
	}
	for _, candidate := range hiperfKnownLocationCandidates() {
		if path := firstUsableHiperfCandidate(candidate); path != "" {
			return path, "known OpenHarmony hiperf location"
		}
	}
	return "", ""
}

func hiperfCodraxBinaryDirCandidates() []string {
	exe, err := traceStreamerExecutablePath()
	if err != nil {
		return nil
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return nil
	}
	return hiperfCodraxBinaryDirCandidatesFor(filepath.Dir(exe), runtime.GOOS, runtime.GOARCH)
}

func hiperfCodraxBinaryDirCandidatesFor(dir, goos, goarch string) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	var out []string
	for _, name := range hiperfBinaryNamesFor(goos) {
		out = append(out, filepath.Join(dir, name))
		for _, platformDir := range traceStreamerHostPlatformDirsFor(goos, goarch) {
			out = append(out,
				filepath.Join(dir, platformDir, name),
				filepath.Join(dir, "hiperf", platformDir, name),
				filepath.Join(dir, "hiperf-host", platformDir, name),
				filepath.Join(dir, "developtools_hiperf", platformDir, name),
			)
		}
	}
	return uniqueNonEmptyStrings(out)
}

func hiperfBinaryNames() []string {
	return hiperfBinaryNamesFor(runtime.GOOS)
}

func hiperfBinaryNamesFor(goos string) []string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return []string{"hiperf_host.exe", "hiperf.exe"}
	}
	return []string{"hiperf_host", "hiperf"}
}

func hiperfKnownLocationCandidates() []string {
	names := hiperfBinaryNames()
	var out []string
	for _, env := range []string{"HIPERF_HOME", "CODRAX_HIPERF_HOME", "OHOS_SDK_HOME", "HARMONYOS_SDK_HOME", "DEVECO_SDK_HOME"} {
		root := strings.TrimSpace(os.Getenv(env))
		if root == "" {
			continue
		}
		for _, name := range names {
			out = append(out,
				filepath.Join(root, name),
				filepath.Join(root, "bin", name),
				filepath.Join(root, "toolchains", name),
				filepath.Join(root, "hiperf", name),
				filepath.Join(root, "developtools_hiperf", name),
			)
		}
	}
	for _, name := range names {
		switch runtime.GOOS {
		case "darwin":
			out = append(out,
				filepath.Join("/Applications", "DevEco-Studio.app", "Contents", "sdk", "toolchains", name),
				filepath.Join("/usr/local/bin", name),
				filepath.Join("/opt/homebrew/bin", name),
			)
		case "linux":
			out = append(out,
				filepath.Join("/usr/local/bin", name),
				filepath.Join("/usr/bin", name),
				filepath.Join("/opt/openharmony", "toolchains", name),
			)
		}
	}
	return out
}

func firstUsableHiperfCandidate(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	candidates := []string{pattern}
	if strings.ContainsAny(pattern, "*?[") {
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			candidates = matches
		} else {
			candidates = nil
		}
	}
	for _, candidate := range candidates {
		if traceToolPathUsable(candidate, nil) {
			return candidate
		}
	}
	return ""
}

func ConvertHiperfProtoFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	return runConversionFileTransaction(ctx, inputPath, func(ledger *conversionFileLedger) error {
		return convertHiperfProtoFileToPerfTraceWithLedger(ctx, inputPath, outputPath, ledger)
	})
}

func convertHiperfProtoFileToPerfTraceWithLedger(ctx context.Context, inputPath, outputPath string, ledger *conversionFileLedger) error {
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
	return writeHiperfProtoDataToPerfTraceWithLedger(ctx, data, outputPath, ledger)
}

func writeHiperfProtoDataToPerfTraceWithLedger(ctx context.Context, data hiperfProtoData, outputPath string, ledger *conversionFileLedger) error {
	if len(data.Samples) == 0 {
		return fmt.Errorf("hiperf protobuf contains no sample records")
	}
	_, err := writeValidatedOwnedPerfTraceWithLedger(
		ctx, ownedPerfTraceWriteSpec{Profile: ownedTracePerfHiperfProto, ExpectedRows: len(data.Samples)}, outputPath, ledger,
		func(writer io.Writer) error { return writeHiperfPerfTrace(ctx, writer, data) },
	)
	return err
}

func readHiperfProtoFile(ctx context.Context, path string) (hiperfProtoData, error) {
	f, err := os.Open(path)
	if err != nil {
		return hiperfProtoData{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return hiperfProtoData{}, err
	}
	return readHiperfProtoAt(ctx, f, info.Size())
}

func readHiperfProtoAt(ctx context.Context, reader io.ReaderAt, size int64) (hiperfProtoData, error) {
	if reader == nil {
		return hiperfProtoData{}, fmt.Errorf("hiperf protobuf reader is nil")
	}
	if size < 0 {
		return hiperfProtoData{}, fmt.Errorf("hiperf protobuf size is negative: %d", size)
	}
	return readHiperfProto(ctx, io.NewSectionReader(reader, 0, size), size)
}

func readHiperfProto(ctx context.Context, reader io.Reader, size int64) (hiperfProtoData, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if size < 0 {
		return hiperfProtoData{}, fmt.Errorf("hiperf protobuf size is negative: %d", size)
	}
	var magic [len(hiperfProtoMagic)]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf magic: %w", err)
	}
	if string(magic[:]) != hiperfProtoMagic {
		return hiperfProtoData{}, fmt.Errorf("unsupported hiperf protobuf magic %q", string(magic[:]))
	}
	var versionBuf [2]byte
	if _, err := io.ReadFull(reader, versionBuf[:]); err != nil {
		return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf version: %w", err)
	}
	if version := binary.LittleEndian.Uint16(versionBuf[:]); version != hiperfProtoVersion {
		return hiperfProtoData{}, fmt.Errorf("unsupported hiperf protobuf version %d", version)
	}
	remaining := size - int64(len(magic)+len(versionBuf))
	data := hiperfProtoData{
		Files:   map[uint32]hiperfProtoFile{},
		Threads: map[uint32]hiperfProtoThread{},
	}
	var sizeBuf [4]byte
	for {
		if err := ctx.Err(); err != nil {
			return hiperfProtoData{}, err
		}
		if _, err := io.ReadFull(reader, sizeBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return hiperfProtoData{}, fmt.Errorf("truncated hiperf protobuf record size: %w", err)
			}
			return hiperfProtoData{}, err
		}
		remaining -= int64(len(sizeBuf))
		recordSize := binary.LittleEndian.Uint32(sizeBuf[:])
		if recordSize == 0 {
			break
		}
		if err := ctx.Err(); err != nil {
			return hiperfProtoData{}, err
		}
		if remaining < 0 || int64(recordSize) > remaining {
			return hiperfProtoData{}, fmt.Errorf("hiperf protobuf record size %d exceeds fixed input remainder %d", recordSize, remaining)
		}
		if uint64(recordSize) > uint64(^uint(0)>>1) {
			return hiperfProtoData{}, fmt.Errorf("hiperf protobuf record size %d exceeds host allocation limit", recordSize)
		}
		record := make([]byte, recordSize)
		if _, err := io.ReadFull(reader, record); err != nil {
			return hiperfProtoData{}, fmt.Errorf("read hiperf protobuf record: %w", err)
		}
		remaining -= int64(recordSize)
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

type protoFieldDecodeFailure uint8

const (
	protoFieldDecodeMalformedKey protoFieldDecodeFailure = iota
	protoFieldDecodeInvalidFieldNumber
	protoFieldDecodeMalformedValue
	protoFieldDecodeUnsupportedWire
)

// protoFieldDecodeError preserves the established human-readable parser error
// while exposing exact endpoint provenance to typed consumers. FieldKnown is
// false for a malformed key or invalid protobuf field number; callers must not
// recover a field identity by parsing Message.
type protoFieldDecodeError struct {
	Failure    protoFieldDecodeFailure
	Field      int
	FieldKnown bool
	// Terminal is true only when the malformed value consumes the physical
	// payload tail. A typed consumer may then localize an optional display
	// endpoint without risking that unparsed bytes conceal a hard endpoint.
	Terminal bool
	Message  string
}

func (err *protoFieldDecodeError) Error() string {
	if err == nil {
		return "malformed protobuf field"
	}
	return err.Message
}

func walkProtoFields(data []byte, fn func(field int, wire int, raw []byte, v uint64) error) error {
	for len(data) > 0 {
		key, n, ok := consumeProtoVarint(data)
		if !ok {
			return &protoFieldDecodeError{
				Failure: protoFieldDecodeMalformedKey, Message: "malformed protobuf field key",
			}
		}
		data = data[n:]
		fieldNumber := key >> 3
		wire := int(key & 0x7)
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			return &protoFieldDecodeError{
				Failure: protoFieldDecodeInvalidFieldNumber,
				Message: fmt.Sprintf("invalid protobuf field number %d", fieldNumber),
			}
		}
		field := int(fieldNumber)
		switch wire {
		case 0:
			v, n, ok := consumeProtoVarint(data)
			if !ok {
				return &protoFieldDecodeError{
					Failure: protoFieldDecodeMalformedValue, Field: field, FieldKnown: true,
					Terminal: protoVarintFailureConsumesTail(data),
					Message:  fmt.Sprintf("malformed protobuf varint field %d", field),
				}
			}
			if err := fn(field, wire, nil, v); err != nil {
				return err
			}
			data = data[n:]
		case 1:
			if len(data) < 8 {
				return &protoFieldDecodeError{
					Failure: protoFieldDecodeMalformedValue, Field: field, FieldKnown: true,
					Terminal: true,
					Message:  fmt.Sprintf("truncated protobuf fixed64 field %d", field),
				}
			}
			if err := fn(field, wire, data[:8], binary.LittleEndian.Uint64(data[:8])); err != nil {
				return err
			}
			data = data[8:]
		case 2:
			l, n, ok := consumeProtoVarint(data)
			if !ok {
				return &protoFieldDecodeError{
					Failure: protoFieldDecodeMalformedValue, Field: field, FieldKnown: true,
					Terminal: protoVarintFailureConsumesTail(data),
					Message:  fmt.Sprintf("truncated protobuf bytes field %d", field),
				}
			}
			if l > uint64(len(data[n:])) {
				return &protoFieldDecodeError{
					Failure: protoFieldDecodeMalformedValue, Field: field, FieldKnown: true,
					Terminal: true,
					Message:  fmt.Sprintf("truncated protobuf bytes field %d", field),
				}
			}
			raw := data[n : n+int(l)]
			if err := fn(field, wire, raw, 0); err != nil {
				return err
			}
			data = data[n+int(l):]
		case 5:
			if len(data) < 4 {
				return &protoFieldDecodeError{
					Failure: protoFieldDecodeMalformedValue, Field: field, FieldKnown: true,
					Terminal: true,
					Message:  fmt.Sprintf("truncated protobuf fixed32 field %d", field),
				}
			}
			if err := fn(field, wire, data[:4], uint64(binary.LittleEndian.Uint32(data[:4]))); err != nil {
				return err
			}
			data = data[4:]
		default:
			return &protoFieldDecodeError{
				Failure: protoFieldDecodeUnsupportedWire, Field: field, FieldKnown: true,
				Terminal: len(data) == 0,
				Message:  fmt.Sprintf("unsupported protobuf wire type %d for field %d", wire, field),
			}
		}
	}
	return nil
}

// consumeProtoVarint rejects an unterminated/overflowing varint no later than
// byte ten. When at most ten bytes remain, the failure necessarily reaches the
// physical payload tail; with more bytes, their protobuf field identity is
// unknowable and typed consumers must fail the entire payload closed.
func protoVarintFailureConsumesTail(data []byte) bool {
	return len(data) <= 10
}

func consumeProtoVarint(data []byte) (uint64, int, bool) {
	var v uint64
	for i, b := range data {
		if i >= 10 || (i == 9 && b > 1) {
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
		period := sample.EventCount
		if period == 0 {
			period = 1
		}
		weight, err := tracewire.CheckedPerfSampleWeight(period)
		if err != nil {
			return err
		}
		leaf, callchain, err := resolveHiperfSampleFrames(ctx, sample, data.Files)
		if err != nil {
			return err
		}
		event := "unknown"
		if sample.ConfigSet && int(sample.ConfigNameID) < len(data.ConfigNames) {
			event = data.ConfigNames[sample.ConfigNameID]
		}
		ts := float64(sample.TimeNS) / 1e9
		source := "hiperf_proto"
		symbolizationStatus := perfTraceSymbolizationStatus(leaf.symbol, leaf.dso, source)
		callchainStatus := perfTraceCallchainStatus(callchain, source)
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
			Source:              tracewire.PerfSampleSourceHiperfProto,
			SymbolizationStatus: tracewire.PerfSymbolizationStatus(symbolizationStatus),
			Clock:               tracewire.PerfSampleClockMonotonicRaw,
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

type resolvedHiperfFrame struct {
	symbol string
	dso    string
	ip     string
}

func resolveHiperfSampleFrames(ctx context.Context, sample hiperfProtoSample, files map[uint32]hiperfProtoFile) (resolvedHiperfFrame, string, error) {
	if len(sample.Frames) == 0 {
		return resolvedHiperfFrame{symbol: "unknown", dso: "unknown"}, "unknown", nil
	}
	leaf := resolveHiperfFrame(sample.Frames[0], files)
	var builder tracewire.PerfCallchainBuilder
	for i := len(sample.Frames) - 1; i >= 0; i-- {
		frame := resolveHiperfFrame(sample.Frames[i], files)
		part := frame.symbol
		if frame.dso != "" && frame.dso != "unknown" {
			part += "@" + frame.dso
		}
		if err := builder.AppendFrame(ctx, part); err != nil {
			return resolvedHiperfFrame{}, "", err
		}
	}
	return leaf, builder.String(), nil
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

func perfTraceHeaderComm(comm string) string {
	return traceDBCommName(sanitizePerfTraceComm(comm), "hiperf")
}

func quoteTraceValue(raw string) string {
	return tracewire.QuotePerfKVValue(raw)
}
