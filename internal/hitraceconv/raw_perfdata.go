package hitraceconv

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
	"strconv"
	"strings"
)

const (
	rawPerfDataAdapterVersion = converterVersion + "+raw-perfdata"
	perfMagic2                = "PERFILE2"

	perfRecordMmap   = 1
	perfRecordComm   = 3
	perfRecordExit   = 4
	perfRecordFork   = 7
	perfRecordSample = 9
	perfRecordMmap2  = 10

	perfFeatureBuildID  = 2
	perfFeatureArch     = 6
	perfFeatureCmdline  = 11
	perfFeatureMetaInfo = 129

	perfFeatureHiperfFilesSymbol = 192

	perfSampleIP           = 1 << 0
	perfSampleTID          = 1 << 1
	perfSampleTime         = 1 << 2
	perfSampleAddr         = 1 << 3
	perfSampleRead         = 1 << 4
	perfSampleCallchain    = 1 << 5
	perfSampleID           = 1 << 6
	perfSampleCPU          = 1 << 7
	perfSamplePeriod       = 1 << 8
	perfSampleStreamID     = 1 << 9
	perfSampleRaw          = 1 << 10
	perfSampleBranchStack  = 1 << 11
	perfSampleRegsUser     = 1 << 12
	perfSampleStackUser    = 1 << 13
	perfSampleWeight       = 1 << 14
	perfSampleDataSrc      = 1 << 15
	perfSampleIdentifier   = 1 << 16
	perfSampleTransaction  = 1 << 17
	perfSampleRegsIntr     = 1 << 18
	perfSamplePhysAddr     = 1 << 19
	perfSampleAux          = 1 << 20
	perfSampleCGroup       = 1 << 21
	perfSampleDataPageSize = 1 << 22
	perfSampleCodePageSize = 1 << 23

	rawPerfVendorSampleBit31 = 1 << 31

	perfFormatTotalTimeEnabled = 1 << 0
	perfFormatTotalTimeRunning = 1 << 1
	perfFormatID               = 1 << 2
	perfFormatGroup            = 1 << 3
	perfFormatLost             = 1 << 4
)

const (
	rawPerfParsedSampleBits    = perfSampleIP | perfSampleTID | perfSampleTime | perfSampleAddr | perfSampleCallchain | perfSampleID | perfSampleCPU | perfSamplePeriod | perfSampleStreamID | perfSampleIdentifier
	rawPerfSkippedSampleBits   = perfSampleRead | perfSampleRaw | perfSampleBranchStack | perfSampleRegsUser | perfSampleStackUser | perfSampleWeight | perfSampleDataSrc | perfSampleTransaction | perfSampleRegsIntr | perfSamplePhysAddr | perfSampleAux | perfSampleCGroup | perfSampleDataPageSize | perfSampleCodePageSize | rawPerfVendorSampleBit31
	rawPerfSupportedSampleBits = rawPerfParsedSampleBits | rawPerfSkippedSampleBits

	rawPerfSymbolFileKernel       = 0
	rawPerfSymbolFileKernelModule = 1
	rawPerfSymbolFileKernelThread = 2
	rawPerfSymbolFileELF          = 3
	rawPerfSymbolFileJava         = 4
	rawPerfSymbolFileJS           = 5
	rawPerfSymbolFileHAP          = 6
	rawPerfSymbolFileCJ           = 7
	rawPerfSymbolFileV8           = 8
)

type rawPerfAttr struct {
	SampleType       uint64
	ReadFormat       uint64
	BranchSampleType uint64
	SampleRegsUser   uint64
	SampleStackUser  uint32
	SampleRegsIntr   uint64
	EventName        string
	IDs              []uint64
	Caveats          []string
}

type rawPerfFileHeader struct {
	AttrSize    uint64
	AttrsOffset uint64
	AttrsSize   uint64
	DataOffset  uint64
	DataSize    uint64
	Features    [32]byte
}

type rawPerfSample struct {
	IP        uint64
	PID       int
	TID       int
	TimeNS    uint64
	CPU       int
	CPUValid  bool
	Period    uint64
	ID        uint64
	EventName string
	Comm      string
	Callchain []uint64
}

type rawPerfMapping struct {
	PID   int
	TID   int
	Addr  uint64
	Len   uint64
	Pgoff uint64
	Path  string
}

type rawPerfSymbol struct {
	Vaddr uint64
	Len   uint32
	Name  string
}

type rawPerfSymbolFile struct {
	Path                    string
	SymbolType              uint32
	TextExecVaddr           uint64
	TextExecVaddrFileOffset uint64
	BuildID                 string
	Symbols                 []rawPerfSymbol
}

type rawPerfResolvedFrame struct {
	IP         string
	DSO        string
	Symbol     string
	Symbolized bool
}

type rawPerfData struct {
	SampleType uint64
	ReadFormat uint64
	EventName  string
	Attrs      []rawPerfAttr
	Features   rawPerfFeatures
	Threads    map[int]string
	Mappings   []rawPerfMapping
	Samples    []rawPerfSample
	Caveats    []string
}

type rawPerfFeatures struct {
	Present      []int
	Arch         string
	Cmdline      []string
	Meta         map[string]string
	BuildIDs     map[string]string
	SymbolFiles  []rawPerfSymbolFile
	BuildIDCount int
	SymbolCount  int
	Caveats      []string
}

func rawPerfParserAllowed(opts Options) bool {
	mode := normalizePerfParserMode(opts.PerfParser)
	return mode == "raw" || mode == "fallback" || mode == "auto" || mode == ""
}

func rawPerfParserRequired(opts Options) bool {
	mode := normalizePerfParserMode(opts.PerfParser)
	return mode == "raw" || mode == "fallback"
}

func normalizePerfParserMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func validatePerfParserMode(mode string) error {
	switch normalizePerfParserMode(mode) {
	case "", "auto", "official", "raw", "fallback":
		return nil
	default:
		return fmt.Errorf("unsupported perf parser mode %q; use auto, official, or raw", mode)
	}
}

