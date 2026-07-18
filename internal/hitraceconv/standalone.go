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
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

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
	maxProfilerStandaloneBlocks   = 256
	maxProfilerHiperfCandidates   = 64
)

const (
	standaloneIntegrityPayloadSHA256 = "standalone_payload_sha256"
	standaloneIntegrityOfficialZero  = "standalone_official_zero_sha"
	standaloneLayoutDirectOffsetZero = "direct_offset0_exact_eof"
	standaloneLayoutRootProfile      = "root_profile_exact_terminus"
	standaloneLayoutRootEnvelope     = "root_envelope_exact_terminus"
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
	Offset          int64
	Length          int64
	DataType        uint32
	PluginName      string
	PluginVersion   string
	Integrity       string
	Layout          string
	PayloadSHA256   [sha256.Size]byte
	PerfEligible    bool
	PerfInputFormat perfInputFormat
	ArtifactPath    string
	BindingPath     string
}

// profilerStandaloneHeader is the one typed structural interpretation of an
// OpenHarmony standalone object header. Artifact admission adds payload
// integrity and layout-anchor policy on top of this grammar; SessionJSON tail
// rejection deliberately consumes only this structural proof so an official
// zero-SHA binary object can never be reinterpreted as text.
type profilerStandaloneHeader struct {
	Length        int64
	DataType      uint32
	PluginName    string
	PluginVersion string
	DeclaredSHA   [sha256.Size]byte
}

type standaloneSegmentInventory struct {
	inputSize            int64
	segments             []standaloneSegment
	input                conversionInputView
	rootProof            *profilerRootProfileProof
	offsetZeroProfiler   bool
	directStandalone     bool
	rootLayoutError      string
	standaloneChainError string
}

func (inventory standaloneSegmentInventory) hasHiperfData() bool {
	for _, segment := range inventory.segments {
		if segment.PerfEligible {
			return true
		}
	}
	return false
}

func standaloneLayoutRejectionItem(scope, reason string) (string, TraceDBCoverage) {
	caveat := fmt.Sprintf(
		"OpenHarmony standalone layout was rejected independently of trace_streamer output: scope=%s reason=%s; no unverified standalone child was published",
		scope, reason)
	return caveat, TraceDBCoverage{
		Family:   "openharmony_standalone_layout",
		Table:    "__standalone_layout__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  reason,
		FieldSources: map[string]string{
			"layout_scope":       scope,
			"publication_policy": "trace_provider_output_independent_standalone_fail_close",
		},
	}
}

func standaloneLayoutRejectionEvidence(inventory standaloneSegmentInventory) ([]string, []TraceDBCoverage) {
	var caveats []string
	var coverage []TraceDBCoverage
	appendEvidence := func(scope, reason string) {
		if reason == "" {
			return
		}
		caveat, item := standaloneLayoutRejectionItem(scope, reason)
		caveats = append(caveats, caveat)
		coverage = append(coverage, item)
	}
	appendEvidence("offset_zero_root", inventory.rootLayoutError)
	if inventory.rootProof != nil {
		appendEvidence("root_profile", inventory.rootProof.Failure)
	}
	appendEvidence("strict_standalone_chain", inventory.standaloneChainError)
	return caveats, coverage
}

func (inventory standaloneSegmentInventory) profilerTraceBodyEnd() (int64, bool) {
	if inventory.rootProof == nil || inventory.rootProof.BodyEnd < profilerTraceHeaderSize ||
		inventory.rootProof.BodyEnd > inventory.inputSize ||
		inventory.rootProof.Header.Length != uint64(inventory.rootProof.BodyEnd) {
		return 0, false
	}
	return inventory.rootProof.BodyEnd, true
}

