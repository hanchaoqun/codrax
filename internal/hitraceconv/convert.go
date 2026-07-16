package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const tracePerfAutoFallbackCaveat = "trace+perf auto conversion uses trace_streamer/SQLite first and falls back to built-in raw trace parsing when SQL is unavailable or fails; explicit trace engine modes do not fall back"

type builtinInputBinding struct {
	input           conversionInputView
	inputSize       int64
	sourceNamespace string
}

type traceMetadata struct {
	binding              *builtinInputBinding
	header               fileHeader
	formats              map[int]eventFormat
	formatPoisoned       map[int]bool
	formatPoisonFamilies map[int]pairCriticalFormatFamilyMask
	formatConflictRows   int
	printkFormats        map[uint64]string
	printkPoisoned       map[uint64]bool
	printkMalformed      int
	bodyRejectedRows     int
	bodyRejectReasons    map[string]int
	pairQuarantinedRows  int
	pairPoisonedLanes    int
	pairPoisonedFamilies int
	pairBarrierBudget    bool
	pairBarrierReport    directPairBarrierReport
	cmdlines             map[int]string
	tgids                map[int]int
	segments             []segmentMeta
}

// builtinRowProvenance is a closed producer-side classification used only by
// the built-in RMQ writer. It is carried with the rendered bytes until the
// final physical ordering is known, then consumed into exact validation-row
// digests. Shared SQL/Profiler sorters must reject it before compaction.
type builtinRowProvenance uint8

const (
	builtinRowProvenanceNone builtinRowProvenance = iota
	builtinRowProvenanceOpaqueMarkerAdvisory
	builtinRowProvenanceIntentionalHeaderOnly
)

func (provenance builtinRowProvenance) valid() bool {
	return provenance == builtinRowProvenanceNone ||
		provenance == builtinRowProvenanceOpaqueMarkerAdvisory ||
		provenance == builtinRowProvenanceIntentionalHeaderOnly
}

type renderedRow struct {
	tsNS                       uint64
	seq                        int
	line                       string
	pairKind                   pairRenderKind
	profilerEndpointSlot       profilerPairEndpointSlot
	profilerPublisherSlot      profilerPairPublisherSlot
	profilerProvenanceFlags    profilerPairRowProvenanceFlags
	profilerLaneID             uint32
	pairLane                   string
	pairTable                  string
	structuredPair             bool
	builtinProvenance          builtinRowProvenance
	profilerTraceClass         profilerTraceClass
	profilerTextMessageOrdinal uint32
	// profilerEventField is the exact FtraceEvent oneof field which produced a
	// structured pair-critical row. Text-compatible and all other rows leave it
	// zero; the row sink rejects a non-structured row carrying this provenance.
	profilerEventField int
}

func (row renderedRow) profilerProvenance() profilerPairRowProvenance {
	return profilerPairRowProvenance{
		LaneID: row.profilerLaneID, TextMessageOrdinal: row.profilerTextMessageOrdinal,
		PairKind: row.pairKind, EndpointSlot: row.profilerEndpointSlot,
		PublisherSlot: row.profilerPublisherSlot, Flags: row.profilerProvenanceFlags,
		TraceClass: row.profilerTraceClass,
	}
}

func (row renderedRow) profilerNeutral() bool {
	return row.pairKind == pairRenderUnknown &&
		row.profilerEndpointSlot == profilerPairEndpointNone &&
		row.profilerPublisherSlot == profilerPairPublisherNone &&
		row.profilerProvenanceFlags == 0 && row.profilerTraceClass == profilerTraceClassNone && row.profilerLaneID == 0 &&
		row.pairLane == "" && row.pairTable == "" && !row.structuredPair &&
		row.builtinProvenance == builtinRowProvenanceNone &&
		row.profilerTextMessageOrdinal == 0 && row.profilerEventField == 0
}

// traceDBStoredRow is the only row shape retained by the bounded sorter and
// written to authenticated runs. Raw lane/table/field aliases exist only while
// renderedRow is admitted; after the closed provenance tuple is staged they
// must not become a second semantic authority in memory or on disk.
type traceDBStoredRow struct {
	tsNS       uint64
	seq        int
	line       string
	provenance profilerPairRowProvenance
}

func compactTraceDBStoredRow(row renderedRow) traceDBStoredRow {
	return traceDBStoredRow{
		tsNS: row.tsNS, seq: row.seq, line: row.line, provenance: row.profilerProvenance(),
	}
}

func (row traceDBStoredRow) profilerProvenance() profilerPairRowProvenance {
	return row.provenance
}

