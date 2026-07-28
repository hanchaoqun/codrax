package tracequery

import (
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
	cases := []string{
		strings.Replace(valid, "kind=schema", "kind=other", 1),
		strings.Replace(valid, "row_ordinal=0", "row_ordinal=1", 1),
		strings.Replace(valid, "chunk=1", "chunk=0", 1),
		strings.Replace(valid, "table_id=7", "table_id=07", 1),
		strings.Replace(valid, "payload=", "payload=*", 1),
		strings.Replace(valid, "chunk_sha256=", "chunk_sha256=0", 1),
		strings.Replace(valid, "record_sha256=", "record_sha256=A", 1),
		valid + " trailing=true",
	}
	for _, line := range cases {
		if event, ok := ParseLine(1, line, nil); ok {
			t.Fatalf("corrupt typed record parsed: %q => %+v", line, event)
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
		index.TraceDBTextRecords != 3 || index.TraceDBTextSchemaRecords != 1 ||
		index.TraceDBTextRowRecords != 1 || index.TraceDBTextReceiptRecords != 1 {
		t.Fatalf("typed preservation accounting mismatch: %+v", index)
	}
}
