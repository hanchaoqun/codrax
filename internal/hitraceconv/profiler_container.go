package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
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

type profilerInputBinding struct {
	input           conversionInputView
	inputSize       int64
	sourceNamespace string
}

func newProfilerInputBinding(input conversionInputView, sourceNamespace string) (*profilerInputBinding, error) {
	if input == nil {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			"",
			errors.New("nil profiler conversion input"),
		)
	}
	inputSize := input.Size()
	if inputSize < 0 || strings.TrimSpace(sourceNamespace) == "" ||
		!filepath.IsAbs(sourceNamespace) || filepath.Clean(sourceNamespace) != sourceNamespace {
		return nil, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			input.DisplayPath(),
			fmt.Errorf("invalid profiler input binding: size=%d namespace=%q", inputSize, sourceNamespace),
		)
	}
	return &profilerInputBinding{
		input: input, inputSize: inputSize, sourceNamespace: sourceNamespace,
	}, nil
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
	Detected                   bool
	Kind                       string
	Messages                   int
	PluginMessages             map[string]int
	StructuredFtrace           int
	MalformedFtrace            int
	UnsupportedFtrace          int
	TextPluginMessages         int
	TextRows                   int
	StructuredRows             int
	RejectedMessages           int
	StandaloneDetected         bool
	SourceFailClosed           bool
	SourceFailReason           string
	TraceCoverage              []TraceDBCoverage
	Caveats                    []string
	publicationCaveatPending   bool
	publicationCaveatIndex     int
	terminalPublicationApplied bool
	profilerPublisherCoverage  profilerPublisherCoverageIndexes
	profilerEventCoverage      profilerFtraceEventCoverageIndexes
}

type profilerBoundedPhysicalLine struct {
	Bytes     []byte
	Present   bool
	Oversized bool
	EOF       bool
}

type profilerPublisherCoverageIndexes struct {
	Present [profilerPairPublisherSlotCount]bool
	Index   [profilerPairPublisherSlotCount]int
}

func (indexes *profilerPublisherCoverageIndexes) observe(
	publisher profilerPairPublisherSlot,
	coverageIndex int,
) bool {
	if indexes == nil || publisher == profilerPairPublisherNone ||
		publisher >= profilerPairPublisherSlotCount || coverageIndex < 0 {
		return false
	}
	if indexes.Present[publisher] {
		return indexes.Index[publisher] == coverageIndex
	}
	for slot := profilerPairPublisherSlot(1); slot < profilerPairPublisherSlotCount; slot++ {
		if indexes.Present[slot] && indexes.Index[slot] == coverageIndex {
			return false
		}
	}
	indexes.Present[publisher] = true
	indexes.Index[publisher] = coverageIndex
	return true
}

func (indexes profilerPublisherCoverageIndexes) coverageIndex(
	publisher profilerPairPublisherSlot,
) (int, bool) {
	if publisher == profilerPairPublisherNone || publisher >= profilerPairPublisherSlotCount ||
		!indexes.Present[publisher] {
		return 0, false
	}
	return indexes.Index[publisher], true
}

func profilerTerminalCountToInt(count uint64, reason string) (int, error) {
	if count > uint64(math.MaxInt) {
		return 0, &traceDBOutputInvariantError{Reason: reason}
	}
	return int(count), nil
}

func validateProfilerTerminalCoverageIndexes(
	extraction profilerContainerExtraction,
	terminal profilerTerminalPublicationLedger,
) error {
	if extraction.profilerPublisherCoverage.Present[profilerPairPublisherNone] {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_publisher_index_mismatch"}
	}
	for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
		rows := terminal.publishers[publisher]
		coverageIndex, present := extraction.profilerPublisherCoverage.coverageIndex(publisher)
		if !present {
			if rows.staged != 0 {
				return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_publisher_index_mismatch"}
			}
			continue
		}
		if coverageIndex < 0 || coverageIndex >= len(extraction.TraceCoverage) ||
			extraction.TraceCoverage[coverageIndex].RowsEmitted < 0 ||
			uint64(extraction.TraceCoverage[coverageIndex].RowsEmitted) != rows.staged {
			return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_publisher_coverage_mismatch"}
		}
		for previous := profilerPairPublisherSlot(1); previous < publisher; previous++ {
			previousIndex, previousPresent := extraction.profilerPublisherCoverage.coverageIndex(previous)
			if previousPresent && previousIndex == coverageIndex {
				return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_publisher_index_mismatch"}
			}
		}
	}
	for eventSlot := 0; eventSlot < profilerFtraceEventSlotCount; eventSlot++ {
		if !extraction.profilerEventCoverage.Present[eventSlot] {
			continue
		}
		coverageIndex := extraction.profilerEventCoverage.Index[eventSlot]
		if coverageIndex < 0 || coverageIndex >= len(extraction.TraceCoverage) {
			return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_event_coverage_index_mismatch"}
		}
		for previous := 0; previous < eventSlot; previous++ {
			if extraction.profilerEventCoverage.Present[previous] &&
				extraction.profilerEventCoverage.Index[previous] == coverageIndex {
				return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_coverage_index_collision"}
			}
		}
		for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
			publisherIndex, publisherPresent := extraction.profilerPublisherCoverage.coverageIndex(publisher)
			if publisherPresent && publisherIndex == coverageIndex {
				return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_coverage_index_collision"}
			}
		}
	}
	return nil
}

func validateProfilerTerminalSourceFailureProjection(
	extraction profilerContainerExtraction,
	sink *traceDBRowSink,
) error {
	if sink == nil || !extraction.SourceFailClosed || !sink.allRowsFailClosed ||
		extraction.TextRows != 0 || extraction.StructuredRows != 0 ||
		extraction.TextPluginMessages != 0 || sink.stats.RowsWritten != 0 ||
		sink.stats.RowsWithheld != sink.stats.RowsAccepted {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_source_fail_close_mismatch"}
	}
	for _, coverage := range extraction.TraceCoverage {
		if coverage.RowsEmitted != 0 {
			return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_source_fail_coverage_mismatch"}
		}
	}
	return nil
}

func cloneProfilerPublicationProjection(
	extraction profilerContainerExtraction,
) profilerContainerExtraction {
	cloned := extraction
	cloned.Caveats = append([]string(nil), extraction.Caveats...)
	cloned.TraceCoverage = append([]TraceDBCoverage(nil), extraction.TraceCoverage...)
	for index := range cloned.TraceCoverage {
		fields := extraction.TraceCoverage[index].FieldSources
		if fields == nil {
			continue
		}
		cloned.TraceCoverage[index].FieldSources = make(map[string]string, len(fields))
		for key, value := range fields {
			cloned.TraceCoverage[index].FieldSources[key] = value
		}
	}
	return cloned
}

func insertProfilerPublicationCaveats(
	extraction *profilerContainerExtraction,
	terminal profilerTerminalPublicationLedger,
) error {
	if extraction == nil || !extraction.publicationCaveatPending ||
		extraction.terminalPublicationApplied {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_caveat_state_invalid"}
	}
	if extraction.publicationCaveatIndex < 0 ||
		extraction.publicationCaveatIndex > len(extraction.Caveats) {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_caveat_index_invalid"}
	}
	var caveats []string
	if extraction.Kind == "openharmony_profiler_session_package" {
		if !extraction.SourceFailClosed {
			coverageIndex, present := extraction.profilerPublisherCoverage.coverageIndex(
				profilerPairPublisherSession,
			)
			if !present || coverageIndex < 0 || coverageIndex >= len(extraction.TraceCoverage) {
				return &traceDBOutputInvariantError{Reason: "profiler_session_coverage_index_mismatch"}
			}
			coverage := &extraction.TraceCoverage[coverageIndex]
			switch {
			case extraction.TextRows > 0:
				caveats = append(caveats, fmt.Sprintf(
					"extracted %d systrace text row(s) from profiler session package payload",
					extraction.TextRows))
			case terminal.textRows.withheld > 0:
				traceDBAppendCoverageSkipped(coverage,
					"session pair-critical endpoint rows were staged but withheld by exact-lane or source-family full-capture barriers")
				caveats = append(caveats, fmt.Sprintf(
					"profiler session package staged exact pair-critical rows, but exact-lane or source-family full-capture barriers withheld them before publication: mmc=%d f2fs=%d block=%d",
					terminal.publisherFamilies[profilerPairPublisherSession][pairRenderMMC].withheld,
					terminal.publisherFamilies[profilerPairPublisherSession][pairRenderF2FS].withheld,
					terminal.publisherFamilies[profilerPairPublisherSession][pairRenderBlock].withheld))
			default:
				traceDBAppendCoverageSkipped(coverage,
					"session package did not contain directly renderable systrace text rows")
				caveats = append(caveats,
					"session package did not contain directly renderable systrace text rows; attach extracted sidecars or export ftrace/bytrace text with the official profiler tooling")
			}
		}
	} else {
		if extraction.Messages == 0 {
			caveats = append(caveats,
				"official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable")
		}
		if extraction.SourceFailClosed && extraction.StructuredFtrace > 0 {
			caveats = append(caveats, fmt.Sprintf(
				"decoded %d authoritative ftrace-plugin TracePluginResult message(s), but all structured rows were withheld by the profiler trace-body source fail-close",
				extraction.StructuredFtrace))
		} else if extraction.StructuredFtrace > 0 {
			caveats = append(caveats, fmt.Sprintf(
				"decoded %d authoritative ftrace-plugin TracePluginResult message(s) and rendered %d structured trace row(s); unsupported or degraded members remain explicit in typed coverage",
				extraction.StructuredFtrace, extraction.StructuredRows))
		}
		if extraction.MalformedFtrace > 0 {
			caveats = append(caveats, fmt.Sprintf(
				"classified %d ftrace-plugin payload(s) as malformed TracePluginResult; no partial structured or text rows were published",
				extraction.MalformedFtrace))
		}
		if extraction.TextRows > 0 {
			caveats = append(caveats, fmt.Sprintf(
				"extracted %d systrace text row(s) from %d profiler plugin message(s)",
				extraction.TextRows, extraction.TextPluginMessages))
		}
	}
	if len(caveats) > 0 {
		insertAt := extraction.publicationCaveatIndex
		merged := make([]string, 0, len(extraction.Caveats)+len(caveats))
		merged = append(merged, extraction.Caveats[:insertAt]...)
		merged = append(merged, caveats...)
		merged = append(merged, extraction.Caveats[insertAt:]...)
		extraction.Caveats = merged
	}
	extraction.publicationCaveatPending = false
	extraction.terminalPublicationApplied = true
	return nil
}

