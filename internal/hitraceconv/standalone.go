package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const (
	profilerTraceHeaderSize       = 1024
	profilerTraceHeaderMagic      = uint64(0x464F5250534F484F)
	profilerDataTypeHiperf        = uint32(1)
	profilerDataTypeStandalone    = uint32(1000)
	profilerPluginNameOffset      = 108
	profilerPluginNameSize        = 128
	profilerPluginVersionOffset   = 236
	profilerPluginVersionSize     = 8
	profilerStandalonePayloadBase = profilerTraceHeaderSize
)

func isProfilerStandaloneDataType(dataType uint32) bool {
	return dataType == profilerDataTypeHiperf || dataType == profilerDataTypeStandalone
}

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

type standaloneSegmentInventory struct {
	inputSize int64
	segments  []standaloneSegment
	input     conversionInputView
}

func (inventory standaloneSegmentInventory) hasHiperfData() bool {
	for _, segment := range inventory.segments {
		if segment.DataType == profilerDataTypeHiperf {
			return true
		}
	}
	return false
}

type traceBundleMetadata struct {
	Schema              string                  `json:"schema"`
	CaptureID           string                  `json:"capture_id"`
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

func extractStandaloneArtifactsWithOptionsAndLedger(
	ctx context.Context,
	opts Options,
	inventory standaloneSegmentInventory,
	outputPath string,
	extractOpts standaloneExtractOptions,
	ledger *conversionFileLedger,
) (artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	input := inventory.input
	if err := completeConversionInputStage(ctx, input, conversionInputStageStandaloneExtract, nil); err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		err = completeStandaloneExtractStage(ctx, input, err)
		if err != nil {
			artifacts = nil
			caveats = nil
			decisions = nil
		}
	}()
	if ledger == nil {
		return nil, nil, nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneExtract,
			input.DisplayPath(),
			fmt.Errorf("nil conversion file ledger"),
		)
	}
	if inventory.inputSize != input.Size() {
		return nil, nil, nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneExtract,
			input.DisplayPath(),
			fmt.Errorf("standalone inventory size %d does not match input authority size %d", inventory.inputSize, input.Size()),
		)
	}
	for index, segment := range inventory.segments {
		if !standaloneSegmentRangeValid(segment, inventory.inputSize) {
			return nil, nil, nil, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageStandaloneExtract,
				input.DisplayPath(),
				fmt.Errorf("standalone inventory segment %d has invalid range: offset=%d length=%d input_size=%d", index, segment.Offset, segment.Length, inventory.inputSize),
			)
		}
		verified, ok := readStandaloneSegmentAt(input, segment.Offset, inventory.inputSize)
		if !ok || verified != segment {
			return nil, nil, nil, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageStandaloneExtract,
				input.DisplayPath(),
				fmt.Errorf("standalone inventory segment %d does not match its authority header", index),
			)
		}
	}
	if len(inventory.segments) == 0 {
		return nil, nil, nil, nil
	}
	if !extractOpts.GeneratePerfTrace {
		perfDataCount := 0
		for _, seg := range inventory.segments {
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
	base := traceSidecarBase(input.DisplayPath(), outputPath)
	perfTraceProviders := map[string]int{}
	perfOrdinal := 0
	for segmentIndex, seg := range inventory.segments {
		if seg.DataType != profilerDataTypeHiperf {
			continue
		}
		perfOrdinal++
		rawArtifact, perfTrace, caveat, providerDecisions, err := extractOneStandaloneHiperfSegment(
			ctx, opts, inventory, segmentIndex, base, perfOrdinal, ledger,
		)
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
		if perfTrace.Perf != nil && perfTrace.Perf.TraceQueryReady {
			rawArtifact.Caveats = append(rawArtifact.Caveats, "raw perf.data sidecar preserved; query-ready normalized .perftrace was generated for trace_query CPU-sample aggregation")
		} else {
			rawArtifact.Caveats = append(rawArtifact.Caveats, "raw perf.data sidecar preserved; normalized .perftrace records capture-quality inventory only and is not query-ready")
		}
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

func completeStandaloneExtractStage(ctx context.Context, input conversionInputView, operationErr error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			if errors.Is(operationErr, contextErr) {
				return operationErr
			}
			return traceDBJoinPreservingSingle(contextErr, operationErr)
		}
	}
	if operationErr != nil {
		var typed *ConversionInputError
		if errors.As(operationErr, &typed) && typed.Stage == conversionInputStageExternalTool.String() {
			return operationErr
		}
	}
	return completeConversionInputStage(ctx, input, conversionInputStageStandaloneExtract, operationErr)
}

func extractOneStandaloneHiperfSegment(
	ctx context.Context,
	opts Options,
	inventory standaloneSegmentInventory,
	segmentIndex int,
	base string,
	ordinal int,
	ledger *conversionFileLedger,
) (rawArtifact Artifact, perfTrace Artifact, caveat string, decisions []PerfProviderDecision, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ledger == nil {
		path := ""
		if inventory.input != nil {
			path = inventory.input.DisplayPath()
		}
		return Artifact{}, Artifact{}, "", nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneExtract,
			path,
			fmt.Errorf("standalone HIPERF conversion file ledger is nil"),
		)
	}
	outPath := numberedSidecarPath(base, ordinal, ".perf.data")
	perfTracePath := numberedSidecarPath(base, ordinal, ".perftrace")
	target, err := prepareSealedConversionPublicationTarget(outPath, ".codrax-hiperf-input-*")
	if err != nil {
		return Artifact{}, Artifact{}, "", nil, err
	}
	privateIdentity := capturePrivatePathIdentity(target.stagingDir.Path())
	defer func() {
		redactPerfProviderPrivateOutputs(&perfTrace, &caveat, &decisions, &err, privateIdentity)
	}()
	defer func() {
		err = traceDBJoinPreservingSingle(err, target.Cleanup())
	}()

	payload, err := newStandaloneHiperfPayloadView(inventory, segmentIndex, outPath)
	if err != nil {
		return Artifact{}, Artifact{}, "", nil, err
	}
	inputFormat, err := detectPerfInputFormatFromView(ctx, payload, conversionInputStageStandaloneExtract)
	if err != nil {
		return Artifact{}, Artifact{}, "", nil, err
	}
	binding, err := newStandaloneHiperfInputBinding(payload, inputFormat)
	if err != nil {
		return Artifact{}, Artifact{}, "", nil, err
	}
	resolution := resolveHiperfProviderTool(opts)
	lease, err := newExternalToolInputLeaseWithPublicProgress(
		ctx,
		opts,
		payload,
		target.stagingDir,
		target.finalLeaf,
		resolution.ExternalInputProfile,
		"hiperf_input_snapshot",
		"hiperf",
		outPath,
		perfTracePath,
	)
	if err != nil {
		return Artifact{}, Artifact{}, "", nil, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, lease.Close())
	}()

	perfTrace, caveat, decisions, err = maybeConvertHiperfPerfDataFromInput(
		ctx, opts, binding, resolution, lease, target.stagingDir, perfTracePath, ledger,
	)
	if err != nil {
		return Artifact{}, Artifact{}, caveat, decisions, err
	}
	sealedPayload, err := sealExternalToolInputSnapshot(ctx, lease, target.stagingDir)
	if err != nil {
		return Artifact{}, Artifact{}, caveat, decisions, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, sealedPayload.Close())
	}()
	if err := publishSealedConversionFileNoReplace(ctx, target, sealedPayload, ledger); err != nil {
		return Artifact{}, Artifact{}, caveat, decisions, err
	}
	segment := inventory.segments[segmentIndex]
	rawArtifact = Artifact{
		Type:          ArtifactPerfData,
		Path:          outPath,
		Bytes:         payload.Size(),
		DataType:      segment.DataType,
		PluginName:    segment.PluginName,
		PluginVersion: segment.PluginVersion,
		SourceOffset:  segment.Offset,
		SourceBytes:   segment.Length,
		Converter:     converterVersion,
		Perf:          perfCapabilityForRawPerfDataArtifact(inputFormat),
	}
	return rawArtifact, perfTrace, caveat, decisions, nil
}

