package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
)

const (
	traceDBSourceRawVisibilityWire   = "codrax_source_raw_visibility/v1"
	traceDBSourceRawVisibilityFamily = "source_rawtrace_visibility"
	traceDBSourceRawVisibilityTable  = "__raw_visibility__"
)

type traceDBSourceRawVisibilitySchema struct {
	Version  int                               `json:"version"`
	ID       int                               `json:"id"`
	Name     string                            `json:"name"`
	Fields   []traceDBSourceRawVisibilityField `json:"fields"`
	PrintFmt string                            `json:"print_fmt"`
}

type traceDBSourceRawVisibilityField struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
	Signed bool   `json:"signed"`
}

type traceDBSourceRawVisibilitySchemaWire struct {
	payload []byte
	digest  string
	emitted bool
}

func newTraceDBSourceRawVisibilityCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: traceDBSourceRawVisibilityFamily,
		Table:  traceDBSourceRawVisibilityTable,
		Role:   "query_ready_background_export",
		FieldSources: map[string]string{
			"scope":      "every source raw record whose exact descriptor was admitted but which has no dedicated strict semantic decoder; strict target families are excluded",
			"envelope":   "exact source timestamp, CPU, common_pid, common_flags and common_preempt_count; namespace PID is copied verbatim and TGID remains unknown",
			"payload":    "byte-exact raw event content encoded as base64url, including the physical event id and common envelope bytes",
			"schema":     "canonical versioned JSON containing exact admitted event id/name, ordered field type/name/offset/size/signed geometry and print-fmt; the first physical row per format carries it and every row carries its SHA-256",
			"authority":  "visibility-only source_raw_visibility advisory; this carrier cannot create a span, pair, duration, CPU-supply fact, wake edge or root-cause candidate",
			"viewer":     "standard ftrace row under the original safe event name; generic viewers can display the occurrence and payload while unaware readers ignore the Codrax body contract",
			"duplicates": "supplementary source observation may coexist with a DB-derived semantic row; the visibility-only token prevents double causal accounting and is not a deduplication claim",
		},
		Metadata: map[string]string{"publication_state": "unavailable"},
	}
}

// traceDBSourceRawVisibilityPublishedRows reads the producer-owned typed
// coverage face exactly once. It binds postvalidation to the number of
// visibility carriers admitted before sorting without inspecting row prose.
func traceDBSourceRawVisibilityPublishedRows(coverage []TraceDBCoverage) (int, error) {
	rows := 0
	found := false
	for _, item := range coverage {
		if item.Family != traceDBSourceRawVisibilityFamily || item.Table != traceDBSourceRawVisibilityTable {
			continue
		}
		if found || item.RowsEmitted < 0 || item.Error != "" {
			return 0, &traceDBOutputInvariantError{Reason: "source_raw_visibility_coverage_invalid"}
		}
		found = true
		state := item.Metadata["publication_state"]
		if item.RowsEmitted > 0 && state != "published_complete_visibility_only_source_census" {
			return 0, &traceDBOutputInvariantError{Reason: "source_raw_visibility_coverage_invalid"}
		}
		if item.RowsEmitted == 0 {
			switch state {
			case "not_applicable_source_profile", "complete_no_visibility_event", "withheld_visibility_envelope_incomplete":
			default:
				return 0, &traceDBOutputInvariantError{Reason: "source_raw_visibility_coverage_invalid"}
			}
		}
		rows = item.RowsEmitted
	}
	if !found {
		return 0, &traceDBOutputInvariantError{Reason: "source_raw_visibility_coverage_missing"}
	}
	return rows, nil
}

func traceDBSourceRawVisibilityEligible(inventory *traceDBSourceNameInventory) bool {
	if inventory == nil || inventory.rawReplay == nil || inventory.rawReplay.input == nil ||
		inventory.rawReplay.header.Magic != traceStreamerRawTraceMagic ||
		!traceDBRawDecodeCensusComplete(inventory.RawDecode) {
		return false
	}
	candidates := inventory.RawDecode.Metrics["visibility_candidate_records"]
	admitted := inventory.RawDecode.Metrics["visibility_envelope_admitted"]
	rejected := inventory.RawDecode.Metrics["visibility_envelope_rejected"]
	return candidates > 0 && candidates == admitted && rejected == 0
}