func maybeConvertRawPerfData(ctx context.Context, opts Options, perfPath, perfTracePath string) (Artifact, string, error) {
	if !rawPerfParserAllowed(opts) {
		return Artifact{}, "raw perf.data fallback disabled by perf parser mode", nil
	}
	if err := ensureOutputDoesNotExist(perfTracePath); err != nil {
		return Artifact{}, "", err
	}
	if err := ConvertRawPerfDataFileToPerfTrace(ctx, perfPath, perfTracePath); err != nil {
		_ = os.Remove(perfTracePath)
		return Artifact{}, fmt.Sprintf("raw perf.data fallback could not parse %q (%v); .perftrace was not generated", perfPath, err), nil
	}
	info, err := os.Stat(perfTracePath)
	if err != nil {
		return Artifact{}, "", err
	}
	return Artifact{
		Type:      ArtifactPerfTrace,
		Path:      perfTracePath,
		Bytes:     info.Size(),
		Converter: rawPerfDataAdapterVersion,
		Perf:      perfCapabilityForRawFallback(detectPerfInputFormat(perfPath)),
		Caveats: []string{
			"generated by Codrax raw perf.data fallback; saved hiperf symbol sections are used when present, otherwise samples remain IP/DSO-level",
		},
	}, "", nil
}

