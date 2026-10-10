package types

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// NativeRuntimeFactPresentationKind classifies a producer-owned presentation
// receipt, not causal evidence. It deliberately does not expand the shared
// RuntimeObservationProducerIsDeterministicQuery authority predicate.
func NativeRuntimeFactPresentationKind(record ObservationRecord) string {
	if record.Origin != AnswerEvidenceOriginRuntimeArtifact || record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		record.ClaimAuthority != ObservationClaimAuthorityDirectObservation || record.GroundingPolicy != ClaimGroundingHard ||
		record.ID == "" || record.SourceRef.PayloadRef == "" || record.SourceRef.QueryScopeID == "" {
		return ""
	}
	switch runtimeObservationProducerBase(record.Producer) {
	case "trace_query":
		return "trace_record"
	case "log_query":
		if record.ProvenanceLane != ObservationProvenanceArtifactSpan || record.SourceRef.ArtifactID == "" {
			return ""
		}
		if record.Predicate == "log_query_coverage" && record.ClaimKey == record.Predicate && record.SourceRef.ArtifactKind == "log_query_result" {
			return "log_query_coverage"
		}
		if _, ok := decodeNativeLogPresentation(record); ok {
			return "log_record"
		}
	}
	return ""
}

func IsNativeRuntimeFactPresentationRecord(record ObservationRecord) bool {
	return NativeRuntimeFactPresentationKind(record) != ""
}

// This is the existing producer Value schema, not a model-facing schema. Long
// text is explicitly a producer preview. Identity/time/status scalars stay
// whole in the projected JSON; absent numeric identity remains JSON null.
type nativeLogPresentation struct {
	RecordID            string `json:"record_id"`
	SourceID            string `json:"source_id"`
	Kind                string `json:"kind"`
	Status              string `json:"status"`
	FirstLine           int    `json:"first_line"`
	LastLine            int    `json:"last_line"`
	ByteStart           int64  `json:"byte_start"`
	ByteEnd             int64  `json:"byte_end"`
	PID                 *int64 `json:"pid"`
	TID                 *int64 `json:"tid"`
	CPU                 *int64 `json:"cpu"`
	Level               string `json:"level"`
	Tag                 string `json:"tag"`
	Comm                string `json:"comm"`
	WallTimestamp       string `json:"wall_timestamp_raw"`
	BootTimestamp       string `json:"boot_timestamp_raw"`
	BootTimestampNS     string `json:"boot_timestamp_ns"`
	ClockDomain         string `json:"clock_domain"`
	MessagePreview      string `json:"message_preview"`
	RawTextPreview      string `json:"raw_text_preview"`
	ParseError          string `json:"parse_error"`
	PreviewOnly         bool   `json:"preview_only"`
	TextFieldsTruncated bool   `json:"text_fields_truncated"`
}

func decodeNativeLogPresentation(record ObservationRecord) (nativeLogPresentation, bool) {
	var value nativeLogPresentation
	if record.Predicate != "log_record" || record.ClaimKey != record.Predicate || record.SourceRef.ArtifactKind != "runtime_log" || !nativeLogPresentationKeys(record.Value) {
		return value, false
	}
	d := json.NewDecoder(strings.NewReader(record.Value))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF || !value.PreviewOnly ||
		value.RecordID == "" || value.RecordID != record.Object || value.SourceID != record.SourceRef.ArtifactID ||
		value.FirstLine <= 0 || value.LastLine < value.FirstLine || value.FirstLine != record.Span.LineStart || value.LastLine != record.Span.LineEnd ||
		value.ClockDomain != record.SourceRef.TimeDomain {
		return nativeLogPresentation{}, false
	}
	switch value.Status {
	case "parsed", "malformed", "unknown", "orphan_continuation":
	default:
		return nativeLogPresentation{}, false
	}
	return value, true
}

