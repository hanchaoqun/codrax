package tracequery

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// TraceDBTextRecordPrefix and TraceDBTextBlockPrefix are the comment-carrier
// wire prefixes of the SQL text-fidelity export. The parser owns the wire
// bytes; the emitter (internal/hitraceconv streamerdb_text_fidelity.go) reads
// them from here so the two sides share one literal and the producer-side
// carrier-family census can bind the emitter to this declaration.
const (
	TraceDBTextRecordPrefix = "# codrax_trace_db_record/v1"
	TraceDBTextBlockPrefix  = "# codrax_trace_db_block/v2"

	maxTraceDBTextRecordPayloadBytes = 32 * 1024
	maxTraceDBTextBlockRawBytes      = 64 * 1024
	maxTraceDBTextBlockRecords       = 4096
)

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

type traceDBTextBlock struct {
	Block        int
	RecordCount  int
	RawBytes     int
	TimestampNS  uint64
	PayloadBytes int
	PayloadSHA   string
	RawSHA       string
	Records      []TraceDBRecordFields
}

var traceDBTextBlockInflaterPool sync.Pool

func parseTraceDBTextRecord(line string) (traceDBTextRecord, bool) {
	if !strings.HasPrefix(line, TraceDBTextRecordPrefix+" ") {
		return traceDBTextRecord{}, false
	}
	rest := line[len(TraceDBTextRecordPrefix)+1:]
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

func parseTraceDBTextBlock(line string) (traceDBTextBlock, bool) {
	if !strings.HasPrefix(line, TraceDBTextBlockPrefix+" ") {
		return traceDBTextBlock{}, false
	}
	rest := line[len(TraceDBTextBlockPrefix)+1:]
	blockRaw, rest, blockOK := cutTraceDBTextRecordField(rest, "block=", false)
	recordsRaw, rest, recordsOK := cutTraceDBTextRecordField(rest, "records=", false)
	rawBytesRaw, rest, rawBytesOK := cutTraceDBTextRecordField(rest, "raw_bytes=", false)
	timestampRaw, rest, timestampOK := cutTraceDBTextRecordField(rest, "ts_ns=", false)
	codec, rest, codecOK := cutTraceDBTextRecordField(rest, "codec=", false)
	payloadRaw, rest, payloadOK := cutTraceDBTextRecordField(rest, "payload=", false)
	payloadHashRaw, rest, payloadHashOK := cutTraceDBTextRecordField(rest, "payload_sha256=", false)
	rawHashRaw, rest, rawHashOK := cutTraceDBTextRecordField(rest, "raw_sha256=", true)
	if !blockOK || !recordsOK || !rawBytesOK || !timestampOK || !codecOK ||
		!payloadOK || !payloadHashOK || !rawHashOK || rest != "" || codec != "deflate" {
		return traceDBTextBlock{}, false
	}
	block, ok := parseCanonicalPositiveTraceDBTextRecordInt(blockRaw)
	if !ok {
		return traceDBTextBlock{}, false
	}
	recordCount, ok := parseCanonicalPositiveTraceDBTextRecordInt(recordsRaw)
	if !ok || recordCount > maxTraceDBTextBlockRecords {
		return traceDBTextBlock{}, false
	}
	rawBytes, ok := parseCanonicalPositiveTraceDBTextRecordInt(rawBytesRaw)
	if !ok || rawBytes > maxTraceDBTextBlockRawBytes {
		return traceDBTextBlock{}, false
	}
	timestampNS, err := strconv.ParseUint(timestampRaw, 10, 64)
	if err != nil || strconv.FormatUint(timestampNS, 10) != timestampRaw {
		return traceDBTextBlock{}, false
	}
	payload, err := strictTraceDBTextRecordBase64.DecodeString(payloadRaw)
	if err != nil || len(payload) == 0 || len(payload) > maxTraceDBTextBlockRawBytes+1024 {
		return traceDBTextBlock{}, false
	}
	payloadDigest := sha256.Sum256(payload)
	expectedPayloadDigest, payloadHashOK := decodeCanonicalTraceDBTextRecordSHA256(payloadHashRaw)
	expectedRawDigest, rawHashOK := decodeCanonicalTraceDBTextRecordSHA256(rawHashRaw)
	if !payloadHashOK || !rawHashOK || payloadDigest != expectedPayloadDigest {
		return traceDBTextBlock{}, false
	}
	compressedReader := bytes.NewReader(payload)
	reader, inflaterOK := acquireTraceDBTextBlockInflater(compressedReader)
	if !inflaterOK {
		return traceDBTextBlock{}, false
	}
	raw := make([]byte, rawBytes)
	_, readErr := io.ReadFull(reader, raw)
	var extra [1]byte
	extraBytes, extraErr := reader.Read(extra[:])
	closeErr := reader.Close()
	if closeErr == nil {
		traceDBTextBlockInflaterPool.Put(reader)
	}
	if readErr != nil || extraBytes != 0 || extraErr != io.EOF || closeErr != nil ||
		len(raw) != rawBytes || len(raw) > maxTraceDBTextBlockRawBytes || len(raw) == 0 ||
		raw[len(raw)-1] != '\n' || bytes.IndexByte(raw, '\r') >= 0 ||
		compressedReader.Len() != 0 {
		return traceDBTextBlock{}, false
	}
	rawDigest := sha256.Sum256(raw)
	if rawDigest != expectedRawDigest {
		return traceDBTextBlock{}, false
	}
	rawText := string(raw[:len(raw)-1])
	lines := strings.Split(rawText, "\n")
	if len(lines) != recordCount {
		return traceDBTextBlock{}, false
	}
	records := make([]TraceDBRecordFields, 0, recordCount)
	for _, recordLine := range lines {
		record, recordOK := parseTraceDBTextRecord(recordLine)
		if !recordOK || record.TimestampNS != timestampNS {
			return traceDBTextBlock{}, false
		}
		records = append(records, TraceDBRecordFields{
			Kind:         record.Kind,
			TableID:      record.TableID,
			RowOrdinal:   record.RowOrdinal,
			Chunk:        record.Chunk,
			Chunks:       record.Chunks,
			TimestampNS:  record.TimestampNS,
			PayloadBytes: record.PayloadBytes,
			RecordSHA256: record.RecordSHA256,
			Payload:      record.Payload,
		})
	}
	return traceDBTextBlock{
		Block:        block,
		RecordCount:  recordCount,
		RawBytes:     rawBytes,
		TimestampNS:  timestampNS,
		PayloadBytes: len(payload),
		PayloadSHA:   payloadHashRaw,
		RawSHA:       rawHashRaw,
		Records:      records,
	}, true
}

func acquireTraceDBTextBlockInflater(source io.Reader) (io.ReadCloser, bool) {
	if source == nil {
		return nil, false
	}
	if pooled := traceDBTextBlockInflaterPool.Get(); pooled != nil {
		reader, ok := pooled.(io.ReadCloser)
		resetter, resetOK := pooled.(flate.Resetter)
		if !ok || !resetOK || resetter.Reset(source, nil) != nil {
			return nil, false
		}
		return reader, true
	}
	return flate.NewReader(source), true
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
				Kind:         record.Kind,
				TableID:      record.TableID,
				RowOrdinal:   record.RowOrdinal,
				Chunk:        record.Chunk,
				Chunks:       record.Chunks,
				TimestampNS:  record.TimestampNS,
				PayloadBytes: record.PayloadBytes,
				RecordSHA256: record.RecordSHA256,
				Payload:      record.Payload,
			},
		},
		FieldText: fmt.Sprintf(
			"kind=%s table_id=%d row_ordinal=%d chunk=%d chunks=%d payload_bytes=%d record_sha256=%s",
			record.Kind, record.TableID, record.RowOrdinal, record.Chunk, record.Chunks,
			record.PayloadBytes, record.RecordSHA256,
		),
	}
}

