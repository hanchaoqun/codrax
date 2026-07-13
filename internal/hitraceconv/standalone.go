package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	profilerTraceHeaderSize       = 1024
	profilerTraceHeaderMagic      = uint64(0x464F5250534F484F)
	profilerDataTypeHiperf        = uint32(1)
	profilerPluginNameOffset      = 108
	profilerPluginNameSize        = 128
	profilerPluginVersionOffset   = 236
	profilerPluginVersionSize     = 8
	profilerStandalonePayloadBase = profilerTraceHeaderSize
)

var profilerTraceHeaderMagicLE = []byte{
	byte(profilerTraceHeaderMagic & 0xff),
	byte((profilerTraceHeaderMagic >> 8) & 0xff),
	byte((profilerTraceHeaderMagic >> 16) & 0xff),
	byte((profilerTraceHeaderMagic >> 24) & 0xff),
	byte((profilerTraceHeaderMagic >> 32) & 0xff),
	byte((profilerTraceHeaderMagic >> 40) & 0xff),
	byte((profilerTraceHeaderMagic >> 48) & 0xff),
	byte((profilerTraceHeaderMagic >> 56) & 0xff),
}

type standaloneSegment struct {
	Offset        int64
	Length        int64
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type traceBundleMetadata struct {
	Version             string                  `json:"version"`
	InputPath           string                  `json:"input_path"`
	Systrace            string                  `json:"systrace,omitempty"`
	Artifacts           []Artifact              `json:"artifacts,omitempty"`
	ProviderDecisions   []PerfProviderDecision  `json:"provider_decisions,omitempty"`
	TraceDecisions      []TraceProviderDecision `json:"trace_provider_decisions,omitempty"`
	TraceDBCoverage     []TraceDBCoverage       `json:"trace_db_coverage,omitempty"`
	TraceCoverage       []TraceDBCoverage       `json:"trace_coverage,omitempty"`
	TraceToolGates      []TraceToolGateStatus   `json:"trace_tool_gates,omitempty"`
	PerfClockAlignments []PerfClockAlignment    `json:"perf_clock_alignments,omitempty"`
	Caveats             []string                `json:"caveats,omitempty"`
}

type standaloneExtractOptions struct {
	GeneratePerfTrace bool
	PrimaryPerfSource string
}

func extractStandaloneArtifacts(ctx context.Context, opts Options, inputSize int64, outputPath string) ([]Artifact, []string, []PerfProviderDecision, error) {
	return extractStandaloneArtifactsWithOptions(ctx, opts, inputSize, outputPath, standaloneExtractOptions{GeneratePerfTrace: true})
}

func extractStandaloneArtifactsWithOptions(ctx context.Context, opts Options, inputSize int64, outputPath string, extractOpts standaloneExtractOptions) ([]Artifact, []string, []PerfProviderDecision, error) {
	ledger, err := newConversionFileLedger(opts.InputPath)
	if err != nil {
		return nil, nil, nil, err
	}
	artifacts, caveats, decisions, err := extractStandaloneArtifactsWithOptionsAndLedger(ctx, opts, inputSize, outputPath, extractOpts, ledger)
	if err != nil {
		err = joinConversionCleanupError(err, ledger)
	}
	return artifacts, caveats, decisions, err
}

func extractStandaloneArtifactsWithOptionsAndLedger(ctx context.Context, opts Options, inputSize int64, outputPath string, extractOpts standaloneExtractOptions, ledger *conversionFileLedger) ([]Artifact, []string, []PerfProviderDecision, error) {
	input := strings.TrimSpace(opts.InputPath)
	segments, err := findStandaloneSegments(ctx, input, inputSize)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(segments) == 0 {
		return nil, nil, nil, nil
	}
	if !extractOpts.GeneratePerfTrace {
		perfDataCount := 0
		for _, seg := range segments {
			if seg.DataType == profilerDataTypeHiperf {
				perfDataCount++
			}
		}
		if perfDataCount > 0 {
			primary := strings.TrimSpace(extractOpts.PrimaryPerfSource)
			if primary == "" {
				primary = "an existing query-ready perf source"
			}
			return nil, []string{fmt.Sprintf("detected %d HIPERF_DATA standalone perf.data segment(s); raw perf.data sidecar extraction and .perftrace fallback generation were skipped because %s is the primary trace_query CPU-sample source", perfDataCount, primary)}, nil, nil
		}
		return nil, nil, nil, nil
	}
	base := traceSidecarBase(input, outputPath)
	in, err := os.Open(input)
	if err != nil {
		return nil, nil, nil, err
	}
	defer in.Close()
	var artifacts []Artifact
	var caveats []string
	var decisions []PerfProviderDecision
	perfTraceProviders := map[string]int{}
	perfOrdinal := 0
	for _, seg := range segments {
		if seg.DataType != profilerDataTypeHiperf {
			continue
		}
		perfOrdinal++
		outPath := numberedSidecarPath(base, perfOrdinal, ".perf.data")
		if err := ensureOutputDoesNotExist(outPath); err != nil {
			return artifacts, caveats, decisions, err
		}
		n, err := copyRangeToFileWithLedger(in, seg.Offset+profilerStandalonePayloadBase, seg.Length-profilerStandalonePayloadBase, outPath, ledger)
		if err != nil {
			return artifacts, caveats, decisions, err
		}
		rawArtifact := Artifact{
			Type:          ArtifactPerfData,
			Path:          outPath,
			Bytes:         n,
			DataType:      seg.DataType,
			PluginName:    seg.PluginName,
			PluginVersion: seg.PluginVersion,
			SourceOffset:  seg.Offset,
			SourceBytes:   seg.Length,
			Converter:     converterVersion,
			Perf:          perfCapabilityForRawPerfDataArtifact(detectPerfInputFormat(outPath)),
		}
		perfTracePath := numberedSidecarPath(base, perfOrdinal, ".perftrace")
		perfTrace, caveat, providerDecisions, err := maybeConvertHiperfPerfData(ctx, opts, outPath, perfTracePath, ledger)
		decisions = append(decisions, providerDecisions...)
		if err != nil {
			return artifacts, caveats, decisions, err
		}
		if perfTrace.Path == "" {
			rawArtifact.Caveats = append(rawArtifact.Caveats, "raw perf.data sidecar extracted; run an official hiperf/simpleperf adapter to produce .perftrace before trace_query can aggregate CPU samples")
			if caveat != "" {
				caveats = append(caveats, caveat)
			}
			artifacts = append(artifacts, rawArtifact)
			continue
		}
		rawArtifact.Caveats = append(rawArtifact.Caveats, "raw perf.data sidecar preserved; normalized .perftrace was generated for trace_query CPU-sample aggregation")
		artifacts = append(artifacts, rawArtifact, perfTrace)
		perfTraceProviders[perfTraceProviderSummaryLabel(perfTrace)]++
	}
	if len(artifacts) > 0 {
		perfDataCount := 0
		perfTraceCount := 0
		for _, artifact := range artifacts {
			switch artifact.Type {
			case ArtifactPerfData:
				perfDataCount++
			case ArtifactPerfTrace:
				perfTraceCount++
			}
		}
		if perfTraceCount > 0 {
			caveats = append(caveats, standalonePerfTraceSummaryCaveat(perfDataCount, perfTraceCount, perfTraceProviders))
		} else {
			caveats = append(caveats, fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s); raw perf.data is preserved as sidecar and still needs official parser conversion to .perftrace for trace_query sample aggregation", perfDataCount))
		}
	}
	return artifacts, caveats, decisions, nil
}

func inputContainsStandalonePerfSidecar(ctx context.Context, input string, inputSize int64) (bool, error) {
	segments, err := findStandaloneSegments(ctx, input, inputSize)
	if err != nil {
		return false, err
	}
	for _, seg := range segments {
		if seg.DataType == profilerDataTypeHiperf {
			return true, nil
		}
	}
	return false, nil
}

func perfTraceProviderSummaryLabel(artifact Artifact) string {
	if artifact.Perf != nil {
		switch artifact.Perf.ProviderKind {
		case perfProviderKindRawFallback:
			return "raw"
		case perfProviderKindOfficialHarmony, perfProviderKindOfficialAndroid:
			return "official"
		}
	}
	if strings.Contains(strings.ToLower(artifact.Converter), "raw-perfdata") {
		return "raw"
	}
	return "official"
}

func standalonePerfTraceSummaryCaveat(perfDataCount, perfTraceCount int, providers map[string]int) string {
	rawCount := providers["raw"]
	officialCount := providers["official"]
	switch {
	case rawCount > 0 && officialCount == 0:
		return fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s) and generated %d normalized .perftrace artifact(s) through Codrax raw perf.data fallback", perfDataCount, perfTraceCount)
	case officialCount > 0 && rawCount == 0:
		return fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s) and generated %d normalized .perftrace artifact(s) through the official perf adapter", perfDataCount, perfTraceCount)
	case rawCount > 0 && officialCount > 0:
		return fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s) and generated %d normalized .perftrace artifact(s) through mixed providers (official=%d raw=%d)", perfDataCount, perfTraceCount, officialCount, rawCount)
	default:
		return fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s) and generated %d normalized .perftrace artifact(s)", perfDataCount, perfTraceCount)
	}
}