// applyProfilerTerminalPublication is the sole post-seal bridge from the
// authenticated source-order terminal ledger into customer-visible row and
// message counts, coverage, and publication caveats. It builds the complete
// projection on private copies and swaps it only after every staged authority
// validates, so callers never observe a partially projected view.
func applyProfilerTerminalPublication(
	extraction *profilerContainerExtraction,
	sink *traceDBRowSink,
) (profilerTerminalPublicationLedger, error) {
	if extraction == nil {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_extraction_missing",
		}
	}
	if extraction.terminalPublicationApplied || !extraction.publicationCaveatPending {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_already_applied",
		}
	}
	if sink == nil || sink.captureLifecycle != profilerCaptureSealed {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_parity_state_invalid",
		}
	}
	if extraction.TextPluginMessages < 0 {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_message_count_invalid",
		}
	}
	if extraction.SourceFailClosed != sink.allRowsFailClosed {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_source_fail_close_state_mismatch",
		}
	}
	terminal := profilerTerminalPublicationLedger{}
	if sink.stats.RowsAccepted == 0 {
		if sink.sourceOrderSidecar.present() || sink.nextTextMessage != 0 ||
			extraction.TextRows != 0 || extraction.StructuredRows != 0 ||
			extraction.TextPluginMessages != 0 {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_zero_row_mismatch",
			}
		}
		if err := validateProfilerTerminalCoverageIndexes(*extraction, terminal); err != nil {
			return profilerTerminalPublicationLedger{}, err
		}
		for _, coverage := range extraction.TraceCoverage {
			if coverage.RowsEmitted != 0 {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_zero_row_coverage_mismatch",
				}
			}
		}
		if extraction.SourceFailClosed {
			if err := validateProfilerTerminalSourceFailureProjection(*extraction, sink); err != nil {
				return profilerTerminalPublicationLedger{}, err
			}
		}
	} else if !sink.sourceOrderSidecar.present() {
		// A registered-run/sidecar construction integrity failure deliberately
		// produces no substitute terminal ledger. Its sole legal result is the
		// already-sealed source-wide empty publication.
		if extraction.SourceFailClosed && sink.captureSourceFailure != "" && sink.allRowsFailClosed {
			if err := validateProfilerTerminalSourceFailureProjection(*extraction, sink); err != nil {
				return profilerTerminalPublicationLedger{}, err
			}
		} else {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_ledger_missing",
			}
		}
	} else {
		terminal = sink.sourceOrderSidecar.terminal
		if err := sink.validateProfilerTerminalPublicationLedger(terminal); err != nil {
			return profilerTerminalPublicationLedger{}, err
		}
	}
	if terminal.sourceNeutralRows != (profilerTerminalPublicationCounts{}) ||
		terminal.publishers[profilerPairPublisherNone] != (profilerTerminalPublicationCounts{}) {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_source_neutral_row",
		}
	}
	if extraction.Kind == "openharmony_profiler_session_package" {
		if extraction.TextPluginMessages != 0 || terminal.textMessages != (profilerTerminalTextMessageLedger{}) ||
			terminal.structuredRows != (profilerTerminalPublicationCounts{}) {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_session_message_mismatch",
			}
		}
		for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
			if publisher != profilerPairPublisherSession &&
				terminal.publishers[publisher] != (profilerTerminalPublicationCounts{}) {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_session_publisher_mismatch",
				}
			}
		}
		if _, present := extraction.profilerPublisherCoverage.coverageIndex(
			profilerPairPublisherSession,
		); !present {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_publisher_index_mismatch",
			}
		}
	} else if terminal.publishers[profilerPairPublisherSession] != (profilerTerminalPublicationCounts{}) {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_session_publisher_mismatch",
		}
	}
	if extraction.SourceFailClosed {
		if terminal.rows.published != 0 {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_source_fail_close_mismatch",
			}
		}
		if err := validateProfilerTerminalSourceFailureProjection(*extraction, sink); err != nil {
			return profilerTerminalPublicationLedger{}, err
		}
		projected := cloneProfilerPublicationProjection(*extraction)
		if err := insertProfilerPublicationCaveats(&projected, terminal); err != nil {
			return profilerTerminalPublicationLedger{}, err
		}
		*extraction = projected
		return terminal, nil
	}
	stagedTextRows, err := profilerTerminalCountToInt(
		terminal.textRows.staged, "profiler_terminal_publication_text_rows_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	stagedStructuredRows, err := profilerTerminalCountToInt(
		terminal.structuredRows.staged, "profiler_terminal_publication_structured_rows_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	stagedMessages, err := profilerTerminalCountToInt(
		terminal.textMessages.staged, "profiler_terminal_publication_message_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	if stagedTextRows != extraction.TextRows || stagedStructuredRows != extraction.StructuredRows {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_row_class_parity_mismatch",
		}
	}
	if stagedMessages != extraction.TextPluginMessages {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_message_count_mismatch",
		}
	}
	if terminal.textMessages.fullyWithheld > terminal.textMessages.pairBearing {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_message_verdict_mismatch",
		}
	}
	publishableRows, err := sink.profilerPublishableRows()
	if err != nil || publishableRows < 0 || uint64(publishableRows) != terminal.rows.published {
		if err != nil {
			return profilerTerminalPublicationLedger{}, err
		}
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_sink_row_mismatch",
		}
	}
	var pairWithheld uint64
	var structuredPairWithheld uint64
	for _, kind := range profilerCaptureKinds {
		if !checkedProfilerUint64AddTo(&pairWithheld, terminal.pairFamilies[kind].withheld) ||
			!checkedProfilerUint64AddTo(&structuredPairWithheld,
				terminal.structuredPairFamilies[kind].withheld) {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_withheld_counter_overflow",
			}
		}
	}
	if structuredPairWithheld > pairWithheld || terminal.rows.withheld != pairWithheld ||
		terminal.structuredRows.withheld != structuredPairWithheld ||
		terminal.textRows.withheld != pairWithheld-structuredPairWithheld {
		return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
			Reason: "profiler_terminal_publication_withheld_conservation_mismatch",
		}
	}
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.structuredField == 0 {
			continue
		}
		counts := terminal.structuredEndpoints[descriptor.slot]
		coverageIndex, present := extraction.profilerEventCoverage.coverageIndexForField(
			descriptor.structuredField,
		)
		if !present {
			if counts.staged != 0 {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_event_coverage_mismatch",
				}
			}
			continue
		}
		if coverageIndex < 0 || coverageIndex >= len(extraction.TraceCoverage) ||
			extraction.TraceCoverage[coverageIndex].RowsEmitted < 0 ||
			uint64(extraction.TraceCoverage[coverageIndex].RowsEmitted) != counts.staged {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_event_coverage_mismatch",
			}
		}
	}
	if err := validateProfilerTerminalCoverageIndexes(*extraction, terminal); err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
		publisherRows := terminal.publishers[publisher]
		publisherPairRows := uint64(0)
		publisherWithheld := uint64(0)
		for _, kind := range profilerCaptureKinds {
			counts := terminal.publisherFamilies[publisher][kind]
			if !checkedProfilerUint64AddTo(&publisherPairRows, counts.staged) ||
				!checkedProfilerUint64AddTo(&publisherWithheld, counts.withheld) {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_publisher_overflow",
				}
			}
			coverageIndex, present := extraction.profilerPublisherCoverage.coverageIndex(publisher)
			if counts.staged == 0 {
				if present {
					_, fieldPresent := extraction.TraceCoverage[coverageIndex].FieldSources[profilerCoverageStagedRowsKey(kind)]
					if fieldPresent {
						return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
							Reason: "profiler_terminal_publication_publisher_family_coverage_mismatch",
						}
					}
				}
				continue
			}
			if !present {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_publisher_index_mismatch",
				}
			}
			raw, fieldPresent := extraction.TraceCoverage[coverageIndex].FieldSources[profilerCoverageStagedRowsKey(kind)]
			if !fieldPresent || raw != strconv.FormatUint(counts.staged, 10) {
				return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
					Reason: "profiler_terminal_publication_publisher_family_coverage_mismatch",
				}
			}
		}
		if publisherPairRows > publisherRows.staged || publisherWithheld != publisherRows.withheld {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_publisher_verdict_mismatch",
			}
		}
	}
	projected := cloneProfilerPublicationProjection(*extraction)
	for publisher := profilerPairPublisherSlot(1); publisher < profilerPairPublisherSlotCount; publisher++ {
		coverageIndex, present := extraction.profilerPublisherCoverage.coverageIndex(publisher)
		if !present {
			continue
		}
		if _, exists := extraction.TraceCoverage[coverageIndex].FieldSources["complete_capture_withheld_rows"]; exists {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_coverage_already_projected",
			}
		}
		published, convertErr := profilerTerminalCountToInt(
			terminal.publishers[publisher].published,
			"profiler_terminal_publication_publisher_rows_overflow",
		)
		if convertErr != nil {
			return profilerTerminalPublicationLedger{}, convertErr
		}
		projected.TraceCoverage[coverageIndex].RowsEmitted = published
		if terminal.publishers[publisher].withheld > 0 {
			if projected.TraceCoverage[coverageIndex].FieldSources == nil {
				projected.TraceCoverage[coverageIndex].FieldSources = map[string]string{}
			}
			projected.TraceCoverage[coverageIndex].FieldSources["complete_capture_withheld_rows"] =
				strconv.FormatUint(terminal.publishers[publisher].withheld, 10)
		}
	}
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.structuredField == 0 {
			continue
		}
		coverageIndex, present := extraction.profilerEventCoverage.coverageIndexForField(
			descriptor.structuredField,
		)
		if !present {
			continue
		}
		if _, exists := extraction.TraceCoverage[coverageIndex].FieldSources["complete_capture_withheld_rows"]; exists {
			return profilerTerminalPublicationLedger{}, &traceDBOutputInvariantError{
				Reason: "profiler_terminal_publication_event_coverage_already_projected",
			}
		}
		counts := terminal.structuredEndpoints[descriptor.slot]
		published, convertErr := profilerTerminalCountToInt(
			counts.published,
			"profiler_terminal_publication_event_rows_overflow",
		)
		if convertErr != nil {
			return profilerTerminalPublicationLedger{}, convertErr
		}
		projected.TraceCoverage[coverageIndex].RowsEmitted = published
		if counts.withheld > 0 {
			if projected.TraceCoverage[coverageIndex].FieldSources == nil {
				projected.TraceCoverage[coverageIndex].FieldSources = map[string]string{}
			}
			projected.TraceCoverage[coverageIndex].FieldSources["complete_capture_withheld_rows"] =
				strconv.FormatUint(counts.withheld, 10)
		}
	}
	publishedTextRows, err := profilerTerminalCountToInt(
		terminal.textRows.published, "profiler_terminal_publication_text_rows_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	publishedStructuredRows, err := profilerTerminalCountToInt(
		terminal.structuredRows.published, "profiler_terminal_publication_structured_rows_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	publishedMessages, err := profilerTerminalCountToInt(
		terminal.textMessages.published, "profiler_terminal_publication_message_overflow",
	)
	if err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	projected.TextRows = publishedTextRows
	projected.StructuredRows = publishedStructuredRows
	projected.TextPluginMessages = publishedMessages
	if err := insertProfilerPublicationCaveats(&projected, terminal); err != nil {
		return profilerTerminalPublicationLedger{}, err
	}
	*extraction = projected
	return terminal, nil
}