func statusInputContainsStandalonePerfSidecar(ctx context.Context, input string) (bool, error) {
	segments, err := findStandaloneSegmentsAtPathForStatus(ctx, input)
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

func findStandaloneSegmentsFromInput(ctx context.Context, input conversionInputView) (inventory standaloneSegmentInventory, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageStandaloneScan, nil); err != nil {
		return standaloneSegmentInventory{}, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageStandaloneScan, err)
		if err != nil {
			inventory = standaloneSegmentInventory{}
		}
	}()
	size := input.Size()
	if size < 0 {
		return standaloneSegmentInventory{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneScan,
			input.DisplayPath(),
			fmt.Errorf("negative conversion input size %d", size),
		)
	}
	segments, err := scanStandaloneSegments(ctx, input, size)
	if err != nil {
		return standaloneSegmentInventory{}, err
	}
	return standaloneSegmentInventory{inputSize: size, segments: segments, input: input}, nil
}

func findStandaloneSegmentsAtPathForStatus(ctx context.Context, path string) (segments []standaloneSegment, err error) {
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
		if err != nil {
			segments = nil
		}
	}()
	inventory, err := findStandaloneSegmentsFromInput(ctx, authority)
	if err != nil {
		return nil, err
	}
	return inventory.segments, nil
}

