package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	profilerDataTypeProtobuf      = uint32(0)
	profilerSessionJSONTag        = "SessionJSON-"
	maxProfilerTextLineBytes      = 1024 * 1024
	maxProfilerPluginFrameBytes   = uint64(64 << 20)
	profilerSessionReaderBufBytes = 256 * 1024
)

type profilerTraceHeader struct {
	Offset        int64
	Length        uint64
	Version       uint32
	Segments      uint32
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type profilerPluginData struct {
	Name                  string
	Status                uint32
	StatusPresent         bool
	Data                  []byte
	ClockID               uint64
	ClockIDPresent        bool
	ClockIDAmbiguous      bool
	TvSec                 uint64
	TvSecPresent          bool
	TvNsec                uint64
	TvNsecPresent         bool
	TimeTupleAmbiguous    bool
	Version               string
	VersionPresent        bool
	SampleInterval        uint32
	SampleIntervalPresent bool
}

type profilerPluginDataDecode struct {
	Plugin        profilerPluginData
	Accepted      bool
	IssueCensus   profilerPluginIssueCensus
	IssueOverflow bool
}

type profilerContainerExtraction struct {
	Detected              bool
	Kind                  string
	Messages              int
	PluginMessages        map[string]int
	StructuredFtrace      int
	MalformedFtrace       int
	UnsupportedFtrace     int
	TextPluginMessages    int
	TextRows              int
	StructuredRows        int
	RejectedMessages      int
	StandaloneDetected    bool
	SourceFailClosed      bool
	SourceFailReason      string
	TraceCoverage         []TraceDBCoverage
	Caveats               []string
	textMessages          []profilerTextMessageRows
	pairPublishers        []profilerPairPublisherCensus
	profilerEventCoverage profilerFtraceEventCoverageIndexes
}

type profilerTextMessageRows struct {
	total  int
	staged profilerPairCensusSet
}

type profilerBoundedPhysicalLine struct {
	Bytes     []byte
	Present   bool
	Oversized bool
	EOF       bool
}

type profilerPairPublisherCensus struct {
	coverageIndex int
	staged        profilerPairCensusSet
}

func profilerPairCensusSetHasRows(census profilerPairCensusSet) bool {
	for _, kind := range profilerCaptureKinds {
		if census[kind].total != 0 {
			return true
		}
	}
	return false
}

func validateProfilerPairPublisherCensus(extraction profilerContainerExtraction, sink *traceDBRowSink) error {
	if sink == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_sink_missing"}
	}
	byCoverage := make([]profilerPairCensusSet, len(extraction.TraceCoverage))
	var stagedTotal profilerPairCensusSet
	for _, publisher := range extraction.pairPublishers {
		if publisher.coverageIndex < 0 || publisher.coverageIndex >= len(byCoverage) {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_coverage_index_invalid"}
		}
		for _, kind := range profilerCaptureKinds {
			count := publisher.staged[kind].total
			if count < 0 || !checkedProfilerIntAddTo(&byCoverage[publisher.coverageIndex][kind].total, count) ||
				!checkedProfilerIntAddTo(&stagedTotal[kind].total, count) {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_staged_counter_invalid"}
			}
		}
	}
	for coverageIndex, staged := range byCoverage {
		captureRows := 0
		for _, kind := range profilerCaptureKinds {
			count := staged[kind].total
			if !checkedProfilerIntAddTo(&captureRows, count) {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_cross_total_overflow"}
			}
			if count == 0 {
				continue
			}
			raw := extraction.TraceCoverage[coverageIndex].FieldSources[profilerCoverageStagedRowsKey(kind)]
			declared, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || declared > uint64(math.MaxInt) || int(declared) != count {
				return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_staged_coverage_mismatch"}
			}
		}
		if captureRows > extraction.TraceCoverage[coverageIndex].RowsEmitted {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_rows_exceed_coverage"}
		}
	}
	for _, kind := range profilerCaptureKinds {
		if stagedTotal[kind].total != sink.pairRows[kind] {
			return &traceDBOutputInvariantError{Reason: "profiler_pair_publisher_sink_account_mismatch"}
		}
	}
	for _, message := range extraction.textMessages {
		if message.total < 0 {
			return &traceDBOutputInvariantError{Reason: "profiler_text_message_staged_counter_negative"}
		}
		captureRows := 0
		for _, kind := range profilerCaptureKinds {
			count := message.staged[kind].total
			if count < 0 || !checkedProfilerIntAddTo(&captureRows, count) {
				return &traceDBOutputInvariantError{Reason: "profiler_text_message_staged_counter_invalid"}
			}
		}
		if captureRows > message.total {
			return &traceDBOutputInvariantError{Reason: "profiler_text_message_pair_rows_exceed_total"}
		}
	}
	return nil
}

type profilerZeroFrameCensus struct {
	count       uint64
	firstOffset int64
	lastOffset  int64
}

func (census *profilerZeroFrameCensus) observe(offset int64) bool {
	if census == nil || census.count == math.MaxUint64 {
		return false
	}
	if census.count == 0 {
		census.firstOffset = offset
	}
	census.count++
	census.lastOffset = offset
	return true
}

func incrementProfilerContainerCounter(counter *int) bool {
	if counter == nil || *counter == math.MaxInt {
		return false
	}
	*counter++
	return true
}

func profilerContainerCounterFailClose(out *profilerContainerExtraction, sink *traceDBRowSink) {
	if out != nil {
		out.SourceFailClosed = true
		out.SourceFailReason = "container_counter_overflow"
	}
	if sink != nil {
		sink.failCloseAllRows()
	}
}

type profilerFtraceSummary struct {
	Version           string
	StatsMessages     int
	StartStats        int
	EndStats          int
	TraceClocks       map[string]int
	StatsCPUs         map[uint64]bool
	StartTotals       profilerFtraceCPUTotals
	EndTotals         profilerFtraceCPUTotals
	StartTotalsSeen   bool
	EndTotalsSeen     bool
	StartTotalsValid  bool
	EndTotalsValid    bool
	DetailMessages    int
	DetailCPUs        map[uint64]bool
	DetailEventCount  int
	DetailOverwrite   uint64
	DetailOverwriteOK bool
	SymbolCount       int
	SymbolExamples    []string
	SymbolTruncated   bool
	ClockDetailCount  int
	ClockDetails      []string
	ClockTruncated    bool
	EventFieldCounts  map[int]int
	Issues            profilerFtraceSummaryIssueCensus
	IssueOverflow     bool
	recognizedMessage bool
}

type profilerFtraceCPUTotals struct {
	Entries       uint64
	Overrun       uint64
	CommitOverrun uint64
	Bytes         uint64
	DroppedEvents uint64
	ReadEvents    uint64
}

type profilerFtraceCPUStats struct {
	Status   uint64
	Clock    string
	PerCPU   []profilerFtracePerCPUStats
	HasStats bool
}

type profilerFtracePerCPUStats struct {
	CPU           uint64
	Entries       uint64
	Overrun       uint64
	CommitOverrun uint64
	Bytes         uint64
	DroppedEvents uint64
	ReadEvents    uint64
}

type profilerFtraceCPUDetail struct {
	CPU              uint64
	EventCount       int
	EventFieldCounts map[int]int
	Overwrite        uint64
	OverwriteValid   bool
}

type profilerFtraceSymbolDetail struct {
	Addr uint64
	Name string
}

type profilerFtraceClockDetail struct {
	ID       uint64
	TimeSec  uint64
	TimeNsec uint64
	ResSec   uint64
	ResNsec  uint64
	HasTime  bool
	HasRes   bool
}

type profilerFtraceEventDescriptor struct {
	Field  int
	Family string
	Name   string
}

var profilerFtraceEventDescriptorList = [...]profilerFtraceEventDescriptor{
	{Field: 113, Family: "binder", Name: "binder_transaction"},
	{Field: 119, Family: "binder", Name: "binder_transaction_received"},
	{Field: 202, Family: "block", Name: "block_bio_complete"},
	{Field: 204, Family: "block", Name: "block_bio_queue"},
	{Field: 205, Family: "block", Name: "block_bio_remap"},
	{Field: 209, Family: "block", Name: "block_rq_complete"},
	{Field: 210, Family: "block", Name: "block_rq_insert"},
	{Field: 211, Family: "block", Name: "block_rq_issue"},
	{Field: 212, Family: "block", Name: "block_rq_remap"},
	{Field: 410, Family: "clock", Name: "clock_set_rate"},
	{Field: 1000, Family: "filemap", Name: "mm_filemap_add_to_page_cache"},
	{Field: 1001, Family: "filemap", Name: "mm_filemap_delete_from_page_cache"},
	{Field: 1109, Family: "trace_marker", Name: "print"},
	{Field: 1400, Family: "ipi", Name: "ipi_entry"},
	{Field: 1401, Family: "ipi", Name: "ipi_exit"},
	{Field: 1402, Family: "ipi", Name: "ipi_raise"},
	{Field: 1500, Family: "irq", Name: "irq_handler_entry"},
	{Field: 1501, Family: "irq", Name: "irq_handler_exit"},
	{Field: 1502, Family: "irq", Name: "softirq_entry"},
	{Field: 1503, Family: "irq", Name: "softirq_exit"},
	{Field: 1504, Family: "irq", Name: "softirq_raise"},
	{Field: 2002, Family: "clock", Name: "clock_set_rate"},
	{Field: 2003, Family: "cpu", Name: "cpu_frequency"},
	{Field: 2004, Family: "cpu", Name: "cpu_frequency_limits"},
	{Field: 2005, Family: "cpu", Name: "cpu_idle"},
	{Field: 2417, Family: "sched", Name: "sched_switch"},
	{Field: 2420, Family: "sched", Name: "sched_wakeup"},
	{Field: 2421, Family: "sched", Name: "sched_wakeup_new"},
	{Field: 2422, Family: "sched", Name: "sched_waking"},
	{Field: 4002, Family: "sched", Name: "sched_blocked_reason"},
	{Field: 4009, Family: "f2fs", Name: "f2fs_sync_file_enter"},
	{Field: 4010, Family: "f2fs", Name: "f2fs_sync_file_exit"},
	{Field: 4011, Family: "f2fs", Name: "f2fs_write_begin"},
	{Field: 4012, Family: "f2fs", Name: "f2fs_write_end"},
	{Field: 4015, Family: "mmc", Name: "mmc_request_done"},
	{Field: 4016, Family: "mmc", Name: "mmc_request_start"},
}

var profilerFtraceEventDescriptors = func() map[int]profilerFtraceEventDescriptor {
	out := make(map[int]profilerFtraceEventDescriptor, len(profilerFtraceEventDescriptorList))
	for _, descriptor := range profilerFtraceEventDescriptorList {
		out[descriptor.Field] = descriptor
	}
	return out
}()

func modernRowSorterCoverage(stats traceDBRowSortStats) TraceDBCoverage {
	coverage := stats.coverage()
	coverage.Family = "builtin_modern_profiler"
	coverage.Table = "__systrace_rows__"
	return coverage
}

const profilerCoverageMMCStagedRows = "complete_capture_mmc_rows_staged"
const profilerCoverageF2FSStagedRows = "complete_capture_f2fs_rows_staged"
const profilerCoverageBlockStagedRows = "complete_capture_block_rows_staged"

func profilerCoverageStagedRowsKey(kind pairRenderKind) string {
	switch kind {
	case pairRenderMMC:
		return profilerCoverageMMCStagedRows
	case pairRenderF2FS:
		return profilerCoverageF2FSStagedRows
	case pairRenderBlock:
		return profilerCoverageBlockStagedRows
	default:
		return ""
	}
}