func traceDBSourceRawVisibilitySchemaFor(format eventFormat) ([]byte, string, error) {
	schema := traceDBSourceRawVisibilitySchema{
		Version: 1, ID: format.ID, Name: format.Name, PrintFmt: format.PrintFmt,
		Fields: make([]traceDBSourceRawVisibilityField, 0, len(format.Fields)),
	}
	for _, field := range format.Fields {
		schema.Fields = append(schema.Fields, traceDBSourceRawVisibilityField{
			Type: field.Type, Name: field.Name, Offset: field.Offset,
			Size: field.Size, Signed: field.Signed,
		})
	}
	payload, err := json.Marshal(schema)
	if err != nil || len(payload) == 0 || len(payload) > maxTraceDBSystraceLineBytes/2 {
		return nil, "", &traceDBOutputInvariantError{
			Reason: "source_raw_visibility_schema_invalid", Cause: err,
		}
	}
	digest := sha256.Sum256(payload)
	return payload, hex.EncodeToString(digest[:]), nil
}

func traceDBSourceRawVisibilityEventName(name string) string {
	if name == "" || len(name) > 128 {
		return "codrax_source_raw_event"
	}
	for _, value := range []byte(name) {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '_' || value == '-' ||
			value == '.' || value == '/' {
			continue
		}
		return "codrax_source_raw_event"
	}
	return name
}

func traceDBSourceRawVisibilityBody(
	format eventFormat,
	content []byte,
	schema *traceDBSourceRawVisibilitySchemaWire,
) (string, error) {
	if schema == nil || len(schema.payload) == 0 || len(schema.digest) != sha256.Size*2 ||
		len(content) < 2 {
		return "", &traceDBOutputInvariantError{Reason: "source_raw_visibility_wire_invalid"}
	}
	body := traceDBSourceRawVisibilityWire +
		" semantic_authority=none format_id=" + strconv.Itoa(format.ID) +
		" event_name_b64=" + base64.RawURLEncoding.EncodeToString([]byte(format.Name)) +
		" schema_sha256=" + schema.digest +
		" payload_b64=" + base64.RawURLEncoding.EncodeToString(content)
	if !schema.emitted {
		body += " schema_b64=" + base64.RawURLEncoding.EncodeToString(schema.payload)
	}
	if len(body) > maxTraceDBSystraceLineBytes || !traceDBSinglePhysicalLine(body, false) {
		return "", &traceDBOutputInvariantError{Reason: "source_raw_visibility_wire_invalid"}
	}
	return body, nil
}

