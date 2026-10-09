package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTransactionHandoffsPublicProtocol(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.systrace")
	data := "# tracer: nop\n" +
		" app-101 (  100) [000] .... 1.001000: tracing_mark_write: B|100|H:MarshRSTransactionData cmdCount: 1, transactionFlag:[101,7]\n" +
		" rs-201 (  200) [001] .... 1.020000: tracing_mark_write: B|200|H:RSMainThread::ProcessCommandUni [101,7]\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "transaction_handoffs", "time_start": 1, "time_end": 1.05})
	r, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir()}, params)
	if err != nil || !r.Success || !strings.Contains(r.Summary, "唯一交接") {
		t.Fatalf("missing exact native handoff public path: err=%v result=%+v", err, r)
	}
	var observation types.ObservationRecord
	for _, rec := range r.Observations {
		if rec.Predicate == TraceTransactionHandoffsPredicate {
			observation = rec
		}
	}
	p, ok := DecodeTraceTransactionHandoffs(observation)
	if !ok || p.TotalKeys != 1 || p.Handoffs[0].Status != "observed_unique_protocol_match" {
		t.Fatalf("public observation invalid %+v %+v", observation, p)
	}
	if r.TraceEvidenceAuthority != nil {
		t.Fatal("transaction observation minted causal authority")
	}
	for _, change := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.Producer = "model" }, func(r *types.ObservationRecord) { r.SourceRef.Path += "other" }, func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs += 1 }, func(r *types.ObservationRecord) {
			r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
		},
	} {
		bad := observation
		change(&bad)
		if _, ok := DecodeTraceTransactionHandoffs(bad); ok {
			t.Fatal("mismatched/duplicate observation admitted")
		}
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), "transaction_handoffs correlates exact") {
		t.Fatal("tool schema missing protocol contract")
	}
}

func TestTransactionHandoffsPublicRequestDefaultsAndReplay(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	start, end := 1.0, 1.05
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("transactions"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到1.05秒"}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("query failed %v %+v", err, r)
	}
	if _, _, ok := ctx.Mutable.ResolveTraceQueryWindowReplay(r.TraceQueryWindowReplay); !ok {
		t.Fatal("native query cannot participate in bounded primary-window replay")
	}
	raw, _ := json.Marshal(r)
	var replay types.ToolResult
	json.Unmarshal(raw, &replay)
	if _, _, ok := ctx.Mutable.ResolveTraceQueryWindowReplay(replay.TraceQueryWindowReplay); ok {
		t.Fatal("history JSON minted private replay")
	}
}
