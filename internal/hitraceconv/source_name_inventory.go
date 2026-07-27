package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	maxTraceDBSourceCmdlineSegmentBytes = 16 << 20
	maxTraceDBSourceCmdlineSegments     = 128
	maxTraceDBSourceCmdlineRows         = 262144
	maxTraceDBSourceSegmentInventory    = 4096
	maxTraceDBSourceUnknownSegmentTypes = 16
)

// traceDBSourceNameInventory binds two non-event source companions to the
// immutable binary input: display-only SEGMENT_CMDLINES names and a
// diagnostic-only common-envelope segment inventory. Neither companion can
// mint an event or become identity, lifecycle, namespace, CPU, or causal
// authority.
type traceDBSourceNameInventory struct {
	Names        map[int64]string
	Coverage     TraceDBCoverage
	RawAuthority TraceDBCoverage
}

func newTraceDBSourceNameInventory() traceDBSourceNameInventory {
	return traceDBSourceNameInventory{
		Names: map[int64]string{},
		Coverage: TraceDBCoverage{
			Family: "resolver.source_metadata",
			Table:  "__source_cmdlines__",
			Role:   "display_name_companion",
			FieldSources: map[string]string{
				"binding": "immutable conversion input; exact V1 CONTENT_TYPE_CMDLINES=2 public TID and comm rows from the legacy 0x0ace or official TraceStreamer 0xdf49 common segment envelope",
				"effect":  "display-only fallback for a uniquely selected canonical thread; never identity, owner, namespace, lifecycle, or CPU authority",
				"scope":   "unique public-TID candidate, or the sole same-TID candidate with positive scheduler switch_count; ambiguity fails closed",
			},
		},
		RawAuthority: TraceDBCoverage{
			Family: "source_rawtrace_authority",
			Table:  "__source_segments__",
			Role:   "diagnostic_inventory",
			FieldSources: map[string]string{
				"binding":         "same immutable logical input generation passed to trace_streamer",
				"segment_header":  "common V1 uint32 type/uint32 payload_size envelope; ranges are checked against the held input size",
				"event_format":    "segment presence and bytes only; this inventory does not claim that a format body parsed successfully",
				"raw_payload":     "segment presence and bytes only; rows_read/rows_emitted count segment records, never raw events or pages",
				"decoder_effect":  "diagnostic only; never synthesizes events or maps trace_streamer aggregate not_match counts back to source records",
				"bounded_witness": "unknown segment type roster is capped; omitted types are counted",
			},
			Metadata: map[string]string{
				"inventory_state":    "unavailable",
				"event_format_state": "unknown",
				"raw_payload_state":  "unknown",
				"decode_authority":   "unavailable_envelope_unclassified",
				"recovery_authority": "aggregate_counts_cannot_reconstruct_source_records",
			},
		},
	}
}

type traceDBSourceRawSegmentAudit struct {
	segmentCount       int
	eventFormatCount   int
	eventFormatBytes   int64
	cmdlineCount       int
	cmdlineBytes       int64
	tgidCount          int
	tgidBytes          int64
	rawCount           int
	rawBytes           int64
	headerPageCount    int
	headerPageBytes    int64
	printkCount        int
	printkBytes        int64
	kallsymsCount      int
	kallsymsBytes      int64
	unknownCount       int
	unknownBytes       int64
	unknownTypes       []uint32
	unknownTypeSeen    map[uint32]bool
	unknownTypeOmitted int
	profilerBoundary   bool
}

