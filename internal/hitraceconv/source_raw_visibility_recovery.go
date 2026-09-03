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
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	// Both wire tokens are read from the parser package so the emission side
	// and the recognition side share one source (colleague_merge_audit §40.13).
	traceDBSourceRawVisibilityWire      = tracequery.SourceRawVisibilityWire
	traceDBSourceRawVisibilityEventName = tracequery.SourceRawVisibilityEventName
	traceDBSourceRawVisibilityFamily    = "source_rawtrace_visibility"
	traceDBSourceRawVisibilityTable     = "__raw_visibility__"
	// maxTraceDBSourceRawVisibilityEventNameBytes mirrors the parser's
	// event_name_b64 cap (tracequery sourceRawVisibilityOnlyPayload): a name the
	// parser would reject fails closed here, before any row reaches the sink.
	maxTraceDBSourceRawVisibilityEventNameBytes         = tracequery.SourceRawVisibilityEventNameMaxBytes
	maxTraceDBSourceRawVisibilityFormatNameWitnesses    = 32
	maxTraceDBSourceRawVisibilityFormatNameWitnessBytes = 512
)

// traceDBCarrierFamily declares one codrax wire family: lines whose wire
// token (`codrax_<family>/v<N>`, grammar tracequery.CarrierWireTokenGrammar)
// starts the line body. An ftrace-body carrier is a standard ftrace row that
// carries no semantic authority of its own and MUST publish under the single
// reserved header name — a carrier wearing the wrapped record's original
// event name is audited as a malformed semantic row by every header-name-keyed
// consumer (§40.13 root-cause class: "the carrier borrows the identity of what
// it carries"). A comment carrier is a `# <wire> …` line outside the ftrace
// row namespace. The file pair is what the structural census binds the
// declaration to: WireFile is the ONE file that declares the wire literal
// (the parser side owns every wire's bytes), EmitterFile is the file that
// renders the line and must refer to that declaration by identifier or
// `tracequery.<Ident>` selector — never by a second literal. So a new family
// cannot be added without registering here, cannot register without a
// declared and referenced wire, and an ftrace-body family cannot register
// without emitting under the reserved name.
type traceDBCarrierFamily struct {
	Wire string
	// Kind: an ftrace-body carrier publishes under the reserved header name;
	// a comment carrier is a `# <wire> …` line that is never an ftrace row.
	Kind        traceDBCarrierKind
	EventName   string
	BodyBuilder string
	// WireFile declares the wire literal (`"<wire>"` for ftrace-body families,
	// `"# <wire>"` for comment families) as a constant. Census keys: a bare
	// name is an internal/hitraceconv file, `tracequery/<name>` a parser file.
	WireFile string
	// EmitterFile renders the carrier line. It is the WireFile itself when the
	// parser package both declares and formats the line; otherwise it must
	// reference the WireFile constant.
	EmitterFile string
}

type traceDBCarrierKind string

const (
	traceDBCarrierKindFtraceBody traceDBCarrierKind = "ftrace_body"
	traceDBCarrierKindComment    traceDBCarrierKind = "comment"
)