// ConvertFile converts a binary Harmony/OpenHarmony HiTrace capture to a
// ftrace/systrace-compatible text file. It never overwrites the output path.
func ConvertFile(ctx context.Context, opts Options) (result Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	input := strings.TrimSpace(opts.InputPath)
	if input == "" {
		return Result{}, fmt.Errorf("input path is required")
	}
	if err := validateOptionEnums(opts); err != nil {
		return Result{}, err
	}
	authority, err := openConversionInputAuthority(input)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if closeErr := authority.Close(); closeErr != nil {
			result = Result{}
			err = traceDBJoinPreservingSingle(err, closeErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	input = authority.DisplayPath()
	opts.InputPath = input
	inputBytes := authority.Size()
	probe, err := authority.Probe()
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	inputFormat := detectPerfInputFormatProbe(probe)
	directPerf, err := validateOptionsForInput(opts, authority, inputFormat)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(input)
	}
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		return Result{}, err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	commit := func(completed Result, completionErr error) (Result, error) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		if completionErr == nil {
			if validateErr := authority.Validate(conversionInputStagePreCommit); validateErr != nil {
				return Result{}, validateErr
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{}, ctxErr
			}
			if closeErr := authority.Close(); closeErr != nil {
				return Result{}, traceDBJoinPreservingSingle(ctx.Err(), closeErr)
			}
			if validateErr := ledger.validateOwnedPaths(); validateErr != nil {
				return Result{}, validateErr
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{}, ctxErr
			}
			if releaseErr := ledger.releaseOwnedAuthorities(); releaseErr != nil {
				return Result{}, fmt.Errorf("release conversion publication authorities: %w", releaseErr)
			}
			committed = true
		}
		return completed, completionErr
	}
	mode := requestedTraceEngineMode(opts.TraceEngine)
	if directPerf {
		directPlan, err := buildTraceProviderPlanWithInput(opts, false, true)
		if err != nil {
			return Result{}, err
		}
		if directPlan.ExecutionBlocker != "" {
			return Result{}, errors.New(directPlan.ExecutionBlocker)
		}
		if err := preflightTracePublicationPaths(opts, input, output, false); err != nil {
			return Result{}, err
		}
		if !directPlan.DirectPerf || directPlan.PreflightEngine != traceEngineDirectPerf {
			return Result{}, fmt.Errorf("typed trace provider plan did not select the direct perf route")
		}
		directInput, err := newDirectPerfInputBinding(authority, inputFormat)
		if err != nil {
			return Result{}, err
		}
		if result, ok, err := maybeConvertDirectSimpleperfPerfData(ctx, opts, directPlan, directInput, output, ledger); ok || err != nil {
			if err != nil {
				return result, err
			}
			return commit(result, nil)
		}
		return Result{}, fmt.Errorf("direct perf input classification was not consumed by the perf conversion lane")
	}
	if err := preflightTracePublicationPaths(opts, input, output, true); err != nil {
		return Result{}, err
	}
	plan, err := buildTraceProviderPlanWithInput(opts, false, directPerf)
	if err != nil {
		return Result{}, err
	}
	standaloneInventory, err := findStandaloneSegmentsFromInput(ctx, authority)
	if err != nil {
		return Result{}, err
	}
	if mode == traceEngineTraceStreamer {
		converted, convertErr := convertTraceStreamerOnly(ctx, opts, plan, standaloneInventory, output, ledger)
		return commit(converted, convertErr)
	}
	hasTracePerfSidecar := standaloneInventory.hasHiperfData()
	traceStreamerExport, err := maybeRunTraceStreamerAuto(ctx, opts, plan, authority, output, hasTracePerfSidecar, ledger)
	if err != nil {
		return Result{}, err
	}
	wrapFallbackFailure := func(fallback error) error {
		return traceProviderFallbackFailure(plan, traceStreamerExport, fallback)
	}
	var initialArtifacts []Artifact
	var initialCaveats []string
	var initialTraceDecisions []TraceProviderDecision
	var initialTraceDBCoverage []TraceDBCoverage
	var initialTraceCoverage []TraceDBCoverage
	if traceStreamerExport.Decision.ProviderName != "" {
		initialTraceDecisions = append(initialTraceDecisions, traceStreamerExport.Decision)
		initialCaveats = append(initialCaveats, traceStreamerExport.Caveats...)
		initialTraceDBCoverage = append(initialTraceDBCoverage, traceStreamerExport.Coverage...)
		initialTraceCoverage = append(initialTraceCoverage, traceStreamerExport.TraceCoverage...)
	}
	keepTraceDBArtifact := opts.KeepTraceDB || strings.TrimSpace(opts.TraceDBOutputPath) != "" ||
		(hasTracePerfSidecar && traceStreamerExport.Artifact.Path != "" && traceStreamerExport.SystraceArtifact.Path == "")
	if traceStreamerExport.Artifact.Path != "" && keepTraceDBArtifact && artifactPathExists(traceStreamerExport.Artifact.Path) {
		initialArtifacts = append(initialArtifacts, traceStreamerExport.Artifact)
	}
	standaloneExtractOpts := standaloneExtractOptions{GeneratePerfTrace: true}
	if traceStreamerExport.SystraceArtifact.Path != "" && traceDBCoverageHasPerfSamples(initialTraceDBCoverage) {
		standaloneExtractOpts.GeneratePerfTrace = false
		standaloneExtractOpts.PrimaryPerfSource = "trace_streamer DB perf_sample rows embedded in systrace"
	}
	extractedStandaloneArtifacts, standaloneCaveats, standaloneDecisions, err := extractStandaloneArtifactsWithOptionsAndLedger(ctx, opts, standaloneInventory, output, standaloneExtractOpts, ledger)
	if err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	standaloneArtifacts := append(initialArtifacts, extractedStandaloneArtifacts...)
	standaloneCaveats = append(initialCaveats, standaloneCaveats...)
	if traceStreamerExport.SystraceArtifact.Path != "" {
		result := Result{
			InputPath:          input,
			OutputPath:         traceStreamerExport.SystraceArtifact.Path,
			InputBytes:         inputBytes,
			OutputBytes:        traceStreamerExport.OutputBytes,
			Artifacts:          append([]Artifact{traceStreamerExport.SystraceArtifact}, standaloneArtifacts...),
			ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
			TraceDecisions:     append([]TraceProviderDecision(nil), initialTraceDecisions...),
			TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
			TraceCoverage:      append([]TraceDBCoverage(nil), initialTraceCoverage...),
			Caveats:            standaloneCaveats,
			EventsWritten:      traceStreamerExport.EventsWritten,
			FirstTimestampSec:  traceStreamerExport.FirstTimestampSec,
			LastTimestampSec:   traceStreamerExport.LastTimestampSec,
			MissingFormatCount: 0,
			UnknownEventCount:  0,
		}
		if err := finalizeResultTraceBundleWithLedger(ctx, input, result.OutputPath, &result, ledger); err != nil {
			return Result{}, wrapFallbackFailure(err)
		}
		return commit(result, nil)
	}
	if traceStreamerSelectionShouldStopBuiltinFallback(plan, traceStreamerExport) {
		result := Result{
			InputPath:         input,
			InputBytes:        inputBytes,
			Artifacts:         append([]Artifact(nil), standaloneArtifacts...),
			ProviderDecisions: append([]PerfProviderDecision(nil), standaloneDecisions...),
			TraceDecisions:    append([]TraceProviderDecision(nil), initialTraceDecisions...),
			TraceDBCoverage:   append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
			TraceCoverage:     append([]TraceDBCoverage(nil), initialTraceCoverage...),
			Caveats:           append(append([]string(nil), initialCaveats...), standaloneCaveats...),
		}
		result.Artifacts = append(initialArtifacts, result.Artifacts...)
		result.Caveats = append(result.Caveats, "systrace output was not produced because selected trace_streamer/SQLite trace conversion did not produce trace_query-ready rows; pass --trace-engine=builtin to use the built-in trace-only converter")
		if err := finalizeResultTraceBundleWithLedger(ctx, input, output, &result, ledger); err != nil {
			return Result{}, err
		}
		return commit(result, nil)
	}
	if result, ok, err := tryConvertProfilerContainerWithLedger(ctx, opts, authority, output, standaloneArtifacts, standaloneCaveats, standaloneDecisions, initialTraceDecisions, initialTraceDBCoverage, ledger); ok || err != nil {
		if err != nil {
			return result, wrapFallbackFailure(err)
		}
		return commit(result, nil)
	}
	meta, err := scanMetadata(ctx, authority, authority.CanonicalPath())
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Result{}, err
		}
		var inputErr *ConversionInputError
		if errors.As(err, &inputErr) {
			return Result{}, err
		}
		fallbackErr := traceProviderFallbackFailure(plan, traceStreamerExport, err)
		analyzableSidecar, sidecarErr := hasAnalyzableStandaloneSidecar(ctx, standaloneArtifacts, ledger)
		if sidecarErr != nil {
			return Result{}, wrapFallbackFailure(sidecarErr)
		}
		if analyzableSidecar {
			fallbackCaveat := builtinTraceBodyFallbackFailureCaveat(authority, standaloneArtifacts, err)
			if validateErr := completeConversionInputStage(ctx, authority, conversionInputStageBuiltinMetadata, nil); validateErr != nil {
				return Result{}, validateErr
			}
			result := Result{
				InputPath:  input,
				InputBytes: inputBytes,
				Artifacts:  append([]Artifact(nil), standaloneArtifacts...),
				ProviderDecisions: append([]PerfProviderDecision(nil),
					standaloneDecisions...),
				TraceDecisions: append(initialTraceDecisions,
					traceProviderFailure(
						newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys), opts, input, output),
						builtinTraceBodyFailureReason(err),
						fallbackCaveat,
					),
				),
				Caveats: append(standaloneCaveats,
					fallbackCaveat),
				TraceDBCoverage: append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
				TraceCoverage:   append([]TraceDBCoverage(nil), initialTraceCoverage...),
			}
			if bundleErr := finalizeResultTraceBundleWithLedger(ctx, input, "", &result, ledger); bundleErr != nil {
				return Result{}, wrapFallbackFailure(errors.Join(err, bundleErr))
			}
			return commit(result, nil)
		}
		return Result{}, fallbackErr
	}
	rows, missing, _, suppressed, first, last, err := renderRows(ctx, meta)
	if err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].tsNS == rows[j].tsNS {
			return rows[i].seq < rows[j].seq
		}
		return rows[i].tsNS < rows[j].tsNS
	})
	if err := ctx.Err(); err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	publication, err := writeValidatedOwnedBuiltinSystraceWithLedger(ctx, output, rows, ledger)
	if err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	artifact := publication.Artifact
	if artifact.Trace == nil {
		return Result{}, wrapFallbackFailure(newOwnedTracePublicationError(
			"consume_public_receipt", artifact.Path, errors.New("builtin systrace artifact has no receipt capability"),
		))
	}
	decision, err := traceProviderPublished(
		newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinSys), opts, input, artifact.Path),
		artifact,
		ledger,
	)
	if err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	result = Result{
		InputPath:          input,
		OutputPath:         artifact.Path,
		InputBytes:         inputBytes,
		OutputBytes:        artifact.Bytes,
		Artifacts:          []Artifact{artifact},
		TraceDecisions:     append(initialTraceDecisions, decision),
		EventsWritten:      artifact.Trace.Rows,
		MissingFormatCount: missing,
		UnknownEventCount:  artifact.Trace.IntentionalHeaderOnly,
		FirstTimestampSec:  float64(first) / 1e9,
		LastTimestampSec:   float64(last) / 1e9,
		TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
		TraceCoverage:      append(append([]TraceDBCoverage(nil), initialTraceCoverage...), publication.TraceCoverage),
	}
	result.Artifacts = append(result.Artifacts, standaloneArtifacts...)
	result.ProviderDecisions = append(result.ProviderDecisions, standaloneDecisions...)
	if missing > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw event(s) had no event format and were skipped to keep systrace output compatible with official parsers", missing))
	}
	if artifact.Trace.IntentionalHeaderOnly > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d event row(s) lacked an official-compatible renderer and were emitted as header-only rows", artifact.Trace.IntentionalHeaderOnly))
	}
	if artifact.Trace.IntentionalUnknown > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d governed print event row(s) retained opaque advisory payloads as EventUnknown without granting trace-query readiness", artifact.Trace.IntentionalUnknown))
	}
	if suppressed > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw ftrace event row(s) had a missing, mistyped, duplicate, or truncated common-field envelope and were suppressed without fabricating an idle/PID0 header", suppressed))
	}
	if len(meta.formatPoisoned) > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d conflicting or malformed raw ftrace event descriptor ID(s) were quarantined for the complete capture; separate exactly equal parsed descriptor blocks are idempotent, but repetition inside one block is malformed and later descriptors cannot rescue a quarantined ID", len(meta.formatPoisoned)))
	}
	if meta.formatConflictRows > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d raw ftrace event row(s) referenced a conflicting or malformed descriptor ID and were kept coverage-only instead of being decoded with an ambiguous layout", meta.formatConflictRows))
	}
	if len(meta.printkPoisoned) > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d conflicting or malformed printk address mapping(s) were quarantined for the complete capture; later mappings cannot rescue a quarantined address", len(meta.printkPoisoned)))
	}
	if meta.printkMalformed > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d printk_formats line(s) were malformed without a usable address and were ignored", meta.printkMalformed))
	}
	if meta.bodyRejectedRows > 0 {
		result.Caveats = append(result.Caveats, fmt.Sprintf("%d governed direct ftrace event row(s) had rejected physical payloads and were kept coverage-only instead of falling back to header-only rows; reasons=%s", meta.bodyRejectedRows, traceDBCountSummary(meta.bodyRejectReasons)))
	}
	if meta.pairQuarantinedRows > 0 || meta.pairPoisonedFamilies > 0 || meta.pairPoisonedLanes > 0 {
		report := meta.pairBarrierReport
		result.Caveats = append(result.Caveats, fmt.Sprintf("direct pair-critical publication completed a full-capture anti-rescue freeze before output: withheld_rows=%d poisoned_lanes=%d poisoned_families=%d budget_fail_closed=%t legacy_budget_reason=%s block_budget_reason=%s shared_authority_reason=%s; rejected Workqueue/DMA/MMC/F2FS/Block endpoints remain coverage-only and cannot be bridged by neighboring rows",
			meta.pairQuarantinedRows, meta.pairPoisonedLanes, meta.pairPoisonedFamilies, meta.pairBarrierBudget,
			firstNonEmpty(report.LegacyBudgetReason, "none"), firstNonEmpty(report.BlockBudgetReason, "none"),
			firstNonEmpty(report.SharedAuthorityReason, "none")))
	}
	if report := meta.pairBarrierReport; report.BlockObserved > 0 {
		emitted := report.BlockRowsStaged - report.BlockRowsWithheld
		fields := map[string]string{
			"scope": "source_local", "proof_domain": "block", "rows_staged": strconv.Itoa(report.BlockRowsStaged),
			"rows_withheld": strconv.Itoa(report.BlockRowsWithheld), "budget_reason": firstNonEmpty(report.BlockBudgetReason, "none"),
			"shared_authority_reason": firstNonEmpty(report.SharedAuthorityReason, "none"),
		}
		result.TraceCoverage = append(result.TraceCoverage, TraceDBCoverage{
			Family: "builtin_raw_ftrace:block_capture", Table: "__complete_capture_barrier__", Role: "unsupported_input",
			Found: true, RowsRead: int(report.BlockObserved), RowsEmitted: emitted, FieldSources: fields,
		})
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	if err := finalizeResultTraceBundleWithLedger(ctx, input, output, &result, ledger); err != nil {
		return Result{}, wrapFallbackFailure(err)
	}
	return commit(result, nil)
}

