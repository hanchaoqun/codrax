package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeWindowSourcePublicCorrectsDeclaredLiteral(t *testing.T) {
	for _, attached := range []bool{false, true} {
		const request = "只分析在1.000到1.050秒里的渲染事件。"
		ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
		if attached {
			ctx.AttachedHitrace = "capture extent 1.001..1.051"
		}
		scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 1.001, "time_end": 1.051, "source_quote": "1.000到1.050秒", "confidence": 1}
		raw := boundedScopeAuthorityPublicParams(t, scope, request)
		before := append([]byte(nil), raw...)
		result, err := (&EmitAnalysis{}).Execute(ctx, raw)
		if err != nil || !result.Success {
			t.Fatalf("literal correction failed: %+v %v", result, err)
		}
		profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
		start, end, ok := profile.ExplicitTimeWindow()
		if !ok || start != 1 || end != 1.05 || profile.SourceQuote != "1.000到1.050秒" {
			t.Fatalf("typed bounds borrowed model/capture extent: %+v", profile)
		}
		if !strings.Contains(result.Summary, "literal endpoints") || !bytes.Equal(raw, before) {
			t.Fatalf("correction must be observable without changing model JSON: %s", result.Summary)
		}
	}
}

func TestRuntimeWindowSourcePublicRejectsUnboundBoundsAtomically(t *testing.T) {
	for _, quote := range []string{"the first response", "1..2 and 4..5", "[1,2]", "(1,2)", "1tick to 2ticks", "-1..2"} {
		request := "Inspect " + quote + "."
		mu := types.NewMutableState(request)
		mu.SetRequestModel(types.RequestModel{RawRequest: "previous accepted request"})
		before, _ := json.Marshal(mu.RequestModel())
		scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 1.1, "time_end": 2.1, "source_quote": quote, "confidence": 1}
		result, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, boundedScopeAuthorityPublicParams(t, scope, request))
		if err != nil || result.Success || !strings.Contains(result.Summary, "bounded_selector") {
			t.Errorf("unbound explicit coordinates must have a bounded repair: quote=%q result=%+v err=%v", quote, result, err)
		}
		after, _ := json.Marshal(mu.RequestModel())
		if !bytes.Equal(before, after) {
			t.Errorf("invalid quote %q replaced previous accepted state", quote)
		}
		// The repair is an executable existing lane, not a retry loop that
		// requires inventing absolute endpoints or silently selecting all data.
		escape := map[string]any{"requested_scope": "bounded_selector", "source_quote": quote, "confidence": 1}
		repaired, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, boundedScopeAuthorityPublicParams(t, escape, request))
		if err != nil || !repaired.Success || mu.RequestModel().RuntimeArtifactScopeProfile.HasExplicitTimeWindows() || mu.RequestModel().RuntimeArtifactScopeProfile.RequestedScope != types.RuntimeArtifactScopeBoundedSelector {
			t.Fatalf("existing bounded repair lane failed: %+v %v", repaired, err)
		}
	}
}

func TestRuntimeWindowSourcePublicCorrectedWindowReachesPreparedQuery(t *testing.T) {
	ctx, prep, _ := hmc17NamedPathContext(t)
	const request = "List thread 101 allocation events in 10000..10050 ms."
	ctx.Mutable = types.NewMutableState(request)
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	m, err := prep.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = m, m.Preview()
	scope := map[string]any{"requested_scope": "explicit_time_window", "time_start": 10.001, "time_end": 10.051, "source_quote": "10000..10050 ms", "confidence": 1}
	result, err := (&EmitAnalysis{}).Execute(ctx, boundedScopeAuthorityPublicParams(t, scope, request))
	if err != nil || !result.Success {
		t.Fatalf("emit failed: %+v %v", result, err)
	}
	query := hmc17NamedQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "resource_stack", "pid": 101})
	payload := hmc17NamedPayload(t, query)
	if payload.TimeStart != 10 || payload.TimeEnd != 10.05 || payload.ResourceStack == nil || payload.ResourceStack.MatchedEvents != 3 || payload.ResourceStack.Window.EndInclusive {
		t.Fatalf("corrected request was lost before native window selection: %+v", payload)
	}
	if len(ctx.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatal("automatic bounds were relabeled as a model-authored query")
	}
}