func maybeConvertRawPerfDataWithDecision(ctx context.Context, opts Options, perfPath, perfTracePath, prior, stage string, inputFormat perfInputFormat, fallback bool) (Artifact, string, []PerfProviderDecision, error) {
	if inputFormat == perfInputUnknown {
		inputFormat = detectPerfInputFormat(perfPath)
	}
	decision := newPerfProviderDecision(stage, perfProviderByName(perfProviderNameRawFallback), opts, perfPath, inputFormat, perfTracePath)
	decision.Fallback = fallback
	if !rawPerfParserAllowed(opts) {
		caveat := "raw perf.data fallback disabled by perf parser mode"
		if prior != "" {
			caveat = prior + "; " + caveat
		}
		decision = perfProviderSkipped(decision, false, "disabled_by_parser_mode", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	if !perfProviderSupportsInput(perfProviderByName(perfProviderNameRawFallback), inputFormat) {
		caveat := fmt.Sprintf("Codrax raw fallback supports %s only, got %s; .perftrace was not generated", perfInputLinuxPerfData, firstNonEmpty(string(inputFormat), "unknown"))
		if prior != "" {
			caveat = prior + "; " + caveat
		}
		decision = perfProviderSkipped(decision, true, "unsupported_input_format", caveat)
		return Artifact{}, caveat, []PerfProviderDecision{decision}, nil
	}
	artifact, caveat, err := maybeConvertRawPerfData(ctx, opts, perfPath, perfTracePath)
	if err != nil {
		decision = perfProviderFailure(decision, "raw_parser_error", err.Error())
		return artifact, caveat, []PerfProviderDecision{decision}, err
	}
	if artifact.Path == "" {
		fullCaveat := caveat
		if prior != "" && caveat != "" {
			fullCaveat = prior + "; " + caveat
		} else if prior != "" {
			fullCaveat = prior
		}
		decision = perfProviderFailure(decision, "raw_parser_unavailable", fullCaveat)
		return artifact, caveat, []PerfProviderDecision{decision}, nil
	}
	decision = perfProviderSuccess(decision, artifact)
	return artifact, caveat, []PerfProviderDecision{decision}, nil
}

func ConvertRawPerfDataFileToPerfTrace(ctx context.Context, inputPath, outputPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := readRawPerfData(ctx, inputPath)
	if err != nil {
		return err
	}
	if len(data.Samples) == 0 {
		return fmt.Errorf("raw perf.data contains no supported sample records")
	}
	if err := ensureOutputDoesNotExist(outputPath); err != nil {
		return err
	}
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	writeErr := writeRawPerfDataPerfTrace(ctx, w, data)
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

func readRawPerfData(ctx context.Context, path string) (rawPerfData, error) {
	f, err := os.Open(path)
	if err != nil {
		return rawPerfData{}, err
	}
	defer f.Close()
	header, err := readRawPerfHeader(f)
	if err != nil {
		return rawPerfData{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return rawPerfData{}, err
	}
	features := readRawPerfFeatures(f, header, info.Size())
	attrs, err := readRawPerfAttrs(f, header)
	if err != nil {
		return rawPerfData{}, err
	}
	attr := attrs[0]
	for _, candidate := range attrs[1:] {
		if candidate.SampleType != attr.SampleType || candidate.ReadFormat != attr.ReadFormat {
			return rawPerfData{}, fmt.Errorf("raw fallback cannot safely parse multi-event perf.data with different sample layouts; use official hiperf/simpleperf tooling")
		}
	}
	unsupported := attr.SampleType &^ rawPerfSupportedSampleBits
	if unsupported != 0 {
		return rawPerfData{}, fmt.Errorf("unsupported sample_type bits 0x%x (%s) in raw fallback", unsupported, rawPerfSampleBitNames(unsupported))
	}
	skipped := attr.SampleType & rawPerfSkippedSampleBits
	if skipped != 0 {
		attr.Caveats = append(attr.Caveats, fmt.Sprintf("raw fallback skipped non-causal sample payload field(s): %s", rawPerfSampleBitNames(skipped)))
	}
	out := rawPerfData{
		SampleType: attr.SampleType,
		ReadFormat: attr.ReadFormat,
		EventName:  attr.EventName,
		Attrs:      attrs,
		Features:   features,
		Threads:    map[int]string{},
		Caveats:    attr.Caveats,
	}
	out.Caveats = append(out.Caveats, rawPerfFeatureCaveats(features)...)
	eventIDs := rawPerfEventIDMap(attrs)
	if len(attrs) > 1 {
		if len(eventIDs) > 0 {
			out.Caveats = append(out.Caveats, fmt.Sprintf("raw fallback parsed %d perf attrs and maps sample ids to event names when present", len(attrs)))
		} else {
			out.Caveats = append(out.Caveats, fmt.Sprintf("raw fallback parsed %d perf attrs but no event id section was present; event labels are best-effort", len(attrs)))
		}
	}
	if _, err := f.Seek(int64(header.DataOffset), io.SeekStart); err != nil {
		return rawPerfData{}, err
	}
	activeThreads := map[int]string{}
	remaining := int64(header.DataSize)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return rawPerfData{}, err
		}
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return rawPerfData{}, fmt.Errorf("read perf record header: %w", err)
		}
		typ := binary.LittleEndian.Uint32(hdr[0:4])
		size := int(binary.LittleEndian.Uint16(hdr[6:8]))
		if size < len(hdr) || int64(size) > remaining {
			return rawPerfData{}, fmt.Errorf("invalid perf record size %d", size)
		}
		payload := make([]byte, size-len(hdr))
		if _, err := io.ReadFull(f, payload); err != nil {
			return rawPerfData{}, fmt.Errorf("read perf record payload: %w", err)
		}
		switch typ {
		case perfRecordComm:
			comm, ok := parseRawPerfComm(payload)
			if ok {
				out.Threads[comm.TID] = comm.Name
				activeThreads[comm.TID] = comm.Name
				if comm.PID > 0 {
					out.Threads[comm.PID] = comm.Name
					activeThreads[comm.PID] = comm.Name
				}
			}
		case perfRecordFork:
			lifetime, ok := parseRawPerfLifetime(payload)
			if ok {
				name := firstNonEmpty(activeThreads[lifetime.PTID], activeThreads[lifetime.PPID])
				if name != "" {
					out.Threads[lifetime.TID] = name
					activeThreads[lifetime.TID] = name
					if lifetime.PID > 0 {
						out.Threads[lifetime.PID] = name
						activeThreads[lifetime.PID] = name
					}
				}
			}
		case perfRecordExit:
			lifetime, ok := parseRawPerfLifetime(payload)
			if ok {
				delete(activeThreads, lifetime.TID)
				if lifetime.PID > 0 {
					delete(activeThreads, lifetime.PID)
				}
			}
		case perfRecordMmap:
			if mapping, ok := parseRawPerfMmap(payload); ok {
				out.Mappings = append(out.Mappings, mapping)
			}
		case perfRecordMmap2:
			if mapping, ok := parseRawPerfMmap2(payload); ok {
				out.Mappings = append(out.Mappings, mapping)
			}
		case perfRecordSample:
			sample, ok := parseRawPerfSample(payload, attr)
			if ok {
				sample.EventName = rawPerfEventNameForSample(sample, attr.EventName, eventIDs)
				sample.Comm = firstNonEmpty(activeThreads[sample.TID], activeThreads[sample.PID])
				out.Samples = append(out.Samples, sample)
			}
		default:
		}
		remaining -= int64(size)
	}
	return out, nil
}

func readRawPerfHeader(r io.ReaderAt) (rawPerfFileHeader, error) {
	buf := make([]byte, 104)
	if _, err := r.ReadAt(buf, 0); err != nil {
		return rawPerfFileHeader{}, fmt.Errorf("read perf.data header: %w", err)
	}
	if string(buf[:8]) != perfMagic2 {
		return rawPerfFileHeader{}, fmt.Errorf("unsupported perf.data magic %q", string(buf[:8]))
	}
	size := binary.LittleEndian.Uint64(buf[8:16])
	if size < 80 {
		return rawPerfFileHeader{}, fmt.Errorf("unsupported perf.data header size %d", size)
	}
	var features [32]byte
	copy(features[:], buf[72:104])
	return rawPerfFileHeader{
		AttrSize:    binary.LittleEndian.Uint64(buf[16:24]),
		AttrsOffset: binary.LittleEndian.Uint64(buf[24:32]),
		AttrsSize:   binary.LittleEndian.Uint64(buf[32:40]),
		DataOffset:  binary.LittleEndian.Uint64(buf[40:48]),
		DataSize:    binary.LittleEndian.Uint64(buf[48:56]),
		Features:    features,
	}, nil
}

func readRawPerfAttrs(r io.ReaderAt, header rawPerfFileHeader) ([]rawPerfAttr, error) {
	if header.AttrsOffset == 0 || header.AttrsSize == 0 {
		return nil, fmt.Errorf("perf.data has no attrs section")
	}
	attrSize := header.AttrSize
	if attrSize == 0 || attrSize > header.AttrsSize {
		attrSize = header.AttrsSize
	}
	if attrSize < 32 {
		return nil, fmt.Errorf("perf attr too small: %d", attrSize)
	}
	count := int(header.AttrsSize / attrSize)
	if count <= 0 {
		count = 1
	}
	if count > 1024 {
		return nil, fmt.Errorf("too many perf attrs: %d", count)
	}
	attrs := make([]rawPerfAttr, 0, count)
	for i := 0; i < count; i++ {
		attr := make([]byte, attrSize)
		offset := int64(header.AttrsOffset + uint64(i)*attrSize)
		if _, err := r.ReadAt(attr, offset); err != nil {
			return nil, fmt.Errorf("read perf attr %d: %w", i, err)
		}
		parsed, err := parseRawPerfAttrEntry(r, attr)
		if err != nil {
			return nil, fmt.Errorf("parse perf attr %d: %w", i, err)
		}
		attrs = append(attrs, parsed)
	}
	if rem := header.AttrsSize % attrSize; rem != 0 {
		attrs[0].Caveats = append(attrs[0].Caveats, fmt.Sprintf("raw fallback ignored %d trailing perf attr byte(s)", rem))
	}
	return attrs, nil
}

func parseRawPerfAttrEntry(r io.ReaderAt, attr []byte) (rawPerfAttr, error) {
	if len(attr) < 32 {
		return rawPerfAttr{}, fmt.Errorf("perf attr too small: %d", len(attr))
	}
	config := binary.LittleEndian.Uint64(attr[8:16])
	out := rawPerfAttr{
		SampleType: binary.LittleEndian.Uint64(attr[24:32]),
		EventName:  "config:0x" + strconv.FormatUint(config, 16),
	}
	if len(attr) >= 40 {
		out.ReadFormat = binary.LittleEndian.Uint64(attr[32:40])
	}
	if len(attr) >= 80 {
		out.BranchSampleType = binary.LittleEndian.Uint64(attr[72:80])
	}
	if len(attr) >= 88 {
		out.SampleRegsUser = binary.LittleEndian.Uint64(attr[80:88])
	}
	if len(attr) >= 92 {
		out.SampleStackUser = binary.LittleEndian.Uint32(attr[88:92])
	}
	if len(attr) >= 104 {
		out.SampleRegsIntr = binary.LittleEndian.Uint64(attr[96:104])
	}
	attrStructSize := int(binary.LittleEndian.Uint32(attr[4:8]))
	if attrStructSize <= 0 || attrStructSize > len(attr) {
		attrStructSize = 0
	}
	if attrStructSize > 0 && len(attr) >= attrStructSize+16 {
		idsOffset := binary.LittleEndian.Uint64(attr[attrStructSize : attrStructSize+8])
		idsSize := binary.LittleEndian.Uint64(attr[attrStructSize+8 : attrStructSize+16])
		ids, caveat := readRawPerfAttrIDs(r, idsOffset, idsSize)
		out.IDs = ids
		if caveat != "" {
			out.Caveats = append(out.Caveats, caveat)
		}
	}
	return out, nil
}

func readRawPerfAttrIDs(r io.ReaderAt, offset, size uint64) ([]uint64, string) {
	if offset == 0 || size == 0 {
		return nil, ""
	}
	if size%8 != 0 {
		return nil, fmt.Sprintf("raw fallback ignored malformed perf attr ids section size=%d", size)
	}
	count := size / 8
	if count > 4096 {
		return nil, fmt.Sprintf("raw fallback ignored oversized perf attr ids section count=%d", count)
	}
	buf := make([]byte, size)
	if _, err := r.ReadAt(buf, int64(offset)); err != nil {
		return nil, fmt.Sprintf("raw fallback could not read perf attr ids section: %v", err)
	}
	ids := make([]uint64, 0, count)
	for i := uint64(0); i < count; i++ {
		ids = append(ids, binary.LittleEndian.Uint64(buf[i*8:i*8+8]))
	}
	return ids, ""
}

func rawPerfEventIDMap(attrs []rawPerfAttr) map[uint64]string {
	out := make(map[uint64]string)
	for _, attr := range attrs {
		for _, id := range attr.IDs {
			if id != 0 && attr.EventName != "" {
				out[id] = attr.EventName
			}
		}
	}
	return out
}

func rawPerfEventNameForSample(sample rawPerfSample, fallback string, eventIDs map[uint64]string) string {
	if sample.ID != 0 {
		if event := eventIDs[sample.ID]; event != "" {
			return event
		}
	}
	return fallback
}

func readRawPerfFeatures(r io.ReaderAt, header rawPerfFileHeader, fileSize int64) rawPerfFeatures {
	ids := rawPerfFeatureIDs(header.Features)
	features := rawPerfFeatures{Present: ids, Meta: map[string]string{}}
	if len(ids) == 0 {
		return features
	}
	if len(ids) > 128 {
		features.Caveats = append(features.Caveats, fmt.Sprintf("raw fallback skipped perf feature descriptors: too many features (%d)", len(ids)))
		return features
	}
	descOffset := header.DataOffset + header.DataSize
	descBytes := uint64(len(ids) * 16)
	if descOffset == 0 || descBytes == 0 || descOffset+descBytes < descOffset || (fileSize > 0 && descOffset+descBytes > uint64(fileSize)) {
		features.Caveats = append(features.Caveats, "raw fallback skipped malformed perf feature descriptors")
		return features
	}
	descData := make([]byte, descBytes)
	if _, err := r.ReadAt(descData, int64(descOffset)); err != nil {
		features.Caveats = append(features.Caveats, fmt.Sprintf("raw fallback could not read perf feature descriptors: %v", err))
		return features
	}
	minSectionOffset := descOffset + descBytes
	for i, id := range ids {
		desc := descData[i*16 : i*16+16]
		offset := binary.LittleEndian.Uint64(desc[0:8])
		size := binary.LittleEndian.Uint64(desc[8:16])
		if size == 0 {
			continue
		}
		if offset < minSectionOffset || offset+size < offset || (fileSize > 0 && offset+size > uint64(fileSize)) {
			features.Caveats = append(features.Caveats, fmt.Sprintf("raw fallback skipped invalid perf feature %d section", id))
			continue
		}
		if maxSize := rawPerfFeatureMaxSectionSize(id); size > maxSize {
			features.Caveats = append(features.Caveats, fmt.Sprintf("raw fallback skipped oversized perf feature %d section (%d bytes)", id, size))
			continue
		}
		buf := make([]byte, size)
		if _, err := r.ReadAt(buf, int64(offset)); err != nil {
			features.Caveats = append(features.Caveats, fmt.Sprintf("raw fallback could not read perf feature %d section: %v", id, err))
			continue
		}
		switch id {
		case perfFeatureBuildID:
			features.BuildIDs = readRawPerfBuildIDRecords(buf)
			features.BuildIDCount = len(features.BuildIDs)
		case perfFeatureArch:
			features.Arch = readRawPerfFeatureString(buf)
		case perfFeatureCmdline:
			features.Cmdline = readRawPerfFeatureStringList(buf)
		case perfFeatureMetaInfo:
			features.Meta = readRawPerfMetaInfo(buf)
		case perfFeatureHiperfFilesSymbol:
			symbolFiles, caveats := readRawPerfHiperfSymbolFiles(buf)
			features.SymbolFiles = symbolFiles
			for _, file := range symbolFiles {
				features.SymbolCount += len(file.Symbols)
			}
			features.Caveats = append(features.Caveats, caveats...)
		}
	}
	return features
}

func rawPerfFeatureMaxSectionSize(id int) uint64 {
	if id == perfFeatureHiperfFilesSymbol {
		return 64 << 20
	}
	return 1 << 20
}

func rawPerfFeatureIDs(bits [32]byte) []int {
	var ids []int
	for i, b := range bits {
		for j := 0; j < 8; j++ {
			if b&(1<<j) != 0 {
				ids = append(ids, i*8+j)
			}
		}
	}
	return ids
}

func readRawPerfFeatureString(buf []byte) string {
	if len(buf) < 4 {
		return ""
	}
	n := int(binary.LittleEndian.Uint32(buf[0:4]))
	if n <= 0 || n > len(buf)-4 {
		n = len(buf) - 4
	}
	return cString(buf[4 : 4+n])
}

func readRawPerfFeatureStringList(buf []byte) []string {
	if len(buf) < 4 {
		return nil
	}
	count := int(binary.LittleEndian.Uint32(buf[0:4]))
	if count < 0 || count > 128 {
		return nil
	}
	off := 4
	var out []string
	for i := 0; i < count; i++ {
		if off+4 > len(buf) {
			return out
		}
		n := int(binary.LittleEndian.Uint32(buf[off : off+4]))
		off += 4
		if n <= 0 || off+n > len(buf) {
			return out
		}
		out = append(out, cString(buf[off:off+n]))
		off += n
	}
	return out
}

func readRawPerfMetaInfo(buf []byte) map[string]string {
	out := map[string]string{}
	off := 0
	for off < len(buf) {
		keyEnd := indexByte(buf[off:], 0)
		if keyEnd <= 0 {
			break
		}
		key := string(buf[off : off+keyEnd])
		off += keyEnd + 1
		valueEnd := indexByte(buf[off:], 0)
		if valueEnd <= 0 {
			break
		}
		out[key] = string(buf[off : off+valueEnd])
		off += valueEnd + 1
	}
	return out
}

func readRawPerfBuildIDRecords(buf []byte) map[string]string {
	out := map[string]string{}
	for off := 0; off+8 <= len(buf); {
		size := int(binary.LittleEndian.Uint16(buf[off+6 : off+8]))
		if size <= 8 || off+size > len(buf) {
			break
		}
		record := buf[off : off+size]
		if len(record) >= 36 {
			buildID := hexLower(record[12:32])
			filename := cString(record[36:])
			if filename != "" && buildID != "" {
				out[filename] = buildID
			}
		}
		off += size
	}
	return out
}

type rawPerfFeatureReader struct {
	buf []byte
	off int
}

func (r *rawPerfFeatureReader) readU32() (uint32, bool) {
	if r.off+4 > len(r.buf) {
		return 0, false
	}
	v := binary.LittleEndian.Uint32(r.buf[r.off : r.off+4])
	r.off += 4
	return v, true
}

func (r *rawPerfFeatureReader) readU64() (uint64, bool) {
	if r.off+8 > len(r.buf) {
		return 0, false
	}
	v := binary.LittleEndian.Uint64(r.buf[r.off : r.off+8])
	r.off += 8
	return v, true
}

func (r *rawPerfFeatureReader) readString() (string, bool) {
	n, ok := r.readU32()
	if !ok || n == 0 || n > uint32(len(r.buf)-r.off) {
		return "", false
	}
	raw := r.buf[r.off : r.off+int(n)]
	r.off += int(n)
	if len(raw) == 0 || raw[len(raw)-1] != 0 {
		return "", false
	}
	return cString(raw), true
}

func readRawPerfHiperfSymbolFiles(buf []byte) ([]rawPerfSymbolFile, []string) {
	// OpenHarmony hiperf FEATURE::HIPERF_FILES_SYMBOL stores PerfFileSectionSymbolsFiles.
	reader := rawPerfFeatureReader{buf: buf}
	count, ok := reader.readU32()
	if !ok {
		return nil, []string{"raw fallback could not parse HIPERF_FILES_SYMBOL: missing symbol file count"}
	}
	if count > 4096 {
		return nil, []string{fmt.Sprintf("raw fallback skipped HIPERF_FILES_SYMBOL: too many symbol files (%d)", count)}
	}
	files := make([]rawPerfSymbolFile, 0, count)
	var caveats []string
	totalSymbols := 0
	for i := uint32(0); i < count; i++ {
		path, ok := reader.readString()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed path", i))
			return files, caveats
		}
		symbolType, ok := reader.readU32()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed symbol type", i))
			return files, caveats
		}
		textExecVaddr, ok := reader.readU64()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed text vaddr", i))
			return files, caveats
		}
		textExecVaddrFileOffset, ok := reader.readU64()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed text file offset", i))
			return files, caveats
		}
		buildID, ok := reader.readString()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed build id", i))
			return files, caveats
		}
		symbolCount, ok := reader.readU32()
		if !ok {
			caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL at file %d: malformed symbol count", i))
			return files, caveats
		}
		if symbolCount > 1_000_000 || totalSymbols+int(symbolCount) > 1_000_000 {
			caveats = append(caveats, fmt.Sprintf("raw fallback skipped remaining HIPERF_FILES_SYMBOL entries: too many symbols (%d)", symbolCount))
			return files, caveats
		}
		file := rawPerfSymbolFile{
			Path:                    path,
			SymbolType:              symbolType,
			TextExecVaddr:           textExecVaddr,
			TextExecVaddrFileOffset: textExecVaddrFileOffset,
			BuildID:                 buildID,
			Symbols:                 make([]rawPerfSymbol, 0, symbolCount),
		}
		for j := uint32(0); j < symbolCount; j++ {
			vaddr, ok := reader.readU64()
			if !ok {
				caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL file %d symbol %d: malformed vaddr", i, j))
				files = append(files, file)
				return files, caveats
			}
			length, ok := reader.readU32()
			if !ok {
				caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL file %d symbol %d: malformed length", i, j))
				files = append(files, file)
				return files, caveats
			}
			name, ok := reader.readString()
			if !ok {
				caveats = append(caveats, fmt.Sprintf("raw fallback stopped parsing HIPERF_FILES_SYMBOL file %d symbol %d: malformed name", i, j))
				files = append(files, file)
				return files, caveats
			}
			if name != "" {
				file.Symbols = append(file.Symbols, rawPerfSymbol{Vaddr: vaddr, Len: length, Name: name})
			}
		}
		totalSymbols += len(file.Symbols)
		files = append(files, file)
	}
	return files, caveats
}

