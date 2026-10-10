package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// LogQuery reads only the host-attached Catalog. Model-supplied paths, preview
// strings and serialized metadata can never construct that source authority.
type LogQuery struct {
	ReadOnly
	EvidenceTool
}

func (*LogQuery) Name() string { return "log_query" }
func (*LogQuery) Description() string {
	return "Query complete attached runtime log sources, independently of the bounded log preview. Filters combine with AND; source_ids and kinds each select a union. Counts refer only to matching logical records in selected, successfully revalidated source files, not complete recording coverage. Inclusive physical line bounds return whole overlapping records (a header may precede first_line). Page in input-source then physical-line order; no inferred cross-source time order. Known fields preserve zero; absent PID/TID stay unknown. Hilog civil timestamps have unknown year/timezone. Kmsg boot nanoseconds are exact decimal strings local to their source, not an established trace clock. Malformed and unknown records remain searchable with exact decoded raw bytes in the JSON payload; gzip line/byte coordinates refer to decompressed original bytes. Adjacent logs and log timestamps alone do not prove a trace cause or source-code behavior."
}
func (*LogQuery) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"source_ids":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":128},"description":"Exact attached source IDs from the log source manifest; omitted means all attached sources."},
"kinds":{"type":"array","maxItems":3,"items":{"type":"string","enum":["hilog","kmsg","text"]},"description":"Optional format filter; malformed records retain recognizable format, text covers unrecognized records."},
"pid":{"type":"integer","minimum":0,"description":"Recorded process ID; omitted does not filter, zero is a real value."},
"tid":{"type":"integer","minimum":0,"description":"Recorded thread ID; omitted does not filter, zero is a real value."},
"contains":{"type":"string","maxLength":4096,"description":"Literal case-sensitive substring in complete decoded raw record bytes; no regular expressions."},
"first_line":{"type":"integer","minimum":0,"description":"Inclusive lower physical line bound per source; zero/omitted is unbounded."},
"last_line":{"type":"integer","minimum":0,"description":"Inclusive upper physical line bound per source; zero/omitted is unbounded."},
"offset":{"type":"integer","minimum":0,"description":"Matching-record offset, default 0; follow next_call without changing filters."},
"limit":{"type":"integer","minimum":0,"maximum":50,"description":"Maximum full records in this page; 0/omitted defaults to 12. Every returned record is previewed; use its raw_call for full original text/bytes."},
"record_ref":{"type":"string","minLength":1,"maxLength":256,"description":"Exact record_id previously returned by log_query. Reads original bytes of this record only; mutually exclusive with all query filters/offset/limit."},
"byte_offset":{"type":"integer","minimum":0,"description":"Only with record_ref: zero-based offset into decoded original record bytes, default 0."},
"byte_limit":{"type":"integer","minimum":0,"maximum":16384,"description":"Only with record_ref: chunk bytes, default 4096. Follow next_call to retrieve the full record."}
}}`)
}

type logQueryParams struct {
	loginput.Query
	RecordRef  string `json:"record_ref,omitempty"`
	ByteOffset int    `json:"byte_offset,omitempty"`
	ByteLimit  int    `json:"byte_limit,omitempty"`
}

type logQueryPayload struct {
	Query  loginput.Query  `json:"query"`
	Result loginput.Result `json:"result"`
}

func (t *LogQuery) Execute(ctx *types.BusContext, raw json.RawMessage) (types.ToolResult, error) {
	out := types.ToolResult{ToolName: t.Name(), Timestamp: time.Now()}
	// A repeated filter cannot be mechanically repaired by choosing its last
	// value: that would silently change which source records authorize facts.
	if err := logQueryUniqueKeys(raw); err != nil {
		out.Summary = "invalid log_query arguments: " + err.Error()
		return out, nil
	}
	var params logQueryParams
	normalized, failure, err := decodeStrictToolParams(t.Name(), raw, t.Parameters(), &params, nil)
	if failure != nil || err != nil {
		if failure != nil {
			return *failure, err
		}
		return out, err
	}
	if err := toolparam.Validate(normalized, t.Parameters()); err != nil {
		out.Summary = "invalid log_query arguments: " + err.Error()
		return out, nil
	}
	if ctx == nil || ctx.AttachedLogCatalog == nil {
		out.Summary = "log_query requires complete attached log sources; a preview alone does not authorize queries. Attach a log file or complete pasted log first."
		return out, nil
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(normalized, &fields)
	if params.RecordRef != "" {
		for key := range fields {
			if key != "record_ref" && key != "byte_offset" && key != "byte_limit" {
				out.Summary = "record_ref byte retrieval is mutually exclusive with query filters, offset and limit"
				return out, nil
			}
		}
		return t.readRecord(ctx, params, out)
	}
	if _, offset := fields["byte_offset"]; offset || fields["byte_limit"] != nil {
		out.Summary = "byte_offset and byte_limit require record_ref"
		return out, nil
	}
	query := params.Query
	if query.Limit == 0 {
		query.Limit = 12
	}
	result, err := ctx.AttachedLogCatalog.Query(contextFromBus(ctx), query)
	if err != nil {
		out.Summary = "log query failed: " + err.Error()
		return out, err
	}
	payload := logQueryPayload{Query: query, Result: result}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}
	ref := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "log-query-result.json", string(encoded))
	if strings.TrimSpace(ctxWorkDir(ctx)) != "" && ref == "" {
		out.Summary = "log query result could not be saved; no payload locator or evidence was published"
		return out, nil
	}
	if err := contextFromBus(ctx).Err(); err != nil {
		out.Summary = "log query canceled before publication"
		return out, err
	}
	digest := sha256.Sum256(encoded)
	scope := hex.EncodeToString(digest[:12])
	if ref == "" {
		// Tests/embedding without an output directory retain a complete JSON
		// carrier inline; never invent a file locator or use head/tail clipping.
		out.Summary = string(encoded)
	} else {
		out.Summary = logQuerySummary(payload, ref, scope)
	}
	out.RawRef = ref
	for _, source := range result.Sources {
		out.Success = out.Success || source.Complete
	}
	if out.Success {
		out.Observations = logQueryObservations(payload, ref, scope, out.Timestamp)
	}
	return out, nil
}

func logQueryUniqueKeys(raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fmt.Errorf("expected one valid JSON object")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("expected a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return err
		}
		key := token.(string)
		if seen[key] {
			return fmt.Errorf("duplicate filter key %q; provide one value", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	return nil
}

func logQuerySummary(payload logQueryPayload, ref, scope string) string {
	result := payload.Result
	records := make([]map[string]any, 0, len(result.Records))
	for i, record := range result.Records {
		preview := logQueryRecordPreview(record)
		preview["payload_pointer"] = fmt.Sprintf("/result/records/%d", i)
		preview["observation_id"] = logQueryRecordObservationID(scope, record.ID)
		preview["raw_call"] = map[string]any{"record_ref": record.ID}
		records = append(records, preview)
	}
	var sources []map[string]any
	healthy := false
	for _, source := range result.Sources {
		healthy = healthy || source.Complete
		if len(sources) < 16 {
			sources = append(sources, map[string]any{"source_id": source.ID, "name_preview": logQueryClip(source.Name, 256), "path_preview": logQueryClip(source.Path, 256), "generation": source.Generation, "complete": source.Complete, "error": logQueryClip(source.Error, 512)})
		}
	}
	var sourceErrors []loginput.SourceError
	for _, problem := range result.SourceErrors {
		if len(sourceErrors) == 8 {
			break
		}
		problem.Error = logQueryClip(problem.Error, 512)
		sourceErrors = append(sourceErrors, problem)
	}
	page := map[string]any{"query": payload.Query, "matched": result.Matched, "returned": result.Returned, "omitted": result.Omitted, "offset": result.Offset, "limit": result.Limit, "source_reads_complete": result.Complete,
		"result_byte_limit_reached": result.ResultByteLimitReached, "source_errors": sourceErrors, "source_errors_omitted": len(result.SourceErrors) - len(sourceErrors), "sources": sources, "sources_total": len(result.Sources), "sources_preview_omitted": len(result.Sources) - len(sources),
		"records_preview": records, "records_preview_omitted": result.Returned - len(records), "full_page_json": ref,
		"boundary": "Counts cover matching records in selected healthy source generations only. Complete means source read to EOF, not recording completeness. Raw bytes and full fields are in full_page_json. Source-local boot time is not mapped to trace; wall year/timezone remain unknown. No causal or source-code authority."}
	if healthy {
		page["coverage_observation_id"] = "log_query:" + scope + "#coverage"
	} else {
		page["error"] = "No selected source was successfully revalidated. Zero counters are not evidence of absent log events."
	}
	next := result.Offset + int64(result.Returned)
	if next < result.Matched && result.Returned > 0 {
		q := payload.Query
		q.Offset = next
		page["next_call"] = q
	}
	if result.ResultByteLimitReached && result.Returned == 0 {
		page["paging_issue"] = "The next matching record exceeds the result-byte budget; do not skip it or claim an empty result."
	}
	encoded, _ := json.Marshal(page)
	if len(encoded) > 64<<10 {
		// Keep every identity and its executable raw retrieval call even when
		// verbose text/escaping would exceed the summary budget.
		for _, record := range records {
			delete(record, "message_preview")
			delete(record, "raw_text_preview")
			record["text_fields_truncated"] = true
		}
		encoded, _ = json.Marshal(page)
	}
	return string(encoded)
}

func (t *LogQuery) readRecord(ctx *types.BusContext, params logQueryParams, out types.ToolResult) (types.ToolResult, error) {
	record, source, err := ctx.AttachedLogCatalog.ReadRecord(contextFromBus(ctx), params.RecordRef)
	if err != nil {
		out.Summary = "log record retrieval failed: " + err.Error()
		return out, err
	}
	if params.ByteOffset > len(record.RawBytes) {
		out.Summary = "byte_offset exceeds the original record byte length"
		return out, nil
	}
	if params.ByteLimit == 0 {
		params.ByteLimit = 4096
	}
	end := min(len(record.RawBytes), params.ByteOffset+params.ByteLimit)
	// For valid UTF-8 records, keep ordinary chunks readable without changing
	// byte offsets. Extremely small chunks or caller-selected mid-rune offsets
	// still progress byte-exactly and disclose base64-only when undecodable.
	if utf8.Valid(record.RawBytes) && end < len(record.RawBytes) {
		aligned := end
		for aligned > params.ByteOffset && !utf8.RuneStart(record.RawBytes[aligned]) {
			aligned--
		}
		if aligned > params.ByteOffset {
			end = aligned
		}
	}
	chunk := record.RawBytes[params.ByteOffset:end]
	text := ""
	if utf8.Valid(chunk) {
		text = string(chunk)
	}
	page := map[string]any{"record_ref": record.ID, "source": source, "record": logQueryRecordPreview(record), "byte_offset": params.ByteOffset, "byte_end": end, "total_bytes": len(record.RawBytes), "raw_base64": chunk, "raw_text": text, "chunk_utf8_valid": utf8.Valid(chunk), "record_utf8_valid": utf8.Valid(record.RawBytes), "boundary": "Exact decoded record bytes; chunk offsets are byte-relative to this record, original physical coordinates are in record. Invalid UTF-8 chunks expose base64 only. No clock mapping or causal authority."}
	if end < len(record.RawBytes) {
		page["next_call"] = map[string]any{"record_ref": record.ID, "byte_offset": end, "byte_limit": params.ByteLimit}
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		return out, err
	}
	ref := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "log-record-bytes.json", string(encoded))
	if strings.TrimSpace(ctxWorkDir(ctx)) != "" && ref == "" {
		out.Summary = "log record bytes could not be saved; no payload locator or evidence was published"
		return out, nil
	}
	if err := contextFromBus(ctx).Err(); err != nil {
		return out, err
	}
	digest := sha256.Sum256(encoded)
	scope := hex.EncodeToString(digest[:12])
	obs := logQueryObservations(logQueryPayload{Result: loginput.Result{Records: []loginput.Record{record}, Sources: []loginput.Source{source}}}, ref, scope, out.Timestamp)[0]
	obs.Predicate, obs.ClaimKey = "log_record_bytes", "log_record_bytes"
	obs.Span.JSONPointer = "/raw_base64"
	obs.Value = string(encoded)
	obs.RawExcerpt = logQueryClip(text, 1024)
	obs.Summary = fmt.Sprintf("Original log record bytes [%d,%d) of %d; source %s physical lines %d–%d", params.ByteOffset, end, len(record.RawBytes), source.ID, record.FirstLine, record.LastLine)
	page["observation_id"] = obs.ID
	if ref != "" {
		page["full_chunk_json"] = ref
	}
	summary, _ := json.Marshal(page)
	out.Success, out.RawRef, out.Summary = true, ref, string(summary)
	out.Observations = []types.ObservationRecord{obs}
	return out, nil
}

func logQueryRecordPreview(record loginput.Record) map[string]any {
	// The compact projection is visibly a preview; exact fields live at the
	// payload pointer. No truncated display field is substituted for raw bytes.
	return map[string]any{"record_id": record.ID, "source_id": record.SourceID, "kind": record.Kind, "status": record.Status,
		"first_line": record.FirstLine, "last_line": record.LastLine, "byte_start": record.ByteStart, "byte_end": record.ByteEnd,
		"pid": record.PID, "tid": record.TID, "cpu": record.CPU, "level": record.Level, "tag": logQueryClip(record.Tag, 256), "comm": logQueryClip(record.Comm, 256),
		"wall_timestamp_raw": record.WallTimestamp, "boot_timestamp_raw": logQueryClip(record.BootTimestamp, 128), "boot_timestamp_ns": logQueryClip(record.BootTimestampNS, 128), "clock_domain": record.ClockDomain,
		"message_preview": logQueryClip(record.Message, 256), "raw_text_preview": logQueryClip(record.RawText, 256), "parse_error": record.ParseError, "preview_only": true,
		"text_fields_truncated": len(record.Message) > 256 || len(record.RawText) > 256 || len(record.Tag) > 256 || len(record.Comm) > 256 || len(record.BootTimestamp) > 128 || len(record.BootTimestampNS) > 128}
}

func logQueryClip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value + "…"
}

func logQueryRecordObservationID(scope, id string) string { return "log_query:" + scope + "#" + id }

func logQueryObservations(payload logQueryPayload, ref, scope string, at time.Time) []types.ObservationRecord {
	byID := make(map[string]loginput.Source, len(payload.Result.Sources))
	for _, source := range payload.Result.Sources {
		byID[source.ID] = source
	}
	var observations []types.ObservationRecord
	for i, record := range payload.Result.Records {
		source := byID[record.SourceID]
		value, _ := json.Marshal(logQueryRecordPreview(record))
		observation := logQueryObservationBase(scope, ref, at)
		observation.ID = logQueryRecordObservationID(scope, record.ID)
		observation.ClaimKey, observation.Predicate = "log_record", "log_record"
		observation.Subject, observation.Object = record.SourceID, record.ID
		observation.SourceRef.Path = source.Path
		observation.SourceRef.ArtifactID = source.ID
		observation.SourceRef.ArtifactKind = "runtime_log"
		observation.SourceRef.TimeDomain = record.ClockDomain
		observation.SourceRef.ClockAlignment = "unmapped"
		observation.Span = types.ObservationSpan{LineStart: int(record.FirstLine), LineEnd: int(record.LastLine), JSONPointer: fmt.Sprintf("/result/records/%d", i)}
		observation.Value = string(value)
		observation.Summary = fmt.Sprintf("Recorded %s log entry at decoded physical lines %d–%d (%s)", record.Kind, record.FirstLine, record.LastLine, record.Status)
		observation.RawExcerpt = logQueryClip(record.RawText, 1024)
		observation.RichNotes = []string{"source_generation=" + source.Generation, "source_sha256=" + source.OriginalSHA256, "decoded_sha256=" + source.DecodedSHA256, "line_coordinates=decoded_original_physical_lines", "record_fields_preview_only=true; full exact fields and raw_base64 remain at payload JSON pointer", "clock_and_causal_relationships=unproven"}
		observations = append(observations, observation)
	}
	coverage := logQueryObservationBase(scope, ref, at)
	coverage.ID = "log_query:" + scope + "#coverage"
	coverage.ClaimKey, coverage.Predicate = "log_query_coverage", "log_query_coverage"
	coverage.Subject, coverage.Object = "selected healthy log sources", scope
	coverage.SourceRef.ArtifactID = "log-query:" + scope
	coverage.SourceRef.ArtifactKind = "log_query_result"
	coverage.Span.JSONPointer = "/result"
	coverage.Value, coverage.Unit = strconv.FormatInt(payload.Result.Matched, 10), "matching logical log records"
	coverage.Summary = fmt.Sprintf("%d matching logical records in selected successfully revalidated source generations; %d returned, %d omitted, %d source failures. This is query coverage, not capture completeness or causality.", payload.Result.Matched, payload.Result.Returned, payload.Result.Omitted, len(payload.Result.SourceErrors))
	queryJSON, _ := json.Marshal(payload.Query)
	sourcesJSON, _ := json.Marshal(payload.Result.Sources)
	coverage.RichNotes = []string{"query_filters=" + string(queryJSON), "source_generations=" + string(sourcesJSON), "source_reads_complete=" + strconv.FormatBool(payload.Result.Complete)}
	return append(observations, coverage)
}

func logQueryObservationBase(scope, ref string, at time.Time) types.ObservationRecord {
	return types.ObservationRecord{Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "log_query", Role: types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, ClaimAuthority: types.ObservationClaimAuthorityDirectObservation,
		SourceRef:  types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, PayloadRef: ref, RawRef: ref, QueryScopeID: scope},
		ObservedAt: at.Format(time.RFC3339Nano), Confidence: 1}
}
