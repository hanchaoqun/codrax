package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitLogTriagePublicSourceCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		inputs   []loginput.Input
		wantLine int
	}{
		{"unique physical line", []loginput.Input{{Name: "app/session.log", Data: []byte("header\nunique evidence\n")}}, 2},
		{"duplicate sources", []loginput.Input{{Name: "app/session.log", Data: []byte("unique evidence\n")}, {Name: "kernel/session.log", Data: []byte("header\nunique evidence\n")}}, 0},
		{"preview only", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := &types.BusContext{AttachedLog: "header\nunique evidence\n", Mutable: types.NewMutableState("inspect logs")}
			if tc.inputs != nil {
				catalog, err := loginput.Prepare(context.Background(), tc.inputs, loginput.Options{})
				if err != nil {
					t.Fatal(err)
				}
				bus.AttachedLogCatalog = catalog
				bus.AttachedLog = catalog.Preview(1000)
			}
			registry := NewRegistry()
			registry.Register(&EmitLogTriage{})
			result, err := registry.Execute(bus, "emit_log_triage", json.RawMessage(`{"meta":{"lang":"unknown","signals":[]},"errors":[],"observations":[{"kind":"runtime_event","summary":"advisory","evidence":"unique evidence","line_start":99,"line_end":101,"diagnostic":false,"confidence":1}]}`))
			if err != nil || !result.Success {
				t.Fatalf("emit: %+v %v", result, err)
			}
			bundle := bus.Mutable.LogTriage()
			if bundle == nil || len(bundle.Observations) != 1 {
				t.Fatalf("missing observation: %+v", bundle)
			}
			got := bundle.Observations[0]
			if got.LineStart != tc.wantLine || got.LineEnd != tc.wantLine {
				t.Fatalf("model coordinates became original-source facts: got=%d-%d want=%d; %+v", got.LineStart, got.LineEnd, tc.wantLine, got)
			}
		})
	}
}

func emitBoundLogObservation(t *testing.T, bus *types.BusContext, rows []map[string]any) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": []any{}, "observations": rows})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	registry.Register(&EmitLogTriage{})
	result, err := registry.Execute(bus, "emit_log_triage", params)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func logObservationInput(text, source string) map[string]any {
	row := map[string]any{"kind": "runtime_event", "summary": "advisory, not a proven producer", "evidence": text, "line_start": 999, "diagnostic": false, "confidence": 1}
	if source != "" {
		row["source_id"] = source
	}
	return row
}

func TestEmitLogTriagePublicSourceSelectionAndLedger(t *testing.T) {
	text := "same exact original event"
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "a/session.log", Data: []byte("prefix\n" + text + "\n")}, {Name: "b/session.log", Data: []byte(text + "\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(4), Mutable: types.NewMutableState("inspect logs")}
	sources := catalog.Sources()
	result := emitBoundLogObservation(t, bus, []map[string]any{logObservationInput(text, sources[0].ID), logObservationInput(text, sources[1].ID)})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bundle := bus.Mutable.LogTriage()
	if len(bundle.Observations) != 2 {
		t.Fatalf("same text from different sources merged: %+v", bundle.Observations)
	}
	for i, obs := range bundle.Observations {
		if !obs.SourceBinding.IsVerified() || obs.SourceBinding.SourceID != sources[i].ID {
			t.Fatalf("unbound row: %+v", obs)
		}
	}
	cloned := logtriage.ValidateBundle(logtriage.ValidateInput{Observations: bundle.Observations}, "")
	if cloned == nil || len(cloned.Observations) != 2 {
		t.Fatalf("clone dropped sources: %+v", cloned)
	}
	if cloned.Observations[0].SourceBinding == bundle.Observations[0].SourceBinding {
		t.Fatal("binding pointer shared across bundle validation")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
	if len(ledger.Records) != 2 {
		t.Fatalf("ledger count: %+v", ledger.Records)
	}
	for i, row := range ledger.Records {
		if row.SourceRef.ArtifactID != sources[i].ID || row.Span.LineStart != 2-i || row.Origin != types.AnswerEvidenceOriginRuntimeArtifact || row.SourceRef.Kind != types.ObservationSourceRuntimeArtifact || row.SourceRef.ClockCalibrated {
			t.Fatalf("wrong source/authority: %+v", row)
		}
	}
	encoded, _ := json.Marshal(bundle)
	var restored types.LogBundle
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	ledger = types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: &restored})
	for _, row := range ledger.Records {
		if row.Span.LineStart != 0 || row.SourceRef.ArtifactID != "attached_log" {
			t.Fatalf("JSON restored physical provenance: %+v", row)
		}
	}
	for _, bad := range []map[string]any{logObservationInput(text, "forged-source"), {"kind": "runtime_event", "summary": "x", "evidence": text, "source_binding": map[string]any{"status": "unique"}, "diagnostic": false, "confidence": 1}} {
		params, _ := json.Marshal(map[string]any{"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": []any{}, "observations": []any{bad}})
		registry := NewRegistry()
		registry.Register(&EmitLogTriage{})
		if r, _ := registry.Execute(bus, "emit_log_triage", params); r.Success {
			t.Fatalf("forged binding accepted: %+v", r)
		}
	}
}