func nativeLogPresentationKeys(raw string) bool {
	// A model-shaped object cannot gain this display lane via duplicate keys,
	// missing-null identity fields or JSON's case-insensitive struct matching.
	keys := strings.Fields("record_id source_id kind status first_line last_line byte_start byte_end pid tid cpu level tag comm wall_timestamp_raw boot_timestamp_raw boot_timestamp_ns clock_domain message_preview raw_text_preview parse_error preview_only text_fields_truncated")
	want := make(map[string]bool, len(keys))
	for _, key := range keys {
		want[key] = true
	}
	d := json.NewDecoder(strings.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !want[key] {
			return false
		}
		delete(want, key)
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	token, err := d.Token()
	return err == nil && token == json.Delim('}') && len(want) == 0 && d.Decode(new(any)) == io.EOF
}

func nativeLogPromptValue(record ObservationRecord) (string, bool) {
	if NativeRuntimeFactPresentationKind(record) != "log_record" {
		return "", false
	}
	v, _ := decodeNativeLogPresentation(record)
	// Keep compact identity/line coordinates together with the value so a
	// consumer never needs to reconstruct them from shortened source labels.
	// Byte ranges, raw-call navigation and duplicate raw text remain retained
	// in the original observation/payload, not repeated in each compact row.
	fields := map[string]any{
		"record_id": v.RecordID, "source_id": v.SourceID, "first_line": v.FirstLine, "last_line": v.LastLine,
		"kind": v.Kind, "status": v.Status, "pid": v.PID, "tid": v.TID, "cpu": v.CPU,
		"level": v.Level, "tag": v.Tag, "comm": v.Comm, "clock_domain": v.ClockDomain,
		"wall_timestamp_raw": nullableLogTimestamp(v.WallTimestamp), "boot_timestamp_raw": nullableLogTimestamp(v.BootTimestamp),
		"boot_timestamp_ns": nullableLogTimestamp(v.BootTimestampNS), "parse_error": v.ParseError,
		"message_preview": v.MessagePreview, "text_fields_truncated": v.TextFieldsTruncated,
	}
	encoded, _ := json.Marshal(fields)
	return string(encoded), true
}

func nativePresentationNoteRecord(record ObservationRecord) ObservationRecord {
	if NativeRuntimeFactPresentationKind(record) != "log_record" {
		return record
	}
	notes := make([]string, 0, len(record.RichNotes))
	for _, note := range record.RichNotes {
		// Duplicate digests are retained in the source receipt. Keep generation
		// here so every consumer can independently distinguish source versions;
		// a consumer with a complete source-metadata section may deduplicate it.
		if strings.HasPrefix(note, "source_sha256=") || strings.HasPrefix(note, "decoded_sha256=") {
			continue
		}
		notes = append(notes, note)
	}
	record.RichNotes = notes
	return record
}

func nullableLogTimestamp(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nativePresentationParent uses exact accepted source/query coordinates. No
// source aliases, filenames, subjects or nearby timestamps are guessed equal.
func nativePresentationParent(record ObservationRecord) string {
	ref := record.SourceRef
	generation := ""
	for _, note := range record.RichNotes {
		if value, ok := strings.CutPrefix(note, "source_generation="); ok {
			generation = value
			break
		}
	}
	value, _ := json.Marshal(struct {
		Source     ObservationSourceRef
		Generation string
	}{Source: ObservationSourceRef{Kind: ref.Kind, Path: ref.Path, CaptureIdentityPath: ref.CaptureIdentityPath,
		ArtifactID: ref.ArtifactID, QueryScopeID: ref.QueryScopeID, QueryWindowKnown: ref.QueryWindowKnown,
		QueryWindowStartTs: ref.QueryWindowStartTs, QueryWindowEndTs: ref.QueryWindowEndTs,
		QueryLineRangeKnown: ref.QueryLineRangeKnown, QueryLineStart: ref.QueryLineStart, QueryLineEnd: ref.QueryLineEnd}, Generation: generation})
	return string(value)
}

// Rebalance only slots already awarded to exact native facts. Requested source
// evidence, VCS and other origin survival floors are unchanged. Unbudgeted
// ledgers are never mutated or reordered by this prompt-only operation.
func rebalanceNativePresentationRecords(sorted, selected []ObservationRecord, intent *AnswerIntentContract, rm *RequestModel) []ObservationRecord {
	budget := 0
	for _, record := range selected {
		if IsNativeRuntimeFactPresentationRecord(record) {
			budget++
		}
	}
	if budget <= 1 {
		return selected
	}
	var groups []PresentationRowGroup
	var members [][]int
	indices, parents := map[string]int{}, map[string]bool{}
	for i, record := range sorted {
		if !IsNativeRuntimeFactPresentationRecord(record) {
			continue
		}
		parent := nativePresentationParent(record)
		priority := observationRecordRankForRequest(record, intent, rm)
		key := parent + "\x00" + record.Predicate + "\x00" + strconv.Itoa(priority)
		index, exists := indices[key]
		if !exists {
			index = len(groups)
			indices[key] = index
			parents[parent] = true
			groups = append(groups, PresentationRowGroup{Key: key, ParentKey: parent, Priority: priority})
			members = append(members, nil)
		}
		groups[index].Rows++
		members[index] = append(members[index], i)
	}
	if len(parents) <= 1 {
		return selected
	}
	counts := AllocatePresentationRows(groups, budget)
	keep := make(map[int]bool, budget)
	for i, count := range counts {
		for _, index := range members[i][:count] {
			keep[index] = true
		}
	}
	var native []ObservationRecord
	for i, record := range sorted {
		if keep[i] {
			native = append(native, record)
		}
	}
	out := append([]ObservationRecord(nil), selected...)
	index := 0
	for i, record := range out {
		if IsNativeRuntimeFactPresentationRecord(record) {
			out[i] = native[index]
			index++
		}
	}
	return out
}