// scanTraceDBSourceNameInventory reads the common V1 segment envelope and only
// the SEGMENT_CMDLINES payload. Other payloads are range-audited and counted
// but never decoded, so both Harmony RMQ file_type=1 and OpenHarmony/Linux
// file_type=0 can disclose saved cmdlines and raw-authority availability
// without pretending their page layouts are interchangeable. Unsupported
// envelopes and malformed optional metadata are soft, typed absence;
// source-generation/cancellation failures remain hard.
func scanTraceDBSourceNameInventory(ctx context.Context, input conversionInputView) (inventory traceDBSourceNameInventory, err error) {
	inventory = newTraceDBSourceNameInventory()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageExternalTool, nil); err != nil {
		return inventory, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, conversionInputStageExternalTool, err)
		if err != nil {
			inventory = newTraceDBSourceNameInventory()
		}
	}()
	size := input.Size()
	if size < fileHeaderSize {
		inventory.Coverage.Skipped = "source cmdline inventory unavailable: truncated common trace header"
		inventory.RawAuthority.Skipped = "source raw authority unavailable: truncated_common_trace_header"
		return inventory, nil
	}
	reader := io.NewSectionReader(input, 0, size)
	header, headerErr := readFileHeader(reader)
	if headerErr != nil {
		inventory.Coverage.Skipped = "source cmdline inventory unavailable: invalid common trace header"
		inventory.RawAuthority.Skipped = "source raw authority unavailable: invalid_common_trace_header"
		return inventory, nil
	}
	if (header.Magic != harmonyRMQMagic && header.Magic != traceStreamerRawTraceMagic) ||
		header.Version != harmonyRMQVersion ||
		(header.FileType != 0 && header.FileType != harmonyRMQFileType) {
		inventory.Coverage.Skipped = fmt.Sprintf(
			"source cmdline inventory unavailable: unsupported envelope magic=0x%04x version=%d file_type=%d",
			header.Magic, header.Version, header.FileType)
		inventory.RawAuthority.Metadata["envelope_profile"] = "unsupported"
		inventory.RawAuthority.Metadata["magic"] = fmt.Sprintf("0x%04x", header.Magic)
		inventory.RawAuthority.Metadata["version"] = strconv.Itoa(int(header.Version))
		inventory.RawAuthority.Metadata["file_type"] = strconv.Itoa(int(header.FileType))
		inventory.RawAuthority.Skipped = "source raw authority unavailable: unsupported_common_trace_envelope"
		return inventory, nil
	}
	inventory.RawAuthority.Found = true
	inventory.RawAuthority.Metadata["magic"] = fmt.Sprintf("0x%04x", header.Magic)
	inventory.RawAuthority.Metadata["version"] = strconv.Itoa(int(header.Version))
	inventory.RawAuthority.Metadata["file_type"] = strconv.Itoa(int(header.FileType))
	inventory.RawAuthority.Metadata["cpu_count_hint"] = strconv.Itoa(header.CPUNum)
	if header.Magic == traceStreamerRawTraceMagic {
		traceDBAddCoverageMetric(&inventory.Coverage, "source_envelope_official_rawtrace_v1", 1)
		inventory.RawAuthority.Metadata["envelope_profile"] = "official_rawtrace_v1"
	} else {
		traceDBAddCoverageMetric(&inventory.Coverage, "source_envelope_legacy_rmq_v1", 1)
		inventory.RawAuthority.Metadata["envelope_profile"] = "legacy_rmq_v1"
	}

	rawAudit := traceDBSourceRawSegmentAudit{unknownTypeSeen: map[uint32]bool{}}
	ambiguous := map[int64]bool{}
	cmdlineSegments := 0
	rejectedRows := 0
	duplicateRows := 0
	conflictingRows := 0
	totalRows := 0
	incomplete := ""
	for {
		if err := ctx.Err(); err != nil {
			return inventory, err
		}
		position, seekErr := reader.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return inventory, seekErr
		}
		remaining := size - position
		if remaining == 0 {
			break
		}
		if remaining < segmentHdrSize {
			incomplete = "truncated segment header"
			break
		}
		segmentType, segmentSize, readErr := readSegmentHeader(reader)
		if readErr != nil {
			incomplete = "unreadable segment header"
			break
		}
		if isProfilerTraceHeaderPrefix(segmentType, segmentSize) {
			rawAudit.profilerBoundary = true
			break
		}
		rawAudit.segmentCount++
		if rawAudit.segmentCount > maxTraceDBSourceSegmentInventory {
			incomplete = "segment inventory cap exceeded"
			break
		}
		payloadOffset, seekErr := reader.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return inventory, seekErr
		}
		if payloadOffset < 0 || payloadOffset > size || int64(segmentSize) > size-payloadOffset {
			incomplete = "segment payload exceeds immutable input"
			break
		}
		rawAudit.observe(header, segmentType, segmentSize)
		if segmentType != segmentCmdlines {
			if _, seekErr := reader.Seek(int64(segmentSize), io.SeekCurrent); seekErr != nil {
				incomplete = "segment skip failed"
				break
			}
			continue
		}
		cmdlineSegments++
		if cmdlineSegments > maxTraceDBSourceCmdlineSegments {
			incomplete = "cmdline segment cap exceeded"
			break
		}
		if segmentSize > maxTraceDBSourceCmdlineSegmentBytes {
			incomplete = "cmdline segment byte cap exceeded"
			break
		}
		payload := make([]byte, int(segmentSize))
		if _, readErr := io.ReadFull(reader, payload); readErr != nil {
			incomplete = "cmdline segment payload truncated"
			break
		}
		for len(payload) > 0 {
			line := payload
			if index := bytes.IndexByte(payload, '\n'); index >= 0 {
				line = payload[:index]
				payload = payload[index+1:]
			} else {
				payload = nil
			}
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			totalRows++
			if totalRows > maxTraceDBSourceCmdlineRows {
				incomplete = "cmdline row cap exceeded"
				break
			}
			tid, name, rowOK := traceDBStrictSourceCmdlineRow(line)
			if !rowOK {
				rejectedRows++
				if parsedTID, tidOK := traceDBSourceCmdlineLeadingTID(line); tidOK {
					ambiguous[parsedTID] = true
					delete(inventory.Names, parsedTID)
				}
				continue
			}
			if ambiguous[tid] {
				continue
			}
			if existing, exists := inventory.Names[tid]; exists {
				if existing == name {
					duplicateRows++
					continue
				}
				conflictingRows++
				ambiguous[tid] = true
				delete(inventory.Names, tid)
				continue
			}
			inventory.Names[tid] = name
		}
		if incomplete != "" {
			break
		}
	}
	finalizeTraceDBSourceRawAuthority(&inventory.RawAuthority, header, rawAudit, incomplete)
	inventory.Coverage.Found = cmdlineSegments > 0
	inventory.Coverage.RowsRead = totalRows
	inventory.Coverage.RowsEmitted = len(inventory.Names)
	traceDBAddCoverageMetric(&inventory.Coverage, "cmdline_segments", int64(cmdlineSegments))
	traceDBAddCoverageMetric(&inventory.Coverage, "cmdline_rows_rejected", int64(rejectedRows))
	traceDBAddCoverageMetric(&inventory.Coverage, "cmdline_rows_duplicate_same_name", int64(duplicateRows))
	traceDBAddCoverageMetric(&inventory.Coverage, "cmdline_rows_conflicting_name", int64(conflictingRows))
	traceDBAddCoverageMetric(&inventory.Coverage, "cmdline_tids_ambiguous", int64(len(ambiguous)))
	if incomplete != "" {
		inventory.Names = map[int64]string{}
		inventory.Coverage.RowsEmitted = 0
		traceDBAddCoverageMetric(&inventory.Coverage, "inventory_incomplete", 1)
		inventory.Coverage.Skipped = "source cmdline inventory withheld: " + incomplete
	} else if cmdlineSegments == 0 {
		inventory.Coverage.Skipped = "source cmdline inventory unavailable: SEGMENT_CMDLINES absent"
	} else if rejectedRows > 0 || conflictingRows > 0 {
		inventory.Coverage.Skipped = fmt.Sprintf(
			"source cmdline rows withheld by exact TID/name audit: rejected=%d conflicting=%d ambiguous_tids=%d",
			rejectedRows, conflictingRows, len(ambiguous))
	}
	return inventory, nil
}