func TestEmitLogTriagePublicStaleSourcesAndBeyondPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	raw := strings.Repeat("ordinary line\n", 2000) + "exact tail evidence\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Path: path}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(64), Mutable: types.NewMutableState("inspect logs")}
	if r := emitBoundLogObservation(t, bus, []map[string]any{logObservationInput("exact tail evidence", "")}); !r.Success {
		t.Fatal(r.Summary)
	}
	obs := bus.Mutable.LogTriage().Observations[0]
	if obs.LineStart != 2001 || !obs.SourceBinding.IsVerified() || obs.SourceBinding.Path != path {
		t.Fatalf("tail not bound: %+v", obs)
	}
	bus.AttachedLog = raw
	if err := os.WriteFile(path, []byte("changed source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := emitBoundLogObservation(t, bus, []map[string]any{logObservationInput("exact tail evidence", "")}); !r.Success {
		t.Fatal(r.Summary)
	}
	obs = bus.Mutable.LogTriage().Observations[0]
	if obs.LineStart != 0 || obs.SourceBinding.IsVerified() || obs.SourceBinding.Status != "source_unavailable" || obs.Evidence != "exact tail evidence" {
		t.Fatalf("stale source upgraded preview: %+v", obs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bus.Ctx = ctx
	registry := NewRegistry()
	registry.Register(&EmitLogTriage{})
	r, err := registry.Execute(bus, "emit_log_triage", json.RawMessage(`{"meta":{"lang":"unknown","signals":[]},"errors":[],"observations":[{"kind":"runtime_event","summary":"x","evidence":"exact tail evidence","diagnostic":false,"confidence":1}]}`))
	if r.Success || err == nil {
		t.Fatalf("canceled lookup published: %+v %v", r, err)
	}
}

func TestEmitLogTriagePublicErrorBindingKeepsCauseAndCardinality(t *testing.T) {
	raw := "outer failure\nCaused by: inner failure\n"
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "error.log", Data: []byte(raw)}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: raw, Mutable: types.NewMutableState("inspect")}
	registry := NewRegistry()
	registry.Register(&EmitLogTriage{})
	params := json.RawMessage(`{"meta":{"lang":"unknown","signals":[]},"errors":[{"type":"Outer","message":"outer failure","cause":{"type":"Inner","message":"inner failure"},"cause_relation":{"authority":"explicit_artifact_marker","marker":"Caused by: inner failure"}}]}`)
	r, err := registry.Execute(bus, "emit_log_triage", params)
	if err != nil || !r.Success {
		t.Fatalf("nested error rejected: %+v %v", r, err)
	}
	e := bus.Mutable.LogTriage().Errors[0]
	if !e.SourceBinding.IsVerified() || e.Cause == nil || !e.Cause.SourceBinding.IsVerified() || e.Cause.SourceBinding.FirstLine != 2 || e.CauseRelation == nil {
		t.Fatalf("error tree/source lost: %+v", e)
	}
	cloned := logtriage.ValidateBundle(logtriage.ValidateInput{Errors: []types.LogError{e}}, "")
	if cloned.Errors[0].SourceBinding == e.SourceBinding || cloned.Errors[0].Cause.SourceBinding == e.Cause.SourceBinding || !cloned.Errors[0].Cause.SourceBinding.IsVerified() {
		t.Fatal("error clone lost receipt or shares writable provenance")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bus.Mutable.LogTriage()})
	if len(ledger.Records) != 2 || ledger.Records[0].Span.LineStart != 1 || ledger.Records[1].Span.LineStart != 2 || ledger.Records[1].ProvenanceLane != types.ObservationProvenanceObservedDirectCause {
		t.Fatalf("error coordinates/cause lane lost: %+v", ledger.Records)
	}
	params, _ = json.Marshal(map[string]any{"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": []any{map[string]any{"type": "A", "message": "outer failure", "source_id": catalog.Sources()[0].ID}, map[string]any{"type": "B", "message": "outer failure"}}})
	r, err = registry.Execute(bus, "emit_log_triage", params)
	if err != nil || r.Success {
		t.Fatalf("selected+global double-count bypass: %+v %v", r, err)
	}
}

func TestEmitLogTriagePublicPhysicalLinesDoNotJoinPreviewProtocol(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "original.log", Data: []byte("native event\n")}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: "native event\n", Mutable: types.NewMutableState("inspect")}
	if r := emitBoundLogObservation(t, bus, []map[string]any{logObservationInput("native event", "")}); !r.Success {
		t.Fatal(r.Summary)
	}
	bundle := bus.Mutable.LogTriage()
	// This independent decoder uses the combined display surface. Equal line
	// numbers do not establish common source identity with a physical source.
	bundle.OperationalSemantics = []types.LogOperationalSemantic{{Protocol: "codrax", Producer: "orchestrator", LineStart: 1, LineEnd: 1, RawExcerpt: "different preview event"}}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
	for _, row := range ledger.Records {
		if row.Producer != "log_triage" {
			continue
		}
		if row.Role != types.AnswerAggregateRolePrincipalAnswer || strings.Contains(strings.Join(row.RichNotes, "\n"), "superseded_by") {
			t.Fatalf("physical line joined unrelated preview protocol: %+v", row)
		}
	}
}