func validateProfilerTerminalWrittenProjection(
	extraction profilerContainerExtraction,
	terminal profilerTerminalPublicationLedger,
	sink *traceDBRowSink,
) error {
	if sink == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_sink_missing"}
	}
	expectedAccepted, err := profilerTerminalCountToInt(
		terminal.rows.staged, "profiler_terminal_publication_rows_overflow",
	)
	if err != nil {
		return err
	}
	expectedWritten, err := profilerTerminalCountToInt(
		terminal.rows.published, "profiler_terminal_publication_rows_overflow",
	)
	if err != nil {
		return err
	}
	expectedWithheld, err := profilerTerminalCountToInt(
		terminal.rows.withheld, "profiler_terminal_publication_rows_overflow",
	)
	if err != nil {
		return err
	}
	// A sidecar-construction integrity failure has no authenticated terminal
	// ledger by design. Its already-validated source-wide empty projection is
	// closed directly against the sink's accepted count.
	if extraction.SourceFailClosed && terminal.rows == (profilerTerminalPublicationCounts{}) &&
		sink.stats.RowsAccepted > 0 {
		expectedAccepted = sink.stats.RowsAccepted
		expectedWritten = 0
		expectedWithheld = sink.stats.RowsAccepted
	}
	if sink.stats.RowsAccepted != expectedAccepted || sink.stats.RowsWritten != expectedWritten ||
		sink.stats.RowsWithheld != expectedWithheld {
		return &traceDBOutputInvariantError{Reason: "profiler_terminal_publication_written_account_mismatch"}
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
	VersionObservations      uint64
	VersionSamples           profilerStableSampleSet
	StatsMessages            uint64
	StartStats               uint64
	EndStats                 uint64
	TraceClockObserved       uint64
	TraceClockSamples        profilerStableSampleSet
	StatsCPUs                profilerSummaryCPUSet
	StartTotals              profilerFtraceCPUTotals
	EndTotals                profilerFtraceCPUTotals
	StartTotalsSeen          bool
	EndTotalsSeen            bool
	StartTotalsValid         bool
	EndTotalsValid           bool
	DetailMessages           uint64
	DetailCPUs               profilerSummaryCPUSet
	DetailEventCount         uint64
	DetailOverwrite          uint64
	DetailOverwriteOK        bool
	SymbolCount              uint64
	SymbolNamedCount         uint64
	SymbolSamples            profilerStableSampleSet
	SymbolTruncated          bool
	ClockDetailCount         uint64
	ClockDetailSamples       profilerStableSampleSet
	ClockTruncated           bool
	KnownEventCounts         [len(profilerFtraceEventDescriptorList)]uint64
	UnknownEventCount        uint64
	UnknownEventFieldSamples profilerStableSampleSet
	Issues                   profilerFtraceSummaryIssueCensus
	IssueOverflow            bool
	recognizedMessage        bool
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
	Status            uint64
	Clock             string
	payload           []byte
	PerCPUOccurrences uint64
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
	CPU                      uint64
	EventCount               uint64
	KnownEventCounts         [len(profilerFtraceEventDescriptorList)]uint64
	UnknownEventCount        uint64
	UnknownEventFieldSamples profilerStableSampleSet
	Overwrite                uint64
	OverwriteValid           bool
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

func tryConvertProfilerContainerWithLedger(ctx context.Context, opts Options, authority *conversionInputAuthority, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision, initialTraceDecisions []TraceProviderDecision, initialTraceDBCoverage []TraceDBCoverage, ledger *conversionFileLedger) (result Result, detected bool, err error) {
	if authority == nil {
		return Result{}, false, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerHeader,
			"",
			errors.New("nil profiler input authority"),
		)
	}
	binding, err := newProfilerInputBinding(authority, authority.CanonicalPath())
	if err != nil {
		return Result{}, false, err
	}
	inputSize := binding.inputSize
	opts.InputPath = authority.DisplayPath()
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
	if err := sink.openProfilerCaptureForNamespace(binding.sourceNamespace); err != nil {
		return Result{}, false, err
	}
	sessionBodySize, embeddedSessionSidecar := profilerContainerSessionLayout(inputSize, standaloneArtifacts)
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
		ctx, binding, sessionBodySize, sink)
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
	if err := completeConversionInputStage(ctx, binding.input, conversionInputStageProfilerBody, nil); err != nil {
		return Result{}, true, err
	}
	if err := sink.sealProfilerCaptureContext(ctx); err != nil {
		return Result{}, true, err
	}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		return Result{}, true, err
	}
	terminal, err := applyProfilerTerminalPublication(&extracted, sink)
	if err != nil {
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
		withheld, countErr := profilerTerminalCountToInt(
			terminal.pairFamilies[pairRenderMMC].withheld,
			"profiler_terminal_publication_mmc_withheld_overflow",
		)
		if countErr != nil {
			return Result{}, true, countErr
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler MMC full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; malformed, opaque, or unattributable exact MMC endpoints remain coverage-only%s", withheld, profilerPairBudgetCaveat(sink, pairRenderMMC)))
		result.TraceCoverage = append(result.TraceCoverage, profilerMMCPairBarrierCoverage(withheld, sink))
	}
	if !extracted.SourceFailClosed && sink.pairKindPoisoned(pairRenderF2FS) {
		withheld, countErr := profilerTerminalCountToInt(
			terminal.pairFamilies[pairRenderF2FS].withheld,
			"profiler_terminal_publication_f2fs_withheld_overflow",
		)
		if countErr != nil {
			return Result{}, true, countErr
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler F2FS full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; known-key failures remain exact-lane coverage-only, while malformed, opaque, or unattributable F2FS endpoints close the source-local family%s", withheld, profilerPairBudgetCaveat(sink, pairRenderF2FS)))
		result.TraceCoverage = append(result.TraceCoverage, profilerF2FSPairBarrierCoverage(withheld, sink))
	}
	if !extracted.SourceFailClosed && sink.pairKindPoisoned(pairRenderBlock) {
		withheld, countErr := profilerTerminalCountToInt(
			terminal.pairFamilies[pairRenderBlock].withheld,
			"profiler_terminal_publication_block_withheld_overflow",
		)
		if countErr != nil {
			return Result{}, true, countErr
		}
		result.Caveats = append(result.Caveats, fmt.Sprintf("profiler Block full-capture anti-rescue barrier failed closed before output: withheld_rows=%d; known owner/source/hard-key failures remain exact-lane coverage-only, while opaque or unattributable Block endpoints close the source-local family%s", withheld, profilerPairBudgetCaveat(sink, pairRenderBlock)))
		result.TraceCoverage = append(result.TraceCoverage, profilerBlockPairBarrierCoverage(withheld, sink))
	}
	publishableRows, err := profilerTerminalCountToInt(
		terminal.rows.published, "profiler_terminal_publication_rows_overflow",
	)
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
		if err := validateProfilerTerminalWrittenProjection(extracted, terminal, sink); err != nil {
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
		if err := validateProfilerTerminalWrittenProjection(extracted, terminal, sink); err != nil {
			return Result{}, true, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(output))
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
			traceProviderInventoryPublished(
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
	if err := finalizeResultTraceBundleWithLedger(ctx, opts.InputPath, result.OutputPath, &result, ledger); err != nil {
		return Result{}, true, err
	}
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

func validateProfilerInputBinding(binding *profilerInputBinding, stage conversionInputStage) error {
	if binding == nil || binding.input == nil {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			stage,
			"",
			errors.New("nil profiler input binding"),
		)
	}
	currentSize := binding.input.Size()
	if binding.inputSize < 0 || binding.inputSize != currentSize ||
		strings.TrimSpace(binding.sourceNamespace) == "" ||
		!filepath.IsAbs(binding.sourceNamespace) || filepath.Clean(binding.sourceNamespace) != binding.sourceNamespace {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			stage,
			binding.input.DisplayPath(),
			fmt.Errorf("invalid profiler input binding: fixed_size=%d current_size=%d namespace=%q", binding.inputSize, currentSize, binding.sourceNamespace),
		)
	}
	return nil
}

func readProfilerTraceHeaderFromInput(ctx context.Context, binding *profilerInputBinding) (
	header profilerTraceHeader,
	ok bool,
	err error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	var input conversionInputView
	if binding != nil {
		input = binding.input
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageProfilerHeader, nil); err != nil {
		return profilerTraceHeader{}, false, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageProfilerHeader, err)
		if err != nil {
			header = profilerTraceHeader{}
			ok = false
		}
	}()
	if err := validateProfilerInputBinding(binding, conversionInputStageProfilerHeader); err != nil {
		return profilerTraceHeader{}, false, err
	}
	return readProfilerTraceHeaderAtExact(input, 0, binding.inputSize)
}

func extractProfilerContainerSystraceRowsWithSessionLimitFromInput(ctx context.Context,
	binding *profilerInputBinding, sessionInputSize int64, sink *traceDBRowSink,
) (profilerContainerExtraction, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	header, ok, err := readProfilerTraceHeaderFromInput(ctx, binding)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if err := ctx.Err(); err != nil {
		return profilerContainerExtraction{}, err
	}
	if sessionInputSize < 0 || sessionInputSize > binding.inputSize {
		return profilerContainerExtraction{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerBody,
			binding.input.DisplayPath(),
			fmt.Errorf("invalid profiler session input boundary %d for fixed size %d", sessionInputSize, binding.inputSize),
		)
	}
	if ok && header.DataType == profilerDataTypeProtobuf {
		return extractProfilerTraceFileFromInput(ctx, binding, header, sink, maxProfilerPluginFrameBytes)
	}
	session, err := extractProfilerSessionPackageFromInput(ctx, binding, sessionInputSize, sink, maxProfilerTextLineBytes)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if session.Detected {
		return session, nil
	}
	return profilerContainerExtraction{}, nil
}

func extractProfilerTraceFileFromInput(ctx context.Context, binding *profilerInputBinding,
	header profilerTraceHeader, sink *traceDBRowSink, maxFrameBytes uint64,
) (extracted profilerContainerExtraction, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var input conversionInputView
	if binding != nil {
		input = binding.input
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageProfilerBody, nil); err != nil {
		return profilerContainerExtraction{}, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageProfilerBody, err)
		if err != nil {
			extracted = profilerContainerExtraction{}
			var inputErr *ConversionInputError
			if sink != nil && errors.As(err, &inputErr) {
				sink.failCloseAllRows()
			}
		}
	}()
	if err := validateProfilerInputBinding(binding, conversionInputStageProfilerBody); err != nil {
		return profilerContainerExtraction{}, err
	}
	return extractProfilerTraceFileAtWithFrameLimit(ctx, input, binding.inputSize, header, sink, maxFrameBytes)
}