func profilerMMCPairBarrierCoverage(withheld int, sink *traceDBRowSink) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_ftrace:mmc",
		Table:    "__complete_capture_barrier__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: withheld,
		Skipped:  "source-scoped MMC endpoint publication failed the profiler full-capture anti-rescue barrier",
		FieldSources: map[string]string{
			"scope":                    "one profiler container source namespace",
			"pairing_guard":            "all structured field-4015/4016 and text-compatible exact MMC rows are sealed before publication; malformed, opaque, or unattributable physical endpoints close the MMC family",
			"budget_fail_closed":       strconv.FormatBool(profilerPairBudgetFailure(sink, pairRenderMMC) != "none"),
			"budget_failure":           profilerPairBudgetFailure(sink, pairRenderMMC),
			"shared_authority_failure": profilerPairAuthorityFailure(sink),
		},
	}
}

func profilerF2FSPairBarrierCoverage(withheld int, sink *traceDBRowSink) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_ftrace:f2fs",
		Table:    "__complete_capture_barrier__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: withheld,
		Skipped:  "F2FS endpoint publication failed the profiler full-capture anti-rescue barrier",
		FieldSources: map[string]string{
			"scope":                    "exact source-local semantic lane when owner and dev/inode/op are proven; whole profiler F2FS family only when that hard key is unknown",
			"pairing_guard":            "structured field-4009..4012 and text-compatible exact F2FS rows share one typed fingerprint authority; known-key non-key failures quarantine only that lane, while malformed, opaque, or unattributable endpoints close the family",
			"budget_fail_closed":       strconv.FormatBool(profilerPairBudgetFailure(sink, pairRenderF2FS) != "none"),
			"budget_failure":           profilerPairBudgetFailure(sink, pairRenderF2FS),
			"shared_authority_failure": profilerPairAuthorityFailure(sink),
		},
	}
}

func profilerBlockPairBarrierCoverage(withheld int, sink *traceDBRowSink) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_ftrace:block",
		Table:    "__complete_capture_barrier__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: withheld,
		Skipped:  "Block endpoint publication failed the profiler full-capture anti-rescue barrier",
		FieldSources: map[string]string{
			"scope":                    "one profiler container source namespace; exact RQ/BIO semantic lane when owner, source, and hard key are proven",
			"pairing_guard":            "structured field-202/204/209/211 and text-compatible exact Block rows share one typed fingerprint authority; inventory fields 205/210/212 never enter the pairing lane",
			"budget_fail_closed":       strconv.FormatBool(profilerPairBudgetFailure(sink, pairRenderBlock) != "none"),
			"budget_failure":           profilerPairBudgetFailure(sink, pairRenderBlock),
			"shared_authority_failure": profilerPairAuthorityFailure(sink),
		},
	}
}

func profilerPairBudgetFailure(sink *traceDBRowSink, kind pairRenderKind) string {
	domain := sink.profilerPairProofDomain(kind)
	if domain == nil || domain.failureReason == "" {
		return "none"
	}
	return domain.failureReason
}

func profilerPairAuthorityFailure(sink *traceDBRowSink) string {
	if sink == nil || sink.pairAuthorityFailure == "" {
		return "none"
	}
	return sink.pairAuthorityFailure
}

func profilerPairBudgetCaveat(sink *traceDBRowSink, kind pairRenderKind) string {
	domain := sink.profilerPairProofDomain(kind)
	if domain == nil {
		return ""
	}
	var parts []string
	if domain.failureReason != "" {
		parts = append(parts, fmt.Sprintf("budget_fail_closed=true reason=%s observations=%d/%d lane_keys=%d/%d",
			domain.failureReason, domain.observations, domain.maxObservations,
			domain.laneKeys, domain.maxLaneKeys))
	}
	if sink.pairAuthorityFailure != "" {
		parts = append(parts, "shared_authority_failure="+sink.pairAuthorityFailure)
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, "; ")
}

func reconcileProfilerMMCCoverage(items []TraceDBCoverage, sink *traceDBRowSink, publishers []profilerPairPublisherCensus, eventIndexes profilerFtraceEventCoverageIndexes) error {
	return reconcileProfilerPairCoverage(items, sink, publishers, eventIndexes,
		pairRenderMMC, profilerCoverageMMCStagedRows, "mmc")
}

func reconcileProfilerF2FSCoverage(items []TraceDBCoverage, sink *traceDBRowSink, publishers []profilerPairPublisherCensus, eventIndexes profilerFtraceEventCoverageIndexes) error {
	return reconcileProfilerPairCoverage(items, sink, publishers, eventIndexes,
		pairRenderF2FS, profilerCoverageF2FSStagedRows, "f2fs")
}

func reconcileProfilerBlockCoverage(items []TraceDBCoverage, sink *traceDBRowSink, publishers []profilerPairPublisherCensus, eventIndexes profilerFtraceEventCoverageIndexes) error {
	return reconcileProfilerPairCoverage(items, sink, publishers, eventIndexes,
		pairRenderBlock, profilerCoverageBlockStagedRows, "block")
}

func reconcileProfilerPairCoverage(items []TraceDBCoverage, sink *traceDBRowSink, publishers []profilerPairPublisherCensus, eventIndexes profilerFtraceEventCoverageIndexes, kind pairRenderKind, stagedKey string, family string) error {
	if sink == nil || stagedKey == "" || family == "" {
		return &traceDBOutputInvariantError{Reason: "profiler_pair_reconcile_input_invalid"}
	}
	withheld, err := sink.withheldPairRowsForKindChecked(kind)
	if err != nil {
		return err
	}
	if withheld <= 0 {
		return nil
	}
	structuredAccounted := 0
	for _, field := range profilerStructuredPairEventFields(kind) {
		count, countErr := sink.withheldStructuredPairRowsForEventFieldChecked(kind, field)
		if countErr != nil {
			return countErr
		}
		if count <= 0 {
			continue
		}
		coverageIndex, present := eventIndexes.coverageIndexForField(field)
		if !present || coverageIndex < 0 || coverageIndex >= len(items) || count > items[coverageIndex].RowsEmitted {
			return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_structured_event_coverage_index_invalid"}
		}
		item := &items[coverageIndex]
		if item.FieldSources == nil {
			item.FieldSources = map[string]string{}
		}
		item.FieldSources["complete_capture_withheld_rows"] = strconv.Itoa(count)
		item.RowsEmitted -= count
		if !checkedProfilerIntAddTo(&structuredAccounted, count) {
			return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_structured_event_accounted_counter_overflow"}
		}
	}
	structuredWithheld, err := sink.withheldStructuredPairRowsForKindChecked(kind)
	if err != nil {
		return err
	}
	if structuredAccounted != structuredWithheld {
		return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_structured_event_attribution_mismatch"}
	}
	accounted := 0
	for _, publisher := range publishers {
		if publisher.coverageIndex < 0 || publisher.coverageIndex >= len(items) {
			return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_coverage_index_invalid"}
		}
		census := publisher.staged[kind]
		count, countErr := sink.withheldPairRowsFromCensusChecked(kind, census)
		if countErr != nil {
			return countErr
		}
		if count <= 0 {
			continue
		}
		item := &items[publisher.coverageIndex]
		if item.FieldSources == nil {
			item.FieldSources = map[string]string{}
		}
		stagedRaw, err := strconv.ParseUint(item.FieldSources[stagedKey], 10, 64)
		if err != nil || stagedRaw > uint64(math.MaxInt) || int(stagedRaw) < count || count > item.RowsEmitted {
			return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_coverage_exceeds_plugin_rows"}
		}
		item.RowsEmitted -= count
		withheldTotal := count
		if existingRaw, present := item.FieldSources["complete_capture_withheld_rows"]; present {
			existing, parseErr := strconv.ParseUint(existingRaw, 10, 64)
			if parseErr != nil || existing > uint64(math.MaxInt) ||
				!checkedProfilerIntAddTo(&withheldTotal, int(existing)) {
				return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_coverage_withheld_counter_overflow"}
			}
		}
		item.FieldSources["complete_capture_withheld_rows"] = strconv.Itoa(withheldTotal)
		if !checkedProfilerIntAddTo(&accounted, count) {
			return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_coverage_accounted_counter_overflow"}
		}
	}
	if accounted != withheld {
		return &traceDBOutputInvariantError{Reason: "profiler_" + family + "_coverage_attribution_mismatch"}
	}
	return nil
}

