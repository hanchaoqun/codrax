package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func requestWindowTestProfile(start, end float64) *types.RuntimeArtifactScopeProfile {
	return &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "explicit request window"}
}

func TestTraceQueryRequestWindowAllRegisteredViews(t *testing.T) {
	ctx := &types.BusContext{AttachedHitrace: "current attachment", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(0, .012345678)}}}
	for _, view := range append(tracequery.CanonicalViewNames(), "", "frame_bundle") {
		t.Run(view, func(t *testing.T) {
			p, caveat := traceQueryApplyRequestWindow(ctx, traceQueryParams{View: view}, "/admitted/trace", "attached_trace")
			if !p.TimeStart.Set() || !p.TimeEnd.Set() || p.TimeStart.Seconds() != 0 || p.TimeEnd.Seconds() != .012345678 || caveat == "" || p.TimeEnd.QueryToleranceSeconds() != 0 {
				t.Fatalf("missing exact shared default: %+v %q", p, caveat)
			}
		})
	}
}

func TestTraceQueryRequestWindowNeverChangesCallerScopes(t *testing.T) {
	for _, raw := range []string{`{"time_start":0}`, `{"time_end":12}`, `{"time_start":1,"time_end":2}`, `{"line_start":1}`, `{"line_end":20}`, `{"line_start":-1}`, `{"business_span_ref":"instance"}`} {
		var p traceQueryParams
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatal(err)
		}
		ctx := &types.BusContext{AttachedHitrace: "current attachment", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 11)}}}
		got, caveat := traceQueryApplyRequestWindow(ctx, p, "/admitted/trace", "attached_trace")
		if !reflect.DeepEqual(got, p) || caveat != "" {
			t.Fatalf("changed explicit scope %s: %+v %s", raw, got, caveat)
		}
	}
	one := requestWindowTestProfile(10, 11)
	member := types.RuntimeArtifactTimeWindow{TimeStart: one.TimeStart, TimeEnd: one.TimeEnd, SourceQuote: one.SourceQuote}
	for name, profile := range map[string]*types.RuntimeArtifactScopeProfile{
		"absent":                  nil,
		"full":                    {RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "whole trace"},
		"invalid":                 requestWindowTestProfile(11, 10),
		"unanchored":              {RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: one.TimeStart, TimeEnd: one.TimeEnd},
		"multiple_even_identical": {RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{member, member}},
		"conflicting_scalar_list": {RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: one.TimeStart, TimeEnd: one.TimeEnd, TimeWindows: []types.RuntimeArtifactTimeWindow{member}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := &types.BusContext{AttachedHitrace: "current attachment", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile}}}
			p, note := traceQueryApplyRequestWindow(ctx, traceQueryParams{}, "/admitted/trace", "attached_trace")
			if p.TimeStart.Set() || p.TimeEnd.Set() || note != "" {
				t.Fatal("invented a request window")
			}
		})
	}
	ctx := &types.BusContext{AttachedHitrace: "current attachment", Mutable: types.NewMutableState("request")}
	ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{member}}})
	if p, _ := traceQueryApplyRequestWindow(ctx, traceQueryParams{}, "/admitted/trace", "attached_trace"); !p.TimeStart.Set() {
		t.Fatal("single array or mutable request fallback lost")
	}
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	if p, _ := traceQueryApplyRequestWindow(ctx, traceQueryParams{}, "/admitted/trace", "attached_trace"); p.TimeStart.Set() {
		t.Fatal("changed system supplement election")
	}
}

func TestTraceQueryRequestWindowStaleBlobIsNotCurrentAttachment(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 11)}}}
	path := filepath.Join(t.TempDir(), "old-attached-trace.txt")
	if err := os.WriteFile(path, []byte("old trace material"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, caveat := traceQueryApplyRequestWindow(ctx, traceQueryParams{}, path, "attached_trace"); p.TimeStart.Set() || caveat != "" {
		t.Fatal("old source-resolver blob falsely bound the current request window")
	}
}

func TestTraceQueryRequestWindowPublicPreparedSourceAndRegistry(t *testing.T) {
	ctx, prep, _ := hmc17NamedPathContext(t)
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	m, err := prep.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = m, m.Preview()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 10.05)}}
	for _, args := range []map[string]any{{"source": "attached_trace"}, {"source": "path", "path": path}, {"source": "path", "path": m.QueryPath()}} {
		args["view"], args["pid"] = "resource_stack", 101
		result := hmc17NamedQuery(t, ctx, args)
		payload := hmc17NamedPayload(t, result)
		if payload.ResourceStack == nil || payload.ResourceStack.MatchedEvents != 3 || payload.ResourceStack.Window.EndInclusive || !strings.Contains(result.Summary, "trace_query_request_window_inherited=") {
			t.Fatalf("lost request window: %+v %s", payload.ResourceStack, result.Summary)
		}
	}
	if calls := ctx.Mutable.TraceQueryCallWindows(); len(calls) != 0 {
		t.Fatalf("inherited request window was recorded as a model call: %+v", calls)
	}
	for _, view := range []string{"event_search", "window_stats"} {
		result := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": view, "pid": 101})
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != 10 || payload.TimeEnd != 10.05 {
			t.Fatalf("%s ignored common default: %v..%v", view, payload.TimeStart, payload.TimeEnd)
		}
	}
	// A distinct, prepared capture cannot borrow the attachment's window.
	body, _ := os.ReadFile(path)
	other := filepath.Join(t.TempDir(), "other.data")
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	result := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": other, "view": "resource_stack", "pid": 101})
	payload := hmc17NamedPayload(t, result)
	if payload.ResourceStack.MatchedEvents != 4 || !payload.ResourceStack.Window.EndInclusive || strings.Contains(result.Summary, "trace_query_request_window_inherited=") {
		t.Fatal("different capture inherited an unbound window")
	}
	// A deliberately explicit wider probe stays wider and is the only model
	// query allowed onto the supplement's model-call window registry.
	result = hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "resource_stack", "pid": 101, "time_start": 9, "time_end": 11})
	payload = hmc17NamedPayload(t, result)
	if payload.ResourceStack.MatchedEvents != 4 || payload.ResourceStack.Window.StartTs != 9 || payload.ResourceStack.Window.EndTs != 11 {
		t.Fatal("overwrote explicit exploration scope")
	}
	if calls := ctx.Mutable.TraceQueryCallWindows(); len(calls) != 1 || calls[0].TimeStart != 9 || calls[0].TimeEnd != 11 {
		t.Fatalf("wrong model-call registry %+v", calls)
	}
}

func TestTraceQueryRequestWindowUniquePreflightPreparedPath(t *testing.T) {
	ctx, _, _ := hmc17NamedPathContext(t)
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(10, 10.05)}}
	query := func() *tracequery.ResourceStackResult {
		return hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101})).ResourceStack
	}
	if p := query(); !p.Window.EndInclusive || p.MatchedEvents != 4 {
		t.Fatal("an arbitrary model path borrowed a request window")
	}
	ctx.RuntimeArtifactPreflight.Artifacts = []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Carrier: "request_path", Source: path}}
	if p := query(); p.Window.EndInclusive || p.MatchedEvents != 3 {
		t.Fatal("unique current preflight source not resolved through preparation")
	}
	ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Carrier: "request_path", Source: "/another/trace.systrace"})
	if p := query(); !p.Window.EndInclusive || p.MatchedEvents != 4 {
		t.Fatal("ambiguous current sources elected a window owner")
	}
}
