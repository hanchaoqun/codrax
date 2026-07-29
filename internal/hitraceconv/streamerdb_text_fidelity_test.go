package hitraceconv

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBTextFidelityWireKey struct {
	Kind       string
	TableID    int
	Ordinal    uint64
	RecordHash string
}

type traceDBTextFidelityWireRecord struct {
	Chunks [][]byte
}

func readTraceDBTextFidelityWire(t *testing.T, body string) map[traceDBTextFidelityWireKey]traceDBTextFidelityWireRecord {
	t.Helper()
	records := map[traceDBTextFidelityWireKey]traceDBTextFidelityWireRecord{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "# codrax_trace_db_record/v1 ") {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 11 {
			t.Fatalf("invalid typed fidelity wire: %q", line)
		}
		field := func(index int, prefix string) string {
			if !strings.HasPrefix(parts[index], prefix) {
				t.Fatalf("typed fidelity field %d does not start with %q: %q", index, prefix, line)
			}
			return strings.TrimPrefix(parts[index], prefix)
		}
		kind := field(2, "kind=")
		tableID, err := strconv.Atoi(field(3, "table_id="))
		if err != nil {
			t.Fatal(err)
		}
		ordinal, err := strconv.ParseUint(field(4, "row_ordinal="), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		chunk, err := strconv.Atoi(field(5, "chunk="))
		if err != nil {
			t.Fatal(err)
		}
		chunks, err := strconv.Atoi(field(6, "chunks="))
		if err != nil {
			t.Fatal(err)
		}
		payload, err := base64.RawURLEncoding.DecodeString(field(8, "payload="))
		if err != nil {
			t.Fatal(err)
		}
		chunkDigest := sha256.Sum256(payload)
		if hex.EncodeToString(chunkDigest[:]) != field(9, "chunk_sha256=") {
			t.Fatalf("typed fidelity chunk hash mismatch: %q", line)
		}
		key := traceDBTextFidelityWireKey{
			Kind: kind, TableID: tableID, Ordinal: ordinal,
			RecordHash: field(10, "record_sha256="),
		}
		record := records[key]
		if record.Chunks == nil {
			record.Chunks = make([][]byte, chunks)
		}
		if len(record.Chunks) != chunks || chunk <= 0 || chunk > chunks || record.Chunks[chunk-1] != nil {
			t.Fatalf("typed fidelity chunk geometry invalid: %q", line)
		}
		record.Chunks[chunk-1] = payload
		records[key] = record
	}
	return records
}

func traceDBTextFidelityWirePayload(t *testing.T, key traceDBTextFidelityWireKey, record traceDBTextFidelityWireRecord) []byte {
	t.Helper()
	var payload []byte
	for index, chunk := range record.Chunks {
		if chunk == nil {
			t.Fatalf("typed fidelity record missing chunk %d: %+v", index+1, key)
		}
		payload = append(payload, chunk...)
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != key.RecordHash {
		t.Fatalf("typed fidelity record hash mismatch: %+v", key)
	}
	return payload
}

func traceDBTextFidelityDecodedBytes(t *testing.T, cell traceDBTextFidelityCell) []byte {
	t.Helper()
	value, err := base64.RawURLEncoding.DecodeString(cell.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTraceDBTextFidelityPreservesEveryTableAndSQLiteStorageClass(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 1000000, 1000000, 2, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 1000, 1, 'one-span', '', NULL, NULL)",
		"CREATE TABLE exact_cells (k TEXT PRIMARY KEY, n, i, r, t TEXT, b BLOB) WITHOUT ROWID",
		"INSERT INTO exact_cells VALUES ('a', NULL, -9223372036854775807, 0.1, CAST(X'410042FF' AS TEXT), X'00FF10')",
		"INSERT INTO exact_cells VALUES ('b', 7, 42, -2.5, 'plain', zeroblob(70000))",
		"CREATE TABLE shadow_rowid (rowid TEXT PRIMARY KEY, note TEXT DEFAULT 'WITHOUT ROWID')",
		"INSERT INTO shadow_rowid VALUES ('logical-rowid', 'kept')",
	})
	output := filepath.Join(t.TempDir(), "text-fidelity.systrace")
	result, err := exportTraceDBToSystrace(t.Context(), path, output)
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	records := readTraceDBTextFidelityWire(t, body)
	if len(records) == 0 {
		t.Fatal("no typed fidelity records were emitted")
	}

	tableNames := map[int]string{}
	tableSchemas := map[int]traceDBTextFidelitySchema{}
	rowsByTable := map[int][]traceDBTextFidelityRow{}
	receipts := map[int]traceDBTextFidelityReceipt{}
	physicalTypedLines := strings.Count(body, "# codrax_trace_db_record/v1 ")
	firstTyped := strings.Index(body, "# codrax_trace_db_record/v1 ")
	if firstTyped < 0 {
		t.Fatal("typed fidelity tail is absent")
	}
	for _, line := range strings.Split(body[firstTyped:], "\n") {
		if line != "" && !strings.HasPrefix(line, "# codrax_trace_db_record/v1 ") {
			t.Fatalf("semantic row appeared after authenticated fidelity tail began: %q", line)
		}
	}
	for key, record := range records {
		payload := traceDBTextFidelityWirePayload(t, key, record)
		switch key.Kind {
		case "schema":
			var schema traceDBTextFidelitySchema
			if err := json.Unmarshal(payload, &schema); err != nil {
				t.Fatal(err)
			}
			tableNames[schema.TableID] = string(traceDBTextFidelityDecodedBytes(t, schema.Table))
			tableSchemas[schema.TableID] = schema
		case "row":
			var row traceDBTextFidelityRow
			if err := json.Unmarshal(payload, &row); err != nil {
				t.Fatal(err)
			}
			rowsByTable[row.TableID] = append(rowsByTable[row.TableID], row)
		case "receipt":
			var receipt traceDBTextFidelityReceipt
			if err := json.Unmarshal(payload, &receipt); err != nil {
				t.Fatal(err)
			}
			receipts[receipt.TableID] = receipt
		default:
			t.Fatalf("unknown typed fidelity kind %q", key.Kind)
		}
	}
	if len(tableSchemas) == 0 || len(tableSchemas) != len(receipts) {
		t.Fatalf("schema/receipt table conservation failed: schemas=%d receipts=%d", len(tableSchemas), len(receipts))
	}
	exactTableID := 0
	shadowTableID := 0
	for tableID, name := range tableNames {
		if name == "exact_cells" {
			exactTableID = tableID
		}
		if name == "shadow_rowid" {
			shadowTableID = tableID
		}
		if receipts[tableID].Rows != uint64(len(rowsByTable[tableID])) {
			t.Fatalf("row conservation failed for table %q: receipt=%+v rows=%d", name, receipts[tableID], len(rowsByTable[tableID]))
		}
	}
	if exactTableID == 0 || tableSchemas[exactTableID].RowID != "without_rowid_primary_key" {
		t.Fatalf("WITHOUT ROWID schema missing: id=%d schema=%+v", exactTableID, tableSchemas[exactTableID])
	}
	if shadowTableID == 0 || tableSchemas[shadowTableID].RowID != "rowid_alias:_rowid_" ||
		len(rowsByTable[shadowTableID]) != 1 || rowsByTable[shadowTableID][0].RowID == nil ||
		rowsByTable[shadowTableID][0].RowID.Storage != "integer" {
		t.Fatalf("shadowed rowid alias was not preserved independently: id=%d schema=%+v rows=%+v",
			shadowTableID, tableSchemas[shadowTableID], rowsByTable[shadowTableID])
	}
	exactRows := rowsByTable[exactTableID]
	sort.Slice(exactRows, func(i, j int) bool { return exactRows[i].Ordinal < exactRows[j].Ordinal })
	if len(exactRows) != 2 {
		t.Fatalf("exact_cells rows=%d, want 2", len(exactRows))
	}
	first := exactRows[0].Cells
	if len(first) != 6 || first[1].Storage != "null" ||
		first[2].Storage != "integer" || first[2].Integer != "-9223372036854775807" ||
		first[3].Storage != "real" || first[3].RealHex != "3fb999999999999a" ||
		first[4].Storage != "text" || string(traceDBTextFidelityDecodedBytes(t, first[4])) != string([]byte{'A', 0, 'B', 0xff}) ||
		first[5].Storage != "blob" || string(traceDBTextFidelityDecodedBytes(t, first[5])) != string([]byte{0, 0xff, 0x10}) {
		t.Fatalf("exact SQLite cell preservation failed: storages=%v integer=%q real=%q text=%q blob=%x",
			[]string{first[0].Storage, first[1].Storage, first[2].Storage, first[3].Storage, first[4].Storage, first[5].Storage},
			first[2].Integer, first[3].RealHex, traceDBTextFidelityDecodedBytes(t, first[4]),
			traceDBTextFidelityDecodedBytes(t, first[5]))
	}
	second := exactRows[1].Cells
	if second[3].RealHex != "c004000000000000" ||
		len(traceDBTextFidelityDecodedBytes(t, second[5])) != 70000 {
		t.Fatalf("large/REAL exact cell preservation failed: real=%q blob_bytes=%d",
			second[3].RealHex, len(traceDBTextFidelityDecodedBytes(t, second[5])))
	}

	if result.Artifact.Trace == nil ||
		result.Artifact.Trace.AdvisoryRows != physicalTypedLines ||
		result.Artifact.Trace.Known != result.EventsWritten ||
		result.Artifact.Trace.AuthoritativeKnown != result.EventsWritten-physicalTypedLines ||
		!result.Artifact.Trace.TraceQueryReady {
		t.Fatalf("typed preservation authority accounting failed: result=%+v lines=%d", result.Artifact.Trace, physicalTypedLines)
	}
	index, err := tracequery.BuildIndex(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	if index.TraceDBTextRecords != physicalTypedLines ||
		index.TraceDBTextSchemaRecords != len(tableSchemas) ||
		index.TraceDBTextReceiptRecords != len(receipts) ||
		len(index.Events) != result.Artifact.Trace.AuthoritativeKnown {
		t.Fatalf("typed preservation query-index isolation failed: index=%+v capability=%+v", index, result.Artifact.Trace)
	}
	var sorter *TraceDBCoverage
	var fidelitySummary *TraceDBCoverage
	for index := range result.Coverage {
		item := &result.Coverage[index]
		if item.Family == "sorter" && item.Table == "__systrace_rows__" {
			sorter = item
		}
		if item.Family == traceDBTextFidelityFamily && item.Table == traceDBTextFidelitySummaryTable {
			fidelitySummary = item
		}
	}
	if sorter == nil || sorter.Metrics["authenticated_tail_rows"] != int64(physicalTypedLines) ||
		sorter.Metrics["semantic_rows_sorted"] != int64(result.EventsWritten-physicalTypedLines) ||
		sorter.Metrics["authenticated_tail_bytes"] <= 0 ||
		sorter.RowsRead != result.EventsWritten || sorter.RowsEmitted != result.EventsWritten {
		t.Fatalf("authenticated-tail sorter accounting failed: sorter=%+v result=%+v typed=%d",
			sorter, result, physicalTypedLines)
	}
	if fidelitySummary == nil || fidelitySummary.ElapsedUS <= 0 {
		t.Fatalf("text-fidelity elapsed timing missing: %+v", fidelitySummary)
	}
}

func TestTraceDBTextFidelityTailTamperFailsBeforeBufferedPublication(t *testing.T) {
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.cleanup() })
	if err := sink.add(renderedRow{tsNS: 1, line: "semantic-row"}); err != nil {
		t.Fatal(err)
	}
	if _, err := emitTraceDBTextFidelityRecord(
		t.Context(), sink, 1, "schema", 1, 0, []byte(`{"version":1}`),
	); err != nil {
		t.Fatal(err)
	}
	if err := sink.prepareForPublication(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sink.textFidelityTail == nil || !sink.textFidelityTail.sealed {
		t.Fatalf("tail was not sealed: %+v", sink.textFidelityTail)
	}
	file, err := os.OpenFile(sink.textFidelityTail.path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{'X'}, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := sink.writeTo(t.Context(), &output); err == nil {
		t.Fatal("tampered authenticated tail unexpectedly published")
	} else if reason, ok := traceDBOutputInvariantReason(err); !ok ||
		reason != "trace_db_text_fidelity_tail_integrity_mismatch" {
		t.Fatalf("tamper error=%v reason=%q ok=%v", err, reason, ok)
	}
	if output.Len() != 0 {
		t.Fatalf("tampered tail escaped buffered publication: bytes=%d", output.Len())
	}
}