func (audit *traceDBSourceRawSegmentAudit) observe(header fileHeader, segmentType, segmentSize uint32) {
	if audit == nil {
		return
	}
	size := int64(segmentSize)
	switch {
	case segmentType == segmentEventsFormat:
		audit.eventFormatCount++
		audit.eventFormatBytes += size
	case segmentType == segmentCmdlines:
		audit.cmdlineCount++
		audit.cmdlineBytes += size
	case segmentType == segmentTGIDs:
		audit.tgidCount++
		audit.tgidBytes += size
	case segmentType == segmentHeaderPage:
		audit.headerPageCount++
		audit.headerPageBytes += size
	case segmentType == segmentPrintk:
		audit.printkCount++
		audit.printkBytes += size
	case segmentType == segmentKallsyms:
		audit.kallsymsCount++
		audit.kallsymsBytes += size
	case isRawTraceSegment(segmentType, header.CPUNum):
		audit.rawCount++
		audit.rawBytes += size
	default:
		audit.unknownCount++
		audit.unknownBytes += size
		if audit.unknownTypeSeen[segmentType] {
			return
		}
		audit.unknownTypeSeen[segmentType] = true
		if len(audit.unknownTypes) >= maxTraceDBSourceUnknownSegmentTypes {
			audit.unknownTypeOmitted++
			return
		}
		audit.unknownTypes = append(audit.unknownTypes, segmentType)
	}
}