type traceBundleMetadata struct {
	Schema              string                  `json:"schema"`
	CaptureID           string                  `json:"capture_id"`
	Version             string                  `json:"version"`
	InputPath           string                  `json:"input_path"`
	ArchiveProvenance   *TraceArchiveProvenance `json:"archive_provenance,omitempty"`
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
	if proofErr := validateStandaloneInventoryProof(inventory); proofErr != nil {
		return nil, nil, nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneExtract,
			input.DisplayPath(),
			proofErr,
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
	}
	if len(inventory.segments) == 0 {
		return nil, nil, nil, nil
	}
	if !extractOpts.GeneratePerfTrace {
		perfDataCount := 0
		for _, seg := range inventory.segments {
			if seg.PerfEligible {
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
		if !seg.PerfEligible {
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

func validateStandaloneInventoryProof(inventory standaloneSegmentInventory) error {
	if inventory.input == nil || inventory.inputSize < 0 ||
		len(inventory.segments) > maxProfilerStandaloneBlocks {
		return fmt.Errorf("standalone inventory authority is incomplete")
	}
	if inventory.standaloneChainError != "" && len(inventory.segments) != 0 {
		return fmt.Errorf("failed standalone chain retained segments")
	}
	if len(inventory.segments) == 0 {
		return nil
	}
	expectedLayout := ""
	cursor := int64(0)
	switch {
	case inventory.directStandalone && inventory.rootProof == nil:
		expectedLayout = standaloneLayoutDirectOffsetZero
	case !inventory.directStandalone && inventory.rootProof != nil && inventory.rootProof.EnvelopeVerified:
		cursor = inventory.rootProof.BodyEnd
		expectedLayout = standaloneLayoutRootEnvelope
		if inventory.rootProof.ProfileVerified {
			expectedLayout = standaloneLayoutRootProfile
		}
	default:
		return fmt.Errorf("standalone inventory has no typed layout anchor")
	}
	hiperfCandidates := 0
	for index, segment := range inventory.segments {
		if segment.Offset != cursor || segment.Layout != expectedLayout ||
			!standaloneSegmentRangeValid(segment, inventory.inputSize) ||
			(segment.Integrity != standaloneIntegrityPayloadSHA256 &&
				segment.Integrity != standaloneIntegrityOfficialZero) ||
			(segment.Integrity == standaloneIntegrityOfficialZero &&
				expectedLayout != standaloneLayoutRootProfile) ||
			segment.PerfEligible != (segment.DataType == profilerDataTypeHiperf &&
				segment.PluginName == "hiperf-plugin") {
			return fmt.Errorf("standalone inventory segment %d has invalid proof binding", index)
		}
		if segment.PerfEligible {
			hiperfCandidates++
			if hiperfCandidates > maxProfilerHiperfCandidates {
				return fmt.Errorf("standalone inventory exceeds HIPERF candidate budget")
			}
		}
		cursor += segment.Length
	}
	if cursor != inventory.inputSize {
		return fmt.Errorf("standalone inventory does not close at held EOF")
	}
	return nil
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
	receiptSegment := segment
	receiptSegment.PerfInputFormat = inputFormat
	receiptSegment.ArtifactPath = outPath
	receiptSegment.BindingPath = target.finalBindingPath
	if err := ledger.recordStandaloneSourceReceipt(target.finalBindingPath, receiptSegment); err != nil {
		return Artifact{}, Artifact{}, caveat, decisions, fmt.Errorf(
			"bind standalone source receipt: %w", err)
	}
	rawArtifact = Artifact{
		Type:          ArtifactPerfData,
		Path:          outPath,
		Bytes:         payload.Size(),
		SHA256:        hex.EncodeToString(segment.PayloadSHA256[:]),
		DataType:      segment.DataType,
		PluginName:    segment.PluginName,
		PluginVersion: segment.PluginVersion,
		SourceOffset:  segment.Offset,
		SourceBytes:   segment.Length,
		Converter:     converterVersion,
		Perf:          perfCapabilityForRawPerfDataArtifact(inputFormat),
		Standalone: &StandaloneSourceProvenance{
			Profile:         "openharmony_standalone_v1",
			LayoutAuthority: segment.Layout,
			WriterProfile:   segment.Integrity,
		},
		standaloneReceipt: func() *standaloneSegment {
			receipt := receiptSegment
			return &receipt
		}(),
	}
	return rawArtifact, perfTrace, caveat, decisions, nil
}

func statusInputContainsStandalonePerfSidecar(ctx context.Context, input string) (bool, error) {
	segments, err := findStandaloneSegmentsAtPathForStatus(ctx, input)
	if err != nil {
		return false, err
	}
	for _, seg := range segments {
		if seg.PerfEligible {
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
	segments, rootProof, offsetZeroProfiler, directStandalone, layoutFailure, err :=
		inspectProfilerStandaloneLayout(ctx, input, size)
	if err != nil {
		return standaloneSegmentInventory{}, err
	}
	rootLayoutError := ""
	chainFailure := layoutFailure
	if rootProof != nil && !rootProof.EnvelopeVerified && layoutFailure == rootProof.Failure {
		// The root proof is the sole owner of its physical/profile failure.
		// No standalone chain was entered, so the semantic root consumer must
		// publish the original typed reason without a fabricated chain prefix.
		chainFailure = ""
	} else if rootProof == nil && offsetZeroProfiler && !directStandalone && layoutFailure != "" {
		rootLayoutError = layoutFailure
		chainFailure = ""
	}
	return standaloneSegmentInventory{
		inputSize: size, segments: segments, input: input, rootProof: rootProof,
		offsetZeroProfiler: offsetZeroProfiler, directStandalone: directStandalone,
		rootLayoutError: rootLayoutError, standaloneChainError: chainFailure,
	}, nil
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

// inspectProfilerStandaloneLayout is the sole production constructor for a
// standalone inventory. It never searches arbitrary payload bytes: the cursor
// starts at offset zero for a direct capture or at a physically proven root
// TraceFile terminus, then advances block-by-block to exact EOF.
func inspectProfilerStandaloneLayout(ctx context.Context, reader io.ReaderAt, size int64) (
	segments []standaloneSegment,
	rootProof *profilerRootProfileProof,
	offsetZeroProfiler bool,
	directStandalone bool,
	chainFailure string,
	err error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		return nil, nil, false, false, "", fmt.Errorf("standalone layout reader is nil")
	}
	if size < 0 {
		return nil, nil, false, false, "", fmt.Errorf("standalone layout size is negative: %d", size)
	}
	if size < profilerTraceHeaderSize {
		return nil, nil, false, false, "", nil
	}
	header, ok, readErr := readProfilerTraceHeaderAtExact(reader, 0, size)
	if readErr != nil {
		return nil, nil, false, false, "", readErr
	}
	if !ok {
		return nil, nil, false, false, "", nil
	}
	offsetZeroProfiler = true
	start := int64(0)
	// WriteStandalonePluginFile appends to a stream whose constructors already
	// emitted the root TraceFile header; its all-zero SHA profile is therefore
	// legal only behind an authenticated root profile. A direct offset-zero
	// compatibility block must carry an exact payload digest.
	allowOfficialZero := false
	switch {
	case header.DataType == profilerDataTypeProtobuf:
		if header.Length > math.MaxInt64 || header.Length < profilerTraceHeaderSize || header.Length > uint64(size) {
			return nil, nil, true, false, "profiler_root_declared_length_mismatch", nil
		}
		candidateEnd := int64(header.Length)
		proof, proofErr := validateProfilerRootProfileEnvelope(
			ctx, reader, header, candidateEnd, maxProfilerPluginFrameBytes)
		if proofErr != nil {
			return nil, nil, true, false, "", proofErr
		}
		rootProof = &proof
		if !proof.EnvelopeVerified {
			return nil, rootProof, true, false, proof.Failure, nil
		}
		start = candidateEnd
		allowOfficialZero = proof.ProfileVerified
	case isProfilerStandaloneDataType(header.DataType):
		directStandalone = true
	default:
		return nil, nil, true, false, "profiler_offset_zero_data_type_unsupported", nil
	}
	layoutAuthority := standaloneLayoutDirectOffsetZero
	if rootProof != nil {
		layoutAuthority = standaloneLayoutRootEnvelope
		if rootProof.ProfileVerified {
			layoutAuthority = standaloneLayoutRootProfile
		}
	}
	segments, chainFailure, err = validateProfilerStandaloneChain(
		ctx, reader, size, start, allowOfficialZero, layoutAuthority)
	if err != nil {
		return nil, rootProof, true, directStandalone, "", err
	}
	if chainFailure != "" {
		return nil, rootProof, true, directStandalone, chainFailure, nil
	}
	return segments, rootProof, true, directStandalone, "", nil
}

func isProfilerTraceHeaderPrefix(first, second uint32) bool {
	return first == uint32(profilerTraceHeaderMagic&0xffffffff) && second == uint32((profilerTraceHeaderMagic>>32)&0xffffffff)
}

func validateProfilerStandaloneChain(ctx context.Context, reader io.ReaderAt, fileSize, start int64,
	allowOfficialZero bool, layoutAuthority string,
) ([]standaloneSegment, string, error) {
	if start < 0 || start > fileSize {
		return nil, "standalone_chain_start_invalid", nil
	}
	if start == fileSize {
		return nil, "", nil
	}
	type candidate struct {
		offset int64
		header profilerStandaloneHeader
	}
	candidates := make([]candidate, 0, 4)
	hiperfCandidates := 0
	// Phase 1 is header-only. Close the complete physical chain, budgets and
	// non-overlapping byte ranges before hashing even one payload byte. This
	// prevents a cap+1 header from turning the pre-admission census into an
	// attacker-controlled large-I/O amplifier.
	for off := start; off < fileSize; {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		header, ok, failure, err := readCanonicalProfilerStandaloneHeaderAt(reader, off, fileSize)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, firstNonEmpty(failure, "standalone_chain_invalid"), nil
		}
		// The cap applies to a confirmed 257th physical block, not arbitrary
		// bytes after 256 valid blocks. Read only its fixed header first so a
		// short tail/bad magic remains an integrity failure while cap+1 still
		// closes before any payload hash.
		if len(candidates) >= maxProfilerStandaloneBlocks {
			return nil, "standalone_block_budget_exceeded", nil
		}
		if allZeroBytes(header.DeclaredSHA[:]) && !allowOfficialZero {
			return nil, "standalone_zero_sha_requires_verified_root", nil
		}
		perfEligible := header.DataType == profilerDataTypeHiperf && header.PluginName == "hiperf-plugin"
		if perfEligible {
			hiperfCandidates++
			if hiperfCandidates > maxProfilerHiperfCandidates {
				return nil, "standalone_hiperf_budget_exceeded", nil
			}
		}
		candidates = append(candidates, candidate{offset: off, header: header})
		off += header.Length
	}

	// Phase 2 seals payload integrity and mints the typed inventory only after
	// the phase-1 cursor proved an exact start-to-EOF partition.
	segments := make([]standaloneSegment, 0, len(candidates))
	for _, item := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		segment, failure, err := sealProfilerStandaloneSegment(
			ctx, reader, item.offset, item.header)
		if err != nil {
			return nil, "", err
		}
		if failure != "" {
			return nil, failure, nil
		}
		segment.Layout = layoutAuthority
		segments = append(segments, segment)
	}
	return segments, "", nil
}

func sealProfilerStandaloneSegment(ctx context.Context, reader io.ReaderAt, off int64,
	header profilerStandaloneHeader,
) (standaloneSegment, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	length := header.Length
	payloadOff := off + profilerStandalonePayloadBase
	payloadBytes := length - profilerStandalonePayloadBase
	integrity := standaloneIntegrityOfficialZero
	payloadDigest, err := hashProfilerStandalonePayload(ctx, reader, payloadOff, payloadBytes)
	if err != nil {
		return standaloneSegment{}, "", err
	}
	if !allZeroBytes(header.DeclaredSHA[:]) {
		if header.DeclaredSHA != payloadDigest {
			return standaloneSegment{}, "standalone_payload_sha256_mismatch", nil
		}
		integrity = standaloneIntegrityPayloadSHA256
	}
	return standaloneSegment{
		Offset:        off,
		Length:        length,
		DataType:      header.DataType,
		PluginName:    header.PluginName,
		PluginVersion: header.PluginVersion,
		Integrity:     integrity,
		PayloadSHA256: payloadDigest,
		PerfEligible:  header.DataType == profilerDataTypeHiperf && header.PluginName == "hiperf-plugin",
	}, "", nil
}

func readCanonicalProfilerStandaloneHeaderAt(reader io.ReaderAt, off, fileSize int64) (
	profilerStandaloneHeader, bool, string, error,
) {
	if reader == nil || off < 0 || off > fileSize || int64(profilerTraceHeaderSize) > fileSize-off {
		return profilerStandaloneHeader{}, false, "standalone_header_truncated", nil
	}
	raw := make([]byte, profilerTraceHeaderSize)
	if _, err := io.ReadFull(io.NewSectionReader(reader, off, profilerTraceHeaderSize), raw); err != nil {
		return profilerStandaloneHeader{}, false, "", err
	}
	if binary.LittleEndian.Uint64(raw[0:8]) != profilerTraceHeaderMagic {
		return profilerStandaloneHeader{}, false, "standalone_magic_mismatch", nil
	}
	declaredLength := binary.LittleEndian.Uint64(raw[8:16])
	if declaredLength < profilerTraceHeaderSize || declaredLength > math.MaxInt64 ||
		declaredLength > uint64(fileSize-off) {
		return profilerStandaloneHeader{}, false, "standalone_declared_length_invalid", nil
	}
	if binary.LittleEndian.Uint32(raw[16:20]) != profilerTraceVersionV1 {
		return profilerStandaloneHeader{}, false, "standalone_version_unsupported", nil
	}
	if binary.LittleEndian.Uint32(raw[20:24]) != 0 {
		return profilerStandaloneHeader{}, false, "standalone_segments_nonzero", nil
	}
	dataType := binary.LittleEndian.Uint32(raw[56:60])
	if !isProfilerStandaloneDataType(dataType) {
		return profilerStandaloneHeader{}, false, "standalone_data_type_unsupported", nil
	}
	pluginName, nameOK := canonicalProfilerHeaderCString(
		raw[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize])
	pluginVersion, versionOK := canonicalProfilerHeaderCString(
		raw[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize])
	if !nameOK || !versionOK || !allZeroBytes(raw[60:profilerPluginNameOffset]) ||
		!allZeroBytes(raw[profilerPluginVersionOffset+profilerPluginVersionSize:]) {
		return profilerStandaloneHeader{}, false, "standalone_reserved_header_noncanonical", nil
	}
	var declaredSHA [sha256.Size]byte
	copy(declaredSHA[:], raw[24:56])
	return profilerStandaloneHeader{
		Length:        int64(declaredLength),
		DataType:      dataType,
		PluginName:    pluginName,
		PluginVersion: pluginVersion,
		DeclaredSHA:   declaredSHA,
	}, true, "", nil
}

func hashProfilerStandalonePayload(ctx context.Context, reader io.ReaderAt, off, size int64) ([sha256.Size]byte, error) {
	hasher := sha256.New()
	const scratchBytes = 256 * 1024
	scratch := make([]byte, scratchBytes)
	left := size
	for left > 0 {
		if err := ctx.Err(); err != nil {
			return [sha256.Size]byte{}, err
		}
		chunk := int64(len(scratch))
		if left < chunk {
			chunk = left
		}
		part := scratch[:int(chunk)]
		if _, err := io.ReadFull(io.NewSectionReader(reader, off, chunk), part); err != nil {
			return [sha256.Size]byte{}, err
		}
		_, _ = hasher.Write(part)
		off += chunk
		left -= chunk
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func canonicalProfilerHeaderCString(field []byte) (string, bool) {
	nul := bytes.IndexByte(field, 0)
	if nul < 0 || !allZeroBytes(field[nul+1:]) || !utf8.Valid(field[:nul]) {
		return "", false
	}
	for _, value := range field[:nul] {
		if value < 0x20 || value == 0x7f {
			return "", false
		}
	}
	return string(field[:nul]), true
}

func allZeroBytes(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
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
		ArchiveProvenance:   cloneTraceArchiveProvenance(ledger.archive),
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
			if publicArtifact.Standalone != nil || publicArtifact.standaloneReceipt != nil {
				return nil, "", nil, fmt.Errorf(
					"receipt-backed standalone raw artifact has an empty public path")
			}
			continue
		}
		bindingPath := originalPath
		var systraceClaim publishedOwnedTraceValidation
		var perfClaim publishedOwnedTraceValidation
		var standaloneClaim publishedStandaloneSourceReceipt
		standaloneClaimPresent := false
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
		if publicArtifact.Type == ArtifactPerfData &&
			(publicArtifact.Standalone != nil || publicArtifact.standaloneReceipt != nil) {
			if ledger == nil {
				return nil, "", nil, fmt.Errorf(
					"conversion file ledger is required to bind standalone raw artifact %s", originalPath)
			}
			standaloneClaim, standaloneClaimPresent =
				ledger.standaloneSourceReceiptForArtifactPath(publicArtifact.Path)
			if !standaloneClaimPresent {
				return nil, "", nil, fmt.Errorf(
					"raw perf.data artifact has no ledger-bound standalone source receipt")
			}
			bindingPath = standaloneClaim.segment.BindingPath
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
		if publicArtifact.Type == ArtifactPerfData &&
			(publicArtifact.Standalone != nil || publicArtifact.standaloneReceipt != nil) {
			if !standaloneClaimPresent {
				return nil, "", nil, fmt.Errorf("standalone raw artifact lost its ledger receipt")
			}
			if err := tracebundle.ValidateCapturePath(wirePath); err != nil {
				return nil, "", nil, fmt.Errorf("standalone tracebundle artifact %q: %w", originalPath, err)
			}
			info, err := ledger.sealedOwnedFileInfo(bindingPath)
			if err != nil {
				return nil, "", nil, fmt.Errorf("bind standalone raw artifact %q: %w", originalPath, err)
			}
			for _, prior := range physical {
				if os.SameFile(prior, info) {
					return nil, "", nil, fmt.Errorf("standalone raw artifact %q duplicates a physical child generation", originalPath)
				}
			}
			physical = append(physical, info)
			measuredBytes, measuredSHA, held, err := ledger.holdAndMeasureSealedOwnedPath(ctx, bindingPath)
			if err != nil {
				return nil, "", nil, fmt.Errorf("measure standalone raw artifact %q: %w", originalPath, err)
			}
			heldChildren = append(heldChildren, held)
			if !standaloneClaim.publishedIdentity.SameVersion(held.sealedIdentity) {
				return nil, "", nil, fmt.Errorf(
					"standalone raw artifact generation differs from its ledger receipt")
			}
			if err := validateStandaloneRawArtifactClaim(
				publicArtifact, standaloneClaim.segment, measuredBytes, measuredSHA); err != nil {
				return nil, "", nil, fmt.Errorf("bind standalone raw artifact %q: %w", originalPath, err)
			}
			out[i].Bytes = measuredBytes
			out[i].SHA256 = measuredSHA
			continue
		}
		if publicArtifact.Standalone != nil || publicArtifact.standaloneReceipt != nil {
			return nil, "", nil, fmt.Errorf("standalone provenance is only valid on a receipt-bound raw perf.data artifact %q", originalPath)
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

func validateStandaloneSourceReceiptToken(receipt standaloneSegment, publishedBytes int64) error {
	if publishedBytes < 0 || receipt.Offset < 0 || receipt.Length < profilerStandalonePayloadBase ||
		receipt.Offset > math.MaxInt64-receipt.Length ||
		receipt.Length-profilerStandalonePayloadBase != publishedBytes ||
		receipt.DataType != profilerDataTypeHiperf || receipt.PluginName != "hiperf-plugin" ||
		!receipt.PerfEligible || !receipt.PerfInputFormat.valid() ||
		receipt.ArtifactPath == "" || strings.TrimSpace(receipt.ArtifactPath) != receipt.ArtifactPath ||
		receipt.BindingPath == "" || strings.TrimSpace(receipt.BindingPath) != receipt.BindingPath ||
		!filepath.IsAbs(receipt.BindingPath) || filepath.Clean(receipt.BindingPath) != receipt.BindingPath {
		return fmt.Errorf("standalone source receipt token is incomplete")
	}
	switch receipt.Layout {
	case standaloneLayoutDirectOffsetZero, standaloneLayoutRootProfile, standaloneLayoutRootEnvelope:
	default:
		return fmt.Errorf("standalone source receipt has an unknown layout authority")
	}
	switch receipt.Integrity {
	case standaloneIntegrityPayloadSHA256:
	case standaloneIntegrityOfficialZero:
		if receipt.Layout != standaloneLayoutRootProfile {
			return fmt.Errorf("zero-SHA standalone requires an authenticated root profile authority")
		}
	default:
		return fmt.Errorf("standalone source receipt has an unknown writer profile")
	}
	return nil
}

func validateStandaloneRawArtifactClaim(artifact Artifact, authority standaloneSegment,
	measuredBytes int64, measuredSHA string,
) error {
	receipt := artifact.standaloneReceipt
	provenance := artifact.Standalone
	if receipt == nil || !reflect.DeepEqual(*receipt, authority) || provenance == nil ||
		validateStandaloneSourceReceiptToken(authority, measuredBytes) != nil ||
		artifact.Type != ArtifactPerfData ||
		artifact.Trace != nil || artifact.Converter != converterVersion ||
		artifact.Path == "" || strings.TrimSpace(artifact.Path) != artifact.Path ||
		artifact.Path != authority.ArtifactPath ||
		artifact.DataType != profilerDataTypeHiperf ||
		!reflect.DeepEqual(artifact.Perf, perfCapabilityForRawPerfDataArtifact(authority.PerfInputFormat)) ||
		artifact.PluginName != "hiperf-plugin" || !authority.PerfEligible ||
		authority.DataType != artifact.DataType || authority.PluginName != artifact.PluginName ||
		authority.PluginVersion != artifact.PluginVersion || authority.Offset != artifact.SourceOffset ||
		authority.Length != artifact.SourceBytes ||
		artifact.Bytes != authority.Length-profilerStandalonePayloadBase || artifact.Bytes != measuredBytes ||
		artifact.SHA256 != measuredSHA || artifact.SHA256 != hex.EncodeToString(authority.PayloadSHA256[:]) ||
		provenance.Profile != "openharmony_standalone_v1" ||
		provenance.LayoutAuthority != authority.Layout || provenance.WriterProfile != authority.Integrity {
		return fmt.Errorf("raw perf.data artifact does not match its standalone source receipt")
	}
	return nil
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