func extractProfilerTraceFileAtWithFrameLimit(ctx context.Context, reader io.ReaderAt, inputSize int64,
	header profilerTraceHeader, sink *traceDBRowSink, maxFrameBytes uint64,
) (profilerContainerExtraction, error) {
	if reader == nil || maxFrameBytes == 0 || maxFrameBytes > uint64(math.MaxInt) {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "invalid_profiler_plugin_frame_limit"}
	}
	if err := sink.enableProfilerTraceClassification(); err != nil {
		return profilerContainerExtraction{}, err
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
		decoded, decodeErr := parseProfilerPluginDataContext(ctx, msg)
		if decodeErr != nil {
			return profilerContainerExtraction{}, decodeErr
		}
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
			publisherSlot, publisherKnown := profilerPairPublisherForRoute(route)
			if !publisherKnown || !sink.beginPairRowCensusForPublisher(publisherSlot) {
				return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_census_nested"}
			}
			appendPluginCoverage := func() (bool, error) {
				staged := sink.endPairRowCensus()
				coverageIndex, ok, observeErr := diagnostics.observeAcceptedContext(ctx, &out, route, name, plugin, decoded.IssueCensus,
					off, outcome, coverage.RowsEmitted, staged)
				if observeErr != nil {
					return false, observeErr
				}
				if !ok {
					return false, nil
				}
				if !out.profilerPublisherCoverage.observe(publisherSlot, coverageIndex) {
					return false, &traceDBOutputInvariantError{
						Reason: "profiler_publisher_coverage_index_mismatch",
					}
				}
				return true, nil
			}
			if route == profilerPluginRouteExactFtrace {
				strictStage, strictStageErr := stageProfilerStrictSystracePayloadContext(ctx, plugin.Data)
				if strictStageErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, strictStageErr
				}
				if strictStage.scan.originText {
					if !sink.beginProfilerTextMessage() {
						_ = sink.endPairRowCensus()
						return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_message_begin_state_invalid"}
					}
					rows, textPayload, rowErr := addProfilerStrictSystraceStageContext(ctx, strictStage, &seq, sink)
					if rowErr != nil {
						sink.abortProfilerTextMessage()
						coverage.Error = rowErr.Error()
						_ = sink.endPairRowCensus()
						return profilerContainerExtraction{}, rowErr
					}
					if messageErr := sink.endProfilerTextMessage(rows); messageErr != nil {
						_ = sink.endPairRowCensus()
						return profilerContainerExtraction{}, messageErr
					}
					coverage.RowsEmitted = rows
					if textPayload {
						outcome = profilerPluginOutcomeStrictText
						if !checkedProfilerIntAddTo(&out.TextRows, rows) || !checkedProfilerIntAddTo(&out.TextPluginMessages, 1) {
							_ = sink.endPairRowCensus()
							profilerContainerCounterFailClose(&out, sink)
							break frames
						}
						coverageOK, coverageErr := appendPluginCoverage()
						if coverageErr != nil {
							return profilerContainerExtraction{}, coverageErr
						}
						if !coverageOK {
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
					coverageOK, coverageErr := appendPluginCoverage()
					if coverageErr != nil {
						return profilerContainerExtraction{}, coverageErr
					}
					if !coverageOK {
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
					coverageOK, coverageErr := appendPluginCoverage()
					if coverageErr != nil {
						return profilerContainerExtraction{}, coverageErr
					}
					if !coverageOK {
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
				structuredRows, eventBatch, summary, ok, renderErr := renderProfilerFtraceStructuredResultForContainerFusedContext(ctx, authority, &seq, sink)
				if renderErr != nil {
					coverage.Error = renderErr.Error()
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, renderErr
				}
				frameObserved, observeErr := diagnostics.observeFtraceFrameContext(ctx, &summary, ok, eventBatch, off)
				if observeErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, observeErr
				}
				if !frameObserved {
					_ = sink.endPairRowCensus()
					profilerContainerCounterFailClose(&out, sink)
					break frames
				}
				coverage.RowsEmitted = structuredRows
				if !checkedProfilerIntAddTo(&out.StructuredRows, structuredRows) {
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
			} else if route == profilerPluginRouteNoncanonicalFtrace {
				blockOpaque, probeErr := profilerPayloadContainsExactBlockEndpointContext(ctx, plugin.Data)
				if probeErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, probeErr
				}
				outcome = profilerPluginOutcomeNoncanonicalFtrace
				if !checkedProfilerIntAddTo(&out.UnsupportedFtrace, 1) {
					_ = sink.endPairRowCensus()
					profilerContainerCounterFailClose(&out, sink)
					break frames
				}
				sink.markPairCaptureOpaque(pairRenderMMC)
				sink.markPairCaptureOpaque(pairRenderF2FS)
				if blockOpaque {
					sink.markPairCaptureOpaque(pairRenderBlock)
				}
			} else if len(plugin.Data) == 0 {
				outcome = profilerPluginOutcomeEmptyPayload
			} else {
				if !sink.beginProfilerTextMessage() {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_text_message_begin_state_invalid"}
				}
				rows, rowErr := addSystraceRowsFromBytesContext(ctx, plugin.Data, &seq, sink)
				if rowErr != nil {
					sink.abortProfilerTextMessage()
					coverage.Error = rowErr.Error()
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, rowErr
				}
				if messageErr := sink.endProfilerTextMessage(rows); messageErr != nil {
					_ = sink.endPairRowCensus()
					return profilerContainerExtraction{}, messageErr
				}
				coverage.RowsEmitted = rows
				if rows > 0 {
					outcome = profilerPluginOutcomeTextRows
					if !checkedProfilerIntAddTo(&out.TextRows, rows) || !checkedProfilerIntAddTo(&out.TextPluginMessages, 1) {
						_ = sink.endPairRowCensus()
						profilerContainerCounterFailClose(&out, sink)
						break frames
					}
				} else {
					outcome = profilerPluginOutcomeNoTextRows
				}
			}
			coverageOK, coverageErr := appendPluginCoverage()
			if coverageErr != nil {
				return profilerContainerExtraction{}, coverageErr
			}
			if !coverageOK {
				profilerContainerCounterFailClose(&out, sink)
				break frames
			}
		} else {
			blockOpaque, probeErr := profilerRejectedPluginFrameContainsExactBlockEndpointContext(ctx, msg)
			if probeErr != nil {
				return profilerContainerExtraction{}, probeErr
			}
			out.RejectedMessages++
			sink.markPairCaptureOpaque(pairRenderMMC)
			sink.markPairCaptureOpaque(pairRenderF2FS)
			if blockOpaque {
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
	if out.SourceFailClosed {
		failCloseProfilerTraceBody(&out, sink, out.SourceFailReason)
	}
	out.publicationCaveatPending = true
	out.publicationCaveatIndex = len(out.Caveats)
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
	return consumeProfilerTracePluginResultContext(ctx, result, true, nil)
}

func consumeProfilerTracePluginResultContext(ctx context.Context, result profilerTracePluginResult, summarize bool,
	visitEvent func(profilerFtraceEventRecord) error,
) (profilerFtraceSummary, bool, error) {
	summary := profilerFtraceSummary{
		StartTotalsValid:  true,
		EndTotalsValid:    true,
		DetailOverwriteOK: true,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceSummary{}, false, err
	}
	if result.Disposition == profilerFtracePayloadNotStructured {
		if err := ctx.Err(); err != nil {
			return profilerFtraceSummary{}, false, err
		}
		return summary, false, nil
	}
	if result.Disposition == profilerFtracePayloadMalformed {
		if visitEvent != nil {
			if err := visitProfilerTracePluginMalformedResult(result, visitEvent); err != nil {
				return profilerFtraceSummary{}, false, err
			}
		}
		if err := ctx.Err(); err != nil {
			return profilerFtraceSummary{}, false, err
		}
		return summary, false, nil
	}
	summary.recognizedMessage = summarize
	err := visitProfilerTracePluginResult(ctx, result, func(field int, raw []byte) error {
		if !summarize && field != 2 {
			return nil
		}
		switch field {
		case 1:
			stats, err := decodeProfilerFtraceCPUStatsContext(ctx, raw)
			if err != nil {
				return summary.observeDecodeIssue(profilerFtraceSummaryIssueCPUStatsMalformed, err)
			}
			if !checkedProfilerUint64AddTo(&summary.StatsMessages, 1) {
				return &traceDBOutputInvariantError{Reason: "profiler_summary_stats_messages_overflow"}
			}
			if stats.Clock != "" {
				if !checkedProfilerUint64AddTo(&summary.TraceClockObserved, 1) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_trace_clock_overflow"}
				}
				if err := summary.TraceClockSamples.observeStringContext(ctx,
					"profiler-ftrace-summary-trace-clock", stats.Clock); err != nil {
					return err
				}
			}
			if stats.Status == 1 {
				if !checkedProfilerUint64AddTo(&summary.EndStats, 1) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_end_stats_overflow"}
				}
			} else {
				if !checkedProfilerUint64AddTo(&summary.StartStats, 1) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_start_stats_overflow"}
				}
			}
			return visitProfilerFtracePerCPUStats(ctx, stats, func(cpu profilerFtracePerCPUStats) error {
				if !summary.StatsCPUs.observe(cpu.CPU) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_stats_cpu_out_of_range"}
				}
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
				return nil
			})
		case 2:
			authority, err := auditProfilerFtraceCPUDetail(ctx, raw)
			if err != nil {
				return err
			}
			var detail profilerFtraceCPUDetail
			var detailSummary *profilerFtraceCPUDetail
			if summarize {
				_, cpuInvalid := authority.cpuIssue()
				if !authority.Malformed && !cpuInvalid {
					candidate, summaryErr := newProfilerFtraceCPUDetailSummary(authority)
					if summaryErr != nil {
						return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_summary_authority_drift"}
					}
					detail = candidate
					detailSummary = &detail
				}
			}
			if detailSummary == nil && visitEvent == nil {
				return nil
			}
			if err := consumeProfilerFtraceCPUDetailAuthorityContext(ctx, authority, detailSummary, visitEvent); err != nil {
				return err
			}
			if detailSummary != nil {
				return summary.observeCPUDetail(*detailSummary)
			}
		case 5:
			symbol, err := decodeProfilerFtraceSymbolDetailContext(ctx, raw)
			if err != nil {
				return summary.observeDecodeIssue(profilerFtraceSummaryIssueSymbolMalformed, err)
			}
			if !checkedProfilerUint64AddTo(&summary.SymbolCount, 1) {
				return &traceDBOutputInvariantError{Reason: "profiler_summary_symbol_count_overflow"}
			}
			if symbol.Name != "" {
				if !checkedProfilerUint64AddTo(&summary.SymbolNamedCount, 1) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_named_symbol_count_overflow"}
				}
				var sampleErr error
				if symbol.Addr != 0 {
					sampleErr = summary.SymbolSamples.observeStringPartsContext(ctx,
						"profiler-ftrace-summary-symbol", "0x", strconv.FormatUint(symbol.Addr, 16), "=", symbol.Name)
				} else {
					sampleErr = summary.SymbolSamples.observeStringContext(ctx,
						"profiler-ftrace-summary-symbol", symbol.Name)
				}
				if sampleErr != nil {
					return sampleErr
				}
				if summary.SymbolNamedCount > profilerDiagnosticSampleLimit {
					summary.SymbolTruncated = true
				}
			}
		case 6:
			clock, err := decodeProfilerFtraceClockDetailContext(ctx, raw)
			if err != nil {
				return summary.observeDecodeIssue(profilerFtraceSummaryIssueClockMalformed, err)
			}
			if label := profilerFtraceClockDetailLabel(clock); label != "" {
				if !checkedProfilerUint64AddTo(&summary.ClockDetailCount, 1) {
					return &traceDBOutputInvariantError{Reason: "profiler_summary_clock_detail_count_overflow"}
				}
				if err := summary.ClockDetailSamples.observeStringContext(ctx,
					"profiler-ftrace-summary-clock-detail", label); err != nil {
					return err
				}
				if summary.ClockDetailCount > profilerDiagnosticSampleLimit {
					summary.ClockTruncated = true
				}
			}
		case 7:
			if result.VersionOccurrences != 1 {
				return nil
			}
			validVersion, err := profilerSinglePhysicalLineBytesContext(ctx, raw, true)
			if err != nil {
				return err
			}
			if validVersion {
				if len(raw) > 0 {
					summary.VersionObservations = 1
					if err := summary.VersionSamples.observeContext(ctx,
						"profiler-ftrace-summary-version", raw); err != nil {
						return err
					}
				}
			} else if !summary.Issues.observe(profilerFtraceSummaryIssueVersionInvalid, 1) {
				summary.IssueOverflow = true
			}
		case 8:
			if err := decodeProfilerFtraceCommDictContext(ctx, raw); err != nil {
				return summary.observeDecodeIssue(profilerFtraceSummaryIssueCommMalformed, err)
			}
		}
		return nil
	})
	if err != nil {
		return profilerFtraceSummary{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceSummary{}, false, err
	}
	return summary, summary.recognizedMessage, nil
}