func finalizeTraceDBSourceRawAuthority(
	coverage *TraceDBCoverage,
	header fileHeader,
	audit traceDBSourceRawSegmentAudit,
	incomplete string,
) {
	if coverage == nil {
		return
	}
	coverage.RowsRead = audit.segmentCount
	traceDBAddCoverageMetric(coverage, "segment_records_audited", int64(audit.segmentCount))
	traceDBAddCoverageMetric(coverage, "event_format_segments", int64(audit.eventFormatCount))
	traceDBAddCoverageMetric(coverage, "event_format_bytes", audit.eventFormatBytes)
	traceDBAddCoverageMetric(coverage, "cmdline_segments", int64(audit.cmdlineCount))
	traceDBAddCoverageMetric(coverage, "cmdline_bytes", audit.cmdlineBytes)
	traceDBAddCoverageMetric(coverage, "tgid_segments", int64(audit.tgidCount))
	traceDBAddCoverageMetric(coverage, "tgid_bytes", audit.tgidBytes)
	traceDBAddCoverageMetric(coverage, "raw_trace_segments", int64(audit.rawCount))
	traceDBAddCoverageMetric(coverage, "raw_trace_bytes", audit.rawBytes)
	traceDBAddCoverageMetric(coverage, "header_page_segments", int64(audit.headerPageCount))
	traceDBAddCoverageMetric(coverage, "header_page_bytes", audit.headerPageBytes)
	traceDBAddCoverageMetric(coverage, "printk_segments", int64(audit.printkCount))
	traceDBAddCoverageMetric(coverage, "printk_bytes", audit.printkBytes)
	traceDBAddCoverageMetric(coverage, "kallsyms_segments", int64(audit.kallsymsCount))
	traceDBAddCoverageMetric(coverage, "kallsyms_bytes", audit.kallsymsBytes)
	traceDBAddCoverageMetric(coverage, "unknown_segments", int64(audit.unknownCount))
	traceDBAddCoverageMetric(coverage, "unknown_segment_bytes", audit.unknownBytes)
	traceDBAddCoverageMetric(coverage, "unknown_segment_types_omitted", int64(audit.unknownTypeOmitted))
	if audit.profilerBoundary {
		traceDBAddCoverageMetric(coverage, "trailing_profiler_boundary", 1)
	}
	if len(audit.unknownTypes) > 0 {
		sort.Slice(audit.unknownTypes, func(i, j int) bool { return audit.unknownTypes[i] < audit.unknownTypes[j] })
		values := make([]string, 0, len(audit.unknownTypes))
		for _, value := range audit.unknownTypes {
			values = append(values, strconv.FormatUint(uint64(value), 10))
		}
		coverage.Metadata["unknown_segment_types"] = strings.Join(values, ",")
	}
	coverage.Metadata["event_format_state"] = traceDBSourceSegmentPresenceState(audit.eventFormatCount, audit.eventFormatBytes)
	coverage.Metadata["raw_payload_state"] = traceDBSourceSegmentPresenceState(audit.rawCount, audit.rawBytes)
	if incomplete != "" {
		coverage.RowsEmitted = 0
		coverage.Metadata["inventory_state"] = "incomplete"
		coverage.Metadata["decode_authority"] = "unavailable_segment_inventory_incomplete"
		coverage.Skipped = "source raw authority inventory withheld: " + strings.ReplaceAll(incomplete, " ", "_")
		traceDBAddCoverageMetric(coverage, "segment_inventory_incomplete", 1)
		return
	}
	coverage.RowsEmitted = audit.segmentCount
	if audit.profilerBoundary {
		coverage.Metadata["inventory_state"] = "complete_common_prefix"
	} else {
		coverage.Metadata["inventory_state"] = "complete"
	}
	traceDBAddCoverageMetric(coverage, "segment_inventory_complete", 1)
	switch {
	case audit.rawCount == 0:
		coverage.Metadata["decode_authority"] = "not_applicable_raw_payload_absent"
	case audit.rawBytes == 0:
		coverage.Metadata["decode_authority"] = "unavailable_raw_payload_empty"
	case audit.eventFormatCount == 0:
		coverage.Metadata["decode_authority"] = "unavailable_event_format_segment_absent"
	case audit.eventFormatBytes == 0:
		coverage.Metadata["decode_authority"] = "unavailable_event_format_segment_empty"
	case header.Magic == traceStreamerRawTraceMagic:
		coverage.Metadata["decode_authority"] = "unavailable_official_page_decoder_not_implemented"
		coverage.Metadata["recovery_authority"] = "requires_official_page_decoder_or_upstream_retained_rows"
	default:
		coverage.Metadata["decode_authority"] = "available_only_after_builtin_rmq_strict_format_and_page_validation"
	}
}