func findStandaloneSegments(ctx context.Context, path string, size int64) ([]standaloneSegment, error) {
	if size < profilerTraceHeaderSize {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const chunkSize = 1024 * 1024
	overlap := len(profilerTraceHeaderMagicLE) - 1
	buf := make([]byte, chunkSize+overlap)
	var segments []standaloneSegment
	seen := map[int64]bool{}
	var base int64
	var carry []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		copy(buf, carry)
		n, readErr := f.Read(buf[len(carry):chunkSize])
		window := buf[:len(carry)+n]
		search := window
		searchBase := base - int64(len(carry))
		for {
			idx := bytes.Index(search, profilerTraceHeaderMagicLE)
			if idx < 0 {
				break
			}
			candidate := searchBase + int64(idx)
			if candidate >= 0 && !seen[candidate] {
				seen[candidate] = true
				if seg, ok := readStandaloneSegmentAt(f, candidate, size); ok {
					segments = append(segments, seg)
				}
			}
			search = search[idx+1:]
			searchBase = candidate + 1
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if len(window) > overlap {
			carry = append(carry[:0], window[len(window)-overlap:]...)
		} else {
			carry = append(carry[:0], window...)
		}
		base += int64(n)
	}
	return segments, nil
}

func isProfilerTraceHeaderPrefix(first, second uint32) bool {
	return first == uint32(profilerTraceHeaderMagic&0xffffffff) && second == uint32((profilerTraceHeaderMagic>>32)&0xffffffff)
}

func readStandaloneSegmentAt(f *os.File, off int64, fileSize int64) (standaloneSegment, bool) {
	if off < 0 || off+profilerTraceHeaderSize > fileSize {
		return standaloneSegment{}, false
	}
	header := make([]byte, profilerTraceHeaderSize)
	if _, err := f.ReadAt(header, off); err != nil {
		return standaloneSegment{}, false
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return standaloneSegment{}, false
	}
	length := int64(binary.LittleEndian.Uint64(header[8:16]))
	if length < profilerTraceHeaderSize || off+length > fileSize {
		return standaloneSegment{}, false
	}
	dataType := binary.LittleEndian.Uint32(header[56:60])
	return standaloneSegment{
		Offset:        off,
		Length:        length,
		DataType:      dataType,
		PluginName:    cString(header[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize]),
		PluginVersion: cString(header[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize]),
	}, true
}

func writeTraceBundle(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision) (Artifact, error) {
	return writeTraceBundleWithLedger(input, outputPath, artifacts, caveats, decisions, traceDecisions, nil)
}

func writeTraceBundleWithLedger(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, ledger *conversionFileLedger) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(input, outputPath, artifacts, caveats, decisions, traceDecisions, nil, nil, traceToolGatesForBundle(), ledger)
}

