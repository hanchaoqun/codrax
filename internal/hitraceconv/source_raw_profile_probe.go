package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxTraceDBRawProbeEventFormatBytes    = 16 << 20
	maxTraceDBRawProbeEventFormatSegments = 128
	maxTraceDBRawProbePages               = 131072
	maxTraceDBRawProbeTargetWitnesses     = 32
	maxTraceDBRawProbeUnknownWitnesses    = 4
	maxTraceDBRawProbeUnknownWitnessBytes = 4096
	maxTraceDBRawProbeTextWitnessBytes    = 256
)

func newTraceDBSourceRawProfileCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "source_rawtrace_profile",
		Table:  "__raw_page_probe__",
		Role:   "diagnostic_probe",
		FieldSources: map[string]string{
			"authority":       "same immutable input generation as trace_streamer; official 0xdf49/version=1/file_type=0|1 only",
			"effect":          "bounded non-publishing structural probe; never emits an event or becomes timestamp, CPU, identity, lifecycle, namespace, name, or causal authority",
			"event_format":    "existing strict tracefs descriptor parser; target witnesses contain only exact admitted name/id/field geometry, never print-fmt payloads",
			"page_layout":     "candidate-only validation against the legacy qword timestamp/qword logical length/byte CPU plus uint32 timestamp-delta/uint16 size record geometry; matching does not authorize decoding",
			"unknown_segment": "bounded SHA-256 plus escaped UTF-8 text witness for otherwise-unclassified small segments; diagnostic classification only",
		},
		Metadata: map[string]string{
			"probe_state":       "unavailable",
			"page_layout_state": "not_evaluated",
			"decoder_readiness": "not_evaluated",
		},
	}
}

func probeTraceDBSourceRawProfile(
	ctx context.Context,
	input conversionInputView,
	header fileHeader,
	segments []segmentMeta,
	inventoryIncomplete string,
) (TraceDBCoverage, TraceDBCoverage, error) {
	coverage := newTraceDBSourceRawProfileCoverage()
	decode := newTraceDBSourceRawDecodeAccumulator()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return coverage, decode.coverage, err
	}
	if header.Magic != traceStreamerRawTraceMagic || header.Version != harmonyRMQVersion ||
		(header.FileType != 0 && header.FileType != harmonyRMQFileType) {
		coverage.Metadata["probe_state"] = "not_applicable_non_official_profile"
		coverage.Metadata["decoder_readiness"] = "not_applicable"
		coverage.Skipped = "official raw page probe not applicable to this envelope"
		decode.setUnavailable("not_applicable_non_official_profile",
			"official raw record decode ledger not applicable to this envelope", false)
		return coverage, decode.coverage, nil
	}
	coverage.Found = true
	decode.coverage.Found = true
	if inventoryIncomplete != "" {
		coverage.Metadata["probe_state"] = "withheld_segment_inventory_incomplete"
		coverage.Metadata["decoder_readiness"] = "unavailable_segment_inventory_incomplete"
		coverage.Skipped = "raw page probe withheld: segment_inventory_incomplete"
		decode.setUnavailable("withheld_segment_inventory_incomplete",
			"raw record decode ledger withheld: segment_inventory_incomplete", true)
		return coverage, decode.coverage, nil
	}

	catalog, formatState := probeTraceDBSourceEventFormats(ctx, input, segments, &coverage)
	if err := ctx.Err(); err != nil {
		return coverage, decode.coverage, err
	}
	coverage.Metadata["event_format_probe_state"] = formatState
	probeTraceDBSourceUnknownSegments(ctx, input, header, segments, &coverage)
	if err := ctx.Err(); err != nil {
		return coverage, decode.coverage, err
	}
	probeTraceDBSourceRawPages(ctx, input, header, segments, catalog, &coverage, &decode)
	if err := ctx.Err(); err != nil {
		return coverage, decode.coverage, err
	}
	return coverage, decode.finalize(catalog, coverage), nil
}

