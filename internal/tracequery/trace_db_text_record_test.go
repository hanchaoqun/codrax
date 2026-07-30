package tracequery

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testTraceDBTextRecordLine(kind string, ordinal uint64, payload []byte) string {
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	return fmt.Sprintf(
		"# codrax_trace_db_record/v1 kind=%s table_id=7 row_ordinal=%d chunk=1 chunks=1 ts_ns=1234567890 payload=%s chunk_sha256=%s record_sha256=%s",
		kind, ordinal, base64.RawURLEncoding.EncodeToString(payload), hash, hash,
	)
}

func testTraceDBTextBlockLine(t testing.TB, block int, lines []string) string {
	t.Helper()
	raw := []byte(strings.Join(lines, "\n") + "\n")
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := compressed.Bytes()
	payloadDigest := sha256.Sum256(payload)
	rawDigest := sha256.Sum256(raw)
	return fmt.Sprintf(
		"# codrax_trace_db_block/v2 block=%d records=%d raw_bytes=%d ts_ns=1234567890 codec=deflate payload=%s payload_sha256=%s raw_sha256=%s",
		block, len(lines), len(raw), base64.RawURLEncoding.EncodeToString(payload),
		hex.EncodeToString(payloadDigest[:]), hex.EncodeToString(rawDigest[:]),
	)
}

func TestTraceDBTextRecordParsesAsKnownPreservationOnlyEvent(t *testing.T) {
	line := testTraceDBTextRecordLine("row", 9, []byte(`{"storage":"blob","bytes_b64":"AP8"}`))
	event, ok := ParseLine(17, line, nil)
	if !ok || event.Type != EventTraceDBRecord || event.CPU != -1 ||
		event.PluginFields == nil || event.PluginFields.TraceDBRecord == nil {
		t.Fatalf("typed trace DB record was not parsed: ok=%t event=%+v", ok, event)
	}
	record := event.PluginFields.TraceDBRecord
	if record.Kind != "row" || record.TableID != 7 || record.RowOrdinal != 9 ||
		record.Chunk != 1 || record.Chunks != 1 || record.TimestampNS != 1234567890 {
		t.Fatalf("typed trace DB envelope mismatch: %+v", record)
	}
	if timestamp, ok := ParseLineTimestampNS(line); !ok || timestamp != 1234567890 {
		t.Fatalf("typed trace DB timestamp mismatch: (%d,%t)", timestamp, ok)
	}
}