func publishTraceDBSourceRawVisibility(
	ctx context.Context,
	inventory *traceDBSourceNameInventory,
	sink *traceDBRowSink,
) (out TraceDBCoverage, resultErr error) {
	out = newTraceDBSourceRawVisibilityCoverage()
	if ctx == nil {
		ctx = context.Background()
	}
	if inventory == nil || inventory.rawReplay == nil || inventory.rawReplay.input == nil {
		out.Metadata["publication_state"] = "not_applicable_source_profile"
		out.Skipped = "source raw visibility not applicable: held official raw replay authority absent"
		return out, nil
	}
	out.Found = inventory.RawDecode.Found
	candidates := inventory.RawDecode.Metrics["visibility_candidate_records"]
	out.RowsRead = int(min(candidates, int64(math.MaxInt)))
	if candidates == 0 && traceDBRawDecodeCensusComplete(inventory.RawDecode) {
		out.Metadata["publication_state"] = "complete_no_visibility_event"
		out.Skipped = "source raw visibility complete: no non-semantic admitted format event present"
		return out, nil
	}
	if !traceDBSourceRawVisibilityEligible(inventory) {
		out.Metadata["publication_state"] = "withheld_visibility_envelope_incomplete"
		out.Skipped = "source raw visibility withheld: candidate common-envelope census is incomplete"
		return out, nil
	}
	if sink == nil {
		err := &traceDBOutputInvariantError{Reason: "trace_row_sink_missing"}
		out.Error = err.Error()
		return out, err
	}
	replay := inventory.rawReplay
	if err := completeConversionInputStage(ctx, replay.input, conversionInputStageExternalTool, nil); err != nil {
		out.Error = err.Error()
		return out, err
	}
	defer func() {
		resultErr = completeConversionInputStage(
			ctx, replay.input, conversionInputStageExternalTool, resultErr)
		if resultErr != nil && out.Error == "" {
			out.Error = resultErr.Error()
		}
	}()
	formatCoverage := TraceDBCoverage{Metadata: map[string]string{}}
	catalog, formatState := probeTraceDBSourceEventFormats(
		ctx, replay.input, replay.segments, &formatCoverage)
	if formatState != "parsed_strict" {
		err := &traceDBOutputInvariantError{Reason: "source_raw_visibility_format_replay_mismatch"}
		out.Error = err.Error()
		return out, err
	}

	schemas := make(map[int]*traceDBSourceRawVisibilitySchemaWire, len(catalog.Formats))
	page := make([]byte, tracePageSize)
	pages, knownRecords, replayCandidates := 0, int64(0), int64(0)
	var payloadBytes, schemaBytes int64
	for _, segment := range replay.segments {
		if !isRawTraceSegment(segment.Type, replay.header.CPUNum) {
			continue
		}
		if segment.Size%tracePageSize != 0 {
			return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_partial_page"}
		}
		reader := io.NewSectionReader(replay.input, segment.Offset, int64(segment.Size))
		for offset := int64(0); offset < int64(segment.Size); offset += tracePageSize {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			pages++
			if pages > maxTraceDBRawProbePages {
				return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_page_cap"}
			}
			if _, err := io.ReadFull(reader, page); err != nil {
				return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_page_read_failed", Cause: err}
			}
			pageHeader, ok := parsePageHeader(page)
			if !ok || pageHeader.CPU < 0 || pageHeader.CPU >= replay.header.CPUNum ||
				pageHeader.Length > uint64(len(page)-pageHeaderSize) {
				return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_page_geometry_mismatch"}
			}
			body := page[pageHeaderSize : pageHeaderSize+int(pageHeader.Length)]
			for eventOffset := 0; eventOffset < len(body); {
				if len(body)-eventOffset < eventHeaderSize {
					return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_event_header_truncated"}
				}
				eventHeader, ok := parseEventHeader(body[eventOffset:])
				if !ok {
					break
				}
				contentStart := eventOffset + eventHeaderSize
				contentEnd := contentStart + int(eventHeader.Size)
				next := contentStart + eventHeader.AlignedSize
				if contentEnd > len(body) || next > len(body) || eventHeader.Size < 2 ||
					pageHeader.TimestampNS > math.MaxUint64-uint64(eventHeader.TimestampOffsetNS) {
					return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_event_geometry_mismatch"}
				}
				content := body[contentStart:contentEnd]
				formatID := int(uint16(content[0]) | uint16(content[1])<<8)
				format, exists := catalog.Formats[formatID]
				if !exists {
					eventOffset = next
					continue
				}
				knownRecords++
				if traceDBRawProbeTargetFormat(format.Name) {
					eventOffset = next
					continue
				}
				replayCandidates++
				event := decodeEvent(format, content)
				headerPID, flags, preemptCount, envelopeOK := decodeDirectFtraceCommonEnvelope(event)
				if !envelopeOK {
					return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_envelope_replay_mismatch"}
				}
				schema := schemas[formatID]
				if schema == nil {
					payload, digest, err := traceDBSourceRawVisibilitySchemaFor(format)
					if err != nil {
						return out, err
					}
					schema = &traceDBSourceRawVisibilitySchemaWire{payload: payload, digest: digest}
					schemas[formatID] = schema
					schemaBytes += int64(len(payload))
				}
				visibilityBody, err := traceDBSourceRawVisibilityBody(format, content, schema)
				if err != nil {
					return out, err
				}
				task := inventory.Names[int64(headerPID)]
				if headerPID == 0 {
					task = "<idle>"
				} else if task == "" {
					task = "unknown"
				}
				ts := pageHeader.TimestampNS + uint64(eventHeader.TimestampOffsetNS)
				if ts > math.MaxInt64 || sink.stats.RowsAccepted == math.MaxInt {
					return out, &traceDBOutputInvariantError{Reason: "source_raw_visibility_sequence_invalid"}
				}
				row, err := prepareTraceDBRenderedRowEnvelopeContext(
					ctx, int64(ts), sink.stats.RowsAccepted, task,
					int64(headerPID), 0, int64(pageHeader.CPU), flags, preemptCount, true,
					traceDBSourceRawVisibilityEventName(format.Name)+": "+visibilityBody)
				if err != nil {
					return out, err
				}
				if err := sink.addContext(ctx, row, nil, false); err != nil {
					return out, err
				}
				out.RowsEmitted++
				payloadBytes += int64(len(content))
				schema.emitted = true
				eventOffset = next
			}
		}
	}
	if knownRecords != inventory.RawDecode.Metrics["records_with_admitted_format"] ||
		replayCandidates != candidates || out.RowsEmitted != int(candidates) {
		err := &traceDBOutputInvariantError{Reason: "source_raw_visibility_census_replay_mismatch"}
		out.Error = err.Error()
		return out, err
	}
	traceDBAddCoverageMetric(&out, "formats_published", int64(len(schemas)))
	traceDBAddCoverageMetric(&out, "payload_bytes_preserved", payloadBytes)
	traceDBAddCoverageMetric(&out, "schema_bytes_preserved", schemaBytes)
	traceDBAddCoverageMetric(&out, "semantic_authority_rows", 0)
	out.Metadata["publication_state"] = "published_complete_visibility_only_source_census"
	out.Metadata["wire"] = fmt.Sprintf("%s; exact payload+schema; semantic_authority=none", traceDBSourceRawVisibilityWire)
	return out, nil
}
