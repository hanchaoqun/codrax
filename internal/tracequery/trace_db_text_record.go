package tracequery

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const traceDBTextRecordPrefix = "# codrax_trace_db_record/v1"

const maxTraceDBTextRecordPayloadBytes = 32 * 1024

var strictTraceDBTextRecordBase64 = base64.RawURLEncoding.Strict()

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
	rest := line[len(traceDBTextRecordPrefix)+1:]
	kind, rest, kindOK := cutTraceDBTextRecordField(rest, "kind=", false)
	tableRaw, rest, tableOK := cutTraceDBTextRecordField(rest, "table_id=", false)
	ordinalRaw, rest, ordinalOK := cutTraceDBTextRecordField(rest, "row_ordinal=", false)
	chunkRaw, rest, chunkOK := cutTraceDBTextRecordField(rest, "chunk=", false)
	chunksRaw, rest, chunksOK := cutTraceDBTextRecordField(rest, "chunks=", false)
	timestampRaw, rest, timestampOK := cutTraceDBTextRecordField(rest, "ts_ns=", false)
	payloadRaw, rest, payloadOK := cutTraceDBTextRecordField(rest, "payload=", false)
	chunkHashRaw, rest, chunkHashOK := cutTraceDBTextRecordField(rest, "chunk_sha256=", false)
	recordHashRaw, rest, recordHashOK := cutTraceDBTextRecordField(rest, "record_sha256=", true)
	if !kindOK || !tableOK || !ordinalOK || !chunkOK || !chunksOK ||
		!timestampOK || !payloadOK || !chunkHashOK || !recordHashOK || rest != "" {
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
	payload, err := strictTraceDBTextRecordBase64.DecodeString(payloadRaw)
	if err != nil || len(payload) == 0 || len(payload) > maxTraceDBTextRecordPayloadBytes {
		return traceDBTextRecord{}, false
	}
	chunkDigest := sha256.Sum256(payload)
	chunkHash, chunkHashOK := decodeCanonicalTraceDBTextRecordSHA256(chunkHashRaw)
	_, recordHashOK = decodeCanonicalTraceDBTextRecordSHA256(recordHashRaw)
	if !chunkHashOK || chunkDigest != chunkHash || !recordHashOK {
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

func cutTraceDBTextRecordField(raw, prefix string, final bool) (value, rest string, ok bool) {
	if !strings.HasPrefix(raw, prefix) {
		return "", raw, false
	}
	raw = raw[len(prefix):]
	if final {
		if raw == "" || strings.IndexByte(raw, ' ') >= 0 {
			return "", raw, false
		}
		return raw, "", true
	}
	end := strings.IndexByte(raw, ' ')
	if end <= 0 {
		return "", raw, false
	}
	return raw[:end], raw[end+1:], true
}

func parseCanonicalPositiveTraceDBTextRecordInt(raw string) (int, bool) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value <= 0 || strconv.Itoa(int(value)) != raw {
		return 0, false
	}
	return int(value), true
}

func decodeCanonicalTraceDBTextRecordSHA256(raw string) ([sha256.Size]byte, bool) {
	var decoded [sha256.Size]byte
	if len(raw) != sha256.Size*2 {
		return decoded, false
	}
	for index := range decoded {
		high, highOK := canonicalTraceDBTextRecordHexNibble(raw[index*2])
		low, lowOK := canonicalTraceDBTextRecordHexNibble(raw[index*2+1])
		if !highOK || !lowOK {
			return [sha256.Size]byte{}, false
		}
		decoded[index] = high<<4 | low
	}
	return decoded, true
}

func canonicalTraceDBTextRecordHexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	default:
		return 0, false
	}
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