func tryConvertProfilerContainer(ctx context.Context, opts Options, inputSize int64, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision, initialTraceDecisions []TraceProviderDecision, initialTraceDBCoverage []TraceDBCoverage) (result Result, detected bool, err error) {
	ledger, err := newConversionFileLedger(opts.InputPath)
	if err != nil {
		return Result{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	result, detected, err = tryConvertProfilerContainerWithLedger(ctx, opts, inputSize, output, standaloneArtifacts, standaloneCaveats, standaloneDecisions, initialTraceDecisions, initialTraceDBCoverage, ledger)
	if err == nil {
		committed = true
	}
	return result, detected, err
}

func tryConvertProfilerContainerWithLedger(ctx context.Context, opts Options, inputSize int64, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision, initialTraceDecisions []TraceProviderDecision, initialTraceDBCoverage []TraceDBCoverage, ledger *conversionFileLedger) (result Result, detected bool, err error) {
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		return Result{}, false, err
	}
	if err := sink.bindContext(ctx); err != nil {
		return Result{}, false, traceDBJoinPreservingSingle(err, sink.cleanup())
	}
	sinkClosed := false
	defer func() {
		if !sinkClosed {
			err = traceDBJoinPreservingSingle(err, sink.cleanup())
		}
	}()
	if err := sink.openProfilerCapture(opts.InputPath); err != nil {
		return Result{}, false, err
	}
	sessionBodySize, embeddedSessionSidecar := profilerContainerSessionLayout(inputSize, standaloneArtifacts)
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimit(
		ctx, opts.InputPath, inputSize, sessionBodySize, sink)
	if err != nil {
		return Result{}, false, err
	}
	if !extracted.Detected {
		return Result{}, false, nil
	}
	if extracted.Kind == "openharmony_profiler_session_package" && sessionBodySize < inputSize {
		extracted.Caveats = append(extracted.Caveats, fmt.Sprintf(
			"profiler trace-body scan was bounded to byte offset %d before a typed terminal standalone sidecar chain; sidecar bytes were not reinterpreted as SessionJSON text records",
			sessionBodySize))
		for index := range extracted.TraceCoverage {
			item := &extracted.TraceCoverage[index]
			if item.FieldSources == nil {
				item.FieldSources = map[string]string{}
			}
			item.FieldSources["trace_body_input_bytes"] = strconv.FormatInt(sessionBodySize, 10)
			item.FieldSources["standalone_sidecar_boundary"] = "contiguous_typed_terminal_artifact_chain"
		}
	}
	if extracted.Kind == "openharmony_profiler_session_package" && embeddedSessionSidecar {
		extracted.Caveats = append(extracted.Caveats,
			"profiler Session text view contains a typed non-terminal standalone sidecar range; the complete profiler trace-body source was failed closed before publication so binary sidecar bytes cannot be reinterpreted as Session records")
		if !extracted.SourceFailClosed {
			failCloseProfilerTraceBody(&extracted, sink, "session_embedded_standalone_sidecar_ambiguity")
		}
	}
	if !extracted.SourceFailClosed {
		if err := validateProfilerPairPublisherCensus(extracted, sink); err != nil {
			return Result{}, true, err
		}
	}
	if err := sink.sealProfilerCaptureContext(ctx); err != nil {
		return Result{}, true, err
	}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		return Result{}, true, err
	}
	result = Result{
		InputPath:          opts.InputPath,
		InputBytes:         inputSize,
		Artifacts:          append([]Artifact(nil), standaloneArtifacts...),
		ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
		TraceDecisions:     append([]TraceProviderDecision(nil), initialTraceDecisions...),
		TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
		TraceCoverage:      append([]TraceDBCoverage(nil), extracted.TraceCoverage...),
		Caveats:            append([]string(nil), extracted.Caveats...),
		MissingFormatCount: 0,
		UnknownEventCount:  extracted.UnsupportedFtrace + extracted.RejectedMessages,
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	if !extracted.SourceFailClosed && sink.pairKindPoisoned(pairRenderMMC) {
		withheld := sink.withheldPairRowsForKind(pairRenderMMC)
		if err := reconcileProfilerMMCCoverage(result.TraceCoverage, sink, extracted.pairPublishers, extracted.profilerEventCoverage); err != nil {
			return Result{}, true, err
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler MMC full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; malformed, opaque, or unattributable exact MMC endpoints remain coverage-only%s", withheld, profilerPairBudgetCaveat(sink, pairRenderMMC)))
		result.TraceCoverage = append(result.TraceCoverage, profilerMMCPairBarrierCoverage(withheld, sink))
	}
	if !extracted.SourceFailClosed && sink.pairKindPoisoned(pairRenderF2FS) {
		withheld := sink.withheldPairRowsForKind(pairRenderF2FS)
		if err := reconcileProfilerF2FSCoverage(result.TraceCoverage, sink, extracted.pairPublishers, extracted.profilerEventCoverage); err != nil {
			return Result{}, true, err
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler F2FS full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; known-key failures remain exact-lane coverage-only, while malformed, opaque, or unattributable F2FS endpoints close the source-local family%s", withheld, profilerPairBudgetCaveat(sink, pairRenderF2FS)))
		result.TraceCoverage = append(result.TraceCoverage, profilerF2FSPairBarrierCoverage(withheld, sink))
	}
	if !extracted.SourceFailClosed && sink.pairKindPoisoned(pairRenderBlock) {
		withheld := sink.withheldPairRowsForKind(pairRenderBlock)
		if err := reconcileProfilerBlockCoverage(result.TraceCoverage, sink, extracted.pairPublishers, extracted.profilerEventCoverage); err != nil {
			return Result{}, true, err
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler Block full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; known owner/source/hard-key failures remain exact-lane coverage-only, while opaque or unattributable Block endpoints close the source-local family%s", withheld, profilerPairBudgetCaveat(sink, pairRenderBlock)))
		result.TraceCoverage = append(result.TraceCoverage, profilerBlockPairBarrierCoverage(withheld, sink))
	}
	publishableRows, err := sink.profilerPublishableRows()
	if err != nil {
		return Result{}, true, err
	}
	if publishableRows == 0 {
		// sealProfilerCaptureContext has already prepared the sink. Close the
		// public Accepted=Written+Withheld ledger explicitly because this lane
		// intentionally creates no output artifact and therefore skips writeTo.
		if err := sink.accountPreparedNoPublication(); err != nil {
			return Result{}, true, err
		}
		// No writer exists to own normal sorter cleanup. Complete it before the
		// zero-output coverage is copied into Result or persisted in the bundle,
		// so current_live_temp_bytes describes returned state rather than stale
		// pre-cleanup storage.
		if err := sink.cleanup(); err != nil {
			return Result{}, true, err
		}
		sinkClosed = true
	}
	if publishableRows > 0 {
		if err := ctx.Err(); err != nil {
			return Result{}, true, err
		}
		out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return Result{}, true, err
		}
		if err := ledger.recordOpenFile(output, out); err != nil {
			return Result{}, true, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(output, out))
		}
		stats, writeErr := sink.writeTo(ctx, out)
		closeErr := out.Close()
		writeErr = traceDBJoinPreservingSingle(writeErr, sink.cleanup())
		stats = sink.stats
		sinkClosed = true
		if writeErr != nil {
			writeErr = traceDBJoinPreservingSingle(writeErr, closeErr, ledger.removeOwnedPath(output))
			result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
			return Result{}, true, writeErr
		}
		if closeErr != nil {
			closeErr = traceDBJoinPreservingSingle(closeErr, ledger.removeOwnedPath(output))
			result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
			return Result{}, true, closeErr
		}
		info, err := os.Lstat(output)
		if err != nil {
			return Result{}, true, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(output))
		}
		if !info.Mode().IsRegular() || !ledger.ownsPathIdentity(output, info) || (stats.RowsWritten > 0 && info.Size() <= 0) {
			return Result{}, true, traceDBJoinPreservingSingle(fmt.Errorf("profiler systrace publication failed identity/regular-file validation: %s", output), ledger.removeOwnedPath(output))
		}
		if err := ledger.sealOwnedPath(output, info.Size()); err != nil {
			return Result{}, true, err
		}
		result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
		result.OutputPath = output
		result.OutputBytes = info.Size()
		result.EventsWritten = stats.RowsWritten
		result.FirstTimestampSec = float64(stats.FirstTSNS) / 1e9
		result.LastTimestampSec = float64(stats.LastTSNS) / 1e9
		result.Artifacts = append([]Artifact{{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     info.Size(),
			Converter: converterVersion + "+openharmony-profiler",
			Caveats:   []string{"generated from OpenHarmony profiler/session plugin payloads"},
		}}, result.Artifacts...)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderSuccess(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				Artifact{Type: ArtifactSystrace, Path: output},
			),
		)
	} else if extracted.SourceFailClosed {
		artifactStatus := "no independent artifact was produced"
		if len(result.Artifacts) > 0 {
			artifactStatus = fmt.Sprintf("%d independently produced artifact(s) remain available", len(result.Artifacts))
		}
		caveat := fmt.Sprintf(
			"OpenHarmony profiler/session trace-body was failed closed before publication because %s; suppressed_rows=%d; %s",
			firstNonEmpty(extracted.SourceFailReason, "profiler_source_failure"), sink.stats.RowsAccepted, artifactStatus)
		decisionReason := "profiler_source_resource_fail_closed"
		if extracted.SourceFailReason == "session_embedded_standalone_sidecar_ambiguity" ||
			extracted.SourceFailReason == profilerPairStorageIntegrityFailure {
			decisionReason = "profiler_source_integrity_fail_closed"
		}
		result.Caveats = append(result.Caveats, caveat)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderFailure(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				decisionReason,
				caveat,
			),
		)
	} else if len(result.Artifacts) == 0 {
		caveat := "OpenHarmony profiler/session container was detected, but no renderable trace rows or sidecar artifacts were found"
		result.Caveats = append(result.Caveats, caveat)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderFailure(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				"no_renderable_trace_rows",
				caveat,
			),
		)
	} else {
		caveat := "OpenHarmony profiler/session container was detected, but only sidecar artifacts were produced"
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderFailure(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				"sidecar_only",
				caveat,
			),
		)
	}
	if publishableRows == 0 {
		result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(sink.stats))
	}
	normalizeResultCollections(&result)
	if bundleArtifact, err := writeTraceBundleWithAllCoverageAndLedger(opts.InputPath, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage, ledger); err != nil {
		return Result{}, true, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
	return result, true, nil
}

func profilerContainerTraceBodySize(inputSize int64, artifacts []Artifact) int64 {
	size, _ := profilerContainerSessionLayout(inputSize, artifacts)
	return size
}

func profilerContainerSessionLayout(inputSize int64, artifacts []Artifact) (int64, bool) {
	type sourceRange struct {
		start int64
		end   int64
	}
	var ranges []sourceRange
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfData || artifact.SourceOffset < 0 || artifact.SourceBytes <= 0 ||
			artifact.SourceOffset > math.MaxInt64-artifact.SourceBytes {
			continue
		}
		end := artifact.SourceOffset + artifact.SourceBytes
		if end > inputSize {
			continue
		}
		ranges = append(ranges, sourceRange{start: artifact.SourceOffset, end: end})
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})
	merged := make([]sourceRange, 0, len(ranges))
	for _, item := range ranges {
		if len(merged) == 0 || item.start > merged[len(merged)-1].end {
			merged = append(merged, item)
			continue
		}
		if item.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = item.end
		}
	}
	if len(merged) > 0 && merged[len(merged)-1].end == inputSize {
		return merged[len(merged)-1].start, len(merged) > 1
	}
	return inputSize, len(merged) > 0
}

func extractProfilerContainerSystraceRows(ctx context.Context, path string, inputSize int64, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	return extractProfilerContainerSystraceRowsWithSessionLimit(ctx, path, inputSize, inputSize, sink)
}

func extractProfilerContainerSystraceRowsWithSessionLimit(ctx context.Context, path string,
	inputSize, sessionInputSize int64, sink *traceDBRowSink,
) (profilerContainerExtraction, error) {
	if inputSize < 0 || sessionInputSize < 0 || sessionInputSize > inputSize {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "invalid_profiler_session_input_boundary"}
	}
	header, ok, err := readProfilerTraceHeaderAtPath(path, 0, inputSize)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if ok && header.DataType == profilerDataTypeProtobuf {
		return extractProfilerTraceFile(ctx, path, inputSize, header, sink)
	}
	session, err := extractProfilerSessionPackage(ctx, path, sessionInputSize, sink)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if session.Detected {
		return session, nil
	}
	return profilerContainerExtraction{}, nil
}

func extractProfilerTraceFile(ctx context.Context, path string, inputSize int64, header profilerTraceHeader, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	return extractProfilerTraceFileWithFrameLimit(ctx, path, inputSize, header, sink, maxProfilerPluginFrameBytes)
}

func extractProfilerTraceFileWithFrameLimit(ctx context.Context, path string, inputSize int64,
	header profilerTraceHeader, sink *traceDBRowSink, maxFrameBytes uint64,
) (profilerContainerExtraction, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	return extractProfilerTraceFileAtWithFrameLimit(ctx, f, inputSize, header, sink, maxFrameBytes)
}

func extractProfilerTraceFileAtWithFrameLimit(ctx context.Context, reader io.ReaderAt, inputSize int64,
	header profilerTraceHeader, sink *traceDBRowSink, maxFrameBytes uint64,
) (profilerContainerExtraction, error) {
	if reader == nil || maxFrameBytes == 0 || maxFrameBytes > uint64(math.MaxInt) {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "invalid_profiler_plugin_frame_limit"}
	}
	var limit int64
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_trace_file",
		PluginMessages: map[string]int{},
		Caveats: []string{
			fmt.Sprintf("OpenHarmony profiler TraceFileHeader detected: data_type=%d version=0x%x segments=%d length=%d", header.DataType, header.Version, header.Segments, header.Length),
		},
	}
	if header.Length < uint64(profilerTraceHeaderSize) {
		out.RejectedMessages++
		out.Caveats = append(out.Caveats, fmt.Sprintf("profiler TraceFileHeader has invalid declared length=%d below header size=%d; no plugin frame bytes are eligible", header.Length, profilerTraceHeaderSize))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("trace_file_declared_length_invalid"))
		limit = profilerTraceHeaderSize
	} else if header.Length > uint64(inputSize) {
		out.RejectedMessages++
		out.Caveats = append(out.Caveats, fmt.Sprintf("profiler TraceFileHeader is truncated: declared length=%d available=%d; only complete framed messages within the available prefix are eligible", header.Length, inputSize))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("trace_file_declared_length_truncated"))
		limit = inputSize
	} else {
		limit = int64(header.Length)
	}
	off := int64(profilerTraceHeaderSize)
	seq := 0
	var zeroFrames profilerZeroFrameCensus
	diagnostics := newProfilerContainerDiagnosticLedger()
	var lenBuf [4]byte