func builtinTraceBodyFallbackFailureCaveat(input conversionInputView, artifacts []Artifact, err error) string {
	reasons := builtinTraceBodyFallbackReasons(input, artifacts)
	if len(reasons) == 0 {
		reasons = append(reasons, "no supported trace body was found")
	}
	message := "systrace output was not produced because " + strings.Join(reasons, "; ")
	if err != nil {
		message += fmt.Sprintf("; built-in sys parser rejected the fallback probe: %v", err)
	}
	return message
}

func builtinTraceBodyFallbackReasons(input conversionInputView, artifacts []Artifact) []string {
	var reasons []string
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfData || artifact.SourceOffset != 0 {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("input starts with OpenHarmony profiler standalone perf sidecar data_type=%d plugin=%s version=%s bytes=%d rather than a trace body", artifact.DataType, firstNonEmpty(artifact.PluginName, "unknown"), firstNonEmpty(artifact.PluginVersion, "unknown"), artifact.SourceBytes))
		break
	}
	if off, ok, err := profilerSessionJSONMarkerOffsetAt(input, input.Size(), 64*1024); err == nil && ok {
		reasons = append(reasons, fmt.Sprintf("input contains OpenHarmony profiler SessionJSON package marker at offset %d but no directly renderable systrace rows were extracted before fallback", off))
	}
	if len(reasons) == 0 {
		perfSidecars := 0
		for _, artifact := range artifacts {
			if artifact.Type == ArtifactPerfData {
				perfSidecars++
			}
		}
		if perfSidecars > 0 {
			reasons = append(reasons, fmt.Sprintf("%d standalone perf sidecar artifact(s) were preserved, but no supported trace body was found", perfSidecars))
		}
	}
	return reasons
}

