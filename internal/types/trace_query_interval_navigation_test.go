package types

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTraceIntervalNavigationPrivateReceipt(t *testing.T) {
	for _, kind := range []string{"native", "without_candidate", "failed", "memo", "wrong_tool", "wrong_path", "unqualified_source", "bad_params", "json", "old_turn", "foreign_run", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			path := writeTraceReadTestFile(t, t.TempDir(), "capture.systrace")
			m := NewMutableState("navigation")
			ref := m.PrepareTraceQuerySourceRead(path)
			out := nativeTraceSourceReadResult(path)
			out.TraceQueryWindowReplay = NativeTraceIntervalNavigationCandidate(path, []string{"measurements", "measurements", "process_measurements"})
			params := json.RawMessage(`{"view":"event_search","time_start":1,"time_end":2}`)
			switch kind {
			case "without_candidate":
				out.TraceQueryWindowReplay = TraceQueryWindowReplayRef{}
			case "failed":
				out.Success = false
			case "memo":
				out.ReusedFromRunMemo = true
			case "wrong_tool":
				out.ToolName = "read_file"
			case "wrong_path":
				out.TraceQueryWindowReplay = NativeTraceIntervalNavigationCandidate(path+".missing", []string{"measurements"})
			case "unqualified_source":
				out.TraceQuerySourceRead = TraceQuerySourceReadRef{}
			case "bad_params":
				params = json.RawMessage(`[]`)
			}
			m.StampTraceIntervalNavigation(context.Background(), ref, &out, params, nil)
			switch kind {
			case "json":
				raw, _ := json.Marshal(out)
				out = ToolResult{}
				if err := json.Unmarshal(raw, &out); err != nil {
					t.Fatal(err)
				}
			case "old_turn":
				m.ResetTurnAArtifacts()
			case "foreign_run":
				m = NewMutableState("another")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "canceled" {
				cancel()
			}
			actual, raw, views, ok := m.ResolveTraceIntervalNavigation(ctx, out.TraceQueryWindowReplay)
			if ok != (kind == "native") {
				t.Fatalf("%s: current=%v", kind, ok)
			}
			if ok {
				if actual != ref.Path() || len(views) != 2 || views[0] != "measurements" || !json.Valid(raw) {
					t.Fatal(actual, string(raw), views)
				}
				views[0] = "changed"
				raw[0] = 'x'
				_, raw2, views2, current := m.ResolveTraceIntervalNavigation(ctx, out.TraceQueryWindowReplay)
				if !current || !json.Valid(raw2) || views2[0] != "measurements" {
					t.Fatal("mutable result escaped receipt")
				}
			}
			m.AppendDispatchToolResult(out)
			if _, current := m.ResolveTraceQuerySourceRead(path); current {
				t.Fatal("navigation granted physical read")
			}
			if _, _, current := m.ResolveTraceQueryWindowReplay(out.TraceQueryWindowReplay); current {
				t.Fatal("navigation enlarged original replay")
			}
		})
	}
}