frames:
	for off <= limit-4 {
		if err := ctx.Err(); err != nil {
			return profilerContainerExtraction{}, err
		}
		if _, err := reader.ReadAt(lenBuf[:], off); err != nil {
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message length at %d: %w", off, err)
		}
		n := uint64(binary.LittleEndian.Uint32(lenBuf[:]))
		if !incrementProfilerContainerCounter(&out.Messages) {
			out.SourceFailClosed = true
			out.SourceFailReason = "container_counter_overflow"
			sink.failCloseAllRows()
			break
		}
		// Header rejection can seed RejectedMessages independently. Guard the
		// counter once per physical frame so each branch below may increment it
		// at most once without duplicating overflow control inside hard gates.
		if out.RejectedMessages == math.MaxInt {
			out.SourceFailClosed = true
			out.SourceFailReason = "container_counter_overflow"
			sink.failCloseAllRows()
			break
		}
		if n == 0 {
			if !zeroFrames.observe(off) {
				out.SourceFailClosed = true
				out.SourceFailReason = "container_counter_overflow"
				sink.failCloseAllRows()
				break
			}
			out.RejectedMessages++
			off += 4
			continue
		}
		remaining := uint64(limit - off - 4)
		if n > remaining {
			out.RejectedMessages++
			sink.markPairCaptureOpaque(pairRenderMMC)
			sink.markPairCaptureOpaque(pairRenderF2FS)
			out.Caveats = append(out.Caveats, fmt.Sprintf("rejected truncated ProfilerPluginData frame at offset %d: declared=%d available=%d; sibling boundary cannot be recovered", off, n, remaining))
			out.TraceCoverage = append(out.TraceCoverage, profilerRejectedPluginCoverage("plugin_frame_truncated"))
			break
		}
		if n > maxFrameBytes {
			out.RejectedMessages++
			out.SourceFailClosed = true
			out.SourceFailReason = "plugin_frame_size_budget_exceeded"
			sink.failCloseAllRows()
			out.Caveats = append(out.Caveats, fmt.Sprintf(
				"rejected oversized ProfilerPluginData frame at offset %d: declared=%d max=%d; frame body was not read, the complete profiler trace-body source was failed closed before publication, and the unauthenticated container suffix was not scanned",
				off, n, maxFrameBytes))
			out.TraceCoverage = append(out.TraceCoverage, profilerPluginFrameBudgetCoverage(n, maxFrameBytes))
			break
		}
		msg := make([]byte, int(n))
		if _, err := reader.ReadAt(msg, off+4); err != nil {
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message at %d: %w", off+4, err)
		}
		decoded := parseProfilerPluginData(msg)
		if decoded.IssueOverflow {
			profilerContainerCounterFailClose(&out, sink)
			break frames
		}
		if decoded.Accepted {
			plugin := decoded.Plugin
			name := firstNonEmpty(plugin.Name, "unknown-plugin")
			route := classifyProfilerPluginRoute(name)
			outcome := profilerPluginOutcomeNoTextRows
			coverage := TraceDBCoverage{
				RowsRead: 1,
			}
			if !sink.beginPairRowCensus() {
				return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_census_nested"}
			}
			textMessageRows := 0
			appendPluginCoverage := func() bool {
				staged := sink.endPairRowCensus()
				coverageIndex, ok := diagnostics.observeAccepted(&out, route, name, plugin, decoded.IssueCensus,
					off, outcome, coverage.RowsEmitted, staged)
				if !ok {
					return false
				}
				if profilerPairCensusSetHasRows(staged) {
					if len(out.pairPublishers) == math.MaxInt {
						return false
					}
					out.pairPublishers = append(out.pairPublishers, profilerPairPublisherCensus{
						coverageIndex: coverageIndex, staged: staged,
					})
					if textMessageRows > 0 {
						if len(out.textMessages) == math.MaxInt {
							return false
						}
						out.textMessages = append(out.textMessages, profilerTextMessageRows{
							total: textMessageRows, staged: staged,
						})
					}
				}
				return true
			}
			if route == profilerPluginRouteExactFtrace {
				strictStage := stageProfilerStrictSystracePayload(plugin.Data)
				if strictStage.scan.originText {
					rows, textPayload, rowErr := addProfilerStrictSystraceStage(strictStage, &seq, sink)
					if rowErr != nil {
						coverage.Error = rowErr.Error()
						_ = appendPluginCoverage()
						return profilerContainerExtraction{}, rowErr
					}
					coverage.RowsEmitted = rows
					if textPayload {
						outcome = profilerPluginOutcomeStrictText
						textMessageRows = rows
						if !checkedProfilerIntAddTo(&out.TextRows, rows) || !checkedProfilerIntAddTo(&out.TextPluginMessages, 1) {
							_ = sink.endPairRowCensus()
							profilerContainerCounterFailClose(&out, sink)
							break frames
						}
						if !appendPluginCoverage() {
							profilerContainerCounterFailClose(&out, sink)
							break frames
						}
						off += 4 + int64(n)
						continue
					}
					outcome = profilerPluginOutcomeUnsupportedFtrace
					if !checkedProfilerIntAddTo(&out.UnsupportedFtrace, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					if !appendPluginCoverage() {
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					off += 4 + int64(n)
					continue
				}

				authority, authorityErr := decodeProfilerTracePluginResultContext(ctx, plugin.Data)
				if authorityErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, authorityErr
				}
				if authority.IssueOverflow || !diagnostics.FtraceEnvelope.observe(authority.Issues, off) {
					_ = sink.endPairRowCensus()
					profilerContainerCounterFailClose(&out, sink)
					break frames
				}
				authorityDegraded := !authority.Issues.empty()
				if authority.Disposition == profilerFtracePayloadNotStructured {
					outcome = profilerPluginOutcomeUnsupportedFtrace
					if !checkedProfilerIntAddTo(&out.UnsupportedFtrace, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					if !appendPluginCoverage() {
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					off += 4 + int64(n)
					continue
				}

				if authority.Disposition == profilerFtracePayloadStructured {
					outcome = profilerPluginOutcomeStructured
					if !checkedProfilerIntAddTo(&out.StructuredFtrace, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
				} else {
					outcome = profilerPluginOutcomeMalformed
					if !checkedProfilerIntAddTo(&out.MalformedFtrace, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
				}
				summary, ok, summaryErr := decodeProfilerFtraceSummaryResultContext(ctx, authority)
				if summaryErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, summaryErr
				} else {
					if ok {
						if summary.IssueOverflow || !diagnostics.FtraceSummary.observe(summary, off) {
							_ = sink.endPairRowCensus()
							profilerContainerCounterFailClose(&out, sink)
							break frames
						}
					}
					structuredRows, eventBatch, renderErr := renderProfilerFtraceStructuredResultForContainerContext(ctx, authority, &seq, sink)
					if renderErr != nil {
						coverage.Error = renderErr.Error()
						_ = appendPluginCoverage()
						return profilerContainerExtraction{}, renderErr
					}
					coverage.RowsEmitted = structuredRows
					if !checkedProfilerIntAddTo(&out.StructuredRows, structuredRows) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					if !diagnostics.FtraceEvents.merge(eventBatch) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
					if authorityDegraded || ok && !summary.Issues.empty() || eventBatch.degraded() || ok && structuredRows == 0 && summary.DetailEventCount > 0 {
						if outcome != profilerPluginOutcomeMalformed {
							outcome = profilerPluginOutcomeStructuredDegraded
						}
						if !checkedProfilerIntAddTo(&out.UnsupportedFtrace, 1) {
							_ = sink.endPairRowCensus()
							profilerContainerCounterFailClose(&out, sink)
							break frames
						}
					}
				}
			} else if route == profilerPluginRouteNoncanonicalFtrace {
				outcome = profilerPluginOutcomeNoncanonicalFtrace
				if !checkedProfilerIntAddTo(&out.UnsupportedFtrace, 1) {
					_ = sink.endPairRowCensus()
					profilerContainerCounterFailClose(&out, sink)
					break frames
				}
				sink.markPairCaptureOpaque(pairRenderMMC)
				sink.markPairCaptureOpaque(pairRenderF2FS)
				if profilerPayloadContainsExactBlockEndpoint(plugin.Data) {
					sink.markPairCaptureOpaque(pairRenderBlock)
				}
			} else if len(plugin.Data) == 0 {
				outcome = profilerPluginOutcomeEmptyPayload
			} else {
				rows, rowErr := addSystraceRowsFromBytes(plugin.Data, &seq, sink)
				if rowErr != nil {
					coverage.Error = rowErr.Error()
					_ = appendPluginCoverage()
					return profilerContainerExtraction{}, rowErr
				}
				coverage.RowsEmitted = rows
				if rows > 0 {
					outcome = profilerPluginOutcomeTextRows
					textMessageRows = rows
					if !checkedProfilerIntAddTo(&out.TextRows, rows) || !checkedProfilerIntAddTo(&out.TextPluginMessages, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
				} else {
					outcome = profilerPluginOutcomeNoTextRows
				}
			}
			if !appendPluginCoverage() {
				profilerContainerCounterFailClose(&out, sink)
				break frames
			}
		} else {
			out.RejectedMessages++
			sink.markPairCaptureOpaque(pairRenderMMC)
			sink.markPairCaptureOpaque(pairRenderF2FS)
			if profilerRejectedPluginFrameContainsExactBlockEndpoint(msg) {
				sink.markPairCaptureOpaque(pairRenderBlock)
			}
			if !diagnostics.observeRejected(&out, decoded.IssueCensus, off) {
				profilerContainerCounterFailClose(&out, sink)
				break frames
			}
		}
		off += 4 + int64(n)
	}
	if !appendProfilerZeroFrameCensus(&out, zeroFrames) {
		profilerContainerCounterFailClose(&out, sink)
	}
	if remaining := limit - off; remaining > 0 && remaining < 4 {
		if !incrementProfilerContainerCounter(&out.RejectedMessages) {
			profilerContainerCounterFailClose(&out, sink)
		}
		sink.markPairCaptureOpaque(pairRenderMMC)
		sink.markPairCaptureOpaque(pairRenderF2FS)
		out.Caveats = append(out.Caveats, fmt.Sprintf("rejected truncated ProfilerPluginData length prefix at offset %d: available=%d", off, remaining))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("plugin_length_prefix_truncated"))
	}
	if !diagnostics.materialize(&out) {
		profilerContainerCounterFailClose(&out, sink)
	}
	withheldStructured := 0
	withheldText := 0
	for _, kind := range profilerCaptureKinds {
		structured, structuredErr := sink.withheldStructuredPairRowsForKindChecked(kind)
		withheld, withheldErr := sink.withheldPairRowsForKindChecked(kind)
		if structuredErr != nil {
			return profilerContainerExtraction{}, structuredErr
		}
		if withheldErr != nil {
			return profilerContainerExtraction{}, withheldErr
		}
		if structured > withheld || !checkedProfilerIntAddTo(&withheldStructured, structured) ||
			!checkedProfilerIntAddTo(&withheldText, withheld-structured) {
			return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_cross_total_invalid"}
		}
	}
	if withheldStructured > out.StructuredRows || withheldText > out.TextRows {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_withheld_exceeds_extraction_staged"}
	}
	out.StructuredRows -= withheldStructured
	out.TextRows -= withheldText
	withheldMessages := 0
	for _, message := range out.textMessages {
		if message.total < 0 {
			return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_message_staged_counter_negative"}
		}
		publishable := message.total
		for _, kind := range profilerCaptureKinds {
			withheld, withheldErr := sink.withheldPairRowsFromCensusChecked(kind, message.staged[kind])
			if withheldErr != nil || withheld > publishable {
				return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_message_withheld_exceeds_staged"}
			}
			publishable -= withheld
		}
		if publishable == 0 {
			if !checkedProfilerIntAddTo(&withheldMessages, 1) {
				return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_message_withheld_counter_overflow"}
			}
		}
	}
	if withheldMessages > out.TextPluginMessages {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_messages_withheld_exceeds_staged"}
	}
	out.TextPluginMessages -= withheldMessages
	if out.SourceFailClosed {
		failCloseProfilerTraceBody(&out, sink, out.SourceFailReason)
	}
	if out.Messages == 0 {
		out.Caveats = append(out.Caveats, "official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable")
	}
	if out.SourceFailClosed && out.StructuredFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf(
			"decoded %d authoritative ftrace-plugin TracePluginResult message(s), but all structured rows were withheld by the profiler trace-body source fail-close",
			out.StructuredFtrace))
	} else if out.StructuredFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("decoded %d authoritative ftrace-plugin TracePluginResult message(s) and rendered %d structured trace row(s); unsupported or degraded members remain explicit in typed coverage", out.StructuredFtrace, out.StructuredRows))
	}
	if out.MalformedFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("classified %d ftrace-plugin payload(s) as malformed TracePluginResult; no partial structured or text rows were published", out.MalformedFtrace))
	}
	if out.TextRows > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from %d profiler plugin message(s)", out.TextRows, out.TextPluginMessages))
	}
	return out, nil
}

