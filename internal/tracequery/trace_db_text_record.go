package tracequery

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const traceDBTextRecordPrefix = "# codrax_trace_db_record/v1"

const maxTraceDBTextRecordPayloadBytes = 32 * 1024

type traceDBTextRecord struct {
	Kind         string
	TableID      int
	RowOrdinal   uint64
	Chunk        int
	Chunks       int
	TimestampNS  uint64
	PayloadBytes int
	RecordSHA256 string
	Payload      []byte
}

func parseTraceDBTextRecord(line string) (traceDBTextRecord, bool) {
	if !strings.HasPrefix(line, traceDBTextRecordPrefix+" ") {
		return traceDBTextRecord{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 11 || parts[0] != "#" || parts[1] != "codrax_trace_db_record/v1" {
		return traceDBTextRecord{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	kind, kindOK := value(2, "kind=")
	tableRaw, tableOK := value(3, "table_id=")
	ordinalRaw, ordinalOK := value(4, "row_ordinal=")
	chunkRaw, chunkOK := value(5, "chunk=")
	chunksRaw, chunksOK := value(6, "chunks=")
	timestampRaw, timestampOK := value(7, "ts_ns=")
	payloadRaw, payloadOK := value(8, "payload=")
	chunkHashRaw, chunkHashOK := value(9, "chunk_sha256=")
	recordHashRaw, recordHashOK := value(10, "record_sha256=")
	if !kindOK || !tableOK || !ordinalOK || !chunkOK || !chunksOK ||
		!timestampOK || !payloadOK || !chunkHashOK || !recordHashOK {
		return traceDBTextRecord{}, false
	}
	if kind != "schema" && kind != "row" && kind != "receipt" {
		return traceDBTextRecord{}, false
	}
	tableID, ok := parseCanonicalPositiveTraceDBTextRecordInt(tableRaw)
	if !ok {
		return traceDBTextRecord{}, false
	}
	rowOrdinal, err := strconv.ParseUint(ordinalRaw, 10, 64)
	if err != nil || strconv.FormatUint(rowOrdinal, 10) != ordinalRaw ||
		(kind == "row") != (rowOrdinal > 0) {
		return traceDBTextRecord{}, false
	}
	chunk, ok := parseCanonicalPositiveTraceDBTextRecordInt(chunkRaw)
	if !ok {
		return traceDBTextRecord{}, false
	}
	chunks, ok := parseCanonicalPositiveTraceDBTextRecordInt(chunksRaw)
	if !ok || chunk > chunks {
		return traceDBTextRecord{}, false
	}
	timestampNS, err := strconv.ParseUint(timestampRaw, 10, 64)
	if err != nil || strconv.FormatUint(timestampNS, 10) != timestampRaw {
		return traceDBTextRecord{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadRaw)
	if err != nil || len(payload) == 0 || len(payload) > maxTraceDBTextRecordPayloadBytes ||
		base64.RawURLEncoding.EncodeToString(payload) != payloadRaw {
		return traceDBTextRecord{}, false
	}
	chunkDigest := sha256.Sum256(payload)
	if !canonicalTraceDBTextRecordSHA256(chunkHashRaw) ||
		hex.EncodeToString(chunkDigest[:]) != chunkHashRaw ||
		!canonicalTraceDBTextRecordSHA256(recordHashRaw) {
		return traceDBTextRecord{}, false
	}
	if chunks == 1 && recordHashRaw != chunkHashRaw {
		return traceDBTextRecord{}, false
	}
	return traceDBTextRecord{
		Kind:         kind,
		TableID:      tableID,
		RowOrdinal:   rowOrdinal,
		Chunk:        chunk,
		Chunks:       chunks,
		TimestampNS:  timestampNS,
		PayloadBytes: len(payload),
		RecordSHA256: recordHashRaw,
		Payload:      payload,
	}, true
}

func parseCanonicalPositiveTraceDBTextRecordInt(raw string) (int, bool) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value <= 0 || strconv.Itoa(int(value)) != raw {
		return 0, false
	}
	return int(value), true
}

func canonicalTraceDBTextRecordSHA256(raw string) bool {
	if len(raw) != sha256.Size*2 || strings.ToLower(raw) != raw {
		return false
	}
	decoded, err := hex.DecodeString(raw)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == raw
}

func traceDBTextRecordEvent(lineNo int, record traceDBTextRecord, intern *stringInterner) Event {
	return Event{
		Line: lineNo,
		Ts:   float64(record.TimestampNS) / 1e9,
		CPU:  -1,
		Type: EventTraceDBRecord,
		Name: intern.intern("codrax_trace_db_record"),
		PluginFields: &PluginFields{
			Domain: "trace_db_fidelity",
			TraceDBRecord: &TraceDBRecordFields{
				Kind:         intern.intern(record.Kind),
				TableID:      record.TableID,
				RowOrdinal:   record.RowOrdinal,
				Chunk:        record.Chunk,
				Chunks:       record.Chunks,
				TimestampNS:  record.TimestampNS,
				PayloadBytes: record.PayloadBytes,
				RecordSHA256: intern.intern(record.RecordSHA256),
				Payload:      record.Payload,
			},
		},
		FieldText: intern.intern(fmt.Sprintf(
			"kind=%s table_id=%d row_ordinal=%d chunk=%d chunks=%d payload_bytes=%d record_sha256=%s",
			record.Kind, record.TableID, record.RowOrdinal, record.Chunk, record.Chunks,
			record.PayloadBytes, record.RecordSHA256,
		)),
	}
}

func countTraceDBTextRecord(idx *Index, ev Event) bool {
	if idx == nil || ev.Type != EventTraceDBRecord || ev.PluginFields == nil ||
		ev.PluginFields.TraceDBRecord == nil {
		return false
	}
	idx.TraceDBTextRecords++
	switch ev.PluginFields.TraceDBRecord.Kind {
	case "schema":
		idx.TraceDBTextSchemaRecords++
	case "row":
		idx.TraceDBTextRowRecords++
	case "receipt":
		idx.TraceDBTextReceiptRecords++
	}
	return true
}