func scanStandaloneSegments(ctx context.Context, reader io.ReaderAt, size int64) ([]standaloneSegment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		return nil, fmt.Errorf("standalone scan reader is nil")
	}
	if size < 0 {
		return nil, fmt.Errorf("standalone scan size is negative: %d", size)
	}
	if size < profilerTraceHeaderSize {
		return nil, nil
	}
	section := io.NewSectionReader(reader, 0, size)
	const chunkSize = 1024 * 1024
	overlap := len(profilerTraceHeaderMagicLE) - 1
	buf := make([]byte, chunkSize+overlap)
	var segments []standaloneSegment
	var base int64
	var carry []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		copy(buf, carry)
		n, readErr := section.Read(buf[len(carry):chunkSize])
		if n == 0 && readErr == nil {
			return nil, io.ErrNoProgress
		}
		window := buf[:len(carry)+n]
		search := window
		searchBase := base - int64(len(carry))
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			idx := bytes.Index(search, profilerTraceHeaderMagicLE)
			if idx < 0 {
				break
			}
			candidate := searchBase + int64(idx)
			if candidate >= 0 {
				if seg, ok := readStandaloneSegmentAt(reader, candidate, size); ok {
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

func readStandaloneSegmentAt(reader io.ReaderAt, off int64, fileSize int64) (standaloneSegment, bool) {
	if reader == nil || off < 0 || off > fileSize || int64(profilerTraceHeaderSize) > fileSize-off {
		return standaloneSegment{}, false
	}
	header := make([]byte, profilerTraceHeaderSize)
	if _, err := io.ReadFull(io.NewSectionReader(reader, off, profilerTraceHeaderSize), header); err != nil {
		return standaloneSegment{}, false
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return standaloneSegment{}, false
	}
	declaredLength := binary.LittleEndian.Uint64(header[8:16])
	if declaredLength < profilerTraceHeaderSize || declaredLength > uint64(fileSize-off) {
		return standaloneSegment{}, false
	}
	length := int64(declaredLength)
	dataType := binary.LittleEndian.Uint32(header[56:60])
	return standaloneSegment{
		Offset:        off,
		Length:        length,
		DataType:      dataType,
		PluginName:    cString(header[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize]),
		PluginVersion: cString(header[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize]),
	}, true
}

func standaloneSegmentRangeValid(segment standaloneSegment, fileSize int64) bool {
	if fileSize < 0 || segment.Offset < 0 || segment.Offset > fileSize {
		return false
	}
	remaining := fileSize - segment.Offset
	return int64(profilerTraceHeaderSize) <= remaining &&
		segment.Length >= profilerTraceHeaderSize &&
		segment.Length <= remaining
}

func writeTraceBundle(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision) (Artifact, error) {
	return writeTraceBundleWithLedger(context.Background(), input, outputPath, artifacts, caveats, decisions, traceDecisions, nil)
}

func writeTraceBundleWithLedger(ctx context.Context, input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, ledger *conversionFileLedger) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(ctx, input, outputPath, artifacts, caveats, decisions, traceDecisions, nil, nil, traceToolGatesForBundle(), ledger)
}

func writeTraceBundleWithCoverage(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, coverage []TraceDBCoverage) (Artifact, error) {
	return writeTraceBundleWithAllCoverage(input, outputPath, artifacts, caveats, decisions, traceDecisions, coverage, nil)
}

func writeTraceBundleWithAllCoverage(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndLedger(context.Background(), input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, nil)
}

func writeTraceBundleWithAllCoverageAndLedger(ctx context.Context, input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, ledger *conversionFileLedger) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(ctx, input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, traceToolGatesForBundle(), ledger)
}

func writeTraceBundleWithAllCoverageAndGates(input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, traceToolGates []TraceToolGateStatus) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedger(context.Background(), input, outputPath, artifacts, caveats, decisions, traceDecisions, dbCoverage, traceCoverage, traceToolGates, nil)
}

type traceBundlePublicationPhase string

const (
	traceBundlePublicationTargetPrepared traceBundlePublicationPhase = "target_prepared"
	traceBundlePublicationStagingWritten traceBundlePublicationPhase = "staging_written"
	traceBundlePublicationStagingAdopted traceBundlePublicationPhase = "staging_adopted"
	traceBundlePublicationBodyAttested   traceBundlePublicationPhase = "body_attested"
	traceBundlePublicationBeforePublish  traceBundlePublicationPhase = "before_publish"
	traceBundlePublicationAfterPublish   traceBundlePublicationPhase = "after_publish"
	traceBundlePublicationBeforeCommit   traceBundlePublicationPhase = "before_commit"
)

type traceBundleStagingWriter interface {
	io.Writer
	Sync() error
	Close() error
}