func appendProfilerZeroFrameCensus(out *profilerContainerExtraction, census profilerZeroFrameCensus) bool {
	if out == nil || census.count == 0 {
		return true
	}
	count, ok := profilerContainerCountToInt(census.count)
	if !ok {
		return false
	}
	out.Caveats = append(out.Caveats, fmt.Sprintf(
		"rejected %d zero-length ProfilerPluginData frame(s); first_offset=%d last_offset=%d; each complete 4-byte prefix advanced to the next frame boundary and remaining siblings, if any, were scanned",
		census.count, census.firstOffset, census.lastOffset))
	out.TraceCoverage = append(out.TraceCoverage, TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "plugin:__rejected__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: count,
		Skipped:  fmt.Sprintf("plugin_frame_zero_length=%d", census.count),
		FieldSources: map[string]string{
			"schema_profile":     "ProfilerPluginData{name=1,status=2,data=3,clock_id=4,tv_sec=5,tv_nsec=6,version=7,sample_interval=8}",
			"aggregation_policy": "exact_count_with_first_last_offset",
			"first_offset":       strconv.FormatInt(census.firstOffset, 10),
			"last_offset":        strconv.FormatInt(census.lastOffset, 10),
			"observed_total":     strconv.FormatUint(census.count, 10),
		},
	})
	return true
}

func profilerContainerCountToInt(count uint64) (int, bool) {
	if count > uint64(math.MaxInt) {
		return 0, false
	}
	return int(count), true
}

func profilerPluginMetadataCaveat(name string, plugin profilerPluginData) string {
	var parts []string
	if plugin.Status != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", plugin.Status))
	}
	timeTuplePresent := plugin.TvSecPresent || plugin.TvNsecPresent
	if !plugin.ClockIDAmbiguous && !plugin.TimeTupleAmbiguous {
		if plugin.ClockIDPresent || timeTuplePresent {
			parts = append(parts, "clock_id="+profilerPluginClockName(plugin.ClockID))
		}
		if timeTuplePresent {
			parts = append(parts, fmt.Sprintf("tv=%d.%09d", plugin.TvSec, plugin.TvNsec))
		}
	}
	if plugin.Version != "" {
		parts = append(parts, "version="+plugin.Version)
	}
	if plugin.SampleInterval != 0 {
		parts = append(parts, fmt.Sprintf("sample_interval_ms=%d", plugin.SampleInterval))
	}
	if len(plugin.Data) > 0 {
		parts = append(parts, fmt.Sprintf("payload_bytes=%d", len(plugin.Data)))
	}
	if len(parts) == 0 {
		parts = append(parts, "metadata=present")
	}
	return fmt.Sprintf("profiler plugin %s metadata: %s", name, strings.Join(parts, "; "))
}

func profilerRejectedPluginCoverage(reason string) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "plugin:__rejected__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  reason,
		FieldSources: map[string]string{
			"schema_profile": "ProfilerPluginData{name=1,status=2,data=3,clock_id=4,tv_sec=5,tv_nsec=6,version=7,sample_interval=8}",
		},
	}
}

func profilerPluginFrameBudgetCoverage(declared, limit uint64) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "plugin:__rejected__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  "plugin_frame_size_budget_exceeded=1",
		FieldSources: map[string]string{
			"declared_frame_bytes":            strconv.FormatUint(declared, 10),
			"max_frame_bytes":                 strconv.FormatUint(limit, 10),
			"frame_body_uninspected":          "true",
			"suffix_scan_stopped":             "true",
			"boundary_recovery_policy":        "disabled_until_lane_specific_header_integrity_is_verified",
			"profiler_trace_body_fail_closed": "all_rows",
			"single_object_memory_bound":      "true",
		},
	}
}

// applyProfilerCaptureSourceFailure is the single post-seal bridge from the
// sink's storage proof into customer-visible source-failure disclosure. The
// sink must already have suppressed every row, so this function cannot mutate
// capture state after seal; it only reconciles diagnostics before output-open
// eligibility is evaluated.
func applyProfilerCaptureSourceFailure(extraction *profilerContainerExtraction, sink *traceDBRowSink) error {
	if sink == nil || sink.captureSourceFailure == "" {
		return nil
	}
	if sink.captureLifecycle != profilerCaptureSealed || !sink.allRowsFailClosed {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_source_failure_state_invalid"}
	}
	if extraction == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_capture_source_failure_extraction_missing"}
	}
	if !extraction.SourceFailClosed {
		failCloseProfilerTraceBody(extraction, sink, sink.captureSourceFailure)
	}
	return nil
}

func failCloseProfilerTraceBody(extraction *profilerContainerExtraction, sink *traceDBRowSink,
	reason string,
) {
	if extraction == nil {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "unknown_profiler_source_failure"
	}
	barrierTable := "__container_resource_barrier__"
	failureClass := "resource_limit"
	if reason == "session_embedded_standalone_sidecar_ambiguity" {
		barrierTable = "__container_integrity_barrier__"
		failureClass = "integrity_ambiguity"
	} else if reason == profilerPairStorageIntegrityFailure {
		barrierTable = "__container_integrity_barrier__"
		failureClass = "storage_integrity_failure"
	}
	extraction.SourceFailClosed = true
	extraction.SourceFailReason = reason
	extraction.TextRows = 0
	extraction.StructuredRows = 0
	extraction.TextPluginMessages = 0
	for index := range extraction.TraceCoverage {
		item := &extraction.TraceCoverage[index]
		if item.RowsEmitted > 0 {
			traceDBAppendCoverageSkipped(item,
				fmt.Sprintf("profiler_source_fail_closed=%s; suppressed_rows=%d", reason, item.RowsEmitted))
			item.RowsEmitted = 0
		}
		if item.FieldSources == nil {
			item.FieldSources = map[string]string{}
		}
		item.FieldSources["profiler_trace_body_source_fail_closed"] = reason
	}
	stagedRows := 0
	if sink != nil {
		stagedRows = sink.stats.RowsAccepted
	}
	barrierFieldSources := map[string]string{
		"scope":             "complete_profiler_trace_body",
		"independent_scope": "not_governed_by_trace_body_barrier",
		"failure_class":     failureClass,
	}
	if reason == profilerPairStorageIntegrityFailure {
		barrierFieldSources["shared_authority_failure"] = profilerPairAuthorityFailure(sink)
		barrierFieldSources["suppressed_rows"] = strconv.Itoa(stagedRows)
		barrierFieldSources["seal_before_output_open"] = "true"
	}
	extraction.TraceCoverage = append(extraction.TraceCoverage, TraceDBCoverage{
		Family:       "builtin_modern_profiler",
		Table:        barrierTable,
		Role:         "unsupported_input",
		Found:        true,
		RowsRead:     stagedRows,
		Skipped:      fmt.Sprintf("profiler_source_fail_closed=%s; suppressed_rows=%d", reason, stagedRows),
		FieldSources: barrierFieldSources,
	})
	if sink != nil && !sink.allRowsFailClosed {
		sink.failCloseAllRows()
	}
}

func profilerContainerEnvelopeCoverage(reason string) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "__container_envelope__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  reason,
		FieldSources: map[string]string{
			"schema_profile": "TraceFileHeader.length bounds the length-prefixed ProfilerPluginData frame sequence",
		},
	}
}

func profilerPluginClockName(id uint64) string {
	switch id {
	case 0:
		return "REALTIME"
	case 1:
		return "MONOTONIC"
	case 2:
		return "PROCESS_CPUTIME_ID"
	case 3:
		return "THREAD_CPUTIME_ID"
	case 4:
		return "MONOTONIC_RAW"
	case 5:
		return "REALTIME_COARSE"
	case 6:
		return "MONOTONIC_COARSE"
	case 7:
		return "BOOTTIME"
	case 8:
		return "REALTIME_ALARM"
	case 9:
		return "BOOTTIME_ALARM"
	case 10:
		return "SGI_CYCLE"
	case 11:
		return "TAI"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", id)
	}
}

func decodeProfilerFtraceSummary(data []byte) (profilerFtraceSummary, bool, error) {
	return decodeProfilerFtraceSummaryResult(decodeProfilerTracePluginResult(data))
}

func decodeProfilerFtraceSummaryResult(result profilerTracePluginResult) (profilerFtraceSummary, bool, error) {
	return decodeProfilerFtraceSummaryResultContext(context.Background(), result)
}