func traceDBSourceSegmentPresenceState(count int, bytes int64) string {
	switch {
	case count == 0:
		return "absent"
	case bytes == 0:
		return "present_empty"
	default:
		return "present_nonempty_unvalidated"
	}
}

func traceDBStrictSourceCmdlineRow(line []byte) (int64, string, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || len(line) > maxTraceDBIdentityDisplayBytes+32 {
		return 0, "", false
	}
	separator := bytes.IndexAny(line, " \t")
	if separator <= 0 {
		return 0, "", false
	}
	tidRaw := string(line[:separator])
	tid, err := strconv.ParseInt(tidRaw, 10, 32)
	if err != nil || tid <= 0 || tid > math.MaxInt32 || strconv.FormatInt(tid, 10) != tidRaw {
		return 0, "", false
	}
	name := strings.TrimSpace(string(line[separator+1:]))
	if name == "" || strings.EqualFold(name, "unknown") || name == "<...>" ||
		traceDBIdentityDisplayText(name) == "" {
		return 0, "", false
	}
	return tid, name, true
}

func traceDBSourceCmdlineLeadingTID(line []byte) (int64, bool) {
	line = bytes.TrimSpace(line)
	separator := bytes.IndexAny(line, " \t")
	if separator <= 0 {
		return 0, false
	}
	tidRaw := string(line[:separator])
	tid, err := strconv.ParseInt(tidRaw, 10, 32)
	return tid, err == nil && tid > 0 && tid <= math.MaxInt32 &&
		strconv.FormatInt(tid, 10) == tidRaw
}
