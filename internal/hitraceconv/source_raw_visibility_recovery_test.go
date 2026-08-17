package hitraceconv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBRawVisibilityCapture(t *testing.T, corruptEnvelope bool) ([]byte, []byte) {
	t.Helper()
	format := eventFormat{
		ID: 33086, Name: "hmfs_writepage",
		Fields: []eventField{
			{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "unsigned long", Name: "ino", Offset: 8, Size: 8},
			{Type: "unsigned long", Name: "index", Offset: 16, Size: 8},
		},
		PrintFmt: `"ino=%lu index=%lu", REC->ino, REC->index`,
	}
	if corruptEnvelope {
		format.Fields[3].Offset = 12
	}
	content := make([]byte, 24)
	binary.LittleEndian.PutUint16(content[0:2], uint16(format.ID))
	content[2] = 0x04
	content[3] = 0x02
	binary.LittleEndian.PutUint32(content[4:8], 25827)
	binary.LittleEndian.PutUint64(content[8:16], 0x1234)
	binary.LittleEndian.PutUint64(content[16:24], 77)

	var capture bytes.Buffer
	writeFileHeader(&capture, 4)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	writeSegment(&capture, segmentCmdlines, []byte("25827 com.tencent.mm\n"))
	writeSegment(&capture, segmentEventsFormat, []byte(directPairFormatBlock(format.ID, format)))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: uint16(format.ID), OffsetNS: 1000, Content: content},
		{EventID: uint16(format.ID), OffsetNS: 2000, Content: content},
	}))
	return capture.Bytes(), content
}

