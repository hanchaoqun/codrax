package tool

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func logQueryPublicBus(t *testing.T, inputs []loginput.Input) (*types.BusContext, *Registry) {
	t.Helper()
	catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(8), Mutable: types.NewMutableState("Read attached logs")}
	registry := NewRegistry()
	registry.Register(&LogQuery{})
	return bus, registry
}

func TestLogQueryPublicRegistryLedgerExactNativeRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hilog.gz")
	data := "10-09 01:02:03.123 0 0 I Camera: start\n  continuation\n10-09 01:02:03.124 <6> [9007199.254740993] -;[2] pid=10 tid=11 comm=worker finished\nunknown\n"
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, compressed.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Path: path}})
	result, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !result.Success || result.RawRef == "" || !json.Valid([]byte(result.Summary)) {
		t.Fatalf("query failed: %+v %v", result, err)
	}
	if registry.IsWrite("log_query") {
		t.Fatal("log query gained write authority")
	}
	raw, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload logQueryPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Result.Matched != 3 || payload.Result.Records[0].PID == nil || *payload.Result.Records[0].PID != 0 || payload.Result.Records[2].PID != nil || payload.Result.Records[1].BootTimestampNS != "9007199254740993" {
		t.Fatalf("known/unknown native values changed: %+v", payload)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	if len(ledger.Records) != 4 || !ledger.HasDirectRuntimeObservation() {
		t.Fatalf("native records missing: %+v", ledger.Records)
	}
	for _, row := range ledger.Records {
		if row.Origin != types.AnswerEvidenceOriginRuntimeArtifact || row.SourceRef.Kind != types.ObservationSourceRuntimeArtifact || row.ClaimAuthority != types.ObservationClaimAuthorityDirectObservation || row.ProvenanceLane != types.ObservationProvenanceArtifactSpan || row.SourceRef.PayloadRef != result.RawRef || row.SourceRef.ClockCalibrated || row.SourceRef.CanonicalTimeDomain != "" {
			t.Fatalf("authority crossed boundary: %+v", row)
		}
		if row.Predicate == "log_record" {
			if row.SourceRef.Path != path || row.Span.JSONPointer == "" || row.Span.LineStart <= 0 {
				t.Fatalf("original source locator lost: %+v", row)
			}
		}
	}
	for _, projection := range types.CompileTraceCausalProjectionSet(ledger).Projections {
		if projection.PrimaryRootCause != nil || len(projection.OnChainCauses) != 0 || len(projection.WakeupPath) != 0 || projection.WindowStartTs != 0 || projection.WindowEndTs != 0 {
			t.Fatalf("logs minted trace causal projection: %+v", projection)
		}
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, compressed.Bytes()) {
		t.Fatal("query altered attached input")
	}
}

func TestLogQueryPublicPaginationEmptyMalformedAndExactFilters(t *testing.T) {
	data := "10-09 01:02:03.123 1 2 I Tag: first\n13-09 01:02:03.123 9 8 E Tag: malformed\n  orphan\nlast\n"
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "memory", Data: []byte(data)}})
	result, err := registry.Execute(bus, "log_query", json.RawMessage(`{"limit":1}`))
	if err != nil || !result.Success {
		t.Fatalf("first page %+v %v", result, err)
	}
	var summary struct {
		Matched  int64          `json:"matched"`
		Returned int            `json:"returned"`
		NextCall loginput.Query `json:"next_call"`
	}
	if err := json.Unmarshal([]byte(result.Summary), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Matched != 4 || summary.Returned != 1 || summary.NextCall.Offset != 1 || summary.NextCall.Limit != 1 {
		t.Fatalf("bad next call: %+v", summary)
	}
	raw, _ := json.Marshal(summary.NextCall)
	next, err := registry.Execute(bus, "log_query", raw)
	if err != nil || !next.Success || !strings.Contains(next.Summary, "malformed") {
		t.Fatalf("next page lost malformed record %+v %v", next, err)
	}
	empty, err := registry.Execute(bus, "log_query", json.RawMessage(`{"contains":"not here"}`))
	if err != nil || !empty.Success || len(empty.Observations) != 1 || empty.Observations[0].Value != "0" {
		t.Fatalf("empty query not distinguished: %+v %v", empty, err)
	}
	for _, input := range []string{`{"path":"/tmp/not-attached.log"}`, `{"pid":null}`, `{"limit":1,"limit":2}`, `{"unknown":true}`, `{} {}`, `{"kinds":["invented"]}`, `{"first_line":2,"last_line":1}`, `{"source_ids":["forged"]}`} {
		bad, err := registry.Execute(bus, "log_query", json.RawMessage(input))
		if bad.Success || len(bad.Observations) != 0 {
			t.Fatalf("invalid parameters granted evidence %s: %+v %v", input, bad, err)
		}
	}
}