func (summary *profilerFtraceSummary) observeDecodeIssue(kind profilerFtraceSummaryIssueKind, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, invariant := traceDBOutputInvariantReason(err); invariant {
		return err
	}
	if summary == nil || err == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_summary_decode_issue_invalid"}
	}
	if !summary.Issues.observe(kind, 1) {
		summary.IssueOverflow = true
	}
	return nil
}

func (summary *profilerFtraceSummary) observeCPUDetail(detail profilerFtraceCPUDetail) error {
	if summary == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_summary_detail_target_nil"}
	}
	if !checkedProfilerUint64AddTo(&summary.DetailMessages, 1) ||
		!checkedProfilerUint64AddTo(&summary.DetailEventCount, detail.EventCount) ||
		!summary.DetailCPUs.observe(detail.CPU) {
		return &traceDBOutputInvariantError{Reason: "profiler_summary_detail_counter_invalid"}
	}
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
	for index, count := range detail.KnownEventCounts {
		if !checkedProfilerUint64AddTo(&summary.KnownEventCounts[index], count) {
			return &traceDBOutputInvariantError{Reason: "profiler_summary_known_event_count_overflow"}
		}
	}
	if !checkedProfilerUint64AddTo(&summary.UnknownEventCount, detail.UnknownEventCount) {
		return &traceDBOutputInvariantError{Reason: "profiler_summary_unknown_event_count_overflow"}
	}
	mergeProfilerStableSampleSet(&summary.UnknownEventFieldSamples, detail.UnknownEventFieldSamples)
	return nil
}