func TestTraceDBSourceRawVisibilityPublishesLosslessNonAuthoritativeRows(t *testing.T) {
	capture, originalContent := traceDBRawVisibilityCapture(t, false)
	path := filepath.Join(t.TempDir(), "visibility.sys")
	if err := os.WriteFile(path, capture, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	if !traceDBSourceRawVisibilityEligible(&inventory) ||
		inventory.RawDecode.Metrics["visibility_candidate_records"] != 2 ||
		inventory.RawDecode.Metrics["visibility_envelope_admitted"] != 2 {
		t.Fatalf("visibility preflight did not close: %+v", inventory.RawDecode)
	}
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBSourceRawVisibility(
		context.Background(), &inventory, sink)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.RowsRead != 2 || coverage.RowsEmitted != 2 ||
		coverage.Metadata["publication_state"] != "published_complete_visibility_only_source_census" ||
		coverage.Metrics["semantic_authority_rows"] != 0 || len(sink.rows) != 2 {
		t.Fatalf("visibility publication mismatch: coverage=%+v rows=%+v", coverage, sink.rows)
	}
	for index, row := range sink.rows {
		if !strings.Contains(row.line, "hmfs_writepage: "+traceDBSourceRawVisibilityWire+" ") ||
			!strings.Contains(row.line, "semantic_authority=none") ||
			!strings.Contains(row.line, "event_name_b64=aG1mc193cml0ZXBhZ2U") {
			t.Fatalf("visibility row lost its exact typed wire: %s", row.line)
		}
		payload := traceDBRawVisibilityToken(row.line, "payload_b64")
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(payload)
		if decodeErr != nil || !bytes.Equal(decoded, originalContent) {
			t.Fatalf("visibility payload did not round-trip: payload=%q err=%v", payload, decodeErr)
		}
		event, ok := tracequery.ParseLine(index+1, row.line, nil)
		if !ok || event.Type != tracequery.EventSourceRawVisibility || event.Name != "hmfs_writepage" ||
			event.SubsystemKind != "" {
			t.Fatalf("visibility row gained filesystem/root authority: %+v ok=%t", event, ok)
		}
	}
	schemaRaw := traceDBRawVisibilityToken(sink.rows[0].line, "schema_b64")
	schemaPayload, err := base64.RawURLEncoding.DecodeString(schemaRaw)
	if err != nil || len(schemaPayload) == 0 {
		t.Fatalf("first visibility row lost schema: %q err=%v", schemaRaw, err)
	}
	var schema traceDBSourceRawVisibilitySchema
	if err := json.Unmarshal(schemaPayload, &schema); err != nil ||
		schema.Name != "hmfs_writepage" || schema.ID != 33086 || len(schema.Fields) != 6 ||
		schema.PrintFmt == "" {
		t.Fatalf("visibility schema did not round-trip: %+v err=%v", schema, err)
	}
	if strings.Contains(sink.rows[1].line, " schema_b64=") {
		t.Fatalf("schema was redundantly copied onto every event: %s", sink.rows[1].line)
	}
}

func TestTraceDBSourceRawVisibilityWithholdsBeforeSinkOnEnvelopeGap(t *testing.T) {
	capture, _ := traceDBRawVisibilityCapture(t, true)
	path := filepath.Join(t.TempDir(), "visibility-bad-envelope.sys")
	if err := os.WriteFile(path, capture, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	if traceDBSourceRawVisibilityEligible(&inventory) ||
		inventory.RawDecode.Metrics["visibility_envelope_rejected"] != 2 {
		t.Fatalf("invalid generic envelope gained visibility authority: %+v", inventory.RawDecode)
	}
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBSourceRawVisibility(
		context.Background(), &inventory, sink)
	if err != nil || coverage.RowsEmitted != 0 || sink.stats.RowsAccepted != 0 ||
		coverage.Metadata["publication_state"] != "withheld_visibility_envelope_incomplete" {
		t.Fatalf("incomplete visibility family leaked rows: coverage=%+v stats=%+v err=%v",
			coverage, sink.stats, err)
	}
}

func TestTraceDBSourceRawVisibilitySpillsWithoutChangingAuthority(t *testing.T) {
	capture, _ := traceDBRawVisibilityCapture(t, false)
	path := filepath.Join(t.TempDir(), "visibility-spill.sys")
	if err := os.WriteFile(path, capture, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBSourceRawVisibility(
		context.Background(), &inventory, sink)
	if err != nil || coverage.RowsEmitted != 2 || sink.stats.RowsAccepted != 2 ||
		len(sink.runs) == 0 {
		t.Fatalf("visibility spill path diverged: coverage=%+v stats=%+v runs=%d err=%v",
			coverage, sink.stats, len(sink.runs), err)
	}
}

func TestTraceStreamerConversionPublishesSourceRawVisibility(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	capture, _ := traceDBRawVisibilityCapture(t, false)
	dir := t.TempDir()
	input := filepath.Join(dir, "visibility.sys")
	output := filepath.Join(dir, "visibility.systrace")
	if err := os.WriteFile(input, capture, 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output,
		TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "hmfs_writepage: "+traceDBSourceRawVisibilityWire) != 2 {
		t.Fatalf("end-to-end visibility rows missing or duplicated:\n%s", body)
	}
	found := false
	for _, coverage := range result.TraceDBCoverage {
		if coverage.Family == "source_rawtrace_visibility" {
			found = coverage.RowsRead == 2 && coverage.RowsEmitted == 2 &&
				coverage.Metadata["publication_state"] == "published_complete_visibility_only_source_census"
		}
	}
	if !found {
		t.Fatalf("end-to-end visibility coverage missing: %+v", result.TraceDBCoverage)
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	visibilityRows := 0
	for _, event := range index.Events {
		if event.Name == "hmfs_writepage" {
			visibilityRows++
			if event.Type != tracequery.EventSourceRawVisibility || event.SubsystemKind != "" {
				t.Fatalf("end-to-end visibility row gained semantic authority: %+v", event)
			}
		}
	}
	if visibilityRows != 0 {
		t.Fatalf("ordinary tracequery index retained visibility-only carrier rows: %d", visibilityRows)
	}
}

func traceDBRawVisibilityToken(line, key string) string {
	prefix := key + "="
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimPrefix(field, prefix)
		}
	}
	return ""
}