// traceDBReservedCarrierFamilies is the single registry of every codrax wire
// family (`codrax_<family>/v<N>`) in the converter and parser packages.
// Ftrace-body carriers MUST publish under the reserved header name; comment
// carriers are `# <wire>` lines outside the ftrace row namespace.
//
// Totality (TestTraceDBReservedCarrierFamilyRegistry, §40.38 fold-in F7/F10):
// every string literal in internal/hitraceconv and internal/tracequery that
// starts with a whole wire token — bare or behind the `# ` comment prefix —
// belongs to a family registered here, every registered family's WireFile
// declares its wire literal and its EmitterFile references that declaration,
// and both files are in the scanned set. A wire token that exists in either
// package and is not listed below turns the census red.
var traceDBReservedCarrierFamilies = []traceDBCarrierFamily{
	{
		Wire:        traceDBSourceRawVisibilityWire,
		Kind:        traceDBCarrierKindFtraceBody,
		EventName:   traceDBSourceRawVisibilityEventName,
		BodyBuilder: "traceDBSourceRawVisibilityBody",
		WireFile:    "tracequery/source_raw_visibility.go",
		EmitterFile: "source_raw_visibility_recovery.go",
	},
	// SQL text-fidelity export: the parser declares the wire, the converter
	// renders the lines (streamerdb_text_fidelity.go).
	{Wire: "codrax_trace_db_record/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/trace_db_text_record.go", EmitterFile: "streamerdb_text_fidelity.go"},
	{Wire: "codrax_trace_db_block/v2", Kind: traceDBCarrierKindComment, WireFile: "tracequery/trace_db_text_record.go", EmitterFile: "streamerdb_text_fidelity.go"},
	// Parser-formatted comment carriers: one file declares and renders each.
	{Wire: "codrax_sched_wakeup_cpu_unavailable/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/cpu_unavailable_wakeup.go", EmitterFile: "tracequery/cpu_unavailable_wakeup.go"},
	{Wire: "codrax_trace_mark_cpu_unavailable/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/cpu_unavailable_trace_mark.go", EmitterFile: "tracequery/cpu_unavailable_trace_mark.go"},
	{Wire: "codrax_frame_map/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/frame_map_relation.go", EmitterFile: "tracequery/frame_map_relation.go"},
	{Wire: "codrax_frame_callstack/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/official_sql_relations.go", EmitterFile: "tracequery/official_sql_relations.go"},
	{Wire: "codrax_frame_gpu/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/official_sql_relations.go", EmitterFile: "tracequery/official_sql_relations.go"},
	{Wire: "codrax_perf_napi_async/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/official_sql_relations.go", EmitterFile: "tracequery/official_sql_relations.go"},
	{Wire: "codrax_trace_async_interval/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/completed_async_interval.go", EmitterFile: "tracequery/completed_async_interval.go"},
	{Wire: "codrax_trace_mark_exact/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/exact_trace_mark.go", EmitterFile: "tracequery/exact_trace_mark.go"},
	{Wire: "codrax_ebpf_interval/v1", Kind: traceDBCarrierKindComment, WireFile: "tracequery/official_ebpf_interval.go", EmitterFile: "tracequery/official_ebpf_interval.go"},
}

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
			"viewer":     "standard ftrace row under the reserved " + traceDBSourceRawVisibilityEventName + " event name so header-name-keyed readers never mistake the carrier for the wrapped record; the original event name is recoverable from event_name_b64 and the schema payload; generic viewers can display the occurrence and payload while unaware readers ignore the Codrax body contract",
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

// traceDBSourceRawVisibilityBody renders the carrier body. The header name is
// NOT derived from the format: every carrier row is published under
// traceDBSourceRawVisibilityEventName and the original name travels only in
// the typed event_name_b64 token (and the schema payload). The name length
// guard mirrors the parser's cap so an unpublishable name fails the family
// closed here instead of surfacing later as a less specific count mismatch.
func traceDBSourceRawVisibilityBody(
	format eventFormat,
	content []byte,
	schema *traceDBSourceRawVisibilitySchemaWire,
) (string, error) {
	if schema == nil || len(schema.payload) == 0 || len(schema.digest) != sha256.Size*2 ||
		len(content) < 2 || len(format.Name) == 0 ||
		len(format.Name) > maxTraceDBSourceRawVisibilityEventNameBytes {
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
					traceDBSourceRawVisibilityEventName+": "+visibilityBody)
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
	out.Metadata["wire"] = fmt.Sprintf("%s; event_name=%s; exact payload+schema; semantic_authority=none",
		traceDBSourceRawVisibilityWire, traceDBSourceRawVisibilityEventName)
	// The header name no longer says which source formats were wrapped, so the
	// diagnostic report keeps the published original names as a bounded
	// witness (the `_witness` suffix is the shared sideband contract).
	names := make([]string, 0, len(schemas))
	for formatID := range schemas {
		names = append(names, catalog.Formats[formatID].Name)
	}
	out.Metadata["published_format_names_witness"] = traceDBSourceRawVisibilityFormatNamesWitness(names)
	return out, nil
}

// traceDBSourceRawVisibilityFormatNamesWitness renders the sorted original
// event names published through the reserved carrier as one bounded witness
// value (name count and byte caps; the omitted tail is counted, never
// truncated mid-name). Unsafe names take the shared decode-ledger witness form.
func traceDBSourceRawVisibilityFormatNamesWitness(names []string) string {
	sorted := make([]string, 0, len(names))
	for _, name := range names {
		sorted = append(sorted, traceDBRawDecodeFormatWitnessName(name))
	}
	sort.Strings(sorted)
	var builder strings.Builder
	emitted := 0
	for _, name := range sorted {
		if emitted >= maxTraceDBSourceRawVisibilityFormatNameWitnesses ||
			builder.Len()+len(name)+1 > maxTraceDBSourceRawVisibilityFormatNameWitnessBytes {
			break
		}
		if emitted > 0 {
			builder.WriteByte(';')
		}
		builder.WriteString(name)
		emitted++
	}
	if omitted := len(sorted) - emitted; omitted > 0 {
		builder.WriteString(fmt.Sprintf(";+%d more", omitted))
	}
	return builder.String()
}