func TestTraceDBTextRecordRejectsCorruptionAndNoncanonicalWire(t *testing.T) {
	valid := testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`))
	noncanonicalBase64 := testTraceDBTextRecordLine("schema", 0, []byte{0})
	noncanonicalBase64 = strings.Replace(noncanonicalBase64, "payload=AA ", "payload=AB ", 1)
	cases := []string{
		strings.Replace(valid, "kind=schema", "kind=other", 1),
		strings.Replace(valid, "row_ordinal=0", "row_ordinal=1", 1),
		strings.Replace(valid, "chunk=1", "chunk=0", 1),
		strings.Replace(valid, "table_id=7", "table_id=07", 1),
		strings.Replace(valid, "payload=", "payload=*", 1),
		strings.Replace(valid, "chunk_sha256=", "chunk_sha256=0", 1),
		strings.Replace(valid, "record_sha256=", "record_sha256=A", 1),
		strings.Replace(valid, " kind=", "  kind=", 1),
		noncanonicalBase64,
		valid + " trailing=true",
	}
	for _, line := range cases {
		if event, ok := ParseLine(1, line, nil); ok {
			t.Fatalf("corrupt typed record parsed: %q => %+v", line, event)
		}
	}
}

func TestTraceDBTextBlockV2ParsesAndCountsRecoveredCanonicalRecords(t *testing.T) {
	lines := []string{
		testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)),
		testTraceDBTextRecordLine("row", 1, []byte(`{"ordinal":1}`)),
		testTraceDBTextRecordLine("receipt", 0, []byte(`{"rows":1}`)),
	}
	line := testTraceDBTextBlockLine(t, 1, lines)
	event, ok := ParseLine(19, line, nil)
	if !ok || event.Type != EventTraceDBRecord || event.PluginFields == nil ||
		event.PluginFields.TraceDBRecord != nil || event.PluginFields.TraceDBBlock == nil {
		t.Fatalf("compact trace DB block was not parsed: ok=%t event=%+v", ok, event)
	}
	block := event.PluginFields.TraceDBBlock
	if block.Block != 1 || block.RecordCount != 3 || len(block.Records) != 3 ||
		block.TimestampNS != 1234567890 || block.RawBytes <= 0 || block.PayloadBytes <= 0 {
		t.Fatalf("compact trace DB envelope mismatch: %+v", block)
	}
	if timestamp, ok := ParseLineTimestampNS(line); !ok || timestamp != 1234567890 {
		t.Fatalf("compact trace DB timestamp mismatch: (%d,%t)", timestamp, ok)
	}

	path := filepath.Join(t.TempDir(), "typed-v2.systrace")
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{MaxEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Events) != 0 || index.ParsedKnown != 1 ||
		index.TraceDBTextCarrierRows != 1 || index.TraceDBTextRecords != 3 ||
		index.TraceDBTextSchemaRecords != 1 || index.TraceDBTextRowRecords != 1 ||
		index.TraceDBTextReceiptRecords != 1 {
		t.Fatalf("compact preservation accounting mismatch: %+v", index)
	}
}

func TestTraceDBTextBlockV2RejectsCorruptionAndBoundedExpansion(t *testing.T) {
	valid := testTraceDBTextBlockLine(t, 1, []string{
		testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)),
	})
	trailingParts := strings.Split(valid, " ")
	trailingPayload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(trailingParts[7], "payload="))
	if err != nil {
		t.Fatal(err)
	}
	trailingPayload = append(trailingPayload, 0)
	trailingDigest := sha256.Sum256(trailingPayload)
	trailingParts[7] = "payload=" + base64.RawURLEncoding.EncodeToString(trailingPayload)
	trailingParts[8] = "payload_sha256=" + hex.EncodeToString(trailingDigest[:])
	validCompressedStreamWithTrailingByte := strings.Join(trailingParts, " ")
	cases := []string{
		strings.Replace(valid, "block=1", "block=01", 1),
		strings.Replace(valid, "records=1", "records=2", 1),
		strings.Replace(valid, "codec=deflate", "codec=gzip", 1),
		strings.Replace(valid, "payload=", "payload=*", 1),
		strings.Replace(valid, "payload_sha256=", "payload_sha256=0", 1),
		strings.Replace(valid, "raw_sha256=", "raw_sha256=A", 1),
		valid + " trailing=true",
		validCompressedStreamWithTrailingByte,
	}
	for _, line := range cases {
		if event, ok := ParseLine(1, line, nil); ok {
			t.Fatalf("corrupt compact block parsed: %q => %+v", line, event)
		}
	}

	bombRaw := bytes.Repeat([]byte{'x'}, maxTraceDBTextBlockRawBytes+1)
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(bombRaw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := compressed.Bytes()
	payloadDigest := sha256.Sum256(payload)
	rawDigest := sha256.Sum256(bombRaw)
	bomb := fmt.Sprintf(
		"# codrax_trace_db_block/v2 block=1 records=1 raw_bytes=%d ts_ns=1234567890 codec=deflate payload=%s payload_sha256=%s raw_sha256=%s",
		maxTraceDBTextBlockRawBytes, base64.RawURLEncoding.EncodeToString(payload),
		hex.EncodeToString(payloadDigest[:]), hex.EncodeToString(rawDigest[:]),
	)
	if event, ok := ParseLine(1, bomb, nil); ok {
		t.Fatalf("oversized compact block parsed: %+v", event)
	}
}

func BenchmarkParseTraceDBTextRecord(b *testing.B) {
	line := testTraceDBTextRecordLine(
		"row",
		9,
		[]byte(`{"version":1,"table_id":7,"ordinal":9,"cells":[{"storage":"text","bytes_b64":"Y3JpdGljYWwtc3Bhbi1uYW1l"}]}`),
	)
	b.ReportAllocs()
	b.SetBytes(int64(len(line)))
	for range b.N {
		if _, ok := parseTraceDBTextRecord(line); !ok {
			b.Fatal("valid record rejected")
		}
	}
}

func BenchmarkParseTraceDBTextBlock(b *testing.B) {
	recordLines := make([]string, 100)
	for index := range recordLines {
		recordLines[index] = testTraceDBTextRecordLine(
			"row",
			uint64(index)+1,
			[]byte(fmt.Sprintf(`{"version":1,"table_id":7,"ordinal":%d,"cells":[{"storage":"text","bytes_b64":"Y3JpdGljYWwtc3Bhbi1uYW1l"}]}`, index+1)),
		)
	}
	line := testTraceDBTextBlockLine(b, 1, recordLines)
	b.ReportAllocs()
	b.SetBytes(int64(len(line)))
	for range b.N {
		if _, ok := parseTraceDBTextBlock(line); !ok {
			b.Fatal("valid compact block rejected")
		}
	}
}

func TestBuildIndexCountsButDoesNotRetainTraceDBTextRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typed.systrace")
	lines := []string{
		testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)),
		testTraceDBTextRecordLine("row", 1, []byte(`{"ordinal":1}`)),
		testTraceDBTextRecordLine("receipt", 0, []byte(`{"rows":1}`)),
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{MaxEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Events) != 0 || index.ParsedKnown != 3 ||
		index.TraceDBTextCarrierRows != 3 || index.TraceDBTextRecords != 3 ||
		index.TraceDBTextSchemaRecords != 1 ||
		index.TraceDBTextRowRecords != 1 || index.TraceDBTextReceiptRecords != 1 ||
		index.RetainedStringBytes > 128 {
		t.Fatalf("typed preservation accounting mismatch: %+v", index)
	}
}