func writeTraceBundleWithCoverage(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, coverage []TraceDBCoverage) (Artifact, error) {
	return writeTraceBundleWithAllCoverage(input, outputPath, artifacts, caveats, decisions, traceDecisions, coverage, nil)
}

func writeTraceBundleWithAllCoverage(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndLedger(input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, nil)
}

func writeTraceBundleWithAllCoverageAndLedger(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, ledger *conversionFileLedger) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, traceToolGatesForBundle(), ledger)
}

func writeTraceBundleWithAllCoverageAndGates(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, traceToolGates []TraceToolGateStatus) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, traceToolGates, nil)
}

func writeTraceBundleWithAllCoverageAndGatesAndLedger(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, traceToolGates []TraceToolGateStatus, ledger *conversionFileLedger) (artifact Artifact, err error) {
	artifacts = dedupeArtifacts(artifacts)
	caveats = dedupeStrings(caveats)
	if len(artifacts) == 0 {
		if len(decisions) == 0 && len(traceDecisions) == 0 && len(dbCoverage) == 0 && len(traceCoverage) == 0 && len(caveats) == 0 {
			return Artifact{}, nil
		}
	}
	base := traceSidecarBase(input, outputPath)
	path := base + ".tracebundle.json"
	meta := traceBundleMetadata{
		Version:             converterVersion,
		InputPath:           input,
		Systrace:            traceBundleSystracePath(outputPath, artifacts),
		Artifacts:           artifacts,
		ProviderDecisions:   decisions,
		TraceDecisions:      traceDecisions,
		TraceDBCoverage:     dbCoverage,
		TraceCoverage:       traceCoverage,
		TraceToolGates:      traceToolGates,
		PerfClockAlignments: perfClockAlignmentsForArtifacts(artifacts),
		Caveats:             caveats,
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	body = append(body, '\n')
	ownedLedger := ledger == nil
	if ownedLedger {
		ledger, err = newConversionFileLedger(input)
		if err != nil {
			return Artifact{}, err
		}
	}
	committed := false
	defer func() {
		if ownedLedger && !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return Artifact{}, fmt.Errorf("output file already exists: %s (delete it first or specify a different output path)", path)
		}
		return Artifact{}, err
	}
	if err := ledger.recordOpenFile(path, out); err != nil {
		return Artifact{}, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(path, out))
	}
	written, writeErr := out.Write(body)
	if writeErr == nil && written != len(body) {
		writeErr = io.ErrShortWrite
	}
	closeErr := out.Close()
	if writeErr != nil {
		return Artifact{}, traceDBJoinPreservingSingle(writeErr, closeErr)
	}
	if closeErr != nil {
		return Artifact{}, closeErr
	}
	info, statErr := os.Lstat(path)
	if statErr != nil {
		return Artifact{}, statErr
	}
	if !info.Mode().IsRegular() {
		return Artifact{}, fmt.Errorf("tracebundle publication is not a regular file: %s", path)
	}
	if !ledger.ownsPathIdentity(path, info) {
		return Artifact{}, fmt.Errorf("tracebundle path changed identity during publication: %s", path)
	}
	if info.Size() != int64(len(body)) {
		return Artifact{}, fmt.Errorf("tracebundle publication size mismatch: path=%s got=%d want=%d", path, info.Size(), len(body))
	}
	if err := ledger.sealOwnedPath(path, info.Size()); err != nil {
		return Artifact{}, err
	}
	committed = true
	return Artifact{Type: ArtifactTraceBundle, Path: path, Bytes: info.Size(), Converter: converterVersion}, nil
}