func decodeProfilerFtraceSummaryResultContext(ctx context.Context, result profilerTracePluginResult) (profilerFtraceSummary, bool, error) {
	summary := profilerFtraceSummary{
		TraceClocks:       map[string]int{},
		StatsCPUs:         map[uint64]bool{},
		DetailCPUs:        map[uint64]bool{},
		EventFieldCounts:  map[int]int{},
		StartTotalsValid:  true,
		EndTotalsValid:    true,
		DetailOverwriteOK: true,
	}
	if result.Disposition == profilerFtracePayloadNotStructured {
		return summary, false, nil
	}
	if result.Disposition == profilerFtracePayloadMalformed {
		return summary, false, nil
	}
	summary.recognizedMessage = true
	err := visitProfilerTracePluginResult(ctx, result, func(field int, raw []byte) error {
		switch field {
		case 1:
			stats, err := decodeProfilerFtraceCPUStats(raw)
			if err != nil {
				if !summary.Issues.observe(profilerFtraceSummaryIssueCPUStatsMalformed, 1) {
					summary.IssueOverflow = true
				}
				return nil
			}
			summary.StatsMessages++
			if stats.Clock != "" {
				summary.TraceClocks[stats.Clock]++
			}
			if stats.Status == 1 {
				summary.EndStats++
			} else {
				summary.StartStats++
			}
			for _, cpu := range stats.PerCPU {
				summary.StatsCPUs[cpu.CPU] = true
				if stats.Status == 1 {
					summary.EndTotalsSeen = true
					if summary.EndTotalsValid && !summary.EndTotals.add(cpu) {
						summary.EndTotalsValid = false
						summary.EndTotals = profilerFtraceCPUTotals{}
						if !summary.Issues.observe(profilerFtraceSummaryIssueEndStatsOverflow, 1) {
							summary.IssueOverflow = true
						}
					}
				} else {
					summary.StartTotalsSeen = true
					if summary.StartTotalsValid && !summary.StartTotals.add(cpu) {
						summary.StartTotalsValid = false
						summary.StartTotals = profilerFtraceCPUTotals{}
						if !summary.Issues.observe(profilerFtraceSummaryIssueStartStatsOverflow, 1) {
							summary.IssueOverflow = true
						}
					}
				}
			}
		case 2:
			detail, err := decodeProfilerFtraceCPUDetail(raw)
			if err != nil {
				return nil
			}
			summary.DetailMessages++
			summary.DetailCPUs[detail.CPU] = true
			summary.DetailEventCount += detail.EventCount
			if !detail.OverwriteValid {
				summary.DetailOverwriteOK = false
				summary.DetailOverwrite = 0
			} else if summary.DetailOverwriteOK {
				if next, ok := checkedProfilerUint64Add(summary.DetailOverwrite, detail.Overwrite); ok {
					summary.DetailOverwrite = next
				} else {
					summary.DetailOverwriteOK = false
					summary.DetailOverwrite = 0
					if !summary.Issues.observe(profilerFtraceSummaryIssueDetailOverwriteOverflow, 1) {
						summary.IssueOverflow = true
					}
				}
			}
			for eventField, count := range detail.EventFieldCounts {
				summary.EventFieldCounts[eventField] += count
			}
		case 5:
			symbol, err := decodeProfilerFtraceSymbolDetail(raw)
			if err != nil {
				if !summary.Issues.observe(profilerFtraceSummaryIssueSymbolMalformed, 1) {
					summary.IssueOverflow = true
				}
				return nil
			}
			summary.SymbolCount++
			if symbol.Name != "" {
				if len(summary.SymbolExamples) < 5 {
					if symbol.Addr != 0 {
						summary.SymbolExamples = append(summary.SymbolExamples, fmt.Sprintf("0x%x=%s", symbol.Addr, symbol.Name))
					} else {
						summary.SymbolExamples = append(summary.SymbolExamples, symbol.Name)
					}
				} else {
					summary.SymbolTruncated = true
				}
			}
		case 6:
			clock, err := decodeProfilerFtraceClockDetail(raw)
			if err != nil {
				if !summary.Issues.observe(profilerFtraceSummaryIssueClockMalformed, 1) {
					summary.IssueOverflow = true
				}
				return nil
			}
			if label := profilerFtraceClockDetailLabel(clock); label != "" {
				summary.ClockDetailCount++
				if len(summary.ClockDetails) < 8 {
					summary.ClockDetails = append(summary.ClockDetails, label)
				} else {
					summary.ClockTruncated = true
				}
			}
		case 7:
			if result.VersionOccurrences != 1 {
				return nil
			}
			if traceDBSinglePhysicalLine(string(raw), true) {
				summary.Version = string(raw)
			} else if !summary.Issues.observe(profilerFtraceSummaryIssueVersionInvalid, 1) {
				summary.IssueOverflow = true
			}
		case 8:
			if err := decodeProfilerFtraceCommDict(raw); err != nil {
				if !summary.Issues.observe(profilerFtraceSummaryIssueCommMalformed, 1) {
					summary.IssueOverflow = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return summary, false, err
	}
	return summary, summary.recognizedMessage, nil
}

func decodeProfilerFtraceCPUStats(data []byte) (profilerFtraceCPUStats, error) {
	var stats profilerFtraceCPUStats
	statusCount, clockCount := 0, 0
	statusWrongWire, clockWrongWire, perCPUWrongWire := false, false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			statusCount++
			if wire == 0 {
				stats.Status = v
			} else {
				statusWrongWire = true
			}
		case 2:
			if wire != 2 {
				perCPUWrongWire = true
				return nil
			}
			perCPU, err := decodeProfilerFtracePerCPUStats(raw)
			if err != nil {
				return err
			}
			stats.HasStats = true
			stats.PerCPU = append(stats.PerCPU, perCPU)
		case 3:
			clockCount++
			if wire == 2 {
				stats.Clock = string(raw)
			} else {
				clockWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	if statusCount > 1 || statusWrongWire || stats.Status > 1 {
		return stats, fmt.Errorf("invalid FtraceCpuStatsMsg status field")
	}
	if perCPUWrongWire {
		return stats, fmt.Errorf("wrong-wire FtraceCpuStatsMsg per_cpu_stats field")
	}
	if clockCount > 1 || clockWrongWire || clockCount == 1 && !traceDBSingleToken(stats.Clock) {
		return stats, fmt.Errorf("invalid FtraceCpuStatsMsg trace_clock field")
	}
	return stats, err
}

func decodeProfilerFtracePerCPUStats(data []byte) (profilerFtracePerCPUStats, error) {
	var stats profilerFtracePerCPUStats
	var counts [10]int
	var wrongWire [10]bool
	var rawValues [10]uint64
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		if field < 1 || field > 9 {
			return nil
		}
		expectedWire := 0
		if field == 6 || field == 7 {
			expectedWire = 1
		}
		counts[field]++
		if wire != expectedWire {
			wrongWire[field] = true
			return nil
		}
		rawValues[field] = v
		switch field {
		case 1:
			stats.CPU = v
		case 2:
			stats.Entries = v
		case 3:
			stats.Overrun = v
		case 4:
			stats.CommitOverrun = v
		case 5:
			stats.Bytes = v
		case 8:
			stats.DroppedEvents = v
		case 9:
			stats.ReadEvents = v
		}
		_ = raw
		return nil
	})
	if err != nil {
		return stats, err
	}
	for field := 1; field <= 9; field++ {
		if counts[field] > 1 || wrongWire[field] {
			return stats, fmt.Errorf("invalid PerCpuStatsMsg field %d", field)
		}
	}
	if stats.CPU > uint64(maxTraceDBCPUIndex) {
		return stats, fmt.Errorf("out-of-range PerCpuStatsMsg cpu field")
	}
	for _, field := range []int{6, 7} {
		if counts[field] == 1 {
			value := math.Float64frombits(rawValues[field])
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return stats, fmt.Errorf("non-finite PerCpuStatsMsg field %d", field)
			}
		}
	}
	return stats, err
}

func decodeProfilerFtraceCPUDetail(data []byte) (profilerFtraceCPUDetail, error) {
	detail := profilerFtraceCPUDetail{EventFieldCounts: map[int]int{}, OverwriteValid: true}
	cpuCount := 0
	cpuWrongWire := false
	overwriteCount := 0
	overwriteWrongWire := false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			cpuCount++
			if wire == 0 {
				detail.CPU = v
			} else {
				cpuWrongWire = true
			}
		case 2:
			if wire == 2 {
				fields, err := decodeProfilerFtraceEventFields(raw)
				if err != nil {
					return nil
				}
				detail.EventCount++
				for _, eventField := range fields {
					detail.EventFieldCounts[eventField]++
				}
			}
		case 3:
			overwriteCount++
			if wire == 0 {
				detail.Overwrite = v
			} else {
				overwriteWrongWire = true
			}
		}
		_ = raw
		return nil
	})
	if err != nil {
		return detail, err
	}
	if cpuCount > 1 {
		return detail, fmt.Errorf("duplicate FtraceCpuDetailMsg cpu field")
	}
	if cpuWrongWire {
		return detail, fmt.Errorf("wrong-wire FtraceCpuDetailMsg cpu field")
	}
	if overwriteCount > 1 || overwriteWrongWire {
		detail.Overwrite = 0
		detail.OverwriteValid = false
	}
	if detail.CPU > uint64(maxTraceDBCPUIndex) {
		return detail, fmt.Errorf("out-of-range FtraceCpuDetailMsg cpu field")
	}
	return detail, err
}

func decodeProfilerFtraceEventFields(data []byte) ([]int, error) {
	record, err := decodeProfilerFtraceEventRecord(0, data)
	if err != nil {
		return nil, err
	}
	envelopeIssues, issueErr := record.checkedEnvelopeIssues()
	if issueErr != nil {
		return nil, issueErr
	}
	if len(envelopeIssues) > 0 {
		labels, ok := profilerFtraceEventIssueLabels(record.Field, envelopeIssues)
		if !ok {
			return nil, &traceDBOutputInvariantError{Reason: "profiler_event_envelope_issue_invalid"}
		}
		return nil, fmt.Errorf("invalid FtraceEvent envelope: %s", strings.Join(labels, ","))
	}
	return []int{record.Field}, nil
}

func decodeProfilerFtraceSymbolDetail(data []byte) (profilerFtraceSymbolDetail, error) {
	var symbol profilerFtraceSymbolDetail
	addrCount, nameCount := 0, 0
	addrWrongWire, nameWrongWire := false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			addrCount++
			if wire == 0 {
				symbol.Addr = v
			} else {
				addrWrongWire = true
			}
		case 2:
			nameCount++
			if wire == 2 {
				symbol.Name = string(raw)
			} else {
				nameWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return symbol, err
	}
	if addrCount > 1 || addrWrongWire || nameCount > 1 || nameWrongWire ||
		nameCount == 1 && !traceDBSinglePhysicalLine(symbol.Name, true) {
		return symbol, fmt.Errorf("invalid SymbolsDetailMsg field")
	}
	return symbol, err
}

func decodeProfilerFtraceClockDetail(data []byte) (profilerFtraceClockDetail, error) {
	var clock profilerFtraceClockDetail
	idCount, timeCount, resCount := 0, 0, 0
	idWrongWire, timeWrongWire, resWrongWire := false, false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			idCount++
			if wire == 0 {
				clock.ID = v
			} else {
				idWrongWire = true
			}
		case 2:
			timeCount++
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.TimeSec, clock.TimeNsec, clock.HasTime = sec, nsec, true
			} else {
				timeWrongWire = true
			}
		case 3:
			resCount++
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.ResSec, clock.ResNsec, clock.HasRes = sec, nsec, true
			} else {
				resWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return clock, err
	}
	if idCount > 1 || idWrongWire || clock.ID > 6 || timeCount > 1 || timeWrongWire || resCount > 1 || resWrongWire {
		return clock, fmt.Errorf("invalid ClockDetailMsg field")
	}
	return clock, err
}

func decodeProfilerFtraceTimeSpec(data []byte) (uint64, uint64, error) {
	var sec, nsec uint64
	secCount, nsecCount := 0, 0
	secWrongWire, nsecWrongWire := false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			secCount++
			if wire == 0 {
				sec = v
			} else {
				secWrongWire = true
			}
		case 2:
			nsecCount++
			if wire == 0 {
				nsec = v
			} else {
				nsecWrongWire = true
			}
		}
		_ = raw
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	if secCount > 1 || secWrongWire || sec > uint64(^uint32(0)) ||
		nsecCount > 1 || nsecWrongWire || nsec > uint64(^uint32(0)) || nsec >= 1e9 {
		return 0, 0, fmt.Errorf("invalid ClockDetailMsg TimeSpec field")
	}
	return sec, nsec, err
}