// traceBundlePublicationOps is a per-call deterministic fault seam. Production
// uses the zero value; tests may wrap only private staging I/O or pause at a
// checkpoint. The exact public publisher is intentionally not injectable.
type traceBundlePublicationOps struct {
	openStaging func(string) (traceBundleStagingWriter, error)
	checkpoint  func(traceBundlePublicationPhase) error
}

func (ops traceBundlePublicationOps) openPrivateStaging(path string) (traceBundleStagingWriter, error) {
	if ops.openStaging != nil {
		return ops.openStaging(path)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
}

func (ops traceBundlePublicationOps) reach(ctx context.Context, phase traceBundlePublicationPhase) error {
	if ops.checkpoint != nil {
		if err := ops.checkpoint(phase); err != nil {
			return fmt.Errorf("tracebundle publication checkpoint %s: %w", phase, err)
		}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("tracebundle publication canceled at %s: %w", phase, err)
		}
	}
	return nil
}

func writeTraceBundleWithAllCoverageAndGatesAndLedger(ctx context.Context, input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, traceToolGates []TraceToolGateStatus, ledger *conversionFileLedger) (artifact Artifact, err error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedgerOps(
		ctx, input, outputPath, artifacts, caveats, decisions, traceDecisions,
		dbCoverage, traceCoverage, traceToolGates, ledger, traceBundlePublicationOps{},
	)
}