func TestLogQueryPublicNilPreviewStalePartialAndWriteFailure(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&LogQuery{})
	for _, bus := range []*types.BusContext{nil, {AttachedLog: "preview-only"}} {
		out, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
		if err != nil || out.Success || len(out.Observations) != 0 {
			t.Fatalf("preview minted complete source: %+v %v", out, err)
		}
	}
	path := filepath.Join(t.TempDir(), "mutable.log")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Path: path}, {Name: "healthy", Data: []byte("healthy\n")}})
	sourceID := bus.AttachedLogCatalog.Sources()[0].ID
	if err := os.WriteFile(path, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !partial.Success || len(partial.Observations) != 2 || !strings.Contains(partial.Summary, "generation changed") {
		t.Fatalf("healthy source lost or stale row leaked: %+v %v", partial, err)
	}
	filter, _ := json.Marshal(map[string]any{"source_ids": []string{sourceID}})
	stale, err := registry.Execute(bus, "log_query", filter)
	if stale.Success || len(stale.Observations) != 0 {
		t.Fatalf("all-stale query claimed empty evidence: %+v %v", stale, err)
	}
	badDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badDir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	bus.WorkDir = badDir
	failed, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || failed.Success || failed.RawRef != "" || len(failed.Observations) != 0 {
		t.Fatalf("failed storage published locator: %+v %v", failed, err)
	}
}

func TestLogQueryPublicLongPageRetainedWithoutBrokenJSON(t *testing.T) {
	data := strings.Repeat(strings.Repeat("row", 400)+"\n", 60)
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "long", Data: []byte(data)}})
	result, err := registry.Execute(bus, "log_query", json.RawMessage(`{"limit":50}`))
	if err != nil || !result.Success || !json.Valid([]byte(result.Summary)) || len(result.Summary) > 64<<10 {
		t.Fatalf("unbounded/broken JSON summary %d %+v %v", len(result.Summary), result, err)
	}
	raw, err := os.ReadFile(result.RawRef)
	if err != nil || !json.Valid(raw) || len(raw) <= MaxInlineBytes {
		t.Fatalf("full payload missing: %d %v", len(raw), err)
	}
	var payload logQueryPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Result.Records) != 50 || len(payload.Result.Records[0].RawBytes) != 1201 {
		t.Fatal("full raw records were clipped")
	}
	var page struct {
		Records []json.RawMessage `json:"records_preview"`
		Next    loginput.Query    `json:"next_call"`
	}
	if err := json.Unmarshal([]byte(result.Summary), &page); err != nil || len(page.Records) != 50 || page.Next.Offset != 50 {
		t.Fatalf("pagination skipped unpublished records: previews=%d next=%+v %v", len(page.Records), page.Next, err)
	}
	bus.WorkDir = ""
	inline, err := registry.Execute(bus, "log_query", json.RawMessage(`{"limit":50}`))
	if err != nil || !inline.Success || inline.RawRef != "" || !json.Valid([]byte(inline.Summary)) || !strings.Contains(inline.Summary, "raw_base64") {
		t.Fatalf("no-workdir carrier lost complete JSON: %+v %v", inline, err)
	}
}

func TestLogQueryPublicRawBytesPaginationAndInvalidUTF8(t *testing.T) {
	data := append([]byte(strings.Repeat("原始消息", 1800)), 0xff, '\n')
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "raw", Data: data}})
	listed, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !listed.Success {
		t.Fatal(err, listed.Summary)
	}
	raw, _ := os.ReadFile(listed.RawRef)
	var payload logQueryPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	record := payload.Result.Records[0]
	if record.RawText != "" || record.Message != "" || record.Tag != "" || record.Comm != "" || record.ParseError != "invalid_utf8" || !bytes.Equal(record.RawBytes, data) {
		t.Fatalf("invalid bytes became fake Unicode text: %+v", record)
	}
	query, _ := json.Marshal(map[string]any{"record_ref": record.ID, "byte_limit": 1025})
	var got []byte
	for {
		out, err := registry.Execute(bus, "log_query", query)
		if err != nil || !out.Success || len(out.Observations) != 1 || !json.Valid([]byte(out.Summary)) {
			t.Fatal(err, out.Summary)
		}
		var chunk struct {
			Data   []byte          `json:"raw_base64"`
			Text   string          `json:"raw_text"`
			Valid  bool            `json:"chunk_utf8_valid"`
			Next   json.RawMessage `json:"next_call"`
			Offset int             `json:"byte_offset"`
		}
		if err := json.Unmarshal([]byte(out.Summary), &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Offset != len(got) || (!chunk.Valid && chunk.Text != "") {
			t.Fatalf("bad UTF8/byte cursor: %+v", chunk)
		}
		got = append(got, chunk.Data...)
		if len(chunk.Next) == 0 {
			break
		}
		query = chunk.Next
	}
	if !bytes.Equal(got, data) {
		t.Fatal("chunk paging lost original bytes")
	}
	for _, args := range []map[string]any{{"record_ref": record.ID, "limit": 1}, {"byte_offset": 0}, {"record_ref": record.ID, "byte_offset": len(data) + 1}, {"record_ref": record.ID + "x"}, {"record_ref": "/tmp/not-attached"}} {
		q, _ := json.Marshal(args)
		out, _ := registry.Execute(bus, "log_query", q)
		if out.Success || len(out.Observations) > 0 {
			t.Fatalf("bad raw call granted observation: %s", q)
		}
	}
}