func decodeProfilerFtraceCommDict(data []byte) error {
	tidCount, commCount := 0, 0
	tidWrongWire, commWrongWire := false, false
	var tid uint64
	var comm string
	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		switch field {
		case 1:
			tidCount++
			if wire == 0 {
				tid = value
			} else {
				tidWrongWire = true
			}
		case 2:
			commCount++
			if wire == 2 {
				comm = string(raw)
			} else {
				commWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if tidCount > 1 || tidWrongWire || tidCount == 1 && uint64(int64(int32(tid))) != tid ||
		commCount > 1 || commWrongWire || commCount == 1 && !traceDBSinglePhysicalLine(comm, true) {
		return fmt.Errorf("invalid CommDictMsg field")
	}
	return nil
}

func (totals *profilerFtraceCPUTotals) add(stats profilerFtracePerCPUStats) bool {
	next := *totals
	var ok bool
	if next.Entries, ok = checkedProfilerUint64Add(next.Entries, stats.Entries); !ok {
		return false
	}
	if next.Overrun, ok = checkedProfilerUint64Add(next.Overrun, stats.Overrun); !ok {
		return false
	}
	if next.CommitOverrun, ok = checkedProfilerUint64Add(next.CommitOverrun, stats.CommitOverrun); !ok {
		return false
	}
	if next.Bytes, ok = checkedProfilerUint64Add(next.Bytes, stats.Bytes); !ok {
		return false
	}
	if next.DroppedEvents, ok = checkedProfilerUint64Add(next.DroppedEvents, stats.DroppedEvents); !ok {
		return false
	}
	if next.ReadEvents, ok = checkedProfilerUint64Add(next.ReadEvents, stats.ReadEvents); !ok {
		return false
	}
	*totals = next
	return true
}

func checkedProfilerUint64Add(left, right uint64) (uint64, bool) {
	if left > ^uint64(0)-right {
		return 0, false
	}
	return left + right, true
}

func profilerFtraceSummaryCaveat(summary profilerFtraceSummary) string {
	var parts []string
	if summary.Version != "" {
		parts = append(parts, "version="+summary.Version)
	}
	parts = append(parts, fmt.Sprintf("stats_messages=%d", summary.StatsMessages))
	if summary.StartStats != 0 || summary.EndStats != 0 {
		parts = append(parts, fmt.Sprintf("stats_start=%d", summary.StartStats))
		parts = append(parts, fmt.Sprintf("stats_end=%d", summary.EndStats))
	}
	if len(summary.TraceClocks) > 0 {
		parts = append(parts, "trace_clock="+joinStringCounts(summary.TraceClocks))
	}
	if len(summary.StatsCPUs) > 0 {
		totals := summary.StartTotals
		totalsValid := summary.StartTotalsValid
		label := "observed"
		if summary.EndTotalsSeen {
			totals = summary.EndTotals
			totalsValid = summary.EndTotalsValid
			label = "end"
		} else if !summary.StartTotalsSeen {
			totalsValid = false
		}
		parts = append(parts, fmt.Sprintf("stats_cpus=%d", len(summary.StatsCPUs)))
		if totalsValid {
			parts = append(parts, fmt.Sprintf("%s_entries=%d", label, totals.Entries))
			parts = append(parts, fmt.Sprintf("%s_dropped=%d", label, totals.DroppedEvents))
			parts = append(parts, fmt.Sprintf("%s_overrun=%d", label, totals.Overrun))
			parts = append(parts, fmt.Sprintf("%s_commit_overrun=%d", label, totals.CommitOverrun))
			parts = append(parts, fmt.Sprintf("%s_read=%d", label, totals.ReadEvents))
			parts = append(parts, fmt.Sprintf("%s_bytes=%d", label, totals.Bytes))
		}
	}
	if summary.DetailMessages > 0 {
		parts = append(parts, fmt.Sprintf("detail_messages=%d", summary.DetailMessages))
		parts = append(parts, fmt.Sprintf("detail_cpus=%d", len(summary.DetailCPUs)))
		parts = append(parts, fmt.Sprintf("structured_event_records=%d", summary.DetailEventCount))
		if summary.DetailOverwriteOK {
			parts = append(parts, fmt.Sprintf("detail_overwrite=%d", summary.DetailOverwrite))
		}
	}
	if len(summary.EventFieldCounts) > 0 {
		parts = append(parts, "event_families="+joinStringCounts(profilerFtraceEventFamilyCounts(summary.EventFieldCounts)))
		parts = append(parts, "event_names="+joinStringCounts(profilerFtraceEventNameCounts(summary.EventFieldCounts)))
	}
	if summary.SymbolCount > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", summary.SymbolCount))
		if len(summary.SymbolExamples) > 0 {
			parts = append(parts, "symbol_examples="+strings.Join(summary.SymbolExamples, ","))
		}
		if summary.SymbolTruncated {
			parts = append(parts, "symbol_examples_truncated=true")
		}
	}
	if len(summary.ClockDetails) > 0 {
		parts = append(parts, "clock_details="+strings.Join(summary.ClockDetails, ","))
	}
	if summary.ClockTruncated {
		parts = append(parts, "clock_details_truncated=true")
	}
	if issueSummary := summary.Issues.summary(); issueSummary != "" {
		parts = append(parts, "degraded="+issueSummary)
	}
	return "ftrace-plugin structured metadata: " + strings.Join(parts, "; ")
}

func profilerFtraceSummaryCoverage(summary profilerFtraceSummary) []TraceDBCoverage {
	if summary.Issues.empty() {
		return nil
	}
	total, ok := summary.Issues.totalOccurrences()
	if !ok {
		return nil
	}
	rowsRead, ok := profilerContainerCountToInt(total)
	if !ok {
		return nil
	}
	fields := map[string]string{
		"schema_profile": "TracePluginResult CPU stats/detail, symbols, clocks, and version metadata",
	}
	summary.Issues.appendFieldSources(fields)
	return []TraceDBCoverage{{
		Family:       "builtin_modern_ftrace:trace_plugin_metadata",
		Table:        "__trace_plugin_metadata__",
		Role:         "unsupported_input",
		Found:        true,
		RowsRead:     rowsRead,
		Skipped:      summary.Issues.summary(),
		FieldSources: fields,
	}}
}

func profilerFtraceEventFamilyCounts(counts map[int]int) map[string]int {
	out := map[string]int{}
	for field, count := range counts {
		desc, ok := profilerFtraceEventDescriptors[field]
		if !ok {
			out["unknown"] += count
			continue
		}
		out[desc.Family] += count
	}
	return out
}

func profilerFtraceEventNameCounts(counts map[int]int) map[string]int {
	out := map[string]int{}
	for field, count := range counts {
		desc, ok := profilerFtraceEventDescriptors[field]
		if !ok {
			out[fmt.Sprintf("event_field_%d", field)] += count
			continue
		}
		out[desc.Name] += count
	}
	return out
}

func joinStringCounts(values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] == 1 {
			parts = append(parts, key)
		} else {
			parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
		}
	}
	return strings.Join(parts, ",")
}

func profilerFtraceClockDetailLabel(clock profilerFtraceClockDetail) string {
	name := profilerFtraceClockName(clock.ID)
	var parts []string
	if clock.HasTime {
		parts = append(parts, fmt.Sprintf("time=%d.%09d", clock.TimeSec, clock.TimeNsec))
	}
	if clock.HasRes {
		parts = append(parts, fmt.Sprintf("res=%d.%09d", clock.ResSec, clock.ResNsec))
	}
	if len(parts) == 0 {
		return name
	}
	return name + "(" + strings.Join(parts, "/") + ")"
}

func profilerFtraceClockName(id uint64) string {
	switch id {
	case 1:
		return "BOOTTIME"
	case 2:
		return "REALTIME"
	case 3:
		return "REALTIME_COARSE"
	case 4:
		return "MONOTONIC"
	case 5:
		return "MONOTONIC_COARSE"
	case 6:
		return "MONOTONIC_RAW"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", id)
	}
}

func extractProfilerSessionPackage(ctx context.Context, path string, inputSize int64,
	sink *traceDBRowSink,
) (profilerContainerExtraction, error) {
	return extractProfilerSessionPackageWithLineLimit(ctx, path, inputSize, sink, maxProfilerTextLineBytes)
}

func extractProfilerSessionPackageWithLineLimit(ctx context.Context, path string, inputSize int64,
	sink *traceDBRowSink, maxLineBytes int,
) (profilerContainerExtraction, error) {
	if inputSize < 0 || maxLineBytes <= 0 || maxLineBytes >= math.MaxInt {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "invalid_profiler_session_line_limit"}
	}
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	if _, ok, err := profilerSessionJSONMarkerOffsetAt(f, inputSize, 64*1024); err != nil {
		return profilerContainerExtraction{}, err
	} else if !ok {
		return profilerContainerExtraction{}, nil
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
	coverage := TraceDBCoverage{
		Family: "builtin_modern_profiler",
		Table:  "session:SessionJSON",
		Role:   "query_ready_export",
		Found:  true,
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(f, 0, inputSize), profilerSessionReaderBufBytes)
	if !sink.beginPairRowCensus() {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_census_nested"}
	}
	oversizedLines := 0
	_, scanErr := scanProfilerBoundedSessionRecords(ctx, reader, maxLineBytes,
		func(record profilerBoundedPhysicalLine) (bool, error) {
			coverage.RowsRead++
			if record.Oversized {
				oversizedLines++
				out.SourceFailClosed = true
				out.SourceFailReason = "session_line_size_budget_exceeded"
				sink.failCloseAllRows()
				return false, nil
			}
			lineRows, rowErr := addSystraceRowsFromBytes(record.Bytes, &seq, sink)
			if rowErr != nil {
				return false, rowErr
			}
			coverage.RowsEmitted += lineRows
			out.TextRows += lineRows
			return true, nil
		})
	if scanErr != nil {
		coverage.Error = scanErr.Error()
		out.TraceCoverage = append(out.TraceCoverage, coverage)
		return profilerContainerExtraction{}, scanErr
	}
	if oversizedLines > 0 {
		out.RejectedMessages += oversizedLines
		traceDBAppendCoverageSkipped(&coverage,
			fmt.Sprintf("session_line_size_budget_exceeded=%d", oversizedLines))
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		coverage.FieldSources["max_session_line_bytes"] = strconv.Itoa(maxLineBytes)
		coverage.FieldSources["oversized_line_reader"] = "bounded_readslice_lf_nul_drain"
		coverage.FieldSources["profiler_trace_body_fail_closed"] = "all_rows"
		coverage.FieldSources["suffix_scan_stopped"] = "true"
		out.Caveats = append(out.Caveats, fmt.Sprintf(
			"rejected %d profiler SessionJSON LF/NUL-delimited record(s) above the %d-byte resource limit; oversized bytes were drained without retention, suffix scanning stopped, and the complete profiler trace-body source was failed closed before publication",
			oversizedLines, maxLineBytes))
	}
	staged := sink.endPairRowCensus()
	for _, kind := range profilerCaptureKinds {
		if staged[kind].total < 0 {
			return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_session_pair_staged_counter_negative"}
		}
		if staged[kind].total > 0 {
			if coverage.FieldSources == nil {
				coverage.FieldSources = map[string]string{}
			}
			coverage.FieldSources[profilerCoverageStagedRowsKey(kind)] = strconv.Itoa(staged[kind].total)
		}
	}
	var withheld profilerPairCensusSet
	withheldTotal := 0
	for _, kind := range profilerCaptureKinds {
		count, countErr := sink.withheldPairRowsFromCensusChecked(kind, staged[kind])
		if countErr != nil {
			return profilerContainerExtraction{}, countErr
		}
		withheld[kind].total = count
		if !checkedProfilerIntAddTo(&withheldTotal, count) {
			return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_session_pair_withheld_counter_overflow"}
		}
	}
	if withheldTotal > out.TextRows {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_session_pair_withheld_exceeds_staged"}
	}
	out.TextRows -= withheldTotal
	if out.SourceFailClosed {
		// The typed resource caveat above is the sole publication verdict.
	} else if out.TextRows > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from profiler session package payload", out.TextRows))
	} else if withheldTotal > 0 {
		traceDBAppendCoverageSkipped(&coverage,
			"session pair-critical endpoint rows were staged but withheld by exact-lane or source-family full-capture barriers")
		out.Caveats = append(out.Caveats, fmt.Sprintf("profiler session package staged exact pair-critical rows, but exact-lane or source-family full-capture barriers withheld them before publication: mmc=%d f2fs=%d block=%d",
			withheld[pairRenderMMC].total, withheld[pairRenderF2FS].total, withheld[pairRenderBlock].total))
	} else {
		traceDBAppendCoverageSkipped(&coverage,
			"session package did not contain directly renderable systrace text rows")
		out.Caveats = append(out.Caveats, "session package did not contain directly renderable systrace text rows; attach extracted sidecars or export ftrace/bytrace text with the official profiler tooling")
	}
	out.pairPublishers = append(out.pairPublishers, profilerPairPublisherCensus{
		coverageIndex: len(out.TraceCoverage), staged: staged,
	})
	out.TraceCoverage = append(out.TraceCoverage, coverage)
	if out.SourceFailClosed {
		failCloseProfilerTraceBody(&out, sink, out.SourceFailReason)
	}
	return out, nil
}

func scanProfilerBoundedSessionRecords(ctx context.Context, reader *bufio.Reader, maxBytes int,
	visit func(profilerBoundedPhysicalLine) (bool, error),
) (bool, error) {
	if ctx == nil {
		return false, &traceDBOutputInvariantError{Reason: "missing_profiler_session_record_context"}
	}
	if reader == nil || visit == nil || maxBytes <= 0 || maxBytes >= math.MaxInt {
		return false, &traceDBOutputInvariantError{Reason: "invalid_profiler_session_record_reader"}
	}
	retainedLimit := maxBytes + 1 // One terminal CR is compatible with a CRLF record.
	var record profilerBoundedPhysicalLine
	appendPart := func(part []byte) {
		if record.Oversized {
			return
		}
		if len(part) > retainedLimit-len(record.Bytes) {
			record.Bytes = nil
			record.Oversized = true
			return
		}
		record.Bytes = append(record.Bytes, part...)
	}
	emit := func(eof bool) (bool, error) {
		record.Present = true
		record.EOF = eof
		record = normalizeProfilerBoundedPhysicalLine(record, maxBytes)
		keepScanning, err := visit(record)
		record = profilerBoundedPhysicalLine{}
		return keepScanning, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		fragment, readErr := reader.ReadSlice('\n')
		start := 0
		for index, value := range fragment {
			if value != 0 && value != '\n' {
				continue
			}
			appendPart(fragment[start:index])
			keepScanning, err := emit(false)
			if err != nil || !keepScanning {
				return !keepScanning, err
			}
			start = index + 1
		}
		appendPart(fragment[start:])
		switch readErr {
		case nil, bufio.ErrBufferFull:
			continue
		case io.EOF:
			if len(record.Bytes) > 0 || record.Oversized {
				keepScanning, err := emit(true)
				return !keepScanning, err
			}
			return false, nil
		default:
			return false, readErr
		}
	}
}