func traceDBCoverageHasPerfSamples(coverage []TraceDBCoverage) bool {
	for _, item := range coverage {
		if strings.EqualFold(strings.TrimSpace(item.Family), "perf") &&
			strings.EqualFold(strings.TrimSpace(item.Table), "perf_sample") &&
			item.RowsEmitted > 0 {
			return true
		}
	}
	return false
}

func traceStreamerSelectionShouldStopBuiltinFallback(plan traceProviderPlan, traceStreamerExport traceStreamerExportResult) bool {
	return plan.includesEngine(traceEngineTraceStreamer) && !plan.allowsBuiltinFallback()
}

func traceProviderFallbackFailure(plan traceProviderPlan, first traceStreamerExportResult, fallback error) error {
	if errors.Is(fallback, context.Canceled) || errors.Is(fallback, context.DeadlineExceeded) {
		return fallback
	}
	var inputErr *ConversionInputError
	if errors.As(fallback, &inputErr) {
		return fallback
	}
	if !plan.allowsBuiltinFallback() || first.Decision.ProviderName == "" || first.Decision.Succeeded || strings.TrimSpace(first.Decision.Reason) == "" {
		return fallback
	}
	firstDecision := first.Decision
	rolledBackDB := strings.TrimSpace(firstDecision.DBPath)
	firstDecision.DBPath = ""
	return &TraceProviderFallbackError{
		FirstDecision: firstDecision,
		FirstSource:   plan.TraceStreamer.Source,
		FirstPath:     plan.TraceStreamer.Path,
		FirstStage:    first.FailureStage,
		FirstCode:     first.FailureCode,
		FirstCaveats:  append([]string(nil), first.Caveats...),
		FirstCause:    first.Cause,
		RolledBackDB:  rolledBackDB,
		Fallback:      fallback,
	}
}