func TestLogQueryPublicEmitRenderObservationOnly(t *testing.T) {
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "device.log", Data: []byte("10-09 01:02:03.123 12 34 E Camera: timeout\n")}})
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}
	rm.ExternalObservationPolicy = &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly, CurrentSourceMode: types.ExternalObservationCurrentSourceExclude, ExclusionKind: types.ExternalObservationSourceExclusionExplicitUserBoundary, Confidence: 1}
	bus.Language = "en"
	bus.Mutable.SetRequestModel(rm)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	out, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	bus.Mutable.AppendDispatchToolResult(out)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{out}})
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if len(ledger.Records) != 2 || !ledger.HasDirectRuntimeObservation() {
		t.Fatalf("native rows disappeared before final emit: %+v", ledger)
	}
	text := fmt.Sprintf("The device log records Camera: timeout, PID 12, TID 34. Log source device.log, decoded physical line 1, record %s. This is a recorded message, not proof of the underlying cause or source-code behavior.", out.Observations[0].Object)
	args, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "summary", "kind": "summary", "text": text}}})
	emitted, err := (&EmitAnswerDocument{}).Execute(bus, args)
	if err != nil || !emitted.Success {
		t.Fatal(err, emitted.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Citations) != 0 {
		t.Fatalf("log location laundered into source citation: %+v", doc.Citations)
	}
	visible := render.RenderAnswerDocument(doc, "en")
	if !strings.Contains(visible, "Camera: timeout") || !strings.Contains(visible, "decoded physical line 1") || !strings.Contains(visible, out.Observations[0].Object) {
		t.Fatal("log reference lost in render", visible)
	}
}

func TestLogQueryPublicEmptySourceCancellationAndStaleRawRef(t *testing.T) {
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "empty", Data: []byte{}}})
	out, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !out.Success || len(out.Observations) != 1 || out.Observations[0].Value != "0" {
		t.Fatal("complete empty source must remain distinguishable from failed source", err, out.Summary)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bus.Ctx = ctx
	out, err = registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if !errors.Is(err, context.Canceled) || out.Success || len(out.Observations) > 0 {
		t.Fatal("cancel published observations", err, out.Summary)
	}
	path := filepath.Join(t.TempDir(), "stale.log")
	if err := os.WriteFile(path, []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bus, registry = logQueryPublicBus(t, []loginput.Input{{Path: path}})
	out, err = registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	id := out.Observations[0].Object
	if err := os.WriteFile(path, []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"record_ref": id})
	out, _ = registry.Execute(bus, "log_query", params)
	if out.Success || len(out.Observations) > 0 || out.RawRef != "" {
		t.Fatal("stale raw record accepted", out.Summary)
	}
}

func TestLogQueryPublicUTF8RawChunkBoundary(t *testing.T) {
	bus, registry := logQueryPublicBus(t, []loginput.Input{{Name: "utf8", Data: []byte(strings.Repeat("中文", 1000) + "\n")}})
	listed, err := registry.Execute(bus, "log_query", json.RawMessage(`{}`))
	if err != nil || !listed.Success {
		t.Fatal(err, listed.Summary)
	}
	q, _ := json.Marshal(map[string]any{"record_ref": listed.Observations[0].Object})
	out, err := registry.Execute(bus, "log_query", q)
	var chunk struct {
		Text  string `json:"raw_text"`
		Data  []byte `json:"raw_base64"`
		Valid bool   `json:"chunk_utf8_valid"`
		End   int    `json:"byte_end"`
	}
	if err != nil || !out.Success {
		t.Fatal(err, out.Summary)
	}
	if err := json.Unmarshal([]byte(out.Summary), &chunk); err != nil {
		t.Fatal(err)
	}
	if !chunk.Valid || chunk.End != 4095 || !bytes.Equal([]byte(chunk.Text), chunk.Data) {
		t.Fatalf("UTF8 chunk not readable and byte-exact: %+v", chunk)
	}
}