func traceDBTextBlockEvent(lineNo int, block traceDBTextBlock, intern *stringInterner) Event {
	return Event{
		Line: lineNo,
		Ts:   float64(block.TimestampNS) / 1e9,
		CPU:  -1,
		Type: EventTraceDBRecord,
		Name: intern.intern("codrax_trace_db_block"),
		PluginFields: &PluginFields{
			Domain: "trace_db_fidelity",
			TraceDBBlock: &TraceDBBlockFields{
				Block:        block.Block,
				RecordCount:  block.RecordCount,
				RawBytes:     block.RawBytes,
				TimestampNS:  block.TimestampNS,
				PayloadBytes: block.PayloadBytes,
				PayloadSHA:   block.PayloadSHA,
				RawSHA:       block.RawSHA,
				Records:      block.Records,
			},
		},
		FieldText: fmt.Sprintf(
			"block=%d records=%d raw_bytes=%d payload_bytes=%d payload_sha256=%s raw_sha256=%s",
			block.Block, block.RecordCount, block.RawBytes, block.PayloadBytes,
			block.PayloadSHA, block.RawSHA,
		),
	}
}

func countTraceDBTextRecord(idx *Index, ev Event) bool {
	if idx == nil || ev.Type != EventTraceDBRecord || ev.PluginFields == nil {
		return false
	}
	var records []TraceDBRecordFields
	if ev.PluginFields.TraceDBRecord != nil && ev.PluginFields.TraceDBBlock == nil {
		records = []TraceDBRecordFields{*ev.PluginFields.TraceDBRecord}
	} else if ev.PluginFields.TraceDBBlock != nil && ev.PluginFields.TraceDBRecord == nil &&
		len(ev.PluginFields.TraceDBBlock.Records) == ev.PluginFields.TraceDBBlock.RecordCount {
		records = ev.PluginFields.TraceDBBlock.Records
	} else {
		return false
	}
	idx.TraceDBTextCarrierRows++
	idx.TraceDBTextRecords += len(records)
	for _, record := range records {
		switch record.Kind {
		case "schema":
			idx.TraceDBTextSchemaRecords++
		case "row":
			idx.TraceDBTextRowRecords++
		case "receipt":
			idx.TraceDBTextReceiptRecords++
		}
	}
	return true
}