func hexLower(buf []byte) string {
	const digits = "0123456789abcdef"
	if len(buf) == 0 {
		return ""
	}
	out := make([]byte, len(buf)*2)
	for i, b := range buf {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0xf]
	}
	return string(out)
}

func rawPerfFeatureCaveats(features rawPerfFeatures) []string {
	if len(features.Present) == 0 && len(features.Caveats) == 0 {
		return nil
	}
	var parts []string
	if len(features.Present) > 0 {
		parts = append(parts, "features="+joinInts(features.Present, ","))
	}
	if features.Arch != "" {
		parts = append(parts, "arch="+features.Arch)
	}
	if len(features.Cmdline) > 0 {
		parts = append(parts, "cmdline="+strings.Join(features.Cmdline, " "))
	}
	if len(features.Meta) > 0 {
		if clock := features.Meta["clockid"]; clock != "" {
			parts = append(parts, "meta.clockid="+clock)
		}
		if _, ok := features.Meta["event_type_info"]; ok {
			parts = append(parts, "meta.event_type_info=present")
		}
	}
	if features.BuildIDCount > 0 {
		parts = append(parts, fmt.Sprintf("build_id_records=%d", features.BuildIDCount))
		parts = append(parts, "build_id_dso_labeling=exact_path")
	}
	if len(features.SymbolFiles) > 0 {
		parts = append(parts, fmt.Sprintf("hiperf_symbol_files=%d", len(features.SymbolFiles)))
		parts = append(parts, fmt.Sprintf("hiperf_symbol_names=%d", features.SymbolCount))
		parts = append(parts, "symbol_source=hiperf_files_symbol")
	}
	if len(parts) == 0 {
		parts = append(parts, "features_present")
	}
	out := []string{"raw fallback parsed perf feature metadata: " + strings.Join(parts, "; ")}
	out = append(out, features.Caveats...)
	return out
}