func TestRuntimeWindowSourceLiteralUnitsAndOrderedMembers(t *testing.T) {
	for _, tc := range []struct {
		quote      string
		start, end float64
	}{
		{"1.000到1.050秒", 1, 1.05},
		{"[1000,1050) ms", 1, 1.05},
		{"[1000ms,1.05s)", 1, 1.05},
		{"1000000µs–1050000μs", 1, 1.05},
		{"1000000000至1050000000纳秒", 1, 1.05},
		{"1e3..1.05e3 milliseconds", 1, 1.05},
		{".1-.3s", .1, .3},
		{"0..1", 0, 1},
		{"+0 to +1 seconds", 0, 1},
		{"between 1000 and 1050 milliseconds", 1, 1.05},
		{"1s 500ms to 2s 500ms", 1.5, 2.5},
		{"1 minute to 2 minutes", 60, 120},
		{"1000us..2000", .001, .002},
		{"请看1.000到1.050秒期间的事件", 1, 1.05},
		{"For process 900, 1..1.05 seconds.", 1, 1.05},
	} {
		t.Run(tc.quote, func(t *testing.T) {
			start, end, confidence := tc.start+.001, tc.end+.001, 1.0
			if tc.quote == "0..1" {
				start, end = tc.start, tc.end
			}
			p := &emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", TimeStart: &start, TimeEnd: &end, SourceQuote: tc.quote, Confidence: &confidence}
			profile, err, _ := parseRuntimeArtifactScopeProfile("Inspect "+tc.quote, false, p)
			if err != "" || profile == nil {
				t.Fatalf("literal rejected: %s", err)
			}
			gotStart, gotEnd, ok := profile.ExplicitTimeWindow()
			if !ok || gotStart != tc.start || gotEnd != tc.end {
				t.Fatalf("wrong exact conversion: %.17g..%.17g, want %.17g..%.17g", gotStart, gotEnd, tc.start, tc.end)
			}
			if *p.TimeStart != start || *p.TimeEnd != end {
				t.Fatal("input parameter mutated")
			}
		})
	}
	const request = "Compare 0..1, 0.25..0.75, and 3..4."
	const members = `[{"time_start":3,"time_end":4,"source_quote":"0..1, 0.25..0.75, and 3..4"},{"time_start":0,"time_end":1,"source_quote":"0..1, 0.25..0.75, and 3..4"},{"time_start":0.25,"time_end":0.75,"source_quote":"0..1, 0.25..0.75, and 3..4"},{"time_start":0,"time_end":1,"source_quote":"0..1, 0.25..0.75, and 3..4"}]`
	ctx := &types.BusContext{Mutable: types.NewMutableState(request)}
	result, err := (&EmitAnalysis{}).Execute(ctx, b1626MultiWindowAnalysisParams(t, members, "bounded_fact_set", request))
	if err != nil || !result.Success {
		t.Fatalf("shared quote with exact member pairs rejected: %+v %v", result, err)
	}
	profile := ctx.Mutable.RequestModel().RuntimeArtifactScopeProfile
	if len(profile.TimeWindows) != 4 || *profile.TimeWindows[0].TimeStart != 3 || *profile.TimeWindows[1].TimeStart != 0 || *profile.TimeWindows[2].TimeStart != .25 || *profile.TimeWindows[3].TimeStart != 0 {
		t.Fatalf("members were sorted, enveloped, or deduplicated: %+v", profile)
	}
	if profile.ContainsExplicitTimeWindow(1.5, 2) {
		t.Fatal("a gap acquired explicit request authority")
	}
	if _, allowed := types.RuntimeTraceReportShapeAuthority(ctx.Mutable.RequestModel()); allowed {
		t.Fatal("literal source binding granted causal report authority")
	}
}

func TestRuntimeWindowSourceRejectsInvalidLiteralWithoutPartialState(t *testing.T) {
	for _, quote := range []string{
		"opaque selector", "9..10 from an older request", "(1,2]", "[1,2)", "2..1", "1..1", "-1..2", "1..-2", "prefix1..2", "1..2ticks", "1..2 ticks", "1..2帧", "1到3毫微秒", "1到3 毫微秒", "1 to 3 frames", "00:01-02:03", "00:01-02:03s", "a/1..2", "1..2/3", "1..2.3.4", "1e99999..2e99999s", "1e-99999..2e-99999s", "1e309..2e309", "1e-400..2e-400", "1e-400..2", "1 and 2", "1..2 and 3..4", "1s 500ms..2", "1..2s 500ms", "1000..1050",
	} {
		t.Run(quote, func(t *testing.T) {
			request := "Inspect " + quote
			if quote == "9..10 from an older request" {
				request = "Inspect 1..2"
			}
			start, end, confidence := 1.1, 2.1, 1.0
			if quote == "[1,2)" {
				end = start
			} // Valid quote never repairs malformed typed shape.
			p := &emitRuntimeArtifactScopeProfileParam{RequestedScope: "explicit_time_window", TimeStart: &start, TimeEnd: &end, SourceQuote: quote, Confidence: &confidence}
			profile, err, _ := parseRuntimeArtifactScopeProfile(request, true, p)
			if err == "" || profile != nil || !strings.Contains(err, "bounded_selector") {
				t.Fatalf("invalid explicit scope was accepted/softened without escape: %+v %q", profile, err)
			}
		})
	}
	const request = "Compare 1..2 seconds with the next response."
	const members = `[{"time_start":1.1,"time_end":2.1,"source_quote":"1..2 seconds"},{"time_start":3,"time_end":4,"source_quote":"the next response"}]`
	mu := types.NewMutableState(request)
	mu.SetRequestModel(types.RequestModel{RawRequest: "previous accepted request"})
	before, _ := json.Marshal(mu.RequestModel())
	result, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, b1626MultiWindowAnalysisParams(t, members, "bounded_fact_set", request))
	after, _ := json.Marshal(mu.RequestModel())
	if err != nil || result.Success || !bytes.Equal(before, after) {
		t.Fatalf("a later invalid member leaked earlier correction: %+v %v", result, err)
	}
}