func decodeProfilerFtraceCPUStats(data []byte) (profilerFtraceCPUStats, error) {
	return decodeProfilerFtraceCPUStatsContext(context.Background(), data)
}

func decodeProfilerFtraceCPUStatsContext(ctx context.Context, data []byte) (profilerFtraceCPUStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var stats profilerFtraceCPUStats
	var clockRaw []byte
	statusCount, clockCount := 0, 0
	statusWrongWire, clockWrongWire, perCPUWrongWire := false, false, false
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
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
			if stats.PerCPUOccurrences == math.MaxUint64 {
				return &traceDBOutputInvariantError{Reason: "profiler_cpu_stats_per_cpu_census_overflow"}
			}
			stats.PerCPUOccurrences++
			if _, err := decodeProfilerFtracePerCPUStatsContext(ctx, raw); err != nil {
				return err
			}
		case 3:
			clockCount++
			if wire == 2 {
				clockRaw = raw
			} else {
				clockWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return profilerFtraceCPUStats{}, err
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceCPUStats{}, err
	}
	if statusCount > 1 || statusWrongWire || stats.Status > 1 {
		return profilerFtraceCPUStats{}, fmt.Errorf("invalid FtraceCpuStatsMsg status field")
	}
	if perCPUWrongWire {
		return profilerFtraceCPUStats{}, fmt.Errorf("wrong-wire FtraceCpuStatsMsg per_cpu_stats field")
	}
	if clockCount > 1 || clockWrongWire {
		return profilerFtraceCPUStats{}, fmt.Errorf("invalid FtraceCpuStatsMsg trace_clock field")
	}
	if clockCount == 1 {
		validClock, validateErr := profilerSingleTokenBytesContext(ctx, clockRaw)
		if validateErr != nil {
			return profilerFtraceCPUStats{}, validateErr
		}
		if !validClock {
			return profilerFtraceCPUStats{}, fmt.Errorf("invalid FtraceCpuStatsMsg trace_clock field")
		}
		clock, cloneErr := profilerCloneBytesStringContext(ctx, clockRaw)
		if cloneErr != nil {
			return profilerFtraceCPUStats{}, cloneErr
		}
		stats.Clock = clock
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceCPUStats{}, err
	}
	stats.payload = data
	return stats, nil
}

func decodeProfilerFtracePerCPUStats(data []byte) (profilerFtracePerCPUStats, error) {
	return decodeProfilerFtracePerCPUStatsContext(context.Background(), data)
}

func decodeProfilerFtracePerCPUStatsContext(ctx context.Context, data []byte) (profilerFtracePerCPUStats, error) {
	var stats profilerFtracePerCPUStats
	var counts [10]int
	var wrongWire [10]bool
	var rawValues [10]uint64
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
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

func visitProfilerFtracePerCPUStats(ctx context.Context, stats profilerFtraceCPUStats, visit func(profilerFtracePerCPUStats) error) error {
	var observed uint64
	var callbackErr error
	err := walkProfilerProtoFieldsContext(ctx, stats.payload, func(field int, wire int, raw []byte, _ uint64) error {
		if field != 2 || wire != 2 {
			return nil
		}
		if observed == math.MaxUint64 {
			return &traceDBOutputInvariantError{Reason: "profiler_cpu_stats_visitor_census_overflow"}
		}
		observed++
		perCPU, decodeErr := decodeProfilerFtracePerCPUStatsContext(ctx, raw)
		if decodeErr != nil {
			if errors.Is(decodeErr, context.Canceled) || errors.Is(decodeErr, context.DeadlineExceeded) {
				return decodeErr
			}
			return &traceDBOutputInvariantError{Reason: "profiler_cpu_stats_visitor_parse_drift"}
		}
		if visit != nil {
			callbackErr = visit(perCPU)
			return callbackErr
		}
		return nil
	})
	if err != nil {
		if callbackErr != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return err
		}
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_stats_visitor_parse_drift"}
	}
	if observed != stats.PerCPUOccurrences {
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_stats_visitor_census_drift"}
	}
	return nil
}

func decodeProfilerFtraceCPUDetail(data []byte) (profilerFtraceCPUDetail, error) {
	return decodeProfilerFtraceCPUDetailContext(context.Background(), data)
}

func decodeProfilerFtraceCPUDetailContext(ctx context.Context, data []byte) (profilerFtraceCPUDetail, error) {
	authority, err := auditProfilerFtraceCPUDetail(ctx, data)
	if err != nil {
		return profilerFtraceCPUDetail{}, err
	}
	detail, err := newProfilerFtraceCPUDetailSummary(authority)
	if err != nil {
		return detail, err
	}
	err = consumeProfilerFtraceCPUDetailAuthorityContext(ctx, authority, &detail, nil)
	return detail, err
}

func newProfilerFtraceCPUDetailSummary(authority profilerFtraceCPUDetailAuthority) (profilerFtraceCPUDetail, error) {
	detail := profilerFtraceCPUDetail{OverwriteValid: true}
	if authority.Malformed {
		return detail, fmt.Errorf("malformed FtraceCpuDetailMsg wire")
	}
	if authority.CPUOccurrences > 1 {
		return detail, fmt.Errorf("duplicate FtraceCpuDetailMsg cpu field")
	}
	if authority.CPUWrongWire {
		return detail, fmt.Errorf("wrong-wire FtraceCpuDetailMsg cpu field")
	}
	if authority.CPU > uint64(maxTraceDBCPUIndex) {
		return detail, fmt.Errorf("out-of-range FtraceCpuDetailMsg cpu field")
	}
	detail.CPU = authority.CPU
	detail.Overwrite = authority.Overwrite
	if authority.OverwriteOccurrences > 1 || authority.OverwriteWrongWire {
		detail.Overwrite = 0
		detail.OverwriteValid = false
	}
	return detail, nil
}

func (detail *profilerFtraceCPUDetail) observeSummaryEvent(record profilerFtraceEventRecord) error {
	if detail == nil || record.Field == profilerFtraceCPUDetailEnvelopeField {
		return nil
	}
	issues, err := record.checkedEnvelopeIssues()
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return nil
	}
	if !checkedProfilerUint64AddTo(&detail.EventCount, 1) {
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_summary_event_count_overflow"}
	}
	if slot, ok := profilerFtraceEventDescriptorSlot(record.Field); ok {
		if !checkedProfilerUint64AddTo(&detail.KnownEventCounts[slot], 1) {
			return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_summary_field_count_overflow"}
		}
		return nil
	}
	if !checkedProfilerUint64AddTo(&detail.UnknownEventCount, 1) {
		return &traceDBOutputInvariantError{Reason: "profiler_cpu_detail_summary_unknown_count_overflow"}
	}
	detail.UnknownEventFieldSamples.observe("profiler-ftrace-summary-unknown-event-field", []byte(strconv.Itoa(record.Field)))
	return nil
}

func consumeProfilerFtraceCPUDetailAuthorityContext(ctx context.Context, authority profilerFtraceCPUDetailAuthority,
	detail *profilerFtraceCPUDetail, visit func(profilerFtraceEventRecord) error,
) error {
	if detail == nil && visit == nil {
		return nil
	}
	return visitProfilerFtraceCPUDetailEvents(ctx, authority, func(record profilerFtraceEventRecord) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if detail != nil {
			if err := detail.observeSummaryEvent(record); err != nil {
				return err
			}
		}
		if visit != nil {
			return visit(record)
		}
		return nil
	})
}

func decodeProfilerFtraceSymbolDetail(data []byte) (profilerFtraceSymbolDetail, error) {
	return decodeProfilerFtraceSymbolDetailContext(context.Background(), data)
}

func decodeProfilerFtraceSymbolDetailContext(ctx context.Context, data []byte) (profilerFtraceSymbolDetail, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var symbol profilerFtraceSymbolDetail
	var nameRaw []byte
	addrCount, nameCount := 0, 0
	addrWrongWire, nameWrongWire := false, false
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
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
				nameRaw = raw
			} else {
				nameWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return profilerFtraceSymbolDetail{}, err
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceSymbolDetail{}, err
	}
	if addrCount > 1 || addrWrongWire || nameCount > 1 || nameWrongWire {
		return profilerFtraceSymbolDetail{}, fmt.Errorf("invalid SymbolsDetailMsg field")
	}
	if nameCount == 1 {
		validName, validateErr := profilerSinglePhysicalLineBytesContext(ctx, nameRaw, true)
		if validateErr != nil {
			return profilerFtraceSymbolDetail{}, validateErr
		}
		if !validName {
			return profilerFtraceSymbolDetail{}, fmt.Errorf("invalid SymbolsDetailMsg field")
		}
		name, cloneErr := profilerCloneBytesStringContext(ctx, nameRaw)
		if cloneErr != nil {
			return profilerFtraceSymbolDetail{}, cloneErr
		}
		symbol.Name = name
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceSymbolDetail{}, err
	}
	return symbol, nil
}

func decodeProfilerFtraceClockDetail(data []byte) (profilerFtraceClockDetail, error) {
	return decodeProfilerFtraceClockDetailContext(context.Background(), data)
}