func normalizeProfilerBoundedPhysicalLine(line profilerBoundedPhysicalLine,
	maxBytes int,
) profilerBoundedPhysicalLine {
	if line.Oversized {
		line.Bytes = nil
		return line
	}
	if len(line.Bytes) > 0 && line.Bytes[len(line.Bytes)-1] == '\r' {
		line.Bytes = line.Bytes[:len(line.Bytes)-1]
	}
	if len(line.Bytes) > maxBytes {
		line.Bytes = nil
		line.Oversized = true
	}
	return line
}

func profilerSessionJSONMarkerOffset(path string, maxProbe int64) (int64, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, false, err
	}
	return profilerSessionJSONMarkerOffsetAt(f, info.Size(), maxProbe)
}

func profilerSessionJSONMarkerOffsetAt(reader io.ReaderAt, inputSize, maxProbe int64) (int64, bool, error) {
	if reader == nil || inputSize <= 0 {
		return 0, false, nil
	}
	if maxProbe <= 0 {
		maxProbe = 64 * 1024
	}
	if maxProbe > inputSize {
		maxProbe = inputSize
	}
	if maxProbe > int64(math.MaxInt) {
		return 0, false, &traceDBOutputInvariantError{Reason: "invalid_profiler_session_marker_probe"}
	}
	probe := make([]byte, int(maxProbe))
	n, err := reader.ReadAt(probe, 0)
	if err != nil && err != io.EOF {
		return 0, false, err
	}
	idx := bytes.Index(probe[:n], []byte(profilerSessionJSONTag))
	if idx < 0 {
		return 0, false, nil
	}
	return int64(idx), true, nil
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
	length := binary.LittleEndian.Uint64(header[8:16])
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

func parseProfilerPluginData(data []byte) profilerPluginDataDecode {
	var decoded profilerPluginDataDecode
	var counts [9]int
	var valid [9]bool
	var uintValues [9]uint64
	var byteValues [9][]byte
	observeIssue := func(kind profilerPluginIssueKind, delta uint64) {
		if !decoded.IssueCensus.observe(kind, delta) {
			decoded.IssueOverflow = true
		}
	}

	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		if field < 1 || field > 8 {
			return nil
		}
		counts[field]++
		expectedWire := 0
		if field == 1 || field == 3 || field == 7 {
			expectedWire = 2
		}
		if wire != expectedWire {
			if kind, ok := profilerPluginWrongWireIssue(field); ok {
				observeIssue(kind, 1)
			}
			return nil
		}
		if counts[field] > 1 {
			return nil
		}
		valid[field] = true
		if wire == 2 {
			byteValues[field] = raw
		} else {
			uintValues[field] = value
		}
		return nil
	})
	if err != nil {
		observeIssue(profilerPluginIssueMalformedWire, 1)
	}
	for field := 1; field <= 8; field++ {
		if counts[field] > 1 {
			if !decoded.IssueCensus.observeDuplicate(field, uint64(counts[field]-1)) {
				decoded.IssueOverflow = true
			}
			valid[field] = false
		}
	}

	hardRejected := err != nil
	if counts[1] != 1 || !valid[1] {
		hardRejected = true
		if counts[1] == 0 {
			observeIssue(profilerPluginIssueNameMissing, 1)
		}
	} else if name := string(byteValues[1]); !traceDBSingleToken(name) {
		hardRejected = true
		valid[1] = false
		observeIssue(profilerPluginIssueNameInvalid, 1)
	} else {
		decoded.Plugin.Name = name
	}
	if counts[3] > 1 || counts[3] == 1 && !valid[3] {
		hardRejected = true
	} else if counts[3] == 1 {
		decoded.Plugin.Data = byteValues[3]
	}

	if counts[2] == 1 && valid[2] {
		if uintValues[2] > uint64(^uint32(0)) {
			valid[2] = false
			observeIssue(profilerPluginIssueStatusOutOfRange, 1)
		} else {
			decoded.Plugin.Status = uint32(uintValues[2])
			decoded.Plugin.StatusPresent = true
		}
	}
	if counts[4] == 1 && valid[4] {
		if uintValues[4] > 11 {
			valid[4] = false
			observeIssue(profilerPluginIssueClockIDOutOfRange, 1)
		} else {
			decoded.Plugin.ClockID = uintValues[4]
			decoded.Plugin.ClockIDPresent = true
		}
	}
	if counts[5] == 1 && valid[5] {
		decoded.Plugin.TvSec = uintValues[5]
		decoded.Plugin.TvSecPresent = true
	}
	if counts[6] == 1 && valid[6] {
		if uintValues[6] >= 1e9 {
			valid[6] = false
			observeIssue(profilerPluginIssueTVNsecOutOfRange, 1)
		} else {
			decoded.Plugin.TvNsec = uintValues[6]
			decoded.Plugin.TvNsecPresent = true
		}
	}
	if counts[7] == 1 && valid[7] {
		version := string(byteValues[7])
		if !traceDBSinglePhysicalLine(version, true) {
			valid[7] = false
			observeIssue(profilerPluginIssueVersionInvalid, 1)
		} else {
			decoded.Plugin.Version = version
			decoded.Plugin.VersionPresent = true
		}
	}
	if counts[8] == 1 && valid[8] {
		if uintValues[8] > uint64(^uint32(0)) {
			valid[8] = false
			observeIssue(profilerPluginIssueSampleIntervalOutOfRange, 1)
		} else {
			decoded.Plugin.SampleInterval = uint32(uintValues[8])
			decoded.Plugin.SampleIntervalPresent = true
		}
	}
	decoded.Plugin.ClockIDAmbiguous = counts[4] > 0 && (counts[4] != 1 || !valid[4])
	decoded.Plugin.TimeTupleAmbiguous = counts[5] > 0 && (counts[5] != 1 || !valid[5]) ||
		counts[6] > 0 && (counts[6] != 1 || !valid[6])
	if hardRejected || decoded.IssueOverflow {
		decoded.Plugin = profilerPluginData{}
		return decoded
	}
	decoded.Accepted = true
	return decoded
}

// profilerPayloadContainsExactBlockEndpoint is a provenance probe, not a text
// renderer. The first non-empty, non-comment physical fragment elects the
// origin: a source-neutral physical ftrace header elects text even when its
// task bytes also form protobuf keys; any other fragment permanently denies
// later embedded headers. Exact endpoint identity is proven separately. When
// text origin is absent, only typed PairFamilies can prove structured Block.
func profilerPayloadContainsExactBlockEndpoint(data []byte) bool {
	scan := scanProfilerStrictSystracePayload(data, nil)
	if scan.originText {
		// Complete strict text wins even when its leading task bytes also form a
		// syntactically complete TracePluginResult. A rejected text payload still
		// retains exact Block provenance when its first physical row established
		// the origin (malformed scalar or a later bad fragment).
		return scan.observed[pairRenderBlock]
	}
	// Without a first-row text origin, later header-looking bytes are metadata.
	// Only the typed structured family authority may then close Block.
	authority := decodeProfilerTracePluginResult(data)
	return authority.PairFamilies&pairCriticalFormatFamilyBlock != 0
}

// profilerRejectedPluginFrameContainsExactBlockEndpoint recovers only precise
// provenance from a rejected ProfilerPluginData frame. A payload is eligible
// solely when the bounded outer protobuf walk reached a complete field-3 bytes
// value; malformed bytes before that field, wrong-wire fields and metadata are
// never searched as text. A later outer decode failure does not erase an exact
// Block endpoint already proven inside a complete data field.
func profilerRejectedPluginFrameContainsExactBlockEndpoint(frame []byte) bool {
	found := false
	_ = walkProtoFields(frame, func(field int, wire int, raw []byte, _ uint64) error {
		if !found && field == 3 && wire == 2 && profilerPayloadContainsExactBlockEndpoint(raw) {
			found = true
		}
		return nil
	})
	return found
}

func addSystraceRowsFromBytes(data []byte, seq *int, sink *traceDBRowSink) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	rows := 0
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != '\n' && data[end] != 0 {
			end++
		}
		part := bytes.TrimSpace(data[start:end])
		if end < len(data) {
			start = end + 1
		} else {
			start = len(data)
		}
		if len(part) == 0 {
			continue
		}
		// Session/bytrace records use the same leading-# comment namespace as
		// strict ftrace compatibility payloads. Apply the negative gate before
		// size census and endpoint admission so a comment-looking Block header,
		// including an oversized one, cannot poison or publish a pairing lane.
		if part[0] == '#' {
			continue
		}
		if len(part) > maxProfilerTextLineBytes {
			if kind, governed, opaque := profilerTextPairCensus(part); governed {
				sink.poisonPairKind(kind)
			} else if opaque {
				sink.markPairCaptureOpaque(pairRenderMMC)
				sink.markPairCaptureOpaque(pairRenderF2FS)
			}
			continue
		}
		line := string(part)
		if line == "" {
			continue
		}
		if profilerTextPairNormalizationCollision(line) {
			continue
		}
		pair := profilerTextPairAdmission(line)
		if !pair.Admitted && (strings.Contains(line, "mmc_request_") || strings.Contains(line, "f2fs_") ||
			strings.Contains(line, "block_rq_") || strings.Contains(line, "block_bio_")) {
			// ParseLine deliberately rejects malformed timestamp/header rows. Ask
			// tracequery's precise raw endpoint authority before dropping them so
			// an exact physical MMC/F2FS/Block endpoint cannot become an invisible hole
			// between two otherwise valid rows. The local exact-name roster keeps
			// this adapter source-neutral and prevents prose/near-name poisoning.
			if probe, malformed := tracequery.ProbeMalformedPairingEndpoint(line); malformed {
				if kind, governed := profilerPairKindForExactName(probe.Name); governed {
					pair = profilerPairAdmission{
						Kind: kind, Governed: true,
						LaneKnown: probe.KeyKnown && probe.SemanticKey != "", Lane: probe.SemanticKey,
					}
				}
			}
		}
		header, headerKnown := tracequery.ProbePhysicalFtraceHeader(line)
		ts, timestampOK := header.TimestampNS, header.TimestampKnown
		if pair.Kind == pairRenderBlock {
			pair.HeaderOwnerKnown = headerKnown && header.OwnerKnown
			if !pair.HeaderOwnerKnown {
				// Block crosses contexts, so owner never enters the semantic key; it
				// is still mandatory provenance for exact-lane quarantine. Unknown
				// owner closes the source-local Block family instead of fabricating idle.
				pair.LaneKnown = false
				pair.Lane = ""
			}
		}
		headerKind, headerGoverned := profilerPairKindForExactName(header.EventName)
		headerGoverned = headerKnown && headerGoverned
		if !timestampOK {
			if pair.Governed {
				pair.poison(sink)
			}
			continue
		}
		if !headerKnown {
			if pair.Governed {
				pair.poison(sink)
			}
			continue
		}
		if sink == nil {
			return rows, fmt.Errorf("systrace row sink is nil")
		}
		row := renderedRow{tsNS: ts, seq: *seq, line: line}
		if pair.Governed {
			if headerGoverned && pair.Kind == headerKind {
				row.pairKind = pair.Kind
				row.pairLane = pair.Lane
				if !pair.Admitted {
					pair.poison(sink)
				}
			} else {
				// A complete outer header is the physical row authority. A raw
				// endpoint probe that disagrees with it must never relabel an
				// unrelated print/vendor row as a nested pair. Quarantine only the
				// proven raw lane/family and publish the outer row without pair
				// metadata.
				pair.poison(sink)
			}
		}
		if err := sink.add(row); err != nil {
			return rows, err
		}
		(*seq)++
		rows++
	}
	return rows, nil
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