func builtinTraceBodyFailureReason(err error) string {
	var decodeErr *BuiltinSysDecodeError
	if errors.As(err, &decodeErr) && strings.TrimSpace(decodeErr.Code) != "" {
		return "builtin_sys_decode_" + decodeErr.Code
	}
	return "no_renderable_trace_body"
}

func hasAnalyzableStandaloneSidecar(ctx context.Context, artifacts []Artifact, ledger *conversionFileLedger) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ledger == nil {
		return false, newOwnedTracePublicationError("consume_standalone_receipt", "", fmt.Errorf("standalone sidecar ledger is unavailable"))
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfTrace {
			continue
		}
		if artifact.Perf == nil {
			return false, newOwnedTracePublicationError("consume_standalone_receipt", artifact.Path, fmt.Errorf("standalone perf capability is absent"))
		}
		profile, ok := ownedTracePerfProfileForProvider(artifact.Perf.ProviderName)
		spec, closed := profile.claimSpec()
		if !ok || !closed || artifact.Perf.ProviderName != spec.providerName {
			return false, newOwnedTracePublicationError("consume_standalone_receipt", artifact.Path, fmt.Errorf("standalone perf profile is not closed"))
		}
		if _, err := validateOwnedPerfTraceArtifactClaim(ledger, artifact, profile); err != nil {
			return false, err
		}
		if err := ledger.validateSealedOwnedPath(ctx, artifact.Path); err != nil {
			return false, newOwnedTracePublicationError("consume_standalone_receipt", artifact.Path, err)
		}
		found = true
	}
	return found, nil
}