func traceToolGatesForBundle() []TraceToolGateStatus {
	gate := buildSysBinaryParityGateStatus()
	if strings.TrimSpace(gate.Name) == "" {
		return nil
	}
	return []TraceToolGateStatus{gate}
}

func traceBundleSystracePath(outputPath string, artifacts []Artifact) string {
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactSystrace && strings.TrimSpace(artifact.Path) != "" {
			return artifact.Path
		}
	}
	return ""
}

func perfClockAlignmentsForArtifacts(artifacts []Artifact) []PerfClockAlignment {
	var out []PerfClockAlignment
	hasSystrace := false
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactSystrace && strings.TrimSpace(artifact.Path) != "" {
			hasSystrace = true
			break
		}
	}
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfTrace || artifact.Perf == nil {
			continue
		}
		confidence := firstNonEmpty(artifact.Perf.TimeAlignment, "unknown")
		item := PerfClockAlignment{
			ArtifactPath:    artifact.Path,
			PerfTimeDomain:  artifact.Perf.TimeDomain,
			TraceTimeDomain: "trace_seconds",
			Confidence:      confidence,
			Calibrated:      strings.EqualFold(confidence, "calibrated"),
			Source:          firstNonEmpty(artifact.Perf.ProviderName, artifact.Converter),
		}
		if !hasSystrace {
			item.TraceTimeDomain = "missing_trace_body"
			item.Confidence = "trace_body_missing"
			item.Calibrated = false
			item.Caveats = append(item.Caveats, "no systrace trace body is available in this tracebundle; trace_query can aggregate perf samples, but cannot correlate them to trace windows until a systrace artifact is attached or generated")
		} else if !item.Calibrated {
			item.Caveats = append(item.Caveats, "no capture-level trace/perf clock map is available; trace_query treats timestamp overlap as supporting evidence unless calibrated")
		}
		out = append(out, item)
	}
	return out
}