func decodeProfilerFtraceClockDetailContext(ctx context.Context, data []byte) (profilerFtraceClockDetail, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var clock profilerFtraceClockDetail
	idCount, timeCount, resCount := 0, 0, 0
	idWrongWire, timeWrongWire, resWrongWire := false, false, false
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
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
				sec, nsec, err := decodeProfilerFtraceTimeSpecContext(ctx, raw)
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
				sec, nsec, err := decodeProfilerFtraceTimeSpecContext(ctx, raw)
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
		return profilerFtraceClockDetail{}, err
	}
	if err := ctx.Err(); err != nil {
		return profilerFtraceClockDetail{}, err
	}
	if idCount > 1 || idWrongWire || clock.ID > 6 || timeCount > 1 || timeWrongWire || resCount > 1 || resWrongWire {
		return profilerFtraceClockDetail{}, fmt.Errorf("invalid ClockDetailMsg field")
	}
	return clock, nil
}

func decodeProfilerFtraceTimeSpec(data []byte) (uint64, uint64, error) {
	return decodeProfilerFtraceTimeSpecContext(context.Background(), data)
}

func decodeProfilerFtraceTimeSpecContext(ctx context.Context, data []byte) (uint64, uint64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var sec, nsec uint64
	secCount, nsecCount := 0, 0
	secWrongWire, nsecWrongWire := false, false
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, v uint64) error {
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
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if secCount > 1 || secWrongWire || sec > uint64(^uint32(0)) ||
		nsecCount > 1 || nsecWrongWire || nsec > uint64(^uint32(0)) || nsec >= 1e9 {
		return 0, 0, fmt.Errorf("invalid ClockDetailMsg TimeSpec field")
	}
	return sec, nsec, nil
}

func decodeProfilerFtraceCommDict(data []byte) error {
	return decodeProfilerFtraceCommDictContext(context.Background(), data)
}