func scanMetadata(ctx context.Context, input conversionInputView, sourceNamespace string) (meta *traceMetadata, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageBuiltinMetadata, nil); err != nil {
		return nil, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageBuiltinMetadata, err)
		if err != nil {
			meta = nil
		}
	}()
	size := input.Size()
	if size < 0 {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageBuiltinMetadata,
			input.DisplayPath(),
			fmt.Errorf("negative conversion input size %d", size),
		)
	}
	if !directPairSourceNamespaceValid(sourceNamespace) {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageBuiltinMetadata,
			input.DisplayPath(),
			fmt.Errorf("invalid frozen direct-pair source namespace"),
		)
	}
	reader := io.NewSectionReader(input, 0, size)
	header, err := readFileHeader(reader)
	if err != nil {
		return nil, err
	}
	if header.Magic != harmonyRMQMagic {
		return nil, &BuiltinSysDecodeError{
			Code:   builtinSysDecodeInvalidMagic,
			Magic:  header.Magic,
			Offset: 0,
			Detail: fmt.Sprintf("the built-in Harmony RMQ decoder requires magic=0x%04x", harmonyRMQMagic),
		}
	}
	if header.Version != harmonyRMQVersion {
		return nil, &BuiltinSysDecodeError{
			Code:    builtinSysDecodeUnsupportedVersion,
			Magic:   header.Magic,
			Version: header.Version,
			Offset:  4,
			Detail:  fmt.Sprintf("the built-in Harmony RMQ decoder supports version=%d only", harmonyRMQVersion),
		}
	}
	if header.FileType != harmonyRMQFileType {
		return nil, &BuiltinSysDecodeError{
			Code:     builtinSysDecodeUnsupportedFileType,
			Magic:    header.Magic,
			Version:  header.Version,
			FileType: header.FileType,
			Offset:   2,
			Detail:   "the built-in decoder supports Harmony RMQ file_type=1 only; OpenHarmony/Linux file_type=0 requires trace_streamer",
		}
	}
	meta = &traceMetadata{
		binding: &builtinInputBinding{
			input: input, inputSize: size, sourceNamespace: sourceNamespace,
		},
		header:               header,
		formats:              map[int]eventFormat{},
		formatPoisoned:       map[int]bool{},
		formatPoisonFamilies: map[int]pairCriticalFormatFamilyMask{},
		printkFormats:        map[uint64]string{},
		printkPoisoned:       map[uint64]bool{},
		cmdlines:             map[int]string{},
		tgids:                map[int]int{},
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		remaining := size - pos
		if remaining == 0 {
			break
		}
		if remaining < 0 {
			return nil, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageBuiltinMetadata,
				input.DisplayPath(),
				fmt.Errorf("segment cursor %d exceeds fixed input size %d", pos, size),
			)
		}
		if remaining < segmentHdrSize {
			return nil, &BuiltinSysDecodeError{
				Code:        builtinSysDecodeTruncatedSegmentHeader,
				FileType:    header.FileType,
				HeaderBytes: int(remaining),
				Offset:      pos,
				Detail:      fmt.Sprintf("segment header requires %d bytes, only %d remain", segmentHdrSize, remaining),
				Cause:       io.ErrUnexpectedEOF,
			}
		}
		typ, segSize, err := readSegmentHeader(reader)
		if err != nil {
			return nil, fmt.Errorf("read segment header at %d: %w", pos, err)
		}
		if isProfilerTraceHeaderPrefix(typ, segSize) {
			break
		}
		payloadOffset, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("read segment payload offset at %d: %w", pos, err)
		}
		if payloadOffset < 0 || payloadOffset > size || int64(segSize) > size-payloadOffset {
			return nil, fmt.Errorf("invalid segment type=%d size=%d at offset=%d", typ, segSize, pos)
		}
		meta.segments = append(meta.segments, segmentMeta{Type: typ, Size: segSize, Offset: payloadOffset})
		switch {
		case typ == segmentEventsFormat || typ == segmentCmdlines || typ == segmentTGIDs || typ == segmentPrintk:
			if uint64(segSize) > uint64(^uint(0)>>1) {
				return nil, fmt.Errorf("metadata segment type=%d size=%d exceeds host addressable memory", typ, segSize)
			}
			data := make([]byte, segSize)
			if _, err := io.ReadFull(reader, data); err != nil {
				return nil, fmt.Errorf("read segment type=%d: %w", typ, err)
			}
			switch typ {
			case segmentEventsFormat:
				formats, err := parseEventFormats(data)
				if err != nil {
					return nil, err
				}
				catalog := eventFormatCatalog{
					Formats:          meta.formats,
					Poisoned:         meta.formatPoisoned,
					PoisonedFamilies: meta.formatPoisonFamilies,
				}
				mergeEventFormatCatalog(&catalog, formats)
				meta.formats = catalog.Formats
				meta.formatPoisoned = catalog.Poisoned
				meta.formatPoisonFamilies = catalog.PoisonedFamilies
			case segmentCmdlines:
				for pid, comm := range parseCmdlines(data) {
					meta.cmdlines[pid] = comm
				}
			case segmentTGIDs:
				for pid, tgid := range parseTGIDs(data) {
					meta.tgids[pid] = tgid
				}
			case segmentPrintk:
				catalog := printkFormatCatalog{Formats: meta.printkFormats, Poisoned: meta.printkPoisoned, Malformed: meta.printkMalformed}
				mergePrintkFormatCatalog(&catalog, parsePrintkFormats(data))
				meta.printkFormats, meta.printkPoisoned, meta.printkMalformed = catalog.Formats, catalog.Poisoned, catalog.Malformed
			}
		default:
			if _, err := reader.Seek(int64(segSize), io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("skip segment type=%d: %w", typ, err)
			}
		}
	}
	if len(meta.formats) == 0 && len(meta.formatPoisoned) == 0 {
		return nil, fmt.Errorf("no event format segment found; not a supported binary hitrace file")
	}
	return meta, nil
}