func joinInts(values []int, sep string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, sep)
}

func indexByte(buf []byte, b byte) int {
	for i, c := range buf {
		if c == b {
			return i
		}
	}
	return -1
}

func rawPerfSampleBitNames(bits uint64) string {
	ordered := []struct {
		bit  uint64
		name string
	}{
		{perfSampleIP, "ip"},
		{perfSampleTID, "tid"},
		{perfSampleTime, "time"},
		{perfSampleAddr, "addr"},
		{perfSampleRead, "read"},
		{perfSampleCallchain, "callchain"},
		{perfSampleID, "id"},
		{perfSampleCPU, "cpu"},
		{perfSamplePeriod, "period"},
		{perfSampleStreamID, "stream_id"},
		{perfSampleRaw, "raw"},
		{perfSampleBranchStack, "branch_stack"},
		{perfSampleRegsUser, "regs_user"},
		{perfSampleStackUser, "stack_user"},
		{perfSampleWeight, "weight"},
		{perfSampleDataSrc, "data_src"},
		{perfSampleIdentifier, "identifier"},
		{perfSampleTransaction, "transaction"},
		{perfSampleRegsIntr, "regs_intr"},
		{perfSamplePhysAddr, "phys_addr"},
		{perfSampleAux, "aux"},
		{perfSampleCGroup, "cgroup"},
		{perfSampleDataPageSize, "data_page_size"},
		{perfSampleCodePageSize, "code_page_size"},
	}
	var names []string
	known := uint64(0)
	for _, item := range ordered {
		if bits&item.bit != 0 {
			names = append(names, item.name)
			known |= item.bit
		}
	}
	if unknown := bits &^ known; unknown != 0 {
		names = append(names, "unknown:0x"+strconv.FormatUint(unknown, 16))
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}

type rawPerfCommRecord struct {
	PID  int
	TID  int
	Name string
}

type rawPerfLifetimeRecord struct {
	PID  int
	PPID int
	TID  int
	PTID int
}

func parseRawPerfComm(payload []byte) (rawPerfCommRecord, bool) {
	if len(payload) < 8 {
		return rawPerfCommRecord{}, false
	}
	pid := int(binary.LittleEndian.Uint32(payload[0:4]))
	tid := int(binary.LittleEndian.Uint32(payload[4:8]))
	name := cString(payload[8:])
	if name == "" {
		return rawPerfCommRecord{}, false
	}
	return rawPerfCommRecord{PID: pid, TID: tid, Name: name}, true
}

func parseRawPerfLifetime(payload []byte) (rawPerfLifetimeRecord, bool) {
	if len(payload) < 16 {
		return rawPerfLifetimeRecord{}, false
	}
	return rawPerfLifetimeRecord{
		PID:  int(binary.LittleEndian.Uint32(payload[0:4])),
		PPID: int(binary.LittleEndian.Uint32(payload[4:8])),
		TID:  int(binary.LittleEndian.Uint32(payload[8:12])),
		PTID: int(binary.LittleEndian.Uint32(payload[12:16])),
	}, true
}

func parseRawPerfMmap(payload []byte) (rawPerfMapping, bool) {
	if len(payload) < 32 {
		return rawPerfMapping{}, false
	}
	mapping := rawPerfMapping{
		PID:   int(binary.LittleEndian.Uint32(payload[0:4])),
		TID:   int(binary.LittleEndian.Uint32(payload[4:8])),
		Addr:  binary.LittleEndian.Uint64(payload[8:16]),
		Len:   binary.LittleEndian.Uint64(payload[16:24]),
		Pgoff: binary.LittleEndian.Uint64(payload[24:32]),
		Path:  cString(payload[32:]),
	}
	return mapping, mapping.Len > 0 && strings.TrimSpace(mapping.Path) != ""
}

func parseRawPerfMmap2(payload []byte) (rawPerfMapping, bool) {
	if len(payload) < 64 {
		return rawPerfMapping{}, false
	}
	mapping := rawPerfMapping{
		PID:   int(binary.LittleEndian.Uint32(payload[0:4])),
		TID:   int(binary.LittleEndian.Uint32(payload[4:8])),
		Addr:  binary.LittleEndian.Uint64(payload[8:16]),
		Len:   binary.LittleEndian.Uint64(payload[16:24]),
		Pgoff: binary.LittleEndian.Uint64(payload[24:32]),
		Path:  cString(payload[64:]),
	}
	return mapping, mapping.Len > 0 && strings.TrimSpace(mapping.Path) != ""
}

func parseRawPerfSample(payload []byte, attr rawPerfAttr) (rawPerfSample, bool) {
	var sample rawPerfSample
	sampleType := attr.SampleType
	off := 0
	readU64 := func() (uint64, bool) {
		if off+8 > len(payload) {
			return 0, false
		}
		v := binary.LittleEndian.Uint64(payload[off : off+8])
		off += 8
		return v, true
	}
	readU32Pair := func() (uint32, uint32, bool) {
		if off+8 > len(payload) {
			return 0, 0, false
		}
		a := binary.LittleEndian.Uint32(payload[off : off+4])
		b := binary.LittleEndian.Uint32(payload[off+4 : off+8])
		off += 8
		return a, b, true
	}
	skipBytes := func(n int) bool {
		if n < 0 || off+n > len(payload) {
			return false
		}
		off += n
		return true
	}
	skipU64 := func() bool {
		_, ok := readU64()
		return ok
	}
	if sampleType&perfSampleIdentifier != 0 {
		v, ok := readU64()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.ID = v
	}
	if sampleType&perfSampleIP != 0 {
		v, ok := readU64()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.IP = v
	}
	if sampleType&perfSampleTID != 0 {
		pid, tid, ok := readU32Pair()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.PID = int(pid)
		sample.TID = int(tid)
	}
	if sampleType&perfSampleTime != 0 {
		v, ok := readU64()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.TimeNS = v
	}
	if sampleType&perfSampleAddr != 0 {
		if _, ok := readU64(); !ok {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleID != 0 {
		v, ok := readU64()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.ID = v
	}
	if sampleType&perfSampleStreamID != 0 {
		if _, ok := readU64(); !ok {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleCPU != 0 {
		cpu, _, ok := readU32Pair()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.CPU = int(cpu)
		sample.CPUValid = true
	}
	if sampleType&perfSamplePeriod != 0 {
		v, ok := readU64()
		if !ok {
			return rawPerfSample{}, false
		}
		sample.Period = v
	}
	if sampleType&perfSampleRead != 0 {
		if !skipRawPerfRead(&off, payload, attr.ReadFormat) {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleCallchain != 0 {
		nr, ok := readU64()
		if !ok || nr > 4096 {
			return rawPerfSample{}, false
		}
		for i := uint64(0); i < nr; i++ {
			v, ok := readU64()
			if !ok {
				return rawPerfSample{}, false
			}
			sample.Callchain = append(sample.Callchain, v)
		}
	}
	if sampleType&perfSampleRaw != 0 {
		if off+4 > len(payload) {
			return rawPerfSample{}, false
		}
		size := int(binary.LittleEndian.Uint32(payload[off : off+4]))
		off += 4
		if !skipBytes(size) {
			return rawPerfSample{}, false
		}
		if rem := off % 8; rem != 0 && !skipBytes(8-rem) {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleBranchStack != 0 {
		nr, ok := readU64()
		if !ok || nr > 4096 {
			return rawPerfSample{}, false
		}
		if !skipBytes(int(nr) * 24) {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleRegsUser != 0 {
		if !skipRawPerfSampleRegs(&off, payload, attr.SampleRegsUser) {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleStackUser != 0 {
		if !skipRawPerfSampleStackUser(&off, payload) {
			return rawPerfSample{}, false
		}
	}
	for _, bit := range []uint64{
		perfSampleWeight,
		perfSampleDataSrc,
		perfSampleTransaction,
	} {
		if sampleType&bit != 0 && !skipU64() {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSampleRegsIntr != 0 {
		if !skipRawPerfSampleRegs(&off, payload, attr.SampleRegsIntr) {
			return rawPerfSample{}, false
		}
	}
	if sampleType&perfSamplePhysAddr != 0 && !skipU64() {
		return rawPerfSample{}, false
	}
	if sampleType&perfSampleAux != 0 {
		size, ok := readU64()
		if !ok || size > uint64(len(payload)-off) || !skipBytes(int(size)) {
			return rawPerfSample{}, false
		}
	}
	for _, bit := range []uint64{
		perfSampleCGroup,
		perfSampleDataPageSize,
		perfSampleCodePageSize,
	} {
		if sampleType&bit != 0 && !skipU64() {
			return rawPerfSample{}, false
		}
	}
	if sample.Period == 0 {
		sample.Period = 1
	}
	return sample, true
}

func skipRawPerfSampleRegs(off *int, payload []byte, regsMask uint64) bool {
	if *off+8 > len(payload) {
		return false
	}
	abi := binary.LittleEndian.Uint64(payload[*off : *off+8])
	*off += 8
	if abi == 0 {
		return true
	}
	bytesToSkip := bits.OnesCount64(regsMask) * 8
	if bytesToSkip < 0 || *off+bytesToSkip > len(payload) {
		return false
	}
	*off += bytesToSkip
	return true
}

func skipRawPerfSampleStackUser(off *int, payload []byte) bool {
	if *off+8 > len(payload) {
		return false
	}
	size := binary.LittleEndian.Uint64(payload[*off : *off+8])
	*off += 8
	if size > uint64(len(payload)-*off) {
		return false
	}
	*off += int(size)
	if size == 0 {
		return true
	}
	if *off+8 > len(payload) {
		return false
	}
	*off += 8
	return true
}

func skipRawPerfRead(off *int, payload []byte, readFormat uint64) bool {
	readU64 := func() bool {
		if *off+8 > len(payload) {
			return false
		}
		*off += 8
		return true
	}
	if readFormat&perfFormatGroup != 0 {
		if *off+8 > len(payload) {
			return false
		}
		nr := binary.LittleEndian.Uint64(payload[*off : *off+8])
		*off += 8
		if nr > 4096 {
			return false
		}
		if readFormat&perfFormatTotalTimeEnabled != 0 && !readU64() {
			return false
		}
		if readFormat&perfFormatTotalTimeRunning != 0 && !readU64() {
			return false
		}
		for i := uint64(0); i < nr; i++ {
			if !readU64() {
				return false
			}
			if readFormat&perfFormatID != 0 && !readU64() {
				return false
			}
			if readFormat&perfFormatLost != 0 && !readU64() {
				return false
			}
		}
		return true
	}
	if !readU64() {
		return false
	}
	if readFormat&perfFormatTotalTimeEnabled != 0 && !readU64() {
		return false
	}
	if readFormat&perfFormatTotalTimeRunning != 0 && !readU64() {
		return false
	}
	if readFormat&perfFormatID != 0 && !readU64() {
		return false
	}
	if readFormat&perfFormatLost != 0 && !readU64() {
		return false
	}
	return true
}

func writeRawPerfDataPerfTrace(ctx context.Context, w io.Writer, data rawPerfData) error {
	if _, err := io.WriteString(w, systraceHeader); err != nil {
		return err
	}
	for _, sample := range data.Samples {
		if err := ctx.Err(); err != nil {
			return err
		}
		tid := sample.TID
		if tid <= 0 {
			tid = sample.PID
		}
		if tid <= 0 {
			tid = 1
		}
		pid := sample.PID
		if pid <= 0 {
			pid = tid
		}
		comm := sanitizePerfTraceComm(firstNonEmpty(sample.Comm, data.Threads[tid], fmt.Sprintf("tid%d", tid)))
		cpu := -1
		if sample.CPUValid {
			cpu = sample.CPU
		}
		frame := rawPerfResolveFrame(&data, sample, sample.IP)
		callchain := rawPerfCallchain(&data, sample)
		ts := float64(sample.TimeNS) / 1e9
		cpuKnown := "false"
		if sample.CPUValid {
			cpuKnown = "true"
		}
		parserCaveats := ""
		if len(data.Caveats) > 0 {
			parserCaveats = " parser_caveats=" + quoteTraceValue(strings.Join(data.Caveats, "; "))
		}
		eventName := firstNonEmpty(sample.EventName, data.EventName, "unknown")
		symbolizationStatus := perfTraceSymbolizationStatus(frame.Symbol, frame.DSO, "raw_perfdata_fallback")
		callchainStatus := perfTraceCallchainStatus(callchain, "raw_perfdata_fallback")
		if _, err := fmt.Fprintf(w, "%16s-%-6d (%5d) [%03d] .... %12.6f: perf_sample: cpu=%d cpu_known=%s pid=%d tid=%d thread_comm=%s sample_weight=%d event=%s symbol=%s dso=%s ip=%s callchain=%s source=raw_perfdata_fallback symbolization_status=%s clock=perf_data clock_confidence=assumed callchain_status=%s%s\n",
			comm, tid, pid, rawPerfHeaderCPU(cpu), ts, cpu, cpuKnown, pid, tid, quoteTraceValue(comm), sample.Period, quoteTraceValue(eventName), quoteTraceValue(frame.Symbol), quoteTraceValue(frame.DSO), quoteTraceValue(frame.IP), quoteTraceValue(callchain), symbolizationStatus, callchainStatus, parserCaveats); err != nil {
			return err
		}
	}
	return nil
}

func rawPerfHeaderCPU(cpu int) int {
	if cpu < 0 {
		return 0
	}
	return cpu
}

func rawPerfIP(ip uint64) string {
	if ip == 0 {
		return "unknown"
	}
	return "0x" + strconv.FormatUint(ip, 16)
}

func rawPerfCallchain(data *rawPerfData, sample rawPerfSample) string {
	parts := make([]string, 0, len(sample.Callchain)+1)
	for i := len(sample.Callchain) - 1; i >= 0; i-- {
		if sample.Callchain[i] != 0 {
			parts = append(parts, rawPerfFrameCallchainLabel(rawPerfResolveFrame(data, sample, sample.Callchain[i])))
		}
	}
	if sample.IP != 0 {
		parts = append(parts, rawPerfFrameCallchainLabel(rawPerfResolveFrame(data, sample, sample.IP)))
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ";")
}

func rawPerfFrameCallchainLabel(frame rawPerfResolvedFrame) string {
	if frame.Symbolized && frame.Symbol != "" && frame.Symbol != "unknown" {
		if frame.DSO != "" && frame.DSO != "unknown" {
			return frame.Symbol + "@" + frame.DSO
		}
		return frame.Symbol
	}
	return frame.IP
}

func rawPerfResolveFrame(data *rawPerfData, sample rawPerfSample, ip uint64) rawPerfResolvedFrame {
	frame := rawPerfResolvedFrame{
		IP:     rawPerfIP(ip),
		DSO:    "unknown",
		Symbol: rawPerfIP(ip),
	}
	mapping, ok := rawPerfBestMapping(data.Mappings, sample, ip)
	if !ok {
		return frame
	}
	frame.DSO = rawPerfDSOForMapping(mapping, data.Features)
	if symbol, ok := rawPerfSymbolForIP(data.Features.SymbolFiles, mapping, ip); ok {
		frame.Symbol = symbol.Name
		frame.Symbolized = true
	}
	return frame
}

func rawPerfBestMapping(mappings []rawPerfMapping, sample rawPerfSample, ip uint64) (rawPerfMapping, bool) {
	var best rawPerfMapping
	for _, mapping := range mappings {
		if mapping.Len == 0 || ip < mapping.Addr || ip >= mapping.Addr+mapping.Len {
			continue
		}
		if mapping.PID > 0 && sample.PID > 0 && mapping.PID != sample.PID {
			continue
		}
		if best.Len == 0 || mapping.Len < best.Len {
			best = mapping
		}
	}
	return best, best.Path != ""
}

func rawPerfDSO(mappings []rawPerfMapping, sample rawPerfSample, features rawPerfFeatures) string {
	best, ok := rawPerfBestMapping(mappings, sample, sample.IP)
	if !ok {
		return "unknown"
	}
	return rawPerfDSOForMapping(best, features)
}

func rawPerfDSOForMapping(mapping rawPerfMapping, features rawPerfFeatures) string {
	if mapping.Path == "" {
		return "unknown"
	}
	if buildID := features.BuildIDs[mapping.Path]; buildID != "" {
		return mapping.Path + "#build_id=" + buildID
	}
	return mapping.Path
}

func rawPerfSymbolForIP(files []rawPerfSymbolFile, mapping rawPerfMapping, ip uint64) (rawPerfSymbol, bool) {
	for _, file := range files {
		if !rawPerfSymbolFileMatchesMapping(file, mapping) {
			continue
		}
		if rawPerfSymbolFileUsesDirectPC(file) {
			if symbol, ok := rawPerfFindSymbol(file.Symbols, ip); ok {
				return symbol, true
			}
			continue
		}
		if mapping.Len == 0 || ip < mapping.Addr || ip >= mapping.Addr+mapping.Len {
			continue
		}
		sectionOffset := ip - mapping.Addr + mapping.Pgoff
		if sectionOffset < file.TextExecVaddrFileOffset {
			continue
		}
		vaddr := sectionOffset - file.TextExecVaddrFileOffset + file.TextExecVaddr
		if symbol, ok := rawPerfFindSymbol(file.Symbols, vaddr); ok {
			return symbol, true
		}
	}
	return rawPerfSymbol{}, false
}

func rawPerfSymbolFileMatchesMapping(file rawPerfSymbolFile, mapping rawPerfMapping) bool {
	filePath := strings.TrimSpace(file.Path)
	mappingPath := strings.TrimSpace(mapping.Path)
	if filePath == "" || mappingPath == "" {
		return false
	}
	if filePath == mappingPath {
		return true
	}
	return strings.HasSuffix(mappingPath, "/"+filePath) || strings.HasSuffix(filePath, "/"+mappingPath)
}

func rawPerfSymbolFileUsesDirectPC(file rawPerfSymbolFile) bool {
	path := strings.TrimSpace(file.Path)
	switch file.SymbolType {
	case rawPerfSymbolFileHAP, rawPerfSymbolFileV8:
		return true
	}
	return strings.HasSuffix(path, ".hap") ||
		strings.HasSuffix(path, ".hsp") ||
		strings.HasSuffix(path, ".abc") ||
		strings.HasSuffix(path, ".hqf") ||
		strings.HasPrefix(path, "[anon:ArkTS Code") ||
		strings.HasPrefix(path, "[anon:JSVM_JIT") ||
		strings.HasPrefix(path, "[anon:ARKWEB_JIT") ||
		strings.HasPrefix(path, "[anon:v8")
}

func rawPerfFindSymbol(symbols []rawPerfSymbol, vaddr uint64) (rawPerfSymbol, bool) {
	var best rawPerfSymbol
	for _, symbol := range symbols {
		if symbol.Name == "" || vaddr < symbol.Vaddr {
			continue
		}
		if symbol.Len > 0 {
			end := symbol.Vaddr + uint64(symbol.Len)
			if end < symbol.Vaddr || vaddr >= end {
				continue
			}
		} else if vaddr != symbol.Vaddr {
			continue
		}
		if best.Name == "" || symbol.Vaddr >= best.Vaddr {
			best = symbol
		}
	}
	return best, best.Name != ""
}