func decodeProfilerFtraceCommDictContext(ctx context.Context, data []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tidCount, commCount := 0, 0
	tidWrongWire, commWrongWire := false, false
	var tid uint64
	var commRaw []byte
	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, value uint64) error {
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
				commRaw = raw
			} else {
				commWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tidCount > 1 || tidWrongWire || tidCount == 1 && uint64(int64(int32(tid))) != tid ||
		commCount > 1 || commWrongWire {
		return fmt.Errorf("invalid CommDictMsg field")
	}
	if commCount == 1 {
		validComm, validateErr := profilerSinglePhysicalLineBytesContext(ctx, commRaw, true)
		if validateErr != nil {
			return validateErr
		}
		if !validComm {
			return fmt.Errorf("invalid CommDictMsg field")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
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
	var ledger profilerFtraceSummaryDiagnosticLedger
	if !ledger.observe(&summary, 0) {
		return "ftrace-plugin structured metadata: unavailable"
	}
	caveat, ok := ledger.caveat()
	if !ok {
		return "ftrace-plugin structured metadata: unavailable"
	}
	return caveat
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

func validateProfilerSessionRowCounterState(coverage *TraceDBCoverage,
	out *profilerContainerExtraction, seq int,
) error {
	if coverage == nil || out == nil || seq < 0 || coverage.RowsRead < 0 ||
		coverage.RowsEmitted != seq || out.TextRows != seq || coverage.RowsRead < coverage.RowsEmitted {
		return &traceDBOutputInvariantError{Reason: "profiler_session_row_counter_state_invalid"}
	}
	return nil
}

func nextProfilerSessionRowsRead(rowsRead int) (int, error) {
	next := rowsRead
	if !checkedProfilerIntAddTo(&next, 1) {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_session_rows_read_overflow"}
	}
	return next, nil
}

func commitProfilerSessionRowCounters(coverage *TraceDBCoverage,
	out *profilerContainerExtraction, oldSeq, nextSeq, lineRows, nextRowsRead int,
) error {
	if err := validateProfilerSessionRowCounterState(coverage, out, oldSeq); err != nil {
		return err
	}
	wantRowsRead, err := nextProfilerSessionRowsRead(coverage.RowsRead)
	if err != nil || wantRowsRead != nextRowsRead || lineRows < 0 || lineRows > 1 {
		return &traceDBOutputInvariantError{Reason: "profiler_session_row_counter_commit_invalid"}
	}
	wantSeq := oldSeq
	if !checkedProfilerIntAddTo(&wantSeq, lineRows) || wantSeq != nextSeq || nextRowsRead < nextSeq {
		return &traceDBOutputInvariantError{Reason: "profiler_session_row_counter_commit_invalid"}
	}
	coverage.RowsRead = nextRowsRead
	coverage.RowsEmitted = nextSeq
	out.TextRows = nextSeq
	return nil
}

func extractProfilerSessionPackageFromInput(ctx context.Context, binding *profilerInputBinding,
	inputSize int64, sink *traceDBRowSink, maxLineBytes int,
) (extracted profilerContainerExtraction, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var input conversionInputView
	if binding != nil {
		input = binding.input
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageProfilerBody, nil); err != nil {
		return profilerContainerExtraction{}, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageProfilerBody, err)
		if err != nil {
			extracted = profilerContainerExtraction{}
			var inputErr *ConversionInputError
			if sink != nil && errors.As(err, &inputErr) {
				sink.failCloseAllRows()
			}
		}
	}()
	if err := validateProfilerInputBinding(binding, conversionInputStageProfilerBody); err != nil {
		return profilerContainerExtraction{}, err
	}
	if inputSize < 0 || inputSize > binding.inputSize {
		return profilerContainerExtraction{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageProfilerBody,
			input.DisplayPath(),
			fmt.Errorf("invalid profiler session input boundary %d for fixed size %d", inputSize, binding.inputSize),
		)
	}
	return extractProfilerSessionPackageAt(ctx, input, inputSize, sink, maxLineBytes)
}

func extractProfilerSessionPackageAt(ctx context.Context, input io.ReaderAt, inputSize int64,
	sink *traceDBRowSink, maxLineBytes int,
) (profilerContainerExtraction, error) {
	if input == nil || inputSize < 0 || maxLineBytes <= 0 || maxLineBytes >= math.MaxInt {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "invalid_profiler_session_line_limit"}
	}
	if _, ok, err := profilerSessionJSONMarkerOffsetAt(input, inputSize, 64*1024); err != nil {
		return profilerContainerExtraction{}, err
	} else if !ok {
		return profilerContainerExtraction{}, nil
	}
	if err := sink.enableProfilerTraceClassification(); err != nil {
		return profilerContainerExtraction{}, err
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
	reader := bufio.NewReaderSize(io.NewSectionReader(input, 0, inputSize), profilerSessionReaderBufBytes)
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherSession) {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_pair_census_nested"}
	}
	oversizedLines := 0
	_, scanErr := scanProfilerBoundedSessionRecords(ctx, reader, maxLineBytes,
		func(record profilerBoundedPhysicalLine) (bool, error) {
			if err := validateProfilerSessionRowCounterState(&coverage, &out, seq); err != nil {
				return false, err
			}
			nextRowsRead, err := nextProfilerSessionRowsRead(coverage.RowsRead)
			if err != nil {
				return false, err
			}
			if record.Oversized {
				if oversizedLines != 0 {
					return false, &traceDBOutputInvariantError{Reason: "profiler_session_oversized_record_count_invalid"}
				}
				coverage.RowsRead = nextRowsRead
				oversizedLines = 1
				out.SourceFailClosed = true
				out.SourceFailReason = "session_line_size_budget_exceeded"
				sink.failCloseAllRows()
				return false, nil
			}
			oldSeq := seq
			lineRows, rowErr := addSystraceRowsFromBytesContext(ctx, record.Bytes, &seq, sink)
			if rowErr != nil {
				return false, rowErr
			}
			if err := commitProfilerSessionRowCounters(&coverage, &out, oldSeq, seq, lineRows, nextRowsRead); err != nil {
				return false, err
			}
			return true, nil
		})
	if scanErr != nil {
		sink.abortPairRowCensus()
		coverage.Error = scanErr.Error()
		out.TraceCoverage = append(out.TraceCoverage, coverage)
		return profilerContainerExtraction{}, scanErr
	}
	if oversizedLines > 0 {
		if out.RejectedMessages != 0 || oversizedLines != 1 {
			sink.abortPairRowCensus()
			return profilerContainerExtraction{}, &traceDBOutputInvariantError{Reason: "profiler_session_oversized_record_count_invalid"}
		}
		out.RejectedMessages = oversizedLines
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
	out.publicationCaveatPending = true
	out.publicationCaveatIndex = len(out.Caveats)
	coverageIndex := len(out.TraceCoverage)
	if !out.profilerPublisherCoverage.observe(profilerPairPublisherSession, coverageIndex) {
		return profilerContainerExtraction{}, &traceDBOutputInvariantError{
			Reason: "profiler_session_coverage_index_mismatch",
		}
	}
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
	if _, err := io.ReadFull(io.NewSectionReader(reader, 0, maxProbe), probe); err != nil {
		return 0, false, err
	}
	idx := bytes.Index(probe, []byte(profilerSessionJSONTag))
	if idx < 0 {
		return 0, false, nil
	}
	return int64(idx), true, nil
}

func readProfilerTraceHeaderAt(r io.ReaderAt, off int64, fileSize int64) (profilerTraceHeader, bool) {
	header, ok, _ := readProfilerTraceHeaderAtExact(r, off, fileSize)
	return header, ok
}

func readProfilerTraceHeaderAtExact(r io.ReaderAt, off int64, fileSize int64) (profilerTraceHeader, bool, error) {
	if r == nil {
		return profilerTraceHeader{}, false, &traceDBOutputInvariantError{Reason: "invalid_profiler_trace_header_reader"}
	}
	if off < 0 || fileSize < 0 || off > fileSize || int64(profilerTraceHeaderSize) > fileSize-off {
		return profilerTraceHeader{}, false, nil
	}
	header := make([]byte, profilerTraceHeaderSize)
	_, err := io.ReadFull(io.NewSectionReader(r, off, profilerTraceHeaderSize), header)
	if err != nil {
		return profilerTraceHeader{}, false, err
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return profilerTraceHeader{}, false, nil
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
	}, true, nil
}

func parseProfilerPluginData(data []byte) profilerPluginDataDecode {
	decoded, _ := parseProfilerPluginDataContext(context.Background(), data)
	return decoded
}

func parseProfilerPluginDataContext(ctx context.Context, data []byte) (profilerPluginDataDecode, error) {
	var decoded profilerPluginDataDecode
	if ctx == nil {
		ctx = context.Background()
	}
	var counts [9]int
	var valid [9]bool
	var uintValues [9]uint64
	var byteValues [9][]byte
	observeIssue := func(kind profilerPluginIssueKind, delta uint64) {
		if !decoded.IssueCensus.observe(kind, delta) {
			decoded.IssueOverflow = true
		}
	}

	err := walkProfilerProtoFieldsContext(ctx, data, func(field int, wire int, raw []byte, value uint64) error {
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return profilerPluginDataDecode{}, err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return profilerPluginDataDecode{}, err
		}
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
	} else if validName, validateErr := profilerSingleTokenBytesContext(ctx, byteValues[1]); validateErr != nil {
		return profilerPluginDataDecode{}, validateErr
	} else if !validName {
		hardRejected = true
		valid[1] = false
		observeIssue(profilerPluginIssueNameInvalid, 1)
	} else {
		name, cloneErr := profilerCloneBytesStringContext(ctx, byteValues[1])
		if cloneErr != nil {
			return profilerPluginDataDecode{}, cloneErr
		}
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
		versionValid, validateErr := profilerSinglePhysicalLineBytesContext(ctx, byteValues[7], true)
		if validateErr != nil {
			return profilerPluginDataDecode{}, validateErr
		}
		if !versionValid {
			valid[7] = false
			observeIssue(profilerPluginIssueVersionInvalid, 1)
		} else {
			version, cloneErr := profilerCloneBytesStringContext(ctx, byteValues[7])
			if cloneErr != nil {
				return profilerPluginDataDecode{}, cloneErr
			}
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
	if err := ctx.Err(); err != nil {
		return profilerPluginDataDecode{}, err
	}
	if hardRejected || decoded.IssueOverflow {
		decoded.Plugin = profilerPluginData{}
		return decoded, nil
	}
	decoded.Accepted = true
	return decoded, nil
}

// profilerPayloadContainsExactBlockEndpoint is a provenance probe, not a text
// renderer. The first non-empty, non-comment physical fragment elects the
// origin: a source-neutral physical ftrace header elects text even when its
// task bytes also form protobuf keys; any other fragment permanently denies
// later embedded headers. Exact endpoint identity is proven separately. When
// text origin is absent, only typed PairFamilies can prove structured Block.
func profilerPayloadContainsExactBlockEndpoint(data []byte) bool {
	found, _ := profilerPayloadContainsExactBlockEndpointContext(context.Background(), data)
	return found
}

func profilerPayloadContainsExactBlockEndpointContext(ctx context.Context, data []byte) (bool, error) {
	scan, err := scanProfilerStrictSystracePayloadContext(ctx, data, nil)
	if err != nil {
		return false, err
	}
	if scan.originText {
		// Complete strict text wins even when its leading task bytes also form a
		// syntactically complete TracePluginResult. A rejected text payload still
		// retains exact Block provenance when its first physical row established
		// the origin (malformed scalar or a later bad fragment).
		return scan.observed[pairRenderBlock], nil
	}
	// Without a first-row text origin, later header-looking bytes are metadata.
	// Only the typed structured family authority may then close Block.
	authority, err := decodeProfilerTracePluginResultContext(ctx, data)
	if err != nil {
		return false, err
	}
	return authority.PairFamilies&pairCriticalFormatFamilyBlock != 0, nil
}

// profilerRejectedPluginFrameContainsExactBlockEndpoint recovers only precise
// provenance from a rejected ProfilerPluginData frame. A payload is eligible
// solely when the bounded outer protobuf walk reached a complete field-3 bytes
// value; malformed bytes before that field, wrong-wire fields and metadata are
// never searched as text. A later outer decode failure does not erase an exact
// Block endpoint already proven inside a complete data field.
func profilerRejectedPluginFrameContainsExactBlockEndpoint(frame []byte) bool {
	found, _ := profilerRejectedPluginFrameContainsExactBlockEndpointContext(context.Background(), frame)
	return found
}

func profilerRejectedPluginFrameContainsExactBlockEndpointContext(ctx context.Context, frame []byte) (bool, error) {
	found := false
	walkErr := walkProfilerProtoFieldsContext(ctx, frame, func(field int, wire int, raw []byte, _ uint64) error {
		if !found && field == 3 && wire == 2 {
			block, probeErr := profilerPayloadContainsExactBlockEndpointContext(ctx, raw)
			if probeErr != nil {
				return probeErr
			}
			found = block
		}
		return nil
	})
	if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
		return false, walkErr
	}
	if _, invariant := traceDBOutputInvariantReason(walkErr); invariant {
		return false, walkErr
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Malformed outer protobuf retains the legacy "best complete field-3
	// prefix" provenance contract; only request/invariant errors abort it.
	return found, nil
}

func addSystraceRowsFromBytes(data []byte, seq *int, sink *traceDBRowSink) (int, error) {
	return addSystraceRowsFromBytesContext(context.Background(), data, seq, sink)
}

func forEachProfilerSystraceRecordContext(ctx context.Context, data []byte, visit func([]byte) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if visit == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_systrace_record_visitor_missing"}
	}
	processed := uint64(0)
	recordStart := 0
	for chunkStart := 0; chunkStart < len(data); {
		chunkEnd := min(chunkStart+profilerContextByteCheckpointBytes, len(data))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(chunkEnd-chunkStart)); err != nil {
			return err
		}
		for offset := chunkStart; offset < chunkEnd; offset++ {
			if data[offset] != '\n' && data[offset] != 0 {
				continue
			}
			if err := visit(data[recordStart:offset]); err != nil {
				return err
			}
			recordStart = offset + 1
		}
		chunkStart = chunkEnd
	}
	if recordStart < len(data) {
		if err := visit(data[recordStart:]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func addSystraceRowsFromBytesContext(ctx context.Context, data []byte, seq *int, sink *traceDBRowSink) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	if sink == nil {
		return 0, &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
	}
	if seq == nil {
		return 0, &traceDBOutputInvariantError{Reason: "profiler_row_sequence_missing"}
	}
	rows := 0
	err := forEachProfilerSystraceRecordContext(ctx, data, func(raw []byte) error {
		part, trimErr := profilerTrimSpaceBytesContext(ctx, raw)
		if trimErr != nil {
			return trimErr
		}
		if len(part) == 0 {
			return nil
		}
		// Session/bytrace records use the same leading-# comment namespace as
		// strict ftrace compatibility payloads. Apply the negative gate before
		// size census and endpoint admission so a comment-looking Block header,
		// including an oversized one, cannot poison or publish a pairing lane.
		if part[0] == '#' {
			return nil
		}
		if len(part) > maxProfilerTextLineBytes {
			var delta traceDBProfilerEventDelta
			deltaPresent := false
			if kind, governed, opaque := profilerTextPairCensus(part); governed {
				delta.poisonKind(kind)
				deltaPresent = true
			} else if opaque {
				delta.markOpaque(pairRenderMMC)
				delta.markOpaque(pairRenderF2FS)
				deltaPresent = true
			}
			if !deltaPresent {
				return ctx.Err()
			}
			return sink.commitProfilerEventDeltaContext(ctx, delta)
		}
		line := string(part)
		if line == "" {
			return nil
		}
		if profilerTextPairNormalizationCollision(line) {
			return ctx.Err()
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
					pair.EndpointSlot, _ = profilerPairEndpointForName(probe.Name)
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
				var delta traceDBProfilerEventDelta
				delta.poisonAdmission(pair)
				return sink.commitProfilerEventDeltaContext(ctx, delta)
			}
			return ctx.Err()
		}
		if !headerKnown {
			if pair.Governed {
				var delta traceDBProfilerEventDelta
				delta.poisonAdmission(pair)
				return sink.commitProfilerEventDeltaContext(ctx, delta)
			}
			return ctx.Err()
		}
		row := renderedRow{tsNS: ts, line: line}
		var delta traceDBProfilerEventDelta
		if pair.Governed {
			if headerGoverned && pair.Kind == headerKind {
				row.pairKind = pair.Kind
				row.pairLane = pair.Lane
				row.profilerEndpointSlot = pair.EndpointSlot
				if !pair.Admitted {
					delta.poisonAdmission(pair)
				}
			} else {
				// A complete outer header is the physical row authority. A raw
				// endpoint probe that disagrees with it must never relabel an
				// unrelated print/vendor row as a nested pair. Quarantine only the
				// proven raw lane/family and publish the outer row without pair
				// metadata.
				delta.poisonAdmission(pair)
			}
		}
		if err := sink.addSequencedProfilerEventContext(ctx, seq, row, delta); err != nil {
			return err
		}
		rows++
		return nil
	})
	if err != nil {
		return rows, err
	}
	if rows > 0 {
		if err := sink.flushTriggeredProfilerEventContext(ctx); err != nil {
			return rows, err
		}
	}
	if err := ctx.Err(); err != nil {
		return rows, err
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