func probeTraceDBSourceEventFormats(
	ctx context.Context,
	input conversionInputView,
	segments []segmentMeta,
	coverage *TraceDBCoverage,
) (eventFormatCatalog, string) {
	catalog := eventFormatCatalog{
		Formats:          map[int]eventFormat{},
		Poisoned:         map[int]bool{},
		PoisonedFamilies: map[int]pairCriticalFormatFamilyMask{},
	}
	segmentCount := 0
	totalBytes := int64(0)
	for _, segment := range segments {
		if segment.Type != segmentEventsFormat {
			continue
		}
		if err := ctx.Err(); err != nil {
			return catalog, "cancelled"
		}
		segmentCount++
		totalBytes += int64(segment.Size)
		if segmentCount > maxTraceDBRawProbeEventFormatSegments ||
			totalBytes > maxTraceDBRawProbeEventFormatBytes {
			traceDBAddCoverageMetric(coverage, "event_format_probe_budget_exceeded", 1)
			return eventFormatCatalog{
				Formats:          map[int]eventFormat{},
				Poisoned:         map[int]bool{},
				PoisonedFamilies: map[int]pairCriticalFormatFamilyMask{},
			}, "withheld_budget_exceeded"
		}
		data := make([]byte, int(segment.Size))
		if _, err := io.ReadFull(io.NewSectionReader(input, segment.Offset, int64(segment.Size)), data); err != nil {
			traceDBAddCoverageMetric(coverage, "event_format_probe_read_failed", 1)
			return eventFormatCatalog{
				Formats:          map[int]eventFormat{},
				Poisoned:         map[int]bool{},
				PoisonedFamilies: map[int]pairCriticalFormatFamilyMask{},
			}, "unavailable_read_failed"
		}
		parsed, err := parseEventFormats(data)
		if err != nil {
			traceDBAddCoverageMetric(coverage, "event_format_probe_parse_failed", 1)
			return eventFormatCatalog{
				Formats:          map[int]eventFormat{},
				Poisoned:         map[int]bool{},
				PoisonedFamilies: map[int]pairCriticalFormatFamilyMask{},
			}, "withheld_strict_parse_failed"
		}
		mergeEventFormatCatalog(&catalog, parsed)
	}
	traceDBAddCoverageMetric(coverage, "event_format_segments_probed", int64(segmentCount))
	traceDBAddCoverageMetric(coverage, "event_format_bytes_probed", totalBytes)
	traceDBAddCoverageMetric(coverage, "event_formats_admitted", int64(len(catalog.Formats)))
	traceDBAddCoverageMetric(coverage, "event_format_ids_poisoned", int64(len(catalog.Poisoned)))
	if segmentCount == 0 {
		return catalog, "absent"
	}

	commonTypeExact := 0
	var targets []string
	for _, format := range catalog.Formats {
		commonExact := traceDBRawProbeCommonTypeExact(format)
		if commonExact {
			commonTypeExact++
		}
		if !traceDBRawProbeTargetFormat(format.Name) {
			continue
		}
		targets = append(targets, fmt.Sprintf("%s#%d/fields=%d/common_type_exact=%t",
			format.Name, format.ID, len(format.Fields), commonExact))
	}
	traceDBAddCoverageMetric(coverage, "event_formats_common_type_exact", int64(commonTypeExact))
	if len(targets) > 0 {
		sort.Strings(targets)
		omitted := 0
		if len(targets) > maxTraceDBRawProbeTargetWitnesses {
			omitted = len(targets) - maxTraceDBRawProbeTargetWitnesses
			targets = targets[:maxTraceDBRawProbeTargetWitnesses]
		}
		coverage.Metadata["target_format_witnesses"] = strings.Join(targets, ",")
		traceDBAddCoverageMetric(coverage, "target_format_witnesses_omitted", int64(omitted))
	}
	return catalog, "parsed_strict"
}

func traceDBRawProbeCommonTypeExact(format eventFormat) bool {
	for _, field := range format.Fields {
		if cleanFieldName(field.Name) == "common_type" {
			return field.Offset == 0 && field.Size == 2
		}
	}
	return false
}

func traceDBRawProbeTargetFormat(name string) bool {
	switch name {
	case "sched_switch", "sched_blocked_reason", "trace_vsync", "tracing_mark_write",
		"sched_wakeup", "sched_wakeup_new", "sched_waking":
		return true
	default:
		return strings.HasPrefix(name, "dma_fence")
	}
}

