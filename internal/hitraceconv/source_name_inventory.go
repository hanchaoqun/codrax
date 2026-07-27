package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	maxTraceDBSourceCmdlineSegmentBytes = 16 << 20
	maxTraceDBSourceCmdlineSegments     = 128
	maxTraceDBSourceCmdlineRows         = 262144
)

// traceDBSourceNameInventory is display-only companion evidence from the
// immutable binary input's SEGMENT_CMDLINES records. Names are keyed by the
// exact public TID printed by the producer. They never become canonical
// ITID/IPID, owner, lifecycle, namespace, or scheduler CPU authority.
type traceDBSourceNameInventory struct {
	Names    map[int64]string
	Coverage TraceDBCoverage
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
	}
}

// scanTraceDBSourceNameInventory reads only the common V1 segment envelope and
// SEGMENT_CMDLINES payload. It deliberately does not decode raw event pages,
// so both Harmony RMQ file_type=1 and OpenHarmony/Linux file_type=0 can provide
// the common saved-cmdline companion without pretending their page layouts are
// interchangeable. Unsupported envelopes and malformed optional metadata are
// soft, typed absence; source-generation/cancellation failures remain hard.
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
		return inventory, nil
	}
	reader := io.NewSectionReader(input, 0, size)
	header, headerErr := readFileHeader(reader)
	if headerErr != nil {
		inventory.Coverage.Skipped = "source cmdline inventory unavailable: invalid common trace header"
		return inventory, nil
	}
	if (header.Magic != harmonyRMQMagic && header.Magic != traceStreamerRawTraceMagic) ||
		header.Version != harmonyRMQVersion ||
		(header.FileType != 0 && header.FileType != harmonyRMQFileType) {
		inventory.Coverage.Skipped = fmt.Sprintf(
			"source cmdline inventory unavailable: unsupported envelope magic=0x%04x version=%d file_type=%d",
			header.Magic, header.Version, header.FileType)
		return inventory, nil
	}
	if header.Magic == traceStreamerRawTraceMagic {
		traceDBAddCoverageMetric(&inventory.Coverage, "source_envelope_official_rawtrace_v1", 1)
	} else {
		traceDBAddCoverageMetric(&inventory.Coverage, "source_envelope_legacy_rmq_v1", 1)
	}

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