func validateBuiltinMetadataSegment(input io.ReaderAt, inputSize int64, segment segmentMeta) error {
	if input == nil || inputSize < 0 || segment.Offset < fileHeaderSize+segmentHdrSize || segment.Offset > inputSize {
		return fmt.Errorf("invalid range offset=%d size=%d input_size=%d", segment.Offset, segment.Size, inputSize)
	}
	if int64(segment.Size) > inputSize-segment.Offset {
		return fmt.Errorf("range exceeds fixed input: offset=%d size=%d input_size=%d", segment.Offset, segment.Size, inputSize)
	}
	headerOffset := segment.Offset - segmentHdrSize
	segmentType, segmentSize, err := readSegmentHeader(io.NewSectionReader(input, headerOffset, segmentHdrSize))
	if err != nil {
		return fmt.Errorf("read segment header at %d: %w", headerOffset, err)
	}
	if segmentType != segment.Type || segmentSize != segment.Size {
		return fmt.Errorf("segment header mismatch at %d: type=%d/%d size=%d/%d", headerOffset, segmentType, segment.Type, segmentSize, segment.Size)
	}
	return nil
}

func renderRows(ctx context.Context, meta *traceMetadata) (
	rows []renderedRow,
	missing int,
	unknown int,
	suppressed int,
	first uint64,
	last uint64,
	err error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	var input conversionInputView
	if meta != nil && meta.binding != nil {
		input = meta.binding.input
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageBuiltinRender, nil); err != nil {
		return nil, 0, 0, 0, 0, 0, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageBuiltinRender, err)
		if err != nil {
			rows = nil
			missing, unknown, suppressed = 0, 0, 0
			first, last = 0, 0
		}
	}()
	currentSize := input.Size()
	if meta.binding.inputSize < 0 || meta.binding.inputSize != currentSize {
		return nil, 0, 0, 0, 0, 0, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageBuiltinRender,
			input.DisplayPath(),
			fmt.Errorf("builtin metadata size %d does not match input authority size %d", meta.binding.inputSize, currentSize),
		)
	}
	if !directPairSourceNamespaceValid(meta.binding.sourceNamespace) {
		return nil, 0, 0, 0, 0, 0, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageBuiltinRender,
			input.DisplayPath(),
			fmt.Errorf("builtin metadata has an invalid frozen direct-pair source namespace"),
		)
	}
	expectedHeaderOffset := int64(fileHeaderSize)
	for index, segment := range meta.segments {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		if err := validateBuiltinMetadataSegment(input, meta.binding.inputSize, segment); err != nil {
			return nil, 0, 0, 0, 0, 0, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageBuiltinRender,
				input.DisplayPath(),
				fmt.Errorf("builtin metadata segment %d is not authority-bound: %w", index, err),
			)
		}
		if segment.Offset-segmentHdrSize != expectedHeaderOffset {
			return nil, 0, 0, 0, 0, 0, conversionInputFailure(
				ConversionInputCodeInternalContract,
				conversionInputStageBuiltinRender,
				input.DisplayPath(),
				fmt.Errorf("builtin metadata segment %d is out of physical order: header_offset=%d want=%d", index, segment.Offset-segmentHdrSize, expectedHeaderOffset),
			)
		}
		expectedHeaderOffset = segment.Offset + int64(segment.Size)
	}
	meta.formatConflictRows = 0
	meta.bodyRejectedRows = 0
	meta.bodyRejectReasons = make(map[string]int)
	meta.pairQuarantinedRows = 0
	meta.pairPoisonedLanes = 0
	meta.pairPoisonedFamilies = 0
	meta.pairBarrierBudget = false
	meta.pairBarrierReport = directPairBarrierReport{}
	pairBarrier, err := newDirectPairCaptureBarrierForNamespace(meta.binding.sourceNamespace)
	if err != nil {
		return nil, 0, 0, 0, 0, 0, err
	}
	seq := 0
	rc := renderContext{
		cmdlines:       meta.cmdlines,
		tgids:          meta.tgids,
		printkFormats:  meta.printkFormats,
		printkPoisoned: meta.printkPoisoned,
	}
	for _, seg := range meta.segments {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		if !isRawTraceSegment(seg.Type, meta.header.CPUNum) {
			continue
		}
		// The Harmony raw-trace contract stores fixed 4096-byte
		// RmqConsumerData pages. Treat a remainder as truncation instead of
		// silently dropping bytes at the end of the segment.
		if seg.Size%tracePageSize != 0 {
			return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
				Code:        builtinSysDecodePartialPageSegment,
				FileType:    meta.header.FileType,
				SegmentType: seg.Type,
				Offset:      seg.Offset,
				Detail:      fmt.Sprintf("raw segment size %d is not a multiple of the %d-byte RMQ page size", seg.Size, tracePageSize),
			}
		}
		if uint64(seg.Size) > uint64(^uint(0)>>1) {
			return nil, 0, 0, 0, 0, 0, fmt.Errorf("raw segment type=%d size=%d exceeds host addressable memory", seg.Type, seg.Size)
		}
		data := make([]byte, seg.Size)
		if _, err := io.ReadFull(io.NewSectionReader(input, seg.Offset, int64(seg.Size)), data); err != nil {
			return nil, 0, 0, 0, 0, 0, err
		}
		for pageOff := 0; pageOff < len(data); pageOff += tracePageSize {
			if err := ctx.Err(); err != nil {
				return nil, 0, 0, 0, 0, 0, err
			}
			page := data[pageOff : pageOff+tracePageSize]
			ph, ok := parsePageHeader(page)
			if !ok {
				continue
			}
			maxBodyLen := len(page) - pageHeaderSize
			if ph.Length > uint64(maxBodyLen) {
				return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
					Code:        builtinSysDecodePageLength,
					FileType:    meta.header.FileType,
					SegmentType: seg.Type,
					Offset:      seg.Offset + int64(pageOff+8),
					Detail:      fmt.Sprintf("RMQ page declares %d data byte(s), maximum is %d", ph.Length, maxBodyLen),
				}
			}
			body := page[pageHeaderSize : pageHeaderSize+int(ph.Length)]
			for off := 0; off < len(body); {
				if err := ctx.Err(); err != nil {
					return nil, 0, 0, 0, 0, 0, err
				}
				if len(body)-off < eventHeaderSize {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeTruncatedEventHeader,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ page ended with %d byte(s), fewer than the %d-byte event header", len(body)-off, eventHeaderSize),
					}
				}
				eh, ok := parseEventHeader(body[off:])
				if !ok {
					// Harmony's reference parser treats a zero-sized record as an
					// explicit end-of-page sentinel. Stop at it; never inspect the
					// physical bytes that follow the logical terminator.
					break
				}
				contentStart := off + eventHeaderSize
				contentEnd := contentStart + int(eh.Size)
				next := contentStart + eh.AlignedSize
				if contentEnd > len(body) || next > len(body) {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeEventBounds,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ event size=%d aligned_size=%d exceeds the %d-byte logical page body", eh.Size, eh.AlignedSize, len(body)),
					}
				}
				content := body[contentStart:contentEnd]
				if len(content) < 2 {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeEventBounds,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+contentStart),
						Detail:      "RMQ event content is too short to contain an event id",
					}
				}
				eventID := int(binary.LittleEndian.Uint16(content[:2]))
				offsetNS := uint64(eh.TimestampOffsetNS)
				if ph.TimestampNS > ^uint64(0)-offsetNS {
					return nil, 0, 0, 0, 0, 0, &BuiltinSysDecodeError{
						Code:        builtinSysDecodeTimestampOverflow,
						FileType:    meta.header.FileType,
						SegmentType: seg.Type,
						Offset:      seg.Offset + int64(pageOff+pageHeaderSize+off),
						Detail:      fmt.Sprintf("RMQ timestamp base=%d plus offset=%d overflows uint64 nanoseconds", ph.TimestampNS, offsetNS),
					}
				}
				ts := ph.TimestampNS + offsetNS
				format, ok := meta.formats[eventID]
				if !ok {
					if meta.formatPoisoned[eventID] {
						pairBarrier.poisonFormatFamilies(meta.formatPoisonFamilies[eventID])
						meta.formatConflictRows++
						off = next
						continue
					}
					missing++
					off = next
					continue
				}
				builtinProvenance := builtinRowProvenanceNone
				rc.builtinProvenance = &builtinProvenance
				line, admission, reason, envelopeOK, pairAudit := renderEventLineDecisionWithPairAudit(rc, ts, ph.CPU, format, content)
				rc.builtinProvenance = nil
				pairBarrier.observe(pairAudit)
				if !envelopeOK || line == "" {
					// The format was known, but the raw common-field envelope was
					// not. Keep the record coverage-only: emitting even a header-only
					// row here would turn missing/mistyped PID bytes into idle PID 0.
					suppressed++
					off = next
					continue
				}
				if admission == bodyRejected {
					if reason == "" {
						reason = "unspecified"
					}
					meta.bodyRejectedRows++
					meta.bodyRejectReasons[format.Name+"_"+reason]++
					off = next
					continue
				}
				if admission == bodyUnsupported {
					line = renderEventHeaderLine(rc, ts, ph.CPU, format, content)
					if line == "" {
						suppressed++
						off = next
						continue
					}
					builtinProvenance = builtinRowProvenanceIntentionalHeaderOnly
					unknown++
				}
				if len(rows) == 0 || ts < first {
					first = ts
				}
				if ts > last {
					last = ts
				}
				row := renderedRow{tsNS: ts, seq: seq, line: line, builtinProvenance: builtinProvenance}
				if pairAudit.EndpointAdmitted {
					row.pairKind = pairAudit.Kind
					pairBarrier.addPublishedRowAt(seq, ts, pairAudit)
				}
				rows = append(rows, row)
				seq++
				off = next
			}
		}
	}
	rows = pairBarrier.filter(rows)
	if err := pairBarrier.validateAccounting(rows); err != nil {
		return nil, 0, 0, 0, 0, 0, err
	}
	report := pairBarrier.report()
	meta.pairBarrierReport = report
	meta.pairQuarantinedRows = report.WithheldRows
	meta.pairPoisonedLanes = report.PoisonedLanes
	meta.pairPoisonedFamilies = report.PoisonedFamilies
	meta.pairBarrierBudget = report.LegacyBudgetReason != "" || report.BlockBudgetReason != "" || report.SharedAuthorityReason != ""
	first, last = 0, 0
	for index, row := range rows {
		if index == 0 || row.tsNS < first {
			first = row.tsNS
		}
		if index == 0 || row.tsNS > last {
			last = row.tsNS
		}
	}
	return rows, missing, unknown, suppressed, first, last, nil
}

func writeRows(w io.Writer, rows []renderedRow) error {
	_, err := writeOwnedBuiltinSystraceRows(context.Background(), w, rows)
	return err
}

func writeSystraceHeader(w io.Writer) error {
	_, err := io.WriteString(w, systraceHeader)
	return err
}