func traceSidecarBase(input, outputPath string) string {
	base := strings.TrimSpace(outputPath)
	if base == "" {
		base = input
	}
	if strings.HasSuffix(base, defaultOutputSuffix) {
		base = strings.TrimSuffix(base, defaultOutputSuffix)
	}
	return base
}

func numberedSidecarPath(base string, ordinal int, suffix string) string {
	if ordinal <= 1 {
		return base + suffix
	}
	ext := filepath.Ext(suffix)
	stem := strings.TrimSuffix(suffix, ext)
	if ext == "" {
		return fmt.Sprintf("%s%s_%d", base, suffix, ordinal)
	}
	return fmt.Sprintf("%s%s_%d%s", base, stem, ordinal, ext)
}

func ensureOutputDoesNotExist(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("output file already exists: %s (delete it first or specify a different output path)", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check output path %s: %w", path, err)
	}
	return nil
}

func copyRangeToFile(in *os.File, off, length int64, outPath string) (int64, error) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		return 0, err
	}
	written, err := copyRangeToFileWithLedger(in, off, length, outPath, ledger)
	if err != nil {
		err = joinConversionCleanupError(err, ledger)
	}
	return written, err
}

func copyRangeToFileWithLedger(in *os.File, off, length int64, outPath string, ledger *conversionFileLedger) (int64, error) {
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	if err := ledger.recordOpenFile(outPath, out); err != nil {
		return 0, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(outPath, out))
	}
	written, copyErr := io.Copy(out, io.NewSectionReader(in, off, length))
	if copyErr == nil && written != length {
		copyErr = io.ErrUnexpectedEOF
	}
	closeErr := out.Close()
	if copyErr != nil {
		return written, traceDBJoinPreservingSingle(copyErr, closeErr, ledger.removeOwnedPath(outPath))
	}
	if closeErr != nil {
		return written, traceDBJoinPreservingSingle(closeErr, ledger.removeOwnedPath(outPath))
	}
	info, err := os.Lstat(outPath)
	if err != nil {
		return written, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(outPath))
	}
	if !info.Mode().IsRegular() || !ledger.ownsPathIdentity(outPath, info) || info.Size() != length {
		err := fmt.Errorf("standalone sidecar publication failed identity/size validation: path=%s got=%d want=%d", outPath, info.Size(), length)
		return written, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(outPath))
	}
	if err := ledger.sealOwnedPath(outPath, info.Size()); err != nil {
		return written, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(outPath))
	}
	return written, nil
}

func cString(data []byte) string {
	if idx := bytes.IndexByte(data, 0); idx >= 0 {
		data = data[:idx]
	}
	return strings.TrimSpace(string(data))
}