func probeTraceDBSourceRawPages(
	ctx context.Context,
	input conversionInputView,
	header fileHeader,
	segments []segmentMeta,
	catalog eventFormatCatalog,
	coverage *TraceDBCoverage,
	decode *traceDBSourceRawDecodeAccumulator,
) {
	rawSegments := 0
	rawBytes := int64(0)
	pageCount := 0
	candidatePages := 0
	invalidPages := 0
	records := int64(0)
	knownRecords := int64(0)
	unknownRecords := int64(0)
	zeroSentinels := int64(0)
	cpuSeen := map[int]bool{}
	failures := map[string]int64{}
	targetRecords := map[string]int64{}
	probeCapped := false
	page := make([]byte, tracePageSize)

	for _, segment := range segments {
		if !isRawTraceSegment(segment.Type, header.CPUNum) {
			continue
		}
		rawSegments++
		rawBytes += int64(segment.Size)
		if segment.Size%tracePageSize != 0 {
			failures["partial_page_segment"]++
			continue
		}
		reader := io.NewSectionReader(input, segment.Offset, int64(segment.Size))
		for offset := int64(0); offset < int64(segment.Size); offset += tracePageSize {
			if err := ctx.Err(); err != nil {
				return
			}
			if pageCount >= maxTraceDBRawProbePages {
				probeCapped = true
				break
			}
			pageCount++
			if _, err := io.ReadFull(reader, page); err != nil {
				failures["page_read_failed"]++
				invalidPages++
				break
			}
			valid, pageRecords, pageKnown, pageUnknown, pageSentinels, cpu, targetCounts, reason :=
				probeTraceDBSourceRMQCandidatePage(page, header.CPUNum, catalog, decode)
			records += pageRecords
			knownRecords += pageKnown
			unknownRecords += pageUnknown
			zeroSentinels += pageSentinels
			if valid {
				candidatePages++
				cpuSeen[cpu] = true
				for name, count := range targetCounts {
					targetRecords[name] += count
				}
			} else {
				invalidPages++
				failures[reason]++
			}
		}
		if probeCapped {
			break
		}
	}

	traceDBAddCoverageMetric(coverage, "raw_segments_probed", int64(rawSegments))
	traceDBAddCoverageMetric(coverage, "raw_bytes_declared_for_probe", rawBytes)
	traceDBAddCoverageMetric(coverage, "raw_bytes_probed", int64(pageCount*tracePageSize))
	traceDBAddCoverageMetric(coverage, "pages_probed", int64(pageCount))
	traceDBAddCoverageMetric(coverage, "pages_qword_length_cpu_candidate", int64(candidatePages))
	traceDBAddCoverageMetric(coverage, "pages_structurally_invalid", int64(invalidPages))
	traceDBAddCoverageMetric(coverage, "records_structurally_scanned", records)
	traceDBAddCoverageMetric(coverage, "records_matching_admitted_format", knownRecords)
	traceDBAddCoverageMetric(coverage, "records_without_admitted_format", unknownRecords)
	traceDBAddCoverageMetric(coverage, "page_zero_size_sentinels", zeroSentinels)
	for reason, count := range failures {
		traceDBAddCoverageMetric(coverage, "page_probe_failure_"+reason, count)
	}
	for name, count := range targetRecords {
		traceDBAddCoverageMetric(coverage, "candidate_records_"+traceDBRawProbeMetricName(name), count)
	}
	if len(cpuSeen) > 0 {
		cpus := make([]int, 0, len(cpuSeen))
		for cpu := range cpuSeen {
			cpus = append(cpus, cpu)
		}
		sort.Ints(cpus)
		values := make([]string, 0, len(cpus))
		for _, cpu := range cpus {
			values = append(values, strconv.Itoa(cpu))
		}
		coverage.Metadata["candidate_cpu_roster"] = strings.Join(values, ",")
	}
	coverage.RowsRead = pageCount
	coverage.RowsEmitted = 0
	switch {
	case rawSegments == 0:
		coverage.Metadata["probe_state"] = "complete"
		coverage.Metadata["page_layout_state"] = "not_applicable_raw_absent"
		coverage.Metadata["decoder_readiness"] = "not_applicable_raw_absent"
	case probeCapped:
		traceDBAddCoverageMetric(coverage, "page_probe_budget_exceeded", 1)
		coverage.Metadata["probe_state"] = "incomplete_page_cap"
		coverage.Metadata["page_layout_state"] = "withheld_probe_cap"
		coverage.Metadata["decoder_readiness"] = "unavailable_probe_incomplete"
		coverage.Skipped = "raw page probe incomplete: page_cap_exceeded"
	case pageCount > 0 && invalidPages == 0 && candidatePages == pageCount:
		coverage.Metadata["probe_state"] = "complete"
		coverage.Metadata["page_layout_state"] = "qword_length_cpu_candidate_all_pages"
		if coverage.Metadata["event_format_probe_state"] == "parsed_strict" &&
			records > 0 && knownRecords > 0 {
			coverage.Metadata["decoder_readiness"] = "structural_candidate_requires_fixture_parity"
		} else if records == 0 {
			coverage.Metadata["decoder_readiness"] = "unavailable_no_candidate_records"
		} else if knownRecords == 0 {
			coverage.Metadata["decoder_readiness"] = "unavailable_no_record_format_match"
		} else {
			coverage.Metadata["decoder_readiness"] = "unavailable_event_format_probe"
		}
	default:
		coverage.Metadata["probe_state"] = "complete"
		coverage.Metadata["page_layout_state"] = "candidate_rejected"
		coverage.Metadata["decoder_readiness"] = "requires_different_page_layout"
	}
}

