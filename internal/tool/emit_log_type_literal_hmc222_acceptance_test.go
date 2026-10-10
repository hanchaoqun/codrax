package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc222LiteralBus(t *testing.T, inputs []loginput.Input) *types.BusContext {
	t.Helper()
	catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(8192), Mutable: types.NewMutableState("说明这份日志里的错误记录")}
}

func hmc222EmitLiteral(t *testing.T, bus *types.BusContext, typ, message, source string) types.LogError {
	t.Helper()
	entry := map[string]any{"type": typ, "message": message}
	if source != "" {
		entry["source_id"] = source
	}
	params, err := json.Marshal(map[string]any{"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": []any{entry}})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	r.Register(&EmitLogTriage{})
	result, err := r.Execute(bus, "emit_log_triage", params)
	if err != nil || !result.Success {
		t.Fatalf("literal provenance must not reject an otherwise valid observation: %+v %v", result, err)
	}
	bundle := bus.Mutable.LogTriage()
	if bundle == nil || len(bundle.Errors) != 1 {
		t.Fatalf("error occurrence lost: %+v", bundle)
	}
	return bundle.Errors[0]
}

func hmc222AssertLiteral(t *testing.T, e types.LogError, want string) {
	t.Helper()
	if got := e.ObservedTypeLiteral(); got != want {
		t.Fatalf("literal=%q want=%q; error=%+v", got, want, e)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: &types.LogBundle{Errors: []types.LogError{e}}})
	if len(ledger.Records) != 1 {
		t.Fatalf("observation lost: %+v", ledger.Records)
	}
	if want != "" && ledger.Records[0].Subject != want {
		t.Fatalf("proven native label lost in handoff: %+v", ledger.Records[0])
	}
	if want == "" && e.Type != e.Message && ledger.Records[0].Subject == e.Type {
		t.Fatalf("unproven type promoted to observed subject: %+v", ledger.Records[0])
	}
}

func TestHMC222LiteralAcceptancePhysicalFieldBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, raw, other, message, want string
		selectSource                    bool
	}{
		{name: "same line outside message", raw: "10-10 12:00:00.000 1 2 E Driver: BufferFault: release failed\n", message: "release failed", want: "BufferFault"},
		{name: "native header only", raw: "BufferFault\n", want: "BufferFault"},
		{name: "CRLF line", raw: "BufferFault: release failed\r\nother\r\n", message: "release failed", want: "BufferFault"},
		{name: "same label distinct messages", raw: "BufferFault: release failed\nBufferFault: allocation failed\n", message: "release failed", want: "BufferFault"},
		{name: "neighbor line", raw: "BufferFault: unrelated failure\nrelease failed\n", message: "release failed"},
		{name: "neighbor source", raw: "release failed\n", other: "BufferFault: unrelated failure\n", message: "release failed", selectSource: true},
		{name: "continuation only", raw: "10-10 12:00:00.000 1 2 E Driver: release failed\n    at BufferFault.handle\n", message: "release failed"},
		{name: "literal and own message on same continuation", raw: "10-10 12:00:00.000 1 2 E Driver: outer failure\n    Caused by: BufferFault: release failed\n", message: "release failed", want: "BufferFault"},
		{name: "multiline message cannot borrow continuation", raw: "10-10 12:00:00.000 1 2 E Driver: release failed\n    at BufferFault.handle\n", message: "release failed\n    at BufferFault.handle"},
		{name: "ambiguous identical messages", raw: "BufferFault: release failed\n", other: "BufferFault: release failed\n", message: "release failed"},
		{name: "explicit source resolves ambiguity", raw: "BufferFault: release failed\n", other: "BufferFault: release failed\n", message: "release failed", selectSource: true, want: "BufferFault"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs := []loginput.Input{{Name: "app.log", Data: []byte(tc.raw)}}
			if tc.other != "" {
				inputs = append(inputs, loginput.Input{Name: "neighbor.log", Data: []byte(tc.other)})
			}
			bus := hmc222LiteralBus(t, inputs)
			source := ""
			if tc.selectSource {
				source = bus.AttachedLogCatalog.Sources()[0].ID
			}
			e := hmc222EmitLiteral(t, bus, "BufferFault", tc.message, source)
			hmc222AssertLiteral(t, e, tc.want)
			if e.Message != tc.message {
				t.Fatalf("raw message changed: got=%q want=%q", e.Message, tc.message)
			}
		})
	}
}

func TestHMC222LiteralAcceptanceCloneJSONAndMutation(t *testing.T) {
	bus := hmc222LiteralBus(t, []loginput.Input{{Name: "app.log", Data: []byte("BufferFault: release failed\n")}})
	e := hmc222EmitLiteral(t, bus, "BufferFault", "release failed", "")
	hmc222AssertLiteral(t, e, "BufferFault")
	cloned := logtriage.ValidateBundle(logtriage.ValidateInput{Errors: []types.LogError{e}}, "")
	if cloned == nil || len(cloned.Errors) != 1 || cloned.Errors[0].SourceBinding == e.SourceBinding {
		t.Fatalf("clone lost occurrence or shares binding: %+v", cloned)
	}
	hmc222AssertLiteral(t, cloned.Errors[0], "BufferFault")
	for _, tc := range []struct {
		name   string
		mutate func(*types.LogError)
	}{
		{"type", func(e *types.LogError) { e.Type = "InventedFault" }},
		{"message", func(e *types.LogError) { e.Message = "another message" }},
		{"source", func(e *types.LogError) { e.SourceBinding.SourceID = "neighbor-source" }},
		{"generation", func(e *types.LogError) { e.SourceBinding.Generation += "-changed" }},
		{"coordinates", func(e *types.LogError) { e.SourceBinding.FirstLine++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := e
			changed.SourceBinding = e.SourceBinding.Clone()
			tc.mutate(&changed)
			hmc222AssertLiteral(t, changed, "")
			hmc222AssertLiteral(t, e, "BufferFault")
		})
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.LogError
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	hmc222AssertLiteral(t, restored, "")
	e.SourceBinding.FirstLine++
	hmc222AssertLiteral(t, cloned.Errors[0], "BufferFault")
}

func TestHMC222LiteralAcceptanceSourceReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	raw := []byte("BufferFault: release failed\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	bus := hmc222LiteralBus(t, []loginput.Input{{Path: path}})
	original := hmc222EmitLiteral(t, bus, "BufferFault", "release failed", "")
	hmc222AssertLiteral(t, original, "BufferFault")
	replacement := filepath.Join(dir, "replacement.log")
	if err := os.WriteFile(replacement, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	changed := hmc222EmitLiteral(t, bus, "BufferFault", "release failed", "")
	hmc222AssertLiteral(t, changed, "")
	if changed.SourceBinding.IsVerified() || changed.SourceBinding.Status != "source_unavailable" {
		t.Fatalf("same-byte replacement revived stale catalog authority: %+v", changed.SourceBinding)
	}
	// The prior receipt describes the held historical generation, not new I/O.
	hmc222AssertLiteral(t, original, "BufferFault")
	if original.SourceBinding.Generation == "" {
		t.Fatal("historical literal lost its source generation")
	}
}