func writeTraceBundleWithAllCoverageAndGatesAndLedgerOps(ctx context.Context, input, outputPath string, artifacts []Artifact, caveats []string, decisions []PerfProviderDecision, traceDecisions []TraceProviderDecision, dbCoverage []TraceDBCoverage, traceCoverage []TraceDBCoverage, traceToolGates []TraceToolGateStatus, ledger *conversionFileLedger, publicationOps traceBundlePublicationOps) (artifact Artifact, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	rawArtifacts := append([]Artifact(nil), artifacts...)
	artifacts = dedupeArtifacts(artifacts)
	caveats = dedupeStrings(caveats)
	if len(artifacts) == 0 {
		if len(decisions) == 0 && len(traceDecisions) == 0 && len(dbCoverage) == 0 && len(traceCoverage) == 0 && len(caveats) == 0 {
			return Artifact{}, nil
		}
	}
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
	if err := auditTraceBundleOwnedSystraceReceipts(outputPath, rawArtifacts, traceDecisions, dbCoverage, traceCoverage, ledger); err != nil {
		return Artifact{}, err
	}
	if err := auditTraceBundleOwnedPerfReceipts(rawArtifacts, decisions, dbCoverage, traceCoverage, ledger); err != nil {
		return Artifact{}, err
	}
	bundleOutputPath := traceBundleOutputBindingPath(outputPath, artifacts)
	base := traceSidecarBase(input, bundleOutputPath)
	path := base + ".tracebundle.json"
	manifestArtifacts, captureID, heldChildren, err := buildTraceBundleV2Artifacts(ctx, path, artifacts, ledger)
	if err != nil {
		return Artifact{}, err
	}
	heldChildrenClosed := false
	defer func() {
		if !heldChildrenClosed {
			err = traceDBJoinPreservingSingle(err, closeHeldSealedOwnedFiles(heldChildren))
		}
	}()
	manifestDecisions, manifestTraceCoverage, err := rewriteTraceBundlePerfMetadata(
		artifacts, manifestArtifacts, decisions, traceCoverage,
	)
	if err != nil {
		return Artifact{}, err
	}
	manifestTraceDecisions, manifestTraceCoverage, err := rewriteTraceBundleSystraceMetadata(
		artifacts, manifestArtifacts, traceDecisions, manifestTraceCoverage,
	)
	if err != nil {
		return Artifact{}, err
	}
	manifestSystrace, err := traceBundleSystracePath(outputPath, artifacts, manifestArtifacts)
	if err != nil {
		return Artifact{}, err
	}
	meta := traceBundleMetadata{
		Schema:              tracebundle.SchemaV2,
		CaptureID:           captureID,
		Version:             converterVersion,
		InputPath:           input,
		Systrace:            manifestSystrace,
		Artifacts:           manifestArtifacts,
		ProviderDecisions:   manifestDecisions,
		TraceDecisions:      manifestTraceDecisions,
		TraceDBCoverage:     dbCoverage,
		TraceCoverage:       manifestTraceCoverage,
		TraceToolGates:      traceToolGates,
		PerfClockAlignments: perfClockAlignmentsForArtifacts(manifestArtifacts, manifestSystrace),
		Caveats:             caveats,
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	body = append(body, '\n')
	if err := tracebundle.ValidateManifestBytes(ctx, body); err != nil {
		return Artifact{}, fmt.Errorf("validate tracebundle final body: %w", err)
	}
	target, err := prepareSealedConversionPublicationTarget(path, ".codrax-tracebundle-*")
	if err != nil {
		return Artifact{}, err
	}
	privateStagingRoot := target.stagingDir.Path()
	targetCleanup := target.Cleanup
	defer func() {
		if targetCleanup != nil {
			cleanupErr := targetCleanup()
			targetCleanup = nil
			err = traceDBJoinPreservingSingle(err, cleanupErr)
		}
		err = redactTraceStreamerPrivateError(err, privateStagingRoot)
	}()
	if err := publicationOps.reach(ctx, traceBundlePublicationTargetPrepared); err != nil {
		return Artifact{}, err
	}
	sealedManifest, err := stageAndValidateTraceBundleManifest(ctx, target, body, publicationOps)
	if err != nil {
		return Artifact{}, err
	}
	sealedManifestClosed := false
	defer func() {
		if !sealedManifestClosed {
			err = traceDBJoinPreservingSingle(err, sealedManifest.Close())
		}
	}()
	for _, child := range heldChildren {
		if err := child.Validate(ctx); err != nil {
			return Artifact{}, fmt.Errorf("revalidate causal child before tracebundle publication: %w", err)
		}
	}
	if err := publicationOps.reach(ctx, traceBundlePublicationBeforePublish); err != nil {
		return Artifact{}, err
	}
	if err := publishSealedConversionFileNoReplace(ctx, target, sealedManifest, ledger); err != nil {
		return Artifact{}, err
	}
	if err := publicationOps.reach(ctx, traceBundlePublicationAfterPublish); err != nil {
		return Artifact{}, err
	}
	for _, child := range heldChildren {
		if err := child.Validate(ctx); err != nil {
			return Artifact{}, fmt.Errorf("revalidate causal child after tracebundle publication: %w", err)
		}
	}
	if err := ledger.validateSealedOwnedPath(ctx, path); err != nil {
		return Artifact{}, fmt.Errorf("revalidate tracebundle publication: %w", err)
	}
	if err := sealedManifest.Close(); err != nil {
		sealedManifestClosed = true
		return Artifact{}, fmt.Errorf("close sealed tracebundle staging generation: %w", err)
	}
	sealedManifestClosed = true
	if cleanupErr := targetCleanup(); cleanupErr != nil {
		targetCleanup = nil
		return Artifact{}, traceDBJoinPreservingSingle(
			fmt.Errorf("cleanup private tracebundle staging: %w", cleanupErr), ledger.removeOwnedPath(path),
		)
	}
	targetCleanup = nil
	if err := publicationOps.reach(ctx, traceBundlePublicationBeforeCommit); err != nil {
		return Artifact{}, err
	}
	heldCloseErr := closeHeldSealedOwnedFiles(heldChildren)
	heldChildrenClosed = true
	if heldCloseErr != nil {
		return Artifact{}, fmt.Errorf("release held causal children after tracebundle publication: %w", heldCloseErr)
	}
	if ownedLedger {
		if err := ledger.validateOwnedPaths(); err != nil {
			return Artifact{}, err
		}
		if err := ledger.releaseOwnedAuthorities(); err != nil {
			return Artifact{}, fmt.Errorf("release tracebundle publication authority: %w", err)
		}
	}
	committed = true
	return Artifact{Type: ArtifactTraceBundle, Path: path, Bytes: int64(len(body)), Converter: converterVersion}, nil
}

func stageAndValidateTraceBundleManifest(ctx context.Context, target sealedConversionPublicationTarget, body []byte, publicationOps traceBundlePublicationOps) (_ *sealedConversionFile, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if target.stagingDir == nil || strings.TrimSpace(target.StagingPath) == "" || strings.TrimSpace(target.finalLeaf) == "" {
		return nil, fmt.Errorf("tracebundle private staging target is incomplete")
	}
	if err := tracebundle.ValidateManifestBytes(ctx, body); err != nil {
		return nil, fmt.Errorf("validate tracebundle staging body: %w", err)
	}
	out, err := publicationOps.openPrivateStaging(target.StagingPath)
	if err != nil {
		return nil, fmt.Errorf("create private tracebundle staging file: %w", err)
	}
	if out == nil {
		return nil, fmt.Errorf("create private tracebundle staging file: writer is nil")
	}
	written, operationErr := io.Copy(out, bytes.NewReader(body))
	if operationErr != nil {
		operationErr = fmt.Errorf("write private tracebundle staging body: %w", operationErr)
	} else if written != int64(len(body)) {
		operationErr = fmt.Errorf("write private tracebundle staging body: wrote=%d want=%d: %w", written, len(body), io.ErrShortWrite)
	}
	if operationErr == nil {
		if syncErr := out.Sync(); syncErr != nil {
			operationErr = fmt.Errorf("sync private tracebundle staging body: %w", syncErr)
		}
	}
	closeErr := out.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close private tracebundle staging body: %w", closeErr)
	}
	if operationErr != nil || closeErr != nil {
		return nil, traceDBJoinPreservingSingle(operationErr, closeErr)
	}
	if err := publicationOps.reach(ctx, traceBundlePublicationStagingWritten); err != nil {
		return nil, err
	}
	sealed, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		return nil, fmt.Errorf("adopt private tracebundle staging generation: %w", err)
	}
	if err := publicationOps.reach(ctx, traceBundlePublicationStagingAdopted); err != nil {
		return nil, traceDBJoinPreservingSingle(err, sealed.Close())
	}
	if err := validateSealedTraceBundleManifestBody(ctx, sealed, body); err != nil {
		return nil, traceDBJoinPreservingSingle(err, sealed.Close())
	}
	if err := publicationOps.reach(ctx, traceBundlePublicationBodyAttested); err != nil {
		return nil, traceDBJoinPreservingSingle(err, sealed.Close())
	}
	return sealed, nil
}