func probeTraceDBSourceRMQCandidatePage(
	page []byte,
	cpuCount int,
	catalog eventFormatCatalog,
	decode *traceDBSourceRawDecodeAccumulator,
) (
	valid bool,
	records int64,
	known int64,
	unknown int64,
	sentinels int64,
	cpu int,
	targets map[string]int64,
	reason string,
) {
	targets = map[string]int64{}
	header, ok := parsePageHeader(page)
	if !ok {
		return false, 0, 0, 0, 0, 0, targets, "short_page_header"
	}
	if header.CPU < 0 || header.CPU >= cpuCount {
		return false, 0, 0, 0, 0, header.CPU, targets, "cpu_out_of_range"
	}
	maxBody := len(page) - pageHeaderSize
	if header.Length > uint64(maxBody) {
		return false, 0, 0, 0, 0, header.CPU, targets, "logical_length_out_of_range"
	}
	body := page[pageHeaderSize : pageHeaderSize+int(header.Length)]
	for offset := 0; offset < len(body); {
		if len(body)-offset < eventHeaderSize {
			return false, records, known, unknown, sentinels, header.CPU, targets, "truncated_event_header"
		}
		eventHeader, ok := parseEventHeader(body[offset:])
		if !ok {
			sentinels++
			break
		}
		contentStart := offset + eventHeaderSize
		contentEnd := contentStart + int(eventHeader.Size)
		next := contentStart + eventHeader.AlignedSize
		if contentEnd > len(body) || next > len(body) {
			return false, records, known, unknown, sentinels, header.CPU, targets, "event_bounds"
		}
		content := body[contentStart:contentEnd]
		if len(content) < 2 {
			return false, records, known, unknown, sentinels, header.CPU, targets, "event_id_truncated"
		}
		if header.TimestampNS > ^uint64(0)-uint64(eventHeader.TimestampOffsetNS) {
			return false, records, known, unknown, sentinels, header.CPU, targets, "timestamp_overflow"
		}
		records++
		eventID := int(binary.LittleEndian.Uint16(content[:2]))
		if format, exists := catalog.Formats[eventID]; exists {
			known++
			if decode != nil {
				decode.observeRecord(format, content, header.CPU,
					header.TimestampNS+uint64(eventHeader.TimestampOffsetNS))
			}
			if traceDBRawProbeTargetFormat(format.Name) {
				targets[format.Name]++
			}
		} else {
			unknown++
			if decode != nil {
				decode.observeUnknownRecord()
			}
		}
		offset = next
	}
	return true, records, known, unknown, sentinels, header.CPU, targets, ""
}

func traceDBRawProbeMetricName(name string) string {
	if strings.HasPrefix(name, "dma_fence") {
		return "dma_fence"
	}
	return name
}

func probeTraceDBSourceUnknownSegments(
	ctx context.Context,
	input conversionInputView,
	header fileHeader,
	segments []segmentMeta,
	coverage *TraceDBCoverage,
) {
	var witnesses []string
	omitted := 0
	for _, segment := range segments {
		if segment.Type == segmentEventsFormat || segment.Type == segmentCmdlines ||
			segment.Type == segmentTGIDs || isRawTraceSegment(segment.Type, header.CPUNum) ||
			isIgnoredSegment(segment.Type) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		if len(witnesses) >= maxTraceDBRawProbeUnknownWitnesses ||
			segment.Size > maxTraceDBRawProbeUnknownWitnessBytes {
			omitted++
			continue
		}
		data := make([]byte, int(segment.Size))
		if _, err := io.ReadFull(io.NewSectionReader(input, segment.Offset, int64(segment.Size)), data); err != nil {
			omitted++
			continue
		}
		sum := sha256.Sum256(data)
		witness := fmt.Sprintf("type=%d/bytes=%d/sha256=%x", segment.Type, segment.Size, sum)
		if text, ok := traceDBRawProbeTextWitness(data); ok {
			witness += "/text=" + text
		}
		witnesses = append(witnesses, witness)
	}
	if len(witnesses) > 0 {
		sort.Strings(witnesses)
		coverage.Metadata["unknown_segment_witnesses"] = strings.Join(witnesses, ",")
	}
	traceDBAddCoverageMetric(coverage, "unknown_segment_witnesses_omitted", int64(omitted))
}

func traceDBRawProbeTextWitness(data []byte) (string, bool) {
	if len(data) == 0 || len(data) > maxTraceDBRawProbeTextWitnessBytes || !utf8.Valid(data) {
		return "", false
	}
	text := string(data)
	for _, value := range text {
		if unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t' {
			return "", false
		}
	}
	return strconv.QuoteToASCII(text), true
}