func validateSealedTraceBundleManifestBody(ctx context.Context, sealed *sealedConversionFile, body []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if sealed == nil {
		return fmt.Errorf("sealed tracebundle staging generation is nil")
	}
	if err := sealed.Validate(); err != nil {
		return fmt.Errorf("validate sealed tracebundle before body attestation: %w", err)
	}
	wantDigest := sha256.Sum256(body)
	wantSHA := hex.EncodeToString(wantDigest[:])
	err := sealed.withOpenFile(func(file *os.File) error {
		if err := tracebundle.ValidateFile(ctx, file, sealed.identity); err != nil {
			return fmt.Errorf("validate held staging generation before attestation: %w", err)
		}
		measuredBytes, measuredSHA, measuredIdentity, err := tracebundle.MeasureFile(ctx, file)
		if err != nil {
			return err
		}
		if measuredBytes != int64(len(body)) || measuredSHA != wantSHA || !sealed.identity.SameVersion(measuredIdentity) {
			return fmt.Errorf("tracebundle staging body attestation mismatch: bytes=%d want=%d sha256=%s want_sha256=%s",
				measuredBytes, len(body), measuredSHA, wantSHA)
		}
		if err := tracebundle.ValidateFile(ctx, file, sealed.identity); err != nil {
			return fmt.Errorf("validate held staging generation after attestation: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("attest sealed tracebundle staging body: %w", err)
	}
	if err := sealed.Validate(); err != nil {
		return fmt.Errorf("validate sealed tracebundle after body attestation: %w", err)
	}
	return nil
}

func buildTraceBundleV2Artifacts(ctx context.Context, bundlePath string, artifacts []Artifact, ledger *conversionFileLedger) (_ []Artifact, _ string, _ []*heldSealedOwnedFile, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	bundleAbs, err := filepath.Abs(filepath.Clean(bundlePath))
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve tracebundle path: %w", err)
	}
	bundleDir := filepath.Dir(bundleAbs)
	out := cloneArtifactList(artifacts)
	members := make([]tracebundle.CaptureMember, 0, len(out))
	heldChildren := make([]*heldSealedOwnedFile, 0, len(out))
	defer func() {
		if err != nil {
			err = traceDBJoinPreservingSingle(err, closeHeldSealedOwnedFiles(heldChildren))
		}
	}()
	physical := make([]os.FileInfo, 0, len(out))
	for i := range out {
		if err := ctx.Err(); err != nil {
			return nil, "", nil, err
		}
		publicArtifact := out[i]
		originalPath := strings.TrimSpace(publicArtifact.Path)
		if originalPath == "" {
			continue
		}
		bindingPath := originalPath
		var systraceClaim publishedOwnedTraceValidation
		var perfClaim publishedOwnedTraceValidation
		if publicArtifact.Type == ArtifactSystrace || publicArtifact.Type == ArtifactPerfTrace {
			if ledger == nil {
				return nil, "", nil, fmt.Errorf("conversion file ledger is required to bind causal child %s", originalPath)
			}
		}
		if publicArtifact.Type == ArtifactSystrace {
			kind, kindErr := ownedSystraceArtifactKind(publicArtifact)
			if kindErr != nil {
				return nil, "", nil, newOwnedTracePublicationError("bind_bundle_systrace_receipt", originalPath, kindErr)
			}
			systraceClaim, err = validateOwnedSystraceArtifactClaim(ledger, publicArtifact, kind)
			if err != nil {
				return nil, "", nil, err
			}
			bindingPath = publicArtifact.traceReceiptBindingPath
		}
		if publicArtifact.Type == ArtifactPerfTrace {
			if publicArtifact.Perf == nil {
				return nil, "", nil, newOwnedTracePublicationError(
					"bind_bundle_perf_receipt", originalPath, fmt.Errorf("perf artifact capability is absent"),
				)
			}
			profile, ok := ownedTracePerfProfileForProvider(publicArtifact.Perf.ProviderName)
			if !ok {
				return nil, "", nil, newOwnedTracePublicationError(
					"bind_bundle_perf_receipt", originalPath, fmt.Errorf("perf artifact provider profile is not closed"),
				)
			}
			perfClaim, err = validateOwnedPerfTraceArtifactClaim(ledger, publicArtifact, profile)
			if err != nil {
				return nil, "", nil, err
			}
		}
		artifactAbs, err := filepath.Abs(filepath.Clean(bindingPath))
		if err != nil {
			return nil, "", nil, fmt.Errorf("resolve tracebundle artifact %q: %w", originalPath, err)
		}
		relative, err := filepath.Rel(bundleDir, artifactAbs)
		if err != nil {
			return nil, "", nil, fmt.Errorf("make tracebundle artifact bundle-relative %q: %w", originalPath, err)
		}
		wirePath := filepath.ToSlash(filepath.Clean(relative))
		out[i].Path = wirePath
		perfSuffix := strings.EqualFold(path.Ext(wirePath), ".perftrace")
		switch {
		case out[i].Type == ArtifactSystrace && perfSuffix:
			return nil, "", nil, fmt.Errorf("tracebundle artifact type=systrace conflicts with .perftrace path %q", wirePath)
		case out[i].Type != ArtifactPerfTrace && perfSuffix:
			return nil, "", nil, fmt.Errorf("tracebundle artifact %q with .perftrace path requires exact type=perftrace", wirePath)
		}
		if out[i].Type != ArtifactSystrace && out[i].Type != ArtifactPerfTrace {
			continue
		}
		if err := tracebundle.ValidateCapturePath(wirePath); err != nil {
			return nil, "", nil, fmt.Errorf("causal tracebundle artifact %q: %w", originalPath, err)
		}
		info, err := ledger.sealedOwnedFileInfo(bindingPath)
		if err != nil {
			if publicArtifact.Type == ArtifactPerfTrace {
				return nil, "", nil, newOwnedTracePublicationError("bind_bundle_perf_receipt", originalPath, err)
			}
			return nil, "", nil, newOwnedTracePublicationError("bind_bundle_systrace_receipt", originalPath, err)
		}
		for _, prior := range physical {
			if os.SameFile(prior, info) {
				return nil, "", nil, newOwnedTracePublicationError(
					"bind_bundle_causal_child_identity", originalPath,
					fmt.Errorf("duplicate physical causal child generation"),
				)
			}
		}
		physical = append(physical, info)
		measuredBytes, measuredSHA, held, err := ledger.holdAndMeasureSealedOwnedPath(ctx, bindingPath)
		if err != nil {
			if publicArtifact.Type == ArtifactPerfTrace {
				return nil, "", nil, newOwnedTracePublicationError("bind_bundle_perf_receipt", originalPath, err)
			}
			return nil, "", nil, newOwnedTracePublicationError("bind_bundle_systrace_receipt", originalPath, err)
		}
		heldChildren = append(heldChildren, held)
		if publicArtifact.Type == ArtifactSystrace {
			claimSHA := hex.EncodeToString(systraceClaim.receipt.wireSHA256[:])
			if systraceClaim.receipt.size != measuredBytes || claimSHA != measuredSHA ||
				!systraceClaim.publishedIdentity.SameVersion(held.sealedIdentity) {
				return nil, "", nil, newOwnedTracePublicationError(
					"bind_bundle_systrace_receipt", originalPath,
					fmt.Errorf("held systrace child does not match its validated public receipt"),
				)
			}
		}
		if publicArtifact.Type == ArtifactPerfTrace {
			claimSHA := hex.EncodeToString(perfClaim.receipt.wireSHA256[:])
			if perfClaim.receipt.size != measuredBytes || claimSHA != measuredSHA ||
				!perfClaim.publishedIdentity.SameVersion(held.sealedIdentity) {
				return nil, "", nil, newOwnedTracePublicationError(
					"bind_bundle_perf_receipt", originalPath,
					fmt.Errorf("held perf child does not match its validated public receipt"),
				)
			}
		}
		out[i].Bytes = measuredBytes
		out[i].SHA256 = measuredSHA
		members = append(members, tracebundle.CaptureMember{
			Type: out[i].Type, Path: wirePath, Bytes: measuredBytes, SHA256: measuredSHA,
		})
	}
	captureID, err := tracebundle.CaptureID(members)
	if err != nil {
		return nil, "", nil, fmt.Errorf("derive tracebundle capture identity: %w", err)
	}
	return out, captureID, heldChildren, nil
}

func traceToolGatesForBundle() []TraceToolGateStatus {
	gate := buildSysBinaryParityGateStatus()
	if strings.TrimSpace(gate.Name) == "" {
		return nil
	}
	return []TraceToolGateStatus{gate}
}

func traceBundleOutputBindingPath(outputPath string, artifacts []Artifact) string {
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactSystrace && artifact.Path == outputPath {
			return artifact.traceReceiptBindingPath
		}
	}
	return outputPath
}

func traceBundleSystracePath(outputPath string, publicArtifacts, manifestArtifacts []Artifact) (string, error) {
	if len(publicArtifacts) != len(manifestArtifacts) {
		return "", newOwnedTracePublicationError(
			"select_bundle_primary_systrace", outputPath,
			fmt.Errorf("tracebundle artifact projection changed cardinality"),
		)
	}
	hasSystrace := false
	for index, artifact := range publicArtifacts {
		if artifact.Type != ArtifactSystrace {
			continue
		}
		hasSystrace = true
		if artifact.Path != outputPath {
			continue
		}
		if manifestArtifacts[index].Type != ArtifactSystrace ||
			strings.TrimSpace(manifestArtifacts[index].Path) == "" {
			return "", newOwnedTracePublicationError(
				"select_bundle_primary_systrace", outputPath,
				fmt.Errorf("primary systrace has no bundle-relative projection"),
			)
		}
		return manifestArtifacts[index].Path, nil
	}
	if hasSystrace {
		return "", newOwnedTracePublicationError(
			"select_bundle_primary_systrace", outputPath,
			fmt.Errorf("primary systrace does not match a validated artifact"),
		)
	}
	return "", nil
}

func perfClockAlignmentsForArtifacts(artifacts []Artifact, primarySystracePath string) []PerfClockAlignment {
	var out []PerfClockAlignment
	hasSystraceInventory := false
	hasQueryReadySystrace := false
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactSystrace && artifact.Path == primarySystracePath &&
			strings.TrimSpace(artifact.Path) != "" {
			hasSystraceInventory = true
			if artifact.Trace != nil && artifact.Trace.TraceQueryReady {
				hasQueryReadySystrace = true
			}
		}
	}
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfTrace || artifact.Perf == nil || !artifact.Perf.TraceQueryReady {
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
		if !hasQueryReadySystrace {
			item.TraceTimeDomain = "missing_trace_body"
			item.Confidence = "trace_body_missing"
			if hasSystraceInventory {
				item.TraceTimeDomain = "trace_body_not_query_ready"
				item.Confidence = "trace_body_not_query_ready"
			}
			item.Calibrated = false
			if hasSystraceInventory {
				item.Caveats = append(item.Caveats, "a systrace inventory artifact exists, but its receipt did not prove trace-query readiness; trace_query can aggregate validated perf samples, but cannot use artifact existence alone to claim trace-window or scheduling-causality capability")
			} else {
				item.Caveats = append(item.Caveats, "no systrace trace body is available in this tracebundle; trace_query can aggregate perf samples, but cannot correlate them to trace windows until a systrace artifact is attached or generated")
			}
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

func copyRangeToFileWithLedger(ctx context.Context, in io.ReaderAt, off, length int64, outPath string, ledger *conversionFileLedger) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if in == nil {
		return 0, fmt.Errorf("standalone source reader is nil")
	}
	if ledger == nil {
		return 0, fmt.Errorf("standalone conversion file ledger is nil")
	}
	if off < 0 || length < 0 {
		return 0, fmt.Errorf("invalid standalone source range: offset=%d length=%d", off, length)
	}
	if off > int64(^uint64(0)>>1)-length {
		return 0, fmt.Errorf("standalone source range overflows int64: offset=%d length=%d", off, length)
	}
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	if err := ledger.recordOpenFile(outPath, out); err != nil {
		return 0, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(outPath, out))
	}
	written, copyErr := copyStandaloneRange(ctx, out, io.NewSectionReader(in, off, length))
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

func copyStandaloneRange(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	return copyCancellableRange(ctx, dst, src, nil)
}

func copyCancellableRange(ctx context.Context, dst io.Writer, src io.Reader, observe func(written int64)) (int64, error) {
	buffer := make([]byte, 64*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			n, writeErr := dst.Write(buffer[:read])
			written += int64(n)
			if observe != nil {
				observe(written)
			}
			if writeErr != nil {
				return written, writeErr
			}
			if n != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
		if read == 0 {
			return written, io.ErrNoProgress
		}
	}
}

func cString(data []byte) string {
	if idx := bytes.IndexByte(data, 0); idx >= 0 {
		data = data[:idx]
	}
	return strings.TrimSpace(string(data))
}
